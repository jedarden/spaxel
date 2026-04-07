// Package api provides REST API handlers for Spaxel notification channels.
package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-chi/chi"
	_ "modernc.org/sqlite"
)

// NotificationsHandler manages notification delivery channels.
// Supported channel types: ntfy, pushover, gotify, webhook, mqtt.
type NotificationsHandler struct {
	mu            sync.RWMutex
	db            *sql.DB
	channels      map[string]*NotificationChannel
	notifyService NotifySender
}

// NotificationChannel represents a notification delivery channel configuration.
//
// Channel types and their config schemas:
//
//	ntfy:    {"url":"https://ntfy.sh/my-topic", "token":"tk_..."} (token optional)
//	pushover: {"app_token":"aXXXXXX...","user_key":"uXXXXXX..."}
//	gotify:  {"url":"https://gotify.example.com","token":"Aq7mXXXX"}
//	webhook: {"url":"https://example.com/hook","method":"POST","headers":{"X-Secret":"abc"}}
//	mqtt:    {} (uses global MQTT connection; no config needed)
type NotificationChannel struct {
	Type    string      `json:"type"`             // ntfy, pushover, gotify, webhook, mqtt
	Enabled bool        `json:"enabled"`          // true if channel is active
	Config  interface{} `json:"config,omitempty"` // channel-specific configuration
}

// NotifySender is the interface for sending test notifications.
type NotifySender interface {
	Send(title, body string, data map[string]interface{}) error
}

// NewNotificationsHandler creates a new notifications handler.
func NewNotificationsHandler(dbPath string) (*NotificationsHandler, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	n := &NotificationsHandler{
		db:       db,
		channels: make(map[string]*NotificationChannel),
	}

	if err := n.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	if err := n.load(); err != nil {
		log.Printf("[WARN] Failed to load notification channels: %v", err)
	}

	return n, nil
}

func (n *NotificationsHandler) migrate() error {
	_, err := n.db.Exec(`
		CREATE TABLE IF NOT EXISTS notification_channels (
			type        TEXT PRIMARY KEY,
			enabled     INTEGER NOT NULL DEFAULT 0,
			config_json TEXT    NOT NULL DEFAULT '{}',
			updated_at  INTEGER NOT NULL DEFAULT (unixepoch() * 1000)
		);
	`)
	return err
}

func (n *NotificationsHandler) load() error {
	rows, err := n.db.Query(`SELECT type, enabled, config_json FROM notification_channels`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var nc NotificationChannel
		var enabled int
		var configJSON string

		if err := rows.Scan(&nc.Type, &enabled, &configJSON); err != nil {
			log.Printf("[WARN] Failed to scan notification channel: %v", err)
			continue
		}

		nc.Enabled = enabled != 0
		if err := json.Unmarshal([]byte(configJSON), &nc.Config); err != nil {
			log.Printf("[WARN] Failed to unmarshal config for %s: %v", nc.Type, err)
			// Keep as raw JSON string if unmarshaling fails
			nc.Config = configJSON
		}

		n.channels[nc.Type] = &nc
	}

	return nil
}

// Close closes the database.
func (n *NotificationsHandler) Close() error {
	return n.db.Close()
}

// SetNotifyService sets the notification sender for test notifications.
func (n *NotificationsHandler) SetNotifyService(ns NotifySender) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.notifyService = ns
}

// GetChannels returns a copy of all notification channels.
func (n *NotificationsHandler) GetChannels() map[string]*NotificationChannel {
	n.mu.RLock()
	defer n.mu.RUnlock()

	result := make(map[string]*NotificationChannel, len(n.channels))
	for k, v := range n.channels {
		result[k] = v
	}
	return result
}

// GetChannel returns a single channel by type.
func (n *NotificationsHandler) GetChannel(channelType string) (*NotificationChannel, bool) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	ch, ok := n.channels[channelType]
	return ch, ok
}

// SetChannel updates or creates a notification channel.
func (n *NotificationsHandler) SetChannel(channelType string, enabled bool, config interface{}) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Validate config based on channel type
	if err := validateChannelConfig(channelType, config); err != nil {
		return err
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		return err
	}

	enabledInt := 0
	if enabled {
		enabledInt = 1
	}

	now := time.Now().UnixMilli()
	_, err = n.db.Exec(`
		INSERT INTO notification_channels (type, enabled, config_json, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(type) DO UPDATE SET enabled = ?, config_json = ?, updated_at = ?
	`, channelType, enabledInt, string(configJSON), now,
		enabledInt, string(configJSON), now)
	if err != nil {
		return err
	}

	n.channels[channelType] = &NotificationChannel{
		Type:    channelType,
		Enabled: enabled,
		Config:  config,
	}

	return nil
}

// validateChannelConfig validates the configuration for a specific channel type.
func validateChannelConfig(channelType string, config interface{}) error {
	if config == nil {
		// Nil config is valid for mqtt, optional for others
		return nil
	}

	configMap, ok := config.(map[string]interface{})
	if !ok {
		// Try to unmarshal as JSON
		jsonBytes, err := json.Marshal(config)
		if err != nil {
			return &ChannelValidationError{Type: channelType, Reason: "config must be a JSON object"}
		}
		if err := json.Unmarshal(jsonBytes, &configMap); err != nil {
			return &ChannelValidationError{Type: channelType, Reason: "config must be a JSON object"}
		}
	}

	switch channelType {
	case "ntfy":
		// url is required, token is optional
		if _, ok := configMap["url"]; !ok {
			return &ChannelValidationError{Type: channelType, Field: "url", Reason: "required field missing"}
		}
	case "pushover":
		// app_token and user_key are required
		if _, ok := configMap["app_token"]; !ok {
			return &ChannelValidationError{Type: channelType, Field: "app_token", Reason: "required field missing"}
		}
		if _, ok := configMap["user_key"]; !ok {
			return &ChannelValidationError{Type: channelType, Field: "user_key", Reason: "required field missing"}
		}
	case "gotify":
		// url and token are required
		if _, ok := configMap["url"]; !ok {
			return &ChannelValidationError{Type: channelType, Field: "url", Reason: "required field missing"}
		}
		if _, ok := configMap["token"]; !ok {
			return &ChannelValidationError{Type: channelType, Field: "token", Reason: "required field missing"}
		}
	case "webhook":
		// url is required, method and headers are optional
		if _, ok := configMap["url"]; !ok {
			return &ChannelValidationError{Type: channelType, Field: "url", Reason: "required field missing"}
		}
		// Validate method if provided
		if method, ok := configMap["method"].(string); ok {
			if method != "GET" && method != "POST" {
				return &ChannelValidationError{Type: channelType, Field: "method", Reason: "must be GET or POST"}
			}
		}
	case "mqtt":
		// No config required for mqtt (uses global connection)
	default:
		return &ChannelValidationError{Type: channelType, Reason: "unknown channel type"}
	}

	return nil
}

// ChannelValidationError represents a configuration validation error.
type ChannelValidationError struct {
	Type   string
	Field  string
	Reason string
}

func (e *ChannelValidationError) Error() string {
	if e.Field != "" {
		return e.Type + "." + e.Field + ": " + e.Reason
	}
	return e.Type + ": " + e.Reason
}

// RegisterRoutes registers notification endpoints.
//
// Notification Channels Endpoints:
//
// The notification channels API manages delivery channels for alerts and notifications.
// Supported channel types: ntfy, pushover, gotify, webhook, mqtt.
//
// GET /api/notifications/config
//
//	@Summary		Get notification channel configuration
//	@Description	Returns all notification channel configurations including enabled status and channel-specific settings.
//	@Tags			notifications
//	@Produce		json
//	@Success		200	{object}	notificationConfigResponse	"Channel configurations"
//	@Router			/api/notifications/config [get]
//
// POST /api/notifications/config
//
//	@Summary		Update notification channel configuration
//	@Description	Updates one or more notification channel configurations. Each channel has type-specific required fields.
//	@Description	<br>ntfy: requires "url", optional "token"<br>
//	@Description	pushover: requires "app_token", "user_key"<br>
//	@Description	gotify: requires "url", "token"<br>
//	@Description	webhook: requires "url", optional "method" (GET/POST), optional "headers"<br>
//	@Description	mqtt: no config required (uses global MQTT connection)
//	@Tags			notifications
//	@Accept			json
//	@Produce		json
//	@Param			request	body		setNotificationConfigRequest	true	"Channel configurations to update"
//	@Success		200	{object}	notificationConfigResponse	"Updated channel configurations"
//	@Failure		400	{object}	map[string]string	"Invalid request body or validation error"
//	@Failure		500	{object}	map[string]string	"Failed to save configuration"
//	@Router			/api/notifications/config [post]
//
// POST /api/notifications/test
//
//	@Summary		Send a test notification
//	@Description	Sends a test notification via the specified channel type. The channel must be enabled.
//	@Tags			notifications
//	@Accept			json
//	@Produce		json
//	@Param			request	body		testNotificationRequest	true	"Test notification parameters"
//	@Success		200	{object}	map[string]interface{}	"Test result"
//	@Failure		400	{object}	map[string]string	"Invalid request or no enabled channel"
//	@Failure		500	{object}	map[string]string	"Failed to send notification"
//	@Router			/api/notifications/test [post]
func (n *NotificationsHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/notifications/config", n.handleGetConfig)
	r.Post("/api/notifications/config", n.handleSetConfig)
	r.Post("/api/notifications/test", n.handleSendTest)
}

// notificationConfigResponse is the response for channel configuration requests.
type notificationConfigResponse struct {
	Channels map[string]*NotificationChannel `json:"channels"`
}

// handleGetConfig handles GET /api/notifications/config requests.
func (n *NotificationsHandler) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, notificationConfigResponse{
		Channels: n.GetChannels(),
	})
}

// setNotificationConfigRequest is the request body for setting channel configuration.
type setNotificationConfigRequest struct {
	Channels map[string]struct {
		Type    string      `json:"type"`
		Enabled bool        `json:"enabled"`
		Config  interface{} `json:"config,omitempty"`
	} `json:"channels"`
}

// handleSetConfig handles POST /api/notifications/config requests.
func (n *NotificationsHandler) handleSetConfig(w http.ResponseWriter, r *http.Request) {
	var req setNotificationConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	// Validate and update each channel
	for channelType, ch := range req.Channels {
		if ch.Type == "" {
			ch.Type = channelType // Use map key as type if not specified
		}
		if ch.Type != channelType {
			writeJSONError(w, http.StatusBadRequest, "channel type mismatch: key is "+channelType+" but body specifies "+ch.Type)
			return
		}

		if err := n.SetChannel(ch.Type, ch.Enabled, ch.Config); err != nil {
			if ce, ok := err.(*ChannelValidationError); ok {
				writeJSONError(w, http.StatusBadRequest, ce.Error())
			} else {
				writeJSONError(w, http.StatusInternalServerError, "failed to save configuration: "+err.Error())
			}
			return
		}
	}

	n.handleGetConfig(w, r)
}

// testNotificationRequest is the request body for sending a test notification.
type testNotificationRequest struct {
	ChannelType string                 `json:"channel_type"` // ntfy, pushover, gotify, webhook, mqtt
	Title       string                 `json:"title"`        // Custom title (optional)
	Body        string                 `json:"body"`         // Custom body (optional)
	Data        map[string]interface{} `json:"data,omitempty"` // Additional data (optional)
}

// testNotificationResponse is the response for a test notification.
type testNotificationResponse struct {
	Status  string `json:"status"`  // "sent" or "simulated"
	Message string `json:"message"` // Human-readable result
}

// handleSendTest handles POST /api/notifications/test requests.
func (n *NotificationsHandler) handleSendTest(w http.ResponseWriter, r *http.Request) {
	var req testNotificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	// Set defaults
	if req.Title == "" {
		req.Title = "Spaxel Test Notification"
	}
	if req.Body == "" {
		req.Body = "This is a test notification from Spaxel."
	}
	if req.Data == nil {
		req.Data = make(map[string]interface{})
	}
	req.Data["test"] = true

	// Check if channel type exists and is enabled
	ch, ok := n.GetChannel(req.ChannelType)
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "unknown channel type: "+req.ChannelType)
		return
	}
	if !ch.Enabled {
		writeJSONError(w, http.StatusBadRequest, "channel is not enabled: "+req.ChannelType)
		return
	}

	// Send test notification
	n.mu.RLock()
	sender := n.notifyService
	n.mu.RUnlock()

	if sender == nil {
		writeJSON(w, http.StatusOK, testNotificationResponse{
			Status:  "simulated",
			Message: "Test notification simulated (no sender attached)",
		})
		return
	}

	if err := sender.Send(req.Title, req.Body, req.Data); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to send notification: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, testNotificationResponse{
		Status:  "sent",
		Message: "Test notification sent successfully",
	})
}

// ── Notification sending (called by automation engine) ────────────────────────────

// SendNotification sends a notification via all enabled channels.
func (n *NotificationsHandler) SendNotification(title, body string, data map[string]interface{}) error {
	n.mu.RLock()
	sender := n.notifyService
	channels := make([]NotificationChannel, 0, len(n.channels))
	for _, ch := range n.channels {
		if ch.Enabled {
			channels = append(channels, *ch)
		}
	}
	n.mu.RUnlock()

	if len(channels) == 0 {
		return nil
	}

	if sender == nil {
		log.Printf("[INFO] No notification sender attached, skipping: %s", title)
		return nil
	}

	return sender.Send(title, body, data)
}
