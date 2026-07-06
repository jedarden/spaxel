# Blob Object Literal Search Pattern

## Definition

A **blob-shaped object literal** is a data structure representing a detected person/presence in 3D space with positional and confidence information. Blobs are the core tracking entity in the Spaxel system, representing real-time detected people from fusion engine results.

## Key Characteristics

### Core Fields (Required)
- **id** - Unique numeric identifier for the blob
- **x, y, z** - 3D world-space position (metres)
- **confidence/weight** - Detection confidence score [0-1]

### Extended Fields (Optional)
- **vx, vy, vz** - Velocity components (m/s)
- **posture** - Body posture state (standing/walking/seated/lying)
- **person_id/person** - Associated person identity
- **person_label/personName** - Display name for identified person
- **person_color/assignedColor** - Hex color for dashboard rendering
- **trails** - Historical position trail array
- **identity_resolved** - Boolean flag for identity confirmation
- **ble_device** - Associated BLE device reference

## Language-Specific Patterns

### JavaScript (TypeScript) Pattern

```javascript
// Minimal blob structure
{ id: number, x: number, y: number, z: number, confidence?: number }

// Extended blob (dashboard state)
{ id: number, x: number, y: number, z: number, confidence: number, 
  vx?: number, vy?: number, vz?: number, posture?: string, 
  person?: string, ble_device?: string, trails?: Array, 
  personName?: string, assignedColor?: string, identityResolved?: boolean }

// Example from quick-actions.test.js
const blob = { id: 123, x: 2, y: 0, z: 3, personName: undefined, assignedColor: undefined, identityResolved: undefined };
```

### Go Pattern

```go
// Minimal fusion.Blob struct literal
fusion.Blob{X: float64, Y: float64, Z: float64, Confidence: float64}

// Extended tracker.Blob struct literal
tracker.Blob{
    ID: int,
    X: float64, Y: float64, Z: float64,
    VX: float64, VY: float64, VZ: float64,
    Weight: float64,
    Posture: Posture,
    // ... identity fields
}

// TrackedBlob struct literal (automation package)
automation.TrackedBlob{
    ID: int, X: float64, Y: float64, Z: float64,
    VX: float64, VY: float64, VZ: float64,
    Confidence: float64,
}
```

## Grep/Ripgrep Patterns

### JavaScript/TypeScript Files

```bash
# Find minimal blob literals (id + position fields)
rg '\{[^}]*id:\s*\d+[^}]*x:\s*[\d.]+[^}]*y:\s*[\d.]+[^}]*z:\s*[\d.]+[^}]*\}' --type js

# Find blob objects with id, x, y, z fields (any order)
rg '(?i)\{[^}]*\bid:\s*\w+[^}]*\bx:\s*[\d.]+[^}]*\by:\s*[\d.]+[^}]*\bz:\s*[\d.]+[^}]*\}' --type js

# Find blob objects with person/identity fields
rg '\{[^}]*\bid:\s*\w+[^}]*\bperson(Name|Label)?:' --type js

# Find blob state assignments
rg 'blobs\[.*\]\s*=\s*\{' --type js
```

### Go Files

```bash
# Find fusion.Blob struct literals
rg 'fusion\.Blob\{[^}]*X:\s*[\d.]+[^}]*Y:\s*[\d.]+[^}]*Z:\s*[\d.]+[^}]*\}' --type go

# Find tracker.Blob struct literals
rg 'tracker\.Blob\{' --type go

# Find TrackedBlob struct literals
rg '(TrackedBlob|automation\.TrackedBlob)\{' --type go

# Find blob result patterns (simulator)
rg 'BlobResult\{' --type go
```

### Cross-Language Pattern

```bash
# Find any object with id + x + y + z fields (both JS and Go)
rg '(?i)\{[^}]*\bid:\s*\w+[^}]*\bx:\s*[\d.]+[^}]*\by:\s*[\d.]+[^}]*\bz:\s*[\d.]+[^}]*\}' 

# Find blob creation in tests
rg 'blob.*=.*\{.*id:.*x:.*y:.*z:' -i
```

## Example Matches

### JavaScript Examples

```javascript
// Quick actions test (minimal)
const blob = { id: 123, x: 2, y: 0, z: 3 };

// State initialization (extended)
appState.blobs[id] = {
    id: id,
    personName: undefined,
    assignedColor: undefined,
    identityResolved: undefined
};

// With person data
const blob = { id: 123, person: 'Alice', x: 2, y: 0, z: 3 };
```

### Go Examples

```go
// Fusion blob (minimal)
Blob{X: 2, Y: 1, Z: 2, Confidence: 0.85}

// Tracker blob creation
b := &Blob{
    ID:       t.nextID,
    X:        meas[0],
    Z:        meas[1],
    Weight:   meas[2],
    LastSeen: now,
}

// Automation TrackedBlob conversion
autoBlobs[i] = automation.TrackedBlob{
    ID:         b.ID,
    X:          b.X,
    Y:          b.Y,
    Z:          b.Z,
    VX:         b.VX,
    VY:         b.VY,
    VZ:         b.VZ,
    Confidence: b.Weight,
}
```

## Search Strategy Recommendations

1. **Start with the minimal pattern** (id + x + y + z) to catch all blob-like objects
2. **Filter by context** to distinguish blobs from similar 3D position objects (nodes, zones, etc.)
3. **Use language-specific patterns** when focusing on specific code areas
4. **Check surrounding code** for blob-specific operations (updateBlob, getBlob, blob state management)

## Related Structures (Not Blobs)

- **Node objects**: Use `mac` instead of `id` for identification
- **Zone objects**: Include dimensions (`w, d, h`) and `zone_type`
- **Link objects**: Include `node_mac`, `peer_mac`, `delta_rms`
- **Events**: Include `timestamp_ms`, `type`, `severity`

## Usage Notes

- Blob objects are created by the fusion engine and consumed by the tracker
- JavaScript blobs are state objects in the dashboard's central state management
- Go blobs are struct literals passed between fusion, tracking, and automation packages
- Search patterns should account for both object literal creation and state assignment patterns
