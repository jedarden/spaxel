# ESP32 OTA & node reconnection — handoff

**Date:** 2026-08-01
**From:** herdr agent `golf` (`w3:tC`, "Check spaxel deployment and CSI accessibility")
**To:** herdr agent `foxtrot` (`w3:tA`, "Troubleshoot exp32 chip reconnection")

We were working the same bug from different angles on the same hardware. This
note hands over everything established so it does not need re-deriving.
Hardware control is ceded to `foxtrot`.

---

## TL;DR

**OTA is proven reliable. Reconnection is the whole remaining problem.**

Do not spend time doubting the update path — three consecutive OTAs succeeded
end to end, and a fourth delivered the current firmware. Every remaining
symptom is the node failing to re-establish its link, not the update failing.

---

## What is verified working (on hardware)

### Indefinite OTA — `bf-436bh`, closed

The partition table was `factory` + a single `ota_0`, commented "A/B" but not
A/B: `esp_ota_get_next_update_partition()` cycles among **OTA partitions only**,
and `factory` is not one. A node could take exactly **one** update, after which
`esp_ota_begin()` returned `ESP_ERR_OTA_PARTITION_CONFLICT` (surfaced only as
the generic `write_failed`).

Now a true A/B layout, confirmed by reading the table back off the chip:

```
otadata  0x10000     8K
ota_0    0x20000  1984K
ota_1    0x210000 1984K
```

Alternation proven:

```
OTA #1:  boot: Loaded app from partition at offset 0x210000   (ota_0 -> ota_1)
OTA #2:  boot: Loaded app from partition at offset 0x20000    (ota_1 -> ota_0)
```

`nvs` deliberately stays at `0x9000/0x6000`, so a serial reflash **without**
`erase-flash` preserves WiFi credentials and the provisioned flag.

### Update reliability — 3/3

Three consecutive OTA cycles, each downloading, verifying, rebooting and
reporting `verified new firmware`. A fourth delivered `0.1.358`.

One cycle *appeared* to fail. It had not — the mothership was `SIGTERM`ed
underneath it by the other session. Check for `Received signal terminated` in
`mothership.log` before believing an OTA failure.

### Version reporting — `bf-556tl`, closed

`websocket.c` hardcoded `firmware_version` as `"1.0.0"`, so the mothership's
filename-derived expectation could never match and **every successful OTA was
logged as a rollback**. That verdict feeds `ota/autoupdate.go:489-490`, which
treats a canary rollback as grounds to `failUpdateCycle` — so fleet rollout
would have aborted on its first *successful* canary.

Two traps on the way to fixing it, both worth knowing:

1. ESP-IDF wraps `project()` with a macro that **reassigns `PROJECT_VERSION`**
   before `configure_file()` runs, so `version.h` generated an empty string.
   Use `SPAXEL_VERSION` (nothing else owns it); `PROJECT_VER` is set too so the
   value reaches the app descriptor.
2. `VERSION` was not a CMake configure dependency, so bumping it **silently
   no-opped** — build succeeds, filename claims the new version, firmware
   reports the old one. Now declared via `CMAKE_CONFIGURE_DEPENDS`.

### Other fixes verified on hardware

- **`bf-2f0uu`** — the OTA URL was built from `cfg.BindAddr`, so nodes were
  handed `http://0.0.0.0:8080/firmware/...`, unroutable in every default
  deployment including k8s. Now `SPAXEL_ADVERTISED_BASE_URL`, separate from the
  bind address, with startup validation.
- **`bf-2cb85`** — `SendOTAVersion(mac, filename)` was called with a *version*
  and looked up via `GetByFilename`, so a version the API listed as present
  resolved to "not found". Added `GetByVersion`; absent firmware now 404s.
- **`bf-5o010`** — every sender ignored `nc.Conn.WriteMessage`'s error. A node
  can sit in `s.connections` with a dead socket, so the membership check is
  necessary but not sufficient. Reboot/identify now fail properly.

### Console observability — ADR-002

`sdkconfig.defaults` set no `CONFIG_ESP_CONSOLE_*`, so a clean build inherited a
**UART0 console this board does not expose** (native USB only, `303a:1001`, no
bridge in `lsusb`). The device booted in total silence over its only host link.

The previously-flashed image had a hand-configured `sdkconfig` that was never
captured in the repo — it vanished the first time anyone built from scratch,
which had never happened because nothing ever compiled this firmware
(`bf-1nxo`). Default is now USB-Serial/JTAG; `sdkconfig.uart-console` preserves
UART for bridge boards.

**This cost about an hour.** With no console there was no way to distinguish
"not booting" from "booting and invisible" — and the real answer was neither.

---

## The remaining bug — node reconnection (`bf-3c282`)

Three distinct signatures observed, all previously requiring a **physical USB
replug**:

1. **WS retry loop** — endless `Websocket client is not connected`, never
   recovering.
2. **`EHOSTUNREACH` loop** — `esp-tls: connect() error: Host is unreachable`
   (errno 118) every 5s, *while the AP simultaneously showed* the station
   associated at −35 dBm, 22 ms inactive, valid DHCP lease, 0% ping loss.
   L2 and L3 healthy in both directions; the node's own TCP connect disagreed.
   Socket numbers climbed across attempts (`sock=54`, `sock=55`) — worth
   checking whether the reconnect path leaks sockets or netif handles.
3. **Post-OTA silence** — booted into the correct slot, then no console output,
   no association, never rejoined.

Note the WS teardown itself looks *correct* in source: `websocket_connect()`
calls `websocket_disconnect()` first, the handle is cleared under lock, and
`disable_auto_reconnect = true`. Someone hardened this previously. The fault
appears to be below the websocket layer.

### Mitigation shipped, NOT yet verified

`firmware/main/main.c` health task now reboots the node if it sees no mothership
connection for **3 minutes**; the task watchdog is enabled (30 s,
panic-on-timeout).

**The self-recovery test was invalid and must be redone.** It reported
`nodes_online: 1` throughout, but the log showed zero `Node connected` events —
impossible if the server had truly restarted. The endpoint was owned by another
process (below), so the mothership under test was never actually restarted.

> **Watchdog caution:** the 30 s timeout is deliberately generous against the
> 60 s OTA boot-validation window. ESPHome shipped a regression (2026.4.0) where
> an aggressive task watchdog reset devices *before* that window closed and
> caused spurious OTA rollbacks. Tune these two together, never independently.

### A lead worth pursuing — NTP

The node cannot reach `pool.ntp.org` because the bench AP has no internet:

```
ntp: Starting NTP sync with server: pool.ntp.org
W ntp: NTP sync timeout after 10000 ms
W spaxel: NTP sync failed, proceeding without stagger
```

A node with no wall clock is a plausible contributor to reconnection failures
(handshake timing, stagger scheduling). `foxtrot`'s `SPAXEL_NTP_LOCAL_ENABLED`
embedded SNTP responder targets exactly this, and looks like the more promising
root-cause angle — the watchdog/supervisor above is a safety net, not a fix.

---

## Hardware gotchas (each cost real time)

- **Opening `/dev/ttyACM0` without `stty -hupcl` RESETS the node.** Several
  early tests were silently disrupted by this. Also, a capture opened *across* a
  reset dies when the USB peripheral re-enumerates — open the port only after
  the device has settled.
- **`esptool`'s `"Hard resetting via RTS pin..."` is a NO-OP on this board.**
  Native-USB-only means no bridge wiring DTR/RTS to EN/BOOT. The message is
  reassuring and means nothing. After flashing via a BOOT-hold, the chip stays
  in download mode until a **physical power cycle**. A verified-good flash
  presented as a firmware that would not boot for nearly an hour because of this.
- **Tell "in download mode" from "not booting":** `esptool --before no-reset`
  succeeding *only* works if the chip is already in download mode; a stable USB
  device number across resets means the app never took the peripheral.
- **`esptool` intermittently wedges** (`bf-4z6wh`): every connect fails with
  `Write timeout` while the node runs fine. Only a replug recovers it. So the
  cabled recovery path is **not** dependable even on a node you can reach —
  which raises, not lowers, the bar for OTA correctness.
- **Flash is genuinely 4MB** (`esptool flash-id`, Device `0x4016`) — the
  sdkconfig "safe on 16MB boards" hedge does not apply to this hardware.
- **TLS is affordable:** enabling HTTPS costs **+1,392 B** because mbedTLS is
  already linked for OTA SHA-256, leaving ~425 KB free. Lower bound — the CA
  bundle is linker-dropped until `esp_crt_bundle_attach()` is called. The
  binding constraint for Secure Boot v2 is the **bootloader** (11,056 B free),
  not the app slot.

---

## Bench rig

- **AP:** `hostapd` on `wlp3s0`, SSID `spaxel-bench`, **channel 1, HT20**.
  HT20 is required — `signal/phase.go` assumes a 64-subcarrier HT20 map, and an
  HT40 AP would feed phase unwrapping the wrong layout (silently).
- **DHCP:** `dnsmasq`, DHCP-only (`port=0`), `192.168.50.0/24`, lease file under
  `~/spaxel-bench/`.
- **Firewall:** an imperative `iptables -I nixos-fw 1 -i wlp3s0 -j
  nixos-fw-accept` — without it DHCP (UDP 67) is dropped and the node associates
  but never gets an IP. Not persistent across reboot.
- **Mothership:** built from source (`CGO_ENABLED=0`, pure-Go sqlite), run under
  `nohup` from `~/spaxel-bench/`.
- **Firmware builds:** `espressif/idf:release-v5.2` container, source mounted at
  `/src`. This is the first working firmware build environment for this repo.

---

## Why this handoff exists

Two herdr agents were working the same bug on one ESP32, one AP, one port, and
one git working tree. Concrete damage:

- `./mothership-ntp-test2` held `192.168.50.1:8080`, so a `pkill` + restart
  killed only the *other* instance and the replacement died unable to bind —
  producing a self-recovery "pass" that never ran.
- Mothership `SIGTERM`s mid-test made a successful OTA look like a failure.
- Uncommitted `ntpserver` work in the shared tree blocked merging.

**One owner per physical rig.** If a second agent needs spaxel, give it
mothership-side work only — that is safely parallelisable; the hardware is not.
