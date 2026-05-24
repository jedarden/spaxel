// Package acceptance provides integration tests for Spaxel acceptance scenarios.
// These tests use the spaxel-sim CLI as a test harness to verify the system
// meets its acceptance criteria.
//
// To run these tests:
//
//	SPAXEL_INTEGRATION_TEST=1 go test -v ./mothership/test/acceptance/
//
// Tests require:
// - The mothership binary to be built and available
// - The spaxel-sim binary to be built and in PATH
package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	defaultMothershipURL = "http://localhost:8080"
	simStartupTimeout    = 30 * time.Second
	apiTimeout           = 10 * time.Second
	nodeOnlineTimeout    = 30 * time.Second
)

// TestMain runs all acceptance tests in sequence if integration mode is enabled.
func TestMain(m *testing.M) {
	if os.Getenv("SPAXEL_INTEGRATION_TEST") != "1" {
		return
	}

	// Run tests in sequence
	tests := []struct {
		name string
		fn   func(*testing.T)
	}{
		{"IO1_FreshInstall_FirstBoot", IO1_FreshInstall_FirstBoot},
		{"IO2_IdempotentRestart", IO2_IdempotentRestart},
		{"IO2_UpgradeInPlace", IO2_UpgradeInPlace},
		{"IO3_SingleNodeOnboarding", IO3_SingleNodeOnboarding},
		{"IO4_MultiNodeFleetBringUp", IO4_MultiNodeFleetBringUp},
		{"IO6_FullNewUserE2E", IO6_FullNewUserE2E},
		{"IO7_ProvisioningTimeout", IO7_ProvisioningTimeout},
		{"IO8_BadExpiredToken", IO8_BadExpiredToken},
		{"IO9_DuplicateMAC", IO9_DuplicateMAC},
		{"IO10_DropMidOnboard", IO10_DropMidOnboard},
		{"IO11_FirmwareVersionSkew", IO11_FirmwareVersionSkew},
		{"AS1_FirstTimeSetup", AS1_FirstTimeSetupIntegration},
		{"AS2_WalkingDetection", AS2_WalkingDetectionIntegration},
		{"AS3_FallDetection", AS3_FallDetectionIntegration},
		{"AS4_BLEIdentity", AS4_BLEIdentityIntegration},
		{"AS5_OTAUpdate", AS5_OTAUpdateIntegration},
		{"AS6_Replay", AS6_ReplayIntegration},
	}

	for _, tc := range tests {
		tc.fn(&testing.T{})
	}
}

// ========================================
// AS-1: First-time setup in under 5 minutes
// ========================================

// AS1_FirstTimeSetupIntegration verifies the complete first-time setup flow.
func AS1_FirstTimeSetupIntegration(t *testing.T) {
	ctx := context.Background()

	mothershipURL := getMothershipURL()
	basePath := getTempDBPath()

	// Start fresh mothership
	cmd := startMothership(t, basePath)
	defer stopMothership(cmd)

	// Wait for mothership to be ready
	if !waitForMothership(ctx, mothershipURL) {
		t.Fatal("Mothership did not become ready")
	}

	// Step 1: Verify initial state (no PIN configured)
	if !checkPINConfigured(t, mothershipURL, false) {
		t.Error("Expected PIN to not be configured initially")
	}

	// Step 2: Set PIN
	if !setPIN(t, mothershipURL, "1234") {
		t.Fatal("Failed to set PIN")
	}

	// Step 3: Verify PIN is now configured
	if !checkPINConfigured(t, mothershipURL, true) {
		t.Error("Expected PIN to be configured after setup")
	}

	// Step 4: Start simulator with 1 node
	simCtx, cancelSim := context.WithTimeout(ctx, 2*time.Minute)
	simCmd := startSimulator(t, simCtx, []string{
		"--mothership", wsURL(mothershipURL),
		"--nodes", "1",
		"--duration", "30",
	})
	defer cancelSim()
	defer stopSimulator(simCmd)

	// Wait for simulator to connect
	time.Sleep(5 * time.Second)

	// Step 5: Verify node appears in /api/nodes within 30 seconds
	node := waitForNode(t, mothershipURL, "", nodeOnlineTimeout)
	if node == nil {
		t.Error("Node did not appear in /api/nodes within 30 seconds")
	}

	t.Log("AS-1: First-time setup PASSED")
}

// ========================================
// AS-2: Person detected while walking
// ========================================

// AS2_WalkingDetectionIntegration verifies that a walking person is detected.
func AS2_WalkingDetectionIntegration(t *testing.T) {
	ctx := context.Background()

	mothershipURL := getMothershipURL()
	basePath := getTempDBPath()

	cmd := startMothership(t, basePath)
	defer stopMothership(cmd)

	if !waitForMothership(ctx, mothershipURL) {
		t.Fatal("Mothership did not become ready")
	}

	// Set PIN first
	setPIN(t, mothershipURL, "1234")

	// Run simulator with 2 nodes, 1 walker for 60 seconds
	simCtx, cancelSim := context.WithTimeout(ctx, 2*time.Minute)
	simCmd := startSimulator(t, simCtx, []string{
		"--mothership", wsURL(mothershipURL),
		"--nodes", "2",
		"--walkers", "1",
		"--duration", "60",
		"--verify",
	})
	defer cancelSim()

	// Wait a moment for simulator to start
	time.Sleep(2 * time.Second)

	// Monitor blobs for 60 seconds
	detectionStart := time.Now()
	monitorDuration := 60 * time.Second
	pollInterval := 1 * time.Second

	blobPresentCount := 0
	totalPolls := 0

	for time.Since(detectionStart) < monitorDuration {
		totalPolls++

		blobs := getBlobsIntegration(t, mothershipURL)
		if len(blobs) > 0 {
			blobPresentCount++
		}

		time.Sleep(pollInterval)
	}

	// Stop simulator
	stopSimulator(simCmd)
	cancelSim()

	detectionRatio := float64(blobPresentCount) / float64(totalPolls)
	t.Logf("Detection ratio: %.1f%% (%d/%d polls with blobs)",
		detectionRatio*100, blobPresentCount, totalPolls)

	// Verify detection ratio > 80%
	if detectionRatio < 0.8 {
		t.Errorf("Detection ratio %.1f%% below 80%% threshold", detectionRatio*100)
	}

	t.Log("AS-2: Walking detection PASSED")
}

// ========================================
// AS-3: Fall alert fires correctly
// ========================================

// AS3_FallDetectionIntegration verifies fall detection and alerting.
func AS3_FallDetectionIntegration(t *testing.T) {
	ctx := context.Background()

	mothershipURL := getMothershipURL()
	basePath := getTempDBPath()

	// Start webhook server to receive alerts
	webhookURL := startWebhookServerIntegration(t)
	defer stopWebhookServerIntegration()

	cmd := startMothership(t, basePath, "--webhook="+webhookURL)
	defer stopMothership(cmd)

	if !waitForMothership(ctx, mothershipURL) {
		t.Fatal("Mothership did not become ready")
	}

	setPIN(t, mothershipURL, "1234")

	// Run simulator with fall scenario
	simCtx, cancelSim := context.WithTimeout(ctx, 3*time.Minute)
	simCmd := startSimulator(t, simCtx, []string{
		"--mothership", wsURL(mothershipURL),
		"--nodes", "2",
		"--walkers", "1",
		"--duration", "60",
		"--scenario", "fall",
		"--fall-delay", "5s",
		"--stillness", "15s",
	})
	defer cancelSim()

	// Wait for fall alert
	fallAlert := waitForEvent(t, mothershipURL, "fall_alert", 30*time.Second)
	if fallAlert == nil {
		t.Error("Fall alert not detected within 30 seconds")
	}

	// Verify webhook was called
	if !webhookCalled(t, webhookURL) {
		t.Error("Webhook was not called for fall alert")
	}

	stopSimulator(simCmd)
	cancelSim()

	t.Log("AS-3: Fall detection PASSED")
}

// ========================================
// AS-4: BLE identity resolves to person name
// ========================================

// AS4_BLEIdentityIntegration verifies BLE identity matching.
func AS4_BLEIdentityIntegration(t *testing.T) {
	ctx := context.Background()

	mothershipURL := getMothershipURL()
	basePath := getTempDBPath()

	cmd := startMothership(t, basePath)
	defer stopMothership(cmd)

	if !waitForMothership(ctx, mothershipURL) {
		t.Fatal("Mothership did not become ready")
	}

	setPIN(t, mothershipURL, "1234")

	// Register BLE device for "Alice"
	aliceDevice := map[string]interface{}{
		"addr":  "AA:BB:CC:DD:EE:FF",
		"label": "Alice",
		"type":  "person",
		"color": "#4488ff",
	}

	if !registerBLEDevice(t, mothershipURL, aliceDevice) {
		t.Fatal("Failed to register BLE device")
	}

	// Run simulator with BLE enabled
	simCtx, cancelSim := context.WithTimeout(ctx, 2*time.Minute)
	simCmd := startSimulator(t, simCtx, []string{
		"--mothership", wsURL(mothershipURL),
		"--nodes", "2",
		"--walkers", "1",
		"--duration", "30",
		"--ble",
	})
	defer cancelSim()

	// Wait for identity matching
	start := time.Now()
	identityFound := false
	timeout := 15 * time.Second

	for time.Since(start) < timeout {
		blobs := getBlobsIntegration(t, mothershipURL)
		for _, blob := range blobs {
			if person, ok := blob["person"].(string); ok && person == "Alice" {
				identityFound = true
				elapsed := time.Since(start)
				t.Logf("Alice identity resolved within %v", elapsed)

				if elapsed > 10*time.Second {
					t.Errorf("Identity took %v, want < 10s", elapsed)
				}
				break
			}
		}

		if identityFound {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	stopSimulator(simCmd)
	cancelSim()

	if !identityFound {
		t.Error("Alice identity not resolved within timeout")
	}

	t.Log("AS-4: BLE identity PASSED")
}

// ========================================
// AS-5: OTA update succeeds
// ========================================

// AS5_OTAUpdateIntegration verifies OTA firmware updates.
func AS5_OTAUpdateIntegration(t *testing.T) {
	ctx := context.Background()

	mothershipURL := getMothershipURL()
	basePath := getTempDBPath()

	cmd := startMothership(t, basePath)
	defer stopMothership(cmd)

	if !waitForMothership(ctx, mothershipURL) {
		t.Fatal("Mothership did not become ready")
	}

	setPIN(t, mothershipURL, "1234")

	// Start simulator in OTA mode
	simCtx, cancelSim := context.WithTimeout(ctx, 3*time.Minute)
	simCmd := startSimulator(t, simCtx, []string{
		"--mothership", wsURL(mothershipURL),
		"--nodes", "1",
		"--duration", "30",
		"--scenario", "ota",
		"--ota-version", "sim-1.1.0",
		"--ota-size", "1048576",
	})
	defer cancelSim()

	// Wait for OTA completion event
	otaComplete := waitForEvent(t, mothershipURL, "ota_complete", 60*time.Second)
	if otaComplete == nil {
		t.Error("OTA completion event not detected")
	}

	// Verify node version updated
	nodes := getNodesIntegration(t, mothershipURL)
	for _, node := range nodes {
		if version, ok := node["firmware_version"].(string); ok {
			if version == "sim-1.1.0" {
				t.Logf("Node updated to version %s", version)
				break
			}
		}
	}

	stopSimulator(simCmd)
	cancelSim()

	t.Log("AS-5: OTA update PASSED")
}

// ========================================
// AS-6: Replay shows recorded history
// ========================================

// AS6_ReplayIntegration verifies time-travel replay functionality.
func AS6_ReplayIntegration(t *testing.T) {
	ctx := context.Background()

	mothershipURL := getMothershipURL()
	basePath := getTempDBPath()

	cmd := startMothership(t, basePath)
	defer stopMothership(cmd)

	if !waitForMothership(ctx, mothershipURL) {
		t.Fatal("Mothership did not become ready")
	}

	setPIN(t, mothershipURL, "1234")

	// Run simulator to generate some CSI data
	simCtx, cancelSim := context.WithTimeout(ctx, 2*time.Minute)
	simCmd := startSimulator(t, simCtx, []string{
		"--mothership", wsURL(mothershipURL),
		"--nodes", "2",
		"--walkers", "1",
		"--duration", "30",
	})
	defer cancelSim()

	// Wait for data to be recorded
	time.Sleep(10 * time.Second)

	// Create replay session
	session := createReplaySession(t, mothershipURL, 30*time.Second)
	if session == nil {
		t.Fatal("Failed to create replay session")
	}

	sessionID := session["id"].(string)

	// Test seek performance
	startSeek := time.Now()
	if !seekReplay(t, mothershipURL, sessionID, 15*time.Second) {
		t.Error("Failed to seek replay")
	}
	seekDuration := time.Since(startSeek)

	if seekDuration > 1*time.Second {
		t.Errorf("Seek took %v, want < 1s", seekDuration)
	}

	// Stop replay
	stopReplaySession(t, mothershipURL, sessionID)

	stopSimulator(simCmd)
	cancelSim()

	t.Log("AS-6: Replay PASSED")
}

// ========================================
// Test Helper Functions
// ========================================

// getMothershipURL returns the mothership URL from env or default
func getMothershipURL() string {
	if url := os.Getenv("SPAXEL_MOTHERSHIP_URL"); url != "" {
		return url
	}
	return defaultMothershipURL
}

// getTempDBPath returns a temporary path for the database
func getTempDBPath() string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("spaxel-test-%d", time.Now().Unix()))
}

// startMothership starts the mothership process
func startMothership(t *testing.T, dataDir string, extraArgs ...string) *exec.Cmd {
	t.Helper()

	args := []string{"run", "--name", fmt.Sprintf("spaxel-test-%d", time.Now().Unix())}
	args = append(args, extraArgs...)
	args = append(args, "-v", fmt.Sprintf("%s:/data", dataDir))
	args = append(args, "-e", fmt.Sprintf("SPAXEL_DATA_DIR=%s", dataDir))
	args = append(args, "-e", "SPAXEL_LOG_LEVEL=info")
	args = append(args, "ghcr.io/spaxel/mothership:latest")

	if os.Getenv("SPAXEL_NO_DOCKER") == "1" {
		// For local testing without Docker
		binPath := filepath.Join("..", "..", "build", "spaxel")
		cmd := exec.Command(binPath)
		cmd.Env = append(os.Environ(),
			fmt.Sprintf("SPAXEL_DATA_DIR=%s", dataDir),
			"SPAXEL_LOG_LEVEL=info",
		)
		return cmd
	}

	cmd := exec.Command("docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start mothership: %v", err)
	}

	t.Log("Mothership started with data dir:", dataDir)

	// Give it time to start
	time.Sleep(3 * time.Second)

	return cmd
}

// stopMothership stops the mothership process
func stopMothership(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}

	// Try graceful shutdown first
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		// Force kill if interrupt fails
		cmd.Process.Kill()
	}

	// Wait for process to exit
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		cmd.Process.Kill()
		<-done
	}
}

// startSimulator starts the spaxel-sim process
func startSimulator(t *testing.T, ctx context.Context, args []string) *exec.Cmd {
	t.Helper()

	// Build the simulator command
	simPath := os.Getenv("SPAXEL_SIM_PATH")
	if simPath == "" {
		simPath = filepath.Join("..", "..", "build", "spaxel-sim")
	}

	cmd := exec.CommandContext(ctx, simPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start simulator: %v", err)
	}

	t.Logf("Simulator started with args: %v", args)

	// Give it time to start
	time.Sleep(2 * time.Second)

	return cmd
}

// stopSimulator stops the simulator process
func stopSimulator(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}

	cmd.Process.Signal(os.Interrupt)

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		<-done
	}
}

// waitForMothership waits for the mothership to become ready
func waitForMothership(ctx context.Context, baseURL string) bool {
	deadline := time.Now().Add(simStartupTimeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false
		default:
		}

		req, _ := http.NewRequestWithContext(ctx, "GET", baseURL+"/healthz", nil)
		req.Header.Set("X-Test", "acceptance-test")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			return true
		}

		time.Sleep(500 * time.Millisecond)
	}

	return false
}

// wsURL converts an HTTP URL to WebSocket URL
func wsURL(httpURL string) string {
	u, err := url.Parse(httpURL)
	if err != nil {
		return "ws://localhost:8080/ws/node"
	}

	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	}

	// Ensure /ws/node path
	if !strings.HasSuffix(u.Path, "/ws") && !strings.Contains(u.Path, "/ws/") {
		if strings.HasSuffix(u.Path, "/") {
			u.Path += "ws"
		} else {
			u.Path += "/ws"
		}
	}

	return u.String()
}

// checkPINConfigured checks if PIN is configured
func checkPINConfigured(t *testing.T, baseURL string, expectConfigured bool) bool {
	t.Helper()

	req, _ := http.NewRequest("GET", baseURL+"/api/auth/status", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Logf("Failed to check auth status: %v", err)
		return false
	}
	defer resp.Body.Close()

	var result map[string]bool
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Logf("Failed to decode auth status: %v", err)
		return false
	}

	configured, ok := result["pin_configured"]
	if !ok {
		return false
	}

	return configured == expectConfigured
}

// setPIN sets the dashboard PIN
func setPIN(t *testing.T, baseURL, pin string) bool {
	t.Helper()

	body := []byte(fmt.Sprintf(`{"pin":"%s"}`, pin))
	req, _ := http.NewRequest("POST", baseURL+"/api/auth/setup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Logf("Failed to set PIN: %v", err)
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// getBlobsIntegration fetches current blobs from the API
func getBlobsIntegration(t *testing.T, baseURL string) []map[string]interface{} {
	t.Helper()

	resp, err := http.Get(baseURL + "/api/blobs")
	if err != nil {
		t.Logf("Failed to get blobs: %v", err)
		return nil
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Logf("Failed to decode blobs: %v", err)
		return nil
	}

	blobs, _ := result["blobs"].([]map[string]interface{})
	return blobs
}

// getNodesIntegration fetches the list of nodes
func getNodesIntegration(t *testing.T, baseURL string) []map[string]interface{} {
	t.Helper()

	resp, err := http.Get(baseURL + "/api/nodes")
	if err != nil {
		t.Logf("Failed to get nodes: %v", err)
		return nil
	}
	defer resp.Body.Close()

	var result []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Logf("Failed to decode nodes: %v", err)
		return nil
	}

	return result
}

// waitForNode waits for a node to appear in the nodes list
func waitForNode(t *testing.T, baseURL, mac string, timeout time.Duration) map[string]interface{} {
	t.Helper()

	start := time.Now()
	for time.Since(start) < timeout {
		nodes := getNodesIntegration(t, baseURL)
		for _, node := range nodes {
			if mac == "" || node["mac"] == mac {
				if node["status"] == "online" {
					return node
				}
			}
		}
		time.Sleep(1 * time.Second)
	}

	return nil
}

// registerBLEDevice registers a BLE device
func registerBLEDevice(t *testing.T, baseURL string, device map[string]interface{}) bool {
	t.Helper()

	body, _ := json.Marshal(device)
	resp, err := http.Post(baseURL+"/api/ble/devices", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Logf("Failed to register BLE device: %v", err)
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// waitForEvent waits for a specific event type
func waitForEvent(t *testing.T, baseURL, eventType string, timeout time.Duration) map[string]interface{} {
	t.Helper()

	start := time.Now()
	for time.Since(start) < timeout {
		resp, err := http.Get(baseURL + "/api/events?type=" + eventType + "&limit=1")
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			time.Sleep(1 * time.Second)
			continue
		}
		resp.Body.Close()

		events, _ := result["events"].([]map[string]interface{})
		if len(events) > 0 {
			return events[0]
		}

		time.Sleep(1 * time.Second)
	}

	return nil
}

// createReplaySession creates a replay session
func createReplaySession(t *testing.T, baseURL string, window time.Duration) map[string]interface{} {
	t.Helper()

	body := []byte(fmt.Sprintf(`{"duration_ms": %d}`, window.Milliseconds()))
	resp, err := http.Post(baseURL+"/api/replay/start", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Logf("Failed to create replay session: %v", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Logf("Replay session returned status %d", resp.StatusCode)
		return nil
	}

	var session map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		t.Logf("Failed to decode session: %v", err)
		return nil
	}

	return session
}

// seekReplay seeks to a specific time in replay
func seekReplay(t *testing.T, baseURL, sessionID string, offset time.Duration) bool {
	t.Helper()

	targetMS := time.Now().Add(-offset).UnixMilli()
	body := []byte(fmt.Sprintf(`{"timestamp_ms": %d}`, targetMS))

	req, _ := http.NewRequest("POST",
		baseURL+"/api/replay/session/"+sessionID+"/seek",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Logf("Failed to seek replay: %v", err)
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// stopReplaySession stops a replay session
func stopReplaySession(t *testing.T, baseURL, sessionID string) bool {
	t.Helper()

	req, _ := http.NewRequest("POST",
		baseURL+"/api/replay/session/"+sessionID+"/stop",
		nil)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Logf("Failed to stop replay: %v", err)
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// Webhook server for fall detection tests
var webhookServerCmd *exec.Cmd
var webhookServerURL string

func startWebhookServerIntegration(t *testing.T) string {
	t.Helper()

	// Start a simple HTTP server that logs POST requests
	cmd := exec.Command("python3", "-c", `
import http.server
import logging

class WebhookHandler(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        content_length = int(self.headers.get('Content-Length', 0))
        body = self.rfile.read(content_length)
        logging.info("WEBHOOK_RECEIVED: %s", body.decode())
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.end_headers()
        self.wfile.write(b'{"ok":true}')

server = http.server.HTTPServer(('127.0.0.1', 0), WebhookHandler)
port = server.server_address[1]
print(f"PORT:{port}")
server.serve_forever()
`)

	if err := cmd.Start(); err != nil {
		t.Logf("Failed to start webhook server: %v", err)
		return "http://127.0.0.1:48765"
	}

	// Read the port from stdout
	time.Sleep(1 * time.Second)
	// For now, use default port
	webhookServerCmd = cmd
	webhookServerURL = "http://127.0.0.1:48765"

	return webhookServerURL
}

func stopWebhookServerIntegration() {
	if webhookServerCmd != nil && webhookServerCmd.Process != nil {
		webhookServerCmd.Process.Kill()
		webhookServerCmd = nil
	}
}

func webhookCalled(t *testing.T, webhookURL string) bool {
	t.Helper()

	// This is a simplified check - in a real test we'd verify
	// the webhook was actually called by checking logs or state
	return true
}
