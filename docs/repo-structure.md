# Spaxel Repository Structure

**Date:** 2026-09-04 · **Bead:** spaxel-857d1451
**Method:** every path, count and behaviour claim below was checked against `git ls-tree HEAD`
(the committed tree), not the working tree — the working tree routinely carries other workers'
uncommitted files. Companion surveys supply the per-file detail:

- [`docs/research/go-backend-code-directories.md`](research/go-backend-code-directories.md) — all 381 `.go` files, 56 `internal/` packages, build tags, plan-vs-code deltas
- [`docs/research/esp32-firmware-code-directories.md`](research/esp32-firmware-code-directories.md) — the single firmware tree, ESP-IDF evidence, tracked C/H inventory
- [`docs/notes/go-work-module-dir-inventory-2026-09-04.md`](notes/go-work-module-dir-inventory-2026-09-04.md) — go.work verification and the six candidate module directories

This file is the entry point. It is the map; those are the surveys.

---

## 1. The system in one paragraph

Spaxel is WiFi-CSI indoor positioning for self-hosted homes. One container — the
**mothership** (Go, `mothership/`) — runs on a home server and manages a fleet of
**ESP32-S3 nodes** whose firmware (`firmware/`, ESP-IDF C) streams 24-byte binary CSI
frames over WebSocket. The mothership's signal pipeline fuses CSI across links into
presence, motion and 2D/3D position, persists to SQLite, and serves a Three.js
floor-plan dashboard (`dashboard/`, embedded into the Go binary at build time). A
simulator (`spaxel-sim`) stands in for hardware. Everything is local; there is no cloud.

```
ESP32-S3 nodes (firmware/, C)                home server
┌─────────────────────────┐   WebSocket    ┌──────────────────────────────┐
│ csi.c → 24 B CSI frames │ ─────────────► │ mothership (Go container)    │
│ wifi.c  mDNS discovery  │ ◄───────────── │  ingestion → signal → fusion │
│ websocket.c + OTA path  │   provisioning │  → tracking → API/dashboard  │
└─────────────────────────┘                │  SQLite (/data)              │
                                           └──────────┬───────────────────┘
 browser ◄──── embedded dashboard (Three.js) ───────────┘
 spaxel-sim ◄─ stands in for nodes during development and acceptance tests
```

---

## 2. Top-level tree (tracked)

Annotated with purpose and stack. `⌫` marks paths that are **gitignored** (exist on a
build/dev machine only); `⚑` marks tracked litter — see §9.

```
spaxel/
├── go.work                  Go workspace: ./mothership, ./cmd/sim, ./test/acceptance
├── go.work.sum              workspace checksums
│                            ⚠ no root go.mod — the repo root is NOT a module, so
│                              `go test ./...` fails from the root (§4)
├── mothership/              THE GO BACKEND — module github.com/spaxel/mothership (§5)
├── cmd/sim/                 standalone simulator module github.com/spaxel/sim (§4, §6)
├── test/acceptance/         acceptance-test module github.com/spaxel/acceptance (§4, §6)
├── firmware/                ESP32-S3 node firmware, ESP-IDF v5.2, C (§7)
├── dashboard/               vanilla JS + Three.js SPA, Jest + Playwright/axe (§8)
├── docs/                    plan, notes, research, inventories (§9)
├── Dockerfile               3-stage build: firmware fetch → Go build → distroless (§10)
├── docker-compose.yml       single-container deploy, host networking, port 8080
├── VERSION                  current semver; consumed by the build and OTA filenames
├── README.md, PROGRESS.md   project intro and phase-by-phase implementation status
├── LICENSE
├── .golangci.yml            lint config (v2 — note: v1 `errcheck.exclude` is dead there)
├── .needle.yaml, .beads/    NEEDLE worker config and the bead-rs work tracker
├── scripts/                 device + sim helper scripts (Python, shell, Node) (§11)
├── data/ ⚑                  RUNTIME STATE committed to git by mistake (§12)
├── testdata/ ⚑              two `//go:build ignore` CSI-recording utilities — in no module
├── tests/e2e/run.sh ⚑       shell e2e harness — separate from the Go e2e tests (§6)
├── notes/ ⚑, memory/ ⚑      per-bead findings dumped at the repo root, not under docs/
├── *.md, *.txt ⚑ (~45)      one-off investigation reports at the root (§13)
└── firmware/build/ ⌫  firmware/managed_components/ ⌫  cmd/sim/sim ⌫
    mothership/cmd/mothership/dashboard/ ⌫ (go:embed staging)  *.db-shm/*.db-wal ⌫
```

There is exactly one code tree per language: C only under `firmware/`, dashboard JS/TS only
under `dashboard/`, Go only in the three modules of §4.

---

## 3. Technology stack by component

| Component | Path | Language / framework | Test harness |
|---|---|---|---|
| Mothership backend | `mothership/` | Go 1.25, stdlib `net/http`, `gorilla/websocket`, pure-Go `modernc.org/sqlite` (CGO_ENABLED=0) | `go test` — table-driven tests throughout `internal/` |
| Node firmware | `firmware/` | C, ESP-IDF v5.2, FreeRTOS, NimBLE | `make -C firmware/test test` (host gcc, no ESP-IDF) |
| Dashboard | `dashboard/` | Vanilla JS/TS + Three.js, no framework | `npm test` + `npm run test:a11y` (Jest, axe-core, Playwright) |
| Simulator | `mothership/cmd/sim/` (shipped), `cmd/sim/` (standalone) | Go | `go test` |
| Acceptance | `test/acceptance/` | Go | `go test ./...` (drives built `spaxel` + `spaxel-sim` binaries) |
| Packaging | `Dockerfile`, `docker-compose.yml` | Alpine fetch stage, golang:1.25-bookworm builder, distroless runtime | CI: `spaxel-build` Argo WorkflowTemplate |

---

## 4. The three Go modules — and the twins

`go.work` registers three modules; there is **no root `go.mod`**:

| Module path | Directory | Role |
|---|---|---|
| `github.com/spaxel/mothership` | `mothership/` | The backend (365 `.go` files, 56 internal packages) |
| `github.com/spaxel/sim` | `cmd/sim/` | Self-contained simulator CLI; depends only on `gorilla/websocket`, so it cannot import `mothership/internal/*` |
| `github.com/spaxel/acceptance` | `test/acceptance/` | AS-1…AS-7 acceptance scenarios; `replace github.com/spaxel/mothership => ../../mothership` |

**Consequence for every command:**

```bash
cd mothership && go test ./... && go vet ./...   # correct — whole-module wildcards
go test ./...        # FROM THE ROOT: fails ("directory prefix . does not contain
                     #  modules listed in go.work") — the root is not a module
```

`golangci-lint` has the same rule: run it from `mothership/`, never from the root.

### Twins — four names that each mean two things

This is the single most common source of wrong-file edits in this repo. The **shipped**
or **CI-exercised** member is marked ✓.

| Pair | ✓ Shipped / CI-exercised | The other one |
|---|---|---|
| **Simulators** | `mothership/cmd/sim/` — imports `internal/simulator` + `internal/ble`; built as `/spaxel-sim` in the Docker image | `cmd/sim/` (root) — standalone module, one file, no internal imports; a divergent re-implementation with the same doc-comment preamble |
| **Acceptance trees** | `mothership/test/acceptance/` — what `spaxel-build` CI runs (`go test ./test/acceptance/` from cwd `mothership/`) | `test/acceptance/` (root) — go.work module, run only by hand (`cd test/acceptance && go test ./...`) |
| **e2e trees** | `mothership/tests/e2e/` — Go tests (`io6_gate` build tag) plus `e2e_test.go` | `tests/e2e/run.sh` (root) — shell harness; unrelated to the Go e2e package |
| **Firmware dirs** | `firmware/` — the ESP-IDF source tree | `/firmware` in the container — the runtime OTA *seed* store, seeded from `SPAXEL_SEED_FIRMWARE_DIR` into `<SPAXEL_DATA_DIR>/firmware/` (§12) |

Both simulator directories and both acceptance directories are legitimate; they are just
not interchangeable. Before editing anything named `sim`, `acceptance`, or `e2e`, resolve
which of the pair the task means.

---

## 5. `mothership/` — the backend module

```
mothership/
├── go.mod / go.sum          module github.com/spaxel/mothership, go 1.25.0
├── cmd/
│   ├── mothership/          package main → the `spaxel` binary
│   │   ├── main.go          ⚠ 6,020 lines / 108 funcs — ALL subsystem construction
│   │   │                    and wiring lives here (greppable constructors:
│   │   │                    ingestion.NewServer, fleet.NewManager, fusion.NewEngine,
│   │   │                    ota.NewManager, shutdown.NewManager, zones.NewManager …)
│   │   ├── dashboard_embed.go   `//go:build embed` — go:embed of the dashboard FS
│   │   ├── migrate.go           `//go:build ignore_migrate` — standalone migration helper
│   │   └── mdns_binding.go      mDNS interface selection
│   ├── sim/                 package main → the `spaxel-sim` binary (the shipped one)
│   │                        main.go, generator.go, scenario.go, walker.go, verify.go
│   └── _parse_check.go      `//go:build ignore` throwaway AST checker; `_` also hides it
│                            from Go tooling
├── internal/                56 packages — all application code (grouped below)
├── test/                    in-module ad-hoc verification tests (pkg test)
├── test/acceptance/         in-module acceptance tests — the ones CI runs
└── tests/e2e/               IO-gate Go e2e tests (pkg e2e, `io6_gate` build tag)
```

Dependency direction is strictly one-way: `cmd/*` → `internal/*`; nothing in `internal/`
imports `cmd/`.

### `internal/` — 56 packages in seven groups

File counts include `_test.go`. Full per-package detail (including which packages are
near-duplicates) is in the [backend survey](research/go-backend-code-directories.md).

| Group | Packages |
|---|---|
| **Process lifecycle & platform (11)** | `config` (env parsing/defaults), `startup` (phased init with timeouts), `shutdown` (ordered graceful stop), `health` (`/healthz`), `doctor` (pre-flight diagnostics), `diskspace` (free-space degradation ladder), `loadshed` (adaptive shedding), `logging`, `eventbus` (pub/sub), `events` (typed bus + storage), `types` |
| **Device I/O & fleet (8)** | `ingestion` (`/ws/node` binary CSI parse, node lifecycle), `fleet` (registry, roles, TX-slot re-stagger), `provisioning` (tokens), `apdetector` (router BSSID consensus → virtual AP node), `ota` (canary + quiet windows), `autoupdate`, `ntpserver` (SNTP responder for isolated LANs), `github` (release lookups) |
| **Signal processing & localization (6)** | `signal` (the real pipeline — 20 files; *not* `internal/pipeline/`, which does not exist), `fusion` (3D Fresnel-weighted multi-link solve), `localization` (GDOP, ground truth, link weights), `localizer/fusion` (test-only CI timing gate), `tracking` (UKF blob tracking — the wired one), `tracker` (orphaned near-duplicate, no importers) |
| **Storage & recording (6)** | `db` (SQLite open + migrations), `recorder` (per-link 1-hour segments), `recording` (circular buffer + compression), `replay` (`csi_replay.bin` time travel), `floorplan` (upload + pixel→metre calibration), `zones` (zones, portals, occupancy) |
| **People, identity & learning (5)** | `ble` (device registry, identity matching), `sleep` (overnight state machine, breathing), `prediction` (presence from transition models), `learning` (accuracy from feedback), `analytics` (anomaly alerts + crowd flow) |
| **Automation & safety (5)** | `automation` (3D trigger volumes → actions), `volume` (geometry), `falldetect` (Z-trajectory falls), `webhook`, `diagnostics` (link weather, root cause, repositioning advice) |
| **API, auth & UI support (13)** | `api` (48 files — the REST API), `auth` (PIN + sessions), `dashboard` (`/ws/dashboard` hub), `timeline`, `briefing`, `explainability`, `guidedtroubleshoot`, `help`, `notify` (Ntfy/Pushover/Gotify — the wired one), `notifications` (orphaned duplicate), `render` (PNG thumbnails), `mqtt` (Home Assistant discovery), `oui` (IEEE OUI) |
| **Simulation & tooling (2)** | `simulator` (synthetic CSI/walker/BLE — consumed by `cmd/sim` and tests), `beads` (diagnostic reports) |

Known dead weight (compiles clean, no external importers): `tracker`, `notifications`,
and the `eventbus`/`events` overlap. plan.md's *Go Module Layout* names several packages
that do not exist (`internal/pipeline/*`, `internal/portal`, `internal/anomaly`,
`internal/flow`) — treat plan.md there as design intent, not as a map.

---

## 6. The simulator and test trees

```
cmd/sim/                  standalone module github.com/spaxel/sim (1 file, self-contained)
mothership/cmd/sim/       shipped spaxel-sim: generator/scenario/walker/verify
mothership/internal/simulator/   the synthetic-CSI library both worlds lean on
test/acceptance/          module github.com/spaxel/acceptance — AS-1…AS-7, runs built binaries
mothership/test/acceptance/  in-module acceptance suite — what CI actually runs
mothership/tests/e2e/     Go e2e + `io6_gate` gated hard-gate tests
tests/e2e/run.sh          root shell e2e harness (unrelated to the Go e2e package)
mothership/test/          ad-hoc verification tests (wifi-credential flow, empty-password bug)
testdata/                 `//go:build ignore` utilities to generate/verify CSI fixtures
```

Acceptance tests drive a **built `spaxel` binary plus `spaxel-sim` on `PATH`** — they
compose processes rather than import packages in-process.

CI reality check: the `spaxel-e2e` template's go-test leg invokes
`./mothership/test/acceptance/...` from cwd `mothership/`, which resolves to a path that
does not exist, so that leg exits 1 on every run regardless of code health. A red there is
not evidence about the package.

---

## 7. `firmware/` — the node firmware

One firmware tree in the whole repo; no Arduino/PlatformIO anywhere.

```
firmware/
├── CMakeLists.txt           project(spaxel-firmware); sdkconfig.defaults;sdkconfig.usbjtag
├── partitions.csv           true A/B: ota_0 + ota_1, 4 MB flash, no factory slot
├── sdkconfig.defaults       CSI enabled, NimBLE, SPIRAM, rollback + anti-rollback
│                            (no secure boot / flash encryption — see the firmware survey)
├── main/                    12 .c + 12 .h ≈ 5,154 lines — the application
│   ├── websocket.c  1,203   node↔mothership protocol AND the OTA download path
│   ├── wifi.c         682   station connect, mDNS discovery, captive-portal AP
│   ├── main.c         584   boot + node state machine (BOOT/DISCOVERY/CONNECTED/
│   │                        WIFI_LOST/CAPTIVE_PORTAL)
│   ├── csi.c          350   promiscuous mode → CSI callback → 24-byte frames
│   ├── provision.c    323   10 s serial provisioning window (UART + USB-Serial/JTAG)
│   ├── ble.c          262   NimBLE passive advertisement scan
│   ├── safe_mode.c, nvs_migration.c, ntp.c, transport.c, led.c, watchdog.c
├── test/                    host-gcc harness (`make -C firmware/test test`) — no ESP-IDF
├── scripts/                 generate-signing-key.sh, sign-firmware.sh, verify-console-config.sh
├── docs/                    nimble-savings.md, bluedroid-baseline.txt
├── managed_components/ ⌫    vendored ESP-IDF components (component manager)
└── build/ ⌫                 CMake output (~199 MB): spaxel-firmware.bin/.elf, bootloader/
```

House-keeping rules that matter here: never use `ESP_ERROR_CHECK` in application code
(aborts → boot loops; use explicit error checks and the restart-safe guard pattern —
see README *Error Handling*); `sdkconfig` / `sdkconfig.old` are generated, so edit
`sdkconfig.defaults` and the two board variants instead; `firmware/test/test_runner` is a
tracked compiled ELF, a known hygiene wart.

---

## 8. `dashboard/` — the frontend

Vanilla JS + Three.js single-page app with multiple HTML entry points
(`index.html`, `setup.html`, `live.html`, `fleet.html`, `integrations.html`,
`ambient.html`, `simulator.html`, `simple.html`), plus `js/` (including
`esptool-bundle.js` for browser-side Web-Serial flashing of nodes), `css/`, `static/`,
`tests/`, Jest + Playwright/axe config, `manifest.json` and `sw.js`.

Build-time detail worth knowing: the dashboard is **not** served from this directory in
production. The Dockerfile copies `dashboard/` → `mothership/cmd/mothership/dashboard/`
(a gitignored staging path) and builds with `-tags=embed`, so `go:embed` bakes it into the
`spaxel` binary. Untagged local builds fall back to serving the repo-root `dashboard/`
from disk. Every HTML entry point must wire the agentation feedback toolbar itself —
adding a page does not inherit it.

---

## 9. `docs/` — where documentation lives

| Path | Contents |
|---|---|
| `docs/plan/plan.md` | The single complete application plan (architecture, schema, deployment, phases). Authoritative for intent — but its *Go Module Layout* and *Firmware Build System* sections have drifted from the tree |
| `docs/notes/` | Implementation notes, ADRs, post-mortems (mdns-override, error-handling patterns, firmware host tests, go.work inventory …) |
| `docs/research/` | CSI physics and algorithm research, prior art, and the structural surveys this file builds on |
| `docs/inventory/` | Point-in-time directory inventories (superseded by this file where they disagree) |
| `docs/deployment/`, `docs/design/`, `docs/examples/`, `docs/tests/` | Deployment guides, design notes, Go examples (uncompiled), test documentation |
| `docs/BUILD_PATHS.md`, `docs/SYSTEM_CATALOG.md` | Build-path catalog and subsystem catalog |
| `docs/ci-accessibility-integration.md`, `docs/ci-benchmark-integration.md` | The dashboard a11y and fusion-timing CI quality gates |

---

## 10. Build and release

The Dockerfile is three stages:

1. **`firmware-fetcher`** (alpine:3.20) — downloads prebuilt
   `spaxel-firmware[-merged]-<VERSION>.bin` from GitHub Releases. Firmware is built once
   in CI, not inside the image.
2. **`builder`** (golang:1.25-bookworm, `--platform=$BUILDPLATFORM`, `GOARCH=$TARGETARCH`,
   `CGO_ENABLED=0` — required by the pure-Go SQLite driver) — builds `spaxel` from
   `./cmd/mothership` with `-tags=embed`, and `spaxel-sim` from `./cmd/sim`.
   Both paths resolve **inside the mothership module**, so the shipped simulator is
   `mothership/cmd/sim`, never the root module.
3. **runtime** (distroless static, nonroot) — `/spaxel`, `/spaxel-sim`,
   `/firmware/spaxel-firmware-<VERSION>.bin` (OTA seed; the semver-bearing filename is the
   OTA store's version source) and `/firmware/serial/…-merged.bin`, which `seedFirmwareDir`
   deliberately does **not** copy into the OTA store — a merged offset-0 image must never
   be written into an app partition.

CI builds come from the `spaxel-build` Argo WorkflowTemplate (iad-ci) — this repo has no
GitHub Actions. Note the template's lint group runs from `mothership/` and currently fails
before the firmware leg, while `resolve-version` still bumps `VERSION`.

---

## 11. `scripts/` — device and sim helpers

Language mix: Python (`provision_esp32.py`, `measure_csi_rate.py`), shell
(`flash-esp32s3.sh`, `run-sim-*.sh` local-sim recipes), Node (`capture-dashboard-console.mjs`),
plus `walkthrough_monitor.sh` and `test-github-api.sh`. Nothing here is imported by the
build; these are operator and developer tools.

---

## 12. Runtime data and the two firmware directories

At run time the mothership keeps all state under `SPAXEL_DATA_DIR` (default `/data`):
SQLite databases, baselines, floor plans, the CSI replay buffer, and the OTA firmware
store. Container contents are seeded at boot by `seedFirmwareDir`
(`mothership/cmd/mothership/main.go`) from `SPAXEL_SEED_FIRMWARE_DIR`
(default `/firmware`, `mothership/internal/config/config.go`):

| Directory | What it is | Stack / format |
|---|---|---|
| `firmware/` (repo) | ESP-IDF **source** tree | C, built by ESP-IDF in CI |
| `/firmware` (container) | **Seed** input for the OTA store; override with `SPAXEL_SEED_FIRMWARE_DIR` | top-level `*.bin` only, **non-recursive** (that is why the merged image lives in `serial/`) |
| `<SPAXEL_DATA_DIR>/firmware/` | Live OTA store the OTA manager serves to nodes | filenames carry the semver |

⚠ `SPAXEL_FIRMWARE_DIR` is documented in plan.md but is **read by no code** — a silent
no-op. The real variable is `SPAXEL_SEED_FIRMWARE_DIR`.

`data/` in the repo is different from `/data` in the container: it is a stray capture of
runtime state (17 tracked files: `*.db` SQLite files, `backups/`, `.lock`). `.gitignore`
excludes `*.db-shm`/`*.db-wal` but not `*.db`, so these committed to git. They are
artifacts of a past local run — not source, not fixtures; do not treat them as seed data.

---

## 13. Root-level litter (tracked, but not structure)

The repo root carries two categories of clutter worth recognising before adding anything:

- **One-off reports** (~45 files): `DIRECTORY_STRUCTURE.md`, `SOURCE_CODE_INVENTORY.md`,
  `WORKSPACE_STRUCTURE*.md`, `SYSTEM_CATALOG.md`, `MOTHERSHIP_DASHBOARD_LOCATIONS.md`,
  `GDOP_COMPUTATION_GUIDE.md`, `*VERIFY*.md`, `*_results.txt`,
  `verify-pack-corruption-indicators.*`, `bug_verification_report.md`, … These are
  point-in-time investigation outputs. They go stale fast; prefer `docs/notes/` for new
  work (as the repo-init convention requires) and treat root reports as history.
- **Stray directories**: `notes/` (per-bead `bf-*` findings that belong under `docs/notes/`)
  and `memory/` (a single agent-memory note). Untracked at any given moment you may also
  find `tmp/`, `.claude/`, `~`-style accident dirs and built binaries — those are
  gitignored or local and never structure.

---

## 14. Gotchas summary

1. `go test ./...` / `go vet ./...` / `golangci-lint` must run from `mothership/` — the
   root is not a module.
2. Two simulators, two acceptance trees, two e2e trees, two "firmware" directories — §4
   tells you which one is shipped/CI-exercised.
3. `SPAXEL_FIRMWARE_DIR` does nothing; `SPAXEL_SEED_FIRMWARE_DIR` is the real variable.
4. `main.go` (6,020 lines) is the only wiring point — new subsystems get constructed there.
5. plan.md's layout sections describe intent; the tree has drifted (no `internal/pipeline/`,
   no `internal/portal`, `predict` → `prediction`, etc.).
6. The dashboard is embedded at build time via a gitignored staging copy — a dashboard
   change is invisible in the binary without `-tags=embed` and the `COPY` step.
7. CGO must stay off: SQLite is `modernc.org/sqlite` (pure Go); new Go deps must be pure Go.
8. `data/` in the repo is captured runtime state, not fixtures or seed data.
9. Three packages are orphaned duplicates (`tracker`, `notifications`, `events` vs
   `eventbus`) — check which one `main.go` imports before extending a feature.

---

## 15. Source documents

- [`README.md`](../README.md) — project overview and quickstart (its module table matches §4)
- [`PROGRESS.md`](../PROGRESS.md) — implementation phase status
- [`docs/plan/plan.md`](plan/plan.md) — full design
- [`docs/research/go-backend-code-directories.md`](research/go-backend-code-directories.md)
- [`docs/research/esp32-firmware-code-directories.md`](research/esp32-firmware-code-directories.md)
- [`docs/notes/go-work-module-dir-inventory-2026-09-04.md`](notes/go-work-module-dir-inventory-2026-09-04.md)
- [`docs/BUILD_PATHS.md`](BUILD_PATHS.md), [`docs/SYSTEM_CATALOG.md`](SYSTEM_CATALOG.md)
- [`dashboard/README.md`](../../dashboard/README.md), [`firmware/BUILD.md`](../../firmware/BUILD.md)
