// Package localization provides ground truth sample storage for self-improving localization
package localization

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Collection gates and grid configuration
const (
	MinBLEConfidence    = 0.7  // Minimum BLE triangulation confidence
	MaxBLEBlobDistance  = 0.5  // Maximum BLE-blob distance (metres)
	ZoneGridCellSize    = 0.5  // Zone grid cell size (metres)
)

// GroundTruthSample represents a single ground truth observation
// Collected when BOTH:
// 1. BLE triangulated confidence > 0.7
// 2. CSI blob position within 0.5m of BLE position
type GroundTruthSample struct {
	ID            int64              `json:"id"`
	Timestamp     time.Time          `json:"timestamp"`
	PersonID      string             `json:"person_id"`
	BLEPosition   Vec3               `json:"ble_position"`   // Ground truth from BLE
	BlobPosition  Vec3               `json:"blob_position"`  // CSI fusion estimate
	PositionError float64            `json:"position_error"` // Distance between BLE and blob
	PerLinkDeltas map[string]float64 `json:"per_link_deltas"` // linkID -> deltaRMS
	PerLinkHealth map[string]float64 `json:"per_link_health"` // linkID -> health score
	BLEConfidence float64            `json:"ble_confidence"`
	ZoneGridX     int                `json:"zone_grid_x"` // floor(x / 0.5)
	ZoneGridY     int                `json:"zone_grid_y"` // floor(y / 0.5)
}

// Vec3 represents a 3D position
type Vec3 struct {
	X, Y, Z float64 `json:"x,y,z"`
}

// GroundTruthStore persists ground truth samples to SQLite
type GroundTruthStore struct {
	mu           sync.RWMutex
	db           *sql.DB
	path         string
	maxPerPerson int // Cap per person (default 10,000)
}

// GroundTruthStoreConfig holds configuration
type GroundTruthStoreConfig struct {
	MaxSamplesPerPerson int
}

// DefaultGroundTruthStoreConfig returns defaults
func DefaultGroundTruthStoreConfig() GroundTruthStoreConfig {
	return GroundTruthStoreConfig{
		MaxSamplesPerPerson: 10000,
	}
}

// NewGroundTruthStore creates a new ground truth sample store
func NewGroundTruthStore(dbPath string, config GroundTruthStoreConfig) (*GroundTruthStore, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)

	store := &GroundTruthStore{
		db:           db,
		path:         dbPath,
		maxPerPerson: config.MaxSamplesPerPerson,
	}

	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

// initSchema creates the database schema
func (s *GroundTruthStore) initSchema() error {
	schema := `
	-- Ground truth samples for weight learning
	CREATE TABLE IF NOT EXISTS ground_truth_samples (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp INTEGER NOT NULL,
		person_id TEXT NOT NULL,
		ble_x REAL NOT NULL,
		ble_y REAL NOT NULL,
		ble_z REAL NOT NULL,
		blob_x REAL NOT NULL,
		blob_y REAL NOT NULL,
		blob_z REAL NOT NULL,
		position_error REAL NOT NULL,
		per_link_deltas_json TEXT,
		per_link_health_json TEXT,
		ble_confidence REAL NOT NULL,
		zone_grid_x INTEGER NOT NULL,
		zone_grid_y INTEGER NOT NULL
	);

	-- Indexes for common queries
	CREATE INDEX IF NOT EXISTS idx_gt_samples_person ON ground_truth_samples(person_id);
	CREATE INDEX IF NOT EXISTS idx_gt_samples_time ON ground_truth_samples(timestamp);
	CREATE INDEX IF NOT EXISTS idx_gt_samples_zone ON ground_truth_samples(zone_grid_x, zone_grid_y);
	CREATE INDEX IF NOT EXISTS idx_gt_samples_person_time ON ground_truth_samples(person_id, timestamp);

	-- Weekly position accuracy rollups
	CREATE TABLE IF NOT EXISTS position_accuracy (
		week TEXT NOT NULL,
		person_id TEXT NOT NULL,
		median_error REAL NOT NULL,
		mean_error REAL NOT NULL,
		sample_count INTEGER NOT NULL,
		min_error REAL NOT NULL,
		max_error REAL NOT NULL,
		p05_error REAL NOT NULL,
		p95_error REAL NOT NULL,
		computed_at INTEGER NOT NULL,
		PRIMARY KEY (week, person_id)
	);

	CREATE INDEX IF NOT EXISTS idx_position_accuracy_week ON position_accuracy(week);

	-- System-wide weekly accuracy
	CREATE TABLE IF NOT EXISTS system_position_accuracy (
		week TEXT PRIMARY KEY,
		median_error REAL NOT NULL,
		mean_error REAL NOT NULL,
		sample_count INTEGER NOT NULL,
		person_count INTEGER NOT NULL,
		computed_at INTEGER NOT NULL
	);
	`

	_, err := s.db.Exec(schema)
	return err
}

// ShouldCollectSample determines if a sample should be collected
func ShouldCollectSample(bleConfidence, bleBlobDistance float64) bool {
	return bleConfidence >= MinBLEConfidence && bleBlobDistance <= MaxBLEBlobDistance
}

// ComputeZoneGrid computes the zone grid coordinates for a position
func ComputeZoneGrid(x, z float64) (gridX, gridY int) {
	gridX = int(math.Floor(x / ZoneGridCellSize))
	gridY = int(math.Floor(z / ZoneGridCellSize))
	return
}

// ComputePositionError calculates the distance between BLE and blob positions
func ComputePositionError(ble, blob Vec3) float64 {
	dx := ble.X - blob.X
	dy := ble.Y - blob.Y
	dz := ble.Z - blob.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// AddSample stores a new ground truth sample
func (s *GroundTruthStore) AddSample(sample GroundTruthSample) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Serialize maps
	deltasJSON, err := json.Marshal(sample.PerLinkDeltas)
	if err != nil {
		return fmt.Errorf("marshal deltas: %w", err)
	}
	healthJSON, err := json.Marshal(sample.PerLinkHealth)
	if err != nil {
		return fmt.Errorf("marshal health: %w", err)
	}

	// Insert sample
	result, err := s.db.Exec(`
		INSERT INTO ground_truth_samples
			(timestamp, person_id, ble_x, ble_y, ble_z, blob_x, blob_y, blob_z,
			 position_error, per_link_deltas_json, per_link_health_json,
			 ble_confidence, zone_grid_x, zone_grid_y)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, sample.Timestamp.Unix(), sample.PersonID,
		sample.BLEPosition.X, sample.BLEPosition.Y, sample.BLEPosition.Z,
		sample.BlobPosition.X, sample.BlobPosition.Y, sample.BlobPosition.Z,
		sample.PositionError, string(deltasJSON), string(healthJSON),
		sample.BLEConfidence, sample.ZoneGridX, sample.ZoneGridY)
	if err != nil {
		return err
	}

	// Get the inserted ID
	if id, err := result.LastInsertId(); err == nil {
		sample.ID = id
	}

	// Enforce per-person cap asynchronously
	go s.enforceCap(sample.PersonID)

	return nil
}

// enforceCap removes oldest samples when over the limit
func (s *GroundTruthStore) enforceCap(personID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Count samples for this person
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM ground_truth_samples WHERE person_id = ?`,
		personID,
	).Scan(&count)
	if err != nil {
		return
	}

	// If over cap, delete oldest
	if count > s.maxPerPerson {
		excess := count - s.maxPerPerson
		_, err = s.db.Exec(`
			DELETE FROM ground_truth_samples
			WHERE id IN (
				SELECT id FROM ground_truth_samples
				WHERE person_id = ?
				ORDER BY timestamp ASC
				LIMIT ?
			)
		`, personID, excess)
		if err != nil {
			log.Printf("[WARN] Failed to enforce sample cap for %s: %v", personID, err)
		} else {
			log.Printf("[DEBUG] Removed %d oldest samples for %s (cap: %d)", excess, personID, s.maxPerPerson)
		}
	}
}

// GetSamplesForZone retrieves samples for a specific zone grid cell
func (s *GroundTruthStore) GetSamplesForZone(gridX, gridY int, limit int) ([]GroundTruthSample, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT id, timestamp, person_id, ble_x, ble_y, ble_z,
		       blob_x, blob_y, blob_z, position_error,
		       per_link_deltas_json, per_link_health_json,
		       ble_confidence, zone_grid_x, zone_grid_y
		FROM ground_truth_samples
		WHERE zone_grid_x = ? AND zone_grid_y = ?
		ORDER BY timestamp DESC
	`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.Query(query, gridX, gridY)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanSamples(rows)
}

// GetRecentSamples retrieves recent samples across all zones
func (s *GroundTruthStore) GetRecentSamples(limit int) ([]GroundTruthSample, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT id, timestamp, person_id, ble_x, ble_y, ble_z,
		       blob_x, blob_y, blob_z, position_error,
		       per_link_deltas_json, per_link_health_json,
		       ble_confidence, zone_grid_x, zone_grid_y
		FROM ground_truth_samples
		ORDER BY timestamp DESC
		LIMIT ?
	`

	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanSamples(rows)
}

// GetSamplesInTimeRange retrieves samples within a time range
func (s *GroundTruthStore) GetSamplesInTimeRange(start, end time.Time) ([]GroundTruthSample, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT id, timestamp, person_id, ble_x, ble_y, ble_z,
		       blob_x, blob_y, blob_z, position_error,
		       per_link_deltas_json, per_link_health_json,
		       ble_confidence, zone_grid_x, zone_grid_y
		FROM ground_truth_samples
		WHERE timestamp >= ? AND timestamp < ?
		ORDER BY timestamp ASC
	`

	rows, err := s.db.Query(query, start.Unix(), end.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanSamples(rows)
}

// scanSamples scans rows into GroundTruthSample slices
func (s *GroundTruthStore) scanSamples(rows *sql.Rows) ([]GroundTruthSample, error) {
	var samples []GroundTruthSample

	for rows.Next() {
		var sample GroundTruthSample
		var timestamp int64
		var deltasJSON, healthJSON string

		err := rows.Scan(
			&sample.ID, &timestamp, &sample.PersonID,
			&sample.BLEPosition.X, &sample.BLEPosition.Y, &sample.BLEPosition.Z,
			&sample.BlobPosition.X, &sample.BlobPosition.Y, &sample.BlobPosition.Z,
			&sample.PositionError, &deltasJSON, &healthJSON,
			&sample.BLEConfidence, &sample.ZoneGridX, &sample.ZoneGridY,
		)
		if err != nil {
			continue
		}

		sample.Timestamp = time.Unix(timestamp, 0)

		if err := json.Unmarshal([]byte(deltasJSON), &sample.PerLinkDeltas); err != nil {
			sample.PerLinkDeltas = make(map[string]float64)
		}
		if err := json.Unmarshal([]byte(healthJSON), &sample.PerLinkHealth); err != nil {
			sample.PerLinkHealth = make(map[string]float64)
		}

		samples = append(samples, sample)
	}

	return samples, rows.Err()
}

// GetZoneSampleCounts returns sample counts per zone
func (s *GroundTruthStore) GetZoneSampleCounts() (map[[2]int]int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT zone_grid_x, zone_grid_y, COUNT(*)
		FROM ground_truth_samples
		GROUP BY zone_grid_x, zone_grid_y
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[[2]int]int)
	for rows.Next() {
		var x, y, count int
		if err := rows.Scan(&x, &y, &count); err != nil {
			continue
		}
		counts[[2]int{x, y}] = count
	}

	return counts, rows.Err()
}

// GetTotalSampleCount returns total number of samples
func (s *GroundTruthStore) GetTotalSampleCount() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM ground_truth_samples`).Scan(&count)
	return count, err
}

// GetSampleCountByPerson returns sample counts per person
func (s *GroundTruthStore) GetSampleCountByPerson() (map[string]int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT person_id, COUNT(*)
		FROM ground_truth_samples
		GROUP BY person_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var personID string
		var count int
		if err := rows.Scan(&personID, &count); err != nil {
			continue
		}
		counts[personID] = count
	}

	return counts, rows.Err()
}

// ComputeWeeklyAccuracy computes position accuracy for a week
func (s *GroundTruthStore) ComputeWeeklyAccuracy(week string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get week start/end
	weekStart, err := parseWeekString(week)
	if err != nil {
		return err
	}
	weekEnd := weekStart.Add(7 * 24 * time.Hour)
	startUnix := weekStart.Unix()
	endUnix := weekEnd.Unix()
	now := time.Now().Unix()

	// Get all samples for the week
	rows, err := s.db.Query(`
		SELECT person_id, position_error
		FROM ground_truth_samples
		WHERE timestamp >= ? AND timestamp < ?
		ORDER BY person_id, position_error
	`, startUnix, endUnix)
	if err != nil {
		return fmt.Errorf("query samples: %w", err)
	}
	defer rows.Close()

	// Group by person
	personErrors := make(map[string][]float64)
	for rows.Next() {
		var personID string
		var errVal float64
		if err := rows.Scan(&personID, &errVal); err != nil {
			continue
		}
		personErrors[personID] = append(personErrors[personID], errVal)
	}

	// Compute per-person stats
	for personID, errors := range personErrors {
		if len(errors) == 0 {
			continue
		}

		// Sort for percentiles
		sorted := make([]float64, len(errors))
		copy(sorted, errors)
		sortFloat64(sorted)

		median := percentile(sorted, 50)
		mean := meanFloat64(sorted)
		min := sorted[0]
		max := sorted[len(sorted)-1]
		p05 := percentile(sorted, 5)
		p95 := percentile(sorted, 95)

		_, err := s.db.Exec(`
			INSERT OR REPLACE INTO position_accuracy
				(week, person_id, median_error, mean_error, sample_count,
				 min_error, max_error, p05_error, p95_error, computed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, week, personID, median, mean, len(errors), min, max, p05, p95, now)
		if err != nil {
			log.Printf("[WARN] Failed to save accuracy for %s: %v", personID, err)
		}
	}

	// Compute system-wide stats
	var allErrors []float64
	for _, errors := range personErrors {
		allErrors = append(allErrors, errors...)
	}

	if len(allErrors) > 0 {
		sorted := make([]float64, len(allErrors))
		copy(sorted, allErrors)
		sortFloat64(sorted)

		_, err := s.db.Exec(`
			INSERT OR REPLACE INTO system_position_accuracy
				(week, median_error, mean_error, sample_count, person_count, computed_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, week, percentile(sorted, 50), meanFloat64(sorted), len(allErrors), len(personErrors), now)
		if err != nil {
			return fmt.Errorf("save system accuracy: %w", err)
		}
	}

	log.Printf("[INFO] Computed position accuracy for %s (%d samples, %d people)",
		week, len(allErrors), len(personErrors))
	return nil
}

// PositionAccuracyRecord represents weekly position accuracy
type PositionAccuracyRecord struct {
	Week        string    `json:"week"`
	PersonID    string    `json:"person_id,omitempty"`
	MedianError float64   `json:"median_error"`
	MeanError   float64   `json:"mean_error"`
	SampleCount int       `json:"sample_count"`
	PersonCount int       `json:"person_count,omitempty"`
	MinError    float64   `json:"min_error,omitempty"`
	MaxError    float64   `json:"max_error,omitempty"`
	P05Error    float64   `json:"p05_error,omitempty"`
	P95Error    float64   `json:"p95_error,omitempty"`
	ComputedAt  time.Time `json:"computed_at"`
}

// GetPositionAccuracyHistory returns system position accuracy history
func (s *GroundTruthStore) GetPositionAccuracyHistory(weeks int) ([]PositionAccuracyRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT week, median_error, mean_error, sample_count, person_count, computed_at
		FROM system_position_accuracy
		ORDER BY week DESC
		LIMIT ?
	`, weeks)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []PositionAccuracyRecord
	for rows.Next() {
		var r PositionAccuracyRecord
		var computedAt int64
		err := rows.Scan(&r.Week, &r.MedianError, &r.MeanError, &r.SampleCount, &r.PersonCount, &computedAt)
		if err != nil {
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

// GetCurrentPositionAccuracy returns current week's position accuracy
func (s *GroundTruthStore) GetCurrentPositionAccuracy() (*PositionAccuracyRecord, error) {
	records, err := s.GetPositionAccuracyHistory(1)
	if err != nil || len(records) == 0 {
		return nil, err
	}
	return &records[0], nil
}

// GetPositionImprovementStats calculates position accuracy improvement
func (s *GroundTruthStore) GetPositionImprovementStats() (map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	currentWeek := GetWeekString(time.Now())
	lastWeek := GetWeekString(time.Now().AddDate(0, 0, -7))

	// Get current week stats
	var currentMedian, currentMean float64
	var currentSamples, currentPersons int
	var currentComputed int64
	currentErr := s.db.QueryRow(`
		SELECT median_error, mean_error, sample_count, person_count, computed_at
		FROM system_position_accuracy WHERE week = ?
	`, currentWeek).Scan(&currentMedian, &currentMean, &currentSamples, &currentPersons, &currentComputed)

	// Get last week stats
	var lastMedian, lastMean float64
	var lastSamples, lastPersons int
	var lastComputed int64
	lastErr := s.db.QueryRow(`
		SELECT median_error, mean_error, sample_count, person_count, computed_at
		FROM system_position_accuracy WHERE week = ?
	`, lastWeek).Scan(&lastMedian, &lastMean, &lastSamples, &lastPersons, &lastComputed)

	// Calculate improvement (negative = improvement, lower error is better)
	improvement := 0.0
	trend := "stable"
	if currentErr == nil && lastErr == nil && lastMedian > 0 {
		improvement = ((lastMedian - currentMedian) / lastMedian) * 100
		if improvement > 10 {
			trend = "improving"
		} else if improvement < -10 {
			trend = "degrading"
		}
	}

	// Get total sample count
	var totalSamples int
	s.db.QueryRow(`SELECT COUNT(*) FROM ground_truth_samples`).Scan(&totalSamples)

	// Get today's samples
	var todaySamples int
	todayStart := time.Now().Truncate(24 * time.Hour)
	s.db.QueryRow(`SELECT COUNT(*) FROM ground_truth_samples WHERE timestamp >= ?`, todayStart.Unix()).Scan(&todaySamples)

	result := map[string]interface{}{
		"current_week":       currentWeek,
		"improvement_pct":    improvement,
		"trend":              trend,
		"total_samples":      totalSamples,
		"today_samples":      todaySamples,
	}

	if currentErr == nil {
		result["current_median_m"] = currentMedian
		result["current_mean_m"] = currentMean
		result["current_samples"] = currentSamples
		result["current_persons"] = currentPersons
	}

	if lastErr == nil {
		result["last_week_median_m"] = lastMedian
		result["last_week_samples"] = lastSamples
	}

	return result, nil
}

// GetSamplesTodayCount returns the count of samples collected today
func (s *GroundTruthStore) GetSamplesTodayCount() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	todayStart := time.Now().Truncate(24 * time.Hour)
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM ground_truth_samples WHERE timestamp >= ?`, todayStart.Unix()).Scan(&count)
	return count, err
}

// Close closes the database connection
func (s *GroundTruthStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Close()
}

// Helper functions

// GetWeekString returns the ISO week string for a time
func GetWeekString(t time.Time) string {
	year, week := t.ISOWeek()
	return fmt.Sprintf("%d-W%02d", year, week)
}

// parseWeekString parses a week string (e.g., "2026-W13") into the Monday of that week
func parseWeekString(week string) (time.Time, error) {
	var year, weekNum int
	_, err := fmt.Sscanf(week, "%d-W%d", &year, &weekNum)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid week format: %w", err)
	}

	// Get the first day of the year
	t := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)

	// Find the first Monday
	for t.Weekday() != time.Monday {
		t = t.AddDate(0, 0, 1)
	}

	// Add the week offset (ISO weeks)
	t = t.AddDate(0, 0, (weekNum-1)*7)

	return t, nil
}

// sortFloat64 sorts a slice of float64
func sortFloat64(s []float64) {
	for i := 0; i < len(s)-1; i++ {
		for j := i + 1; j < len(s); j++ {
			if s[i] > s[j] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}

// meanFloat64 computes the mean of a slice
func meanFloat64(s []float64) float64 {
	if len(s) == 0 {
		return 0
	}
	var sum float64
	for _, v := range s {
		sum += v
	}
	return sum / float64(len(s))
}

// percentile computes the percentile of a sorted slice
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}

	idx := (p / 100.0) * float64(len(sorted)-1)
	lower := int(math.Floor(idx))
	upper := int(math.Ceil(idx))

	if lower == upper || upper >= len(sorted) {
		return sorted[lower]
	}

	frac := idx - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}
