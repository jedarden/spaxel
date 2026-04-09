// Package replay implements CSI replay with time-travel debugging.
//
// Pipeline provides a separate signal processing pipeline for replay that
// can have different parameters than the live pipeline.
package replay

import (
	"log"
	"sync"
	"time"

	"github.com/spaxel/mothership/internal/ingestion"
	sigproc "github.com/spaxel/mothership/internal/signal"
)

// Pipeline is a replay-specific signal processing pipeline with tunable parameters.
type Pipeline struct {
	mu     sync.RWMutex
	params *TunableParams

	// Signal processor (shared or cloned from live)
	processor *sigproc.ProcessorManager

	// Per-link baseline states for replay
	baselineStates map[string]*sigproc.BaselineState

	// Motion state cache
	motionStates map[string]*MotionState
}

// MotionState represents motion detection state for a link.
type MotionState struct {
	LinkID            string
	SmoothDeltaRMS    float64
	MotionDetected    bool
	AmbientConfidence float64
	BaselineConf      float64
	LastUpdate        time.Time
}

// NewPipeline creates a new replay pipeline.
func NewPipeline() *Pipeline {
	return &Pipeline{
		params:         &TunableParams{},
		baselineStates: make(map[string]*sigproc.BaselineState),
		motionStates:   make(map[string]*MotionState),
	}
}

// SetProcessorManager sets the signal processor for the pipeline.
func (p *Pipeline) SetProcessorManager(pm *sigproc.ProcessorManager) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.processor = pm
}

// SetParams updates the tunable parameters.
func (p *Pipeline) SetParams(params *TunableParams) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.params = params

	// Reset baseline states when parameters change
	p.baselineStates = make(map[string]*sigproc.BaselineState)
}

// GetParams returns the current parameters.
func (p *Pipeline) GetParams() *TunableParams {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.params
}

// Reset resets the pipeline state (e.g., after seeking).
func (p *Pipeline) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.baselineStates = make(map[string]*sigproc.BaselineState)
	p.motionStates = make(map[string]*MotionState)
}

// ProcessFrame processes a single CSI frame through the replay pipeline.
func (p *Pipeline) ProcessFrame(parsed *ingestion.ParsedFrame, recvTime time.Time) *sigproc.ProcessingResult {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.processor == nil {
		return nil
	}

	// Get link ID
	linkID := parsed.LinkID()

	// Get or create baseline state for this link
	baseline, exists := p.baselineStates[linkID]
	if !exists {
		baseline = &sigproc.BaselineState{}
		p.baselineStates[linkID] = baseline
	}

	// Apply replay parameters if set
	result := p.processWithParams(linkID, parsed, baseline, recvTime)

	// Update motion state cache
	if result != nil {
		p.motionStates[linkID] = &MotionState{
			LinkID:            linkID,
			SmoothDeltaRMS:    result.SmoothDeltaRMS,
			MotionDetected:    result.MotionDetected,
			AmbientConfidence: result.AmbientConfidence,
			BaselineConf:      result.BaselineConfidence(),
			LastUpdate:        recvTime,
		}
	}

	return result
}

// processWithParams processes a frame with replay-specific parameters.
func (p *Pipeline) processWithParams(linkID string, parsed *ingestion.ParsedFrame,
	baseline *sigproc.BaselineState, recvTime time.Time) *sigproc.ProcessingResult {

	// Use default processor for now - parameters are applied via baseline
	result, err := p.processor.ProcessWithBaseline(linkID, parsed.Payload,
		parsed.RSSI, int(parsed.NSub), recvTime, baseline)

	if err != nil {
		log.Printf("[DEBUG] Replay pipeline error for %s: %v", linkID, err)
		return nil
	}

	// Apply replay parameter overrides
	if p.params != nil {
		// Override deltaRMS threshold if set
		if p.params.DeltaRMSThreshold != nil {
			// Re-check motion detection with new threshold
			result.MotionDetected = result.SmoothDeltaRMS > *p.params.DeltaRMSThreshold
		}
	}

	return result
}

// GetAllMotionStates returns all cached motion states.
func (p *Pipeline) GetAllMotionStates() []*MotionState {
	p.mu.RLock()
	defer p.mu.RUnlock()

	states := make([]*MotionState, 0, len(p.motionStates))
	for _, state := range p.motionStates {
		states = append(states, state)
	}
	return states
}

// HasMotionData returns true if any motion data is available.
func (p *Pipeline) HasMotionData() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.motionStates) > 0
}

// GetBaselineState returns the baseline state for a link.
func (p *Pipeline) GetBaselineState(linkID string) (*sigproc.BaselineState, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	baseline, exists := p.baselineStates[linkID]
	return baseline, exists
}

// SetBaselineState sets the baseline state for a link.
func (p *Pipeline) SetBaselineState(linkID string, baseline *sigproc.BaselineState) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.baselineStates[linkID] = baseline
}

// Clone creates a deep copy of the pipeline state.
func (p *Pipeline) Clone() *Pipeline {
	p.mu.RLock()
	defer p.mu.RUnlock()

	clone := &Pipeline{
		params:         p.params,
		processor:     p.processor,
		baselineStates: make(map[string]*sigproc.BaselineState),
		motionStates:   make(map[string]*MotionState),
	}

	// Clone baseline states
	for k, v := range p.baselineStates {
		clone.baselineStates[k] = v.Clone()
	}

	// Clone motion states
	for k, v := range p.motionStates {
		clone.motionStates[k] = &MotionState{
			LinkID:            v.LinkID,
			SmoothDeltaRMS:    v.SmoothDeltaRMS,
			MotionDetected:    v.MotionDetected,
			AmbientConfidence: v.AmbientConfidence,
			BaselineConf:      v.BaselineConf,
			LastUpdate:        v.LastUpdate,
		}
	}

	return clone
}

// ApplyLiveBaselines copies baseline states from the live pipeline.
func (p *Pipeline) ApplyLiveBaselines(liveBaselines map[string]*sigproc.BaselineState) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for linkID, baseline := range liveBaselines {
		p.baselineStates[linkID] = baseline.Clone()
	}
}

// GetBaselineStates returns a copy of all baseline states.
func (p *Pipeline) GetBaselineStates() map[string]*sigproc.BaselineState {
	p.mu.RLock()
	defer p.mu.RUnlock()

	states := make(map[string]*sigproc.BaselineState, len(p.baselineStates))
	for k, v := range p.baselineStates {
		states[k] = v.Clone()
	}
	return states
}
