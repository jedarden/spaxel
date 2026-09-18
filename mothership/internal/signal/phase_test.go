package signal

import (
	"math"
	"testing"
)

func TestPhaseSanitize_Basic(t *testing.T) {
	// Create a simple payload with 64 subcarriers
	nSub := 64
	payload := make([]int8, nSub*2)

	// Fill with simple I/Q values (I=10, Q=10 for each subcarrier)
	for k := 0; k < nSub; k++ {
		payload[k*2] = 10   // I
		payload[k*2+1] = 10 // Q
	}

	result, err := PhaseSanitize(payload, -50, nSub)
	if err != nil {
		t.Fatalf("PhaseSanitize failed: %v", err)
	}

	if len(result.Amplitude) != nSub {
		t.Errorf("Expected %d amplitude values, got %d", nSub, len(result.Amplitude))
	}

	if len(result.ResidualPhase) != nSub {
		t.Errorf("Expected %d phase values, got %d", nSub, len(result.ResidualPhase))
	}

	// Amplitude should be sqrt(10^2 + 10^2) = sqrt(200) ≈ 14.14
	// With RSSI normalization: norm = 10^((-30-(-50))/20) = 10^1 = 10
	// So amplitude should be ~141.4
	expectedAmp := math.Sqrt(200) * math.Pow(10, (-30-(-50))/20)
	for k := 0; k < nSub; k++ {
		if math.Abs(result.Amplitude[k]-expectedAmp) > 0.01 {
			t.Errorf("Subcarrier %d: expected amplitude %.2f, got %.2f", k, expectedAmp, result.Amplitude[k])
		}
	}
}

func TestPhaseSanitize_ZeroRSSI(t *testing.T) {
	// RSSI = 0 should skip normalization
	nSub := 64
	payload := make([]int8, nSub*2)

	for k := 0; k < nSub; k++ {
		payload[k*2] = 10
		payload[k*2+1] = 0
	}

	result, err := PhaseSanitize(payload, 0, nSub)
	if err != nil {
		t.Fatalf("PhaseSanitize failed: %v", err)
	}

	// Amplitude should be just sqrt(10^2 + 0) = 10 (no normalization)
	for k := 0; k < nSub; k++ {
		if math.Abs(result.Amplitude[k]-10) > 0.01 {
			t.Errorf("Subcarrier %d: expected amplitude 10, got %.2f", k, result.Amplitude[k])
		}
	}
}

func TestPhaseSanitize_PhaseUnwrapping(t *testing.T) {
	nSub := 64
	payload := make([]int8, nSub*2)

	// Create phase that wraps: Q increases to cause phase wrap
	for k := 0; k < nSub; k++ {
		// Angle = atan2(Q, I), we'll make it increase
		angle := float64(k) * 0.15 // Will cause some wrapping
		payload[k*2] = int8(math.Cos(angle) * 100)
		payload[k*2+1] = int8(math.Sin(angle) * 100)
	}

	result, err := PhaseSanitize(payload, -40, nSub)
	if err != nil {
		t.Fatalf("PhaseSanitize failed: %v", err)
	}

	// Unwrapped phase should be monotonically increasing (mostly)
	// Check that we don't have large negative jumps
	for k := 1; k < nSub; k++ {
		diff := result.RawPhase[k] - result.RawPhase[k-1]
		if diff < -math.Pi {
			t.Errorf("Phase unwrapping failed at k=%d: diff=%.2f", k, diff)
		}
	}
}

func TestPhaseSanitize_STOCFORemoval(t *testing.T) {
	nSub := 64
	payload := make([]int8, nSub*2)

	// Create a linear phase ramp (STO-like)
	slope := 0.1 // radians per subcarrier
	for k := 0; k < nSub; k++ {
		angle := slope * float64(k)
		payload[k*2] = int8(math.Cos(angle) * 100)
		payload[k*2+1] = int8(math.Sin(angle) * 100)
	}

	result, err := PhaseSanitize(payload, -40, nSub)
	if err != nil {
		t.Fatalf("PhaseSanitize failed: %v", err)
	}

	// STO slope should be close to input slope
	if math.Abs(result.STOSlope-slope) > 0.05 {
		t.Errorf("Expected STO slope ~%.2f, got %.2f", slope, result.STOSlope)
	}

	// Residual phase should be near zero (we removed the linear component)
	for k := 0; k < nSub; k++ {
		if IsDataSubcarrier(k) && math.Abs(result.ResidualPhase[k]) > 0.5 {
			t.Errorf("Subcarrier %d: residual phase too large: %.2f", k, result.ResidualPhase[k])
		}
	}
}

func TestPhaseSanitize_TooShort(t *testing.T) {
	payload := []int8{1, 2, 3}
	_, err := PhaseSanitize(payload, -40, 64)
	if err == nil {
		t.Error("Expected error for short payload")
	}
}

func TestPhaseSanitize_ZeroSubcarriers(t *testing.T) {
	payload := []int8{}
	_, err := PhaseSanitize(payload, -40, 0)
	if err == nil {
		t.Error("Expected error for zero subcarriers")
	}
}

func TestDataSubcarrierIndices(t *testing.T) {
	indices := DataSubcarrierIndices(64)

	// Should have 46 data subcarriers (64 total - 3 null - 11 guard - 4 pilot)
	// Note: plan says 47, but actual count is 46
	if len(indices) != 46 {
		t.Errorf("Expected 46 data subcarriers, got %d", len(indices))
	}

	// Check that null, guard, and pilot subcarriers are excluded
	for _, idx := range indices {
		if NullSubcarriers[idx] {
			t.Errorf("Null subcarrier %d should not be in data indices", idx)
		}
		if GuardBandSubcarriers[idx] {
			t.Errorf("Guard band subcarrier %d should not be in data indices", idx)
		}
		if PilotSubcarriers[idx] {
			t.Errorf("Pilot subcarrier %d should not be in data indices", idx)
		}
	}
}

func TestMeanPhase(t *testing.T) {
	phase := []float64{0, 1, 2, 3, 4, 5}
	indices := []int{0, 2, 4}

	mean := MeanPhase(phase, indices)
	expected := (0.0 + 2.0 + 4.0) / 3.0

	if math.Abs(mean-expected) > 0.001 {
		t.Errorf("Expected mean %.2f, got %.2f", expected, mean)
	}
}

func TestPhaseVariance(t *testing.T) {
	phase := []float64{1, 2, 3, 4, 5}
	indices := []int{0, 1, 2, 3, 4}

	variance := PhaseVariance(phase, indices)
	// Variance of [1,2,3,4,5] = 2.5 (sample variance)
	expected := 2.0 // population variance

	if math.Abs(variance-expected) > 0.01 {
		t.Errorf("Expected variance %.2f, got %.2f", expected, variance)
	}
}

func TestPhaseVariance_Empty(t *testing.T) {
	phase := []float64{1, 2, 3}
	indices := []int{}

	variance := PhaseVariance(phase, indices)
	if variance != 0 {
		t.Errorf("Expected 0 variance for empty indices, got %.2f", variance)
	}
}

// TestPhaseSanitize_DetrendedPhaseRetainsIntercept verifies the intercept-bearing
// detrended phase output. OLS residuals (ResidualPhase) sum to zero over the
// fitted data subcarriers, so any consumer averaging ResidualPhase over
// DataSubcarrierIndices gets exactly 0 — the common-mode breathing-bearing
// component is destroyed. DetrendedPhase removes only the STO slope and retains
// the CFO intercept, so its mean over the data subcarriers recovers the
// common-mode phase.
func TestPhaseSanitize_DetrendedPhaseRetainsIntercept(t *testing.T) {
	nSub := 64
	slope := 0.05    // radians per subcarrier (STO-like ramp)
	intercept := 0.7 // radians (CFO-like common mode, non-zero)
	payload := make([]int8, nSub*2)

	for k := 0; k < nSub; k++ {
		angle := slope*float64(k) + intercept
		payload[k*2] = int8(math.Cos(angle) * 100)
		payload[k*2+1] = int8(math.Sin(angle) * 100)
	}

	result, err := PhaseSanitize(payload, -40, nSub)
	if err != nil {
		t.Fatalf("PhaseSanitize failed: %v", err)
	}

	dataIndices := DataSubcarrierIndices(nSub)

	// Property of OLS with intercept: residuals sum to zero over the fitted set.
	meanResidual := MeanPhase(result.ResidualPhase, dataIndices)
	if math.Abs(meanResidual) > 1e-9 {
		t.Errorf("Expected mean ResidualPhase over data subcarriers ~= 0, got %.3e", meanResidual)
	}

	// DetrendedPhase retains the common-mode component: its mean over the
	// fitted set equals the CFO intercept (and is decidedly non-zero).
	meanDetrended := MeanPhase(result.DetrendedPhase, dataIndices)
	if math.Abs(meanDetrended-result.CFOIntercept) > 1e-9 {
		t.Errorf("Expected mean DetrendedPhase %.6f to match CFOIntercept, got %.6f",
			result.CFOIntercept, meanDetrended)
	}
	if math.Abs(meanDetrended) < 0.3 {
		t.Errorf("Expected non-zero mean DetrendedPhase (intercept=%.2f), got %.6f",
			intercept, meanDetrended)
	}

	// The recovered intercept should track the injected common-mode phase
	// (int8 quantization keeps this well within 0.05 rad).
	if math.Abs(meanDetrended-intercept) > 0.05 {
		t.Errorf("Expected mean DetrendedPhase ~= injected intercept %.2f, got %.6f",
			intercept, meanDetrended)
	}

	// Relationship: DetrendedPhase[k] - ResidualPhase[k] = CFOIntercept
	for k := 0; k < nSub; k++ {
		diff := result.DetrendedPhase[k] - result.ResidualPhase[k]
		if math.Abs(diff-result.CFOIntercept) > 1e-9 {
			t.Errorf("Subcarrier %d: DetrendedPhase-ResidualPhase = %.6f, want CFOIntercept %.6f",
				k, diff, result.CFOIntercept)
		}
		if math.IsNaN(result.DetrendedPhase[k]) || math.IsInf(result.DetrendedPhase[k], 0) {
			t.Errorf("Subcarrier %d: DetrendedPhase is NaN/Inf", k)
		}
	}

	// Length contract matches the other per-subcarrier outputs.
	if len(result.DetrendedPhase) != nSub {
		t.Errorf("Expected %d detrended phase values, got %d", nSub, len(result.DetrendedPhase))
	}
}

// TestPhaseSanitize_DetrendedPhaseDegenerateBranches verifies the degenerate
// early-return paths (fewer than 2 data subcarriers): with no regression,
// STOSlope is 0 and DetrendedPhase equals the unwrapped phase.
func TestPhaseSanitize_DetrendedPhaseDegenerateBranches(t *testing.T) {
	tests := []struct {
		name    string
		nSub    int
		payload []int8
	}{
		{
			name:    "single subcarrier (no data indices)",
			nSub:    1,
			payload: []int8{10, 5},
		},
		{
			name:    "two subcarriers (indices 0,1 are null/guard)",
			nSub:    2,
			payload: []int8{10, 5, -10, -5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := PhaseSanitize(tt.payload, -40, tt.nSub)
			if err != nil {
				t.Fatalf("PhaseSanitize failed: %v", err)
			}

			if result.STOSlope != 0 || result.CFOIntercept != 0 {
				t.Errorf("Expected zero STO/CFO on degenerate branch, got slope=%f intercept=%f",
					result.STOSlope, result.CFOIntercept)
			}
			if len(result.DetrendedPhase) != tt.nSub {
				t.Fatalf("Expected %d detrended phase values, got %d", tt.nSub, len(result.DetrendedPhase))
			}
			for k := 0; k < tt.nSub; k++ {
				if result.DetrendedPhase[k] != result.RawPhase[k] {
					t.Errorf("Subcarrier %d: DetrendedPhase %.6f != RawPhase %.6f on degenerate branch",
						k, result.DetrendedPhase[k], result.RawPhase[k])
				}
			}
		})
	}
}
