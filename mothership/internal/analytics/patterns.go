package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math"
	"sync"
	"time"
)

const (
	// PatternReadySamples is the minimum sample_count before a slot is considered "ready".
	PatternReadySamples = 50

	// ColdStartDays is the number of days before any anomaly alerts fire.
	ColdStartDays = 7

	// AlertThresholdYellow is the yellow warning threshold.
	AlertThresholdYellow = 0.60

	// AlertThresholdRed is the red alert threshold.
	AlertThresholdRed = 0.85

	// OutlierProtectionThreshold — skip model update if anomaly_score >= this.
	OutlierProtectionThreshold = 0.50

	// Epsilon prevents division by zero in z-score computation.
	Epsilon = 1e-9
)

// PatternSlot represents a single (zone_id, hour_of_day, day_of_week) statistical slot.
type PatternSlot struct {
	ZoneID      int     `json:"zone_id"`
	HourOfDay   int     `json:"hour_of_day"`  // 0-23
	DayOfWeek   int     `json:"day_of_week"`  // 0-6 (0=Sunday)
	MeanCount   float64 `json:"mean_count"`
	Variance    float64 `json:"variance"`
	SampleCount int     `json:"sample_count"`
	UpdatedAt   int64   `json:"updated_at"` // Unix ms
}

// patternKey is the composite key for pattern slots.
type patternKey struct {
	zoneID    int
	hourOfDay int
	dayOfWeek int
}

// OccupancyProvider provides current zone occupancy counts.
type OccupancyProvider interface {
	GetZoneOccupancyCounts() map[int]int // zone_id -> blob count
}

// PatternLearner learns occupancy patterns using Welford's online algorithm.
// It persists to the anomaly_patterns table in the main database.
type PatternLearner struct {
	mu           sync.RWMutex
	db           *sql.DB
	startTime    time.Time
	securityMode bool

	// In-memory cache of loaded patterns
	patterns map[patternKey]*PatternSlot
}

// NewPatternLearner creates a new pattern learner using the main database.
func NewPatternLearner(db *sql.DB) (*PatternLearner, error) {
	// Ensure required tables exist
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value_json TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS anomaly_patterns (
			zone_id     INTEGER NOT NULL,
			hour_of_day INTEGER NOT NULL CHECK (hour_of_day BETWEEN 0 AND 23),
			day_of_week INTEGER NOT NULL CHECK (day_of_week BETWEEN 0 AND 6),
			mean_count  REAL NOT NULL DEFAULT 0,
			variance    REAL NOT NULL DEFAULT 0,
			sample_count INTEGER NOT NULL DEFAULT 0,
			updated_at  INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (zone_id, hour_of_day, day_of_week)
		);
	`)
	if err != nil {
		return nil, fmt.Errorf("create pattern tables: %w", err)
	}

	pl := &PatternLearner{
		db:       db,
		patterns: make(map[patternKey]*PatternSlot),
	}

	// Try to load learning start time from settings
	var startMs int64
	err = db.QueryRow(`SELECT value_json FROM settings WHERE key = 'pattern_learning_start_ms'`).Scan(&startMs)
	if err == sql.ErrNoRows {
		pl.startTime = time.Now()
		db.Exec(`INSERT INTO settings (key, value_json) VALUES ('pattern_learning_start_ms', ?)`,
			time.Now().UnixMilli())
	} else if err == nil {
		pl.startTime = time.UnixMilli(startMs)
	}

	// Load existing patterns from anomaly_patterns table
	if err := pl.loadPatterns(); err != nil {
		log.Printf("[WARN] Failed to load anomaly patterns: %v", err)
	}

	return pl, nil
}

func (pl *PatternLearner) loadPatterns() error {
	rows, err := pl.db.Query(`
		SELECT zone_id, hour_of_day, day_of_week, mean_count, variance, sample_count, updated_at
		FROM anomaly_patterns
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		slot := &PatternSlot{}
		if err := rows.Scan(&slot.ZoneID, &slot.HourOfDay, &slot.DayOfWeek,
			&slot.MeanCount, &slot.Variance, &slot.SampleCount, &slot.UpdatedAt); err != nil {
			continue
		}
		key := patternKey{slot.ZoneID, slot.HourOfDay, slot.DayOfWeek}
		pl.patterns[key] = slot
	}
	return rows.Err()
}

// IsColdStart returns true if the system is within the 7-day cold start period.
func (pl *PatternLearner) IsColdStart() bool {
	pl.mu.RLock()
	defer pl.mu.RUnlock()
	return time.Since(pl.startTime) < ColdStartDays*24*time.Hour
}

// IsSlotReady returns true if a specific pattern slot has enough samples.
func (pl *PatternLearner) IsSlotReady(zoneID, hourOfDay, dayOfWeek int) bool {
	pl.mu.RLock()
	defer pl.mu.RUnlock()

	key := patternKey{zoneID, hourOfDay, dayOfWeek}
	slot, exists := pl.patterns[key]
	return exists && slot.SampleCount >= PatternReadySamples
}

// GetPattern returns a pattern slot for inspection (returns a copy).
func (pl *PatternLearner) GetPattern(zoneID, hourOfDay, dayOfWeek int) *PatternSlot {
	pl.mu.RLock()
	defer pl.mu.RUnlock()

	key := patternKey{zoneID, hourOfDay, dayOfWeek}
	slot, exists := pl.patterns[key]
	if !exists {
		return nil
	}
	cp := *slot
	return &cp
}

// GetPatterns returns all patterns, optionally filtered by zone.
func (pl *PatternLearner) GetPatterns(zoneID int) []*PatternSlot {
	pl.mu.RLock()
	defer pl.mu.RUnlock()

	var result []*PatternSlot
	for key, slot := range pl.patterns {
		if zoneID > 0 && key.zoneID != zoneID {
			continue
		}
		cp := *slot
		result = append(result, &cp)
	}
	return result
}

// WelfordUpdate applies one step of Welford's online algorithm.
// Given current mean, M2 accumulator, count, and a new observation,
// it returns the updated mean, M2, and count.
//
// Variance is recovered as M2 / n (population variance).
// This is numerically stable even for large counts.
func WelfordUpdate(mean, m2, count, newValue float64) (newMean, newM2, newCount float64) {
	newCount = count + 1
	delta := newValue - mean
	newMean = mean + delta/newCount
	delta2 := newValue - newMean
	newM2 = m2 + delta*delta2
	return
}

// ObserveAndUpdate records an observation and updates the model using Welford's algorithm.
// anomalyScore is the current anomaly score for this observation (0 if not yet computed).
// If anomalyScore >= OutlierProtectionThreshold, the model update is skipped (outlier protection).
func (pl *PatternLearner) ObserveAndUpdate(zoneID, hourOfDay, dayOfWeek int, observedCount int, anomalyScore float64) error {
	// Outlier protection: don't learn from anomalies
	if anomalyScore >= OutlierProtectionThreshold {
		return nil
	}

	pl.mu.Lock()
	defer pl.mu.Unlock()

	key := patternKey{zoneID, hourOfDay, dayOfWeek}
	slot, exists := pl.patterns[key]

	var mean, m2, count float64
	if exists {
		mean = slot.MeanCount
		count = float64(slot.SampleCount)
		// Recover M2 from stored variance: M2 = variance * n
		m2 = slot.Variance * count
	}

	newMean, newM2, newCount := WelfordUpdate(mean, m2, count, float64(observedCount))

	// Population variance
	variance := 0.0
	if newCount > 0 {
		variance = newM2 / newCount
	}

	nowMs := time.Now().UnixMilli()

	_, err := pl.db.Exec(`
		INSERT INTO anomaly_patterns (zone_id, hour_of_day, day_of_week, mean_count, variance, sample_count, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(zone_id, hour_of_day, day_of_week) DO UPDATE SET
			mean_count = excluded.mean_count,
			variance = excluded.variance,
			sample_count = excluded.sample_count,
			updated_at = excluded.updated_at
	`, zoneID, hourOfDay, dayOfWeek, newMean, variance, int(newCount), nowMs)
	if err != nil {
		return err
	}

	pl.patterns[key] = &PatternSlot{
		ZoneID:      zoneID,
		HourOfDay:   hourOfDay,
		DayOfWeek:   dayOfWeek,
		MeanCount:   newMean,
		Variance:    variance,
		SampleCount: int(newCount),
		UpdatedAt:   nowMs,
	}

	return nil
}

// AnomalyResult holds the result of an anomaly score computation.
type AnomalyResult struct {
	CompositeScore float64 `json:"composite_score"`
	TimeScore      float64 `json:"time_score"`
	ZoneScore      float64 `json:"zone_score"`
	IsAlert        bool    `json:"is_alert"`
	IsWarning      bool    `json:"is_warning"`
	Suppressed     bool    `json:"suppressed"` // true if cold start or slot not ready
}

// ComputeAnomalyScore computes the anomaly score for an observation.
func (pl *PatternLearner) ComputeAnomalyScore(zoneID, hourOfDay, dayOfWeek int, observedCount int) AnomalyResult {
	pl.mu.RLock()
	defer pl.mu.RUnlock()

	result := AnomalyResult{}

	// Security mode override: any detection = score 1.0
	if pl.securityMode {
		result.CompositeScore = 1.0
		result.TimeScore = 1.0
		result.ZoneScore = 1.0
		result.IsAlert = true
		return result
	}

	// Cold start: suppress all anomaly alerts
	if time.Since(pl.startTime) < ColdStartDays*24*time.Hour {
		result.Suppressed = true
		return result
	}

	key := patternKey{zoneID, hourOfDay, dayOfWeek}
	slot, exists := pl.patterns[key]

	// Slot not ready
	if !exists || slot.SampleCount < PatternReadySamples {
		result.Suppressed = true
		return result
	}

	// Z-score: (observed - mean) / sqrt(variance + epsilon)
	stdDev := math.Sqrt(slot.Variance + Epsilon)
	zScore := (float64(observedCount) - slot.MeanCount) / stdDev

	// Normalize z-score to [0, 1]: 0 below 1σ, linear to 1.0 at 4σ
	result.TimeScore = normalizeZScore(zScore)

	// Zone score: 1.0 if zone normally empty at this time but now occupied
	if slot.MeanCount < 0.1 && observedCount > 0 {
		result.ZoneScore = 1.0
	}

	// Composite score: max of time_score and zone_score
	result.CompositeScore = math.Max(result.TimeScore, result.ZoneScore)

	// Threshold checks
	result.IsAlert = result.CompositeScore >= AlertThresholdRed
	result.IsWarning = result.CompositeScore >= AlertThresholdYellow && !result.IsAlert

	return result
}

// computeScoreLocked computes anomaly score while holding the write lock.
// Used internally by updateAllZones to avoid lock ordering issues.
func (pl *PatternLearner) computeScoreLocked(zoneID, hourOfDay, dayOfWeek int, observedCount int) float64 {
	if pl.securityMode {
		return 1.0
	}
	if time.Since(pl.startTime) < ColdStartDays*24*time.Hour {
		return 0.0
	}

	key := patternKey{zoneID, hourOfDay, dayOfWeek}
	slot, exists := pl.patterns[key]
	if !exists || slot.SampleCount < PatternReadySamples {
		return 0.0
	}

	stdDev := math.Sqrt(slot.Variance + Epsilon)
	zScore := (float64(observedCount) - slot.MeanCount) / stdDev
	timeScore := normalizeZScore(zScore)

	zoneScore := 0.0
	if slot.MeanCount < 0.1 && observedCount > 0 {
		zoneScore = 1.0
	}

	return math.Max(timeScore, zoneScore)
}

// normalizeZScore maps |z| to [0, 1]: 0 below 1σ, linear to 1.0 at 4σ.
func normalizeZScore(z float64) float64 {
	absZ := math.Abs(z)
	if absZ < 1.0 {
		return 0.0
	}
	normalized := (absZ - 1.0) / 3.0
	if normalized > 1.0 {
		normalized = 1.0
	}
	return normalized
}

// SetSecurityMode sets the security mode flag.
func (pl *PatternLearner) SetSecurityMode(enabled bool) {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	pl.securityMode = enabled
}

// IsSecurityMode returns whether security mode is active.
func (pl *PatternLearner) IsSecurityMode() bool {
	pl.mu.RLock()
	defer pl.mu.RUnlock()
	return pl.securityMode
}

// SetLearningStartTime sets the learning start time. Used for testing.
func (pl *PatternLearner) SetLearningStartTime(t time.Time) {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	pl.startTime = t
}

// RunHourlyUpdate starts a goroutine that runs pattern updates every hour,
// aligned to the top of each hour. It observes zone occupancy from the provider
// and updates the model for all zones.
func (pl *PatternLearner) RunHourlyUpdate(ctx context.Context, provider OccupancyProvider) {
	go func() {
		// Align to the start of the next hour
		now := time.Now()
		nextHour := now.Truncate(time.Hour).Add(time.Hour)
		initialTimer := time.NewTimer(nextHour.Sub(now))

		select {
		case <-ctx.Done():
			initialTimer.Stop()
			return
		case <-initialTimer.C:
		}

		pl.updateAllZones(provider)

		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pl.updateAllZones(provider)
			}
		}
	}()
}

// updateAllZones observes occupancy for all zones and updates the model.
// Holds a single write lock for the entire operation to avoid deadlocks.
func (pl *PatternLearner) updateAllZones(provider OccupancyProvider) {
	if provider == nil {
		return
	}

	counts := provider.GetZoneOccupancyCounts()
	now := time.Now()
	hourOfDay := now.Hour()
	dayOfWeek := int(now.Weekday())

	pl.mu.Lock()
	defer pl.mu.Unlock()

	for zoneID, count := range counts {
		// Compute anomaly score under the write lock
		composite := pl.computeScoreLocked(zoneID, hourOfDay, dayOfWeek, count)

		// Outlier protection
		if composite >= OutlierProtectionThreshold {
			continue
		}

		key := patternKey{zoneID, hourOfDay, dayOfWeek}
		slot, exists := pl.patterns[key]

		var mean, m2, countF float64
		if exists {
			mean = slot.MeanCount
			countF = float64(slot.SampleCount)
			m2 = slot.Variance * countF
		}

		newMean, newM2, newCount := WelfordUpdate(mean, m2, countF, float64(count))

		variance := 0.0
		if newCount > 0 {
			variance = newM2 / newCount
		}

		nowMs := time.Now().UnixMilli()

		_, err := pl.db.Exec(`
			INSERT INTO anomaly_patterns (zone_id, hour_of_day, day_of_week, mean_count, variance, sample_count, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(zone_id, hour_of_day, day_of_week) DO UPDATE SET
				mean_count = excluded.mean_count,
				variance = excluded.variance,
				sample_count = excluded.sample_count,
				updated_at = excluded.updated_at
		`, zoneID, hourOfDay, dayOfWeek, newMean, variance, int(newCount), nowMs)
		if err != nil {
			log.Printf("[WARN] Failed to update pattern for zone %d: %v", zoneID, err)
			continue
		}

		pl.patterns[key] = &PatternSlot{
			ZoneID:      zoneID,
			HourOfDay:   hourOfDay,
			DayOfWeek:   dayOfWeek,
			MeanCount:   newMean,
			Variance:    variance,
			SampleCount: int(newCount),
			UpdatedAt:   nowMs,
		}
	}
}
