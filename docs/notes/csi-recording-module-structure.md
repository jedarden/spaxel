# CSI Recording Module Structure

**Date:** 2026-08-28  
**Purpose:** Document the complete CSI recording module architecture, file organization, and data flow for the Spaxel indoor positioning system.

---

## Module Overview

The CSI recording system consists of three main components:

1. **Firmware CSI Capture** (ESP32-S3) - Raw CSI data extraction from WiFi packets
2. **Mothership Recording** (Go) - Disk-backed circular buffer with time-based retention
3. **Replay System** (Go) - Time-travel debugging with parameter tuning

---

## File Organization

### Firmware (ESP32-S3)

**Location:** `/home/coding/spaxel/firmware/main/`

| File | Purpose | Key Functions |
|------|---------|---------------|
| `csi.c` | CSI capture implementation | `csi_init()`, `csi_wifi_start()`, `csi_set_role()`, `wifi_csi_cb()` |
| `csi.h` | CSI subsystem interface | Role configuration, TX/RX control, statistics |

**Key Components:**
- **CSI Queue:** FreeRTOS queue for CSI data (`SPAXEL_CSI_QUEUE_SIZE`)
- **RX Task:** Processes CSI callbacks from ESP-IDF (`csi_rx_task`)
- **TX Task:** Generates probe packets for active sensing (`csi_tx_task`)
- **Variance Detection:** On-device motion hint computation (`compute_amplitude_variance()`)
- **Binary Frame Serialization:** WebSocket binary format (24-byte header + I/Q payload)

**Binary Frame Format:**
```
Header (24 bytes):
  node_mac:     6 bytes    — source node MAC
  peer_mac:     6 bytes    — transmitting node MAC (RX mode)
  timestamp_us: 8 bytes    — microseconds since boot (uint64 LE)
  rssi:         1 byte     — signed RSSI in dBm (int8)
  noise_floor:  1 byte     — signed noise floor in dBm (int8)
  channel:      1 byte     — WiFi channel number (uint8)
  n_sub:        1 byte     — number of subcarriers (uint8)

Payload (n_sub * 2 bytes):
  Per subcarrier: int8 I, int8 Q
```

---

### Mothership Recording Module

**Location:** `/home/coding/spaxel/mothership/internal/recording/`

| File | Purpose | Key Types/Functions |
|------|---------|---------------------|
| `buffer.go` | Core circular buffer implementation | `Buffer`, `NewBuffer()`, `WriteFrame()`, `ReadFrames()` |
| `buffer_test.go` | Buffer unit tests | Table-driven tests for write/read/eviction |
| `benchmark.go` | Performance benchmarks | `BenchmarkWriteFrame()`, compression benchmarks |
| `compression.go` | Zstandard compression | `Compressor`, `CompressFrame()`, `DecompressFrame()` |

**Binary File Layout:**
```
Header (32 bytes):
  magic[8]      "SPAXLREC"
  writePos[8]   uint64 LE — next write position
  oldestPos[8]  uint64 LE — oldest valid record (0 = empty)
  wrapPos[8]    uint64 LE — write position at last wrap

Record (10 + frameLen bytes):
  recvTimeNS[8]  int64 LE  — Unix nanosecond timestamp
  frameLen[2]    uint16 LE — frame byte length
  frameData[N]   raw CSI frame bytes
```

**Configuration:**
- **Default Capacity:** 512 MB (`DefaultMaxMB`)
- **Default Retention:** 48 hours (`DefaultRetention`)
- **Environment Variable:** `SPAXEL_RECORDING_RETENTION` (e.g., "24h", "72h")
- **Compression:** Optional zstd compression (`enableCompression` parameter)

**Key Features:**
- **Thread-safe:** Mutex-protected concurrent access
- **Circular eviction:** Oldest records evicted when buffer full or retention exceeded
- **Time-based pruning:** Records older than retention period auto-evicted
- **Crash recovery:** File header recovery on restart

---

### Mothership Recorder Module

**Location:** `/home/coding/spaxel/mothership/internal/recorder/`

| File | Purpose | Key Types/Functions |
|------|---------|---------------------|
| `manager.go` | Per-link recording manager | `Manager`, `NewManager()`, `RecordFrame()`, `Shutdown()` |
| `segment.go` | Time-based segment management | `Segment`, `OpenSegment()`, `CloseSegment()` |
| `manager_test.go` | Manager unit tests | Concurrent recording tests |
| `segment_test.go` | Segment unit tests | File lifecycle tests |

**Configuration:**
```go
type Config struct {
    DataDir         string        // Base directory for segment files
    RetentionHours  int           // Hours to retain segments (default: 48)
    MaxBytesPerLink int64         // Max bytes per link (default: 1 GB)
    BufferSize      int           // Per-link channel capacity (default: 1000)
    CleanupInterval time.Duration // Cleanup sweep interval (default: 1 hour)
}
```

**Architecture:**
- **Per-link channels:** Each TX→RX link gets a dedicated `linkRecorder`
- **Segment rotation:** Creates new time-based segment files (e.g., `link_20260828_120000.bin`)
- **Automatic cleanup:** Background goroutine removes old segments
- **Pause control:** `atomic.Bool` for pause/resume during load shedding

---

### Mothership Replay Module

**Location:** `/home/coding/spaxel/mothership/internal/replay/`

| File | Purpose | Key Types/Functions |
|------|---------|---------------------|
| `store.go` | Replay store implementation | `RecordingStore`, `NewRecordingStore()`, `AppendFrame()` |
| `worker.go` | Replay session worker | `replayWorker`, session state machine |
| `engine.go` | Replay pipeline engine | `Engine`, `Start()`, `SeekTo()`, `SetParams()` |
| `pipeline.go` | Signal processing pipeline | `Pipeline`, ProcessFrame() |
| `session.go` | Replay session management | `Session`, state tracking |
| `types.go` | Replay type definitions | API request/response structs |

**File Layout:**
```
Header (32 bytes):
  magic[8]     "SPAXLREP"
  writePos[8]  uint64 LE — next write position
  oldestPos[8] uint64 LE — oldest valid record
  wrapPos[8]   uint64 LE — write position at last wrap

Record (10 + frameLen bytes):
  recvTimeNS[8] int64 LE — Unix nanosecond timestamp
  frameLen[2]   uint16 LE — frame byte length
  frameData[N]  raw CSI frame bytes
```

**Capabilities:**
- **Time-travel:** Seek to arbitrary timestamp in recorded history
- **Parameter tuning:** Adjust pipeline parameters (thresholds, weights) during replay
- **Speed control:** Playback at 1×, 2×, 5×, or frame-by-frame
- **Session management:** Multiple concurrent replay sessions

---

### Ingestion Server Integration

**Location:** `/home/coding/spaxel/mothership/internal/ingestion/`

| File | Purpose | CSI Recording Integration |
|------|---------|---------------------------|
| `server.go` | WebSocket server for node connections | `SetReplayStore()`, `SetRecorder()` |
| `frametracker.go` | Frame tracking and statistics | `RecordFrameArrival()` |

**Integration Points:**
1. **CSI Frame Reception:** WebSocket binary frames parsed from ESP32 nodes
2. **Recording Path:** `recorder.RecordFrame()` called for each valid CSI frame
3. **Replay Path:** `replayStore.AppendFrame()` writes to replay buffer
4. **Statistics:** Frame tracking via `frameTracker.RecordFrame()`

**Server Wiring:**
```go
type Server struct {
    replayStore ReplayAppender  // Disk-backed recording store
    recorder    Recorder         // Per-link frame recorder
    // ...
}
```

---

## Data Flow

### Recording Path

```
ESP32 Node CSI Capture
    ↓ (WebSocket binary frame)
Ingestion Server
    ↓ ParseBinaryFrame()
Recorder Manager
    ↓ per-link channel
Recording Buffer
    ↓ WriteFrame()
Disk File (csi_recording.bin)
```

### Replay Path

```
User Request (POST /api/replay/start)
    ↓
Replay Engine
    ↓ SeekTo()
Replay Store
    ↓ ReadFrames()
Signal Processing Pipeline
    ↓ ProcessFrame()
Dashboard WebSocket
    ↓ {blobs, timestamp_ms}
Browser UI
```

---

## Module Boundaries

### Firmware Boundary
- **CSI callback** runs in ESP-IDF WiFi task context
- **Serialization** happens in dedicated CSI RX task (Core 1)
- **Transmission** via WebSocket (gorilla/websocket)

### Mothership Boundary  
- **Ingestion** handles WebSocket protocol and frame validation
- **Recording** is passive append-only disk writer
- **Replay** is isolated pipeline with no live operation interference

### Storage Boundary
- **Recording buffer** is append-only circular buffer
- **Replay store** is separate append-only circular buffer
- **Segment files** are time-based rotated files

---

## Configuration Points

### Environment Variables
- `SPAXEL_RECORDING_RETENTION` - Time-based retention (default: 48h)
- `SPAXEL_REPLAY_MAX_MB` - Replay buffer capacity (default: 360 MB)

### Code Configuration
- **Recording:** `DefaultMaxMB = 512` (MB), `DefaultRetention = 48h`
- **Replay:** `DefaultMaxMB = 360` (MB)
- **Recorder:** `RetentionHours = 48`, `MaxBytesPerLink = 1GB`

---

## Testing Infrastructure

### Unit Tests
- `buffer_test.go` - Circular buffer operations
- `store_test.go` - Replay store operations
- `manager_test.go` - Concurrent recording
- `segment_test.go` - File lifecycle

### Integration Tests  
- `integration_test.go` - Full pipeline replay
- `seek_fuzz_test.go` - Fuzz testing for seek operations

### Test Data
- `/home/coding/spaxel/testdata/generate_csi_recording.go` - Test recording generator
- `/home/coding/spaxel/testdata/verify_recording.go` - Recording verification

---

## Dependencies

### Firmware Dependencies
- ESP-IDF 5.2.x WiFi CSI API
- FreeRTOS queues and tasks
- ESP WebSocket client

### Mothership Dependencies  
- `gorilla/websocket` - WebSocket protocol
- `modernc.org/sqlite` - Metadata storage
- Standard library: `encoding/binary`, `os`, `sync`, `time`

---

## Performance Characteristics

### Write Performance
- **Target:** 600 frames/second (30 Hz × 20 links typical fleet)
- **Operation:** Append-only with circular eviction
- **Contention:** Mutex-protected concurrent access

### Storage Requirements  
- **8-node fleet (28 links):** ~360 MB / 48 hours
- **Single link:** ~7.5 MB / hour at 20 Hz
- **Compression:** Zstd reduces to ~30-40% of raw size

### Replay Performance
- **Seek speed:** O(N) linear scan from oldest position
- **Playback:** Real-time with configurable speed multiplier
- **Memory:** Minimal in-memory buffering

---

## Architecture Decisions

### Circular Buffer Design
**Rationale:** CSI data is high-volume time-series data where:
- Recent data is most valuable (time-travel debugging)
- Old data auto-expires based on time/size limits
- Append-only pattern prevents fragmentation

### Separate Recording and Replay
**Rationale:** 
- **Isolation:** Replay failures don't affect live recording
- **Independent retention:** Different retention policies (48h vs configurable)
- **Performance:** No read/write contention in hot path

### Per-Link Recording
**Rationale:**
- **Granular retention:** Can retain different periods per link
- **Independent failure:** One link failure doesn't corrupt entire buffer
- **Debugging:** Can inspect specific link behavior

---

## File Size Estimates

### Recording Buffer
- **Minimum:** 50 MB (2-node fleet, 24h retention)
- **Typical:** 360 MB (8-node fleet, 48h retention, 20 Hz)
- **Maximum:** 512 MB (configurable limit)

### Replay Store  
- **Default:** 360 MB (~48h at 8-node fleet)
- **Configurable:** Via `SPAXEL_REPLAY_MAX_MB`

### Segment Files
- **Per link:** Up to 1 GB (default `MaxBytesPerLink`)
- **Cleanup:** Automatic hourly sweep removes old segments

---

## Key Interfaces

### Recording Module
```go
type Buffer interface {
    WriteFrame(recvTime time.Time, data []byte) error
    ReadFrames(from, to time.Time) (<-chan Frame, error)
    Close() error
}
```

### Recorder Module
```go
type Recorder interface {
    RecordFrame(linkID string, recvTime time.Time, data []byte) error
    Pause() error
    Resume() error
    Shutdown() error
}
```

### Replay Module
```go
type ReplayAppender interface {
    AppendFrame(recvTime time.Time, data []byte) error
}

type RecordingStore interface {
    AppendFrame(recvTime time.Time, data []byte) error
    SeekTo(target time.Time) (time.Time, error)
    ReadFrames(count int) ([]Frame, error)
}
```

---

## Operational Considerations

### Disk Space Management
- **Primary mechanism:** Time-based retention (environment variable)
- **Secondary mechanism:** Size-based circular eviction
- **Alert threshold:** < 100 MB free triggers warning

### Crash Recovery
- **File header:** Write position persisted to disk
- **Truncation recovery:** Incomplete final writes detected and removed
- **Startup:** Buffer reopens and validates file header

### Performance Monitoring
- **Write rate:** Frames per second per link
- **Buffer health:** Write position vs. oldest position gap
- **Disk usage:** Monitor `/data` filesystem free space

---

## Future Extensions

### Compression
- **Current:** Optional zstd compression in recording buffer
- **Potential:** GPU-accelerated compression for high-rate fleets

### Distributed Recording
- **Current:** Single mothership recording
- **Potential:** Shard recording across multiple storage nodes

### Real-time Analytics
- **Current:** Post-hoc replay analysis
- **Potential:** On-the-fly anomaly detection during recording

---

## Related Documentation

- **CSI Format:** `csi-format-examples.md`, `csi-format-validation-notes.md`
- **Recording I/O:** `csi-recording-io-code-locations.md`, `csi-recording-file-format.md`
- **Architecture:** `docs/plan/plan.md` (Component 14: Time-Travel Debugging)

---

**Maintenance:** This document should be updated when:
- New files are added to CSI recording modules
- File format changes occur
- Performance characteristics change significantly
- New configuration options are added
