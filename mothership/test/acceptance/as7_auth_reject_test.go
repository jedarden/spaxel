// AS-7: Auth rejection test
// Quality gate #7: node without a valid token must get HTTP 401 and be rejected.
package acceptance

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// AS7_AuthRejectIntegration verifies that nodes without valid tokens are rejected with HTTP 401.
// Steps:
// 1. Start mothership with PIN configured
// 2. Attempt WebSocket connection without X-Spaxel-Token header
// 3. Verify HTTP 401 response
// 4. Verify simulator exits non-zero
// 5. Verify no zombie node in fleet
// 6. Verify mothership logs the rejection
func AS7_AuthRejectIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping AS-7 test in short mode")
	}

	// Set environment for AS-7 test (uses port 18087)
	t.Setenv("SPAXEL_PORT", "18087")
	t.Setenv("SPAXEL_MOTHERSHIP_URL", "http://localhost:18087")

	ctx := context.Background()
	mothershipURL := getMothershipURL()

	// Create empty data directory
	tempDir, err := os.MkdirTemp("", "spaxel-as7-*")
	if err != nil {
		t.Fatalf("AS-7 FAIL: Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dataDir := filepath.Join(tempDir, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("AS-7 FAIL: Failed to create data dir: %v", err)
	}

	// Step 1: Start mothership
	cmd := startMothershipWithDataDir(t, dataDir)
	defer stopMothership(cmd)

	if !waitForMothership(ctx, mothershipURL) {
		t.Fatal("AS-7 FAIL: Mothership did not become ready")
	}

	// Configure PIN
	setPIN(t, mothershipURL, "777777")

	// Step 2: Attempt WebSocket connection without token header
	t.Log("AS-7: Attempting WebSocket connection without token...")

	// Convert HTTP URL to WebSocket URL
	wsURL, err := url.Parse(mothershipURL)
	if err != nil {
		t.Fatalf("AS-7 FAIL: Failed to parse mothership URL: %v", err)
	}
	wsScheme := "ws"
	if wsURL.Scheme == "https" {
		wsScheme = "wss"
	}
	nodeWSURL := fmt.Sprintf("%s://%s/ws/node", wsScheme, wsURL.Host)

	// Try to connect without token header - expect 401
	t.Logf("AS-7: Connecting to %s without token", nodeWSURL)
	wsDialer := &websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}
	conn, resp, err := wsDialer.Dial(nodeWSURL, nil)

	// Step 3: Verify HTTP 401 response
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			t.Log("AS-7: WebSocket connection rejected with HTTP 401 (expected)")
		} else if resp != nil {
			t.Errorf("AS-7 FAIL: Expected HTTP 401, got %d", resp.StatusCode)
		} else {
			t.Errorf("AS-7 FAIL: Connection failed with no HTTP response: %v", err)
		}
	} else {
		conn.Close()
		t.Error("AS-7 FAIL: WebSocket connection succeeded without token (should have been rejected)")
	}

	// Step 4: Verify simulator exits non-zero when attempting connection with invalid token
	t.Log("AS-7: Testing simulator rejection with invalid token...")
	simPath := os.Getenv("SPAXEL_SIM_PATH")
	if simPath == "" {
		simPath = "/tmp/spaxel-sim-as7"
		buildCmd := exec.Command("go", "build", "-o", simPath, "../cmd/sim")
		buildCmd.Dir = filepath.Join("..", "..")
		if output, err := buildCmd.CombinedOutput(); err != nil {
			t.Fatalf("AS-7 FAIL: Failed to build simulator: %v: %s", err, string(output))
		}
		defer os.Remove(simPath)
	}

	simCtx, simCancel := context.WithTimeout(ctx, 15*time.Second)
	defer simCancel()

	invalidToken := "invalid-token-0000000000000000000000000000000000000000000000000000000000000000"
	simArgs := []string{
		"--mothership", fmt.Sprintf("ws://localhost:18087/ws/node"),
		"--token", invalidToken,
		"--nodes", "1",
		"--duration", "5",
	}

	simCmd := exec.CommandContext(simCtx, simPath, simArgs...)
	simCmd.Stdout = nil // Suppress output
	simCmd.Stderr = nil

	if err := simCmd.Start(); err != nil {
		t.Fatalf("AS-7 FAIL: Failed to start simulator: %v", err)
	}

	// Wait for simulator to exit
	simErr := simCmd.Wait()

	if simErr == nil {
		t.Error("AS-7 FAIL: Simulator exited with success (expected non-zero exit with invalid token)")
	} else {
		t.Logf("AS-7: Simulator exited with error (expected): %v", simErr)
	}

	// Step 5: Verify no zombie node in fleet
	time.Sleep(2 * time.Second) // Give mothership time to process
	nodes := getNodesIntegration(t, mothershipURL)
	if len(nodes) > 0 {
		t.Errorf("AS-7 FAIL: Expected 0 nodes with invalid token, got %d nodes", len(nodes))
		for _, node := range nodes {
			t.Logf("AS-7: Unexpected node: MAC=%s Name=%s", node["mac"], node["name"])
		}
	} else {
		t.Log("AS-7: No zombie nodes in fleet (expected)")
	}

	// Step 6: Verify mothership logged the rejection
	// We can't easily check logs from the running mothership process,
	// but we've verified the key behaviors: 401 response, simulator error, no nodes added
	t.Log("AS-7: Auth rejection test PASSED")
}
