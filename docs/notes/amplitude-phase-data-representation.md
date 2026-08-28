# Amplitude and Phase Data Representation

## Overview

The CSI recording file stores **raw I/Q (in-phase/quadrature) data**, not pre-computed amplitude and phase values. Amplitude and phase are derived from the raw I/Q components during signal processing in the mothership.

## Raw Data Storage

### Data Types

| Component | Data Type | Range | Notes |
|-----------|-----------|-------|-------|
| I (in-phase) | `int8` | -128 to +127 | Typically ±50 in practice |
| Q (quadrature) | `int8` | -128 to +127 | Typically ±50 in practice |
| Subcarrier count | `uint8` | 0-128 | Typically 64 for ESP32-S3 |

### Array Layout and Ordering

The raw CSI payload uses **interleaved I/Q pairs**:

```
[I₀, Q₀, I₁, Q₁, I₂, Q₂, ..., Iₙ₋₁, Qₙ₋₁]
```

For subcarrier at index `k`:
- I component: offset `k × 2`
- Q component: offset `k × 2 + 1`

**Example** (3 subcarriers): `[I₀, Q₀, I₁, Q₁, I₂, Q₂]` = `[-20, 35, -18, 40, -22, 38]`

### Byte Order

All multi-byte integers use **little-endian** byte order throughout the CSI recording format.

## Amplitude and Phase Derivation

Amplitude and phase are **not stored directly** in the recording. They are computed from raw I/Q data during signal processing:

### Conversion Formulas

```go
amplitude[k] = sqrt(I²[k] + Q²[k])
phase[k]     = atan2(Q[k], I[k])  // radians, range [-π, π]
```

### Data Flow

1. **Recording**: Raw I/Q (`int8[]`) → CSI binary frame → disk
2. **Ingestion**: Parse binary frame → `CSIFrame` struct with `IQData []int8`
3. **Signal Processing**: Convert I/Q → `ProcessedCSI` with:
   - `Amplitude []float64`
   - `ResidualPhase []float64`
4. **Feature Extraction**: Compute motion features from amplitude/phase

## Subcarrier Structure

### ESP32-S3 HT20 Mode (64 Subcarriers)

All 64 subcarriers are recorded, but only a subset is used for motion detection:

| Subcarrier Type | Indices | Usage |
|----------------|---------|-------|
| Data subcarriers | Most of 0-63 | **Used for motion detection** |
| Pilot subcarriers | 7, 21, 43, 57 | Excluded from processing |
| Null subcarriers | 0 (DC), 1, 63 | Excluded from processing |
| Guard band | 27-37 | Excluded from processing |

### Data Subcarriers

**47 usable subcarriers** (out of 64 total) after excluding:
- 4 pilot subcarriers (7, 21, 43, 57)
- DC and nulls (0, 1, 63)
- Guard band (27-37)

These 47 subcarriers provide the spatial diversity needed for WiFi-based motion detection.

## Binary Frame Format

### CSI Frame Header (24 bytes)

```
Offset | Size | Field        | Type        | Description
-------|------|--------------|-------------|------------------------
0      | 6    | node_mac     | uint8[6]    | Sender MAC address
6      | 6    | peer_mac     | uint8[6]    | Peer MAC address
12     | 8    | timestamp_us | uint64 LE   | Microseconds since boot
20     | 1    | rssi         | int8        | Signal strength (dBm)
21     | 1    | noise_floor  | int8        | Noise floor (dBm)
22     | 1    | channel      | uint8       | WiFi channel (1-14)
23     | 1    | n_sub        | uint8       | Subcarrier count
```

### CSI Payload (n_sub × 2 bytes)

```
Offset | Size | Field  | Type   | Description
-------|------|--------|--------|------------------------
24     | 2    | iq[0]  | int8×2 | I₀, Q₀ for subcarrier 0
26     | 2    | iq[1]  | int8×2 | I₁, Q₁ for subcarrier 1
...    | ...  | ...    | ...    | ...
24+k×2 | 2    | iq[k]  | int8×2 | Iₖ, Qₖ for subcarrier k
```

**Total frame size**: `24 + (n_sub × 2)` bytes (typically **152 bytes** for 64 subcarriers)

## Recording File Structure

### File Header (32 bytes)

```
Offset | Size | Field      | Type     | Description
-------|------|------------|----------|------------------------
0      | 8    | magic      | char[8]  | "SPAXLREP"
8      | 8    | writePos   | uint64 LE| Current write position
16     | 8    | oldestPos  | uint64 LE| Oldest record position
24     | 8    | wrapPos    | uint64 LE| Wrap position (0 if no wrap)
```

### Record Format

```
Offset | Size | Field        | Type     | Description
-------|------|--------------|----------|------------------------
0      | 8    | recvTimeNS   | int64 LE | Unix nanosecond timestamp
8      | 2    | frameLen     | uint16 LE| CSI frame byte count
10     | N    | frameData    | uint8[]  | Raw CSI binary frame
```

**Total record size**: `10 + frameLen` bytes

## Implementation References

### Firmware (ESP32-S3)

- **CSI capture**: `firmware/main/csi.c` — Raw I/Q extraction from ESP32 CSI buffer
- **WebSocket serialization**: `firmware/main/websocket.c:409-449` — Binary frame packing and transmission

### Mothership (Go)

- **Frame parsing**: `mothership/internal/ingestion/frame.go` — Binary frame → `CSIFrame` struct
- **Signal processing**: `mothership/internal/signal/processor.go` — I/Q → amplitude/phase conversion
- **Feature extraction**: `mothership/internal/signal/features.go` — Motion detection features

### Test Data Generation

- **Recording generator**: `testdata/generate_csi_recording.go:56-109` — Creates synthetic CSI frames with realistic I/Q values

## Key Design Decisions

### Why Store I/Q Instead of Amplitude/Phase?

1. **Lossless**: I/Q captures all signal information; computing amplitude/phase is deterministic
2. **Space efficient**: 2 bytes per subcarrier (int8 + int8) vs. 16 bytes (float64 + float64)
3. **Flexible processing**: Different algorithms can derive amplitude/phase as needed
4. **Hardware native**: ESP32 CSI buffer provides I/Q directly

### Phase Sanitization

Raw phase values suffer from phase wrapping and ambiguity. The mothership applies **phase sanitization** before motion detection:

```go
// Phase sanitization in processor.go
ResidualPhase = unwrap(phase) - remove_linear_trend(phase)
```

This removes:
- Phase wrapping discontinuities (jumps from π to -π)
- Linear phase progression (distance-dependent, not motion-dependent)

The result is a **residual phase** that reflects only motion-induced changes.

## Related Documentation

- [CSI Recording File Structure and Serialization Format](csi-recording-file-format.md)
- [CSI Recording I/O Code Paths and Entry Points](csi-recording-io-code-paths.md)
- Signal processing: `mothership/internal/signal/processor.go`
- Frame parsing: `mothership/internal/ingestion/frame.go`
