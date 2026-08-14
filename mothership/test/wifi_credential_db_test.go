// Package test provides database WiFi credential provisioning tests.
// This test validates ADR-005: WiFi credentials are configured in the database
// and the provisioning endpoint returns credentials from the database.
package test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/go-chi/chi/v5"
	"github.com/spaxel/mothership/internal/api"
	"github.com/spaxel/mothership/internal/provisioning"
)

// TestWiFiCredentialDatabase_ConfigureAndVerify tests the complete database
// configuration path: configure WiFi credentials in database, then verify
// the provisioning endpoint returns those credentials.
func TestWiFiCredentialDatabase_ConfigureAndVerify(t *testing.T) {
	// Setup: in-memory database with settings table
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

	// Wire up the components
	settingsHandler := api.NewSettingsHandler(db)
	networkHandler := api.NewNetworkSettingsHandler(settingsHandler)
	provSrv := provisioning.NewServer(tmpDir, "spaxel", 8080, "pool.ntp.org", "")
	provSrv.SetSettingsProvider(settingsHandler)

	// Step 1: Configure fleet WiFi via PUT /api/settings/network
	t.Run("ConfigureDatabaseWiFi", func(t *testing.T) {
		r := chi.NewRouter()
		networkHandler.RegisterRoutes(r)

		testSSID := "TestFleetWiFi"
		testPass := "testFleetPassword123"
		body := fmt.Sprintf(`{"wifi_ssid": "%s", "wifi_password": "%s"}`, testSSID, testPass)

		req := httptest.NewRequest(http.MethodPut, "/api/settings/network", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("PUT /api/settings/network failed: %d - %s", rr.Code, rr.Body.String())
		}

		var resp struct {
			WifiSSID   string `json:"wifi_ssid"`
			Configured bool   `json:"configured"`
		}
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if resp.WifiSSID != testSSID {
			t.Errorf("Expected SSID %q, got %q", testSSID, resp.WifiSSID)
		}
		if !resp.Configured {
			t.Error("Expected configured=true after setting both SSID and password")
		}

		// Verify password is not echoed back
		if strings.Contains(rr.Body.String(), testPass) {
			t.Error("Password must not be echoed in GET response")
		}
	})

	// Step 2: Verify provisioning endpoint returns credentials from database
	t.Run("ProvisioningReturnsDatabaseCredentials", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/provision", nil)
		rr := httptest.NewRecorder()
		provSrv.HandleProvision(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("POST /api/provision failed: %d - %s", rr.Code, rr.Body.String())
		}

		var payload provisioning.Payload
		if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
			t.Fatalf("Failed to decode payload: %v", err)
		}

		// Verify the provisioning endpoint returned credentials from database
		if payload.WifiSSID != "TestFleetWiFi" {
			t.Errorf("Expected WiFi SSID from database, got %q", payload.WifiSSID)
		}
		if payload.WifiPass != "testFleetPassword123" {
			t.Errorf("Expected WiFi password from database, got %q", payload.WifiPass)
		}

		// Verify other payload fields are populated
		if payload.NodeID == "" {
			t.Error("Expected node_id to be generated")
		}
		if payload.NodeToken == "" {
			t.Error("Expected node_token to be generated")
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
	})
}

// TestWiFiCredentialDatabase_MultipleProvisioningRequests verifies that multiple
// provisioning requests without WiFi parameters return the same database credentials.
func TestWiFiCredentialDatabase_MultipleProvisioningRequests(t *testing.T) {
	// Setup database and configure WiFi credentials
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
	provSrv := provisioning.NewServer(tmpDir, "spaxel", 8080, "pool.ntp.org", "")
	provSrv.SetSettingsProvider(settingsHandler)

	// Configure WiFi credentials in database
	r := chi.NewRouter()
	networkHandler.RegisterRoutes(r)
	configSSID := "SharedFleetWiFi"
	configPass := "sharedPassword123"
	body := fmt.Sprintf(`{"wifi_ssid": "%s", "wifi_password": "%s"}`, configSSID, configPass)

	req := httptest.NewRequest(http.MethodPut, "/api/settings/network", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Failed to configure database WiFi: %d - %s", rr.Code, rr.Body.String())
	}

	// Make multiple provisioning requests - all should return the same database credentials
	for i := 1; i <= 3; i++ {
		t.Run(fmt.Sprintf("ProvisioningRequest_%d", i), func(t *testing.T) {
			provReq := httptest.NewRequest(http.MethodPost, "/api/provision", nil)
			provRR := httptest.NewRecorder()
			provSrv.HandleProvision(provRR, provReq)

			if provRR.Code != http.StatusOK {
				t.Fatalf("POST /api/provision failed on request %d: %d - %s", i, provRR.Code, provRR.Body.String())
			}

			var payload provisioning.Payload
			if err := json.NewDecoder(provRR.Body).Decode(&payload); err != nil {
				t.Fatalf("Failed to decode payload on request %d: %v", i, err)
			}

			// Each request should return the same database credentials
			if payload.WifiSSID != configSSID {
				t.Errorf("Request %d: Expected SSID %q, got %q", i, configSSID, payload.WifiSSID)
			}
			if payload.WifiPass != configPass {
				t.Errorf("Request %d: Expected password matching database, got %q", i, payload.WifiPass)
			}

			// Each request should get a unique node ID and token
			if payload.NodeID == "" {
				t.Errorf("Request %d: Expected node_id to be generated", i)
			}
			if payload.NodeToken == "" {
				t.Errorf("Request %d: Expected node_token to be generated", i)
			}
		})
	}
}

// TestWiFiCredentialDatabase_OpenNetwork tests configuring an open network
// (empty password) in the database and provisioning it.
func TestWiFiCredentialDatabase_OpenNetwork(t *testing.T) {
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
	provSrv := provisioning.NewServer(tmpDir, "spaxel", 8080, "pool.ntp.org", "")
	provSrv.SetSettingsProvider(settingsHandler)

	// Configure open network (empty password)
	r := chi.NewRouter()
	networkHandler.RegisterRoutes(r)
	openSSID := "OpenGuestNetwork"
	emptyPass := ""
	body := fmt.Sprintf(`{"wifi_ssid": "%s", "wifi_password": "%s"}`, openSSID, emptyPass)

	req := httptest.NewRequest(http.MethodPut, "/api/settings/network", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("PUT /api/settings/network for open network failed: %d - %s", rr.Code, rr.Body.String())
	}

	// Provision should get SSID with empty password
	provReq := httptest.NewRequest(http.MethodPost, "/api/provision", nil)
	provRR := httptest.NewRecorder()
	provSrv.HandleProvision(provRR, provReq)

	if provRR.Code != http.StatusOK {
		t.Fatalf("POST /api/provision failed: %d - %s", provRR.Code, provRR.Body.String())
	}

	var payload provisioning.Payload
	if err := json.NewDecoder(provRR.Body).Decode(&payload); err != nil {
		t.Fatalf("Failed to decode payload: %v", err)
	}

	if payload.WifiSSID != openSSID {
		t.Errorf("Expected open network SSID, got %q", payload.WifiSSID)
	}
	if payload.WifiPass != "" {
		t.Errorf("Expected empty password for open network, got %q", payload.WifiPass)
	}
}

// TestWiFiCredentialDatabase_UpdateCredentials tests that updating WiFi
// credentials in the database is reflected in subsequent provisioning requests.
func TestWiFiCredentialDatabase_UpdateCredentials(t *testing.T) {
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
	provSrv := provisioning.NewServer(tmpDir, "spaxel", 8080, "pool.ntp.org", "")
	provSrv.SetSettingsProvider(settingsHandler)

	// Initial configuration
	r := chi.NewRouter()
	networkHandler.RegisterRoutes(r)

	initialSSID := "InitialNetwork"
	initialPass := "initialPass123"
	body := fmt.Sprintf(`{"wifi_ssid": "%s", "wifi_password": "%s"}`, initialSSID, initialPass)

	req := httptest.NewRequest(http.MethodPut, "/api/settings/network", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Failed to configure initial credentials: %d - %s", rr.Code, rr.Body.String())
	}

	// Provision with initial credentials
	provReq := httptest.NewRequest(http.MethodPost, "/api/provision", nil)
	provRR := httptest.NewRecorder()
	provSrv.HandleProvision(provRR, provReq)

	if provRR.Code != http.StatusOK {
		t.Fatalf("POST /api/provision failed: %d - %s", provRR.Code, provRR.Body.String())
	}

	var payload provisioning.Payload
	if err := json.NewDecoder(provRR.Body).Decode(&payload); err != nil {
		t.Fatalf("Failed to decode initial payload: %v", err)
	}

	if payload.WifiSSID != initialSSID {
		t.Errorf("Expected initial SSID, got %q", payload.WifiSSID)
	}

	// Update credentials
	updatedSSID := "UpdatedNetwork"
	updatedPass := "updatedPass123"
	updateBody := fmt.Sprintf(`{"wifi_ssid": "%s", "wifi_password": "%s"}`, updatedSSID, updatedPass)

	updateReq := httptest.NewRequest(http.MethodPut, "/api/settings/network", strings.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRR := httptest.NewRecorder()
	r.ServeHTTP(updateRR, updateReq)

	if updateRR.Code != http.StatusOK {
		t.Fatalf("Failed to update credentials: %d - %s", updateRR.Code, updateRR.Body.String())
	}

	// Provision again - should get updated credentials
	provReq2 := httptest.NewRequest(http.MethodPost, "/api/provision", nil)
	provRR2 := httptest.NewRecorder()
	provSrv.HandleProvision(provRR2, provReq2)

	if provRR2.Code != http.StatusOK {
		t.Fatalf("POST /api/provision after update failed: %d - %s", provRR2.Code, provRR2.Body.String())
	}

	var payload2 provisioning.Payload
	if err := json.NewDecoder(provRR2.Body).Decode(&payload2); err != nil {
		t.Fatalf("Failed to decode updated payload: %v", err)
	}

	// Should have the updated credentials
	if payload2.WifiSSID != updatedSSID {
		t.Errorf("Expected updated SSID, got %q", payload2.WifiSSID)
	}
	if payload2.WifiPass != updatedPass {
		t.Errorf("Expected updated password, got %q", payload2.WifiPass)
	}
}
