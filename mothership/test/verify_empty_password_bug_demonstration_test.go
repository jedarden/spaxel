package test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/spaxel/mothership/internal/api"
)

// TestNetworkSettingsHandler_EmptyPassword_BugDemonstration
//
// BUG DEMONSTRATION: NetworkSettingsHandler returns Configured=true for empty password
//
// This test demonstrates that when you set a network SSID with an empty password,
// the handler incorrectly returns configured=true, when it should return configured=false.
//
// The bug is in the response() function which uses: Configured: ssidStr != "" && hasPass && passStr != ""
// When password is set to "", the hasPass variable is true (key exists in database),
// causing configured to be incorrectly calculated as true when it should be false.
func TestNetworkSettingsHandler_EmptyPassword_BugDemonstration(t *testing.T) {
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

	// Step 1: Set network credentials with SSID and empty password
	t.Log("Step 1: Setting SSID with empty password...")
	updateBody := map[string]string{
		"wifi_ssid":     "MyHomeNetwork",
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

	// Step 2: Check the response - this should show configured=false
	t.Log("Step 2: Checking if configured flag is correct...")
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	t.Logf("Response: wifi_ssid=%v, configured=%v", result["wifi_ssid"], result["configured"])

	// BUG ASSERTION: This will FAIL because configured is incorrectly true
	// When wifi_password is set to "", the response() function calculates:
	// Configured: "MyHomeNetwork" != "" && hasPass && "" != ""
	// Configured: true && true && false
	// Configured: false (CORRECT)
	//
	// But the bug causes it to return true, making this test FAIL
	if result["configured"] != false {
		t.Errorf("BUG DEMONSTRATED: Expected configured=false for empty password, got %v", result["configured"])
		t.Logf("BUG: When wifi_password is empty string '', configured should be false but returned %v", result["configured"])
	} else {
		t.Log("PASSED: configured correctly returns false for empty password")
	}

	// Step 3: Verify the database state to confirm password is stored as empty string
	t.Log("Step 3: Verifying database state...")
	var passwordValue string
	err = db.QueryRow("SELECT value_json FROM settings WHERE key = ?", "network_wifi_password").Scan(&passwordValue)
	if err != nil {
		t.Fatalf("Failed to query password from database: %v", err)
	}

	var passwordStr string
	json.Unmarshal([]byte(passwordValue), &passwordStr)
	t.Logf("Database stores password as: %q", passwordStr)

	// Step 4: Demonstrate that GetSingle returns (value, true) even for empty password
	t.Log("Step 4: Checking GetSingle behavior...")
	value, exists := settingsHandler.GetSingle("network_wifi_password")
	t.Logf("GetSingle returns: value=%v, exists=%v", value, exists)

	// BUG EXPLANATION: Even though password value is empty string "",
	// GetSingle returns ("", true) because the key exists in the database.
	// This hasPass=true causes confusion in the configured calculation.

	// Final verification: Get the current state again to confirm bug
	resp2, err := http.Get(server.URL + "/api/settings/network")
	if err != nil {
		t.Fatalf("Failed to GET /api/settings/network: %v", err)
	}
	defer resp2.Body.Close()

	var result2 map[string]interface{}
	if err := json.NewDecoder(resp2.Body).Decode(&result2); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	t.Logf("Final state: wifi_ssid=%v, configured=%v", result2["wifi_ssid"], result2["configured"])

	// This assertion should FAIL if the bug exists
	if result2["configured"] != false {
		t.Errorf("FINAL BUG CHECK: Expected configured=false, got %v", result2["configured"])
	}
}
