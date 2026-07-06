# bf-1m2x — Inventory Currency Verification (HEAD re-check)

> **Purpose:** a short verification note confirming the blob-creation inventory in
> `notes/bf-1q3m-consolidated.md` (the source of truth, verified at `1a26c12`) is **still
> current at the present HEAD**. This bead CLOSES the "is the inventory complete and current?"
> loop. It does **not** re-do discovery from scratch — it re-runs only the two completeness
> greps and reconciles the counts against the consolidated report. **No Go/JS source was
> modified** (notes-only).

---

## 1. SHAs + delta vs the report's verification commit

| | SHA | When | Notes |
|---|---|---|---|
| Report verified at | `1a26c1259640d4cff63973319548c1e3a7246e50` (`1a26c12`) | 2026-07-06 15:49 -0400 | `docs(bf-1bmg): inventory JS/TS dashboard blob-creation sites` |
| **Current HEAD** | `0ae0c0310f1f12dbef251f8a5b10fb750f3a736c` (`0ae0c03`) | 2026-07-06 16:08 -0400 | `docs(bf-1q3m): consolidate & re-verify all blob-creation findings into single report` |

**Delta vs `1a26c12`:** the two intervening commits are `docs(bf-1bmg)` and `docs(bf-1q3m)`.
Verified that **no Go or JS source changed**:

```
$ git diff --name-only 1a26c12 HEAD | grep -vE '^notes/|^.beads/'
(empty — 0 files)
```

Only `notes/bf-1q3m-consolidated.md` (518 insertions) and `.beads/` housekeeping landed. The
tracked Go/JS blob-creation sites therefore cannot have moved. The greps below confirm this
empirically.

---

## 2. Completeness greps re-run at HEAD `0ae0c03`

Exactly the two sweeps from consolidated report §1.1, run verbatim:

### 2.1 Primary construction sites — `Blob{|TrackedBlob{` (expect 5)

```
mothership/internal/fusion/fusion.go:260           blobs[i] = Blob{X: p[0], Y: p[1], Z: p[2], Confidence: p[3]}   (C1)
mothership/internal/tracking/tracker.go:160        b := &Blob{                                                       (A2)
mothership/internal/tracker/tracker.go:162         b := &Blob{                                                       (A1)
mothership/cmd/mothership/main.go:2303             autoBlobs[i] = automation.TrackedBlob{                            (E2)
mothership/cmd/mothership/main.go:5494             b := sigproc.TrackedBlob{                                         (E1)
→ count = 5  ✅  (matches report §3.1 — C1/A2/A1/E2/E1)
```

### 2.2 Projection / derived type literals (expect 15 non-test)

```
cmd/mothership/main.go:2206        explainability.BlobSnapshot{     (P1)
cmd/mothership/main.go:2326        volume.BlobPos{                  (P2)
internal/tracking/tracker.go:173   BlobEvent{                       (P11)
internal/tracking/tracker.go:188   BlobEvent{                       (P12)
internal/explainability/handler.go:194  BlobExplanation{            (P4)
internal/explainability/handler.go:255  BlobExplanation{            (P5)
internal/explainability/handler.go:357  BlobExplanation{            (P6)
internal/falldetect/detector.go:277     BlobSnapshot{               (P7)
internal/replay/pipeline.go:114         BlobUpdate{                 (P8)
internal/replay/pipeline.go:132         BlobUpdate{                 (P9)
internal/simulator/engine.go:460        BlobResult{                 (P10)
internal/volume/shape.go:375           BlobState{                   (P13)
internal/volume/shape.go:575           BlobState{                   (P14)
internal/volume/shape.go:820           BlobState{                   (P15)
internal/volume/shape.go:879           BlobState{                   (P16)
→ count = 15  ✅  (matches report §1.2 named-literal sweep)
```

### 2.3 JS production sites (expect 2)

```
dashboard/js/state.js:290        appState.blobs[id] = { id: id };          (J1 — updateBlob creation literal)
dashboard/js/websocket.js:167    _blobStates.set(b.id, { x,z,vx,vz,ts })   (J2 — _captureBlobStates dead-reckoning cache)
→ count = 2  ✅  (matches report §3.4)
```

### 2.4 Type catalogue cross-check (expect 15 = 5 primary + 1 alias + 9 projection)

`grep ^type (Blob|TrackedBlob|BlobSnapshot|BlobState|BlobPos|BlobUpdate|BlobEvent|BlobResult|BlobExplanation)`
returns **14 struct defs + 1 type alias = 15 types** at HEAD — identical to report §2. The
alias `api.TrackedBlob = signal.TrackedBlob` (`internal/api/tracks.go:30`) is unchanged.

---

## 3. Result: zero deltas vs `1a26c12`

| Category | Expected (report) | At HEAD `0ae0c03` | Delta |
|---|---|---|---|
| Primary construction sites | 5 | **5** | none |
| Non-test named projection literals | 15 | **15** | none |
| Projection/boundary sites incl. anon-struct (P3) | 16 | **16** | none |
| JS production sites | 2 | **2** | none |
| Total production sites | 23 (5+16+2) | **23** | none |
| Blob-shaped types | 15 | **15** | none |

**No site added or removed since `1a26c12`.** Expected, because no Go/JS source committed in
the interval (§1). Every `file:line` in the consolidated report remains exact at HEAD.

---

## 4. The 16-sites vs 15-literals distinction (reconciling report §0 and §1.2)

The consolidated report §0 says **16 projection/boundary sites** while §1.2's completeness
sweep reports **15 non-test projection literals**. Both are correct and this re-check confirms
the reconciliation: site **P3** (`cmd/mothership/main.go:2288`, fed to `fallDetector.Update`)
is an **anonymous struct literal** (`{ID,X,Y,Z,VX,VY,VZ,Posture}`), not a named projection
type — so it does not appear in the `BlobSnapshot{|BlobState{|…` grep. It is counted in the
16-site catalogue (report §3.2) but not in the 15-literal grep (report §1.2). No discrepancy;
no drift.

---

## 5. Source of truth & method

- **Source of truth:** `notes/bf-1q3m-consolidated.md` (explicitly supersedes
  `notes/bf-3ldj-findings.md` and `notes/bf-4bhd.md` for the inventory).
- **Method:** re-ran only the two completeness greps + the type-def grep against the current
  HEAD; **did not re-search from scratch** (the anti-pattern that burned prior attempts).
  Counts reconciled against report §1.2 / §3.
- **Scope of this bead:** currency confirmation only. The discovery itself was delivered by
  the closed dependency subtree (bf-1q3m, bf-3ldj, bf-55rp, bf-5uzm, bf-4wwt, bf-1bmg) and
  consolidated in the report cited above.

## Acceptance criteria

- [x] Verification note records current HEAD SHA (`0ae0c03`) + grep counts
      (5 primary / 15 non-test projection / 2 JS production).
- [x] Delta vs `1a26c12` explicitly documented — **none** (only `notes/` + `.beads/` changed;
      0 Go/JS files).
- [x] Note cites `notes/bf-1q3m-consolidated.md` as the source of truth and does not re-search
      from scratch.
