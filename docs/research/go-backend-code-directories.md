# Go Backend Code Directories — Mothership Survey

**Date:** 2026-09-03
**Bead:** spaxel-f99f4f0f
**Method:** structural survey using `go.mod` files, `package` declarations, `//go:build` tags,
and the entrypoint's own import list as markers. Every count below was produced by `find`/`grep`
over the working tree, and the mothership module was verified with `go build ./...` and
`go vet ./...` (both clean) before this document was written.

Companion to `esp32-firmware-code-directories.md` (firmware half of the same survey series).

---

## TL;DR

- **381 `.go` files** in the repository across **3 Go modules**, stitched by a root `go.work`.
- **All Go backend code lives under `mothership/`** (365 files, 56 packages under `internal/`).
  The mothership module alone contains **two** `package main` entrypoints: the mothership binary
  and a CSI simulator.
- The Docker image builds **both** binaries from the **mothership module**, not from the
  repo-root `cmd/sim` module that `go.work` also tracks (see §6 — the root module is a
  second, divergent simulator).
- plan.md's "Go Module Layout" section names several directories that **do not exist**
  (`internal/pipeline/*`, `internal/portal`, `internal/anomaly`, `internal/flow`, `cmd/sim`
  as the shipped simulator) and misses the ones that do (`internal/signal`, `internal/zones`,
  `internal/learning`, `internal/analytics`). Treat plan.md §Go Module Layout as design intent,
  not a map of the tree. §7 lists each divergence.
- Three package pairs are near-duplicates (`tracker`/`tracking`, `recorder`/`recording`,
  `notify`/`notifications`); one member of each pair is imported by nothing outside itself.

---

## 1. Workspace layout

```
go.work  (repo root)
  go 1.25.0
  use ./mothership
  use ./cmd/sim
  use ./test/acceptance
```

There is **no root `go.mod`** — the repository root is not itself a module. `go.work.sum`
sits alongside it (tracked in git).

| Module path (`go.mod`) | Directory | Go files | Role |
|---|---|---|---|
| `github.com/spaxel/mothership` | `mothership/` | 365 | The Go backend: HTTP+WebSocket server, signal pipeline, localization, fleet management, OTA, REST API |
| `github.com/spaxel/sim` | `cmd/sim/` | 1 | Standalone CSI simulator CLI (see §6 — not the one Docker ships) |
| `github.com/spaxel/acceptance` | `test/acceptance/` | 10 | Cross-module acceptance tests (AS-1…AS-7, integration); has a `replace` directive pointing at `../../mothership` |

Module dependency shape: `cmd/sim` depends on nothing but `gorilla/websocket` (self-contained,
no `replace` — it **cannot** import `mothership/internal/*` across the module boundary).
`test/acceptance` reaches into the mothership via its `replace` directive.
`mothership` is the only module with internal packages.

Go toolchain: `go 1.25.0` declared in all three modules and in `go.work`.

---

## 2. `mothership/` — the backend module

```
mothership/
  go.mod                module github.com/spaxel/mothership, go 1.25.0
  cmd/                  package main entrypoints
  internal/             56 packages — all application code
  test/                 in-module test files (pkg test)
  test/acceptance/      in-module acceptance tests (pkg acceptance)
  tests/e2e/            IO-gate e2e tests (pkg e2e)
```

### 2.1 Entrypoints (`cmd/`)

| Directory | Files | Package | Binary | Notes |
|---|---|---|---|---|
| `mothership/cmd/mothership/` | 9 | `main` | `spaxel` | **Primary entrypoint.** `main.go` is a **6,020-line / 108-function** monolith that performs all subsystem construction and wiring. `dashboard_embed.go` (build tag `embed`) supplies the `go:embed` dashboard filesystem; `migrate.go` (tag `ignore_migrate`) is a standalone DB migration helper; `mdns_binding.go` carries the mDNS interface selection logic |
| `mothership/cmd/sim/` | 6 | `main` | `spaxel-sim` | CSI simulator CLI. **This is the binary the Docker image ships.** 6 files (`main.go`, `generator.go`, `scenario.go`, `walker.go`, `verify.go` + test) and it **imports `internal/simulator` and `internal/ble`**, unlike the root module's sim |
| `mothership/cmd/` | 1 | `main` | — | `_parse_check.go` only — a throwaway `//go:build ignore` AST parse-checker. The leading `_` also makes Go tooling ignore the file entirely |

`dashboard_embed.go` is the only `embed`-tagged file in the tree. Untagged local builds fall
back to serving the repo-root `dashboard/` directory from disk; the Dockerfile copies
`dashboard/` → `cmd/mothership/dashboard/` immediately before the tagged production build.

### 2.2 `internal/` — 56 packages

Grouped by function. "Files" counts all `.go` files including `_test.go`.

**Process lifecycle & platform (11)**

| Package | Files | Responsibility |
|---|---|---|
| `config` | 2 | Env-var parsing, validation, documented defaults |
| `startup` | 2 | Phase-sequenced init with per-phase timeout enforcement |
| `shutdown` | 3 | Ordered graceful-shutdown manager + adapters |
| `health` | 2 | Health checks backing `/healthz` |
| `doctor` | 2 | Pre-flight configuration diagnostics |
| `diskspace` | 2 | `/data` free-space monitoring; disk-full degradation ladder |
| `loadshed` | 2 | Adaptive load shedding driven by fusion-loop iteration timing |
| `logging` | 3 | Structured logging to file + stdout |
| `eventbus` | 2 | In-process pub/sub event bus |
| `events` | 7 | Second, typed event bus **plus** event storage — overlaps `eventbus` |
| `types` | 1 | Shared type definitions |

**Device I/O & fleet (8)**

| Package | Files | Responsibility |
|---|---|---|
| `ingestion` | 13 | `/ws/node` WebSocket server, binary CSI frame parsing, node lifecycle |
| `fleet` | 15 | Node registry, role assignment, TX slot collision detection + re-stagger |
| `provisioning` | 2 | `/api/provision` payload generation, per-node token derivation |
| `apdetector` | 1 | Router BSSID consensus detection → virtual AP node for passive radar |
| `ota` | 7 | Firmware serving + OTA update manager (canary, quiet window) |
| `autoupdate` | 1 | Adapters bridging `AutoUpdateManager` to other subsystems |
| `ntpserver` | 2 | Minimal SNTP responder so isolated deployments still get wall-clock time |
| `github` | 2 | GitHub API client (release/Kaniko lookups) |

**Signal processing & localization (6)**

| Package | Files | Responsibility |
|---|---|---|
| `signal` | 20 | Signal processing **and** ambient confidence/link-health scoring — this is the real home of the pipeline, not `internal/pipeline/` as plan.md claims |
| `fusion` | 4 | 3D Fresnel-zone weighted multi-link localization (grid, peaks, explain) |
| `localization` | 11 | GDOP computation, ground-truth store, self-improving link weights |
| `localizer/` | 1 | `localizer/fusion` holds **only** `timing_budget_test.go` — the CI fusion-timing gate. Test-only package; `go build ./...` does not compile it, `go vet ./...` does |
| `tracking` | 4 | Biomechanical blob tracking (UKF). **This is the wired one** |
| `tracker` | 6 | Near-identical second UKF tracker (`tracker.go`, `ukf.go`, same test names). **Imported by nothing outside itself** — orphaned duplicate |

**Storage & recording (6)**

| Package | Files | Responsibility |
|---|---|---|
| `db` | 4 | SQLite open + schema migrations (`modernc.org/sqlite`, pure Go) |
| `recorder` | 4 | Per-link CSI segment recording (1-hour append-only segments, retention) |
| `recording` | 4 | Disk-backed circular CSI buffer + compression. `benchmark.go` is a `//go:build ignore` standalone benchmark tool living inside the library package |
| `replay` | 13 | Time-travel replay: `csi_replay.bin` reader/writer, seek, replay pipeline |
| `floorplan` | 2 | Floor-plan image upload, pixel→meter calibration |
| `zones` | 4 | Zones, portals, occupancy management |

**People, identity & learning (5)**

| Package | Files | Responsibility |
|---|---|---|
| `ble` | 8 | BLE device registry + blob identity matching |
| `sleep` | 14 | Overnight sleep state machine, breathing analysis, reports |
| `prediction` | 8 | Presence prediction from transition-probability models |
| `learning` | 5 | Detection accuracy metrics from user feedback |
| `analytics` | 9 | Anomaly detection/alerting + crowd-flow accumulation |

**Automation & safety (5)**

| Package | Files | Responsibility |
|---|---|---|
| `automation` | 3 | Spatial automation engine (3D trigger volumes → actions) |
| `volume` | 3 | 3D volume geometry + point-in-volume tests |
| `falldetect` | 2 | Fall detection from blob Z-trajectory |
| `webhook` | 2 | System webhook publishing for all events |
| `diagnostics` | 3 | Link-weather diagnostics, root-cause analysis, repositioning advice |

**API, auth & UI support (13)**

| Package | Files | Responsibility |
|---|---|---|
| `api` | 48 | **The REST API.** Largest package in the module. Its doc comment ("REST API handlers for active alerts") describes one file, not the package |
| `auth` | 3 | PIN auth, sessions, install secret |
| `dashboard` | 3 | `/ws/dashboard` hub, snapshot/incremental broadcasting |
| `timeline` | 2 | Persistent timeline events, buffered writes to SQLite |
| `briefing` | 5 | Morning briefing generation |
| `explainability` | 3 | "Why is this blob here?" contribution breakdowns |
| `guidedtroubleshoot` | 4 | First-time feature tooltips + troubleshooting flows |
| `help` | 4 | Feature-discovery monitoring |
| `notify` | 5 | Push notification delivery (Ntfy/Pushover/Gotify) with thumbnails. **The wired one** |
| `notifications` | 8 | Second notification manager (batching, quiet hours, channel configs). **Imported by nothing outside itself** — orphaned duplicate |
| `render` | 2 | Floor-plan thumbnail rendering (PNG) |
| `mqtt` | 3 | MQTT client + Home Assistant auto-discovery |
| `oui` | 5 | IEEE OUI lookup. `gen_data.go` is a `//go:build ignore` generator (`//go:generate go run gen_data.go > oui_data.go`) |

**Simulation & tooling (2)**

| Package | Files | Responsibility |
|---|---|---|
| `simulator` | 20 | Synthetic CSI/walker/BLE generation used by `spaxel-sim` and tests |
| `beads` | 3 | Diagnostic-report writing (no package doc comment) |

### 2.3 Test directories inside the module

| Directory | Files | Package | Content |
|---|---|---|---|
| `mothership/test/` | 11 | `test` | Ad-hoc verification tests (WiFi-credential flow ×9, empty-password bug ×4) + notes. Not part of the module's `internal/` surface |
| `mothership/test/acceptance/` | 11 | `acceptance` | In-module acceptance tests, one per scenario: AS-1…AS-7, IO install/upgrade, shared `test_helpers.go` |
| `mothership/tests/e2e/` | 4 | `e2e` | `io6_gate_test.go` + `io6_gate_conclusion_test.go`, both behind `//go:build io6_gate`; `assertions_test.go`; `e2e_test.go` |

`mothership/test/test.test` is an untracked compiled test binary (build artifact, not source).

---

## 3. Root `test/acceptance/` — module 3

`package acceptance`. Ten files: `acceptance_test.go`, one per acceptance scenario
(`as1_setup` … `as7_auth_reject`), `integration_test.go`, `diagnostics.go`, plus two markdown
notes and a shell wrapper (`run_with_diagnostics.sh`).

Its doc comment states the contract: tests drive a built `spaxel` binary plus a `spaxel-sim`
binary on `PATH`, so it composes the two binaries rather than importing the mothership in-process
(the `replace` directive makes the module resolvable for types it does reference).

---

## 4. Go files outside any module

These sit in directories with no `go.mod`, so no Go tooling in the workspace builds them:

| File | Purpose |
|---|---|
| `docs/gdop-usage-example.go` | GDOP usage example prose |
| `docs/gdop-usage-example-enhanced.go` | ditto, extended |
| `docs/examples/gdop_usage_examples.go` | ditto, third copy |
| `testdata/generate_csi_recording.go` | Generates CSI recording fixtures |
| `testdata/verify_recording.go` | Verifies those fixtures |

All five are utility/example programs, not part of any build. They are invisible to
`go build ./...` and `go vet ./...`.

---

## 5. `//go:build` tags as structural markers

Complete inventory (7 tagged files):

| Tag | Files | Effect |
|---|---|---|
| `embed` | `cmd/mothership/dashboard_embed.go` | Embeds the dashboard into the production binary |
| `ignore` ×3 | `internal/oui/gen_data.go`, `internal/recording/benchmark.go`, `cmd/_parse_check.go` | Standalone tools; excluded from normal builds. **This is why `internal/oui` and `internal/recording` report two package names** (`main` + library) — the `main` files are generators/benchmarks, not build participants |
| `io6_gate` ×2 | `tests/e2e/io6_gate_test.go`, `io6_gate_conclusion_test.go` | IO-6 hard-gate e2e tests, run only when the tag is supplied |
| `ignore_migrate` | `cmd/mothership/migrate.go` | Standalone DB migration helper |

When grepping for `package main` to find entrypoints, **filter on build tags** — the raw count
(8 directories) overstates the real entrypoint count (2).

---

## 6. Two simulators, and the one that ships is not the one plan.md documents

| | `cmd/sim/` (root module) | `mothership/cmd/sim/` (mothership module) |
|---|---|---|
| Module | `github.com/spaxel/sim` | `github.com/spaxel/mothership` |
| Files | 1 (`main.go`) | 6 (`main.go`, `generator.go`, `scenario.go`, `walker.go`, `verify.go`, test) |
| Imports | `gorilla/websocket` only — fully self-contained | `gorilla/websocket` **+ `internal/simulator` + `internal/ble`** |
| In `go.work` | yes | yes (as part of mothership) |
| Built by Dockerfile | **no** | **yes** → `/spaxel-sim` in the image |
| Documented by plan.md as "Module 2: the CSI simulator CLI" | yes | not mentioned |

Both carry the same doc-comment preamble ("Command … is a CSI simulator CLI for testing Spaxel
without hardware"), so they read as the same tool. They are separate implementations.
`cmd/sim/sim` also exists in the working tree as an untracked build artifact.

Consequence for anyone writing a bead against "the simulator": the Docker image runs the
**mothership-module** one, so behaviour changes must land in `mothership/cmd/sim/` +
`mothership/internal/simulator/`, not in the root `cmd/sim/`.

---

## 7. Divergences from plan.md §"Go Module Layout"

plan.md describes the target layout; the tree has drifted. Each row verified against the
working tree on 2026-09-03.

| plan.md says | Actually in the tree |
|---|---|
| `internal/pipeline/phase`, `.../nbvi`, `.../feature`, `.../baseline` | No `internal/pipeline/` at all. The pipeline lives in **`internal/signal/`** (20 files) |
| `internal/localizer/{fresnel,ukf,gdop,fusion}` | `internal/localizer/` contains only `fusion/` with a single **test-only** file. Real code is in `internal/fusion/` (Fresnel grid) and `internal/localization/` (GDOP) and `internal/tracking/` (UKF) |
| `internal/portal` | Does not exist. Portals/occupancy are in **`internal/zones/`** |
| `internal/anomaly` | Does not exist. Anomaly detection is in **`internal/analytics/`** |
| `internal/flow` | Does not exist. Crowd flow is in **`internal/analytics/`** |
| `internal/predict` | Is **`internal/prediction/`** |
| `cmd/sim` as the shipped simulator module | The Docker image builds **`mothership/cmd/sim`**; the root module is a second, divergent implementation (§6) |
| "no `test/integration/` directory" | Correct — and `mothership/test/` + `mothership/test/acceptance/` + `mothership/tests/e2e/` (three in-module test dirs) are also unmentioned |
| `internal/auth`, `internal/db`, `internal/config`, `internal/oui`, `internal/ble`, `internal/mqtt`, `internal/replay`, `internal/notify`, `internal/sleep`, `internal/ingestion`, `internal/fleet`, `internal/ota`, `internal/notifications`(≈) | Present, as named |

Unmentioned in plan.md but present: `analytics`, `apdetector`, `automation`, `autoupdate`,
`beads`, `briefing`, `dashboard`, `diagnostics`, `diskspace`, `doctor`, `eventbus`, `events`,
`explainability`, `falldetect`, `floorplan`, `github`, `guidedtroubleshoot`, `health`, `help`,
`learning`, `loadshed`, `logging`, `ntpserver`, `provisioning`, `recorder`, `render`,
`shutdown`, `simulator`, `startup`, `timeline`, `tracker`, `tracking`, `types`, `volume`,
`webhook`, `zones`.

---

## 8. Duplicate / orphaned packages

Three pairs look interchangeable. The entrypoint (`cmd/mothership/main.go`) imports exactly one
of each pair; the other compiles but has **zero importers outside its own directory**:

| Wired (imported by `main.go`) | Orphan (no external importers) | Overlap |
|---|---|---|
| `internal/tracking` | `internal/tracker` | Both ship `tracker.go`, `ukf.go`, `tracker_test.go`, `identity_fields_test.go` |
| `internal/recording` + `internal/recorder` | — (both wired; they are complementary, not duplicates: circular buffer vs. segmented per-link recorder) | Shared domain, different mechanism |
| `internal/notify` | `internal/notifications` | Two notification managers with disjoint feature sets (delivery vs. batching/channels) |
| `internal/eventbus` + `internal/events` | — (both wired) | `events` is a typed bus *and* event storage; `eventbus` is a bare pub/sub. Overlapping responsibility, not byte-identical |
| `internal/fusion` | `internal/localizer/fusion` (test-only) | Name collision only; the latter is the timing-budget CI gate |

`go build ./...` and `go vet ./...` both pass with the orphans present — they are dead weight,
not breakage.

---

## 9. Verification performed

```bash
# module inventory
find . -name go.mod -not -path './.git/*'            # → 3 modules
cat go.work                                          # → use mothership, cmd/sim, test/acceptance

# package inventory
grep -h '^package ' mothership/internal/*/*.go | sort -u

# build + vet the backend module (run from mothership/)
go build ./...    # exit 0
go vet ./...      # exit 0
```

`go test ./...` was **not** run as part of this survey bead: the deliverable is documentation
only, no Go code was modified, and the acceptance suites are known-red on this host for
environmental reasons (port 8080 collision with telegram-relay; see repo memory). The structural
claims above rest on `go build`/`go vet` plus direct file inspection.

---

## 10. Pointers for the next bead

- Largest packages, by file count: `internal/api` (48), `internal/signal` (20),
  `internal/simulator` (20), `internal/sleep` (14), `internal/replay` (13),
  `internal/ingestion` (13), `internal/fleet` (15).
- The whole application wiring is in `mothership/cmd/mothership/main.go` (6,020 lines,
  108 top-level functions). Subsystem constructors are greppable there:
  `ingestion.NewServer`, `fleet.NewManager`, `fusion.NewEngine`, `ota.NewManager`,
  `apdetector.NewDetector` + `ingestSrv.SetAPDetector` (ADR-003 wiring, now present),
  `shutdown.NewManager`, `recorder.NewManager`, `zones.NewManager`.
- Dependency direction is strictly one-way: `cmd/*` → `internal/*`; `internal/*` packages do
  not import `cmd/`. `internal/simulator` is consumed by the in-module sim and by tests.
- Module boundary gotcha: root `cmd/sim` has no `replace`, so it cannot import
  `mothership/internal/...` — only `mothership/cmd/sim` can reach the internal simulator.
