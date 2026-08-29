# Spaxel Repository Directory Structure

**Last updated:** 2026-08-29  
**Repository:** /home/coding/spaxel/  
**Purpose:** WiFi CSI-based indoor positioning system for self-hosted homes

## Overview

Spaxel is a WiFi CSI-based indoor positioning system with a Go backend (mothership), ESP32-S3 firmware, and a vanilla JavaScript + Three.js dashboard. This document catalogs the repository's directory structure, excluding build outputs and scratch directories.

---

## Top-Level Directories

### Core Application Components

| Directory | Purpose | Contents |
|-----------|---------|-----------|
| `mothership/` | Go backend server | WebSocket ingestion, signal processing pipeline, localization, fleet management, dashboard server, SQLite persistence |
| `firmware/` | ESP32-S3 firmware (ESP-IDF C) | WiFi CSI capture, WebSocket client, BLE scanning, OTA updates, provisioning |
| `dashboard/` | Frontend UI | Vanilla JS + Three.js 3D visualization, HTML views, CSS, static assets |
| `cmd/` | Command-line tools | `sim` - CSI simulator CLI for development/testing |
| `docs/` | Project documentation | Plans, research, design notes, deployment guides, inventory |

### Build and Deployment

| Directory | Purpose | Notes |
|-----------|---------|-------|
| `Dockerfile` | Container image build | Multi-stage: ESP-IDF firmware build + Go binary + distroless runtime |
| `docker-compose.yml` | Single-service deployment | Production deployment manifest |
| `.dockerignore` | Docker build exclusions | Build-time filtering |
| `go.work` | Go workspace definition | Stitches together 3 Go modules (mothership, cmd/sim, test/acceptance) |
| `go.work.sum` | Go workspace dependencies | Module checksums |
| `VERSION` | Release version | Single source of truth for release versioning |

### Testing and Quality

| Directory | Purpose | Contents |
|-----------|---------|-----------|
| `test/` | Cross-cutting acceptance tests | `acceptance/` - simulator-based integration tests |
| `tests/` | End-to-end test harness | `e2e/` - shell-based test harness (`run.sh`) |
| `testdata/` | Test fixtures and data | Static test data files |
| `.golangci.yml` | Go linting configuration | Quality gate configuration |

### Data and Runtime

| Directory | Purpose | Notes |
|-----------|---------|-------|
| `data/` | Runtime data directory | SQLite, floor plans, CSI replay buffer, firmware uploads |
| `.beads/` | Bead tracking state | `beads.db` (SQLite), checkpoint/, events.jsonl, heartbeats.jsonl, receipts/ |

### Documentation and Memory

| Directory | Purpose | Contents |
|-----------|---------|-----------|
| `memory/` | Project auto-memory | Persistent context across conversations |
| `notes/` | Project notes | Development notes and observations |
| `README.md` | Project overview | Main README |
| `LICENSE` | License file | SPDX license text |

### Configuration and State

| Directory/File | Purpose | Notes |
|---------------|---------|-------|
| `.claude/` | Claude Code configuration | Project-specific Claude settings |
| `.git/` | Git repository | Version control |
| `.gitattributes` | Git attributes | File handling configuration |
| `.gitignore` | Git exclusions | Build outputs, temp files |
| `.needle.yaml` | NEEDLE fleet configuration | Fleet worker configuration |
| `.needle-predispatch-sha` | NEEDLE state tracking | Pre-dispatch SHA tracking |

### Scripts and Utilities

| Directory | Purpose | Contents |
|-----------|---------|-----------|
| `scripts/` | Utility scripts | Development and deployment scripts |
| `.marathon/` | Marathon test configuration | Test framework configuration |

---

## Major Component Subdirectories

### mothership/ (Go Backend)

```
mothership/
├── cmd/                    # Go entrypoints
│   └── mothership/         # Main application
├── internal/               # Internal packages (45+ packages)
│   ├── analytics/          # Analytics and metrics
│   ├── apdetector/         # AP auto-detection for passive radar
│   ├── api/                # REST API handlers
│   ├── auth/               # Authentication (HMAC, sessions, PIN)
│   ├── automation/         # Spatial automation builder
│   ├── autoupdate/          # OTA auto-update manager
│   ├── ble/                # BLE scanning and identity matching
│   ├── briefing/           # Morning briefing generation
│   ├── config/             # Environment variable parsing
│   ├── dashboard/          # Dashboard embedding (go:embed)
│   ├── db/                 # SQLite schema and migrations
│   ├── diagnostics/        # System diagnostics
│   ├── doctor/             # Health checks and repairs
│   ├── eventbus/           # Event bus
│   ├── events/             # Event logging and timeline
│   ├── explainability/     # Detection explainability ("Why is this here?")
│   ├── falldetect/         # Fall detection algorithm
│   ├── fleet/              # Fleet manager and role assignment
│   ├── floorplan/          # Floor plan management
│   ├── fusion/             # Fusion loop orchestrator
│   ├── github/             # GitHub API client (releases)
│   ├── guidedtroubleshoot/ # Guided troubleshooting system
│   ├── health/             # Health endpoint
│   ├── help/               # Help system
│   ├── ingestion/          # WebSocket ingestion server
│   ├── learning/           # Machine learning (anomaly, prediction)
│   ├── loadshed/           # Load shedding under pressure
│   ├── localization/       # Localization algorithms
│   ├── localizer/          # Fresnel zone localization + UKF
│   ├── mqtt/               # MQTT client integration
│   ├── notifications/     # Notification delivery
│   ├── notify/             # Notification renderer (PNG generation)
│   ├── ntpserver/          # NTP server configuration
│   ├── ota/                # OTA update system
│   ├── oui/                # OUI lookup table (manufacturer detection)
│   ├── prediction/         # Presence prediction engine
│   ├── provisioning/       # Node provisioning (token derivation)
│   ├── recorder/           # CSI recording buffer
│   ├── recording/          # CSI replay store
│   ├── replay/             # Time-travel replay
│   ├── shutdown/           # Graceful shutdown
│   ├── signal/             # Signal processing pipeline
│   ├── simulator/          # CSI simulator support
│   ├── sleep/              # Sleep quality monitoring
│   ├── startup/            # Startup sequencing
│   ├── timeline/           # Activity timeline
│   ├── tracker/            # Blob tracking
│   ├── tracking/           # Crowd flow tracking
│   ├── volume/             # Trigger volumes (spatial automation)
│   └── webhook/            # Webhook delivery
├── build/                  # Build output (exclude from git)
├── test/                   # In-module acceptance tests
└── tests/                  # In-module integration tests
```

### firmware/ (ESP32-S3 Firmware)

```
firmware/
├── main/                   # Primary firmware component
│   ├── ble.c / ble.h       # BLE passive scanning
│   ├── csi.c / csi.h       # CSI capture and processing
│   ├── main.c              # Application entry point
│   ├── ntp.c / ntp.h       # NTP time synchronization
│   ├── nvs_migration.c/h   # NVS schema migration
│   ├── provision.c/h       # Serial provisioning
│   ├── spaxel.h            # Firmware header
│   ├── transport.c/h       # Transport layer
│   ├── version.h.in        # Version template
│   ├── websocket.c/h       # WebSocket client
│   └── wifi.c / wifi.h     # WiFi management
├── build/                  # ESP-IDF build output (exclude from git)
├── docs/                   # Firmware documentation
├── managed_components/     # ESP-IDF managed components
├── scripts/                # Firmware utility scripts
└── test/                   # Host-based firmware tests (gcc harness)
```

### dashboard/ (Frontend UI)

```
dashboard/
├── css/                    # Stylesheets
├── js/                     # Vanilla JavaScript modules
├── static/                 # Static assets (images, fonts, vendor libs)
│   ├── agentation.js       # UI feedback tool
│   ├── esptool-js/         # Web Serial provisioning library
│   └── three/              # Three.js 3D library
├── test-results/          # Test output (exclude from git)
├── tests/                 # Frontend tests (Playwright, Jest)
└── types/                 # TypeScript type definitions
```

### docs/ (Documentation)

```
docs/
├── build-paths-catalog.md              # Build paths reference
├── BUILD_PATHS.md                      # Build system documentation
├── ci-*.md                             # CI integration documentation
├── codebase-structure-and-test-patterns.md # Code structure overview
├── deployment/                          # Deployment guides
├── design/                              # Architecture Decision Records (ADRs)
├── examples/                            # Usage examples
├── gdop-*.md                            # GDOP computation documentation
├── inventory/                           # Repository inventories
├── notes/                               # Development notes
├── plan/                                # Implementation plan (plan.md)
├── research/                            # Third-party research and references
├── SYSTEM_CATALOG.md                    # System component catalog
├── tests/                               # Test documentation
├── trace-*.md                          # Tracing and profiling documentation
└── wifi-credential-provisioning-flow.md # WiFi provisioning flow
```

### test/ (Cross-Cutting Acceptance Tests)

```
test/
└── acceptance/            # Go module for acceptance tests
```

### tests/ (E2E Test Harness)

```
tests/
└── e2e/
    └── run.sh            # Shell-based end-to-end test harness
```

---

## Build Output and Scratch Directories (Excluded)

The following directories are build outputs or scratch space and should **not** be committed to git:

| Directory | Purpose | Exclude From Git |
|-----------|---------|------------------|
| `mothership/build/` | Go build artifacts | ✓ (in .gitignore) |
| `firmware/build/` | ESP-IDF build artifacts | ✓ (in .gitignore) |
| `dashboard/test-results/` | Frontend test output | ✓ (in .gitignore) |
| `~` | Temporary scratch directory | ✓ (in .gitignore) |

---

## Key Files

| File | Purpose |
|------|---------|
| `README.md` | Project overview and quickstart |
| `LICENSE` | SPDX license text |
| `VERSION` | Release version (single source of truth) |
| `go.work` | Go workspace stitching 3 modules |
| `go.work.sum` | Go workspace checksums |
| `.gitignore` | Git exclusion patterns |
| `.gitattributes` | Git file handling attributes |
| `.dockerignore` | Docker build exclusions |
| `.golangci.yml` | Go linting configuration |
| `.needle.yaml` | NEEDLE fleet worker configuration |
| `docker-compose.yml` | Production deployment manifest |
| `Dockerfile` | Container image build (multi-stage) |

---

## Module Structure

The repository uses **Go workspaces** to stitch together three separate Go modules:

1. **`mothership/`** — Go module (mothership/go.mod)
2. **`cmd/sim/`** — Go module (cmd/sim/go.mod) — CSI simulator CLI
3. **`test/acceptance/`** — Go module (test/acceptance/go.mod) — cross-cutting acceptance tests

See `go.work` for the workspace definition.

---

## Data Persistence

Runtime data is stored in `/data/` (configured via `SPAXEL_DATA_DIR`):

- `spaxel.db` — SQLite database (nodes, zones, events, baselines, etc.)
- `csi_replay.bin` — CSI replay buffer (append-only circular buffer)
- `floorplan/` — Floor plan images
- `firmware/` — Firmware binaries for OTA

---

## Summary Statistics

- **Top-level directories:** 19
- **Major component directories:** 5 (mothership, firmware, dashboard, cmd, docs)
- **Internal packages (mothership):** 45+
- **Go modules:** 3 (mothership, cmd/sim, test/acceptance)
- **Build systems:** Go (mothership), ESP-IDF (firmware), npm (dashboard tests)

---

## Related Documentation

- `README.md` — Project overview and quickstart
- `docs/plan/plan.md` — Complete implementation plan
- `docs/SYSTEM_CATALOG.md` — System component catalog
- `CODEBASE_STRUCTURE_AND_TEST_PATTERNS.md` — Code structure and testing patterns
- `go.work` — Go workspace definition
