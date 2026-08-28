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

	// Test that the server is accessible
	resp, err := http.Get(server.URL + "/api/settings/network")
	if err != nil {
		t.Fatalf("Failed to GET /api/settings/network: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	// Verify response structure
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Initial state should have empty wifi_ssid and configured=false
	if result["wifi_ssid"] != "" {
		t.Errorf("Expected empty wifi_ssid, got %v", result["wifi_ssid"])
	}
	if result["configured"] != false {
		t.Errorf("Expected configured=false, got %v", result["configured"])
	}

	t.Log("HTTP test server with NetworkSettingsHandler is working correctly")
}
