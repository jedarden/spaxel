package notifications

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestNewWebhookClient tests creating a new webhook client.
func TestNewWebhookClient(t *testing.T) {
	client := NewWebhookClient("https://example.com/webhook")

	if client == nil {
		t.Fatal("NewWebhookClient() returned nil")
	}

	if client.URL != "https://example.com/webhook" {
		t.Errorf("URL = %s, want https://example.com/webhook", client.URL)
	}

	if client.Method != "POST" {
		t.Errorf("Method = %s, want POST", client.Method)
	}

	if client.Timeout != 10*time.Second {
		t.Errorf("Timeout = %v, want 10s", client.Timeout)
	}

	if client.Headers == nil {
		t.Error("Headers map should be initialized")
	}
}

// TestWebhookSendBasic tests sending a basic webhook notification.
func TestWebhookSendBasic(t *testing.T) {
	var receivedPayload WebhookPayload
	var receivedContentType string
	var receivedUserAgent string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		receivedUserAgent = r.Header.Get("User-Agent")

		var payload WebhookPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("Failed to decode payload: %v", err)
		}
		receivedPayload = payload

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewWebhookClient(server.URL)

	payload := NewWebhookPayload("test_event", "Test message")

	err := client.Send(payload)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if receivedPayload.EventType != "test_event" {
		t.Errorf("EventType = %s, want test_event", receivedPayload.EventType)
	}

	if receivedPayload.Message != "Test message" {
		t.Errorf("Message = %s, want 'Test message'", receivedPayload.Message)
	}

	if receivedContentType != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", receivedContentType)
	}

	if receivedUserAgent != "Spaxel/1.0" {
		t.Errorf("User-Agent = %s, want 'Spaxel/1.0'", receivedUserAgent)
	}
}

// TestWebhookSendWithAllFields tests sending a webhook with all fields.
func TestWebhookSendWithAllFields(t *testing.T) {
	var receivedPayload WebhookPayload

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewWebhookClient(server.URL)

	blobID := 42
	x, y, z, confidence := 1.5, 2.3, 0.8, 0.92

	payload := NewWebhookPayload("zone_enter", "Alice entered Kitchen")
	payload.Title = "Zone Entry"
	payload.Priority = "low"
	payload.PersonID = "person-123"
	payload.PersonName = "Alice"
	payload.ZoneID = "zone-kitchen"
	payload.ZoneName = "Kitchen"
	payload.SetBlobPosition(blobID, x, y, z, confidence)
	payload.SetNodeInfo("AA:BB:CC:DD:EE:FF", "Kitchen Node", "rx")
	payload.AddMetadata("custom_field", "custom_value")

	err := client.Send(payload)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if receivedPayload.EventType != "zone_enter" {
		t.Errorf("EventType = %s, want zone_enter", receivedPayload.EventType)
	}

	if receivedPayload.Title != "Zone Entry" {
		t.Errorf("Title = %s, want 'Zone Entry'", receivedPayload.Title)
	}

	if receivedPayload.Priority != "low" {
		t.Errorf("Priority = %s, want low", receivedPayload.Priority)
	}

	if receivedPayload.PersonID != "person-123" {
		t.Errorf("PersonID = %s, want person-123", receivedPayload.PersonID)
	}

	if receivedPayload.PersonName != "Alice" {
		t.Errorf("PersonName = %s, want Alice", receivedPayload.PersonName)
	}

	if receivedPayload.ZoneID != "zone-kitchen" {
		t.Errorf("ZoneID = %s, want zone-kitchen", receivedPayload.ZoneID)
	}

	if receivedPayload.ZoneName != "Kitchen" {
		t.Errorf("ZoneName = %s, want Kitchen", receivedPayload.ZoneName)
	}

	if receivedPayload.BlobID == nil || *receivedPayload.BlobID != blobID {
		t.Errorf("BlobID = %v, want %d", receivedPayload.BlobID, blobID)
	}

	if receivedPayload.BlobX == nil || *receivedPayload.BlobX != x {
		t.Errorf("BlobX = %v, want %f", receivedPayload.BlobX, x)
	}

	if receivedPayload.BlobY == nil || *receivedPayload.BlobY != y {
		t.Errorf("BlobY = %v, want %f", receivedPayload.BlobY, y)
	}

	if receivedPayload.BlobZ == nil || *receivedPayload.BlobZ != z {
		t.Errorf("BlobZ = %v, want %f", receivedPayload.BlobZ, z)
	}

	if receivedPayload.Confidence == nil || *receivedPayload.Confidence != confidence {
		t.Errorf("Confidence = %v, want %f", receivedPayload.Confidence, confidence)
	}

	if receivedPayload.NodeMAC != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("NodeMAC = %s, want AA:BB:CC:DD:EE:FF", receivedPayload.NodeMAC)
	}

	if receivedPayload.NodeName != "Kitchen Node" {
		t.Errorf("NodeName = %s, want 'Kitchen Node'", receivedPayload.NodeName)
	}

	if receivedPayload.NodeRole != "rx" {
		t.Errorf("NodeRole = %s, want rx", receivedPayload.NodeRole)
	}

	if receivedPayload.Metadata == nil {
		t.Error("Metadata should not be nil")
	} else if receivedPayload.Metadata["custom_field"] != "custom_value" {
		t.Errorf("Metadata[custom_field] = %v, want 'custom_value'", receivedPayload.Metadata["custom_field"])
	}
}

// TestWebhookSendWithPNGImage tests sending with PNG image.
func TestWebhookSendWithPNGImage(t *testing.T) {
	var receivedPayload WebhookPayload

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewWebhookClient(server.URL)

	// Minimal valid PNG
	pngData := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	}

	payload := NewWebhookPayload("test", "test")
	payload.AttachPNGImage(pngData)

	err := client.Send(payload)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if receivedPayload.FloorplanPNGBase64 == "" {
		t.Error("FloorplanPNGBase64 should be set")
	}

	// Should start with valid base64
	if !strings.HasPrefix(receivedPayload.FloorplanPNGBase64, "iVBORw") {
		t.Errorf("FloorplanPNGBase64 should start with 'iVBORw', got: %s", receivedPayload.FloorplanPNGBase64[:10])
	}
}

// TestWebhookSendErrorCases tests error handling.
func TestWebhookSendErrorCases(t *testing.T) {
	t.Run("nil client", func(t *testing.T) {
		var client *WebhookClient = nil
		payload := NewWebhookPayload("test", "test")
		err := client.Send(payload)
		if err == nil {
			t.Error("Expected error for nil client")
		}
		if !strings.Contains(err.Error(), "nil") {
			t.Errorf("Error should mention nil, got: %v", err)
		}
	})

	t.Run("missing URL", func(t *testing.T) {
		client := NewWebhookClient("")
		payload := NewWebhookPayload("test", "test")
		err := client.Send(payload)
		if err == nil {
			t.Error("Expected error for missing URL")
		}
		if !strings.Contains(err.Error(), "URL") {
			t.Errorf("Error should mention URL, got: %v", err)
		}
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal server error"))
		}))
		defer server.Close()

		client := NewWebhookClient(server.URL)
		payload := NewWebhookPayload("test", "test")
		err := client.Send(payload)
		if err == nil {
			t.Error("Expected error for server error")
		}
		if !strings.Contains(err.Error(), "500") {
			t.Errorf("Error should mention status 500, got: %v", err)
		}
	})
}

// TestWebhookSetters tests client setter methods.
func TestWebhookSetters(t *testing.T) {
	client := NewWebhookClient("https://example.com/hook")

	client.SetHeader("X-Custom-Header", "custom-value")
	if client.Headers["X-Custom-Header"] != "custom-value" {
		t.Errorf("Custom header not set correctly")
	}

	client.SetBasicAuth("user", "pass")
	if client.Headers["X-Webhook-Username"] != "user" {
		t.Errorf("Username not set correctly")
	}
	if client.Headers["X-Webhook-Password"] != "pass" {
		t.Errorf("Password not set correctly")
	}

	client.SetTimeout(30 * time.Second)
	if client.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", client.Timeout)
	}
}

// TestWebhookCustomHeaders tests sending with custom headers.
func TestWebhookCustomHeaders(t *testing.T) {
	var receivedHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader = r.Header.Get("X-Custom-Header")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewWebhookClient(server.URL)
	client.SetHeader("X-Custom-Header", "test-value-123")

	payload := NewWebhookPayload("test", "test")
	err := client.Send(payload)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if receivedHeader != "test-value-123" {
		t.Errorf("Custom header = %s, want 'test-value-123'", receivedHeader)
	}
}

// TestWebhookPayloadHelpers tests payload helper functions.
func TestWebhookPayloadHelpers(t *testing.T) {
	t.Run("NewFallDetectedPayload", func(t *testing.T) {
		payload := NewFallDetectedPayload("Alice", "Kitchen", 1, 1.5, 2.0, 0.8, 0.95)

		if payload.EventType != "fall_detected" {
			t.Errorf("EventType = %s, want fall_detected", payload.EventType)
		}

		if payload.Title != "Fall Detected" {
			t.Errorf("Title = %s, want 'Fall Detected'", payload.Title)
		}

		if payload.Priority != "urgent" {
			t.Errorf("Priority = %s, want urgent", payload.Priority)
		}

		if payload.PersonName != "Alice" {
			t.Errorf("PersonName = %s, want Alice", payload.PersonName)
		}

		if payload.ZoneName != "Kitchen" {
			t.Errorf("ZoneName = %s, want Kitchen", payload.ZoneName)
		}

		if payload.Metadata == nil || payload.Metadata["requires_action"] != true {
			t.Error("Metadata should have requires_action=true")
		}
	})

	t.Run("NewZoneEnterPayload", func(t *testing.T) {
		payload := NewZoneEnterPayload("Bob", "Living Room")

		if payload.EventType != "zone_enter" {
			t.Errorf("EventType = %s, want zone_enter", payload.EventType)
		}

		if payload.Message != "Bob entered Living Room" {
			t.Errorf("Message = %s, want 'Bob entered Living Room'", payload.Message)
		}

		if payload.PersonName != "Bob" {
			t.Errorf("PersonName = %s, want Bob", payload.PersonName)
		}

		if payload.ZoneName != "Living Room" {
			t.Errorf("ZoneName = %s, want 'Living Room'", payload.ZoneName)
		}

		if payload.Priority != "low" {
			t.Errorf("Priority = %s, want low", payload.Priority)
		}
	})

	t.Run("NewZoneLeavePayload", func(t *testing.T) {
		payload := NewZoneLeavePayload("Charlie", "Bedroom")

		if payload.EventType != "zone_leave" {
			t.Errorf("EventType = %s, want zone_leave", payload.EventType)
		}

		if payload.Message != "Charlie left Bedroom" {
			t.Errorf("Message = %s, want 'Charlie left Bedroom'", payload.Message)
		}
	})

	t.Run("NewAnomalyAlertPayload", func(t *testing.T) {
		payload := NewAnomalyAlertPayload("Hallway", 0.92, "Unusual motion detected")

		if payload.EventType != "anomaly_alert" {
			t.Errorf("EventType = %s, want anomaly_alert", payload.EventType)
		}

		if payload.Title != "Anomaly Alert" {
			t.Errorf("Title = %s, want 'Anomaly Alert'", payload.Title)
		}

		if payload.Priority != "high" {
			t.Errorf("Priority = %s, want high", payload.Priority)
		}

		if payload.Metadata == nil || payload.Metadata["anomaly_score"] != 0.92 {
			t.Error("Metadata should have anomaly_score=0.92")
		}
	})

	t.Run("NewNodeOfflinePayload", func(t *testing.T) {
		payload := NewNodeOfflinePayload("AA:BB:CC:DD:EE:FF", "Kitchen Node", "rx")

		if payload.EventType != "node_offline" {
			t.Errorf("EventType = %s, want node_offline", payload.EventType)
		}

		if payload.Title != "Node Offline" {
			t.Errorf("Title = %s, want 'Node Offline'", payload.Title)
		}

		if payload.NodeMAC != "AA:BB:CC:DD:EE:FF" {
			t.Errorf("NodeMAC = %s, want AA:BB:CC:DD:EE:FF", payload.NodeMAC)
		}

		if payload.NodeName != "Kitchen Node" {
			t.Errorf("NodeName = %s, want 'Kitchen Node'", payload.NodeName)
		}

		if payload.NodeRole != "rx" {
			t.Errorf("NodeRole = %s, want rx", payload.NodeRole)
		}
	})
}

// TestWebhookTimestampFields tests timestamp handling.
func TestWebhookTimestampFields(t *testing.T) {
	var receivedPayload WebhookPayload

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewWebhookClient(server.URL)

	before := time.Now()
	payload := NewWebhookPayload("test", "test")
	after := time.Now()

	err := client.Send(payload)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if receivedPayload.Timestamp < before.Unix() || receivedPayload.Timestamp > after.Unix() {
		t.Errorf("Timestamp = %d, want between %d and %d", receivedPayload.Timestamp, before.Unix(), after.Unix())
	}

	// Verify ISO format is parseable
	_, err = time.Parse(time.RFC3339, receivedPayload.TimestampISO)
	if err != nil {
		t.Errorf("TimestampISO = %s is not valid RFC3339: %v", receivedPayload.TimestampISO, err)
	}
}

// TestWebhookAttachPNGImage tests the PNG attachment helper.
func TestWebhookAttachPNGImage(t *testing.T) {
	t.Run("valid PNG", func(t *testing.T) {
		pngData := []byte{
			0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
			0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		}

		payload := NewWebhookPayload("test", "test")
		payload.AttachPNGImage(pngData)

		if payload.FloorplanPNGBase64 == "" {
			t.Error("FloorplanPNGBase64 should be set")
		}
	})

	t.Run("empty PNG", func(t *testing.T) {
		payload := NewWebhookPayload("test", "test")
		payload.AttachPNGImage([]byte{})

		if payload.FloorplanPNGBase64 != "" {
			t.Error("FloorplanPNGBase64 should be empty for empty input")
		}
	})

	t.Run("invalid PNG", func(t *testing.T) {
		invalidData := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

		payload := NewWebhookPayload("test", "test")
		payload.AttachPNGImage(invalidData)

		// Invalid PNG should not be attached (signature check fails)
		if payload.FloorplanPNGBase64 != "" {
			t.Error("FloorplanPNGBase64 should be empty for invalid PNG")
		}
	})
}

// TestWebhookSetBlobPosition tests setting blob position.
func TestWebhookSetBlobPosition(t *testing.T) {
	payload := NewWebhookPayload("test", "test")
	payload.SetBlobPosition(42, 1.5, 2.3, 0.8, 0.95)

	if payload.BlobID == nil || *payload.BlobID != 42 {
		t.Errorf("BlobID = %v, want 42", payload.BlobID)
	}

	if payload.BlobX == nil || *payload.BlobX != 1.5 {
		t.Errorf("BlobX = %v, want 1.5", payload.BlobX)
	}

	if payload.BlobY == nil || *payload.BlobY != 2.3 {
		t.Errorf("BlobY = %v, want 2.3", payload.BlobY)
	}

	if payload.BlobZ == nil || *payload.BlobZ != 0.8 {
		t.Errorf("BlobZ = %v, want 0.8", payload.BlobZ)
	}

	if payload.Confidence == nil || *payload.Confidence != 0.95 {
		t.Errorf("Confidence = %v, want 0.95", payload.Confidence)
	}
}

// TestWebhookSetNodeInfo tests setting node information.
func TestWebhookSetNodeInfo(t *testing.T) {
	payload := NewWebhookPayload("test", "test")
	payload.SetNodeInfo("AA:BB:CC:DD:EE:FF", "Test Node", "tx")

	if payload.NodeMAC != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("NodeMAC = %s, want AA:BB:CC:DD:EE:FF", payload.NodeMAC)
	}

	if payload.NodeName != "Test Node" {
		t.Errorf("NodeName = %s, want 'Test Node'", payload.NodeName)
	}

	if payload.NodeRole != "tx" {
		t.Errorf("NodeRole = %s, want tx", payload.NodeRole)
	}
}

// TestWebhookAddMetadata tests adding metadata.
func TestWebhookAddMetadata(t *testing.T) {
	payload := NewWebhookPayload("test", "test")

	payload.AddMetadata("key1", "value1")
	payload.AddMetadata("key2", 42)
	payload.AddMetadata("key3", true)

	if payload.Metadata == nil {
		t.Fatal("Metadata should not be nil")
	}

	if payload.Metadata["key1"] != "value1" {
		t.Errorf("Metadata[key1] = %v, want 'value1'", payload.Metadata["key1"])
	}

	if payload.Metadata["key2"] != 42 {
		t.Errorf("Metadata[key2] = %v, want 42", payload.Metadata["key2"])
	}

	if payload.Metadata["key3"] != true {
		t.Errorf("Metadata[key3] = %v, want true", payload.Metadata["key3"])
	}
}

// TestWebhookMethodOverride tests using different HTTP methods.
func TestWebhookMethodOverride(t *testing.T) {
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewWebhookClient(server.URL)
	client.Method = "PUT"

	payload := NewWebhookPayload("test", "test")
	err := client.Send(payload)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if receivedMethod != "PUT" {
		t.Errorf("Method = %s, want PUT", receivedMethod)
	}
}

// TestWebhookNilHTTPClient tests sending with nil HTTPClient.
func TestWebhookNilHTTPClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewWebhookClient(server.URL)
	client.HTTPClient = nil // Explicitly set to nil

	payload := NewWebhookPayload("test", "test")
	err := client.Send(payload)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	// Should not panic, should create default client
}

// TestWebhookJSONSerialization tests that payload serializes correctly.
func TestWebhookJSONSerialization(t *testing.T) {
	payload := NewWebhookPayload("test_event", "Test message")
	payload.Title = "Test Title"
	payload.Priority = "high"
	payload.PersonID = "person-123"
	payload.PersonName = "Alice"
	payload.ZoneID = "zone-kitchen"
	payload.ZoneName = "Kitchen"

	blobID := 42
	x, y, z, confidence := 1.5, 2.3, 0.8, 0.92
	payload.SetBlobPosition(blobID, x, y, z, confidence)

	payload.SetNodeInfo("AA:BB:CC:DD:EE:FF", "Kitchen Node", "rx")
	payload.AddMetadata("custom", "value")

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	// Verify it's valid JSON that can be unmarshaled
	var unmarshaled WebhookPayload
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if unmarshaled.EventType != payload.EventType {
		t.Errorf("Unmarshaled EventType = %s, want %s", unmarshaled.EventType, payload.EventType)
	}

	if unmarshaled.PersonName != payload.PersonName {
		t.Errorf("Unmarshaled PersonName = %s, want %s", unmarshaled.PersonName, payload.PersonName)
	}

	if unmarshaled.BlobID == nil || *unmarshaled.BlobID != blobID {
		t.Errorf("Unmarshaled BlobID = %v, want %d", unmarshaled.BlobID, blobID)
	}
}
