package signal

import (
	"math"
	"testing"
)

func TestBreathingDetector_New(t *testing.T) {
	bd := NewBreathingDetector(64)
	if bd == nil {
		t.Fatal("NewBreathingDetector returned nil")
	}
	if bd.nSub != 64 {
		t.Errorf("nSub = %d, want 64", bd.nSub)
	}
}

func TestBreathingDetector_Process_MotionPresent(t *testing.T) {
	bd := NewBreathingDetector(64)

	// Create residual phase data
	phase := make([]float64, 64)

	// When motion is present, breathing should not be computed
	features := bd.Process(phase, BreathingMotionThreshold+0.01)

	if features.Computed {
		t.Error("BreathingFeatures.Computed should be false when motion is present")
	}
}

func TestBreathingDetector_Process_NoMotion(t *testing.T) {
	bd := NewBreathingDetector(64)

	// Create residual phase data - simulate low-level breathing motion
	phase := make([]float64, 64)
	for i := range phase {
		phase[i] = 0.001 * math.Sin(float64(i)*0.1)
	}

	// No motion present
	features := bd.Process(phase, BreathingMotionThreshold/2)

	if !features.Computed {
		t.Error("BreathingFeatures.Computed should be true when no motion is present")
	}
}

func TestBreathingDetector_DetectionThreshold(t *testing.T) {
	bd := NewBreathingDetector(64)

	// Create phase data that simulates breathing (low amplitude oscillation)
	phase := make([]float64, 64)

	// Process many frames with simulated breathing
	for frame := 0; frame < BreathingSustainTime*int(BreathingSampleRate)+100; frame++ {
		// Simulate breathing oscillation in the 0.1-0.5 Hz band
		// At 20 Hz sample rate, a 0.3 Hz signal has period ~67 frames
		breathingPhase := 0.01 * math.Sin(2*math.Pi*float64(frame)/67.0)
		for k := 0; k < 64; k++ {
			if IsDataSubcarrier(k) {
				phase[k] = breathingPhase
			}
		}

		// No motion
		features := bd.Process(phase, 0.0)

		// After sustain time, should detect breathing
		if frame > BreathingSustainTime*int(BreathingSampleRate) {
			if !features.Detected {
				t.Errorf("Breathing should be detected after %d frames, frame %d", BreathingSustainTime*int(BreathingSampleRate), frame)
			}
		}
	}
}

func TestBreathingDetector_NoDetectionBelowThreshold(t *testing.T) {
	bd := NewBreathingDetector(64)

	// Create phase data with very low noise (below breathing threshold)
	phase := make([]float64, 64)

	// Process many frames with very low noise
	for frame := 0; frame < BreathingRMSWindow+100; frame++ {
		for k := 0; k < 64; k++ {
			if IsDataSubcarrier(k) {
				phase[k] = 0.0001 // Very low noise
			}
		}

		features := bd.Process(phase, 0.0)

		// Should not detect breathing with such low signal
		if features.Detected {
			t.Error("Should not detect breathing with very low signal")
			break
		}
	}
}

func TestBreathingDetector_Reset(t *testing.T) {
	bd := NewBreathingDetector(64)

	// Process some data
	phase := make([]float64, 64)
	for frame := 0; frame < 100; frame++ {
		bd.Process(phase, 0.0)
	}

	// Verify state exists
	if bd.rmsCount == 0 {
		t.Error("Expected some RMS samples after processing")
	}

	// Reset
	bd.Reset()

	// Verify state cleared
	if bd.rmsCount != 0 {
		t.Errorf("rmsCount after reset = %d, want 0", bd.rmsCount)
	}
	if bd.rateCount != 0 {
		t.Errorf("rateCount after reset = %d, want 0", bd.rateCount)
	}
	if bd.detected {
		t.Error("detected should be false after reset")
	}
}

func TestBreathingDetector_BreathingRateEstimation(t *testing.T) {
	bd := NewBreathingDetector(64)

	// Simulate breathing at 12 breaths per minute = 0.2 Hz
	// Period = 5 seconds = 100 samples at 20 Hz
	targetBPM := 12.0
	period := 60.0 / targetBPM * BreathingSampleRate

	phase := make([]float64, 64)

	// Process enough frames for FFT window
	for frame := 0; frame < BreathingFFTSize+100; frame++ {
		// Generate breathing signal
		breathingPhase := 0.01 * math.Sin(2*math.Pi*float64(frame)/period)
		for k := 0; k < 64; k++ {
			if IsDataSubcarrier(k) {
				phase[k] = breathingPhase
			}
		}

		bd.Process(phase, 0.0)
	}

	rate := bd.GetBreathingRate()
	// Allow 2 BPM tolerance
	if rate < targetBPM-2 || rate > targetBPM+2 {
		t.Errorf("Breathing rate = %.1f BPM, want ~%.1f BPM", rate, targetBPM)
	}
}

func TestBreathingDetector_GetState(t *testing.T) {
	bd := NewBreathingDetector(64)

	// Initially not detected
	detected, rms, rate := bd.GetState()
	if detected {
		t.Error("Should not be detected initially")
	}
	if rms != 0 {
		t.Errorf("Initial RMS = %f, want 0", rms)
	}
	if rate != 0 {
		t.Errorf("Initial rate = %f, want 0", rate)
	}
}

func TestBreathingDetector_IsDetected(t *testing.T) {
	bd := NewBreathingDetector(64)

	if bd.IsDetected() {
		t.Error("Should not be detected initially")
	}
}

func TestBreathingDetector_GetDetectionDuration(t *testing.T) {
	bd := NewBreathingDetector(64)

	// No detection yet
	if dur := bd.GetDetectionDuration(); dur != 0 {
		t.Errorf("Duration with no detection = %v, want 0", dur)
	}

	// Process data to trigger detection
	phase := make([]float64, 64)
	for frame := 0; frame < BreathingSustainTime*int(BreathingSampleRate)+50; frame++ {
		breathingPhase := 0.01 * math.Sin(2*math.Pi*float64(frame)/67.0)
		for k := 0; k < 64; k++ {
			if IsDataSubcarrier(k) {
				phase[k] = breathingPhase
			}
		}
		bd.Process(phase, 0.0)
	}

	if !bd.IsDetected() {
		t.Fatal("Should be detected after processing")
	}

	dur := bd.GetDetectionDuration()
	if dur <= 0 {
		t.Errorf("Duration after detection = %v, want > 0", dur)
	}
}

func TestBiquadFilter(t *testing.T) {
	bd := NewBreathingDetector(64)

	// Test that the filter passes signals in the breathing band
	// and attenuates signals outside

	// DC component should be heavily attenuated
	dcInput := 1.0
	for i := 0; i < 100; i++ {
		bd.applyFilter(dcInput)
	}
	// Filter should settle near 0 for DC
	// (bandpass filter rejects DC)

	// Reset
	bd.Reset()

	// Low frequency in breathing band should pass
	// 0.3 Hz at 20 Hz sample rate = period of ~67 samples
	for i := 0; i < 200; i++ {
		input := math.Sin(2 * math.Pi * float64(i) / 67.0)
		output := bd.applyFilter(input)
		// After settling, output should be non-zero
		if i > 100 && math.Abs(output) < 1e-6 {
			t.Errorf("Filter output too low at frame %d: %f", i, output)
		}
	}
}

func TestComputeMeanPhase(t *testing.T) {
	bd := NewBreathingDetector(64)

	// All zeros
	phase := make([]float64, 64)
	mean := bd.computeMeanPhase(phase)
	if mean != 0 {
		t.Errorf("Mean of zeros = %f, want 0", mean)
	}

	// Non-zero values on data subcarriers only
	for k := 0; k < 64; k++ {
		if IsDataSubcarrier(k) {
			phase[k] = 1.0
		}
	}
	mean = bd.computeMeanPhase(phase)
	if mean != 1.0 {
		t.Errorf("Mean of ones = %f, want 1.0", mean)
	}
}

func TestComputeRMS(t *testing.T) {
	bd := NewBreathingDetector(64)

	// Empty buffer
	if rms := bd.computeRMS(); rms != 0 {
		t.Errorf("RMS of empty buffer = %f, want 0", rms)
	}

	// Fill with constant
	for i := 0; i < BreathingRMSWindow; i++ {
		bd.rmsBuffer[i] = 1.0
	}
	bd.rmsCount = BreathingRMSWindow

	if rms := bd.computeRMS(); rms != 1.0 {
		t.Errorf("RMS of all ones = %f, want 1.0", rms)
	}

	// Fill with alternating +/- 1
	for i := 0; i < BreathingRMSWindow; i++ {
		if i%2 == 0 {
			bd.rmsBuffer[i] = 1.0
		} else {
			bd.rmsBuffer[i] = -1.0
		}
	}
	bd.rmsCount = BreathingRMSWindow

	if rms := bd.computeRMS(); rms != 1.0 {
		t.Errorf("RMS of alternating +/-1 = %f, want 1.0", rms)
	}
}

func TestBreathingDetector_HealthGating(t *testing.T) {
	bd := NewBreathingDetector(64)

	// Create phase data that simulates breathing (low amplitude oscillation)
	phase := make([]float64, 64)

	// First, establish detection with good health (score >= 0.7)
	for frame := 0; frame < BreathingSustainTime*int(BreathingSampleRate)+100; frame++ {
		breathingPhase := 0.01 * math.Sin(2*math.Pi*float64(frame)/67.0)
		for k := 0; k < 64; k++ {
			if IsDataSubcarrier(k) {
				phase[k] = breathingPhase
			}
		}
		features := bd.ProcessWithHealth(phase, 0.0, 0.8) // Good health
		// After sustain time, should detect breathing
		if frame > BreathingSustainTime*int(BreathingSampleRate) {
			if !features.Detected {
				t.Errorf("Breathing should be detected with good health at frame %d", frame)
			}
			if features.HealthGated {
				t.Error("HealthGated should be false with health score 0.8")
			}
		}
	}

	// Verify detection is active
	if !bd.IsDetected() {
		t.Fatal("Breathing should be detected after good health processing")
	}

	// Now drop health below threshold (0.7) - detection should be gated off
	features := bd.ProcessWithHealth(phase, 0.0, 0.5) // Poor health (below 0.7)

	if !features.HealthGated {
		t.Error("HealthGated should be true when health score < 0.7")
	}
	if features.Computed {
		t.Error("Computed should be false when health gated")
	}
	if features.Detected {
		t.Error("Detected should be false when health gated")
	}

	// Verify internal state is reset
	if bd.IsDetected() {
		t.Error("IsDetected should return false after health gating")
	}
}

func TestBreathingDetector_HealthGatingThreshold(t *testing.T) {
	bd := NewBreathingDetector(64)
	phase := make([]float64, 64)

	tests := []struct {
		name       string
		health     float64
		wantGated  bool
		wantDetect bool
	}{
		{"health=0.9 (excellent)", 0.9, false, true},
		{"health=0.7 (threshold)", 0.7, false, true},
		{"health=0.69 (below threshold)", 0.69, true, false},
		{"health=0.5 (poor)", 0.5, true, false},
		{"health=0.0 (worst)", 0.0, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset detector for each test
			bd.Reset()

			// Process frames at the given health level
			for frame := 0; frame < BreathingSustainTime*int(BreathingSampleRate)+50; frame++ {
				breathingPhase := 0.01 * math.Sin(2*math.Pi*float64(frame)/67.0)
				for k := 0; k < 64; k++ {
					if IsDataSubcarrier(k) {
						phase[k] = breathingPhase
					}
				}
				features := bd.ProcessWithHealth(phase, 0.0, tt.health)

				if tt.wantGated {
					// Should always be gated
					if !features.HealthGated {
						t.Errorf("frame %d: expected HealthGated=true for health %f", frame, tt.health)
					}
				} else if frame > BreathingSustainTime*int(BreathingSampleRate) {
					// After sustain time, should detect if not gated
					if !features.Detected && !features.HealthGated {
						t.Errorf("frame %d: expected detection with health %f", frame, tt.health)
					}
				}
			}
		})
	}
}

func TestBreathingDetector_IsHealthGated(t *testing.T) {
	bd := NewBreathingDetector(64)
	phase := make([]float64, 64)

	// Initially not gated
	if bd.IsHealthGated() {
		t.Error("Should not be health gated initially")
	}

	// Process with low health
	bd.ProcessWithHealth(phase, 0.0, 0.5)

	if !bd.IsHealthGated() {
		t.Error("Should be health gated after processing with low health")
	}

	// Process with good health
	bd.ProcessWithHealth(phase, 0.0, 0.8)

	if bd.IsHealthGated() {
		t.Error("Should not be health gated after processing with good health")
	}
}
