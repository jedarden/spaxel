# Mothership Directory Structure — Tree View and Organization Map

**Date:** 2026-09-03
**Bead:** spaxel-82d167d0
**Method:** every tree, count and cross-reference below was generated live over the working tree
with `find`/`ls`/`grep` and a small Python pass over all 9 HTML entry points. Reference
relationships (which page loads which script, which file references which asset) were resolved by
grepping the actual sources, not inferred from names — which is where the findings in §5 come
from. `go build ./...` and `go vet ./...` on the mothership module were both clean (exit 0) at the
time of writing.

Companions in this survey series: `go-backend-code-directories.md` (Go package-level detail),
`esp32-firmware-code-directories.md` (firmware half). The repo-root catalogs this document
once leant on for the top level (`top_level_directories.md`, `DIRECTORY_STRUCTURE.md`) were
removed by the 2026-09-04 root exhaust sweep; the surviving top-level maps are
`docs/inventory/repository-directory-structure.md` and `docs/repo-structure.md`. This document
adds the tree view and the dashboard's internal organization.

---

## TL;DR

- The repository is **three Go modules stitched by a root `go.work`** (`mothership/`,
  `cmd/sim/`, `test/acceptance/`) plus two large non-Go trees (`dashboard/`, `firmware/`).
  There is no root `go.mod`.
- The **mothership module** holds all backend code: `cmd/mothership/` (the binary),
  `cmd/sim/` (the simulator Docker actually ships), **56 packages under `internal/`**, and two
  test trees (`test/acceptance/` = AS-1…AS-7, `tests/e2e/` = IO-6 gate).
- The **dashboard is a flat, no-build vanilla-JS app**: 9 HTML entry points each hand-pick
  plain `<script src>` tags out of one flat `js/` directory (85 files) and one flat `css/`
  directory (27 files). There are **no `components/`, `utils/`, or `src/` directories** — the
  web-app conventions the task's acceptance criteria name do not exist here; the equivalents are
  `js/` (logic modules), `css/` (styles), `static/` (served-as-is assets), `tests/`
  (Playwright a11y) and `types/` (ambient TS). §4 maps the criteria onto reality.
- Tests are **co-located** in `js/` (23 jest files, `*.test.js`) rather than mirrored under
  `tests/`; `tests/` holds only Playwright accessibility specs.
- Three things look like organization but are not load-bearing: `dashboard/static/{css,js}` is
  mostly an unreferenced legacy mirror (only `static/icons/` is used — by `manifest.json`), six
  app modules in `js/` are referenced by nothing, and two Python helper scripts sit inside
  `css/`. §5 itemizes.

---

## 1. Repository top level

```
spaxel/
├── go.work                  # workspace: use ./mothership, ./cmd/sim, ./test/acceptance
├── go.work.sum
├── VERSION                  # release version, single source of truth (CI auto-bumps)
├── Dockerfile               # 3-stage: ESP-IDF firmware → Go build → distroless runtime
├── docker-compose.yml       # single-service deployment manifest
├── .golangci.yml            # lint config (v2 — see go-backend survey §lint)
├── .dockerignore  .gitattributes  .gitignore  .needle.yaml  LICENSE
│
├── mothership/              # ── Go module 1: the backend (365 .go files) — see §2
├── cmd/sim/                 # ── Go module 2: standalone simulator CLI (1 .go file)
├── test/acceptance/         # ── Go module 3: cross-module acceptance tests (10 .go files)
│
├── dashboard/               # web UI, embedded into the binary via go:embed — see §3
├── firmware/                # ESP32-S3 ESP-IDF project (C) — see esp32-firmware-code-directories.md
├── docs/                    # plan/plan.md, notes/, research/, design/, deployment/, inventory/
├── data/                    # runtime data dir (SQLite, floor plans, CSI replay, firmware uploads)
├── tests/e2e/run.sh         # shell end-to-end harness (not a Go module)
├── scripts/                 # utility scripts
├── notes/                   # supplemental project notes
├── memory/                  # tracked agent memory notes
├── testdata/                # test fixtures
├── tmp/                     # scratch (gitignored)
├── .marathon/               # test-runner configuration
├── .beads/                  # bead-rs workspace (checkpoint/ is git-tracked)
├── .claude/                 # local agent settings (untracked)
└── ~                        # tilde-named untracked scratch directory (never committed)
```

Root also carried ~30 tracked markdown reports from prior investigation sessions
(`top_level_directories.md`, `DIRECTORY_STRUCTURE.md`, `WORKSPACE_STRUCTURE_DOCUMENTATION.md`,
`SOURCE_CODE_INVENTORY.md`, the root `SYSTEM_CATALOG.md`, `MOTHERSHIP_DASHBOARD_LOCATIONS.md`, …)
alongside search-output scratch (`mothership-files.txt`, `refs-results.txt`, `*-results.txt`).
The 2026-09-04 root exhaust sweep deleted the scratch and the raw dumps and collapsed the
duplicated structure catalogs to one copy per topic under `docs/` — `WORKSPACE_STRUCTURE.md`
and `rust-source-inventory.md` remain at the root as the reconciled keepers, with `README.md`
and `PROGRESS.md` there by design. The durable documentation lives under `docs/`; the
root-level set is a work-log, not a map.

---

## 2. `mothership/` — the Go backend

```
mothership/
├── go.mod                   # module github.com/spaxel/mothership (go 1.25.0)
├── go.sum
├── cmd/
│   ├── mothership/          # THE BINARY — 9 files
│   │   ├── main.go          # entrypoint: startup phases, subsystem wiring, routes
│   │   ├── dashboard_embed.go     # //go:build embed — go:embed of the dashboard
│   │   ├── migrate.go       # schema migration registration
│   │   ├── mdns_binding.go  # mDNS advertisement
│   │   └── *_test.go        # main, dashboard_static, firmware, mdns_binding, splitlink tests
│   └── sim/                 # the simulator the Docker image ships (NOT the root cmd/sim module)
│       ├── main.go  generator.go  scenario.go  walker.go  verify.go
│       └── Makefile  README.md
├── internal/                # 56 packages — full package map in go-backend-code-directories.md
│   ├── api/                 # 48 files — REST + WebSocket handlers (largest package)
│   ├── simulator/           # 20 files — pre-deployment simulation engine
│   ├── signal/              # 20 files — phase sanitisation, NBVI, features, baseline
│   ├── fleet/               # 15 files — node registry, role assignment, stagger
│   ├── sleep/               # 14 files
│   ├── replay/              # 13 files — csi_replay.bin + replay pipeline
│   ├── ingestion/           # 13 files — /ws/node WebSocket server, frame parsing
│   ├── localization/        # 11 files
│   ├── … plus 48 more: analytics, apdetector, auth, automation, autoupdate, beads, ble,
│   │      briefing, config, dashboard, db, diagnostics, diskspace, doctor, eventbus, events,
│   │      explainability, falldetect, floorplan, fusion, github, guidedtroubleshoot, health,
│   │      help, learning, loadshed, localizer, logging, mqtt, notifications, notify,
│   │      ntpserver, ota, oui, prediction, provisioning, recorder, recording, render,
│   │      shutdown, simulator, timeline, tracker, tracking, types, volume, webhook, zones
├── test/acceptance/         # AS-1…AS-7 + IO install/upgrade tests (11 files) + test_helpers.go
├── tests/e2e/               # e2e_test.go, assertions_test.go, io6_gate_test.go (+ _conclusion)
├── build/                   # (untracked) local build output
└── mothership  sim  ota.test  test.test   # (untracked) stray `go build`/`go test -c` artifacts
```

Notes:
- `internal/` naming is *domain* naming, not the plan.md layout. plan.md §Go Module Layout
  describes `pipeline/{phase,nbvi,feature,baseline}`, `localizer/{fresnel,ukf,gdop,fusion}`,
  `portal`, `anomaly`, `flow` — none of which exist; the real signal-processing home is
  `internal/signal/` and the tracking home is `internal/{tracker,tracking,localization}`.
  `go-backend-code-directories.md` §7 enumerates every divergence.
- Two near-duplicate package pairs exist (`tracker`/`tracking`, `recorder`/`recording`,
  `notify`/`notifications`); one member of each is imported by nothing outside itself.
- The **embed path**: the Dockerfile copies the repo-root `dashboard/` into
  `cmd/mothership/dashboard/` (gitignored) immediately before a tagged build;
  `dashboard_embed.go` then embeds it behind the `embed` build tag. Untagged local builds fall
  back to serving the canonical tree from disk via `main.go`.

---

## 3. `dashboard/` — the web UI

```
dashboard/
├── index.html               # home — loads js/home-cards.js only
├── live.html                # expert 3D view — 52 <script> tags (largest page)
├── fleet.html               # fleet table — js/fleet-page.js
├── simple.html              # simple mode — js/simple-mode.js
├── ambient.html             # ambient/wall mode — auth + ambient_renderer + ambient_briefing + ambient
├── setup.html               # onboarding/calibration — viz3d, websocket, app, placement,
│                            #   floorplan-setup, zone-editor, …
├── simulator.html           # pre-deployment simulator — CDN Three.js + viz3d, placement, simulate
├── integrations.html        # integrations — js/integrations.js
├── test-transformcontrols.html   # manual test page for TransformControls
│
├── js/                      # 85 files, FLAT — all app logic lives here
│   ├── (56 app modules)     # app.js, viz3d.js, websocket.js, onboard.js, fleet.js,
│   │                        #   simple-mode.js, ambient*.js, placement.js, zone-editor.js,
│   │                        #   timeline.js, command-palette.js, replay.js, explainability.js, …
│   ├── (23 co-located jest tests)   # *.test.js + *.test.setup.js — run by jest, never by a page
│   ├── esptool-bundle.js    # vendored esptool-js; dynamically imported by onboard.js
│   │                        #   (`await import('/js/esptool-bundle.js')`)
│   ├── testProfiler.js  run-profiled-tests.js  profile-suite.js  profile-demo.js
│   │                        # leak-profiling harness (node CLI, not browser)
│   └── PROFILING.md         # how to run the profiling harness
│
├── css/                     # 27 stylesheets, FLAT — one per panel/feature
│   ├── tokens.css           # design tokens — linked by 8 of 9 pages
│   ├── layout.css           # shell/layout — linked by 8 pages
│   ├── panels.css           # 4 pages · scene.css, floorplan.css, ambient.css, simulator.css (2 each)
│   └── (per-feature)        # home, fleet-page, simple, briefing, timeline, explainability,
│                            #   command-palette, notifications, quick-actions, security, sleep,
│                            #   anomaly, apdetection, ble-panel, replay, troubleshooting, …
│   └── _fix_html.py  _tokenize.py   # stray Python helpers — see §5
│
├── static/                  # served verbatim under /static/
│   ├── icons/               # PWA icons (72…512 px + maskable + icon.svg) — referenced by manifest.json
│   ├── css/mobile.css       # linked by one page AND precached by sw.js
│   ├── css/fleet-page.css   # ⚠ unreferenced (css/fleet-page.css is the live copy)
│   └── js/fleet.js  js/mobile.js    # ⚠ unreferenced by any page or sw.js
│
├── tests/                   # Playwright accessibility specs ONLY
│   ├── a11y.spec.js  a11y-dashboard.spec.js  a11y-onboarding.spec.js
│   └── accessibility/       # axe-import.spec.ts, helper.{js,ts}, smoke.spec.{js,ts}
│                            #   (mixed .js and .ts in the same directory)
├── types/                   # ambient TS: spaxel.d.ts, blob-identity.check.ts
│
├── manifest.json            # PWA manifest → /static/icons/*
├── sw.js                    # service worker; precaches 34 entries (/css/*, 3 /js/*, mobile.css)
├── help_articles.json       # content for the help/guided-troubleshooting UI
│
├── package.json             # scripts: test (jest --verbose), test:a11y (playwright), typecheck (tsc)
├── jest.config.js  playwright.config.js  tsconfig.json
├── generate-icons.js        # regenerates static/icons/*
├── package-lock.json
├── node_modules/            # (gitignored) jest, playwright, axe-core, typescript, http-server
├── test-results/            # (gitignored) playwright output
└── (leak-profiling reports) # CONFIRMED_LEAK_REPORT.md, LEAK_PROFILING_ANALYSIS.md,
                             #   leak-*.json, profiling-*.md, test-profiling-results.json
```

**How pages load code.** There is no bundler and no import graph at build time. Each HTML page
declares the exact scripts it needs, in dependency order:

| Page | Loads |
|---|---|
| `index.html` | `js/home-cards.js` (1 script) |
| `live.html` | Three.js r128 + OrbitControls + TransformControls **from CDN**, then 45+ local modules (`blob-identity`, `zone-lookup`, `viz3d`, …) |
| `fleet.html` / `simple.html` / `integrations.html` | one page module each |
| `ambient.html` | 4 modules (auth → renderer → briefing → ambient) |
| `setup.html` | 7 modules (viz3d, websocket, app, placement, floorplan-setup, zone-editor, …) |
| `simulator.html` | CDN Three.js + viz3d, placement, simulate |

Three.js is a **CDN dependency at runtime** (r128, plus the non-module `examples/js/controls`
builds) for `live.html` and `simulator.html`; everything else is local. The only dynamic
`import()` in the app is `onboard.js` pulling in the vendored `esptool-bundle.js`, plus
`fxaa.js` importing Three.js postprocessing modules.

---

## 4. Key file types and where they live

The acceptance criteria name generic web-project directories (`src/`, `public/`, `components/`,
`utils/`, `styles/`). This codebase predates/avoids those conventions — the mapping is:

| Generic name | Actual location | Notes |
|---|---|---|
| `src/` (app source) | `dashboard/js/` | flat, 85 files, no subdirectories |
| `public/` (served verbatim) | `dashboard/` itself + `dashboard/static/` | the whole tree is served/embedded; `static/` is the `/static/` URL subtree |
| `components/` | **does not exist** | UI "components" are per-feature `js/` modules paired 1:1 with a `css/` file of the same name (e.g. `js/ble-panel.js` + `css/ble-panel.css`) |
| `utils/` | **does not exist** | helpers live inline in the modules that use them (`zone-lookup.js`, `fxaa.js`, `state.js` are the closest thing) |
| `styles/` | `dashboard/css/` | flat, one sheet per feature + shared `tokens.css`/`layout.css` |
| tests (unit) | **co-located in `js/*.test.js`** | jest + jsdom; 23 files, never loaded by a page |
| tests (e2e/a11y) | `dashboard/tests/` | Playwright + axe-core, mixed `.js`/`.ts` |
| types | `dashboard/types/` | ambient `.d.ts` + a `.check.ts`; `npm run typecheck` |
| entry points | `dashboard/*.html` (9) | each hand-picks its scripts |
| assets | `dashboard/static/icons/` | PWA icons only |
| Go backend source | `mothership/internal/` | 56 packages |
| Go entry point | `mothership/cmd/mothership/main.go` | |
| Firmware source | `firmware/main/*.c` | 16 `.c` + headers |
| Docs | `docs/{plan,notes,research,design,deployment,inventory,tests}` | `plan/plan.md` is the architecture document |

---

## 5. Findings — things the tree does not advertise

1. **The dashboard is flat by design, but the flatness has a cost.** With 85 files in one
   directory, module relationships are only discoverable from the HTML that loads them. The
   page→script table in §3 is the first such map committed under `docs/`.
2. **`dashboard/static/{css,js}` is a stale mirror, with one live exception.** Only
   `static/icons/` (PWA manifest) and `static/css/mobile.css` are referenced anywhere. The other
   three files — `static/css/fleet-page.css`, `static/js/fleet.js`, `static/js/mobile.js` — are
   referenced by no page and not precached by `sw.js`; the live copies are `css/fleet-page.css`
   and `js/fleet.js`. Deleting the three would require no other change. (Not done here — this
   bead is a map, not a cleanup.)
3. **Six app modules are referenced by nothing**: `js/apdetection.js`, `js/automations.js`,
   `js/controls.js`, `js/explain.js`, `js/notifications.js`, `js/tooltip.js`. Verified against
   every HTML page, every `js/` file, and `sw.js`'s precache list. Note the near-name twins that
   *are* live: `explainability.js` (vs `explain.js`) and `tooltips.js` (vs `tooltip.js`). Some of
   these look like functionality the plan describes (AP detection, notifications UI) — worth a
   dedicated bead before treating them as dead.
4. **`css/_fix_html.py` and `css/_tokenize.py`** are Python scripts living in a stylesheet
   directory — one-off codegen/fixup helpers that drifted into `css/`.
5. **Tests live in two idioms.** `js/*.test.js` (jest, co-located) vs `tests/*.spec.js`
   (Playwright, centralised) vs `js/*.test.profiling.js` (a third, node-CLI harness documented
   in `js/PROFILING.md`). The `tests/accessibility/` subdirectory mixes `.js` and `.ts` versions
   of the same specs (`smoke.spec.js` + `smoke.spec.ts`, `helper.js` + `helper.ts`).
6. **Stray build artifacts** sit untracked in `mothership/` (`mothership`, `sim`, `ota.test`,
   `test.test`) — the `.gitignore` already covers the known ones; `ota.test`/`test.test` are
   `go test -c` outputs not currently listed.
7. **plan.md's dashboard description says "HTML, JS (Three.js), CSS embedded via go:embed"** —
   accurate, but plan.md's command for it (`esbuild` bundling `esptool-js`) describes only
   `js/esptool-bundle.js`, which is in fact a **pre-built vendored bundle checked into the
   tree**, not built at image build time.
