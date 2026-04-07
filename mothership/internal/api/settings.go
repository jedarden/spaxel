// Package api provides REST API handlers for Spaxel settings.
package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi"
	_ "modernc.org/sqlite"
)

// SettingsHandler manages application settings.
// Settings are stored as key-value pairs in the settings table with JSON-encoded values.
type SettingsHandler struct {
	mu   sync.RWMutex
	db   *sql.DB
	// cache is an in-memory cache of settings for fast reads
	cache map[string]interface{}
}

// NewSettingsHandler creates a new settings handler using the provided database connection.
// The database connection must be to the main spaxel.db which contains the settings table.
func NewSettingsHandler(db *sql.DB) *SettingsHandler {
	s := &SettingsHandler{
		db:    db,
		cache: make(map[string]interface{}),
	}
	// Load initial settings into cache
	if err := s.load(); err != nil {
		log.Printf("[WARN] Failed to load settings: %v", err)
	}
	return s
}

// NewSettingsHandlerWithPath creates a new settings handler by opening a database
// at the specified path. This is a convenience function for handlers that manage
// their own database connections.
func NewSettingsHandlerWithPath(dbPath string) (*SettingsHandler, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	// Verify the settings table exists
	var tableName string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='settings'").Scan(&tableName)
	if err != nil {
		db.Close()
		return nil, err
	}

	s := &SettingsHandler{
		db:    db,
		cache: make(map[string]interface{}),
	}
	// Load initial settings into cache
	if err := s.load(); err != nil {
		log.Printf("[WARN] Failed to load settings: %v", err)
	}

	return s, nil
}

// Close closes the database connection.
func (s *SettingsHandler) Close() error {
	return s.db.Close()
}

// load reads all settings from the database into the in-memory cache.
// It must be called with the mutex lock held.
func (s *SettingsHandler) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`SELECT key, value_json FROM settings`)
	if err != nil {
		return err
	}
	defer rows.Close()

	// Clear existing cache
	s.cache = make(map[string]interface{})

	for rows.Next() {
		var key, valueJSON string
		if err := rows.Scan(&key, &valueJSON); err != nil {
			log.Printf("[WARN] Failed to scan setting key=%s: %v", key, err)
			continue
		}

		var value interface{}
		if err := json.Unmarshal([]byte(valueJSON), &value); err != nil {
			// If not valid JSON, store as string
			log.Printf("[WARN] Failed to unmarshal setting key=%s: %v", key, err)
			s.cache[key] = valueJSON
		} else {
			s.cache[key] = value
		}
	}

	return rows.Err()
}

// Get returns all settings as a map.
// Default values are included for keys that don't exist in the database.
func (s *SettingsHandler) Get() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]interface{}, len(s.cache)+len(defaultSettings))

	// First add cached values
	for k, v := range s.cache {
		result[k] = v
	}

	// Then add defaults for any missing keys
	for k, v := range defaultSettings {
		if _, exists := s.cache[k]; !exists {
			result[k] = v
		}
	}

	return result
}

// GetSingle returns a single setting value by key.
// Returns the value, true if found, or nil, false if not found.
func (s *SettingsHandler) GetSingle(key string) (interface{}, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if val, exists := s.cache[key]; exists {
		return val, true
	}

	// Check defaults
	if val, exists := defaultSettings[key]; exists {
		return val, true
	}

	return nil, false
}

// Set updates a single setting value.
// The value is marshaled to JSON and stored in the database.
func (s *SettingsHandler) Set(key string, value interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.setLocked(key, value)
}

// setLocked updates a setting without acquiring the mutex.
// The caller must hold s.mu.
func (s *SettingsHandler) setLocked(key string, value interface{}) error {
	valueJSON, err := json.Marshal(value)
	if err != nil {
		return err
	}

	now := time.Now().UnixMilli()
	_, err = s.db.Exec(`
		INSERT INTO settings (key, value_json, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value_json = ?, updated_at = ?
	`, key, string(valueJSON), now, string(valueJSON), now)
	if err != nil {
		return err
	}

	s.cache[key] = value
	return nil
}

// Update merges a partial settings map with existing settings.
// Only the keys provided in the updates map are modified.
func (s *SettingsHandler) Update(updates map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, value := range updates {
		if err := s.setLocked(key, value); err != nil {
			return err
		}
	}
	return nil
}

// Delete removes a setting from both the database and cache.
func (s *SettingsHandler) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`DELETE FROM settings WHERE key = ?`, key)
	if err != nil {
		return err
	}

	delete(s.cache, key)
	return nil
}

// defaultSettings defines the default values for all known settings.
// These are returned when a key hasn't been set in the database.
var defaultSettings = map[string]interface{}{
	"fusion_rate_hz":        10.0,    // Fusion loop rate in Hz
	"grid_cell_m":           0.2,     // Fresnel grid cell size in meters
	"delta_rms_threshold":   0.02,    // Motion detection threshold
	"tau_s":                 30.0,    // EMA baseline time constant in seconds
	"fresnel_decay":         2.0,     // Fresnel zone weight decay rate
	"n_subcarriers":         16,      // Number of subcarriers for NBVI selection
	"breathing_sensitivity": 0.005,   // Breathing detection threshold (radians RMS)
	"motion_threshold":      0.05,    // Smooth deltaRMS threshold for motion gating
	"dwell_seconds":         30,      // Default dwell trigger duration in seconds
	"vacant_seconds":        300,     // Default vacant trigger duration in seconds
	"max_tracked_blobs":     20,      // Maximum number of blobs to track simultaneously
	"replay_retention_hours": 48,      // CSI replay buffer retention in hours
	"replay_max_mb":         360,     // CSI replay buffer max size in MB
	"security_mode":         false,   // Security mode enabled state
	"security_mode_armed_at": nil,    // Timestamp when security mode was armed
	"events_archive_days":   90,      // Events archive retention in days
	"quiet_hours_start":     "",      // Quiet hours start time (HH:MM format)
	"quiet_hours_end":       "",      // Quiet hours end time (HH:MM format)
}

// RegisterRoutes registers settings endpoints on the given router.
//
// Settings Endpoints:
//
//	GET  /api/settings — Return all configurable settings as JSON
//
//	@Summary		Get all settings
//	@Description	Returns all system settings as a JSON object. Default values are included
//	@Description	for any settings that haven't been explicitly set.
//	@Tags			settings
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}	"Settings object"
//	@Router			/api/settings [get]
//
//	POST /api/settings — Update settings (partial update, merge semantics)
//
//	@Summary		Update settings
//	@Description	Updates one or more settings. Only the keys provided in the request body
//	@Description	are modified; other settings remain unchanged.
//	@Tags			settings
//	@Accept			json
//	@Produce		json
//	@Param			request	body	map[string]interface{}	true	"Settings to update (partial)"
//	@Success		200	{object}	map[string]interface{}	"Updated settings object"
//	@Failure		400	{object}	map[string]string	"Invalid request body or validation error"
//	@Failure		500	{object}	map[string]string	"Failed to update settings"
//	@Router			/api/settings [post]
//
//	PATCH /api/settings — Update settings (alias for POST)
func (s *SettingsHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/settings", s.handleGetSettings)
	r.Post("/api/settings", s.handleUpdateSettings)
	r.Patch("/api/settings", s.handleUpdateSettings)
}

// handleGetSettings handles GET /api/settings requests.
func (s *SettingsHandler) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	settings := s.Get()
	writeJSON(w, http.StatusOK, settings)
}

// handleUpdateSettings handles POST/PATCH /api/settings requests.
// It validates known settings and applies partial updates with merge semantics.
func (s *SettingsHandler) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	// Validate known settings
	if err := validateSettings(updates); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if err := s.Update(updates); err != nil {
		log.Printf("[ERROR] Failed to update settings: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update settings"})
		return
	}

	// Return updated settings
	s.handleGetSettings(w, r)
}

// validateSettings validates the provided settings values.
// Returns an error if any setting value is outside its valid range.
func validateSettings(settings map[string]interface{}) error {
	// Validate fusion_rate_hz: 1-20 Hz
	if v, ok := settings["fusion_rate_hz"]; ok {
		if f, ok := asFloat64(v); !ok || f < 1 || f > 20 {
			return &ValidationError{Key: "fusion_rate_hz", Reason: "must be between 1 and 20"}
		}
	}

	// Validate grid_cell_m: 0.05-1.0 meters
	if v, ok := settings["grid_cell_m"]; ok {
		if f, ok := asFloat64(v); !ok || f < 0.05 || f > 1.0 {
			return &ValidationError{Key: "grid_cell_m", Reason: "must be between 0.05 and 1.0"}
		}
	}

	// Validate delta_rms_threshold: 0.001-1.0
	if v, ok := settings["delta_rms_threshold"]; ok {
		if f, ok := asFloat64(v); !ok || f < 0.001 || f > 1.0 {
			return &ValidationError{Key: "delta_rms_threshold", Reason: "must be between 0.001 and 1.0"}
		}
	}

	// Validate tau_s: 1-600 seconds
	if v, ok := settings["tau_s"]; ok {
		if f, ok := asFloat64(v); !ok || f < 1 || f > 600 {
			return &ValidationError{Key: "tau_s", Reason: "must be between 1 and 600"}
		}
	}

	// Validate fresnel_decay: 1.0-4.0
	if v, ok := settings["fresnel_decay"]; ok {
		if f, ok := asFloat64(v); !ok || f < 1.0 || f > 4.0 {
			return &ValidationError{Key: "fresnel_decay", Reason: "must be between 1.0 and 4.0"}
		}
	}

	// Validate n_subcarriers: 8-47
	if v, ok := settings["n_subcarriers"]; ok {
		if f, ok := asFloat64(v); !ok || f < 8 || f > 47 {
			return &ValidationError{Key: "n_subcarriers", Reason: "must be between 8 and 47"}
		}
	}

	// Validate breathing_sensitivity: 0.001-0.1
	if v, ok := settings["breathing_sensitivity"]; ok {
		if f, ok := asFloat64(v); !ok || f < 0.001 || f > 0.1 {
			return &ValidationError{Key: "breathing_sensitivity", Reason: "must be between 0.001 and 0.1"}
		}
	}

	// Validate motion_threshold: 0.01-0.5
	if v, ok := settings["motion_threshold"]; ok {
		if f, ok := asFloat64(v); !ok || f < 0.01 || f > 0.5 {
			return &ValidationError{Key: "motion_threshold", Reason: "must be between 0.01 and 0.5"}
		}
	}

	// Validate dwell_seconds: 1-3600
	if v, ok := settings["dwell_seconds"]; ok {
		if f, ok := asFloat64(v); !ok || f < 1 || f > 3600 {
			return &ValidationError{Key: "dwell_seconds", Reason: "must be between 1 and 3600"}
		}
	}

	// Validate vacant_seconds: 10-7200
	if v, ok := settings["vacant_seconds"]; ok {
		if f, ok := asFloat64(v); !ok || f < 10 || f > 7200 {
			return &ValidationError{Key: "vacant_seconds", Reason: "must be between 10 and 7200"}
		}
	}

	// Validate max_tracked_blobs: 1-100
	if v, ok := settings["max_tracked_blobs"]; ok {
		if f, ok := asFloat64(v); !ok || f < 1 || f > 100 {
			return &ValidationError{Key: "max_tracked_blobs", Reason: "must be between 1 and 100"}
		}
	}

	// Validate replay_retention_hours: 1-168 (1 hour to 1 week)
	if v, ok := settings["replay_retention_hours"]; ok {
		if f, ok := asFloat64(v); !ok || f < 1 || f > 168 {
			return &ValidationError{Key: "replay_retention_hours", Reason: "must be between 1 and 168"}
		}
	}

	// Validate replay_max_mb: 10-10000
	if v, ok := settings["replay_max_mb"]; ok {
		if f, ok := asFloat64(v); !ok || f < 10 || f > 10000 {
			return &ValidationError{Key: "replay_max_mb", Reason: "must be between 10 and 10000"}
		}
	}

	// Validate security_mode: boolean
	if v, ok := settings["security_mode"]; ok {
		if _, ok := v.(bool); !ok {
			return &ValidationError{Key: "security_mode", Reason: "must be a boolean"}
		}
	}

	// Validate events_archive_days: 1-365
	if v, ok := settings["events_archive_days"]; ok {
		if f, ok := asFloat64(v); !ok || f < 1 || f > 365 {
			return &ValidationError{Key: "events_archive_days", Reason: "must be between 1 and 365"}
		}
	}

	return nil
}

// asFloat64 attempts to convert a value to float64.
// JSON numbers are unmarshaled as float64, but integers may be unmarshaled
// differently depending on the JSON decoder.
func asFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case int32:
		return float64(val), true
	default:
		return 0, false
	}
}

// ValidationError represents a validation error for a specific setting.
type ValidationError struct {
	Key    string
	Reason string
}

func (e *ValidationError) Error() string {
	return e.Key + ": " + e.Reason
}
