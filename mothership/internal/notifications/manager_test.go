package notifications

import (
	"testing"
	"time"
)

// TestNewManager tests creating a new notification manager.
func TestNewManager(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"

	m, err := New(Config{DBPath: dbPath})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer m.Close()

	if m == nil {
		t.Fatal("New() returned nil")
	}
}

// TestNewManagerErrors tests error cases for New.
func TestNewManagerErrors(t *testing.T) {
	t.Run("empty DBPath", func(t *testing.T) {
		_, err := New(Config{})
		if err == nil {
			t.Error("New() with empty DBPath should return error")
		}
	})
}

// TestConfigPersistence tests saving and loading configuration.
func TestConfigPersistence(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"

	m, err := New(Config{DBPath: dbPath})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer m.Close()

	// Set custom config
	cfg := NotificationConfig{
		Channel:          "test-channel",
		QuietFrom:        "23:00",
		QuietTo:          "06:00",
		QuietDaysBitmask: 0x1F, // Mon-Fri only
		MorningDigest:    false,
	}

	err = m.SetConfig(cfg)
	if err != nil {
		t.Fatalf("SetConfig() error = %v", err)
	}

	// Get config and verify
	retrieved := m.GetConfig()
	if retrieved.Channel != "test-channel" {
		t.Errorf("Channel = %s, want test-channel", retrieved.Channel)
	}
	if retrieved.QuietFrom != "23:00" {
		t.Errorf("QuietFrom = %s, want 23:00", retrieved.QuietFrom)
	}
	if retrieved.QuietTo != "06:00" {
		t.Errorf("QuietTo = %s, want 06:00", retrieved.QuietTo)
	}
	if retrieved.QuietDaysBitmask != 0x1F {
		t.Errorf("QuietDaysBitmask = %x, want 1f", retrieved.QuietDaysBitmask)
	}
	if retrieved.MorningDigest {
		t.Error("MorningDigest = true, want false")
	}
}

// TestPriorityString tests the Priority String method.
func TestPriorityString(t *testing.T) {
	tests := []struct {
		name     string
		priority Priority
		want     string
	}{
		{"LOW", Low, "LOW"},
		{"MEDIUM", Medium, "MEDIUM"},
		{"HIGH", High, "HIGH"},
		{"URGENT", Urgent, "URGENT"},
		{"UNKNOWN", Priority(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.priority.String(); got != tt.want {
				t.Errorf("Priority.String() = %s, want %s", got, tt.want)
			}
		})
	}
}

// TestNotifyUrgentImmediate tests that urgent notifications are sent immediately.
func TestNotifyUrgentImmediate(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"

	var receivedEvent Event
	m, err := New(Config{
		DBPath:       dbPath,
		SendCallback: func(e Event) { receivedEvent = e },
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer m.Close()

	event := Event{
		Type:     FallDetected,
		Priority: Urgent,
		Title:    "Fall Detected",
		Body:     "Fall detected in hallway",
	}

	err = m.Notify(event)
	if err != nil {
		t.Fatalf("Notify() error = %v", err)
	}

	// Verify callback was called
	if receivedEvent.Type != FallDetected {
		t.Errorf("received event Type = %s, want FallDetected", receivedEvent.Type)
	}
	if receivedEvent.Priority != Urgent {
		t.Errorf("received event Priority = %d, want Urgent", receivedEvent.Priority)
	}
	if receivedEvent.Title != "Fall Detected" {
		t.Errorf("received event Title = %s, want 'Fall Detected'", receivedEvent.Title)
	}
}

// TestNotifyHighImmediate tests that high priority notifications are sent immediately.
func TestNotifyHighImmediate(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"

	var receivedEvent Event
	m, err := New(Config{
		DBPath:       dbPath,
		SendCallback: func(e Event) { receivedEvent = e },
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer m.Close()

	event := Event{
		Type:     AnomalyAlert,
		Priority: High,
		Title:    "Anomaly Alert",
		Body:     "Unusual activity detected",
	}

	err = m.Notify(event)
	if err != nil {
		t.Fatalf("Notify() error = %v", err)
	}

	if receivedEvent.Type != AnomalyAlert {
		t.Errorf("received event Type = %s, want AnomalyAlert", receivedEvent.Type)
	}
}

// TestBatching tests that LOW and MEDIUM events are batched.
func TestBatching(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"

	var receivedEvents []Event
	m, err := New(Config{
		DBPath:       dbPath,
		BatchWindowSec: 1, // Short window for testing
		SendCallback: func(e Event) { receivedEvents = append(receivedEvents, e) },
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer m.Close()

	// Send multiple low priority events
	for i := 0; i < 3; i++ {
		event := Event{
			Type:     ZoneEnter,
			Priority: Low,
			Title:    "Test Event",
			Body:     "Test message",
		}
		err = m.Notify(event)
		if err != nil {
			t.Logf("Notify() %d error = %v", i, err)
		}
	}

	// Events should be queued, not sent immediately
	if len(receivedEvents) != 0 {
		t.Errorf("got %d events sent immediately, want 0", len(receivedEvents))
	}

	// Wait for batch window
	time.Sleep(1500 * time.Millisecond)

	// Check pending count is now 0
	low, medium, _ := m.GetPendingCount()
	if low != 0 {
		t.Errorf("pendingLow = %d, want 0", low)
	}
	if medium != 0 {
		t.Errorf("pendingMedium = %d, want 0", medium)
	}

	// Should have received a summary event
	if len(receivedEvents) == 0 {
		t.Fatal("no events received after batch window")
	}

	summary := receivedEvents[0]
	if summary.Data["is_batch"] != true {
		t.Error("received event is not marked as batch")
	}
}

// TestBatchMaxSize tests that batch is flushed when max size is reached.
func TestBatchMaxSize(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"

	var receivedEvents []Event
	m, err := New(Config{
		DBPath:       dbPath,
		MaxBatchSize: 3,
		SendCallback: func(e Event) { receivedEvents = append(receivedEvents, e) },
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer m.Close()

	// Send exactly max batch size events
	for i := 0; i < 3; i++ {
		event := Event{
			Type:     ZoneEnter,
			Priority: Low,
			Title:    "Test Event",
			Body:     "Test message",
		}
		err = m.Notify(event)
		if err != nil {
			t.Logf("Notify() %d error = %v", i, err)
		}
	}

	// Should have triggered immediate flush due to max batch size
	// Give a moment for the goroutine to run
	time.Sleep(100 * time.Millisecond)

	if len(receivedEvents) == 0 {
		t.Error("no events received after max batch size reached")
	}
}

// TestQuietHoursQueueing tests that LOW/MEDIUM events are queued during quiet hours.
func TestQuietHoursQueueing(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"

	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("Skipping test: cannot load timezone")
	}

	m, err := New(Config{
		DBPath:       dbPath,
		Location:     loc,
		SendCallback: func(e Event) {},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer m.Close()

	// Set quiet hours to cover current time
	now := time.Now().In(loc)
	cfg := NotificationConfig{
		Channel:          "default",
		QuietFrom:        "00:00",
		QuietTo:          "23:59",
		QuietDaysBitmask: 0xFF, // All days
		MorningDigest:    true,
	}

	err = m.SetConfig(cfg)
	if err != nil {
		t.Fatalf("SetConfig() error = %v", err)
	}

	// Send low priority event during quiet hours
	event := Event{
		Type:     ZoneEnter,
		Priority: Low,
		Title:    "Test Event",
		Body:     "Should be queued",
	}

	err = m.Notify(event)
	if err != nil {
		t.Fatalf("Notify() error = %v", err)
	}

	// Verify it was queued for digest
	low, medium, digest := m.GetPendingCount()
	if low != 0 {
		t.Errorf("pendingLow = %d, want 0 (should be in digest queue)", low)
	}
	if medium != 0 {
		t.Errorf("pendingMedium = %d, want 0", medium)
	}
	if digest != 1 {
		t.Errorf("queuedForDigest = %d, want 1", digest)
	}
}

// TestQuietHoursHighPriority tests that HIGH events are sent immediately even during quiet hours.
func TestQuietHoursHighPriority(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"

	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("Skipping test: cannot load timezone")
	}

	var receivedEvent Event
	m, err := New(Config{
		DBPath:       dbPath,
		Location:     loc,
		SendCallback: func(e Event) { receivedEvent = e },
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer m.Close()

	// Set quiet hours to cover current time
	cfg := NotificationConfig{
		Channel:          "default",
		QuietFrom:        "00:00",
		QuietTo:          "23:59",
		QuietDaysBitmask: 0xFF,
		MorningDigest:    true,
	}

	err = m.SetConfig(cfg)
	if err != nil {
		t.Fatalf("SetConfig() error = %v", err)
	}

	// Send high priority event during quiet hours
	event := Event{
		Type:     FallDetected,
		Priority: High,
		Title:    "High Priority Event",
		Body:     "Should bypass quiet hours",
	}

	err = m.Notify(event)
	if err != nil {
		t.Fatalf("Notify() error = %v", err)
	}

	// Verify it was sent immediately
	if receivedEvent.Type != FallDetected {
		t.Errorf("received event Type = %s, want FallDetected", receivedEvent.Type)
	}
	if receivedEvent.Priority != High {
		t.Errorf("received event Priority = %d, want High", receivedEvent.Priority)
	}

	// Should not be in any queue
	low, medium, digest := m.GetPendingCount()
	if low != 0 || medium != 0 || digest != 0 {
		t.Errorf("Events queued (low=%d, medium=%d, digest=%d), want all 0", low, medium, digest)
	}
}

// TestUrgentBypassesAll tests that URGENT events bypass both batching and quiet hours.
func TestUrgentBypassesAll(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"

	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("Skipping test: cannot load timezone")
	}

	var receivedEvent Event
	m, err := New(Config{
		DBPath:       dbPath,
		Location:     loc,
		SendCallback: func(e Event) { receivedEvent = e },
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer m.Close()

	// Set quiet hours and enable batching
	cfg := NotificationConfig{
		Channel:          "default",
		QuietFrom:        "00:00",
		QuietTo:          "23:59",
		QuietDaysBitmask: 0xFF,
		MorningDigest:    true,
	}
	err = m.SetConfig(cfg)
	if err != nil {
		t.Fatalf("SetConfig() error = %v", err)
	}

	// Send urgent event during quiet hours
	event := Event{
		Type:     FallEscalation,
		Priority: Urgent,
		Title:    "URGENT: Fall Escalation",
		Body:     "Immediate action required",
	}

	err = m.Notify(event)
	if err != nil {
		t.Fatalf("Notify() error = %v", err)
	}

	// Verify it was sent immediately
	if receivedEvent.Type != FallEscalation {
		t.Errorf("received event Type = %s, want FallEscalation", receivedEvent.Type)
	}
	if receivedEvent.Priority != Urgent {
		t.Errorf("received event Priority = %d, want Urgent", receivedEvent.Priority)
	}

	// Should not be in any queue
	low, medium, digest := m.GetPendingCount()
	if low != 0 || medium != 0 || digest != 0 {
		t.Errorf("Events queued (low=%d, medium=%d, digest=%d), want all 0", low, medium, digest)
	}
}

// TestQuietDaysBitmask tests that quiet hours only apply on specified days.
func TestQuietDaysBitmask(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"

	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("Skipping test: cannot load timezone")
	}

	now := time.Now().In(loc)
	currentDayMask := uint8(1 << now.Weekday())

	m, err := New(Config{
		DBPath:   dbPath,
		Location: loc,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer m.Close()

	// Set quiet hours for only the current day
	cfg := NotificationConfig{
		Channel:          "default",
		QuietFrom:        "00:00",
		QuietTo:          "23:59",
		QuietDaysBitmask: currentDayMask,
		MorningDigest:    true,
	}

	err = m.SetConfig(cfg)
	if err != nil {
		t.Fatalf("SetConfig() error = %v", err)
	}

	// Send low priority event
	event := Event{
		Type:     ZoneEnter,
		Priority: Low,
		Title:    "Test Event",
		Body:     "Should be queued",
	}

	err = m.Notify(event)
	if err != nil {
		t.Fatalf("Notify() error = %v", err)
	}

	// Should be queued (current day is in quiet hours)
	low, medium, digest := m.GetPendingCount()
	if digest != 1 {
		t.Errorf("queuedForDigest = %d, want 1 (current day should be in quiet hours)", digest)
	}
}

// TestMediumPriorityBatching tests that MEDIUM priority events are batched.
func TestMediumPriorityBatching(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"

	var receivedEvents []Event
	m, err := New(Config{
		DBPath:       dbPath,
		BatchWindowSec: 1,
		SendCallback: func(e Event) { receivedEvents = append(receivedEvents, e) },
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer m.Close()

	// Send medium priority events
	for i := 0; i < 2; i++ {
		event := Event{
			Type:     ZoneLeave,
			Priority: Medium,
			Title:    "Medium Event",
			Body:     "Test message",
		}
		err = m.Notify(event)
		if err != nil {
			t.Logf("Notify() %d error = %v", i, err)
		}
	}

	// Events should be queued
	low, medium, _ := m.GetPendingCount()
	if medium != 2 {
		t.Errorf("pendingMedium = %d, want 2", medium)
	}

	// Wait for batch window
	time.Sleep(1500 * time.Millisecond)

	// Should be flushed now
	low, medium, _ = m.GetPendingCount()
	if medium != 0 {
		t.Errorf("pendingMedium = %d, want 0 after flush", medium)
	}
}

// TestGetHistory tests retrieving notification history.
func TestGetHistory(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"

	m, err := New(Config{
		DBPath:       dbPath,
		SendCallback: func(e Event) {},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer m.Close()

	// Send some events
	events := []Event{
		{Type: ZoneEnter, Priority: Low, Title: "Event 1", Body: "Body 1"},
		{Type: ZoneLeave, Priority: Medium, Title: "Event 2", Body: "Body 2"},
		{Type: FallDetected, Priority: Urgent, Title: "Event 3", Body: "Body 3"},
	}

	for _, e := range events {
		if err := m.Notify(e); err != nil {
			t.Fatalf("Notify() error = %v", err)
		}
	}

	// Get history
	history, err := m.GetHistory(10)
	if err != nil {
		t.Fatalf("GetHistory() error = %v", err)
	}

	if len(history) != 3 {
		t.Fatalf("GetHistory() returned %d events, want 3", len(history))
	}

	// Verify events (should be in reverse chronological order)
	if history[0]["title"] != "Event 3" {
		t.Errorf("history[0].title = %s, want Event 3", history[0]["title"])
	}
	if history[2]["title"] != "Event 1" {
		t.Errorf("history[2].title = %s, want Event 1", history[2]["title"])
	}

	// Check that urgent event was not batched or queued
	if history[0]["was_batched"] != false {
		t.Error("Urgent event was marked as batched")
	}
	if history[0]["was_queued"] != false {
		t.Error("Urgent event was marked as queued")
	}
}

// TestFlush tests manually flushing pending notifications.
func TestFlush(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"

	var receivedEvents []Event
	m, err := New(Config{
		DBPath:       dbPath,
		SendCallback: func(e Event) { receivedEvents = append(receivedEvents, e) },
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer m.Close()

	// Set quiet hours to queue events
	cfg := NotificationConfig{
		Channel:          "default",
		QuietFrom:        "00:00",
		QuietTo:          "23:59",
		QuietDaysBitmask: 0xFF,
		MorningDigest:    true,
	}
	m.SetConfig(cfg)

	// Send some events
	for i := 0; i < 3; i++ {
		event := Event{
			Type:     ZoneEnter,
			Priority: Low,
			Title:    "Test Event",
			Body:     "Should be queued",
		}
		m.Notify(event)
	}

	// Verify events are queued
	low, medium, digest := m.GetPendingCount()
	if digest != 3 {
		t.Errorf("queuedForDigest = %d, want 3", digest)
	}

	// Flush
	m.Flush()

	// Verify events were sent
	if len(receivedEvents) != 3 {
		t.Errorf("received %d events, want 3", len(receivedEvents))
	}

	// Verify queues are empty
	low, medium, digest = m.GetPendingCount()
	if low != 0 || medium != 0 || digest != 0 {
		t.Errorf("Queues not empty after flush: low=%d, medium=%d, digest=%d", low, medium, digest)
	}
}

// TestCreateSummary tests the summary creation logic.
func TestCreateSummary(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"

	m, err := New(Config{DBPath: dbPath})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer m.Close()

	events := []*Event{
		{Type: ZoneEnter, Priority: Low, Title: "Event 1"},
		{Type: ZoneEnter, Priority: Low, Title: "Event 2"},
		{Type: ZoneLeave, Priority: Low, Title: "Event 3"},
		{Type: ZoneVacant, Priority: Low, Title: "Event 4"},
		{Type: NodeOffline, Priority: Low, Title: "Event 5"},
	}

	summary := m.createSummary(events)

	if summary.Data["is_batch"] != true {
		t.Error("Summary not marked as batch")
	}

	eventCount, ok := summary.Data["event_count"].(int)
	if !ok || eventCount != 5 {
		t.Errorf("event_count = %v, want 5", summary.Data["event_count"])
	}

	// Check title contains count
	if summary.Title != "Activity Update: 5 events" {
		t.Errorf("Title = %s, want 'Activity Update: 5 events'", summary.Title)
	}
}

// TestSingleEventBatch tests that a single event in a batch is sent directly.
func TestSingleEventBatch(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"

	var receivedEvent Event
	m, err := New(Config{
		DBPath:       dbPath,
		BatchWindowSec: 1,
		SendCallback: func(e Event) { receivedEvent = e },
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer m.Close()

	event := Event{
		Type:     ZoneEnter,
		Priority: Low,
		Title:    "Single Event",
		Body:     "Only one event",
	}

	m.Notify(event)
	time.Sleep(1500 * time.Millisecond)

	// Single event should be sent as-is, not as a summary
	if receivedEvent.Title != "Single Event" {
		t.Errorf("Title = %s, want 'Single Event'", receivedEvent.Title)
	}

	if receivedEvent.Data["is_batch"] == true {
		t.Error("Single event was marked as batch")
	}
}

// TestNotifyWithTimestamp tests that events with timestamps preserve them.
func TestNotifyWithTimestamp(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"

	var receivedEvent Event
	m, err := New(Config{
		DBPath:       dbPath,
		SendCallback: func(e Event) { receivedEvent = e },
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer m.Close()

	expectedTime := time.Date(2024, 4, 10, 12, 30, 0, 0, time.UTC)
	event := Event{
		Type:     ZoneEnter,
		Priority: Urgent,
		Title:    "Test",
		Body:     "Test",
		Timestamp: expectedTime,
	}

	m.Notify(event)

	if !receivedEvent.Timestamp.Equal(expectedTime) {
		t.Errorf("Timestamp = %v, want %v", receivedEvent.Timestamp, expectedTime)
	}
}

// TestGetPendingCount tests getting pending notification counts.
func TestGetPendingCount(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"

	m, err := New(Config{
		DBPath: dbPath,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer m.Close()

	// Initially all zeros
	low, medium, digest := m.GetPendingCount()
	if low != 0 || medium != 0 || digest != 0 {
		t.Errorf("Initial counts not zero: low=%d, medium=%d, digest=%d", low, medium, digest)
	}

	// Set quiet hours to queue events
	cfg := NotificationConfig{
		Channel:          "default",
		QuietFrom:        "00:00",
		QuietTo:          "23:59",
		QuietDaysBitmask: 0xFF,
		MorningDigest:    true,
	}
	m.SetConfig(cfg)

	// Add events to each queue
	m.Notify(Event{Type: ZoneEnter, Priority: Low, Title: "Low"})
	m.Notify(Event{Type: ZoneLeave, Priority: Medium, Title: "Medium"})

	low, medium, digest = m.GetPendingCount()
	if low != 0 {
		t.Errorf("pendingLow = %d, want 0 (low priority goes to digest during quiet hours)", low)
	}
	if medium != 0 {
		t.Errorf("pendingMedium = %d, want 0 (medium priority goes to digest during quiet hours)", medium)
	}
	if digest != 2 {
		t.Errorf("queuedForDigest = %d, want 2", digest)
	}
}

// TestSetAndGetConfig tests getting and setting configuration.
func TestSetAndGetConfig(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"

	m, err := New(Config{DBPath: dbPath})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer m.Close()

	// Test default config
	defaultCfg := m.GetConfig()
	if defaultCfg.Channel != "default" {
		t.Errorf("Default Channel = %s, want 'default'", defaultCfg.Channel)
	}
	if defaultCfg.QuietFrom != "22:00" {
		t.Errorf("Default QuietFrom = %s, want '22:00'", defaultCfg.QuietFrom)
	}
	if defaultCfg.QuietTo != "07:00" {
		t.Errorf("Default QuietTo = %s, want '07:00'", defaultCfg.QuietTo)
	}

	// Set custom config
	customCfg := NotificationConfig{
		Channel:          "custom",
		QuietFrom:        "21:30",
		QuietTo:          "08:30",
		QuietDaysBitmask: 0x07, // Sun-Tue only
		MorningDigest:    false,
	}

	err = m.SetConfig(customCfg)
	if err != nil {
		t.Fatalf("SetConfig() error = %v", err)
	}

	retrieved := m.GetConfig()
	if retrieved.Channel != "custom" {
		t.Errorf("Channel = %s, want 'custom'", retrieved.Channel)
	}
	if retrieved.QuietFrom != "21:30" {
		t.Errorf("QuietFrom = %s, want '21:30'", retrieved.QuietFrom)
	}
	if retrieved.QuietTo != "08:30" {
		t.Errorf("QuietTo = %s, want '08:30'", retrieved.QuietTo)
	}
	if retrieved.QuietDaysBitmask != 0x07 {
		t.Errorf("QuietDaysBitmask = %x, want 07", retrieved.QuietDaysBitmask)
	}
	if retrieved.MorningDigest {
		t.Error("MorningDigest = true, want false")
	}
}

// TestEventTypes tests all event type constants.
func TestEventTypes(t *testing.T) {
	types := []EventType{
		ZoneEnter,
		ZoneLeave,
		ZoneVacant,
		FallDetected,
		FallEscalation,
		AnomalyAlert,
		NodeOffline,
		SleepSummary,
	}

	for _, tpe := range types {
		if tpe == "" {
			t.Errorf("Event type %v is empty", tpe)
		}
	}
}

// TestSetSendCallback tests setting the send callback.
func TestSetSendCallback(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"

	var callbackCalled bool
	m, err := New(Config{
		DBPath: dbPath,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer m.Close()

	// Set callback after creation
	m.SetSendCallback(func(e Event) {
		callbackCalled = true
	})

	event := Event{
		Type:     ZoneEnter,
		Priority: Urgent,
		Title:    "Test",
		Body:     "Test",
	}

	m.Notify(event)

	if !callbackCalled {
		t.Error("Callback was not called")
	}
}
