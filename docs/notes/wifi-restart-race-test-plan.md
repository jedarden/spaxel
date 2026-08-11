# WiFi Restart Race Condition Test Plan

## Bead ID
bf-9gfph

## Date
2026-08-08

## Overview
This document describes the test strategy for verifying the fix for the `wifi_start_connect()` vs `esp_restart()` race condition.

## Problem Statement
When OTA is triggered during active WiFi reconnection, a race condition could occur where:
1. `wifi_start_connect()` attempts to call ESP-IDF WiFi APIs
2. `esp_restart()` is called (for OTA timeout, reboot message, or OTA completion)
3. ESP-IDF WiFi APIs are called in an invalid state, causing ESP_ERROR_CHECK to abort

## The Fix
Two protections were added:

### 1. Restart-Safe Guard in `wifi_start_connect()` (wifi.c:162-166)
```c
// Check if restart is imminent - skip WiFi operations to prevent race
if (g_state.restarting) {
    ESP_LOGW(TAG, "Restart imminent, skipping WiFi connection attempt");
    return ESP_OK;
}
```

### 2. OTA Interference Check in State Machine (main.c:374-381)
```c
case NODE_STATE_WIFI_LOST:
    // Check if OTA is in progress - if so, delay reconnection
    // to avoid racing with the OTA download process.
    // See ADR-004 / bf-xss9y.
    if (g_state.ota_in_progress) {
        ESP_LOGW(TAG, "WiFi lost but OTA in progress - delaying reconnection");
        vTaskDelay(pdMS_TO_TICKS(5000));
        break;
    }
```

## Three esp_restart() Trigger Points

1. **OTA Timeout Watchdog** (`websocket.c:129`)
   - Triggered when OTA download times out (30s default)
   - Sets `g_state.restarting = true` before `esp_restart()`

2. **Reboot Message from Mothership** (`websocket.c:835`)
   - Triggered by `{type: "reboot", delay_ms: N}` WebSocket message
   - Sets `g_state.restarting = true` before `esp_restart()`

3. **OTA Completion** (`websocket.c:1045`)
   - Triggered after successful OTA download and verification
   - Sets `g_state.restarting = true` before `esp_restart()`

## Test Scenarios

### Scenario 1: OTA During WiFi Reconnection (Manual Test)

**Purpose**: Verify that OTA completes successfully even if WiFi reconnects during download.

**Prerequisites**:
- ESP32-S3 node with firmware v0.2.18+
- Working WiFi connection
- Running mothership with OTA firmware available

**Steps**:
1. Node is connected and operational
2. Trigger OTA update from dashboard
3. During HTTP download phase, induce WiFi disconnection:
   - Move node away from AP range
   - Power cycle the AP
   - Disconnect AP ethernet cable
4. Observe serial monitor logs

**Expected Log Output (Fix Working)**:
```
I (5678) ws: Starting OTA download: http://192.168.1.100:8080/firmware/spaxel-0.2.19.bin
I (5678) [OTA] Set ota_in_progress=true - WiFi reconnection blocked
W (6789) wifi: WiFi disconnected
W (6789) spaxel: WiFi lost
W (6789) spaxel: WiFi lost but OTA in progress - delaying reconnection
I (6901) [OTA] HTTP connection open, content length: 1672582 bytes
I (6902) [OTA] Target partition: ota_0
I (6903) [OTA] OTA begin successful, handle=0x...
[download progress messages...]
I (9876) [OTA] OTA complete, preparing to reboot
I (9876) [OTA] Clearing ota_in_progress=false before restart
I (9876) [OTA] Setting restarting flag before esp_restart()
I (9876) [OTA] Calling esp_restart() NOW
```

**Expected Log Output (Fix NOT Working - SHOULD NOT OCCUR)**:
```
I (5678) ws: Starting OTA download: http://192.168.1.100:8080/firmware/spaxel-0.2.19.bin
I (5678) [OTA] Set ota_in_progress=true - WiFi reconnection blocked
W (6789) wifi: WiFi disconnected
W (6789) spaxel: WiFi lost
W (6789) spaxel: Attempting WiFi reconnect  <-- NO CHECK!
I (6789) [wifi_start_connect] Step 1: esp_wifi_set_mode(WIFI_MODE_STA)
E (6789) esp_wifi: WiFi is in running state
E (6789) wifi: [wifi_start_connect] FAILED at esp_wifi_set_mode: ESP_ERR_INVALID_STATE
Guru Meditation Error: Core 0 panic'ed (ESP_ERROR_CHECK)
```

**Acceptance Criteria**:
- ✅ "WiFi lost but OTA in progress - delaying reconnection" message appears
- ✅ NO "[wifi_start_connect] Step 1:" messages during OTA
- ✅ OTA download completes successfully
- ✅ Node reboots with new firmware
- ✅ No ESP_ERROR_CHECK abort occurs
- ✅ Node reconnects to WiFi after reboot

### Scenario 2: Reboot Command During WiFi Reconnection (Automated Test)

**Purpose**: Verify that a reboot message during WiFi reconnection doesn't cause ESP_ERROR_CHECK abort.

**Prerequisites**:
- Node in `NODE_STATE_WIFI_LOST` or `NODE_STATE_MOTHERSHIP_DISCOVERY`
- Actively attempting WiFi reconnection

**Steps**:
1. Simulate WiFi disconnection event
2. State machine enters `NODE_STATE_WIFI_LOST`
3. `wifi_start_connect()` begins execution (reaches Step 1: `esp_wifi_set_mode()`)
4. Simultaneously, mothership sends `{type: "reboot", delay_ms: 100}` message
5. Verify that `g_state.restarting` is set before `esp_restart()` is called
6. Verify that `wifi_start_connect()` checks `g_state.restarting` and returns early

**Expected Behavior**:
```
I (1234) spaxel: WiFi lost
I (1235) spaxel: Attempting WiFi reconnect
I (1236) wifi: [wifi_start_connect] Step 1: esp_wifi_set_mode(WIFI_MODE_STA)
I (1240) ws: Reboot requested in 100 ms
I (1240) ws: Setting restarting flag before reboot command restart
W (1241) wifi: Restart imminent, skipping WiFi connection attempt  <-- EARLY RETURN
I (1340) esp_restart: Restarting...
```

**Acceptance Criteria**:
- ✅ "Restart imminent, skipping WiFi connection attempt" message appears
- ✅ No ESP_ERROR_CHECK abort occurs
- ✅ System reboots cleanly
- ✅ After reboot, normal WiFi reconnection proceeds

### Scenario 3: OTA Timeout During WiFi Reconnection (Automated Test)

**Purpose**: Verify that OTA timeout watchdog can trigger safely even during WiFi reconnection.

**Prerequisites**:
- Node in `NODE_STATE_WIFI_LOST`
- OTA download stalled or network very slow

**Steps**:
1. Trigger OTA update
2. Simulate network conditions that cause OTA to timeout (>30s)
3. During timeout, WiFi may also be trying to reconnect
4. OTA watchdog task detects timeout
5. Verify watchdog sets `g_state.restarting = true` before `esp_restart()`
6. Verify `wifi_start_connect()` respects the flag

**Expected Behavior**:
```
I (5678) ws: Starting OTA download: http://...
I (5678) [OTA] Set ota_in_progress=true - WiFi reconnection blocked
W (6789) wifi: WiFi disconnected
W (6789) spaxel: WiFi lost
W (6789) spaxel: WiFi lost but OTA in progress - delaying reconnection
I (35000) [OTA] OTA timeout - watchdog triggered
W (35000) [OTA] Setting restarting flag before OTA timeout restart
W (35001) wifi: Restart imminent, skipping WiFi connection attempt
I (35001) esp_restart: Restarting...
```

**Acceptance Criteria**:
- ✅ Watchdog sets `g_state.restarting = true`
- ✅ "Restart imminent, skipping WiFi connection attempt" message appears
- ✅ System reboots cleanly
- ✅ No double-restart or crash

### Scenario 4: OTA Completion During WiFi Reconnection (Manual + Automated)

**Purpose**: Verify the most common case: OTA completes successfully while node is reconnecting to WiFi.

**Prerequisites**:
- Node with marginal WiFi connection (frequent disconnections)
- Large firmware image (>1MB) for longer download time

**Steps**:
1. Node enters `NODE_STATE_WIFI_LOST` due to poor signal
2. Mothership triggers OTA update
3. Node sets `g_state.ota_in_progress = true` to block WiFi interference
4. OTA completes successfully (download + verify + write)
5. OTA task sets `g_state.restarting = true`
6. OTA task calls `esp_restart()`

**Expected Behavior**:
```
W (1234) wifi: WiFi disconnected
W (1234) spaxel: WiFi lost
I (1235) ws: Starting OTA download: http://...
I (1235) [OTA] Set ota_in_progress=true - WiFi reconnection blocked
[download progress...]
I (9876) [OTA] OTA complete, preparing to reboot
I (9876) [OTA] Clearing ota_in_progress=false before restart
W (9876) [OTA] Setting restarting flag before esp_restart()
W (9877) wifi: Restart imminent, skipping WiFi connection attempt
I (9878) [OTA] Calling esp_restart() NOW
```

**Acceptance Criteria**:
- ✅ OTA completion logs show both flags being set correctly
- ✅ `ota_in_progress` cleared before `restarting` is set
- ✅ "Restart imminent, skipping WiFi connection attempt" appears
- ✅ No race condition between flag clearing and setting
- ✅ Node reboots with new firmware

## Regression Tests

### Test 1: Normal WiFi Reconnection (No OTA, No Restart)

**Purpose**: Ensure normal WiFi reconnection still works.

**Steps**:
1. Disconnect AP power
2. Wait for WiFi disconnect event
3. Wait for WiFi reconnect attempt
4. Reconnect AP power
5. Verify successful reconnection

**Expected**: Normal reconnection proceeds without hitting the restart-safe guard.

### Test 2: Multiple Rapid Reboot Commands

**Purpose**: Ensure rapid reboot commands don't cause race conditions.

**Steps**:
1. Send `{type: "reboot", delay_ms: 100}`
2. Immediately send another `{type: "reboot", delay_ms: 100}`
3. Verify only one reboot occurs
4. Verify no ESP_ERROR_CHECK abort

**Expected**: Second reboot is ignored or queued; system reboots cleanly once.

### Test 3: OTA Then Immediate Reboot Command

**Purpose**: Verify interaction between OTA completion and external reboot command.

**Steps**:
1. Start OTA download
2. During download, send `{type: "reboot", delay_ms: 100}`
3. Verify which restart wins (OTA completion should override)
4. Verify clean reboot

**Expected**: OTA completion takes precedence; immediate reboot is ignored.

## Test Implementation

### Automated Tests (Go)

File: `mothership/test/acceptance/as5_wifi_restart_race_test.go`

```go
package acceptance

import (
    "context"
    "testing"
    "time"
)

// AS5_WiFiRestartRace_NoAbortDuringReboot verifies that reboot during WiFi
// reconnection doesn't cause ESP_ERROR_CHECK abort.
func AS5_WiFiRestartRace_NoAbortDuringReboot(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping acceptance test in short mode")
    }

    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
    defer cancel()

    // Test requires real hardware
    if os.Getenv("SPAXEL_HARDWARE_TEST") != "1" {
        t.Skip("Set SPAXEL_HARDWARE_TEST=1 to run hardware test")
    }

    t.Run("RebootCommandDuringWiFiReconnect", testRebootDuringWiFiReconnect)
    t.Run("OTATimeoutDuringWiFiReconnect", testOTATimeoutDuringWiFiReconnect)
    t.Run("OTACompletionDuringWiFiReconnect", testOTACompletionDuringWiFiReconnect)
}
```

### Firmware Tests (C)

File: `firmware/test/test_wifi_restart_race.c`

```c
// Test that the restarting flag prevents WiFi operations
void test_restarting_flag_blocks_wifi_ops(void) {
    // Set restarting flag
    g_state.restarting = true;
    
    // Attempt WiFi connection should return ESP_OK early
    esp_err_t err = wifi_start_connect();
    TEST_ASSERT_EQUAL(ESP_OK, err);
    
    // Verify logs contain "Restart imminent" message
    // (checked via manual inspection or log parsing)
    
    g_state.restarting = false;
}
```

## Manual Test Procedure

### Equipment Required
- ESP32-S3 development board
- USB cable for serial monitoring
- WiFi access point (can be phone hotspot)
- Computer running mothership
- Serial monitor (115200 baud)

### Pre-Test Setup
1. Flash firmware with detailed logging enabled
2. Connect serial monitor
3. Configure WiFi credentials
4. Connect node to mothership
5. Verify node is online and streaming CSI

### Test Execution

#### Test Case 1: OTA During WiFi Disconnection

1. **Prepare**: Monitor serial output for log messages
2. **Start OTA**: From dashboard, click "Update" on a node
3. **Induce Disconnect**: During download phase (5-10 seconds in), briefly disconnect WiFi:
   - Unplug AP ethernet cable for 2 seconds
   - Or move node far from AP
4. **Observe Logs**: Look for the following sequence:
   ```
   [OTA] Set ota_in_progress=true
   WiFi lost
   WiFi lost but OTA in progress - delaying reconnection
   [OTA] HTTP connection open
   [OTA] OTA complete, preparing to reboot
   [OTA] Setting restarting flag before esp_restart()
   Restart imminent, skipping WiFi connection attempt
   [OTA] Calling esp_restart() NOW
   ```
5. **Verify Success**:
   - Node reboots within 5 seconds
   - Comes back online with new firmware version
   - Dashboard shows new version
   - No ESP_ERROR_CHECK abort occurred

#### Test Case 2: Reboot Command During WiFi Reconnection

1. **Prepare**: Monitor serial output
2. **Induce Poor WiFi**: Place node at edge of WiFi range to cause frequent disconnections
3. **Trigger Reboot**: From dashboard, send reboot command
4. **Observe Logs**: Look for:
   ```
   WiFi lost
   Attempting WiFi reconnect
   [wifi_start_connect] Step 1: esp_wifi_set_mode(WIFI_MODE_STA)
   Reboot requested in 100 ms
   Setting restarting flag before reboot command restart
   Restart imminent, skipping WiFi connection attempt
   ```
5. **Verify Success**:
   - Node reboots cleanly
   - No panic or abort
   - Reconnects after reboot

#### Test Case 3: All Three Restart Points

For each restart point, repeat Test Case 2:

1. **OTA Timeout**: Start OTA, unplug network to cause timeout
2. **Reboot Message**: Send `{type: "reboot"}` message
3. **OTA Completion**: Let OTA complete naturally

## Log Pattern Verification

Use log parsing to automatically verify the fix:

```bash
# Check for restart-safe guard activation
grep "Restart imminent, skipping WiFi connection attempt" serial.log

# Check for OTA interference prevention
grep "WiFi lost but OTA in progress - delaying reconnection" serial.log

# Check for ESP_ERROR_CHECK abort (should NOT find this)
grep "Guru Meditation Error" serial.log

# Verify proper flag sequence during OTA
grep -A 2 "Setting restarting flag" serial.log | grep -v "Restart imminent"
```

## Success Criteria Summary

All tests pass if:
- ✅ No ESP_ERROR_CHECK aborts occur during any restart scenario
- ✅ "Restart imminent, skipping WiFi connection attempt" appears in all three restart cases
- ✅ OTA completes successfully even with WiFi disconnection
- ✅ All three esp_restart() trigger points set `g_state.restarting = true`
- ✅ Normal WiFi reconnection still works when no restart is pending
- ✅ Log patterns show expected guard activation

## Failure Modes

If tests fail, check for:
1. **Missing guard logs**: "Restart imminent" message missing → guard not activated
2. **ESP_ERROR_CHECK abort**: Race condition still present → fix ineffective
3. **OTA doesn't complete**: OTA interference still present → check OTA flag logic
4. **No reboot after OTA**: Restart blocked incorrectly → check restart logic

## References

- `docs/notes/ota-wifi-reconnection-race-summary.md` - Detailed investigation
- `firmware/main/wifi.c:162-166` - Restart-safe guard implementation
- `firmware/main/main.c:374-381` - OTA interference check
- `firmware/main/websocket.c:129, 835, 1045` - Three esp_restart() trigger points
