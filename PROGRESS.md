# Spaxel Implementation Progress

## Phase 1 — Foundation

Goal: Bare-minimum loop from ESP32 to browser. Zero-config with passive radar and mDNS from day one.

### Status

| Item | Status | Notes |
|------|--------|-------|
| ESP32 firmware skeleton | Not started | |
| Passive radar support | Not started | (part of firmware) |
| BLE scanning | Not started | (part of firmware) |
| Mothership WebSocket ingestion | **Done** | See iteration 1 below |
| Dashboard skeleton | Not started | |
| Docker packaging | Not started | |

### Iteration Log

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

**Remaining for Phase 1:**
- ESP32 firmware (WiFi, mDNS discovery, CSI capture, WebSocket client)
- Dashboard skeleton (HTML/JS + Three.js)
- Docker packaging
