# Programming Languages Inventory - Spaxel Project

**Generated:** 2026-08-29  
**Purpose:** Comprehensive mapping of all programming languages used in the spaxel codebase with directory locations and supporting evidence.

## Overview

The spaxel project is a multi-language distributed system consisting of:
- **Backend services** (Go)
- **Embedded firmware** (C for ESP32)
- **Web dashboard** (JavaScript/TypeScript/HTML)
- **Build automation** (Shell scripts, CMake, Make)
- **Documentation** (Markdown)

This document provides a complete inventory of all programming languages and their directory mappings.

## Language Summary

| Language | File Count | Primary Purpose | Key Directories |
|----------|------------|----------------|-----------------|
| Go | 372 | Backend services | `mothership/`, `test/acceptance/`, `cmd/sim/` |
| JavaScript/TypeScript | 266 | Web dashboard | `dashboard/js/`, `dashboard/tests/` |
| C | 94 | ESP32 firmware | `firmware/main/`, `firmware/test/` |
| HTML | 205 | Web UI | `dashboard/` |
| Python | 19 | Automation scripts | `scripts/`, `firmware/test/` |
| Shell | 17 | Build automation | `scripts/`, `.marathon/` |
| Markdown | 808 | Documentation | `docs/`, `notes/` |

---

## Go Language (Golang)

**Total Files:** 372  
**Primary Use:** Backend services, API, simulation, and testing

### Directory Mapping

| Directory | File Count | Purpose |
|-----------|------------|---------|
| `mothership/internal/api/` | 48 | REST API handlers |
| `mothership/internal/simulator/` | 20 | Node simulation |
| `mothership/internal/signal/` | 20 | Signal processing |
| `mothership/internal/fleet/` | 15 | Fleet management |
| `mothership/internal/sleep/` | 14 | Sleep analysis |
| `mothership/internal/replay/` | 13 | CSI recording replay |
| `mothership/internal/ingestion/` | 13 | Data ingestion |
| `mothership/test/acceptance/` | 11 | Acceptance tests |
| `mothership/internal/localization/` | 11 | Indoor localization |
| `mothership/internal/analytics/` | 9 | Analytics processing |
| `mothership/cmd/mothership/` | 9 | Main service entry |
| `mothership/internal/prediction/` | 8 | ML prediction |
| `mothership/internal/notifications/` | 8 | Alert notifications |
| `mothership/internal/ble/` | 8 | Bluetooth Low Energy |
| `mothership/internal/ota/` | 7 | Over-the-air updates |
| `test/acceptance/` | 10 | Integration tests |

### Build Evidence

**Module Definition:** `mothership/go.mod`
```go
module github.com/spaxel/mothership

go 1.25.0

require (
    github.com/eclipse/paho.mqtt.golang v1.5.0
    github.com/fogleman/gg v1.3.0
    github.com/go-chi/chi/v5 v5.2.5
    // ... 48 more dependencies
)
```

**Related Modules:**
- `test/acceptance/go.mod` - Acceptance test module
- `cmd/sim/go.mod` - Simulator binary module

### Key Features Implemented

- **REST API:** Chi router with middleware stack
- **WebSocket:** Real-time CSI data streaming
- **MQTT:** Message brokering for device communication
- **Database:** SQLite for persistent storage
- **Signal Processing:** GDOP computation, fingerprinting
- **Localization:** Trilateration, zone detection
- **Simulation:** Fleet-wide device simulation
- **Analytics:** Sleep tracking, pattern detection

---

## JavaScript / TypeScript

**Total Files:** 266  
**Primary Use:** Web dashboard frontend, accessibility testing

### Directory Mapping

| Directory | File Count | Purpose |
|-----------|------------|---------|
| `dashboard/js/` | 85 | Application logic |
| `dashboard/tests/accessibility/` | 5 | Playwright a11y tests |
| `dashboard/tests/` | 3 | Jest unit tests |
| `dashboard/` | 5 | Build config, tooling |
| `dashboard/types/` | 2 | TypeScript definitions |
| `dashboard/static/js/` | 2 | Static assets |

### Build Evidence

**Package Definition:** `dashboard/package.json`
```json
{
  "name": "spaxel-dashboard",
  "private": true,
  "scripts": {
    "test": "jest --verbose",
    "test:a11y": "playwright test",
    "typecheck": "tsc --noEmit -p tsconfig.json"
  },
  "devDependencies": {
    "@axe-core/playwright": "^4.10.1",
    "@playwright/test": "^1.50.0",
    "@types/jest": "^29.5.14"
  }
}
```

### Key Application Modules

**JavaScript Files (`dashboard/js/`):**
- `simulate.js` - CSI simulation interface
- `websocket.js` - Real-time data streaming
- `onboard.js` - Device onboarding flow
- `placement.js` - Floor plan editor
- `ambient.js` - Ambient mode visualization
- `volume-editor.js` - Volume zone management
- `ota.js` - OTA update interface
- `profile-suite.js` - Performance profiling

**TypeScript Definitions (`dashboard/types/`):**
- `spaxel.d.ts` - Core type definitions
- `blob-identity.check.ts` - Type guards

**Testing:**
- `jest.config.js` - Jest unit testing
- `playwright.config.js` - E2E testing
- Accessibility test suites for onboarding, dashboard

---

## C Language (ESP32 Firmware)

**Total Files:** 94  
**Primary Use:** Embedded firmware for ESP32-S3 devices

### Directory Mapping

| Directory | File Count | Purpose |
|-----------|------------|---------|
| `firmware/main/` | 20 | Main firmware logic |
| `firmware/managed_components/espressif__mdns/` | 14 | mDNS service |
| `firmware/managed_components/espressif__mdns/private_include/` | 13 | Private headers |
| `firmware/test/` | 10 | Unit tests |
| `firmware/managed_components/espressif__mdns/tests/host_unit_test/stubs/` | 7 | Test stubs |
| `firmware/managed_components/espressif__mdns/tests/host_unit_test/unity/` | 4 | Unity test framework |
| `firmware/managed_components/espressif__esp_websocket_client/include/` | 2 | WebSocket client |

### Build Evidence

**Build System:** CMake with ESP-IDF
- `firmware/CMakeLists.txt` - Top-level project definition
- `firmware/main/CMakeLists.txt` - Main component
- `firmware/main/CMakeLists.txt` - Bootloader config

**Makefiles:**
- `firmware/test/Makefile` - Test runner
- `mothership/cmd/sim/Makefile` - Go tooling integration

### Key Firmware Components

**Main Application (`firmware/main/`):**
- CSI recording and buffering
- WiFi provisioning and management
- BLE advertising and scanning
- OTA update handling
- NVS (Non-Volatile Storage) for configuration
- Serial console for diagnostics
- Deep sleep handling

**Test Suite (`firmware/test/`):**
- `test_csi_frame.c` - CSI frame validation
- `test_wifi_restart_race.c` - Race condition testing
- `test_ota_during_wifi_reconnect.c` - OTA reliability
- `test_nvs_migration.c` - Config migration
- `test_serial_prov.c` - Provisioning protocol

---

## Python

**Total Files:** 19  
**Primary Use:** Automation scripts, test utilities

### Directory Mapping

| Directory | File Count | Purpose |
|-----------|------------|---------|
| `scripts/` | 2 | Build automation |
| `firmware/managed_components/espressif__mdns/tests/host_test/` | 3 | Integration tests |
| `firmware/managed_components/espressif__esp_websocket_client/tests/autobahn-testsuite/` | 2 | WebSocket protocol tests |
| `firmware/managed_components/espressif__esp_websocket_client/examples/target/` | 2 | Example code |
| `dashboard/css/` | 2 | CSS generation |
| `firmware/managed_components/...` (various) | 8 | Component tests |

### Key Scripts

**Project Scripts:**
- `scripts/measure_csi_rate.py` - CSI data rate measurement
- `scripts/provision_esp32.py` - Device provisioning automation

**Component Tests:**
- Autobahn WebSocket fuzzing tests
- mDNS integration test harnesses

---

## Shell Scripts

**Total Files:** 17  
**Primary Use:** Build automation, deployment, testing

### Directory Mapping

| Directory | File Count | Purpose |
|-----------|------------|---------|
| `scripts/` | 7 | Main automation scripts |
| `firmware/scripts/` | 3 | Firmware tooling |
| `tests/e2e/` | 1 | End-to-end test runner |
| `test/acceptance/` | 1 | Acceptance test runner |
| `.marathon/` | 1 | Marathon test runner |
| `docs/notes/` | 1 | Documentation scripts |

### Key Scripts

**Build & Deployment:**
- `scripts/flash-esp32s3.sh` - Firmware flashing
- `firmware/scripts/sign-firmware.sh` - OTA signing
- `firmware/scripts/generate-signing-key.sh` - Key management
- `firmware/scripts/verify-console-config.sh` - Config validation

**Simulation:**
- `scripts/run-sim-*.sh` - Various simulation modes (identity, local, BLE match/fixture)
- `scripts/run-sim-dashboard-console.sh` - Dashboard integration

**Testing:**
- `test/acceptance/run_with_diagnostics.sh` - Diagnostics collection
- `tests/e2e/run.sh` - E2E test execution

---

## HTML

**Total Files:** 205  
**Primary Use:** Web dashboard user interface

### Directory Mapping

| Directory | File Count | Purpose |
|-----------|------------|---------|
| `dashboard/` | 9 | Main dashboard pages |
| `dashboard/static/` | Majority of files | Static HTML assets |

### Key Pages

- Dashboard main interface
- Device onboarding flow
- Floor plan editor
- OTA update interface
- Ambient mode visualization
- Settings and configuration

---

## Markdown Documentation

**Total Files:** 808  
**Primary Use:** Technical documentation, research notes

### Directory Mapping

| Directory | File Count | Purpose |
|-----------|------------|---------|
| `notes/` | 52 | Project notes |
| `docs/notes/` | 43 | Archived notes |
| `docs/research/` | 18 | Research papers |
| `docs/research/papers/` | 10 | Academic references |
| `docs/` | 16 | Main documentation |
| `firmware/managed_components/...` | 11 | Component docs |
| `docs/deployment/` | 4 | Deployment guides |
| `docs/plan/` | 2 | Project planning |

### Documentation Categories

**Research Papers (`docs/research/`):**
- `01-csi-fundamentals.md` - CSI technology overview
- `02-physics.md` - RF physics for localization
- `03-algorithms.md` - Localization algorithms
- `04-signal-processing.md` - Signal processing techniques
- `05-mesh-topology.md` - Network topology analysis
- `06-accuracy-and-limits.md` - Accuracy limitations
- `07-literature.md` - Academic references

**Technical Notes:**
- `csi-recording-module-structure.md` - CSI data flow
- `github-api-authentication-kaniko-releases.md` - CI/CD integration
- `role-assignment-and-node-registration-flow.md` - Device registration
- `protobuf-survey-free-heap.md` - Memory optimization
- `threejs-import-patterns.md` - Frontend dependencies

---

## Configuration & Build Files

**Total Configuration Files:** 337

### File Types by Distribution

| File Type | Count | Purpose |
|-----------|-------|---------|
| JSON | ~150 | Package configs, build metadata |
| YAML/YML | ~100 | CI/CD, deployment configs |
| TOML | ~5 | Go module configs |
| CMake | ~9 | ESP-IDF build system |
| Makefile | ~8 | Build automation |
| CFG | ~5 | ESP-IDF configuration |

### Key Configuration Files

**Go Modules:**
- `mothership/go.mod` - Main backend dependencies
- `test/acceptance/go.mod` - Test dependencies
- `cmd/sim/go.mod` - Simulator dependencies

**JavaScript:**
- `dashboard/package.json` - Frontend dependencies
- `dashboard/jest.config.js` - Jest test config
- `dashboard/playwright.config.js` - E2E test config

**Firmware:**
- `firmware/CMakeLists.txt` - ESP-IDF project
- `firmware/sdkconfig` - ESP32 configuration

---

## Build Toolchain Summary

### Go (Backend)
- **Build:** `go build ./mothership/cmd/mothership`
- **Test:** `go test ./mothership/...`
- **Dependencies:** 51 packages (via `go.mod`)

### JavaScript/TypeScript (Frontend)
- **Build:** Minimal (no bundler - direct ES modules)
- **Test:** Jest (unit), Playwright (E2E/accessibility)
- **Type Check:** TypeScript compiler (`tsc --noEmit`)

### C (Firmware)
- **Build:** ESP-IDF (CMake-based)
- **Flash:** `esptool.py` via wrapper scripts
- **Test:** Unity framework (host-based unit tests)
- **Target:** ESP32-S3 (dual-core Xtensa LX7)

---

## Language Interdependencies

```
┌─────────────────┐
│   ESP32 C       │
│   Firmware     │◄───────┐
│                 │        │
│  • CSI          │        │
│  • WiFi         │        │
│  • BLE          │        │
│  • OTA          │        │
└─────────────────┘        │
         │                 │
         │ WebSocket       │
         │ MQTT            │
         ▼                 │
┌─────────────────┐        │
│   Go Backend    │        │
│   (Mothership)  │        │
│                 │        │
│  • REST API     │        │
│  • Signal Proc  │        │
│  • Simulation   │        │
│  • Analytics    │        │
└─────────────────┘        │
         │                 │
         │ WebSocket       │
         │ JSON            │
         ▼                 │
┌─────────────────┐        │
│   JavaScript/   │        │
│   TypeScript    │        │
│   Dashboard     │        │
│                 │        │
│  • UI           │        │
│  • Real-time    │        │
│  • Config       │        │
└─────────────────┘        │
                           │
                           │ OTA Update
                           │ Config Push
                           │
                    ┌──────┴──────┐
                    │  Python     │
                    │  Scripts    │
                    │             │
                    │  • Build    │
                    │  • Test     │
                    │  • Deploy   │
                    └─────────────┘
```

---

## Testing by Language

### Go Tests
- **Unit:** `*_test.go` files throughout `mothership/internal/`
- **Acceptance:** `test/acceptance/*.go`
- **Integration:** `mothership/test/acceptance/*.go`

### JavaScript Tests
- **Unit:** Jest tests in `dashboard/tests/`
- **E2E:** Playwright tests in `dashboard/tests/accessibility/`

### C Tests
- **Unit:** Unity tests in `firmware/test/`
- **Integration:** Host tests in `managed_components/`

---

## Development Workflow

### Backend Development (Go)
```bash
cd mothership
go build ./cmd/mothership
go test ./...
```

### Frontend Development (JavaScript/TypeScript)
```bash
cd dashboard
npm test           # Jest unit tests
npm run test:a11y  # Playwright accessibility tests
npm run typecheck  # TypeScript validation
```

### Firmware Development (C)
```bash
cd firmware
idf.py build       # ESP-IDF build
./scripts/flash-esp32s3.sh  # Flash device
```

---

## Language Statistics Summary

- **Total Source Files:** 1,595 (excluding configs/docs)
- **Primary Backend Language:** Go (372 files, 23% of codebase)
- **Primary Firmware Language:** C (94 files, 6% of codebase)
- **Primary Frontend Languages:** JavaScript/TypeScript (266 files, 17% of codebase)
- **Documentation:** Markdown (808 files, 51% of repository)
- **Configuration:** JSON/YAML/TOML/CMake (337 files, 21% of repository)

---

## Conclusion

The spaxel project employs a carefully selected multi-language architecture:

1. **Go** provides the scalable, concurrent backend needed for fleet management
2. **C** delivers the resource-constrained embedded firmware for ESP32 devices
3. **JavaScript/TypeScript** offers an accessible, testable web interface
4. **Python** enables flexible automation and testing
5. **Shell scripts** tie together build and deployment processes

This language distribution reflects the distributed nature of the system: edge devices (C), cloud backend (Go), and user interface (JavaScript/TypeScript).

---

**Document Status:** ✅ Complete  
**Last Updated:** 2026-08-29  
**Maintained By:** Spaxel Development Team