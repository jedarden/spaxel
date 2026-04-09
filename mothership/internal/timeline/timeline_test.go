// Package timeline provides tests for the timeline event storage system.
package timeline

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/spaxel/mothership/internal/eventbus"
	_ "modernc.org/sqlite"
)

func TestTimelineStorage(t *testing.T) {
	t.Run("BasicStorage", func(t *testing.T) {
		db := setupTestDB(t)
		storage := New(db)
		defer storage.Close()

		// Publish an event
		eventbus.PublishDefault(eventbus.Event{
			Type:        eventbus.TypeDetection,
			TimestampMs: time.Now().UnixMilli(),
			Zone:        "Kitchen",
			Person:      "Alice",
			BlobID:      123,
			Detail:      map[string]interface{}{"confidence": 0.85},
			Severity:    eventbus.SeverityInfo,
		})

		// Wait for flush (max 1 second)
		time.Sleep(200 * time.Millisecond)

		// Verify event was stored
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM events").Scan(&count)
		if err != nil {
			t.Fatalf("Query event count: %v", err)
		}
		if count != 1 {
			t.Errorf("Event count = %d, want 1", count)
		}

		// Verify event details
		var eventType, zone, person string
		var blobID int
		var detailJSON string
		err = db.QueryRow(`
			SELECT type, zone, person, blob_id, detail_json
			FROM events LIMIT 1
		`).Scan(&eventType, &zone, &person, &blobID, &detailJSON)
		if err != nil {
			t.Fatalf("Query event: %v", err)
		}

		if eventType != eventbus.TypeDetection {
			t.Errorf("Event type = %s, want %s", eventType, eventbus.TypeDetection)
		}
		if zone != "Kitchen" {
			t.Errorf("Zone = %s, want Kitchen", zone)
		}
		if person != "Alice" {
			t.Errorf("Person = %s, want Alice", person)
		}
		if blobID != 123 {
			t.Errorf("BlobID = %d, want 123", blobID)
		}

		// Verify detail JSON
		var detail map[string]interface{}
		if err := json.Unmarshal([]byte(detailJSON), &detail); err != nil {
			t.Fatalf("Unmarshal detail JSON: %v", err)
		}
		if conf, ok := detail["confidence"]; !ok || conf.(float64) != 0.85 {
			t.Errorf("Detail confidence = %v, want 0.85", conf)
		}
	})

	t.Run("NeverBlocksPublishers", func(t *testing.T) {
		db := setupTestDB(t)
		storage := New(db)
		defer storage.Close()

		// Publish many events rapidly - should never block
		done := make(chan bool)
		go func() {
			for i := 0; i < 10000; i++ {
				eventbus.PublishDefault(eventbus.Event{
					Type:        eventbus.TypeDetection,
					TimestampMs: time.Now().UnixMilli(),
					Zone:        "Test",
				})
			}
			done <- true
		}()

		select {
		case <-done:
			// All events published successfully
		case <-time.After(1 * time.Second):
			t.Error("Publishing blocked - took more than 1 second for 10000 events")
		}
	})

	t.Run("DropOldestOnOverflow", func(t *testing.T) {
		db := setupTestDB(t)
		storage := New(db)
		defer storage.Close()

		// Publish more events than queue size
		for i := 0; i < queueSize+100; i++ {
			eventbus.PublishDefault(eventbus.Event{
				Type:        eventbus.TypeDetection,
				TimestampMs: time.Now().UnixMilli(),
				Zone:        "Test",
				Person:      "TestPerson",
			})
		}

		// Wait for flush
		time.Sleep(200 * time.Millisecond)

		// Check that some events were dropped
		stats := storage.Stats()
		if stats.Dropped == 0 {
			t.Error("Expected some events to be dropped, but Dropped = 0")
		}

		// Verify that at least queueSize events were stored
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM events").Scan(&count)
		if err != nil {
			t.Fatalf("Query event count: %v", err)
		}
		if count < queueSize {
			t.Errorf("Event count = %d, want at least %d", count, queueSize)
		}
	})

	t.Run("AllEventTypesStored", func(t *testing.T) {
		db := setupTestDB(t)
		storage := New(db)
		defer storage.Close()

		eventTypes := []string{
			eventbus.TypeDetection,
			eventbus.TypeZoneEntry,
			eventbus.TypeZoneExit,
			eventbus.TypePortalCrossing,
			eventbus.TypeTriggerFired,
			eventbus.TypeFallAlert,
			eventbus.TypeAnomaly,
			eventbus.TypeSecurityAlert,
			eventbus.TypeNodeOnline,
			eventbus.TypeNodeOffline,
			eventbus.TypeOTAUpdate,
			eventbus.TypeBaselineChanged,
			eventbus.TypeSystem,
			eventbus.TypeLearningMilestone,
		}

		// Publish one of each type
		for _, eventType := range eventTypes {
			eventbus.PublishDefault(eventbus.Event{
				Type:        eventType,
				TimestampMs: time.Now().UnixMilli(),
				Zone:        "TestZone",
				Person:      "TestPerson",
			})
		}

		// Wait for flush
		time.Sleep(200 * time.Millisecond)

		// Verify all event types were stored
		rows, err := db.Query("SELECT DISTINCT type FROM events ORDER BY type")
		if err != nil {
			t.Fatalf("Query event types: %v", err)
		}
		defer rows.Close()

		var storedTypes []string
		for rows.Next() {
			var eventType string
			if err := rows.Scan(&eventType); err != nil {
				t.Fatalf("Scan event type: %v", err)
			}
			storedTypes = append(storedTypes, eventType)
		}

		if len(storedTypes) != len(eventTypes) {
			t.Errorf("Stored %d event types, want %d", len(storedTypes), len(eventTypes))
		}
	})

	t.Run("EventsStoredWithinOneSecond", func(t *testing.T) {
		db := setupTestDB(t)
		storage := New(db)
		defer storage.Close()

		// Publish an event
		publishTime := time.Now()
		eventbus.PublishDefault(eventbus.Event{
			Type:        eventbus.TypeDetection,
			TimestampMs: publishTime.UnixMilli(),
			Zone:        "Kitchen",
		})

		// Poll for event to be stored
		var count int
		var stored bool
		for i := 0; i < 100; i++ { // Check for up to 1 second
			err := db.QueryRow("SELECT COUNT(*) FROM events").Scan(&count)
			if err != nil {
				t.Fatalf("Query event count: %v", err)
			}
			if count > 0 {
				stored = true
				break
			}
			time.Sleep(10 * time.Millisecond)
		}

		elapsed := time.Since(publishTime)
		if !stored {
			t.Error("Event was not stored within 1 second")
		}
		if elapsed > 1*time.Second {
			t.Errorf("Event stored in %v, want < 1 second", elapsed)
		}
	})

	t.Run("StringDetailHandling", func(t *testing.T) {
		db := setupTestDB(t)
		storage := New(db)
		defer storage.Close()

		// Publish event with string detail (from eventbus)
		detailJSON := `{"message":"test message","value":42}`
		eventbus.PublishDefault(eventbus.Event{
			Type:        eventbus.TypeSystem,
			TimestampMs: time.Now().UnixMilli(),
			Detail:      detailJSON,
			Severity:    eventbus.SeverityInfo,
		})

		// Wait for flush
		time.Sleep(200 * time.Millisecond)

		// Verify detail was stored correctly
		var storedDetail string
		err := db.QueryRow("SELECT detail_json FROM events LIMIT 1").Scan(&storedDetail)
		if err != nil {
			t.Fatalf("Query detail: %v", err)
		}

		if storedDetail != detailJSON {
			t.Errorf("Detail = %s, want %s", storedDetail, detailJSON)
		}
	})

	t.Run("CloseFlushesRemainingEvents", func(t *testing.T) {
		db := setupTestDB(t)
		storage := New(db)

		// Publish some events
		for i := 0; i < 50; i++ {
			eventbus.PublishDefault(eventbus.Event{
				Type:        eventbus.TypeDetection,
				TimestampMs: time.Now().UnixMilli(),
				Zone:        "Test",
			})
		}

		// Close should flush all events
		if err := storage.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}

		// Verify all events were stored
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM events").Scan(&count)
		if err != nil {
			t.Fatalf("Query event count: %v", err)
		}
		if count != 50 {
			t.Errorf("Event count = %d, want 50", count)
		}
	})

	t.Run("Stats", func(t *testing.T) {
		db := setupTestDB(t)
		storage := New(db)
		defer storage.Close()

		// Initial stats
		stats := storage.Stats()
		if stats.Queued < 0 || stats.Queued > queueSize {
			t.Errorf("Queued = %d, want between 0 and %d", stats.Queued, queueSize)
		}

		// Publish events to fill queue
		for i := 0; i < queueSize+10; i++ {
			eventbus.PublishDefault(eventbus.Event{
				Type:        eventbus.TypeDetection,
				TimestampMs: time.Now().UnixMilli(),
			})
		}

		// Wait for some to flush
		time.Sleep(50 * time.Millisecond)

		stats = storage.Stats()
		if stats.Dropped == 0 {
			t.Error("Expected events to be dropped when queue overflows")
		}
	})

	t.Run("ConcurrentWrites", func(t *testing.T) {
		db := setupTestDB(t)
		storage := New(db)
		defer storage.Close()

		// Publish events from multiple goroutines
		const numGoroutines = 10
		const eventsPerGoroutine = 100

		done := make(chan bool, numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func() {
				for j := 0; j < eventsPerGoroutine; j++ {
					eventbus.PublishDefault(eventbus.Event{
						Type:        eventbus.TypeDetection,
						TimestampMs: time.Now().UnixMilli(),
						Zone:        "Test",
					})
				}
				done <- true
			}()
		}

		// Wait for all goroutines to complete
		for i := 0; i < numGoroutines; i++ {
			<-done
		}

		// Wait for all events to flush
		time.Sleep(500 * time.Millisecond)

		// Verify events were stored (may have dropped some due to queue size)
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM events").Scan(&count)
		if err != nil {
			t.Fatalf("Query event count: %v", err)
		}
		if count == 0 {
			t.Error("No events stored, expected at least some")
		}
	})
}

func TestTimelineStorageIndexes(t *testing.T) {
	db := setupTestDB(t)

	// Verify all required indexes exist
	indexes := []string{
		"idx_events_time",
		"idx_events_zone",
		"idx_events_person",
		"idx_events_type",
	}

	for _, idx := range indexes {
		var name string
		err := db.QueryRow(`
			SELECT name FROM sqlite_master WHERE type='index' AND name=?
		`, idx).Scan(&name)
		if err == sql.ErrNoRows {
			t.Errorf("Index %s does not exist", idx)
		} else if err != nil {
			t.Fatalf("Check index %s: %v", idx, err)
		}
	}
}

// setupTestDB creates an in-memory database for testing.
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Open database: %v", err)
	}

	// Create the events table
	_, err = db.Exec(`
		CREATE TABLE events (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp_ms INTEGER NOT NULL,
			type        TEXT NOT NULL,
			zone        TEXT,
			person      TEXT,
			blob_id     INTEGER,
			detail_json TEXT,
			severity    TEXT NOT NULL DEFAULT 'info'
		);
		CREATE INDEX idx_events_time ON events(timestamp_ms DESC);
		CREATE INDEX idx_events_zone ON events(zone, timestamp_ms DESC);
		CREATE INDEX idx_events_person ON events(person, timestamp_ms DESC);
		CREATE INDEX idx_events_type ON events(type, timestamp_ms DESC);
	`)
	if err != nil {
		t.Fatalf("Create tables: %v", err)
	}

	return db
}
