# Dashboard JS/TS Identity-Field Surface — Verification (bf-3wkz)

> **Purpose:** closes the Tier-4 dashboard track of `notes/bf-1q3m-consolidated.md` §6
> (#7/#8/#9). Verifies the dashboard JS/TS blob surface declares and correctly surfaces the
> three canonical identity fields (`personName`, `assignedColor`, `identityResolved`), aligned
> with the Go JSON tags emitted by the projections that feed the dashboard.
>
> **Verified against HEAD `6e92f90` on 2026-07-06.** One real mismatch found and fixed
> (`volume.BlobPos`); everything else confirmed.
>
> **Independently re-verified at HEAD `7309564` on 2026-07-06** (second pass, retry of the same
> bead): all 8 Go types re-checked to emit the canonical camelCase trio, `state.js:288-294` and
> `websocket.js:159-174` re-confirmed, `go vet ./...` and `go test ./...` green. No further
> changes required. See §7.

---

## 1. TS declaration — confirmed (`dashboard/types/spaxel.d.ts`)

The `Blob` interface (`:10–91`) declares the canonical trio **and** the deprecated aliases:

| Field | Line | JSON key | Status |
|---|---|---|---|
| `personName?: string` | 53 | `personName` | ✅ canonical |
| `assignedColor?: string` | 74 | `assignedColor` | ✅ canonical |
| `identityResolved?: boolean` | 90 | `identityResolved` | ✅ canonical (tri-state via `undefined`) |
| `personLabel?: string` | 60 | `personLabel` | deprecated alias |
| `personColor?: string` | 81 | `personColor` | deprecated alias |
| `personId?: string` | 66 | `personId` | auxiliary |

`spaxel.d.ts` is the **single declaration point** for the JS blob schema (bf-1q3m §6 Tier-4 #7);
no JS file redeclares it.

---

## 2. TS ↔ Go JSON-tag mapping (the cross-check)

The canonical trio is carried by **8 Go types**. **7 emit camelCase** matching the TS; **1
(`volume.BlobPos`) emitted snake_case** — the mismatch — and is fixed in this bead.

| # | Go type | file:line | `personName` | `assignedColor` | `identityResolved` | Feeds dashboard? |
|---|---|---|---|---|---|---|
| 1 | `dashboard.blobJSON` | `internal/dashboard/hub.go:558-560` | `personName` ✅ | `assignedColor` ✅ | `identityResolved` (*bool) ✅ | **Yes — `/ws/dashboard` wire format** |
| 2 | `api.Track` | `internal/api/tracks.go:31-33` | `personName` ✅ | `assignedColor` ✅ | `identityResolved` (*bool) ✅ | **Yes — `GET /api/tracks`** |
| 3 | `explainability.BlobSnapshot` | `internal/explainability/handler.go:109-111` | `personName` ✅ | `assignedColor` ✅ | `identityResolved` (*bool) ✅ | **Yes — explainability REST** (via `BlobExplanation`, surfaced by `api/feedback.go:61`) |
| 4 | `volume.BlobPos` | `internal/volume/shape.go:1090-1092` | ~~`person_name`~~ → `personName` 🔧 | ~~`assigned_color`~~ → `assignedColor` 🔧 | ~~`identity_resolved`~~ → `identityResolved` 🔧 | No (internal trigger-eval; `Evaluate` returns `[]string`) — see §3 |
| 5 | `signal.TrackedBlob` | `internal/signal/processor.go:605-607` | `personName` ✅ | `assignedColor` ✅ | `identityResolved` (*bool) ✅ | Source struct (3D tracker feed) |
| 6 | `tracker.Blob` (3D) | `internal/tracker/tracker.go:58-60` | `personName` ✅ | `assignedColor` ✅ | `identityResolved` (*bool) ✅ | Source (3D) |
| 7 | `tracking.Blob` (2D) | `internal/tracking/tracker.go:45-47` | `personName` ✅ | `assignedColor` ✅ | `identityResolved` (*bool) ✅ | Source (2D, drives `blobJSON` via `BroadcastLocUpdate`) |
| 8 | `automation.TrackedBlob` | `internal/automation/engine.go:1347-1349` | `personName` ✅ | `assignedColor` ✅ | `identityResolved` (*bool) ✅ | Engine-internal (Tier-1 #1 type; identity populated by a follow-up bead) |

> **Deprecated aliases** (`person_label`/`person_color`, snake_case) are present on the source
> types (#5–8) and on `blobJSON`/`api.Track` exactly as the TS declares them — consistent, no
> change. The many other snake_case `person_id`/`person_name`/`person_label` fields elsewhere
> (`events`, `notifications`, `ble.registry`, `analytics`, etc.) belong to unrelated
> event/notification/registry types, **not** the canonical trio, and are out of scope.

**Net result: all 8 types now emit the canonical trio as `personName`/`assignedColor`/
`identityResolved`, exactly matching `spaxel.d.ts`.**

---

## 3. The mismatch & fix — `volume.BlobPos`

`volume.BlobPos` (`internal/volume/shape.go:1080`) was the sole outlier: bf-5v3q added the
canonical trio with **snake_case** JSON tags, while the sibling explainability.BlobSnapshot
(bf-2ibc) explicitly used camelCase to match the dashboard. The Go struct field names were
already canonical (`PersonName`/`AssignedColor`/`IdentityResolved`); only the JSON tags
diverged.

**Fix:** tags changed `person_name`→`personName`, `assigned_color`→`assignedColor`,
`identity_resolved`→`identityResolved`; comment updated to cross-reference the dashboard
alignment convention (mirroring `explainability.BlobSnapshot`).

**Safety of the change:**
- `volume.BlobPos` is **not** JSON-marshalled to the dashboard. It is consumed in Go:
  `volume.Store.Evaluate(blobs []BlobPos) []string` returns fired-trigger IDs
  (`shape.go:550`); `api.VolumeTriggersHandler.EvaluateTriggers` likewise returns `[]string`
  (`volume_triggers.go:782`). Webhook/MQTT/notification payloads are built from separate
  `volume.FiredEvent`-derived maps, not from `BlobPos` (`volume_triggers.go:579,885,967`).
- The person-filter read is a Go field access (`blob.PersonName`), not a JSON decode — so the
  tag change cannot affect trigger matching.
- No test references the snake_case keys (verified: the only hits for `person_name`/
  `assigned_color`/`identity_resolved` in `internal/volume/` were the struct-definition lines
  themselves).
- `go vet ./...` and `go test ./...` both pass after the change.

The tags were therefore inert today but would have leaked snake_case keys inconsistent with the
TS interface the moment `BlobPos` is ever surfaced (debug endpoint, volume-trigger state API).
Aligning them removes that latent drift and makes the cross-check uniform.

---

## 4. JS plumbing — confirmed

### J1 — `dashboard/js/state.js:288-294` (creation path)
```js
function updateBlob(id, updates) {
    if (!appState.blobs[id]) {
        appState.blobs[id] = { id: id };          // ← :290  creation literal (id only)
    }
    Object.assign(appState.blobs[id], updates);   // ← :292  server payload merged in
    ...
}
```
✅ Confirmed. The creation literal is `{ id: id }` only; the server-supplied identity
(`personName`/`assignedColor`/`identityResolved` from `blobJSON`) arrives in `updates` and is
merged by `Object.assign` at `:292`. **No pre-init needed** — no change.

### J2 — `dashboard/js/websocket.js:167` (dead-reckoning cache)
```js
_blobStates.set(b.id, { x: b.x, z: b.z, vx: b.vx||0, vz: b.vz||0, ts: Date.now() });  // ← :167
```
✅ Confirmed identity-free by design. This is the `<5s` disconnect extrapolation cache
(position + velocity only). It never carries identity. **Left alone.**

---

## 5. Acceptance criteria

- [x] **spaxel.d.ts Blob interface confirmed** to declare `personName`, `assignedColor`,
  `identityResolved` (+ deprecated `personLabel`/`personColor` aliases) — §1.
- [x] **TS field names ↔ Go JSON-tag mapping documented; mismatch fixed** — §2 mapping table;
  the single mismatch (`volume.BlobPos` snake_case) fixed in §3.
- [x] **state.js:290 creation path confirmed** to surface server identity via `Object.assign`;
  no change needed — §4 (J1).
- [x] (Out-of-band confirmation) **websocket.js:167 (J2)** intentionally identity-free — §4 (J2).

---

## 6. Provenance

| Source | Used for | Status at HEAD `6e92f90` |
|---|---|---|
| `notes/bf-1q3m-consolidated.md` §6 Tier-4 (#7/#8/#9) | dashboard identity-field track definition | consumed; this bead closes it |
| `notes/bf-5151*` (canonical identity fields) | camelCase convention authority | confirmed across 8 types |
| `notes/bf-2ibc` (explainability.BlobSnapshot) | sibling child — camelCase reference pattern | confirmed at `handler.go:109-111` |
| `notes/bf-5v3q` (volume.BlobPos) | sibling child — introduced snake_case | fixed here to camelCase |

---

## 7. Independent re-verification (second pass, HEAD `7309564`)

The bead was retried (`failure-count:1`); the fix + this note landed in commit `7309564`
(already on `origin/main`) but the bead was left open. This section records a fresh,
independent confirmation rather than trusting the prior pass:

- **8 Go types** — `grep` of `PersonName|AssignedColor|IdentityResolved` JSON tags across
  `dashboard/hub.go:558-560`, `api/tracks.go:31-33`, `explainability/handler.go:109-111`,
  `volume/shape.go:1090-1092`, `signal/processor.go:605-607`, `tracker/tracker.go:58-60`,
  `tracking/tracker.go:45-47`, `automation/engine.go:1347-1349` — all emit
  `personName`/`assignedColor`/`identityResolved` (the `*bool` tri-state for the flag). ✅
- **`volume.BlobPos`** fix from §3 confirmed present at `shape.go:1090-1092` (camelCase). ✅
- **JS creation path** — `dashboard/js/state.js:288-294` `updateBlob`: creation literal
  `{ id: id }` at `:290`, server identity merged via `Object.assign(...)` at `:292`. ✅ no change
- **JS dead-reckoning cache** — `dashboard/js/websocket.js:159-174` `_captureBlobStates`
  caches `{ x, z, vx, vz, ts }` only; identity-free by design. ✅ left alone
- **Gates** — `go vet ./...` exit 0; `go test ./...` all packages `ok`. ✅

No source change was needed in this pass — the surface is correct and uniform. Bead closed.
