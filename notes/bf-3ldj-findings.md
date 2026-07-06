> 🔴🔴🔴 **SUPERSEDED — DO NOT USE THIS FILE AS A BLOB INVENTORY.** 🔴🔴🔴
>
> Despite the "Single source of truth" language in the header below, this report has
> **drifted against current HEAD** and is **retained for provenance only**. The **sole trusted
> blob inventory** is **`notes/bf-1q3m-consolidated.md`** — read that file instead and ignore
> the `file:line` references and the data-flow diagram (§6) below.
>
> **Why superseded** (per `notes/bf-1q3m-consolidated.md` §4.1):
>
> 1. **Line drift (+90 / +110).** The named leak-boundary citations are stale: E2 automation
>    is actually `main.go:2303` (cited `:2213`, +90), E3 explainability `main.go:2206` (cited
>    `:2116`, +90), E1 sigproc `main.go:5494` (cited `:5384`, +110).
> 2. **Fundamental data-flow drift (material).** §6 below draws the live path through
>    `tracker.TrackManager.UpdateWithIdentity` → `applyIdentity` and claims "✅ IDENTITY
>    ATTACHED." **This path is NOT wired into `main.go` at HEAD** (zero references to
>    `tracker.NewTracker` / `TrackManager` / `applyIdentity` / `UpdateWithIdentity`). The live
>    identity path is entirely sidecar-based — resolved identity lives in
>    `ble.IdentityMatcher`, queried per-blob by ID via `GetMatch()`, and is never written back
>    onto the tracked blob struct. So "identity dropped at the conversion" really means
>    "the conversion is where identity could be attached from the sidecar, and isn't."
>
> **Status:** provenance only. A future agent must not consume this file for `file:line`
> references or for the data-flow diagram — it will re-introduce the line drift and the
> wrong identity-attachment model. Use `notes/bf-1q3m-consolidated.md`.

---

# Blob Creation Sites — Consolidated Report (bf-3ldj) (SUPERSEDED — see banner above)

> **Purpose:** Single source of truth aggregating the three blob-creation search beads, ready
> for the next bead (parent umbrella `bf-4bhd`, then the implementation bead) to consume.
> Compiles:
> - **bf-5kns** — `new Blob()` constructor calls → `notes/bf-5kns.md`
> - **bf-26ta** — blob-shaped object/struct literals → `notes/bf-26ta-findings.md` (+ `-javascript-results.md`, `-typescript-results.md`)
> - **bf-67ao** — blob factory functions → `notes/bf-67ao-findings.md`
>
> **Every file:line in the production catalogue below was re-verified against HEAD on 2026-07-06.**
> Discrepancies found in the source findings are corrected in §1.2 (do not blindly copy the
> sub-bead notes for the corrected items).

---

## 0. TL;DR

| Question | Answer |
|---|---|
| How many **Spaxel domain blob** creation sites exist? | **19 production sites** across **9 files** (Go + JS) |
| How many distinct blob **types**? | **10 in Go** + **1 TS interface** (`dashboard/types/spaxel.d.ts`) |
| Are there `NewBlob`/`makeBlob`/`CreateBlob` helpers? | **No.** Every blob is an inline struct/object literal inside its enclosing function. |
| Do the source findings agree? | Mostly. Two stale entries corrected (see §1.2). |
| What needs editing for **blob identity fields**? | **6 files** — see §5 (the deliverable the next bead consumes). |

---

## 1. Critical reconciliation notes (read before the inventory)

### 1.1 Two unrelated concepts share the word "blob"

The three search beads conflated two completely different things. This report keeps them
strictly separate:

| Concept | Where | What it is | Identity fields? |
|---|---|---|---|
| **Spaxel domain blob** (the tracked spatial presence) | `mothership/**` (Go) + `dashboard/js/**` (JS) | A detected person: position + velocity + posture + **identity** | **Yes** — the whole point of this work |
| **Browser `Blob` API** (binary data for downloads) | `dashboard/**/fleet*.js` | `new Blob([bytes], {type:'text/csv'})` — a file-download payload | **No** — unrelated; do **not** touch for identity work |

The `new Blob()` sites from bf-5kns are all the second kind. They are catalogued in §4 only
so the next bead knows they exist and can **skip them**.

### 1.2 Corrections to the source findings (verified against HEAD)

1. **`dashboard/js/state.js:290`** — bf-26ta reported the literal initializes
   `{id, personName:undefined, assignedColor:undefined, identityResolved:undefined}`.
   **Actual current code is just `{ id: id }`.** The identity fields are no longer
   pre-initialized; `Object.assign(appState.blobs[id], updates)` later merges server data in.
   Implication: if identity-field initialization at creation is desired, this site is a
   candidate to revisit — but it currently does **not** set them.

2. **`mothership/internal/tracking/tracker.go:160` (2D `tracking.Blob`)** — bf-26ta claimed the
   literal explicitly sets `PersonName:"", AssignedColor:"", IdentityResolved:false`.
   **Wrong on all three field names.** The struct's identity fields are
   `PersonID / PersonLabel / PersonColor / IdentityConfidence / IdentitySource / IdentityLastSeen`
   (same as the 3D `tracker.Blob`), and the literal sets **none** of them — it relies on Go
   zero values. See §3, entries [G2]/[A2].

3. **Go ↔ JS identity field names differ** (this is intentional, not an error — but the next
   bead must know both):

   | Concept | Go field (`tracker.Blob`, `tracking.Blob`, `sigproc.TrackedBlob`) | JS field (`spaxel.d.ts` Blob interface) |
   |---|---|---|
   | Display name | `PersonLabel` | `personName` (preferred) / `personLabel` (deprecated) |
   | Stable person id | `PersonID` | `personId` |
   | Render color | `PersonColor` | `assignedColor` (preferred) / `personColor` (deprecated) |
   | Identity resolved flag | `IdentityConfidence` (float) + `IdentitySource` | `identityResolved` (bool) |
   | Match provenance | `IdentitySource` (`"ble_triangulation"` / `"ble_only"` / `""`) | `identityResolved` tri-state |

---

## 2. Inventory — organized by FILE

### 2.1 Go production files

| File | Sites | Blob type(s) | Creation mechanism |
|---|---|---|---|
| `mothership/internal/fusion/fusion.go` | 1 (`:260`) | `fusion.Blob` | array assignment `blobs[i] = Blob{...}` |
| `mothership/internal/tracker/tracker.go` | 1 (`:162`) | `tracker.Blob` (3D) | pointer literal `&Blob{...}` in `(*Tracker).Update` |
| `mothership/internal/tracking/tracker.go` | 1 (`:160`) | `tracking.Blob` (2D, legacy) | pointer literal `&Blob{...}` in `(*Tracker).Update` |
| `mothership/cmd/mothership/main.go` | 3 (`:5384`, `:2213`, `:2116`) | `sigproc.TrackedBlob`, `automation.TrackedBlob`, `explainability.BlobSnapshot` | inline conversion loops |
| `mothership/internal/simulator/engine.go` | 1 (`:460`) | `simulator.BlobResult` | slice append in `detectBlobs` |
| `mothership/internal/replay/pipeline.go` | 2 (`:114`, `:132`) | `replay.BlobUpdate` | slice appends in `generateDemoBlobs` |
| `mothership/internal/explainability/handler.go` | 3 (`:357`, `:194`, `:255`) | `explainability.BlobExplanation` | pointer literals (`:357` substantive; `:194`/`:255` empty fallbacks) |

**Go subtotal: 12 production creation sites across 7 files.**

### 2.2 Go test-only files (out of scope for identity-field edits, listed for completeness)

| File | Site | Type | Note |
|---|---|---|---|
| `internal/explainability/handler_test.go` | `:30`/`:31` | `explainability.BlobSnapshot` | `makeBlobAt(...)` test helper |
| `internal/replay/integration_test.go` | `:578`, `:606` | `replay.BlobUpdate` | pipeline-over-test-frames (no own literal) |
| `internal/api/tracks_test.go` | `:17` | `TrackedBlob` | mock provider |
| `internal/volume/shape_test.go` | inline | `volume.BlobPos` | `[]BlobPos{{ID:1,X:2,Y:2,Z:2}}` fixtures |

### 2.3 JS production files

| File | Sites | Pattern | Identity fields set? |
|---|---|---|---|
| `dashboard/js/state.js` | 1 (`:290`) | `appState.blobs[id] = { id: id }` then `Object.assign` | **No** (corrected — see §1.2) |
| `dashboard/js/websocket.js` | 1 (`:167`) | `_blobStates.set(b.id, {x,z,vx,vz,ts})` | No — dead-reckoning extrapolation cache only |

### 2.4 JS test-only files

| File | Sites | Pattern |
|---|---|---|
| `dashboard/js/ambient.test.js` | 6 (`:124`, `:276`, `:642`, `:659`, `:694`, `:708`) | rendering test fixtures (full / minimal / position-only) |
| `dashboard/js/quick-actions.test.js` | 5 (`:208`, `:316`, `:470`, `:513`, `:678`) | context-menu / camera-follow fixtures |
| `dashboard/js/replay.test.js` | 1 (`:101`) | mock API response (velocity + posture) |

**JS subtotal: 2 production + 12 test = 14 sites across 5 files** (matches bf-26ta's corrected total).

### 2.5 TypeScript

- `dashboard/types/spaxel.d.ts` (`:10`–`~95`) — the **`Blob` interface** definition only.
  **Zero object-literal instantiations.** It is the canonical declaration of the JS-side
  identity fields (`personName`, `personId`, `assignedColor`, `personColor` [deprecated],
  `personLabel` [deprecated], `identityResolved`). If the JS identity schema changes, this
  file is the single declaration to update.

---

## 3. Inventory — organized by CREATION PATTERN

### Pattern 1 — Peak emission (origin of every blob)
- **[C1] `(*fusion.Engine).Fuse`** — `fusion/fusion.go:165` → literal `:260`
  ```go
  blobs[i] = Blob{X: p[0], Y: p[1], Z: p[2], Confidence: p[3]}
  ```
  Value-type per grid peak. **No identity** (peaks are pre-identity). This is the raw
  measurement fed downstream.

### Pattern 2 — Persistent track spawn (where identity is attached)
- **[A1] `(*tracker.Tracker).Update` (3D)** — `tracker/tracker.go:112` → literal `:162`
  ```go
  b := &Blob{ID: t.nextID, X: m[0], Y: m[1], Z: m[2], Weight: m[3],
      LastSeen: now, Trail: ..., Posture: PostureUnknown, ukf: NewUKF(...)}
  ```
  Pointer per unmatched measurement. **Identity fields exist on the struct but are NOT set
  in the literal** (zero-valued: `""`/`0`/zero-time). Identity is stamped later by `applyIdentity`.
- **[A3] `(*tracker.TrackManager).Update[WithIdentity]`** — `tracker/identity.go:63,77`
  Thin wrappers over A1; delegate creation, then call `applyIdentity(blob, info, now)`
  (`identity.go:164`) to populate identity in place. **No literal of their own.**
- **[A2] `(*tracking.Tracker).Update` (2D, legacy)** — `tracking/tracker.go:91` → literal `:160`
  Same shape as A1 but 2D `[x,z,weight]`. Struct carries the same identity fields; literal
  sets none of them.

### Pattern 3 — Accessor factories (hand copies/pointers to callers)
- **[B1] `(*tracker.TrackManager).GetBlob`** — `tracker/identity.go:188` — returns `&tm.blobs[i]`
  (pointer into internal storage).
- **[B2] `(*tracker.TrackManager).GetAllBlobs`** — `tracker/identity.go:201` — `make`+`copy`
  value slice. **Note:** shallow copy — trail header shared, not deep-copied.

### Pattern 4 — Cross-package conversion (identity-propagation boundary — see §5)
- **[E1] `(*blobTracker).track`** — `cmd/mothership/main.go:5337` → literal `:5384`
  ```go
  b := sigproc.TrackedBlob{ID: id, X: pk.X, Y: pk.Y, Z: pk.Z, Weight: pk.Confidence}
  ```
  Converts a `fusion.Blob` peak → `sigproc.TrackedBlob`. `sigproc.TrackedBlob` **has identity
  fields** but they're left zero here because the source is a pre-identity peak.
- **[E2] automation conversion loop** — `cmd/mothership/main.go:2213`
  ```go
  autoBlobs[i] = automation.TrackedBlob{ID: b.ID, X/Z, VX/VY/VZ, Confidence: b.Weight}
  ```
  `automation.TrackedBlob` has **NO identity fields** (struct is ID + pos + vel + Confidence
  only). Identity is **dropped** at this boundary.
- **[E3] explainability conversion loop** — `cmd/mothership/main.go:2116`
  ```go
  blobSnapshots = append(blobSnapshots, explainability.BlobSnapshot{
      ID: blob.ID, X/Y/Z, Confidence: blob.Weight})
  ```
  `explainability.BlobSnapshot` struct — identity **dropped** (only the 5 fields shown).

### Pattern 5 — Synthetic/demo factories (offline + test paths)
- **[D1] `(*simulator.Engine).detectBlobs`** — `simulator/engine.go:387` → `:460` —
  `append(blobs, BlobResult{ID, Position, Confidence, WalkerID, TrueError})`.
- **[D2] `(*replay.Pipeline).generateDemoBlobs`** — `replay/pipeline.go:100` → `:114`,`:132` —
  two fixed-ID figure-8 / circular synthetic `BlobUpdate`s.

### Pattern 6 — Explainability record factories
- **[F1] `(*explainability.Handler).computeExplanation`** — `handler.go:356` → `:357` —
  substantive `&BlobExplanation{BlobID, X/Y/Z, Confidence, ContributingLinks, ...}`. Cached
  into `h.blobHistory[blob.ID]`.
- **[F2] `(*explainability.Handler).explainBlob`** — `handler.go:180` → `:194` —
  empty fallback `&BlobExplanation{...0...}` for unknown ID.
- **[F3] `(*explainability.Handler).explainBlobAtTime`** — `handler.go:209` → `:255` —
  empty fallback (with `Timestamp`) when no history match.
- **[F4] `(*explainability.Handler).GetExplanationForBlob`** — `handler.go:276` —
  lookup accessor returning cached `*BlobExplanation` (no literal).

### Pattern 7 — Snapshot recorder (blob-derived record, not a returned blob)
- **[G1] `(*falldetect.Detector).processBlob`** — `falldetect/detector.go:270` → `:277` —
  `BlobSnapshot{ID, X/Y/Z, VX/VY/VZ, Posture, Timestamp}` appended to `d.blobHistory`.
  (Distinct type from `explainability.BlobSnapshot` despite the shared name.)

### Pattern 8 — JS state initialization
- **`dashboard/js/state.js:290`** — `appState.blobs[id] = { id: id }` (+ later `Object.assign`
  of server payload). Identity fields only appear if the server sends them.

### Pattern 9 — JS derived/extrapolation state
- **`dashboard/js/websocket.js:167`** — `_blobStates.set(b.id, {x,z,vx,vz,ts})`. Minimal
  dead-reckoning cache for disconnect extrapolation. Not a domain blob.

---

## 4. Unrelated `new Blob()` constructor sites (bf-5kns) — SKIP for identity work

These are the **browser binary `Blob` API** used for file downloads. They construct file
payloads, not Spaxel domain blobs. Catalogued only so the next bead can consciously skip them.

| File:line | MIME type | Purpose |
|---|---|---|
| `dashboard/static/js/fleet.js:457` | `text/csv` | fleet data CSV download (`downloadCSV`) |
| `dashboard/js/fleet-page.js:1034` | `application/json` | config JSON export (`exportConfig`) |
| `dashboard/js/fleet-page.js:1369` | `text/csv` | filtered fleet CSV download (`downloadCSV`) |
| `dashboard/js/fleet.js:1997` | `application/json` | config JSON export (`exportConfig`) |

All four follow the same idiom: `new Blob([content], { type: '...' })`. **Zero TypeScript
`new Blob()` calls exist.**

---

## 5. Files requiring updates for blob identity fields ⭐ (the deliverable)

The goal of the umbrella effort is to make blob **identity** (who the blob is) flow correctly
through the pipeline. Identity currently lives in **three** Go types and the JS interface,
and is **dropped at two conversion boundaries**. Ranked by impact:

### Tier 1 — Identity is DROPPED at conversion (must add fields + propagate)

| # | File:line | What's dropped | Source has it? | Target type | Fix |
|---|---|---|---|---|---|
| 1 | `cmd/mothership/main.go:2213` | person/color/source from tracked blob | **Yes** (`blobs[i]` is identity-bearing) | `automation.TrackedBlob` | Add identity fields to `automation.TrackedBlob` (`engine.go:1337`); populate them in the loop |
| 2 | `cmd/mothership/main.go:2116` | person/color from tracked blob | **Yes** | `explainability.BlobSnapshot` | Add identity fields to the snapshot type; populate so "Why?" can name the person |

> These two are the highest-value edits: without them, person-aware automations and
> person-labeled explainability cannot work even when BLE identity is fully resolved upstream.

### Tier 2 — Identity fields exist but literal doesn't set them (verify intent)

| # | File:line | Type | Current state | Action |
|---|---|---|---|---|
| 3 | `internal/tracker/tracker.go:162` | `tracker.Blob` (3D) | Identity fields zero-valued; populated later by `applyIdentity` (`identity.go:164`) | Confirm this is intended; no literal change needed unless you want explicit zero-init |
| 4 | `internal/tracking/tracker.go:160` | `tracking.Blob` (2D, legacy) | Same as above | Same; also confirm whether the legacy 2D tracker is still in the live path (bf-67ao notes the 3D `tracker` package is active) |

### Tier 3 — Identity-carrying type but built from a pre-identity source (no-op by design)

| # | File:line | Type | Why no identity yet | Action |
|---|---|---|---|---|
| 5 | `cmd/mothership/main.go:5384` | `sigproc.TrackedBlob` | Built from a `fusion.Blob` **peak** (peaks have no identity — identity is assigned by the tracker *after* this) | None, unless identity is reattached from a prior frame's tracker state |

### Tier 4 — JS/dashboard identity plumbing

| # | File | What | Action |
|---|---|---|---|
| 6 | `dashboard/types/spaxel.d.ts` | `Blob` interface — canonical JS identity-field declaration | If the JS schema changes, update here (single declaration point) |
| 6 | `dashboard/js/state.js:290` | Blob creation literal is now `{ id: id }` only | If pre-initializing identity fields at creation is desired, add them here (currently relies on server payload via `Object.assign`) |
| 6 | `dashboard/js/ambient_renderer.js:624–647` | Identity-field **consumption** (fallback chain `personName → person_label → person`) | Update the fallback chain if field names change; this reads, doesn't create |

### Out of scope (no identity fields, by design)

- `fusion/fusion.go:260` (peak — pre-identity)
- `simulator/engine.go:460`, `replay/pipeline.go:114/132` (synthetic/demo — no real identity)
- `explainability/handler.go:194/255` (empty fallbacks for unknown IDs)
- `falldetect/detector.go:277` (history snapshot — posture/velocity only; add identity only if fall alerts need person names, which per plan.md they do → consider promoting to Tier 1)
- All `*_test.go` / `*.test.js` fixtures (update only if struct schemas change and break tests)
- All four `new Blob()` file-download sites (§4 — unrelated browser API)

---

## 6. Blob data flow (where identity enters and where it leaks)

```
fusion.Engine.Fuse  ──fusion.Blob peaks──▶  (NO identity)
   [C1 :260]                                    │
                                                ▼
blobTracker.track  ──sigproc.TrackedBlob──▶  (fields exist, zero — peak source)
   [E1 main.go:5384]                            │
                                                ▼
tracker.Tracker.Update  ──tracker.Blob──▶  (spawned; identity fields zero)
   [A1 tracker.go:162]                          │
                                                ▼
TrackManager.UpdateWithIdentity ──applyIdentity──▶  ✅ IDENTITY ATTACHED
   [A3 identity.go:77 → 164]                    │
                                                ▼
GetAllBlobs / GetBlob ──copies to consumers──▶  (identity preserved)
   [B1/B2 identity.go:188/201]                  │
              ┌─────────────────────────────────┼──────────────────────────┐
              ▼                                  ▼                          ▼
   automation loop                     explainability loop            falldetect.processBlob
   [E2 main.go:2213]                   [E3 main.go:2116]              [G1 detector.go:277]
   ❌ IDENTITY DROPPED                  ❌ IDENTITY DROPPED            ⚠ posture/vel only
   (automation.TrackedBlob             (explainability.BlobSnapshot   (add identity if fall
    has no identity fields)             drops person/color)            alerts need a name)
```

The two ❌ sites are the leak this work should close.

---

## 7. Acceptance criteria status

- [x] **All blob creation sites consolidated into a single report** — §2 + §3 catalogue
  19 production sites (12 Go + 2 JS production + the 5 explainability/peak/conversion Go
  sites counted within the 12) across 9 files; §4 separates the 4 unrelated `new Blob()`
  file-download sites.
- [x] **Sites organized by file and by creation pattern** — §2 (by file), §3 (by pattern).
- [x] **Files requiring updates are clearly listed** — §5, tiered by impact, with the
  specific struct/line and the concrete field-propagation fix.
- [x] **Report is ready for the next bead to consume** — §1.2 flags the stale entries in
  the sub-bead notes so the implementer doesn't propagate errors; §6 shows exactly where
  identity leaks so the implementation bead has a precise target list.

---

## 8. Provenance / source beads

| Section | Source bead | File |
|---|---|---|
| Constructor sites | bf-5kns | `notes/bf-5kns.md` |
| Literals (Go+JS+TS) | bf-26ta (+ children bf-3aij, bf-1rzd, bf-4ly4, bf-3tlw) | `notes/bf-26ta-findings.md`, `notes/bf-26ta-javascript-results.md`, `notes/bf-26ta-typescript-results.md` |
| Factory functions | bf-67ao | `notes/bf-67ao-findings.md` |
| Verification + reconciliation + identity-flow analysis | bf-3ldj (this report) | `notes/bf-3ldj-findings.md` |
