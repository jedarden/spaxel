// Package help provides tests for the feature notification manager.
package help

import (
	"database/sql"
	"os"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// TestNotifierFireAndRetrieve tests firing a notification and retrieving it.
func TestNotifierFireAndRetrieve(t *testing.T) {
	db := createTestDB(t)
	defer db.Close()

	notifier, err := NewNotifier(db)
	if err != nil {
		t.Fatalf("Failed to create notifier: %v", err)
	}

	// Fire a notification
	eventID := "test_feature_1"
	fired := notifier.FireNotification(eventID, "Test Feature", "This is a test notification")
	if !fired {
		t.Error("Expected notification to be fired")
	}

	// Verify it was recorded
	var firedAt int64
	err = db.QueryRow("SELECT fired_at FROM feature_notifications WHERE event_id = ?", eventID).Scan(&firedAt)
	if err != nil {
		t.Errorf("Failed to query notification: %v", err)
	}

	// Try firing again - should not fire
	firedAgain := notifier.FireNotification(eventID, "Test Feature", "This is a test notification")
	if firedAgain {
		t.Error("Expected notification to not fire again")
	}

	// Get pending notifications
	notifications, err := notifier.GetPendingNotifications()
	if err != nil {
		t.Fatalf("Failed to get pending notifications: %v", err)
	}

	if len(notifications) != 1 {
		t.Errorf("Expected 1 notification, got %d", len(notifications))
	}

	if notifications[0].EventID != eventID {
		t.Errorf("Expected event_id %s, got %s", eventID, notifications[0].EventID)
	}
}

// TestNotifierAcknowledge tests acknowledging a notification.
func TestNotifierAcknowledge(t *testing.T) {
	db := createTestDB(t)
	defer db.Close()

	notifier, err := NewNotifier(db)
	if err != nil {
		t.Fatalf("Failed to create notifier: %v", err)
	}

	// Fire a notification
	eventID := "test_feature_2"
	notifier.FireNotification(eventID, "Test Feature", "This is a test notification")

	// Verify it's pending
	notifications, _ := notifier.GetPendingNotifications()
	if len(notifications) != 1 {
		t.Errorf("Expected 1 pending notification, got %d", len(notifications))
	}

	// Acknowledge it
	err = notifier.AcknowledgeNotification(eventID)
	if err != nil {
		t.Errorf("Failed to acknowledge notification: %v", err)
	}

	// Verify it's no longer pending
	notifications, _ = notifier.GetPendingNotifications()
	if len(notifications) != 0 {
		t.Errorf("Expected 0 pending notifications after acknowledge, got %d", len(notifications))
	}
}

// TestNotifierQuietHours tests that notifications are suppressed during quiet hours.
func TestNotifierQuietHours(t *testing.T) {
	db := createTestDB(t)
	defer db.Close()

	notifier, err := NewNotifier(db)
	if err != nil {
		t.Fatalf("Failed to create notifier: %v", err)
	}

	// Set quiet hours for current time
	now := time.Now()
	_ = now.Hour()*60 + now.Minute() // Current time in minutes (not directly used)

	notifier.SetQuietHours(&QuietHours{
		Enabled:   true,
		StartHour: now.Hour(),
		StartMin:  now.Minute(),
		EndHour:   now.Hour(),
		EndMin:    now.Minute() + 30,
		DaysMask:  1 << uint(now.Weekday()),
	})

	// Try to fire during quiet hours - should be suppressed
	fired := notifier.FireNotification("quiet_test", "Quiet Test", "Should be suppressed")
	if fired {
		t.Error("Expected notification to be suppressed during quiet hours")
	}

	// Disable quiet hours and try again
	notifier.SetQuietHours(&QuietHours{Enabled: false})
	fired = notifier.FireNotification("quiet_test_2", "Quiet Test 2", "Should fire now")
	if !fired {
		t.Error("Expected notification to fire when quiet hours disabled")
	}
}

// TestNotifierContentHelpers tests the content generation for known event types.
func TestNotifierContentHelpers(t *testing.T) {
	tests := []struct {
		eventID     string
		wantTitle   string
		wantMessage string
	}{
		{EventDiurnalBaselineActivated, "Your system has learned your home's daily patterns", ""},
		{EventFirstSleepSessionComplete, "Your first sleep session was tracked overnight", ""},
		{EventWeightUpdateApproved, "Localization accuracy improved", ""},
		{EventAutomationFirstFired, "Your first automation just ran", ""},
		{EventPredictionModelReady, "Presence predictions are now available", ""},
		{"unknown_event", "New Feature Available", "A new feature is now available"},
	}

	for _, tt := range tests {
		t.Run(tt.eventID, func(t *testing.T) {
			title := getNotificationTitle(tt.eventID)
			if title != tt.wantTitle {
				t.Errorf("getNotificationTitle() = %q, want %q", title, tt.wantTitle)
			}

			message := getNotificationMessage(tt.eventID)
			if tt.wantMessage != "" && message != tt.wantMessage {
				t.Errorf("getNotificationMessage() = %q, want %q", message, tt.wantMessage)
			}
		})
	}
}

// TestNotifierFireWithAction tests firing notifications with action buttons.
func TestNotifierFireWithAction(t *testing.T) {
	db := createTestDB(t)
	defer db.Close()

	notifier, err := NewNotifier(db)
	if err != nil {
		t.Fatalf("Failed to create notifier: %v", err)
	}

	// Fire notification with action
	eventID := EventDiurnalBaselineActivated
	fired := notifier.FireNotificationWithAction(
		eventID,
		"Diurnal Baseline Ready",
		"Your system has learned patterns",
		"View Details",
		"#/diurnal",
	)

	if !fired {
		t.Error("Expected notification to be fired")
	}

	// Retrieve and check action
	notifications, _ := notifier.GetPendingNotifications()
	if len(notifications) != 1 {
		t.Fatalf("Expected 1 notification, got %d", len(notifications))
	}

	if notifications[0].ActionLabel != "View Details" {
		t.Errorf("Expected action label 'View Details', got %q", notifications[0].ActionLabel)
	}

	if notifications[0].ActionURL != "#/diurnal" {
		t.Errorf("Expected action URL '#/diurnal', got %q", notifications[0].ActionURL)
	}
}

// createTestDB creates an in-memory test database.
func createTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// Create schema
	if err := ensureSchema(db); err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	return db
}

// TestMain sets up test environment
func TestMain(m *testing.M) {
	// Run tests
	code := m.Run()
	os.Exit(code)
}
