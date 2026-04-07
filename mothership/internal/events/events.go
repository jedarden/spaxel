// Package events provides event types and SQLite storage for the unified activity timeline.
package events

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/spaxel/mothership/internal/eventbus"
)

// EventType represents the type of an event.
type EventType string

const (
	EventTypeDetection        EventType = "detection"
	EventTypeZoneEntry        EventType = "zone_entry"
	EventTypeZoneExit         EventType = "zone_exit"
	EventTypePortalCrossing   EventType = "portal_crossing"
	EventTypeTriggerFired     EventType = "trigger_fired"
	EventTypeFallAlert        EventType = "fall_alert"
	EventTypeAnomaly          EventType = "anomaly"
	EventTypeSecurityAlert    EventType = "security_alert"
	EventTypeNodeOnline       EventType = "node_online"
	EventTypeNodeOffline      EventType = "node_offline"
	EventTypeOTAUpdate        EventType = "ota_update"
	EventTypeBaselineChanged  EventType = "baseline_changed"
	EventTypeSystem           EventType = "system"
	EventTypeLearningMilestone EventType = "learning_milestone"
)

// EventSeverity represents the severity level of an event.
type EventSeverity string

const (
	SeverityInfo     EventSeverity = "info"
	SeverityWarning  EventSeverity = "warning"
	SeverityAlert    EventSeverity = "alert"
	SeverityCritical EventSeverity = "critical"
)

// Event represents a single event in the unified activity timeline.
type Event struct {
	ID          int64
	TimestampMs int64
	Type        EventType
	Zone        string
	Person      string
	BlobID      int
	DetailJSON  string
	Severity    EventSeverity
}

// QueryParams defines parameters for querying events.
type QueryParams struct {
	Limit       int
	BeforeID    int64 // Cursor for pagination
	AfterID     int64
	BeforeTS    int64 // Timestamp-based cursor for keyset pagination (used by REST API)
	Type        EventType
	Zone        string
	Person      string
	AfterTime   time.Time
	BeforeTime  time.Time
	SearchQuery string // FTS5 search query
}

// InsertEvent inserts a new event into the database and publishes it to the event bus.
func InsertEvent(db *sql.DB, e Event) (int64, error) {
	if e.TimestampMs == 0 {
		e.TimestampMs = time.Now().UnixMilli()
	}
	if e.Severity == "" {
		e.Severity = SeverityInfo
	}

	result, err := db.Exec(`
		INSERT INTO events (timestamp_ms, type, zone, person, blob_id, detail_json, severity)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, e.TimestampMs, e.Type, e.Zone, e.Person, e.BlobID, e.DetailJSON, e.Severity)
	if err != nil {
		return 0, fmt.Errorf("insert event: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get last insert id: %w", err)
	}

	// Publish to the internal event bus for WebSocket clients and other subscribers.
	// This is non-blocking; subscribers run in separate goroutines.
	eventbus.PublishDefault(eventbus.Event{
		Type:        string(e.Type),
		TimestampMs: e.TimestampMs,
		Zone:        e.Zone,
		Person:      e.Person,
		BlobID:      e.BlobID,
		Detail:      e.DetailJSON, // Pass as string; subscribers can parse if needed
		Severity:    string(e.Severity),
	})

	return id, nil
}

// QueryEvents retrieves events from the database based on the provided parameters.
// Returns the events, the next cursor ID for pagination, and whether there are more results.
func QueryEvents(db *sql.DB, params QueryParams) ([]Event, string, bool, error) {
	if params.Limit <= 0 {
		params.Limit = 50
	}
	if params.Limit > 1000 {
		params.Limit = 1000 // Max limit
	}

	query := `
			SELECT id, timestamp_ms, type, zone, person, blob_id, detail_json, severity
			FROM events
			WHERE 1=1
		`
	args := []interface{}{}

	// Cursor pagination: prefer BeforeTS (timestamp keyset) over BeforeID
	if params.BeforeTS > 0 {
		query += " AND timestamp_ms < ?"
		args = append(args, params.BeforeTS)
	} else if params.BeforeID > 0 {
		query += " AND id < ?"
		args = append(args, params.BeforeID)
	} else if params.AfterID > 0 {
		query += " AND id > ?"
		args = append(args, params.AfterID)
	}

	// Time range filters
	if !params.AfterTime.IsZero() {
		query += " AND timestamp_ms >= ?"
		args = append(args, params.AfterTime.UnixMilli())
	}
	if !params.BeforeTime.IsZero() {
		query += " AND timestamp_ms <= ?"
		args = append(args, params.BeforeTime.UnixMilli())
	}

	// Type filter
	if params.Type != "" {
		query += " AND type = ?"
		args = append(args, params.Type)
	}

	// Zone filter
	if params.Zone != "" {
		query += " AND zone = ?"
		args = append(args, params.Zone)
	}

	// Person filter
	if params.Person != "" {
		query += " AND person = ?"
		args = append(args, params.Person)
	}

	// FTS5 full-text search (must use the FTS table)
	if params.SearchQuery != "" {
		ftsQuery := prepareFTSQuery(params.SearchQuery)
		// Use subquery to search FTS and join with events table
		query = `
				SELECT e.id, e.timestamp_ms, e.type, e.zone, e.person, e.blob_id, e.detail_json, e.severity
				FROM events e
				INNER JOIN events_fts fts ON e.id = fts.rowid
				WHERE events_fts MATCH ?
			`
		args = []interface{}{ftsQuery}

		// Reapply other filters to the subquery
		if params.BeforeTS > 0 {
			query += " AND e.timestamp_ms < ?"
			args = append(args, params.BeforeTS)
		} else if params.BeforeID > 0 {
			query += " AND e.id < ?"
			args = append(args, params.BeforeID)
		} else if params.AfterID > 0 {
			query += " AND e.id > ?"
			args = append(args, params.AfterID)
		}
		if !params.AfterTime.IsZero() {
			query += " AND e.timestamp_ms >= ?"
			args = append(args, params.AfterTime.UnixMilli())
		}
		if !params.BeforeTime.IsZero() {
			query += " AND e.timestamp_ms <= ?"
			args = append(args, params.BeforeTime.UnixMilli())
		}
		if params.Type != "" {
			query += " AND e.type = ?"
			args = append(args, params.Type)
		}
		if params.Zone != "" {
			query += " AND e.zone = ?"
			args = append(args, params.Zone)
		}
		if params.Person != "" {
			query += " AND e.person = ?"
			args = append(args, params.Person)
		}
	}

	query += " ORDER BY id DESC LIMIT ?"
	args = append(args, params.Limit+1) // Fetch one extra to check for more results

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, "", false, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		err := rows.Scan(&e.ID, &e.TimestampMs, &e.Type, &e.Zone, &e.Person, &e.BlobID, &e.DetailJSON, &e.Severity)
		if err != nil {
			return nil, "", false, fmt.Errorf("scan event: %w", err)
		}
		events = append(events, e)
	}

	if err := rows.Err(); err != nil {
		return nil, "", false, fmt.Errorf("iterate rows: %w", err)
	}

	// Check if there are more results
	hasMore := len(events) > params.Limit
	nextCursor := ""
	if hasMore {
		// Remove the extra event
		events = events[:params.Limit]
		nextCursor = fmt.Sprintf("%d", events[len(events)-1].ID)
	}

	return events, nextCursor, hasMore, nil
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
	if strings.Contains(q, " AND ") || strings.Contains(q, " OR ") ||
		strings.Contains(q, " NOT ") || strings.Contains(q, " NEAR ") {
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
	return q + "*"
}

// ArchiveDays is the number of days after which events are archived.
const ArchiveDays = 90

// ArchiveDaysMs is ArchiveDays expressed in milliseconds.
const ArchiveDaysMs = ArchiveDays * 24 * 60 * 60 * 1000

// RunArchiveJob moves events older than 90 days from the events table to the events_archive table.
// This should be called nightly (e.g., at 02:00 local time).
func RunArchiveJob(db *sql.DB) error {
	// Get the cutoff timestamp
	cutoffMs := time.Now().UnixMilli() - ArchiveDaysMs

	// Begin transaction for atomic archive operation
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get count of events to be archived
	var count int
	err = tx.QueryRow("SELECT COUNT(*) FROM events WHERE timestamp_ms < ?", cutoffMs).Scan(&count)
	if err != nil {
		return fmt.Errorf("count events to archive: %w", err)
	}

	if count == 0 {
		log.Printf("[events archive] No events to archive (cutoff: %d ms ago)", ArchiveDays)
		return nil
	}

	log.Printf("[events archive] Archiving %d events older than %d days", count, ArchiveDays)

	// Copy old events to archive table
	// We preserve the original ID to maintain referential integrity
	_, err = tx.Exec(`
		INSERT INTO events_archive (id, timestamp_ms, type, zone, person, blob_id, detail_json, severity)
		SELECT id, timestamp_ms, type, zone, person, blob_id, detail_json, severity
		FROM events
		WHERE timestamp_ms < ?
	`, cutoffMs)
	if err != nil {
		return fmt.Errorf("copy events to archive: %w", err)
	}

	// Delete the archived events from the main events table
	// The FTS5 triggers will automatically remove them from events_fts
	result, err := tx.Exec("DELETE FROM events WHERE timestamp_ms < ?", cutoffMs)
	if err != nil {
		return fmt.Errorf("delete archived events: %w", err)
	}

	// Verify the delete count matches the copy count
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected != int64(count) {
		log.Printf("[WARN] Events archive: copied %d but deleted %d rows", count, rowsAffected)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit archive transaction: %w", err)
	}

	log.Printf("[events archive] Successfully archived %d events", count)
	return nil
}

// GetEventByID retrieves a single event by its ID.
func GetEventByID(db *sql.DB, id int64) (*Event, error) {
	var e Event
	err := db.QueryRow(`
		SELECT id, timestamp_ms, type, zone, person, blob_id, detail_json, severity
		FROM events
		WHERE id = ?
	`, id).Scan(&e.ID, &e.TimestampMs, &e.Type, &e.Zone, &e.Person, &e.BlobID, &e.DetailJSON, &e.Severity)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("event not found: %d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("query event: %w", err)
	}
	return &e, nil
}

// InsertDetectionEvent is a convenience function for inserting detection events.
func InsertDetectionEvent(db *sql.DB, zone string, person string, blobID int, detail map[string]interface{}) (int64, error) {
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return 0, fmt.Errorf("marshal detail: %w", err)
	}

	return InsertEvent(db, Event{
		Type:       EventTypeDetection,
		Zone:       zone,
		Person:     person,
		BlobID:     blobID,
		DetailJSON: string(detailJSON),
		Severity:   SeverityInfo,
	})
}

// InsertAlertEvent is a convenience function for inserting alert events.
func InsertAlertEvent(db *sql.DB, eventType EventType, zone string, person string, severity EventSeverity, detail map[string]interface{}) (int64, error) {
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return 0, fmt.Errorf("marshal detail: %w", err)
	}

	return InsertEvent(db, Event{
		Type:       eventType,
		Zone:       zone,
		Person:     person,
		DetailJSON: string(detailJSON),
		Severity:   severity,
	})
}

// InsertSystemEvent is a convenience function for inserting system events.
func InsertSystemEvent(db *sql.DB, message string, detail map[string]interface{}) (int64, error) {
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return 0, fmt.Errorf("marshal detail: %w", err)
	}

	if detail == nil {
		detail = make(map[string]interface{})
	}
	detail["message"] = message

	detailJSON, err = json.Marshal(detail)
	if err != nil {
		return 0, fmt.Errorf("marshal detail with message: %w", err)
	}

	return InsertEvent(db, Event{
		Type:       EventTypeSystem,
		DetailJSON: string(detailJSON),
		Severity:   SeverityInfo,
	})
}

// StartArchiveScheduler starts a goroutine that runs the archive job nightly at 02:00 local time.
// The goroutine runs until the done channel is closed.
func StartArchiveScheduler(db *sql.DB, done <-chan struct{}) {
	go func() {
		for {
			// Calculate duration until next 02:00 local time
			now := time.Now()
			nextRun := time.Date(now.Year(), now.Month(), now.Day(), 2, 0, 0, 0, now.Location())

			// If we're already past 02:00 today, schedule for tomorrow
			if now.After(nextRun) {
				nextRun = nextRun.Add(24 * time.Hour)
			}

			duration := nextRun.Sub(now)
			log.Printf("[events archive] Next run scheduled for %s (in %s)", nextRun.Format(time.RFC1123), duration.Round(time.Second))

			// Wait until the next scheduled run time or done signal
			select {
			case <-time.After(duration):
				// Time to run the archive job
				log.Printf("[events archive] Running scheduled archive job")
				if err := RunArchiveJob(db); err != nil {
					log.Printf("[ERROR] Events archive job failed: %v", err)
				}
			case <-done:
				log.Printf("[events archive] Scheduler stopped")
				return
			}
		}
	}()
}
