# Test Results: All Three esp_restart() Trigger Points

## Overview
Comprehensive test verification of all three `esp_restart()` trigger points in the Spaxel firmware to ensure the restart-safe guard prevents ESP_ERROR_CHECK aborts.

## Test Date
2026-08-13

## Test Environment
- Host: gcc (plain C compiler, no ESP-IDF toolchain)
- Test framework: `/home/coding/spaxel/firmware/test/test_runner.h`
- Test file: `test_all_restart_trigger_points.c`

## Trigger Points Tested

### 1. OTA Timeout Scenario ✓ SAFE
**Location:** `firmware/main/websocket.c:127-129`
**Function:** `ota_validation_timeout_cb()`
**Trigger Condition:** OTA validation timer expires (60s timeout)
**Context:** Node connected but role message not received within 60s
**Purpose:** Force reboot to trigger ESP-IDF rollback mechanism

**Code:**
```c
g_state.restarting = true;
ESP_LOGW(TAG, "Setting restarting flag before OTA timeout restart");
esp_restart();
```

**Result:** SAFE - Guard set before esp_restart(), WiFi operations blocked

### 2. Reboot Command Scenario ✓ SAFE
**Location:** `firmware/main/websocket.c:833-835`
**Function:** `handle_reboot_msg()`
**Trigger Condition:** Mothership sends `{"type":"reboot"}` message
**Context:** Operator-initiated restart from dashboard or API
**Purpose:** Controlled restart for maintenance or reconfiguration

**Code:**
```c
g_state.restarting = true;
ESP_LOGW(TAG, "Setting restarting flag before reboot command restart");
esp_restart();
```

**Result:** SAFE - Guard set before esp_restart(), WiFi operations blocked

### 3. OTA Completion Scenario ✓ SAFE
**Location:** `firmware/main/websocket.c:1043-1045`
**Function:** `ota_task()`
**Trigger Condition:** OTA download, verification, and partition setup all succeed
**Context:** Normal successful OTA update completion
**Purpose:** Reboot to activate new firmware partition

**Code:**
```c
g_state.restarting = true;
ESP_LOGW(TAG, "[OTA] Setting restarting flag before esp_restart()");
ESP_LOGI(TAG, "[OTA] Calling esp_restart() NOW");
esp_restart();
```

**Result:** SAFE - Guard set before esp_restart(), WiFi operations blocked

## Guard Verification

**Guard Location:** `firmware/main/wifi.c:162-170`
**Function:** `wifi_start_connect()`

```c
if (g_state.restarting) {
    ESP_LOGW(TAG, "[RESTART-SAFE-GUARD] Skipping WiFi connection - restart flag is set");
    ESP_LOGW(TAG, "[RESTART-SAFE-GUARD] This is a guard-triggered skip, NOT an error");
    ESP_LOGW(TAG, "[RESTART-SAFE-GUARD] WiFi operations will resume after next boot");
    ESP_LOGW(TAG, "[RESTART-SAFE-GUARD] State: restarting=%d, provisioned=%d",
             g_state.restarting, g_state.provisioned);
    return ESP_OK;
}
```

## What the Guard Prevents

Without the guard, WiFi API calls during restart would cause:
1. **ESP_ERROR_CHECK abort in `esp_wifi_set_mode()`**
2. **ESP_ERROR_CHECK abort in `esp_wifi_set_config()`**
3. **ESP_ERROR_CHECK abort in `esp_wifi_start()`**
4. **ESP_ERROR_CHECK abort in `esp_wifi_connect()`**
5. **WiFi driver state corruption**
6. **Unreliable node state after reboot**

## Test Execution

```bash
cd /home/coding/spaxel/firmware/test
make test
```

**Result:** All tests passed ✓

### Tests Run:
1. `all_three_restart_points_set_flag` - Verifies each trigger point sets the flag
2. `ota_timeout_scenario_with_guard` - Tests OTA timeout scenario
3. `reboot_command_scenario_with_guard` - Tests reboot command scenario
4. `ota_completion_scenario_with_guard` - Tests OTA completion scenario
5. `verify_guard_behavior_with_wifi_operations` - Verifies guard blocks WiFi ops
6. `race_condition_prevention` - Tests WiFi reconnect vs restart race
7. `guard_logging_verification` - Verifies guard logging messages
8. `document_all_trigger_points` - Documents all trigger points
9. Plus all existing firmware tests (CSI frame, NVS migration, serial prov, etc.)

## Conclusion

**All three esp_restart() trigger points are SAFE ✓**

Each trigger point correctly sets `g_state.restarting = true` **before** calling `esp_restart()`. The restart-safe guard in `wifi_start_connect()` checks this flag and prevents WiFi operations, successfully preventing ESP_ERROR_CHECK aborts in all scenarios.

No trigger points show issues - the restart-safe guard is functioning correctly across all code paths.
