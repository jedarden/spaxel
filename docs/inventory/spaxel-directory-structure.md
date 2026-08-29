# Spaxel Repository Directory Structure

**Last updated:** 2026-08-29

This document provides a comprehensive inventory of the Spaxel repository structure, organized by purpose and component.

---

## Top-Level Directories

### Core Source Components

| Directory | Purpose |
|-----------|---------|
| **`mothership/`** | Go backend - mothership container application. Contains the main server, WebSocket handlers, signal processing pipeline, localization engine, and fleet management. |
| **`firmware/`** | ESP32-S3 firmware written in C for ESP-IDF. Handles WiFi, CSI capture, BLE scanning, OTA updates, and WebSocket communication. |
| **`dashboard/`** | Web UI - vanilla JavaScript + Three.js 3D visualization. Served as static assets embedded in the mothership binary. |
| **`cmd/sim/`** | CSI simulator CLI (Go). Generates synthetic CSI data for development/testing without hardware. |

### Testing & Validation

| Directory | Purpose | Notes |
|-----------|---------|-------|
| **`test/acceptance/`** | Acceptance/integration tests (Go module) | Uses `spaxel-sim` for hardware-free validation |
| **`tests/e2e/`** | Shell-based end-to-end test harness | Orchestrates container + simulator runs |
| **`testdata/`** | Test fixtures and data | |

### Documentation

| Directory | Purpose |
|-----------|---------|
| **`docs/`** | All project documentation |
| **`docs/plan/`** | Implementation plan (`plan.md`) |
| **`docs/research/`** | External research, references, prior art |
| **`docs/notes/`** | Design decisions, feature notes, constraints |
| **`docs/design/`** | Design specifications |
| **`docs/deployment/`** | Deployment guides and procedures |
| **`docs/inventory/`** | Repository structure and source inventories |
| **`docs/examples/`** | Example configurations and usage |
| **`docs/tests/`** | Test documentation |

### Scripts & Automation

| Directory | Purpose |
|-----------|---------|
| **`scripts/`** | Utility scripts for flashing, provisioning, and testing |

### Build Outputs (Exclude from Documentation)

| Directory | Purpose | Exclude Reason |
|-----------|---------|-----------------|
| **`firmware/build/`** | ESP-IDF build artifacts | Build output |
| **`mothership/build/`** | Go build artifacts | Build output |
| **`~/`** | User home directory | Outside repository scope |

### Project Configuration & Meta

| Directory | Purpose |
|-----------|---------|
| **`.claude/`** | Claude Code configuration and worktrees |
| **`.marathon/`** | Marathon test configuration |
| **`memory/`** | Project memory/knowledge base |
| **`notes/`** | Additional notes (project-specific) |
| **`data/`** | Runtime data directory (mounted as Docker volume) |

---

## Major Component Subdirectories

### `mothership/` — Go Backend

```
mothership/
├── cmd/                          # Entry points
│   ├── mothership/              # Main mothership server (embedded dashboard)
│   └── sim/                     # Simulator CLI (separate Go module)
├── internal/                     # All internal packages (60+ packages)
│   ├── analytics/               # Analytics and metrics
│   ├── apdetector/              # AP BSSID auto-detection for passive radar
│   ├── api/                     # HTTP API handlers
│   ├── auth/                    # Authentication (PIN, sessions, tokens)
│   ├── automation/              # Spatial automation triggers
│   ├── autoupdate/              # Automatic OTA update management
│   ├── ble/                     # BLE scanning, identity matching
│   ├── briefing/                # Morning briefing generation
│   ├── config/                  # Configuration and env var handling
│   ├── dashboard/               # Dashboard WebSocket and embedding
│   ├── db/                      # SQLite database and migrations
│   ├── diagnostics/             # System diagnostics
│   ├── diskspace/               # Disk space monitoring
│   ├── doctor/                  # Doctor/recovery tools
│   ├── eventbus/                # Internal event bus
│   ├── events/                  # Event logging and timeline
│   ├── explainability/          # Detection explainability ("Why is this here?")
│   ├── falldetect/              # Fall detection logic
│   ├── fleet/                   # Fleet manager (role assignment, optimization)
│   ├── floorplan/               # Floor plan management
│   ├── fusion/                  # Fusion engine and localization loop
│   ├── github/                  # GitHub API integration
│   ├── guidedtroubleshoot/      # Guided troubleshooting prompts
│   ├── health/                  # Health check endpoints
│   ├── help/                    # Help articles and command palette
│   ├── ingestion/               # WebSocket ingestion server (node connections)
│   ├── learning/                # Machine learning (patterns, weights)
│   ├── loadshed/                # Load shedding under pressure
│   ├── localization/            # Blob localization (Fresnel, UKF)
│   ├── localizer/               # Localization algorithms
│   ├── mqtt/                    # MQTT client (Home Assistant integration)
│   ├── notifications/           # Notification channels and rendering
│   ├── notify/                  # Notification delivery
│   ├── ntpserver/               # NTP server configuration
│   ├── ota/                     # OTA firmware serving and triggering
│   ├── oui/                     # IEEE OUI lookup (router manufacturer names)
│   ├── prediction/              # Presence prediction models
│   ├── provisioning/            # Node provisioning and token generation
│   ├── recorder/                # CSI recorder
│   ├── recording/               # CSI recording/replay buffer management
│   ├── render/                  # Server-side rendering (PNG thumbnails)
│   ├── replay/                  # Time-travel replay engine
│   ├── shutdown/                # Graceful shutdown handling
│   ├── signal/                  # Signal processing pipeline
│   ├── simulator/               # Simulator client for testing
│   ├── sleep/                   # Sleep quality monitoring
│   ├── startup/                 # Startup sequencing
│   ├── timeline/                # Activity timeline
│   ├── tracker/                 # Blob tracking
│   ├── tracking/                # Person tracking
│   ├── volume/                  # 3D volume/zone management
│   ├── webhook/                 # Webhook delivery
│   └── zones/                   # Zone management
└── test/                        # Mothership acceptance tests
    └── acceptance/             # In-module acceptance tests
```

### `firmware/` — ESP32-S3 Firmware

```
firmware/
├── main/                         # Main ESP-IDF component
│   ├── ble.c / ble.h             # BLE passive scanning
│   ├── csi.c / csi.h             # CSI capture and processing
│   ├── led.c / led.h             # LED control (identify, OTA status)
│   ├── main.c                    # App entry point
│   ├── ntp.c / ntp.h             # NTP time synchronization
│   ├── nvs_migration.c/h         # NVS schema migration
│   ├── provision.c/h             # Serial provisioning handler
│   ├── spaxel.h                  # Firmware-wide header
│   ├── transport.c/h             # Transport layer abstraction
│   ├── version.h.in              # Version template (build-time generated)
│   ├── websocket.c/h             # WebSocket client
│   └── wifi.c / wifi.h           # WiFi connection and management
├── build/                        # ESP-IDF build output (exclude from docs)
├── docs/                         # Firmware documentation
├── managed_components/          # ESP-IDF managed components
├── scripts/                     # Firmware utility scripts
├── test/                         # Host-based tests (gcc harness, no hardware)
├── CMakeLists.txt               # ESP-IDF project configuration
├── partitions.csv                # Partition table (factory, OTA, NVS)
├── README.md                     # Firmware documentation
├── sdkconfig                     # Current ESP-IDF configuration
├── sdkconfig.defaults            # Default configuration overrides
├── sdkconfig.old                # Previous configuration (backup)
├── sdkconfig.uart-console        # UART0 console configuration
└── sdkconfig.usbjtag             # USB-Serial/JTAG console configuration
```

### `dashboard/` — Web UI

```
dashboard/
├── css/                          # Stylesheets (if any)
├── js/                           # JavaScript modules
├── static/                       # Static assets (Three.js, libraries)
│   └── esptool-js/               # Web Serial flashing library
├── test-results/                 # Test output (exclude from docs)
├── tests/                        # Dashboard tests
├── types/                        # TypeScript type definitions
├── ambient.html                  # Ambient mode (wall-mount display)
├── fleet.html                    # Fleet status panel
├── help_articles.json            # Help article content
├── index.html                    # Main entry point (live 3D view)
├── integrations.html             # Integration configuration
├── live.html                     # Live view (3D scene)
├── manifest.json                # Web app manifest
├── package.json                  # npm dependencies
├── setup.html                    # Setup/calibration view
├── simple.html                   # Simple mode (card-based UI)
├── simulator.html                # Simulator control panel
├── sw.js                         # Service worker
└── test-transformcontrols.html   # TransformControls test page
```

### `test/acceptance/` — Acceptance Tests

```
test/acceptance/
├── acceptance_test.go            # Main test suite
├── as1_setup_test.go            # AS-1: First-time setup
├── as2_walking_test.go          # AS-2: Person detected while walking
├── as3_fall_test.go             # AS-3: Fall alert
├── as4_ble_test.go              # AS-4: BLE identity resolution
├── as5_ota_test.go              # AS-5: OTA update
├── as6_replay_test.go           # AS-6: Time-travel replay
├── as7_auth_reject_test.go      # AS-7: Auth rejection
├── diagnostics.go                # Test diagnostics helpers
├── integration_test.go           # Integration test helpers
└── run_with_diagnostics.sh      # Test runner with diagnostics
```

### `scripts/` — Utility Scripts

```
scripts/
├── capture-dashboard-console.mjs    # Dashboard console capture
├── flash-esp32s3.sh                 # ESP32-S3 flashing helper
├── measure_csi_rate.py              # CSI rate measurement
├── provision_esp32.py                # Device provisioning helper
├── run-sim-ble-fixture.sh          # BLE fixture simulator
├── run-sim-ble-match.sh            # BLE matching simulator
├── run-sim-dashboard-console.sh    # Dashboard console simulator
├── run-sim-identity.sh             # Identity matching simulator
├── run-sim-local.sh                 # Local simulator
└── test-github-api.sh              # GitHub API test script
```

---

## Build Artifacts and Excluded Directories

### Build Outputs (Not Tracked in Documentation)

| Path | Purpose | Generated By |
|------|---------|-------------|
| `firmware/build/` | ESP-IDF build artifacts (object files, binaries) | `idf.py build` |
| `mothership/build/` | Go build artifacts | `go build` |
| `dashboard/node_modules/` | npm dependencies (if present) | `npm install` |

### Runtime Data (Docker Volume)

| Path | Purpose |
|------|---------|
| `data/` | Runtime data mount point (SQLite, CSI replay, firmware uploads) |
| `data/backups/` | Database backups |
| `data/csi/` | CSI replay buffer |
| `data/firmware/` | Firmware binary storage |
| `data/floorplan/` | Floor plan images |
| `data/simulator/` | Simulator state |

### Scratch and Temporary Directories

| Path | Purpose | Exclude Reason |
|------|---------|-----------------|
| `~/` | User home directory (scratch space) | Outside repository scope |
| `~/.needle/` | NEEDLE fleet work files | Temporary operational state |

---

## Go Module Layout

The repository uses a Go workspace (`go.work`) to stitch together three separate Go modules:

| Module Path | Purpose |
|------------|---------|
| `mothership/` | Primary backend (mothership binary) |
| `cmd/sim/` | CSI simulator CLI |
| `test/acceptance/` | Cross-cutting acceptance tests |

---

## Key Architecture Points

1. **Mothership is monolithic** — All Go backend code lives under `mothership/internal/` with clear separation of concerns (60+ packages).
2. **Dashboard is embedded** — Served as static assets via `go:embed` in `mothership/cmd/mothership/`.
3. **Firmware is standalone** — ESP-IDF C project with its own build system, independent of Go code.
4. **No integration test directory** — Integration tests live in `test/acceptance/` (cross-cutting module) and `mothership/test/acceptance/` (in-module).
5. **Testing spans three approaches**:
   - Unit tests: `*_test.go` alongside source
   - Integration tests: `test/acceptance/` (simulator-based)
   - Firmware tests: `firmware/test/` (host-based gcc harness)
   - E2E tests: `tests/e2e/run.sh` (shell harness)

---

## Summary

- **Total top-level directories:** 16 (excluding `.` and `.git`)
- **Major source components:** 4 (`mothership/`, `firmware/`, `dashboard/`, `cmd/sim/`)
- **Internal packages (Go):** 60+ packages under `mothership/internal/`
- **Testing approaches:** 4 (unit, integration, firmware host-tests, e2e shell harness)
- **Build output directories:** 2 (exclude from documentation)
- **Documentation categories:** 9 (plan, research, notes, design, deployment, inventory, examples, tests, general)

This structure supports:
- Clear separation between Go backend, ESP32 firmware, and web UI
- Hardware-free testing via simulator
- Multiple testing strategies (unit, integration, host-based, e2e)
- Extensive documentation organized by purpose
- Build artifacts and runtime data properly isolated from source
