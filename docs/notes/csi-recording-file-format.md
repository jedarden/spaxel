# CSI Recording File Format

## Overview

Spaxel stores raw CSI (Channel State Information) frames in an **append-only circular buffer** on disk at `/data/csi_replay.bin`. This binary format is optimized for high write rates (~30 Hz × 20 links = 600 frames/second) and enables time-travel replay for debugging and algorithm tuning.

**File path:** `/data/csi_replay.bin` (configurable via `SPAXEL_REPLAY_MAX_MB` env var)

**Default retention:** 360 MB (~48 hours at 20 Hz with 8 nodes)

**Encoding:** All multi-byte integers are **little-endian** (LSB first)

---

## File Header (32 bytes)

The file begins with a fixed 32-byte header that stores metadata and circular buffer state:

| Offset | Size | Type | Description |
|--------|------|------|-------------|
| 0 | 8 | char[8] | Magic number: `"SPAXLREP"` (ASCII, null-terminated) |
| 8 | 8 | uint64 LE | `writePos` — Byte offset of next write position |
| 16 | 8 | uint64 LE | `oldestPos` — Byte offset of oldest valid record (0 = empty) |
| 24 | 8 | uint64 LE | `wrapPos` — Write position at last wrap (0 = no wrap yet) |

### Header Fields Explained

- **`writePos`**: Points to where the next record will be written. Always ≥ 32 (after header).
- **`oldestPos`**: Points to the oldest valid record in the buffer. When `oldestPos == 0`, the buffer is empty.
- **`wrapPos`**: When `writePos` reaches the end of the file, it wraps back to byte 32 (start of data area). `wrapPos` stores the position where wrapping occurred, allowing the scanner to jump from the end of the right-side arc to the beginning of the left-side arc.

### Wrap Behavior

The circular buffer has two "arcs":
- **Right arc:** From byte 32 to `wrapPos` (older data)
- **Left arc:** From byte 32 to `writePos` (newer data)

When `writePos` would exceed the file size, it wraps to byte 32 and `wrapPos` is set. Old records are evicted as needed when new writes would overlap them.

---

## Per-Frame Record Structure (variable length)

Each CSI frame is stored as a record with a 10-byte prefix followed by the raw frame bytes:

```
┌─────────────────────────────────────────────────────────────┐
│ recvTimeNS  (8 bytes, int64 LE) — Unix nanosecond timestamp │
├─────────────────────────────────────────────────────────────┤
│ frameLen    (2 bytes, uint16 LE) — Frame byte count         │
├─────────────────────────────────────────────────────────────┤
│ frameData   (N bytes) — Raw CSI binary frame from node        │
└─────────────────────────────────────────────────────────────┘
```

### Record Fields

- **`recvTimeNS`**: Unix timestamp in nanoseconds when the mothership received this frame. Used for replay seek operations and timeline navigation.
- **`frameLen`**: Number of bytes in `frameData` (typically 152 bytes for 64 subcarriers). Range: 24–280 bytes.
- **`frameData`**: Raw CSI frame exactly as received from the ESP32 node via WebSocket (see CSI Frame Format below).

**Total record size:** `10 + frameLen` bytes

---

## CSI Frame Format (within `frameData`)

The `frameData` field contains the CSI frame exactly as sent by the ESP32-S3 node over WebSocket. This is a 24-byte header followed by N×2 bytes of I/Q payload.

### CSI Frame Header (24 bytes)

| Offset | Size | Type | Description |
|--------|------|------|-------------|
| 0 | 6 | uint8[6] | `node_mac` — Source node MAC address (AA:BB:CC:DD:EE:FF) |
| 6 | 6 | uint8[6] | `peer_mac` — Transmitting peer MAC (TX node or AP) |
| 12 | 8 | uint64 LE | `timestamp_us` — Microseconds since node boot (esp_timer_get_time()) |
| 20 | 1 | int8 | `rssi` — Received signal strength in dBm (typically -30 to -90) |
| 21 | 1 | int8 | `noise_floor` — Noise floor in dBm |
| 22 | 1 | uint8 | `channel` — WiFi channel (1–14 for 2.4 GHz) |
| 23 | 1 | uint8 | `n_sub` — Number of subcarriers in payload (typically 64) |

### CSI Frame Payload (n_sub × 2 bytes)

Following the header are `n_sub` pairs of signed 8-bit integers representing in-phase (I) and quadrature (Q) components for each subcarrier:

```
Offset 24:  I₀  (int8)  — In-phase component for subcarrier 0
Offset 25:  Q₀  (int8)  — Quadrature component for subcarrier 0
Offset 26:  I₁  (int8)  — In-phase component for subcarrier 1
Offset 27:  Q₁  (int8)  — Quadrature component for subcarrier 1
...
Offset 24+2k:  I_k  (int8)
Offset 24+2k+1: Q_k  (int8)
...
```

**Total CSI frame size:** `24 + (n_sub × 2)` bytes

**For 64 subcarriers:** 24 + 128 = **152 bytes**

### Amplitude and Phase Representation

The CSI data is stored as **raw Cartesian I/Q components**, not as amplitude/phase:

- **I (in-phase)**: Real part of complex signal
- **Q (quadrature)**: Imaginary part of complex signal

**Conversion to amplitude/phase** (done in signal processing pipeline):
```go
amplitude[k] = sqrt(I²[k] + Q²[k])
phase[k]     = atan2(Q[k], I[k])  // radians, range [-π, π]
```

**Note:** I and Q values are signed 8-bit integers (`int8`), range -128 to +127. Typical values are much smaller (±50). Large values near ±128 indicate very strong signals or saturation.

### Subcarrier Layout

For **HT20 (802.11n 20 MHz)** mode with 64 total subcarriers:
- **Indices 0–63**: All subcarriers (including nulls, guards, pilots, and data)
- **Data subcarriers (47)**: Used for motion detection — indices excluding nulls/guards/pilots
- **Pilots (4)**: Indices 7, 21, 43, 57 — excluded from NBVI selection
- **Null subcarriers**: Index 0 (DC), 1, 63 — excluded from all processing
- **Guard band**: Indices 27–37 — center guard + upper null carriers, excluded

The signal processing pipeline applies subcarrier selection masks to use only the 47 data subcarriers for deltaRMS, phase variance, and breathing detection.

---

## Complete Example: 64-Subcarrier CSI Frame

### Hex Dump

```
Offset  0  1  2  3  4  5  6  7  8  9  A  B  C  D  E  F
      ┌──────────────────────────────────────┐
00    │ AA BB CC DD EE FF 11 22 33 44 55 66 │  ← node_mac[6], peer_mac[6]
10    │ 00 00 00 00 00 01 23 45 D6 B0 00 3C │  ← timestamp_us (LE), rssi, noise_floor
20    │ 06 40 32 1F 29 3A ... (payload)    │  ← channel=6, n_sub=64, I₀, Q₀, I₁, Q₁, ...
      └──────────────────────────────────────┘
```

### Field Values

- **`node_mac`**: `AA:BB:CC:DD:EE:FF`
- **`peer_mac`**: `11:22:33:44:55:66`
- **`timestamp_us`**: `0x00000000012345` = 119,305 µs since boot
- **`rssi`**: `0xD6` = -42 dBm (interpreted as int8)
- **`noise_floor`**: `0xB0` = -80 dBm
- **`channel`**: 6
- **`n_sub`**: 64
- **Payload starts at offset 24**: 128 bytes of I/Q pairs (64 subcarriers × 2)

---

## Complete Example: Single File Record

### Binary Layout

```
Offset  0  1  2  3  4  5  6  7  8  9  A  B  C  D  E  F
      ┌──────────────────────────────────────────────┐
00    │ 53 50 41 58 4C 52 45 50 00 00 00 00 00 00 │  ← "SPAXLREP" magic
10    │ 00 00 00 00 00 00 20 00 00 00 00 00 00 00 │  ← writePos=8192, oldestPos=0
20    │ 00 00 00 00 00 00 00 00                      │  ← wrapPos=0
      ├──────────────────────────────────────────────┤
20    │ 00 01 D6 A8 B2 CF 5D 00 98 00 00 00 00 01 │  ← Record prefix:
30    │ AA BB CC DD EE FF 11 22 33 44 55 66 00 00... │  ← recvTimeNS, frameLen,
      │                                            │     CSI frame starts here
      └──────────────────────────────────────────────┘
```

### Record Breakdown

**File Header (bytes 0–31):**
- Magic: `"SPAXLREP"` at offset 0
- `writePos`: 8192 (next write at byte 8192)
- `oldestPos`: 0 (buffer empty)
- `wrapPos`: 0 (no wrap yet)

**First Record (bytes 32–183):**
- `recvTimeNS`: `0x000001D6A8B2CF5D` = 2024-08-28 12:34:56.789012345 UTC
- `frameLen`: `0x0098` = 152 bytes
- `frameData`: 152-byte CSI frame (24-byte header + 128-byte I/Q payload)

---

## Data Types and Endianness

| Type | Size | Range | Notes |
|------|------|-------|-------|
| uint8 | 1 byte | 0–255 | Unsigned byte |
| int8 | 1 byte | -128–127 | Signed byte (reinterpreted from uint8) |
| uint16 | 2 bytes | 0–65535 | **Little-endian** |
| uint64 | 8 bytes | 0–18,446,744,073,709,551,615 | **Little-endian** |
| int64 | 8 bytes | -9,223,372,036,854,775,808 to 9,223,372,036,854,775,807 | **Little-endian** |

**Important Notes:**

1. **All multi-byte integers are little-endian**: Least significant byte first
2. **RSSI and noise_floor are stored as uint8 but interpreted as int8**: Values 128–255 map to -128–-1 when cast
3. **MAC addresses are big-endian byte arrays**: `AA:BB:CC:DD:EE:FF` is stored as `[0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF]`
4. **`n_sub` includes all subcarriers**: For 64-subcarrier CSI, `n_sub = 64` regardless of which are actually data subcarriers

---

## File Size Calculations

### Per-Frame Storage

For 64 subcarriers (typical):
- CSI frame: 24 + (64 × 2) = **152 bytes**
- Record overhead: 10 bytes
- **Total per frame:** 162 bytes

### Write Rate Estimates

| Configuration | Frame Rate | Links | Throughput | 48-Hour Size |
|--------------|------------|-------|------------|---------------|
| Minimal (2 nodes) | 20 Hz | 2 | 6.5 KB/s | ~27 MB |
| Typical (8 nodes) | 20 Hz | 28 | 90 KB/s | ~377 MB |
| Large (16 nodes) | 20 Hz | 120 | 389 KB/s | ~1.6 GB |

**Default capacity:** 360 MB = ~48 hours at typical rate (8 nodes, 20 Hz)

---

## Validation Rules (Ingestion Server)

Before writing to the replay store, the mothership validates each CSI frame:

1. **Minimum length:** Frame must be ≥ 24 bytes (header-only is valid)
2. **Payload length match:** `len(frame) == 24 + (n_sub × 2)`
3. **Subcarrier count limit:** `n_sub ≤ 128`
4. **RSSI = 0:** Allowed but logged; AGC normalization skipped
5. **Channel validity:** `1 ≤ channel ≤ 14`
6. **Channel = 0:** Invalid (never a valid 2.4 GHz channel)

Malformed frames are dropped (not written to replay store) and logged at DEBUG level to avoid log flooding at high frame rates.

---

## Recovery and Corruption Handling

On startup, the mothership reads the file header:

1. **Magic check:** First 8 bytes must be `"SPAXLREP"`
2. **Range validation:** `writePos`, `oldestPos`, `wrapPos` must be within valid ranges
3. **Truncation recovery:** If the file was truncated during an unclean shutdown, scan backward from `writePos` to find the last complete record

**Corrupted records:** If a record header or frame appears corrupted:
- Skip to next record position
- Reset `oldestPos` if necessary
- Continue scanning (graceful degradation)

---

## See Also

- **Plan:** `docs/plan/plan.md` — Component 14 (Time-Travel Debugging)
- **Code:** `mothership/internal/replay/store.go` — RecordingStore implementation
- **Code:** `mothership/internal/ingestion/frame.go` — CSI frame parsing
- **Test data:** `testdata/generate_csi_recording.go` — Example CSI frame generation
