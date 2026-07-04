package signal

import (
	"math"
	"testing"
)

// TestPhaseSanitizeProperty is a property-based test for phase sanitization.
// The key property is: for all valid int8 I/Q pairs, the output should
// never contain NaN or Inf values.
//
// This test exhaustively checks corner cases of int8 inputs:
// - All-zero I/Q
// - Maximum int8 values (127)
// - Minimum int8 values (-128)
// - Alternating signs
// - Typical CSI values
func TestPhaseSanitizeProperty(t *testing.T) {
	// Define test cases covering all int8 corner cases
	testCases := []struct {
		name        string
		nSub        int
		payloadGen  func(nSub int) []int8
		rssiDBm     int8
		description string
	}{
		{
			name: "all zero I/Q pairs",
			nSub: 64,
			payloadGen: func(nSub int) []int8 {
				return make([]int8, nSub*2)
			},
			rssiDBm:     -50,
			description: "All I and Q values are zero - should handle gracefully",
		},
		{
			name: "maximum positive int8 (127)",
			nSub: 64,
			payloadGen: func(nSub int) []int8 {
				payload := make([]int8, nSub*2)
				for i := range payload {
					payload[i] = 127
				}
				return payload
			},
			rssiDBm:     -50,
			description: "All I and Q at maximum int8 value",
		},
		{
			name: "maximum negative int8 (-128)",
			nSub: 64,
			payloadGen: func(nSub int) []int8 {
				payload := make([]int8, nSub*2)
				for i := range payload {
					payload[i] = -128
				}
				return payload
			},
			rssiDBm:     -50,
			description: "All I and Q at minimum int8 value",
		},
		{
			name: "alternating signs",
			nSub: 64,
			payloadGen: func(nSub int) []int8 {
				payload := make([]int8, nSub*2)
				for i := range payload {
					if i%2 == 0 {
						payload[i] = 127
					} else {
						payload[i] = -128
					}
				}
				return payload
			},
			rssiDBm:     -50,
			description: "Alternating between max positive and max negative",
		},
		{
			name: "collinear I/Q (same line through origin)",
			nSub: 64,
			payloadGen: func(nSub int) []int8 {
				// All (I,Q) samples lie on the same line through the origin
				// (phase = π/4 on one ray, -3π/4 on the opposing ray), with
				// varying magnitudes. Adjacent samples can sit at exactly ±π
				// apart — the phase-unwrap wrap boundary — which must not
				// produce NaN/Inf or panic.
				payload := make([]int8, nSub*2)
				mags := []int8{1, 10, 50, 127, 50, 10, -1, -10, -50, -127}
				for k := 0; k < nSub; k++ {
					m := mags[k%len(mags)]
					payload[k*2] = m   // I
					payload[k*2+1] = m // Q (collinear: Q == I along the diagonal)
				}
				return payload
			},
			rssiDBm:     -50,
			description: "Collinear complex samples straddling the ±π unwrap boundary",
		},
		{
			name: "I max, Q zero",
			nSub: 64,
			payloadGen: func(nSub int) []int8 {
				payload := make([]int8, nSub*2)
				for i := 0; i < nSub*2; i += 2 {
					payload[i] = 127 // I
					payload[i+1] = 0 // Q
				}
				return payload
			},
			rssiDBm:     -50,
			description: "I at max, Q at zero",
		},
		{
			name: "I zero, Q max",
			nSub: 64,
			payloadGen: func(nSub int) []int8 {
				payload := make([]int8, nSub*2)
				for i := 0; i < nSub*2; i += 2 {
					payload[i] = 0     // I
					payload[i+1] = 127 // Q
				}
				return payload
			},
			rssiDBm:     -50,
			description: "I at zero, Q at max",
		},
		{
			name: "typical CSI values",
			nSub: 64,
			payloadGen: func(nSub int) []int8 {
				payload := make([]int8, nSub*2)
				for i := range payload {
					// Typical CSI I/Q values are small (usually -50 to 50)
					payload[i] = int8(i - 64)
				}
				return payload
			},
			rssiDBm:     -50,
			description: "Typical CSI signal values",
		},
		{
			name: "mixed extreme values",
			nSub: 64,
			payloadGen: func(nSub int) []int8 {
				payload := make([]int8, nSub*2)
				extremes := []int8{-128, -127, -1, 0, 1, 126, 127}
				for i := range payload {
					payload[i] = extremes[i%len(extremes)]
				}
				return payload
			},
			rssiDBm:     -50,
			description: "Mix of all extreme int8 values",
		},
		{
			name: "single subcarrier",
			nSub: 1,
			payloadGen: func(nSub int) []int8 {
				return []int8{10, 5}
			},
			rssiDBm:     -50,
			description: "Single subcarrier (minimum for meaningful CSI)",
		},
		{
			name: "two subcarriers",
			nSub: 2,
			payloadGen: func(nSub int) []int8 {
				return []int8{10, 5, -10, -5}
			},
			rssiDBm:     -50,
			description: "Two subcarriers (minimum for regression)",
		},
		{
			name: "rssi zero (no AGC normalization)",
			nSub: 64,
			payloadGen: func(nSub int) []int8 {
				payload := make([]int8, nSub*2)
				for i := range payload {
					payload[i] = 10
				}
				return payload
			},
			rssiDBm:     0,
			description: "RSSI=0 skips normalization",
		},
		{
			name: "strong RSSI (causes large normalization)",
			nSub: 64,
			payloadGen: func(nSub int) []int8 {
				payload := make([]int8, nSub*2)
				for i := range payload {
					payload[i] = 127
				}
				return payload
			},
			rssiDBm:     -10, // Strong signal
			description: "Strong RSSI with max I/Q values",
		},
		{
			name: "weak RSSI (causes small normalization)",
			nSub: 64,
			payloadGen: func(nSub int) []int8 {
				payload := make([]int8, nSub*2)
				for i := range payload {
					payload[i] = 1
				}
				return payload
			},
			rssiDBm:     -90, // Weak signal
			description: "Weak RSSI with small I/Q values",
		},
		{
			name: "HT20 with 47 data subcarriers",
			nSub: 64,
			payloadGen: func(nSub int) []int8 {
				payload := make([]int8, nSub*2)
				for i := range payload {
					payload[i] = int8(i%20 - 10)
				}
				return payload
			},
			rssiDBm:     -50,
			description: "Full HT20 frame with typical values",
		},
		{
			name: "symmetric phase ramp",
			nSub: 64,
			payloadGen: func(nSub int) []int8 {
				// Create a phase ramp that would need unwrapping
				payload := make([]int8, nSub*2)
				for k := 0; k < nSub; k++ {
					angle := float64(k) * 0.1 // Gradual phase increase
					i := math.Cos(angle) * 50
					q := math.Sin(angle) * 50
					payload[k*2] = int8(i)
					payload[k*2+1] = int8(q)
				}
				return payload
			},
			rssiDBm:     -50,
			description: "Phase ramp that triggers unwrapping logic",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payload := tc.payloadGen(tc.nSub)

			// The key property: PhaseSanitize should never return NaN or Inf.
			// Every case in this table is a well-formed payload (nSub ≥ 1,
			// payload length matches), so the function MUST succeed and
			// produce finite output. An error here is a regression, not an
			// acceptable outcome — invalid inputs are covered separately by
			// TestPhaseSanitizeInvalidInputs.
			result, err := PhaseSanitize(payload, tc.rssiDBm, tc.nSub)
			if err != nil {
				t.Fatalf("PhaseSanitize returned unexpected error for valid edge case %q: %v",
					tc.description, err)
			}

			// Verify result is not nil
			if result == nil {
				t.Fatal("PhaseSanitize returned nil result and nil error")
			}

			// Property: No NaN in Amplitude
			for i, amp := range result.Amplitude {
				if math.IsNaN(amp) {
					t.Errorf("Amplitude[%d] is NaN (rssi=%d, nSub=%d, case: %s)",
						i, tc.rssiDBm, tc.nSub, tc.description)
				}
				if math.IsInf(amp, 0) {
					t.Errorf("Amplitude[%d] is Inf (rssi=%d, nSub=%d, case: %s)",
						i, tc.rssiDBm, tc.nSub, tc.description)
				}
			}

			// Property: No NaN in ResidualPhase
			for i, phase := range result.ResidualPhase {
				if math.IsNaN(phase) {
					t.Errorf("ResidualPhase[%d] is NaN (rssi=%d, nSub=%d, case: %s)",
						i, tc.rssiDBm, tc.nSub, tc.description)
				}
				if math.IsInf(phase, 0) {
					t.Errorf("ResidualPhase[%d] is Inf (rssi=%d, nSub=%d, case: %s)",
						i, tc.rssiDBm, tc.nSub, tc.description)
				}
			}

			// Property: STOSlope and CFOIntercept should be finite
			if math.IsNaN(result.STOSlope) {
				t.Errorf("STOSlope is NaN (rssi=%d, nSub=%d, case: %s)",
					tc.rssiDBm, tc.nSub, tc.description)
			}
			if math.IsInf(result.STOSlope, 0) {
				t.Errorf("STOSlope is Inf (rssi=%d, nSub=%d, case: %s)",
					tc.rssiDBm, tc.nSub, tc.description)
			}
			if math.IsNaN(result.CFOIntercept) {
				t.Errorf("CFOIntercept is NaN (rssi=%d, nSub=%d, case: %s)",
					tc.rssiDBm, tc.nSub, tc.description)
			}
			if math.IsInf(result.CFOIntercept, 0) {
				t.Errorf("CFOIntercept is Inf (rssi=%d, nSub=%d, case: %s)",
					tc.rssiDBm, tc.nSub, tc.description)
			}

			// Additional sanity checks
			if len(result.Amplitude) != tc.nSub {
				t.Errorf("Amplitude length mismatch: got %d, want %d",
					len(result.Amplitude), tc.nSub)
			}
			if len(result.ResidualPhase) != tc.nSub {
				t.Errorf("ResidualPhase length mismatch: got %d, want %d",
					len(result.ResidualPhase), tc.nSub)
			}

			// Amplitude should be non-negative (it's sqrt(I² + Q²))
			for i, amp := range result.Amplitude {
				if amp < 0 {
					t.Errorf("Amplitude[%d] is negative: %f", i, amp)
				}
			}

			t.Logf("Property test passed: %s (rssi=%d, nSub=%d)",
				tc.description, tc.rssiDBm, tc.nSub)
		})
	}
}

// TestPhaseSanitizeInvalidInputs tests that invalid inputs return proper errors
func TestPhaseSanitizeInvalidInputs(t *testing.T) {
	tests := []struct {
		name        string
		payload     []int8
		rssiDBm     int8
		nSub        int
		wantErr     bool
		errContains string
	}{
		{
			name:        "payload too short",
			payload:     []int8{1, 2, 3},
			rssiDBm:     -50,
			nSub:        64,
			wantErr:     true,
			errContains: "too short",
		},
		{
			name:        "zero subcarriers",
			payload:     []int8{},
			rssiDBm:     -50,
			nSub:        0,
			wantErr:     true,
			errContains: "zero subcarriers",
		},
		{
			name:        "nSub larger than payload",
			payload:     []int8{1, 2, 3, 4},
			rssiDBm:     -50,
			nSub:        10,
			wantErr:     true,
			errContains: "too short",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := PhaseSanitize(tt.payload, tt.rssiDBm, tt.nSub)
			if (err != nil) != tt.wantErr {
				t.Errorf("PhaseSanitize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && result != nil {
				t.Error("PhaseSanitize() returned non-nil result for error case")
			}
			if tt.errContains != "" && err != nil {
				if !containsString(err.Error(), tt.errContains) {
					t.Errorf("Error should contain %q, got %q", tt.errContains, err.Error())
				}
			}
		})
	}
}

// TestPhaseSanitizeRegressionDenomZero tests the degenerate case where
// the OLS regression denominator is zero (collinear subcarriers)
func TestPhaseSanitizeRegressionDenomZero(t *testing.T) {
	// Create a payload where all data subcarriers have the same value
	// This results in constant phase, which should produce slope ~ 0
	nSub := 64
	payload := make([]int8, nSub*2)
	for i := range payload {
		payload[i] = 50 // All identical values
	}

	result, err := PhaseSanitize(payload, -50, nSub)
	if err != nil {
		t.Fatalf("PhaseSanitize failed: %v", err)
	}

	// With constant I/Q values, phase is constant (atan2(50,50) = π/4)
	// STO slope should be very close to 0 (no phase ramp)
	if math.Abs(result.STOSlope) > 1e-10 {
		t.Errorf("Expected STOSlope≈0 for constant phase, got %f", result.STOSlope)
	}
	// CFO intercept should be π/4 (the constant phase value)
	expectedCFO := math.Pi / 4
	if math.Abs(result.CFOIntercept-expectedCFO) > 0.01 {
		t.Errorf("Expected CFOIntercept≈%f for constant phase, got %f", expectedCFO, result.CFOIntercept)
	}

	// Most importantly: we should still have valid output without NaN/Inf
	for i, amp := range result.Amplitude {
		if math.IsNaN(amp) || math.IsInf(amp, 0) {
			t.Errorf("Amplitude[%d] is NaN/Inf in degenerate case", i)
		}
	}
	for i, phase := range result.ResidualPhase {
		if math.IsNaN(phase) || math.IsInf(phase, 0) {
			t.Errorf("ResidualPhase[%d] is NaN/Inf in degenerate case", i)
		}
	}
}

func containsString(s, substr string) bool {
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
