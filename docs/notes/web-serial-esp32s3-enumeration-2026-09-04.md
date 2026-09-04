# Web Serial ESP32-S3 enumeration — verification record

**Bead:** spaxel-dbd6dfaa ("Verify ESP32-S3 enumeration via Web Serial")
**Date:** 2026-09-04
**Predecessor:** `docs/notes/web-serial-chromium-verification-2026-09-04.md`
(spaxel-9d7cd066 — Web Serial *availability*, which this bead's dependency pointed at)

**Verdict:** **The enumeration ACs are not satisfiable on this host — ex44 has no
ESP32-S3, and in fact no serial device of any kind, attached.** Web Serial itself is
present and functional (re-confirmed live in §4), so the empty result is a hardware
absence, not an API or permission failure. Everything the ACs ask to observe about a
*specific port object* (its VID, its PID, its by-id twin) is unobservable without a
device; every observable part was executed and is recorded below, together with the
expected values from the bench evidence (§5) and the discrepancy classes worth knowing
before a board is attached (§7).

Two new durable headless findings came out of the live run (§4.2): the headless
`requestPort()` hang is **not** cured by injecting a user gesture, and the product
wizard's device-not-found error path is **unreachable** headless — it shows an
eternally disabled "Waiting for device…" button instead.

## 1. Environment

| Component | Value |
|---|---|
| Host | ex44 (standalone Hetzner, NixOS) |
| Browser | Chromium **151.0.7922.173** (nix-store build `/nix/store/53p8msmqxpi829zdrw6qkvaamidxy9cj-chromium-151.0.7922.173/bin/chromium`), headless |
| Driver | playwright **1.59.1** from `dashboard/node_modules`; probes executed via `page.evaluate` (CDP `Runtime.evaluate`) — the programmatic equivalent of typing into the DevTools console |
| Instance under test | mothership dev instance, `SPAXEL_BIND_ADDR=127.0.0.1:18080`, `GET /healthz` → 200 `{"status":"ok"}` immediately before the run |
| Page | `http://127.0.0.1:18080/live` (200, `<title>Spaxel Dashboard</title>`) — the page that wires the onboarding wizard (`live.html:3580` loads `js/onboard.js`) |
| Origin security | `isSecureContext: true` — Web Serial's hard prerequisite, satisfied |
| Probe artifacts | `tmp/web-serial-enum-probe.js`, `tmp/web-serial-enum-results.json`, `tmp/web-serial-enum-wizard.png` — all git-ignored |

`localhost:8080` remains held by the unrelated `telegram-relay` (same host fact recorded
in `MOTHERSHIP_DASHBOARD_ACCESS_FINDINGS.md` §1/§4 and by the predecessor bead); the run
used the working URL.

## 2. Host-side enumeration — the deciding evidence

A full sweep of `/sys/bus/usb/devices/*` (the same data `lsusb` reads; `lsusb` is not
installed here) returns **six** USB devices, none of them serial-capable:

| Device | VID:PID | Product |
|---|---|---|
| 1-11 | `0b05:19af` | AURA LED Controller (ASUS RGB, not a serial device) |
| 1-8 | `05e3:0608` | USB2.0 Hub (Genesys) |
| 1-9 | `174c:2074` | ASM107x hub |
| 2-9 | `174c:3074` | ASM107x hub |
| usb1 | `1d6b:0002` | xHCI Host Controller (root hub) |
| usb2 | `1d6b:0003` | xHCI Host Controller (root hub) |

Corroborating checks, all empty:

- No device anywhere in sysfs carries `idVendor = 303a` (Espressif).
- No USB-attached tty exists: `/dev/ttyUSB*` and `/dev/ttyACM*` are absent, and no
  `/sys/class/tty/*/device` has an `idVendor` (i.e. no tty is backed by a USB device).
- `/sys/bus/usb-serial/devices/` is empty; `/dev/serial` does not exist, so
  `/dev/serial/by-id/` does not either.
- The `cdc_acm`, `cp210x`, `ch341` and `ftdi_sio` modules are not even loaded — the
  kernel has never seen a device that would need them.

This is why `getPorts()` returns `[]` (§4.1) and why no chooser content can appear
(§4.2): **there is no port to enumerate or to request.** The predecessor bead recorded
the same host state earlier today; it was re-verified fresh for this bead rather than
inherited, because hardware state is exactly the kind of premise that changes.

## 3. Acceptance-criteria results

| AC | Result |
|---|---|
| Run `navigator.serial.getPorts()` in DevTools Console | **Executed** (CDP `Runtime.evaluate` in-page, twice). Resolved to `[]` both times, no exception, no prompt, no page error (§4.1) |
| Check the returned port list for the ESP32-S3 device | **Empty list — nothing to check.** The probe applied the AC's own predicate (`info.usbVendorId === 0x303a`) to every returned port and reported `esp32s3Present: false` |
| Verify `usbVendorId: 0x303a`, `usbProductId` appropriate for ESP32-S3 | **Unobservable — no port object exists.** Expected values for this project's board are documented in §5 from the bench evidence |
| Run `navigator.serial.requestPort()` and verify device appears | **Executed three ways** (§4.2): without a gesture, with a real user gesture, and through the product's own wizard. All three pend forever in headless; no device can appear because none is attached |
| Confirm `/dev/serial/by-id/` path matches Web Serial enumeration | **Not possible — `/dev/serial` does not exist on this host.** The correlation contract to run when a board *is* attached is written out in §6 |
| Document the VID/PID found and any discrepancies | **Found: none** (absence is the finding). Expected values + the discrepancy classes to expect in the field are in §5 and §7 |

## 4. Browser-side observations (live run, 2026-09-04 21:40 UTC)

### 4.1 `getPorts()` — empty, spec-correct, twice

```js
await navigator.serial.getPorts()
// → { isArray: true, count: 0, value: [] }        (first call)
// → { secondCallCount: 0 }                        (second call, deterministic)
```

Two independent reasons stack, as in the predecessor bead: a fresh profile has granted
this origin no ports (the permission model — `getPorts()` answers only from the
granted-port store), **and** this host has no serial hardware to grant (§2). After the
whole run — including three aborted `requestPort()` attempts — a closing `getPorts()`
still returned 0: an uncompleted chooser grants nothing.

### 4.2 `requestPort()` — three attempts, one new finding

| Attempt | Shape | Result after 12 s |
|---|---|---|
| E2 | `navigator.serial.requestPort()` with no gesture | **timed out — neither resolved nor rejected**, no `SecurityError`, no page error |
| E3 | real user gesture: a button injected into the page whose handler calls `requestPort()` synchronously, clicked via CDP `Input` (trusted event → transient user activation — the exact spec shape a human click provides) | **timed out — identical hang** |
| E4 | the product's own flow: `#add-node-btn` → wizard "Connect Your ESP32-S3" step → "Select Device" (`#wizard-next` → `onboard.js:115`) | **stalled**: button disabled, text "Waiting for device…", error element `#connect-error` never shown, across 6 samples over 12 s (screenshot `tmp/web-serial-enum-wizard.png`) |

**New finding — the predecessor's remedy is insufficient.** The sibling bead recorded
that `requestPort()` hangs headless *without* a gesture and recommended "inject a
gesture or stub `navigator.serial`" as the workaround, implying the gesture half was a
fix. It is not: **with a genuine user activation, headless Chromium 151 still pends
forever.** The hang is a *chooser-rendering* artifact of headless, not an
*activation* artifact — headed Chrome would open the chooser here (and reject with
`NotFoundError` when the user cancels it). Consequence for automation: a gesture is
worth having for fidelity, but the only working headless strategy remains **stubbing
`navigator.serial`** (as the jest suite already does at
`dashboard/js/onboard.test.setup.js:48`).

**Second finding — the wizard's failure UX is unreachable headless.** The deliberate
`NotFoundError` branch at `onboard.js:120-124` ("No device detected. Did you hold the
BOOT button while plugging in? …") can only fire when `requestPort()` *rejects*. In
headless it never rejects, so the wizard presents an eternally disabled
"Waiting for device…" button with no guidance — exactly what the screenshot shows.
Anyone testing the no-device UX must run headed, not headless.

Console noise during the run (the `/undefined` WebSocket, the `/api/events` 400) is the
pre-existing D2/D3 already owned by `MOTHERSHIP_DASHBOARD_ACCESS_FINDINGS.md` §3 —
unrelated to serial, re-observed only as corroboration.

## 5. The board's expected USB identity (from repo bench evidence)

The validation board this project has been flashed since 2026-07-28 is known-good and
its USB identity is already documented from real sessions:

| Source | Fact |
|---|---|
| `docs/plan/plan.md` ADR-002 (Context) | the validation board presents **`303a:1001`** — "Espressif USB JTAG/serial debug unit" — with no bridge chip on the bus |
| `docs/notes/recovery-mechanisms.md:158` | enumerated as **`303a:1001` → `/dev/ttyACM0`** |
| `docs/notes/esp32-ota-and-reconnection-handoff.md:297` | by-id path: **`/dev/serial/by-id/usb-Espressif_USB_JTAG_serial_debug_unit_50:78:7D:1A:3D:C8-if00`** (the node's MAC as the serial number) |

So the values this AC would expect when the bench board is attached:

- `usbVendorId` **`0x303a`** — Web Serial returns it as a plain number, **`12346`**.
- `usbProductId` **`0x1001`** — likewise **`4097`**.

⚠ **Rendering discrepancy, not a device discrepancy:** `port.getInfo()` yields decimal
numbers. A check written against the literal `"0x303a"` or the *string* `"12346"`
false-negatives against a correct enumeration. Compare numerically: `info.usbVendorId
=== 0x303a`.

## 6. The by-id ↔ Web Serial correlation contract (3 commands, when a board is attached)

`getPorts()` will still return `[]` immediately after plugging a board in — a port
enters that list only after one `requestPort()` grant on this origin (or after the
`connect` event + grant on a later session). The correlation this AC is really asking
for is then:

```bash
# 1. Host side — the kernel's view of the same physical device:
ls -l /dev/serial/by-id/
#   → usb-Espressif_USB_JTAG_serial_debug_unit_50:78:7D:1A:3D:C8-if00 -> ../../ttyACM0

# 2. Tie the tty to its USB identity:
cat /sys/class/tty/ttyACM0/device/../idVendor /sys/class/tty/ttyACM0/device/../idProduct
#   → 303a / 1001
```

```js
// 3. Browser side — the same numbers through Web Serial, after granting once:
const ports = await navigator.serial.getPorts();
ports.map(p => p.getInfo())   // → [{usbVendorId: 12346, usbProductId: 4097}]
```

Match rule: Web Serial's `usbVendorId`/`usbProductId` are the `idVendor`/`idProduct` of
the USB device that backs the tty; the by-id symlink is a stable name for that same
`sysfs` device (embedding its serial number — here the node MAC). Equal triples
(`303a`,`1001`, same physical port) = correlated.

## 7. Discrepancy classes to expect in the field (pre-recorded, none observable here)

1. **Bridge-chip boards carry no `303a` at all.** A devkit wired through CP210x
   (`10c4:ea60`), CH340 (`1a86:7523`) or FTDI (`0403:6001`) enumerates under the
   *bridge's* VID/PID; GPIO43/44 boards never present Espressif's own VID. ADR-002
   chose to provision over both transports precisely so these boards stay first-class.
   Verified product wiring today: `dashboard/js/onboard.js` applies **no VID filter** —
   `getAuthorizedPort()` (:109) takes the first granted port and the Connect step
   (:115) calls bare `requestPort()` — so bridge boards are selectable. **Any future
   filter on `0x303a` would silently exclude every bridge-chip board** and is not
   warranted by this AC.
2. **`usbProductId` is firmware-dependent, not a fixed "ESP32-S3 PID".** `0x1001` is
   what the native USB-Serial/JTAG peripheral reports. A board running a TinyUSB CDC
   build of the firmware reports a different PID under the same `303a` vendor ID. "PID
   appropriate for ESP32-S3" is therefore "whatever the running firmware enumerates
   as," and `0x1001` is the expected value only for the current USB-Serial/JTAG
   console default (ADR-002 decision 3).
3. **Enumerated ≠ usable.** The wedge documented in
   `recovery-mechanisms.md` §3.5 leaves the device fully enumerated (`303a:1001`,
   `/dev/ttyACM0` present) while every transfer times out. A successful enumeration
   (or a chooser entry) is not evidence the port works; only a completed provisioning
   round-trip is.

## 8. What could not be done here

No physical ESP32-S3 was attached to ex44 for this run, so no port object existed to
inspect, request, or correlate against a by-id path. §5 supplies the expected values
from the project's own bench record rather than leaving them to be rediscovered, and
§6 is the ready-made re-check the moment the bench board is plugged into the machine
running the browser — which is the machine whose serial hardware Web Serial can ever
see (a headless server's USB bus is irrelevant to a browser running on a desktop).

## 9. Reproduce

```bash
# instance (if not already running) — see MOTHERSHIP_DASHBOARD_STARTUP_PROCEDURE.md Path B
SPAXEL_BIND_ADDR=127.0.0.1:18080 SPAXEL_MDNS_ENABLED=false \
  SPAXEL_DATA_DIR=tmp/spaxel-devdata go run ./cmd/mothership &   # from mothership/

node tmp/web-serial-enum-probe.js    # playwright + executablePath(nix chromium);
                                     # writes tmp/web-serial-enum-results.json + screenshot

# host half (no lsusb on NixOS — read sysfs directly):
for d in /sys/bus/usb/devices/*; do [ -f "$d/idVendor" ] || continue; \
  printf "%s %s:%s %s\n" "$(basename $d)" "$(cat $d/idVendor)" "$(cat $d/idProduct)" \
  "$(cat $d/product 2>/dev/null)"; done
```

Ops reminder (unchanged from the predecessor bead): the playwright-cached Chromium
cannot launch on NixOS (`libglib-2.0.so.0` absent) — always pass `executablePath`
pointing at the nix-store chromium.

## 10. Gates

Docs-only change — no Go or dashboard source touched, so `go test ./...` / `go vet
./...` were not run for this bead (and could not meaningfully run: the shared working
tree carries another worker's in-flight edits to `mothership/cmd/mothership/main.go`,
`mothership/internal/config/config.go`, `mothership/internal/fleet/handler.go` and
`mothership/internal/fleet/handler_test.go`, so any result would describe their tree,
not this bead's). The probe touched only git-ignored `tmp/` artifacts and one live
read-only dashboard page; no mutating request was sent to the instance.
