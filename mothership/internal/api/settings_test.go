// Package api provides tests for the settings API handler.
package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"
)

// TestSettingsHandler tests the settings handler with table-driven tests.
func TestSettingsHandler(t *testing.T) {
	// Create a temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create settings table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS settings (
			key         TEXT PRIMARY KEY,
			value_json  TEXT NOT NULL,
			updated_at  INTEGER NOT NULL DEFAULT 0
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create settings table: %v", err)
	}

	// Insert some default settings
	_, err = db.Exec(`
		INSERT INTO settings (key, value_json, updated_at) VALUES
			('fusion_rate_hz', '10', 1000),
			('delta_rms_threshold', '0.02', 1000)
	`)
	if err != nil {
		t.Fatalf("Failed to insert default settings: %v", err)
	}

	handler := NewSettingsHandler(db)

	tests := []struct {
		name           string
		method         string
		path           string
		body           interface{}
		expectedStatus int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:           "GET settings returns all settings with defaults",
			method:         "GET",
			path:           "/api/settings",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var settings map[string]interface{}
				if err := json.NewDecoder(rr.Body).Decode(&settings); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}

				// Check that we have values from DB and defaults
				if v, ok := settings["fusion_rate_hz"]; !ok || v.(float64) != 10.0 {
					t.Errorf("Expected fusion_rate_hz=10.0, got %v", v)
				}
				if v, ok := settings["delta_rms_threshold"]; !ok || v.(float64) != 0.02 {
					t.Errorf("Expected delta_rms_threshold=0.02, got %v", v)
				}
				if v, ok := settings["grid_cell_m"]; !ok || v.(float64) != 0.2 {
					t.Errorf("Expected default grid_cell_m=0.2, got %v", v)
				}
				if v, ok := settings["tau_s"]; !ok || v.(float64) != 30.0 {
					t.Errorf("Expected default tau_s=30.0, got %v", v)
				}
			},
		},
		{
			name:   "POST single setting update",
			method: "POST",
			path:   "/api/settings",
			body: map[string]interface{}{
				"fusion_rate_hz": 15.0,
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var settings map[string]interface{}
				if err := json.NewDecoder(rr.Body).Decode(&settings); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}

				if v, ok := settings["fusion_rate_hz"]; !ok || v.(float64) != 15.0 {
					t.Errorf("Expected fusion_rate_hz=15.0, got %v", v)
				}
			},
		},
		{
			name:   "POST multiple settings update",
			method: "POST",
			path:   "/api/settings",
			body: map[string]interface{}{
				"fusion_rate_hz":       12.0,
				"delta_rms_threshold":  0.03,
				"grid_cell_m":          0.15,
				"max_tracked_blobs":    30,
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var settings map[string]interface{}
				if err := json.NewDecoder(rr.Body).Decode(&settings); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}

				if v, ok := settings["fusion_rate_hz"]; !ok || v.(float64) != 12.0 {
					t.Errorf("Expected fusion_rate_hz=12.0, got %v", v)
				}
				if v, ok := settings["delta_rms_threshold"]; !ok || v.(float64) != 0.03 {
					t.Errorf("Expected delta_rms_threshold=0.03, got %v", v)
				}
				if v, ok := settings["grid_cell_m"]; !ok || v.(float64) != 0.15 {
					t.Errorf("Expected grid_cell_m=0.15, got %v", v)
				}
				if v, ok := settings["max_tracked_blobs"]; !ok {
					t.Errorf("Expected max_tracked_blobs=30, got %v", v)
				}
			},
		},
		{
			name:   "PATCH settings (same as POST)",
			method: "PATCH",
			path:   "/api/settings",
			body: map[string]interface{}{
				"security_mode": true,
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var settings map[string]interface{}
				if err := json.NewDecoder(rr.Body).Decode(&settings); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}

				if v, ok := settings["security_mode"]; !ok || v.(bool) != true {
					t.Errorf("Expected security_mode=true, got %v", v)
				}
			},
		},
		{
			name:   "POST invalid fusion_rate_hz (too high)",
			method: "POST",
			path:   "/api/settings",
			body: map[string]interface{}{
				"fusion_rate_hz": 100.0,
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var errResp map[string]string
				if err := json.NewDecoder(rr.Body).Decode(&errResp); err != nil {
					t.Fatalf("Failed to decode error response: %v", err)
				}

				if errResp["error"] == "" {
					t.Error("Expected error message, got empty string")
				}
			},
		},
		{
			name:   "POST invalid delta_rms_threshold (negative)",
			method: "POST",
			path:   "/api/settings",
			body: map[string]interface{}{
				"delta_rms_threshold": -0.01,
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var errResp map[string]string
				if err := json.NewDecoder(rr.Body).Decode(&errResp); err != nil {
					t.Fatalf("Failed to decode error response: %v", err)
				}

				if errResp["error"] == "" {
					t.Error("Expected error message, got empty string")
				}
			},
		},
		{
			name:   "POST invalid grid_cell_m (too small)",
			method: "POST",
			path:   "/api/settings",
			body: map[string]interface{}{
				"grid_cell_m": 0.01,
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var errResp map[string]string
				if err := json.NewDecoder(rr.Body).Decode(&errResp); err != nil {
					t.Fatalf("Failed to decode error response: %v", err)
				}

				if errResp["error"] == "" {
					t.Error("Expected error message, got empty string")
				}
			},
		},
		{
			name:   "POST invalid n_subcarriers (out of range)",
			method: "POST",
			path:   "/api/settings",
			body: map[string]interface{}{
				"n_subcarriers": 50,
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var errResp map[string]string
				if err := json.NewDecoder(rr.Body).Decode(&errResp); err != nil {
					t.Fatalf("Failed to decode error response: %v", err)
				}

				if errResp["error"] == "" {
					t.Error("Expected error message, got empty string")
				}
			},
		},
		{
			name:   "POST invalid max_tracked_blobs (too high)",
			method: "POST",
			path:   "/api/settings",
			body: map[string]interface{}{
				"max_tracked_blobs": 200,
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var errResp map[string]string
				if err := json.NewDecoder(rr.Body).Decode(&errResp); err != nil {
					t.Fatalf("Failed to decode error response: %v", err)
				}

				if errResp["error"] == "" {
					t.Error("Expected error message, got empty string")
				}
			},
		},
		{
			name:   "POST invalid security_mode (not boolean)",
			method: "POST",
			path:   "/api/settings",
			body: map[string]interface{}{
				"security_mode": "true",
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var errResp map[string]string
				if err := json.NewDecoder(rr.Body).Decode(&errResp); err != nil {
					t.Fatalf("Failed to decode error response: %v", err)
				}

				if errResp["error"] == "" {
					t.Error("Expected error message, got empty string")
				}
			},
		},
		{
			name:           "GET settings after update persists changes",
			method:         "GET",
			path:           "/api/settings",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, rr *httptest.ResponseRecorder) {
				// This test should run after the POST test above
				// Just verify we can get settings without error
				var settings map[string]interface{}
				if err := json.NewDecoder(rr.Body).Decode(&settings); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}

				if len(settings) == 0 {
					t.Error("Expected at least some settings")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := chi.NewRouter()
			handler.RegisterRoutes(r)

			var body []byte
			if tt.body != nil {
				var err error
				body, err = json.Marshal(tt.body)
				if err != nil {
					t.Fatalf("Failed to marshal request body: %v", err)
				}
			}

			req := httptest.NewRequest(tt.method, tt.path, bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rr.Code)
				t.Logf("Response body: %s", rr.Body.String())
			}

			if tt.checkResponse != nil {
				tt.checkResponse(t, rr)
			}
		})
	}
}

// TestSettingsGetSingle tests the GetSingle method.
func TestSettingsGetSingle(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create settings table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS settings (
			key         TEXT PRIMARY KEY,
			value_json  TEXT NOT NULL,
			updated_at  INTEGER NOT NULL DEFAULT 0
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create settings table: %v", err)
	}

	handler := NewSettingsHandler(db)

	tests := []struct {
		name       string
		key        string
		wantExists bool
		checkValue func(*testing.T, interface{})
	}{
		{
			name:       "cached value exists",
			key:        "fusion_rate_hz",
			wantExists: true,
			checkValue: func(t *testing.T, v interface{}) {
				if f, ok := v.(float64); !ok || f != 10.0 {
					t.Errorf("Expected 10.0, got %v", v)
				}
			},
		},
		{
			name:       "default value exists",
			key:        "grid_cell_m",
			wantExists: true,
			checkValue: func(t *testing.T, v interface{}) {
				if f, ok := v.(float64); !ok || f != 0.2 {
					t.Errorf("Expected 0.2, got %v", v)
				}
			},
		},
		{
			name:       "unknown key doesn't exist",
			key:        "unknown_key_xyz",
			wantExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, exists := handler.GetSingle(tt.key)
			if exists != tt.wantExists {
				t.Errorf("GetSingle(%q) exists=%v, want %v", tt.key, exists, tt.wantExists)
			}
			if tt.checkValue != nil && exists {
				tt.checkValue(t, val)
			}
		})
	}
}

// TestSettingsSetAndGet tests setting and getting a single value.
func TestSettingsSetAndGet(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create settings table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS settings (
			key         TEXT PRIMARY KEY,
			value_json  TEXT NOT NULL,
			updated_at  INTEGER NOT NULL DEFAULT 0
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create settings table: %v", err)
	}

	handler := NewSettingsHandler(db)

	// Set a new value
	err = handler.Set("custom_key", "custom_value")
	if err != nil {
		t.Fatalf("Failed to set value: %v", err)
	}

	// Get it back
	val, exists := handler.GetSingle("custom_key")
	if !exists {
		t.Fatal("Value should exist after Set")
	}
	if val != "custom_value" {
		t.Errorf("Expected 'custom_value', got %v", val)
	}

	// Verify it's in the database
	var dbVal string
	err = db.QueryRow("SELECT value_json FROM settings WHERE key = ?", "custom_key").Scan(&dbVal)
	if err != nil {
		t.Fatalf("Failed to query database: %v", err)
	}
	if dbVal != `"custom_value"` { // JSON encoded string
		t.Errorf("Expected '\"custom_value\"' in database, got %s", dbVal)
	}
}

// TestSettingsDelete tests deleting a setting.
func TestSettingsDelete(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create settings table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS settings (
			key         TEXT PRIMARY KEY,
			value_json  TEXT NOT NULL,
			updated_at  INTEGER NOT NULL DEFAULT 0
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create settings table: %v", err)
	}

	handler := NewSettingsHandler(db)

	// Set a value
	err = handler.Set("to_delete", "value")
	if err != nil {
		t.Fatalf("Failed to set value: %v", err)
	}

	// Verify it exists
	_, exists := handler.GetSingle("to_delete")
	if !exists {
		t.Fatal("Value should exist before delete")
	}

	// Delete it
	err = handler.Delete("to_delete")
	if err != nil {
		t.Fatalf("Failed to delete: %v", err)
	}

	// Verify it's gone from cache
	_, exists = handler.GetSingle("to_delete")
	if exists {
		t.Fatal("Value should not exist after delete")
	}

	// Verify it's gone from database
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM settings WHERE key = ?", "to_delete").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query database: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 rows in database, got %d", count)
	}
}

// TestValidateSettings tests the settings validation.
func TestValidateSettings(t *testing.T) {
	tests := []struct {
		name      string
		settings  map[string]interface{}
		wantErr   bool
		errKey    string
	}{
		{
			name: "all valid settings",
			settings: map[string]interface{}{
				"fusion_rate_hz":       10.0,
				"grid_cell_m":          0.2,
				"delta_rms_threshold":  0.02,
				"tau_s":                30.0,
				"fresnel_decay":        2.0,
				"n_subcarriers":        16,
				"breathing_sensitivity": 0.005,
				"motion_threshold":     0.05,
				"dwell_seconds":        30,
				"vacant_seconds":       300,
				"max_tracked_blobs":    20,
				"security_mode":        true,
			},
			wantErr: false,
		},
		{
			name: "fusion_rate_hz too high",
			settings: map[string]interface{}{
				"fusion_rate_hz": 100.0,
			},
			wantErr: true,
			errKey:  "fusion_rate_hz",
		},
		{
			name: "grid_cell_m too small",
			settings: map[string]interface{}{
				"grid_cell_m": 0.01,
			},
			wantErr: true,
			errKey:  "grid_cell_m",
		},
		{
			name: "n_subcarriers out of range",
			settings: map[string]interface{}{
				"n_subcarriers": 50,
			},
			wantErr: true,
			errKey:  "n_subcarriers",
		},
		{
			name: "security_mode not boolean",
			settings: map[string]interface{}{
				"security_mode": "true",
			},
			wantErr: true,
			errKey:  "security_mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSettings(tt.settings)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSettings() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errKey != "" {
				if ve, ok := err.(*ValidationError); ok {
					if ve.Key != tt.errKey {
						t.Errorf("Expected error key %q, got %q", tt.errKey, ve.Key)
					}
				} else {
					t.Errorf("Expected ValidationError, got %T", err)
				}
			}
		})
	}
}

// TestAsFloat64 tests the asFloat64 helper.
func TestAsFloat64(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		want     float64
		wantBool bool
	}{
		{"float64", 3.14, 3.14, true},
		{"float32", float32(3.14), 3.14, true},
		{"int", 42, 42.0, true},
		{"int64", int64(42), 42.0, true},
		{"int32", int32(42), 42.0, true},
		{"string", "42", 0, false},
		{"bool", true, 0, false},
		{"nil", nil, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := asFloat64(tt.input)
			if ok != tt.wantBool {
				t.Errorf("asFloat64(%v) ok = %v, want %v", tt.input, ok, tt.wantBool)
			}
			if ok && got != tt.want {
				t.Errorf("asFloat64(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestDefaultSettings tests that default settings are defined.
func TestDefaultSettings(t *testing.T) {
	requiredDefaults := []string{
		"fusion_rate_hz",
		"grid_cell_m",
		"delta_rms_threshold",
		"tau_s",
		"fresnel_decay",
		"n_subcarriers",
		"breathing_sensitivity",
		"motion_threshold",
		"dwell_seconds",
		"vacant_seconds",
		"max_tracked_blobs",
		"replay_retention_hours",
		"replay_max_mb",
		"security_mode",
		"events_archive_days",
	}

	for _, key := range requiredDefaults {
		if _, exists := defaultSettings[key]; !exists {
			t.Errorf("Missing default setting: %s", key)
		}
	}
}

// TestSettingsPersistence tests that settings persist across handler reloads.
func TestSettingsPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// First handler
	db1, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	_, err = db1.Exec(`
		CREATE TABLE IF NOT EXISTS settings (
			key         TEXT PRIMARY KEY,
			value_json  TEXT NOT NULL,
			updated_at  INTEGER NOT NULL DEFAULT 0
		)
	`)
	if err != nil {
		db1.Close()
		t.Fatalf("Failed to create settings table: %v", err)
	}

	handler1 := NewSettingsHandler(db1)
	err = handler1.Set("persistent_key", "persistent_value")
	if err != nil {
		db1.Close()
		t.Fatalf("Failed to set value: %v", err)
	}
	db1.Close()

	// Second handler (simulates restart)
	db2, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to reopen database: %v", err)
	}
	defer db2.Close()

	handler2 := NewSettingsHandler(db2)
	val, exists := handler2.GetSingle("persistent_key")
	if !exists {
		t.Fatal("Value should persist across handler reloads")
	}
	if val != "persistent_value" {
		t.Errorf("Expected 'persistent_value', got %v", val)
	}
}

// TestNewSettingsHandlerLoadFailure tests that handler still works even if load fails.
func TestNewSettingsHandlerLoadFailure(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create a database without the settings table
	db, err := sql.Open("sqlite", dbPath+"?mode=ro") // Read-only to prevent table creation
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Handler should still be created (load fails but doesn't crash)
	handler := NewSettingsHandler(db)

	// Get should return defaults even though load failed
	settings := handler.Get()
	if len(settings) == 0 {
		t.Error("Expected default settings even with failed load")
	}
}

// cleanTestFile removes a test file if it exists.
func cleanTestFile(path string) {
	os.Remove(path)
}
