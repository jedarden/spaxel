package api

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
)

// newTestNetworkSettingsHandlerForBug creates a handler with a temporary database for bug testing
func newTestNetworkSettingsHandlerForBug(t *testing.T) (*NetworkSettingsHandler, chi.Router) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	t.Cleanup(func() { db.Close() }) //nolint:errcheck

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

	settingsHandler := NewSettingsHandler(db)
	h := NewNetworkSettingsHandler(settingsHandler)

	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return h, r
}

// TestVerifyEmptyPasswordBug demonstrates that NetworkSettingsHandler incorrectly
// reports configured=true when SSID is set but password is empty (open network).
//
// BUG: When SSID is configured with an empty password, GET /api/settings/network
// returns configured=true. This is incorrect because an empty password (open network)
// should not count as "configured" - SSID alone isn't enough for the provisioning
// defaulting logic.
//
// EXPECTED: configured should be false when password is empty.
func TestVerifyEmptyPasswordBug(t *testing.T) {
	_, r := newTestNetworkSettingsHandlerForBug(t)

	// Step 1: Configure SSID with empty password (open network)
	ssid := "OpenGuestNetwork"
	emptyPassword := ""
	body, _ := json.Marshal(struct {
		WifiSSID     *string `json:"wifi_ssid,omitempty"`
		WifiPassword *string `json:"wifi_password,omitempty"`
	}{
		WifiSSID:     &ssid,
		WifiPassword: &emptyPassword,
	})

	putReq := httptest.NewRequest(http.MethodPut, "/api/settings/network", bytes.NewReader(body))
	putRR := httptest.NewRecorder()
	r.ServeHTTP(putRR, putReq)

	if putRR.Code != http.StatusOK {
		t.Fatalf("PUT request failed: got %d: %s", putRR.Code, putRR.Body.String())
	}

	// Step 2: Call GET /api/settings/network to check configured status
	getReq := httptest.NewRequest(http.MethodGet, "/api/settings/network", nil)
	getRR := httptest.NewRecorder()
	r.ServeHTTP(getRR, getReq)

	if getRR.Code != http.StatusOK {
		t.Fatalf("GET request failed: got %d: %s", getRR.Code, getRR.Body.String())
	}

	// Step 3: Verify the bug - response.Configured should be false but is true
	var resp struct {
		WifiSSID   string `json:"wifi_ssid"`
		Configured bool   `json:"configured"`
	}
	if err := json.NewDecoder(getRR.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode GET response: %v", err)
	}

	// This test demonstrates the bug:
	// - SSID is set: "OpenGuestNetwork"
	// - Password is empty (open network)
	// - Configured should be false (empty password doesn't count as configured)
	// - But Configured is actually true (BUG)
	if resp.Configured {
		t.Logf("BUG CONFIRMED: configured=true when SSID=%q with empty password (expected false)", resp.WifiSSID)
		t.Logf("An empty password (open network) should NOT count as 'configured'")
		t.Logf("This breaks provisioning defaulting logic which expects both SSID and password")
	}

	// The assertion that proves the bug exists
	if resp.Configured != false {
		t.Errorf("BUG: expected configured=false for SSID with empty password, got configured=true")
	}
}
