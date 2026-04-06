// Package auth provides PIN-based authentication and session management for the dashboard.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Handler handles authentication endpoints.
type Handler struct {
	db        *sql.DB
	secretKey []byte // for session token signing
}

// Config holds handler configuration.
type Config struct {
	DB        *sql.DB
	SecretKey []byte
}

// NewHandler creates a new auth handler.
func NewHandler(cfg Config) (*Handler, error) {
	if cfg.DB == nil {
		return nil, fmt.Errorf("database is required")
	}

	// Generate random secret key if not provided
	secretKey := cfg.SecretKey
	if len(secretKey) == 0 {
		secretKey = make([]byte, 32)
		if _, err := rand.Read(secretKey); err != nil {
			return nil, fmt.Errorf("generate secret key: %w", err)
		}
	}

	h := &Handler{
		db:        cfg.DB,
		secretKey: secretKey,
	}

	// Initialize auth schema and install secret
	if err := h.initializeAuth(); err != nil {
		return nil, fmt.Errorf("initialize auth: %w", err)
	}

	// Start session cleanup goroutine
	go h.cleanupExpiredSessions()

	return h, nil
}

// initializeAuth ensures the auth table has a singleton row and generates an install secret.
func (h *Handler) initializeAuth() error {
	// Check if auth table exists and has a row
	var count int
	err := h.db.QueryRow("SELECT COUNT(*) FROM auth").Scan(&count)
	if err != nil {
		// Table might not exist yet, create it
		_, err = h.db.Exec(`
			CREATE TABLE IF NOT EXISTS auth (
				id              INTEGER PRIMARY KEY CHECK (id = 1),
				install_secret  BLOB NOT NULL,
				pin_bcrypt      TEXT,
				updated_at      INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
			)
		`)
		if err != nil {
			return fmt.Errorf("create auth table: %w", err)
		}
	}

	// Create sessions table if it doesn't exist
	_, err = h.db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			session_id  TEXT PRIMARY KEY,
			created_at  INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000),
			expires_at  INTEGER NOT NULL,
			last_seen_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
		)
	`)
	if err != nil {
		return fmt.Errorf("create sessions table: %w", err)
	}

	// Create index on expires_at for efficient cleanup
	_, err = h.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at)
	`)
	if err != nil {
		return fmt.Errorf("create sessions index: %w", err)
	}

	// Check if we have an auth row
	err = h.db.QueryRow("SELECT COUNT(*) FROM auth WHERE id = 1").Scan(&count)
	if err != nil {
		return fmt.Errorf("check auth row: %w", err)
	}

	if count == 0 {
		// Generate install secret
		installSecret := make([]byte, 32)
		if _, err := rand.Read(installSecret); err != nil {
			return fmt.Errorf("generate install secret: %w", err)
		}

		// Insert auth row
		_, err = h.db.Exec(`
			INSERT INTO auth (id, install_secret, pin_bcrypt)
			VALUES (1, ?, NULL)
		`, installSecret)
		if err != nil {
			return fmt.Errorf("insert auth row: %w", err)
		}

		log.Printf("[INFO] Generated new install secret")
	}

	return nil
}

// RegisterRoutes registers auth routes with the given router.
func (h *Handler) RegisterRoutes(mux interface{ HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) }) {
	mux.HandleFunc("GET /api/auth/status", h.handleStatus)
	mux.HandleFunc("POST /api/auth/setup", h.handleSetup)
	mux.HandleFunc("POST /api/auth/login", h.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", h.handleLogout)
}

// handleStatus returns whether a PIN is configured.
// No authentication required.
func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	var pinBcrypt sql.NullString
	err := h.db.QueryRow("SELECT pin_bcrypt FROM auth WHERE id = 1").Scan(&pinBcrypt)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		log.Printf("[ERROR] Failed to check PIN status: %v", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{
		"pin_configured": pinBcrypt.Valid,
	})
}

// handleSetup sets a PIN on first run.
// No authentication required, but only works if PIN is not yet set.
func (h *Handler) handleSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if PIN is already configured
	var pinBcrypt sql.NullString
	err := h.db.QueryRow("SELECT pin_bcrypt FROM auth WHERE id = 1").Scan(&pinBcrypt)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	if pinBcrypt.Valid {
		http.Error(w, "PIN already configured", http.StatusConflict)
		return
	}

	// Parse request
	var req struct {
		PIN string `json:"pin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Validate PIN
	if len(req.PIN) < 4 || len(req.PIN) > 8 {
		http.Error(w, "PIN must be 4-8 digits", http.StatusBadRequest)
		return
	}

	// Ensure PIN is numeric
	for _, c := range req.PIN {
		if c < '0' || c > '9' {
			http.Error(w, "PIN must contain only digits", http.StatusBadRequest)
			return
		}
	}

	// Hash PIN with bcrypt (cost 12)
	hash, err := bcrypt.GenerateFromPassword([]byte(req.PIN), 12)
	if err != nil {
		http.Error(w, "Failed to hash PIN", http.StatusInternalServerError)
		log.Printf("[ERROR] Failed to hash PIN: %v", err)
		return
	}

	// Store hash
	_, err = h.db.Exec(`
		UPDATE auth
		SET pin_bcrypt = ?, updated_at = ?
		WHERE id = 1
	`, hash, time.Now().UnixMilli())
	if err != nil {
		http.Error(w, "Failed to store PIN", http.StatusInternalServerError)
		log.Printf("[ERROR] Failed to store PIN: %v", err)
		return
	}

	log.Printf("[INFO] PIN configured successfully")

	// Create session and set cookie
	sessionID, err := h.createSession()
	if err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		log.Printf("[ERROR] Failed to create session: %v", err)
		return
	}

	h.setSessionCookie(w, sessionID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
}

// handleLogin authenticates a user with their PIN.
// No authentication required.
func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request
	var req struct {
		PIN string `json:"pin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Get stored PIN hash
	var pinHash string
	err := h.db.QueryRow("SELECT pin_bcrypt FROM auth WHERE id = 1").Scan(&pinHash)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "PIN not configured", http.StatusNotFound)
		} else {
			http.Error(w, "Database error", http.StatusInternalServerError)
		}
		return
	}

	if pinHash == "" {
		http.Error(w, "PIN not configured", http.StatusNotFound)
		return
	}

	// Verify PIN
	if err := bcrypt.CompareHashAndPassword([]byte(pinHash), []byte(req.PIN)); err != nil {
		// Invalid PIN
		http.Error(w, "Invalid PIN", http.StatusUnauthorized)
		log.Printf("[WARN] Failed login attempt from %s", r.RemoteAddr)
		return
	}

	// Create session
	sessionID, err := h.createSession()
	if err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		log.Printf("[ERROR] Failed to create session: %v", err)
		return
	}

	h.setSessionCookie(w, sessionID)

	log.Printf("[INFO] Successful login from %s", r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
}

// handleLogout clears the session cookie and deletes the session.
// Authentication required.
func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get session ID from cookie
	cookie, err := r.Cookie("spaxel_session")
	if err == nil && cookie.Value != "" {
		// Delete session from database
		_, _ = h.db.Exec("DELETE FROM sessions WHERE session_id = ?", cookie.Value)
	}

	// Clear cookie by setting max-age to -1
	http.SetCookie(w, &http.Cookie{
		Name:     "spaxel_session",
		Value:    "",
		MaxAge:   -1,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})

	log.Printf("[INFO] Logout from %s", r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
}

// createSession creates a new session and returns the session ID.
func (h *Handler) createSession() (string, error) {
	// Generate 32-byte random session ID (64 hex chars)
	sessionBytes := make([]byte, 32)
	if _, err := rand.Read(sessionBytes); err != nil {
		return "", fmt.Errorf("generate session ID: %w", err)
	}
	sessionID := hex.EncodeToString(sessionBytes)

	// Calculate expiry (7 days from now)
	expiresAt := time.Now().Add(7 * 24 * time.Hour).UnixMilli()

	// Insert session
	_, err := h.db.Exec(`
		INSERT INTO sessions (session_id, created_at, expires_at, last_seen_at)
		VALUES (?, ?, ?, ?)
	`, sessionID, time.Now().UnixMilli(), expiresAt, time.Now().UnixMilli())
	if err != nil {
		return "", fmt.Errorf("insert session: %w", err)
	}

	return sessionID, nil
}

// setSessionCookie sets the session cookie on the response.
func (h *Handler) setSessionCookie(w http.ResponseWriter, sessionID string) {
	// Detect if we're using HTTPS
	isSecure := false // In production, check r.TLS != nil or X-Forwarded-Proto

	http.SetCookie(w, &http.Cookie{
		Name:     "spaxel_session",
		Value:    sessionID,
		MaxAge:   604800, // 7 days in seconds
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

// ValidateSession checks if a session is valid and extends it if near expiry.
// Returns the session ID if valid, empty string otherwise.
func (h *Handler) ValidateSession(r *http.Request) string {
	cookie, err := r.Cookie("spaxel_session")
	if err != nil || cookie.Value == "" {
		return ""
	}

	sessionID := cookie.Value

	// Check if session exists and is valid
	var expiresAt int64
	err = h.db.QueryRow(`
		SELECT expires_at FROM sessions WHERE session_id = ?
	`, sessionID).Scan(&expiresAt)
	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("[ERROR] Failed to validate session: %v", err)
		}
		return ""
	}

	// Check if expired
	now := time.Now().UnixMilli()
	if now > expiresAt {
		return ""
	}

	// Rolling session extension: if within 24h of expiry, extend by 7 days
	if expiresAt-now < 24*60*60*1000 {
		newExpiresAt := now + 7*24*60*60*1000
		_, err = h.db.Exec(`
			UPDATE sessions
			SET expires_at = ?, last_seen_at = ?
			WHERE session_id = ?
		`, newExpiresAt, now, sessionID)
		if err != nil {
			log.Printf("[WARN] Failed to extend session: %v", err)
		}
	} else {
		// Just update last_seen_at
		_, err = h.db.Exec(`
			UPDATE sessions SET last_seen_at = ? WHERE session_id = ?
		`, now, sessionID)
		if err != nil {
			log.Printf("[WARN] Failed to update last_seen_at: %v", err)
		}
	}

	return sessionID
}

// IsAuthenticated checks if the request is authenticated.
func (h *Handler) IsAuthenticated(r *http.Request) bool {
	return h.ValidateSession(r) != ""
}

// RequireAuth is middleware that requires authentication.
// Returns 401 if not authenticated.
func (h *Handler) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.IsAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// RequireAuthHandler wraps a standard http.Handler with authentication.
func (h *Handler) RequireAuthHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.IsAuthenticated(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// cleanupExpiredSessions runs periodically to delete expired sessions.
func (h *Handler) cleanupExpiredSessions() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		result, err := h.db.Exec(`
			DELETE FROM sessions WHERE expires_at < ?
		`, time.Now().UnixMilli())
		if err != nil {
			log.Printf("[ERROR] Failed to cleanup expired sessions: %v", err)
			continue
		}

		if rowsAffected, _ := result.RowsAffected(); rowsAffected > 0 {
			log.Printf("[INFO] Cleaned up %d expired sessions", rowsAffected)
		}
	}
}

// Close cleans up resources.
func (h *Handler) Close() error {
	// Nothing to clean up currently
	return nil
}

// GetInstallSecret retrieves the installation secret.
func (h *Handler) GetInstallSecret() ([]byte, error) {
	var secret []byte
	err := h.db.QueryRow("SELECT install_secret FROM auth WHERE id = 1").Scan(&secret)
	if err != nil {
		return nil, fmt.Errorf("get install secret: %w", err)
	}
	return secret, nil
}

// DeriveNodeToken derives a node token from the install secret and node MAC.
// Uses HMAC-SHA256(install_secret, mac) for secure token derivation.
func (h *Handler) DeriveNodeToken(mac string) (string, error) {
	secret, err := h.GetInstallSecret()
	if err != nil {
		return "", err
	}

	// Normalize MAC to uppercase without colons
	mac = strings.ToUpper(strings.ReplaceAll(mac, ":", ""))

	// Compute HMAC-SHA256(install_secret, mac)
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(mac))
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ValidateNodeToken checks if a node token is valid.
// Returns true if the token matches the expected HMAC-SHA256(install_secret, mac).
func (h *Handler) ValidateNodeToken(mac, token string) bool {
	secret, err := h.GetInstallSecret()
	if err != nil {
		log.Printf("[ERROR] Failed to get install secret for token validation: %v", err)
		return false
	}

	// Normalize MAC to uppercase without colons
	mac = strings.ToUpper(strings.ReplaceAll(mac, ":", ""))

	// Compute expected token
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(mac))
	expectedToken := hex.EncodeToString(h.Sum(nil))

	// Use constant-time comparison to prevent timing attacks
	return subtle.ConstantTimeCompare([]byte(expectedToken), []byte(token)) == 1
}

// GetInstallSecretForNodes returns the install secret for use by node validation.
// This is used by the ingestion server to validate node tokens.
func (h *Handler) GetInstallSecretForNodes() ([]byte, error) {
	return h.GetInstallSecret()
}

// Helper function to check if a path should be excluded from auth
func isPublicPath(path string) bool {
	publicPaths := []string{
		"/healthz",
		"/api/auth/status",
		"/api/auth/setup",
		"/api/auth/login",
		"/api/provision",
	}

	for _, pp := range publicPaths {
		if path == pp {
			return true
		}
	}

	// Firmware is served without auth (URL contains SHA256 for integrity)
	if len(path) > 10 && path[:10] == "/firmware/" {
		return true
	}

	return false
}
