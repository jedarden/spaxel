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
