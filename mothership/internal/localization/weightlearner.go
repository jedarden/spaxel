// Package localization provides weight learning for self-improving localization
package localization

import (
	"log"
	"math"
	"sync"
	"time"
)

// ErrorHistoryEntry tracks error at a point in time for improvement tracking
type ErrorHistoryEntry struct {
	Timestamp     time.Time `json:"timestamp"`
	AvgErrorM     float64   `json:"avg_error_m"`
	Observations  int64     `json:"observations"`
	LinksLearning int       `json:"links_learning"`
}

// LearnedWeights stores per-link learned weight adjustments
type LearnedWeights struct {
	mu sync.RWMutex

	// Per-link weight multipliers: "nodeMAC-peerMAC" -> weight
	linkWeights map[string]float64

	// Per-link sigma adjustments for Fresnel zone calculations
	linkSigmas map[string]float64

	// Learning statistics per link
	linkStats map[string]*LinkLearningStats

	// Error history for improvement tracking (last 100 entries, ~16 minutes at 10s interval)
	errorHistory []ErrorHistoryEntry

	// Global learning rate
	learningRate float64

	// Decay factor for old observations
	decayFactor float64

	// Last update time
	lastUpdate time.Time
}

// LinkLearningStats tracks learning statistics for a single link
type LinkLearningStats struct {
	ObservationCount   int64     `json:"observation_count"`
	CorrectCount       int64     `json:"correct_count"`
	ErrorSum           float64   `json:"error_sum"`
	ErrorSumSquared    float64   `json:"error_sum_squared"`
	LastError          float64   `json:"last_error"`
	WeightAdjustments  int64     `json:"weight_adjustments"`
	LastAdjustmentTime time.Time `json:"last_adjustment_time"`
}

// NewLearnedWeights creates a new learned weights store
func NewLearnedWeights() *LearnedWeights {
	return &LearnedWeights{
		linkWeights:  make(map[string]float64),
		linkSigmas:   make(map[string]float64),
		linkStats:    make(map[string]*LinkLearningStats),
		errorHistory: make([]ErrorHistoryEntry, 0, 100),
		learningRate: 0.1,
		decayFactor:  0.99,
		lastUpdate:   time.Now(),
	}
}

// GetLinkWeight returns the learned weight multiplier for a link
func (lw *LearnedWeights) GetLinkWeight(linkID string) float64 {
	lw.mu.RLock()
	defer lw.mu.RUnlock()
	if w, ok := lw.linkWeights[linkID]; ok {
		return w
	}
	return 1.0 // Default: no adjustment
}

// GetLinkSigma returns the learned sigma adjustment for a link
func (lw *LearnedWeights) GetLinkSigma(linkID string) float64 {
	lw.mu.RLock()
	defer lw.mu.RUnlock()
	if s, ok := lw.linkSigmas[linkID]; ok {
		return s
	}
	return 0.0 // Default: no adjustment
}

// GetAllWeights returns a copy of all learned weights
func (lw *LearnedWeights) GetAllWeights() map[string]float64 {
	lw.mu.RLock()
	defer lw.mu.RUnlock()
	result := make(map[string]float64, len(lw.linkWeights))
	for k, v := range lw.linkWeights {
		result[k] = v
	}
	return result
}

// GetAllSigmas returns a copy of all learned sigmas
func (lw *LearnedWeights) GetAllSigmas() map[string]float64 {
	lw.mu.RLock()
	defer lw.mu.RUnlock()
	result := make(map[string]float64, len(lw.linkSigmas))
	for k, v := range lw.linkSigmas {
		result[k] = v
	}
	return result
}

// GetAllStats returns all learning statistics
func (lw *LearnedWeights) GetAllStats() map[string]*LinkLearningStats {
	lw.mu.RLock()
	defer lw.mu.RUnlock()
	result := make(map[string]*LinkLearningStats, len(lw.linkStats))
	for k, v := range lw.linkStats {
		result[k] = v
	}
	return result
}

// SetWeights sets the weight and sigma for a link (used for loading from persistence)
func (lw *LearnedWeights) SetWeights(linkID string, weight, sigma float64) {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	lw.linkWeights[linkID] = weight
	lw.linkSigmas[linkID] = sigma
	lw.lastUpdate = time.Now()
}

// SetStats sets the stats for a link (used for loading from persistence)
func (lw *LearnedWeights) SetStats(linkID string, stats *LinkLearningStats) {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	lw.linkStats[linkID] = stats
}

// WeightLearner learns Fresnel zone weights from ground truth feedback
type WeightLearner struct {
	mu sync.RWMutex

	weights      *LearnedWeights
	groundTruth  GroundTruthSource
	fusionEngine *Engine

	// Configuration
	config WeightLearnerConfig

	// Learning buffer: stores recent predictions for comparison
	predictionBuffer []*LearningSample

	// Maximum buffer size
	maxBufferSize int
}

// WeightLearnerConfig holds configuration for the weight learner
type WeightLearnerConfig struct {
	// LearningRate controls how fast weights adapt (0-1)
	LearningRate float64

	// MinSamples is the minimum samples before learning starts
	MinSamples int

	// MaxErrorDistance is the maximum distance error to consider (metres)
	MaxErrorDistance float64

	// RewardThreshold is the error threshold for positive reward (metres)
	RewardThreshold float64

	// PenaltyThreshold is the error threshold for penalty (metres)
	PenaltyThreshold float64

	// MinWeight is the minimum allowed weight multiplier
	MinWeight float64

	// MaxWeight is the maximum allowed weight multiplier
	MaxWeight float64

	// SigmaAdjustmentRate is the rate for sigma adjustments
	SigmaAdjustmentRate float64

	// MinSigma is the minimum sigma multiplier
	MinSigma float64

	// MaxSigma is the maximum sigma multiplier
	MaxSigma float64
}

// DefaultWeightLearnerConfig returns sensible defaults
func DefaultWeightLearnerConfig() WeightLearnerConfig {
	return WeightLearnerConfig{
		LearningRate:        0.05,
		MinSamples:          10,
		MaxErrorDistance:    3.0,
		RewardThreshold:     0.5,
		PenaltyThreshold:    1.5,
		MinWeight:           0.1,
		MaxWeight:           3.0,
		SigmaAdjustmentRate: 0.02,
		MinSigma:            0.5,
		MaxSigma:            2.0,
	}
}

// LearningSample stores a prediction for later comparison with ground truth
type LearningSample struct {
	Timestamp   time.Time
	Peaks       [][3]float64 // Predicted positions (x, z, weight)
	LinkStates  []LinkMotion // Link states used for prediction
	EntityID    string       // Associated BLE entity (if known)
	GroundTruth *GroundTruthPosition
}

// NewWeightLearner creates a new weight learner
func NewWeightLearner(groundTruth GroundTruthSource, engine *Engine, config WeightLearnerConfig) *WeightLearner {
	if config.LearningRate <= 0 {
		config = DefaultWeightLearnerConfig()
	}

	return &WeightLearner{
		weights:          NewLearnedWeights(),
		groundTruth:      groundTruth,
		fusionEngine:     engine,
		config:           config,
		predictionBuffer: make([]*LearningSample, 0, 100),
		maxBufferSize:    100,
	}
}

// GetLearnedWeights returns the learned weights for use by the fusion engine
func (wl *WeightLearner) GetLearnedWeights() *LearnedWeights {
	return wl.weights
}

// RecordPrediction records a prediction for later learning
func (wl *WeightLearner) RecordPrediction(peaks [][3]float64, linkStates []LinkMotion, entityID string) {
	wl.mu.Lock()
	defer wl.mu.Unlock()

	sample := &LearningSample{
		Timestamp:  time.Now(),
		Peaks:      peaks,
		LinkStates: linkStates,
		EntityID:   entityID,
	}

	// Add to buffer
	wl.predictionBuffer = append(wl.predictionBuffer, sample)

	// Trim buffer if needed
	if len(wl.predictionBuffer) > wl.maxBufferSize {
		wl.predictionBuffer = wl.predictionBuffer[1:]
	}
}

// ProcessLearning processes buffered predictions against available ground truth
func (wl *WeightLearner) ProcessLearning() error {
	wl.mu.Lock()
	defer wl.mu.Unlock()

	if wl.groundTruth == nil {
		return nil
	}

	var processedSamples []*LearningSample

	for _, sample := range wl.predictionBuffer {
		// Get ground truth for this sample
		var gt *GroundTruthPosition
		if sample.EntityID != "" {
			gt = wl.groundTruth.GetGroundTruth(sample.EntityID)
		} else {
			// Try to find any recent ground truth
			allGT := wl.groundTruth.GetAllGroundTruth()
			for _, pos := range allGT {
				if pos.Timestamp.After(sample.Timestamp.Add(-2*time.Second)) &&
					pos.Timestamp.Before(sample.Timestamp.Add(2*time.Second)) {
					gt = pos
					break
				}
			}
		}

		if gt == nil {
			continue
		}

		// Find the closest prediction peak to ground truth
		bestError := math.MaxFloat64
		var bestPeak [3]float64

		for _, peak := range sample.Peaks {
			dx := peak[0] - gt.X
			dz := peak[1] - gt.Z
			error := math.Sqrt(dx*dx + dz*dz)
			if error < bestError {
				bestError = error
				bestPeak = peak
			}
		}

		// Skip if no valid peak found
		if bestError == math.MaxFloat64 {
			continue
		}

		// Store ground truth with sample
		sample.GroundTruth = gt

		// Learn from this sample
		wl.learnFromSample(sample, bestPeak, bestError, gt)

		processedSamples = append(processedSamples, sample)
	}

	// Remove processed samples
	if len(processedSamples) > 0 {
		newBuffer := make([]*LearningSample, 0, wl.maxBufferSize)
		for _, sample := range wl.predictionBuffer {
			kept := true
			for _, processed := range processedSamples {
				if sample == processed {
					kept = false
					break
				}
			}
			if kept {
				newBuffer = append(newBuffer, sample)
			}
		}
		wl.predictionBuffer = newBuffer
	}

	return nil
}

// learnFromSample updates weights based on a single prediction-ground truth pair
func (wl *WeightLearner) learnFromSample(sample *LearningSample, bestPeak [3]float64, error float64, gt *GroundTruthPosition) {
	// Skip if error is too large (outlier)
	if error > wl.config.MaxErrorDistance {
		return
	}

	// Compute reward/penalty signal
	var reward float64
	if error < wl.config.RewardThreshold {
		// Good prediction: reward the links
		reward = (wl.config.RewardThreshold - error) / wl.config.RewardThreshold
	} else if error > wl.config.PenaltyThreshold {
		// Bad prediction: penalize the links
		reward = -(error - wl.config.PenaltyThreshold) / wl.config.PenaltyThreshold
	} else {
		// Neutral zone
		reward = 0
	}

	// Scale by ground truth confidence and learning rate
	adjustment := reward * gt.Confidence * wl.config.LearningRate

	// Update weights for each link based on its contribution
	for _, lm := range sample.LinkStates {
		if !lm.Motion || lm.DeltaRMS < 0.01 {
			continue
		}

		linkID := lm.NodeMAC + "-" + lm.PeerMAC

		// Compute this link's contribution to the peak
		// Links closer to the ground truth position have more influence
		linkContribution := wl.computeLinkContribution(lm, bestPeak, gt)

		// Update weight
		linkAdjustment := adjustment * linkContribution
		wl.updateLinkWeight(linkID, linkAdjustment)

		// Also adjust sigma based on error
		sigmaAdjustment := wl.computeSigmaAdjustment(error, linkContribution)
		wl.updateLinkSigma(linkID, sigmaAdjustment)

		// Update stats
		wl.updateStats(linkID, error)
	}
}

// computeLinkContribution estimates how much a link contributed to a prediction
func (wl *WeightLearner) computeLinkContribution(lm LinkMotion, peak [3]float64, gt *GroundTruthPosition) float64 {
	// Simple heuristic: links whose Fresnel zone passes near both the peak and ground truth
	// contribute more. We approximate this by checking if the ground truth is close to the
	// line between the peak and the link midpoint.

	// For now, use deltaRMS as a proxy for contribution
	contribution := lm.DeltaRMS

	// Weight by health score if available
	if lm.HealthScore > 0 {
		contribution *= lm.HealthScore
	}

	// Normalize
	if contribution > 1.0 {
		contribution = 1.0
	}

	return contribution
}

// computeSigmaAdjustment computes sigma adjustment based on error
func (wl *WeightLearner) computeSigmaAdjustment(error, contribution float64) float64 {
	// If error is large and contribution is high, increase sigma (widen Fresnel zone)
	// If error is small and contribution is high, decrease sigma (narrow Fresnel zone)

	if error < wl.config.RewardThreshold {
		// Good localization: narrow the Fresnel zone slightly
		return -wl.config.SigmaAdjustmentRate * contribution
	} else if error > wl.config.PenaltyThreshold {
		// Poor localization: widen the Fresnel zone
		return wl.config.SigmaAdjustmentRate * contribution
	}
	return 0
}

// updateLinkWeight updates the weight for a link
func (wl *WeightLearner) updateLinkWeight(linkID string, adjustment float64) {
	wl.weights.mu.Lock()
	defer wl.weights.mu.Unlock()

	currentWeight := wl.weights.linkWeights[linkID]
	if currentWeight == 0 {
		currentWeight = 1.0
	}

	newWeight := currentWeight + adjustment

	// Clamp to allowed range
	if newWeight < wl.config.MinWeight {
		newWeight = wl.config.MinWeight
	}
	if newWeight > wl.config.MaxWeight {
		newWeight = wl.config.MaxWeight
	}

	wl.weights.linkWeights[linkID] = newWeight
	wl.weights.lastUpdate = time.Now()

	// Update stats
	if wl.weights.linkStats[linkID] == nil {
		wl.weights.linkStats[linkID] = &LinkLearningStats{}
	}
	wl.weights.linkStats[linkID].WeightAdjustments++
	wl.weights.linkStats[linkID].LastAdjustmentTime = time.Now()
}

// updateLinkSigma updates the sigma for a link
func (wl *WeightLearner) updateLinkSigma(linkID string, adjustment float64) {
	wl.weights.mu.Lock()
	defer wl.weights.mu.Unlock()

	currentSigma := wl.weights.linkSigmas[linkID]
	newSigma := currentSigma + adjustment

	// Clamp to allowed range
	if newSigma < wl.config.MinSigma {
		newSigma = wl.config.MinSigma
	}
	if newSigma > wl.config.MaxSigma {
		newSigma = wl.config.MaxSigma
	}

	wl.weights.linkSigmas[linkID] = newSigma
}

// updateStats updates learning statistics for a link
func (wl *WeightLearner) updateStats(linkID string, error float64) {
	wl.weights.mu.Lock()
	defer wl.weights.mu.Unlock()

	stats := wl.weights.linkStats[linkID]
	if stats == nil {
		stats = &LinkLearningStats{}
		wl.weights.linkStats[linkID] = stats
	}

	stats.ObservationCount++
	stats.ErrorSum += error
	stats.ErrorSumSquared += error * error
	stats.LastError = error

	if error < wl.config.RewardThreshold {
		stats.CorrectCount++
	}
}

// GetLinkStats returns learning statistics for a link
func (wl *WeightLearner) GetLinkStats(linkID string) *LinkLearningStats {
	wl.weights.mu.RLock()
	defer wl.weights.mu.RUnlock()
	if stats, ok := wl.weights.linkStats[linkID]; ok {
		return stats
	}
	return nil
}

// GetAllStats returns all learning statistics
func (wl *WeightLearner) GetAllStats() map[string]*LinkLearningStats {
	wl.weights.mu.RLock()
	defer wl.weights.mu.RUnlock()
	result := make(map[string]*LinkLearningStats, len(wl.weights.linkStats))
	for k, v := range wl.weights.linkStats {
		result[k] = v
	}
	return result
}

// GetLearningProgress returns overall learning progress
func (wl *WeightLearner) GetLearningProgress() map[string]interface{} {
	wl.weights.mu.RLock()
	defer wl.weights.mu.RUnlock()

	var totalObs, totalCorrect int64
	var totalError float64
	linkCount := len(wl.weights.linkStats)

	for _, stats := range wl.weights.linkStats {
		totalObs += stats.ObservationCount
		totalCorrect += stats.CorrectCount
		totalError += stats.ErrorSum
	}

	avgError := 0.0
	if totalObs > 0 {
		avgError = totalError / float64(totalObs)
	}

	accuracy := 0.0
	if totalObs > 0 {
		accuracy = float64(totalCorrect) / float64(totalObs)
	}

	return map[string]interface{}{
		"links_learning":      linkCount,
		"total_observations":  totalObs,
		"correct_predictions": totalCorrect,
		"accuracy":            accuracy,
		"average_error_m":     avgError,
		"last_update":         wl.weights.lastUpdate,
	}
}

// RecordErrorSnapshot records a snapshot of current error for improvement tracking
func (wl *WeightLearner) RecordErrorSnapshot() {
	wl.weights.mu.Lock()
	defer wl.weights.mu.Unlock()

	var totalObs int64
	var totalError float64
	linkCount := len(wl.weights.linkStats)

	for _, stats := range wl.weights.linkStats {
		totalObs += stats.ObservationCount
		totalError += stats.ErrorSum
	}

	avgError := 0.0
	if totalObs > 0 {
		avgError = totalError / float64(totalObs)
	}

	entry := ErrorHistoryEntry{
		Timestamp:     time.Now(),
		AvgErrorM:     avgError,
		Observations:  totalObs,
		LinksLearning: linkCount,
	}

	// Add to history
	wl.weights.errorHistory = append(wl.weights.errorHistory, entry)

	// Keep only last 100 entries
	if len(wl.weights.errorHistory) > 100 {
		wl.weights.errorHistory = wl.weights.errorHistory[1:]
	}
}

// GetImprovementHistory returns error history for improvement visualization
func (wl *WeightLearner) GetImprovementHistory() []ErrorHistoryEntry {
	wl.weights.mu.RLock()
	defer wl.weights.mu.RUnlock()

	result := make([]ErrorHistoryEntry, len(wl.weights.errorHistory))
	copy(result, wl.weights.errorHistory)
	return result
}

// GetImprovementStats calculates improvement statistics over time
func (wl *WeightLearner) GetImprovementStats() map[string]interface{} {
	wl.weights.mu.RLock()
	defer wl.weights.mu.RUnlock()

	history := wl.weights.errorHistory
	if len(history) < 2 {
		return map[string]interface{}{
			"samples":           len(history),
			"improvement_pct":   0.0,
			"initial_error_m":   0.0,
			"current_error_m":   0.0,
			"learning_duration": "0s",
			"trend":             "insufficient_data",
		}
	}

	initial := history[0]
	current := history[len(history)-1]

	// Calculate improvement percentage
	improvement := 0.0
	if initial.AvgErrorM > 0 {
		improvement = ((initial.AvgErrorM - current.AvgErrorM) / initial.AvgErrorM) * 100
	}

	// Determine trend
	trend := "stable"
	if improvement > 5 {
		trend = "improving"
	} else if improvement < -5 {
		trend = "degrading"
	}

	duration := current.Timestamp.Sub(initial.Timestamp)

	return map[string]interface{}{
		"samples":             len(history),
		"improvement_pct":     improvement,
		"initial_error_m":     initial.AvgErrorM,
		"current_error_m":     current.AvgErrorM,
		"initial_observations": initial.Observations,
		"current_observations": current.Observations,
		"learning_duration":   duration.String(),
		"trend":               trend,
		"first_sample":        initial.Timestamp.Format(time.RFC3339),
		"last_sample":         current.Timestamp.Format(time.RFC3339),
	}
}

// ContinuousWeightAdjuster provides real-time weight adjustment
type ContinuousWeightAdjuster struct {
	mu sync.RWMutex

	learner     *WeightLearner
	groundTruth GroundTruthSource

	// Adjustment interval
	interval time.Duration

	// Running flag
	running bool

	// Stop channel
	stopCh chan struct{}
}

// NewContinuousWeightAdjuster creates a continuous weight adjuster
func NewContinuousWeightAdjuster(learner *WeightLearner, interval time.Duration) *ContinuousWeightAdjuster {
	if interval <= 0 {
		interval = 10 * time.Second
	}

	return &ContinuousWeightAdjuster{
		learner:  learner,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start begins continuous weight adjustment
func (cwa *ContinuousWeightAdjuster) Start() {
	cwa.mu.Lock()
	if cwa.running {
		cwa.mu.Unlock()
		return
	}
	cwa.running = true
	cwa.mu.Unlock()

	ticker := time.NewTicker(cwa.interval)
	defer ticker.Stop()

	log.Printf("[INFO] Continuous weight adjuster started (interval: %v)", cwa.interval)

	for {
		select {
		case <-cwa.stopCh:
			log.Printf("[INFO] Continuous weight adjuster stopped")
			return
		case <-ticker.C:
			if err := cwa.learner.ProcessLearning(); err != nil {
				log.Printf("[WARN] Weight adjustment failed: %v", err)
			}
			// Record error snapshot for improvement tracking
			cwa.learner.RecordErrorSnapshot()
		}
	}
}

// Stop stops continuous weight adjustment
func (cwa *ContinuousWeightAdjuster) Stop() {
	cwa.mu.Lock()
	defer cwa.mu.Unlock()

	if cwa.running {
		cwa.running = false
		close(cwa.stopCh)
	}
}
