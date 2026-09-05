# Kaniko cache-busting: history, behavior, and the single-arch verdict

**Bead:** spaxel-bd6f48c8 ("Document cache-busting behavior and findings")
**Date:** 2026-09-04
**Scope:** the `FIRMWARE_CACHE_BUST` mechanism (2026-06-03 → 2026-08-02), the
executor that made it necessary, what the image build caches today, and what
is still open for cross-arch testing.

Every claim below is attributed to one of three evidence classes, because the
two things a reader would most want — live kaniko logs and live BuildKit
layer logs — do not exist:

| Class | Meaning |
|---|---|
| **verified** | Re-run by this bead against the tree at HEAD `af9fc579` or the live cluster |
| **recorded** | Surviving verbatim quote from the era, in a commit message, a removed Dockerfile comment, or `notes/bf-38dbu.md` |
| **absent** | Confirmed not to exist (pods deleted, objects TTL-reaped, registry private) |

---

## 1. Conclusion up front (AC5)

**Cache-busting is not working incorrectly for single-arch — there is no
cache-busting mechanism left in the build for it to be working at all.** The
question as posed is moot on three levels, each independently verified:

1. **No kaniko.** The live `spaxel-build` WorkflowTemplate in `iad-ci` runs
   `docker:29.7.2-dind` + buildx with a registry buildcache
   (`--cache-to/--cache-from=type=registry,ref=<image-repo>:buildcache,mode=max`)
   and `--progress=plain`. Zero kaniko references. Kaniko has not been the
   executor since declarative-config `920a55ae` (ADR-001 multi-arch rework);
   verified live 2026-09-04.
2. **No cache-bust mechanism.** `git show HEAD:Dockerfile | grep -c
   FIRMWARE_CACHE_BUST` → `0`. The mechanism was removed at `64f20297`
   (2026-08-02) — see §3 — and the stage it busted was removed at `b25dc130`
   (2026-08-15), which moved ESP-IDF compilation out of the image build
   entirely.
3. **Nothing left to bust.** Even in the kaniko era, the failures the
   cache-bust commits were chasing turned out not to be cache-mediated at all
   — see §4. The mechanism was solving a problem that had already gone away.

The one real single-arch observation that exists (workflow
`spaxel-build-84smx`, 2026-09-04) reached docker-build and died in **17
seconds** on the version-keyed firmware fetch — a step-ordering race, not a
cache behavior. No caching configuration, working or broken, could have
changed that outcome: the fetcher's `curl -fsSL` is deliberately version-busted,
so it executes on every release build and fails hard when the artifact it
asks for has not been published yet (§5).

---

## 2. What the mechanism was

`FIRMWARE_CACHE_BUST` was a build-arg + literal-text cache-key breaker living
in the Dockerfile's ESP-IDF `firmware-builder` stage. Its original purpose
(d4560756, 2026-06-03) was narrow and real: release 0.1.352 shipped an image
containing firmware compiled with `CONFIG_ESPTOOLPY_FLASHSIZE=16MB` after
`sdkconfig.defaults` had already been changed to 4MB, because the executor
served a cached firmware layer that bypassed the config change. An ESP32-S3
with 4MB flash then failed on every boot:

```
spi_flash: Detected size(4096k) smaller than binary image header(16384k). Probe failed.
```

(recorded — d4560756 commit message)

That June failure *was* genuinely cache-mediated, and a cache-bust was the
correct category of fix for it. The mechanism's history:

| Commit | Date | Effect |
|---|---|---|
| d4560756 | 2026-06-03 | introduced — `ARG FIRMWARE_CACHE_BUST` + `RUN rm -f sdkconfig sdkconfig.old`, "bust Kaniko cache + force sdkconfig regen to fix 16MB crash loop" |
| 1d3378fb | 2026-08-02 | repaired — the ARG was declared but never referenced by any `RUN`, so it keyed nothing |
| c217cf3a | 2026-08-02 | repaired — ARG *substitution* in the RUN bodies replaced with literal cache-bust text |
| 64f20297 | 2026-08-02 | **mechanism removed** — root cause found elsewhere; also defaulted `TARGETPLATFORM`/`BUILDPLATFORM`/`TARGETARCH` |
| b25dc130 | 2026-08-15 | `FROM espressif/idf:v5.2 AS firmware-builder` replaced by `FROM alpine:3.20 AS firmware-fetcher` — ESP-IDF compilation left the image build for the separate `firmware-build` Argo step (ADR-001) |

**Correction to an earlier deliverable.**
`docs/notes/amd64-build-cache-analysis.md` §1.3 attributes the mechanism's
removal to `b25dc130`. That is wrong, and the error is reproduced from this
bead's own verification:

```
git show b25dc130^:Dockerfile | grep -c FIRMWARE_CACHE_BUST   → 0  (already gone)
git show 64f20297^:Dockerfile | grep -c FIRMWARE_CACHE_BUST   → 1  (still present)
git show 64f20297:Dockerfile   | grep -c FIRMWARE_CACHE_BUST  → 0  (removed)
```

`64f20297` removed the mechanism (2026-08-02); `b25dc130` removed the *stage*
(2026-08-15). The analysis doc's §2.2 hit/miss table and §3 capture recipe are
unaffected — only that one attribution line is.

---

## 3. Why cache behavior occurred — the mechanics (AC3)

Two different cache-key rules are in play across the two eras, and
misreading which one is running is what produced three consecutive
"fix the cache key" commits in a single day.

### 3.1 Kaniko era: the cache key is literal instruction text, before substitution

The removed Dockerfile (recovered verbatim from `git show 64f20297^:Dockerfile`)
carried the canonical explanation, written after three empirical debug runs:

> an ARG reference does NOT work here, even when the RUN body echoes it.
> Verified empirically across three separate CI debug runs (bf-38dbu,
> 2026-08-02): kaniko logs "Using caching version of cmd: RUN echo
> \"cache-bust: $FIRMWARE_CACHE_BUST\" && ..." with the variable name still
> UNexpanded, and reused the exact same cache digest after the ARG's default
> value was bumped. Its cache key is computed from the Dockerfile
> instruction's literal source text, before shell/ARG substitution.
> The only thing that reliably busts the key is changing the RUN line's
> actual characters.

So the answer to "were the cache-bust prefixes working as intended?" is
**yes, mechanically — but only the literal-text ones.** The surviving
implementation at `64f20297^` used exactly that shape, on three RUN commands:

```dockerfile
RUN echo "cache-bust-v2-2026-08-02" && apk add --no-cache ...
RUN echo "cache-bust-v2-2026-08-02" && python3 -c 'import re ...' ...
RUN echo "cache-bust-v2-2026-08-02" && /opt/esp/idf/install.sh ...
```

Changing `cache-bust-v1…` → `cache-bust-v2…` changed the literal source text
of each RUN, which changed the key, which forced execution. `ARG`-value
bumping did nothing (§4's verbatim evidence). This is why commit c217cf3a
exists: it converted a *substituted* reference into *literal* text, which is
the only form kaniko's keying can see.

### 3.2 BuildKit era (today): ARG values DO bust, and position matters

BuildKit's documented rule is different: an `ARG` value change is a cache
miss for every instruction that follows its declaration, **whether or not the
instruction references it**. That single difference is why the old
literal-marker hack would now be both unnecessary and wrong. The current
`Dockerfile` (HEAD `af9fc579`) is built around this rule:

- `ARG VERSION=dev` sits at **line 11**, before the fetcher stage's first
  `RUN` (line 14). A version bump therefore busts the whole fetcher stage
  wholesale — which is *by design*, since the stage's output is
  `spaxel-firmware-${VERSION}.bin` fetched from that version's Release.
- `RUN go mod download` sits at **lines 38–39**, *before* `ARG VERSION` at
  line 53. Module download therefore survives a version bump and hits while
  `go.mod`/`go.sum` are unchanged — the classic "copy module files first"
  ordering, and the one layer of genuine reuse a release build gets.
- `TARGETARCH`/`TARGETPLATFORM` are referenced *inside* the Go build commands
  (`GOARCH=$TARGETARCH`, `-ldflags="... -X main.version=${VERSION}"`), so
  both the ARG value and the command text participate in the key. No
  literal markers are needed, and none should be added.

### 3.3 The `rm -f sdkconfig` half

`d4560756` also added `RUN rm -f sdkconfig sdkconfig.old` so `idf.py
set-target` regenerated from `sdkconfig.defaults` instead of honoring a
stale generated `sdkconfig`. That half was about *file state inside the
layer*, not the cache key, and it is now structurally irrelevant: the
`firmware-build` Argo step clones fresh (`--depth 1`) into an empty
container on every run and pays the full ESP-IDF compile each time — no
cache exists on that leg to serve a stale config. The cost is a multi-minute
compile per release; the benefit is that the June 2026 failure mode cannot
recur.

---

## 4. The kaniko-era evidence that survives (AC4)

No executor log for any historical build exists — `podGC: OnPodCompletion`
deletes every pod the instant its step exits, and this was checked directly
against the one recent run: `kubectl --server=http://traefik-iad-ci:8001 get
pods -n argo-workflows -l workflows.argoproj.io/workflow=spaxel-build-84smx`
→ "No resources found". What survives is what was quoted into durable
records at the time. Those quotes are the AC4 evidence, and they are the
primary source for this whole analysis:

**1. Kaniko reusing a layer with the variable name still unexpanded**
(removed Dockerfile comment at `64f20297^`, quoting the bf-38dbu debug runs):

```
Using caching version of cmd: RUN echo "cache-bust: $FIRMWARE_CACHE_BUST" && ...
```

The literal `$FIRMWARE_CACHE_BUST` in the log is the proof: substitution
never happened, so the key never changed, so the digest was reused after the
ARG's default was bumped.

**2. The same cache digest reused across an ARG bump** (same comment):
kaniko "reused the exact same cache digest after the ARG's default value was
bumped". Direct evidence that ARG-value busting was a dead end in this
executor.

**3. The 16MB firmware served from cache** (d4560756 commit message): release
0.1.352's image contained `CONFIG_ESPTOOLPY_FLASHSIZE=16MB` firmware after
`sdkconfig.defaults` said 4MB — the layer predating the config change was
served intact, and the device logged the flash-size mismatch at boot. This is
the one confirmed instance of cache actually delivering a stale artifact.

**4. The red-herring verdict** (64f20297 commit message, verbatim):

> Root cause found, not another cache-key guess: the CI kaniko invocation
> ... never passes --customPlatform or --build-arg ... 100% deterministically,
> regardless of any layer caching. The three prior 'fix the kaniko cache key'
> commits (0e1343c, 1d3378f, c217cf3) were chasing a red herring...

Concretely: kaniko does not auto-populate `TARGETPLATFORM`/`BUILDPLATFORM`/
`TARGETARCH`, and the CI invocation never passed them as build args. The
Dockerfile compared `"$TARGETPLATFORM" != "linux/amd64"`, which with an empty
value is always true, so placeholder firmware was written unconditionally and
`idf.py set-target` failed **100% deterministically regardless of any layer
caching**. Caching looked guilty because a rebuild "fixed" it — the rebuild
was carrying the real fix each time, not the cache miss.

**Evidence limitation, stated plainly (absent):** per-layer hit/miss lines
for the kaniko era survive only where quoted above. No complete build log
from that era exists anywhere in this repo, in the cluster, or in the
retained workflow history, and the executor itself is gone. This document
deliberately does not reconstruct a log it never saw.

---

## 5. Which layers hit and which miss today (AC2)

Static analysis, substituted for the logs that do not exist. It is
deterministic given BuildKit's rules (§3.2); the one thing only a live run
could add is confirmation, and no per-layer `CACHED`/`DONE` output survives
any run (podGC verified above; the 84smx docker-build pod lived 17s and died
before producing layer output).

Two consecutive single-arch **release** builds, different `VERSION`,
`go.mod`/`go.sum` unchanged:

| Stage | Instruction (HEAD line) | Expectation | Why |
|---|---|---|---|
| fetcher | `FROM alpine:3.20` (10) | **HIT** | digest unchanged |
| fetcher | `ARG VERSION=dev` (11) | **MISS** | build-arg value differs per release |
| fetcher | `RUN apk add --no-cache curl` (14) | **MISS** | downstream of the ARG; referenced or not, the value change busts it |
| fetcher | `RUN curl …/v${VERSION}/…` (19–26) | **MISS** | by design — new firmware artifact per release |
| builder | `FROM --platform=… golang:1.25-bookworm` (33) | **HIT** | digest unchanged |
| builder | `COPY mothership/go.mod go.sum` (38) | **HIT** | file content unchanged |
| builder | `RUN go mod download` (39) | **HIT** | sits *before* `ARG VERSION` (line 53) — the one real reuse |
| builder | `COPY mothership/` (42) | MISS on any Go change | source content |
| builder | `COPY dashboard/` (46) | MISS on any dashboard change | source content |
| builder | `ARG VERSION/TARGETPLATFORM/TARGETARCH` (53–55) | VERSION **MISS**; platform args HIT | `linux/amd64` is stable across single-arch runs |
| builder | `RUN go build … ${VERSION}` (56–61) | **MISS** | parent already missed |
| builder | `RUN go build spaxel-sim` (65–69) | **MISS** | parent already missed |
| runtime | `FROM distroless/static-debian12:nonroot` (73) | **HIT** | digest unchanged |
| runtime | `COPY --from=…` ×4 (78, 82, 86, 91) | **MISS** | upstream stages missed |

Net: **a release build is version-bust end to end.** The only reuse is the
three `FROM` layers plus the module download while dependencies hold. Two
consequences, both inherent and neither a defect:

1. There is no room for a cache-bust mechanism to be "working" — the `ARG
   VERSION` declaration placement already forces the only bust that matters,
   and it forces it every release. A version-keyed build cannot be cached
   across versions, and should not be.
2. The runtime stage misses even when the incoming firmware bytes are
   identical to the previous release's, because the `COPY` destinations
   embed `${VERSION}` in the *filename*
   (`/firmware/spaxel-firmware-${VERSION}.bin`, lines 86 and 91). The
   filename, not the content, is the cache input there.

Cache-state caveat for any future run: the buildcache is a registry cache
(`mode=max`), each run overwrites the single `:buildcache` tag, and the
`ronaldraygun/spaxel` repo is **private** — the tag is not anonymously
inspectable, so cross-run reuse cannot be verified from the outside either
(checked 2026-09-04; no credentials may be pulled into context to do so).
Only a run's own `--progress=plain` output can show it.

### The observed single-arch run, and why cache is irrelevant to it

Workflow `spaxel-build-84smx` (2026-09-04, version 0.2.160) is the only
recent real single-arch run to reach docker-build. Its `build` pod lived
**17 seconds**, exit 1. Full diagnosis in
`docs/notes/amd64-build-verification-2026-09-04.md`; the cache-relevant
summary:

- The template runs `build-firmware` and `build` in the **same parallel step
  group** (both pods started 13:32:36Z).
- `build-firmware` published Release `v0.2.160` at 13:35:27Z — **2m34s
  after** docker-build had already exited.
- docker-build's fetcher stage `curl -fsSL`s a version-keyed Release URL;
  `-fsSL` fails hard on the 404.

The cache link: the fetcher's `RUN curl` is downstream of `ARG VERSION`
(§5 table), so it **always executes on a release build** — a cache hit is
structurally impossible for that layer, and a cache miss is not a variable.
Whether the release existed when the curl fired is a step-ordering property,
not a caching one. No caching configuration — BuildKit, kaniko, or a
resurrected literal marker — changes this outcome. The fix is a
`depends`/step-group reorder in the `spaxel-build` WorkflowTemplate, which
lives in `jedarden/declarative-config` and is out of this repo's scope.

---

## 6. Open questions and next steps (AC6)

The family's remaining open beads, and what each actually has left to do:

| Bead | Question it owns | Status of the question |
|---|---|---|
| `spaxel-82232358` | Build/test single-arch to verify cache-busting in isolation | **premise collapsed** — no mechanism exists to verify (§1); its real residual is the §5 table confirmed by a live run |
| `spaxel-21fea911` | Cross-architecture cache poisoning (arm64 + amd64) | **structurally excluded today** — see below |
| `spaxel-4fc10b08` | Architectural fix if cache-busting proves insufficient | **no fix is indicated** — the mechanism was already proven unnecessary and removed; the one architectural defect actually observed is the docker-build ordering race, owned in declarative-config |
| `spaxel-dc8d6c66` | Document final caching solution; update template if needed | this document is the caching answer; template changes belong to declarative-config |

**Cross-arch poisoning cannot recur, for three structural reasons.** The
scenario the family feared — an amd64-built layer cached and served to an
arm64 build — required (a) a single flat cache keyed on instruction text
only, and (b) a `RUN` whose output legitimately differs by architecture
while its source text is identical. Neither exists anymore:

1. BuildKit's registry cache is platform-aware: entries are keyed under the
   cache manifest's platform in addition to the instruction inputs.
2. Even for textually identical `RUN`s, the enclosing stage's base image is
   `FROM --platform=$BUILDPLATFORM golang:1.25-bookworm` — the base digest
   differs per platform, so the parent chain differs, so the entry differs.
3. No conditional-on-platform `RUN` remains in the Dockerfile. The only
   platform-varying inputs are `GOARCH=$TARGETARCH` in the command text
   itself and the `TARGETARCH` ARG value — both of which are *supposed* to
   split the cache key, and do.

What cross-arch testing still genuinely needs to establish, in dependency
order:

1. **Fix the docker-build ordering race first** (declarative-config: make
   `build` depend on `build-firmware`). Until then no multi-arch run survives
   long enough to produce a cache observation at all.
2. **Confirm the §5 table empirically** for both legs from a real run's
   `--progress=plain` output (`grep -E 'CACHED|DONE|exporting|pushing|ERROR|
   failed to solve'`). Preconditions per
   `docs/notes/amd64-build-cache-analysis.md` §3: the
   `update-declarative-config` sed parameterisation is done (`spaxel-fce2f7e4`,
   declarative-config `0ec96fc9`, live on the cluster 2026-09-04); still needed
   is a `podGC` override so the docker-build pod outlives its own completion.
3. **Verify the buildcache's multi-platform shape** — whether the single
   `:buildcache` tag holds a per-platform manifest list with entries for both
   legs, or whether each run overwrites the other leg's entries. Unverifiable
   externally while the repo is private; answerable only from a run's own
   push output.
4. **Runtime proof, not just cache proof** — that an arm64 image actually
   boots and serves. This is the residue of the old poisoning worry that
   *no* cache scheme addresses: correctness of the cross-compiled artifact
   (`GOARCH=$TARGETARCH` handles the compile; nothing has yet observed the
   binary run). Also confirm the firmware-fetcher's arch-independent
   assumption — the same ESP32-S3 binaries are copied into every platform's
   image, which is correct for an ESP32 product and worth stating in the
   final doc.
5. **Firmware leg caching is a separate decision** — the `firmware-build`
   step has no cache at all (fresh clone, full ESP-IDF compile per release).
   If its cost ever matters, the lever is ccache/sccache on that step, never
   a Dockerfile cache-bust; there is nothing left in the image build to bust.

---

## 7. Answer summary

| AC | Answer |
|---|---|
| 1. Write findings doc | this file, `docs/kaniko-cache-busting-analysis.md` |
| 2. Which layers hit / miss | §5 — statically derived: on a release build only the three `FROM` layers and `go mod download` hit; everything else is version-bust by design |
| 3. Why the behavior occurred | §3 — kaniko keyed on literal instruction text pre-substitution (so ARG busts were invisible and literal markers did work); BuildKit keys on post-ARG values (so the `ARG VERSION` placement now does the busting, and `go mod download` survives it) |
| 4. Evidence from logs | §4 — the verbatim surviving kaniko quotes (unexpanded `$FIRMWARE_CACHE_BUST` reuse, same-digest-after-ARG-bump, 16MB stale layer, red-herring verdict); no live log exists for any run, confirmed directly against the cluster |
| 5. Is cache-busting working correctly for single-arch? | **moot** — the mechanism was removed at `64f20297` (2026-08-02) after the underlying failure was shown to be caching-independent (empty platform ARGs under kaniko), and its target stage left the image build at `b25dc130` (2026-08-15). Single-arch caching today is version-bust by design and correct; the one observed docker-build failure was a step-ordering race, not a cache behavior |
| 6. Open questions / cross-arch next steps | §6 — poisoning structurally excluded; remaining work is the ordering fix, one empirical log diff, the buildcache's multi-platform shape, and runtime proof of the arm64 artifact |
