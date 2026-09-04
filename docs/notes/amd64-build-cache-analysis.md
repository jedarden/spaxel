# amd64 build cache analysis — spaxel-5afc6c01

**Captured:** 2026-09-04
**Bead:** spaxel-5afc6c01 ("Extract and analyze kaniko cache logs from amd64 build")
**Evidence source:** live `spaxel-build` WorkflowTemplate (iad-ci, read via the
credential-free kubectl endpoint) + `Dockerfile` at HEAD cc589b5d + git history.
**Not** evidence source: build logs — see §1. Every claim below is either
verifiable against the template/Dockerfile in-tree or explicitly marked as
requiring a live log to confirm.

---

## 1. Premise findings — three of the five ACs have no live object to act on

### 1.1 There is no kaniko executor

AC1 asks for "kaniko executor logs from the docker-build step pod". The live
`docker-build` template runs:

```
image: docker:29.7.2-dind
docker buildx build --platform={{workflow.parameters.platforms}} ...
  --cache-to=type=registry,ref={{workflow.parameters.image-repo}}:buildcache,mode=max
  --cache-from=type=registry,ref={{workflow.parameters.image-repo}}:buildcache
  --progress=plain
```

No kaniko executor exists anywhere in the pipeline. Cache arbitration is
BuildKit's, driven by the registry `buildcache` tag, and `--progress=plain` is
already set — so a real run emits per-instruction `CACHED` / `DONE` lines. This
agrees with the Kaniko executor-image pin family closed premise-false
2026-09-04 under spaxel-67dfcf40 (spaxel-build has run docker-dind + buildx
since declarative-config 920a55ae).

### 1.2 No amd64 build run exists to retrieve logs from

`kubectl --server=http://traefik-iad-ci:8001 get workflows -n argo-workflows`
filtered for `spaxel` returns **0 items** as of 2026-09-04. The only amd64-only
submission (`spaxel-build-amd64only-d1592c49-r9dhs`, 2026-09-04T09:47:45Z) is
TTL-reaped, and it never reached `docker-build` anyway: `should-build` evaluated
false and skipped all nine gated steps (recorded on closed
spaxel-7710e8db / spaxel-4b1b1af9). The template sets no `podGC` override, so
the controller default (OnPodCompletion) deletes each pod the moment it exits —
even a run that had reached docker-build would have left no logs behind.

AC2 and AC3 therefore have no input: there is nothing to extract and no
hit/miss list to read out.

### 1.3 FIRMWARE_CACHE_BUST no longer exists

AC4 asks to verify that FIRMWARE_CACHE_BUST changes triggered the expected
invalidations. `git show HEAD:Dockerfile | grep -c FIRMWARE_CACHE_BUST` → **0**.
The mechanism lived in the ESP-IDF `firmware-builder` stage:

| Commit | Date | Effect |
|---|---|---|
| d4560756 | 2026-06-03 | introduced — bust Kaniko cache + force sdkconfig regen (16 MB crash loop) |
| 1d3378fb | 2026-08-02 | repaired — ARG was declared but never referenced by the RUNs |
| c217cf3a | 2026-08-02 | repaired — ARG substitution replaced with literal text |
| 64f20297 | 2026-08-02 | last touch — default TARGETPLATFORM/BUILDPLATFORM/TARGETARCH |
| b25dc130 | 2026-08-15 | **removed the stage outright** (ADR-001 firmware/image decoupling) |

b25dc130 replaced `FROM espressif/idf:v5.2 AS firmware-builder` with
`FROM alpine:3.20 AS firmware-fetcher`, moving compilation to the separate
`firmware-build` Argo step. So the cache-bust premise fails twice: the mechanism
is gone, and the layer it was busting is no longer part of the image build.

---

## 2. Cache analysis of the amd64 build as it exists today

Substituting static analysis for the missing logs. This is derived from the
build definition and is deterministic given BuildKit's cache-key rules; the one
item that only a live log can confirm empirically is flagged.

### 2.1 The cache backend

Registry cache only. Every run creates a throwaway `docker-container`-driver
builder (`docker buildx create --use --name multiarch-builder`), so no local
builder state survives between runs — cross-run reuse is entirely
`--cache-from/--cache-to type=registry ... :buildcache,mode=max`. `mode=max`
matters: intermediate layers of every stage are exported, not just the final
image, which is what makes the per-stage hits below possible at all. Each run
overwrites the single `:buildcache` tag, so Docker Hub garbage-collects older
cache manifests outside the newest chain.

### 2.2 Expected hit/miss across two consecutive amd64 *release* builds (different VERSION)

| Stage | Instruction | Cache key contributors | Expectation |
|---|---|---|---|
| fetcher | `FROM alpine:3.20` | image digest | **HIT** |
| fetcher | `ARG VERSION` | build-arg value | **MISS** (new version each release) |
| fetcher | `RUN apk add --no-cache curl` | parent chain incl. ARG value | **MISS** — Docker's documented ARG rule: an ARG value change is a cache miss for the RUN instructions that follow it, whether or not they reference it |
| fetcher | `WORKDIR /firmware` | parent | MISS (inherited) |
| fetcher | `RUN curl .../v${VERSION}/...` | parent + VERSION | **MISS** — new firmware artifact per release, by design |
| builder | `FROM golang:1.25-bookworm` | digest | **HIT** |
| builder | `WORKDIR /app` | parent | HIT |
| builder | `COPY mothership/go.mod go.sum` | file content | **HIT** while dependencies are unchanged |
| builder | `RUN go mod download` | parent | **HIT** — this is the layer the "copy module files first" ordering exists to protect, and it is *not* downstream of the VERSION ARG (that declaration is at line 53, after this RUN) |
| builder | `COPY mothership/` | source tree content | MISS on any Go change |
| builder | `COPY dashboard/ → cmd/mothership/dashboard/` | dashboard content | MISS on any dashboard change |
| builder | `ARG VERSION` / `ARG TARGETPLATFORM` / `ARG TARGETARCH` | values | VERSION **MISS**; the platform args HIT (stable `linux/amd64` across amd64-only runs) |
| builder | `RUN go build … -X main.version=${VERSION}` | parent (already missed) | **MISS** |
| builder | `RUN go build spaxel-sim` | parent | **MISS** |
| runtime | `FROM distroless/static-debian12:nonroot` | digest | **HIT** |
| runtime | `COPY --from=builder` (×2) + `COPY --from=firmware-fetcher` (×2) | producing stage's result content | **MISS** whenever any upstream stage missed |

Net: on a normal release-driven amd64 build the only reuse is the three `FROM`
layers plus, while `go.mod`/`go.sum` are untouched, the module download. The
`VERSION` build-arg is declared *before* the first cacheable RUN in the fetcher
stage, so a version bump busts the fetcher stage wholesale and everything
downstream of it. Two consequences worth recording:

1. The image build is effectively version-bust end to end — this is inherent to
   a Dockerfile that bakes a versioned firmware artifact and a
   `-X main.version` binary, and is not a defect to fix with a cache-bust.
2. The `COPY --from=firmware-fetcher` destinations *include* `${VERSION}` in
   the filename (`/firmware/spaxel-firmware-${VERSION}.bin`), so the runtime
   stage misses even when the incoming firmware bytes are identical to the
   previous release's — the filename, not the content, is what changes.

### 2.3 The ESP-IDF compile has no cache at all

The `firmware-build` step (`espressif/idf:v5.2`) clones fresh (`--depth 1`),
installs `idf-component-manager>=2,<3`, runs `idf.py set-target esp32s3` and
`idf.py build` on every invocation. There is no ccache, no build-directory
persist, and no registry cache — so the stale-cached-firmware problem that
FIRMWARE_CACHE_BUST existed to solve cannot recur here, at the cost of paying
the full multi-minute ESP-IDF compile on every release. If that cost ever
matters, the lever is a ccache/sccache volume or a persistent build dir on this
step — not resurrecting a Dockerfile cache-bust, which has nothing left to bust.

---

## 3. How to capture real logs when a run exists

Preconditions, in order:

1. **spaxel-fce2f7e4 must land first.** It parameterises
   `update-declarative-config`'s image ref. Until then a should-build=true run
   pointed at a scratch `image-repo` rewrites the production image pin via a
   sed hardcoded to `ronaldraygun/spaxel:` — a real run is not safely
   submittable today. Do not force one.
2. `should-build` is an output of `resolve-version` computed from
   `git diff --name-only HEAD~1 HEAD` (anything outside `docs/`, `.beads/`,
   `*.md`, `LICENSE`, `.gitignore` is substantive). It is not a submission
   parameter and cannot be forced without triggering the full release chain.
3. Override `podGC` to `OnWorkflowCompletion` on the submitted Workflow (copy
   the templateRef, keep `serviceAccountName`) so the docker-build pod
   outlives its own completion.

Then, while running or immediately after:

```bash
kubectl --server=http://traefik-iad-ci:8001 logs -n argo-workflows <pod> -c main
# cache lines only:
kubectl ... logs -n argo-workflows <pod> -c main \
  | grep -E 'CACHED|DONE|exporting|pushing|ERROR|failed to solve'
```

BuildKit plain progress marks a reused instruction as `CACHED` on the line
following the `#N [stage step] instruction` header; an executed one ends in
`DONE <dur>`. The §2.2 table is the expected result against which to diff —
any layer listed HIT there that comes back MISS is the finding.

---

## 4. AC disposition

| AC | Status | Reason |
|---|---|---|
| 1. Get kaniko executor logs | premise-false | no kaniko — docker:29.7.2-dind + buildx; and 0 spaxel workflows exist in iad-ci to read |
| 2. Extract cache messages | not achievable | no pod logs survive podGC OnPodCompletion; the only amd64 run never reached docker-build |
| 3. Identify cached vs missed layers | answered statically | §2.2 — derived from the live Dockerfile + registry-cache config, not from logs |
| 4. Verify FIRMWARE_CACHE_BUST invalidations | premise-false | mechanism removed at b25dc130 with the stage it busted (§1.3) |
| 5. Save raw logs + analysis summary | partially | raw logs do not exist and cannot be retro-fetched; this document is the analysis summary, plus the §3 capture recipe for the first real run |

Raw-log capture remains blocked on spaxel-fce2f7e4. This bead makes no source
change and submits no workflow.
