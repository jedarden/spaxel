// Package auth provides authentication tests.
package auth

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// This file pins the session-lifecycle and route-protection contracts of the
// auth Handler: failed-login handling, cookie attributes, session expiration
// and rolling extension, session-ID uniqueness, and the full Middleware
// decision matrix including the protected WebSocket route (/ws/dashboard).
// The specification for these behaviors lives in docs/notes/dashboard-pin-auth.md.

// newAuthEnv opens an in-memory SQLite database and returns it with a Handler
// initialized against it. Cleanup order matters: the handler's cleanup
// goroutine must stop before the database closes.
func newAuthEnv(t *testing.T) (*sql.DB, *Handler) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	// :memory: SQLite is per-connection; pin the pool to a single connection.
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	h, err := NewHandler(Config{DB: db})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return db, h
}

// doSetup submits a first-run PIN setup request directly to the handler.
func doSetup(t *testing.T, h *Handler, pin string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/setup", strings.NewReader(`{"pin": "`+pin+`"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.handleSetup(w, req)
	return w
}

// doLogin submits a login request directly to the handler.
func doLogin(t *testing.T, h *Handler, pin string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"pin": "`+pin+`"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.handleLogin(w, req)
	return w
}

// sessionCookie extracts the spaxel_session cookie from a response, failing
// the test if none was set.
func sessionCookie(t *testing.T, w *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == "spaxel_session" {
			return c
		}
	}
	t.Fatal("expected spaxel_session cookie on response")
	return nil
}

// configurePin performs first-run setup and returns the session cookie it
// minted.
func configurePin(t *testing.T, h *Handler) *http.Cookie {
	t.Helper()
	w := doSetup(t, h, "4321")
	if w.Code != http.StatusOK {
		t.Fatalf("setup: expected 200, got %d (body %q)", w.Code, w.Body.String())
	}
	return sessionCookie(t, w)
}

// countSessions returns the number of rows in the sessions table.
func countSessions(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// sessionRow reads back the stored timestamps for a session.
func sessionRow(t *testing.T, db *sql.DB, sessionID string) (expiresAt, lastSeenAt int64) {
	t.Helper()
	err := db.QueryRow(
		"SELECT expires_at, last_seen_at FROM sessions WHERE session_id = ?",
		sessionID,
	).Scan(&expiresAt, &lastSeenAt)
	if err != nil {
		t.Fatalf("query session row: %v", err)
	}
	return expiresAt, lastSeenAt
}

const probePassthroughBody = "PROBE-HANDLER-REACHED"

// probeMiddleware runs a request through the auth Middleware with a marker
// handler underneath and reports whether the marker was reached.
func probeMiddleware(h *Handler, method, path string, cookie *http.Cookie) (*httptest.ResponseRecorder, bool) {
	reached := false
	base := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		_, _ = w.Write([]byte(probePassthroughBody))
	})
	req := httptest.NewRequest(method, path, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	h.Middleware(base).ServeHTTP(w, req)
	return w, reached
}

// TestLogin_FailureCreatesNoCookieAndNoSession pins the failed-login contract:
// a wrong PIN gets a 401 with no Set-Cookie and mints no sessions row, and
// login while no PIN is configured gets a 404 with no cookie either.
func TestLogin_FailureCreatesNoCookieAndNoSession(t *testing.T) {
	t.Run("wrong PIN", func(t *testing.T) {
		db, h := newAuthEnv(t)
		configurePin(t, h)
		before := countSessions(t, db)

		w := doLogin(t, h, "9999")
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("wrong-PIN login: expected 401, got %d", w.Code)
		}
		if cookies := w.Result().Cookies(); len(cookies) != 0 {
			t.Errorf("failed login set %d cookie(s), want 0: %v", len(cookies), cookies)
		}
		if got := countSessions(t, db); got != before {
			t.Errorf("failed login changed sessions row count: before=%d after=%d", before, got)
		}
	})

	t.Run("no PIN configured", func(t *testing.T) {
		_, h := newAuthEnv(t)

		w := doLogin(t, h, "1234")
		if w.Code != http.StatusNotFound {
			t.Fatalf("login without configured PIN: expected 404, got %d", w.Code)
		}
		if cookies := w.Result().Cookies(); len(cookies) != 0 {
			t.Errorf("login without configured PIN set %d cookie(s), want 0: %v", len(cookies), cookies)
		}
	})
}

// TestSessionCookieAttributes pins the session cookie contract on both paths
// that mint a session: first-run setup and login.
func TestSessionCookieAttributes(t *testing.T) {
	cases := []struct {
		name    string
		prepare bool // true = set the PIN first (login path); false = first-run setup path
		mint    func(t *testing.T, h *Handler) *httptest.ResponseRecorder
	}{
		{"first-run setup", false, func(t *testing.T, h *Handler) *httptest.ResponseRecorder { return doSetup(t, h, "4321") }},
		{"login", true, func(t *testing.T, h *Handler) *httptest.ResponseRecorder { return doLogin(t, h, "4321") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, h := newAuthEnv(t)
			if tc.prepare {
				configurePin(t, h)
			}

			w := tc.mint(t, h)
			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d (body %q)", w.Code, w.Body.String())
			}
			c := sessionCookie(t, w)
			if c.Path != "/" {
				t.Errorf("cookie Path = %q, want \"/\"", c.Path)
			}
			if c.MaxAge != 604800 {
				t.Errorf("cookie MaxAge = %d, want 604800 (7 days)", c.MaxAge)
			}
			if !c.HttpOnly {
				t.Error("cookie is not HttpOnly")
			}
			if c.SameSite != http.SameSiteStrictMode {
				t.Errorf("cookie SameSite = %v, want Strict", c.SameSite)
			}
			if c.Secure {
				t.Error("cookie Secure = true, want false (TLS terminates upstream)")
			}
		})
	}
}

// TestValidateSession_ExpiredSessionRejected pins expiration: once
// expires_at is in the past the session no longer validates.
func TestValidateSession_ExpiredSessionRejected(t *testing.T) {
	db, h := newAuthEnv(t)
	configurePin(t, h)
	cookie := sessionCookie(t, doLogin(t, h, "4321"))

	past := time.Now().Add(-time.Minute).UnixMilli()
	if _, err := db.Exec("UPDATE sessions SET expires_at = ? WHERE session_id = ?", past, cookie.Value); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	req.AddCookie(cookie)
	if got := h.ValidateSession(req); got != "" {
		t.Errorf("expired session validated as %q, want empty", got)
	}
}

// TestValidateSession_RollingExtension pins the rolling session window:
// validation within 24h of expiry re-extends expires_at by 7 days, and
// validation further out leaves expires_at alone; both paths bump
// last_seen_at.
func TestValidateSession_RollingExtension(t *testing.T) {
	cases := []struct {
		name       string
		backdate   time.Duration // stored expires_at = now + backdate
		wantExtend bool
	}{
		{"within 24h of expiry extends by 7 days", 12 * time.Hour, true},
		{"more than 24h from expiry keeps expires_at", 6 * 24 * time.Hour, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, h := newAuthEnv(t)
			configurePin(t, h)
			cookie := sessionCookie(t, doLogin(t, h, "4321"))

			now := time.Now()
			expires := now.Add(tc.backdate).UnixMilli()
			oldLastSeen := now.Add(-time.Hour).UnixMilli()
			if _, err := db.Exec(
				"UPDATE sessions SET expires_at = ?, last_seen_at = ? WHERE session_id = ?",
				expires, oldLastSeen, cookie.Value,
			); err != nil {
				t.Fatal(err)
			}

			req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
			req.AddCookie(cookie)
			if got := h.ValidateSession(req); got == "" {
				t.Fatal("expected session to validate")
			}

			gotExpires, gotLastSeen := sessionRow(t, db, cookie.Value)
			if tc.wantExtend {
				want := now.Add(7 * 24 * time.Hour).UnixMilli()
				if diff := gotExpires - want; diff < -3*60*1000 || diff > 3*60*1000 {
					t.Errorf("extended expires_at off by %dms: got %d, want ~%d", diff, gotExpires, want)
				}
			} else if diff := gotExpires - expires; diff < -5000 || diff > 5000 {
				t.Errorf("expires_at changed on far-from-expiry validation: set %d, got %d", expires, gotExpires)
			}
			if gotLastSeen <= oldLastSeen {
				t.Errorf("last_seen_at not bumped: set %d, got %d", oldLastSeen, gotLastSeen)
			}
		})
	}
}

// TestSessionIDs_UniqueAcrossLogins pins that each login mints a fresh
// 64-hex-char session ID and that concurrent sessions coexist.
func TestSessionIDs_UniqueAcrossLogins(t *testing.T) {
	db, h := newAuthEnv(t)
	configurePin(t, h)

	first := sessionCookie(t, doLogin(t, h, "4321"))
	second := sessionCookie(t, doLogin(t, h, "4321"))

	if first.Value == second.Value {
		t.Fatal("two logins produced the same session ID")
	}
	for _, c := range []*http.Cookie{first, second} {
		if len(c.Value) != 64 {
			t.Errorf("session ID length = %d, want 64 (32 random bytes hex)", len(c.Value))
		}
		if _, err := hex.DecodeString(c.Value); err != nil {
			t.Errorf("session ID is not hex: %v", err)
		}
	}
	if got := countSessions(t, db); got != 3 { // setup + two logins
		t.Errorf("sessions rows = %d, want 3", got)
	}
}

// TestMiddleware_ProtectedRoutes pins the full route-protection decision
// matrix: the onboarding window, the locked-out behavior for pages vs. API vs.
// WebSocket, the public-path and static-asset exemptions, and pass-through for
// a valid session — including the protected WebSocket route /ws/dashboard.
func TestMiddleware_ProtectedRoutes(t *testing.T) {
	t.Run("onboarding window passes all routes", func(t *testing.T) {
		_, h := newAuthEnv(t) // no PIN configured yet

		for _, probe := range []struct{ method, path string }{
			{http.MethodGet, "/"},
			{http.MethodGet, "/ambient.html"},
			{http.MethodGet, "/api/nodes"},
			{http.MethodGet, "/ws/dashboard"},
			{http.MethodPost, "/api/nodes"},
			{http.MethodGet, "/firmware/spaxel-1.0.0.bin"},
		} {
			w, reached := probeMiddleware(h, probe.method, probe.path, nil)
			if !reached {
				t.Errorf("%s %s blocked during onboarding: status %d body %.40q",
					probe.method, probe.path, w.Code, w.Body.String())
			}
		}
	})

	t.Run("locked out without a session", func(t *testing.T) {
		_, h := newAuthEnv(t)
		configurePin(t, h)

		for _, probe := range []struct {
			method, path string
			wantPass     bool
		}{
			// Pages and protected API/WS routes are gated.
			{http.MethodGet, "/", false},
			{http.MethodGet, "/ambient.html", false},
			{http.MethodGet, "/api/nodes", false},
			{http.MethodGet, "/ws/dashboard", false},
			{http.MethodGet, "/api/doctor", false},
			// Public paths and login-page assets pass.
			{http.MethodGet, "/healthz", true},
			{http.MethodGet, "/api/auth/status", true},
			{http.MethodPost, "/api/auth/login", true},
			{http.MethodPost, "/api/auth/logout", true},
			{http.MethodGet, "/api/provision", true},
			{http.MethodGet, "/ws/node", true},
			{http.MethodGet, "/firmware/spaxel-1.0.0.bin", true},
			{http.MethodGet, "/js/auth.js", true},
			{http.MethodGet, "/css/panels.css", true},
			{http.MethodGet, "/favicon.ico", true},
		} {
			w, reached := probeMiddleware(h, probe.method, probe.path, nil)
			if reached != probe.wantPass {
				t.Errorf("%s %s: reached=%v, want %v (status %d body %.40q)",
					probe.method, probe.path, reached, probe.wantPass, w.Code, w.Body.String())
			}
		}

		// Protected API/WS requests get the JSON 401 contract.
		for _, path := range []string{"/api/nodes", "/api/doctor", "/ws/dashboard"} {
			w, _ := probeMiddleware(h, http.MethodGet, path, nil)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s: status = %d, want 401", path, w.Code)
			}
			if ct := w.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("%s: Content-Type = %q, want application/json", path, ct)
			}
			var body map[string]string
			if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
				t.Fatalf("%s: decode 401 body: %v", path, err)
			}
			if body["error"] != "authentication required" {
				t.Errorf("%s: error = %q, want %q", path, body["error"], "authentication required")
			}
		}

		// Page requests get the login-only page, not dashboard markup.
		for _, path := range []string{"/", "/ambient.html"} {
			w, _ := probeMiddleware(h, http.MethodGet, path, nil)
			if w.Code != http.StatusOK {
				t.Errorf("%s: status = %d, want 200 (login page)", path, w.Code)
			}
			if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
				t.Errorf("%s: Content-Type = %q, want text/html", path, ct)
			}
			if !strings.Contains(w.Body.String(), "auth.js") {
				t.Errorf("%s: response is not the login page: %.60q", path, w.Body.String())
			}
		}
	})

	t.Run("valid session passes protected routes", func(t *testing.T) {
		_, h := newAuthEnv(t)
		cookie := configurePin(t, h)

		for _, probe := range []struct{ method, path string }{
			{http.MethodGet, "/"},
			{http.MethodGet, "/ambient.html"},
			{http.MethodGet, "/api/nodes"},
			{http.MethodGet, "/ws/dashboard"},
			{http.MethodGet, "/api/doctor"},
		} {
			w, reached := probeMiddleware(h, probe.method, probe.path, cookie)
			if !reached || w.Code != http.StatusOK {
				t.Errorf("%s %s with valid session: reached=%v status=%d, want reached=true status=200",
					probe.method, probe.path, reached, w.Code)
			}
		}
	})

	t.Run("expired and unknown sessions are rejected", func(t *testing.T) {
		db, h := newAuthEnv(t)
		cookie := configurePin(t, h)

		past := time.Now().Add(-time.Minute).UnixMilli()
		if _, err := db.Exec("UPDATE sessions SET expires_at = ? WHERE session_id = ?", past, cookie.Value); err != nil {
			t.Fatal(err)
		}

		for _, path := range []string{"/api/nodes", "/api/doctor", "/ws/dashboard"} {
			w, reached := probeMiddleware(h, http.MethodGet, path, cookie)
			if reached {
				t.Errorf("%s: expired session admitted", path)
			}
			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s: status = %d, want 401", path, w.Code)
			}
		}

		w, reached := probeMiddleware(h, http.MethodGet, "/api/nodes",
			&http.Cookie{Name: "spaxel_session", Value: "deadbeef"})
		if reached {
			t.Error("unknown session ID admitted")
		}
		if w.Code != http.StatusUnauthorized {
			t.Errorf("unknown session ID: status = %d, want 401", w.Code)
		}
	})
}
