// Package sleep provides SQLite persistence for sleep sessions.
package sleep

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Storage handles SQLite persistence for sleep sessions.
type Storage struct {
	mu   sync.RWMutex
	db   *sql.DB
	path string
}

// WakeEpisode is defined in analyzer.go - this is an alias for storage compatibility
// Storage uses the same WakeEpisode struct from analyzer

// SleepSessionRecord represents a persisted sleep session.
type SleepSessionRecord struct {
	ID                      string        `json:"id"`
	PersonID                string        `json:"person_id"`
	LinkID                  string        `json:"link_id"`
	ZoneID                  string        `json:"zone_id"`
	SessionDate             time.Time     `json:"session_date"`
	SleepOnset              time.Time     `json:"sleep_onset"`
	WakeTime                time.Time     `json:"wake_time,omitempty"`
	TimeInBedMinutes        float64       `json:"time_in_bed_minutes"`
	SleepLatencyMinutes     float64       `json:"sleep_latency_minutes"`
	WakeEpisodeCount        int           `json:"wake_episode_count"`
	WASOMinutes             float64       `json:"waso_minutes"`
	BreathingRateMean       float64       `json:"breathing_rate_mean"`
	BreathingRateStdDev     float64       `json:"breathing_rate_std_dev"`
	BreathingAnomalyCount   int           `json:"breathing_anomaly_count"`
	SleepEfficiency         float64       `json:"sleep_efficiency"`
	OverallScore            float64       `json:"overall_score"`
	QualityRating           string        `json:"quality_rating"`
	WakeEpisodes            []WakeEpisode `json:"wake_episodes,omitempty"`
}

// NewStorage creates a new storage instance.
func NewStorage(dataDir string) (*Storage, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "sleep.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	s := &Storage{
		db:   db,
		path: dbPath,
	}

	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	log.Printf("[INFO] Sleep storage initialized at %s", dbPath)
	return s, nil
}

func (s *Storage) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS sleep_sessions (
			id                       TEXT PRIMARY KEY,
			person_id                TEXT NOT NULL DEFAULT '',
			link_id                  TEXT NOT NULL,
			zone_id                  TEXT NOT NULL DEFAULT '',
			session_date             INTEGER NOT NULL,
			sleep_onset              INTEGER NOT NULL DEFAULT 0,
			wake_time                INTEGER NOT NULL DEFAULT 0,
			time_in_bed_minutes      REAL NOT NULL DEFAULT 0,
			sleep_latency_minutes    REAL NOT NULL DEFAULT 0,
			wake_episode_count       INTEGER NOT NULL DEFAULT 0,
			waso_minutes             REAL NOT NULL DEFAULT 0,
			breathing_rate_mean      REAL NOT NULL DEFAULT 0,
			breathing_rate_std_dev   REAL NOT NULL DEFAULT 0,
			breathing_anomaly_count  INTEGER NOT NULL DEFAULT 0,
			sleep_efficiency         REAL NOT NULL DEFAULT 0,
			overall_score            REAL NOT NULL DEFAULT 0,
			quality_rating           TEXT NOT NULL DEFAULT '',
			created_at               INTEGER NOT NULL DEFAULT 0,
			updated_at               INTEGER NOT NULL DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS sleep_wake_episodes (
			id             TEXT PRIMARY KEY,
			session_id     TEXT NOT NULL,
			episode_start  INTEGER NOT NULL,
			episode_end    INTEGER NOT NULL DEFAULT 0,
			duration_seconds REAL NOT NULL DEFAULT 0,
			FOREIGN KEY (session_id) REFERENCES sleep_sessions(id)
		);

		CREATE INDEX IF NOT EXISTS idx_sleep_sessions_date ON sleep_sessions(session_date);
		CREATE INDEX IF NOT EXISTS idx_sleep_sessions_person ON sleep_sessions(person_id);
		CREATE INDEX IF NOT EXISTS idx_sleep_sessions_link ON sleep_sessions(link_id);
		CREATE INDEX IF NOT EXISTS idx_wake_episodes_session ON sleep_wake_episodes(session_id);
	`)
	return err
}

// Close closes the database connection.
func (s *Storage) Close() error {
	return s.db.Close()
}

// SaveSession persists a sleep session record.
func (s *Storage) SaveSession(record *SleepSessionRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UnixNano()

	// Check if session exists
	var exists bool
	err := s.db.QueryRow(`SELECT 1 FROM sleep_sessions WHERE id = ?`, record.ID).Scan(&exists)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("check session exists: %w", err)
	}

	if exists {
		// Update existing session
		_, err = s.db.Exec(`
			UPDATE sleep_sessions SET
				person_id = ?,
				link_id = ?,
				zone_id = ?,
				session_date = ?,
				sleep_onset = ?,
				wake_time = ?,
				time_in_bed_minutes = ?,
				sleep_latency_minutes = ?,
				wake_episode_count = ?,
				waso_minutes = ?,
				breathing_rate_mean = ?,
				breathing_rate_std_dev = ?,
				breathing_anomaly_count = ?,
				sleep_efficiency = ?,
				overall_score = ?,
				quality_rating = ?,
				updated_at = ?
			WHERE id = ?
		`,
			record.PersonID,
			record.LinkID,
			record.ZoneID,
			record.SessionDate.UnixNano(),
			record.SleepOnset.UnixNano(),
			record.WakeTime.UnixNano(),
			record.TimeInBedMinutes,
			record.SleepLatencyMinutes,
			record.WakeEpisodeCount,
			record.WASOMinutes,
			record.BreathingRateMean,
			record.BreathingRateStdDev,
			record.BreathingAnomalyCount,
			record.SleepEfficiency,
			record.OverallScore,
			record.QualityRating,
			now,
			record.ID,
		)
	} else {
		// Insert new session
		_, err = s.db.Exec(`
			INSERT INTO sleep_sessions (
				id, person_id, link_id, zone_id, session_date,
				sleep_onset, wake_time, time_in_bed_minutes, sleep_latency_minutes,
				wake_episode_count, waso_minutes, breathing_rate_mean, breathing_rate_std_dev,
				breathing_anomaly_count, sleep_efficiency, overall_score, quality_rating,
				created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			record.ID,
			record.PersonID,
			record.LinkID,
			record.ZoneID,
			record.SessionDate.UnixNano(),
			record.SleepOnset.UnixNano(),
			record.WakeTime.UnixNano(),
			record.TimeInBedMinutes,
			record.SleepLatencyMinutes,
			record.WakeEpisodeCount,
			record.WASOMinutes,
			record.BreathingRateMean,
			record.BreathingRateStdDev,
			record.BreathingAnomalyCount,
			record.SleepEfficiency,
			record.OverallScore,
			record.QualityRating,
			now,
			now,
		)
	}

	if err != nil {
		return fmt.Errorf("save session: %w", err)
	}

	// Save wake episodes
	for _, episode := range record.WakeEpisodes {
		if err := s.saveWakeEpisode(episode); err != nil {
			log.Printf("[WARN] Failed to save wake episode: %v", err)
		}
	}

	return nil
}

// saveWakeEpisode persists a wake episode.
func (s *Storage) saveWakeEpisode(episode WakeEpisode) error {
	// Check if episode exists
	var exists bool
	err := s.db.QueryRow(`SELECT 1 FROM sleep_wake_episodes WHERE id = ?`, episode.ID).Scan(&exists)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("check episode exists: %w", err)
	}

	if exists {
		_, err = s.db.Exec(`
			UPDATE sleep_wake_episodes SET
				session_id = ?,
				episode_start = ?,
				episode_end = ?,
				duration_seconds = ?
			WHERE id = ?
		`,
			episode.SessionID,
			episode.EpisodeStart.UnixNano(),
			episode.EpisodeEnd.UnixNano(),
			episode.Duration.Seconds(),
			episode.ID,
		)
	} else {
		_, err = s.db.Exec(`
			INSERT INTO sleep_wake_episodes (
				id, session_id, episode_start, episode_end, duration_seconds
			) VALUES (?, ?, ?, ?, ?)
		`,
			episode.ID,
			episode.SessionID,
			episode.EpisodeStart.UnixNano(),
			episode.EpisodeEnd.UnixNano(),
			episode.Duration.Seconds(),
		)
	}

	return err
}

// GetSession retrieves a sleep session by ID.
func (s *Storage) GetSession(id string) (*SleepSessionRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	record := &SleepSessionRecord{ID: id}
	var sessionDate, sleepOnset, wakeTime int64

	err := s.db.QueryRow(`
		SELECT person_id, link_id, zone_id, session_date, sleep_onset, wake_time,
		       time_in_bed_minutes, sleep_latency_minutes, wake_episode_count,
		       waso_minutes, breathing_rate_mean, breathing_rate_std_dev,
		       breathing_anomaly_count, sleep_efficiency, overall_score, quality_rating
		FROM sleep_sessions WHERE id = ?
	`, id).Scan(
		&record.PersonID, &record.LinkID, &record.ZoneID, &sessionDate, &sleepOnset, &wakeTime,
		&record.TimeInBedMinutes, &record.SleepLatencyMinutes, &record.WakeEpisodeCount,
		&record.WASOMinutes, &record.BreathingRateMean, &record.BreathingRateStdDev,
		&record.BreathingAnomalyCount, &record.SleepEfficiency, &record.OverallScore, &record.QualityRating,
	)
	if err != nil {
		return nil, err
	}

	record.SessionDate = time.Unix(0, sessionDate)
	if sleepOnset > 0 {
		record.SleepOnset = time.Unix(0, sleepOnset)
	}
	if wakeTime > 0 {
		record.WakeTime = time.Unix(0, wakeTime)
	}

	// Load wake episodes
	episodes, err := s.getWakeEpisodes(id)
	if err != nil {
		return nil, err
	}
	record.WakeEpisodes = episodes

	return record, nil
}

// getWakeEpisodes retrieves wake episodes for a session.
func (s *Storage) getWakeEpisodes(sessionID string) ([]WakeEpisode, error) {
	rows, err := s.db.Query(`
		SELECT id, session_id, episode_start, episode_end, duration_seconds
		FROM sleep_wake_episodes WHERE session_id = ?
		ORDER BY episode_start
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var episodes []WakeEpisode
	for rows.Next() {
		var ep WakeEpisode
		var start, end int64
		if err := rows.Scan(&ep.ID, &ep.SessionID, &start, &end, &ep.DurationSeconds); err != nil {
			continue
		}
		ep.EpisodeStart = time.Unix(0, start)
		if end > 0 {
			ep.EpisodeEnd = time.Unix(0, end)
		}
		ep.Duration = time.Duration(ep.DurationSeconds * float64(time.Second))
		episodes = append(episodes, ep)
	}

	return episodes, nil
}

// GetSessionsByDateRange retrieves sessions within a date range.
func (s *Storage) GetSessionsByDateRange(start, end time.Time) ([]*SleepSessionRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, person_id, link_id, zone_id, session_date, sleep_onset, wake_time,
		       time_in_bed_minutes, sleep_latency_minutes, wake_episode_count,
		       waso_minutes, breathing_rate_mean, breathing_rate_std_dev,
		       breathing_anomaly_count, sleep_efficiency, overall_score, quality_rating
		FROM sleep_sessions
		WHERE session_date >= ? AND session_date <= ?
		ORDER BY session_date DESC
	`, start.UnixNano(), end.UnixNano())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanSessions(rows)
}

// GetSessionsByPerson retrieves all sessions for a person.
func (s *Storage) GetSessionsByPerson(personID string, limit int) ([]*SleepSessionRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT id, person_id, link_id, zone_id, session_date, sleep_onset, wake_time,
		       time_in_bed_minutes, sleep_latency_minutes, wake_episode_count,
		       waso_minutes, breathing_rate_mean, breathing_rate_std_dev,
		       breathing_anomaly_count, sleep_efficiency, overall_score, quality_rating
		FROM sleep_sessions
		WHERE person_id = ?
		ORDER BY session_date DESC
	`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.Query(query, personID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanSessions(rows)
}

// GetSessionsByLink retrieves all sessions for a link.
func (s *Storage) GetSessionsByLink(linkID string, limit int) ([]*SleepSessionRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT id, person_id, link_id, zone_id, session_date, sleep_onset, wake_time,
		       time_in_bed_minutes, sleep_latency_minutes, wake_episode_count,
		       waso_minutes, breathing_rate_mean, breathing_rate_std_dev,
		       breathing_anomaly_count, sleep_efficiency, overall_score, quality_rating
		FROM sleep_sessions
		WHERE link_id = ?
		ORDER BY session_date DESC
	`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.Query(query, linkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanSessions(rows)
}

// GetRecentSessions retrieves the most recent sessions.
func (s *Storage) GetRecentSessions(limit int) ([]*SleepSessionRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, person_id, link_id, zone_id, session_date, sleep_onset, wake_time,
		       time_in_bed_minutes, sleep_latency_minutes, wake_episode_count,
		       waso_minutes, breathing_rate_mean, breathing_rate_std_dev,
		       breathing_anomaly_count, sleep_efficiency, overall_score, quality_rating
		FROM sleep_sessions
		ORDER BY session_date DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanSessions(rows)
}

// scanSessions is a helper to scan multiple session rows.
func (s *Storage) scanSessions(rows *sql.Rows) ([]*SleepSessionRecord, error) {
	var records []*SleepSessionRecord

	for rows.Next() {
		record := &SleepSessionRecord{}
		var sessionDate, sleepOnset, wakeTime int64

		err := rows.Scan(
			&record.ID, &record.PersonID, &record.LinkID, &record.ZoneID, &sessionDate,
			&sleepOnset, &wakeTime, &record.TimeInBedMinutes, &record.SleepLatencyMinutes,
			&record.WakeEpisodeCount, &record.WASOMinutes, &record.BreathingRateMean,
			&record.BreathingRateStdDev, &record.BreathingAnomalyCount,
			&record.SleepEfficiency, &record.OverallScore, &record.QualityRating,
		)
		if err != nil {
			log.Printf("[WARN] Failed to scan session: %v", err)
			continue
		}

		record.SessionDate = time.Unix(0, sessionDate)
		if sleepOnset > 0 {
			record.SleepOnset = time.Unix(0, sleepOnset)
		}
		if wakeTime > 0 {
			record.WakeTime = time.Unix(0, wakeTime)
		}

		records = append(records, record)
	}

	return records, rows.Err()
}

// GetWeeklyTrends calculates weekly sleep trends.
func (s *Storage) GetWeeklyTrends(personID string) (*WeeklyTrends, error) {
	now := time.Now()
	weekStart := now.AddDate(0, 0, -7)

	var sessions []*SleepSessionRecord
	var err error

	if personID != "" {
		sessions, err = s.GetSessionsByPerson(personID, 7)
	} else {
		sessions, err = s.GetSessionsByDateRange(weekStart, now)
	}

	if err != nil {
		return nil, err
	}

	trends := &WeeklyTrends{
		DailyDurations:    make([]float64, 0, 7),
		DailyEfficiencies: make([]float64, 0, 7),
		DailyDates:        make([]string, 0, 7),
	}

	var totalDuration, totalEfficiency, totalBreathing float64
	var count int

	for _, session := range sessions {
		trends.DailyDurations = append(trends.DailyDurations, session.TimeInBedMinutes)
		trends.DailyEfficiencies = append(trends.DailyEfficiencies, session.SleepEfficiency)
		trends.DailyDates = append(trends.DailyDates, session.SessionDate.Format("Mon"))

		totalDuration += session.TimeInBedMinutes
		totalEfficiency += session.SleepEfficiency
		totalBreathing += session.BreathingRateMean
		count++
	}

	if count > 0 {
		trends.AvgDurationMinutes = totalDuration / float64(count)
		trends.AvgEfficiency = totalEfficiency / float64(count)
		trends.AvgBreathingRate = totalBreathing / float64(count)
		trends.NightsCount = count
	}

	return trends, nil
}

// WeeklyTrends holds aggregated weekly sleep statistics.
type WeeklyTrends struct {
	DailyDurations    []float64 `json:"daily_durations"`
	DailyEfficiencies []float64 `json:"daily_efficiencies"`
	DailyDates        []string  `json:"daily_dates"`
	AvgDurationMinutes float64  `json:"avg_duration_minutes"`
	AvgEfficiency     float64   `json:"avg_efficiency"`
	AvgBreathingRate  float64   `json:"avg_breathing_rate"`
	NightsCount       int       `json:"nights_count"`
}

// DeleteOldSessions deletes sessions older than the specified days.
func (s *Storage) DeleteOldSessions(days int) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().AddDate(0, 0, -days).UnixNano()

	result, err := s.db.Exec(`DELETE FROM sleep_sessions WHERE session_date < ?`, cutoff)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}
