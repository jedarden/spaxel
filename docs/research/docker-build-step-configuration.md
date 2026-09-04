# Docker-Build Step Configuration Documentation

**Date:** 2026-09-01  
**Status:** Production  
**Workflow:** spaxel-build-workflowtemplate.yml

## Overview

The docker-build step is a critical component of the spaxel CI/CD pipeline that builds multi-architecture container images for the spaxel application. It uses **Docker-in-Docker (DinD)** rather than Kaniko for maximum flexibility and performance.

## File Location

The authoritative manifest lives in the **`jedarden/declarative-config`** repo (outside
this one), at `k8s/iad-ci/argo-workflows/spaxel-build-workflowtemplate.yml`, and is
applied to the cluster by ArgoCD. Cite it by template + step name rather than line
number — the file is edited in place and line numbers go stale.

**Verification basis for this document:** the *live* `WorkflowTemplate/spaxel-build`
object in the `argo-workflows` namespace on iad-ci (read via the credential-free
endpoint), re-verified 2026-09-04 against this repo's `Dockerfile` at the same date:

```bash
kubectl --server=http://traefik-iad-ci:8001 \
  get workflowtemplate spaxel-build -n argo-workflows -o json | \
  jq '.spec.templates[] | select(.name=="docker-build")'
```

Every claim below marked *(live 2026-09-04)* was checked against that object; the
Dockerfile-side claims were checked against the repo `Dockerfile` the same day.

## Step Structure

### Template Definition

```yaml
- name: docker-build
  inputs:
    parameters:
      - name: version
  retryStrategy:
    limit: "2"
    retryPolicy: OnError
    backoff:
      duration: 30s
      factor: "2"
  container:
    image: docker:29.7.2-dind
    command: [sh, -c]
```

### Input Parameters

| Parameter | Source | Description |
|-----------|--------|-------------|
| `version` | `{{inputs.parameters.version}}` | Version tag for the image (from VERSION file) |

### Workflow-Level Parameters

| Parameter | Default Value | Description |
|-----------|---------------|-------------|
| `git-repo` | `jedarden/spaxel` | Forgejo repository path |
| `image-repo` | `ronaldraygun/spaxel` | Docker Hub repository path |
| `branch` | `main` | Git branch to build from |

## Container Images and Tools

### Primary Container Image

**`docker:29.7.2-dind`** *(live 2026-09-04)*
- Docker Engine with Docker-in-Docker support
- Version: 29.7.2 (Docker CLI and daemon)
- Runs in privileged mode for full Docker functionality

### Base Images Pulled by the Build Itself

The step's buildx invocation consumes the repo `Dockerfile`, whose three `FROM` stages
pull three further images (per build platform leg). These are documented in depth in
`docs/research/mothership-build-and-configuration.md` §2.1; listed here because they are
images this step pulls, not images the step runs in:

| Image | Dockerfile stage | Role |
|---|---|---|
| `alpine:3.20` | `firmware-fetcher` | Adds `curl` and downloads the prebuilt ESP32-S3 firmware from GitHub Releases (see *Firmware Integration* below) |
| `golang:1.25-bookworm` | `builder` | Builds `spaxel` and `spaxel-sim` on `$BUILDPLATFORM` (native; Go cross-compiles via `GOARCH=$TARGETARCH`) |
| `gcr.io/distroless/static-debian12:nonroot` | runtime | Final single-layer image; no shell, runs as UID 65532 |

No ESP-IDF image is pulled here — per ADR-001 the firmware is built once by the separate
`firmware-build` step and fetched as a release artifact.

### Build Platform

**Multi-architecture support:**
- `linux/amd64` - Standard x86_64 platform
- `linux/arm64` - ARM64/AArch64 platform

**Native builds** (no QEMU emulation) for maximum performance and correctness — in effect
rather than by construction: every `RUN` executes on `$BUILDPLATFORM` (the amd64 CI
runner) and Go cross-compiles via `GOARCH=$TARGETARCH`, so no per-arch native builder and
no emulator is involved. See `docs/research/spaxel-build-architecture-parameters.md` §3.

### Tools and Commands

- **docker buildx** - Multi-architecture build tool
- **docker buildx create** - Builder instance creation
- **docker buildx build** - Main build command
- **docker buildx imagetools** - Image inspection

## Build Arguments

### Docker Build Arguments

| Argument | Value | Purpose |
|----------|-------|---------|
| `VERSION` | `{{inputs.parameters.version}}` | The only build-arg CI passes explicitly. Embeds the version in the Go binary via `-X main.version` **and** names the firmware files copied into the image — the OTA store derives its version from that filename, so this arg has functional weight |

The Dockerfile declares further build args that buildx populates per platform leg
(`TARGETARCH`, `TARGETPLATFORM`, `BUILDPLATFORM`) — CI never passes them explicitly, and
they cannot be set at workflow-submit time. Full table with defaults and consumers:
`docs/research/spaxel-build-architecture-parameters.md` §3.

### Buildx Command Arguments

```bash
--platform=linux/amd64,linux/arm64          # Multi-arch platforms
--build-arg=VERSION={{version}}             # Version build arg
--tag={{image-repo}}:{{version}}            # Version-specific tag
--tag={{image-repo}}:latest                 # Latest tag
--push                                       # Push to registry
--cache-to=type=registry,...                # Cache upload
--cache-from=type=registry,...              # Cache download
--progress=plain                             # Verbose output
--file=Dockerfile                            # Dockerfile path
.                                            # Build context
```

## Environment Variables

### Git Configuration

```yaml
GIT_CONFIG_COUNT: "1"
GIT_CONFIG_KEY_0: credential.helper
GIT_CONFIG_VALUE_0: '!f() { test "$1" = get && echo "username=x-token" && echo "password=$FORGEJO_TOKEN"; }; f'
```

### Authentication Tokens

| Variable | Source | Purpose |
|----------|--------|---------|
| `GH_TOKEN` | Secret: `github-webhook-secret` | GitHub API authentication |
| `FORGEJO_TOKEN` | Secret: `forgejo-webhook-token` | Forgejo/Git.ardenone.com authentication |

**Note:** The workflow uses both GitHub and Forgejo tokens because:
- Forgejo is the primary git repository (git.ardenone.com)
- GitHub is used for firmware releases and as a read-only mirror

## Volume Mounts

### Docker Registry Credentials

```yaml
volumeMounts:
  - name: docker-config
    mountPath: /root/.docker
```

**Volume Definition:**
```yaml
volumes:
  - name: docker-config
    secret:
      secretName: docker-hub-registry
      items:
        - key: .dockerconfigjson
          path: config.json
```

This provides Docker Hub authentication for pushing images to the `ronaldraygun/spaxel` repository.

## Working Directory

**Base Directory:** `/tmp`
**Clone Directory:** `/tmp/repo`

```bash
cd /tmp
git clone --depth 1 --branch {{workflow.parameters.branch}} \
  "https://git.ardenone.com/{{workflow.parameters.git-repo}}.git" \
  repo
cd repo
```

## Security Context

```yaml
securityContext:
  privileged: true
```

**Reason:** Docker-in-Docker requires privileged mode to run the Docker daemon with full capabilities.

## Resource Limits

| Resource | Request | Limit |
|----------|---------|-------|
| CPU | 2000m (2 cores) | 3500m (3.5 cores) |
| Memory | 4 GiB | 6 GiB |

## Active Deadline

**5400 seconds (90 minutes)** - Maximum time allowed for the docker-build step to complete.

## Retry Strategy

```yaml
retryStrategy:
  limit: "2"                    # Maximum 2 retries
  retryPolicy: OnError          # Retry on error only
  backoff:
    duration: 30s              # Initial backoff
    factor: "2"                # Exponential backoff
```

## Build Process

### 1. Docker Daemon Initialization

```bash
set -e
dockerd-entrypoint.sh &
sleep 5
```

Starts the Docker-in-Docker daemon and waits for it to be ready.

### 2. Repository Clone

```bash
cd /tmp
git clone --depth 1 --branch {{workflow.parameters.branch}} \
  "https://git.ardenone.com/{{workflow.parameters.git-repo}}.git" \
  repo
cd repo
```

### 3. Buildx Setup

```bash
docker buildx create --use --name multiarch-builder --driver docker-container
```

Creates a multi-architecture builder instance.

### 4. Multi-Architecture Build

```bash
docker buildx build \
  --platform=linux/amd64,linux/arm64 \
  --build-arg=VERSION={{inputs.parameters.version}} \
  --tag={{workflow.parameters.image-repo}}:{{inputs.parameters.version}} \
  --tag={{workflow.parameters.image-repo}}:latest \
  --push \
  --cache-to=type=registry,ref={{workflow.parameters.image-repo}}:buildcache,mode=max \
  --cache-from=type=registry,ref={{workflow.parameters.image-repo}}:buildcache \
  --progress=plain \
  --file=Dockerfile \
  .
```

### 5. Image Inspection

```bash
docker buildx imagetools inspect {{workflow.parameters.image-repo}}:{{inputs.parameters.version}}
```

Verifies the resulting multi-architecture manifest.

## Special Features

### 1. Multi-Architecture Builds

- **Native compilation** for both AMD64 and ARM64
- **No QEMU emulation** - faster and more reliable
- **Single manifest** - Docker automatically serves the right architecture

### 2. BuildKit Registry Cache

```bash
--cache-to=type=registry,ref={{workflow.parameters.image-repo}}:buildcache,mode=max
--cache-from=type=registry,ref={{workflow.parameters.image-repo}}:buildcache
```

- Stores build cache in the Docker registry
- Speeds up incremental builds
- `mode=max` includes all layers for maximum cache hits

### 3. Firmware Integration

*(Corrected 2026-09-04 against the repo `Dockerfile` — the snippet previously quoted here
described an older single-fetch layout and no longer matched the file.)*

The Dockerfile's first stage (`alpine:3.20 AS firmware-fetcher`) installs `curl` and
downloads **two** firmware artifacts from GitHub Releases, both required (`&&`-chained —
there is no fallback; either one missing fails the build):

```dockerfile
FROM alpine:3.20 AS firmware-fetcher
ARG VERSION=dev
RUN apk add --no-cache curl
WORKDIR /firmware
RUN curl -fsSL \
    "https://github.com/jedarden/spaxel/releases/download/v${VERSION}/spaxel-firmware-${VERSION}-merged.bin" \
    -o spaxel-firmware-merged.bin && \
    curl -fsSL \
    "https://github.com/jedarden/spaxel/releases/download/v${VERSION}/spaxel-firmware.bin" \
    -o spaxel-firmware.bin && \
    ...
```

Stage 3 then places them at two deliberately different destinations:

| Artifact | Copied to | Why |
|---|---|---|
| `spaxel-firmware.bin` | `/firmware/spaxel-firmware-${VERSION}.bin` | App-only image; seeded into the OTA store at startup. The semver-bearing filename **is** the OTA store's version source, so the `VERSION` build-arg has functional weight beyond labelling. |
| `spaxel-firmware-merged.bin` | `/firmware/serial/spaxel-firmware-${VERSION}-merged.bin` | Offset-0 full-flash image for first-flash serial provisioning, isolated in a subdirectory `seedFirmwareDir` deliberately does **not** copy into the OTA store — a merged image must never be written into an OTA app partition. |

**The firmware is not part of the Go binary.** Only the dashboard is embedded via
`go:embed`; the firmware arrives as files under `/firmware/`. This step's role is to bake
the version-correct firmware *files* into the image at build time.

**Ordering caveat:** `build-firmware` and this step's `build` node are **siblings in the
same parallel step group** (the entrypoint is a `steps:` template, not a `dag:`), not a
sequence — `docker-build` does not wait for the firmware
release to exist. Both use the same resolved `version`, so the fetch stage can race the
`firmware-build` step's release upload for that version. If a build fails in
`firmware-fetcher` with a 404 on `spaxel-firmware-${VERSION}-merged.bin`, that race is the
first thing to check, and the step's `retryStrategy` (2 retries, 30 s→60 s backoff) is
what usually absorbs it.

### 4. Dual Tagging

Each build produces two tags:
- `ronaldraygun/spaxel:{{version}}` - Version-specific (immutable)
- `ronaldraygun/spaxel:latest` - Latest (rolling)

### 5. Authentication Integration

The workflow authenticates to:
- **Docker Hub** (via `docker-hub-registry` secret) - For image push
- **Forgejo** (via `forgejo-webhook-token` secret) - For git clone
- **GitHub** (via `github-webhook-secret` secret) - For firmware downloads

## Comparison: Docker-in-Docker vs Kaniko

The spaxel workflow uses Docker-in-Docker instead of Kaniko:

| Feature | Docker-in-Docker | Kaniko |
|---------|------------------|--------|
| Daemon | Full Docker daemon | Daemon-less |
| Privileged | Yes required | No |
| Cache | Full BuildKit | Limited |
| Multi-arch | Native with buildx | Native |
| Complexity | Higher | Lower |
| Use case | General builds | Kubernetes-optimized |

**Why DinD for spaxel:**
- Full BuildKit feature support
- Maximum cache effectiveness
- Direct Docker command compatibility
- Multi-architecture native builds

## Workflow Integration

The docker-build step is part of a larger workflow:

```yaml
steps:
  - - name: resolve-version
      template: resolve-version
  - - name: lint
      template: golangci-lint
    - name: a11y-test
      template: a11y-test
  - - name: go-test
      template: go-test
    - name: timing-benchmark
      template: timing-benchmark
  - - name: acceptance-test
      template: acceptance-test
  - - name: firmware-test
      template: firmware-test
  - - name: build-firmware
      template: firmware-build
  - - name: build                  # ← docker-build step
      template: docker-build
  - - name: update-declarative-config
      template: update-declarative-config
```

The docker-build step:
- Runs after all test/lint steps (they occupy earlier step groups) and only when
  `{{steps.resolve-version.outputs.parameters.should-build}} == true` — the same guard
  every build step carries, so doc-only pushes skip it entirely
- Runs **in parallel with** `build-firmware`, not after it (see *Firmware Integration*
  above for the fetch-vs-upload consequence)
- Runs before `update-declarative-config`, which rewrites the deployment's image pin to
  the version this step just pushed

## Output Artifacts

### Docker Registry

The step produces images in Docker Hub:
- `ronaldraygun/spaxel:{{version}}` - Primary image tag
- `ronaldraygun/spaxel:latest` - Rolling tag
- `ronaldraygun/spaxel:buildcache` - BuildKit cache manifest

### Verification

```bash
docker buildx imagetools inspect ronaldraygun/spaxel:{{version}}
```

This outputs the manifest showing both AMD64 and ARM64 architectures.

## Dependencies

### External Services

1. **Docker Hub** - Image registry
2. **Forgejo (git.ardenone.com)** - Git repository
3. **GitHub (github.com)** - Firmware releases

### Secrets

1. `docker-hub-registry` - Docker Hub credentials
2. `forgejo-webhook-token` - Forgejo API token
3. `github-webhook-secret` - GitHub API token

## Troubleshooting

### Common Issues

**1. Privileged mode errors**
- Symptom: "cannot mount docker socket" or permission denied
- Cause: Missing privileged security context
- Fix: Ensure `securityContext.privileged: true`

**2. Authentication failures**
- Symptom: "no basic auth credentials" when pushing
- Cause: Missing or incorrect Docker Hub secret
- Fix: Verify `docker-hub-registry` secret exists and is valid

**3. Build cache not working**
- Symptom: Every rebuild is slow
- Cause: Cache manifest not being used
- Fix: Check `--cache-from` matches previous `--cache-to`

**4. Multi-arch build failures**
- Symptom: One architecture fails, other succeeds
- Cause: Platform-specific code issues
- Fix: Build each architecture separately to debug

## Related Documentation

- **`docs/BUILD_PATHS.md`** - What triggers docker builds in spaxel
- **`docs/kaniko-version-research.md`** - Kaniko v1.24.0 research (not currently used)
- **github-api-authentication-kaniko-releases.md** - GitHub API client for Kaniko (not currently used)
- **declarative-config/k8s/iad-ci/argo-workflows/spaxel-build-workflowtemplate.yml** - Full workflow definition

## Version History

| Date | Version | Changes |
|------|---------|---------|
| 2026-09-01 | 1.0 | Initial documentation of docker-build step (bead spaxel-afa4858e) |
| 2026-09-04 | 1.1 | Verified every claim against the live `WorkflowTemplate/spaxel-build` (image, env, resources, deadline, retry, volume mount, securityContext, buildx flags, step ordering — all accurate); corrected the firmware-fetch section to match the current `Dockerfile` (fetcher stage, two mandatory artifacts, OTA-store vs serial isolation, "not part of the Go binary"); corrected the step ordering (docker-build is a parallel sibling of build-firmware, not downstream of it); added the base images the build pulls (3 `FROM` stages, not 4); replaced the out-of-repo local path citation with the declarative-config repo path; `steps:` group terminology rather than `dag:` (bead spaxel-c52b5ef7) |

## Sources

- Argo WorkflowTemplate: `jedarden/declarative-config` → `k8s/iad-ci/argo-workflows/spaxel-build-workflowtemplate.yml` (authoritative manifest lives outside this repo; the live cluster object was used for verification)
- Repo `Dockerfile` (this repo) — base images, build args, firmware fetch
- Docker Documentation: https://docs.docker.com/
- Docker Buildx Documentation: https://docs.docker.com/buildx/working-with-buildx/
