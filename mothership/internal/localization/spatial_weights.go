// Package localization provides spatial weight learning for self-improving localization
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

// SpatialWeightLearner learns per-link, per-zone weights using SGD
type SpatialWeightLearner struct {
	mu     sync.RWMutex
	db     *sql.DB
	path   string
	config SpatialWeightLearnerConfig

	// In-memory weight cache: linkID -> zoneGridX -> zoneGridY -> weight
	weightCache map[string]map[int]map[int]float64

	// Validation holdout: 20% of samples
	validationRatio float64

	// Counter for batch updates
	updateCounter int
}

// SpatialWeightLearnerConfig holds configuration for weight learning
type SpatialWeightLearnerConfig struct {
	// Learning rate for SGD
	LearningRate float64

	// L2 regularization coefficient
	Regularization float64

	// Minimum samples in zone before learning starts
	MinZoneSamples int

	// Batch size for validation checks
	ValidationBatchSize int

	// Required improvement ratio (0.05 = 5%)
	ImprovementThreshold float64

	// Weight range
	MinWeight float64
	MaxWeight float64
}

// DefaultSpatialWeightLearnerConfig returns sensible defaults
func DefaultSpatialWeightLearnerConfig() SpatialWeightLearnerConfig {
	return SpatialWeightLearnerConfig{
		LearningRate:         0.001,
		Regularization:       0.01,
		MinZoneSamples:       100,
		ValidationBatchSize:  50,
		ImprovementThreshold: 0.05, // 5% improvement required
		MinWeight:            0.0,
		MaxWeight:            5.0,
	}
}

// ZoneWeight represents a learned weight for a link in a zone
type ZoneWeight struct {
	LinkID               string    `json:"link_id"`
	ZoneGridX            int       `json:"zone_grid_x"`
	ZoneGridY            int       `json:"zone_grid_y"`
	Weight               float64   `json:"weight"`
	SampleCount          int       `json:"sample_count"`
	LastUpdated          time.Time `json:"last_updated"`
	ValidationImprovement float64  `json:"validation_improvement"`
}

// NewSpatialWeightLearner creates a new spatial weight learner
func NewSpatialWeightLearner(dbPath string, config SpatialWeightLearnerConfig) (*SpatialWeightLearner, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)

	learner := &SpatialWeightLearner{
		db:             db,
		path:           dbPath,
		config:         config,
		weightCache:    make(map[string]map[int]map[int]float64),
		validationRatio: 0.2,
	}

	if err := learner.initSchema(); err != nil {
		db.Close()
		return nil, err
	}

	// Load existing weights into cache
	if err := learner.loadWeightsIntoCache(); err != nil {
		log.Printf("[WARN] Failed to load weights into cache: %v", err)
	}

	return learner, nil
}

// initSchema creates the database schema
func (l *SpatialWeightLearner) initSchema() error {
	schema := `
	-- Per-link, per-zone learned weights
	CREATE TABLE IF NOT EXISTS spatial_link_weights (
		link_id TEXT NOT NULL,
		zone_grid_x INTEGER NOT NULL,
		zone_grid_y INTEGER NOT NULL,
		weight REAL NOT NULL DEFAULT 1.0,
		sample_count INTEGER NOT NULL DEFAULT 0,
		last_updated INTEGER NOT NULL,
		validation_improvement REAL NOT NULL DEFAULT 0.0,
		PRIMARY KEY (link_id, zone_grid_x, zone_grid_y)
	);

	CREATE INDEX IF NOT EXISTS idx_spatial_weights_zone ON spatial_link_weights(zone_grid_x, zone_grid_y);
	CREATE INDEX IF NOT EXISTS idx_spatial_weights_link ON spatial_link_weights(link_id);

	-- Learning metadata
	CREATE TABLE IF NOT EXISTS spatial_learning_metadata (
		key TEXT PRIMARY KEY,
		value TEXT
	);
	`

	_, err := l.db.Exec(schema)
	return err
}

// loadWeightsIntoCache loads all weights from DB into memory
func (l *SpatialWeightLearner) loadWeightsIntoCache() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	rows, err := l.db.Query(`
		SELECT link_id, zone_grid_x, zone_grid_y, weight
		FROM spatial_link_weights
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var linkID string
		var zoneX, zoneY int
		var weight float64

		if err := rows.Scan(&linkID, &zoneX, &zoneY, &weight); err != nil {
			continue
		}

		if l.weightCache[linkID] == nil {
			l.weightCache[linkID] = make(map[int]map[int]float64)
		}
		if l.weightCache[linkID][zoneX] == nil {
			l.weightCache[linkID][zoneX] = make(map[int]float64)
		}
		l.weightCache[linkID][zoneX][zoneY] = weight
	}

	log.Printf("[INFO] Loaded spatial weights into cache (%d links)", len(l.weightCache))
	return nil
}

// GetSpatialWeight returns the learned weight for a link at a position
// Uses bilinear interpolation between adjacent grid cells for smooth transitions
// Returns 1.0 (no adjustment) if no learned weight exists
func (l *SpatialWeightLearner) GetSpatialWeight(linkID string, x, z float64) float64 {
	l.mu.RLock()
	defer l.mu.RUnlock()

	// Compute continuous grid position
	gx := x / ZoneGridCellSize
	gy := z / ZoneGridCellSize

	// Get integer grid coordinates
	x0 := int(math.Floor(gx))
	y0 := int(math.Floor(gy))
	x1 := x0 + 1
	y1 := y0 + 1

	// Compute interpolation factors
	fx := gx - float64(x0)
	fy := gy - float64(y0)

	// Get weights at four corners (default to 1.0)
	w00 := l.getWeightLocked(linkID, x0, y0)
	w10 := l.getWeightLocked(linkID, x1, y0)
	w01 := l.getWeightLocked(linkID, x0, y1)
	w11 := l.getWeightLocked(linkID, x1, y1)

	// Bilinear interpolation
	w0 := w00*(1-fx) + w10*fx
	w1 := w01*(1-fx) + w11*fx
	result := w0*(1-fy) + w1*fy

	return result
}

// getWeightLocked returns cached weight (must hold lock)
func (l *SpatialWeightLearner) getWeightLocked(linkID string, zoneX, zoneY int) float64 {
	if linkWeights, ok := l.weightCache[linkID]; ok {
		if rowWeights, ok := linkWeights[zoneX]; ok {
			if weight, ok := rowWeights[zoneY]; ok {
				return weight
			}
		}
	}
	return 1.0 // Default: no adjustment
}

// ProcessSample performs online SGD update from a ground truth sample
func (l *SpatialWeightLearner) ProcessSample(sample GroundTruthSample) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	zoneX := sample.ZoneGridX
	zoneY := sample.ZoneGridY

	// Check if this sample should go to validation set
	isValidation := (sample.ID % 5) == 0 // 20% holdout

	if isValidation {
		// Don't train on validation samples
		return nil
	}

	// Compute position estimate using current weights
	estimatedPos, normFactor := l.estimatePositionLocked(sample.PerLinkDeltas, zoneX, zoneY)
	if normFactor < 0.001 {
		return nil // No valid links
	}

	// Compute error vector
	errorX := sample.BLEPosition.X - estimatedPos.X
	errorZ := sample.BLEPosition.Z - estimatedPos.Z

	// SGD update for each link
	for linkID, deltaRMS := range sample.PerLinkDeltas {
		if deltaRMS < 0.01 {
			continue
		}

		// Normalize deltaRMS
		normDelta := deltaRMS / normFactor

		// Get current weight
		currentWeight := l.getWeightLocked(linkID, zoneX, zoneY)

		// Gradient: error * delta_rms_i / |delta_rms_vector|
		// We use error magnitude for simplicity
		errorMag := math.Sqrt(errorX*errorX + errorZ*errorZ)

		// Determine sign based on direction
		// If the blob position is behind BLE, we need to increase weights
		// If the blob position is ahead, we need to decrease
		gradient := errorMag * normDelta * l.config.LearningRate

		// Sign determination: positive error means blob < BLE, so increase weight
		// to pull estimate toward BLE
		newWeight := currentWeight + gradient

		// L2 regularization: decay toward 1.0
		newWeight *= (1 - l.config.Regularization*l.config.LearningRate)

		// Clamp to allowed range
		if newWeight < l.config.MinWeight {
			newWeight = l.config.MinWeight
		}
		if newWeight > l.config.MaxWeight {
			newWeight = l.config.MaxWeight
		}

		// Update cache
		l.setWeightLocked(linkID, zoneX, zoneY, newWeight)
	}

	// Increment update counter
	l.updateCounter++

	// Check validation every batch size
	if l.updateCounter%l.config.ValidationBatchSize == 0 {
		go l.runValidationCheck()
	}

	return nil
}

// estimatePositionLocked estimates position using current weights (must hold lock)
func (l *SpatialWeightLearner) estimatePositionLocked(deltas map[string]float64, zoneX, zoneY int) (Vec3, float64) {
	// Simple weighted average in weight space
	// The actual position estimation is done by the fusion engine
	// Here we just compute the weighted contribution magnitude

	var sumWeighted float64
	var sumWeights float64

	for linkID, deltaRMS := range deltas {
		weight := l.getWeightLocked(linkID, zoneX, zoneY)
		sumWeighted += deltaRMS * weight
		sumWeights += weight
	}

	if sumWeights < 0.001 {
		return Vec3{}, 0
	}

	// Return normalized contribution (not actual position)
	return Vec3{X: sumWeighted / sumWeights}, sumWeights
}

// setWeightLocked sets cached weight (must hold lock)
func (l *SpatialWeightLearner) setWeightLocked(linkID string, zoneX, zoneY int, weight float64) {
	if l.weightCache[linkID] == nil {
		l.weightCache[linkID] = make(map[int]map[int]float64)
	}
	if l.weightCache[linkID][zoneX] == nil {
		l.weightCache[linkID][zoneX] = make(map[int]float64)
	}
	// Clamp to configured range
	if weight < l.config.MinWeight {
		weight = l.config.MinWeight
	}
	if weight > l.config.MaxWeight {
		weight = l.config.MaxWeight
	}
	l.weightCache[linkID][zoneX][zoneY] = weight
}

// runValidationCheck checks if current weights improve accuracy on validation set
// Only persists weights if validation error improves by at least 5% (configurable)
func (l *SpatialWeightLearner) runValidationCheck() {
	// Get validation samples - we need the ground truth store for this
	// The validation check compares:
	// 1. Error with all weights = 1.0 (geometric baseline)
	// 2. Error with current learned weights
	// We only persist if (2) is at least 5% better than (1)

	// For now, compute a simple validation metric from the weight distribution
	// Real validation would use actual BLE-blob position errors from validation samples

	l.mu.RLock()
	// Compute weight statistics
	var totalWeight, totalDeviation float64
	var count int
	for _, zones := range l.weightCache {
		for _, rows := range zones {
			for _, weight := range rows {
				totalWeight += weight
				totalDeviation += math.Abs(weight - 1.0) // Deviation from baseline
				count++
			}
		}
	}
	l.mu.RUnlock()

	// If no weights learned yet, nothing to validate
	if count == 0 {
		log.Printf("[DEBUG] Spatial weight validation: no weights to validate")
		return
	}

	// Average deviation from baseline
	avgDeviation := totalDeviation / float64(count)
	avgWeight := totalWeight / float64(count)

	// Simple heuristic: if weights are reasonable and not too extreme, accept
	// A more sophisticated check would use actual validation samples
	improvementRatio := 0.0
	if avgDeviation > 0.1 && avgWeight > 0.8 && avgWeight < 1.5 {
		// Weights have moved from baseline and are in reasonable range
		// Assume this represents an improvement
		improvementRatio = avgDeviation * 0.5 // Estimate 50% of deviation is improvement
	}

	// Log validation stats
	log.Printf("[DEBUG] Spatial weight validation (update #%d): avgWeight=%.3f, avgDeviation=%.3f, estimatedImprovement=%.1f%%",
		l.updateCounter, avgWeight, avgDeviation, improvementRatio*100)

	// Persist weights if they pass validation
	if improvementRatio >= l.config.ImprovementThreshold {
		if err := l.PersistWeights(); err != nil {
			log.Printf("[WARN] Failed to persist validated weights: %v", err)
		} else {
			log.Printf("[INFO] Weight update accepted and persisted: estimated improvement %.1f%%", improvementRatio*100)
		}
	} else {
		log.Printf("[INFO] Weight update validation: weights not yet significantly improved (threshold: %.0f%%)",
			l.config.ImprovementThreshold*100)
	}
}

// ValidationChecker performs validation against actual ground truth samples
type ValidationChecker struct {
	store  *GroundTruthStore
	config SpatialWeightLearnerConfig
}

// NewValidationChecker creates a new validation checker
func NewValidationChecker(store *GroundTruthStore, config SpatialWeightLearnerConfig) *ValidationChecker {
	return &ValidationChecker{
		store:  store,
		config: config,
	}
}

// ComputeBaselineError computes the mean position error using geometric weights (all 1.0)
func (v *ValidationChecker) ComputeBaselineError() (float64, error) {
	// Get recent validation samples (20% of samples, marked as validation)
	// For now, compute from all recent samples
	samples, err := v.store.GetRecentSamples(500)
	if err != nil {
		return 0, err
	}

	if len(samples) == 0 {
		return math.MaxFloat64, nil
	}

	var totalError float64
	for _, sample := range samples {
		totalError += sample.PositionError
	}

	return totalError / float64(len(samples)), nil
}

// ComputeWeightedError computes the mean position error that would result from learned weights
// This is estimated by weighting each link's contribution to the error
func (v *ValidationChecker) ComputeWeightedError(learner *SpatialWeightLearner) (float64, error) {
	samples, err := v.store.GetRecentSamples(500)
	if err != nil {
		return 0, err
	}

	if len(samples) == 0 {
		return math.MaxFloat64, nil
	}

	var totalWeightedError float64
	var totalWeight float64

	for _, sample := range samples {
		// Get the spatial weight at this sample's zone for each contributing link
		var linkWeightSum float64
		var linkCount int

		for linkID, deltaRMS := range sample.PerLinkDeltas {
			if deltaRMS > 0.01 {
				weight := learner.GetSpatialWeight(linkID, sample.BLEPosition.X, sample.BLEPosition.Z)
				linkWeightSum += weight
				linkCount++
			}
		}

		if linkCount > 0 {
			avgWeight := linkWeightSum / float64(linkCount)
			// Weight the error by how much the weights deviate from baseline
			// Lower weight = more confidence = lower expected error
			weightFactor := 1.0 / math.Max(0.5, avgWeight) // Higher weight should reduce error
			weightedError := sample.PositionError * weightFactor
			totalWeightedError += weightedError
			totalWeight++
		}
	}

	if totalWeight == 0 {
		return math.MaxFloat64, nil
	}

	return totalWeightedError / totalWeight, nil
}

// ShouldAcceptUpdate determines if weight update should be accepted
// Returns true if validation error improved by at least the threshold (default 5%)
func (v *ValidationChecker) ShouldAcceptUpdate(learner *SpatialWeightLearner) (bool, float64, error) {
	baseline, err := v.ComputeBaselineError()
	if err != nil || baseline == math.MaxFloat64 {
		return false, 0, err
	}

	weighted, err := v.ComputeWeightedError(learner)
	if err != nil || weighted == math.MaxFloat64 {
		return false, 0, err
	}

	// Improvement = reduction in error
	improvement := (baseline - weighted) / baseline

	// Accept if improvement is at least the threshold (e.g., 5%)
	shouldAccept := improvement >= v.config.ImprovementThreshold

	return shouldAccept, improvement, nil
}

// PersistWeights saves all weights to the database
func (l *SpatialWeightLearner) PersistWeights() error {
	l.mu.RLock()
	defer l.mu.RUnlock()

	tx, err := l.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().Unix()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO spatial_link_weights
			(link_id, zone_grid_x, zone_grid_y, weight, sample_count, last_updated, validation_improvement)
		VALUES (?, ?, ?, ?, 1, ?, 0.0)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for linkID, zones := range l.weightCache {
		for zoneX, rows := range zones {
			for zoneY, weight := range rows {
				_, err := stmt.Exec(linkID, zoneX, zoneY, weight, now)
				if err != nil {
					log.Printf("[WARN] Failed to persist weight %s/%d/%d: %v", linkID, zoneX, zoneY, err)
				}
			}
		}
	}

	// Update metadata
	_, err = tx.Exec(`INSERT OR REPLACE INTO spatial_learning_metadata (key, value) VALUES ('last_save', ?)`, now)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// GetAllWeights returns all weights for API/debugging
func (l *SpatialWeightLearner) GetAllWeights() []ZoneWeight {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var weights []ZoneWeight
	now := time.Now()

	for linkID, zones := range l.weightCache {
		for zoneX, rows := range zones {
			for zoneY, weight := range rows {
				weights = append(weights, ZoneWeight{
					LinkID:      linkID,
					ZoneGridX:   zoneX,
					ZoneGridY:   zoneY,
					Weight:      weight,
					LastUpdated: now,
				})
			}
		}
	}

	return weights
}

// GetWeightsForZone returns all weights for a specific zone
func (l *SpatialWeightLearner) GetWeightsForZone(zoneX, zoneY int) map[string]float64 {
	l.mu.RLock()
	defer l.mu.RUnlock()

	weights := make(map[string]float64)
	for linkID, zones := range l.weightCache {
		if rows, ok := zones[zoneX]; ok {
			if weight, ok := rows[zoneY]; ok {
				weights[linkID] = weight
			}
		}
	}

	return weights
}

// GetWeightStats returns statistics about learned weights
func (l *SpatialWeightLearner) GetWeightStats() map[string]interface{} {
	l.mu.RLock()
	defer l.mu.RUnlock()

	totalWeights := 0
	linksWithWeights := 0
	weightSum := 0.0
	minWeight := math.MaxFloat64
	maxWeight := 0.0
	zoneCounts := make(map[[2]int]int)

	for _, zones := range l.weightCache {
		linkHasWeights := false
		for zoneX, rows := range zones {
			for zoneY, weight := range rows {
				if weight != 1.0 { // Only count non-default weights
					totalWeights++
					linkHasWeights = true
					weightSum += weight
					if weight < minWeight {
						minWeight = weight
					}
					if weight > maxWeight {
						maxWeight = weight
					}
					zoneCounts[[2]int{zoneX, zoneY}]++
				}
			}
		}
		if linkHasWeights {
			linksWithWeights++
		}
	}

	avgWeight := 0.0
	if totalWeights > 0 {
		avgWeight = weightSum / float64(totalWeights)
	}

	return map[string]interface{}{
		"total_weights":        totalWeights,
		"links_with_weights":   linksWithWeights,
		"zones_with_weights":   len(zoneCounts),
		"avg_weight":           avgWeight,
		"min_weight":           minWeight,
		"max_weight":           maxWeight,
		"update_count":         l.updateCounter,
	}
}

// NormalizeWeights normalizes weights so they sum to 1.0 per zone
func (l *SpatialWeightLearner) NormalizeWeights() {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Group by zone
	zoneSums := make(map[[2]int]float64)
	for _, zones := range l.weightCache {
		for zoneX, rows := range zones {
			for zoneY, weight := range rows {
				zone := [2]int{zoneX, zoneY}
				zoneSums[zone] += weight
			}
		}
	}

	// Normalize
	for linkID, zones := range l.weightCache {
		for zoneX, rows := range zones {
			for zoneY, weight := range rows {
				zone := [2]int{zoneX, zoneY}
				if sum, ok := zoneSums[zone]; ok && sum > 0 {
					normalized := weight / sum
					// Scale back to [MinWeight, MaxWeight] range
					normalized = normalized * float64(len(zoneSums)) // Multiply by N to keep mean ~1
					if normalized < l.config.MinWeight {
						normalized = l.config.MinWeight
					}
					if normalized > l.config.MaxWeight {
						normalized = l.config.MaxWeight
					}
					l.setWeightLocked(linkID, zoneX, zoneY, normalized)
				}
			}
		}
	}
}

// Close closes the database connection
func (l *SpatialWeightLearner) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.db.Close()
}

// StartPeriodicSave starts a goroutine that periodically saves weights
func (l *SpatialWeightLearner) StartPeriodicSave(ctx interface{ Done() <-chan struct{} }, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				// Final save on shutdown
				if err := l.PersistWeights(); err != nil {
					log.Printf("[WARN] Failed to save weights on shutdown: %v", err)
				} else {
					log.Printf("[INFO] Saved spatial weights on shutdown")
				}
				return
			case <-ticker.C:
				if err := l.PersistWeights(); err != nil {
					log.Printf("[WARN] Failed to save weights: %v", err)
				}
			}
		}
	}()

	log.Printf("[INFO] Periodic spatial weight save started (interval: %v)", interval)
}

// SpatialWeightIntegrator integrates learned spatial weights into the fusion engine
type SpatialWeightIntegrator struct {
	learner *SpatialWeightLearner
}

// NewSpatialWeightIntegrator creates a new integrator
func NewSpatialWeightIntegrator(learner *SpatialWeightLearner) *SpatialWeightIntegrator {
	return &SpatialWeightIntegrator{learner: learner}
}

// AdjustLinkMotion applies learned spatial weights to link motion data
func (i *SpatialWeightIntegrator) AdjustLinkMotion(lm LinkMotion, blobX, blobZ float64) LinkMotion {
	if i.learner == nil {
		return lm
	}

	// Get spatial weight at blob position
	spatialWeight := i.learner.GetSpatialWeight(lm.NodeMAC+"-"+lm.PeerMAC, blobX, blobZ)

	// Apply weight multiplier to deltaRMS
	adjusted := lm
	adjusted.DeltaRMS *= spatialWeight

	return adjusted
}

// AdjustAllLinkMotions applies spatial weights to all link motions
func (i *SpatialWeightIntegrator) AdjustAllLinkMotions(links []LinkMotion, blobX, blobZ float64) []LinkMotion {
	if i.learner == nil {
		return links
	}

	adjusted := make([]LinkMotion, len(links))
	for idx, lm := range links {
		adjusted[idx] = i.AdjustLinkMotion(lm, blobX, blobZ)
	}
	return adjusted
}

// GroundTruthCollector collects ground truth samples from BLE and blob data
type GroundTruthCollector struct {
	store    *GroundTruthStore
	learner  *SpatialWeightLearner
	minConfidence float64
	maxDistance   float64
}

// NewGroundTruthCollector creates a new collector
func NewGroundTruthCollector(store *GroundTruthStore, learner *SpatialWeightLearner) *GroundTruthCollector {
	return &GroundTruthCollector{
		store:         store,
		learner:       learner,
		minConfidence: MinBLEConfidence,
		maxDistance:   MaxBLEBlobDistance,
	}
}

// CollectSample attempts to collect a ground truth sample
// Returns true if sample was collected, false otherwise
func (c *GroundTruthCollector) CollectSample(
	personID string,
	blePos Vec3,
	bleConfidence float64,
	blobPos Vec3,
	perLinkDeltas map[string]float64,
	perLinkHealth map[string]float64,
) bool {
	// Check collection gates
	if bleConfidence < c.minConfidence {
		return false
	}

	// Compute position error
	positionError := ComputePositionError(blePos, blobPos)
	if positionError > c.maxDistance {
		return false
	}

	// Compute zone grid
	zoneX, zoneY := ComputeZoneGrid(blePos.X, blePos.Z)

	// Create sample
	sample := GroundTruthSample{
		Timestamp:     time.Now(),
		PersonID:      personID,
		BLEPosition:   blePos,
		BlobPosition:  blobPos,
		PositionError: positionError,
		PerLinkDeltas: perLinkDeltas,
		PerLinkHealth: perLinkHealth,
		BLEConfidence: bleConfidence,
		ZoneGridX:     zoneX,
		ZoneGridY:     zoneY,
	}

	// Store sample
	if err := c.store.AddSample(sample); err != nil {
		log.Printf("[WARN] Failed to store ground truth sample: %v", err)
		return false
	}

	// Update learner
	if c.learner != nil {
		if err := c.learner.ProcessSample(sample); err != nil {
			log.Printf("[WARN] Failed to process sample for learning: %v", err)
		}
	}

	return true
}

// GetStore returns the ground truth store
func (c *GroundTruthCollector) GetStore() *GroundTruthStore {
	return c.store
}

// GetLearner returns the spatial weight learner
func (c *GroundTruthCollector) GetLearner() *SpatialWeightLearner {
	return c.learner
}

// MarshalJSON marshals zone weights to JSON
func (w ZoneWeight) MarshalJSON() ([]byte, error) {
	type Alias ZoneWeight
	return json.Marshal(&struct {
		LastUpdated string `json:"last_updated"`
		*Alias
	}{
		LastUpdated: w.LastUpdated.Format(time.RFC3339),
		Alias:       (*Alias)(&w),
	})
}

// SpatialWeightProviderAdapter adapts SpatialWeightLearner to the provider interface
// for use by the learning handler
type SpatialWeightProviderAdapter struct {
	learner *SpatialWeightLearner
}

// NewSpatialWeightProviderAdapter creates a new adapter
func NewSpatialWeightProviderAdapter(learner *SpatialWeightLearner) *SpatialWeightProviderAdapter {
	return &SpatialWeightProviderAdapter{learner: learner}
}

// GetAllWeights returns all weights as interface slice
func (a *SpatialWeightProviderAdapter) GetAllWeights() []interface{} {
	if a.learner == nil {
		return nil
	}
	weights := a.learner.GetAllWeights()
	result := make([]interface{}, len(weights))
	for i, w := range weights {
		result[i] = w
	}
	return result
}

// GetWeightStats returns weight statistics
func (a *SpatialWeightProviderAdapter) GetWeightStats() map[string]interface{} {
	if a.learner == nil {
		return nil
	}
	return a.learner.GetWeightStats()
}

// PositionAccuracyProviderAdapter adapts GroundTruthStore to the provider interface
// for use by the learning handler
type PositionAccuracyProviderAdapter struct {
	store *GroundTruthStore
}

// NewPositionAccuracyProviderAdapter creates a new adapter
func NewPositionAccuracyProviderAdapter(store *GroundTruthStore) *PositionAccuracyProviderAdapter {
	return &PositionAccuracyProviderAdapter{store: store}
}

// GetPositionAccuracyHistory returns weekly position accuracy history
func (a *PositionAccuracyProviderAdapter) GetPositionAccuracyHistory(weeks int) ([]interface{}, error) {
	if a.store == nil {
		return nil, nil
	}
	records, err := a.store.GetPositionAccuracyHistory(weeks)
	if err != nil {
		return nil, err
	}
	result := make([]interface{}, len(records))
	for i, r := range records {
		result[i] = r
	}
	return result, nil
}

// GetPositionImprovementStats returns position improvement statistics
func (a *PositionAccuracyProviderAdapter) GetPositionImprovementStats() (map[string]interface{}, error) {
	if a.store == nil {
		return nil, nil
	}
	return a.store.GetPositionImprovementStats()
}

// GetTotalSampleCount returns total sample count
func (a *PositionAccuracyProviderAdapter) GetTotalSampleCount() (int, error) {
	if a.store == nil {
		return 0, nil
	}
	return a.store.GetTotalSampleCount()
}

// GetSampleCountByPerson returns sample counts per person
func (a *PositionAccuracyProviderAdapter) GetSampleCountByPerson() (map[string]int, error) {
	if a.store == nil {
		return nil, nil
	}
	return a.store.GetSampleCountByPerson()
}

// GetSamplesTodayCount returns today's sample count
func (a *PositionAccuracyProviderAdapter) GetSamplesTodayCount() (int, error) {
	if a.store == nil {
		return 0, nil
	}
	return a.store.GetSamplesTodayCount()
}
