# Spaxel Build Path Filter — Consolidated Specification

**Bead:** spaxel-b4b8b3db — "Compile complete path filter list for spaxel builds"
**Date:** 2026-09-05
**Verified at:** HEAD `9f8bffcd`, confirmed an ancestor of `origin/main`.
**Purpose:** the single terminal deliverable of the path-identification chain.
Every path list below was re-derived from the tracked tree at dispatch time —
nothing is inherited from the earlier surveys — and the live filter predicate
was simulated over all 935 tracked paths to establish what actually triggers
today.

**Feeds:** `spaxel-e8c03eba` ("Add path filter to argo-events sensor for
spaxel") and any later rework of the filter. The implementation target is the
Lua predicate in `jedarden/declarative-config` →
`k8s/iad-ci/argo-events/spaxel-sensor.yml` (unchanged since 2026-08-08).

This document **supersedes** the earlier path lists, which remain on disk as
point-in-time snapshots and are not rewritten here (per the repo convention for
dated survey docs):

| Superseded input | Bead | Corrections carried forward |
|---|---|---|
| `docs/BUILD_PATHS.md` (2026-08-27) | — | §6.1, §6.2 |
| `docs/build-paths-catalog.md` (2026-08-27) | — | §6.1–§6.5 |
| `docs/research/build-config-file-inventory.md` | spaxel-7fa15b01 | §6.6, §6.7 |
| `docs/research/esp32-firmware-code-directories.md` | spaxel-62c8ab42 | — (still accurate) |
| `docs/research/go-backend-code-directories.md` | spaxel-f99f4f0f | §6.8 |
| `docs/notes/ci-doc-only-push-path-filter.md` | spaxel-6816405f | — (still accurate) |

---

## 0. Answer summary

Simulating the live sensor predicate over the tracked tree:

| | Count |
|---|---|
| Tracked paths at HEAD | 935 |
| Already dropped by the live filter (no build) | 299 |
| Would trigger a build today | **636** |

Of those 636, **612 are legitimately build-triggering** and **24 are inert**
paths the filter should also drop (§4.2). Nothing that should build is being
skipped — the filter's only defect is over-triggering, which is the safe
direction.

The consolidated specification is three lists:

1. **Trigger set — firmware** (§1): `firmware/main/**` plus 7 project-root
   build-config files. Tier A.
2. **Trigger set — mothership Go** (§2): the whole `mothership/` module, one
   Go module since `d1f6ecba`. Tier A.
3. **Trigger set — build configuration** (§3): `Dockerfile`, `.dockerignore`,
   `docker-compose.yml`, `VERSION`, `go.work`, `go.work.sum`, `.golangci.yml`.
4. **Exclusion set** (§4): what must never build, and why — split into
   already-implemented, safe-to-add, and embed-coupled (do not add yet).
5. **Ready-to-implement filter** (§5): the exact `ignored_path()` predicate.

---

## 1. Firmware paths that trigger builds

One firmware tree exists: `firmware/`, an ESP-IDF v5.2 project for the
ESP32-S3. All 24 tracked C/H source files live in `firmware/main/` (12 `.c` +
12 `.h`); the 10 in `firmware/test/` are the host-side gcc harness and are
**Tier B, not Tier A** (§4.3).

### 1.1 Tier A — firmware source and firmware build configuration

Every path below reaches the shipped image: CI's `firmware-build` step
compiles it (`espressif/idf:v5.2`, hardcoded `esp32s3`), uploads
`spaxel-firmware-${VERSION}.bin` and `-merged.bin` to GitHub Releases, and the
Dockerfile's `firmware-fetcher` stage fetches exactly those version-bearing
filenames into the image. A firmware change therefore always implies a full
image build and a production rollout.

| Path | Role |
|---|---|
| `firmware/main/**` | The firmware: 12 `.c` + 12 `.h`, `main/CMakeLists.txt` (component registration + version header generation), `main/version.h.in` (the `configure_file` template — **where `VERSION` reaches the binary**), `main/idf_component.yml` (component-manager manifest) |
| `firmware/CMakeLists.txt` | Top-level project; sets the `SDKCONFIG_DEFAULTS` layering (`sdkconfig.defaults;sdkconfig.usbjtag`) |
| `firmware/sdkconfig.defaults` | Base ESP-IDF config — CSI, BLE/NimBLE + coexistence, SPIRAM, OTA rollback + anti-rollback, custom partition table |
| `firmware/sdkconfig.usbjtag` | Board-variant layer: USB-Serial/JTAG console (the default build) |
| `firmware/sdkconfig.uart-console` | Board-variant layer: UART0 console (bridge-equipped boards, selected explicitly) |
| `firmware/partitions.csv` | Flash layout — `ota_0`/`ota_1` A/B geometry. **Cannot be delivered over OTA**; a change here needs serial reflash and is the highest-consequence path in the repo |
| `firmware/dependencies.lock` | Pins vendored component versions (`esp_websocket_client`, `mdns`) — the firmware analogue of `go.sum` |
| `firmware/scripts/**` | `generate-signing-key.sh`, `sign-firmware.sh`, `verify-console-config.sh` — signing and console verification around the build |

Not build inputs: `firmware/sdkconfig` and `firmware/sdkconfig.old` are
generated (gitignored); `firmware/build/` (199 MB) and
`firmware/managed_components/` are gitignored; `firmware/docs/` is prose;
`firmware/BUILD.md`, `firmware/README.md`, `firmware/CONTRIBUTING.md` are
markdown and already dropped by the `*.md` rule.

---

## 2. Mothership Go paths that trigger builds

One Go module since `d1f6ecba`: `go.work` is `use ./mothership` and nothing
else. There is no root `go.mod`, no root `cmd/sim`, and no root
`test/acceptance` — all three were consolidated away.

| Path | Tier | Role |
|---|---|---|
| `mothership/**` | A | The entire module is compiled into the image — both binaries |

### 2.1 What `mothership/**` contains (56 packages under `internal/`)

| Surface | Content | Ships as |
|---|---|---|
| `mothership/cmd/mothership/` (9 files) | Primary entrypoint. `main.go` wires every subsystem; `dashboard_embed.go` carries the `//go:embed dashboard` directive at line 11; `migrate.go` (tag `ignore_migrate`) is the standalone migration helper | `/spaxel` |
| `mothership/cmd/sim/` (6 files) | CSI simulator CLI — imports `internal/simulator` + `internal/ble`. **This is the simulator the image ships**; the root `cmd/sim` it used to be confused with no longer exists | `/spaxel-sim` |
| `mothership/internal/**` (56 packages) | All application code: `api` (48 files), `signal` (the real pipeline), `fusion`, `localization` (GDOP), `tracking` (wired UKF), `ingestion`, `fleet`, `ota`, `db`, `recording`+`recorder`, `replay`, `ble`, `sleep`, `prediction`, `analytics`, `automation`, `auth`, `mqtt`, `notify`, `simulator`, … | linked into `/spaxel` |
| `mothership/go.mod`, `mothership/go.sum` | Dependency set; layer-cached first in the builder | build input |
| `mothership/test/` (23 files) | In-module verification + acceptance tests | **Tier B** — gate, no image content |
| `mothership/tests/e2e/` (5 files) | `io6_gate`-tagged e2e + assertions | **Tier B** — gate, no image content |

`mothership/internal/localizer/fusion/` holds only a test-only timing-budget
gate; it never compiles into either binary but is still under `mothership/**`
and correctly keeps triggering.

No `.go` file outside `mothership/` is built. Five tracked Go files sit outside
any module and are invisible to `go build ./...` and `go vet ./...`:
`docs/gdop-usage-example.go`, `docs/gdop-usage-example-enhanced.go`,
`docs/examples/gdop_usage_examples.go`, `testdata/generate_csi_recording.go`,
`testdata/verify_recording.go`. The first three are under `docs/` and already
dropped; the two under `testdata/` are an exclusion gap (§4.2).

---

## 3. Build configuration files that trigger builds

Seven tracked root files. All Tier A except `.golangci.yml`, which is Tier B.

| Path | Tier | Why it must trigger |
|---|---|---|
| `Dockerfile` | A | Defines the image: `firmware-fetcher` (alpine:3.20) → `builder` (golang:1.25-bookworm, `--platform=$BUILDPLATFORM`) → runtime (distroless static). `COPY mothership/`, `COPY dashboard/ ./cmd/mothership/dashboard/`, builds `-tags=embed` |
| `.dockerignore` | A | Filters the **build context**. Adding or removing a rule changes what `COPY` can pick up, so it is image-content-affecting, not cosmetic. Currently excludes `docs/`, `*.md` (`!README.md`), `.marathon/`, `firmware/{build,managed_components,.cache}`, IDE/OS files, `tmp/` |
| `docker-compose.yml` | A + C | Carries a `build:` block (`context: .`, `dockerfile: Dockerfile`) — **not** deploy-only. Also encodes the local deployment surface (host networking, volumes, healthcheck, Traefik labels) |
| `VERSION` | A | Image tag, `-X main.version` ldflag, the firmware release filename `spaxel-firmware-${VERSION}.bin`, and the OTA store's version source. `0.2.167` at HEAD; CI auto-bumps it per build, so it changes on every functional push |
| `go.work` | A | Single `use ./mothership`. Governs module resolution for both binaries |
| `go.work.sum` | A | Workspace checksums |
| `.golangci.yml` | **B** | The only lint config; `golangci-lint config path` run from `mothership/` resolves to this root file. A red here fails the workflow **before** `firmware-build`/`docker-build` ever run. Must keep triggering — see §4.3 |

Zero Rust in this repository — no `Cargo.toml`, no `Cargo.lock`, no `*.rs`
tracked. No `Cargo`/`*.rs` pattern belongs in any trigger or ignore list.

No CI definition and no Kubernetes manifest live in this repository, by
policy: GitHub Actions are disabled org-wide, and the `spaxel-build`
WorkflowTemplate plus the `spaxel-sensor` live in `jedarden/declarative-config`.
The one tracked YAML, `acceptance-test-hang-workflow.yml`, is a committed
debug `kind: Workflow` **run manifest** — it triggers nothing and is an
exclusion candidate (§4.2).

---

## 4. Excluded paths — and why

Three distinct groups. The distinction matters: §4.1 and §4.2 are paths that
should never build; §4.3 is paths that look excludable and must **not** be.

### 4.1 Already excluded by the live filter (299 tracked paths)

| Pattern | Paths covered | Why |
|---|---|---|
| `docs/**` | 3 of the 5 non-module Go files, all doc trees | Documentation never changes image content |
| `*.md` (any depth) | `README.md`, `PROGRESS.md`, `notes/**` (55), `memory/**` (1), every `firmware/**/*.md` and `dashboard/**/*.md` | Markdown, wherever it sits. The `any depth` property is load-bearing: it is what already covers `notes/**` and `memory/**` without prefix rules |
| `.beads/**` | bead checkpoint + heartbeat churn | The most frequent push category in this repo; never functional |
| `.needle*` (prefix) | `.needle.yaml`, `.needle-predispatch-sha` | NEEDLE harness bookkeeping |
| `LICENSE` | 1 | Non-functional |
| `.gitignore` | 1 | Non-functional |

Plus two non-path guards in the same sensor: only `refs/heads/main` fires, and
commits authored by `Argo Workflows CI` (the version auto-bump) are dropped to
prevent a cascade loop.

The filter **fails open**: missing `commits`, a non-table path list, a
non-string path, or a GitHub-truncated payload (`size > #commits`) all build.
No release can be silently dropped by a malformed webhook.

### 4.2 Inert paths that currently DO trigger — recommended additions

These are tracked, outside the live ignore list, and change no image content
and no gate. Adding them is a *narrowing* of what builds, so each entry is a
small bet that the path is truly inert; the list is ordered by confidence.

| Add to ignore list | Paths | Why inert |
|---|---|---|
| **`data/`** | 17 — `data/*.db` (15), `data/backups/pre-upgrade-*.sqlite`, `data/.lock` | **Runtime SQLite state committed to the repo.** Nothing `COPY`s `data/`; it is the seed/store volume, not a build input. This is the largest single inert group and was absent from every prior catalog |
| **`testdata/`** | 2 — `generate_csi_recording.go`, `verify_recording.go` | Outside any Go module; `go build ./...` and `go vet ./...` never compile them. Fixture utilities. Rule must be the `testdata/` prefix, **not** `*.go` |
| **`.gitattributes`** | 1 | One line: a `.beads/issues.jsonl merge=beads` driver |
| **`acceptance-test-hang-workflow.yml`** | 1 | A committed debug Argo `kind: Workflow` run manifest. Triggers nothing. Better deleted than ignored — a committed `kind: Workflow` is repo residue either way |
| **`.marathon/**`** | 2 of 3 — `.marathon/start.sh`, `.marathon/.gitignore` | Marathon logs (the third file, `.marathon/README.md`, is already dropped by `*.md`). Already excluded from the Docker context by `.dockerignore` |
| **`firmware/docs/bluedroid-baseline.txt`** | 1 | A measurement baseline note; the sibling `.md` is already dropped |

### 4.3 Embed-coupled inert paths — do NOT add yet

`//go:embed dashboard` at `mothership/cmd/mothership/dashboard_embed.go:11` is
**unqualified**, so every tracked file under `dashboard/` is embedded into the
binary — including test reports and dev configs. Those files are Tier A by
mechanism even though nothing at runtime reads them. Ignoring them in the
sensor is a judgement call, not a mechanical one, and the clean fix is to
narrow the embed pattern, not to add ignore rules.

| Paths | Apparently inert | Why not to ignore |
|---|---|---|
| `dashboard/leak-detection-report.json`, `dashboard/leak-isolation-results.json`, `dashboard/leak-test-full-lifecycle.json`, `dashboard/test-profiling-results.json` | Test reports | Embedded; prune the embed surface first |
| `dashboard/jest.config.js`, `dashboard/playwright.config.js`, `dashboard/tsconfig.json`, `dashboard/package.json`, `dashboard/package-lock.json`, `dashboard/generate-icons.js`, `dashboard/run-leak-profiling.js`, `dashboard/tests/**` (8) | Dev/test tooling | Embedded; also `dashboard/tests/**` + the two test configs gate the `a11y-test` step, so they are Tier B as well |

**A blanket `dashboard/*.json` rule would be a defect.** Three dashboard JSON
and JS assets are genuinely functional and are read at runtime:
`dashboard/manifest.json` (linked from `index.html:12` as the PWA manifest),
`dashboard/sw.js` (registered as a service worker at `index.html:121`), and
`dashboard/help_articles.json` (`fetch`ed by `dashboard/js/help.js:45`). Any
ignore rule must name the specific inert files, never a `dashboard/*.json`
glob.

### 4.4 Deliberately NOT excluded — Tier B gating paths

These look like test-only or tooling churn. Excluding any of them would let a
change that can break a release skip CI entirely, so they must keep
triggering. (`docs/BUILD_PATHS.md`'s example list made exactly this mistake by
ignoring `**_test.go`, `firmware/test/**`, `test/**` and `tests/**` wholesale.)

| Paths | Gate that would be skipped |
|---|---|
| `.golangci.yml` | `golangci-lint` — fails the workflow before `firmware-build`/`docker-build` |
| `firmware/test/**` (13: `Makefile`, 8 `test_*.c`, `test_runner.c/.h`, results `.md`) | `firmware-test` — host gcc harness, `make -C firmware/test test` |
| `dashboard/tests/**`, `dashboard/playwright.config.js`, `dashboard/package.json`, `dashboard/package-lock.json` | `a11y-test` — Playwright + axe-core |
| `dashboard/jest.config.js`, `dashboard/js/*.test.js` | Dashboard unit tests (jest) — not a `spaxel-build` step, but gates local verification |
| `mothership/test/**` (23), `mothership/tests/e2e/**` (5) | `acceptance-test` / `go-test` legs |
| `scripts/**` (11) | Exercised by the acceptance/e2e paths; not an image build input, but not inert either |
| `docker-compose.yml` | Tier C local redeploy — rare and cheap; modelling "deploy-only" in the filter buys nothing and risks a stale local path |

---

## 5. Ready-to-implement filter

### 5.1 The exclusion predicate

Written in the exact shape of the live `ignored_path()` so it can replace it
in `k8s/iad-ci/argo-events/spaxel-sensor.yml` without touching the surrounding
fail-open logic:

```lua
local function ignored_path(path)
  -- already live (unchanged)
  if string.sub(path, 1, 5) == "docs/"    then return true end
  if string.sub(path, 1, 7) == ".beads/"  then return true end
  if string.sub(path, 1, 7) == ".needle"  then return true end
  if string.match(path, "%.md$")          then return true end
  if path == "LICENSE"                    then return true end
  if path == ".gitignore"                 then return true end
  -- new in this spec: inert paths that previously triggered builds
  if string.sub(path, 1, 5) == "data/"    then return true end
  if string.sub(path, 1, 10) == "testdata/" then return true end
  if string.sub(path, 1, 10) == ".marathon/" then return true end
  if path == ".gitattributes"                       then return true end
  if path == "acceptance-test-hang-workflow.yml"    then return true end
  if path == "firmware/docs/bluedroid-baseline.txt" then return true end
  return false
end
```

Deliberately **absent** from that function: `dashboard/**` JSON/JS reports and
dev configs (§4.3 — embed-coupled), every Tier B path (§4.4), and
`docker-compose.yml` (§4.4).

### 5.2 The trigger side (no change required)

The sensor needs no positive path list: anything not ignored triggers. For a
`git diff`-based caller that wants the trigger set expressed explicitly, the
corrected form is:

```bash
# Firmware build trigger
git diff --name-only HEAD~1 HEAD | grep -qE '^firmware/(main/|CMakeLists\.txt|sdkconfig\.(defaults|usbjtag|uart-console)|partitions\.csv|dependencies\.lock|scripts/)'

# Mothership + build-config trigger
git diff --name-only HEAD~1 HEAD | grep -qE '^(mothership/|dashboard/|Dockerfile$|\.dockerignore$|docker-compose\.yml$|VERSION$|go\.work$|go\.work\.sum$|\.golangci\.yml$)'
```

Note the `$` anchors on the single files. The example in
`docs/build-paths-catalog.md:367` —
`'^(mothership|cmd/sim|dashboard|go\.work|VERSION|Dockerfile)/'` — never
matches `VERSION`, `Dockerfile` or `go.work`, because the trailing `/` requires
them to be directories. Its doc-only skip pattern `'^(\.md|docs/|notes/|README)'`
likewise anchors `.md` at the start of the path, so it matches no tracked file;
the correct form is `\.md$`.

### 5.3 Follow-up that would simplify this spec

Narrowing `//go:embed dashboard` to the app-shipped subset
(`dashboard/*.html`, `dashboard/js/**`, `dashboard/css/**`,
`dashboard/static/**`) would shrink the binary, make `dashboard/**`
classification honest, and convert §4.3 from "do not add yet" into ordinary
§4.2 additions. Separately, extending `.dockerignore` to `data/`, `memory/`,
`testdata/`, `.beads/` and `notes/` removes dead bytes from every context
upload without touching image content — none of them is `COPY`ed.

---

## 6. Corrections to the prior path documents

Recorded here rather than edited in place; the earlier docs are dated
snapshots.

1. **`docs/build-paths-catalog.md` §"Go Workspace" and §4 are stale.** It
   describes a three-module workspace (`mothership`, `cmd/sim`,
   `test/acceptance`) and a root `cmd/sim/{main.go,go.mod}`. Since `d1f6ecba`
   the workspace is `use ./mothership` only, the shipped simulator is
   `mothership/cmd/sim/`, and root `test/acceptance/` + `tests/e2e/run.sh` are
   gone. Its "Firmware Tests" note that they "run during CI image build" is
   also wrong — they run in the separate `firmware-test` CI step.
2. **`docs/build-paths-catalog.md` §"Implementation Notes" omits `.dockerignore`**
   and the firmware build-config layer (`sdkconfig.usbjtag`,
   `sdkconfig.uart-console`, `dependencies.lock`, `main/idf_component.yml`,
   `main/version.h.in`, `main/CMakeLists.txt`), and `.golangci.yml` — the lint
   gate that blocks the build legs from ever running.
3. **`docs/build-paths-catalog.md`'s `firmware/main/` file list is stale**:
   it omits `safe_mode.c` and `watchdog.c`, and lists no `Kconfig` (correct
   today — `firmware/main/Kconfig.projbuild` is not tracked at `origin/main`).
4. **Both 2026-08-27 catalogs leave the Rust/Cargo acceptance criterion
   unanswered.** Answer: zero Rust in the repository.
5. **`docs/BUILD_PATHS.md` would break gating** by ignoring `**_test.go`,
   `firmware/test/**`, `test/**`, `tests/**` (§4.4), and its note that
   `dashboard/test-results/` "does NOT trigger builds" was wrong at the embed
   layer (that file has since been untracked).
6. **`docs/research/build-config-file-inventory.md` §6 overstates the gap.**
   It lists `memory/**` (1 file) and `notes/**` (55 files) as paths that
   "currently DO trigger a build" — all 56 are `.md`, so the live `%.md$`
   any-depth rule already drops them. It also omits the largest real inert
   group, `data/` (17 tracked runtime DBs), and
   `firmware/docs/bluedroid-baseline.txt`, though its own §5.5 names `data/`
   as not build-affecting. Its `.marathon/**` row is right but incomplete:
   2 of the 3 files escape the `*.md` rule, not all 3.
7. **`docs/research/build-config-file-inventory.md` §8.12's tracked-root
   listing is now stale**: `VERSION` is `0.2.167`, not `0.2.166`.
8. **`docs/research/go-backend-code-directories.md` §1/§3/§6 predate the
   consolidation.** Its three-module table, its root `test/acceptance/` §3,
   and its root-vs-mothership simulator comparison §6 describe the tree as it
   was on 2026-09-03. §2.1's `mothership/cmd/_parse_check.go` row is also
   stale — that file is untracked. Its `internal/` package inventory (56
   packages) and its package-level responsibilities remain accurate.

---

## 7. Verification commands

```bash
# Reproduce the trigger/exclude split against whatever HEAD is current
git ls-tree -r HEAD --name-only | python3 -c "
import sys
def ignored(p):
    return (p.startswith('docs/') or p.startswith('.beads/')
            or p.startswith('.needle') or p.endswith('.md')
            or p in ('LICENSE','.gitignore'))
t=[p for l in sys.stdin if (p:=l.strip()) and not ignored(p)]
print('would trigger:', len(t))
from collections import Counter
for k,v in Counter(p.split('/')[0] for p in t).most_common(): print(f'{v:5d}  {k}')"

# Firmware build-config layer
git ls-tree -r HEAD --name-only -- firmware | grep -vE '\.(c|h)$'

# The embed surface (why dashboard/** is Tier A)
git grep -n 'go:embed' HEAD -- mothership

# Single-module workspace
cat go.work

# Runtime-functional dashboard assets a JSON glob would break
git grep -n 'manifest.json\|help_articles\|sw.js' HEAD -- dashboard/index.html dashboard/js/help.js

# The live filter this spec targets
#   jedarden/declarative-config:k8s/iad-ci/argo-events/spaxel-sensor.yml
# (Lua `ignored_path`; last changed 2026-08-08)

# Rust (expect zero hits)
git ls-tree -r HEAD --name-only | grep -iE 'cargo|\.rs$'
```
