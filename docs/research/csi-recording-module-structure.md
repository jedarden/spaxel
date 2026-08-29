# CSI Recording Module Structure

**Date:** 2026-08-28  
**Purpose:** Document the organization, boundaries, and dependencies of Spaxel's CSI recording and replay system.

## Overview

CSI recording in Spaxel is split across two primary modules:

1. **Recording Module** (`internal/recording/`) — Disk-backed circular buffer for raw CSI frame storage
2. **Replay Module** (`internal/replay/`) — Time-travel debugging and replay pipeline

These modules are connected through the ingestion server, which writes incoming CSI frames to the recording buffer.

---

## Module 1: Recording (`internal/recording/`)

### Purpose
Provides a disk-backed circular buffer that stores raw CSI frames with time-based retention. This is the foundation for time-travel replay (Component 14 of the plan).

### File Organization

```
internal/recording/
├── buffer.go           — Main circular buffer implementation
├── compression.go      — Zstd compression for CSI chunks
├── buffer_test.go      — Unit tests for buffer operations
├── benchmark.go        — Performance benchmarks
```

### Key Data Structures

#### `Buffer` (buffer.go)
```go
type Buffer struct {
    mu           sync.Mutex
    f            *os.File           // Underlying file
    fileSize     int64             // Total file size (header + data)
    writePos     int64             // Next write position
    oldestPos    int64             // Oldest valid record (0 = empty)
    wrapPos      int64             // Position at last wrap
    retention    time.Duration     // Time-based retention period
    compression  bool              // Compression enabled flag
    compressor   *Compressor       // Zstd compressor
    chunkSize    int               // Target chunk size
    pendingChunk []byte           // Pending compressed data
}
```

**Key Operations:**
- `Append(recvTimeNS int64, rawFrame []byte)` — Write a CSI frame
- `Scan(fn func(recvTimeNS int64, frame []byte) bool)` — Iterate all frames
- `ScanRange(from, to time.Time, fn ...)` — Time-bounded iteration
- `SeekToTimestamp(target time.Time)` — Find nearest frame
- `GetTimestampRange()` — Get oldest/newest timestamps
- `Prune()` — Evict records older than retention period

#### `Compressor` / `Decompressor` (compression.go)

**Chunk Header Format (16 bytes):**
```
magic[4]        "CSCZ" (Compressed Spaxel Chunk)
version[1]       1
frameCount[2]    uint16 LE — number of frames in chunk
uncompressedLen[4] uint32 LE — total uncompressed size
reserved[5]      zero padding
```

**Compression Flow:**
1. Frames added via `AddFrame(recvTimeNS, frame)`
2. Chunk compressed when reaching target size (default 64 KB)
3. Compressed chunk written with header to buffer
4. `Decompressor.DecompressChunk()` decompresses and iterates frames

#### `Stats` Structure
```go
type Stats struct {
    HasData   bool          // Whether any records exist
    WritePos  int64         // Current write position
    OldestPos int64         // Oldest valid record
    FileSize  int64         // Total file size
    Retention time.Duration // Configured retention period
}
```

### File Layout

**Binary File Format (`csi_replay.bin`):**

```
Header (32 bytes):
  magic[8]      "SPAXLREC"
  writePos[8]   uint64 LE — next write position
  oldestPos[8]  uint64 LE — oldest valid record (0 = empty)
  wrapPos[8]    uint64 LE — last wrap position

Record Format (uncompressed):
  recvTimeNS[8]  int64 LE   — Unix nanosecond timestamp
  frameLen[2]   uint16 LE  — frame bytes length
  frameData[N]  raw CSI frame bytes

Compressed Chunk:
  chunkLen[2]     uint16 LE   — length of header+data
  header[16]     ChunkHeader (above)
  compressedData []byte      — zstd compressed frames
```

**Constants:**
- `maxFrameBytes = 280` — Maximum CSI frame size (24-byte header + 128×2 payload)
- `DefaultRetention = 48h` — Default time-based retention
- `DefaultMaxMB = 512` — Default disk capacity
- `recordOverhead = 10` — recvTimeNS(8) + frameLen(2)

### Dependencies

**External Dependencies:**
- `github.com/klauspost/compress/zstd` — Zstd compression library

**Internal Dependencies:**
- None (standalone module)

---

## Module 2: Replay (`internal/replay/`)

### Purpose
Provides time-travel debugging capabilities by replaying recorded CSI frames through a separate pipeline instance with adjustable parameters.

### File Organization

```
internal/replay/
├── types.go            — Session types, parameters, errors
├── store.go            — RecordingStore (circular buffer for replay)
├── engine.go           — Session management and coordination
├── session.go          — Session state machine
├── worker.go           — Playback worker goroutine
├── pipeline.go         — Replay pipeline implementation
├── buffer_adapter.go   — Adapter: recording.Buffer → RecordingStore
├── verify_recording_test.go  — Integration tests
├── engine_test.go      — Engine tests
├── pipeline_test.go    — Pipeline tests
├── store_test.go       — Store tests
├── integration_test.go — End-to-end tests
├── seek_fuzz_test.go   — Fuzz tests for seek operations
```

### Key Data Structures

#### `Session` (types.go)
```go
type Session struct {
    mu         sync.RWMutex
    id         string
    fromMS     int64              // Session start timestamp
    toMS       int64              // Session end timestamp
    currentMS  int64              // Current playback position
    speed      int                // Playback speed (1-5x)
    state      SessionState       // paused/playing/stopped
    params     *TunableParams     // Pipeline parameters
    created_at int64
    updated_at int64
    ctx        context.Context
    cancel     context.CancelFunc
    stopCh     chan struct{}
}
```

**Session States:**
- `StatePaused` — Playback paused
- `StatePlaying` — Actively playing back
- `StateStopped` — Terminated

#### `TunableParams` (types.go)
```go
type TunableParams struct {
    DeltaRMSThreshold    *float64 // Motion detection threshold
    TauS                 *float64 // Baseline time constant
    FresnelDecay         *float64 // Fresnel zone decay rate
    NSubcarriers         *int     // Subcarrier selection count
    BreathingSensitivity *float64 // Breathing band sensitivity
    FresnelWeightSigma   *float64 // Link weight uncertainty
    MinConfidence        *float64 // Minimum blob confidence
}
```

#### `Engine` (engine.go)
```go
type Engine struct {
    mu               sync.RWMutex
    sessions         map[string]*Session
    buffer           *recording.Buffer      // CSI recording buffer
    blobBroadcaster  BlobBroadcaster        // Dashboard updates
    defaultParams    *TunableParams         // Default parameters
    sessionIDCounter uint64
}
```

**Key Operations:**
- `StartSession(fromMS, toMS)` — Create new replay session
- `GetSession(id)` — Retrieve session
- `StopSession(id)` — Terminate session
- `Seek(id, targetMS)` — Seek to timestamp
- `Play(id, speed)` — Start playback
- `Pause(id)` — Pause playback
- `SetParams(id, params)` — Update pipeline parameters

#### `RecordingStore` (store.go)
```go
type RecordingStore struct {
    mu        sync.Mutex
    f         *os.File
    fileSize  int64
    writePos  int64
    oldestPos int64
    wrapPos   int64
}
```

**File Format (identical to recording.Buffer):**
```
Header (32 bytes):
  magic[8]     "SPAXLREP"
  writePos[8]  uint64 LE
  oldestPos[8] uint64 LE
  wrapPos[8]   uint64 LE

Record (10 + frameLen):
  recvTimeNS[8] int64 LE
  frameLen[2]  uint16 LE
  frameData[N] raw CSI bytes
```

#### `BlobUpdate` (types.go)
```go
type BlobUpdate struct {
    ID                 int       // Blob ID
    X, Y, Z             float64   // Position
    VX, VY, VZ          float64   // Velocity
    Weight             float64   // Confidence weight
    Posture            string    // Detected posture
    PersonID           string    // BLE person ID
    PersonLabel        string    // Person name
    PersonColor        string    // Visualization color
    IdentityConfidence float64   // Identity match confidence
    IdentitySource     string    // "ble" | "prediction"
    Trail              []float64 // [x,z,x,z,...] footprint history
}
```

### Dependencies

**Internal Dependencies:**
- `github.com/spaxel/mothership/internal/recording` — CSI recording buffer

**External Dependencies:**
- None (uses only standard library)

---

## Module 3: Ingestion Integration (`internal/ingestion/`)

### Purpose
Receives CSI frames from ESP32 nodes via WebSocket and writes them to the recording buffer.

### Key Integration Points

#### `ReplayAppender` Interface (server.go)
```go
type ReplayAppender interface {
    Append(recvTimeNS int64, rawFrame []byte) error
}
```

#### Frame Flow
```
ESP32 Node → WebSocket Binary Frame → Ingestion Server
                                            ↓
                                     Parse Binary Frame
                                            ↓
                                recording.Buffer.Append()
```

### File Organization (Relevant Files)

```
internal/ingestion/
├── server.go      — WebSocket server, CSI frame parsing
├── ring.go         — In-memory ring buffer for live pipeline
├── message.go      — CSI binary frame format parsing
└── ratecontrol.go  — Adaptive rate control
```

---

## Data Flow Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│ ESP32 Node                                                              │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │ CSI Capture (WiFi Promiscuous Mode)                           │  │
│  └────────────────────────────────────────────────────────────────┘  │
│                                   ↓                                   │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │ WebSocket Binary Frame (24-byte header + I/Q payload)        │  │
│  └────────────────────────────────────────────────────────────────┘  │
└───────────────────────────────┬───────────────────────────────────────┘
                                ↓ WebSocket
┌─────────────────────────────────────────────────────────────────────┐
│ Mothership Ingestion Server                                         │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │ Parse Binary Frame                                            │  │
│  │   - Extract MAC, timestamp, RSSI, channel, subcarriers     │  │
│  │   - Validate frame format                                     │  │
│  └────────────────────────────────────────────────────────────────┘  │
│                                   ↓                                   │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │ recording.Buffer.Append(recvTimeNS, rawFrame)               │  │
│  │   → Write to csi_replay.bin                                   │  │
│  │   → Prune old records (time-based eviction)                  │  │
│  │   → Optional compression (zstd chunks)                        │  │
│  └────────────────────────────────────────────────────────────────┘  │
│                                   ↓                                   │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │ Live Signal Pipeline                                          │  │
│  │   → Process frames in real-time (detection, localization)     │  │
│  └────────────────────────────────────────────────────────────────┘  │
└───────────────────────────────┬───────────────────────────────────────┘
                                ↓
┌─────────────────────────────────────────────────────────────────────┐
│ CSI Replay Buffer (csi_replay.bin)                                  │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │ Disk-Backed Circular Buffer                                   │  │
│  │   - Header: magic, writePos, oldestPos, wrapPos              │  │
│  │   - Records: timestamp + frame data                            │  │
│  │   - Retention: 48h default (configurable)                     │  │
│  │   - Compression: zstd chunks (optional)                        │  │
│  └────────────────────────────────────────────────────────────────┘  │
└───────────────────────────────┬───────────────────────────────────────┘
                                ↓
┌─────────────────────────────────────────────────────────────────────┐
│ Replay Engine (internal/replay/)                                   │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │ Session Management                                            │  │
│  │   - Start/Stop sessions                                       │  │
│  │   - Seek to timestamp                                        │  │
│  │   - Play/Pause/Speed control                                 │  │
│  └────────────────────────────────────────────────────────────────┘  │
│                                   ↓                                   │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │ Replay Pipeline                                               │  │
│  │   - Read frames from buffer                                   │  │
│  │   - Re-run detection with TunableParams                       │  │
│  │   - Emit BlobUpdate to dashboard                             │  │
│  └────────────────────────────────────────────────────────────────┘  │
└───────────────────────────────┬───────────────────────────────────────┘
                                ↓
┌─────────────────────────────────────────────────────────────────────┐
│ Dashboard (Time-Travel View)                                         │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │ 3D Scene at Replayed Moment                                   │  │
│  │   - Show blobs as they were detected                          │  │
│  │   - Adjust detection parameters live                           │  │
│  │   - Scrub timeline with parameter tuning                       │  │
│  └────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Module Dependencies

### Dependency Graph

```
┌─────────────────────────────────────────────────────────────────────┐
│                         main.go                                   │
│                    (cmd/mothership/)                               │
└───────────────────────────┬───────────────────────────────────────┘
                            │
              ┌─────────────┴─────────────┐
              │                           │
    ┌─────────▼─────────┐     ┌───────▼────────┐
    │   Ingestion       │     │    Recording   │
    │   Server         │────▶│    Buffer      │
    └─────────┬─────────┘     └───────┬────────┘
              │                       │
              │                ┌──────▼─────────┐
              │                │  Replay         │
              │                │  Engine         │
              │                └───────┬─────────┘
              │                        │
              ▼                        ▼
         WebSocket              Dashboard
        (Node → Mothership)      (Time-Travel UI)
```

### Import Relationships

**recording module:**
```
buffer.go          → encoding/binary, os, sync, time
compression.go    → bytes, encoding/binary, github.com/klauspost/compress/zstd
```

**replay module:**
```
types.go          → context, encoding/json, math, sync, time
store.go          → encoding/binary, errors, os, sync
engine.go         → fmt, sync, time, github.com/spaxel/mothership/internal/recording
session.go        → (types.go only)
worker.go         → (types.go, store.go)
pipeline.go       → (types.go, store.go)
buffer_adapter.go → github.com/spaxel/mothership/internal/recording
```

**ingestion module:**
```
server.go         → recording.Buffer (via ReplayAppender interface)
                   → internal ring buffer, rate control, message parsing
```

**main.go wiring:**
```go
// Create recording buffer
buf, err := recording.NewBuffer(
    filepath.Join(cfg.DataDir, "csi_replay.bin"),
    cfg.ReplayMaxMB,
    0,                    // retention (uses env var)
    cfg.ReplayCompression,
    cfg.ReplayChunkSizeMB*1024*1024,
)

// Wrap with replay adapter
adapter := replay.NewBufferAdapter(buf)

// Create replay engine
replayEngine := replay.NewEngine(buf, dashboard)

// Wire ingestion to recording
ingestionServer.SetReplayStore(adapter)
```

---

## Configuration

### Environment Variables

| Variable | Module | Default | Purpose |
|----------|--------|---------|---------|
| `SPAXEL_RECORDING_RETENTION` | recording | "48h" | Time-based retention period |
| `SPAXEL_REPLAY_MAX_MB` | main | 360 | Recording buffer capacity (MB) |
| `SPAXEL_REPLAY_COMPRESSION` | main | false | Enable zstd compression |
| `SPAXEL_REPLAY_CHUNK_SIZE_MB` | main | 0 | Target chunk size (MB) |

### Default Parameters

**Recording Buffer:**
- Disk capacity: 512 MB (`DefaultMaxMB`)
- Retention: 48 hours (`DefaultRetention`)
- Chunk size: 64 KB (`DefaultChunkSize`)
- Compression level: 3 (fast decode)

**Replay Engine:**
- Default playback speed: 1×
- Speed range: 1–5×
- Session timeout: managed by context cancellation

---

## Key Interfaces

### `ReplayAppender` (ingestion/server.go)
```go
type ReplayAppender interface {
    Append(recvTimeNS int64, rawFrame []byte) error
}
```
Implemented by:
- `recording.Buffer` — Direct circular buffer writes
- `replay.BufferAdapter` — Wrapper for replay compatibility

### `BlobBroadcaster` (replay/types.go)
```go
type BlobBroadcaster interface {
    BroadcastReplayBlobs(blobs []BlobUpdate, timestampMS int64)
}
```
Implemented by dashboard to push replay results to WebSocket clients.

---

## Performance Characteristics

### Storage Estimates

**Uncompressed:**
- Frame size: ~150 bytes (24-byte header + 64×2 I/Q)
- Rate: 600 frames/second (30 Hz × 20 links)
- Bandwidth: ~90 KB/s = ~324 MB/hour
- 48h retention: ~15.6 GB

**Compressed (zstd level 3):**
- Target compression ratio: 8:1 (CSI data is highly repetitive)
- Bandwidth: ~11 KB/s = ~40 MB/hour
- 48h retention: ~1.9 GB

### Timing

**Buffer Operations:**
- `Append()`: O(1) — single write + header sync
- `Scan()`: O(n) — linear scan through all frames
- `SeekToTimestamp()`: O(n) — worst case scan from oldest
- `Prune()`: O(k) — k evicted records

**Replay Playback:**
- 10 Hz fusion tick (100 ms budget)
- Speed 1×: real-time replay
- Speed 5×: 5× accelerated replay

---

## Testing Strategy

### Unit Tests

**recording module:**
- `buffer_test.go` — Circular buffer operations, wrap logic, eviction
- Compression round-trip tests
- Timestamp range queries

**replay module:**
- `store_test.go` — RecordingStore operations
- `engine_test.go` — Session lifecycle
- `pipeline_test.go` — Replay pipeline correctness
- `seek_fuzz_test.go` — Seek operation fuzzing

### Integration Tests

**replay/integration_test.go:**
- End-to-end replay session lifecycle
- Parameter tuning during replay
- Dashboard blob broadcasting

### Verification Tests

**replay/verify_recording_test.go:**
- Validates recording integrity after writes
- Corruption recovery tests
- Header validation

---

## File Summary

### Recording Module Files

| File | Lines | Purpose |
|------|-------|---------|
| `buffer.go` | ~910 | Circular buffer implementation, compression integration |
| `compression.go` | ~230 | Zstd compression/decompression |
| `buffer_test.go` | ~400 | Buffer unit tests |
| `benchmark.go` | ~200 | Performance benchmarks |

### Replay Module Files

| File | Lines | Purpose |
|------|-------|---------|
| `types.go` | ~320 | Session types, parameters, shared structs |
| `store.go` | ~365 | RecordingStore implementation |
| `engine.go` | ~250 | Session management, parameter merging |
| `session.go` | ~240 | Session state machine, playback loop |
| `worker.go` | ~150 | Playback worker goroutine |
| `pipeline.go` | ~200 | Replay pipeline coordination |
| `buffer_adapter.go` | ~60 | Adapter wrapper |
| Test files | ~800+ | Comprehensive test coverage |

### Integration Points

| Module | Integration Point | Purpose |
|--------|------------------|---------|
| `ingestion/server.go` | `ReplayAppender` interface | Write frames to buffer |
| `cmd/mothership/main.go` | `recording.NewBuffer()` | Create and configure buffer |
| `cmd/mothership/main.go` | `replay.NewEngine()` | Create replay system |
| Dashboard | `BlobBroadcaster` interface | Receive replay blob updates |

---

## Design Patterns

### Circular Buffer with Time-Based Eviction
- **Pattern:** Ring buffer with wrap-around writes
- **Eviction:** Time-based (older than retention period) + space-based (when buffer full)
- **Recovery:** Truncation to last valid header on corruption

### Adapter Pattern
- **Purpose:** Bridge between `recording.Buffer` and `replay.RecordingStore`
- **Implementation:** `buffer_adapter.go` wraps Buffer to expose ReplayStore-compatible interface
- **Benefit:** Allows replay module to work with either storage backend

### Session Pattern
- **Purpose:** Manage isolated replay state per dashboard client
- **State Machine:** Paused ↔ Playing ↔ Stopped
- **Context:** Cancellation for cleanup on session end

### Parameter Merging
- **Purpose:** Layer default → session-specific → user-tuned parameters
- **Implementation:** Deep copy with selective field updates in `SetParams()`
- **Benefit:** Preserves user adjustments across session lifetime

---

## Future Extensions

### Planned Features (from plan.md)

1. **Parameter Tuning Overlay** — Live UI controls for `TunableParams` during replay
2. **"Apply to Live" Button** — Copy tuned parameters from replay to live pipeline
3. **Timeline Integration** — Scrub through activity timeline with replay
4. **Compression Auto-Tuning** — Adaptive chunk size based on compression ratio
5. **Multi-Session Support** — Multiple concurrent replay sessions

### Storage Optimization

- **Adaptive Retention:** Extend retention based on measured compression ratio
- **Sparse Storage:** Compress consecutive quiescent periods more aggressively
- **Index Build:** Optional timestamp index for O(log n) seeks (trade-off: write overhead)

---

## Conclusion

The CSI recording system is organized into two cleanly separated modules:

1. **Recording (`internal/recording/`)** — Low-level circular buffer with compression
2. **Replay (`internal/replay/`)** — High-level session management and time-travel debugging

The modules are designed for:
- **Concurrent safety** — All operations are mutex-protected
- **Durability** — Disk-backed with crash recovery
- **Performance** — O(1) writes, efficient compression
- **Flexibility** — Pluggable storage backend via adapter pattern

This architecture supports the plan's Component 14 (Time-Travel Debugging) and provides the foundation for CSI-based forensics, algorithm tuning, and historical analysis.
