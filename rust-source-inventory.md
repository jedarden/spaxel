# Rust Source File Inventory - Spaxel

**Generated:** 2026-08-29
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
| **CSI Simulator** | Go | `cmd/sim/` |
| **Persistence** | SQLite (pure Go) | `modernc.org/sqlite` (no CGO) |
| **Testing** | Go (acceptance/integration) | `test/acceptance/`, `mothership/test/acceptance/` |

## Source File Distribution

### Go Source Files (.go)
Located in:
- `mothership/cmd/mothership/` - Main entrypoint
- `mothership/internal/` - Internal packages (ingestion, pipeline, localizer, fleet, ble, etc.)
- `cmd/sim/` - CSI simulator CLI
- `test/acceptance/` - Cross-cutting acceptance tests
- `mothership/test/acceptance/` - In-module acceptance tests
- `testdata/` - Test data generation scripts
- `docs/examples/` - Example code

### C Source Files (.c, .h)
Located in:
- `firmware/main/` - ESP32-S3 firmware source
  - `main.c`, `wifi.c`, `csi.c`, `ws.c`, `ble.c`, `ota.c`, `nvs.c`, `serial_prov.c`, `sntp.c`, `led.c`

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
