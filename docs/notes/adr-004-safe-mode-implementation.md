# Safe Mode Implementation (ADR-004 Extension)

## Overview

This document extends ADR-004 with ESPHome-style safe mode functionality that provides an additional recovery layer beyond bootloader rollback.

## Problem Statement

Bootloader rollback only catches images that **fail to boot**. It does nothing for the more common and dangerous case: an image that boots, connects, and is broken. The existing health gate makes this worse — it marks the slot valid on the first role message (bf-5vwo8), so a broken build can actively cancel its own rollback.

## Solution: ESPHome-Style Safe Mode

Safe mode converts "soft-bricked node behind a wall" into "node that is still reachable for another OTA" without requiring additional partition space or flash budget.

### Architecture

```
Boot → Increment Counter → Start Validation Timer
     ↓
     Stay up for 60s? → Yes → Reset Counter (boot is good)
     ↓
     No (crash/validation fail) → Increment Counter
     ↓
     Counter >= 10? → Yes → Boot into Safe Mode (network + OTA only)
     ↓
     Safe Mode: 5 min timeout → Reboot to normal mode
```

### Key Components

#### 1. Boot Failure Counter (NVS Persistent)

- **Storage:** NVS key `NVS_KEY_BOOT_COUNTER` (uint32_t)
- **Scope:** Persists across reboots and power cycles
- **Reset:** Cleared on successful boot validation

#### 2. Boot Validation Window

- **Duration:** `SAFE_MODE_BOOT_GOOD_AFTER_S = 60` seconds
- **Trigger:** Starts after OTA validation confirms firmware is stable
- **Completion:** Marks boot as good, resets failure counter to 0

#### 3. Safe Mode Activation

- **Threshold:** `SAFE_MODE_BOOT_COUNT_THRESHOLD = 10` consecutive failures
- **Flag:** NVS key `NVS_KEY_SAFE_MODE` (u8: 0=disabled, 1=enabled)
- **Behavior:** Disables CSI, BLE, health reporting; enables only network + OTA + serial logging

#### 4. Safe Mode Exit

- **Timeout:** `SAFE_MODE_REBOOT_TIMEOUT_S = 300` seconds (5 minutes)
- **Action:** Automatic reboot to attempt normal boot
- **Manual:** Can also exit via `safe_mode_exit()` API

### Integration Points

#### Boot Sequence (`main.c:app_main`)

```c
1. NVS initialization
2. NVS schema migration
3. safe_mode_init() ← Load counter, check safe mode flag
4. WiFi initialization (always enabled)
5. CSI/BLE initialization (skipped if safe mode active)
6. WebSocket initialization (always enabled for OTA)
7. State machine and health tasks
```

#### OTA Validation (`websocket.c`)

```c
confirm_ota_valid():
  - Mark OTA partition valid (existing)
  - Start boot-good timer (NEW) ← 60s countdown to mark boot good

ota_validation_timeout_cb():
  - Mark boot as failed (NEW) ← Increment counter, enter safe mode
  - Reboot for rollback (existing)
```

### Watchdog Coordination (CRITICAL)

**⚠️ Interaction Warning (bf-2tgcx):**

ESPHome 2026.4.0 shipped a regression where the Task Watchdog reset devices **before** the 60-second boot-validation window closed, spuriously triggering OTA rollback (esphome/esphome#15767).

Spaxel has the same shape:
- 60-second role-message validation window
- 60-second safe mode boot-good window
- Watchdog about to be enabled

**Requirement:** Watchdog timeout MUST be greater than validation window + boot-good window combined.

**Recommended configuration:**
```
# Task watchdog timeout (CONFIG_ESP_TASK_WDT_TIMEOUT_S)
# Must be > 120s to cover both validation phases
CONFIG_ESP_TASK_WDT_TIMEOUT_S=150  # 2.5 minutes minimum
CONFIG_ESP_TASK_WDT_CHECK_IDLE_TASK_CPU=n
```

**Validation timeline:**
```
0s:     Boot starts, OTA validation timer armed
60s:    OTA validation completes → boot-good timer starts
120s:   Boot-good timer fires → boot marked good, counter reset
150s:   Earliest safe watchdog reset point
```

### API Reference

#### Initialization
```c
esp_err_t safe_mode_init(void);
```
Loads boot counter and safe mode flag from NVS. Must be called early in boot.

#### State Query
```c
bool safe_mode_is_active(void);
```
Returns true if currently in safe mode.

#### Boot Outcome
```c
esp_err_t safe_mode_mark_boot_good(void);   // Boot succeeded
esp_err_t safe_mode_mark_boot_failed(void); // Boot failed
```
Marks boot outcome and updates counter. Auto-enters safe mode at threshold.

#### Manual Control
```c
esp_err_t safe_mode_enter(void);  // Force safe mode next boot
esp_err_t safe_mode_exit(void);   // Exit safe mode
```
For mothership-initiated safe mode entry/exit.

#### Timers
```c
esp_err_t safe_mode_start_boot_good_timer(void);  // 60s auto-confirm
esp_err_t safe_mode_stop_boot_good_timer(void);
esp_err_t safe_mode_start_exit_timer(void);      // 5min safe mode reboot
```

### Configuration Constants

All defined in `safe_mode.h`:

```c
#define SAFE_MODE_BOOT_COUNT_THRESHOLD 10    // Failures before safe mode
#define SAFE_MODE_BOOT_GOOD_AFTER_S 60       // Validation window
#define SAFE_MODE_REBOOT_TIMEOUT_S 300       // Safe mode exit timeout
```

### Interaction with Existing Rollback

Safe mode complements, does not replace, existing rollback mechanisms:

1. **Bootloader rollback** (automatic): Unconfirmed partition → rollback on reboot
2. **Health timeout** (main.c:465-488): 3-minute mothership-lost → reboot
3. **Safe mode** (this ADR): 10 boot failures → degraded mode with OTA access

All three can act independently. Safe mode is the "last resort" when the node keeps failing validation but doesn't crash hard enough to trigger automatic rollback.

### Testing Strategy

#### Unit Tests
```bash
# Test boot counter increment and threshold
# Test safe mode flag persistence across reboots
# Test timer creation and callback firing
```

#### Integration Tests
```bash
# Test normal boot → counter stays at 0
# Test forced safe mode → CSI/BLE disabled, OTA works
# Test safe mode exit timer → reboots after 5 min
```

#### Hardware Tests
```bash
# Deploy broken build → observe 10-failure threshold
# Verify OTA still works in safe mode
# Confirm watchdog doesn't reset before validation completes
```

### Rollback Plan

If safe mode causes issues:
1. Disable via mothership command (new WebSocket message type)
2. Manual NVS erase via USB Serial API
3. Physical button reset (if implemented)
4. Last resort: USB programming cable and `esptool.py`

## Implementation Status

✅ NVS keys added (`NVS_KEY_BOOT_COUNTER`, `NVS_KEY_SAFE_MODE`)
✅ `safe_mode.c` and `safe_mode.h` implemented
✅ Boot sequence integration (CSI/BLE conditional init)
✅ OTA validation integration (boot-good timer, fail marking)
⏳ Watchdog configuration documentation (this file)
⏳ Testing and verification

## References

- ESPHome safe_mode component: https://esphome.io/components/safe_mode
- ESPHome regression (bf-2tgcx): esphome/esphome#15767
- ADR-004: Original rollback and health gate design
- bf-5vwo8: Health gate marks slot valid on first role message
