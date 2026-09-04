# ESP32-S3 Firmware Code Directories — Survey

**Date:** 2026-09-03
**Bead:** spaxel-62c8ab42
**Method:** extension scan (`.c .cpp .h .hpp .cc .cxx .ino .S`) over the tracked tree and the
untracked working tree, plus build-system marker search (`platformio.ini`, `sdkconfig*`,
`partitions*.csv`, `CMakeLists.txt`, `idf_component.yml`, `dependencies.lock`, `Kconfig*`).
Target chip confirmed from `sdkconfig` and the CMake build cache.

## Result

There is **exactly one firmware code tree: `firmware/`** — an ESP-IDF v5.2 C project for the
ESP32-S3. No Arduino/PlatformIO project exists anywhere in the repository, and no C, C++ or
assembly source is tracked outside `firmware/`. Everything else is mothership Go, dashboard
JS, or tooling.

All 34 git-tracked C/H files in the repository live in `firmware/main/` (24) and
`firmware/test/` (10). `git ls-files '*.c' '*.h' '*.cpp' '*.cc' '*.cxx' '*.S'` returns
nothing else.

## Firmware directories

| Directory | Role | C/H files | Tracked | Evidence |
|---|---|---|---|---|
| `firmware/` | ESP-IDF project root | — | yes (13 root files) | `CMakeLists.txt` (`project(spaxel-firmware)`), `partitions.csv`, `sdkconfig.defaults` + two board variants (`sdkconfig.uart-console`, `sdkconfig.usbjtag`), `dependencies.lock` |
| `firmware/main/` | First-party application source — **the firmware** | 12 `.c` + 12 `.h` = 5,154 lines | yes | `idf_component_register(SRCS ...)` lists all 12 translation units; `REQUIRES esp_wifi esp_netif nvs_flash esp_http_client esp_timer bt driver log esp_http_server mbedtls app_update json freertos esp_system`; `-Werror` |
| `firmware/test/` | Host-based gcc test harness (no ESP-IDF) | 8 `test_*.c` + `test_runner.c/.h` | yes | `Makefile` (`make -C firmware/test test`); rationale in `docs/notes/firmware-host-test-approach.md` |
| `firmware/managed_components/` | Vendored ESP-IDF components (third-party) | 34 `.c` + 23 `.h` | **no** (gitignored, fetched by component manager) | `dependencies.lock`: `espressif/esp_websocket_client` 1.8.0, `espressif/mdns` |
| `firmware/build/` | CMake build output — artifacts, not source | 8 `.c` + 3 `.h` (generated) | **no** (gitignored) | 199 MB; `spaxel-firmware.bin/.elf`, `bootloader/`, `partition_table/`, `esp-idf/` prebuilt libs |
| `firmware/scripts/` | Firmware signing + console verification shell scripts | 0 | yes | `generate-signing-key.sh`, `sign-firmware.sh`, `verify-console-config.sh` |
| `firmware/docs/` | Firmware notes | 0 | yes | `nimble-savings.md`, `bluedroid-baseline.txt` |

### Key files in `firmware/main/` (by size)

| File | Lines | Responsibility |
|---|---|---|
| `websocket.c` | 1,203 | node↔mothership WebSocket protocol **and the OTA download path** |
| `wifi.c` | 682 | station connect, mDNS discovery, captive portal AP |
| `main.c` | 584 | boot + node state machine (`BOOT`/`DISCOVERY`/`CONNECTED`/`WIFI_LOST`/`CAPTIVE_PORTAL`) |
| `csi.c` | 350 | promiscuous mode, CSI callback, 24-byte binary frame serialization |
| `provision.c` | 323 | 10 s serial provisioning window, bounded JSON parser (UART + USB-Serial/JTAG) |
| `ble.c` | 262 | NimBLE passive advertisement scan |
| `safe_mode.c` | 258 | boot-loop safe mode |
| `nvs_migration.c` | 216 | NVS schema versioning/migration |
| `ntp.c` | 172 | SNTP sync for TX slot staggering |
| `transport.c` | 165 | transport selection (ws/wss, cert bundle) |
| `led.c` | 129 | identify blink / status LED |
| `watchdog.c` | 68 | task watchdog |
| (+ 12 `.h`, `version.h.in`, `idf_component.yml`, `CMakeLists.txt`) | | |

## ESP32-S3 target evidence

- `firmware/sdkconfig.defaults:5` and `firmware/sdkconfig`: `CONFIG_IDF_TARGET="esp32s3"`
- `firmware/build/project_description.json`: `idf_path: /home/coding/esp-idf/esp-idf-v5.2`, `project_name: spaxel-firmware` → **ESP-IDF v5.2**
- `firmware/main/` includes 19 distinct `esp_*.h` headers (`esp_wifi.h`, `esp_nimble_hci.h`, `esp_ota_ops.h`, `esp_http_client.h`, `esp_sntp.h`, `esp_task_wdt.h`, `esp_crt_bundle.h`, …)
- `firmware/partitions.csv`: true A/B — `ota_0` + `ota_1`, each 0x1F0000 B on a 4 MB flash, `nvs`/`phy_init`/`otadata`; `factory` deliberately dropped (documented in the file, confirmed on hardware 2026-07-30)
- `firmware/sdkconfig.defaults`: `CONFIG_ESP_WIFI_CSI_ENABLED=y`, `CONFIG_BT_ENABLED=y` + `CONFIG_BT_NIMBLE_ENABLED=y`, `CONFIG_SPIRAM=y`, `CONFIG_BOOTLOADER_APP_ROLLBACK_ENABLE=y` + `CONFIG_APP_ANTI_ROLLBACK=y`, `CONFIG_PARTITION_TABLE_CUSTOM=y` → `partitions.csv`
- `CMakeLists.txt:56` layers `SDKCONFIG_DEFAULTS "sdkconfig.defaults;sdkconfig.usbjtag"` (ADR-002 console routing; `sdkconfig.uart-console` is the bridge-board variant)

**Not configured** (contrary to the 2026-08-29 inventory's "Security Features" section):
no `CONFIG_SECURE_BOOT*` or `CONFIG_SECURE_FLASH_ENC_ENABLED` line in `sdkconfig.defaults`
or `sdkconfig`. Anti-rollback is set; secure boot and flash encryption are not.

## Markers checked and absent

| Marker | Result |
|---|---|
| `platformio.ini` | none, anywhere |
| `*.ino` (Arduino sketch) | none, anywhere |
| `.cpp` / `.cc` / `.cxx` / `.S` | none tracked |
| Second firmware tree | none — ESP32 markers outside `firmware/` appear only in docs, the Dockerfile, `cmd/sim` (Go simulator), `dashboard/js/esptool-bundle.js` (browser-side flashing), and beads metadata |

## Directories that are NOT firmware code (commonly mistaken)

- **`data/firmware/`** — empty. Runtime OTA seed store (the `SPAXEL_SEED_FIRMWARE_DIR`
  target that the mothership seeds into `/data/firmware/`), never source.
- **`tmp/acc-repro/`** — disposable scratch copy of the whole repository, including a
  duplicate `firmware/` tree. Scratch, per the workspace file-organization rules; excluded
  from this survey and not a second project.
- **`dashboard/js/esptool-bundle.js`, `dashboard/js/onboard.js`** — browser JS that talks
  *to* the ESP32 over Web Serial; not firmware.
- **`cmd/sim/`** — Go simulator that *emulates* ESP32 nodes; not firmware.
- **`mothership/`, `test/`, `tests/`** — Go modules and harnesses.

## Plan-vs-code deltas (for the next bead)

1. `docs/plan/plan.md` "Firmware Build System" lists `main/{ota.c,nvs.c,serial_prov.c,sntp.c,ws.c}`.
   Actual: OTA lives in `websocket.c`, NVS in `nvs_migration.c`, serial provisioning in
   `provision.c`, SNTP in `ntp.c`, WS client in `websocket.c`.
2. `plan.md` partition layout (`factory` 4 MB + `ota_0` + `ota_1`, 16 MB flash) is stale:
   the real `partitions.csv` drops `factory`, uses 4 MB flash with 2,031,616 B slots, and
   documents why (A/B cannot fit a factory image; bootloader rollback covers a bad slot).
3. `plan.md`'s Dockerfile builds ESP-IDF inline (`espressif/idf:v5.2` stage). The current
   `Dockerfile` has a `firmware-fetcher` stage that downloads prebuilt
   `spaxel-firmware[-merged]-<VERSION>.bin` from GitHub Releases instead (ADR-001
   direction). `firmware/` remains the build input for the CI firmware-build step.
4. `plan.md`'s sample `sdkconfig.defaults` uses old key names
   (`CONFIG_ESP32S3_SPIRAM_SUPPORT`, `CONFIG_ESP_WIFI_PROMISCUOUS_FILTER`) that do not match
   the committed file (`CONFIG_SPIRAM`, `CONFIG_ESP_WIFI_CSI_ENABLED`).

## Hygiene notes

- `firmware/test/test_runner` is a **tracked ELF binary** (mode 100755,
  blob `ee3f74e0`) — compiled host-test output committed to git. Candidate for
  `firmware/test/.gitignore`.
- `firmware/build/` is 199 MB on disk (gitignored). Any extension scan that does not
  exclude it will over-count generated sources; `firmware/test/build/` is likewise
  gitignored via `firmware/test/.gitignore`.
- `firmware/sdkconfig` and `firmware/sdkconfig.old` are generated and gitignored; only
  `sdkconfig.defaults` and the two board-variant files are authoritative.

## Related docs

- `docs/research/firmware-directories-inventory.md` — 2026-08-29 directory-centric listing
  (counts `build/` artifact subdirectories as "firmware directories"; 14 total)
- `docs/research/firmware-directory-search-results.md` — 2026-08-28 binary/ELF inventory
- `docs/notes/firmware-host-test-approach.md` — why the host harness does not link `firmware/main/*.c`
- `docs/research/programming-languages-inventory.md` — language distribution across the repo
- `firmware/BUILD.md` — build/flash instructions (Docker-based; ESP-IDF v5.2 at
  `/home/coding/esp-idf/esp-idf-v5.2/`)
