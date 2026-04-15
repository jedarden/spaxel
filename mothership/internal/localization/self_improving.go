// Package localization provides self-improving localization using BLE ground truth
package localization

import (
	"log"
	"math"
	"sync"
	"time"
)

// SelfImprovingLocalizerConfig holds configuration for the self-improving localizer
type SelfImprovingLocalizerConfig struct {
	RoomWidth   float64
	RoomDepth   float64
	OriginX     float64
	OriginZ     float64
	AdjustmentInterval time.Duration // How often to adjust weights

	// BLE ground truth configuration
	BLEConfig BLETrilaterationConfig

	// Weight learning configuration
	LearningRate         float64
	Regularization       float64
	MinZoneSamples       int
	ValidationBatchSize  int
	ImprovementThreshold float64
	MinWeight            float64
	MaxWeight            float64

	// Collection gates
	MinBLEConfidence   float64
	MaxBLEBlobDistance float64
}

// DefaultSelfImprovingConfig returns sensible defaults (alias for DefaultSelfImprovingLocalizerConfig).
func DefaultSelfImprovingConfig() SelfImprovingLocalizerConfig {
	return DefaultSelfImprovingLocalizerConfig()
}

// DefaultSelfImprovingLocalizerConfig returns sensible defaults
func DefaultSelfImprovingLocalizerConfig() SelfImprovingLocalizerConfig {
	return SelfImprovingLocalizerConfig{
		RoomWidth:           10.0,
		RoomDepth:           10.0,
		OriginX:             0.0,
		OriginZ:             0.0,
		AdjustmentInterval:  10 * time.Second,
		BLEConfig:           DefaultBLETrilaterationConfig(),
		LearningRate:        0.001,
		Regularization:      0.01,
		MinZoneSamples:      100,
		ValidationBatchSize: 50,
		ImprovementThreshold: 0.05,
		MinWeight:           0.1,
		MaxWeight:           3.0,
		MinBLEConfidence:    MinBLEConfidence,
		MaxBLEBlobDistance:  MaxBLEBlobDistance,
	}
}

// SelfImprovingLocalizer ties together BLE ground truth, weight learning, and fusion
type SelfImprovingLocalizer struct {
	mu sync.RWMutex

	// Core components
	engine                *Engine
	weightLearner         *WeightLearner
	weightStore           *WeightStore
	spatialWeightLearner  *SpatialWeightLearner
	groundTruthProvider   GroundTruthSource

	// Configuration
	config SelfImprovingLocalizerConfig

	// Runtime state
	running       bool
	stopChan      chan struct{}
	lastAdjust    time.Time
	sampleCount   int
	adjustCount   int

	// Improvement tracking
	improvementHistory []ImprovementRecord
}

// ImprovementRecord records a weight adjustment and its effect
type ImprovementRecord struct {
	Timestamp       time.Time `json:"timestamp"`
	AdjustmentCount int       `json:"adjustment_count"`
	SampleCount     int       `json:"sample_count"`
	BaselineError   float64   `json:"baseline_error"`
	CurrentError    float64   `json:"current_error"`
	ImprovementPct  float64   `json:"improvement_pct"`
}

// NewSelfImprovingLocalizer creates a new self-improving localizer
func NewSelfImprovingLocalizer(config SelfImprovingLocalizerConfig) *SelfImprovingLocalizer {
	// Create fusion engine
	engine := NewEngine(config.RoomWidth, config.RoomDepth, config.OriginX, config.OriginZ)

	// Create BLE ground truth provider
	groundTruthProvider := NewBLEGroundTruthProvider(config.BLEConfig)

	// Create weight learner with proper config
	weightLearner := NewWeightLearner(groundTruthProvider, engine, WeightLearnerConfig{
		LearningRate:        config.LearningRate,
		MinSamples:          config.MinZoneSamples,
		MaxErrorDistance:    2.0, // Default max error distance
		RewardThreshold:     0.5, // Default reward threshold
		PenaltyThreshold:    1.5, // Default penalty threshold
		MinWeight:           config.MinWeight,
		MaxWeight:           config.MaxWeight,
		SigmaAdjustmentRate: 0.02,
		MinSigma:            0.5,
		MaxSigma:            2.0,
	})

	return &SelfImprovingLocalizer{
		engine:              engine,
		weightLearner:       weightLearner,
		groundTruthProvider: groundTruthProvider,
		config:              config,
		stopChan:            make(chan struct{}),
		improvementHistory:   make([]ImprovementRecord, 0),
	}
}

// GetEngine returns the fusion engine
func (s *SelfImprovingLocalizer) GetEngine() *Engine {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.engine
}

// SetLearnedWeights sets the learned weights
func (s *SelfImprovingLocalizer) SetLearnedWeights(weights *LearnedWeights) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.engine.SetLearnedWeights(weights)
}

// GetLearnedWeights returns the current learned weights
func (s *SelfImprovingLocalizer) GetLearnedWeights() *LearnedWeights {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.engine.GetLearnedWeights()
}

// SetNodePosition updates a node's position
func (s *SelfImprovingLocalizer) SetNodePosition(mac string, x, y, z float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.engine.SetNodePosition(mac, x, z)
	if provider, ok := s.groundTruthProvider.(*BLEGroundTruthProvider); ok {
		provider.SetNodePosition(mac, x, y, z)
	}
}

// SetSpatialWeightLearner sets the spatial weight learner
func (s *SelfImprovingLocalizer) SetSpatialWeightLearner(learner *SpatialWeightLearner) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.spatialWeightLearner = learner
	s.engine.SetSpatialWeightLearner(learner)
}

// AddBLEObservation adds a BLE RSSI observation for ground truth
func (s *SelfImprovingLocalizer) AddBLEObservation(deviceAddr, nodeMAC string, rssi float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if provider, ok := s.groundTruthProvider.(*BLEGroundTruthProvider); ok {
		provider.AddObservation(deviceAddr, nodeMAC, rssi, time.Now())
	}
}

// Fuse performs fusion with the given link motions
func (s *SelfImprovingLocalizer) Fuse(links []LinkMotion) *FusionResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.engine.Fuse(links)
}

// Start begins the background adjustment loop
func (s *SelfImprovingLocalizer) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	go s.adjustmentLoop()

	// Start BLE ground truth provider metrics if available
	if provider, ok := s.groundTruthProvider.(*BLEGroundTruthProvider); ok {
		provider.RegisterMetrics()
	}

	log.Printf("[INFO] Self-improving localizer started (adjustment interval: %v)", s.config.AdjustmentInterval)
}

// Stop halts the background adjustment loop
func (s *SelfImprovingLocalizer) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stopChan)
	s.mu.Unlock()

	log.Printf("[INFO] Self-improving localizer stopped")
}

// adjustmentLoop runs periodic weight adjustments
func (s *SelfImprovingLocalizer) adjustmentLoop() {
	ticker := time.NewTicker(s.config.AdjustmentInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.adjustWeights()
		}
	}
}

// adjustWeights performs weight adjustment based on collected ground truth
func (s *SelfImprovingLocalizer) adjustWeights() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get current ground truth positions
	allGT := s.groundTruthProvider.GetAllGroundTruth()
	if len(allGT) == 0 {
		return // No ground truth available
	}

	// Get current learned weights
	weights := s.engine.GetLearnedWeights()
	if weights == nil {
		// Initialize with default weights
		weights = NewLearnedWeights()
		s.engine.SetLearnedWeights(weights)
	}

	// Get last fusion result
	lastResult := s.engine.LastResult()
	if lastResult == nil || len(lastResult.Peaks) == 0 {
		return // No fusion result available
	}

	// For each ground truth position, record the prediction
	for entityID, gtPos := range allGT {
		if gtPos.Confidence < s.config.MinBLEConfidence {
			continue
		}

		// Record the prediction with the entity ID
		// Note: LinkStates not available from FusionResult, passing nil for now
		s.weightLearner.RecordPrediction(lastResult.Peaks, nil, entityID)
	}

	// Process learning - this will match predictions with ground truth
	if err := s.weightLearner.ProcessLearning(); err != nil {
		log.Printf("[WARN] Failed to process learning: %v", err)
		return
	}

	s.sampleCount += len(allGT)
	s.adjustCount++
	s.lastAdjust = time.Now()

	log.Printf("[DEBUG] Weight adjustment #%d: processed %d ground truth positions (total: %d)",
		s.adjustCount, len(allGT), s.sampleCount)

	// Record improvement snapshot
	var samples []GroundTruthSample
	for entityID, gtPos := range allGT {
		// Find nearest peak to ground truth position
		minDist := math.MaxFloat64
		for _, peak := range lastResult.Peaks {
			dx := peak[0] - gtPos.X
			dz := peak[2] - gtPos.Z
			dist := math.Sqrt(dx*dx + dz*dz)
			if dist < minDist {
				minDist = dist
			}
		}

		sample := GroundTruthSample{
			Timestamp:     time.Now(),
			PersonID:      entityID,
			BLEPosition:   Vec3{X: gtPos.X, Y: gtPos.Y, Z: gtPos.Z},
			PositionError: minDist,
			BLEConfidence: gtPos.Confidence,
		}
		samples = append(samples, sample)
	}
	s.recordImprovementSnapshot(samples)

	// Persist weights if store is available
	if s.weightStore != nil {
		if err := s.weightStore.SaveWeights(weights); err != nil {
			log.Printf("[WARN] Failed to save weights: %v", err)
		}
	}
}

// recordImprovementSnapshot records the current improvement state
func (s *SelfImprovingLocalizer) recordImprovementSnapshot(samples []GroundTruthSample) {
	if len(samples) == 0 {
		return
	}

	// Compute average position error
	var totalError float64
	for _, s := range samples {
		totalError += s.PositionError
	}
	avgError := totalError / float64(len(samples))

	// Get baseline error (from first record or current)
	baselineError := avgError
	if len(s.improvementHistory) > 0 {
		baselineError = s.improvementHistory[0].BaselineError
	}

	// Compute improvement percentage
	improvementPct := 0.0
	if baselineError > 0 {
		improvementPct = ((baselineError - avgError) / baselineError) * 100
	}

	record := ImprovementRecord{
		Timestamp:       time.Now(),
		AdjustmentCount: s.adjustCount,
		SampleCount:     s.sampleCount,
		BaselineError:   baselineError,
		CurrentError:    avgError,
		ImprovementPct:  improvementPct,
	}

	s.improvementHistory = append(s.improvementHistory, record)

	// Keep last 100 records
	if len(s.improvementHistory) > 100 {
		s.improvementHistory = s.improvementHistory[len(s.improvementHistory)-100:]
	}
}

// GetLearningProgress returns current learning progress
func (s *SelfImprovingLocalizer) GetLearningProgress() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	progress := map[string]interface{}{
		"running":          s.running,
		"sample_count":     s.sampleCount,
		"adjustment_count": s.adjustCount,
		"last_adjustment":  s.lastAdjust,
	}

	// Add weight stats
	weights := s.engine.GetLearnedWeights()
	if weights != nil {
		stats := weights.GetAllStats()
		progress["weights_learned"] = len(stats)
		progress["weight_stats"] = stats
	}

	return progress
}

// GetImprovementStats returns improvement statistics
func (s *SelfImprovingLocalizer) GetImprovementStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.improvementHistory) == 0 {
		return map[string]interface{}{
			"message": "no improvement data yet",
		}
	}

	latest := s.improvementHistory[len(s.improvementHistory)-1]

	// Compute trend (last 5 records)
	trend := "stable"
	if len(s.improvementHistory) >= 5 {
		recent := s.improvementHistory[len(s.improvementHistory)-5:]
		improvingCount := 0
		for _, r := range recent {
			if r.ImprovementPct > 0 {
				improvingCount++
			}
		}
		if improvingCount >= 4 {
			trend = "improving"
		} else if improvingCount == 0 {
			trend = "degrading"
		}
	}

	return map[string]interface{}{
		"total_samples":      s.sampleCount,
		"adjustments":        s.adjustCount,
		"baseline_error_m":   latest.BaselineError,
		"current_error_m":    latest.CurrentError,
		"improvement_pct":    latest.ImprovementPct,
		"trend":              trend,
		"last_adjustment":    latest.Timestamp,
	}
}

// GetImprovementHistory returns improvement history records
func (s *SelfImprovingLocalizer) GetImprovementHistory() []interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]interface{}, len(s.improvementHistory))
	for i, r := range s.improvementHistory {
		result[i] = r
	}
	return result
}

// GetGroundTruthProvider returns the ground truth provider
func (s *SelfImprovingLocalizer) GetGroundTruthProvider() GroundTruthSource {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.groundTruthProvider
}

// GetGroundTruth returns the ground truth position for a specific entity.
func (s *SelfImprovingLocalizer) GetGroundTruth(entityID string) *GroundTruthPosition {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.groundTruthProvider.GetGroundTruth(entityID)
}

// GetAllGroundTruth returns all current ground truth positions
func (s *SelfImprovingLocalizer) GetAllGroundTruth() map[string]*GroundTruthPosition {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.groundTruthProvider.GetAllGroundTruth()
}

// SetWeightStore sets the weight store for persistence
func (s *SelfImprovingLocalizer) SetWeightStore(store *WeightStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.weightStore = store
}

// SetSpatialWeightLearnerStore sets the spatial weight learner (if separate)
func (s *SelfImprovingLocalizer) SetSpatialWeightLearnerStore(learner *SpatialWeightLearner) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.spatialWeightLearner = learner
	s.engine.SetSpatialWeightLearner(learner)
}
