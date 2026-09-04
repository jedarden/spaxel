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

These parameters are passed between workflow steps:

| Parameter | Description | Source | Consumed By |
|-----------|-------------|--------|-------------|
| `version` | SemVer version string (e.g., `0.2.124`) | Output from `resolve-version` step | `firmware-build`, `docker-build`, `update-declarative-config`, `golangci-lint`, `a11y-test` |

**Version Resolution Logic:**
- If `VERSION` file changed in commit → use the new version from file
- If `VERSION` file unchanged → auto-bump patch version (e.g., `0.2.123` → `0.2.124`)
- For doc/bead-only changes → version is read but `should-build` is `false`

## Step Output Parameters

| Parameter | Description | Source | Used By |
|-----------|-------------|--------|---------|
| `should-build` | Boolean flag (`true`/`false`) indicating if build should proceed | `resolve-version` step | All subsequent steps (via `when` clause) |
| `version` | Resolved version string | `resolve-version` step | Build and config update steps |

## Secret Requirements

The workflow requires the following secrets to be mounted:

| Secret Name | Key | Environment Variable | Purpose |
|-------------|-----|---------------------|---------|
| `github-webhook-secret` | `token` | `GH_TOKEN` | GitHub API authentication (releases, repo operations) |
| `forgejo-webhook-token` | `token` | `GIT_TOKEN`, `FORGEJO_TOKEN` | Forgejo git push authentication |
| `docker-hub-registry` | `.dockerconfigjson` | (mounted as volume) | Docker registry authentication for image push |

**Secret locations:**
- GitHub token: `github-webhook-secret` secret in `argo-workflows` namespace
- Forgejo token: `forgejo-webhook-token` secret in `argo-workflows` namespace
- Docker config: `docker-hub-registry` secret in `argo-workflows` namespace

## Parameter Dependencies

```
git-repo ──────────────────────────────────────┐
                                             ├──→ resolve-version → version ──┐
image-repo ──────────────────────────────────┘                              │
                                                                               ├──→ firmware-build ──→ GitHub Releases
branch ──────────────────────────────────────┐                               │
                                             ├──→ resolve-version → should-build
GH_TOKEN, GIT_TOKEN, FORGEJO_TOKEN ──────────┘                              │
                                                                               ├──→ docker-build ──→ image-repo:version
                                                                              │
                                                                              └──→ update-declarative-config
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
