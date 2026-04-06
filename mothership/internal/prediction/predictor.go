// Package prediction provides presence prediction using transition probability models.
package prediction

import (
	"log"
	"sync"
	"time"
)

// MinimumDataAge is the minimum age of data required for predictions (7 days).
const MinimumDataAge = 7 * 24 * time.Hour

// MinimumSamplesPerSlot is the minimum number of samples per slot for a prediction.
const MinimumSamplesPerSlot = 3

// ZoneInfoProvider provides zone information for predictions.
type ZoneInfoProvider interface {
	GetZone(id string) (name string, ok bool)
}

// PersonInfoProvider provides person information for predictions.
type PersonInfoProvider interface {
	GetPerson(id string) (name, color string, ok bool)
	GetAllPeople() ([]struct {
		ID    string
		Name  string
		Color string
	}, error)
}

// CurrentPositionProvider provides current person positions.
type CurrentPositionProvider interface {
	GetPersonPositions() map[string]struct {
		ZoneID    string
		EntryTime time.Time
	}
}

// MQTTClient interface for MQTT publishing.
type MQTTClient interface {
	Publish(topic string, payload []byte) error
	IsConnected() bool
}

// Predictor generates predictions for person movements.
type Predictor struct {
	mu sync.RWMutex

	store       *ModelStore
	zoneProvider    ZoneInfoProvider
	personProvider  PersonInfoProvider
	positionProvider CurrentPositionProvider
	mqttClient      MQTTClient
	mothershipID    string

	// Cached predictions, updated periodically
	predictions []PersonPrediction
	lastUpdate  time.Time
}

// NewPredictor creates a new predictor.
func NewPredictor(store *ModelStore) *Predictor {
	return &Predictor{
		store:       store,
		predictions: []PersonPrediction{},
	}
}

// SetZoneProvider sets the zone info provider.
func (p *Predictor) SetZoneProvider(provider ZoneInfoProvider) {
	p.mu.Lock()
	p.zoneProvider = provider
	p.mu.Unlock()
}

// SetPersonProvider sets the person info provider.
func (p *Predictor) SetPersonProvider(provider PersonInfoProvider) {
	p.mu.Lock()
	p.personProvider = provider
	p.mu.Unlock()
}

// SetPositionProvider sets the current position provider.
func (p *Predictor) SetPositionProvider(provider CurrentPositionProvider) {
	p.mu.Lock()
	p.positionProvider = provider
	p.mu.Unlock()
}

// SetMQTTClient sets the MQTT client for publishing predictions.
func (p *Predictor) SetMQTTClient(client MQTTClient, mothershipID string) {
	p.mu.Lock()
	p.mqttClient = client
	p.mothershipID = mothershipID
	p.mu.Unlock()
}

// GetPredictions returns current predictions for all tracked people.
func (p *Predictor) GetPredictions() []PersonPrediction {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Return a copy
	result := make([]PersonPrediction, len(p.predictions))
	copy(result, p.predictions)
	return result
}

// UpdatePredictions computes new predictions based on current state.
func (p *Predictor) UpdatePredictions() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.personProvider == nil || p.positionProvider == nil {
		return
	}

	now := time.Now()
	dataAge := p.store.GetDataAge()

	// Check if we have enough data overall
	hasMinimumData := dataAge >= MinimumDataAge

	// Get all people
	people, err := p.personProvider.GetAllPeople()
	if err != nil {
		log.Printf("[WARN] prediction: failed to get people: %v", err)
		return
	}

	// Get current positions
	positions := p.positionProvider.GetPersonPositions()

	var predictions []PersonPrediction

	for _, person := range people {
		pos, exists := positions[person.ID]
		if !exists || pos.ZoneID == "" {
			continue
		}

		prediction := p.predictForPerson(person.ID, person.Name, pos.ZoneID, pos.EntryTime, now, hasMinimumData, dataAge)
		predictions = append(predictions, prediction)
	}

	p.predictions = predictions
	p.lastUpdate = now

	// Publish to MQTT
	p.publishPredictions()

	log.Printf("[INFO] prediction: updated %d predictions", len(predictions))
}

// predictForPerson generates a prediction for a single person.
func (p *Predictor) predictForPerson(personID, personName, currentZoneID string, entryTime, now time.Time, hasMinimumData bool, dataAge time.Duration) PersonPrediction {
	prediction := PersonPrediction{
		PersonID:        personID,
		PersonLabel:     personName,
		CurrentZoneID:   currentZoneID,
		DataConfidence:  "insufficient_data",
	}

	// Get zone name
	if p.zoneProvider != nil {
		prediction.CurrentZoneName, _ = p.zoneProvider.GetZone(currentZoneID)
	}

	// Calculate hour of week
	hourOfWeek := HourOfWeek(now)

	// Get sample count for this slot
	sampleCount, err := p.store.GetTransitionCountForSlot(personID, currentZoneID, hourOfWeek)
	if err != nil {
		sampleCount = 0
	}
	prediction.SampleCount = sampleCount

	// Check data confidence
	if !hasMinimumData {
		daysRemaining := int(MinimumDataAge - dataAge + 23*time.Hour) / int(24*time.Hour)
		if daysRemaining < 0 {
			daysRemaining = 0
		}
		prediction.DaysRemaining = daysRemaining
		return prediction
	}

	if sampleCount < MinimumSamplesPerSlot {
		prediction.DaysRemaining = 0 // We have the data, just not enough samples for this slot
		return prediction
	}

	// We have sufficient data
	prediction.DataConfidence = "sufficient"

	// Get transition probabilities
	probs, err := p.store.GetTransitionProbabilitiesForFromZone(personID, currentZoneID, hourOfWeek)
	if err != nil || len(probs) == 0 {
		return prediction
	}

	// Find the most likely transition
	bestProb := probs[0]
	for _, prob := range probs {
		if prob.Probability > bestProb.Probability {
			bestProb = prob
		}
	}

	prediction.PredictedNextZoneID = bestProb.ToZoneID
	prediction.PredictionConfidence = bestProb.Probability

	// Get zone name
	if p.zoneProvider != nil {
		prediction.PredictedNextZoneName, _ = p.zoneProvider.GetZone(bestProb.ToZoneID)
	}

	// Get dwell time statistics
	dwellStats, err := p.store.GetDwellTimeStats(personID, currentZoneID, hourOfWeek)
	if err != nil || dwellStats == nil || dwellStats.Count < MinimumSamplesPerSlot {
		// No dwell time data - estimate based on elapsed time
		elapsed := now.Sub(entryTime).Minutes()
		prediction.EstimatedTransitionMinutes = max(0, 15-elapsed) // Default estimate of 15 minutes
	} else {
		// Calculate expected remaining time
		elapsed := now.Sub(entryTime).Minutes()
		expectedRemaining := dwellStats.MeanMinutes - elapsed
		if expectedRemaining < 0 {
			expectedRemaining = 0
		}
		prediction.EstimatedTransitionMinutes = expectedRemaining
	}

	return prediction
}

// publishPredictions publishes predictions to MQTT.
func (p *Predictor) publishPredictions() {
	if p.mqttClient == nil || !p.mqttClient.IsConnected() {
		return
	}

	for _, pred := range p.predictions {
		topic := ""
		if p.mothershipID != "" {
			topic = "spaxel/" + p.mothershipID + "/person/" + pred.PersonID + "/predicted_zone"
		} else {
			topic = "spaxel/person/" + pred.PersonID + "/predicted_zone"
		}

		payload := map[string]interface{}{
			"zone_id":           pred.PredictedNextZoneID,
			"zone_name":         pred.PredictedNextZoneName,
			"confidence":        pred.PredictionConfidence,
			"estimated_minutes": pred.EstimatedTransitionMinutes,
			"current_zone":      pred.CurrentZoneID,
			"data_confidence":   pred.DataConfidence,
		}

		data, err := marshalJSON(payload)
		if err != nil {
			continue
		}

		if err := p.mqttClient.Publish(topic, data); err != nil {
			log.Printf("[WARN] prediction: failed to publish MQTT: %v", err)
		}
	}
}

// marshalJSON is a simple JSON marshaler.
func marshalJSON(v interface{}) ([]byte, error) {
	// Use a simple implementation
	switch val := v.(type) {
	case map[string]interface{}:
		return marshalMap(val)
	default:
		return nil, nil
	}
}

func marshalMap(m map[string]interface{}) ([]byte, error) {
	result := []byte("{")
	first := true
	for k, v := range m {
		if !first {
			result = append(result, ',')
		}
		first = false
		result = append(result, '"')
		result = append(result, k...)
		result = append(result, '"', ':')

		switch val := v.(type) {
		case string:
			result = append(result, '"')
			result = append(result, val...)
			result = append(result, '"')
		case float64:
			result = append(result, formatFloat(val)...)
		case int:
			result = append(result, formatInt(int64(val))...)
		case int64:
			result = append(result, formatInt(val)...)
		case bool:
			if val {
				result = append(result, "true"...)
			} else {
				result = append(result, "false"...)
			}
		case nil:
			result = append(result, "null"...)
		}
	}
	result = append(result, '}')
	return result, nil
}

func formatFloat(f float64) []byte {
	// Simple float formatting
	if f == float64(int64(f)) {
		return formatInt(int64(f))
	}

	intPart := int64(f)
	fracPart := f - float64(intPart)

	result := formatInt(intPart)
	result = append(result, '.')

	// Get fractional part with 6 decimal places
	fracPart *= 1000000
	frac := int64(fracPart)
	if frac < 0 {
		frac = -frac
	}

	fracStr := formatInt(frac)
	// Pad with zeros if needed
	for len(fracStr) < 6 {
		fracStr = append([]byte{'0'}, fracStr...)
	}
	// Trim trailing zeros
	for len(fracStr) > 0 && fracStr[len(fracStr)-1] == '0' {
		fracStr = fracStr[:len(fracStr)-1]
	}
	result = append(result, fracStr...)

	return result
}

func formatInt(i int64) []byte {
	if i == 0 {
		return []byte{'0'}
	}

	negative := i < 0
	if negative {
		i = -i
	}

	var digits []byte
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}

	if negative {
		digits = append([]byte{'-'}, digits...)
	}

	return digits
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
