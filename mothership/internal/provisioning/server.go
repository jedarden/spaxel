// Package provisioning handles the /api/provision endpoint used by the
// dashboard Web Serial onboarding wizard to generate per-node configuration blobs.
package provisioning

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// Payload is the provisioning blob written to the node's NVS via Web Serial.
type Payload struct {
	Version   int    `json:"version"`
	WifiSSID  string `json:"wifi_ssid"`
	WifiPass  string `json:"wifi_pass"`
	NodeID    string `json:"node_id"`
	NodeToken string `json:"node_token"`
	MsMDNS   string `json:"ms_mdns"`
	MsPort    int    `json:"ms_port"`
	NTPServer string `json:"ntp_server"`
	Debug     bool   `json:"debug"`
}

// provisionRequest is the optional POST body for /api/provision.
type provisionRequest struct {
	WifiSSID string `json:"wifi_ssid"`
	WifiPass string `json:"wifi_pass"`
	MAC      string `json:"mac,omitempty"` // optional; used to derive deterministic node_token
	Debug    bool   `json:"debug,omitempty"`
}

// Server handles provisioning payload generation.
type Server struct {
	mu            sync.RWMutex
	secretFile    string
	installSecret []byte // 32-byte HMAC key; persisted to secretFile
	mdnsName      string
	msPort        int
	ntpServer     string
}

// NewServer creates a provisioning server.
// dataDir is where the install secret is persisted.
// mdnsName and msPort are embedded in the payload so the node can find the mothership.
// ntpServer is the NTP server hostname to embed in the provisioning payload.
// installSecretHex is an optional 64-char hex string; if provided, it overrides the persisted secret.
func NewServer(dataDir, mdnsName string, msPort int, ntpServer string, installSecretHex string) *Server {
	s := &Server{
		secretFile: filepath.Join(dataDir, "install_secret.bin"),
		mdnsName:   mdnsName,
		msPort:     msPort,
		ntpServer:  ntpServer,
	}
	// If install secret provided via config, use it instead of loading/creating
	if installSecretHex != "" {
		decoded, err := hex.DecodeString(installSecretHex)
		if err == nil && len(decoded) == 32 {
			s.installSecret = decoded
			log.Printf("[INFO] provisioning: using install secret from SPAXEL_INSTALL_SECRET")
		} else {
			log.Printf("[WARN] provisioning: invalid SPAXEL_INSTALL_SECRET, will use persisted secret")
		}
	}
	if err := s.loadOrCreateSecret(); err != nil {
		log.Printf("[ERROR] provisioning: could not load/create install secret: %v", err)
	}
	return s
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// loadOrCreateSecret reads or generates the 32-byte install secret.
func (s *Server) loadOrCreateSecret() error {
	data, err := os.ReadFile(s.secretFile)
	if err == nil && len(data) == 32 {
		s.installSecret = data
		log.Printf("[INFO] provisioning: loaded install secret from %s", s.secretFile)
		return nil
	}

	// Generate a new secret
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return fmt.Errorf("generate secret: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(s.secretFile), 0700); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	if err := os.WriteFile(s.secretFile, secret, 0600); err != nil {
		return fmt.Errorf("write secret: %w", err)
	}
	s.installSecret = secret
	log.Printf("[INFO] provisioning: generated new install secret at %s", s.secretFile)
	return nil
}

// deriveToken computes HMAC-SHA256(installSecret, mac) and returns 64-char hex.
func (s *Server) deriveToken(mac string) string {
	mac = strings.ToUpper(strings.ReplaceAll(mac, ":", ""))
	h := hmac.New(sha256.New, s.installSecret)
	h.Write([]byte(mac))
	return hex.EncodeToString(h.Sum(nil))
}

// HandleProvision serves POST /api/provision.
//
// Request body (JSON, all optional):
//
//	{ "wifi_ssid": "...", "wifi_pass": "...", "mac": "AA:BB:CC:DD:EE:FF", "debug": false }
//
// Returns the provisioning payload to be written to NVS via Web Serial.
func (s *Server) HandleProvision(w http.ResponseWriter, r *http.Request) {
	var req provisionRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
	}

	s.mu.RLock()
	secret := s.installSecret
	s.mu.RUnlock()

	if secret == nil {
		http.Error(w, "provisioning not ready (no install secret)", http.StatusServiceUnavailable)
		return
	}

	nodeID := uuid.NewString()
	var token string
	if req.MAC != "" {
		token = s.deriveToken(req.MAC)
	} else {
		// Placeholder token — will be re-derived when the node sends hello with its MAC.
		// The ingestion server has a 120-second grace window.
		raw := make([]byte, 32)
		rand.Read(raw) //nolint:errcheck // best-effort placeholder
		token = hex.EncodeToString(raw)
	}

	payload := Payload{
		Version:    1,
		WifiSSID:   req.WifiSSID,
		WifiPass:   req.WifiPass,
		NodeID:     nodeID,
		NodeToken:  token,
		MsMDNS:     s.mdnsName,
		MsPort:     s.msPort,
		NTPServer:  s.ntpServer,
		Debug:      req.Debug,
	}

	log.Printf("[INFO] provisioning: generated payload node_id=%s mac=%s", nodeID, req.MAC)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(payload)
}
