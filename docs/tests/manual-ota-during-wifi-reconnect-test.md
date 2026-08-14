# Manual Test Procedure: WiFi Restart Race Condition Fix

**Bead:** bf-9gfph  
**Purpose:** Verify that the restart-safe guard prevents ESP_ERROR_CHECK abort when OTA occurs during WiFi reconnection  
**Hardware Required:** 1 ESP32-S3 node + USB connection for serial monitoring  
**Software Required:** mothership running, serial monitor (115200 baud)

## Background

This test validates the fix for a race condition where `wifi_start_connect()` could be called while `esp_restart()` is imminent, causing ESP_ERROR_CHECK aborts and unstable node behavior.

### Race Window

- **Start:** WiFi lost during OTA download  
- **End:** OTA completion + restart flag set  
- **Duration:** Up to 5000ms (state machine delay in `NODE_STATE_WIFI_LOST`)

### Protection Mechanism

1. **`ota_in_progress` flag** (websocket.c:861) - Blocks WiFi reconnect during OTA
2. **`restarting` flag** (websocket.c:1042) - Set before all `esp_restart()` calls
3. **Guard in `wifi_start_connect()`** (wifi.c:162-170) - Skips WiFi operations if `restarting` flag is set

## Test Scenarios

### Scenario 1: OTA During Active WiFi Reconnection

**Setup:**
1. Node is connected to mothership and streaming CSI
2. Serial monitor connected at 115200 baud
3. Node provisioned and operational

**Procedure:**

1. **Initiate OTA update from dashboard:**
   - Navigate to Fleet Status page
   - Click "Update" on a test node
   - Note the time: T0

2. **Simulate WiFi disconnection during OTA:**
   - Option A: Physically power cycle router (extreme test)
   - Option B: Change WiFi password on router (forces reconnect)
   - Option C: Unplug and replug node's WiFi antenna (if applicable)
   - Time window: Within 10-30 seconds of OTA start (while download is active)

3. **Monitor serial output for expected sequence:**
   ```
   [OTA] Starting OTA download...
   [OTA] Set ota_in_progress=true - WiFi reconnection blocked
   WiFi disconnected
   WiFi lost but OTA in progress - delaying reconnection
   [OTA] Download progress: 50%
   WiFi lost but OTA in progress - delaying reconnection
   [OTA] Download progress: 100%
   [OTA] Verifying firmware...
   [OTA] OTA complete, preparing to reboot
   [OTA] Clearing ota_in_progress=false before restart
   [OTA] Setting restarting flag before esp_restart()
   [RESTART-SAFE-GUARD] Skipping WiFi connection - restart flag is set
   [RESTART-SAFE-GUARD] This is a guard-triggered skip, NOT an error
   [RESTART-SAFE-GUARD] WiFi operations will resume after next boot
   [RESTART-SAFE-GUARD] State: restarting=1, provisioned=1
   [OTA] Calling esp_restart() NOW
   ```

4. **Verify success criteria:**
   - ✓ No ESP_ERROR_CHECK abort appears in serial log
   - ✓ No Guru Meditation Error / WDT reset
   - ✓ `[RESTART-SAFE-GUARD]` messages appear
   - ✓ Node reboots cleanly
   - ✓ Node reconnects to mothership within 30 seconds
   - ✓ Node reports updated firmware version

**Expected Result:** Node completes OTA and reboots without abort, even though WiFi was disconnected during the download.

### Scenario 2: Reboot Command During WiFi Reconnection

**Setup:**
1. Node is connected to mothership
2. Serial monitor connected

**Procedure:**

1. **Trigger WiFi reconnection:**
   - Temporarily disable node's WiFi in router (block MAC)
   - Wait for "WiFi disconnected" log message
   - Re-enable WiFi

2. **Immediately send reboot command from dashboard:**
   - Click "Restart" on node in Fleet Status
   - Or send POST to `/api/nodes/{mac}/reboot` with `delay_ms=100`

3. **Monitor serial output:**
   ```
   WiFi disconnected
   Starting WiFi reconnection...
   [REBOOT CMD] Setting restarting flag before reboot command restart
   [RESTART-SAFE-GUARD] Skipping WiFi connection - restart flag is set
   [RESTART-SAFE-GUARD] This is a guard-triggered skip, NOT an error
   ```

4. **Verify success criteria:**
   - ✓ No ESP_ERROR_CHECK abort
   - ✓ `[RESTART-SAFE-GUARD]` message appears
   - ✓ Node reboots and reconnects

**Expected Result:** Reboot command aborts WiFi reconnection cleanly and reboots.

### Scenario 3: OTA Timeout During WiFi Reconnection

**Setup:**
1. Node is connected to mothership
2. Serial monitor connected
3. Prepare invalid OTA URL (will timeout)

**Procedure:**

1. **Trigger OTA with timeout scenario:**
   - Modify firmware to have 60-second validation timeout (already in code)
   - Start OTA but don't send role message within 60 seconds
   - Or use non-routable OTA URL that times out

2. **Simultaneously trigger WiFi reconnection:**
   - Block/unblock node's WiFi from router

3. **Monitor serial output:**
   ```
   [OTA TIMEOUT] Setting restarting flag before OTA timeout restart
   [RESTART-SAFE-GUARD] Skipping WiFi connection - restart flag is set
   ```

4. **Verify success criteria:**
   - ✓ No ESP_ERROR_CHECK abort
   - ✓ Node reboots and recovers

**Expected Result:** OTA timeout triggers clean reboot without interfering with WiFi reconnection.

### Scenario 4: All Three Restart Points (Comprehensive)

**Setup:**
1. Three separate test runs (one per restart point)
2. Serial monitor connected for each

**Procedure:**

For each restart point, verify the specific log message appears:

1. **OTA Timeout (websocket.c:127-129):**
   ```
   Setting restarting flag before OTA timeout restart
   ```

2. **Reboot Command (websocket.c:833-835):**
   ```
   Setting restarting flag before reboot command restart
   ```

3. **OTA Completion (websocket.c:1043-1045):**
   ```
   [OTA] Setting restarting flag before esp_restart()
   [OTA] Calling esp_restart() NOW
   ```

Each should be followed by the guard message if WiFi reconnection is attempted:
```
[RESTART-SAFE-GUARD] Skipping WiFi connection - restart flag is set
[RESTART-SAFE-GUARD] This is a guard-triggered skip, NOT an error
```

**Expected Result:** All three restart points set the flag correctly and prevent WiFi operations during restart.

## Regression Test Procedure

### Automated Host Tests (No Hardware)

These run on any machine with gcc:

```bash
# Run from repo root
make -C firmware/test test
```

**Expected output:** All tests pass, including:
- `ota_during_wifi_reconnect_basic_race`
- `ota_timeout_scenario_with_guard`
- `reboot_command_scenario_with_guard`
- `ota_completion_scenario_with_guard`
- `restart_safe_guard_prevents_esp_error_check_abort`
- `race_condition_prevention`

### Go Acceptance Tests (With Mothership)

These require a running mothership:

```bash
# Run hardware-based acceptance tests
cd mothership
SPAXEL_HARDWARE_TEST=1 go test ./test/acceptance/ -v -run AS5_WiFiRestartRace
```

**Expected output:** Tests pass with real node (if `SPAXEL_HARDWARE_TEST=1`)

## Success Criteria Summary

- ✓ No ESP_ERROR_CHECK abort in any scenario
- ✓ No Guru Meditation Error / WDT reset
- ✓ `[RESTART-SAFE-GUARD]` logs appear when race condition triggers
- ✓ OTA completes successfully in all scenarios
- ✓ Node reboots cleanly and reconnects
- ✓ Normal WiFi reconnection still works (no regressions)

## Log Analysis Checklist

When reviewing serial logs from a test run, verify:

1. **Restart flag is set BEFORE esp_restart():**
   - Search for "Setting restarting flag" immediately before "esp_restart()"
   - Should be within 1-3 log lines

2. **Guard message appears if WiFi reconnection is attempted:**
   - `[RESTART-SAFE-GUARD] Skipping WiFi connection`
   - Should appear when `g_state.restarting = true`

3. **No WiFi API calls after guard:**
   - No `esp_wifi_set_mode` after guard
   - No `esp_wifi_set_config` after guard
   - No `esp_wifi_start` after guard
   - No `esp_wifi_connect` after guard

4. **Normal operation without restart flag:**
   - When `restarting=false`, WiFi operations should proceed normally
   - No guard messages should appear

## Failure Mode Analysis

### Without the Fix (Expected Old Behavior)

**Symptoms:**
- Guru Meditation Error
- WDT timeout
- ESP_ERROR_CHECK abort with backtrace
- Random restarts during OTA
- Node failing to reconnect after OTA

**Example abort location:**
```
ESP_ERROR_CHECK esp_wifi_set_mode() at wifi.c:XXX
```

### With the Fix (Current Behavior)

**Symptoms:**
- Clean reboot with `[RESTART-SAFE-GUARD]` log message
- Node successfully reboots and reconnects
- OTA completes successfully
- No error messages or aborts

## Continuous Integration

These tests are integrated into CI:

1. **Host tests** (`make -C firmware/test test`) - Run on every build
2. **Acceptance tests** (Go tests) - Manual with `SPAXEL_HARDWARE_TEST=1`

## Documentation References

- Firmware implementation: `firmware/main/wifi.c` lines 162-170
- Restart trigger points: `firmware/main/websocket.c` lines 127, 833, 1043
- Test implementations: `firmware/test/test_ota_during_wifi_reconnect.c`, `firmware/test/test_all_restart_trigger_points.c`
- Mothership acceptance: `mothership/test/acceptance/as5_wifi_restart_race_test.go`

## Test Sign-Off

**Tester:** _________________  
**Date:** _________________  
**Node MAC:** _________________  
**Firmware Version Before:** _________________  
**Firmware Version After:** _________________  

**Test Results:**
- [ ] Scenario 1: OTA During WiFi Reconnection - PASSED
- [ ] Scenario 2: Reboot Command During WiFi Reconnection - PASSED  
- [ ] Scenario 3: OTA Timeout During WiFi Reconnection - PASSED
- [ ] Scenario 4: All Three Restart Points - PASSED
- [ ] Automated Host Tests - PASSED
- [ ] Serial logs reviewed - PASSED

**Notes:**
_______________
_______________
_______________
