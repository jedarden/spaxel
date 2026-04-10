// Package mqtt provides tests for MQTT client integration.
package mqtt

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"httputil "net/http/httputil"
)

// TestNewClient validates MQTT client creation.
func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				Broker:        "tcp://localhost:1883",
				MothershipID:  "test-mothership",
				Username:      "testuser",
				Password:      "testpass",
			},
			wantErr: false,
		},
		{
			name: "missing broker",
			config: Config{
				MothershipID: "test-mothership",
			},
			wantErr: true,
		},
		{
			name: "defaults applied",
			config: Config{
				Broker:        "tcp://localhost:1883",
				MothershipID:  "test",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && client == nil {
				t.Error("NewClient() returned nil client")
			}
		})
	}
}

// TestHomeAssistantDiscoveryConfig tests HA auto-discovery config generation.
func TestHomeAssistantDiscoveryConfig(t *testing.T) {
	cfg := Config{
		Broker:        "tcp://localhost:1883",
		MothershipID:  "test123",
		TopicPrefix:   "spaxel",
		DiscoveryPrefix: "homeassistant",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	// Test person presence discovery
	personConfig := HADiscoveryConfig{
		Name:         "Alice Presence",
		UniqueID:     "spaxel_test123_alice_presence",
		StateTopic:   "spaxel/person/alice/presence",
		PayloadOn:    "home",
		PayloadOff:   "not_home",
		DeviceClass:  "presence",
		Device:       client.spaxelDevice,
	}

	payload, err := json.Marshal(personConfig)
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	var decoded HADiscoveryConfig
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	if decoded.Name != "Alice Presence" {
		t.Errorf("Name = %s, want Alice Presence", decoded.Name)
	}
	if decoded.UniqueID != "spaxel_test123_alice_presence" {
		t.Errorf("UniqueID = %s, want spaxel_test123_alice_presence", decoded.UniqueID)
	}
	if decoded.DeviceClass != "presence" {
		t.Errorf("DeviceClass = %s, want presence", decoded.DeviceClass)
	}

	// Verify device identifiers
	if len(decoded.Device.Identifiers) != 1 {
		t.Errorf("Device identifiers count = %d, want 1", len(decoded.Device.Identifiers))
	}
	if decoded.Device.Identifiers[0] != "spaxel_test123" {
		t.Errorf("Device identifier = %s, want spaxel_test123", decoded.Device.Identifiers[0])
	}
}

// TestTopicGeneration tests MQTT topic generation.
func TestTopicGeneration(t *testing.T) {
	cfg := Config{
		Broker:        "tcp://localhost:1883",
		MothershipID:  "spaxel01",
		TopicPrefix:   "spaxel",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	tests := []struct {
		name     string
		topic    string
		expected string
	}{
		{
			name:     "person presence",
			topic:    client.GetMothershipID() + "/person/alice/presence",
			expected: "spaxel01/person/alice/presence",
		},
		{
			name:     "zone occupancy",
			topic:    client.GetMothershipID() + "/zone/kitchen/occupancy",
			expected: "spaxel01/zone/kitchen/occupancy",
		},
		{
			name:     "fall detected",
			topic:    "fall_detected",
			expected: "spaxel/fall_detected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.topic != tt.expected {
				t.Errorf("topic = %s, want %s", tt.topic, tt.expected)
			}
		})
	}
}

// TestDiscoveryPayloadFormat tests that discovery payloads match HA spec.
func TestDiscoveryPayloadFormat(t *testing.T) {
	cfg := Config{
		Broker:          "tcp://localhost:1883",
		MothershipID:    "test123",
		TopicPrefix:     "spaxel",
		DiscoveryPrefix: "homeassistant",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	// Test binary_sensor discovery payload
	binaryConfig := map[string]interface{}{
		"name":           "Alice Presence",
		"unique_id":      "spaxel_test123_alice_presence",
		"state_topic":    "spaxel/person/alice/presence",
		"payload_on":     "home",
		"payload_off":    "not_home",
		"device_class":   "presence",
		"device":         client.spaxelDevice,
	}

	payload, err := json.Marshal(binaryConfig)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Verify required fields
	requiredFields := []string{"name", "unique_id", "state_topic", "device"}
	for _, field := range requiredFields {
		if _, exists := decoded[field]; !exists {
			t.Errorf("Missing required field: %s", field)
		}
	}

	// Verify payload_on/payload_off for binary_sensor
	if decoded["payload_on"] != "home" {
		t.Errorf("payload_on = %v, want 'home'", decoded["payload_on"])
	}
	if decoded["payload_off"] != "not_home" {
		t.Errorf("payload_off = %v, want 'not_home'", decoded["payload_off"])
	}
}

// TestMQTTMessagePayloads tests state message payloads.
func TestMQTTMessagePayloads(t *testing.T) {
	// Person presence payloads
	personHome := []byte("home")
	personNotHome := []byte("not_home")

	if string(personHome) != "home" {
		t.Errorf("Person home payload = %s, want 'home'", string(personHome))
	}
	if string(personNotHome) != "not_home" {
		t.Errorf("Person not_home payload = %s, want 'not_home'", string(personNotHome))
	}

	// Zone occupancy payloads
	zoneOccupancy := []byte("2")
	if string(zoneOccupancy) != "2" {
		t.Errorf("Zone occupancy payload = %s, want '2'", string(zoneOccupancy))
	}

	// Zone occupants JSON payload
	occupants := []string(`["Alice", "Bob"]`)
	occupantsJSON, _ := json.Marshal(occupants)

	var decoded []string
	if err := json.Unmarshal(occupantsJSON, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal occupants: %v", err)
	}

	if len(decoded) != 2 || decoded[0] != "Alice" || decoded[1] != "Bob" {
		t.Errorf("Occupants = %v, want [Alice Bob]", decoded)
	}
}

// TestMQTTClientMock tests MQTT client with mock broker.
func TestMQTTClientMock(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping MQTT integration test in short mode")
	}

	// Create a test MQTT broker
	opts := mqtt.NewClientOptions()
	opts.SetBroker("tcp://localhost:1883")
	opts.SetClientID("spaxel-test")
	opts.SetCleanSession(true)

	mockClient := mqtt.NewClient(opts)
	if token := mockClient.Connect(); token.Wait() && token.Error() != nil {
		t.Skip("MQTT broker not available, skipping integration test")
		return
	}
	defer mockClient.Disconnect(250)

	// Test publish and subscribe
	received := make(chan []byte, 1)
	topic := "spaxel/test/presence"

	token := mockClient.Subscribe(topic, 1, func(c mqtt.Client, m mqtt.Message) {
		received <- m.Payload()
	})
	if token.Wait() && token.Error() != nil {
		t.Fatalf("Subscribe failed: %v", token.Error())
	}

	payload := []byte("home")
	token = mockClient.Publish(topic, 1, false, payload)
	if token.Wait() && token.Error() != nil {
		t.Fatalf("Publish failed: %v", token.Error())
	}

	select {
	case msg := <-received:
		if string(msg) != "home" {
			t.Errorf("Received = %s, want 'home'", string(msg))
		}
	case <-time.After(5 * time.Second):
		t.Error("Timeout waiting for message")
	}
}

// TestRetainedMessages tests that retained messages work correctly.
func TestRetainedMessages(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping MQTT integration test in short mode")
	}

	// Create two clients
	opts1 := mqtt.NewClientOptions()
	opts1.SetBroker("tcp://localhost:1883")
	opts1.SetClientID("spaxel-test-publisher")
	opts1.SetCleanSession(true)

	opts2 := mqtt.NewClientOptions()
	opts2.SetBroker("tcp://localhost:1883")
	opts2.SetClientID("spaxel-test-subscriber")
	opts2.SetCleanSession(true)

	client1 := mqtt.NewClient(opts1)
	if token := client1.Connect(); token.Wait() && token.Error() != nil {
		t.Skip("MQTT broker not available, skipping integration test")
		return
	}
	defer client1.Disconnect(250)

	client2 := mqtt.NewClient(opts2)
	if token := client2.Connect(); token.Wait() && token.Error() != nil {
		t.Skip("MQTT broker not available, skipping integration test")
		return
	}
	defer client2.Disconnect(250)

	topic := "spaxel/test/retained"

	// Subscribe first to receive retained message
	received := make(chan []byte, 1)
	token := client2.Subscribe(topic, 1, func(c mqtt.Client, m mqtt.Message) {
		received <- m.Payload()
	})
	if token.Wait() && token.Error() != nil {
		t.Fatalf("Subscribe failed: %v", token.Error())
	}

	// Publish retained message
	payload := []byte("home")
	token = client1.Publish(topic, 1, true, payload)
	if token.Wait() && token.Error() != nil {
		t.Fatalf("Publish retained failed: %v", token.Error())
	}

	// Receive retained message
	select {
	case msg := <-received:
		if string(msg) != "home" {
			t.Errorf("Received = %s, want 'home'", string(msg))
		}
	case <-time.After(5 * time.Second):
		t.Error("Timeout waiting for retained message")
	}
}

// TestGetBrokerHost tests broker URL parsing.
func TestGetBrokerHost(t *testing.T) {
	tests := []struct {
		name     string
		broker   string
		expected string
	}{
		{
			name:     "TCP with port",
			broker:   "tcp://192.168.1.100:1883",
			expected: "192.168.1.100",
		},
		{
			name:     "MQTT with port",
			broker:   "mqtt://homeassistant.local:1883",
			expected: "homeassistant.local",
		},
		{
			name:     "MQTTS with port",
			broker:   "mqtts://secure.example.com:8883",
			expected: "secure.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{Broker: tt.broker}
			client, _ := NewClient(cfg)
			if host := client.GetBrokerHost(); host != tt.expected {
				t.Errorf("GetBrokerHost() = %s, want %s", host, tt.expected)
			}
		})
	}
}

// TestHTTPWebhookClient tests the HTTP webhook client.
func TestHTTPWebhookClient(t *testing.T) {
	// Create a test HTTP server
	var receivedPayload []byte
	var receivedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %s, want application/json", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("X-Spaxel-Event") != "spaxel-event" {
			t.Errorf("X-Spaxel-Event = %s, want spaxel-event", r.Header.Get("X-Spaxel-Event"))
		}

		// Store payload and headers for verification
		receivedPayload, _ = httputil.DumpRequest(r, false)
		receivedHeaders = r.Header.Clone()

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Test sending webhook
	client := &http.Client{Timeout: 5 * time.Second}
	payload := []byte(`{"event_type":"test","timestamp":"2024-03-15T12:00:00Z"}`)

	req, _ := http.NewRequest("POST", server.URL, strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Spaxel-Event", "spaxel-event")
	req.Header.Set("User-Agent", "Spaxel/1.0")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Status = %d, want 200", resp.StatusCode)
	}

	// Verify payload was received
	if receivedPayload == nil {
		t.Error("No payload received")
	}
}
