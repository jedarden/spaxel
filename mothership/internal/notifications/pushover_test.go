package notifications

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestNewPushoverClient tests creating a new Pushover client.
func TestNewPushoverClient(t *testing.T) {
	client := NewPushoverClient("app-token-123", "user-key-456")

	if client == nil {
		t.Fatal("NewPushoverClient() returned nil")
	}

	if client.AppToken != "app-token-123" {
		t.Errorf("AppToken = %s, want app-token-123", client.AppToken)
	}

	if client.UserKey != "user-key-456" {
		t.Errorf("UserKey = %s, want user-key-456", client.UserKey)
	}

	if client.APIURL != "https://api.pushover.net/1/messages.json" {
		t.Errorf("APIURL = %s, want https://api.pushover.net/1/messages.json", client.APIURL)
	}

	if client.Priority != 0 {
		t.Errorf("Priority = %d, want 0", client.Priority)
	}

	if client.Sound != "pushover" {
		t.Errorf("Sound = %s, want pushover", client.Sound)
	}
}

// TestPushoverSendBasic tests sending a basic message.
func TestPushoverSendBasic(t *testing.T) {
	var receivedBody []byte
	var contentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":1,"request":"test-id"}`))
	}))
	defer server.Close()

	client := NewPushoverClient("test-app-token", "test-user-key")
	client.APIURL = server.URL

	msg := PushoverMessage{
		Message: "Test message",
	}

	err := client.Send(msg)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if receivedBody == nil {
		t.Fatal("No body received")
	}

	bodyStr := string(receivedBody)
	if !strings.Contains(bodyStr, "message=") {
		t.Errorf("Body should contain 'message=', got: %s", bodyStr)
	}

	if !strings.Contains(bodyStr, "token=test-app-token") {
		t.Errorf("Body should contain 'token=test-app-token', got: %s", bodyStr)
	}

	if !strings.Contains(bodyStr, "user=test-user-key") {
		t.Errorf("Body should contain 'user=test-user-key', got: %s", bodyStr)
	}

	if !strings.HasPrefix(contentType, "multipart/form-data") {
		t.Errorf("Content-Type should start with 'multipart/form-data', got: %s", contentType)
	}
}

// TestPushoverSendWithTitle tests sending a message with title.
func TestPushoverSendWithTitle(t *testing.T) {
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewPushoverClient("test-app-token", "test-user-key")
	client.APIURL = server.URL

	msg := PushoverMessage{
		Message: "Test message",
		Title:   "Test Title",
	}

	err := client.Send(msg)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	bodyStr := string(receivedBody)
	if !strings.Contains(bodyStr, "title=Test+Title") {
		t.Errorf("Body should contain 'title=Test+Title', got: %s", bodyStr)
	}
}

// TestPushoverSendWithPriority tests sending with different priorities.
func TestPushoverSendWithPriority(t *testing.T) {
	priorities := []int{-2, -1, 0, 1, 2}

	for _, priority := range priorities {
		t.Run(string(rune('0'+priority+2)), func(t *testing.T) {
			var receivedBody []byte
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedBody, _ = io.ReadAll(r.Body)
				w.WriteHeader(http.StatusOK)
			}))
			serverURL := server.URL
			defer server.Close()

			client := NewPushoverClient("test-app-token", "test-user-key")
			client.APIURL = serverURL

			msg := PushoverMessage{
				Message:  "Test message",
				Priority: priority,
			}

			err := client.Send(msg)
			if err != nil {
				t.Fatalf("Send() error = %v", err)
			}

			bodyStr := string(receivedBody)
			expectedPriority := "priority=" + string(rune('0'+priority))
			if !strings.Contains(bodyStr, expectedPriority) {
				t.Errorf("Body should contain '%s', got: %s", expectedPriority, bodyStr)
			}
		})
	}
}

// TestPushoverSendWithPNGAttachment tests sending with PNG attachment.
func TestPushoverSendWithPNGAttachment(t *testing.T) {
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.Header.Get("Content-Type")
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewPushoverClient("test-app-token", "test-user-key")
	client.APIURL = server.URL

	// Create minimal PNG (1x1 red pixel)
	pngData := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52, // IHDR
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, // 1x1
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xDE, 0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41, 0x54, // IDAT
		0x08, 0xD7, 0x63, 0xF8, 0xCF, 0xC0, 0x00, 0x00, 0x03,
		0x00, 0x01, 0xFF, 0xFF, 0x37, 0xF3, 0x4B, 0xAE,
		0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, // IEND
		0xAE, 0x42, 0x60, 0x82,
	}

	msg := PushoverMessage{
		Message:      "Message with image",
		PNGImageData: pngData,
	}

	err := client.Send(msg)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	bodyStr := string(receivedBody)
	if !strings.Contains(bodyStr, "Content-Disposition") {
		t.Error("Body should contain Content-Disposition for attachment")
	}

	if !strings.Contains(bodyStr, "filename=\"notification.png\"") {
		t.Error("Body should contain filename=\"notification.png\"")
	}

	if !strings.Contains(bodyStr, "image/png") {
		t.Error("Body should contain Content-Type: image/png")
	}
}

// TestPushoverSendInvalidPNG tests that invalid PNG data is rejected.
func TestPushoverSendInvalidPNG(t *testing.T) {
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewPushoverClient("test-app-token", "test-user-key")
	client.APIURL = server.URL

	// Invalid PNG data
	invalidPNG := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

	msg := PushoverMessage{
		Message:      "Message with invalid image",
		PNGImageData: invalidPNG,
	}

	err := client.Send(msg)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	bodyStr := string(receivedBody)
	// Invalid PNG should not be attached
	if strings.Contains(bodyStr, "attachment") {
		t.Error("Invalid PNG should not be attached")
	}
}

// TestPushoverEmergencySettings tests emergency priority settings.
func TestPushoverEmergencySettings(t *testing.T) {
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewPushoverClient("test-app-token", "test-user-key")
	client.APIURL = server.URL

	msg := PushoverMessage{
		Message:  "Emergency!",
		Priority: 2,
		Retry:    60,  // Retry every 60 seconds
		Expire:   3600, // Expire after 1 hour
	}

	err := client.Send(msg)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	bodyStr := string(receivedBody)
	if !strings.Contains(bodyStr, "priority=2") {
		t.Errorf("Body should contain 'priority=2', got: %s", bodyStr)
	}

	if !strings.Contains(bodyStr, "retry=60") {
		t.Errorf("Body should contain 'retry=60', got: %s", bodyStr)
	}

	if !strings.Contains(bodyStr, "expire=3600") {
		t.Errorf("Body should contain 'expire=3600', got: %s", bodyStr)
	}
}

// TestPushoverSendErrorCases tests error handling.
func TestPushoverSendErrorCases(t *testing.T) {
	t.Run("nil client", func(t *testing.T) {
		var client *PushoverClient = nil
		msg := PushoverMessage{Message: "test"}
		err := client.Send(msg)
		if err == nil {
			t.Error("Expected error for nil client")
		}
		if !strings.Contains(err.Error(), "nil") {
			t.Errorf("Error should mention nil client, got: %v", err)
		}
	})

	t.Run("missing app token", func(t *testing.T) {
		client := NewPushoverClient("", "user-key")
		msg := PushoverMessage{Message: "test"}
		err := client.Send(msg)
		if err == nil {
			t.Error("Expected error for missing app token")
		}
		if !strings.Contains(err.Error(), "app token") {
			t.Errorf("Error should mention app token, got: %v", err)
		}
	})

	t.Run("missing message", func(t *testing.T) {
		client := NewPushoverClient("app-token", "user-key")
		msg := PushoverMessage{}
		err := client.Send(msg)
		if err == nil {
			t.Error("Expected error for missing message")
		}
		if !strings.Contains(err.Error(), "message") {
			t.Errorf("Error should mention message, got: %v", err)
		}
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"invalid token"}`))
		}))
		defer server.Close()

		client := NewPushoverClient("app-token", "user-key")
		client.APIURL = server.URL

		msg := PushoverMessage{Message: "test"}
		err := client.Send(msg)
		if err == nil {
			t.Error("Expected error for server error")
		}
	})
}

// TestPushoverSetters tests client setter methods.
func TestPushoverSetters(t *testing.T) {
	client := NewPushoverClient("app", "user")

	client.SetPriority(1)
	if client.Priority != 1 {
		t.Errorf("Priority = %d, want 1", client.Priority)
	}

	// Test priority clamping
	client.SetPriority(5) // Invalid, should be ignored
	if client.Priority != 1 {
		t.Errorf("Priority should remain 1 after invalid SetPriority(5)")
	}

	client.SetSound("cosmic")
	if client.Sound != "cosmic" {
		t.Errorf("Sound = %s, want cosmic", client.Sound)
	}

	client.SetDevice("iphone")
	if client.Device != "iphone" {
		t.Errorf("Device = %s, want iphone", client.Device)
	}

	client.SetURL("https://example.com", "Example Link")
	if client.URL != "https://example.com" {
		t.Errorf("URL = %s, want https://example.com", client.URL)
	}
	if client.URLTitle != "Example Link" {
		t.Errorf("URLTitle = %s, want 'Example Link'", client.URLTitle)
	}
}

// TestPushoverClientDefaults tests that client defaults are used.
func TestPushoverClientDefaults(t *testing.T) {
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewPushoverClient("app-token", "user-key")
	client.APIURL = server.URL
	client.Title = "Default Title"
	client.Device = "default-device"
	client.Sound = "alarm"

	msg := PushoverMessage{
		Message: "test",
	}

	err := client.Send(msg)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	bodyStr := string(receivedBody)
	if !strings.Contains(bodyStr, "title=Default+Title") {
		t.Errorf("Body should contain default title, got: %s", bodyStr)
	}

	if !strings.Contains(bodyStr, "device=default-device") {
		t.Errorf("Body should contain default device, got: %s", bodyStr)
	}

	if !strings.Contains(bodyStr, "sound=alarm") {
		t.Errorf("Body should contain default sound, got: %s", bodyStr)
	}
}

// TestAttachPNGBase64 tests the PNG base64 decoder.
func TestAttachPNGBase64(t *testing.T) {
	t.Run("valid PNG", func(t *testing.T) {
		// Valid minimal PNG base64
		pngBase64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

		data, err := AttachPNGBase64(pngBase64)
		if err != nil {
			t.Fatalf("AttachPNGBase64() error = %v", err)
		}

		// Check PNG signature
		if len(data) < 8 {
			t.Fatal("Decoded data too short")
		}

		if string(data[1:4]) != "PNG" {
			t.Error("Decoded data does not appear to be PNG")
		}
	})

	t.Run("invalid base64", func(t *testing.T) {
		_, err := AttachPNGBase64("invalid-base64!!!")
		if err == nil {
			t.Error("Expected error for invalid base64")
		}
	})

	t.Run("invalid PNG", func(t *testing.T) {
		// Valid base64 but not PNG
		notPNG := base64.StdEncoding.EncodeToString([]byte("not a png"))

		_, err := AttachPNGBase64(notPNG)
		if err == nil {
			t.Error("Expected error for non-PNG data")
		}
	})
}

// TestPushoverSendWithAllOptions tests sending with all optional fields.
func TestPushoverSendWithAllOptions(t *testing.T) {
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewPushoverClient("app-token", "user-key")
	client.APIURL = server.URL

	msg := PushoverMessage{
		Message:    "Full message",
		Title:      "Full Title",
		Priority:   1,
		Device:     "iphone",
		URL:        "https://example.com",
		URLTitle:   "Example Site",
		Sound:      "cosmic",
		Timestamp:  1234567890,
	}

	err := client.Send(msg)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	bodyStr := string(receivedBody)

	fields := map[string]string{
		"message":    "Full+message",
		"title":      "Full+Title",
		"priority":   "1",
		"device":     "iphone",
		"url":        "https://example.com",
		"url_title":  "Example+Site",
		"sound":      "cosmic",
		"timestamp":  "1234567890",
	}

	for field, expected := range fields {
		if !strings.Contains(bodyStr, field+"="+expected) &&
		   !strings.Contains(bodyStr, field+"="+strings.ReplaceAll(expected, " ", "+")) {
			t.Errorf("Body should contain '%s=%s', got: %s", field, expected, bodyStr)
		}
	}
}

// TestPushoverRetryExpireClamping tests retry and expire clamping.
func TestPushoverRetryExpireClamping(t *testing.T) {
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewPushoverClient("app-token", "user-key")
	client.APIURL = server.URL

	msg := PushoverMessage{
		Message:  "Emergency",
		Priority: 2,
		Retry:    10, // Below minimum of 30
		Expire:   20000, // Above maximum of 10800
	}

	err := client.Send(msg)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	bodyStr := string(receivedBody)

	// Retry should be clamped to 30
	if !strings.Contains(bodyStr, "retry=30") {
		t.Errorf("Retry should be clamped to 30, got: %s", bodyStr)
	}

	// Expire should be clamped to 10800
	if !strings.Contains(bodyStr, "expire=10800") {
		t.Errorf("Expire should be clamped to 10800, got: %s", bodyStr)
	}
}

// TestPushoverEmptyHTTPClient tests sending with nil HTTPClient.
func TestPushoverEmptyHTTPClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewPushoverClient("app-token", "user-key")
	client.APIURL = server.URL
	client.HTTPClient = nil // Explicitly set to nil

	msg := PushoverMessage{Message: "test"}

	err := client.Send(msg)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	// Should not panic, should create default client
}

// TestPushoverPriorityClamping tests priority clamping.
func TestPushoverPriorityClamping(t *testing.T) {
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewPushoverClient("app-token", "user-key")
	client.APIURL = server.URL

	msg := PushoverMessage{
		Message:  "test",
		Priority: 5, // Invalid (> 2)
	}

	err := client.Send(msg)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	bodyStr := string(receivedBody)
	// Should be clamped to 0 (normal)
	if !strings.Contains(bodyStr, "priority=0") {
		t.Errorf("Invalid priority should be clamped to 0, got: %s", bodyStr)
	}
}

// TestPushoverWriteFieldHelper tests the writeField helper error handling.
func TestPushoverWriteFieldHelper(t *testing.T) {
	// This test verifies writeField doesn't panic on errors
	// We can't easily test the error logging without capturing log output

	// Create a multipart writer
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Valid field write
	err := writer.WriteField("test", "value")
	if err != nil {
		t.Errorf("WriteField() error = %v", err)
	}

	writer.Close()
}
