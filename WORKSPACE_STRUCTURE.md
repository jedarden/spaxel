# Spaxel Workspace Directory Structure

**Generated:** 2026-08-29  
**Workspace Root:** `/home/coding/spaxel`

This document catalogs all top-level directories and modules within the spaxel workspace to understand the current layout.

---

## Overview

Spaxel is a WiFi CSI-based indoor positioning system for self-hosted homes. The workspace is organized as a multi-module Go workspace with separate components for the mothership (Go backend), firmware (ESP32-S3), dashboard (web UI), and testing infrastructure.

---

## Go Workspace Modules

The workspace uses Go 1.25.0 with three Go modules defined in `go.work`:

```
go 1.25.0

use ./mothership

use (
    ./cmd/sim
    ./test/acceptance
)
```

### Module 1: `mothership/` (Primary Backend)
- **Purpose:** Main Go backend serving as the "mothership" container
- **Entry Point:** `mothership/cmd/mothership/main.go`
- **Structure:**
  - `cmd/mothership/` - Main application entry point
  - `internal/` - Core backend packages (40+ packages):
    - `ingestion/` - WebSocket server, binary frame parsing, node lifecycle
    - `pipeline/` - Signal processing (phase/, nbvi/, feature/, baseline/)
    - `localizer/` - Fresnel zone localization (fresnel/, ukf/, gdop/, fusion/)
    - `fleet/` - Node registry, role assignment, stagger scheduling
    - `ble/` - BLE identity matching
    - `portal/` - Room crossing detection
    - `replay/` - CSI replay buffer (csi_replay.bin reader/writer)
    - `anomaly/` - Pattern learning, anomaly scoring
    - `predict/` - Presence prediction models
    - `sleep/` - Sleep quality monitoring
    - `flow/` - Crowd flow visualization
    - `notify/` - Notification rendering (fogleman/gg), delivery channels
    - `mqtt/` - MQTT client, HA auto-discovery
    - `auth/` - HMAC token derivation, PIN authentication, sessions
    - `oui/` - OUI lookup table (go:generate from IEEE list)
    - `db/` - SQLite migrations and schema
    - `config/` - Environment variable parsing
    - `api/` - REST API handlers
    - `dashboard/` - Embedded dashboard assets (go:embed)
    - `provisioning/` - Node provisioning payload generation
    - `ota/` - OTA update system
    - `apdetector/` - AP auto-detection for passive radar
    - And 15+ other specialized packages
  - `test/acceptance/` - In-module acceptance tests using spaxel-sim
- **Key Tech:** Go 1.25, pure Go SQLite (modernc.org/sqlite), WebSocket (gorilla/websocket)

### Module 2: `cmd/sim/` (CSI Simulator CLI)
- **Purpose:** CSI simulator for hardware-free testing
- **Entry Point:** `cmd/sim/main.go`
- **Binary:** `spaxel-sim` (built separately, baked into Docker image)
- **Function:** Emulates ESP32 nodes connecting to mothership, sending synthetic CSI binary frames
- **Usage:** Integration testing without physical hardware

### Module 3: `test/acceptance/` (Cross-cutting Acceptance Tests)
- **Purpose:** Cross-cutting acceptance/integration tests
- **Structure:** Separate Go module for orchestrating mothership + simulator tests
- **Tests:** Validates end-to-end scenarios from fresh install to multi-node operation

---

## Top-Level Directories

### `/mothership/` - Primary Backend Module
- **Type:** Go module (mothership/go.mod)
- **Purpose:** The single Docker container "mothership" — all backend services
- **Key Files:**
  - `go.mod`, `go.sum` - Go module definition
  - `cmd/mothership/main.go` - Application entry point
  - `internal/` - 40+ internal packages (see Go Workspace Modules above)
  - `test/acceptance/` - In-module acceptance tests
  - `tests/e2e/` - Shell-based end-to-end test harness (run.sh)

### `/cmd/sim/` - CSI Simulator
- **Type:** Go module (cmd/sim/go.mod)
- **Purpose:** Command-line tool that emulates ESP32 nodes for testing
- **Key Files:**
  - `main.go` - Simulator entry point
  - `go.mod` - Module definition
- **Output:** `spaxel-sim` binary (included in Docker image)

### `/test/acceptance/` - Cross-Cutting Tests
- **Type:** Go module (test/acceptance/go.mod)
- **Purpose:** Cross-module acceptance/integration tests
- **Usage:** Validates mothership + simulator interaction patterns

### `/firmware/` - ESP32-S3 Firmware
- **Type:** ESP-IDF C project (non-Go)
- **Purpose:** ESP32-S3 firmware that runs on each node
- **Structure:**
  - `main/` - Main firmware source (main.c, wifi.c, csi.c, ws.c, ble.c, ota.c, nvs.c, serial_prov.c)
  - `test/` - Host-based gcc tests (test_*.c)
  - `build/` - ESP-IDF build artifacts (bootloader, partition table, app image)
  - `managed_components/` - ESP-IDF components (esp_websocket_client, mdns)
  - `docs/` - Firmware documentation
  - `scripts/` - Build/utility scripts
- **Key Tech:** ESP-IDF 5.2.x, FreeRTOS tasks, CSI callback, WebSocket client, BLE scan

### `/dashboard/` - Web UI
- **Type:** Static web assets (HTML, JS, CSS)
- **Purpose:** Single-page application dashboard served by mothership
- **Key Files:**
  - `index.html` - Main dashboard (expert mode with Three.js 3D scene)
  - `simple.html` - Simple mode (mobile-first card UI)
  - `ambient.html` - Ambient mode (wall-mounted tablet display)
  - `setup.html` - Interactive onboarding wizard
  - `simulator.html` - CSI simulator UI
  - `test-*.html` - Various test pages
  - `manifest.json` - Web app manifest
  - `sw.js` - Service worker
  - `package.json` - Node.js dependencies (dev tools: jest, esbuild)
  - `tsconfig.json` - TypeScript config (if using TS)
  - `README.md` - Dashboard documentation
- **Key Tech:** Vanilla JS + Three.js (no build toolchain for production)
- **Integration:** Embedded into Go binary via `//go:embed` (copied to `mothership/cmd/mothership/dashboard/` at build time)

### `/docs/` - Documentation
- **Type:** Project documentation
- **Structure:**
  - `plan/plan.md` - Master implementation plan (this document lives here)
  - `notes/` - Feature-specific notes (individual beads have subfolders)
  - `research/` - Third-party research, prior art, study notes
  - `deployment/` - Deployment documentation
  - `design/` - Design documents
  - `examples/` - Example configurations
  - `tests/` - Test documentation
  - `inventory/` - Code inventory

### `/data/` - Runtime Data
- **Type:** Docker volume mount point
- **Purpose:** Persistent data directory
- **Contents:**
  - `spaxel.db` - SQLite database (nodes, zones, events, baselines, predictions, etc.)
  - `csi_replay.bin` - CSI recording buffer (append-only circular buffer, 48h default)
  - `floorplan/` - Floor plan images
  - `firmware/` - Uploaded firmware binaries for OTA

### `/dashboard/` - (Duplicate entry - already listed)
- See above

### `/scripts/` - Utility Scripts
- **Type:** Shell and utility scripts
- **Purpose:** Development and deployment helpers
- **Examples:** Build scripts, deployment helpers, test utilities

### `/testdata/` - Test Data
- **Type:** Test fixtures
- **Purpose:** Static data for unit/integration tests

### `/tests/` - Test Infrastructure
- **Type:** Shell-based E2E tests
- **Purpose:** End-to-end test harness
- **Key File:** `run.sh` - Test runner script

### `/notes/` - Investigation Notes
- **Type:** Working notes and investigations
- **Purpose:** Temporary notes during development/debugging
- **Contents:** Bug investigation reports, verification notes

### `/memory/` - Agent Memory
- **Type:** Persistent memory for Claude agents
- **Purpose:** Cross-session memory for Claude Code
- **Contents:** `MEMORY.md` index, topic-specific memory files

---

## Configuration Files (Root Level)

### Deployment & Build
- **`Dockerfile`** - Multi-stage build (ESP-IDF firmware → Go binary → distroless runtime)
- **`docker-compose.yml`** - Single-service deployment manifest
- **`.dockerignore`** - Docker build exclusions
- **`VERSION`** - Single source of truth for release version (read by Dockerfile, CI, etc.)

### Go Workspace
- **`go.work`** - Go workspace definition (stitches mothership, cmd/sim, test/acceptance)
- **`go.work.sum`** - Go workspace checksum

### Git
- **`.git/`** - Git repository
- **`.gitattributes`** - Git attributes
- **`.gitignore`** - Git ignore patterns

### Linting & Quality
- **`.golangci.yml`** - golangci-lint configuration

### NEEDLE Fleet Dispatch
- **`.needle.yaml`** - NEEDLE fleet dispatch configuration
- **`.needle-predispatch-sha`** - NEEDLE predispatch SHA tracking

### Bead Tracking
- **`.beads/`** - Bead tracking storage (bead-rs or bf backend)
  - `checkpoint/` - Bead checkpoint (git-tracked: current.json, forensic.jsonl, objects/)
  - `config.json` or `config.yaml` - Bead backend config

### Marathon
- **`.marathon/`** - Marathon-related files (if applicable)

---

## Top-Level Markdown Documentation Files

### Status & Progress
- **`README.md`** - Project overview and quickstart
- **`PROGRESS.md`** - Implementation progress tracking (83 implementation beads archived)
- **`DIRECTORY_STRUCTURE.md`** - (This file may duplicate this document)
- **`WORKSPACE_STRUCTURE_DOCUMENTATION.md`** - May be a duplicate or related

### Investigation & Research
- **`SYSTEM_CATALOG.md`** - Complete system catalog
- **`SOURCE_CODE_INVENTORY.md`** - Source code inventory
- **`CODEBASE_STRUCTURE_AND_TEST_PATTERNS.md`** - Codebase structure and testing patterns
- **`MOTHERSHIP_DASHBOARD_LOCATIONS.md`** - Dashboard location mapping
- **`MOTHERSHIP_DASHBOARD_STARTUP_INVESTIGATION.md`** - Startup investigation
- **`GDOP_COMPUTATION_GUIDE.md`** - GDOP algorithm guide
- **`API_IMPLEMENTATION_STATUS.md`** - API implementation status

### Bug & Issue Tracking
- **`bug_verification_report.md`** - Bug verification reports
- **`console-implementation-status.md`** - Console implementation status
- **`CSI_RECORDING_FILES_SEARCH_RESULTS.md`** - CSI recording file search
- **`ble_person_investigation.md`** - BLE person identification investigation
- **`mothership_verification.md`** - Mothership verification notes
- **`rust-source-inventory.md`** - Rust source inventory (if applicable)
- **`rust-test-modules-report.md`** - Rust test modules report

### CI/CD
- **`acceptance-test-hang-workflow.yml`** - CI workflow configuration

### Deployment
- **`declarative-config-verification-report.md`** - Declarative config verification
- **`BENCH_HOSTNAME_INFO.md`** - Bench hostname information
- **`dashboard_discovery_notes.md`** - Dashboard discovery notes
- **`gdop_search_results.md`** - GDOP search results
- **`mothership_location.md`** - Mothership location notes
- **`search-prerequisites-verification.md`** - Search prerequisites verification
- **`top_level_directories.md`** - Top-level directories overview

### Scripts
- **`blob_observation.sh`** - Blob observation utility
- **`window_test.sh`** - Window test script
- **`fix_ble_handlers.py`** - BLE handler fix script (Python)

---

## Key Module Relationships

```
┌─────────────────────────────────────────────────────────────────┐
│ Docker Image (Single Container)                                  │
│                                                                   │
│  ┌────────────────────────────────────────────────────────┐    │
│  │ Go Workspace (go.work)                                 │    │
│  │                                                         │    │
│  │ Module 1: mothership/ (Primary Backend)               │    │
│  │   ├── cmd/mothership/main.go (entry point)             │    │
│  │   ├── internal/ (40+ packages)                         │    │
│  │   ├── cmd/mothership/dashboard/ (embedded UI assets)    │    │
│  │   └── test/acceptance/ (in-module tests)               │    │
│  │                                                         │    │
│  │ Module 2: cmd/sim/ (CSI Simulator CLI)                │    │
│  │   └── main.go                                          │    │
│  │                                                         │    │
│  │ Module 3: test/acceptance/ (Cross-cutting tests)       │    │
│  │   └── (acceptance test suites)                        │    │
│  └────────────────────────────────────────────────────────┘    │
│                                                                   │
│  ┌──────────────┐  ┌──────────────────┐                      │
│  │ dashboard/   │  │ firmware/         │                      │
│  │ (Static Web) │  │ (ESP-IDF C)       │                      │
│  └──────────────┘  └──────────────────┘                      │
│                                                                   │
│  Firmware artifact built once (amd64-only), copied into         │
│  all platform images                                            │
└─────────────────────────────────────────────────────────────────┘
```

---

## Development Workflow

### Module Independence
- **mothership/** - Independent Go module, can be developed and tested alone
- **cmd/sim/** - Independent Go module for simulation
- **test/acceptance/** - Cross-cutting tests that validate integration
- **firmware/** - Independent ESP-IDF C project, built separately
- **dashboard/** - Static assets, no build step (embedded into Go binary)

### Build Order
1. Build ESP32-S3 firmware (amd64 only in firmware-builder stage)
2. Build Go binaries for each target platform (amd64, arm64)
3. Copy firmware artifact into all platform images
4. Assemble multi-arch manifest list

### Testing Strategy
- **Unit tests:** `go test ./...` within each module
- **Acceptance tests:** `test/acceptance/` + `mothership/test/acceptance/`
- **Integration tests:** `spaxel-sim` + running mothership
- **Firmware tests:** Host-based gcc tests in `firmware/test/`
- **E2E tests:** Shell harness in `tests/e2e/run.sh`

---

## Deployment Artifacts

### Docker Image Contents
- **Mothership binary** (`/spaxel`) - Entry point
- **Simulator binary** (`/spaxel-sim`) - CSI simulation tool
- **Firmware binary** (`/firmware/spaxel-firmware-merged.bin`) - ESP32-S3 firmware
- **Embedded dashboard** - Static assets baked into binary via `//go:embed`

### Runtime Data Volume (`/data`)
- SQLite database (`spaxel.db`)
- CSI replay buffer (`csi_replay.bin`)
- Floor plans (`floorplan/`)
- Firmware uploads (`firmware/`)

---

## Architecture Summary

**Mothership (Go):**
- Backend server running all services
- SQLite for persistence
- WebSocket for node communication and dashboard feed
- HTTP/REST for API
- No external dependencies (pure Go SQLite)

**Firmware (ESP-IDF C):**
- Runs on ESP32-S3 hardware
- WiFi, CSI capture, WebSocket client, BLE scan
- OTA update support
- mDNS for mothership discovery

**Dashboard (Vanilla JS + Three.js):**
- Single-page application
- 3D scene (expert mode)
- Simple mode (mobile-first)
- Ambient mode (wall-mounted display)
- Embedded in Go binary, served by mothership

---

## Notes

- This workspace uses a **Go workspace** (Go 1.25.0 feature) to stitch together multiple Go modules
- **No `test/integration/` directory** — integration tests live in `test/acceptance/` and `mothership/test/acceptance/`
- **Dashboard is embedded** — no volume mount needed for UI; updating UI requires new Docker image
- **Firmware built once per architecture** — ESP-IDF build runs on amd64, artifact copied to all platform images
- **Pure Go SQLite** — no CGO, no `gcc` needed in final image (uses `modernc.org/sqlite`)
- **Multi-arch support** — Builds both `linux/amd64` and `linux/arm64` images

---

## Related Documentation

- **Master Plan:** `docs/plan/plan.md` - Full architecture and implementation plan
- **Progress:** `PROGRESS.md` - Implementation progress and archived beads
- **Go Module Layout:** See `go.work` and individual `go.mod` files
- **Firmware Build:** `firmware/` directory with ESP-IDF project structure
