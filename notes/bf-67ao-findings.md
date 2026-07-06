> ⚠️ **Secondary — folded into the consolidated inventory.** This was an input to
> `notes/bf-3ldj-findings.md`, which is itself **superseded** (see its banner). The
> authoritative blob inventory is **`notes/bf-1q3m-consolidated.md`** — consume that file for
> `file:line` references. Retained for provenance only.

---

# Blob Factory Functions — Analysis Report (secondary — see banner above)

## Task: bf-67ao — Find all functions/methods that create and return blob objects

### Scope & Definition

A **blob factory** is a function or method whose job is to **construct and return** a blob
object — either a newly-spawned tracked blob, a snapshot/copy handed to a caller, a
cross-package adapter, or a synthetic/explainability blob. This is a narrower lens than the
earlier `notes/bf-4bhd.md` survey (which catalogued every struct literal site, including
slice copies and test fixtures). Here we list only the **functions** and document each one's
**signature** plus the **blob-creation pattern** it uses.

There are **no `NewBlob`/`newBlob`/`makeBlob` constructor helpers** in the codebase — blobs
are constructed inline via struct literals inside the factory functions below. The
"factory" is therefore the enclosing method that returns the constructed blob(s).

### Blob types produced by factories

| Type | Package | File:line (type def) |
|------|---------|----------------------|
| `tracker.Blob` | 3D tracker (X,Y,Z + posture + identity) | `mothership/internal/tracker/tracker.go:36` |
| `tracking.Blob` | 2D floor tracker (X,Z) | `mothership/internal/tracking/tracker.go:21` |
| `fusion.Blob` | fusion peak (X,Y,Z + confidence) | `mothership/internal/fusion/fusion.go:36` |
| `signal.TrackedBlob` | signal-pipeline blob | `mothership/internal/signal/processor.go:587` |
| `automation.TrackedBlob` | automation-engine blob | `mothership/internal/automation/engine.go:1337` |
| `replay.BlobUpdate` | replay/demo blob | `mothership/internal/replay/types.go:303` |
| `simulator.BlobResult` | simulator output blob | `mothership/internal/simulator/engine.go:80` |
| `explainability.BlobExplanation` | explainability record | `mothership/internal/explainability/handler.go:27` |
| `explainability.BlobSnapshot` | explainability input snapshot | `mothership/internal/explainability/handler.go:95` |
| `falldetect.BlobSnapshot` | fall-detect history record | `mothership/internal/falldetect/detector.go:69` |

> Note: `explainability.BlobSnapshot` and `falldetect.BlobSnapshot` are **distinct
> types** in different packages, both named `BlobSnapshot`. The former is an input fed to
> `computeExplanation` (F1); the latter is a history record appended inside `processBlob`
> (G1).

---

## A. Primary tracker factories (spawn new persistent tracks)

These create the live, UKF-backed blob tracks that carry identity through the pipeline.

### A1. `(*tracker.Tracker).Update` — 3D tracker
- **Signature:** `func (t *Tracker) Update(measurements [][4]float64) []Blob`
  (`mothership/internal/tracker/tracker.go:112`)
- **Creation site:** `tracker/tracker.go:162`
  ```go
  b := &Blob{
      ID: t.nextID, X: m[0], Y: m[1], Z: m[2],
      Weight:   m[3],
      LastSeen: now,
      Trail:    [][3]float64{{m[0], m[1], m[2]}},
      Posture:  PostureUnknown,
      ukf:      NewUKF(m[0], m[1], m[2]),
  }
  ```
- **Pattern:** Pointer construction (`&Blob{}`) for each unmatched `[x,y,z,weight]`
  measurement; ID assigned from monotonic `t.nextID`; a fresh `UKF` is attached. Fires the
  `onBlobAppeared` callback. The method **returns a deep-copy value snapshot** (`out[i] = *b`
  with trail copied and `ukf` nilled) at `tracker.go:193-200`.

### A2. `(*tracking.Tracker).Update` — 2D tracker
- **Signature:** `func (t *Tracker) Update(measurements [][3]float64) []Blob`
  (`mothership/internal/tracking/tracker.go:91`)
- **Creation site:** `tracking/tracker.go:160`
  ```go
  b := &Blob{
      ID: t.nextID, X: meas[0], Z: meas[1], Weight: meas[2],
      LastSeen: now,
      Trail:    [][2]float64{{meas[0], meas[1]}},
      ukf:      NewUKF(meas[0], meas[1]),
  }
  ```
- **Pattern:** Same as A1 but 2D (`[x,z,weight]`, 2D UKF). Fires `onBlobAppear` callback
  (`BlobEvent{}`). Returns a value snapshot at `tracking/tracker.go:199`.

### A3. `(*tracker.TrackManager).Update` / `.UpdateWithIdentity` — identity-aware wrappers
- **Signatures:**
  - `func (tm *TrackManager) Update(measurements [][4]float64) []Blob`
    (`mothership/internal/tracker/identity.go:63`)
  - `func (tm *TrackManager) UpdateWithIdentity(measurements [][4]float64, identities map[int]*IdentityInfo) []Blob`
    (`identity.go:77`)
- **Pattern:** Thin wrappers over `tracker.Tracker.Update` (A1). They delegate track
  creation to the inner `Tracker`, then call `applyIdentity(blob, info, now)`
  (`identity.go:164`) to stamp BLE identity fields onto the returned blobs. No direct
  `Blob{}` literal — they *route* blobs produced by A1 and mutate identity in place.

---

## B. Accessor factories (return blob copies/pointers to callers)

### B1. `(*tracker.TrackManager).GetBlob`
- **Signature:** `func (tm *TrackManager) GetBlob(id int) *Blob`
  (`mothership/internal/tracker/identity.go:188`)
- **Pattern:** Linear scan over `tm.blobs`; returns `&tm.blobs[i]` for the matching ID, else
  `nil`. Returns a **pointer into internal storage** (callers must not mutate concurrently).

### B2. `(*tracker.TrackManager).GetAllBlobs`
- **Signature:** `func (tm *TrackManager) GetAllBlobs() []Blob`
  (`mothership/internal/tracker/identity.go:201`)
- **Pattern:** `make([]Blob, len(tm.blobs))` + `copy(result, tm.blobs)` — returns a flat
  value-copy slice of every active blob (no trail deep-copy here; trail header shared).

---

## C. Fusion peak factory

### C1. `(*fusion.Engine).Fuse`
- **Signature:** `func (e *Engine) Fuse(links []LinkMotion) *Result`
  (`mothership/internal/fusion/fusion.go:165`)
- **Creation site:** `fusion.go:260`
  ```go
  blobs[i] = Blob{X: p[0], Y: p[1], Z: p[2], Confidence: p[3]}
  ```
- **Pattern:** Value-type construction (`Blob{}`) — one per extracted grid peak
  (`e.grid.Peaks(...)`). Returned inside `*Result.Blobs []Blob`; `Result` also carries
  `PerBlobContributions` and `AllContributions` computed in the same loop. This is the
  origin of every downstream blob (the peaks fed into A1/A3 as measurements).

---

## D. Simulator / replay synthetic factories

### D1. `(*simulator.Engine).detectBlobs`
- **Signature:** `func (e *Engine) detectBlobs() []BlobResult`
  (`mothership/internal/simulator/engine.go:387`)
- **Creation site:** `simulator/engine.go:460`
  ```go
  blobs = append(blobs, BlobResult{
      ID: blobID, Position: blobPos,
      Confidence: math.Min(1.0, value/5.0),
      WalkerID: nearestWalker, TrueError: minDist,
  })
  ```
- **Pattern:** Value construction appended to a slice; one `BlobResult` per grid local
  maximum, annotated with the nearest ground-truth walker and true error. Called from
  `RunSimulation` (`engine.go:193`).

### D2. `(*replay.Pipeline).generateDemoBlobs`
- **Signature:** `func (p *Pipeline) generateDemoBlobs(timestampNS int64) []BlobUpdate`
  (`mothership/internal/replay/pipeline.go:100`)
- **Creation sites:** `pipeline.go:114` and `pipeline.go:132`
  ```go
  blobs = append(blobs, BlobUpdate{ID: 1, X: x1, Z: z1, VX: vx1, VZ: vz1, Weight: 0.8, Trail: ..., Posture: "walking"})
  blobs = append(blobs, BlobUpdate{ID: 2, ...}) // optional second blob
  ```
- **Pattern:** Two fixed-ID synthetic blobs following figure-8 / circular trajectories
  derived from `timestampNS`. Demo-only stand-in for the real pipeline output during replay.

---

## E. Cross-package conversion factories

### E1. `(*blobTracker).track`
- **Signature:** `func (bt *blobTracker) track(result *fusion.Result) []sigproc.TrackedBlob`
  (`mothership/cmd/mothership/main.go:5337`)
- **Creation site:** `main.go:5384`
  ```go
  b := sigproc.TrackedBlob{ID: id, X: pk.X, Y: pk.Y, Z: pk.Z, Weight: pk.Confidence}
  ```
- **Pattern:** Converts each `fusion.Blob` peak (C1) into a `signal.TrackedBlob`, assigning
  / reusing IDs via `bt.nextID` / `bt.prev`, and deriving velocity `(pk.X - pb.X)/dt` from
  the previous frame. Dedicated factory function returning `[]sigproc.TrackedBlob`.

### E2. Automation conversion loop (inline, inside `main`)
- **Location:** `mothership/cmd/mothership/main.go:2213`
  ```go
  autoBlobs[i] = automation.TrackedBlob{ID: b.ID, X: b.X, Y: b.Y, Z: b.Z, VX: b.VX, ...}
  ```
- **Pattern:** Inline value construction inside the main fusion loop (enclosing func is
  `main()` at `main.go:481`), converting the active blob set to `[]automation.TrackedBlob`
  for the trigger engine. **Not a dedicated factory function** — included because it is a
  distinct creation site that must track blob field changes.

### E3. Explainability conversion loop (inline, inside `main`)
- **Location:** `mothership/cmd/mothership/main.go:2116`
  ```go
  blobSnapshots := make([]explainability.BlobSnapshot, 0, len(blobs))
  for _, blob := range blobs {
      blobSnapshots = append(blobSnapshots, explainability.BlobSnapshot{
          ID: blob.ID, X: blob.X, Y: blob.Y, Z: blob.Z, Confidence: blob.Weight,
      })
  }
  ```
- **Pattern:** Sibling of E2 — inline value construction inside `main()`'s fusion loop
  (enclosing func `main()` at `main.go:481`), converting the active blob set into
  `[]explainability.BlobSnapshot` to feed the explainability handler (the input consumed by
  F1 `computeExplanation`). **Not a dedicated factory function** — included because it is
  the only site that constructs `explainability.BlobSnapshot`, and it must track blob field
  changes just like E2. (Discovered in the bf-67ao completeness sweep.)

---

## F. Explainability factories (produce `BlobExplanation`)

### F1. `(*explainability.Handler).computeExplanation`
- **Signature:** `func (h *Handler) computeExplanation(blob BlobSnapshot, links []LinkState, _ *GridSnapshot) *BlobExplanation`
  (`mothership/internal/explainability/handler.go:356`)
- **Creation site:** `handler.go:357`
  ```go
  explanation := &BlobExplanation{BlobID: blob.ID, X/Y/Z, Confidence, ContributingLinks: []LinkContribution{}, AllLinks: ..., FresnelZones: []FresnelZone{}}
  ```
- **Pattern:** The substantive factory — builds the full explanation (contributing links +
  Fresnel zones) for one blob. Result is cached into `h.blobHistory[blob.ID]` by
  `UpdateBlobs` (`handler.go:122`).

### F2. `(*explainability.Handler).explainBlob` (HTTP handler)
- **Signature:** `func (h *Handler) explainBlob(w http.ResponseWriter, r *http.Request)`
  (`handler.go:180`)
- **Creation site:** `handler.go:194` — empty `&BlobExplanation{BlobID: blobID, X:0, Y:0, Z:0, Confidence:0}` returned for an unknown blob ID.

### F3. `(*explainability.Handler).explainBlobAtTime` (HTTP handler)
- **Signature:** `func (h *Handler) explainBlobAtTime(w http.ResponseWriter, r *http.Request)`
  (`handler.go:209`)
- **Creation site:** `handler.go:255` — empty `&BlobExplanation{...}` (with `Timestamp`)
  returned when no historical explanation matches.

### F4. `(*explainability.Handler).GetExplanationForBlob`
- **Signature:** `func (h *Handler) GetExplanationForBlob(blobID int, timestamp int64) *BlobExplanation`
  (`handler.go:276`)
- **Pattern:** Lookup accessor — returns a cached `*BlobExplanation` from `blobHistory` or
  `blobHistoryByTime` (within a 1-minute window); no literal construction of its own, but it
  is the programmatic return path for the records F1 produces.

---

## G. Snapshot recorder (blob-derived record, not a returned blob)

### G1. `(*falldetect.Detector).processBlob`
- **Signature:** `func (d *Detector) processBlob(blob struct{ ID int; X,Y,Z,VX,VY,VZ float64; Posture string }, now time.Time)`
  (`mothership/internal/falldetect/detector.go:270`)
- **Creation site:** `detector.go:277`
  ```go
  snapshot := BlobSnapshot{ID: blob.ID, X/Y/Z, VX/VY/VZ, Posture: blob.Posture, Timestamp: now}
  ```
- **Pattern:** Constructs a `falldetect.BlobSnapshot` **history record** (not a tracked
  blob) appended to `d.blobHistory` for fall-pattern analysis. Included for completeness —
  it is the only site that builds this type.

---

## Summary table

| # | Factory function | File:line (func) | Creates at | Returns |
|---|------------------|------------------|------------|---------|
| A1 | `(*tracker.Tracker).Update` | `tracker/tracker.go:112` | `:162` | `[]Blob` (snapshot) |
| A2 | `(*tracking.Tracker).Update` | `tracking/tracker.go:91` | `:160` | `[]Blob` (snapshot) |
| A3 | `(*tracker.TrackManager).Update[WithIdentity]` | `tracker/identity.go:63,77` | (delegates to A1) | `[]Blob` |
| B1 | `(*tracker.TrackManager).GetBlob` | `tracker/identity.go:188` | — | `*Blob` |
| B2 | `(*tracker.TrackManager).GetAllBlobs` | `tracker/identity.go:201` | — | `[]Blob` (copy) |
| C1 | `(*fusion.Engine).Fuse` | `fusion/fusion.go:165` | `:260` | `*Result` (`.Blobs []Blob`) |
| D1 | `(*simulator.Engine).detectBlobs` | `simulator/engine.go:387` | `:460` | `[]BlobResult` |
| D2 | `(*replay.Pipeline).generateDemoBlobs` | `replay/pipeline.go:100` | `:114,:132` | `[]BlobUpdate` |
| E1 | `(*blobTracker).track` | `cmd/mothership/main.go:5337` | `:5384` | `[]sigproc.TrackedBlob` |
| E2 | automation loop (in `main`) | `cmd/mothership/main.go:2213` | `:2213` | (inline conversion) |
| E3 | explainability loop (in `main`) | `cmd/mothership/main.go:2116` | `:2116` | (inline conversion) |
| F1 | `(*explainability.Handler).computeExplanation` | `explainability/handler.go:356` | `:357` | `*BlobExplanation` |
| F2 | `(*explainability.Handler).explainBlob` | `explainability/handler.go:180` | `:194` | (writes JSON) |
| F3 | `(*explainability.Handler).explainBlobAtTime` | `explainability/handler.go:209` | `:255` | (writes JSON) |
| F4 | `(*explainability.Handler).GetExplanationForBlob` | `explainability/handler.go:276` | — | `*BlobExplanation` |
| G1 | `(*falldetect.Detector).processBlob` | `falldetect/detector.go:270` | `:277` | (records snapshot) |

---

## Acceptance Criteria Status

- [x] All blob factory functions are identified — **16 documented creation sites** (14
  dedicated functions/methods + 2 inline conversion loops in `main()`) across **10 blob
  types**. The 10th type (`explainability.BlobSnapshot`) and its sole creation site
  (`main.go:2116`, entry E3) were added during the bf-67ao completeness sweep.
- [x] Each location documented with file path and line number — see table above (function
  def + literal site).
- [x] Function signature and blob creation pattern noted — each entry lists the full
  signature and the construction idiom (`&Blob{}` pointer, `Blob{}` value, slice copy,
  delegation, or record append).

### Notes / caveats

- The data flow is: **C1 (`Fuse`)** produces `fusion.Blob` peaks → **E1 (`track`)** converts
  them to `signal.TrackedBlob` → **A1/A3 (`Update`)** spawn persistent `tracker.Blob`
  tracks → **B1/B2** hand copies to consumers. D1/D2 are offline/test-path factories; F1–F4
  are explainability; G1 is a fall-detect recorder.
- The 2D `tracking` package (A2) appears to be a legacy/alternate tracker; the active
  pipeline uses the 3D `tracker` package (A1/A3). Both are listed for completeness.
- Several earlier beads already surveyed literal sites and JS/TS blob objects
  (`bf-4bhd`, `bf-3tlw`, `bf-26ta`, `bf-5kns`, `bf-4ly4`); this report is the
  function-level (factory) view and does not duplicate the per-literal inventory.
- **Test-only factories (deliberately excluded from the production catalogue above):** the
  completeness sweep found these in `*_test.go`, all constructing/returning blob-typed
  values but only as test fixtures or mocks:
  - `internal/explainability/handler_test.go:30` `makeBlobAt(...) BlobSnapshot` — builds an
    `explainability.BlobSnapshot` fixture.
  - `internal/replay/integration_test.go:578` `processFramesDirectly(...) []BlobUpdate` and
    `:606` `processFramesWithThreshold(...) []BlobUpdate` — run the replay pipeline over
    test frames and return its `BlobUpdate` output (no literal of their own).
  - `internal/api/tracks_test.go:17` `mockTracksProvider.GetTrackedBlobs() []TrackedBlob` —
    mock interface stub returning a canned blob slice.
  - `internal/replay/{engine,pipeline}_test.go` `mockBroadcaster.BroadcastReplayBlobs(...)`
    — mocks that *receive* blobs as a parameter (not factories).
- **Struct constructors (correctly excluded):** `cmd/mothership/main.go:5329`
  `newBlobTracker() *blobTracker` constructs the *tracker*, not a blob. There are no
  `NewBlob`/`makeBlob`/`CreateBlob` blob-constructor helpers in production code.
