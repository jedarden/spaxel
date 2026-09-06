# Anti-rollback secure version — the one-way eFuse counter

**Date:** 2026-09-05
**Bead:** spaxel-497aee78 (split child of spaxel-8aa9703c / ADR-004)
**Companion record:** [`docs/notes/ota-security-hardening-2026-08-15.md`](ota-security-hardening-2026-08-15.md)

## Summary

Anti-rollback is enabled in `firmware/sdkconfig.defaults` — written there as
`CONFIG_APP_ANTI_ROLLBACK`, the deprecated short name of
`CONFIG_BOOTLOADER_APP_ANTI_ROLLBACK` — and the generated `sdkconfig` of the
2026-09-05 build confirms it really was on. But until this change it protected
nothing: `CONFIG_BOOTLOADER_APP_SECURE_VERSION` was never set, so **every image
this project ever shipped carried secure_version 0**. Verified by parsing the
header of the 2026-09-05 build artifact (`firmware/build/spaxel-firmware.bin`,
app version 0.2.173, IDF v5.2.3): image-header byte 19 = 0 and the app
descriptor's `secure_version` = 0. With the eFuse counter also at 0, the
bootloader's `image.secure_version >= eFuse` check always passed — a downgrade
attack was unthrottled.

(The short name works only through ESP-IDF's `sdkconfig.rename` map, and
several other lines in that file name symbols that do not exist at all; the
2026-09-04 pass over that file, which renames the real symbol and drops the
nonexistent ones, is a separate change from this one.)

The release process now owns a separate counter, and it is **not** the app
version. It moves only when a release changes the security posture.

## Where the number comes from

```
firmware/SECURE_VERSION                 ← single source of truth, tracked in git
        │  read + validated by firmware/CMakeLists.txt before project()
        ▼
sdkconfig.secure-version                ← generated fragment, appended to SDKCONFIG_DEFAULTS
        │
        ▼
CONFIG_BOOTLOADER_APP_SECURE_VERSION    ← ESP-IDF Kconfig
        │
        ├── esptool elf2image  → esp_image_header_t.secure_version (byte 19)
        └── compile time       → esp_app_desc_t.secure_version (u32 at +4)
        │
        ▼
ESP32-S3 bootloader compares it against the SECURE_VERSION eFuse field
```

Both stamps are checked against each other by
`firmware/scripts/inspect-firmware-header.py`; if they ever disagree, the
build wired one path but not the other, and that is a release blocker.

`firmware/CMakeLists.txt` fails the configure outright if the file is missing,
is not a single non-negative integer, or exceeds 65535 (the width of
`CONFIG_BOOTLOADER_APP_SEC_VER_SIZE_EFUSE_FIELD` on the ESP32-S3). It also
reconciles a generated local `firmware/sdkconfig` that still carries the old
value: ESP-IDF lets an existing `sdkconfig` win over `SDKCONFIG_DEFAULTS`, so
a defaults-only change would silently no-op forever — the same failure class
the VERSION handling in that file already guards against.

**Do not move the number into `sdkconfig.defaults`.** That file is hand-edited
policy and, more importantly, a defaults line cannot override a value already
recorded in a generated local `sdkconfig`, which is exactly how a bump would
ship the old version again while everyone believed it had landed.

## The one-way eFuse

The `SECURE_VERSION` field lives in eFuse **BLOCK0** on the ESP32-S3, 16 bits
wide (`CONFIG_BOOTLOADER_APP_SEC_VER_SIZE_EFUSE_FIELD`).

**eFuse bits can only be programmed from 0 to 1, never back.** There is no
command, no API and no recovery mode that clears one. Consequently:

- The counter can only ever increase, on any node, forever.
- A value that has reached a node is permanent for that node. A bootloader
  will refuse to boot any image whose `secure_version` is lower than the
  burned value, *including an image the node already ran successfully*.
- The only "undo" is physical access plus `esptool.py write_flash` of an image
  carrying a value ≥ the burned one. If no such image exists, the node is
  unrecoverable in the field.

### Who burns it, and when

Nobody in this repository burns it. Two paths do, both device-side:

1. **The OTA install path.** `esp_ota_set_boot_partition()` — which
   `ota_task` in `firmware/main/websocket.c` calls as part of applying an
   update — advances the eFuse when the incoming image's `secure_version` is
   higher than the burned value.
2. **The bootloader**, when it boots an image whose `secure_version` is higher
   than the burned value.

Manual burning for provisioning or for a bench recovery is
`espefuse.py --port /dev/ttyUSB0 burn_efuse SECURE_VERSION <N>` (host IDF
toolchain only — the dashboard's bundled esptool-js is read-only for eFuse,
see `docs/plan/plan.md`).

> **Verify the exact trigger on the bench before the first fleet bump.** The
> statement above is ESP-IDF's documented behaviour for v5.2; this project has
> no hardware-in-the-loop CI (see the heap-measurement beads), so nothing here
> has exercised it. Confirm with `espefuse.py summary` before and after a
> controlled OTA on a bench node.

## The interaction that can take a node down

`CONFIG_BOOTLOADER_APP_ROLLBACK_ENABLE=y` and anti-rollback are both on, and
together they create a window where a *failed* update is worse than a failed
update without anti-rollback:

```
node runs image A (secure_version 0, eFuse 0)
  → OTA installs image B (secure_version 1)
  → esp_ota_set_boot_partition() burns eFuse 0 → 1        ← before B is proven
  → reboot into B
  → B fails validation inside the 60s window (websocket.c SPAXEL_OTA_VALID_TIMEOUT_S)
  → rollback asks for slot A
  → bootloader refuses: A has secure_version 0 < eFuse 1
  → neither slot is bootable
```

The node then needs physical USB access to recover. For nodes mounted around a
living space that is the real cost of a bump, not the eFuse bits.

**This is why the bump policy below insists on a proven image, and why the
first bump is the most dangerous one in the fleet's life: every node is
currently running a secure_version 0 image, so the first release that carries
1 has exactly the failure shape above.**

## Bump policy

### Bump when

A release changes what the node will accept as trustworthy, or closes a way
in:

- a fix for a vulnerability reachable over the network (WiFi CSI parsing,
  BLE, the WebSocket control channel, the HTTP OTA download, the NVS
  migration path);
- a change to the update trust chain itself — mothership authentication, TLS
  certificate handling, image verification;
- enabling Secure Boot V2 or signing (spaxel-30a23d74);
- disabling a debug surface (JTAG, USB download mode).

### Do not bump when

- the change is a feature, a CSI/positioning improvement, an LED behaviour,
  or a version-number-only change;
- a dependency is bumped with no security consequence;
- the change is in `mothership/` — the mothership cannot retroactively lower
  a node's counter, and bumping firmware for a server-side change just spends
  a permanent bit;
- the image has not been validated on a canary node through the full OTA
  validation and boot-good windows.

Each bump is a permanent floor. Bumping on every release makes every future
node refuse every older image, and turns any future bad release into a fleet
of nodes needing physical access.

### Procedure

```bash
cd firmware
./scripts/bump-secure-version.sh --reason "what changed and why it is security"
git add SECURE_VERSION SECURE_VERSION_LOG.md
git commit -m "..."        # both files together, always
# build, then gate the artifact before it ships:
./scripts/inspect-firmware-header.py build/spaxel-firmware.bin --expect <N>
./scripts/inspect-firmware-header.py spaxel-firmware-<VERSION>-merged.bin --expect <N>
```

`scripts/bump-secure-version.sh` refuses to decrease the value and refuses a
bump with no reason; the reason becomes a row in
[`firmware/SECURE_VERSION_LOG.md`](../../firmware/SECURE_VERSION_LOG.md), which
is the audit trail that makes the counter impossible to regress quietly.

Order of operations matters: **canary first, bump second.** Build the image,
flash it to one node, let it pass validation and the boot-good window, and
only then record the bump and ship. Bumping before the canary means the fleet
receives an unproven image with an advanced counter — the failure shape above.

Never bump twice for one release, and never bump in the same push as unrelated
work: the bump commit should be reviewable on its own.

## What ships, and a CI gap

CI builds and attaches `spaxel-firmware-<VERSION>.bin` and the merged image to
the GitHub Release (the `firmware-build` step of the `spaxel-build` template,
IDF v5.2, `esp32s3`). The Docker image fetches them from there.

**Gap, disclosed rather than fixed:** the Argo sensor's firmware trigger set
(see [`docs/build-path-filter-spec.md`](build-path-filter-spec.md)) is
`firmware/main/**`, `firmware/CMakeLists.txt`, the three sdkconfig layers,
`firmware/partitions.csv`, `firmware/dependencies.lock` and
`firmware/scripts/**`. `firmware/SECURE_VERSION` and
`firmware/SECURE_VERSION_LOG.md` are **not** in it, so a push containing only a
bump will not start a build. The bump reaches the fleet with the next push
that does. Wiring the sensor is a `declarative-config` change and lives
outside this repository.

## Verification status

| Check | Result |
|---|---|
| Existing artifact parses as expected | ✅ `inspect-firmware-header.py` reads `firmware/build/spaxel-firmware.bin` (app version 0.2.173, IDF v5.2.3, chip_id 9) — `secure_version` 0 in both the image header and the app descriptor, proving the premise |
| Bump script guard rails | ✅ refuses decrease, refuses missing `--reason`, refuses non-integer and >65535, refuses a no-op |
| Generated fragment + stale-sdkconfig reconciliation | ✅ by inspection of the CMake; **not exercised** |
| Release image carrying secure_version 1 | ❌ **not built** — ex44 has no reachable ESP-IDF toolchain from this workspace, so no artifact with the new value was produced. The first release build must run `inspect-firmware-header.py --expect 1` on both attached images before it ships. |

## Scope

This is firmware/bootloader-level anti-rollback only. The auto-update
manager's own downgrade/rollback decision logic is owned by spaxel-005c84ce
and is deliberately not touched here.
