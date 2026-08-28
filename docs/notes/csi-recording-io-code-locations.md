# CSI Recording I/O Code Locations and Data Structures

**Last updated:** 2026-08-28  
**Purpose:** Catalog all code responsible for CSI recording I/O operations, data structures, and serialization paths.

---

## Overview

Spaxel's CSI recording system provides efficient disk-backed circular buffering with optional compression, supporting both live streaming and time-travel replay capabilities. The system consists of:

1. **Binary file format** with append-only circular buffer
2. **Recording buffer** for real-time CSI frame writes
3. **Replay store** for time-travel debugging
4. **Frame parsing** and validation
5. **Compression** for space optimization

---

## Binary File Format

### File Header (32 bytes)

**Defined in:** `mothership/internal/recording/buffer.go`

```go
// Header structure (total 32 bytes)
magic[8]      // "SPAXLREC" or "SPAXLREP" 
writePos[8]   // uint64 LE — absolute file offset of next write
oldestPos[8]   // uint64 LE — absolute file offset of oldest valid record (0 = empty)
wrapPos[8]     // uint64 LE — writePos at last wrap point (0 = no pending wrap)
reserved[8]    // zeroed, reserved for future use
```

### Per-Record Format (10 + frameLen bytes)

```
recvTimeNS[8]   // int64 LE — Unix nanosecond receive timestamp
frameLen[2]     // uint16 LE — length of following frame bytes
frameData[N]    // raw CSI frame bytes (WebSocket binary frame format)
```

### CSI Frame Structure (within frameData)

**Parsed in:** `mothership/internal/ingestion/frame.go`

```
Header (24 bytes):
  node_mac[6]     // source node MAC
  peer_mac[6]     // transmitting peer MAC  
  timestamp_us[8] // uint64, microseconds since node boot
  rssi[1]         // int8, dBm
  noise_floor[1]  // int8, dBm
  channel[1]      // uint8, WiFi channel
  n_sub[1]       // uint8, subcarrier count

Payload (n_sub × 2 bytes):
  Interleaved int8 I,Q pairs for each subcarrier
```

**File location:** `/data/csi_replay.bin`

---

## Read/Write Entry Points

### Recording Buffer (Live I/O)

**Location:** `mothership/internal/recording/buffer.go`

#### Write Operations

1. **`Append(recvTimeNS int64, rawFrame []byte) error`**
   - Main write entry point
   - Supports both compressed and uncompressed modes
   - Handles circular buffer wrapping
   - Enforces retention limits via `pruneOlderThan()`

2. **`appendUncompressed(recvTimeNS int64, rawFrame []byte) error`**
   - Legacy format (one frame per record)
   - Direct write without compression

3. **`appendCompressed(recvTimeNS int64, rawFrame []byte) error`**
   - Chunked zstd compression
   - Batches frames into 64 KB chunks
   - Estimated 8:1 compression ratio

#### Read Operations

1. **`Scan(fn func(recvTimeNS int64, frame []byte) bool) error`**
   - Chronological scan of all records
   - Callback receives timestamp and raw frame bytes
   - Continues until callback returns false or EOF

2. **`ScanRange(from, to time.Time, fn func(recvTimeNS int64, frame []byte) bool) error`**
   - Time-range filtered scan
   - Seeks to start timestamp, scans until end timestamp
   - Handles wrap-around at end of file

3. **`SeekToTimestamp(target time.Time) ([]byte, int64, error)`**
   - Binary search for frame closest to target timestamp
   - Returns frame bytes and file position
   - Used by time-travel replay scrubbing

4. **`GetTimestampRange() (oldest, newest time.Time, err)`**
   - Returns time range of available data
   - Reads first and last record timestamps
   - Used for replay UI timeline bounds

#### Buffer Management

- **`pruneOlderThan(cutoff int64)`** - Removes records beyond retention period
- **`syncHeader()`** - Updates file header with current write positions
- **`wrapWritePos()`** - Handles circular buffer wrap-around
- **`Close() error`** - Flushes pending data and closes file

---

### Replay Store (Time-Travel)

**Location:** `mothership/internal/replay/store.go`

#### Interface

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

#### Operations

- **`Open(path string) (*RecordingStore, error)`** - Opens existing recording file
- **`ReadAt(pos int64) ([]byte, error)`** - Read frame at file position
- **`SeekToTimestamp(target time.Time) ([]byte, int64, error)`** - Same as Buffer
- **`Close() error`** - Close file

#### Replay Worker Integration

**Location:** `mothership/internal/replay/worker.go`

- Processes recorded frames through separate signal pipeline
- Supports variable-speed playback (1x, 2x, 5x)
- Implements seek, play/pause controls
- Generates blob updates for dashboard visualization

---

## Frame Parsing and Validation

**Location:** `mothership/internal/ingestion/frame.go`

### Entry Point

```go
func ParseFrame(data []byte) (*CSIFrame, error)
```

### CSIFrame Structure

```go
type CSIFrame struct {
    NodeMAC      []byte  // 6 bytes
    PeerMAC      []byte  // 6 bytes
    TimestampUS  uint64  // microseconds since boot
    RSSI         int8    // dBm
    NoiseFloor   int8    // dBm
    Channel      uint8   // WiFi channel
    NSub         uint8   // subcarrier count
    IQPairs      []complex64  // parsed I/Q values
}
```

### Validation Rules

1. **Minimum length check:** `len(frame) < 24` → reject
2. **Payload length check:** `24 + n_sub×2 != len(frame)` → reject
3. **Subcarrier count check:** `n_sub > 128` → reject
4. **Channel validation:** `channel == 0 or > 14` → reject
5. **RSSI zero handling:** `rssi == 0` allowed (invalid RSSI per spec)

### Error Handling

- Frames failing validation are silently dropped at DEBUG level
- Per-connection malformed frame counter tracked
- >100 malformed/minute → WARN log
- >1000 malformed/minute → connection closed

---

## Compression Support

**Location:** `mothership/internal/recording/compression.go`

### Chunk Header Structure

```go
type ChunkHeader struct {
    Magic           [4]byte  // "CSCZ" (Compressed Spaxel Chunk)
    Version         byte     // 1
    FrameCount      uint16   // number of frames in chunk
    UncompressedLen uint32   // total size of uncompressed data
}
```

### Compressor

```go
type Compressor struct {
    chunkSize   int           // target chunk size (default 64 KB)
    pending     []byte        // accumulated frames
    frameCount  int           // frames in current chunk
    level       int           // zstd compression level (default 3)
}
```

#### Operations

- **`AddFrame(recvTimeNS int64, frame []byte) error`** - Add frame to chunk
- **`Flush() ([]byte, error)`** - Compress and return chunk
- **`Reset()`** - Clear pending data

### Decompressor

```go
type Decompressor struct {
    reader *zstd.Decoder
}
```

#### Operations

- **`Decompress(data []byte) ([]byte, error)`** - Decompress chunk
- **`ScanChunk(data []byte, fn func(recvTimeNS int64, frame []byte) bool) error`** - Iterate frames in chunk

---

## Firmware CSI Code

**Location:** `firmware/main/csi.c`, `csi.h`

### CSI Callback

```c
void wifi_csi_cb(void *ctx, wifi_csi_info_t *info)
```

- Called by ESP32 WiFi stack for each CSI frame
- Extracts I/Q pairs from CSI matrix
- Validates frame structure
- Queues frame for processing

### Processing Task

```c
void csi_rx_task(void *arg)
```

- FreeRTOS task that processes CSI queue
- Performs on-device variance computation
- Sends motion hints if variance exceeds threshold
- Transmits frames via WebSocket

### Constants

- **`SPAXEL_CSI_QUEUE_SIZE`** - Queue size for CSI frame buffering
- **`SPAXEL_CSI_MAX_SUBCARRIERS`** - Maximum subcarrier limit (128)
- Variance window: 100 samples for on-device motion detection

---

## Key Configuration Constants

**Location:** `mothership/internal/recording/buffer.go`

| Constant | Value | Description |
|----------|-------|-------------|
| `DefaultMaxMB` | 360 | Max file size in MB (~48h at 20 Hz, 20 links) |
| `DefaultRetention` | 48h | Time-based retention period |
| `maxFrameBytes` | 280 | Max frame size (24-byte header + 128×2 payload) |
| `DefaultChunkSize` | 64 KB | Target compression chunk size |
| `CompressionLevel` | 3 | zstd compression level (fast decompression) |

### Environment Variables

- `SPAXEL_REPLAY_MAX_MB` - Override max file size
- `SPAXEL_REPLAY_RETAIN_H` - Override retention period

---

## Serialization Code Paths

### Write Path (Live Recording)

```
Ingestion Server (WebSocket)
  → ParseFrame() validation
  → Buffer.Append()
    → appendCompressed() [if compression enabled]
      → Compressor.AddFrame()
      → Compressor.Flush() [when chunk full]
        → zstd compression
        → write to file with ChunkHeader
    → syncHeader()
      → update file header positions
      → pruneOlderThan() [if needed]
```

### Read Path (Replay/Scan)

```
Replay Worker / Scan Request
  → Buffer.SeekToTimestamp() or Buffer.Scan()
    → binary search for target position
    → read record header (10 bytes)
    → read frame data (frameLen bytes)
  → ParseFrame() [if parsed frame needed]
  → Signal processing pipeline
```

### Decompression Path

```
Read from file
  → Detect chunk magic ("CSCZ")
  → Read ChunkHeader
  → Decompressor.Decompress()
    → zstd decompression
  → ScanChunk()
    → Iterate over individual frame records
    → Extract recvTimeNS and frame bytes
```

---

## Test Data Generation

**Location:** `testdata/generate_csi_recording.go`

### Generator Tool

Generates realistic CSI recordings for testing:

```bash
go run testdata/generate_csi_recording.go -o test.bin
```

Features:
- Creates proper binary file format with headers
- Simulates idle/walking patterns
- Body absorption effects on RSSI
- Configurable activity levels and sampling rates

### Verification Tool

**Location:** `testdata/verify_recording.go`

```bash
go run testdata/verify_recording.go test.bin
```

Validates:
- Header integrity
- Frame structure
- Statistics (duration, amplitude, frame counts)

---

## Integration Points

### Ingestion Server

**Location:** `mothership/internal/ingestion/server.go`

- Receives WebSocket binary frames from nodes
- Calls `ParseFrame()` for validation
- Calls `Buffer.Append()` to write to recording buffer
- Passes frames to signal processing pipeline

### Replay Engine

**Location:** `mothership/internal/replay/engine.go`

- Manages replay sessions
- Coordinates between recording buffer and pipeline
- Supports parameter tuning during replay
- Time-travel debugging interface

### Buffer Adapter

**Location:** `mothership/internal/replay/buffer_adapter.go`

- Wraps `recording.Buffer` to implement `FrameReader` interface
- Enables replay worker to read from same buffer used for live recording
- Abstracts storage backend from replay logic

---

## Performance Characteristics

### Write Performance

- **Throughput:** ~600 frames/second (20 Hz × 30 links)
- **Latency:** < 1 ms per frame append
- **Compression overhead:** ~2-3 ms per 64 KB chunk
- **File I/O:** Sequential writes, optimized for SSD

### Read Performance

- **Sequential scan:** ~100 MB/s (uncompressed), ~300 MB/s (decompressed)
- **Random seek:** O(N) linear scan from oldest position (acceptable for infrequent seeks)
- **Decompression:** ~0.5 ms per 64 KB chunk

### Storage Efficiency

- **Uncompressed:** ~150 bytes/frame → ~7.5 MB/hour (20 Hz, 20 links)
- **Compressed:** ~8:1 ratio → ~0.9 MB/hour
- **360 MB file:** ~48 hours at 20 Hz, 20 links (compressed)

---

## Error Handling and Recovery

### File Corruption Recovery

On open, if file header magic doesn't match:
1. Scan backward from `writePos` for last complete frame
2. Truncate file at that position
3. Log warning about corrupted data
4. Resume recording from valid position

### Graceful Shutdown

On shutdown:
1. Flush compression buffer
2. Sync file header
3. Close file descriptor
4. Data integrity guaranteed by OS fsync

### Crash Recovery

On ungraceful shutdown:
- Last write may be incomplete (uncompressed header only)
- Recovery scans for last complete frame
- At most one frame lost per crash
- Replay automatically skips incomplete records

---

## Future Enhancements

### Planned

1. **Sparse indexing** - O(1) seeks via timestamp index file
2. **Parallel compression** - Use multiple compressor cores
3. **Delta encoding** - Compress I/Q deltas instead of raw values
4. **Adaptive compression level** - Adjust based on disk space

### Considered

- **Memory-mapped I/O** - Rejected for complexity vs. benefit
- **Separate index file** - Deferred until replay seek performance becomes bottleneck
- **Variable bit-rate encoding** - Not worth added complexity for CSI data

---

## References

- **Plan document:** `docs/plan/plan.md` - Component 14: Time-Travel Debugging
- **Schema documentation:** `docs/plan/plan.md` - SQLite Schema > CSI Replay Store
- **Testing strategy:** `docs/plan/plan.md` - Testing Strategy > Unit Tests
