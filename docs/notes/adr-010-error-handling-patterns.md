# ADR-010: 2026-08-13 — Use explicit error checking instead of ESP_ERROR_CHECK, with restart-safe guards before long-running operations

## Context

`ESP_ERROR_CHECK` is an ESP-IDF macro that aborts the system on any error. While appropriate for fatal initialization failures, using it in application-level code creates **abort loops** when operations fail temporarily or race with system restart.

During OTA development (2026-08-07), a race condition was discovered where WiFi reconnection attempts interfered with OTA download, causing `ESP_ERROR_CHECK` to abort the system mid-OTA. This led to rollback loops and unreliable updates.

### The Problem with ESP_ERROR_CHECK

1. **Abort loops**: A temporary error triggers restart, which retries the failing operation, causing another abort
2. **Race conditions**: Operations don't check if the system is about to restart before starting
3. **Poor user experience**: Aborts are visible as crashes rather than graceful degradation
4. **Difficult debugging**: Aborts provide no opportunity for graceful error handling or retry logic

### Existing Patterns

The Spaxel firmware already uses **explicit error checking** throughout `firmware/main/`:

```c
esp_err_t err = some_function();
if (err != ESP_OK) {
    ESP_LOGE(TAG, "Operation failed: %s", esp_err_to_name(err));
    return err;  // Never ESP_ERROR_CHECK here
}
```

However, there was no documented **pattern** for when and how to check restart flags before starting operations, and no guidance on where `ESP_ERROR_CHECK` might be appropriate.

## Decision

### 1. Never use ESP_ERROR_CHECK in application-level code

**Prohibited locations:**
- All files under `firmware/main/` (application code)
- Any code that runs after FreeRTOS tasks start
- Any operation that may fail transiently

**Explicit error checking is required:**
```c
esp_err_t err = operation();
if (err != ESP_OK) {
    ESP_LOGE(TAG, "Operation failed: %s", esp_err_to_name(err));
    return err;  // Let caller decide what to do
}
```

### 2. Use ESP_ERROR_CHECK only in one-time initialization

**Allowed locations:**
- `app_main()` before FreeRTOS tasks start
- Driver initialization during system bring-up

**Example:**
```c
void app_main() {
    // These are fatal - if they fail, system cannot boot
    ESP_ERROR_CHECK(nvs_flash_init());
    ESP_ERROR_CHECK(esp_netif_init());
    ESP_ERROR_CHECK(esp_event_loop_create_default());
    
    // Now start tasks...
    xTaskCreate(...);
}
```

### 3. Check restart flags before long-running operations

**The restart-safe guard pattern:**

```c
if (g_state.restarting) {
    ESP_LOGW(TAG, "[RESTART-SAFE-GUARD] Skipping operation - restart imminent");
    return ESP_OK;  // Graceful skip, NOT an error
}
```

**Required before:**
- Any `esp_wifi_*` API call (they use ESP_ERROR_CHECK internally)
- Any operation with `vTaskDelay` or long blocking calls
- Any operation that touches hardware while restart is imminent

**Why returning ESP_OK is correct:**
- The operation completed successfully (by skipping)
- Returning an error would cause immediate retry, creating the race
- The operation will be retried on next boot after restart completes

### 4. Coordinate between tasks using flags

**Required flags:**
- `g_state.restarting` - Set when system is about to reboot (6 trigger points)
- `g_state.ota_in_progress` - Set during OTA download to block WiFi reconnection
- `g_state.provisioned` - Set when device has WiFi credentials

**Usage pattern:**
```c
// Task A: Set flag before critical operation
g_state.restarting = true;
// ... prepare for restart ...
esp_restart();

// Task B: Check flag before operation
if (g_state.restarting) {
    return ESP_OK;  // Skip gracefully
}
```

### 5. State machine handles errors, not individual functions

**Function responsibility:** Return error codes, never abort

```c
esp_err_t wifi_start_connect(void) {
    if (g_state.restarting) {
        return ESP_OK;  // Guard triggered
    }
    
    esp_err_t err = esp_wifi_set_mode(WIFI_MODE_STA);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed: %s", esp_err_to_name(err));
        return err;  // Return error, don't abort
    }
    return ESP_OK;
}
```

**State machine responsibility:** Decide what to do with errors

```c
case NODE_STATE_WIFI_LOST:
    esp_err_t err = wifi_start_connect();
    if (err != ESP_OK) {
        if (g_state.restarting) {
            ESP_LOGW(TAG, "[RESTART-SAFE-GUARD] Expected during restart");
        } else {
            ESP_LOGE(TAG, "Real error, will retry");
            // State machine will retry with exponential backoff
        }
    }
    break;
```

## Alternatives Considered

### 1. Use ESP_ERROR_CHECK everywhere
**Rejected:** Creates abort loops for transient errors. WiFi disconnects, network timeouts, and temporary hardware states are all **expected failures** that should be retried, not cause system restart.

### 2. Wrap all ESP-IDF APIs with guards
**Rejected:** Too invasive. The restart guard is a **pattern**, not a wrapper. Each function checks the flag once at entry, not around every API call. This keeps code readable and maintainable.

### 3. Use global try-catch mechanism
**Rejected:** C has no try-catch. FreeRTOS tasks have no exception handling. The flag pattern is the idiomatic C/FreeRTOS approach to coordination.

### 4. Let ESP-IDF handle errors with its own error checking
**Rejected:** ESP-IDF uses `ESP_ERROR_CHECK` internally in many APIs. We cannot change that. Our guards prevent calling those APIs when they would abort, providing the safety layer ESP-IDF doesn't have.

## Consequences

### Positive

1. **No abort loops**: Transient errors are retried gracefully
2. **Clean reboots**: System restarts without races between tasks
3. **Reliable OTA**: Updates complete without interruption from WiFi reconnection
4. **Better debugging**: Errors are logged and handled, not silently aborted
5. **Predictable behavior**: Flag checks are explicit and logged with `[RESTART-SAFE-GUARD]` prefix

### Cost

1. **More code**: Explicit error checking requires more lines than `ESP_ERROR_CHECK(err)`
2. **Flag discipline**: Developers must remember to check flags before operations
3. **Testing burden**: Each new operation needs test coverage for guard scenarios

### Risk

1. **Forgotten checks**: A new long-running operation without restart guard could reintroduce races
   - **Mitigation**: Code review must check for flag checks before blocking operations
   - **Mitigation**: Add lint rule or static analysis to detect missing guards

2. **Flag race conditions**: If flag is set after check but before operation, race could still occur
   - **Mitigation**: `volatile bool` ensures visibility across tasks
   - **Mitigation**: Flag is set well before `esp_restart()` (not immediately before)

### Migration Path

**Existing code:**
- Most code already uses explicit error checking (grep shows no `ESP_ERROR_CHECK` in `firmware/main/`)
- Restart guards are already implemented in `wifi_start_connect()` (lines 162-170)
- OTA guard is already implemented in state machine (main.c lines 362-367)

**New code:**
- Follow the pattern in `docs/notes/error-handling-patterns.md`
- Add flag checks before any new long-running operation
- Use explicit error checking, never `ESP_ERROR_CHECK`

## Implementation Status

### Completed ✅

1. **Restart-safe guard in `wifi_start_connect()`** (wifi.c:162-170)
   - Checks `g_state.restarting` before WiFi operations
   - Returns ESP_OK for graceful skip
   - Comprehensive logging with `[RESTART-SAFE-GUARD]` prefix

2. **OTA guard in state machine** (main.c:362-367)
   - Checks `g_state.ota_in_progress` before WiFi reconnection
   - Delays 5 seconds instead of reconnecting
   - Prevents race with OTA HTTP download

3. **Documentation** (docs/notes/error-handling-patterns.md)
   - Complete explanation of error handling philosophy
   - Pattern examples with code
   - Testing guidelines
   - Related documentation references

### In Progress 🔄

1. **Test coverage expansion**
   - Add tests for new operations that use the pattern
   - Validate all 6 restart trigger points (test_all_restart_trigger_points.c)

2. **Developer onboarding**
   - Add pattern to firmware contributor guide
   - Add code review checklist for flag checks

## References

### Related ADRs

- **ADR-004**: OTA reliability - making OTA trustworthy (covers OTA guard)
- **ADR-006**: Authenticate firmware downloads (OTA URL routing)

### Related Issues

- **bf-xss9y**: OTA vs WiFi reconnection race condition
- **bf-3c282**: WebSocket reconnect race
- **bf-5fr9b**: Canary rollback verification

### Documentation

- **`docs/notes/error-handling-patterns.md`** - Complete error handling pattern guide
- **`docs/notes/ota-wifi-reconnection-race-summary.md`** - OTA vs WiFi race analysis
- **`docs/notes/ota-wifi-race-investigation.md`** - Detailed investigation with call sequences
- **`docs/tests/manual-ota-during-wifi-reconnect-test.md`** - Manual test procedures

### Test Files

- **`firmware/test/test_ota_during_wifi_reconnect.c`**
- **`firmware/test/test_all_restart_trigger_points.c`**
- **`firmware/test/test_restart_flag_propagation.c`**

## Follow-up

**Tracked as:** bf-4oc0e (this bead)

**Future work:**
1. Add static analysis or lint rule to detect missing restart guards
2. Create firmware contributor guide with this pattern as a core principle
3. Add code review checklist item: "Does this operation check restart flags?"
4. Consider adding a helper macro to standardize guard logging

---

**Status:** ACCEPTED - This is the standard error handling pattern for Spaxel firmware.
