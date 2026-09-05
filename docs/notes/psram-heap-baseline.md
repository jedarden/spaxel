# PSRAM idle-heap baseline — turnkey measurement runbook

Bead `spaxel-7ace37f9` (ADR-008 decision 5). Status: **measurement NOT taken —
no ESP32-S3 board is attached to ex44** (verified 2026-09-05: `lsusb` shows no
`303a`/CP210x/CH340/FTDI device, no `/dev/ttyUSB*`/`/dev/ttyACM*`). Everything
below that does not need hardware is done; the board-time procedure is at
§6 and is copy-pasteable. Unblock condition: a human attaches a node
(same shape as `spaxel-082135bc`).

## 1. Why this measurement gates PKI work

ADR-008 decision 5 makes the real idle-heap delta of `CONFIG_SPIRAM=y` a
go/no-go for certificate-based identity: if the gain is not large enough to
hold TLS buffers, the fallback is the ADR-007 symmetric design. The number
must be measured. **This document deliberately contains no heap number** —
neither configuration has ever been booted with heap telemetry recorded, and
a guessed figure would poison the gate instead of informing it.

## 2. Premise correction — "PSRAM is not set" is stale

The task text says `CONFIG_SPIRAM is NOT set (current state)`. That was true
when the family was written; it is not true at HEAD. Child 1
(`spaxel-be68a42f`, "Enable CONFIG_SPIRAM in sdkconfig and validate device
boots") landed, so `firmware/sdkconfig.defaults:125` already reads
`CONFIG_SPIRAM=y`.

The PSRAM-off arm therefore has to be synthesized for the comparison. It is a
one-line flip of that single line (the `SPIRAM_USE_*` lines are inert while
`CONFIG_SPIRAM` is off — ESP-IDF ignores them, and `# CONFIG_X is not set` is
the canonical kconfig form, not deleting the line):

```
sed -i 's|^CONFIG_SPIRAM=y|# CONFIG_SPIRAM is not set|' firmware/sdkconfig.defaults
```

## 3. What was already done (no hardware needed)

Both configurations were built from `git archive HEAD` sandboxes at commit
`4138c2b9` (2026-09-05) so the live tree's unrelated in-flight edits could not
leak into the comparison. Both compile clean.

### 3.1 Toolchain actually used

| Item | Value | Note |
|---|---|---|
| ESP-IDF | `v5.2` = **5.2.0** (`/home/coding/esp-idf/esp-idf-v5.2`) | CI `spaxel-build` firmware leg pins `espressif/idf:v5.2` = 5.2.3. Patch difference only; if the two numbers ever disagree with CI sizes, re-run under CI before trusting either. |
| Toolchain | `xtensa-esp-elf` esp-13.2.0_20230928 | |
| Python env | `idf5.2_py3.12_env` | **Not** `idf5.2_py3.13_env` — see §3.2 |

### 3.2 Local build gotchas (both hit, both worked around)

1. `source export.sh` **aborts on ex44**: `openocd-esp32` cannot load
   `libusb-1.0.so.0` (NixOS), and export.sh treats the tool as uninstalled and
   exits 1 without exporting anything. openocd is JTAG-flash-only and
   irrelevant to a build. Workaround — eval what `idf_tools.py export` prints
   instead of sourcing export.sh:

   ```bash
   export IDF_TOOLS_PATH=/home/coding/.espressif
   export IDF_PATH=/home/coding/esp-idf/esp-idf-v5.2
   eval "$(python3 "$IDF_PATH/tools/idf_tools.py" export 2>/dev/null | grep '^export ' | tail -1)"
   ```

2. The default `idf5.2_py3.13_env` **crashes during sdkconfig generation**:
   `idf-component-manager`'s manifest schema builder hits
   `TypeError: expected string or bytes-like object, got 're.Pattern'` inside
   the `schema` package's `json_schema` (a Python 3.13 incompatibility). Use
   the 3.12 env:

   ```bash
   export IDF_PYTHON_ENV_PATH=/home/coding/.espressif/python_env/idf5.2_py3.12_env
   export PATH="/home/coding/.espressif/python_env/idf5.2_py3.12_env/bin:$PATH"
   ```

   A second IDF v5.2 checkout exists at `/home/coding/esp/esp-idf` (5.2.3, the
   version CI pins). It needs the same py3.12 env plus
   `LD_LIBRARY_PATH=/home/coding/.local/lib` for the same openocd reason. Both
   checkouts produce warning-free builds of this firmware; the §3.3 figures
   below come from the 5.2.0 one.

### 3.3 Static `idf.py size` results (measured, commit 4138c2b9)

| Metric | PSRAM off | PSRAM on | Δ (on − off) |
|---|---:|---:|---:|
| `spaxel-firmware.bin` | 0x121380 (1,182,336 B) | 0x122a70 (1,193,072 B) | +10,736 B |
| Total image size | 1,184,525 B | 1,190,405 B | +5,880 B |
| Used static IRAM | 16,383 B (1 B remain) | 16,383 B (1 B remain) | 0 |
| Used static D/IRAM | 114,451 B (231,405 remain, 33.1%) | 116,567 B (229,289 remain, 33.7%) | **+2,116 B** |
| — D/IRAM `.data` | 19,640 B | 19,772 B | +132 B |
| — D/IRAM `.bss` | 25,576 B | 25,632 B | +56 B |
| — D/IRAM `.text` | 69,235 B | 71,163 B | +1,928 B |
| Flash `.text` | 785,671 B | 788,179 B | +2,508 B |
| Flash `.rodata` | 293,340 B | 294,652 B | +1,312 B |
| App partition headroom | 42% free | 41% free | −1% |

Reading: enabling PSRAM **costs** ~2.1 KB of precious internal D/IRAM at
static-link time (esp_psram init, heap-descriptor and allocator code pulled
into internal RAM) plus ~11 KB of flash. That cost is the debit side of the
ledger; the credit side is the dynamic PSRAM pool, which only exists at
runtime and is what §6 measures.

Both images fit the `0x1f0000` app partition with ~41–42% headroom, so
partition pressure is not a factor in the ADR-008 decision.

## 4. The metric you will read already includes PSRAM — verified, not assumed

`free_heap_bytes` in node health (`firmware/main/websocket.c:571`) is
`esp_get_free_heap_size()`. In IDF v5.2 that is
`heap_caps_get_free_size(MALLOC_CAP_DEFAULT)` and — the part that matters —
with `CONFIG_SPIRAM_USE_MALLOC=y` (the setting at HEAD) the PSRAM heap region
is registered under `MALLOC_CAP_SPIRAM | MALLOC_CAP_DEFAULT`
(`components/esp_psram/esp_psram.c:305,315`). `MALLOC_CAP_DEFAULT` therefore
spans **both** pools, and the existing API field answers the acceptance
criterion as-is. No firmware or mothership instrumentation change is needed
for the measurement.

Optional refinement (not required, do not block on it): the field cannot tell
you *which* pool the bytes are in. If the ADR-008 discussion ends up needing
the split, the cheap addition is a second field carrying
`esp_get_free_internal_heap_size()` (= `MALLOC_CAP_8BIT | MALLOC_CAP_DMA |
MALLOC_CAP_INTERNAL`) so internal and PSRAM pools can be read apart. That is
a firmware+mothership change of its own and is deliberately not bundled here.

## 5. Expectation to check against — not a result

For orientation only: an ESP32-S3 with 2 MB embedded PSRAM (the part this
node reports to `esptool flash_id`: "Embedded PSRAM 2MB (AP_3v3)") typically
yields an extra pool on the order of 2 MB minus MMU/page-structure overhead,
while the internal free pool drops slightly (§3.3 static cost, plus runtime
page descriptors). **Treat this as a sanity band for the measurement, never as
the measurement.** If the measured delta lands wildly below that band, the
interesting failure modes are: PSRAM failed to initialise at boot (check
`esp_psram` log lines / `esp_psram_get_size()`), `SPIRAM_USE_MALLOC` not
actually selected in the flashed image (diff `build/sdkconfig` against
`sdkconfig.defaults` — the build dir copy is what shipped), or the node never
reached idle (a busy link or OTA in flight).

## 6. Board-time procedure (the part that still needs a human)

Flashing host: ex44 itself has no USB-serial device today. The sanctioned
path is the bench rig — `declarative-config/nixos/bench/modules/hardware-bench.nix`
documents `ssh bench esptool ...` (that node also carries the USB hub power
control and stable `/dev/serial/by-id` names). It was offline as of
2026-09-05, so a human needs to power it or attach a board before the steps
below can run.

Prerequisites: one ESP32-S3 node on USB serial, mothership reachable, node
provisioned to WiFi (dashboard "Add Node" / Chrome Web Serial, or
`idf.py -p <PORT> flash` after `idf.py menuconfig` holds the SSID).

```bash
# 0. Build each arm per §3 (PSRAM off = the §2 sed applied; on = HEAD as-is).
#    Flash arm A (PSRAM OFF), then provision + let it connect.

# 1. Reach idle: leave the node connected with no CSI workload for >= 60 s.

# 2. Read free_heap_bytes from the fleet API (repeat 3x, 10 s apart, and keep
#    the median — the health tick is 10 s and single samples jitter):
curl -s "$SPAXEL_API/api/nodes/<node-mac>" | jq '.free_heap_bytes'
#    record: PSRAM OFF  = ____ B  (median of 3)

# 3. Flash arm B (PSRAM ON, HEAD as-is), re-provision, wait >= 60 s, repeat:
#    record: PSRAM ON   = ____ B  (median of 3)

# 4. Delta = PSRAM ON − PSRAM OFF. That delta is the ADR-008 decision-5
#    number. Record it here, on the bead, and in the ADR-008 discussion.
```

Decision guidance (from the bead description): this is a measurement with a
decision attached. A large positive delta keeps the PKI/identity track; a
small or negative delta is a legitimate input that should send the design
back to the ADR-007 symmetric approach — that outcome is a success of the
gate, not a failed task.

## 7. Scope note

This file is documentation only — no firmware, mothership, or build-system
code was touched, so the repo-level `go test ./...` / `go vet ./...` gate is
not exercised by this change.
