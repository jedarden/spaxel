# Primary Tracked-Blob Go Types — Inventory & Construction Sites (bf-55rp)

> **Scope:** the FIVE primary tracked-blob Go types that make up the tracked-blob
> lifecycle, plus the `api.TrackedBlob` alias. Re-verification of
> `notes/bf-3ldj-findings.md` (§1–3) and `notes/bf-4bhd.md` (Blob Type Definitions +
> Patterns 1–2) against current HEAD.
>
> **Verified against:** `c4d42e8` (`c4d42e8 test(bf-4b1c): differential lock-in for
> geometry placement and fusion peaks`) on **2026-07-06**.
>
> **This bead is documentation only — no Go source was modified.**

---

## 0. TL;DR

| Question | Answer |
|---|---|
| How many primary tracked-blob Go types? | **5** + the `api.TrackedBlob` alias |
| How many DIRECT struct-literal construction sites (production)? | **5** — exactly one per primary type |
| Did anything move since bf-3ldj / bf-4bhd? | **Yes — 2 sites.** `automation.TrackedBlob` `:2213 → :2303`; `signal.TrackedBlob` (`sigproc`) `:5384 → :5494` |
| Did any type definition move? | **No** — all 6 definitions (5 structs + 1 alias) are at their previously reported `file:line` |
| Did any field list drift? | **Yes — `tracking.Blob` (2D).** The 3 identity fields `PersonName`, `AssignedColor`, `IdentityResolved` have been **removed**; the 2D type's identity field set is now identical to the 3D `tracker.Blob`. See §3.1. |
| Is the `api.TrackedBlob` alias still pure? | **Yes** — `type TrackedBlob = signal.TrackedBlob` (no separate struct) |

---

## 1. The six definitions (file:line, verified against HEAD)

| # | Type | Package | Definition site | Kind |
|---|---|---|---|---|
| 1 | `tracking.Blob` (2D, legacy floor tracker) | `mothership/internal/tracking` | `internal/tracking/tracker.go:21` | struct |
| 2 | `tracker.Blob` (3D, identity-bearing) | `mothership/internal/tracker` | `internal/tracker/tracker.go:36` | struct |
| 3 | `fusion.Blob` (peak) | `mothership/internal/fusion` | `internal/fusion/fusion.go:36` | struct |
| 4 | `automation.TrackedBlob` | `mothership/internal/automation` | `internal/automation/engine.go:1337` | struct |
| 5 | `signal.TrackedBlob` | `mothership/internal/signal` | `internal/signal/processor.go:587` | struct |
| 6 | `api.TrackedBlob` (alias of #5) | `mothership/internal/api` | `internal/api/tracks.go:30` | **type alias** |

All six `file:line` entries match bf-4bhd.md / bf-3ldj-findings.md exactly — **none of the
definitions moved.**

### 1.1 The `api.TrackedBlob` alias relationship

```go
// internal/api/tracks.go:29-30
// TrackedBlob is an alias for signal.TrackedBlob.
type TrackedBlob = signal.TrackedBlob
```

This is a **pure Go type alias** (`=`, not a named type wrapping the struct). Consequences:

- `api.TrackedBlob` and `signal.TrackedBlob` are the **same type** — no conversion needed.
- Any field added/removed on `signal.TrackedBlob` (`processor.go:587`) **automatically** surfaces
  under the `api.TrackedBlob` name. No separate edit at `tracks.go:30`.
- The `TracksProvider` interface (`tracks.go:33`) returns `[]signal.TrackedBlob`, which is
  interchangeable with `[]api.TrackedBlob`.

---

## 2. Every DIRECT struct-literal construction site (verified against HEAD)

> "Direct struct-literal" = a `&Blob{}` / `Blob{}` / `TrackedBlob{}` literal that brings a
> new value of one of the five types into existence. Snapshot deep-copies (`out[i] = *b`,
> `make`+`copy`) are **dereferences / slice copies, not literals** — out of scope here and
> correctly excluded from bf-4bhd's Pattern 3 as separate operations.

A whole-repo sweep
(`grep -rn "Blob{\|TrackedBlob{" mothership/`, test files excluded, projection-type
literals filtered out) returns **exactly five** production construction sites — one per
primary type:

| # | Type built | Site (HEAD) | Site in prior reports | Pattern | Built from |
|---|---|---|---|---|---|
| C1 | `fusion.Blob` | `internal/fusion/fusion.go:260` | `:260` ✓ | `Blob{}` value (single line) | fusion grid peak |
| A2 | `tracking.Blob` (2D) | `internal/tracking/tracker.go:160` | `:160` ✓ | `&Blob{}` pointer | unmatched 2D measurement |
| A1 | `tracker.Blob` (3D) | `internal/tracker/tracker.go:162` | `:162` ✓ | `&Blob{}` pointer | unmatched 3D measurement |
| E2 | `automation.TrackedBlob` | `cmd/mothership/main.go:2303` | **`:2213` → moved +90** | `TrackedBlob{}` value | tracked blob (conversion) |
| E1 | `signal.TrackedBlob` (`sigproc`) | `cmd/mothership/main.go:5494` | **`:5384` → moved +110** | `TrackedBlob{}` value | `fusion.Blob` peak (conversion) |

**No other production direct-literal sites exist** for any of the five types. The two `main.go`
sites are the only ones that moved; the two tracker sites and the fusion site are unchanged.

### 2.1 Literal text (verified, verbatim)

**C1 — `fusion.Blob` peak emission** (`fusion/fusion.go:260`), inside `(*Engine).Fuse`:
```go
blobs[i] = Blob{X: p[0], Y: p[1], Z: p[2], Confidence: p[3]}
```
Value-type per grid peak. **No identity** — peaks are pre-identity. This is the raw
measurement that originates every blob downstream.

**A2 — `tracking.Blob` (2D) track spawn** (`tracking/tracker.go:160`), inside `(*Tracker).Update`:
```go
b := &Blob{
    ID:       t.nextID,
    X:        meas[0],
    Z:        meas[1],
    Weight:   meas[2],
    LastSeen: now,
    Trail:    [][2]float64{{meas[0], meas[1]}},
    ukf:      NewUKF(meas[0], meas[1]),
}
```
Identity fields exist on the struct (see §3.1) but are **not set** in the literal — left at
Go zero values, populated later by the BLE-matching layer.

**A1 — `tracker.Blob` (3D) track spawn** (`tracker/tracker.go:162`), inside `(*Tracker).Update`:
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
Identity fields exist on the struct but are **not set** in the literal — populated later by
`applyIdentity` (`tracker/identity.go`, the 3D identity-matching layer).

**E2 — `automation.TrackedBlob` conversion** (`cmd/mothership/main.go:2303`), in the live
10 Hz loop, loop over tracked `blobs`:
```go
autoBlobs[i] = automation.TrackedBlob{
    ID:         b.ID,
    X:          b.X,
    Y:          b.Y,
    Z:          b.Z,
    VX:         b.VX,
    VY:         b.VY,
    VZ:         b.VZ,
    Confidence: b.Weight,
}
```
The source blob **has identity**, but `automation.TrackedBlob` (see §3.4) has **no identity
fields** — identity is **dropped** at this boundary. (This is the long-standing "automation
leak" flagged in bf-3ldj §5 Tier 1; re-confirmed unchanged at HEAD.)

**E1 — `signal.TrackedBlob` (`sigproc`) conversion** (`cmd/mothership/main.go:5494`), inside
`(*blobTracker).track`, built from a `fusion.Blob` peak `pk`:
```go
b := sigproc.TrackedBlob{
    ID:     id,
    X:      pk.X,
    Y:      pk.Y,
    Z:      pk.Z,
    Weight: pk.Confidence,
}
// velocity filled in afterward from bt.prev[id]:
//   b.VX = (pk.X - pb.X) / dt ; b.VY = ... ; b.VZ = ...
```
Built from a **pre-identity peak**, so the `signal.TrackedBlob` identity fields (which do
exist — see §3.5) are left zero here by design.

---

## 3. Field lists (verified against HEAD) + the one drift

### 3.1 `tracking.Blob` (2D) — **field list DRIFTED (correction to bf-4bhd.md)**

`internal/tracking/tracker.go:21`. **Current** struct:
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

> ⚠️ **Correction to `notes/bf-4bhd.md` §"1. tracking.Blob".** The prior report listed
> `PersonName`, `AssignedColor`, `IdentityResolved` as fields of this struct, and the
> literal as initializing `PersonName:""`, `AssignedColor:""`, `IdentityResolved:false`.
> bf-3ldj §1.2 already half-corrected this ("the literal sets none of them"). As of HEAD,
> **those three fields no longer exist on the struct at all** — they have been removed. The
> 2D type's identity field set is now **identical** to the 3D `tracker.Blob`:
> `PersonID / PersonLabel / PersonColor / IdentityConfidence / IdentitySource / IdentityLastSeen`.
> The 2D-vs-3D identity-field divergence noted in bf-4bhd's "Key Findings" has been closed.

### 3.2 `tracker.Blob` (3D, identity-bearing) — unchanged

`internal/tracker/tracker.go:36`. **Current** struct (fields match bf-4bhd; json tags added
since, immaterial to the field set):
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
Identity-matching lives in `tracker/identity.go` (`applyIdentity` / `clearIdentity`),
operating on this 3D type only.

### 3.3 `fusion.Blob` (peak) — unchanged

`internal/fusion/fusion.go:36`:
```go
type Blob struct {
    X, Y, Z    float64 // world-space position (metres)
    Confidence float64 // normalised [0..1]
}
```
**No identity, no velocity, no ID** — the pre-identity raw peak.

### 3.4 `automation.TrackedBlob` — unchanged

`internal/automation/engine.go:1337`:
```go
type TrackedBlob struct {
    ID         int
    X, Y, Z    float64
    VX, VY, VZ float64
    Confidence float64
}
```
**No identity fields.** This is why the E2 conversion (`main.go:2303`) drops identity.

### 3.5 `signal.TrackedBlob` — unchanged

`internal/signal/processor.go:587`:
```go
type TrackedBlob struct {
    ID         int
    X, Y, Z    float64
    VX, VY, VZ float64
    Weight     float64
    // Identity fields
    PersonID           string  `json:"person_id,omitempty"`
    PersonLabel        string  `json:"person_label,omitempty"`
    PersonColor        string  `json:"person_color,omitempty"`
    IdentityConfidence float64 `json:"identity_confidence,omitempty"`
    IdentitySource     string  `json:"identity_source,omitempty"`
    Posture            string  `json:"posture,omitempty"`
}
```
Identity fields **present** (so the type can carry identity), but the E1 conversion builds
from a pre-identity peak and leaves them zero. `api.TrackedBlob` (`tracks.go:30`) is this
exact type under an alias.

---

## 4. Re-location methodology (acceptance criterion)

Moved sites were re-located, exactly as the task prescribes, with:

```bash
grep -rn "Blob{$\|&Blob{$\|TrackedBlob{$" mothership/
```

Result (test files included this time, for completeness):

```
mothership/internal/tracker/tracker.go:162:    b := &Blob{          # A1 (3D, prod)
mothership/cmd/mothership/main.go:2303:       autoBlobs[i] = automation.TrackedBlob{   # E2 (prod, MOVED)
mothership/cmd/mothership/main.go:5494:       b := sigproc.TrackedBlob{                # E1 (prod, MOVED)
mothership/internal/api/tracks_test.go:49:    blobs := []TrackedBlob{                  # TEST (out of scope)
mothership/internal/tracking/tracker.go:160:  b := &Blob{          # A2 (2D, prod)
```

The end-of-line `$` anchor misses the single-line fusion literal, so a second pass with the
broader `grep -rn "Blob{\|TrackedBlob{" mothership/` (filtering projection types and tests)
catches it:

```
mothership/internal/fusion/fusion.go:260: blobs[i] = Blob{X: p[0], Y: p[1], Z: p[2], Confidence: p[3]}   # C1 (prod)
```

**Re-run these two greps before editing any of these sites** — `main.go` is a large,
frequently-edited file and the two conversion lines drift by tens of lines between beads.

---

## 5. Acceptance criteria status

- [x] Each of the 5 primary types has its struct definition recorded with file:line,
      verified against current HEAD — §1 table + §3 verbatim structs.
- [x] Every direct struct-literal construction site listed (file:line + pattern:
      `&Blob{}` / `Blob{}` / `TrackedBlob{}`) — §2 table + §2.1 verbatim literals;
      whole-repo sweep confirms exactly five production sites, no others.
- [x] The `api.TrackedBlob` alias relationship to `signal.TrackedBlob` is noted — §1.1.
- [x] Re-locate moved sites with `grep -rn "Blob{$\|&Blob{$\|TrackedBlob{$" mothership/` —
      §4; two moved sites identified and corrected (`:2213→:2303`, `:5384→:5494`).
- [x] Findings written to `notes/bf-55rp-primary-types.md` — this file.

---

## 6. Net changes since the source reports (the delta this re-verification surfaces)

1. **Two construction sites moved** (no semantic change):
   - `automation.TrackedBlob`: `main.go:2213 → :2303`
   - `signal.TrackedBlob` (`sigproc`): `main.go:5384 → :5494`
2. **One field-list drift** in `tracking.Blob` (2D): `PersonName`, `AssignedColor`,
   `IdentityResolved` removed; identity field set now matches `tracker.Blob` (3D) exactly.
3. **All six type definitions** (5 structs + 1 alias) are at their previously reported
   `file:line`; the alias is still a pure `=` alias.
4. The **automation identity leak** (E2 drops identity because `automation.TrackedBlob`
   has no identity fields) and the **peak-origin zero-identity** of E1 are both unchanged
   and remain the open work items tracked in bf-3ldj §5.

---

## 7. Provenance

| Source report | Used for | Status at HEAD |
|---|---|---|
| `notes/bf-3ldj-findings.md` §1–3 | Type defs, primary literals, identity-flow context | §1 defs valid; one literal line number stale (E2); field-set note for 2D now fully resolved |
| `notes/bf-4bhd.md` (Blob Type Definitions + Patterns 1–2) | Type defs, `&Blob{}`/`Blob{}` patterns, alias note | §1 defs valid; 2D field list stale (§3.1); E2/E1 line numbers stale |
| This report (bf-55rp) | Re-verification + corrections + alias documentation | Verified against `c4d42e8`, 2026-07-06 |
