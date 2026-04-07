// Package sleep provides breathing anomaly detection and per-night statistics.
package sleep

import (
	"encoding/json"
	"math"
	"sync"
)

// Anomaly detection constants
const (
	BreathingAnomalyEmaAlpha  = 0.05  // Rolling personal average EMA (slow, ~20-night half-life)
	BreathingAnomalyThreshold = 1.25  // Flag if avg > personal_avg × 1.25
	BreathingRegularityRegular  = 0.10 // CV below this = regular
	BreathingRegularityIrregular = 0.25 // CV above this = irregular
)

// BreathingAnomalyTracker maintains per-person rolling averages and detects
// elevated breathing rates compared to personal baselines.
type BreathingAnomalyTracker struct {
	mu       sync.RWMutex
	personal map[string]float64 // person -> EMA of nightly avg BPM
}

// NewBreathingAnomalyTracker creates a new anomaly tracker.
func NewBreathingAnomalyTracker() *BreathingAnomalyTracker {
	return &BreathingAnomalyTracker{
		personal: make(map[string]float64),
	}
}

// UpdatePersonalAverage updates the rolling EMA for a person with the night's average BPM.
func (t *BreathingAnomalyTracker) UpdatePersonalAverage(person string, nightlyAvgBPM float64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if nightlyAvgBPM <= 0 {
		return
	}

	if existing, ok := t.personal[person]; ok && existing > 0 {
		t.personal[person] = BreathingAnomalyEmaAlpha*nightlyAvgBPM + (1-BreathingAnomalyEmaAlpha)*existing
	} else {
		t.personal[person] = nightlyAvgBPM
	}
}

// CheckAnomaly returns true if the nightly average BPM is elevated
// (>25% above the person's rolling average).
func (t *BreathingAnomalyTracker) CheckAnomaly(person string, nightlyAvgBPM float64) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	avg, ok := t.personal[person]
	if !ok || avg <= 0 {
		return false
	}

	return nightlyAvgBPM > avg*BreathingAnomalyThreshold
}

// GetPersonalAverage returns the current rolling average for a person.
func (t *BreathingAnomalyTracker) GetPersonalAverage(person string) float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.personal[person]
}

// LoadFromJSON restores personal averages from JSON.
func (t *BreathingAnomalyTracker) LoadFromJSON(data []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return json.Unmarshal(data, &t.personal)
}

// SaveToJSON serializes personal averages to JSON.
func (t *BreathingAnomalyTracker) SaveToJSON() ([]byte, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return json.Marshal(t.personal)
}

// ComputeBreathingRegularity computes the coefficient of variation (CV = std/mean)
// of a slice of breathing rate samples.
func ComputeBreathingRegularity(samples []float64) float64 {
	if len(samples) == 0 {
		return 0
	}

	var sum, sumSq float64
	for _, s := range samples {
		sum += s
		sumSq += s * s
	}

	mean := sum / float64(len(samples))
	if mean == 0 {
		return 0
	}

	variance := sumSq/float64(len(samples)) - mean*mean
	stdDev := math.Sqrt(math.Max(0, variance))

	return stdDev / mean
}

// BreathingRegularityLabel returns a human-readable label for the CV value.
func BreathingRegularityLabel(cv float64) string {
	switch {
	case cv < BreathingRegularityRegular:
		return "regular"
	case cv > BreathingRegularityIrregular:
		return "irregular"
	default:
		return "normal"
	}
}
