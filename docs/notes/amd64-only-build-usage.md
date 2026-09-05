# spaxel-build — amd64-only usage guide

**Date:** 2026-09-04
**Bead:** `spaxel-723a3646` (usage documentation)
**Companion to:** `docs/notes/amd64-only-build-template-design.md` (the design and
implementation record for the same change — that document explains *why*; this one
explains *how to use it*).
**Verified live:** 2026-09-04 against `WorkflowTemplate/spaxel-build` in namespace
`argo-workflows` on iad-ci via the credential-free read-only endpoint.
**Updated 2026-09-04** (`spaxel-fce2f7e4`, declarative-config `0ec96fc9`):
`update-declarative-config`'s sed now takes its image ref from
`{{workflow.parameters.image-repo}}` instead of a hardcoded `ronaldraygun/spaxel:`,
so a scratch-repo run can no longer rewrite the production pin at all. §3.2's
scratch-repo rule is unchanged and still required — it is what keeps the
single-platform manifest out of the production *tags* — but the version-matching
rule below it is no longer load-bearing for pin safety.

---

## 1. Template name and path (AC1)

There is **no separate amd64-only template**. An earlier fork named
`spaxel-build-amd64` existed for one day (2026-09-01 → 2026-09-04) and was **deleted**
— amd64-only is now a *parameter* on the original template, not a different template.

| What | Value |
|---|---|
| Template name | `spaxel-build` (the original; unchanged) |
| Manifest path (declarative-config) | `jedarden/declarative-config` → `k8s/iad-ci/argo-workflows/spaxel-build-workflowtemplate.yml` |
| Live object | `WorkflowTemplate/spaxel-build`, namespace `argo-workflows`, cluster `iad-ci`, ArgoCD app `argo-workflows-ns-iad-ci` |
| Entrypoint | `build` |
| Steps | `resolve-version`, `golangci-lint`, `a11y-test`, `go-test`, `timing-benchmark`, `acceptance-test`, `firmware-test`, `firmware-build`, `docker-build`, `update-declarative-config` |
| amd64-only knob | workflow-level parameter **`platforms`** |
| Manifest change commits | declarative-config `a979e063` (add `platforms`), `e30e76f0` (delete the fork manifest) |

Confirm the live definition any time with:

```bash
kubectl --server=http://traefik-iad-ci:8001 \
  get workflowtemplate spaxel-build -n argo-workflows -o yaml
```

The deleted fork is expected to return `NotFound`:

```bash
kubectl --server=http://traefik-iad-ci:8001 \
  get workflowtemplate spaxel-build-amd64 -n argo-workflows -o name
# Error from server (NotFound) ...  ← correct state; do not recreate it
```

---

## 2. Parameters (AC2)

### 2.1 Workflow-level parameters (overridable at submit)

All four are declared at `spec.arguments.parameters` and are optional — the push
sensor passes only `git-repo` and `image-repo`, so every value below is its default
unless you override it.

| Parameter | Default | Effect |
|---|---|---|
| `git-repo` | `jedarden/spaxel` | Forgejo repo cloned and built. |
| `image-repo` | `ronaldraygun/spaxel` | Destination repository for `:version` and `:latest` tags. Also names the `:buildcache` cache ref. |
| `branch` | `main` | Ref `resolve-version` checks out; drives version resolution and the doc-only `should-build` gate. |
| `platforms` | `linux/amd64,linux/arm64` | **The amd64-only knob.** Substituted verbatim into `docker-build`'s buildx invocation as `--platform={{workflow.parameters.platforms}}`. Any buildx-accepted comma-separated platform list is valid. |

### 2.2 The `platforms` parameter in detail

| Property | Value |
|---|---|
| Scope | workflow-level (`spec.arguments.parameters`), so overridable under a `workflowTemplateRef` |
| Type | string; comma-separated buildx platform list |
| Default | `linux/amd64,linux/arm64` — byte-identical to the literal that was there before, so the default reproduces the original multi-arch behaviour exactly |
| Consumed at | `docker-build` only, once |
| amd64-only value | `linux/amd64` |
| Validation | none — the value is passed to `--platform` verbatim, which is why it must carry the `linux/` OS half. Bare `amd64` is rejected by buildx. |

Consumed by `docker-build` here:

```
docker buildx build \
  --platform={{workflow.parameters.platforms}} \
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

### 2.3 Parameters that are *not* submitter-suppliable

`version` is the one template-level (inter-step) parameter. It has no default and
cannot be supplied by the submitter — `resolve-version` always derives it from the
repo's `VERSION` file and forwards it downstream. It is the image tag, not a platform
knob; it is unaffected by `platforms`. There is deliberately no per-architecture
version.

### 2.4 Steps unaffected by `platforms` (census: all of them except `docker-build`)

| Step | Affected? | Why |
|---|---|---|
| `resolve-version` | no | Version is resolved once and applied to every platform. |
| `golangci-lint`, `a11y-test`, `go-test`, `timing-benchmark`, `acceptance-test`, `firmware-test` | no | Test the source in amd64 CI containers; they do not inspect the image's target platforms. |
| `firmware-build` | no | ESP32-S3 is the *hardware* target — an independent axis by ADR-001. Firmware is built once and fetched by every image platform, so it is unaffected by which image platforms are built. |
| `update-declarative-config` | no | Its sed rewrites image pins matching `{{workflow.parameters.image-repo}}` (since `spaxel-fce2f7e4`); a scratch-repo run matches nothing in the production files. |

---

## 3. Example usage (AC3)

### 3.1 Trigger an amd64-only build

```bash
kubectl --kubeconfig=/home/coding/.kube/iad-ci.kubeconfig create -f - <<EOF
apiVersion: argoproj.io/v1alpha1
kind: Workflow
metadata:
  generateName: spaxel-build-amd64only-
  namespace: argo-workflows
spec:
  arguments:
    parameters:
      - name: platforms
        value: linux/amd64
  workflowTemplateRef:
    name: spaxel-build
EOF
```

Only `platforms` is needed; the other three parameters fall back to their defaults.

### 3.2 The same submission, made safe for testing (always use this unless you are cutting a real release)

An amd64-only run is still the **full release path**: it pushes `:version` and
`:latest` to `image-repo`. Since `spaxel-fce2f7e4` (declarative-config `0ec96fc9`)
`update-declarative-config` matches its sed against
`{{workflow.parameters.image-repo}}` rather than a hardcoded literal, so a run
pointed at a scratch repository can no longer rewrite the production pin even when
its resolved version differs from the published one. Passing a scratch repository is
still required — that is what keeps a single-platform manifest out of the production
*tags* — and leaving `branch` at its default keeps the run's resolved version the
published one:

```bash
kubectl --kubeconfig=/home/coding/.kube/iad-ci.kubeconfig create -f - <<EOF
apiVersion: argoproj.io/v1alpha1
kind: Workflow
metadata:
  generateName: spaxel-build-amd64-verify-
  namespace: argo-workflows
spec:
  arguments:
    parameters:
      - name: platforms
        value: linux/amd64
      - name: image-repo
        value: ronaldraygun/spaxel-amd64verify   # scratch, not production
  workflowTemplateRef:
    name: spaxel-build
EOF
```

Rules for a non-release amd64-only submission:

1. **`image-repo` → a scratch repository** (e.g. `ronaldraygun/spaxel-amd64verify`).
   The production `:version`/`:latest` tags stay untouched — this is the part that
   stops an amd64-only single-platform manifest replacing the multi-arch manifest
   list under the production tags.
2. **`branch` left at `main` (the default).** Not a pin-safety rule any more — the sed
   follows `image-repo`, so it matches nothing in the production files under a scratch
   repo whatever version resolves — but it keeps `resolve-version` landing on the
   already-published version, which makes the run's output predictable and comparable
   against a real release.
3. **Pin safety no longer depends on `should-build`.** Before `spaxel-fce2f7e4`
   (declarative-config `0ec96fc9`), a substantive commit on `main` set
   `should-build=true` and the sed then rewrote the production pin even under a scratch
   repo — a run was only inert-by-construction when `should-build=false`. That residual
   risk is closed: the sed's pattern is now `{{workflow.parameters.image-repo}}`, and no
   production file references a scratch repository. The verified example below predates
   the fix and remains a valid demonstration that an inert run still proves Argo
   accepted and resolved the override.

Verified example (predates the fix, still valid): workflow
`spaxel-build-amd64-verify-28wp5` (2026-09-04) submitted with `platforms=linux/amd64`
+ the scratch `image-repo` resolved `should-build=false`, so every build step skipped
— no image, no firmware upload, no pin rewrite — while still proving that Argo
accepted and resolved the parameter override.

### 3.3 The default path (unchanged, for comparison)

Every push-triggered run is unaffected. The push sensor (`spaxel-push` →
`k8s/iad-ci/argo-events/spaxel-sensor.yml`) submits with only `git-repo` and
`image-repo`, so `platforms` falls back to `linux/amd64,linux/arm64` and the rendered
`--platform` flag is byte-identical to the pre-change literal. Sensor-triggered runs
continue to publish a two-platform manifest list — verify with the
`imagetools inspect` output in `docker-build`'s own logs.

### 3.4 Other legal values

`platforms` is not boolean. It passes straight into `--platform`, so:

| Value | Effect |
|---|---|
| `linux/amd64` | amd64-only (this capability) |
| `linux/arm64` | arm64-only — useful for a Pi-focused verification build |
| `linux/amd64,linux/arm64` | the default; multi-arch manifest list (ADR-001) |
| `linux/amd64,linux/arm/v7` | any buildx-accepted list works; nothing in the template constrains the set |

Do not write bare `amd64` — buildx requires the `linux/` OS half.

---

## 4. Differences from the original template behaviour (AC4)

The original `spaxel-build` template **is still the only template**; its default
behaviour is unchanged. What changed is the mechanism by which a non-default platform
set is requested.

| Aspect | Before 2026-09-04 | Now |
|---|---|---|
| How to request amd64-only | Submit `spaxel-build-amd64` (a separate ~28 KB copy of the template, added 2026-09-01) | Submit `spaxel-build` with `--parameter platforms=linux/amd64` |
| Workflow-level parameters | 3 (`git-repo`, `image-repo`, `branch`) | 4 (`+ platforms`) |
| `docker-build` platform flag | `--platform=linux/amd64,linux/arm64` literal | `--platform={{workflow.parameters.platforms}}` |
| Default (sensor-triggered) behaviour | multi-arch manifest list | **identical** multi-arch manifest list — the default value is the old literal |
| `spaxel-build-amd64` fork | existed, referenced by nothing, and had already rotted (its `resolve-version` was missing its `outputs` block, so the template could not run) | deleted; live object is `NotFound` |
| Reachability | the fork was unreachable by the push sensor (which targets `spaxel-build` only) | irrelevant — same template, sensor unchanged |
| Image tags pushed | `<repo>:<version>` + `:latest` | unchanged |
| Firmware step | built once, ESP32 target, platform-independent | unchanged and deliberately not coupled to `platforms` |

**Replaces or complements?** The parameter **replaces** the fork template. It
complements nothing: there is no second template to keep in mind, and the fork should
not be recreated (the design doc §1 records why — a second 28 KB copy drifted within
24 hours, was referenced by nothing, and a fork pushing the same tags with a
different platform set would overwrite the production multi-arch manifest).

**Why this shape:** `platforms` is the buildx flag's own name, so the value needs no
translation layer; a parameter scales to future requests (`arm64-only`, a third
platform) where a naming scheme would need another fork; and the defaulted parameter
makes the change invisible to every existing submitter, including the push sensor.

---

## 5. Operational caveats

1. **Same-tag hazard.** Any submission pushes `:version` and `:latest` to
   `image-repo`. A manifest list and a single-platform manifest are both legal
   contents for one tag and the later push wins, so an amd64-only run pointed at
   production silently downgrades that version's manifest to amd64 — breaking
   arm64 consumers (the plan's Raspberry Pi reference platform). Use the §3.2 recipe.
2. **Production pin rewrite is scoped to `image-repo`** (since `spaxel-fce2f7e4`,
   declarative-config `0ec96fc9`) — the sed matches only the repo you passed, so a
   scratch-repo run cannot touch the production pin. **The same-tag hazard above is
   therefore the one remaining way a scratch-less amd64-only run damages production.**
3. **ADR-001 governs the default.** The shipping shape remains the multi-arch
   manifest list. Making amd64-only the *default* would supersede ADR-001 and needs
   a new ADR plus a `docs/plan/plan.md` edit — not a default flip inside the
   template.
4. **ArgoCD owns the manifest.** All template changes go through a
   declarative-config commit; direct `kubectl apply` against the live object is
   prohibited and reverted by `selfHeal`.
5. **Template changes are validated by the sync**, not by `--dry-run` (a prohibited
   verb here, and unnecessary): Argo rejects a malformed template at reconcile, and a
   broken template cannot create workflows.

---

## 6. Related documents

| Document | Relationship |
|---|---|
| `docs/notes/amd64-only-build-template-design.md` | The design decision, the full diff, the implementation record, and why the fork was deleted. |
| `docs/research/spaxel-build-workflow-parameters.md` | The complete parameter reference for `spaxel-build` (now including `platforms`). |
| `docs/research/spaxel-build-architecture-targeting.md` | Research on how amd64/arm64 targeting worked when a separate fork existed; describes the deleted fork as it was. |
| `docs/research/docker-build-step-configuration.md` | Full anatomy of the `docker-build` step. |
| `docs/notes/ci-doc-only-push-path-filter.md` | The push sensor and its path filter (why sensor runs are unaffected). |
| `docs/plan/plan.md` ADR-001 | The architectural decision that makes multi-arch the default and decouples the ESP32 firmware build from the image build. |
