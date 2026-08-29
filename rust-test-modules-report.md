# Rust Test Modules Inventory - Spaxel

**Generated:** 2026-08-29
**Task:** Identify all Rust-specific test directories and test modules within the spaxel source tree

## Summary

**Total Rust test modules found: 0**

The spaxel codebase does not contain any Rust source code or test modules. This is by design.

## Technology Stack (Non-Rust)

| Component | Language | Primary Location | Test Pattern |
|-----------|----------|-----------------|--------------|
| **Mothership Backend** | Go | `mothership/cmd/mothership/`, `mothership/internal/` | `*_test.go` |
| **ESP32-S3 Firmware** | ESP-IDF C | `firmware/main/` | `test_*.c` (gcc harness) |
| **Dashboard** | Vanilla JS + Three.js | `dashboard/` | `*.test.js` |
| **CSI Simulator** | Go | `cmd/sim/` | `*_test.go` |
| **Acceptance Tests** | Go | `test/acceptance/`, `mothership/test/acceptance/` | `as*_test.go` |

## Actual Test Modules by Language

### Go Tests (134+ files)
- **Unit tests:** `mothership/internal/*/` - Table-driven tests alongside implementation
- **Integration tests:** `test/acceptance/integration_test.go`
- **Acceptance tests:** `test/acceptance/as1_setup_test.go` through `as7_*`
- **E2E tests:** `mothership/tests/e2e/e2e_test.go`

### C Firmware Tests (10 files)
- **Location:** `firmware/test/`
- **Files:**
  - `test_csi_frame.c`
  - `test_nvs_migration.c`
  - `test_serial_prov.c`
  - `test_sanity.c`
  - `test_all_restart_trigger_points.c`
  - `test_ota_during_wifi_reconnect.c`
  - `test_wifi_restart_race.c`
  - `test_console_config.c`
  - `test_runner.c` (harness)
- **Execution:** Host-based gcc harness (`make -C firmware/test test`)

### JavaScript Tests (20 files)
- **Location:** `dashboard/js/`
- **Files:**
  - `ambient.test.js`
  - `blob-identity.test.js`
  - `backward-compat.test.js`
  - (plus 17 others)

## Rationale for No Rust

From `docs/plan/plan.md` and the project architecture:

1. **Go backend** chosen for low-latency ingestion, single binary deployment, and Docker packaging
2. **Pure Go SQLite** (`modernc.org/sqlite`) used instead of `mattn/go-sqlite3` to avoid CGO dependency
3. **ESP-IDF C required** for ESP32-S3 CSI hardware access (no Rust CSI API available)
4. **Vanilla JavaScript** chosen for dashboard to avoid build toolchain complexity

## Conclusion

No Rust test modules exist in spaxel because:
- **No Rust source code exists** in the project
- The technology stack is **Go (backend) + C (firmware) + JavaScript (frontend)**
- Test organization follows the language of each component (Go tests, C tests, JavaScript tests)
