# Simulator-to-Registry Position Wiring Trace

## Overview

This document traces the complete data flow for position data from virtual node creation in the simulator to persistence in the fleet registry. There are **two distinct paths** depending on which simulator is being used.

## Path 1: CSI Simulator (cmd/sim/main.go)

### 1. Position Creation
**File**: `/home/coding/spaxel/cmd/sim/main.go`

Virtual nodes are created with computed positions using `generateNodePositions()`:
- Single node: center of room
- Two nodes: opposite corners (spans both X and Y)
- 3+ nodes: row-major grid with alternating Z heights

```go
// Lines 198-253
func createVirtualNodes(count int, space *Space, rng *rand.Rand) []*VirtualNode {
    positions := generateNodePositions(count, space)
    // ... creates VirtualNode structs with Position field
}
```

### 2. Hello Message Transmission
**File**: `/home/coding/spaxel/cmd/sim/main.go` (Lines 345-365)

The simulator connects to mothership via WebSocket and sends a hello message:
```go
hello := map[string]interface{}{
    "type":            "hello",
    "mac":             macToString(n.MAC),
    "pos_x":           n.Position.X,
    "pos_y":           n.Position.Y,
    "pos_z":           n.Position.Z,
    // ... other fields
}
```

### 3. Hello Message Reception
**File**: `/home/coding/spaxel/mothership/internal/ingestion/message.go` (Lines 10-34)

The `HelloMessage` struct defines the position fields as pointers:
```go
type HelloMessage struct {
    // ...
    PosX *float64 `json:"pos_x,omitempty"`
    PosY *float64 `json:"pos_y,omitempty"`
    PosZ *float64 `json:"pos_z,omitempty"`
}
```
Pointers distinguish "not announced" (nil) from "position is (0,0,0)".

### 4. Fleet Notification
**File**: `/home/coding/spaxel/mothership/internal/ingestion/server.go` (Lines 26-37)

The ingestion server calls the `FleetNotifier` callback:
```go
type FleetNotifier interface {
    OnNodeConnected(mac, firmware, chip string, posX, posY, posZ *float64)
    OnNodeDisconnected(mac string)
}
```

### 5. Registry Persistence
**File**: `/home/coding/spaxel/mothership/internal/fleet/manager.go` (Lines 182-231)

The Fleet Manager's `OnNodeConnected()` method:
1. Upserts the node record: `registry.UpsertNode(mac, firmware, chip)`
2. Persists announced position (if all three axes present):
   ```go
   if posX != nil && posY != nil && posZ != nil {
       registry.SetNodePosition(mac, *posX, *posY, *posZ)
   }
   ```
3. Forwards position to fusion engine via `nodePositionSink`

### 6. Storage
**File**: `/home/coding/spaxel/mothership/internal/fleet/registry.go` (Lines 179-183)

Positions are stored in SQLite:
```go
func (r *Registry) SetNodePosition(mac string, x, y, z float64) error {
    _, err := r.db.Exec(`UPDATE nodes SET pos_x=?, pos_y=?, pos_z=? WHERE mac=?`, 
                       x, y, z, mac)
    return err
}
```

## Path 2: Virtual Node Planning Simulator (mothership/internal/simulator/)

This path is used for planning and coverage optimization, distinct from the CSI simulator.

### 1. Virtual Node Store
**File**: `/home/coding/spaxel/mothership/internal/simulator/virtual_state.go`

Virtual nodes are created and managed in `VirtualNodeStore`:
```go
func (s *VirtualNodeStore) CreateVirtualNode(id, name string, position Point) 
```

### 2. Registry Bridge
**File**: `/home/coding/spaxel/mothership/internal/simulator/registry_bridge.go`

The `FleetRegistryBridge` integrates virtual nodes with the fleet registry:

**Key Method - effectivePositions()** (Lines 50-99):
- Resolves final positions for all nodes
- Nodes at default origin (0,0,1) are reassigned spread-out geometry via `DefaultNodePositions()`
- Explicitly-placed nodes keep their position
- Deterministic assignment: sorted by ID, no collisions

**Sync Method** (Lines 131-189):
```go
func (b *FleetRegistryBridge) SyncToRegistry(registry RegistryNodeAdapter) error
```
- Lists all nodes from store
- Resolves effective positions
- Creates or updates registry entries
- Sets position and role

### 3. Registry Adapter Interface
**File**: `/home/coding/spaxel/mothership/internal/simulator/registry_bridge.go` (Lines 109-129)

```go
type RegistryNodeAdapter interface {
    AddVirtualNode(mac, name string, x, y, z float64) error
    SetNodePosition(mac string, x, y, z float64) error
    SetNodeRole(mac, role string) error
    DeleteNode(mac string) error
    GetNode(mac string) (*NodeRecord, error)
    GetAllNodes() ([]NodeRecord, error)
}
```

### 4. Fleet Registry Implementation
**File**: `/home/coding/spaxel/mothership/internal/fleet/registry.go` (Lines 273-287)

```go
func (r *Registry) AddVirtualNode(mac, name string, x, y, z float64) error {
    _, err := r.db.Exec(`
        INSERT INTO nodes (mac, name, role, pos_x, pos_y, pos_z, virtual, ...)
        VALUES (?, ?, 'virtual', ?, ?, ?, 1, ...)
    `, mac, name, x, y, z, ...)
    return err
}
```

## Fusion Engine Integration

The registry positions flow to the 3D fusion engine for blob generation:

**File**: `/home/coding/spaxel/mothership/internal/fleet/manager.go` (Lines 110-120, 173-180)

```go
// Sink for position updates
nodePositionSink func(mac string, x, y, z float64)

func (m *Manager) ForwardNodePosition(mac string, x, y, z float64) {
    if sink != nil {
        sink(mac, x, y, z)
    }
}
```

This is called from:
1. `OnNodeConnected()` after persisting hello-announced position
2. REST API handler after PATCH /api/nodes/{mac}/position

**File**: `/home/coding/spaxel/mothership/internal/fusion/fusion.go` (Lines 64-73)

The fusion engine maintains a node position map:
```go
type Engine struct {
    nodePos map[string]NodePosition
    // ...
}
```

## Key Files in Pipeline

1. **Simulator CLI**: `/home/coding/spaxel/cmd/sim/main.go`
2. **Hello Message**: `/home/coding/spaxel/mothership/internal/ingestion/message.go`
3. **Ingestion Server**: `/home/coding/spaxel/mothership/internal/ingestion/server.go`
4. **Fleet Manager**: `/home/coding/spaxel/mothership/internal/fleet/manager.go`
5. **Fleet Registry**: `/home/coding/spaxel/mothership/internal/fleet/registry.go`
6. **Virtual Node Store**: `/home/coding/spaxel/mothership/internal/simulator/virtual_state.go`
7. **Registry Bridge**: `/home/coding/spaxel/mothership/internal/simulator/registry_bridge.go`
8. **Fusion Engine**: `/home/coding/spaxel/mothership/internal/fusion/fusion.go`

## Verified Data Flow

✅ **Complete Path 1 (CSI Simulator)**:
1. `cmd/sim/main.go:generateNodePositions()` - computes positions
2. `cmd/sim/main.go:connectNodes()` - sends hello with pos_x/y/z
3. `ingestion/server.go` - receives WebSocket hello message
4. `fleet/manager.go:OnNodeConnected()` - persists position
5. `fleet/registry.go:SetNodePosition()` - stores in SQLite
6. `fleet/manager.go:ForwardNodePosition()` - forwards to fusion engine

✅ **Complete Path 2 (Virtual Node Planning)**:
1. `simulator/virtual_state.go:CreateVirtualNode()` - creates virtual node
2. `simulator/registry_bridge.go:SyncToRegistry()` - syncs to registry
3. `fleet/registry.go:AddVirtualNode()` - stores with position
4. Position available to fusion engine via registry queries

## Integration Points

- **Ingestion → Fleet**: `FleetNotifier.OnNodeConnected(mac, firmware, chip, posX, posY, posZ)`
- **Fleet → Fusion**: `nodePositionSink(mac, x, y, z)` callback
- **Simulator Bridge → Registry**: `RegistryNodeAdapter` interface

## Acceptance Criteria Status

✅ Complete data flow path documented from simulator to registry
✅ Two distinct paths identified and documented
✅ List of files involved in the simulator-to-registry pipeline provided
