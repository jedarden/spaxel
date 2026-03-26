package signal

import (
	"sync"
	"time"
)

// LinkProcessor manages signal processing for a single link
type LinkProcessor struct {
	mu             sync.RWMutex
	baseline       *BaselineState
	motionDetector *MotionDetector
	nSub           int
	alpha          float64 // EMA alpha for baseline updates
	linkID         string
}

// NewLinkProcessor creates a new link processor
func NewLinkProcessor(linkID string, nSub int, alpha float64) *LinkProcessor {
	return &LinkProcessor{
		baseline:       NewBaselineState(nSub),
		motionDetector: NewMotionDetector(nSub),
		nSub:           nSub,
		alpha:          alpha,
		linkID:         linkID,
	}
}

// ProcessResult holds the result of processing a CSI frame
type ProcessResult struct {
	Processed       *ProcessedCSI
	Features        *MotionFeatures
	BaselineUpdated bool
	LinkID          string
	RecvTime        time.Time
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

	// Get current baseline
	baseline := lp.baseline.Get()

	// Extract motion features
	features := lp.motionDetector.Process(processed, baseline)

	// Update baseline (motion-gated)
	baselineUpdated := lp.baseline.Update(
		processed.Amplitude,
		features.SmoothDeltaRMS,
		lp.alpha,
	)

	return &ProcessResult{
		Processed:       processed,
		Features:        features,
		BaselineUpdated: baselineUpdated,
		LinkID:          lp.linkID,
		RecvTime:        recvTime,
	}, nil
}

// GetBaseline returns the current baseline
func (lp *LinkProcessor) GetBaseline() *BaselineState {
	return lp.baseline
}

// GetMotionDetector returns the motion detector
func (lp *LinkProcessor) GetMotionDetector() *MotionDetector {
	return lp.motionDetector
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

// Reset resets the link processor state
func (lp *LinkProcessor) Reset() {
	lp.mu.Lock()
	defer lp.mu.Unlock()
	lp.baseline.Reset()
	lp.motionDetector.Reset()
}

// ProcessorManager manages LinkProcessors for all links
type ProcessorManager struct {
	mu          sync.RWMutex
	processors  map[string]*LinkProcessor
	nSub        int
	alpha       float64
	fusionRate  float64 // Hz
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

// GetAllMotionStates returns motion states for all links
type LinkMotionState struct {
	LinkID         string
	MotionDetected bool
	SmoothDeltaRMS float64
	BaselineConf   float64
}

// GetAllMotionStates returns motion states for all links
func (pm *ProcessorManager) GetAllMotionStates() []LinkMotionState {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	states := make([]LinkMotionState, 0, len(pm.processors))
	for linkID, processor := range pm.processors {
		states = append(states, LinkMotionState{
			LinkID:         linkID,
			MotionDetected: processor.IsMotionDetected(),
			SmoothDeltaRMS: processor.GetSmoothDeltaRMS(),
			BaselineConf:   processor.GetBaseline().GetConfidence(),
		})
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

// RestoreBaseline restores a baseline from a snapshot
func (pm *ProcessorManager) RestoreBaseline(linkID string, snapshot *BaselineSnapshot) {
	processor := pm.GetOrCreateProcessor(linkID)
	processor.GetBaseline().RestoreFromSnapshot(snapshot.Values, snapshot.SampleTime)
	processor.GetBaseline().mu.Lock()
	processor.GetBaseline().Confidence = snapshot.Confidence
	processor.GetBaseline().mu.Unlock()
}
