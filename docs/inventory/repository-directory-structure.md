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
├── mothership/          # Go backend (primary application)
├── firmware/            # ESP32-S3 firmware (ESP-IDF C project)
├── dashboard/           # Frontend web interface
├── cmd/                 # Additional CLI tools
├── docs/                # Documentation and plans
├── scripts/             # Utility scripts
├── tests/               # End-to-end integration tests
├── test/                # Acceptance tests (Go module)
├── testdata/            # Test fixtures and data
├── data/                # Runtime data directory (gitignored)
├── notes/               # Development notes (gitignored)
├── memory/              # Agent memory (gitignored)
└── [System directories] # .git, .beads, .claude, .marathon
```

---

## Major Components

### 1. Mothership (`mothership/`)

**Purpose:** Go backend server — ingestion, pipeline, localization, fleet manager, dashboard server, and all storage.

```
mothership/
├── cmd/                 # Application entry points
│   └── mothership/      # Main application (dashboard embedded via go:embed)
├── internal/            # Internal packages (55 packages)
│   ├── ingestion/        # WebSocket server, binary frame parsing, node lifecycle
│   ├── pipeline/        # Signal processing pipeline
│   │   ├── phase/       # Phase sanitization (unwrap, OLS, residual)
│   │   ├── nbvi/        # NBVI subcarrier selection
│   │   ├── feature/     # deltaRMS, phase variance, breathing band
│   │   └── baseline/    # EMA baseline, diurnal slots, snapshot persistence
│   ├── localizer/       # Fusion & localization engine
│   │   ├── fresnel/     # Zone number cache, grid accumulation
│   │   ├── ukf/         # Biomechanical UKF (gonum/mat)
│   │   ├── gdop/        # Fisher information matrix, GDOP computation
│   │   └── fusion/      # Full localization loop (10 Hz)
│   ├── fleet/           # Node registry, role assignment, stagger scheduler
│   ├── ble/             # BLE centroid, rotation heuristics, identity matching
│   ├── portal/          # Crossing detection, zone occupancy
│   ├── replay/          # CSI replay buffer reader/writer
│   ├── anomaly/         # Pattern learning, anomaly scoring
│   ├── predict/         # Presence prediction model
│   ├── sleep/           # Sleep state machine, breathing FFT
│   ├── flow/            # Crowd flow accumulator
│   ├── notify/          # Notification renderer (fogleman/gg)
│   ├── mqtt/            # MQTT client, HA auto-discovery
│   ├── auth/            # HMAC token derivation, bcrypt PIN, sessions
│   ├── oui/             # OUI lookup table generation
│   ├── db/              # SQLite open/migrate, schema migrations
│   ├── config/          # Environment variable parsing
│   ├── api/             # REST API handlers
│   ├── automation/      # Spatial automation trigger system
│   ├── autoupdate/      # OTA auto-update manager
│   ├── briefing/        # Morning briefing generation
│   ├── dashboard/       # Dashboard WebSocket feed
│   ├── diagnostics/     # Link health diagnostics
│   ├── doctor/          # System health checking
│   ├── eventbus/        # Event publishing system
│   ├── events/          # Event storage and querying
│   ├── explainability/  # Detection explanation ("Why is this here?")
│   ├── falldetect/      # Fall detection algorithm
│   ├── floorplan/       # Floor plan management
│   ├── github/          # GitHub API client
│   ├── guidedtroubleshoot/ # Contextual help system
│   ├── health/          # Health check endpoints
│   ├── help/            # Help article system
│   ├── learning/        # Machine learning components
│   ├── loadshed/        # Load shedding under pressure
│   ├── localization/    # Localization state management
│   ├── ntpserver/       # NTP server configuration
│   ├── ota/             # OTA update server
│   ├── prediction/      # Presence prediction (alias for predict?)
│   ├── provisioning/    # Node provisioning flow
│   ├── recorder/        # CSI recording management
│   ├── recording/       # Recording buffer implementation
│   ├── render/          # Notification image rendering
│   ├── shutdown/        # Graceful shutdown
│   ├── signal/          # Signal processing utilities
│   ├── simulator/       # CSI simulator integration
│   ├── startup/         # Startup sequencing
│   ├── timeline/        # Activity timeline
│   ├── tracker/         # Entity tracking
│   ├── tracking/        # Position tracking
│   ├── volume/          # Trigger volume management
│   ├── webhook/         # Webhook client
│   └── zones/           # Zone management
├── build/               # Build output directory (gitignored)
├── test/                # Unit and integration tests
├── tests/               # Additional test files
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
├── main/                # Main application source
│   ├── main.c           # app_main(), startup sequencing
│   ├── wifi.c/h         # WiFi station, mDNS, captive portal
│   ├── csi.c/h          # Promiscuous mode, CSI callback
│   ├── ws.c/h           # WebSocket client
│   ├── ble.c/h          # BLE passive scan
│   ├── ota.c/h          # OTA download, verification
│   ├── nvs.c/h          # NVS read/write helpers
│   ├── serial_prov.c    # Serial provisioning listener
│   ├── sntp.c/h         # NTP sync
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
│   ├── test_nvs.c       # NVS schema migration tests
│   ├── test_csi.c       # Binary frame serialization tests
│   └── test_serial_prov.c  # Provisioning parser + fuzz tests
├── docs/                # Firmware-specific documentation
├── scripts/             # Build/utility scripts
├── CMakeLists.txt       # Top-level project configuration
├── partitions.csv       # Partition layout (factory + ota_0 + ota_1 + nvs)
├── sdkconfig            # Current active configuration (gitignored?)
├── sdkconfig.defaults   # Project-specific defaults
├── sdkconfig.old        # Previous configuration
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

### 4. CLI Tools (`cmd/`)

**Purpose:** Additional command-line utilities beyond the main mothership binary.

```
cmd/
└── sim/                 # CSI simulator CLI (spaxel-sim)
    └── (Go source for simulator)
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
tests/                    # Shell-based E2E test harness
└── e2e/
    └── run.sh          # End-to-end test runner script

test/                     # Acceptance tests (separate Go module)
└── acceptance/           # Simulator-based acceptance tests
    ├── [test files]    # Integration tests using spaxel-sim
    └── go.mod          # Separate module for cross-cutting tests

testdata/                 # Test fixtures and data
└── [test data files]    # Static test data, fixtures

mothership/test/           # In-module mothership tests
mothership/tests/          # Additional mothership test files
firmware/test/            # Host-based firmware tests (gcc harness)
```

**Testing Strategy:**
1. **Unit tests:** Package-level `_test.go` files
2. **Integration tests:** `test/acceptance/` using `spaxel-sim`
3. **E2E tests:** `tests/e2e/run.sh` shell harness
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

The repository uses a **Go workspace** structure (`go.work` at root) stitching together three separate Go modules:

1. **`mothership/`** — Primary backend module (`mothership/go.mod`)
2. **`cmd/sim/`** — Simulator CLI module (`cmd/sim/go.mod`)  
3. **`test/acceptance/`** — Cross-cutting acceptance tests (`test/acceptance/go.mod`)

```
go.work
mothership/go.mod        # Main application module
cmd/sim/go.mod           # Simulator tool module
test/acceptance/go.mod   # Acceptance tests module
```

There is **no single root Go module** — the workspace pattern allows each component to have its own dependencies.

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
2. **Implementation status:** `PROGRESS.md` (check if exists) for current progress
3. **Testing:** Run `go test ./...` from `mothership/` for unit tests
4. **Firmware build:** Requires ESP-IDF environment setup before building
5. **Dashboard development:** No build step required — edit files and refresh browser

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

**Document Version:** 1.0  
**Last Updated:** 2026-08-29  
**Maintained in:** `docs/inventory/repository-directory-structure.md`
