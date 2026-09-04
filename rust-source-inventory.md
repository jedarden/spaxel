# Rust Source File Inventory - Spaxel

**Generated:** 2026-08-29; path references corrected 2026-09-04 (spaxel-7737eea8 — this
is the surviving copy; the duplicate `RUST_SOURCE_INVENTORY.md` was deleted)
**Scope:** Complete inventory of all `.rs` (Rust) source files in the codebase.

## Summary

**Total Rust files found: 0**

The spaxel codebase does not contain any Rust source files. This is by design, as the project uses the following technology stack:

## Technology Stack

| Component | Language/Technology | Location |
|-----------|---------------------|----------|
| **Mothership Backend** | Go | `mothership/` (cmd/mothership/, internal/) |
| **ESP32-S3 Firmware** | ESP-IDF C | `firmware/` |
| **Dashboard** | Vanilla JS + Three.js | `dashboard/` |
| **CSI Simulator** | Go | `mothership/cmd/sim/` |
| **Persistence** | SQLite (pure Go) | `modernc.org/sqlite` (no CGO) |
| **Testing** | Go (acceptance/integration) | `mothership/test/acceptance/`, `mothership/tests/e2e/` |

## Source File Distribution

### Go Source Files (.go)
Located in:
- `mothership/cmd/mothership/` - Main entrypoint
- `mothership/internal/` - Internal packages (ingestion, signal, localizer, fleet, ble, etc.)
- `mothership/cmd/sim/` - CSI simulator CLI
- `mothership/test/acceptance/` - Acceptance scenarios (AS-1…AS-7) + IO install/upgrade tests
- `mothership/tests/e2e/` - End-to-end Go tests
- `testdata/` - Test data generation scripts
- `docs/examples/` - Example code

### C Source Files (.c, .h)
Located in:
- `firmware/main/` - ESP32-S3 firmware source
  - `main.c`, `wifi.c`, `csi.c`, `websocket.c`, `transport.c`, `ble.c`, `led.c`,
    `ntp.c`, `nvs_migration.c`, `provision.c`, `safe_mode.c`, `watchdog.c`
- `firmware/test/` - Host-based gcc test harness (`test_*.c`, 9 files)

### JavaScript/HTML Files
Located in:
- `dashboard/` - Web UI assets (HTML, JS, CSS)

## Architecture Notes

From `docs/plan/plan.md`:

> **Technology Choices:**
> - Mothership backend: Go (Low-latency ingestion, single binary, easy Docker packaging)
> - Dashboard frontend: Vanilla JS + Three.js (No build toolchain; Three.js provides hardware-accelerated 3D)
> - ESP32 firmware: ESP-IDF (C) (Full CSI API access, OTA support, NVS for config)
> - Persistence: SQLite (`modernc.org/sqlite`, pure Go - no CGO)

## Rationale for No Rust

The plan document specifies:
- No CGO requirement (`modernc.org/sqlite` used instead of `mattn/go-sqlite3`)
- Pure Go SQLite for compatibility
- Go backend chosen for single binary deployment and Docker packaging
- ESP-IDF C required for ESP32-S3 CSI hardware access

Rust is not part of the current technology stack for Spaxel.
