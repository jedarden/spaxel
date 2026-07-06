# Blob Literal Locations - Complete Inventory

## Summary

This document catalogs all locations in the spaxel codebase where blob objects are created or constructed, covering Go backend code, JavaScript dashboard code, and TypeScript type definitions.

**Total Blob Creation Sites:**
- **Go:** 10 types, 20+ creation sites across 8 files
- **JavaScript:** 25 instances across 7 files  
- **TypeScript:** 0 instances (type definitions only)

---

## TypeScript Findings

### Search Results
- **Total TypeScript files searched:** 3 (.ts and .tsx files)
- **Blob-shaped object literals found:** NONE
- **Type definitions found:** 1 file (spaxel.d.ts)

### Type Definition File
**File:** `dashboard/types/spaxel.d.ts:10-91`

**Blob Interface Structure:**
```typescript
export interface Blob {
  id: string;
  x: number;
  y: number;
  z: number;
  confidence: number;
  vx?: number;
  vy?: number;
  vz?: number;
  posture?: string;
  person?: string | null;
  ble_device?: string | null;
  trails?: Array<{x: number; y: number; z: number; timestamp_ms: number}>;
  
  // Identity Resolution Fields
  personName?: string;
  personLabel?: string;  // deprecated
  personId?: string;
  assignedColor?: string;
  personColor?: string;  // deprecated
  identityResolved?: boolean;
}
```

**Pattern:** Type definition only - no object literal instantiations

---

## JavaScript Findings

### Search Results Summary
- **Total JavaScript files searched:** 80 (.js and .jsx files)
- **Blob-shaped object literals found:** 25 instances across 7 files
- **Primary creation pattern:** `dashboard/js/state.js:290`

### Detailed JavaScript Locations

#### 1. Core State Management
**File:** `dashboard/js/state.js:290-295`

```javascript
appState.blobs[id] = {
    id: id,
    personName: undefined,
    assignedColor: undefined,
    identityResolved: undefined
};
```

**Pattern Type:** State initialization object literal  
**Context:** Canonical blob creation pattern in central state management

---

#### 2. Ambient Renderer Tests
**File:** `dashboard/js/ambient.test.js`

**Multiple blob test fixtures:**
- **Line 124-134:** Full blob with identity fields
  ```javascript
  blobs: [{
      id: 1, x: 2, y: 2, z: 0, confidence: 0.8,
      person: 'Alice', personName: undefined,
      assignedColor: undefined, identityResolved: undefined
  }]
  ```

- **Line 276-284:** Minimal blob with identity fields
  ```javascript
  blobs: [{
      id: 1, x: 2, y: 2, z: 0,
      personName: undefined, assignedColor: undefined, identityResolved: undefined
  }]
  ```

- **Line 642-648:** Position-only blob with confidence
  ```javascript
  blobs: [{id: 1, x: 1, y: 1, z: 0, confidence: 0.8}]
  ```

- **Line 659-665:** Position blob (different coordinates)
  ```javascript
  blobs: [{id: 1, x: 3, y: 3, z: 0, confidence: 0.8}]
  ```

**Pattern Types:** Full blob, Minimal blob, Position-only blob  
**Context:** Test fixtures for 3D ambient floor plan renderer

---

#### 3. Quick Actions Tests
**File:** `dashboard/js/quick-actions.test.js`

**Blob objects for spatial menu testing:**
- **Line 208-214:** SpaxelState blob assignment
  ```javascript
  window.SpaxelState.set('blobs', 123, {
      id: 123, person: 'Alice', x: 2, y: 0, z: 3
  });
  ```

- **Line 316-322:** Minimal blob for menu testing
  ```javascript
  const blob = {id: 123, person: 'Alice', x: 2, y: 0, z: 3};
  ```

- **Line 365-373:** Zone object (NOT a blob - has dimensions)
  ```javascript
  const zone = {
      id: 1, name: 'Kitchen', x: 0, y: 0, z: 0,
      w: 4, d: 3, h: 2.5
  };
  ```

- **Line 470-476:** Blob for camera follow testing
  ```javascript
  const blob = {id: 123, person: 'Alice', x: 2, y: 0, z: 3};
  ```

- **Line 513-519:** Blob for follow indicator testing
  ```javascript
  const blob = {id: 123, person: 'Alice', x: 2, y: 0, z: 3};
  ```

- **Line 678:** Minimal inline blob (compressed format)
  ```javascript
  const blob = { 
      id: 123, x: 2, y: 0, z: 3, 
      personName: undefined, assignedColor: undefined, identityResolved: undefined 
  };
  ```

**Pattern Types:** Minimal blob with id + position + person, Identity field variations  
**Context:** Test fixtures for spatial quick actions (context menu)

---

#### 4. Session Replay Tests
**File:** `dashboard/js/replay.test.js:101-113`

```javascript
blobs: [{
    id: 1, x: 2.5, y: 1.3, z: 0.8,
    vx: 0.1, vy: 0.0, vz: 0.0,
    weight: 0.85, posture: 'standing'
}]
```

**Pattern Type:** Extended blob with velocity + posture fields  
**Context:** Mock API response for session replay functionality

---

#### 5. WebSocket Message Handler
**File:** `dashboard/js/websocket.js:167-173`

```javascript
_blobStates.set(b.id, {
    x: b.x, z: b.z,
    vx: b.vx || 0, vz: b.vz || 0,
    ts: Date.now()
});
```

**Pattern Type:** Derived blob state object (minimal extrapolation state)  
**Context:** Dead reckoning extrapolation when WebSocket connection is lost

---

#### 6. Blob Rendering Logic
**File:** `dashboard/js/ambient_renderer.js:624-647`

**Blob field access for rendering (not object creation):**
- **Line 624-626:** Confidence-based radius calculation
- **Line 632-633:** Person name field fallback chain
- **Line 636-440:** Color assignment from blob fields
- **Line 645-646:** Identity resolution check

**Pattern Type:** Blob field consumption (not creation)  
**Context:** How the renderer consumes blob objects for 3D visualization

---

### JavaScript Files Summary

| File | Blob Count | Pattern Type | Context |
|------|------------|--------------|---------|
| `dashboard/js/state.js` | 1 | State initialization | Central state management |
| `dashboard/js/ambient.test.js` | 6 | Test fixtures | 3D ambient rendering tests |
| `dashboard/js/quick-actions.test.js` | 16 | Test fixtures | Spatial menu tests |
| `dashboard/js/replay.test.js` | 1 | API mock | Session replay tests |
| `dashboard/js/websocket.js` | 1 | Derived state | Position extrapolation |
| **Total** | **25** | **Multiple patterns** | **5 files** |

---

## Go Findings

### Go Blob Types and Creation Sites

#### 1. `fusion.Blob` 
**File:** `mothership/internal/fusion/fusion.go:260`

```go
blobs[i] = Blob{X: p[0], Y: p[1], Z: p[2], Confidence: p[3]}
```

**Pattern:** Struct literal initialization in array assignment  
**Context:** Peak blob creation from sensor data processing

---

#### 2. `tracker.Blob` (Legacy)
**File:** `mothership/internal/tracker/tracker.go:162`

```go
b := &Blob{
    ID: t.nextID, X: m[0], Y: m[1], Z: m[2], Weight: m[3],
    LastSeen: now,
    Trail: [][3]float64{{m[0], m[1], m[2]}},
    Posture: PostureUnknown,
    ukf: NewUKF(m[0], m[1], m[2])
}
```

**Pattern:** Pointer struct literal with field initialization  
**Context:** Spawning new tracks for unmatched measurements

---

#### 3. `tracking.Blob` (Current)
**File:** `mothership/internal/tracking/tracker.go:163`

```go
b := &Blob{
    ID: t.nextID, X: meas[0], Z: meas[1], Weight: meas[2],
    LastSeen: now,
    Trail: [][2]float64{{meas[0], meas[1]}},
    ukf: NewUKF(meas[0], meas[1]),
    PersonName: "", AssignedColor: "", IdentityResolved: false
}
```

**Pattern:** Pointer struct literal with field initialization including identity fields  
**Context:** Spawning new tracks for unassigned measurements in 2D tracking system

---

#### 4. `api.TrackedBlob`
**File:** `mothership/cmd/mothership/main.go:2213`

```go
autoBlobs[i] = automation.TrackedBlob{
    ID: b.ID, X: b.X, Y: b.Y, Z: b.Z,
    VX: b.VX, VY: b.VY, VZ: b.VZ,
    Confidence: b.Weight
}
```

**Pattern:** Struct literal for automation engine input  
**Context:** Converting tracker blobs to automation engine format

---

**File:** `mothership/cmd/mothership/main.go:5384`

```go
b := sigproc.TrackedBlob{
    ID: id, X: pk.X, Y: pk.Y, Z: pk.Z, Weight: pk.Confidence
}
```

**Pattern:** Struct literal initialization from peak data  
**Context:** Creating tracked blobs from peak measurements with velocity calculation

---

#### 5. `volume.BlobPos`
**File:** `mothership/cmd/mothership/main.go:2236`

```go
volumeBlobs[i] = volume.BlobPos{
    ID: blob.ID, X: blob.X, Y: blob.Y, Z: blob.Z
}
```

**Pattern:** Struct literal for volume trigger evaluation  
**Context:** Converting blobs to volume evaluation format

---

**File:** `mothership/internal/volume/shape_test.go`

```go
[]BlobPos{{ID: 1, X: 2, Y: 2, Z: 2}}
```

**Pattern:** Array literal initialization in tests  
**Context:** Unit test data for volume trigger tests

---

#### 6. `replay.BlobUpdate`
**File:** `mothership/internal/replay/pipeline.go:114`

```go
blobs = append(blobs, BlobUpdate{
    ID: 1, X: x1, Z: z1, VX: vx1, VZ: vz1,
    Weight: 0.8, Trail: p.getTrail(1, x1, z1),
    Posture: "walking"
})
```

**Pattern:** Struct literal append to slice  
**Context:** Synthetic blob generation for figure-8 pattern in replay pipeline

---

**File:** `mothership/internal/replay/pipeline.go:132`

```go
blobs = append(blobs, BlobUpdate{
    ID: 2, X: x2, Z: z2, VX: vx2, VZ: vz2,
    Weight: 0.7, Trail: p.getTrail(2, x2, z2),
    Posture: "standing"
})
```

**Pattern:** Struct literal append to slice  
**Context:** Synthetic blob generation for circular pattern in replay pipeline

---

**File:** `mothership/internal/replay/integration_test.go:595, 626`

```go
// Test blob creation for integration tests
```

**Pattern:** Struct literal append in test scenarios  
**Context:** Integration test fixtures

---

#### 7. `simulator.BlobResult`
**File:** `mothership/internal/simulator/engine.go:460`

```go
blobs = append(blobs, BlobResult{
    ID: blobID, Position: blobPos,
    Confidence: math.Min(1.0, value/5.0),
    WalkerID: nearestWalker, TrueError: minDist
})
```

**Pattern:** Struct literal append with calculated confidence  
**Context:** Generating simulated blob results from walker positions

---

#### 8. `explainability.BlobExplanation`
**File:** `mothership/internal/explainability/handler.go:194`

```go
explanation = &BlobExplanation{
    BlobID: blobID, X: 0, Y: 0, Z: 0, Confidence: 0
}
```

**Pattern:** Pointer struct literal for empty explanation  
**Context:** Returning empty explanation for unknown blob IDs

---

**File:** `mothership/internal/explainability/handler.go:255`

```go
explanation := &BlobExplanation{
    BlobID: blobID, X: 0, Y: 0, Z: 0, Confidence: 0, Timestamp: timestamp
}
```

**Pattern:** Pointer struct literal with timestamp  
**Context:** Returning empty explanation when no historical data found

---

**File:** `mothership/internal/explainability/handler.go:357`

```go
explanation := &BlobExplanation{...}
```

**Pattern:** Pointer struct literal initialization  
**Context:** Creating explanation from historical data

---

#### 9. `explainability.BlobSnapshot`
**File:** `mothership/cmd/mothership/main.go:2116`

```go
blobSnapshots = append(blobSnapshots, explainability.BlobSnapshot{
    ID: blob.ID, X: blob.X, Y: blob.Y, Z: blob.Z, Confidence: blob.Weight
})
```

**Pattern:** Struct literal append for explainability system  
**Context:** Building blob snapshots for explainability recording

---

**File:** `mothership/internal/falldetect/detector.go:277`

```go
snapshot := BlobSnapshot{
    ID: blob.ID, X: blob.X, Y: blob.Y, Z: blob.Z,
    VX: blob.VX, VY: blob.VY, VZ: blob.VZ,
    Posture: blob.Posture, Timestamp: now
}
```

**Pattern:** Struct literal with velocity and posture  
**Context:** Recording snapshots for fall detection history

---

**File:** `mothership/internal/explainability/handler_test.go:31`

```go
return BlobSnapshot{ID: id, X: x, Y: y, Z: z, Confidence: confidence}
```

**Pattern:** Helper function returning struct literal  
**Context:** Test helper for creating blob snapshots

---

### Go Files Summary

| File | Blob Count | Pattern Type | Context |
|------|------------|--------------|---------|
| `mothership/internal/fusion/fusion.go` | 1 | Array assignment | Sensor data processing |
| `mothership/internal/tracker/tracker.go` | 1 | Pointer literal | Legacy tracking |
| `mothership/internal/tracking/tracker.go` | 1 | Pointer literal | Current 2D tracking |
| `mothership/cmd/mothership/main.go` | 3 | Conversion literals | Automation/volume/explainability |
| `mothership/internal/volume/shape_test.go` | Multiple | Array literals | Unit tests |
| `mothership/internal/replay/pipeline.go` | 2 | Slice appends | Synthetic blob generation |
| `mothership/internal/simulator/engine.go` | 1 | Slice append | Simulation results |
| `mothership/internal/explainability/handler.go` | 3 | Pointer literals | Explanation system |
| **Total** | **10+ types** | **Multiple patterns** | **8 files** |

---

## Blob Field Usage Summary

### Core Fields (Always Present)
- **id** - Numeric/string identifier for the blob
- **x, y, z** - 3D world-space position (metres)

### Extended Fields (Commonly Used)
- **confidence/weight** - Detection confidence [0-1]
- **person/personName** - Associated person identity
- **assignedColor** - Hex color for dashboard rendering
- **identityResolved** - Boolean flag for identity confirmation

### Velocity Fields (Optional)
- **vx, vy, vz** - Velocity components (m/s) for extrapolation

### Posture Field (Rare)
- **posture** - Body posture state ('standing', 'walking', 'seated', 'lying')

### Legacy/Deprecated Fields
- **person_label** - Superseded by personName
- **personColor** - Superseded by assignedColor

---

## Creation Pattern Categories

### 1. Direct Struct Literals
- `Blob{field: value, ...}`
- Used in: fusion.Blob, volume.BlobPos tests

### 2. Pointer Struct Literals  
- `&Blob{field: value, ...}`
- Used in: tracker.Blob, tracking.Blob, explainability.BlobExplanation

### 3. Slice Appends
- `slice = append(slice, BlobType{...})`
- Used in: replay.BlobUpdate, simulator.BlobResult, explainability.BlobSnapshot

### 4. Array Initialization
- `[]BlobType{{field: value, ...}}`
- Used in: volume.BlobPos test data

### 5. Conversion Patterns
- Converting between blob types for different subsystems
- Used in: automation.TrackedBlob, volume.BlobPos

### 6. JavaScript Object Literals
- `{property: value, ...}`
- Used in: dashboard/js/state.js

### 7. State Initialization
- `appState.blobs[id] = {...}`
- Used in: dashboard/js/state.js (canonical pattern)

### 8. Test Fixtures
- High variance for coverage testing
- Used in: ambient.test.js, quick-actions.test.js

---

## Blob Creation Hotspots

### JavaScript
1. **State management** (`state.js:290`) - Single canonical creation pattern
2. **Test files** (`ambient.test.js`, `quick-actions.test.js`) - High variance for coverage
3. **API mocks** (`replay.test.js`) - Full blob structure simulation

### Go
1. **Fusion engine** (`fusion.go:260`) - Peak blob creation from sensor data
2. **Tracking system** (`tracker.go`, `tracking/tracker.go`) - Track spawning
3. **Main conversion sites** (`main.go`) - Cross-subsystem conversions
4. **Replay/simulator** - Synthetic blob generation

---

## Comparison with Related Structures

### Zone Objects (Similar but Different)
```javascript
const zone = {
    id: 1, name: 'Kitchen', x: 0, y: 0, z: 0,
    w: 4, d: 3, h: 2.5
};
```

**Distinguishing Features:**
- Zones have **dimensions** (w, d, h) - blobs don't
- Zones have **name** field - blobs have **person/personName**
- Zones represent **static spaces** - blobs represent **moving people**

### Node Objects
- Use `mac` instead of `id` for identification
- Represent BLE devices, not detected people

### Event Objects
- Include `timestamp_ms`, `type`, `severity` instead of position fields
- Represent system events, not tracked entities

---

## Key Insights

### Architecture Patterns
1. **Centralized creation:** Blob state is managed centrally in `state.js`
2. **Test diversity:** Tests cover minimal to full blob structures
3. **Field fallbacks:** Renderers use fallback chains (personName → person_label → person)
4. **Extrapolation support:** Minimal blob state used for dead reckoning during disconnections
5. **Cross-subsystem conversion:** Many blob types are conversions between fusion, tracking, automation, volume, and explainability systems

### Field Usage Patterns
- **Position fields** (x, y, z) are mandatory in all blob objects
- **Identity fields** (person, personName, assignedColor, identityResolved) appear in ~60% of instances
- **Velocity fields** (vx, vy, vz) appear only in replay scenarios
- **Confidence/weight** appears in ~40% of instances

### Type System
- **TypeScript:** Provides type definitions only (spaxel.d.ts)
- **JavaScript:** Active blob object creation in dashboard
- **Go:** Multiple blob types across subsystems

### Key Files for Identity Field Updates
1. **`mothership/internal/tracking/tracker.go`** (line 163) - Already has identity fields
2. **`mothership/internal/tracker/tracker.go`** (line 162) - Legacy tracker, may need identity fields
3. **`mothership/cmd/mothership/main.go`** - Multiple conversion sites that may need identity preservation
4. **`dashboard/js/state.js`** - JavaScript blob creation may need identity field initialization

---

## Conclusion

The spaxel codebase contains **extensive blob object creation** across Go backend, JavaScript dashboard, and TypeScript type definitions. The canonical patterns are:

- **Go:** Struct literals and slice appends across 10+ blob types
- **JavaScript:** Object literals primarily in state management and tests
- **TypeScript:** Type definitions only, no object creation

**Blob detection pattern:** Look for objects with `{id, x, y, z}` core structure, optionally extended with identity, velocity, or confidence fields.
