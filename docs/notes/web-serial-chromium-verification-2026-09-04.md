# Web Serial API availability in Chromium — verification record

**Bead:** spaxel-9d7cd066 ("Test Web Serial API availability in Chromium")
**Date:** 2026-09-04
**Verdict:** **Web Serial API is present and functional in this Chromium build on this host.**
`navigator.serial` exists on the served dashboard pages, `await navigator.serial.getPorts()`
executes without throwing (resolves to an empty `SerialPort[]`), and no permission prompt or
error of any kind was produced. The only behaviour worth flagging is a headless-specific
`requestPort()` hang, documented in §5.

## 1. Environment

| Component | Value |
|---|---|
| Browser | Chromium **151.0.7922.173** (nix-store build `/nix/store/53p8msmqxpi829zdrw6qkvaamidxy9cj-chromium-151.0.7922.173/bin/chromium`), headless |
| Driver | playwright **1.59.1** from `dashboard/node_modules`; probes executed via `page.evaluate` (CDP `Runtime.evaluate`) — the programmatic equivalent of typing into the DevTools console |
| Instance under test | mothership dev instance (Path B build), pid **3219177**, `SPAXEL_BIND_ADDR=127.0.0.1:18080`, started by spaxel-fc68b47e and still running at verification time (`GET /healthz` → 200 `{"status":"ok"}`) |
| Origin | `http://127.0.0.1:18080` — **`isSecureContext: true`** (loopback origins are trustworthy), which is Web Serial's hard prerequisite |
| Probe artifacts | `tmp/web-serial-probe.js` (script), `tmp/web-serial-results.json` (raw results), `tmp/web-serial-root.png` + `tmp/web-serial-live.png` (screenshots) — all git-ignored |

### The literal AC URL is unsatisfiable on this host

The AC says "navigate to `http://localhost:8080`". That was attempted and recorded: it returns
**HTTP 404 `text/plain` "404 page not found"** — the answer of the unrelated `telegram-relay`
(pid 2794), which holds `*:8080` on ex44. This is the same host fact already recorded in
`MOTHERSHIP_DASHBOARD_ACCESS_FINDINGS.md` §1/§4 and in the closed blocker spaxel-435deb61.
The verification therefore ran against the working URL `http://127.0.0.1:18080`, which serves
this repo's dashboard. `localhost:8080` would have been a secure context too — the port
collision, not the origin, is what blocks that URL.

## 2. Acceptance-criteria results

| AC | Result |
|---|---|
| Open Chromium, navigate to `http://localhost:8080` | **Attempted → 404** (telegram-relay's answer, not spaxel's). Verification continued on `http://127.0.0.1:18080` (200, `<title>Spaxel</title>`) |
| Open DevTools console, run `navigator.serial.getPorts()` | **Ran, no error thrown.** Resolved to `[]` (see §4) |
| Verify the API is accessible (no errors thrown) | **Confirmed.** Two consecutive calls on each of two pages, zero exceptions, zero page errors during the whole run |
| Run `navigator.serial` to confirm the interface exists | **Confirmed.** `'serial' in navigator` → `true`; `String(navigator.serial)` → `"[object Serial]"`; constructor name `Serial`; `instanceof EventTarget` → `true` |
| Document any Web Serial permission prompts or errors | **No prompt appeared and none was expected** from `getPorts()` (§5). `requestPort()` — the prompt-bearing call — was additionally probed and **hangs in headless** (§5) |

## 3. Interface shape observed

Both `http://127.0.0.1:18080/` and `http://127.0.0.1:18080/live` (identical on both):

```
'serial' in navigator                → true
String(navigator.serial)             → "[object Serial]"
navigator.serial.constructor.name    → "Serial"
navigator.serial instanceof EventTarget → true
prototype methods                    → ["onconnect", "ondisconnect", "getPorts", "constructor", "requestPort"]
window.isSecureContext               → true
UA                                   → HeadlessChrome/151.0.0.0 (X11; Linux x86_64)
```

The prototype carries exactly the four Web Serial surface members (`getPorts`, `requestPort`,
`onconnect`, `ondisconnect`) — no truncated or feature-flagged build.

## 4. `getPorts()` result

```js
await navigator.serial.getPorts()   // → []  (Array, length 0, twice per page)
```

Empty is the **correct** result, for two independent reasons that stack here:

1. **Permission model:** `getPorts()` only ever returns ports the user has *already granted* to
   this origin. This probe ran on a fresh ephemeral Chromium profile — nothing was ever granted,
   so `[]` is the spec-correct answer even on a host full of serial hardware.
2. **No hardware attached:** ex44 currently has no serial devices at all — `/dev/ttyUSB*`,
   `/dev/ttyACM*` and `/dev/serial` are absent and `lsusb` lists no serial-class device
   (no CP210x/CH340/FTDI/Espressif `303a:` entry). The bench ESP32-S3 was not plugged in for
   this run.

Neither call produced a permission prompt, a console message, a page error, or a chooser.

## 5. Permission prompts and errors — what was and wasn't exercised

- **`getPorts()` never prompts** (spec behaviour, confirmed empirically): it answers from the
  granted-port store with no user interaction.
- **`requestPort()` is the only prompt-bearing call**, and it is what the product actually uses
  for onboarding (`dashboard/js/onboard.js:115`, wired from the "+ Add Node" wizard served on
  `/live` — `dashboard/live.html:3580` loads `js/onboard.js`). It requires a user gesture and
  renders Chromium's port chooser. **Not exercisable here**: headless renders no chooser and no
  serial hardware exists to list.
- **Headless gotcha (new finding):** calling `navigator.serial.requestPort()` *without* a user
  gesture in headless Chromium 151 **neither resolves nor rejects — it hangs** (observed ≥15 s on
  both pages; the page stayed responsive and no `pageerror` fired). This differs from headed
  Chrome, which rejects promptly with `SecurityError: … Must be handling a user gesture to show a
  permission request`. Consequence: an automated/headless run of the onboarding wizard will sit
  forever inside the `await` at `onboard.js:115` instead of failing fast into the wizard's
  `catch`. Anything driving this dashboard programmatically should inject a user gesture or stub
  `navigator.serial` (the jest suite already does the latter — `dashboard/js/onboard.test.setup.js:48`).
- **No other errors:** the whole run produced zero `pageerror`s. The console noise that did appear
  (the `/undefined` WebSocket on `/`, the `/api/events` 400 listing `anomaly_detected`) is
  pre-existing and already owned by `MOTHERSHIP_DASHBOARD_ACCESS_FINDINGS.md` §3 defects D2/D3 —
  unrelated to Web Serial, re-observed here only as corroboration.

## 6. Product wiring this verification applies to

| Site (line numbers at HEAD) | Role |
|---|---|
| `dashboard/js/onboard.js:109` | `getAuthorizedPort()` — `getPorts()`, reuse first granted port |
| `dashboard/js/onboard.js:113-115` | `requestPort()` — the prompt-bearing path, error-mapped to `UserError` |
| `dashboard/js/onboard.js:298` | Wizard browser check — `if (navigator.serial)` feature gate (passes on this build) |
| `dashboard/live.html:3580` | Loads `js/onboard.js` onto the `/live` dashboard |

## 7. Reproduce

```bash
# instance (if not already running) — see MOTHERSHIP_DASHBOARD_STARTUP_PROCEDURE.md Path B
SPAXEL_BIND_ADDR=127.0.0.1:18080 SPAXEL_MDNS_ENABLED=false \
  SPAXEL_DATA_DIR=tmp/spaxel-devdata go run ./cmd/mothership &   # from mothership/

node tmp/web-serial-probe.js        # playwright + executablePath(nix chromium), writes
                                    # tmp/web-serial-results.json + screenshots
```

Ops reminder (same as `MOTHERSHIP_DASHBOARD_ACCESS_FINDINGS.md` §4 row 3): the playwright-cached
Chromium cannot launch on NixOS (`libglib-2.0.so.0` absent) — always pass `executablePath`
pointing at the nix-store chromium.

## 8. Gates

Docs-only change — no Go or dashboard source touched, so `go test ./...` / `go vet ./...` were
not run for this bead (and could not meaningfully run: the shared working tree currently carries
another worker's in-flight edits to `mothership/cmd/mothership/main.go`,
`mothership/internal/config/config.go` and `mothership/internal/fleet/handler.go`, so any test
result would describe their tree, not this bead's).
