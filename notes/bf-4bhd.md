# Blob Creation Code Paths - Analysis Report

## Task: Find all blob creation code paths

### Summary

This report documents all locations where blob objects are created or constructed in the spaxel codebase. There are **FIVE distinct primary Blob types** across different packages (the tracked-blob lifecycle), plus **seven additional Blob-shaped projection types** that are derived from blobs for specific subsystems (see "Related Blob-shaped projection types" below).

1. **`tracking.Blob`** - 2D floor tracker (X, Z coordinates)
2. **`tracker.Blob`** - 3D tracker with identity support (X, Y, Z coordinates + posture)
3. **`fusion.Blob`** - Simple fusion result (X, Y, Z + confidence)
4. **`automation.TrackedBlob`** - Automation engine blob representation
5. **`signal.TrackedBlob`** - Fusion-to-tracking pipeline blob (X, Y, Z + velocity + identity fields)

> **Line numbers verified against current `main` (2026-07-06).** Sites move by a few lines as the codebase evolves; re-locate with `grep -n "&Blob{\|Blob{\|TrackedBlob{" mothership/` before editing.

---

## Blob Type Definitions

### 1. `tracking.Blob` (mothership/internal/tracking/tracker.go:21)
```go
type Blob struct {
    ID       int
    X        float64 // metres, room X
    Z        float64 // metres, room Z
    VX       float64 // m/s
    VZ       float64 // m/s
    Weight   float64 // localisation confidence [0..1]
    LastSeen time.Time
    Trail    [][2]float64
    ukf      *UKF
    
    // Identity fields
    PersonID           string
    PersonLabel        string
    PersonColor        string
    PersonName         string
    AssignedColor      string
    IdentityResolved   bool
    IdentityConfidence float64
    IdentitySource     string
    IdentityLastSeen   time.Time
    Posture            Posture
}
```

### 2. `tracker.Blob` (mothership/internal/tracker/tracker.go:36)
```go
type Blob struct {
    ID         int
    X, Y, Z    float64 // world-space position, metres
    VX, VY, VZ float64 // velocity, m/s
    Weight     float64 // detection confidence [0..1]
    Posture    Posture
    LastSeen   time.Time
    Trail      [][3]float64
    
    // Identity fields
    PersonID           string
    PersonLabel        string
    PersonColor        string
    IdentityConfidence float64
    IdentitySource     string
    IdentityLastSeen   time.Time
    ukf *UKF
}
```

### 3. `fusion.Blob` (mothership/internal/fusion/fusion.go:36)
```go
type Blob struct {
    X, Y, Z    float64 // world-space position (metres)
    Confidence float64 // normalised [0..1]
}
```

### 4. `automation.TrackedBlob` (mothership/internal/automation/engine.go:1337)
```go
type TrackedBlob struct {
    ID         int
    X, Y, Z    float64
    VX, VY, VZ float64
    Confidence float64
}
```

### 5. `signal.TrackedBlob` (mothership/internal/signal/processor.go:587)

> **Type alias:** `api.TrackedBlob` at `mothership/internal/api/tracks.go:30` is a
> pure alias (`type TrackedBlob = signal.TrackedBlob`), not a new struct. Any field
> change to `signal.TrackedBlob` propagates to `api.TrackedBlob` automatically —
> no separate edit needed, but be aware the type surfaces under two names.
```go
type TrackedBlob struct {
    ID         int
    X, Y, Z    float64
    VX, VY, VZ float64
    Weight     float64
    
    // Identity fields
    PersonID           string
    PersonLabel        string
    PersonColor        string
    IdentityConfidence float64
    IdentitySource     string
    Posture            string
}
```

---

## Blob Creation Sites

### Pattern 1: Direct struct construction with `&Blob{}`

#### 1.1 `tracking/tracker.go:160` - New blob from measurement
```go
b := &Blob{
    ID:       t.nextID,
    X:        meas[0],
    Z:        meas[1],
    Weight:   meas[2],
    LastSeen: now,
    Trail:    [][2]float64{{meas[0], meas[1]}},
    ukf:      NewUKF(meas[0], meas[1]),
    // Initialize identity fields with default values
    PersonName:       "",
    AssignedColor:    "",
    IdentityResolved: false,
}
```
- **Context**: Tracker.Update() when unmatched measurements create new tracks
- **Pattern**: Pointer construction, initializes ID, position, weight, trail, UKF filter

#### 1.2 `tracker/tracker.go:162` - New 3D blob from measurement
```go
b := &Blob{
    ID: t.nextID,
    X:  m[0], Y: m[1], Z: m[2],
    Weight:   m[3],
    LastSeen: now,
    Trail:    [][3]float64{{m[0], m[1], m[2]}},
    Posture:  PostureUnknown,
    ukf:      NewUKF(m[0], m[1], m[2]),
}
```
- **Context**: Tracker.Update() when unmatched measurements create new 3D tracks
- **Pattern**: Similar to 1.1 but with Y coordinate and Posture field

---

### Pattern 2: Value-type construction with `Blob{}`

#### 2.1 `fusion/fusion.go:260` - Blob from peak detection
```go
blobs[i] = Blob{X: p[0], Y: p[1], Z: p[2], Confidence: p[3]}
```
- **Context**: Engine.Fuse() converting fusion grid peaks to blobs
- **Pattern**: Value-type construction from 4-element peak array [x, y, z, confidence]

---

### Pattern 3: Slice allocation and copying

#### 3.1 `tracking/tracker.go:199-203` - Return snapshot (deep copy)
```go
out := make([]Blob, len(t.blobs))
for i, b := range t.blobs {
    out[i] = *b
    trail := make([][2]float64, len(b.Trail))
    copy(trail, b.Trail)
    out[i].Trail = trail
}
```
- **Context**: Tracker.Update() returning immutable snapshot
- **Pattern**: Dereference pointer, deep-copy trail slice

#### 3.2 `tracker/tracker.go:193-199` - Return 3D snapshot (deep copy)
```go
out := make([]Blob, len(t.blobs))
for i, b := range t.blobs {
    out[i] = *b
    trail := make([][3]float64, len(b.Trail))
    copy(trail, b.Trail)
    out[i].Trail = trail
    out[i].ukf = nil
}
```
- **Context**: Tracker.Update() returning immutable snapshot
- **Pattern**: Similar to 3.1 but clears ukf field for caller safety

#### 3.3 `tracker/identity.go:201` (`GetAllBlobs`), copy at `205-206` - Get all blobs copy
```go
result := make([]Blob, len(tm.blobs))
copy(result, tm.blobs)
```
- **Context**: TrackManager.GetAllBlobs() returning all blobs
- **Pattern**: Slice copy operation

---

### Pattern 4: automation.TrackedBlob construction

#### 4.1 `cmd/mothership/main.go:2213` - Automation blob conversion
```go
autoBlobs[i] = automation.TrackedBlob{
    // fields populated
}
```
- **Context**: Converting blobs to automation engine format
- **Pattern**: Cross-package type conversion

---

### Pattern 5: signal.TrackedBlob construction

#### 5.1 `cmd/mothership/main.go:5384` - Fusion result to tracked blob
```go
b := sigproc.TrackedBlob{
    ID:     id,
    X:      pk.X,
    Y:      pk.Y,
    Z:      pk.Z,
    Weight: pk.Confidence,
}
```
- **Context**: blobTracker.track() converting fusion peaks to tracked blobs
- **Pattern**: Type conversion from fusion.Blob to signal.TrackedBlob

---

### Pattern 6: Test data construction

#### 6.1 `fusion/fusion_test.go:631` - Test fixture
```go
Blobs: []Blob{{X: 2, Y: 1, Z: 2, Confidence: 0.85}}
```
- **Context**: Test data creation
- **Pattern**: Inline slice literal with struct literals

#### 6.2 `fusion/fusion_test.go:680` - Test fixture
```go
Blobs: []Blob{{X: 2, Y: 1, Z: 2, Confidence: 0.80}}
```

#### 6.3 `fusion/fusion_test.go:700` - Test fixture
```go
result := &Result{Blobs: []Blob{{X: 1, Y: 1, Z: 1, Confidence: 0.5}}}
```

---

## Files That Update Blob Fields

### Identity assignment (tracker/identity.go)

> These functions operate on `tracker.Blob` (the 3D tracker), whose identity
> fields are `PersonID`, `PersonLabel`, `PersonColor`, `IdentityConfidence`,
> `IdentitySource`, `IdentityLastSeen`. The extra fields `PersonName`,
> `AssignedColor`, `IdentityResolved` belong only to `tracking.Blob` (2D).

#### applyIdentity() - Lines 164-176
```go
func (tm *TrackManager) applyIdentity(blob *Blob, info *IdentityInfo, now time.Time) {
    blob.PersonID = info.PersonID
    blob.PersonLabel = info.PersonLabel
    blob.PersonColor = info.PersonColor
    blob.IdentityConfidence = info.IdentityConfidence
    blob.IdentityLastSeen = now

    if info.IdentitySource != "" {
        blob.IdentitySource = info.IdentitySource
    } else {
        blob.IdentitySource = "ble_triangulation"
    }
}
```
- **Context**: Applying BLE identity match to blob
- **Pattern**: Field mutation on existing blob pointer

#### clearIdentity() - Lines 179-185
```go
func (tm *TrackManager) clearIdentity(blob *Blob) {
    blob.PersonID = ""
    blob.PersonLabel = ""
    blob.PersonColor = ""
    blob.IdentityConfidence = 0
    blob.IdentitySource = ""
}
```
- **Context**: Clearing identity when match expires
- **Pattern**: Resetting identity fields to defaults

---

## Related Blob-shaped projection types

Beyond the five primary Blob types above, the codebase defines seven additional
`Blob*`-named structs. These are **projections** — read-only views built *from* a
tracked blob for a specific subsystem, not new tracked entities. They do not own a
UKF or a stable session ID. They matter for this inventory because any change to
blob fields must be propagated to the projections that copy those fields.

| Type | File:line | Built from | Purpose |
|------|-----------|-----------|---------|
| `tracking.BlobEvent` | `tracking/tracker.go:52` | `tracking.Blob` | Lifecycle event emitted on blob appear/disappear/re-ident (security-mode persistence, timeline) |
| `falldetect.BlobSnapshot` | `falldetect/detector.go:69` | `tracker.Blob` | Z-trajectory input to the fall-detection state machine |
| `explainability.BlobSnapshot` | `explainability/handler.go:95` | `tracker.Blob` | Per-link contribution / confidence breakdown for the "Why?" overlay |
| `explainability.BlobExplanation` | `explainability/handler.go:27` | derived | Rendered explanation result (not a direct blob copy) |
| `volume.BlobState` | `volume/shape.go:139` | tracked blob pos/vel | Input to spatial trigger volume point-in-volume tests (automation) |
| `volume.BlobPos` | `volume/shape.go:1080` | blob pos | Lightweight position used inside the volume containment math |
| `api.BlobPos` | `api/triggers.go:624` | blob pos | REST/JSON-facing position used by trigger evaluation in the API layer |
| `replay.BlobUpdate` | `replay/types.go:303` | replay pipeline blob | Incremental blob frame pushed to the dashboard during time-travel replay |
| `simulator.BlobResult` | `simulator/engine.go:80` | synthetic walker | Synthetic blob emitted by `spaxel-sim` (test fixture, not production) |

**Construction pattern (all projections):** value-type struct literal
(`SomeBlobSnapshot{ID: b.ID, X: b.X, ...}`) at the boundary where a tracked blob
enters the subsystem.

**Enumerated projection construction sites** (non-test, verified 2026-07-06) — every
place a projection is *built*. A field added to a tracked blob that a projection copies
must be propagated at each of these sites:

| Site | Type built | Built from | Notes |
|------|-----------|-----------|-------|
| `cmd/mothership/main.go:2116` | `explainability.BlobSnapshot` | tracked blob | "Why?" overlay input; loop over all blobs |
| `cmd/mothership/main.go:2236` | `volume.BlobPos` | tracked blob | volume-trigger evaluation; loop over all blobs |
| `explainability/handler.go:194,255,357` | `BlobExplanation` | derived | rendered explanation result (3 return sites) |
| `falldetect/detector.go:277` | `BlobSnapshot` | tracked blob | Z-trajectory input to fall-detect state machine |
| `replay/pipeline.go:114,132` | `BlobUpdate` | replay blob | incremental frames pushed to dashboard during replay |
| `simulator/engine.go:460` | `BlobResult` | synthetic walker | emitted by `spaxel-sim` (test fixture, not production) |
| `tracking/tracker.go:173,188` | `BlobEvent` | tracked blob | appear/disappear lifecycle events |
| `volume/shape.go:375,575,820,879` | `BlobState` | tracked blob | per-blob state in volume trigger state machine (4 sites) |

Re-locate any moved site with:
`grep -rn "BlobSnapshot{\|BlobState{\|BlobPos{\|BlobUpdate{\|BlobEvent{\|BlobResult{\|BlobExplanation{" mothership/`

---

## Key Findings

### Files Requiring Updates for Blob Structure Changes

1. **`mothership/internal/tracking/tracker.go`** (line 160)
   - Constructor for new 2D blobs
   
2. **`mothership/internal/tracker/tracker.go`** (line 162)
   - Constructor for new 3D blobs with posture
   
3. **`mothership/internal/fusion/fusion.go`** (line 260)
   - Constructor for fusion result blobs
   
4. **`mothership/cmd/mothership/main.go`** (lines 2213, 5384)
   - Type conversion constructors for automation and signal packages
   
5. **`mothership/internal/tracker/identity.go`** (lines 164-185)
   - Identity field assignment (`applyIdentity`) and clearing (`clearIdentity`)

### Creation Patterns by Type

| Blob Type | Construction Method | Typical Context |
|-----------|-------------------|-----------------|
| `tracking.Blob` | `&Blob{}` | New track from measurement |
| `tracker.Blob` | `&Blob{}` | New 3D track from measurement |
| `fusion.Blob` | `Blob{}` | Peak detection result |
| `automation.TrackedBlob` | `TrackedBlob{}` | Automation trigger evaluation |
| `signal.TrackedBlob` | `TrackedBlob{}` | Fusion to tracking pipeline |

### Identity Fields Affected

When updating blob identity fields, these files need changes:
- **Constructors**: `tracking/tracker.go:160` (2D) and `tracker/tracker.go:162` (3D) — initialize new fields
- **Identity matching**: `tracker/identity.go:164-185` — `applyIdentity()` and `clearIdentity()` (operate on `tracker.Blob` only)
- **Type conversions**: `cmd/mothership/main.go:2213` (automation) and `:5384` (signal)
- **Field-set caveat**: `tracking.Blob` (2D) carries `PersonName`, `AssignedColor`, `IdentityResolved`; `tracker.Blob` (3D) does **not**. Adding an identity field to one type does not automatically add it to the other — both definitions must be edited deliberately.

---

## Recommendations

1. **For adding new identity fields**: Update all 5 creation sites + the 2 identity functions
2. **For changing blob structure**: Update the 5 type definitions, all constructors, and every projection type in the table above that copies the changed field
3. **For identity field logic**: Focus on `tracker/identity.go` (note: only the 3D tracker has identity functions today)
4. **For testing**: Update `fusion_test.go` fixtures when structure changes
5. **For projection types**: Run the grep in the projection-types section to find each boundary construction site before changing copied fields

---

## Acceptance Criteria Status

- [x] All blob creation sites are identified and listed
- [x] Each site is documented with file path and line number
- [x] Creation pattern is noted (constructor, literal, factory, etc.)
- [x] Report is ready for the next bead to use

**Totals:** 5 primary Blob types · 8 primary construction sites (3 direct struct literals + 3 slice/copy snapshots + 2 cross-package conversions) · 2 identity field-mutation functions · 9 additional Blob-shaped projection types · 15 projection construction sites enumerated across 7 files.
