// Package guidedtroubleshoot tests
package guidedtroubleshoot

import (
	"context"
	"testing"
	"time"
)

func TestEditTracker_RecordEdit(t *testing.T) {
	tracker := NewEditTracker()

	tests := []struct {
		name          string
		key           string
		edits         int
		wantHint      bool
		description   string
	}{
		{
			name:        "non-qualifying key",
			key:         "theme",
			edits:       5,
			wantHint:    false,
			description: "non-qualifying keys never trigger hints",
		},
		{
			name:        "below threshold",
			key:         "delta_rms_threshold",
			edits:       2,
			wantHint:    false,
			description: "less than 3 edits doesn't trigger hint",
		},
		{
			name:        "at threshold",
			key:         "tau_s",
			edits:       3,
			wantHint:    true,
			description: "3 edits triggers hint",
		},
		{
			name:        "above threshold",
			key:         "fresnel_decay",
			edits:       5,
			wantHint:    true,
			description: "more than 3 edits triggers hint",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker.Reset()

			var gotHint bool
			for i := 0; i < tt.edits; i++ {
				gotHint, _ = tracker.RecordEdit(tt.key)
			}

			if gotHint != tt.wantHint {
				t.Errorf("%s: RecordEdit() hint = %v, want %v", tt.description, gotHint, tt.wantHint)
			}
		})
	}
}

func TestEditTracker_TimeWindow(t *testing.T) {
	tracker := NewEditTracker()
	key := "delta_rms_threshold"

	// First edit
	hint, _ := tracker.RecordEdit(key)
	if hint {
		t.Error("First edit should not trigger hint")
	}

	// Second edit immediately
	hint, _ = tracker.RecordEdit(key)
	if hint {
		t.Error("Second edit should not trigger hint")
	}

	// Third edit after 1 second (within window)
	time.Sleep(1 * time.Second)
	hint, _ = tracker.RecordEdit(key)
	if !hint {
		t.Error("Third edit within window should trigger hint")
	}

	// Mark hint shown
	tracker.MarkHintShown(key)

	// Wait for cooldown to pass (simulated by resetting)
	tracker.Reset()

	// Should be able to trigger again
	hint, _ = tracker.RecordEdit(key)
	if hint {
		t.Error("First edit after reset should not trigger hint")
	}

	hint, _ = tracker.RecordEdit(key)
	if hint {
		t.Error("Second edit after reset should not trigger hint")
	}

	hint, _ = tracker.RecordEdit(key)
	if !hint {
		t.Error("Third edit after reset should trigger hint")
	}
}

func TestEditTracker_OutOfWindow(t *testing.T) {
	tracker := NewEditTracker()
	tracker.EditWindow = 50 * time.Millisecond // short window for testing
	key := "breathing_sensitivity"

	// First edit
	hint, _ := tracker.RecordEdit(key)
	if hint {
		t.Error("First edit should not trigger hint")
	}

	// Wait longer than the window
	time.Sleep(100 * time.Millisecond)

	// Second edit (should reset counter due to window expiry)
	tracker.RecordEdit(key)

	// Third edit immediately after second
	hint, _ = tracker.RecordEdit(key)
	if hint {
		t.Error("Third edit with expired window should not trigger hint (counter reset)")
	}

	// Fourth edit
	hint, _ = tracker.RecordEdit(key)
	if !hint {
		t.Error("Fourth edit should trigger hint")
	}
}

func TestZoneQualityTracker_UpdateQuality(t *testing.T) {
	getAll := func() ([]ZoneInfo, error) {
		return []ZoneInfo{
			{ID: 1, Name: "Kitchen", Quality: 50},
		}, nil
	}

	tracker := NewZoneQualityTracker(getAll)

	tests := []struct {
		name           string
		initialQuality float64
		newQuality     float64
		elapsed        time.Duration
		wantBanner     bool
		wantResolved   bool
	}{
		{
			name:           "good quality",
			initialQuality: 80,
			newQuality:     75,
			elapsed:        1 * time.Hour,
			wantBanner:     false,
			wantResolved:   false,
		},
		{
			name:           "quality drops but not long enough",
			initialQuality: 80,
			newQuality:     50,
			elapsed:        1 * time.Hour,
			wantBanner:     false,
			wantResolved:   false,
		},
		{
			name:           "quality poor for 24+ hours",
			initialQuality: 80,
			newQuality:     50,
			elapsed:        25 * time.Hour,
			wantBanner:     false, // Historical initialization: firstPoorTime is set to now, not initialTime
			wantResolved:   false,
		},
		{
			name:           "quality recovers",
			initialQuality: 50,
			newQuality:     75,
			elapsed:        1 * time.Hour,
			wantBanner:     false,
			wantResolved:   false,
		},
		{
			name:           "quality recovers above threshold",
			initialQuality: 50,
			newQuality:     75,
			elapsed:        1 * time.Hour,
			wantBanner:     false,
			wantResolved:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker.Reset()

			now := time.Now()
			initialTime := now.Add(-tt.elapsed)

			// Set initial quality
			tracker.UpdateQuality(1, tt.initialQuality, initialTime)

			// Update quality
			gotBanner, gotResolved := tracker.UpdateQuality(1, tt.newQuality, now)

			if gotBanner != tt.wantBanner {
				t.Errorf("UpdateQuality() banner = %v, want %v", gotBanner, tt.wantBanner)
			}
			if gotResolved != tt.wantResolved {
				t.Errorf("UpdateQuality() resolved = %v, want %v", gotResolved, tt.wantResolved)
			}
		})
	}
}

func TestZoneQualityTracker_RecoveryWithHysteresis(t *testing.T) {
	getAll := func() ([]ZoneInfo, error) {
		return []ZoneInfo{
			{ID: 1, Name: "Kitchen", Quality: 50},
		}, nil
	}

	tracker := NewZoneQualityTracker(getAll)
	now := time.Now()

	// Set poor quality
	tracker.UpdateQuality(1, 50, now.Add(-25*time.Hour))

	// Check banner should show
	showBanner, _ := tracker.UpdateQuality(1, 50, now)
	if !showBanner {
		t.Error("Should show banner after 25h of poor quality")
	}

	// Quality improves but not enough for recovery
	_, resolved := tracker.UpdateQuality(1, 65, now.Add(1*time.Second))
	if resolved {
		t.Error("Should not resolve with quality just above threshold")
	}

	// Quality recovers fully
	showBanner2, resolved2 := tracker.UpdateQuality(1, 75, now.Add(2*time.Second))
	if showBanner2 {
		t.Error("Should not show banner after recovery")
	}
	if !resolved2 {
		t.Error("Should resolve with quality above recovery threshold")
	}
}

func TestZoneQualityTracker_GetZonesWithPoorQuality(t *testing.T) {
	getAll := func() ([]ZoneInfo, error) {
		return []ZoneInfo{
			{ID: 1, Name: "Kitchen", Quality: 50},
			{ID: 2, Name: "Living Room", Quality: 80},
		}, nil
	}

	tracker := NewZoneQualityTracker(getAll)
	now := time.Now()

	// Set poor quality for zone 1
	tracker.UpdateQuality(1, 50, now.Add(-25*time.Hour))

	// Set good quality for zone 2
	tracker.UpdateQuality(2, 80, now)

	zones := tracker.GetZonesWithPoorQuality()

	if len(zones) != 1 {
		t.Errorf("Got %d zones with poor quality, want 1", len(zones))
	}

	if len(zones) > 0 && zones[0] != 1 {
		t.Errorf("Got zone %d with poor quality, want 1", zones[0])
	}
}

func TestManager_BasicFlow(t *testing.T) {
	getAll := func() ([]ZoneInfo, error) {
		return []ZoneInfo{
			{ID: 1, Name: "Kitchen", Quality: 50},
		}, nil
	}

	cfg := ManagerConfig{
		CheckInterval: 100 * time.Millisecond,
		GetAllZones:   getAll,
	}

	mgr := NewManager(cfg)

	qualityCalls := 0

	mgr.SetOnQualityIssue(func(zoneID int, quality float64) {
		qualityCalls++
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go mgr.Run(ctx)

	// Wait for initial check
	time.Sleep(200 * time.Millisecond)

	// Update zone quality to trigger issue
	mgr.UpdateZoneQuality(1, 50)

	// Wait for next check
	time.Sleep(150 * time.Millisecond)

	// The callback should not have been called yet (need 24h)
	if qualityCalls > 0 {
		t.Error("Quality callback should not fire immediately (needs 24h poor quality)")
	}

	cancel()
}

func TestManager_SettingsEditTracking(t *testing.T) {
	cfg := ManagerConfig{
		CheckInterval: 1 * time.Minute,
	}

	mgr := NewManager(cfg)

	// Record edits
	mgr.RecordSettingsEdit("delta_rms_threshold")
	mgr.RecordSettingsEdit("delta_rms_threshold")

	hintPending := mgr.RecordSettingsEdit("delta_rms_threshold")

	if !hintPending {
		t.Error("Third edit should trigger hint")
	}

	// Check edit count
	count := mgr.GetSettingsEditCount("delta_rms_threshold")
	if count != 3 {
		t.Errorf("Got edit count %d, want 3", count)
	}

	// Mark hint shown
	mgr.MarkSettingsHintShown("delta_rms_threshold")

	// Edit count should still be 3
	count = mgr.GetSettingsEditCount("delta_rms_threshold")
	if count != 3 {
		t.Errorf("Got edit count %d after marking hint shown, want 3", count)
	}
}

func TestManager_Callbacks(t *testing.T) {
	cfg := ManagerConfig{
		CheckInterval: 1 * time.Minute,
	}

	mgr := NewManager(cfg)

	// Test quality callback
	qualityCalled := false
	mgr.SetOnQualityIssue(func(zoneID int, quality float64) {
		qualityCalled = true
	})

	mgr.TriggerCalibrationComplete(1, 40.0, 85.0)

	if qualityCalled {
		t.Error("Quality callback should not be called by calibration complete")
	}

	// Test node offline callback
	offlineCalled := false
	var offlineMAC string
	var offlineDuration time.Duration

	mgr.SetOnNodeOffline(func(mac string, duration time.Duration) {
		offlineCalled = true
		offlineMAC = mac
		offlineDuration = duration
	})

	mgr.TriggerNodeOffline("AA:BB:CC:DD:EE:FF", 2*time.Hour)

	if !offlineCalled {
		t.Error("Node offline callback should be called")
	}
	if offlineMAC != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("Got MAC %s, want AA:BB:CC:DD:EE:FF", offlineMAC)
	}
	if offlineDuration != 2*time.Hour {
		t.Errorf("Got duration %v, want 2h", offlineDuration)
	}
}

func TestQualifyingSettingsKeys(t *testing.T) {
	// Verify all expected keys are present
	expectedKeys := []string{
		"delta_rms_threshold",
		"breathing_sensitivity",
		"tau_s",
		"fresnel_decay",
		"n_subcarriers",
		"motion_threshold",
	}

	for _, key := range expectedKeys {
		if !QualifyingSettingsKeys[key] {
			t.Errorf("Key %s not in QualifyingSettingsKeys", key)
		}
	}

	// Verify non-qualifying keys are not present
	nonQualifying := []string{
		"theme",
		"layout",
		"notification_config",
		"mqtt_config",
	}

	tracker := NewEditTracker()
	for _, key := range nonQualifying {
		hint, _ := tracker.RecordEdit(key)
		if hint {
			t.Errorf("Non-qualifying key %s should not trigger hint", key)
		}
	}
}
