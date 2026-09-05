# Root Exhaust Classification — reference map for the raw result dumps

**Bead:** spaxel-b7905337 (child 1 of 4 of spaxel-1b8df9a3, repo-root exhaust sweep)
**Date:** 2026-09-04
**Scope:** the 24 raw tool-output dumps / one-off probe scripts at the repo root (this
bead only). The durable root `.md` docs (PROGRESS.md, WORKSPACE_STRUCTURE*.md,
SYSTEM_CATALOG.md, …) are children 2 and 3 and are **not** classified here.

## Method

For each file below, a literal-filename search was run over **every tracked file**
(`git grep -n -F -- <name>`), then repeated for the extension-less stems
(`blob_observation`, `window_test`) and for glob shapes (`*-results.txt`,
`*_results.txt`) that a literal grep would miss. Results were filtered to exclude:

- `.git/`, `.beads/` (bead descriptions name these files — guaranteed false positives)
- `mothership/build/`, `node_modules/`
- `.needle.yaml` / `.needle-predispatch-sha` state
- the root exhaust cluster itself (every root-level tracked file except the 13
  newcomer-facing keepers) — the exhaust files cite each other, which is the scratch
  signature, not evidence of use. The 13 keepers were scanned **un**-excluded.

Also checked, not assumed:

- `jedarden/declarative-config` `k8s/iad-ci/argo-workflows/spaxel-build-workflowtemplate.yml`
  and `spaxel-e2e-workflowtemplate.yml` — **zero references** (confirms the dispatching
  agent's verification).
- `mothership/`, `firmware/`, `cmd/`, `test/`, `tests/`, `dashboard/`, `scripts/` —
  scanned directly (see the one finding below; the dispatcher's blanket "nothing under
  mothership/ references any of these" was *almost* right but missed a Go comment).

**Result: no file in this list has a functional reference anywhere.** Nothing is a
build input, nothing is exec'd, nothing is read by code, tests, CI, Docker, or
go.work, and none of the 13 keepers names any of them. Every hit is prose.

## Per-file classification

### Cluster A — git-integrity dumps (11 files)

`git fsck` / `git rev-list` / `git verify-pack` / ref-database inspection output from
the 2026-08-28…09-02 repository-integrity investigation (spaxel-8b55504f and
predecessors). All regenerable from the object database on demand.

| File | Reference check | Decision |
|---|---|---|
| `refs-results.txt` | none outside the cluster | **delete** |
| `idx-results.txt` | none | **delete** |
| `packs-results.txt` | none | **delete** |
| `refdb-results.txt` | none | **delete** |
| `dangling-results.txt` | none | **delete** |
| `fsck-results.txt` | none | **delete** |
| `midx-results.txt` | none | **delete** |
| `corruption-report.txt` | none | **delete** |
| `verify-pack-corruption-indicators.json` | none | **delete** |
| `verify-pack-corruption-indicators.txt` | none | **delete** |
| `pack-verification-report.md` | none | **delete** |

Note: `refs-results.txt` carried an **uncommitted working-tree modification** — a newer
capture (2026-09-02, spaxel-8b55504f) superseding the committed 2026-08-28 snapshot.
Deletion discards both. No information loss: it is a point-in-time re-verification of
ref integrity that reported 0 broken refs / 0 dangling objects, and it can be
regenerated with the same `git for-each-ref` / `git fsck` sequence.

### Cluster B — search/scan output (3 files)

Raw grep/result dumps from code searches across the mothership tree.

| File | Reference check | Decision |
|---|---|---|
| `mothership-matches.txt` (1455 lines) | none | **delete** |
| `mothership-code-search.txt` (327 lines) | none | **delete** |
| `mothership-files.txt` (5399 lines) | prose only — `docs/research/mothership-directory-structure.md:78` cites it as an example of root search scratch | **delete** |

### Cluster C — one-off verification reports (8 files)

Dated point-in-time investigation outputs. Each names its own bead/task and date;
none has been updated since, none is linked from a keeper, and the substantive
findings they recorded have landed in `docs/notes/`, `docs/inventory/`, or the code.

| File | What it is | Reference check | Decision |
|---|---|---|---|
| `gdop_search_results.md` | 2026-08-23 term search for GDOP symbols | none | **delete** |
| `bug_verification_report.md` | NetworkSettingsHandler `configured` flag check (finding: no bug) | `docs/repo-structure.md:333` — listed by name in the §13 "root-level litter" inventory | **delete** |
| `mothership_verification.md` | 2026-08-28 directory-existence check | none | **delete** |
| `declarative-config-verification-report.md` | 2026-08-28 declarative-config structure check | none | **delete** |
| `search-prerequisites-verification.md` | 2026-08-28 spaxel-d189e61e preconditions check | none | **delete** |
| `repository-structure-findings.md` | 2026-09-01 repo-structure findings | none | **delete** |
| `top_level_directories.md` | 2026-08-28 top-level directory catalog (refreshed 09-03) | `docs/research/mothership-directory-structure.md:14` and `:75` — the research doc positions itself against this catalog | **delete** — a `git ls-tree`-regenerable listing, not a maintained doc |
| `rust-test-modules-report.md` | 2026-08-29 Rust test-module inventory | none | **delete** |

### Cluster D — one-off probe scripts (2 files)

| File | Reference check | Decision |
|---|---|---|
| `blob_observation.sh` | `mothership/cmd/sim/verify.go:136` (Go **comment**), `docs/SYSTEM_CATALOG.md:41`, `docs/build-paths-catalog.md:304`, `docs/inventory/source-code-by-type-and-directory.md:297`, `notes/hardware-free-runtime.md:310` | **delete** — see below |
| `window_test.sh` | `docs/SYSTEM_CATALOG.md:43`, `docs/build-paths-catalog.md:305`, `docs/inventory/source-code-by-type-and-directory.md:314` | **delete** — see below |

**`blob_observation.sh` — the verify.go substring is a stale comment, not a dependency.**
`mothership/cmd/sim/verify.go:136` reads `…matching blob_observation.sh's observation
method.` It attributes *where the polling approach came from*; there is no `exec`, no
`os/exec.Command`, no path construction, and no other mention of the script anywhere in
the module. The surrounding code is self-contained: it polls `GET /api/blobs` 12 times at
500 ms and keeps the peak blob count. Verified against the bead's explicit ask.

The script itself is doubly superseded, and says so in its own header: the canonical
acceptance path is `spaxel-sim --verify` (same poll/assert, exits 0/1), and
`scripts/run-sim-local.sh` is documented in `notes/hardware-free-runtime.md:310` as
"the bead-agnostic canonical form that supersedes it for general use".

**`window_test.sh` is superseded by the e2e suite.** It proved (bf-ekr9c) that
`spaxel-sim` connects regardless of `SPAXEL_MIGRATION_WINDOW_HOURS`. That assertion is
now a permanent test: `mothership/tests/e2e/e2e_test.go` §5 "MIGRATION WINDOW
INDEPENDENCE" (`:58-64`), with strict mode wired via
`SPAXEL_MIGRATION_WINDOW_HOURS=0` at `:240`. No invocation path exists — the only
`bash ./blob_observation.sh`-shaped hit anywhere is the script's own usage comment, and
neither script appears in the Dockerfile, compose file, go.work, or CI.

## Keepers verified untouched

`README.md`, `LICENSE`, `VERSION`, `go.work`, `go.work.sum`, `Dockerfile`,
`docker-compose.yml`, `.gitignore`, `.gitattributes`, `.dockerignore`, `.golangci.yml`,
`.needle.yaml`, `.needle-predispatch-sha` — each scanned directly against all 24
filenames: **zero mentions**. None is modified by this bead.

## Follow-ups recorded for child 3 / the parent

No file proved live, so **nothing is moved to child 3's scope for retention.** The
deletions do leave descriptive mentions that will name files no longer present — these
are prose/catalog mentions, not functional references, and are listed so the sibling
beads can reconcile them rather than rediscover them:

1. `docs/inventory/source-code-by-type-and-directory.md:297,314` — inventories both
   scripts under "Shell Scripts (19 files)"; count drops to 17 and the two bullet
   lines go stale.
2. `docs/SYSTEM_CATALOG.md:41,43` and `docs/build-paths-catalog.md:304,305` — catalog
   bullets for both scripts.
3. `docs/research/mothership-directory-structure.md:14,75,78` — positions itself
   against `top_level_directories.md` and cites `mothership-files.txt` /
   `refs-results.txt` / `*-results.txt` as the root scratch set.
4. `docs/repo-structure.md:69,332-333` — §13 "root-level litter" section names
   `*_results.txt`, `verify-pack-corruption-indicators.*`,
   `bug_verification_report.md`; the section stays true but its examples shrink.
5. `mothership/cmd/sim/verify.go:136` — the attribution comment now names a deleted
   script; harmless, but a one-line rewording ("matching the peak-over-window poll the
   `--verify` flag performs") would remove the dangling reference. Left alone here:
   comment-only Go change, out of this bead's scope.

---

## Addendum — child 3 disposition of the durable root `.md` docs (spaxel-2ee3e526, 2026-09-04)

This document classified only the 24 raw dumps. The durable root `.md` docs were
child 3's (`spaxel-2ee3e526`); their per-file record follows. Method: every file was
read, then a literal-filename reference sweep was run over all tracked files
(`git grep -n -F`, excluding `.beads/`, `.needle*`, build dirs, and the root exhaust
cluster itself). README.md, PROGRESS.md, `.needle.yaml`, Dockerfile and
docker-compose.yml referenced **none** of the 19 candidates.

**Moved with `git mv` (history follows) into `docs/notes/`, kebab-case:**
`API_IMPLEMENTATION_STATUS.md` → `api-implementation-status.md`;
`BASELINE_CAPTURE_SUMMARY.md` → `baseline-capture-summary.md`;
`BLE_PERSONID_INVESTIGATION.md` → `ble-personid-investigation.md`;
`console-implementation-status.md` → `console-implementation-status.md`;
`dashboard_discovery_notes.md` → `dashboard-discovery-notes.md`;
`EMPTY_PASSWORD_TEST_RESULTS.md` → `empty-password-test-results.md`;
`MOTHERSHIP_DASHBOARD_ACCESS_FINDINGS.md` → `mothership-dashboard-access-findings.md`;
`MOTHERSHIP_DASHBOARD_DEPENDENCIES.md` → `mothership-dashboard-dependencies.md`;
`MOTHERSHIP_DASHBOARD_LOCATIONS.md` → `mothership-dashboard-locations.md`;
`MOTHERSHIP_DASHBOARD_STARTUP_INVESTIGATION.md` → `mothership-dashboard-startup-investigation.md`;
`MOTHERSHIP_DASHBOARD_STARTUP_PROCEDURE.md` → `mothership-dashboard-startup-procedure.md`;
`PRESENCE_DETECTION_VERIFICATION.md` → `presence-detection-verification.md`.
The two reconciled inventory keepers went to `docs/inventory/` (that directory is the
established home for that kind of file):
`WORKSPACE_STRUCTURE.md` → `workspace-structure.md`,
`rust-source-inventory.md` → `rust-source-inventory.md`.

**Consolidated, then deleted:** `GDOP_COMPUTATION_GUIDE.md`. Its simulator-side
sections (§1–§6, §8, appendix) duplicate `docs/gdop-computation-functions.md`,
`docs/gdop-function-analysis.md`, `docs/gdop-function-signature.md` and
`notes/gdop-computation-catalog.md`, and its specifics had drifted: the appendix cites
`computeGDOPAngular` at gdop.go:114-165 (actual: :296) and §11 places the
`GDOPCalculator` interface in `optimiser.go:13-16` (actual: `healer.go:13-16`). Its
unique accurate residue — the fleet-side integration (§7 before/after pattern, §9
`FleetHealer.GetWorstCoverageZone`/`SuggestNodePosition`, §11 interface and the
9999.0 infinity encoding) — is preserved as a new "Fleet-Side GDOP Integration"
section in `docs/gdop-computation-functions.md`, re-verified against HEAD. §10's
"complete example" was dropped: it sketches a `FleetHealer.SetGDOPImprovementAccessor`
that does not exist; the real accessor is `DiagnosticEngine.SetGDOPImprovementAccessor`
(`mothership/internal/diagnostics/linkweather.go:155`, wired at `main.go:2238`), now
noted in the same section.

**Deleted (stale beyond recovery / duplicate / empty), disclosed per the dispatch:**
- `mothership_location.md` — describes a layout that no longer exists: its tree lists
  root `cmd/` and `test/` (both gone), compiled `mothership`/`sim` binaries (git-ignored
  residue), and claims `go.work` coordinates "multiple modules" (it defines exactly one).
  Superseded by `docs/inventory/workspace-structure.md`.
- `BENCH_HOSTNAME_INFO.md` — strict condensed duplicate of
  `docs/deployment/bench-connectivity.md` (same hostname, IP, port 6080, nc check,
  network path, same 2026-08-28 verification date, same usage notes; it even cites that
  document as its source).
- `PERMISSION_VERIFICATION_SUMMARY.md` — a five-line stub: title, a date, and an empty
  "Verification Scope" heading. Nothing to preserve.
- `CSI_RECORDING_FILES_SEARCH_RESULTS.md` — a raw grep/find search dump (bead
  spaxel-f7b5d01c, 2026-08-29): the same category this document's Cluster B deleted,
  which the child-1 sweep missed. Regenerable; zero references.

**References updated in the same commit:** the `MOTHERSHIP_DASHBOARD_*` sibling
cross-references inside the five moved dashboard docs; `baseline-capture-summary.md`
self-reference and the `presence-detection-verification.md` → baseline link;
`docs/notes/web-serial-chromium-verification-2026-09-04.md` (:26,:95,:110,:118),
`docs/notes/web-serial-esp32s3-enumeration-2026-09-04.md` (:35,:121,:208) and
`docs/notes/mothership-dashboard-startup-verification-2026-09-04.md` (:5, which also
dropped its now-false "(repo root" qualifier);
`docs/SYSTEM_CATALOG.md` (Investigation Records section),
`docs/repo-structure.md` (§13), `docs/research/mothership-directory-structure.md` (§1),
and the relocated `docs/inventory/workspace-structure.md` (root-files block, header
provenance, PROGRESS link). `notes/bf-3f16.md` still names `API_IMPLEMENTATION_STATUS.md`
— that is a closed bead's historical note and was deliberately left untouched.

**Retained at the root, by design:** `README.md` and `PROGRESS.md` (README links
PROGRESS.md twice and `.needle.yaml` lists it in the worker reading list; it is the one
status file a newcomer needs). One untracked root file remains out of scope:
`BUG_VERIFICATION_EMPTY_PASSWORD.md` — an in-flight verification report distinct from
the moved `empty-password-test-results.md`, not owned by this sweep, left alone.
`acceptance-test-hang-workflow.yml` is owned by a separate twin chain.
