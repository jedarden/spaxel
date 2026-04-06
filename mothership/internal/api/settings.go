// Package api provides REST API handlers for Spaxel settings.
package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-chi/chi"
	_ "modernc.org/sqlite"
)

// SettingsHandler manages application settings.
type SettingsHandler struct {
	mu   sync.RWMutex
	db   *sql.DB
	data map[string]interface{}
}

// NewSettingsHandler creates a new settings handler.
func NewSettingsHandler(dbPath string) (*SettingsHandler, error) {
	if err := os.MkdirAll(dbPath[:len(dbPath)-len("/settings.db")], 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	s := &SettingsHandler{
		db:   db,
		data: make(map[string]interface{}),
	}

	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	if err := s.load(); err != nil {
		log.Printf("[WARN] Failed to load settings: %v", err)
	}

	return s, nil
}

func (s *SettingsHandler) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS settings (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at INTEGER NOT NULL DEFAULT 0
		);
	`)
	return err
}

func (s *SettingsHandler) load() error {
	rows, err := s.db.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return err
	}
	defer rows.Close()

	s.mu.Lock()
	defer s.mu.Unlock()

	for rows.Next() {
		var key, valueStr string
		if err := rows.Scan(&key, &valueStr); err != nil {
			continue
		}

		var value interface{}
		if err := json.Unmarshal([]byte(valueStr), &value); err != nil {
			// If not valid JSON, store as string
			value = valueStr
		}
		s.data[key] = value
	}

	return nil
}

// Close closes the database connection.
func (s *SettingsHandler) Close() error {
	return s.db.Close()
}

// Get returns all settings as a map.
func (s *SettingsHandler) Get() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]interface{}, len(s.data))
	for k, v := range s.data {
		result[k] = v
	}
	return result
}

// Set updates a single setting value.
func (s *SettingsHandler) Set(key string, value interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.setLocked(key, value)
}

func (s *SettingsHandler) setLocked(key string, value interface{}) error {
	valueJSON, err := json.Marshal(value)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`
		INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = ?, updated_at = ?
	`, key, string(valueJSON), nowMS(), string(valueJSON), nowMS())
	if err != nil {
		return err
	}

	s.data[key] = value
	return nil
}

// Update merges a partial settings map.
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

// Delete removes a setting.
func (s *SettingsHandler) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`DELETE FROM settings WHERE key = ?`, key)
	if err != nil {
		return err
	}

	delete(s.data, key)
	return nil
}

// RegisterRoutes registers settings endpoints.
// GET  /api/settings — return all configurable settings as JSON
// POST /api/settings — update settings (partial update, merge semantics)
func (s *SettingsHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/settings", s.handleGetSettings)
	r.Post("/api/settings", s.handleUpdateSettings)
	r.Patch("/api/settings", s.handleUpdateSettings)
}

func (s *SettingsHandler) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	settings := s.Get()

	// Add default values for keys that don't exist yet
	if _, ok := settings["fusion_rate_hz"]; !ok {
		settings["fusion_rate_hz"] = 10
	}
	if _, ok := settings["grid_cell_m"]; !ok {
		settings["grid_cell_m"] = 0.2
	}
	if _, ok := settings["delta_rms_threshold"]; !ok {
		settings["delta_rms_threshold"] = 0.02
	}
	if _, ok := settings["tau_s"]; !ok {
		settings["tau_s"] = 30.0
	}
	if _, ok := settings["fresnel_decay"]; !ok {
		settings["fresnel_decay"] = 2.0
	}
	if _, ok := settings["n_subcarriers"]; !ok {
		settings["n_subcarriers"] = 16
	}
	if _, ok := settings["breathing_sensitivity"]; !ok {
		settings["breathing_sensitivity"] = 0.005
	}
	if _, ok := settings["motion_threshold"]; !ok {
		settings["motion_threshold"] = 0.05
	}
	if _, ok := settings["dwell_seconds"]; !ok {
		settings["dwell_seconds"] = 30
	}
	if _, ok := settings["vacant_seconds"]; !ok {
		settings["vacant_seconds"] = 300
	}
	if _, ok := settings["max_tracked_blobs"]; !ok {
		settings["max_tracked_blobs"] = 20
	}
	if _, ok := settings["replay_retention_hours"]; !ok {
		settings["replay_retention_hours"] = 48
	}
	if _, ok := settings["replay_max_mb"]; !ok {
		settings["replay_max_mb"] = 360
	}

	writeJSON(w, settings)
}

func (s *SettingsHandler) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Validate some known settings
	if v, ok := updates["fusion_rate_hz"]; ok {
		if f, ok := v.(float64); !ok || f < 1 || f > 20 {
			http.Error(w, "fusion_rate_hz must be between 1 and 20", http.StatusBadRequest)
			return
		}
	}
	if v, ok := updates["grid_cell_m"]; ok {
		if f, ok := v.(float64); !ok || f < 0.05 || f > 1.0 {
			http.Error(w, "grid_cell_m must be between 0.05 and 1.0", http.StatusBadRequest)
			return
		}
	}
	if v, ok := updates["delta_rms_threshold"]; ok {
		if f, ok := v.(float64); !ok || f < 0.001 || f > 1.0 {
			http.Error(w, "delta_rms_threshold must be between 0.001 and 1.0", http.StatusBadRequest)
			return
		}
	}
	if v, ok := updates["tau_s"]; ok {
		if f, ok := v.(float64); !ok || f < 1 || f > 600 {
			http.Error(w, "tau_s must be between 1 and 600", http.StatusBadRequest)
			return
		}
	}
	if v, ok := updates["fresnel_decay"]; ok {
		if f, ok := v.(float64); !ok || f < 1.0 || f > 4.0 {
			http.Error(w, "fresnel_decay must be between 1.0 and 4.0", http.StatusBadRequest)
			return
		}
	}
	if v, ok := updates["n_subcarriers"]; ok {
		if f, ok := v.(float64); !ok || f < 8 || f > 47 {
			http.Error(w, "n_subcarriers must be between 8 and 47", http.StatusBadRequest)
			return
		}
	}
	if v, ok := updates["max_tracked_blobs"]; ok {
		if f, ok := v.(float64); !ok || f < 1 || f > 100 {
			http.Error(w, "max_tracked_blobs must be between 1 and 100", http.StatusBadRequest)
			return
		}
	}

	if err := s.Update(updates); err != nil {
		http.Error(w, "failed to update settings", http.StatusInternalServerError)
		return
	}

	// Return updated settings
	s.handleGetSettings(w, r)
}

func nowMS() int64 {
	return time.Now().UnixNano()
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}
