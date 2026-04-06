// Package prediction provides presence prediction using transition probability models.
package prediction

import (
	"sync"
	"time"
)

// ZoneAdapter provides zone information from the zones manager.
type ZoneAdapter struct {
	mu    sync.RWMutex
	zones map[string]struct {
		Name string
	}
}

// NewZoneAdapter creates a new zone adapter.
func NewZoneAdapter() *ZoneAdapter {
	return &ZoneAdapter{
		zones: make(map[string]struct{ Name string }),
	}
}

// UpdateZone updates or adds a zone.
func (a *ZoneAdapter) UpdateZone(id, name string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.zones[id] = struct{ Name string }{Name: name}
}

// RemoveZone removes a zone.
func (a *ZoneAdapter) RemoveZone(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.zones, id)
}

// GetZone implements ZoneInfoProvider.
func (a *ZoneAdapter) GetZone(id string) (string, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if z, ok := a.zones[id]; ok {
		return z.Name, true
	}
	return "", false
}

// PersonAdapter provides person information from the BLE registry.
type PersonAdapter struct {
	mu      sync.RWMutex
	people  map[string]struct {
		Name  string
		Color string
	}
	peopleList []struct {
		ID    string
		Name  string
		Color string
	}
}

// NewPersonAdapter creates a new person adapter.
func NewPersonAdapter() *PersonAdapter {
	return &PersonAdapter{
		people: make(map[string]struct {
			Name  string
			Color string
		}),
	}
}

// UpdatePerson updates or adds a person.
func (a *PersonAdapter) UpdatePerson(id, name, color string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.people[id] = struct {
		Name  string
		Color string
	}{Name: name, Color: color}
	a.rebuildListLocked()
}

// RemovePerson removes a person.
func (a *PersonAdapter) RemovePerson(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.people, id)
	a.rebuildListLocked()
}

func (a *PersonAdapter) rebuildListLocked() {
	a.peopleList = nil
	for id, p := range a.people {
		a.peopleList = append(a.peopleList, struct {
			ID    string
			Name  string
			Color string
		}{ID: id, Name: p.Name, Color: p.Color})
	}
}

// GetPerson implements PersonInfoProvider.
func (a *PersonAdapter) GetPerson(id string) (string, string, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if p, ok := a.people[id]; ok {
		return p.Name, p.Color, true
	}
	return "", "", false
}

// GetAllPeople implements PersonInfoProvider.
func (a *PersonAdapter) GetAllPeople() ([]struct {
	ID    string
	Name  string
	Color string
}, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make([]struct {
		ID    string
		Name  string
		Color string
	}, len(a.peopleList))
	copy(result, a.peopleList)
	return result, nil
}

// PositionAdapter provides current person positions from the history updater.
type PositionAdapter struct {
	history *HistoryUpdater
}

// NewPositionAdapter creates a new position adapter.
func NewPositionAdapter(history *HistoryUpdater) *PositionAdapter {
	return &PositionAdapter{history: history}
}

// GetPersonPositions implements CurrentPositionProvider.
func (a *PositionAdapter) GetPersonPositions() map[string]struct {
	ZoneID    string
	EntryTime time.Time
} {
	if a.history == nil {
		return nil
	}

	zones := a.history.GetAllPersonZones()
	result := make(map[string]struct {
		ZoneID    string
		EntryTime time.Time
	})
	for k, v := range zones {
		result[k] = struct {
			ZoneID    string
			EntryTime time.Time
		}{
			ZoneID:    v.ZoneID,
			EntryTime: v.EntryTime,
		}
	}
	return result
}

// MQTTAdapter wraps an MQTT client for prediction publishing.
type MQTTAdapter struct {
	client       MQTTClient
	mothershipID string
}

// NewMQTTAdapter creates a new MQTT adapter.
func NewMQTTAdapter(client MQTTClient, mothershipID string) *MQTTAdapter {
	return &MQTTAdapter{
		client:       client,
		mothershipID: mothershipID,
	}
}

// PublishPrediction publishes a prediction to MQTT.
func (a *MQTTAdapter) PublishPrediction(pred PersonPrediction) error {
	if a.client == nil || !a.client.IsConnected() {
		return nil
	}

	topic := "spaxel/person/" + pred.PersonID + "/predicted_zone"
	if a.mothershipID != "" {
		topic = "spaxel/" + a.mothershipID + "/person/" + pred.PersonID + "/predicted_zone"
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
		return err
	}

	return a.client.Publish(topic, data)
}
