# Spaxel Repository Directory Structure Inventory

**Generated:** 2026-08-29  
**Repository:** spaxel (WiFi CSI-based indoor positioning system)  
**Format:** Structured catalog with directories grouped by purpose

---

## Overview

Spaxel is a WiFi CSI-based indoor positioning system consisting of three primary components:
- **Mothership:** Go backend server (`mothership/`)
- **Firmware:** ESP32-S3 embedded software (`firmware/`)  
- **Dashboard:** Vanilla JS + Three.js frontend (`dashboard/`)

The repository follows a Go module structure with ESP-IDF firmware as a major sub-component.

---

## Top-Level Directory Structure

```
spaxel/
├── mothership/          # Go module: backend + simulator + all Go tests
├── firmware/            # ESP32-S3 firmware (ESP-IDF C project)
├── dashboard/           # Frontend web interface
├── docs/                # Documentation and plans
├── scripts/             # Utility scripts
├── testdata/            # CSI-recording utilities (//go:build ignore, in no module)
├── data/                # Captured runtime state (SQLite, backups) — not source
├── notes/               # Per-bead investigation findings (belongs under docs/notes/)
├── memory/              # Agent memory notes
└── [System directories] # .git, .beads, .claude, .marathon
```

---

## Root-Level Files (tracked)

| File | Purpose |
|------|---------|
| `Dockerfile` | 3-stage image build: firmware fetch → Go build → distroless runtime |
| `docker-compose.yml` | Single-service deployment manifest (host networking) |
| `.dockerignore` | Docker build exclusions |
| `VERSION` | Release version — single source of truth, consumed by the build and OTA filenames |
| `go.work` / `go.work.sum` | Go workspace definition (one module: `./mothership`) |
| `.gitignore` / `.gitattributes` | VCS controls |
| `.golangci.yml` | golangci-lint configuration (v2) |
| `.needle.yaml` / `.needle-predispatch-sha` | NEEDLE fleet-dispatch config and pre-dispatch SHA tracking |
| `README.md` | Project overview and quickstart |
| `PROGRESS.md` | Phase-by-phase implementation status |
| `LICENSE` | Project license |
| `*.md` investigation reports | Point-in-time work-log reports (API status, dashboard discovery/access, GDOP guide, verification summaries, …). Durable documentation lives under `docs/`; disposition of the root set is tracked by the repo-root exhaust sweep (`spaxel-1b8df9a3`) |
| `acceptance-test-hang-workflow.yml` | Argo Workflow draft left at the root while debugging acceptance hangs (disposition owned by a separate sweep) |

---

## Major Components

### 1. Mothership (`mothership/`)

**Purpose:** Go backend server — ingestion, pipeline, localization, fleet manager, dashboard server, and all storage.

```
mothership/
├── cmd/                 # Application entry points
│   ├── mothership/      # Main application (dashboard embedded via go:embed)
│   └── sim/             # spaxel-sim CLI — the simulator the Docker image ships
├── internal/            # Internal packages (56 packages)
│   ├── ingestion/        # WebSocket server, binary frame parsing, node lifecycle
│   ├── signal/          # Signal processing (flat package: phase sanitization, features,
│   │                    #   breathing, baseline, diurnal, ambient, persistence, processor)
│   ├── localizer/       # Fusion & localization engine
│   │   └── fusion/      # Full localization loop (10 Hz)
│   ├── fleet/           # Node registry, role assignment, stagger scheduler
│   ├── ble/             # BLE centroid, rotation heuristics, identity matching
│   ├── replay/          # CSI replay buffer reader/writer
│   ├── sleep/           # Sleep state machine, breathing FFT
│   ├── notify/          # Notification renderer (fogleman/gg)
│   ├── mqtt/            # MQTT client, HA auto-discovery
│   ├── auth/            # HMAC token derivation, bcrypt PIN, sessions
│   ├── oui/             # OUI lookup table generation
│   ├── db/              # SQLite open/migrate, schema migrations
│   ├── config/          # Environment variable parsing
│   ├── api/             # REST API handlers
│   ├── analytics/       # Anomaly scoring, patterns, crowd flow, alert chain
│   ├── apdetector/      # AP auto-detection for passive radar
│   ├── automation/      # Spatial automation trigger system
│   ├── autoupdate/      # OTA auto-update manager
│   ├── beads/           # Bead-state diagnostics helpers
│   ├── briefing/        # Morning briefing generation
│   ├── dashboard/       # Dashboard WebSocket feed
│   ├── diagnostics/     # Link health diagnostics
│   ├── diskspace/       # Disk space monitoring
│   ├── doctor/          # System health checking
│   ├── eventbus/        # Event publishing system
│   ├── events/          # Event storage and querying
│   ├── explainability/  # Detection explanation ("Why is this here?")
│   ├── falldetect/      # Fall detection algorithm
│   ├── floorplan/       # Floor plan management
│   ├── fusion/          # 3D grid fusion engine
│   ├── github/          # GitHub API client
│   ├── guidedtroubleshoot/ # Contextual help system
│   ├── health/          # Health check endpoints
│   ├── help/            # Help article system
│   ├── learning/        # Machine learning components
│   ├── loadshed/        # Load shedding under pressure
│   ├── localization/    # Localization state management
│   ├── logging/         # Shared logging setup
│   ├── ntpserver/       # NTP server configuration
│   ├── ota/             # OTA update server
│   ├── prediction/      # Presence prediction engine
│   ├── provisioning/    # Node provisioning flow
│   ├── recorder/        # CSI recording management
│   ├── recording/       # Recording buffer implementation
│   ├── render/          # Notification image rendering
│   ├── shutdown/        # Graceful shutdown
│   ├── simulator/       # CSI simulator integration
│   ├── startup/         # Startup sequencing
│   ├── timeline/        # Activity timeline
│   ├── tracker/         # Entity tracking + BLE identity
│   ├── tracking/        # UKF tracking core
│   ├── types/           # Shared log-level types
│   ├── volume/          # Trigger volume management
│   ├── webhook/         # Webhook client
│   └── zones/           # Zone management
├── build/               # Build output directory (gitignored)
├── test/acceptance/     # Acceptance scenarios AS-1…AS-7 + IO install/upgrade tests
├── tests/e2e/           # End-to-end Go tests (e2e_test.go, IO-6 gate)
├── go.mod               # Go module definition
├── go.sum               # Go module checksums
├── mothership           # Compiled binary (gitignored)
├── sim                  # Simulator binary (gitignored)
├── *.test              # Test binaries (gitignored)
└── [build artifacts]    # Various compiled outputs
```

**Go Module:** `mothership/go.mod`  
**Entry Point:** `mothership/cmd/mothership/main.go`

---

### 2. Firmware (`firmware/`)

**Purpose:** ESP32-S3 firmware — WiFi CSI capture, BLE scanning, OTA updates, WebSocket communication.

```
firmware/
├── main/                # Main application source (12 .c files + headers)
│   ├── main.c           # app_main(), startup sequencing
│   ├── wifi.c/h         # WiFi station, mDNS, captive portal
│   ├── csi.c/h          # Promiscuous mode, CSI callback
│   ├── websocket.c/h    # WebSocket client (binary CSI up, JSON config down)
│   ├── transport.c/h    # Transport abstraction (UART0 + USB-Serial/JTAG)
│   ├── ble.c/h          # BLE passive scan
│   ├── ntp.c/h          # NTP sync for TX stagger slots
│   ├── nvs_migration.c/h # NVS read/write helpers + schema migration
│   ├── provision.c/h    # Serial provisioning listener
│   ├── safe_mode.c/h    # Safe-mode entry/recovery
│   ├── watchdog.c/h     # Task watchdog (esp_task_wdt)
│   ├── led.c/h          # LED control
│   └── CMakeLists.txt   # Component build configuration
├── build/               # ESP-IDF build output (gitignored)
│   ├── bootloader/      # Bootloader binary
│   ├── partition_table/ # Partition table
│   ├── spaxel-firmware.bin  # Merged firmware image
│   ├── spaxel-firmware.elf    # ELF binary
│   └── [build artifacts]
├── managed_components/  # ESP-IDF component manager
├── test/                # Host-based gcc tests (no hardware)
│   ├── test_runner.c    # Test harness
│   ├── test_nvs_migration.c      # NVS schema migration tests
│   ├── test_csi_frame.c          # Binary frame serialization tests
│   ├── test_serial_prov.c        # Provisioning parser + fuzz tests
│   ├── test_console_config.c     # Console routing config
│   ├── test_sanity.c             # Sanity checks
│   ├── test_wifi_restart_race.c  # WiFi restart race
│   ├── test_ota_during_wifi_reconnect.c
│   └── test_all_restart_trigger_points.c
├── docs/                # Firmware-specific documentation
├── scripts/             # Build/utility scripts
├── CMakeLists.txt       # Top-level project configuration
├── partitions.csv       # Partition layout (factory + ota_0 + ota_1 + nvs)
├── sdkconfig.defaults   # Project-specific defaults (committed; the active
│                        #   `sdkconfig` is generated and gitignored)
├── sdkconfig.uart-console      # UART console configuration variant
├── sdkconfig.usbjtag           # USB-Serial/JTAG console variant
├── BUILD.md             # Build instructions
├── CONTRIBUTING.md       # Contribution guidelines
├── README.md            # Firmware overview
└── dependencies.lock    # Component dependencies lock file
```

**Build System:** ESP-IDF 5.2.x  
**Architecture:** ESP32-S3 (dual-core Xtensa LX7)  
**Key Technologies:** WiFi CSI, BLE, WebSocket, OTA, NVS

---

### 3. Dashboard (`dashboard/`)

**Purpose:** Single-page application served by mothership — Three.js 3D visualization, UI controls.

```
dashboard/
├── index.html           # Main dashboard entry point (3D Live View)
├── simple.html          # Simple mode (progressive disclosure)
├── ambient.html         # Ambient mode for wall-mounted tablets
├── fleet.html           # Fleet status table view
├── setup.html           # Setup/Calibration interface
├── integrations.html    # Home automation integration settings
├── simulator.html       # Pre-deployment simulator interface
├── test-transformcontrols.html  # Testing page
├── help_articles.json   # Help content database
├── manifest.json        # Progressive Web App manifest
├── package.json         # Node.js dependencies
├── package-lock.json    # Dependency lock file
├── jest.config.js       # Jest test configuration
├── playwright.config.js # Playwright E2E test config
├── tsconfig.json        # TypeScript configuration
├── sw.js                # Service Worker for offline support
├── generate-icons.js    # Icon generation script
├── run-leak-profiling.js # Memory profiling tool
├── css/                 # Stylesheets
├── js/                  # JavaScript application code
│   └── (application modules)
├── types/               # TypeScript type definitions
├── static/              # Static assets
│   └── esptool-js/      # Web Serial flashing library
├── test-results/        # Test output (gitignored)
├── tests/               # Frontend tests
├── [Profiling reports]  # Memory leak analysis reports
└── README.md            # Dashboard documentation
```

**Technology Stack:** Vanilla JS + Three.js (no build toolchain required)  
**Embedded:** The entire dashboard is embedded into the Go binary via `//go:embed` at build time.

---

### 4. Simulator CLI (`mothership/cmd/sim/`)

**Purpose:** The CSI simulator CLI (`spaxel-sim`) — part of the mothership module,
not a separate one; this is the binary the Docker image ships.

```
mothership/cmd/sim/
├── main.go             # CLI entry point
├── generator.go        # Synthetic CSI frame generation
├── walker.go           # Synthetic walker motion
├── scenario.go         # Test scenario definitions
├── verify.go           # Result verification (--verify poll/assert)
├── main_test.go
├── Makefile
└── README.md
```

**Simulator Usage:** Emulates ESP32 nodes for testing without hardware. See `spaxel-sim --help`.

---

### 5. Documentation (`docs/`)

**Purpose:** Project documentation, research, plans, and design notes.

```
docs/
├── plan/                # Implementation plan
│   └── plan.md          # Master implementation plan (this document)
├── research/            # Third-party research and reference material
├── notes/               # Development notes and decisions
├── design/              # Design documents and ADRs
├── deployment/         # Deployment guides and configurations
├── examples/            # Example configurations and use cases
├── inventory/          # Repository structure inventories
├── tests/              # Test documentation and results
├── build-paths-catalog.md       # Build path reference
├── BUILD_PATHS.md              # Build system documentation
├── ci-accessibility-integration.md   # CI accessibility testing
├── ci-benchmark-integration.md      # CI benchmark integration
├── codebase-structure-and-test-patterns.md  # Code architecture
├── gdop-*.md                    # GDOP computation documentation
├── io-harness-blocking-analysis.md   # Test harness analysis
├── kaniko-version-research.md    # Container build research
├── profiling-data-validation-report.md  # Performance profiling
├── trace-*.md                  # tracing system documentation
├── wifi-credential-provisioning-flow.md  # WiFi setup docs
├── SYSTEM_CATALOG.md            # System catalog
└── [various analysis documents]
```

**Key Document:** `docs/plan/plan.md` is the single source of truth for implementation status and architecture.

---

### 6. Testing Directories

**Purpose:** Multi-level testing strategy from unit tests to hardware-in-the-loop integration.

```
mothership/test/acceptance/   # Acceptance scenarios AS-1…AS-7 (+ WiFi restart race),
                              #   integration_test.go, io_install_upgrade_test.go
mothership/tests/e2e/         # End-to-end Go tests: e2e_test.go, assertions_test.go,
                              #   io6_gate_test.go (+ _conclusion)
testdata/                     # CSI-recording utilities (//go:build ignore, in no module)
firmware/test/                # Host-based firmware tests (gcc harness, 9 test_*.c)
dashboard/tests/              # Playwright accessibility specs
dashboard/js/*.test.js        # Co-located jest unit tests
```

**Testing Strategy:**
1. **Unit tests:** Package-level `_test.go` files, co-located
2. **Acceptance tests:** `mothership/test/acceptance/` using `spaxel-sim`
3. **E2E tests:** `mothership/tests/e2e/` (Go, drives a running mothership + sim)
4. **Firmware tests:** `firmware/test/` with gcc (no hardware required)

---

### 7. Utility Scripts (`scripts/`)

**Purpose:** Automation and utility scripts for development and testing.

```
scripts/
├── flash-esp32s3.sh          # Firmware flashing utility
├── provision_esp32.py        # Device provisioning script
├── measure_csi_rate.py       # CSI rate measurement tool
├── run-sim-ble-fixture.sh   # BLE fixture simulator
├── run-sim-ble-match.sh     # BLE identity matching test
├── run-sim-dashboard-console.sh  # Dashboard console simulator
├── run-sim-identity.sh       # Identity testing simulator
├── run-sim-local.sh          # Local simulator runner
├── capture-dashboard-console.mjs  # Console capture utility
└── test-github-api.sh        # GitHub API validation
```

---

### 8. Runtime Data (`data/`)

**Purpose:** Runtime data storage — gitignored, mounted as Docker volume in production.

```
data/                      # Runtime data directory (gitignored)
├── backups/               # Database backups
├── csi/                   # CSI replay buffer (csi_replay.bin)
├── firmware/              # Firmware binaries for OTA
├── floorplan/             # Uploaded floor plan images
└── simulator/             # Simulator state/output
```

**Note:** Entire `data/` directory is excluded from git and designed for Docker volume mounting.

---

### 9. System and Configuration Directories

**Purpose:** Build system, version control, and agent configuration.

```
.git/                     # Git repository (version control)
├── [git metadata]        # Commits, branches, refs, objects

.beads/                   # Bead tracking system (bead-rs)
├── checkpoint/           # Bead checkpoint data
├── traces/               # Trace data
├── receipts/             # Receipts
└── recovery/             # Recovery data

.claude/                  # Claude Code agent configuration
└── (agent settings)

.marathon/                # Unknown system directory

~/.needle/                # NEEDLE fleet dispatch workspace
```

**Note:** These directories contain system state and configuration managed by tooling.

---

## Build Output and Scratch Directories (Exclude from Documentation)

The following directories contain build artifacts and should be excluded from documentation catalogs:

1. **`mothership/build/`** — Go build cache and binaries
2. **`firmware/build/`** — ESP-IDF build output
3. **`data/`** — Runtime data (backups, CSI buffer, firmware cache)
4. **`mothership/mothership`** — Compiled Go binary
5. **`mothership/sim`** — Compiled simulator binary
6. **`mothership/*.test`** — Compiled test binaries
7. **`notes/`** — Development scratch notes
8. **`memory/`** — Agent memory files

---

## Directory Size Summary

| Component | Approximate Size | Notes |
|-----------|------------------|-------|
| `mothership/` | ~150 MB | Mostly binaries in root |
| `firmware/` | ~50 MB | ESP-IDF build artifacts |
| `dashboard/` | ~20 MB | Static assets + dependencies |
| `docs/` | ~5 MB | Documentation and research |
| `data/` | Variable | Runtime data, can grow large |

---

## Go Module Architecture

The repository uses a **Go workspace** structure (`go.work` at root) with **one module**:

1. **`mothership/`** — the backend module (`mothership/go.mod`); the simulator CLI
   (`mothership/cmd/sim/`) and both test trees (`mothership/test/acceptance/`,
   `mothership/tests/e2e/`) live inside it

```
go.work
mothership/go.mod        # The only Go module (go 1.25.0)
```

There is **no root `go.mod`** and no second/third module — run `go` commands from
`mothership/`.

---

## ESP-IDF Project Structure

The `firmware/` directory is a complete ESP-IDF 5.2.x project with its own build system:

```
firmware/
├── CMakeLists.txt       # Top-level project file
├── sdkconfig            # Active configuration (generated)
├── sdkconfig.defaults   # Base configuration defaults
├── partitions.csv       # Partition table layout
├── main/                # Main component (source code)
│   ├── CMakeLists.txt   # Component configuration
│   └── [source files]   # .c/.h files
├── managed_components/ # ESP-IDF component dependencies
└── build/               # Build output (ESP-IDF managed)
```

**Build Command:** `idf.py -C firmware build` (requires ESP-IDF environment)

---

## Key File Types by Directory

| Directory | File Types | Purpose |
|-----------|------------|---------|
| `mothership/cmd/` | `.go` | Application entry points |
| `mothership/internal/` | `.go`, `_test.go` | Packages and tests |
| `firmware/main/` | `.c`, `.h` | Firmware source code |
| `firmware/test/` | `.c` | Host-based firmware tests |
| `dashboard/` | `.html`, `.js`, `.css`, `.json` | Frontend assets |
| `docs/` | `.md` | Documentation |
| `scripts/` | `.sh`, `.py`, `.mjs` | Utility scripts |

---

## Dependency External References

The repository integrates with external systems:

- **ESP-IDF v5.2.x** — Espressif IoT Development Framework for ESP32-S3
- **Three.js** — 3D graphics library for dashboard (loaded via CDN in development, bundled in production)
- **esptool-js** — Web Serial API library for firmware flashing (bundled in `dashboard/static/`)
- **Go modules** — Standard Go dependency management via `go.mod`

---

## Deployment Artifacts

**Docker Image:** Multi-arch build (`linux/amd64`, `linux/arm64`)  
**Firmware Binary:** Embedded in Docker image at `/firmware/spaxel-firmware-merged.bin`  
**Runtime Data Volume:** `/data` (SQLite database, CSI buffer, floor plans)

---

## Notes for Navigation

1. **Start here:** `docs/plan/plan.md` for complete system architecture
2. **Newer structural survey:** `docs/repo-structure.md` supersedes this inventory
   where the two disagree
3. **Implementation status:** `PROGRESS.md` for current progress
4. **Testing:** Run `go test ./...` from `mothership/` for unit tests
5. **Firmware build:** Built in CI (`spaxel-build` Argo workflow); a local build needs
   an ESP-IDF environment
6. **Dashboard development:** No build step required — edit files and refresh browser

---

## Inventory Completeness

This inventory covers:
- ✅ All top-level directories with purposes
- ✅ Subdirectories within major components
- ✅ Build output and scratch directories identified
- ✅ Go module architecture
- ✅ ESP-IDF project structure
- ✅ Testing infrastructure
- ✅ Documentation organization

**Excluded from this inventory:**
- Individual file listings (use `find` or `tree` for detailed file-level views)
- Git internal structure (`.git/` internals)
- Build artifacts in `build/` directories
- Runtime data in `data/` directory

---

**Document Version:** 1.1 (2026-09-04 — absorbed the deleted root `DIRECTORY_STRUCTURE.md`
root-level-files table and reconciled the module/test-tree layout; see `docs/repo-structure.md`
for the newest survey)  
**Generated:** 2026-08-29  
**Maintained in:** `docs/inventory/repository-directory-structure.md`
