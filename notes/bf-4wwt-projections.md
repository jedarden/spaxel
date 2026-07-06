# Blob Projection & Derived Types — Inventory (bf-4wwt)

**Scope:** the blob-shaped **PROJECTION / derived types** — read-only views built *from* a
tracked blob for a subsystem (not new tracked entities, no UKF, no stable session ID).
These matter because any field added to a tracked blob that a projection copies must be
propagated at every construction site below.

**Re-verified against HEAD `3591865` on 2026-07-06.** Companion to `notes/bf-4bhd.md`
("Related Blob-shaped projection types" table) and `notes/bf-55rp-primary-types.md`
(primary tracked-blob types) / `notes/bf-5uzm-conversions.md` (cross-package conversions).

---

## Re-verification summary vs `bf-4bhd`

The 9 projection types and 15 non-test construction sites from `bf-4bhd` were all
re-located at current HEAD. **Only the two `cmd/mothership/main.go` sites moved** — both
shifted **+90 lines** (code inserted above the fusion loop). Every other site is unchanged.

| Site (bf-4bhd) | Site (HEAD 3591865) | Delta |
|---|---|---|
| `cmd/mothership/main.go:2116` (`explainability.BlobSnapshot`) | `cmd/mothership/main.go:2206` | **+90** |
| `cmd/mothership/main.go:2236` (`volume.BlobPos`) | `cmd/mothership/main.go:2326` | **+90** |
| all other 13 sites | unchanged | 0 |

Re-locate future moves with:
```bash
grep -rnE "BlobSnapshot\{|BlobState\{|BlobPos\{|BlobUpdate\{|BlobEvent\{|BlobResult\{|BlobExplanation\{" mothership/
```
(`grep -E` is required — plain BRE treats `|` as a literal pipe and matches nothing.)

---

## Projection type definitions

All 9 definitions confirmed at their cited lines:

| # | Type | Definition (file:line) | Built from | Notes |
|---|------|------------------------|-----------|-------|
| 1 | `explainability.BlobExplanation` | `internal/explainability/handler.go:27` | derived (from `BlobSnapshot` + link state) | Rendered "Why?" overlay result; JSON-facing. Not a direct blob copy. |
| 2 | `explainability.BlobSnapshot` | `internal/explainability/handler.go:95` | tracked blob pos + Weight | Lightweight blob view: `{ID, X,Y,Z, Confidence, Weight}`. Input to the explainability handler. |
| 3 | `falldetect.BlobSnapshot` | `internal/falldetect/detector.go:69` | anonymous struct (see note A) | Z-trajectory input to the fall-detect state machine: `{ID, X,Y,Z, VX,VY,VZ, Posture, Timestamp, DeltaRMS}`. **Distinct type** from `explainability.BlobSnapshot` despite the shared name. |
| 4 | `volume.BlobState` | `internal/volume/shape.go:139` | tracked blob ID (see note C) | Per-blob state inside the trigger state machine: `{BlobID, Inside, EnterTime, LastCheckTime}`. Owns no position — position comes in via `volume.BlobPos`. |
| 5 | `volume.BlobPos` | `internal/volume/shape.go:1080` | tracked blob pos | `{ID, PersonID, X, Y, Z}`. Position input to volume point-in-volume tests. |
| 6 | `api.BlobPos` | `internal/api/triggers.go:624` | *(none in production — see note B)* | `{ID, X, Y, Z}` (no `PersonID`). **Test-only / dead in production.** |
| 7 | `tracking.BlobEvent` | `internal/tracking/tracker.go:52` | `tracking.Blob` | Lifecycle event: `{BlobID, X, Z, Timestamp}`. Emitted on appear/disappear (2D tracker; X,Z only). |
| 8 | `replay.BlobUpdate` | `internal/replay/types.go:303` | synthetic (see note D) | Incremental replay frame pushed to dashboard: `{ID, X,Y,Z, VX,VY,VZ, Weight, Posture, PersonID/Label/Color, Trail, ...}`. |
| 9 | `simulator.BlobResult` | `internal/simulator/engine.go:80` | synthetic (see note D) | Simulated detection from `spaxel-sim`'s internal fusion grid: `{ID, Position, Confidence, Velocity, WalkerID, TrueError}`. Test fixture. |

---

## Construction sites (non-test, verified at HEAD `3591865`)

15 non-test sites — exactly the count in `bf-4bhd`. Plus one additional boundary site
for falldetect (note A) that `bf-4bhd` did not enumerate.

### `explainability.BlobSnapshot` (1 site)
| Site | Built from | Context |
|---|---|---|
| `cmd/mothership/main.go:2206` | `signal.TrackedBlob` (`blobs` loop var) | Fusion loop builds `[]BlobSnapshot` for the "Why?" overlay. Copies `ID, X, Y, Z, Weight`. |

### `explainability.BlobExplanation` (3 sites)
| Site | Built from | Context |
|---|---|---|
| `internal/explainability/handler.go:194` | blobID only (zero fallback) | `GetExplanationForBlob` returns empty explanation for an unknown blob. |
| `internal/explainability/handler.go:255` | blobID only (zero fallback) | Empty explanation when nothing is found in history. |
| `internal/explainability/handler.go:357` | `BlobSnapshot` param | `computeExplanation` — the real derivation: copies `ID, X, Y, Z, Confidence` from the snapshot, then attaches contributing links / BLE match / Fresnel zones. |

### `falldetect.BlobSnapshot` (1 site + 1 boundary)
| Site | Built from | Context |
|---|---|---|
| `internal/falldetect/detector.go:277` | anonymous-struct `blob` param | `processBlob` copies `ID, X,Y,Z, VX,VY,VZ, Posture, Timestamp` into the snapshot. |
| `cmd/mothership/main.go:2288` *(boundary, not in bf-4bhd)* | `signal.TrackedBlob` | Fusion loop builds the anonymous-struct slice inline (`[]struct{ID,X,Y,Z,VX,VY,VZ,Posture}{{...}}`) per blob and calls `fallDetector.Update(...)`. **This is where tracked-blob fields actually cross into falldetect** — see note A. |

### `volume.BlobState` (4 sites — only one is blob-derived; see note C)
| Site | Built from | Context |
|---|---|---|
| `internal/volume/shape.go:375` | persisted int64 timestamps | **Deserialization** — restores trigger state from DB (`inside, enterTimeMs, lastCheckMs`). Not blob-derived. |
| `internal/volume/shape.go:575` | `volume.BlobPos` (`blob.ID`) | **Only blob-derived site** — first-seen state for a blob entering evaluation. |
| `internal/volume/shape.go:820` | sentinel `-999` | In-memory-only slot storing the inside-count. Not a blob. |
| `internal/volume/shape.go:879` | negative hash key (`-1000-hash`) | Prediction-based synthetic state for a person×zone combination. Not a tracked blob. |

### `volume.BlobPos` (1 site)
| Site | Built from | Context |
|---|---|---|
| `cmd/mothership/main.go:2326` | `signal.TrackedBlob` | Fusion loop builds `[]volume.BlobPos` for trigger evaluation. Copies `ID, X, Y, Z` — **does NOT copy `PersonID`** (left zero in production despite the field existing). |

### `api.BlobPos` (0 production sites — see note B)
| Site | Built from | Context |
|---|---|---|
| *(none)* | — | The only literal construction is `internal/api/triggers_test.go:777`. `TriggersHandler.EvaluateTriggers([]BlobPos)` at `api/triggers.go:549` is **not wired in `main.go`** (only `volumeTriggersHandler` is constructed/called). Test-only / dead code. |

### `tracking.BlobEvent` (2 sites)
| Site | Built from | Context |
|---|---|---|
| `internal/tracking/tracker.go:173` | `*tracking.Blob` (`b`) | `onBlobAppear` — new track created. Copies `ID, X, Z`. |
| `internal/tracking/tracker.go:188` | `*tracking.Blob` (`b`) | `onBlobDisappear` — stale track removed. Copies `ID, X, Z`. |

### `replay.BlobUpdate` (2 sites — synthetic; see note D)
| Site | Built from | Context |
|---|---|---|
| `internal/replay/pipeline.go:114` | computed (figure-8 formula) | `generateDemoBlobs` — synthetic blob 1. |
| `internal/replay/pipeline.go:132` | computed (circular formula) | `generateDemoBlobs` — synthetic blob 2 (intermittent). |

### `simulator.BlobResult` (1 site — synthetic; see note D)
| Site | Built from | Context |
|---|---|---|
| `internal/simulator/engine.go:460` | simulator fusion-grid peak + nearest walker | `detectBlobs` — synthetic detection emitted by `spaxel-sim`. |

**Total: 15 non-test sites** (matching `bf-4bhd`) **+ 1 falldetect boundary site** (main.go:2288).

---

## The shared source: `[]signal.TrackedBlob` in the fusion loop

All four production blob-derived projections (`explainability.BlobSnapshot`,
`volume.BlobPos`, the falldetect anonymous struct, and `automation.TrackedBlob`) are built
from the **same** `blobs` slice in the mothership fusion loop:

```go
// cmd/mothership/main.go:2039, 2057
blobTracker := newBlobTracker()
blobs := blobTracker.track(result)   // []sigproc.TrackedBlob  (sigproc = internal/signal)
```

`sigproc` is the import alias for `github.com/spaxel/mothership/internal/signal`
(`main.go:63`), so `blobs` is `[]signal.TrackedBlob`. No reassignment of `blobs` occurs
between `track()` (line 2057) and the projection sites (2288/2206/2326) — confirmed by
scoped grep. **A field added to `signal.TrackedBlob` that any of these four projections
should copy must be added at all four sites** (plus the `automation.TrackedBlob`
conversion at `main.go:2303`, covered in `bf-5uzm`).

---

## Notes / corrections to `bf-4bhd`

### Note A — `falldetect` enters via an anonymous struct, not a named blob type
`falldetect.Detector.Update(blobs []struct{ID, X,Y,Z, VX,VY,VZ, Posture})`
(`detector.go:249`) and `processBlob(blob struct{...})` (`detector.go:270`) take an
**inline anonymous struct**, not a named tracked-blob type. The projection
`falldetect.BlobSnapshot` is copied from that anonymous struct at `detector.go:277`, but
the **actual boundary where tracked-blob fields enter the subsystem** is
`cmd/mothership/main.go:2288`, where `signal.TrackedBlob` is converted into the anonymous
struct literal inline. `bf-4bhd` enumerated only `detector.go:277`; the main.go:2288
boundary site is added here. Any new `signal.TrackedBlob` field that fall-detection needs
must be threaded through both `main.go:2288` (the literal) and the anonymous-struct
signature at `detector.go:249/270/349/417/480/504` (every method that takes the struct).

### Note B — `api.BlobPos` is test-only / dead in production
`api.BlobPos` (`api/triggers.go:624`) and its consumer
`TriggersHandler.EvaluateTriggers([]BlobPos)` (`api/triggers.go:549`) have **no production
caller**. `main.go` wires only `volumeTriggersHandler` (`api.NewVolumeTriggersHandler` at
`main.go:1795`; `EvaluateTriggers(volumeBlobs)` at `main.go:2333`), which takes
`[]volume.BlobPos` (`api/volume_triggers.go:782`). The sole `api.BlobPos{}` literal is in
`triggers_test.go:777`. Treat `api.BlobPos` as a vestigial duplicate of `volume.BlobPos`
(fewer fields — no `PersonID`); do not rely on it for production field propagation.

### Note C — `volume.BlobState` is mostly not blob-derived
Of its 4 construction sites, **only `shape.go:575` is built from a blob** (`volume.BlobPos.ID`).
`shape.go:375` deserializes persisted trigger state from int64 timestamps;
`shape.go:820` creates a sentinel `-999` slot that smuggles the inside-count through the
same map; `shape.go:879` creates a negative-keyed prediction state. `bf-4bhd`'s "built
from tracked blob" label applies cleanly only to site 575.

### Note D — `replay.BlobUpdate` and `simulator.BlobResult` are synthetic, not blob copies
Neither is built from a tracked blob:
- `replay.BlobUpdate` sites (`pipeline.go:114,132`) live in `generateDemoBlobs`
  (`pipeline.go:100`) and compute positions from trig formulas (figure-8 / circular
  walkers) — synthetic demo data for the replay preview, not a copy of a live/replayed blob.
- `simulator.BlobResult` (`engine.go:460`) is built inside `detectBlobs()` from the
  simulator's own fusion-grid peak (`blobPos`, `value`) and the nearest synthetic walker.

A new tracked-blob field does **not** need to be propagated to these two types — they
generate their own data. They are listed for completeness as blob-*shaped* projections,
not blob-*derived* ones.

### Note E — `volume.BlobPos.PersonID` is not populated in production
`volume.BlobPos` has a `PersonID` field (`shape.go:1082`), but the sole production
construction (`main.go:2326`) copies only `ID, X, Y, Z` and leaves `PersonID = ""`. If a
trigger ever needs identity, this is the site to fix.

---

## Field-propagation checklist (when adding a field to `signal.TrackedBlob`)

Production projection sites that copy tracked-blob fields and would need updating:
1. `cmd/mothership/main.go:2206` — `explainability.BlobSnapshot`
2. `cmd/mothership/main.go:2288` — falldetect anonymous struct (+ the `detector.go` anon-struct signatures)
3. `cmd/mothership/main.go:2326` — `volume.BlobPos`
4. `cmd/mothership/main.go:2303` — `automation.TrackedBlob` (cross-package conversion; see `bf-5uzm`)

(For `tracking.Blob` fields specifically: update `tracking.BlobEvent` at
`tracker.go:173,188`. For `tracker.Blob` (3D): no projection in this set copies from it
directly — `explainability.BlobSnapshot` and `falldetect.BlobSnapshot` are fed from
`signal.TrackedBlob`, not `tracker.Blob`, at the production boundary.)

---

## Acceptance criteria

- [x] Each projection type definition recorded with file:line and which tracked-blob type it is built from
- [x] Every projection construction boundary site listed (file:line)
- [x] Re-located moved sites with the `grep -rnE "..."` command (main.go sites moved +90)
- [x] All line numbers verified against current HEAD (`3591865`, 2026-07-06)
- [x] Findings written to `notes/bf-4wwt-projections.md`
