// Package mqtt provides event publishing to MQTT from the internal event bus.
package mqtt

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/spaxel/mothership/internal/eventbus"
)

// EventPublisher subscribes to the internal event bus and publishes events to MQTT.
type EventPublisher struct {
	mu       sync.RWMutex
	client   *Client
	zones    map[string]string // zoneID -> zoneName
	people   map[string]string // personID -> personName
	stopped  chan struct{}
}

// NewEventPublisher creates a new MQTT event publisher.
func NewEventPublisher(client *Client) *EventPublisher {
	return &EventPublisher{
		client:  client,
		zones:   make(map[string]string),
		people:  make(map[string]string),
		stopped: make(chan struct{}),
	}
}

// Start begins subscribing to events and publishing them to MQTT.
func (p *EventPublisher) Start() {
	log.Printf("[INFO] MQTT event publisher starting")

	// Subscribe to all event types
	eventbus.SubscribeDefault(func(e eventbus.Event) {
		p.publishEvent(e)
	})
}

// Stop stops the event publisher.
func (p *EventPublisher) Stop() {
	close(p.stopped)
	log.Printf("[INFO] MQTT event publisher stopped")
}

// SetZones sets the current zone mapping.
func (p *EventPublisher) SetZones(zones map[string]string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.zones = zones
}

// SetPeople sets the current person mapping.
func (p *EventPublisher) SetPeople(people map[string]string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.people = people
}

// publishEvent publishes a single event to MQTT based on its type.
func (p *EventPublisher) publishEvent(e eventbus.Event) {
	select {
	case <-p.stopped:
		return
	default:
	}

	if !p.client.IsConnected() {
		return
	}

	switch e.Type {
	case eventbus.TypeZoneEntry:
		p.publishZoneEntry(e)
	case eventbus.TypeZoneExit:
		p.publishZoneExit(e)
	case eventbus.TypeFallAlert:
		p.publishFallAlert(e)
	case eventbus.TypeAnomaly, eventbus.TypeSecurityAlert:
		p.publishAlert(e)
	}
}

// publishZoneEntry publishes zone entry events to MQTT.
func (p *EventPublisher) publishZoneEntry(e eventbus.Event) {
	p.mu.RLock()
	zoneName := p.zones[e.Zone]
	personName := e.Person
	p.mu.RUnlock()

	// Update person presence if we have identity
	if personName != "" {
		if err := p.client.PublishPersonPresence(personName, true); err != nil {
			log.Printf("[WARN] Failed to publish person presence: %v", err)
		}
	}
}

// publishZoneExit publishes zone exit events to MQTT.
func (p *EventPublisher) publishZoneExit(e eventbus.Event) {
	p.mu.RLock()
	zoneName := p.zones[e.Zone]
	personName := e.Person
	p.mu.RUnlock()

	// Update person presence if they've left all zones
	// This is a simplified check - in production you'd track which zones
	// a person is currently in
}

// publishFallAlert publishes fall detection events to MQTT.
func (p *EventPublisher) publishFallAlert(e eventbus.Event) {
	detail, ok := e.Detail.(map[string]interface{})
	if !ok {
		return
	}

	personID, _ := detail["person_id"].(string)
	personLabel, _ := detail["person_label"].(string)
	zoneID, _ := detail["zone_id"].(string)
	zoneName := p.zones[zoneID]

	timestamp := time.Now()
	if ts, ok := detail["timestamp"].(string); ok {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			timestamp = t
		}
	}

	if err := p.client.PublishFallEvent(personID, personLabel, zoneID, zoneName, timestamp); err != nil {
		log.Printf("[WARN] Failed to publish fall event: %v", err)
	}
}

// publishAlert publishes generic alert events to MQTT.
func (p *EventPublisher) publishAlert(e eventbus.Event) {
	// Alerts are logged and can be sent to notification channels
	// MQTT publishing for generic alerts is optional
	log.Printf("[INFO] Alert event: type=%s zone=%s person=%s severity=%s",
		e.Type, e.Zone, e.Person, e.Severity)
}

// PublishSystemHealth publishes periodic system health updates to MQTT.
// This should be called on a timer (e.g., every 60 seconds).
func (p *EventPublisher) PublishSystemHealth(nodeCount, onlineCount int, detectionQuality float64, mode string) {
	if !p.client.IsConnected() {
		return
	}

	if err := p.client.PublishSystemHealth(nodeCount, onlineCount, detectionQuality, mode); err != nil {
		log.Printf("[WARN] Failed to publish system health: %v", err)
	}
}
