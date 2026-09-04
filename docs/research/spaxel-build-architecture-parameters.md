# spaxel-build WorkflowTemplate — Architecture / Platform / Build-Target Parameters

**Date:** 2026-09-04
**Scope:** Read-only research for bead `spaxel-703df68d`. Acceptance: list the
architecture/platform parameters of the `spaxel-build` WorkflowTemplate, document how
each is used, and note defaults and constraints. No templates were modified.

**Verification basis:** the *live* `WorkflowTemplate/spaxel-build` and
`WorkflowTemplate/spaxel-build-amd64` objects in `argo-workflows` on iad-ci
(read via the credential-free endpoint, 2026-09-04), plus this repo's `Dockerfile`
at the same date. Line references to the template are given by step and field, since
the authoritative manifest lives in `jedarden/declarative-config`, outside this repo.

---

## 0. The answer in one line

**There is no architecture or platform parameter anywhere in the template.** The
template exposes exactly three workflow-level parameters (`git-repo`, `image-repo`,
`branch`), and none of them selects a platform. Architecture targeting is a **hardcoded
literal** inside the `docker-build` step's buildx invocation, and the hardware build
target is a **hardcoded literal** (`esp32s3`) inside `firmware-build`. Everything else
that looks like an architecture knob is a Dockerfile build-arg, which is not a template
parameter and cannot be set at workflow-submit time.

So "list the architecture parameters" resolves to: enumerate the three real parameters,
state explicitly that none is architectural, and then inventory the *literals and build
args* that actually carry platform semantics — because those are what an operator has to
edit (or fork a template over) to change a target.

---

## 1. Workflow-level parameters (the only true parameters)

Overridable at submit time via `spec.arguments.parameters` under a
`workflowTemplateRef`. Verified live 2026-09-04.

| Parameter | Default | Used by | Architecture relevance |
|---|---|---|---|
| `git-repo` | `jedarden/spaxel` | `firmware-build`, `docker-build`, `resolve-version` (clone URL, release repo) | None. Selects the *source*, not the target. |
| `image-repo` | `ronaldraygun/spaxel` | `docker-build` (`--tag`, `--cache-to/from`), `update-declarative-config` | None. Selects the *registry*, not the platform. The same value is pushed for every architecture in the manifest list. |
| `branch` | `main` | `firmware-build`, `docker-build`, `resolve-version` (clone ref) | None. |

There is no `platforms`, `arch`, `goarch`, or `target` parameter. Adding one is the
recommended way to make amd64-only a submit-time choice — see §6.

---

## 2. Where platform semantics actually live (hardcoded literals)

These are the values that determine what gets built. They are **not parameters**: each
is a literal in the template body, so changing any of them means editing
`jedarden/declarative-config` → `k8s/iad-ci/argo-workflows/spaxel-build-workflowtemplate.yml`
and letting ArgoCD sync.

### 2.1 `docker-build` — container image platforms

```sh
docker buildx create --use --name multiarch-builder --driver docker-container
docker buildx build \
  --platform=linux/amd64,linux/arm64 \        # ← the single image-platform knob
  --build-arg=VERSION={{inputs.parameters.version}} \
  --tag={{workflow.parameters.image-repo}}:{{inputs.parameters.version}} \
  --tag={{workflow.parameters.image-repo}}:latest \
  --push \
  --cache-to=type=registry,ref={{workflow.parameters.image-repo}}:buildcache,mode=max \
  --cache-from=type=registry,ref={{workflow.parameters.image-repo}}:buildcache \
  --file=Dockerfile .
```

| Literal | Default | How it is used | Constraints |
|---|---|---|---|
| `--platform` | `linux/amd64,linux/arm64` | Drives one buildx build leg per platform; result is a **multi-arch manifest list** pushed under both tags. | Only knob in the whole pipeline; a literal, so it cannot be narrowed at submit time. No QEMU is involved — see §3. |
| `--name multiarch-builder` | `multiarch-builder` | Names the buildx `docker-container`-driver builder instance. | Name only, but the amd64 fork uses `amd64-builder`, so tooling that greps for a builder name sees different values per template. |
| `--tag ...:<version>` / `:latest` | from `image-repo` + resolved `version` | One manifest list covers both tags; tags are arch-agnostic. | Constraint: `:latest` is written on every substantive build. Org rule forbids `:latest` in *deployment manifests*; the tag itself is produced here regardless. |
| `:buildcache` | registry cache ref | Layer cache, `mode=max`. | Arch-independent ref: both platforms share one cache entry namespace. |
| `image: docker:29.7.2-dind` + `privileged: true` | — | DinD provides the dockerd that buildx needs. | Not arch-selective, but the CI node pool is amd64 (see §5 note on `gh`). |

### 2.2 `firmware-build` — hardware build target

```sh
idf.py set-target esp32s3
idf.py build
python -m esptool --chip esp32s3 merge_bin \
  --flash_mode dio --flash_freq 80m --flash_size 4MB \
  --output build/spaxel-firmware-{{version}}-merged.bin ...
```

| Literal | Default | How it is used | Constraints |
|---|---|---|---|
| `set-target esp32s3` | `esp32s3` | Selects the ESP32-S3 as the chip target for the whole ESP-IDF build. | The firmware build target is independent of the image platform by design (ADR-001). One firmware artifact serves every host architecture. |
| `--chip esp32s3` (merge_bin) | `esp32s3` | Must match the `set-target` value or `merge_bin` refuses. | Drift between the two literals is a build break, not a silent mis-target. |
| `--flash_size 4MB` | `4MB` | Partitions the merged image for a 4 MB flash. | Must agree with `partitions.csv` and `CONFIG_ESPTOOLPY_FLASHSIZE_16MB` expectations in sdkconfig; the size check in the same step gates each app slot against the A/B table. |
| `--flash_mode dio`, `--flash_freq 80m` | `dio` / `80m` | Flash mode and clock for the merged image. | Pair with bootloader config; changing either is a hardware-compat change, not a CI preference. |
| `gh_${GH_VERSION}_linux_amd64.tar.gz` | `linux_amd64` | Installs the `gh` CLI used to publish the release. | **amd64-only literal**, and a latent constraint: it assumes the CI runner is amd64. It constrains the *runner*, not the artifact — but it means this step cannot simply be moved to an arm64 runner. |
| Runner image | `espressif/idf:v5.2` | Provides the ESP-IDF toolchain. | ESP-IDF is x86_64-only; this is why firmware is built once and fetched by all image platforms. |

### 2.3 `resolve-version` — the build *target version* (affects both artifacts)

Not an architecture parameter, but it is the third axis of "build target" and it is
the only parameter that flows between steps.

| Item | Default | How it is used | Constraints |
|---|---|---|---|
| `version` (step output) | resolved per push | Consumed by `firmware-build`, `docker-build`, `update-declarative-config`, `golangci-lint`, `a11y-test` | A `VERSION` change in the commit wins; otherwise patch is auto-bumped and committed back (`ci: auto-bump version to X.Y.Z`). |
| `should-build` (step output) | `true`/`false` | Gates every build/test step via `when`. | Doc/bead-only pushes still resolve a `version` (needed so downstream references are non-empty) but skip the builds. |
| Path filter | `^docs/`, `^\.beads/`, `^\.needle`, `\.md$`, `^LICENSE$`, `^\.gitignore$` | Anything outside these patterns is "substantive" and triggers a full build. | The bump commit itself lands on the mirror only, so a re-run resolves the same base rather than compounding. |

---

## 3. Dockerfile build-args (the actual mechanism — not template parameters)

The repo `Dockerfile` is what makes both platforms buildable without per-arch builders
or QEMU. These `ARG`s are populated by buildx per platform leg; CI passes only
`--build-arg=VERSION`.

| Build arg | Declared default | Where consumed | Notes / constraints |
|---|---|---|---|
| `TARGETARCH` | top-level: *(unset)*; stages 2 and 3: `amd64` | Stage 2 only — both `go build` runs use `GOOS=linux GOARCH=$TARGETARCH` | The single load-bearing arch variable. buildx injects it from `--platform`; the `amd64` default means a plain `docker build` with no `--platform` silently produces an amd64 image. |
| `TARGETPLATFORM` | top-level: *(unset)*; stage 2: `linux/amd64` | **Declared, not consumed.** | Present so kaniko/local builds don't see an empty value. Dead beyond its default — a candidate for removal, kept for that guard. |
| `BUILDPLATFORM` | `linux/amd64` | `FROM --platform=$BUILDPLATFORM golang:1.25-bookworm AS builder` | Pins the *build* platform to the builder's native platform so Go cross-compiles instead of emulating. The default matches the current amd64 CI runner. |
| `VERSION` | `dev` | ldflags `-X main.version`; also the firmware filename in stage 3 (`/firmware/spaxel-firmware-${VERSION}.bin`, `/firmware/serial/...`) | The filename *is* the OTA store's version source, so this arg has functional weight beyond labeling. |
| `GOOS` / `GOARCH` (env in `RUN`) | `linux` / `$TARGETARCH` | Both stage-2 builds (`spaxel`, `spaxel-sim`) | `GOOS` is hardcoded `linux` — there is no non-Linux image target, ever. |

**Why no QEMU is needed:** every `RUN` in the image executes on `$BUILDPLATFORM`
(native); the target platform only labels the resulting manifest entry and is consumed
as `GOARCH=$TARGETARCH`. The template's comment "Buildx will build each platform
natively (no QEMU emulation)" is accurate in effect, but via Go cross-compilation, not
because two native builders exist.

**Stage 3 note:** `ARG TARGETARCH=amd64` is declared in the distroless stage but never
used (that stage is COPY-only, so nothing executes under the target platform). The
declaration is inert.

---

## 4. Related templates and variants

| Template | Platform behaviour | Status 2026-09-04 |
|---|---|---|
| `spaxel-build` | `--platform=linux/amd64,linux/arm64` (multi-arch manifest list) | Authoritative; the push sensor targets this one only. |
| `spaxel-build-amd64` | identical except `--platform=linux/amd64` and builder name `amd64-builder` | **Broken as applied, verified live today:** its `resolve-version` template still has **no template-scope `outputs`** (`version`, `should-build` are absent) — the same mis-indentation recorded in `spaxel-build-architecture-targeting.md` §5.1, still present. Its entrypoint still references `{{steps.resolve-version.outputs.parameters.*}}`, so any run would fail to resolve them. It has never run and nothing triggers it. |

Treat the amd64 variant as documentation of *intent*, not as a usable amd64-only path.
The usable paths today are: edit the literal (option 1 of the targeting doc) or
parameterize (§6 below).

---

## 5. Constraints summary

1. **Platform cannot be selected at submit time.** With no architecture parameter, the
   only way to build fewer platforms is to edit the `--platform` literal in
   declarative-config, or use the (currently broken) amd64 fork.
2. **Both platforms always publish, or neither does.** A single `docker buildx build`
   produces the whole manifest list; there is no per-arch skip, and a failure in either
   leg fails the step (subject to its `retryStrategy`: limit 2, 30 s backoff, factor 2,
   OnError).
3. **Firmware is platform-independent; images are not.** One ESP32-S3 binary is
   published per version and fetched by every image platform (ADR-001). Firmware target
   (`esp32s3`) and image platforms (`amd64`/`arm64`) vary on independent axes and must
   not be conflated.
4. **The defaults favour amd64.** `BUILDPLATFORM=linux/amd64`, `TARGETARCH=amd64`,
   `TARGETPLATFORM=linux/amd64`, and the `gh` `linux_amd64` download all assume an amd64
   builder. Moving CI to arm64 runners requires touching all four, not just the last.
5. **`GOOS` is fixed to `linux`.** No Windows/macOS image target exists in the build
   definition; only the architecture axis is variable.
6. **Version resolution is not arch-aware.** `resolve-version` computes one version per
   push, applied identically to firmware and to both image platforms. There is no
   mechanism for an amd64-only release or a per-arch firmware/image version skew.
7. **Tags are arch-agnostic by design.** `ronaldraygun/spaxel:<version>` is a manifest
   list. Any consumer that needs a single-platform digest must resolve it
   (`docker buildx imagetools inspect`, which `docker-build` runs after push), not
   assume a host arch.

---

## 6. If a platform parameter is wanted

The recommended shape (targeting doc §4.3): add a workflow-level parameter with the
current literal as its default, and substitute it into the existing flag —

```yaml
# spec.arguments.parameters
- name: platforms
  value: linux/amd64,linux/arm64
# docker-build script
--platform={{workflow.parameters.platforms}} \
```

This makes amd64-only a submit-time argument (`--platform=linux/amd64` on submit),
retires the broken template fork, and requires no change to the Dockerfile (buildx
already injects `TARGETARCH` per leg). It is a declarative-config edit — ArgoCD syncs
it; nothing in this repo changes.

---

## 7. Related documents

- `docs/research/spaxel-build-architecture-targeting.md` — how targeting works and how
  to restrict it (this doc's §0/§4/§6 follow its findings; its live checks were
  re-verified 2026-09-04)
- `docs/research/spaxel-build-workflow-parameters.md` — full parameter reference:
  secrets, per-step resources, deadlines, retries (its "Related Documentation" filename
  is corrected by the targeting doc §1)
- `docs/research/docker-build-step-configuration.md` — DinD/buildx/cache mechanics of
  the `docker-build` step
- `docs/notes/ci-doc-only-push-path-filter.md` — the push sensor and why doc-only
  pushes skip builds
- `docs/plan/plan.md` → ADR-001 — firmware/image decoupling rationale
