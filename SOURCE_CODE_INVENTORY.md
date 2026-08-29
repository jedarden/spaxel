# Spaxel Source Code Inventory

**Generated:** 2026-08-29  
**Purpose:** Comprehensive inventory of all source code files organized by type and directory

## Overview by File Type

| File Type | Count | Primary Locations |
|-----------|-------|-------------------|
| **Go (.go)** | 352 | `mothership/`, `cmd/sim`, `testdata/` |
| **C (.c)** | 39 | `firmware/main/`, `firmware/test/` |
| **C Headers (.h)** | 34 | `firmware/main/`, `firmware/managed_components/` |
| **JavaScript (.js)** | 98 | `dashboard/js/` |
| **TypeScript (.ts)** | 5 | `dashboard/tests/accessibility/`, `dashboard/types/` |
| **Python (.py)** | 18 | `dashboard/css/`, `scripts/`, `firmware/managed_components/` |
| **Shell (.sh)** | 18 | `scripts/`, `tests/`, `firmware/scripts/` |
| **JSON (.json)** | 200+ | `.beads/`, `dashboard/`, `docs/` |
| **Markdown (.md)** | - | Documentation throughout |

---

## 1. Go Source Files (352 files)

### Main Entry Points
- `mothership/cmd/mothership/main.go`
- `mothership/cmd/mothership/dashboard_embed.go`
- `cmd/sim/main.go`

### Core Mothership Backend (`mothership/cmd/mothership/`)
```
main.go
dashboard_embed.go
dashboard_static_test.go
firmware_test.go
main_test.go
mdns_binding.go
mdns_binding_test.go
migrate.go
splitlink_test.go
```

### Simulator (`mothership/cmd/sim/`)
```
main.go
generator.go
scenario.go
verify.go
walker.go
main_test.go
```

### Internal Packages (`mothership/internal/`)

#### API Handlers (`api/`)
```
alerts.go
analytics.go
analytics_test.go
backup.go
backup_test.go
baseline.go
baseline_test.go
ble_test.go
briefing.go
briefing_test.go
diurnal.go
diurnal_test.go
events.go
events_test.go
feedback.go
framestats.go
guided.go
integrations.go
localization.go
localization_test.go
network_settings.go
network_settings_test.go
notification_settings.go
notification_settings_test.go
notifications.go
notifications_test.go
prediction.go
prediction_test.go
replay.go
replay_test.go
security.go
security_test.go
settings.go
settings_test.go
simulator.go
simulator_test.go
status.go
status_test.go
tracks.go
tracks_identity_test.go
tracks_test.go
triggers.go
triggers_test.go
utils.go
volume_triggers.go
volume_triggers_test.go
zones.go
zones_test.go
```

#### Signal Processing (`signal/`)
```
ambient.go
ambient_test.go
baseline.go
baseline_test.go
breathing.go
breathing_noise_test.go
breathing_test.go
diurnal.go
diurnal_test.go
features.go
features_test.go
healthpersist.go
healthpersist_test.go
persist.go
persist_test.go
phase.go
phase_property_test.go
phase_test.go
processor.go
identity_fields_test.go
```

#### Fusion & Localization (`fusion/`, `localization/`, `tracker/`)
```
fusion/
├── explain.go
├── fusion.go
├── fusion_test.go
└── grid3d.go

localization/
├── fusion.go
├── gdop_example.go
├── grid.go
├── groundtruth.go
├── groundtruth_store.go
├── groundtruth_test.go
├── self_improving.go
├── spatial_weights.go
├── spatial_weights_test.go
├── weightlearner.go
└── weightstore.go

tracker/
├── ble_provider.go
├── identity.go
├── tracker.go
├── tracker_test.go
└── ukf.go
```

#### Ingestion (`ingestion/`)
```
frame.go
frame_test.go
frametracker.go
frame_fuzz_test.go
json_fuzz_test.go
message.go
message_test.go
ratecontrol.go
ratecontrol_test.go
ring.go
ring_test.go
server.go
server_test.go
```

#### Fleet Management (`fleet/`)
```
collision.go
collision_debug_test.go
collision_test.go
fleethandler.go
fleet_test.go
handler.go
handler_test.go
healer.go
healer_test.go
manager.go
optimiser.go
registry.go
selfheal.go
selfheal_test.go
weather.go
```

#### Other Key Packages
- `automation/` - Trigger engine
- `analytics/` - Anomaly detection, flow tracking
- `auth/` - Token validation, session management
- `ble/` - Device registry, identity matching, rotation handling
- `briefing/` - Morning briefing generation
- `db/` - SQLite database, migrations
- `events/` - Event storage and timeline
- `ota/` - Firmware update system
- `prediction/` - Presence prediction models
- `replay/` - CSI replay engine
- `recording/` - CSI recording buffer
- `render/` - Floor plan rendering
- `shutdown/` - Graceful shutdown
- `simulator/` - CSI simulation engine
- `sleep/` - Sleep quality monitoring
- `zones/` - Zone management

### Tests (`mothership/test/`, `test/acceptance/`)
- Integration tests in `test/acceptance/`
- E2E tests in `mothership/tests/e2e/`
- Acceptance scenarios (AS1-AS7)

---

## 2. C Source Files (39 files)

### Core Firmware (`firmware/main/`)
```
ble.c           - BLE scanning and advertisement parsing
csi.c           - CSI capture and processing
led.c           - LED control
main.c          - Main application entry point
ntp.c           - NTP time synchronization
nvs_migration.c - NVS schema migration
provision.c     - Serial provisioning handler
transport.c     - Transport layer (WebSocket)
websocket.c     - WebSocket client
wifi.c          - WiFi connection management
```

### Firmware Tests (`firmware/test/`)
```
test_all_restart_trigger_points.c
test_console_config.c
test_csi_frame.c
test_nvs_migration.c
test_ota_during_wifi_reconnect.c
test_runner.c
test_sanity.c
test_serial_prov.c
test_wifi_restart_race.c
```

### Managed Components (`firmware/managed_components/`)
- ESP WebSocket Client implementation
- mDNS implementation

---

## 3. C Header Files (34 files)

### Core Firmware Headers (`firmware/main/`)
```
ble.h
csi.h
led.h
ntp.h
nvs_migration.h
provision.h
spaxel.h
transport.h
websocket.h
wifi.h
```

### Managed Components Headers
- ESP WebSocket Client headers
- mDNS headers (public and private)

---

## 4. JavaScript/TypeScript Files (103 files)

### Core Dashboard JavaScript (`dashboard/js/`)
```
app.js                    - Main application entry point
websocket.js             - WebSocket client
state.js                  - Application state management
router.js                 - Client-side routing
auth.js                   - Authentication handling
```

### 3D Visualization
```
viz3d.js                  - Three.js 3D scene rendering
fresnel.js                - Fresnel zone visualization
fxaa.js                   - Anti-aliasing
ambient_renderer.js      - Ambient mode rendering
controls.js               - View controls
layers.js                - Layer management
placement.js             - Node placement UI
```

### Feature Modules
```
accuracy.js               - Accuracy tracking
anomaly.js                - Anomaly detection
apdetection.js            - AP auto-detection
automation-builder.js     - Trigger volume editor
automations.js           - Automation management
blob-identity.js          - Blob-BLE identity mapping
ble-panel.js             - BLE device panel
briefing.js              - Morning briefing
command-palette.js       - Command palette (Ctrl+K)
crowdflow.js             - Crowd flow visualization
diurnal-chart.js         - Diurnal baseline chart
explainability.js        - Detection explanation
explain.js               - Explainability UI
feedback.js              - User feedback collection
fleet.js                 - Fleet status
fleet-page.js             - Fleet management page
floorplan-setup.js       - Floor plan editor
guided-help.js           - Guided troubleshooting
help.js                  - Help system
home-cards.js            - Home mode cards
integrations.js           - Third-party integrations
layer.js                 - Layer controls
linkhealth.js            - Link health visualization
notifications.js         - Notification handling
onboard.js               - Onboarding wizard
ota.js                   - OTA management
panels.js                - UI panels
portal.js                - Portal management
proactive.js             - Proactive help
quick-actions.js         - Context menu actions
replay.js                - Time-travel replay
security-panel.js        - Security mode settings
settings-panel.js       - Settings management
sidebar-timeline.js      - Activity timeline
simple-mode.js           - Simple mode UI
sleep.js                 - Sleep monitoring
simulate.js              - Simulator integration
timeline.js              - Timeline view
tooltip.js               - Tooltip system
tooltips.js              - Tooltip definitions
troubleshoot.js          - Troubleshooting wizard
volume-editor.js         - Trigger volume editor
zone-editor.js           - Zone management
zone-lookup.js           - Zone search
```

### TypeScript Files
```
dashboard/tests/accessibility/
├── axe-import.spec.ts
├── helper.ts
├── smoke.spec.ts

dashboard/types/
├── blob-identity.check.ts
└── spaxel.d.ts
```

### Build & Test Config
```
generate-icons.js          - Icon generation
jest.config.js            - Jest test configuration
playwright.config.js      - Playwright E2E configuration
sw.js                     - Service worker
esptool-bundle.js        - Esptool bundle for Web Serial
```

---

## 5. Python Scripts (18 files)

### Dashboard CSS Tools
```
dashboard/css/_fix_html.py
dashboard/css/_tokenize.py
```

### Scripts
```
scripts/measure_csi_rate.py
scripts/provision_esp32.py
fix_ble_handlers.py
```

### Firmware Dependencies
```
firmware/managed_components/espressif__esp_websocket_client/examples/target/
├── pytest_websocket.py
└── websocket_server.py

firmware/managed_components/espressif__esp_websocket_client/tests/
├── autobahn-testsuite/
│   ├── conftest.py
│   ├── pytest_autobahn.py
│   └── scripts/generate_summary.py
└── unit/pytest_websocket.py
```

---

## 6. Shell Scripts (18 files)

### ESP32 Flashing & Setup
```
scripts/flash-esp32s3.sh
firmware/scripts/generate-signing-key.sh
firmware/scripts/sign-firmware.sh
firmware/scripts/verify-console-config.sh
```

### Simulator Fixtures
```
scripts/run-sim-ble-fixture.sh
scripts/run-sim-ble-match.sh
scripts/run-sim-dashboard-console.sh
scripts/run-sim-identity.sh
scripts/run-sim-local.sh
```

### Testing & Diagnostics
```
test/acceptance/run_with_diagnostics.sh
tests/e2e/run.sh
blob_observation.sh
window_test.sh
scripts/test-github-api.sh
```

### Managed Components
```
firmware/managed_components/espressif__esp_websocket_client/examples/linux/start_server.sh
firmware/managed_components/espressif__esp_websocket_client/examples/target/generate_certs.sh
firmware/managed_components/espressif__esp_websocket_client/tests/autobahn-testsuite/run_tests.sh
```

---

## 7. JSON Configuration Files (200+)

### Build & Package Configuration
```
dashboard/package.json
dashboard/package-lock.json
dashboard/tsconfig.json
dashboard/manifest.json
```

### Checkpoint & State
```
.beads/checkpoint/current.json
.beads/checkpoint/previous.json
.beads/config.json
```

### Test Results & Diagnostics
```
dashboard/leak-detection-report.json
dashboard/leak-isolation-results.json
dashboard/leak-test-full-lifecycle.json
dashboard/test-profiling-results.json
dashboard/test-results/.last-run.json
```

### Managed Components Checksums
```
firmware/managed_components/espressif__esp_websocket_client/CHECKSUMS.json
firmware/managed_components/espressif__mdns/CHECKSUMS.json
```

### Runtime State
```
~/.needle/state/heartbeats/*.json
~/.needle/state/workers.json
```

---

## 8. Key Directories Summary

```
spaxel/
├── mothership/              # Go backend (primary application)
│   ├── cmd/mothership/     # Main entry point
│   ├── cmd/sim/            # CSI simulator
│   ├── internal/           # All internal packages
│   │   ├── api/             # REST API handlers
│   │   ├── signal/          # Signal processing
│   │   ├── fusion/          # Localization engine
│   │   ├── ingestion/       # WebSocket ingestion
│   │   ├── fleet/          # Node management
│   │   └── [30+ packages]
│   └── test/acceptance/     # Integration tests
├── firmware/                # ESP32-S3 firmware (C)
│   ├── main/                # Core firmware
│   ├── test/                # Firmware tests
│   └── managed_components/ # ESP-IDF components
├── dashboard/               # Web UI (JavaScript)
│   ├── js/                  # All frontend code
│   ├── tests/               # E2E and accessibility tests
│   └── static/              # Static assets
├── scripts/                 # Utility scripts
├── test/acceptance/         # Cross-cutting acceptance tests
├── docs/                    # Documentation
└── testdata/               # Test data generation
```

---

## File Type Distribution by Module

| Module | Go | C | Headers | JS/TS | Python | Shell | Total |
|--------|-----|---|---------|-------|--------|-------|-------|
| Mothership | 320+ | - | - | - | - | - | ~320 |
| Firmware | - | 20 | 10 | - | 8 | 3 | ~41 |
| Dashboard | - | - | - | 100+ | 2 | - | ~102 |
| Scripts | 2 | - | - | - | 3 | 15 | ~20 |
| Tests | 30 | 19 | - | - | 5 | 2 | ~56 |
| **Total** | **352** | **39** | **34** | **103** | **18** | **18** | **~564** |

---

## Notes

1. **Excluded paths**: `.git/`, `node_modules/`, `target/`, `build/` directories excluded
2. **Managed components**: ESP-IDF managed components counted but not fully inventoried
3. **Test files**: Test files (`_test.go`) included in respective module counts
4. **JSON files**: Only representative JSON files shown; full list includes 200+ bead trace files
5. **Documentation**: Markdown files not fully enumerated; exists throughout `docs/`

---

**This inventory is auto-generated and should be regenerated when major structural changes occur.**
