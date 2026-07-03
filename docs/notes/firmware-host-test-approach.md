# Firmware Host-Based Test Approach — Decision Record

**Bead:** bf-21t (decision spike; parent harness bead: bf-4ne)
**Date:** 2026-07-03
**Decision:** **gcc host harness** (plain gcc-compiled test runner under `firmware/test/`,
*not* the ESP-IDF `--target linux` / Unity-host path).

This is a decision spike only. It records *why* the ESP-IDF linux target was rejected
so that the harness build-out (bf-4ne) does not have to re-derive it. The chosen
approach and the reason below **must be carried into bf-4ne's test-runner header
comment**.

---

## TL;DR

`firmware/main/*.c` cannot be compiled for the ESP-IDF `linux` target because the
core files pull in **hardware-radio / peripheral drivers that have no host build**:

- `csi.c` → `esp_wifi.h` (CSI callback + promiscuous-mode API) — no host build.
- `provision.c` → `driver/uart.h` (UART provisioning window) — no host build.

The firmware is built as a single `main` component, so the whole component must
link for any one file to be host-tested via `idf.py --target linux`. It doesn't, so
the linux target is impractical. Fall back to a gcc harness that **does not link
`firmware/main/*.c` directly** — it tests pure-logic extractions and binary-format
contracts instead.

---

## Investigation

ESP-IDF was not installed on the investigation host (`IDF_PATH` unset, no `idf.py`),
so this is reasoned from the source plus the documented ESP-IDF `linux`-target
component matrix (IDF 5.2.x, the version pinned for this project). gcc 14.2 is
available, which the fallback path requires.

### What the ESP-IDF `linux` target provides

Host builds exist for the "hostable" components:
`nvs_flash`/`nvs` (file-backed host emulation — the canonical IDF host-test
example), `esp_log`, `esp_timer`, `esp_event`, `freertos` (POSIX/SMP-on-host
port — so `freertos/task.h`, `queue.h`, `event_groups.h`, `semphr.h` resolve),
`cJSON` (pure C), and partial `esp_netif`/`mbedtls`.

What it does **not** provide (hardware/peripheral-only, target-arch):
`esp_wifi` (depends on the radio MAC/baseband binary blob), the entire
`driver/*` namespace (`uart`, `gpio`, `temperature_sensor`, …), `esp_bt`/BLE,
`esp_http_server`/`esp_http_client`/`esp_websocket_client`, `esp_ota_ops`,
`mdns`, `lwip` (as these files use it).

### Per-file verdict for the three target files

| File | Non-hostable include | Specific symbols used | linux target |
|------|----------------------|-----------------------|--------------|
| `nvs_migration.c` | — (only `nvs`, `esp_log`, `spaxel.h`) | `nvs_open/get_str/set_str/commit/erase_key`, `nvs_handle_t` | ✅ feasible *in isolation* |
| `csi.c` | `esp_wifi.h` | `wifi_csi_info_t`, `wifi_csi_config_t`, `esp_wifi_set_csi_config`, `esp_wifi_set_csi_rx_cb`, `esp_wifi_set_csi`, `esp_wifi_set_promiscuous` | ❌ **blocked** |
| `provision.c` | `driver/uart.h` | `uart_param_config`, `uart_driver_install`, `uart_write_bytes`, `uart_read_bytes`, `uart_driver_delete`, `UART_NUM_0`, `uart_config_t`, `UART_DATA_8_BITS` | ❌ **blocked** |

### Why the lone feasible file doesn't rescue the approach

`nvs_migration.c` is the one file whose dependency set (`nvs` + `esp_log` +
`freertos`, transitively via `spaxel.h`) is entirely hostable — NVS has a real
file-backed host build. So *in isolation* it could be run through the IDF linux
target. But:

1. The firmware compiles `main` as **one component** —
   `firmware/main/CMakeLists.txt`'s `idf_component_register(...)` lists every
   `*.c` as SRCS and declares
   `REQUIRES esp_wifi esp_netif nvs_flash esp_http_client esp_timer bt driver log esp_http_server mbedtls app_update json freertos`
   (verified verbatim in the tree). That `REQUIRES` line itself names `esp_wifi`,
   `bt`, and `driver` — three components with **no `linux` build** — so the `main`
   component cannot even be configured for the host target, regardless of which
   translation unit you care about. The IDF host-test model builds whole
   components, not cherry-picked TUs; `csi.c`, `wifi.c`, `websocket.c`, `ble.c`,
   `provision.c`, `led.c`, `ntp.c` all drag in unhostable drivers.
2. Making `main` linkable for the host would mean wrapping every driver include
   in host/target `#ifdef` guards or splitting hardware code into a separate
   component — i.e. refactoring production firmware purely to satisfy host
   tests, which risks the on-target build and defeats the purpose.
3. Even if it worked, you would then run **two different test mechanisms** (IDF
   linux for nvs, gcc for csi/prov). A single coherent harness is simpler and is
   what the parent bead (bf-4ne) asks for.

Conclusion: the ESP-IDF linux target is **rejected** as the unified harness
approach.

---

## Decision: gcc host harness under `firmware/test/`

A plain gcc-compiled test runner that:

- Does **not** `#include` or link `firmware/main/*.c` directly (they can't compile
  without the ESP-IDF target toolchain).
- Instead tests:
  - **Pure-logic extractions** — small, dependency-free pieces of behavior copied
    into separately-compilable test units (e.g. the Welford amplitude-variance in
    `csi.c`, the JSON→NVS-key mapping in `provision.c`, the migration-step logic
    in `nvs_migration.c`), *or* refactored into `*_logic.c` files with zero `esp_`
    includes that both the firmware and the harness can compile.
  - **Binary-format contracts** — the 24-byte CSI frame layout and the
    provisioning JSON schema, validated by re-implementing a reference
    encoder/decoder in the test.
- Uses assert-style macros (or Unity host) — either is fine; the key constraint is
  "compiles with plain gcc, no ESP-IDF toolchain."
- Is wired so adding new `test_*.c` files later is trivial (convention-based or a
  single `SOURCES` list), per bf-4ne.

### What this does NOT cover (accepted trade-off)

The gcc harness cannot exercise the actual `esp_wifi`/`uart`/`nvs` call sites —
those remain validated on-target (and via the Go-side `spaxel-sim` acceptance
suite, which already drives the full node→mothership contract host-free). The
firmware host tests are a *logic-and-format* safety net, not a hardware test.

---

## Carry-forward into child 2 (bf-4ne — the harness build-out)

bf-4ne must, in its test-runner header comment, state:

1. **Chosen approach:** gcc host harness.
2. **Concrete reason:** `csi.c` is blocked by `esp_wifi.h` and `provision.c` by
   `driver/uart.h`; neither has an ESP-IDF `linux`-target build, so
   `firmware/main/*.c` cannot be linked directly and the Unity/`idf.py --target
   linux` path was rejected. (Even `nvs_migration.c`, the lone hostable file,
   is blocked because `main` builds as one component.)
3. **The single documented run command** for the harness.

This doc (`docs/notes/firmware-host-test-approach.md`) is the durable reference
for that comment.
