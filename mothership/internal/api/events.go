// Package api provides REST API handlers for Spaxel events timeline.
package api

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi"
)

const (
	eventsDefaultLimit = 50
	eventsMaxLimit     = 500
)

// EventsHandler manages the events timeline.
type EventsHandler struct {
	mu  sync.RWMutex
	db  *sql.DB
	hub DashboardHub
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

// NewEventsHandler creates a new events handler using the shared database connection.
// The events table schema must already exist (created by migrations 001 and 011).
func NewEventsHandler(db *sql.DB) *EventsHandler {
	log.Printf("[INFO] Events handler initialized")
	return &EventsHandler{db: db}
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
//	after (ISO8601), type, zone, person, q (FTS5 query).
//
// GET /api/events/{id} — single event by ID.
func (e *EventsHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/events", e.listEvents)
	r.Get("/api/events/{id}", e.getEvent)
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
	person := r.URL.Query().Get("person")
	afterStr := r.URL.Query().Get("after")

	// Validate event type
	if eventType != "" && !isValidEventType(eventType) {
		writeJSONError(w, http.StatusBadRequest, "invalid event type")
		return
	}

	// Validate after timestamp
	var afterTS int64
	if afterStr != "" {
		t, err := time.Parse(time.RFC3339, afterStr)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid after timestamp")
			return
		}
		afterTS = t.UnixNano() / 1e6
	}

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

	// Collect filter conditions (excludes before cursor — that's pagination, not filtering)
	type cond struct {
		sql string
		arg interface{}
	}
	var filters []cond

	if eventType != "" {
		filters = append(filters, cond{p + "type = ?", eventType})
	}
	if zone != "" {
		filters = append(filters, cond{p + "zone = ?", zone})
	}
	if person != "" {
		filters = append(filters, cond{p + "person = ?", person})
	}
	if afterTS > 0 {
		filters = append(filters, cond{p + "timestamp_ms >= ?", afterTS})
	}

	// Build WHERE clause with all filters (no before, no LIMIT)
	whereSQL := baseWhere
	whereArgs := append([]interface{}{}, baseArgs...)
	for _, f := range filters {
		whereSQL += " AND " + f.sql
		whereArgs = append(whereArgs, f.arg)
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
