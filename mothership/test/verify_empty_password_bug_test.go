package test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/spaxel/mothership/internal/api"
)

// TestNetworkSettingsHandler_EmptyPassword verifies that the NetworkSettingsHandler
// correctly handles empty password fields in the settings update request.
func TestNetworkSettingsHandler_EmptyPassword(t *testing.T) {
	// Set up the HTTP test server with NetworkSettingsHandler
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

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

	settingsHandler := api.NewSettingsHandler(db)
	networkHandler := api.NewNetworkSettingsHandler(settingsHandler)

	r := chi.NewRouter()
	networkHandler.RegisterRoutes(r)

	server := httptest.NewServer(r)
	defer server.Close()

	// Test 1: Initial state should have empty wifi_ssid and configured=false
	t.Run("InitialState", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/api/settings/network")
		if err != nil {
			t.Fatalf("Failed to GET /api/settings/network: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result["wifi_ssid"] != "" {
			t.Errorf("Expected empty wifi_ssid, got %v", result["wifi_ssid"])
		}
		if result["configured"] != false {
			t.Errorf("Expected configured=false, got %v", result["configured"])
		}
	})

	// Test 2: BUG DEMONSTRATION - Update with SSID and empty password
	// This should result in configured=false, but the bug causes configured=true
	t.Run("EmptyPasswordBug", func(t *testing.T) {
		// First, set only SSID (no password)
		ssid := "TestNetwork"
		updateBody := map[string]string{
			"wifi_ssid":     ssid,
			"wifi_password": "",
		}
		bodyBytes, _ := json.Marshal(updateBody)

		req, _ := http.NewRequest("PUT", server.URL+"/api/settings/network", strings.NewReader(string(bodyBytes)))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to PUT /api/settings/network: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// BUG: The current implementation returns configured=true when password key exists
		// even if the password value is empty. This test FAILS because:
		// - We set wifi_ssid to "TestNetwork"
		// - We set wifi_password to "" (empty string)
		// - The hasPass variable is true (key exists)
		// - But passStr is "" (empty value)
		// - Configured should be false but returns true due to the hasPass logic

		// This assertion demonstrates the bug - it will FAIL
		if result["configured"] != false {
			t.Errorf("BUG DEMONSTRATED: Expected configured=false for empty password, got %v", result["configured"])
			t.Logf("Bug details: wifi_ssid=%v, configured=%v", result["wifi_ssid"], result["configured"])
		}
	})

	// Test 3: Verify that proper SSID + password results in configured=true
	t.Run("ProperCredentials", func(t *testing.T) {
		updateBody := map[string]string{
			"wifi_ssid":     "TestNetwork",
			"wifi_password": "TestPassword123",
		}
		bodyBytes, _ := json.Marshal(updateBody)

		req, _ := http.NewRequest("PUT", server.URL+"/api/settings/network",
			strings.NewReader(string(bodyBytes))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to PUT /api/settings/network: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result["configured"] != true {
			t.Errorf("Expected configured=true for proper credentials, got %v", result["configured"])
		}
	})
}
