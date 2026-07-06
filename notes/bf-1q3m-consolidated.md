# Blob Creation & Identity-Flow — Consolidated Source-of-Truth Report (bf-1q3m)

> **Purpose:** the single, re-verified source of truth aggregating the four child inventory
> beads, ready for the **implementation bead** (the identity-leak fix) to consume without
> re-searching. Merges:
> - **bf-55rp** — primary tracked-blob Go types → `notes/bf-55rp-primary-types.md`
> - **bf-5uzm** — cross-package conversion boundaries in `main.go` → `notes/bf-5uzm-conversions.md`
> - **bf-4wwt** — projection / derived types → `notes/bf-4wwt-projections.md`
> - **bf-1bmg** — JS/TS dashboard blob-creation sites → `notes/bf-1bmg-js-ts.md`
>
> **🔴 This report SUPERSEDES `notes/bf-3ldj-findings.md` and `notes/bf-4bhd.md` for the blob
> inventory.** Both older reports drifted against current HEAD (see §4 for the exact stale
> entries). The implementation bead should consume **this file only**; the two superseded
> files are retained for provenance but must not be trusted for `file:line` references.
>
> **Every `file:line` below was re-verified against HEAD `1a26c12`
> (`1a26c1259640d4cff63973319548c1e3a7246e50`) on 2026-07-06 by running the greps**, not by
> trusting the child notes. §1 records the verification method and result. The stale entries
> surfaced by re-verification are flagged in §4. **This is documentation only — no Go/JS
> source was modified.**

---

## 0. TL;DR

| Question | Answer |
|---|---|
| How many blob-shaped **types**? | **15** — 5 primary tracked-blob Go structs + 1 Go type alias (`api.TrackedBlob`) + 9 projection/derived structs + 1 TS interface (`spaxel.d.ts`) |
| How many **production** blob construction sites? | **5 primary** + **16 projection/boundary** + **2 JS production** = **23** (plus 12 JS test fixtures, listed for completeness) |
| Are there `NewBlob`/`makeBlob`/`CreateBlob` helpers? | **No** — every blob is an inline struct/object literal inside its enclosing function |
| Where does identity enter the live pipeline? | **It doesn't enter on the blob struct.** Resolved identity lives in a **sidecar** — `ble.IdentityMatcher` (`main.go:801`), queried per-blob by ID via `GetMatch()` |
| Where does identity **leak** (drop)? | **Two confirmed boundaries:** E2 automation (`main.go:2303`) and E3 explainability (`main.go:2206`); plus a cheap third — volume (`main.go:2326`, field exists, left `""`) |
| What does the implementation bead need to edit? | **See §6 (tiered fix-target list).** Tier 1 = E2 + E3 (+ volume); the fixes thread identity from the sidecar into the target structs |

---

## 1. Re-verification against current HEAD (the acceptance criterion)

### 1.1 Method

The child beads were each verified at *different* commits (bf-55rp @ `c4d42e8`,
bf-5uzm @ `7846408`, bf-4wwt @ `3591865`, bf-1bmg @ `68bd308`). The current HEAD is
`1a26c12`. Because the prior reports drifted before, every `file:line` in this report was
re-confirmed by running the greps against `1a26c12`:

```bash
# Type defs
grep -nE "^type (Blob|TrackedBlob|BlobSnapshot|BlobState|BlobPos|BlobUpdate|BlobEvent|BlobResult|BlobExplanation) " mothership/internal/**/*.go
grep -n "^type TrackedBlob = signal" mothership/internal/api/tracks.go
# Primary literals
grep -rnE "Blob\{|TrackedBlob\{" mothership/ --include=*.go | grep -v "_test.go"
# Projection literals
grep -rnE "BlobSnapshot\{|BlobState\{|BlobPos\{|BlobUpdate\{|BlobEvent\{|BlobResult\{|BlobExplanation\{" mothership/ --include=*.go | grep -v "_test.go"
```

**No code changed between `c4d42e8` and HEAD** — every commit since is `docs`/`.beads` only:

```
$ git diff --name-only c4d42e8 HEAD | grep -vE '^notes/|^.beads/'
(empty — 0 files)
```

So the type definitions and the **named leak boundaries** are all exact at HEAD. The stale
entries found (§4) are all minor off-by-1-to-6 drifts on *secondary citation* sites — none
affect the fix-target list.

### 1.2 Verification result

| Category | Sites checked | Result at HEAD `1a26c12` |
|---|---|---|
| 6 type definitions (5 structs + alias) | 6 | **6/6 exact** |
| 9 projection type definitions | 9 | **9/9 exact** |
| 5 primary construction sites | 5 | **5/5 exact** |
| 4 named conversion boundaries (E1/E2/E3/volume) | 4 | **4/4 exact** |
| 16 projection construction sites (incl. falldetect boundary) | 16 | **16/16 exact** |
| 2 JS production sites | 2 | **2/2 exact** |
| 4 JS `new Blob()` download sites (out of scope) | 4 | **4/4 exact** |
| `spaxel.d.ts` Blob interface (open/close) | 2 | **2/2 exact** |
| Material claims (tracker unwired; no `signal.TrackedBlob` identity writes) | 2 | **2/2 confirmed** |
| Secondary citation sites (sidecar, GetMatch list, anon-struct rows) | ~8 | **4 minor off-by-one drifts — see §4** |

**Completeness sweeps confirm no production site is missed** (§3): the two greps return
exactly 5 primary + 15 projection non-test literals, matching the child counts.

---

## 2. Master type catalogue (15 types)

### 2.1 The five primary tracked-blob Go types + alias (verified at HEAD)

| # | Type | Package | Definition (file:line) | Carries identity? | Kind |
|---|---|---|---|---|---|
| 1 | `tracking.Blob` (2D, legacy) | `internal/tracking` | `internal/tracking/tracker.go:21` | **Yes** — 6 fields (now identical to 3D; see §4 stale #1) | struct |
| 2 | `tracker.Blob` (3D, identity-bearing) | `internal/tracker` | `internal/tracker/tracker.go:36` | **Yes** — 6 fields | struct |
| 3 | `fusion.Blob` (peak) | `internal/fusion` | `internal/fusion/fusion.go:36` | No — `{X,Y,Z,Confidence}` only | struct |
| 4 | `automation.TrackedBlob` | `internal/automation` | `internal/automation/engine.go:1337` | **No** — `{ID,X/Y/Z,VX/VY/VZ,Confidence}` only | struct |
| 5 | `signal.TrackedBlob` | `internal/signal` | `internal/signal/processor.go:587` | **Yes** — 6 fields + `Posture` | struct |
| 6 | `api.TrackedBlob` | `internal/api` | `internal/api/tracks.go:30` | (same as #5) | **type alias** of #5 |

> **Alias note (verified):** `internal/api/tracks.go:30` is a **pure Go type alias**
> (`type TrackedBlob = signal.TrackedBlob`). A field added/removed on `signal.TrackedBlob`
> surfaces automatically under `api.TrackedBlob` — no separate edit. The two names are the
> *same* type.

#### 2.1.1 Field lists (verbatim at HEAD)

**`fusion.Blob`** (`fusion.go:36`) — pre-identity peak, no ID/velocity/identity:
```go
type Blob struct {
    X, Y, Z    float64 // world-space position (metres)
    Confidence float64 // normalised [0..1]
}
```

**`tracker.Blob`** (3D, `tracker/tracker.go:36`) — the identity-bearing 3D type:
```go
type Blob struct {
    ID         int
    X, Y, Z    float64
    VX, VY, VZ float64
    Weight     float64
    Posture    Posture
    LastSeen   time.Time
    Trail      [][3]float64
    // Identity fields
    PersonID           string    `json:"person_id,omitempty"`
    PersonLabel        string    `json:"person_label,omitempty"`
    PersonColor        string    `json:"person_color,omitempty"`
    IdentityConfidence float64   `json:"identity_confidence,omitempty"`
    IdentitySource     string    `json:"identity_source,omitempty"`
    IdentityLastSeen   time.Time `json:"-"`
    ukf *UKF // internal — nil in copies returned to callers
}
```

**`tracking.Blob`** (2D, `tracking/tracker.go:21`) — identity field set now **identical** to
the 3D type (this is the §4 stale-correction to bf-4bhd):
```go
type Blob struct {
    ID       int
    X        float64
    Z        float64
    VX       float64
    VZ       float64
    Weight   float64
    LastSeen time.Time
    Trail    [][2]float64
    ukf      *UKF
    // Identity fields (populated by BLE-to-blob matching)
    PersonID           string    `json:"person_id,omitempty"`
    PersonLabel        string    `json:"person_label,omitempty"`
    PersonColor        string    `json:"person_color,omitempty"`
    IdentityConfidence float64   `json:"identity_confidence,omitempty"`
    IdentitySource     string    `json:"identity_source,omitempty"`
    IdentityLastSeen   time.Time `json:"-"`
    Posture            Posture   `json:"posture,omitempty"`
}
```

**`automation.TrackedBlob`** (`automation/engine.go:1337`) — **NO identity fields** (this is
why the E2 conversion drops identity):
```go
type TrackedBlob struct {
    ID         int
    X, Y, Z    float64
    VX, VY, VZ float64
    Confidence float64
}
```

**`signal.TrackedBlob`** (`signal/processor.go:587`) — identity fields **present** (so the
type *can* carry identity), but the E1 conversion builds from a pre-identity peak and leaves
them zero, and **no live path ever writes them** (§5):
```go
type TrackedBlob struct {
    ID         int
    X, Y, Z    float64
    VX, VY, VZ float64
    Weight     float64
    PersonID           string  `json:"person_id,omitempty"`
    PersonLabel        string  `json:"person_label,omitempty"`
    PersonColor        string  `json:"person_color,omitempty"`
    IdentityConfidence float64 `json:"identity_confidence,omitempty"`
    IdentitySource     string  `json:"identity_source,omitempty"`
    Posture            string  `json:"posture,omitempty"`
}
```

### 2.2 The nine projection / derived types (verified at HEAD)

Read-only views built *from* a tracked blob for a subsystem. They own no UKF / stable session
ID. A field added to a tracked blob that a projection copies **must** be propagated at every
projection construction site (§3.2 + §7 checklist).

| # | Type | Definition (file:line) | Built from | Carries identity? | Notes |
|---|---|---|---|---|---|
| 1 | `explainability.BlobExplanation` | `internal/explainability/handler.go:27` | derived (snapshot + link state) | partial | Rendered "Why?" overlay result; JSON-facing |
| 2 | `explainability.BlobSnapshot` | `internal/explainability/handler.go:95` | tracked blob pos + Weight | **No** | `{ID,X,Y,Z,Confidence,Weight}`. Input to "Why?" overlay |
| 3 | `falldetect.BlobSnapshot` | `internal/falldetect/detector.go:69` | anon struct (see §3.2 note A) | No | Z-trajectory input to fall-detect state machine. **Distinct type** from #2 despite the shared name |
| 4 | `volume.BlobState` | `internal/volume/shape.go:139` | `volume.BlobPos.ID` (see §3.2 note C) | No | Per-blob trigger state: `{BlobID,Inside,EnterTime,LastCheckTime}`. No position |
| 5 | `volume.BlobPos` | `internal/volume/shape.go:1080` | tracked blob pos | **Yes — `PersonID` (present, unpopulated)** | `{ID,PersonID,X,Y,Z}`. Cheapest leak to fix (§6 Tier 1) |
| 6 | `api.BlobPos` | `internal/api/triggers.go:624` | *(none in production)* | No | `{ID,X,Y,Z}`. **Test-only / dead code** — `EvaluateTriggers` is unwired (§3.2 note B) |
| 7 | `tracking.BlobEvent` | `internal/tracking/tracker.go:52` | `tracking.Blob` | No | Lifecycle event: `{BlobID,X,Z,Timestamp}` (2D; X,Z only) |
| 8 | `replay.BlobUpdate` | `internal/replay/types.go:303` | **synthetic** (trig formulas) | (declared but synthetic) | Demo replay frame. Not blob-derived (§3.2 note D) |
| 9 | `simulator.BlobResult` | `internal/simulator/engine.go:80` | **synthetic** (sim fusion grid) | (declared but synthetic) | `spaxel-sim` fixture. Not blob-derived (§3.2 note D) |

---

## 3. Master construction-site catalogue (all production sites, verified at HEAD)

> "Direct struct literal" = a `&Blob{}`/`Blob{}`/`TrackedBlob{}`/projection literal that brings
> a new value into existence. Slice copies / dereferences (`out[i] = *b`, `make`+`copy`) are
> separate copy operations, not creation literals — out of scope (see bf-4bhd Pattern 3).

### 3.1 Primary construction sites — exactly 5 (one per primary type)

| # | Type built | Site (HEAD) | Pattern | Source | Identity at creation? |
|---|---|---|---|---|---|
| C1 | `fusion.Blob` | `internal/fusion/fusion.go:260` | `Blob{}` value (single line) | fusion grid peak | None — pre-identity peak |
| A2 | `tracking.Blob` (2D) | `internal/tracking/tracker.go:160` | `&Blob{}` pointer | unmatched 2D measurement | Fields exist on struct, **not set** (zero) |
| A1 | `tracker.Blob` (3D) | `internal/tracker/tracker.go:162` | `&Blob{}` pointer | unmatched 3D measurement | Fields exist on struct, **not set** (zero) |
| E2 | `automation.TrackedBlob` | `cmd/mothership/main.go:2303` | `TrackedBlob{}` value | `sigproc.TrackedBlob` conversion | ❌ **Target has no identity fields** |
| E1 | `signal.TrackedBlob` (sigproc) | `cmd/mothership/main.go:5494` | `TrackedBlob{}` value | `fusion.Blob` peak conversion | By-design zero (pre-identity peak source) |

**Verbatim literals (HEAD):**

```go
// C1 — fusion/fusion.go:260 (inside (*Engine).Fuse)
blobs[i] = Blob{X: p[0], Y: p[1], Z: p[2], Confidence: p[3]}

// A2 — tracking/tracker.go:160 (inside (*Tracker).Update)
b := &Blob{ID: t.nextID, X: meas[0], Z: meas[1], Weight: meas[2],
    LastSeen: now, Trail: [][2]float64{{meas[0], meas[1]}}, ukf: NewUKF(meas[0], meas[1])}

// A1 — tracker/tracker.go:162 (inside (*Tracker).Update)
b := &Blob{ID: t.nextID, X: m[0], Y: m[1], Z: m[2], Weight: m[3],
    LastSeen: now, Trail: [][3]float64{{m[0], m[1], m[2]}}, Posture: PostureUnknown,
    ukf: NewUKF(m[0], m[1], m[2])}

// E2 — cmd/mothership/main.go:2303 (Stage 6: Trigger evaluation)
autoBlobs[i] = automation.TrackedBlob{
    ID: b.ID, X: b.X, Y: b.Y, Z: b.Z, VX: b.VX, VY: b.VY, VZ: b.VZ, Confidence: b.Weight,
}

// E1 — cmd/mothership/main.go:5494 (inside (*blobTracker).track)
b := sigproc.TrackedBlob{ID: id, X: pk.X, Y: pk.Y, Z: pk.Z, Weight: pk.Confidence}
// then b.VX/VY/VZ derived from bt.prev[id] (main.go:5501-5505)
```

### 3.2 Projection / boundary construction sites — 16 production sites

| # | Site (HEAD) | Type built | Source | Identity? |
|---|---|---|---|---|
| P1 | `cmd/mothership/main.go:2206` | `explainability.BlobSnapshot` | `sigproc.TrackedBlob` | ⚠ dropped (compensated by `identityMap` side-channel, `main.go:2216`) |
| P2 | `cmd/mothership/main.go:2326` | `volume.BlobPos` | `sigproc.TrackedBlob` | ⚠ `PersonID` field exists, left `""` |
| P3 | `cmd/mothership/main.go:2288` | falldetect anon-struct literal | `sigproc.TrackedBlob` | none (anon struct has no identity; `Posture` field also not set) |
| P4 | `internal/explainability/handler.go:194` | `BlobExplanation` | blobID only | empty fallback (unknown ID) |
| P5 | `internal/explainability/handler.go:255` | `BlobExplanation` | blobID only | empty fallback (no history) |
| P6 | `internal/explainability/handler.go:357` | `BlobExplanation` | `BlobSnapshot` | substantive derivation (copies ID/X/Y/Z/Confidence + attaches links/BLE) |
| P7 | `internal/falldetect/detector.go:277` | `falldetect.BlobSnapshot` | anon-struct param | copies ID/X/Y/Z/VX/VY/VZ/Posture/Timestamp; no identity |
| P8 | `internal/replay/pipeline.go:114` | `replay.BlobUpdate` | synthetic (figure-8) | synthetic — not blob-derived |
| P9 | `internal/replay/pipeline.go:132` | `replay.BlobUpdate` | synthetic (circular) | synthetic — not blob-derived |
| P10 | `internal/simulator/engine.go:460` | `simulator.BlobResult` | synthetic (sim grid) | synthetic — not blob-derived |
| P11 | `internal/tracking/tracker.go:173` | `tracking.BlobEvent` | `*tracking.Blob` | copies ID/X/Z (onBlobAppear) |
| P12 | `internal/tracking/tracker.go:188` | `tracking.BlobEvent` | `*tracking.Blob` | copies ID/X/Z (onBlobDisappear) |
| P13 | `internal/volume/shape.go:375` | `volume.BlobState` | persisted int64 timestamps | deserialization — not blob-derived |
| P14 | `internal/volume/shape.go:575` | `volume.BlobState` | `volume.BlobPos.ID` | **only blob-derived BlobState site** (first-seen state) |
| P15 | `internal/volume/shape.go:820` | `volume.BlobState` | sentinel `-999` | in-memory only — not blob-derived |
| P16 | `internal/volume/shape.go:879` | `volume.BlobState` | negative hash key | prediction synthetic state — not blob-derived |

### 3.3 Every `main.go` blob-conversion loop (9, from bf-5uzm) — the live 10 Hz loop projections

All four production blob-derived conversions (P1/P2/P3 + E2) plus the rest of the loop:

| # | main.go:line | Source | Target | Fields copied | Identity? |
|---|---|---|---|---|---|
| L1 | `:2086` | `sigproc.TrackedBlob` | `map[string]interface{}` (detection-event detail) | x,y,z,vx,vy,vz,confidence,posture | person passed **separately** as `personID` arg to `eventsHandler.LogEvent` (`:2100`) — not in the map |
| L2 | `:2113` | `sigproc.TrackedBlob` | anon struct `{ID,X,Y,Z,Weight}` → `identityMatcher.UpdateBlobs` (`:2119`) | ID,X,Y,Z,Weight | n/a — matcher is the identity **producer**, not consumer |
| L3 | **`:2206` (P1/E3)** | `sigproc.TrackedBlob` | `explainability.BlobSnapshot` | ID,X,Y,Z,Confidence | ⚠ dropped (compensated via `identityMap` `:2216`) |
| L4 | `:2271` | `sigproc.TrackedBlob` + sidecar | `analytics.TrackUpdate` | ID,X,Y,Z,VX,VY,VZ,PersonID | ✅ **PersonID populated from sidecar** (`GetMatch`, `:2267`) — **the reference pattern** |
| L5 | `:2288` (P3) | `sigproc.TrackedBlob` | anon struct `{ID,X,Y,Z,VX,VY,VZ,Posture}` → `fallDetector.Update` | ID,X,Y,Z,VX,VY,VZ | `Posture` field declared but not set; no identity |
| L6 | **`:2303` (E2)** | `sigproc.TrackedBlob` | `automation.TrackedBlob` | ID,X/Y/Z,VX/VY/VZ,Confidence | ❌ dropped; target has no identity field |
| L7 | **`:2326` (P2/volume)** | `sigproc.TrackedBlob` | `volume.BlobPos` | ID,X,Y,Z | ⚠ `PersonID` field exists, left `""` |
| L8 | `:2153` | `sigproc.TrackedBlob` | `localization.Vec3` (ground-truth blob position) | X,Y,Z | n/a — pure position vector (no ID) |
| L9 | **`:5494` (E1)** | `fusion.Blob` peak | `sigproc.TrackedBlob` | ID(assoc),X,Y,Z,Weight(+derived VX/VY/VZ) | by-design zero (pre-identity peak) |

> **L4 (`analytics.TrackUpdate`, `:2271`) is the reference implementation for how identity
> *should* be threaded:** pull `PersonID` from `identityMatcher.GetMatch(blob.ID)` at the
> conversion site (`:2267`). **The E2/volume/E3 fixes should mirror this pattern** (§6).

### 3.4 JS production sites — 2

| # | File:line | Enclosing fn | Pattern | Identity set? |
|---|---|---|---|---|
| **J1** | `dashboard/js/state.js:290` | `updateBlob(id, updates)` (opens `:288`) | `appState.blobs[id] = { id: id };` then `Object.assign(...)` at `:292` | **No** — only `{id}`; server-supplied identity arrives via `updates` and is merged by `Object.assign`. This is the **canonical creation point** for every live dashboard blob |
| **J2** | `dashboard/js/websocket.js:167` | `_captureBlobStates()` (opens `:162`) | `_blobStates.set(b.id, {x,z,vx,vz,ts})` | **No** — intentional. Dead-reckoning cache for <5s disconnect extrapolation; derived snapshot, never carries identity |

```js
// J1 — dashboard/js/state.js:288-294
function updateBlob(id, updates) {
    if (!appState.blobs[id]) {
        appState.blobs[id] = { id: id };          // ← :290  creation literal (id only)
    }
    Object.assign(appState.blobs[id], updates);   // ← :292  server payload merged in
    notify('blobs.' + id, appState.blobs[id], null);
    notify('blobs', appState.blobs, null);
}

// J2 — dashboard/js/websocket.js:162-174
_blobStates.set(b.id, { x: b.x, z: b.z, vx: b.vx||0, vz: b.vz||0, ts: Date.now() });  // ← :167
```

### 3.5 Out-of-scope sites (catalogued so the implementer can skip them)

**`new Blob()` browser-API downloads (4)** — file payloads, not domain blobs; ⛔ skip:
`dashboard/static/js/fleet.js:457` (CSV), `dashboard/js/fleet-page.js:1034` (JSON),
`dashboard/js/fleet-page.js:1369` (CSV), `dashboard/js/fleet.js:1997` (JSON).

**JS test fixtures (12)** — `dashboard/js/ambient.test.js` (6: `:124,:273,:636,:653,:688,:702`),
`dashboard/js/quick-actions.test.js` (5: `:208,:316,:470,:513,:678`), `dashboard/js/replay.test.js`
(1: `:101`). Update only if a schema change breaks them.

**Go test-only fixtures** — `fusion/fusion_test.go:631,680,700`, `explainability/handler_test.go:30/31`,
`replay/integration_test.go:578,606`, `api/tracks_test.go:17`, `volume/shape_test.go` (inline).

---

## 4. Stale entries explicitly flagged 🔴

Re-verification against HEAD `1a26c12` surfaced the following stale items. **The implementation
bead must not propagate these.** None of them affect the Tier-1 fix targets (all four named
boundaries are exact at HEAD).

### 4.1 Stale in the SUPERSEDED reports (bf-3ldj, bf-4bhd) — do not use these numbers

| Source | Stale claim | Correct (HEAD `1a26c12`) | Why stale |
|---|---|---|---|
| bf-3ldj §3/§5/§6 | E2 automation leak at `main.go:2213` | **`main.go:2303`** (+90) | code inserted above the loop |
| bf-3ldj §3/§5 | E3 explainability leak at `main.go:2116` | **`main.go:2206`** (+90) | same |
| bf-3ldj §3/§6 | E1 at `main.go:5384` | **`main.go:5494`** (+110) | same |
| bf-3ldj §6 (data-flow) | live path runs `tracker.TrackManager.UpdateWithIdentity` → `applyIdentity` (✅ IDENTITY ATTACHED) | **FALSE** — `tracker.Tracker`/`TrackManager`/`applyIdentity` are **not wired into main.go at all** (zero refs). Live loop uses `blobTracker` (`main.go:2039`/`func :5439`/`track :5447`); identity is never written onto the blob — see §5 | fundamental architecture drift |
| bf-4bhd §1 (Blob Type Defs) | `tracking.Blob` (2D) has fields `PersonName`, `AssignedColor`, `IdentityResolved` | **Removed.** 2D identity set is now identical to 3D: `PersonID/PersonLabel/PersonColor/IdentityConfidence/IdentitySource/IdentityLastSeen` (§2.1.1) | field-list drift |
| bf-4bhd Pattern 1.1 | `tracking.Blob` literal sets `PersonName:"", AssignedColor:"", IdentityResolved:false` | **Wrong** — those fields don't exist; the literal (`tracking/tracker.go:160`) sets **none** of the identity fields (Go zero values) | field-name drift |
| bf-4bhd §4.1/§5 | E2 at `main.go:2213` | **`main.go:2303`** (+90) | code drift |
| bf-4bhd §5.1 | E1 at `main.go:5384` | **`main.go:5494`** (+110) | code drift |
| bf-4bhd projection table | explainability `:2116`, volume `:2236` | `:2206`, `:2326` (+90 each) | code drift |

### 4.2 Minor drift in the CHILD notes (still trustworthy for the named boundaries)

The four child beads are accurate on all named leak boundaries and type definitions. The
following are **secondary citation** off-by-one/to-few drifts found during this re-verification:

| Child note | Cited | Actual (HEAD) | Severity |
|---|---|---|---|
| bf-5uzm §2 row L1 | detection-event detail map at `:2087` | literal `detail := map[string]interface{}{` opens at **`:2086`** (`:2087` is the first field line) | trivial (off-by-1) |
| bf-5uzm §3 | `newBlobTracker` at `main.go:5433` | `func newBlobTracker()` at **`:5439`** (off-by-6); `(*blobTracker).track` at `:5447` is correct | trivial |
| bf-5uzm §3 | `ble.IdentityMatcher` declared at `main.go:799` | `identityMatcher = ble.NewIdentityMatcher(...)` at **`:801`** | trivial (off-by-2) |
| bf-5uzm §3 | GetMatch call sites `main.go:2077,2138,2218,2267,2381` | cited sites are exact **except** `:2381`→**`:2382`** (off-by-1); also **non-exhaustive** — there are 9 total GetMatch calls (`+2359,2416,2471,2486`); the 4 in-loop sites it cites are correct | trivial |
| bf-5uzm §1.3/§3 | `identityMap` range `main.go:2215-2236` | `identityMap := make(...)` at **`:2216`**, `UpdateBlobs(...)` at `:2236` | trivial (start off-by-1) |
| bf-5uzm §1.2 | E2 block labelled "Stage 6 (main.go:2298)" | comment `// Stage 6: Trigger evaluation` at **`:2297`** | trivial (off-by-1) |
| bf-4wwt | re-verified at `3591865`; main.go sites moved +90 vs bf-4bhd | still +90 vs bf-4bhd at HEAD `1a26c12`; all 4wwt sites exact at HEAD | none — confirmed |

> **Bottom line:** the child beads' **named leak boundaries** (E2 `:2303`, E3 `:2206`,
> volume `:2326`, E1 `:5494`) and **all 15 type definitions** are exact at HEAD. Only
> secondary citation lines drifted, and only by 1–6 lines. The fix-target list in §6 is
> unaffected.

---

## 5. The blob data-flow diagram (where identity enters, where it leaks)

> **This diagram corrects bf-3ldj §6**, which drew the live path through
> `tracker.TrackManager.UpdateWithIdentity` → `applyIdentity` and claimed "IDENTITY ATTACHED."
> **That path is not wired into `main.go` at HEAD** (verified: zero references to
> `tracker.NewTracker`/`TrackManager`/`applyIdentity`/`UpdateWithIdentity` in `cmd/mothership/main.go`).
> The live identity path is entirely sidecar-based.

```
fusion.Engine.Fuse ──fusion.Blob peaks──▶  (NO identity — peaks are pre-identity)
   [C1 fusion.go:260]                            │
                                                 ▼
blobTracker.track ──sigproc.TrackedBlob──▶  (6 identity fields EXIST, ALL ZERO)
   [E1 main.go:5494]                             │   ← identity is NEVER written onto the struct
        (blobTracker: main.go:2039; func :5439; track :5447)
                                                 ▼
pm.SetTrackedBlobs(blobs) ──blobs: []sigproc.TrackedBlob──▶  every consumer reads ZERO identity
   [main.go:2057-2058]                           │
                                                 │   resolved identity is held SEPARATELY in a SIDECAR:
                                                 │     ble.IdentityMatcher  (declared main.go:801)
                                                 │     queried per-blob by ID via GetMatch(blob.ID)
                                                 │     e.g. main.go:2077, 2138, 2218, 2267 (+2359,2382,...)
                                                 │   It is NEVER written back onto the blob struct
                                                 │   (verified: zero non-test writes to signal.TrackedBlob identity fields)
                                                 ▼
   ┌──────────────┬───────────────┬───────────────────┬────────────────┬──────────────────┐
   ▼              ▼               ▼                   ▼                ▼                  ▼
 analytics      explainability   automation          volume           identityMatcher    fall-detect
 [L4 :2271]     [L3 :2206 E3]    [L6 :2303 E2]       [L7 :2326]       [L2 :2113]         [L5 :2288]
 ✅ pulls       ⚠ snapshot       ❌ target has       ⚠ PersonID       (sidecar: the       (anon struct;
   PersonID        drops person     NO identity         field EXISTS     identity SOURCE)   Posture field
   from sidecar    BUT identity     field at all;       but left ""      — not a leak,      also not set;
   → CORRECT      Map side-        personProvider      — cheapest        this is where      no identity)
   (reference      channel          can never fire      to fix (1 line)   identity is made)
   pattern)        (:2216) partly   (needs a PersonID
   — MIRROR THIS   compensates      nothing supplies)
```

**The leak in one sentence:** in the live path, identity lives only in the `ble.IdentityMatcher`
sidecar (keyed by `blob.ID`); the tracked blob struct (`sigproc.TrackedBlob`) carries the
identity *fields* but they are permanently zero. So "identity dropped at the conversion"
really means **"the conversion is the place identity could be attached from the sidecar, and
isn't."** Every identity-aware boundary must call `identityMatcher.GetMatch(blob.ID)` at the
conversion site — exactly as `analytics.TrackUpdate` (`L4 :2271`) already does.

---

## 6. Tiered fix-target list ⭐ (the deliverable the implementation bead consumes)

Ranked by impact. The Tier-1 set is the **identity leak this effort closes.**

### Tier 1 — Identity DROPPED at a live conversion boundary (must add fields + populate from sidecar)

| # | Site (HEAD) | Boundary | Target type (file:line) | What's wrong | Concrete fix |
|---|---|---|---|---|---|
| **1** | `cmd/mothership/main.go:2303` | `sigproc.TrackedBlob` → `automation.TrackedBlob` | `automation.TrackedBlob` (`internal/automation/engine.go:1337`) | Target type has **no identity fields**; `automationEngine.personProvider` (`SetPersonProvider` `:3147`) needs a `PersonID` nothing supplies → **person-aware automations ("when Alice enters…") are structurally blocked** | (a) Add identity fields to `automation.TrackedBlob` (`PersonID` minimum; `PersonLabel`/`PersonColor` if the engine renders); (b) populate them from `identityMatcher.GetMatch(b.ID)` in the loop (`:2303`), mirroring `analytics.TrackUpdate` (`:2267-2271`); (c) thread the `PersonID` into `Evaluate` (`:2314`) so `personProvider` (`engine.go:952`) can resolve label/color |
| **2** | `cmd/mothership/main.go:2206` | `sigproc.TrackedBlob` → `explainability.BlobSnapshot` | `explainability.BlobSnapshot` (`internal/explainability/handler.go:95`) | Target type has **no identity fields**; person is currently smuggled in via a parallel `identityMap` (`main.go:2216`) — works for the "Why?" overlay but is a dual source of truth | (a) Add identity fields to `BlobSnapshot`; (b) populate from sidecar at `:2206`; (c) fold the parallel `identityMap` (`:2216`) into the snapshot so the overlay has **one** identity source (then drop the side-channel arg from `UpdateBlobs` `:2236`) |
| **3** | `cmd/mothership/main.go:2326` | `sigproc.TrackedBlob` → `volume.BlobPos` | `volume.BlobPos` (`internal/volume/shape.go:1080`) | `PersonID` field **already exists** but is left `""` | **One-line fix:** `PersonID: <from identityMatcher.GetMatch(blob.ID)>` — unblocks person-filtered volume triggers (`condition_params.person`) |

> **Why Tier-1 = #1+#2 (+ optionally #3):** without #1 and #2, person-aware automations and
> person-labeled explainability cannot work even when BLE identity is fully resolved upstream.
> #3 is bundled because the field already exists and the fix is a single line — do it in the
> same bead while touching the loop.

### Tier 2 — Identity-machinery present but unwired (architectural decision, not a literal edit)

| # | Site (HEAD) | What | Action |
|---|---|---|---|
| 4 | `internal/tracker/identity.go:164` (`applyIdentity`) / `:179` (`clearIdentity`) | The `tracker.Blob` (3D) identity machinery exists and is correct, but `tracker.TrackManager` is **not wired into `main.go`** — it never runs in the live loop | Decide the architecture: either (a) leave the sidecar as the single identity source (current design — then Tier 1 fixes are the whole job), or (b) wire `tracker.TrackManager` into the live loop so identity is attached to the blob struct once and flows automatically. **Recommendation: (a)** — the sidecar is already correct and `analytics.TrackUpdate` proves the pattern works; wiring the 3D tracker is a larger change with its own risk. Document the decision in the implementation bead |
| 5 | `internal/tracker/tracker.go:162` (A1) / `internal/tracking/tracker.go:160` (A2) | `tracker.Blob`/`tracking.Blob` identity fields exist but the spawn literals leave them zero (populated later by `applyIdentity` — which only runs if #4 is wired) | No literal change needed unless #4 option (b) is chosen |

### Tier 3 — Identity-carrying type but built from a pre-identity source (no-op by design)

| # | Site (HEAD) | Type | Why no identity | Action |
|---|---|---|---|---|
| 6 | `cmd/mothership/main.go:5494` (E1) | `signal.TrackedBlob` | Built from a `fusion.Blob` **peak** (peaks have no identity); identity would attach *after* this in a tracker-driven design | **None** under Tier-2 option (a) (sidecar design). If option (b), identity reattaches from the tracker state, not from E1 |

### Tier 4 — JS / dashboard identity plumbing

| # | File:line | What | Action |
|---|---|---|---|
| 7 | `dashboard/types/spaxel.d.ts:10–91` | `Blob` interface — canonical JS identity-field declaration (`personName`,`personId`,`assignedColor`,`personColor`[deprecated],`personLabel`[deprecated],`identityResolved`) | If the JS schema changes, update here — **single declaration point**, no JS file redeclares it |
| 8 | `dashboard/js/state.js:290` (J1) | Blob creation literal is `{ id: id }` only | If pre-initializing identity at creation is desired, add fields here. Currently relies on server payload via `Object.assign` (`:292`) — **no change needed** if the server sends identity fields (which Tier-1 #1/#2 unblock on the Go side) |
| 9 | `dashboard/js/ambient_renderer.js` (identity-field **consumption**) | Fallback chain `personName → person_label → person` | Update the fallback chain if field names change; this **reads**, doesn't create |

### Out of scope for identity work (no identity by design — do not touch)

- `fusion/fusion.go:260` (C1 — pre-identity peak)
- `simulator/engine.go:460`, `replay/pipeline.go:114,132` (synthetic/demo)
- `explainability/handler.go:194,255` (empty fallbacks for unknown IDs)
- `falldetect/detector.go:277` (history snapshot — posture/velocity only; promote to Tier 1 only if fall alerts need a person name, which per `plan.md` Component 16 they do — **flagged for a future bead**, not this one)
- `volume/shape.go:375,820,879` (non-blob-derived BlobState sites), `tracking/tracker.go:173,188` (BlobEvent lifecycle)
- `api.BlobPos` (`api/triggers.go:624`) — **dead code**, `EvaluateTriggers` unwired in `main.go`; do not rely on it for field propagation (it's a vestigial duplicate of `volume.BlobPos`)
- All test files and all four `new Blob()` download sites (§3.5)

---

## 7. Field-propagation checklist (when adding a field to `signal.TrackedBlob`)

Production sites in the live loop that copy tracked-blob fields and would need updating. The
implementer should re-locate each with `grep` first (`main.go` drifts):

```bash
grep -nE "sigproc.TrackedBlob{|automation.TrackedBlob{|explainability.BlobSnapshot{|volume.BlobPos{" mothership/cmd/mothership/main.go
```

1. `cmd/mothership/main.go:2206` — `explainability.BlobSnapshot` (Tier-1 #2)
2. `cmd/mothership/main.go:2288` — falldetect anon struct (+ the `detector.go` anon-struct signatures at `:249,270,349,417,480,504`)
3. `cmd/mothership/main.go:2326` — `volume.BlobPos` (Tier-1 #3)
4. `cmd/mothership/main.go:2303` — `automation.TrackedBlob` (Tier-1 #1) + the `automation.TrackedBlob` struct def at `engine.go:1337`
5. (Reference pattern to mirror: `cmd/mothership/main.go:2271` — `analytics.TrackUpdate`)

> Note: `tracking.Blob` field changes → also update `tracking.BlobEvent` at `tracker.go:173,188`.
> `tracker.Blob` (3D) field changes → no projection in the live set copies from it directly
> (the snapshots/falldetect are fed from `signal.TrackedBlob`, not `tracker.Blob`, at the
> production boundary).

---

## 8. Acceptance criteria status

- [x] **Single consolidated report exists at `notes/bf-1q3m-consolidated.md`** — this file.
- [x] **Every file:line in the catalogue re-verified against current HEAD (greps run)** — §1
  records the method; the type defs (§2), primary sites (§3.1), projection sites (§3.2),
  main.go loops (§3.3), and JS sites (§3.4) are all verified at `1a26c12`. Completeness
  sweeps (§1.2) confirm no production site is missed.
- [x] **Stale entries from sub-bead notes explicitly flagged** — §4.1 (superseded reports:
  bf-3ldj/bf-4bhd line + field-list + data-flow drift) and §4.2 (minor child-note drifts).
- [x] **Tiered fix-target list for the identity leak produced** — §6 (Tier 1 = `:2303`
  automation + `:2206` explainability + `:2326` volume; Tiers 2–4 follow).
- [x] **Report explicitly states it supersedes `notes/bf-3ldj-findings.md` and `notes/bf-4bhd.md`
  for the inventory** — header banner + §4.1.
- [x] **Report is ready for the implementation bead to consume without re-searching** — §5
  data-flow, §6 tiered targets, §7 propagation checklist, and the §3.3 reference pattern
  (`analytics.TrackUpdate` `:2271`) give the implementer exact sites + the population pattern.

---

## 9. Provenance

| Source | Used for | Status at HEAD `1a26c12` |
|---|---|---|
| `notes/bf-55rp-primary-types.md` | 5 primary types + alias + primary construction sites | type defs + named boundaries exact; verified |
| `notes/bf-5uzm-conversions.md` | main.go conversion boundaries + data-flow correction | named boundaries exact; §4.2 flags minor secondary-citation drift |
| `notes/bf-4wwt-projections.md` | 9 projection types + 16 construction sites | all exact at HEAD; confirmed |
| `notes/bf-1bmg-js-ts.md` | 2 JS production sites + spaxel.d.ts + 4 `new Blob()` downloads | all exact at HEAD; confirmed |
| `notes/bf-3ldj-findings.md` | prior consolidated inventory | **SUPERSEDED** — §4.1 (line + data-flow drift) |
| `notes/bf-4bhd.md` | prior inventory + projection table | **SUPERSEDED** — §4.1 (2D field-list + line drift) |
| This report (bf-1q3m) | Consolidation + re-verification + tiered fix-target list + data-flow | Verified against `1a26c12`, 2026-07-06 |
