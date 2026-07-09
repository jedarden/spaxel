package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// dashboardAssetsMarker is a real asset the regression assertions check for.
// resolveDashboardDirForTest uses it to prove it found the actual dashboard/
// tree, not just any directory that happens to be named "dashboard".
var dashboardAssetsMarker = filepath.Join("css", "tokens.css")

// resolveDashboardDirForTest locates the repo's real dashboard/ assets for the
// test. go test runs with the working directory set to the package source dir
// (mothership/cmd/mothership/), so the runtime findDashboardDir() candidates
// (./dashboard, ./../dashboard, /app/dashboard, plus an executable-relative walk
// from the temp test binary in /tmp) all miss. We instead walk upward from the
// CWD until we find a dashboard/ that actually contains the marker asset, then
// fall back to an executable-relative walk. Returns "" if none is found — the
// caller then t.Skips rather than failing, so a checkout without dashboard/
// (e.g. an embedded-only build that strips the source tree) does not break
// `go test ./...`.
func resolveDashboardDirForTest(t *testing.T) string {
	t.Helper()
	search := func(start string) string {
		dir := start
		for i := 0; i < 12; i++ {
			cand := filepath.Join(dir, "dashboard")
			if _, err := os.Stat(filepath.Join(cand, dashboardAssetsMarker)); err == nil {
				return cand
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				return "" // reached filesystem root
			}
			dir = parent
		}
		return ""
	}
	if cwd, err := os.Getwd(); err == nil {
		if d := search(cwd); d != "" {
			return d
		}
	}
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		if d := search(filepath.Dir(exe)); d != "" {
			return d
		}
	}
	return ""
}

// mimeMatches reports whether a response Content-Type is acceptable for the
// expected MIME family. wantPrefix is the preferred prefix; for JavaScript,
// application/javascript is also accepted because some OS MIME registries
// report it instead of text/javascript.
func mimeMatches(ct, wantPrefix string) bool {
	if strings.HasPrefix(ct, wantPrefix) {
		return true
	}
	if wantPrefix == "text/javascript" && strings.HasPrefix(ct, "application/javascript") {
		return true
	}
	return false
}

// truncateBody keeps failure output readable.
func truncateBody(s string) string {
	const max = 200
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

// TestDashboardStaticAssets is the bf-3uud3 regression test for the bf-1cgqe
// "404 / text/plain" failure: the dashboard static handler must serve the real
// CSS, JS, and index.html assets over GET with the correct Content-Type.
//
// It MUST use GET (not HEAD): the route is GET-only and chi returns 405 for
// HEAD — which is exactly why earlier `curl -sI` (HEAD) checks misleadingly
// reported the handler as broken. httptest.NewRequest(http.MethodGet, ...) is
// the contract the assertions depend on.
//
// If the handler is unregistered, status is 404 (not 200); if it serves the
// wrong type (the bf-1cgqe text/plain regression), mimeMatches rejects it.
// Both failure modes are exercised as negative controls below so the
// assertions are known to have teeth.
func TestDashboardStaticAssets(t *testing.T) {
	dashboardDir := resolveDashboardDirForTest(t)
	if dashboardDir == "" {
		t.Skip("dashboard/ assets not found from test CWD or binary dir; cannot exercise real assets")
	}

	r := chi.NewRouter()
	if !registerDashboardStatic(r, dashboardDir) {
		t.Fatalf("registerDashboardStatic(%q) registered nothing; resolved dashboard dir is not a directory", dashboardDir)
	}

	cases := []struct {
		name     string
		target   string
		wantMIME string // Content-Type must begin with this; text/javascript also accepts application/javascript
	}{
		{"css asset", "/css/tokens.css", "text/css"},
		{"js asset", "/js/app.js", "text/javascript"},
		{"index html", "/", "text/html"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.target, nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s: status = %d, want 200 (body=%q)", tc.target, rec.Code, truncateBody(rec.Body.String()))
			}
			ct := rec.Header().Get("Content-Type")
			if !mimeMatches(ct, tc.wantMIME) {
				t.Fatalf("GET %s: Content-Type = %q, want %q prefix (the bf-1cgqe regression served text/plain here)", tc.target, ct, tc.wantMIME)
			}
			// Explicit guard against the text/plain regression: a correctly
			// served CSS/JS asset must never report text/plain.
			if strings.HasPrefix(ct, "text/plain") {
				t.Fatalf("GET %s: Content-Type = %q, must not be text/plain (the bf-1cgqe regression)", tc.target, ct)
			}
			if rec.Body.Len() == 0 {
				t.Fatalf("GET %s: response body is empty", tc.target)
			}
		})
	}
}

// TestDashboardStaticAssetsGuardUnregistered proves the assertions have teeth
// against an UNREGISTERED handler: a bare chi router with no static route must
// not return 200 for the assets. If it did, the main test would be guarding
// nothing — a regression that simply drops the registration would slip through.
func TestDashboardStaticAssetsGuardUnregistered(t *testing.T) {
	if resolveDashboardDirForTest(t) == "" {
		t.Skip("dashboard/ assets not found; negative control not applicable")
	}
	r := chi.NewRouter() // intentionally no static handler registered
	for _, target := range []string{"/css/tokens.css", "/js/app.js", "/"} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code == http.StatusOK {
			t.Fatalf("GET %s on an UNREGISTERED router returned 200; the main test could not detect a dropped registration", target)
		}
	}
}

// TestDashboardStaticAssetsGuardRejectsTextPlain proves the MIME assertion has
// teeth against the bf-1cgqe failure mode specifically: a handler that serves
// the real bytes but lies about Content-Type (text/plain) must be rejected by
// mimeMatches. If mimeMatches ever accepted text/plain as text/css, the
// regression would go undetected.
func TestDashboardStaticAssetsGuardRejectsTextPlain(t *testing.T) {
	// Deliberately buggy handler: serves bytes, wrong type — exactly the
	// bf-1cgqe symptom (the file is found, but Content-Type is text/plain).
	r := chi.NewRouter()
	r.Get("/*", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "/* bogus css */")
	})
	req := httptest.NewRequest(http.MethodGet, "/css/tokens.css", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	ct := rec.Header().Get("Content-Type")
	if mimeMatches(ct, "text/css") {
		t.Fatalf("mimeMatches accepted %q as text/css; the bf-1cgqe text/plain regression would go undetected", ct)
	}
}
