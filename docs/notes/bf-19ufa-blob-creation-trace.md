# bf-19ufa — Blob DOM/mesh creation trace (dashboard render path)

Split from **bf-1018k** ("Resolve blobSeen=false in identity-less dashboard capture").
Sibling: **bf-2aogb** (viz3d non-init root cause — closed). This bead is explicitly
*independent of the viz3d init state*: it answers only whether blob creation is
**gated on an identity being present**.

## TL;DR (the answer to the acceptance criterion)

**Blob creation is NOT gated on identity — not in the `/live` (viz3d) path, not in
the `/ambient` (Canvas 2D) path. There is no identity gate to name.** The blob
render path creates a blob for *every* entry in the `blobs` array regardless of
whether `personName` / `identityResolved` is set; identity only *recolors* an
already-drawn blob (person color vs. neutral grey).

Therefore the `blobSeen=false` recorded for the identity-less run
(`identity-less.live.json`) is **not** caused by the absence of identity. It is
caused by the render path not reaching blob creation at all — i.e. the viz3d
renderer not initializing (`renderEvidence.viz3d=false`), which is bf-2aogb's
domain. Decisive proof: the **identity-RESOLVED** run behaves identically
(`identity.live.console.txt` → `viz3d=false, blobSeen=false`) even though it
received `[Spaxel] BLE scan: 2 devices` and `presence_transition … by blob #0`
(see §3). If rendering were gated on identity, the identity run would show blobs;
it does not.

This satisfies the acceptance in the negative sense: the "specific gate or
condition" that prevents a blob from appearing in identity-less mode is **the
viz3d renderer init, not identity** — and that gate is identity-agnostic.

---

## 1. `/live` render path — viz3d (Three.js WebGL)

There are no DOM *elements* for blobs here; viz3d renders blobs as Three.js
objects (a `THREE.Group` humanoid + trail `Line` + pillar `Line`) added to the
`_scene`. "blob seen" in the capture harness means an entry exists in viz3d's
in-memory `_blobs3D` map (read via `window.Viz3D.getBlobStates()`).

Dispatch chain (file:line):

1. `app.js:1063` — `loc_update` WS message → `Viz3D.handleLocUpdate(msg)`.
2. `app.js:1253-1254` and `app.js:1295-1296` — snapshot / incremental frames
   route `msg.blobs` → `Viz3D.handleLocUpdate({ type:'loc_update', blobs: msg.blobs })`.
3. `viz3d.js:1613-1615` — `handleLocUpdate(msg)` → `applyLocUpdate(msg.blobs || [])`.
4. `viz3d.js:771-796` — `applyLocUpdate(blobs)`: for each blob `b`, if not already
   in `_blobs3D` it calls `_createBlobObj(b.id)` and stores it; then sets position,
   velocity, posture, trail, pillar. **No `if (personName/identityResolved)`
   anywhere in this function.**
5. `viz3d.js:705-738` — `_createBlobObj(id)` builds the group/humanoid/trail/pillar.
   Color is `BLOB_COLORS[id % BLOB_COLORS.length]` (a palette slot indexed by blob
   id), **never** derived from identity. Identity is not consulted at creation.

Identity is a separate, *additive* rendering step layered on top of an
already-existing blob: `_identityLabels` (`viz3d.js:805+`) attaches a text sprite
*only when* identity data is present. Its absence does not prevent the mesh from
existing — it just leaves the blob with its default palette color.

The only conditions gating creation in this path are:
(a) viz3d initialized so `_scene` exists and `applyLocUpdate` is reachable; and
(b) `handleLocUpdate` receives a non-empty `blobs` array.
Both are **independent of identity**.

A repo-wide grep for `identityResolved` across `dashboard/js/*.js` (excluding
`*.test.js`) returns exactly two hits — both in viz3d's state-serialization dump
(`viz3d.js:3882`, `viz3d.js:3918`), which *read* the field for debugging output.
Neither is a creation gate.

## 2. `/ambient` render path — ambient_renderer (Canvas 2D)

Dispatch chain:

1. `ambient.js:383` — `handleWebSocketMessage(data)`.
2. `ambient.js:388 / 422 / 467` — snapshot / `loc_update` / incremental all do
   `currentState.blobs = data.blobs` with **no identity check**.
3. `ambient.js:404 / 434 / 507` — `renderer.updateState(currentState)`.
4. `ambient_renderer.js:138-178` — `updateState(state)` lerps positions and sets
   `currentState.blobs = state.blobs` (line 172; the copy that feeds `drawPeople`).
5. `ambient_renderer.js:608-656` — `drawPeople(ctx, bounds, colors)`:
   - line 609: `currentState.blobs.forEach(blob => { … })` — iterates **every**
     blob unconditionally.
   - line 636: `let blobColor = '#6b7280';` — neutral grey is the **default**.
   - lines 638-641: `const personName = blob.person_label || blob.person || null;
     if (personName) { blobColor = getPersonColor(personName); }` — identity only
     *overrides the color*; it never skips drawing.
   - line 644-647: the arc is drawn regardless.
   - line 650: `const name = personName ? getFirstName(personName) : '?';` — an
     identity-less blob is drawn with a literal `?` label.

So an identity-less blob is **fully rendered** (grey circle + `?`). The only
condition gating ambient rendering is a blob being present in
`currentState.blobs` — again independent of identity.

(For completeness: the identity-less ambient capture also reports `blobSeen=false`,
but its `renderEvidence.fallbackPx=211` shows the grey fallback color *was* drawn
on the canvas — i.e. identity-less rendering works; the `blobSeen` probe just
returned 0 blobs within the short capture window, a timing/seed matter, not an
identity gate. That is out of scope for this bead but corroborates the finding.)

## 3. Decisive runtime proof (both captures)

`scripts/capture-dashboard-console.mjs` records `blobSeen` and `renderEvidence`
for each captured route. Comparing the two `/live` runs under
`docs/notes/bf-4do5y-runtime-capture/`:

| run | identity on wire | renderEvidence.viz3d | blobSeen | blob events in console |
|---|---|---|---|---|
| `identity-less.live` | none | **false** | **false** | `detection … by blob #3`, `#4` |
| `identity.live` | yes (BLE scan, presence_transition blob #0) | **false** | **false** | `detection … by blob #13`, `#14` |

Both runs received real blobs from `/ws/dashboard` and emitted `[Spaxel] Event:
detection …` lines (proving the WS feed and blob processing work), yet **both**
report `viz3d=false` and `blobSeen=false`. If identity gated blob creation, the
identity run would render blobs where the identity-less run would not — the
opposite of what is observed (they are identical). **Identity is not the gate.**

## 4. Conclusion / input to parent bf-1018k

- Blob creation is identity-agnostic in both render paths. There is no code path
  in `dashboard/js/{viz3d,ambient,ambient_renderer}.js` that withholds a blob
  because `personName`/`identityResolved` is unset.
- The `blobSeen=false` for the identity-less run is therefore fully attributable
  to the renderer never initializing during that capture window (`viz3d=false`),
  which is the condition diagnosed by **bf-2aogb** — a condition independent of
  identity (proven by §3).
- **Recommendation for bf-1018k:** do not chase an identity gate; there isn't one.
  Resolution lives in making the viz3d renderer initialize (bf-2aogb's root cause
  + the child that applies the fix), at which point identity-less blobs will
  render in their default palette color as designed.
