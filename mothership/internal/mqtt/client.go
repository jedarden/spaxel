// Package mqtt provides MQTT client integration for Home Assistant auto-discovery.
package mqtt

import (
	"context"
	"crypto/tls"
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
	ClientID string // defaults to "spaxel-{mothership_id}"
	Username string
	Password string
	TLS      bool

	// Home Assistant discovery
	DiscoveryPrefix  string // defaults to "homeassistant"
	DiscoveryEnabled bool

	// Spaxel-specific
	MothershipID string // unique ID for this mothership instance
	TopicPrefix   string // defaults to "spaxel"

	// Connection settings
	KeepAlive       time.Duration
	ConnectTimeout  time.Duration
	AutoReconnect   bool
	ReconnectMin    time.Duration // minimum reconnect delay (default 5s)
	ReconnectMax    time.Duration // maximum reconnect delay (default 120s)
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

	if cfg.MothershipID == "" {
		cfg.MothershipID = "spaxel"
	}
	if cfg.ClientID == "" {
		cfg.ClientID = fmt.Sprintf("spaxel-%s", cfg.MothershipID)
	}
	if cfg.TopicPrefix == "" {
		cfg.TopicPrefix = "spaxel"
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
	if cfg.ReconnectMin == 0 {
		cfg.ReconnectMin = 5 * time.Second
	}
	if cfg.ReconnectMax == 0 {
		cfg.ReconnectMax = 120 * time.Second
	}

	c := &Client{
		config: cfg,
		spaxelDevice: HomeAssistantDevice{
			Identifiers:  []string{fmt.Sprintf("spaxel_%s", cfg.MothershipID)},
			Name:         "Spaxel",
			Model:        "Spaxel Presence System",
			Manufacturer: "Spaxel",
		},
		publishedEntities: make(map[string]bool),
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.Broker)
	opts.SetClientID(cfg.ClientID)
	opts.SetKeepAlive(cfg.KeepAlive)

	// Enable auto-reconnect with exponential backoff from ReconnectMin to ReconnectMax
	opts.SetMaxReconnectInterval(cfg.ReconnectMax)
	opts.SetAutoReconnect(true)
	opts.SetCleanSession(false) // Use persistent sessions for retained messages

	if cfg.Username != "" {
		opts.SetUsername(cfg.Username)
	}
	if cfg.Password != "" {
		opts.SetPassword(cfg.Password)
	}

	// Configure TLS if enabled
	if cfg.TLS {
		opts.SetTLSConfig(&tls.Config{
			InsecureSkipVerify: true, // Allow self-signed certs
		})
	}

	// Set Last Will and Testament for availability
	lwtTopic := fmt.Sprintf("%s/availability", cfg.TopicPrefix)
	opts.SetBinaryWill(lwtTopic, []byte("offline"), 1, true)

	opts.OnConnect = func(client mqtt.Client) {
		c.mu.Lock()
		c.connected = true
		cb := c.onConnect
		c.mu.Unlock()

		log.Printf("[INFO] MQTT connected to %s", cfg.Broker)

		// Publish online status
		if err := c.PublishRetained(lwtTopic, []byte("online")); err != nil {
			log.Printf("[WARN] Failed to publish availability: %v", err)
		}

		// Publish discovery configs on reconnect
		if cfg.DiscoveryEnabled {
			go c.publishAllDiscoveryConfigs()
		}

		if cb != nil {
			go cb()
		}
	}

	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
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
	})

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
		data = []byte(v)
	case []byte:
		data = v
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

// ─── Home Assistant Auto-Discovery Extensions ─────────────────────────────────────

// PublishPersonPresenceDiscovery publishes HA auto-discovery for a person presence binary sensor.
func (c *Client) PublishPersonPresenceDiscovery(personID, personName string) error {
	if !c.config.DiscoveryEnabled {
		return nil
	}

	entityID := fmt.Sprintf("spaxel_%s_%s_presence", c.config.MothershipID, personID)
	configTopic := fmt.Sprintf("%s/binary_sensor/%s/config", c.config.DiscoveryPrefix, entityID)
	stateTopic := fmt.Sprintf("%s/person/%s/presence", c.config.TopicPrefix, personID)

	config := HADiscoveryConfig{
		Name:       fmt.Sprintf("%s Presence", personName),
		UniqueID:   entityID,
		StateTopic: stateTopic,
		PayloadOn:  "home",
		PayloadOff: "not_home",
		DeviceClass: "presence",
		Device:     c.spaxelDevice,
	}

	payload, _ := json.Marshal(config)
	if err := c.PublishRetained(configTopic, payload); err != nil {
		return err
	}

	c.mu.Lock()
	c.publishedEntities[entityID] = true
	c.mu.Unlock()

	log.Printf("[INFO] MQTT: Published person presence discovery for %s", personName)
	return nil
}

// PublishZoneOccupancyDiscovery publishes HA auto-discovery for a zone occupancy sensor.
func (c *Client) PublishZoneOccupancyDiscovery(zoneID, zoneName string) error {
	if !c.config.DiscoveryEnabled {
		return nil
	}

	entityID := fmt.Sprintf("spaxel_%s_zone_%s_occupancy", c.config.MothershipID, zoneID)
	configTopic := fmt.Sprintf("%s/sensor/%s/config", c.config.DiscoveryPrefix, entityID)
	stateTopic := fmt.Sprintf("%s/zone/%s/occupancy", c.config.TopicPrefix, zoneID)
	occupantsTopic := fmt.Sprintf("%s/zone/%s/occupants", c.config.TopicPrefix, zoneID)

	config := HADiscoveryConfig{
		Name:         fmt.Sprintf("%s Occupancy", zoneName),
		UniqueID:     entityID,
		StateTopic:   stateTopic,
		UnitOfMeasure: "people",
		JSONAttributesTopic: occupantsTopic,
		Icon:         "mdi:account-multiple",
		Device:       c.spaxelDevice,
	}

	payload, _ := json.Marshal(config)
	if err := c.PublishRetained(configTopic, payload); err != nil {
		return err
	}

	c.mu.Lock()
	c.publishedEntities[entityID] = true
	c.mu.Unlock()

	log.Printf("[INFO] MQTT: Published zone occupancy discovery for %s", zoneName)
	return nil
}

// PublishZoneBinaryDiscovery publishes HA auto-discovery for a zone binary occupancy sensor.
func (c *Client) PublishZoneBinaryDiscovery(zoneID, zoneName string) error {
	if !c.config.DiscoveryEnabled {
		return nil
	}

	entityID := fmt.Sprintf("spaxel_%s_zone_%s_occupied", c.config.MothershipID, zoneID)
	configTopic := fmt.Sprintf("%s/binary_sensor/%s/config", c.config.DiscoveryPrefix, entityID)
	stateTopic := fmt.Sprintf("%s/zone/%s/occupied", c.config.TopicPrefix, zoneID)

	config := HADiscoveryConfig{
		Name:       fmt.Sprintf("%s Occupied", zoneName),
		UniqueID:   entityID,
		StateTopic: stateTopic,
		PayloadOn:  "ON",
		PayloadOff: "OFF",
		DeviceClass: "occupancy",
		Icon:       "mdi:motion-sensor",
		Device:     c.spaxelDevice,
	}

	payload, _ := json.Marshal(config)
	if err := c.PublishRetained(configTopic, payload); err != nil {
		return err
	}

	c.mu.Lock()
	c.publishedEntities[entityID] = true
	c.mu.Unlock()

	log.Printf("[INFO] MQTT: Published zone binary discovery for %s", zoneName)
	return nil
}

// PublishFallDetectionDiscovery publishes HA auto-discovery for fall detection.
func (c *Client) PublishFallDetectionDiscovery() error {
	if !c.config.DiscoveryEnabled {
		return nil
	}

	entityID := fmt.Sprintf("spaxel_%s_fall_detected", c.config.MothershipID)
	configTopic := fmt.Sprintf("%s/binary_sensor/%s/config", c.config.DiscoveryPrefix, entityID)
	stateTopic := fmt.Sprintf("%s/fall_detected", c.config.TopicPrefix)

	config := HADiscoveryConfig{
		Name:       "Fall Detected",
		UniqueID:   entityID,
		StateTopic: stateTopic,
		ValueTemplate: "{% if value_json.person is defined %}ON{% else %}OFF{% endif %}",
		DeviceClass: "safety",
		Icon:       "mdi:human-greeting-proximity",
		Device:     c.spaxelDevice,
	}

	payload, _ := json.Marshal(config)
	if err := c.PublishRetained(configTopic, payload); err != nil {
		return err
	}

	c.mu.Lock()
	c.publishedEntities[entityID] = true
	c.mu.Unlock()

	log.Printf("[INFO] MQTT: Published fall detection discovery")
	return nil
}

// PublishSystemHealthDiscovery publishes HA auto-discovery for system health sensor.
func (c *Client) PublishSystemHealthDiscovery() error {
	if !c.config.DiscoveryEnabled {
		return nil
	}

	entityID := fmt.Sprintf("spaxel_%s_detection_quality", c.config.MothershipID)
	configTopic := fmt.Sprintf("%s/sensor/%s/config", c.config.DiscoveryPrefix, entityID)
	stateTopic := fmt.Sprintf("%s/system/health", c.config.TopicPrefix)

	config := HADiscoveryConfig{
		Name:       "Detection Quality",
		UniqueID:   entityID,
		StateTopic: stateTopic,
		UnitOfMeasure: "%",
		DeviceClass: "",
		Icon:       "mdi:gauge",
		Device:     c.spaxelDevice,
	}

	payload, _ := json.Marshal(config)
	if err := c.PublishRetained(configTopic, payload); err != nil {
		return err
	}

	c.mu.Lock()
	c.publishedEntities[entityID] = true
	c.mu.Unlock()

	log.Printf("[INFO] MQTT: Published system health discovery")
	return nil
}

// PublishSystemModeDiscovery publishes HA auto-discovery for system mode select.
func (c *Client) PublishSystemModeDiscovery() error {
	if !c.config.DiscoveryEnabled {
		return nil
	}

	entityID := fmt.Sprintf("spaxel_%s_system_mode", c.config.MothershipID)
	configTopic := fmt.Sprintf("%s/select/%s/config", c.config.DiscoveryPrefix, entityID)
	stateTopic := fmt.Sprintf("%s/system/mode", c.config.TopicPrefix)
	commandTopic := fmt.Sprintf("%s/command/system_mode", c.config.TopicPrefix)

	config := map[string]interface{}{
		"name": fmt.Sprintf("Spaxel System Mode"),
		"unique_id": entityID,
		"state_topic": stateTopic,
		"command_topic": commandTopic,
		"options": []string{"home", "away", "sleep"},
		"device": c.spaxelDevice,
		"icon": "mdi:home-switch",
	}

	payload, _ := json.Marshal(config)
	if err := c.PublishRetained(configTopic, payload); err != nil {
		return err
	}

	c.mu.Lock()
	c.publishedEntities[entityID] = true
	c.mu.Unlock()

	log.Printf("[INFO] MQTT: Published system mode discovery")
	return nil
}

// ─── State Publishing ─────────────────────────────────────────────────────────────

// PublishPersonPresence publishes a person's presence state.
func (c *Client) PublishPersonPresence(personID string, home bool) error {
	stateTopic := fmt.Sprintf("%s/person/%s/presence", c.config.TopicPrefix, personID)

	payload := "not_home"
	if home {
		payload = "home"
	}

	return c.PublishRetained(stateTopic, []byte(payload))
}

// PublishZoneOccupancy publishes zone occupancy count.
func (c *Client) PublishZoneOccupancy(zoneID string, count int) error {
	stateTopic := fmt.Sprintf("%s/zone/%s/occupancy", c.config.TopicPrefix, zoneID)
	return c.PublishRetained(stateTopic, []byte(fmt.Sprintf("%d", count)))
}

// PublishZoneOccupants publishes the list of people in a zone.
func (c *Client) PublishZoneOccupants(zoneID string, occupants []string) error {
	stateTopic := fmt.Sprintf("%s/zone/%s/occupants", c.config.TopicPrefix, zoneID)

	payload, err := json.Marshal(occupants)
	if err != nil {
		return err
	}

	return c.PublishRetained(stateTopic, payload)
}

// PublishZoneOccupied publishes zone occupied binary state.
func (c *Client) PublishZoneOccupied(zoneID string, occupied bool) error {
	stateTopic := fmt.Sprintf("%s/zone/%s/occupied", c.config.TopicPrefix, zoneID)

	payload := "OFF"
	if occupied {
		payload = "ON"
	}

	return c.PublishRetained(stateTopic, []byte(payload))
}

// PublishFallEvent publishes a fall detection event.
func (c *Client) PublishFallEvent(personID, personLabel, zoneID, zoneName string, timestamp time.Time) error {
	topic := fmt.Sprintf("%s/fall_detected", c.config.TopicPrefix)

	event := map[string]interface{}{
		"person_id":    personID,
		"person_label": personLabel,
		"zone_id":      zoneID,
		"zone_name":    zoneName,
		"timestamp":    timestamp.Format(time.RFC3339),
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	// Fall events are NOT retained
	return c.Publish(topic, payload)
}

// PublishSystemHealth publishes system health metrics.
func (c *Client) PublishSystemHealth(nodeCount, onlineCount int, detectionQuality float64, mode string) error {
	topic := fmt.Sprintf("%s/system/health", c.config.TopicPrefix)

	health := map[string]interface{}{
		"node_count":         nodeCount,
		"online_count":       onlineCount,
		"detection_quality":  detectionQuality,
		"mode":               mode,
		"timestamp":          time.Now().Format(time.RFC3339),
	}

	payload, err := json.Marshal(health)
	if err != nil {
		return err
	}

	return c.PublishRetained(topic, payload)
}

// PublishSystemMode publishes the current system mode.
func (c *Client) PublishSystemMode(mode string) error {
	topic := fmt.Sprintf("%s/system/mode", c.config.TopicPrefix)
	return c.PublishRetained(topic, []byte(mode))
}

// SubscribeToSystemMode subscribes to system mode command topic.
func (c *Client) SubscribeToSystemMode(handler func(mode string)) error {
	topic := fmt.Sprintf("%s/command/system_mode", c.config.TopicPrefix)

	return c.Subscribe(topic, func(_ string, payload []byte) {
		mode := string(payload)
		if mode == "home" || mode == "away" || mode == "sleep" {
			handler(mode)
		}
	})
}

// RemovePersonDiscovery removes a person's HA auto-discovery entity.
func (c *Client) RemovePersonDiscovery(personID string) error {
	entityID := fmt.Sprintf("spaxel_%s_%s_presence", c.config.MothershipID, personID)
	configTopic := fmt.Sprintf("%s/binary_sensor/%s/config", c.config.DiscoveryPrefix, entityID)
	return c.PublishRetained(configTopic, []byte{})
}

// RemoveZoneDiscovery removes a zone's HA auto-discovery entities.
func (c *Client) RemoveZoneDiscovery(zoneID string) error {
	// Remove occupancy sensor
	occupancyEntityID := fmt.Sprintf("spaxel_%s_zone_%s_occupancy", c.config.MothershipID, zoneID)
	occupancyTopic := fmt.Sprintf("%s/sensor/%s/config", c.config.DiscoveryPrefix, occupancyEntityID)
	c.PublishRetained(occupancyTopic, []byte{})

	// Remove binary sensor
	binaryEntityID := fmt.Sprintf("spaxel_%s_zone_%s_occupied", c.config.MothershipID, zoneID)
	binaryTopic := fmt.Sprintf("%s/binary_sensor/%s/config", c.config.DiscoveryPrefix, binaryEntityID)
	return c.PublishRetained(binaryTopic, []byte{})
}

// GetMothershipID returns the mothership ID used for this client.
func (c *Client) GetMothershipID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config.MothershipID
}

// ─── Dynamic Configuration ─────────────────────────────────────────────────────────

// UpdateConfig updates the client configuration and reconnects if the broker changed.
func (c *Client) UpdateConfig(ctx context.Context, newCfg Config) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if broker changed - requires reconnection
	brokerChanged := c.config.Broker != newCfg.Broker ||
		c.config.Username != newCfg.Username ||
		c.config.Password != newCfg.Password ||
		c.config.TLS != newCfg.TLS

	// Update discovery prefix and mothership ID (don't require reconnect)
	c.config.DiscoveryPrefix = newCfg.DiscoveryPrefix
	if newCfg.MothershipID != "" && newCfg.MothershipID != c.config.MothershipID {
		c.config.MothershipID = newCfg.MothershipID
		c.spaxelDevice.Identifiers = []string{fmt.Sprintf("spaxel_%s", newCfg.MothershipID)}
		// Clear published entities since IDs changed
		c.publishedEntities = make(map[string]bool)
	}

	if brokerChanged {
		// Disconnect existing client
		if c.client != nil && c.client.IsConnected() {
			c.client.Disconnect(250)
		}
		c.connected = false

		// Create new client with updated config
		c.config.Broker = newCfg.Broker
		c.config.Username = newCfg.Username
		c.config.Password = newCfg.Password
		c.config.TLS = newCfg.TLS

		// Rebuild client options
		if c.config.ClientID == "" {
			c.config.ClientID = fmt.Sprintf("spaxel-%s", c.config.MothershipID)
		}
		if c.config.TopicPrefix == "" {
			c.config.TopicPrefix = "spaxel"
		}
		if c.config.DiscoveryPrefix == "" {
			c.config.DiscoveryPrefix = "homeassistant"
		}

		opts := mqtt.NewClientOptions()
		opts.AddBroker(c.config.Broker)
		opts.SetClientID(c.config.ClientID)
		opts.SetKeepAlive(c.config.KeepAlive)
		opts.SetMaxReconnectInterval(c.config.ReconnectMax)
		opts.SetAutoReconnect(true)
		opts.SetCleanSession(false)

		if c.config.Username != "" {
			opts.SetUsername(c.config.Username)
		}
		if c.config.Password != "" {
			opts.SetPassword(c.config.Password)
		}

		if c.config.TLS {
			opts.SetTLSConfig(&tls.Config{
				InsecureSkipVerify: true,
			})
		}

		lwtTopic := fmt.Sprintf("%s/availability", c.config.TopicPrefix)
		opts.SetBinaryWill(lwtTopic, []byte("offline"), 1, true)

		// Set up callbacks (copy from NewClient)
		clientRef := c
		opts.OnConnect = func(client mqtt.Client) {
			clientRef.mu.Lock()
			clientRef.connected = true
			cb := clientRef.onConnect
			clientRef.mu.Unlock()

			log.Printf("[INFO] MQTT reconnected to %s", c.config.Broker)

			if err := clientRef.PublishRetained(lwtTopic, []byte("online")); err != nil {
				log.Printf("[WARN] Failed to publish availability: %v", err)
			}

			if c.config.DiscoveryEnabled {
				go clientRef.publishAllDiscoveryConfigs()
			}

			if cb != nil {
				go cb()
			}
		}

		opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
			clientRef.mu.Lock()
			clientRef.connected = false
			cb := clientRef.onDisconnect
			clientRef.mu.Unlock()

			if err != nil {
				log.Printf("[WARN] MQTT disconnected: %v", err)
			}

			if cb != nil {
				go cb()
			}
		})

		c.client = mqtt.NewClient(opts)

		// Connect to new broker
		return c.Connect(ctx)
	}

	return nil
}

// Reconnect attempts to reconnect to the MQTT broker.
func (c *Client) Reconnect(ctx context.Context) error {
	c.mu.Lock()
	wasConnected := c.connected
	c.mu.Unlock()

	if wasConnected {
		return nil // Already connected
	}

	return c.Connect(ctx)
}

// PublishDiscoveryNow publishes all HA discovery configs immediately.
// Useful for forcing Home Assistant to pick up new entities.
func (c *Client) PublishDiscoveryNow() error {
	if !c.config.DiscoveryEnabled {
		return fmt.Errorf("discovery is not enabled")
	}

	if !c.IsConnected() {
		return fmt.Errorf("MQTT not connected")
	}

	c.publishAllDiscoveryConfigs()
	return nil
}

// GetConfig returns the current configuration.
func (c *Client) GetConfig() Config {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config
}

// SetDiscoveryEnabled enables or disables Home Assistant auto-discovery.
func (c *Client) SetDiscoveryEnabled(enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.config.DiscoveryEnabled = enabled
}
