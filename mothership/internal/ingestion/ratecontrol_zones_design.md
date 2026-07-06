# Zone-Aware Rate Control Design

## Overview

Extend the current per-node `RateController` to support fleet-level coordination with zone awareness and sentinel link designation.

## Current State

**RateController** (mothership/internal/ingestion/ratecontrol.go):
- Manages per-node adaptive rates: 50 Hz active, 2 Hz idle
- Per-node state machine: active/idle with 30-second timeout
- OnMotionHint ramps adjacent nodes (already has adjacency function)
- No zone awareness, no fleet-level coordination

## New Requirements (Component 24)

1. **Zone-level idle detection**: When all nodes in a zone have been idle, designate one sentinel link per zone at 5 Hz; all others drop to 1 Hz
2. **Fleet-wide idle state**: When all zones are idle, only sentinel links at 5 Hz run
3. **Adjacent zone ramping**: When activity detected in one zone, ramp that zone to full rate AND adjacent zones to 5 Hz
4. **Prediction engine interface**: RampZone(zoneID) for preemptive zone ramping

## Design

### Rate Tiers

```go
const (
    // RateActive is the CSI sampling rate (Hz) when motion is detected.
    RateActive = 50
    
    // RateIdle is the CSI sampling rate (Hz) when a node is idle but fleet is not fully idle.
    RateIdle = 2
    
    // RateSentinel is the CSI sampling rate (Hz) for the designated sentinel link in an idle zone.
    // Runs at 5 Hz to maintain minimal coverage while reducing bandwidth.
    RateSentinel = 5
    
    // RateFleetIdle is the CSI sampling rate (Hz) for non-sentinel links when the entire fleet is idle.
    // Only one sentinel per zone runs at RateSentinel; all others run at 1 Hz.
    RateFleetIdle = 1
)
```

### Data Structures

```go
// zoneRateState tracks the adaptive rate state for a single zone.
type zoneRateState struct {
    zoneID          string
    nodes           map[string]bool // nodes in this zone
    sentinelNodeMAC string          // designated sentinel node (may be empty)
    allNodesIdle    bool            // true when all nodes in zone are idle
    lastMotionAt    time.Time       // last motion in this zone
}

// RateController manages per-node and zone-aware adaptive sensing rates.
type RateController struct {
    // Existing fields
    mu            sync.Mutex
    nodes         map[string]*nodeRateState
    configSender  func(nodeMAC string, rateHz int, varianceThreshold float64)
    adjacentNodes func(nodeMAC string) []string
    
    // New zone-aware fields
    zones                  map[string]*zoneRateState // keyed by zone ID
    zoneMembership         func(nodeMAC string) []string // returns zone IDs for a node
    adjacentZones          func(zoneID string) []string // returns zone IDs adjacent to a zone
    fleetIdle              bool // true when all zones are idle
}
```

### Zone Membership Algorithm

**Determining which zones a node belongs to:**
- A node is in a zone if its position (X, Y, Z) is within the zone's bounds
- Zone bounds: (min_x <= node.X <= max_x) && (min_y <= node.Y <= max_y) && (min_z <= node.Z <= max_z)
- A node can be in multiple zones (overlapping zones)
- Use zone membership provider function from zone manager

**Zone membership provider signature:**
```go
// ZoneMembershipFn returns the zone IDs that a node belongs to based on its position.
// The caller must provide this function; it typically queries the zone manager.
type ZoneMembershipFn func(nodeMAC string) []string
```

### Sentinel Designation Algorithm

**Choosing the sentinel node for a zone:**
- When a zone becomes idle, designate one node as the sentinel
- Selection criteria: the node with the lexicographically smallest MAC address (deterministic)
- Sentinel gets RateSentinel (5 Hz), all other nodes in zone get RateFleetIdle (1 Hz)
- Redesignate only when previous sentinel disconnects

**Zone idle detection:**
- Zone is idle when all its nodes have been idle for the timeout period
- Track last motion time per zone (max of last motion times of all nodes in zone)
- When zone transitions to idle, designate sentinel and adjust rates

### Fleet Idle Detection

**Fleet is idle when:**
- All zones are in the idle state (allNodesIdle = true)
- OR no zones exist (fallback to per-node behavior)

**When fleet becomes idle:**
- All zones: sentinel runs at 5 Hz, others at 1 Hz
- When fleet was idle, one zone becomes active: that zone at 50 Hz, adjacent zones at 5 Hz

### Adjacent Zone Ramping

**When motion is detected in a zone:**
- Ramp that zone's nodes to RateActive (50 Hz)
- Ramp adjacent zones' nodes to RateSentinel (5 Hz) - preemptive coverage
- Adjacent zones determined by portals in zone manager

**Adjacent zones provider signature:**
```go
// AdjacentZonesFn returns zone IDs that are adjacent to a given zone.
// The caller must provide this function; it typically queries the zone manager for portals.
type AdjacentZonesFn func(zoneID string) []string
```

### Prediction Engine Interface

**RampZone(zoneID) method:**
- Called by prediction engine when P(arrival in zone) > threshold
- Ramps all nodes in the zone to RateActive (50 Hz)
- Resets zone's lastMotionAt to now
- Optionally ramps adjacent zones to RateSentinel (5 Hz)

```go
// RampZone preemptively ramps all nodes in a zone to active rate.
// Called by the prediction engine when a zone arrival is predicted.
func (rc *RateController) RampZone(zoneID string) {
    // Set all nodes in zone to active
    // Update zone state
    // Optionally ramp adjacent zones to sentinel
}
```

### State Machine

**Node state:**
- Active (50 Hz) → Idle (2 Hz) after 30s timeout
- Idle → Active immediately on motion detection
- Idle (2 Hz) → Fleet Idle (1 Hz) when fleet becomes idle AND not sentinel
- Idle (2 Hz) → Sentinel (5 Hz) when fleet becomes idle AND is sentinel

**Zone state:**
- All nodes idle → Zone idle
- Zone idle → Zone active when any node detects motion
- Zone active → Zone idle after timeout (same 30s)

**Fleet state:**
- All zones idle → Fleet idle
- Fleet idle → Fleet active when any zone becomes active

## Integration Points

### Required from callers:

1. **Node position provider**: Function to get a node's (X, Y, Z) position
   - Already available from fusion.Engine.NodePositions()
   - Used to determine zone membership

2. **Zone membership provider**: Function to get zone IDs for a node
   - Queries zone manager for zones containing the node's position
   - Returns []string (may be empty for nodes in no zones)

3. **Adjacent zones provider**: Function to get adjacent zone IDs
   - Queries zone manager for portals connected to a zone
   - Returns []string (may be empty for isolated zones)

4. **Zone bounds provider**: Function to get zone bounds
   - Queries zone manager for zone (min_x, min_y, min_z, max_x, max_y, max_z)
   - Used for spatial containment test

## Implementation Plan

1. Add new rate constants (RateSentinel, RateFleetIdle)
2. Add zoneRateState struct
3. Add zone-related fields to RateController
4. Implement SetZoneMembershipFn() and SetAdjacentZonesFn()
5. Implement zone membership tracking in OnMotionState()
6. Implement zone idle detection in checkIdleTimeouts()
7. Implement sentinel designation logic
8. Implement fleet idle detection
9. Implement RampZone() method
10. Implement adjacent zone ramping in OnMotionState()
11. Update tests to cover zone-aware behavior

## Backward Compatibility

- If no zone membership function is set, fall back to per-node behavior only
- Existing tests should continue to pass
- Zone-aware features are opt-in via SetZoneMembershipFn()

## Testing Strategy

1. Test zone membership determination
2. Test zone idle detection
3. Test sentinel designation
4. Test fleet idle detection
5. Test adjacent zone ramping
6. Test RampZone() prediction engine interface
7. Test fallback to per-node behavior when zones not configured
8. Test node disconnection handles sentinel redesignation
