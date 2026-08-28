// Package test provides tests for WiFi credential handling when environment variables are missing.
//
// WiFi Test Structure Documentation:
// See /home/coding/spaxel/mothership/test/WIFI_TEST_STRUCTURE_NOTES.md for comprehensive
// documentation on test structure, environment variable setup, and validation patterns.
//
// Where to Add New WiFi Tests:
// 1. Create new file: /home/coding/spaxel/mothership/test/wifi_credential_<purpose>_test.go
// 2. Use naming pattern: TestWiFiCredential_<Scenario> for test functions
// 3. Follow existing patterns in this file for:
//    - Environment variable setup with t.Setenv()
//    - Temporary directory creation with t.TempDir()
//    - SQLite database initialization
//    - SettingsHandler and Provisioning server creation
//    - Panic recovery with defer/recover()
//    - JSON payload decoding and assertions
//
// Current Test Coverage:
// - This file: Missing environment variables handling
// - wifi_credential_env_test.go: Environment variable provisioning flows
// - wifi_credential_e2e_test.go: End-to-end acceptance criteria
// - wifi_credential_flow_test.go: Various provisioning flows and fallbacks
//
// ADR-005 Design Context:
// WiFi credentials are OPTIONAL per ADR-005:
// - Empty credentials are allowed (enables captive portal onboarding)
// - Environment variables seed database on first boot only
// - Database settings are authoritative after first boot
// - Request body overrides both env vars and database settings
//
// Validation Implementation:
// - Config loading: mothership/internal/config/config.go:76-77, 274-278
// - First-boot seeding: mothership/cmd/mothership/main.go:656-701 (seedWiFiCredentialsIfFirstBoot)
//
// Error Handling Pattern:
// Tests verify NO PANIC occurs with missing credentials (defer/recover pattern)
// Empty wifi_ssid and wifi_pass fields are expected when no credentials configured
// Essential provisioning fields (node_id, node_token) are still generated
package test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
	"github.com/go-chi/chi/v5"
	"github.com/spaxel/mothership/internal/api"
	"github.com/spaxel/mothership/internal/provisioning"
)

// TestWiFiCredential_MissingEnvironmentVariables verifies that the system
// handles missing WiFi credentials gracefully without crashing or panicking.
// This test addresses the specific scenario where:
// 1. No WIFI_SSID/WIFI_PASSWORD env vars are set
// 2. No database settings are configured
// 3. System should handle this appropriately (per ADR-005, this means allowing
//    captive portal onboarding with empty credentials)
func TestWiFiCredential_MissingEnvironmentVariables(t *testing.T) {
	t.Run("NoEnvVars_NoDatabaseSettings_NoCrash", func(t *testing.T) {
		// Setup: Create temporary directory and database
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)")
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

		// Clear any environment variables that might be set
		// (t.Setenv automatically restores the original value after the test)
		t.Setenv("SPAXEL_WIFI_SSID", "")
		t.Setenv("SPAXEL_WIFI_PASSWORD", "")

		// Verify env vars are not set
		if os.Getenv("SPAXEL_WIFI_SSID") != "" {
			t.Error("Expected SPAXEL_WIFI_SSID to be empty for this test")
		}
		if os.Getenv("SPAXEL_WIFI_PASSWORD") != "" {
			t.Error("Expected SPAXEL_WIFI_PASSWORD to be empty for this test")
		}

		// Create provisioning server with settings provider
		settingsHandler := api.NewSettingsHandler(db)
		provSrv := provisioning.NewServer(tmpDir, "spaxel", 8080, "pool.ntp.org", "")
		provSrv.SetSettingsProvider(settingsHandler)

		// Verify the server was created successfully (no panic)
		if provSrv == nil {
			t.Fatal("Failed to create provisioning server")
		}

		// Verify install secret was loaded/generated (no crash during init)
		secret := provSrv.GetInstallSecret()
		if len(secret) != 32 {
			t.Errorf("Expected 32-byte install secret, got %d bytes", len(secret))
		}

		// Test provisioning with absolutely no credentials available
		// (no env vars, no database settings, no request body override)
		provReq := httptest.NewRequest(http.MethodPost, "/api/provision", nil)
		provRR := httptest.NewRecorder()

		// This call should not panic or crash
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Provisioning with missing credentials panicked: %v", r)
			}
		}()

		provSrv.HandleProvision(provRR, provReq)

		// Verify HTTP response is received (no crash)
		if provRR.Code == 0 {
			t.Error("Expected HTTP response status code, got 0 (possible panic)")
		}

		// Per ADR-005, the system allows credential-less provisioning
		// for captive portal onboarding, so we expect HTTP 200
		if provRR.Code != http.StatusOK {
			t.Errorf("Expected HTTP 200 (credential-less provisioning allowed per ADR-005), got %d. Body: %s",
				provRR.Code, provRR.Body.String())
		}

		// Verify we can decode the response (no malformed JSON)
		var payload provisioning.Payload
		if err := json.NewDecoder(provRR.Body).Decode(&payload); err != nil {
			t.Errorf("Failed to decode provisioning payload: %v", err)
		}

		// Verify the response clearly indicates missing credentials
		// (empty wifi_ssid and wifi_pass fields)
		if payload.WifiSSID != "" {
			t.Errorf("Expected empty SSID when no credentials configured, got %q", payload.WifiSSID)
		}
		if payload.WifiPass != "" {
			t.Errorf("Expected empty password when no credentials configured, got %q", payload.WifiPass)
		}

		// Verify essential provisioning fields are still generated
		// (device can still be provisioned for captive portal onboarding)
		if payload.NodeID == "" {
			t.Error("Expected node_id to be generated even without WiFi credentials")
		}
		if payload.NodeToken == "" {
			t.Error("Expected node_token to be generated even without WiFi credentials")
		}
		if payload.MsMDNS == "" {
			t.Error("Expected ms_mdns to be set even without WiFi credentials")
		}
		if payload.MsPort == 0 {
			t.Error("Expected ms_port to be set even without WiFi credentials")
		}
		if payload.NTPServer == "" {
			t.Error("Expected ntp_server to be set even without WiFi credentials")
		}

		// Log verification: the test itself confirms no panic occurred
		// The system logs a warning message (visible in test output)
		t.Log("✓ System handled missing WiFi credentials gracefully without crashing")
		t.Log("✓ Provisioning payload returned with empty WiFi credentials")
		t.Log("✓ Node can proceed with captive portal onboarding (per ADR-005)")
	})

	t.Run("NoEnvVars_WithDatabaseSettings_Fallback", func(t *testing.T) {
		// Setup: Configure database settings (simulating env vars were used on first boot)
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)")
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

		// Clear environment variables
		t.Setenv("SPAXEL_WIFI_SSID", "")
		t.Setenv("SPAXEL_WIFI_PASSWORD", "")

		// Configure database settings (simulating first-boot seeding from env vars)
		settingsHandler := api.NewSettingsHandler(db)
		networkHandler := api.NewNetworkSettingsHandler(settingsHandler)
		r := chi.NewRouter()
		networkHandler.RegisterRoutes(r)

		dbSSID := "DatabaseFleetWiFi"
		dbPass := "DatabaseFleetPass123"
		body := fmt.Sprintf(`{"wifi_ssid": "%s", "wifi_password": "%s"}`, dbSSID, dbPass)

		req := httptest.NewRequest(http.MethodPut, "/api/settings/network", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Failed to configure database settings: %d - %s", rr.Code, rr.Body.String())
		}

		// Create provisioning server
		provSrv := provisioning.NewServer(tmpDir, "spaxel", 8080, "pool.ntp.org", "")
		provSrv.SetSettingsProvider(settingsHandler)

		// Test provisioning with no env vars but database settings available
		provReq := httptest.NewRequest(http.MethodPost, "/api/provision", nil)
		provRR := httptest.NewRecorder()

		// This should not crash
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Provisioning panicked: %v", r)
			}
		}()

		provSrv.HandleProvision(provRR, provReq)

		if provRR.Code != http.StatusOK {
			t.Errorf("Expected HTTP 200, got %d. Body: %s", provRR.Code, provRR.Body.String())
		}

		var payload provisioning.Payload
		if err := json.NewDecoder(provRR.Body).Decode(&payload); err != nil {
			t.Fatalf("Failed to decode payload: %v", err)
		}

		// Should use database settings when env vars are missing
		if payload.WifiSSID != dbSSID {
			t.Errorf("Expected database SSID %q when env vars missing, got %q", dbSSID, payload.WifiSSID)
		}
		if payload.WifiPass != dbPass {
			t.Errorf("Expected database password when env vars missing, got %q", payload.WifiPass)
		}

		t.Log("✓ System correctly falls back to database settings when env vars are missing")
	})

	t.Run("EnvVarsCleared_DatabaseNotConfigured_CredentiallessProvisioning", func(t *testing.T) {
		// This test simulates the scenario where:
		// 1. Environment variables were used on first boot and are now cleared
		// 2. Database settings were never configured (empty database)
		// 3. System should still handle this gracefully

		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)")
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

		// Clear environment variables (simulating they were unset after first boot)
		t.Setenv("SPAXEL_WIFI_SSID", "")
		t.Setenv("SPAXEL_WIFI_PASSWORD", "")

		settingsHandler := api.NewSettingsHandler(db)
		provSrv := provisioning.NewServer(tmpDir, "spaxel", 8080, "pool.ntp.org", "")
		provSrv.SetSettingsProvider(settingsHandler)

		// Verify database has no WiFi settings configured
		// GetSingle returns (value, false) when key doesn't exist
		if ssid, ok := settingsHandler.GetSingle("network_wifi_ssid"); ok {
			t.Errorf("Expected no WiFi SSID in database, found ok=true value=%q", ssid)
		}
		if pass, ok := settingsHandler.GetSingle("network_wifi_password"); ok {
			t.Errorf("Expected no WiFi password in database, found ok=true value=%q", pass)
		}

		// Provisioning should still succeed with empty credentials
		provReq := httptest.NewRequest(http.MethodPost, "/api/provision", nil)
		provRR := httptest.NewRecorder()

		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Provisioning with completely missing credentials panicked: %v", r)
			}
		}()

		provSrv.HandleProvision(provRR, provReq)

		if provRR.Code != http.StatusOK {
			t.Errorf("Expected HTTP 200 for credential-less provisioning, got %d. Body: %s",
				provRR.Code, provRR.Body.String())
		}

		var payload provisioning.Payload
		if err := json.NewDecoder(provRR.Body).Decode(&payload); err != nil {
			t.Fatalf("Failed to decode payload: %v", err)
		}

		// All credential fields should be empty
		if payload.WifiSSID != "" {
			t.Errorf("Expected empty SSID, got %q", payload.WifiSSID)
		}
		if payload.WifiPass != "" {
			t.Errorf("Expected empty password, got %q", payload.WifiPass)
		}

		// But provisioning should still be possible (captive portal path)
		if payload.NodeID == "" {
			t.Error("Expected node_id to be generated for captive portal onboarding")
		}

		t.Log("✓ System handles completely missing credentials gracefully (captive portal path available)")
	})
}
