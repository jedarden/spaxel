# CSI Recording Module Structure

**Created:** 2026-08-28  
**Component:** Component 14 (Time-Travel Debugging) from plan.md  
**Status:** Implemented and operational

## Overview

The CSI recording module enables time-travel debugging by continuously recording raw CSI frames to disk, then replaying them through the detection pipeline with adjustable parameters. This is a foundational feature for system tuning, debugging, and historical analysis.

## Module Architecture

The recording system is split across four main packages with clear separation of concerns:

```
mothership/internal/
├── recording/          # Core circular buffer with compression
├── recorder/           # Per-link segmented recording
├── replay/             # Time-travel replay engine
└── api/replay.go        # REST API for replay control
```

---

## 1. Recording Buffer Package

**Location:** `mothership/internal/recording/`

### Primary Files

| File | Purpose | Key Components |
|------|---------|----------------|
| `buffer.go` | Disk-backed circular buffer for CSI frames | `Buffer` struct, `Append()`, `Scan()`, `ScanRange()`, `SeekToTimestamp()`, `GetTimestampRange()`, `Prune()` |
| `compression.go` | Zstd compression for CSI frame chunks | `Compressor`, `Decompressor`, `ChunkHeader`, compression constants |
| `benchmark.go` | Performance benchmarks | Buffer operation benchmarks |
| `buffer_test.go` | Unit tests | Circular buffer tests, compression tests |

### Core Data Structure: Buffer

The `Buffer` struct is a thread-safe, disk-backed circular buffer with optional compression:

```go
type Buffer struct {
    mu           sync.Mutex
    f            *os.File
    fileSize     int64          // Total file size (header + data)
    writePos     int64          // Offset of next write
    oldestPos    int64          // Offset of oldest valid record
    wrapPos      int64          // Write position at last wrap
    retention    time.Duration // Time-based retention (default 48h)
    compression  bool           // Enable zstd compression
    compressor   *Compressor
    chunkSize    int            // Target chunk size (default 64KB)
}
```

### File Format

**Uncompressed Format (legacy):**
```
Header (32 bytes):
  magic[8]      "SPAXLREC"
  writePos[8]   uint64 LE
  oldestPos[8]  uint64 LE
  wrapPos[8]    uint64 LE

Record (10 + frameLen bytes):
  recvTimeNS[8]  int64 LE  — Unix nanosecond timestamp
  frameLen[2]    uint16 LE — CSI frame bytes length
  frameData[N]   raw CSI frame bytes
```

**Compressed Format:**
```
Record (variable):
  chunkLen[2]      uint16 LE — length of header+data
  header[16]      ChunkHeader:
    magic[4]           "CSCZ"
    version[1]        1
    frameCount[2]     uint16 LE
    uncompressedLen[4] uint32 LE
    reserved[5]      zero padding
  compressedData[N]  zstd-compressed frame payloads
```

### Key Operations

- **Append(recvTimeNS, rawFrame)** - Writes a CSI frame, auto-prunes expired records
- **Scan(fn)** - Iterates all records chronologically
- **ScanRange(from, to, fn)** - Iterates records within time range
- **SeekToTimestamp(target)** - Finds frame closest to target timestamp (O(N) scan)
- **GetTimestampRange()** - Returns oldest and newest timestamps available
- **Prune()** - Removes records older than retention period
- **CompressionEnabled()** - Reports whether compression is active
- **EffectiveRetention()** - Estimates actual retention based on compression ratio

### Configuration

**Environment Variables:**
- `SPAXEL_RECORDING_RETENTION` - Time-based retention (e.g., "24h", "72h")
- Overrides default 48-hour retention

**Constants:**
- `DefaultRetention = 48 * time.Hour` (default retention period)
- `DefaultMaxMB = 512` (default buffer capacity in megabytes)
- `maxFrameBytes = 280` (maximum CSI frame size: 24-byte header + 128×2 payload)

---

## 2. Recorder Manager Package

**Location:** `mothership/internal/recorder/`

### Primary Files

| File | Purpose | Key Components |
|------|---------|----------------|
| `manager.go` | Per-link CSI frame recorder manager | `Manager`, `linkRecorder`, `Write()`, `ReadFrom()`, `AvailableRange()`, `Close()` |
| `segment.go` | Hourly segment file operations | `segmentFileName()`, `WriteRecord()`, `ScanSegment()`, `ScanSegmentFrom()`, `listSegmentFiles()` |
| `manager_test.go` | Unit tests | Recorder lifecycle tests |
| `segment_test.go` | Unit tests | Segment format tests |

### Core Data Structures

**Manager:**
```go
type Manager struct {
    mu       sync.RWMutex
    config   Config
    links    map[string]*linkRecorder  // Per-link recorders
    done     chan struct{}
    wg       sync.WaitGroup
    paused   atomic.Bool                 // Load shedding control
}
```

**Config:**
```go
type Config struct {
    DataDir         string        // Base directory (e.g., "/data/csi")
    RetentionHours  int           // Hours to retain (default: 48)
    MaxBytesPerLink int64         // Per-link size limit (default: 1GB)
    BufferSize      int           // Per-link channel capacity (default: 1000)
    CleanupInterval time.Duration // Cleanup sweep interval (default: 1 hour)
}
```

### File Organization

**Directory Structure:**
```
data/csi/
├── AA:BB:CC:DD:EE:FF-11:22:33:44:55:66/  # Link ID (MAC:MAC format)
│   ├── 20240101-00.csi                     # Hourly segment (UTC)
│   ├── 20240101-01.csi
│   └── ...
└── ZZ:YY:XX:WW:VV:UU:MM:AA:BB:CC:DD/
    └── ...
```

**Segment File Format:**
- Filename: `YYYYMMDD-HH.csi` (UTC-based hourly rotation)
- Record format per segment:
  ```
  [4-byte BE uint32: payloadLen][payload]
  payloadLen = 8 + len(rawCSIframe)
  payload = [8-byte BE int64: recvTimeNS][raw CSI frame]
  ```

### Key Operations

- **Write(linkID, frame)** - Non-blocking write with per-link buffering
- **ReadFrom(linkID, since)** - Channel-based chronological reader
- **AvailableRange(linkID)** - Returns time range (start, end) for a link
- **PauseWrites() / ResumeWrites()** - Load shedding control
- **Close()** - Graceful shutdown with flush

### Cleanup Strategy

The recorder runs a background cleanup goroutine that:
1. **Time-based pruning:** Deletes segment files older than `RetentionHours`
2. **Size-based enforcement:** Enforces `MaxBytesPerLink` by deleting oldest files
3. **Runs hourly:** Configurable via `CleanupInterval`

---

## 3. Replay Engine Package

**Location:** `mothership/internal/replay/`

### Primary Files

| File | Purpose | Key Components |
|------|---------|----------------|
| `store.go` | Recording store (legacy circular buffer) | `RecordingStore`, `Append()`, `Scan()`, `ScanRange()` |
| `engine.go` | Time-travel replay engine orchestration | `Engine`, `StartSession()`, `GetSession()`, `Seek()`, `Play()`, `Pause()`, `SetParams()` |
| `types.go` | Session types and tunable parameters | `Session`, `SessionState`, `TunableParams`, `BlobUpdate`, `BlobBroadcaster` |
| `session.go` | Session lifecycle management | `NewSession()`, playback loop, state transitions |
| `worker.go` | Playback worker goroutine | `Worker`, processing loop, blob generation |
| `pipeline.go` | Replay pipeline wrapper | Worker-pipeline interface |
| `buffer_adapter.go` | Adapter between recording.Buffer and replay | `BufferAdapter`, `FrameReader` interface |
| `verify_recording_test.go` | Recording verification tests | Integration tests |

### Core Data Structures

**Engine:**
```go
type Engine struct {
    mu               sync.RWMutex
    sessions         map[string]*Session
    buffer           *recording.Buffer
    blobBroadcaster  BlobBroadcaster
    defaultParams    *TunableParams
    sessionIDCounter uint64
}
```

**Session:**
```go
type Session struct {
    mu         sync.RWMutex
    id         string
    fromMS     int64              // Session start timestamp
    toMS       int64              // Session end timestamp
    currentMS  int64              // Current playback position
    speed      int                // Playback speed (1, 2, or 5)
    state      SessionState       // "paused" | "playing" | "stopped"
    params     *TunableParams    // Pipeline parameters
    ctx        context.Context
    cancel     context.CancelFunc
    stopCh     chan struct{}
}
```

**Tunable Parameters:**
```go
type TunableParams struct {
    DeltaRMSThreshold    *float64 `json:"delta_rms_threshold,omitempty"`
    TauS                 *float64 `json:"tau_s,omitempty"`
    FresnelDecay         *float64 `json:"fresnel_decay,omitempty"`
    NSubcarriers         *int     `json:"n_subcarriers,omitempty"`
    BreathingSensitivity *float64 `json:"breathing_sensitivity,omitempty"`
    FresnelWeightSigma   *float64 `json:"fresnel_weight_sigma,omitempty"`
    MinConfidence        *float64 `json:"min_confidence,omitempty"`
}
```

### Key Operations

**Engine:**
- **StartSession(fromMS, toMS)** - Creates new replay session, clamps to available data
- **GetSession(id)** - Retrieves session by ID
- **StopSession(id)** - Stops and removes session
- **Seek(id, targetMS)** - Seeks session to timestamp
- **Play(id, speed)** - Starts playback at 1x, 2x, or 5x speed
- **Pause(id)** - Pauses playback
- **SetParams(id, params)** - Updates tunable parameters (merges with existing)
- **GetTimestampRange()** - Returns available data range

**Session:**
- **SeekTo(targetMS)** - Moves playback position, clamps to [fromMS, toMS]
- **Play(speed)** - Starts playback goroutine
- **Pause()** - Pauses playback
- **Stop()** - Terminates session
- **SetParams(params)** - Updates pipeline parameters

### Playback Flow

1. **Session creation:** User specifies time range → Engine creates Session
2. **Worker goroutine:** `playbackLoop()` runs at 10 Hz (100ms ticks)
3. **Frame emission:** `emitFrames()` reads CSI frames from buffer for current position
4. **Pipeline processing:** Frames fed through signal processing pipeline
5. **Blob generation:** Fusion engine produces blob updates
6. **Broadcast:** Blob updates sent to dashboard via WebSocket

---

## 4. REST API Layer

**Location:** `mothership/internal/api/replay.go`

### ReplayHandler

Manages HTTP API endpoints for replay control:

**Endpoints:**
- `GET /api/replay/sessions` - List all sessions and store statistics
- `POST /api/replay/start` - Start new replay session
- `POST /api/replay/stop` - Stop a session
- `POST /api/replay/seek` - Seek to timestamp within session
- `POST /api/replay/tune` - Update pipeline parameters
- `PATCH /api/replay/params` - Partial parameter update (RESTful)
- `POST /api/replay/set-speed` - Change playback speed
- `POST /api/replay/set-state` - Change playback state (playing/paused)
- `POST /api/replay/jump-to-time` - Quick session creation + seek (dashboard UX shortcut)
- `POST /api/replay/apply-live` - Apply replay params to live pipeline
- `GET /api/replay/session/{id}` - Poll session state with blobs

**Request/Response Types:**
- `startSessionRequest` - `{from_iso8601, to_iso8601, speed}`
- `tuneRequest` - `{session_id, delta_rms_threshold?, tau_s?, ...}`
- `seekRequest` - `{session_id, timestamp_iso8601}`
- `SessionInfo` - `{id, from_ms, to_ms, current_ms, state}`

---

## 5. Integration Points

### Ingestion Server → Recording Buffer

**Location:** `mothership/internal/ingestion/server.go`

**Wiring (main.go:883-898):**
```go
buf, err := recording.NewBuffer(
    filepath.Join(cfg.DataDir, "csi_replay.bin"),
    cfg.ReplayMaxMB,
    0,  // retention from env var
    cfg.ReplayCompression,
    cfg.ReplayChunkSizeMB*1024*1024
)
adapter := replay.NewBufferAdapter(buf)
replayStore = adapter
```

**Frame Recording (server.go:738-747):**
```go
replay := s.replayStore  // ReplayAppender interface
if replay != nil && sh == nil || sh.ShouldWriteReplay() {
    if err := replay.Append(recvTime.UnixNano(), data); err != nil {
        // Log and continue
    }
}
```

### Main.go Module Initialization

**Location:** `mothership/cmd/mothership/main.go`

**Key initialization sections:**
1. **Lines 62-64:** Import recording, recorder, replay packages
2. **Lines 883-917:** Create and configure recording buffer
3. **Lines 921-931:** Create per-link recorder manager
4. **Lines 1867-1884:** Wire replay handler with dashboard hub
5. **Line 1892:** Wire replay handler with processor manager

**Configuration Flow:**
```
1. Create recording.Buffer → csi_replay.bin (360 MB default)
2. Wrap with replay.BufferAdapter → replay.FrameReader interface
3. Create api.ReplayHandler with replay.FrameReader
4. Create recorder.Manager → data/csi/ directory
5. Wire replay worker with signal processor & fusion engine
6. Register REST routes
7. Start worker goroutines
```

---

## 6. Data Flow Diagram

```
ESP32 Node → WebSocket /ws/node
                │
                ▼
Ingestion Server (binary CSI frames)
                │
                ├──────────────┐
                │              │
                ▼              ▼
        Signal Pipeline   Recording Buffer
        (live only)     (csi_replay.bin)
                           │
                           │
                           ▼
                     Replay Worker
                           │
                           ├───────────────┐
                           │               │
                           ▼               ▼
                    Time-Travel    Dashboard REST
                    Sessions       API (/api/replay/*)
```

**Live Path (no replay):**
1. ESP32 → Ingestion → Signal Pipeline → Fusion → Dashboard WebSocket

**Replay Path:**
1. Recording Buffer → Replay Worker → Signal Pipeline → Fusion → Dashboard REST/WebSocket

---

## 7. File Inventory

### Complete File Listing

**Core Implementation:**
- `mothership/internal/recording/buffer.go` - Circular buffer with compression (911 lines)
- `mothership/internal/recording/compression.go` - Zstd compression utilities (231 lines)
- `mothership/internal/recorder/manager.go` - Per-link recorder manager (374 lines)
- `mothership/internal/recorder/segment.go` - Segment file operations (175 lines)
- `mothership/internal/replay/store.go` - Recording store legacy (365 lines)
- `mothership/internal/replay/engine.go` - Replay engine orchestration (252 lines)
- `mothership/internal/replay/types.go` - Session types and parameters (896 lines)
- `mothership/internal/replay/session.go` - Session lifecycle (90 lines)
- `mothership/internal/replay/worker.go` - Playback worker goroutine
- `mothership/internal/replay/pipeline.go` - Pipeline wrapper
- `mothership/internal/replay/buffer_adapter.go` - Buffer adapter for replay
- `mothership/internal/api/replay.go` - REST API handlers (1072 lines)

**Tests:**
- `mothership/internal/recording/buffer_test.go` - Buffer unit tests
- `mothership/internal/recording/benchmark.go` - Performance benchmarks
- `mothership/internal/recorder/manager_test.go` - Recorder manager tests
- `mothership/internal/recorder/segment_test.go` - Segment tests
- `mothership/internal/replay/store_test.go` - Store tests
- `mothership/internal/replay/engine_test.go` - Engine tests
- `mothership/internal/replay/verify_recording_test.go` - Integration tests
- `mothership/internal/replay/seek_fuzz_test.go` - Fuzz tests for seek operation
- `mothership/internal/replay/pipeline_test.go` - Pipeline tests
- `mothership/internal/replay/integration_test.go` - Full replay integration tests

**Acceptance Tests:**
- `test/acceptance/as6_replay_test.go` - Replay acceptance scenarios
- `testdata/generate_csi_recording.go` - Test data generator
- `testdata/verify_recording.go` - Recording verification utility
- `mothership/test/acceptance/as6_replay_test.go` - Mothership replay tests

---

## 8. Configuration

### Environment Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `SPAXEL_RECORDING_RETENTION` | 48h | Time-based retention for CSI data |
| `SPAXEL_REPLAY_MAX_MB` | 360 | Maximum recording buffer size in MB |
| `SPAXEL_REPLAY_COMPRESSION` | false | Enable zstd compression for CSI frames |
| `SPAXEL_REPLAY_CHUNK_SIZE_MB` | 64 | Target compressed chunk size in MB |

### Data Directory Layout

```
/data/
├── csi_replay.bin          # Circular buffer (recording.Buffer)
├── csi/                     # Per-link segmented recording (recorder.Manager)
│   ├── AA:BB:CC:DD:EE:FF-11:22:33:44:55:66/
│   │   └── 20240128-12.csi
│   └── ...
├── spaxel.db                 # SQLite database
└── ...
```

---

## 9. Key Design Decisions

### Circular Buffer vs. Append-Only

**Recording.Buffer** uses a circular buffer design:
- **Pros:** Fixed file size, automatic eviction, bounded disk usage
- **Cons:** Oldest data evicted when buffer wraps (mitigated by configurable retention)

**Recorder.Manager** uses append-only hourly segments:
- **Pros:** Simple file rotation, natural time-based cleanup, per-link isolation
- **Cons:** Can grow unbounded if cleanup fails (mitigated by MaxBytesPerLink cap)

**Coexistence:** Both systems run in parallel:
- `csi_replay.bin` - Fast circular buffer for time-travel (feature focus)
- `data/csi/` - Per-link segments for long-term analysis

### Compression Strategy

Zstd compression at level 3:
- **Tradeoff:** CPU time for disk space (~8:1 ratio typical)
- **Chunking:** 64KB target size balances compression ratio vs. latency
- **Level 3:** Fast decode speed critical for replay read-heavy workload

### Time-Based vs. Size-Based Retention

Two independent retention mechanisms:
1. **Time-based:** Prunes records older than retention period (primary mechanism)
2. **Size-based:** Hard cap on total buffer size (prevents disk exhaustion)

**Rationale:** Time-based is user-facing (configure as "48h retention"), size-based is safety (prevents runaway disk growth).

---

## 10. Performance Characteristics

### Buffer Operations

- **Append:** O(1) amortized (occasional eviction burst)
- **Scan:** O(N) where N = number of frames in range
- **SeekToTimestamp:** O(N) scan in worst case (typical: <180K frames/hour at 50 Hz)
- **Prune:** O(E) where E = expired records

### Storage Estimates

**Uncompressed:**
- Frame size: ~150 bytes (24-byte header + 128×2 payload)
- Per-link rate: 30 Hz (typical)
- Storage: ~450 KB/s per link
- 8-node fleet: ~3.6 MB/s = ~13 GB/hour

**Compressed (8:1 ratio):**
- Effective rate: ~56 KB/s per link
- 8-node fleet: ~450 KB/s = ~1.6 GB/hour

**48-hour retention (uncompressed):** ~624 GB for 8-node fleet  
**48-hour retention (compressed):** ~78 GB for 8-node fleet

### Replay Latency

- **Session creation:** O(1) if data available, else error
- **Seek:** O(N) where N = frames between oldest and target
- **Frame emission:** 100ms tick (10 Hz playback)
- **Pipeline processing:** ~5-15 ms per frame (live pipeline budget)

---

## 11. Dependencies

### Internal Packages

- `recording.Buffer` depends only on standard library
- `recorder.Manager` depends only on standard library
- `replay.Engine` depends on:
  - `recording.Buffer` (via adapter interface)
  - Signal processing pipeline (for blob generation)
  - Localization engine (for blob generation)

### External Dependencies

- **github.com/klauspost/compress/zstd** - Zstd compression
- **github.com/go-chi/chi/v5** - HTTP routing (API layer)

---

## 12. Testing Strategy

### Unit Tests

- `buffer_test.go` - Circular buffer operations, compression correctness
- `store_test.go` - RecordingStore append/scan operations
- `manager_test.go` - Recorder lifecycle, segment rotation
- `segment_test.go` - Segment file format validation

### Integration Tests

- `verify_recording_test.go` - End-to-end recording verification
- `integration_test.go` - Full replay pipeline integration
- `as6_replay_test.go` - Acceptance test scenarios

### Fuzz Tests

- `seek_fuzz_test.go` - Property-based testing for seek operation
  - Random timestamps within valid range
  - Edge cases: before oldest, after newest, exact match

---

## 13. Module Boundaries

### What CSI Recording Does NOT Include

**Separate subsystems:**
- **CSI capture firmware** (`firmware/main/csi.c`) - Hardware-level CSI extraction
- **Binary frame parsing** (`mothership/internal/ingestion`) - WebSocket protocol handling
- **Signal processing pipeline** (`mothership/internal/pipeline/`) - Phase sanitization, deltaRMS, NBVI
- **Fusion engine** (`mothership/internal/fusion/`) - Blob generation from motion features
- **Dashboard WebSocket** - Frontend replay visualization

**CSI recording is the storage layer ONLY:**
- Takes raw CSI frames from ingestion
- Writes to disk in compressed/uncompressed format
- Reads back for replay
- Does NOT process, analyze, or visualize

### What Uses CSI Recording

**Consumers:**
1. **Replay Worker** - Primary consumer for time-travel debugging
2. **API ReplayHandler** - REST API for replay control
3. **Dashboard** (indirect) - Via ReplayHandler WebSocket

---

## 14. Operational Characteristics

### Startup Sequence

1. Open/create `csi_replay.bin` (read existing header or initialize new)
2. Create per-link recorder manager in `data/csi/`
3. Start background cleanup goroutine (hourly)
4. Wire replay buffer to ingestion server
5. Register REST API endpoints
6. Start replay worker goroutines

### Graceful Shutdown

1. Stop accepting new CSI frames
2. Flush any pending compressed data
3. Close recording buffer file
4. Stop replay worker
5. Close recorder manager channels
6. Stop cleanup goroutine

### Load Shedding

When system under load (`/api/healthz` degraded):
1. **Level 1:** Suspend CSI replay buffer writes (call `PauseWrites()`)
2. **Level 2:** (handled by ingestion) - drop CSI frames at source

### Error Handling

**Corrupted record recovery:**
- Truncate to last known good state
- Reset oldestPos/wrapPos if corruption detected
- Log warning and continue

**Disk full:**
- Log error and drop frames
- Alert via dashboard (not yet implemented)
- Continue operation with degraded functionality

---

## 15. Future Enhancements

### Planned (per plan.md):

- **Phase 8:** Time-travel debugging UI integration
- **Parameter tuning overlay:** Slider-based adjustment of pipeline parameters during replay
- **"Apply to Live" button:** Copy replay parameters to live configuration
- **Timeline navigation:** Scrub back through historical CSI data

### Potential Improvements:

- **Async compression:** Move compression to background goroutine (reduce ingestion latency)
- **Index file:** Accelerate SeekToTimestamp with O(log N) timestamp index
- **Deduplication:** Store only delta frames if CSI frame repeats detected
- **Multi-rate recording:** Adaptive frame rate based on activity level

---

## 16. Related Documentation

**Plan References:**
- Component 14: Time-Travel Debugging (docs/plan/plan.md)
- CSI Recording section in REST API Specification
- Database schema (if applicable)

**Related Beads:**
- Time-travel debugging implementation beads
- Replay API integration beads
- Dashboard replay UI beads

**See Also:**
- `docs/notes/replay-architecture.md` (if exists)
- `docs/research/csi-format-spec.md` (if exists)
- `firmware/main/csi.c` - CSI capture firmware

---

## Summary

The CSI recording module is a well-structured, multi-layer system that enables time-travel debugging for WiFi CSI data. The architecture cleanly separates concerns:

1. **Storage layer** (recording.Buffer, recorder.Manager) - Disk I/O and retention
2. **Replay engine** (replay package) - Session management and playback orchestration
3. **API layer** (api/replay.go) - REST endpoints for user control
4. **Integration points** - Wiring in main.go and ingestion server

The module is production-ready with compression, configurable retention, graceful shutdown, and load shedding. The circular buffer design provides bounded memory usage while the segment-based recorder provides long-term historical analysis.
