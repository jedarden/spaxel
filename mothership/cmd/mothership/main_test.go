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
	sigproc "github.com/spaxel/mothership/internal/signal"
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

// TestTrackedBlobsToFleet locks the /api/blobs serve-frame contract: blobs
// leave the process fleet-frame (X=width, Y=depth, Z=height) while the fusion
// engine and its in-process consumers work Y-up. Velocities swap with their
// axes; ID, weight and identity fields pass through; the input slice — shared
// with the fusion loop — must be left untouched. See fusion.NodeFromFleet for
// the authority on the frame contract (spaxel-608a37be).
func TestTrackedBlobsToFleet(t *testing.T) {
	cases := []struct {
		name  string
		in    []sigproc.TrackedBlob
		wantX []float64 // expected per-blob (X, Y, Z)
		wantY []float64
		wantZ []float64
	}{
		{
			name:  "walker at height 1.7 m, depth 2.0 m",
			in:    []sigproc.TrackedBlob{{ID: 1, X: 2.5, Y: 1.7, Z: 2.0, Weight: 0.9, VX: 0.1, VY: 0.2, VZ: 0.3}},
			wantX: []float64{2.5},
			wantY: []float64{2.0}, // served Y is depth (engine Z)
			wantZ: []float64{1.7}, // served Z is height (engine Y)
		},
		{
			name:  "floor-level peak",
			in:    []sigproc.TrackedBlob{{ID: 2, X: 0, Y: 0, Z: 0}},
			wantX: []float64{0},
			wantY: []float64{0},
			wantZ: []float64{0},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := trackedBlobsToFleet(tc.in)
			if len(got) != len(tc.in) {
				t.Fatalf("len = %d, want %d", len(got), len(tc.in))
			}
			for i := range got {
				if got[i].X != tc.wantX[i] || got[i].Y != tc.wantY[i] || got[i].Z != tc.wantZ[i] {
					t.Errorf("blob %d pos = (%v, %v, %v), want (%v, %v, %v)",
						i, got[i].X, got[i].Y, got[i].Z, tc.wantX[i], tc.wantY[i], tc.wantZ[i])
				}
				if got[i].VX != tc.in[i].VX || got[i].VY != tc.in[i].VZ || got[i].VZ != tc.in[i].VY {
					t.Errorf("blob %d vel = (%v, %v, %v), want (%v, %v, %v)",
						i, got[i].VX, got[i].VY, got[i].VZ, tc.in[i].VX, tc.in[i].VZ, tc.in[i].VY)
				}
				if got[i].ID != tc.in[i].ID || got[i].Weight != tc.in[i].Weight {
					t.Errorf("blob %d ID/weight = (%d, %v), want (%d, %v)",
						i, got[i].ID, got[i].Weight, tc.in[i].ID, tc.in[i].Weight)
				}
			}
		})
	}

	t.Run("input slice not mutated", func(t *testing.T) {
		in := []sigproc.TrackedBlob{{ID: 7, X: 1, Y: 2, Z: 3, VY: 4, VZ: 5}}
		_ = trackedBlobsToFleet(in)
		if in[0].Y != 2 || in[0].Z != 3 || in[0].VY != 4 || in[0].VZ != 5 {
			t.Errorf("trackedBlobsToFleet mutated its input: %+v", in[0])
		}
	})
}
