// Package contract pins the mothership's REST/WebSocket wire contract.
//
// Unlike the per-handler unit tests in internal/, these tests wire the real
// handlers into a chi router exactly the way cmd/mothership/main.go does
// (sqlite settings store, real ProcessorManager, real provisioning/OTA/
// dashboard servers) and assert on the observable HTTP surface: status codes,
// JSON shapes, headers and error envelopes. The contract they encode is
// documented in docs/notes/api-contract.md — change both together.
package contract

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/spaxel/mothership/internal/api"
	"github.com/spaxel/mothership/internal/auth"
	"github.com/spaxel/mothership/internal/dashboard"
	"github.com/spaxel/mothership/internal/health"
	"github.com/spaxel/mothership/internal/ingestion"
	"github.com/spaxel/mothership/internal/ota"
	"github.com/spaxel/mothership/internal/provisioning"
	sigproc "github.com/spaxel/mothership/internal/signal"

	_ "modernc.org/sqlite"
)

const (
	testVersion = "contract-test"
	// Node identity used by the firmware download-auth and provisioning tests.
	testMAC       = "AA:BB:CC:DD:EE:FF"
	testNodeToken = "valid-node-token"
)

// rig is a fully wired mothership HTTP surface on a throwaway data dir.
type rig struct {
	t         *testing.T
	router    chi.Router
	srv       *httptest.Server // /ws tests dial this; REST tests go through router directly
	pm        *sigproc.ProcessorManager
	hub       *dashboard.Hub
	prov      *provisioning.Server
	settings  *api.SettingsHandler
	ota       *ota.Server
	ingestion *ingestion.Server
	fwDir     string
}

// newRig wires the contract-relevant handlers the way main.go does.
// demoMode mirrors auth.DemoModeMiddleware(cfg.DemoMode).
func newRig(t *testing.T, demoMode bool) *rig {
	t.Helper()
	rg := &rig{t: t}

	dataDir := t.TempDir()
	rg.fwDir = filepath.Join(dataDir, "firmware")
	if err := os.MkdirAll(rg.fwDir, 0o755); err != nil {
		t.Fatalf("mkdir firmware dir: %v", err)
	}

	db, err := sql.Open("sqlite", filepath.Join(dataDir, "spaxel.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS settings (
		key         TEXT PRIMARY KEY,
		value_json  TEXT NOT NULL,
		updated_at  INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		t.Fatalf("create settings table: %v", err)
	}

	rg.router = chi.NewRouter()
	rg.router.Use(middleware.Recoverer, auth.DemoModeMiddleware(demoMode))

	// Auth (also creates the auth/sessions tables on NewHandler).
	authHandler, err := auth.NewHandler(auth.Config{DB: db, DemoMode: demoMode})
	if err != nil {
		t.Fatalf("auth.NewHandler: %v", err)
	}
	authHandler.RegisterRoutes(rg.router)

	// Settings + network settings (ADR-005 fleet WiFi).
	rg.settings = api.NewSettingsHandler(db)
	rg.settings.RegisterRoutes(rg.router)
	api.NewNetworkSettingsHandler(rg.settings).RegisterRoutes(rg.router)

	// Fusion manager feeding /api/status (blob count, detection quality) and
	// the /api/blobs closure — same wiring as main.go.
	rg.pm = sigproc.NewProcessorManager(sigproc.ProcessorManagerConfig{
		NSub: 64, FusionRate: 10.0, Tau: 30.0,
	})

	// Status/occupancy.
	status := api.NewStatusHandler(time.Now(), func() int { return 2 }, testVersion)
	status.SetProcessorManager(rg.pm)
	status.RegisterRoutes(rg.router)

	// Tracked blobs — same closure main.go registers over the ProcessorManager.
	rg.router.Get("/api/blobs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rg.pm.GetTrackedBlobs())
	})

	// Provisioning (secret generated into the temp data dir).
	rg.prov = provisioning.NewServer(dataDir, "spaxel", 8080, "pool.ntp.org", "")
	rg.prov.SetSettingsProvider(rg.settings)
	rg.router.Post("/api/provision", rg.prov.HandleProvision)

	// Firmware list/serve with a token validator mirroring main.go's wiring.
	rg.ota = ota.NewServer(rg.fwDir)
	rg.ota.SetTokenValidator(func(mac, token string) bool {
		return mac == testMAC && token == testNodeToken
	})
	rg.ota.SetMigrationDeadline(time.Now().Add(time.Hour)) // tokenless grace window active
	rg.ota.Scan()                                          // HandleList serves the cached scan
	rg.router.Get("/api/firmware", rg.ota.HandleList)
	rg.router.Get("/firmware/{filename}", rg.ota.HandleServe)

	// Health (SELECT 1 against the temp sqlite DB).
	hc := health.New(health.Config{DB: db, GetNodeCount: func() int { return 1 }})
	rg.router.Get("/healthz", hc.Handler(testVersion))

	// Dashboard WebSocket (hub not running; WS tests start it).
	rg.hub = dashboard.NewHub(0)
	rg.router.Get("/ws/dashboard", dashboard.NewServer(rg.hub).HandleDashboardWS)

	// Node WebSocket ingestion (/ws/node) — same validator wiring as the OTA
	// server above (the production pairing is HMAC(installSecret, mac); the
	// rig pins the same accept/reject surface with a fixed pairing). The
	// migration deadline stays zero (strict mode); tests that exercise the
	// tokenless grace window call SetMigrationDeadline on the exposed server
	// before dialing. No fleet manager is wired, so a successful hello gets
	// the bare-server role/config defaults (rx, idle rate) — that shape is
	// itself part of the pinned contract.
	rg.ingestion = ingestion.NewServer()
	rg.ingestion.SetTokenValidator(func(mac, token string) bool {
		return mac == testMAC && token == testNodeToken
	})
	rg.router.Get("/ws/node", rg.ingestion.HandleNodeWS)

	rg.srv = httptest.NewServer(rg.router)
	t.Cleanup(rg.srv.Close)
	return rg
}

// req runs a request against the router and returns the recorder.
func (rg *rig) req(method, path string, body []byte, hdr map[string]string) *httptest.ResponseRecorder {
	rg.t.Helper()
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, rd)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rg.router.ServeHTTP(w, req)
	return w
}

func (rg *rig) get(path string, hdr map[string]string) *httptest.ResponseRecorder {
	return rg.req("GET", path, nil, hdr)
}

// post marshals body as JSON and POSTs it (body nil → empty request body).
func (rg *rig) post(path string, body any) *httptest.ResponseRecorder {
	rg.t.Helper()
	if body == nil {
		return rg.req("POST", path, nil, nil)
	}
	raw, err := json.Marshal(body)
	if err != nil {
		rg.t.Fatalf("marshal request body: %v", err)
	}
	return rg.req("POST", path, raw, map[string]string{"Content-Type": "application/json"})
}

func (rg *rig) put(path string, body any) *httptest.ResponseRecorder {
	rg.t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		rg.t.Fatalf("marshal request body: %v", err)
	}
	return rg.req("PUT", path, raw, map[string]string{"Content-Type": "application/json"})
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
		t.Fatalf("decode response body %q as %T: %v", w.Body.String(), out, err)
	}
}
