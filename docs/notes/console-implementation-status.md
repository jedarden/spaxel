# Console Defaults Implementation Status

**Task:** spaxel-5049d982 - "Console defaults to unconnected UART0 — USB-Serial/JTAG boards boot silently"

**Date:** 2026-08-23

## Summary

The ESP32-S3 console configuration work is **IMPLEMENTED but NOT VERIFIED**. The default build now correctly outputs to USB-Serial/JTAG, making all boot logs and diagnostics visible on native-USB boards. However, the critical final step—confirming panic output is actually visible—has not been performed on hardware.

## What Was Done (✓ Complete)

### 1. Board-Variant Console Profiles

**File: `firmware/sdkconfig.usbjtag`** (ESP32-S3 default)
```ini
CONFIG_ESP_CONSOLE_USB_SERIAL_JTAG=y
CONFIG_ESP_CONSOLE_UART_DEFAULT=n
CONFIG_ESP_CONSOLE_SECONDARY_NONE=y
CONFIG_ESP_SYSTEM_PANIC_PRINT_REBOOT=y
```

**File: `firmware/sdkconfig.uart-console`** (Bridge-equipped boards)
```ini
CONFIG_ESP_CONSOLE_UART_DEFAULT=y
CONFIG_ESP_CONSOLE_USB_SERIAL_JTAG=n
CONFIG_ESP_CONSOLE_SECONDARY_USB_SERIAL_JTAG=y
CONFIG_ESP_SYSTEM_PANIC_PRINT_REBOOT=y
```

### 2. Build System Integration

**File: `firmware/CMakeLists.txt` (lines 55-57)**
```cmake
if(NOT DEFINED SDKCONFIG_DEFAULTS)
    set(SDKCONFIG_DEFAULTS "sdkconfig.defaults;sdkconfig.usbjtag")
endif()
```

This makes USB-Serial/JTAG the **shipped default** for ESP32-S3. UART0 boards override via:
```bash
idf.py -D SDKCONFIG_DEFAULTS="sdkconfig.defaults;sdkconfig.uart-console" build
```

### 3. Verification Infrastructure

- **Script:** `firmware/scripts/verify-console-config.sh` — Validates generated `sdkconfig` has correct settings
- **Test:** `firmware/test/test_console_config.c` — Host-based tests ensuring defaults are committed correctly
- **Both profiles** explicitly set `CONFIG_ESP_SYSTEM_PANIC_PRINT_REBOOT=y`

### 4. Documentation

**File:** `firmware/README.md` (lines 192-272)

- Default native-USB build instructions (USB-Serial/JTAG)
- UART0 override instructions for bridge-equipped boards
- Console panic check procedure documented (lines 261-271)

## What's Missing (❌ Not Done)

### Critical Gap: Panic Output Never Verified on Hardware

The third acceptance criterion requires:
> "Panic output (backtrace) is confirmed visible on the chosen console — deliberately trigger a fault once to verify, since diagnosing future field crashes depends on it."

**Current State:**
- ✅ Panic output **configured** (`CONFIG_ESP_SYSTEM_PANIC_PRINT_REBOOT=y` in both profiles)
- ✅ Test procedure **documented** in README.md (add `esp_system_abort("console panic probe")`)
- ❌ Panic output **never verified** on actual hardware
- ❌ No captured backtrace output in notes/docs
- ❌ No confirmation that USB-Serial/JTAG peripheral actually carries panic text

**Why This Matters:**

Without this verification, we risk:
1. **Silent crashes in production** — A node could panic and reboot without any diagnostic output, leaving field crashes undiagnosable
2. **Configuration correct, peripheral broken** — The ESP-IDF config might be right but the USB peripheral might not flush output during a crash
3. **Timing issues** — Panic handler might not complete output before reset

## How To Verify (When Hardware Is Available)

Per the documented procedure in `firmware/README.md` lines 261-271:

### Step 1: Add Panic Probe

**File:** `firmware/main/main.c`

Add immediately after the first `app_main` log line:
```c
ESP_LOGI(TAG, "System starting...");
esp_system_abort("console panic probe");  // TEMPORARY: remove after verification
```

### Step 2: Build and Flash

```bash
cd firmware
rm -rf build sdkconfig sdkconfig.old
idf.py set-target esp32s3
idf.py build

# Verify console selection before flashing
./scripts/verify-console-config.sh build/config/sdkconfig usb

idf.py -p /dev/ttyACM0 flash
```

### Step 3: Capture and Verify

```bash
idf.py -p /dev/ttyACM0 monitor
```

**Expected Output:**
```
I (xxx) main_task: Started on CPU0
I (xxx) main_task: System starting...
Guru Meditation Error: Core 0 panic'ed (Abort at 0x4013xxxxx)
Backtrace: 0x4013xxxxx:0x4014xxxxx 0x4014xxxxx 0x4013xxxxx
...
```

### Step 4: Remove Probe and Rebuild

```c
// Remove this line immediately after verification!
// esp_system_abort("console panic probe");
```

Rebuild clean and flash the production image.

## Acceptance Criteria Status

| Criterion | Status | Evidence |
|-----------|--------|----------|
| Clean build prints full boot log on USB-Serial/JTAG | ✅ CONFIG correct, but not flash-verified | Commit `af303e6` + build config |
| UART0 console reachable via documented override | ✅ Complete | Profile exists + documented |
| Panic output confirmed visible on hardware | ❌ NOT DONE | No verification record exists |

## Recommendation

**DO NOT CLOSE THIS BEAD YET.** The implementation is correct, but the critical hardware verification step is missing. Without confirming that panic output actually appears on the console, we risk deploying nodes that crash silently in the field.

**Required Action:**
When ESP32-S3 hardware is available, perform the panic probe verification per the steps above. Document the captured backtrace in this directory (e.g., `panic-verification-esp32s3.md`) with timestamp and board ID.

**Alternative:** If no hardware is available, this gap must be accepted as a known limitation with a TODO to verify on first hardware bring-up.
