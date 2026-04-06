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

	_ "modernc.org/sqlite"
)

func TestHandler_StatusNotConfigured(t *testing.T) {
	// Create in-memory database
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create handler
	h, err := NewHandler(Config{DB: db})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	// Test status endpoint
	req := httptest.NewRequest("GET", "/api/auth/status", nil)
	w := httptest.NewRecorder()
	h.handleStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Should return pin_configured: false
	var resp map[string]bool
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	if resp["pin_configured"] {
		t.Error("Expected pin_configured to be false initially")
	}
}

func TestHandler_SetupPIN(t *testing.T) {
	// Create in-memory database
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create handler
	h, err := NewHandler(Config{DB: db})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	// Test setup with valid PIN
	reqBody := `{"pin": "1234"}`
	req := httptest.NewRequest("POST", "/api/auth/setup", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.handleSetup(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Check session cookie was set
	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "spaxel_session" {
			sessionCookie = c
			break
		}
	}

	if sessionCookie == nil {
		t.Error("Expected session cookie to be set")
	} else if sessionCookie.MaxAge != 604800 {
		t.Errorf("Expected MaxAge 604800, got %d", sessionCookie.MaxAge)
	}

	// Verify PIN is now configured
	var pinBcrypt sql.NullString
	err = db.QueryRow("SELECT pin_bcrypt FROM auth WHERE id = 1").Scan(&pinBcrypt)
	if err != nil {
		t.Fatal(err)
	}

	if !pinBcrypt.Valid {
		t.Error("Expected PIN to be configured after setup")
	}
}

func TestHandler_SetupPINInvalid(t *testing.T) {
	tests := []struct {
		name       string
		pin        string
		wantStatus int
	}{
		{"too short", "123", http.StatusBadRequest},
		{"too long", "123456789", http.StatusBadRequest},
		{"non-numeric", "abcd", http.StatusBadRequest},
		{"mixed", "12a4", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := sql.Open("sqlite", ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()

			h, err := NewHandler(Config{DB: db})
			if err != nil {
				t.Fatal(err)
			}
			defer h.Close()

			reqBody := `{"pin": "` + tt.pin + `"}`
			req := httptest.NewRequest("POST", "/api/auth/setup", strings.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.handleSetup(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("Expected status %d, got %d", tt.wantStatus, w.Code)
			}
		})
	}
}

func TestHandler_SetupPINAlreadyConfigured(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	h, err := NewHandler(Config{DB: db})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	// First setup should succeed
	reqBody := `{"pin": "1234"}`
	req := httptest.NewRequest("POST", "/api/auth/setup", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.handleSetup(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("First setup failed: %d", w.Code)
	}

	// Second setup should fail
	req = httptest.NewRequest("POST", "/api/auth/setup", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.handleSetup(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("Expected status 409, got %d", w.Code)
	}
}

func TestHandler_LoginInvalidPIN(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	h, err := NewHandler(Config{DB: db})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	// Setup PIN first
	reqBody := `{"pin": "1234"}`
	req := httptest.NewRequest("POST", "/api/auth/setup", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.handleSetup(w, req)

	// Try login with wrong PIN
	reqBody = `{"pin": "9999"}`
	req = httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.handleLogin(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestHandler_LoginValidPIN(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	h, err := NewHandler(Config{DB: db})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	// Setup PIN first
	reqBody := `{"pin": "1234"}`
	req := httptest.NewRequest("POST", "/api/auth/setup", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.handleSetup(w, req)

	// Login with correct PIN
	reqBody = `{"pin": "1234"}`
	req = httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.handleLogin(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Check session cookie was set
	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "spaxel_session" {
			sessionCookie = c
			break
		}
	}

	if sessionCookie == nil {
		t.Error("Expected session cookie to be set")
	}
}

func TestHandler_ValidateSession(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	h, err := NewHandler(Config{DB: db})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	// Setup and login
	reqBody := `{"pin": "1234"}`
	req := httptest.NewRequest("POST", "/api/auth/setup", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.handleSetup(w, req)

	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "spaxel_session" {
			sessionCookie = c
			break
		}
	}

	if sessionCookie == nil {
		t.Fatal("Session cookie not set")
	}

	// Validate session
	req = httptest.NewRequest("GET", "/api/test", nil)
	req.AddCookie(sessionCookie)
	sessionID := h.ValidateSession(req)

	if sessionID == "" {
		t.Error("Expected session to be valid")
	}

	if sessionID != sessionCookie.Value {
		t.Error("Session ID mismatch")
	}
}

func TestHandler_ValidateSessionInvalid(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	h, err := NewHandler(Config{DB: db})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	// Test with no cookie
	req := httptest.NewRequest("GET", "/api/test", nil)
	sessionID := h.ValidateSession(req)

	if sessionID != "" {
		t.Error("Expected session to be invalid")
	}

	// Test with invalid cookie
	req.AddCookie(&http.Cookie{Name: "spaxel_session", Value: "invalid"})
	sessionID = h.ValidateSession(req)

	if sessionID != "" {
		t.Error("Expected session to be invalid")
	}
}

func TestHandler_Logout(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	h, err := NewHandler(Config{DB: db})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	// Setup and login
	reqBody := `{"pin": "1234"}`
	req := httptest.NewRequest("POST", "/api/auth/setup", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.handleSetup(w, req)

	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "spaxel_session" {
			sessionCookie = c
			break
		}
	}

	if sessionCookie == nil {
		t.Fatal("Session cookie not set")
	}

	// Logout
	req = httptest.NewRequest("POST", "/api/auth/logout", nil)
	req.AddCookie(sessionCookie)
	w = httptest.NewRecorder()
	h.handleLogout(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Check cookie was cleared
	cookies = w.Result().Cookies()
	var clearedCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "spaxel_session" {
			clearedCookie = c
			break
		}
	}

	if clearedCookie == nil || clearedCookie.MaxAge != -1 {
		t.Error("Expected cookie to be cleared (MaxAge=-1)")
	}

	// Verify session was deleted
	req = httptest.NewRequest("GET", "/api/test", nil)
	req.AddCookie(sessionCookie)
	sessionID := h.ValidateSession(req)

	if sessionID != "" {
		t.Error("Expected session to be invalid after logout")
	}
}

func TestPublicPaths(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/healthz", true},
		{"/api/auth/status", true},
		{"/api/auth/setup", true},
		{"/api/auth/login", true},
		{"/api/provision", true},
		{"/firmware/spaxel-1.0.0.bin", true},
		{"/api/settings", false},
		{"/api/nodes", false},
		{"/ws/dashboard", false},
		{"/ws/node", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := isPublicPath(tt.path)
			if result != tt.expected {
				t.Errorf("isPublicPath(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestInstallSecret_GeneratedOnFirstRun(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	h, err := NewHandler(Config{DB: db})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	// Verify secret was stored
	secret, err := h.GetInstallSecret()
	if err != nil {
		t.Fatal(err)
	}

	if len(secret) != 32 {
		t.Errorf("Expected 32-byte secret, got %d bytes", len(secret))
	}

	// Verify it's hex-encodable (all bytes)
	hexStr := hex.EncodeToString(secret)
	if len(hexStr) != 64 {
		t.Errorf("Expected 64-char hex string, got %d chars", len(hexStr))
	}
}

func TestInstallSecret_EnvVarOverride(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	knownSecret := "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2"
	t.Setenv("SPAXEL_INSTALL_SECRET", knownSecret)

	h, err := NewHandler(Config{DB: db})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	// Verify the stored secret matches the env var
	secret, err := h.GetInstallSecret()
	if err != nil {
		t.Fatal(err)
	}

	gotHex := hex.EncodeToString(secret)
	if gotHex != knownSecret {
		t.Errorf("Expected secret %q, got %q", knownSecret, gotHex)
	}
}

func TestInstallSecret_EnvVarInvalidHex(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	t.Setenv("SPAXEL_INSTALL_SECRET", "zzzz-invalid-hex")

	_, err = NewHandler(Config{DB: db})
	if err == nil {
		t.Fatal("Expected error for invalid hex in SPAXEL_INSTALL_SECRET")
	}
}

func TestInstallSecret_EnvVarWrongLength(t *testing.T) {
	tests := []struct {
		name   string
		secret string
	}{
		{"too short (8 bytes)", "a1b2c3d4e5f6a7b8"},
		{"too long (40 bytes)", "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := sql.Open("sqlite", ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()

			t.Setenv("SPAXEL_INSTALL_SECRET", tt.secret)

			_, err = NewHandler(Config{DB: db})
			if err == nil {
				t.Fatal("Expected error for wrong-length SPAXEL_INSTALL_SECRET")
			}
		})
	}
}

func TestInstallSecret_PersistedAcrossRestarts(t *testing.T) {
	// Use a temp file so the DB persists across handler instances
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}

	// First handler: generates secret
	h1, err := NewHandler(Config{DB: db})
	if err != nil {
		t.Fatal(err)
	}

	secret1, err := h1.GetInstallSecret()
	if err != nil {
		t.Fatal(err)
	}
	if len(secret1) != 32 {
		t.Fatalf("Expected 32-byte secret, got %d", len(secret1))
	}

	// Close first handler
	h1.Close()

	// Second handler: should load same secret from DB (no env var set)
	h2, err := NewHandler(Config{DB: db})
	if err != nil {
		t.Fatal(err)
	}
	defer h2.Close()

	secret2, err := h2.GetInstallSecret()
	if err != nil {
		t.Fatal(err)
	}

	if hex.EncodeToString(secret1) != hex.EncodeToString(secret2) {
		t.Error("Secret changed across handler restarts")
	}
}

func TestInstallSecret_NodeTokenDerivation(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	h, err := NewHandler(Config{DB: db})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	tests := []struct {
		name  string
		mac   string
		mac2  string
		same  bool // whether mac and mac2 should produce the same token
	}{
		{"same MAC always produces same token", "AA:BB:CC:DD:EE:FF", "AA:BB:CC:DD:EE:FF", true},
		{"different MACs produce different tokens", "AA:BB:CC:DD:EE:FF", "11:22:33:44:55:66", false},
		{"case insensitive", "aa:bb:cc:dd:ee:ff", "AA:BB:CC:DD:EE:FF", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token1, err := h.DeriveNodeToken(tt.mac)
			if err != nil {
				t.Fatal(err)
			}
			token2, err := h.DeriveNodeToken(tt.mac2)
			if err != nil {
				t.Fatal(err)
			}

			if tt.same && token1 != token2 {
				t.Errorf("Expected same token for %q and %q, got %q vs %q", tt.mac, tt.mac2, token1, token2)
			}
			if !tt.same && token1 == token2 {
				t.Errorf("Expected different tokens for %q and %q, got same %q", tt.mac, tt.mac2, token1)
			}
		})
	}
}

func TestHandleInstallSecret_FirstRun(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	h, err := NewHandler(Config{DB: db})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	// First run (no PIN configured): should return secret without auth
	req := httptest.NewRequest("GET", "/api/auth/install-secret", nil)
	w := httptest.NewRecorder()
	h.handleInstallSecret(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 on first run, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	if len(resp["install_secret"]) != 64 {
		t.Errorf("Expected 64-char hex secret, got %d chars", len(resp["install_secret"]))
	}
}

func TestHandleInstallSecret_AfterPINSet_Unauthorized(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	h, err := NewHandler(Config{DB: db})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	// Configure PIN
	reqBody := `{"pin": "1234"}`
	req := httptest.NewRequest("POST", "/api/auth/setup", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.handleSetup(w, req)

	// Request without auth: should be rejected
	req = httptest.NewRequest("GET", "/api/auth/install-secret", nil)
	w = httptest.NewRecorder()
	h.handleInstallSecret(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 after PIN set, got %d", w.Code)
	}
}

func TestHandleInstallSecret_AfterPINSet_Authorized(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	h, err := NewHandler(Config{DB: db})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	// Configure PIN and get session
	reqBody := `{"pin": "1234"}`
	req := httptest.NewRequest("POST", "/api/auth/setup", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.handleSetup(w, req)

	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "spaxel_session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("Session cookie not set after setup")
	}

	// Request with valid session: should succeed
	req = httptest.NewRequest("GET", "/api/auth/install-secret", nil)
	req.AddCookie(sessionCookie)
	w = httptest.NewRecorder()
	h.handleInstallSecret(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 with valid session, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	if len(resp["install_secret"]) != 64 {
		t.Errorf("Expected 64-char hex secret, got %d chars", len(resp["install_secret"]))
	}
}
