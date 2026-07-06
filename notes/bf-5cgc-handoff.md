# Blob-Creation → Identity-Field Implementation Handoff (bf-5cgc)

> **Purpose:** formalize `notes/bf-1q3m-consolidated.md` §6 (tiered fix-target list) into a
> **tracked deliverable that links each blob-creation site to the OPEN implementation bead that
> owns it**, plus the exact population pattern to copy. This is what closes bf-4bhd's acceptance
> criterion *"Report is ready for the next bead to use"* and hands the work to bf-5151 and friends.
>
> **Consumes (do not re-search):** `notes/bf-1q3m-consolidated.md` — the sole trusted blob
> inventory. `notes/bf-4bhd.md` and `notes/bf-3ldj-findings.md` are SUPERSEDED (see bf-1q3m §4).
>
> **Verification stamp:** every `file:line` below was re-confirmed against current HEAD
> `1ecc999` (`1ecc9992d184c14b143a1ccb0d0cc932fdec388b`) on 2026-07-06 by running the greps.
> No Go/JS source moved between bf-1q3m's verification commit (`1a26c12`) and HEAD
> (`git diff --stat 1a26c12 HEAD -- mothership/ dashboard/` is empty — only `docs`/`.beads`
> commits since), so all four named Tier-1 boundaries are exact. Re-locate any moved site with:
> `grep -nE "sigproc.TrackedBlob{|automation.TrackedBlob{|explainability.BlobSnapshot{|volume.BlobPos{" mothership/cmd/mothership/main.go`

---

## 0. TL;DR — the handoff in one table

| # | Leak site (HEAD `1ecc999`) | Target type (struct def) | Fix | Owner bead(s) |
|---|---|---|---|---|
| **1** | `cmd/mothership/main.go:2303` | `automation.TrackedBlob` (`internal/automation/engine.go:1337`) | add identity fields (min `PersonID`); populate from `identityMatcher.GetMatch(b.ID)`; engine already resolves label/color via `personProvider` (`engine.go:952`) | **bf-5151** / **bf-64h5** |
| **2** | `cmd/mothership/main.go:2206` | `explainability.BlobSnapshot` (`internal/explainability/handler.go:95`) | add identity fields; populate from sidecar; **fold the parallel `identityMap`** (`main.go:2216`) so the "Why?" overlay has one identity source (then drop the side-channel arg from `UpdateBlobs` `:2236`) | **bf-5151** |
| **3** | `cmd/mothership/main.go:2326` | `volume.BlobPos` (`internal/volume/shape.go:1080`) | **one-line populate** — `PersonID` field *already exists* (left `""`); reading it is already stubbed at `shape.go:624` | **bf-5151** |

> **Reference pattern to mirror for all three:** `analytics.TrackUpdate`
> (`cmd/mothership/main.go:2271`) pulls `PersonID` from the sidecar via
> `identityMatcher.GetMatch(blob.ID)` at the conversion site (`main.go:2267`). Copy that shape.

---

## 1. The reference population pattern (copy this)

`cmd/mothership/main.go:2264-2280` — the **only** live boundary today that correctly threads
identity from the sidecar into a target struct. Every Tier-1 fix should look like this:

```go
// Stage 4: Update flow analytics — main.go:2262
for _, blob := range blobs {
    // Get person ID from identity matcher           ← :2264
    var personID string
    if identityMatcher != nil {                      ← :2266
        if match := identityMatcher.GetMatch(blob.ID); match != nil {   ← :2267  THE CALL
            personID = match.PersonID                ← :2268
        }
    }
    flowAccumulator.UpdateTrack(analytics.TrackUpdate{   ← :2271  THE TARGET STRUCT
        ID:       blob.ID,
        X:        blob.X,
        Y:        blob.Y,
        Z:        blob.Z,
        VX:       blob.VX,
        VY:       blob.VY,
        VZ:       blob.VZ,
        PersonID: personID,                          ← :2279  IDENTITY THREADED IN
    })
}
```

The three Tier-1 sites are all in the **same 10 Hz loop**, immediately adjacent to this block:
`explainability` (Stage 2, `:2206`), `automation` (Stage 6, `:2303`), `volume` (Stage 6, `:2326`).
The `identityMatcher` is in scope for all of them. So each fix is: declare a `personID` (and, for
automation/explainability, the other identity fields) the same way, then set them on the target
struct literal.

### 1.1 What `GetMatch` returns (the sidecar fields available to populate from)

`ble.IdentityMatcher.GetMatch(blobID int) *IdentityMatch` (`internal/ble/identity.go:609`).
`IdentityMatch` (`internal/ble/identity.go:34`) carries:

| Field | Type | Use for |
|---|---|---|
| `PersonID` | `string` | the stable person key — **always populate this** (drives `personProvider.GetPerson`) |
| `PersonName` | `string` | resolved label (e.g. "Alice") |
| `PersonColor` | `string` | CSS hex color |
| `DeviceAddr` | `string` | matched BLE device address |
| `Confidence` | `float64` | match confidence [0..1] |
| `TriangulationPos` | `Position{X,Y,Z}` | BLE RSSI centroid (used by explainability overlay) |

> Note the field-name mapping the existing code already uses (main.go:2221-2230): the
> explainability `identityMap` copies `match.PersonName → PersonLabel`. Keep that mapping
> consistent when folding #2.

---

## 2. Per-site concrete fix (what the implementer edits)

### Tier-1 #1 — automation (`main.go:2303` → bf-5151 / bf-64h5)

**What's wrong:** `automation.TrackedBlob` (`internal/automation/engine.go:1337`) is
`{ID, X/Y/Z, VX/VY/VZ, Confidence}` — **no identity fields**. The conversion at `main.go:2303`
therefore cannot carry a person. Downstream, `automationEngine.personProvider`
(`SetPersonProvider`, `engine.go:474`) needs a `PersonID` to resolve label/color
(`engine.go:952-953`: `data.PersonName, data.PersonColor, _ = e.personProvider.GetPerson(data.PersonID)`),
but nothing supplies one → **person-aware automations ("when Alice enters…") are structurally blocked.**

**Fix (three parts):**
1. **Add identity fields** to `automation.TrackedBlob` (`engine.go:1337`). Minimum: `PersonID string`.
   Add `PersonLabel`/`PersonColor` only if the engine itself renders them (it doesn't today — it
   resolves them via `personProvider` at `:952`/`:1165`, so `PersonID` alone is sufficient for
   label/color; add the others only if a webhook payload or event needs them pre-resolved).
2. **Populate** at the conversion site (`main.go:2303`), mirroring `analytics.TrackUpdate`:
   ```go
   var personID string
   if identityMatcher != nil {
       if m := identityMatcher.GetMatch(b.ID); m != nil {
           personID = m.PersonID
       }
   }
   autoBlobs[i] = automation.TrackedBlob{
       ID: b.ID, X: b.X, Y: b.Y, Z: b.Z, VX: b.VX, VY: b.VY, VZ: b.VZ,
       Confidence: b.Weight,
       PersonID: personID,            // ← new
   }
   ```
3. **Verify the `PersonID` reaches `Evaluate`** (`engine.go:1346`) so `personProvider` (`:952`)
   can resolve it. (The engine already does `data.PersonName == "" && data.PersonID != ""` →
   `personProvider.GetPerson(data.PersonID)`; confirm the `TrackedBlob → data` copy in `Evaluate`
   includes the new field.)

### Tier-1 #2 — explainability (`main.go:2206` → bf-5151)

**What's wrong:** `explainability.BlobSnapshot` (`internal/explainability/handler.go:95`) is
`{ID, X, Y, Z, Confidence}` — **no identity fields**. The person is currently smuggled into the
"Why?" overlay via a **parallel `identityMap`** built separately at `main.go:2216-2232` and passed
as a 4th arg to `explainabilityHandler.UpdateBlobs(blobSnapshots, linkStates, gridSnapshot, identityMap)`
(`main.go:2236`). This works today but is a **dual source of truth** (snapshot + side-channel map).

**Fix:**
1. **Add identity fields** to `BlobSnapshot` (`handler.go:95`). Mirror the `explainability.BLEMatch`
   shape already in `handler.go:53` (PersonID/PersonLabel/PersonColor/DeviceAddr/Confidence/
   MatchMethod/TriangulationPos) — or embed `*BLEMatch` directly.
2. **Populate** at `main.go:2206` from the sidecar (same `GetMatch` call the `identityMap` loop
   already makes at `:2218`), so the snapshot carries identity directly.
3. **Fold** the parallel `identityMap` (`:2216`) into the snapshot, then **drop** the `identityMap`
   arg from `UpdateBlobs` (`:2236`) and update `UpdateBlobs`'s signature/`BuildWebSocketSnapshot`
   to read identity off the snapshot instead of the map. **Goal:** one identity source per blob.

> This is the largest of the three (signature change to `UpdateBlobs` + handler reads). If
> bf-5151 wants to bound scope, #1+#3 (automation + volume) can ship first and #2 follow — but
> all three live in the same loop and share the same `GetMatch` call, so doing them together is
> cheapest.

### Tier-1 #3 — volume (`main.go:2326` → bf-5151)

**What's wrong:** `volume.BlobPos` (`internal/volume/shape.go:1080`) **already has**
`PersonID string` (line 1082) — but the conversion at `main.go:2326` leaves it `""`. The consumer
is already waiting on it: `shape.go:623-624` reads `t.ConditionParams.PersonID` and currently has
`_ = blob.PersonID // Placeholder for person filter implementation`.

**Fix (one line):**
```go
var personID string
if identityMatcher != nil {
    if m := identityMatcher.GetMatch(blob.ID); m != nil {
        personID = m.PersonID
    }
}
volumeBlobs[i] = volume.BlobPos{
    ID: blob.ID, X: blob.X, Y: blob.Y, Z: blob.Z,
    PersonID: personID,   // ← the one new line
}
```
This **unblocks person-filtered volume triggers** (`condition_params.person`).

---

## 3. Scope note ⚠️ — Go-backend fix vs. TS-framed beads

The implementation beads as written are **TypeScript / dashboard-frontend-framed** (they mention
"frontend", "TypeScript types", "console errors", "3D dashboard frontend"):
- **bf-64h5** — *"Add identity-related fields to the blob/person data structure in the … 3D
  dashboard frontend … Add TypeScript types/interfaces."*
- **bf-1wvm** — *"Update code that creates blob objects … No TypeScript errors."*
- **bf-iner** — *"frontend logic to match BLE registry identities to blob IDs."*
- **bf-4qto / bf-56uk / bf-f841** — TS defaults / TS compliance / runtime testing.

**But the three Tier-1 leak sites above are all Go backend** (`cmd/mothership/main.go` +
`internal/{automation,explainability,volume}`). **bf-5151** (*"Add identity fields to blob
creation code … Use the list of blob creation sites from previous bead"*) is the bead that maps
to the **Go** Tier-1 work — that is where this handoff is primarily directed.

**Mapping the consolidated report's tiers to the bead set:**

| Consolidated-report tier | Bead(s) | Layer |
|---|---|---|
| **Tier 1** (Go leaks: automation `:2303`, explainability `:2206`, volume `:2326`) | **bf-5151** (primary; automation also bf-64h5) | Go backend |
| **Tier 4 #7** (`dashboard/types/spaxel.d.ts:10-91` — canonical JS `Blob` interface) | **bf-64h5** / **bf-1wvm** | dashboard TS types |
| **Tier 4 #8/#9** (`state.js:290` creation; `ambient_renderer.js` consumption) | **bf-1wvm** / **bf-56uk** / **bf-f841** | dashboard JS |
| **BLE matching** (the sidecar itself — already implemented server-side in `internal/ble`) | **bf-iner** (frontend mirror of the match) | dashboard JS |

**Recommendation for whoever picks up bf-5151:** treat its scope as *"add identity fields to the
**Go** blob-creation/conversion sites in §0 above"* — i.e. this is a **backend** task despite the
sibling beads' TS framing. The frontend identity plumbing (bf-64h5/bf-1wvm/bf-iner) is a separate
concern and only becomes useful once the Go side actually emits identity fields to the dashboard
(which Tier-1 #1/#2 unblock). If the team prefers a clean split, **re-scope bf-5151 to "Go blob
identity fields"** and leave the TS bead titles as the frontend track.

---

## 4. Tier 2 / Tier 3 / out-of-scope (for completeness — listed so nothing is silently dropped)

From `notes/bf-1q3m-consolidated.md` §6. The implementer should be aware of these but they are
**not** part of the Tier-1 handoff above.

### Tier 2 — identity machinery present but unwired (architectural decision, not a literal edit)

| # | Site | Situation | Action |
|---|---|---|---|
| 4 | `internal/tracker/identity.go:164` (`applyIdentity`) / `:179` (`clearIdentity`) | The `tracker.Blob` (3D) identity machinery exists and is correct, but `tracker.TrackManager` is **not wired into `main.go`** (zero refs) — it never runs in the live loop | **Decision to record in the implementation bead:** (a) leave the `ble.IdentityMatcher` sidecar as the single identity source (current design — then Tier 1 is the whole job), **or** (b) wire `tracker.TrackManager` into the live loop so identity attaches to the blob struct once. **Recommendation: (a)** — the sidecar already works and `analytics.TrackUpdate` proves the pattern; wiring the 3D tracker is a larger, riskier change. |
| 5 | `internal/tracker/tracker.go:162` (A1) / `internal/tracking/tracker.go:160` (A2) | `tracker.Blob`/`tracking.Blob` identity fields exist but the spawn literals leave them zero (populated later by `applyIdentity` — which only runs if #4 is wired) | No literal change unless Tier-2 option (b) is chosen |

### Tier 3 — identity-carrying type but built from a pre-identity source (no-op by design)

| # | Site | Why no identity | Action |
|---|---|---|---|
| 6 | `cmd/mothership/main.go:5494` (E1) — `signal.TrackedBlob` built from a `fusion.Blob` **peak** | Peaks are pre-identity; identity would attach *after* this in a tracker-driven design | **None** under Tier-2 option (a) (sidecar design). Under option (b), identity reattaches from tracker state, not from E1. |

### Out of scope for identity work (no identity by design — do not touch)

- `internal/fusion/fusion.go:260` (C1 — pre-identity peak)
- `internal/simulator/engine.go:460`, `internal/replay/pipeline.go:114,132` (synthetic/demo)
- `internal/explainability/handler.go:194,255` (empty fallbacks for unknown IDs)
- `internal/falldetect/detector.go:277` (history snapshot — posture/velocity only; promote to
  Tier 1 **only** if fall alerts need a person name, which per `plan.md` Component 16 they do —
  **flagged for a future bead**, not this handoff)
- `internal/volume/shape.go:375,820,879` (non-blob-derived `BlobState` sites),
  `internal/tracking/tracker.go:173,188` (`BlobEvent` lifecycle)
- **`api.BlobPos` (`internal/api/triggers.go:624`) — DEAD CODE.** `EvaluateTriggers` is unwired
  in `main.go` (the *live* volume trigger path goes through `volumeTriggersHandler.EvaluateTriggers`
  at `main.go:2333`, which takes `volume.BlobPos`, not `api.BlobPos`). `api.BlobPos` is a vestigial
  duplicate of `volume.BlobPos`; **do not rely on it for field propagation.**
- All test files and all four `new Blob()` browser-API download sites (bf-1q3m §3.5)

---

## 5. Field-propagation checklist (when touching `signal.TrackedBlob` or its conversions)

If the implementer changes the field set on any tracked-blob type, these production copy sites in
the live loop must be kept in sync (re-locate with grep first — `main.go` drifts):

1. `cmd/mothership/main.go:2206` — `explainability.BlobSnapshot` (Tier-1 #2)
2. `cmd/mothership/main.go:2288` — falldetect anon struct (+ `detector.go` anon-struct signatures)
3. `cmd/mothership/main.go:2326` — `volume.BlobPos` (Tier-1 #3)
4. `cmd/mothership/main.go:2303` — `automation.TrackedBlob` (Tier-1 #1) + struct def `engine.go:1337`
5. (Reference pattern to mirror: `cmd/mothership/main.go:2271` — `analytics.TrackUpdate`)

---

## 6. Acceptance criteria status

**bf-5cgc (this bead):**
- [x] Handoff artefact maps each Tier-1 site to the responsible open implementation bead + the
  population pattern — §0 + §2 (automation→bf-5151/bf-64h5, explainability→bf-5151,
  volume→bf-5151; reference pattern §1; GetMatch fields §1.1)
- [x] Tier 2 / Tier 3 / out-of-scope sites listed for completeness (tracker wiring decision,
  E1 no-op, dead `api.BlobPos`) — §4
- [x] Closes bf-4bhd acceptance criterion *"Report is ready for the next bead to use"* — this
  note is the formalized, bead-linked deliverable that bf-4bhd's inventory was building toward
  (bf-4bhd's own acceptance box is ticked; this handoff makes it *actionable* by naming the owners)
- [x] Comments posted on the linked implementation beads (bf-5151, bf-64h5, bf-1wvm, bf-iner,
  bf-4bhd) pointing here

**bf-4bhd (parent inventory bead):** its criterion *"Report is ready for the next bead to use"*
is satisfied by `notes/bf-1q3m-consolidated.md` (the re-verified inventory) **plus** this note
(the bead-linked handoff). bf-4bhd may be closed on that basis.

---

## 7. Provenance

| Source | Used for | Status at HEAD `1ecc999` |
|---|---|---|
| `notes/bf-1q3m-consolidated.md` §6 | tiered fix-target list (the input to this handoff) | verified exact at `1a26c12`; re-confirmed exact at `1ecc999` (no code drift) |
| `notes/bf-1q3m-consolidated.md` §5 | data-flow / leak explanation | confirmed |
| `cmd/mothership/main.go:2206,2216,2267,2271,2303,2326` | Tier-1 sites + reference pattern | all 6 exact at `1ecc999` |
| `internal/automation/engine.go:1337,952,1346` | TrackedBlob def + personProvider + Evaluate | exact |
| `internal/explainability/handler.go:53,95` | BlobSnapshot + BLEMatch defs | exact |
| `internal/volume/shape.go:624,1080` | BlobPos def + person-filter stub | exact |
| `internal/ble/identity.go:34,609` | IdentityMatch + GetMatch | exact |
| This note (bf-5cgc) | formalized site→bead handoff + population pattern | verified at `1ecc999`, 2026-07-06 |
