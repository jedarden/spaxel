package contract

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/spaxel/mothership/internal/auth"
	sigproc "github.com/spaxel/mothership/internal/signal"

	_ "modernc.org/sqlite"
)

// --- status & health ---------------------------------------------------------

func TestHealthzContract(t *testing.T) {
	rg := newRig(t, false)

	w := rg.get("/healthz", nil)
	if w.Code != 200 {
		t.Fatalf("status = %d, body %q", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	var got struct {
		Status        string `json:"status"`
		UptimeS       int64  `json:"uptime_s"`
		Version       string `json:"version"`
		NodesOnline   int    `json:"nodes_online"`
		DB            string `json:"db"`
		SheddingLevel int    `json:"shedding_level"`
		Reason        string `json:"reason"`
	}
	decodeBody(t, w, &got)
	if got.Status != "ok" {
		t.Errorf("status = %q, want ok", got.Status)
	}
	if got.Version != testVersion {
		t.Errorf("version = %q, want %q", got.Version, testVersion)
	}
	if got.NodesOnline != 1 {
		t.Errorf("nodes_online = %d, want 1", got.NodesOnline)
	}
	if got.DB != "ok" {
		t.Errorf("db = %q, want ok", got.DB)
	}
	if got.SheddingLevel != 0 {
		t.Errorf("shedding_level = %d, want 0", got.SheddingLevel)
	}
	if got.UptimeS < 0 {
		t.Errorf("uptime_s = %d, want >= 0", got.UptimeS)
	}
	if got.Reason != "" {
		t.Errorf("reason = %q on a healthy body, want absent", got.Reason)
	}
}

func TestStatusAndOccupancyContract(t *testing.T) {
	rg := newRig(t, false)
	rg.pm.SetTrackedBlobs([]sigproc.TrackedBlob{
		{ID: 1, X: 1, Z: 2, Weight: 0.5},
		{ID: 2, X: 3, Z: 4, Weight: 0.6},
	})

	w := rg.get("/api/status", nil)
	if w.Code != 200 {
		t.Fatalf("status = %d, body %q", w.Code, w.Body.String())
	}
	var got struct {
		Version          string `json:"version"`
		Nodes            int    `json:"nodes"`
		Blobs            int    `json:"blobs"`
		UptimeS          int64  `json:"uptime_s"`
		DetectionQuality int    `json:"detection_quality"`
	}
	decodeBody(t, w, &got)
	if got.Version != testVersion {
		t.Errorf("version = %q, want %q", got.Version, testVersion)
	}
	if got.Nodes != 2 {
		t.Errorf("nodes = %d, want 2", got.Nodes)
	}
	if got.Blobs != 2 {
		t.Errorf("blobs = %d, want 2 (tracked via ProcessorManager)", got.Blobs)
	}
	if got.UptimeS < 0 {
		t.Errorf("uptime_s = %d, want >= 0", got.UptimeS)
	}
	if got.DetectionQuality < 0 || got.DetectionQuality > 100 {
		t.Errorf("detection_quality = %d, want within [0,100]", got.DetectionQuality)
	}

	w = rg.get("/api/occupancy", nil)
	if w.Code != 200 {
		t.Fatalf("occupancy status = %d, body %q", w.Code, w.Body.String())
	}
	var occ map[string]struct {
		Count  int      `json:"count"`
		People []string `json:"people"`
	}
	decodeBody(t, w, &occ)
	if len(occ) != 0 {
		t.Errorf("occupancy = %v, want empty object with no zones manager", occ)
	}
}

// --- auth --------------------------------------------------------------------

// newAuthRig builds a fresh auth surface on its own DB so PIN state set in one
// test cannot leak into the next.
func newAuthRig(t *testing.T) chi.Router {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	h, err := auth.NewHandler(auth.Config{DB: db})
	if err != nil {
		t.Fatalf("auth.NewHandler: %v", err)
	}
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return r
}

func authReq(t *testing.T, r chi.Router, method, path, body, cookie string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "spaxel_session", Value: cookie})
	}
	r.ServeHTTP(w, req)
	return w
}

func TestAuthStatusFreshInstall(t *testing.T) {
	r := newAuthRig(t)
	w := authReq(t, r, "GET", "/api/auth/status", "", "")
	if w.Code != 200 {
		t.Fatalf("status = %d, body %q", w.Code, w.Body.String())
	}
	var got struct {
		PINConfigured bool `json:"pin_configured"`
		DemoMode      bool `json:"demo_mode"`
	}
	decodeBody(t, w, &got)
	if got.PINConfigured {
		t.Error("pin_configured = true on a fresh install, want false")
	}
	if got.DemoMode {
		t.Error("demo_mode = true, want false")
	}
}

func TestInstallSecretFirstRunAndAfterPIN(t *testing.T) {
	r := newAuthRig(t)

	// First run (no PIN): the install secret is retrievable for Web-Serial
	// onboarding bootstrap.
	w := authReq(t, r, "GET", "/api/auth/install-secret", "", "")
	if w.Code != 200 {
		t.Fatalf("first-run install-secret status = %d, body %q", w.Code, w.Body.String())
	}
	var got struct {
		InstallSecret string `json:"install_secret"`
	}
	decodeBody(t, w, &got)
	if len(got.InstallSecret) != 64 {
		t.Fatalf("install_secret = %d hex chars, want 64", len(got.InstallSecret))
	}
	if _, err := hex.DecodeString(got.InstallSecret); err != nil {
		t.Errorf("install_secret not hex: %v", err)
	}

	// Once a PIN exists the secret needs a session.
	if w := authReq(t, r, "POST", "/api/auth/setup", `{"pin":"1234"}`, ""); w.Code != 200 {
		t.Fatalf("setup status = %d, body %q", w.Code, w.Body.String())
	}
	w = authReq(t, r, "GET", "/api/auth/install-secret", "", "")
	if w.Code != 401 {
		t.Errorf("install-secret after PIN without session = %d, want 401", w.Code)
	}
}

func TestAuthSetupValidation(t *testing.T) {
	r := newAuthRig(t)
	cases := []struct {
		name string
		pin  string
	}{
		{"non-digit", "abcd"},
		{"too-short", "123"},
		{"too-long", "123456789"},
		{"empty", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := authReq(t, r, "POST", "/api/auth/setup", fmt.Sprintf(`{"pin":%q}`, tc.pin), "")
			if w.Code != 400 {
				t.Fatalf("pin %q: status = %d, want 400 (body %q)", tc.pin, w.Code, w.Body.String())
			}
		})
	}
}

func TestAuthSetupLoginChangePINLifecycle(t *testing.T) {
	r := newAuthRig(t)

	// Setup succeeds and mints a session cookie.
	w := authReq(t, r, "POST", "/api/auth/setup", `{"pin":"1234"}`, "")
	if w.Code != 200 {
		t.Fatalf("setup = %d, body %q", w.Code, w.Body.String())
	}
	var ok struct {
		OK string `json:"ok"`
	}
	decodeBody(t, w, &ok)
	if ok.OK != "true" {
		t.Errorf(`ok = %q, want "true" (string, not boolean)`, ok.OK)
	}
	var session string
	for _, c := range w.Result().Cookies() {
		if c.Name == "spaxel_session" && c.Value != "" {
			session = c.Value
		}
	}
	if session == "" {
		t.Fatal("no spaxel_session cookie on setup response")
	}

	// Re-setup is refused.
	if w := authReq(t, r, "POST", "/api/auth/setup", `{"pin":"5678"}`, ""); w.Code != 409 {
		t.Errorf("second setup = %d, want 409", w.Code)
	}

	// Status now reports a configured PIN.
	w = authReq(t, r, "GET", "/api/auth/status", "", "")
	var st struct {
		PINConfigured bool `json:"pin_configured"`
	}
	decodeBody(t, w, &st)
	if !st.PINConfigured {
		t.Error("pin_configured = false after setup, want true")
	}

	// Login: wrong PIN 401 (plain-text error), right PIN 200 + cookie.
	w = authReq(t, r, "POST", "/api/auth/login", `{"pin":"9999"}`, "")
	if w.Code != 401 {
		t.Errorf("wrong-PIN login = %d, want 401", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Invalid PIN") {
		t.Errorf("wrong-PIN body = %q, want to contain %q", w.Body.String(), "Invalid PIN")
	}

	w = authReq(t, r, "POST", "/api/auth/login", `{"pin":"1234"}`, "")
	if w.Code != 200 {
		t.Fatalf("login = %d, body %q", w.Code, w.Body.String())
	}

	// change-pin requires the session even when the old PIN is correct.
	w = authReq(t, r, "POST", "/api/auth/change-pin", `{"old_pin":"1234","new_pin":"5678"}`, "")
	if w.Code != 401 {
		t.Errorf("change-pin without session = %d, want 401", w.Code)
	}

	// With a session: rotate, old PIN dies, new PIN logs in.
	w = authReq(t, r, "POST", "/api/auth/change-pin", `{"old_pin":"1234","new_pin":"5678"}`, session)
	if w.Code != 200 {
		t.Fatalf("change-pin = %d, body %q", w.Code, w.Body.String())
	}
	if w := authReq(t, r, "POST", "/api/auth/login", `{"pin":"1234"}`, ""); w.Code != 401 {
		t.Errorf("old PIN still logs in after rotation = %d, want 401", w.Code)
	}
	if w := authReq(t, r, "POST", "/api/auth/login", `{"pin":"5678"}`, ""); w.Code != 200 {
		t.Errorf("new PIN login = %d, want 200", w.Code)
	}
}

// --- tracked blobs -----------------------------------------------------------

func TestBlobsContract(t *testing.T) {
	rg := newRig(t, false)
	resolved := true
	rg.pm.SetTrackedBlobs([]sigproc.TrackedBlob{{
		ID:                 3,
		X:                  1.25,
		Z:                  -2.4,
		VX:                 0.1,
		Weight:             0.87,
		PersonID:           "u-123",
		PersonLabel:        "alice",
		PersonColor:        "#aabbcc",
		IdentityConfidence: 0.9,
		IdentitySource:     "ble_triangulation",
		PersonName:         "alice",
		AssignedColor:      "#aabbcc",
		IdentityResolved:   &resolved,
	}})

	w := rg.get("/api/blobs", nil)
	if w.Code != 200 {
		t.Fatalf("status = %d, body %q", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}

	// Bare array — no envelope. Kinematic core uses the struct's Go field
	// names (no json tags), identity enrichment uses snake/camelCase tags.
	var blobs []map[string]any
	decodeBody(t, w, &blobs)
	if len(blobs) != 1 {
		t.Fatalf("got %d blobs, want 1 (body %s)", len(blobs), w.Body.String())
	}
	b := blobs[0]
	for key, want := range map[string]any{
		"ID": float64(3), "X": 1.25, "Z": -2.4, "Weight": 0.87,
		"person_id": "u-123", "person_label": "alice", "person_color": "#aabbcc",
		"identity_confidence": 0.9, "identity_source": "ble_triangulation",
		"personName": "alice", "assignedColor": "#aabbcc", "identityResolved": true,
	} {
		got, ok := b[key]
		if !ok {
			t.Errorf("blob missing key %q", key)
			continue
		}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("blob[%q] = %v, want %v", key, got, want)
		}
	}
	// Lowercase kinematic aliases must NOT appear — the REST contract is the
	// capitalized Go field names (the dashboard's lowercase blobJSON is a
	// WebSocket-only shape).
	for _, absent := range []string{"id", "x", "z", "weight", "blobs"} {
		if _, ok := b[absent]; ok {
			t.Errorf("blob has unexpected key %q", absent)
		}
	}
	if _, ok := b["posture"]; ok {
		t.Error("posture set although left empty — omitempty violated")
	}
}

func TestBlobsEmptyIsNullOrNotArrayButNeverEnvelope(t *testing.T) {
	rg := newRig(t, false)
	w := rg.get("/api/blobs", nil)
	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	body := strings.TrimSpace(w.Body.String())
	// encoding/json encodes a nil slice as `null`. That quirk is part of the
	// contract (documented in docs/notes/api-contract.md §4) — consumers must
	// null-check. If this ever becomes `[]`, update the doc and this test.
	if body != "null" && body != "[]" {
		t.Fatalf("empty tracked-blobs body = %q, want null or [] (never an envelope)", body)
	}
}

// --- provisioning ------------------------------------------------------------

type provisionPayload struct {
	Version   int    `json:"version"`
	WifiSSID  string `json:"wifi_ssid"`
	WifiPass  string `json:"wifi_pass"`
	NodeID    string `json:"node_id"`
	NodeToken string `json:"node_token"`
	MsMDNS    string `json:"ms_mdns"`
	MsIP      string `json:"ms_ip"`
	MsPort    int    `json:"ms_port"`
	NTPServer string `json:"ntp_server"`
	Debug     bool   `json:"debug"`
}

func TestProvisionEmptyBodyDefaults(t *testing.T) {
	rg := newRig(t, false)

	w := rg.post("/api/provision", nil)
	if w.Code != 200 {
		t.Fatalf("status = %d, body %q", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	var p provisionPayload
	decodeBody(t, w, &p)
	if p.Version != 1 {
		t.Errorf("version = %d, want 1", p.Version)
	}
	if p.NodeID == "" || len(strings.Split(p.NodeID, "-")) != 5 {
		t.Errorf("node_id = %q, want a UUID (5 dash-separated fields)", p.NodeID)
	}
	if len(p.NodeToken) != 64 {
		t.Errorf("tokenless node_token = %d chars, want 64 (random placeholder)", len(p.NodeToken))
	}
	if p.MsMDNS != "spaxel" {
		t.Errorf("ms_mdns = %q, want spaxel", p.MsMDNS)
	}
	if p.MsPort != 8080 {
		t.Errorf("ms_port = %d, want 8080", p.MsPort)
	}
	if p.NTPServer != "pool.ntp.org" {
		t.Errorf("ntp_server = %q, want pool.ntp.org", p.NTPServer)
	}
	if p.MsIP != "" {
		t.Errorf("ms_ip = %q, want absent/empty without an override", p.MsIP)
	}
}

func TestProvisionMacDerivesDeterministicHMACToken(t *testing.T) {
	rg := newRig(t, false)

	// Lowercase, colon-separated MAC must work (case/separator-insensitive).
	w := rg.post("/api/provision", map[string]any{"mac": "aa:bb:cc:dd:ee:ff"})
	if w.Code != 200 {
		t.Fatalf("status = %d, body %q", w.Code, w.Body.String())
	}
	var p provisionPayload
	decodeBody(t, w, &p)

	mac := hmac.New(sha256.New, rg.prov.GetInstallSecret())
	mac.Write([]byte("AABBCCDDEEFF"))
	want := hex.EncodeToString(mac.Sum(nil))
	if p.NodeToken != want {
		t.Errorf("node_token = %q, want HMAC-SHA256(installSecret, \"AABBCCDDEEFF\") = %q", p.NodeToken, want)
	}
	if !rg.prov.ValidateToken("AA:BB:CC:DD:EE:FF", p.NodeToken) {
		t.Error("ValidateToken rejected the token it just derived")
	}
	if rg.prov.ValidateToken("AA:BB:CC:DD:EE:FF", "deadbeef") {
		t.Error("ValidateToken accepted a bogus token")
	}

	// Same MAC → same token (deterministic), different node_id.
	w2 := rg.post("/api/provision", map[string]any{"mac": "AA:BB:CC:DD:EE:FF"})
	var p2 provisionPayload
	decodeBody(t, w2, &p2)
	if p2.NodeToken != p.NodeToken {
		t.Error("re-provisioning the same MAC produced a different token")
	}
	if p2.NodeID == p.NodeID {
		t.Error("node_id repeated across provisioning calls, want a fresh UUID")
	}
}

func TestProvisionWiFiDefaultsToSettingsRequestWins(t *testing.T) {
	rg := newRig(t, false)

	// No WiFi configured anywhere: empty credentials are legal.
	w := rg.post("/api/provision", nil)
	var p provisionPayload
	decodeBody(t, w, &p)
	if p.WifiSSID != "" || p.WifiPass != "" {
		t.Errorf("payload wifi = %q/%q with nothing configured, want empty", p.WifiSSID, p.WifiPass)
	}

	// Configure the fleet network (through the real PUT contract).
	if w := rg.put("/api/settings/network", map[string]any{
		"wifi_ssid": "fleet-net", "wifi_password": "fleet-password",
	}); w.Code != 200 {
		t.Fatalf("PUT network settings = %d, body %q", w.Code, w.Body.String())
	}

	// Omitted credentials fall back to the stored settings (ADR-005).
	w = rg.post("/api/provision", nil)
	decodeBody(t, w, &p)
	if p.WifiSSID != "fleet-net" || p.WifiPass != "fleet-password" {
		t.Errorf("payload wifi = %q/%q, want fleet defaults fleet-net/fleet-password", p.WifiSSID, p.WifiPass)
	}

	// An explicit request body overrides the fleet defaults.
	w = rg.post("/api/provision", map[string]any{
		"wifi_ssid": "override-net", "wifi_pass": "override-password",
	})
	decodeBody(t, w, &p)
	if p.WifiSSID != "override-net" || p.WifiPass != "override-password" {
		t.Errorf("payload wifi = %q/%q, want request override to win", p.WifiSSID, p.WifiPass)
	}

	// ms_ip override round-trips.
	w = rg.post("/api/provision", map[string]any{"ms_ip": "10.1.2.3"})
	decodeBody(t, w, &p)
	if p.MsIP != "10.1.2.3" {
		t.Errorf("ms_ip = %q, want 10.1.2.3", p.MsIP)
	}
}

func TestProvisionBadJSONIs400PlainText(t *testing.T) {
	rg := newRig(t, false)
	w := rg.req("POST", "/api/provision", []byte("{not json"), nil)
	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); strings.HasPrefix(ct, "application/json") {
		t.Errorf("error Content-Type = %q, want plain text (http.Error shape)", ct)
	}
	if body := w.Body.String(); !strings.Contains(body, "invalid JSON body") {
		t.Errorf("body = %q, want to contain %q", body, "invalid JSON body")
	}
}

// --- network settings --------------------------------------------------------

type networkSettings struct {
	WifiSSID   string `json:"wifi_ssid"`
	WifiPass   string `json:"wifi_password,omitempty"`
	Configured bool   `json:"configured"`
}

func TestNetworkSettingsGetPutContract(t *testing.T) {
	rg := newRig(t, false)

	// Fresh install: nothing configured, password never echoed.
	w := rg.get("/api/settings/network", nil)
	if w.Code != 200 {
		t.Fatalf("GET = %d, body %q", w.Code, w.Body.String())
	}
	var ns networkSettings
	decodeBody(t, w, &ns)
	if ns.WifiSSID != "" || ns.Configured {
		t.Errorf("fresh GET = %+v, want empty/unconfigured", ns)
	}
	if strings.Contains(w.Body.String(), "wifi_password") {
		t.Error("GET response leaks wifi_password key — password is write-only")
	}

	// Partial update: SSID only → not configured yet (no password).
	w = rg.put("/api/settings/network", map[string]any{"wifi_ssid": "fleet-net"})
	if w.Code != 200 {
		t.Fatalf("PUT ssid = %d, body %q", w.Code, w.Body.String())
	}
	decodeBody(t, w, &ns)
	if ns.WifiSSID != "fleet-net" || ns.Configured {
		t.Errorf("after SSID-only PUT = %+v, want ssid set, configured false", ns)
	}

	// Password set → configured.
	w = rg.put("/api/settings/network", map[string]any{"wifi_password": "fleet-password"})
	if w.Code != 200 {
		t.Fatalf("PUT password = %d, body %q", w.Code, w.Body.String())
	}
	decodeBody(t, w, &ns)
	if ns.WifiSSID != "fleet-net" || !ns.Configured {
		t.Errorf("after both set = %+v, want configured true and SSID intact", ns)
	}

	// GET still hides the password; recovery view exposes it.
	w = rg.get("/api/settings/network", nil)
	decodeBody(t, w, &ns)
	if ns.WifiPass != "" {
		t.Error("GET echoes wifi_password — must be write-only")
	}
	w = rg.get("/api/settings/network/recovery", nil)
	if w.Code != 200 {
		t.Fatalf("GET recovery = %d, body %q", w.Code, w.Body.String())
	}
	var rec networkSettings
	decodeBody(t, w, &rec)
	if rec.WifiSSID != "fleet-net" || rec.WifiPass != "fleet-password" {
		t.Errorf("recovery = %+v, want ssid+password", rec)
	}
}

func TestNetworkSettingsValidation(t *testing.T) {
	rg := newRig(t, false)
	longSSID := strings.Repeat("x", 33)
	cases := []struct {
		name    string
		body    map[string]any
		wantSub string
	}{
		{"empty ssid", map[string]any{"wifi_ssid": ""}, "wifi_ssid: must not be empty"},
		{"ssid over 32 chars", map[string]any{"wifi_ssid": longSSID}, "wifi_ssid: must be 32 characters or fewer"},
		{"short password", map[string]any{"wifi_password": "short"}, "wifi_password: must be at least 8 characters"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := rg.put("/api/settings/network", tc.body)
			if w.Code != 400 {
				t.Fatalf("status = %d, want 400 (body %q)", w.Code, w.Body.String())
			}
			var errBody struct {
				Error string `json:"error"`
			}
			decodeBody(t, w, &errBody)
			if !strings.Contains(errBody.Error, tc.wantSub) {
				t.Errorf("error = %q, want to contain %q", errBody.Error, tc.wantSub)
			}
		})
	}

	// Malformed JSON → 400 with the invalid-body message.
	w := rg.req("PUT", "/api/settings/network", []byte("]]"), nil)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "invalid request body") {
		t.Fatalf("malformed body: status %d body %q, want 400 invalid request body", w.Code, w.Body.String())
	}
}

// --- firmware ----------------------------------------------------------------

const firmwareBody = "fake spaxel firmware payload v1.2.4\n"

// seedFirmware writes the images the firmware tests serve and re-scans the
// OTA server (HandleList serves the cached scan result; only HandleServe
// re-scans on a miss).
func seedFirmware(t *testing.T, rg *rig) {
	t.Helper()
	files := map[string]string{
		"spaxel-v1.2.4.bin":   firmwareBody,
		"legacy-fallback.bin": "legacy image without a semver\n",
		"readme.txt":          "not firmware\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(rg.fwDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	rg.ota.Scan()
}

func TestFirmwareListContract(t *testing.T) {
	rg := newRig(t, false)
	seedFirmware(t, rg)

	// The list endpoint re-scans the dir on miss, but a GET right after
	// seeding must already reflect disk state.
	w := rg.get("/api/firmware", nil)
	if w.Code != 200 {
		t.Fatalf("status = %d, body %q", w.Code, w.Body.String())
	}
	var metas []struct {
		Filename   string    `json:"filename"`
		Version    string    `json:"version"`
		SHA256     string    `json:"sha256"`
		SizeBytes  int64     `json:"size_bytes"`
		IsLatest   bool      `json:"is_latest"`
		UploadedAt time.Time `json:"uploaded_at"`
	}
	decodeBody(t, w, &metas)

	byName := map[string]int{}
	for i, m := range metas {
		byName[m.Filename] = i
	}
	i, ok := byName["spaxel-v1.2.4.bin"]
	if !ok {
		t.Fatalf("versioned image missing from list: %+v", metas)
	}
	m := metas[i]
	sum := sha256.Sum256([]byte(firmwareBody))
	if m.Version != "1.2.4" {
		t.Errorf("version = %q, want 1.2.4 (parsed from filename)", m.Version)
	}
	if m.SHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("sha256 = %q, want digest of the on-disk image", m.SHA256)
	}
	if m.SizeBytes != int64(len(firmwareBody)) {
		t.Errorf("size_bytes = %d, want %d", m.SizeBytes, len(firmwareBody))
	}
	if !m.IsLatest {
		t.Error("is_latest = false for the only semver image, want true")
	}
	if m.UploadedAt.IsZero() {
		t.Error("uploaded_at unset")
	}

	if j, ok := byName["legacy-fallback.bin"]; ok {
		if metas[j].IsLatest {
			t.Error("legacy image advertised as latest; non-semver files must not be")
		}
	}
	for _, m := range metas {
		if m.Filename == "readme.txt" {
			t.Error("non-.bin file listed by /api/firmware")
		}
	}
}

func TestFirmwareDownloadContract(t *testing.T) {
	rg := newRig(t, false)
	seedFirmware(t, rg)

	nodeHdr := map[string]string{
		"X-Spaxel-MAC":   testMAC,
		"X-Spaxel-Token": testNodeToken,
	}

	// Valid node headers → raw image with checksum headers.
	w := rg.get("/firmware/spaxel-v1.2.4.bin", nodeHdr)
	if w.Code != 200 {
		t.Fatalf("download = %d, body %q", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != firmwareBody {
		t.Errorf("served body = %q, want the exact image bytes", got)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/octet-stream") {
		t.Errorf("Content-Type = %q, want application/octet-stream", ct)
	}
	sum := sha256.Sum256([]byte(firmwareBody))
	if got := w.Header().Get("X-SHA256"); got != hex.EncodeToString(sum[:]) {
		t.Errorf("X-SHA256 = %q, want image digest", got)
	}
	if w.Header().Get("X-Firmware-Version") == "" {
		t.Error("X-Firmware-Version header missing")
	}

	// Invalid token → 404, not 401 (no filename enumeration, ADR-006).
	badHdr := map[string]string{"X-Spaxel-MAC": testMAC, "X-Spaxel-Token": "wrong"}
	if w = rg.get("/firmware/spaxel-v1.2.4.bin", badHdr); w.Code != 404 {
		t.Errorf("invalid token = %d, want 404", w.Code)
	}
	// MAC/token pairing is checked, not just presence.
	mismatchHdr := map[string]string{"X-Spaxel-MAC": "11:22:33:44:55:66", "X-Spaxel-Token": testNodeToken}
	if w = rg.get("/firmware/spaxel-v1.2.4.bin", mismatchHdr); w.Code != 404 {
		t.Errorf("mismatched MAC = %d, want 404", w.Code)
	}

	// Tokenless request inside the migration window is still served.
	if w = rg.get("/firmware/spaxel-v1.2.4.bin", nil); w.Code != 200 {
		t.Errorf("tokenless within migration window = %d, want 200", w.Code)
	}

	// Unknown .bin → 404 (after a re-scan attempt); non-.bin → 404.
	if w = rg.get("/firmware/no-such-image.bin", nodeHdr); w.Code != 404 {
		t.Errorf("unknown image = %d, want 404", w.Code)
	}
	if w = rg.get("/firmware/readme.txt", nodeHdr); w.Code != 404 {
		t.Errorf("non-.bin path = %d, want 404", w.Code)
	}
	// Path traversal is neutralized.
	if w = rg.get("/firmware/..%2F..%2Fetc%2Fpasswd", nodeHdr); w.Code != 404 {
		t.Errorf("traversal path = %d, want 404", w.Code)
	}
}

// --- demo mode ---------------------------------------------------------------

func TestDemoModeBlocksMutations(t *testing.T) {
	rg := newRig(t, true)

	w := rg.post("/api/provision", nil)
	if w.Code != 403 {
		t.Fatalf("POST in demo mode = %d, want 403", w.Code)
	}
	var errBody struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	decodeBody(t, w, &errBody)
	if errBody.Error != "demo mode active" {
		t.Errorf("error = %q, want %q", errBody.Error, "demo mode active")
	}

	// Reads pass straight through.
	if w = rg.get("/healthz", nil); w.Code != 200 {
		t.Errorf("GET in demo mode = %d, want 200", w.Code)
	}
	if w = rg.get("/api/status", nil); w.Code != 200 {
		t.Errorf("GET /api/status in demo mode = %d, want 200", w.Code)
	}
}
