# spaxel-build WorkflowTemplate Parameters

Complete parameter reference for the `spaxel-build` WorkflowTemplate in iad-ci.

## Workflow-Level Parameters

These parameters are defined at the workflow level and can be overridden when submitting a workflow:

| Parameter | Description | Default Value | Required |
|-----------|-------------|----------------|----------|
| `git-repo` | Forgejo repository path (format: `owner/repo`) | `jedarden/spaxel` | Optional |
| `image-repo` | Docker image repository for build output | `ronaldraygun/spaxel` | Optional |
| `branch` | Git branch to build from | `main` | Optional |

**Usage Example:**
```yaml
kubectl create -f - <<EOF
apiVersion: argoproj.io/v1alpha1
kind: Workflow
metadata:
  generateName: spaxel-build-
  namespace: argo-workflows
spec:
  workflowTemplateRef:
    name: spaxel-build
  arguments:
    parameters:
    - name: git-repo
      value: "jedarden/spaxel"
    - name: image-repo
      value: "ronaldraygun/spaxel"
    - name: branch
      value: "main"
EOF
```

## Template-Level Parameters

There is exactly **one** template-level (inter-step) parameter: `version`. It has **no
default value and cannot be supplied by the submitter** — it is always produced by the
`resolve-version` step and forwarded to downstream templates as a step argument:

```yaml
arguments:
  parameters:
  - name: version
    value: "{{steps.resolve-version.outputs.parameters.version}}"
```

| Parameter | Description | Default | Required | Source | Consumed By |
|-----------|-------------|---------|----------|--------|-------------|
| `version` | Resolved SemVer string (e.g., `0.2.124`); also emitted as the `should-build` companion output | none — always derived from the repo's `VERSION` file | Required, but internally supplied (never by the submitter) | `resolve-version` output `/tmp/version` | Declared input on `firmware-build`, `docker-build`, `update-declarative-config`, `golangci-lint`, `a11y-test` |

**Note — declared vs. actually read:** `golangci-lint` and `a11y-test` *declare* the
`version` input and receive it as an argument, but neither of their scripts references
`{{inputs.parameters.version}}` anywhere; it is inert there. Only `firmware-build`
(firmware filename/tag), `docker-build` (image tag), and `update-declarative-config`
(`sed` replacement) actually read it. Verified against the live template 2026-09-04.

**Version Resolution Logic:**
- Doc/bead-only change (all changed files match the ignore patterns below) → `should-build=false`, `version` = the repo's current `VERSION` unchanged, exit 0
- `VERSION` file changed in the commit → use the new version from the file
- `VERSION` file unchanged on a substantive change → auto-bump patch (e.g. `0.2.123` → `0.2.124`), then `git commit` and push that bump back to **Forgejo** (not the GitHub mirror) using `GIT_TOKEN`
- `resolve-version` clones with `--depth 2` specifically so `HEAD~1` exists for the `VERSION`/changed-file diff

## Step Output Parameters

`resolve-version` is the only template with outputs; it materializes both as files that
Argo reads into output parameters:

| Output Parameter | Materialized At | Description | Used By |
|------------------|-----------------|-------------|---------|
| `should-build` | `/tmp/should_build` | Boolean flag (`true`/`false`) indicating if build should proceed | Every step after `resolve-version`, via the `when` clause only (never passed as an argument) |
| `version` | `/tmp/version` | Resolved version string | Forwarded as the `version` step argument to the five templates listed above |

No template declares artifacts — `version` and `should-build` are the entire inter-step
data flow. Everything else (source checkout, firmware image, container image) is
re-fetched or produced inside each step independently.

## Secret Requirements

The workflow requires the following secrets to be mounted:

| Secret Name | Key | Environment Variable | Scope | Purpose |
|-------------|-----|---------------------|-------|---------|
| `github-webhook-secret` | `token` | `GH_TOKEN` | 9 of 10 steps (all but `update-declarative-config`) | GitHub API authentication (release download/upload via `gh`) |
| `forgejo-webhook-token` | `token` | `FORGEJO_TOKEN` | all 10 steps | Forgejo git authentication, via the `GIT_CONFIG_*` credential helper |
| `forgejo-webhook-token` | `token` | `GIT_TOKEN` | `resolve-version` only | Same secret value, bound to a second env var so the auto-bump push URL can interpolate it |
| `docker-hub-registry` | `.dockerconfigjson` | (mounted as volume) | `docker-build` only | Docker registry authentication for image push |

**Secret locations:**
- GitHub token: `github-webhook-secret` secret in `argo-workflows` namespace
- Forgejo token: `forgejo-webhook-token` secret in `argo-workflows` namespace
- Docker config: `docker-hub-registry` secret in `argo-workflows` namespace

## Parameter Dependencies

### Consumer matrix (verified against the live template 2026-09-04)

Which step actually references which workflow parameter:

| Step template | `git-repo` | `branch` | `image-repo` | `version` (input) | `should-build` (when) |
|---------------|:----------:|:--------:|:------------:|:-----------------:|:---------------------:|
| `resolve-version` | ✔ | ✔ | — | n/a | n/a |
| `golangci-lint` | ✔ | ✔ | — | declared, **unread** | ✔ |
| `a11y-test` | ✔ | ✔ | — | declared, **unread** | ✔ |
| `go-test` | ✔ | ✔ | — | — | ✔ |
| `timing-benchmark` | ✔ | ✔ | — | — | ✔ |
| `acceptance-test` | ✔ | ✔ | — | — | ✔ |
| `firmware-test` | ✔ | ✔ | — | — | ✔ |
| `firmware-build` | ✔ | ✔ | — | ✔ | ✔ |
| `docker-build` | ✔ | ✔ | ✔ | ✔ | ✔ |
| `update-declarative-config` | — | — | — | ✔ | ✔ |

`GH_TOKEN`, `GIT_TOKEN`, and `FORGEJO_TOKEN` are **not** uniformly mounted — the three
secrets have different scopes. Verified against the live template 2026-09-04:

| Step template | `FORGEJO_TOKEN` | `GH_TOKEN` | `GIT_TOKEN` | Git credential helper |
|---------------|:---------------:|:----------:|:-----------:|:---------------------:|
| `resolve-version` | ✔ | ✔ | ✔ | ✔ |
| `golangci-lint` | ✔ | ✔ | — | ✔ |
| `a11y-test` | ✔ | ✔ | — | ✔ |
| `go-test` | ✔ | ✔ | — | ✔ |
| `timing-benchmark` | ✔ | ✔ | — | ✔ |
| `acceptance-test` | ✔ | ✔ | — | ✔ |
| `firmware-test` | ✔ | ✔ | — | ✔ |
| `firmware-build` | ✔ | ✔ | — | ✔ |
| `docker-build` | ✔ | ✔ | — | ✔ |
| `update-declarative-config` | ✔ | — | — | ✔ |

How each is actually used:

- `FORGEJO_TOKEN` (every step) is consumed *indirectly*: every step also sets
  `GIT_CONFIG_COUNT=1` / `GIT_CONFIG_KEY_0=credential.helper` /
  `GIT_CONFIG_VALUE_0='!f() { test "$1" = get && echo "username=x-token" && echo
  "password=$FORGEJO_TOKEN"; }; f'`, so all `git clone`/`git push` traffic authenticates
  through the helper. No step script names `FORGEJO_TOKEN` literally.
- `GIT_TOKEN` is mounted on **`resolve-version` only** — it appears verbatim in exactly
  one place, the `https://x-token:${GIT_TOKEN}@…` push URL for the auto-bump commit. It
  is a distinct env var from `FORGEJO_TOKEN` even though both resolve to the same
  `forgejo-webhook-token/token` key.
- `GH_TOKEN` backs the `gh` CLI calls (release download in `firmware-build`, release
  upload/asset attach). `update-declarative-config` is the only step with no GitHub
  contact, and it is the only step without it.

### Couplings and gotchas

1. **`image-repo` reaches exactly one step.** It is referenced only inside
   `docker-build` (as the `docker buildx build -t` target). `resolve-version` and
   `update-declarative-config` never see it.
2. **`update-declarative-config` hardcodes the image prefix anyway.** Its `sed` rewrites
   the literal `ronaldraygun/spaxel:[^ "]*` in
   `k8s/ardenone-cluster/spaxel/deployment.yml` and `nixos/bench/modules/mothership.nix`.
   Overriding `image-repo` therefore changes the *pushed* tag but **not** the
   *deployed* pin, and a mismatch between the two ships a pin pointing at an image that
   was never published. Treat `image-repo` as fixed in practice.
3. **`update-declarative-config` consumes neither `git-repo` nor `branch`.** It clones
   `jedarden/declarative-config` at a hardcoded URL on whatever its default branch is,
   so building a non-`main` source branch still rewrites the pin that `main`'s ArgoCD
   will deploy.
4. **`version` is the only chain between resolve and build.** `should-build` gates
   execution but is never passed as an argument, so `version` and `should-build` must
   stay consistent by convention: a doc-only push writes the repo's current `VERSION`
   into `/tmp/version` (not a bump) precisely so that any `version` consumer that
   somehow still runs sees a real value.
5. **`git-repo` is a template-wide assumption.** Every step that clones does so from
   `https://git.ardenone.com/{{workflow.parameters.git-repo}}.git`, but
   `update-declarative-config` and the hardcoded image prefix in (2) remain
   spaxel-specific — the template is not fully reusable for a different `git-repo`.
6. **`docker-config` secret volume is mounted only by `docker-build`** (at
   `/root/.docker`); the token secrets are env-level and fleet-wide as noted above.

```
                    ┌────────────── should-build (when gate, all steps) ──────────────┐
                    │                                                                  │
 workflow args      │   resolve-version ── version ──→ firmware-build ──→ GitHub Release
 ─ git-repo ───┬────┴───────────────│
 ─ branch ─────┼──→ clone steps:    ├── version ──→ docker-build ──→ image-repo:version
 ─ image-repo ─┼─── (docker-build   │
               │     only)          └── version ──→ update-declarative-config
               │                          (hardcodes ronaldraygun/spaxel: prefix,
               │                           clones declarative-config default branch)
               └──→ golangci-lint · a11y-test · go-test · timing-benchmark ·
                    acceptance-test · firmware-test   (clone + test only)
```

## Conditional Execution

All build steps are guarded by the `should-build` condition:
```yaml
when: '{{steps.resolve-version.outputs.parameters.should-build}} == true'
```

**Doc-only changes skip builds:**
- Files matching patterns: `^docs/`, `^\.beads/`, `^\.needle`, `\.md$`, `^LICENSE$`, `^\.gitignore$`
- Changed files not matching these patterns trigger a full build

## Resource Limits by Step

| Step | CPU Limit | Memory Limit | Purpose |
|------|-----------|--------------|---------|
| `resolve-version` | not specified | not specified | Git clone and version resolution |
| `firmware-build` | 3500m | 6Gi | ESP32-S3 firmware compilation |
| `docker-build` | 3500m | 6Gi | Multi-arch Docker image build |
| `golangci-lint` | 2000m | 4Gi | Go linting |
| `a11y-test` | 2000m | 4Gi | Accessibility testing |
| `go-test` | 2000m | 4Gi | Go unit tests |
| `timing-benchmark` | 2000m | 4Gi | Fusion loop performance benchmarks |
| `acceptance-test` | 3500m | 6Gi | Integration tests with sim |
| `firmware-test` | 2000m | 4Gi | Host-based firmware tests |
| `update-declarative-config` | not specified | not specified | Update deployment pins |

## Active Deadlines

| Step | Timeout | Purpose |
|------|---------|---------|
| Overall workflow | 33900s (9.4 hours) | Total workflow runtime limit |
| `resolve-version` | 300s (5 min) | Git clone and version calculation |
| `firmware-build` | 3600s (1 hour) | ESP32-S3 compilation |
| `docker-build` | 5400s (1.5 hours) | Multi-arch image build |
| `golangci-lint` | 600s (10 min) | Linting |
| `a11y-test` | 600s (10 min) | Accessibility tests |
| `go-test` | 1500s (25 min) | Go unit tests |
| `timing-benchmark` | 900s (15 min) | Performance benchmarks |
| `acceptance-test` | 900s (15 min) | Integration tests |
| `firmware-test` | 1200s (20 min) | Firmware unit tests |
| `update-declarative-config` | 120s (2 min) | Config updates |

## Retry Strategy

Most build/test steps use exponential backoff retry:

| Step | Retry Limit | Backoff Duration | Backoff Factor | Policy |
|------|-------------|-------------------|----------------|---------|
| `firmware-build` | 2 | 30s | 2 | OnError |
| `docker-build` | 2 | 30s | 2 | OnError |
| `golangci-lint` | 1 | 30s | 2 | OnError |
| `a11y-test` | 1 | 30s | 2 | OnError |
| `go-test` | 1 | 30s | 2 | OnError |
| `timing-benchmark` | 1 | 30s | 2 | OnError |
| `acceptance-test` | 1 | 30s | 2 | OnError |
| `firmware-test` | 1 | 30s | 2 | OnError |

**Note:** `acceptance-test` has `continueOn.failed: true` - failures don't block the workflow.

**No retry at all:** `resolve-version` and `update-declarative-config` define no
`retryStrategy` — a failed clone/version bump or a failed declarative-config push fails
the workflow on the first attempt. The workflow-level `activeDeadlineSeconds` is 33900.

## Built Artifacts

| Artifact | Location | Format |
|----------|----------|--------|
| Firmware binary | GitHub Releases (`jedarden/spaxel`) | `spaxel-firmware-<version>-merged.bin` |
| Docker image | Docker Hub (`ronaldraygun/spaxel`) | Multi-arch manifest (amd64, arm64) |
| Config updates | `jedarden/declarative-config` repo | Deployment YAML updates |

## Related Documentation

- **Workflow location:** `k8s/iad-ci/argo-workflows/spaxel-build-workflowtemplate.yml` in `jedarden/declarative-config` (the earlier `spaxel-build.yml` citation here was wrong — see `spaxel-build-architecture-targeting.md` §1)
- **Namespace:** `argo-workflows`
- **ServiceAccount:** `argo-workflow`
- **Template type:** WorkflowTemplate (can be referenced via `workflowTemplateRef`)

## Verification

**Re-verified field-by-field against the live WorkflowTemplate on 2026-09-04**
(`kubectl --server=http://traefik-iad-ci:8001 get workflowtemplate spaxel-build -n
argo-workflows -o json`): 3 workflow parameters and their defaults, the single
template-level parameter and its 5 declared (3 reading) consumers, both
`resolve-version` outputs, the `when` gate and doc-only exclusion patterns, all 10
resource limits, all 11 deadlines, every retry strategy, and the
`continueOn.failed` on `acceptance-test` — all confirmed accurate. Corrections made
in this pass: the previous dependency diagram wrongly showed `image-repo` feeding
`resolve-version`; `update-declarative-config` consumes neither `git-repo` nor
`branch`; `golangci-lint`/`a11y-test` accept `version` without reading it; and the
secret-mounting claim was corrected — the earlier text said all three token env vars
were mounted on every step, when `GIT_TOKEN` exists on `resolve-version` alone and
`update-declarative-config` has no `GH_TOKEN`.

To verify the current template definition:
```bash
kubectl --server=http://traefik-iad-ci:8001 \
  get workflowtemplate spaxel-build -n argo-workflows -o yaml
```

To submit a manual workflow run:
```bash
kubectl --kubeconfig=/home/coding/.kube/iad-ci.kubeconfig \
  create -f - <<EOF
apiVersion: argoproj.io/v1alpha1
kind: Workflow
metadata:
  generateName: spaxel-build-manual-
  namespace: argo-workflows
spec:
  workflowTemplateRef:
    name: spaxel-build
EOF
```
