# Simulator Position Data Structure

## Overview

This document describes the position data structures used by the Spaxel simulator (`cmd/sim/main.go`) and how position data flows through the system.

## Core Position Data Structures

### 1. Point Structure (Simulator)

```go
type Point struct {
    X, Y, Z float64
}
```

**Location**: `cmd/sim/main.go:81-83`

**Description**: Represents a 3D position in space. All position data in the simulator uses this structure.

### 2. VirtualNode Structure

```go
type VirtualNode struct {
    ID       int
    MAC      [6]byte
    Position Point
    Conn     *websocket.Conn
    mu       sync.Mutex
}
```

**Location**: `cmd/sim/main.go:65-71`

**Description**: Represents a simulated ESP32 node. The `Position` field contains the node's 3D coordinates.

### 3. Walker Structure

```go
type Walker struct {
    ID       int
    Position Point
    Velocity Point
}
```

**Location**: `cmd/sim/main.go:74-78`

**Description**: Represents a simulated person (walker) that moves through the space. `Position` is current location, `Velocity` is movement vector.

### 4. Space Structure

```go
type Space struct {
    Width, Depth, Height float64
}
```

**Location**: `cmd/sim/main.go:86-88`

**Description**: Represents room dimensions in meters (Width=X, Depth=Y, Height=Z).

## Position Data Fields

### Simulator Hello Message Position Fields

When the simulator connects to the mothership, it sends position data in the hello message:

```json
{
    "type": "hello",
    "mac": "02:53:AC:00:00:01",
    "firmware_version": "sim-1.0.0",
    "capabilities": ["csi", "ble", "tx", "rx"],
    "chip": "ESP32-S3",
    "flash_mb": 16,
    "uptime_ms": 1000,
    "pos_x": 0.0,
    "pos_y": 0.0,
    "pos_z": 2.5
}
```

**Field Descriptions**:
- `pos_x`: X coordinate in meters (width axis)
- `pos_y`: Y coordinate in meters (depth axis)  
- `pos_z`: Z coordinate in meters (height axis)

### Mothership HelloMessage Structure

```go
type HelloMessage struct {
    Type            string   `json:"type"`
    MAC             string   `json:"mac"`
    // ... other fields ...
    
    // PosX/PosY/PosZ carry the node's announced 3D world position, in meters.
    // Pointers (not plain float64) so an *absent* position is distinguishable
    // from a genuine (0,0,0)
    PosX *float64 `json:"pos_x,omitempty"`
    PosY *float64 `json:"pos_y,omitempty"`
    PosZ *float64 `json:"pos_z,omitempty"`
}
```

**Location**: `mothership/internal/ingestion/message.go:11-34`

**Important**: Position fields are pointers (`*float64`) to distinguish between:
- Not announced: `nil` (real ESP32 nodes)
- Origin: `0.0` (valid position)
- Default: Schema default (0, 0, 1) before user positions the node

### NodeRecord Structure (Database)

```go
type NodeRecord struct {
    MAC             string    `json:"mac"`
    Name            string    `json:"name"`
    Role            string    `json:"role"`
    // ... other fields ...
    PosX            float64   `json:"pos_x"`
    PosY            float64   `json:"pos_y"`
    PosZ            float64   `json:"pos_z"`
    Virtual         bool      `json:"virtual"`
    // ... more fields ...
}
```

**Location**: `mothership/internal/fleet/registry.go:28-44`

**Database Schema**:
```sql
CREATE TABLE nodes (
    mac              TEXT PRIMARY KEY,
    pos_x            REAL    NOT NULL DEFAULT 0,
    pos_y            REAL    NOT NULL DEFAULT 0,
    pos_z            REAL    NOT NULL DEFAULT 0,
    -- ... other fields ...
);
```

**Location**: `mothership/internal/fleet/registry.go:94-110`

## Position Data Flow

### 1. Simulator Initialization

1. **Space Definition**: Simulator reads room dimensions from `--space` flag (default "6x5x2.5")
2. **Node Positioning**: `generateNodePositions()` assigns positions to virtual nodes
3. **Walker Creation**: Walkers start at room center with random velocities

### 2. Node Connection Flow

**Location**: `mothership/internal/fleet/manager.go:183-231`

1. **Hello Message**: Simulator sends hello with position fields (pos_x, pos_y, pos_z)
2. **Parse Position**: `OnNodeConnected()` receives posX, posY, posZ as `*float64`
3. **Persist to Registry**: If all three position fields present, call `SetNodePosition(mac, x, y, z)`
4. **Forward to Fusion Engine**: Position forwarded to blob-producing fusion engine via `nodePositionSink`

### 3. Position Update Flow

**Location**: `mothership/internal/fleet/manager.go:167-180`

When a node position changes:
- `ForwardNodePosition(mac, x, y, z)` is called
- Forwards to registered sink (fusion engine) if available
- Ensures engine position stays in sync with fleet registry

## Node Positioning Strategy

### Default Node Positions

**Location**: `cmd/sim/main.go:219-253`

The simulator uses different positioning strategies based on node count:

- **1 node**: Center of room `(width/2, depth/2, height/2)`
- **2 nodes**: Opposite corners at ceiling height `(0,0,height)` and `(width,depth,height)`
- **3 nodes**: Two corners + one midpoint on floor
- **4 nodes**: Four corners at ceiling height
- **5+ nodes**: Grid pattern distributed across the space

### Example Positions (4 nodes, 6x5x2.5m room)

```
Node 0: (0.00, 0.00, 2.50) - Front-left corner, ceiling
Node 1: (6.00, 0.00, 2.50) - Front-right corner, ceiling
Node 2: (0.00, 5.00, 2.50) - Back-left corner, ceiling
Node 3: (6.00, 5.00, 2.50) - Back-right corner, ceiling
```

## Walker Movement

**Location**: `cmd/sim/main.go:692-750`

Walkers use random walk behavior:
1. **Velocity Update**: Gaussian random walk with `σ = 0.3 m/s` per axis
2. **Position Update**: `position += velocity * dt` (dt = 50ms)
3. **Wall Reflection**: Bounce off walls with velocity reversal
4. **Speed Clamp**: Maximum velocity limited to 2.0 m/s

## Coordinate System

- **X axis**: Room width (0 to Width meters)
- **Y axis**: Room depth (0 to Depth meters)
- **Z axis**: Room height (0 to Height meters, 0 = floor)
- **Origin**: (0, 0, 0) = front-left corner on floor

## Key Acceptance Criteria Verification

✅ **Position data structure is documented**: All structures and fields documented above

✅ **Source of position data in simulator is identified**:
- Simulated nodes: `generateNodePositions()` function at `cmd/sim/main.go:219`
- Walkers: `createWalkers()` function at `cmd/sim/main.go:275`
- Position announcements: Hello message at `cmd/sim/main.go:348-359`

✅ **Field names and types are known**:
- Point struct: `X, Y, Z float64`
- Hello message: `pos_x, pos_y, pos_z *float64` (JSON)
- Database: `pos_x, pos_y, pos_z REAL` (SQLite)
- NodeRecord: `PosX, PosY, PosZ float64` (Go)

## Related Files

- `cmd/sim/main.go` - Simulator main implementation
- `mothership/internal/fleet/manager.go` - Fleet manager with position handling
- `mothership/internal/fleet/registry.go` - Node position persistence
- `mothership/internal/ingestion/message.go` - Hello message parsing
- `mothership/cmd/mothership/main.go` - Position loading and engine seeding
