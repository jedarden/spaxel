package signal

import (
	"math"
)

// Feature extraction constants
const (
	DefaultDeltaRMSThreshold   = 0.02 // Motion detection threshold
	NBVIWindowSize             = 100  // Samples for NBVI calculation (~5s at 20Hz)
	NBVIUpdateInterval         = 40   // Recalculate every 40 samples
	NBVIMinSamples             = 50   // Minimum samples before selection
	NBVITopCount               = 16   // Select top 16 subcarriers
	NBVIMinThreshold           = 0.001 // Minimum NBVI to be considered
	DeltaRMSSmoothingAlpha     = 0.3  // Exponential smoothing for deltaRMS
)

// NBVITracker tracks NBVI statistics for subcarrier selection
// Uses Welford's online algorithm for numerical stability
type NBVITracker struct {
	count    int
	mean     [64]float64
	m2       [64]float64 // Sum of squared deviations for Welford's algorithm
	selected [64]bool    // Selected subcarrier mask
	nSub     int
}

// NewNBVITracker creates a new NBVI tracker
func NewNBVITracker(nSub int) *NBVITracker {
	return &NBVITracker{
		nSub: nSub,
	}
}

// Update adds a new amplitude sample and updates statistics
// Uses Welford's online algorithm for numerical stability
func (n *NBVITracker) Update(amplitude []float64) {
	if len(amplitude) < n.nSub {
		return
	}

	n.count++
	for k := 0; k < n.nSub; k++ {
		// Skip non-data subcarriers
		if !IsDataSubcarrier(k) {
			continue
		}

		x := amplitude[k]
		delta := x - n.mean[k]
		n.mean[k] += delta / float64(n.count)
		delta2 := x - n.mean[k]
		n.m2[k] += delta * delta2
	}

	// Recalculate selection periodically
	if n.count >= NBVIMinSamples && n.count%NBVIUpdateInterval == 0 {
		n.recalculateSelection()
	}
}

// recalculateSelection computes NBVI scores and selects top subcarriers
func (n *NBVITracker) recalculateSelection() {
	if n.count < 2 {
		return
	}

	// Calculate NBVI for each data subcarrier
	// NBVI = variance / mean^2
	type nbviScore struct {
		idx   int
		score float64
	}

	var scores []nbviScore
	for k := 0; k < n.nSub; k++ {
		if !IsDataSubcarrier(k) {
			continue
		}

		// Variance from Welford's algorithm
		variance := n.m2[k] / float64(n.count-1)
		meanSq := n.mean[k] * n.mean[k]

		if meanSq < 1e-10 {
			continue // Skip near-zero mean
		}

		nbvi := variance / meanSq
		if nbvi >= NBVIMinThreshold {
			scores = append(scores, nbviScore{idx: k, score: nbvi})
		}
	}

	// Reset selection
	for k := range n.selected {
		n.selected[k] = false
	}

	// If not enough subcarriers pass threshold, use all data subcarriers
	if len(scores) < 8 {
		for k := 0; k < n.nSub; k++ {
			if IsDataSubcarrier(k) {
				n.selected[k] = true
			}
		}
		return
	}

	// Sort by score descending and select top N
	// Simple selection sort (fine for 47 elements)
	for i := 0; i < NBVITopCount && i < len(scores); i++ {
		maxIdx := i
		for j := i + 1; j < len(scores); j++ {
			if scores[j].score > scores[maxIdx].score {
				maxIdx = j
			}
		}
		scores[i], scores[maxIdx] = scores[maxIdx], scores[i]
		n.selected[scores[i].idx] = true
	}
}

// GetSelectedIndices returns the currently selected subcarrier indices
func (n *NBVITracker) GetSelectedIndices() []int {
	// If not enough samples, return all data subcarriers
	if n.count < NBVIMinSamples {
		return DataSubcarrierIndices(n.nSub)
	}

	var indices []int
	for k := 0; k < n.nSub; k++ {
		if n.selected[k] {
			indices = append(indices, k)
		}
	}

	// Fallback to all data subcarriers if selection is empty
	if len(indices) == 0 {
		return DataSubcarrierIndices(n.nSub)
	}

	return indices
}

// IsSelected returns whether a subcarrier is currently selected
func (n *NBVITracker) IsSelected(idx int) bool {
	if n.count < NBVIMinSamples {
		return IsDataSubcarrier(idx)
	}
	if idx < 0 || idx >= 64 {
		return false
	}
	return n.selected[idx]
}

// SampleCount returns the number of samples processed
func (n *NBVITracker) SampleCount() int {
	return n.count
}

// MotionDetector computes motion features from processed CSI
type MotionDetector struct {
	nbviTracker   *NBVITracker
	smoothDeltaRMS float64
	lastDeltaRMS   float64
	motionDetected bool
	nSub           int
}

// NewMotionDetector creates a new motion detector
func NewMotionDetector(nSub int) *MotionDetector {
	return &MotionDetector{
		nbviTracker: NewNBVITracker(nSub),
		nSub:        nSub,
	}
}

// MotionFeatures holds extracted motion features
type MotionFeatures struct {
	DeltaRMS        float64 // Raw delta RMS
	SmoothDeltaRMS  float64 // Smoothed delta RMS
	MotionDetected  bool    // True if motion above threshold
	PhaseVariance   float64 // Phase variance over selected subcarriers
	SelectedCount   int     // Number of selected subcarriers
}

// Process processes a new CSI frame and extracts motion features
func (md *MotionDetector) Process(processed *ProcessedCSI, baseline []float64) *MotionFeatures {
	// Update NBVI tracker with amplitude
	md.nbviTracker.Update(processed.Amplitude)

	// Get selected subcarrier indices
	selected := md.nbviTracker.GetSelectedIndices()

	// Compute deltaRMS over selected subcarriers
	var deltaRMS float64
	if len(selected) > 0 && len(baseline) >= md.nSub {
		var sumSqDiff float64
		for _, k := range selected {
			if k < len(processed.Amplitude) && k < len(baseline) {
				diff := processed.Amplitude[k] - baseline[k]
				sumSqDiff += diff * diff
			}
		}
		deltaRMS = math.Sqrt(sumSqDiff / float64(len(selected)))
	}

	// Apply exponential smoothing
	md.lastDeltaRMS = deltaRMS
	md.smoothDeltaRMS = DeltaRMSSmoothingAlpha*deltaRMS + (1-DeltaRMSSmoothingAlpha)*md.smoothDeltaRMS

	// Motion detection
	md.motionDetected = md.smoothDeltaRMS > DefaultDeltaRMSThreshold

	// Compute phase variance over selected subcarriers
	phaseVar := PhaseVariance(processed.ResidualPhase, selected)

	return &MotionFeatures{
		DeltaRMS:       deltaRMS,
		SmoothDeltaRMS: md.smoothDeltaRMS,
		MotionDetected: md.motionDetected,
		PhaseVariance:  phaseVar,
		SelectedCount:  len(selected),
	}
}

// IsMotionDetected returns the current motion state
func (md *MotionDetector) IsMotionDetected() bool {
	return md.motionDetected
}

// GetSmoothDeltaRMS returns the current smoothed deltaRMS
func (md *MotionDetector) GetSmoothDeltaRMS() float64 {
	return md.smoothDeltaRMS
}

// GetNBVITracker returns the NBVI tracker for diagnostics
func (md *MotionDetector) GetNBVITracker() *NBVITracker {
	return md.nbviTracker
}

// Reset resets the motion detector state
func (md *MotionDetector) Reset() {
	md.nbviTracker = NewNBVITracker(md.nSub)
	md.smoothDeltaRMS = 0
	md.lastDeltaRMS = 0
	md.motionDetected = false
}
