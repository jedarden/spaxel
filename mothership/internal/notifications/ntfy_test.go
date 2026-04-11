package notifications

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestNtfyClientNewClient tests creating a new ntfy client.
func TestNtfyClientNewClient(t *testing.T) {
	client := NewNtfyClient("test-topic")

	if client == nil {
		t.Fatal("NewNtfyClient() returned nil")
	}

	if client.Topic != "test-topic" {
		t.Errorf("Topic = %s, want test-topic", client.Topic)
	}

	if client.URL != "https://ntfy.sh" {
		t.Errorf("URL = %s, want https://ntfy.sh", client.URL)
	}

	if client.Priority != "default" {
		t.Errorf("Priority = %s, want default", client.Priority)
	}

	if client.HTTPClient == nil {
		t.Error("HTTPClient is nil")
	}
}

// TestNtfyClientSend tests sending a notification via ntfy.
func TestNtfyClientSend(t *testing.T) {
	// Capture request details in handler
	var receivedMethod, receivedPath, receivedBody string
	receivedHeaders := make(map[string]string)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path

		// Capture headers
		receivedHeaders["Title"] = r.Header.Get("Title")
		receivedHeaders["Priority"] = r.Header.Get("Priority")
		receivedHeaders["Tags"] = r.Header.Get("Tags")
		receivedHeaders["Content-Type"] = r.Header.Get("Content-Type")

		// Capture body
		bodyBuf := new(bytes.Buffer)
		bodyBuf.ReadFrom(r.Body)
		receivedBody = bodyBuf.String()

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewNtfyClient("test-topic")
	client.URL = server.URL

	msg := NtfyMessage{
		Topic:    "test-topic",
		Title:    "Test Title",
		Message:  "Test message body",
		Priority: "high",
		Tags:     []string{"test", "alert"},
	}

	err := client.Send(msg)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	// Verify method
	if receivedMethod != "POST" {
		t.Errorf("Method = %s, want POST", receivedMethod)
	}

	// Verify URL path
	if !strings.HasSuffix(receivedPath, "/test-topic") {
		t.Errorf("URL path = %s, want /test-topic", receivedPath)
	}

	// Verify headers
	title := receivedHeaders["Title"]
	if title != "Test Title" {
		t.Errorf("Title header = %s, want 'Test Title'", title)
	}

	priority := receivedHeaders["Priority"]
	if priority != "high" {
		t.Errorf("Priority header = %s, want 'high'", priority)
	}

	tags := receivedHeaders["Tags"]
	if tags != "test,alert" {
		t.Errorf("Tags header = %s, want 'test,alert'", tags)
	}

	contentType := receivedHeaders["Content-Type"]
	if contentType != "text/plain" {
		t.Errorf("Content-Type header = %s, want 'text/plain'", contentType)
	}

	// Verify body
	if receivedBody != "Test message body" {
		t.Errorf("Body = %s, want 'Test message body'", receivedBody)
	}
}

// TestNtfyClientSendWithToken tests sending with authentication token.
func TestNtfyClientSendWithToken(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewNtfyClient("private-topic")
	client.URL = server.URL
	client.Token = "test-token-12345"

	msg := NtfyMessage{
		Topic:   "private-topic",
		Message: "Secret message",
	}

	err := client.Send(msg)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if receivedAuth != "Bearer test-token-12345" {
		t.Errorf("Authorization header = %s, want 'Bearer test-token-12345'", receivedAuth)
	}
}

// TestNtfyClientSendWithImage tests sending with PNG image attachment.
func TestNtfyClientSendWithImage(t *testing.T) {
	var receivedAttach string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAttach = r.Header.Get("Attach")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewNtfyClient("test-topic")
	client.URL = server.URL

	// Create a minimal PNG (1x1 transparent pixel)
	pngData := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52, // IHDR chunk start
		// ... minimal valid PNG would continue
		0x82, 0x50, 0x4E, 0x47, // Just for testing, use AttachPNGImage helper
	}

	msg := NtfyMessage{
		Topic:   "test-topic",
		Message: "Message with image",
		Image:   AttachPNGImage(pngData),
	}

	err := client.Send(msg)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if !strings.HasPrefix(receivedAttach, "data:image/png;base64,") {
		t.Errorf("Attach header should start with 'data:image/png;base64,', got: %s", receivedAttach)
	}
}

// TestNtfyClientSendErrorCases tests error handling.
func TestNtfyClientSendErrorCases(t *testing.T) {
	t.Run("nil client", func(t *testing.T) {
		var client *NtfyClient = nil
		msg := NtfyMessage{Message: "test"}
		err := client.Send(msg)
		if err == nil {
			t.Error("Expected error for nil client")
		}
		if !strings.Contains(err.Error(), "nil") {
			t.Errorf("Error message should mention nil client, got: %v", err)
		}
	})

	t.Run("missing topic", func(t *testing.T) {
		client := NewNtfyClient("test-topic")
		client.Topic = "" // Clear topic
		msg := NtfyMessage{Message: "test"}
		err := client.Send(msg)
		if err == nil {
			t.Error("Expected error for missing topic")
		}
		if !strings.Contains(err.Error(), "topic") {
			t.Errorf("Error message should mention topic, got: %v", err)
		}
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte("Bad gateway"))
		}))
		defer server.Close()

		client := NewNtfyClient("test-topic")
		client.URL = server.URL

		msg := NtfyMessage{Topic: "test-topic", Message: "test"}
		err := client.Send(msg)
		if err == nil {
			t.Error("Expected error for server error")
		}
	})
}

// TestNtfyClientSetters tests client setter methods.
func TestNtfyClientSetters(t *testing.T) {
	client := NewNtfyClient("test-topic")

	client.SetPriority("urgent")
	if client.Priority != "urgent" {
		t.Errorf("Priority = %s, want urgent", client.Priority)
	}

	client.SetToken("my-token")
	if client.Token != "my-token" {
		t.Errorf("Token = %s, want my-token", client.Token)
	}

	client.SetURL("https://ntfy.example.com")
	if client.URL != "https://ntfy.example.com" {
		t.Errorf("URL = %s, want https://ntfy.example.com", client.URL)
	}
}

// TestNtfyClientDefaults tests that client defaults are used when message fields are empty.
func TestNtfyClientDefaults(t *testing.T) {
	var receivedPriority, receivedTags string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPriority = r.Header.Get("Priority")
		receivedTags = r.Header.Get("Tags")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewNtfyClient("test-topic")
	client.URL = server.URL
	client.Priority = "urgent"
	client.Tags = []string{"default-tag"}
	client.Click = "https://example.com"

	msg := NtfyMessage{
		// Leave fields empty to test defaults
		Message: "test",
	}

	err := client.Send(msg)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if receivedPriority != "urgent" {
		t.Errorf("Priority = %s, want urgent", receivedPriority)
	}

	if receivedTags != "default-tag" {
		t.Errorf("Tags = %s, want default-tag", receivedTags)
	}
}

// TestAttachPNGImage tests the PNG attachment helper.
func TestAttachPNGImage(t *testing.T) {
	pngData := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature
		0x00, 0x00, 0x00, 0x0D,                   // IHDR length
	}

	result := AttachPNGImage(pngData)

	// Verify the data URL prefix
	prefix := "data:image/png;base64,"
	if !strings.HasPrefix(result, prefix) {
		t.Errorf("Result should start with %s, got: %s", prefix, result)
	}

	// Verify the result is longer than just the prefix (has encoded data)
	if len(result) <= len(prefix) {
		t.Errorf("Result should have encoded data after prefix, got length %d", len(result))
	}

	// Extract and verify the base64 encoded portion
	encodedPart := result[len(prefix):]
	if encodedPart == "" {
		t.Error("Encoded part should not be empty")
	}

	// Verify the encoded data is different from input (base64 encoding changes it)
	if string(pngData) == encodedPart {
		t.Error("Encoded data should be base64 encoded, not plain text")
	}
}

// TestNtfyMessageAllFields tests sending a message with all fields set.
func TestNtfyMessageAllFields(t *testing.T) {
	headers := make(map[string]string)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, key := range []string{"Title", "Priority", "Tags", "Click", "Icon", "Delay", "Email", "Attach"} {
			headers[key] = r.Header.Get(key)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewNtfyTopic("test-topic")
	client.URL = server.URL

	msg := NtfyMessage{
		Topic:    "test-topic",
		Title:    "Full Title",
		Message:  "Full message",
		Priority: "urgent",
		Tags:     []string{"tag1", "tag2", "tag3"},
		Click:    "https://example.com/click",
		Icon:     "https://example.com/icon.png",
		Delay:    "30s",
		Email:    "test@example.com",
		Image:    "data:image/png;base64,iVBORw0KGgo=",
	}

	err := client.Send(msg)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if headers["Title"] != "Full Title" {
		t.Errorf("Title = %s, want 'Full Title'", headers["Title"])
	}

	if headers["Priority"] != "urgent" {
		t.Errorf("Priority = %s, want 'urgent'", headers["Priority"])
	}

	if headers["Tags"] != "tag1,tag2,tag3" {
		t.Errorf("Tags = %s, want 'tag1,tag2,tag3'", headers["Tags"])
	}

	if headers["Click"] != "https://example.com/click" {
		t.Errorf("Click = %s, want 'https://example.com/click'", headers["Click"])
	}
}

// TestNtfyInvalidPriority tests that invalid priorities are ignored.
func TestNtfyInvalidPriority(t *testing.T) {
	var receivedPriority string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPriority = r.Header.Get("Priority")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewNtfyClient("test-topic")
	client.URL = server.URL

	msg := NtfyMessage{
		Topic:    "test-topic",
		Message:  "test",
		Priority: "invalid",
	}

	err := client.Send(msg)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	// Invalid priority should be ignored (header not set)
	if receivedPriority != "" {
		t.Errorf("Priority header should be empty for invalid priority, got: %s", receivedPriority)
	}
}

// TestNtfyClientValidPriorities tests that all valid priorities are sent correctly.
func TestNtfyClientValidPriorities(t *testing.T) {
	validPriorities := []string{"min", "low", "default", "high", "urgent"}

	for _, priority := range validPriorities {
		t.Run(priority, func(t *testing.T) {
			var receivedPriority string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedPriority = r.Header.Get("Priority")
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			client := NewNtfyClient("test-topic")
			client.URL = server.URL

			msg := NtfyMessage{
				Topic:    "test-topic",
				Message:  "test",
				Priority: priority,
			}

			err := client.Send(msg)
			if err != nil {
				t.Fatalf("Send() error = %v", err)
			}

			if receivedPriority != priority {
				t.Errorf("Priority = %s, want %s", receivedPriority, priority)
			}
		})
	}
}

// TestNtfyClientMessageFieldPriority tests that message priority overrides client default.
func TestNtfyClientMessageFieldPriority(t *testing.T) {
	var receivedPriority string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPriority = r.Header.Get("Priority")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewNtfyClient("test-topic")
	client.URL = server.URL
	client.Priority = "low" // Client default

	msg := NtfyMessage{
		Topic:    "test-topic",
		Message:  "test",
		Priority: "urgent", // Message priority should override
	}

	err := client.Send(msg)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if receivedPriority != "urgent" {
		t.Errorf("Priority = %s, want urgent (message priority should override client default)", receivedPriority)
	}
}

// TestNtfyClientEmptyMessage tests sending a notification with empty message body.
func TestNtfyClientEmptyMessage(t *testing.T) {
	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBuf := new(bytes.Buffer)
		bodyBuf.ReadFrom(r.Body)
		receivedBody = bodyBuf.String()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewNtfyClient("test-topic")
	client.URL = server.URL

	msg := NtfyMessage{
		Topic:   "test-topic",
		Title:   "Title Only",
		Message: "", // Empty body is valid
	}

	err := client.Send(msg)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if receivedBody != "" {
		t.Errorf("Body = %s, want empty string", receivedBody)
	}
}

// TestNtfyClientCustomURL tests using a custom ntfy server URL.
func TestNtfyClientCustomURL(t *testing.T) {
	var receivedHost string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHost = r.Host
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewNtfyClient("test-topic")
	client.SetURL(server.URL) // Use custom URL

	msg := NtfyMessage{
		Topic:   "test-topic",
		Message: "test",
	}

	err := client.Send(msg)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if receivedHost == "" {
		t.Error("Expected request to be sent to custom URL host")
	}
}

// TestNtfyClientClickHeader tests the Click header functionality.
func TestNtfyClientClickHeader(t *testing.T) {
	var receivedClick string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedClick = r.Header.Get("Click")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewNtfyClient("test-topic")
	client.URL = server.URL
	client.Click = "https://example.com/default" // Client default

	msg := NtfyMessage{
		Topic:   "test-topic",
		Message: "test",
		Click:   "https://example.com/specific", // Message click should override
	}

	err := client.Send(msg)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if receivedClick != "https://example.com/specific" {
		t.Errorf("Click = %s, want https://example.com/specific", receivedClick)
	}
}

// TestNtfyClientEmailHeader tests the Email header functionality.
func TestNtfyClientEmailHeader(t *testing.T) {
	var receivedEmail string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedEmail = r.Header.Get("Email")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewNtfyClient("test-topic")
	client.URL = server.URL

	msg := NtfyMessage{
		Topic:   "test-topic",
		Message: "test",
		Email:   "user@example.com",
	}

	err := client.Send(msg)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if receivedEmail != "user@example.com" {
		t.Errorf("Email = %s, want user@example.com", receivedEmail)
	}
}

// NewNtfyTopic is a convenience function for creating a client with just a topic.
func NewNtfyTopic(topic string) *NtfyClient {
	return &NtfyClient{
		URL:        "https://ntfy.sh",
		Topic:      topic,
		HTTPClient: &http.Client{},
	}
}
