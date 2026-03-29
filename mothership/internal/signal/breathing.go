// Package signal implements breathing band detection for stationary person presence
package signal

import (
	"math"
	"sync"
	"time"
)

// Breathing detection constants
const (
	BreathingSampleRate       = 20.0  // Hz (active rate)
	BreathingMinHz            = 0.1   // Lower bound of breathing band
	BreathingMaxHz            = 0.5   // Upper bound of breathing band
	BreathingRMSWindow        = 1200  // 60 seconds at 20Hz
	BreathingThreshold        = 0.005 // Radians - detection threshold
	BreathingSustainTime      = 30    // Seconds sustained before detection
	BreathingFFTSize          = 512   // FFT window size (25.6s at 20Hz)
	BreathingFFTZeroPad       = 1024  // Zero-padded size for resolution
	BreathingMotionThreshold  = 0.03  // smoothDeltaRMS below which breathing is computed
	BreathingEMAlpha          = 0.01  // EMA smoothing for breathing rate
	BreathingHealthThreshold  = 0.7   // Minimum link health for breathing detection
)

// BiquadCoeffs holds coefficients for one biquad section
type BiquadCoeffs struct {
	B0, B1, B2 float64
	A1, A2     float64
}

// Precomputed coefficients for 4th order Butterworth bandpass 0.1-0.5 Hz at Fs=20 Hz
// These are computed using scipy.signal.butter(4, [0.1, 0.5], btype='band', fs=20)
var butterworthBiquads = [2]BiquadCoeffs{
	// Section 1
	{
		B0: 0.0004019,
		B1: 0.0,
		B2: -0.0004019,
		A1: -1.9645,
		A2: 0.9651,
	},
	// Section 2
	{
		B0: 0.0004019,
		B1: 0.0,
		B2: -0.0004019,
		A1: -1.9499,
		A2: 0.9508,
	},
}

// BiquadState holds state for one biquad section
type BiquadState struct {
	X1, X2 float64 // Previous inputs
	Y1, Y2 float64 // Previous outputs
}

// BreathingDetector detects stationary persons via breathing micro-motion
type BreathingDetector struct {
	mu sync.RWMutex

	// Filter state for each biquad section
	biquadStates [2]BiquadState

	// RMS computation window
	rmsBuffer   []float64
	rmsWriteIdx int
	rmsCount    int

	// Detection state
	breathingRMS   float64
	sustainedCount int // Frames above threshold
	detected       bool
	detectionStart time.Time
	healthGated    bool // True if disabled due to low link health

	// Breathing rate estimation
	rateBuffer    []float64 // Phase history for FFT
	rateWriteIdx  int
	rateCount     int
	breathingRate float64 // BPM

	nSub int
}

// BreathingFeatures holds the result of breathing detection
type BreathingFeatures struct {
	Computed        bool    // True if breathing was computed (room was still)
	BreathingRMS    float64 // RMS of filtered phase (radians)
	Detected        bool    // True if stationary person detected
	BreathingRate   float64 // Estimated breathing rate in BPM
	SustainedFrames int     // Frames above threshold
	HealthGated     bool    // True if detection was disabled due to poor health
}

// NewBreathingDetector creates a new breathing detector
func NewBreathingDetector(nSub int) *BreathingDetector {
	return &BreathingDetector{
		rmsBuffer:  make([]float64, BreathingRMSWindow),
		rateBuffer: make([]float64, BreathingFFTSize),
		nSub:       nSub,
	}
}

// Process processes residual phase data and returns breathing features
// Should only be called when smoothDeltaRMS < BreathingMotionThreshold
func (bd *BreathingDetector) Process(residualPhase []float64, smoothDeltaRMS float64) *BreathingFeatures {
	return bd.ProcessWithHealth(residualPhase, smoothDeltaRMS, 1.0)
}

// ProcessWithHealth processes residual phase data with health gating.
// If healthScore < BreathingHealthThreshold, detection is disabled.
// Should only be called when smoothDeltaRMS < BreathingMotionThreshold
func (bd *BreathingDetector) ProcessWithHealth(residualPhase []float64, smoothDeltaRMS float64, healthScore float64) *BreathingFeatures {
	bd.mu.Lock()
	defer bd.mu.Unlock()

	features := &BreathingFeatures{
		Computed: false,
	}

	// Health gate: disable detection when link health is poor
	if healthScore < BreathingHealthThreshold {
		bd.healthGated = true
		// Reset sustained count when gated
		bd.sustainedCount = 0
		bd.detected = false
		features.HealthGated = true
		return features
	}
	bd.healthGated = false

	// Only compute when room is still (no walking motion)
	if smoothDeltaRMS >= BreathingMotionThreshold {
		// Reset sustained count on motion
		bd.sustainedCount = 0
		return features
	}

	// Compute mean phase over data subcarriers
	meanPhase := bd.computeMeanPhase(residualPhase)

	// Apply Butterworth bandpass filter
	filtered := bd.applyFilter(meanPhase)

	// Store in RMS buffer
	bd.rmsBuffer[bd.rmsWriteIdx] = filtered
	bd.rmsWriteIdx = (bd.rmsWriteIdx + 1) % BreathingRMSWindow
	if bd.rmsCount < BreathingRMSWindow {
		bd.rmsCount++
	}

	// Compute RMS over window
	bd.breathingRMS = bd.computeRMS()

	// Detection logic with sustained requirement
	if bd.breathingRMS > BreathingThreshold {
		bd.sustainedCount++
		if bd.sustainedCount >= BreathingSustainTime*int(BreathingSampleRate) {
			if !bd.detected {
				bd.detectionStart = time.Now()
			}
			bd.detected = true
		}
	} else {
		bd.sustainedCount = 0
		bd.detected = false
	}

	// Breathing rate estimation via FFT
	bd.rateBuffer[bd.rateWriteIdx] = filtered
	bd.rateWriteIdx = (bd.rateWriteIdx + 1) % BreathingFFTSize
	if bd.rateCount < BreathingFFTSize {
		bd.rateCount++
	}

	// Estimate breathing rate when we have enough data
	if bd.rateCount >= BreathingFFTSize {
		bd.breathingRate = bd.estimateBreathingRate()
	}

	features.Computed = true
	features.BreathingRMS = bd.breathingRMS
	features.Detected = bd.detected
	features.BreathingRate = bd.breathingRate
	features.SustainedFrames = bd.sustainedCount

	return features
}

// computeMeanPhase computes mean residual phase over data subcarriers
func (bd *BreathingDetector) computeMeanPhase(phase []float64) float64 {
	indices := DataSubcarrierIndices(bd.nSub)
	if len(indices) == 0 || len(phase) == 0 {
		return 0
	}

	var sum float64
	count := 0
	for _, k := range indices {
		if k < len(phase) {
			sum += phase[k]
			count++
		}
	}

	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

// applyFilter applies the 4th order Butterworth bandpass filter
func (bd *BreathingDetector) applyFilter(x float64) float64 {
	// Cascade through both biquad sections
	y := x
	for i := 0; i < 2; i++ {
		coef := butterworthBiquads[i]
		state := &bd.biquadStates[i]

		// Direct Form II transposed
		// y = b0*x + b1*x1 + b2*x2 - a1*y1 - a2*y2
		yNew := coef.B0*y + coef.B1*state.X1 + coef.B2*state.X2 - coef.A1*state.Y1 - coef.A2*state.Y2

		// Shift state
		state.X2 = state.X1
		state.X1 = y
		state.Y2 = state.Y1
		state.Y1 = yNew

		y = yNew
	}
	return y
}

// computeRMS computes the RMS of the filtered signal over the window
func (bd *BreathingDetector) computeRMS() float64 {
	if bd.rmsCount == 0 {
		return 0
	}

	var sumSq float64
	for i := 0; i < bd.rmsCount; i++ {
		v := bd.rmsBuffer[i]
		sumSq += v * v
	}

	return math.Sqrt(sumSq / float64(bd.rmsCount))
}

// estimateBreathingRate estimates breathing rate using FFT
func (bd *BreathingDetector) estimateBreathingRate() float64 {
	// Simple DFT for the breathing band
	// We only need bins in 0.1-0.5 Hz range
	// Frequency resolution: Fs/N = 20/1024 ≈ 0.0195 Hz
	// Bins: 0.1 Hz = bin 5, 0.5 Hz = bin 26

	// Zero-padded FFT (simplified - just compute relevant bins)
	maxMag := 0.0
	maxBin := 5

	// Compute magnitude for each bin in breathing range
	for bin := 5; bin <= 26; bin++ {
		// Compute DFT for this bin (real input, complex output)
		var re, im float64
		for n := 0; n < bd.rateCount; n++ {
			// Circular buffer access
			idx := (bd.rateWriteIdx - bd.rateCount + n + BreathingFFTSize) % BreathingFFTSize
			angle := -2 * math.Pi * float64(n*bin) / BreathingFFTZeroPad
			re += bd.rateBuffer[idx] * math.Cos(angle)
			im += bd.rateBuffer[idx] * math.Sin(angle)
		}

		mag := math.Sqrt(re*re + im*im)
		if mag > maxMag {
			maxMag = mag
			maxBin = bin
		}
	}

	// Convert bin to Hz, then to BPM
	freqHz := float64(maxBin) * BreathingSampleRate / BreathingFFTZeroPad
	bpm := freqHz * 60.0

	// Validate range (6-30 BPM)
	if bpm < 6 || bpm > 30 {
		// Out of physiological range - return previous estimate or 0
		return bd.breathingRate
	}

	// Apply EMA smoothing
	if bd.breathingRate > 0 {
		bpm = BreathingEMAlpha*bpm + (1-BreathingEMAlpha)*bd.breathingRate
	}

	return bpm
}

// GetState returns current breathing detection state
func (bd *BreathingDetector) GetState() (detected bool, rms float64, rate float64) {
	bd.mu.RLock()
	defer bd.mu.RUnlock()
	return bd.detected, bd.breathingRMS, bd.breathingRate
}

// Reset resets the breathing detector state
func (bd *BreathingDetector) Reset() {
	bd.mu.Lock()
	defer bd.mu.Unlock()

	for i := 0; i < 2; i++ {
		bd.biquadStates[i] = BiquadState{}
	}
	for i := range bd.rmsBuffer {
		bd.rmsBuffer[i] = 0
	}
	bd.rmsWriteIdx = 0
	bd.rmsCount = 0
	bd.breathingRMS = 0
	bd.sustainedCount = 0
	bd.detected = false
	bd.healthGated = false
	bd.breathingRate = 0
	for i := range bd.rateBuffer {
		bd.rateBuffer[i] = 0
	}
	bd.rateWriteIdx = 0
	bd.rateCount = 0
}

// IsDetected returns whether a stationary person is currently detected
func (bd *BreathingDetector) IsDetected() bool {
	bd.mu.RLock()
	defer bd.mu.RUnlock()
	return bd.detected
}

// GetBreathingRMS returns the current breathing RMS value
func (bd *BreathingDetector) GetBreathingRMS() float64 {
	bd.mu.RLock()
	defer bd.mu.RUnlock()
	return bd.breathingRMS
}

// GetBreathingRate returns the estimated breathing rate in BPM
func (bd *BreathingDetector) GetBreathingRate() float64 {
	bd.mu.RLock()
	defer bd.mu.RUnlock()
	return bd.breathingRate
}

// GetDetectionDuration returns how long a person has been detected
func (bd *BreathingDetector) GetDetectionDuration() time.Duration {
	bd.mu.RLock()
	defer bd.mu.RUnlock()
	if !bd.detected {
		return 0
	}
	return time.Since(bd.detectionStart)
}

// IsHealthGated returns whether detection is disabled due to poor link health
func (bd *BreathingDetector) IsHealthGated() bool {
	bd.mu.RLock()
	defer bd.mu.RUnlock()
	return bd.healthGated
}
