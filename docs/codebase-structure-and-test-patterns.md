# Spaxel Codebase Structure and Test Patterns

**Generated:** 2026-08-29; refreshed 2026-09-04 (spaxel-7737eea8 — this is the surviving
copy; the duplicate root `CODEBASE_STRUCTURE_AND_TEST_PATTERNS.md` was deleted and its
module/entry-point notes folded in)  
**Purpose:** Comprehensive mapping of Spaxel codebase directory structure, programming languages, and test file conventions.

Directory-level detail with per-file annotations lives in
`docs/inventory/repository-directory-structure.md` and `docs/repo-structure.md`.

## Major Directories

### Application Code Directories

- **`mothership/`** — Go backend application (the only Go module; `go.work` uses just
  `./mothership`)
  - Entry point: `cmd/mothership/` (`main.go` — startup phases, subsystem wiring, routes)
  - Simulator CLI: `cmd/sim/` (`spaxel-sim`, shipped in the Docker image)
  - Internal packages: `internal/` (56 packages)
  - Build artifacts: `build/`

- **`firmware/`** — ESP32-S3 firmware (C-based, ESP-IDF)
  - Source code in `main/` (12 `.c` sources + headers)
  - Build system: CMakeLists.txt, partitions.csv, sdkconfig.defaults
  - Managed components: `managed_components/espressif__*/`

- **`dashboard/`** — Web frontend
  - Static assets: 9 HTML entry points, flat `js/`, flat `css/`
  - Embedded into Go binary via go:embed (`-tags=embed`)

- **`scripts/`** — Automation and provisioning scripts
  - Shell scripts (.sh): flash, run-sim-*, verify
  - Python provisioning tools (.py)

### Test-Related Directories

- **`mothership/test/`** — Mothership-specific tests
  - WiFi credential tests
  - Integration tests

- **`mothership/test/acceptance/`** — Mothership acceptance tests
  - Acceptance scenarios AS-1…AS-7 (`as*_test.go` pattern) plus a WiFi restart race scenario
  - IO-style installation/upgrade tests (`io_install_upgrade_test.go`, `integration_test.go`)

- **`mothership/tests/e2e/`** — Mothership end-to-end tests (Go)
  - `e2e_test.go`, `assertions_test.go`, IO-6 gate tests

- **`testdata/`** — CSI-recording utilities (`//go:build ignore`, in no module)
  - Recording generators/verifiers

- **`firmware/test/`** — C-based firmware tests
  - Host-based gcc harness (not ESP-IDF host test); 9 `test_*.c` files + Makefile
  - Unit tests for NVS, CSI, provisioning, console config, restart races

- **`dashboard/tests/`** — Dashboard accessibility tests
  - Playwright + axe-core integration
  - Unit tests are co-located in `dashboard/js/*.test.js` (jest) instead

### Data and Runtime Directories

- **`data/`** — Runtime data (created at runtime)
  - `backups/` — Database backups
  - `floorplan/` — Floor plan images
  - `firmware/` — Firmware binaries for OTA
  - `csi/` — CSI replay buffer
  - `simulator/` — Simulator state

- **`.beads/`** — Bead tracking state

### Documentation Directories

- **`docs/`** — Project documentation
  - `plan/` — Implementation plan
  - `design/` — Design documents
  - `deployment/` — Deployment guides
  - `examples/` — Example configurations
  - `tests/` — Test documentation
  - `research/` — Research findings

- **`notes/`** — Development notes
- **`memory/`** — System memory and config notes

### Configuration and Build

- **`.claude/`** — Claude Code configuration
- **`.marathon/`** — Marathon instruction system
- **`.git/`** — Git repository

## Module Structure and Entry Points

The repository has **one Go module** (`mothership/go.mod`, go 1.25.0), stitched into a
workspace by the root `go.work` (`use ./mothership`). There is no root `go.mod` and no
separate simulator/acceptance module — run all `go` commands from `mothership/`.

| Entry point | Path |
|---|---|
| Mothership binary | `mothership/cmd/mothership/main.go` |
| Simulator CLI (`spaxel-sim`) | `mothership/cmd/sim/main.go` |
| Firmware | `firmware/main/main.c` (`app_main`) |
| Dashboard | static files under `dashboard/`, embedded via `//go:embed` |

Key architectural patterns: pure-Go dependencies only (`modernc.org/sqlite`, no CGO),
table-driven tests alongside implementation, and a single wiring point
(`mothership/cmd/mothership/main.go`) where new subsystems are constructed.

## Programming Languages

### Primary Languages

- **Go** — Mothership backend (378 files total, 173 test files)
- **C** — ESP32-S3 firmware
- **JavaScript** — Dashboard frontend
- **TypeScript** — Type definitions and build tooling

### Supporting Languages

- **Python** — Provisioning scripts, automation
- **Shell** — Build, flash, and test automation scripts

### Configuration and Data

- **JSON** — Configuration files, test data
- **YAML** — Build and deployment manifests
- **Markdown** — Documentation

## Test File Patterns

### Go Test Patterns (`*_test.go`)

**Standard convention:**
- `*_test.go` — 173 total Go test files

**Acceptance test pattern:**
- `as*_test.go` — Acceptance scenarios (e.g., `as1_first_time_setup_test.go`,
  `as2_walking_detection_test.go`, `as7_auth_reject_test.go`)

**Integration test pattern:**
- `integration_test.go` — General integration tests
- `*_integration_test.go` — Component-specific integration tests

**WiFi credential test patterns:**
- `wifi_credential_*_test.go` — Categorized by type:
  - `wifi_credential_flow_test.go`
  - `wifi_credential_db_test.go`
  - `wifi_credential_env_test.go`
  - `wifi_credential_e2e_test.go`
  - `wifi_credential_edge_cases_test.go`
  - `wifi_credential_missing_env_vars_test.go`

**IO-style installation/upgrade tests:**
- `io_install_upgrade_test.go`

### JavaScript/TypeScript Test Patterns

**Unit tests:**
- `*.test.js` — JavaScript unit tests
- `*.test.ts` — TypeScript unit tests

**Spec tests:**
- `*.spec.js` — JavaScript specification tests
- `*.spec.ts` — TypeScript specification tests

**Examples:**
- `a11y.spec.js` — Accessibility tests
- `blob-identity.test.js` — Blob identity tests
- `onboard.leak-detection.test.js` — Onboarding leak detection

### C Test Patterns (Firmware)

**Host-based tests (gcc harness):**
- `test_*.c` — C test files in `firmware/test/`
- Examples:
  - `test_runner.c` — Test runner
  - `test_wifi_restart_race.c` — WiFi race condition test
  - `test_ota_during_wifi_reconnect.c` — OTA reconnection test

**Test structure:**
- Makefile-based test execution
- Host-based only (no ESP-IDF `idf.py test --target linux`)
- Tests logic and binary format contracts independently

## Test Coverage Statistics

### By Language

- **Go:** 378 total files, 173 test files (46% test coverage by file count)
- **C (firmware):** host-based tests in `firmware/test/` (9 files)
- **JavaScript/TypeScript:** Multiple `*.test.js` and `*.spec.js` files

### By Component

**Mothership:**
- Internal packages: 56 packages
- Internal Go files: 332
- Internal test files: 142 (43% test coverage in internal packages)

**Firmware:**
- Host-based unit tests in `firmware/test/` (9 files)
- Covers NVS, CSI, provisioning logic independently

**Dashboard:**
- Accessibility tests via axe-core (Playwright, `dashboard/tests/`)
- Component unit tests co-located in `dashboard/js/*.test.js` (jest)

## Key Test Directories Summary

```
mothership/test/             # Mothership-specific tests
├── wifi_credential_*_test.go  # Categorized WiFi tests
└── [integration tests]

mothership/test/acceptance/  # Acceptance scenarios + IO install/upgrade
├── as1_first_time_setup_test.go … as7_auth_reject_test.go
├── as5_wifi_restart_race_test.go
├── integration_test.go
├── io_install_upgrade_test.go
└── test_helpers.go

mothership/tests/e2e/        # End-to-end Go tests
├── e2e_test.go
├── assertions_test.go
├── io6_gate_test.go
└── io6_gate_conclusion_test.go

testdata/                    # CSI-recording utilities (//go:build ignore)

firmware/test/               # C-based firmware tests
├── test_*.c                 # 9 test files
└── Makefile                 # Test build/run

dashboard/tests/             # Playwright accessibility specs
dashboard/js/*.test.js       # Co-located jest unit tests
```

## Test Execution

### Go Tests
```bash
cd mothership && go test ./...       # Run all Go tests
cd mothership && go vet ./...         # Run Go vet
```

### Firmware Tests
```bash
make -C firmware/test test           # Run C-based host tests
```

### Acceptance/E2E Tests
```bash
cd mothership && go test ./test/acceptance/ ./tests/e2e/
# Both trees live inside the mothership module; the acceptance tests drive
# built `spaxel` + `spaxel-sim` binaries.
```

---

**Notes:**
- Test files follow standard Go convention (`*_test.go`)
- Acceptance tests use `as*_test.go` pattern for scenario-based testing
- Firmware tests are host-based gcc harness, not ESP-IDF host tests
- Dashboard includes axe-core accessibility testing
- Total test coverage: 46% of Go files are test files
- There is no shell e2e harness at the repo root — `tests/e2e/run.sh` does not exist;
  the e2e coverage is the Go suite in `mothership/tests/e2e/`
