// Package test provides edge case and error scenario tests for WiFi credential handling.
// This test file complements the existing WiFi credential tests by covering:
// - Dashboard wizard interaction with network settings
// - Additional error scenarios not covered in other test files
// - Edge cases around per-device overrides
//
// Per the task requirements, this file focuses on edge cases and error handling,
// not happy path credential provisioning (which is covered in wifi_credential_*_test.go).
package test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/go-chi/chi/v5"
	"github.com/spaxel/mothership/internal/api"
	"github.com/spaxel/mothership/internal/provisioning"
)

// TestWiFiCredentialEdgeCase_DashboardWizard_ConfiguredNetwork tests the dashboard
// wizard behavior when fleet network credentials are already configured.
// The wizard should show the configured status and skip WiFi input prompts.
func TestWiFiCredentialEdgeCase_DashboardWizard_ConfiguredNetwork(t *testing.T) {
	t.Run("WizardShowsConfiguredStatus", func(t *testing.T) {
		// Setup: Configure fleet network in database
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

		settingsHandler := api.NewSettingsHandler(db)
		networkHandler := api.NewNetworkSettingsHandler(settingsHandler)

		// Configure fleet network
		r := chi.NewRouter()
		networkHandler.RegisterRoutes(r)

		fleetSSID := "ConfiguredFleetWiFi"
		fleetPass := "fleetPassword123"
		body := fmt.Sprintf(`{"wifi_ssid": "%s", "wifi_password": "%s"}`, fleetSSID, fleetPass)

		req := httptest.NewRequest(http.MethodPut, "/api/settings/network", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Failed to configure fleet network: %d - %s", rr.Code, rr.Body.String())
		}

		// GET /api/settings/network should return configured=true
		getReq := httptest.NewRequest(http.MethodGet, "/api/settings/network", nil)
		getRR := httptest.NewRecorder()
		r.ServeHTTP(getRR, getReq)

		if getRR.Code != http.StatusOK {
			t.Fatalf("GET /api/settings/network failed: %d - %s", getRR.Code, getRR.Body.String())
		}

		var resp struct {
			WifiSSID   string `json:"wifi_ssid"`
			Configured bool   `json:"configured"`
		}
		if err := json.NewDecoder(getRR.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Verify the response indicates configured network
		if !resp.Configured {
			t.Error("Expected configured=true for fleet network")
		}
		if resp.WifiSSID != fleetSSID {
			t.Errorf("Expected SSID %q, got %q", fleetSSID, resp.WifiSSID)
		}

		// Verify password is NOT in the response (write-only)
		respBody := getRR.Body.String()
		if strings.Contains(respBody, fleetPass) {
			t.Error("Password must not be returned in GET response (write-only)")
		}
	})

	t.Run("WizardShowsUnconfiguredStatus", func(t *testing.T) {
		// Setup: Empty database, no network configured
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

		settingsHandler := api.NewSettingsHandler(db)
		networkHandler := api.NewNetworkSettingsHandler(settingsHandler)

		// GET /api/settings/network should return configured=false
		r := chi.NewRouter()
		networkHandler.RegisterRoutes(r)

		req := httptest.NewRequest(http.MethodGet, "/api/settings/network", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("GET /api/settings/network failed: %d - %s", rr.Code, rr.Body.String())
		}

		var resp struct {
			WifiSSID   string `json:"wifi_ssid"`
			Configured bool   `json:"configured"`
		}
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Verify the response indicates unconfigured network
		if resp.Configured {
			t.Error("Expected configured=false when no network settings exist")
		}
		if resp.WifiSSID != "" {
			t.Errorf("Expected empty SSID, got %q", resp.WifiSSID)
		}
	})
}

// TestWiFiCredentialEdgeCase_DashboardWizard_PartialConfiguration tests wizard behavior
// when only SSID or only password is configured (partial configuration).
func TestWiFiCredentialEdgeCase_DashboardWizard_PartialConfiguration(t *testing.T) {
	tests := []struct {
		name         string
		configureSSID bool
		configurePass bool
		expectConfig  bool
		description   string
	}{
		{
			name:         "Only SSID configured",
			configureSSID: true,
			configurePass: false,
			expectConfig:  false, // Password field not set, so unconfigured
			description:   "SSID without password field should show unconfigured",
		},
		{
			name:         "Only password configured",
			configureSSID: false,
			configurePass: true,
			expectConfig:  false, // Both required
			description:   "Password without SSID should show unconfigured",
		},
		{
			name:         "Both SSID and empty password configured",
			configureSSID: true,
			configurePass: true, // Empty password (open network)
			expectConfig:  true,  // Empty password is valid for open networks
			description:   "Empty password (open network) should show configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			settingsHandler := api.NewSettingsHandler(db)
			networkHandler := api.NewNetworkSettingsHandler(settingsHandler)

			r := chi.NewRouter()
			networkHandler.RegisterRoutes(r)

			// Configure only SSID (omit password field entirely)
				if tt.configureSSID {
			body := `{"wifi_ssid": "TestSSID"}`
			req := httptest.NewRequest(http.MethodPut, "/api/settings/network", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				rr := httptest.NewRecorder()
				r.ServeHTTP(rr, req)

				if rr.Code != http.StatusOK {
					t.Fatalf("Failed to configure SSID: %d - %s", rr.Code, rr.Body.String())
				}

			if tt.configurePass {
				if tt.name == "Both SSID and empty password configured" {
					// Configure SSID with empty password
					body := `{"wifi_ssid": "OpenNet", "wifi_password": ""}`
					req := httptest.NewRequest(http.MethodPut, "/api/settings/network", strings.NewReader(body))
					req.Header.Set("Content-Type", "application/json")
					rr := httptest.NewRecorder()
					r.ServeHTTP(rr, req)

					if rr.Code != http.StatusOK {
						t.Fatalf("Failed to configure open network: %d - %s", rr.Code, rr.Body.String())
					}
				} else {
					// Configure only password (omit SSID field entirely)
					body := `{"wifi_password": "Password123"}`
					req := httptest.NewRequest(http.MethodPut, "/api/settings/network", strings.NewReader(body))
					req.Header.Set("Content-Type", "application/json")
					rr := httptest.NewRecorder()
					r.ServeHTTP(rr, req)

					if rr.Code != http.StatusOK {
						t.Fatalf("Failed to configure password: %d - %s", rr.Code, rr.Body.String())
					}
				}
			}
				}

			// GET /api/settings/network
			getReq := httptest.NewRequest(http.MethodGet, "/api/settings/network", nil)
			getRR := httptest.NewRecorder()
			r.ServeHTTP(getRR, getReq)

			if getRR.Code != http.StatusOK {
				t.Fatalf("GET /api/settings/network failed: %d - %s", getRR.Code, getRR.Body.String())
			}

			var resp struct {
				WifiSSID   string `json:"wifi_ssid"`
				Configured bool   `json:"configured"`
			}
			if err := json.NewDecoder(getRR.Body).Decode(&resp); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if resp.Configured != tt.expectConfig {
				t.Errorf("%s: expected configured=%v, got %v. %s",
					tt.name, tt.expectConfig, resp.Configured, tt.description)
			}
		})
	}
}

// TestWiFiCredentialEdgeCase_PerDeviceOverride_EmptyValues tests that empty or
// whitespace-only override values fall back to database settings rather than
// being treated as valid overrides.
func TestWiFiCredentialEdgeCase_PerDeviceOverride_EmptyValues(t *testing.T) {
	// Setup: Configure fleet network in database
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

	settingsHandler := api.NewSettingsHandler(db)
	networkHandler := api.NewNetworkSettingsHandler(settingsHandler)
	provSrv := provisioning.NewServer(tmpDir, "spaxel", 8080, "pool.ntp.org", "")
	provSrv.SetSettingsProvider(settingsHandler)

	// Configure fleet network
	r := chi.NewRouter()
	networkHandler.RegisterRoutes(r)

	fleetSSID := "FleetWiFi"
	fleetPass := "fleetPass123"
	body := fmt.Sprintf(`{"wifi_ssid": "%s", "wifi_password": "%s"}`, fleetSSID, fleetPass)

	req := httptest.NewRequest(http.MethodPut, "/api/settings/network", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Failed to configure fleet network: %d - %s", rr.Code, rr.Body.String())
	}

	tests := []struct {
		name           string
		overrideSSID   string
		overridePass   string
		expectSSID      string
		expectPass      string
		description    string
	}{
		{
			name:          "Empty SSID override falls back to DB",
			overrideSSID:   "",
			overridePass:   fleetPass,
			expectSSID:     fleetSSID, // Should use DB SSID
			expectPass:     fleetPass,
			description:   "Empty SSID in request should use database value",
		},
		{
			name:          "Empty password override falls back to DB for both",
			overrideSSID:   "OverrideSSID",
			overridePass:   "",
			expectSSID:     fleetSSID, // Conservative: empty password -> use DB for BOTH SSID and password
			expectPass:     fleetPass,
			description:   "Empty password triggers DB fallback for both fields (conservative approach)",
		},
		{
			name:          "Both empty falls back to DB",
			overrideSSID:   "",
			overridePass:   "",
			expectSSID:     fleetSSID,
			expectPass:     fleetPass,
			description:   "Both empty should use database values",
		},
		{
			name:          "Whitespace-only SSID falls back to DB",
			overrideSSID:   "   ",
			overridePass:   fleetPass,
			expectSSID:     fleetSSID,
			expectPass:     fleetPass,
			description:   "Whitespace-only SSID should be treated as empty and use DB",
		},
		{
			name:          "Whitespace-only password falls back to DB for both",
			overrideSSID:   fleetSSID,
			overridePass:   "   ",
			expectSSID:     fleetSSID, // Conservative: whitespace password -> use DB for BOTH
			expectPass:     fleetPass,
			description:   "Whitespace-only password triggers DB fallback for both fields (conservative approach)",
		},
		{
			name:          "Valid override takes precedence",
			overrideSSID:   "OverrideSSID",
			overridePass:   "OverridePass123",
			expectSSID:     "OverrideSSID",
			expectPass:     "OverridePass123",
			description:   "Valid non-empty override should take precedence over DB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overrideBody := fmt.Sprintf(`{"wifi_ssid": "%s", "wifi_pass": "%s"}`, tt.overrideSSID, tt.overridePass)

			provReq := httptest.NewRequest(http.MethodPost, "/api/provision", strings.NewReader(overrideBody))
			provReq.Header.Set("Content-Type", "application/json")
			provRR := httptest.NewRecorder()
			provSrv.HandleProvision(provRR, provReq)

			if provRR.Code != http.StatusOK {
				t.Fatalf("POST /api/provision failed: %d - %s", provRR.Code, provRR.Body.String())
			}

			var payload provisioning.Payload
			if err := json.NewDecoder(provRR.Body).Decode(&payload); err != nil {
				t.Fatalf("Failed to decode payload: %v", err)
			}

			if payload.WifiSSID != tt.expectSSID {
				t.Errorf("SSID: expected %q, got %q. %s", tt.expectSSID, payload.WifiSSID, tt.description)
			}
			if payload.WifiPass != tt.expectPass {
				t.Errorf("Password: expected %q, got %q. %s", tt.expectPass, payload.WifiPass, tt.description)
			}
		})
	}
}

// TestWiFiCredentialEdgeCase_ValidationError_SSIDAfterTrim tests that SSID values
// with leading/trailing whitespace are properly validated after trimming.
func TestWiFiCredentialEdgeCase_ValidationError_SSIDAfterTrim(t *testing.T) {
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

	settingsHandler := api.NewSettingsHandler(db)
	networkHandler := api.NewNetworkSettingsHandler(settingsHandler)

	r := chi.NewRouter()
	networkHandler.RegisterRoutes(r)

	tests := []struct {
		name          string
		requestBody   string
		expectStatus  int
		expectError   string
	}{
		{
			name:         "SSID with only leading spaces",
			requestBody:   `{"wifi_ssid": "   ValidSSID"}`,
			expectStatus:  200, // Should be trimmed and accepted
			expectError:   "",
		},
		{
			name:         "SSID with only trailing spaces",
			requestBody:   `{"wifi_ssid": "ValidSSID   "}`,
			expectStatus:  200, // Should be trimmed and accepted
			expectError:   "",
		},
		{
			name:         "SSID with leading and trailing spaces",
			requestBody:   `{"wifi_ssid": "   ValidSSID   "}`,
			expectStatus:  200, // Should be trimmed and accepted
			expectError:   "",
		},
		{
			name:         "SSID that is only spaces after trim",
			requestBody:   `{"wifi_ssid": "     "}`,
			expectStatus:  400,
			expectError:   "must not be empty",
		},
		{
			name:         "SSID with tab characters",
			requestBody:   `{"wifi_ssid": "Valid\tSSID"}`,
			expectStatus:  200, // Tabs should be preserved
			expectError:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/api/settings/network", strings.NewReader(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != tt.expectStatus {
				t.Errorf("Expected status %d, got %d. Response: %s", tt.expectStatus, rr.Code, rr.Body.String())
			}

			if tt.expectError != "" && !strings.Contains(rr.Body.String(), tt.expectError) {
				t.Errorf("Expected error message containing %q, got: %s", tt.expectError, rr.Body.String())
			}
		})
	}
}

// TestWiFiCredentialEdgeCase_ValidationError_PasswordBounds tests password validation
// edge cases including minimum length, maximum length, and special characters.
func TestWiFiCredentialEdgeCase_ValidationError_PasswordBounds(t *testing.T) {
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

	settingsHandler := api.NewSettingsHandler(db)
	networkHandler := api.NewNetworkSettingsHandler(settingsHandler)

	r := chi.NewRouter()
	networkHandler.RegisterRoutes(r)

	tests := []struct {
		name          string
		requestBody   string
		expectStatus  int
		expectError   string
	}{
		{
			name:         "Password at minimum boundary (8 chars)",
			requestBody:   `{"wifi_ssid": "TestNet", "wifi_password": "12345678"}`,
			expectStatus:  200,
			expectError:   "",
		},
		{
			name:         "Password one below minimum (7 chars)",
			requestBody:   `{"wifi_ssid": "TestNet", "wifi_password": "1234567"}`,
			expectStatus:  400,
			expectError:   "at least 8 characters",
		},
		{
			name:         "Empty password (open network)",
			requestBody:   `{"wifi_ssid": "OpenNet", "wifi_password": ""}`,
			expectStatus:  200, // Empty password is valid for open networks
			expectError:   "",
		},
		{
			name:         "Password with special characters",
			requestBody:   `{"wifi_ssid": "TestNet", "wifi_password": "P@ssw0rd!#$%"}`,
			expectStatus:  200,
			expectError:   "",
		},
		{
			name:         "Password with unicode characters",
			requestBody:   `{"wifi_ssid": "TestNet", "wifi_password": "p@sswörd123"}`,
			expectStatus:  200,
			expectError:   "",
		},
		{
			name:         "Very long password (100 chars)",
			requestBody:   `{"wifi_ssid": "TestNet", "wifi_password": "` + strings.Repeat("a", 100) + `"}`,
			expectStatus:  200, // No upper bound on password length
			expectError:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/api/settings/network", strings.NewReader(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != tt.expectStatus {
				t.Errorf("Expected status %d, got %d. Response: %s", tt.expectStatus, rr.Code, rr.Body.String())
			}

			if tt.expectError != "" && !strings.Contains(rr.Body.String(), tt.expectError) {
				t.Errorf("Expected error message containing %q, got: %s", tt.expectError, rr.Body.String())
			}
		})
	}
}

// TestWiFiCredentialEdgeCase_ConcurrentUpdates tests that concurrent updates to
// network settings are handled safely without race conditions.
func TestWiFiCredentialEdgeCase_ConcurrentUpdates(t *testing.T) {
	t.Run("ConcurrentDatabaseUpdates", func(t *testing.T) {
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

		settingsHandler := api.NewSettingsHandler(db)
		networkHandler := api.NewNetworkSettingsHandler(settingsHandler)

		r := chi.NewRouter()
		networkHandler.RegisterRoutes(r)

		// Launch 10 concurrent updates
		const concurrency = 10
		var wg sync.WaitGroup
		errors := make(chan error, concurrency)

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()

				ssid := fmt.Sprintf("ConcurrentSSID_%d", index)
				pass := fmt.Sprintf("concurrentPass_%d", index)
				body := fmt.Sprintf(`{"wifi_ssid": "%s", "wifi_password": "%s"}`, ssid, pass)

				req := httptest.NewRequest(http.MethodPut, "/api/settings/network", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				rr := httptest.NewRecorder()
				r.ServeHTTP(rr, req)

				if rr.Code != http.StatusOK {
					errors <- fmt.Errorf("goroutine %d: status %d, body: %s",
						index, rr.Code, rr.Body.String())
				}
			}(i)
		}

		// Wait for all goroutines to complete
		wg.Wait()
		close(errors)

		// Check for any errors
		for err := range errors {
			t.Errorf("Concurrent update failed: %v", err)
		}

		// Verify final state is consistent
		getReq := httptest.NewRequest(http.MethodGet, "/api/settings/network", nil)
		getRR := httptest.NewRecorder()
		r.ServeHTTP(getRR, getReq)

		if getRR.Code != http.StatusOK {
			t.Fatalf("GET /api/settings/network failed: %d - %s", getRR.Code, getRR.Body.String())
		}

		var resp struct {
			WifiSSID   string `json:"wifi_ssid"`
			Configured bool   `json:"configured"`
		}
		if err := json.NewDecoder(getRR.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Should be configured (any of the concurrent writes should have succeeded)
		if !resp.Configured {
			t.Error("Expected network to be configured after concurrent updates")
		}
		if resp.WifiSSID == "" {
			t.Error("Expected non-empty SSID after concurrent updates")
		}
	})

	t.Run("ConcurrentReadDuringWrite", func(t *testing.T) {
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

		settingsHandler := api.NewSettingsHandler(db)
		networkHandler := api.NewNetworkSettingsHandler(settingsHandler)

		// Configure initial network
		r := chi.NewRouter()
		networkHandler.RegisterRoutes(r)

		body := `{"wifi_ssid": "InitialSSID", "wifi_password": "InitialPass123"}`
		req := httptest.NewRequest(http.MethodPut, "/api/settings/network", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Failed to configure initial network: %d - %s", rr.Code, rr.Body.String())
		}

		// Launch concurrent readers while a write happens
		const readers = 5
		var wg sync.WaitGroup
		readDone := make(chan bool, readers)

		// Start readers
		for i := 0; i < readers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-readDone

				getReq := httptest.NewRequest(http.MethodGet, "/api/settings/network", nil)
				getRR := httptest.NewRecorder()
				r.ServeHTTP(getRR, getReq)

				if getRR.Code != http.StatusOK {
					t.Errorf("Concurrent GET failed: %d - %s", getRR.Code, getRR.Body.String())
				}
			}()
		}

		// Start a writer
		wg.Add(1)
		go func() {
			defer wg.Done()

			updateBody := `{"wifi_ssid": "UpdatedSSID", "wifi_password": "UpdatedPass123"}`
			updateReq := httptest.NewRequest(http.MethodPut, "/api/settings/network", strings.NewReader(updateBody))
			updateReq.Header.Set("Content-Type", "application/json")
			updateRR := httptest.NewRecorder()
			r.ServeHTTP(updateRR, updateReq)

			if updateRR.Code != http.StatusOK {
				t.Errorf("Concurrent PUT failed: %d - %s", updateRR.Code, updateRR.Body.String())
			}
		}()

		// Signal readers to proceed
		close(readDone)
		wg.Wait()
	})
}

// TestWiFiCredentialEdgeCase_PartialUpdate tests updating only SSID or only password
// (partial updates via pointer fields in the request struct).
func TestWiFiCredentialEdgeCase_PartialUpdate(t *testing.T) {
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

	settingsHandler := api.NewSettingsHandler(db)
	networkHandler := api.NewNetworkSettingsHandler(settingsHandler)

	r := chi.NewRouter()
	networkHandler.RegisterRoutes(r)

	// Test 1: Update SSID only, keep existing password
	t.Run("UpdateSSIDOnly", func(t *testing.T) {
		// First configure both
		body := `{"wifi_ssid": "InitialSSID", "wifi_password": "InitialPass123"}`
		req := httptest.NewRequest(http.MethodPut, "/api/settings/network", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Failed to configure initial network: %d - %s", rr.Code, rr.Body.String())
		}

		// Update only SSID
		updateSSID := `{"wifi_ssid": "UpdatedSSID"}`
		updateReq := httptest.NewRequest(http.MethodPut, "/api/settings/network", strings.NewReader(updateSSID))
		updateReq.Header.Set("Content-Type", "application/json")
		updateRR := httptest.NewRecorder()
		r.ServeHTTP(updateRR, updateReq)

		if updateRR.Code != http.StatusOK {
			t.Fatalf("Failed to update SSID only: %d - %s", updateRR.Code, updateRR.Body.String())
		}

		// Verify SSID changed but password persisted
		getReq := httptest.NewRequest(http.MethodGet, "/api/settings/network", nil)
		getRR := httptest.NewRecorder()
		r.ServeHTTP(getRR, getReq)

		if getRR.Code != http.StatusOK {
			t.Fatalf("GET /api/settings/network failed: %d - %s", getRR.Code, getRR.Body.String())
		}

		var resp struct {
			WifiSSID   string `json:"wifi_ssid"`
			Configured bool   `json:"configured"`
		}
		if err := json.NewDecoder(getRR.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if resp.WifiSSID != "UpdatedSSID" {
			t.Errorf("Expected SSID to be updated, got %q", resp.WifiSSID)
		}
		if !resp.Configured {
			t.Error("Expected configured=true after SSID-only update (password should persist)")
		}
	})

	// Test 2: Update password only, keep existing SSID
	t.Run("UpdatePasswordOnly", func(t *testing.T) {
		// Configure both
		body := `{"wifi_ssid": "MySSID", "wifi_password": "OldPass123"}`
		req := httptest.NewRequest(http.MethodPut, "/api/settings/network", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Failed to configure initial network: %d - %s", rr.Code, rr.Body.String())
		}

		// Update only password
		updatePass := `{"wifi_password": "NewPass123"}`
		updateReq := httptest.NewRequest(http.MethodPut, "/api/settings/network", strings.NewReader(updatePass))
		updateReq.Header.Set("Content-Type", "application/json")
		updateRR := httptest.NewRecorder()
		r.ServeHTTP(updateRR, updateReq)

		if updateRR.Code != http.StatusOK {
			t.Fatalf("Failed to update password only: %d - %s", updateRR.Code, updateRR.Body.String())
		}

		// Verify password changed but SSID persisted
		getReq := httptest.NewRequest(http.MethodGet, "/api/settings/network", nil)
		getRR := httptest.NewRecorder()
		r.ServeHTTP(getRR, getReq)

		if getRR.Code != http.StatusOK {
			t.Fatalf("GET /api/settings/network failed: %d - %s", getRR.Code, getRR.Body.String())
		}

		var resp struct {
			WifiSSID   string `json:"wifi_ssid"`
			Configured bool   `json:"configured"`
		}
		if err := json.NewDecoder(getRR.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if resp.WifiSSID != "MySSID" {
			t.Errorf("Expected SSID to persist, got %q", resp.WifiSSID)
		}
		if !resp.Configured {
			t.Error("Expected configured=true after password-only update (SSID should persist)")
		}
	})

	// Test 3: Update SSID to empty value (should be rejected)
	t.Run("UpdateSSIDToEmptyRejected", func(t *testing.T) {
		// Configure network
		body := `{"wifi_ssid": "TestSSID", "wifi_password": "TestPass123"}`
		req := httptest.NewRequest(http.MethodPut, "/api/settings/network", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Failed to configure network: %d - %s", rr.Code, rr.Body.String())
		}

		// Try to set SSID to empty
		emptySSID := `{"wifi_ssid": ""}`
		emptyReq := httptest.NewRequest(http.MethodPut, "/api/settings/network", strings.NewReader(emptySSID))
		emptyReq.Header.Set("Content-Type", "application/json")
		emptyRR := httptest.NewRecorder()
		r.ServeHTTP(emptyRR, emptyReq)

		if emptyRR.Code != http.StatusBadRequest {
			t.Errorf("Expected 400 when setting SSID to empty, got %d. Body: %s",
				emptyRR.Code, emptyRR.Body.String())
		}

		if !strings.Contains(emptyRR.Body.String(), "must not be empty") {
			t.Error("Expected error message about empty SSID")
		}
	})
}

// TestWiFiCredentialEdgeCase_ProvisioningWithNoCredentialsAnywhere tests provisioning
// when there are absolutely no credentials available (no DB, no env vars, no request override).
// Per ADR-005, this should succeed with empty credentials, allowing captive portal onboarding.
func TestWiFiCredentialEdgeCase_ProvisioningWithNoCredentialsAnywhere(t *testing.T) {
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

	settingsHandler := api.NewSettingsHandler(db)
	provSrv := provisioning.NewServer(tmpDir, "spaxel", 8080, "pool.ntp.org", "")
	provSrv.SetSettingsProvider(settingsHandler)

	// Provision with absolutely no credentials (no DB, no request body)
	provReq := httptest.NewRequest(http.MethodPost, "/api/provision", nil)
	provRR := httptest.NewRecorder()
	provSrv.HandleProvision(provRR, provReq)

	// Per ADR-005 decision 3: provisioning should succeed with empty credentials
	// This allows captive-portal-only onboarding as a valid path
	if provRR.Code != http.StatusOK {
		t.Errorf("Expected 200 OK (credential-less provisioning allowed per ADR-005), got %d. Body: %s",
			provRR.Code, provRR.Body.String())
	}

	var payload provisioning.Payload
	if err := json.NewDecoder(provRR.Body).Decode(&payload); err != nil {
		t.Fatalf("Failed to decode payload: %v", err)
	}

	// Should have empty WiFi credentials
	if payload.WifiSSID != "" || payload.WifiPass != "" {
		t.Errorf("Expected empty credentials when none configured, got SSID=%q Pass=***",
			payload.WifiSSID)
	}

	// Other required fields should still be populated
	if payload.NodeID == "" {
		t.Error("Expected node_id to be generated even without WiFi credentials")
	}
	if payload.NodeToken == "" {
		t.Error("Expected node_token to be generated even without WiFi credentials")
	}
	if payload.MsMDNS != "spaxel" {
		t.Errorf("Expected ms_mdns=spaxel, got %q", payload.MsMDNS)
	}
	if payload.MsPort != 8080 {
		t.Errorf("Expected ms_port=8080, got %d", payload.MsPort)
	}
	if payload.NTPServer != "pool.ntp.org" {
		t.Errorf("Expected ntp_server=pool.ntp.org, got %q", payload.NTPServer)
	}
}

// TestWiFiCredentialEdgeCase_SettingsIntegrity tests that the settings handler's
// in-memory cache stays synchronized with the database after network changes.
func TestWiFiCredentialEdgeCase_SettingsIntegrity(t *testing.T) {
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

	settingsHandler := api.NewSettingsHandler(db)
	networkHandler := api.NewNetworkSettingsHandler(settingsHandler)

	r := chi.NewRouter()
	networkHandler.RegisterRoutes(r)

	// Configure network
	ssid := "CacheTestSSID"
	pass := "CacheTestPass123"
	body := fmt.Sprintf(`{"wifi_ssid": "%s", "wifi_password": "%s"}`, ssid, pass)

	req := httptest.NewRequest(http.MethodPut, "/api/settings/network", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Failed to configure network: %d - %s", rr.Code, rr.Body.String())
	}

	// Verify cache is synchronized via GET
	getReq := httptest.NewRequest(http.MethodGet, "/api/settings/network", nil)
	getRR := httptest.NewRecorder()
	r.ServeHTTP(getRR, getReq)

	if getRR.Code != http.StatusOK {
		t.Fatalf("GET /api/settings/network failed: %d - %s", getRR.Code, getRR.Body.String())
	}

	var resp struct {
		WifiSSID   string `json:"wifi_ssid"`
		Configured bool   `json:"configured"`
	}
	if err := json.NewDecoder(getRR.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Cache should reflect the updated values
	if resp.WifiSSID != ssid {
		t.Errorf("Cache not synchronized: expected SSID %q, got %q", ssid, resp.WifiSSID)
	}
	if !resp.Configured {
		t.Error("Cache not synchronized: expected configured=true")
	}

	// Verify cache via direct settings handler access
	if cachedSSID, ok := settingsHandler.GetSingle("network_wifi_ssid"); !ok || cachedSSID != ssid {
		t.Errorf("Cache not synchronized via GetSingle: expected ok=true value=%q, got ok=%v value=%q",
			ssid, ok, cachedSSID)
	}

	// Update password and verify cache update
	newPass := "UpdatedCachePass123"
	updateBody := `{"wifi_password": "` + newPass + `"}`
	updateReq := httptest.NewRequest(http.MethodPut, "/api/settings/network", strings.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRR := httptest.NewRecorder()
	r.ServeHTTP(updateRR, updateReq)

	if updateRR.Code != http.StatusOK {
		t.Fatalf("Failed to update password: %d - %s", updateRR.Code, updateRR.Body.String())
	}

	// Verify cache reflects the password change (configured should remain true)
	getReq2 := httptest.NewRequest(http.MethodGet, "/api/settings/network", nil)
	getRR2 := httptest.NewRecorder()
	r.ServeHTTP(getRR2, getReq2)

	if getRR2.Code != http.StatusOK {
		t.Fatalf("GET /api/settings/network after update failed: %d - %s", getRR2.Code, getRR2.Body.String())
	}

	var resp2 struct {
		WifiSSID   string `json:"wifi_ssid"`
		Configured bool   `json:"configured"`
	}
	if err := json.NewDecoder(getRR2.Body).Decode(&resp2); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp2.WifiSSID != ssid {
		t.Errorf("SSID should remain after password update, expected %q, got %q",
			ssid, resp2.WifiSSID)
	}
	if !resp2.Configured {
		t.Error("Configured should remain true after password update")
	}
}

// TestWiFiCredentialEdgeCase_ProvisioningOverrideValidation tests that the
// provisioning endpoint validates request body parameters, even when override is used.
func TestWiFiCredentialEdgeCase_ProvisioningOverrideValidation(t *testing.T) {
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

	settingsHandler := api.NewSettingsHandler(db)
	provSrv := provisioning.NewServer(tmpDir, "spaxel", 8080, "pool.ntp.org", "")
	provSrv.SetSettingsProvider(settingsHandler)

	// Configure fleet network as fallback
	networkHandler := api.NewNetworkSettingsHandler(settingsHandler)
	r := chi.NewRouter()
	networkHandler.RegisterRoutes(r)
	body := `{"wifi_ssid": "FallbackSSID", "wifi_password": "FallbackPass123"}`
	req := httptest.NewRequest(http.MethodPut, "/api/settings/network", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Failed to configure fallback network: %d - %s", rr.Code, rr.Body.String())
	}

	tests := []struct {
		name         string
		requestBody   string
		expectStatus int
		expectSSID   string // Expected SSID in response (empty if error)
		description  string
	}{
		{
			name:         "Override with empty SSID uses DB",
			requestBody:   `{"wifi_ssid": "", "wifi_pass": "OverridePass"}`,
			expectStatus: 200,
			expectSSID:   "FallbackSSID",
			description:  "Empty SSID override should fall back to database",
		},
		{
			name:         "Override with empty password uses DB for both",
			requestBody:   `{"wifi_ssid": "OverrideSSID", "wifi_pass": ""}`,
			expectStatus: 200,
			expectSSID:   "FallbackSSID", // Conservative: empty password -> use DB for BOTH SSID and password
			description:  "Empty password override should fall back to database for BOTH SSID and password (conservative approach)",
		},
		{
			name:         "Override with invalid SSID uses DB",
			requestBody:   `{"wifi_ssid": "   ", "wifi_pass": "OverridePass"}`,
			expectStatus: 200,
			expectSSID:   "FallbackSSID",
			description:  "Whitespace-only SSID override should fall back to database",
		},
		}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provReq := httptest.NewRequest(http.MethodPost, "/api/provision", strings.NewReader(tt.requestBody))
			provReq.Header.Set("Content-Type", "application/json")
			provRR := httptest.NewRecorder()
			provSrv.HandleProvision(provRR, provReq)

			if provRR.Code != tt.expectStatus {
				t.Errorf("Expected status %d, got %d. %s. Body: %s",
					tt.expectStatus, provRR.Code, tt.description, provRR.Body.String())
			}

			if tt.expectStatus == 200 {
				var payload provisioning.Payload
				if err := json.NewDecoder(provRR.Body).Decode(&payload); err != nil {
					t.Fatalf("Failed to decode payload: %v", err)
				}

				if payload.WifiSSID != tt.expectSSID {
					t.Errorf("SSID: expected %q, got %q. %s",
						tt.expectSSID, payload.WifiSSID, tt.description)
				}
				// Password should come from DB since override was empty/invalid
				if tt.expectSSID == "FallbackSSID" {
					// Fallback to DB means using the DB password too
					if payload.WifiPass != "FallbackPass123" {
						t.Errorf("Password: expected fallback DB password, got %q. %s",
							payload.WifiPass, tt.description)
					}
				} else if tt.expectSSID == "OverrideSSID" && tt.requestBody != "" {
					// Override SSID was valid, so password should fall back to DB
					if payload.WifiPass != "FallbackPass123" {
						t.Errorf("Password: expected fallback DB password, got %q. %s",
							payload.WifiPass, tt.description)
					}
				}
			}
		})
	}
}
