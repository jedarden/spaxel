# Build Configuration & Deployment File Inventory

**Bead:** spaxel-7fa15b01 — "Identify build configuration and deployment files"
**Date:** 2026-09-04
**Verified at:** HEAD `ba620c83` via `git ls-tree -r HEAD` (tracked state only).
The working tree carries unrelated in-flight edits; none of the findings below
come from uncommitted files.
**Re-verified at:** HEAD `bedfc4a3` (2026-09-05). Two things moved under this
doc between draft and landing — the Go module consolidation (`d1f6ecba`) and
the root/dashboard residue cleanups — and the affected rows are corrected in
place (§5.1, §5.2, §6, errata §8.11–8.12).
**Feeds:** the `spaxel-sensor` push path filter (ADR-009 decision 3, already
landed — see `docs/notes/ci-doc-only-push-path-filter.md`).

Companion docs this inventory builds on and corrects:

| Doc | Role |
|---|---|
| `docs/build-paths-catalog.md` (2026-08-27) | prior component-by-component catalog; see errata §8 |
| `docs/BUILD_PATHS.md` (2026-08-27) | prior path-list variant; see errata §8 |
| `docs/research/docker-build-step-configuration.md` | the live `spaxel-build` template's docker-build step |
| `docs/research/spaxel-build-architecture-targeting.md` / `-parameters.md` | the live CI DAG and its parameters |
| `docs/notes/ci-doc-only-push-path-filter.md` | the landed sensor-side ignore list |
| `docs/research/go-backend-code-directories.md` | mothership package survey (code, not build config) |
| `docs/research/esp32-firmware-code-directories.md` | firmware source survey (code, not build config) |

---

## 0. Answer summary

| Acceptance criterion | Result |
|---|---|
| Docker-related files | **3, all present:** `Dockerfile`, `docker-compose.yml`, `.dockerignore` (§1) |
| Rust/Cargo configuration | **None. Zero Rust in this repo** — no `Cargo.toml`, no `Cargo.lock`, no `*.rs` at HEAD (§2) |
| CI/CD configuration affecting builds | **None in-repo by policy** (no `.github/`, Argo templates live in `declarative-config`); 1 committed debug Workflow YAML + 1 lint gate config in-repo (§3) |
| Deployment manifests / Kubernetes specs | **None in-repo.** Production manifests live in `jedarden/declarative-config`; `docker-compose.yml` is the local-manifest surface (§4) |
| Other build-affecting configuration | Versioning, Go workspace/module, firmware build config, dashboard embed — tiered in §5; non-functional paths that currently *do* trigger builds in §6 |

The distinction that matters for a path filter is not "source vs. not" but
**which of three tiers a path falls into**:

- **Tier A — changes shipped image content.** Must trigger a build + deploy.
- **Tier B — gates CI but changes no image content.** Should not be silently
  ignored: these files can fail a release, so a filter must not drop them from
  *gating* concerns, but they also cannot be skipped outright without deciding
  whether a lint-only or test-only change warrants a deploy.
- **Tier C — deployment-shaped, no rebuild.** Restart/redeploy only.

---

## 1. AC1 — Docker-related files

| File | Tier | Effect |
|---|---|---|
| `Dockerfile` | A | Defines the whole image. Any change can change image content → rebuild + redeploy |
| `.dockerignore` | A | Filters the build *context*. Adding/removing an ignore rule can change what `COPY` picks up, so it is image-content-affecting, not cosmetic |
| `docker-compose.yml` | A + C | Carries a `build:` block (`context: .`, `dockerfile: Dockerfile`) — so it is **not** deploy-only; editing it can change how a local image is built. It also encodes the local deployment surface (volumes, env, healthcheck, Traefik labels) |

### 1.1 What the Dockerfile actually consumes

Three stages (verified at HEAD; the `plan.md` prose describing an in-image
ESP-IDF `firmware-builder` stage is **stale** — see §8):

1. `firmware-fetcher` (`alpine:3.20`) — `curl`s two prebuilt firmware artifacts
   from GitHub Releases for the resolved `VERSION`:
   `spaxel-firmware-${VERSION}.bin` (app image, OTA store) and
   `spaxel-firmware-${VERSION}-merged.bin` (offset-0 full-flash image for
   serial first-flash).
2. `builder` (`golang:1.25-bookworm`, pinned `--platform=$BUILDPLATFORM`) —
   `COPY mothership/go.mod mothership/go.sum` → `go mod download` →
   `COPY mothership/` → `COPY dashboard/ ./cmd/mothership/dashboard/` →
   `CGO_ENABLED=0 GOARCH=$TARGETARCH go build -tags=embed ./cmd/mothership`
   and the same for `./cmd/sim` (both binaries land in the image).
3. runtime (`gcr.io/distroless/static-debian12:nonroot`) — copies `/spaxel`,
   `/spaxel-sim`, and the two firmware artifacts into `/firmware/…` and
   `/firmware/serial/…`.

Consequences for trigger classification:

- Firmware source affects the **image** only transitively: CI's
  `firmware-build` step publishes the release artifacts, and stage 1 fetches
  them by version. A firmware-only change therefore still requires a full image
  build (the filename is version-bearing and is copied in), but the compile
  happens in CI, not in the Docker build.
- `.dockerignore` does **not** exclude most repo residue (`.beads/`, the root
  `*.txt` scratch files, `data/`, `dashboard/test-results/`, …) — those bytes
  are uploaded as build context on every build even though only
  `mothership/` and `dashboard/` are `COPY`ed. It excludes `docs/`, `*.md`
  (with a `!README.md` re-include), `.marathon/`, `firmware/{build,managed_components,.cache}`,
  IDE/OS files, and `tmp/`/`temp/`/`*.tmp`.

### 1.2 The embed surface is wider than "dashboard source"

`mothership/cmd/mothership/dashboard_embed.go:11` carries an unqualified
`//go:embed dashboard`, so **every tracked file under `dashboard/` is embedded
into the binary** — application JS/CSS/HTML *and* `jest.config.js`,
`playwright.config.js`, `package.json`, `dashboard/tests/**`,
`dashboard/leak-detection-report.json`, `dashboard/test-results/.last-run.json`.
Any change under `dashboard/` is image-content-affecting in the strict sense.
(The *right* fix is a narrower embed pattern or a `.dockerignore`/build-time
prune; see §7 recommendations.)

---

## 2. AC2 — Rust/Cargo configuration

**Premise-false: there is no Rust in this repository.**

Verified at HEAD `ba620c83`:

```
git ls-tree -r HEAD --name-only | grep -iE 'cargo|\.rs$'   →  (no hits)
```

The only case-insensitive "rust" matches are three *markdown* inventory
documents (`RUST_SOURCE_INVENTORY.md`, `rust-source-inventory.md`,
`rust-test-modules-report.md`) recording earlier investigations that reached
the same conclusion. The Go modules consume no Rust toolchain, the Dockerfile
installs no Rust toolchain, and CI has no Rust step.

**Filter implication:** no `Cargo.toml`/`Cargo.lock`/`*.rs` pattern is needed
in any trigger or ignore list, and a future dispatch asking for a "Rust build"
in spaxel is premise-false on its face. If Rust ever appears, it will need a
new Docker stage and a new entry here.

---

## 3. AC3 — CI/CD configuration files that affect builds

**No CI definition lives in this repository, by policy.** GitHub Actions are
disabled org-wide and must never be created (`.github/` does not exist at
HEAD — verified). All CI runs on Argo Workflows in `iad-ci`, and the
`spaxel-build` WorkflowTemplate + the `spaxel-sensor` that fires it live in
`jedarden/declarative-config` (`k8s/iad-ci/argo-workflows/`,
`k8s/iad-ci/argo-events/spaxel-sensor.yml`) — a different repo, outside this
one.

What *is* in-repo:

| File | Tier | Role |
|---|---|---|
| `.golangci.yml` | B | The only lint config (no nested copy). `golangci-lint config path` run from `mothership/` resolves to `../.golangci.yml`, so this root file governs the CI `golangci-lint` gate even though that step executes from `mothership/`. A lint failure fails the workflow before `firmware-build`/`docker-build` run |
| `acceptance-test-hang-workflow.yml` | none (should be ignored) | A committed **debug** Argo `kind: Workflow` (not a template) used to reproduce an acceptance-test hang with `podGC: OnWorkflowCompletion`. It is a run manifest, not CI configuration; it triggers nothing. It is also exactly the kind of top-level YAML that the sensor filter's own residual-risk note warns about — a new top-level non-functional file triggers builds until added to `ignored_path` |

The live pipeline's steps, for classification purposes
(`docs/notes/amd64-only-build-usage.md` step table + `docs/research/spaxel-build-architecture-targeting.md`):

```
resolve-version → golangci-lint → a11y-test → go-test → timing-benchmark
              → acceptance-test → firmware-test → firmware-build
              → docker-build → update-declarative-config
```

- `firmware-build` compiles the ESP32-S3 image once on amd64
  (`espressif/idf:v5.2`, hardcoded `esp32s3` target) and uploads
  `spaxel-firmware-<version>[-merged].bin` to GitHub Releases. It is
  architecture-independent of the image build (ADR-001).
- `docker-build` builds the image and consumes those release artifacts.
- `update-declarative-config` rewrites the production image pin in
  `declarative-config` — that push is what actually rolls production, via ArgoCD.

### 3.1 The already-landed trigger filter

`spaxel-sensor` carries a Lua path filter that drops a push before any
workflow is created when **every** changed path in **every** commit matches:

| Ignored | Rationale |
|---|---|
| `docs/**` | documentation |
| `.beads/**`, `.needle*` | bead/NEEDLE bookkeeping |
| `*.md` (any depth) | README, PROGRESS, root notes |
| `LICENSE`, `.gitignore` | non-functional |

It **fails open**: malformed/truncated payloads build rather than skip, so a
release can never be silently dropped. The gap is the opposite direction —
non-functional paths *outside* the list still trigger builds (§6).

---

## 4. AC4 — Deployment manifests / Kubernetes specs

**None in this repository.** Verified: `git ls-tree -r HEAD` contains no
`kind:`-bearing manifests, no Helm chart, no kustomization. `docs/deployment/*`
is prose (bench connectivity, env vars, migration guide, WiFi configuration),
not manifests.

- **Local:** `docker-compose.yml` (§1) — the only deployment manifest in-repo.
- **Production:** `jedarden/declarative-config` → `k8s/…` ArgoCD-managed
  manifests (the Deployment setting `SPAXEL_BIND_ADDR`/advertised base URL,
  IngressRoute, the `spaxel-build` template, `spaxel-sensor`). Changes there
  are deploy-affecting without any rebuild of this repo, and conversely the
  CI step `update-declarative-config` is the bridge that turns a successful
  build here into a production rollout.
- `acceptance-test-hang-workflow.yml` is an Argo *Workflow* run manifest
  committed in-repo for debugging; it is not a deployment manifest (§3).

---

## 5. AC5 — Other build-affecting configuration

### 5.1 Tier A — changes shipped image content

| Path | Why |
|---|---|
| `mothership/**/*.go` | Compiled into `/spaxel` |
| `mothership/go.mod`, `mothership/go.sum` | Dependency set; layer-cached first in the builder |
| `dashboard/**` (all of it) | Embedded wholesale by `//go:embed dashboard` — includes tests, configs, reports (§1.2) |
| `mothership/cmd/mothership/dashboard_embed.go` | Holds the embed pattern itself; narrowing it changes image content |
| `mothership/cmd/sim/**` | `/spaxel-sim` is baked into the image; the simulator lives inside the mothership module since `d1f6ecba` (before that a separate root module — §8.11) |
| `go.work`, `go.work.sum` | Workspace, single `use ./mothership` since `d1f6ecba`; governs module resolution for both builds |
| `Dockerfile`, `.dockerignore` | §1 |
| `VERSION` | Image tag, `-X main.version` ldflag, firmware release filename (`spaxel-firmware-${VERSION}.bin`), and the OTA store's version source. At first-draft HEAD `ba620c83`: `0.2.158`; at re-verification `bedfc4a3`: `0.2.166` (CI auto-bumps it per build). The `plan.md` figure `0.1.357` is stale |
| `firmware/main/**`, and the firmware build config in §5.4 | Reaches the image transitively through the CI `firmware-build` release artifact that stage 1 fetches (§1.1) |

### 5.2 Tier B — gates CI, no image content change

| Path | Gate |
|---|---|
| `.golangci.yml` | `golangci-lint` step (resolves from `mothership/` to the root file) — a red here means `firmware-build`/`docker-build` never run |
| `firmware/test/**` (`Makefile`, `test_*.c`, `test_runner.c/.h`) | `firmware-test` step — the host gcc harness (`make -C firmware/test test`); no ESP-IDF needed |
| `dashboard/tests/**`, `dashboard/playwright.config.js`, `dashboard/package.json`, `dashboard/package-lock.json` | `a11y-test` step (Playwright + axe-core) |
| `dashboard/jest.config.js`, `dashboard/js/*.test.js` + their setup files | dashboard unit tests (jest; `npm test`) — not currently a `spaxel-build` step, but they gate local/other verification |
| `mothership/test/acceptance/**`, `mothership/tests/e2e/**` | `acceptance-test` / `go-test` legs (the root-level `test/acceptance/` and `tests/e2e/run.sh` cited in the first draft are gone — §8.11) |
| `scripts/*` (run-sim-*.sh, flash-esp32s3.sh, measure_csi_rate.py, provision_esp32.py, capture-dashboard-console.mjs, test-github-api.sh) | exercised by acceptance/e2e paths, not by the image build |

These decide whether a release *happens*; they don't change what ships. A path
filter that wants "test-only change ⇒ run gates but skip the deploy" needs this
tier kept distinct from Tier A.

### 5.3 Tier C — deployment-shaped, no rebuild

| Path | Effect |
|---|---|
| `docker-compose.yml` | Local redeploy (and local *build* via its `build:` block — see §1) |
| *(external)* `declarative-config` manifests | Production redeploy via ArgoCD; `update-declarative-config` is the automated bridge |

### 5.4 Firmware build configuration (missed by the prior catalogs)

| File | Role |
|---|---|
| `firmware/CMakeLists.txt` | Top-level project; sets the `SDKCONFIG_DEFAULTS` layering default `sdkconfig.defaults;sdkconfig.usbjtag` when not overridden, and configures `main/version.h.in` |
| `firmware/main/CMakeLists.txt` | Component registration + version header generation |
| `firmware/sdkconfig.defaults` | Base ESP-IDF config (CSI, BLE/coexistence, OTA rollback, partition table) |
| `firmware/sdkconfig.usbjtag` | Board-variant layer: USB-Serial/JTAG as primary console (default build) |
| `firmware/sdkconfig.uart-console` | Board-variant layer: UART0 console (bridge-equipped boards; selected explicitly) |
| `firmware/partitions.csv` | Flash partition layout — OTA slot geometry; **cannot be delivered over OTA** |
| `firmware/main/version.h.in` | `configure_file` template the build expands into the firmware version header — this is where `VERSION` reaches the binary |
| `firmware/main/idf_component.yml` | ESP-IDF component-manager manifest (declares `esp_websocket_client`, `mdns`, …) |
| `firmware/dependencies.lock` | Lockfile pinning those components' versions — the firmware analogue of `go.sum` |
| `firmware/scripts/*` (`generate-signing-key.sh`, `sign-firmware.sh`, `verify-console-config.sh`) | Signing / console-config verification helpers used around the firmware build |

All of these are Tier A via the release artifact (§5.1).

### 5.5 Explicitly not build-affecting

`docs/**`, `*.md`, `LICENSE`, `.gitignore`, `.gitattributes` (a single
`.beads/issues.jsonl merge=beads` driver), `.beads/**`, `.needle.yaml`,
`.needle-predispatch-sha`, `.marathon/**`, `memory/`, `notes/`, `testdata/`,
`data/`, `tmp/`, and the repo-residue files catalogued in §6.

---

## 6. Non-functional tracked paths that currently DO trigger a build

The landed sensor ignore list (§3.1) is prefix/extension-based and fails open.
Everything below is tracked at HEAD, is non-functional for builds, and falls
**outside** that list — so a commit touching only these still fires
`spaxel-build`, bumps `VERSION`, and rolls production. This is the concrete,
actionable gap this inventory hands to the filter.

| Group | Tracked paths (re-verified at `bedfc4a3`) |
|---|---|
| Debug run manifest | `acceptance-test-hang-workflow.yml` |
| Repo bookkeeping | `.gitattributes`, `.needle-predispatch-sha` (matched by the `.needle*` prefix rule only in its `.needle*` form — verified `.needle-predispatch-sha` *is* covered) |
| Dashboard non-app assets | `dashboard/leak-detection-report.json`, `dashboard/leak-isolation-results.json`, `dashboard/leak-test-full-lifecycle.json` — Tier A by embed (§1.2), so ignoring them in the sensor is a judgement call; §7.4 is the clean fix |
| Misc trees | `.marathon/**` (3 files), `memory/**` (1), `notes/**` (55), `testdata/**` (2) |

Resolved between the first draft and re-verification, so no longer part of the
gap: the root scratch/diagnostic residue (all thirteen `.txt`/`.json` files —
`corruption-report.txt`, `dangling-results.txt`, `fsck-results.txt`,
`idx-results.txt`, `midx-results.txt`, `packs-results.txt`,
`refdb-results.txt`, `refs-results.txt`, `mothership-code-search.txt`,
`mothership-files.txt`, `mothership-matches.txt`,
`verify-pack-corruption-indicators.txt`,
`verify-pack-corruption-indicators.json`), the helper scripts without gate
coverage (`blob_observation.sh`, `window_test.sh`, `fix_ble_handlers.py`), and
`dashboard/test-results/.last-run.json` are no longer tracked at HEAD — removed
by the root-exhaust and dashboard-catalog cleanups (§8.12). The tracked root
listing at re-verification is
`.dockerignore .gitattributes .gitignore .golangci.yml .needle-predispatch-sha
.needle.yaml Dockerfile LICENSE PROGRESS.md README.md VERSION
acceptance-test-hang-workflow.yml docker-compose.yml go.work go.work.sum` plus
directories.

Two caveats before adding any of these to `ignored_path`:

1. `dashboard/**` entries are **Tier A by embed** (§1.2) even though they are
   obviously non-functional — ignoring them in the sensor is safe only because
   nothing at runtime reads them, which is a judgement call, not a mechanical
   one. The cleaner fix is to stop embedding them (§7).
2. The filter fails open by design; every addition should preserve that
   property (ignore-list entries are a *narrowing* of what builds, so each one
   is a small bet that the path is truly inert).

Recommended additions, in confidence order: root `*.txt`/`*.json` residue,
`acceptance-test-hang-workflow.yml`, `.gitattributes`, `.marathon/**`,
`memory/**`, `notes/**`, `testdata/**`, and `.dockerignore`-policy leftovers
once the embed surface is pruned.

---

## 7. Recommendations for the path-filter consumer

1. **Treat the sensor ignore list as necessary but incomplete.** Add the
   remaining §6 groups (`.gitattributes` and the misc trees first — zero risk;
   the dashboard leak JSONs only after the embed surface is pruned or a runtime
   read is ruled out; `acceptance-test-hang-workflow.yml` should ideally be
   deleted rather than ignored).
2. **Keep Tier B out of the ignore list.** `.golangci.yml`,
   `firmware/test/**`, `dashboard/tests/**`, acceptance tests must keep
   triggering runs: they gate releases.
3. **Don't model "deploy-only" in the filter.** Tier C (`docker-compose.yml`)
   is rare and cheap; skipping it buys nothing and risks a stale local path.
4. **Prune the embed surface.** Narrowing `//go:embed dashboard` to the
   app-shipped subset (`dashboard/*.html`, `dashboard/js/**`,
   `dashboard/css/**`, `dashboard/static/**`) would (a) shrink the binary,
   (b) make `dashboard/**` classification honest, and (c) remove the §6
   caveat. Same end state via a build-time prune of the copied
   `cmd/mothership/dashboard/` tree.
5. **Prune the build context.** Extending `.dockerignore` to `.beads/`,
   `data/`, `memory/`, `notes/`, `testdata/`, `tmp/`, and the root residue
   files removes dead bytes from every context upload without touching image
   content (nothing `COPY`s them).
6. **~~Clean up the residue itself~~ — done between drafts.** The §6 root
   scratch files and the uncovered helper scripts were diagnostic debris from a
   2026-08–09 pack-corruption investigation and are no longer tracked (§6,
   §8.12). Nothing left to do here.

---

## 8. Errata against the prior catalogs (2026-08-27)

Both `docs/build-paths-catalog.md` and `docs/BUILD_PATHS.md` predate this bead
by minutes and cover AC1/AC4/AC5 substantially; the corrections below are why
this inventory exists rather than a pointer.

1. **`docker-compose.yml` is not deploy-only.** It has a `build:` block, so it
   is image-build-affecting too. Both catalogs list it as "No rebuild".
2. **`dashboard/**` is embedded in full**, including tests, jest/playwright
   configs and report JSONs (`//go:embed dashboard`). `BUILD_PATHS.md`'s
   note that `dashboard/node_modules/` and `dashboard/test-results/` "do NOT
   trigger builds" is wrong at the embed layer (`test-results/.last-run.json`
   is tracked and embedded).
3. **`.dockerignore` is omitted from both**, and it is build-affecting.
4. **`.golangci.yml` is omitted from both**, yet it is the CI lint gate that
   blocks `firmware-build`/`docker-build` from ever running.
5. **The Rust/Cargo AC is unanswered in both.** Answer: zero Rust (§2).
6. **`BUILD_PATHS.md` places the simulator at `mothership/cmd/sim/`.** It is
   `cmd/sim/` at the repo root, a separate Go module (three-module workspace:
   `mothership`, `cmd/sim`, `test/acceptance`).
7. **`BUILD_PATHS.md`'s example paths-ignore list would break gating** — it
   ignores `**_test.go`, `firmware/test/**`, `test/**`, `tests/**` wholesale,
   which would let lint/test/acceptance-gated changes skip CI entirely (§5.2).
8. **Firmware build config is under-listed.** Both name only
   `CMakeLists.txt`/`partitions.csv`/`sdkconfig.defaults`; the variant
   sdkconfig layers (`sdkconfig.usbjtag`, `sdkconfig.uart-console`),
   `dependencies.lock`, `main/idf_component.yml`, `main/version.h.in`, and
   `main/CMakeLists.txt` are missing (§5.4).
9. **"Firmware binary upload to GitHub Releases" is now real** (the CI
   `firmware-build` step) but happens *outside* the image build — the
   Dockerfile fetches rather than compiles. `plan.md`'s Dockerfile prose
   (in-image `espressif/idf` stage, `make -C test test` inside
   `firmware-builder`) is stale on the same point; the authoritative in-repo
   records are `docs/research/docker-build-step-configuration.md` and
   `docs/research/spaxel-build-architecture-targeting.md`.
10. **`VERSION` figure is stale** in prose that cites it: `0.2.158` at HEAD,
    not `0.1.357`.
11. **The tree moved under this doc after its first draft.** `d1f6ecba`
    consolidated the Go layout: `go.work` is now `use ./mothership` only, the
    simulator lives at `mothership/cmd/sim/` inside that single module
    (erratum 6's "it is `cmd/sim/` at the repo root, a separate Go module"
    no longer holds), and root `test/acceptance/` plus `tests/e2e/run.sh` are
    gone — acceptance tests live at `mothership/test/acceptance/`, e2e at
    `mothership/tests/e2e/`. §5.1 and §5.2 corrected in place; §1.1's builder
    prose still describes the Dockerfile accurately because it builds
    `./cmd/sim` from inside the mothership module either way.
12. **The root residue gap narrowed.** The §6 root scratch files, the three
    uncovered helper scripts, and `dashboard/test-results/.last-run.json` were
    removed from the repo between this doc's first draft and its
    re-verification at `bedfc4a3`; the live gap is the four groups §6 now
    lists, and §7.6 is closed.

---

## 9. Verification commands

Reproduce any claim above against whatever HEAD is current:

```bash
# Docker-related files
git ls-tree HEAD --name-only | grep -i docker

# Rust/Cargo (expect zero hits)
git ls-tree -r HEAD --name-only | grep -iE 'cargo|\.rs$'

# In-repo CI-ish files
git ls-tree -r HEAD --name-only | grep -iE '\.github|argo|workflows|golangci|\.ya?ml$'

# Kubernetes manifests in-repo (expect none)
git ls-tree -r HEAD --name-only | grep -iE 'k8s|kube|helm|kustomiz'

# Go module/workspace files
git ls-tree -r HEAD --name-only | grep -E 'go\.(mod|sum|work)'

# Firmware build config
git ls-tree -r HEAD --name-only -- firmware | grep -vE '\.(c|h)$'

# Embed surface (why dashboard/** is Tier A)
git grep -n 'go:embed' HEAD -- mothership

# Which lint config the CI step picks up from mothership/
( cd mothership && golangci-lint config path )   # → ../.golangci.yml

# Compose build block
git show HEAD:docker-compose.yml | grep -nA2 '^ *build:'
```
