// Package auth provides authentication tests.
package auth

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// This file pins the persistence contract of the auth layer across a
// mothership restart. Every piece of authentication state — the PIN hash and
// install secret in the auth singleton, the rows in the sessions table —
// lives in SQLite, so closing the database and constructing a fresh Handler
// over a new connection must behave exactly like the pre-restart process.
// The specification for these behaviors lives in docs/notes/dashboard-pin-auth.md.

// openRestartedHandler opens a file-backed SQLite database and a Handler over
// it, standing in for one mothership process. Each call with the same path is
// a fresh process: a new connection pool over the same file on disk.
func openRestartedHandler(t *testing.T, dbPath string) (*sql.DB, *Handler) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	h, err := NewHandler(Config{DB: db})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return db, h
}

// closeInstance tears down one mothership "process" explicitly, so the next
// openRestartedHandler is a genuine restart rather than a second connection
// to a live one. (The helper's t.Cleanup closes are idempotent backstops.)
func closeInstance(_ *sql.DB, h *Handler) {
	_ = h.Close()
}

// stopDB closes a database that closeInstance did not (kept separate so the
// restart ordering in the tests reads: handler down, then pool down).
func stopDB(db *sql.DB) {
	_ = db.Close()
}

// authStatus GETs /api/auth/status and reports pin_configured.
func authStatus(t *testing.T, h *Handler) bool {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	w := httptest.NewRecorder()
	h.handleStatus(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status: expected 200, got %d", w.Code)
	}
	var resp map[string]bool
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	return resp["pin_configured"]
}

// TestPINAuth_PersistedAcrossRestart drives the full first-run journey against
// a file-backed database, restarts (closes the pool, opens a fresh Handler on
// a new connection), and pins that authentication configuration survives:
// no re-setup prompt, correct PIN still accepted, wrong PIN still rejected,
// the pre-restart session still validates, and the install secret — which
// node tokens are derived from — is unchanged.
func TestPINAuth_PersistedAcrossRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "spaxel-auth-restart.db")

	// --- Instance 1: fresh install and first-run setup ---
	db1, h1 := openRestartedHandler(t, dbPath)
	if authStatus(t, h1) {
		t.Fatal("fresh install: pin_configured should be false before setup")
	}
	cookie1 := configurePin(t, h1) // sets PIN "4321", mints a session
	secret1, err := h1.GetInstallSecret()
	if err != nil {
		t.Fatalf("get install secret (instance 1): %v", err)
	}

	// --- Restart: instance 1 fully down, then a new process on the same file ---
	closeInstance(db1, h1)
	stopDB(db1)
	_, h2 := openRestartedHandler(t, dbPath) // instance 2: fresh pool, same file

	// No re-setup prompt: the PIN configuration survives.
	if !authStatus(t, h2) {
		t.Error("after restart: pin_configured should be true")
	}

	// The onboarding window stays closed: setup is refused as already-done.
	w := doSetup(t, h2, "5678")
	if w.Code != http.StatusConflict {
		t.Errorf("setup after restart: expected 409, got %d (body %q)", w.Code, w.Body.String())
	}

	// Wrong PIN still rejected.
	if w := doLogin(t, h2, "9999"); w.Code != http.StatusUnauthorized {
		t.Errorf("wrong PIN after restart: expected 401, got %d", w.Code)
	}

	// The pre-restart PIN is still the PIN.
	w = doLogin(t, h2, "4321")
	if w.Code != http.StatusOK {
		t.Fatalf("correct PIN after restart: expected 200, got %d (body %q)", w.Code, w.Body.String())
	}
	_ = sessionCookie(t, w) // login still mints a session cookie

	// A session minted before the restart still validates afterwards:
	// sessions are SQLite rows, not signed tokens, so the fresh process
	// honors cookies issued by the previous one.
	req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	req.AddCookie(cookie1)
	if got := h2.ValidateSession(req); got == "" {
		t.Error("pre-restart session cookie should still validate after restart")
	}

	// The install secret is unchanged, so node tokens derived before the
	// restart keep validating.
	secret2, err := h2.GetInstallSecret()
	if err != nil {
		t.Fatalf("get install secret (instance 2): %v", err)
	}
	if hex.EncodeToString(secret1) != hex.EncodeToString(secret2) {
		t.Error("install secret changed across restart")
	}
}

// TestPINAuth_RestartBeforeSetupStaysOnboarding pins the inverse: a restart
// before any setup must not conjure a PIN. The onboarding window reopens
// exactly as it was — pin_configured false, login 404, middleware passing
// traffic — which is what makes a crashed first run recoverable.
func TestPINAuth_RestartBeforeSetupStaysOnboarding(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "spaxel-auth-restart-fresh.db")

	db1, h1 := openRestartedHandler(t, dbPath)
	closeInstance(db1, h1)
	stopDB(db1)

	_, h2 := openRestartedHandler(t, dbPath)

	if authStatus(t, h2) {
		t.Error("restart before setup: pin_configured should still be false")
	}

	if w := doLogin(t, h2, "1234"); w.Code != http.StatusNotFound {
		t.Errorf("login before any setup: expected 404, got %d", w.Code)
	}

	// Onboarding window still open: unauthenticated traffic passes.
	if _, reached := probeMiddleware(h2, http.MethodGet, "/api/nodes", nil); !reached {
		t.Error("onboarding window should stay open across a restart with no PIN")
	}
}
