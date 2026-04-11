// Package webhook provides tests for system webhook integration.
package webhook

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/spaxel/mothership/internal/eventbus"
)

// TestNewPublisher validates webhook publisher creation.
func TestNewPublisher(t *testing.T) {
	cfg := Config{
		URL:     "http://example.com/webhook",
		Enabled: true,
	}

	publisher := NewPublisher(cfg)
	if publisher == nil {
		t.Fatal("NewPublisher() returned nil")
	}

	retrievedCfg := publisher.GetConfig()
	if retrievedCfg.URL != cfg.URL {
		t.Errorf("URL = %s, want %s", retrievedCfg.URL, cfg.URL)
	}
	if retrievedCfg.Enabled != cfg.Enabled {
		t.Errorf("Enabled = %v, want %v", retrievedCfg.Enabled, cfg.Enabled)
	}
}

// TestPublishEvent tests event publishing to webhook.
func TestPublishEvent(t *testing.T) {
	// Create a test HTTP server
	var receivedPayload string
	var receivedContentType string
	var receivedEventHeader string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read and store payload
		bodyBytes, _ := io.ReadAll(r.Body)
		receivedPayload = string(bodyBytes)

		receivedContentType = r.Header.Get("Content-Type")
		receivedEventHeader = r.Header.Get("X-Spaxel-Event")

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create publisher with test server URL
	cfg := Config{
		URL:     server.URL,
		Enabled: true,
	}

	publisher := NewPublisher(cfg)
	publisher.Start()
	defer publisher.Stop()

	// Create a test event
	event := eventbus.Event{
		Type:        "zone_entry",
		TimestampMs: time.Now().UnixMilli(),
		Zone:        "kitchen",
		Person:      "Alice",
		BlobID:      1,
		Severity:    "info",
		Detail:      map[string]interface{}{"test": true},
	}

	// Publish event (simulating eventbus callback)
	publisher.publishEvent(event)

	// Wait a bit for async processing
	time.Sleep(100 * time.Millisecond)

	// Verify request
	if receivedPayload == "" {
		t.Fatal("No payload received")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(receivedPayload), &payload); err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	if payload["event_type"] != "zone_entry" {
		t.Errorf("event_type = %s, want zone_entry", payload["event_type"])
	}
	if payload["zone"] != "kitchen" {
		t.Errorf("zone = %s, want kitchen", payload["zone"])
	}
	if payload["person"] != "Alice" {
		t.Errorf("person = %s, want Alice", payload["person"])
	}
	if payload["blob_id"] != float64(1) {
		t.Errorf("blob_id = %v, want 1", payload["blob_id"])
	}

	if receivedContentType != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", receivedContentType)
	}
	if receivedEventHeader != "spaxel-event" {
		t.Errorf("X-Spaxel-Event = %s, want spaxel-event", receivedEventHeader)
	}
}

// TestPublishEventDisabled tests that events are not sent when disabled.
func TestPublishEventDisabled(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := Config{
		URL:     server.URL,
		Enabled: false, // Disabled
	}

	publisher := NewPublisher(cfg)
	publisher.Start()
	defer publisher.Stop()

	// Create and publish event
	event := eventbus.Event{
		Type:        "zone_entry",
		TimestampMs: time.Now().UnixMilli(),
		Zone:        "kitchen",
	}

	publisher.publishEvent(event)
	time.Sleep(100 * time.Millisecond)

	if callCount > 0 {
		t.Errorf("Server was called %d times, want 0 (disabled)", callCount)
	}
}

// TestRetryOn5xx tests retry behavior on server errors.
func TestRetryOn5xx(t *testing.T) {
	callCount := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		callCount++

		// Return 500 on first call, 200 on retry
		if callCount == 1 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	cfg := Config{
		URL:        server.URL,
		Enabled:    true,
		RetryDelay: 10 * time.Millisecond, // Short delay for testing
	}

	publisher := NewPublisher(cfg)
	publisher.Start()
	defer publisher.Stop()

	event := eventbus.Event{
		Type:        "test",
		TimestampMs: time.Now().UnixMilli(),
	}

	publisher.publishEvent(event)
	time.Sleep(200 * time.Millisecond) // Wait for retry

	mu.Lock()
	count := callCount
	mu.Unlock()

	if count < 2 {
		t.Errorf("Retry count = %d, want at least 2", count)
	}
}

// TestEventPayloadSchema tests the event payload JSON schema.
func TestEventPayloadSchema(t *testing.T) {
	payload := EventPayload{
		EventType: "zone_entry",
		Timestamp: "2024-03-15T12:00:00Z",
		Zone:      "kitchen",
		Person:    "Alice",
		BlobID:    1,
		Severity:  "info",
		Detail: map[string]interface{}{
			"motion_level": 0.5,
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(jsonData, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Verify required fields
	requiredFields := []string{"event_type", "timestamp"}
	for _, field := range requiredFields {
		if _, exists := decoded[field]; !exists {
			t.Errorf("Missing required field: %s", field)
		}
	}

	if decoded["event_type"] != "zone_entry" {
		t.Errorf("event_type = %s, want zone_entry", decoded["event_type"])
	}
	if decoded["zone"] != "kitchen" {
		t.Errorf("zone = %s, want kitchen", decoded["zone"])
	}
	if decoded["detail"] == nil {
		t.Error("detail field is nil")
	}
}

// TestTestWebhook tests the TestWebhook method.
func TestTestWebhook(t *testing.T) {
	var receivedPayload string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		receivedPayload = string(bodyBytes)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := Config{
		URL:     server.URL,
		Enabled: true,
	}

	publisher := NewPublisher(cfg)

	err := publisher.TestWebhook()
	if err != nil {
		t.Fatalf("TestWebhook() failed: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(receivedPayload), &payload); err != nil {
		t.Fatalf("Failed to unmarshal test payload: %v", err)
	}

	if payload["event_type"] != "test" {
		t.Errorf("event_type = %s, want test", payload["event_type"])
	}
	if payload["detail"] == nil {
		t.Error("detail field is missing")
	}
}

// TestValidationError tests validation error formatting.
func TestValidationError(t *testing.T) {
	err := &ValidationError{
		Field:  "url",
		Reason: "invalid URL format",
	}

	expected := "url: invalid URL format"
	if err.Error() != expected {
		t.Errorf("Error() = %s, want %s", err.Error(), expected)
	}
}

// TestHTTPError tests HTTP error formatting.
func TestHTTPError(t *testing.T) {
	err := &HTTPError{
		StatusCode: 500,
		Message:    "server error",
	}

	if err.Error() != "server error" {
		t.Errorf("Error() = %s, want 'server error'", err.Error())
	}
	if err.StatusCode != 500 {
		t.Errorf("StatusCode = %d, want 500", err.StatusCode)
	}
}

// TestConcurrentPublishing tests concurrent event publishing.
func TestConcurrentPublishing(t *testing.T) {
	callCount := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		callCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := Config{
		URL:     server.URL,
		Enabled: true,
	}

	publisher := NewPublisher(cfg)
	publisher.Start()
	defer publisher.Stop()

	// Publish multiple events concurrently
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			event := eventbus.Event{
				Type:        "test",
				TimestampMs: time.Now().UnixMilli(),
				BlobID:      i,
			}
			publisher.publishEvent(event)
		}(i)
	}

	wg.Wait()
	time.Sleep(200 * time.Millisecond) // Wait for all requests

	mu.Lock()
	finalCount := callCount
	mu.Unlock()

	if finalCount != 10 {
		t.Errorf("Call count = %d, want 10", finalCount)
	}
}

// TestConfigUpdate tests dynamic configuration updates.
func TestConfigUpdate(t *testing.T) {
	cfg1 := Config{
		URL:     "http://example.com/webhook",
		Enabled: true,
	}

	publisher := NewPublisher(cfg1)

	// Update config
	cfg2 := Config{
		URL:     "http://updated.example.com/webhook",
		Enabled: false,
	}

	publisher.UpdateConfig(cfg2)

	retrieved := publisher.GetConfig()
	// Note: URL comparison is direct since Config is a value type
	if retrieved.Enabled != false {
		t.Errorf("Enabled = %v, want false after update", retrieved.Enabled)
	}
}

// TestAllEventTypes tests publishing of various event types.
func TestAllEventTypes(t *testing.T) {
	receivedEvents := make(map[string]int)
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		json.NewDecoder(r.Body).Decode(&payload)

		mu.Lock()
		eventType, _ := payload["event_type"].(string)
		receivedEvents[eventType]++
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := Config{
		URL:     server.URL,
		Enabled: true,
	}

	publisher := NewPublisher(cfg)
	publisher.Start()
	defer publisher.Stop()

	events := []eventbus.Event{
		{Type: eventbus.TypeZoneEntry, TimestampMs: time.Now().UnixMilli(), Zone: "kitchen", Person: "Alice"},
		{Type: eventbus.TypeZoneExit, TimestampMs: time.Now().UnixMilli(), Zone: "kitchen", Person: "Alice"},
		{Type: eventbus.TypeFallAlert, TimestampMs: time.Now().UnixMilli(), Severity: "alert"},
		{Type: eventbus.TypeAnomaly, TimestampMs: time.Now().UnixMilli(), Severity: "warning"},
		{Type: eventbus.TypeSecurityAlert, TimestampMs: time.Now().UnixMilli(), Severity: "critical"},
	}

	for _, event := range events {
		publisher.publishEvent(event)
	}

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	expectedTypes := []string{"zone_entry", "zone_exit", "fall_alert", "anomaly", "security_alert"}
	for _, eventType := range expectedTypes {
		if receivedEvents[eventType] == 0 {
			t.Errorf("Event type %s not received", eventType)
		}
	}
}
