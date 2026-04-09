# Spaxel CSI Simulator

A Go CLI tool for testing the Spaxel mothership without ESP32 hardware. The simulator opens WebSocket connections as virtual nodes and sends synthetic CSI binary frames.

## Building

```bash
cd mothership
go build -o spaxel-sim ./cmd/sim
```

## Usage

```bash
spaxel-sim [flags]
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--mothership` | string | `ws://localhost:8080/ws/node` | Mothership WebSocket URL |
| `--token` | string | `""` | Node authentication token (X-Spaxel-Token header) |
| `--nodes` | int | `4` | Number of virtual nodes to simulate |
| `--walkers` | int | `1` | Number of walking persons to simulate |
| `--rate` | int | `20` | CSI packet rate in Hz per node |
| `--duration` | duration | `30s` | Simulation duration (0 = run forever) |
| `--ble` | bool | `false` | Also send simulated BLE advertisements |
| `--seed` | int64 | `42` | Random seed for reproducible runs |
| `--width` | float64 | `6.0` | Space width in meters |
| `--depth` | float64 | `5.0` | Space depth in meters |
| `--height` | float64 | `2.5` | Space height in meters |
| `--space` | string | `""` | Space dimensions as "WxDxH" (overrides --width/--depth/--height) |
| `--show-frame-rate` | bool | `true` | Show per-second frame counts to stdout |
| `--verbose` | bool | `false` | Enable verbose logging |

### Examples

**Basic simulation (4 nodes, 1 walker, 30 seconds):**
```bash
spaxel-sim --mothership ws://localhost:8080/ws/node --nodes 4 --walkers 1 --duration 30s
```

**With authentication:**
```bash
spaxel-sim --mothership ws://localhost:8080/ws/node --token <your-token> --nodes 4
```

**Custom space dimensions:**
```bash
spaxel-sim --space "10x8x3.0" --nodes 6 --walkers 2
```

**With BLE advertisements:**
```bash
spaxel-sim --ble --walkers 2
```

**Reproducible run (fixed seed):**
```bash
spaxel-sim --seed 12345 --duration 60s
```

**Run indefinitely (for manual testing):**
```bash
spaxel-sim --duration 0
```

## CSI Frame Format

The simulator generates valid CSI binary frames matching the Spaxel specification:

```
Header (24 bytes):
  node_mac:     6 bytes  — source node MAC
  peer_mac:     6 bytes  — transmitting peer MAC
  timestamp_us: 8 bytes  — uint64, microseconds since node boot
  rssi:         1 byte   — int8, dBm
  noise_floor:  1 byte   — int8, dBm
  channel:      1 byte   — uint8, WiFi channel
  n_sub:        1 byte   — uint8, subcarrier count

Payload (n_sub × 2 bytes):
  Per subcarrier: int8 I, int8 Q
```

## Simulation Model

### Virtual Nodes
- Positioned at corners and edges of the defined space
- Mixed heights (high at 80% of room height, low at 30%)
- MAC addresses: `AA:BB:CC:DD:E0:00` through `AA:BB:CC:DD:E0:0N`

### Walkers
- Start at center of room (average person height: 1.7m)
- Random walk with Gaussian velocity updates
- Bounce off walls
- BLE address: `11:22:33:44:55:00` through `11:22:33:44:55:0N`

### Signal Model
- **Path loss:** Free space path loss model (PL(d) = PL₀ + 10·n·log₁₀(d/d₀))
  - PL₀ = 40 dB at d₀ = 1m
  - n = 2.0 (free space)
- **Fresnel modulation:** Amplitude increases when walker is in Fresnel zones
  - Zone 1: maximum modulation
  - Zone 5+: no modulation
- **Noise:** Gaussian noise added to I/Q values

## Integration Testing

The simulator is designed for integration testing:

```bash
# Start mothership
docker run -d -p 8080:8080 --name spaxel-test ghcr.io/spaxel/spaxel:latest

# Run simulator for 30 seconds
spaxel-sim --mothership ws://localhost:8080/ws/node --nodes 4 --walkers 1 --duration 30s

# Check blob count (should be > 0 if detection is working)
curl -s http://localhost:8080/api/blobs | jq '. | length > 0'
```

## Output

The simulator logs:
- Connection status
- Per-second frame rates (if `--show-frame-rate`)
- Final statistics on completion
- Errors (authentication failures, WebSocket errors, etc.)

Example output:
```
[INFO] CSI Simulator starting
[INFO] Configuration: nodes=4, walkers=1, rate=20 Hz, duration=30s
[INFO] Space: 6.0x5.0x2.5 m
[INFO] Connecting to: ws://localhost:8080/ws/node
[INFO] Node AA:BB:CC:DD:E0:00 connected to mothership
[STATS] Node AA:BB:CC:DD:E0:00: 20 frames/s
[STATS] Node AA:BB:CC:DD:E0:00: 20 frames/s
[INFO] Simulation completed successfully
[STATS] Node AA:BB:CC:DD:E0:00: sent 600 frames
```

## Authentication

When the mothership requires authentication (SPAXEL_INSTALL_SECRET is set), you must provide a valid node token. Generate a token for a simulated node using the mothership's provisioning API or derive it manually:

```bash
# Get token from mothership provisioning endpoint
curl -s http://localhost:8080/api/provision -d '{"mac":"AA:BB:CC:DD:E0:00"}'
```

## Error Handling

The simulator exits with non-zero status on:
- WebSocket connection failure
- Authentication rejection (HTTP 401)
- Mothership rejection (`{type:"reject"}` message)

## Testing

Run the test suite:

```bash
cd mothership
go test ./cmd/sim/...
```

Tests cover:
- Space dimension parsing
- MAC address conversion
- CSI frame structure validation
- Fresnel zone modulation
- RSSI calculation
- Walker position updates
