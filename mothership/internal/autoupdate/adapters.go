// Package autoupdate provides adapters for integrating the AutoUpdateManager with existing systems.
package autoupdate

import (
	"encoding/json"
	"log"
	"time"

	"github.com/spaxel/mothership/internal/dashboard"
	"github.com/spaxel/mothership/internal/eventbus"
	"github.com/spaxel/mothership/internal/fleet"
	"github.com/spaxel/mothership/internal/ota"
)

// qualityProviderAdapter adapts LinkWeatherDiagnostics to implement ota.QualityProvider.
type qualityProviderAdapter struct {
	diagnostics *fleet.LinkWeatherDiagnostics
}

// NewQualityProvider creates an ota.QualityProvider from LinkWeatherDiagnostics.
func NewQualityProvider(diagnostics *fleet.LinkWeatherDiagnostics) ota.QualityProvider {
	return &qualityProviderAdapter{diagnostics: diagnostics}
}

func (a *qualityProviderAdapter) GetSystemQuality() float64 {
	if a.diagnostics == nil {
		return 0.5 // Default mid-range quality
	}

	_, avgConfidence, _ := a.diagnostics.GetSystemWeatherSummary()
	return avgConfidence / 100.0 // Convert from percentage to 0-1 scale
}

func (a *qualityProviderAdapter) GetLinkQuality(linkID string) float64 {
	if a.diagnostics == nil {
		return 0.5
	}

	report := a.diagnostics.GetReport(linkID)
	if report == nil {
		return 0.5
	}

	return report.Confidence
}

// NodeConnectedGetter is the interface needed to get connected nodes.
// This is implemented by fleet.Manager.
type NodeConnectedGetter interface {
	GetConnectedMACs() []string
}

// nodeProviderAdapter adapts fleet.Registry and fleet.Manager to implement ota.NodeProvider.
type nodeProviderAdapter struct {
	registry *fleet.Registry
	weather  *fleet.LinkWeatherDiagnostics
	connGetter NodeConnectedGetter
}

// NewNodeProvider creates an ota.NodeProvider from fleet.Registry and fleet.Manager.
func NewNodeProvider(registry *fleet.Registry, weather *fleet.LinkWeatherDiagnostics) ota.NodeProvider {
	return &nodeProviderAdapter{
		registry: registry,
		weather:  weather,
	}
}

// NewNodeProviderWithConnected creates an ota.NodeProvider with a connected nodes getter.
func NewNodeProviderWithConnected(registry *fleet.Registry, weather *fleet.LinkWeatherDiagnostics, connGetter NodeConnectedGetter) ota.NodeProvider {
	return &nodeProviderAdapter{
		registry:  registry,
		weather:   weather,
		connGetter: connGetter,
	}
}

// SetConnectedGetter sets the source for connected node MACs.
// This should be the fleet.Manager which implements GetConnectedMACs().
func (p *nodeProviderAdapter) SetConnectedGetter(getter NodeConnectedGetter) {
	p.connGetter = getter
}

func (p *nodeProviderAdapter) GetConnectedNodes() []string {
	if p.connGetter != nil {
		return p.connGetter.GetConnectedMACs()
	}
	return nil
}

func (p *nodeProviderAdapter) GetNodeHealthScore(mac string) float64 {
	if p.weather == nil {
		return 0.5 // Default mid-range health
	}

	// Get all link IDs involving this node
	linkIDs := p.weather.GetAllLinkIDs()
	if len(linkIDs) == 0 {
		return 0.5
	}

	// Get reports for all links involving this node
	var totalScore float64
	var linkCount int

	for _, linkID := range linkIDs {
		if len(linkID) < 35 {
			continue
		}

		// Check if this link involves our node
		nodeAMAC := linkID[:17]
		nodeBMAC := linkID[18:]

		if nodeAMAC != mac && nodeBMAC != mac {
			continue
		}

		report := p.weather.GetReport(linkID)
		if report != nil {
			totalScore += report.Confidence
			linkCount++
		}
	}

	if linkCount == 0 {
		return 0.5
	}

	return totalScore / float64(linkCount)
}

func (p *nodeProviderAdapter) GetNodeRole(mac string) string {
	if p.registry == nil {
		return ""
	}

	node, err := p.registry.GetNode(mac)
	if err != nil {
		return ""
	}

	return node.Role
}

func (p *nodeProviderAdapter) GetNodePosition(mac string) (x, y, z float64, err error) {
	if p.registry == nil {
		return 0, 0, 0, &NodeNotFoundError{MAC: mac}
	}

	node, err := p.registry.GetNode(mac)
	if err != nil {
		return 0, 0, 0, err
	}

	return node.PosX, node.PosY, node.PosZ, nil
}

// NodeNotFoundError is returned when a node is not found.
type NodeNotFoundError struct {
	MAC string
}

func (e *NodeNotFoundError) Error() string {
	return "node not found: " + e.MAC
}

// eventNotifierAdapter adapts eventbus to implement ota.EventNotifier.
type eventNotifierAdapter struct{}

// NewEventNotifier creates an ota.EventNotifier using the eventbus.
func NewEventNotifier() ota.EventNotifier {
	return &eventNotifierAdapter{}
}

func (a *eventNotifierAdapter) PublishOTAEvent(eventType, mac, message string, metadata map[string]interface{}) {
	event := eventbus.Event{
		Type:        eventbus.TypeOTAUpdate,
		TimestampMs: timestampNowMs(),
		Severity:    eventbus.SeverityInfo,
		Detail: map[string]interface{}{
			"ota_event": eventType,
			"mac":       mac,
			"message":   message,
			"metadata":  metadata,
		},
	}

	eventbus.PublishDefault(event)
}

func (a *eventNotifierAdapter) PublishSystemEvent(message string) {
	event := eventbus.Event{
		Type:        eventbus.TypeSystem,
		TimestampMs: timestampNowMs(),
		Severity:    eventbus.SeverityInfo,
		Detail: map[string]interface{}{
			"message": message,
		},
	}

	eventbus.PublishDefault(event)
}

// timestampNowMs returns the current Unix timestamp in milliseconds.
func timestampNowMs() int64 {
	return timestampToMs(time.Now())
}

// timestampToMs converts a time.Time to Unix milliseconds.
func timestampToMs(t time.Time) int64 {
	return t.Unix()*1000 + int64(t.Nanosecond()/1e6)
}

// zoneVacancyChecker checks if all zones have been vacant for a minimum duration.
type zoneVacancyChecker struct {
	getAllZoneIDs     func() []string
	getZoneOccupancy  func(zoneID string) (count int, lastSeen time.Time, ok bool)
	minVacantDuration time.Duration
}

// NewZoneVacancyChecker creates a zone vacancy checker.
func NewZoneVacancyChecker(minVacantDuration time.Duration) *zoneVacancyChecker {
	return &zoneVacancyChecker{
		minVacantDuration: minVacantDuration,
	}
}

// SetAllZonesGetter sets the function to get all zone IDs.
func (z *zoneVacancyChecker) SetAllZonesGetter(fn func() []string) {
	z.getAllZoneIDs = fn
}

// SetZoneOccupancyGetter sets the function to get zone occupancy.
func (z *zoneVacancyChecker) SetZoneOccupancyGetter(fn func(zoneID string) (count int, lastSeen time.Time, ok bool)) {
	z.getZoneOccupancy = fn
}

// AreAllZonesVacant checks if all zones have been vacant for the minimum duration.
func (z *zoneVacancyChecker) AreAllZonesVacant() bool {
	if z.getZoneOccupancy == nil {
		// No occupancy data available, assume vacant
		return true
	}

	// Get all zones to check
	var zonesToCheck []string
	if z.getAllZoneIDs != nil {
		zonesToCheck = z.getAllZoneIDs()
	}

	// If no zones defined, consider vacant
	if len(zonesToCheck) == 0 {
		return true
	}

	now := time.Now()
	for _, zoneID := range zonesToCheck {
		count, lastSeen, ok := z.getZoneOccupancy(zoneID)
		if !ok {
			// Can't get occupancy data for this zone, fail conservatively
			log.Printf("[DEBUG] ota: zone %s occupancy data unavailable", zoneID)
			return false
		}

		// Check if zone has occupants
		if count > 0 {
			log.Printf("[DEBUG] ota: zone %s not vacant (count=%d)", zoneID, count)
			return false
		}

		// Check if zone has been vacant for long enough
		if !lastSeen.IsZero() && now.Sub(lastSeen) < z.minVacantDuration {
			log.Printf("[DEBUG] ota: zone %s not vacant long enough (vacant for %v, need %v)", zoneID, now.Sub(lastSeen), z.minVacantDuration)
			return false
		}
	}

	log.Printf("[DEBUG] ota: all zones vacant for %v+", z.minVacantDuration)
	return true
}

// LogZoneVacancy logs the current zone vacancy state for debugging.
func (z *zoneVacancyChecker) LogZoneVacancy() {
	if z.getZoneOccupancy == nil {
		log.Printf("[DEBUG] ota: zone vacancy check not configured")
		return
	}

	// Get all zones to check
	var zonesToCheck []string
	if z.getAllZoneIDs != nil {
		zonesToCheck = z.getAllZoneIDs()
	}

	if len(zonesToCheck) == 0 {
		log.Printf("[DEBUG] ota: no zones configured for vacancy check")
		return
	}

	now := time.Now()
	for _, zoneID := range zonesToCheck {
		count, lastSeen, ok := z.getZoneOccupancy(zoneID)
		if !ok {
			log.Printf("[DEBUG] ota: zone %s: data unavailable", zoneID)
			continue
		}

		vacantDuration := "unknown"
		if !lastSeen.IsZero() {
			vacantDuration = now.Sub(lastSeen).String()
		}

		log.Printf("[DEBUG] ota: zone %s: count=%d, vacant_for=%s", zoneID, count, vacantDuration)
	}
}

// autoUpdateDashboardBroadcaster adapts dashboard.Hub to implement ota.DashboardBroadcaster.
type autoUpdateDashboardBroadcaster struct {
	hub *dashboard.Hub
}

// NewDashboardBroadcaster creates a new dashboard broadcaster for OTA auto-update progress.
func NewDashboardBroadcaster(hub *dashboard.Hub) ota.DashboardBroadcaster {
	return &autoUpdateDashboardBroadcaster{hub: hub}
}

func (b *autoUpdateDashboardBroadcaster) BroadcastOTAProgress(mac, state string, progressPct uint8, expectedVersion, previousVersion, errorMsg string) {
	msg := map[string]interface{}{
		"type":             "ota_progress",
		"mac":              mac,
		"state":            state,
		"progress_pct":     progressPct,
		"expected_version": expectedVersion,
		"previous_version": previousVersion,
		"error":            errorMsg,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[ERROR] Failed to marshal OTA progress: %v", err)
		return
	}
	if b.hub != nil {
		b.hub.Broadcast(data)
	}
}
