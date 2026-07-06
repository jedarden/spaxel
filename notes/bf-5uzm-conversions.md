# Cross-Package Blob Conversion Boundaries in main.go (bf-5uzm)

> **Scope:** The cross-package conversion boundaries in `mothership/cmd/mothership/main.go`
> where a tracked blob is converted from one type to another. These are the identity-leak
> sites — the actual goal of the umbrella effort (parent findings: `notes/bf-3ldj-findings.md`
> §5 Tier 1 + §6 data flow, `notes/bf-4bhd.md` Pattern 4).
>
> **All file:line references below were re-verified against HEAD `7846408` on 2026-07-06.**
> The four sites named in the task have **drifted** since the reference notes were written;
> the corrected (current) line numbers are authoritative and are recorded in §1.
>
> Paths: the reference notes use the module-relative path `cmd/mothership/main.go`. The
> repo-relative path (used throughout this doc) is `mothership/cmd/mothership/main.go` —
> same file.

---

## 0. TL;DR

| Task site (claimed) | Verified site (HEAD `7846408`) | Boundary | Target carries identity? | Verdict |
|---|---|---|---|---|
| `main.go:5384` (E1) | **`main.go:5494`** | `fusion.Blob` peak → `sigproc.TrackedBlob` | Target **has** 6 identity fields | Left zero — **by design** (peak source is pre-identity) |
| `main.go:2213` (E2) | **`main.go:2303`** | `sigproc.TrackedBlob` → `automation.TrackedBlob` | Target has **none** | ❌ **LEAK — identity dropped**, no compensation |
| `main.go:2116` (E3) | **`main.go:2206`** | `sigproc.TrackedBlob` → `explainability.BlobSnapshot` | Target has **none** | ⚠ **LEAK — dropped at the snapshot**, but a separate `identityMap` side-channel (`main.go:2216`) partially compensates |
| `main.go:2236` (volume) | **`main.go:2326`** | `sigproc.TrackedBlob` → `volume.BlobPos` | Target **has** `PersonID` | ⚠ **Dropped** — field exists but conversion leaves it `""` |

The two boundaries to fix for the umbrella goal are **E2 (automation, `:2303`)** and
**E3 (explainability, `:2206`)** — exactly as the reference notes flag, only the line
numbers have moved (+90 to +110 lines).

**One material correction to the reference notes** (see §3): the live 10 Hz loop does **not**
run `tracker.TrackManager`/`applyIdentity`. It runs a simple peak-associator (`blobTracker`,
`main.go:5433`) that emits `sigproc.TrackedBlob` with **identity fields permanently zero**.
Resolved identity lives in a sidecar — `ble.IdentityMatcher`, queried by `blob.ID` — and is
never written back onto the blob struct. So "identity dropped at the conversion" really means
"the conversion is the place identity *could* be attached from the sidecar, and isn't."

---

## 1. The four named conversion boundaries (verified against HEAD)

All four live in two places: **E1** is in `(*blobTracker).track` (`main.go:5447`); **E2, E3,
volume** are inline stages of the live 10 Hz fusion loop. For the latter three the loop source
is `blobs`, declared at `main.go:2057`:

```go
blobs := blobTracker.track(result)   // []sigproc.TrackedBlob  (main.go:2057)
```

So the **source type for E2 / E3 / volume is `sigproc.TrackedBlob`** (alias of
`signal.TrackedBlob`; import at `main.go:63`: `sigproc "github.com/spaxel/mothership/internal/signal"`).

### 1.1 E1 — `main.go:5494` — `fusion.Blob` peak → `sigproc.TrackedBlob`

- **Enclosing func:** `(*blobTracker).track` (`main.go:5447`).
- **Source type:** `fusion.Blob` — `{X, Y, Z, Confidence}` (`internal/fusion/fusion.go:36`).
  **No identity fields** (a fusion peak is pre-identity by construction).
- **Target type:** `sigproc.TrackedBlob` (`internal/signal/processor.go:587`) — **has** 6
  identity fields (`PersonID`, `PersonLabel`, `PersonColor`, `IdentityConfidence`,
  `IdentitySource`, `Posture`).
- **Literal (`main.go:5494`):**
  ```go
  b := sigproc.TrackedBlob{
      ID:     id,           // from greedy nearest-neighbour association, NOT from the peak
      X:      pk.X,
      Y:      pk.Y,
      Z:      pk.Z,
      Weight: pk.Confidence,
  }
  // then, if a previous position exists for this id (main.go:5501-5505):
  //   b.VX = (pk.X - pb.X) / dt ; b.VY = ... ; b.VZ = ...
  ```
- **Copied:** `ID` (tracker-assigned), `X/Y/Z`, `Weight` (← peak `Confidence`); `VX/VY/VZ`
  derived from the previous frame's position of the same ID.
- **Identity dropped:** all 6 identity fields left at Go zero values.
- **Why this is "by design", not a leak:** the source is a pre-identity peak, and `blobTracker`
  is a pure position-associator — it has no access to BLE matches. Identity is attached
  *later* (in the sidecar; see §3). Tier 3 in `bf-3ldj` §5.

### 1.2 E2 — `main.go:2303` — `sigproc.TrackedBlob` → `automation.TrackedBlob`  ❌ LEAK

- **Enclosing stage:** "Stage 6: Trigger evaluation" (`main.go:2298`), inside the
  `if automationEngine != nil {` block.
- **Source type:** `sigproc.TrackedBlob` (`b`, range var over `blobs`).
- **Target type:** `automation.TrackedBlob` (`internal/automation/engine.go:1337`):
  ```go
  type TrackedBlob struct {
      ID         int
      X, Y, Z    float64
      VX, VY, VZ float64
      Confidence float64
  }
  ```
  **The target type carries NO identity fields at all** (ID + pos + vel + Confidence only).
- **Literal (`main.go:2303`):**
  ```go
  autoBlobs[i] = automation.TrackedBlob{
      ID:         b.ID,
      X:          b.X,  Y: b.Y,  Z: b.Z,
      VX:         b.VX, VY: b.VY, VZ: b.VZ,
      Confidence: b.Weight,
  }
  ```
- **Copied:** `ID`, `X/Y/Z`, `VX/VY/VZ`, `Confidence` (← `b.Weight`).
- **Identity dropped:** all of it — and the target struct has nowhere to put it even if the
  source had it.
- **No side-channel compensation:** `automationEngine.Evaluate(autoBlobs, zoneForBlob)`
  (`main.go:2314`) is passed only a `zoneForBlob` resolver (returns zone name), **not** a
  person resolver. The engine does have a `personProvider` (wired at `main.go:3147`,
  `automationEngine.SetPersonProvider(&automationPersonAdapter{registry: bleRegistry})`),
  but it resolves `PersonName`/`PersonColor` **given a `PersonID`** (`engine.go:952`) — and
  no path supplies a `PersonID` for a blob, because `automation.TrackedBlob` has no such field
  and `Evaluate` receives only position. **Person-aware automations ("when Alice enters…") are
  therefore structurally blocked at this boundary.** This is the highest-value fix.

### 1.3 E3 — `main.go:2206` — `sigproc.TrackedBlob` → `explainability.BlobSnapshot`  ⚠ LEAK (partly compensated)

- **Enclosing stage:** "Stage 2" explainability block (`main.go:2160`,
  `if explainabilityHandler != nil {`).
- **Source type:** `sigproc.TrackedBlob` (`blob`, range var over `blobs`).
- **Target type:** `explainability.BlobSnapshot` (`internal/explainability/handler.go:95`):
  ```go
  type BlobSnapshot struct {
      ID         int
      X, Y, Z    float64
      Confidence float64
      Weight     float64 // Peak height in the grid
  }
  ```
  **The target type carries NO identity fields.**
- **Literal (`main.go:2206`):**
  ```go
  blobSnapshots = append(blobSnapshots, explainability.BlobSnapshot{
      ID:         blob.ID,
      X:          blob.X,  Y: blob.Y,  Z: blob.Z,
      Confidence: blob.Weight,
  })
  ```
- **Copied:** `ID`, `X/Y/Z`, `Confidence` (← `blob.Weight`). (`BlobSnapshot.Weight` — peak
  height — is left `0`; no grid snapshot is available in this path, see `main.go:2235`.)
- **Identity dropped:** person/color/source — none carried by the snapshot.
- **Side-channel compensation (important):** immediately after the snapshot loop, a separate
  `identityMap` is built from the sidecar and passed alongside the snapshots
  (`main.go:2215-2236`):
  ```go
  identityMap := make(map[int]*explainability.BLEMatch)
  for _, blob := range blobs {
      match := identityMatcher.GetMatch(blob.ID)
      if match != nil && match.PersonID != "" { ... identityMap[blob.ID] = &explainability.BLEMatch{...} }
  }
  explainabilityHandler.UpdateBlobs(blobSnapshots, linkStates, gridSnapshot, identityMap)
  ```
  So the "Why?" overlay **does** receive identity — via the `identityMap` argument keyed by
  `blob.ID`, not via the `BlobSnapshot`. The snapshot struct itself remains identity-free; if
  `BlobSnapshot` is ever consumed in isolation (without `identityMap`), the person name is
  lost. The clean fix is to put identity on `BlobSnapshot` and drop the parallel map.

### 1.4 volume — `main.go:2326` — `sigproc.TrackedBlob` → `volume.BlobPos`  ⚠ Dropped

- **Enclosing stage:** "Stage 6: Trigger evaluation" (`main.go:2322`),
  `if volumeTriggersHandler != nil {`.
- **Source type:** `sigproc.TrackedBlob` (`blob`, range var over `blobs`).
- **Target type:** `volume.BlobPos` (`internal/volume/shape.go:1080`):
  ```go
  type BlobPos struct {
      ID       int
      PersonID string   // ← identity field EXISTS on the target
      X, Y, Z  float64
  }
  ```
  **The target type DOES carry an identity field (`PersonID`).**
- **Literal (`main.go:2326`):**
  ```go
  volumeBlobs[i] = volume.BlobPos{
      ID: blob.ID,
      X:  blob.X,  Y: blob.Y,  Z: blob.Z,
  }
  ```
- **Copied:** `ID`, `X/Y/Z` only.
- **Identity dropped:** `PersonID` — the field is **present** on `volume.BlobPos` but the
  conversion **does not populate it** (left `""`). Velocity is also not copied (not carried by
  `BlobPos`). Because the field already exists, this is the cheapest boundary to repair: a
  single `PersonID: <from sidecar>` line would unblock person-filtered volume triggers
  (`condition_params.person`).

---

## 2. Complete enumeration of blob-conversion loops in main.go

Acceptance criterion: *"Every main.go blob-conversion loop enumerated with source type,
target type, copied fields."* Beyond the four named boundaries, the live loop projects the
tracked blob into several other shapes. Full list (HEAD `7846408`):

| # | main.go:line | Source | Target type | Fields copied | Identity? |
|---|---|---|---|---|---|
| 1 | `:2087` | `sigproc.TrackedBlob` (`blob`) | `map[string]interface{}` (detection-event JSON detail) | `x,y,z,vx,vy,vz,confidence,posture` | person passed **separately** as `personID` arg to `eventsHandler.LogEvent` (`:2100`) — not in the map |
| 2 | `:2113` | `sigproc.TrackedBlob` (`b`) | anon struct `{ID,X,Y,Z,Weight}` → `identityMatcher.UpdateBlobs` | `ID,X,Y,Z,Weight` | n/a — matcher is the identity **producer**, not consumer; velocity dropped |
| 3 | **`:2206` (E3)** | `sigproc.TrackedBlob` | `explainability.BlobSnapshot` | `ID,X,Y,Z,Confidence` | ⚠ dropped (compensated via `identityMap` side-channel, `:2216`) |
| 4 | `:2271` | `sigproc.TrackedBlob` + sidecar | `analytics.TrackUpdate` | `ID,X,Y,Z,VX,VY,VZ,PersonID` | ✅ **PersonID populated from sidecar** (`identityMatcher.GetMatch`, `:2267`) — the one consumer that does it right |
| 5 | `:2288` | `sigproc.TrackedBlob` (`blob`) | anon struct `{ID,X,Y,Z,VX,VY,VZ,Posture}` → `fallDetector.Update` | `ID,X,Y,Z,VX,VY,VZ` | `Posture` field **exists on the anon struct but is not set** (`:2293`); no identity |
| 6 | **`:2303` (E2)** | `sigproc.TrackedBlob` | `automation.TrackedBlob` | `ID,X/Y/Z,VX/VY/VZ,Confidence` | ❌ dropped; target has no identity field |
| 7 | **`:2326` (volume)** | `sigproc.TrackedBlob` | `volume.BlobPos` | `ID,X,Y,Z` | ⚠ `PersonID` field exists on target, left `""` |
| 8 | `:2153` | `sigproc.TrackedBlob` (`blob`) | `localization.Vec3` (ground-truth collector position) | `X,Y,Z` | n/a — pure position vector, no `ID` |
| 9 | **`:5494` (E1)** | `fusion.Blob` peak (`pk`) | `sigproc.TrackedBlob` | `ID(assoc),X,Y,Z,Weight(+derived VX/VY/VZ)` | by-design zero (pre-identity peak) |

Notes on the table:
- **Rows 3, 6, 7, 9** are the four boundaries named in the task.
- **Row 4 (`analytics.TrackUpdate`)** is the reference implementation for how identity *should*
  be threaded: pull `PersonID` from `identityMatcher.GetMatch(blob.ID)` at the conversion site.
  The E2/volume fixes should mirror this pattern.
- **Row 5 (fall-detect anon struct)** has its own latent bug — the `Posture` field is declared
  but never set — tracked separately under falldetect (see `bf-3ldj` §3 [G1]
  `falldetect/detector.go:277` for the in-package `BlobSnapshot`).

---

## 3. Material correction to the reference data-flow diagram

`bf-3ldj-findings.md` §6 draws the live path as:

```
fusion.Blob peak → sigproc.TrackedBlob [E1] → tracker.Blob [A1 tracker.go:162]
   → TrackManager.UpdateWithIdentity [A3] ✅ IDENTITY ATTACHED → GetAllBlobs [B1/B2] → consumers
```

**That is not the live path in HEAD `7846408`.** Verified facts:

1. **`tracker.Tracker` / `tracker.TrackManager` is not wired into `main.go` at all** — zero
   references in `cmd/mothership/main.go` (non-test). The `applyIdentity` / `clearIdentity`
   identity machinery in `internal/tracker/identity.go:164-185` therefore never runs in the
   live loop. (grep for `tracker\.\|TrackManager` in main.go returns nothing.)
2. The live loop uses `blobTracker` (`main.go:5433`, a hand-rolled greedy nearest-neighbour
   associator) whose `track()` returns `[]sigproc.TrackedBlob` (`main.go:5447`).
3. **`sigproc.TrackedBlob`'s 6 identity fields are never written in any live path** — grep
   across `internal/` and `cmd/` (non-test) for `\.PersonLabel =` / `\.PersonID =` /
   `\.PersonColor =` / `\.IdentityConfidence =` / `\.IdentitySource =` finds **zero** writes
   to a `sigproc.TrackedBlob`. The only identity-field writes in the tree are on
   `tracker.Blob` inside `internal/tracker/identity.go` (the unused-in-live-path 3D tracker).
4. **Resolved identity lives in a sidecar:** `ble.IdentityMatcher` (`main.go:799`), queried by
   `blob.ID` via `identityMatcher.GetMatch(blob.ID)` (e.g. `main.go:2077, 2138, 2218, 2267,
   2381`). It is never written back onto the blob struct.

**Corrected live data flow:**

```
fusion.Engine.Fuse ──fusion.Blob peaks──▶  (NO identity)
   [fusion.go:260]                            │
                                              ▼
blobTracker.track ──sigproc.TrackedBlob──▶  (6 identity fields exist, ALL ZERO)
   [E1 main.go:5494]                          │  ← identity never written onto the struct
                                              ▼
pm.SetTrackedBlobs(blobs) ──blobs: []sigproc.TrackedBlob──▶  every consumer reads ZERO identity
   [main.go:2057-2058]                        │
                                              │   resolved identity held SEPARATELY in:
                                              │   ble.IdentityMatcher  (main.go:799)
                                              │   queried per-blob by ID via GetMatch()
                                              ▼
   ┌──────────────┬───────────────┬───────────────────┬────────────────┬──────────────────┐
   ▼              ▼               ▼                   ▼                ▼                  ▼
 analytics      explainability   automation          volume           identityMatcher    fall-detect
 [:2271]        [:2206 E3]       [:2303 E2]          [:2326]          [:2113]            [:2288]
 ✅ pulls       ⚠ snapshot       ❌ target has       ⚠ PersonID       (sidecar: the       (anon struct;
   PersonID        drops person     NO identity         field exists     identity SOURCE)   Posture field
   from sidecar    BUT identity      field; no          but left ""      — not a leak,      also not set)
   → correct       Map side-         side-channel        — cheap to       this is where
                  channel (         compensation        fix              identity is made)
                  :2216) partly
                  compensates
```

**Implication for the fix:** because identity is in the sidecar (not on the blob), every
identity-aware conversion boundary must call `identityMatcher.GetMatch(blob.ID)` at the
conversion site — exactly as `analytics.TrackUpdate` already does (`main.go:2267-2279`).
Merely adding fields to the target structs is insufficient; the population step is the actual
fix. The two flagged boundaries:

- **E2 (`:2303`, automation):** add identity fields to `automation.TrackedBlob`
  (`engine.go:1337`) **and** populate them from the sidecar in the loop. Without this,
  `automationEngine.personProvider` can never fire (it needs a `PersonID` nothing supplies).
- **E3 (`:2206`, explainability):** add identity fields to `explainability.BlobSnapshot`
  (`handler.go:95`), populate from the sidecar, and fold the parallel `identityMap`
  (`main.go:2216`) into the snapshot so the "Why?" overlay has one identity source.
- **volume (`:2326`):** the `PersonID` field already exists — just populate it from the
  sidecar (one line) to enable person-filtered volume triggers.

---

## 4. Per-boundary: does the target type carry identity fields?

Acceptance criterion explicit checklist:

| Boundary | main.go (HEAD) | Target type | Target carries identity fields? |
|---|---|---|---|
| E1 | `:5494` | `sigproc.TrackedBlob` (`signal/processor.go:587`) | **Yes** — 6 fields (`PersonID`, `PersonLabel`, `PersonColor`, `IdentityConfidence`, `IdentitySource`, `Posture`); left zero (pre-identity peak source) |
| E2 | `:2303` | `automation.TrackedBlob` (`automation/engine.go:1337`) | **No** — `ID, X/Y/Z, VX/VY/VZ, Confidence` only |
| E3 | `:2206` | `explainability.BlobSnapshot` (`explainability/handler.go:95`) | **No** — `ID, X/Y/Z, Confidence, Weight` only |
| volume | `:2326` | `volume.BlobPos` (`volume/shape.go:1080`) | **Yes** — `PersonID` (single field); present but unpopulated |

---

## 5. Line-number reconciliation (task claim vs HEAD)

All four task-claimed lines have drifted; the code at the old lines is now unrelated
non-conversion code. Re-locate any future drift with:
`grep -n "sigproc.TrackedBlob{\|automation.TrackedBlob{\|explainability.BlobSnapshot{\|volume.BlobPos{" mothership/cmd/mothership/main.go`

| Boundary | Task claim | Verified (HEAD `7846408`, 2026-07-06) | Delta |
|---|---|---|---|
| E1 | `main.go:5384` | **`main.go:5494`** | +110 |
| E2 | `main.go:2213` | **`main.go:2303`** | +90 |
| E3 | `main.go:2116` | **`main.go:2206`** | +90 |
| volume | `main.go:2236` | **`main.go:2326`** | +90 |

Reference-note line numbers that are still accurate (verified): `fusion.Blob`
`fusion/fusion.go:36`; `sigproc.TrackedBlob` `signal/processor.go:587`;
`automation.TrackedBlob` `automation/engine.go:1337`; `explainability.BlobSnapshot`
`explainability/handler.go:95`; `volume.BlobPos` `volume/shape.go:1080`.

---

## 6. Acceptance criteria status

- [x] **Every main.go blob-conversion loop enumerated with source type, target type, copied
      fields** — §2 table (9 loops; the 4 named boundaries + 5 additional projections).
- [x] **The two identity-dropping boundaries (`:2303` automation, `:2206` explainability)
      explicitly flagged as the leak to fix** — §0, §1.2, §1.3, §3. (Line numbers updated
      from the task's `:2213`/`:2116` to the verified `:2303`/`:2206`.)
- [x] **Whether the target type carries identity fields is noted for each boundary** — §4
      checklist (E1 yes/zero, E2 none, E3 none, volume yes/unpopulated).
- [x] **All line numbers verified against current HEAD** — §1, §5 (HEAD `7846408`,
      2026-07-06).
- [x] **Findings written to `notes/bf-<id>-conversions.md`** — this file
      (`notes/bf-5uzm-conversions.md`).
