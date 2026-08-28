# Protobuf Survey: Response Messages That Should Include free_heap_bytes

**Date:** 2026-08-28
**Task:** Survey all .proto files and identify response messages that should include `free_heap_bytes`

## Key Finding

**No .proto files exist in the spaxel codebase.** The project uses **JSON over WebSocket** for all node↔mothership communication, not Protocol Buffers.

## All Message Types in Spaxel

### Upstream Messages (Node → Mothership)
*Location: `/home/coding/spaxel/mothership/internal/ingestion/message.go`*

1. **`HelloMessage`** - Initial registration on WebSocket connect
2. **`HealthMessage`** - Periodic health status (every 10s)
3. **`BLEMessage`** - BLE scan results (every 5s)
4. **`MotionHintMessage`** - On-device variance exceeds threshold
5. **`OTAStatusMessage`** - OTA update progress

### Downstream Messages (Mothership → Node)
*Location: `/home/coding/spaxel/mothership/internal/ingestion/message.go`*

6. **`RoleMessage`** - Role assignment (tx/rx/tx_rx/passive/idle)
7. **`ConfigMessage`** - Configuration parameters
8. **`OTAMessage`** - OTA trigger
9. **`RebootMessage`** - Reboot command
10. **`IdentifyMessage`** - LED blink for identification
11. **`BaselineRequestMessage`** - Baseline data request
12. **`ShutdownMessage`** - Shutdown notification
13. **`RejectMessage`** - Connection rejection

### REST API Response Types
*Location: `/home/coding/spaxel/mothership/internal/fleet/`*

14. **`FleetNode`** (handler.go) - Node information for dashboard
15. **`NodeRecord`** (registry.go) - Internal node registry
16. **`fleetHealthResponse`** (fleethandler.go) - Fleet health status
17. **`optimiseResponse`** - Optimization results
18. **`simulateResponse`** - Simulation results
19. **`systemModeResponse`** - Security/mode status
20. **`autoAwayConfigResponse`** - Auto-away configuration

---

## Categorization: Which Messages Need `free_heap_bytes`?

### ✅ ALREADY HAVE IT (Correctly Implemented)

| Message | Location | Purpose |
|---------|-----------|---------|
| `HealthMessage` | message.go:41 | Periodic health monitoring (every 10s) |
| `FleetNode` | fleet/handler.go | Dashboard node display |
| `NodeRecord` | fleet/registry.go | Node registry storage |

### ✅ SHOULD ADD (High-Value Candidates)

| Message | Priority | Rationale |
|---------|----------|-----------|
| `HelloMessage` | **HIGH** | Initial connection - provides baseline heap assessment before node enters active operation |
| `OTAStatusMessage` | **HIGH** | Critical during OTA - heap exhaustion is a common OTA failure cause; monitoring helps diagnose download/write failures |

### ❌ DOES NOT NEED (Not Relevant)

- **`BLEMessage`** - Only reports discovered BLE devices; heap state not relevant
- **`MotionHintMessage`** - Simple event notification; heap state not relevant
- **All downstream messages** (Role, Config, OTA, Reboot, Identify, BaselineRequest, Shutdown, Reject) - These are commands TO the node, not responses FROM the node

### ❓ OPTIONAL (Could Add but Less Critical)

- **`fleetHealthResponse`** - Could include min/average heap across fleet for aggregate monitoring
- **`optimiseResponse`** - Could include heap impact of role changes

---

## Recommended Implementation

Add `free_heap_bytes` field to **2 messages**:

### 1. HelloMessage
```go
type HelloMessage struct {
    Type            string   `json:"type"`
    MAC             string   `json:"mac"`
    NodeID          string   `json:"node_id,omitempty"`
    FirmwareVersion string   `json:"firmware_version"`
    Capabilities    []string `json:"capabilities"`
    Chip            string   `json:"chip,omitempty"`
    FlashMB         int      `json:"flash_mb,omitempty"`
    UptimeMS        int64    `json:"uptime_ms,omitempty"`
    APBSSID         string   `json:"ap_bssid,omitempty"`
    APChannel       int      `json:"ap_channel,omitempty"`
    Token           string   `json:"token,omitempty"`
    FreeHeapBytes   int64    `json:"free_heap_bytes,omitempty"`  // ← ADD THIS
    PosX            *float64 `json:"pos_x,omitempty"`
    PosY            *float64 `json:"pos_y,omitempty"`
    PosZ            *float64 `json:"pos_z,omitempty"`
}
```

### 2. OTAStatusMessage
```go
type OTAStatusMessage struct {
    Type        string  `json:"type"`
    MAC         string  `json:"mac"`
    State       string  `json:"state"` // downloading | verifying | writing | rebooting | failed
    ProgressPct int     `json:"progress_pct,omitempty"`
    Error       string  `json:"error,omitempty"`
    FreeHeapBytes int64 `json:"free_heap_bytes,omitempty"`  // ← ADD THIS
}
```

---

## Benefits of This Addition

With these changes, `free_heap_bytes` will be visible at **three critical points**:

1. **Initial Connection** (`HelloMessage`) - Establish baseline heap state
2. **Periodic Health** (`HealthMessage`) - Monitor heap over time (already exists)
3. **Firmware Updates** (`OTAStatusMessage`) - Diagnose heap-related OTA failures

This provides complete heap visibility across the node lifecycle without adding unnecessary noise to event-type messages like `BLEMessage` or `MotionHintMessage`.

---

## Files That Would Need Updates

1. **Firmware:** `/home/coding/spaxel/firmware/main/websocket.c`
   - Add `free_heap_bytes` to `hello` message JSON construction
   - Add `free_heap_bytes` to `ota_status` message JSON construction

2. **Mothership:** `/home/coding/spaxel/mothership/internal/ingestion/message.go`
   - Add `FreeHeapBytes` field to `HelloMessage` struct
   - Add `FreeHeapBytes` field to `OTAStatusMessage` struct

3. **Database:** `/home/coding/spaxel/mothership/internal/db/migrations.go`
   - No migration needed - heap data flows through messages; not persisted separately

4. **Tests:** Update test fixtures in:
   - `/home/coding/spaxel/mothership/internal/ingestion/message_test.go`
   - `/home/coding/spaxel/mothership/tests/e2e/e2e_test.go`
   - `/home/coding/spaxel/mothership/cmd/sim/main.go` (simulator)

---

## Implementation Notes

- Use `int64` type for `free_heap_bytes` to match existing usage
- Field should be `omitempty` to maintain backward compatibility
- Value obtained via `esp_get_free_heap_size()` in firmware
- Sent in JSON as number (e.g., `204800` for 200 KB)
- Mothership stores this in `Node.FreeHeapBytes` (registry) for dashboard display
