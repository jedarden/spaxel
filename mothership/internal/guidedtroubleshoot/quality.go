// Package guidedtroubleshoot provides proactive contextual help and
// post-feedback explanations for Spaxel users.
package guidedtroubleshoot

import (
	"context"
	"log"
	"sync"
	"time"
)

// Qualifying settings keys that trigger repeated-edit hints
var QualifyingSettingsKeys = map[string]bool{
	"delta_rms_threshold":    true,
	"breathing_sensitivity":  true,
	"tau_s":                  true,
	"fresnel_decay":          true,
	"n_subcarriers":          true,
	"motion_threshold":       true,
}

// EditTracker tracks edits to settings keys for repeated-edit hints.
type EditTracker struct {
	mu    sync.RWMutex
	edits map[string]*editState // key -> edit state
}

// editState tracks the edit count and last edit time for a settings key.
type editState struct {
	count      int
	lastEdit   time.Time
	firstEdit  time.Time
	hintShown  bool
	hintReset  time.Time // When to allow showing hint again (24h cooldown)
}

// NewEditTracker creates a new edit tracker.
func NewEditTracker() *EditTracker {
	return &EditTracker{
		edits: make(map[string]*editState),
	}
}

// RecordEdit records an edit to a settings key.
// Returns (hintPending bool, hintReset bool).
// hintPending is true if the edit count has reached the threshold.
// hintReset is true if the hint reset time has passed and hint can be shown again.
func (t *EditTracker) RecordEdit(key string) (bool, bool) {
	if !QualifyingSettingsKeys[key] {
		return false, false
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	state, exists := t.edits[key]

	if !exists {
		state = &editState{
			firstEdit: now,
		}
		t.edits[key] = state
	}

	// Check if we're past the reset window (24 hours cooldown)
	if !state.hintReset.IsZero() && now.After(state.hintReset) {
		// Reset the counter after cooldown
		state.count = 0
		state.hintReset = time.Time{}
		state.hintShown = false
	}

	// Check if edits are within the 60-minute window
	windowStart := now.Add(-60 * time.Minute)
	if state.lastEdit.Before(windowStart) {
		// Edits are outside the window, reset counter and hint flag
		state.count = 1
		state.firstEdit = now
		state.hintShown = false
	} else {
		state.count++
	}

	state.lastEdit = now

	// Trigger hint at 3 edits within the window
	if state.count >= 3 && !state.hintShown {
		return true, false
	}

	return false, false
}

// MarkHintShown marks that a hint has been displayed for a key.
// Sets a 24-hour cooldown before the hint can be shown again.
func (t *EditTracker) MarkHintShown(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	state := t.edits[key]
	if state != nil {
		state.hintShown = true
		state.hintReset = time.Now().Add(24 * time.Hour)
	}
}

// GetEditCount returns the current edit count for a key.
func (t *EditTracker) GetEditCount(key string) int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	state := t.edits[key]
	if state == nil {
		return 0
	}
	return state.count
}

// Reset resets all edit tracking (used for testing).
func (t *EditTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.edits = make(map[string]*editState)
}

// ZoneQualityTracker tracks detection quality per zone.
type ZoneQualityTracker struct {
	mu     sync.RWMutex
	zones  map[int]*zoneQualityState // zone ID -> quality state
	getAll func() ([]ZoneInfo, error)
}

// ZoneInfo represents information about a zone.
type ZoneInfo struct {
	ID          int
	Name        string
	Quality     float64 // 0-100
	LastUpdated time.Time
}

// zoneQualityState tracks the quality state for a single zone.
type zoneQualityState struct {
	zoneID           int
	quality          float64
	firstPoorTime    time.Time // When quality first dropped below 60%
	lastPoorTime     time.Time
	bannerShown      bool
	resolvedCount    int
	hysteresis       float64 // For quality improvements
}

const (
	QualityThreshold     = 60.0 // Quality below this triggers issues
	QualityRecovery     = 70.0 // Quality above this marks recovery
	PoorQualityDuration = 24 * time.Hour
)

// NewZoneQualityTracker creates a new zone quality tracker.
func NewZoneQualityTracker(getAll func() ([]ZoneInfo, error)) *ZoneQualityTracker {
	return &ZoneQualityTracker{
		zones:  make(map[int]*zoneQualityState),
		getAll: getAll,
	}
}

// UpdateQuality updates the quality for a zone.
// Returns (shouldShowBanner bool, issueResolved bool).
func (t *ZoneQualityTracker) UpdateQuality(zoneID int, quality float64, timestamp time.Time) (bool, bool) {
	if quality < 0 || quality > 100 {
		return false, false
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	state := t.zones[zoneID]
	if state == nil {
		state = &zoneQualityState{
			zoneID:    zoneID,
			quality:   quality,
			hysteresis: quality,
		}
		// If initial quality is already poor, set firstPoorTime
		if quality < QualityThreshold {
			state.firstPoorTime = timestamp
			state.lastPoorTime = timestamp
		}
		t.zones[zoneID] = state
		return false, false
	}

	// Check for quality degradation
	if quality < QualityThreshold && state.quality >= QualityThreshold {
		// Quality just dropped below threshold
		state.firstPoorTime = timestamp
		state.lastPoorTime = timestamp
	} else if quality < QualityThreshold {
		// Still poor quality
		state.lastPoorTime = timestamp
	}

	// Check for recovery (with hysteresis to prevent flapping)
	if quality >= QualityRecovery && state.quality < QualityRecovery {
		state.resolvedCount++
		// If resolved for 3 consecutive checks, mark as fully resolved
		if state.resolvedCount >= 3 {
			state.bannerShown = false
			state.resolvedCount = 0
			state.firstPoorTime = time.Time{}
			return false, true // Issue resolved
		}
	} else {
		state.resolvedCount = 0
	}

	state.quality = quality
	state.hysteresis = quality

	// Check if we should show banner (poor quality for >24h and not yet shown)
	if quality < QualityThreshold &&
		!state.firstPoorTime.IsZero() &&
		timestamp.Sub(state.firstPoorTime) > PoorQualityDuration &&
		!state.bannerShown {
		return true, false
	}

	return false, false
}

// MarkBannerShown marks that a banner has been shown for a zone.
func (t *ZoneQualityTracker) MarkBannerShown(zoneID int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	state := t.zones[zoneID]
	if state != nil {
		state.bannerShown = true
	}
}

// GetZonesWithPoorQuality returns zones with quality < 60% for >24 hours.
func (t *ZoneQualityTracker) GetZonesWithPoorQuality() []int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var zones []int
	now := time.Now()

	for _, state := range t.zones {
		if state.quality < QualityThreshold &&
			!state.firstPoorTime.IsZero() &&
			now.Sub(state.firstPoorTime) > PoorQualityDuration {
			zones = append(zones, state.zoneID)
		}
	}

	return zones
}

// Reset clears all zone quality tracking (used for testing).
func (t *ZoneQualityTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.zones = make(map[int]*zoneQualityState)
}

// Manager coordinates all guided troubleshooting features.
type Manager struct {
	editTracker        *EditTracker
	qualityTracker     *ZoneQualityTracker
	discoveryTracker   *DiscoveryTracker
	fleetNotifier      *FleetNotifier
	mu                 sync.RWMutex
	running            bool
	ctx                context.Context
	cancel             context.CancelFunc
	checkInterval      time.Duration
	onQualityIssue     func(zoneID int, quality float64)
	onNodeOffline      func(mac string, offlineDuration time.Duration)
	onCalibrationComplete func(zoneID int, qualityBefore, qualityAfter float64)
}

// ManagerConfig holds configuration for the guided troubleshooting manager.
type ManagerConfig struct {
	CheckInterval       time.Duration // How often to check quality issues
	GetAllZones         func() ([]ZoneInfo, error)
	GetNodeLastSeen     func(mac string) time.Time
}

// NewManager creates a new guided troubleshooting manager.
func NewManager(cfg ManagerConfig) *Manager {
	if cfg.CheckInterval == 0 {
		cfg.CheckInterval = 5 * time.Minute
	}

	return &Manager{
		editTracker:      NewEditTracker(),
		qualityTracker:   NewZoneQualityTracker(cfg.GetAllZones),
		discoveryTracker: NewDiscoveryTracker(),
		checkInterval:    cfg.CheckInterval,
	}
}

// Run starts the background check loop.
func (m *Manager) Run(ctx context.Context) {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.ctx, m.cancel = context.WithCancel(ctx)
	m.mu.Unlock()

	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()

	log.Printf("[INFO] guidedtroubleshoot: manager started (interval: %v)", m.checkInterval)

	// Initial check
	m.checkQuality()
	m.checkOfflineNodes()

	for {
		select {
		case <-m.ctx.Done():
			log.Printf("[INFO] guidedtroubleshoot: manager stopped")
			return
		case <-ticker.C:
			m.checkQuality()
			m.checkOfflineNodes()
		}
	}
}

// Stop stops the background check loop.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cancel != nil {
		m.cancel()
	}
	m.running = false
}

// checkQuality checks zone quality and triggers callbacks.
func (m *Manager) checkQuality() {
	m.mu.RLock()
	getAll := m.qualityTracker.getAll
	m.mu.RUnlock()

	if getAll == nil {
		return
	}

	zones, err := getAll()
	if err != nil {
		log.Printf("[WARN] guidedtroubleshoot: failed to get zones: %v", err)
		return
	}

	now := time.Now()
	for _, zone := range zones {
		shouldShow, resolved := m.qualityTracker.UpdateQuality(zone.ID, zone.Quality, now)

		if shouldShow && m.onQualityIssue != nil {
			m.onQualityIssue(zone.ID, zone.Quality)
		}

		if resolved && m.onQualityIssue != nil {
			// Could trigger a "resolved" notification
			log.Printf("[INFO] guidedtroubleshoot: zone %d quality recovered to %.1f%%", zone.ID, zone.Quality)
		}
	}
}

// checkOfflineNodes checks all tracked offline nodes and triggers callbacks.
func (m *Manager) checkOfflineNodes() {
	m.mu.RLock()
	notifier := m.fleetNotifier
	m.mu.RUnlock()

	if notifier != nil {
		notifier.CheckOfflineNodes()
	}
}

// SetFleetNotifier sets the fleet notifier for tracking node offline events.
func (m *Manager) SetFleetNotifier(notifier *FleetNotifier) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fleetNotifier = notifier
}

// SetOnQualityIssue sets the callback for quality issues.
func (m *Manager) SetOnQualityIssue(fn func(zoneID int, quality float64)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onQualityIssue = fn
}

// SetOnNodeOffline sets the callback for node offline events.
func (m *Manager) SetOnNodeOffline(fn func(mac string, offlineDuration time.Duration)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onNodeOffline = fn
}

// SetOnCalibrationComplete sets the callback for calibration completion.
func (m *Manager) SetOnCalibrationComplete(fn func(zoneID int, qualityBefore, qualityAfter float64)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onCalibrationComplete = fn
}

// RecordSettingsEdit records an edit to a settings key.
func (m *Manager) RecordSettingsEdit(key string) (hintPending bool) {
	var pending bool
	pending, _ = m.editTracker.RecordEdit(key)
	return pending
}

// MarkSettingsHintShown marks that a settings hint has been displayed.
func (m *Manager) MarkSettingsHintShown(key string) {
	m.editTracker.MarkHintShown(key)
}

// GetSettingsEditCount returns the edit count for a settings key.
func (m *Manager) GetSettingsEditCount(key string) int {
	return m.editTracker.GetEditCount(key)
}

// UpdateZoneQuality updates the quality for a zone.
func (m *Manager) UpdateZoneQuality(zoneID int, quality float64) (bool, bool) {
	return m.qualityTracker.UpdateQuality(zoneID, quality, time.Now())
}

// MarkQualityBannerShown marks that a quality banner has been shown.
func (m *Manager) MarkQualityBannerShown(zoneID int) {
	m.qualityTracker.MarkBannerShown(zoneID)
}

// GetZonesWithPoorQuality returns zones with quality issues.
func (m *Manager) GetZonesWithPoorQuality() []int {
	return m.qualityTracker.GetZonesWithPoorQuality()
}

// TriggerCalibrationComplete triggers the calibration complete callback.
func (m *Manager) TriggerCalibrationComplete(zoneID int, qualityBefore, qualityAfter float64) {
	m.mu.RLock()
	fn := m.onCalibrationComplete
	m.mu.RUnlock()

	if fn != nil {
		fn(zoneID, qualityBefore, qualityAfter)
	}
}

// TriggerNodeOffline triggers the node offline callback.
func (m *Manager) TriggerNodeOffline(mac string, offlineDuration time.Duration) {
	m.mu.RLock()
	fn := m.onNodeOffline
	m.mu.RUnlock()

	if fn != nil {
		fn(mac, offlineDuration)
	}
}

// IsRunning returns whether the manager is running.
func (m *Manager) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// Discovery methods

// ShouldShowTooltip returns true if the tooltip for this feature should be shown.
func (m *Manager) ShouldShowTooltip(featureID string) bool {
	return m.discoveryTracker.ShouldShowTooltip(featureID)
}

// MarkTooltipShown marks that a tooltip has been shown for a feature.
func (m *Manager) MarkTooltipShown(featureID string) {
	m.discoveryTracker.MarkTooltipShown(featureID)
}

// GetTooltip returns the tooltip content for a feature, if available.
func (m *Manager) GetTooltip(featureID string) (Tooltip, bool) {
	return m.discoveryTracker.GetTooltip(featureID)
}

// IsFeatureDiscovered returns true if the feature has been discovered (tooltip shown).
func (m *Manager) IsFeatureDiscovered(featureID string) bool {
	return m.discoveryTracker.IsFeatureDiscovered(featureID)
}
