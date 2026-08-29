# CSI Recording Module Structure

**Created:** 2026-08-28  
**Purpose:** Document the CSI recording module files, organization, and component boundaries in the Spaxel codebase.

## Overview

The CSI recording module is responsible for capturing, storing, and replaying raw CSI frames from ESP32-S3 nodes. It enables time-travel debugging (Phase 8 feature) by maintaining a circular buffer of CSI data that can be replayed with adjustable algorithm parameters.

## Module Organization

The CSI recording functionality is organized across four main directories under `mothership/internal/`:

### 1. Core Recording (`recording/`)

**Purpose:** Disk-backed circular buffer for CSI frame storage

**Files:**
- **buffer.go** (24,690 bytes)  
  Main circular buffer implementation with file header, record format, and retention logic
  
  Key features:
  - File magic: `"SPAXLREC"` (8 bytes)
  - Header layout: writePos, oldestPos, wrapPos (8 bytes each, little-endian)
  - Record format: `[recvTimeNS(8)][frameLen(2)][frameData(N)]`
  - Configurable retention via `SPAXEL_RECORDING_RETENTION` env var (default: 48h)
  - Default max size: 512 MB via `SPAXEL_REPLAY_MAX_MB`
  - Automatic eviction of oldest records when size/time limits exceeded

- **compression.go** (6,819 bytes)  
  Compression support for reducing CSI storage footprint

- **benchmark.go** (14,042 bytes)  
  Performance benchmarking utilities for recording subsystem

- **buffer_test.go** (10,000 bytes)  
  Table-driven tests for buffer operations

### 2. Link Recording Manager (`recorder/`)

**Purpose:** Per-link CSI frame recording with segmented hourly files

**Files:**
- **manager.go** (8,376 bytes)  
  Top-level manager for per-link recorders
  
  Responsibilities:
  - Manages map of `linkRecorder` instances (one per active link)
  - Configurable retention (default: 48 hours) and max bytes per link (default: 1 GB)
  - Background cleanup goroutine with hourly sweep interval
  - Pause/resume control for load shedding

- **segment.go** (5,048 bytes)  
  Hourly segment file management
  
  File layout:
  - Directory: `data/{nodeMAC}-{peerMAC}/`
  - Filename: `YYYYMMDD-HH.csi` (UTC)
  - Record format: `[4-byte BE uint32: payloadLen][8-byte BE int64: recvTimeNS][raw CSI frame]`
  - Records appended chronologically, rotated hourly

- **manager_test.go** (12,798 bytes)  
  Integration tests for recorder lifecycle

- **segment_test.go** (8,447 bytes)  
  Tests for segment file operations

### 3. Replay Engine (`replay/`)

**Purpose:** Time-travel debugging system for historical CSI data

**Files:**
- **engine.go** (6,327 bytes)  
  Core replay engine managing sessions and coordinating with recording buffer
  
  Key capabilities:
  - Session management with unique session IDs
  - Timestamp range clamping to available data
  - Default tunable parameters (deltaRMS threshold, tau, Fresnel decay, etc.)
  - Integration with blob broadcaster for dashboard updates

- **store.go** (9,608 bytes)  
  Replay store interface for accessing CSI data

- **pipeline.go** (5,242 bytes)  
  Replay pipeline processing (separate from live signal processing pipeline)

- **worker.go** (6,591 bytes)  
  Background worker goroutine for replay operations

- **session.go** (2,300 bytes)  
  Session state management

- **types.go** (7,520 bytes)  
  Type definitions for replay system

- **buffer_adapter.go** (1,954 bytes)  
  Adapter integrating recording buffer with replay system

- **integration_test.go** (20,858 bytes)  
  End-to-end tests for replay functionality

- **seek_fuzz_test.go** (18,546 bytes)  
  Fuzz testing for timestamp seeking operations

### 4. CSI Ingestion (`ingestion/`)

**Purpose:** WebSocket server receiving CSI frames from ESP32-S3 nodes

**Key files:**
- **server.go** (38,663 bytes)  
  WebSocket endpoint `/ws/node` handling binary CSI frames and JSON messages
  
  Interfaces:
  - `CSIBroadcaster` - broadcasts CSI frames to dashboard
  - `FleetNotifier` - notifies fleet manager of node lifecycle
  - `OTAStatusHandler` - receives OTA status updates
  - `BLERelayHandler` - receives BLE scan results

- **frame.go** (4,260 bytes)  
  Binary CSI frame parsing and validation
  
  Validation rules:
  - Frame length: 24-280 bytes (header + n_sub×2 payload)
  - Channel validation: 1-14 (2.4 GHz)
  - RSSI normalization: 0 = invalid
  - Malformed frame counting with automatic connection termination

- **ring.go** (3,016 bytes)  
  Per-link in-memory ring buffer (256 samples, ~5-12 seconds at 20-50 Hz)

- **message.go** (6,067 bytes)  
  JSON message handling (hello, health, BLE, OTA status, etc.)

- **ratecontrol.go** (12,095 bytes)  
  Adaptive sensing rate control (idle 2 Hz ↔ active 50 Hz)

## Related Supporting Modules

### Signal Processing Pipeline

**Directory:** `mothership/internal/signal/`  
**Purpose:** Converts raw CSI into motion features

**Key files:**
- **phase.go** - Phase sanitization (unwrap, STO/CFO removal)
- **nbvi.go** - NBVI subcarrier selection (Welford online algorithm)
- **feature.go** - deltaRMS, phase variance, breathing band extraction
- **processor.go** - Main pipeline orchestrator

### Configuration

**File:** `mothership/internal/config/config.go`  
**Purpose:** Environment variable parsing and defaults

**Recording-related config:**
- `SPAXEL_RECORDING_RETENTION` - Time-based retention (e.g., "24h", "72h")
- `SPAXEL_REPLAY_MAX_MB` - Size cap for recording buffer (default: 360 MB = 48h at 8 nodes)
- `SPAXEL_FUSION_RATE_HZ` - Fusion loop rate (default: 10 Hz)
- `SPAXEL_BIND_ADDR` - Listen address (default: 0.0.0.0:8080)

### API Layer

**File:** `mothership/internal/api/replay.go`  
**Purpose:** REST endpoints for replay control

**Endpoints:**
- `POST /api/replay/start` - Begin replay session
- `POST /api/replay/seek` - Seek to timestamp
- `POST /api/replay/play` - Start playback at speed (1×, 2×, 5×)
- `POST /api/replay/pause` - Pause playback
- `POST /api/replay/stop` - End session
- `PATCH /api/replay/params` - Update tunable parameters

### Load Shedding

**Directory:** `mothership/internal/loadshed/`  
**Purpose:** Prevent overload under high CSI rates

**Integration:** Recording buffer writes are suspended in Level 2 load shedding

## Data Flow Architecture

```
ESP32 Node → WebSocket /ws/node
    │
    ├─▶ Binary CSI Frame (raw bytes)
    │   └─▶ ingestion.ParseBinaryFrame() → validation
    │       └─▶ recording.Buffer.Write() → circular buffer
    │       └─▶ ring buffer → signal processing pipeline
    │
    └─▶ JSON Messages (hello, health, BLE, OTA)
        └─▶ message handlers

Recording Buffer (disk-backed)
    │
    └─▶ replay.Engine (time-travel debugging)
        └─▶ replay pipeline (separate from live)
            └─▶ dashboard WebSocket feed
```

## Storage Locations

### On Disk

**Directory:** `/data/` (configured via `SPAXEL_DATA_DIR`)

**Files:**
- `csi_replay.bin` - Circular buffer (managed by `recording.Buffer`)
- `data/{nodeMAC}-{peerMAC}/{YYYYMMDD-HH}.csi` - Per-link segment files (managed by `recorder.Manager`)

### In Memory

- **Ring buffers** - Per-link, 256 samples each (~38 KB per link)
- **Replay session state** - Active replay sessions with tunable parameters
- **CSI frame cache** - Recent frames for dashboard broadcasting

## CSI Frame Format

**Binary frame received via WebSocket (24 bytes header + n_sub×2 payload):**

```
Header (24 bytes):
  node_mac:     6 bytes    — source node MAC
  peer_mac:     6 bytes    — transmitting node MAC
  timestamp_us: 8 bytes    — microseconds since node boot (uint64 LE)
  rssi:         1 byte     — signed RSSI in dBm (int8)
  noise_floor:  1 byte     — signed noise floor in dBm (int8)
  channel:      1 byte     — WiFi channel (uint8)
  n_sub:        1 byte     — number of subcarriers (uint8, typically 64)

Payload (n_sub × 2 bytes):
  Per subcarrier: int8 I, int8 Q
```

**Total frame size:** 152 bytes for 64 subcarriers (typical)

## Module Boundaries

### Recording Module (`recording/`)
**Boundary:** Pure CSI storage and retrieval  
**Responsibilities:**
- Circular buffer file I/O
- Retention policy enforcement
- Timestamp range queries

**Does NOT handle:**
- CSI parsing (handled by `ingestion/`)
- Signal processing (handled by `signal/`)
- Node lifecycle (handled by `fleet/`)

### Recorder Module (`recorder/`)
**Boundary:** Per-link file-based recording with hourly segments  
**Responsibilities:**
- Link-specific segment file creation
- Hourly rotation
- Background cleanup

**Does NOT handle:**
- CSI ingestion (handled by `ingestion/`)
- Replay operations (handled by `replay/`)

### Replay Module (`replay/`)
**Boundary:** Time-travel debugging and parameter tuning  
**Responsibilities:**
- Session management
- Timestamp seeking
- Parameter tuning and pipeline re-runs
- Dashboard integration for replay visualization

**Does NOT handle:**
- Live CSI ingestion (handled by `ingestion/`)
- Real-time blob tracking (handled by `localization/`)

### Ingestion Module (`ingestion/`)
**Boundary:** WebSocket server and CSI frame validation  
**Responsibilities:**
- Node connection lifecycle
- Binary frame parsing and validation
- JSON message handling
- Rate control and adaptive sensing

**Does NOT handle:**
- CSI storage (delegates to `recording/`)
- Signal feature extraction (delegates to `signal/`)
- Fleet management (delegates to `fleet/`)

## Integration Points

### Main Entry Point

**File:** `mothership/cmd/mothership/main.go`  
The mothership wires these subsystems together during startup sequencing:

1. Create `recording.Buffer` with configured retention
2. Create `recorder.Manager` with data directory
3. Create `replay.Engine` with buffer and blob broadcaster
4. Create `ingestion.Server` with CSI broadcaster callback
5. Register CSI frame callback from ingestion → recording buffer

### Signal Processing Integration

CSI frames flow from ingestion through the signal processing pipeline:

```
ingestion.ParseBinaryFrame()
    └─▶ signal.Processor.Sanitize()       → phase sanitization
        └─▶ signal.Processor.Extract()     → deltaRMS, phase variance
            └─▶ signal.Processor.Baseline() → EMA baseline
```

### Dashboard Integration

Replay results are pushed to the dashboard via WebSocket:

```
replay.Engine
    └─▶ blobBroadcaster.BroadcastBlobs()
        └─▶ /ws/dashboard endpoint
```

## Test Coverage

### Unit Tests

- `recording/buffer_test.go` - Circular buffer operations
- `recorder/manager_test.go` - Recorder lifecycle
- `recorder/segment_test.go` - Segment file handling
- `replay/store_test.go` - Replay store operations
- `replay/engine_test.go` - Replay engine logic
- `ingestion/frame_test.go` - Frame parsing

### Integration Tests

- `test/acceptance/as6_replay_test.go` - End-to-end replay validation
- `replay/integration_test.go` - Full pipeline with real CSI data

### Fuzz Tests

- `replay/seek_fuzz_test.go` - Robustness of timestamp seeking
- `ingestion/frame_fuzz_test.go` - Frame parser robustness
- `ingestion/json_fuzz_test.go` - JSON message parser robustness

## Performance Considerations

### Write Rate

- **Per link:** ~150 bytes/frame × 20-50 Hz = 3-7.5 KB/s
- **8-node fleet (28 links):** ~84-210 KB/s
- **Peak (120 links, large fleet):** ~360-900 KB/s

### Storage Growth

- **Default retention:** 48 hours
- **8-node fleet estimate:** ~150-360 MB over 48h
- **Large fleet estimate:** Up to 1.5 GB (managed by `SPAXEL_REPLAY_MAX_MB`)

### Load Shedding

CSI recording is suspended in Level 2 load shedding to preserve system responsiveness under heavy load.

## Configuration Summary

| Environment Variable | Default | Purpose |
|---------------------|---------|---------|
| `SPAXEL_RECORDING_RETENTION` | 48h | Time-based CSI retention |
| `SPAXEL_REPLAY_MAX_MB` | 360 | Max CSI buffer size in MB |
| `SPAXEL_FUSION_RATE_HZ` | 10 | Fusion loop rate |
| `SPAXEL_DATA_DIR` | /data | Base directory for CSI files |
| `SPAXEL_BIND_ADDR` | 0.0.0.0:8080 | Mothership listen address |

## Key Takeaways

1. **Three-layer architecture:**
   - **Recording layer** (`recording/`, `recorder/`) - Storage persistence
   - **Replay layer** (`replay/`) - Time-travel debugging interface
   - **Ingestion layer** (`ingestion/`) - WebSocket data capture

2. **Separation of concerns:**
   - CSI parsing is isolated in `ingestion/frame.go`
   - Storage implementation is isolated in `recording/buffer.go`
   - Replay logic is isolated in `replay/engine.go`

3. **Modular design enables:**
   - Independent testing of each component
   - Alternative storage backends (theoretically possible)
   - Parameter tuning without affecting live pipeline

4. **Performance boundaries:**
   - Circular buffer prevents unbounded disk growth
   - Load shedding protects system under high CSI rates
   - Configurable retention balances storage vs. debugging capability

## References

- **Plan:** Component 14 (Time-Travel Debugging)
- **Schema:** CSI replay store format and file header specification
- **Tests:** `test/acceptance/as6_replay_test.go` (AS-6: Replay shows what happened at 2am)
