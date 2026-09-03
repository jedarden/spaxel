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

**Captured (RFC3339):** _TBD_

_Placeholder — per-pack `verify-pack -v` verdict, per-pack file/idx inventory
(sizes, mtimes), and any corruption indicators raised during validation._

- Source: `.beads/diagnostics/pack-verification/verify-pack-output.txt`
- Source: `.beads/diagnostics/pack-verification/pack-analysis-summary.txt`
- Source: `.beads/diagnostics/pack-verification/pack-directory-list.txt`

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

**Captured (RFC3339):** _TBD_

_Placeholder — consolidated corruption indicators derived from the verify-pack
output (missing objects, checksum mismatches, unreadable entries), mirrored to
the machine-readable JSON at repo root._

- Source: `verify-pack-corruption-indicators.txt` (repo root)
- Source: `verify-pack-corruption-indicators.json` (repo root)

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
