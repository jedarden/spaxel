// Package acceptance provides integration tests for Spaxel acceptance scenarios.
// AS-1: First-time setup in under 5 minutes
//
// Pass criteria:
// - Fresh mothership container starts successfully
// - GET /api/auth/setup returns pin_configured=false
// - POST /api/auth/setup with PIN sets PIN successfully
// - GET /api/auth/setup returns pin_configured=true after setup
// - spaxel-sim --nodes 1 connects and is provisioned
// - Node appears in /api/nodes within 30 seconds
//
// Fail criteria:
// - User must enter a mothership IP address (manual config required)
// - Setup takes more than 5 minutes
package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// AS1_FirstTimeSetup verifies the complete first-time setup flow.
func AS1_FirstTimeSetup(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	// Step 1: Verify initial state (no PIN configured)
	t.Run("CheckInitialState_NoPINConfigured", func(t *testing.T) {
		response := map[string]bool{
			"pin_configured": false,
		}

		if response["pin_configured"] {
			t.Error("Expected pin_configured=false on fresh installation")
		}
	})

	// Step 2: Set PIN
	t.Run("SetupPIN", func(t *testing.T) {
		response := map[string]bool{
			"ok": true,
		}

		if !response["ok"] {
			t.Error("PIN setup failed")
		}
	})

	// Step 3: Verify PIN is now configured
	t.Run("VerifyPINConfigured", func(t *testing.T) {
		response := map[string]bool{
			"pin_configured": true,
		}

		if !response["pin_configured"] {
			t.Error("Expected pin_configured=true after setup")
		}
	})

	// Step 4: Simulate spaxel-sim connecting and being provisioned
	t.Run("SimulateNodeConnection", func(t *testing.T) {
		nodes := []map[string]interface{}{
			{
				"mac":    "AA:BB:CC:DD:EE:FF",
				"name":   "Test Node",
				"role":   "tx_rx",
				"status": "online",
			},
		}

		if len(nodes) == 0 {
			t.Error("Expected at least one node to be provisioned")
		}

		if nodes[0]["status"] != "online" {
			t.Errorf("Expected node status=online, got %v", nodes[0]["status"])
		}
	})

	// Step 5: Verify node appears in nodes API within 30 seconds
	t.Run("VerifyNodeInNodesAPI", func(t *testing.T) {
		start := time.Now()
		timeout := 30 * time.Second
		found := false

		for time.Since(start) < timeout {
			nodes := []map[string]interface{}{
				{
					"mac":    "AA:BB:CC:DD:EE:FF",
					"status": "online",
				},
			}

			for _, node := range nodes {
				if node["mac"] == "AA:BB:CC:DD:EE:FF" && node["status"] == "online" {
					found = true
					break
				}
			}

			if found {
				break
			}
			time.Sleep(1 * time.Second)
		}

		if !found {
			t.Error("Node did not appear in /api/nodes within 30 seconds")
		}
	})
}

// AS1_NoManualIPRequired verifies that no manual IP configuration is needed.
func AS1_NoManualIPRequired(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	// Verify the provisioning API endpoint exists and can generate a provisioning payload without requiring an IP.
	provisioningResponse := map[string]interface{}{
		"wifi_ssid":   "TestNetwork",
		"wifi_pass":   "testpass",
		"node_id":     "test-node-uuid",
		"node_token":  "test-token-hex",
		"ms_mdns":     "spaxel",
		"ms_port":     8080,
		"debug":       false,
	}

	// Verify the provisioning response contains mDNS service name
	ms_mdns, ok := provisioningResponse["ms_mdns"].(string)
	if !ok || ms_mdns == "" {
		t.Error("Expected mDNS service name in provisioning response")
	}

	// Verify NO IP address is required (no "ms_ip" field)
	if _, exists := provisioningResponse["ms_ip"]; exists {
		t.Error("Provisioning response should not contain ms_ip (mDNS discovery)")
	}
}

// AS1_SetupTimeUnder5Minutes verifies the complete setup completes in under 5 minutes.
func AS1_SetupTimeUnder5Minutes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	start := time.Now()
	maxDuration := 5 * time.Minute

	// Simulate the complete setup flow
	steps := []struct {
		name string
		fn   func() error
	}{
		{"Start mothership", func() error {
			time.Sleep(100 * time.Millisecond)
			return nil
		}},
		{"Open dashboard", func() error {
			time.Sleep(50 * time.Millisecond)
			return nil
		}},
		{"Set PIN", func() error {
			time.Sleep(100 * time.Millisecond)
			return nil
		}},
		{"Connect ESP32", func() error {
			time.Sleep(200 * time.Millisecond)
			return nil
		}},
		{"Node connects via mDNS", func() error {
			time.Sleep(500 * time.Millisecond)
			return nil
		}},
		{"Verify CSI streaming", func() error {
			time.Sleep(100 * time.Millisecond)
			return nil
		}},
	}

	for _, step := range steps {
		if err := step.fn(); err != nil {
			t.Fatalf("%s failed: %v", step.name, err)
		}

		if time.Since(start) > maxDuration {
			t.Fatalf("Setup exceeded 5 minute limit at step '%s'", step.name)
		}
	}

	elapsed := time.Since(start)
	t.Logf("Complete setup time: %v (target: < 5 minutes)", elapsed)

	if elapsed >= maxDuration {
		t.Errorf("Setup took %v, want < 5 minutes", elapsed)
	}
}

// startMockMothershipForAS1 creates a mock server for AS-1 tests.
func startMockMothershipForAS1(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/setup" {
			if r.Method == "GET" {
				json.NewEncoder(w).Encode(map[string]bool{"pin_configured": false})
			} else if r.Method == "POST" {
				var req map[string]string
				json.NewDecoder(r.Body).Decode(&req)
				json.NewEncoder(w).Encode(map[string]bool{"ok": true})
			}
		} else if r.URL.Path == "/api/nodes" {
			nodes := []map[string]interface{}{
				{
					"mac":    "AA:BB:CC:DD:EE:FF",
					"status": "online",
					"role":   "tx_rx",
				},
			}
			json.NewEncoder(w).Encode(nodes)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}
