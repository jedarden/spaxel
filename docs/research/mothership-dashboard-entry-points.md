# Mothership — Entry Points, Main Files, and Routing

Companion to [`mothership-directory-structure.md`](mothership-directory-structure.md), which maps
the tree. This document answers "where does the application actually start, and what files drive
initialization" — verified against the live code (file/line references below) and against an
empirical route probe (`httptest` routers replicating both serving modes in
`mothership/cmd/mothership/main.go`, run against the real `dashboard/` tree).

Spaxel's Mothership is a **single Go binary** (one Docker container) that serves the dashboard.
There is no Node server, no bundler, and no client build step — so "entry point" means three
distinct things here, and the acceptance-criteria examples (`server.js`, `main.js`, `app.tsx`)
map onto them:

| Concept from the AC | What it is in Spaxel |
|---|---|
| `server.js` (server entry) | `mothership/cmd/mothership/main.go` — `func main()` |
| `index.html` (page entries) | 9 static HTML pages in `dashboard/` |
| `main.js` / `app.tsx` (app init) | Per-page JS bootstrap modules in `dashboard/js/` |

---

## 1. Server entry point — the Go binary

**`mothership/cmd/mothership/main.go`** — `func main()` at line 703. This is the only process
entry point in the repository; there is no other `func main()` that serves user-facing traffic
(the simulator under `cmd/sim/` is a development tool, not an entry point of the product).

Startup sequence that matters for the dashboard:

1. Config load, then HTTP middleware (`main.go:749-753`):
   `middleware.Logger` → `middleware.Recoverer` → `auth.DemoModeMiddleware(cfg.DemoMode)`.
2. `go dashboardHub.Run()` (`main.go:4894`) starts the in-process WebSocket hub.
3. `r.HandleFunc("/ws/dashboard", dashboardSrv.HandleDashboardWS)` (`main.go:4896`) — the
   dashboard's single live data channel.
4. Page routes + static serving (below).

> **Auth note:** there is **no server-side auth middleware on any HTML page route**. The only
> middleware on those routes is logging/recovery plus `DemoModeMiddleware`. Ambient mode gates
> itself *client-side* (`dashboard/js/auth.js` → redirect to `/` if unauthenticated); session
> enforcement for data happens on `/ws/dashboard`. Anything that needs server-side page auth is
> a change to this file, not to the pages.

### Embedded vs. filesystem asset serving

`mothership/cmd/mothership/dashboard_embed.go:11-12` declares `//go:embed dashboard`. The
`embed` build tag is set by the Dockerfile, which copies repo-root `dashboard/` →
`cmd/mothership/dashboard/` before building; an untagged local build has no embedded FS and
falls back to serving from disk via `findDashboardDir()`. Two code paths, one URL space:

- **Embedded** (`main.go:4922-4936`): `fs.Sub(dashboardFS, "dashboard")` →
  `http.StripPrefix("/", http.FileServer(http.FS(...)))` on the `/*` catch-all, registered for
  **both GET and HEAD** (`main.go:4935`, the bf-1cgqe regression — a GET-only route returned
  405 to `curl -sI`).
- **Filesystem / dev** (`main.go:4937+`, handler at `dashboardStaticHandler`, line 381,
  registered by `registerDashboardStatic`, line 411, also GET+HEAD at 420): serves the file if
  it exists, serves `index.html` for a directory, and — for any path with **no extension** —
  falls back to `index.html` (an SPA-style catch-all). `TestDashboardStaticAssets` in
  `dashboard_static_test.go` pins GET+HEAD MIME behavior for both.

---

## 2. Page entry points (`dashboard/*.html`)

Nine HTML pages exist. Five have short server routes; four are reachable only by literal
filename.

| Page | URL (embedded mode) | Route defined at | Purpose |
|---|---|---|---|
| `index.html` | `/` (also `/index.html` → 301) | static catch-all | Landing / home: status banner + three cards (People & Zones, Devices & Fleet Health, Recent Events) |
| `live.html` | `/live` | `main.go:4908` | Expert 3D view — the largest page (4.5k lines): viz, timeline, panels, replay, command palette |
| `simple.html` | `/simple` | `main.go:4917` | Mobile-first card UI |
| `fleet.html` | `/fleet` | `main.go:4904` | Sensor fleet status / health table |
| `setup.html` | `/setup` | `main.go:4912` | Placement, floor-plan and zone editor |
| `ambient.html` | `/ambient` | `main.go:4899` | Headless ambient display (TV / kiosk) |
| `simulator.html` | `/simulator.html` | static catch-all | Node simulator UI — **no short route exists** |
| `integrations.html` | `/integrations.html` | static catch-all | Integrations status UI — **no short route exists** |
| `test-transformcontrols.html` | `/test-transformcontrols.html` | static catch-all | Manual Three.js TransformControls dev page (CDN only, not part of the app shell) |

### Route-probe results (empirical, both serving modes)

| Path | Embedded | Filesystem (dev) |
|---|---|---|
| `/` | 200 `index.html` | 200 `index.html` |
| `/index.html` | 301 → `./` | 301 → `./` |
| `/live` `/fleet` `/ambient` `/setup` `/simple` | 200 (page route) | 200 (page route) |
| `/simulator`, `/integrations` | **404** | **200** (extension-less → SPA fallback serves `index.html`) |
| `/simulator.html`, `/integrations.html` | 200 | 200 |
| `/nope` (any extension-less junk) | 404 | 200 `index.html` |

Two consequences worth flagging:

- `/simulator` and `/integrations` are **unrouted**. `simulator.html` and `integrations.html`
  each contain a self-referential mobile-nav link using the short form (`href="/simulator"`,
  `href="/integrations"`), which **404s in the embedded/production build** and silently lands
  the user on the home page in the dev build. Reachable working URLs are the `.html` forms.
  (Fix, if wanted: five more `r.Get` lines in `main.go` next to 4899-4919.)
- The dev-mode extension-less fallback is unconditional, so *any* misspelled short path renders
  the home page with a 200 in dev — a silent failure mode the embedded build does not have.

---

## 3. Main application files — where the app initializes

Each page owns a small bootstrap chain: page-specific `<script src>` tags in hand-maintained
dependency order, ending in one module that self-initializes on `DOMContentLoaded`. The shared
idiom (used by every bootstrap module) is:

```js
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
} else {
    init();
}
```

### `dashboard/js/app.js` — the main application file

`app.js` (2,709 lines, header: "Spaxel Dashboard - Main Application") is the closest analogue
to a `main.js` for the primary experience. `init()` at `app.js:2126` runs the whole startup:

```js
function init() {
    // parse ?maxFps= (30 cap / 60 uncapped / 0 disables the RAF loop)
    initScene();            // Three.js scene, camera, renderer, node/link meshes
    initChart();            // RSSI history chart
    connectWebSocket();     // /ws/dashboard + reconnection manager
    startHealthPolling();   // 10s /api/health poll
    startDiurnalPolling();  // 30s diurnal poll
    animate();              // RAF loop
    handleURLParameters();  // ?highlight=<MAC> → Viz3D.flyToNode, then strip the param
}
```

…registered via the `DOMContentLoaded` idiom at `app.js:2165`, and it exports the shared
facade `window.SpaxelApp` at `app.js:2183` (`getLinks`, `getNodes`, `refreshNodeList`,
`refreshLinkList`, `showToast`, `registerMessageHandler`, `getTargetFPS`,
`isStrugglingDevice`, …) that every other module consumes instead of importing.

### Per-page bootstrap owners

| Page | Bootstrap file(s) | What they initialize |
|---|---|---|
| `live.html` | **`js/app.js`** (+ `js/router.js`, `js/websocket.js`, `js/viz3d.js`, ~45 page modules) | Scene, chart, WS, polling, RAF — the app proper |
| `index.html` | `js/home-cards.js` (only script) | Connects to `/ws/dashboard`, renders the three cards from the snapshot + incremental updates; self-inits |
| `simple.html` | `js/simple-mode.js` | Card UI, 30s REST poll fallback, expert-PIN gate (`localStorage: spaxel_ui_mode`, `spaxel_expert_pin_set`); self-inits |
| `fleet.html` | `js/fleet-page.js` | Fleet table render + WS updates |
| `setup.html` | `js/placement.js`, `js/floorplan-setup.js`, `js/zone-editor.js` | Page inline script calls `Placement.init()` then `FloorPlanSetup.init()` on DOMContentLoaded |
| `ambient.html` | `js/auth.js` → `js/ambient_renderer.js` → `js/ambient_briefing.js` → `js/ambient.js` | Inline gate: `SpaxelAuth.checkStatus()` → authenticated ? `SpaxelAmbientMode.enable()` : `location.href='/'` |
| `simulator.html` | `js/simulate.js` | Inline `DOMContentLoaded` → `SpaxelSimulator.init()` |
| `integrations.html` | `js/integrations.js` | Inline `DOMContentLoaded` → `SpaxelIntegrations.fetch()` |
| `test-transformcontrols.html` | none (CDN Three.js only) | Manual test harness |

### Supporting infrastructure files (loaded, not bootstrapping)

- **`dashboard/js/websocket.js`** — WS reconnection manager used by `app.js`: backoff 1s→10s
  (±500 ms jitter), "silent" threshold 5 s (below it, blobs are extrapolated rather than
  redrawn), dimming modal at 30 s.
- **`dashboard/js/auth.js`** — client-side PIN setup/login/session overlay (`checkAuthStatus`,
  `renderOverlays`); the only auth surface on any page.
- **`dashboard/sw.js`** — service worker, `CACHE_NAME 'spaxel-dashboard-v1'`, precaches 30
  entries: `/`, `/index.html`, `/live.html`, 23 CSS files, three page-adjacent JS files
  (`home-cards.js`, `onboard.js`, `troubleshoot.js`), `static/css/mobile.css`, and
  `manifest.json` — and explicitly never caches WebSocket/REST data. Notably absent are
  `app.js`, `router.js`, `websocket.js`, `viz3d.js` and `static/js/mobile.js`, so the offline
  shell covers the home page, not the expert page.
- **`dashboard/static/js/mobile.js`** — "Mobile Expert Mode" (hamburger, bottom sheets,
  breakpoint 768 px), loaded by `live.html:4510` as an **ES module** — the only ES module
  besides `js/fxaa.js`.
- Three.js **r128 from CDN at runtime** (`live.html`, `simulator.html`), including non-module
  `examples/js` OrbitControls/TransformControls builds — no vendored JS, so the dashboard is
  not usable offline despite the service worker.

---

## 4. Routing and navigation setup

Routing exists at **two layers**, and they do not overlap.

### Layer 1 — server (chi): page identity

Five explicit page routes (`/ambient`, `/fleet`, `/live`, `/setup`, `/simple`, lines 4899-4919)
registered **before** the `/*` catch-all that serves everything else out of the embedded FS or
the dashboard directory. Root `/` resolves to `index.html` through the catch-all; `/index.html`
redirects to `./` (301, `http.ServeFile` canonicalization). There is no server-side route
rewriting, no trailing-slash normalization, and no server-side auth on any of these.

### Layer 2 — client (`dashboard/js/router.js`): modes inside `live.html`

`router.js` (217 lines) is a hash router for the *expert page's modes*, not for page
navigation. `ROUTES` at line 13:

| Route | Purpose |
|---|---|
| `live` *(default)* | 3D viz |
| `timeline` | event timeline |
| `automations` | automation rules |
| `settings` | settings panel |
| `ambient` | ambient handoff |
| `replay` | historical replay |
| `simulate` | in-page simulator |

`init()` (line 65) reads `location.hash` (defaulting to `live`), then subscribes to
`hashchange` and `popstate`. `setMode(mode, updateHash=true)` writes the fragment with
`history.replaceState({mode}, '', '#'+mode)` and notifies subscribers. Exported as
`window.SpaxelRouter` (line 199): `init`, `onModeChange`, `getMode`, `getPreviousMode`,
`navigate`, `isMode`, `getRoutes`; it self-inits on `DOMContentLoaded`.

Deep-link fragments consumers rely on: `#zones` and `#timeline` (used by `index.html`'s cards).

### Cross-page navigation

Plain `<a href>` elements, no client router and no SPA transitions:

- `index.html` header links: `/`, `/simple`, `/live`, `/fleet`; cards link to `/live#zones`,
  `/fleet`, `/live#timeline`.
- `live.html` hamburger menu (inline script at ~4021) links out to the other pages.
- Every page carries a mobile bottom-nav variant with the same hrefs.

### Deep-link parameters

| Param | Page | Handled by | Effect |
|---|---|---|---|
| `?maxFps=30\|60\|0` | `live.html` | `app.js` `init()` | FPS cap; `0` disables the RAF loop |
| `?highlight=<MAC>` | `live.html` | `handleURLParameters()` (`app.js:2105`) | `Viz3D.flyToNode(mac)`, then strips the param from the URL |
| `?reprovision=<MAC>` | `live.html` | inline script at ~4495 | Calls `SpaxelOnboard.reprove` |
| `#zones`, `#timeline` | `live.html` | `SpaxelRouter` | Selects the initial mode |

---

## 5. Findings and corrections

1. **`/simulator` and `/integrations` are unrouted** (embedded mode 404, dev mode silently
   serves `index.html`), while the pages' own mobile-nav links use those short forms — see the
   probe table above. The pages work only via `/simulator.html` / `/integrations.html`.
2. **Correction to the sibling doc:** `mothership-directory-structure.md` §5(2) states that
   `static/js/fleet.js` and `static/js/mobile.js` are "referenced by no page and not precached
   by sw.js". Both halves are wrong: `mobile.js` is loaded by `live.html:4510`
   (`<script type="module" src="static/js/mobile.js">`), and `js/fleet.js` is loaded by
   `live.html:3590` (`<script src="js/fleet.js">`). Neither is precached by `sw.js` (true part
   of the original claim), but both are referenced by a page.
3. **No server-side page auth.** Only `DemoModeMiddleware` sits in front of the page routes;
   ambient mode's PIN gate is client-side and `/ws/dashboard` is where data-level enforcement
   lives (`main.go:4896`).
4. **Two serving modes, one URL space, divergent miss behavior** — embedded 404s unknown
   extension-less paths; the dev filesystem handler renders `index.html` with a 200 for any of
   them. Anything debugging "why does this page load the home page" in dev is hitting the SPA
   fallback, not a route.
5. **Three.js comes from the CDN at runtime**, so the service worker's offline shell does not
   make the expert page work offline; the vendor files are not in the precache list.
