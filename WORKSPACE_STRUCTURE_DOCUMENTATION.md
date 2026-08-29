# Spaxel Workspace Directory Structure

**Generated:** 2026-08-29  
**Workspace:** `/home/coding/spaxel`

## Overview

Spaxel is a WiFi CSI-based indoor positioning system consisting of a Go mothership backend, ESP32-S3 firmware, and a vanilla JavaScript dashboard. The workspace is organized as a polyglot repository with three Go modules managed by a Go workspace.

## Top-Level Directory Structure

### Core Modules (Go Workspace)

| Directory | Type | Purpose |
|-----------|------|---------|
| **`mothership/`** | Go module | Primary Go backend - mothership server (cmd/mothership/ entrypoint, internal/ packages) |
| **`cmd/sim/`** | Go module | CSI simulator CLI (spaxel-sim) - synthetic node emulation for testing |
| **`test/acceptance/`** | Go module | Cross-cutting acceptance/integration tests |

**Go Workspace Configuration** (`go.work`):
```go
go 1.25.0

use ./mothership

use (
    ./cmd/sim
    ./test/acceptance
)
```

### Firmware & Device Code

| Directory | Type | Purpose |
|-----------|------|---------|
| **`firmware/`** | ESP-IDF C project | ESP32-S3 firmware source code (CMake-based build) |
| `firmware/main/` | C source | Main firmware application code |
| `firmware/test/` | C test harness | Host-based firmware tests (gcc harness) |
| `firmware/build/` | Build artifacts | Compiled firmware binaries |

### Dashboard & Frontend

| Directory | Type | Purpose |
|-----------|------|---------|
| **`dashboard/`** | Static web assets | Vanilla JavaScript + Three.js web UI |
| `dashboard/js/` | JavaScript | Main application code |
| `dashboard/css/` | CSS | Stylesheets |
| `dashboard/static/` | Static assets | Embedded static files (served via Go embed) |
| `dashboard/types/` | TypeScript | Type definitions |
| `dashboard/tests/` | Web tests | Frontend test suites |
| `dashboard/node_modules/` | NPM packages | JavaScript dependencies |

### Data & State

| Directory | Type | Purpose |
|-----------|------|---------|
| **`data/`** | Runtime data | Persistent data directory (SQLite, floorplans, CSI recordings) |
| `data/backups/` | Backups | Database backups and pre-upgrade snapshots |
| `data/floorplan/` | Floorplans | User-uploaded floor plan images |
| `data/firmware/` | Firmware | OTA firmware binaries |
| `data/csi/` | CSI data | CSI replay buffer (`csi_replay.bin`) |
| `data/simulator/` | Simulator data | CSI simulator runtime data |

### Documentation & Planning

| Directory | Type | Purpose |
|-----------|------|---------|
| **`docs/`** | Documentation | Comprehensive project documentation |
| `docs/plan/` | Planning | Implementation plan and architecture docs |
| `docs/research/` | Research | Third-party research and reference material |
| `docs/notes/` | Notes | Design notes and decisions |
| `docs/design/` | Design | Design documentation |
| `docs/deployment/` | Deployment | Deployment guides |
| `docs/inventory/` | Inventory | Code inventory and catalog |
| `docs/examples/` | Examples | Usage examples |
| `docs/tests/` | Test docs | Testing documentation |
| `docs/traces/` | Traces | Trace analysis documentation |

### Testing

| Directory | Type | Purpose |
|-----------|------|---------|
| **`test/`** | Test entry | Test organization (non-Go) |
| `test/acceptance/` | Go tests | Acceptance tests (separate Go module) |
| **`tests/`** | Test suites | E2E and integration test harnesses |
| `tests/e2e/` | Shell tests | End-to-end shell test scripts |
| `testdata/` | Test fixtures | Test data and fixtures |
| `mothership/test/` | Go tests | Mothership acceptance tests (in-module) |
| `mothership/tests/` | Go tests | Additional mothership tests |

### Build & Deployment

| Directory | Type | Purpose |
|-----------|------|---------|
| **`scripts/`** | Build scripts | Utility scripts for building and testing |
| `*.sh` files | Shell scripts | Various utility scripts (blob_observation.sh, window_test.sh, etc.) |
| **`Dockerfile`** | Container | Multi-stage Docker build (ESP-IDF + Go + distroless runtime) |
| **`docker-compose.yml`** | Orchestration | Single-service deployment manifest |
| **`.dockerignore`** | Build config | Docker build exclusions |

### Configuration & Versioning

| File/Directory | Type | Purpose |
|---------------|------|---------|
| **`VERSION`** | Versioning | Single source of truth for release version |
| **`go.work`** | Go workspace | Go workspace module configuration |
| **`go.work.sum`** | Go dependencies | Go workspace dependency checksums |
| **`.needle.yaml`** | NEEDLE config | NEEDLE fleet dispatcher configuration |
| **`.golangci.yml`** | Linting | golangci-lint configuration |
| **`.gitignore`** | Git | Git exclusions |
| **`.gitattributes`** | Git | Git attributes |

### Memory & Notes

| Directory | Type | Purpose |
|-----------|------|---------|
| **`memory/`** | Agent memory | Claude Code agent memory files |
| **`notes/`** | Notes | Additional notes and observations |
| **`.claude/`** | Claude config | Claude Code workspace configuration |

### Build Artifacts & Cache

| Directory | Type | Purpose |
|-----------|------|---------|
| **`mothership/build/`** | Build output | Mothership build artifacts |
| `mothership/mothership` | Compiled binary | Built mothership executable |
| `mothership/sim` | Compiled binary | Built simulator executable |
| `mothership/*.test` | Test binaries | Compiled Go test binaries |
| `dashboard/node_modules/` | NPM cache | Installed JavaScript packages |

### Git & Version Control

| Directory | Type | Purpose |
|-----------|------|---------|
| **`.git/`** | Git repository | Git version control data |
| `.git/branches/` | Git | Branch references |
| `.git/worktrees/` | Git | Git worktree data |
| `.beads/` | Bead tracking | Bead-rs issue tracking system |

### CI/CD Configuration

| File | Type | Purpose |
|------|------|---------|
| **`acceptance-test-hang-workflow.yml`** | CI config | Argo Workflow template for testing |
| `*.yml` files | CI configs | Various CI/CD configurations |

### Documentation Files (Root Level)

| File | Type | Purpose |
|------|------|---------|
| **`README.md`** | Documentation | Main project README |
| **`LICENSE`** | Legal | Project license |
| **`PROGRESS.md`** | Status | Implementation progress tracking |
| **`DIRECTORY_STRUCTURE.md`** | Documentation | Existing directory structure docs |
| **`SYSTEM_CATALOG.md`** | Documentation | System component catalog |
| **`SOURCE_CODE_INVENTORY.md`** | Documentation | Code inventory |
| **`CODEBASE_STRUCTURE_AND_TEST_PATTERNS.md`** | Documentation | Codebase structure documentation |
| Various `.md` files | Documentation | Specialized documentation (ADR, investigations, etc.) |

## Mothership Internal Packages

The `mothership/internal/` directory contains 54 internal packages organized by domain:

### Core Signal Processing
- `signal/` - Signal processing utilities
- `pipeline/` - CSI processing pipeline
  - `phase/` - Phase sanitization
  - `nbvi/` - NBVI subcarrier selection
  - `feature/` - Feature extraction
  - `baseline/` - Baseline management

### Localization & Detection
- `localizer/` - Spatial localization
  - `fresnel/` - Fresnel zone computation
  - `ukf/` - Biomechanical UKF filter
  - `gdop/` - Geometric Dilution of Precision
  - `fusion/` - Multi-link fusion engine

### Fleet & Node Management
- `fleet/` - Fleet manager and role assignment
- `ingestion/` - WebSocket ingestion server
- `provisioning/` - Node provisioning
- `ota/` - OTA update system
- `auth/` - Authentication and session management

### Automation & Intelligence
- `automation/` - Spatial automation triggers
- `anomaly/` - Anomaly detection
- `prediction/` - Presence prediction
- `learning/` - Machine learning components
- `sleep/` - Sleep quality monitoring
- `ble/` - BLE device scanning and identity

### Dashboard & API
- `dashboard/` - Dashboard WebSocket feed
- `api/` - REST API handlers
- `events/` - Event system and timeline
- `timeline/` - Timeline management
- `explainability/` - Detection explanation

### Data & Persistence
- `db/` - SQLite database management
- `recorder/` - CSI recording
- `recording/` - CSI replay system
- `replay/` - Time-travel debugging
- `zones/` - Zone management
- `floorplan/` - Floor plan handling

### Monitoring & Diagnostics
- `health/` - System health monitoring
- `diagnostics/` - Diagnostic tools
- `doctor/` - System doctor
- `loadshed/` - Load shedding under pressure
- `diskspace/` - Disk space monitoring
- `startup/` - Startup sequencing

### Integration & Notifications
- `mqtt/` - MQTT client integration
- `notifications/` - Notification system
- `notify/` - Notification rendering
- `webhook/` - Webhook delivery
- `github/` - GitHub API integration

### Specialized Features
- `falldetect/` - Fall detection algorithm
- `tracking/` - Blob tracking
- `tracker/` - Person tracking
- `volume/` - 3D volume handling
- `briefing/` - Morning briefing generation
- `guidedtroubleshoot/` - Guided troubleshooting system
- `help/` - Help system
- `ntpserver/` - NTP server configuration
- `oui/` - OUI lookup table
- `apdetector/` - AP auto-detection
- `simulator/` - CSI simulator integration
- `config/` - Configuration management
- `eventbus/` - Event bus
- `analytics/` - Analytics components
- `autoupdate/` - Auto-update management
- `render/` - Rendering services
- `shutdown/` - Graceful shutdown

## Key Build Artifacts

### Mothership Module
- `mothership/cmd/mothership/` - Main entry point
- `mothership/go.mod` - Go module definition
- `mothership/go.sum` - Dependency checksums

### Firmware Module
- `firmware/CMakeLists.txt` - CMake build configuration
- `firmware/sdkconfig` - ESP-IDF configuration
- `firmware/partitions.csv` - Flash partition table
- `firmware/main/` - Component: main application
- `firmware/managed_components/` - ESP-IDF components

## Module Classification

### Primary Go Modules (3)
1. **`mothership`** - Core backend application
2. **`cmd/sim`** - CSI simulator CLI
3. **`test/acceptance`** - Cross-cutting acceptance tests

### Non-Go Modules
- **`firmware/`** - ESP-IDF C project
- **`dashboard/`** - Static JavaScript web application

## Dependencies

### Go Dependencies
- Managed via `mothership/go.mod` and `go.work.sum`
- Key dependencies: `modernc.org/sqlite`, `gorilla/websocket`, `gonum.org/v1/gonum/mat`

### JavaScript Dependencies
- Managed via `dashboard/package.json`
- Key dependencies: Three.js (3D rendering), test frameworks

### Firmware Dependencies
- Managed via ESP-IDF component system
- ESP-IDF version 5.2.x

## Data Flow

```
firmware/ → ESP32-S3 devices
mothership/ → Docker container (Go backend)
dashboard/ → Served by mothership (static assets)
test/ → CI/CD validation
```

## Summary

The Spaxel workspace is a well-organized polyglot repository with:

- **3 Go modules** managed by a Go workspace
- **1 ESP-IDF C project** for embedded firmware
- **1 JavaScript dashboard** for web UI
- **54 internal packages** in mothership organized by domain
- **Comprehensive documentation** in `docs/` with plan, research, and design subdirectories
- **Testing infrastructure** spanning unit, integration, and E2E tests
- **Build automation** via Docker, CMake, and Go tooling

The structure supports the architecture of a WiFi CSI-based indoor positioning system with clear separation between firmware, backend, and frontend components.
