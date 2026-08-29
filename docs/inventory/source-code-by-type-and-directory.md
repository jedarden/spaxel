# Spaxel Source Code Inventory

**Generated:** 2026-08-29  
**Total Source Files:** 789 files

## Summary Statistics

| File Type | Count | Primary Location |
|-----------|-------|------------------|
| Go (.go) | 372 | mothership/, cmd/, test/ |
| JavaScript (.js) | 97 | dashboard/ |
| TypeScript (.ts) | 169 | dashboard/, firmware/build/ |
| C source (.c) | 57 | firmware/main/, firmware/managed_components/ |
| C headers (.h) | 37 | firmware/main/, firmware/managed_components/ |
| Python (.py) | 19 | scripts/, dashboard/css/, firmware/managed_components/ |
| HTML (.html) | 9 | dashboard/ |
| CSS (.css) | 29 | dashboard/ |
| Shell scripts (.sh) | 19 | scripts/, firmware/, tests/ |
| YAML (.yml/.yaml) | 19 | Project root, firmware/ |

## Directory Structure Overview

```
/home/coding/spaxel/
├── mothership/          # Go backend service (356 .go files)
│   ├── cmd/            # Main application entrypoints
│   └── internal/      # Internal packages (35+ subsystems)
├── dashboard/          # Web frontend (97 .js, 169 .ts files)
│   ├── js/            # JavaScript modules
│   ├── types/         # TypeScript type definitions
│   ├── css/           # Stylesheets
│   └── tests/         # Frontend tests
├── firmware/          # ESP32-S3 firmware (57 .c, 37 .h files)
│   ├── main/          # Main application code
│   ├── test/          # Firmware tests
│   └── managed_components/  # ESP-IDF components
├── cmd/               # Additional commands (simulator)
├── test/              # Acceptance tests
└── scripts/           # Utility scripts
```

## Detailed Inventory by Type

### 1. Go Source Files (372 files)

#### Main Mothership Application (`mothership/`)

**Command Entry Points** (16 files):
- `cmd/mothership/main.go` - Main application entry
- `cmd/mothership/dashboard_embed.go` - Dashboard embedding
- `cmd/mothership/main_test.go` - Main tests
- `cmd/mothership/mdns_binding.go` - mDNS binding
- `cmd/mothership/migrate.go` - Database migrations
- `cmd/mothership/firmware_test.go` - Firmware tests
- `cmd/mothership/dashboard_static_test.go` - Dashboard static tests
- `cmd/mothership/mdns_binding_test.go` - mDNS binding tests
- `cmd/sim/main.go` - Simulator CLI

**Internal Packages** (356 files across 35+ subsystems):

| Package | File Count | Purpose |
|---------|-------------|---------|
| analytics | 8 | Analytics and metrics |
| api | 38 | REST API handlers |
| auth | 3 | Authentication and session management |
| automation | 3 | Spatial automation system |
| ble | 6 | Bluetooth Low Energy handling |
| briefing | 5 | Morning briefing generation |
| config | 2 | Configuration management |
| dashboard | 3 | Dashboard serving |
| db | 4 | Database operations |
| diagnostics | 2 | System diagnostics |
| events | 5 | Event handling and logging |
| explainability | 3 | Detection explainability |
| falldetect | 2 | Fall detection algorithms |
| fleet | 10 | Node fleet management |
| floorplan | 2 | Floor plan handling |
| fusion | 4 | Sensor fusion engine |
| github | 2 | GitHub API integration |
| guidedtroubleshoot | 3 | Guided troubleshooting system |
| health | 2 | Health monitoring |
| help | 2 | Help system |
| ingestion | 8 | CSI data ingestion |
| learning | 5 | Machine learning models |
| loadshed | 2 | Load shedding under pressure |
| localization | 9 | Position localization |
| mqtt | 2 | MQTT client |
| notifications | 4 | Notification system |
| ntpserver | 2 | NTP server configuration |
| ota | 5 | Over-the-air updates |
| oui | 4 | OUI database lookup |
| prediction | 6 | Presence prediction |
| provisioning | 2 | Node provisioning |
| recorder | 3 | CSI recording |
| replay | 8 | Time-travel replay |
| signal | 13 | Signal processing pipeline |
| simulator | 12 | CSI simulator |
| sleep | 10 | Sleep quality monitoring |
| startup | 2 | Startup sequencing |
| timeline | 2 | Activity timeline |
| tracker | 5 | Blob tracking |
| tracking | 3 | Tracking core |
| volume | 3 | 3D volume detection |
| webhook | 2 | Webhook handling |
| zones | 4 | Zone management |

**Test Files** (40+ files):
- `mothership/test/acceptance/` - Acceptance tests (12 files)
- `mothership/tests/e2e/` - End-to-end tests (3 files)
- Unit tests throughout internal packages

### 2. JavaScript Files (97 files)

#### Dashboard Frontend (`dashboard/js/`)

**Core Application** (86 files):
- `app.js` - Main application
- `auth.js` - Authentication
- `ambient.js`, `ambient_renderer.js` - Ambient mode
- `anomaly.js`, `apdetection.js`, `accuracy.js` - Analytics
- `automation-builder.js`, `automations.js` - Automation
- `ble-panel.js` - BLE device panel
- `briefing.js` - Morning briefing
- `command-palette.js` - Command palette
- `controls.js` - UI controls
- `crowdflow.js` - Crowd flow visualization
- `diurnal-chart.js` - Diurnal baseline chart
- `explainability.js`, `explain.js` - Explainability
- `fleet.js`, `fleet-page.js` - Fleet management
- `floorplan-setup.js` - Floor plan editor
- `guided-help.js` - Guided troubleshooting
- `help.js` - Help system
- `home-cards.js` - Home screen cards
- `integrations.js` - Third-party integrations
- `layers.js` - 3D layer controls
- `linkhealth.js` - Link health display
- `notifications.js` - Notification panel
- `onboard.js` + tests - Onboarding wizard
- `ota.js` - OTA updates
- `panels.js` - UI panels
- `placement.js` - Node placement
- `portal.js` - Portal editor
- `proactive.js` - Proactive features
- `replay.js` - Time-travel replay
- `router.js` - URL routing
- `security-panel.js` - Security mode
- `settings-panel.js` - Settings
- `simulate.js` - Simulator
- `sleep.js` - Sleep analysis
- `state.js` - State management
- `timeline.js` - Activity timeline
- `tooltip.js`, `tooltips.js` - Tooltips
- `troubleshoot.js` - Troubleshooting
- `viz3d.js` - 3D visualization
- `volume-editor.js` - Volume editor
- `websocket.js` - WebSocket client
- `zone-editor.js`, `zone-lookup.js` - Zone management

**Test Files** (25+ files):
- `*.test.js` - Unit tests
- `tests/*.spec.js` - E2E tests

**Static Files** (2 files):
- `static/js/fleet.js`
- `static/js/mobile.js`

### 3. TypeScript Files (169 files)

#### Dashboard (`dashboard/`)

**Type Definitions** (2 files):
- `types/spaxel.d.ts` - Main type definitions
- `types/blob-identity.check.ts` - Blob identity types

**Test Files** (3 files):
- `tests/accessibility/axe-import.spec.ts`
- `tests/accessibility/helper.ts`
- `tests/accessibility/smoke.spec.ts`

**Build System** (164 files):
- `firmware/build/bootloader/*/compiler_depend.ts` (90+ files)
- `firmware/build/*/compiler_depend.ts` (74+ files)

### 4. C Source Files (57 files)

#### ESP32 Firmware (`firmware/`)

**Main Application** (9 files):
- `main/main.c` - Main entry point
- `main/ble.c` - BLE scanning
- `main/csi.c` - CSI capture
- `main/led.c` - LED control
- `main/ntp.c` - NTP time sync
- `main/nvs_migration.c` - NVS schema migration
- `main/provision.c` - Provisioning handler
- `main/transport.c` - Transport layer
- `main/websocket.c` - WebSocket client
- `main/wifi.c` - WiFi management

**Test Files** (8 files):
- `test/test_*.c` - Unit tests

**Build System** (2 files):
- `build/project_elf_src_esp32s3.c`
- `build/bootloader/project_elf_src_esp32s3.c`

**Third-party Components** (38 files):
- `managed_components/espressif__esp_websocket_client/` (7 files)
- `managed_components/espressif__mdns/` (31 files)

### 5. C Header Files (37 files)

#### ESP32 Firmware (`firmware/`)

**Main Application Headers** (9 files):
- `main/spaxel.h` - Main header
- `main/ble.h`
- `main/csi.h`
- `main/led.h`
- `main/ntp.h`
- `main/nvs_migration.h`
- `main/provision.h`
- `main/transport.h`
- `main/websocket.h`
- `main/wifi.h`

**Build System** (2 files):
- `build/spaxel-firmware/main/version.h`
- Various SDK config headers

**Third-party** (21 files):
- ESP WebSocket client headers
- mDNS headers

**Test Headers** (1 file):
- `test/test_runner.h`

### 6. Python Files (19 files)

#### Scripts and Utilities

**Dashboard CSS Generation** (2 files):
- `dashboard/css/_fix_html.py`
- `dashboard/css/_tokenize.py`

**Main Scripts** (3 files):
- `fix_ble_handlers.py`
- `scripts/measure_csi_rate.py`
- `scripts/provision_esp32.py`

**Third-party Test Scripts** (14 files):
- ESP WebSocket client tests
- mDNS tests

### 7. HTML Files (9 files)

#### Dashboard Pages (`dashboard/`)

- `index.html` - Main dashboard
- `live.html` - Live 3D view
- `ambient.html` - Ambient mode
- `fleet.html` - Fleet status
- `setup.html` - Setup/calibration
- `simple.html` - Simple mode
- `simulator.html` - Simulator
- `integrations.html` - Integrations
- `test-transformcontrols.html` - Testing

### 8. CSS Files (29 files)

#### Dashboard Styles (`dashboard/css/`)

**Main Stylesheets** (27 files):
- `layout.css` - Layout
- `tokens.css` - Design tokens
- `ambient.css` - Ambient mode
- `anomaly.css` - Anomaly display
- `briefing.css` - Briefing card
- `explainability.css` - Explainability overlay
- `floorplan.css` - Floor plan
- `home.css` - Home screen
- `notifications.css` - Notifications
- `replay.css` - Time-travel replay
- `sleep.css` - Sleep analysis
- `timeline.css` - Activity timeline
- Plus 15 more feature-specific stylesheets

**Static CSS** (2 files):
- `static/css/fleet-page.css`
- `static/css/mobile.css`

### 9. Shell Scripts (19 files)

#### Project Scripts

**Main Scripts** (8 files):
- `blob_observation.sh`
- `scripts/flash-esp32s3.sh`
- `scripts/run-sim-ble-fixture.sh`
- `scripts/run-sim-ble-match.sh`
- `scripts/run-sim-dashboard-console.sh`
- `scripts/run-sim-identity.sh`
- `scripts/run-sim-local.sh`
- `scripts/test-github-api.sh`

**Firmware Scripts** (3 files):
- `firmware/scripts/generate-signing-key.sh`
- `firmware/scripts/sign-firmware.sh`
- `firmware/scripts/verify-console-config.sh`

**Test Scripts** (3 files):
- `test/acceptance/run_with_diagnostics.sh`
- `tests/e2e/run.sh`
- `window_test.sh`

**Third-party** (5 files):
- ESP WebSocket client examples
- mDNS utilities

### 10. YAML Files (19 files)

#### Configuration

**Project Config** (4 files):
- `docker-compose.yml`
- `.golangci.yml`
- `.needle.yaml`
- `acceptance-test-hang-workflow.yml`

**Firmware Config** (3 files):
- `firmware/main/idf_component.yml`
- `firmware/main/CMakeLists.txt`

**Third-party** (12 files):
- ESP WebSocket client configs
- mDNS configs

## Go Module Structure

The project uses a Go workspace with 3 modules:

1. **Main mothership**: `mothership/go.mod`
2. **Simulator CLI**: `cmd/sim/go.mod`
3. **Acceptance tests**: `test/acceptance/go.mod`

## Architecture Summary

### Three-Tier Architecture

**Backend (Go)**: 372 files
- Mothership service with 35+ internal packages
- Handles CSI ingestion, signal processing, localization
- Manages ESP32 fleet, OTA updates, MQTT integration
- Comprehensive test coverage

**Frontend (JavaScript/TypeScript)**: 266 files
- React-based web dashboard
- 3D visualization with Three.js
- Real-time WebSocket updates
- Multiple UI modes (expert, simple, ambient)

**Firmware (C)**: 94 files
- ESP32-S3 native code
- CSI capture, BLE scanning, WebSocket client
- WiFi management, OTA updates
- Host-based unit tests

### Key Technologies

- **Backend**: Go, SQLite, WebSocket, mDNS, MQTT
- **Frontend**: Vanilla JavaScript, Three.js, React
- **Firmware**: ESP-IDF 5.2, FreeRTOS
- **Infrastructure**: Docker, Shell scripts, CMake, Make

## File Distribution by Purpose

| Purpose | Files | Languages |
|---------|-------|-----------|
| Core application logic | 450+ | Go, C |
| Frontend UI | 266 | JavaScript, TypeScript, HTML, CSS |
| Testing | 80+ | Go, C, JavaScript, Shell |
| Build automation | 150+ | Make, CMake, Shell, Python |
| Configuration | 40+ | YAML, JSON, Shell |
| Documentation | 50+ | Markdown |

## Notes

- No Rust (.rs) files found in the codebase
- TypeScript files are primarily build-generated dependencies
- Third-party components (ESP-IDF managed_components) account for ~100 C/header files
- The project uses extensive testing with multiple test frameworks
- Build system complexity reflects the multi-language architecture (Go + C + JavaScript)
