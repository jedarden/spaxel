# spaxel-build — amd64-only restriction: design and implementation plan

**Date:** 2026-09-04
**Bead:** `spaxel-fd1f07e8` (design); implementation tracked by `spaxel-b0d532f0`
**Scope:** Design only. The authoritative template manifest lives in
`jedarden/declarative-config` → `k8s/iad-ci/argo-workflows/`, which is outside this
repo — nothing in *this* repo changes under this design except this document.
No template was modified while producing it.
**Usage:** the operator-facing how-to (submitting an amd64-only build, the full
parameter table, and the safety recipe) is now in
`docs/notes/amd64-only-build-usage.md` (`spaxel-723a3646`).

**Verification basis:** the live `WorkflowTemplate/spaxel-build` and
`WorkflowTemplate/spaxel-build-amd64` objects in `argo-workflows` on iad-ci, read via
the credential-free endpoint on 2026-09-04; this repo's `Dockerfile` the same day; and
the three prior research docs in this chain:
`docs/research/spaxel-build-architecture-parameters.md`,
`docs/research/spaxel-build-architecture-targeting.md`,
`docs/research/docker-build-step-configuration.md`.

---

## 0. The answer in one line

**Modify `spaxel-build` in place** by adding one workflow-level parameter —
`platforms`, default `linux/amd64,linux/arm64` — and substituting it into
`docker-build`'s buildx invocation as `--platform={{workflow.parameters.platforms}}`.
**Delete `spaxel-build-amd64`.** Do not maintain two templates, and do not flip the
default to amd64-only (ADR-001 would be regressed — see §7).

This is option (3) of the targeting doc's §4, which both prior research docs already
recommend. It is one added parameter and one changed line in a single manifest, it
makes amd64-only a *submit-time* argument rather than a fork, and its default value
reproduces today's behaviour byte-for-byte.

---

## 1. Decision — modify the existing template (AC1)

### 1.1 The two candidates

| | A. Modify `spaxel-build` (chosen) | B. Keep/create `spaxel-build-amd64` |
|---|---|---|
| Manifests to maintain | 1 | 2 (~28.5 KB each, near-identical) |
| amd64-only invocation | `--parameter platforms=linux/amd64` at submit | separate template ref |
| Default behaviour | unchanged (multi-arch) | unchanged, but only while the fork is left unused |
| Sensor changes | none (sensor targets `spaxel-build`; verified it passes only `git-repo`,`image-repo`) | fork is unreachable by the push sensor — it would need its own trigger, or the sensor would need re-pointing |
| Drift risk | none beyond the existing one | demonstrated: the fork rotted within a day of creation (§1.3) |
| Deploy rewrites | unchanged | unchanged, but an activated fork silently downgrades tags (§1.4) |

### 1.2 Two-template maintenance cost — already demonstrated, not hypothetical

The fork was added 2026-09-01 (commit `91fee0a1824e` in declarative-config) by copying
the multi-arch template and editing the platform literal. By 2026-09-02 the targeting
doc found it broken: `resolve-version`'s template-scope `outputs` (`version`,
`should-build`) and its `activeDeadlineSeconds: 300` were indented one level too deep,
the API server pruned them at apply time, and the entrypoint's
`{{steps.resolve-version.outputs.parameters.*}}` references became unresolvable. I
re-verified this live on 2026-09-04 — `resolve-version` in the fork still has no
`outputs` key at all:

```
[.spec.templates[] | select(.name=="resolve-version")] | .[0].outputs | keys
  → null                (spaxel-build-amd64)
  → ["parameters"]      (spaxel-build)
```

A second copy of a 28 KB template drifted within 24 hours and nothing noticed, because
nothing exercised it. That is the empirical answer to the bead's note that "a new
template may be cleaner."

### 1.3 The fork is unreachable

- The push sensor targets `spaxel-build` only (targeting doc §1; `docs/notes/ci-doc-only-push-path-filter.md`).
- Live census 2026-09-04: 79 workflows retained in `argo-workflows`, and the distinct
  `spec.workflowTemplateRef.name` set contains `spaxel-build` but **not**
  `spaxel-build-amd64` — zero references across everything the namespace still holds.

### 1.4 The fork is not merely dead — activating it would be destructive

Both templates push the **same** repository and the **same** tags
(`{{workflow.parameters.image-repo}}:{{version}}` and `:latest`). A manifest list and a
single-platform manifest are both legal contents for one tag; the later push wins.
So the first amd64-only fork run would overwrite the versioned multi-arch manifest for
that version, and any arm64 consumer (plan.md's Raspberry Pi reference platform)
pinned to it would break. Two templates that write the same tag with different
platform sets is a footgun, not a redundancy.

### 1.5 Naming

`spaxel-build-amd64` hardcodes one platform into the template's identity. The next
request ("arm64-only", "amd64 + riscv64") would need a third fork. A parameter scales;
the naming scheme does not.

### 1.6 Backward compatibility (bead note: "make parameter optional with default behaviour")

Satisfied by construction: the parameter is declared at `spec.arguments.parameters`
with the current literal as its default, so

- the push sensor — which passes only `git-repo` and `image-repo` (verified from the
  two most recent submitted workflows, `spaxel-build-46b4b`/`spaxel-build-nfwn2`) —
  needs **no change**; `branch` already demonstrates the defaulted-parameter pattern,
- every existing submission resolves to `linux/amd64,linux/arm64`, i.e. today's
  behaviour exactly (§3.2).

---

## 2. The parameter (AC2)

**Exact name: `platforms`.**

```yaml
# spec.arguments.parameters — added, position irrelevant
- name: platforms
  value: linux/amd64,linux/arm64
```

| Property | Value |
|---|---|
| Scope | workflow-level (`spec.arguments.parameters`), so overridable at submit under a `workflowTemplateRef` |
| Type | string; comma-separated buildx platform list |
| Default | `linux/amd64,linux/arm64` — identical to the current literal |
| Consumed at | `docker-build` only, once (§3) |
| Submit-time amd64-only value | `linux/amd64` |

**Why `platforms` and not the alternatives from the bead's notes:**

- `platforms` is the buildx flag's own name (`--platform=`), so the value passes into
  the flag **verbatim** with no translation or validation layer. Any buildx-accepted
  platform list is valid, and the parameter's semantics are exactly the flag's.
- `architectures` invites bare values (`amd64,arm64`) that buildx rejects — they need
  the `linux/<arch>` OS half — so it would require either translation or runtime
  validation failures that `platforms` sidesteps.
- `amd64_only` (boolean) hardcodes one platform as special, cannot express a list, and
  cannot be combined with a future second axis without inventing more booleans.
- Continuity: both prior research docs independently recommend `platforms`
  (parameters doc §6, targeting doc §4.3). Choosing it closes the chain instead of
  forking the vocabulary.

---

## 3. docker-build step changes (AC3)

### 3.1 The complete diff

One line inside the `docker-build` container's script (`spec.templates[] → name:
docker-build → container.args[0]`):

```diff
 docker buildx build \
-  --platform=linux/amd64,linux/arm64 \
+  --platform={{workflow.parameters.platforms}} \
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

plus the parameter declaration in §2. **That is the whole change.**

### 3.2 Backward compatibility proof

With the default in place the rendered command is byte-identical to today's:

```
--platform=linux/amd64,linux/arm64
```

Nothing else in the step changes:

| Element | Change | Why not |
|---|---|---|
| `docker buildx create --name multiarch-builder` | none | cosmetic; the amd64 fork's `amd64-builder` rename is exactly the kind of gratuitous divergence that made the fork hard to diff. Keep `multiarch-builder`. |
| `--build-arg=VERSION` | none | version axis, not platform |
| `--tag` / `--push` | none | unchanged publishing shape (see §7 for the hazard this creates on non-default submissions) |
| `--cache-to`/`--cache-from` (`:buildcache`) | none | both platforms already share one cache namespace; a single-platform run simply caches fewer legs |
| `docker buildx imagetools inspect` | none (§5.6) | it prints whatever was pushed; only the *expected* output shape differs |
| resources / `activeDeadlineSeconds` / `retryStrategy` | none | amd64-only is a strict subset of the work |
| `dockerd-entrypoint.sh &` + `sleep 5`, clone, secrets, volume mounts | none | platform-independent |

### 3.3 No Dockerfile change

Verified against this repo's `Dockerfile` on 2026-09-04: buildx injects `TARGETARCH`
per platform leg and stage 2 consumes it as `GOOS=linux GOARCH=$TARGETARCH`; every
`RUN` executes on `$BUILDPLATFORM` (native, no QEMU); stage 3 is COPY-only. A
one-platform run is the two-platform run minus a leg — the Dockerfile is already
parameterised on exactly the axis this changes. Its `amd64` defaults
(`TARGETARCH=amd64`, `TARGETPLATFORM=linux/amd64`, `BUILDPLATFORM=linux/amd64`) only
matter for plain `docker build` invocations that pass no `--platform`, which this step
never does.

---

## 4. Other steps that need platform awareness (AC4)

**Answer: none.** Full census of the entrypoint's step groups:

| Step / component | Needs change | Reason |
|---|---|---|
| `resolve-version` | no | Version is resolved once per push and applied identically to firmware and to every image platform. There is deliberately no per-arch version. |
| `golangci-lint`, `a11y-test`, `go-test`, `timing-benchmark`, `acceptance-test`, `firmware-test` | no | All run in `golang`/node containers on the amd64 CI node; they test the source, not the image's target platforms. |
| `firmware-build` | **no — and must not be coupled** | Its `esp32s3` literals are the *hardware* target, an independent axis from the image platform by ADR-001 (firmware is built once and fetched by every image platform). Parameterising `platforms` must not touch `set-target`/`--chip`. Its `gh_${GH_VERSION}_linux_amd64.tar.gz` literal is a **runner**-arch assumption (the CI node pool), not a target-platform knob — it constrains which node the step can run on, not what it produces. |
| `update-declarative-config` | no | Its sed rewrites `ronaldraygun/spaxel:*` pins by literal, with no architecture in the pattern. See §7.2 for the hazard this *does* create on an amd64-only submission. |
| push sensor (`spaxel-push` → event source/sensor in declarative-config) | no | Passes only `git-repo` and `image-repo`; `branch` already falls back to its template default, and `platforms` will behave the same. Verified from the two most recent submitted workflows. |
| repo `Dockerfile` | no | §3.3. |
| `spaxel-e2e` (separate template) | no | Runs `docker compose` on the CI node; it exercises the built image on the node's own arch and is unaffected by what the manifest list contains. |

The only thing that *changes meaningfully* elsewhere is an observation, not a change:
`imagetools inspect`'s output shape (§5.6).

---

## 5. Implementation plan (AC5)

All template edits happen in **`jedarden/declarative-config`**, are applied by ArgoCD,
and must never be `kubectl apply`-ed directly (ArgoCD-managed; `selfHeal` reverts and
mutating verbs are prohibited). Every step below is either a declarative-config
commit or a read-only check, except the one sanctioned workflow submission in step 4.

### Step 1 — edit `k8s/iad-ci/argo-workflows/spaxel-build-workflowtemplate.yml`

1. Add to `spec.arguments.parameters`:

   ```yaml
   - name: platforms
     value: linux/amd64,linux/arm64
   ```

2. In the `docker-build` template's script, replace the platform literal:
   `--platform=linux/amd64,linux/arm64 \` → `--platform={{workflow.parameters.platforms}} \`

### Step 2 — retire the fork: delete `k8s/iad-ci/argo-workflows/spaxel-build-amd64-workflowtemplate.yml`

Safe because it is referenced by nothing (§1.3) and is broken as applied (§1.2).
Deleting it also removes the only trap in §1.4 and resolves the doc-drift note in the
targeting doc §5.4 (root CLAUDE.md's template table already lists `spaxel-build` only,
so deletion makes that table correct rather than stale).

### Step 3 — commit and let ArgoCD sync

Commit both manifest changes to declarative-config, push, and confirm the app
`argo-workflows-ns-iad-ci` syncs. Read-only verification:

```bash
# parameter present, in the live object
kubectl --server=http://traefik-iad-ci:8001 get workflowtemplate spaxel-build \
  -n argo-workflows -o json \
  | jq -r '.spec.arguments.parameters[] | "\(.name)=\(.value)"'
# expect a 4th line: platforms=linux/amd64,linux/arm64

# substitution landed, and the fork is gone
kubectl --server=http://traefik-iad-ci:8001 get workflowtemplate spaxel-build \
  -n argo-workflows -o json \
  | jq -r '.spec.templates[] | select(.name=="docker-build") | .container.args[0]' \
  | grep -- --platform
kubectl --server=http://traefik-iad-ci:8001 get workflowtemplate spaxel-build-amd64 \
  -n argo-workflows -o name   # expect: NotFound
```

**ArgoCD prune check (do this before assuming step 2 took effect):** if the
Application's sync policy has `prune` disabled, a deleted manifest leaves the live
`spaxel-build-amd64` object orphaned. Verify with the `NotFound` check above; if the
object survives a synced app, surface that in declarative-config rather than deleting
the live object by hand.

Syntax validation is implicit in the sync — Argo's own validation rejects a malformed
template at reconcile, and a broken template cannot create workflows. There is no need
for a `kubectl apply --dry-run` (and `apply` is prohibited regardless of `--dry-run`).

### Step 4 — functional verification of the amd64-only path

Lint and a11y currently fail in the first step group (documented red, `spaxel-a20b01d0`
/ the lint-red note), so a full pipeline cannot reach `docker-build` right now and an
amd64-only *end-to-end* proof must wait for a lint-green commit. What can be verified
immediately is the part this design actually changes — that a submission accepts and
resolves the parameter — using the sanctioned manual submission (the one permitted
`kubectl create`, of an Argo Workflow in `argo-workflows`):

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
  workflowTemplateRef:
    name: spaxel-build
EOF
```

then read `kubectl get workflow <name> -o jsonpath='{.spec.arguments.parameters}'` and
confirm `platforms=linux/amd64` survived resolution. **Use the safety recipe below for
this submission** so the run cannot publish anything destructive even if it did reach
`docker-build`:

- `image-repo` → a scratch repository (e.g. `ronaldraygun/spaxel-amd64verify`) instead
  of the production one, so the production `:version`/`:latest` tags are untouched;
- `branch=main` (default), so `resolve-version` lands on the already-published version
  and `update-declarative-config`'s sed becomes a no-op rather than rewriting the
  production pin to an amd64-only manifest.

When a lint-green commit does land, the first sensor-triggered multi-arch run is the
full end-to-end regression test for the default path: it must still produce a
two-platform manifest list, which `imagetools inspect` shows in the step's own logs.

### Step 5 — rollback

Revert the declarative-config commit; ArgoCD restores the literal and (if pruning is
on) the fork manifest. Because the default equals today's literal, there is no cluster
state to unwind: the only side effect any amd64-only run can have had is pushed tags in
whatever repository it was pointed at, and the next normal multi-arch run republishes
both tags as a manifest list. No Dockerfile, sensor, or repo change is involved in
either direction.

---

## 6. Verification checklist (for the implementer bead)

1. `spec.arguments.parameters` has 4 entries; `platforms` default is exactly
   `linux/amd64,linux/arm64` (byte-compare against the pre-change literal).
2. `docker-build`'s script contains `--platform={{workflow.parameters.platforms}}` and
   no remaining `linux/amd64,linux/arm64` literal anywhere in the template.
3. Every other line of `docker-build`'s script is unchanged (diff against the
   pre-change manifest; expect the 1-line diff of §3.1 plus the parameter block).
4. `spaxel-build-amd64` manifest deleted; live object `NotFound` after sync.
5. Rendered command with the default is byte-identical to today's (§3.2) — this is the
   backward-compatibility proof, and it is checkable by inspection of the template, no
   build required.
6. Manual submit with `platforms=linux/amd64` + scratch `image-repo` resolves the
   parameter (and, once lint is green, produces a single-platform manifest under the
   scratch repo).

---

## 7. Risks and operational rules

### 7.1 ADR-001 reconciliation

`docs/plan/plan.md` ADR-001 decides the publishing shape: firmware built once
(ESP32-S3, platform-independent) and a **real multi-arch manifest list** published as
`ronaldraygun/spaxel:$(cat VERSION)` — that list is what serves the plan's Raspberry
Pi/arm64 users. This design **preserves** that: the default value *is* the multi-arch
literal, so the decided default behaviour is untouched, and amd64-only becomes an
opt-in submit-time choice rather than the shipping shape.

If a future change wants amd64-only as the *default*, that is a decision that
supersedes ADR-001 and needs a new ADR plus a plan.md edit — not a parameter-default
flip slipped into a template change. Arm64 support must not be lost by accident here.

### 7.2 Same-tag hazard on non-default submissions

An amd64-only submission is still the **full release path**: it tags
`<image-repo>:<version>` and `:latest`, and `update-declarative-config` rewrites the
production pin (`k8s/ardenone-cluster/spaxel/deployment.yml`,
`nixos/bench/modules/mothership.nix`) to that version with an
arch-agnostic sed — regardless of which `image-repo` was passed, because the sed's
pattern hardcodes `ronaldraygun/spaxel:`. A casually-submitted amd64-only run can
therefore (a) replace a multi-arch manifest with a single-platform one under the
production tags, and (b) pin production to it.

Operational rule: any amd64-only submission that is not a real release must set a
scratch `image-repo` — that is what keeps a single-platform manifest out of the
production *tags* (hazard (a) above), and it is the recipe in step 4.

**Follow-up landed** (`spaxel-fce2f7e4`, declarative-config `0ec96fc9`, 2026-09-04):
`update-declarative-config`'s sed now matches `{{workflow.parameters.image-repo}}`
instead of a hardcoded `ronaldraygun/spaxel:`, so hazard (b) is closed — a
scratch-repo run cannot pin production at all, whatever version resolves. The
`branch=main` clause of the original rule is no longer load-bearing for pin safety;
it survives in the usage guide only so a verification run resolves the
already-published version and stays comparable with a real release.

### 7.3 Residual observations (not defects in this design)

- The fork's manifest, until deleted, remains the only place `amd64-builder` and the
  broken `resolve-version` exist; deletion (step 2) retires both.
- `docs/research/*` records describing the fork stay accurate — they describe it as it
  was, and each carries its verification date. This document supersedes their
  *recommendation* sections with a concrete decision.
- Per-arch image testing does not exist: `spaxel-e2e` exercises the node's native arch
  only. Out of scope for platform restriction, but worth a bead if arm64 correctness
  ever matters in CI.

---

## 8. Direct answers for the implementer (`spaxel-b0d532f0`)

| Its AC | Answer |
|---|---|
| "Implement the chosen approach" | Modify `spaxel-build` in declarative-config; delete `spaxel-build-amd64`. §5. |
| "Add/modify the parameter" | Add `platforms` at `spec.arguments.parameters`, value `linux/amd64,linux/arm64`. §2. |
| "Update docker-build to only target amd64" | **Do not** hard-restrict: substitute `--platform={{workflow.parameters.platforms}}`. Its own note says the parameter is optional with default behaviour — the default keeps multi-arch (ADR-001); amd64-only is `--parameter platforms=linux/amd64` at submit. §3, §7.1. |
| "Ensure all other steps are compatible" | They are; nothing else changes. §4. |
| "Verify YAML syntax" | ArgoCD sync validation + the read-only checks in §5 step 3; no `kubectl apply --dry-run` (prohibited verb, and unnecessary). |
| Its note "if creating new, can simplify by removing multi-arch logic" | Moot — we are not creating new. Do **not** remove any multi-arch logic. |

---

## 9. Implementation record (`spaxel-b0d532f0`, 2026-09-04)

Steps 1–3 and 6 of §5 are done. The change lives in **`jedarden/declarative-config`**
(the manifest is outside this repo; nothing else here changes — this section is the
only spaxel-side artifact, as §Scope states).

| Step | Result |
|---|---|
| 1 — edit `spaxel-build-workflowtemplate.yml` | Done. `platforms` added at `spec.arguments.parameters` with default `linux/amd64,linux/arm64` (byte-identical to the removed literal); `docker-build` now reads `--platform={{workflow.parameters.platforms}}`. Whole diff is the §3.1 line plus the parameter block. declarative-config commit `a979e063`. |
| 2 — delete the fork | Done. `spaxel-build-amd64-workflowtemplate.yml` removed (797 lines). Its deletion rode in declarative-config commit `e30e76f0` rather than `a979e063`: a concurrent worker's `git commit` in the shared checkout swept the already-staged `git rm` into its own janitor-harness commit 29 s before mine. Net tree state is exactly as designed; only the attribution is off. |
| 3 — ArgoCD sync + read-only verification | Done. App `argo-workflows-ns-iad-ci` (rs-manager argocd) reached `operationState=Succeeded, "successfully synced (all tasks run)"` at revision `a979e063`. Live checks on iad-ci: `spaxel-build` exposes 4 parameters incl. `platforms=linux/amd64,linux/arm64`; `docker-build` carries the substituted `--platform`; `spaxel-build-amd64` → `NotFound` (prune is on, no orphan). Residual app `OutOfSync` covers only other, concurrently-in-flight templates — `spaxel-build` itself reports `Synced`. |
| 6 — functional verify of the amd64-only path | Done via the sanctioned manual submission with the §5 safety recipe (scratch `image-repo=ronaldraygun/spaxel-amd64verify`, `branch` default, `platforms=linux/amd64`). Workflow `spaxel-build-amd64-verify-28wp5` → **Succeeded**, stored params show `platforms=linux/amd64` overriding the default, and `resolve-version` resolved `should-build=false`, so every build step skipped — inert by construction, no image, no firmware, no pin rewrite. This also proves Argo instantiates the modified template. |

YAML validity (bead AC5): proven by the sync — Argo validated and applied the manifest,
and the verification workflow instantiated it. No `kubectl apply --dry-run` was used
(prohibited verb; also unnecessary).

Remaining: none. §7.2's follow-up landed as bead `spaxel-fce2f7e4`
(declarative-config `0ec96fc9`, 2026-09-04): `update-declarative-config`'s sed takes
its image ref from `{{workflow.parameters.image-repo}}`. Production path verified
byte-identical under the default parameter (old-literal sed output vs
parameterised-with-default output diff empty on both target files); a scratch-repo
run leaves the declarative-config clone clean, so the step takes its existing "No
image tag changes to commit" branch. Default-behaviour risk in this change is the
ArgoCD propagation lag on the app, not the diff — see the closure notes on the bead.
