# amd64-only workflow submission — spaxel-d1592c49

**Date:** 2026-09-04
**Bead:** `spaxel-d1592c49` (Submit amd64-only workflow to iad-ci)
**How-to reference:** `docs/notes/amd64-only-build-usage.md` §3.1/§3.2 — this is the
execution record of that recipe, not a new recipe.

---

## Result

| What | Value |
|---|---|
| Workflow name (from the API) | **`spaxel-build-amd64only-d1592c49-r9dhs`** |
| Submitted | 2026-09-04 09:47:48Z |
| Finished | 2026-09-04 09:48:08Z (20 s) |
| Phase | `Succeeded` |
| Namespace / cluster / template | `argo-workflows` / `iad-ci` / `WorkflowTemplate/spaxel-build` |
| Parameters | `platforms=linux/amd64`, `image-repo=ronaldraygun/spaxel-amd64verify` (scratch); `git-repo` and `branch` left at defaults |

## Submission command (AC1, AC2)

```bash
kubectl --kubeconfig=/home/coding/.kube/iad-ci.kubeconfig create -f - -o name <<'EOF'
apiVersion: argoproj.io/v1alpha1
kind: Workflow
metadata:
  generateName: spaxel-build-amd64only-d1592c49-
  namespace: argo-workflows
spec:
  arguments:
    parameters:
      - name: platforms
        value: linux/amd64
      - name: image-repo
        value: ronaldraygun/spaxel-amd64verify
  workflowTemplateRef:
    name: spaxel-build
EOF
# → workflow.argoproj.io/spaxel-build-amd64only-d1592c49-r9dhs
```

The scratch `image-repo` is mandatory per the bead's handoff note and
`amd64-only-build-usage.md` §3.2: `update-declarative-config`'s sed is hardcoded to
`ronaldraygun/spaxel:` regardless of the parameter (open bead `spaxel-fce2f7e4`), so a
non-release run must not point at production.
*(Recorded at run time. `spaxel-fce2f7e4` landed later the same day — declarative-config
`0ec96fc9` — so the sed now follows the parameter and a scratch-repo run rewrites nothing
at any version; the scratch repo stays mandatory for the §5 same-tag hazard.)*

## Outcome: parameter override accepted, image build gated off

Argo accepted the submission and resolved both overrides (`platforms` consumed by
`docker-build`'s `--platform` flag; `image-repo` by the tags/cache refs). The run
completed `Succeeded` — but **no image was built**, and this is the expected outcome at
this HEAD, not a failure:

- `resolve-version` → `Succeeded`, emitted `should-build=false`.
- All nine gated steps report `Skipped` with `when 'false == true' evaluated false`:
  `lint`, `a11y-test`, `go-test`, `timing-benchmark`, `acceptance-test`,
  `firmware-test`, `build-firmware`, `build` (docker-build), `update-declarative-config`.

`should-build=false` because Forgejo `main`'s top commit (`7828d4d1`) diffs only
`.beads/checkpoint/*` — everything the doc-only exclusion pattern
(`^docs/|^\.beads/|^\.needle|\.md$|^LICENSE$|^\.gitignore$`) ignores. Because the gate
read `false`, no `VERSION` auto-bump was committed, nothing was pushed anywhere, and
production's image pin and multi-arch manifest were untouched. The run was therefore
inert-by-construction — the safe case from §3.2 rule 3.

Pod logs are unrecoverable (`podGC: OnPodCompletion`); the node list above is quoted
from the live `Workflow` object before its TTL reaps it. This also restores a live
inspection target: the doc's previously verified example (`spaxel-build-amd64-verify-28wp5`)
has already been reaped from the cluster.

## Why no real cache-bust build ran (and why forcing one would be harmful)

A build executes only when `should-build=true`, which requires `main`'s top commit to
touch a non-doc path. None of the knobs can force it: the template has no override for
the diff, and the two controllable inputs (`git-repo`, `branch`) can only select another
ref. Manufacturing a substantive commit to flip the gate would trigger the full release
chain — `VERSION` auto-bump pushed to `main`, a sensor-triggered production build, and a
declarative-config pin rewrite — which is precisely what a test submission must not do.
Until `spaxel-fce2f7e4` parameterises the pin-rewrite sed, an amd64-only run against a
scratch repo is only safe while `should-build=false`, so the honest ceiling of this bead
is a verified parameter-override submission, which is what was delivered.
*(Ceiling superseded 2026-09-04 by `spaxel-fce2f7e4` / `0ec96fc9` — it was a property of
the template as it stood on this run's date, not a permanent one.)*

## Acceptance criteria

| AC | Status | Evidence |
|---|---|---|
| 1. kubectl with the iad-ci kubeconfig | met | `--kubeconfig=/home/coding/.kube/iad-ci.kubeconfig create` returned the object name |
| 2. amd64-only parameter | met | `platforms=linux/amd64` stored on the object; consumed by `docker-build`'s `--platform` (not executed — gated, see above) |
| 3. Created and starts running | met | phase `Running` at submit, `Succeeded` 20 s later; `resolve-version` pod ran |
| 4. Workflow name captured | met | `spaxel-build-amd64only-d1592c49-r9dhs` |
