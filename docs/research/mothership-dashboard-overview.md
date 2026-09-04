# Mothership Dashboard — The Complete Picture

**Date:** 2026-09-04
**Bead:** spaxel-6dbba42d
**Method:** synthesis of the three preceding survey documents, with every load-bearing claim
re-verified against the live tree on the date above (`ls dashboard/*.html`, `grep` on
`cmd/mothership/main.go`, `Dockerfile`, `go.work`, `dashboard/package.json`, `sw.js`, and the
page→script references). Where the source surveys disagreed with each other, the conflict was
re-settled by direct grep and the resolved fact is what appears here (see §7).

This is the one document to read for "how is the Mothership dashboard structured and how do I
run it." Depth lives in the series:

| Question | Detail document |
|---|---|
| What is every directory and file? | [`mothership-directory-structure.md`](mothership-directory-structure.md) |
| Where does execution start, what routes exist? | [`mothership-dashboard-entry-points.md`](mothership-dashboard-entry-points.md) |
| How do I build/start it, what configures it? | [`mothership-build-and-configuration.md`](mothership-build-and-configuration.md) |

---

## TL;DR — the mental model

- Mothership is a **single Go binary** (`mothership/cmd/mothership/main.go`) that serves the
  REST API, one WebSocket, **and** the dashboard from the same port (8080). There is **no Node
  server, no bundler, and no client build step**.
- The dashboard is a **flat, no-build vanilla-JS app**: 9 HTML pages each hand-pick plain
  `<script src>` tags from one flat `js/` (85 files) and one flat `css/` (27 files) directory.
  There is no `src/`, `components/`, or `utils/`; "components" are per-feature `js`+`css` pairs
  of the same name.
- The UI reaches the browser two ways: **embedded** into the binary via `go:embed` under the
  `embed` build tag (production, what the Dockerfile builds), or **served from disk**
  (untagged local builds — `findDashboardDir()` candidates). Same URL space either way.
- **Production start is `docker compose up -d`** (compose builds the image locally, host
  networking for mDNS). **Local dev start is `cd mothership && go run ./cmd/mothership`.**
  There is no `npm start` — `dashboard/package.json` exists purely for tests.
- Configuration is **environment variables only**, parsed by
  `mothership/internal/config/config.go` (the single authority). No `.env`, no per-service
  config file; every `${VAR}` in `docker-compose.yml` has an in-file default.
- The repo is **three Go modules stitched by `go.work`** (`mothership/`, `cmd/sim/`,
  `test/acceptance/`) — there is **no root `go.mod`**, and root-level `go build ./...` fails.
- Version flows from the root `VERSION` file → Docker build arg → `-ldflags -X main.version` →
  `/healthz`, `/api/status`, and the firmware artifact filename. CI auto-bumps it.
- The dashboard depends on **Three.js r128 from a CDN at runtime** for `live.html` and
  `simulator.html`; the service worker's offline shell covers the home page only, so the expert
  page is not usable offline.

---

## 1. Project structure — annotated tree

```
spaxel/
├── go.work                  # stitches ./mothership, ./cmd/sim, ./test/acceptance (no root go.mod)
├── VERSION                  # release version, single source of truth (currently 0.2.149; CI auto-bumps)
├── Dockerfile               # 3 stages: alpine firmware-fetcher → golang:1.25 builder → distroless nonroot
├── docker-compose.yml       # THE deployment manifest — builds locally, host networking, /data volume
├── .golangci.yml            # lint gate (v2 config; run it from mothership/, never repo root)
│
├── mothership/              # ── Go module 1: the backend (365 .go files)
│   ├── go.mod               #    github.com/spaxel/mothership, Go 1.25, pure-Go sqlite (no CGO)
│   ├── cmd/mothership/      #    THE BINARY: main.go (func main() at :703), dashboard_embed.go,
│   │                        #    migrate.go, mdns_binding.go + tests
│   ├── cmd/sim/             #    the node simulator that ships in the image (separate from root cmd/sim)
│   └── internal/            #    56 domain packages: api (48 files), ingestion (/ws/node),
│                            #    fleet, signal, simulator, localization, ota, auth, config, db, …
│
├── dashboard/               # ── the web UI — embedded via go:embed, or served from disk in dev
│   ├── index.html           # 9 HTML entry points, each hand-picking its scripts:
│   ├── live.html            #   home · expert 3D (52 <script> tags, largest page) · fleet ·
│   ├── fleet.html  simple.html  ambient.html  setup.html
│   ├── simulator.html  integrations.html  test-transformcontrols.html
│   ├── js/                  # 85 flat files: 56 app modules (app.js, viz3d.js, websocket.js, …),
│   │                        #   20 co-located jest tests (*.test.js — jest's own testMatch),
│   │                        #   vendored esptool-bundle.js, and a node-CLI leak-profiling
│   │                        #   harness (PROFILING.md)
│   ├── css/                 # 27 flat stylesheets: tokens.css + layout.css (shared) + one per feature
│   ├── static/              # served verbatim under /static/ — icons/ (PWA) and css/mobile.css are
│   │                        #   live; css/fleet-page.css and js/fleet.js are dead copies (§7)
│   ├── tests/               # Playwright a11y specs ONLY (unit tests live beside their code in js/)
│   ├── types/               # ambient TS overlay for `tsc --noEmit`; the runtime app is plain JS
│   ├── manifest.json  sw.js # PWA shell ('spaxel-dashboard-v1', precaches the home-page shell only)
│   ├── help_articles.json   # content for the help / guided-troubleshooting UI
│   └── package.json         # TEST TOOLING ONLY: test (jest), test:a11y (playwright), typecheck (tsc)
│
├── cmd/sim/                 # ── Go module 2: standalone simulator CLI (dev tool, not product entry)
├── test/acceptance/         # ── Go module 3: AS-1…AS-7 acceptance suite (gated by env in CI)
├── firmware/                # ESP32-S3 ESP-IDF project (C): CMakeLists.txt, sdkconfig.defaults,
│                            #   sdkconfig.{usbjtag,uart-console}, partitions.csv (true A/B), test/Makefile
├── tests/e2e/run.sh         # IO-6 shell e2e harness (mothership + spaxel-sim, ephemeral port)
├── docs/                    # plan/plan.md (architecture), notes/, research/ (this series), design/
└── data/                    # runtime state: spaxel.db, floor plans, CSI replay buffer, /data/firmware OTA store
```

Read the full per-file map in [`mothership-directory-structure.md`](mothership-directory-structure.md).

---

## 2. Entry points

"Entry point" means three different things here, because there is no client build:

| Generic concept | What it is in Spaxel |
|---|---|
| `server.js` (server entry) | `mothership/cmd/mothership/main.go` — `func main()` at line **703**; the only process entry point |
| `index.html` (page entries) | 9 static HTML pages in `dashboard/` |
| `main.js` / `app.tsx` (app init) | Per-page bootstrap modules in `dashboard/js/` — **`js/app.js`** is the main application file |

### 2.1 Server startup (the parts that matter to the dashboard)

1. `func main()` (`main.go:703`) → config load, then middleware:
   `middleware.Logger` → `middleware.Recoverer` → `auth.DemoModeMiddleware`.
2. `go dashboardHub.Run()` + `r.HandleFunc("/ws/dashboard", …)` (`main.go:4896`) — the
   dashboard's single live data channel.
3. Five short page routes registered **before** the `/*` catch-all:
   `/ambient`, `/fleet`, `/live`, `/setup`, `/simple`.
4. The `/*` catch-all serves everything else from the embedded FS (production) or the
   dashboard directory (dev). `/` resolves to `index.html` through it; `/index.html` 301s to
   `./`. Static routes answer **GET and HEAD** (regression-pinned by `TestDashboardStaticAssets`).

**Two serving modes, one URL space.** `dashboard_embed.go` declares `//go:build embed` +
`//go:embed dashboard`; the Dockerfile copies repo-root `dashboard/` →
`cmd/mothership/dashboard/` right before the tagged build. Without the tag the binary serves
from disk via `SPAXEL_STATIC_DIR` or `findDashboardDir()` candidates (`./dashboard`,
`./../dashboard`, `/app/dashboard`, executable-relative). They diverge on misses: embedded
**404s** an unknown extension-less path, the dev filesystem handler falls back to
`index.html` with a **200** — the SPA-style fallback is dev-only, and it silently swallows
typos.

**No server-side page auth.** Only logging/recovery + `DemoModeMiddleware` sit in front of the
page routes; the PIN gate is client-side (`js/auth.js`, ambient mode) and data-level
enforcement lives on `/ws/dashboard`.

### 2.2 The nine pages

| Page | URL | Served by | Purpose |
|---|---|---|---|
| `index.html` | `/` | static catch-all | Home: status banner + three cards |
| `live.html` | `/live` | `r.Get` | Expert 3D view — the app proper (largest page) |
| `simple.html` | `/simple` | `r.Get` | Mobile-first card UI, expert-PIN gate |
| `fleet.html` | `/fleet` | `r.Get` | Fleet health table |
| `setup.html` | `/setup` | `r.Get` | Placement, floor-plan and zone editor |
| `ambient.html` | `/ambient` | `r.Get` | Headless ambient display (TV/kiosk) |
| `simulator.html` | `/simulator.html` | static catch-all | Node simulator UI — **no short route** |
| `integrations.html` | `/integrations.html` | static catch-all | Integrations status — **no short route** |
| `test-transformcontrols.html` | literal filename | static catch-all | Manual Three.js dev harness |

`/simulator` and `/integrations` (short forms) are **unrouted** — 404 in production, silently
serve the home page in dev — yet those pages' own mobile-nav links use the short forms. The
`.html` URLs are the working ones. (Fix is five `r.Get` lines next to the existing block.)

### 2.3 Client bootstrap

Each page ends its `<script>` list in a module that self-initializes on `DOMContentLoaded`
(`if (document.readyState === 'loading') …`). `js/app.js` (2,709 lines) is the main
application file for `live.html`: `init()` builds the Three.js scene + chart, opens
`/ws/dashboard` with the `js/websocket.js` reconnect manager (1s→10s backoff), starts the
10s health and 30s diurnal polls, then the RAF loop — and exports the shared `window.SpaxelApp`
facade every other module consumes instead of importing.

| Page | Bootstrap file(s) |
|---|---|
| `live.html` | `js/app.js` (+ `router.js`, `websocket.js`, `viz3d.js`, ~45 page modules) |
| `index.html` | `js/home-cards.js` (only script) |
| `simple.html` | `js/simple-mode.js` |
| `fleet.html` | `js/fleet-page.js` |
| `setup.html` | `js/placement.js`, `js/floorplan-setup.js`, `js/zone-editor.js` |
| `ambient.html` | `js/auth.js` → `ambient_renderer.js` → `ambient_briefing.js` → `ambient.js` |
| `simulator.html` | `js/simulate.js` |
| `integrations.html` | `js/integrations.js` |
| `test-transformcontrols.html` | none (CDN Three.js only) |

**Routing is two non-overlapping layers.** The server (chi) owns page identity; the client
owns *modes inside `live.html`* via `js/router.js` — a hash router (`window.SpaxelRouter`) with
routes `live` (default), `timeline`, `automations`, `settings`, `ambient`, `replay`, `simulate`,
exported as `#zones`/`#timeline` deep links the home cards rely on. Cross-page navigation is
plain `<a href>`. Deep-link params: `?maxFps=30|60|0`, `?highlight=<MAC>`,
`?reprovision=<MAC>`.

---

## 3. Build and start

### 3.1 Start it

```bash
# Production (compose builds the image from the local tree — build: block, not a pinned image)
docker compose up -d
docker compose logs -f spaxel
curl -s localhost:8080/healthz

# Local development — no Docker, serves ../dashboard from disk on :8080
cd mothership && go run ./cmd/mothership
```

Compose properties that are load-bearing, not cosmetic: `network_mode: host` (mDNS multicast
dies on Docker bridge — nodes discover the mothership through it, and `ports:` is ignored),
`cap_add: [NET_BIND_SERVICE]` (binds UDP 123 for the optional embedded SNTP server),
`stop_grace_period: 35s` (the process needs its full 30 s ordered shutdown),
`spaxel-data:/data` (SQLite, baselines, floor plans, replay buffer), and an optional
`./firmware:/firmware:ro` firmware override.

The image build **fetches the firmware artifact from GitHub Releases** rather than compiling
it (`spaxel-firmware-${VERSION}-merged.bin`), so a fresh compose build needs network access to
GitHub.

The embedded form, if you build it by hand (`-tags=embed` is required or the binary carries no
UI; the staging directory is deliberately untracked):

```bash
mkdir -p mothership/cmd/mothership/dashboard && cp -r dashboard/* mothership/cmd/mothership/dashboard/
cd mothership && CGO_ENABLED=0 go build -tags=embed -ldflags="-X main.version=$(cat ../VERSION)" -o spaxel ./cmd/mothership
```

First run needs no pre-configuration: open `http://<host>:8080`, do the one-time PIN setup
(bcrypt in SQLite), and note the auto-generated installation secret printed once at boot
(`SPAXEL_INSTALL_SECRET` overrides it for scripted deploys). Everything lands in `/data`.

### 3.2 The Dockerfile's three stages

| Stage | Base | Does |
|---|---|---|
| 1 `firmware-fetcher` | `alpine:3.20` | Downloads `spaxel-firmware[-merged]-${VERSION}.bin` from GitHub Releases; the merged offset-0 image is isolated under `/firmware/serial/` so it can never enter the OTA store |
| 2 `builder` | `golang:1.25-bookworm` | Copies `dashboard/` into `cmd/mothership/dashboard/`, builds `spaxel` (`-tags=embed`, `CGO_ENABLED=0`, `GOARCH=$TARGETARCH`, `-X main.version=${VERSION}`) and `spaxel-sim` |
| 3 runtime | `gcr.io/distroless/static-debian12:nonroot` | `ENTRYPOINT ["/spaxel"]`, `EXPOSE 8080`, `VOLUME /data`; firmware seeded at `/firmware/spaxel-firmware-${VERSION}.bin` |

At startup the mothership seeds `/firmware/*.bin` **non-recursively** into `/data/firmware/`
(the OTA store), and the semver-bearing filename is the OTA store's version source — which is
why the seeded file is named with `${VERSION}`.

### 3.3 Dashboard tooling — tests only

`dashboard/package.json` has exactly three scripts and **none produces a deployable
artifact** — the app is served as-is:

| Command | Gate |
|---|---|
| `npm test` | Jest + jsdom over `js/*.test.js` |
| `npm run test:a11y` | Playwright + axe-core WCAG 2A/2AA; self-starts `http-server . -p 3210` |
| `npm run typecheck` | `tsc --noEmit` over `types/**` only (excludes `js/` — types are an overlay) |

First-time a11y setup: `npm ci && npx playwright install --with-deps chromium`.

### 3.4 Firmware and the quality gates

```bash
cd firmware && . $IDF_PATH/export.sh && idf.py set-target esp32s3 && idf.py build   # needs ESP-IDF v5.2.x
make -C firmware/test test                                                          # host-side logic tests (gcc only)
```

| Command | Gate |
|---|---|
| `cd mothership && go test ./...` / `go vet ./...` | backend unit + vet |
| `cd mothership && golangci-lint run --timeout 5m ./...` | lint — **from `mothership/`**, never repo root |
| `cd mothership/test/acceptance && go test ./...` | AS-1…AS-7 (skips without `SPAXEL_INTEGRATION_TEST`) |
| `bash tests/e2e/run.sh` | IO-6 harness (honours `MOTHERSHIP_IMAGE`, `LOCAL_BUILD`, `SPAXEL_E2E_BIND_ADDR`) |
| `cd dashboard && npm test && npm run typecheck && npm run test:a11y` | dashboard gates |

---

## 4. Configuration

**One authority: `mothership/internal/config/config.go`** — the only reader of `SPAXEL_*` in
the backend. There is no `.env` and no per-service config file; `docker-compose.yml` supplies
defaults for the handful of variables it names (`VERSION`, `TZ`, `SPAXEL_MDNS_NAME`,
`SPAXEL_MDNS_ENABLED`, `SPAXEL_NTP_LOCAL_ENABLED`, `TRAEFIK_ENABLE`).

The variables that shape a dashboard deployment:

| Variable | Default | Why you'd touch it |
|---|---|---|
| `SPAXEL_BIND_ADDR` | `0.0.0.0:8080` | `127.0.0.1:8080` behind a proxy |
| `SPAXEL_ADVERTISED_BASE_URL` | derived | must be routable *from nodes* (OTA, mDNS TXT) |
| `SPAXEL_DATA_DIR` | `/data` | relocate SQLite/replay/OTA store |
| `SPAXEL_STATIC_DIR` | `/dashboard` | dev dashboard path (untagged builds) |
| `SPAXEL_SEED_FIRMWARE_DIR` | `/firmware` | firmware seed source |
| `SPAXEL_DEMO_MODE` | `false` | read-only dashboard, no PIN required |
| `SPAXEL_MAX_DASHBOARD_CLIENTS` | `10` | WS fan-out cap |
| `SPAXEL_MDNS_ENABLED` / `_NAME` | `true` / `spaxel` | disable when host networking is unavailable |
| `SPAXEL_LOG_LEVEL` / `_STDOUT` / `_FILE_PATH` | `info` / `true` / — | logging |
| `SPAXEL_INSTALL_SECRET` | auto-generated | HMAC root for node tokens |
| `SPAXEL_NTP_LOCAL_ENABLED` | `false` | embedded SNTP (needs the compose cap) |
| `SPAXEL_MQTT_BROKER` (+user/pass) | unset | MQTT off unless the broker is set |
| `SPAXEL_GITHUB_TOKEN` | unset | release/firmware artifact lookups |
| `TZ` | `UTC` | diurnal baselines, briefings, quiet windows |

Config-file inventory beyond env vars — the files that shape a build or run: `Dockerfile`,
`docker-compose.yml`, `go.work`/`go.work.sum`, `VERSION`, `.golangci.yml` at the root;
`mothership/go.mod` (pure-Go sqlite is the no-CGO rule), `cmd/sim/go.mod`,
`test/acceptance/go.mod`; `dashboard/{package.json,tsconfig.json,jest.config.js,
playwright.config.js,manifest.json,sw.js}`; `firmware/{CMakeLists.txt,sdkconfig.defaults,
sdkconfig.usbjtag,sdkconfig.uart-console,partitions.csv,dependencies.lock,test/Makefile}`;
`tests/e2e/run.sh`. Full purposes per file:
[`mothership-build-and-configuration.md` §2](mothership-build-and-configuration.md).

---

## 5. How a request flows (the synthesis view)

1. **Boot:** `main.go` loads config → starts `dashboardHub` → registers `/ws/dashboard`, the
   five page routes, and the `/*` static catch-all (embedded FS or disk).
2. **Page load:** browser hits `/` (or `/live`, `/fleet`, …) → catch-all/page route returns the
   HTML → the page's hand-ordered `<script>` tags execute → the last bootstrap module
   self-initializes on `DOMContentLoaded`.
3. **Live data:** the page opens **`/ws/dashboard`**; the hub fans the fleet snapshot and
   incremental blobs out (capped by `SPAXEL_MAX_DASHBOARD_CLIENTS`). `app.js` reconciles the
   snapshot and per-blob updates into the Three.js scene and exports them through
   `window.SpaxelApp` for every other module.
4. **Fallbacks/polls:** REST polls cover what WS doesn't push (health every 10 s, diurnal
   every 30 s, simple-mode's 30 s poll fallback).
5. **Node side** (for completeness): ESP32-S3 nodes connect to the *other* WebSocket,
   `/ws/node` (`internal/ingestion/`), discover the mothership via mDNS — which is why
   `network_mode: host` is non-negotiable.
6. **State:** everything durable lives in `/data` (`spaxel.db` via pure-Go sqlite, floor
   plans, CSI replay buffer, seeded `/data/firmware` OTA store), guarded by a `flock` on
   `/data/.lock`.

---

## 6. Non-obvious properties worth knowing before you change anything

1. **Flat by design, discoverable only from HTML.** With 85 files in `js/`, the only map of
   "what loads what" is the pages themselves — the page→script table in
   [`mothership-directory-structure.md` §3](mothership-directory-structure.md) is the first
   committed one.
2. **`static/` is a stale mirror with two live exceptions** (`static/icons/` for the PWA
   manifest, `static/css/mobile.css`). Resolved state of the mirror, after re-settling the
   disagreement between the source surveys: `static/js/mobile.js` **is** live
   (`live.html:4510`, ES module) and `js/fleet.js` **is** live (`live.html:3590`); the
   genuinely dead copies are `static/css/fleet-page.css` and `static/js/fleet.js` (§7).
3. **Six app modules are referenced by nothing**: `js/apdetection.js`, `js/automations.js`,
   `js/controls.js`, `js/explain.js`, `js/notifications.js`, `js/tooltip.js` — mind the
   near-name twins that *are* live (`explainability.js`, `tooltips.js`). Several look like
   plan.md features; treat as a dedicated bead before deleting.
4. **The offline PWA shell is thinner than it looks.** `sw.js` precaches the home-page shell
   only (no `app.js`, `websocket.js`, `viz3d.js`), and Three.js comes from a CDN at runtime —
   so `live.html` is not usable offline despite the service worker.
5. **Dev-mode 200s lie.** Any extension-less miss in the dev filesystem handler renders
   `index.html` with 200. Debugging "why does this path load the home page" in dev is the SPA
   fallback, not a route.
6. **plan.md drift.** Its Go layout (`pipeline/`, `localizer/`, `portal`, …) does not exist,
   and five documented env vars are read by no code: `SPAXEL_FIRMWARE_DIR` (superseded by
   `SPAXEL_SEED_FIRMWARE_DIR`), `SPAXEL_REPLAY_RETAIN_H`, `SPAXEL_GRID_CELL_M`,
   `SPAXEL_NODE_STALE_S`, `SPAXEL_SKIP_MIGRATIONS`. `config.go` is the authority.
7. **`esptool-bundle.js` is a vendored pre-built bundle** checked into the tree, not built at
   image build time (plan.md's esbuild instruction describes only this file).
8. **The `-tags=embed` staging copy exists only inside the Dockerfile.** Building the tagged
   form by hand without copying `dashboard/` first yields a binary with an empty UI and no
   error pointing at the cause.

---

## 7. Corrections settled during this synthesis

Two claims in [`mothership-directory-structure.md` §5(2)](mothership-directory-structure.md)
were corrected by [`mothership-dashboard-entry-points.md` §5(2)](mothership-dashboard-entry-points.md);
both were re-verified by grep over every `dashboard/*.html`, `js/` file and `sw.js` here:

| File | Structure doc claimed | Resolved fact (verified 2026-09-04) |
|---|---|---|
| `static/js/mobile.js` | unreferenced | **live** — `live.html:4510`, `<script type="module">`, the only ES module besides `js/fxaa.js` |
| `static/js/fleet.js` | unreferenced | **still dead** — zero references; the live copy is `js/fleet.js` (`live.html:3590`). The sibling doc's correction conflated the two paths |
| `static/css/fleet-page.css` | unreferenced | **still dead** — zero references; the live copy is `css/fleet-page.css` |

So the stale mirror is smaller than first reported: **two** dead files, not three — but the
live `static/js/mobile.js` means the mirror cannot simply be deleted wholesale; `static/icons/`
and `static/css/mobile.css` were already live, and now `static/js/mobile.js` joins them.
