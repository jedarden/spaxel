# CI: Docker build-context and Dockerfile audit of the `spaxel-e2e` WorkflowTemplate

**Bead:** spaxel-8849b19f (child #4 of bf-2yur, depends on bf-1bitu / spaxel-630f1172)
**Date:** 2026-09-04
**Question:** which Docker build contexts and Dockerfile references does the `spaxel-e2e`
Argo WorkflowTemplate carry — where does each build run from, and what does each
Dockerfile path resolve to?

## Answer

**The template contains exactly one Docker image build.** It builds from the
*repository root* context `/tmp/spaxel-src` and resolves the *repo-root*
`Dockerfile` by default (no `-f`), producing the throwaway tag
`spaxel-e2e:test` inside the step's dind daemon. That Dockerfile is the only
git-tracked Dockerfile in the repository, so there is exactly one
(context, Dockerfile) pair to account for:

> **builds from `/tmp/spaxel-src` (repo root) with `Dockerfile` at that root,
> tagged `spaxel-e2e:test`, inside the `docker:dind` sidecar daemon.**

Everything else Docker-shaped in the template is *runtime* use of the Docker
CLI against that one daemon (info/run/logs/stop/start/rm) plus the socket
plumbing that connects the step to it. No `docker://` template type, no second
`docker build`, no Dockerfile reference other than the implicit default.

## Source audited

Two artifacts, checked against each other rather than assumed to agree:

- **declarative-config source** — `declarative-config/k8s/iad-ci/argo-workflows/spaxel-e2e-workflowtemplate.yml`,
  516 lines (head `64e3e540`).
- **live cluster object** — `kubectl --kubeconfig=/home/coding/.kube/iad-ci.kubeconfig
  get workflowtemplate spaxel-e2e -n argo-workflows -o yaml`, 522 lines,
  `metadata.resourceVersion` 71293261, `metadata.generation` 2 (the object was
  recreated in the 2026-09-02 operator cutover, so its generation counter is
  low even though the content predates it).

The three `script.source` blocks are **byte-identical** between the two, and
images, volumes and sidecars match field-for-field. The live object's extra 6
lines are API plumbing only (`last-applied-configuration` annotation, managed
fields, status). **Line numbers below are given for both**, so any later
audit can re-anchor without re-deriving which file a number came from.

**Numbering note for the parent investigation:** the dispatch hint "dind
docker-e2e step context around line ~265, docker build around ~311-312" comes
from an older snapshot of this template. In the current declarative-config
manifest the docker-socket mount sits at 311-312 and the build at 347; in the
live cluster dump they sit at 505 and 366. Both are correct for their file —
the template gained ~40 lines of inline `acceptance-tests` source between
snapshots.

## Complete inventory of Docker references

`docker-e2e` step — template def at src 300 (live 307), image
`golang:1.25-alpine` at src 308, deadline 900s.

| what | src lines | live lines | detail |
|---|---|---|---|
| **build context + Dockerfile** | **346-347** | **365-366** | `cd /tmp/spaxel-src` then `docker build -t spaxel-e2e:test .` — context `.` ⇒ `/tmp/spaxel-src`; Dockerfile = `/tmp/spaxel-src/Dockerfile` (implicit default, no `-f`) |
| dind sidecar | 504-515 | 508-519 | `image: docker:dind`, `command: [dockerd-entrypoint.sh]`, `DOCKER_TLS_CERTDIR=""`, `privileged: true`, mounts `docker-sock` at `/var/run` |
| socket volume (spec) | 32-34 | 520-522 | `docker-sock` = `emptyDir: {}` — the only Docker-specific volume |
| socket volume (step mount) | 310-312 | 504-506 | script container mounts `docker-sock` at `/var/run`, so `docker` CLI reaches `/var/run/docker.sock` |
| docker CLI install | 318 | 337 | `apk add --no-cache docker-cli curl jq git` — CLI only, daemon comes from the sidecar |
| daemon readiness probe | 324, 328 | 343, 347 | `docker info` ×2 (poll loop + hard failure after 60s) |
| image build banner | 345 | 364 | `echo "=== Building mothership Docker image ==="` |
| container start | 352-358 | 371-377 | `docker run -d --name spaxel-e2e-test -p 8080:8080 … spaxel-e2e:test` (tmpfs `/data`, 100M) |
| container logs on failure | 368, 414 | 387, 433 | `docker logs spaxel-e2e-test --tail 50` |
| container restart (IO-2) | 406-407 | 425-426 | `docker stop` / `docker start` |
| container teardown | 480-481 | 499-500 | `docker stop … \|\| true` / `docker rm … \|\| true` |

Images referenced by the template as containers (not build contexts):
`golang:1.23-bookworm` ×2 (go-test, acceptance-tests), `golang:1.25-alpine`
(docker-e2e script), `docker:dind` (sidecar).

## Working-directory base for the build

The template sets no `workingDir`, so each step starts at the base image's
own `WORKDIR` (`/go` for `golang:1.25-alpine`) and every base transition is an
explicit absolute `cd`. The `docker-e2e` chain is:

| line (src) | command | CWD after |
|---|---|---|
| 333-335 | `git clone … /tmp/spaxel-src` | `/go` (clone target is absolute; CWD irrelevant) |
| 340 | `cd /tmp/spaxel-src/mothership` | `/tmp/spaxel-src/mothership` — used only for the `go build` of the simulator |
| 346 | `cd /tmp/spaxel-src` | **`/tmp/spaxel-src` — the build base** |
| 347 | `docker build -t spaxel-e2e:test .` | unchanged; context `.` resolves against `/tmp/spaxel-src` |

Note the deliberate two-step sequence: the simulator binary is built *outside*
the image (`go build -o /tmp/spaxel-sim ./cmd/sim` from `mothership/`), then
CWD moves up one level so the Docker context is the repo root. The build never
runs from `mothership/`.

## Resolved module locations for the Dockerfile path

`/tmp/spaxel-src/Dockerfile` is the **only** git-tracked Dockerfile in the
repository (`git ls-files | grep -E '^Dockerfile$'` → one hit). A
`tmp/acc-repro/Dockerfile` exists in one local checkout but is untracked
scratch and never reaches a CI clone.

Because the context is the repo root, every relative path inside the
Dockerfile resolves from `/tmp/spaxel-src`. The three-stage layout:

| stage | base | resolves from context | produces |
|---|---|---|---|
| 1 `firmware-fetcher` | `alpine:3.20` | nothing copied — downloads `spaxel-firmware-{,-merged.}bin` from GitHub Releases into `/firmware` | firmware binaries for the image |
| 2 `builder` | `golang:1.25-bookworm` | `mothership/go.mod`, `mothership/go.sum` → `/app`; `mothership/` → `/app`; `dashboard/` → `/app/cmd/mothership/dashboard/` | `/app/spaxel` (`./cmd/mothership`, `-tags=embed`) and `/app/spaxel-sim` (`./cmd/sim`) |
| 3 runtime | `gcr.io/distroless/static-debian12:nonroot` | nothing from context — `COPY --from=builder` | `/spaxel`, `/spaxel-sim`, `/firmware/spaxel-firmware-<VERSION>.bin`, `/firmware/serial/spaxel-firmware-<VERSION>-merged.bin` |

Consequences of the root-context choice, all verified against the tracked
tree (paths exist; `//go:embed dashboard` at `mothership/cmd/mothership/dashboard_embed.go:11`):

- The **Go module root inside the image is `/app`, i.e. repo `mothership/`** —
  the module files are copied explicitly and `./cmd/mothership` / `./cmd/sim`
  are resolved from there.
- The **root `go.work` is never copied** — no `COPY` names it — so the image
  build compiles the `mothership` module standalone and does not inherit the
  root-workspace breakage that makes `./...` from the repo root fail in CI.
- `dashboard/` must be present in the context or the `-tags=embed` build
  fails; it is copied to the exact path the embed directive expects.
- Building from `mothership/` instead would fail twice over: no `Dockerfile`
  at that level, and `COPY mothership/ ./` would not resolve.

## Gap found: the build passes no `VERSION` build-arg

`docker build -t spaxel-e2e:test .` (src 347) carries no `--build-arg`, so the
Dockerfile's `ARG VERSION=dev` default applies and stage 1 fetches
`…/releases/download/vdev/spaxel-firmware-dev-merged.bin` → **404 every run**.
The production path does not have this problem: `spaxel-build-workflowtemplate.yml`
builds the *same* root Dockerfile from `/tmp/repo` and passes
`--build-arg=VERSION={{inputs.parameters.version}}` (its line 372).

This is the template-side mechanism behind the already-documented CI red
("docker-e2e firmware-fetch 404"): the e2e build cannot succeed as written
unless a `vdev` release existed, and none does. Recorded here because the
build-context inventory is where the omission is visible; the fix belongs to
whatever bead owns that red, not to this audit.

## Statement for the final report (bf-2yur)

One Docker build in the template: context `/tmp/spaxel-src` (repo root, via
`cd` at src 346), implicit root `Dockerfile`, tag `spaxel-e2e:test`, executed
by the privileged `docker:dind` sidecar over a shared `emptyDir` socket. One
tracked Dockerfile exists in the repo and it is that one. The build takes no
build-args, which pins the firmware-fetch stage to a non-existent `vdev`
release.
