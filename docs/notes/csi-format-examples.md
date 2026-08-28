# CSI Format Examples — Annotated Hex Dumps

**Date:** 2026-08-28  
**Purpose:** Concrete examples of CSI recording formats with annotated hex dumps

---

## Overview

Spaxel uses **two binary CSI recording formats**:

| Format | Magic | Purpose | File | Status |
|--------|-------|---------|------|--------|
| Recording Store | `SPAXLREP` | Long-term replay storage | `mothership/internal/replay/store.go` | Legacy |
| Recording Buffer | `SPAXLREC` | Runtime circular buffer | `mothership/internal/recording/buffer.go` | **Active** |

This document provides annotated hex dumps for both formats and the CSI frame format they contain.

---

## CSI Frame Format (within both file formats)

Every CSI frame sent from ESP32 nodes follows this binary layout:

```
┌─────────────────────────────────────────────────────────────────┐
│ CSI Frame Header (24 bytes)                                      │
├─────────────────────────────────────────────────────────────────┤
│ 0-5:   node_mac[6]      — Source node MAC address               │
│ 6-11:  peer_mac[6]      — Transmitting peer MAC (AP or TX node)   │
│ 12-19: timestamp_us[8]  — uint64 LE, microseconds since boot    │
│ 20:    rssi[1]          — int8, signal strength in dBm          │
│ 21:    noise_floor[1]   — int8, noise floor in dBm              │
│ 22:    channel[1]       — uint8, WiFi channel (1-14)             │
│ 23:    n_sub[1]         — uint8, subcarrier count (typically 64) │
├─────────────────────────────────────────────────────────────────┤
│ CSI Frame Payload (n_sub × 2 bytes)                              │
│ 24+: I₀,Q₀,I₁,Q₁,... — Interleaved I/Q pairs (int8 each)         │
└─────────────────────────────────────────────────────────────────┘
```

### Example: 64-Subcarrier CSI Frame

**Total size:** 152 bytes (24-byte header + 128-byte payload)

#### Hex Dump (with annotations)

```
Offset  0  1  2  3  4  5  6  7  8  9  A  B  C  D  E  F
      ┌──────────────────────────────────────────────────┐
00    │ AA BB CC DD EE FF 11 22 33 44 55 66 00 00 00 00 │ ← node_mac, peer_mac
10    │ 00 01 23 45 67 89 AB CD D6 B0 06 40 32 1F 29 3A │ ← timestamp_us (LE), rssi, noise_floor, channel, n_sub
20    │ 2A 47 35 28 1C 41 52 69 7E 85 9A B7 C4 D1 E8 F5 │ ← I₀,Q₀,I₁,Q₁,... (subcarriers 0-7)
30    │ 02 1F 3C 59 76 93 B0 CD EA 07 24 41 5E 7B 98 B5 │ ← subcarriers 8-15
40    │ D2 EF 0C 29 46 63 80 9D BA D7 F4 11 2E 4B 68 85 │ ← subcarriers 16-23
50    │ A2 BF DC F9 16 33 50 6D 8A A7 C4 E1 FE 1B 38 55 │ ← subcarriers 24-31
60    │ 72 8F AC C9 E6 03 20 3D 5A 77 94 B1 CE EB 08 25 │ ← subcarriers 32-39
70    │ 42 5F 7C 99 B6 D3 F0 0D 2A 47 64 81 9E BB D8 F5 │ ← subcarriers 40-47
80    │ 12 2F 4C 69 86 A3 C0 DD FA 17 34 51 6E 8B A8 C5 │ ← subcarriers 48-55
90    │ E2 FF 1C 39 56 73 90 AD CA E7 04 21 3E 5B 78 95 │ ← subcarriers 56-63
A0    │                                                 │
      └──────────────────────────────────────────────────┘
```

#### Field Values

| Field | Bytes | Value | Interpretation |
|-------|-------|-------|----------------|
| node_mac | 0-5 | `AA:BB:CC:DD:EE:FF` | Source node MAC |
| peer_mac | 6-11 | `11:22:33:44:55:66` | Peer MAC |
| timestamp_us | 12-19 | `0xCDAB896745230100` | 1,474,503,293,901 µs since boot |
| rssi | 20 | `0xD6` | -42 dBm (as int8) |
| noise_floor | 21 | `0xB0` | -80 dBm (as int8) |
| channel | 22 | `0x06` | Channel 6 |
| n_sub | 23 | `0x40` | 64 subcarriers |
| Payload | 24-151 | I/Q pairs | 128 bytes (64 × 2) |

**Key notes:**
- All multi-byte integers are **little-endian**
- `rssi` and `noise_floor` are stored as `uint8` but interpreted as `int8`
- Values ≥128 map to negative when cast (e.g., `0xD6` = 214 → -42 as int8)

---

## Recording Buffer Format (`SPAXLREC`) — **Active Format**

**Used by:** `mothership/internal/recording/buffer.go` (current implementation)  
**File path:** `/data/csi_replay.bin` (configurable)  
**Features:** Circular buffer, optional zstd compression, time-based retention

### File Header (32 bytes)

```
Offset  0  1  2  3  4  5  6  7  8  9  A  B  C  D  E  F
      ┌──────────────────────────────────────────────────┐
00    │ 53 50 41 58 4C 52 45 43 00 00 00 00 00 00 00 00 │ ← "SPAXLREC" magic, writePos=0
10    │ 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 │ ← oldestPos=0, wrapPos=0
      └──────────────────────────────────────────────────┘
```

| Offset | Size | Field | Example Value |
|--------|------|-------|---------------|
| 0 | 8 | magic | `"SPAXLREC"` (`53 50 41 58 4C 52 45 43`) |
| 8 | 8 | writePos | `0x00000000000020A0` = 8352 (next write position) |
| 16 | 8 | oldestPos | `0x0000000000000020` = 32 (oldest valid record) |
| 24 | 8 | wrapPos | `0x0000000000000000` = 0 (no wrap yet) |

### Per-Frame Record (variable length)

```
┌─────────────────────────────────────────────────────────────┐
│ recvTimeNS  (8 bytes, int64 LE) — Unix nanosecond timestamp │
├─────────────────────────────────────────────────────────────┤
│ frameLen    (2 bytes, uint16 LE) — CSI frame byte count      │
├─────────────────────────────────────────────────────────────┤
│ frameData   (N bytes) — Raw CSI frame (format above)         │
└─────────────────────────────────────────────────────────────┘
```

**Total record size:** `10 + frameLen` bytes

### Complete Example: First 3 Records

#### Initial State (empty buffer)

```
Offset  0  1  2  3  4  5  6  7  8  9  A  B  C  D  E  F
      ┌──────────────────────────────────────────────────┐
00    │ 53 50 41 58 4C 52 45 43 20 00 00 00 00 00 00 00 │ ← magic, writePos=32 (0x20)
10    │ 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 │ ← oldestPos=0, wrapPos=0
      └──────────────────────────────────────────────────┘
```

#### After Writing First CSI Frame (152 bytes)

```
Offset  0  1  2  3  4  5  6  7  8  9  A  B  C  D  E  F
      ┌──────────────────────────────────────────────────┐
00    │ 53 50 41 58 4C 52 45 43 C2 00 00 00 00 00 00 00 │ ← writePos=0xC2 (194)
10    │ 20 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 │ ← oldestPos=32 (0x20)
      ├──────────────────────────────────────────────────┤
20    │ 00 D8 A8 B2 CF 5D 00 00 98 00 AA BB CC DD EE FF │ ← recvTimeNS=0x00005DCFB2A8D800
30    │ 11 22 33 44 55 66 00 01 23 45 67 89 AB CD D6 B0 │ ← frameLen=0x0098 (152), CSI starts
40    │ 06 40 32 1F 29 3A 2A 47 35 28 1C 41 52 69 7E 85 │ ← CSI frame continued...
      │                                              │     (152 bytes total)
      └──────────────────────────────────────────────────┘
```

**Record breakdown:**
- `recvTimeNS`: `0x00005DCFB2A8D800` = 2024-08-28 12:34:56.789012 UTC
- `frameLen`: `0x0098` = 152 bytes
- `frameData`: CSI frame (bytes 42-193)

#### Header After 3 Frames (456 bytes total = 32 + 3×162)

```
Offset  0  1  2  3  4  5  6  7  8  9  A  B  C  D  E  F
      ┌──────────────────────────────────────────────────┐
00    │ 53 50 41 58 4C 52 45 43 36 02 00 00 00 00 00 00 │ ← writePos=0x236 (566)
10    │ 20 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 │ ← oldestPos=32, wrapPos=0
      └──────────────────────────────────────────────────┘
```

### Wrap Behavior Example

When `writePos` reaches the end of the file, it wraps back to byte 32:

```
Before wrap (file size 1 MB):
  writePos = 1048512
  oldestPos = 32
  wrapPos = 0

After record would exceed file:
  writePos = 32          ← wraps to data area start
  oldestPos = 1024       ← oldest record evicted
  wrapPos = 1048512      ← stores wrap position for scanner
```

---

## Recording Store Format (`SPAXLREP`) — Legacy Format

**Used by:** `mothership/internal/replay/store.go` (legacy, uncompressed only)  
**Status:** Superseded by `SPAXLREC` buffer with compression

### Structure

**Identical to `SPAXLREC` except magic number.**

The header and record format are the same — only the magic differs:
- `SPAXLREP` vs. `SPAXLREC`

This format lacks:
- Compression support
- Chunked record types
- `CSCZ` compressed chunks

---

## Compressed Chunk Format (`CSCZ`)

When `SPAXLREC` compression is enabled, frames are batched into zstd-compressed chunks:

### Chunk Header (16 bytes, after 2-byte length prefix)

```
Offset  0  1  2  3  4  5  6  7  8  9  A  B  C  D  E  F
      ┌──────────────────────────────────────────────────┐
00    │ 43 53 43 5A 01 00 0A 00 80 00 00 00 00 00 00 00 │ ← "CSCZ", v1, frames=10, uncomp=128
      └──────────────────────────────────────────────────┘
```

| Offset | Size | Field | Description |
|--------|------|-------|-------------|
| 0 | 4 | magic | `"CSCZ"` |
| 4 | 1 | version | Chunk format version (currently 1) |
| 5 | 2 | frameCount | Number of frames in this chunk |
| 7 | 4 | uncompressedLen | Total uncompressed size (bytes) |
| 11 | 5 | reserved | Future use |

### Complete Compressed Record

```
┌─────────────────────────────────────────────────────────────┐
│ chunkLen    (2 bytes, uint16 LE) — Header+data length        │
├─────────────────────────────────────────────────────────────┤
│ chunkHeader (16 bytes) — CSCZ header above                   │
├─────────────────────────────────────────────────────────────┤
│ zstdData   (variable) — Compressed frame batch              │
└─────────────────────────────────────────────────────────────┘
```

**Example:** A 100-frame batch might compress from ~16 KB to ~2 KB (8:1 ratio).

---

## Validation Examples

### Valid CSI Frame

```go
frame := []byte{
    0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF,  // node_mac
    0x11, 0x22, 0x33, 0x44, 0x55, 0x66,  // peer_mac
    0x00, 0x00, 0x00, 0x00, 0x01, 0x23, 0x45, 0x67,  // timestamp_us
    0xD6, 0xB0, 0x06, 0x40,  // rssi=-42, noise_floor=-80, channel=6, n_sub=64
    // ... 128 bytes of I/Q payload ...
}
```

**Validation checks:**
1. ✅ Length ≥ 24 bytes (minimum)
2. ✅ Length == 24 + (n_sub × 2) = 152 bytes
3. ✅ n_sub (64) ≤ 128
4. ⚠️  rssi != 0 (AGC normalization applies)
5. ✅ channel (6) in range 1-14

### Invalid CSI Frame Examples

#### Too Short
```go
frame := []byte{0xAA, 0xBB}  // Only 2 bytes
```
**Error:** `frame too short: 2 bytes (minimum 24)`

#### Payload Mismatch
```go
frame := make([]byte, 100)  // Claims 64 subcarries but only 76 bytes total
frame[23] = 64  // n_sub = 64, but len(frame) != 24 + 128
```
**Error:** `payload length mismatch: expected 152 bytes, got 100`

#### Invalid Channel
```go
frame := []byte{...}
frame[22] = 0  // channel = 0
```
**Error:** `invalid channel: 0`

#### Implausible Subcarrier Count
```go
frame := []byte{...}
frame[23] = 200  // n_sub = 200
```
**Error:** `implausible subcarrier count: 200 (max 128)`

---

## Cross-Reference: Code vs. Documentation

### File Format Constants

| Format | Documentation | Code Location | Constant |
|--------|--------------|---------------|-----------|
| Recording Buffer | ✅ `docs/notes/csi-recording-format.md` | `internal/recording/buffer.go:35` | `fileMagic = "SPAXLREC"` |
| Recording Store | ⚠️  Legacy only | `internal/replay/store.go` | `fileMagic = "SPAXLREP"` |
| Compressed Chunk | ✅ `docs/notes/csi-recording-format.md` | `internal/recording/compression.go` | `chunkMagic = "CSCZ"` |

### CSI Frame Parsing

| Aspect | Documentation | Code Location |
|--------|--------------|---------------|
| Header size (24 bytes) | ✅ `csi-recording-file-format.md` | `internal/ingestion/frame.go:12` |
| n_sub ≤ 128 validation | ✅ `csi-recording-file-format.md` | `internal/ingestion/frame.go:75` |
| Channel 1-14 validation | ✅ `csi-recording-file-format.md` | `internal/ingestion/frame.go:87-94` |
| I/Q as int8 | ✅ `amplitude-phase-data-representation.md` | `internal/ingestion/frame.go:99-102` |

### Discrepancy Found

**Issue:** `docs/notes/csi-recording-file-format.md` documents `"SPAXLREP"` as the active format, but the active code uses `"SPAXLREC"`.

**Resolution:** 
- `"SPAXLREP"` is the **legacy** format in `internal/replay/store.go`
- `"SPAXLREC"` is the **active** format in `internal/recording/buffer.go`
- Documentation should clarify this distinction (addressed in `csi-recording-format.md`)

---

## Test Data Validation

**Generated file:** `testdata/csi_session_mixed_activity.bin`  
**Generator:** `testdata/generate_csi_recording.go`  
**Format used:** `SPAXLREP` (legacy) — **should be updated to `SPAXLREC`**

### Verification Command

```bash
# Check magic number
xxd -l 32 -p testdata/csi_session_mixed_activity.bin | head -1
# Expected: 535041584c524550 (SPAXLREP)
# Should be: 535041584c524543 (SPAXLREC)

# Count frames
go run testdata/verify_recording.go testdata/csi_session_mixed_activity.bin
```

---

## Endianness Notes

**All multi-byte integers are little-endian:**

| Field | Bytes | Little-Endian Interpretation |
|-------|-------|------------------------------|
| timestamp_us | 12-19 | `0xCDAB896745230100` = `0x000123456789ABCD` |
| recvTimeNS | 0-7 (record) | `0x00D8A8B2CF5D0000` = `0x00005DCFB2A8D800` |
| frameLen | 8-9 (record) | `0x9800` = `0x0098` = 152 |

**MAC addresses are stored big-endian as byte arrays:**
```go
mac := []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}
// Displayed as: AA:BB:CC:DD:EE:FF
```

---

## Related Files

- **Code:** `mothership/internal/recording/buffer.go` — Recording buffer implementation
- **Code:** `mothership/internal/recording/compression.go` — Chunked compression
- **Code:** `mothership/internal/ingestion/frame.go` — CSI frame parsing
- **Code:** `mothership/internal/replay/store.go` — Legacy replay store
- **Test:** `testdata/generate_csi_recording.go` — CSI recording generator
- **Test:** `testdata/verify_recording.go` — Recording verifier
- **Docs:** `docs/notes/csi-recording-format.md` — Format comparison
- **Docs:** `docs/notes/amplitude-phase-data-representation.md` — I/Q to amplitude/phase
