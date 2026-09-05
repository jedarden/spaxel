# Spaxel Workspace Structure

**Generated:** 2026-08-29, refreshed 2026-09-04 (spaxel-7737eea8; absorbs the deleted
`WORKSPACE_STRUCTURE_DOCUMENTATION.md`)
**Relocated:** 2026-09-04 from the repository root to `docs/inventory/` (spaxel-2ee3e526,
root exhaust sweep) — the root keeps only the newcomer-facing set plus `README.md` and
`PROGRESS.md`
**Workspace Root:** `/home/coding/spaxel`

Spaxel is a WiFi CSI-based indoor positioning system for self-hosted homes: ESP32-S3
nodes stream channel-state information to a single Go binary (the *mothership*), which
processes it into presence, tracking, and health state and serves an embedded
vanilla-JS dashboard.

This file is the top-level map: the Go module, its packages, the root files, and the
build/test entry points. Per-file directory detail lives in
`docs/inventory/repository-directory-structure.md`; `docs/repo-structure.md` is the
newest structural survey and supersedes older inventories where they disagree.

---

## Go Workspace

`go.work` at the repository root defines **exactly one module**:

```
go 1.25.0

use ./mothership
```

There is no root `go.mod`. The simulator CLI and both test trees live **inside** the
`mothership` module (`mothership/cmd/sim/`, `mothership/test/acceptance/`,
`mothership/tests/e2e/`) — there is no top-level `cmd/`, `test/`, or `tests/`
directory. `go build`, `go test`, `go vet`, and `golangci-lint` must be run from
`mothership/`; the repository root is not a module.

---

## `mothership/` — the Go module

```
mothership/
├── go.mod               # module github.com/spaxel/mothership (go 1.25.0)
├── go.sum
├── cmd/
│   ├── mothership/      # the binary (9 files): main.go (startup phases, subsystem
│   │                    #   wiring, routes), dashboard_embed.go (go:embed of the UI),
│   │                    #   migrate.go, mdns_binding.go + their tests
│   └── sim/             # spaxel-sim CLI — the simulator the Docker image ships
│                        #   (main, generator, walker, scenario, verify + Makefile/README)
├── internal/            # 56 packages, grouped below
├── test/acceptance/     # acceptance scenarios AS-1…AS-7 (+ WiFi restart race) and the
│                        #   IO install/upgrade tests — 11 files incl. test_helpers.go
└── tests/e2e/           # end-to-end Go tests: e2e_test.go, assertions_test.go and the
                         #   IO-6 gate pair (4 files)
```

### `internal/` packages (56), grouped by domain

**Ingestion & signal processing (6)**
`ingestion` WebSocket server + binary frame parsing · `signal` flat package: phase
sanitisation, features, breathing, baseline, diurnal, ambient, persistence, processor ·
`recorder` CSI recording to disk · `recording` recording buffer · `replay` CSI replay
pipeline · `simulator` pre-deployment simulation engine

**Localization & tracking (6)**
`fusion` 3D grid fusion engine · `localizer` (contains `fusion/`, the 10 Hz loop) ·
`localization` grid, ground truth, spatial-weight learning · `tracker` blob tracking +
BLE identity · `tracking` UKF tracking core · `ble` BLE registry, rotation heuristics,
identity matching

**Fleet & node lifecycle (5)**
`fleet` node registry, role assignment, stagger scheduling, self-heal · `provisioning`
node provisioning/token generation · `ota` OTA manager and firmware serving ·
`autoupdate` canary auto-update · `apdetector` AP auto-detection for passive radar

**Spaces, events & automation (7)**
`zones` zone manager + occupancy history · `floorplan` floor plan images/calibration ·
`volume` trigger volume shapes · `automation` spatial automation triggers · `eventbus`
in-process event bus · `events` event storage/querying · `timeline` activity timeline

**Inference & monitoring (6)**
`analytics` anomaly scoring, patterns, crowd flow, alert chain · `prediction` presence
prediction (15-min horizon) · `learning` feedback processing + accuracy trends ·
`sleep` sleep quality (breathing FFT, records, reports) · `falldetect` fall detection ·
`health` health-check endpoints

**API & persistence (5)**
`api` REST + WebSocket handlers (largest package) · `auth` HMAC tokens, PIN, sessions ·
`config` environment parsing · `db` SQLite open/migrate (`modernc.org/sqlite`, no CGO) ·
`dashboard` dashboard WebSocket feed

**Notifications & integrations (9)**
`briefing` morning briefing · `notify` notification rendering/dispatch ·
`notifications` channels (ntfy, Pushover, webhook) · `render` floor-plan image
rendering · `webhook` webhook publisher · `mqtt` MQTT client + Home Assistant
auto-discovery · `github` GitHub releases client · `help` help-article monitor ·
`guidedtroubleshoot` contextual troubleshooting

**Platform & operations (12)**
`startup` startup sequencing · `shutdown` graceful shutdown · `doctor` system health
checks · `diagnostics` link weather + repositioning advice · `explainability` detection
explanation ("why is this here?") · `loadshed` load shedding under pressure · `diskspace` disk monitoring · `logging` ·
`types` shared log-level types · `beads` bead-state diagnostics · `ntpserver` NTP
server for testing · `oui` OUI lookup table (generated from the IEEE list)

> Near-duplicate package pairs exist by history (`tracker`/`tracking`,
> `recorder`/`recording`, `notify`/`notifications`) — check what `main.go` imports
> before extending a feature. See `docs/research/go-backend-code-directories.md` for
> the per-package file-level map.

---

## Top-Level Directories

| Directory | Contents |
|---|---|
| `mothership/` | The Go module (above) — backend, simulator, all Go tests |
| `firmware/` | ESP32-S3 ESP-IDF C project: `main/` (12 `.c` sources + headers), `test/` (9 host-based gcc test files + Makefile), `scripts/` (signing/verify), `managed_components/`, `build/` (gitignored) |
| `dashboard/` | Vanilla JS + Three.js web UI: 9 HTML entry points, flat `js/` (app modules + co-located jest tests + the node-CLI leak-profiling harness), flat `css/`, `static/` (icons + mobile), `types/`, `tests/` (Playwright a11y). Embedded into the Go binary at build time |
| `docs/` | `plan/plan.md` (master plan), `notes/`, `research/`, `inventory/`, `design/`, `deployment/`, `examples/`, `tests/` |
| `scripts/` | Operator/developer helpers: `flash-esp32s3.sh`, `run-sim-*.sh`, `provision_esp32.py`, `measure_csi_rate.py`, `capture-dashboard-console.mjs`, `walkthrough_monitor.sh`, `test-github-api.sh` |
| `testdata/` | Two `//go:build ignore` CSI-recording utilities (in no module) |
| `data/` | Captured runtime state (SQLite, backups) — not source, not fixtures |
| `notes/` | Per-bead investigation findings dumped at the root (belongs under `docs/notes/`) |
| `memory/` | Agent memory notes |

---

## Root-Level Files (tracked)

**Build & deploy:** `Dockerfile` (3-stage: firmware fetch → Go build → distroless
runtime) · `docker-compose.yml` (single service, host networking) · `.dockerignore` ·
`VERSION` (single source of truth; consumed by the build and OTA filenames)

**Go workspace:** `go.work` · `go.work.sum`

**VCS & lint:** `.gitignore` · `.gitattributes` · `.golangci.yml` (golangci-lint v2)

**NEEDLE / beads:** `.needle.yaml` (fleet dispatch config) ·
`.needle-predispatch-sha` (pre-dispatch SHA tracking) · `.beads/` (bead-rs store;
`checkpoint/` is git-tracked)

**Project docs:** `README.md` (overview + quickstart) · `PROGRESS.md`
(phase-by-phase implementation status) · `LICENSE`

**Investigation reports:** none at the root any more. The point-in-time work-log that
used to sit here (`API_IMPLEMENTATION_STATUS.md`, `BASELINE_CAPTURE_SUMMARY.md`,
`BENCH_HOSTNAME_INFO.md`, `BLE_PERSONID_INVESTIGATION.md`,
`console-implementation-status.md`, `CSI_RECORDING_FILES_SEARCH_RESULTS.md`,
`dashboard_discovery_notes.md`, `EMPTY_PASSWORD_TEST_RESULTS.md`,
`GDOP_COMPUTATION_GUIDE.md`, the `MOTHERSHIP_DASHBOARD_*` set,
`mothership_location.md`, `PERMISSION_VERIFICATION_SUMMARY.md`,
`PRESENCE_DETECTION_VERIFICATION.md`, `rust-source-inventory.md`) was disposed of on
2026-09-04 under the repo-root exhaust sweep `spaxel-1b8df9a3`: the survivors were
relocated into `docs/notes/` and `docs/inventory/` under kebab-case names, and the
stale/duplicate ones were deleted. Per-file record:
`docs/notes/root-exhaust-classification.md` (addendum).

Durable documentation belongs under `docs/`; investigation reports are history, not a map.

---

## Development Workflow

**Build (Docker, 3 stages)** — `firmware-fetcher` downloads the prebuilt firmware
bin from GitHub Releases; `builder` (golang:1.25-bookworm, `CGO_ENABLED=0`) builds
`spaxel` from `./cmd/mothership` (`-tags=embed`) and `spaxel-sim` from `./cmd/sim`,
both inside the mothership module; `runtime` is distroless static/nonroot. CI runs the
`spaxel-build` Argo WorkflowTemplate (iad-ci); this repo has no GitHub Actions.

**Test**

```bash
cd mothership && go test ./...                      # unit + integration (all packages)
cd mothership && go vet ./...                       # vet
cd mothership && go test ./test/acceptance/ ./tests/e2e/   # acceptance + e2e trees
make -C firmware/test test                          # firmware host tests (gcc, no ESP-IDF)
cd dashboard && npm test                            # jest (co-located *.test.js)
cd dashboard && npm run test:a11y                   # Playwright + axe-core
```

**Embedded dashboard:** the Dockerfile copies `dashboard/` to
`cmd/mothership/dashboard/` before a tagged build and `dashboard_embed.go` embeds it
behind the `embed` build tag; untagged local builds serve the canonical tree from disk.

---

## Related Documentation

- `docs/plan/plan.md` — master implementation plan (architecture, schema, deployment)
- `docs/repo-structure.md` — newest structural survey (supersedes older inventories)
- `docs/inventory/repository-directory-structure.md` — directory-level inventory
- `docs/SYSTEM_CATALOG.md` — component catalog + build-trigger classification
- `docs/codebase-structure-and-test-patterns.md` — test conventions and coverage
- `docs/research/go-backend-code-directories.md`,
  `docs/research/esp32-firmware-code-directories.md` — per-package trees
- `../../PROGRESS.md` — implementation phase status
