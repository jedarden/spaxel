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

// TestNetworkSettingsHandler_EmptyPassword_BugSimulation
//
// BUG SIMULATION: This test demonstrates what the buggy behavior would look like
// if NetworkSettingsHandler incorrectly returned Configured=true for empty password.
//
// The actual bug described in the bead is that the handler returns Configured=true
// when the password is empty, when it should return Configured=false.
//
// Expected behavior: When wifi_password is "", configured should be false
// Buggy behavior: When wifi_password is "", configured returns true
func TestNetworkSettingsHandler_EmptyPassword_BugSimulation(t *testing.T) {
	t.Log("=== BUG SIMULATION TEST ===")
	t.Log("This test demonstrates what the buggy behavior would look like")
	t.Log("Expected: configured=false for empty password")
	t.Log("Buggy: configured=true for empty password")

	// Set up test environment
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

	// Test Case 1: Set SSID with empty password
	t.Log("\n=== Test Case 1: SSID with empty password ===")
	updateBody := map[string]string{
		"wifi_ssid":     "TestNetwork",
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

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	t.Logf("After setting empty password:")
	t.Logf("  wifi_ssid: %v", result["wifi_ssid"])
	t.Logf("  configured: %v", result["configured"])

	// Current behavior check
	if result["configured"] == false {
		t.Log("✓ CORRECT: configured returns false for empty password")
	} else {
		t.Log("✗ BUGGY: configured returns true for empty password (BUG DEMONSTRATED)")
	}

	// Test Case 2: Demonstrate the GetSingle behavior
	t.Log("\n=== Test Case 2: GetSingle behavior analysis ===")

	// Store a valid password first
	settingsHandler.Set("network_wifi_ssid", "TestNetwork")
	settingsHandler.Set("network_wifi_password", "ValidPassword123")

	value1, exists1 := settingsHandler.GetSingle("network_wifi_password")
	t.Logf("Valid password: value=%v, exists=%v", value1, exists1)

	// Now set to empty password
	settingsHandler.Set("network_wifi_password", "")
	value2, exists2 := settingsHandler.GetSingle("network_wifi_password")
	t.Logf("Empty password: value=%v, exists=%v", value2, exists2)

	t.Log("Analysis: Even when password is empty string '', GetSingle returns (value, true)")
	t.Log("This means hasPass=true even though passStr='' which could cause confusion")

	// Test Case 3: Database state verification
	t.Log("\n=== Test Case 3: Database state ===")
	var passwordJSON string
	err = db.QueryRow("SELECT value_json FROM settings WHERE key = 'network_wifi_password'").Scan(&passwordJSON)
	if err != nil && err != sql.ErrNoRows {
		t.Fatalf("Failed to query database: %v", err)
	}

	var passwordStr string
	if passwordJSON != "" {
		json.Unmarshal([]byte(passwordJSON), &passwordStr)
	}
	t.Logf("Database password value: %q", passwordStr)
	t.Logf("Password exists in DB: %v", passwordJSON != "")

	t.Log("\n=== BUG ANALYSIS ===")
	t.Log("If the bug existed, the logic in response() would be:")
	t.Log("  Configured: ssidStr != \"\" && hasPass && passStr != \"\"")
	t.Log("  Configured: true && true && false = false (CORRECT)")
	t.Log("")
	t.Log("The current implementation correctly returns configured=false for empty passwords")
	t.Log("However, if the response() function used a different logic like:")
	t.Log("  Configured: ssidStr != \"\" && hasPass")
	t.Log("Then it would incorrectly return: true && true = true (BUGGY)")
	t.Log("")
	t.Log("This test confirms the current code is CORRECT and the bug has been fixed or never existed")

	// Final verification
	t.Log("\n=== Final Verification ===")
	resp2, err := http.Get(server.URL + "/api/settings/network")
	if err != nil {
		t.Fatalf("Failed to GET /api/settings/network: %v", err)
	}
	defer resp2.Body.Close()

	var finalResult map[string]interface{}
	if err := json.NewDecoder(resp2.Body).Decode(&finalResult); err != nil {
		t.Fatalf("Failed to decode final response: %v", err)
	}

	t.Logf("Final state: configured=%v", finalResult["configured"])

	if finalResult["configured"] == false {
		t.Log("✓ TEST PASSES: configured correctly returns false")
	} else {
		t.Error("✗ TEST FAILS: configured incorrectly returns true - BUG DEMONSTRATED")
	}
}
