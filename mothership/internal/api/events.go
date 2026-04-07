// Package api provides REST API handlers for Spaxel events timeline.
package api

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi"
	_ "modernc.org/sqlite"
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

// NewEventsHandler creates a new events handler.
func NewEventsHandler(dbPath string) (*EventsHandler, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	e := &EventsHandler{
		db: db,
	}

	if err := e.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	log.Printf("[INFO] Events handler initialized with DB at %s", dbPath)
	return e, nil
}

func (e *EventsHandler) migrate() error {
	_, err := e.db.Exec(`
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
		CREATE INDEX IF NOT EXISTS idx_events_archive_time ON events_archive(timestamp_ms DESC);
	`)
	return err
}

// Archive moves events older than 90 days (or the specified duration) to the archive table.
// If retentionDays is nil, defaults to 90 days.
func (e *EventsHandler) Archive(retentionDays *int) {
	days := 90
	if retentionDays != nil {
		days = *retentionDays
	}
	cutoff := time.Now().AddDate(0, 0, -days).UnixNano() / 1e6

	tx, err := e.db.Begin()
	if err != nil {
		log.Printf("[WARN] archive: begin tx: %v", err)
		return
	}
	defer tx.Rollback()

	tx.Exec(`INSERT OR IGNORE INTO events_archive (id, timestamp_ms, type, zone, person, blob_id, detail_json, severity)
		SELECT id, timestamp_ms, type, zone, person, blob_id, detail_json, severity
		FROM events WHERE timestamp_ms < ?`, cutoff)
	tx.Exec(`DELETE FROM events WHERE timestamp_ms < ?`, cutoff)

	if err := tx.Commit(); err != nil {
		log.Printf("[WARN] archive: commit: %v", err)
		return
	}

	log.Printf("[INFO] events archived: removed events older than %d days", days)
}

// Close closes the database.
func (e *EventsHandler) Close() error {
	return e.db.Close()
}

// RegisterRoutes registers events endpoints.
//
// Events represent the unified activity timeline for the Spaxel system.
// All system events (detections, zone transitions, alerts, system events)
// are logged here and can be retrieved via the API.
//
// GET /api/events
//
//	@Summary		List events
//	@Description	Returns paginated events with optional filtering by type, zone, person, and time range.
//	@Tags			events
//	@Produce		json
//	@Param			limit		query	int		false	"Max events to return (default: 200)"
//	@Param			before		query	int		false	"Return events before this ID (cursor for pagination)"
//	@Param			type		query	string	false	"Filter by event type"
//	@Param			zone		query	string	false	"Filter by zone name"
//	@Param			person		query	string	false	"Filter by person name"
//	@Param			after		query	string	false	"ISO8601 timestamp - only events after this time"
//	@Param			q			query	string	false	"Text search across event descriptions"
//	@Success		200	{object}	eventsResponse	"List of events with pagination cursor"
//	@Router			/api/events [get]
//
// GET /api/events/{id}
//
//	@Summary		Get single event
//	@Description	Returns full details for a specific event.
//	@Tags			events
//	@Produce		json
//	@Param			id		path	int		true	"Event ID"
//	@Success		200	{object}	Event	"Event details"
//	@Failure		404		{object}	map[string]string	"Event not found"
//	@Router			/api/events/{id} [get]
func (e *EventsHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/events", e.listEvents)
	r.Get("/api/events/{id}", e.getEvent)
}

type eventsResponse struct {
	Events []*Event `json:"events"`
	Cursor int64     `json:"cursor,omitempty"`
	Total  int       `json:"total"`
}

func (e *EventsHandler) listEvents(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	limitStr := r.URL.Query().Get("limit")
	limit := 200
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}

	beforeStr := r.URL.Query().Get("before")
	var beforeID int64
	if beforeStr != "" {
		beforeID, _ = strconv.ParseInt(beforeStr, 10, 64)
	}

	eventType := r.URL.Query().Get("type")
	zone := r.URL.Query().Get("zone")
	person := r.URL.Query().Get("person")
	afterStr := r.URL.Query().Get("after")
	searchQuery := r.URL.Query().Get("q")

	// Build query
	query := `
		SELECT id, timestamp_ms, type, zone, person, blob_id, detail_json, severity
		FROM events
		WHERE 1=1
	`
	args := []interface{}{}

	if beforeID > 0 {
		query += " AND id < ?"
		args = append(args, beforeID)
	}

	if eventType != "" {
		query += " AND type = ?"
		args = append(args, eventType)
	}

	if zone != "" {
		query += " AND zone = ?"
		args = append(args, zone)
	}

	if person != "" {
		query += " AND person = ?"
		args = append(args, person)
	}

	if afterStr != "" {
		afterTime, err := time.Parse(time.RFC3339, afterStr)
		if err == nil {
			query += " AND timestamp_ms >= ?"
			args = append(args, afterTime.UnixNano()/1e6)
		}
	}

	if searchQuery != "" {
		query += " AND (type LIKE ? OR zone LIKE ? OR person LIKE ? OR detail_json LIKE ?)"
		pattern := "%" + searchQuery + "%"
		args = append(args, pattern, pattern, pattern, pattern)
	}

	// Get total count
	countQuery := "SELECT COUNT(*) FROM events" + query[50:] // Skip SELECT ... FROM events WHERE
	var total int
	err := e.db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		http.Error(w, "failed to count events", http.StatusInternalServerError)
		return
	}

	// Add ordering and limit
	query += " ORDER BY timestamp_ms DESC, id DESC LIMIT ?"
	args = append(args, limit+1) // Fetch one extra to determine if there's a next page

	rows, err := e.db.Query(query, args...)
	if err != nil {
		http.Error(w, "failed to query events", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	events := make([]*Event, 0, limit)
	var nextCursor int64

	for rows.Next() {
		var event Event
		err := rows.Scan(&event.ID, &event.Timestamp, &event.Type, &event.Zone,
			&event.Person, &event.BlobID, &event.DetailJSON, &event.Severity)
		if err != nil {
			continue
		}

		if len(events) < limit {
			events = append(events, &event)
		} else {
			// This is the extra row - use it for cursor
			nextCursor = event.ID
		}
	}

	response := eventsResponse{
		Events: events,
		Total:  total,
	}
	if nextCursor > 0 {
		response.Cursor = nextCursor
	}

	writeJSON(w, http.StatusOK, response)
}

func (e *EventsHandler) getEvent(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid event id", http.StatusBadRequest)
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
		http.Error(w, "event not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "failed to query event", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, event)
}
