// Package api provides REST API handlers for Spaxel events timeline.
package api

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/spaxel/mothership/internal/events"
)

const (
	eventsDefaultLimit = 50
	eventsMaxLimit     = 500
)

// EventsHandler manages the events timeline.
type EventsHandler struct {
	mu             sync.RWMutex
	db             *sql.DB
	hub            DashboardHub
	ownsDB         bool
	feedbackHandler any // FeedbackHandler for POST /api/events/{id}/feedback
}

// DashboardHub is the interface for broadcasting to dashboard clients.
type DashboardHub interface {
	BroadcastEventFromDB(id int64, timestamp int64, eventType, zone, person string, blobID int, detailJSON, severity string)
}

// Event represents a timeline event.
type Event struct {
	ID         int64     `json:"id"`
	Timestamp  int64     `json:"timestamp_ms"`
	Type       string    `json:"type"`
	Zone       string    `json:"zone,omitempty"`
	Person     string    `json:"person,omitempty"`
	BlobID     int       `json:"blob_id,omitempty"`
	DetailJSON string    `json:"detail_json,omitempty"`
	Severity   string    `json:"severity"`
}

// LogEvent logs a new event to the database and broadcasts it.
func (h *EventsHandler) LogEvent(eventType string, timestamp time.Time, zone, person string, blobID int, detailJSON string, severity string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	ts := timestamp.UnixNano() / 1e6
	if severity == "" {
		severity = "info"
	}

	result, err := h.db.Exec(`
		INSERT INTO events (timestamp_ms, type, zone, person, blob_id, detail_json, severity)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, ts, eventType, zone, person, blobID, detailJSON, severity)
	if err != nil {
		return err
	}

	id, _ := result.LastInsertId()

	// Broadcast to dashboard clients
	if h.hub != nil {
		h.hub.BroadcastEventFromDB(id, ts, eventType, zone, person, blobID, detailJSON, severity)
	}

	return nil
}

// SetHub sets the dashboard hub for broadcasting events.
func (e *EventsHandler) SetHub(hub DashboardHub) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.hub = hub
}

// SetFeedbackHandler sets the feedback handler for event feedback endpoints.
func (e *EventsHandler) SetFeedbackHandler(handler any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.feedbackHandler = handler
}

// NewEventsHandler creates a new events handler backed by a SQLite file at dbPath.
// It opens the database, creates the schema, and takes ownership of the connection.
// Use Close() to release resources.
func NewEventsHandler(dbPath string) (*EventsHandler, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open events db: %w", err)
	}
	if err := createEventsSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("init events schema: %w", err)
	}
	log.Printf("[INFO] Events handler initialized (own DB: %s)", dbPath)
	return &EventsHandler{db: db, ownsDB: true}, nil
}

// NewEventsHandlerFromDB creates a new events handler using an existing database connection.
// The events table schema must already exist (created by migrations 001 and 011).
func NewEventsHandlerFromDB(db *sql.DB) *EventsHandler {
	log.Printf("[INFO] Events handler initialized (shared DB)")
	return &EventsHandler{db: db}
}

// Close releases resources. If the handler owns the DB connection, it closes it.
func (e *EventsHandler) Close() {
	if e.ownsDB {
		e.db.Close()
	}
}

// Archive runs the archive job to move old events to the archive table.
func (e *EventsHandler) Archive(_ interface{}) {
	events.RunArchiveJob(e.db)
}

// createEventsSchema creates the events, events_archive, and FTS5 tables.
func createEventsSchema(db *sql.DB) error {
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
	CREATE INDEX IF NOT EXISTS idx_events_zone ON events(zone, timestamp_ms DESC);
	CREATE INDEX IF NOT EXISTS idx_events_person ON events(person, timestamp_ms DESC);
	CREATE TABLE IF NOT EXISTS events_archive (
		id          INTEGER PRIMARY KEY,
		timestamp_ms INTEGER NOT NULL,
		type        TEXT    NOT NULL,
		zone        TEXT,
		person      TEXT,
		blob_id     INTEGER,
		detail_json TEXT,
		severity    TEXT    NOT NULL DEFAULT 'info'
	);
	CREATE VIRTUAL TABLE IF NOT EXISTS events_fts USING fts5(
		type, zone, person, detail_json,
		content='events', content_rowid='id'
	);
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
	_, err := db.Exec(schema)
	return err
}

// isValidEventType checks whether the event type string is a known type.
func isValidEventType(t string) bool {
	switch t {
	case "detection", "zone_entry", "zone_exit", "portal_crossing",
		"trigger_fired", "fall_alert", "anomaly", "security_alert",
		"node_online", "node_offline", "ota_update", "baseline_changed",
		"system", "learning_milestone":
		return true
	}
	return false
}

// RegisterRoutes registers events endpoints.
//
// GET /api/events — paginated event list with FTS5 search and keyset cursor pagination.
//
//	Query params: limit (default 50, max 500), before (timestamp_ms cursor),
//	since (ISO8601), until (ISO8601), type, zone_id, person_id, q (FTS5 query), mode (expert|simple).
//
// GET /api/events/{id} — single event by ID.
//
// POST /api/events/{id}/feedback — submit feedback for an event.
func (e *EventsHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/events", e.listEvents)
	r.Get("/api/events/{id}", e.getEvent)
	r.Post("/api/events/{id}/feedback", e.postEventFeedback)
}

// eventsResponse is the JSON response for GET /api/events.
type eventsResponse struct {
	Events        []*Event `json:"events"`
	Cursor        string   `json:"cursor,omitempty"`
	HasMore       bool     `json:"has_more"`
	TotalFiltered int      `json:"total_filtered"`
}

// prepareFTSQuery appends a trailing * for prefix matching if the query
// doesn't already end with a FTS5 operator character. This enables
// partial word matching (e.g., "kit" matches "kitchen").
func prepareFTSQuery(q string) string {
	q = strings.TrimSpace(q)
	if q == "" {
		return q
	}
	// If the query already ends with a FTS5 special character or operator, leave it alone.
	last := q[len(q)-1]
	if last == '*' || last == '"' || last == ')' {
		return q
	}
	// For simple terms (no operators), append * for prefix matching.
	// If the query contains FTS5 operators (AND, OR, NOT, NEAR), append * to each
	// simple token instead.
	if strings.Contains(q, " AND ") || strings.Contains(q, " OR ") ||
		strings.Contains(q, " NOT ") || strings.Contains(q, " NEAR ") {
		// Has operators — append * to each token that isn't an operator or quoted phrase.
		parts := strings.Fields(q)
		for i, p := range parts {
			if p == "AND" || p == "OR" || p == "NOT" || p == "NEAR" {
				continue
			}
			if (strings.HasPrefix(p, `"`) && strings.HasSuffix(p, `"`)) || p == "(" || p == ")" {
				continue
			}
			parts[i] = p + "*"
		}
		return strings.Join(parts, " ")
	}
	// Simple single-term query — just append * for prefix matching.
	return q + "*"
}

func (e *EventsHandler) listEvents(w http.ResponseWriter, r *http.Request) {
	// Parse limit
	limit := eventsDefaultLimit
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > eventsMaxLimit {
		limit = eventsMaxLimit
	}

	// Parse before cursor (timestamp_ms as string)
	var beforeTS int64
	if s := r.URL.Query().Get("before"); s != "" {
		beforeTS, _ = strconv.ParseInt(s, 10, 64)
	}

	// Parse filters
	q := r.URL.Query().Get("q")
	eventType := r.URL.Query().Get("type")
	zone := r.URL.Query().Get("zone")
	zoneID := r.URL.Query().Get("zone_id")
	if zoneID != "" && zone == "" {
		zone = zoneID
	}
	person := r.URL.Query().Get("person")
	personID := r.URL.Query().Get("person_id")
	if personID != "" && person == "" {
		person = personID
	}
	afterStr := r.URL.Query().Get("after")
	sinceStr := r.URL.Query().Get("since") // Alias for after
	untilStr := r.URL.Query().Get("until") // Upper bound timestamp
	mode := r.URL.Query().Get("mode")      // "expert" or "simple" (default: simple)

	// Validate event type
	if eventType != "" && !isValidEventType(eventType) {
		writeJSONError(w, http.StatusBadRequest, "invalid event type")
		return
	}

	// Validate after/since timestamp (prefer since if both provided)
	var afterTS int64
	timeStr := afterStr
	if sinceStr != "" {
		timeStr = sinceStr
	}
	if timeStr != "" {
		t, err := time.Parse(time.RFC3339, timeStr)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid since/after timestamp")
			return
		}
		afterTS = t.UnixNano() / 1e6
	}

	// Validate until timestamp (upper bound)
	var untilTS int64
	if untilStr != "" {
		t, err := time.Parse(time.RFC3339, untilStr)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid until timestamp")
			return
		}
		untilTS = t.UnixNano() / 1e6
	}

	// In simple mode, filter out system-only event types
	// Simple mode shows: zone_entry, zone_exit, portal_crossing, fall_alert, anomaly, security_alert, learning_milestone
	// Simple mode hides: node_online, node_offline, ota_update, baseline_changed, system
	simpleModeTypes := map[string]bool{
		"zone_entry":        true,
		"zone_exit":         true,
		"portal_crossing":   true,
		"fall_alert":        true,
		"anomaly":           true,
		"security_alert":    true,
		"learning_milestone": true,
		"presence_transition": true,
		"stationary_detected": true,
	}
	isSimpleMode := mode != "expert"

	// Prepare FTS5 query with prefix matching
	if q != "" {
		q = prepareFTSQuery(q)
	}

	// Determine query mode: FTS5 or regular
	useFTS := q != ""
	p := "" // column prefix for FTS JOIN queries
	if useFTS {
		p = "e."
	}

	// Build SELECT columns and FROM clause
	selectCols := p + "id, " + p + "timestamp_ms, " + p + "type, " + p + "zone, " +
		p + "person, " + p + "blob_id, " + p + "detail_json, " + p + "severity"

	var fromTable, baseWhere string
	var baseArgs []interface{}

	if useFTS {
		fromTable = "events e JOIN events_fts ft ON e.id = ft.rowid"
		baseWhere = "events_fts MATCH ?"
		baseArgs = []interface{}{q}
	} else {
		fromTable = "events"
		baseWhere = "1=1"
		baseArgs = []interface{}{}
	}

	// Build WHERE clause with filters
	whereSQL := baseWhere
	whereArgs := append([]interface{}{}, baseArgs...)

	if eventType != "" {
		whereSQL += " AND " + p + "type = ?"
		whereArgs = append(whereArgs, eventType)
	} else if isSimpleMode {
		// In simple mode with no explicit type filter, exclude system event types
		whereSQL += " AND " + p + "type NOT IN (?, ?, ?, ?, ?)"
		whereArgs = append(whereArgs, "node_online", "node_offline", "ota_update", "baseline_changed", "system")
	}
	if zone != "" {
		whereSQL += " AND " + p + "zone = ?"
		whereArgs = append(whereArgs, zone)
	}
	if person != "" {
		whereSQL += " AND " + p + "person = ?"
		whereArgs = append(whereArgs, person)
	}
	if afterTS > 0 {
		whereSQL += " AND " + p + "timestamp_ms >= ?"
		whereArgs = append(whereArgs, afterTS)
	}
	if untilTS > 0 {
		whereSQL += " AND " + p + "timestamp_ms <= ?"
		whereArgs = append(whereArgs, untilTS)
	}

	// COUNT for total_filtered
	countSQL := "SELECT COUNT(*) FROM " + fromTable + " WHERE " + whereSQL
	var totalFiltered int
	if err := e.db.QueryRow(countSQL, whereArgs...).Scan(&totalFiltered); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to count events")
		return
	}

	// Data query: add before cursor + ordering + limit
	dataWhere := whereSQL
	dataArgs := append([]interface{}{}, whereArgs...)
	if beforeTS > 0 {
		dataWhere += " AND " + p + "timestamp_ms < ?"
		dataArgs = append(dataArgs, beforeTS)
	}
	// untilTS is already included in the base WHERE clause via whereArgs

	dataSQL := "SELECT " + selectCols + " FROM " + fromTable +
		" WHERE " + dataWhere +
		" ORDER BY " + p + "timestamp_ms DESC, " + p + "id DESC" +
		" LIMIT ?"
	dataArgs = append(dataArgs, limit+1)

	rows, err := e.db.Query(dataSQL, dataArgs...)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to query events")
		return
	}
	defer rows.Close()

	events := make([]*Event, 0, limit)
	for rows.Next() {
		var ev Event
		if err := rows.Scan(&ev.ID, &ev.Timestamp, &ev.Type, &ev.Zone,
			&ev.Person, &ev.BlobID, &ev.DetailJSON, &ev.Severity); err != nil {
			continue
		}
		events = append(events, &ev)
	}

	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}

	cursor := ""
	if hasMore && len(events) > 0 {
		cursor = strconv.FormatInt(events[len(events)-1].Timestamp, 10)
	}

	writeJSON(w, http.StatusOK, eventsResponse{
		Events:        events,
		Cursor:        cursor,
		HasMore:       hasMore,
		TotalFiltered: totalFiltered,
	})
}

func (e *EventsHandler) getEvent(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid event id")
		return
	}

	var event Event
	err = e.db.QueryRow(`
		SELECT id, timestamp_ms, type, zone, person, blob_id, detail_json, severity
		FROM events
		WHERE id = ?
	`, id).Scan(&event.ID, &event.Timestamp, &event.Type, &event.Zone,
		&event.Person, &event.BlobID, &event.DetailJSON, &event.Severity)
	if err == sql.ErrNoRows {
		writeJSONError(w, http.StatusNotFound, "event not found")
		return
	} else if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to query event")
		return
	}

	writeJSON(w, http.StatusOK, event)
}

// postEventFeedback handles POST /api/events/{id}/feedback
// It delegates to the feedback module after validating the event exists.
func (e *EventsHandler) postEventFeedback(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	eventID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid event id")
		return
	}

	// Verify the event exists
	var exists bool
	err = e.db.QueryRow("SELECT EXISTS(SELECT 1 FROM events WHERE id = ?)", eventID).Scan(&exists)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to query event")
		return
	}
	if !exists {
		writeJSONError(w, http.StatusNotFound, "event not found")
		return
	}

	// Decode request body
	var req FeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Set the event ID from the URL path
	req.EventID = eventID

	// Validate feedback type
	if req.Type != "correct" && req.Type != "incorrect" && req.Type != "missed" {
		writeJSONError(w, http.StatusBadRequest, "invalid feedback type: must be 'correct', 'incorrect', or 'missed'")
		return
	}

	// Delegate to feedback handler if available
	if e.feedbackHandler != nil {
		// Use the feedback handler to process the request
		type submitter interface {
			SubmitFeedback(w http.ResponseWriter, r *http.Request, req FeedbackRequest)
		}
		if fh, ok := e.feedbackHandler.(submitter); ok {
			fh.SubmitFeedback(w, r, req)
			return
		}
	}

	// Fallback: log a feedback event
	_ = e.LogEvent("feedback", time.Now(), "", "", 0,
		fmt.Sprintf(`{"event_id":%d,"type":"%s","blob_id":%d}`, eventID, req.Type, req.BlobID), "info")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":     true,
		"message": "Feedback recorded",
	})
}

// FeedbackRequest represents a feedback submission for an event.
type FeedbackRequest struct {
	Type     string `json:"type"`     // "correct" or "incorrect"
	BlobID   int    `json:"blob_id"`  // Optional: blob ID being rated
	Position *struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
		Z float64 `json:"z"`
	} `json:"position,omitempty"` // For "missed" feedback
}
