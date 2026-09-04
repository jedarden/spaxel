# go.work state and on-disk module-dir inventory — verified 2026-09-04

**Bead:** spaxel-2a7336ca (split-child #3 of bf-2tk7 → spaxel-eca58056)
**Scope:** read-only verification. No code or config was changed by this bead.
**Supersedes two claims in `ci-test-sim-reference-map.md` (2026-07-04)** — see §4.

## 1. go.work — verbatim, and the task's premise is FALSE

`git show HEAD:go.work` is byte-identical to the working tree (`git diff --stat HEAD -- go.work`
is empty), so this is the committed state, not a local edit:

```
go 1.25.0

use ./mothership

use (
	./cmd/sim
	./test/acceptance
)
```

**THREE use directives, not two.** The dispatch premise "go.work currently lists exactly
`./mothership` and `./test/acceptance`" is false as of 2026-09-04.

Machine-checked:

```
$ go work edit -json | jq -c '{Go, Use: [.Use[].DiskPath]}'
{"Go":"1.25.0","Use":["./mothership","./cmd/sim","./test/acceptance"]}

$ go list -m          # workspace mode, from repo root
github.com/spaxel/mothership
github.com/spaxel/sim
github.com/spaxel/acceptance
```

All three use-directives resolve to real module roots.

## 2. Root `cmd/sim` — NOT removed; this premise is FALSE too

Both halves of the "root cmd/sim was removed" premise fail:

| Check | Result |
|---|---|
| `git status` shows `cmd/sim/{go.mod,go.sum,main.go}` deleted | **No.** No `cmd/sim` entry at all. |
| `./cmd/sim` in go.work | **Yes** — it is listed. |
| `git diff --stat HEAD -- cmd/sim go.work` | Empty — identical to HEAD. |
| Files tracked at HEAD | `cmd/sim/go.mod`, `cmd/sim/go.sum`, `cmd/sim/main.go` — all present. |

**Never deleted in any commit.** The only `--diff-filter=D` commit touching `cmd/sim` is
`03f2b900` ("chore: remove remaining compiled binaries"), which deleted the *compiled binary*
`cmd/sim/spaxel-sim` — not the sources. Latest source change is `47233521` ("feat(sim):
provision real per-node HMAC token by default (bf-4mle6)").

### Why the reference map says otherwise

`ci-test-sim-reference-map.md` (2026-07-04) records "root `cmd/sim` is **staged-deleted**
(`git status`: `D cmd/sim/{go.mod,go.sum,main.go}`)". That was a **working-tree-only,
never-committed** deletion. It has since been lost (tree restored from HEAD), which restored
the three files. The committed history never carried the deletion. Everything else in that
note's table still holds; only the `cmd/sim` row's "Exists on disk: No" and the
"REMOVE — commit the deletion" decision are stale.

## 3. Directory inventory (on-disk, 2026-09-04)

| Candidate dir | Exists | Own `go.mod` | Key files |
|---|---|---|---|
| `cmd/sim` (root) | **YES** — not gone | **YES** — `module github.com/spaxel/sim`, `go 1.25.0`, requires `gorilla/websocket v1.5.3` | `go.mod` (85 B), `go.sum` (175 B), `main.go` (29,150 B). Plus gitignored build artifact `sim` (`.gitignore:46` `/cmd/sim/sim`) |
| `mothership/cmd/sim` | YES | no — part of `github.com/spaxel/mothership` | `generator.go`, `main.go`, `main_test.go`, `scenario.go`, `verify.go`, `walker.go`, `Makefile`, `README.md` |
| `test/acceptance` (root) | YES | **YES** — `module github.com/spaxel/acceptance`, `replace github.com/spaxel/mothership => ../../mothership` | `acceptance_test.go`, `as1_setup_test.go`…`as7_auth_reject_test.go` (7), `integration_test.go`, `diagnostics.go`, `go.mod`; also `CHANGES_SUMMARY.md`, `DIAGNOSTICS.md`, `run_with_diagnostics.sh` |
| `mothership/test/acceptance` | YES | no — part of mothership module | `as1_first_time_setup_test.go`…`as7_auth_reject_test.go` (8, incl. `as5_wifi_restart_race_test.go`), `integration_test.go`, `io_install_upgrade_test.go`, `test_helpers.go` |
| `tests/e2e/run.sh` (root) | YES | n/a — shell harness | `run.sh` (13,877 B, executable); only file in the dir |
| `mothership/tests/e2e/e2e_test.go` | YES | n/a — part of mothership module | `e2e_test.go` (50,707 B), `assertions_test.go`, `io6_gate_test.go`, `io6_gate_conclusion_test.go` |

Only **two** of the six have their own `go.mod`: root `cmd/sim` and root `test/acceptance`.
Both mothership-prefixed trees are packages of the mothership module.

## 4. Which acceptance dir CI uses

- **`mothership/test/acceptance` is the CI-exercised one.** `spaxel-build` runs
  `go test ./test/acceptance/` with cwd `repo/mothership` → resolves to the in-module dir.
  The in-repo debug workflow `acceptance-test-hang-workflow.yml` does the same: `cd mothership`
  then `go test ./test/acceptance/...`.
- **Root `test/acceptance` is a go.work member but is not CI-exercised.** It is a standalone
  module (`github.com/spaxel/acceptance`) that must be run explicitly (`cd test/acceptance &&
  go test ./...`). Same shape as before — unchanged from the reference map.
- **Known CI red, not a verdict on the directory:** `spaxel-e2e`'s `go-test` leg runs
  `./mothership/test/acceptance/...` from cwd `mothership/`, resolving to the nonexistent
  nested path `mothership/mothership/test/acceptance` → exit 1 on every run. Documented in
  `ci-e2e-template-runsh-audit.md` and in the acceptance-suite diagnosis memory.

## 5. Functional verification (all three modules build and vet clean)

```
$ cd mothership   && go vet ./...          → exit 0
$ cd cmd/sim      && go vet ./...          → OK
$ cd test/acceptance && go vet ./...       → OK
$ cd mothership   && go test ./...         → exit 0; 58 packages ok, incl.
                                             github.com/spaxel/mothership/cmd/sim,
                                             …/test/acceptance, …/tests/e2e
```

Caveat worth recording: **`go test ./...` run from the repo root fails** (exit 1,
`pattern ./...: directory prefix . does not contain modules listed in go.work or their
selected dependencies`) because the workspace root is not itself a module. Scoped patterns
work from root (`go list ./cmd/sim/...` → `github.com/spaxel/sim`). Always run the
whole-module wildcard from inside `mothership/` — same root cause as the golangci-lint
root-vs-`mothership/` trap. The vet/test runs above include the uncommitted in-flight edits
present in the working tree at the time (`mothership/cmd/mothership/main.go`,
`internal/config/config.go`, `internal/fleet/handler.go`, `internal/fleet/handler_test.go`);
they are not this bead's work and were left untouched.

## 6. Consequence for the parent (spaxel-eca58056)

The reference map's **REMOVE root `cmd/sim`** decision has lost its precondition: there is no
staged deletion to commit. Root `cmd/sim` is now a live, tracked, go.work-registered,
vet-clean standalone module. Re-affirming removal is a deliberate new change that also has to
carry the doc/config sync that note already lists (README L33/L76, `.golangci.yml` L139/L144,
`PROGRESS.md`, and `docs/plan/plan.md`'s Go Module Layout, which still describes the
three-module workspace). Deciding that is the parent's call, not this verification bead's.
Nothing was removed and `go.work` was not edited here.
