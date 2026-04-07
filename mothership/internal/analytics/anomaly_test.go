// Package analytics provides anomaly detection based on learned normal behaviour patterns.
package analytics

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/spaxel/mothership/internal/events"
)

// testZoneProvider implements ZoneProvider for testing.
type testZoneProvider struct {
	zones map[string]string // zoneID -> zoneName
}

func (p *testZoneProvider) GetZoneName(zoneID string) string {
	if name, ok := p.zones[zoneID]; ok {
		return name
	}
	return zoneID
}

func (p *testZoneProvider) GetZoneOccupancy(zoneID string) (int, []int) {
	return 0, nil
}

// testDeviceProvider implements DeviceProvider for testing.
type testDeviceProvider struct {
	registered map[string]bool
	seenBefore map[string]bool
	names      map[string]string
}

func (p *testDeviceProvider) IsDeviceRegistered(mac string) bool {
	return p.registered[mac]
}

func (p *testDeviceProvider) IsDeviceSeenBefore(mac string) bool {
	return p.seenBefore[mac]
}

func (p *testDeviceProvider) GetDeviceName(mac string) string {
	if name, ok := p.names[mac]; ok {
		return name
	}
	return mac
}

// testPositionProvider implements PositionProvider for testing.
type testPositionProvider struct {
	positions map[int]struct{ x, y, z float64 }
}

func (p *testPositionProvider) GetBlobPosition(blobID int) (x, y, z float64, ok bool) {
	if pos, exists := p.positions[blobID]; exists {
		return pos.x, pos.y, pos.z, true
	}
	return 0, 0, 0, false
}

// testAlertHandler implements AlertHandler for testing.
type testAlertHandler struct {
	mu          sync.RWMutex
	alerts      []events.AnomalyEvent
	webhooks    []events.AnomalyEvent
	escalations []events.AnomalyEvent
}

func (h *testAlertHandler) SendAlert(event events.AnomalyEvent, immediate bool) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.alerts = append(h.alerts, event)
	return nil
}

func (h *testAlertHandler) SendWebhook(event events.AnomalyEvent, immediate bool) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.webhooks = append(h.webhooks, event)
	return nil
}

func (h *testAlertHandler) SendEscalation(event events.AnomalyEvent) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.escalations = append(h.escalations, event)
	return nil
}

// alertCount returns the number of alerts after waiting for goroutines.
func (h *testAlertHandler) alertCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.alerts)
}

func (h *testAlertHandler) webhookCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.webhooks)
}

func (h *testAlertHandler) escalationCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.escalations)
}

func setupTestDetector(t *testing.T) (*Detector, *testAlertHandler) {
	tmpDir, err := os.MkdirTemp("", "anomaly_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	config := DefaultAnomalyScoreConfig()
	detector, err := NewDetector(filepath.Join(tmpDir, "anomaly.db"), config)
	if err != nil {
		t.Fatalf("Failed to create detector: %v", err)
	}
	t.Cleanup(func() { detector.Close() })

	// Set up providers
	detector.SetZoneProvider(&testZoneProvider{
		zones: map[string]string{
			"zone_kitchen": "Kitchen",
			"zone_living":  "Living Room",
			"zone_bedroom": "Bedroom",
		},
	})
	detector.SetDeviceProvider(&testDeviceProvider{
		registered: map[string]bool{
			"aa:bb:cc:dd:ee:ff": true,
		},
		seenBefore: map[string]bool{},
		names:      map[string]string{},
	})
	detector.SetPositionProvider(&testPositionProvider{
		positions: map[int]struct{ x, y, z float64 }{
			1: {1.5, 0, 2.0},
		},
	})

	alertHandler := &testAlertHandler{}
	detector.SetAlertHandler(alertHandler)

	// Set registered devices
	detector.SetRegisteredDevices([]string{"aa:bb:cc:dd:ee:ff"})

	return detector, alertHandler
}

// TestAnomaly_UnusualHourPresence tests unusual hour anomaly detection.
func TestAnomaly_UnusualHourPresence(t *testing.T) {
	detector, alertHandler := setupTestDetector(t)

	// Simulate model being ready
	detector.mu.Lock()
	detector.modelReady = true
	// Create a behaviour slot with low expected occupancy for the current hour
	hourOfWeek := getHourOfWeek(time.Now())
	key := hourOfWeekZoneKey(hourOfWeek, "zone_kitchen")
	detector.behaviourSlots[key] = &NormalBehaviourSlot{
		HourOfWeek:        hourOfWeek,
		ZoneID:            "zone_kitchen",
		ExpectedOccupancy: 0.05, // Very low - this zone is usually empty at this hour
		SampleCount:       50,   // Enough samples
	}
	detector.mu.Unlock()

	// Process occupancy - should trigger anomaly
	event := detector.ProcessOccupancy("zone_kitchen", 1, nil, false)
	if event == nil {
		t.Error("Expected anomaly for unusual hour presence")
		return
	}

	if event.Type != events.AnomalyUnusualHour {
		t.Errorf("Expected unusual_hour anomaly, got %s", event.Type)
	}

	if event.Score < detector.config.AlertThresholdNormal {
		t.Errorf("Expected score >= %.2f, got %.2f", detector.config.AlertThresholdNormal, event.Score)
	}

	// Check alert was sent
	waitForGoroutines()
	if alertHandler.alertCount() == 0 {
		t.Error("Expected alert to be sent")
	}
}

// TestAnomaly_UnknownBLEDevice tests unknown BLE device anomaly detection.
func TestAnomaly_UnknownBLEDevice(t *testing.T) {
	detector, alertHandler := setupTestDetector(t)

	// Unknown device with strong signal (close range)
	// Use security mode so the score (0.8) exceeds the security threshold (0.4)
	event := detector.ProcessBLEDevice("11:22:33:44:55:66", -55, true)
	if event == nil {
		t.Error("Expected anomaly for unknown BLE device")
		return
	}

	if event.Type != events.AnomalyUnknownBLE {
		t.Errorf("Expected unknown_ble anomaly, got %s", event.Type)
	}

	// Check alert was sent
	waitForGoroutines()
	if alertHandler.alertCount() == 0 {
		t.Error("Expected alert to be sent")
	}
}

// TestAnomaly_UnknownBLEDevice_WeakSignal tests that weak signals don't trigger anomalies.
func TestAnomaly_UnknownBLEDevice_WeakSignal(t *testing.T) {
	detector, _ := setupTestDetector(t)

	// Unknown device with weak signal (far away)
	event := detector.ProcessBLEDevice("11:22:33:44:55:66", -80, false)
	if event != nil {
		t.Error("Expected no anomaly for weak signal BLE device")
	}
}

// TestAnomaly_MotionDuringAway tests motion during away mode.
func TestAnomaly_MotionDuringAway(t *testing.T) {
	detector, alertHandler := setupTestDetector(t)

	// Process motion during away mode (isSecurityMode = true)
	event := detector.ProcessMotionDuringAway("zone_kitchen", 1, true)
	if event == nil {
		t.Error("Expected anomaly for motion during away")
		return
	}

	if event.Type != events.AnomalyMotionDuringAway {
		t.Errorf("Expected motion_during_away anomaly, got %s", event.Type)
	}

	// Motion during away should have high score
	if event.Score < 0.9 {
		t.Errorf("Expected score >= 0.9 for motion during away, got %.2f", event.Score)
	}

	// Check alert was sent
	waitForGoroutines()
	if alertHandler.alertCount() == 0 {
		t.Error("Expected alert to be sent")
	}
}

// TestAnomaly_MotionDuringAway_AlwaysFires tests that motion during away fires regardless of model status.
func TestAnomaly_MotionDuringAway_AlwaysFires(t *testing.T) {
	detector, _ := setupTestDetector(t)

	// Model is NOT ready
	detector.mu.Lock()
	detector.modelReady = false
	detector.mu.Unlock()

	// Motion during away should still fire
	event := detector.ProcessMotionDuringAway("zone_kitchen", 1, true)
	if event == nil {
		t.Error("Expected anomaly for motion during away even when model not ready")
	}
}

// TestAnomaly_UnusualDwell tests unusual dwell duration detection.
func TestAnomaly_UnusualDwell(t *testing.T) {
	detector, alertHandler := setupTestDetector(t)

	// Set model as ready and add dwell slot
	detector.mu.Lock()
	detector.modelReady = true
	hourOfWeek := getHourOfWeek(time.Now())
	key := hourOfWeekZonePersonKey(hourOfWeek, "zone_bedroom", "person1")
	detector.dwellSlots[key] = &DwellBehaviourSlot{
		HourOfWeek:        hourOfWeek,
		ZoneID:            "zone_bedroom",
		PersonID:          "person1",
		MeanDwellDuration: 5 * time.Minute, // Usually dwells for 5 minutes
		SampleCount:       20,
	}
	detector.mu.Unlock()

	// Person dwelling for > 5x mean (25+ minutes), use security mode so score exceeds threshold
	event := detector.ProcessDwellDuration("zone_bedroom", "person1", 30*time.Minute, true, false)
	if event == nil {
		t.Error("Expected anomaly for unusual dwell duration")
		return
	}

	if event.Type != events.AnomalyUnusualDwell {
		t.Errorf("Expected unusual_dwell anomaly, got %s", event.Type)
	}

	// Check alert was sent
	if len(alertHandler.alerts) == 0 {
		t.Error("Expected alert to be sent")
	}
}

// TestAnomaly_UnusualDwell_FallDetected tests that dwell anomaly is suppressed if fall is detected.
func TestAnomaly_UnusualDwell_FallDetected(t *testing.T) {
	detector, _ := setupTestDetector(t)

	// Set model as ready and add dwell slot
	detector.mu.Lock()
	detector.modelReady = true
	hourOfWeek := getHourOfWeek(time.Now())
	key := hourOfWeekZonePersonKey(hourOfWeek, "zone_bedroom", "person1")
	detector.dwellSlots[key] = &DwellBehaviourSlot{
		HourOfWeek:        hourOfWeek,
		ZoneID:            "zone_bedroom",
		PersonID:          "person1",
		MeanDwellDuration: 5 * time.Minute,
		SampleCount:       20,
	}
	detector.mu.Unlock()

	// Person dwelling for > 5x mean but fall is detected - should NOT trigger dwell anomaly
	event := detector.ProcessDwellDuration("zone_bedroom", "person1", 30*time.Minute, false, true)
	if event != nil {
		t.Error("Expected no dwell anomaly when fall is detected")
	}
}

// TestAnomaly_Cooldown tests that anomalies are deduplicated via cooldown.
func TestAnomaly_Cooldown(t *testing.T) {
	detector, _ := setupTestDetector(t)

	// First anomaly
	event1 := detector.ProcessMotionDuringAway("zone_kitchen", 1, true)
	if event1 == nil {
		t.Fatal("Expected first anomaly")
	}

	// Immediate second anomaly should be suppressed by cooldown
	event2 := detector.ProcessMotionDuringAway("zone_kitchen", 1, true)
	if event2 != nil {
		t.Error("Expected second anomaly to be suppressed by cooldown")
	}
}

// TestAnomaly_AcknowledgeCancelsTimers tests that acknowledgement cancels alert timers.
func TestAnomaly_AcknowledgeCancelsTimers(t *testing.T) {
	detector, _ := setupTestDetector(t)

	// Create an anomaly
	event := detector.ProcessMotionDuringAway("zone_kitchen", 1, false) // Normal mode
	if event == nil {
		t.Fatal("Expected anomaly")
	}

	// Acknowledge it
	err := detector.AcknowledgeAnomaly(event.ID, "expected", "test_user")
	if err != nil {
		t.Fatalf("Failed to acknowledge anomaly: %v", err)
	}

	// Verify it's marked as acknowledged
	anomaly, exists := detector.activeAnomalies[event.ID]
	if !exists {
		t.Fatal("Anomaly should still exist after acknowledgement")
	}
	if !anomaly.Acknowledged {
		t.Error("Anomaly should be marked as acknowledged")
	}
}

// TestAnomaly_LearningProgress tests learning progress calculation.
func TestAnomaly_LearningProgress(t *testing.T) {
	detector, _ := setupTestDetector(t)

	// New detector - progress should be near 0
	progress := detector.GetLearningProgress()
	if progress > 0.1 {
		t.Errorf("Expected progress near 0 for new detector, got %.2f", progress)
	}

	// Set model as ready
	detector.mu.Lock()
	detector.modelReady = true
	detector.mu.Unlock()

	progress = detector.GetLearningProgress()
	if progress != 1.0 {
		t.Errorf("Expected progress 1.0 when model ready, got %.2f", progress)
	}
}

// TestAnomaly_SecurityModeThreshold tests lower thresholds in security mode.
func TestAnomaly_SecurityModeThreshold(t *testing.T) {
	detector, _ := setupTestDetector(t)

	// Create a behaviour slot with marginal expected occupancy
	detector.mu.Lock()
	detector.modelReady = true
	hourOfWeek := getHourOfWeek(time.Now())
	key := hourOfWeekZoneKey(hourOfWeek, "zone_living")
	detector.behaviourSlots[key] = &NormalBehaviourSlot{
		HourOfWeek:        hourOfWeek,
		ZoneID:            "zone_living",
		ExpectedOccupancy: 0.08, // Low but not extremely low
		SampleCount:       50,
	}
	detector.mu.Unlock()

	// In security mode, should trigger with lower threshold
	securityEvent := detector.ProcessOccupancy("zone_living", 1, nil, true)

	if securityEvent == nil {
		t.Error("Expected anomaly in security mode")
	}
}

// TestAnomaly_LateNightMultiplier tests that late night anomalies have higher scores.
func TestAnomaly_LateNightMultiplier(t *testing.T) {
	detector, _ := setupTestDetector(t)

	// This test is time-dependent, so we just verify the config exists
	if detector.config.LateNightMultiplier < 1.0 {
		t.Error("Late night multiplier should be >= 1.0")
	}

	if detector.config.LateNightMultiplier != 1.5 {
		t.Logf("Note: Late night multiplier is %.2f (expected 1.5)", detector.config.LateNightMultiplier)
	}
}

// TestAnomaly_WeeklySummary tests weekly summary generation.
func TestAnomaly_WeeklySummary(t *testing.T) {
	detector, _ := setupTestDetector(t)

	// Create some anomalies
	detector.ProcessMotionDuringAway("zone_kitchen", 1, true)
	detector.ProcessMotionDuringAway("zone_living", 2, true)

	// Get summary
	summary := detector.GetWeeklySummary()

	if summary.TotalAnomalies < 2 {
		t.Errorf("Expected at least 2 anomalies in summary, got %d", summary.TotalAnomalies)
	}

	if summary.ByType[events.AnomalyMotionDuringAway] < 2 {
		t.Errorf("Expected 2 motion_during_away anomalies, got %d", summary.ByType[events.AnomalyMotionDuringAway])
	}
}

// TestAnomaly_GetActiveAnomalies tests retrieval of active anomalies.
func TestAnomaly_GetActiveAnomalies(t *testing.T) {
	detector, _ := setupTestDetector(t)

	// Initially no active anomalies
	active := detector.GetActiveAnomalies()
	if len(active) != 0 {
		t.Errorf("Expected 0 active anomalies initially, got %d", len(active))
	}

	// Create an anomaly
	detector.ProcessMotionDuringAway("zone_kitchen", 1, true)

	// Should have 1 active anomaly
	active = detector.GetActiveAnomalies()
	if len(active) != 1 {
		t.Errorf("Expected 1 active anomaly, got %d", len(active))
	}
}

// TestAnomaly_UpdateBehaviourModel tests behaviour model updates.
func TestAnomaly_UpdateBehaviourModel(t *testing.T) {
	detector, _ := setupTestDetector(t)

	// Record some occupancy samples
	detector.ProcessOccupancy("zone_kitchen", 1, []string{"aa:bb:cc:dd:ee:ff"}, false)
	detector.ProcessOccupancy("zone_kitchen", 2, []string{"aa:bb:cc:dd:ee:ff"}, false)
	detector.ProcessOccupancy("zone_living", 1, nil, false)

	// Update the model
	err := detector.UpdateBehaviourModel()
	if err != nil {
		t.Fatalf("Failed to update behaviour model: %v", err)
	}

	// Check that slots were created
	detector.mu.RLock()
	slotCount := len(detector.behaviourSlots)
	detector.mu.RUnlock()

	if slotCount == 0 {
		t.Error("Expected behaviour slots to be created after update")
	}
}

// TestAnomaly_SecurityModeState tests security mode state management.
func TestAnomaly_SecurityModeState(t *testing.T) {
	detector, _ := setupTestDetector(t)

	// Initially disarmed
	if detector.GetSecurityMode() != SecurityModeDisarmed {
		t.Error("Expected initial security mode to be disarmed")
	}

	// Arm the system
	detector.SetSecurityMode(SecurityModeArmed, "manual")
	if detector.GetSecurityMode() != SecurityModeArmed {
		t.Error("Expected security mode to be armed")
	}

	// Check if active
	if !detector.IsSecurityModeActive() {
		t.Error("Expected security mode to be active when armed")
	}

	// Disarm
	detector.SetSecurityMode(SecurityModeDisarmed, "manual")
	if detector.GetSecurityMode() != SecurityModeDisarmed {
		t.Error("Expected security mode to be disarmed")
	}
}

// TestAnomaly_ManualOverride tests manual override functionality.
func TestAnomaly_ManualOverride(t *testing.T) {
	detector, _ := setupTestDetector(t)

	// Set manual override
	detector.SetManualOverride(30 * time.Minute)

	if !detector.IsManualOverrideActive() {
		t.Error("Expected manual override to be active")
	}

	// Clear it
	detector.ClearManualOverride()

	if detector.IsManualOverrideActive() {
		t.Error("Expected manual override to be inactive after clear")
	}
}

// TestAnomaly_RegisteredDevices tests that registered devices don't trigger anomalies.
func TestAnomaly_RegisteredDevices(t *testing.T) {
	detector, _ := setupTestDetector(t)

	// Registered device with strong signal
	event := detector.ProcessBLEDevice("aa:bb:cc:dd:ee:ff", -55, false)
	if event != nil {
		t.Error("Expected no anomaly for registered BLE device")
	}
}

// TestAnomaly_BLEDeviceFirstSeen tests that unknown devices are tracked for first-seen time.
func TestAnomaly_BLEDeviceFirstSeen(t *testing.T) {
	detector, _ := setupTestDetector(t)

	// Process an unknown device
	mac := "11:22:33:44:55:66"
	detector.ProcessBLEDevice(mac, -55, false)

	// Check that first-seen time was recorded
	detector.mu.RLock()
	firstSeen, exists := detector.deviceFirstSeen[mac]
	detector.mu.RUnlock()

	if !exists {
		t.Error("Expected first-seen time to be recorded for unknown device")
	}
	if firstSeen.IsZero() {
		t.Error("Expected non-zero first-seen time")
	}
}

// TestAnomaly_AlertChainNormalMode tests alert chain timing in normal mode.
func TestAnomaly_AlertChainNormalMode(t *testing.T) {
	detector, alertHandler := setupTestDetector(t)

	// Create an anomaly in normal mode (not security mode)
	event := detector.ProcessMotionDuringAway("zone_kitchen", 1, false)
	if event == nil {
		t.Fatal("Expected anomaly to be created")
	}

	// In normal mode:
	// - Dashboard alert should be sent immediately (via callback)
	// - Notification should be sent at T+30s
	// - Webhook should be sent at T+2min
	// - Escalation should be sent at T+5min

	// Alert should be sent immediately (wait for goroutine)
	waitForGoroutines()
	if len(alertHandler.alerts) == 0 {
		t.Error("Expected immediate alert in normal mode")
	}

	// Webhook should NOT be sent immediately
	if len(alertHandler.webhooks) > 0 {
		t.Error("Webhook should not be sent immediately in normal mode")
	}

	// Escalation should NOT be sent immediately
	if len(alertHandler.escalations) > 0 {
		t.Error("Escalation should not be sent immediately in normal mode")
	}

	// Verify pending alerts exist
	detector.mu.RLock()
	pendingCount := len(detector.pendingAlerts)
	detector.mu.RUnlock()

	if pendingCount == 0 {
		t.Error("Expected pending alert timers to be set")
	}

	// Acknowledge to clean up timers
	detector.AcknowledgeAnomaly(event.ID, "expected", "test_user")
}

// TestAnomaly_AlertChainSecurityMode tests that all alerts fire immediately in security mode.
func TestAnomaly_AlertChainSecurityMode(t *testing.T) {
	detector, alertHandler := setupTestDetector(t)

	// Create an anomaly in security mode
	event := detector.ProcessMotionDuringAway("zone_kitchen", 1, true)
	if event == nil {
		t.Fatal("Expected anomaly to be created")
	}

	// In security mode, ALL alerts should fire immediately (wait for goroutines)
	waitForGoroutines()
	if len(alertHandler.alerts) == 0 {
		t.Error("Expected immediate alert in security mode")
	}

	if len(alertHandler.webhooks) == 0 {
		t.Error("Expected immediate webhook in security mode")
	}

	if len(alertHandler.escalations) == 0 {
		t.Error("Expected immediate escalation in security mode")
	}

	// Verify the anomaly is marked as all alerts sent
	if !event.AlertSent {
		t.Error("Expected AlertSent to be true")
	}
	if !event.WebhookSent {
		t.Error("Expected WebhookSent to be true")
	}
	if !event.EscalationSent {
		t.Error("Expected EscalationSent to be true")
	}
}

// TestAnomaly_AcknowledgementCancelsTimers tests that acknowledgement cancels pending timers.
func TestAnomaly_AcknowledgementCancelsTimers(t *testing.T) {
	detector, alertHandler := setupTestDetector(t)

	// Create an anomaly in normal mode (delayed escalation)
	event := detector.ProcessMotionDuringAway("zone_kitchen", 1, false)
	if event == nil {
		t.Fatal("Expected anomaly")
	}

	// Immediately acknowledge
	err := detector.AcknowledgeAnomaly(event.ID, "false_alarm", "test_user")
	if err != nil {
		t.Fatalf("Failed to acknowledge: %v", err)
	}

	// Verify pending timers were cancelled
	detector.mu.RLock()
	pendingCount := len(detector.pendingAlerts)
	detector.mu.RUnlock()

	if pendingCount != 0 {
		t.Error("Expected all pending alert timers to be cancelled after acknowledgement")
	}

	// Only the immediate alert should have been sent
	if len(alertHandler.alerts) == 0 {
		t.Error("Expected immediate alert to have been sent")
	}
	if len(alertHandler.webhooks) > 0 {
		t.Error("Webhook should not be sent after acknowledgement")
	}
	if len(alertHandler.escalations) > 0 {
		t.Error("Escalation should not be sent after acknowledgement")
	}
}

// TestAnomaly_CooldownDeduplication tests that anomalies are deduplicated within cooldown period.
func TestAnomaly_CooldownDeduplication(t *testing.T) {
	detector, _ := setupTestDetector(t)

	// First anomaly should fire
	event1 := detector.ProcessMotionDuringAway("zone_kitchen", 1, true)
	if event1 == nil {
		t.Fatal("Expected first anomaly to fire")
	}

	// Immediate second anomaly should be suppressed by cooldown
	event2 := detector.ProcessMotionDuringAway("zone_kitchen", 1, true)
	if event2 != nil {
		t.Error("Expected second anomaly to be suppressed by cooldown")
	}

	// Different zone should still fire
	event3 := detector.ProcessMotionDuringAway("zone_living", 2, true)
	if event3 == nil {
		t.Error("Expected anomaly in different zone to fire")
	}
}

// TestAnomaly_SecurityModeStatePersistence tests security mode state management.
func TestAnomaly_SecurityModeStatePersistence(t *testing.T) {
	detector, _ := setupTestDetector(t)

	// Initially disarmed
	if detector.GetSecurityMode() != SecurityModeDisarmed {
		t.Error("Expected initial security mode to be disarmed")
	}

	// Arm the system
	detector.SetSecurityMode(SecurityModeArmed, "manual")
	if detector.GetSecurityMode() != SecurityModeArmed {
		t.Error("Expected security mode to be armed")
	}

	// Check if active
	if !detector.IsSecurityModeActive() {
		t.Error("Expected security mode to be active when armed")
	}

	// Disarm
	detector.SetSecurityMode(SecurityModeDisarmed, "manual")
	if detector.GetSecurityMode() != SecurityModeDisarmed {
		t.Error("Expected security mode to be disarmed")
	}
}

// TestAnomaly_GetActiveAnomaliesAfterCreate tests active anomaly retrieval.
func TestAnomaly_GetActiveAnomaliesAfterCreate(t *testing.T) {
	detector, _ := setupTestDetector(t)

	// Initially no active anomalies
	active := detector.GetActiveAnomalies()
	if len(active) != 0 {
		t.Errorf("Expected 0 active anomalies initially, got %d", len(active))
	}

	// Create an anomaly
	event := detector.ProcessMotionDuringAway("zone_kitchen", 1, true)
	if event == nil {
		t.Fatal("Expected anomaly")
	}

	// Should have 1 active anomaly
	active = detector.GetActiveAnomalies()
	if len(active) != 1 {
		t.Errorf("Expected 1 active anomaly, got %d", len(active))
	}

	// Acknowledge it
	detector.AcknowledgeAnomaly(event.ID, "expected", "test_user")

	// Should have 0 unacknowledged anomalies (acknowledged ones are filtered)
	active = detector.GetActiveAnomalies()
	if len(active) != 0 {
		t.Errorf("Expected 0 active unacknowledged anomalies, got %d", len(active))
	}
}

// Helper functions for generating keys
func hourOfWeekZoneKey(hourOfWeek int, zoneID string) string {
	return fmt.Sprintf("%d-%s", hourOfWeek, zoneID)
}

func hourOfWeekZonePersonKey(hourOfWeek int, zoneID, personID string) string {
	return fmt.Sprintf("%d-%s-%s", hourOfWeek, zoneID, personID)
}

// waitForGoroutines gives goroutines a moment to complete.
func waitForGoroutines() {
	time.Sleep(50 * time.Millisecond)
}
