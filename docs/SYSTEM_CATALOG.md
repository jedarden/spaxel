# Spaxel Repository Structure Catalog

**Generated:** 2026-08-21
**Purpose:** Complete directory structure reference for build path classification and trigger routing

## Repository Overview

Spaxel is a WiFi CSI-based indoor positioning system with three main components:
- **Firmware:** ESP32-S3 C code (ESP-IDF)
- **Mothership:** Go backend with embedded dashboard
- **Dashboard:** Vanilla JS + Three.js web interface

---

## Root-Level Files

### Core Build & Deployment
- `Dockerfile` — Multi-stage build (firmware + Go + distroless runtime)
- `docker-compose.yml` — Single-service deployment manifest
- `VERSION` — Single source of truth for release version
- `go.work` — Go workspace stitching mothership, cmd/sim, test/acceptance modules
- `go.work.sum` — Go workspace dependencies

### Configuration
- `.needle.yaml` — NEEDLE fleet dispatch configuration
- `.needle-predispatch-sha` — NEEDLE pre-dispatch SHA tracking
- `.golangci.yml` — Linting configuration
- `.gitignore`, `.dockerignore`, `.gitattributes` — VCS and build controls

### Documentation
- `README.md` — Project overview and quickstart
- `PROGRESS.md` — Implementation status tracking
- `LICENSE` — Apache 2.0

### ADR & Investigation Records
- `API_IMPLEMENTATION_STATUS.md`
- `BLE_PERSONID_INVESTIGATION.md`
- `GDOP_COMPUTATION_GUIDE.md`

### Scripts
- `blob_observation.sh` — Observation/testing script
- `fix_ble_handlers.py` — BLE handler fixes
- `window_test.sh` — Testing script

---

## Directory Tree Structure

```
spaxel/
├── firmware/                    # ESP32-S3 firmware (ESP-IDF C)
│   ├── main/                   # Main application component
│   │   ├── main.c              # Entry point, startup sequencing
│   │   ├── wifi.c / wifi.h     # WiFi station, mDNS, captive portal
│   │   ├── csi.c / csi.h       # CSI capture, binary frame serialization
│   │   ├── ws.c / websocket.c  # WebSocket client, JSON/binary framing
│   │   ├── ble.c / ble.h       # BLE passive scan, advertisement parsing
│   │   ├── ota.c               # OTA download, SHA-256 verification
│   │   ├── nvs.c               # NVS helpers (not migration)
│   │   ├── nvs_migration.c/h   # NVS schema migration
│   │   ├── serial_prov.c       # Serial provisioning (UART + USB-Serial/JTAG)
│   │   ├── sntp.c              # NTP sync for TX stagger scheduling
│   │   ├── led.c               # LED control (identify, OTA progress, status)
│   │   ├── transport.c/h       # Transport layer abstractions
│   │   ├── spaxel.h            # Common headers
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
│   ├── internal/              # All internal packages
│   │   ├── ingestion/         # WebSocket server, binary frame parsing
│   │   ├── pipeline/          # Signal processing pipeline
│   │   │   ├── phase/        # Phase sanitization (unwrap, OLS, residual)
│   │   │   ├── nbvi/         # NBVI subcarrier selection
│   │   │   ├── feature/      # deltaRMS, phase variance, breathing band
│   │   │   └── baseline/     # EMA baseline, diurnal slots, snapshots
│   │   ├── localizer/        # Fusion & localization
│   │   │   ├── fresnel/      # Zone number cache, grid accumulation
│   │   │   ├── ukf/          # Biomechanical UKF (gonum/mat)
│   │   │   ├── gdop/         # Fisher information matrix, GDOP computation
│   │   │   └── fusion/       # Full localization loop (10 Hz)
│   │   ├── fleet/            # Node registry, role assignment, stagger scheduler
│   │   ├── ble/              # BLE centroid, rotation heuristics, identity matching
│   │   ├── portal/           # Crossing detection, zone occupancy
│   │   ├── replay/           # CSI replay buffer reader/writer
│   │   ├── anomaly/          # Pattern model (Welford), anomaly scoring
│   │   ├── predict/          # Presence prediction model
│   │   ├── sleep/            # Sleep state machine, breathing FFT
│   │   ├── flow/             # Crowd flow accumulator, dwell heatmap
│   │   ├── notify/           # Notification rendering (fogleman/gg)
│   │   ├── mqtt/             # MQTT client, HA auto-discovery
│   │   ├── auth/             # HMAC token derivation, bcrypt PIN, sessions
│   │   ├── oui/              # OUI lookup table (go:generate from IEEE)
│   │   ├── db/               # SQLite open/migrate, schema migrations
│   │   ├── config/           # Environment variable parsing
│   │   ├── apdetector/       # AP auto-detection for passive radar
│   │   ├── automation/       # Spatial automation (triggers, volumes)
│   │   ├── autoupdate/       # OTA auto-update with canary deployment
│   │   ├── doctor/           # System health diagnostics
│   │   ├── falldetect/       # Fall detection algorithm
│   │   ├── floorplan/        # Floor plan image management
│   │   ├── guidedtroubleshoot/ # Contextual help system
│   │   ├── health/           # Health check endpoints
│   │   ├── help/             # Help system
│   │   ├── loadshed/         # Load shedding under high CPU
│   │   ├── ntpserver/        # NTP server for testing
│   │   ├── notifications/   # Notification channel management
│   │   ├── ota/              # OTA manager, firmware serving
│   │   ├── prediction/       # Presence prediction engine
│   │   ├── provisioning/    # Node provisioning, token generation
│   │   ├── recorder/         # CSI recording to disk
│   │   ├── recording/        # CSI replay storage
│   │   ├── render/          # 2D rendering for notifications
│   │   ├── shutdown/        # Graceful shutdown orchestration
│   │   ├── simulator/       # CSI simulator interfaces
│   │   ├── sleep/            # Sleep quality monitoring
│   │   ├── startup/          # Startup sequencing phases
│   │   ├── timeline/         # Activity timeline
│   │   ├── tracker/          # Blob tracking
│   │   ├── volume/           # 3D volume handling
│   │   └── webhook/          # Webhook delivery
│   ├── test/                 # Mothership tests
│   ├── tests/                # Additional test files
│   └── build/                # Go build output
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
├── cmd/sim/                   # CSI simulator CLI (separate Go module)
│   └── main.go              # Simulator entry point
│
├── test/acceptance/          # Cross-cutting acceptance tests (Go module)
│
├── tests/e2e/               # Shell-based E2E test harness
│   └── run.sh              # E2E test runner
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

**Internal Packages (50+):**
All subsystems are independent internal packages under `internal/`:
- `ingestion` — WebSocket server, frame parsing
- `pipeline/*` — Signal processing stages
- `localizer/*` — Fusion, Fresnel, UKF, GDOP
- `fleet` — Node management
- `ble` — BLE device registry, identity matching
- `portal` — Crossing detection, zone occupancy
- `replay` — CSI replay buffer
- `anomaly` — Pattern learning, anomaly scoring
- `predict` — Presence prediction
- `sleep` — Sleep quality monitoring
- `flow` — Crowd flow visualization
- `notify` — Notification rendering
- `mqtt` — Home Assistant integration
- `auth` — Token derivation, PIN, sessions
- `db` — Schema migrations
- `config` — Environment parsing
- And 30+ more specialized packages

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

### Build & Test Modules

**`cmd/sim/`** — CSI simulator CLI (separate Go module)
- Generates synthetic CSI frames for development/testing
- Multi-node, multi-walker simulation
- BLE advertisement simulation

**`test/acceptance/`** — Cross-cutting acceptance tests (Go module)
- Simulator-based integration tests
- Validates full new-user journey

**`tests/e2e/`** — Shell-based E2E harness
- `run.sh` — End-to-end test runner

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

This catalog serves as the foundation for classifying which paths trigger builds:

### Firmware Build Triggers
- **Firmware sources:** `firmware/main/*.c`, `firmware/main/*.h`
- **ESP-IDF config:** `firmware/sdkconfig.defaults`, `firmware/partitions.csv`
- **Build scripts:** `firmware/scripts/`
- **Dependencies:** `firmware/managed_components/`

### Mothership Build Triggers
- **Go sources:** `mothership/**/*.go`
- **Go modules:** `mothership/go.mod`, `go.work`, `go.work.sum`
- **Embedded assets:** `dashboard/**` (any change triggers rebuild due to `//go:embed`)
- **Templates:** Any Go templates

### Dashboard Updates (No Rebuild Required)
- **Dashboard static files** are hot-reloadable during development
- But **changes are embedded at build time** for production

### Documentation Changes
- **No build impact:** `docs/**`, `*.md`, `scripts/`
- These are purely informational

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
- **Go workspace:** Three separate Go modules are stitched together by `go.work`: `mothership/`, `cmd/sim/`, `test/acceptance/`.
- **Firmware artifact:** The merged binary at `firmware/build/spaxel-firmware` is the OTA payload baked into the Docker image.
- **Bead backend:** Uses `bead` CLI (bead-rs) as of 2026-08-14; deprecated `bf` CLI should not be used.
