# Spaxel Source Code Inventory

**Generated:** 2026-08-29
**Repository:** /home/coding/spaxel

## Summary

| Language | Files | Primary Location |
|----------|-------|------------------|
| Go (.go) | 372 | mothership/ |
| C (.c) | 61 | firmware/ |
| C Headers (.h) | 37 | firmware/ |
| JavaScript (.js) | 97 | dashboard/ |
| Python (.py) | 19 | dashboard/, docs/ |
| Shell (.sh) | 19 | Various |

**Total Source Files:** 603 files

---

## 1. Go Source Files (372 files)

### Mothership Application Core
- **mothership/cmd/mothership/** (9 files)
  - Main application entry point and startup

### Internal Packages by Domain

#### API & HTTP Layer (48 files)
- **mothership/internal/api/** - REST API endpoints, WebSocket handlers, HTTP middleware

#### Fleet Management (15 files)
- **mothership/internal/fleet/** - Node registry, role assignment, self-healing

#### Signal Processing Pipeline (20 files)
- **mothership/internal/signal/** - CSI processing, phase sanitization, feature extraction

#### Data Ingestion (13 files)
- **mothership/internal/ingestion/** - WebSocket server, frame parsing, node connections

#### Localization & Fusion (11 files)
- **mothership/internal/localization/** - Fresnel zones, UKF tracking, spatial fusion
- **mothership/internal/fusion/** - Multi-link fusion engine (4 files)
- **mothership/internal/localizer/fusion/** (1 file)

#### Tracking & Analysis
- **mothership/internal/tracker/** (6 files) - Blob tracking
- **mothership/internal/analytics/** (9 files) - Crowd flow, coverage analysis
- **mothership/internal/prediction/** (8 files) - Presence prediction models
- **mothership/internal/ble/** (8 files) - BLE device matching
- **mothership/internal/sleep/** (14 files) - Sleep quality monitoring

#### Automation & Notifications
- **mothership/internal/automation/** (3 files) - Trigger system
- **mothership/internal/notifications/** (8 files) - Alert delivery
- **mothership/internal/events/** (7 files) - Event logging
- **mothership/internal/webhook/** (2 files) - Webhook delivery

#### System Integration
- **mothership/internal/mqtt/** (3 files) - MQTT integration
- **mothership/internal/ota/** (7 files) - OTA update management
- **mothership/internal/auth/** (3 files) - Authentication & sessions
- **mothership/internal/provisioning/** (2 files) - Node provisioning
- **mothership/internal/apdetector/** (1 file) - AP detection for passive radar
- **mothership/internal/autoupdate/** (1 file) - Automatic updates

#### Data Management
- **mothership/internal/db/** (4 files) - SQLite database, migrations
- **mothership/internal/replay/** (13 files) - CSI replay system
- **mothership/internal/recorder/** (4 files) - CSI recording
- **mothership/internal/recording/** (4 files) - Recording management

#### Configuration & Diagnostics
- **mothership/internal/config/** (2 files) - Configuration handling
- **mothership/internal/diagnostics/** (3 files) - System diagnostics
- **mothership/internal/health/** (2 files) - Health checks
- **mothership/internal/doctor/** (2 files) - System doctor
- **mothership/internal/diskspace/** (2 files) - Disk space monitoring
- **mothership/internal/loadshed/** (2 files) - Load shedding
- **mothership/internal/ntpserver/** (2 files) - NTP server config
- **mothership/internal/oui/** (5 files) - OUI lookup table

#### User Interface Components
- **mothership/internal/dashboard/** (3 files) - Dashboard serving
- **mothership/internal/explainability/** (3 files) - Detection explanation
- **mothership/internal/help/** (4 files) - Help system
- **mothership/internal/guidedtroubleshoot/** (4 files) - Guided troubleshooting
- **mothership/internal/timeline/** (2 files) - Timeline events
- **mothership/internal/briefing/** (5 files) - Morning briefings
- **mothership/internal/notify/** (5 files) - Notification rendering
- **mothership/internal/render/** (2 files) - Image rendering
- **mothership/internal/shutdown/** (3 files) - Graceful shutdown

#### Simulation & Testing
- **mothership/internal/simulator/** (20 files) - CSI simulator for testing

#### Other Components
- **mothership/internal/zones/** (4 files) - Zone management
- **mothership/internal/volume/** (3 files) - 3D volumes
- **mothership/internal/floorplan/** (2 files) - Floor plan handling
- **mothership/internal/falldetect/** (2 files) - Fall detection
- **mothership/internal/eventbus/** (2 files) - Event bus
- **mothership/internal/github/** (2 files) - GitHub API
- **mothership/internal/startup/** (2 files) - Startup sequencing
- **mothership/internal/learning/** (5 files) - Learning system

### Test Files

#### Mothership Tests
- **mothership/test/** (9 files)
- **mothership/test/acceptance/** (11 files)
- **mothership/tests/e2e/** (4 files)

#### Cross-Repository Tests
- **test/acceptance/** (10 files)

### Simulator CLI
- **cmd/sim/** (6 files) - Standalone CSI simulator
- **cmd/sim/main.go** - Main entry point

### Documentation Tools
- **docs/** (2 files) - Documentation utilities
- **docs/examples/** (1 file) - Example code

---

## 2. C Source Files (61 files)

### ESP32 Firmware (firmware/main/)

The bulk of C code is in the firmware directory for the ESP32-S3:

**Main Firmware Components:**
- firmware/main/*.c - Core firmware modules

**Key C Modules:**
- CSI capture and processing
- WebSocket communication
- WiFi management
- BLE scanning
- OTA updates
- Provisioning (serial and captive portal)
- LED control
- mDNS discovery
- NTP synchronization
- NVS (Non-Volatile Storage) management

**Test Files:**
- firmware/test/test_*.c - Host-based unit tests with gcc harness

---

## 3. C Header Files (37 files)

### ESP32 Firmware Headers (firmware/main/)

**Main Headers:**
- firmware/main/*.h - Header files for firmware modules

**Key Headers:**
- CSI function declarations
- WebSocket protocol definitions
- WiFi and BLE headers
- OTA update interfaces
- Provisioning types
- Hardware abstractions

---

## 4. JavaScript Files (97 files)

### Dashboard Frontend (dashboard/js/)

**Core Application:**
- app.js - Main application entry
- ambient.js - Ambient mode
- controls.js - UI controls
- auth.js - Authentication

**3D Visualization:**
- fresnel.js - Fresnel zone rendering
- fxaa.js - Anti-aliasing
- explainability.js - Detection explanation UI
- explain.js - Explanation components

**Fleet Management:**
- fleet.js - Fleet overview
- fleet-page.js - Fleet status page
- ble-panel.js - BLE device panel

**Automation:**
- automations.js - Automation system
- automation-builder.js - Visual automation builder

**Analysis Features:**
- anomaly.js - Anomaly detection UI
- apdetection.js - AP detection
- crowdflow.js - Crowd flow visualization
- accuracy.js - Accuracy tracking
- prediction.js - Prediction display
- diurnal-chart.js - Diurnal baseline charts
- sleep.js - Sleep monitoring
- briefing.js - Morning briefings
- blob-identity.js - Person identification

**Interactive Features:**
- command-palette.js - Command palette (Ctrl+K)
- feedback.js - Feedback collection
- explainability.js - Detection explanation
- guided-help.js - Guided troubleshooting
- floorplan-setup.js - Floor plan configuration

**Testing Files:**
- Many .test.js files for unit tests
- Test setup files

### Build Tools (dashboard/)
- jest.config.js - Jest test configuration
- generate-icons.js - Icon generation
- esptool-bundle.js - ESP tool bundling

---

## 5. Python Files (19 files)

### Dashboard CSS Tools (dashboard/css/)
- _fix_html.py - HTML fixing utilities
- _tokenize.py - Tokenization utilities

### Documentation (docs/)
- Python scripts for documentation generation or analysis

### Build/Development
- Build utilities and development tools

---

## 6. Shell Scripts (19 files)

### Build & Deployment
- Docker build scripts
- Deployment automation
- CI/CD integration

### Development Tools
- Development environment setup
- Testing utilities
- Code generation helpers

### Root Directory
- blob_observation.sh - Blob observation utility

---

## Directory Structure Overview

```
spaxel/
├── mothership/              # Go backend (main application)
│   ├── cmd/                 # Entry points
│   ├── internal/            # Internal packages (50+ domains)
│   └── test/                # Tests
├── firmware/                # ESP32-S3 firmware (C)
│   └── main/                # Main firmware code (61 .c, 37 .h)
├── dashboard/               # Web UI (JavaScript)
│   ├── js/                  # JavaScript modules (97 .js)
│   └── css/                 # CSS + Python tools (19 .py)
├── cmd/sim/                 # Simulator CLI (6 .go)
├── test/acceptance/         # Cross-cutting tests (10 .go)
└── [config files]
```

---

## Key Architectural Patterns

### Mothership (Go)
- **Package organization:** Clear separation by domain (api, signal, fleet, etc.)
- **Internal packages:** Most logic in internal/ with public interfaces at higher levels
- **Test structure:** Dedicated test/ directories with acceptance tests

### Firmware (C)
- **ESP-IDF based:** All firmware code follows ESP-IDF project structure
- **Component-based:** Modular C files under firmware/main/
- **Host testing:** GCC harness in firmware/test/ for hardware-free testing

### Dashboard (JavaScript)
- **Vanilla JS:** No build framework, direct browser execution
- **Feature modules:** Each major feature in its own .js file
- **Tested:** Jest configuration with .test.js files alongside implementation

### Cross-Cutting Concerns
- **WebSocket:** Primary communication protocol
- **SQLite:** Single database for all persistence
- **Testing:** Comprehensive coverage at all layers
- **Documentation:** Embedded in repo
