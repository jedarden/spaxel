# Spaxel Build Paths Catalog

**Last updated:** 2026-08-27

This document identifies all file paths in the spaxel repository that contain substantive code requiring spaxel rebuilds and deployments. Changes to these paths should trigger appropriate build pipelines and deployment cycles.

## Overview

Spaxel is a multi-component system with separate build artifacts:
- **ESP32-S3 firmware** (C, ESP-IDF)
- **Mothership** (Go backend binary)
- **Dashboard** (static HTML/JS/CSS, embedded via `go:embed`)
- **CSI Simulator** (Go CLI tool)
- **Acceptance tests** (Go test modules)

---

## 1. Firmware (`firmware/`)

**Build system:** ESP-IDF v5.2.x CMake

**Paths requiring firmware rebuild:**

```
firmware/
├── main/                      # Main firmware source (ESP32-S3)
│   ├── main.c                # Entry point, startup sequencing
│   ├── wifi.c/h              # WiFi station, mDNS, captive portal
│   ├── csi.c/h               # CSI capture, processing, frame serialization
│   ├── ws.c/h                # WebSocket client (esp_websocket_client)
│   ├── ble.c/h               # BLE passive scan, advertisement parsing
│   ├── ota.c/h               # OTA download, SHA-256 verification
│   ├── nvs.c/h               # NVS helpers, provisioning storage
│   ├── nvs_migration.c/h     # NVS schema migration
│   ├── serial_prov.c/h       # Serial provisioning listener
│   ├── provision.c/h         # Provisioning JSON parser
│   ├── sntp.c/h              # NTP time sync
│   ├── led.c/h               # LED control (identify blink, OTA progress)
│   ├── transport.c/h          # Transport layer abstractions
│   └── CMakeLists.txt        # Component build config
├── test/                      # Host-based tests (gcc harness)
│   ├── test_*.c              # Unit tests (CSI framing, NVS migration, etc.)
│   ├── test_runner.c/h       # Test harness
│   └── Makefile              # Test build recipe
├── CMakeLists.txt            # Top-level project config
├── partitions.csv            # Flash partition layout (factory/ota_0/ota_1/nvs/otadata)
├── sdkconfig.defaults        # ESP-IDF configuration defaults
└── build/                    # ESP-IDF build output (generated)
```

**Trigger conditions:**
- Any change to `firmware/main/*.c` or `firmware/main/*.h` requires firmware rebuild
- Changes to `partitions.csv` or `sdkconfig.defaults` require full clean rebuild
- Test code changes (`firmware/test/*.c`) do not affect production firmware

**Build artifact:** `spaxel-firmware-merged.bin` (merged bootloader + partition table + app)

---

## 2. Mothership (`mothership/`)

**Build system:** Go 1.25+ modules

**Paths requiring mothership rebuild:**

```
mothership/
├── cmd/
│   └── mothership/
│       ├── main.go           # Application entrypoint, startup sequencing
│       └── dashboard/        # Dashboard static files (go:embed source)
│           ├── index.html
│           ├── *.html        # All dashboard pages
│           ├── css/
│           ├── js/
│           ├── static/
│           └── ...
├── internal/                 # All internal packages (30+ modules)
│   ├── ingestion/            # WebSocket server, binary frame parsing
│   ├── pipeline/             # Signal processing pipeline
│   │   ├── phase/           # Phase sanitization
│   │   ├── nbvi/            # NBVI subcarrier selection
│   │   ├── feature/         # deltaRMS, breathing band
│   │   └── baseline/        # EMA baseline, diurnal slots
│   ├── localizer/            # Fusion & localization
│   │   ├── fresnel/         # Fresnel zone weighted localization
│   │   ├── ukf/             # Biomechanical Kalman filter
│   │   ├── gdop/            # Geometric dilution of precision
│   │   └── fusion/          # Full localization loop (10 Hz)
│   ├── fleet/                # Node registry, role assignment, TX stagger
│   ├── ble/                  # BLE centroid, rotation heuristics, identity
│   ├── portal/               # Room transition detection, zone occupancy
│   ├── anomaly/              # Pattern learning, anomaly scoring
│   ├── prediction/           # Presence prediction models
│   ├── sleep/                # Sleep quality monitoring
│   ├── flow/                 # Crowd flow accumulation
│   ├── notify/               # Notification renderer (fogleman/gg)
│   ├── mqtt/                 # MQTT client, HA auto-discovery
│   ├── auth/                 # HMAC token derivation, session management
│   ├── oui/                  # OUI lookup table
│   ├── db/                   # SQLite migrations, schema management
│   ├── config/               # Environment variable parsing
│   ├── ota/                  # OTA update manager
│   ├── apdetector/           # AP BSSID auto-detection for passive radar
│   ├── provisioning/         # Node provisioning payload generation
│   ├── replay/               # CSI replay buffer reader/writer
│   ├── volume/               # 3D trigger volumes (spatial automation)
│   ├── webhook/              # Webhook delivery
│   ├── automation/           # Automation trigger evaluation
│   ├── doctor/               # System health diagnostics
│   ├── falldetect/           # Fall detection
│   ├── floorplan/            # Floor plan management
│   ├── guidedtroubleshoot/  # Contextual help system
│   ├── health/               # Health check endpoint
│   ├── loadshed/             # Pipeline overload management
│   ├── ntpserver/            # NTP server for testing
│   ├── recorder/             # CSI recording management
│   ├── rendering/            # Dashboard 3D scene rendering
│   ├── simulator/           # CSI simulator integration
│   ├── startup/              # Startup sequencing
│   ├── tracker/              # Blob tracking
│   └── tracking/             # Multi-blob tracking
├── go.mod                    # Go module definition
├── go.sum                    # Go module checksums
└── test/acceptance/          # In-module acceptance tests
```

**Trigger conditions:**
- Any `.go` file change requires `mothership` binary rebuild
- Dashboard static file changes require rebuild (embedded via `//go:embed`)
- `go.mod`/`go.sum` changes require dependency re-resolution

**Build artifact:** `spaxel` (static Linux binary, Go `net/http` stdlib + embedded dashboard)

---

## 3. CSI Simulator (`cmd/sim/`)

**Build system:** Go 1.25+ (separate module from mothership)

**Paths requiring simulator rebuild:**

```
cmd/sim/
├── main.go                   # Simulator CLI entry point
├── csi.go                   # Synthetic CSI frame generation
├── walker.go                # Random walk simulation
├── ble.go                   # Simulated BLE advertisements
├── websocket.go             # WebSocket client (virtual node)
├── go.mod                    # Go module definition
├── go.sum                    # Go module checksums
└── Makefile                  # Build recipe
```

**Trigger conditions:**
- Any `.go` file change requires `spaxel-sim` binary rebuild
- Used for development/testing, not production deployments

**Build artifact:** `spaxel-sim` (standalone CLI tool)

---

## 4. Dashboard (`dashboard/`)

**Build system:** None (static files, embedded via `//go:embed`)

**Paths requiring mothership rebuild (dashboard updated):**

```
dashboard/
├── index.html               # Main 3D live view
├── live.html                # Live view page
├── simple.html              # Simple mode (card-based UI)
├── ambient.html             # Ambient display mode
├── setup.html               # Setup/calibration interface
├── fleet.html               # Fleet status panel
├── integrations.html        # Integration settings
├── simulator.html           # Simulator UI
├── test-*.html             # Test pages
├── css/                     # Stylesheets
│   └── *.css
├── js/                      # JavaScript modules
│   ├── *.js
│   └── *.ts
├── static/                  # Static assets
│   ├── icons/
│   ├── css/
│   └── js/
├── package.json             # npm dependencies (dev only, for testing)
├── jest.config.js            # Jest test config
├── playwright.config.js      # Playwright E2E config
└── sw.js                    # Service worker
```

**Trigger conditions:**
- Any file change requires mothership rebuild (dashboard is embedded)
- Frontend has no separate build step — static files are served directly
- `package.json` changes do NOT affect production (dev dependencies only)

**Deployment:** Embedded in mothership binary at `mothership/cmd/mothership/dashboard/`

---

## 5. Acceptance Tests (`test/acceptance/`)

**Build system:** Go 1.25+ (separate module)

**Paths requiring test module rebuild:**

```
test/acceptance/
├── *.go                     # Acceptance/integration tests
├── go.mod                    # Go module definition
├── go.sum                    # Go module checksums
└── run_with_diagnostics.sh  # Test runner script
```

**Trigger conditions:**
- Any `.go` file change requires test module rebuild
- Tests run against running mothership container (in-container or via Docker network)

---

## 6. Build Configuration Files

**Paths requiring Docker image rebuild:**

```
/
├── Dockerfile               # Multi-stage build (firmware + Go + distroless runtime)
├── docker-compose.yml        # Deployment manifest
├── VERSION                  # Single source of truth for release version
├── go.work                  # Go workspace stitching (mothership + cmd/sim + test/acceptance)
└── go.work.sum              # Go workspace checksum
```

**Trigger conditions:**
- `Dockerfile` changes require new Docker image build
- `docker-compose.yml` changes require deployment re-application
- `VERSION` changes trigger CI auto-bump and new image tag
- `go.work`/`go.work.sum` changes affect Go module resolution

---

## 7. E2E Test Harness (`tests/e2e/`)

**Shell-based integration tests**

```
tests/e2e/
└── run.sh                   # E2E test harness script
```

**Trigger conditions:**
- Changes to test harness do NOT affect production builds
- Used for validation during CI, not for deployment

---

## 8. Documentation & Notes (No Build Impact)

**Paths that do NOT trigger builds:**

```
docs/
├── plan/
│   └── plan.md              # Implementation plan (this file)
├── notes/                   # Design notes, research
├── research/                # Third-party research
└── tests/                   # Test documentation

notes/                        # Additional operational notes
README.md                     # Project README
PROGRESS.md                   # Implementation progress tracking
*.md                         # All markdown documentation
scripts/                     # Utility scripts (not build-critical)
```

---

## Path Filter Implementation

This catalog feeds directly into CI/CD path filtering. The following patterns should trigger builds:

**Trigger firmware build:**
- `firmware/main/**/*.{c,h}`
- `firmware/CMakeLists.txt`
- `firmware/partitions.csv`
- `firmware/sdkconfig.defaults`

**Trigger mothership build:**
- `mothership/**/*.go`
- `mothership/go.mod`
- `mothership/go.sum`
- `dashboard/**/*` (embedded in mothership)
- `cmd/mothership/**/*`

**Trigger Docker image build:**
- `Dockerfile`
- `docker-compose.yml`
- `VERSION`

**Trigger simulator build:**
- `cmd/sim/**/*.go`
- `cmd/sim/go.mod`
- `cmd/sim/go.sum`

**Do NOT trigger builds (documentation only):**
- `docs/**/*.md`
- `notes/**/*.md`
- `*.md`
- `scripts/**/*`
- `tests/e2e/**/*`

---

## Deployment Strategy

Per ADR-009, automatic convergence is the target state. Deploying mothership version X should converge the fleet onto firmware X. The following sequence ensures consistency:

1. **Code changes** in any path above trigger appropriate build
2. **VERSION** bump (automatic via CI for substantive commits)
3. **Firmware build** produces versioned artifact: `spaxel-firmware-<VERSION>.bin`
4. **Mothership build** produces binary: `spaxel` (with embedded dashboard and firmware seed)
5. **Docker image** published as: `ghcr.io/spaxel/spaxel:<VERSION>`
6. **Deployment** updates mothership, which seeds firmware and triggers OTA convergence

---

## Usage in Path Filters

CI/CD systems can use this catalog to configure path-based triggers:

```yaml
# Example GitHub Actions / Argo Workflows path filter
trigger_firmware:
  paths:
    - 'firmware/main/**/*.{c,h}'
    - 'firmware/CMakeLists.txt'
    - 'firmware/partitions.csv'
    - 'firmware/sdkconfig.defaults'

trigger_mothership:
  paths:
    - 'mothership/**/*.go'
    - 'mothership/go.mod'
    - 'mothership/go.sum'
    - 'dashboard/**/*'
    - 'cmd/mothership/**/*'

trigger_image:
  paths:
    - 'Dockerfile'
    - 'docker-compose.yml'
    - 'VERSION'

trigger_simulator:
  paths:
    - 'cmd/sim/**/*.go'
    - 'cmd/sim/go.mod'
    - 'cmd/sim/go.sum'

documentation_only:
  paths:
    - 'docs/**/*.md'
    - 'notes/**/*.md'
    - '*.md'
    - 'README.md'
    - 'PROGRESS.md'
```

---

**Related ADRs:**
- ADR-001: Decouple ESP32 firmware build from mothership image
- ADR-009: Automatic convergence of mothership and node firmware versions
