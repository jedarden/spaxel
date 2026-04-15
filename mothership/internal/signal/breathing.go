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

	// FFT-based breathing detection constants (Phase 6)
	FFTBreathingBufferSize    = 60    // 30 seconds at 2Hz adaptive rate
	FFTMinBreathingHz         = 0.2   // Lower bound of breathing band (FFT)
	FFTMaxBreathingHz         = 1.0   // Upper bound of breathing band (FFT) - double breathing rate
	FFTSNRThreshold           = 15.0  // Minimum SNR in dB to declare breathing
	FFTSampleRateHz           = 2.0   // Adaptive sensing rate for breathing buffer
	FFTMinSamples             = 30    // Minimum 15s of data before detection can fire

	// Dwell tracker timeouts (Phase 6)
	DwellMotionToPossiblyTime   = 500  // ms - debounce before transitioning to POSSIBLY_PRESENT
	DwellPossiblyToClearTime    = 60000 // ms - 60s without motion/breathing -> CLEAR
	DwellStationaryToClearTime  = 120000 // ms - 120s without breathing -> CLEAR
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

// ============================================
// FFT-Based Breathing Detector (Phase 6)
// ============================================

// FFTBreathingResult holds the result of FFT-based breathing detection
type FFTBreathingResult struct {
	IsBreathing   bool    // True if breathing detected
	FrequencyHz   float64 // Peak frequency in Hz (0.2-1.0 Hz band)
	Confidence    float64 // Detection confidence (0-1)
	PeakSNRdB     float64 // Peak-to-median SNR in dB
	BreathingBPM  float64 // Estimated breathing rate in breaths per minute
}

// FFTBreathingDetector detects stationary persons via FFT analysis of deltaRMS samples
// Uses a rolling buffer and FFT to find periodic breathing signals
type FFTBreathingDetector struct {
	mu sync.RWMutex

	// Rolling buffer for deltaRMS samples
	buffer      []float64
	bufferSize  int
	writeIdx    int
	sampleCount int

	// Precomputed Hann window coefficients
	hannWindow []float64

	// Configuration
	sampleRateHz float64 // Current sample rate (adaptive, default 2Hz)
	minFreqHz    float64 // Low end of breathing band (default 0.2 Hz)
	maxFreqHz    float64 // High end of breathing band (default 1.0 Hz)
	snrThreshold float64 // Minimum peak-to-noise ratio in dB

	// Last detection result
	lastResult FFTBreathingResult

	// Health gating
	healthGated bool
}

// NewFFTBreathingDetector creates a new FFT-based breathing detector
func NewFFTBreathingDetector() *FFTBreathingDetector {
	return NewFFTBreathingDetectorWithConfig(
		FFTBreathingBufferSize,
		FFTSampleRateHz,
		FFTMinBreathingHz,
		FFTMaxBreathingHz,
		FFTSNRThreshold,
	)
}

// NewFFTBreathingDetectorWithConfig creates a new FFT-based breathing detector with custom config
func NewFFTBreathingDetectorWithConfig(bufferSize int, sampleRateHz, minFreqHz, maxFreqHz, snrThreshold float64) *FFTBreathingDetector {
	bd := &FFTBreathingDetector{
		buffer:       make([]float64, bufferSize),
		bufferSize:   bufferSize,
		sampleRateHz: sampleRateHz,
		minFreqHz:    minFreqHz,
		maxFreqHz:    maxFreqHz,
		snrThreshold: snrThreshold,
	}

	// Precompute Hann window coefficients
	bd.hannWindow = make([]float64, bufferSize)
	for i := 0; i < bufferSize; i++ {
		bd.hannWindow[i] = 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(bufferSize-1)))
	}

	return bd
}

// AddSample adds a deltaRMS sample to the rolling buffer
func (bd *FFTBreathingDetector) AddSample(deltaRMS float64) {
	bd.mu.Lock()
	defer bd.mu.Unlock()

	bd.buffer[bd.writeIdx] = deltaRMS
	bd.writeIdx = (bd.writeIdx + 1) % bd.bufferSize
	if bd.sampleCount < bd.bufferSize {
		bd.sampleCount++
	}
}

// SetHealthGated enables or disables detection based on link health
func (bd *FFTBreathingDetector) SetHealthGated(gated bool) {
	bd.mu.Lock()
	defer bd.mu.Unlock()
	bd.healthGated = gated
}

// Detect runs FFT analysis and returns breathing detection result
func (bd *FFTBreathingDetector) Detect() FFTBreathingResult {
	bd.mu.Lock()
	defer bd.mu.Unlock()

	// Not enough samples
	if bd.sampleCount < FFTMinSamples {
		return FFTBreathingResult{}
	}

	// Health gated - detection disabled
	if bd.healthGated {
		return FFTBreathingResult{}
	}

	// Build the windowed sample array
	n := bd.sampleCount
	windowed := make([]float64, n)

	// Copy samples in chronological order with Hann window applied
	startIdx := (bd.writeIdx - n + bd.bufferSize) % bd.bufferSize
	for i := 0; i < n; i++ {
		idx := (startIdx + i) % bd.bufferSize
		windowed[i] = bd.buffer[idx] * bd.hannWindow[i]
	}

	// Compute DFT for the breathing band
	// Frequency resolution = sampleRate / n
	binResolution := bd.sampleRateHz / float64(n)

	// Find bins corresponding to breathing band
	minBin := int(math.Floor(bd.minFreqHz / binResolution))
	maxBin := int(math.Ceil(bd.maxFreqHz / binResolution))

	if minBin < 1 {
		minBin = 1 // Skip DC component
	}
	if maxBin > n/2 {
		maxBin = n / 2 // Nyquist limit
	}

	// Compute amplitude spectrum using DFT
	spectrum := make([]float64, maxBin+1)
	for bin := 0; bin <= maxBin; bin++ {
		var re, im float64
		for i := 0; i < n; i++ {
			angle := -2 * math.Pi * float64(i*bin) / float64(n)
			re += windowed[i] * math.Cos(angle)
			im += windowed[i] * math.Sin(angle)
		}
		spectrum[bin] = math.Sqrt(re*re + im*im)
	}

	// Find peak in breathing band
	peakAmplitude := 0.0
	peakBin := minBin
	for bin := minBin; bin <= maxBin; bin++ {
		if spectrum[bin] > peakAmplitude {
			peakAmplitude = spectrum[bin]
			peakBin = bin
		}
	}

	// Compute in-band amplitude statistics for SNR estimation.
	// Use the median of all in-band bins as the noise floor.
	// Exclude the peak bin to get a better baseline estimate.
	inBandAmps := make([]float64, 0, maxBin-minBin+1)
	for bin := minBin; bin <= maxBin; bin++ {
		if bin != peakBin {
			inBandAmps = append(inBandAmps, spectrum[bin])
		}
	}
	// Fall back to full spectrum median if not enough in-band bins
	var medianAmplitude float64
	if len(inBandAmps) >= 3 {
		medianAmplitude = computeMedian(inBandAmps)
	} else {
		medianAmplitude = computeMedian(spectrum)
	}

	// Avoid division by zero
	if medianAmplitude < 1e-10 {
		bd.lastResult = FFTBreathingResult{}
		return bd.lastResult
	}

	// Compute SNR in dB
	snrDb := 20 * math.Log10(peakAmplitude/medianAmplitude)

	// Frequency at peak
	freqHz := float64(peakBin) * binResolution

	// Convert to breathing rate in BPM
	// Note: the phase oscillates at TWICE the physical breathing rate
	// because path length changes twice per breath cycle
	// So breathing_bpm = (freq_hz / 2) * 60 = freq_hz * 30
	breathingBPM := freqHz * 30.0

	// Validate physiological range (6-30 BPM)
	if breathingBPM < 6 || breathingBPM > 30 {
		bd.lastResult = FFTBreathingResult{
			IsBreathing:  false,
			FrequencyHz:  freqHz,
			Confidence:   0,
			PeakSNRdB:    snrDb,
			BreathingBPM: breathingBPM,
		}
		return bd.lastResult
	}

	// Detection decision
	isBreathing := snrDb >= bd.snrThreshold

	// Confidence based on SNR (0-1 scale)
	confidence := math.Min(1.0, math.Max(0.0, (snrDb-bd.snrThreshold)/10.0))

	bd.lastResult = FFTBreathingResult{
		IsBreathing:   isBreathing,
		FrequencyHz:   freqHz,
		Confidence:    confidence,
		PeakSNRdB:     snrDb,
		BreathingBPM:  breathingBPM,
	}

	return bd.lastResult
}

// GetLastResult returns the most recent detection result
func (bd *FFTBreathingDetector) GetLastResult() FFTBreathingResult {
	bd.mu.RLock()
	defer bd.mu.RUnlock()
	return bd.lastResult
}

// Reset clears the detector state
func (bd *FFTBreathingDetector) Reset() {
	bd.mu.Lock()
	defer bd.mu.Unlock()

	for i := range bd.buffer {
		bd.buffer[i] = 0
	}
	bd.writeIdx = 0
	bd.sampleCount = 0
	bd.lastResult = FFTBreathingResult{}
	bd.healthGated = false
}

// computeMedian computes the median of a float64 slice
func computeMedian(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}

	// Copy and sort
	sorted := make([]float64, len(data))
	copy(sorted, data)

	// Simple sort (good enough for small arrays)
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j] < sorted[i] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

// ============================================
// Dwell State Tracker (Phase 6)
// ============================================

// DwellState represents the current dwell state of a link
type DwellState int

const (
	DwellClear DwellState = iota
	DwellMotionDetected
	DwellPossiblyPresent
	DwellStationaryDetected
)

// String returns the string representation of the dwell state
func (s DwellState) String() string {
	switch s {
	case DwellClear:
		return "CLEAR"
	case DwellMotionDetected:
		return "MOTION_DETECTED"
	case DwellPossiblyPresent:
		return "POSSIBLY_PRESENT"
	case DwellStationaryDetected:
		return "STATIONARY_DETECTED"
	default:
		return "UNKNOWN"
	}
}

// MarshalText implements encoding.TextMarshaler
func (s DwellState) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// DwellTracker tracks the dwell state for presence detection
type DwellTracker struct {
	mu sync.RWMutex

	state           DwellState
	lastMotionTime  time.Time // When motion was last detected
	lastBreathTime  time.Time // When breathing was last detected
	stateChangeTime time.Time // When we entered the current state
	motionDebounceStart time.Time // When current quiescence started

	// FFT-based breathing detector
	fftDetector *FFTBreathingDetector

	// Configuration
	motionDebounceMs    int64 // ms to wait before transitioning from MOTION to POSSIBLY
	possiblyTimeoutMs   int64 // ms before transitioning POSSIBLY to CLEAR
	stationaryTimeoutMs int64 // ms before transitioning STATIONARY to CLEAR
}

// NewDwellTracker creates a new dwell tracker
func NewDwellTracker() *DwellTracker {
	return &DwellTracker{
		state:               DwellClear,
		fftDetector:         NewFFTBreathingDetector(),
		motionDebounceMs:    DwellMotionToPossiblyTime,
		possiblyTimeoutMs:   DwellPossiblyToClearTime,
		stationaryTimeoutMs: DwellStationaryToClearTime,
	}
}

// DwellUpdate contains the result of updating the dwell tracker
type DwellUpdate struct {
	State             DwellState
	PreviousState     DwellState
	StateChanged      bool
	BreathingDetected bool
	BreathingRate     float64 // BPM
	BreathingSNR      float64 // dB
	HealthGated       bool
}

// Update processes new motion and health data, returns state update
func (dt *DwellTracker) Update(isMotion bool, deltaRMS float64, healthScore float64, now time.Time) DwellUpdate {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	previousState := dt.state
	update := DwellUpdate{
		PreviousState: previousState,
		State:         previousState,
	}

	// Health gate for breathing detection
	healthGated := healthScore < BreathingHealthThreshold
	dt.fftDetector.SetHealthGated(healthGated)
	update.HealthGated = healthGated

	// Add sample to FFT detector
	dt.fftDetector.AddSample(deltaRMS)

	// Run breathing detection
	fftResult := dt.fftDetector.Detect()
	update.BreathingDetected = fftResult.IsBreathing
	update.BreathingRate = fftResult.BreathingBPM
	update.BreathingSNR = fftResult.PeakSNRdB

	// Update timestamps
	if isMotion {
		dt.lastMotionTime = now
	}
	if fftResult.IsBreathing {
		dt.lastBreathTime = now
	}

	// State machine transitions
	switch dt.state {
	case DwellClear:
		if isMotion {
			dt.transitionTo(DwellMotionDetected, now)
		}

	case DwellMotionDetected:
		if isMotion {
			// Stay in motion - lastMotionTime already updated above
		} else {
			// Check if debounce period elapsed since last motion
			timeSinceMotion := now.Sub(dt.lastMotionTime).Milliseconds()
			if timeSinceMotion >= dt.motionDebounceMs {
				dt.transitionTo(DwellPossiblyPresent, now)
			}
		}

	case DwellPossiblyPresent:
		if isMotion {
			// Back to motion
			dt.transitionTo(DwellMotionDetected, now)
		} else if fftResult.IsBreathing {
			// Breathing detected!
			dt.transitionTo(DwellStationaryDetected, now)
		} else {
			// Check for timeout to CLEAR
			timeSinceMotion := now.Sub(dt.lastMotionTime).Milliseconds()
			timeSinceBreath := now.Sub(dt.lastBreathTime).Milliseconds()
			if timeSinceMotion >= dt.possiblyTimeoutMs && timeSinceBreath >= dt.possiblyTimeoutMs {
				dt.transitionTo(DwellClear, now)
			}
		}

	case DwellStationaryDetected:
		if isMotion {
			// Motion interrupts breathing detection
			dt.transitionTo(DwellMotionDetected, now)
		} else if !fftResult.IsBreathing {
			// Breathing no longer detected
			dt.transitionTo(DwellPossiblyPresent, now)
		} else {
			// Check for timeout to CLEAR
			timeSinceBreath := now.Sub(dt.lastBreathTime).Milliseconds()
			if timeSinceBreath >= dt.stationaryTimeoutMs {
				dt.transitionTo(DwellClear, now)
			}
		}
	}

	update.State = dt.state
	update.StateChanged = dt.state != previousState

	return update
}

// transitionTo transitions to a new state
func (dt *DwellTracker) transitionTo(newState DwellState, now time.Time) {
	dt.state = newState
	dt.stateChangeTime = now
	if newState == DwellMotionDetected {
		dt.motionDebounceStart = time.Time{}
	}
}

// GetState returns the current dwell state
func (dt *DwellTracker) GetState() DwellState {
	dt.mu.RLock()
	defer dt.mu.RUnlock()
	return dt.state
}

// GetBreathingRate returns the current estimated breathing rate in BPM
func (dt *DwellTracker) GetBreathingRate() float64 {
	dt.mu.RLock()
	defer dt.mu.RUnlock()
	return dt.fftDetector.GetLastResult().BreathingBPM
}

// GetBreathingSNR returns the current breathing detection SNR in dB
func (dt *DwellTracker) GetBreathingSNR() float64 {
	dt.mu.RLock()
	defer dt.mu.RUnlock()
	return dt.fftDetector.GetLastResult().PeakSNRdB
}

// IsBreathingDetected returns whether breathing is currently detected
func (dt *DwellTracker) IsBreathingDetected() bool {
	dt.mu.RLock()
	defer dt.mu.RUnlock()
	return dt.fftDetector.GetLastResult().IsBreathing
}

// GetStateDuration returns how long we've been in the current state
func (dt *DwellTracker) GetStateDuration(now time.Time) time.Duration {
	dt.mu.RLock()
	defer dt.mu.RUnlock()
	if dt.stateChangeTime.IsZero() {
		return 0
	}
	return now.Sub(dt.stateChangeTime)
}

// Reset clears the dwell tracker state
func (dt *DwellTracker) Reset() {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	dt.state = DwellClear
	dt.lastMotionTime = time.Time{}
	dt.lastBreathTime = time.Time{}
	dt.stateChangeTime = time.Time{}
	dt.motionDebounceStart = time.Time{}
	dt.fftDetector.Reset()
}
