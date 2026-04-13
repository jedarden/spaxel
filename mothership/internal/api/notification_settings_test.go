// Package api provides REST API handlers for Spaxel notification settings.
package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestNotificationSettingsHandler tests the notification settings endpoints.
func TestNotificationSettingsHandler(t *testing.T) {
	// Create a temporary database
	tmpDir, err := os.MkdirTemp("", "notification_settings_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")

	// Create the settings table
	db, err := openTestDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create handler
	handler := NewNotificationSettingsHandler(db)

	// Create a test router
	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	t.Run("GET /api/settings/notifications - initial state with defaults", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/settings/notifications", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var response notificationSettingsResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}

		// Check defaults
		if response.ChannelType != "none" {
			t.Errorf("Expected channel_type 'none', got '%s'", response.ChannelType)
		}
		if !response.SmartBatchingEnabled {
			t.Error("Expected smart_batching_enabled to be true by default")
		}
		if response.SmartBatchingWindow != 30 {
			t.Errorf("Expected smart_batching_window 30, got %d", response.SmartBatchingWindow)
		}
		if !response.MorningDigestEnabled {
			t.Error("Expected morning_digest_enabled to be true by default")
		}
		if response.MorningDigestTime != "07:00" {
			t.Errorf("Expected morning_digest_time '07:00', got '%s'", response.MorningDigestTime)
		}
		if response.QuietHoursDays != 0x7F {
			t.Errorf("Expected quiet_hours_days 0x7F (all days), got %d", response.QuietHoursDays)
		}
		if response.EventTypes == nil {
			t.Error("Expected event_types to be initialized")
		}
	})

	t.Run("PUT /api/settings/notifications - update channel type", func(t *testing.T) {
		reqBody := `{
			"channel_type": "ntfy",
			"channel_config": {
				"url": "https://ntfy.sh/my-topic",
				"topic": "my-topic"
			}
		}`

		req := httptest.NewRequest("PUT", "/api/settings/notifications", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var response notificationSettingsResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}

		if response.ChannelType != "ntfy" {
			t.Errorf("Expected channel_type 'ntfy', got '%s'", response.ChannelType)
		}
	})

	t.Run("PUT /api/settings/notifications - validation error for invalid channel type", func(t *testing.T) {
		reqBody := `{"channel_type": "invalid_channel"}`

		req := httptest.NewRequest("PUT", "/api/settings/notifications", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("PUT /api/settings/notifications - update quiet hours", func(t *testing.T) {
		reqBody := `{
			"quiet_hours_enabled": true,
			"quiet_hours_start": "22:00",
			"quiet_hours_end": "07:00",
			"quiet_hours_days": 127
		}`

		req := httptest.NewRequest("PUT", "/api/settings/notifications", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var response notificationSettingsResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}

		if !response.QuietHoursEnabled {
			t.Error("Expected quiet_hours_enabled to be true")
		}
		if response.QuietHoursStart != "22:00" {
			t.Errorf("Expected quiet_hours_start '22:00', got '%s'", response.QuietHoursStart)
		}
		if response.QuietHoursEnd != "07:00" {
			t.Errorf("Expected quiet_hours_end '07:00', got '%s'", response.QuietHoursEnd)
		}
		if response.QuietHoursDays != 127 {
			t.Errorf("Expected quiet_hours_days 127, got %d", response.QuietHoursDays)
		}
	})

	t.Run("PUT /api/settings/notifications - update morning digest", func(t *testing.T) {
		reqBody := `{
			"morning_digest_enabled": true,
			"morning_digest_time": "08:30"
		}`

		req := httptest.NewRequest("PUT", "/api/settings/notifications", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("PUT /api/settings/notifications - update smart batching", func(t *testing.T) {
		reqBody := `{
			"smart_batching_enabled": false,
			"smart_batching_window": 60
		}`

		req := httptest.NewRequest("PUT", "/api/settings/notifications", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("PUT /api/settings/notifications - update event types", func(t *testing.T) {
		reqBody := `{
			"event_types": {
				"zone_enter": true,
				"zone_leave": false,
				"fall_detected": true
			}
		}`

		req := httptest.NewRequest("PUT", "/api/settings/notifications", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var response notificationSettingsResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}

		if response.EventTypes["zone_enter"] != true {
			t.Error("Expected zone_enter to be true")
		}
		if response.EventTypes["zone_leave"] != false {
			t.Error("Expected zone_leave to be false")
		}
	})

	t.Run("PUT /api/settings/notifications - invalid time format", func(t *testing.T) {
		reqBody := `{"quiet_hours_start": "25:00"}`

		req := httptest.NewRequest("PUT", "/api/settings/notifications", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400 for invalid time, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("POST /api/notifications/test - no channel configured", func(t *testing.T) {
		// Reset channel to none to ensure isolation from prior sub-tests
		resetReq := httptest.NewRequest("PUT", "/api/settings/notifications", strings.NewReader(`{"channel_type":"none"}`))
		resetReq.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(httptest.NewRecorder(), resetReq)

		reqBody := `{"title": "Test", "body": "Test body"}`

		req := httptest.NewRequest("POST", "/api/notifications/test", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("POST /api/notifications/test - simulated (no sender)", func(t *testing.T) {
		// First set a channel type
		setReq := `{"channel_type": "ntfy", "channel_config": {"url": "https://ntfy.sh/test"}}`
		setReqHTTP := httptest.NewRequest("PUT", "/api/settings/notifications", strings.NewReader(setReq))
		setReqHTTP.Header.Set("Content-Type", "application/json")
		setW := httptest.NewRecorder()
		router.ServeHTTP(setW, setReqHTTP)

		// Now test
		reqBody := `{"title": "Test", "body": "Test body"}`
		req := httptest.NewRequest("POST", "/api/notifications/test", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var response map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}

		if response["status"] != "simulated" {
			t.Errorf("Expected status 'simulated', got '%v'", response["status"])
		}
	})
}

// openTestDB creates and opens a test database with the settings table.
func openTestDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	// Create settings table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS settings (
			key         TEXT PRIMARY KEY,
			value_json  TEXT NOT NULL,
			updated_at  INTEGER NOT NULL DEFAULT (unixepoch() * 1000)
		);
	`)
	if err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

// TestNotificationSettingsValidation tests validation logic.
func TestNotificationSettingsValidation(t *testing.T) {
	t.Run("validateChannelType - valid types", func(t *testing.T) {
		validTypes := []string{"none", "ntfy", "pushover", "webhook"}
		for _, typ := range validTypes {
			if err := validateChannelType(typ); err != nil {
				t.Errorf("Channel type '%s' should be valid: %v", typ, err)
			}
		}
	})

	t.Run("validateChannelType - invalid type", func(t *testing.T) {
		if err := validateChannelType("invalid"); err == nil {
			t.Error("Expected error for invalid channel type")
		}
	})

	t.Run("validateTimeFormat - valid times", func(t *testing.T) {
		validTimes := []string{"00:00", "23:59", "07:30", "12:00"}
		for _, timeStr := range validTimes {
			if err := validateTimeFormat(timeStr); err != nil {
				t.Errorf("Time '%s' should be valid: %v", timeStr, err)
			}
		}
	})

	t.Run("validateTimeFormat - invalid times", func(t *testing.T) {
		invalidTimes := []string{"25:00", "12:60", "abcd", "1:00", "12:3"}
		for _, timeStr := range invalidTimes {
			if err := validateTimeFormat(timeStr); err == nil {
				t.Errorf("Time '%s' should be invalid", timeStr)
			}
		}
	})

	t.Run("validateEventTypes - valid types", func(t *testing.T) {
		validTypes := map[string]bool{
			"zone_enter":      true,
			"zone_leave":      true,
			"fall_detected":   true,
			"anomaly_alert":   true,
		}
		if err := validateEventTypes(validTypes); err != nil {
			t.Errorf("Valid event types should pass validation: %v", err)
		}
	})

	t.Run("validateEventTypes - invalid type", func(t *testing.T) {
		invalidTypes := map[string]bool{
			"invalid_type": true,
		}
		if err := validateEventTypes(invalidTypes); err == nil {
			t.Error("Invalid event type should fail validation")
		}
	})
}
