# Pack Verification Report

**Report type:** Git pack / object-database verification report
**Repository path:** `/home/coding/spaxel`
**Generated (RFC3339):** 2026-09-03T04:50:30Z
**Git HEAD at verification:** _TBD — set when section content is filled_
**Chain:** spaxel-bda3f5f6 (umbrella) → spaxel-eb8920c3 (MIDX capture, published 88afaa4c) → spaxel-05d5d50b (this structure)

> **Status: TEMPLATE.** This file carries the report structure only — every section
> below is a placeholder pending the content-fill step. All measurement data is
> already staged and committed under `.beads/diagnostics/pack-verification/`;
> content is filled from those artifacts, not by re-collection.

## 1. Executive Summary

_Placeholder — one-paragraph verdict at content fill: overall object-database
integrity (pass/fail), corruption found (yes/no), dangling object totals by
type, and MIDX applicability for the current pack layout._

## 2. Repository & Environment Snapshot

**Captured (RFC3339):** _TBD_

_Placeholder — git HEAD, branch state, `git count-objects -v` statistics
(object counts, pack count, size-pack), and refs summary at capture time._

- Source: `.beads/diagnostics/pack-verification/verification-summary.txt`
- Source: `.beads/diagnostics/pack-verification/count-objects-verbose.txt`

## 3. Git fsck — Full Integrity Scan

**Captured (RFC3339):** _TBD_

_Placeholder — `git fsck --full` exit status, error/corruption line count, and
dangling line count. Quote only anomaly lines, not the full output._

- Source: `.beads/diagnostics/pack-verification/git-fsck-full.txt`

## 4. Pack Index Validation (`git verify-pack`)

**Captured (RFC3339):** 2026-09-02T10:02Z (vintage of `pack-directory-list.txt`; pre-gc — see §4.2 and Appendix A)

### 4.1 Pack file inventory — as captured (3-pack layout)

All nine pack files are present and complete: 3 × (`.pack` + `.idx` + `.rev`),
zero garbage, zero stray files. Sizes are exact bytes as listed in
`pack-directory-list.txt`; object counts and delta depths are from
`pack-analysis-summary.txt`, cross-checked against the `per_pack` block of
`verify-pack-corruption-indicators.json`. Rows are sorted by `.pack` size.
In every triplet the `.rev` file shares its `.idx` mtime, so the mtime column
shows `.pack` vs `.idx`+`.rev`. All times are server-local EDT, as captured.

| Pack | `.pack` (B) | `.idx` (B) | `.rev` (B) | Objects | Max Δ depth | mtime EDT (.pack / .idx+.rev) | Status |
|---|---:|---:|---:|---:|---:|---|---|
| `6e51fdce` | 212,470,544 (202.63 MiB) | 397,216 | 56,644 | 14,148 | 29 | Aug 28 00:10 / Jun 27 08:04 | ✅ OK — largest |
| `fd6e34f4` | 150,043,947 (143.09 MiB) | 187,860 | 26,736 | 6,671 | 37 | Aug 29 10:14 / Aug 27 21:08 | ✅ OK |
| `4c6a0d90` | 6,145,745 (5.86 MiB) | 55,168 | 7,780 | 1,932 | 19 | 2026-09-02 02:04 / 02:04 | ✅ OK — most recent |
| **Total (3 packs)** | **368,660,236 (351.58 MiB)** | **640,244** | **91,160** | **22,751** | 37 max | — | **3/3 ✅** |

**Status legend.** ✅ OK = `verify-pack -v` ran to its `ok` terminator with zero
checksum failures, zero error lines, offsets strictly ascending and within pack
bounds, and the reported non-delta count matching the parsed one — all checks
true for all three packs per `verify-pack-corruption-indicators.json`
(`packs_not_ok`, `corrupt_packs`, `error_lines` all empty). ⚠️ = anomaly raised
(0 in this capture). ❌ = corrupt pack / failed checksum (0 in this capture).

**Aggregates**

- **Total pack storage (all `.pack` files):** 368,660,236 B = **351.58 MiB** across 3 packs.
- **Index storage (all `.idx`):** 640,244 B (0.61 MiB); **reverse-index storage (all `.rev`):** 91,160 B (0.09 MiB).
- **Entire pack directory** (3 `.pack` + 3 `.idx` + 3 `.rev` + `multi-pack-index` at 638,296 B): 370,029,936 B = 352.89 MiB.
- **Cross-check:** Σ(`.pack` + `.idx`) = 369,300,480 B = **352.19 MiB**, reproducing the
  `git count-objects -v` figure `size-pack: 352.19 MiB` exactly (`size-pack` counts `.pack`+`.idx`
  and excludes `.rev` and the MIDX) — the inventory is internally consistent with the §2 statistics.
- **Largest pack:** `6e51fdce` — 212,470,544 B `.pack` (202.63 MiB), **57.63 %** of total pack
  storage, holding 14,148 of 22,751 in-pack objects (62.2 %).
- **Most recent pack:** `4c6a0d90` — the only triplet written in one stroke (all components
  2026-09-02 02:04 EDT, alongside the then-present `multi-pack-index`); the other two carry
  Jun/Aug mtimes.
- **Deepest delta chain:** 37 (`fd6e34f4`); each pack's chain-length histogram sums exactly to its
  delta-object count (831 / 5,859 / 4,215).
- **mtime observation — no action:** `6e51fdce` and `fd6e34f4` carry `.pack` mtimes months/days
  newer than their `.idx`/`.rev`, i.e. those `.pack` files were touched or rewritten without an
  index rewrite. Benign at capture: `verify-pack` validates content and checksums, not timestamps.
- The `multi-pack-index` present at capture is §5's subject; the authoritative post-gc verdict
  supersedes the capture-era "Present / PASSED" summary lines (Appendix A).

### 4.2 Pack file inventory — at content fill (1-pack layout)

Read-only listing taken at fill time (**2026-09-03T05:23:41Z**, not a staged
artifact): the 2026-09-02T21:59Z gc repacked the three captured packs into one.

| Pack | `.pack` (B) | `.idx` (B) | `.rev` (B) | mtime (UTC) | Status |
|---|---:|---:|---:|---|---|
| `b8585481` | 241,430,341 (230.25 MiB) | 568,800 | 81,156 | 2026-09-02T21:59:38–39Z (all three, gc instant) | ✅ triplet complete · fsck-passed · verify-pack not re-run |
| **Total (1 pack)** | **241,430,341 (230.25 MiB)** | **568,800** | **81,156** | — | **1/1 complete** · MIDX absent (§5) |

- **Total pack storage now:** 241,430,341 B = **230.25 MiB** — the repack shed 127,229,895 B
  (−34.5 %) of `.pack` bytes versus the captured layout while holding the 20,276 in-pack objects.
- **Largest = most recent = only pack:** `b8585481`.
- **Status basis:** this layout has no staged `verify-pack -v` run of its own; its integrity record is
  the zero-mutation `git fsck --full` spot-check (exit 0, zero corruption/missing/error lines) run at
  the umbrella bead's close against this exact single-pack directory (see §3).

**Which source wins:** §4.1 is the vintage record the chain's verify-pack verdict was rendered
against (point-in-time capture semantics — Appendix A); §4.2 is the live layout at fill time and
wins for any forward-looking statement about the object database.

- Source: `.beads/diagnostics/pack-verification/verify-pack-output.txt` (1.9 MB verbatim `verify-pack -v`)
- Source: `.beads/diagnostics/pack-verification/pack-analysis-summary.txt` (per-pack objects / delta histograms)
- Source: `.beads/diagnostics/pack-verification/pack-directory-list.txt` (exact byte sizes and mtimes)
- Source: `verify-pack-corruption-indicators.json` (repo root — per-pack cross-check: object counts, offsets, checksum verdicts)
- Fill-time listing: `ls --full-time .git/objects/pack/` at 2026-09-03T05:23:41Z (§4.2 only, read-only)

## 5. Multi-Pack-Index (MIDX) Status

**Captured (RFC3339):** _TBD_

_Placeholder — whether `multi-pack-index` exists for the current pack layout,
`git multi-pack-index verify` result, and applicability note (vacuous pass when
a single pack makes a MIDX moot)._

- Source: `.beads/diagnostics/pack-verification/multi-pack-index-verify.txt`
- Source: `.beads/diagnostics/pack-verification/multi-pack-index-verify-verbose.txt`

## 6. Dangling Objects Analysis

**Captured (RFC3339):** _TBD_

_Placeholder — total dangling count, breakdown by type (blob/commit/tree),
representative object IDs, and disposition (reachable via reflogs, safe to
prune, or evidence of an interrupted operation)._

- Source: `.beads/diagnostics/pack-verification/dangling-objects.log`
- Source: `.beads/diagnostics/pack-verification/dangling-count.txt`
- Source: `.beads/diagnostics/pack-verification/dangling-by-type.txt`

## 7. Corruption Indicators

**Captured (RFC3339):** staged indicators 2026-09-02T10:01Z → 2026-09-03T04:10Z;
fill-time re-verification 2026-09-03T06:00–06:01Z at HEAD `5d5380e0` (single-pack
layout, zero mutation — read-only verbs only). Each row states its own vintage.

Consolidated corruption indicators across every verification layer in this
chain, one status per layer. This is the compile view: per-layer detail lives in
§3 (fsck), §4 (pack index), §5 (MIDX), §6 (dangling); the machine-readable
per-pack mirror is `verify-pack-corruption-indicators.json` (repo root).

### 7.1 Status legend

| Marker | Meaning |
|---|---|
| ✅ PASS | Layer ran to completion and reported zero corruption indicators |
| ⚠️ BENIGN | Non-zero finding that is documented churn, not corruption |
| ⚪ N/A | Layer does not apply to the current layout (nothing to verify) |
| ❌ FAIL | Corruption confirmed (0 occurrences in this chain) |

### 7.2 Consolidated status matrix

| # | Verification layer | Status | Key numbers | Vintage | Source |
|---|---|---|---|---|---|
| 1 | Pack integrity — `git verify-pack -v` | ✅ PASS | 3/3 packs terminate `: ok`; 0 checksum failures; 0 corrupt packs; 0 unresolvable delta bases; 0 malformed lines; 22,751/22,751 unique OIDs parsed; offsets strictly ascending and in bounds | 2026-09-02T10:02Z (pre-gc 3-pack layout — point-in-time, §4.1) | `verify-pack-output.txt`; `verify-pack-corruption-indicators.txt`/`.json` |
| 2 | Object-graph integrity — `git fsck --full` | ✅ PASS | exit 0; 0 error / missing / broken-link lines (stderr empty); 215 `dangling` lines — the file's only content type | 2026-09-02T10:01Z @ HEAD `3405d08e` | `git-fsck-full.txt` |
| 3 | fsck re-run at fill | ✅ PASS | exit 0; stderr 0 lines (no corruption/missing/error); 19 dangling (12 commit / 4 blob / 3 tree) | 2026-09-03T06:00:12Z @ HEAD `5d5380e0` | live run this section (zero mutation) |
| 4 | Pack index (`.idx`) structural check | ✅ VALID | 20,276 objects; fanout monotonic; OID table sorted; self-checksum valid; index checksum cross-matches pack filename and pack trailer; offset span 12…241,430,246 all within pack bounds; delta-chain histogram max depth 50 | 2026-09-03T04:10Z (single-pack `b8585481` capture, spaxel-eb8920c3) | `multi-pack-index-verify.txt` (offsets block); commit `1a6f16be` |
| 5 | Multi-pack-index | ⚪ N/A — ABSENT | no base MIDX, no incremental chain, no per-layer `.midx`; `git multi-pack-index verify` exit 0 on an absent MIDX is **vacuous** (control-tested on a fresh empty repo, git 2.54) — recorded ABSENT / NOT APPLICABLE, never PASSED; re-confirmed absent 2026-09-03T06:00:12Z | verdict 2026-09-03T04:13Z, published `88afaa4c`; absence live-confirmed at fill | `multi-pack-index-verify{,-verbose}.txt` (⚠️ pre-deletion vintage — see below); §5 |
| 6 | Dangling objects | ⚠️ BENIGN | capture 215 (168 commit / 40 tree / 7 blob) → published re-run 9 (7 commit / 2 tree) @ 02:59:05Z → fill re-run 19 (12 c / 4 b / 3 t); unreferenced-but-present rebase/stash/merge residue; pool emptied to 0 at the 21:59Z gc | per-row vintages as listed; trajectory in `dangling-results.txt` | `dangling-objects.log`; `dangling-count.txt`; `dangling-by-type.txt`; `dangling-results.txt` (repo root) |
| 7 | References (refs) | ✅ PASS | staged: 4 branches / 0 tags / HEAD `3405d08e` @ 2026-09-02T10:06Z; at fill: 2 local branches (`main`, `backup/local-lineage-pre-reconcile`), 0 tags, 3 remote-tracking, 1 stash — **all resolve** (`cat-file -e` OK on every ref target); HEAD symref `refs/heads/main` → `5d5380e0` valid; main reflog intact (38 entries); fsck emitted zero ref errors (§ row 2–3) | staged 2026-09-02T10:06:29Z; live 2026-09-03T06:01:07Z | `verification-summary.txt` (REFS block); live enumeration this section |
| 8 | Loose/garbage objects | ✅ NONE | garbage 0, size-garbage 0 B, prune-packable 0 at both vintages (loose 85 @ capture → 258 @ fill — ordinary accumulation, zero corruption) | 2026-09-02T10:06Z and 2026-09-03T06:00:12Z | `count-objects-verbose.txt`; live `git count-objects -v` |

**Reading the matrix:** rows 1–4 and 7–8 are the corruption-bearing layers and
all pass with zero indicators; row 5 is vacuous for a one-pack repository; row 6
is the only non-zero count anywhere in the chain and is expected churn (§6).

### 7.3 Corruption-vocabulary scan — 0 hits

Word-boundary scan of the full 22,842-line `verify-pack -v` output for the
corruption vocabulary (`corrupt`, `checksum error`, `fatal`, `error`,
`missing`, `unreadable`, `premature end`, `SHA1 COLLISION`): **0 matches**
(`verify-pack-corruption-indicators.txt` §3). The fsck capture contributes 0
non-dangling lines (`git-fsck-full.txt` is 215 lines, all `dangling …`), and the
fill-time re-run contributes 0 stderr bytes. No layer produced a single
corruption-vocabulary line at any vintage.

### 7.4 Dangling ≠ corruption — interpretation rule

Every dangling count in this report is **unreferenced-but-present**: the object
exists, parses, and its checksum verifies; only the reference to it is gone.
fsck classifies dangling lines as informational (stdout), not errors (stderr) —
in both captures stderr is empty. The pool is publish/stash/merge residue:
`dangling-results.txt` traces the fill-era objects to dropped stash commits and
index snapshots from per-deliverable publish windows, and the pool emptied to 0
at the last gc (spaxel-af4d54a6 capture, 00:57:11Z). Disposition: reachable via
reflogs until the next gc, then prunable — no action required, no data loss.

### 7.5 MIDX staged-artifact vintage caveat

`multi-pack-index-verify.txt` / `-verbose.txt` (executed 2026-09-03T04:10:35Z)
show a **real** verify pass — OID-order 20,275 / sort+offsets 20,276, exit 0 —
which requires a MIDX that existed at that instant. That MIDX was subsequently
lost to a failed `git multi-pack-index write --bitmap` whose abort path deleted
the base file (incident recorded 2026-09-03); it has been absent at every
observation since (05:23:41Z listing, published verdict `88afaa4c`, umbrella
re-close spot-check, and the 06:00:12Z fill check). The staged artifact is
therefore a pre-deletion vintage of a MIDX that no longer exists and is not
missing anything the repository needs — a single-pack layout is fully served by
its `.idx` (row 4), which is why the status is ⚪ N/A rather than ❌. The
authoritative verdict is the published ABSENT / NOT APPLICABLE one (§5).

### 7.6 Machine-readable mirror

`verify-pack-corruption-indicators.json` (repo root) mirrors rows 1, 2 and 6
per-pack: `packs_not_ok`, `corrupt_packs`, `error_lines` all empty arrays;
`per_pack` object counts and delta histograms; `dangling` totals by type. It is
the parse output of the 2026-09-02T12:39:42Z compilation (spaxel-6ba4e5cb,
published `49bc049b`) and is the artifact
downstream consumers should read instead of re-parsing the 1.9 MB raw output.

### 7.7 Verdict

**NO CORRUPTION DETECTED** — at every vintage in this chain: 0 corrupt packs,
0 missing objects, 0 checksum failures, 0 unresolvable delta bases, 0
unreadable entries, 0 broken refs, 0 garbage files. The object database is
internally consistent across all layers, and the only moving indicator (the
dangling pool, row 6) is documented churn with zero corruption signal. Two
open layout notes carry forward, neither a defect: the MIDX is absent and not
applicable to the single-pack layout (§5, §7.5), and `verify-pack -v` has not
been re-run against the post-gc single-pack layout — its integrity record there
is the zero-mutation fsck pair (rows 2–3), per §4.2.

- Source: `verify-pack-corruption-indicators.txt` (repo root)
- Source: `verify-pack-corruption-indicators.json` (repo root)
- Source: `.beads/diagnostics/pack-verification/git-fsck-full.txt`
- Source: `.beads/diagnostics/pack-verification/dangling-objects.log`, `dangling-count.txt`, `dangling-by-type.txt`
- Source: `.beads/diagnostics/pack-verification/multi-pack-index-verify.txt`, `multi-pack-index-verify-verbose.txt`
- Source: `.beads/diagnostics/pack-verification/count-objects-verbose.txt`, `verification-summary.txt` (REFS block)
- Source: `dangling-results.txt` (repo root, published 0a17123b)
- Fill-time re-verification: `git fsck --full`, `git count-objects -v`, `git for-each-ref`, `ls .git/objects/pack/` at 2026-09-03T06:00–06:01Z (read-only, this section)

## 8. Raw Data Staging Inventory

**Captured (RFC3339):** _TBD_

_Placeholder — table of every staged artifact with size and capture vintage,
so each quoted number in sections 2–7 is traceable to its file and timestamp._

- Source: `.beads/diagnostics/pack-verification/staging-index.txt`

## 9. Conclusions & Recommendations

_Placeholder — final integrity verdict, list of anomalies requiring action,
recommended remediations (gc/repack, MIDX write, prune policy), and residual
risks left open._

## Appendix A: Data-Vintage Caveats

_Placeholder — records which staged artifacts predate repo mutations (notably
the 2026-09-02T21:59Z gc that collapsed 3 packs / 22,751 objects into
1 pack / 20,276). Where an artifact is stale relative to a later published
verdict (e.g. the pre-gc MIDX "Present / PASSED" lines in
`verification-summary.txt` vs. the authoritative MIDX-absent verdict published
for spaxel-eb8920c3), this section states which source wins. Point-in-time
capture semantics apply to a staging bead: a capture reflects the repo state
at its own timestamp, not at read time._
