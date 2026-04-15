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

// PredictionHorizon is the default prediction horizon (15 minutes).
const PredictionHorizon = 15 * time.Minute

// AccuracyWindow is the rolling window for accuracy calculation (7 days).
const AccuracyWindow = 7 * 24 * time.Hour

// MinPredictionsForAccuracy is the minimum predictions needed for valid accuracy.
const MinPredictionsForAccuracy = 10

// RecordedPrediction represents a prediction made at a specific time.
type RecordedPrediction struct {
	ID                      string    `json:"id"`
	PersonID                string    `json:"person_id"`
	PredictedAt             time.Time `json:"predicted_at"`
	TargetTime              time.Time `json:"target_time"`              // When the prediction targets
	CurrentZoneID           string    `json:"current_zone_id"`
	PredictedZoneID         string    `json:"predicted_zone_id"`        // Zone predicted at target time
	ActualZoneID            string    `json:"actual_zone_id,omitempty"` // Actual zone at target time (filled later)
	PredictionConfidence    float64   `json:"prediction_confidence"`
	HorizonMinutes          int       `json:"horizon_minutes"`
	Evaluated               bool      `json:"evaluated"`
	Correct                 bool      `json:"correct,omitempty"`
	EvaluatedAt             time.Time `json:"evaluated_at,omitempty"`
}

// AccuracyStats represents accuracy statistics for a person.
type AccuracyStats struct {
	PersonID            string    `json:"person_id"`
	HorizonMinutes      int       `json:"horizon_minutes"`
	TotalPredictions    int       `json:"total_predictions"`
	CorrectPredictions  int       `json:"correct_predictions"`
	Accuracy            float64   `json:"accuracy"`
	WindowStart         time.Time `json:"window_start"`
	WindowEnd           time.Time `json:"window_end"`
	LastUpdated         time.Time `json:"last_updated"`
	MeetsTarget         bool      `json:"meets_target"` // true if accuracy >= 75%
	ConfusionMatrix     map[string]map[string]int `json:"confusion_matrix,omitempty"` // actual -> predicted -> count
}

// ZoneOccupancyPattern represents typical occupancy patterns for a zone.
type ZoneOccupancyPattern struct {
	ZoneID           string    `json:"zone_id"`
	HourOfWeek       int       `json:"hour_of_week"`
	OccupancyProb    float64   `json:"occupancy_probability"`    // P(occupied | hour)
	MeanDwellMinutes float64   `json:"mean_dwell_minutes"`
	StddevDwell      float64   `json:"stddev_dwell_minutes"`
	SampleCount      int       `json:"sample_count"`
	LastComputed     time.Time `json:"last_computed"`
}

// AccuracyTracker tracks prediction accuracy over time.
type AccuracyTracker struct {
	mu     sync.RWMutex
	db     *sql.DB
	path   string

	// Pending predictions awaiting evaluation
	pendingPredictions map[string]RecordedPrediction // id -> prediction

	// Cached accuracy stats
	cachedStats map[string]*AccuracyStats // personID -> stats

	// Horizon for predictions
	horizon time.Duration
}

// NewAccuracyTracker creates a new accuracy tracker.
func NewAccuracyTracker(dbPath string) (*AccuracyTracker, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	t := &AccuracyTracker{
		db:                 db,
		path:               dbPath,
		pendingPredictions: make(map[string]RecordedPrediction),
		cachedStats:        make(map[string]*AccuracyStats),
		horizon:            PredictionHorizon,
	}

	if err := t.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	// Load pending predictions
	if err := t.loadPendingPredictions(); err != nil {
		log.Printf("[WARN] prediction: failed to load pending predictions: %v", err)
	}

	return t, nil
}

func (t *AccuracyTracker) migrate() error {
	_, err := t.db.Exec(`
		CREATE TABLE IF NOT EXISTS recorded_predictions (
			id                   TEXT PRIMARY KEY,
			person_id            TEXT    NOT NULL,
			predicted_at         INTEGER NOT NULL,
			target_time          INTEGER NOT NULL,
			current_zone_id      TEXT    NOT NULL,
			predicted_zone_id    TEXT    NOT NULL,
			actual_zone_id       TEXT,
			prediction_confidence REAL   NOT NULL,
			horizon_minutes      INTEGER NOT NULL,
			evaluated            INTEGER NOT NULL DEFAULT 0,
			correct              INTEGER DEFAULT 0,
			evaluated_at         INTEGER
		);

		CREATE INDEX IF NOT EXISTS idx_predictions_person ON recorded_predictions(person_id);
		CREATE INDEX IF NOT EXISTS idx_predictions_target ON recorded_predictions(target_time);
		CREATE INDEX IF NOT EXISTS idx_predictions_evaluated ON recorded_predictions(evaluated);
		CREATE INDEX IF NOT EXISTS idx_predictions_person_target ON recorded_predictions(person_id, target_time);

		CREATE TABLE IF NOT EXISTS accuracy_stats (
			person_id        TEXT    NOT NULL,
			horizon_minutes  INTEGER NOT NULL,
			total_predictions INTEGER NOT NULL,
			correct_predictions INTEGER NOT NULL,
			accuracy         REAL    NOT NULL,
			window_start     INTEGER NOT NULL,
			window_end       INTEGER NOT NULL,
			last_updated     INTEGER NOT NULL,
			PRIMARY KEY (person_id, horizon_minutes)
		);

		CREATE TABLE IF NOT EXISTS zone_occupancy_patterns (
			zone_id            TEXT    NOT NULL,
			hour_of_week       INTEGER NOT NULL,
			occupancy_prob     REAL    NOT NULL,
			mean_dwell_minutes REAL    NOT NULL,
			stddev_dwell       REAL    NOT NULL,
			sample_count       INTEGER NOT NULL,
			last_computed      INTEGER NOT NULL,
			PRIMARY KEY (zone_id, hour_of_week)
		);

		CREATE TABLE IF NOT EXISTS zone_occupancy_history (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			zone_id     TEXT    NOT NULL,
			person_id   TEXT,
			enter_time  INTEGER NOT NULL,
			exit_time   INTEGER,
			duration_minutes REAL
		);

		CREATE INDEX IF NOT EXISTS idx_occupancy_zone ON zone_occupancy_history(zone_id);
		CREATE INDEX IF NOT EXISTS idx_occupancy_enter ON zone_occupancy_history(enter_time);
	`)
	return err
}

func (t *AccuracyTracker) loadPendingPredictions() error {
	// Load predictions that haven't been evaluated yet and target time is in the future
	now := time.Now()
	rows, err := t.db.Query(`
		SELECT id, person_id, predicted_at, target_time, current_zone_id, predicted_zone_id,
		       prediction_confidence, horizon_minutes
		FROM recorded_predictions
		WHERE evaluated = 0 AND target_time > ?
	`, now.Add(-5*time.Minute).UnixNano()) // Small grace period
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var p RecordedPrediction
		var predictedAt, targetTime int64
		if err := rows.Scan(&p.ID, &p.PersonID, &predictedAt, &targetTime,
			&p.CurrentZoneID, &p.PredictedZoneID, &p.PredictionConfidence, &p.HorizonMinutes); err != nil {
			continue
		}
		p.PredictedAt = time.Unix(0, predictedAt)
		p.TargetTime = time.Unix(0, targetTime)
		t.pendingPredictions[p.ID] = p
	}

	log.Printf("[INFO] prediction: loaded %d pending predictions", len(t.pendingPredictions))
	return nil
}

// Close closes the database.
func (t *AccuracyTracker) Close() error {
	return t.db.Close()
}

// RecordPrediction records a new prediction for later evaluation.
func (t *AccuracyTracker) RecordPrediction(personID, currentZoneID, predictedZoneID string, confidence float64, horizon time.Duration) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	targetTime := now.Add(horizon)

	id := fmt.Sprintf("%s-%d-%d", personID, now.UnixNano(), int(horizon.Minutes()))

	p := RecordedPrediction{
		ID:                   id,
		PersonID:             personID,
		PredictedAt:          now,
		TargetTime:           targetTime,
		CurrentZoneID:        currentZoneID,
		PredictedZoneID:      predictedZoneID,
		PredictionConfidence: confidence,
		HorizonMinutes:       int(horizon.Minutes()),
		Evaluated:            false,
	}

	// Store in database
	_, err := t.db.Exec(`
		INSERT INTO recorded_predictions
		(id, person_id, predicted_at, target_time, current_zone_id, predicted_zone_id,
		 prediction_confidence, horizon_minutes, evaluated)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0)
	`, p.ID, p.PersonID, p.PredictedAt.UnixNano(), p.TargetTime.UnixNano(),
		p.CurrentZoneID, p.PredictedZoneID, p.PredictionConfidence, p.HorizonMinutes)

	if err != nil {
		return err
	}

	// Add to pending
	t.pendingPredictions[p.ID] = p

	log.Printf("[DEBUG] prediction: recorded prediction for %s: %s -> %s (target: %s, confidence: %.2f)",
		personID, currentZoneID, predictedZoneID, targetTime.Format("15:04:05"), confidence)

	return nil
}

// EvaluatePending checks pending predictions against actual positions.
// actualPositions is a map of personID -> zoneID at the current time.
func (t *AccuracyTracker) EvaluatePending(actualPositions map[string]string) (int, int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	var evaluated, correct int

	for id, pred := range t.pendingPredictions {
		// Check if target time has passed
		if pred.TargetTime.After(now) {
			continue
		}

		// Get actual zone
		actualZone, exists := actualPositions[pred.PersonID]
		if !exists {
			// Person not tracked - skip this prediction
			continue
		}

		// Evaluate
		isCorrect := actualZone == pred.PredictedZoneID

		// Update in database
		_, err := t.db.Exec(`
			UPDATE recorded_predictions
			SET actual_zone_id = ?, evaluated = 1, correct = ?, evaluated_at = ?
			WHERE id = ?
		`, actualZone, isCorrect, now.UnixNano(), id)

		if err != nil {
			log.Printf("[WARN] prediction: failed to update prediction %s: %v", id, err)
			continue
		}

		// Remove from pending
		delete(t.pendingPredictions, id)

		evaluated++
		if isCorrect {
			correct++
		}

		log.Printf("[DEBUG] prediction: evaluated prediction for %s: predicted=%s, actual=%s, correct=%v",
			pred.PersonID, pred.PredictedZoneID, actualZone, isCorrect)
	}

	// Invalidate cached stats
	t.cachedStats = make(map[string]*AccuracyStats)

	if evaluated > 0 {
		log.Printf("[INFO] prediction: evaluated %d predictions, %d correct (%.1f%%)",
			evaluated, correct, float64(correct)/float64(evaluated)*100)
	}

	return evaluated, correct, nil
}

// GetAccuracyStats returns accuracy statistics for a person.
func (t *AccuracyTracker) GetAccuracyStats(personID string, horizonMinutes int) (*AccuracyStats, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Check cache
	cacheKey := fmt.Sprintf("%s-%d", personID, horizonMinutes)
	if stats, ok := t.cachedStats[cacheKey]; ok {
		return stats, nil
	}

	// Calculate from database
	stats, err := t.computeAccuracyStats(personID, horizonMinutes)
	if err != nil {
		return nil, err
	}

	t.cachedStats[cacheKey] = stats
	return stats, nil
}

func (t *AccuracyTracker) computeAccuracyStats(personID string, horizonMinutes int) (*AccuracyStats, error) {
	windowStart := time.Now().Add(-AccuracyWindow)

	row := t.db.QueryRow(`
		SELECT COUNT(*), SUM(CASE WHEN correct = 1 THEN 1 ELSE 0 END)
		FROM recorded_predictions
		WHERE person_id = ? AND horizon_minutes = ? AND evaluated = 1 AND predicted_at >= ?
	`, personID, horizonMinutes, windowStart.UnixNano())

	var total, correct int
	if err := row.Scan(&total, &correct); err != nil {
		return nil, err
	}

	// Handle NULL sum
	if correct == 0 && total > 0 {
		// Re-scan to handle properly
		row = t.db.QueryRow(`
			SELECT COUNT(*), COALESCE(SUM(CASE WHEN correct = 1 THEN 1 ELSE 0 END), 0)
			FROM recorded_predictions
			WHERE person_id = ? AND horizon_minutes = ? AND evaluated = 1 AND predicted_at >= ?
		`, personID, horizonMinutes, windowStart.UnixNano())
		if err := row.Scan(&total, &correct); err != nil {
			return nil, err
		}
	}

	accuracy := 0.0
	if total > 0 {
		accuracy = float64(correct) / float64(total)
	}

	stats := &AccuracyStats{
		PersonID:           personID,
		HorizonMinutes:     horizonMinutes,
		TotalPredictions:   total,
		CorrectPredictions: correct,
		Accuracy:           accuracy,
		WindowStart:        windowStart,
		WindowEnd:          time.Now(),
		LastUpdated:        time.Now(),
		MeetsTarget:        total >= MinPredictionsForAccuracy && accuracy >= 0.75,
	}

	return stats, nil
}

// GetAllAccuracyStats returns accuracy stats for all people.
func (t *AccuracyTracker) GetAllAccuracyStats() ([]AccuracyStats, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Get distinct person_ids
	rows, err := t.db.Query(`
		SELECT DISTINCT person_id FROM recorded_predictions WHERE evaluated = 1
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var personIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		personIDs = append(personIDs, id)
	}

	var stats []AccuracyStats
	for _, personID := range personIDs {
		s, err := t.computeAccuracyStats(personID, int(t.horizon.Minutes()))
		if err != nil {
			continue
		}
		stats = append(stats, *s)
	}

	return stats, nil
}

// GetOverallAccuracy returns overall prediction accuracy across all people.
func (t *AccuracyTracker) GetOverallAccuracy() (float64, int, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	windowStart := time.Now().Add(-AccuracyWindow)

	row := t.db.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(CASE WHEN correct = 1 THEN 1 ELSE 0 END), 0)
		FROM recorded_predictions
		WHERE evaluated = 1 AND predicted_at >= ?
	`, windowStart.UnixNano())

	var total, correct int
	if err := row.Scan(&total, &correct); err != nil {
		return 0, 0, err
	}

	if total == 0 {
		return 0, 0, nil
	}

	return float64(correct) / float64(total), total, nil
}

// RecordZoneOccupancy records a zone occupancy event.
func (t *AccuracyTracker) RecordZoneOccupancy(zoneID, personID string, enterTime time.Time) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	_, err := t.db.Exec(`
		INSERT INTO zone_occupancy_history (zone_id, person_id, enter_time)
		VALUES (?, ?, ?)
	`, zoneID, personID, enterTime.UnixNano())

	return err
}

// RecordZoneExit records a zone exit event.
func (t *AccuracyTracker) RecordZoneExit(zoneID, personID string, exitTime time.Time) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Find the most recent un-closed entry for this person/zone
	result, err := t.db.Exec(`
		UPDATE zone_occupancy_history
		SET exit_time = ?, duration_minutes = (? - enter_time) / 60000000000.0
		WHERE zone_id = ? AND person_id = ? AND exit_time IS NULL
	`, exitTime.UnixNano(), exitTime.UnixNano(), zoneID, personID)

	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		log.Printf("[WARN] prediction: no open occupancy record for %s in %s", personID, zoneID)
	}

	return nil
}

// ComputeZoneOccupancyPatterns computes occupancy patterns for all zones.
func (t *AccuracyTracker) ComputeZoneOccupancyPatterns() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	log.Printf("[INFO] prediction: computing zone occupancy patterns")

	// Get all unique zone/hour combinations
	rows, err := t.db.Query(`
		SELECT DISTINCT zone_id, (CAST(strftime('%w', datetime(enter_time/1000000000, 'unixepoch', 'localtime')) AS INTEGER) * 24 +
		                           CAST(strftime('%H', datetime(enter_time/1000000000, 'unixepoch', 'localtime')) AS INTEGER)) as hour_of_week
		FROM zone_occupancy_history
		WHERE exit_time IS NOT NULL
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type zoneHour struct {
		zoneID     string
		hourOfWeek int
	}
	var combinations []zoneHour
	for rows.Next() {
		var zh zoneHour
		if err := rows.Scan(&zh.zoneID, &zh.hourOfWeek); err != nil {
			continue
		}
		combinations = append(combinations, zh)
	}

	var patterns []ZoneOccupancyPattern
	now := time.Now()

	for _, zh := range combinations {
		// Calculate occupancy probability
		// For each hour-of-week slot, calculate:
		// - How many times was the zone occupied during this hour?
		// - What's the average dwell time?

		row := t.db.QueryRow(`
			SELECT
				COUNT(*) as entries,
				AVG(duration_minutes) as mean_dwell,
				COALESCE(
					SQRT(AVG(duration_minutes * duration_minutes) - AVG(duration_minutes) * AVG(duration_minutes)),
					0
				) as stddev_dwell
			FROM zone_occupancy_history
			WHERE zone_id = ? AND
			      (CAST(strftime('%w', datetime(enter_time/1000000000, 'unixepoch', 'localtime')) AS INTEGER) * 24 +
			       CAST(strftime('%H', datetime(enter_time/1000000000, 'unixepoch', 'localtime')) AS INTEGER)) = ?
			AND exit_time IS NOT NULL
		`, zh.zoneID, zh.hourOfWeek)

		var entries int
		var meanDwell, stddevDwell float64
		if err := row.Scan(&entries, &meanDwell, &stddevDwell); err != nil || entries == 0 {
			continue
		}

		// Estimate occupancy probability based on frequency
		// This is a simplification - in reality we'd need to track total observation time
		occupancyProb := float64(entries) / 7.0 // Normalized by days
		if occupancyProb > 1.0 {
			occupancyProb = 1.0
		}

		patterns = append(patterns, ZoneOccupancyPattern{
			ZoneID:           zh.zoneID,
			HourOfWeek:       zh.hourOfWeek,
			OccupancyProb:    occupancyProb,
			MeanDwellMinutes: meanDwell,
			StddevDwell:      stddevDwell,
			SampleCount:      entries,
			LastComputed:     now,
		})
	}

	// Save patterns
	if len(patterns) > 0 {
		tx, err := t.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()

		for _, p := range patterns {
			_, err := tx.Exec(`
				INSERT OR REPLACE INTO zone_occupancy_patterns
				(zone_id, hour_of_week, occupancy_prob, mean_dwell_minutes, stddev_dwell, sample_count, last_computed)
				VALUES (?, ?, ?, ?, ?, ?, ?)
			`, p.ZoneID, p.HourOfWeek, p.OccupancyProb, p.MeanDwellMinutes, p.StddevDwell, p.SampleCount, p.LastComputed.UnixNano())
			if err != nil {
				log.Printf("[WARN] prediction: failed to save pattern: %v", err)
			}
		}

		if err := tx.Commit(); err != nil {
			return err
		}
	}

	log.Printf("[INFO] prediction: computed %d zone occupancy patterns", len(patterns))
	return nil
}

// GetZoneOccupancyPattern returns the occupancy pattern for a zone at a specific hour.
func (t *AccuracyTracker) GetZoneOccupancyPattern(zoneID string, hourOfWeek int) (*ZoneOccupancyPattern, error) {
	row := t.db.QueryRow(`
		SELECT zone_id, hour_of_week, occupancy_prob, mean_dwell_minutes, stddev_dwell, sample_count, last_computed
		FROM zone_occupancy_patterns
		WHERE zone_id = ? AND hour_of_week = ?
	`, zoneID, hourOfWeek)

	var p ZoneOccupancyPattern
	var lastComputed int64
	err := row.Scan(&p.ZoneID, &p.HourOfWeek, &p.OccupancyProb, &p.MeanDwellMinutes, &p.StddevDwell, &p.SampleCount, &lastComputed)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.LastComputed = time.Unix(0, lastComputed)
	return &p, nil
}

// PredictZoneOccupancy predicts if a zone will be occupied at the target time.
func (t *AccuracyTracker) PredictZoneOccupancy(zoneID string, targetTime time.Time) (float64, error) {
	hourOfWeek := HourOfWeek(targetTime)

	pattern, err := t.GetZoneOccupancyPattern(zoneID, hourOfWeek)
	if err != nil {
		return 0, err
	}
	if pattern == nil {
		return 0, nil // No data
	}

	return pattern.OccupancyProb, nil
}

// GetPendingCount returns the number of pending predictions.
func (t *AccuracyTracker) GetPendingCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.pendingPredictions)
}

// CleanupOldPredictions removes predictions older than the accuracy window.
func (t *AccuracyTracker) CleanupOldPredictions() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	cutoff := time.Now().Add(-AccuracyWindow - 24*time.Hour) // Extra buffer

	result, err := t.db.Exec(`
		DELETE FROM recorded_predictions
		WHERE predicted_at < ? AND evaluated = 1
	`, cutoff.UnixNano())

	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows > 0 {
		log.Printf("[INFO] prediction: cleaned up %d old predictions", rows)
	}

	return nil
}

// SetHorizon sets the prediction horizon.
func (t *AccuracyTracker) SetHorizon(h time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.horizon = h
}

// GetHorizon returns the current prediction horizon.
func (t *AccuracyTracker) GetHorizon() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.horizon
}
