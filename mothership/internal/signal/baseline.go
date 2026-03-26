package signal

import (
	"sync"
	"time"
)

// Baseline configuration constants
const (
	DefaultBaselineTimeConstant = 30.0       // seconds
	DefaultMotionThreshold      = 0.05       // deltaRMS threshold for motion gating
	DefaultAlpha                = 0.0033     // dt / (tau + dt) for dt=0.1s, tau=30s
	BaselineConfidenceMin       = 0.3        // Minimum confidence for stale baselines
	BaselineStaleDays           = 7          // Days before baseline is considered stale
)

// BaselineState holds the EMA baseline for a single link
type BaselineState struct {
	mu           sync.RWMutex
	Values       []float64 // Per-subcarrier baseline amplitude
	Initialized  bool      // True if baseline has been set
	SampleCount  int       // Number of samples used to train baseline
	LastUpdate   time.Time // Time of last baseline update
	Confidence   float64   // 0-1 confidence in the baseline
}

// NewBaselineState creates a new baseline state
func NewBaselineState(nSub int) *BaselineState {
	return &BaselineState{
		Values:      make([]float64, nSub),
		Initialized: false,
		Confidence:  0.0,
	}
}

// Initialize sets the baseline to the first amplitude sample
func (b *BaselineState) Initialize(amplitude []float64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	copy(b.Values, amplitude)
	b.Initialized = true
	b.SampleCount = 1
	b.LastUpdate = time.Now()
	b.Confidence = 1.0 // Fresh initialization = full confidence
}

// Update performs EMA update with motion gating
// Only updates if smoothDeltaRMS is below the motion threshold
// alpha = dt / (tau + dt), typically ~0.0033 for dt=0.1s, tau=30s
func (b *BaselineState) Update(amplitude []float64, smoothDeltaRMS float64, alpha float64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Motion-gated update: only update when room is still
	if smoothDeltaRMS >= DefaultMotionThreshold {
		return false
	}

	// Update EMA for each subcarrier
	for k := 0; k < len(b.Values) && k < len(amplitude); k++ {
		b.Values[k] = alpha*amplitude[k] + (1-alpha)*b.Values[k]
	}

	b.SampleCount++
	b.LastUpdate = time.Now()

	// Increase confidence as we accumulate quiet-room samples
	// Confidence asymptotically approaches 1.0
	if b.Confidence < 1.0 {
		b.Confidence = b.Confidence + (1.0-b.Confidence)*0.01
		if b.Confidence > 1.0 {
			b.Confidence = 1.0
		}
	}

	return true
}

// Get returns a copy of the current baseline values
func (b *BaselineState) Get() []float64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]float64, len(b.Values))
	copy(result, b.Values)
	return result
}

// IsInitialized returns whether the baseline has been initialized
func (b *BaselineState) IsInitialized() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.Initialized
}

// GetConfidence returns the current baseline confidence
func (b *BaselineState) GetConfidence() float64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.Confidence
}

// RestoreFromSnapshot restores baseline from a persisted snapshot
// Sets confidence to minimum if snapshot is stale (>7 days old)
func (b *BaselineState) RestoreFromSnapshot(values []float64, snapshotTime time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()

	copy(b.Values, values)
	b.Initialized = true
	b.LastUpdate = snapshotTime

	// Check staleness
	age := time.Since(snapshotTime)
	if age > BaselineStaleDays*24*time.Hour {
		b.Confidence = BaselineConfidenceMin
	} else {
		// Confidence degrades with age
		ageFraction := float64(age) / float64(BaselineStaleDays*24*time.Hour)
		b.Confidence = 1.0 - ageFraction*0.7 // Range from 0.3 to 1.0
	}
}

// Snapshot returns a snapshot of the baseline for persistence
type BaselineSnapshot struct {
	Values     []float64
	SampleTime time.Time
	Confidence float64
}

// GetSnapshot returns a snapshot for persistence
func (b *BaselineState) GetSnapshot() *BaselineSnapshot {
	b.mu.RLock()
	defer b.mu.RUnlock()

	values := make([]float64, len(b.Values))
	copy(values, b.Values)

	return &BaselineSnapshot{
		Values:     values,
		SampleTime: b.LastUpdate,
		Confidence: b.Confidence,
	}
}

// Reset clears the baseline (for manual re-baseline or node position change)
func (b *BaselineState) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i := range b.Values {
		b.Values[i] = 0
	}
	b.Initialized = false
	b.SampleCount = 0
	b.Confidence = 0.0
}

// BaselineManager manages baselines for all links
type BaselineManager struct {
	mu     sync.RWMutex
	baselines map[string]*BaselineState // keyed by linkID (nodeMAC:peerMAC)
	nSub     int
}

// NewBaselineManager creates a new baseline manager
func NewBaselineManager(nSub int) *BaselineManager {
	return &BaselineManager{
		baselines: make(map[string]*BaselineState),
		nSub:      nSub,
	}
}

// GetOrCreate returns the baseline state for a link, creating if needed
func (bm *BaselineManager) GetOrCreate(linkID string) *BaselineState {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if bs, exists := bm.baselines[linkID]; exists {
		return bs
	}

	bs := NewBaselineState(bm.nSub)
	bm.baselines[linkID] = bs
	return bs
}

// Get returns the baseline state for a link, or nil if not exists
func (bm *BaselineManager) Get(linkID string) *BaselineState {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	return bm.baselines[linkID]
}

// GetAllSnapshots returns snapshots of all baselines for persistence
func (bm *BaselineManager) GetAllSnapshots() map[string]*BaselineSnapshot {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	result := make(map[string]*BaselineSnapshot)
	for linkID, state := range bm.baselines {
		if state.IsInitialized() {
			result[linkID] = state.GetSnapshot()
		}
	}
	return result
}

// Remove removes a baseline for a link
func (bm *BaselineManager) Remove(linkID string) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	delete(bm.baselines, linkID)
}

// LinkCount returns the number of tracked links
func (bm *BaselineManager) LinkCount() int {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	return len(bm.baselines)
}
