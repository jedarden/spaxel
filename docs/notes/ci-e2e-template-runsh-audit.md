# CI: `run.sh` reference audit of the `spaxel-e2e` WorkflowTemplate

**Bead:** spaxel-630f1172 (child #3 of bf-2yur, depends on bf-3ws5o)
**Date:** 2026-08-20
**Question:** does the `spaxel-e2e` Argo WorkflowTemplate invoke `run.sh` — and if so,
does the path resolve to the repo-root `tests/e2e/run.sh` or to `mothership/tests/e2e/run.sh`?

## Answer

**Neither. The template contains zero `run.sh` invocations of any form.**

`grep -i 'run\.sh'` over the entire WorkflowTemplate manifest — spec, arguments,
metadata, volumes, sidecars, and the embedded `kubectl.kubernetes.io/last-applied-configuration`
annotation — returns **no matches**. All three steps are fully inline `script.source`
shell; no external shell script of any kind is referenced. The only `.sh` string
in the whole manifest is `dockerd-entrypoint.sh`, the stock command of the
`docker:dind` sidecar (see line evidence below) — an image entrypoint, not an
e2e script invocation.

Consequence for the parent investigation: **the repo-root `tests/e2e/run.sh`
harness (13,877 B, executable, committed 2026-06-27) is dead from CI's
perspective** — nothing in the WorkflowTemplate invokes it. The `docker-e2e`
step reimplements an equivalent flow (clone → build image → run container →
health-check → simulator → curl-based assertions) inline rather than calling
the checked-in harness.

## Source audited

The live template is authoritative for what CI runs, so the audit ran against
the cluster object, not a guess at the declarative-config source:

```
kubectl --kubeconfig=/home/coding/.kube/iad-ci.kubeconfig \
  get workflowtemplate spaxel-e2e -n argo-workflows -o yaml
```

- namespace `iad-ci` / `argo-workflows`, name `spaxel-e2e`
- `metadata.resourceVersion` 62793060, `generation` 5, created 2026-05-27
- ArgoCD-managed (label `argocd.argoproj.io/instance: argo-workflows-ns-iad-ci`,
  i.e. synced from `declarative-config/k8s/iad-ci/argo-workflows/`)
- **Verified byte-identical:** the live `spec` and the spec embedded in the
  `last-applied-configuration` annotation compare equal (parsed-JSON,
  sorted-keys comparison), and `run.sh` is absent from **both**. So the
  declarative-config source at last sync contained the same zero references —
  the finding is not a live-drift artifact.

## Line-number evidence

Line numbers refer to the `kubectl … -o yaml` dump (479 lines); regenerate with
the command above. The manifest has one giant line and a structured tail:

| Line | Content | run.sh? |
|------|---------|---------|
| 6 | `kubectl.kubernetes.io/last-applied-configuration` — full applied spec as one JSON line (all three script sources embedded here) | no occurrence |
| 35 | template `e2e-test` (entrypoint, steps fan-out) | — |
| 37 / 39 / 51 | step refs `go-test`, `acceptance-tests`, `docker-e2e` | — |
| 54 | template def `go-test` (`golang:1.25-bookworm`) | no `run.sh` in source |
| 94 | template def `acceptance-tests` (`golang:1.25-bookworm`) | no `run.sh` in source |
| 277 | template def `docker-e2e` (`golang:1.25-alpine` + dind sidecar) | no `run.sh` in source |
| 466 | `dockerd-entrypoint.sh` — dind **sidecar** command (`docker:dind` image) | the only `.sh` string in the manifest; not an e2e invocation |

Additional negative searches over the same dump, all zero hits: `tests/e2e`,
`test/e2e`, `e2e/run`, `mothership/tests`, `runsh`, `run_sh`.

## Working-directory bases (per step) and what run.sh paths *would* resolve to

There are no invocations to tag, so this table instead fixes each step's CWD
and evaluates the two candidate paths from each base — the resolution analysis
the audit was after. Bases verified against the repo layout; existence checks
run from a local checkout of the same structure as the CI shallow clone
(`/tmp/spaxel-src`).

Every step clones `jedarden/spaxel` to `/tmp/spaxel-src` first.

| Step | CWD base used by the script | `tests/e2e/run.sh` from that base | `mothership/tests/e2e/run.sh` from that base |
|------|------------------------------|-----------------------------------|----------------------------------------------|
| `go-test` | `/tmp/spaxel-src/mothership` (`cd` immediately after clone; never leaves) | `/tmp/spaxel-src/mothership/tests/e2e/run.sh` → **non-existent** (that dir holds only `*_test.go` files) | `/tmp/spaxel-src/mothership/mothership/tests/e2e/run.sh` → **non-existent** (nested `mothership/mothership` doesn't exist in a fresh clone) |
| `acceptance-tests` | `/tmp/spaxel-src/mothership` (re-`cd`'d before each IO test) | same as above → **non-existent** | same as above → **non-existent** |
| `docker-e2e` | `/tmp/spaxel-src/mothership` (build sim) → `/tmp/spaxel-src` (docker build, stays) | from `/tmp/spaxel-src`: `/tmp/spaxel-src/tests/e2e/run.sh` → **exists** (the root harness) — but nothing invokes it | from `/tmp/spaxel-src`: `/tmp/spaxel-src/mothership/tests/e2e/run.sh` → **non-existent** |

Repo-side facts (verified 2026-08-20):

- `tests/e2e/run.sh` — **exists** at repo root (executable, 13,877 B).
- `mothership/tests/e2e/` — exists but contains only Go test files
  (`assertions_test.go`, `e2e_test.go`, `io6_gate_test.go`,
  `io6_gate_conclusion_test.go`); **no `run.sh`**, and git history
  (`--all --diff-filter=A`) shows `mothership/tests/e2e/run.sh` was **never**
  added on any branch. The only `run.sh` the repo has ever contained is the
  root one.

So even if a bare relative `run.sh`-style call were added later, the deciding
factor would be which of the two bases above it ran from: two of the three
steps sit in `/tmp/spaxel-src/mothership`, from which a repo-root-relative
`tests/e2e/run.sh` path silently misses.

## Side observation (same defect class, different path) — for bf-2yur

The `go-test` step has exactly the base-confusion this audit family is probing,
just with a Go package path instead of `run.sh`:

```
cd /tmp/spaxel-src/mothership
...
GO_BUILD_SKIP=1 go test -short -v ./mothership/test/acceptance/...
```

The pattern is written **repo-root-relative** but executed from
`/tmp/spaxel-src/mothership`, so it targets
`/tmp/spaxel-src/mothership/mothership/test/acceptance/...`, which does not
exist in a fresh clone (verified: `go list ./mothership/test/acceptance/...`
from a `mothership/` CWD fails; the nested `mothership/mothership` only exists
on this bench as an untracked local build artifact). The correct spelling from
that base is `./test/acceptance/...` — which is exactly what the
`acceptance-tests` step uses from the same CWD. Worth folding into the parent
bead's fix list.

## Reproduction

```bash
kubectl --kubeconfig=/home/coding/.kube/iad-ci.kubeconfig \
  get workflowtemplate spaxel-e2e -n argo-workflows -o yaml > wft.yaml
grep -in 'run\.sh' wft.yaml          # → no matches
grep -in '\.sh' wft.yaml             # → line 6 only via the annotation JSON? no:
                                     #   only real hit is line 466 dockerd-entrypoint.sh
python3 - <<'EOF'                    # live spec == last-applied, both run.sh-free
import json, subprocess
w = json.loads(subprocess.run(
    ["kubectl","--kubeconfig=/home/coding/.kube/iad-ci.kubeconfig",
     "get","workflowtemplate","spaxel-e2e","-n","argo-workflows","-o","json"],
    capture_output=True, text=True, check=True).stdout)
live = json.dumps(w["spec"], sort_keys=True)
applied = json.dumps(json.loads(w["metadata"]["annotations"]
    ["kubectl.kubernetes.io/last-applied-configuration"])["spec"], sort_keys=True)
print("live == applied:", live == applied)
print("run.sh in either:", "run.sh" in live or "run.sh" in applied)
EOF
```

## Addendum 2026-09-04 — re-verified against the recreated object; invocation → CWD map

Re-derived for bead spaxel-2906589d (only child of closed `spaxel-d3311966`, a
second extraction chain). The object audited above no longer exists in that
form: `spaxel-e2e` was **recreated 2026-08-26** (`creationTimestamp
2026-08-26T19:59:58Z`, now `resourceVersion 71293261`), and its last source
change is `declarative-config` commit `64e3e540` (2026-08-29, "correct Go
image version and install missing dependencies"). The zero-`run.sh` conclusion
**holds** on the new object. Nothing below is carried over from the 2026-08-20
section — every base was re-derived from the current manifest.

### Object identity and negative searches (2026-09-04)

- live `spec` (kubectl, credential-free read endpoint) vs
  `declarative-config/k8s/iad-ci/argo-workflows/spaxel-e2e-workflowtemplate.yml`
  `spec`: **equal** (normalized-JSON, sorted-keys).
- Case-insensitive hit counts over the manifest: `run\.sh` **0**, `runsh` **0**,
  `run_sh` **0**, `tests/e2e` **0**, `test/e2e` **0**.
- Only `.sh` string in the whole manifest: `dockerd-entrypoint.sh`
  (template line 507) — the `docker:dind` sidecar's stock command.
- `workingDir` appears **nowhere** in the manifest (0 occurrences), so no step
  overrides its image's default working directory.

### Working-directory derivation

Each step is an Argo `script` (`command: [sh]` + `source`), so a step's initial
CWD is the base image's own `WORKDIR`. Base images were verified against the
Docker Hub registry config blobs (linux/amd64), not assumed:

| image | registry-verified `WorkingDir` | `Cmd` | `User` |
|---|---|---|---|
| `golang:1.23-bookworm` (`go-test`, `acceptance-tests`) | `/go` | `bash` | unset → root |
| `golang:1.25-alpine` (`docker-e2e`) | `/go` | `/bin/sh` | unset → root |

Every `git clone` targets the absolute path `/tmp/spaxel-src`, so the clone is
CWD-independent; every subsequent base comes from an explicit absolute `cd`.
There are no other volume mounts and no repo mount anywhere in the template.

### Invocation → working directory map

Line numbers are the declarative-config manifest. `run.sh` has no row anywhere
— there is nothing to tag.

**`go-test`** — `golang:1.23-bookworm`, no volumeMounts, deadline 300s

| line | command | CWD at that point | resolves to |
|---|---|---|---|
| 62 | `git clone … /tmp/spaxel-src` | `/go` (image WORKDIR) | absolute target |
| 65 | `cd /tmp/spaxel-src/mothership` | — | **base for the rest of the step; no later `cd`** |
| 69 | `go build -v ./cmd/mothership` | `/tmp/spaxel-src/mothership` | exists |
| 74 | `go build -v ./cmd/sim` | `/tmp/spaxel-src/mothership` | exists |
| 79 | `GO_BUILD_SKIP=1 go test -short -v ./mothership/test/acceptance/...` | `/tmp/spaxel-src/mothership` | `…/mothership/mothership/test/acceptance/...` → **non-existent** (the CWD bug recorded above) |

**`acceptance-tests`** — `golang:1.23-bookworm`, no volumeMounts, deadline 600s

| line | command | CWD at that point | resolves to |
|---|---|---|---|
| 118 | `git clone … /tmp/spaxel-src` | `/go` | absolute target |
| 121 | `cd /tmp/spaxel-src/mothership` | — | CWD base |
| 125 | `go build -o /tmp/spaxel-sim ./cmd/sim` | `/tmp/spaxel-src/mothership` | package exists; output absolute |
| 130 | `go build -o /tmp/spaxel-mothership ./cmd/mothership` | same | same |
| 138 | `TEST_DATA=$(mktemp -d)` | same | absolute path |
| 146 | `/tmp/spaxel-mothership &` | `/tmp/spaxel-src/mothership` | the server's CWD; `SPAXEL_DATA_DIR` and `SPAXEL_BIND_ADDR` are absolute/explicit so data placement does not follow it |
| 204 | `cd /tmp/spaxel-src/mothership` | already there (explicit no-op) | base for all seven `go test` runs |
| 205, 215, 225, 235, 245, 255, 265 | `go test -v -timeout 2m ./test/acceptance/... -run <TEST>` | `/tmp/spaxel-src/mothership` | `…/mothership/test/acceptance/...` → **exists** (the correct spelling, unlike `go-test` line 79) |

**`docker-e2e`** — `golang:1.25-alpine`, deadline 900s, one volumeMount

The step's only mount is `docker-sock` (emptyDir) at `/var/run`, shared with
the `dind` sidecar so the CLI reaches the sidecar daemon at
`/var/run/docker.sock`. It shapes the Docker socket path only — no repo, no
workspace, nothing that moves a path base. The repo arrives by clone into the
container's writable layer, not by mount.

| line | command | CWD at that point | resolves to |
|---|---|---|---|
| 333 | `git clone … /tmp/spaxel-src` | `/go` | absolute target |
| 340 | `cd /tmp/spaxel-src/mothership` | — | CWD base |
| 341 | `go build -o /tmp/spaxel-sim ./cmd/sim` | `/tmp/spaxel-src/mothership` | package exists; output absolute |
| 346 | `cd /tmp/spaxel-src` | — | **final base; no further `cd` for the rest of the step** |
| 347 | `docker build -t spaxel-e2e:test .` | `/tmp/spaxel-src` | build context = repo **root**, so the root `Dockerfile` |
| 352–481 | `docker run/stop/start/rm/logs`, `curl` | `/tmp/spaxel-src` | CWD-independent |
| 431 | `/tmp/spaxel-sim … > /tmp/sim.log &` | `/tmp/spaxel-src` | binary and log both absolute; CWD-independent |

### Where a `run.sh` reference would have to live

`tests/e2e/run.sh` exists only as `/tmp/spaxel-src/tests/e2e/run.sh`. It is
addressable by a relative path from exactly one base in this template —
`docker-e2e`'s final base `/tmp/spaxel-src` — and from no base in the other two
steps, which both sit in `…/mothership`, where `tests/e2e/` does not exist.
`mothership/tests/e2e/` still holds only Go test files and no `.sh`
(re-verified 2026-09-04), so the "non-existent" cell for
`mothership/tests/e2e/run.sh` above still stands. Nothing invokes the root
harness either way.

### Image change since the audit above (path-neutral)

`64e3e540` (2026-08-29) moved `go-test` and `acceptance-tests` from
`golang:1.25-bookworm` to `golang:1.23-bookworm`. `WorkingDir` is `/go` in
every tag this template has used, so **no path base moved**. Flagged as
out-of-scope but adjacent: `mothership/go.mod` declares `go 1.25.0` while those
two steps now run a 1.23 toolchain, which under the default `GOTOOLCHAIN=auto`
has `go` fetch a 1.25 toolchain at build time — a CWD-independent failure mode
that deserves its own bead if a step starts failing on toolchain download.

## Addendum 2026-09-04 (b) — each invocation resolved to a filesystem path

For bead spaxel-e50733ed (split-child of closed spaxel-2906589d, downstream of
spaxel-5494ad08's final report). Where the addendum above derived each step's
CWD base, this one closes the chain's actual question: **for each `run.sh`
occurrence, the absolute path it would resolve to, and whether that path
exists.** Every fact below was re-verified live on 2026-09-04, not cited.

### The invocation set

`spaxel-e2e` at `resourceVersion 71293261` (unchanged since the addendum above;
re-read from the credential-free iad-ci endpoint, and its `spec` still equals
the `declarative-config` manifest normalized-JSON) contains **zero `run.sh`
occurrences** — case-insensitive `run\.sh` 0, `runsh` 0, `run_sh` 0,
`tests/e2e` 0, `test/e2e` 0; only `.sh` string still `dockerd-entrypoint.sh`.
There is therefore **no invocation to tag**: the raw list produced by
spaxel-d3311966 is the empty set, and the table below tags the only thing that
can carry one — each step's final CWD base — with the resolution of both
candidate spellings from that base. `workingDir` remains absent on all four
templates, so the bases stand as derived above.

### Resolution table — valid vs non-existent, per base

Clone root is always `/tmp/spaxel-src`. Existence is judged against a fresh
clone of `origin/main`, whose tree matches the CI clone.

| Step | Final CWD base | Spelling | Resolves to | Exists? | Verdict |
|---|---|---|---|---|---|
| `go-test` | `/tmp/spaxel-src/mothership` | `tests/e2e/run.sh` | `/tmp/spaxel-src/mothership/tests/e2e/run.sh` | **no** | **non-existent** — `mothership/tests/e2e/` holds only `*_test.go` |
| `go-test` | `/tmp/spaxel-src/mothership` | `mothership/tests/e2e/run.sh` | `/tmp/spaxel-src/mothership/mothership/tests/e2e/run.sh` | **no** | **non-existent** — no nested `mothership/mothership` |
| `acceptance-tests` | `/tmp/spaxel-src/mothership` | `tests/e2e/run.sh` | `/tmp/spaxel-src/mothership/tests/e2e/run.sh` | **no** | **non-existent** — same as above |
| `acceptance-tests` | `/tmp/spaxel-src/mothership` | `mothership/tests/e2e/run.sh` | `/tmp/spaxel-src/mothership/mothership/tests/e2e/run.sh` | **no** | **non-existent** |
| `docker-e2e` | `/tmp/spaxel-src` | `tests/e2e/run.sh` | `/tmp/spaxel-src/tests/e2e/run.sh` | **yes** | **valid** — the one base from which the root harness is addressable; nothing invokes it |
| `docker-e2e` | `/tmp/spaxel-src` | `mothership/tests/e2e/run.sh` | `/tmp/spaxel-src/mothership/tests/e2e/run.sh` | **no** | **non-existent** |

### Existence answers, repo side

Checked against the pristine `origin/main` tree (`git ls-tree` / `git cat-file`
— no checkout, so no working-tree or untracked-file noise), and cross-checked
against the local working tree, which agrees:

- `tests/e2e/run.sh` — **yes, exists.** Blob `7cacbbd9`, mode `100755`,
  13,877 B, last touched by `0491965c` (2026-04-07). It is the only `run.sh`
  path in the tracked tree.
- `mothership/tests/e2e/run.sh` — **no, does not exist.** `mothership/tests/e2e/`
  is tracked with exactly four files (`assertions_test.go`, `e2e_test.go`,
  `io6_gate_conclusion_test.go`, `io6_gate_test.go`) and no shell script.
  `git log --all --diff-filter=A -- mothership/tests/e2e/run.sh` is empty, so
  the path was **never added on any branch** — it is not a deleted file that a
  revert would restore, it never existed.
- `mothership/mothership/` — no such directory anywhere in the tree, so the
  doubly-nested spelling is unresolvable from any base.

### No caller anywhere

Beyond the template, a search for anything that actually *executes* the
harness — Makefiles, shell scripts, workflows, YAML, across this repo and every
tracked file of `declarative-config` at HEAD — finds **zero call sites**. (The
only `run.sh` executions in the declarative-config checkout are under
`.claude/worktrees/` throwaway agent copies of `drawrace-ci`, a different
repo's `e2e/phone-smoke/run.sh`.) The root harness is reachable only by a human
running `./tests/e2e/run.sh` from a repo root, per its own `SCRIPT_DIR` /
`PROJECT_ROOT` derivation, which is repo-root-relative and would break from any
other CWD.

### Statement for the final report (spaxel-5494ad08)

**This template references neither `tests/e2e/run.sh` nor
`mothership/tests/e2e/run.sh`** — it contains no `run.sh` reference of any
form. Had it contained one, the two-step base split above means the deciding
variable would be which step it appeared in: `go-test` and `acceptance-tests`
sit in `…/mothership`, where *both* candidate spellings are non-existent, and
only `docker-e2e`'s final base resolves the repo-root spelling to a real file.
