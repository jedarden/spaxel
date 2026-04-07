package sleep

import (
	"math"
	"testing"
)

func TestComputeFFTBreathingRateSynthetic15BPM(t *testing.T) {
	// Generate a synthetic phase signal at 0.25 Hz (15 bpm) at 20 Hz sample rate.
	// The FFT should identify the dominant frequency as 15 bpm.
	sampleRate := 20.0
	nSamples := 512
	zeroPad := 1024

	freqHz := 0.25 // 15 bpm
	buffer := make([]float64, nSamples)
	for i := 0; i < nSamples; i++ {
		t_sec := float64(i) / sampleRate
		buffer[i] = math.Sin(2.0 * math.Pi * freqHz * t_sec)
	}

	bpm := computeFFTBreathingRate(buffer, 0, sampleRate, float64(zeroPad))

	// 15 bpm ± 1 bpm tolerance (frequency resolution is ~1.17 bpm/bin)
	if math.Abs(bpm-15.0) > 1.5 {
		t.Errorf("FFT breathing rate = %.2f bpm, want ~15.0 bpm", bpm)
	}
}

func TestComputeFFTBreathingRateSynthetic12BPM(t *testing.T) {
	// 0.2 Hz = 12 bpm
	sampleRate := 20.0
	buffer := make([]float64, 512)
	for i := range buffer {
		buffer[i] = math.Sin(2.0 * math.Pi * 0.2 * float64(i) / sampleRate)
	}

	bpm := computeFFTBreathingRate(buffer, 0, sampleRate, 1024)

	if math.Abs(bpm-12.0) > 1.5 {
		t.Errorf("FFT breathing rate = %.2f bpm, want ~12.0 bpm", bpm)
	}
}

func TestComputeFFTBreathingRateSynthetic20BPM(t *testing.T) {
	// 0.333 Hz ≈ 20 bpm
	sampleRate := 20.0
	buffer := make([]float64, 512)
	for i := range buffer {
		buffer[i] = math.Sin(2.0 * math.Pi * (1.0/3.0) * float64(i) / sampleRate)
	}

	bpm := computeFFTBreathingRate(buffer, 0, sampleRate, 1024)

	if math.Abs(bpm-20.0) > 2.0 {
		t.Errorf("FFT breathing rate = %.2f bpm, want ~20.0 bpm", bpm)
	}
}

func TestComputeFFTBreathingRateWithNoise(t *testing.T) {
	// 15 bpm signal + Gaussian-like noise. FFT should still find the dominant peak.
	sampleRate := 20.0
	buffer := make([]float64, 512)
	for i := range buffer {
		t_sec := float64(i) / sampleRate
		signal := math.Sin(2.0 * math.Pi * 0.25 * t_sec)
		// Simple pseudo-noise: sum of incommensurate frequencies
		noise := 0.3*math.Sin(2*math.Pi*3.7*t_sec) +
			0.2*math.Sin(2*math.Pi*7.1*t_sec) +
			0.15*math.Sin(2*math.Pi*0.03*t_sec)
		buffer[i] = signal + noise
	}

	bpm := computeFFTBreathingRate(buffer, 0, sampleRate, 1024)

	if math.Abs(bpm-15.0) > 2.0 {
		t.Errorf("FFT breathing rate with noise = %.2f bpm, want ~15.0 bpm", bpm)
	}
}

func TestComputeFFTBreathingRateCircularBuffer(t *testing.T) {
	// Verify that the circular buffer read order is correct when writeIdx != 0.
	sampleRate := 20.0
	nSamples := 512
	zeroPad := 1024

	freqHz := 0.25 // 15 bpm
	buffer := make([]float64, nSamples)

	// Simulate a filled circular buffer where writeIdx is at position 100
	writeIdx := 100
	for i := 0; i < nSamples; i++ {
		// Sample at logical position i was written to (writeIdx + i) % nSamples
		t_sec := float64(i) / sampleRate
		buffer[(writeIdx+i)%nSamples] = math.Sin(2.0 * math.Pi * freqHz * t_sec)
	}

	bpm := computeFFTBreathingRate(buffer, writeIdx, sampleRate, float64(zeroPad))

	if math.Abs(bpm-15.0) > 1.5 {
		t.Errorf("FFT circular buffer breathing rate = %.2f bpm, want ~15.0 bpm", bpm)
	}
}

func TestBreathingRateEstimatorEmaSmoothing(t *testing.T) {
	// Feed a constant 15 bpm signal and verify EMA converges.
	est := NewBreathingRateEstimator()
	sampleRate := 20.0

	// Fill the buffer with a 15 bpm signal
	for i := 0; i < 512; i++ {
		t_sec := float64(i) / sampleRate
		phase := math.Sin(2.0 * math.Pi * 0.25 * t_sec)
		est.AddPhaseSample(phase)
	}

	// First estimate should be close to 15
	rate := est.EstimateRate()
	if math.Abs(rate-15.0) > 2.0 {
		t.Errorf("First estimate = %.2f bpm, want ~15.0 bpm", rate)
	}

	// Continue feeding and estimating — EMA should stabilize near 15
	for rep := 0; rep < 10; rep++ {
		for i := 0; i < 512; i++ {
			t_sec := float64(i) / sampleRate
			phase := math.Sin(2.0 * math.Pi * 0.25 * t_sec)
			est.AddPhaseSample(phase)
		}
		est.EstimateRate()
	}

	finalRate := est.GetRate()
	if math.Abs(finalRate-15.0) > 1.0 {
		t.Errorf("EMA-stabilized rate = %.2f bpm, want ~15.0 bpm", finalRate)
	}
}

func TestBreathingRateEstimatorInsufficientSamples(t *testing.T) {
	est := NewBreathingRateEstimator()

	// Add fewer than 512 samples
	for i := 0; i < 100; i++ {
		est.AddPhaseSample(0.1)
	}

	rate := est.EstimateRate()
	if rate != 0 {
		t.Errorf("Rate with insufficient samples = %.2f, want 0", rate)
	}

	if est.Ready() {
		t.Error("Ready() should be false with insufficient samples")
	}
}

func TestBreathingRateEstimatorReset(t *testing.T) {
	est := NewBreathingRateEstimator()

	for i := 0; i < 512; i++ {
		est.AddPhaseSample(0.1)
	}
	est.EstimateRate() // Populate EMA

	est.Reset()

	if est.GetRate() != 0 {
		t.Errorf("Rate after reset = %.2f, want 0", est.GetRate())
	}
	if est.Ready() {
		t.Error("Ready() should be false after reset")
	}
}

func TestComputeBreathingRegularity(t *testing.T) {
	tests := []struct {
		name     string
		samples  []float64
		wantCV   float64
		tol      float64
	}{
		{
			name:    "constant rate — zero CV",
			samples: []float64{14.0, 14.0, 14.0, 14.0, 14.0},
			wantCV:  0.0,
			tol:     0.001,
		},
		{
			name:    "small variation",
			samples: []float64{14.0, 14.5, 13.5, 14.2, 13.8},
			wantCV:  0.024,
			tol:     0.01,
		},
		{
			name:    "large variation",
			samples: []float64{10.0, 20.0, 12.0, 18.0, 15.0},
			wantCV:  0.276,
			tol:     0.05,
		},
		{
			name:    "empty samples — zero CV",
			samples: []float64{},
			wantCV:  0.0,
			tol:     0.0,
		},
		{
			name:    "single sample — zero CV",
			samples: []float64{14.0},
			wantCV:  0.0,
			tol:     0.0,
		},
		{
			name:    "zero mean — zero CV (no division by zero)",
			samples: []float64{0.0, 0.0, 0.0},
			wantCV:  0.0,
			tol:     0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cv := ComputeBreathingRegularity(tt.samples)
			if math.Abs(cv-tt.wantCV) > tt.tol {
				t.Errorf("ComputeBreathingRegularity() = %.4f, want %.4f ± %.4f", cv, tt.wantCV, tt.tol)
			}
		})
	}
}

func TestBreathingRegularityLabel(t *testing.T) {
	tests := []struct {
		cv      float64
		want    string
	}{
		{0.05, "regular"},
		{0.09, "regular"},
		{0.10, "normal"},  // boundary
		{0.15, "normal"},
		{0.25, "normal"},  // boundary
		{0.26, "irregular"},
		{0.50, "irregular"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := BreathingRegularityLabel(tt.cv)
			if got != tt.want {
				t.Errorf("BreathingRegularityLabel(%.2f) = %q, want %q", tt.cv, got, tt.want)
			}
		})
	}
}
