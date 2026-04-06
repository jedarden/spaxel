// Package auth provides authentication tests.
package auth

import (
	"database/sql"
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
