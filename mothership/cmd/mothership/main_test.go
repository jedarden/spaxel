package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/spaxel/mothership/internal/api"
	appconfig "github.com/spaxel/mothership/internal/config"
	"github.com/spaxel/mothership/internal/provisioning"
	_ "modernc.org/sqlite"
)

func newSeedSettingsHandler(t *testing.T) *api.SettingsHandler {
	t.Helper()

	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatalf("open settings database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`
		CREATE TABLE settings (
			key TEXT PRIMARY KEY,
			value_json TEXT NOT NULL,
			updated_at INTEGER NOT NULL DEFAULT 0
		)
	`); err != nil {
		t.Fatalf("create settings table: %v", err)
	}

	return api.NewSettingsHandler(db)
}

func settingString(t *testing.T, settings *api.SettingsHandler, key string) (string, bool) {
	t.Helper()

	value, ok := settings.GetSingle(key)
	if !ok {
		return "", false
	}
	stringValue, ok := value.(string)
	if !ok {
		t.Fatalf("setting %q has type %T, want string", key, value)
	}
	return stringValue, true
}

func TestSeedWiFiCredentialsIfFirstBoot(t *testing.T) {
	const (
		ssidKey     = "network_wifi_ssid"
		passwordKey = "network_wifi_password"
		envSSID     = "EnvFleet"
		envPassword = "env-passphrase"
	)

	tests := []struct {
		name               string
		initialSSID        *string
		initialPassword    *string
		wantSSID           *string
		wantPassword       *string
		wantSeedOnFirstRun bool
	}{
		{
			name:               "fresh database seeds both values",
			wantSSID:           stringPtr(envSSID),
			wantPassword:       stringPtr(envPassword),
			wantSeedOnFirstRun: true,
		},
		{
			name:            "existing values are preserved",
			initialSSID:     stringPtr("StoredFleet"),
			initialPassword: stringPtr("stored-passphrase"),
			wantSSID:        stringPtr("StoredFleet"),
			wantPassword:    stringPtr("stored-passphrase"),
		},
		{
			name:        "existing SSID prevents partial overwrite",
			initialSSID: stringPtr("StoredFleet"),
			wantSSID:    stringPtr("StoredFleet"),
		},
		{
			name:            "existing password prevents partial overwrite",
			initialPassword: stringPtr("stored-passphrase"),
			wantPassword:    stringPtr("stored-passphrase"),
		},
		{
			name:        "existing empty value is still authoritative",
			initialSSID: stringPtr(""),
			wantSSID:    stringPtr(""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := newSeedSettingsHandler(t)
			if tt.initialSSID != nil {
				if err := settings.Set(ssidKey, *tt.initialSSID); err != nil {
					t.Fatalf("set initial SSID: %v", err)
				}
			}
			if tt.initialPassword != nil {
				if err := settings.Set(passwordKey, *tt.initialPassword); err != nil {
					t.Fatalf("set initial password: %v", err)
				}
			}

			seedWiFiCredentialsIfFirstBoot(settings, envSSID, envPassword)

			gotSSID, hasSSID := settingString(t, settings, ssidKey)
			if tt.wantSSID == nil {
				if hasSSID {
					t.Fatalf("unexpected SSID %q", gotSSID)
				}
			} else if !hasSSID || gotSSID != *tt.wantSSID {
				t.Fatalf("SSID = (%q, %t), want (%q, true)", gotSSID, hasSSID, *tt.wantSSID)
			}

			gotPassword, hasPassword := settingString(t, settings, passwordKey)
			if tt.wantPassword == nil {
				if hasPassword {
					t.Fatalf("unexpected password %q", gotPassword)
				}
			} else if !hasPassword || gotPassword != *tt.wantPassword {
				t.Fatalf("password = (%q, %t), want (%q, true)", gotPassword, hasPassword, *tt.wantPassword)
			}

			// A second invocation with different env values must be a no-op too.
			seedWiFiCredentialsIfFirstBoot(settings, "ChangedFleet", "changed-passphrase")
			gotSSID, _ = settingString(t, settings, ssidKey)
			gotPassword, _ = settingString(t, settings, passwordKey)
			if tt.wantSeedOnFirstRun {
				if gotSSID != envSSID || gotPassword != envPassword {
					t.Fatalf("second boot changed seeded values: SSID=%q password=%q", gotSSID, gotPassword)
				}
			}
		})
	}
}

func TestWiFiCredentialsFromEnvironmentReachProvisioning(t *testing.T) {
	const (
		envSSID        = "EnvProvisioningNetwork"
		envPassword    = "env-provisioning-pass"
		mDNSName       = "spaxel"
		mothershipPort = 8080
		ntpServer      = "pool.ntp.org"
	)

	t.Setenv("SPAXEL_WIFI_SSID", envSSID)
	t.Setenv("SPAXEL_WIFI_PASSWORD", envPassword)

	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatalf("load config from environment: %v", err)
	}
	if cfg.WifiSSID != envSSID || cfg.WifiPassword != envPassword {
		t.Fatalf("config WiFi credentials = (%q, %q), want (%q, %q)",
			cfg.WifiSSID, cfg.WifiPassword, envSSID, envPassword)
	}

	settings := newSeedSettingsHandler(t)
	seedWiFiCredentialsIfFirstBoot(settings, cfg.WifiSSID, cfg.WifiPassword)

	prov := provisioning.NewServer(t.TempDir(), mDNSName, mothershipPort, ntpServer, "")
	prov.SetSettingsProvider(settings)
	req := httptest.NewRequest(http.MethodPost, "/api/provision", nil)
	rec := httptest.NewRecorder()
	prov.HandleProvision(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("provisioning status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var payload provisioning.Payload
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode provisioning payload: %v", err)
	}
	if payload.WifiSSID != envSSID || payload.WifiPass != envPassword {
		t.Fatalf("provisioning WiFi credentials = (%q, %q), want (%q, %q)",
			payload.WifiSSID, payload.WifiPass, envSSID, envPassword)
	}
}

func stringPtr(value string) *string {
	return &value
}
