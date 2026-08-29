# Spaxel Codebase Structure and Test Patterns

**Generated:** 2026-08-29  
**Purpose:** Comprehensive mapping of Spaxel codebase directory structure, programming languages, and test file conventions.

## Major Directories

### Application Code Directories

- **`mothership/`** — Go backend application
  - Entry point: `cmd/mothership/`
  - Internal packages: `internal/` (53 packages)
  - Build artifacts: `build/`

- **`firmware/`** — ESP32-S3 firmware (C-based, ESP-IDF)
  - Source code in `main/` and `components/`
  - Build system: CMakeLists.txt, partitions.csv, sdkconfig
  - Managed components: `managed_components/espressif__*/`

- **`dashboard/`** — Web frontend
  - Static assets: HTML, JavaScript, CSS
  - Embedded into Go binary via go:embed

- **`cmd/`** — Additional Go commands
  - `sim/` — CSI simulator CLI (`spaxel-sim`)

- **`scripts/`** — Automation and provisioning scripts
  - Shell scripts (.sh)
  - Python provisioning tools (.py)

### Test-Related Directories

- **`test/`** — Main test directory
  - `acceptance/` — Acceptance tests (`as*_test.go` pattern)

- **`tests/`** — Alternative test directory  
  - `e2e/` — End-to-end test harness (`run.sh`)

- **`testdata/`** — Test data storage
  - CSI recordings
  - Test generators and fixtures

- **`mothership/test/`** — Mothership-specific tests
  - WiFi credential tests
  - Integration tests

- **`mothership/test/acceptance/`** — Mothership acceptance tests
  - IO-style installation/upgrade tests

- **`mothership/tests/`** — Additional mothership test suites
  - E2E tests

- **`firmware/test/`** — C-based firmware tests
  - Host-based gcc harness (not ESP-IDF host test)
  - Unit tests for NVS, CSI, provisioning

- **`dashboard/tests/`** — Dashboard accessibility tests
  - axe-core integration

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

## Programming Languages

### Primary Languages

- **Go** — Mothership backend (372 files total, 172 test files)
- **C** — ESP32-S3 firmware (94 files)
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
- `*_test.go` — 172 total Go test files

**Acceptance test pattern:**
- `as*_test.go` — Acceptance scenarios (e.g., `as1_setup_test.go`, `as2_walking_test.go`)

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

- **Go:** 372 total files, 172 test files (46% test coverage by file count)
- **C (firmware):** 94 files, host-based tests in `firmware/test/`
- **JavaScript/TypeScript:** Multiple `*.test.js` and `*.spec.js` files

### By Component

**Mothership:**
- Internal packages: 53 packages
- Internal Go files: 316
- Internal test files: 134 (42% test coverage in internal packages)

**Firmware:**
- Host-based unit tests in `firmware/test/`
- Covers NVS, CSI, provisioning logic independently

**Dashboard:**
- Accessibility tests via axe-core
- Component unit tests

## Key Test Directories Summary

```
test/
├── acceptance/           # Go acceptance tests (as*_test.go)
├── acceptance_test.go    # General acceptance test
├── integration_test.go   # Integration test
└── [scenario tests]      # AS1-AS7 scenario tests

tests/
├── e2e/                  # End-to-end test harness
│   └── run.sh           # Shell-based E2E tests

testdata/                  # Test data and fixtures
├── [CSI recordings]
└── [test generators]

mothership/test/          # Mothership-specific tests
├── wifi_credential_*_test.go  # Categorized WiFi tests
└── [integration tests]

mothership/test/acceptance/  # Mothership acceptance tests
└── io_*_test.go              # IO-style tests

mothership/tests/        # Additional mothership tests
└── e2e/                  # E2E tests

firmware/test/           # C-based firmware tests
├── test_*.c            # C test files
└── Makefile            # Test build/run

dashboard/tests/         # Dashboard tests
└── [a11y tests]        # Accessibility tests
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

### Integration/Acceptance Tests
```bash
./tests/e2e/run.sh                    # Run E2E test harness
```

---

**Notes:**
- Test files follow standard Go convention (`*_test.go`)
- Acceptance tests use `as*_test.go` pattern for scenario-based testing
- Firmware tests are host-based gcc harness, not ESP-IDF host tests
- Dashboard includes axe-core accessibility testing
- Total test coverage: 46% of Go files are test files
