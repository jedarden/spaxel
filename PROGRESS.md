# Spaxel Implementation Progress

## Phase 1 — Foundation

Goal: Bare-minimum loop from ESP32 to browser. Zero-config with passive radar and mDNS from day one.

### Status

| Item | Status | Notes |
|------|--------|-------|
| ESP32 firmware skeleton | **Done** | See iteration 2 below |
| Passive radar support | **Done** | BSSID filter in csi.c |
| BLE scanning | **Done** | Core 0 concurrent with WiFi |
| Mothership WebSocket ingestion | **Done** | See iteration 1 below |
| Dashboard skeleton | **Done** | See iteration 3 below |
| Docker packaging | **Done** | See iteration 4 below |

### Iteration Log

#### Iteration 4 — 2026-03-26

**Completed:** Docker packaging

Implemented container deployment with:

- **Dockerfile:** Multi-stage build for minimal production image
  - Stage 1: `golang:1.23-bookworm` builder with module caching
  - Stage 2: `distroless/static-debian12:nonroot` runtime (no shell, runs as UID 65532)
  - CGO_ENABLED=0 build (pure-Go SQLite compatible)
  - Dashboard static files copied to `/dashboard/`
  - Firmware directory as volume mount point (`/firmware/`)
  - Built-in healthcheck via wget

- **docker-compose.yml:** Production-ready orchestration
  - `network_mode: host` for mDNS multicast discovery (required for ESP32 nodes)
  - Volume mounts: `spaxel-data` for persistence, `./firmware` for OTA binaries
  - Environment variables: TZ, mDNS config, optional MQTT
  - Resource limits: 512m RAM, 2 CPUs (scalable to 1g for 16+ nodes)
  - 35s stop_grace_period for graceful shutdown
  - Ulimits: 4096/8192 file descriptors for node connections
  - Traefik labels (disabled by default, enable for TLS/proxy)

- **Supporting files:**
  - `VERSION` — 0.1.0 for image tagging
  - `.dockerignore` — Excludes docs, build artifacts, IDE files

**Key design decisions:**
- Host networking is REQUIRED for mDNS — Docker bridge blocks multicast 224.0.0.251
- distroless image minimizes attack surface (no shell, no package manager)
- Firmware binaries not bundled — users mount their own `/firmware` volume
- Traefik labels included but disabled — enable for production TLS

**Files created:**
```
Dockerfile
docker-compose.yml
VERSION
.dockerignore
```

**Phase 1 Status:** COMPLETE

All Phase 1 items implemented:
- ✅ ESP32 firmware skeleton
- ✅ Passive radar support
- ✅ BLE scanning
- ✅ Mothership WebSocket ingestion
- ✅ Dashboard skeleton
- ✅ Docker packaging

**Next:** Phase 2 — Signal Processing (baseline, deltaRMS, Fresnel zones)

#### Iteration 3 — 2026-03-26

**Completed:** Dashboard skeleton with 3D visualization

Implemented the web dashboard in vanilla JS + Three.js with:

- **Static file structure:** `dashboard/` directory with HTML/JS
  - `index.html` — Dark theme UI with status bar, node/link panels, amplitude chart
  - `js/app.js` — Main application (~350 lines)

- **3D Scene (Three.js):**
  - Ground grid (10×10m, 20 divisions)
  - OrbitControls for pan/zoom/rotate with damping
  - Axes helper for orientation
  - Responsive canvas with devicePixelRatio support
  - FPS counter in status bar

- **WebSocket connection (`/ws/dashboard`):**
  - Auto-reconnect with 3s backoff
  - JSON message handling for state, node events, link events
  - Binary CSI frame parsing (24-byte header + I/Q payload)
  - Connection status indicator (green/red dot)

- **Node/Link panels:**
  - Live list of connected nodes with MAC, firmware, chip
  - Active links with node:peer MAC pairs
  - Click-to-select for amplitude chart display

- **Amplitude chart (Canvas 2D):**
  - 64-bar visualization of subcarrier amplitudes
  - Real-time update from selected link's CSI frames
  - Color gradient based on amplitude intensity
  - Channel and RSSI display overlay

- **Dashboard package (Go):**
  - `internal/dashboard/hub.go` — Hub for client management and broadcasting
  - `internal/dashboard/server.go` — WebSocket handler with ping/pong keepalive
  - `internal/dashboard/hub_test.go` — 5 tests for hub operations
  - IngestionState interface for querying node/link state
  - CSIBroadcaster interface implementation

- **Main.go updates:**
  - Static file serving with SPA fallback
  - Dashboard WebSocket endpoint at `/ws/dashboard`
  - Auto-discovery of dashboard directory
  - Wiring between ingestion and dashboard packages

**Dependencies used (frontend):**
- Three.js r128 (CDN)
- OrbitControls (CDN)

**Tests:** 5 new tests for dashboard hub. All 27 tests pass.

**Files created:**
```
dashboard/
├── index.html
└── js/
    └── app.js

mothership/internal/dashboard/
├── hub.go
├── hub_test.go
└── server.go
```

**Remaining for Phase 1:**
- Docker packaging

#### Iteration 2 — 2026-03-26

**Completed:** ESP32-S3 firmware skeleton

Implemented the full ESP32 firmware in ESP-IDF C with:

- **Project structure:** Standard ESP-IDF layout with `firmware/` root
  - Top-level `CMakeLists.txt`, `sdkconfig.defaults`, `partitions.csv` (factory + OTA slots)
  - `main/` component with 5 source modules

- **State machine (main.c):** 7-state node lifecycle
  - BOOT → WIFI_CONNECTING → MOTHERSHIP_DISCOVERY → CONNECTED
  - Degraded states: WIFI_LOST, MOTHERSHIP_UNAVAILABLE, CAPTIVE_PORTAL
  - Exponential backoff on WiFi failures (1s → 30s max)
  - 10-failure threshold before captive portal

- **WiFi module (wifi.c/h):**
  - STA connection with exponential backoff
  - mDNS discovery for `_spaxel._tcp.local` with fallback to cached IP
  - Captive portal AP mode (`spaxel-XXXX`) with HTTP config page
  - Event-driven connection state via FreeRTOS event group

- **WebSocket client (websocket.c/h):**
  - Bidirectional communication on single connection
  - Binary CSI frame transmission (24-byte header + I/Q payload)
  - JSON message handling: hello, health, ble, ota_status upstream
  - Downstream command parsing: role, config, ota, reboot, identify, reject
  - OTA download task with progress reporting and automatic reboot

- **CSI capture (csi.c/h):**
  - WiFi promiscuous mode with CSI callback
  - Queue-based processing (32-frame buffer)
  - Passive mode BSSID filtering for radar
  - On-device amplitude variance tracking for motion hints (Welford's algorithm)
  - TX task for active probing

- **BLE scanner (ble.c/h):**
  - Passive BLE scanning on Core 0 (concurrent with WiFi)
  - Device cache (60 entries) with name and manufacturer data parsing
  - 5-second reporting interval via WebSocket
  - GAP event handling for advertisement processing

- **NVS persistence:** Full schema with 15 keys
  - WiFi credentials, node ID/token, mothership config
  - Role/rate persistence for degraded mode operation
  - Schema versioning for migration support

**Files created:**
```
firmware/
├── CMakeLists.txt
├── sdkconfig.defaults
├── partitions.csv
└── main/
    ├── CMakeLists.txt
    ├── spaxel.h
    ├── main.c
    ├── wifi.h / wifi.c
    ├── websocket.h / websocket.c
    ├── csi.h / csi.c
    └── ble.h / ble.c
```

**Remaining for Phase 1:**
- Dashboard skeleton (HTML/JS + Three.js)
- Docker packaging

#### Iteration 1 — 2026-03-26

**Completed:** Mothership WebSocket ingestion server

Implemented the core ingestion server in Go with:

- **Module structure:** `mothership/` with `cmd/mothership/` entrypoint and `internal/ingestion/` package
- **WebSocket endpoint:** `/ws/node` accepts bidirectional connections from ESP32 nodes
- **Binary frame parsing:** 24-byte header + variable payload, per spec in plan.md
  - Validation: min/max length, payload size match, channel validity (1-14), subcarrier limit (128)
  - Malformed frame tracking with warn/close thresholds (100/1000 per minute)
- **JSON message handling:** Parses hello, health, ble, motion_hint, ota_status
- **Per-link ring buffers:** 256-sample circular buffers keyed by `nodeMAC:peerMAC`
- **Connection lifecycle:** Node registration via hello, ping/pong keepalive (30s/60s), graceful shutdown
- **mDNS advertisement:** `_spaxel._tcp.local` via github.com/hashicorp/mdns
- **Role/config push:** Sends initial `rx` role and 20 Hz config on connect
- **Health endpoint:** `GET /healthz` returns `{"status":"ok","version":"..."}`

**Dependencies used:**
- `github.com/go-chi/chi` — HTTP routing
- `github.com/gorilla/websocket` — WebSocket server
- `github.com/hashicorp/mdns` — mDNS advertisement

**Tests:** 22 tests covering frame parsing, JSON messages, and ring buffer operations. All pass.

**Files created:**
```
mothership/
├── cmd/mothership/main.go
├── go.mod
├── go.sum
└── internal/ingestion/
    ├── frame.go
    ├── frame_test.go
    ├── message.go
    ├── message_test.go
    ├── ring.go
    ├── ring_test.go
    └── server.go
```
