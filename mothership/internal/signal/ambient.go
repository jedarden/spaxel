// Package signal implements ambient confidence scoring for link health monitoring
package signal

import (
	"math"
	"sync"
	"time"
)

// Ambient confidence constants
const (
	HealthWindow         = 60  // Seconds of history for health metrics
	HealthSampleRate     = 20  // Expected samples per second at active rate (default)
	HealthHistorySize    = HealthWindow * HealthSampleRate
	PhaseStabilityWindow = 100 // Samples for phase stability calculation
	DriftWindow          = 200 // Samples for drift calculation
	NoiseFloor           = -95 // dBm - assumed noise floor for SNR calculation

	// Health score weights (per specification)
	SNRWeight           = 0.40
	PhaseStabilityWeight = 0.30
	PacketRateWeight    = 0.20
	BaselineDriftWeight = 0.10
)

// LinkHealth holds per-link health metrics
type LinkHealth struct {
	mu sync.RWMutex

	// Signal quality metrics (raw values)
	SNR             float64   // Signal-to-noise ratio estimate (raw)
	PhaseStability  float64   // Phase variance (radians²)
	PacketRate      float64   // Actual packet rate (Hz)
	DriftRate       float64   // Baseline drift rate (normalized 0-1)
	PhaseVariance   float64   // Current phase variance

	// Sub-scores (0-1 range, for dashboard breakdown)
	SNRScore          float64
	PhaseStabilityScore float64
	PacketRateScore   float64
	DriftScore        float64

	// History buffers
	rssiHistory     []int8
	rssiWriteIdx    int
	rssiCount       int

	phaseVarHistory  []float64
	phaseVarWriteIdx int
	phaseVarCount    int

	timestampHistory  []time.Time
	timestampWriteIdx int
	timestampCount    int

	baselineHistory   [][]float64 // Snapshots for drift calculation
	baselineWriteIdx  int
	baselineCount     int

	// Motion tracking for SNR estimation
	deltaRMSHistory   []float64 // Motion-period deltaRMS values
	deltaRMSWriteIdx  int
	deltaRMSCount     int
	quietDeltaRMSHistory []float64 // Quiet-period deltaRMS for noise estimation
	quietDeltaRMSWriteIdx int
	quietDeltaRMSCount int

	// Composite score
	ambientConfidence float64
	lastUpdate        time.Time

	// Tracking state
	nSub            int
	linkID          string
	configuredRate  float64 // Configured packet rate (Hz)
}

// NewLinkHealth creates a new link health monitor
func NewLinkHealth(linkID string, nSub int) *LinkHealth {
	return &LinkHealth{
		rssiHistory:          make([]int8, HealthHistorySize),
		phaseVarHistory:      make([]float64, PhaseStabilityWindow),
		timestampHistory:     make([]time.Time, HealthHistorySize),
		baselineHistory:      make([][]float64, DriftWindow),
		deltaRMSHistory:      make([]float64, 600), // 30s at 20Hz for motion periods
		quietDeltaRMSHistory: make([]float64, 1200), // 60s at 20Hz for quiet periods
		nSub:                 nSub,
		linkID:               linkID,
		PhaseStability:       1.0, // Assume unstable until proven otherwise
		configuredRate:       float64(HealthSampleRate), // Default to 20 Hz
	}
}

// SetConfiguredRate sets the expected packet rate for the link
func (lh *LinkHealth) SetConfiguredRate(rateHz float64) {
	lh.mu.Lock()
	defer lh.mu.Unlock()
	lh.configuredRate = rateHz
}

// UpdateRSSI adds a new RSSI sample
func (lh *LinkHealth) UpdateRSSI(rssi int8) {
	lh.mu.Lock()
	defer lh.mu.Unlock()

	lh.rssiHistory[lh.rssiWriteIdx] = rssi
	lh.rssiWriteIdx = (lh.rssiWriteIdx + 1) % HealthHistorySize
	if lh.rssiCount < HealthHistorySize {
		lh.rssiCount++
	}
}

// UpdateDeltaRMS adds a deltaRMS sample for motion detection
// If isMotion is true, we the sample goes to deltaRMSHistory (motion signal)
// Otherwise it it goes to quietDeltaRMSHistory (noise floor estimation)
func (lh *LinkHealth) UpdateDeltaRMS(deltaRMS float64, isMotion bool) {
	lh.mu.Lock()
	defer lh.mu.Unlock()

	if isMotion {
		// Motion period - track signal level
		lh.deltaRMSHistory[lh.deltaRMSWriteIdx] = deltaRMS
		lh.deltaRMSWriteIdx = (lh.deltaRMSWriteIdx + 1) % cap(lh.deltaRMSHistory)
		if lh.deltaRMSCount < cap(lh.deltaRMSHistory) {
			lh.deltaRMSCount++
		}
	} else {
		// Quiet period - track noise floor
		lh.quietDeltaRMSHistory[lh.quietDeltaRMSWriteIdx] = deltaRMS
		lh.quietDeltaRMSWriteIdx = (lh.quietDeltaRMSWriteIdx + 1) % cap(lh.quietDeltaRMSHistory)
		if lh.quietDeltaRMSCount < cap(lh.quietDeltaRMSHistory) {
			lh.quietDeltaRMSCount++
		}
	}
}

// UpdateTimestamp records a packet arrival for rate calculation
func (lh *LinkHealth) UpdateTimestamp(t time.Time) {
	lh.mu.Lock()
	defer lh.mu.Unlock()

	lh.timestampHistory[lh.timestampWriteIdx] = t
	lh.timestampWriteIdx = (lh.timestampWriteIdx + 1) % HealthHistorySize
	if lh.timestampCount < HealthHistorySize {
		lh.timestampCount++
	}
}

// UpdatePhaseVariance adds a new phase variance sample
func (lh *LinkHealth) UpdatePhaseVariance(phaseVar float64) {
	lh.mu.Lock()
	defer lh.mu.Unlock()

	lh.PhaseVariance = phaseVar
	lh.phaseVarHistory[lh.phaseVarWriteIdx] = phaseVar
	lh.phaseVarWriteIdx = (lh.phaseVarWriteIdx + 1) % PhaseStabilityWindow
	if lh.phaseVarCount < PhaseStabilityWindow {
		lh.phaseVarCount++
	}
}

// UpdateBaseline adds a baseline snapshot for drift tracking
func (lh *LinkHealth) UpdateBaseline(baseline []float64) {
	lh.mu.Lock()
	defer lh.mu.Unlock()

	// Copy baseline
	snapshot := make([]float64, lh.nSub)
	copy(snapshot, baseline)

	lh.baselineHistory[lh.baselineWriteIdx] = snapshot
	lh.baselineWriteIdx = (lh.baselineWriteIdx + 1) % DriftWindow
	if lh.baselineCount < DriftWindow {
		lh.baselineCount++
	}
}

// ComputeHealth calculates the composite health score
func (lh *LinkHealth) ComputeHealth() {
	lh.mu.Lock()
	defer lh.mu.Unlock()

	// 1. Compute SNR estimate from motion/quiet deltaRMS ratio
	lh.SNR, lh.SNRScore = lh.computeSNR()

	// 2. Compute phase stability (Mean variance, lower is better)
	lh.PhaseStability, lh.PhaseStabilityScore = lh.computePhaseStability()

	// 3. Compute actual packet rate
	lh.PacketRate, lh.PacketRateScore = lh.computePacketRate()

	// 4. Compute baseline drift rate
	lh.DriftRate, lh.DriftScore = lh.computeDriftRate()

	// 5. Compute composite confidence score using specified weights
	lh.ambientConfidence = lh.computeCompositeScore()
	lh.lastUpdate = time.Now()
}

// computeSNR estimates SNR from motion-period vs quiet-period deltaRMS ratio
// Returns: (raw SNR ratio or RSSI-based fallback, normalized score 0-1)
// Uses log10 mapping: SNR=100 -> score=1.0, SNR=10 -> score=0.5
func (lh *LinkHealth) computeSNR() (float64, float64) {
	// Prefer motion/quiet deltaRMS ratio when we have enough data
	if lh.quietDeltaRMSCount >= 10 && lh.deltaRMSCount >= 5 {
		// Compute mean of motion-period deltaRMS (signal level)
		var signalSum float64
		for i := 0; i < lh.deltaRMSCount; i++ {
			signalSum += lh.deltaRMSHistory[i]
		}
		signalLevel := signalSum / float64(lh.deltaRMSCount)

		// Compute variance of quiet-period deltaRMS (noise floor)
		var quietSum float64
		var quietSumSq float64
		for i := 0; i < lh.quietDeltaRMSCount; i++ {
			v := lh.quietDeltaRMSHistory[i]
			quietSum += v
			quietSumSq += v * v
		}
		quietMean := quietSum / float64(lh.quietDeltaRMSCount)
		quietVariance := quietSumSq/float64(lh.quietDeltaRMSCount) - quietMean*quietMean

		// SNR ratio = signal / noise_stddev
		var snrRatio float64
		if quietVariance > 0 {
			snrRatio = signalLevel / math.Sqrt(quietVariance)
		} else {
			snrRatio = 1.0 // Avoid division by zero
		}

		// Map to 0-1 via log10: score = min(1.0, log10(SNR_ratio) / log10(100))
		// SNR=100:1 -> score=1.0, SNR=10:1 -> score=0.5, SNR=1:1 -> score=0
		var score float64
		if snrRatio <= 1.0 {
			score = 0.0
		} else {
			score = math.Log10(snrRatio) / math.Log10(100.0)
			if score > 1.0 {
				score = 1.0
			}
			if score < 0 {
				score = 0.0
			}
		}
		return snrRatio, score
	}

	// Fall back to RSSI-based estimate when motion/quiet data unavailable
	if lh.rssiCount == 0 {
		return 0.0, 0.5 // Unknown - assume moderate
	}

	// Compute mean RSSI
	var sum float64
	for i := 0; i < lh.rssiCount; i++ {
		sum += float64(lh.rssiHistory[i])
	}
	meanRSSI := sum / float64(lh.rssiCount)

	// SNR = RSSI - noise_floor (in dB)
	snr := meanRSSI - float64(NoiseFloor)

	// Normalize to 0-1 range
	// SNR of 40+ dB is excellent (1.0), SNR of 10 dB is poor (0.25)
	var score float64
	if snr < 10 {
		score = 0.1
	} else if snr > 40 {
		score = 1.0
	} else {
		score = (snr - 10) / 30
	}
	return snr, score
}

// computePhaseStability computes mean phase variance over the window
// Spec: score = max(0, 1 - phase_variance / 0.5)
// variance=0 -> score=1.0, variance=0.5 -> score=0.0
// Returns: (raw variance, normalized score 0-1 where 1 is most stable)
func (lh *LinkHealth) computePhaseStability() (float64, float64) {
	if lh.phaseVarCount == 0 {
		return 1.0, 0.5 // Unknown - assume moderate
	}

	var sum float64
	for i := 0; i < lh.phaseVarCount; i++ {
		sum += lh.phaseVarHistory[i]
	}
	meanVar := sum / float64(lh.phaseVarCount)

	// Score: per spec, score = max(0, 1 - phase_variance / 0.5)
	// variance=0 -> score=1.0, variance=0.5 -> score=0.0
	score := 1.0 - meanVar/0.5
	if score > 1.0 {
		score = 1.0
	}
	if score < 0 {
		score = 0.0
	}

	// Return raw variance (capped at 1.0 for display)
	if meanVar > 1.0 {
		return 1.0, score
	}
	return meanVar, score
}

// computePacketRate calculates the actual packet reception rate
// Returns: (rate in Hz, normalized score 0-1)
func (lh *LinkHealth) computePacketRate() (float64, float64) {
	if lh.timestampCount < 2 {
		return 0, 0.25 // No data - assume poor
	}

	// Count packets in last window
	now := time.Now()
	windowStart := now.Add(-HealthWindow * time.Second)
	count := 0
	var firstTime, lastTime time.Time

	for i := 0; i < lh.timestampCount; i++ {
		t := lh.timestampHistory[i]
		if t.After(windowStart) && t.Before(now) {
			count++
			if firstTime.IsZero() || t.Before(firstTime) {
				firstTime = t
			}
			if lastTime.IsZero() || t.After(lastTime) {
				lastTime = t
			}
		}
	}

	if count < 2 || firstTime.Equal(lastTime) {
		rate := float64(count) / float64(HealthWindow)
		return rate, rate / lh.configuredRate
	}

	// Calculate rate from first to last in window
	duration := lastTime.Sub(firstTime).Seconds()
	if duration <= 0 {
		rate := float64(count) / float64(HealthWindow)
		return rate, rate / lh.configuredRate
	}

	rate := float64(count-1) / duration

	// Score: rate/configuredRate, capped at 1.0
	score := rate / lh.configuredRate
	if score > 1.0 {
		score = 1.0
	}
	if score < 0.1 {
		score = 0.1
	}

	return rate, score
}

// computeDriftRate calculates baseline drift rate
// Spec: drift_rate = |B_t - B_{t-1h}| / |B_{t-1h}| (normalized L2 change per hour)
// score = max(0, 1 - drift_rate / 0.1) where 10% per hour -> score=0
// Returns: (drift rate normalized 0-1, score 0-1 where 1 is stable)
func (lh *LinkHealth) computeDriftRate() (float64, float64) {
	if lh.baselineCount < 2 {
		return 0, 1.0 // No drift data - assume stable
	}

	// Compare oldest and newest baselines
	oldestIdx := (lh.baselineWriteIdx - lh.baselineCount + DriftWindow) % DriftWindow
	newestIdx := (lh.baselineWriteIdx - 1 + DriftWindow) % DriftWindow

	oldest := lh.baselineHistory[oldestIdx]
	newest := lh.baselineHistory[newestIdx]

	if oldest == nil || newest == nil || len(oldest) != len(newest) {
		return 0, 1.0
	}

	// Compute L2 norm change (normalized)
	var diffSqSum float64
	var oldSqSum float64
	for k := 0; k < len(oldest) && k < len(newest); k++ {
		diff := newest[k] - oldest[k]
		diffSqSum += diff * diff
		oldSqSum += oldest[k] * oldest[k]
	}

	if oldSqSum == 0 {
		return 0, 1.0
	}

	// Normalized L2 change per hour
	driftRate := math.Sqrt(diffSqSum) / math.Sqrt(oldSqSum)

	// Score: per spec, score = max(0, 1 - drift_rate / 0.1)
	// 0% drift -> score=1.0, 10% drift -> score=0.0
	score := 1.0 - driftRate/0.1
	if score > 1.0 {
		score = 1.0
	}
	if score < 0 {
		score = 0.0
	}

	// Cap driftRate display at 1.0
	if driftRate > 1.0 {
		return 1.0, score
	}
	return driftRate, score
}

// computeCompositeScore combines all metrics into a single confidence score
// Uses specified weights: SNR 40%, Phase Stability 30%, Packet Rate 20%, Baseline Drift 10%
func (lh *LinkHealth) computeCompositeScore() float64 {
	// Use precomputed sub-scores with specified weights
	score := SNRWeight*lh.SNRScore +
		PhaseStabilityWeight*lh.PhaseStabilityScore +
		PacketRateWeight*lh.PacketRateScore +
		BaselineDriftWeight*lh.DriftScore

	// Clamp to 0-1
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}

	return score
}

// GetHealthMetrics returns current health metrics
func (lh *LinkHealth) GetHealthMetrics() (snr, phaseStability, packetRate, driftRate, confidence float64) {
	lh.mu.RLock()
	defer lh.mu.RUnlock()
	return lh.SNR, lh.PhaseStability, lh.PacketRate, lh.DriftRate, lh.ambientConfidence
}

// HealthDetails represents the detailed health scores for API response
type HealthDetails struct {
	SNR           float64 `json:"snr"`
	PhaseStability float64 `json:"phase_stability"`
	PacketRate    float64 `json:"packet_rate"`
	BaselineDrift float64 `json:"baseline_drift"`
}

// GetHealthDetails returns the sub-scores for dashboard breakdown
func (lh *LinkHealth) GetHealthDetails() HealthDetails {
	lh.mu.RLock()
	defer lh.mu.RUnlock()
	return HealthDetails{
		SNR:           lh.SNRScore,
		PhaseStability: lh.PhaseStabilityScore,
		PacketRate:    lh.PacketRateScore,
		BaselineDrift: lh.DriftScore,
	}
}

// GetAmbientConfidence returns the composite confidence score
func (lh *LinkHealth) GetAmbientConfidence() float64 {
	lh.mu.RLock()
	defer lh.mu.RUnlock()
	return lh.ambientConfidence
}


// GetDeltaRMSVariance returns the variance of deltaRMS values during motion periods
// This is used for periodic interference detection (diagnostic Rule 5)
func (lh *LinkHealth) GetDeltaRMSVariance() float64 {
	lh.mu.RLock()
	defer lh.mu.RUnlock()

	if lh.deltaRMSCount < 2 {
		return 0
	}

	// Compute variance using Welford's algorithm
	var mean, m2 float64
	count := 0
	for i := 0; i < lh.deltaRMSCount; i++ {
		v := lh.deltaRMSHistory[i]
		count++
		delta := v - mean
		mean += delta / float64(count)
		delta2 := v - mean
		m2 += delta * delta2
	}

	if count < 2 {
		return 0
	}

	return m2 / float64(count-1)
}

// Reset clears all health tracking state
func (lh *LinkHealth) Reset() {
	lh.mu.Lock()
	defer lh.mu.Unlock()

	for i := range lh.rssiHistory {
		lh.rssiHistory[i] = 0
	}
	lh.rssiWriteIdx = 0
	lh.rssiCount = 0

	for i := range lh.phaseVarHistory {
		lh.phaseVarHistory[i] = 0
	}
	lh.phaseVarWriteIdx = 0
	lh.phaseVarCount = 0

	for i := range lh.timestampHistory {
		lh.timestampHistory[i] = time.Time{}
	}
	lh.timestampWriteIdx = 0
	lh.timestampCount = 0

	for i := range lh.baselineHistory {
		lh.baselineHistory[i] = nil
	}
	lh.baselineWriteIdx = 0
	lh.baselineCount = 0

	lh.SNR = 0.5
	lh.PhaseStability = 1.0
	lh.PacketRate = 0
	lh.DriftRate = 0
	lh.ambientConfidence = 0
}

// HealthSnapshot represents a serializable snapshot of health state
type HealthSnapshot struct {
	LinkID            string
	SNR               float64
	PhaseStability    float64
	PacketRate        float64
	DriftRate         float64
	AmbientConfidence float64
	LastUpdate        time.Time
}

// GetSnapshot returns a snapshot for persistence
func (lh *LinkHealth) GetSnapshot() *HealthSnapshot {
	lh.mu.RLock()
	defer lh.mu.RUnlock()

	return &HealthSnapshot{
		LinkID:            lh.linkID,
		SNR:               lh.SNR,
		PhaseStability:    lh.PhaseStability,
		PacketRate:        lh.PacketRate,
		DriftRate:         lh.DriftRate,
		AmbientConfidence: lh.ambientConfidence,
		LastUpdate:        lh.lastUpdate,
	}
}

// HealthManager manages link health for all links
type HealthManager struct {
	mu     sync.RWMutex
	health map[string]*LinkHealth // keyed by linkID
	nSub   int
}

// NewHealthManager creates a new health manager
func NewHealthManager(nSub int) *HealthManager {
	return &HealthManager{
		health: make(map[string]*LinkHealth),
		nSub:   nSub,
	}
}

// GetOrCreate returns health tracker for a link, creating if needed
func (hm *HealthManager) GetOrCreate(linkID string) *LinkHealth {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	if h, exists := hm.health[linkID]; exists {
		return h
	}

	h := NewLinkHealth(linkID, hm.nSub)
	hm.health[linkID] = h
	return h
}

// Get returns health tracker for a link, or nil if not exists
func (hm *HealthManager) Get(linkID string) *LinkHealth {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	return hm.health[linkID]
}

// GetAllHealth returns health metrics for all links
func (hm *HealthManager) GetAllHealth() map[string]*HealthSnapshot {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	result := make(map[string]*HealthSnapshot)
	for linkID, h := range hm.health {
		result[linkID] = h.GetSnapshot()
	}
	return result
}

// ComputeAllHealth triggers health computation for all links
func (hm *HealthManager) ComputeAllHealth() {
	hm.mu.RLock()
	links := make([]*LinkHealth, 0, len(hm.health))
	for _, h := range hm.health {
		links = append(links, h)
	}
	hm.mu.RUnlock()

	for _, h := range links {
		h.ComputeHealth()
	}
}

// Remove removes health tracking for a link
func (hm *HealthManager) Remove(linkID string) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	delete(hm.health, linkID)
}

// GetSystemHealth returns overall system health score
func (hm *HealthManager) GetSystemHealth() float64 {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	if len(hm.health) == 0 {
		return 0
	}

	var sum float64
	for _, h := range hm.health {
		sum += h.GetAmbientConfidence()
	}

	return sum / float64(len(hm.health))
}

// GetWorstLink returns the link with lowest health score
func (hm *HealthManager) GetWorstLink() (linkID string, score float64) {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	worstScore := 2.0 // Start above 1.0
	worstID := ""

	for linkID, h := range hm.health {
		conf := h.GetAmbientConfidence()
		if conf < worstScore {
			worstScore = conf
			worstID = linkID
		}
	}

	return worstID, worstScore
}
