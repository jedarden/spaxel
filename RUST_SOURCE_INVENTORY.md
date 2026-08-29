# Rust Source File Inventory - Spaxel

**Date:** 2026-08-29  
**Repository:** /home/coding/spaxel  
**Result:** **NO RUST SOURCE FILES FOUND**

## Summary

The spaxel codebase contains **0 Rust (.rs) source files**.

This project does not use Rust as a programming language. The technology stack consists of:

- **Go** — Mothership backend (mothership/)
- **C** — ESP32-S3 firmware (firmware/)
- **JavaScript** — Dashboard frontend (dashboard/)
- **Shell** — Build/deployment scripts

## Technology Breakdown

### Go Backend (Mothership)
Location: `/home/coding/spaxel/mothership/`

The mothership is a Go-based backend that:
- Ingests CSI data via WebSocket
- Runs signal processing pipeline
- Performs localization and fusion
- Manages ESP32 node fleet
- Serves dashboard web interface
- Provides REST API

### C Firmware (ESP32-S3)
Location: `/home/coding/spaxel/firmware/`

ESP-IDF project written in C that:
- Runs on ESP32-S3 microcontrollers
- Captures WiFi CSI data
- Streams data to mothership
- Supports OTA updates
- Performs BLE scanning

### JavaScript Dashboard
Location: `/home/coding/spaxel/dashboard/`

Vanilla JavaScript + Three.js frontend:
- 3D spatial visualization
- Real-time blob tracking
- Node management interface
- Floor plan editor

## Verification

Searched entire `/home/coding/spaxel` directory tree:
```bash
find /home/coding/spaxel -name "*.rs" -type f
```

**Result:** 0 files found

## Conclusion

This is a **Go + C + JavaScript** project. No Rust development is present or planned for the spaxel codebase according to the current architecture documentation in `docs/plan/plan.md`.

If you are looking for specific language components:
- **Go code:** See `mothership/` directory
- **C firmware:** See `firmware/` directory  
- **JavaScript:** See `dashboard/` directory
- **Tests:** Go tests in `mothership/`, shell tests in `tests/`
