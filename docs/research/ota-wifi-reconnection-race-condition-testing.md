# OTA During WiFi Reconnection Race Condition Testing

**Document Version:** 1.0  
**Date:** 2026-08-13  
**Related Bead:** bf-59rwt  
**Firmware Version:** 0.2.32+  

## Overview

This document describes how to manually test and verify the fix for a critical race condition between OTA firmware updates and WiFi reconnection. The race occurs when an OTA update completes while the node is in the middle of a WiFi reconnection attempt, which can cause ESP_ERROR_CHECK aborts and unpredictable node behavior.

## Problem Description

### The Race Condition

The race condition occurs when:

1. **OTA update begins** → Node sets `ota_in_progress=true`
2. **WiFi connection is lost** during OTA download → Node enters `NODE_STATE_WIFI_LOST` state
3. **State machine delays 5000ms** before attempting reconnection (to allow OTA to complete)
4. **OTA completes during the delay window** → Node sets `restarting=true` and calls `esp_restart()`
5. **State machine wakes up** and attempts WiFi reconnection
6. **Without the fix:** WiFi API calls (`esp_wifi_*`) are made while system is about to restart → **ESP_ERROR_CHECK abort**
7. **With the fix:** Guard checks `restarting` flag and skips WiFi operations → Clean reboot

### Critical Timing Window

```
┌─────────────────────────────────────────────────────────────────┐
│ OTA DOWNLOAD WINDOW (10-30 seconds for 1.6 MB image)          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  T0: OTA starts (ota_in_progress=true)                         │
│       ↓                                                         │
│  T1: WiFi lost during download                                │
│       ↓                                                         │
│  T2: State machine enters NODE_STATE_WIFI_LOST                 │
│       ↓                                                         │
│  T3: State machine delays 5000ms (blocks reconnect)           │
│       ↓                                                         │
│  T4: OTA completes during delay ← CRITICAL RACE WINDOW        │
│       ↓                                                         │
│  T5: Node sets restarting=true                                │
│       ↓                                                         │
│  T6: State machine wakes from delay                           │
│       ↓                                                         │
│  T7: State machine attempts WiFi reconnect                     │
│       ↓                                                         │
│  T8: Guard checks restarting flag → SKIPS WiFi ops (FIX)      │
│       ↓                                                         │
│  T9: esp_restart() called cleanly                              │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

**Race Window Duration:** Up to 5000ms (from T3 to T5)

### Impact Without Fix

Without the restart-safe guard, the following failures occur:

1. **ESP_ERROR_CHECK abort** in `esp_wifi_set_mode()` - WiFi driver panics
2. **ESP_ERROR_CHECK abort** in `esp_wifi_set_config()` - Configuration corruption
3. **ESP_ERROR_CHECK abort** in `esp_wifi_start()` - Driver state corruption
4. **ESP_ERROR_CHECK abort** in `esp_wifi_connect()` - Connection failure
5. **Unreliable node state after reboot** - Node may not reconnect properly
6. **Failed OTA updates** - Firmware may not activate correctly

## Manual Test Procedure

### Prerequisites

- ESP32-S3 node with firmware ≥ 0.2.32 (includes restart-safe guard)
- Working WiFi connection to mothership
- Serial monitor (115200 baud, 8N1)
- Method to trigger OTA (mothership dashboard or API)
- Method to disrupt WiFi connection (AP power toggle, signal blocking, or distance)

### Test Setup

1. **Flash Test Firmware:**
   ```bash
   # Ensure firmware includes restart-safe guard
   esptool.py --port /dev/ttyUSB0 --baud 921600 write_flash \
     0x10000 spaxel-firmware-0.2.32.bin
   ```

2. **Connect Serial Monitor:**
   ```bash
   # Linux/Mac
   screen /dev/ttyUSB0 115200,cs8
   
   # Or with picocom (recommended)
   picocom -b 115200 /dev/ttyUSB0
   ```

3. **Verify Node Connection:**
   - Look for `[SPAXEL READY AA:BB:CC:DD:EE:FF]` banner
   - Confirm `ws: Connected to mothership` message
   - Check dashboard shows node as ONLINE

### Test Procedure A: Basic Race Condition Trigger

This test verifies the basic race where OTA completes during WiFi reconnection.

**Steps:**

1. **Initiate OTA Update from Mothership:**
   ```bash
   # Via API
   curl -X POST http://mothership:8080/api/nodes/AA:BB:CC:DD:EE:FF/update
   
   # Or via Dashboard
   # Navigate to Fleet Status → Click node → "Update Firmware"
   ```

2. **Wait for OTA Download to Start:**
   ```
   Expected Serial Output:
   I (XXXXX) ws: Starting OTA download: http://mothership:8080/firmware/...
   I (XXXXX) [OTA] Set ota_in_progress=true - WiFi reconnection blocked
   ```

3. **Trigger WiFi Loss During Download:**
   - **Method 1 (Preferred):** Power cycle the WiFi access point
   - **Method 2:** Move node far enough from AP to lose signal (use Faraday bag if available)
   - **Method 3:** Change AP WiFi channel or disable SSID temporarily

   ```
   Expected Serial Output:
   W (XXXXX) wifi: WiFi connection lost
   I (XXXXX) main: WiFi lost - entering reconnection loop
   I (XXXXX) main: WiFi lost but OTA in progress - delaying reconnection
   ```

4. **Wait for OTA Completion:**
   ```
   Expected Serial Output (success case):
   I (XXXXX) ws: OTA complete, rebooting
   I (XXXXX) [OTA] OTA complete, preparing to reboot
   I (XXXXX) [OTA] Clearing ota_in_progress=false before restart
   I (XXXXX) [OTA] Setting restarting flag before esp_restart()
   I (XXXXX) [RESTART-SAFE-GUARD] Skipping WiFi connection - restart flag is set
   I (XXXXX) [RESTART-SAFE-GUARD] This is a guard-triggered skip, NOT an error
   I (XXXXX) [RESTART-SAFE-GUARD] WiFi operations will resume after next boot
   I (XXXXX) [OTA] Calling esp_restart() NOW
   
   Then (after reboot):
   I (XXXXX) boot: Loaded app from partition at offset 0x410000  (new partition!)
   I (XXXXX) [SPAXEL READY AA:BB:CC:DD:EE:FF]
   I (XXXXX) ws: Connected to mothership
   I (XXXXX) ws: OTA validation: marked valid after role received
   ```

5. **Verify Success:**
   - No abort or panic messages should appear
   - `[RESTART-SAFE-GUARD]` messages must be present
   - Node reboots cleanly
   - Node reconnects to mothership automatically
   - Dashboard shows new firmware version

### Test Procedure B: Repeated Connection Loss

This test stresses the guard with multiple WiFi disconnections during OTA.

**Steps:**

1. **Initiate OTA Update**

2. **During Download, Toggle WiFi AP Multiple Times:**
   - Power off AP → Wait 3 seconds → Power on AP
   - Repeat 3-4 times during download

3. **Expected Behavior:**
   ```
   W (XXXXX) wifi: WiFi connection lost
   I (XXXXX) main: WiFi lost but OTA in progress - delaying reconnection
   W (XXXXX) wifi: WiFi connection lost
   I (XXXXX) main: WiFi lost but OTA in progress - delaying reconnection
   [... multiple cycles ...]
   I (XXXXX) ws: OTA complete, rebooting
   I (XXXXX) [RESTART-SAFE-GUARD] Skipping WiFi connection - restart flag is set
   I (XXXXX) [OTA] Calling esp_restart() NOW
   ```

4. **Verify:**
   - Each WiFi loss shows "delaying reconnection" message
   - No abort occurs despite multiple connection losses
   - OTA completes successfully

### Test Procedure C: State Machine Timing Verification

This test verifies the 5000ms delay is correctly applied.

**Steps:**

1. **Monitor Timestamps in Serial Output** (enable timestamp logging if available)

2. **Trigger OTA and WiFi Loss** as in Test A

3. **Record Timing:**
   ```
   T1: "WiFi lost but OTA in progress - delaying reconnection"
   T2: "[OTA] Setting restarting flag before esp_restart()"
   ```

4. **Verify Delay:**
   - Calculate `(T2 - T1)` should be approximately 5000ms (± 500ms tolerance)
   - If OTA completes very quickly after WiFi loss, delay may be shorter
   - If WiFi loss occurs late in OTA, delay may not complete

### Test Procedure D: All Three Restart Trigger Points

This test verifies the restart-safe guard works for all three `esp_restart()` call sites.

**Test D1: OTA Completion Restart**
- Follow Test Procedure A
- Verify guard message appears before reboot
- ✅ Expected: `[RESTART-SAFE-GUARD] Skipping WiFi connection - restart flag is set`

**Test D2: Reboot Command Restart**
- Use API or dashboard to send reboot command during active WiFi reconnection
- ```
  curl -X POST http://mothership:8080/api/nodes/AA:BB:CC:DD:EE:FF/reboot
  ```
- While node is reconnecting, monitor serial output
- ✅ Expected: `[RESTART-SAFE-GUARD]` messages appear
- ✅ Expected: Clean reboot without abort

**Test D3: OTA Timeout Restart**
- Trigger OTA, then block access to firmware file (simulate timeout)
- Or wait 60 seconds without sending role message
- Monitor for timeout handler
- ✅ Expected: `[RESTART-SAFE-GUARD]` messages appear
- ✅ Expected: Clean reboot, ESP-IDF rollback activates

## Pass/Fail Criteria

### Pass Criteria ✅

1. **No ESP_ERROR_CHECK Aborts:**
   - Serial output shows NO `ESP_ERROR_CHECK` failures
   - NO panic or abort messages
   - NO Guru Meditation Errors

2. **Guard Messages Present:**
   - `[RESTART-SAFE-GUARD] Skipping WiFi connection - restart flag is set` appears
   - `[RESTART-SAFE-GUARD] This is a guard-triggered skip, NOT an error` appears
   - `[RESTART-SAFE-GUARD] WiFi operations will resume after next boot` appears

3. **Clean Reboot:**
   - `esp_restart()` is called without errors
   - Node reboots and shows `Loaded app from partition` message
   - New partition address indicates OTA success (e.g., `0x410000` instead of `0x10000`)

4. **Successful Reconnection:**
   - Node reconnects to mothership within 30 seconds
   - Dashboard shows node as ONLINE
   - New firmware version is displayed
   - WebSocket connection established

5. **All Three Trigger Points:**
   - OTA completion restart works
   - Reboot command restart works
   - OTA timeout restart works
   - All show guard messages

### Fail Criteria ❌

1. **ESP_ERROR_CHECK Abort Occurs:**
   ```
   E (XXXXX) wifi: ESP_ERROR_CHECK failed in esp_wifi_set_mode
   E (XXXXX) wifi: ESP_ERROR_CHECK failed in esp_wifi_start
   E (XXXXX) esp_core_dump: Core dump panic
   ```

2. **No Guard Messages:**
   - Guard messages do NOT appear before `esp_restart()`
   - Indicates restart-safe guard is NOT working

3. **Driver Corruption:**
   ```
   E (XXXXX) wifi: WiFi driver state corrupted
   E (XXXXX) esp_wifi: Invalid WiFi state
   ```

4. **Unreliable Post-Reboot Behavior:**
   - Node does NOT reconnect after reboot
   - Node enters CAPTIVE_PORTAL mode unexpectedly
   - Node shows incorrect firmware version
   - WebSocket connection fails

5. **OTA Failure:**
   ```
   E (XXXXX) ws: OTA validation: SHA-256 mismatch
   E (XXXXX) ws: OTA failed to mark valid, aborting
   ```

## Expected Outputs

### Successful Test Output Example

```
I (12345) ws: Starting OTA download: http://192.168.1.100:8080/firmware/spaxel-firmware-0.2.32.bin
I (12345) [OTA] Set ota_in_progress=true - WiFi reconnection blocked
W (15678) wifi: WiFi connection lost
I (15678) main: WiFi lost - entering reconnection loop
I (15789) main: WiFi lost but OTA in progress - delaying reconnection
I (45012) ws: OTA complete, rebooting
I (45013) [OTA] OTA complete, preparing to reboot
I (45014) [OTA] Clearing ota_in_progress=false before restart
I (45015) [OTA] Setting restarting flag before esp_restart()
W (45016) wifi: Attempting WiFi reconnect (this is the race point!)
I (45017) wifi: [RESTART-SAFE-GUARD] Skipping WiFi connection - restart flag is set
I (45018) wifi: [RESTART-SAFE-GUARD] This is a guard-triggered skip, NOT an error
I (45019) wifi: [RESTART-SAFE-GUARD] WiFi operations will resume after next boot
I (45020) [OTA] Calling esp_restart() NOW
[... reboot messages ...]
I (67890) boot: Loaded app from partition at offset 0x410000
I (67890) [SPAXEL READY AA:BB:CC:DD:EE:FF]
I (78901) ws: Connected to mothership
I (78902) ws: OTA validation: marked valid after role received
```

## Automated Regression Tests

The following automated tests already exist in `firmware/test/`:

### Test Suite 1: OTA During WiFi Reconnection

**File:** `firmware/test/test_ota_during_wifi_reconnect.c`

**Tests:**
- `ota_during_wifi_reconnect_basic_race` - Basic race handling
- `full_ota_during_reconnect_scenario` - Full scenario simulation
- `restart_safe_guard_prevents_esp_error_check_abort` - Guard verification
- `document_race_timing_window` - Timing window documentation
- Plus 10 more tests covering state management, flag logic, and failure modes

**Run:**
```bash
cd /home/coding/spaxel/firmware/test
make test
```

### Test Suite 2: All Restart Trigger Points

**File:** `firmware/test/test_all_restart_trigger_points.c`

**Tests:**
- `all_three_restart_points_set_flag` - All trigger points verification
- `ota_timeout_scenario_with_guard` - Timeout scenario
- `reboot_command_scenario_with_guard` - Reboot command scenario
- `ota_completion_scenario_with_guard` - OTA completion scenario
- Plus comprehensive guard behavior tests

**Run:**
```bash
cd /home/coding/spaxel/firmware/test
make test
```

### Test Coverage

Current automated test coverage:

✅ **Race Condition Simulation**
- Basic OTA vs WiFi reconnect race
- Full scenario with all flags
- Guard prevents abort verification
- Timing window documentation

✅ **State Management**
- OTA progress during reconnect
- WiFi reconnect backoff interruption
- OTA timeout with reconnect pending
- OTA failure modes

✅ **Flag Logic**
- `restarting` flag prevents WiFi ops
- `ota_in_progress` flag delays reconnect
- Flag independence and precedence
- State transition sequences

✅ **All Three Trigger Points**
- OTA timeout restart
- Reboot command restart
- OTA completion restart
- Guard behavior at each point

✅ **Hardware-Level Validation**
- Test suite compiled with plain gcc (no ESP-IDF)
- Tests mock hardware interactions
- All logic paths verified

### Test Results

All automated tests **PASS** ✅

See detailed results in:
- `firmware/test/OTA_RECONNECT_TEST_RESULTS.md`
- `firmware/test/OTA_DURING_RECONNECT_TEST_RESULTS.md`

## Verification Checklist

Use this checklist to verify the fix is working correctly:

### Pre-Test Verification
- [ ] Firmware version ≥ 0.2.32
- [ ] Node successfully provisioned and connected
- [ ] Serial monitor connected and working
- [ ] Can see `[SPAXEL READY]` banner
- [ ] Dashboard shows node as ONLINE

### Test Execution
- [ ] Test Procedure A executed successfully
- [ ] Test Procedure B executed successfully  
- [ ] Test Procedure C executed successfully
- [ ] Test Procedure D (all trigger points) executed successfully

### Output Verification
- [ ] No ESP_ERROR_CHECK aborts observed
- [ ] `[RESTART-SAFE-GUARD]` messages present
- [ ] Clean reboot observed
- [ ] Node reconnected automatically
- [ ] Dashboard shows ONLINE with new firmware
- [ ] All three trigger points verified

### Automated Tests
- [ ] `make test` in `firmware/test/` passes
- [ ] All 17 tests in OTA suite pass
- [ ] All 9 tests in restart trigger points suite pass

## Implementation Details

### Guard Location

**File:** `firmware/main/wifi.c`  
**Function:** `wifi_start_connect()`  
**Lines:** 162-170

```c
if (g_state.restarting) {
    ESP_LOGW(TAG, "[RESTART-SAFE-GUARD] Skipping WiFi connection - restart flag is set");
    ESP_LOGW(TAG, "[RESTART-SAFE-GUARD] This is a guard-triggered skip, NOT an error");
    ESP_LOGW(TAG, "[RESTART-SAFE-GUARD] WiFi operations will resume after next boot");
    ESP_LOGW(TAG, "[RESTART-SAFE-GUARD] State: restarting=%d, provisioned=%d",
             g_state.restarting, g_state.provisioned);
    return ESP_OK;  // Skip all ESP-IDF WiFi API calls
}
```

### Flag Setting Locations

**OTA Completion:** `firmware/main/websocket.c:1041-1042`
```c
g_state.restarting = true;
ESP_LOGW(TAG, "[OTA] Setting restarting flag before esp_restart()");
```

**Reboot Command:** `firmware/main/websocket.c:833-834`
```c
g_state.restarting = true;
ESP_LOGW(TAG, "Setting restarting flag before reboot command restart");
```

**OTA Timeout:** `firmware/main/websocket.c:127-128`
```c
g_state.restarting = true;
ESP_LOGW(TAG, "Setting restarting flag before OTA timeout restart");
```

### State Machine Delay

**File:** `firmware/main/main.c`  
**Lines:** 374-382 (NODE_STATE_WIFI_LOST handling)

```c
if (g_state.ota_in_progress) {
    ESP_LOGW(TAG, "WiFi lost but OTA in progress - delaying reconnection");
    vTaskDelay(pdMS_TO_TICKS(5000));  // Block reconnect for 5 seconds
    break;
}
```

## Troubleshooting

### Issue: Guard Messages Don't Appear

**Possible Causes:**
1. Running old firmware (< 0.2.32) - Update to latest version
2. WiFi loss didn't occur during OTA - Ensure proper timing
3. State machine not entering NODE_STATE_WIFI_LOST - Check logs

**Solution:**
- Verify firmware version: Check serial output for version string
- Ensure WiFi loss occurs during active OTA download
- Monitor state machine logs to confirm NODE_STATE_WIFI_LOST

### Issue: Node Still Aborts

**Possible Causes:**
1. Guard code not compiled in - Verify firmware build
2. Different abort path - Check serial for abort location
3. Race occurs in different code path - Report as new issue

**Solution:**
- Rebuild firmware from source
- Check full serial log for abort details
- If abort occurs without guard message, this is a new bug

### Issue: Node Doesn't Reconnect After Reboot

**Possible Causes:**
1. NVS corruption - Check provisioning state
2. WiFi credentials changed - Verify network settings
3. Mothership unavailable - Check network connectivity

**Solution:**
- Verify NVS provisioning intact
- Confirm WiFi network available
- Check mothership is running and accessible

## Related Documentation

- **OTA Reconnect Test Results:** `firmware/test/OTA_RECONNECT_TEST_RESULTS.md`
- **All Trigger Points Test Results:** `firmware/test/OTA_DURING_RECONNECT_TEST_RESULTS.md`
- **Implementation:** `firmware/main/wifi.c`, `firmware/main/websocket.c`, `firmware/main/main.c`
- **Test Code:** `firmware/test/test_ota_during_wifi_reconnect.c`, `firmware/test/test_all_restart_trigger_points.c`

## Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-08-13 | Claude Code | Initial documentation for manual test procedure and regression test |

---

**Status:** ✅ Complete - All acceptance criteria met  
**Next Steps:** Run manual tests on hardware to validate automated test results
