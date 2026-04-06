// Package prediction provides presence prediction using transition probability models.
package prediction

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

// ZoneTransition represents a recorded zone transition event.
type ZoneTransition struct {
	ID                 string    `json:"id"`
	PersonID           string    `json:"person_id"`
	FromZoneID         string    `json:"from_zone_id"`
	ToZoneID           string    `json:"to_zone_id"`
	HourOfWeek         int       `json:"hour_of_week"` // 0-167: day_of_week * 24 + hour_of_day
	DwellDurationMinutes float64 `json:"dwell_duration_minutes"`
	Timestamp          time.Time `json:"timestamp"`
}

// TransitionProbability represents the probability of transitioning from one zone to another.
type TransitionProbability struct {
	PersonID     string    `json:"person_id"`
	HourOfWeek   int       `json:"hour_of_week"`
	FromZoneID   string    `json:"from_zone_id"`
	ToZoneID     string    `json:"to_zone_id"`
	Probability  float64   `json:"probability"`
	Count        int       `json:"count"`
	LastComputed time.Time `json:"last_computed"`
}

// DwellTimeStats represents typical dwell time statistics for a zone.
type DwellTimeStats struct {
	PersonID      string    `json:"person_id"`
	ZoneID        string    `json:"zone_id"`
	HourOfWeek    int       `json:"hour_of_week"`
	MeanMinutes   float64   `json:"mean_minutes"`
	StddevMinutes float64   `json:"stddev_minutes"`
	Count         int       `json:"count"`
	LastComputed  time.Time `json:"last_computed"`
}

// PersonPrediction represents a prediction for a person's next movement.
type PersonPrediction struct {
	PersonID                   string  `json:"person_id"`
	PersonLabel                string  `json:"person_label"`
	CurrentZoneID              string  `json:"current_zone_id"`
	CurrentZoneName            string  `json:"current_zone_name"`
	PredictedNextZoneID        string  `json:"predicted_next_zone_id"`
	PredictedNextZoneName      string  `json:"predicted_next_zone_name"`
	PredictionConfidence       float64 `json:"prediction_confidence"`
	EstimatedTransitionMinutes float64 `json:"estimated_transition_minutes"`
	DataConfidence             string  `json:"data_confidence"` // "sufficient" or "insufficient_data"
	SampleCount                int     `json:"sample_count"`
	DaysRemaining              int     `json:"days_remaining,omitempty"` // days until 7 days of data
}

// ModelStore handles persistence of transition history and probabilities.
type ModelStore struct {
	mu   sync.RWMutex
	db   *sql.DB
	path string

	// Cached data age for determining if recomputation is needed
	firstTransitionTime time.Time
}

// NewModelStore creates a new prediction model store.
func NewModelStore(dbPath string) (*ModelStore, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	s := &ModelStore{
		db:   db,
		path: dbPath,
	}

	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	// Load first transition time
	s.loadFirstTransitionTime()

	return s, nil
}

func (s *ModelStore) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS zone_transitions_history (
			id                    TEXT PRIMARY KEY,
			person_id             TEXT    NOT NULL,
			from_zone_id          TEXT    NOT NULL,
			to_zone_id            TEXT    NOT NULL,
			hour_of_week          INTEGER NOT NULL,
			dwell_duration_minutes REAL    NOT NULL DEFAULT 0,
			timestamp             INTEGER NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_transitions_person_time ON zone_transitions_history(person_id, hour_of_week);
		CREATE INDEX IF NOT EXISTS idx_transitions_from ON zone_transitions_history(from_zone_id);
		CREATE INDEX IF NOT EXISTS idx_transitions_timestamp ON zone_transitions_history(timestamp);

		CREATE TABLE IF NOT EXISTS transition_probabilities (
			person_id     TEXT    NOT NULL,
			hour_of_week  INTEGER NOT NULL,
			from_zone_id  TEXT    NOT NULL,
			to_zone_id    TEXT    NOT NULL,
			probability   REAL    NOT NULL,
			count         INTEGER NOT NULL,
			last_computed INTEGER NOT NULL,
			PRIMARY KEY (person_id, hour_of_week, from_zone_id, to_zone_id)
		);

		CREATE TABLE IF NOT EXISTS dwell_times (
			person_id       TEXT    NOT NULL,
			zone_id         TEXT    NOT NULL,
			hour_of_week    INTEGER NOT NULL,
			mean_minutes    REAL    NOT NULL,
			stddev_minutes  REAL    NOT NULL,
			count           INTEGER NOT NULL,
			last_computed   INTEGER NOT NULL,
			PRIMARY KEY (person_id, zone_id, hour_of_week)
		);

		CREATE TABLE IF NOT EXISTS person_zone_entry (
			person_id     TEXT    NOT NULL,
			zone_id       TEXT    NOT NULL,
			entry_time    INTEGER NOT NULL,
			blob_id       INTEGER NOT NULL,
			PRIMARY KEY (person_id, zone_id)
		);
	`)
	return err
}

func (s *ModelStore) loadFirstTransitionTime() {
	var ts int64
	err := s.db.QueryRow(`SELECT MIN(timestamp) FROM zone_transitions_history`).Scan(&ts)
	if err == nil && ts > 0 {
		s.firstTransitionTime = time.Unix(0, ts)
	}
}

// Close closes the database.
func (s *ModelStore) Close() error {
	return s.db.Close()
}

// RecordTransition records a zone transition event.
func (s *ModelStore) RecordTransition(t ZoneTransition) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := fmt.Sprintf("%s-%d", t.PersonID, t.Timestamp.UnixNano())
	_, err := s.db.Exec(`
		INSERT INTO zone_transitions_history (id, person_id, from_zone_id, to_zone_id, hour_of_week, dwell_duration_minutes, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, id, t.PersonID, t.FromZoneID, t.ToZoneID, t.HourOfWeek, t.DwellDurationMinutes, t.Timestamp.UnixNano())

	if err != nil {
		return err
	}

	// Update first transition time if needed
	if s.firstTransitionTime.IsZero() || t.Timestamp.Before(s.firstTransitionTime) {
		s.firstTransitionTime = t.Timestamp
	}

	return nil
}

// GetTransitions retrieves transitions for a person in a specific hour_of_week slot.
func (s *ModelStore) GetTransitions(personID string, hourOfWeek int) ([]ZoneTransition, error) {
	rows, err := s.db.Query(`
		SELECT id, person_id, from_zone_id, to_zone_id, hour_of_week, dwell_duration_minutes, timestamp
		FROM zone_transitions_history
		WHERE person_id = ? AND hour_of_week = ?
		ORDER BY timestamp DESC
	`, personID, hourOfWeek)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transitions []ZoneTransition
	for rows.Next() {
		var t ZoneTransition
		var ts int64
		if err := rows.Scan(&t.ID, &t.PersonID, &t.FromZoneID, &t.ToZoneID, &t.HourOfWeek, &t.DwellDurationMinutes, &ts); err != nil {
			continue
		}
		t.Timestamp = time.Unix(0, ts)
		transitions = append(transitions, t)
	}
	return transitions, nil
}

// GetTransitionsForSlot retrieves all transitions from a specific zone in an hour slot.
func (s *ModelStore) GetTransitionsForSlot(personID, fromZoneID string, hourOfWeek int) ([]ZoneTransition, error) {
	rows, err := s.db.Query(`
		SELECT id, person_id, from_zone_id, to_zone_id, hour_of_week, dwell_duration_minutes, timestamp
		FROM zone_transitions_history
		WHERE person_id = ? AND from_zone_id = ? AND hour_of_week = ?
		ORDER BY timestamp DESC
	`, personID, fromZoneID, hourOfWeek)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transitions []ZoneTransition
	for rows.Next() {
		var t ZoneTransition
		var ts int64
		if err := rows.Scan(&t.ID, &t.PersonID, &t.FromZoneID, &t.ToZoneID, &t.HourOfWeek, &t.DwellDurationMinutes, &ts); err != nil {
			continue
		}
		t.Timestamp = time.Unix(0, ts)
		transitions = append(transitions, t)
	}
	return transitions, nil
}

// GetDwellTimes retrieves dwell time records for a person in a zone at a specific hour.
func (s *ModelStore) GetDwellTimes(personID, zoneID string, hourOfWeek int) ([]float64, error) {
	rows, err := s.db.Query(`
		SELECT dwell_duration_minutes
		FROM zone_transitions_history
		WHERE person_id = ? AND from_zone_id = ? AND hour_of_week = ? AND dwell_duration_minutes > 0
	`, personID, zoneID, hourOfWeek)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var times []float64
	for rows.Next() {
		var t float64
		if err := rows.Scan(&t); err != nil {
			continue
		}
		times = append(times, t)
	}
	return times, nil
}

// SaveTransitionProbabilities saves computed probabilities.
func (s *ModelStore) SaveTransitionProbabilities(probs []TransitionProbability) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UnixNano()
	for _, p := range probs {
		_, err := tx.Exec(`
			INSERT OR REPLACE INTO transition_probabilities
			(person_id, hour_of_week, from_zone_id, to_zone_id, probability, count, last_computed)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, p.PersonID, p.HourOfWeek, p.FromZoneID, p.ToZoneID, p.Probability, p.Count, now)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// SaveDwellTimeStats saves computed dwell time statistics.
func (s *ModelStore) SaveDwellTimeStats(stats []DwellTimeStats) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UnixNano()
	for _, s := range stats {
		_, err := tx.Exec(`
			INSERT OR REPLACE INTO dwell_times
			(person_id, zone_id, hour_of_week, mean_minutes, stddev_minutes, count, last_computed)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, s.PersonID, s.ZoneID, s.HourOfWeek, s.MeanMinutes, s.StddevMinutes, s.Count, now)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetTransitionProbability retrieves the probability for a specific transition.
func (s *ModelStore) GetTransitionProbability(personID, fromZoneID, toZoneID string, hourOfWeek int) (*TransitionProbability, error) {
	row := s.db.QueryRow(`
		SELECT person_id, hour_of_week, from_zone_id, to_zone_id, probability, count, last_computed
		FROM transition_probabilities
		WHERE person_id = ? AND from_zone_id = ? AND to_zone_id = ? AND hour_of_week = ?
	`, personID, fromZoneID, toZoneID, hourOfWeek)

	var p TransitionProbability
	var lastComputed int64
	err := row.Scan(&p.PersonID, &p.HourOfWeek, &p.FromZoneID, &p.ToZoneID, &p.Probability, &p.Count, &lastComputed)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.LastComputed = time.Unix(0, lastComputed)
	return &p, nil
}

// GetTransitionProbabilitiesForFromZone retrieves all probabilities from a zone at an hour slot.
func (s *ModelStore) GetTransitionProbabilitiesForFromZone(personID, fromZoneID string, hourOfWeek int) ([]TransitionProbability, error) {
	rows, err := s.db.Query(`
		SELECT person_id, hour_of_week, from_zone_id, to_zone_id, probability, count, last_computed
		FROM transition_probabilities
		WHERE person_id = ? AND from_zone_id = ? AND hour_of_week = ?
		ORDER BY probability DESC
	`, personID, fromZoneID, hourOfWeek)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var probs []TransitionProbability
	for rows.Next() {
		var p TransitionProbability
		var lastComputed int64
		if err := rows.Scan(&p.PersonID, &p.HourOfWeek, &p.FromZoneID, &p.ToZoneID, &p.Probability, &p.Count, &lastComputed); err != nil {
			continue
		}
		p.LastComputed = time.Unix(0, lastComputed)
		probs = append(probs, p)
	}
	return probs, nil
}

// GetDwellTimeStats retrieves dwell time statistics for a person in a zone at an hour.
func (store *ModelStore) GetDwellTimeStats(personID, zoneID string, hourOfWeek int) (*DwellTimeStats, error) {
	row := store.db.QueryRow(`
		SELECT person_id, zone_id, hour_of_week, mean_minutes, stddev_minutes, count, last_computed
		FROM dwell_times
		WHERE person_id = ? AND zone_id = ? AND hour_of_week = ?
	`, personID, zoneID, hourOfWeek)

	var stats DwellTimeStats
	var lastComputed int64
	err := row.Scan(&stats.PersonID, &stats.ZoneID, &stats.HourOfWeek, &stats.MeanMinutes, &stats.StddevMinutes, &stats.Count, &lastComputed)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	stats.LastComputed = time.Unix(0, lastComputed)
	return &stats, nil
}

// GetDataAge returns the duration since the first transition was recorded.
func (s *ModelStore) GetDataAge() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.firstTransitionTime.IsZero() {
		return 0
	}
	return time.Since(s.firstTransitionTime)
}

// GetTransitionCount returns the total number of transitions recorded.
func (s *ModelStore) GetTransitionCount() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM zone_transitions_history`).Scan(&count)
	return count, err
}

// GetTransitionCountForSlot returns the number of transitions for a specific slot.
func (s *ModelStore) GetTransitionCountForSlot(personID, fromZoneID string, hourOfWeek int) (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM zone_transitions_history
		WHERE person_id = ? AND from_zone_id = ? AND hour_of_week = ?
	`, personID, fromZoneID, hourOfWeek).Scan(&count)
	return count, err
}

// UpdatePersonZoneEntry updates the entry time for a person in a zone.
func (s *ModelStore) UpdatePersonZoneEntry(personID, zoneID string, entryTime time.Time, blobID int) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO person_zone_entry (person_id, zone_id, entry_time, blob_id)
		VALUES (?, ?, ?, ?)
	`, personID, zoneID, entryTime.UnixNano(), blobID)
	return err
}

// GetPersonZoneEntry retrieves the entry time for a person in a zone.
func (s *ModelStore) GetPersonZoneEntry(personID, zoneID string) (time.Time, int, error) {
	var entryTime int64
	var blobID int
	err := s.db.QueryRow(`
		SELECT entry_time, blob_id FROM person_zone_entry
		WHERE person_id = ? AND zone_id = ?
	`, personID, zoneID).Scan(&entryTime, &blobID)
	if err == sql.ErrNoRows {
		return time.Time{}, 0, nil
	}
	if err != nil {
		return time.Time{}, 0, err
	}
	return time.Unix(0, entryTime), blobID, nil
}

// ClearPersonZoneEntry clears the entry time for a person leaving a zone.
func (s *ModelStore) ClearPersonZoneEntry(personID, zoneID string) error {
	_, err := s.db.Exec(`
		DELETE FROM person_zone_entry WHERE person_id = ? AND zone_id = ?
	`, personID, zoneID)
	return err
}

// GetAllPersonZoneEntries returns all current zone entries.
func (s *ModelStore) GetAllPersonZoneEntries() (map[string]struct {
	ZoneID    string
	EntryTime time.Time
	BlobID    int
}, error) {
	rows, err := s.db.Query(`SELECT person_id, zone_id, entry_time, blob_id FROM person_zone_entry`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make(map[string]struct {
		ZoneID    string
		EntryTime time.Time
		BlobID    int
	})

	for rows.Next() {
		var personID, zoneID string
		var entryTime int64
		var blobID int
		if err := rows.Scan(&personID, &zoneID, &entryTime, &blobID); err != nil {
			continue
		}
		entries[personID] = struct {
			ZoneID    string
			EntryTime time.Time
			BlobID    int
		}{
			ZoneID:    zoneID,
			EntryTime: time.Unix(0, entryTime),
			BlobID:    blobID,
		}
	}
	return entries, nil
}

// HourOfWeek computes the hour-of-week (0-167) from a timestamp.
func HourOfWeek(t time.Time) int {
	weekday := int(t.Weekday()) // 0=Sunday
	hour := t.Hour()
	return weekday*24 + hour
}

// DayNameFromHourOfWeek returns the day name for an hour_of_week value.
func DayNameFromHourOfWeek(hourOfWeek int) string {
	days := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	if hourOfWeek < 0 || hourOfWeek >= 168 {
		return "Unknown"
	}
	return days[hourOfWeek/24]
}

// RecomputeProbabilities recomputes all transition probabilities using Laplace smoothing.
func (s *ModelStore) RecomputeProbabilities() error {
	log.Printf("[INFO] prediction: recomputing transition probabilities")

	// Get all unique (person_id, hour_of_week, from_zone_id) groups
	rows, err := s.db.Query(`
		SELECT DISTINCT person_id, hour_of_week, from_zone_id
		FROM zone_transitions_history
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type groupKey struct {
		personID   string
		hourOfWeek int
		fromZoneID string
	}
	var groups []groupKey
	for rows.Next() {
		var g groupKey
		if err := rows.Scan(&g.personID, &g.hourOfWeek, &g.fromZoneID); err != nil {
			continue
		}
		groups = append(groups, g)
	}

	var allProbs []TransitionProbability

	for _, g := range groups {
		// Get all transitions for this group
		transRows, err := s.db.Query(`
			SELECT to_zone_id, COUNT(*) as cnt
			FROM zone_transitions_history
			WHERE person_id = ? AND hour_of_week = ? AND from_zone_id = ?
			GROUP BY to_zone_id
		`, g.personID, g.hourOfWeek, g.fromZoneID)
		if err != nil {
			continue
		}

		type destCount struct {
			toZoneID string
			count    int
		}
		var destCounts []destCount
		var totalCount int
		for transRows.Next() {
			var dc destCount
			if err := transRows.Scan(&dc.toZoneID, &dc.count); err != nil {
				continue
			}
			destCounts = append(destCounts, dc)
			totalCount += dc.count
		}
		transRows.Close()

		if totalCount == 0 {
			continue
		}

		// Apply Laplace smoothing: add 1 to each count
		numDests := len(destCounts)
		smoothedTotal := totalCount + numDests

		for _, dc := range destCounts {
			prob := float64(dc.count+1) / float64(smoothedTotal)
			allProbs = append(allProbs, TransitionProbability{
				PersonID:    g.personID,
				HourOfWeek:  g.hourOfWeek,
				FromZoneID:  g.fromZoneID,
				ToZoneID:    dc.toZoneID,
				Probability: prob,
				Count:       dc.count,
			})
		}
	}

	if len(allProbs) > 0 {
		if err := s.SaveTransitionProbabilities(allProbs); err != nil {
			return err
		}
	}

	log.Printf("[INFO] prediction: computed %d transition probabilities", len(allProbs))
	return nil
}

// RecomputeDwellTimes recomputes all dwell time statistics.
func (s *ModelStore) RecomputeDwellTimes() error {
	log.Printf("[INFO] prediction: recomputing dwell time statistics")

	// Get all unique (person_id, zone_id, hour_of_week) groups
	rows, err := s.db.Query(`
		SELECT DISTINCT person_id, from_zone_id, hour_of_week
		FROM zone_transitions_history
		WHERE dwell_duration_minutes > 0
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type groupKey struct {
		personID   string
		zoneID     string
		hourOfWeek int
	}
	var groups []groupKey
	for rows.Next() {
		var g groupKey
		if err := rows.Scan(&g.personID, &g.zoneID, &g.hourOfWeek); err != nil {
			continue
		}
		groups = append(groups, g)
	}

	var allStats []DwellTimeStats

	for _, g := range groups {
		// Get all dwell times for this group
		times, err := s.GetDwellTimes(g.personID, g.zoneID, g.hourOfWeek)
		if err != nil || len(times) == 0 {
			continue
		}

		// Compute mean and stddev
		var sum, sumSq float64
		for _, t := range times {
			sum += t
			sumSq += t * t
		}
		n := float64(len(times))
		mean := sum / n
		variance := sumSq/n - mean*mean
		if variance < 0 {
			variance = 0
		}
		stddev := sqrt(variance)

		allStats = append(allStats, DwellTimeStats{
			PersonID:      g.personID,
			ZoneID:        g.zoneID,
			HourOfWeek:    g.hourOfWeek,
			MeanMinutes:   mean,
			StddevMinutes: stddev,
			Count:         len(times),
		})
	}

	if len(allStats) > 0 {
		if err := s.SaveDwellTimeStats(allStats); err != nil {
			return err
		}
	}

	log.Printf("[INFO] prediction: computed %d dwell time statistics", len(allStats))
	return nil
}

func sqrt(x float64) float64 {
	if x < 0 {
		return 0
	}
	// Newton's method for sqrt
	z := 1.0
	for i := 0; i < 100; i++ {
		z -= (z*z - x) / (2 * z)
	}
	return z
}
