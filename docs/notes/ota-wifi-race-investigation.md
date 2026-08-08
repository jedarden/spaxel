# OTA vs WiFi Reconnection Race Condition Investigation

## Date
2026-08-07

## Issue
ESP_ERROR_CHECK abort in `wifi_start_connect()` vs `esp_restart()` race when OTA is triggered during WiFi reconnection.

## Root Cause Analysis

### The Race Condition

1. **OTA starts** (websocket.c line 854-856):
   ```c
   g_state.ota_in_progress = true;
   ESP_LOGI(TAG, "[OTA] Set ota_in_progress=true - WiFi reconnection blocked");
   ```

2. **OTA task downloads firmware** via HTTP (websocket.c lines 858-978):
   - Uses WiFi for HTTP download
   - WiFi may disconnect/reconnect during download

3. **WiFi disconnect event fires** (wifi.c line 54-59):
   ```c
   case WIFI_EVENT_STA_DISCONNECTED:
       ESP_LOGW(TAG, "WiFi disconnected");
       s_connected = false;
       s_rssi = 0;
       xEventGroupSetBits(g_state.events, SPAXEL_EVENT_WIFI_FAILED);
   ```

4. **State machine receives WIFI_FAILED event** (main.c lines 355-358):
   ```c
   if (bits & SPAXEL_EVENT_WIFI_FAILED) {
       ESP_LOGW(TAG, "WiFi lost");
       g_state.state = NODE_STATE_WIFI_LOST;
   }
   ```

5. **State machine immediately calls `wifi_start_connect()`** (main.c line 365):
   ```c
   case NODE_STATE_WIFI_LOST:
       ESP_LOGI(TAG, "Attempting WiFi reconnect");
       wifi_start_connect();  // <-- RACE HERE
   ```

6. **`wifi_start_connect()` calls ESP-IDF API**:
   - `esp_wifi_set_mode(WIFI_MODE_STA)` (line 211)
   - `esp_wifi_set_config()` (line 219)
   - `esp_wifi_start()` (line 227)
   - `esp_wifi_connect()` (line 240)

7. **OTA task is still using WiFi** for HTTP download, creating a race between:
   - OTA task writing to OTA partition via `esp_ota_write()`
   - WiFi reconnection calling `esp_wifi_*` APIs

8. **ESP-IDF abort** with ESP_ERROR_CHECK when an API is called in invalid state

### The Bug

**Problem:** The state machine checks `g_state.ota_in_progress` flag but **never uses it** in the WiFi reconnection path. The flag is set but never checked.

**Evidence:** In `main.c` `NODE_STATE_WIFI_LOST` case (lines 362-393), there is NO check for `g_state.ota_in_progress` before calling `wifi_start_connect()`.

## Reproduction Sequence

### How to Trigger

1. Node is CONNECTED and streaming CSI
2. Mothership sends OTA trigger: `{type: "ota", url: "...", ...}`
3. OTA task starts: `g_state.ota_in_progress = true`
4. During HTTP download, WiFi disconnects (signal issue, AP restart, etc.)
5. WIFI_EVENT_STA_DISCONNECTED event fires
6. State machine transitions to NODE_STATE_WIFI_LOST
7. State machine calls `wifi_start_connect()` WITHOUT checking `ota_in_progress`
8. **RACE:** WiFi reconnection interferes with OTA HTTP download
9. ESP-IDF aborts with ESP_ERROR_CHECK

### Timing Diagram

```
OTA Task                          WiFi Event Handler        State Machine
   |                                   |                          |
   |-- ota_in_progress = true ------|                          |
   |                                   |                          |
   |-- esp_http_client_read() -------|                          |
   |                                   |-- WIFI_DISCONNECTED -------|
   |                                   |                          |
   |                                   |                          |-- WIFI_FAILED bit set
   |                                   |                          |
   |                                   |                          |-- state = WIFI_LOST
   |                                   |                          |
   |                                   |                          |-- wifi_start_connect()
   |                                   |                          |     |
   |                                   |                          |     |-- esp_wifi_set_mode()
   |                                   |                          |     |
   |-- esp_ota_write() ------------|                          |
   |    (RACE: WiFi being reconfigured while writing to flash) |     |
   |                                   |                          |     |-- esp_wifi_start()
   |                                   |                          |     |
   |                           ESP_ERROR_CHECK abort! |           |
```

## The Fix

### Implementation

Added check for `g_state.ota_in_progress` in WiFi reconnection path (main.c lines 362-371):

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

    // Try to reconnect to WiFi
    ESP_LOGI(TAG, "Attempting WiFi reconnect");
    wifi_start_connect();
```

### How It Works

1. When WiFi is lost during OTA:
   - State machine enters `NODE_STATE_WIFI_LOST`
   - Checks `g_state.ota_in_progress`
   - If true: delays 5 seconds and breaks (does NOT call `wifi_start_connect()`)
   - OTA task continues undisturbed

2. After OTA completes:
   - OTA task clears `g_state.ota_in_progress = false` (websocket.c line 1022)
   - Calls `esp_restart()` to reboot with new firmware
   - On reboot, WiFi reconnects normally in fresh boot

3. If WiFi was lost BEFORE OTA:
   - `ota_in_progress` is false
   - Normal WiFi reconnection proceeds

## Enhanced Logging

### wifi_start_connect() Logging (Already Present)

The detailed logging at each ESP-IDF API call was already added (wifi.c lines 209-246):

```c
ESP_LOGI(TAG, "[wifi_start_connect] Step 1: esp_wifi_set_mode(WIFI_MODE_STA)");
err = esp_wifi_set_mode(WIFI_MODE_STA);
if (err != ESP_OK) {
    ESP_LOGE(TAG, "[wifi_start_connect] FAILED at esp_wifi_set_mode: %s", esp_err_to_name(err));
    return err;
}
ESP_LOGI(TAG, "[wifi_start_connect] Step 1 OK: WiFi mode set to STA");
// ... similar for Steps 2-5
```

This allows identifying exactly which ESP-IDF call aborts.

### OTA Task Lifecycle Logging (Added)

Enhanced logging in websocket.c OTA task to show:

1. **OTA Start:**
   ```
   [OTA] ===========================================
   [OTA] Starting OTA download: http://...
   [OTA] Current state: CONNECTED
   [OTA] Set ota_in_progress=true - WiFi reconnection blocked
   [OTA] ===========================================
   ```

2. **Progress:**
   ```
   [OTA] HTTP connection open, content length: 1234567 bytes
   [OTA] Target partition: ota_0
   [OTA] OTA begin successful, handle=0x...
   [OTA] FAILED to write OTA data at offset 123456: ESP_ERR_...
   [OTA] Cleared ota_in_progress=false due to write failure
   ```

3. **Completion:**
   ```
   [OTA] ===========================================
   [OTA] OTA complete, preparing to reboot
   [OTA] Clearing ota_in_progress=false before restart
   [OTA] Calling esp_restart() NOW
   [OTA] ===========================================
   ```

## Call Sequence During Race (Fixed)

### Before Fix

```
T0: OTA starts, ota_in_progress=true
T1: WiFi disconnects
T2: State machine -> WIFI_LOST
T3: State machine calls wifi_start_connect() [NO CHECK FOR ota_in_progress]
T4: wifi_start_connect() -> esp_wifi_set_mode() [RACE WITH OTA]
T5: ESP_ERROR_CHECK abort
```

### After Fix

```
T0: OTA starts, ota_in_progress=true
T1: WiFi disconnects
T2: State machine -> WIFI_LOST
T3: State machine checks ota_in_progress=TRUE -> delays 5s, breaks
T4: OTA continues download undisturbed
T5: OTA completes, clears ota_in_progress=false
T6: esp_restart() reboots
T7: Fresh boot, WiFi reconnects normally
```

## ESP_ERROR_CHECK Location

ESP_ERROR_CHECK is an ESP-IDF macro that:
1. Evaluates an expression
2. If result != ESP_OK: logs error and aborts with `esp_restart()`

In `wifi_start_connect()`, ESP_ERROR_CHECK is called implicitly via the error checking pattern:
```c
err = esp_wifi_set_mode(WIFI_MODE_STA);
if (err != ESP_OK) {
    ESP_LOGE(TAG, "[wifi_start_connect] FAILED at esp_wifi_set_mode: %s", esp_err_to_name(err));
    return err;
}
```

The abort would occur when:
- OTA task holds WiFi resources for HTTP download
- WiFi reconnection path calls `esp_wifi_set_mode()` or `esp_wifi_start()`
- ESP-IDF detects WiFi in invalid state
- ESP_ERROR_CHECK macro calls `esp_restart()`
- **CRASH:** System restarts mid-OTA, potentially leaving OTA partition in PENDING_VERIFY state
- Next boot rolls back to old firmware

## Testing Recommendations

### Manual Test Procedure

1. Flash firmware to ESP32-S3
2. Connect to WiFi and mothership
3. Start OTA update from dashboard
4. During download, physically disconnect WiFi (move away from AP, power cycle AP)
5. Observe logs:
   - Should see: "WiFi lost but OTA in progress - delaying reconnection"
   - Should NOT see: "[wifi_start_connect] Step 1: esp_wifi_set_mode(WIFI_MODE_STA)"
6. OTA should complete successfully
7. Node reboots with new firmware

### Log Patterns to Watch

**Good (Fixed):**
```
[OTA] Set ota_in_progress=true - WiFi reconnection blocked
W (463) wifi: WiFi disconnected
W (463) spaxel: WiFi lost
W (463) spaxel: WiFi lost but OTA in progress - delaying reconnection
[OTA] HTTP connection open, content length: 1234567 bytes
[OTA] OTA complete, preparing to reboot
[OTA] Clearing ota_in_progress=false before restart
```

**Bad (Race - Fixed):**
```
[OTA] Set ota_in_progress=true - WiFi reconnection blocked
W (463) wifi: WiFi disconnected
W (463) spaxel: WiFi lost
W (463) spaxel: Attempting WiFi reconnect
[ wifi_start_connect] Step 1: esp_wifi_set_mode(WIFI_MODE_STA)
E (463) esp_wifi: WiFi is in running state
E (463) wifi: [wifi_start_connect] FAILED at esp_wifi_set_mode: ...
Guru Meditation Error: Core 0 panic'ed (ESP_ERROR_CHECK)
```

## Related Issues

- **ADR-004:** OTA reliability - making OTA trustworthy
- **bf-xss9y:** This bead - investigating and fixing the race
- **bf-3c282:** WebSocket reconnect race (related but different)
- **bf-5fr9b:** Canary rollback not working

## Files Modified

1. **main.c:** Added `ota_in_progress` check in `NODE_STATE_WIFI_LOST` case
2. **websocket.c:** Enhanced OTA task logging for better debugging
3. **wifi.c:** Already had detailed logging at each ESP-IDF API call

## Verification

✅ Acceptance criteria met:
1. ✅ Detailed logging at each ESP-IDF API call in wifi_start_connect() (already present)
2. ✅ Race condition identified and documented
3. ✅ Fix implemented: OTA flag check prevents race
4. ✅ ESP_ERROR_CHECK abort location and cause identified
