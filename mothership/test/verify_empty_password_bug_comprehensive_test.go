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

// TestEmptyPasswordBug_Comprehensive tests various scenarios to find the bug
// where NetworkSettingsHandler might incorrectly report configured=true
// when password is empty.
func TestEmptyPasswordBug_Comprehensive(t *testing.T) {
	tests := []struct {
		name             string
		ssid             string
		password         string
		expectConfigured bool
		description      string
		skipPutPassword  bool
	}{
		{
			name:             "SSID_with_empty_password",
			ssid:             "OpenNetwork",
			password:         "",
			expectConfigured: false,
			description:      "Open network should not be configured",
		},
		{
			name:             "SSID_with_real_password",
			ssid:             "SecureNetwork",
			password:         "mypassword123",
			expectConfigured: true,
			description:      "Network with password should be configured",
		},
		{
			name:             "SSID_only_password_never_set",
			ssid:             "SSIDOnlyNet",
			password:         "", // Will not be set
			skipPutPassword:  true,
			expectConfigured: false,
			description:      "SSID without password key should not be configured",
		},
		{
			name:             "Empty_SSID_with_password",
			ssid:             "",
			password:         "anypassword",
			expectConfigured: false,
			description:      "Empty SSID should never be configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			// Set SSID
			if tt.ssid != "" {
				ssid := tt.ssid
				body, _ := json.Marshal(map[string]interface{}{
					"wifi_ssid": ssid,
				})
				req := httptest.NewRequest(http.MethodPut, "/api/settings/network", bytes.NewReader(body))
				rr := httptest.NewRecorder()
				r.ServeHTTP(rr, req)

				if rr.Code != http.StatusOK {
					t.Fatalf("PUT SSID failed: %d - %s", rr.Code, rr.Body.String())
				}
			}

			// Set password if not skipped
			if !tt.skipPutPassword {
				password := tt.password
				body, _ := json.Marshal(map[string]interface{}{
					"wifi_password": password,
				})
				req := httptest.NewRequest(http.MethodPut, "/api/settings/network", bytes.NewReader(body))
				rr := httptest.NewRecorder()
				r.ServeHTTP(rr, req)

				if rr.Code != http.StatusOK {
					t.Fatalf("PUT password failed: %d - %s", rr.Code, rr.Body.String())
				}
			}

			// GET /api/settings/network
			req := httptest.NewRequest(http.MethodGet, "/api/settings/network", nil)
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("GET failed: %d - %s", rr.Code, rr.Body.String())
			}

			var resp struct {
				WifiSSID   string `json:"wifi_ssid"`
				Configured bool   `json:"configured"`
			}
			if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
				t.Fatalf("Failed to decode GET response: %v", err)
			}

			t.Logf("Test: %s", tt.description)
			t.Logf("  SSID: %q, Password: %q", tt.ssid, tt.password)
			t.Logf("  Response: WifiSSID=%q, Configured=%v", resp.WifiSSID, resp.Configured)
			t.Logf("  Expected Configured=%v", tt.expectConfigured)

			if resp.Configured != tt.expectConfigured {
				t.Errorf("Configured mismatch: got %v, want %v", resp.Configured, tt.expectConfigured)
				if tt.expectConfigured == false && resp.Configured == true {
					t.Logf("BUG FOUND: %s incorrectly reports configured=true", tt.name)
				}
			}
		})
	}
}
