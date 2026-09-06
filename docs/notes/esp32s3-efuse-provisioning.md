# ESP32-S3 eFuse provisioning — Secure Boot V2, key revocation, JTAG and download-mode closure

Split child of spaxel-6a829fea (ADR-004 OTA security hardening). This is the
written reference the burn script (spaxel-2e256299) will implement. It is a
reference only: nothing in this repository burns an eFuse, and every command
below is a host-side provisioning action run by a person with the board on a
bench, not something a build or a CI leg does.

> **Delivered unvalidated on hardware.** Every eFuse name, bit position and
> log string below was read out of the pinned ESP-IDF v5.2.3 tree, not out of
> a chip. The bench bead spaxel-6c9344e4 is deferred and ex44 has no flashing
> host, so no step here has been executed against a real ESP32-S3. Treat the
> first bench run of the burn script as the validation gate, and expect to
> correct this document from what the bench shows.

All bit positions and symbol names are from
`components/efuse/esp32s3/esp_efuse_table.c` and
`components/efuse/esp32s3/include/esp_efuse_chip.h` in ESP-IDF **v5.2.3**
(`/home/coding/esp/esp-idf`, tag `v5.2.3`), which is the toolchain version
this repo builds with. ESP32-S3 eFuse names differ from other targets — do
not copy a table from an ESP32 or an ESP32-C3 doc.

## The premise this doc corrects

The parent bead's description, and everything downstream of the commit that
set it, carried this line:

> `CONFIG_SECURE_BOOT_V2_ALLOW_EFUSE_DISABLE=y is set (b4264982)`

That symbol does not exist in ESP-IDF 5.x. `firmware/sdkconfig.defaults`
(~line 125, corrected 2026-09-04) records the full correction: **every**
`CONFIG_SECURE_BOOT_V2*` line this repo once carried named a symbol that
never existed, so no spaxel firmware build has ever had Secure Boot on, and
the images are signed by the build but verified by nobody. The real enable
symbol is `CONFIG_SECURE_BOOT`, and it is deliberately still commented out in
`firmware/sdkconfig.defaults` pending the ADR-007 phase-3 go/no-go (open bead
spaxel-30a23d74, which also owns the flash-encryption question).

The second half of the correction is operational, and it is the reason this
document exists: **burning eFuses is a provisioning action, not a config
flip.** Uncommenting `CONFIG_SECURE_BOOT=y` in a build produces a bootloader
and app that *expect* the hardware to be provisioned; it provisions nothing.
The order below is the provisioning that has to happen before that flip is
safe to ship.

## What gets burned on an ESP32-S3 node

Everything the fleet needs lives in eFuse block 0 (the common block) plus one
256-bit key block for the digest. `espefuse.py summary` prints all of it.

| eFuse | BLK0 bit(s) | Purpose | Reversible? |
|---|---|---|---|
| `SECURE_BOOT_EN` | 116 | enables Secure Boot verification in ROM and bootloader | **no** — write-once |
| `KEY_PURPOSE_0`..`KEY_PURPOSE_5` | 88/92/96/100/104/108, 4 bits each | assigns the purpose of `BLOCK_KEY0`..`BLOCK_KEY5` | no once write-protected |
| `BLOCK_KEY0`..`BLOCK_KEY5` | BLK1..BLK6, 256 bits | key material; holds the Secure Boot public-key digest here | no once write-protected |
| `SECURE_BOOT_KEY_REVOKE0` | 85 | permanently retires the digest in the `DIGEST0` slot | **no** |
| `SECURE_BOOT_KEY_REVOKE1` | 86 | same, `DIGEST1` slot | **no** |
| `SECURE_BOOT_KEY_REVOKE2` | 87 | same, `DIGEST2` slot | **no** |
| `SECURE_VERSION` | 142, 16 bits | anti-rollback counter (matches `CONFIG_BOOTLOADER_APP_SEC_VER_SIZE_EFUSE_FIELD`, default 16 on S3) | increments only |
| `DIS_PAD_JTAG` | 51 | hard JTAG disable (aka `HARD_DIS_JTAG`) — permanent | **no** |
| `SOFT_DIS_JTAG` | 48, 3 bits | soft JTAG disable (odd number of 1s); software can re-enable via the HMAC module | semi — see below |
| `DIS_USB_JTAG` | 118 | the USB-Serial-JTAG peripheral no longer switches into JTAG mode | **no** |
| `DIS_USB_SERIAL_JTAG_DOWNLOAD_MODE` | 132 | forbids download mode through USB-Serial-JTAG | **no** |
| `DIS_USB_OTG_DOWNLOAD_MODE` | 159 | forbids download through USB-OTG | **no** |
| `DIS_DOWNLOAD_MODE` | 128 | forbids **all** ROM download/boot modes (`boot_mode` 0,1,2,3,6,7) | **no** |
| `DIS_DIRECT_BOOT` | 129 | disables direct/legacy SPI boot (`DIS_LEGACY_SPI_BOOT`) | **no** |
| `ENABLE_SECURITY_DOWNLOAD` | 133 | re-opens a *secure* download mode as a recovery path | **no** |

The Secure Boot key purposes an S3 understands are in
`esp_efuse_chip.h`: `ESP_EFUSE_KEY_PURPOSE_SECURE_BOOT_DIGEST0 = 9`,
`DIGEST1 = 10`, `DIGEST2 = 11`. **Three trusted digests, ever** — this is the
hardware ceiling behind the rotation policy in
[firmware-signing-keys.md](firmware-signing-keys.md), and it is why that
document caps rotation at three keys over the life of a node.

The three-digest ceiling has a sharper edge than "rotate at most twice", and
it is easy to walk into. When an app built with `CONFIG_SECURE_BOOT=y` boots,
`esp_secure_boot_init_checks()` → `secure_boot_v2_check()`
(`components/bootloader_support/src/secure_boot.c`, v5.2.3 lines ~117-135)
does two things automatically:

- every `SECURE_BOOT_DIGESTx` slot that is **empty gets revoked** —
  `"Unused SECURE_BOOT_DIGEST%d should be revoked. Fixing.."` — *unless*
  `CONFIG_SECURE_BOOT_ALLOW_UNUSED_DIGEST_SLOTS` is set;
- every slot in use that is missing write-protection gets it burned in
  (`"The KEY_PURPOSE_SECURE_BOOT_DIGEST%d should be write-protected. Fixing.."`).

So the rotation headroom is consumed at the first boot of the first
`CONFIG_SECURE_BOOT=y` image, not held open for later. The fleet's choice is
between (a) provisioning **all** rotation keys' digests up front —
`sign-firmware.sh` already accepts up to three keys per image, which is
exactly the multi-digest shape the hardware wants — or (b) setting
`CONFIG_SECURE_BOOT_ALLOW_UNUSED_DIGEST_SLOTS` and owning the risk the v5.2
doc names for it: an unrevoked empty slot is a slot an attacker with physical
access can fill with their own key. Decision belongs to the phase-3 review
(spaxel-30a23d74), not to the burn script; the script should implement
whichever the review picks and refuse to guess.

### What is deliberately not burned

- **`DIS_USB_SERIAL_JTAG`** (BLK0 bit 119) disables the USB-Serial-JTAG
  peripheral *entirely* — console included. The fleet's console profile is
  `usb` (see "Console survival" below), so burning this bit
  destroys the only console a node has. Not in the set, ever, until the
  console profile moves to `uart` first.
- **`DIS_USB_OTG`** (BLK0 bit 45) kills the whole USB-OTG peripheral. The
  specific download-mode bit is enough; the blanket one buys nothing here.
- **`SECURE_BOOT_AGGRESSIVE_REVOKE`** (BLK0 bit 117) revokes a key the
  moment a verification against it fails, instead of waiting for the
  controlled migration the rotation policy describes. Correct for a
  threat model with regular physical attack; wrong for a fleet that flashes
  the occasional dev image. Phase-3 decision.
- **`WR_DIS` write-protection of `SECURE_VERSION`**. The counter must stay
  writable by the running firmware: the OTA install path advances it
  (`esp_ota_set_boot_partition()` — see
  [anti-rollback-secure-version.md](anti-rollback-secure-version.md),
  "Who burns it, and when"). Write-protecting the field converts every
  future OTA into a brick. Burn the *value*, never the *protection*.

## Burn order, with rationale

The ordering constraint is one-directional and absolute: **every step that
closes a recovery path must come after the step that puts a known-good,
prod-signed image on the node, and `SECURE_BOOT_EN` must come last.** A node
that has `SECURE_BOOT_EN` burned and no matching key digest provisioned is a
paperweight: the ROM verifies the bootloader's signature block against the
provisioned digest, finds nothing, and refuses to boot — permanently.

Pre-flight, once per node:

```bash
# 0a. The image that is about to be flashed must be the prod-signed one.
#     Never a dev-signed image: its digest is not the one being provisioned.
espsecure.py signature_info_v2 spaxel-signed.bin          # shows signature blocks
python3 firmware/scripts/inspect-firmware-header.py spaxel-signed.bin   # secure_version

# 0b. The eFuse state before anything is burned — save this output with the
#     node's record. It is the baseline every later check is diffed against.
espefuse.py --port /dev/ttyACM0 --chip esp32s3 summary
```

Then, in order:

### (a) Flash the prod-signed image — while flashing is still possible

Flash `spaxel-signed.bin` with the normal flow (`scripts/flash-esp32s3.sh`).
This is
the last time USB flashing works on this node, ever, once (c) lands. The
image carries the `secure_version` the build stamped from
`firmware/SECURE_VERSION`; step (d) burns the matching counter value.

Why first: everything after this step progressively removes the ability to
fix a mistake over USB. A node is only ever one burn away from
"recovery means signed OTA or physical replacement", and the order exists so
that the node can already boot and take an OTA at every later step.

### (b) Provision the key digest — before any `CONFIG_SECURE_BOOT=y` image

```bash
# Digest of the production key's public half. Computed host-side from the
# key file; the key itself travels exactly as firmware-signing-keys.md
# prescribes (stdin/fd, never a path in a log) and is never printed.
bao-as openbao-v2 bao kv get -field=key_pem \
    secret/ardenone-cluster/spaxel/firmware-signing/prod \
  > /run/user/$UID/prod-key.pem          # mode 600, tmpfs, removed after
espsecure.py digest_sbv2_public_key --keyfile /run/user/$UID/prod-key.pem \
    --output /run/user/$UID/prod-digest.bin

# Burn into a free key block with a digest purpose. espefuse sets the
# write-protections on the block and the purpose field as part of the burn.
espefuse.py --port /dev/ttyACM0 --chip esp32s3 \
    burn_key BLOCK_KEY0 SECURE_BOOT_DIGEST0 /run/user/$UID/prod-digest.bin
shred -u /run/user/$UID/prod-key.pem /run/user/$UID/prod-digest.bin
```

Why before (e) and before any Secure-Boot-enabled build is flashed: a
bootloader that enables `SECURE_BOOT_EN` itself on first boot (the
`CONFIG_SECURE_BOOT_BUILD_SIGNED_BINARIES` release flow) can only do so
safely if the digest it will be verified against is already in the block.
Digest first, enable second — no ordering under which the node boots
unprotected one time less.

**Never flash a `CONFIG_SECURE_BOOT=y` image onto a node whose digest is not
yet provisioned.** The first boot enables the eFuse and then refuses to boot
the very image that enabled it. `firmware/sdkconfig.defaults` keeps the
symbol commented out for exactly this reason.

### (c) Close JTAG and the download modes

```bash
espefuse.py --port /dev/ttyACM0 --chip esp32s3 burn_efuse DIS_PAD_JTAG
espefuse.py --port /dev/ttyACM0 --chip esp32s3 burn_efuse DIS_USB_JTAG
espefuse.py --port /dev/ttyACM0 --chip esp32s3 burn_efuse DIS_USB_SERIAL_JTAG_DOWNLOAD_MODE
espefuse.py --port /dev/ttyACM0 --chip esp32s3 burn_efuse DIS_USB_OTG_DOWNLOAD_MODE
espefuse.py --port /dev/ttyACM0 --chip esp32s3 burn_efuse DIS_DIRECT_BOOT
# Fleet decision, burn script must gate it on an explicit flag:
espefuse.py --port /dev/ttyACM0 --chip esp32s3 burn_efuse DIS_DOWNLOAD_MODE
```

`DIS_PAD_JTAG` + `DIS_USB_JTAG` close both physical JTAG paths on the S3.
`SOFT_DIS_JTAG` is the software-settable third layer the runtime can apply
(and un-apply through the HMAC module) — it is not a provisioning action and
the burn script has no business setting it. The download-mode closures are
the ones that end the 2026-08-07 incident class: the credential that was
extracted went out through the esptool read path, and
`DIS_DOWNLOAD_MODE` is the bit that closes it for good.

The consequence must be stated in the same breath as the command:
**`DIS_DOWNLOAD_MODE` is the point of no return for flashing.** After it, a
node can only be updated by a signed image it already trusts (OTA) or
replaced. If the fleet ever wants a recovery path, `ENABLE_SECURITY_DOWNLOAD`
is the narrower bit — but it re-opens a download mode, which is the thing
being closed, so it is a phase-3 decision too. This is why (a) is step one
and why the burn script must refuse to run these against a node that has not
booted the prod-signed image.

### (d) Burn the anti-rollback counter

```bash
espefuse.py --port /dev/ttyACM0 --chip esp32s3 burn_efuse SECURE_VERSION <N>
```

`N` is the `secure_version` the flashed image carries (step 0a read it out of
the header). The field is 16 bits (`{EFUSE_BLK0, 142, 16}`), matching
`CONFIG_BOOTLOADER_APP_SEC_VER_SIZE_EFUSE_FIELD`, and it only ever moves
upward. Do **not** write-protect it — see the table above.

The fleet-level rules about which `N` to burn, when, and why the first bump
is the most dangerous moment in the fleet's life are owned by
[anti-rollback-secure-version.md](anti-rollback-secure-version.md); this doc
only fixes the per-node mechanics.

### (e) Enable Secure Boot — last, irreversible, final

```bash
espefuse.py --port /dev/ttyACM0 --chip esp32s3 burn_efuse SECURE_BOOT_EN
```

From this boot on, the ROM verifies the bootloader's signature block and the
bootloader verifies every app image, against the digest provisioned in (b).
There is no un-burn. A node in this state that rejects its own image does not
boot, and cannot be re-flashed if (c) also landed. That is the protection
working.

## Console survival — which profile lives through provisioning

`firmware/scripts/verify-console-config.sh` pins the two profiles the build
can produce. The shipped one is `usb`:

```
CONFIG_ESP_CONSOLE_USB_SERIAL_JTAG=y
# CONFIG_ESP_CONSOLE_UART_DEFAULT is not set
CONFIG_ESP_CONSOLE_SECONDARY_NONE=y
CONFIG_ESP_CONSOLE_UART_NUM=-1
```

That means **USB-Serial-JTAG is the only console a node has.** Walk the burn
set against it:

| Burned bit | `usb` console | Why |
|---|---|---|
| `DIS_PAD_JTAG` | survives | touches the pad-JTAG path only |
| `DIS_USB_JTAG` | **survives** | stops the peripheral switching to JTAG mode; console and download keep working — this is the bit that kills JTAG without killing the console |
| `DIS_USB_SERIAL_JTAG_DOWNLOAD_MODE` | survives | download only; the console channel stays up |
| `DIS_DOWNLOAD_MODE` | survives | no download mode at all, but the console still prints; flashing is what dies, not logging |
| `DIS_USB_SERIAL_JTAG` | **dead** | the whole peripheral is off — no console, no logs, nothing |

So the rule for any worker holding a burn script: **the set above never
includes `DIS_USB_SERIAL_JTAG`.** JTAG closure on the S3 is achieved with
`DIS_PAD_JTAG` + `DIS_USB_JTAG`; disabling the USB-Serial-JTAG *device* is a
different, larger act that also deletes the fleet's only console. If some
future hardening review genuinely wants that bit burned, the build has to
move to the `uart` profile first (`verify-console-config.sh uart`), which is
a re-flash — and therefore a change that has to land *before* (c) closes the
download modes, because after (c) there is no re-flashing to be had.

## Reading a provisioned board

From the console of a **running** app built with `CONFIG_SECURE_BOOT=y`
(v5.2.3, `components/bootloader_support/src/secure_boot.c` and
`esp_image_format.c` — verbatim strings):

- `Mismatch in secure boot settings: the app config is enabled but eFuse not`
  (log error, `esp_secure_boot_init_checks`) — **not** provisioned. This is
  the loud, ordinary signature of every node in the fleet today, once a
  Secure-Boot-enabled build ships.
- `Not enabled Secure Boot (SECURE_BOOT_EN->1)` and
  `Not disabled ROM Download mode (DIS_DOWNLOAD_MODE->1)` /
  `Not enabled Security download mode (ENABLE_SECURITY_DOWNLOAD->1)`
  (the release-mode check, `esp_secure_boot_cfg_verify_release_mode`) —
  same meaning, itemised: which bits are still open.
- `"Fixed"` (log info, after one of the `"should not be writeable. Fixing.."`
  warnings) — the app just burned a missing protection itself. Harmless
  after (b), and the correct signal that provisioning was incomplete.
- An **unsigned** image handed to a provisioned node is refused at boot with
  `Secure boot signature verification failed` (`esp_image_format.c`) and does
  not enter download mode. That refusal is the enforcement proof, and it is
  the parent bead's bench AC: a provisioned node boots a signed image and
  refuses an unsigned one. Run that probe on the bench *before* `SECURE_BOOT_EN`
  is burned fleet-wide, and never on the only console a mounted node has.

The authoritative check is host-side, not console-side, and should be captured
into the node's record after every burn:

```bash
espefuse.py --port /dev/ttyACM0 --chip esp32s3 summary
# SECURE_BOOT_EN = 1, KEY_PURPOSE_0 = SECURE_BOOT_DIGEST0, revocation bits,
# every DIS_* bit from (c), SECURE_VERSION = <N>
```

## What this document does not own

- **Key custody, the dev/prod split, the ceremony, rotation policy, and the
  revocation-burn mechanics for a compromised key** —
  [firmware-signing-keys.md](firmware-signing-keys.md). This doc covers the
  per-node provisioning step; that one owns where the key lives and what to
  do when it leaks. `SECURE_BOOT_KEY_REVOKE0..2` burned in *rotation* (rather
  than as unused-slot hygiene) is that document's scenario, not this one's.
- **The anti-rollback bump policy, the OTA-rollback brick window, and what
  ships vs what CI gates** —
  [anti-rollback-secure-version.md](anti-rollback-secure-version.md), plus
  `firmware/scripts/bump-secure-version.sh`.
- **The burn script itself** — spaxel-2e256299, `firmware/scripts/burn-efuses.sh`,
  implements the order above; spaxel-30940d60 adds the prod-signed-image guard;
  spaxel-566b18de wires it into `firmware/scripts/README.md`.
- **The go/no-go on enabling Secure Boot and flash encryption** —
  spaxel-30a23d74 (ADR-007 phase 3), including the `ALLOW_UNUSED_DIGEST_SLOTS`
  and `ENABLE_SECURITY_DOWNLOAD` choices flagged above.

## Validation status

Delivered unvalidated on hardware. The bench bead
spaxel-6c9344e4 (real ESP32-S3 flashing on the Lenovo T450s bench) is
**deferred**, and ex44 has no flashing host, so every command in this
document is unexecuted against real silicon. The symbol table, the log
strings and the CLI shapes come from the pinned ESP-IDF v5.2.3 tree, which is
the same version the firmware builds with — but eFuse behaviour under
`espefuse.py` is exactly the kind of thing the docs get subtly wrong per chip
revision, and the doc should be corrected from the bench run rather than
defended. The first bench run of `burn-efuses.sh` should diff
`espefuse.py summary` before and after each step above and attach both dumps
to the bench bead's record.

## References

- ESP-IDF v5.2 Secure Boot V2 (ESP32-S3):
  <https://docs.espressif.com/projects/esp-idf/en/v5.2/esp32s3/security/secure-boot-v2.html>
- ESP-IDF v5.2 eFuse reference: <https://docs.espressif.com/projects/esp-idf/en/v5.2/api-reference/peripherals/efuse.html>
- Local pinned tree: `/home/coding/esp/esp-idf` (tag `v5.2.3`) —
  `components/efuse/esp32s3/esp_efuse_table.c` (bit positions),
  `components/efuse/esp32s3/include/esp_efuse_chip.h` (key purposes),
  `components/bootloader_support/src/secure_boot.c` (runtime checks and the
  unused-slot auto-revoke),
  `docs/en/security/host-based-security-workflows.rst` (the
  `digest_sbv2_public_key` / `burn_key` / `burn_efuse SECURE_BOOT_EN`
  command shapes).
- Repo: [firmware-signing-keys.md](firmware-signing-keys.md),
  [anti-rollback-secure-version.md](anti-rollback-secure-version.md),
  `firmware/sdkconfig.defaults` (~line 125, the corrected Secure Boot block),
  `firmware/scripts/sign-firmware.sh`,
  `firmware/scripts/verify-console-config.sh`,
  `firmware/scripts/inspect-firmware-header.py`,
  `scripts/flash-esp32s3.sh`.
- Beads: parent spaxel-6a829fea, script spaxel-2e256299, guard
  spaxel-30940d60, README wiring spaxel-566b18de, bench spaxel-6c9344e4
  (deferred), phase-3 go/no-go spaxel-30a23d74.
