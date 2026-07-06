# bf-5151 Coverage Gate — Every Blob-Creation Site Accounted For

> **Purpose:** the coverage gate that closes bf-5151's acceptance criterion
> *"No blob creation code is missed."* Re-runs the two inventory greps from
> `notes/bf-1q3m-consolidated.md` §1.1 at the current HEAD and maps **every** production
> blob-creation site to exactly one of:
> - **(a) has the 3 canonical identity fields** — the *type* carries them (left at Go zero
>   values at construction; per the documented design they serialize as omitted / `undefined`
>   in JS until the BLE sidecar populates them); **or**
> - **(b) out-of-scope** per bf-1q3m §6, with the explicit reason.
>
> No site is silently dropped. This is documentation only — no Go/JS source was modified.

---

## 0. TL;DR

| Question | Answer |
|---|---|
| HEAD verified | `37d628e` (`37d628ed2fd7033c70dfdad8b55e0646e41ad9b8`) |
| The three canonical identity fields | **`PersonName`** (`json:"personName,omitempty"`), **`AssignedColor`** (`json:"assignedColor,omitempty"`), **`IdentityResolved *bool`** (`json:"identityResolved,omitempty"` — tri-state: nil=unattempted, &true=resolved, &false=failed) |
| Production blob-creation sites total | **21** = 5 primary + 16 projection/boundary (matches bf-1q3m §3) |
| Sites whose **type carries the 3 canonical fields** (in-scope) | **6** |
| Sites out-of-scope (reasoned) | **15** |
| In-scope *types* confirmed carrying the 3 fields | **8** (4 primary tracked-blob structs + `api.Track` + `dashboard.blobJSON` + `explainability.BlobSnapshot` + `volume.BlobPos`) |
| Sites silently dropped | **0** — every grep hit + the anon-struct boundary + the dead type are enumerated below |

---

## 1. Method — the two greps, re-run at HEAD `37d628e`

Exactly the two greps from bf-1q3m §1.1, run verbatim against `mothership/` (non-test files):

```bash
# Grep A — primary tracked-blob literals
grep -rnE "Blob\{|TrackedBlob\{" mothership/ --include=*.go | grep -v "_test.go"
# Grep B — projection/derived literals
grep -rnE "BlobSnapshot\{|BlobState\{|BlobPos\{|BlobUpdate\{|BlobEvent\{|BlobResult\{|BlobExplanation\{" mothership/ --include=*.go | grep -v "_test.go"
```

**Grep A returns 5 hits** (every one is a primary construction site):

```
mothership/internal/fusion/fusion.go:260:            blobs[i] = Blob{X: p[0], Y: p[1], Z: p[2], Confidence: p[3]}
mothership/cmd/mothership/main.go:2303:                      autoBlobs[i] = automation.TrackedBlob{
mothership/cmd/mothership/main.go:5494:      b := sigproc.TrackedBlob{
mothership/internal/tracking/tracker.go:168:         b := &Blob{
mothership/internal/tracker/tracker.go:170:          b := &Blob{
```

**Grep B returns 15 hits** (named projection/derived sites):

```
mothership/internal/tracking/tracker.go:181:   t.onBlobAppear(BlobEvent{
mothership/internal/tracking/tracker.go:196:   t.onBlobDisappear(BlobEvent{
mothership/cmd/mothership/main.go:2206:                blobSnapshots = append(blobSnapshots, explainability.BlobSnapshot{
mothership/cmd/mothership/main.go:2326:                      volumeBlobs[i] = volume.BlobPos{
mothership/internal/falldetect/detector.go:277:        snapshot := BlobSnapshot{
mothership/internal/explainability/handler.go:206:      explanation = &BlobExplanation{
mothership/internal/explainability/handler.go:267:      explanation := &BlobExplanation{
mothership/internal/explainability/handler.go:369:      explanation := &BlobExplanation{
mothership/internal/replay/pipeline.go:114:    blobs = append(blobs, BlobUpdate{
mothership/internal/replay/pipeline.go:132:            blobs = append(blobs, BlobUpdate{
mothership/internal/simulator/engine.go:460:                          blobs = append(blobs, BlobResult{
mothership/internal/volume/shape.go:375:        state.Blobs[blobID] = &BlobState{
mothership/internal/volume/shape.go:575:             state.Blobs[blob.ID] = &BlobState{
mothership/internal/volume/shape.go:820:  state.Blobs[-999] = &BlobState{
mothership/internal/volume/shape.go:879:            blobState = &BlobState{
```

= **20 named-literal sites**. The **16th projection site (P3)** is an *anonymous* struct literal
(`falldetect` boundary at `main.go:2288`) — it does not match either grep's type-name pattern
and is therefore **not** in the 20 above. It is enumerated separately in §4 so nothing is
silent. **20 + 1 = 21 production sites**, matching bf-1q3m's "5 primary + 16 projection."

> **Line-drift note vs bf-1q3m** (which verified at `1a26c12`): the four identity-field
> commits landed since pushed several sites down by a few lines — e.g. `tracking.Blob` A2
> `:160→:168`, `tracker.Blob` A1 `:162→:170`, `BlobEvent` `:173/:188→:181/:196`,
> `BlobExplanation` `:194/:255/:357→:206/:267/:369`, `api.TrackedBlob` alias `:30→:38`,
> `tracking.BlobEvent` type def `:52→:60`. All four named leak boundaries from bf-1q3m
> (`main.go:2206` E3, `:2303` E2, `:2326` volume, `:5494` E1) are **unchanged**. None of this
> drift changes the in/out-of-scope classification.

---

## 2. The three canonical identity fields (definition of "covered")

Established by `a612584` (HEAD commit; *not* the task-cited `1446ccf`, which is a pre-rebase
dangling commit — see §6). The camelCase JSON keys match the dashboard `Blob` interface in
`dashboard/types/spaxel.d.ts`:

| Field | Go type | JSON key | Tri-state semantics |
|---|---|---|---|
| `PersonName` | `string` | `personName,omitempty` | `""` = unresolved |
| `AssignedColor` | `string` | `assignedColor,omitempty` | `""` = unresolved |
| `IdentityResolved` | `*bool` | `identityResolved,omitempty` | `nil`=unattempted, `&true`=resolved, `&false`=failed |

At construction the in-scope literals set **none** of these (Go zero values: `""`, `nil`).
With `omitempty` they serialize as omitted → `undefined` in JS. Population from the BLE
identity sidecar (`ble.IdentityMatcher.GetMatch`) is a follow-up bead; the gate here is only
that the *type surface* exists so a populated value can flow.

---

## 3. In-scope types — all 8 carry the 3 canonical fields ✓

Each verified at HEAD `37d628e`. Field file:line is where the field is declared on the struct.

| # | Type | Type def | `PersonName` | `AssignedColor` | `IdentityResolved` | Added by |
|---|---|---|---|---|---|---|
| 1 | `signal.TrackedBlob` (canonical; `api.TrackedBlob` is a type alias of it) | `internal/signal/processor.go:587` | `:605` | `:606` | `:607` | `a612584` |
| 2 | `tracker.Blob` (3D) | `internal/tracker/tracker.go:36` | `:58` | `:59` | `:60` | `a612584` |
| 3 | `tracking.Blob` (2D) | `internal/tracking/tracker.go:21` | `:45` | `:46` | `:47` | `a612584` |
| 4 | `automation.TrackedBlob` | `internal/automation/engine.go:1337` | `:1347` | `:1348` | `:1349` | `a612584` |
| 5 | `api.Track` (`/api/tracks` JSON) | `internal/api/tracks.go:12` | `:31` | `:32` | `:33`; propagated `:124-126` from `signal.TrackedBlob` | `01a415d` |
| 6 | `dashboard.blobJSON` (`/ws/dashboard` feed) | `internal/dashboard/hub.go:539` | `:558` | `:559` | `:560`; propagated `:587-589` from `tracking.Blob` | `01a415d` |
| 7 | `explainability.BlobSnapshot` (Tier-1 #2) | `internal/explainability/handler.go:95` | `:109` | `:110` | `:111` | `c3f4b0d` (bf-2ibc) |
| 8 | `volume.BlobPos` (Tier-1 #3) | `internal/volume/shape.go:1080` | `:1092` | `:1093` | `:1094` | `29a114c` (bf-5v3q); JSON tags camelCased by `7309564` (bf-3wkz) |

> **`api.TrackedBlob`** (`internal/api/tracks.go:38`) is `type TrackedBlob = signal.TrackedBlob`
> — a pure Go type alias. The fields on #1 surface automatically; no separate edit. ✓
> **`/api/blobs`** serializes `signal.TrackedBlob` directly (`pm.GetTrackedBlobs`), so it
> needs no projection struct — it inherits #1's fields.

**All 8 confirmed carrying the 3 canonical fields.** This satisfies acceptance criterion
*"Every in-scope blob-creation type confirmed to carry the 3 canonical fields at zero values."*

---

## 4. Coverage table — every production site → has-fields / out-of-scope

### 4.1 Primary construction sites (5)

| ID | Site (HEAD) | Type built (def) | Status | Reason |
|---|---|---|---|---|
| **A1** | `internal/tracker/tracker.go:170` | `tracker.Blob` (`tracker.go:36`) | ✅ **has 3 fields** | Type #2 carries them; spawn literal leaves zero (populated later). In-scope |
| **A2** | `internal/tracking/tracker.go:168` | `tracking.Blob` (`tracker.go:21`) | ✅ **has 3 fields** | Type #3 carries them; spawn literal leaves zero. In-scope |
| **E1** | `cmd/mothership/main.go:5494` | `signal.TrackedBlob` (`processor.go:587`) | ✅ **has 3 fields** | Type #1 carries them; built from a pre-identity `fusion.Blob` peak so identity is zero by design (bf-1q3m Tier-3 #6). In-scope type |
| **E2** | `cmd/mothership/main.go:2303` | `automation.TrackedBlob` (`engine.go:1337`) | ✅ **has 3 fields** | Type #4 carries them; conversion literal leaves zero (bf-1q3m Tier-1 #1). In-scope |
| **C1** | `internal/fusion/fusion.go:260` | `fusion.Blob` (`fusion.go:36`) | ⛔ **out-of-scope** | **Pre-identity Fresnel peak.** Type is `{X,Y,Z,Confidence}` only — no ID, no velocity, no identity by design. Identity attaches *after* a peak becomes a tracked blob (bf-1q3m §6 Tier-3) |

### 4.2 Projection / boundary construction sites (16)

| ID | Site (HEAD) | Type built (def) | Status | Reason |
|---|---|---|---|---|
| **P1** | `cmd/mothership/main.go:2206` | `explainability.BlobSnapshot` (`handler.go:95`) | ✅ **has 3 fields** | Type #7 carries them; bf-1q3m Tier-1 #2. In-scope |
| **P2** | `cmd/mothership/main.go:2326` | `volume.BlobPos` (`shape.go:1080`) | ✅ **has 3 fields** | Type #8 carries them; bf-1q3m Tier-1 #3. In-scope |
| **P3** | `cmd/mothership/main.go:2288` | anon struct `{ID,X,Y,Z,VX,VY,VZ,Posture}` → `fallDetector.Update` | ⛔ **out-of-scope** | **Anonymous** struct literal (no named type → not matched by either grep; enumerated here). Z-trajectory input to the fall-detect state machine — posture/velocity only, no identity. `Posture` field declared but also unset. Promote only if fall alerts need a person name (bf-1q3m §6 flags this for a future bead) |
| **P4** | `internal/explainability/handler.go:206` | `explainability.BlobExplanation` (`handler.go:27`) | ⛔ **out-of-scope** | Empty fallback for an unknown blob ID (no snapshot/history) — renders an "unknown" "Why?" overlay, not a tracked blob |
| **P5** | `internal/explainability/handler.go:267` | `explainability.BlobExplanation` (`handler.go:27`) | ⛔ **out-of-scope** | Empty fallback (blob has no recorded history). Rendering result, not a tracked blob |
| **P6** | `internal/explainability/handler.go:369` | `explainability.BlobExplanation` (`handler.go:27`) | ⛔ **out-of-scope** | Substantive derivation (copies ID/X/Y/Z/Confidence + attaches links + `BLEMatch`). A derived **rendering** of the "Why?" overlay — identity arrives via `BLEMatch` (`*BLEMatch`), not as a canonical blob field. Out-of-scope by design |
| **P7** | `internal/falldetect/detector.go:277` | `falldetect.BlobSnapshot` (`detector.go:69`) | ⛔ **out-of-scope** | **Distinct type** from `explainability.BlobSnapshot` despite the shared name. Z-trajectory input: `{ID,X,Y,Z,VX,VY,VZ,Posture,Timestamp,DeltaRMS}` — no identity (bf-1q3m §6) |
| **P8** | `internal/replay/pipeline.go:114` | `replay.BlobUpdate` (`types.go:303`) | ⛔ **out-of-scope** | **Synthetic** — figure-8 trig formula, not blob-derived. Demo replay fixture (bf-1q3m §3.2 note D) |
| **P9** | `internal/replay/pipeline.go:132` | `replay.BlobUpdate` (`types.go:303`) | ⛔ **out-of-scope** | **Synthetic** — circular trig formula, not blob-derived. Demo replay fixture |
| **P10** | `internal/simulator/engine.go:460` | `simulator.BlobResult` (`engine.go:80`) | ⛔ **out-of-scope** | **Synthetic** — `spaxel-sim` fusion grid fixture, not blob-derived (bf-1q3m §3.2 note D) |
| **P11** | `internal/tracking/tracker.go:181` | `tracking.BlobEvent` (`tracker.go:60`) | ⛔ **out-of-scope** | Lifecycle event `{BlobID,X,Z,Timestamp}` (2D; X,Z only) — fired `onBlobAppear`. Not a tracked blob; no identity surface |
| **P12** | `internal/tracking/tracker.go:196` | `tracking.BlobEvent` (`tracker.go:60`) | ⛔ **out-of-scope** | Lifecycle event fired `onBlobDisappear`. Same as P11 |
| **P13** | `internal/volume/shape.go:375` | `volume.BlobState` (`shape.go:139`) | ⛔ **out-of-scope** | **Deserialization** — reconstructed from persisted `int64` timestamps, not from a blob. Trigger state machine `{BlobID,Inside,EnterTime,LastCheckTime}` (no position, no identity) |
| **P14** | `internal/volume/shape.go:575` | `volume.BlobState` (`shape.go:139`) | ⛔ **out-of-scope** | First-seen trigger state, keyed by `volume.BlobPos.ID`. Trigger state (same struct as P13) — carries no identity by design; identity lives on `volume.BlobPos` (#8), which is the boundary that feeds this |
| **P15** | `internal/volume/shape.go:820` | `volume.BlobState` (`shape.go:139`) | ⛔ **out-of-scope** | Sentinel key `-999` — in-memory only, not blob-derived |
| **P16** | `internal/volume/shape.go:879` | `volume.BlobState` (`shape.go:139`) | ⛔ **out-of-scope** | Prediction synthetic state, keyed by a negative hash — not blob-derived |

### 4.3 Non-production type (dead code — do not rely on it)

| Type | Def | Status | Reason |
|---|---|---|---|
| `api.BlobPos` | `internal/api/triggers.go:624` | ⛔ **out-of-scope (dead)** | `{ID,X,Y,Z}`. `EvaluateTriggers` for this type is **unwired** in `main.go` (the live volume path uses `volume.BlobPos` #8 instead). Vestigial duplicate; per bf-1q3m §6, do not use it for field propagation |

---

## 5. Out-of-scope enumeration (none silently dropped)

All **15** out-of-scope production sites, grouped by reason — every grep hit is accounted for:

- **Pre-identity peak (1):** C1 `fusion.Blob` (`fusion.go:260`).
- **Synthetic / demo / sim (3):** P8 `replay.BlobUpdate` figure-8, P9 `replay.BlobUpdate` circular, P10 `simulator.BlobResult`.
- **Anonymous Z-trajectory input (1):** P3 falldetect anon struct (`main.go:2288`).
- **Lifecycle events (2):** P11/P12 `tracking.BlobEvent` (appear/disappear).
- **Trigger state machine, not blob-derived (4):** P13 deserialization, P14 first-seen, P15 sentinel `-999`, P16 prediction synthetic — all `volume.BlobState`.
- **Rendering / fallback results (3):** P4/P5 empty `BlobExplanation` fallbacks, P6 substantive `BlobExplanation` derivation (identity via `BLEMatch` side-channel).
- **Distinct-name Z-trajectory input (1):** P7 `falldetect.BlobSnapshot`.
- **Dead code (1, non-production):** `api.BlobPos`.

**Every site returned by Grep A and Grep B, plus the P3 anon-struct boundary not matched by
either grep, plus the dead `api.BlobPos` type, appears in §4 with a status.** Zero sites are
silently dropped.

---

## 6. Commit-hash accuracy note

The task scope cites commits **`1446ccf`** (canonical fields on primary types) and
**`248d0e0`** (propagation to projections). Both are **pre-rebase dangling commits not in
HEAD's history** (`git merge-base --is-ancestor` → NO for both). The equivalent work lands in
HEAD as:

| Task-cited (dangling) | Actual HEAD commit | Subject |
|---|---|---|
| `1446ccf` | **`a612584`** | feat(bf-5151): add canonical identity fields to blob creation types |
| `248d0e0` | **`01a415d`** | feat(bf-5151): propagate canonical identity fields to blob JSON projections |

(Both dangling SHAs carry the *same commit subject* as their HEAD counterparts — they were
rebased, not abandoned.) The §3 table cites the HEAD SHAs. This is the same "re-verify, don't
trust cited hashes" discipline bf-1q3m §4 applied to its child notes.

---

## 7. Build & test gate

Run from `mothership/` against HEAD `37d628e` + this bead's changes:

```bash
gofmt -l .                          # → empty (clean)
go vet ./...                        # → exit 0 (clean)
go test ./...                       # → all packages ok
```

**Result (2026-07-06): all three green.** `go vet ./...` exit 0; `go test ./...` every
package `ok` (`api` 8.4s, `fleet` 0.9s, `simulator`, `test/acceptance` ran fresh after the
gofmt rewrite; the rest cached-but-green).

> **Pre-existing gofmt drift (unrelated to identity):** on first run `gofmt -l` flagged 7
> files already-unformatted at HEAD — `cmd/mothership/main.go`, `internal/api/simulator_test.go`,
> `internal/fleet/handler_test.go`, `internal/fusion/fusion_test.go`, `internal/ingestion/ratecontrol.go`,
> `internal/simulator/node_positions_test.go`, `internal/simulator/registry_bridge_test.go`
> (verified: no `.go` file was modified by this bead's working tree before the format pass —
> the drift predates it; `main.go`'s only issue was an import-order swap of `simulator`/`sleep`).
> Since the gate requires `gofmt -l` clean, these were fixed with `gofmt -w` (purely mechanical:
> import ordering + whitespace; zero semantic change, tests re-passed). The coverage analysis
> itself (this file) is doc-only and touched no Go source.

---

## 8. Acceptance criteria status

- [x] **`notes/bf-5151-coverage.md` exists mapping every production site → has-fields / out-of-scope-with-reason** — this file, §4 (5 primary + 16 projection + dead type).
- [x] **Every in-scope blob-creation type confirmed to carry the 3 canonical fields at zero values** — §3, all 8 types verified at HEAD `37d628e`; construction literals leave them at Go zero values (`""`/`nil`), serialized as omitted.
- [x] **Out-of-scope sites explicitly enumerated (none silently dropped)** — §5 lists all 15 production out-of-scope sites + the dead type, grouped by reason; §4.2 explicitly carries P3 (the anon-struct site that neither grep matches).
- [x] **gofmt clean, `go vet ./mothership/...` clean, `go test ./mothership/...` passes** — §7: all three green at HEAD `37d628e` + this bead's changes (7 pre-existing-drift files reformatted mechanically).

---

## 9. Provenance

| Source | Used for | Status at HEAD `37d628e` |
|---|---|---|
| `notes/bf-1q3m-consolidated.md` §1.1 | the two inventory greps | re-run verbatim; 20 named hits + P3 anon-struct = 21 production sites (matches) |
| `notes/bf-1q3m-consolidated.md` §6 | out-of-scope reasons (Tier list + "out of scope" list) | all reasons carried into §4/§5; classifications unchanged |
| `notes/bf-1q3m-consolidated.md` §3 | the 21-site catalogue | all 21 present; minor line drift only (§1) |
| Commits `a612584` / `01a415d` / `c3f4b0d` / `29a114c` / `7309564` | the field additions under test | all in HEAD; supersede task-cited `1446ccf`/`248d0e0` (§6) |
| This report (bf-5151 coverage gate) | the gate that closes bf-5151's "no site missed" criterion | verified against `37d628e`, 2026-07-06 |
