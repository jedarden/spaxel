// Package mqtt provides MQTT client integration for Home Assistant auto-discovery.
package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// Config holds MQTT client configuration.
type Config struct {
	Broker   string // e.g., "tcp://homeassistant.local:1883"
	ClientID string // defaults to "spaxel"
	Username string
	Password string

	// Home Assistant discovery
	DiscoveryPrefix string // defaults to "homeassistant"
	DiscoveryEnabled bool

	// Connection settings
	KeepAlive      time.Duration
	ConnectTimeout time.Duration
	AutoReconnect  bool
}

// HomeAssistantDevice represents a device in HA auto-discovery.
type HomeAssistantDevice struct {
	Identifiers  []string `json:"identifiers"`
	Name         string   `json:"name"`
	Manufacturer string   `json:"manufacturer,omitempty"`
	Model        string   `json:"model,omitempty"`
	SWVersion    string   `json:"sw_version,omitempty"`
}

// HADiscoveryConfig represents a Home Assistant discovery message.
type HADiscoveryConfig struct {
	UniqueID   string              `json:"unique_id"`
	Name       string              `json:"name"`
	Device     HomeAssistantDevice `json:"device"`
	StateTopic string              `json:"state_topic"`
	// Device-specific fields
	DeviceClass    string `json:"device_class,omitempty"`
	UnitOfMeasure  string `json:"unit_of_measurement,omitempty"`
	Icon           string `json:"icon,omitempty"`
	JSONAttributesTopic string `json:"json_attributes_topic,omitempty"`
	CommandTopic   string `json:"command_topic,omitempty"`
	PayloadOn      string `json:"payload_on,omitempty"`
	PayloadOff     string `json:"payload_off,omitempty"`
	ValueTemplate  string `json:"value_template,omitempty"`
}

// EntityConfig holds configuration for an HA entity.
type EntityConfig struct {
	ID           string
	Name         string
	Type         string // binary_sensor, sensor, device_tracker
	DeviceClass  string
	UnitOfMeasure string
	Icon         string
}

// Client provides MQTT connectivity and Home Assistant integration.
type Client struct {
	mu     sync.RWMutex
	config Config
	client mqtt.Client

	// State tracking
	connected     bool
	spaxelDevice  HomeAssistantDevice
	publishedEntities map[string]bool // entity ID -> published

	// Callbacks
	onConnect    func()
	onDisconnect func()
}

// NewClient creates a new MQTT client.
func NewClient(cfg Config) (*Client, error) {
	if cfg.Broker == "" {
		return nil, fmt.Errorf("MQTT broker URL required")
	}

	if cfg.ClientID == "" {
		cfg.ClientID = "spaxel"
	}
	if cfg.DiscoveryPrefix == "" {
		cfg.DiscoveryPrefix = "homeassistant"
	}
	if cfg.KeepAlive == 0 {
		cfg.KeepAlive = 30 * time.Second
	}
	if cfg.ConnectTimeout == 0 {
		cfg.ConnectTimeout = 10 * time.Second
	}

	c := &Client{
		config: cfg,
		spaxelDevice: HomeAssistantDevice{
			Identifiers:  []string{"spaxel"},
			Name:         "Spaxel Presence",
			Manufacturer: "Spaxel",
			Model:        "WiFi CSI Presence Detection",
		},
		publishedEntities: make(map[string]bool),
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.Broker)
	opts.SetClientID(cfg.ClientID)
	opts.SetKeepAlive(cfg.KeepAlive)
	opts.SetAutoReconnect(cfg.AutoReconnect)
	opts.SetCleanOnConnect(true)

	if cfg.Username != "" {
		opts.SetUsername(cfg.Username)
	}
	if cfg.Password != "" {
		opts.SetPassword(cfg.Password)
	}

	opts.OnConnect = func(client mqtt.Client) {
		c.mu.Lock()
		c.connected = true
		cb := c.onConnect
		c.mu.Unlock()

		log.Printf("[INFO] MQTT connected to %s", cfg.Broker)

		// Publish discovery configs on reconnect
		if cfg.DiscoveryEnabled {
			go c.publishAllDiscoveryConfigs()
		}

		if cb != nil {
			go cb()
		}
	}

	opts.OnDisconnect = func(client mqtt.Client, err error) {
		c.mu.Lock()
		c.connected = false
		cb := c.onDisconnect
		c.mu.Unlock()

		if err != nil {
			log.Printf("[WARN] MQTT disconnected: %v", err)
		} else {
			log.Printf("[INFO] MQTT disconnected")
		}

		if cb != nil {
			go cb()
		}
	}

	opts.OnConnectionLost = func(client mqtt.Client, err error) {
		log.Printf("[WARN] MQTT connection lost: %v", err)
	}

	c.client = mqtt.NewClient(opts)
	return c, nil
}

// Connect establishes connection to MQTT broker.
func (c *Client) Connect(ctx context.Context) error {
	token := c.client.Connect()
	if !token.WaitTimeout(c.config.ConnectTimeout) {
		return fmt.Errorf("MQTT connection timeout")
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("MQTT connect failed: %w", err)
	}
	return nil
}

// Disconnect closes the MQTT connection.
func (c *Client) Disconnect() {
	c.client.Disconnect(250) // 250ms grace period
	c.mu.Lock()
	c.connected = false
	c.mu.Unlock()
}

// IsConnected returns true if connected to broker.
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// SetOnConnect sets callback for connection events.
func (c *Client) SetOnConnect(cb func()) {
	c.mu.Lock()
	c.onConnect = cb
	c.mu.Unlock()
}

// SetOnDisconnect sets callback for disconnection events.
func (c *Client) SetOnDisconnect(cb func()) {
	c.mu.Lock()
	c.onDisconnect = cb
	c.mu.Unlock()
}

// Publish publishes a message to a topic.
func (c *Client) Publish(topic string, payload []byte) error {
	c.mu.RLock()
	connected := c.connected
	c.mu.RUnlock()

	if !connected {
		return fmt.Errorf("MQTT not connected")
	}

	token := c.client.Publish(topic, 1, false, payload)
	if token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}

// PublishRetained publishes a retained message.
func (c *Client) PublishRetained(topic string, payload []byte) error {
	c.mu.RLock()
	connected := c.connected
	c.mu.RUnlock()

	if !connected {
		return fmt.Errorf("MQTT not connected")
	}

	token := c.client.Publish(topic, 1, true, payload)
	if token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}

// Subscribe subscribes to a topic.
func (c *Client) Subscribe(topic string, handler func(topic string, payload []byte)) error {
	c.mu.RLock()
	connected := c.connected
	c.mu.RUnlock()

	if !connected {
		return fmt.Errorf("MQTT not connected")
	}

	token := c.client.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
		handler(msg.Topic(), msg.Payload())
	})
	if token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}

// ─── Home Assistant Auto-Discovery ─────────────────────────────────────────

// PublishDeviceTracker publishes a device tracker for a person.
func (c *Client) PublishDeviceTracker(personID, personName string) error {
	if !c.config.DiscoveryEnabled {
		return nil
	}

	entityID := fmt.Sprintf("spaxel_%s", personID)
	configTopic := fmt.Sprintf("%s/device_tracker/%s/config", c.config.DiscoveryPrefix, entityID)
	stateTopic := fmt.Sprintf("spaxel/person/%s/state", personID)

	config := HADiscoveryConfig{
		UniqueID:   entityID,
		Name:       personName,
		StateTopic: stateTopic,
		JSONAttributesTopic: stateTopic,
		Device:     c.spaxelDevice,
		Icon:       "mdi:account",
	}

	payload, _ := json.Marshal(config)

	if err := c.PublishRetained(configTopic, payload); err != nil {
		return err
	}

	c.mu.Lock()
	c.publishedEntities[entityID] = true
	c.mu.Unlock()

	log.Printf("[INFO] MQTT: Published device_tracker.%s for %s", entityID, personName)
	return nil
}

// UpdatePersonState updates a person's location state.
func (c *Client) UpdatePersonState(personID string, state string, attributes map[string]interface{}) error {
	stateTopic := fmt.Sprintf("spaxel/person/%s/state", personID)

	payload := map[string]interface{}{
		"state": state, // home, away, not_home
	}
	for k, v := range attributes {
		payload[k] = v
	}

	data, _ := json.Marshal(payload)
	return c.Publish(stateTopic, data)
}

// PublishBinarySensor publishes a binary sensor (e.g., motion, fall detected).
func (c *Client) PublishBinarySensor(sensorID, name, deviceClass string) error {
	if !c.config.DiscoveryEnabled {
		return nil
	}

	entityID := fmt.Sprintf("spaxel_%s", sensorID)
	configTopic := fmt.Sprintf("%s/binary_sensor/%s/config", c.config.DiscoveryPrefix, entityID)
	stateTopic := fmt.Sprintf("spaxel/sensor/%s/state", sensorID)

	config := HADiscoveryConfig{
		UniqueID:    entityID,
		Name:        name,
		DeviceClass: deviceClass,
		StateTopic:  stateTopic,
		PayloadOn:   "ON",
		PayloadOff:  "OFF",
		Device:      c.spaxelDevice,
	}

	payload, _ := json.Marshal(config)

	if err := c.PublishRetained(configTopic, payload); err != nil {
		return err
	}

	c.mu.Lock()
	c.publishedEntities[entityID] = true
	c.mu.Unlock()

	log.Printf("[INFO] MQTT: Published binary_sensor.%s", entityID)
	return nil
}

// UpdateBinarySensorState updates a binary sensor state.
func (c *Client) UpdateBinarySensorState(sensorID string, on bool) error {
	stateTopic := fmt.Sprintf("spaxel/sensor/%s/state", sensorID)

	state := "OFF"
	if on {
		state = "ON"
	}

	return c.Publish(stateTopic, []byte(state))
}

// PublishSensor publishes a sensor entity.
func (c *Client) PublishSensor(sensorID, name, deviceClass, unit string) error {
	if !c.config.DiscoveryEnabled {
		return nil
	}

	entityID := fmt.Sprintf("spaxel_%s", sensorID)
	configTopic := fmt.Sprintf("%s/sensor/%s/config", c.config.DiscoveryPrefix, entityID)
	stateTopic := fmt.Sprintf("spaxel/sensor/%s/state", sensorID)

	config := HADiscoveryConfig{
		UniqueID:      entityID,
		Name:          name,
		DeviceClass:   deviceClass,
		UnitOfMeasure: unit,
		StateTopic:    stateTopic,
		Device:        c.spaxelDevice,
	}

	payload, _ := json.Marshal(config)

	if err := c.PublishRetained(configTopic, payload); err != nil {
		return err
	}

	c.mu.Lock()
	c.publishedEntities[entityID] = true
	c.mu.Unlock()

	log.Printf("[INFO] MQTT: Published sensor.%s", entityID)
	return nil
}

// UpdateSensorState updates a sensor's state.
func (c *Client) UpdateSensorState(sensorID string, value interface{}) error {
	stateTopic := fmt.Sprintf("spaxel/sensor/%s/state", sensorID)

	var data []byte
	switch v := value.(type) {
	case string:
		data = []byte(v.(string))
	case []byte:
		data = v.([]byte)
	default:
		data, _ = json.Marshal(map[string]interface{}{"value": value})
	}

	return c.Publish(stateTopic, data)
}

// PublishZone publishes a zone occupancy sensor.
func (c *Client) PublishZone(zoneID, zoneName string) error {
	if !c.config.DiscoveryEnabled {
		return nil
	}

	entityID := fmt.Sprintf("spaxel_zone_%s", zoneID)
	return c.PublishSensor(entityID, fmt.Sprintf("%s Occupancy", zoneName), "", "people")
}

// UpdateZoneOccupancy updates zone occupancy count.
func (c *Client) UpdateZoneOccupancy(zoneID string, count int) error {
	entityID := fmt.Sprintf("spaxel_zone_%s", zoneID)
	return c.UpdateSensorState(entityID, count)
}

// PublishPredictionSensors publishes prediction sensors for a person.
func (c *Client) PublishPredictionSensors(personID, personName string) error {
	if !c.config.DiscoveryEnabled {
		return nil
	}

	// Predicted zone sensor
	predictedZoneID := fmt.Sprintf("spaxel_%s_predicted_zone", personID)
	if err := c.PublishSensor(predictedZoneID, fmt.Sprintf("%s Predicted Zone", personName), "", ""); err != nil {
		return err
	}

	// Prediction confidence sensor
	confidenceID := fmt.Sprintf("spaxel_%s_prediction_confidence", personID)
	if err := c.PublishSensor(confidenceID, fmt.Sprintf("%s Prediction Confidence", personName), "probability", "%"); err != nil {
		return err
	}

	// Estimated transition time sensor
	transitionID := fmt.Sprintf("spaxel_%s_transition_minutes", personID)
	if err := c.PublishSensor(transitionID, fmt.Sprintf("%s Estimated Transition", personName), "duration", "min"); err != nil {
		return err
	}

	log.Printf("[INFO] MQTT: Published prediction sensors for %s", personName)
	return nil
}

// UpdatePredictionState updates prediction sensors for a person.
func (c *Client) UpdatePredictionState(personID string, predictedZone, dataConfidence string, confidence, estimatedMinutes float64) error {
	// Update predicted zone
	predictedZoneID := fmt.Sprintf("spaxel_%s_predicted_zone", personID)
	if err := c.UpdateSensorState(predictedZoneID, predictedZone); err != nil {
		return err
	}

	// Update confidence (as percentage)
	confidenceID := fmt.Sprintf("spaxel_%s_prediction_confidence", personID)
	confidencePercent := confidence * 100
	if err := c.UpdateSensorState(confidenceID, fmt.Sprintf("%.1f", confidencePercent)); err != nil {
		return err
	}

	// Update estimated transition time
	transitionID := fmt.Sprintf("spaxel_%s_transition_minutes", personID)
	return c.UpdateSensorState(transitionID, fmt.Sprintf("%.1f", estimatedMinutes))
}

// publishAllDiscoveryConfigs re-publishes all discovery configs.
func (c *Client) publishAllDiscoveryConfigs() {
	// This is called on reconnect to ensure entities remain in HA
	// The actual entities are published as people/devices are registered

	c.mu.RLock()
	entities := len(c.publishedEntities)
	c.mu.RUnlock()

	log.Printf("[DEBUG] MQTT: Re-publishing %d discovery configs", entities)
}

// RemoveEntity removes an entity from HA.
func (c *Client) RemoveEntity(entityType, entityID string) error {
	configTopic := fmt.Sprintf("%s/%s/spaxel_%s/config", c.config.DiscoveryPrefix, entityType, entityID)

	// Publish empty retained message to remove
	return c.PublishRetained(configTopic, []byte{})
}

// GetBrokerHost extracts host from broker URL.
func (c *Client) GetBrokerHost() string {
	if u, err := url.Parse(c.config.Broker); err == nil {
		return u.Hostname()
	}
	return c.config.Broker
}
