# CSI Format Validation — Documentation vs. Code

**Date:** 2026-08-28  
**Purpose:** Validate CSI recording documentation against actual implementation  
**Validation scope:** File formats, CSI frame structure, I/Q representation, compression

---

## Validation Methodology

1. **Read documentation:** All CSI-related docs in `docs/notes/`
2. **Read source code:** `mothership/internal/recording/`, `mothership/internal/ingestion/`, `testdata/`
3. **Cross-reference constants:** Magic numbers, field sizes, validation rules
4. **Test data generation:** Run `testdata/generate_csi_recording.go` and verify output
5. **Identify discrepancies:** Document any mismatches between docs and code

---

## Validation Results

### ✅ File Header Structure

**Documentation:** `docs/notes/csi-recording-file-format.md` lines 15-24  
**Code:** `mothership/internal/recording/buffer.go` lines 3-9

| Field | Docs | Code | Status |
|-------|------|------|--------|
| Magic size | 8 bytes | 8 bytes | ✅ Match |
| writePos size | 8 bytes uint64 LE | 8 bytes uint64 LE | ✅ Match |
| oldestPos size | 8 bytes uint64 LE | 8 bytes uint64 LE | ✅ Match |
| wrapPos size | 8 bytes uint64 LE | 8 bytes uint64 LE | ✅ Match |
| Total header | 32 bytes | 32 bytes | ✅ Match |

**Code reference:**
```go
// buffer.go:3-9
//	Header (32 bytes):
//	  magic[8]      "SPAXLREC"
//	  writePos[8]   uint64 LE
//	  oldestPos[8]  uint64 LE
//	  wrapPos[8]    uint64 LE
```

---

### ✅ Per-Frame Record Structure

**Documentation:** `docs/notes/csi-recording-file-format.md` lines 42-54  
**Code:** `mothership/internal/recording/buffer.go` lines 11-14

| Field | Docs | Code | Status |
|-------|------|------|--------|
| recvTimeNS | 8 bytes int64 LE | 8 bytes int64 LE | ✅ Match |
| frameLen | 2 bytes uint16 LE | 2 bytes uint16 LE | ✅ Match |
| frameData | N bytes | N bytes | ✅ Match |
| Total overhead | 10 bytes | 10 bytes | ✅ Match |

**Code reference:**
```go
// buffer.go:11-14
//	Record (10 + frameLen bytes):
//	  recvTimeNS[8]  int64 LE
//	  frameLen[2]    uint16 LE
//	  frameData[N]   raw CSI frame bytes
```

---

### ⚠️ Magic Number Discrepancy

**Issue:** Two different magic numbers exist, documentation not clear on which is active.

| Format | Magic | Code Location | Usage |
|--------|-------|---------------|-------|
| Recording Buffer | `SPAXLREC` | `buffer.go:35` | **Active** |
| Recording Store | `SPAXLREP` | `store.go` | Legacy |

**Code evidence:**
```go
// buffer.go:35 — Active implementation
const fileMagic = "SPAXLREC"

// store.go — Legacy implementation
const fileMagic = "SPAXLREP"
```

**Documentation status:**
- `csi-recording-file-format.md`: Documents `SPAXLREP` (legacy) without noting it's superseded
- `csi-recording-format.md`: ✅ Correctly documents both formats with status
- `csi-io-code-paths.md`: ✅ Correctly identifies `SPAXLREC` as active

**Recommendation:** Update `csi-recording-file-format.md` to reference `csi-recording-format.md` for format comparison.

---

### ✅ CSI Frame Header Structure

**Documentation:** `docs/notes/csi-recording-file-format.md` lines 70-80  
**Code:** `mothership/internal/ingestion/frame.go` lines 19-28

| Field | Offset | Docs | Code | Status |
|-------|--------|------|------|--------|
| node_mac | 0-5 | 6 bytes uint8[6] | 6 bytes [6]byte | ✅ Match |
| peer_mac | 6-11 | 6 bytes uint8[6] | 6 bytes [6]byte | ✅ Match |
| timestamp_us | 12-19 | 8 bytes uint64 LE | 8 bytes uint64 | ✅ Match |
| rssi | 20 | 1 byte int8 | 1 byte int8 | ✅ Match |
| noise_floor | 21 | 1 byte int8 | 1 byte int8 | ✅ Match |
| channel | 22 | 1 byte uint8 | 1 byte uint8 | ✅ Match |
| n_sub | 23 | 1 byte uint8 | 1 byte uint8 | ✅ Match |
| **Total header** | 0-23 | **24 bytes** | **24 bytes** | ✅ Match |

**Code reference:**
```go
// frame.go:19-41
type CSIFrame struct {
    NodeMAC     [6]byte    // 0-5
    PeerMAC     [6]byte    // 6-11
    TimestampUS uint64      // 12-19
    RSSI        int8        // 20
    NoiseFloor  int8        // 21
    Channel     uint8       // 22
    NSub        uint8       // 23
    Payload     []int8      // 24+
}
```

---

### ✅ CSI Frame Payload Structure

**Documentation:** `docs/notes/csi-recording-file-format.md` lines 82-95  
**Code:** `mothership/internal/ingestion/frame.go` lines 96-103

| Aspect | Docs | Code | Status |
|--------|------|------|--------|
| I/Q interleaving | I₀, Q₀, I₁, Q₁, ... | I₀, Q₀, I₁, Q₁, ... | ✅ Match |
| Data type | int8 | int8 | ✅ Match |
| Size per subcarrier | 2 bytes | 2 bytes | ✅ Match |
| Total for 64 subcarriers | 128 bytes | 128 bytes | ✅ Match |

**Code reference:**
```go
// frame.go:96-103
frame.Payload = make([]int8, int(nSub)*2)
payloadData := data[HeaderSize:]
for i := range frame.Payload {
    frame.Payload[i] = int8(payloadData[i])
}
```

---

### ✅ CSI Frame Validation Rules

**Documentation:** `docs/notes/csi-recording-file-format.md` lines 227-237  
**Code:** `mothership/internal/ingestion/frame.go` lines 47-94

| Rule | Docs | Code | Status |
|------|------|------|--------|
| Min length ≥ 24 | ✅ | frame.go:49-52 | ✅ Implemented |
| Payload match `24 + n_sub×2` | ✅ | frame.go:67-72 | ✅ Implemented |
| `n_sub ≤ 128` | ✅ | frame.go:74-78 | ✅ Implemented |
| `rssi == 0` allowed | ✅ | frame.go:82-84 | ✅ Implemented (logged) |
| `channel 1-14` | ✅ | frame.go:87-94 | ✅ Implemented |
| `channel == 0` invalid | ✅ | frame.go:87-90 | ✅ Implemented |

**Code evidence:**
```go
// frame.go:49-52 — Rule 1
if len(data) < MinFrameSize {
    return nil, fmt.Errorf("frame too short: %d bytes (minimum %d)", len(data), MinFrameSize)
}

// frame.go:67-72 — Rule 2
expectedLen := HeaderSize + int(nSub)*2
if len(data) != expectedLen {
    return nil, fmt.Errorf("payload length mismatch: expected %d bytes, got %d", expectedLen, len(data))
}

// frame.go:74-78 — Rule 3
if nSub > 128 {
    return nil, fmt.Errorf("implausible subcarrier count: %d (max 128)", nSub)
}

// frame.go:87-94 — Rules 5-6
if frame.Channel == 0 {
    return nil, fmt.Errorf("invalid channel: %d", frame.Channel)
}
if frame.Channel > 14 {
    return nil, fmt.Errorf("invalid channel: %d", frame.Channel)
}
```

---

### ✅ Endianness

**Documentation:** `docs/notes/csi-recording-file-format.md` lines 11, 187-202  
**Code:** All `binary.LittleEndian` calls throughout

| Field | Docs | Code | Status |
|-------|------|------|--------|
| All multi-byte integers | Little-endian | `binary.LittleEndian.Put*` | ✅ Match |
| MAC addresses | Big-endian byte array | `[6]byte` stored as-is | ✅ Match |

**Code evidence:**
```go
// buffer.go:205-206 — Writing little-endian
binary.LittleEndian.PutUint64(buf[0:8], uint64(recvTimeNS))
binary.LittleEndian.PutUint16(buf[8:10], uint16(frameLen))

// buffer.go:425-426 — Reading little-endian
recvTimeNS := int64(binary.LittleEndian.Uint64(hdr[0:8]))
frameLen := int64(binary.LittleEndian.Uint16(hdr[8:10]))
```

---

### ✅ Amplitude/Phase Representation

**Documentation:** `docs/notes/amplitude-phase-data-representation.md`  
**Code:** `mothership/internal/ingestion/frame.go` lines 96-103

| Aspect | Docs | Code | Status |
|--------|------|------|--------|
| Stored as I/Q (Cartesian) | ✅ | Payload is `[]int8` I/Q pairs | ✅ Match |
| Amplitude conversion | `√(I² + Q²)` | Not in frame.go (pipeline) | ✅ Out of scope |
| Phase conversion | `atan2(Q, I)` | Not in frame.go (pipeline) | ✅ Out of scope |
| I/Q range | -128 to +127 | int8 = -128 to +127 | ✅ Match |

**Code evidence:**
```go
// frame.go:96-103 — I/Q stored as int8
frame.Payload = make([]int8, int(nSub)*2)
payloadData := data[HeaderSize:]
for i := range frame.Payload {
    frame.Payload[i] = int8(payloadData[i])  // Cast uint8 → int8
}
```

**Note:** Amplitude/phase conversion happens in the signal processing pipeline, not in frame parsing. This is correctly documented as out-of-scope for `frame.go`.

---

### ✅ Compressed Chunk Format

**Documentation:** `docs/notes/csi-io-code-paths.md` lines 64-77  
**Code:** `mothership/internal/recording/compression.go`

| Field | Docs | Code | Status |
|-------|------|------|--------|
| Magic | `"CSCZ"` | `chunkMagic = "CSCZ"` | ✅ Match |
| Header size | 16 bytes | 16 bytes | ✅ Match |
| Version field | 1 byte | `version[1]` | ✅ Match |
| frameCount | 2 bytes | `frameCount[2]` | ✅ Match |
| uncompressedLen | 4 bytes | `uncompressedLen[4]` | ✅ Match |
| reserved | 5 bytes | `reserved[5]` | ✅ Match |

**Code evidence:**
```go
// compression.go — Chunk header structure
chunkMagic   = "CSCZ"
headerSize   = 16

// Build chunk header
header[0:4] = []byte(chunkMagic)
header[4] = 1                              // version
binary.LittleEndian.PutUint16(header[5:7], uint16(frameCount))
binary.LittleEndian.PutUint32(header[7:11], uint32(uncompressedLen))
```

---

### ✅ Circular Buffer Behavior

**Documentation:** `docs/notes/csi-recording-file-format.md` lines 32-38  
**Code:** `mothership/internal/recording/buffer.go` lines 188-192

| Behavior | Docs | Code | Status |
|----------|------|------|--------|
| Wrap at EOF | `writePos` → 32 | `b.writePos = headerSize` | ✅ Match |
| Set `wrapPos` | Stores wrap position | `b.wrapPos = b.writePos` | ✅ Match |
| Two arcs (right/left) | ✅ | Scan handles wrap | ✅ Match |

**Code evidence:**
```go
// buffer.go:188-192 — Wrap logic
if b.writePos+recordSize > b.fileSize {
    b.wrapPos = b.writePos           // Store wrap position
    b.writePos = headerSize          // Wrap to data start
}
```

---

## Test Data Validation

### `testdata/generate_csi_recording.go`

**Format used:** `SPAXLREP` (line 16)  
**Should be:** `SPAXLREC` (active format)

**Validation:**
```go
// generate_csi_recording.go:16
const fileMagic = "SPAXLREP"  // ❌ Should be "SPAXLREC"

// generate_csi_recording.go:26-34 — Header structure matches
func writeHeader(f *os.File, writePos, oldestPos, wrapPos uint64) error {
    header := make([]byte, headerSize)
    copy(header[0:8], []byte(fileMagic))
    binary.LittleEndian.PutUint64(header[8:16], writePos)
    binary.LittleEndian.PutUint64(header[16:24], oldestPos)
    binary.LittleEndian.PutUint64(header[24:32], wrapPos)
    // ✅ Structure matches buffer.go exactly
}
```

**Finding:** Test generator uses legacy magic number but otherwise implements the correct format. Should be updated for consistency.

---

## Summary of Findings

### ✅ Fully Validated

1. **File header structure** — Exact match between docs and code
2. **Per-frame record structure** — Exact match
3. **CSI frame header** — Exact match
4. **CSI frame payload** — Exact match
5. **CSI frame validation rules** — All 6 rules implemented correctly
6. **Endianness** — All integers little-endian as documented
7. **I/Q representation** — Stored as int8, matches docs
8. **Compressed chunk format** — Exact match
9. **Circular buffer wrap behavior** — Matches documentation

### ⚠️ Documentation Gaps

1. **Magic number confusion:**
   - `csi-recording-file-format.md` documents `SPAXLREP` without noting it's legacy
   - `csi-recording-format.md` correctly compares both formats
   - **Action:** Cross-reference formats in primary documentation

2. **Test data uses legacy format:**
   - `generate_csi_recording.go` uses `SPAXLREP` instead of active `SPAXLREC`
   - **Action:** Update to `SPAXLREC` for consistency

### ✅ Code Quality Observations

1. **Validation is comprehensive:** All documented rules are enforced with DEBUG logging
2. **Error messages are clear:** Frame validation errors explain what failed
3. **Constants are centralized:** Magic numbers, sizes defined once
4. **Documentation is current:** Code comments match file format docs

---

## Recommendations

### High Priority

1. **Update test generator magic:**
   ```go
   // testdata/generate_csi_recording.go:16
   - const fileMagic = "SPAXLREP"
   + const fileMagic = "SPAXLREC"
   ```

2. **Cross-reference format docs:**
   - Add to `csi-recording-file-format.md`:
     > **Note:** This documents the `SPAXLREC` Recording Buffer format (active implementation).
     > For a comparison with the legacy `SPAXLREP` Recording Store format, see `csi-recording-format.md`.

### Low Priority

3. **Regenerate test data:**
   ```bash
   go run testdata/generate_csi_recording.go
   # Verify magic with: xxd -l 8 -p testdata/csi_session_mixed_activity.bin
   # Expected: 535041584c524543 (SPAXLREC)
   ```

---

## Validation Checklist

- [x] File header structure matches code
- [x] Record structure matches code
- [x] CSI frame header matches code
- [x] CSI frame payload matches code
- [x] Validation rules implemented correctly
- [x] Endianness matches documentation
- [x] I/Q representation accurate
- [x] Compressed chunk format matches
- [x] Circular buffer behavior matches
- [x] Magic number usage clarified
- [ ] Test data updated to active format

---

## Related Files

- **Documentation:** `docs/notes/csi-recording-file-format.md`
- **Documentation:** `docs/notes/csi-recording-format.md` (format comparison)
- **Documentation:** `docs/notes/csi-io-code-paths.md` (I/O code paths)
- **Documentation:** `docs/notes/amplitude-phase-data-representation.md` (I/Q docs)
- **Code:** `mothership/internal/recording/buffer.go` (recording buffer)
- **Code:** `mothership/internal/recording/compression.go` (compression)
- **Code:** `mothership/internal/ingestion/frame.go` (CSI frame parsing)
- **Code:** `mothership/internal/replay/store.go` (legacy replay store)
- **Test:** `testdata/generate_csi_recording.go` (test data generator)
- **Test:** `testdata/verify_recording.go` (recording verifier)
