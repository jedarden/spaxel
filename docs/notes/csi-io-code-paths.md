# CSI Recording I/O Code Paths

**Last updated:** 2026-08-28
**Purpose:** Quick reference for CSI recording file I/O implementation locations

## Overview

Spaxel implements a dual-path CSI recording system:
- **Write path:** ESP32 WiFi → WebSocket → mothership → circular buffer (with optional zstd compression)
- **Read path:** Replay API → timestamp queries → decompression → signal processing

## Core I/O Files

### Mothership (Go)
| File | Purpose |
|------|---------|
| `internal/recording/buffer.go` | Circular buffer implementation (read/write/seek) |
| `internal/recording/compression.go` | zstd compression for CSI chunks |
| `internal/replay/store.go` | Replay-specific recording store |
| `internal/replay/worker.go` | Frame reading and replay execution |
| `internal/ingestion/server.go` | CSI frame ingestion entry point |
| `internal/ingestion/frame.go` | CSI frame parsing and validation |

### Firmware (C)
| File | Purpose |
|------|---------|
| `firmware/main/csi.c` | WiFi CSI hardware capture callback |
| `firmware/main/websocket.c` | CSI frame transmission over WebSocket |

## Key Entry Points

### Write Operations

**Firmware side:**
```c
// wifi_csi_cb() - hardware CSI capture
// websocket_send_csi() - transmit to mothership
```

**Mothership side:**
```go
// handleBinaryFrame() - main CSI ingestion (server.go:719)
// replay.Append() - write to circular buffer (server.go:747)
// Buffer.Append() - core write operation (buffer.go:158)
```

### Read Operations

**Primary read functions:**
```go
// Buffer.Scan() - iterate all frames (buffer.go:293)
// Buffer.ScanRange() - iterate time range (buffer.go)
// Buffer.SeekToTimestamp() - jump to timestamp (buffer.go)
// RecordingStore.Scan() - replay-specific scanning (store.go)
```

**Replay control:**
```go
// StartSession() - create replay session (api/replay.go:217)
// Worker.Seek() - seek to timestamp (replay/worker.go:192)
// POST /api/replay/* - REST control endpoints
```

## CSI Frame Format

**Binary frame (24 + n_sub×2 bytes):**
```
Header (24 bytes):
  node_mac[6]      // Source node MAC
  peer_mac[6]      // Transmitting peer MAC
  timestamp_us[8]  // Microseconds since boot (uint64 LE)
  rssi[1]          // Signal strength dBm (int8)
  noise_floor[1]   // Noise floor dBm (int8)
  channel[1]       // WiFi channel (uint8)
  n_sub[1]         // Subcarrier count (uint8)

Payload (n_sub × 2 bytes):
  I0,Q0,I1,Q1,...  // Signed 8-bit I/Q pairs
```

**Typical size:** 152 bytes for 64 subcarriers

## File Structure (on disk)

**Recording buffer header (32 bytes):**
```
magic[8]        = "SPAXLREC"
version[4]      = 1
write_pos[8]    // Next write offset
oldest_pos[8]   // Oldest valid record
wrap_pos[8]     // Wrap position
reserved[4]     = 0
```

**Per-frame record (10 + N bytes):**
```
recv_time_ns[8]  // Unix nanoseconds (int64 LE)
frame_len[2]     // Frame byte count (uint16 LE)
frame_data[N]    // Raw CSI frame bytes
```

**Compression (optional):**
- Chunks: 64KB target size
- Format: 2-byte length + "CSCZ" header + zstd data
- Triggered at 64KB or 5-second interval

## Constants & Limits

```go
MaxFrameBytes    = 280       // Maximum frame size
MaxSubcarriers   = 128       // Maximum subcarriers
DefaultMaxMB     = 360       // Default capacity (~48 hours)
DefaultRetention = 48h       // Time-based retention
DefaultChunkSize = 64 KB     // Compression target
```

## Data Flow Diagram

```
ESP32 WiFi Hardware
  ↓ (wifi_csi_cb)
CSI Capture
  ↓ (websocket_send_csi)
Binary WebSocket Frame
  ↓ (handleBinaryFrame)
Frame Parser
  ↓ (ParseFrame)
Validation
  ↓ (replay.Append)
Circular Buffer
  ↓ (Append)
File Storage (csi_replay.bin)
```

**Replay flow:**
```
POST /api/replay/start
  ↓ (StartSession)
Replay Session
  ↓ (Worker.Seek)
Buffer.ScanRange()
  ↓ (Decompression)
Frame Processing
  ↓ (ProcessFrame)
Signal Pipeline
```

## Related Documentation

- `docs/notes/csi-recording-file-format.md` - Complete binary format specification
- `docs/plan/plan.md` - Component 14: Time-Travel Debugging
- `testdata/generate_csi_recording.go` - Test data generator

## Notes

- Recording is subject to load shedding under high CPU load
- Compression is optional (default: enabled)
- Buffer evicts oldest data when size limit reached
- Replay can adjust pipeline parameters without affecting live system
