# Spaxel Build Path Filter — Consolidated Specification

**Bead:** spaxel-b4b8b3db — "Compile complete path filter list for spaxel builds"
**Date:** 2026-09-05
**Verified at:** HEAD `9f8bffcd` (= `origin/main`), via `git ls-tree` / `git grep`
against the commit — not the working tree, which carries twins' in-flight edits.
**Supersedes:** `docs/BUILD_PATHS.md` and `docs/build-paths-catalog.md`
(2026-08-27 prior catalogs; corrected by `build-config-file-inventory.md` §8 and
consolidated here). Those two should be deleted once this spec is consumed.

This is the final deliverable of the path-filter split chain: one place that
says which paths must trigger `spaxel-build`, which must not, and what the
filter should look like when implemented.

## Inputs consolidated

| Source bead | Deliverable | Contribution |
|---|---|---|
| spaxel-eeafe10b | `docs/research/esp32-firmware-code-directories.md` | firmware tree survey |
| spaxel-4e6f529b | `docs/research/go-backend-code-directories.md` | mothership package survey |
| spaxel-7fa15b01 | `docs/research/build-config-file-inventory.md` | build-config tiers + the gap list (§6 there) |
| spaxel-6816405f (landed) | `docs/notes/ci-doc-only-push-path-filter.md` | the implemented sensor filter + its behavioral verification |
| — | `jedarden/declarative-config` → `k8s/iad-ci/argo-events/spaxel-sensor.yml` (read at local `0ec96fc9`, 2026-09-04) | ground truth of what actually runs |

---

## 0. Answer summary

| Acceptance criterion | Result |
|---|---|
| Firmware paths that trigger builds | `firmware/main/**` + 6 config files + `firmware/scripts/*.sh`; `firmware/test/**` as a **gate** (§3) |
| Mothership Go paths that trigger builds | `mothership/**/*.go` (61 Go-bearing dirs at HEAD), `mothership/go.{mod,sum}`, `go.work`, `go.work.sum` (§4) |
| Build configuration files that trigger builds | `Dockerfile`, `.dockerignore`, `docker-compose.yml`, `VERSION`, `.golangci.yml`, and all of `dashboard/**` by embed (§5) |
| Excluded paths and why | §6 — the live ignore list, the Tier-B set that must never be excluded, and the confidence-ordered additions still missing |
| Implementation-ready deliverable | §7 — the trigger table plus the concrete `ignored_path` patch for the sensor |

There is **no Rust in this repository** — re-verified at HEAD (`git ls-tree -r
HEAD --name-only | grep -iE 'cargo|\.rs$'` → 0 hits). No `Cargo.toml` /
`Cargo.lock` / `*.rs` pattern belongs in any trigger or ignore list.

---

## 1. Where the filter lives and what it does today

The filter is **already implemented** at the argo-events sensor, in the
declarative-config repo (not here). `spaxel-sensor`'s `spaxel-push` dependency:

- matches `push` events on `refs/heads/main` from the `github-webhooks`
  event source for `spaxel`;
- drops events whose `head_commit.author.name` is `"Argo Workflows CI"` —
  the cascade guard that stops the CI `VERSION` bump from re-triggering;
- runs a Lua script that skips the push only when **every** changed path in
  **every** commit matches the ignore list:

  | `ignored_path` rule | Matches |
  |---|---|
  | prefix `docs/` | all documentation |
  | prefix `.beads/` | bead checkpoint churn |
  | prefix `.needle` | `.needle.yaml`, `.needle-predispatch-sha` |
  | `%.md` suffix (any depth) | every markdown file, repo-wide |
  | exact `LICENSE` | license text |
  | exact `.gitignore` | ignore rules |

- **fails open**: missing `commits`, non-table path lists, and GitHub's
  truncated-payload case (`size > #commits`) all **build** rather than skip.
  A substantive release can never be silently dropped by a malformed webhook;
- gates **both** triggers — `spaxel-build` and `spaxel-e2e` are submitted on
  `conditions: spaxel-push`, so one ignored push runs neither workflow.

**Design consequence for this spec:** the sensor implements the *complement*
of the "should trigger" lists below. An ignore list that fails open is the
correct shape for a release pipeline — an unknown path builds, it is never
silently skipped — so the positive lists in §3–§5 are the reference
classification, and the actionable artifact is the ignore list plus its
missing entries (§6.3, §7.2).

## 2. The three tiers

Not "source vs. not" — the filter-relevant distinction is:

- **Tier A — changes shipped image content.** Must trigger build + deploy.
- **Tier B — gates CI, changes no image content.** Must keep triggering:
  these decide whether a release *happens*. Excluding them would let a
  lint/test/acceptance regression ship unvetted.
- **Tier C — deployment-shaped, no rebuild.** Restart/redeploy only. Not
  worth modelling in the filter (§7.3).

---

## 3. AC1 — firmware paths that trigger builds

### 3.1 Watch list

| Pattern | Tier | Why |
|---|---|---|
| `firmware/main/**` | A | The firmware: 24 tracked `.c`/`.h` at HEAD (12 `.c` + 12 `.h`), plus `main/CMakeLists.txt` (component registration + version header generation), `main/version.h.in` (the `configure_file` template through which `VERSION` reaches the binary), `main/idf_component.yml` (component manifest) |
| `firmware/CMakeLists.txt` | A | Top-level project; sets the `SDKCONFIG_DEFAULTS` layering (`sdkconfig.defaults;sdkconfig.usbjtag` when not overridden) |
| `firmware/sdkconfig.defaults` | A | Base ESP-IDF config — CSI, BLE/coexistence, OTA rollback, partition table |
| `firmware/sdkconfig.usbjtag` | A | Board-variant layer: USB-Serial/JTAG console (default build) |
| `firmware/sdkconfig.uart-console` | A | Board-variant layer: UART0 console (bridge-equipped boards) |
| `firmware/partitions.csv` | A | Flash partition layout / OTA slot geometry; cannot be delivered over OTA |
| `firmware/dependencies.lock` | A | Component-manager lockfile — the firmware analogue of `go.sum` |
| `firmware/scripts/*.sh` | A | `generate-signing-key.sh`, `sign-firmware.sh`, `verify-console-config.sh` — used around the `firmware-build` step; `.sh` is not matched by any ignore rule |

Firmware markdown (`firmware/README.md`, `BUILD.md`, `CONTRIBUTING.md`,
`firmware/docs/*`, `firmware/test/*.md`, `firmware/scripts/README.md`) is
already suppressed by the `*.md` rule — no per-file entries needed.

### 3.2 Why firmware source triggers the *image* build

Firmware never compiles inside the Docker build (the `plan.md` prose
describing an in-image ESP-IDF stage is stale). The chain is: CI's
`firmware-build` step (`espressif/idf:v5.2`, hardcoded `esp32s3`) uploads
`spaxel-firmware-${VERSION}.bin` and `-merged.bin` to GitHub Releases →
Dockerfile stage 1 (`firmware-fetcher`) curls them by the resolved `VERSION`
→ the runtime image copies them into `/firmware/`. A firmware-only change
therefore still requires the full image build, because the artifact
filename is version-bearing and is baked into the image.

### 3.3 Firmware exclusions

| Path | Why excluded |
|---|---|
| `firmware/build/` | gitignored generated tree (`.gitignore:16`) — can never fire |
| `firmware/managed_components/` | gitignored, fetched by the component manager (`.gitignore:17`) |
| `firmware/test/build/` | gitignored (`firmware/test/.gitignore:3`) |
| `data/firmware/` | runtime OTA seed store, outside `firmware/` entirely; not firmware source |
| `*.cpp *.cc *.cxx *.hpp *.S *.ino *.ld *.lds` | **zero tracked files** of any of these extensions repo-wide; the only `.ld` on disk are generated ESP-IDF linker scripts inside the gitignored build tree. Nothing to watch |

### 3.4 `firmware/test/**` is a gate, not an exclusion

`firmware/test/` (`Makefile`, `test_*.c`, `test_runner.c/.h`) is the host-gcc
harness behind the CI `firmware-test` step — no ESP-IDF needed. It is
**Tier B**: keep triggering. Residue caveat: `firmware/test/test_runner` is a
**tracked 36,592-byte ELF binary** (committed build output) — see §6.3.

---

## 4. AC2 — mothership Go paths that trigger builds

### 4.1 Watch list

| Pattern | Tier | Why |
|---|---|---|
| `mothership/**/*.go` | A | Compiled into `/spaxel`. 61 Go-bearing dirs at HEAD `9f8bffcd`: 56 under `internal/` (including nested `localizer/fusion`), 2 under `cmd/`, 3 test dirs (`mothership/test`, `mothership/test/acceptance`, `mothership/tests/e2e`). Down one from the 2026-09-04 survey's 62 — `mothership/cmd/_parse_check.go` was deleted in the residue cleanup |
| `mothership/go.mod`, `mothership/go.sum` | A | Dependency set; layer-cached first in the builder (`COPY mothership/go.mod mothership/go.sum` → `go mod download`) |
| `mothership/cmd/mothership/**` | A | Primary binary: `main.go` (6,029 lines at HEAD, all subsystem wiring) and `dashboard_embed.go:11` — the unqualified `//go:embed dashboard` that makes all of `dashboard/**` Tier A |
| `mothership/cmd/sim/**` | A | `/spaxel-sim` is baked into the image; simulator lives inside the single mothership module since `d1f6ecba` (`go.work` = `use ./mothership` only). Its `Makefile` sits inside an already-watched path |
| `go.work`, `go.work.sum` | A | Workspace, governs module resolution for both builds |

### 4.2 Go-specific notes

- **Build tags.** `-tags=embed` activates `dashboard_embed.go`;
  `//go:build ignore` files (`mothership/cmd/mothership/migrate.go`,
  `mothership/internal/oui/gen_data.go`,
  `mothership/internal/recording/benchmark.go`) are not compiled into either
  binary. They are inert for image content, but they are three files inside
  watched paths — no separate rule is warranted (fail-open; a wrong bet here
  buys nothing).
- **No vendor/ tree, no generated code.** Zero tracked files under
  `mothership/vendor/`; zero `_pb.go` / `_string.go` / `zz_generated*`
  anywhere. Nothing to exclude on those grounds.
- **A raw `package main` grep overstates entrypoints** (8 dirs vs. the 2 real
  ones) precisely because of the build tags above — the survey documented
  this; don't "fix" the watch list from a naive grep.

### 4.3 Mothership exclusions

| Path | Why excluded / inert |
|---|---|
| `mothership/**/*.md` | README + design notes — already suppressed by the `*.md` rule |
| `mothership/internal/db/fix_migration.sed`, `mothership/test/fix_section.txt` | Dead: zero code references (verified `git grep` at HEAD). Even unignored they would only invalidate a builder-stage layer — the runtime stage copies **only** `/spaxel`, `/spaxel-sim` and the firmware artifacts, so nothing under `mothership/` reaches the image except compiled output and the embedded dashboard |
| compiled artifacts (`mothership/mothership`, `mothership/sim`, `*.test`, `mothership/cmd/sim/sim`, `mothership/cmd/mothership/dashboard/`) | gitignored (`.gitignore:42-56`) |
| out-of-module Go | `docs/*.go`, `docs/examples/*.go` (suppressed by `docs/`); `testdata/generate_csi_recording.go`, `testdata/verify_recording.go` — invisible to `go build`/`go vet`, proposed for ignore in §6.3 |

---

## 5. AC3 — build configuration files that trigger builds

| File | Tier | Effect |
|---|---|---|
| `Dockerfile` | A | Defines the image: `firmware-fetcher` (alpine) → `builder` (`golang:1.25-bookworm`, `COPY mothership/` + `COPY dashboard/`, `-tags=embed` builds of `./cmd/mothership` and `./cmd/sim`) → `distroless/static-debian12` runtime copying only the two binaries and the firmware artifacts |
| `.dockerignore` | A | Filters the build *context*; adding/removing a rule changes what `COPY` can pick up. Today it excludes `docs/`, `*.md` (with `!README.md`), `.marathon/`, `firmware/{build,managed_components,.cache}`, IDE/OS files, `tmp/`/`temp/` — and does **not** exclude `.beads/`, `data/`, `memory/`, `notes/`, `testdata/`, which are uploaded as dead bytes on every build |
| `docker-compose.yml` | A + C | Carries a `build:` block (`context: .`, `dockerfile: Dockerfile`) — not deploy-only — and encodes the local deployment surface (volumes, env, healthcheck, Traefik labels) |
| `VERSION` | A | Image tag, `-X main.version` ldflag, the firmware release filename (`spaxel-firmware-${VERSION}.bin`) and the OTA store's version source. Auto-bumped by CI per build (`0.2.167` at HEAD; any figure in prose is immediately stale) |
| `go.work`, `go.work.sum` | A | §4.1 |
| `.golangci.yml` | **B** | The only lint config; resolves from `mothership/` to this root file. A red here fails the workflow before `firmware-build`/`docker-build` ever run — **must keep triggering** |
| `dashboard/**` — all of it | A | Embedded wholesale by `//go:embed dashboard`: app JS/CSS/HTML (91 js / 29 css / 9 html at HEAD) **and** `jest.config.js`, `playwright.config.js`, `package.json`, `package-lock.json`, `dashboard/tests/**` (8 files), the leak/profiling report JSONs, `static/icons/*`, `help_articles.json`, `manifest.json`. Any change under `dashboard/` is image-content-affecting until the embed surface is pruned |

**CI/CD configuration:** none in-repo by policy — no `.github/` (disabled
org-wide), and the `spaxel-build` / `spaxel-e2e` WorkflowTemplates plus this
sensor live in `jedarden/declarative-config`. In-repo stragglers:
`acceptance-test-hang-workflow.yml` (a committed debug Argo `kind: Workflow`
*run manifest* — triggers nothing, and is exactly the top-level YAML the
filter's residual-risk note warns about).

**Deployment manifests:** none in-repo. Production manifests are
ArgoCD-managed in `declarative-config`; changes there roll production without
any rebuild of this repo, and `update-declarative-config` is the CI step that
bridges a successful build here into that rollout. That repo's changes are
outside this filter's scope entirely.

---

## 6. AC4 — excluded paths, and why

### 6.1 Already excluded (live, verified in `spaxel-sensor.yml`)

`docs/**`, `.beads/**`, `.needle*`, `*.md` (any depth), `LICENSE`,
`.gitignore` — plus the two event-level guards (branch `main` only; author
`!= "Argo Workflows CI"`). Rationale and the fail-open property are in §1.
`.needle-predispatch-sha` **is** covered by the `.needle` prefix rule.

### 6.2 Must NEVER be excluded (Tier B — these gate releases)

| Path | Gate that would silently pass |
|---|---|
| `.golangci.yml` | `golangci-lint` step — a red here is what stops `firmware-build`/`docker-build` |
| `firmware/test/**` | `firmware-test` (host gcc harness) |
| `dashboard/tests/**`, `dashboard/playwright.config.js`, `dashboard/package.json`, `dashboard/package-lock.json` | `a11y-test` (Playwright + axe-core) |
| `dashboard/jest.config.js`, `dashboard/js/*.test.js` | dashboard unit tests |
| `mothership/test/acceptance/**`, `mothership/tests/e2e/**` | `acceptance-test` / `go-test` legs |
| `scripts/*` used by those gates | acceptance/e2e support |

Erratum to keep with this spec: `docs/BUILD_PATHS.md`'s example ignore list
(`**_test.go`, `firmware/test/**`, `test/**`, `tests/**`) would let exactly
these gated changes skip CI entirely. Do not implement that list; this
specification replaces it.

### 6.3 Proposed additional ignores — the concrete, still-open gap

Everything below is tracked at HEAD `9f8bffcd`, non-functional for builds,
and **outside** the current ignore list — so a commit touching only these
still fires `spaxel-build`, bumps `VERSION`, and rolls production. Ordered by
confidence (how safe the narrowing bet is):

| # | Paths | Why inert | Action |
|---|---|---|---|
| 1 | `.gitattributes` | One line: the `.beads/issues.jsonl merge=beads` driver | Add to `ignored_path` |
| 2 | `acceptance-test-hang-workflow.yml` | Debug run manifest; runs nothing | **Delete the file** (preferred); ignore as the fallback |
| 3 | `.marathon/**` (3 files), `memory/**` (1), `notes/**` (55), `testdata/**` (2) | Agent/residue trees — nothing `COPY`s or embeds them; `testdata/`'s two Go files are outside every module | Add prefix rules |
| 4 | `mothership/internal/db/fix_migration.sed`, `mothership/test/fix_section.txt` | Dead, zero references; builder-stage-only bytes | Untrack (preferred) or ignore |
| 5 | `firmware/test/test_runner` | A tracked 36,592-byte **ELF binary** — committed build output | **Untrack** (and gitignore it in `firmware/test/.gitignore`); do not merely ignore — a rebuilt binary would re-fire |
| 6 | `dashboard/leak-detection-report.json`, `dashboard/leak-isolation-results.json`, `dashboard/leak-test-full-lifecycle.json`, `dashboard/test-profiling-results.json`, and the other dashboard non-app assets | Non-functional **today**, but Tier A by embed — ignoring them in the sensor is a judgement call, not a mechanical one | Only after the embed surface is pruned (`//go:embed dashboard` → the app-shipped subset); then these become mechanical ignores |

Caveats that hold for every addition: the filter fails open by design, and an
ignore entry is a *narrowing* of what builds — each one is a small bet that
the path is truly inert. Keep each addition COPY-inert **and** embed-inert
(`dashboard/**` is the trap: it is embedded, not just copied).

---

## 7. Implementation-ready specification

### 7.1 The complete trigger set (positive form)

| Trigger group | Patterns | Tier |
|---|---|---|
| Go backend | `mothership/**/*.go`, `mothership/go.mod`, `mothership/go.sum`, `go.work`, `go.work.sum` | A |
| Dashboard (embed surface) | `dashboard/**` | A |
| Firmware source + config | `firmware/main/**`, `firmware/CMakeLists.txt`, `firmware/sdkconfig.defaults`, `firmware/sdkconfig.usbjtag`, `firmware/sdkconfig.uart-console`, `firmware/partitions.csv`, `firmware/dependencies.lock`, `firmware/scripts/*.sh` | A |
| Firmware gate | `firmware/test/**` | B |
| Docker | `Dockerfile`, `.dockerignore`, `docker-compose.yml` | A (+C for compose) |
| Lint gate | `.golangci.yml` | B |
| Test gates | `mothership/test/acceptance/**`, `mothership/tests/e2e/**`, `dashboard/tests/**`, `dashboard/jest.config.js`, `dashboard/playwright.config.js`, `dashboard/package*.json` | B |
| Tooling | `scripts/**` | B |
| Anything else tracked | — | **builds (fail-open default)** |

### 7.2 The concrete ignore-list patch (proposed)

Against the live `ignored_path` function, add — in §6.3 confidence order:

```lua
local function ignored_path(path)
  return string.sub(path, 1, 5) == "docs/"
    or string.sub(path, 1, 7) == ".beads/"
    or string.sub(path, 1, 7) == ".needle"
    or string.match(path, "%.md$") ~= nil
    or path == "LICENSE"
    or path == ".gitignore"
    -- additions (spaxel-b4b8b3db §6.3): each verified COPY- and embed-inert
    or path == ".gitattributes"
    or path == "acceptance-test-hang-workflow.yml"
    or string.sub(path, 1, 10) == ".marathon/"
    or string.sub(path, 1, 7) == "memory/"
    or string.sub(path, 1, 6) == "notes/"
    or string.sub(path, 1, 9) == "testdata/"
  end
```

Items 4–6 of §6.3 are handled **outside the sensor** (untrack the dead
`mothership/` stragglers and the `firmware/test/test_runner` ELF; prune the
embed surface before touching `dashboard/**` entries) — ignoring them in the
sensor would paper over tracked residue that should simply not be in git.

### 7.3 Invariants for whoever edits the filter next

1. **Fail open stays.** Never invert to a positive allow-list at the sensor:
   an unknown path must build.
2. **Never add a §6.2 path.** Tier B keeps triggering even when the change is
   test-only.
3. **Do not model Tier C.** `docker-compose.yml` is rare and cheap; a
   deploy-only path class buys nothing and risks a stale local path.
4. **The author guard stays.** Removing the `Argo Workflows CI` author filter
   reintroduces the VERSION-bump cascade loop.
5. **Ignores are shared.** `spaxel-e2e` fires on the same dependency, so every
   ignore also suppresses the e2e run — intended; the e2e suite has nothing to
   learn from a docs push either.
6. **Workflow-level skip complements, never replaces.** `resolve-version`
   already refuses to bump on a `VERSION`-only diff; that run still consumes a
   slot, which is why the sensor-side list is the primary lever.

---

## 8. Drift notes (things that will look wrong later)

- **Go dir count.** 61 at HEAD `9f8bffcd` vs. 62 in the 2026-09-04 survey —
  `mothership/cmd/_parse_check.go` was removed in the residue cleanup. The
  pattern `mothership/**/*.go` is the stable fact; the count is not.
- **Line and file counts** (`main.go` 6,029; 34 firmware `.c`/`.h`; 91/29/9
  dashboard js/css/html) are HEAD snapshots for orientation — same caveat.
- **`VERSION`.** `0.2.167` at HEAD, auto-bumped per build. Never cite a
  version figure as current in prose.
- **Provenance of the sensor read.** Read from the local
  `/home/coding/declarative-config` checkout at `0ec96fc9` (2026-09-04,
  includes the `fce2f7e4` sed-scoping fix). The canonical source remains
  `jedarden/declarative-config`; verify there before editing.

## 9. Verification commands

```bash
# The live filter
sed -n '30,90p' /home/coding/declarative-config/k8s/iad-ci/argo-events/spaxel-sensor.yml

# Firmware source vs. config (config rows of §3.1)
git ls-tree -r HEAD --name-only -- firmware | grep -vE '\.(c|h)$'
git ls-tree -r HEAD --name-only -- firmware | grep -cE '\.(c|h)$'    # → 24 in main/ + 10 in test/

# Zero Rust / zero generated Go
git ls-tree -r HEAD --name-only | grep -icE 'cargo|\.rs$'            # → 0
git ls-tree -r HEAD --name-only -- mothership | grep -E '_pb\.go|zz_generated'

# Go-bearing dirs (→ 61 at 9f8bffcd)
git ls-tree -r HEAD --name-only -- mothership | grep -E '\.go$' | xargs -n1 dirname | sort -u | wc -l

# The embed surface that makes dashboard/** Tier A
git grep -n 'go:embed' HEAD -- mothership

# Ignored trees really are ignored
git check-ignore -v firmware/build firmware/managed_components firmware/test/build \
  dashboard/node_modules dashboard/test-results

# Tracked ELF residue (§6.3 #5)
git cat-file -p $(git rev-parse HEAD:firmware/test/test_runner) | od -A x -t x1 | head -1
```
