// Package signal implements CSI signal processing algorithms
package signal

import (
	"fmt"
	"math"
)

// Constants from the plan
const (
	RSSIRefdBm = -30.0 // Reference RSSI for normalization
)

// HT20 subcarrier map (64 total)
// Null subcarriers (excluded): indices 0 (DC), 1, 63 (guard)
// Guard band (excluded): indices 27-37 (center guard + upper null carriers)
// Pilot subcarriers (excluded from NBVI): indices 7, 21, 43, 57
// Data subcarriers (eligible): all remaining = 47 subcarriers

// NullSubcarriers are subcarrier indices that are null/guard
var NullSubcarriers = map[int]bool{
	0:  true,  // DC
	1:  true,  // guard
	63: true,  // guard
}

// GuardBandSubcarriers are the center guard band indices
var GuardBandSubcarriers = map[int]bool{
	27: true, 28: true, 29: true, 30: true, 31: true, 32: true,
	33: true, 34: true, 35: true, 36: true, 37: true,
}

// PilotSubcarriers are pilot subcarrier indices
var PilotSubcarriers = map[int]bool{
	7:  true,
	21: true,
	43: true,
	57: true,
}

// IsDataSubcarrier returns true if the index is a data subcarrier
func IsDataSubcarrier(idx int) bool {
	return !NullSubcarriers[idx] && !GuardBandSubcarriers[idx] && !PilotSubcarriers[idx]
}

// DataSubcarrierIndices returns all data subcarrier indices (47 for HT20)
func DataSubcarrierIndices(nSub int) []int {
	var indices []int
	for i := 0; i < nSub; i++ {
		if IsDataSubcarrier(i) {
			indices = append(indices, i)
		}
	}
	return indices
}

// ProcessedCSI holds the result of phase sanitization
type ProcessedCSI struct {
	Amplitude     []float64 // RSSI-normalized amplitude per subcarrier
	ResidualPhase []float64 // Residual phase after STO/CFO removal per subcarrier
	RawPhase      []float64 // Unwrapped phase before regression (for diagnostics)
	STOSlope      float64   // STO slope (radians/subcarrier)
	CFOIntercept  float64   // CFO intercept (radians)
}

// PhaseSanitize performs full phase sanitization on CSI I/Q data
// Input: payload (interleaved I,Q int8 pairs), rssi_dbm
// Returns processed CSI or error if processing fails
func PhaseSanitize(payload []int8, rssiDBm int8, nSub int) (*ProcessedCSI, error) {
	if len(payload) < nSub*2 {
		return nil, fmt.Errorf("payload too short: %d bytes for %d subcarriers", len(payload), nSub)
	}
	if nSub == 0 {
		return nil, fmt.Errorf("zero subcarriers")
	}

	// Step 1: Complex CSI computation
	amplitude := make([]float64, nSub)
	phase := make([]float64, nSub)

	for k := 0; k < nSub; k++ {
		i := float64(payload[k*2])
		q := float64(payload[k*2+1])
		amplitude[k] = math.Sqrt(i*i + q*q)
		phase[k] = math.Atan2(q, i)
	}

	// Step 2: RSSI normalization (AGC compensation)
	if rssiDBm != 0 {
		norm := math.Pow(10.0, (RSSIRefdBm-float64(rssiDBm))/20.0)
		for k := 0; k < nSub; k++ {
			amplitude[k] *= norm
		}
	}

	// Step 3: Spatial phase unwrapping (across subcarriers)
	unwrapped := make([]float64, nSub)
	unwrapped[0] = phase[0]
	for k := 1; k < nSub; k++ {
		delta := phase[k] - phase[k-1]
		for delta > math.Pi {
			delta -= 2 * math.Pi
		}
		for delta < -math.Pi {
			delta += 2 * math.Pi
		}
		unwrapped[k] = unwrapped[k-1] + delta
	}

	// Step 4: Linear regression (OLS) over data subcarriers
	dataIndices := DataSubcarrierIndices(nSub)
	if len(dataIndices) < 2 {
		// Not enough data subcarriers for regression, return with zero STO/CFO
		return &ProcessedCSI{
			Amplitude:     amplitude,
			ResidualPhase: unwrapped,
			RawPhase:      unwrapped,
			STOSlope:      0,
			CFOIntercept:  0,
		}, nil
	}

	// Compute OLS: unwrapped_phase_k = a*k + b
	var sumK, sumKK, sumY, sumKY float64
	n := float64(len(dataIndices))

	for _, k := range dataIndices {
		kf := float64(k)
		y := unwrapped[k]
		sumK += kf
		sumKK += kf * kf
		sumY += y
		sumKY += kf * y
	}

	denom := n*sumKK - sumK*sumK
	if math.Abs(denom) < 1e-10 {
		// Degenerate case, skip regression
		return &ProcessedCSI{
			Amplitude:     amplitude,
			ResidualPhase: unwrapped,
			RawPhase:      unwrapped,
			STOSlope:      0,
			CFOIntercept:  0,
		}, nil
	}

	a := (n*sumKY - sumK*sumY) / denom // STO slope
	b := (sumY - a*sumK) / n           // CFO intercept

	// Step 5: Residual phase (remove STO/CFO)
	residual := make([]float64, nSub)
	for k := 0; k < nSub; k++ {
		residual[k] = unwrapped[k] - (a*float64(k) + b)
	}

	// Check for NaN/Inf
	for k := 0; k < nSub; k++ {
		if math.IsNaN(amplitude[k]) || math.IsInf(amplitude[k], 0) {
			return nil, fmt.Errorf("NaN/Inf in amplitude at subcarrier %d", k)
		}
		if math.IsNaN(residual[k]) || math.IsInf(residual[k], 0) {
			return nil, fmt.Errorf("NaN/Inf in residual phase at subcarrier %d", k)
		}
	}

	return &ProcessedCSI{
		Amplitude:     amplitude,
		ResidualPhase: residual,
		RawPhase:      unwrapped,
		STOSlope:      a,
		CFOIntercept:  b,
	}, nil
}

// MeanPhase computes the mean of residual phase over specified subcarrier indices
func MeanPhase(phase []float64, indices []int) float64 {
	if len(indices) == 0 {
		return 0
	}
	var sum float64
	for _, k := range indices {
		sum += phase[k]
	}
	return sum / float64(len(indices))
}

// PhaseVariance computes variance of phase over specified subcarrier indices
func PhaseVariance(phase []float64, indices []int) float64 {
	if len(indices) < 2 {
		return 0
	}
	mean := MeanPhase(phase, indices)
	var sumSq float64
	for _, k := range indices {
		diff := phase[k] - mean
		sumSq += diff * diff
	}
	return sumSq / float64(len(indices))
}
