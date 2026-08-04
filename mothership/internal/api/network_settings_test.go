package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"
)

func newTestNetworkSettingsHandler(t *testing.T) (*NetworkSettingsHandler, chi.Router) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	t.Cleanup(func() { db.Close() }) //nolint:errcheck

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS settings (
			key         TEXT PRIMARY KEY,
			value_json  TEXT NOT NULL,
			updated_at  INTEGER NOT NULL DEFAULT 0
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create settings table: %v", err)
	}

	settingsHandler := NewSettingsHandler(db)
	h := NewNetworkSettingsHandler(settingsHandler)

	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return h, r
}

func TestNetworkSettingsHandler_GetEmpty(t *testing.T) {
	_, r := newTestNetworkSettingsHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/settings/network", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp networkSettingsResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.WifiSSID != "" {
		t.Errorf("expected empty wifi_ssid, got %q", resp.WifiSSID)
	}
	if resp.Configured {
		t.Errorf("expected configured=false with no settings stored")
	}
}

func TestNetworkSettingsHandler_PutThenGet(t *testing.T) {
	_, r := newTestNetworkSettingsHandler(t)

	ssid := "MyFleetNetwork"
	pass := "supersecret123"
	body, _ := json.Marshal(networkSettingsRequest{WifiSSID: &ssid, WifiPassword: &pass})

	req := httptest.NewRequest(http.MethodPut, "/api/settings/network", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var putResp networkSettingsResponse
	if err := json.NewDecoder(rr.Body).Decode(&putResp); err != nil {
		t.Fatalf("failed to decode PUT response: %v", err)
	}
	if putResp.WifiSSID != ssid {
		t.Errorf("expected wifi_ssid=%q, got %q", ssid, putResp.WifiSSID)
	}
	if !putResp.Configured {
		t.Errorf("expected configured=true after setting ssid+password")
	}

	// The password must never be echoed back.
	if bytes.Contains(rr.Body.Bytes(), []byte(pass)) {
		t.Errorf("response body must not contain the wifi password: %s", rr.Body.String())
	}

	// GET should reflect the same state.
	getReq := httptest.NewRequest(http.MethodGet, "/api/settings/network", nil)
	getRR := httptest.NewRecorder()
	r.ServeHTTP(getRR, getReq)

	var getResp networkSettingsResponse
	if err := json.NewDecoder(getRR.Body).Decode(&getResp); err != nil {
		t.Fatalf("failed to decode GET response: %v", err)
	}
	if getResp.WifiSSID != ssid || !getResp.Configured {
		t.Errorf("GET after PUT mismatch: got %+v", getResp)
	}
}

func TestNetworkSettingsHandler_PutRejectsEmptySSID(t *testing.T) {
	_, r := newTestNetworkSettingsHandler(t)

	empty := "   "
	body, _ := json.Marshal(networkSettingsRequest{WifiSSID: &empty})

	req := httptest.NewRequest(http.MethodPut, "/api/settings/network", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for blank ssid, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestNetworkSettingsHandler_PutRejectsShortPassword(t *testing.T) {
	_, r := newTestNetworkSettingsHandler(t)

	short := "1234567" // 7 chars, below WPA2 minimum of 8
	body, _ := json.Marshal(networkSettingsRequest{WifiPassword: &short})

	req := httptest.NewRequest(http.MethodPut, "/api/settings/network", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for short password, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestNetworkSettingsHandler_PutAllowsEmptyPasswordForOpenNetwork(t *testing.T) {
	_, r := newTestNetworkSettingsHandler(t)

	ssid := "OpenGuestNet"
	empty := ""
	body, _ := json.Marshal(networkSettingsRequest{WifiSSID: &ssid, WifiPassword: &empty})

	req := httptest.NewRequest(http.MethodPut, "/api/settings/network", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for empty password (open network), got %d: %s", rr.Code, rr.Body.String())
	}

	var resp networkSettingsResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	// An open network (empty password) is not "configured" in the sense of
	// having a password ready to hand to a node — SSID alone isn't enough
	// for the provisioning defaulting logic to treat it as complete.
	if resp.Configured {
		t.Errorf("expected configured=false when password is empty, got true")
	}
}

// TestNetworkSettingsHandler_SharesCacheWithSettingsHandler verifies that
// writes made through NetworkSettingsHandler are immediately visible via the
// underlying SettingsHandler.GetSingle — this is the mechanism the
// provisioning server relies on (ADR-005) to read fleet WiFi credentials
// without its own DB access or a second, potentially stale cache.
func TestNetworkSettingsHandler_SharesCacheWithSettingsHandler(t *testing.T) {
	h, r := newTestNetworkSettingsHandler(t)

	ssid := "SharedCacheNet"
	pass := "sharedcachepass"
	body, _ := json.Marshal(networkSettingsRequest{WifiSSID: &ssid, WifiPassword: &pass})

	req := httptest.NewRequest(http.MethodPut, "/api/settings/network", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	v, ok := h.settings.GetSingle(networkSettingWifiSSID)
	if !ok || v.(string) != ssid {
		t.Errorf("expected settingsHandler.GetSingle(%q) = %q, got %v (ok=%v)", networkSettingWifiSSID, ssid, v, ok)
	}
}
