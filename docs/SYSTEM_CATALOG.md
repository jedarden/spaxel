# Spaxel Repository Structure Catalog

**Generated:** 2026-08-21; reconciled 2026-09-04 (spaxel-7737eea8 — this is the surviving
copy; the duplicate root `SYSTEM_CATALOG.md` was deleted and its build-trigger path
classification folded in)  
**Purpose:** Complete directory structure reference for build path classification and trigger routing

## Repository Overview

Spaxel is a WiFi CSI-based indoor positioning system with three main components:
- **Firmware:** ESP32-S3 C code (ESP-IDF)
- **Mothership:** Go backend with embedded dashboard
- **Dashboard:** Vanilla JS + Three.js web interface

---

## Root-Level Files

### Core Build & Deployment
- `Dockerfile` — Multi-stage build (firmware fetch + Go + distroless runtime)
- `docker-compose.yml` — Single-service deployment manifest
- `VERSION` — Single source of truth for release version
- `go.work` — Go workspace with a single module (`use ./mothership`)
- `go.work.sum` — Go workspace dependencies

### Configuration
- `.needle.yaml` — NEEDLE fleet dispatch configuration
- `.needle-predispatch-sha` — NEEDLE pre-dispatch SHA tracking
- `.golangci.yml` — Linting configuration (v2)
- `.gitignore`, `.dockerignore`, `.gitattributes` — VCS and build controls

### Documentation
- `README.md` — Project overview and quickstart
- `PROGRESS.md` — Implementation status tracking
- `LICENSE` — Project license

### Investigation Records (relocated from the root, 2026-09-04)
The point-in-time investigation reports no longer live at the repo root; they were
moved into `docs/notes/` and `docs/inventory/` under kebab-case names
(`api-implementation-status.md`, `ble-personid-investigation.md`,
`mothership-dashboard-*.md`, the verification summaries, …), the GDOP guide was
consolidated into `docs/gdop-computation-functions.md`, and four
stale/duplicate files were deleted. See
`docs/notes/root-exhaust-classification.md` (addendum) for the per-file record.
Durable documentation lives under `docs/`; only `README.md` and `PROGRESS.md`
remain at the root.

---

## Directory Tree Structure

```
spaxel/
├── firmware/                    # ESP32-S3 firmware (ESP-IDF C)
│   ├── main/                   # Main application component
│   │   ├── main.c              # Entry point, startup sequencing
│   │   ├── wifi.c / wifi.h     # WiFi station, mDNS, captive portal
│   │   ├── csi.c / csi.h       # CSI capture, binary frame serialization
│   │   ├── websocket.c / .h    # WebSocket client, binary CSI up / JSON config down
│   │   ├── transport.c / .h    # Transport abstraction (UART0 + USB-Serial/JTAG)
│   │   ├── ble.c / ble.h       # BLE passive scan, advertisement parsing
│   │   ├── led.c / led.h       # LED control (identify, OTA progress)
│   │   ├── ntp.c / ntp.h       # NTP time sync for TX stagger slots
│   │   ├── nvs_migration.c/.h  # NVS read/write + schema migration
│   │   ├── provision.c / .h    # Serial provisioning handler
│   │   ├── safe_mode.c         # Safe-mode entry/recovery
│   │   ├── watchdog.c          # Task watchdog (esp_task_wdt)
│   │   ├── spaxel.h            # Common header
│   │   └── CMakeLists.txt      # Component manifest
│   ├── managed_components/     # ESP-IDF component dependencies
│   │   ├── espressif__esp_websocket_client/
│   │   └── espressif__mdns/
│   ├── build/                  # ESP-IDF build output
│   │   ├── bootloader/
│   │   ├── partition_table/
│   │   ├── spaxel-firmware/    # Merged binary artifact
│   │   └── log/
│   ├── scripts/               # Build/deployment scripts
│   └── test/
│       └── build/             # Host-test harness (gcc, no ESP-IDF)
│
├── mothership/                # Go backend (mothership module)
│   ├── cmd/
│   │   └── mothership/        # Main entry point
│   │       └── main.go        # Startup sequencing, subsystem wiring
│   ├── internal/              # All internal packages (56)
│   │   ├── ingestion/         # WebSocket server, binary frame parsing
│   │   ├── signal/            # Signal processing (flat: phase sanitization, features,
│   │   │                      #   breathing, baseline, diurnal, ambient, processor)
│   │   ├── localizer/         # Fusion & localization
│   │   │   └── fusion/        # Full localization loop (10 Hz)
│   │   ├── fusion/            # 3D grid fusion engine
│   │   ├── localization/      # Grid, ground truth, spatial-weight learning
│   │   ├── tracker/           # Blob tracking + BLE identity
│   │   ├── tracking/          # UKF tracking core
│   │   ├── fleet/             # Node registry, role assignment, stagger scheduler
│   │   ├── ble/               # BLE centroid, rotation heuristics, identity matching
│   │   ├── replay/            # CSI replay buffer reader/writer + pipeline
│   │   ├── analytics/         # Anomaly scoring, patterns, crowd flow, alert chain
│   │   ├── prediction/        # Presence prediction engine
│   │   ├── learning/          # Feedback processing, accuracy trends
│   │   ├── sleep/             # Sleep state machine, breathing FFT
│   │   ├── falldetect/        # Fall detection algorithm
│   │   ├── notify/            # Notification rendering (fogleman/gg)
│   │   ├── notifications/     # Notification channels (ntfy, Pushover, webhook)
│   │   ├── briefing/          # Morning briefing generation
│   │   ├── render/            # 2D rendering for notifications
│   │   ├── webhook/           # Webhook publisher
│   │   ├── mqtt/              # MQTT client, HA auto-discovery
│   │   ├── github/            # GitHub releases client
│   │   ├── help/              # Help article monitor
│   │   ├── guidedtroubleshoot/ # Contextual help system
│   │   ├── explainability/    # Detection explanation
│   │   ├── timeline/          # Activity timeline
│   │   ├── eventbus/          # In-process event bus
│   │   ├── events/            # Event storage and querying
│   │   ├── zones/             # Zone manager + occupancy history
│   │   ├── floorplan/         # Floor plan image management
│   │   ├── volume/            # 3D trigger volume shapes
│   │   ├── automation/        # Spatial automation (triggers, volumes)
│   │   ├── api/               # REST + WebSocket handlers
│   │   ├── auth/              # HMAC token derivation, bcrypt PIN, sessions
│   │   ├── dashboard/         # Dashboard WebSocket feed
│   │   ├── config/            # Environment variable parsing
│   │   ├── db/                # SQLite open/migrate, schema migrations
│   │   ├── provisioning/      # Node provisioning, token generation
│   │   ├── ota/               # OTA manager, firmware serving
│   │   ├── autoupdate/        # OTA auto-update with canary deployment
│   │   ├── apdetector/        # AP auto-detection for passive radar
│   │   ├── recorder/          # CSI recording to disk
│   │   ├── recording/         # CSI replay storage
│   │   ├── simulator/         # CSI simulator engine
│   │   ├── oui/               # OUI lookup table (generated from IEEE list)
│   │   ├── health/            # Health check endpoints
│   │   ├── doctor/            # System health diagnostics
│   │   ├── diagnostics/       # Link weather + repositioning advice
│   │   ├── loadshed/          # Load shedding under high CPU
│   │   ├── diskspace/         # Disk space monitoring
│   │   ├── startup/           # Startup sequencing phases
│   │   ├── shutdown/          # Graceful shutdown orchestration
│   │   ├── logging/           # Shared logging setup
│   │   ├── types/             # Shared log-level types
│   │   ├── beads/             # Bead-state diagnostics helpers
│   │   └── ntpserver/         # NTP server for testing
│   ├── test/acceptance/       # Acceptance scenarios AS-1…AS-7 + IO install/upgrade
│   ├── tests/e2e/             # End-to-end Go tests (incl. IO-6 gate)
│   └── build/                 # Go build output (gitignored)
│
├── dashboard/                # Embedded web UI (embedded into Go binary)
│   ├── index.html            # Main entry point
│   ├── js/                   # Vanilla JavaScript modules
│   │   ├── app.js                    # Main application entry
│   │   ├── auth.js                   # Authentication (PIN setup/login)
│   │   ├── ambient.js                # Ambient mode renderer
│   │   ├── ambient_renderer.js      # Ambient Canvas 2D rendering
│   │   ├── ambient_briefing.js       # Morning briefing display
│   │   ├── accuracy.js               # Accuracy tracking, feedback UI
│   │   ├── anomaly.js                # Anomaly detection display
│   │   ├── automation-builder.js     # 3D trigger volume editor
│   │   ├── automations.js            # Automation rule management
│   │   ├── apdetection.js            # AP detection UI
│   │   ├── ble-panel.js              # BLE device registry UI
│   │   ├── blob-identity.js          # BLE-to-blob matching display
│   │   ├── briefing.js               # Morning briefing generation
│   │   └── [40+ more modules]        # See full listing below
│   ├── css/                  # Stylesheets
│   ├── static/               # Static assets
│   │   ├── css/
│   │   ├── icons/
│   │   └── js/
│   ├── types/               # TypeScript definitions (.d.ts)
│   ├── tests/               # Dashboard tests
│   │   ├── ambient.test.js
│   │   ├── blob-identity.test.js
│   │   ├── backward-compat.test.js
│   │   └── [more test files]
│   ├── test-results/         # Test output
│   └── node_modules/         # NPM dependencies (generated)
│
├── docs/                      # Documentation
│   ├── plan/
│   │   └── plan.md           # Master implementation plan (this document)
│   ├── notes/                # Development notes, ADRs, investigations
│   │   ├── bf-15oi-runtime-capture/
│   │   ├── bf-2gmx-runtime-capture/
│   │   ├── bf-4do5y-runtime-capture/
│   │   ├── mdns-override.md
│   │   ├── token-reject-root-cause.md
│   │   ├── simulation-testing.md
│   │   ├── firmware-host-test-approach.md
│   │   ├── ambient-traffic-measurement.md
│   │   └── [more notes]
│   ├── research/             # Third-party research, reference material
│   │   └── papers/          # Academic papers on CSI, localization
│   ├── deployment/           # Deployment guides
│   ├── examples/             # Example configurations
│   └── tests/                # Test documentation
│
├── scripts/                 # Utility scripts
│
├── .beads/                  # Bead tracking (bead-rs backend)
│   ├── config.json          # Bead configuration
│   ├── checkpoint/          # Durable checkpoint storage
│   │   ├── current.json     # Current state
│   │   ├── previous.json    # Previous state
│   │   ├── forensic.jsonl   # Full history
│   │   ├── objects/         # Checkpoint objects
│   │   └── manifests/       # Checkpoint manifests
│   ├── recovery/            # Recovery checkpoints
│   │   └── run-*/
│   ├── receipts/            # Bead receipts
│   └── traces/               # Execution traces per bead
│       └── spaxel-*/
│
├── .marathon/               # Marathon test state
├── .claude/                 # Claude Code workspace files
│   ├── worktrees/           # Git worktrees
│   └── [memory files]
├── ~                        # Scratch/overflow directory
│   └── .needle/
│
├── .git/                    # Git repository
└── notes/                   # Additional notes
```

---

## Module Breakdown

### Firmware (`firmware/`)

**Language:** C (ESP-IDF framework)
**Purpose:** ESP32-S3 node firmware

**Key Components:**
- WiFi connectivity (station + mDNS + captive portal)
- CSI capture in promiscuous mode
- WebSocket client (binary CSI upstream, JSON config downstream)
- BLE passive scanning (Core 0)
- OTA updates with rollback
- NVS persistence + migration
- Serial provisioning (UART + USB-Serial/JTAG)
- NTP sync for TX scheduling
- LED control

**Build Artifacts:**
- `build/spaxel-firmware` — Merged binary (bootloader + partition table + app)
- Flashed to ESP32-S3 at offset 0x10000

---

### Mothership (`mothership/`)

**Language:** Go (Go 1.25+)
**Purpose:** Backend server, ingestion, processing, localization, dashboard

**Architecture:**
- **Ingestion:** WebSocket server at `/ws/node`, binary frame parsing, node lifecycle
- **Pipeline:** Phase sanitization → NBVI subcarrier selection → feature extraction → baseline
- **Localization:** Fresnel zone accumulation → peak extraction → UKF tracking → BLE identity fusion
- **Fleet:** Node registry, role assignment, TX stagger scheduling, self-healing
- **Storage:** SQLite with `modernc.org/sqlite` (pure Go, no CGO)
- **Dashboard:** Embedded static files served at `/`

**Internal Packages (56):**
All subsystems are independent internal packages under `internal/`, grouped by domain:
- **Ingestion & signal** — `ingestion`, `signal` (flat: phase sanitization, features, breathing, baseline, diurnal), `recorder`, `recording`, `replay`, `simulator`
- **Localization & tracking** — `fusion`, `localizer` (+`fusion/`), `localization`, `tracker`, `tracking`, `ble`
- **Fleet & node lifecycle** — `fleet`, `provisioning`, `ota`, `autoupdate`, `apdetector`
- **Spaces, events & automation** — `zones`, `floorplan`, `volume`, `automation`, `eventbus`, `events`, `timeline`
- **Inference & monitoring** — `analytics` (anomaly scoring, patterns, crowd flow), `prediction`, `learning`, `sleep`, `falldetect`, `health`
- **API & persistence** — `api`, `auth`, `config`, `db`, `dashboard`
- **Notifications & integrations** — `briefing`, `notify`, `notifications`, `render`, `webhook`, `mqtt`, `github`, `help`, `guidedtroubleshoot`
- **Platform & operations** — `startup`, `shutdown`, `doctor`, `diagnostics`, `explainability`, `loadshed`, `diskspace`, `logging`, `types`, `beads`, `ntpserver`, `oui`

---

### Dashboard (`dashboard/`)

**Language:** Vanilla JavaScript (ES6+) + Three.js
**Purpose:** 3D spatial visualization, control interface

**Key Modules:**
- `app.js` — Main application, WebSocket client, 3D scene setup
- `auth.js` — PIN setup, login/logout
- `ambient.js` + `ambient_renderer.js` — Wall-mounted tablet mode
- `accuracy.js` — Feedback loop, accuracy tracking
- `automation-builder.js` — 3D trigger volume editor
- `ble-panel.js` — BLE device registry
- `blob-identity.js` — BLE-to-blob matching display
- `briefing.js` — Morning briefing
- And 40+ more specialized modules

**Static Assets:**
- `css/` — Stylesheets
- `icons/` — Icons
- `types/` — TypeScript definitions (`.d.ts`)

**Embedding:** Entire `dashboard/` is embedded into the Go binary via `//go:embed` in `cmd/mothership/`

---

### Documentation (`docs/`)

**Purpose:** Design decisions, research, operational guidance

**Structure:**
- `plan/plan.md` — **Master implementation plan** (this document)
- `notes/` — Development notes, ADRs, investigation records
- `research/papers/` — Academic papers on CSI, localization
- `deployment/` — Deployment guides
- `examples/` — Example configurations
- `tests/` — Test documentation

---

### Simulator, Acceptance & E2E

All three live inside the single `mothership` Go module:

**`mothership/cmd/sim/`** — CSI simulator CLI (`spaxel-sim`)
- Generates synthetic CSI frames for development/testing
- Multi-node, multi-walker simulation
- BLE advertisement simulation

**`mothership/test/acceptance/`** — Acceptance scenarios (AS-1…AS-7)
- Simulator-based integration tests
- Validates full new-user journey, plus IO install/upgrade tests

**`mothership/tests/e2e/`** — End-to-end Go tests
- `e2e_test.go`, `assertions_test.go`, IO-6 gate tests

---

### Bead Tracking (`.beads/`)

**Backend:** bead-rs (as of 2026-08-14 migration)
**Purpose:** Track implementation beads, progress, recovery

**Structure:**
- `checkpoint/` — Durable checkpoint (git-tracked)
  - `current.json` — Current state
  - `previous.json` — Previous state
  - `forensic.jsonl` — Full history
  - `objects/` — Per-bead objects
  - `manifests/` — Checkpoint manifests
- `recovery/` — Recovery checkpoints
- `receipts/` — Bead receipts
- `traces/` — Per-bead execution traces

---

## Build Impact Classification

This catalog serves as the foundation for classifying which paths trigger builds
(the path globs below are folded in from the deleted root copy):

### Firmware Build Triggers
- `firmware/main/*.c`, `firmware/main/*.h`
- `firmware/CMakeLists.txt`, `firmware/partitions.csv`, `firmware/sdkconfig.defaults`
- `firmware/test/*.c`, `firmware/test/*.h`, `firmware/scripts/*.sh`
- `firmware/managed_components/`
- `VERSION` — a bump is build-relevant here too (the firmware bakes a version header)

### Mothership Build Triggers
- `mothership/cmd/**/*.go`, `mothership/internal/**/*.go`
- `mothership/test/**/*.go`, `mothership/tests/**/*.go`
- `mothership/go.mod`, `mothership/go.sum`, `go.work`, `go.work.sum`
- `VERSION`

### Dashboard Build Triggers (embedded)
- `dashboard/*.html`, `dashboard/css/*.css`, `dashboard/js/*.js`
- `dashboard/static/**`, `dashboard/types/**`
- `dashboard/package.json`, `dashboard/tsconfig.json`
- Dashboard files are hot-reloadable during development (served from disk), but they
  are embedded at build time via `//go:embed`, so production images rebuild.

### Container & Lint Triggers
- `Dockerfile`, `docker-compose.yml`
- `.golangci.yml` (lint only), `.jest.config.js` / `.playwright.config.js` (test config)

### Documentation Changes
- `docs/**/*.md`, `README.md`, `PROGRESS.md`
- No build impact — these are purely informational

---

## Key File Reference

| Path | Purpose |
|------|---------|
| `VERSION` | Release version (single source of truth) |
| `Dockerfile` | Multi-stage build (firmware → Go → runtime) |
| `docker-compose.yml` | Production deployment manifest |
| `go.work` | Go workspace definition |
| `docs/plan/plan.md` | Master implementation plan |
| `PROGRESS.md` | Implementation status tracking |
| `.needle.yaml` | NEEDLE fleet dispatch config |

---

## Notes

- **Dashboard is embedded:** Entire `dashboard/` directory is embedded into the Go binary via `//go:embed`. Dashboard changes require rebuilding the mothership binary.
- **Go workspace:** One Go module — `mothership/` (go 1.25.0) — stitched into the workspace by the root `go.work` (`use ./mothership`). The simulator CLI (`mothership/cmd/sim/`) and both test trees live inside it.
- **Firmware artifact:** The merged binary at `firmware/build/spaxel-firmware` is the OTA payload baked into the Docker image.
- **Bead backend:** Uses `bead` CLI (bead-rs) as of 2026-08-14; deprecated `bf` CLI should not be used.
