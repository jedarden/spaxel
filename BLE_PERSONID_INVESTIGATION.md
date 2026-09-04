# BLE PersonID Population Investigation - bead bf-5f3f9

## Summary

**PersonID IS POPULATED** at EvaluateTriggers entry. The field is set through a multi-stage BLE identity matching pipeline that flows from the BLE device registry through blob matching to volume trigger evaluation.

## Complete Code Path

### 1. BLE Device Registry (Source of PersonID)

**File:** `mothership/internal/ble/registry.go`

The `DeviceRecord` struct contains the PersonID field:
```go
type DeviceRecord struct {
    // ... other fields ...
    PersonID     string     `json:"person_id"`    // FK to people.id
    PersonName   string     `json:"person_name"`  // Person name (joined from people)
    PersonColor  string     `json:"person_color"` // Person's color (joined from people)
    // ... more fields ...
}
```

PersonID is populated when a BLE device is assigned to a person via:
```go
func (r *Registry) AssignToPerson(mac, personID string) error {
    return r.UpdateDevice(mac, map[string]interface{}{"person_id": personID})
}
```

The PersonID comes from the `people` table (foreign key relationship).

### 2. BLE Identity Matching

**File:** `mothership/internal/ble/identity.go`

The `IdentityMatcher` triangulates BLE device positions and matches them to blobs:

```go
// In assignBlobsToDevices() function
match := &IdentityMatch{
    BlobID:            bestBlob.ID,
    DeviceAddr:        td.Device.Addr,
    DeviceName:        deviceName,
    PersonID:          td.Device.PersonID,        // ← From device registry
    PersonName:        td.Device.PersonName,      // ← From people table join
    PersonColor:       getPersonColor(td.Device),
    Confidence:        matchConf,
    // ... more fields ...
}
```

The matcher provides `GetMatch(blobID)` which returns the `IdentityMatch` for a given blob.

### 3. Identity Write-Back (Stage 2b of Fusion Loop)

**File:** `mothership/cmd/mothership/main.go` (lines 2211-2235)

This is the critical stage where PersonID flows from BLE matching to TrackedBlob:

```go
// Stage 2b: write resolved BLE identity back onto the served TrackedBlob slice
identityYes, identityNo := true, false
for i := range blobs {
    match := identityMatcher.GetMatch(blobs[i].ID)
    if match == nil {
        blobs[i].IdentityResolved = &identityNo
        continue
    }
    blobs[i].PersonID = match.PersonID          // ← PersonID populated here
    blobs[i].PersonLabel = match.PersonName
    blobs[i].PersonColor = match.PersonColor
    blobs[i].IdentityConfidence = match.Confidence
    blobs[i].IdentitySource = "ble"
    blobs[i].PersonName = match.PersonName
    blobs[i].AssignedColor = match.PersonColor
    blobs[i].IdentityResolved = &identityYes
}
pm.SetTrackedBlobs(blobs)  // Publish the identity-enriched blobs
```

**This happens BEFORE volume trigger evaluation.**

### 4. Volume Trigger Construction

**File:** `mothership/cmd/mothership/main.go` (lines 2472-2491)

When evaluating volume triggers, BlobPos is constructed from the identity-enriched TrackedBlob:

```go
volumeBlobs := make([]volume.BlobPos, len(blobs))
for i, blob := range blobs {
    volumeBlobs[i] = volume.BlobPos{
        ID: blob.ID,
        X:  blob.X,
        Y:  blob.Y,
        Z:  blob.Z,
        PersonID:         blob.PersonID,      // ← Populated from TrackedBlob
        PersonName:       blob.PersonName,    // ← Populated from TrackedBlob
        AssignedColor:    blob.AssignedColor, // ← Populated from TrackedBlob
        IdentityResolved: blob.IdentityResolved,
    }
}
```

### 5. PersonID Filtering in EvaluateTriggers

**File:** `mothership/internal/volume/shape.go` (lines 617-651)

The `evaluateEnter()` function filters by PersonID:

```go
func (s *Store) evaluateEnter(t *Trigger, state *TriggerState, blobs []BlobPos, now time.Time) bool {
    for _, blob := range blobs {
        // Check person filter - skip this blob if PersonID doesn't match
        if t.ConditionParams.PersonID != "" && t.ConditionParams.PersonID != "anyone" {
            if t.ConditionParams.PersonID != blob.PersonID {
                continue    // ← PersonID is used for filtering here
            }
        }
        
        // ... rest of enter detection logic ...
    }
}
```

## Data Flow Diagram

```
BLE Device Registry (SQLite)
    ↓
    PersonID assigned to device MAC
    ↓
BLE IdentityMatcher
    ↓ (triangulation + proximity matching)
    IdentityMatch{PersonID, PersonName, PersonColor}
    ↓
Stage 2b: Identity Write-Back (main.go:2226)
    ↓
    TrackedBlob.PersonID = match.PersonID
    ↓
Volume Trigger Construction (main.go:2484)
    ↓
    BlobPos{PersonID: blob.PersonID}
    ↓
EvaluateTriggers (shape.go:622-624)
    ↓
    if trigger.ConditionParams.PersonID != blob.PersonID { continue }
```

## Key Files and Functions

1. **BLE Registry:** `mothership/internal/ble/registry.go`
   - `DeviceRecord` struct (line 51)
   - `AssignToPerson()` (line 746)

2. **BLE Identity Matching:** `mothership/internal/ble/identity.go`
   - `IdentityMatch` struct (line 87)
   - `assignBlobsToDevices()` (line 487)

3. **Identity Write-Back:** `mothership/cmd/mothership/main.go`
   - Stage 2b (lines 2211-2235)
   - `blobs[i].PersonID = match.PersonID` (line 2226)

4. **Volume Trigger Construction:** `mothership/cmd/mothership/main.go`
   - Volume trigger evaluation (lines 2472-2491)
   - `PersonID: blob.PersonID` (line 2484)

5. **PersonID Filtering:** `mothership/internal/volume/shape.go`
   - `evaluateEnter()` (lines 617-651)
   - PersonID comparison (lines 622-624)

## Struct Fields Involved

**DeviceRecord** (BLE registry):
- `PersonID string` - Foreign key to people.id

**IdentityMatch** (BLE matcher, `mothership/internal/ble/identity.go:38`):
- `DeviceName string` `json:"device_name,omitempty"` (line 41) - BLE hardware name, e.g. "iPhone"
- `PersonID string` - From DeviceRecord
- `PersonName string` `json:"person_name,omitempty"` (line 43) - From people table join

Both identity name fields are plain `string` — never nil, empty when unknown —
and both carry `omitempty`, so an unidentified match drops them from the JSON
rather than sending `""`. Human-facing projections should read them through
`IdentityMatch.Label()` (identity.go:60), which prefers `PersonName` and falls
back to `DeviceName`. `api.IdentityMatch`
(`mothership/internal/api/status.go:45`) mirrors the pair with identical types
and tags — `PersonName string` `json:"person_name,omitempty"` (line 46),
`DeviceName string` `json:"device_name,omitempty"` (line 48) — as a deliberate
duplicate so `internal/api` need not import the `ble` type (status.go:44);
`getOccupancy` applies the same PersonName-then-DeviceName fallback at
status.go:191-200.

**TrackedBlob** (signal processor):
- `PersonID string` - Set by identity write-back
- `PersonName string` - Set by identity write-back
- `AssignedColor string` - Set by identity write-back

**BlobPos** (volume package):
- `PersonID string` - Set from TrackedBlob.PersonID
- `PersonName string` - Set from TrackedBlob.PersonName
- `AssignedColor string` - Set from TrackedBlob.AssignedColor

## Conclusion

PersonID **IS POPULATED** when EvaluateTriggers is called with a BlobPos. The field is populated through the BLE identity matching pipeline in Stage 2b of the fusion loop (main.go:2226) BEFORE volume triggers are evaluated. The PersonID flows from:

1. BLE device registry (person_id foreign key)
2. → BLE identity matcher (GetMatch returns PersonID)
3. → Identity write-back (TrackedBlob.PersonID = match.PersonID)
4. → BlobPos construction (PersonID: blob.PersonID)
5. → EvaluateTriggers filtering (t.ConditionParams.PersonID != blob.PersonID)

The TODO comment in evaluateEnter() can be safely removed or replaced with a reference to this documented code path.
