# Blob Creation Code Paths - Analysis Report

## Task: Find all blob creation code paths

### Summary

This report documents all locations where blob objects are created or constructed in the spaxel codebase. There are **FOUR distinct Blob types** across different packages:

1. **`tracking.Blob`** - 2D floor tracker (X, Z coordinates)
2. **`tracker.Blob`** - 3D tracker with identity support (X, Y, Z coordinates + posture)
3. **`fusion.Blob`** - Simple fusion result (X, Y, Z + confidence)
4. **`automation.TrackedBlob`** - Automation engine blob representation

---

## Blob Type Definitions

### 1. `tracking.Blob` (mothership/internal/tracking/tracker.go)
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

### 2. `tracker.Blob` (mothership/internal/tracker/tracker.go)
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

### 3. `fusion.Blob` (mothership/internal/fusion/fusion.go)
```go
type Blob struct {
    X, Y, Z    float64 // world-space position (metres)
    Confidence float64 // normalised [0..1]
}
```

### 4. `automation.TrackedBlob` (mothership/internal/automation/engine.go)
```go
type TrackedBlob struct {
    ID         int
    X, Y, Z    float64
    VX, VY, VZ float64
    Confidence float64
}
```

### 5. `signal.TrackedBlob` (mothership/internal/signal/processor.go)
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

#### 1.1 `tracking/tracker.go:163` - New blob from measurement
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

#### 3.1 `tracking/tracker.go:206-208` - Return snapshot (deep copy)
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

#### 3.2 `tracker/tracker.go:193-195` - Return 3D snapshot (deep copy)
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

#### 3.3 `tracker/identity.go:214-216` - Get all blobs copy
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

#### applyIdentity() - Lines 167-182
```go
func (tm *TrackManager) applyIdentity(blob *Blob, info *IdentityInfo, now time.Time) {
    blob.PersonID = info.PersonID
    blob.PersonLabel = info.PersonLabel
    blob.PersonColor = info.PersonColor
    blob.PersonName = info.PersonName
    blob.AssignedColor = info.AssignedColor
    blob.IdentityResolved = info.IdentityResolved
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

#### clearIdentity() - Lines 185-194
```go
func (tm *TrackManager) clearIdentity(blob *Blob) {
    blob.PersonID = ""
    blob.PersonLabel = ""
    blob.PersonColor = ""
    blob.PersonName = ""
    blob.AssignedColor = ""
    blob.IdentityResolved = false
    blob.IdentityConfidence = 0
    blob.IdentitySource = ""
}
```
- **Context**: Clearing identity when match expires
- **Pattern**: Resetting identity fields to defaults

---

## Key Findings

### Files Requiring Updates for Blob Structure Changes

1. **`mothership/internal/tracking/tracker.go`** (line 163)
   - Constructor for new 2D blobs
   
2. **`mothership/internal/tracker/tracker.go`** (line 162)
   - Constructor for new 3D blobs with posture
   
3. **`mothership/internal/fusion/fusion.go`** (line 260)
   - Constructor for fusion result blobs
   
4. **`mothership/cmd/mothership/main.go`** (lines 2213, 5384)
   - Type conversion constructors for automation and signal packages
   
5. **`mothership/internal/tracker/identity.go`** (lines 167-194)
   - Identity field assignment and clearing

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
- **Constructors**: tracker.go (2 locations) - initialize new fields
- **Identity matching**: identity.go - applyIdentity() and clearIdentity()
- **Type conversions**: main.go - automation and signal TrackedBlob conversions

---

## Recommendations

1. **For adding new identity fields**: Update all 5 creation sites
2. **For changing blob structure**: Update type definitions and all constructors
3. **For identity field logic**: Focus on tracker/identity.go
4. **For testing**: Update fusion_test.go fixtures when structure changes

---

## Acceptance Criteria Status

- [x] All blob creation sites are identified and listed
- [x] Each site is documented with file path and line number
- [x] Creation pattern is noted (constructor, literal, factory, etc.)
- [x] Report is ready for the next bead to use

**Total blob creation sites found: 8 primary + 3 field mutation sites**
