# Dashboard ↔ Mothership Integration

**Date:** 2026-09-03
**Bead:** spaxel-2dd43f31
**Scope:** How the `dashboard/` frontend is embedded into and communicates with the
`mothership/` Go backend, plus the components that surround that pairing. Every
path and line number below was read from the working tree on the date above.

---

## 1. How the dashboard is embedded into the binary

There is exactly **one** `//go:embed` directive for the dashboard, and it lives in a
build-tag-gated file rather than in `main.go`:

| File | Role |
|---|---|
| `mothership/cmd/mothership/dashboard_embed.go` | `//go:build embed` gated. `//go:embed dashboard` → `embeddedDashboardFS`, then an `init()` sets `dashboardFS = embeddedDashboardFS` and `dashboardEmbedded = true` |
| `mothership/cmd/mothership/main.go:80` | `var dashboardFS embed.FS` — the zero value used by non-embed builds |
| `mothership/cmd/mothership/main.go:84` | `var dashboardEmbedded = false` — flips to true only by the embed file's `init()` |

The gating matters: `//go:embed` cannot reference a directory that does not exist at
compile time, so the canonical repo-root `dashboard/` tree is **copied** into
`cmd/mothership/dashboard/` immediately before a tagged production build. That staging
directory is deliberately **not tracked** — `dashboard_embed.go`'s comment states that
untagged local builds use the filesystem fallback and therefore still serve the
canonical tree. `Dockerfile:44-46` performs the copy and `Dockerfile:52` passes
`-tags=embed`.

**Serving path** (`main.go:4921-4960`):

- Embed path: `fs.Sub(dashboardFS, "dashboard")` → `http.FileServer(http.FS(...))`
  registered as **both** `r.Get("/*", …)` and `r.Head("/*", …)`. HEAD was added
  because a GET-only route returned chi's 405 to `curl -sI` preflighters (bf-1cgqe).
- Fallback path (no `-tags=embed`): `cfg.StaticDir` if it exists, else
  `findDashboardDir()` — candidates `./dashboard`, `./../dashboard`, `/app/dashboard`,
  then an **executable-relative** walk up 8 parent levels looking for a `dashboard/`
  subdirectory (main.go:300-319). Resolved dir is served by
  `registerDashboardStatic()` (main.go:411).
- `/` (index) has no explicit route — it falls out of the `/*` catch-all file server.

**Named page routes** (main.go:4899-4918) go through `serveEmbeddedFile()`
(main.go:325), which prefers the embedded FS and falls back to disk per page:
`/ambient` → `ambient.html`, `/fleet` → `fleet.html`, `/live`, `/setup`, `/simple`.

**Guard test:** `mothership/cmd/mothership/dashboard_static_test.go` asserts the
dashboard assets are present/resolvable, so an embed regression fails the Go build
rather than surfacing as a blank page in the browser.

**HTML entry points** (all in `dashboard/`): `index.html`, `live.html`, `fleet.html`,
`ambient.html`, `simple.html`, `setup.html`, `integrations.html`, `simulator.html`,
plus `test-transformcontrols.html` (dev-only, not routed server-side).

---

## 2. WebSocket endpoints

Two WebSocket endpoints, on the single HTTP port (8080):

### 2.1 `GET /ws/dashboard` — browser-facing live feed

Registered at `main.go:4896` → `dashboard.NewServer(dashboardHub)` (constructed at
`main.go:1840`) → `internal/dashboard/server.go:63 HandleDashboardWS`.

- Library: `gorilla/websocket`. Upgrader allows **all origins** (`CheckOrigin` returns
  true, labelled "for development"), `ReadBufferSize: 256`, `WriteBufferSize: 4096`,
  per-client send buffer 1024 frames.
- Keepalive: 30 s ping / 60 s read deadline (`server.go:24-26`); the pong handler
  resets the deadline.
- Max concurrent clients is enforced at register time in `Hub.Run()` — an over-limit
  client has its send channel closed (i.e. dropped, not queued).

**Server → client protocol** (`internal/dashboard/hub.go`):

| Frame | When | Shape |
|---|---|---|
| `snapshot` | First frame on every connect, built by `buildSnapshot()` (hub.go:655) | `{"type":"snapshot","timestamp_ms":…,"nodes":…,"links":…,"motion_states":…,"ble_devices":…,"zones":…, …}` |
| delta | Every 100 ms tick (`tickDelta`, hub.go:750) | **No `type` field.** Carries only fields whose JSON changed vs the `snapshotCache`, plus `timestamp_ms` |
| health | 60 s ticker | system health (load level, goroutines, mem, bead count) |
| BLE scan | 5 s ticker | `devices` |
| typed events | on occurrence | `node_connected`/`node_disconnected` (`mac`, `firmware_version`, `chip`), `link_active`/`link_inactive` (`id`), raw CSI pass-through, `blob_explain` = `{"type":"blob_explain","blob_id":N,"snapshot":{…}}` (hub.go:252), zone/portal changes (`action`), security alerts, self-heal coverage `coverage_before`/`coverage_after`/`coverage_delta`/`mean_gdop_before`/`mean_gdop_after`, briefing |

The snapshot is sent **before** the client is added to the broadcast map
(hub.go:361-383), so no delta can race ahead of the initial state — the ordering
guarantee the frontend's snapshot-then-merge logic depends on.

**Client → server commands** (`server.go` `handleCommand`, from `server.go:126`):
`replay_seek`, `replay_play`, `replay_pause`, `replay_set_params`,
`replay_apply_to_live`. Sent from `dashboard/js/explainability.js:617` and
`dashboard/js/websocket.js:399`.

**Frontend clients** — five independent connection sites, not one shared helper:
`dashboard/js/websocket.js:134` (`SpaxelWebSocket`, the main live-view client),
`home-cards.js:260`, `simple-mode.js:16`, `onboard.js:1399`, `ambient.js:295`.
`websocket.js` owns reconnect backoff, the silent → dimmed → modal disconnect
progression, and blob-position extrapolation from last velocity during brief gaps.

### 2.2 `GET /ws/node` — ESP32 node connection (not the dashboard's)

Registered at `main.go:783` → `internal/ingestion/server.go:528 HandleNodeWS`. Listed
because the dashboard's fleet/links/diagnostics views are a *projection* of it:
binary CSI frames upstream plus JSON `hello`, `health`, `ble`, `motion_hint`,
`ota_status` (`internal/ingestion/message.go:156-180`); `role`/`config`/`ota`/`reboot`/
`identify`/`shutdown`/`reject` downstream. The ingestion server is also the hub's data
source — the hub's `IngestionState`, `BLEState` and `TriggerState` providers are fed
from it, so `/ws/node` traffic is what the dashboard eventually renders.

---

## 3. REST API surface used by the dashboard

**181 distinct route patterns** are registered across the mothership module. By file:

| Source | Routes |
|---|---|
| `cmd/mothership/main.go` | 92 (routes, healthz, provision, firmware, backup, doctor, blobs, weather, coverage, healing, links, diagnostics, security, mode, events/history, nodes actions) |
| `internal/api/simulator.go` | 22 |
| `internal/api/localization.go` | 17 |
| `internal/api/prediction.go` | 12 |
| `internal/api/replay.go` | 11 |
| `internal/api/zones.go` | 10 |
| `internal/api/volume_triggers.go` | 10 |
| `internal/api/briefing.go` | 9 |
| `internal/api/guided.go` | 8 |
| others (`triggers`, `security`, `notifications`, `alerts`, `settings`, `notification_settings`, `network_settings`, `integrations`, `events`) | 3-5 each |

Endpoint families the dashboard JS actually calls (grep over `dashboard/js/*.js`):
`/api/zones`, `/api/nodes` (+ per-node actions, `rebaseline-all`, `update-all`,
`virtual`), `/api/settings` (+ `/network`, `/notifications`, `/integration`, and
`/network/recovery`), `/api/people`, `/api/feedback`, `/api/simulator/{simulate,
gdop/heatmap,shopping-list}`, `/api/events`, `/api/ble/devices`, `/api/sleep*`,
`/api/replay/*` (REST control plane — note the WS carries the *interactive* replay
commands too), `/api/portals`, `/api/notifications/*`, `/api/links`,
`/api/learning/*`, `/api/export` + `/api/import`, `/api/guided/tooltip`,
`/api/diurnal/status`, `/api/analytics/corridors`, `/api/weather`, `/api/triggers`,
`/api/security/{arm,disarm}`, `/api/room`, `/api/recordings`, `/api/provision`,
`/api/mode`, `/api/firmware*`, `/api/diagnostics`, `/api/help_articles`.

### Authentication — current state (diverges from the plan)

The plan (`docs/plan/plan.md`, REST API Specification) states "session cookie required
on all `/api/*` endpoints" and "`/ws/dashboard` verifies the session cookie before
upgrading". **The code does not do this today.** Observed wiring:

- `internal/auth/handler.go:170 RegisterRoutes` registers
  `GET /api/auth/status`, `GET /api/auth/install-secret`, `POST /api/auth/setup`,
  `POST /api/auth/login`, `POST /api/auth/logout`,
  `POST /api/auth/change-pin`. Sessions are server-side rows in the `sessions` table
  bound to a `spaxel_session` cookie (handler.go:316, 526).
- `RequireAuth` middleware exists (handler.go:601) but is applied to exactly two
  routes: `/api/auth/change-pin` (handler.go:188, 200) and `/api/doctor`
  (main.go:5053). Nothing under `internal/api/` references it.
- The only global middleware is `middleware.Logger`, `middleware.Recoverer`, and
  `auth.DemoModeMiddleware(cfg.DemoMode)` (main.go:749-753). DemoMode is a
  write-blocker in demo mode, not an authentication gate.
- `HandleDashboardWS` (server.go:63) upgrades without consulting the session cookie.

So the PIN mechanism is implemented and functional as a *flow* (setup → login →
cookie → change-pin), but it is not enforced as a *boundary* over the API or the
dashboard WebSocket. Flagged here as an integration fact for whoever picks up
auth-enforcement work; it is not something this documentation bead changes.

**Deliberately unauthenticated** (by design, documented in the plan / ADR-006):
`/api/provision` (main.go:4836), `/firmware/{filename}` (ADR-006 node-token headers),
`/firmware/serial/{filename}` (main.go:4732 — serves exactly the one image named at
build, per main.go:5802, so the public first-flash path cannot become an arbitrary file
server), `/healthz`.

---

## 4. Related components

```
go.work  →  ./mothership   ./cmd/sim   ./test/acceptance
```

| Component | Path | Relationship to the dashboard |
|---|---|---|
| **Mothership backend** | `mothership/` (Go module, entry `cmd/mothership/main.go`) | Serves every dashboard page + `/ws/dashboard` + all REST routes. 55 packages under `internal/`, of which the dashboard-facing ones are `dashboard` (hub + WS server), `api` (REST handlers), `auth` (PIN/sessions), `events`, `explainability`, `guidedtroubleshoot`, `help`, `briefing`, `replay`, `simulator` |
| **Ingestion / fleet** | `internal/ingestion`, `internal/fleet` | Node WebSocket `/ws/node`, node registry, role assignment. Source of the node/link state the dashboard renders |
| **Pipeline & localization** | `internal/signal`, `internal/localizer`, `internal/fusion`, `internal/tracking` | 10 Hz fusion loop → hub broadcasts → dashboard blobs |
| **Firmware** | `firmware/` (ESP-IDF C, ESP32-S3) | Connects to `/ws/node`, not to the dashboard directly. The dashboard *does* touch it via Web Serial onboarding: `dashboard/js/esptool-bundle.js` + `/api/provision` + `/firmware/serial/{filename}` |
| **Dashboard frontend** | `dashboard/` | Vanilla JS + Three.js, no build toolchain. 85 JS files under `js/`, `css/`, `static/`, `sw.js` (service worker), `manifest.json` (PWA), `jest.config.js` + `playwright.config.js` (frontend tests) |
| **Simulator CLI** | `cmd/sim` (`spaxel-sim`) | Separate Go module; emulates nodes against `/ws/node`, exercising the same pipeline whose output the dashboard shows. Also drives `/api/blobs` for assertions |
| **Acceptance tests** | `test/acceptance`, `mothership/test/acceptance` | Consume the same WS/REST surface end to end (IO-1 … IO-11 scenarios in the plan) |
| **OTA subsystem** | `internal/ota`, `internal/autoupdate` | `/api/firmware*` + `/firmware/*` routes; `dashboard/js/ota.js` and the fleet page are the UI over it |

---

## 5. Integration points, summarised

1. **Embed** — `dashboard/` → `cmd/mothership/dashboard/` (Dockerfile copy) →
   `//go:embed` behind `-tags=embed` → `http.FileServer` on `/*`.
2. **Live state** — fusion/ingestion → `Hub` (providers set from `main.go`) →
   10 Hz delta frames on `/ws/dashboard` → `SpaxelWebSocket` → Three.js scene.
3. **Interactive control** — browser → `/ws/dashboard` JSON commands (replay,
   explainability) and → REST for everything else (zones, nodes, settings, triggers,
   firmware, provisioning).
4. **Node projection** — `/ws/node` carries node telemetry; the dashboard sees it only
   as hub-broadcast state, never by connecting to `/ws/node` itself.
5. **Onboarding** — browser Web Serial → `esptool-bundle.js` → `/api/provision` →
   `/firmware/serial/…` → device; the device then joins over `/ws/node`.
6. **Auth** — PIN flow at `/api/auth/*`; enforcement currently limited to
   `change-pin` and `doctor` (see §3).
