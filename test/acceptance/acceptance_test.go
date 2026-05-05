// Package acceptance provides integration tests for Spaxel acceptance scenarios.
// These tests use the spaxel-sim CLI as a test harness to verify the system
// meets its acceptance criteria.
//
// To run these tests:
//   go test -v ./test/acceptance/...
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
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	defaultMothershipURL = "http://localhost:8080"
	defaultMothershipWS  = "ws://localhost:8080/ws/node"
	healthTimeout        = 30 * time.Second
	apiTimeout           = 10 * time.Second
	nodeOnlineTimeout     = 30 * time.Second
	simStartupTimeout     = 20 * time.Second
)

// TestMain runs all acceptance tests in sequence.
func TestMain(m *testing.M) {
	// Check if integration test mode is enabled
	if os.Getenv("SPAXEL_INTEGRATION_TEST") != "1" && os.Getenv("ACCEPTANCE_TEST") != "1" {
		// Skip tests by default unless explicitly enabled
		fmt.Println("Skipping acceptance tests (set SPAXEL_INTEGRATION_TEST=1 or ACCEPTANCE_TEST=1 to run)")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// TestHarness manages the acceptance test lifecycle.
type TestHarness struct {
	MothershipCmd *exec.Cmd
	SimulatorCmd  *exec.Cmd
	WebhookServer *http.Server
	MothershipURL string
	APIURL        string
	DataDir       string
	t             *testing.T
	stderrBuf     *bytes.Buffer
	webhookCalled bool
	webhookMu     sync.Mutex
}

// NewTestHarness creates a new test harness.
func NewTestHarness(t *testing.T) *TestHarness {
	dataDir, err := os.MkdirTemp("", "spaxel-acceptance-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	return &TestHarness{
		MothershipURL: defaultMothershipURL,
		APIURL:        defaultMothershipURL,
		DataDir:       dataDir,
		t:             t,
		stderrBuf:     &bytes.Buffer{},
	}
}

// Start starts the mothership process.
func (h *TestHarness) Start(ctx context.Context) error {
	// Build mothership if needed
	mothershipBin := "/tmp/spaxel-mothership-acceptance"
	if _, err := os.Stat(mothershipBin); os.IsNotExist(err) {
		goCmd := findGoCmd()
		buildCmd := exec.CommandContext(ctx, goCmd, "build", "-o", mothershipBin, "./mothership/cmd/mothership")
		buildCmd.Dir = repoRoot()
		if output, err := buildCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to build mothership: %w: %s", err, string(output))
		}
	}

	// Start mothership
	h.MothershipCmd = exec.CommandContext(ctx, mothershipBin)
	h.MothershipCmd.Env = append(os.Environ(),
		"SPAXEL_BIND_ADDR=127.0.0.1:8080",
		"SPAXEL_DATA_DIR="+h.DataDir,
		"SPAXEL_LOG_LEVEL=info",
		"TZ=UTC",
	)
	h.MothershipCmd.Stdout = io.Discard
	h.MothershipCmd.Stderr = io.MultiWriter(os.Stderr, h.stderrBuf)

	if err := h.MothershipCmd.Start(); err != nil {
		return fmt.Errorf("failed to start mothership: %w", err)
	}

	h.t.Logf("Mothership started (PID: %d, DataDir: %s)", h.MothershipCmd.Process.Pid, h.DataDir)

	// Wait for health check
	if err := h.WaitForHealth(ctx); err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	return nil
}

// Stop stops all processes.
func (h *TestHarness) Stop() {
	if h.MothershipCmd != nil && h.MothershipCmd.Process != nil {
		h.MothershipCmd.Process.Signal(os.Interrupt)
		h.MothershipCmd.Wait()
	}
	if h.SimulatorCmd != nil && h.SimulatorCmd.Process != nil {
		h.SimulatorCmd.Process.Kill()
		h.SimulatorCmd.Wait()
	}
	if h.WebhookServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		h.WebhookServer.Shutdown(ctx)
	}
	// Clean up data directory
	if h.DataDir != "" {
		os.RemoveAll(h.DataDir)
	}
}

// WaitForHealth waits for the /healthz endpoint to return ok.
func (h *TestHarness) WaitForHealth(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			req, _ := http.NewRequestWithContext(ctx, "GET", h.APIURL+"/healthz", nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				continue
			}

			var health map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
				resp.Body.Close()
				continue
			}
			resp.Body.Close()

			if health["status"] == "ok" {
				h.t.Logf("Mothership healthy")
				return nil
			}
		}
	}
}

// StartWebhookServer starts a webhook server for receiving alerts.
func (h *TestHarness) StartWebhookServer() string {
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		h.webhookMu.Lock()
		h.webhookCalled = true
		h.webhookMu.Unlock()

		var payload map[string]interface{}
		json.NewDecoder(r.Body).Decode(&payload)
		h.t.Logf("Webhook received: %+v", payload)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})

	// Find an available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		h.t.Fatalf("Failed to start webhook server: %v", err)
	}

	h.WebhookServer = &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		h.WebhookServer.Serve(listener)
	}()

	webhookURL := fmt.Sprintf("http://127.0.0.1:%d/webhook", listener.Addr().(*net.TCPAddr).Port)
	h.t.Logf("Webhook server started on %s", webhookURL)

	return webhookURL
}

// WebhookCalled returns true if the webhook was called.
func (h *TestHarness) WebhookCalled() bool {
	h.webhookMu.Lock()
	defer h.webhookMu.Unlock()
	return h.webhookCalled
}

// RunSimulator starts the spaxel-sim simulator.
func (h *TestHarness) RunSimulator(ctx context.Context, args []string) error {
	// Build simulator if needed
	simBin := "/tmp/spaxel-sim-acceptance"
	if _, err := os.Stat(simBin); os.IsNotExist(err) {
		goCmd := findGoCmd()
		buildCmd := exec.CommandContext(ctx, goCmd, "build", "-o", simBin, "./mothership/cmd/sim")
		buildCmd.Dir = repoRoot()
		if output, err := buildCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to build simulator: %w: %s", err, string(output))
		}
	}

	// Build default args
	defaultArgs := []string{"--mothership", defaultMothershipWS}
	allArgs := append(defaultArgs, args...)

	h.SimulatorCmd = exec.CommandContext(ctx, simBin, allArgs...)
	h.SimulatorCmd.Stdout = io.MultiWriter(os.Stderr, h.stderrBuf)
	h.SimulatorCmd.Stderr = io.MultiWriter(os.Stderr, h.stderrBuf)

	if err := h.SimulatorCmd.Start(); err != nil {
		return fmt.Errorf("failed to start simulator: %w", err)
	}

	h.t.Logf("Simulator started with args: %v", allArgs)
	return nil
}

// GetNodes fetches the list of nodes from /api/nodes.
func (h *TestHarness) GetNodes(ctx context.Context) ([]map[string]interface{}, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", h.APIURL+"/api/nodes", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var nodes []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&nodes); err != nil {
		return nil, err
	}

	return nodes, nil
}

// GetEvents fetches events from /api/events.
func (h *TestHarness) GetEvents(ctx context.Context, eventType string, limit int) ([]map[string]interface{}, error) {
	url := h.APIURL + "/api/events"
	params := []string{}
	if eventType != "" {
		params = append(params, "type="+eventType)
	}
	if limit > 0 {
		params = append(params, fmt.Sprintf("limit=%d", limit))
	}
	if len(params) > 0 {
		url += "?" + strings.Join(params, "&")
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	events, _ := result["events"].([]map[string]interface{})
	return events, nil
}

// GetBlobs fetches current blobs from /api/blobs.
func (h *TestHarness) GetBlobs(ctx context.Context) ([]map[string]interface{}, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", h.APIURL+"/api/blobs", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	blobs, _ := result["blobs"].([]map[string]interface{})
	return blobs, nil
}

// SetPIN sets the dashboard PIN via /api/auth/setup.
func (h *TestHarness) SetPIN(ctx context.Context, pin string) error {
	body := []byte(fmt.Sprintf(`{"pin":"%s"}`, pin))
	req, _ := http.NewRequestWithContext(ctx, "POST", h.APIURL+"/api/auth/setup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("PIN setup returned status %d", resp.StatusCode)
	}

	return nil
}

// CheckPINConfigured checks if PIN is configured via /api/auth/status.
func (h *TestHarness) CheckPINConfigured(ctx context.Context) (bool, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", h.APIURL+"/api/auth/status", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}

	configured, _ := result["pin_configured"].(bool)
	return configured, nil
}

// RegisterBLEDevice registers a BLE device via /api/ble/devices.
func (h *TestHarness) RegisterBLEDevice(ctx context.Context, device map[string]interface{}) error {
	body, _ := json.Marshal(device)
	req, _ := http.NewRequestWithContext(ctx, "POST", h.APIURL+"/api/ble/devices", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("BLE registration returned status %d", resp.StatusCode)
	}

	return nil
}

// CreateReplaySession creates a replay session via /api/replay/start.
func (h *TestHarness) CreateReplaySession(ctx context.Context, fromMS, toMS int64) (map[string]interface{}, error) {
	body := map[string]interface{}{
		"from_iso8601": time.UnixMilli(fromMS).UTC().Format("2006-01-02T15:04:05Z"),
		"to_iso8601":   time.UnixMilli(toMS).UTC().Format("2006-01-02T15:04:05Z"),
	}
	bodyBytes, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", h.APIURL+"/api/replay/start", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Replay start returned status %d", resp.StatusCode)
	}

	var session map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, err
	}

	return session, nil
}

// StopReplaySession stops a replay session.
func (h *TestHarness) StopReplaySession(ctx context.Context, sessionID string) error {
	stopReq := map[string]interface{}{
		"session_id": sessionID,
	}
	body, _ := json.Marshal(stopReq)
	url := h.APIURL + "/api/replay/stop"
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Replay stop returned status %d", resp.StatusCode)
	}

	return nil
}

// SeekReplaySession seeks to a specific time in a replay session.
func (h *TestHarness) SeekReplaySession(ctx context.Context, sessionID string, timestampMS int64) error {
	seekReq := map[string]interface{}{
		"session_id":         sessionID,
		"timestamp_iso8601": time.UnixMilli(timestampMS).UTC().Format("2006-01-02T15:04:05Z"),
	}
	body, _ := json.Marshal(seekReq)
	url := h.APIURL + "/api/replay/seek"
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Replay seek returned status %d", resp.StatusCode)
	}

	return nil
}

// WaitForNode waits for a node to appear in /api/nodes.
func (h *TestHarness) WaitForNode(ctx context.Context, mac string) (map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(ctx, nodeOnlineTimeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			nodes, err := h.GetNodes(ctx)
			if err != nil {
				continue
			}

			for _, node := range nodes {
				if mac == "" || node["mac"] == mac {
					if status, ok := node["status"].(string); ok && status == "online" {
						return node, nil
					}
				}
			}
		}
	}
}

// WaitForEvent waits for a specific event type to appear.
func (h *TestHarness) WaitForEvent(ctx context.Context, eventType string, timeout time.Duration) (map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			events, err := h.GetEvents(ctx, eventType, 1)
			if err != nil {
				continue
			}

			if len(events) > 0 {
				return events[0], nil
			}
		}
	}
}

// Helper functions

func findGoCmd() string {
	if goroot := os.Getenv("GOROOT"); goroot != "" {
		candidate := filepath.Join(goroot, "bin", "go")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidate := filepath.Join(home, ".local", "go", "bin", "go")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "go"
}

func repoRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	// test/acceptance → go up two levels
	return filepath.Join(wd, "..", "..")
}
