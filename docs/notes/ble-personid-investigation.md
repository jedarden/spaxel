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

Complete extraction — all 11 fields, declaration order, with types and JSON
tags as declared at identity.go:38-50. `DeviceName` and `PersonName` were
documented in the previous pass; the other nine are the remainder.

| Field | Type | JSON tag | Line | Populated by / semantics |
|---|---|---|---|---|
| `BlobID` | `int` | `json:"blob_id"` | 39 | ID of the CSI blob the identity is attached to. `-1` for BLE-only placeholder tracks (identity.go:602), which are keyed by PersonID in `bleOnlyTracks` instead of by blob |
| `DeviceAddr` | `string` | `json:"device_addr"` | 40 | BLE address — the `DeviceRecord` registry key, copied from `td.Device.Addr` |
| `DeviceName` | `string` | `json:"device_name,omitempty"` | 41 | BLE hardware name, e.g. "iPhone"; advertised `Device.Name` falling back to the registry's `DeviceName` when the advertisement is empty (identity.go:482-485) |
| `PersonID` | `string` | `json:"person_id,omitempty"` | 42 | From `DeviceRecord` — foreign key to people.id |
| `PersonName` | `string` | `json:"person_name,omitempty"` | 43 | From people table join |
| `PersonColor` | `string` | `json:"person_color,omitempty"` | 44 | `getPersonColor(td.Device)` (identity.go:709): `defaultColorForPerson(PersonID)` — a hash of the PersonID into a fixed 8-colour palette (identity.go:720) — or gray `#6b7280` when there is no PersonID. It does **not** read a colour stored in the people table; the function's own comment concedes this |
| `Confidence` | `float64` | `json:"confidence"` | 45 | `computeMatchConfidence` (identity.go:521) = f_observations × f_node_count × f_residual × f_distance, gated at `MinMatchConfidence` 0.6; **halved** (`×0.5`) for BLE-only tracks (identity.go:608) |
| `TriangulationPos` | `Position` | `json:"triangulation_pos"` | 46 | RSSI-triangulated **device** position (`TriangulatedDevice.Position`) — a different point from the blob's position; the geometric divergence between the two is what bf-6d2ii is logging |
| `TriangulationConf` | `float64` | `json:"triangulation_confidence"` | 47 | `TriangulatedDevice.Confidence` — quality of the RSSI triangulation, independent of the match `Confidence` above |
| `Timestamp` | `time.Time` | `json:"timestamp"` | 48 | `time.Now()` at match creation; drives expiry in `decayOldMatches` against `matchTimeout` |
| `IsBLEOnly` | `bool` | `json:"is_ble_only"` | 49 | True when no CSI blob was within `MaxBLEBlobDistance` (2.0 m) — the match is a placeholder track with no spatial blob behind it |

Complete struct definition, verbatim (identity.go:37-50, including the doc comment):

```go
// IdentityMatch represents a match between a blob and a device/person.
type IdentityMatch struct {
	BlobID            int       `json:"blob_id"`
	DeviceAddr        string    `json:"device_addr"`
	DeviceName        string    `json:"device_name,omitempty"`
	PersonID          string    `json:"person_id,omitempty"`
	PersonName        string    `json:"person_name,omitempty"`
	PersonColor       string    `json:"person_color,omitempty"`
	Confidence        float64   `json:"confidence"`
	TriangulationPos  Position  `json:"triangulation_pos"`
	TriangulationConf float64   `json:"triangulation_confidence"`
	Timestamp         time.Time `json:"timestamp"`
	IsBLEOnly         bool      `json:"is_ble_only"` // True if no CSI blob within range
}
```

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

Wire notes on the remaining nine fields:

- The complete struct reaches JSON **verbatim** only at `GET /api/ble/matches`
  (main.go:3362-3368, served via `GetAllMatches`, identity.go:676). Everywhere
  else it is projected down first.
- That projection is four fields, not a mirror: `bleIdentityAdapter.GetMatch`
  (main.go:6009-6025) copies `PersonName`, `PersonID`, `DeviceName` and
  `IsBLEOnly` into `api.IdentityMatch` and drops `BlobID`, `DeviceAddr`,
  `PersonColor`, `Confidence`, `TriangulationPos`, `TriangulationConf` and
  `Timestamp`. `api.IdentityMatch` (status.go:45-50) is the dashboard-feed
  subset, not a field-for-field copy.
- `TriangulationPos`'s element type `Position` (identity.go:71-73) carries **no
  JSON tags** — `type Position struct { X, Y, Z float64 }` — so
  `encoding/json` emits the exported Go field names `"X"`, `"Y"`, `"Z"` inside
  `triangulation_pos`, not `"x"`/`"y"`/`"z"`. Consumers of
  `/api/ble/matches` must read the capitalised keys.
- `Timestamp` is `time.Time`, so it marshals as an RFC3339 string rather than
  the Unix-millisecond integer the node↔mothership protocol convention uses;
  that convention governs the WebSocket frames, not this dashboard-facing REST
  shape.
- Seven fields have no `omitempty` — `BlobID`, `DeviceAddr`, `Confidence`,
  `TriangulationPos`, `TriangulationConf`, `Timestamp` and `IsBLEOnly` are
  always emitted, including `blob_id: -1` on BLE-only tracks and
  `is_ble_only: false` on blob-attached ones. Only the four `string` identity
  fields disappear when empty.
- `ForceMatch` (identity.go:741, the manual-override path) sets only `BlobID`,
  `PersonID`, `PersonName`, `PersonColor`, `Confidence`, `Timestamp` and
  `IsBLEOnly`, leaving `DeviceAddr`, `DeviceName`, `TriangulationPos` and
  `TriangulationConf` at their zero values — so a forced match legitimately
  serves `device_addr: ""`, `triangulation_pos: {"X":0,"Y":0,"Z":0}` and
  `triangulation_confidence: 0`.

**TrackedBlob** (signal processor):
- `PersonID string` - Set by identity write-back
- `PersonName string` - Set by identity write-back
- `AssignedColor string` - Set by identity write-back

**BlobPos** (volume package):
- `PersonID string` - Set from TrackedBlob.PersonID
- `PersonName string` - Set from TrackedBlob.PersonName
- `AssignedColor string` - Set from TrackedBlob.AssignedColor

## IdentityMatch Analysis Summary

Consolidated from the three analysis passes this family produced — (1) struct
fields and types, (2) existing methods, (3) `Label()` test cases and contract —
each re-verified against HEAD with zero drift. Section 1 compresses the
detailed field table above; sections 2 and 3 are new to this document and are
the reference for anything touching `IdentityMatch` API surface.

### 1. Struct fields and types

`ble.IdentityMatch` (`mothership/internal/ble/identity.go:37-50`) — 11 fields,
declaration order:

| Field | Type | JSON tag |
|---|---|---|
| `BlobID` | `int` | `json:"blob_id"` |
| `DeviceAddr` | `string` | `json:"device_addr"` |
| `DeviceName` | `string` | `json:"device_name,omitempty"` |
| `PersonID` | `string` | `json:"person_id,omitempty"` |
| `PersonName` | `string` | `json:"person_name,omitempty"` |
| `PersonColor` | `string` | `json:"person_color,omitempty"` |
| `Confidence` | `float64` | `json:"confidence"` |
| `TriangulationPos` | `Position` | `json:"triangulation_pos"` |
| `TriangulationConf` | `float64` | `json:"triangulation_confidence"` |
| `Timestamp` | `time.Time` | `json:"timestamp"` |
| `IsBLEOnly` | `bool` | `json:"is_ble_only"` |

The four `string` identity fields carry `omitempty` and drop out of the JSON
when empty; the other seven always serialize (per-field population semantics:
see the detailed table above). Two companion facts that recur across this
family: `api.IdentityMatch` (`mothership/internal/api/status.go:45-50`) is a
deliberate **four-field projection** (`PersonName`, `PersonID`, `DeviceName`,
`IsBLEOnly`) so `internal/api` need not import the `ble` type — not a mirror;
and `Position` (identity.go:71-73) carries no JSON tags, so
`triangulation_pos` emits capitalised `"X"`/`"Y"`/`"Z"` keys.

### 2. Existing methods

`mothership/internal/ble/identity.go` defines **two distinct types**, the
likely source of this family's repeated confusion: `IdentityMatch` (line 38,
the per-person data record) and `IdentityMatcher` (line 86, the matcher
engine). The file holds 29 `func` declarations in total:

- **Methods with an `IdentityMatch` receiver: exactly one.**
  `func (m *IdentityMatch) Label() string` (identity.go:60) — nil-safe,
  returns `PersonName` if set else `DeviceName`. A repo-wide grep finds no
  other `IdentityMatch` method anywhere in `mothership/`.
- **Methods on the different type `*IdentityMatcher` (21)** — if a task says
  "add/find an IdentityMatch method", check which type it actually means:
  `SetRotationDetector` (104), `UpdateBlobs` (126), `getTriangulatedDevices`
  (162), `triangulateAllDevices` (175), `triangulate` (258), `assignBLEToBlobs`
  (399), `createBLEOnlyTracks` (556), `decayOldMatches` (621), `GetMatches`
  (645), `GetMatch` (657), `GetBLEOnlyTracks` (664), `GetAllMatches` (676),
  `GetPersistentIdentity` (691), `ForceMatch` (741), `ClearMatch` (760),
  `ProcessBLEObservations` (770), `GetRotationCandidates` (779),
  `GetRotationHistory` (788), `ExtendGracePeriod` (798),
  `IsWithinGracePeriod` (807), `EnrichBlobsWithIdentity` (819).
- **Package-level funcs in the same file, not methods (7):** `bleDebug` (13),
  `NewIdentityMatcher` (111), `rssiToDistance` (394), `computeMatchConfidence`
  (521), `sortDevicesByConfidence` (698), `getPersonColor` (709),
  `defaultColorForPerson` (720).

### 3. Test cases and the `Label()` contract

`mothership/internal/ble/label_test.go:9-43` — one table-driven test,
`TestIdentityMatch_Label`, four cases:

| Case | Receiver | Want |
|---|---|---|
| person name preferred over device name | `&IdentityMatch{PersonName: "Alice", DeviceName: "iPhone"}` | `"Alice"` |
| device name fallback when person name empty | `&IdentityMatch{DeviceName: "Dog Tracker"}` | `"Dog Tracker"` |
| empty when both names empty | `&IdentityMatch{}` | `""` |
| nil receiver returns empty without panic | `nil` | `""` |

Contract (label_test.go:5-8 doc comment, matching the implementation at
identity.go:59-70):

- Returns the canonical human-facing identity for an `IdentityMatch`.
- Precedence: `PersonName` if non-empty, else `DeviceName`. Only `!= ""` is
  checked — no trimming, no coalescing of whitespace-only strings.
- Both empty → `""`.
- Nil-safe: an explicit `if m == nil { return "" }` guard means a nil
  `*IdentityMatch` yields `""` without panicking.
- Single `string` return; no error, no ok-flag.
- Purpose (implementation doc comment): the accessor every human-facing
  projection (falldetect, zone-crossing, anomaly/notify) reads identity
  through at runtime, because projections previously read `DeviceName`
  directly — the BLE hardware name, frequently empty for registered persons.

Verified live (Go 1.25.0, from `mothership/`):
`go test ./internal/ble/ -run TestIdentityMatch_Label -v` → all 4 subtests PASS.

## Conclusion

PersonID **IS POPULATED** when EvaluateTriggers is called with a BlobPos. The field is populated through the BLE identity matching pipeline in Stage 2b of the fusion loop (main.go:2226) BEFORE volume triggers are evaluated. The PersonID flows from:

1. BLE device registry (person_id foreign key)
2. → BLE identity matcher (GetMatch returns PersonID)
3. → Identity write-back (TrackedBlob.PersonID = match.PersonID)
4. → BlobPos construction (PersonID: blob.PersonID)
5. → EvaluateTriggers filtering (t.ConditionParams.PersonID != blob.PersonID)

The TODO comment in evaluateEnter() can be safely removed or replaced with a reference to this documented code path.
