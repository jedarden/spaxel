// Package sleep provides FFT-based breathing rate estimation for sleep monitoring.
package sleep

import (
	"math"
	"math/cmplx"
	"sync"

	"gonum.org/v1/gonum/dsp/fourier"
)

// FFT estimator constants
const (
	FFTEstimatorSampleRate = 20.0              // Hz — matches signal pipeline
	FFTEstimatorFFTSize     = 512              // Input window (25.6 s at 20 Hz)
	FFTEstimatorZeroPad     = 1024             // Zero-padded FFT size
	FFTEstimatorEMAlpha     = 1.0 / 60.0       // 60-second EMA smoothing
	FFTEstimatorMinHz       = 0.1              // 6 BPM lower bound
	FFTEstimatorMaxHz       = 0.5              // 30 BPM upper bound
	FFTEstimatorMinBPM      = 6.0
	FFTEstimatorMaxBPM      = 30.0
)

// BreathingRateEstimator accumulates phase samples and estimates breathing rate via FFT.
// It operates on bandpass-filtered residual phase from the most motion-sensitive link
// in a sleep zone, producing one BPM estimate per 25.6-second window with 60-second EMA smoothing.
type BreathingRateEstimator struct {
	mu          sync.RWMutex
	buffer      []float64 // Circular buffer of FFTEstimatorFFTSize samples
	writeIdx    int
	sampleCount int
	emaRate     float64 // EMA-smoothed BPM
	lastRate    float64 // Most recent raw FFT BPM
}

// NewBreathingRateEstimator creates a new FFT-based breathing rate estimator.
func NewBreathingRateEstimator() *BreathingRateEstimator {
	return &BreathingRateEstimator{
		buffer: make([]float64, FFTEstimatorFFTSize),
	}
}

// AddPhaseSample adds a bandpass-filtered phase sample to the circular buffer.
func (e *BreathingRateEstimator) AddPhaseSample(phase float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.buffer[e.writeIdx] = phase
	e.writeIdx = (e.writeIdx + 1) % FFTEstimatorFFTSize
	if e.sampleCount < FFTEstimatorFFTSize {
		e.sampleCount++
	}
}

// EstimateRate runs FFT on accumulated samples and returns EMA-smoothed BPM.
// Returns 0 if insufficient samples have been collected.
func (e *BreathingRateEstimator) EstimateRate() float64 {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.sampleCount < FFTEstimatorFFTSize {
		return e.emaRate
	}

	bpm := computeFFTBreathingRate(e.buffer, e.writeIdx, FFTEstimatorSampleRate, FFTEstimatorZeroPad)

	// Reject out-of-physiological-range estimates
	if bpm < FFTEstimatorMinBPM || bpm > FFTEstimatorMaxBPM {
		return e.emaRate
	}

	// Apply 60-second EMA smoothing: ema = α × bpm + (1-α) × ema
	if e.emaRate > 0 {
		bpm = FFTEstimatorEMAlpha*bpm + (1-FFTEstimatorEMAlpha)*e.emaRate
	}

	e.emaRate = bpm
	e.lastRate = bpm
	return bpm
}

// GetRate returns the current EMA-smoothed breathing rate.
func (e *BreathingRateEstimator) GetRate() float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.emaRate
}

// Reset clears the estimator state.
func (e *BreathingRateEstimator) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()

	for i := range e.buffer {
		e.buffer[i] = 0
	}
	e.writeIdx = 0
	e.sampleCount = 0
	e.emaRate = 0
	e.lastRate = 0
}

// Ready returns true if the buffer has enough samples for an FFT window.
func (e *BreathingRateEstimator) Ready() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.sampleCount >= FFTEstimatorFFTSize
}

// computeFFTBreathingRate runs FFT on phase samples and returns the dominant
// breathing frequency converted to BPM.
//
// Parameters:
//   - buffer: circular buffer of phase samples (length n)
//   - writeIdx: position where the next sample will be written
//   - sampleRate: sampling rate in Hz
//   - zeroPadSize: FFT size (must be power of 2, >= len(buffer))
func computeFFTBreathingRate(buffer []float64, writeIdx int, sampleRate, zeroPadSize float64) float64 {
	n := len(buffer)
	N := int(zeroPadSize)

	// Build zero-padded real input from circular buffer (chronological order)
	seq := make([]float64, N)
	for i := 0; i < n; i++ {
		idx := (writeIdx + i) % n
		seq[i] = buffer[idx]
	}

	// Run FFT
	fft := fourier.NewFFT(N)
	coeff := fft.Coefficients(nil, seq)

	// Frequency resolution: Fs / N
	freqRes := sampleRate / zeroPadSize

	// Bin range for 0.1–0.5 Hz
	minBin := int(math.Ceil(FFTEstimatorMinHz / freqRes))
	maxBin := int(math.Floor(FFTEstimatorMaxHz / freqRes))
	if minBin < 1 {
		minBin = 1 // skip DC
	}
	if maxBin > N/2 {
		maxBin = N / 2 // Nyquist
	}

	// Find dominant magnitude peak in breathing band
	maxMag := 0.0
	peakBin := minBin
	for bin := minBin; bin <= maxBin; bin++ {
		mag := cmplx.Abs(coeff[bin])
		if mag > maxMag {
			maxMag = mag
			peakBin = bin
		}
	}

	// Convert bin index to BPM: bpm = bin_idx × (Fs/N) × 60
	freqHz := float64(peakBin) * freqRes
	return freqHz * 60.0
}
