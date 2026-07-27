// Package ota provides tests for auto-update API handlers.
package ota

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/spaxel/mothership/internal/events"
)

// setupTestDB creates a test database with the events schema.
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// Create events schema
	schema := `
	CREATE TABLE IF NOT EXISTS events (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp_ms INTEGER NOT NULL,
		type        TEXT    NOT NULL,
		zone        TEXT,
		person      TEXT,
		blob_id     INTEGER,
		detail_json TEXT,
		severity    TEXT    NOT NULL DEFAULT 'info'
	);
	CREATE INDEX IF NOT EXISTS idx_events_time ON events(timestamp_ms DESC);
	CREATE INDEX IF NOT EXISTS idx_events_type ON events(type, timestamp_ms DESC);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create events schema: %v", err)
	}

	return db
}

// insertOTAEvent inserts a test OTA event into the database.
func insertOTAEvent(t *testing.T, db *sql.DB, eventType, mac, message string, metadata map[string]interface{}, timestamp time.Time) int64 {
	t.Helper()

	detail := map[string]interface{}{
		"ota_event": eventType,
		"mac":       mac,
		"message":   message,
		"metadata":  metadata,
	}

	detailJSON, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("failed to marshal detail: %v", err)
	}

	result, err := db.Exec(`
		INSERT INTO events (timestamp_ms, type, zone, person, blob_id, detail_json, severity)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, timestamp.UnixMilli(), events.EventTypeOTAUpdate, "", "", 0, string(detailJSON), "info")
	if err != nil {
		t.Fatalf("failed to insert OTA event: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("failed to get last insert id: %v", err)
	}

	return id
}

// TestHandleHistory_EmptyDatabase tests that handleHistory returns empty history when no events exist.
func TestHandleHistory_EmptyDatabase(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	handler := &AutoAPIHandler{
		db:       db,
		timezone: time.UTC,
	}

	req := httptest.NewRequest("GET", "/api/ota/auto/history", nil)
	w := httptest.NewRecorder()

	handler.handleHistory(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	history, ok := response["history"].([]interface{})
	if !ok {
		t.Fatal("response history field is not an array")
	}

	if len(history) != 0 {
		t.Errorf("expected empty history, got %d events", len(history))
	}

	hasMore, ok := response["has_more"].(bool)
	if !ok || hasMore {
		t.Error("expected has_more to be false for empty history")
	}
}

// TestHandleHistory_WithEvents tests that handleHistory returns OTA events in correct order.
func TestHandleHistory_WithEvents(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	now := time.Now().UTC()

	// Insert test events in chronological order
	insertOTAEvent(t, db, "update_started", "", "Auto-update cycle started for firmware v1.2.3",
		map[string]interface{}{"firmware_version": "1.2.3", "filename": "spaxel-1.2.3.bin"},
		now.Add(-2*time.Hour))

	insertOTAEvent(t, db, "canary_deploy", "AA:BB:CC:DD:EE:FF", "Deploying canary update to node AA:BB:CC:DD:EE:FF",
		map[string]interface{}{"firmware_version": "1.2.3", "baseline_quality": 0.85},
		now.Add(-90*time.Minute))

	insertOTAEvent(t, db, "canary_evaluated", "AA:BB:CC:DD:EE:FF", "Canary evaluation: quality delta 2.50%",
		map[string]interface{}{"baseline_quality": 0.85, "current_quality": 0.8725, "quality_delta": 0.025},
		now.Add(-80*time.Minute))

	insertOTAEvent(t, db, "canary_passed", "AA:BB:CC:DD:EE:FF", "Canary passed, proceeding with fleet update",
		map[string]interface{}{"quality_delta": 0.025},
		now.Add(-70*time.Minute))

	insertOTAEvent(t, db, "update_complete", "", "Auto-update complete for firmware v1.2.3",
		map[string]interface{}{"firmware_version": "1.2.3"},
		now.Add(-30*time.Minute))

	handler := &AutoAPIHandler{
		db:       db,
		timezone: time.UTC,
	}

	req := httptest.NewRequest("GET", "/api/ota/auto/history", nil)
	w := httptest.NewRecorder()

	handler.handleHistory(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	history, ok := response["history"].([]interface{})
	if !ok {
		t.Fatal("response history field is not an array")
	}

	if len(history) != 5 {
		t.Errorf("expected 5 history events, got %d", len(history))
	}

	// Verify events are ordered newest-first (descending timestamp)
	// First event should be update_complete (most recent)
	firstEvent, ok := history[0].(map[string]interface{})
	if !ok {
		t.Fatal("first event is not an object")
	}

	otaEvent, ok := firstEvent["ota_event"].(string)
	if !ok || otaEvent != "update_complete" {
		t.Errorf("expected first event to be update_complete, got %s", otaEvent)
	}

	// Verify all events have required fields
	for i, evt := range history {
		eventObj, ok := evt.(map[string]interface{})
		if !ok {
			t.Errorf("event %d is not an object", i)
			continue
		}

		// Check for required fields
		if _, ok := eventObj["id"]; !ok {
			t.Errorf("event %d missing id field", i)
		}
		if _, ok := eventObj["timestamp"]; !ok {
			t.Errorf("event %d missing timestamp field", i)
		}
		if _, ok := eventObj["type"]; !ok {
			t.Errorf("event %d missing type field", i)
		}
		if _, ok := eventObj["detail"]; !ok {
			t.Errorf("event %d missing detail field", i)
		}
	}
}

// TestHandleHistory_Pagination tests cursor-based pagination.
func TestHandleHistory_Pagination(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	now := time.Now().UTC()

	// Insert 10 test events with timestamps in chronological order
	// (older events inserted first with older timestamps, newer events inserted later with newer timestamps)
	// This matches real-world usage where event publication time and database insertion time are aligned
	baseTime := now.Add(-10 * time.Minute)
	for i := 0; i < 10; i++ {
		insertOTAEvent(t, db, "test_event", "AA:BB:CC:DD:EE:FF",
			fmt.Sprintf("Test event %d", i),
			map[string]interface{}{"index": i},
			baseTime.Add(time.Duration(i)*time.Minute))
	}

	handler := &AutoAPIHandler{
		db:       db,
		timezone: time.UTC,
	}

	// First page with limit=5
	req := httptest.NewRequest("GET", "/api/ota/auto/history?limit=5", nil)
	w := httptest.NewRecorder()

	handler.handleHistory(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	history, ok := response["history"].([]interface{})
	if !ok {
		t.Fatal("response history field is not an array")
	}

	if len(history) != 5 {
		t.Errorf("expected 5 events on first page, got %d", len(history))
	}

	hasMore, ok := response["has_more"].(bool)
	if !ok || !hasMore {
		t.Error("expected has_more to be true when more events exist")
	}

	cursor, ok := response["cursor"].(string)
	if !ok || cursor == "" {
		t.Error("expected cursor to be returned when more events exist")
	}

	// Second page using cursor
	if cursor != "" {
		req = httptest.NewRequest("GET", "/api/ota/auto/history?limit=5&before="+cursor, nil)
		w = httptest.NewRecorder()

		handler.handleHistory(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200 on second page, got %d", w.Code)
		}

		var secondResponse map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &secondResponse); err != nil {
			t.Fatalf("failed to decode second response: %v", err)
		}

		secondHistory, ok := secondResponse["history"].([]interface{})
		if !ok {
			t.Fatal("second response history field is not an array")
		}

		if len(secondHistory) != 5 {
			t.Errorf("expected 5 events on second page, got %d", len(secondHistory))
		}

		hasMore, ok := secondResponse["has_more"].(bool)
		if !ok || hasMore {
			t.Error("expected has_more to be false on last page")
		}
	}
}

// TestHandleHistory_DatabaseUnavailable tests error handling when database is unavailable.
func TestHandleHistory_DatabaseUnavailable(t *testing.T) {
	handler := &AutoAPIHandler{
		db:       nil,
		timezone: time.UTC,
	}

	req := httptest.NewRequest("GET", "/api/ota/auto/history", nil)
	w := httptest.NewRecorder()

	handler.handleHistory(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}
