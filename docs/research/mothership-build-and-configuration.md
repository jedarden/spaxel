# Mothership — Build Process, Start Commands, and Configuration Files

**Date:** 2026-09-03
**Bead:** spaxel-f37438c5
**Method:** every command below was taken from the live build files (`Dockerfile`,
`docker-compose.yml`, `dashboard/package.json`, `firmware/test/Makefile`, `tests/e2e/run.sh`)
and every environment variable was extracted by grepping `mothership/internal/config/config.go`
rather than copied from `docs/plan/plan.md` — which is where §4's drift findings come from.
Companions in this survey series: [`mothership-directory-structure.md`](mothership-directory-structure.md)
(what the tree looks like) and [`mothership-dashboard-entry-points.md`](mothership-dashboard-entry-points.md)
(where execution starts). This document answers the next question: **how do you build it, how do
you start it, and what configures it.**

---

## TL;DR

- There is **no `npm start`**. The dashboard has no build step and no dev server of its own —
  `dashboard/package.json` exists purely for **tests** (Jest, Playwright/axe, `tsc --noEmit`).
  The nearest equivalent is `cd mothership && go run ./cmd/mothership`, which serves the
  dashboard straight off the repo's `dashboard/` tree.
- The **production start command is `docker compose up -d`**, which builds the image locally
  (compose has a `build:` block, not a pinned image) with `network_mode: host` for mDNS.
- Configuration is **environment variables + one compose file**. There is **no committed
  `.env`** and no per-service config file; `mothership/internal/config/config.go` is the
  authoritative parser and every `${VAR}` in `docker-compose.yml` has an in-file default.
- Build inputs are: `Dockerfile` (3 stages: firmware fetch → Go build → distroless runtime),
  `go.work` (three Go modules), `VERSION` (single release-version source, injected via
  ldflags), `dashboard/package.json`/`tsconfig.json`/`jest.config.js`/`playwright.config.js`
  (test tooling only), `firmware/{CMakeLists.txt,sdkconfig*,partitions.csv,dependencies.lock}`
  (ESP-IDF), and `.golangci.yml` (lint gate).

---

## 1. Build and start commands

### 1.1 Production — Docker Compose (the real "start" command)

```bash
docker compose up -d          # build from source + start (compose has a build: block)
docker compose logs -f spaxel # watch startup phases ("Spaxel mothership v0.2.x starting")
curl -s localhost:8080/healthz
```

`docker-compose.yml` defaults to **building the image from the local tree** (`build.context: .`,
`build.args.VERSION: ${VERSION:-dev}`); the commented-out `image:` line is the alternative for a
pre-built `ghcr.io/spaxel/spaxel` pull. Because the compose service builds locally, a fresh
checkout needs **network access to GitHub Releases at build time** — Dockerfile stage 1 fetches
the ESP32 firmware artifact `spaxel-firmware-${VERSION}-merged.bin` from
`github.com/jedarden/spaxel/releases/download/v${VERSION}/` (it is *not* compiled in the image
build any more; CI builds it once and publishes it as a release artifact).

Non-negotiable compose properties (all commented in the file itself):

| Property | Why |
|---|---|
| `network_mode: host` | mDNS multicast (224.0.0.251) is blocked by Docker bridge networking; nodes discover the mothership via mDNS. `ports:` is ignored in this mode — 8080 is already on the host. |
| `cap_add: [NET_BIND_SERVICE]` | lets the distroless nonroot container bind UDP 123 for the embedded SNTP server (`SPAXEL_NTP_LOCAL_ENABLED=true`). Harmless when that is off. |
| `stop_grace_period: 35s` | the process needs the full 30 s of its ordered graceful shutdown. |
| `volumes: spaxel-data:/data` | SQLite DB, baselines, floor plans, CSI replay buffer. Bind-mounting `/data` also works. |
| `volumes: ./firmware:/firmware:ro` | **optional** firmware override; the image already seeds `/firmware` from the release artifact (see §3.2). |
| healthcheck `wget http://localhost:8080/healthz` | note: distroless has no shell, and the healthcheck relies on `wget` being present — verify it actually passes on your base image before relying on it for orchestration. |

### 1.2 Local development — plain Go, no Docker

```bash
cd mothership
go run ./cmd/mothership          # serves the dashboard from ../dashboard on :8080
```

Untagged builds use the **filesystem fallback** (`main.go` ~line 4935): `dashboard_embed.go`
carries `//go:build embed`, so without the tag `dashboardEmbedded` stays false and static files
are resolved from `SPAXEL_STATIC_DIR` if it exists, then `findDashboardDir()` candidates
(`./dashboard`, `./../dashboard`, `/app/dashboard`, executable-relative). Running from
`mothership/` therefore finds `../dashboard` automatically and logs
`Serving dashboard from filesystem at …`. If every candidate is missing, static serving is
silently skipped with a single WARN and the API still comes up.

The tagged/embedded production form (what the Dockerfile runs):

```bash
mkdir -p mothership/cmd/mothership/dashboard && cp -r dashboard/* mothership/cmd/mothership/dashboard/
cd mothership && CGO_ENABLED=0 go build -tags=embed -ldflags="-X main.version=$(cat ../VERSION)" -o spaxel ./cmd/mothership
```

`-tags=embed` is **required** for the binary to carry the UI; the staging copy under
`cmd/mothership/dashboard/` is deliberately untracked (verified: not in `git ls-files`).

Other binaries:

```bash
cd mothership && go build -o spaxel-sim ./cmd/sim   # CSI/node simulator (also baked into the image)
go run ./cmd/sim --mothership ws://localhost:8080/ws/node --nodes 4 --walkers 1 --duration 30s
```

`cmd/sim` is a **separate Go module** (`cmd/sim/go.mod`) stitched in by `go.work`; build it from
its own directory, not through a root module (there is no root `go.mod` — verified).

### 1.3 Dashboard tooling — tests only, no build

`dashboard/package.json` defines exactly three scripts, and none of them produces deployable
artifacts:

| Command | What it does |
|---|---|
| `npm test` | Jest unit tests (`jest.config.js`, jsdom env, `**/*.test.js`) |
| `npm run test:a11y` | Playwright + `@axe-core/playwright` WCAG 2A/2AA gate (`playwright.config.js`); starts its own `npx http-server . -p 3210` webServer and loads `index`, `live`, `fleet`, `setup`, `integrations` |
| `npm run typecheck` | `tsc --noEmit -p tsconfig.json` over `types/**` only |

First-time setup for the a11y gate: `npm ci && npx playwright install --with-deps chromium`.
`dashboard/README.md` documents this and the CI invocation (`node:20-bookworm-slim`, run before
the container build).

### 1.4 Firmware (ESP-IDF)

```bash
# Host build (needs ESP-IDF v5.2.x; on NixOS use Docker instead):
cd firmware && . $IDF_PATH/export.sh
idf.py set-target esp32s3 && idf.py build
idf.py -p /dev/ttyUSB0 flash

# Board console variant (ADR-002) — layered via SDKCONFIG_DEFAULTS:
idf.py -D SDKCONFIG_DEFAULTS="sdkconfig.defaults;sdkconfig.usbjtag" build   # shipped default
idf.py -D SDKCONFIG_DEFAULTS="sdkconfig.defaults;sdkconfig.uart-console" build  # bridge-chip boards

# Host-side logic tests — plain gcc, no ESP-IDF needed:
make -C firmware/test test
```

`firmware/BUILD.md` is the fuller runbook (including the Docker-based firmware build used on
NixOS). The release firmware artifact that the mothership image fetches is produced by CI, not
by the Dockerfile.

### 1.5 Verification / quality-gate commands

| Command | Gate |
|---|---|
| `cd mothership && go test ./...` | unit tests (repo rule before closing any bead) |
| `cd mothership && go vet ./...` | vet |
| `cd mothership && golangci-lint run --timeout 5m ./...` | lint gate — **run from `mothership/`**, never repo root (the root `go.work` makes root-level `./...` exit 7) |
| `cd mothership/test/acceptance && go test ./...` | AS-1…AS-7 acceptance suite (separate module) |
| `bash tests/e2e/run.sh` | IO-6 shell harness: builds/starts a mothership + `spaxel-sim`, asserts blob/events; honours `MOTHERSHIP_IMAGE`, `LOCAL_BUILD=true`, and `SPAXEL_E2E_BIND_ADDR` (ephemeral-port default since spaxel-8b7dc1a9) |
| `cd dashboard && npm test && npm run typecheck && npm run test:a11y` | dashboard unit / types / a11y |
| `make -C firmware/test test` | firmware host tests (gcc harness) |

---

## 2. Configuration-file inventory

Nothing in this repository is configured by a per-service config file — there is no `spaxel.conf`
and no `.env`. The full set of files that shape a build or a run:

### 2.1 Repository root

| File | Purpose |
|---|---|
| `Dockerfile` | 3-stage image build: **(1)** `alpine:3.20` firmware-fetcher downloads `spaxel-firmware[-merged]-${VERSION}.bin` from GitHub Releases; **(2)** `golang:1.25-bookworm` builds `spaxel` (`-tags=embed`, `CGO_ENABLED=0`, `GOARCH=$TARGETARCH`, `-X main.version=${VERSION}`) and `spaxel-sim`; **(3)** `distroless/static-debian12:nonroot` runtime, `ENTRYPOINT ["/spaxel"]`, `EXPOSE 8080`, `VOLUME /data`. App image is seeded at `/firmware/spaxel-firmware-${VERSION}.bin`; the merged offset-0 image is isolated at `/firmware/serial/…-merged.bin` so it can never enter the OTA store. |
| `docker-compose.yml` | The deployment manifest — host networking, volumes, env defaults, healthcheck, resource limits, optional Traefik labels (off by default via `TRAEFIK_ENABLE:-false`). |
| `.dockerignore` | Keeps `node_modules`, build output, and scratch files out of the build context. |
| `go.work` / `go.work.sum` | Stitches the three modules (`./mothership`, `./cmd/sim`, `./test/acceptance`). **No root `go.mod` exists** — a root-level `go build ./...` fails with "directory prefix . does not contain modules listed in go.work". |
| `VERSION` | Single source of truth for the release version (currently `0.2.149`). Consumed as the Docker build arg → `-ldflags "-X main.version=…"` → `/healthz`, `/api/status`, firmware filename, and the OTA store's semver-bearing seeded filename. CI auto-bumps it on substantive pushes. |
| `.golangci.yml` | Lint gate (v2 config): `errcheck`, `staticcheck` (S/SA only), `govet`, `ineffassign`, `unused`, plus a very long per-path errcheck/unused exclusion list. Note the `errcheck.exclude` regex list is **dead under v2** (v1 syntax) — per-path rules are what actually apply. |
| `.gitignore` / `.gitattributes` | Repo hygiene; `.gitattributes` pins line endings for cross-platform firmware sources. |

### 2.2 Go modules

| File | Purpose |
|---|---|
| `mothership/go.mod` / `go.sum` | Backend module `github.com/spaxel/mothership`, Go 1.25. Notable deps: `modernc.org/sqlite` (pure-Go SQLite — the no-CGO rule), `gorilla/websocket`, `go-chi/chi/v5`, `hashicorp/mdns`, `gonum`, `fogleman/gg`, `paho.mqtt.golang`. |
| `cmd/sim/go.mod` / `go.sum` | Simulator module (separate on purpose: the sim ships in the same image but is not part of the backend's dependency graph). |
| `test/acceptance/go.mod` / `go.sum` | Cross-cutting AS-1…AS-7 acceptance module. |

### 2.3 Dashboard (`dashboard/`)

| File | Purpose |
|---|---|
| `package.json` / `package-lock.json` | **Test tooling only** (`jest`, `@playwright/test`, `@axe-core/playwright`, `typescript`, `http-server`). No build/bundle script exists — the app is served as-is. |
| `tsconfig.json` | Type-checks `types/**` with `strict`, `noEmit`, `moduleResolution: Bundler`; **excludes `js/` and `tests/`** — the runtime app is plain JS, types are an overlay, not a compile step. |
| `jest.config.js` | jsdom environment; `setupFiles: js/onboard.test.setup.js`, `setupFilesAfterEach`-style `setupFilesAfterEnv: js/ambient.test.setup.js`; matches `**/*.test.js`. |
| `playwright.config.js` | a11y gate: `testDir: ./tests`, `baseURL: http://localhost:3210`, headless, self-started `npx http-server . -p 3210`. |
| `manifest.json` / `sw.js` | PWA manifest + service worker (offline capability); served as ordinary static files. |
| `generate-icons.js`, `run-leak-profiling.js` | One-off dev utilities (icon generation, memory-leak profiling harness), not part of any gate. |

### 2.4 Firmware (`firmware/`)

| File | Purpose |
|---|---|
| `CMakeLists.txt` (+ `main/CMakeLists.txt`) | ESP-IDF project definition; layers `sdkconfig.usbjtag` over `sdkconfig.defaults` for the shipped build (ADR-002 console-on-USB-Serial/JTAG default). |
| `sdkconfig.defaults` | Committed base config: target `esp32s3`, CSI/promiscuous, WiFi+BLE coexistence, OTA rollback, flash size, partition table. |
| `sdkconfig.usbjtag`, `sdkconfig.uart-console` | Board-variant console overlays, selected via `SDKCONFIG_DEFAULTS` (§1.4). `sdkconfig` / `sdkconfig.old` are **generated** local artifacts, not sources of truth. |
| `partitions.csv` | True A/B layout: `nvs`(0x9000) + `phy_init` + `otadata`(0x10000) + `ota_0`/`ota_1` (2×0x1F0000). **No `factory` slot** — 4 MB flash cannot hold three ~1.55 MB images; bootloader rollback replaces it. NVS offset kept stable so reflash without `erase-flash` preserves provisioning. |
| `dependencies.lock` | Pins the ESP-IDF component-manager dependency versions (reproducible `managed_components/`). |
| `test/Makefile` | gcc host-test harness: globs `test_*.c` + `test_runner.c`, builds `build/spaxel_host_tests`, `make test` = build + run. Self-registering `TEST()` constructors — new tests need no Makefile edit. |
| `main/` `BUILD.md` | Firmware sources and the build/flash runbook. |

### 2.5 Test harness configuration

| File | Purpose |
|---|---|
| `tests/e2e/run.sh` | IO-6 shell harness. Env knobs: `MOTHERSHIP_IMAGE` (default `spaxel-e2e:test`), `LOCAL_BUILD` (run the Go binary instead of Docker), `SIM_NODES/SIM_WALKERS/SIM_RATE/SIM_SEED/SIM_DURATION`, `HEALTH_TIMEOUT`, `TEST_TIMEOUT`. Binds the mothership to `127.0.0.1:<ephemeral>` by default. |
| `test/acceptance/*.go` | AS-1…AS-7 scenarios; gate on `SPAXEL_INTEGRATION_TEST` / `SPAXEL_HARDWARE_TEST` so they skip outside CI. |
| `.claude/`, `.beads/`, `.needle.yaml` | Agent/tooling config, not product config. `.beads/` is the bead-rs store and is **never hand-edited**. |

---

## 3. Required setup steps

### 3.1 First run (nothing to pre-configure)

1. `docker compose up -d` (or the local Go run from §1.2).
2. Open `http://<host>:8080` → one-time **PIN setup** page (bcrypt hash stored in SQLite;
   `/api/auth/setup`, first run only).
3. The mothership auto-generates a 256-bit **installation secret** on first boot, stores it in
   SQLite and prints it once: `[SPAXEL] Installation secret: <hex>`. It derives every node's
   HMAC token (`POST /api/provision`); override with `SPAXEL_INSTALL_SECRET` for scripted deploys.
4. Data lands in `/data` (`spaxel.db`, CSI replay buffer, seeded `/data/firmware/` OTA store).
   `SPAXEL_DATA_DIR` relocates it. A `flock` on `/data/.lock` prevents a second instance.
5. Fleet WiFi credentials (ADR-005) are **not** env config beyond first-boot seeding:
   `SPAXEL_WIFI_SSID`/`SPAXEL_WIFI_PASSWORD` seed the `network` setting exactly once, and the
   dashboard's Settings → Network is authoritative afterwards.

### 3.2 Firmware seeding

At startup the mothership copies `/firmware/*.bin` (from the image, or the `./firmware` volume
override) into `/data/firmware/` — non-recursively, so the image's `serial/` subdirectory is
excluded and a merged offset-0 image can never be offered to `esp_ota`. The **semver-bearing
filename is the OTA store's version source**, which is why the Dockerfile names the seeded file
`spaxel-firmware-${VERSION}.bin` instead of the historical bare `spaxel-firmware.bin`.

---

## 4. Environment variables — verified against `internal/config`

`mothership/internal/config/config.go` is the **only** reader of `SPAXEL_*` in the backend
(extracted by grep; test files excluded). This is the real list, with the code's own defaults:

| Variable | Default | Notes |
|---|---|---|
| `SPAXEL_BIND_ADDR` | `0.0.0.0:8080` | HTTP + both WebSocket endpoints. `127.0.0.1:8080` to restrict behind a proxy. |
| `SPAXEL_ADVERTISED_BASE_URL` | derived | Base URL handed to nodes (OTA download, mDNS TXT). Must be routable from the node; see ADR-004. |
| `SPAXEL_DATA_DIR` | `/data` | SQLite, floor plans, replay buffer, OTA store. |
| `SPAXEL_STATIC_DIR` | `/dashboard` | Untagged-build dashboard path; falls back to `findDashboardDir()` (§1.2). |
| `SPAXEL_SEED_FIRMWARE_DIR` | `/firmware` | Read-only source dir seeded into `/data/firmware/`. |
| `SPAXEL_INSTALL_SECRET` | auto-generated | 64-hex HMAC root for node tokens. |
| `SPAXEL_MDNS_ENABLED` | `true` | `false` when host networking is unavailable; nodes then need the cached `ms_ip` NVS key. |
| `SPAXEL_MDNS_NAME` | `spaxel` | Must match the firmware's `ms_mdns` NVS key. |
| `SPAXEL_LOG_LEVEL` | `info` | `debug`/`info`/`warn`/`error`. |
| `SPAXEL_LOG_STDOUT` | `true` | stdout logging toggle. |
| `SPAXEL_LOG_FILE_PATH` | — | Optional file log target. |
| `SPAXEL_FUSION_RATE_HZ` | `10` | Range [1,20]. |
| `SPAXEL_REPLAY_MAX_MB` | `360` | Range [10,10000]. |
| `SPAXEL_REPLAY_COMPRESSION` | `true` | zstd for the CSI replay buffer. |
| `SPAXEL_REPLAY_CHUNK_MB` | `64` | Range [1,100]. |
| `SPAXEL_MIGRATION_WINDOW_HOURS` | `24` | How long untokened nodes are tolerated; range [0,168], 0 = disabled. |
| `SPAXEL_DEMO_MODE` | `false` | Read-only dashboard, mutating endpoints blocked, no PIN required. |
| `SPAXEL_MAX_DASHBOARD_CLIENTS` | `10` | Range [1,100]. |
| `SPAXEL_NTP_SERVER` | `pool.ntp.org` | Becomes this host's own address when `SPAXEL_NTP_LOCAL_ENABLED` is on. |
| `SPAXEL_NTP_LOCAL_ENABLED` | `false` | Embedded SNTP server (needs the compose `NET_BIND_SERVICE` cap). |
| `SPAXEL_MQTT_BROKER` / `_USERNAME` / `_PASSWORD` | unset | MQTT disabled unless the broker is set. Not exposed in compose by default (commented lines). |
| `SPAXEL_GITHUB_TOKEN` | unset | GitHub API access (release/firmware artifact lookups). |
| `SPAXEL_WIFI_SSID` / `SPAXEL_WIFI_PASSWORD` | unset | **First-boot seed only** for the stored `network` setting (ADR-005). |
| `TZ` | `UTC` | Diurnal baselines, briefings, quiet windows. |

**Drift warning — documented-but-dead variables.** Five variables that `docs/plan/plan.md`'s
Deployment table lists are read by **no code** (verified by repo-wide grep over non-test Go
sources): `SPAXEL_FIRMWARE_DIR` (superseded by `SPAXEL_SEED_FIRMWARE_DIR`),
`SPAXEL_REPLAY_RETAIN_H`, `SPAXEL_GRID_CELL_M`, `SPAXEL_NODE_STALE_S`,
`SPAXEL_SKIP_MIGRATIONS`. Setting any of them is a silent no-op. `config.go` is the source of
truth; plan.md's table has not been kept in step with it.

**Compose-level variables** (substituted by docker compose, with in-file defaults — there is no
committed `.env` file, though compose would auto-read one if created next to the yml):
`VERSION`, `TZ`, `SPAXEL_MDNS_NAME`, `SPAXEL_MDNS_ENABLED`, `SPAXEL_NTP_LOCAL_ENABLED`,
`TRAEFIK_ENABLE`. The MQTT and log-level entries are commented out rather than defaulted.

**Test-harness-only variables:** `SPAXEL_E2E_BIND_ADDR` (e2e harness bind override),
`SPAXEL_INTEGRATION_TEST` / `SPAXEL_HARDWARE_TEST` (acceptance-suite gates),
`SPAXEL_MOTHERSHIP_URL` / `SPAXEL_MOTHERSHIP_PATH` / `SPAXEL_SIM_PATH` / `SPAXEL_NO_DOCKER` /
`SPAXEL_PORT` / `SPAXEL_VERSION` (harness plumbing). None of these affect a production run.

---

## 5. Gaps and cross-references

- Per-package code map: [`mothership-directory-structure.md`](mothership-directory-structure.md);
  execution start + route map: [`mothership-dashboard-entry-points.md`](mothership-dashboard-entry-points.md).
- CI-side build (Argo `spaxel-build` WorkflowTemplate, multi-arch, firmware artifact publishing):
  [`spaxel-build-workflow-parameters.md`](spaxel-build-workflow-parameters.md),
  [`spaxel-build-architecture-targeting.md`](spaxel-build-architecture-targeting.md),
  [`docker-build-step-configuration.md`](docker-build-step-configuration.md). The template lives
  in `jedarden/declarative-config` (separate repo), so those documents are the interface.
- Firmware build/flash runbook: `firmware/BUILD.md`; host-test rationale:
  `docs/notes/firmware-host-test-approach.md`.
- Dashboard↔mothership serving/embedding detail: `docs/notes/dashboard-mothership-integration.md`.
- **Undocumented-by-anything-else:** the `-tags=embed` staging step (copy `dashboard/` →
  `cmd/mothership/dashboard/`) exists only inside the Dockerfile; a developer building with the
  tag for the first time needs this document's §1.2 to know the copy is required and that the
  staged directory is deliberately untracked.
