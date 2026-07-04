// Package api provides tests for the system status API handler.
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// statusResponse is the JSON shape returned by GET /api/status.
// Mirrors the map written by StatusHandler.getStatus.
type statusResponse struct {
	Version          string `json:"version"`
	Nodes            int    `json:"nodes"`
	Blobs            int    `json:"blobs"`
	UptimeS          int64  `json:"uptime_s"`
	DetectionQuality int    `json:"detection_quality"`
}

// newStatusRouter wires a StatusHandler into a chi router for handler-level tests.
func newStatusRouter(h *StatusHandler) http.Handler {
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return r
}

// TestStatusHandlerVersionWired is the regression test for the hardcoded
// "1.0.0" version string (bf-5hz): the version passed to NewStatusHandler —
// which mirrors the build-time `-X main.version` ldflag injected in
// cmd/mothership/main.go — must be the value surfaced at GET /api/status, not
// a constant. Table-driven across representative version strings.
func TestStatusHandlerVersionWired(t *testing.T) {
	tests := []struct {
		name    string
		version string
	}{
		{"release_semver", "0.1.357"},
		{"dev_build", "dev"},
		{"dirty_tree", "0.1.358-dirty"},
		{"empty_string", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewStatusHandler(time.Now(), func() int { return 0 }, tt.version)
			server := newStatusRouter(h)

			req := httptest.NewRequest("GET", "/api/status", nil)
			w := httptest.NewRecorder()
			server.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d", w.Code)
			}

			var resp statusResponse
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil { //nolint:errcheck
				t.Fatalf("failed to decode response: %v", err)
			}

			if resp.Version != tt.version {
				t.Errorf("version = %q, want %q (must not be hardcoded \"1.0.0\")", resp.Version, tt.version)
			}
		})
	}
}

// TestStatusHandlerFields verifies the non-version fields of GET /api/status:
// node count is sourced from the callback, uptime is non-negative, and a nil
// processor manager yields zero blobs and zero detection quality.
func TestStatusHandlerFields(t *testing.T) {
	start := time.Now().Add(-30 * time.Second)
	h := NewStatusHandler(start, func() int { return 4 }, "0.1.357")
	server := newStatusRouter(h)

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp statusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil { //nolint:errcheck
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Nodes != 4 {
		t.Errorf("nodes = %d, want 4", resp.Nodes)
	}
	if resp.Blobs != 0 {
		t.Errorf("blobs = %d, want 0 (no processor manager)", resp.Blobs)
	}
	if resp.UptimeS < 0 {
		t.Errorf("uptime_s = %d, want >= 0", resp.UptimeS)
	}
	if resp.DetectionQuality != 0 {
		t.Errorf("detection_quality = %d, want 0 (no processor manager)", resp.DetectionQuality)
	}
}

// TestStatusHandlerNilNodeCountCallback confirms a nil getNodeCount callback
// is tolerated and reports zero nodes rather than panicking.
func TestStatusHandlerNilNodeCountCallback(t *testing.T) {
	h := NewStatusHandler(time.Now(), nil, "0.1.357")
	server := newStatusRouter(h)

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp statusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil { //nolint:errcheck
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Nodes != 0 {
		t.Errorf("nodes = %d, want 0 for nil callback", resp.Nodes)
	}
}
