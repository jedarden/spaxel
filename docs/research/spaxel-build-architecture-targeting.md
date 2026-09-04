# spaxel-build WorkflowTemplate — Architecture Targeting

**Date:** 2026-09-02
**Scope:** Read-only research for bead `spaxel-36bcfae8` — locate the template, explain how
amd64/arm64 targeting works, and identify what restricts the build to amd64-only.
No templates were modified.

> **Status (2026-09-04): the amd64-only fork described below is deleted.**
> `k8s/iad-ci/argo-workflows/spaxel-build-amd64-workflowtemplate.yml` was removed
> (declarative-config commit `e30e76f0`) and amd64-only became a submit-time
> parameter on the original template instead. See
> `docs/notes/amd64-only-build-usage.md` for current usage and
> `docs/notes/amd64-only-build-template-design.md` for the decision record. The
> rest of this document is retained as the record of the targeting mechanism and of
> why the fork route was abandoned.

---

## 1. Location

The template lives in the **declarative-config** repo (not this one) and is applied to the
`iad-ci` cluster by ArgoCD:

| What | Value |
|---|---|
| Manifest (multi-arch, authoritative) | `jedarden/declarative-config` → `k8s/iad-ci/argo-workflows/spaxel-build-workflowtemplate.yml` (28,512 B) |
| Manifest (amd64-only variant) | `jedarden/declarative-config` → `k8s/iad-ci/argo-workflows/spaxel-build-amd64-workflowtemplate.yml` (28,447 B, added 2026-09-01, commit `91fee0a1824e` "feat(spaxel): add amd64-only workflow template") |
| Live objects | `WorkflowTemplate/spaxel-build` and `WorkflowTemplate/spaxel-build-amd64` in namespace `argo-workflows` on iad-ci, ArgoCD instance label `argo-workflows-ns-iad-ci` |
| Push trigger | `jedarden/declarative-config` → `k8s/iad-ci/argo-events/spaxel-sensor.yml` (`spaxel-push` event, Lua path filter) — targets **`spaxel-build` only** (see `docs/notes/ci-doc-only-push-path-filter.md`) |

**Filename correction:** `docs/research/spaxel-build-workflow-parameters.md:156` cites
`k8s/iad-ci/argo-workflows/spaxel-build.yml` — that file does not exist. The Forgejo
contents API for `k8s/iad-ci/argo-workflows/` lists exactly three spaxel-related files:
`spaxel-build-workflowtemplate.yml`, `spaxel-build-amd64-workflowtemplate.yml`, and
`spaxel-e2e-workflowtemplate.yml`.

Verify the live definition with:

```bash
kubectl --server=http://traefik-iad-ci:8001 \
  get workflowtemplate spaxel-build -n argo-workflows -o yaml
```

---

## 2. Workflow structure (architecture-relevant steps)

```
resolve-version                    # clone, doc-only path filter, VERSION bump/resolution
  ├─ lint / a11y-test              # gate: should-build == true
  ├─ go-test / timing-benchmark
  ├─ acceptance-test               # continueOn.failed
  ├─ firmware-test
  ├─ build-firmware  ──► GitHub Releases (versioned ESP32-S3 artifact)
  ├─ docker-build    ──► ronaldraygun/spaxel:<version> + :latest  ◄── ARCHITECTURE LIVES HERE
  └─ update-declarative-config     # rewrites image pins (arch-agnostic)
```

- **`firmware-build`** runs `espressif/idf:v5.2`, compiles the ESP32-S3 image, and uploads
  `spaxel-firmware-<version>-merged.bin` to GitHub Releases. Its output targets the ESP32,
  so it is independent of the mothership image architecture. This is the ADR-001
  decoupling, implemented: the image build no longer compiles firmware.
- **`docker-build`** runs `docker:29.7.2-dind` (privileged) with a buildx
  `docker-container` driver builder and pushes a manifest list.

---

## 3. Where architecture targeting is configured

**There is no architecture parameter anywhere in the template.** The only workflow-level
parameters are `git-repo` (`jedarden/spaxel`), `image-repo` (`ronaldraygun/spaxel`), and
`branch` (`main`), plus the step-local `version`/`should-build` outputs. Architecture is a
**hardcoded literal** in the `docker-build` step's buildx invocation:

```dockerfile
docker buildx build \
  --platform=linux/amd64,linux/arm64 \        # ← the single architecture knob
  --build-arg=VERSION={{inputs.parameters.version}} \
  --tag={{workflow.parameters.image-repo}}:{{inputs.parameters.version}} \
  --tag={{workflow.parameters.image-repo}}:latest \
  --push ...
```

So by default every substantive push publishes a **multi-arch manifest list
(amd64 + arm64)**. It cannot be narrowed at submit time today.

**Why both platforms build without per-arch builders or QEMU** — the repo `Dockerfile`
does the cross-platform work:

- Stage 1 (`firmware-fetcher`, alpine) downloads the firmware from GitHub Releases.
- Stage 2 (`builder`) is `FROM --platform=$BUILDPLATFORM golang:1.25-bookworm` and
  cross-compiles with `GOOS=linux GOARCH=$TARGETARCH` — `TARGETARCH` is injected by
  buildx per platform leg. (`ARG TARGETPLATFORM` is declared and defaulted but otherwise
  unused; the Dockerfile comment at its top explains the defaults exist so standalone
  kaniko/local builds don't see empty values.)
- Stage 3 (distroless) is COPY-only — no `RUN`, so nothing executes under the target
  platform.

Every `RUN` therefore executes on the builder's native platform; the target platform only
labels the resulting manifest entries. The template's comment "Buildx will build each
platform natively (no QEMU emulation)" is true in effect, but via Go cross-compilation,
not because two native builders exist.

---

## 4. Restricting the build to amd64-only

The knob is the literal `--platform` value. Restriction options, in increasing order of
invasiveness:

1. **Edit the flag in place** (spaxel-build): `--platform=linux/amd64,linux/arm64` →
   `--platform=linux/amd64` in `k8s/iad-ci/argo-workflows/spaxel-build-workflowtemplate.yml`.
   One line; loses arm64 entirely.
2. **Use the existing amd64-only fork**: `spaxel-build-amd64-workflowtemplate.yml` already
   contains exactly this change — its `docker-build` step is identical to the multi-arch
   template except `--platform=linux/amd64`, the builder name (`amd64-builder` vs
   `multiarch-builder`), and log text. A spec-level diff of the two live templates shows
   **no other functional difference**. To make it the shipping path you would point the
   argo-events sensor at it (or replace `spaxel-build` with its content) — but see
   finding (1) below first: **it is currently broken.**
3. **Parameterize (recommended long-term)**: add a workflow parameter, e.g.
   `platforms` (default `linux/amd64,linux/arm64`), consumed as
   `--platform={{workflow.parameters.platforms}}`. Then amd64-only becomes a submit-time
   argument and no template fork needs maintaining. This is the only way to restrict
   platforms without editing declarative-config, given the flag is a literal today.

Nothing else in the pipeline is architecture-conditional: `firmware-build` and
`update-declarative-config` are unchanged by arch (the latter only rewrites
`ronaldraygun/spaxel:*` image pins).

---

## 5. Findings / caveats

1. **`spaxel-build-amd64` is broken as applied.** Compared to `spaxel-build`, its
   `resolve-version` template's `outputs` block (`version` and `should-build`) and its
   `activeDeadlineSeconds: 300` are indented one level too deep: they sit inside the
   `script:` block (8 spaces) instead of at template scope (6 spaces). The API server
   prunes those unknown script fields at apply time, so the live template carries
   **no** template-level outputs and no backstop deadline (verified live 2026-09-03
   against `workflowtemplate spaxel-build-amd64` on iad-ci; the multi-arch template
   is unaffected). The entrypoint is unchanged, so it
   still passes `{{steps.resolve-version.outputs.parameters.version}}` to
   `firmware-build`/`docker-build`/`update-declarative-config` and gates every step on
   `{{steps.resolve-version.outputs.parameters.should-build}} == true`. A run cannot
   resolve those references. This looks like an editing accident when the fork was created.
   If the amd64 variant is to be used, re-outdent the block — or regenerate the fork from
   the current multi-arch manifest and re-apply the one-line platform change.
2. **The amd64 variant has never run and nothing triggers it.** A cluster-wide census of
   `spec.workflowTemplateRef` shows 0 workflows referencing `spaxel-build-amd64`; the push
   sensor targets `spaxel-build` only (its manifest lives outside this repo's readable
   scope; the sensor itself is not listable by the read-only service account).
3. **CI currently fails before the image build, for unrelated reasons.** Runs
   `spaxel-build-sw77s`, `spaxel-build-9jmvc`, and `spaxel-build-ml2wd` (2026-09-02) failed
   at `lint` (golangci-lint exit 1) and `a11y-test`. Because those run before
   `build-firmware`/`docker-build` in the DAG, no image is being published at the moment
   regardless of architecture.
4. The root `CLAUDE.md` WorkflowTemplate table lists only `spaxel-build`; the 2026-09-01
   amd64 variant is not reflected there (minor doc drift if the variant survives).

---

## 6. Verification commands

```bash
# Live template definitions (multi-arch + amd64 fork)
kubectl --server=http://traefik-iad-ci:8001 get workflowtemplate spaxel-build \
  -n argo-workflows -o yaml
kubectl --server=http://traefik-iad-ci:8001 get workflowtemplate spaxel-build-amd64 \
  -n argo-workflows -o yaml

# Which template recent runs actually used
kubectl --server=http://traefik-iad-ci:8001 get workflows -n argo-workflows -o json \
  | jq -r '.items[] | select(.metadata.name|startswith("spaxel-build")) |
           "\(.metadata.name) \(.spec.workflowTemplateRef.name) \(.status.phase)"'

# Manifest filenames/sizes in declarative-config (Forgejo contents API)
curl -sS -H "Authorization: token $FORGEJO_TOKEN" \
  "https://git.ardenone.com/api/v1/repos/jedarden/declarative-config/contents/k8s/iad-ci/argo-workflows"
```

## 7. Related documents

- `docs/research/docker-build-step-configuration.md` — deep dive on the `docker-build` step (DinD, buildx, secrets)
- `docs/research/spaxel-build-workflow-parameters.md` — parameter reference (note the `spaxel-build.yml` filename error in its "Related Documentation" section)
- `docs/notes/ci-doc-only-push-path-filter.md` — the push sensor and its path filter
- `docs/plan/plan.md` → ADR-001 — firmware/image decoupling rationale
