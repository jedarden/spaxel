// Package learning provides feedback storage and processing for detection accuracy
package learning

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// FeedbackType represents the type of feedback
type FeedbackType string

const (
	TruePositive   FeedbackType = "TRUE_POSITIVE"
	FalsePositive  FeedbackType = "FALSE_POSITIVE"
	FalseNegative  FeedbackType = "FALSE_NEGATIVE"
	WrongIdentity  FeedbackType = "WRONG_IDENTITY"
	WrongZone      FeedbackType = "WRONG_ZONE"
)

// EventType represents the type of detection event
type EventType string

const (
	BlobDetection   EventType = "blob_detection"
	ZoneTransition  EventType = "zone_transition"
	FallAlert       EventType = "fall_alert"
	Anomaly         EventType = "anomaly"
)

// FeedbackRecord represents a single feedback entry
type FeedbackRecord struct {
	ID           string                 `json:"id"`
	EventID      string                 `json:"event_id"`
	EventType    EventType              `json:"event_type"`
	FeedbackType FeedbackType           `json:"feedback_type"`
	Details      map[string]interface{} `json:"details"`
	Timestamp    time.Time              `json:"timestamp"`
	Applied      bool                   `json:"applied"`
	ProcessedAt  *time.Time             `json:"processed_at,omitempty"`
}

// FalsePositiveFrame represents CSI data for a known false positive
type FalsePositiveFrame struct {
	LinkID     string                 `json:"link_id"`
	Timestamp  time.Time              `json:"timestamp"`
	DeltaRMS   float64                `json:"delta_rms"`
	Context    map[string]interface{} `json:"context"`
}

// FalseNegativeFrame represents CSI data for a known false negative
type FalseNegativeFrame struct {
	LinkID             string                 `json:"link_id"`
	Timestamp          time.Time              `json:"timestamp"`
	ExpectedPositionX  float64                `json:"expected_position_x"`
	ExpectedPositionY  float64                `json:"expected_position_y"`
	ExpectedPositionZ  float64                `json:"expected_position_z"`
	Context            map[string]interface{} `json:"context"`
}

// FeedbackStore persists detection feedback to SQLite
type FeedbackStore struct {
	mu   sync.RWMutex
	db   *sql.DB
	path string
}

// NewFeedbackStore creates a new feedback persistence store
func NewFeedbackStore(dbPath string) (*FeedbackStore, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)

	store := &FeedbackStore{
		db:   db,
		path: dbPath,
	}

	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

// initSchema creates the necessary tables
func (s *FeedbackStore) initSchema() error {
	schema := `
	-- Detection feedback from users
	CREATE TABLE IF NOT EXISTS detection_feedback (
		id TEXT PRIMARY KEY,
		event_id TEXT,
		event_type TEXT NOT NULL,
		feedback_type TEXT NOT NULL,
		details_json TEXT,
		timestamp INTEGER NOT NULL,
		applied INTEGER DEFAULT 0,
		processed_at INTEGER
	);

	-- Known false positive CSI frames for weight learner
	CREATE TABLE IF NOT EXISTS false_positive_frames (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		link_id TEXT NOT NULL,
		timestamp INTEGER NOT NULL,
		delta_rms REAL NOT NULL,
		context_json TEXT
	);

	-- Known false negative CSI frames for weight learner
	CREATE TABLE IF NOT EXISTS false_negative_frames (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		link_id TEXT NOT NULL,
		timestamp INTEGER NOT NULL,
		expected_position_x REAL NOT NULL,
		expected_position_y REAL NOT NULL,
		expected_position_z REAL NOT NULL,
		context_json TEXT
	);

	-- Detection accuracy metrics (weekly rollups)
	CREATE TABLE IF NOT EXISTS detection_accuracy (
		week TEXT NOT NULL,
		scope_type TEXT NOT NULL,
		scope_id TEXT NOT NULL,
		precision REAL NOT NULL,
		recall REAL NOT NULL,
		f1 REAL NOT NULL,
		tp_count INTEGER NOT NULL,
		fp_count INTEGER NOT NULL,
		fn_count INTEGER NOT NULL,
		computed_at INTEGER NOT NULL,
		PRIMARY KEY (week, scope_type, scope_id)
	);

	-- Indexes for common queries
	CREATE INDEX IF NOT EXISTS idx_feedback_applied ON detection_feedback(applied);
	CREATE INDEX IF NOT EXISTS idx_feedback_time ON detection_feedback(timestamp);
	CREATE INDEX IF NOT EXISTS idx_feedback_event ON detection_feedback(event_id);
	CREATE INDEX IF NOT EXISTS idx_fp_link_time ON false_positive_frames(link_id, timestamp);
	CREATE INDEX IF NOT EXISTS idx_fn_link_time ON false_negative_frames(link_id, timestamp);
	CREATE INDEX IF NOT EXISTS idx_accuracy_week ON detection_accuracy(week);
	`

	_, err := s.db.Exec(schema)
	return err
}

// RecordFeedback stores a new feedback entry
func (s *FeedbackStore) RecordFeedback(feedback FeedbackRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	detailsJSON, err := json.Marshal(feedback.Details)
	if err != nil {
		return fmt.Errorf("marshal details: %w", err)
	}

	var processedAt interface{}
	if feedback.ProcessedAt != nil {
		processedAt = feedback.ProcessedAt.Unix()
	}

	_, err = s.db.Exec(`
		INSERT INTO detection_feedback (id, event_id, event_type, feedback_type, details_json, timestamp, applied, processed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, feedback.ID, feedback.EventID, feedback.EventType, feedback.FeedbackType,
		string(detailsJSON), feedback.Timestamp.Unix(), boolToInt(feedback.Applied), processedAt)

	return err
}

// GetUnprocessedFeedback returns all feedback entries where applied = false
func (s *FeedbackStore) GetUnprocessedFeedback() ([]FeedbackRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, event_id, event_type, feedback_type, details_json, timestamp, applied, processed_at
		FROM detection_feedback
		WHERE applied = 0
		ORDER BY timestamp ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []FeedbackRecord
	for rows.Next() {
		var r FeedbackRecord
		var timestamp int64
		var processedAt sql.NullInt64
		var detailsJSON string

		if err := rows.Scan(&r.ID, &r.EventID, &r.EventType, &r.FeedbackType,
			&detailsJSON, &timestamp, &r.Applied, &processedAt); err != nil {
			continue
		}

		r.Timestamp = time.Unix(timestamp, 0)
		if processedAt.Valid {
			t := time.Unix(processedAt.Int64, 0)
			r.ProcessedAt = &t
		}

		if err := json.Unmarshal([]byte(detailsJSON), &r.Details); err != nil {
			r.Details = make(map[string]interface{})
		}

		records = append(records, r)
	}

	return records, nil
}

// MarkFeedbackProcessed marks feedback as processed after the learner has applied it
func (s *FeedbackStore) MarkFeedbackProcessed(ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().Unix()

	stmt, err := tx.Prepare(`
		UPDATE detection_feedback
		SET applied = 1, processed_at = ?
		WHERE id = ?
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, id := range ids {
		if _, err := stmt.Exec(now, id); err != nil {
			log.Printf("[WARN] Failed to mark feedback %s as processed: %v", id, err)
		}
	}

	return tx.Commit()
}

// AddFalsePositiveFrame adds CSI frame data for a known false positive
func (s *FeedbackStore) AddFalsePositiveFrame(frame FalsePositiveFrame) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	contextJSON, err := json.Marshal(frame.Context)
	if err != nil {
		return fmt.Errorf("marshal context: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT INTO false_positive_frames (link_id, timestamp, delta_rms, context_json)
		VALUES (?, ?, ?, ?)
	`, frame.LinkID, frame.Timestamp.Unix(), frame.DeltaRMS, string(contextJSON))

	return err
}

// AddFalseNegativeFrame adds CSI frame data for a known false negative
func (s *FeedbackStore) AddFalseNegativeFrame(frame FalseNegativeFrame) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	contextJSON, err := json.Marshal(frame.Context)
	if err != nil {
		return fmt.Errorf("marshal context: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT INTO false_negative_frames (link_id, timestamp, expected_position_x, expected_position_y, expected_position_z, context_json)
		VALUES (?, ?, ?, ?, ?, ?)
	`, frame.LinkID, frame.Timestamp.Unix(),
		frame.ExpectedPositionX, frame.ExpectedPositionY, frame.ExpectedPositionZ,
		string(contextJSON))

	return err
}

// GetFalsePositiveFrames returns all false positive frames for a link within a window
func (s *FeedbackStore) GetFalsePositiveFrames(linkID string, window time.Duration) ([]FalsePositiveFrame, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoff := time.Now().Add(-window).Unix()

	rows, err := s.db.Query(`
		SELECT link_id, timestamp, delta_rms, context_json
		FROM false_positive_frames
		WHERE link_id = ? AND timestamp >= ?
		ORDER BY timestamp ASC
	`, linkID, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var frames []FalsePositiveFrame
	for rows.Next() {
		var f FalsePositiveFrame
		var timestamp int64
		var contextJSON string

		if err := rows.Scan(&f.LinkID, &timestamp, &f.DeltaRMS, &contextJSON); err != nil {
			continue
		}

		f.Timestamp = time.Unix(timestamp, 0)
		if err := json.Unmarshal([]byte(contextJSON), &f.Context); err != nil {
			f.Context = make(map[string]interface{})
		}

		frames = append(frames, f)
	}

	return frames, nil
}

// GetFalseNegativeFrames returns all false negative frames for a link within a window
func (s *FeedbackStore) GetFalseNegativeFrames(linkID string, window time.Duration) ([]FalseNegativeFrame, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoff := time.Now().Add(-window).Unix()

	rows, err := s.db.Query(`
		SELECT link_id, timestamp, expected_position_x, expected_position_y, expected_position_z, context_json
		FROM false_negative_frames
		WHERE link_id = ? AND timestamp >= ?
		ORDER BY timestamp ASC
	`, linkID, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var frames []FalseNegativeFrame
	for rows.Next() {
		var f FalseNegativeFrame
		var timestamp int64
		var contextJSON string

		if err := rows.Scan(&f.LinkID, &timestamp, &f.ExpectedPositionX, &f.ExpectedPositionY,
			&f.ExpectedPositionZ, &contextJSON); err != nil {
			continue
		}

		f.Timestamp = time.Unix(timestamp, 0)
		if err := json.Unmarshal([]byte(contextJSON), &f.Context); err != nil {
			f.Context = make(map[string]interface{})
		}

		frames = append(frames, f)
	}

	return frames, nil
}

// AccuracyRecord represents weekly accuracy metrics for a scope
type AccuracyRecord struct {
	Week      string    `json:"week"`
	ScopeType string    `json:"scope_type"`
	ScopeID   string    `json:"scope_id"`
	Precision float64   `json:"precision"`
	Recall    float64   `json:"recall"`
	F1        float64   `json:"f1"`
	TPCount   int       `json:"tp_count"`
	FPCount   int       `json:"fp_count"`
	FNCount   int       `json:"fn_count"`
	ComputedAt time.Time `json:"computed_at"`
}

// SaveAccuracyRecord saves a weekly accuracy record
func (s *FeedbackStore) SaveAccuracyRecord(record AccuracyRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO detection_accuracy
			(week, scope_type, scope_id, precision, recall, f1, tp_count, fp_count, fn_count, computed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, record.Week, record.ScopeType, record.ScopeID, record.Precision, record.Recall,
		record.F1, record.TPCount, record.FPCount, record.FNCount, record.ComputedAt.Unix())

	return err
}

// GetAccuracyHistory returns accuracy records for a scope over time
func (s *FeedbackStore) GetAccuracyHistory(scopeType, scopeID string, weeks int) ([]AccuracyRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT week, scope_type, scope_id, precision, recall, f1, tp_count, fp_count, fn_count, computed_at
		FROM detection_accuracy
		WHERE scope_type = ? AND scope_id = ?
		ORDER BY week DESC
		LIMIT ?
	`, scopeType, scopeID, weeks)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []AccuracyRecord
	for rows.Next() {
		var r AccuracyRecord
		var computedAt int64

		if err := rows.Scan(&r.Week, &r.ScopeType, &r.ScopeID, &r.Precision, &r.Recall,
			&r.F1, &r.TPCount, &r.FPCount, &r.FNCount, &computedAt); err != nil {
			continue
		}

		r.ComputedAt = time.Unix(computedAt, 0)
		records = append(records, r)
	}

	// Reverse to get chronological order
	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}

	return records, nil
}

// GetAllAccuracyRecords returns all accuracy records for a week
func (s *FeedbackStore) GetAllAccuracyRecords(week string) ([]AccuracyRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT week, scope_type, scope_id, precision, recall, f1, tp_count, fp_count, fn_count, computed_at
		FROM detection_accuracy
		WHERE week = ?
		ORDER BY scope_type, scope_id
	`, week)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []AccuracyRecord
	for rows.Next() {
		var r AccuracyRecord
		var computedAt int64

		if err := rows.Scan(&r.Week, &r.ScopeType, &r.ScopeID, &r.Precision, &r.Recall,
			&r.F1, &r.TPCount, &r.FPCount, &r.FNCount, &computedAt); err != nil {
			continue
		}

		r.ComputedAt = time.Unix(computedAt, 0)
		records = append(records, r)
	}

	return records, nil
}

// GetFeedbackStats returns overall feedback statistics
func (s *FeedbackStore) GetFeedbackStats() (map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := make(map[string]interface{})

	// Total feedback count
	var totalCount int
	row := s.db.QueryRow(`SELECT COUNT(*) FROM detection_feedback`)
	if err := row.Scan(&totalCount); err == nil {
		stats["total_count"] = totalCount
	}

	// Unprocessed count
	var unprocessedCount int
	row = s.db.QueryRow(`SELECT COUNT(*) FROM detection_feedback WHERE applied = 0`)
	if err := row.Scan(&unprocessedCount); err == nil {
		stats["unprocessed_count"] = unprocessedCount
	}

	// Processed count
	var processedCount int
	row = s.db.QueryRow(`SELECT COUNT(*) FROM detection_feedback WHERE applied = 1`)
	if err := row.Scan(&processedCount); err == nil {
		stats["processed_count"] = processedCount
	}

	// Count by feedback type
	typeRows, err := s.db.Query(`
		SELECT feedback_type, COUNT(*) as count
		FROM detection_feedback
		GROUP BY feedback_type
	`)
	if err == nil {
		defer typeRows.Close()
		byType := make(map[string]int)
		for typeRows.Next() {
			var ft string
			var count int
			if err := typeRows.Scan(&ft, &count); err == nil {
				byType[ft] = count
			}
		}
		stats["by_type"] = byType
	}

	// This week's feedback count
	weekStart := getWeekStart(time.Now()).Unix()
	var weekCount int
	row = s.db.QueryRow(`SELECT COUNT(*) FROM detection_feedback WHERE timestamp >= ?`, weekStart)
	if err := row.Scan(&weekCount); err == nil {
		stats["this_week_count"] = weekCount
	}

	return stats, nil
}

// GetFeedbackByEvent returns feedback for a specific event
func (s *FeedbackStore) GetFeedbackByEvent(eventID string) ([]FeedbackRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, event_id, event_type, feedback_type, details_json, timestamp, applied, processed_at
		FROM detection_feedback
		WHERE event_id = ?
		ORDER BY timestamp DESC
	`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []FeedbackRecord
	for rows.Next() {
		var r FeedbackRecord
		var timestamp int64
		var processedAt sql.NullInt64
		var detailsJSON string

		if err := rows.Scan(&r.ID, &r.EventID, &r.EventType, &r.FeedbackType,
			&detailsJSON, &timestamp, &r.Applied, &processedAt); err != nil {
			continue
		}

		r.Timestamp = time.Unix(timestamp, 0)
		if processedAt.Valid {
			t := time.Unix(processedAt.Int64, 0)
			r.ProcessedAt = &t
		}

		if err := json.Unmarshal([]byte(detailsJSON), &r.Details); err != nil {
			r.Details = make(map[string]interface{})
		}

		records = append(records, r)
	}

	return records, nil
}

// GetFeedbackCount returns the total number of feedback entries
func (s *FeedbackStore) GetFeedbackCount() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	row := s.db.QueryRow(`SELECT COUNT(*) FROM detection_feedback`)
	err := row.Scan(&count)
	return count, err
}

// Close closes the database connection
func (s *FeedbackStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Close()
}

// getWeekStart returns the Monday of the week containing t
func getWeekStart(t time.Time) time.Time {
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return t.AddDate(0, 0, -weekday+1).Truncate(24 * time.Hour)
}

// GetWeekString returns the ISO week string for a time (e.g., "2026-W13")
func GetWeekString(t time.Time) string {
	year, week := t.ISOWeek()
	return fmt.Sprintf("%d-W%02d", year, week)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
