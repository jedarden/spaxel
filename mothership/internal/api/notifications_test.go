// Package api provides REST API handlers for Spaxel notification channels.
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"
)

func TestNotificationsHandler(t *testing.T) {
	// Create a temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "notifications.db")

	handler, err := NewNotificationsHandler(dbPath)
	if err != nil {
		t.Fatalf("Failed to create notifications handler: %v", err)
	}
	defer handler.Close()

	// Create a test router
	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	t.Run("GET /api/notifications/config - initial empty state", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/notifications/config", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var resp notificationConfigResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if len(resp.Channels) != 0 {
			t.Errorf("Expected 0 channels, got %d", len(resp.Channels))
		}
	})

	t.Run("POST /api/notifications/config - set ntfy channel", func(t *testing.T) {
		reqBody := setNotificationConfigRequest{
			Channels: map[string]struct {
				Type    string      `json:"type"`
				Enabled bool        `json:"enabled"`
				Config  interface{} `json:"config,omitempty"`
			}{
				"ntfy": {
					Type:    "ntfy",
					Enabled: true,
					Config: map[string]string{
						"url":   "https://ntfy.sh/my-topic",
						"token": "tk_test123",
					},
				},
			},
		}

		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/notifications/config", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp notificationConfigResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if len(resp.Channels) != 1 {
			t.Errorf("Expected 1 channel, got %d", len(resp.Channels))
		}

		ntfy, ok := resp.Channels["ntfy"]
		if !ok {
			t.Fatal("ntfy channel not found")
		}

		if !ntfy.Enabled {
			t.Error("Expected ntfy channel to be enabled")
		}
	})

	t.Run("POST /api/notifications/config - validation error: missing required field", func(t *testing.T) {
		reqBody := setNotificationConfigRequest{
			Channels: map[string]struct {
				Type    string      `json:"type"`
				Enabled bool        `json:"enabled"`
				Config  interface{} `json:"config,omitempty"`
			}{
				"pushover": {
					Type:    "pushover",
					Enabled: true,
					Config: map[string]string{
						"app_token": "test123",
						// missing user_key
					},
				},
			},
		}

		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/notifications/config", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d: %s", w.Code, w.Body.String())
		}

		var errResp map[string]string
		if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
			t.Fatalf("Failed to decode error response: %v", err)
		}

		if errResp["error"] == "" {
			t.Error("Expected error message in response")
		}
	})

	t.Run("POST /api/notifications/config - multiple channels", func(t *testing.T) {
		reqBody := setNotificationConfigRequest{
			Channels: map[string]struct {
				Type    string      `json:"type"`
				Enabled bool        `json:"enabled"`
				Config  interface{} `json:"config,omitempty"`
			}{
				"gotify": {
					Type:    "gotify",
					Enabled: true,
					Config: map[string]string{
						"url":   "https://gotify.example.com",
						"token": "Aq7mXXXX",
					},
				},
				"webhook": {
					Type:    "webhook",
					Enabled: false,
					Config: map[string]interface{}{
						"url":    "https://example.com/hook",
						"method": "POST",
						"headers": map[string]string{
							"X-Secret": "abc",
						},
					},
				},
				"mqtt": {
					Type:    "mqtt",
					Enabled: true,
					Config:  map[string]string{}, // no config needed
				},
			},
		}

		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/notifications/config", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp notificationConfigResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Should have 4 channels total (ntfy from previous test + gotify, webhook, mqtt)
		if len(resp.Channels) != 4 {
			t.Errorf("Expected 4 channels, got %d", len(resp.Channels))
		}

		// Verify gotify
		gotify, ok := resp.Channels["gotify"]
		if !ok || !gotify.Enabled {
			t.Error("gotify channel not found or not enabled")
		}

		// Verify webhook is disabled
		webhook, ok := resp.Channels["webhook"]
		if !ok || webhook.Enabled {
			t.Error("webhook channel not found or should be disabled")
		}

		// Verify mqtt
		mqtt, ok := resp.Channels["mqtt"]
		if !ok || !mqtt.Enabled {
			t.Error("mqtt channel not found or not enabled")
		}
	})

	t.Run("POST /api/notifications/test - no sender attached (simulated)", func(t *testing.T) {
		reqBody := testNotificationRequest{
			ChannelType: "ntfy",
			Title:       "Test Alert",
			Body:        "This is a test notification",
		}

		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/notifications/test", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp testNotificationResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if resp.Status != "simulated" {
			t.Errorf("Expected status 'simulated', got '%s'", resp.Status)
		}
	})

	t.Run("POST /api/notifications/test - unknown channel type", func(t *testing.T) {
		reqBody := testNotificationRequest{
			ChannelType: "unknown",
		}

		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/notifications/test", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("POST /api/notifications/test - disabled channel", func(t *testing.T) {
		reqBody := testNotificationRequest{
			ChannelType: "webhook", // webhook was set to disabled
		}

		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/notifications/test", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("POST /api/notifications/test - with custom sender", func(t *testing.T) {
		// Create a mock sender
		mockSender := &mockNotifySender{}
		handler.SetNotifyService(mockSender)

		reqBody := testNotificationRequest{
			ChannelType: "ntfy",
			Title:       "Custom Title",
			Body:        "Custom Body",
			Data: map[string]interface{}{
				"priority": "high",
			},
		}

		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/notifications/test", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		if !mockSender.called {
			t.Error("Expected sender.Send to be called")
		}

		if mockSender.title != "Custom Title" {
			t.Errorf("Expected title 'Custom Title', got '%s'", mockSender.title)
		}

		if mockSender.body != "Custom Body" {
			t.Errorf("Expected body 'Custom Body', got '%s'", mockSender.body)
		}
	})
}

// mockNotifySender is a test implementation of NotifySender.
type mockNotifySender struct {
	called bool
	title  string
	body   string
	data   map[string]interface{}
}

func (m *mockNotifySender) Send(title, body string, data map[string]interface{}) error {
	m.called = true
	m.title = title
	m.body = body
	m.data = data
	return nil
}

func TestValidateChannelConfig(t *testing.T) {
	tests := []struct {
		name        string
		channelType string
		config      interface{}
		wantErr     bool
		errField    string
	}{
		{
			name:        "ntfy - valid config",
			channelType: "ntfy",
			config: map[string]string{
				"url":   "https://ntfy.sh/my-topic",
				"token": "tk_test",
			},
			wantErr: false,
		},
		{
			name:        "ntfy - missing url",
			channelType: "ntfy",
			config: map[string]string{
				"token": "tk_test",
			},
			wantErr:  true,
			errField: "url",
		},
		{
			name:        "ntfy - url only (token optional)",
			channelType: "ntfy",
			config: map[string]string{
				"url": "https://ntfy.sh/my-topic",
			},
			wantErr: false,
		},
		{
			name:        "pushover - valid config",
			channelType: "pushover",
			config: map[string]string{
				"app_token": "aXXXXXX",
				"user_key":  "uXXXXXX",
			},
			wantErr: false,
		},
		{
			name:        "pushover - missing app_token",
			channelType: "pushover",
			config: map[string]string{
				"user_key": "uXXXXXX",
			},
			wantErr:  true,
			errField: "app_token",
		},
		{
			name:        "pushover - missing user_key",
			channelType: "pushover",
			config: map[string]string{
				"app_token": "aXXXXXX",
			},
			wantErr:  true,
			errField: "user_key",
		},
		{
			name:        "gotify - valid config",
			channelType: "gotify",
			config: map[string]string{
				"url":   "https://gotify.example.com",
				"token": "Aq7mXXXX",
			},
			wantErr: false,
		},
		{
			name:        "gotify - missing url",
			channelType: "gotify",
			config: map[string]string{
				"token": "Aq7mXXXX",
			},
			wantErr:  true,
			errField: "url",
		},
		{
			name:        "gotify - missing token",
			channelType: "gotify",
			config: map[string]string{
				"url": "https://gotify.example.com",
			},
			wantErr:  true,
			errField: "token",
		},
		{
			name:        "webhook - valid config with all fields",
			channelType: "webhook",
			config: map[string]interface{}{
				"url":    "https://example.com/hook",
				"method": "POST",
				"headers": map[string]string{
					"X-Secret": "abc",
				},
			},
			wantErr: false,
		},
		{
			name:        "webhook - url only",
			channelType: "webhook",
			config: map[string]string{
				"url": "https://example.com/hook",
			},
			wantErr: false,
		},
		{
			name:        "webhook - missing url",
			channelType: "webhook",
			config: map[string]string{
				"method": "POST",
			},
			wantErr:  true,
			errField: "url",
		},
		{
			name:        "webhook - invalid method",
			channelType: "webhook",
			config: map[string]string{
				"url":    "https://example.com/hook",
				"method": "DELETE",
			},
			wantErr:  true,
			errField: "method",
		},
		{
			name:        "mqtt - no config needed",
			channelType: "mqtt",
			config:      map[string]string{},
			wantErr:     false,
		},
		{
			name:        "mqtt - nil config",
			channelType: "mqtt",
			config:      nil,
			wantErr:     false,
		},
		{
			name:        "unknown channel type",
			channelType: "unknown",
			config:      map[string]string{},
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateChannelConfig(tt.channelType, tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateChannelConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errField != "" {
				ce, ok := err.(*ChannelValidationError)
				if !ok {
					t.Errorf("Expected ChannelValidationError, got %T", err)
					return
				}
				if ce.Field != tt.errField {
					t.Errorf("Expected error field '%s', got '%s'", tt.errField, ce.Field)
				}
			}
		})
	}
}

func TestNotificationsHandlerPersistence(t *testing.T) {
	// Create a temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "notifications.db")

	// Create first handler and set some channels
	h1, err := NewNotificationsHandler(dbPath)
	if err != nil {
		t.Fatalf("Failed to create first handler: %v", err)
	}

	err = h1.SetChannel("ntfy", true, map[string]string{
		"url":   "https://ntfy.sh/test",
		"token": "tk_test",
	})
	if err != nil {
		t.Fatalf("Failed to set channel: %v", err)
	}

	err = h1.SetChannel("pushover", false, map[string]string{
		"app_token": "a123",
		"user_key":  "u456",
	})
	if err != nil {
		t.Fatalf("Failed to set channel: %v", err)
	}

	h1.Close()

	// Create second handler with same database - should load persisted channels
	h2, err := NewNotificationsHandler(dbPath)
	if err != nil {
		t.Fatalf("Failed to create second handler: %v", err)
	}
	defer h2.Close()

	channels := h2.GetChannels()

	if len(channels) != 2 {
		t.Errorf("Expected 2 channels, got %d", len(channels))
	}

	// Verify ntfy channel
	ntfy, ok := channels["ntfy"]
	if !ok {
		t.Fatal("ntfy channel not found")
	}
	if !ntfy.Enabled {
		t.Error("Expected ntfy to be enabled")
	}
	config, ok := ntfy.Config.(map[string]interface{})
	if !ok {
		t.Fatal("ntfy config is not a map")
	}
	if config["url"] != "https://ntfy.sh/test" {
		t.Errorf("Expected url 'https://ntfy.sh/test', got '%v'", config["url"])
	}

	// Verify pushover channel
	pushover, ok := channels["pushover"]
	if !ok {
		t.Fatal("pushover channel not found")
	}
	if pushover.Enabled {
		t.Error("Expected pushover to be disabled")
	}
}

func TestNotificationsHandlerSendNotification(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "notifications.db")

	handler, err := NewNotificationsHandler(dbPath)
	if err != nil {
		t.Fatalf("Failed to create notifications handler: %v", err)
	}
	defer handler.Close()

	// Set up a mock sender
	mockSender := &mockNotifySender{}
	handler.SetNotifyService(mockSender)

	// No channels enabled - should not call sender
	err = handler.SendNotification("Test", "Body", nil)
	if err != nil {
		t.Errorf("SendNotification() with no channels should not error, got: %v", err)
	}
	if mockSender.called {
		t.Error("Expected sender not to be called when no channels enabled")
	}

	// Enable a channel
	err = handler.SetChannel("ntfy", true, map[string]string{"url": "https://ntfy.sh/test"})
	if err != nil {
		t.Fatalf("Failed to set channel: %v", err)
	}

	// Now SendNotification should call sender
	err = handler.SendNotification("Test Title", "Test Body", map[string]interface{}{"key": "value"})
	if err != nil {
		t.Errorf("SendNotification() error = %v", err)
	}
	if !mockSender.called {
		t.Error("Expected sender to be called")
	}
	if mockSender.title != "Test Title" {
		t.Errorf("Expected title 'Test Title', got '%s'", mockSender.title)
	}
}

func TestNewNotificationsHandlerWithPath(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create a handler with path
	handler, err := NewNotificationsHandler(dbPath)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}
	defer handler.Close()

	// Verify the database file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Database file was not created")
	}
}

func TestChannelValidationError(t *testing.T) {
	err := &ChannelValidationError{
		Type:   "ntfy",
		Field:  "url",
		Reason: "required field missing",
	}

	expected := "ntfy.url: required field missing"
	if err.Error() != expected {
		t.Errorf("Expected error '%s', got '%s'", expected, err.Error())
	}

	// Error without field
	err2 := &ChannelValidationError{
		Type:   "unknown",
		Reason: "unknown channel type",
	}

	expected2 := "unknown: unknown channel type"
	if err2.Error() != expected2 {
		t.Errorf("Expected error '%s', got '%s'", expected2, err2.Error())
	}
}

// Helper function to read all of response body
func readAll(r io.Reader) string {
	b, _ := io.ReadAll(r)
	return string(b)
}

// TestNotificationsTestEndpointIntegration tests the full integration flow
// from the HTTP test endpoint through to actual HTTP delivery.
func TestNotificationsTestEndpointIntegration(t *testing.T) {
	// Create a mock HTTP server to receive the notification
	var receivedMethod, receivedPath, receivedTitle, receivedBody string
	receivedHeaders := make(map[string]string)
	receivedData := make(map[string]interface{})
	serverCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true
		receivedMethod = r.Method
		receivedPath = r.URL.Path

		// Capture headers
		receivedHeaders["Title"] = r.Header.Get("Title")
		receivedHeaders["Content-Type"] = r.Header.Get("Content-Type")

		// Capture body
		bodyBuf := new(bytes.Buffer)
		bodyBuf.ReadFrom(r.Body)
		receivedBody = bodyBuf.String()

		// Decode data from query params (for test endpoint integration)
		if dataStr := r.URL.Query().Get("data"); dataStr != "" {
			if err := json.Unmarshal([]byte(dataStr), &receivedData); err == nil {
				// Successfully parsed data
			}
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create a temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "notifications.db")

	handler, err := NewNotificationsHandler(dbPath)
	if err != nil {
		t.Fatalf("Failed to create notifications handler: %v", err)
	}
	defer handler.Close()

	// Set up an ntfy channel pointing to the mock server
	err = handler.SetChannel("ntfy", true, map[string]string{
		"url": server.URL,
	})
	if err != nil {
		t.Fatalf("Failed to set ntfy channel: %v", err)
	}

	// Create an adapter that implements NotifySender using a real ntfy client
	ntfyAdapter := &ntfyNotifyAdapter{
		client: &ntfyClient{
			url: server.URL,
		},
	}
	handler.SetNotifyService(ntfyAdapter)

	// Create a test router
	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	t.Run("POST /api/notifications/test - integration with ntfy delivery", func(t *testing.T) {
		// Reset server state
		serverCalled = false
		receivedTitle = ""
		receivedBody = ""

		reqBody := testNotificationRequest{
			ChannelType: "ntfy",
			Title:       "Integration Test Notification",
			Body:        "This is an integration test of the notification endpoint",
			Data: map[string]interface{}{
				"test":     true,
				"priority": "high",
			},
		}

		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/notifications/test", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp testNotificationResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if resp.Status != "sent" {
			t.Errorf("Expected status 'sent', got '%s'", resp.Status)
		}

		// Verify the mock server received the notification
		if !serverCalled {
			t.Error("Expected mock server to be called")
		}

		if receivedMethod != "POST" {
			t.Errorf("Expected method POST, got %s", receivedMethod)
		}

		// The ntfy client appends the topic to the URL
		if receivedPath == "" {
			t.Error("Expected non-empty path")
		}

		if receivedTitle != "Integration Test Notification" {
			t.Errorf("Expected title 'Integration Test Notification', got '%s'", receivedTitle)
		}

		if receivedBody != "This is an integration test of the notification endpoint" {
			t.Errorf("Expected body 'This is an integration test of the notification endpoint', got '%s'", receivedBody)
		}

		if receivedHeaders["Content-Type"] != "text/plain" {
			t.Errorf("Expected Content-Type 'text/plain', got '%s'", receivedHeaders["Content-Type"])
		}
	})

	t.Run("POST /api/notifications/test - integration with webhook delivery", func(t *testing.T) {
		// Reset server state
		serverCalled = false
		receivedBody = ""

		// Create a mock server for webhook
		var receivedPayload map[string]interface{}
		webhookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			serverCalled = true
			if err := json.NewDecoder(r.Body).Decode(&receivedPayload); err != nil {
				t.Errorf("Failed to decode webhook payload: %v", err)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer webhookServer.Close()

		// Set up a webhook channel pointing to the mock server
		err = handler.SetChannel("webhook", true, map[string]string{
			"url": webhookServer.URL,
		})
		if err != nil {
			t.Fatalf("Failed to set webhook channel: %v", err)
		}

		// Create an adapter that implements NotifySender using a real webhook client
		webhookAdapter := &webhookNotifyAdapter{
			client: &webhookClient{
				url: webhookServer.URL,
			},
		}
		handler.SetNotifyService(webhookAdapter)

		reqBody := testNotificationRequest{
			ChannelType: "webhook",
			Title:       "Webhook Integration Test",
			Body:        "Testing webhook delivery through test endpoint",
		}

		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/notifications/test", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp testNotificationResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if resp.Status != "sent" {
			t.Errorf("Expected status 'sent', got '%s'", resp.Status)
		}

		// Verify the mock server received the webhook payload
		if !serverCalled {
			t.Error("Expected webhook server to be called")
		}

		if receivedPayload["event_type"] != "test_notification" {
			t.Errorf("Expected event_type 'test_notification', got '%v'", receivedPayload["event_type"])
		}

		if receivedPayload["title"] != "Webhook Integration Test" {
			t.Errorf("Expected title 'Webhook Integration Test', got '%v'", receivedPayload["title"])
		}

		if receivedPayload["message"] != "Testing webhook delivery through test endpoint" {
			t.Errorf("Expected message 'Testing webhook delivery through test endpoint', got '%v'", receivedPayload["message"])
		}

		// Verify test flag is set
		if receivedPayload["metadata"] == nil {
			t.Error("Expected metadata to be present")
		} else {
			metadata, ok := receivedPayload["metadata"].(map[string]interface{})
			if !ok {
				t.Error("Expected metadata to be a map")
			} else if metadata["test"] != true {
				t.Error("Expected test=true in metadata")
			}
		}
	})
}

// ntfyNotifyAdapter implements NotifySender using a simplified ntfy client.
type ntfyNotifyAdapter struct {
	client *ntfyClient
}

func (a *ntfyNotifyAdapter) Send(title, body string, data map[string]interface{}) error {
	// Build URL (ntfy appends topic to base URL)
	url := a.client.url + "/spaxel-test"

	// Create request body
	reqBody := body

	// Create request
	req, err := http.NewRequest("POST", url, bytes.NewBufferString(reqBody))
	if err != nil {
		return err
	}

	// Set headers
	req.Header.Set("Content-Type", "text/plain")
	if title != "" {
		req.Header.Set("Title", title)
	}

	// Send request
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy returned status %d", resp.StatusCode)
	}

	return nil
}

// webhookNotifyAdapter implements NotifySender using a simplified webhook client.
type webhookNotifyAdapter struct {
	client *webhookClient
}

func (a *webhookNotifyAdapter) Send(title, body string, data map[string]interface{}) error {
	// Build payload
	payload := map[string]interface{}{
		"event_type": "test_notification",
		"title":      title,
		"message":    body,
		"timestamp":  time.Now().Unix(),
		"metadata":   data,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	// Create request
	req, err := http.NewRequest("POST", a.client.url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Spaxel/1.0")

	// Send request
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// Simplified ntfy client for integration testing.
type ntfyClient struct {
	url string
}

// Simplified webhook client for integration testing.
type webhookClient struct {
	url string
}
