# Mothership Dashboard — Access Findings

**Date:** 2026-09-04
**Bead:** spaxel-a48234c7 (documentation-and-cleanup step of the dashboard-access split tree)
**Evidence chain:** instance started and checklist-verified by spaxel-fc68b47e; browser-verified with
Chromium by spaxel-68620881; every claim below statically corroborated at HEAD and re-probed against
the live instance by this bead.
**Verified against:** `main` @ `5e2f28b4` — every `file:line` below was confirmed via `git show HEAD:`,
not against the working tree (which carries another worker's in-flight edits to `onboard.js`,
`troubleshoot.js`, `main.go` and `config.go`; none of those files is cited here).
**Sibling deliverables:** `mothership-dashboard-startup-procedure.md` (how to start it),
`mothership-dashboard-startup-investigation.md` (startup internals + env surface),
`mothership-dashboard-dependencies.md` (prerequisites),
`mothership-dashboard-locations.md` (asset locations),
`docs/notes/mothership-dashboard-startup-verification-2026-09-04.md` (execution log of the running instance)

---

## 1. Dashboard URL and access method

| What | Value |
|---|---|
| Canonical URL (documented default) | `http://localhost:8080/` — bind default `0.0.0.0:8080` (`config.go:92`) |
| URL on this host (ex44) | `http://127.0.0.1:18080/` |
| Why not 8080 here | `*:8080` is held by the unrelated `telegram-relay` service (pid 2794) — see §4 row 1 |
| Why loopback | a stock start is **unauthenticated** (the dashboard PIN middleware is defined but never mounted — procedure §0 fact 3 / §7.1), and ex44 is a shared multi-user host |
| Access method | any browser, over loopback; **no login, no PIN, no token**. Realtime push via `ws://127.0.0.1:18080/ws/dashboard` (10 Hz snapshot/event frames) |
| Process health | `GET /healthz` → 200 `{"status":"ok",…}` — the only reliable liveness check (a `listening` log line can precede a failed bind) |
| UI routes | `/`, `/simple`, `/live`, `/fleet`, `/setup`, `/ambient` — all resolve |
| Running instance | pid 3219177, Path B (dev run from source), started per procedure §3; runbook in the execution note §6 |

A literal navigation to `http://localhost:8080` from the browser returns `404` `text/plain`
"page not found" — that answer comes from `telegram-relay`, not from spaxel. The port conflict is
visible from the client side, not only at bind time.

## 2. Browser verification (Chromium route sweep, 2026-09-04)

Browser: Chromium 151.0.7922.173 (nix-store build), driven headless via playwright 1.59.1 from
`dashboard/node_modules`. To replay on this host, pass `executablePath` pointing at the nix
chromium — the playwright-cached builds (`~/.cache/ms-playwright/chromium-1234`) cannot launch on
NixOS (`libglib-2.0.so.0` absent). See §4 row 3.

| Route | Result |
|---|---|
| `/` | **CLEAN.** 0 console errors, 0 warnings, 0 page errors, 0 failed or ≥400 requests. WebSocket connected with live snapshot frames. Header bar with brand + nav, green "All clear — No one detected, 0 devices online" banner, 3 panels (People & Zones / Devices & Fleet Health / Recent Events). Screenshot read back and visually confirmed |
| `/simple` | Renders. Sole console error: `/favicon.ico` 404 (cosmetic — D5) |
| `/live` | Renders (8 canvases, 3D view up). Two defects — D2, D3 — plus non-fatal warnings (§3 end) |
| `/fleet` | **CLEAN.** 0 errors |
| `/setup` | Serves 200 and renders text only. 3 uncaught ReferenceErrors — D1 |

## 3. Defects found

All five are **unresolved**; none was owned by an open bead as of 2026-09-04 (open-bead titles and
descriptions swept for `anomaly_detected`, `THREE`, `undefined`, `favicon`, `agentation`,
`sidebar` — zero hits). Recorded here so a future owner starts from the mechanism, not the symptom.

### D1 — `/setup`: 3× `ReferenceError: THREE is not defined`; the 3D floorplan setup UI never initializes

`setup.html:57` loads `js/viz3d.js` but carries **none** of the three.js tags that `live.html` has:
`live.html:3521` (`three.min.js` r128 from cdnjs), `:3523` (`OrbitControls`), `:3542`
(`viz3d.js`). `viz3d.js` dereferences `THREE` at load, so every entry point throws and the page
renders its text-only fallback.

Corroborated live by this bead: the running instance's `/setup` serves 7 local script tags
(`viz3d.js`, `websocket.js`, `app.js`, `placement.js`, `floorplan-setup.js`, `zone-editor.js`,
`portal.js`) with no CDN three.js, while `/live` serves the full set.

Fix shape: add the same three.js + OrbitControls/TransformControls tags to `setup.html` ahead of
`viz3d.js` (same pattern as `live.html:3521-3523`).

### D2 — `/live`: `WebSocket connection to 'ws://…/undefined' failed: Unexpected response code: 200`

`app.js:760` is the **only** `SpaxelWebSocket.connect()` call site in the tree and passes no
argument; `websocket.js:49` is `function connect(url)` with no default, so `websocket.js:58`
executes `new WebSocket(undefined)`, which resolves to the literal path `/undefined`. Introduced by
`ff3428fe` (2026-04-07, "robust WebSocket reconnection").

Why the browser reports code **200**: unknown paths are answered by the SPA fallback with
`index.html` (`GET /undefined` → 200, `text/html` — reproduced with curl), so the upgrade attempt
gets a 200 instead of a 101.

Why `/` (home) is unaffected: home never calls `SpaxelWebSocket` — `home-cards.js` builds its own
socket with an explicit URL (`new WebSocket(...)` at `home-cards.js:261`, `ws(s)://location.host + '/ws/dashboard'`).

Fix shape: default the argument in `websocket.js` `connect()` to
`ws(s)://location.host + '/ws/dashboard'`, or pass the URL explicitly at `app.js:760`.

### D3 — `/live`: sidebar timeline loads zero events — server 400 on the events fetch (mechanism corrected vs the earlier record)

Observed in the browser run: console
`[SidebarTimeline] Failed to load events: Error: Failed to fetch events: Bad Request` at
`js/sidebar-timeline.js:559`, from an HTTP 400 on `GET /api/events?...`.

Mechanism: with the default filter state (`categories: { presence: true, zones: true, alerts: true, … }`,
`sidebar-timeline.js:171`), `buildServerParams` (`:518-530`) sends the alerts category's type list,
which contains **`anomaly_detected`** (`sidebar-timeline.js:153`). The server's `isValidEventType`
(`mothership/internal/api/events.go:180-188`) knows `anomaly` and `AnomalyDetected` but **not**
`anomaly_detected`, and one unknown type rejects the whole list (`events.go:299`), so the initial
load 400s and the timeline renders nothing.

Reproduced by this bead against the running instance, with the exact default type list:

```
GET /api/events?limit=100&types=presence_transition,stationary_detected,detection,sleep_session_end,
    zone_entry,zone_exit,portal_crossing,anomaly,anomaly_detected,security_alert,fall_alert
→ HTTP 400 {"error":"invalid event type: anomaly_detected"}
```

**Correction of the earlier record:** the spaxel-68620881 note quotes the rejected type as
`sleep_session_` (trailing underscore). That type never existed in any revision:
`git log -S "sleep_session_'" --all -- dashboard/js/sidebar-timeline.js` is empty (the only pickaxe
hits are the substring inside `sleep_session_end`, which the server accepts — reproduced 200), and
the served file at `:151` produces `sleep_session_end`. The quoted URL and the quoted error echo in
that note were truncated in transcription. The console error itself was real; the malformed-type
reading of it was not. The rejected type is `anomaly_detected`.

Fix shape: align the namespace — either accept `anomaly_detected` in the server's valid set, or have
the client request a spelling the server already accepts (`anomaly` / `AnomalyDetected`).

### Non-fatal `/live` warnings (observed, not addressed here)

- WebGL `GPU stall due to ReadPixels` ×4 — headless software GL in the verification environment, not a product defect.
- `[AutomationBuilder] #automations-panel not found`
- `[Simulate] Viz3D not ready yet`
- Horizontal overflow on `/live`: document `scrollWidth` 2162 vs viewport 1440 in the verification run (browser-measured, not reproduced statically).

### D5 — `/favicon.ico` 404 (cosmetic)

No favicon file is tracked under `dashboard/` and no `<link rel="icon">` exists in any HTML, so
browsers fall back to requesting `/favicon.ico`, which the server answers `404 text/plain`
(reproduced with curl). Surfaces as the sole console error on `/simple`.

### D6 — agentation is not wired into any dashboard page (workspace-standard gap)

`git grep agentation -- 'dashboard/*.html'` is empty, and the browser run confirmed
`#agentation-root` absent on all five routes. The workspace standard is that **every** page carries
the agentation toolbar, with the import map placed before the module that needs it — without the
map the module dies on an unresolvable bare specifier while the page still renders, which reads as
wired when it isn't. Recorded as a gap; not fixed in this documentation bead.

## 4. Errors encountered during access, and resolutions

| # | Error | Resolution |
|---|---|---|
| 1 | Stock start dies with `[FATAL] HTTP server error: listen tcp 0.0.0.0:8080: bind: address already in use` | `*:8080` is owned by the unrelated `telegram-relay` (pid 2794, up 3d+). Resolved with the procedure §8 issue 1's own remedy: `SPAXEL_BIND_ADDR=127.0.0.1:18080` |
| 2 | `http://localhost:8080` in the browser → `404` `text/plain` | That answer is telegram-relay's, not spaxel's; use `http://127.0.0.1:18080/` on this host |
| 3 | playwright-cached Chromium cannot launch on this NixOS host (`libglib-2.0.so.0` absent) | Pass `executablePath` pointing at the nix-store chromium build |
| 4 | `[WARN] Health check failed: … dial tcp 127.0.0.1:18080: connect: connection refused (continuing anyway)` at boot | Benign and structural: the phase-7 startup self-probe logs one line *before* `HTTP server listening`, so it races the bind and loses; `/healthz` is 200 immediately after. Expect it on every healthy boot |
| 5 | Dev run cannot open `/data` as non-root | `SPAXEL_DATA_DIR` to a writable path (procedure §3 step 2) |

## 5. Configuration changes made

**None to the repository.** The access work changed no tracked config file, no manifest, no compose
file, and no code. The verification instance's runtime configuration is three environment variables
on the process — `SPAXEL_DATA_DIR` (git-ignored `tmp/spaxel-devdata/`), `SPAXEL_BIND_ADDR=127.0.0.1:18080`,
`SPAXEL_MDNS_ENABLED=false` — each with its rationale tabulated in the execution note §2. They are
not persisted anywhere; deleting `tmp/spaxel-devdata/` and restarting reproduces the state from
scratch. The instance's reported version is `dev` (Path B builds pass no `-ldflags -X`).

## 6. Cross-references

- Operating procedure — paths A/B/C, healthy-boot log, verification checklist, 12-row troubleshooting: `mothership-dashboard-startup-procedure.md`
- Execution log and runbook of the running instance: `docs/notes/mothership-dashboard-startup-verification-2026-09-04.md`
- Dependency inventory and the full 28-variable env table: `mothership-dashboard-dependencies.md`
- Startup sequence internals and serving modes: `mothership-dashboard-startup-investigation.md`
- Asset locations and the embed mechanism: `mothership-dashboard-locations.md`
- Bead-level provenance: spaxel-fc68b47e (instance), spaxel-68620881 (Chromium run), spaxel-a48234c7 (this document)

## 7. Acceptance-criteria mapping

| AC | Status |
|---|---|
| Document the startup procedure (commands, steps) | **Met at HEAD before this bead** — `mothership-dashboard-startup-procedure.md` carries the full commands for paths A/B/C; this document links rather than duplicates, and §1 records the URL and access method actually used |
| Record any errors encountered and how they were resolved | **Met** — §4 lists the access errors, all resolved; §3 records the product defects encountered during access (D1/D2/D3/D5/D6) with mechanisms, evidence, and fix shapes; D3 corrects the earlier record |
| Note the dashboard URL and access method | **Met** — §1 |
| Update relevant project documentation if needed | **Met** — this file is new; `mothership-dashboard-startup-procedure.md` §10 gained a cross-reference; D3 supersedes the mis-transcribed mechanism in the 68620881 notes |
| Document any configuration changes made | **Met** — §5 (none in-repo; the runtime env deviations are tabulated in the execution note §2) |
