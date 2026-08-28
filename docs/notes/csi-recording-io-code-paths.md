# CSI Recording I/O Code Paths

**Date:** 2026-08-28  
**Purpose:** Document all code that reads or writes CSI recording files

## Overview

CSI recording uses a binary append-only file at `/data/csi_replay.bin` (configurable via `SPAXEL_REPLAY_MAX_MB`). The system supports both raw and compressed recording formats with a circular buffer eviction policy.

## Binary CSI Frame Format

**Parsed by:** `mothership/internal/ingestion/frame.go:ParseFrame()`

**Header (24 bytes):**
```
node_mac[6]      — source node MAC address
peer_mac[6]      — transmitting peer MAC  
timestamp_us[8]  — uint64 LE, microseconds since node boot
rssi[1]          — int8, signal strength in dBm
noise_floor[1]   — int8, noise floor in dBm
channel[1]       — uint8, WiFi channel (1-14)
n_sub[1]         — uint8, subcarrier count (0-128)
payload[n_sub×2]  — I,Q pairs as signed bytes
```

**Validation:** Checks frame length ≥ 24 bytes, payload size matches n_sub×2, channel 1-14.

## CSI Recording — Write Path

### Entry Point
**File:** `mothership/internal/ingestion/server.go:handleBinaryFrame()`

**Call chain:**
```
handleBinaryFrame(nc, data)
  → ParseFrame(data)                    // Deserialize binary CSI
  → replay.Append(recvTime.UnixNano(), data)  // Write to recording
  → recording.Buffer.Append()            // Binary serialization
```

### Core Implementation
**File:** `mothership/internal/recording/buffer.go`

**Key struct:** `Buffer` with methods:
- `NewBuffer(path, maxMB, retention, enableCompression, chunkSize)` — Initialize
- `Append(recvTimeNS int64, rawFrame []byte) error` — Write frame with timestamp
- `appendUncompressed()` — Direct binary write
- `appendCompressed()` — Batched compression via zstd

**File format (magic: "SPAXLREC"):**
```
Header (32 bytes):
  magic[8]        — "SPAXLREC"
  writePos[8]     — Current write position
  oldestPos[8]    — Oldest retained frame (eviction cursor)
  wrapPos[8]     — Wrap detection

Record (variable):
  recvTimeNS[8]   — int64, nanosecond timestamp
  frameLen[2]     — uint16, frame length
  frameData[N]    — Raw CSI binary frame
```

### Compression
**File:** `mothership/internal/recording/compression.go`

**Compressed chunk format (magic: "CSCZ"):**
```
Header (16 bytes):
  magic[4]            — "CSCZ"
  version[1]          — 1
  frameCount[2]       — Number of frames in chunk
  uncompressedLen[4]  — Total uncompressed size
  reserved[5]         — Future use

Data: zstd-compressed frame batch
```

**Methods:**
- `Compressor.AddFrame()` — Accumulate frames
- `Compressor.Flush()` — Compress and emit chunk
- `Decompressor.DecompressChunk()` — Decompress and iterate

## CSI Replay — Read Path

### Core Implementation
**Files:** `mothership/internal/replay/`

**Key types:**
- `RecordingStore` (store.go) — Legacy replay format
- `BufferAdapter` (buffer_adapter.go) — Wraps `recording.Buffer`
- `Worker` (worker.go) — Reads and processes frames
- `Engine` (engine.go) — Session management

**Key methods:**
- `Scan(fn func(recvTimeNS, frame) bool)` — Iterate all frames
- `ScanRange(fromNS, toNS, fn)` — Time-range filtered scan
- `recording.Buffer.SeekToTimestamp(target)` — Binary search

### REST API
**File:** `mothership/internal/api/replay.go`

**Handler:** `ReplayHandler` with endpoints:
- `POST /api/replay/start` — Create replay session with time range
- `POST /api/replay/seek` — Jump to timestamp
- `POST /api/replay/play` — Start/resume playback
- `POST /api/replay/pause` — Pause playback
- `PATCH /api/replay/params` — Tune pipeline parameters during replay

**Features:**
- Playback speed control (1x-5x)
- Adjustable detection thresholds during replay
- Session state management

## Application Integration

**File:** `mothership/cmd/mothership/main.go:888`

**Initialization:**
```go
buf, err := recording.NewBuffer(
    filepath.Join(cfg.DataDir, "csi_replay.bin"),
    cfg.ReplayMaxMB,
    0,
    cfg.ReplayCompression,
    cfg.ReplayChunkSizeMB*1024*1024
)
adapter := replay.NewBufferAdapter(buf)
ingestSrv.SetReplayStore(adapter)  // For writing
replayStore = adapter               // For reading
```

## Complete I/O Flow

**Write path (live ingestion):**
```
WebSocket binary frame
  → handleBinaryFrame()
  → ParseFrame()                    // Validate + deserialize
  → replay.Append()                 // Queue for write
  → recording.Buffer.Append()        // Serialize to binary
  → appendCompressed()               // Optional zstd chunking
  → Write to /data/csi_replay.bin
```

**Read path (replay/debugging):**
```
/api/replay/start {from_iso8601, to_iso8601}
  → Create replay session
  → ScanRange(fromNS, toNS, fn)
  → recording.Buffer.scan()         // Read file
  → Decompress chunks                // If compressed
  → Return raw CSI frames
  → Pipeline processing with tuned parameters
```

## Configuration

- **File location:** `$SPAXEL_DATA_DIR/csi_replay.bin` (default `/data/csi_replay.bin`)
- **Max size:** `SPAXEL_REPLAY_MAX_MB` (default 512 MB)
- **Compression:** `SPAXEL_REPLAY_COMPRESSION` (bool)
- **Chunk size:** `SPAXEL_REPLAY_CHUNK_SIZE_MB` (default 4 MB)

## Test Data

**File:** `testdata/csi_session_mixed_activity.bin` (377 MB)
- Generated by `test/generate_csi_recording.go`
- Contains realistic idle/walking activity patterns
- Used for compression benchmarking and replay testing

## Entry Points Summary

| Operation | Entry Point | File |
|-----------|-------------|------|
| Parse incoming CSI | `ParseFrame()` | `internal/ingestion/frame.go` |
| Write CSI to disk | `Buffer.Append()` | `internal/recording/buffer.go` |
| Compress chunks | `Compressor.Flush()` | `internal/recording/compression.go` |
| Read CSI frames | `Scan()`, `ScanRange()` | `internal/replay/buffer_adapter.go` |
| Replay API | `ReplayHandler` | `internal/api/replay.go` |
| Initialize buffer | `NewBuffer()` | `internal/recording/buffer.go` |

## Related Documentation

- `docs/plan/plan.md` — Component 14: Time-Travel Debugging
- `docs/plan/plan.md` — Component 46: CSI Simulator
- `docs/plan/plan.md` — CSI Replay Store (file format specification)
