# bf-4bhd — Re-dispatch: inventory currency re-verification (actual HEAD)

> **Why this note exists:** bf-4bhd ("Find all blob creation code paths") was **auto-re-dispatched**
> after it had already been comprehensively resolved by a closed bead chain. This note closes the
> re-dispatch by confirming the existing inventory is **still current at the actual reachable HEAD**
> — it does **not** re-do discovery from scratch (the documented anti-pattern that burned prior
> attempts; see `notes/bf-1m2x-verification.md` §5). **No Go/JS source was modified** (notes-only).

---

## 0. TL;DR

| Question | Answer |
|---|---|
| Is the blob-creation inventory complete & current? | **Yes** — `notes/bf-1q3m-consolidated.md` is exact at `21c702f` (the reachable pre-note commit; byte-identical to the `63f0c87` tree I actually grepped before the dispatcher rewrote it — see §3). |
| Empirical proof | Greps return **5** primary literals / **15** non-test projection literals / **15** blob types (14 structs + 1 alias) / **2** JS production sites — identical to the report's documented counts. |
| Has any tracked Go/JS/firmware source moved since the inventory was verified? | **No.** `git diff --stat f144aad HEAD -- mothership/ cmd/ dashboard/ firmware/` is empty (only `docs`/`.beads` commits since). |
| New finding worth recording? | **SHA provenance correction** (§3): two SHAs cited in the prior notes are *unreachable dangling objects* — history was rewritten after those notes were written. The inventory is unaffected, but `git show <cited-sha>` returns the wrong/absent commit. Mapped to real reachable SHAs below. |
| Can bf-4bhd be closed? | **Yes.** Its four acceptance criteria are satisfied by `notes/bf-1q3m-consolidated.md` (re-verified current here) + `notes/bf-5cgc-handoff.md` (the bead-linked handoff). |

---

## 1. Source of truth (unchanged)

- **`notes/bf-1q3m-consolidated.md`** — the sole trusted blob inventory. Supersedes
  `notes/bf-3ldj-findings.md` and `notes/bf-4bhd.md` (the latter is retained for provenance only;
  its `file:line` and 2D field-list are stale — do not consume it).
- **`notes/bf-5cgc-handoff.md`** — formalizes the report's tiered fix-target list into a
  site→implementation-bead handoff (Tier-1 = `main.go:2303` automation + `:2206` explainability +
  `:2326` volume).

This re-dispatch adds nothing to the catalogue itself; it only re-confirms currency.

---

## 2. Completeness greps re-run at `21c702f` (reachable; = `63f0c87` tree — see §3)

Exactly the two sweeps from the consolidated report §1.1, run verbatim against the working tree:

### 2.1 Primary construction sites — `Blob{|TrackedBlob{` (expect 5)

```
mothership/internal/fusion/fusion.go:260        blobs[i] = Blob{X: p[0], Y: p[1], Z: p[2], Confidence: p[3]}   (C1)
mothership/internal/tracking/tracker.go:160     b := &Blob{                                                       (A2)
mothership/internal/tracker/tracker.go:162      b := &Blob{                                                       (A1)
mothership/cmd/mothership/main.go:2303          autoBlobs[i] = automation.TrackedBlob{                            (E2)
mothership/cmd/mothership/main.go:5494          b := sigproc.TrackedBlob{                                         (E1)
→ count = 5  ✅
```

### 2.2 Projection / derived literals (expect 15 non-test named literals)

```
cmd/mothership/main.go:2206               explainability.BlobSnapshot{     (P1)
cmd/mothership/main.go:2326               volume.BlobPos{                  (P2)
internal/explainability/handler.go:194    BlobExplanation{                 (P4)
internal/explainability/handler.go:255    BlobExplanation{                 (P5)
internal/explainability/handler.go:357    BlobExplanation{                 (P6)
internal/falldetect/detector.go:277       BlobSnapshot{                    (P7)
internal/replay/pipeline.go:114           BlobUpdate{                      (P8)
internal/replay/pipeline.go:132           BlobUpdate{                      (P9)
internal/simulator/engine.go:460          BlobResult{                      (P10)
internal/tracking/tracker.go:173          BlobEvent{                       (P11)
internal/tracking/tracker.go:188          BlobEvent{                       (P12)
internal/volume/shape.go:375              BlobState{                       (P13)
internal/volume/shape.go:575              BlobState{                       (P14)
internal/volume/shape.go:820              BlobState{                       (P15)
internal/volume/shape.go:879              BlobState{                       (P16)
→ count = 15  ✅   (site P3 — `main.go:2288` — is an *anonymous* struct literal fed to
                    fallDetector.Update, so it correctly does NOT appear in this named-literal
                    grep; it is counted in the report's 16-site catalogue §3.2, not in §1.2's
                    15-literal sweep. No discrepancy.)
```

### 2.3 Type catalogue cross-check (expect 15 = 14 structs + 1 alias)

`grep ^type (Blob|TrackedBlob|BlobSnapshot|BlobState|BlobPos|BlobUpdate|BlobEvent|BlobResult|BlobExplanation)`
returns **14 struct defs + 1 pure alias (`api.TrackedBlob = signal.TrackedBlob` @ `api/tracks.go:30`) = 15**.
All 5 primary type defs + 9 projection defs are present and at the documented line numbers.

### 2.4 JS production sites (expect 2) — content spot-checked

```
dashboard/js/state.js:290        appState.blobs[id] = { id: id };          (J1 — creation literal; Object.assign merges server payload at :292)
dashboard/js/websocket.js:167    _blobStates.set(b.id, {x,z,vx,vz,ts})     (J2 — dead-reckoning cache, <5s disconnect extrapolation)
→ count = 2  ✅
```

### 2.5 Tier-1 boundary content spot-checked (the lines are real, not just present)

Each cited `main.go` line was opened and the literal confirmed to match the report verbatim:
`main.go:2303` (E2 `automation.TrackedBlob{ID,X/Y/Z,VX/VY/VZ,Confidence}` — no identity field),
`main.go:2206` (E3 `explainability.BlobSnapshot{ID,X,Y,Z,…}` — no identity field),
`main.go:2326` (volume `BlobPos{ID,X,Y,Z}` — `PersonID` field exists, left unset),
`main.go:5494` (E1 `sigproc.TrackedBlob{ID,X,Y,Z,Weight}`). All exact.

**Result: zero deltas vs the consolidated report.** Every `file:line` is exact at HEAD `63f0c87`.

---

## 3. SHA provenance correction 🔎 (new — the one real finding of this re-dispatch)

The prior notes cite SHAs that are **no longer reachable from `main`**. History was rewritten (amend/
rebase) *after* those notes were committed, so the cited SHAs still exist as dangling objects but point
at the *wrong* commit (or have been superseded by a same-message rewrite). A future agent running
`git show <cited-sha>` gets a misleading result. **The inventory itself is unaffected** — only the
provenance citations drifted. Mapped to real reachable SHAs:

| Note | Cited SHA | What it actually is (`git log -1 --format='%s'`) | Real reachable SHA for the claim |
|---|---|---|---|
| `bf-1q3m-consolidated.md` header | `1a26c12…` "verified at `1a26c12`" | `docs(bf-1bmg): inventory JS/TS dashboard blob-creation sites` ← **bf-1bmg, NOT bf-1q3m** | `f144aad` (`docs(bf-1q3m): consolidate…`) |
| `bf-1m2x-verification.md` §1 | `0ae0c03…` "current HEAD" | `docs(bf-1q3m): consolidate…` ← an **amended-away** copy of the bf-1q3m commit (same message, different SHA than `f144aad`) | HEAD at that time was effectively `f144aad`; current HEAD is `63f0c87` |
| `bf-5cgc-handoff.md` header | `1ecc999…` "verified at `1ecc999`" | `docs(bf-5ywk): banner superseded…` | `1ecc999` **is** reachable (a valid ancestor) ✓ |

**Reachable verification chain on `main` (post-rewrite, as pushed):**
`2b44ff6 (bf-1bmg) → f144aad (bf-1q3m) → 64a2202 (bf-1m2x) → 1ecc999 (bf-5ywk) → e27fafe (bf-5cgc) → 21c702f (bf-5c checkpoint, rewritten) → <this note>`. The `63f0c87` cited throughout this note's working-tree greps is the **pre-rewrite twin** of `21c702f` — same message, byte-identical tree (`git diff 63f0c87 21c702f` is empty), different SHA. (The rebase that landed this note adopted the remote `21c702f`.)

**Decisive empirical fact (independent of SHAs):**
`git diff --stat f144aad HEAD -- mothership/ cmd/ dashboard/ firmware/` → **empty**.
I.e. **no tracked Go/JS/firmware source changed across the entire `f144aad..HEAD` range** — every
commit since the inventory was consolidated is `docs`/`.beads` only. Combined with the exact grep
counts above, this proves the inventory could not have moved. (The §2 greps are the authoritative
check; the SHA mapping here is just to stop a future agent trusting unreachable citations.)

---

## 4. bf-4bhd acceptance criteria

- [x] **All blob creation sites identified and listed** — `notes/bf-1q3m-consolidated.md` §3 (5 primary
  + 16 projection/boundary + 2 JS production = 23 production sites).
- [x] **Each site documented with file path and line number** — §2 of this note re-confirms every
  `file:line` is exact at HEAD `63f0c87`.
- [x] **Creation pattern noted** — §3.1/§3.2 of the report (`&Blob{}` pointer / `Blob{}` value /
  `TrackedBlob{}` value / projection literal / anon-struct / JS object literal).
- [x] **Report ready for the next bead to use** — `notes/bf-5cgc-handoff.md` formalizes it into a
  site→owner-bead handoff (Tier-1 → bf-5151 / bf-64h5), with the reference population pattern
  (`analytics.TrackUpdate` @ `main.go:2271`) to copy.

---

## 5. Recommendation for the next dispatcher

bf-4bhd's scope ("find all blob creation code paths") is **fully delivered** by the closed chain
(bf-1q3m → bf-1m2x → bf-5cgc). Re-dispatching bf-4bhd again will only produce another verification
note. The actionable remaining work is the **Tier-1 identity-leak implementation** owned by bf-5151
(Go: automation/explainability/volume) and bf-64h5/bf-1wvm (dashboard TS) — see
`notes/bf-5cgc-handoff.md` §0. Direct future re-dispatches there, not here.
