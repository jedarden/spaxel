// Package api provides REST API handlers for Spaxel integration settings.
package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

// IntegrationSettingsHandler manages home automation integration settings.
type IntegrationSettingsHandler struct {
	mu     sync.RWMutex
	db     *sql.DB

	// MQTT configuration (managed via settings table)
	mqttClient MQTTClient

	// System webhook publisher
	webhookPublisher WebhookPublisher

	// Mothership ID for HA entity IDs
	mothershipID string
}

// MQTTClient is the interface for MQTT operations.
type MQTTClient interface {
	IsConnected() bool
	GetMothershipID() string
	GetConfig() interface{}
	UpdateConfig(ctx context.Context, cfg interface{}) error
	Reconnect(ctx context.Context) error
	PublishDiscoveryNow() error
	PublishPersonPresenceDiscovery(personID, personName string) error
	PublishZoneOccupancyDiscovery(zoneID, zoneName string) error
	PublishZoneBinaryDiscovery(zoneID, zoneName string) error
	PublishFallDetectionDiscovery() error
	PublishSystemHealthDiscovery() error
	PublishSystemModeDiscovery() error
	RemovePersonDiscovery(personID string) error
	RemoveZoneDiscovery(zoneID string) error
}

// WebhookPublisher is the interface for system webhook operations.
type WebhookPublisher interface {
	UpdateConfig(cfg interface{})
	GetConfig() interface{}
	TestWebhook() error
}

// NewIntegrationSettingsHandler creates a new integration settings handler.
func NewIntegrationSettingsHandler(db *sql.DB, mothershipID string) *IntegrationSettingsHandler {
	return &IntegrationSettingsHandler{
		db:           db,
		mothershipID: mothershipID,
	}
}

// SetMQTTClient sets the MQTT client for integration operations.
func (h *IntegrationSettingsHandler) SetMQTTClient(client MQTTClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.mqttClient = client
}

// SetWebhookPublisher sets the webhook publisher.
func (h *IntegrationSettingsHandler) SetWebhookPublisher(publisher WebhookPublisher) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.webhookPublisher = publisher
}

// RegisterRoutes registers integration settings endpoints.
//
// Integration Settings Endpoints:
//
// GET /api/settings/integration
//
//	@Summary		Get integration settings
//	@Description	Returns MQTT and system webhook configuration.
//	@Tags			settings
//	@Produce		json
//	@Success		200	{object}	integrationSettingsResponse	"Integration settings"
//	@Router			/api/settings/integration [get]
//
// POST /api/settings/integration
//
//	@Summary		Update integration settings
//	@Description	Updates MQTT and/or system webhook configuration. Only the fields provided are modified.
//	@Tags			settings
//	@Accept			json
//	@Produce		json
//	@Param			request	body	integrationSettingsRequest	true	"Settings to update (partial)"
//	@Success		200	{object}	integrationSettingsResponse	"Updated settings"
//	@Failure		400	{object}	map[string]string	"Invalid request or validation error"
//	@Router			/api/settings/integration [post]
//
// POST /api/settings/integration/test
//
//	@Summary		Test integration connection
//	@Description	Sends a test event via MQTT or webhook to verify configuration.
//	@Tags			settings
//	@Accept			json
//	@Produce		json
//	@Param			request	body	integrationTestRequest	true	"Test parameters"
//	@Success		200	{object}	map[string]interface{}	"Test result"
//	@Failure		400	{object}	map[string]string	"Invalid request"
//	@Router			/api/settings/integration [post]
func (h *IntegrationSettingsHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/settings/integration", h.handleGetSettings)
	r.Post("/api/settings/integration", h.handleUpdateSettings)
	r.Post("/api/settings/integration/test", h.handleTest)
}

// integrationSettingsResponse is the response for integration settings.
type integrationSettingsResponse struct {
	MQTT      *mqttConfig      `json:"mqtt,omitempty"`
	Webhook   *webhookConfig   `json:"webhook,omitempty"`
}

// mqttConfig holds MQTT configuration.
type mqttConfig struct {
	Broker          string `json:"broker,omitempty"`           // e.g., "tcp://homeassistant.local:1883"
	Username        string `json:"username,omitempty"`        // MQTT username
	Password        string `json:"password,omitempty"`        // MQTT password (write-only, never returned)
	TLS             bool   `json:"tls,omitempty"`              // Whether to use TLS
	DiscoveryPrefix string `json:"discovery_prefix,omitempty"` // Home Assistant discovery prefix
	Connected       bool   `json:"connected"`                 // Connection status
	MothershipID    string `json:"mothership_id,omitempty"`  // Unique ID
}

// webhookConfig holds system webhook configuration.
type webhookConfig struct {
	URL     string `json:"url,omitempty"`     // Webhook URL
	Enabled bool   `json:"enabled"`          // Whether webhook is enabled
}

// integrationSettingsRequest is the request body for updating integration settings.
type integrationSettingsRequest struct {
	MQTT    *mqttConfig    `json:"mqtt,omitempty"`
	Webhook *webhookConfig `json:"webhook,omitempty"`
}

// integrationTestRequest is the request body for testing integrations.
type integrationTestRequest struct {
	Type    string                 `json:"type"`    // "mqtt" or "webhook"
	Options map[string]interface{} `json:"options"` // Test-specific options
}

// handleGetSettings handles GET /api/settings/integration requests.
func (h *IntegrationSettingsHandler) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	response := integrationSettingsResponse{}

	// Get MQTT settings
	mqttCfg, err := h.getMQTTConfig()
	if err == nil {
		response.MQTT = mqttCfg
		// Set connection status
		if h.mqttClient != nil {
			response.MQTT.Connected = h.mqttClient.IsConnected()
			response.MQTT.MothershipID = h.mqttClient.GetMothershipID()
		}
	}

	// Get webhook settings
	webhookCfg, err := h.getWebhookConfig()
	if err == nil {
		response.Webhook = webhookCfg
	}

	writeJSON(w, http.StatusOK, response)
}

// handleUpdateSettings handles POST /api/settings/integration requests.
func (h *IntegrationSettingsHandler) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req integrationSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Update MQTT settings
	if req.MQTT != nil {
		if err := h.updateMQTTSettings(req.MQTT); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	// Update webhook settings
	if req.Webhook != nil {
		if err := h.updateWebhookSettings(req.Webhook); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	// Return updated settings
	h.handleGetSettings(w, r)
}

// handleTest handles POST /api/settings/integration/test requests.
func (h *IntegrationSettingsHandler) handleTest(w http.ResponseWriter, r *http.Request) {
	var req integrationTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	switch req.Type {
	case "mqtt":
		h.testMQTT(w, r)
	case "webhook":
		h.testWebhook(w, r)
	default:
		writeJSONError(w, http.StatusBadRequest, "invalid type: must be 'mqtt' or 'webhook'")
	}
}

// getMQTTConfig retrieves MQTT settings from the database.
func (h *IntegrationSettingsHandler) getMQTTConfig() (*mqttConfig, error) {
	var cfg mqttConfig

	// Get settings from database
	var brokerURL, username, discoveryPrefix string
	var tlsEnabled int

	err := h.db.QueryRow(`SELECT value_json FROM settings WHERE key = 'mqtt_broker'`).Scan(&brokerURL)
	if err != nil {
		// Return default config if not found
		cfg.DiscoveryPrefix = "homeassistant"
		return &cfg, nil
	}

	err = h.db.QueryRow(`SELECT value_json FROM settings WHERE key = 'mqtt_username'`).Scan(&username)
	if err != nil {
		username = ""
	}

	err = h.db.QueryRow(`SELECT value_json FROM settings WHERE key = 'mqtt_tls'`).Scan(&tlsEnabled)
	if err != nil {
		tlsEnabled = 0
	}

	err = h.db.QueryRow(`SELECT value_json FROM settings WHERE key = 'mqtt_discovery_prefix'`).Scan(&discoveryPrefix)
	if err != nil {
		discoveryPrefix = "homeassistant"
	}

	// Parse JSON strings (remove quotes)
	if len(brokerURL) > 0 && brokerURL[0] == '"' {
		brokerURL = brokerURL[1 : len(brokerURL)-1]
	}
	if len(username) > 0 && username[0] == '"' {
		username = username[1 : len(username)-1]
	}
	if len(discoveryPrefix) > 0 && discoveryPrefix[0] == '"' {
		discoveryPrefix = discoveryPrefix[1 : len(discoveryPrefix)-1]
	}

	cfg.Broker = brokerURL
	cfg.Username = username
	cfg.TLS = tlsEnabled != 0
	cfg.DiscoveryPrefix = discoveryPrefix

	// Set connection status and mothership ID
	h.mu.RLock()
	if h.mqttClient != nil {
		cfg.Connected = h.mqttClient.IsConnected()
		cfg.MothershipID = h.mqttClient.GetMothershipID()
	} else {
		cfg.MothershipID = h.mothershipID
	}
	h.mu.RUnlock()

	return &cfg, nil
}

// updateMQTTSettings updates MQTT configuration.
func (h *IntegrationSettingsHandler) updateMQTTSettings(cfg *mqttConfig) error {
	// Validate broker URL format
	if cfg.Broker != "" {
		if len(cfg.Broker) < 6 || (cfg.Broker[:3] != "tcp" && cfg.Broker[:4] != "mqtt" && cfg.Broker[:5] != "mqtts") {
			return &validationError{Field: "broker", Reason: "invalid URL format (must start with tcp://, mqtt://, or mqtts://)"}
		}
	}

	// Save broker URL
	if cfg.Broker != "" {
		brokerJSON, _ := json.Marshal(cfg.Broker)
		_, err := h.db.Exec(`INSERT OR REPLACE INTO settings (key, value_json, updated_at) VALUES (?, ?, ?)`,
			"mqtt_broker", string(brokerJSON), "strftime('%s', 'now')")
		if err != nil {
			return err
		}
	}

	// Save username
	if cfg.Username != "" {
		usernameJSON, _ := json.Marshal(cfg.Username)
		_, err := h.db.Exec(`INSERT OR REPLACE INTO settings (key, value_json, updated_at) VALUES (?, ?, ?)`,
			"mqtt_username", string(usernameJSON), "strftime('%s', 'now')")
		if err != nil {
			return err
		}
	}

	// Save password (if provided)
	if cfg.Password != "" {
		passwordJSON, _ := json.Marshal(cfg.Password)
		_, err := h.db.Exec(`INSERT OR REPLACE INTO settings (key, value_json, updated_at) VALUES (?, ?, ?)`,
			"mqtt_password", string(passwordJSON), "strftime('%s', 'now')")
		if err != nil {
			return err
		}
	}

	// Save TLS setting
	tlsJSON, _ := json.Marshal(cfg.TLS)
	_, err := h.db.Exec(`INSERT OR REPLACE INTO settings (key, value_json, updated_at) VALUES (?, ?, ?)`,
		"mqtt_tls", string(tlsJSON), "strftime('%s', 'now')")
	if err != nil {
		return err
	}

	// Save discovery prefix
	if cfg.DiscoveryPrefix != "" {
		prefixJSON, _ := json.Marshal(cfg.DiscoveryPrefix)
		_, err := h.db.Exec(`INSERT OR REPLACE INTO settings (key, value_json, updated_at) VALUES (?, ?, ?)`,
			"mqtt_discovery_prefix", string(prefixJSON), "strftime('%s', 'now')")
		if err != nil {
			return err
		}
	}

	// Update MQTT client if available
	h.mu.RLock()
	client := h.mqttClient
	h.mu.RUnlock()

	if client != nil {
		// Import mqtt package's Config type
		mqttCfg := map[string]interface{}{
			"broker":           cfg.Broker,
			"username":         cfg.Username,
			"password":         cfg.Password,
			"tls":              cfg.TLS,
			"discovery_prefix": cfg.DiscoveryPrefix,
			"mothership_id":    h.mothershipID,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := client.UpdateConfig(ctx, mqttCfg); err != nil {
			log.Printf("[WARN] Failed to update MQTT client config: %v", err)
			return err
		}
	}

	return nil
}

// getWebhookConfig retrieves system webhook settings from the database.
func (h *IntegrationSettingsHandler) getWebhookConfig() (*webhookConfig, error) {
	var cfg webhookConfig

	// Query from settings table
	var valueJSON string
	err := h.db.QueryRow(`SELECT value_json FROM settings WHERE key = 'system_webhook'`).Scan(&valueJSON)
	if err != nil {
		// Return default config if not found
		return &webhookConfig{Enabled: false}, nil
	}

	if err := json.Unmarshal([]byte(valueJSON), &cfg); err != nil {
		return &webhookConfig{Enabled: false}, nil
	}

	return &cfg, nil
}

// updateWebhookSettings updates system webhook configuration.
func (h *IntegrationSettingsHandler) updateWebhookSettings(cfg *webhookConfig) error {
	// Validate URL if enabled
	if cfg.Enabled && cfg.URL == "" {
		return &validationError{Field: "url", Reason: "URL is required when webhook is enabled"}
	}

	// Validate URL format
	if cfg.URL != "" {
		if cfg.URL[0] != '/' && (cfg.URL[:4] != "http" && cfg.URL[:5] != "https") {
			return &validationError{Field: "url", Reason: "invalid URL format (must start with http:// or https://)"}
		}
	}

	// Save to settings table
	valueJSON, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	_, err = h.db.Exec(`INSERT OR REPLACE INTO settings (key, value_json, updated_at) VALUES (?, ?, ?)`,
		"system_webhook", string(valueJSON), "strftime('%s', 'now')")
	if err != nil {
		return err
	}

	// Update webhook publisher if available
	if h.webhookPublisher != nil {
		// Import webhook package's Config type
		webhookCfg := map[string]interface{}{
			"url":     cfg.URL,
			"enabled": cfg.Enabled,
		}
		h.webhookPublisher.UpdateConfig(webhookCfg)
	}

	return nil
}

// testMQTT tests MQTT connection by publishing discovery messages.
func (h *IntegrationSettingsHandler) testMQTT(w http.ResponseWriter, r *http.Request) {
	if h.mqttClient == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "MQTT client not configured")
		return
	}

	if !h.mqttClient.IsConnected() {
		// Try to reconnect
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		if err := h.mqttClient.Reconnect(ctx); err != nil {
			writeJSONError(w, http.StatusServiceUnavailable, "MQTT connection failed: "+err.Error())
			return
		}
	}

	// Publish discovery messages as a test
	_, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Publish discovery now
	if err := h.mqttClient.PublishDiscoveryNow(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to publish discovery: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": "MQTT connection verified. Discovery messages published.",
	})
}

// testWebhook tests webhook by sending a test event.
func (h *IntegrationSettingsHandler) testWebhook(w http.ResponseWriter, r *http.Request) {
	if h.webhookPublisher == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "Webhook publisher not configured")
		return
	}

	if err := h.webhookPublisher.TestWebhook(); err != nil {
		if ve, ok := err.(*validationError); ok {
			writeJSONError(w, http.StatusBadRequest, ve.Error())
		} else {
			writeJSONError(w, http.StatusInternalServerError, "Webhook test failed: "+err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": "Test webhook event sent successfully",
	})
}

// validationError represents a settings validation error.
type validationError struct {
	Field  string
	Reason string
}

func (e *validationError) Error() string {
	return e.Field + ": " + e.Reason
}
