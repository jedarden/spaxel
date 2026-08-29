# Spaxel Codebase Structure and Test Patterns

**Generated:** 2026-08-29
**Repository:** /home/coding/spaxel
**Project:** WiFi CSI-based indoor positioning system

---

## Overview

Spaxel is a multi-language project with distinct components for the mothership (Go backend), firmware (ESP-IDF C for ESP32-S3), and dashboard (Vanilla JavaScript + Three.js). The project follows a modular architecture with clear separation of concerns.

---

## Major Directories

### Top-Level Directories

```
/home/coding/spaxel/
├── cmd/                    # Go command-line tools
├── dashboard/              # Web UI (Vanilla JS + Three.js)
├── data/                   # Runtime data directory
├── docs/                   # Documentation
├── firmware/               # ESP32-S3 firmware (ESP-IDF C)
├── memory/                 # Project memory files
├── mothership/            # Go backend (primary application)
├── notes/                  # Working notes
├── scripts/                # Utility scripts
├── test/                   # Acceptance tests (Go)
├── testdata/              # Test data files
└── tests/                  # E2E tests (Go)
```

### Mothership Structure (Go Backend)

```
mothership/
├── cmd/                    # Application entrypoints
│   └── mothership/         # Main binary
├── internal/               # Internal packages (55+ packages)
│   ├── analytics/         # Analytics processing
│   ├── apdetector/        # Access Point auto-detection
│   ├── api/               # REST API handlers
│   ├── auth/              # Authentication (HMAC, bcrypt, sessions)
│   ├── automation/        # Spatial automation triggers
│   ├── autoupdate/        # OTA auto-update manager
│   ├── ble/               # BLE scanning and identity matching
│   ├── briefing/          # Morning briefing generation
│   ├── config/            # Configuration management
│   ├── dashboard/         # Dashboard WebSocket feed
│   ├── db/                # SQLite database and migrations
│   ├── diagnostics/       # System diagnostics
│   ├── doctor/            # Health check subsystem
│   ├── eventbus/          # Event bus
│   ├── events/            # Event logging and timeline
│   ├── explainability/    # Detection explainability
│   ├── falldetect/        # Fall detection
│   ├── fleet/             # Fleet manager (node registry, roles)
│   ├── floorplan/         # Floor plan management
│   ├── fusion/            # Multi-sensor fusion engine
│   ├── github/            # GitHub API client
│   ├── guidedtroubleshoot/# Guided troubleshooting
│   ├── health/            # Health monitoring
│   ├── help/              # Help system
│   ├── ingestion/         # WebSocket ingestion server
│   ├── learning/          # Machine learning (anomaly, prediction)
│   ├── loadshed/          # Load shedding under pressure
│   ├── localization/      # Spatial localization
│   ├── localizer/         # Localization algorithms
│   ├── mqtt/              # MQTT client integration
│   ├── notifications/     # Notification delivery
│   ├── notify/            # Notification rendering
│   ├── ntpserver/         # NTP server
│   ├── ota/               # OTA firmware update system
│   ├── oui/               # OUI lookup table
│   ├── prediction/       # Presence prediction
│   ├── provisioning/     # Node provisioning
│   ├── recorder/          # CSI recording
│   ├── recording/         # Recording management
│   ├── replay/            # CSI replay system
│   ├── render/            # Rendering utilities
│   ├── shutdown/          # Graceful shutdown
│   ├── signal/            # Signal processing pipeline
│   ├── simulator/         # CSI simulator
│   ├── sleep/             # Sleep quality monitoring
│   ├── startup/           # Startup sequencing
│   ├── timeline/          # Timeline management
│   ├── tracker/           # Blob tracking
│   ├── tracking/          # Tracking algorithms
│   ├── volume/            # 3D spatial volumes
│   ├── webhook/           # Webhook delivery
│   └── zones/             # Zone management
├── test/                   # Mothership unit tests
└── tests/
    └── e2e/               # Mothership E2E tests
```

### Firmware Structure (ESP-IDF C)

```
firmware/
├── main/                   # Main firmware component
│   ├── *.c                # C source files (10 files)
│   ├── *.h                # C header files (10 files)
│   ├── CMakeLists.txt     # Component build configuration
├── test/                   # Host-based firmware tests (gcc harness)
│   ├── test_*.c           # Test source files
│   ├── test_runner.c      # Test runner
├── docs/                   # Firmware documentation
├── scripts/                # Firmware utility scripts
└── managed_components/     # ESP-IDF components (third-party)
```

### Dashboard Structure (JavaScript)

```
dashboard/
├── js/                     # JavaScript source (80+ files)
│   ├── *.js               # Application code
│   ├── *.test.js         # Test files (20 files)
├── css/                    # Stylesheets
├── static/                 # Static assets
├── types/                  # TypeScript definitions
├── tests/                  # Test files
└── test-results/           # Test output directory
```

---

## Programming Languages Used

| Language | Component | Purpose | Notes |
|----------|-----------|---------|-------|
| **Go** | mothership/, cmd/, test/ | Backend application | Primary language for mothership, simulator CLI, acceptance tests |
| **C** | firmware/main, firmware/test | ESP32-S3 firmware | ESP-IDF framework, WiFi CSI, BLE, WebSocket |
| **JavaScript** | dashboard/ | Web UI | Vanilla JS + Three.js (no build toolchain) |
| **Shell** | scripts/, tests/e2e/ | Automation | Bash scripts for CI, E2E orchestration |

---

## Test File Patterns

### Go Tests

**Pattern:** `*_test.go`

**Locations:**
- `mothership/internal/*` — 134 test files across internal packages
- `test/acceptance/` — Acceptance test suite
- `mothership/test/` — Unit tests for specific subsystems
- `mothership/tests/e2e/` — End-to-end tests

**Examples:**
```
mothership/internal/ingestion/server_test.go
test/acceptance/as1_setup_test.go
mothership/tests/e2e/e2e_test.go
```

**Test Types:**
- Unit tests: Package-level `*_test.go` files
- Integration tests: `integration_test.go` in test/acceptance/
- Acceptance tests: `as*_test.go` (AS-1 through AS-7 scenarios)
- E2E tests: `e2e_test.go` with scenario-specific tests

---

### JavaScript Tests

**Pattern:** `*.test.js`

**Location:** `dashboard/js/`

**Count:** 20 test files

**Examples:**
```
ambient.test.js
blob-identity.test.js
backward-compat.test.js
```

**Test Types:**
- Unit tests for individual components
- Integration tests for UI workflows
- Setup/teardown: `*.test.setup.js` files

---

### C Firmware Tests

**Patterns:**
- `test_*.c` — Test source files
- `*_test.c` — Alternative naming convention

**Location:** `firmware/test/`

**Files:**
```
test_csi_frame.c
test_nvs_migration.c
test_serial_prov.c
test_sanity.c
test_all_restart_trigger_points.c
test_ota_during_wifi_reconnect.c
test_wifi_restart_race.c
test_console_config.c
test_runner.c (harness)
```

**Test Execution:** Host-based gcc harness (not ESP-IDF target testing)

---

## Test-Related Directories

### Primary Test Directories

| Directory | Purpose | Language |
|------------|---------|----------|
| `test/` | Acceptance tests | Go |
| `tests/` | E2E tests | Go |
| `testdata/` | Test fixtures/data | Various |
| `mothership/test/` | Mothership unit tests | Go |
| `mothership/tests/e2e/` | Mothership E2E tests | Go |
| `firmware/test/` | Firmware host tests | C |
| `dashboard/tests/` | Dashboard tests | JavaScript |
| `dashboard/test-results/` | Test output | N/A |

### Documentation Test Directories

- `docs/tests/` — Test documentation and plans

---

## Module Structure (Go Modules)

The project uses Go workspaces with multiple modules:

1. **`mothership`** — Main backend module
   - `go.mod` at `/home/coding/spaxel/mothership/go.mod`
   - Contains the primary application logic

2. **`cmd/sim`** — CSI simulator CLI
   - `go.mod` at `/home/coding/spaxel/cmd/sim/go.mod`
   - Standalone simulator tool

3. **`test/acceptance`** — Acceptance test suite
   - `go.mod` at `/home/coding/spaxel/test/acceptance/go.mod`
   - Cross-cutting acceptance tests

4. **Workspace coordination**
   - `go.work` at repository root
   - Stitches together the three modules

---

## Build Artifacts and Generated Files

### Generated/Build Directories (Excluded from Analysis)

```
.git/                    # Git metadata
.beads/                  # Bead tracking data
build/                   # Build artifacts (firmware)
node_modules/            # NPM dependencies (if present)
managed_components/      # ESP-IDF managed components
```

### Runtime Data Directories

```
data/
├── backups/             # Database backups
├── csi/                 # CSI replay buffer
├── firmware/            # Firmware storage
├── floorplan/           # Floor plan images
└── simulator/           # Simulator state
```

---

## Key Architectural Patterns

### Mothership (Go)
- **Package organization:** 55+ internal packages under `internal/`
- **Testing strategy:** Table-driven tests alongside implementation
- **No CGO:** Pure Go libraries only (modernc.org/sqlite)
- **Entry points:** `cmd/mothership/` for main binary

### Firmware (ESP-IDF C)
- **Component structure:** Single `main/` component
- **Testing:** Host-based gcc harness in `firmware/test/`
- **Build system:** CMake (ESP-IDF)
- **Target:** ESP32-S3 chip

### Dashboard (JavaScript)
- **No build step:** Vanilla JS loaded directly
- **Testing:** 20 `*.test.js` files
- **3D rendering:** Three.js via CDN

---

## Configuration Files

| File | Purpose | Language/System |
|------|---------|-----------------|
| `go.mod` / `go.work` | Go module definition | Go |
| `CMakeLists.txt` | ESP-IDF build config | CMake |
| `package.json` | Node dependencies (if any) | NPM |
| `Dockerfile` | Container image build | Docker |
| `docker-compose.yml` | Deployment manifest | Docker Compose |
| `sdkconfig.defaults` | ESP-IDF project config | ESP-IDF |

---

## Entry Points

### Application Entry Points

1. **Mothership backend:** `mothership/cmd/mothership/main.go`
2. **CSI simulator:** `cmd/sim/main.go`
3. **Dashboard:** Served static files from `dashboard/`
4. **Firmware:** `firmware/main/main.c` (ESP32-S3 app_main)

### Test Entry Points

1. **Go tests:** Run via `go test ./...` in respective modules
2. **Firmware tests:** `make -C firmware/test test` (gcc harness)
3. **JavaScript tests:** Test files in `dashboard/js/*.test.js`

---

## Summary Statistics

| Component | Directories | Test Files | Primary Language |
|-----------|------------|------------|-------------------|
| Mothership | 60+ | 134+ `*_test.go` | Go |
| Firmware | 8 | 10 `test_*.c` | C (ESP-IDF) |
| Dashboard | 7 | 20 `*.test.js` | JavaScript |
| Test Suite | 3 | 9 acceptance scenarios | Go |

---

## Development Workflow

### Running Tests

**Go (Mothership):**
```bash
cd mothership && go test ./...
```

**Go (Acceptance):**
```bash
cd test/acceptance && go test ./...
```

**Firmware (host-based):**
```bash
make -C firmware/test test
```

**Building Firmware:**
```bash
cd firmware && idf.py build
```

**Building Mothership:**
```bash
cd mothership && go build ./cmd/mothership
```

---

## Notes

- **No Rust code** identified in the current codebase
- **Test organization:** Separate test directories at each level (unit, integration, acceptance, E2E)
- **Modular architecture:** Clear boundaries between mothership, firmware, and dashboard
- **Cross-language testing:** Go tests can drive firmware simulator for integration testing
