// Package ota provides automatic OTA update functionality with canary strategy and quiet window scheduling.
package ota

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Prometheus metrics for auto-update operations
var (
	autoUpdateTriggerCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "auto_update_triggers_total",
			Help: "Total number of auto-update trigger events",
		},
		[]string{"trigger_type", "result"}, // trigger_type: "auto", result: "success" or "failure"
	)

	autoUpdateFirmwareVersion = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "spaxel_autoupdate_firmware_version",
			Help: "Current firmware version being deployed by auto-update (encoded as semantic version)",
		},
		[]string{"stage"}, // selected, canary_before, canary_after, fleet_target
	)
)

// AutoUpdateManager manages automatic OTA updates with canary deployment and quiet window scheduling.
type AutoUpdateManager struct {
	mu                 sync.RWMutex
	server             *Server
	otaManager         *Manager
	settingsProvider   SettingsProvider
	qualityProvider    QualityProvider
	nodeProvider       NodeProvider
	notifier           EventNotifier
	timezone           *time.Location
	zoneVacancyChecker ZoneVacancyChecker

	// State
	running                      bool
	cancel                       context.CancelFunc
	wg                           sync.WaitGroup
	currentCanaryNode            string
	canaryPreviousVersion        string // Firmware version before canary update, for rollback
	baselineQuality              float64
	updateStartTime              time.Time
	updateState                  UpdateState
	pendingFirmware            *FirmwareMeta

	// Version selection tracking
	firmwareSelectionFromCache bool           // Whether latest firmware came from cached scan
	firmwareSnapshotTimestamp   time.Time     // When the firmware version list was last scanned
	selectedFirmwareVersion     string        // The firmware version selected for this update cycle
	canaryFirmwareVersionAfter  string        // Firmware version after canary update
}

// SettingsProvider provides access to system settings.
type SettingsProvider interface {
	GetSingle(key string) (interface{}, bool)
}

// QualityProvider provides system-wide detection quality metrics.
type QualityProvider interface {
	GetSystemQuality() float64
	GetLinkQuality(linkID string) float64
}

// NodeProvider provides node information for canary selection.
type NodeProvider interface {
	GetConnectedNodes() []string
	GetNodeHealthScore(mac string) float64
	GetNodeRole(mac string) string
	GetNodePosition(mac string) (x, y, z float64, err error)
	GetNodeFirmwareVersion(mac string) string
}

// EventNotifier publishes events to the timeline.
type EventNotifier interface {
	PublishOTAEvent(eventType, mac, message string, metadata map[string]interface{})
}

// ZoneVacancyChecker checks if zones are vacant for auto-update scheduling.
type ZoneVacancyChecker interface {
	AreAllZonesVacant() bool
}

// UpdateState represents the current state of an auto-update cycle.
type UpdateState string

const (
	StateIdle          UpdateState = "idle"
	StateChecking      UpdateState = "checking"
	StateWaitingWindow UpdateState = "waiting_window"
	StateCanaryDeploy  UpdateState = "canary_deploy"
	StateCanaryMonitor UpdateState = "canary_monitor"
	StateFleetDeploy   UpdateState = "fleet_deploy"
	StateRollback      UpdateState = "rollback"
	StateComplete      UpdateState = "complete"
	StateFailed        UpdateState = "failed"
)

// AutoUpdateConfig holds the configuration for auto-updates.
type AutoUpdateConfig struct {
	Enabled           bool    `json:"enabled"`
	QuietWindowStart  string  `json:"quiet_window_start"`  // HH:MM format
	QuietWindowEnd    string  `json:"quiet_window_end"`    // HH:MM format
	CanaryDurationMin int     `json:"canary_duration_min"` // Canary monitoring duration
	QualityThreshold  float64 `json:"quality_threshold"`   // Quality degradation threshold (0-1)
}

// formatDuration formats a duration into a human-readable string.
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}

// DefaultAutoUpdateConfig returns the default auto-update configuration.
func DefaultAutoUpdateConfig() AutoUpdateConfig {
	return AutoUpdateConfig{
		Enabled:           false,
		QuietWindowStart:  "02:00",
		QuietWindowEnd:    "05:00",
		CanaryDurationMin: 10,
		QualityThreshold:  0.05, // 5% degradation threshold
	}
}

// NewAutoUpdateManager creates a new auto-update manager.
func NewAutoUpdateManager(server *Server, otaMgr *Manager, timezone *time.Location) *AutoUpdateManager {
	return &AutoUpdateManager{
		server:      server,
		otaManager:  otaMgr,
		timezone:    timezone,
		updateState: StateIdle,
	}
}

// SetSettingsProvider sets the settings provider.
func (m *AutoUpdateManager) SetSettingsProvider(sp SettingsProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settingsProvider = sp
}

// SetQualityProvider sets the quality provider.
func (m *AutoUpdateManager) SetQualityProvider(qp QualityProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.qualityProvider = qp
}

// SetNodeProvider sets the node provider.
func (m *AutoUpdateManager) SetNodeProvider(np NodeProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nodeProvider = np
}

// SetEventNotifier sets the event notifier.
func (m *AutoUpdateManager) SetEventNotifier(en EventNotifier) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifier = en
}

// SetZoneVacancyChecker sets the zone vacancy checker.
func (m *AutoUpdateManager) SetZoneVacancyChecker(zvc ZoneVacancyChecker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.zoneVacancyChecker = zvc
}

// GetConfig returns the current auto-update configuration from settings.
func (m *AutoUpdateManager) GetConfig() AutoUpdateConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.settingsProvider == nil {
		return DefaultAutoUpdateConfig()
	}

	config := DefaultAutoUpdateConfig()

	// Read enabled setting
	if enabled, ok := m.settingsProvider.GetSingle("auto_update_enabled"); ok {
		if b, ok := enabled.(bool); ok {
			config.Enabled = b
		}
	}

	// Read quiet window settings
	if start, ok := m.settingsProvider.GetSingle("quiet_window_start"); ok {
		if s, ok := start.(string); ok {
			config.QuietWindowStart = s
		}
	}
	if end, ok := m.settingsProvider.GetSingle("quiet_window_end"); ok {
		if e, ok := end.(string); ok {
			config.QuietWindowEnd = e
		}
	}

	// Read canary duration
	if duration, ok := m.settingsProvider.GetSingle("canary_duration_min"); ok {
		if d, ok := duration.(float64); ok {
			config.CanaryDurationMin = int(d)
		}
	}

	// Read quality threshold
	if threshold, ok := m.settingsProvider.GetSingle("auto_update_quality_threshold"); ok {
		if t, ok := threshold.(float64); ok {
			config.QualityThreshold = t
		}
	}

	return config
}

// Start begins the auto-update manager background loop.
func (m *AutoUpdateManager) Start(ctx context.Context) {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	ctx, m.cancel = context.WithCancel(ctx)
	m.mu.Unlock()

	m.wg.Add(1)
	go m.run(ctx)

	log.Printf("[INFO] ota: auto-update manager started")
}

// Stop gracefully shuts down the auto-update manager.
func (m *AutoUpdateManager) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Unlock()

	m.wg.Wait()
	log.Printf("[INFO] ota: auto-update manager stopped")
}

// run is the main background loop.
func (m *AutoUpdateManager) run(ctx context.Context) {
	defer m.wg.Done()

	// Check immediately on startup
	m.checkForNewFirmware(ctx)

	// Then check every minute
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.checkForNewFirmware(ctx)
		}
	}
}

// checkForNewFirmware checks if new firmware is available and initiates update if conditions are met.
func (m *AutoUpdateManager) checkForNewFirmware(ctx context.Context) {
	// Get config before acquiring any lock
	config := m.GetConfig()

	if !config.Enabled {
		return
	}

	// Get latest firmware and scan timestamp
	latest := m.server.GetLatest()
	if latest == nil {
		log.Printf("[DEBUG] ota: auto-update check - no firmware available")
		return
	}

	// Track whether this was from cached scan or fresh GetLatest() call
	scanTimestamp := m.server.GetLastScanTimestamp()
	fromCache := !scanTimestamp.IsZero() && time.Since(scanTimestamp) < 1*time.Minute

	m.mu.Lock()
	defer m.mu.Unlock()

	// Store firmware selection metadata
	m.firmwareSelectionFromCache = fromCache
	m.firmwareSnapshotTimestamp = scanTimestamp
	m.selectedFirmwareVersion = latest.Version
	m.canaryFirmwareVersionAfter = latest.Version

	// Build comprehensive selection reason
	selectionReason := "latest_stable_fresh_scan"
	if fromCache {
		selectionReason = "latest_stable_cached_scan"
	}

	// Log firmware selection with comprehensive structured information
	log.Printf("[INFO] ota: AUTO-UPDATE firmware selection: version=%s filename=%s sha256=%s selection_reason=%s source=%s snapshot_timestamp=%s snapshot_age=%s",
		latest.Version,
		latest.Filename,
		latest.SHA256,
		selectionReason,
		map[bool]string{true: "cache", false: "fresh_getLatest"}[fromCache],
		scanTimestamp.Format(time.RFC3339),
		formatDuration(time.Since(scanTimestamp)))

	// Check if we're already in an update cycle
	if m.updateState != StateIdle && m.updateState != StateComplete && m.updateState != StateFailed {
		log.Printf("[DEBUG] ota: auto-update check - already in cycle (state=%s)", m.updateState)
		return
	}

	// Check if this is new firmware (different from current pending)
	if m.pendingFirmware != nil && m.pendingFirmware.Filename == latest.Filename {
		log.Printf("[DEBUG] ota: auto-update check - firmware already pending (%s)", latest.Filename)
		return
	}
	m.pendingFirmware = latest

	// Check if we're in quiet window
	if !m.isInQuietWindow(config) {
		log.Printf("[DEBUG] ota: auto-update check - not in quiet window (start=%s end=%s)",
			config.QuietWindowStart, config.QuietWindowEnd)
		return
	}

	// Check if zones are vacant (all zones empty for >10 minutes)
	if !m.zonesVacant(ctx) {
		log.Printf("[DEBUG] ota: zones not vacant, skipping auto-update")
		return
	}

	// All conditions met, start update cycle
	m.startUpdateCycle(ctx, latest)
}

// isInQuietWindow checks if current time is within the configured quiet window.
func (m *AutoUpdateManager) isInQuietWindow(config AutoUpdateConfig) bool {
	if config.QuietWindowStart == "" || config.QuietWindowEnd == "" {
		return true // No quiet window configured
	}

	now := time.Now().In(m.timezone)

	startTime, err := time.Parse("15:04", config.QuietWindowStart)
	if err != nil {
		log.Printf("[WARN] ota: invalid quiet_window_start: %v", err)
		return true
	}

	endTime, err := time.Parse("15:04", config.QuietWindowEnd)
	if err != nil {
		log.Printf("[WARN] ota: invalid quiet_window_end: %v", err)
		return true
	}

	currentTime := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), 0, 0, m.timezone)
	start := time.Date(now.Year(), now.Month(), now.Day(), startTime.Hour(), startTime.Minute(), 0, 0, m.timezone)
	end := time.Date(now.Year(), now.Month(), now.Day(), endTime.Hour(), endTime.Minute(), 0, 0, m.timezone)

	// Handle overnight windows (e.g., 22:00 to 06:00)
	if end.Before(start) {
		if currentTime.Before(start) {
			// Before start, check if it's after end from previous day
			end = end.Add(24 * time.Hour)
		} else {
			// After start, end is tomorrow
			end = end.Add(24 * time.Hour)
		}
	}

	return currentTime.After(start) && currentTime.Before(end)
}

// zonesVacant checks if all zones have been vacant for >10 minutes.
func (m *AutoUpdateManager) zonesVacant(ctx context.Context) bool {
	m.mu.RLock()
	zvc := m.zoneVacancyChecker
	m.mu.RUnlock()

	if zvc == nil {
		// No zone checker configured, assume vacant
		return true
	}

	return zvc.AreAllZonesVacant()
}

// startUpdateCycle begins the canary update cycle.
func (m *AutoUpdateManager) startUpdateCycle(ctx context.Context, firmware *FirmwareMeta) {
	m.mu.Lock()
	m.updateState = StateChecking
	m.updateStartTime = time.Now()
	m.currentCanaryNode = ""
	m.baselineQuality = 0
	m.mu.Unlock()

	// Increment Prometheus counter for auto-update trigger
	autoUpdateTriggerCounter.WithLabelValues("auto", "success").Inc()

	// Determine comprehensive selection reason
	selectionReason := "latest_stable_from_cache"
	if !m.firmwareSelectionFromCache {
		selectionReason = "latest_stable_from_fresh_getLatest"
	}

	// Explicit log line: AUTO-UPDATE triggered (distinguish from manual OTA)
	// Shows: which version, why selected, where from, how fresh the data is
	log.Printf("[INFO] ota: Auto-update triggered: trigger_type=auto version=%s filename=%s sha256=%s selection_reason=%s source=%s snapshot_timestamp=%s snapshot_age=%s",
		firmware.Version,
		firmware.Filename,
		firmware.SHA256,
		selectionReason,
		map[bool]string{true: "cache", false: "fresh_getLatest"}[m.firmwareSelectionFromCache],
		m.firmwareSnapshotTimestamp.Format(time.RFC3339),
		formatDuration(time.Since(m.firmwareSnapshotTimestamp)))

	log.Printf("[INFO] ota: AUTO-UPDATE cycle started: firmware_version=%s filename=%s sha256=%s selection_reason=%s source=%s snapshot_age=%s",
		firmware.Version,
		firmware.Filename,
		firmware.SHA256,
		selectionReason,
		map[bool]string{true: "cache", false: "fresh_getLatest"}[m.firmwareSelectionFromCache],
		formatDuration(time.Since(m.firmwareSnapshotTimestamp)))

	m.publishEvent("update_started", "", fmt.Sprintf("AUTO-UPDATE cycle started for firmware %s", firmware.Version), map[string]interface{}{
		"firmware_version":     firmware.Version,
		"filename":             firmware.Filename,
		"from_cache":           m.firmwareSelectionFromCache,
		"snapshot_timestamp":   m.firmwareSnapshotTimestamp.Format(time.RFC3339),
		"trigger_type":         "automatic",
	})

	// Select canary node and deploy
	canaryMAC := m.selectCanaryNode()
	if canaryMAC == "" {
		m.failUpdateCycle("no suitable canary node found")
		return
	}

	m.mu.Lock()
	m.currentCanaryNode = canaryMAC
	m.updateState = StateCanaryDeploy
	m.mu.Unlock()

	// Get baseline quality before canary update
	m.mu.Lock()
	if m.qualityProvider != nil {
		m.baselineQuality = m.qualityProvider.GetSystemQuality()
	}
	m.mu.Unlock()

	// Store the canary node's current firmware version for potential rollback
	m.mu.Lock()
	if m.nodeProvider != nil {
		m.canaryPreviousVersion = m.nodeProvider.GetNodeFirmwareVersion(canaryMAC)
	} else {
		m.canaryPreviousVersion = ""
	}
	m.mu.Unlock()

	log.Printf("[INFO] ota: AUTO-UPDATE canary deployment: node=%s version_before=%s version_after=%s baseline_quality=%.2f trigger_type=automatic_canary",
		canaryMAC, m.canaryPreviousVersion, firmware.Version, m.baselineQuality)

	autoUpdateTriggerCounter.WithLabelValues("auto", "success").Inc()

	m.publishEvent("canary_deploy", canaryMAC, fmt.Sprintf("AUTO-UPDATE: Deploying canary update to node %s", canaryMAC), map[string]interface{}{
		"firmware_version":         firmware.Version,
		"previous_firmware_version": m.canaryPreviousVersion,
		"baseline_quality":          m.baselineQuality,
		"version_before":            m.canaryPreviousVersion,
		"version_after":             firmware.Version,
		"trigger_type":              "automatic_canary",
	})

	// Trigger OTA on canary node
	if err := m.otaManager.SendOTA(canaryMAC); err != nil {
		m.failUpdateCycle(fmt.Sprintf("failed to send OTA to canary: %v", err))
		return
	}

	// Start monitoring canary
	m.wg.Add(1)
	go m.monitorCanary(ctx, firmware)
}

// selectCanaryNode selects the best node for canary deployment.
// Chooses the node with the lowest coverage impact (highest health score that isn't critical).
func (m *AutoUpdateManager) selectCanaryNode() string {
	m.mu.RLock()
	np := m.nodeProvider
	m.mu.Unlock()

	if np == nil {
		return ""
	}

	nodes := np.GetConnectedNodes()
	if len(nodes) == 0 {
		return ""
	}

	// Get health scores for all nodes
	type nodeScore struct {
		mac    string
		health float64
		role   string
	}

	scores := make([]nodeScore, 0, len(nodes))
	for _, mac := range nodes {
		health := np.GetNodeHealthScore(mac)
		role := np.GetNodeRole(mac)

		// Skip virtual nodes and APs
		if role == "ap" || role == "passive_ap" {
			continue
		}

		scores = append(scores, nodeScore{
			mac:    mac,
			health: health,
			role:   role,
		})
	}

	if len(scores) == 0 {
		return ""
	}

	// Sort by health score (descending) - choose the healthiest node as canary
	// This minimizes risk: if the update fails, we lose our best node temporarily
	for i := 0; i < len(scores)-1; i++ {
		for j := i + 1; j < len(scores); j++ {
			if scores[i].health < scores[j].health {
				scores[i], scores[j] = scores[j], scores[i]
			}
		}
	}

	// Return the healthiest node
	return scores[0].mac
}

// monitorCanary monitors the canary node during the canary duration.
func (m *AutoUpdateManager) monitorCanary(ctx context.Context, firmware *FirmwareMeta) {
	defer m.wg.Done()

	m.mu.Lock()
	config := m.GetConfig()
	canaryMAC := m.currentCanaryNode
	m.mu.Unlock()

	duration := time.Duration(config.CanaryDurationMin) * time.Minute
	deadline := time.Now().Add(duration)

	log.Printf("[INFO] ota: monitoring canary %s for %v minutes", canaryMAC, config.CanaryDurationMin)

	// Monitor loop
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if time.Now().After(deadline) {
				// Canary period complete, check quality
				m.evaluateCanary(ctx, firmware)
				return
			}

			// Check if canary node came back online
			progress := m.otaManager.GetProgress()
			if p, ok := progress[canaryMAC]; ok {
				if p.State == OTAVerified && p.ExpectedVersion == firmware.Version {
					// Node successfully updated and verified
					log.Printf("[INFO] ota: canary %s verified with version %s", canaryMAC, firmware.Version)
				} else if p.State == OTAFailed {
					m.failUpdateCycle(fmt.Sprintf("canary %s failed to update: %s", canaryMAC, p.Error))
					return
				} else if p.State == OTARollback {
					m.failUpdateCycle(fmt.Sprintf("canary %s rolled back after update", canaryMAC))
					return
				}
			}
		}
	}
}

// evaluateCanary evaluates the canary node's quality and decides whether to proceed.
func (m *AutoUpdateManager) evaluateCanary(ctx context.Context, firmware *FirmwareMeta) {
	m.mu.Lock()
	config := m.GetConfig()
	canaryMAC := m.currentCanaryNode
	baselineQuality := m.baselineQuality
	m.mu.Unlock()

	// Check current quality
	var currentQuality float64
	if m.qualityProvider != nil {
		currentQuality = m.qualityProvider.GetSystemQuality()
	}

	// Calculate quality change
	qualityDelta := currentQuality - baselineQuality
	qualityChanged := math.Abs(qualityDelta)

	m.publishEvent("canary_evaluated", canaryMAC, fmt.Sprintf("Canary evaluation: quality delta %.2f%%", qualityDelta*100), map[string]interface{}{
		"baseline_quality": baselineQuality,
		"current_quality":  currentQuality,
		"quality_delta":    qualityDelta,
	})

	// Decision threshold
	if qualityChanged > config.QualityThreshold {
		// Quality degraded beyond threshold, abort and trigger rollback
		m.mu.Lock()
		m.updateState = StateRollback
		previousVersion := m.canaryPreviousVersion
		m.mu.Unlock()

		m.publishEvent("canary_failed", canaryMAC, fmt.Sprintf("Canary quality degraded %.2f%%, aborting update", qualityDelta*100), map[string]interface{}{
			"threshold":        config.QualityThreshold,
			"quality_delta":    qualityDelta,
			"rollback_version": previousVersion,
		})

		log.Printf("[WARN] ota: canary quality degraded %.2f%% (threshold %.2f%%), aborting auto-update",
			qualityDelta*100, config.QualityThreshold*100)

		// Trigger rollback to previous firmware version
		if previousVersion != "" {
			if err := m.otaManager.SendOTAVersion(canaryMAC, previousVersion); err != nil {
				log.Printf("[ERROR] ota: failed to trigger rollback for canary %s to version %s: %v",
					canaryMAC, previousVersion, err)
				m.failUpdateCycle(fmt.Sprintf("canary quality degraded and rollback failed: %v", err))
				return
			}
			log.Printf("[INFO] ota: triggered rollback for canary %s to version %s", canaryMAC, previousVersion)
		} else {
			log.Printf("[WARN] ota: cannot rollback canary %s: previous firmware version unknown", canaryMAC)
		}

		m.failUpdateCycle(fmt.Sprintf("canary quality degraded: %.2f%%", qualityDelta*100))
		return
	}

	// Canary passed, proceed with fleet update
	m.mu.Lock()
	m.updateState = StateFleetDeploy
	m.canaryFirmwareVersionAfter = firmware.Version
	m.mu.Unlock()

	log.Printf("[INFO] ota: AUTO-UPDATE canary passed: node=%s version_before=%s version_after=%s quality_delta=%.2f%% threshold=%.2f%%",
		canaryMAC, m.canaryPreviousVersion, firmware.Version, qualityDelta*100, config.QualityThreshold*100)

	m.publishEvent("canary_passed", canaryMAC, "AUTO-UPDATE: Canary passed, proceeding with fleet update", map[string]interface{}{
		"quality_delta":    qualityDelta,
		"version_before":   m.canaryPreviousVersion,
		"version_after":    firmware.Version,
		"trigger_type":     "automatic_canary_success",
	})

	// Start fleet rollout
	m.wg.Add(1)
	go m.fleetRollout(ctx, firmware)
}

// fleetRollout performs a rolling update of all remaining nodes.
func (m *AutoUpdateManager) fleetRollout(ctx context.Context, firmware *FirmwareMeta) {
	m.mu.RLock()
	np := m.nodeProvider
	canaryMAC := m.currentCanaryNode
	m.mu.RUnlock()

	if np == nil {
		m.failUpdateCycle("node provider not available")
		return
	}

	nodes := np.GetConnectedNodes()
	if len(nodes) == 0 {
		m.failUpdateCycle("no connected nodes for fleet update")
		return
	}

	// Filter out the canary node (already updated)
	var remainingNodes []string
	for _, mac := range nodes {
		if mac != canaryMAC {
			remainingNodes = append(remainingNodes, mac)
		}
	}

	nodesUpdatedCount := len(remainingNodes)

	defer m.wg.Done()
	defer func() {
		m.mu.Lock()
		m.updateState = StateComplete
		m.mu.Unlock()

		autoUpdateTriggerCounter.WithLabelValues("auto", "success").Inc()

		log.Printf("[INFO] ota: AUTO-UPDATE fleet rollout complete: firmware_version=%s nodes_updated=%d",
			firmware.Version, nodesUpdatedCount)

		m.publishEvent("update_complete", "", fmt.Sprintf("AUTO-UPDATE complete for firmware %s", firmware.Version), map[string]interface{}{
			"firmware_version": firmware.Version,
			"trigger_type":     "automatic_fleet_complete",
		})
	}()

	if len(remainingNodes) == 0 {
		log.Printf("[INFO] ota: all nodes already updated")
		return
	}

	log.Printf("[INFO] ota: rolling out to %d remaining nodes", len(remainingNodes))

	// Rolling update with 30 second gap
	rollingGap := 30 * time.Second

	for i, mac := range remainingNodes {
		select {
		case <-ctx.Done():
			m.failUpdateCycle("context cancelled during fleet rollout")
			return
		default:
		}

		// Get current firmware version before update
		var versionBefore string
		if np != nil {
			versionBefore = np.GetNodeFirmwareVersion(mac)
		}

		m.publishEvent("node_update", mac, fmt.Sprintf("Updating node %s (%d/%d)", mac, i+1, len(remainingNodes)), map[string]interface{}{
			"version_before": versionBefore,
			"version_after":  firmware.Version,
		})

		log.Printf("[INFO] ota: AUTO-UPDATE node update: node=%s version_before=%s version_after=%s trigger_type=automatic_fleet",
			mac, versionBefore, firmware.Version)

		if err := m.otaManager.SendOTA(mac); err != nil {
			log.Printf("[WARN] ota: failed to update node %s: %v", mac, err)
			// Continue with next node
		}

		// Wait before next node (except for last)
		if i < len(remainingNodes)-1 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(rollingGap):
			}
		}
	}
}

// failUpdateCycle marks the current update cycle as failed.
func (m *AutoUpdateManager) failUpdateCycle(reason string) {
	m.mu.Lock()
	m.updateState = StateFailed
	m.mu.Unlock()

	// Increment Prometheus counter for auto-update failure
	autoUpdateTriggerCounter.WithLabelValues("auto", "failure").Inc()

	m.publishEvent("update_failed", m.currentCanaryNode, fmt.Sprintf("Auto-update failed: %s", reason), map[string]interface{}{
		"reason": reason,
	})

	log.Printf("[WARN] ota: auto-update failed: %s", reason)
}

// publishEvent publishes an OTA event to the timeline.
func (m *AutoUpdateManager) publishEvent(eventType, mac, message string, metadata map[string]interface{}) {
	m.mu.RLock()
	nt := m.notifier
	m.mu.RUnlock()

	if nt == nil {
		return
	}

	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadata["canary_node"] = m.currentCanaryNode
	metadata["update_state"] = string(m.updateState)

	nt.PublishOTAEvent(eventType, mac, message, metadata)
}

// GetState returns the current auto-update state.
func (m *AutoUpdateManager) GetState() UpdateState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.updateState
}

// GetCanaryNode returns the current canary node MAC.
func (m *AutoUpdateManager) GetCanaryNode() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentCanaryNode
}

// GetBaselineQuality returns the baseline quality recorded before canary deployment.
func (m *AutoUpdateManager) GetBaselineQuality() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.baselineQuality
}

// TriggerUpdate manually triggers an auto-update cycle for testing.
func (m *AutoUpdateManager) TriggerUpdate(ctx context.Context) error {
	m.mu.RLock()
	config := m.GetConfig()
	m.mu.RUnlock()

	if !config.Enabled {
		return fmt.Errorf("auto-update is disabled")
	}

	latest := m.server.GetLatest()
	if latest == nil {
		return fmt.Errorf("no firmware available")
	}

	m.startUpdateCycle(ctx, latest)
	return nil
}

// CancelUpdate cancels the current update cycle.
func (m *AutoUpdateManager) CancelUpdate() {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
	}
	m.updateState = StateIdle
	m.currentCanaryNode = ""
	m.pendingFirmware = nil
	m.mu.Unlock()

	m.publishEvent("update_cancelled", "", "Auto-update cycle cancelled", nil)
	log.Printf("[INFO] ota: auto-update cycle cancelled")
}

// OnFirmwareUploaded is called when new firmware is uploaded.
func (m *AutoUpdateManager) OnFirmwareUploaded(filename string) {
	log.Printf("[INFO] ota: new firmware uploaded: %s", filename)

	// Trigger immediate check
	m.checkForNewFirmware(context.Background())
}
