# Firmware size-components capture — 2026-09-05

Capture of `idf.py size` and `idf.py size-components` for the spaxel ESP32-S3
firmware, produced by bead **spaxel-972d684f** ("Run size-components and
capture output").

## Provenance

| | |
|---|---|
| Built from | `git archive HEAD` sandbox — spaxel @ `b995e781fc7d90d6de31d78ed0b8469e6f219512` (tracked `firmware/` only; clean committed state, no dirty-tree input) |
| Target | esp32s3 (`idf.py set-target esp32s3`) |
| ESP-IDF | v5.2.3 (`IDF_PATH` = `~/esp/esp-idf`) |
| esp-idf-size | 1.7.1 |
| Toolchain | xtensa-esp-elf-gcc 13.2.0 (crosstool-NG esp-13.2.0_20230928) |
| cmake / ninja | 3.24.0 / 1.11.1 |
| Python env | `~/.espressif/python_env/idf5.2_py3.12_env` |
| Capture UTC | 2026-09-05T04:47:11Z |
| Result | `set-target` 0, `build` 0, `size` 0, `size-components` 0 — both JSON captures exit 0 |

Build log (full text incl. both tables and exit codes): sandbox
`~/scratch/spaxel-fwsize-20260905-003818/firmware/build.log`. Machine-readable
capture checked in next to this file:
[`size-components-2026-09-05.json`](./size-components-2026-09-05.json) — one
object per archive (62 archives) keyed by input section.

## Totals (`idf.py size`)

```
Used static IRAM:   16383 bytes (      1 remain, 100.0% used)
      .text size:   15356 bytes
   .vectors size:    1027 bytes
Used stat D/IRAM:  120807 bytes ( 225049 remain, 34.9% used)
      .data size:   20312 bytes
      .bss  size:   25576 bytes
      .text size:   74919 bytes
Used Flash size : 1080691 bytes
           .text:  793675 bytes
         .rodata:  286760 bytes
Total image size: 1192305 bytes (.bin may be padded larger)
```

- `spaxel-firmware.bin` 0x1231e0 bytes; smallest app partition 0x1f0000 →
  **0xcce20 bytes (41%) free**. Bootloader 0x6b70, 16% free.
- **IRAM is the tight segment: 16383/16384 used, 1 byte remain** — any new
  IRAM-resident code will need `noflash`/place-in-flash triage first.

## Per-archive breakdown (`idf.py size-components`, sorted by flash)

| Archive | Flash total | RAM (static) | .flash.text | .flash.rodata | .dram0.data | .dram0.bss |
|---|---:|---:|---:|---:|---:|---:|
| `libnet80211.a` | 130,298 | 11,994 | 104,532 | 20,966 | 938 | 7,194 |
| `libpthread.a` | 126,267 | 20 | 683 | 125,576 | 8 | 12 |
| `libmbedcrypto.a` | 108,887 | 464 | 74,269 | 34,422 | 116 | 268 |
| `liblwip.a` | 107,010 | 3,834 | 103,014 | 3,980 | 16 | 3,818 |
| `libmbedtls.a` | 94,147 | 686 | 28,146 | 65,975 | 26 | 660 |
| `libc.a` | 90,116 | 584 | 84,407 | 5,445 | 264 | 320 |
| `libwpa_supplicant.a` | 62,674 | 1,329 | 60,971 | 1,695 | 8 | 1,321 |
| `libpp.a` | 57,844 | 16,212 | 38,757 | 4,024 | 2,592 | 1,149 |
| `libphy.a` | 35,182 | 8,315 | 26,953 | 0 | 3,148 | 86 |
| `libespressif__mdns.a` | 34,181 | 2,058 | 33,231 | 902 | 48 | 2,010 |
| `libesp_hw_support.a` | 33,248 | 8,027 | 23,898 | 1,463 | 424 | 140 |
| `libmain.a` | 25,739 | 5,996 | 25,471 | 116 | 152 | 5,844 |
| `libfreertos.a` | 21,554 | 19,938 | 979 | 1,394 | 3,100 | 757 |
| `libhal.a` | 17,683 | 10,864 | 6,737 | 86 | 2,953 | 4 |
| `libdriver.a` | 17,670 | 141 | 16,404 | 1,146 | 120 | 21 |
| `libhttp_parser.a` | 16,117 | 0 | 15,365 | 752 | 0 | 0 |
| `libesp_system.a` | 16,016 | 5,089 | 10,567 | 657 | 476 | 297 |
| `libnvs_flash.a` | 15,526 | 24 | 15,278 | 248 | 0 | 24 |
| `libspi_flash.a` | 15,175 | 13,717 | 1,073 | 417 | 2,136 | 32 |
| `libesp_http_server.a` | 11,802 | 0 | 11,196 | 606 | 0 | 0 |
| `libheap.a` | 10,163 | 5,522 | 3,745 | 904 | 12 | 8 |
| `libcoexist.a` | 9,566 | 1,438 | 3,277 | 4,853 | 1,436 | 2 |
| `libtcp_transport.a` | 9,387 | 0 | 9,207 | 180 | 0 | 0 |
| `libesp_netif.a` | 8,701 | 37 | 8,115 | 578 | 8 | 29 |
| `libesp_http_client.a` | 8,687 | 0 | 8,396 | 291 | 0 | 0 |
| `libmbedx509.a` | 8,631 | 0 | 8,586 | 45 | 0 | 0 |
| `libespressif__esp_websocket_client.a` | 8,265 | 0 | 8,102 | 163 | 0 | 0 |
| `libesp-tls.a` | 7,026 | 4 | 6,934 | 92 | 0 | 4 |
| `libbootloader_support.a` | 6,327 | 812 | 3,942 | 1,577 | 0 | 4 |
| `libvfs.a` | 5,924 | 284 | 5,321 | 371 | 232 | 52 |
| `libjson.a` | 5,549 | 20 | 5,537 | 0 | 12 | 8 |
| `libesp_ringbuf.a` | 5,522 | 4,917 | 0 | 605 | 0 | 0 |
| `libesp_wifi.a` | 4,660 | 919 | 3,686 | 86 | 476 | 31 |
| `libefuse.a` | 3,929 | 424 | 3,528 | 341 | 60 | 364 |
| `libnewlib.a` | 3,800 | 1,957 | 1,948 | 95 | 164 | 200 |
| `libxtensa.a` | 3,708 | 3,541 | 119 | 48 | 1,060 | 0 |
| `libesp_event.a` | 3,675 | 4 | 3,512 | 163 | 0 | 4 |
| `libesp_mm.a` | 3,547 | 507 | 2,834 | 250 | 4 | 44 |
| `libesp_phy.a` | 3,402 | 219 | 2,904 | 314 | 8 | 35 |
| `libbt.a` | 3,260 | 257 | 3,151 | 49 | 60 | 197 |
| `libesp_psram.a` | 2,918 | 1,571 | 1,297 | 110 | 1 | 60 |
| `libesp_timer.a` | 2,805 | 1,264 | 1,473 | 104 | 32 | 36 |
| `libapp_update.a` | 2,713 | 12 | 2,617 | 96 | 0 | 12 |
| `libsoc.a` | 2,493 | 245 | 28 | 2,220 | 0 | 0 |
| `libesp_partition.a` | 2,024 | 8 | 1,835 | 189 | 0 | 8 |
| `libesp_common.a` | 1,793 | 0 | 51 | 1,742 | 0 | 0 |
| `libstdc++.a` | 1,553 | 21 | 1,342 | 207 | 4 | 17 |
| `liblog.a` | 1,283 | 557 | 959 | 39 | 8 | 272 |
| `libesp_rom.a` | 792 | 714 | 78 | 0 | 0 | 0 |
| `libesp_coex.a` | 601 | 245 | 356 | 0 | 72 | 0 |
| `libbtdm_app.a` | 475 | 41 | 438 | 0 | 0 | 4 |
| `libesp_app_format.a` | 444 | 10 | 184 | 4 | 0 | 10 |
| `libxt_hal.a` | 437 | 405 | 0 | 32 | 0 | 0 |
| `libcore.a` | 269 | 9 | 226 | 43 | 0 | 9 |
| `libgcc.a` | 98 | 0 | 98 | 0 | 0 | 0 |
| `libm.a` | 60 | 0 | 60 | 0 | 0 | 0 |
| `libcxx.a` | 47 | 0 | 47 | 0 | 0 | 0 |
| `(exe)` | 14 | 3 | 3 | 8 | 0 | 0 |
| `libnvs_sec_provider.a` | 5 | 0 | 5 | 0 | 0 | 0 |
| `libesp_eth.a` | 4 | 0 | 0 | 4 | 0 | 0 |
| `libbtbb.a` | 0 | 0 | 0 | 0 | 0 | 0 |
| `libmesh.a` | 0 | 0 | 0 | 0 | 0 | 0 |

| **Σ 62 archives** | **1,181,693** | **135,259** | | | | |


The two headline numbers measure different things, and both reconcile
exactly:

- **Per-archive Σ `flash_total` = 1,181,693** = flash `.text` 785,872 +
  `.rodata` 285,673 + `.appdesc` 256 + IRAM `.text` 89,291 + vectors 427 +
  `.dram0.data` 20,174 — i.e. every byte the archive contributes to the flash
  *image*, including code that executes from IRAM and `.data` initializers,
  which are stored in flash and copied out at boot.
- **`idf.py size` Used Flash = 1,080,691** = ELF `.text` 793,675 + `.rodata`
  286,760 + `.appdesc` 256 only. (793,675 + 286,760 + 256 = 1,080,691.)

The remaining deltas between the per-archive columns and the ELF totals
(~7.8 KB of `.text`, ~1.1 KB of `.rodata`) are linker-generated filler, stubs
and alignment padding that belong to no archive; and the `Total image size`
(1,192,305) is the per-archive sum plus ~10.6 KB of image and segment headers
plus that padding. `.flash.rodata_noload` (1,731) is excluded from
`flash_total`. The text table in `build.log` carries the additional columns
(`.rodata_noload`, `.appdesc`) per archive.

## Reading the numbers — where the firmware actually goes

The spaxel application is `libmain.a`: **25,739 flash bytes (2.2%) and 5,996
bytes of static RAM**. Everything else is ESP-IDF and managed components, so
app-level size work moves the needle far less than component/Kconfig choices:

| Driver | Archive | Flash | Share |
|---|---|---:|---:|
| WiFi (net80211) | `libnet80211.a` | 130,298 | 11.0% |
| TLS crypto | `libmbedcrypto.a` | 108,887 | 9.2% |
| TCP/IP | `liblwip.a` | 107,010 | 9.1% |
| TLS X.509 | `libmbedtls.a` | 94,147 | 8.0% |
| libc | `libc.a` | 90,116 | 7.6% |
| WiFi supplicant | `libwpa_supplicant.a` | 62,674 | 5.3% |
| WiFi (pp) | `libpp.a` | 57,844 | 4.9% |
| **spaxel app** | `libmain.a` | **25,739** | **2.2%** |

## Observations flagged for downstream analysis

1. **The 125 KB `.flash.rodata` blob attributed to `libpthread.a` is the
   whole-firmware merged string pool, misattributed by the map — not pthread
   code.** `esp_idf_size` reports `libpthread.a` = 126,267 flash bytes (10.7%
   of all flash), of which 125,576 is `.flash.rodata`. The map shows it is a
   single output section
   (`.rodata.pthread_cleanup_thread_specific_data_callback.str1.4` at
   `0x3c0d0120`, size `0x1ea5a`) attributed to
   `pthread_local_storage.c.obj`, whose own input section was `0x3d` (61
   bytes) before relaxing. Reading the image confirms what it really is: the
   blob at bin offset `0x118` is 90.7% printable ASCII across 2,906
   NUL-terminated strings owned by *other* archives — spaxel's own logs
   (`SPAXEL Firmware starting...`, `Arming CSI for role %s`), esp_system's
   `cpu_start` strings, `ESP_ERR_*` name tables, and ANSI-color-wrapped
   `ESP_LOG` format strings. GNU ld merges every input `.rodata.str1.4`
   (mergeable-string) section and reports the merged output under the first
   contributor, so `size-components` charges the entire pool to libpthread
   and understates every other archive's rodata.
   **The actionable lever this exposes:** ~125 KB — 10.7% of flash — is log
   and assertion *string* payload, so `CONFIG_LOG_DEFAULT_LEVEL`,
   `CONFIG_COMPILER_OPTIMIZATION_ASSERTIONS`, and per-component log levels
   buy far more flash than any app-code change (the app is 2.2%). Treat
   per-archive `.flash.rodata` columns in the table above as distorted by
   this pooling; `.flash.text` and RAM columns are unaffected.
2. **IRAM headroom is effectively zero** (16,383/16,384, 100.0% used) — see
   totals above. New IRAM code or an IRAM-resident optimization must come with
   a placement plan.
3. **`ESP_TASK_WDT_TIMEOUT_S=150` is outside Kconfig range [1,60]** — the
   build emits `warning: user value 150 ... ignored due to being outside the
   active range ([1, 60]) -- falling back on defaults`. Already documented and
   deliberately unresolved: commit `4138c2b9` (bead spaxel-b2a66fb7) records
   that the line has never been effective, that the runtime window is the 90s
   `SPAXEL_WATCHDOG_TIMEOUT_S` armed by `watchdog_init()` via
   `esp_task_wdt_reconfigure()`, and that fixing the line belongs to the
   in-flight symbol-correctness changeset that owns `sdkconfig.defaults`. This
   capture is independent build-time confirmation of that warning on
   ESP-IDF v5.2.3.
4. Nimble remains removed — compare with
   [`nimble-savings.md`](./nimble-savings.md) (the WiFi+TLS stacks above are
   the same footprint class that motivated it).

## Tree freshness

`main` advanced during this run: `b995e781` → `83d08d5c` (watchdog doc
corrections `4138c2b9` — comment-only, no code paths — plus a version bump and
a notes doc). None of it changes a size input materially, and the capture is
pinned to the `b995e781` `firmware/` tree either way; re-run the recipe above
against a newer tree to refresh.

## Reproducing

The build environment on ex44 is NOT what `export.sh` gives you (openocd has
no libusb on NixOS and aborts the export; the `idf5.2_py3.13_env` is broken —
old idf-component-manager vs Python 3.13). Working recipe, from a scratch dir
holding `firmware/` (e.g. a `git archive HEAD firmware | tar -x` sandbox):

```bash
export IDF_PATH=$HOME/esp/esp-idf
export IDF_TOOLS_PATH=$HOME/.espressif
export ESP_ROM_ELF_DIR=$HOME/.espressif/tools/esp-rom-elfs/20230320/
export IDF_PYTHON_ENV_PATH=$HOME/.espressif/python_env/idf5.2_py3.12_env
export PATH=$IDF_PYTHON_ENV_PATH/bin:$IDF_PATH/tools:$PATH
# plus the xtensa/riscv32/ulp toolchain bins and cmake 3.24.0 / ninja 1.11.1
# under $IDF_TOOLS_PATH/tools (see .espressif/tools/*)

cd firmware
idf.py set-target esp32s3
idf.py build
idf.py size
idf.py size-components                 # text table
idf.py size-components --format json   # machine-readable; idf.py interleaves
                                       # its own log lines on stdout, so skip
                                       # to the first line that is just a brace
```

CI equivalent: the `spaxel-fwverify` WorkflowTemplate (espressif/idf:v5.2)
proves the build leg; capturing sizes there would mean appending the two
`idf.py size` steps to the firmware-build node.
