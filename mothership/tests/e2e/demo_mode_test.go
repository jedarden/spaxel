package e2e

// Integration coverage for SPAXEL_DEMO_MODE (read-only demo dashboard).
//
// Unlike the auth-package unit tests (TestDemoModeMiddleware &
// TestMiddlewareDemoModePINBypass), which drive the middleware with
// httptest.Recorders against an isolated handler, this file boots the real
// mothership binary as a subprocess — the same production wiring as a demo
// deployment: chi middleware stack Logger → Recoverer → DemoModeMiddleware —
// and asserts over HTTP.
//
// Demo mode cannot be reached from a fresh install over HTTP alone: setup is
// itself a POST, and DemoModeMiddleware runs ahead of routing, so with
// SPAXEL_DEMO_MODE=true the very call that would configure a PIN is rejected
// with 403. The test therefore runs two phases against one data directory:
//
//	Phase A (demo off)   — set the PIN, log in with it, prove those POSTs work.
//	Phase B (demo on)    — restart the SAME data dir with SPAXEL_DEMO_MODE=true
//	                       and prove reads stay open, pages render without any
//	                       login, and every mutating verb is 403'd.
//
// (bead spaxel-b8be7ab5)

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
	"testing"
	"time"
)

// demoModePIN satisfies the 4-8 digit validation in handleSetup. Declared as a
// test fixture value, not a credential: the mothership under test is a
// subprocess bound to loopback on a throwaway data directory.
const demoModePIN = "246813"

// demoRejectedBody is the JSON DemoModeMiddleware writes with 403.
type demoRejectedBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

const (
	demoRejectedError   = "demo mode active"
	demoRejectedMessage = "Spaxel is running in demo mode - mutating operations are disabled"
)

// buildDemoBinary compiles the mothership into a per-run path. The fixed
// /tmp/spaxel-mothership-test path the older tests share is avoided on purpose:
// a stale binary there (or a concurrent test rebuilding it mid-run) would
// silently test the wrong code, and two workers sharing this checkout would
// race on the rebuild.
func buildDemoBinary(t *testing.T) string {
	t.Helper()

	bin := filepath.Join(t.TempDir(), "spaxel-mothership-demo")
	cmd := exec.Command(findGoCmd(), "build", "-o", bin, "./cmd/mothership")
	cmd.Dir = moduleRoot()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build mothership for demo-mode test: %v\n%s", err, out)
	}
	return bin
}

// demoServer is a mothership subprocess under test.
type demoServer struct {
	cmd     *exec.Cmd
	stderr  *bytes.Buffer
	baseURL string
}

// startDemoServer launches the mothership against dataDir and waits for
// /healthz. demoCap <= 0 leaves SPAXEL_DEMO_MAX_DASHBOARD_CLIENTS unset so the
// built-in default applies.
func startDemoServer(t *testing.T, bin, dataDir string, demoMode bool, demoCap int) *demoServer {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve an ephemeral port: %v", err)
	}
	bindAddr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatalf("failed to release reserved port %s: %v", bindAddr, err)
	}

	env := append(os.Environ(),
		"SPAXEL_BIND_ADDR="+bindAddr,
		"SPAXEL_DATA_DIR="+dataDir,
		"SPAXEL_LOG_LEVEL=info",
		"TZ=UTC",
		// Keep the auth window strict so nothing in the run depends on the 24h
		// default (same rationale as the harness Start()).
		"SPAXEL_MIGRATION_WINDOW_HOURS=0",
		// Hermetic: no multicast advertisement, and an explicit demo value so an
		// ambient SPAXEL_DEMO_MODE on the host can never leak into a phase.
		"SPAXEL_MDNS_ENABLED=false",
		fmt.Sprintf("SPAXEL_DEMO_MODE=%t", demoMode),
		// The docker build embeds the dashboard behind the `embed` tag; a plain
		// `go build` does not, and main.go then serves it from SPAXEL_STATIC_DIR.
		// Point that at the canonical repo-root dashboard/ so the page assertions
		// below exercise the real files instead of chi's 404 (the default
		// "/dashboard" is a container path that does not exist on the host).
		"SPAXEL_STATIC_DIR="+filepath.Join(moduleRoot(), "..", "dashboard"),
	)
	if demoCap > 0 {
		env = append(env, fmt.Sprintf("SPAXEL_DEMO_MAX_DASHBOARD_CLIENTS=%d", demoCap))
	}

	stderr := &bytes.Buffer{}
	cmd := &exec.Cmd{
		Path: bin,
		Env:  env,
		// The extension-less page routes (/, /fleet, /simple) resolve their files
		// through findDashboardDir()'s CWD candidates ("./dashboard" first), which
		// only work when the process runs from the repo root — the documented
		// development invocation. Run it from there so both serving paths (this
		// one and the SPAXEL_STATIC_DIR fallback for /css, /js) reach the same
		// canonical dashboard tree.
		Dir:    filepath.Join(moduleRoot(), ".."),
		Stdout: io.Discard,
		Stderr: stderr,
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start mothership (demo=%t): %v", demoMode, err)
	}

	srv := &demoServer{cmd: cmd, stderr: stderr, baseURL: "http://" + bindAddr}
	if err := srv.waitHealthy(t); err != nil {
		t.Logf("mothership stderr:\n%s", stderr.String())
		srv.stop()
		t.Fatalf("mothership (demo=%t) never became healthy: %v", demoMode, err)
	}
	return srv
}

func (s *demoServer) waitHealthy(t *testing.T) error {
	t.Helper()

	deadline := time.Now().Add(HealthTimeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(s.baseURL + "/healthz")
		if err == nil {
			var health HealthResponse
			decodeErr := json.NewDecoder(resp.Body).Decode(&health)
			resp.Body.Close() //nolint:errcheck
			if decodeErr == nil && health.Status == "ok" {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("health check did not report ok within %s", HealthTimeout)
}

// stop shuts the subprocess down and reports whether it exited cleanly.
func (s *demoServer) stop() error {
	if s.cmd.Process == nil {
		return nil
	}
	_ = s.cmd.Process.Signal(os.Interrupt)
	return s.cmd.Wait()
}

// demoDo performs one request against the server under test.
func demoDo(t *testing.T, method, url, body string) (int, string, string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		t.Fatalf("build %s %s: %v", method, url, err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("%s %s: reading body: %v", method, url, err)
	}
	return resp.StatusCode, resp.Header.Get("Content-Type"), string(payload)
}

// requireDemoRejection asserts the exact demo-mode 403 contract.
func requireDemoRejection(t *testing.T, method, url, body string) {
	t.Helper()

	status, contentType, payload := demoDo(t, method, url, body)
	if status != http.StatusForbidden {
		t.Fatalf("%s %s: got status %d, want 403 (body: %s)", method, url, status, payload)
	}
	if !strings.Contains(contentType, "application/json") {
		t.Fatalf("%s %s: got Content-Type %q, want application/json", method, url, contentType)
	}
	var rejected demoRejectedBody
	if err := json.Unmarshal([]byte(payload), &rejected); err != nil {
		t.Fatalf("%s %s: body is not the demo rejection JSON: %v (body: %s)", method, url, err, payload)
	}
	if rejected.Error != demoRejectedError || rejected.Message != demoRejectedMessage {
		t.Fatalf("%s %s: unexpected rejection body %+v, want error=%q message=%q",
			method, url, rejected, demoRejectedError, demoRejectedMessage)
	}
}

// requireDemoStatus fetches /api/auth/status and returns its two flags.
func requireDemoStatus(t *testing.T, baseURL string) (demoMode, pinConfigured bool) {
	t.Helper()

	status, contentType, payload := demoDo(t, http.MethodGet, baseURL+"/api/auth/status", "")
	if status != http.StatusOK {
		t.Fatalf("GET /api/auth/status: got status %d, want 200 (body: %s)", status, payload)
	}
	if !strings.Contains(contentType, "application/json") {
		t.Fatalf("GET /api/auth/status: got Content-Type %q, want application/json", contentType)
	}
	var parsed struct {
		DemoMode      bool `json:"demo_mode"`
		PINConfigured bool `json:"pin_configured"`
	}
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		t.Fatalf("GET /api/auth/status: body is not JSON: %v (body: %s)", err, payload)
	}
	return parsed.DemoMode, parsed.PINConfigured
}

func TestDemoModeIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping demo-mode integration test in short mode")
	}

	bin := buildDemoBinary(t)
	dataDir := t.TempDir()

	// ---------------------------------------------------------------------------
	// Phase A — demo OFF: configure the PIN that Phase B will find in the data dir.
	// ---------------------------------------------------------------------------
	t.Run("phase A sets a PIN with demo mode off", func(t *testing.T) {
		srv := startDemoServer(t, bin, dataDir, false, 0)
		defer func() {
			// Stopped before Phase B so the SQLite database is released. The
			// subprocess exits cleanly on SIGINT, so a non-nil error here is worth
			// a log line, not a failure.
			if err := srv.stop(); err != nil {
				t.Logf("phase A mothership exited with %v (expected after SIGINT)", err)
			}
		}()

		// Startup log must reflect the normal cap while demo mode is off.
		if !strings.Contains(srv.stderr.String(), "Dashboard client cap: 10 (demo mode: false)") {
			t.Errorf("startup log missing normal-mode cap line; stderr:\n%s", srv.stderr.String())
		}

		demoMode, pinConfigured := requireDemoStatus(t, srv.baseURL)
		if demoMode {
			t.Error("/api/auth/status reports demo_mode=true with SPAXEL_DEMO_MODE unset")
		}
		if pinConfigured {
			t.Error("fresh data dir already reports pin_configured=true")
		}

		// First-run setup: the POST that demo mode would block.
		status, _, body := demoDo(t, http.MethodPost, srv.baseURL+"/api/auth/setup", `{"pin":"`+demoModePIN+`"}`)
		if status != http.StatusOK {
			t.Fatalf("POST /api/auth/setup: got status %d, want 200 (body: %s)", status, body)
		}

		demoMode, pinConfigured = requireDemoStatus(t, srv.baseURL)
		if !pinConfigured {
			t.Error("pin_configured still false after successful setup")
		}

		// The correct PIN logs in while demo mode is off — the same POST Phase B
		// must reject.
		status, _, body = demoDo(t, http.MethodPost, srv.baseURL+"/api/auth/login", `{"pin":"`+demoModePIN+`"}`)
		if status != http.StatusOK {
			t.Fatalf("POST /api/auth/login with correct PIN: got status %d, want 200 (body: %s)", status, body)
		}
	})

	// ---------------------------------------------------------------------------
	// Phase B — demo ON, same data dir: reads open, mutations 403, no login.
	// ---------------------------------------------------------------------------
	t.Run("phase B demo mode blocks mutations and leaves reads open", func(t *testing.T) {
		// SPAXEL_DEMO_MAX_DASHBOARD_CLIENTS=7 exercises the demo cap override; the
		// startup log line asserts the value actually reached the hub.
		srv := startDemoServer(t, bin, dataDir, true, 7)
		defer func() { _ = srv.stop() }()

		if !strings.Contains(srv.stderr.String(), "Dashboard client cap: 7 (demo mode: true)") {
			t.Errorf("startup log missing demo-mode cap line; stderr:\n%s", srv.stderr.String())
		}

		t.Run("status reports demo mode with the PIN still configured", func(t *testing.T) {
			// Deliberately cookie-less: no login has happened in this phase.
			demoMode, pinConfigured := requireDemoStatus(t, srv.baseURL)
			if !demoMode {
				t.Error("demo_mode=false while SPAXEL_DEMO_MODE=true")
			}
			if !pinConfigured {
				t.Error("pin_configured=false — Phase A's PIN did not survive the restart")
			}
		})

		t.Run("read-only GET endpoints stay open without a session", func(t *testing.T) {
			for _, path := range []string{
				"/healthz",
				"/api/auth/status",
				"/api/nodes",
				"/api/settings",
				"/api/zones",
				"/api/mode",
			} {
				t.Run(path, func(t *testing.T) {
					status, contentType, body := demoDo(t, http.MethodGet, srv.baseURL+path, "")
					if status != http.StatusOK {
						t.Fatalf("GET %s: got status %d, want 200 (body: %s)", path, status, body)
					}
					if strings.Contains(contentType, "text/html") {
						t.Errorf("GET %s: API endpoint answered with HTML", path)
					}
				})
			}
		})

		t.Run("dashboard pages render without a PIN prompt", func(t *testing.T) {
			// Each real page carries a title the login-only fallback page does not;
			// the login page's inline background color (#1a1a2e) must never appear.
			for _, page := range []struct {
				path  string
				title string
			}{
				{"/", "Spaxel</title>"},
				{"/fleet", "Spaxel Fleet Status"},
				{"/simple", "Spaxel - Simple Mode"},
			} {
				t.Run(page.path, func(t *testing.T) {
					status, contentType, body := demoDo(t, http.MethodGet, srv.baseURL+page.path, "")
					if status != http.StatusOK {
						t.Fatalf("GET %s: got status %d, want 200", page.path, status)
					}
					if !strings.Contains(contentType, "text/html") {
						t.Fatalf("GET %s: got Content-Type %q, want text/html", page.path, contentType)
					}
					if !strings.Contains(body, page.title) {
						t.Errorf("GET %s: body lacks expected title %q — got:\n%.400s", page.path, page.title, body)
					}
					if strings.Contains(body, "#1a1a2e") {
						t.Errorf("GET %s: served the login-only page instead of the dashboard", page.path)
					}
				})
			}
		})

		t.Run("mutating endpoints are rejected with 403", func(t *testing.T) {
			for _, mutation := range []struct {
				name   string
				method string
				path   string
				body   string
			}{
				{"update settings via POST", http.MethodPost, "/api/settings", `{"theme":"dark"}`},
				{"update settings via PATCH", http.MethodPatch, "/api/settings", `{"theme":"dark"}`},
				{"create zone", http.MethodPost, "/api/zones", `{"name":"demo"}`},
				{"update node position", http.MethodPut, "/api/nodes/AA:BB:CC:DD:EE:FF/position", `{"x":1,"y":2}`},
				{"reboot node", http.MethodPost, "/api/nodes/AA:BB:CC:DD:EE:FF/reboot", ""},
				{"delete node", http.MethodDelete, "/api/nodes/AA:BB:CC:DD:EE:FF", ""},
				// Auth endpoints are mutating too: setup is pointless (PIN exists) and
				// login would mint a session, so both must die at the middleware.
				{"login with the correct PIN", http.MethodPost, "/api/auth/login", `{"pin":"` + demoModePIN + `"}`},
				{"setup another PIN", http.MethodPost, "/api/auth/setup", `{"pin":"135790"}`},
				// Middleware runs ahead of chi routing, so an unknown path is still
				// 403 (not 404) for a mutating verb — proof the block is global.
				{"unknown path still blocked", http.MethodPost, "/api/demo-mode-nonexistent", `{}`},
			} {
				t.Run(mutation.name, func(t *testing.T) {
					requireDemoRejection(t, mutation.method, srv.baseURL+mutation.path, mutation.body)
				})
			}
		})

		t.Run("reads still work after the rejected mutations", func(t *testing.T) {
			for _, path := range []string{"/api/nodes", "/api/settings", "/api/zones"} {
				status, _, body := demoDo(t, http.MethodGet, srv.baseURL+path, "")
				if status != http.StatusOK {
					t.Fatalf("GET %s after mutations: got status %d, want 200 (body: %s)", path, status, body)
				}
			}
		})
	})
}
