// Package mqtt provides event publishing to MQTT from the internal event bus.
package mqtt

import (
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

	// Track person presence across zones
	personZones map[string]map[string]bool // personID -> set of zoneIDs they're in
	zoneOccupants map[string]map[string]bool // zoneID -> set of personIDs in zone

	// System health ticker
	healthTicker *time.Ticker
	healthDone   chan struct{}
}

// NewEventPublisher creates a new MQTT event publisher.
func NewEventPublisher(client *Client) *EventPublisher {
	return &EventPublisher{
		client:       client,
		zones:        make(map[string]string),
		people:       make(map[string]string),
		personZones:  make(map[string]map[string]bool),
		zoneOccupants: make(map[string]map[string]bool),
		stopped:      make(chan struct{}),
		healthDone:   make(chan struct{}),
	}
}

// Start begins subscribing to events and publishing them to MQTT.
func (p *EventPublisher) Start() {
	log.Printf("[INFO] MQTT event publisher starting")

	// Subscribe to all event types
	eventbus.SubscribeDefault(func(e eventbus.Event) {
		p.publishEvent(e)
	})

	// Start system health ticker (every 60 seconds)
	p.healthTicker = time.NewTicker(60 * time.Second)
	go p.healthLoop()
}

// Stop stops the event publisher.
func (p *EventPublisher) Stop() {
	close(p.stopped)
	if p.healthTicker != nil {
		p.healthTicker.Stop()
	}
	close(p.healthDone)
	log.Printf("[INFO] MQTT event publisher stopped")
}

// healthLoop publishes system health updates periodically.
func (p *EventPublisher) healthLoop() {
	for {
		select {
		case <-p.healthTicker.C:
			// System health is published via PublishSystemHealth method
			// which should be called by the main application
		case <-p.healthDone:
			return
		}
	}
}

// SetZones sets the current zone mapping.
func (p *EventPublisher) SetZones(zones map[string]string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Initialize zone occupant tracking for new zones
	for zoneID := range zones {
		if _, exists := p.zoneOccupants[zoneID]; !exists {
			p.zoneOccupants[zoneID] = make(map[string]bool)
		}
	}

	p.zones = zones
}

// SetPeople sets the current person mapping.
func (p *EventPublisher) SetPeople(people map[string]string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Initialize person zone tracking for new people
	for personID := range people {
		if _, exists := p.personZones[personID]; !exists {
			p.personZones[personID] = make(map[string]bool)
		}
	}

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
	p.mu.Lock()
	defer p.mu.Unlock()

	zoneID := e.Zone
	personName := e.Person

	// Update zone occupants tracking
	if personName != "" {
		if _, exists := p.zoneOccupants[zoneID]; !exists {
			p.zoneOccupants[zoneID] = make(map[string]bool)
		}
		if _, exists := p.personZones[personName]; !exists {
			p.personZones[personName] = make(map[string]bool)
		}

		p.zoneOccupants[zoneID][personName] = true
		p.personZones[personName][zoneID] = true

		// Publish person presence (home = in at least one zone)
		if err := p.client.PublishPersonPresence(personName, true); err != nil {
			log.Printf("[WARN] Failed to publish person presence: %v", err)
		}

		// Publish zone occupancy
		occupancy := len(p.zoneOccupants[zoneID])
		if err := p.client.PublishZoneOccupancy(zoneID, occupancy); err != nil {
			log.Printf("[WARN] Failed to publish zone occupancy: %v", err)
		}

		// Publish zone occupants list
		occupants := p.getZoneOccupantsList(zoneID)
		if err := p.client.PublishZoneOccupants(zoneID, occupants); err != nil {
			log.Printf("[WARN] Failed to publish zone occupants: %v", err)
		}

		// Publish zone occupied binary state
		if err := p.client.PublishZoneOccupied(zoneID, occupancy > 0); err != nil {
			log.Printf("[WARN] Failed to publish zone occupied: %v", err)
		}
	}
}

// publishZoneExit publishes zone exit events to MQTT.
func (p *EventPublisher) publishZoneExit(e eventbus.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()

	zoneID := e.Zone
	personName := e.Person

	if personName != "" {
		// Remove from zone occupants tracking
		if zoneOccupants, exists := p.zoneOccupants[zoneID]; exists {
			delete(zoneOccupants, personName)
		}

		// Remove zone from person's zone set
		if personZones, exists := p.personZones[personName]; exists {
			delete(personZones, zoneID)

			// Check if person is now in no zones
			if len(personZones) == 0 {
				// Person has left all zones - set to not_home
				if err := p.client.PublishPersonPresence(personName, false); err != nil {
					log.Printf("[WARN] Failed to publish person presence: %v", err)
				}
			}
		}

		// Publish zone occupancy
		occupancy := len(p.zoneOccupants[zoneID])
		if err := p.client.PublishZoneOccupancy(zoneID, occupancy); err != nil {
			log.Printf("[WARN] Failed to publish zone occupancy: %v", err)
		}

		// Publish zone occupants list
		occupants := p.getZoneOccupantsList(zoneID)
		if err := p.client.PublishZoneOccupants(zoneID, occupants); err != nil {
			log.Printf("[WARN] Failed to publish zone occupants: %v", err)
		}

		// Publish zone occupied binary state
		if err := p.client.PublishZoneOccupied(zoneID, occupancy > 0); err != nil {
			log.Printf("[WARN] Failed to publish zone occupied: %v", err)
		}
	}
}

// getZoneOccupantsList returns a sorted list of occupant names for a zone.
func (p *EventPublisher) getZoneOccupantsList(zoneID string) []string {
	occupants, exists := p.zoneOccupants[zoneID]
	if !exists {
		return []string{}
	}

	// Convert map keys to slice
	result := make([]string, 0, len(occupants))
	for personID := range occupants {
		if name, ok := p.people[personID]; ok {
			result = append(result, name)
		}
	}

	return result
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

	p.mu.RLock()
	zoneName := p.zones[zoneID]
	p.mu.RUnlock()

	timestamp := time.Now()
	if ts, ok := detail["timestamp"].(string); ok {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			timestamp = t
		}
	}

	if err := p.client.PublishFallEvent(personID, personLabel, zoneID, zoneName, timestamp); err != nil {
		log.Printf("[WARN] Failed to publish fall event: %v", err)
	}

	// Reset fall detection state after a delay (fall events are one-shot)
	go func() {
		time.Sleep(30 * time.Second)
		p.mu.RLock()
		connected := p.client.IsConnected()
		p.mu.RUnlock()

		if connected {
			// Publish empty fall event to reset sensor
			p.client.Publish("spaxel/fall_detected", []byte("{}"))
		}
	}()
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
	p.mu.RLock()
	connected := p.client.IsConnected()
	p.mu.RUnlock()

	if !connected {
		return
	}

	if err := p.client.PublishSystemHealth(nodeCount, onlineCount, detectionQuality, mode); err != nil {
		log.Printf("[WARN] Failed to publish system health: %v", err)
	}
}

// PublishPersonDiscovery publishes HA auto-discovery for a person.
func (p *EventPublisher) PublishPersonDiscovery(personID, personName string) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.client.IsConnected() {
		return nil
	}

	return p.client.PublishPersonPresenceDiscovery(personID, personName)
}

// PublishZoneDiscovery publishes HA auto-discovery for a zone.
func (p *EventPublisher) PublishZoneDiscovery(zoneID, zoneName string) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.client.IsConnected() {
		return nil
	}

	// Publish both occupancy sensor and binary sensor
	if err := p.client.PublishZoneOccupancyDiscovery(zoneID, zoneName); err != nil {
		return err
	}
	return p.client.PublishZoneBinaryDiscovery(zoneID, zoneName)
}

// PublishFallDiscovery publishes HA auto-discovery for fall detection.
func (p *EventPublisher) PublishFallDiscovery() error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.client.IsConnected() {
		return nil
	}

	return p.client.PublishFallDetectionDiscovery()
}

// PublishSystemHealthDiscovery publishes HA auto-discovery for system health.
func (p *EventPublisher) PublishSystemHealthDiscovery() error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.client.IsConnected() {
		return nil
	}

	return p.client.PublishSystemHealthDiscovery()
}

// PublishSystemModeDiscovery publishes HA auto-discovery for system mode.
func (p *EventPublisher) PublishSystemModeDiscovery() error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.client.IsConnected() {
		return nil
	}

	return p.client.PublishSystemModeDiscovery()
}

// RemovePersonDiscovery removes a person's HA auto-discovery entity.
func (p *EventPublisher) RemovePersonDiscovery(personID string) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.client.IsConnected() {
		return nil
	}

	return p.client.RemovePersonDiscovery(personID)
}

// RemoveZoneDiscovery removes a zone's HA auto-discovery entities.
func (p *EventPublisher) RemoveZoneDiscovery(zoneID string) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.client.IsConnected() {
		return nil
	}

	return p.client.RemoveZoneDiscovery(zoneID)
}

// UpdatePersonPresence directly updates a person's presence state.
// Use this when you need to manually set presence (e.g., from BLE detection).
func (p *EventPublisher) UpdatePersonPresence(personID string, home bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.client.IsConnected() {
		return
	}

	if err := p.client.PublishPersonPresence(personID, home); err != nil {
		log.Printf("[WARN] Failed to update person presence: %v", err)
	}
}

// UpdateZoneOccupancy directly updates zone occupancy.
// Use this when you need to manually set occupancy (e.g., from blob tracking).
func (p *EventPublisher) UpdateZoneOccupancy(zoneID string, count int, occupants []string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.client.IsConnected() {
		return
	}

	// Update local tracking
	p.zoneOccupants[zoneID] = make(map[string]bool)
	for _, occupant := range occupants {
		p.zoneOccupants[zoneID][occupant] = true
	}

	if err := p.client.PublishZoneOccupancy(zoneID, count); err != nil {
		log.Printf("[WARN] Failed to update zone occupancy: %v", err)
	}

	if err := p.client.PublishZoneOccupants(zoneID, occupants); err != nil {
		log.Printf("[WARN] Failed to update zone occupants: %v", err)
	}

	if err := p.client.PublishZoneOccupied(zoneID, count > 0); err != nil {
		log.Printf("[WARN] Failed to update zone occupied: %v", err)
	}
}

// PublishSystemMode publishes the current system mode.
func (p *EventPublisher) PublishSystemMode(mode string) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.client.IsConnected() {
		return
	}

	if err := p.client.PublishSystemMode(mode); err != nil {
		log.Printf("[WARN] Failed to publish system mode: %v", err)
	}
}

// SubscribeToSystemMode subscribes to system mode commands from MQTT.
// The handler will be called when HA sends a mode change command.
func (p *EventPublisher) SubscribeToSystemMode(handler func(mode string)) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.client.IsConnected() {
		return nil
	}

	return p.client.SubscribeToSystemMode(handler)
}
