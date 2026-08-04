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

func TestHandleProvision_ErrorsWithNoCredentialsAvailable(t *testing.T) {
	s := newTestServer(t)
	// No SetSettingsProvider call at all — mirrors a deployment where it's
	// never wired, which must fail loudly rather than provision a dead node.

	rr, _ := doProvisionRequest(t, s, "")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when no wifi credentials available, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleProvision_ErrorsWhenProviderHasNoValue(t *testing.T) {
	s := newTestServer(t)
	s.SetSettingsProvider(&fakeSettingsProvider{values: map[string]interface{}{}})

	rr, _ := doProvisionRequest(t, s, "")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when provider has no stored ssid, got %d: %s", rr.Code, rr.Body.String())
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
