package test

import (
	"bytes"
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

// TestEmptyPasswordBug demonstrates that NetworkSettingsHandler incorrectly
// reports configured=true when SSID is set but password is empty (open network).
//
// This test reproduces the bug by:
// 1. Setting an SSID with an empty password via PUT /api/settings/network
// 2. Calling GET /api/settings/network
// 3. Verifying that response.Configured returns true (BUG) instead of expected false
func TestEmptyPasswordBug(t *testing.T) {
	// Setup test database and handler
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
	h := api.NewNetworkSettingsHandler(settingsHandler)

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	// Step 1: Configure SSID with empty password (open network)
	ssid := "OpenGuestNetwork"
	emptyPassword := ""
	body, _ := json.Marshal(map[string]interface{}{
		"wifi_ssid":     ssid,
		"wifi_password": emptyPassword,
	})

	putReq := httptest.NewRequest(http.MethodPut, "/api/settings/network", bytes.NewReader(body))
	putRR := httptest.NewRecorder()
	r.ServeHTTP(putRR, putReq)

	if putRR.Code != http.StatusOK {
		t.Fatalf("PUT request failed: %d - %s", putRR.Code, putRR.Body.String())
	}

	t.Logf("PUT response: %s", putRR.Body.String())

	// Step 2: GET /api/settings/network to check configured flag
	getReq := httptest.NewRequest(http.MethodGet, "/api/settings/network", nil)
	getRR := httptest.NewRecorder()
	r.ServeHTTP(getRR, getReq)

	if getRR.Code != http.StatusOK {
		t.Fatalf("GET request failed: %d - %s", getRR.Code, getRR.Body.String())
	}

	var resp struct {
		WifiSSID   string `json:"wifi_ssid"`
		Configured bool   `json:"configured"`
	}
	if err := json.NewDecoder(getRR.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode GET response: %v", err)
	}

	t.Logf("GET response: WifiSSID=%q, Configured=%v", resp.WifiSSID, resp.Configured)

	// Step 3: Verify the bug - Configured should be false but is true
	if resp.Configured {
		t.Logf("BUG CONFIRMED: Configured=true when SSID=%q and password is empty (expected false)", resp.WifiSSID)
		t.Error("BUG: Configured should be false for open networks (empty password), but got true")
	} else {
		t.Logf("No bug: Configured correctly returns false for SSID=%q with empty password", resp.WifiSSID)
	}
}

// TestEmptyPasswordBug_NeverSetPassword tests the case where SSID is set but password
// was never set at all (key doesn't exist in database).
func TestEmptyPasswordBug_NeverSetPassword(t *testing.T) {
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
	h := api.NewNetworkSettingsHandler(settingsHandler)

	r := chi.NewRouter()
	h.RegisterRoutes(r)

	// Set SSID only, never touch password
	ssid := "OnlySSIDNetwork"
	body, _ := json.Marshal(map[string]interface{}{
		"wifi_ssid": ssid,
	})

	putReq := httptest.NewRequest(http.MethodPut, "/api/settings/network", bytes.NewReader(body))
	putRR := httptest.NewRecorder()
	r.ServeHTTP(putRR, putReq)

	if putRR.Code != http.StatusOK {
		t.Fatalf("PUT request failed: %d - %s", putRR.Code, putRR.Body.String())
	}

	// Check configured flag
	getReq := httptest.NewRequest(http.MethodGet, "/api/settings/network", nil)
	getRR := httptest.NewRecorder()
	r.ServeHTTP(getRR, getReq)

	var resp struct {
		WifiSSID   string `json:"wifi_ssid"`
		Configured bool   `json:"configured"`
	}
	if err := json.NewDecoder(getRR.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode GET response: %v", err)
	}

	t.Logf("GET response: WifiSSID=%q, Configured=%v", resp.WifiSSID, resp.Configured)

	// When password was never set, Configured should be false
	if resp.Configured {
		t.Errorf("BUG: Configured should be false when password was never set, got true")
	}
}
