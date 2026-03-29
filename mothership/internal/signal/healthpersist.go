// Package signal implements link health persistence for weekly trends and diagnostics
package signal

import (
	"context"
	"database/sql"
	"log"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// HealthStore persists link health data to SQLite for diagnostics and trends
type HealthStore struct {
	mu   sync.RWMutex
	db   *sql.DB
	path string
}

// NewHealthStore creates a new health persistence store
func NewHealthStore(dbPath string) (*HealthStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	// Single-writer mode for SQLite
	db.SetMaxOpenConns(1)

	store := &HealthStore{
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
func (s *HealthStore) initSchema() error {
	schema := `
	-- Per-link health log (sampled every minute)
	CREATE TABLE IF NOT EXISTS link_health_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		link_id TEXT NOT NULL,
		timestamp INTEGER NOT NULL,
		snr REAL NOT NULL,
		phase_stability REAL NOT NULL,
		packet_rate REAL NOT NULL,
		drift_rate REAL NOT NULL,
		composite_score REAL NOT NULL,
		delta_rms_variance REAL DEFAULT 0,
		is_quiet_period INTEGER DEFAULT 0
	);

	-- Daily health summaries for weekly trends
	CREATE TABLE IF NOT EXISTS link_health_daily (
		link_id TEXT NOT NULL,
		date TEXT NOT NULL,
		avg_health REAL NOT NULL,
		min_health REAL NOT NULL,
		max_health REAL NOT NULL,
		avg_snr REAL NOT NULL,
		avg_phase_stability REAL NOT NULL,
		avg_packet_rate REAL NOT NULL,
		sample_count INTEGER NOT NULL,
		PRIMARY KEY (link_id, date)
	);

	-- Feedback events for diagnostic Rule 4
	CREATE TABLE IF NOT EXISTS feedback_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		link_id TEXT NOT NULL,
		event_type TEXT NOT NULL,
		pos_x REAL NOT NULL,
		pos_y REAL NOT NULL,
		pos_z REAL NOT NULL,
		timestamp INTEGER NOT NULL
	);

	-- Indexes for common queries
	CREATE INDEX IF NOT EXISTS idx_health_log_link_time ON link_health_log(link_id, timestamp);
	CREATE INDEX IF NOT EXISTS idx_health_daily_link ON link_health_daily(link_id);
	CREATE INDEX IF NOT EXISTS idx_feedback_link_time ON feedback_events(link_id, timestamp);
	`

	_, err := s.db.Exec(schema)
	return err
}

// HealthLogEntry represents a health log sample
type HealthLogEntry struct {
	LinkID           string
	Timestamp        time.Time
	SNR              float64
	PhaseStability   float64
	PacketRate       float64
	DriftRate        float64
	CompositeScore   float64
	DeltaRMSVariance float64
	IsQuietPeriod    bool
}

// DailyHealthSummary represents aggregated daily health metrics
type DailyHealthSummary struct {
	LinkID            string
	Date              time.Time
	AvgHealth         float64
	MinHealth         float64
	MaxHealth         float64
	AvgSNR            float64
	AvgPhaseStability float64
	AvgPacketRate     float64
	SampleCount       int
}

// FeedbackEventRecord represents a feedback event for storage
type FeedbackEventRecord struct {
	LinkID    string
	EventType string
	PosX      float64
	PosY      float64
	PosZ      float64
	Timestamp time.Time
}

// LogHealth samples health metrics to the log
func (s *HealthStore) LogHealth(entry HealthLogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		INSERT INTO link_health_log
			(link_id, timestamp, snr, phase_stability, packet_rate, drift_rate, composite_score, delta_rms_variance, is_quiet_period)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, entry.LinkID, entry.Timestamp.Unix(), entry.SNR, entry.PhaseStability,
		entry.PacketRate, entry.DriftRate, entry.CompositeScore,
		entry.DeltaRMSVariance, boolToInt(entry.IsQuietPeriod))

	return err
}

// LogHealthBatch logs multiple health entries in a single transaction
func (s *HealthStore) LogHealthBatch(entries []HealthLogEntry) error {
	if len(entries) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO link_health_log
			(link_id, timestamp, snr, phase_stability, packet_rate, drift_rate, composite_score, delta_rms_variance, is_quiet_period)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, entry := range entries {
		_, err = stmt.Exec(entry.LinkID, entry.Timestamp.Unix(), entry.SNR, entry.PhaseStability,
			entry.PacketRate, entry.DriftRate, entry.CompositeScore,
			entry.DeltaRMSVariance, boolToInt(entry.IsQuietPeriod))
		if err != nil {
			log.Printf("[WARN] Failed to log health for %s: %v", entry.LinkID, err)
		}
	}

	return tx.Commit()
}

// GetHealthHistory returns health samples for a link within a time window
func (s *HealthStore) GetHealthHistory(linkID string, window time.Duration) ([]HealthLogEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoff := time.Now().Add(-window).Unix()

	rows, err := s.db.Query(`
		SELECT link_id, timestamp, snr, phase_stability, packet_rate, drift_rate, composite_score, delta_rms_variance, is_quiet_period
		FROM link_health_log
		WHERE link_id = ? AND timestamp >= ?
		ORDER BY timestamp ASC
	`, linkID, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []HealthLogEntry
	for rows.Next() {
		var entry HealthLogEntry
		var timestamp int64
		var isQuiet int

		if err := rows.Scan(&entry.LinkID, &timestamp, &entry.SNR, &entry.PhaseStability,
			&entry.PacketRate, &entry.DriftRate, &entry.CompositeScore,
			&entry.DeltaRMSVariance, &isQuiet); err != nil {
			continue
		}

		entry.Timestamp = time.Unix(timestamp, 0)
		entry.IsQuietPeriod = isQuiet != 0
		entries = append(entries, entry)
	}

	return entries, nil
}

// GetRecentHealth returns the most recent health samples for all links
func (s *HealthStore) GetRecentHealth(limit int) (map[string][]HealthLogEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Get distinct link IDs
	linkRows, err := s.db.Query(`SELECT DISTINCT link_id FROM link_health_log`)
	if err != nil {
		return nil, err
	}
	defer linkRows.Close()

	var linkIDs []string
	for linkRows.Next() {
		var linkID string
		if err := linkRows.Scan(&linkID); err != nil {
			continue
		}
		linkIDs = append(linkIDs, linkID)
	}

	result := make(map[string][]HealthLogEntry)
	for _, linkID := range linkIDs {
		rows, err := s.db.Query(`
			SELECT link_id, timestamp, snr, phase_stability, packet_rate, drift_rate, composite_score, delta_rms_variance, is_quiet_period
			FROM link_health_log
			WHERE link_id = ?
			ORDER BY timestamp DESC
			LIMIT ?
		`, linkID, limit)
		if err != nil {
			continue
		}

		var entries []HealthLogEntry
		for rows.Next() {
			var entry HealthLogEntry
			var timestamp int64
			var isQuiet int

			if err := rows.Scan(&entry.LinkID, &timestamp, &entry.SNR, &entry.PhaseStability,
				&entry.PacketRate, &entry.DriftRate, &entry.CompositeScore,
				&entry.DeltaRMSVariance, &isQuiet); err != nil {
				continue
			}

			entry.Timestamp = time.Unix(timestamp, 0)
			entry.IsQuietPeriod = isQuiet != 0
			entries = append(entries, entry)
		}
		rows.Close()

		// Reverse to get chronological order
		for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
			entries[i], entries[j] = entries[j], entries[i]
		}

		result[linkID] = entries
	}

	return result, nil
}

// AggregateDaily runs daily aggregation for all links
func (s *HealthStore) AggregateDaily() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get yesterday's date
	yesterday := time.Now().Add(-24 * time.Hour).Format("2006-01-02")

	// Aggregate per-link stats for yesterday
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO link_health_daily
			(link_id, date, avg_health, min_health, max_health, avg_snr, avg_phase_stability, avg_packet_rate, sample_count)
		SELECT
			link_id,
			? as date,
			AVG(composite_score) as avg_health,
			MIN(composite_score) as min_health,
			MAX(composite_score) as max_health,
			AVG(snr) as avg_snr,
			AVG(phase_stability) as avg_phase_stability,
			AVG(packet_rate) as avg_packet_rate,
			COUNT(*) as sample_count
		FROM link_health_log
		WHERE date(timestamp, 'unixepoch') = ?
		GROUP BY link_id
	`, yesterday, yesterday)

	return err
}

// GetWeeklyTrend returns daily health summaries for the past week
func (s *HealthStore) GetWeeklyTrend(linkID string) ([]DailyHealthSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	weekAgo := time.Now().Add(-7 * 24 * time.Hour).Format("2006-01-02")

	rows, err := s.db.Query(`
		SELECT link_id, date, avg_health, min_health, max_health, avg_snr, avg_phase_stability, avg_packet_rate, sample_count
		FROM link_health_daily
		WHERE link_id = ? AND date >= ?
		ORDER BY date ASC
	`, linkID, weekAgo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []DailyHealthSummary
	for rows.Next() {
		var summary DailyHealthSummary
		var dateStr string

		if err := rows.Scan(&summary.LinkID, &dateStr, &summary.AvgHealth, &summary.MinHealth,
			&summary.MaxHealth, &summary.AvgSNR, &summary.AvgPhaseStability,
			&summary.AvgPacketRate, &summary.SampleCount); err != nil {
			continue
		}

		summary.Date, _ = time.Parse("2006-01-02", dateStr)
		summaries = append(summaries, summary)
	}

	return summaries, nil
}

// GetAllWeeklyTrends returns weekly trends for all links
func (s *HealthStore) GetAllWeeklyTrends() (map[string][]DailyHealthSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	weekAgo := time.Now().Add(-7 * 24 * time.Hour).Format("2006-01-02")

	rows, err := s.db.Query(`
		SELECT link_id, date, avg_health, min_health, max_health, avg_snr, avg_phase_stability, avg_packet_rate, sample_count
		FROM link_health_daily
		WHERE date >= ?
		ORDER BY link_id, date ASC
	`, weekAgo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]DailyHealthSummary)
	for rows.Next() {
		var summary DailyHealthSummary
		var dateStr string

		if err := rows.Scan(&summary.LinkID, &dateStr, &summary.AvgHealth, &summary.MinHealth,
			&summary.MaxHealth, &summary.AvgSNR, &summary.AvgPhaseStability,
			&summary.AvgPacketRate, &summary.SampleCount); err != nil {
			continue
		}

		summary.Date, _ = time.Parse("2006-01-02", dateStr)
		result[summary.LinkID] = append(result[summary.LinkID], summary)
	}

	return result, nil
}

// RecordFeedback records a feedback event
func (s *HealthStore) RecordFeedback(event FeedbackEventRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		INSERT INTO feedback_events (link_id, event_type, pos_x, pos_y, pos_z, timestamp)
		VALUES (?, ?, ?, ?, ?, ?)
	`, event.LinkID, event.EventType, event.PosX, event.PosY, event.PosZ, event.Timestamp.Unix())

	return err
}

// GetFeedbackEvents returns feedback events for a link within a time window
func (s *HealthStore) GetFeedbackEvents(linkID string, window time.Duration) ([]FeedbackEventRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoff := time.Now().Add(-window).Unix()

	rows, err := s.db.Query(`
		SELECT link_id, event_type, pos_x, pos_y, pos_z, timestamp
		FROM feedback_events
		WHERE link_id = ? AND timestamp >= ?
		ORDER BY timestamp ASC
	`, linkID, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []FeedbackEventRecord
	for rows.Next() {
		var event FeedbackEventRecord
		var timestamp int64

		if err := rows.Scan(&event.LinkID, &event.EventType, &event.PosX, &event.PosY,
			&event.PosZ, &timestamp); err != nil {
			continue
		}

		event.Timestamp = time.Unix(timestamp, 0)
		events = append(events, event)
	}

	return events, nil
}

// PruneOldHealthLogs removes health log entries older than the specified age
func (s *HealthStore) PruneOldHealthLogs(maxAge time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-maxAge).Unix()

	result, err := s.db.Exec(`DELETE FROM link_health_log WHERE timestamp < ?`, cutoff)
	if err != nil {
		return 0, err
	}

	deleted, _ := result.RowsAffected()
	return int(deleted), nil
}

// StartPeriodicTasks starts background tasks for daily aggregation and cleanup
func (s *HealthStore) StartPeriodicTasks(ctx context.Context) {
	go func() {
		// Run daily aggregation at midnight
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()

		lastAggregation := time.Time{}

		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				// Run aggregation at midnight (or if we missed it)
				if now.Hour() == 0 || now.Sub(lastAggregation) >= 24*time.Hour {
					if err := s.AggregateDaily(); err != nil {
						log.Printf("[WARN] Failed to aggregate daily health stats: %v", err)
					} else {
						lastAggregation = now
						log.Printf("[INFO] Daily health aggregation complete")
					}

					// Prune old logs (keep 30 days)
					deleted, err := s.PruneOldHealthLogs(30 * 24 * time.Hour)
					if err != nil {
						log.Printf("[WARN] Failed to prune health logs: %v", err)
					} else if deleted > 0 {
						log.Printf("[INFO] Pruned %d old health log entries", deleted)
					}
				}
			}
		}
	}()
}

// Close closes the database connection
func (s *HealthStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Close()
}

// GetAllLinkIDs returns all unique link IDs that have health data
func (s *HealthStore) GetAllLinkIDs() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`SELECT DISTINCT link_id FROM link_health_log ORDER BY link_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var linkIDs []string
	for rows.Next() {
		var linkID string
		if err := rows.Scan(&linkID); err != nil {
			continue
		}
		linkIDs = append(linkIDs, linkID)
	}

	return linkIDs, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
