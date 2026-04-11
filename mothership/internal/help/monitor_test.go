// Package help provides tests for the feature discovery monitor.
package help

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// TestFeatureMonitorBasic tests the basic monitor functionality.
func TestFeatureMonitorBasic(t *testing.T) {
	db := createMonitorTestDB(t)
	defer db.Close()

	notifier, err := NewNotifier(db)
	if err != nil {
		t.Fatalf("Failed to create notifier: %v", err)
	}

	monitor := NewFeatureMonitor(FeatureMonitorConfig{
		DB:            db,
		Notifier:      notifier,
		CheckInterval: 100 * time.Millisecond,
	})

	// Set up a checker that returns true after a delay
	callCount := 0
	monitor.SetDiurnalReadyChecker(func() bool {
		callCount++
		return callCount >= 2 // Return true on second call
	})

	// Start the monitor
	monitor.Start()
	defer monitor.Stop()

	// Wait for at least 2 check cycles
	time.Sleep(250 * time.Millisecond)

	// Verify notification was fired
	notifications, err := notifier.GetPendingNotifications()
	if err != nil {
		t.Fatalf("Failed to get pending notifications: %v", err)
	}

	found := false
	for _, n := range notifications {
		if n.EventID == EventDiurnalBaselineActivated {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected DiurnalBaselineActivated notification to be fired")
	}
}

// TestFeatureMonitorMultipleFeatures tests monitoring multiple features.
func TestFeatureMonitorMultipleFeatures(t *testing.T) {
	db := createMonitorTestDB(t)
	defer db.Close()

	notifier, err := NewNotifier(db)
	if err != nil {
		t.Fatalf("Failed to create notifier: %v", err)
	}

	monitor := NewFeatureMonitor(FeatureMonitorConfig{
		DB:            db,
		Notifier:      notifier,
		CheckInterval: 100 * time.Millisecond,
	})

	// Set up checkers with different readiness
	diurnalCallCount := 0
	monitor.SetDiurnalReadyChecker(func() bool {
		diurnalCallCount++
		return diurnalCallCount >= 1 // Ready immediately
	})

	sleepCallCount := 0
	monitor.SetFirstSleepSessionChecker(func() bool {
		sleepCallCount++
		return sleepCallCount >= 3 // Ready after 3 checks
	})

	weightCallCount := 0
	monitor.SetWeightUpdateChecker(func() bool {
		weightCallCount++
		return false // Never ready
	})

	// Start the monitor
	monitor.Start()
	defer monitor.Stop()

	// Wait for enough cycles
	time.Sleep(450 * time.Millisecond)

	// Verify notifications
	notifications, err := notifier.GetPendingNotifications()
	if err != nil {
		t.Fatalf("Failed to get pending notifications: %v", err)
	}

	foundDiurnal := false
	foundSleep := false
	foundWeight := false

	for _, n := range notifications {
		switch n.EventID {
		case EventDiurnalBaselineActivated:
			foundDiurnal = true
		case EventFirstSleepSessionComplete:
			foundSleep = true
		case EventWeightUpdateApproved:
			foundWeight = true
		}
	}

	if !foundDiurnal {
		t.Error("Expected DiurnalBaselineActivated notification")
	}
	if !foundSleep {
		t.Error("Expected FirstSleepSessionComplete notification")
	}
	if foundWeight {
		t.Error("Did not expect WeightUpdateApproved notification")
	}
}

// TestFeatureMonitorPredictionPerPerson tests per-person prediction readiness.
func TestFeatureMonitorPredictionPerPerson(t *testing.T) {
	db := createMonitorTestDB(t)
	defer db.Close()

	// Set up prediction_models table with some persons
	setupPredictionModels(t, db)

	notifier, err := NewNotifier(db)
	if err != nil {
		t.Fatalf("Failed to create notifier: %v", err)
	}

	monitor := NewFeatureMonitor(FeatureMonitorConfig{
		DB:            db,
		Notifier:      notifier,
		CheckInterval: 100 * time.Millisecond,
	})

	// Set up prediction checker that returns true after 2 calls
	callCount := make(map[string]int)
	monitor.SetPredictionReadyChecker(func(personID string) bool {
		callCount[personID]++
		return callCount[personID] >= 2
	})

	// Start the monitor
	monitor.Start()
	defer monitor.Stop()

	// Wait for enough cycles
	time.Sleep(250 * time.Millisecond)

	// Verify notifications for both persons
	notifications, err := notifier.GetPendingNotifications()
	if err != nil {
		t.Fatalf("Failed to get pending notifications: %v", err)
	}

	foundAlice := false
	foundBob := false

	for _, n := range notifications {
		if n.EventID == "prediction_model_ready_Alice" {
			foundAlice = true
			if n.Title != "Presence predictions are now available for Alice" {
				t.Errorf("Expected person-specific title for Alice, got: %s", n.Title)
			}
		}
		if n.EventID == "prediction_model_ready_Bob" {
			foundBob = true
		}
	}

	if !foundAlice {
		t.Error("Expected prediction model ready notification for Alice")
	}
	if !foundBob {
		t.Error("Expected prediction model ready notification for Bob")
	}
}

// TestFeatureMonitorQuietHours tests that notifications respect quiet hours.
func TestFeatureMonitorQuietHours(t *testing.T) {
	db := createMonitorTestDB(t)
	defer db.Close()

	notifier, err := NewNotifier(db)
	if err != nil {
		t.Fatalf("Failed to create notifier: %v", err)
	}

	// Set quiet hours for current time
	now := time.Now()
	notifier.SetQuietHours(&QuietHours{
		Enabled:   true,
		StartHour: now.Hour(),
		StartMin:  now.Minute(),
		EndHour:   now.Hour(),
		EndMin:    now.Minute() + 30,
		DaysMask:  1 << uint(now.Weekday()),
	})

	monitor := NewFeatureMonitor(FeatureMonitorConfig{
		DB:            db,
		Notifier:      notifier,
		CheckInterval: 100 * time.Millisecond,
	})

	readyCalled := false
	monitor.SetDiurnalReadyChecker(func() bool {
		readyCalled = true
		return true
	})

	// Start the monitor
	monitor.Start()
	defer monitor.Stop()

	// Wait for check
	time.Sleep(150 * time.Millisecond)

	// Verify checker was called
	if !readyCalled {
		t.Error("Expected checker to be called even during quiet hours")
	}

	// Verify notification was NOT fired (suppressed by quiet hours)
	notifications, err := notifier.GetPendingNotifications()
	if err != nil {
		t.Fatalf("Failed to get pending notifications: %v", err)
	}

	for _, n := range notifications {
		if n.EventID == EventDiurnalBaselineActivated {
			t.Error("Expected notification to be suppressed during quiet hours")
		}
	}
}

// setupPredictionModels creates test prediction model entries.
func setupPredictionModels(t *testing.T, db *sql.DB) {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS prediction_models (
			person TEXT NOT NULL,
			zone_id INTEGER NOT NULL,
			time_slot INTEGER NOT NULL,
			day_type TEXT NOT NULL,
			probability REAL NOT NULL DEFAULT 0,
			sample_count INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL,
			PRIMARY KEY (person, zone_id, time_slot, day_type)
		);
	`)
	if err != nil {
		t.Fatalf("Failed to create prediction_models table: %v", err)
	}

	// Insert test data for Alice and Bob
	now := time.Now().Unix()
	_, err = db.Exec(`
		INSERT INTO prediction_models (person, zone_id, time_slot, day_type, probability, sample_count, updated_at)
		VALUES
			('Alice', 1, 10, 'weekday', 0.5, 10, ?),
			('Alice', 1, 11, 'weekday', 0.6, 8, ?),
			('Bob', 1, 10, 'weekday', 0.4, 5, ?);
	`, now, now, now)
	if err != nil {
		t.Fatalf("Failed to insert prediction models: %v", err)
	}
}

// createMonitorTestDB creates an in-memory test database with the feature_notifications schema.
func createMonitorTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// Create the feature_notifications table
	schema := `
	CREATE TABLE IF NOT EXISTS feature_notifications (
		event_id TEXT PRIMARY KEY,
		fired_at INTEGER NOT NULL,
		acknowledged_at INTEGER
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	return db
}

// TestPredictionModelReadyEventID tests the event ID generation.
func TestPredictionModelReadyEventID(t *testing.T) {
	tests := []struct {
		personID string
		want     string
	}{
		{"Alice", "prediction_model_ready_Alice"},
		{"Bob", "prediction_model_ready_Bob"},
		{"Charlie-123", "prediction_model_ready_Charlie-123"},
	}

	for _, tt := range tests {
		t.Run(tt.personID, func(t *testing.T) {
			got := PredictionModelReadyEventID(tt.personID)
			if got != tt.want {
				t.Errorf("PredictionModelReadyEventID(%q) = %q, want %q", tt.personID, got, tt.want)
			}
		})
	}
}

// TestGetPersonNotificationTitle tests person-specific notification titles.
func TestGetPersonNotificationTitle(t *testing.T) {
	tests := []struct {
		personID  string
		baseEvent string
		want      string
	}{
		{"Alice", EventPredictionModelReady, "Presence predictions are now available for Alice"},
		{"Bob", EventPredictionModelReady, "Presence predictions are now available for Bob"},
		{"Alice", EventDiurnalBaselineActivated, "Your system has learned your home's daily patterns"},
	}

	for _, tt := range tests {
		t.Run(tt.personID+"_"+tt.baseEvent, func(t *testing.T) {
			got := getPersonNotificationTitle(tt.personID, tt.baseEvent)
			if got != tt.want {
				t.Errorf("getPersonNotificationTitle(%q, %q) = %q, want %q", tt.personID, tt.baseEvent, got, tt.want)
			}
		})
	}
}

// TestGetPersonNotificationMessage tests person-specific notification messages.
func TestGetPersonNotificationMessage(t *testing.T) {
	personID := "Alice"
	baseEvent := EventPredictionModelReady
	got := getPersonNotificationMessage(personID, baseEvent)

	wantPrefix := "The system has learned when " + personID + " is typically in each room"
	if len(got) < len(wantPrefix) || got[:len(wantPrefix)] != wantPrefix {
		t.Errorf("getPersonNotificationMessage(%q, %q) = %q, want prefix %q", personID, baseEvent, got, wantPrefix)
	}
}

// TestFeatureMonitorIdempotent tests that notifications fire only once.
func TestFeatureMonitorIdempotent(t *testing.T) {
	db := createMonitorTestDB(t)
	defer db.Close()

	notifier, err := NewNotifier(db)
	if err != nil {
		t.Fatalf("Failed to create notifier: %v", err)
	}

	monitor := NewFeatureMonitor(FeatureMonitorConfig{
		DB:            db,
		Notifier:      notifier,
		CheckInterval: 50 * time.Millisecond,
	})

	readyCalledCount := 0
	monitor.SetDiurnalReadyChecker(func() bool {
		readyCalledCount++
		t.Logf("Checker called: count=%d at %v", readyCalledCount, time.Now().Format("15:04:05.000"))
		return true // Always ready
	})

	// Start the monitor
	t.Logf("Starting monitor at %v", time.Now().Format("15:04:05.000"))
	monitor.Start()

	// Wait for multiple check cycles - wait for at least 3 ticker intervals
	// Initial check happens immediately, then ticker fires every 50ms
	waitTime := 200 * time.Millisecond
	t.Logf("Waiting %v for ticker fires...", waitTime)
	time.Sleep(waitTime)

	t.Logf("After sleep: count=%d, now calling Stop()", readyCalledCount)
	monitor.Stop()

	t.Logf("After Stop: count=%d", readyCalledCount)

	// Verify checker was called at least once (it might be called only 1-2 times due to timing)
	if readyCalledCount < 1 {
		t.Errorf("Expected checker to be called at least once, got %d", readyCalledCount)
	}

	// Verify notification was fired only once
	notifications, err := notifier.GetPendingNotifications()
	if err != nil {
		t.Fatalf("Failed to get pending notifications: %v", err)
	}

	count := 0
	for _, n := range notifications {
		if n.EventID == EventDiurnalBaselineActivated {
			count++
		}
	}

	if count != 1 {
		t.Errorf("Expected exactly 1 notification, got %d", count)
	}
}
