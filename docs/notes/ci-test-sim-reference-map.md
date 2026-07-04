# CI & go.work Reference Map — test/sim trees

**Bead:** bf-2tk7 (foundational investigation for parent bf-id5)
**Date:** 2026-07-04
**Purpose:** Definitive map of what CI (`spaxel-build`, `spaxel-e2e` WorkflowTemplates in
`jedarden/declarative-config`) and `go.work` actually reference, so the parent bead removes /
consolidates only what nothing references.

## Method & scope note

The WorkflowTemplates live in `jedarden/declarative-config` (outside this repo). The CI-side
facts below come from the bf-2tk7 task brief's "already-known facts" (read by the brief's
author) and were **verified against the spaxel tree** for every path that exists here:
existence, module boundary, and whether the documented local/CI commands resolve. One item
— the `workingDir` of `spaxel-e2e` line ~72 — cannot be determined from inside this repo and
is flagged for the parent to confirm (see *Suspicious path*).

## go.work (verified)

```
go 1.25.0
use ./mothership
use ./test/acceptance
```

Exactly two modules. **No `./cmd/sim`** — root `cmd/sim` is staged-deleted
(`git status`: `D cmd/sim/{go.mod,go.sum,main.go}`) and there is no root `cmd/` dir. Confirmed.

## Reference map

| Path | Referenced by CI? | In go.work? | Exists on disk? | Decision |
|------|-------------------|-------------|-----------------|----------|
| `cmd/sim` (root) | **No** | **No** | **No** — staged-deleted; no root `cmd/` | **REMOVE** — already removed in working tree; commit the deletion. |
| `mothership/cmd/sim` | **Yes** — `spaxel-build` runs `./cmd/sim` after `cd repo/mothership`; **Dockerfile L97** builds `./cmd/sim` from WORKDIR `/app` after `COPY mothership/ ./`; **`tests/e2e/run.sh` L238–242** mounts `mothership:/src` (`-w /src`) and runs `go build ./cmd/sim`. All three resolve `./cmd/sim` from a **mothership** cwd → `mothership/cmd/sim`. | Yes (part of `./mothership`) | Yes (`generator/walker/scenario/verify/main.go` + `main_test.go`, `Makefile`, `README.md`; **no own go.mod** → in `github.com/spaxel/mothership` module) | **KEEP** — sole CSI simulator; CI-built and image-baked. |
| `test/acceptance` (root) | **No** — `spaxel-build` cds into `mothership` and runs `go test ./...`, which does not cross into the separate root module | **Yes** — `use ./test/acceptance` | Yes — own module `github.com/spaxel/acceptance`, `replace github.com/spaxel/mothership => ../../mothership` | **DOCUMENT** — not CI-exercised; run via documented `cd test/acceptance && go test ./...` (README L79). go.work member — do **not** remove blindly; consolidation candidate only. |
| `mothership/test/acceptance` | **Yes** — `spaxel-build` runs `./test/acceptance` from the `mothership` cd; `spaxel-e2e` line ~72 refs `./mothership/test/acceptance` (see *Suspicious path*) | Yes (part of `./mothership`) | Yes — 9 in-module test files + `test_helpers.go` | **KEEP** — canonical in-module acceptance suite; CI compile-covered and explicitly run. |
| `tests/e2e/run.sh` | **No** — per brief, neither template invokes it | **No** | Yes — 429-line shell harness (Docker-based e2e) | **DOCUMENT** — orphaned from CI and go.work; runs only manually. Parent decision: wire into CI or remove. |
| `mothership/tests/e2e/e2e_test.go` | **No explicit CI path** (per brief) | Yes (part of `./mothership`) | Yes — Go e2e (gorilla/websocket, builds binaries via os/exec) | **KEEP / DOCUMENT** — transitively compile-covered by `spaxel-build`'s `go test ./...` (no build tag; skips at runtime via `testing.Short()` / `SPAXEL_INTEGRATION_TEST=1`). Not named explicitly by any CI path. |

## Suspicious path — `./mothership/test/acceptance/...` at spaxel-e2e ~line 72

> **Decision impact: NONE.** `spaxel-build` explicitly runs `./test/acceptance` from its
> `cd repo/mothership` (→ `mothership/test/acceptance`), so that directory is CI-exercised
> and **KEEP regardless** of how the e2e line-72 step resolves. The question below only
> determines whether the e2e *invocation* is live or a dead ref — it does not change any
> keep/remove decision for the directory itself. Recorded for completeness.

**Structural facts (verified here):**
- `mothership/test/acceptance/` is a **real** directory (9 test files, part of the mothership module).
- Root `test/acceptance/` is **also** a real directory (separate module).
- Both `./test/acceptance` **and** `./mothership/test/acceptance` appear in the e2e template.

**Resolution depends entirely on the step's `workingDir`** (cannot be read from inside this repo):

| workingDir of line-~72 step | `./mothership/test/acceptance` resolves to | Verdict |
|------------------------------|--------------------------------------------|---------|
| repo root (`/tmp/spaxel-src`) | `/tmp/spaxel-src/mothership/test/acceptance` (real) | **LIVE** — tests the mothership in-module acceptance suite. (`./test/acceptance` elsewhere in the template then resolves to the *root* module — two distinct suites.) |
| `/tmp/spaxel-src/mothership` (the cd) | `/tmp/spaxel-src/mothership/mothership/test/acceptance` (does **not** exist) | **BUG** — nested path; `go test` errors / finds no packages. |

**Most-likely verdict:** Because both `./test/acceptance` and `./mothership/test/acceptance`
coexist in the template and resolve to distinct real dirs **only** when cwd = repo root, the
line-~72 step most likely runs from repo root and is **real** — it exercises the mothership
in-module acceptance suite. The "suspicion" is purely that the prefix differs from
`spaxel-build`'s `./test/acceptance` (which is cwd-relative to the `mothership` cd and points
at the *same* `mothership/test/acceptance` dir).

**Confirm for parent (bf-id5):** read `workingDir:` of the `spaxel-e2e` step at ~line 72.
- repo root → live; leave as-is.
- mothership → dead ref; fix to `./test/acceptance` (matching build) or drop.
- **The directory `mothership/test/acceptance/` itself is KEEP regardless** — it is the
  canonical in-module acceptance suite exercised by `spaxel-build`.

## Related doc-debt surfaced (not in scope to fix here)

Verified via `grep -rn 'cmd/sim'` across the repo (excluding `.beads/`, `mothership/cmd/sim`,
traces, node_modules). Beyond the live code paths above, root-`cmd/sim` still appears in
**docs/config that were not updated when root `cmd/sim` was removed**:

- **`README.md` L76** — `go build -o spaxel-sim ./cmd/sim` is now **broken from repo root**
  (root `cmd/sim` removed). Should be `cd mothership && go build -o spaxel-sim ./cmd/sim`
  (or workspace-aware).
- **`README.md` L33** — table row `` [`cmd/sim/`](cmd/sim/) `` is a **dead link** (no root
  `cmd/` dir), and labels the simulator `github.com/spaxel/sim` — a **stale module name**;
  it is now `github.com/spaxel/mothership` (no separate sim module). Should point at
  `mothership/cmd/sim/`.
- **`.golangci.yml` L139, L144** — exclude rules `path: cmd/sim/main\.go` and
  `cmd/sim/scenario\.go` reference the **gone root path**. Update to `mothership/cmd/sim/...`
  (or drop) so the excludes still match when lint runs in the mothership module.
- **`PROGRESS.md` L477, L511** — prose references to `cmd/sim` / `cmd/sim/` for the simulator.
- **`docs/plan/plan.md`** — the *Go Module Layout* section (L3480–3530) and L3527 specifically
  still describe the **three**-module workspace (`mothership`, `cmd/sim`, `test/acceptance`)
  with `cmd/sim/ — Go module (cmd/sim/go.mod)`. That is now **factually stale**: root
  `cmd/sim` is gone, the simulator lives at `mothership/cmd/sim` with no own go.mod, and
  `go.work` has **two** members. L3375, L3666, L3855–3856 carry the same stale
  `go build ./cmd/sim`-from-root framing. **Highest-priority doc-debt for parent bf-id5** —
  the plan is the canonical "where things live" reference and now mis-describes the layout
  this bead family is consolidating.

All of the above belong to the cmd/sim-consolidation child (or a dedicated doc-sync child),
**not** to this investigation bead.

## Summary decisions

- **Remove:** root `cmd/sim` (deletion already staged — commit it).
- **Keep (CI-active):** `mothership/cmd/sim`, `mothership/test/acceptance`.
- **Keep (transitively compile-covered by `go test ./...`):** `mothership/tests/e2e/e2e_test.go`.
- **Document / parent-decide:** root `test/acceptance` (go.work member, not CI-run),
  `tests/e2e/run.sh` (orphaned shell harness).
- **Confirm before acting:** `workingDir` of `spaxel-e2e` line ~72 (see above).

---

## Appendix A — `spaxel-build` line-by-line path inventory (bf-1yeh)

> Split-child #1 of bf-2tk7. Granular per-line evidence underlying the table above. Template
> read end-to-end at `jedarden/declarative-config` →
> `k8s/iad-ci/argo-workflows/spaxel-build-workflowtemplate.yml` (417 lines). Every Go step
> clones the repo to `/tmp/repo` and then `cd`s into `repo/mothership`, so **every `./...`,
> `./cmd/...`, `./test/...`, `./internal/...` path in a Go step resolves UNDER the mothership
> module** (`/tmp/repo/mothership`). Verified against the spaxel tree on 2026-07-04.

### Step working directories

| Step (template) | Clone dest | `cd` to | Effective working dir | Tree |
|-----------------|-----------|---------|-----------------------|------|
| `resolve-version` (L66) | `/tmp/repo` (L77) | `/tmp/repo` (L78) | repo ROOT | root |
| `docker-build` (L117) | n/a — kaniko git context | n/a | repo ROOT (kaniko context root) | root |
| `update-declarative-config` (L155) | `/tmp/dc` (declarative-config repo) | `/tmp/dc` (L170) | **declarative-config**, not spaxel | — |
| `golangci-lint` (L191) | `/tmp/repo` (L210–212) | `repo/mothership` (L213) | `/tmp/repo/mothership` | **mothership** |
| `a11y-test` (L233) | `/tmp/repo` (L250–252) | `repo/dashboard` (L253) | `/tmp/repo/dashboard` | dashboard (Node, non-Go) |
| `go-test` (L272) | `/tmp/repo` (L285–288) | `repo/mothership` (L289) | `/tmp/repo/mothership` | **mothership** |
| `timing-benchmark` (L309) | `/tmp/repo` (L322–325) | `repo/mothership` (L326) | `/tmp/repo/mothership` | **mothership** |
| `acceptance-test` (L365) | `/tmp/repo` (L378–381) | `repo/mothership` (L382) | `/tmp/repo/mothership` | **mothership** |

### Every Go build / test / vet / bench target

| Command | Line | cwd base | Resolves to | Exists? |
|---------|------|----------|-------------|---------|
| `golangci-lint run --timeout 5m ./...` | L217 | `mothership` | `mothership/...` (whole module) | yes |
| `go test -v -timeout 20m ./...` | L291 | `mothership` | `mothership/...` (whole module) | yes |
| `go vet ./...` | L293 | `mothership` | `mothership/...` (whole module) | yes |
| `go test -bench=BenchmarkFusionLoop -benchtime=10s -count=1 ./internal/localizer/fusion/` | L329–330 | `mothership` | `mothership/internal/localizer/fusion/` (`BenchmarkFusionLoop` at `…/fusion/timing_budget_test.go:94`) | yes |
| `go build -v -o /tmp/spaxel-sim ./cmd/sim` | **L385** | `mothership` | **`mothership/cmd/sim`** (NOT root `cmd/sim`) | yes (`mothership/cmd/sim/main.go` + others; **no own go.mod** → in `github.com/spaxel/mothership`) |
| `go build -v -o build/spaxel ./cmd/mothership` | **L392** | `mothership` | **`mothership/cmd/mothership`** | yes (`main.go`, `dashboard_embed.go`, `migrate.go`) |
| `go test -v -timeout 8m ./test/acceptance/` | **L400** | `mothership` | **`mothership/test/acceptance/`** (NOT root `test/acceptance`) | yes (7 `as*_test.go` + `integration_test.go`, `io_install_upgrade_test.go`, `test_helpers.go` = 10 `.go` files) |

### Docker build context / Dockerfile (kaniko, `docker-build` step)

| Item | Line | Value | Resolves to |
|------|------|-------|-------------|
| Build context | L130 | `--context=git://github.com/jedarden/spaxel.git#refs/heads/main` | repo ROOT |
| Dockerfile | L131 | `--dockerfile=Dockerfile` | root `/Dockerfile` (relative to context root) |
| Image tags | L132–133 | `ronaldraygun/spaxel:<version>` + `:latest` | — |
| Build arg | L134 | `--build-arg=VERSION=<version>` | consumed by Dockerfile |

### `run.sh` invocations

**None.** The `spaxel-build` template invokes no shell harness — specifically not
`tests/e2e/run.sh`. (The only scripts are inline `sh -c` step bodies.)

### Explicit no-references (per bf-1yeh acceptance criteria)

| Path | Referenced by this template? | Evidence |
|------|------------------------------|----------|
| root `cmd/sim` | **NO** | The sole `./cmd/sim` ref (L385) runs with cwd=`mothership` → `mothership/cmd/sim`. Root `cmd/sim` is removed (staged-deleted; no root `cmd/` dir). |
| root `test/acceptance` | **NO** | The sole `./test/acceptance/` ref (L400) runs with cwd=`mothership` → `mothership/test/acceptance/`. The separate root module `github.com/spaxel/acceptance` is never touched. |
| `tests/e2e/run.sh` | **NO** | No `run.sh` invocation anywhere in the template. |
| `mothership/tests/e2e/e2e_test.go` | **NO explicit path** | Not named by any target. Only transitively compile-covered by `go test ./...` (L291) since it is a no-build-tag file inside the mothership module. |

### Note on `go test ./...` transitive coverage

The `go test ./...` at L291 (cwd=`mothership`) compiles and runs every package in the
mothership module, which transitively includes `mothership/cmd/sim` (its `main_test.go`),
`mothership/test/acceptance/`, and `mothership/tests/e2e/` — but only `mothership/test/acceptance/`
is *also* named by an explicit path (L400). `mothership/tests/e2e/e2e_test.go` is reached only
via the wildcard, not by an explicit target.
