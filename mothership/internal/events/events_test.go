// Package events provides tests for the unified activity timeline.
package events

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// openTestDB creates an in-memory SQLite database for testing.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// Create the schema
	schema := `
	-- Events table
	CREATE TABLE IF NOT EXISTS events (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp_ms INTEGER NOT NULL,
		type        TEXT NOT NULL,
		zone        TEXT,
		person      TEXT,
		blob_id     INTEGER,
		detail_json TEXT,
		severity    TEXT NOT NULL DEFAULT 'info'
	);
	CREATE INDEX IF NOT EXISTS idx_events_ts ON events(timestamp_ms DESC);
	CREATE INDEX IF NOT EXISTS idx_events_type ON events(type);
	CREATE INDEX IF NOT EXISTS idx_events_zone ON events(zone);
	CREATE INDEX IF NOT EXISTS idx_events_person ON events(person);

	-- Events archive
	CREATE TABLE IF NOT EXISTS events_archive (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		type        TEXT NOT NULL,
		timestamp_ms INTEGER NOT NULL,
		zone        TEXT,
		person      TEXT,
		blob_id     INTEGER,
		detail_json TEXT,
		severity    TEXT NOT NULL DEFAULT 'info'
	);

	-- FTS5 table
	CREATE VIRTUAL TABLE IF NOT EXISTS events_fts USING fts5(
		type, zone, person, detail_json,
		content='events', content_rowid='id'
	);

	-- FTS5 triggers
	CREATE TRIGGER IF NOT EXISTS events_fts_insert AFTER INSERT ON events BEGIN
		INSERT INTO events_fts(rowid, type, zone, person, detail_json)
		VALUES (new.id, new.type, new.zone, new.person, new.detail_json);
	END;

	CREATE TRIGGER IF NOT EXISTS events_fts_delete AFTER DELETE ON events BEGIN
		INSERT INTO events_fts(events_fts, rowid, type, zone, person, detail_json)
		VALUES ('delete', old.id, old.type, old.zone, old.person, old.detail_json);
	END;

	CREATE TRIGGER IF NOT EXISTS events_fts_update AFTER UPDATE ON events BEGIN
		INSERT INTO events_fts(events_fts, rowid, type, zone, person, detail_json)
		VALUES ('delete', old.id, old.type, old.zone, old.person, old.detail_json);
		INSERT INTO events_fts(rowid, type, zone, person, detail_json)
		VALUES (new.id, new.type, new.zone, new.person, new.detail_json);
	END;
	`

	_, err = db.Exec(schema)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	return db
}

func TestInsertEvent(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	tests := []struct {
		name      string
		event     Event
		wantID    bool
		wantErr   bool
		checkFunc func(*testing.T, int64, Event)
	}{
		{
			name: "basic event",
			event: Event{
				Type:       EventTypeDetection,
				Zone:       "Kitchen",
				Person:     "Alice",
				DetailJSON: `{"motion":0.15}`,
			},
			wantID: true,
		},
		{
			name: "event with timestamp",
			event: Event{
				TimestampMs: 1710000000000,
				Type:       EventTypeZoneEntry,
				Zone:       "Living Room",
				Person:     "Bob",
				Severity:   SeverityInfo,
			},
			wantID: true,
		},
		{
			name: "event with blob ID",
			event: Event{
				Type:       EventTypeDetection,
				Zone:       "Hallway",
				BlobID:     42,
				DetailJSON: `{"confidence":0.85}`,
			},
			wantID: true,
		},
		{
			name: "alert event",
			event: Event{
				Type:       EventTypeFallAlert,
				Zone:       "Bathroom",
				Person:     "Alice",
				Severity:   SeverityAlert,
				DetailJSON: `{"z_velocity":-1.8}`,
			},
			wantID: true,
			checkFunc: func(t *testing.T, id int64, e Event) {
				// Verify the event was inserted with the alert severity
				var severity string
				err := db.QueryRow("SELECT severity FROM events WHERE id = ?", id).Scan(&severity)
				if err != nil {
					t.Fatalf("failed to query severity: %v", err)
				}
				if severity != string(SeverityAlert) {
					t.Errorf("got severity %q, want %q", severity, SeverityAlert)
				}
			},
		},
		{
			name: "event with default severity",
			event: Event{
				Type:       EventTypeSystem,
				DetailJSON: `{"message":"test"}`,
			},
			wantID: true,
			checkFunc: func(t *testing.T, id int64, e Event) {
				// Verify default severity is 'info'
				var severity string
				err := db.QueryRow("SELECT severity FROM events WHERE id = ?", id).Scan(&severity)
				if err != nil {
					t.Fatalf("failed to query severity: %v", err)
				}
				if severity != string(SeverityInfo) {
					t.Errorf("got severity %q, want %q", severity, SeverityInfo)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := InsertEvent(db, tt.event)
			if (err != nil) != tt.wantErr {
				t.Errorf("InsertEvent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantID && id == 0 {
				t.Error("InsertEvent() returned zero ID, want non-zero")
			}
			if tt.checkFunc != nil {
				tt.checkFunc(t, id, tt.event)
			}
		})
	}
}

func TestQueryEvents(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	// Insert test data
	now := time.Now().UnixMilli()
	testEvents := []Event{
		{Type: EventTypeDetection, Zone: "Kitchen", Person: "Alice", DetailJSON: `{"x":1.0}`, TimestampMs: now},
		{Type: EventTypeZoneEntry, Zone: "Living Room", Person: "Bob", DetailJSON: `{"from":"Hallway"}`, TimestampMs: now - 1000},
		{Type: EventTypeFallAlert, Zone: "Bathroom", Person: "Alice", Severity: SeverityAlert, DetailJSON: `{"z":0.3}`, TimestampMs: now - 2000},
		{Type: EventTypeSystem, Zone: "", Person: "", DetailJSON: `{"message":"test"}`, TimestampMs: now - 3000},
	}

	for _, e := range testEvents {
		_, err := InsertEvent(db, e)
		if err != nil {
			t.Fatalf("failed to insert test event: %v", err)
		}
	}

	tests := []struct {
		name        string
		params      QueryParams
		wantCount   int
		wantMore    bool
		checkTypes  []EventType
		checkZones  []string
		checkPerson string
	}{
		{
			name:      "default limit",
			params:    QueryParams{},
			wantCount: 4,
			wantMore:  false,
		},
		{
			name: "limit 2",
			params: QueryParams{
				Limit: 2,
			},
			wantCount: 2,
			wantMore:  true,
		},
		{
			name: "filter by type",
			params: QueryParams{
				Type:  EventTypeDetection,
				Limit: 10,
			},
			wantCount:  1,
			wantMore:   false,
			checkTypes: []EventType{EventTypeDetection},
		},
		{
			name: "filter by zone",
			params: QueryParams{
				Zone:  "Kitchen",
				Limit: 10,
			},
			wantCount:  1,
			wantMore:   false,
			checkZones: []string{"Kitchen"},
		},
		{
			name: "filter by person",
			params: QueryParams{
				Person: "Alice",
				Limit:  10,
			},
			wantCount:   2,
			wantMore:    false,
			checkPerson: "Alice",
		},
		{
			name: "filter by severity",
			params: QueryParams{
				Type:  EventTypeFallAlert,
				Limit: 10,
			},
			wantCount: 1,
		},
		{
			name: "before_id cursor",
			params: QueryParams{
				Limit:    2,
				BeforeID: 3, // Should return events with ID < 3
			},
			wantCount: 2,
			wantMore:  false,
		},
		{
			name: "after_id cursor",
			params: QueryParams{
				Limit:   2,
				AfterID: 2, // Should return events with ID > 2
			},
			wantCount: 2,
			wantMore:  false,
		},
		{
			name: "time range",
			params: QueryParams{
				AfterTime:  time.UnixMilli(now - 2500),
				BeforeTime: time.UnixMilli(now - 500),
				Limit:      10,
			},
			wantCount: 2, // Events at now-1000 and now-2000
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, nextCursor, hasMore, err := QueryEvents(db, tt.params)
			if err != nil {
				t.Fatalf("QueryEvents() error = %v", err)
			}

			if len(events) != tt.wantCount {
				t.Errorf("QueryEvents() returned %d events, want %d", len(events), tt.wantCount)
			}

			if hasMore != tt.wantMore {
				t.Errorf("QueryEvents() hasMore = %v, want %v", hasMore, tt.wantMore)
			}

			if tt.wantMore && nextCursor == "" {
				t.Error("QueryEvents() returned empty nextCursor when hasMore is true")
			}

			// Check type filter results
			if len(tt.checkTypes) > 0 {
				for i, e := range events {
					if i >= len(tt.checkTypes) {
						break
					}
					if e.Type != tt.checkTypes[i] {
						t.Errorf("events[%d].Type = %v, want %v", i, e.Type, tt.checkTypes[i])
					}
				}
			}

			// Check zone filter results
			if len(tt.checkZones) > 0 {
				for i, e := range events {
					if i >= len(tt.checkZones) {
						break
					}
					if e.Zone != tt.checkZones[i] {
						t.Errorf("events[%d].Zone = %v, want %v", i, e.Zone, tt.checkZones[i])
					}
				}
			}

			// Check person filter results
			if tt.checkPerson != "" {
				for _, e := range events {
					if e.Person != tt.checkPerson {
						t.Errorf("event has person %q, want %q", e.Person, tt.checkPerson)
					}
				}
			}
		})
	}
}

func TestQueryEventsFTS(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	// Insert test data with searchable content
	now := time.Now().UnixMilli()
	testEvents := []Event{
		{Type: EventTypeFallAlert, Zone: "Kitchen", Person: "Alice", DetailJSON: `{"message":"Alice fell in the kitchen"}`, TimestampMs: now},
		{Type: EventTypeDetection, Zone: "Living Room", Person: "Bob", DetailJSON: `{"message":"Bob walking in living room"}`, TimestampMs: now - 1000},
		{Type: EventTypeSystem, Zone: "", Person: "", DetailJSON: `{"message":"System started"}`, TimestampMs: now - 2000},
	}

	for _, e := range testEvents {
		_, err := InsertEvent(db, e)
		if err != nil {
			t.Fatalf("failed to insert test event: %v", err)
		}
	}

	// Give FTS5 a moment to process
	time.Sleep(10 * time.Millisecond)

	tests := []struct {
		name      string
		search    string
		wantCount int
	}{
		{
			name:      "search for Alice",
			search:    "Alice",
			wantCount: 1,
		},
		{
			name:      "search for kitchen",
			search:    "kitchen",
			wantCount: 1,
		},
		{
			name:      "search for walking",
			search:    "walking",
			wantCount: 1,
		},
		{
			name:      "search for living room",
			search:    "living room",
			wantCount: 1,
		},
		{
			name:      "search with OR",
			search:    "Alice OR Bob",
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, _, _, err := QueryEvents(db, QueryParams{
				SearchQuery: tt.search,
				Limit:       10,
			})
			if err != nil {
				t.Fatalf("QueryEvents() error = %v", err)
			}

			if len(events) != tt.wantCount {
				t.Errorf("QueryEvents() search=%q returned %d events, want %d", tt.search, len(events), tt.wantCount)
			}
		})
	}
}

func TestRunArchiveJob(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	now := time.Now().UnixMilli()
	oldCutoff := now - ArchiveDaysMs - 1000  // Older than archive threshold
	youngCutoff := now - ArchiveDaysMs + 1000 // Newer than archive threshold

	// Insert old events (should be archived)
	oldEvents := []Event{
		{Type: EventTypeDetection, Zone: "Kitchen", DetailJSON: `{"old":1}`, TimestampMs: oldCutoff},
		{Type: EventTypeZoneEntry, Zone: "Living Room", DetailJSON: `{"old":2}`, TimestampMs: oldCutoff - 1000},
	}

	// Insert new events (should NOT be archived)
	newEvents := []Event{
		{Type: EventTypeDetection, Zone: "Bedroom", DetailJSON: `{"new":1}`, TimestampMs: youngCutoff},
		{Type: EventTypeSystem, DetailJSON: `{"new":2}`, TimestampMs: now},
	}

	for _, e := range oldEvents {
		_, err := InsertEvent(db, e)
		if err != nil {
			t.Fatalf("failed to insert old event: %v", err)
		}
	}

	for _, e := range newEvents {
		_, err := InsertEvent(db, e)
		if err != nil {
			t.Fatalf("failed to insert new event: %v", err)
		}
	}

	// Verify initial state
	var countBefore int
	err := db.QueryRow("SELECT COUNT(*) FROM events").Scan(&countBefore)
	if err != nil {
		t.Fatalf("failed to count events before archive: %v", err)
	}
	if countBefore != 4 {
		t.Fatalf("expected 4 events before archive, got %d", countBefore)
	}

	// Run archive job
	err = RunArchiveJob(db)
	if err != nil {
		t.Fatalf("RunArchiveJob() error = %v", err)
	}

	// Verify events table now has only new events
	var countAfter int
	err = db.QueryRow("SELECT COUNT(*) FROM events").Scan(&countAfter)
	if err != nil {
		t.Fatalf("failed to count events after archive: %v", err)
	}
	if countAfter != 2 {
		t.Errorf("events table has %d rows after archive, want 2", countAfter)
	}

	// Verify archive table has the old events
	var archiveCount int
	err = db.QueryRow("SELECT COUNT(*) FROM events_archive").Scan(&archiveCount)
	if err != nil {
		t.Fatalf("failed to count archived events: %v", err)
	}
	if archiveCount != 2 {
		t.Errorf("events_archive table has %d rows, want 2", archiveCount)
	}

	// Verify FTS5 table was updated (old events removed)
	var ftsCount int
	err = db.QueryRow("SELECT COUNT(*) FROM events_fts").Scan(&ftsCount)
	if err != nil {
		t.Fatalf("failed to count FTS entries: %v", err)
	}
	if ftsCount != 2 {
		t.Errorf("events_fts table has %d rows after archive, want 2", ftsCount)
	}

	// Verify the remaining events are the new ones by type
	var types []string
	rows, err := db.Query("SELECT type FROM events ORDER BY timestamp_ms DESC")
	if err != nil {
		t.Fatalf("failed to query remaining events: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var eType string
		if err := rows.Scan(&eType); err != nil {
			t.Fatalf("failed to scan type: %v", err)
		}
		types = append(types, eType)
	}

	if len(types) != 2 {
		t.Fatalf("expected 2 remaining events, got %d", len(types))
	}
	// Should have one Detection event and one System event
	hasDetection := false
	hasSystem := false
	for _, t := range types {
		if t == string(EventTypeDetection) {
			hasDetection = true
		}
		if t == string(EventTypeSystem) {
			hasSystem = true
		}
	}
	if !hasDetection || !hasSystem {
		t.Errorf("remaining event types are %v, want Detection and System", types)
	}
}

func TestRunArchiveJobEmpty(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	// Run archive job on empty database
	err := RunArchiveJob(db)
	if err != nil {
		t.Fatalf("RunArchiveJob() error = %v", err)
	}

	// Verify nothing broke
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM events").Scan(&count)
	if err != nil {
		t.Fatalf("failed to count events: %v", err)
	}
	if count != 0 {
		t.Errorf("events table has %d rows, want 0", count)
	}
}

func TestGetEventByID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	// Insert a test event
	id, err := InsertEvent(db, Event{
		Type:       EventTypeDetection,
		Zone:       "Kitchen",
		Person:     "Alice",
		DetailJSON: `{"test":true}`,
		Severity:   SeverityInfo,
	})
	if err != nil {
		t.Fatalf("failed to insert event: %v", err)
	}

	tests := []struct {
		name    string
		id      int64
		wantErr bool
		check   func(*testing.T, *Event)
	}{
		{
			name: "existing event",
			id:   id,
			check: func(t *testing.T, e *Event) {
				if e.Type != EventTypeDetection {
					t.Errorf("Type = %v, want %v", e.Type, EventTypeDetection)
				}
				if e.Zone != "Kitchen" {
					t.Errorf("Zone = %q, want %q", e.Zone, "Kitchen")
				}
				if e.Person != "Alice" {
					t.Errorf("Person = %q, want %q", e.Person, "Alice")
				}
			},
		},
		{
			name:    "non-existent event",
			id:      99999,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := GetEventByID(db, tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetEventByID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, event)
			}
		})
	}
}

func TestInsertDetectionEvent(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	id, err := InsertDetectionEvent(db, "Kitchen", "Alice", 42, map[string]interface{}{
		"confidence": 0.85,
		"position": map[string]interface{}{
			"x": 1.2,
			"y": 3.4,
			"z": 0.9,
		},
	})
	if err != nil {
		t.Fatalf("InsertDetectionEvent() error = %v", err)
	}

	if id == 0 {
		t.Error("InsertDetectionEvent() returned zero ID")
	}

	// Verify the event was inserted correctly
	var e Event
	err = db.QueryRow("SELECT type, zone, person, blob_id, detail_json, severity FROM events WHERE id = ?", id).
		Scan(&e.Type, &e.Zone, &e.Person, &e.BlobID, &e.DetailJSON, &e.Severity)
	if err != nil {
		t.Fatalf("failed to query event: %v", err)
	}

	if e.Type != EventTypeDetection {
		t.Errorf("Type = %v, want %v", e.Type, EventTypeDetection)
	}
	if e.Zone != "Kitchen" {
		t.Errorf("Zone = %q, want %q", e.Zone, "Kitchen")
	}
	if e.Person != "Alice" {
		t.Errorf("Person = %q, want %q", e.Person, "Alice")
	}
	if e.BlobID != 42 {
		t.Errorf("BlobID = %d, want 42", e.BlobID)
	}
	if e.Severity != SeverityInfo {
		t.Errorf("Severity = %v, want %v", e.Severity, SeverityInfo)
	}
}

func TestInsertAlertEvent(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	id, err := InsertAlertEvent(db, EventTypeFallAlert, "Bathroom", "Alice", SeverityAlert, map[string]interface{}{
		"z_velocity": -1.8,
		"z":          0.3,
	})
	if err != nil {
		t.Fatalf("InsertAlertEvent() error = %v", err)
	}

	if id == 0 {
		t.Error("InsertAlertEvent() returned zero ID")
	}

	// Verify the event was inserted correctly
	var severity string
	err = db.QueryRow("SELECT severity FROM events WHERE id = ?", id).Scan(&severity)
	if err != nil {
		t.Fatalf("failed to query event: %v", err)
	}

	if severity != string(SeverityAlert) {
		t.Errorf("Severity = %q, want %q", severity, SeverityAlert)
	}
}

func TestInsertSystemEvent(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	id, err := InsertSystemEvent(db, "Mothership started", nil)
	if err != nil {
		t.Fatalf("InsertSystemEvent() error = %v", err)
	}

	if id == 0 {
		t.Error("InsertSystemEvent() returned zero ID")
	}

	// Verify the event was inserted correctly
	var eType string
	var detailJSON string
	err = db.QueryRow("SELECT type, detail_json FROM events WHERE id = ?", id).Scan(&eType, &detailJSON)
	if err != nil {
		t.Fatalf("failed to query event: %v", err)
	}

	if eType != string(EventTypeSystem) {
		t.Errorf("Type = %q, want %q", eType, EventTypeSystem)
	}
}

func TestStartArchiveScheduler(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	// Start the scheduler with a done channel
	done := make(chan struct{})
	defer close(done)

	// The scheduler should start without error
	StartArchiveScheduler(db, done)

	// Give the goroutine a moment to start
	time.Sleep(10 * time.Millisecond)

	// The test passes if we got here without panic
	// The scheduler will schedule for next 02:00, which is in the future
}

func TestStartArchiveScheduler_StopsOnDone(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	done := make(chan struct{})

	// Start the scheduler
	StartArchiveScheduler(db, done)

	// Give the goroutine a moment to start
	time.Sleep(10 * time.Millisecond)

	// Signal done - should stop the scheduler gracefully
	close(done)

	// Give the goroutine a moment to stop
	time.Sleep(10 * time.Millisecond)

	// Test passes if we got here without deadlock
}

