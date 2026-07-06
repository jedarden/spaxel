# Position Format Validation: Registry vs Simulator

## Overview

Analysis of position data flow from simulator → registry_bridge → fleet registry identified a **coordinate system mismatch** between simulator and registry coordinate systems.

## Registry's Expected Position Format

### Database Schema (SQLite)
```sql
CREATE TABLE IF NOT EXISTS nodes (
    mac              TEXT PRIMARY KEY,
    ...
    pos_x            REAL    NOT NULL DEFAULT 0,
    pos_y            REAL    NOT NULL DEFAULT 0,
    pos_z            REAL    NOT NULL DEFAULT 0,
    ...
);
```

### Registry Coordinate System (from `internal/fleet/registry.go`)
```go
type RoomConfig struct {
    Width   float64 // meters, X axis
    Depth   float64 // meters, Z axis
    Height  float64 // meters, Y axis
    OriginX float64
    OriginZ float64
}
```

**Registry coordinate convention:**
- `pos_x` = width (horizontal X axis)
- `pos_y` = height (vertical Y axis)
- `pos_z` = depth (horizontal Z axis)

### Registry API Interface
```go
type NodeRegistry interface {
    SetNodePosition(mac string, x, y, z float64) error
    AddVirtualNode(mac, name string, x, y, z float64) error
}
```

## Simulator's Position Format

### Simulator Data Structure
```go
type Point struct{ X, Y, Z float64 }
```

### Simulator Coordinate System (from `internal/simulator/space.go`)
DefaultSpace room bounds:
```go
Room{
    MinX: 0, MinY: 0, MinZ: 0,
    MaxX: 6, MaxY: 5, MaxZ: 2.5,  // 6m × 5m × 2.5m room
}
```

**Simulator coordinate convention:**
- `Point.X` = width (horizontal X axis)
- `Point.Y` = depth (horizontal Y axis)
- `Point.Z` = height (vertical Z axis)

### VirtualNodeState Storage
```go
type VirtualNodeState struct {
    ID       string
    Name     string
    Position Point     // {X, Y, Z}
    ...
}
```

## Format Mismatch Identified

### Coordinate System Difference

| Axis | Simulator (Point) | Registry (pos_*) | Match? |
|------|-------------------|------------------|--------|
| X    | width             | width            | ✅     |
| Y    | depth             | height           | ❌     |
| Z    | height            | depth            | ❌     |

**The Y and Z axes are swapped between the two systems!**

### Current Registry Bridge Implementation

From `internal/simulator/registry_bridge.go`:
```go
func (b *FleetRegistryBridge) SyncToRegistry(registry RegistryNodeAdapter) error {
    for _, node := range nodes {
        mac := b.virtualMAC(node.ID)
        pos := positions[node.ID]  // Point{X, Y, Z}
        
        // Direct field access - NO COORDINATE TRANSFORMATION
        registry.AddVirtualNode(mac, node.Name, pos.X, pos.Y, pos.Z)
        //                                     ^^^^^^^^^^^^^^^^
        //                                     Y and Z are NOT swapped!
    }
}
```

**This is incorrect!** The simulator's Y (depth) and Z (height) are being passed directly to the registry, which expects Y as height and Z as depth.

## Required Transformations

### Coordinate Mapping

To correctly transform simulator positions to registry positions:

```go
// Simulator Point → Registry position
registry_x = simulator_point.X     // width → width (no change)
registry_y = simulator_point.Z     // height → height (swapped!)
registry_z = simulator_point.Y     // depth → depth (swapped!)
```

### Corrected Implementation

```go
func (b *FleetRegistryBridge) SyncToRegistry(registry RegistryNodeAdapter) error {
    for _, node := range nodes {
        mac := b.virtualMAC(node.ID)
        pos := positions[node.ID]
        
        // CORRECT: Swap Y and Z for registry coordinate system
        registry.AddVirtualNode(mac, node.Name, 
            pos.X,    // width (no change)
            pos.Z,    // height (was Point.Z)
            pos.Y,    // depth (was Point.Y)
        )
    }
}
```

### Example Transformation

**Simulator position:** Point{X: 3.0, Y: 2.5, Z: 1.5}
- X=3.0m (width position)
- Y=2.5m (depth position)
- Z=1.5m (height position)

**Registry position (corrected):** pos_x=3.0, pos_y=1.5, pos_z=2.5
- pos_x=3.0m (width position) ✅
- pos_y=1.5m (height position) ✅
- pos_z=2.5m (depth position) ✅

**Registry position (current implementation):** pos_x=3.0, pos_y=2.5, pos_z=1.5
- pos_x=3.0m (width position) ✅
- pos_y=2.5m (INCORRECT - treating depth as height!)
- pos_z=1.5m (INCORRECT - treating height as depth!)

## Impact Analysis

### Current Behavior
- Virtual nodes from simulator are written to registry with **swapped Y/Z coordinates**
- Registry interprets simulator's depth (Y) as height
- Registry interprets simulator's height (Z) as depth
- This affects:
  - Fusion engine positioning calculations
  - Coverage optimization algorithms
  - Any registry consumers that use 3D positions

### Risk Assessment
- **High risk**: Coordinate system mismatch causes incorrect spatial positioning
- **Affected systems**: Fusion engine, coverage optimization, position-based tracking
- **Data corruption risk**: Existing registry entries may have incorrect coordinates

## Recommendations

1. **Immediate**: Fix coordinate transformation in `registry_bridge.go`
2. **Data cleanup**: Script to correct existing registry node positions
3. **Testing**: Add integration tests validating coordinate transformation
4. **Documentation**: Document coordinate systems in both codebases

## Related Files

- `mothership/internal/simulator/registry_bridge.go` - Bridge implementation (needs fix)
- `mothership/internal/simulator/space.go` - Simulator coordinate system
- `mothership/internal/fleet/registry.go` - Registry coordinate system
- `mothership/internal/simulator/virtual_state.go` - Virtual node storage
