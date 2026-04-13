// Package api provides REST API handlers for Spaxel notification settings.
package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

// NotificationSettingsHandler manages notification settings.
// These settings are stored in the settings table alongside other system settings.
type NotificationSettingsHandler struct {
	mu            sync.RWMutex
	db            *sql.DB
	notifyService NotificationTestSender
}

// NotificationTestSender is the interface for sending test notifications.
type NotificationTestSender interface {
	Send(title, body string, data map[string]interface{}) error
}

// NewNotificationSettingsHandler creates a new notification settings handler.
func NewNotificationSettingsHandler(db *sql.DB) *NotificationSettingsHandler {
	return &NotificationSettingsHandler{
		db: db,
	}
}

// SetNotifyService sets the notification sender for test notifications.
func (h *NotificationSettingsHandler) SetNotifyService(ns NotificationTestSender) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.notifyService = ns
}

// RegisterRoutes registers notification settings endpoints.
//
// Notification Settings Endpoints:
//
// GET /api/settings/notifications
//
//	@Summary		Get notification settings
//	@Description	Returns all notification settings including channel configuration, quiet hours, batching, and event type preferences.
//	@Tags			settings
//	@Produce		json
//	@Success		200	{object}	notificationSettingsResponse	"Notification settings"
//	@Router			/api/settings/notifications [get]
//
// PUT /api/settings/notifications
//
//	@Summary		Update notification settings
//	@Description	Updates notification settings. Only the fields provided are modified.
//	@Tags			settings
//	@Accept			json
//	@Produce		json
//	@Param			request	body	notificationSettingsRequest	true	"Settings to update (partial)"
//	@Success		200	{object}	notificationSettingsResponse	"Updated settings"
//	@Failure		400	{object}	map[string]string	"Invalid request or validation error"
//	@Failure		500	{object}	map[string]string	"Failed to update settings"
//	@Router			/api/settings/notifications [put]
//
// POST /api/notifications/test
//
//	@Summary		Send a test notification
//	@Description	Sends a test notification via the configured channel.
//	@Tags			notifications
//	@Accept			json
//	@Produce		json
//	@Param			request	body	testNotificationRequest	true	"Test notification parameters"
//	@Success		200	{object}	map[string]interface{}	"Test result"
//	@Failure		400	{object}	map[string]string	"Invalid request or no enabled channel"
//	@Failure		500	{object}	map[string]string	"Failed to send notification"
//	@Router			/api/notifications/test [post]
func (h *NotificationSettingsHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/settings/notifications", h.handleGetSettings)
	r.Put("/api/settings/notifications", h.handleUpdateSettings)
	r.Post("/api/notifications/test", h.handleSendTest)
}

// notificationSettingsResponse is the response for notification settings requests.
type notificationSettingsResponse struct {
	// Channel configuration
	ChannelType   string                 `json:"channel_type"`   // "none", "ntfy", "pushover", "webhook"
	ChannelConfig map[string]interface{} `json:"channel_config"` // Channel-specific credentials

	// Quiet hours
	QuietHoursEnabled bool   `json:"quiet_hours_enabled"`
	QuietHoursStart   string `json:"quiet_hours_start"`   // HH:MM format
	QuietHoursEnd     string `json:"quiet_hours_end"`     // HH:MM format
	QuietHoursDays    int    `json:"quiet_hours_days"`    // Bitmask: 0x7F = all days (Sun=1, Mon=2, ..., Sat=64)

	// Morning digest
	MorningDigestEnabled bool   `json:"morning_digest_enabled"`
	MorningDigestTime    string `json:"morning_digest_time"` // HH:MM format

	// Smart batching
	SmartBatchingEnabled bool `json:"smart_batching_enabled"` // Default: true
	SmartBatchingWindow  int  `json:"smart_batching_window"`  // Seconds, default: 30

	// Event type preferences
	EventTypes map[string]bool `json:"event_types"` // Event type -> enabled
}

// notificationSettingsRequest is the request body for updating notification settings.
// All fields are optional; only provided fields are updated.
type notificationSettingsRequest struct {
	// Channel configuration
	ChannelType   *string                 `json:"channel_type,omitempty"`
	ChannelConfig *map[string]interface{} `json:"channel_config,omitempty"`

	// Quiet hours
	QuietHoursEnabled *bool   `json:"quiet_hours_enabled,omitempty"`
	QuietHoursStart   *string `json:"quiet_hours_start,omitempty"`
	QuietHoursEnd     *string `json:"quiet_hours_end,omitempty"`
	QuietHoursDays    *int    `json:"quiet_hours_days,omitempty"`

	// Morning digest
	MorningDigestEnabled *bool   `json:"morning_digest_enabled,omitempty"`
	MorningDigestTime    *string `json:"morning_digest_time,omitempty"`

	// Smart batching
	SmartBatchingEnabled *bool `json:"smart_batching_enabled,omitempty"`
	SmartBatchingWindow  *int  `json:"smart_batching_window,omitempty"`

	// Event type preferences
	EventTypes *map[string]bool `json:"event_types,omitempty"`
}

// handleGetSettings handles GET /api/settings/notifications requests.
func (h *NotificationSettingsHandler) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.getSettings()
	if err != nil {
		log.Printf("[ERROR] Failed to get notification settings: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to get notification settings")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

// handleUpdateSettings handles PUT /api/settings/notifications requests.
func (h *NotificationSettingsHandler) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req notificationSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	// Validate and update settings
	if err := h.updateSettings(&req); err != nil {
		if ve, ok := err.(*NotificationValidationError); ok {
			writeJSONError(w, http.StatusBadRequest, ve.Error())
		} else {
			writeJSONError(w, http.StatusInternalServerError, "failed to update settings: "+err.Error())
		}
		return
	}

	// Return updated settings
	settings, err := h.getSettings()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to get updated settings")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

// handleSendTest handles POST /api/notifications/test requests.
func (h *NotificationSettingsHandler) handleSendTest(w http.ResponseWriter, r *http.Request) {
	var req testNotificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	// Get current settings to determine channel type
	if req.ChannelType == "" {
		settings, err := h.getSettings()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to get current settings")
			return
		}
		req.ChannelType = settings.ChannelType
	}

	// Validate channel type
	if req.ChannelType == "" || req.ChannelType == "none" {
		writeJSONError(w, http.StatusBadRequest, "no notification channel configured")
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

	// Send test notification
	h.mu.RLock()
	sender := h.notifyService
	h.mu.RUnlock()

	if sender == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":  "simulated",
			"message": "Test notification simulated (no sender attached)",
		})
		return
	}

	if err := sender.Send(req.Title, req.Body, req.Data); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to send notification: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "sent",
		"message": "Test notification sent successfully",
	})
}

// getSettings retrieves notification settings from the database.
func (h *NotificationSettingsHandler) getSettings() (*notificationSettingsResponse, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	response := &notificationSettingsResponse{
		// Set defaults
		ChannelType:            "none",
		ChannelConfig:          make(map[string]interface{}),
		QuietHoursDays:         0x7F, // All days
		MorningDigestTime:      "07:00",
		SmartBatchingEnabled:   true,
		SmartBatchingWindow:    30,
		EventTypes:             getDefaultEventTypes(),
	}

	// Get channel type
	var channelTypeJSON string
	err := h.db.QueryRow(`SELECT value_json FROM settings WHERE key = 'notification_channel_type'`).Scan(&channelTypeJSON)
	if err == nil {
		json.Unmarshal([]byte(channelTypeJSON), &response.ChannelType) //nolint:errcheck
	}

	// Get channel config
	var configJSON string
	err = h.db.QueryRow(`SELECT value_json FROM settings WHERE key = 'notification_channel_config'`).Scan(&configJSON)
	if err == nil {
		var config map[string]interface{}
		if err := json.Unmarshal([]byte(configJSON), &config); err == nil {
			response.ChannelConfig = config
		}
	}

	// Get quiet hours enabled
	var quietEnabled int
	err = h.db.QueryRow(`SELECT value_json FROM settings WHERE key = 'notification_quiet_hours_enabled'`).Scan(&quietEnabled)
	if err == nil {
		response.QuietHoursEnabled = quietEnabled != 0
	}

	// Get quiet hours start
	var quietStartJSON string
	err = h.db.QueryRow(`SELECT value_json FROM settings WHERE key = 'notification_quiet_hours_start'`).Scan(&quietStartJSON)
	if err == nil {
		json.Unmarshal([]byte(quietStartJSON), &response.QuietHoursStart) //nolint:errcheck
	} else {
		response.QuietHoursStart = "22:00"
	}

	// Get quiet hours end
	var quietEndJSON string
	err = h.db.QueryRow(`SELECT value_json FROM settings WHERE key = 'notification_quiet_hours_end'`).Scan(&quietEndJSON)
	if err == nil {
		json.Unmarshal([]byte(quietEndJSON), &response.QuietHoursEnd) //nolint:errcheck
	} else {
		response.QuietHoursEnd = "07:00"
	}

	// Get quiet hours days mask
	var quietDays int
	err = h.db.QueryRow(`SELECT value_json FROM settings WHERE key = 'notification_quiet_hours_days'`).Scan(&quietDays)
	if err == nil {
		response.QuietHoursDays = quietDays
	}

	// Get morning digest enabled
	var morningDigest int
	err = h.db.QueryRow(`SELECT value_json FROM settings WHERE key = 'notification_morning_digest_enabled'`).Scan(&morningDigest)
	if err == nil {
		response.MorningDigestEnabled = morningDigest != 0
	} else {
		response.MorningDigestEnabled = true // Default on
	}

	// Get morning digest time
	var digestTimeJSON string
	err = h.db.QueryRow(`SELECT value_json FROM settings WHERE key = 'notification_morning_digest_time'`).Scan(&digestTimeJSON)
	if err == nil {
		json.Unmarshal([]byte(digestTimeJSON), &response.MorningDigestTime) //nolint:errcheck
	}

	// Get smart batching enabled
	var smartBatching int
	err = h.db.QueryRow(`SELECT value_json FROM settings WHERE key = 'notification_smart_batching_enabled'`).Scan(&smartBatching)
	if err == nil {
		response.SmartBatchingEnabled = smartBatching != 0
	}

	// Get smart batching window
	var batchWindow int
	err = h.db.QueryRow(`SELECT value_json FROM settings WHERE key = 'notification_smart_batching_window'`).Scan(&batchWindow)
	if err == nil {
		response.SmartBatchingWindow = batchWindow
	}

	// Get event type preferences
	var eventTypesJSON string
	err = h.db.QueryRow(`SELECT value_json FROM settings WHERE key = 'notification_event_types'`).Scan(&eventTypesJSON)
	if err == nil {
		var eventTypes map[string]bool
		if err := json.Unmarshal([]byte(eventTypesJSON), &eventTypes); err == nil {
			response.EventTypes = eventTypes
		}
	}

	return response, nil
}

// updateSettings updates notification settings in the database.
func (h *NotificationSettingsHandler) updateSettings(req *notificationSettingsRequest) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Helper function to save a setting
	saveSetting := func(key string, value interface{}) error {
		valueJSON, err := json.Marshal(value)
		if err != nil {
			return err
		}
		_, err = h.db.Exec(`
			INSERT INTO settings (key, value_json, updated_at) VALUES (?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET value_json = ?, updated_at = ?
		`, key, string(valueJSON), time.Now().UnixMilli(),
			string(valueJSON), time.Now().UnixMilli())
		return err
	}

	// Update channel type
	if req.ChannelType != nil {
		if err := validateChannelType(*req.ChannelType); err != nil {
			return err
		}
		if err := saveSetting("notification_channel_type", *req.ChannelType); err != nil {
			return err
		}
	}

	// Update channel config
	if req.ChannelConfig != nil {
		if err := validateChannelConfig(*req.ChannelType, *req.ChannelConfig); err != nil {
			return err
		}
		if err := saveSetting("notification_channel_config", *req.ChannelConfig); err != nil {
			return err
		}
	}

	// Update quiet hours
	if req.QuietHoursEnabled != nil {
		val := 0
		if *req.QuietHoursEnabled {
			val = 1
		}
		if err := saveSetting("notification_quiet_hours_enabled", val); err != nil {
			return err
		}
	}

	if req.QuietHoursStart != nil {
		if err := validateTimeFormat(*req.QuietHoursStart); err != nil {
			return &NotificationValidationError{Field: "quiet_hours_start", Reason: err.Error()}
		}
		if err := saveSetting("notification_quiet_hours_start", *req.QuietHoursStart); err != nil {
			return err
		}
	}

	if req.QuietHoursEnd != nil {
		if err := validateTimeFormat(*req.QuietHoursEnd); err != nil {
			return &NotificationValidationError{Field: "quiet_hours_end", Reason: err.Error()}
		}
		if err := saveSetting("notification_quiet_hours_end", *req.QuietHoursEnd); err != nil {
			return err
		}
	}

	if req.QuietHoursDays != nil {
		if *req.QuietHoursDays < 0 || *req.QuietHoursDays > 0x7F {
			return &NotificationValidationError{Field: "quiet_hours_days", Reason: "must be between 0 and 127"}
		}
		if err := saveSetting("notification_quiet_hours_days", *req.QuietHoursDays); err != nil {
			return err
		}
	}

	// Update morning digest
	if req.MorningDigestEnabled != nil {
		val := 0
		if *req.MorningDigestEnabled {
			val = 1
		}
		if err := saveSetting("notification_morning_digest_enabled", val); err != nil {
			return err
		}
	}

	if req.MorningDigestTime != nil {
		if err := validateTimeFormat(*req.MorningDigestTime); err != nil {
			return &NotificationValidationError{Field: "morning_digest_time", Reason: err.Error()}
		}
		if err := saveSetting("notification_morning_digest_time", *req.MorningDigestTime); err != nil {
			return err
		}
	}

	// Update smart batching
	if req.SmartBatchingEnabled != nil {
		val := 0
		if *req.SmartBatchingEnabled {
			val = 1
		}
		if err := saveSetting("notification_smart_batching_enabled", val); err != nil {
			return err
		}
	}

	if req.SmartBatchingWindow != nil {
		if *req.SmartBatchingWindow < 5 || *req.SmartBatchingWindow > 300 {
			return &NotificationValidationError{Field: "smart_batching_window", Reason: "must be between 5 and 300 seconds"}
		}
		if err := saveSetting("notification_smart_batching_window", *req.SmartBatchingWindow); err != nil {
			return err
		}
	}

	// Update event types
	if req.EventTypes != nil {
		if err := validateEventTypes(*req.EventTypes); err != nil {
			return err
		}
		if err := saveSetting("notification_event_types", *req.EventTypes); err != nil {
			return err
		}
	}

	return nil
}

// validateChannelType validates the channel type.
func validateChannelType(channelType string) error {
	switch channelType {
	case "none", "ntfy", "pushover", "webhook":
		return nil
	default:
		return &NotificationValidationError{Field: "channel_type", Reason: "must be one of: none, ntfy, pushover, webhook"}
	}
}

// validateTimeFormat validates a time string in HH:MM format.
func validateTimeFormat(timeStr string) error {
	if len(timeStr) != 5 {
		return &NotificationValidationError{Reason: "invalid time format (must be HH:MM)"}
	}
	if timeStr[2] != ':' {
		return &NotificationValidationError{Reason: "invalid time format (must be HH:MM)"}
	}
	hour := timeStr[0:2]
	minute := timeStr[3:5]
	for _, c := range hour {
		if c < '0' || c > '9' {
			return &NotificationValidationError{Reason: "invalid hour (must be 00-23)"}
		}
	}
	for _, c := range minute {
		if c < '0' || c > '9' {
			return &NotificationValidationError{Reason: "invalid minute (must be 00-59)"}
		}
	}
	// Validate numeric ranges
	hourVal := int(hour[0]-'0')*10 + int(hour[1]-'0')
	minuteVal := int(minute[0]-'0')*10 + int(minute[1]-'0')
	if hourVal > 23 {
		return &NotificationValidationError{Reason: "invalid hour (must be 00-23)"}
	}
	if minuteVal > 59 {
		return &NotificationValidationError{Reason: "invalid minute (must be 00-59)"}
	}
	return nil
}

// validateEventTypes validates event type preferences.
func validateEventTypes(eventTypes map[string]bool) error {
	validTypes := getDefaultEventTypes()
	for eventType := range eventTypes {
		if _, ok := validTypes[eventType]; !ok {
			return &NotificationValidationError{Field: "event_types", Reason: "invalid event type: " + eventType}
		}
	}
	return nil
}

// getDefaultEventTypes returns the default event type preferences (all enabled).
func getDefaultEventTypes() map[string]bool {
	return map[string]bool{
		"zone_enter":      true,
		"zone_leave":      true,
		"zone_vacant":     true,
		"fall_detected":   true,
		"fall_escalation": true,
		"anomaly_alert":   true,
		"node_offline":    true,
		"sleep_summary":   true,
	}
}

// NotificationValidationError represents a validation error for a specific setting.
type NotificationValidationError struct {
	Field  string
	Reason string
}

func (e *NotificationValidationError) Error() string {
	if e.Field != "" {
		return e.Field + ": " + e.Reason
	}
	return e.Reason
}

