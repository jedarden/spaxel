# Spaxel Source Code Inventory

**Generated:** 2026-08-29; reconciled 2026-09-04 (spaxel-7737eea8 — this is the surviving
copy; the duplicate root `SOURCE_CODE_INVENTORY.md` was deleted and its per-file purpose
annotations folded in)  
**Total Source Files:** 789 files (counts below are point-in-time; re-derive with
`git ls-files` before relying on them)

## Summary Statistics

| File Type | Count | Primary Location |
|-----------|-------|------------------|
| Go (.go) | 378 | mothership/ |
| JavaScript (.js) | 97 | dashboard/ |
| TypeScript (.ts) | 169 | dashboard/, firmware/build/ |
| C source (.c) | 57 | firmware/main/, firmware/managed_components/ |
| C headers (.h) | 37 | firmware/main/, firmware/managed_components/ |
| Python (.py) | 4 project (+ vendored) | scripts/, dashboard/css/ |
| HTML (.html) | 9 | dashboard/ |
| CSS (.css) | 29 | dashboard/ |
| Shell scripts (.sh) | 11 project (+ vendored) | scripts/, firmware/scripts/ |
| YAML (.yml/.yaml) | 19 | Project root, firmware/ |

## Directory Structure Overview

```
/home/coding/spaxel/
├── mothership/          # Go backend service (the only Go module)
│   ├── cmd/            # mothership/ (binary) + sim/ (spaxel-sim CLI)
│   ├── internal/      # Internal packages (56 packages)
│   ├── test/acceptance/  # Acceptance scenarios + IO install/upgrade tests
│   └── tests/e2e/     # End-to-end Go tests
├── dashboard/          # Web frontend (97 .js, 169 .ts files)
│   ├── js/            # JavaScript modules (+ co-located jest tests)
│   ├── types/         # TypeScript type definitions
│   ├── css/           # Stylesheets
│   └── tests/         # Playwright accessibility tests
├── firmware/          # ESP32-S3 firmware (12 .c in main/, 9 host-test .c)
│   ├── main/          # Main application code
│   ├── test/          # Firmware host tests (gcc harness)
│   └── managed_components/  # ESP-IDF components
└── scripts/           # Utility scripts
```

## Detailed Inventory by Type

### 1. Go Source Files (378 files)

#### Main Mothership Application (`mothership/`)

**Command Entry Points** (`mothership/cmd/`):
- `mothership/main.go` - Main application entry (startup phases, subsystem wiring)
- `mothership/dashboard_embed.go` - Dashboard embedding (go:embed, `-tags=embed`)
- `mothership/mdns_binding.go` - mDNS advertisement
- `mothership/migrate.go` - Database migration registration
- `sim/main.go` (+ `generator.go`, `walker.go`, `scenario.go`, `verify.go`) - Simulator CLI

**Internal Packages** (`mothership/internal/`, 56 packages, 332 files):

| Domain | Packages |
|---------|-------------|
| Ingestion & signal | `ingestion`, `signal` (flat: phase/features/breathing/baseline/diurnal), `recorder`, `recording`, `replay`, `simulator` |
| Localization & tracking | `fusion`, `localizer` (+`fusion/`), `localization`, `tracker`, `tracking`, `ble` |
| Fleet & node lifecycle | `fleet`, `provisioning`, `ota`, `autoupdate`, `apdetector` |
| Spaces, events & automation | `zones`, `floorplan`, `volume`, `automation`, `eventbus`, `events`, `timeline` |
| Inference & monitoring | `analytics`, `prediction`, `learning`, `sleep`, `falldetect`, `health` |
| API & persistence | `api`, `auth`, `config`, `db`, `dashboard` |
| Notifications & integrations | `briefing`, `notify`, `notifications`, `render`, `webhook`, `mqtt`, `github`, `help`, `guidedtroubleshoot` |
| Platform & operations | `startup`, `shutdown`, `doctor`, `diagnostics`, `explainability`, `loadshed`, `diskspace`, `logging`, `types`, `beads`, `ntpserver`, `oui` |

Per-file detail per package: `docs/research/go-backend-code-directories.md`.

**Test Files**:
- `mothership/test/acceptance/` - Acceptance tests (11 files, AS-1…AS-7 + IO install/upgrade)
- `mothership/tests/e2e/` - End-to-end tests (4 files, incl. IO-6 gate)
- Unit tests throughout internal packages (142 `*_test.go`)

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

**Main Application** (12 files):
- `main/main.c` - Main entry point, startup sequencing
- `main/wifi.c` - WiFi station, mDNS, captive portal
- `main/csi.c` - CSI capture (promiscuous mode, callback, frame serialization)
- `main/websocket.c` - WebSocket client (binary CSI up, JSON config down)
- `main/transport.c` - Transport abstraction (UART0 + USB-Serial/JTAG)
- `main/ble.c` - BLE passive scanning, advertisement parsing
- `main/led.c` - LED control (identify, OTA progress)
- `main/ntp.c` - NTP time sync for TX stagger slots
- `main/nvs_migration.c` - NVS read/write + schema migration
- `main/provision.c` - Serial provisioning handler
- `main/safe_mode.c` - Safe-mode entry/recovery
- `main/watchdog.c` - Task watchdog (esp_task_wdt)

**Test Files** (9 files):
- `test/test_*.c` - Host-based gcc unit tests (`test_runner.c` harness)

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

### 6. Python Files (4 project files + vendored)

#### Scripts and Utilities

**Dashboard CSS Generation** (2 files):
- `dashboard/css/_fix_html.py`
- `dashboard/css/_tokenize.py`

**Main Scripts** (2 files):
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

### 9. Shell Scripts (11 project files + vendored)

#### Project Scripts

**Simulator & Device Scripts** (`scripts/`, 8 files):
- `scripts/flash-esp32s3.sh`
- `scripts/run-sim-ble-fixture.sh`
- `scripts/run-sim-ble-match.sh`
- `scripts/run-sim-dashboard-console.sh`
- `scripts/run-sim-identity.sh`
- `scripts/run-sim-local.sh`
- `scripts/test-github-api.sh`
- `scripts/walkthrough_monitor.sh`

**Firmware Scripts** (3 files):
- `firmware/scripts/generate-signing-key.sh`
- `firmware/scripts/sign-firmware.sh`
- `firmware/scripts/verify-console-config.sh`

**Third-party** (vendored, under `firmware/managed_components/` and `.beads/`):
- ESP WebSocket client examples and mDNS utilities

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

The project uses a Go workspace with **one module**:

1. **Main mothership**: `mothership/go.mod` — the simulator CLI
   (`mothership/cmd/sim/`) and both test trees (`mothership/test/acceptance/`,
   `mothership/tests/e2e/`) live inside it

## Architecture Summary

### Three-Tier Architecture

**Backend (Go)**: 378 files
- Mothership service with 56 internal packages
- Handles CSI ingestion, signal processing, localization
- Manages ESP32 fleet, OTA updates, MQTT integration
- Comprehensive test coverage

**Frontend (JavaScript/TypeScript)**: 266 files
- Vanilla-JS web dashboard (no framework, no build step)
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
