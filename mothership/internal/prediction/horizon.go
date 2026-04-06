// Package prediction provides presence prediction using transition probability models.
package prediction

import (
	"log"
	"math/rand"
	"sync"
	"time"
)

// HorizonPredictor provides time-horizon based predictions.
type HorizonPredictor struct {
	mu sync.RWMutex

	store           *ModelStore
	accuracyTracker *AccuracyTracker
	zoneProvider    ZoneInfoProvider
	personProvider  PersonInfoProvider
	positionProvider CurrentPositionProvider

	// Configuration
	horizon       time.Duration
	monteCarloRuns int // Number of Monte Carlo simulations

	// Random source for simulations
	rng *rand.Rand
}

// NewHorizonPredictor creates a new horizon-based predictor.
func NewHorizonPredictor(store *ModelStore, tracker *AccuracyTracker) *HorizonPredictor {
	return &HorizonPredictor{
		store:           store,
		accuracyTracker: tracker,
		horizon:         PredictionHorizon,
		monteCarloRuns:  1000,
		rng:             rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// SetZoneProvider sets the zone info provider.
func (h *HorizonPredictor) SetZoneProvider(provider ZoneInfoProvider) {
	h.mu.Lock()
	h.zoneProvider = provider
	h.mu.Unlock()
}

// SetPersonProvider sets the person info provider.
func (h *HorizonPredictor) SetPersonProvider(provider PersonInfoProvider) {
	h.mu.Lock()
	h.personProvider = provider
	h.mu.Unlock()
}

// SetPositionProvider sets the current position provider.
func (h *HorizonPredictor) SetPositionProvider(provider CurrentPositionProvider) {
	h.mu.Lock()
	h.positionProvider = provider
	h.mu.Unlock()
}

// SetHorizon sets the prediction horizon.
func (h *HorizonPredictor) SetHorizon(d time.Duration) {
	h.mu.Lock()
	h.horizon = d
	h.mu.Unlock()
}

// SetMonteCarloRuns sets the number of Monte Carlo simulations.
func (h *HorizonPredictor) SetMonteCarloRuns(n int) {
	h.mu.Lock()
	h.monteCarloRuns = n
	h.mu.Unlock()
}

// HorizonPrediction represents a prediction at a specific time horizon.
type HorizonPrediction struct {
	PersonID             string            `json:"person_id"`
	PersonLabel          string            `json:"person_label"`
	CurrentZoneID        string            `json:"current_zone_id"`
	CurrentZoneName      string            `json:"current_zone_name"`
	PredictedZoneID      string            `json:"predicted_zone_id"`
	PredictedZoneName    string            `json:"predicted_zone_name"`
	HorizonMinutes       int               `json:"horizon_minutes"`
	Confidence           float64           `json:"confidence"`
	ZoneProbabilities    map[string]float64 `json:"zone_probabilities"` // zoneID -> probability
	DataConfidence       string            `json:"data_confidence"`
	SampleCount          int               `json:"sample_count"`
	ModelReady           bool              `json:"model_ready"`
	EstimatedTransitions int               `json:"estimated_transitions"`
}

// PredictAtHorizon predicts where a person will be at the specified horizon.
// Uses Monte Carlo simulation of the Markov chain to handle multiple possible transitions.
func (h *HorizonPredictor) PredictAtHorizon(personID, currentZoneID string, horizon time.Duration) *HorizonPrediction {
	h.mu.RLock()
	defer h.mu.RUnlock()

	prediction := &HorizonPrediction{
		PersonID:          personID,
		CurrentZoneID:     currentZoneID,
		HorizonMinutes:    int(horizon.Minutes()),
		ZoneProbabilities: make(map[string]float64),
	}

	// Get zone name
	if h.zoneProvider != nil {
		prediction.CurrentZoneName, _ = h.zoneProvider.GetZone(currentZoneID)
	}

	// Get person label
	if h.personProvider != nil {
		name, _, ok := h.personProvider.GetPerson(personID)
		if ok {
			prediction.PersonLabel = name
		}
	}

	// Check data age
	dataAge := h.store.GetDataAge()
	if dataAge < MinimumDataAge {
		daysRemaining := int(MinimumDataAge-dataAge + 23*time.Hour) / int(24*time.Hour)
		if daysRemaining < 0 {
			daysRemaining = 0
		}
		prediction.DataConfidence = "insufficient_data"
		prediction.SampleCount = 0
		prediction.ModelReady = false
		return prediction
	}

	// Get sample count for current hour slot
	now := time.Now()
	hourOfWeek := HourOfWeek(now)
	sampleCount, _ := h.store.GetTransitionCountForSlot(personID, currentZoneID, hourOfWeek)
	prediction.SampleCount = sampleCount

	if sampleCount < MinimumSamplesPerSlot {
		prediction.DataConfidence = "insufficient_samples"
		prediction.ModelReady = true
		return prediction
	}

	prediction.DataConfidence = "sufficient"
	prediction.ModelReady = true

	// Run Monte Carlo simulation
	zoneCounts := h.runMonteCarlo(personID, currentZoneID, now, horizon)

	// Calculate probabilities
	total := float64(h.monteCarloRuns)
	for zoneID, count := range zoneCounts {
		prediction.ZoneProbabilities[zoneID] = float64(count) / total
	}

	// Find most likely zone
	bestZone := ""
	bestProb := 0.0
	for zoneID, prob := range prediction.ZoneProbabilities {
		if prob > bestProb {
			bestProb = prob
			bestZone = zoneID
		}
	}

	prediction.PredictedZoneID = bestZone
	prediction.Confidence = bestProb

	if h.zoneProvider != nil && bestZone != "" {
		prediction.PredictedZoneName, _ = h.zoneProvider.GetZone(bestZone)
	}

	// Record prediction for accuracy tracking
	if h.accuracyTracker != nil && bestZone != "" {
		if err := h.accuracyTracker.RecordPrediction(personID, currentZoneID, bestZone, bestProb, horizon); err != nil {
			log.Printf("[WARN] prediction: failed to record prediction: %v", err)
		}
	}

	return prediction
}

// runMonteCarlo runs Monte Carlo simulations to predict zone at horizon.
func (h *HorizonPredictor) runMonteCarlo(personID, startZone string, startTime time.Time, horizon time.Duration) map[string]int {
	zoneCounts := make(map[string]int)

	for i := 0; i < h.monteCarloRuns; i++ {
		finalZone := h.simulateOnePath(personID, startZone, startTime, horizon)
		zoneCounts[finalZone]++
	}

	return zoneCounts
}

// simulateOnePath simulates one possible path through zones.
func (h *HorizonPredictor) simulateOnePath(personID, currentZone string, currentTime time.Time, remainingHorizon time.Duration) string {
	for remainingHorizon > 0 {
		// Get dwell time for current zone
		hourOfWeek := HourOfWeek(currentTime)
		dwellStats, err := h.store.GetDwellTimeStats(personID, currentZone, hourOfWeek)

		var dwellMinutes float64
		if err != nil || dwellStats == nil || dwellStats.Count < MinimumSamplesPerSlot {
			// Default dwell time if no data
			dwellMinutes = 15.0
		} else {
			// Sample from normal distribution
			dwellMinutes = h.sampleNormal(dwellStats.MeanMinutes, dwellStats.StddevMinutes)
			if dwellMinutes < 1 {
				dwellMinutes = 1 // Minimum 1 minute
			}
		}

		dwellDuration := time.Duration(dwellMinutes * float64(time.Minute))

		// If dwell time exceeds remaining horizon, stay in current zone
		if dwellDuration >= remainingHorizon {
			return currentZone
		}

		// Transition to a new zone
		remainingHorizon -= dwellDuration
		currentTime = currentTime.Add(dwellDuration)
		hourOfWeek = HourOfWeek(currentTime)

		// Get transition probabilities
		probs, err := h.store.GetTransitionProbabilitiesForFromZone(personID, currentZone, hourOfWeek)
		if err != nil || len(probs) == 0 {
			// No transition data - stay in current zone
			return currentZone
		}

		// Sample next zone based on probabilities
		nextZone := h.sampleNextZone(probs)
		if nextZone == "" {
			return currentZone
		}

		currentZone = nextZone
	}

	return currentZone
}

// sampleNormal samples from a normal distribution using Box-Muller transform.
func (h *HorizonPredictor) sampleNormal(mean, stddev float64) float64 {
	if stddev <= 0 {
		return mean
	}

	h.mu.Lock()
	u1 := h.rng.Float64()
	u2 := h.rng.Float64()
	h.mu.Unlock()

	// Box-Muller transform
	// Avoid log(0)
	for u1 == 0 {
		h.mu.Lock()
		u1 = h.rng.Float64()
		h.mu.Unlock()
	}

	z := sqrtFast(-2 * logFast(u1)) * cosFast(2 * 3.14159265359 * u2)
	return mean + stddev*z
}

// sampleNextZone samples the next zone based on transition probabilities.
func (h *HorizonPredictor) sampleNextZone(probs []TransitionProbability) string {
	h.mu.Lock()
	r := h.rng.Float64()
	h.mu.Unlock()

	cumulative := 0.0
	for _, p := range probs {
		cumulative += p.Probability
		if r <= cumulative {
			return p.ToZoneID
		}
	}

	// Fallback to most likely
	if len(probs) > 0 {
		return probs[0].ToZoneID
	}
	return ""
}

// UpdateAllPredictions updates predictions for all tracked people.
func (h *HorizonPredictor) UpdateAllPredictions() []HorizonPrediction {
	h.mu.RLock()
	personProvider := h.personProvider
	positionProvider := h.positionProvider
	horizon := h.horizon
	h.mu.RUnlock()

	if personProvider == nil || positionProvider == nil {
		return nil
	}

	people, err := personProvider.GetAllPeople()
	if err != nil {
		log.Printf("[WARN] prediction: failed to get people: %v", err)
		return nil
	}

	positions := positionProvider.GetPersonPositions()

	var predictions []HorizonPrediction
	for _, person := range people {
		pos, exists := positions[person.ID]
		if !exists || pos.ZoneID == "" {
			continue
		}

		pred := h.PredictAtHorizon(person.ID, pos.ZoneID, horizon)
		predictions = append(predictions, *pred)
	}

	log.Printf("[INFO] prediction: updated %d horizon predictions (%dm horizon)", len(predictions), int(horizon.Minutes()))
	return predictions
}

// Helper math functions to avoid importing math package
func sqrtFast(x float64) float64 {
	if x < 0 {
		return 0
	}
	z := 1.0
	for i := 0; i < 20; i++ {
		z -= (z*z - x) / (2 * z)
	}
	return z
}

func logFast(x float64) float64 {
	if x <= 0 {
		return -1e308
	}
	// Natural log using approximation
	n := 0
	for x >= 2 {
		x /= 2
		n++
	}
	for x < 1 {
		x *= 2
		n--
	}
	// Taylor series for ln(x) around 1
	x = x - 1
	result := 0.0
	term := x
	for i := 1; i < 50; i++ {
		sign := 1.0
		if i%2 == 0 {
			sign = -1.0
		}
		result += sign * term / float64(i)
		term *= x
	}
	return result + float64(n)*0.6931471805599453 // ln(2)
}

func cosFast(x float64) float64 {
	// Taylor series for cos(x)
	// Reduce to [-pi, pi]
	for x > 3.14159265359 {
		x -= 2 * 3.14159265359
	}
	for x < -3.14159265359 {
		x += 2 * 3.14159265359
	}

	result := 1.0
	term := 1.0
	for i := 1; i < 20; i++ {
		term *= -x * x / float64(2*i*(2*i-1))
		result += term
	}
	return result
}
