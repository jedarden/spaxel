// Package notify provides tests for the notification service.
package notify

import (
	"testing"
	"time"
)

// TestServiceCreation tests creating a new notification service.
func TestServiceCreation(t *testing.T) {
	// Create a temporary database
	dbPath := t.TempDir() + "/test_notify.db"

	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	if service == nil {
		t.Fatal("NewService() returned nil")
	}
}

// TestQuietHours tests the quiet hours functionality.
func TestQuietHours(t *testing.T) {
	dbPath := t.TempDir() + "/test_notify.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	// Test setting quiet hours
	qh := QuietHoursConfig{
		Enabled:       true,
		StartHour:     22,
		StartMin:      0,
		EndHour:       7,
		EndMin:        0,
		MorningDigest: true,
		DigestHour:    7,
		DigestMin:     0,
	}

	err = service.SetQuietHours(qh)
	if err != nil {
		t.Fatalf("SetQuietHours() error = %v", err)
	}

	// Verify quiet hours were set
	retrieved := service.GetQuietHours()
	if !retrieved.Enabled {
		t.Error("Quiet hours not enabled")
	}
	if retrieved.StartHour != 22 {
		t.Errorf("StartHour = %d, want 22", retrieved.StartHour)
	}
	if retrieved.EndHour != 7 {
		t.Errorf("EndHour = %d, want 7", retrieved.EndHour)
	}
}

// TestBatchingConfig tests the batching configuration.
func TestBatchingConfig(t *testing.T) {
	dbPath := t.TempDir() + "/test_notify.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	// Test setting batching config
	bc := BatchingConfig{
		Enabled:          true,
		BatchWindowSec:   30,
		MaxBatchSize:     5,
		BatchLowPriority: true,
		BatchMedium:      true,
	}

	err = service.SetBatchingConfig(bc)
	if err != nil {
		t.Fatalf("SetBatchingConfig() error = %v", err)
	}

	// Verify batching config was set
	retrieved := service.GetBatchingConfig()
	if !retrieved.Enabled {
		t.Error("Batching not enabled")
	}
	if retrieved.BatchWindowSec != 30 {
		t.Errorf("BatchWindowSec = %d, want 30", retrieved.BatchWindowSec)
	}
	if retrieved.MaxBatchSize != 5 {
		t.Errorf("MaxBatchSize = %d, want 5", retrieved.MaxBatchSize)
	}
}

// TestAddChannel tests adding a notification channel.
func TestAddChannel(t *testing.T) {
	dbPath := t.TempDir() + "/test_notify.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	// Test adding a channel
	cc := ChannelConfig{
		Type:    ChannelNtfy,
		Enabled: true,
		URL:     "https://ntfy.sh/test-topic",
	}

	err = service.AddChannel("test-ntfy", cc)
	if err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	// Verify channel was added
	channels := service.GetChannels()
	if len(channels) != 1 {
		t.Errorf("Got %d channels, want 1", len(channels))
	}

	if _, ok := channels["test-ntfy"]; !ok {
		t.Error("Channel 'test-ntfy' not found")
	}
}

// TestRemoveChannel tests removing a notification channel.
func TestRemoveChannel(t *testing.T) {
	dbPath := t.TempDir() + "/test_notify.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	// Add a channel
	cc := ChannelConfig{
		Type:    ChannelNtfy,
		Enabled: true,
		URL:     "https://ntfy.sh/test-topic",
	}

	err = service.AddChannel("test-ntfy", cc)
	if err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	// Remove the channel
	err = service.RemoveChannel("test-ntfy")
	if err != nil {
		t.Fatalf("RemoveChannel() error = %v", err)
	}

	// Verify channel was removed
	channels := service.GetChannels()
	if len(channels) != 0 {
		t.Errorf("Got %d channels, want 0", len(channels))
	}
}

// TestSendImmediate tests sending an urgent notification immediately.
func TestSendImmediate(t *testing.T) {
	dbPath := t.TempDir() + "/test_notify.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	// Add a test channel (webhook that doesn't require external service)
	cc := ChannelConfig{
		Type:    ChannelWebhook,
		Enabled: true,
		URL:     "http://localhost:9999/test-webhook", // Will fail but that's OK for this test
	}

	err = service.AddChannel("test-webhook", cc)
	if err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	// Send urgent notification (should attempt immediate send)
	notif := Notification{
		Type:     TypeFallDetected,
		Title:    "Test Fall",
		Body:     "Test fall detected",
		Priority: PriorityUrgent,
		Tags:     []string{"test", "fall"},
	}

	err = service.Send(notif)
	// Will fail because webhook server doesn't exist, but that's expected
	// We're just testing that it doesn't block or panic
	if err != nil {
		// Expected to fail due to no server
		t.Log("Send failed as expected (no webhook server):", err)
	}
}

// TestBatching tests the batching functionality.
func TestBatching(t *testing.T) {
	dbPath := t.TempDir() + "/test_notify.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	// Enable batching
	bc := BatchingConfig{
		Enabled:          true,
		BatchWindowSec:   1, // Short window for testing
		MaxBatchSize:     5,
		BatchLowPriority: true,
		BatchMedium:      true,
	}

	err = service.SetBatchingConfig(bc)
	if err != nil {
		t.Fatalf("SetBatchingConfig() error = %v", err)
	}

	// Add a test channel
	cc := ChannelConfig{
		Type:    ChannelWebhook,
		Enabled: true,
		URL:     "http://localhost:9999/test",
	}

	err = service.AddChannel("test", cc)
	if err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	// Send multiple low priority notifications
	for i := 0; i < 3; i++ {
		notif := Notification{
			Type:     TypeZoneEnter,
			Title:    "Test Notification",
			Body:     "Test message",
			Priority: PriorityLow,
		}
		err = service.Send(notif)
		if err != nil {
			// Expected to fail due to no server
			t.Logf("Send %d failed as expected: %v", i, err)
		}
	}

	// Wait for batch window to expire
	time.Sleep(2 * time.Second)

	// Notifications should have been flushed
}

// TestQuietHoursQueueing tests queuing during quiet hours.
func TestQuietHoursQueueing(t *testing.T) {
	dbPath := t.TempDir() + "/test_notify.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	// Set quiet hours to current time
	now := time.Now()
	qh := QuietHoursConfig{
		Enabled:       true,
		StartHour:     now.Hour(),
		StartMin:      0,
		EndHour:       (now.Hour() + 1) % 24,
		EndMin:        0,
		MorningDigest: true,
	}

	err = service.SetQuietHours(qh)
	if err != nil {
		t.Fatalf("SetQuietHours() error = %v", err)
	}

	// Add a test channel
	cc := ChannelConfig{
		Type:    ChannelWebhook,
		Enabled: true,
		URL:     "http://localhost:9999/test",
	}

	err = service.AddChannel("test", cc)
	if err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	// Send a low priority notification during quiet hours
	// It should be queued, not sent immediately
	notif := Notification{
		Type:     TypeZoneEnter,
		Title:    "Test During Quiet Hours",
		Body:     "This should be queued",
		Priority: PriorityLow,
	}

	err = service.Send(notif)
	if err != nil {
		// May fail due to no server, but shouldn't panic
		t.Log("Send result:", err)
	}

	// The notification should have been queued
	// We can't easily test this without exposing internal state
}

// TestMergeNotifications tests merging multiple notifications.
func TestMergeNotifications(t *testing.T) {
	dbPath := t.TempDir() + "/test_notify.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	// Create test notifications
	notifs := []Notification{
		{
			Type:     TypeZoneEnter,
			Title:    "Alice entered Kitchen",
			Body:     "Alice entered from Hallway",
			Priority: PriorityLow,
			PersonName: "Alice",
			ZoneName:   "Kitchen",
		},
		{
			Type:     TypeZoneLeave,
			Title:    "Bob left Living Room",
			Body:     "Bob left for Bedroom",
			Priority: PriorityLow,
			PersonName: "Bob",
			ZoneName:   "Living Room",
		},
		{
			Type:     TypeZoneEnter,
			Title:    "Charlie entered Hallway",
			Body:     "Charlie entered from Kitchen",
			Priority: PriorityLow,
			PersonName: "Charlie",
			ZoneName:   "Hallway",
		},
	}

	// Merge notifications
	merged := service.mergeNotifications(notifs)

	if merged.Title != "Activity Update" {
		t.Errorf("Title = %s, want 'Activity Update'", merged.Title)
	}

	if merged.Priority != PriorityLow {
		t.Errorf("Priority = %d, want %d", merged.Priority, PriorityLow)
	}

	// Check that all people and zones are captured
	if len(merged.Tags) == 0 {
		t.Error("No tags in merged notification")
	}
}

// TestGenerateFloorPlanThumbnail tests generating floor plan thumbnails.
func TestGenerateFloorPlanThumbnail(t *testing.T) {
	dbPath := t.TempDir() + "/test_notify.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	zones := []struct {
		ID, Name, Color string
		X, Y, W, D      float64
		Highlight      bool
	}{
		{
			ID:        "kitchen",
			Name:      "Kitchen",
			Color:     "#4fc3f7",
			X:         1.0,
			Y:         1.0,
			W:         3.0,
			D:         2.0,
			Highlight: true,
		},
	}

	people := []struct {
		Name, Color string
		X, Y, Z     float64
		Confidence float64
		IsFall     bool
	}{
		{
			Name:      "Alice",
			Color:     "#4488ff",
			X:         2.5,
			Y:         2.0,
			Z:         1.0,
			Confidence: 0.85,
			IsFall:    false,
		},
	}

	data, err := service.GenerateFloorPlanThumbnail(300, 300, zones, people)
	if err != nil {
		t.Fatalf("GenerateFloorPlanThumbnail() error = %v", err)
	}

	if len(data) == 0 {
		t.Fatal("GenerateFloorPlanThumbnail() returned empty data")
	}

	// Check PNG signature
	pngSig := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	if len(data) < 8 {
		t.Fatalf("Output too short: %d bytes", len(data))
	}

	for i, b := range pngSig {
		if data[i] != b {
			t.Errorf("Output does not appear to be PNG (byte %d = %d, want %d)", i, data[i], b)
		}
	}
}
