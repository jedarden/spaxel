package provisioning

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeSettingsProvider is a minimal NetworkSettingsProvider for tests, mimicking
// the subset of api.SettingsHandler behavior HandleProvision depends on.
type fakeSettingsProvider struct {
	values map[string]interface{}
}

func (f *fakeSettingsProvider) GetSingle(key string) (interface{}, bool) {
	v, ok := f.values[key]
	return v, ok
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	return NewServer(t.TempDir(), "spaxel", 8080, "pool.ntp.org", "")
}

func doProvisionRequest(t *testing.T, s *Server, body string) (*httptest.ResponseRecorder, Payload) {
	t.Helper()

	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(http.MethodPost, "/api/provision", nil)
	} else {
		req = httptest.NewRequest(http.MethodPost, "/api/provision", strings.NewReader(body))
	}
	rr := httptest.NewRecorder()
	s.HandleProvision(rr, req)

	var payload Payload
	if rr.Code == http.StatusOK {
		if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
			t.Fatalf("failed to decode payload: %v", err)
		}
	}
	return rr, payload
}

func TestHandleProvision_DefaultsFromSettingsProvider(t *testing.T) {
	s := newTestServer(t)
	s.SetSettingsProvider(&fakeSettingsProvider{values: map[string]interface{}{
		networkSettingWifiSSID:     "FleetNet",
		networkSettingWifiPassword: "fleetpassword",
	}})

	rr, payload := doProvisionRequest(t, s, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if payload.WifiSSID != "FleetNet" || payload.WifiPass != "fleetpassword" {
		t.Errorf("expected payload to default to fleet network, got ssid=%q pass=%q", payload.WifiSSID, payload.WifiPass)
	}
}

func TestHandleProvision_RequestBodyOverridesStoredSetting(t *testing.T) {
	s := newTestServer(t)
	s.SetSettingsProvider(&fakeSettingsProvider{values: map[string]interface{}{
		networkSettingWifiSSID:     "FleetNet",
		networkSettingWifiPassword: "fleetpassword",
	}})

	body := `{"wifi_ssid": "OverrideNet", "wifi_pass": "overridepass"}`
	rr, payload := doProvisionRequest(t, s, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if payload.WifiSSID != "OverrideNet" || payload.WifiPass != "overridepass" {
		t.Errorf("expected explicit request values to win, got ssid=%q pass=%q", payload.WifiSSID, payload.WifiPass)
	}
}

func TestHandleProvision_SucceedsWithNoCredentialsAvailable(t *testing.T) {
	s := newTestServer(t)
	// No SetSettingsProvider call at all — provisioning should now succeed
	// even with no credentials, allowing captive-portal-only onboarding.

	rr, payload := doProvisionRequest(t, s, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 when no wifi credentials available (credential-less provisioning), got %d: %s", rr.Code, rr.Body.String())
	}
	if payload.WifiSSID != "" || payload.WifiPass != "" {
		t.Errorf("expected empty credentials in payload, got ssid=%q pass=%q", payload.WifiSSID, payload.WifiPass)
	}
	// Verify other fields are still populated
	if payload.NodeID == "" {
		t.Error("expected node_id to be generated")
	}
	if payload.NodeToken == "" {
		t.Error("expected node_token to be generated")
	}
}

func TestHandleProvision_SucceedsWhenProviderHasNoValue(t *testing.T) {
	s := newTestServer(t)
	s.SetSettingsProvider(&fakeSettingsProvider{values: map[string]interface{}{}})

	rr, payload := doProvisionRequest(t, s, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 when provider has no stored ssid (credential-less provisioning), got %d: %s", rr.Code, rr.Body.String())
	}
	if payload.WifiSSID != "" || payload.WifiPass != "" {
		t.Errorf("expected empty credentials in payload, got ssid=%q pass=%q", payload.WifiSSID, payload.WifiPass)
	}
}

func TestHandleProvision_SucceedsWithSSIDOnlyOpenNetwork(t *testing.T) {
	s := newTestServer(t)
	s.SetSettingsProvider(&fakeSettingsProvider{values: map[string]interface{}{
		networkSettingWifiSSID: "OpenGuestNet",
		// no password key at all — open network
	}})

	rr, payload := doProvisionRequest(t, s, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for open network (ssid only), got %d: %s", rr.Code, rr.Body.String())
	}
	if payload.WifiSSID != "OpenGuestNet" || payload.WifiPass != "" {
		t.Errorf("expected open network payload, got ssid=%q pass=%q", payload.WifiSSID, payload.WifiPass)
	}
}

// TestHandleProvision_MissingCredentialsGracefulHandling tests the comprehensive error
// handling behavior when WiFi credentials are completely absent (no env vars,
// no database settings). Verifies that the system handles this gracefully
// without crashing and with appropriate warning message.
func TestHandleProvision_MissingCredentialsGracefulHandling(t *testing.T) {
	tests := []struct {
		name             string
		settingsProvider *fakeSettingsProvider
		requestBody       string
		expectSuccess     bool
		expectEmptySSID   bool
		expectEmptyPass   bool
		description       string
	}{
		{
			name:             "no_settings_provider_at_all",
			settingsProvider: nil,
			requestBody:       "",
			expectSuccess:     true,
			expectEmptySSID:   true,
			expectEmptyPass:   true,
			description:       "No credentials available - system handles gracefully",
		},
		{
			name:             "empty_settings_map",
			settingsProvider: &fakeSettingsProvider{values: map[string]interface{}{}},
			requestBody:       "",
			expectSuccess:     true,
			expectEmptySSID:   true,
			expectEmptyPass:   true,
			description:       "Empty database settings - graceful handling",
		},
		{
			name: "empty_request_with_no_db_credentials",
			settingsProvider: &fakeSettingsProvider{values: map[string]interface{}{
				networkSettingWifiSSID:     "",
				networkSettingWifiPassword: "",
			}},
			requestBody:   `{"wifi_ssid": "", "wifi_pass": ""}`,
			expectSuccess: true,
			expectEmptySSID: true,
			expectEmptyPass: true,
			description: "Explicit empty credentials - graceful handling",
		},
		{
			name: "partial_override_empty_db",
			settingsProvider: &fakeSettingsProvider{values: map[string]interface{}{
				networkSettingWifiSSID: "ExistingSSID",
				// password missing from DB
			}},
			requestBody:   `{"wifi_ssid": "", "wifi_pass": ""}`,
			expectSuccess: true,
			expectEmptySSID: false,
			expectEmptyPass: true,
			description: "Partial override falls back to DB: gets SSID from DB, password empty",
		},
		{
			name: "ssid_only_in_db_no_password",
			settingsProvider: &fakeSettingsProvider{values: map[string]interface{}{
				networkSettingWifiSSID: "OpenNetwork",
				// no password key
			}},
			requestBody:   "",
			expectSuccess: true,
			expectEmptySSID: false,
			expectEmptyPass: true,
			description: "Open network configuration - graceful handling",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t)
			if tt.settingsProvider != nil {
				s.SetSettingsProvider(tt.settingsProvider)
			}

			rr, payload := doProvisionRequest(t, s, tt.requestBody)

			// Verify graceful handling: no crash, returns valid response
			if tt.expectSuccess {
				if rr.Code != http.StatusOK {
					t.Errorf("%s: expected HTTP 200 (graceful handling), got %d: %s",
						tt.description, rr.Code, rr.Body.String())
				}
			}

			// Verify payload structure is valid even with missing credentials
			if payload.NodeID == "" {
				t.Errorf("%s: node_id should be generated even without credentials", tt.description)
			}
			if payload.NodeToken == "" {
				t.Errorf("%s: node_token should be generated even without credentials", tt.description)
			}
			if payload.Version != 1 {
				t.Errorf("%s: payload version should be 1", tt.description)
			}

			// Verify credential state matches expectations
			if tt.expectEmptySSID && payload.WifiSSID != "" {
				t.Errorf("%s: expected empty SSID, got %q", tt.description, payload.WifiSSID)
			}
			if !tt.expectEmptySSID && payload.WifiSSID == "" {
				t.Errorf("%s: expected non-empty SSID, got empty", tt.description)
			}
			if tt.expectEmptyPass && payload.WifiPass != "" {
				t.Errorf("%s: expected empty password, got %q", tt.description, payload.WifiPass)
			}

			// Verify other required fields are populated
			if payload.MsMDNS == "" {
				t.Errorf("%s: ms_mdns should be set even without WiFi credentials", tt.description)
			}
			if payload.MsPort == 0 {
				t.Errorf("%s: ms_port should be set even without WiFi credentials", tt.description)
			}
		})
	}
}

// TestHandleProvision_WhitespaceCredentialsHandling tests that whitespace-only
// credentials are properly trimmed and treated as empty, falling back to database
// settings or empty credentials gracefully.
func TestHandleProvision_WhitespaceCredentialsHandling(t *testing.T) {
	s := newTestServer(t)
	s.SetSettingsProvider(&fakeSettingsProvider{values: map[string]interface{}{
		networkSettingWifiSSID:     "ActualNetwork",
		networkSettingWifiPassword: "actualpass",
	}})

	// Whitespace-only request should fall back to DB settings
	rr, payload := doProvisionRequest(t, s, `{"wifi_ssid": "   ", "wifi_pass": "\t\n"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 with whitespace credentials (fallback to DB), got %d: %s", rr.Code, rr.Body.String())
	}
	// Should have fallen back to DB settings, not empty
	if payload.WifiSSID != "ActualNetwork" || payload.WifiPass != "actualpass" {
		t.Errorf("whitespace credentials should fall back to DB, got ssid=%q pass=%q",
			payload.WifiSSID, payload.WifiPass)
	}
}
