# CSI Recording Format Documentation

**Last updated:** 2026-08-28  
**Purpose:** Document the CSI recording format used in Spaxel for time-travel debugging and replay

## Overview

Spaxel records raw CSI (Channel State Information) frames to disk-backed circular buffers, enabling time-travel debugging and parameter tuning without requiring hardware. The system uses two related formats:

1. **Recording Store** (`SPAXLREP`) — Long-term storage for replay sessions
2. **Recording Buffer** (`SPAXLREC`) — Runtime circular buffer with optional compression

Both formats share the same record structure but differ in their magic numbers and use cases.

---

## CSI Frame Format (Network Transmission)

Before being written to disk, CSI frames arrive as binary WebSocket frames from ESP32 nodes.

### Frame Header (24 bytes, fixed)

| Offset | Size | Field | Type | Description |
|--------|------|-------|------|-------------|
| 0 | 6 | `node_mac` | bytes[6] | Source node MAC address |
| 6 | 6 | `peer_mac` | bytes[6] | Transmitting peer MAC address (TX node or router) |
| 12 | 8 | `timestamp_us` | uint64 LE | Microseconds since node boot (monotonic) |
| 20 | 8 | `rssi` | int8 | Signal strength in dBm |
| 21 | 1 | `noise_floor` | int8 | Noise floor in dBm |
| 22 | 1 | `channel` | uint8 | WiFi channel (1-14 for 2.4 GHz) |
| 23 | 1 | `n_sub` | uint8 | Number of subcarriers (typically 64, max 128) |

**Note:** All multi-byte integers are **little-endian**.

### Frame Payload (n_sub × 2 bytes)

The payload consists of interleaved I/Q pairs for each subcarrier:

```
[I0, Q0, I1, Q1, I2, Q2, ..., I(n_sub-1), Q(n_sub-1)]
```

Each I and Q value is a signed 8-bit integer (`int8`).

**Total frame size:** `24 + (n_sub × 2)` bytes
- Typical (64 subcarriers): 152 bytes
- Maximum (128 subcarriers): 280 bytes

### Amplitude/Phase Computation

After parsing, the pipeline converts I/Q pairs to amplitude and phase:

```go
amplitude[k] = sqrt(I_k² + Q_k²)
phase[k]     = atan2(Q_k, I_k)  // radians, range [-π, π]
```

**RSSI normalization** (signal processing step):
```
normalized_amplitude[k] = amplitude[k] × 10^((RSSIRef - rssi_dbm) / 20)
```
Where `RSSIRef = -30.0` dBm. This compensates for automatic gain control (AGC).

---

## Recording Store Format (`SPAXLREP`)

**File:** `/data/csi_replay.bin` (configurable path)  
**Magic:** `"SPAXLREP"` (8 bytes)  
**Purpose:** Long-term CSI frame storage for replay/time-travel debugging

### File Header (32 bytes)

| Offset | Size | Field | Type | Description |
|--------|------|-------|------|-------------|
| 0 | 8 | `magic` | char[8] | `"SPAXLREP"` null-terminated |
| 8 | 8 | `writePos` | uint64 LE | Byte offset of next write position |
| 16 | 8 | `oldestPos` | uint64 LE | Byte offset of oldest valid record (0 = empty) |
| 24 | 8 | `wrapPos` | uint64 LE | Write position at last wrap (0 = no wrap) |

### Record Format (variable length)

Each CSI frame is stored as a record:

| Offset | Size | Field | Type | Description |
|--------|------|-------|------|-------------|
| 0 | 8 | `recvTimeNS` | int64 LE | Unix nanosecond timestamp (mothership receive time) |
| 8 | 2 | `frameLen` | uint16 LE | Length of CSI frame in bytes |
| 10 | N | `frameData` | bytes[] | Raw CSI frame bytes (header + payload) |

**Record overhead:** 10 bytes per frame (before CSI frame data)

### Circular Buffer Behavior

1. **Append writes** append records sequentially starting at `writePos`
2. **Wrap-around:** When `writePos + recordSize > fileSize`, the buffer wraps:
   - `wrapPos` is set to current `writePos`
   - `writePos` resets to `headerSize` (32)
3. **Eviction:** Old records are evicted when new writes would overwrite them
4. **Oldest tracking:** `oldestPos` points to the oldest still-valid record

### Default Configuration

| Setting | Value | Description |
|---------|-------|-------------|
| `SPAXEL_REPLAY_MAX_MB` | 360 MB | Total buffer capacity (~48 hours at 20 Hz, 20 links) |
| `SPAXEL_REPLAY_RETAIN_H` | 48 hours | Advisory retention time |
| Max frame size | 280 bytes | Enforced limit (24 header + 128×2 payload) |
| File size | 32 + (maxMB × 1024 × 1024) | Header + data capacity |

---

## Recording Buffer Format (`SPAXLREC`)

**File:** Runtime in-memory buffer (same on-disk format)  
**Magic:** `"SPAXLREC"` (8 bytes)  
**Purpose:** Real-time CSI capture with optional zstd compression

### Differences from Recording Store

1. **Compression:** Supports compressed chunks with magic `"CSCZ"` prefix
2. **Same header structure:** 32-byte header with `writePos`, `oldestPos`, `wrapPos`
3. **Same record format:** 10-byte record header + CSI frame data

### Compressed Chunk Structure

When a chunk is compressed:
- Magic: `"CSCZ"` (8 bytes)
- `compressedSize` (8 bytes, uint64 LE)
- `decompressedSize` (8 bytes, uint64 LE)  
- `compressedData` (variable)

---

## Valid CSI Recording Requirements

A valid CSI recording file must satisfy:

### File-Level Requirements

1. **Valid magic number:**
   - Recording Store: `"SPAXLREP"`
   - Recording Buffer: `"SPAXLREC"` (or `"CSCZ"` for compressed chunks)

2. **Minimum file size:** 32 bytes (at least the header)

3. **Valid header positions:**
   - `writePos >= headerSize (32)`
   - `writePos <= fileSize`
   - `oldestPos == 0` OR `oldestPos >= headerSize`
   - `wrapPos == 0` OR `wrapPos >= headerSize`

### CSI Frame Validation (per frame)

1. **Minimum frame length:** 24 bytes (header only; `n_sub=0` is valid)

2. **Payload length match:**
   ```
   expectedLen = 24 + (n_sub × 2)
   actualLen must == expectedLen
   ```

3. **Subcarrier count:** `0 <= n_sub <= 128` (values >128 are rejected)

4. **Valid channel:** `1 <= channel <= 14` (2.4 GHz WiFi channels)

5. **Channel != 0:** Channel 0 is invalid (rejected)

### Parse-Time Validation

The ingestion server (`mothership/internal/ingestion/frame.go`) validates all incoming frames:

```go
// Validation rule 1: minimum length
if len(data) < 24 { reject }

// Validation rule 2: n_sub from byte 23
nSub := data[23]

// Validation rule 3: payload length match
expectedLen := 24 + int(nSub)*2
if len(data) != expectedLen { reject }

// Validation rule 4: n_sub <= 128
if nSub > 128 { reject }

// Validation rule 5: RSSI == 0 allowed but logged (AGC skip)
if rssi == 0 { log DEBUG; continue }

// Validation rule 6: channel must be 1-14
if channel == 0 || channel > 14 { reject }
```

### Amplitude/Phase Validity

After parsing, the signal processing pipeline checks for:

1. **Non-finite values:** NaN or Inf in amplitude/phase after computation → frame skipped
2. **Zero division:** Regression denominator near-zero → sanitization fails

---

## Replay/Load Functions

### Core Replay Engine

**Location:** `mothership/internal/replay/`

| File | Purpose |
|------|---------|
| `store.go` | Low-level disk I/O for `RecordingStore` |
| `engine.go` | Session management, play/pause/seek controls |
| `worker.go` | Background replay processing goroutine |
| `pipeline.go` | Re-runs detection pipeline with tuned parameters |
| `types.go` | Session state and tunable parameter structs |

### Key Replay Functions

```go
// Append a CSI frame to the store
Append(recvTimeNS int64, rawFrame []byte) error

// Scan all records from oldest to newest
Scan(fn func(recvTimeNS int64, frame []byte) bool) error

// Scan records within a time range
ScanRange(fromNS, toNS int64, fn func(recvTimeNS int64, frame []byte) bool) error

// Get store statistics
Stats() Stats { HasData, WritePos, OldestPos, FileSize }
```

### Recording Buffer

**Location:** `mothership/internal/recording/buffer.go`

```go
// Circular buffer with time-based retention
type Buffer struct {
    maxRetention time.Duration
    compression  bool
}

// Append a frame
Append(recvTime time.Time, frame []byte) error

// Scan all frames
Scan(ctx context.Context, fn func(recvTime time.Time, frame []byte) error) error

// Scan time range
ScanRange(from, to time.Time, fn func(...) error) error
```

---

## Test Data Location

### Primary Test Data Directory

**Path:** `/home/coding/spaxel/testdata/`

**Contents:**
- `csi_session_mixed_activity.bin` (377 MB) — Sample CSI recording for testing
- `generate_csi_recording.go` — Tool to create synthetic CSI recordings
- `verify_recording.go` — Validation tool to check recording integrity

### Runtime Data Directories

**Path:** `/home/coding/spaxel/data/`

**Contents:**
- `csi_replay.bin` (361 MB) — Runtime replay store (current recording buffer)
- `csi/` — Additional CSI storage directory

### CSI Simulator

**Location:** `/home/coding/spaxel/cmd/sim/`

Generates synthetic CSI for testing without hardware:
```bash
spaxel-sim \
  --mothership ws://localhost:8080/ws/node \
  --nodes 4 \
  --walkers 1 \
  --rate 20 \
  --duration 60s \
  --ble \
  --seed 42
```

---

## Creating Valid CSI Recordings

### Method 1: Using the Generator

```bash
cd /home/coding/spaxel
go run testdata/generate_csi_recording.go
```

This creates `testdata/csi_session_mixed_activity.bin` with:
- 60 seconds duration
- 20 Hz sampling (1,200 total frames)
- Pattern: 20s idle → 20s walking → 20s idle
- 64 subcarriers per frame
- Channel 6, RSSI -40 to -60 dBm

### Method 2: Using the Simulator

```bash
# Generate synthetic CSI and stream to mothership
spaxel-sim --mothership ws://localhost:8080/ws/node \
          --nodes 2 \
          --walkers 1 \
          --rate 20 \
          --duration 30s

# The mothership will record to /data/csi_replay.bin automatically
```

### Method 3: From Real Hardware

1. Flash ESP32-S3 with Spaxel firmware
2. Provision node via dashboard
3. CSI frames stream automatically to `/data/csi_replay.bin`

### Verification

```bash
cd /home/coding/spaxel
go run testdata/verify_recording.go
```

Output example:
```
CSI Recording Verification
=========================
File: /home/coding/spaxel/testdata/csi_session_mixed_activity.bin
Magic: SPAXLREP ✓
File size: 395251448 bytes (377.00 MB)
Write position: 395251478
Oldest position: 32
Wrap position: 0

Frame Statistics
================
Total frames: 1200
Duration: 60.00 seconds
Average amplitude per frame: 2543.42
Frame size: 152 bytes (header: 24 + payload: 128)
Subcarriers: 64
RSSI: -50 dBm
Channel: 6

✓ Recording is valid and readable by replay code!
```

---

## Replay API Endpoints

| Method | Path | Request | Response |
|--------|------|---------|----------|
| `POST` | `/api/replay/start` | `{from_iso8601, to_iso8601}` | `{session_id}` |
| `POST` | `/api/replay/seek` | `{session_id, timestamp_iso8601}` | `{ok:true}` |
| `POST` | `/api/replay/play` | `{session_id, speed:1\|2\|5}` | `{ok:true}` |
| `POST` | `/api/replay/pause` | `{session_id}` | `{ok:true}` |
| `POST` | `/api/replay/stop` | `{session_id}` | `{ok:true}` |
| `PATCH` | `/api/replay/params` | `{session_id, delta_rms_threshold?, ...}` | Re-runs pipeline |
| `POST` | `/api/replay/apply-params` | `{session_id}` | Copies tuned params to live |

---

## Code Locations

### CSI Frame Parsing
- `mothership/internal/ingestion/frame.go` — ParseFrame(), CSIFrame struct
- `mothership/internal/signal/phase.go` — Phase sanitization, amplitude/phase computation

### Replay Storage
- `mothership/internal/replay/store.go` — RecordingStore implementation
- `mothership/internal/recording/buffer.go` — Runtime circular buffer

### Replay Engine
- `mothership/internal/replay/engine.go` — Session management
- `mothership/internal/replay/worker.go` — Background replay worker
- `mothership/internal/replay/pipeline.go` — Pipeline reprocessing

### Test Data Tools
- `testdata/generate_csi_recording.go` — Synthetic CSI generator
- `testdata/verify_recording.go` — Recording validation
- `cmd/sim/main.go` — Hardware simulator

### Tests
- `mothership/test/acceptance/as6_replay_test.go` — Replay acceptance tests
- `test/acceptance/as6_replay_test.go` — Cross-cutting replay tests

---

## Usage Example

### Recording CSI (Automatic)

CSI frames are automatically recorded to `/data/csi_replay.bin` when nodes are streaming. No manual action needed.

### Replaying a Time Range

```bash
# Start replay session for last 60 seconds
curl -X POST http://localhost:8080/api/replay/start \
  -H "Content-Type: application/json" \
  -d '{"from_iso8601":"2026-08-28T14:00:00Z","to_iso8601":"2026-08-28T14:01:00Z"}'

# Response: {"session_id":"abc123..."}

# Seek to specific timestamp
curl -X POST http://localhost:8080/api/replay/seek \
  -H "Content-Type: application/json" \
  -d '{"session_id":"abc123...","timestamp_iso8601":"2026-08-28T14:00:30Z"}'

# Play at 2x speed
curl -X POST http://localhost:8080/api/replay/play \
  -H "Content-Type: application/json" \
  -d '{"session_id":"abc123...","speed":2}'
```

### Tuning Parameters During Replay

```bash
# Adjust detection threshold and re-run pipeline on recorded data
curl -X PATCH http://localhost:8080/api/replay/params \
  -H "Content-Type: application/json" \
  -d '{
    "session_id":"abc123...",
    "delta_rms_threshold":0.03,
    "fresnel_decay":2.5,
    "n_subcarriers":16
  }'
```

The dashboard immediately shows how detection would differ with the new parameters.

---

## Key Takeaways

1. **CSI frames are I/Q pairs:** Each subcarrier has two int8 values (in-phase and quadrature)
2. **Amplitude/phase are derived:** Computed from I/Q, not stored directly
3. **Two file formats:** `SPAXLREP` (replay store) and `SPAXLREC` (buffer with compression)
4. **Circular buffer:** Old data evicted automatically when buffer is full
5. **Test data exists:** 377 MB sample recording in `testdata/`
6. **Validation is strict:** Frames are rejected for malformed length, invalid channels, or implausible subcarrier counts
7. **Time-travel debugging:** Replay enables "what happened at 2am?" investigations without hardware
