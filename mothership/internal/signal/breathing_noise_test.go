package signal

import (
	"math/rand"
	"testing"
	"time"
)

// Test false positive rate with 1000 trials per spec requirement
func TestFFTBreathingDetector_NoDetectionWithNoise(t *testing.T) {
	bd := NewFFTBreathingDetector()

	// Generate uniform random noise (no periodic component)
	falsePositives := 0
	trials := 1000

	for trial := 0; trial < trials; trial++ {
		bd.Reset()

		// Fill buffer with random noise (sigma=0.001)
		for i := 0; i < FFTBreathingBufferSize; i++ {
			noise := (rand.Float64() - 0.5) * 0.001
			bd.AddSample(noise)
		}

		result := bd.Detect()
		if result.IsBreathing {
			falsePositives++
		}
	}

	falsePositiveRate := float64(falsePositives) / float64(trials)
	t.Logf("False positive rate: %.1f%% (target < 5%%)", falsePositiveRate*100)

	// Allow up to 5% false positive rate
	if falsePositiveRate > 0.05 {
		t.Errorf("False positive rate = %.1f%%, want < 5%%", falsePositiveRate*100)
	}
}

// TestFFTBreathingDetector_NoDetectionOnConstantFloor guards against reading
// Hann-window DC leakage as breathing. A perfectly constant (pure DC)
// deltaRMS floor must not produce an in-band peak that clears the SNR
// threshold — spaxel-ce221745.
func TestFFTBreathingDetector_NoDetectionOnConstantFloor(t *testing.T) {
	floors := []float64{0.0, 0.005, 0.01, 0.025, 0.05}
	for _, floor := range floors {
		bd := NewFFTBreathingDetector()
		for i := 0; i < FFTBreathingBufferSize; i++ {
			bd.AddSample(floor)
		}
		result := bd.Detect()
		if result.IsBreathing {
			t.Errorf("floor=%g: constant deltaRMS floor read as breathing (SNR %.2f dB)",
				floor, result.PeakSNRdB)
		}
	}
}

// TestFFTBreathingDetector_NoDetectionOnStepEdge covers the transition
// buffer: a few motion samples followed by a constant floor. Mean removal
// eliminates the DC offset, but the residual step edge must still stay
// below the SNR threshold.
func TestFFTBreathingDetector_NoDetectionOnStepEdge(t *testing.T) {
	bd := NewFFTBreathingDetector()
	for i := 0; i < FFTBreathingBufferSize; i++ {
		v := 0.01
		if i < 4 {
			v = 0.10 // 2 s of motion at the 2 Hz adaptive rate
		}
		bd.AddSample(v)
	}
	result := bd.Detect()
	if result.IsBreathing {
		t.Errorf("motion->flat-floor step edge read as breathing (SNR %.2f dB)", result.PeakSNRdB)
	}
}

// TestDwellTracker_NoStationaryLiftOnFlatFloor verifies the end-to-end
// effect of the DC-leak fix: after motion stops, a link whose deltaRMS
// never oscillates must decay POSSIBLY_PRESENT -> CLEAR, never lift to
// STATIONARY_DETECTED, and must not get stuck past the stationary timeout
// (spaxel-ce221745).
func TestDwellTracker_NoStationaryLiftOnFlatFloor(t *testing.T) {
	dt := NewDwellTracker()
	now := time.Now()
	motionUntil := now.Add(2 * time.Second) // 2 s of motion at 2 Hz

	liftedToStationary := false
	var finalUpdate DwellUpdate
	for i := 0; i < 600; i++ { // 5 simulated minutes at 500 ms steps
		ts := now.Add(time.Duration(i) * 500 * time.Millisecond)
		finalUpdate = dt.Update(ts.Before(motionUntil), 0.01, 1.0, ts)
		if finalUpdate.State == DwellStationaryDetected {
			liftedToStationary = true
		}
	}

	if liftedToStationary || dt.GetState() == DwellStationaryDetected {
		t.Error("DwellTracker lifted to STATIONARY_DETECTED on a flat deltaRMS floor")
	}
	if finalUpdate.State != DwellClear {
		t.Errorf("State after 5 min on flat floor = %v, want CLEAR", finalUpdate.State)
	}
}
