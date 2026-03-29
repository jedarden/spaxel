package signal

import (
	"sync"
	"time"
)

// LinkProcessor manages signal processing for a single link
type LinkProcessor struct {
	mu             sync.RWMutex
	baseline       *BaselineState
	diurnal        *DiurnalBaseline
	motionDetector *MotionDetector
	breathing      *BreathingDetector
	health         *LinkHealth
	nSub           int
	alpha          float64 // EMA alpha for baseline updates
	linkID         string
}

// NewLinkProcessor creates a new link processor
func NewLinkProcessor(linkID string, nSub int, alpha float64) *LinkProcessor {
	return &LinkProcessor{
		baseline:       NewBaselineState(nSub),
		diurnal:        NewDiurnalBaseline(linkID, nSub),
		motionDetector: NewMotionDetector(nSub),
		breathing:      NewBreathingDetector(nSub),
		health:         NewLinkHealth(linkID, nSub),
		nSub:           nSub,
		alpha:          alpha,
		linkID:         linkID,
	}
}

// ProcessResult holds the result of processing a CSI frame
type ProcessResult struct {
	Processed         *ProcessedCSI
	Features          *MotionFeatures
	BreathingFeatures *BreathingFeatures
	BaselineUpdated   bool
	LinkID            string
	RecvTime          time.Time
	ActiveBaseline    []float64 // The baseline used (may be diurnal-blended)
	DiurnalWeight     float64   // Weight of diurnal in baseline (0-1)
	DiurnalReady      bool      // True if diurnal slot has enough samples
}

// Process processes a raw CSI frame and returns processed data with features
func (lp *LinkProcessor) Process(payload []int8, rssiDBm int8, nSub int, recvTime time.Time) (*ProcessResult, error) {
	lp.mu.Lock()
	defer lp.mu.Unlock()

	// Phase sanitization
	processed, err := PhaseSanitize(payload, rssiDBm, nSub)
	if err != nil {
		return nil, err
	}

	// Initialize baseline if needed
	if !lp.baseline.IsInitialized() {
		lp.baseline.Initialize(processed.Amplitude)
	}

	// Get EMA baseline
	emaBaseline := lp.baseline.Get()

	// Get diurnal-aware active baseline (crossfade between EMA and hourly slot)
	activeBaseline, diurnalWeight, diurnalReady := lp.diurnal.GetActiveBaseline(emaBaseline)

	// Extract motion features using active baseline
	features := lp.motionDetector.Process(processed, activeBaseline)

	// Update EMA baseline (motion-gated)
	baselineUpdated := lp.baseline.Update(
		processed.Amplitude,
		features.SmoothDeltaRMS,
		lp.alpha,
	)

	// Update diurnal baseline during quiet periods
	if features.SmoothDeltaRMS < DefaultMotionThreshold {
		lp.diurnal.Update(processed.Amplitude)
	}

	// Update health tracking
	lp.health.UpdateRSSI(rssiDBm)
	lp.health.UpdateTimestamp(recvTime)
	lp.health.UpdatePhaseVariance(features.PhaseVariance)
	if baselineUpdated {
		lp.health.UpdateBaseline(activeBaseline)
	}

	// Track motion/quiet deltaRMS for SNR estimation
	isMotion := features.MotionDetected
	lp.health.UpdateDeltaRMS(features.DeltaRMS, isMotion)

	// Breathing detection (only when room is still and health is good)
	var breathingFeatures *BreathingFeatures
	healthScore := lp.health.GetAmbientConfidence()
	if features.SmoothDeltaRMS < BreathingMotionThreshold {
		breathingFeatures = lp.breathing.ProcessWithHealth(processed.ResidualPhase, features.SmoothDeltaRMS, healthScore)
	} else {
		breathingFeatures = &BreathingFeatures{Computed: false}
	}

	return &ProcessResult{
		Processed:         processed,
		Features:          features,
		BreathingFeatures: breathingFeatures,
		BaselineUpdated:   baselineUpdated,
		LinkID:            lp.linkID,
		RecvTime:          recvTime,
		ActiveBaseline:    activeBaseline,
		DiurnalWeight:     diurnalWeight,
		DiurnalReady:      diurnalReady,
	}, nil
}

// GetBaseline returns the current baseline
func (lp *LinkProcessor) GetBaseline() *BaselineState {
	return lp.baseline
}

// GetDiurnal returns the diurnal baseline manager
func (lp *LinkProcessor) GetDiurnal() *DiurnalBaseline {
	return lp.diurnal
}

// GetMotionDetector returns the motion detector
func (lp *LinkProcessor) GetMotionDetector() *MotionDetector {
	return lp.motionDetector
}

// GetBreathing returns the breathing detector
func (lp *LinkProcessor) GetBreathing() *BreathingDetector {
	return lp.breathing
}

// GetHealth returns the link health tracker
func (lp *LinkProcessor) GetHealth() *LinkHealth {
	return lp.health
}

// IsMotionDetected returns whether motion is currently detected
func (lp *LinkProcessor) IsMotionDetected() bool {
	lp.mu.RLock()
	defer lp.mu.RUnlock()
	return lp.motionDetector.IsMotionDetected()
}

// GetSmoothDeltaRMS returns the current smoothed deltaRMS
func (lp *LinkProcessor) GetSmoothDeltaRMS() float64 {
	lp.mu.RLock()
	defer lp.mu.RUnlock()
	return lp.motionDetector.GetSmoothDeltaRMS()
}

// IsBreathingDetected returns whether a stationary person is detected
func (lp *LinkProcessor) IsBreathingDetected() bool {
	lp.mu.RLock()
	defer lp.mu.RUnlock()
	return lp.breathing.IsDetected()
}

// GetAmbientConfidence returns the link's health confidence score
func (lp *LinkProcessor) GetAmbientConfidence() float64 {
	lp.mu.RLock()
	defer lp.mu.RUnlock()
	return lp.health.GetAmbientConfidence()
}

// Reset resets the link processor state
func (lp *LinkProcessor) Reset() {
	lp.mu.Lock()
	defer lp.mu.Unlock()
	lp.baseline.Reset()
	lp.diurnal.Reset()
	lp.motionDetector.Reset()
	lp.breathing.Reset()
	lp.health.Reset()
}

// ProcessorManager manages LinkProcessors for all links
type ProcessorManager struct {
	mu         sync.RWMutex
	processors map[string]*LinkProcessor
	nSub       int
	alpha      float64
	fusionRate float64 // Hz
}

// ProcessorManagerConfig holds configuration for ProcessorManager
type ProcessorManagerConfig struct {
	NSub       int     // Number of subcarriers (typically 64)
	FusionRate float64 // Fusion loop rate in Hz (typically 10)
	Tau        float64 // Baseline time constant in seconds (typically 30)
}

// NewProcessorManager creates a new processor manager
func NewProcessorManager(cfg ProcessorManagerConfig) *ProcessorManager {
	// Calculate alpha: α = dt / (τ + dt)
	dt := 1.0 / cfg.FusionRate
	alpha := dt / (cfg.Tau + dt)

	return &ProcessorManager{
		processors: make(map[string]*LinkProcessor),
		nSub:       cfg.NSub,
		alpha:      alpha,
		fusionRate: cfg.FusionRate,
	}
}

// Process processes a CSI frame for a link
func (pm *ProcessorManager) Process(linkID string, payload []int8, rssiDBm int8, nSub int, recvTime time.Time) (*ProcessResult, error) {
	pm.mu.Lock()
	processor, exists := pm.processors[linkID]
	if !exists {
		processor = NewLinkProcessor(linkID, pm.nSub, pm.alpha)
		pm.processors[linkID] = processor
	}
	pm.mu.Unlock()

	return processor.Process(payload, rssiDBm, nSub, recvTime)
}

// GetProcessor returns the processor for a link, or nil if not exists
func (pm *ProcessorManager) GetProcessor(linkID string) *LinkProcessor {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.processors[linkID]
}

// GetOrCreateProcessor returns the processor for a link, creating if needed
func (pm *ProcessorManager) GetOrCreateProcessor(linkID string) *LinkProcessor {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if processor, exists := pm.processors[linkID]; exists {
		return processor
	}

	processor := NewLinkProcessor(linkID, pm.nSub, pm.alpha)
	pm.processors[linkID] = processor
	return processor
}

// RemoveProcessor removes a processor for a link
func (pm *ProcessorManager) RemoveProcessor(linkID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.processors, linkID)
}

// LinkMotionState represents the motion state of a single link
type LinkMotionState struct {
	LinkID            string
	MotionDetected    bool
	SmoothDeltaRMS    float64
	BaselineConf      float64
	BreathingDetected bool
	BreathingRate     float64
	AmbientConfidence float64
	DiurnalConfidence float64
}

// GetAllMotionStates returns motion states for all links
func (pm *ProcessorManager) GetAllMotionStates() []LinkMotionState {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	states := make([]LinkMotionState, 0, len(pm.processors))
	for linkID, processor := range pm.processors {
		processor.mu.RLock()
		state := LinkMotionState{
			LinkID:            linkID,
			MotionDetected:    processor.motionDetector.IsMotionDetected(),
			SmoothDeltaRMS:    processor.motionDetector.GetSmoothDeltaRMS(),
			BaselineConf:      processor.baseline.GetConfidence(),
			BreathingDetected: processor.breathing.IsDetected(),
			BreathingRate:     processor.breathing.GetBreathingRate(),
			AmbientConfidence: processor.health.GetAmbientConfidence(),
			DiurnalConfidence: processor.diurnal.GetOverallConfidence(),
		}
		processor.mu.RUnlock()
		states = append(states, state)
	}
	return states
}

// ActiveLinks returns the number of links with motion detected
func (pm *ProcessorManager) ActiveLinks() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	count := 0
	for _, processor := range pm.processors {
		if processor.IsMotionDetected() {
			count++
		}
	}
	return count
}

// LinkCount returns the total number of tracked links
func (pm *ProcessorManager) LinkCount() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.processors)
}

// GetAllLinkIDs returns all tracked link IDs
func (pm *ProcessorManager) GetAllLinkIDs() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	ids := make([]string, 0, len(pm.processors))
	for linkID := range pm.processors {
		ids = append(ids, linkID)
	}
	return ids
}

// GetAllBaselines returns snapshots of all baselines for persistence
func (pm *ProcessorManager) GetAllBaselines() map[string]*BaselineSnapshot {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	result := make(map[string]*BaselineSnapshot)
	for linkID, processor := range pm.processors {
		if snapshot := processor.GetBaseline().GetSnapshot(); snapshot != nil {
			result[linkID] = snapshot
		}
	}
	return result
}

// GetAllDiurnalSnapshots returns diurnal snapshots for all links
func (pm *ProcessorManager) GetAllDiurnalSnapshots() map[string]*DiurnalSnapshot {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	result := make(map[string]*DiurnalSnapshot)
	for linkID, processor := range pm.processors {
		result[linkID] = processor.GetDiurnal().GetSnapshot()
	}
	return result
}

// RestoreBaseline restores a baseline from a snapshot
func (pm *ProcessorManager) RestoreBaseline(linkID string, snapshot *BaselineSnapshot) {
	processor := pm.GetOrCreateProcessor(linkID)
	processor.GetBaseline().RestoreFromSnapshot(snapshot.Values, snapshot.SampleTime)
	processor.GetBaseline().mu.Lock()
	processor.GetBaseline().Confidence = snapshot.Confidence
	processor.GetBaseline().mu.Unlock()
}

// RestoreDiurnal restores a diurnal baseline from a snapshot
func (pm *ProcessorManager) RestoreDiurnal(linkID string, snapshot *DiurnalSnapshot) {
	processor := pm.GetOrCreateProcessor(linkID)
	processor.GetDiurnal().RestoreFromSnapshot(snapshot)
}

// ComputeAllHealth triggers health computation for all links
func (pm *ProcessorManager) ComputeAllHealth() {
	pm.mu.RLock()
	processors := make([]*LinkProcessor, 0, len(pm.processors))
	for _, p := range pm.processors {
		processors = append(processors, p)
	}
	pm.mu.RUnlock()

	for _, p := range processors {
		p.health.ComputeHealth()
	}
}

// GetSystemHealth returns overall system health score
func (pm *ProcessorManager) GetSystemHealth() float64 {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if len(pm.processors) == 0 {
		return 0
	}

	var sum float64
	for _, processor := range pm.processors {
		sum += processor.health.GetAmbientConfidence()
	}

	return sum / float64(len(pm.processors))
}

// GetWorstLink returns the link with lowest health score
func (pm *ProcessorManager) GetWorstLink() (linkID string, score float64) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	worstScore := 2.0 // Start above 1.0
	worstID := ""

	for linkID, processor := range pm.processors {
		conf := processor.health.GetAmbientConfidence()
		if conf < worstScore {
			worstScore = conf
			worstID = linkID
		}
	}

	return worstID, worstScore
}

// GetStationaryPersonCount returns the number of links detecting stationary persons
func (pm *ProcessorManager) GetStationaryPersonCount() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	count := 0
	for _, processor := range pm.processors {
		if processor.breathing.IsDetected() {
			count++
		}
	}
	return count
}

// DiurnalLearningStatus represents the diurnal baseline learning state for a link
type DiurnalLearningStatus struct {
	LinkID             string    `json:"link_id"`
	IsLearning         bool      `json:"is_learning"`
	DaysRemaining      float64   `json:"days_remaining"`
	Progress           float64   `json:"progress"` // 0-100 percentage
	IsReady            bool      `json:"is_ready"`
	SlotsReady         int       `json:"slots_ready"` // Number of slots with >= 100 samples
	DiurnalConfidence  float64   `json:"diurnal_confidence"`
	CreatedAt          time.Time `json:"created_at"`
}

// GetDiurnalLearningStatus returns diurnal learning status for all links
func (pm *ProcessorManager) GetDiurnalLearningStatus() []DiurnalLearningStatus {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	statuses := make([]DiurnalLearningStatus, 0, len(pm.processors))
	for linkID, processor := range pm.processors {
		diurnal := processor.diurnal
		if diurnal == nil {
			continue
		}

		status := DiurnalLearningStatus{
			LinkID:            linkID,
			IsLearning:        diurnal.IsLearning(),
			Progress:          diurnal.GetLearningProgress(),
			IsReady:           diurnal.IsReady(),
			DiurnalConfidence: diurnal.GetOverallConfidence(),
			CreatedAt:         diurnal.GetCreatedAt(),
		}

		// Calculate days remaining
		elapsed := time.Since(diurnal.GetCreatedAt())
		learningPeriod := DiurnalLearningDays * 24 * time.Hour
		if elapsed < learningPeriod {
			status.DaysRemaining = float64(learningPeriod-elapsed) / float64(24*time.Hour)
		}

		// Count ready slots
		for h := 0; h < DiurnalSlots; h++ {
			slot := diurnal.GetSlot(h)
			if slot != nil && slot.SampleCount >= DiurnalMinSamples {
				status.SlotsReady++
			}
		}

		statuses = append(statuses, status)
	}
	return statuses
}

// GetDiurnalCompositeConfidence returns the composite confidence for a link including diurnal progress
func (pm *ProcessorManager) GetDiurnalCompositeConfidence(linkID string, packetRateRatio float64) float64 {
	pm.mu.RLock()
	processor, exists := pm.processors[linkID]
	pm.mu.RUnlock()

	if !exists || processor == nil || processor.diurnal == nil {
		return 0.0
	}

	return processor.diurnal.CompositeConfidence(packetRateRatio)
}

// CheckDiurnalReadinessTransitions checks for links that have newly become ready
// Returns a list of link IDs that transitioned from not-ready to ready
func (pm *ProcessorManager) CheckDiurnalReadinessTransitions(previouslyReady map[string]bool) []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var newlyReady []string
	for linkID, processor := range pm.processors {
		if processor.diurnal == nil {
			continue
		}

		isReady := processor.diurnal.IsReady()
		wasReady := previouslyReady[linkID]

		if isReady && !wasReady {
			newlyReady = append(newlyReady, linkID)
		}
	}
	return newlyReady
}

// GetLinkCompositeConfidence returns composite confidence for a specific link
func (lp *LinkProcessor) GetLinkCompositeConfidence(packetRateRatio float64) float64 {
	lp.mu.RLock()
	defer lp.mu.RUnlock()

	if lp.diurnal == nil {
		return lp.baseline.GetConfidence()
	}

	return lp.diurnal.CompositeConfidence(packetRateRatio)
}
