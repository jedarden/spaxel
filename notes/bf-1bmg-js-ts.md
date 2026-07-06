# JS / TS Dashboard Blob-Creation Sites (bf-1bmg)

> **Scope:** Every blob-creation site on the **dashboard JS/TS side** of Spaxel — the JS/TS
> slice of the consolidated blob-identity effort (parent findings: `notes/bf-3ldj-findings.md`,
> §2.3–§2.5).
>
> **Every line number below was re-verified against `HEAD = 68bd308c3c4cccd3a709282161fa880e49d4fa88`
> on 2026-07-06** by reading the file, not by trusting the source notes. Two classes of drift in
> the source notes are corrected in §1.2 (do not copy them blindly).

---

## 0. TL;DR

| Question | Answer |
|---|---|
| How many **JS domain-blob creation** sites exist in production code? | **2** — `dashboard/js/state.js:290` and `dashboard/js/websocket.js:167` |
| Where is the JS-side identity schema declared? | **One place:** the `Blob` interface in `dashboard/types/spaxel.d.ts:10–91` |
| Are there JS factory helpers (`makeBlob`/`createBlob`)? | **No.** Both production sites are inline object literals. |
| How many `new Blob(...)` constructor calls exist? | **4** — all the **browser binary `Blob` API** for file downloads. **OUT OF SCOPE** for identity work (see §3). |
| Are there test-only blob fixtures? | **Yes — 12** across 3 files (listed in §4 for completeness). |
| Anything to fix on the JS side for blob identity? | Only if the schema changes → update `spaxel.d.ts` (single point); `state.js:290` already merges server identity via `Object.assign`. See §5. |

---

## 1. Read this first

### 1.1 Two unrelated concepts share the word "blob"

| Concept | Where | What it is | Identity fields? |
|---|---|---|---|
| **Spaxel domain blob** (tracked spatial presence) | `dashboard/js/state.js`, `dashboard/js/websocket.js` | A detected person: `{id, x, y, z, vx…, posture, person/personName, assignedColor, identityResolved}` | **Yes** — the whole point of this work |
| **Browser `Blob` API** (binary data for downloads) | `dashboard/**/fleet*.js` | `new Blob([bytes], {type:'text/csv'})` — a file-download payload | **No** — unrelated; do **not** touch for identity work |

The 4 `new Blob()` sites in §3 are all the second kind. They are catalogued only so the next
bead knows they exist and can **skip them**.

### 1.2 Corrections to the source notes (verified against HEAD)

1. **`dashboard/static/js/fleet.js:457` vs `dashboard/js/fleet.js`** — the bf-1bmg task brief
   lists the CSV download site as "`fleet.js:457`". **`fleet.js:457` does not exist in
   `dashboard/js/fleet.js`** — that file's only `new Blob()` is at `:1997` (JSON export).
   The `:457` site lives in a **different file**, `dashboard/static/js/fleet.js`
   (16 KB; not a copy of the 77 KB `dashboard/js/fleet.js` — `diff` confirms they differ).
   See §3, entry [D1].

2. **`dashboard/js/ambient.test.js` test-fixture line numbers (bf-3ldj §2.4)** — the source
   notes cite `124, 276, 642, 659, 694, 708`. Only **124** is the blob-literal opening line.
   `276` is a `y:` field line; `642 / 659 / 694 / 708` are the closing `}]` lines of their
   respective fixtures. The **actual literal-opening lines** (the `blobs: [{` line) are
   **124, 273, 636, 653, 688, 702**. Count (6) is unchanged. See §4.

3. **`dashboard/js/state.js:290`** — confirmed: the literal is exactly `{ id: id }`. Identity
   fields are **not** pre-initialized; `Object.assign(appState.blobs[id], updates)` at `:292`
   merges the server payload (including any identity fields) afterward. (Already corrected in
   bf-3ldj §1.2; re-confirmed here.)

---

## 2. Production JS domain-blob creation sites (the real inventory)

| # | File:line | Enclosing function | Pattern | Identity fields set at creation? |
|---|---|---|---|---|
| **[P1]** | `dashboard/js/state.js:290` | `updateBlob(id, updates)` (fn opens `:288`) | `appState.blobs[id] = { id: id };` then `:292 Object.assign(appState.blobs[id], updates)` | **No** — only `{id}`; server-supplied identity fields arrive via the `updates` payload and are merged by `Object.assign`. This is the **canonical creation point** for every live blob in the dashboard. |
| **[P2]** | `dashboard/js/websocket.js:167` | `_captureBlobStates()` (fn opens `:162`; Map declared `:26`) | `_blobStates.set(b.id, { x: b.x, z: b.z, vx: b.vx‖0, vz: b.vz‖0, ts: Date.now() })` | **No** — and intentionally none. This is a **dead-reckoning cache** for the <5 s disconnect-extrapolation feature (positions + velocity + timestamp only). It is a derived snapshot of an already-existing blob, not a domain blob, and never carries identity. |

```js
// [P1] dashboard/js/state.js:288-294
function updateBlob(id, updates) {
    if (!appState.blobs[id]) {
        appState.blobs[id] = { id: id };          // ← :290  creation literal (id only)
    }
    Object.assign(appState.blobs[id], updates);   // ← :292  server payload merged in
    notify('blobs.' + id, appState.blobs[id], null);
    notify('blobs', appState.blobs, null);
}

// [P2] dashboard/js/websocket.js:162-174
function _captureBlobStates() {
    if (!_lastSnapshot || !_lastSnapshot.blobs) return;
    _lastSnapshot.blobs.forEach(function (b) {
        _blobStates.set(b.id, {                   // ← :167  dead-reckoning cache
            x: b.x, z: b.z,
            vx: b.vx || 0, vz: b.vz || 0,
            ts: Date.now()
        });
    });
}
```

**Production subtotal: 2 creation sites across 2 files.** A defensive sweep of every
`dashboard/js/*.js` production file (all `blobs:` assignments, `set('blobs', …)` calls, and
`{…posture/confidence…}` literals — see §6) found **no other** domain-blob object-literal
creation. Empty `blobs: []` initializers in `simple-mode.js:35`, `ambient.js:36`,
`ambient_renderer.js:44`, and `home-cards.js:28` are array defaults, not blob creation.

### 2.1 Adjacent call sites that re-publish existing blobs (NOT new-literal creation — listed for completeness)

These route through `state.js` `updateBlob` / the `SpaxelState.set('blobs', …)` API but pass an
**already-existing** blob object, so no new literal is constructed:

| File:line | Pattern | Note |
|---|---|---|
| `dashboard/js/quick-actions.js:697` | `blobs[blob.id].person = null; blobs[blob.id].ble_device = null; SpaxelState.set('blobs', blob.id, blobs[blob.id])` | Identity-clear flow (re-assign / forget person). Mutates an existing blob in place, then re-publishes. |
| `dashboard/js/quick-actions.js:1689` | `blobs.forEach(b => SpaxelState.set('blobs', b.id, b))` | Refresh loop after `GET /api/blobs`; re-publishes each server blob through the state API. |

Neither constructs a blob literal; both delegate creation to `updateBlob` → **[P1]**.

---

## 3. `new Blob()` constructor sites — ⛔ OUT OF SCOPE (unrelated browser `Blob` API)

These construct **file-download payloads** via the standard browser `Blob` API, not Spaxel
domain blobs. They have no identity fields and no relationship to tracked-presence work.
Catalogued only so they can be **explicitly skipped**.

| # | File:line | Enclosing function | MIME type | Purpose |
|---|---|---|---|---|
| **[D1]** | `dashboard/static/js/fleet.js:457` | `downloadCSV(nodes)` (fn opens `:423`) | `text/csv` | Fleet table CSV download. ⚠️ **Note:** this is `dashboard/static/js/fleet.js`, a **different file** from `dashboard/js/fleet.js` — see §1.2. |
| **[D2]** | `dashboard/js/fleet-page.js:1034` | `async exportConfig()` (fn opens `:1026`) | `application/json` | Fleet config JSON export |
| **[D3]** | `dashboard/js/fleet-page.js:1369` | `downloadCSV()` (fn opens `:1337`) | `text/csv` | Filtered fleet CSV download |
| **[D4]** | `dashboard/js/fleet.js:1997` | `exportConfig()` (fn opens `:1988`) | `application/json` | Fleet config JSON export |

All four follow the identical idiom: `new Blob([content], { type: '...' })`.

- **Zero `new Blob()` calls exist in TypeScript** (`dashboard/types/`) — verified by grep.
- **Two MIME-type pairs:** CSV downloads `[D1]`,`[D3]`; JSON exports `[D2]`,`[D4]`. `[D2]`/`[D4]`
  are the same `exportConfig` logic duplicated across the two fleet files; `[D1]`/`[D3]` are the
  `downloadCSV` counterpart.

---

## 4. JS test-only blob fixtures (out of scope for edits — listed for completeness)

These are rendering/context-menu/mock-API fixtures in `*.test.js`. Update only if a struct
schema change breaks them. Line numbers below are the **literal-opening lines** verified against
HEAD (correcting the bf-3ldj §2.4 drift — see §1.2).

### 4.1 `dashboard/js/ambient.test.js` — 6 sites

| Line | Shape | Fields | Used by |
|---|---|---|---|
| `:124` | full person blob | `id,x,y,z,confidence,person:'Alice'` | "draw person blob at correct position" |
| `:273` | minimal position | `id,x,y,z` | ambient zone-presence brightness restore |
| `:636` | full | `id,x,y,z,confidence` | camera-follow start position (1,1) |
| `:653` | full | `id,x,y,z,confidence` | camera-follow move target (3,3) |
| `:688` | full | `id,x,y,z,confidence` | lerp initial position (1,1) |
| `:702` | full | `id,x,y,z,confidence` | lerp target position (3,3) |

*(Empty `blobs: []` at `:86` and `:237` are not creation sites. Zone/alert `id:` lines at
`:78`,`:229`,`:265`,`:330`,`:366`,`:389`,`:423` are non-blob fixtures.)*

### 4.2 `dashboard/js/quick-actions.test.js` — 5 sites

| Line | Shape | Fields | Used by |
|---|---|---|---|
| `:208` | via state API | `SpaxelState.set('blobs',123,{id,person:'Alice',x,y,z})` | context-menu appearance test |
| `:316` | `const blob = {}` | `id:123,person,x,y,z` | (context-menu fixture) |
| `:470` | `const blob = {}` | `id:123,person:'Alice',x,y,z` | "Follow" camera-mode test |
| `:513` | `const blob = {}` | `id:123,…` | context-menu fixture |
| `:678` | single-line | `{ id:123, x:2, y:0, z:3 }` | minimal position fixture |

*(Inline `{ id: 1 }` / `{ id:123, person:'Alice' }` lookup refs passed to `show(...)` at
`:227`,`:556`,`:573`,`:586`,`:605`,`:628`,`:750` are target references, not blob creation.
Zone/node fixtures at `:365` etc. are non-blob.)*

### 4.3 `dashboard/js/replay.test.js` — 1 site

| Line | Shape | Fields | Used by |
|---|---|---|---|
| `:101` | mock API array | `blobs:[{id,x:2.5,y:1.3,z:0.8,vx,vy,vz,weight:0.85,posture:'standing'}]` | mock `/api/replay/session/...` response fed to `Viz3D.updateReplayBlobs` |

*(The `expect.objectContaining({id:1,x:2.5,…})` matcher at `:291` is an **assertion**, not a
creation — it references the `:101` fixture. Empty `blobs: []` at `:230` is not a creation site.)*

**Test subtotal: 6 + 5 + 1 = 12 fixtures across 3 files.**

---

## 5. The canonical JS identity-field declaration: `dashboard/types/spaxel.d.ts`

`dashboard/types/spaxel.d.ts:10–91` declares the **`Blob` interface** — the **single source of
truth** for the JS-side blob shape, including all identity fields. If the JS identity schema
changes, **this is the only declaration to update**; no JS file redeclares it.

```ts
export interface Blob {              // :10  ← interface opens
  id: string;                       // :12  (note: string here; runtime blobs use numeric id)
  x: number; y: number; z: number;  // :15-17
  confidence: number;               // :20
  vx?: number; vy?: number; vz?: number;        // :23-25
  posture?: string;                             // :28
  person?: string | null;                       // :31  legacy shorthand display field
  ble_device?: string | null;                   // :34
  trails?: Array<{x,y,z,timestamp_ms}>;         // :37-42

  // ── Identity Resolution Fields (header comment :44-46) ──
  personName?: string;              // :53  ✅ preferred display name
  personLabel?: string;             // :60  ⚠️ @deprecated → use personName
  personId?: string;                // :66  stable person id
  assignedColor?: string;           // :74  ✅ preferred render color (hex/rgb)
  personColor?: string;             // :81  ⚠️ @deprecated → use assignedColor
  identityResolved?: boolean;       // :90  tri-state: true / false / undefined
}                                  // :91  ← interface closes
```

**Identity fields (the 6 the umbrella effort cares about):** `personName`, `personLabel`
(deprecated), `personId`, `assignedColor`, `personColor` (deprecated), `identityResolved`.
Plus the legacy shorthand `person?` at `:31` (still consumed by `ambient_renderer.js` via the
fallback chain `personName → person_label → person`).

**Zero object-literal instantiations** of `Blob` exist in `dashboard/types/` — it is a
declaration-only `.d.ts`. **Zero `new Blob()` constructor calls** exist in TypeScript (verified).

### Go ↔ JS identity-field name map (for the implementation bead)

| Concept | Go field (`tracker.Blob` etc.) | JS field (`spaxel.d.ts`) |
|---|---|---|
| Display name | `PersonLabel` | `personName` (preferred) / `personLabel` (deprecated) / `person` (legacy shorthand) |
| Stable person id | `PersonID` | `personId` |
| Render color | `PersonColor` | `assignedColor` (preferred) / `personColor` (deprecated) |
| Identity resolved | `IdentityConfidence` (float) + `IdentitySource` | `identityResolved` (bool, tri-state) |

---

## 6. Methodology / how the production sweep was done

To guarantee no production JS blob-creation site was missed, every `dashboard/js/*.js` file
(excluding `*.test.js`, `*.test.setup.js`, `node_modules/`, and the `static/` duplicate) was
scanned for:

1. `blobs:` array/literal assignments (e.g. `blobs: [{`, `blobs: []`, `.blobs[id] =`)
2. `SpaxelState.set('blobs', …)` calls
3. Object literals carrying blob-shaped field sets (`posture:` / `confidence:` + `id`)

Results: the only **new-literal** production creation sites are **[P1]** and **[P2]** in §2.
Every other hit was either an empty-array default, an index/lookup of an existing blob, a
non-blob record (zone/node/alert/explainability/mesh-handle), or a re-publish of an existing
blob (§2.1).

---

## 7. Acceptance criteria status

- [x] **Every JS blob-creation site listed with file:line + pattern** — §2 (2 production sites)
  + §2.1 (2 adjacent re-publish call sites) + §4 (12 test fixtures).
- [x] **`spaxel.d.ts` Blob interface documented as the canonical JS identity-field declaration**
  — §5, with the 6 identity fields and the Go ↔ JS name map.
- [x] **The 4 `new Blob()` download sites explicitly flagged as OUT OF SCOPE / unrelated browser
  API** — §3, each marked ⛔ with MIME type + enclosing function.
- [x] **All line numbers verified against current HEAD** — re-verified against
  `68bd308` on 2026-07-06 by reading each file; §1.2 corrects the two drifts found in the
  source notes (the `static/js/fleet.js` mislabel and the `ambient.test.js` fixture lines).
- [x] **Findings written to `notes/bf-1bmg-js-ts.md`** — this file.

---

## 8. Provenance

| Source | Used for |
|---|---|
| `notes/bf-3ldj-findings.md` §2.3–§2.5, §4 | Starting inventory (JS production + test sites, the 4 `new Blob()` downloads) — re-verified and corrected where drift found (§1.2) |
| Direct file reads against `HEAD 68bd308` | Every line number in this report |
