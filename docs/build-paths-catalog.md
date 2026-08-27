# Spaxel Build Paths Catalog

**Purpose:** This document identifies all file paths in the spaxel repository that contain substantive code requiring rebuilds and deployments. Changes to these paths should trigger the appropriate build pipeline and deployment processes.

**Last updated:** 2026-08-27

---

## Overview

Spaxel is a WiFi CSI-based indoor positioning system with three main components:
1. **Firmware** - ESP32-S3 C code (ESP-IDF framework)
2. **Mothership** - Go backend server
3. **Dashboard** - Web UI (embedded into Go binary)

This catalog categorizes paths by component and deployment impact.

---

## 1. Firmware Code (ESP32-S3)

### Core Application Code
**Path:** `firmware/main/`
**Impact:** Changes require ESP32-S3 firmware rebuild and OTA deployment to all nodes

**Critical files:**
- `firmware/main/main.c` - Application entry point, startup sequencing
- `firmware/main/csi.c` / `csi.h` - CSI capture and processing
- `firmware/main/wifi.c` / `wifi.h` - WiFi connection, mDNS, captive portal
- `firmware/main/websocket.c` / `websocket.h` - WebSocket client, JSON/binary framing
- `firmware/main/ble.c` / `ble.h` - BLE scanning and advertisement parsing
- `firmware/main/provision.c` / `provision.h` - Serial provisioning protocol
- `firmware/main/nvs_migration.c` / `nvs_migration.h` - NVS schema migration
- `firmware/main/transport.c` / `transport.h` - Binary CSI frame serialization
- `firmware/main/ntp.c` / `ntp.h` - NTP synchronization
- `firmware/main/led.c` / `led.h` - LED control (identify, OTA progress)
- `firmware/main/spaxel.h` - Shared firmware headers

**Build trigger:** Any change to these files requires:
1. ESP-IDF rebuild (`idf.py build`)
2. Firmware binary upload to GitHub Releases
3. Mothership Docker image rebuild (to embed new firmware)
4. OTA rollout to fleet

### Firmware Build Configuration
**Path:** `firmware/CMakeLists.txt`, `firmware/sdkconfig.defaults`
**Impact:** Changes affect ESP32-S3 firmware build configuration

**Critical settings:**
- `CONFIG_ESP32S3_SPIRAM_SUPPORT=y`
- `CONFIG_ESP_WIFI_PROMISCUOUS_FILTER=y`
- `CONFIG_ESP_WIFI_CSI_ENABLED=y`
- `CONFIG_BT_ENABLED=y`
- `CONFIG_BT_BLE_ENABLED=y`
- `CONFIG_BOOTLOADER_APP_ROLLBACK_ENABLE=y`

**Build trigger:** Changes require firmware rebuild and redeployment

### Firmware Tests
**Path:** `firmware/test/`
**Impact:** Changes to test code do NOT trigger production builds

**Files:** (host-based gcc harness tests)
- `firmware/test/test_*.c` - Unit tests (provisioning, CSI frame format, NVS migration)
- `firmware/test/test_runner.h` - Test runner

**Note:** These tests run during CI image build but do not affect runtime firmware

---

## 2. Mothership (Go Backend)

### Core Application Code
**Path:** `mothership/`
**Impact:** Changes require Go binary rebuild and Docker image deployment

**Entry point:**
- `mothership/cmd/mothership/main.go` - Application startup, subsystem wiring

**Internal packages (all paths under `mothership/internal/`):**

**Signal Processing & Localization:**
- `mothership/internal/pipeline/` - CSI signal processing pipeline
  - `phase/` - Phase sanitization (unwrap, OLS, STO/CFO removal)
  - `nbvi/` - NBVI subcarrier selection
  - `feature/` - deltaRMS, phase variance, breathing band
  - `baseline/` - EMA baseline, diurnal slots, snapshots
- `mothership/internal/localizer/` - Spatial localization
  - `fresnel/` - Fresnel zone weighted accumulation
  - `ukf/` - Biomechanical Unscented Kalman Filter
  - `gdop/` - Geometric Dilution of Precision
  - `fusion/` - Full 10Hz localization loop

**Fleet Management:**
- `mothership/internal/fleet/` - Node registry, role assignment, stagger scheduling
- `mothership/internal/ota/` - OTA updates, rolling deployment, rollback detection

**Ingestion & Transport:**
- `mothership/internal/ingestion/` - WebSocket server, binary frame parsing
- `mothership/internal/recording/` - CSI replay buffer (append-only file)

**Data Persistence:**
- `mothership/internal/db/` - SQLite schema, migrations, queries

**API & Dashboard:**
- `mothership/internal/api/` - REST API handlers
- `mothership/internal/dashboard/` - WebSocket dashboard feed, snapshot/incremental updates

**Automation & Triggers:**
- `mothership/internal/automation/` - Spatial automation builder
- `mothership/internal/volume/` - Trigger volume point-in-tests
- `mothership/internal/webhook/` - Webhook execution

**Person Detection & Identity:**
- `mothership/internal/ble/` - BLE scanning, centroid, rotation heuristics, identity matching
- `mothership/internal/tracker/` - Blob tracking, ID assignment

**Advanced Features:**
- `mothership/internal/anomaly/` - Anomaly detection pattern learning
- `mothership/internal/prediction/` - Presence prediction models
- `mothership/internal/sleep/` - Sleep quality monitoring
- `mothership/internal/falldetect/` - Fall detection alert chain
- `mothership/internal/flow/` - Crowd flow accumulation
- `mothership/internal/replay/` - Time-travel replay engine
- `mothership/internal/simulator/` - CSI simulator integration

**Infrastructure:**
- `mothership/internal/auth/` - HMAC token derivation, PIN bcrypt, sessions
- `mothership/internal/config/` - Environment variable parsing
- `mothership/internal/mqtt/` - MQTT client, HA auto-discovery
- `mothership/internal/notify/` - Notification renderer (PNG thumbnails)
- `mothership/internal/doctor/` - System health diagnostics
- `mothership/internal/provisioning/` - Provisioning payload generation
- `mothership/internal/oui/` - OUI lookup table (generated from IEEE registry)
- `mothership/internal/startup/` - Startup sequencing
- `mothership/internal/shutdown/` - Graceful shutdown
- `mothership/internal/health/` - Health checks
- `mothership/internal/loadshed/` - Load shedding under high CPU
- `mothership/internal/events/` - Unified event timeline
- `mothership/internal/timeline/` - Timeline management
- `mothership/internal/zones/` - Zone management
- `mothership/internal/apdetector/` - AP auto-detection for passive radar
- `mothership/internal/ntpserver/` - NTP server for nodes
- `mothership/internal/briefing/` - Morning briefing generation
- `mothership/internal/guidedtroubleshoot/` - Contextual help system
- `mothership/internal/explainability/` - Detection explanation UI data
- `mothership/internal/diagnostics/` - Per-link diagnostics
- `mothership/internal/diskpace/` - Disk space monitoring
- `mothership/internal/eventbus/` - Event bus
- `mothership/internal/help/` - Help system
- `mothership/internal/learning/` - Learning models
- `mothership/internal/portal/` - Portal crossing detection
- `mothership/internal/tracking/` - Person tracking
- `mothership/internal/volume/` - Spatial volumes
- `mothership/internal/webhook/` - Webhook client

**Go module definition:**
- `mothership/go.mod` - Go module dependencies
- `mothership/go.sum` - Dependency checksums

**Build trigger:** Any change to Go source files requires:
1. Go binary rebuild (`go build`)
2. Docker image rebuild
3. Container deployment

---

## 3. Dashboard (Web UI)

### Dashboard Files
**Path:** `dashboard/`
**Impact:** Changes require Go binary rebuild (dashboard embedded via `go:embed`)

**Core files:**
- `dashboard/index.html` - Main dashboard entry point
- `dashboard/live.html` - Live 3D view
- `dashboard/simple.html` - Simple mode (card-based)
- `dashboard/ambient.html` - Ambient mode (wall-mounted display)
- `dashboard/setup.html` - Setup/calibration view
- `dashboard/fleet.html` - Fleet status table
- `dashboard/integrations.html` - Integrations configuration
- `dashboard/simulator.html` - Pre-deployment simulator
- `dashboard/test-transformcontrols.html` - TransformControls test page

**JavaScript application code:**
- `dashboard/js/` - All application JavaScript files
  - `blob-identity.js` - BLE-to-blob identity matching
  - `linkhealth.js` - Link health visualization
  - `placement.js` - Node placement UI
  - (and all other `.js` files in `dashboard/js/`)

**Service worker:**
- `dashboard/sw.js` - Service worker for offline support

**Build configuration:**
- `dashboard/package.json` - NPM dependencies for dev tools

**Tests:**
- `dashboard/tests/` - Accessibility tests (axe-core)
- `dashboard/jest.config.js` - Jest config
- `dashboard/playwright.config.js` - Playwright config

**Note:** Dashboard is embedded into the Go binary at build time via `go:embed` directive in `cmd/mothership/main.go`. Changes to any dashboard file require a mothership binary rebuild.

**Build trigger:** Any change to dashboard files requires:
1. Go binary rebuild (to re-embed dashboard)
2. Docker image rebuild
3. Container deployment

---

## 4. Simulator & Test Tools

### CSI Simulator
**Path:** `cmd/sim/`
**Impact:** Changes require simulator binary rebuild

**Files:**
- `cmd/sim/main.go` - Simulator entry point
- `cmd/sim/go.mod` - Go module definition

**Build trigger:** Changes require simulator rebuild (included in Docker image)

### Acceptance Tests
**Path:** `test/acceptance/`, `mothership/test/acceptance/`
**Impact:** Changes do NOT trigger production builds (test-only code)

**Note:** These tests use `spaxel-sim` to validate end-to-end behavior

---

## 5. Docker & Deployment Configuration

### Container Build
**Path:** `Dockerfile`
**Impact:** Changes require full Docker rebuild and redeployment

**What it controls:**
- Multi-stage build process (firmware fetcher, Go builder, runtime)
- Build arguments (VERSION, TARGETPLATFORM, TARGETARCH)
- Binary compilation flags (CGO_ENABLED=0, go build tags)
- Volume mounts (/data)
- Exposed ports (8080)
- Entry point (/spaxel)

**Build trigger:** Changes require:
1. Docker image rebuild (`docker buildx build`)
2. Container registry push
3. Kubernetes deployment update

### Container Orchestration
**Path:** `docker-compose.yml`
**Impact:** Changes require Docker Compose redeployment

**What it controls:**
- Service configuration (image, ports, volumes, environment)
- Network mode (host networking for mDNS)
- Resource limits (memory, CPU)
- Health checks
- Restart policy
- Traefik labels for ingress

**Build trigger:** Changes require `docker compose up -d` or equivalent deployment update

---

## 6. Build System & Versioning

### Go Workspace
**Path:** `go.work`, `go.work.sum`
**Impact:** Changes affect Go module resolution

**Purpose:** Defines multi-module workspace (mothership, cmd/sim, test/acceptance)

### Version File
**Path:** `VERSION`
**Impact:** Changes trigger versioned builds

**Purpose:** Single source of truth for release version (e.g., "0.2.94")

**Build trigger:** Version bump should trigger:
1. Firmware build with new version
2. Go binary build with version ldflag
3. Docker image build with version tag
4. GitHub Release creation

---

## 7. Scripts & Tools (Non-Build-Triggering)

### Operational Scripts
**Path:** `scripts/`
**Impact:** Changes do NOT trigger builds (runtime utilities)

**Examples:**
- `scripts/provision_esp32.py` - ESP32 provisioning helper
- `scripts/measure_csi_rate.py` - CSI rate measurement

### Developer Scripts
**Path:** Root-level `.sh` files
**Impact:** Changes do NOT trigger builds (development tools)

**Examples:**
- `blob_observation.sh` - Blob observation helper
- `window_test.sh` - Window testing script

---

## 8. Documentation (Non-Build-Triggering)

### Documentation Paths
**Paths:** `docs/`, `notes/`, various `*.md` files
**Impact:** Changes do NOT trigger builds

**Exceptions:** None - documentation changes never require rebuilds

---

## Build Trigger Summary

| Component | Path | Rebuild Required | Deployment Required |
|-----------|------|------------------|---------------------|
| **Firmware** | `firmware/main/*.c`, `firmware/main/*.h` | Yes (ESP-IDF) | Yes (OTA) |
| **Firmware Config** | `firmware/CMakeLists.txt`, `firmware/sdkconfig.defaults` | Yes (ESP-IDF) | Yes (OTA) |
| **Mothership** | `mothership/**/*.go` | Yes (Go build) | Yes (Docker) |
| **Dashboard** | `dashboard/**/*` (all HTML, JS, etc.) | Yes (Go build for embed) | Yes (Docker) |
| **Simulator** | `cmd/sim/**/*.go` | Yes (Go build) | Yes (Docker) |
| **Dockerfile** | `Dockerfile` | Yes (Docker build) | Yes (Docker) |
| **Compose** | `docker-compose.yml` | No | Yes (Compose) |
| **Version** | `VERSION` | Yes (all builds) | Yes (all deployments) |
| **Go Workspace** | `go.work`, `go.work.sum` | Yes (Go build) | Yes (Docker) |
| **Tests** | `firmware/test/`, `test/acceptance/`, `mothership/test/` | No | No |
| **Scripts** | `scripts/**/*`, `*.sh` | No | No |
| **Docs** | `docs/**/*`, `*.md` | No | No |

---

## Implementation Notes

### CI/CD Integration

This catalog feeds directly into CI path filters. For the `spaxel-build` WorkflowTemplate:

1. **Firmware builds** should trigger on changes to:
   - `firmware/main/**`
   - `firmware/CMakeLists.txt`
   - `firmware/sdkconfig.defaults`

2. **Go/mothership builds** should trigger on changes to:
   - `mothership/**`
   - `cmd/sim/**`
   - `dashboard/**`
   - `go.work`
   - `VERSION`
   - `Dockerfile`

3. **Documentation-only commits** should skip all build steps (ADR-009 decision 3)

### Path Filter Example

For a git-commit-based trigger:
```bash
# Firmware build trigger
git diff --name-only HEAD~1 HEAD | grep -qE '^firmware/(main/|CMakeLists.txt|sdkconfig.defaults)'

# Go build trigger
git diff --name-only HEAD~1 HEAD | grep -qE '^(mothership|cmd/sim|dashboard|go\.work|VERSION|Dockerfile)/'

# Skip builds for docs-only commits
git diff --name-only HEAD~1 HEAD | grep -qvE '^(\.md|docs/|notes/|README)'
```

### Deployment Impact Levels

**Critical Impact (requires OTA rollout):**
- Any firmware change
- Changes protocol between mothership and nodes

**High Impact (requires container redeployment):**
- Mothership Go code changes
- Dashboard changes
- Dockerfile changes

**Medium Impact (requires orchestration update):**
- docker-compose.yml changes

**Low Impact (no deployment):**
- Test code changes
- Documentation updates
- Script changes

---

## Related Documentation

- `docs/plan/plan.md` - Full system architecture
- `ADR-001` - Decoupling firmware build from mothership image
- `ADR-009` - Automatic convergence enforcement
- `PROGRESS.md` - Implementation status
