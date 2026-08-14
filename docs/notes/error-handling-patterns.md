# Error Handling Patterns in Spaxel Firmware

## Date
2026-08-13

## Overview

This document explains the error handling patterns used in the Spaxel firmware, specifically when to use `ESP_ERROR_CHECK` versus graceful error handling, and how the restart-safe guard pattern prevents race conditions.

## Background: What is ESP_ERROR_CHECK?

`ESP_ERROR_CHECK` is an ESP-IDF macro that:
1. Evaluates an `esp_err_t` expression
2. If result != `ESP_OK`: logs error and **aborts the system with `esp_restart()`**
3. If result == `ESP_OK`: continues execution

```c
ESP_ERROR_CHECK(foo());
// Expands to:
esp_err_t err = foo();
if (err != ESP_OK) {
    ESP_LOGE("TAG", "ESP_ERROR_CHECK: foo() failed: esp_err_to_name(err));
    abort(); // or esp_restart() in many configurations
}
```

### Why ESP_ERROR_CHECK is Dangerous

**Problem**: `ESP_ERROR_CHECK` immediately aborts the system on ANY error. This is appropriate for:
- Fatal initialization failures (NVS, WiFi driver)
- Unrecoverable hardware errors

**But it is INAPPROPRIATE for:**
- Operations that may fail temporarily (WiFi connection, network operations)
- Situations where another task is about to reboot the system
- Operations that race with system restart

In these cases, `ESP_ERROR_CHECK` creates **abort loops** where a temporary error triggers a restart, which then retries the failing operation, causing another abort, ad infinitum.

## Spaxel's Error Handling Philosophy

### Rule 1: NEVER use ESP_ERROR_CHECK in application-level code

The Spaxel firmware **does not use `ESP_ERROR_CHECK`** anywhere in `firmware/main/`. Instead, we use explicit error checking:

```c
esp_err_t err = some_function();
if (err != ESP_OK) {
    ESP_LOGE(TAG, "Operation failed: %s", esp_err_to_name(err));
    // Handle gracefully: return error, retry, delay, or set flag
    return err;
}
```

### Rule 2: Check restart flags before long-running operations

The **restart-safe guard pattern** prevents races between WiFi operations and imminent system restart:

```c
if (g_state.restarting) {
    ESP_LOGW(TAG, "[RESTART-SAFE-GUARD] Skipping operation - restart imminent");
    return ESP_OK;  // NOT an error - graceful skip
}
```

**Where this is required:**
- Any function that calls `esp_wifi_*` APIs (they use ESP_ERROR_CHECK internally)
- Any function with long-running delays (`vTaskDelay`, `esp_http_client_read`)
- Any operation that touches hardware while another task is preparing to restart

**Why returning ESP_OK is correct here:**
- The caller (state machine) expects ESP_OK to mean "operation completed successfully"
- In this case, "skip due to imminent restart" IS successful completion
- The operation will be retried on next boot after restart completes
- Returning an error would cause the state machine to retry immediately, creating the race we're trying to prevent

### Rule 3: Use flags to coordinate between tasks

The firmware uses several global flags to prevent task races:

| Flag | Purpose | Set By | Checked By | Prevents |
|------|---------|--------|------------|----------|
| `g_state.restarting` | System is about to reboot | 6 trigger points | WiFi operations, long tasks | WiFi vs restart race |
| `g_state.ota_in_progress` | OTA download in progress | OTA task | WiFi reconnection path | WiFi vs OTA HTTP race |
| `g_state.provisioned` | Device has WiFi credentials | Provisioning | WiFi connect | Unnecessary connect attempts |

**Implementation Pattern:**
```c
// Task A: Set flag before critical operation
g_state.restarting = true;

// Task B: Check flag before operation that would race
if (g_state.restarting) {
    // Skip or delay operation
    return ESP_OK;
}
```

### Rule 4: Return errors, don't abort

Always return `esp_err_t` error codes to callers. Let the **state machine** decide what to do:

```c
esp_err_t wifi_start_connect(void) {
    // Check restart guard
    if (g_state.restarting) {
        return ESP_OK;  // Graceful skip
    }

    // Attempt WiFi connection
    esp_err_t err = esp_wifi_set_mode(WIFI_MODE_STA);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed to set WiFi mode: %s", esp_err_to_name(err));
        return err;  // Return error, don't abort
    }

    // ... more operations ...
    return ESP_OK;
}
```

The state machine then handles retries, exponential backoff, and state transitions:

```c
case NODE_STATE_WIFI_LOST:
    esp_err_t err = wifi_start_connect();
    if (err != ESP_OK) {
        if (g_state.restarting) {
            ESP_LOGW(TAG, "[RESTART-SAFE-GUARD] WiFi connect skipped during restart (expected)");
        } else {
            ESP_LOGE(TAG, "WiFi reconnect failed, will retry");
        }
    }
    break;
```

## When to Use ESP_ERROR_CHECK

### ONLY in these cases:

1. **One-time initialization during app_main()** - before FreeRTOS tasks start:
   ```c
   void app_main() {
       ESP_ERROR_CHECK(nvs_flash_init());
       ESP_ERROR_CHECK(esp_netif_init());
       // If these fail, system cannot boot - abort is correct
   }
   ```

2. **Hardware driver initialization** - when the driver is required for system operation:
   ```c
   ESP_ERROR_CHECK(esp_event_loop_create_default());
   ```

3. **Tests that explicitly verify abort behavior** - not in production code

### NEVER in these cases:

1. ❌ WiFi operations (`esp_wifi_*` APIs) - use restart guard instead
2. ❌ Network operations (`esp_http_*`, `esp_https_*`) - may fail temporarily
3. ❌ Operations after FreeRTOS tasks start - tasks may be racing with restart
4. ❌ Any operation that can fail transiently - retry gracefully instead

## The Restart-Safe Guard Pattern

### Complete Pattern with Both Guards

```c
esp_err_t wifi_start_connect(void) {
    // Provisioning check
    if (!g_state.provisioned) {
        ESP_LOGW(TAG, "Not provisioned, cannot connect");
        return ESP_ERR_INVALID_STATE;
    }

    // RESTART-SAFE GUARD
    if (g_state.restarting) {
        ESP_LOGW(TAG, "[RESTART-SAFE-GUARD] Skipping WiFi connection - restart flag is set");
        ESP_LOGW(TAG, "[RESTART-SAFE-GUARD] This is a guard-triggered skip, NOT an error");
        ESP_LOGW(TAG, "[RESTART-SAFE-GUARD] WiFi operations will resume after next boot");
        return ESP_OK;  // Graceful skip, NOT an error
    }

    // Normal operation with explicit error checking
    esp_err_t err = esp_wifi_set_mode(WIFI_MODE_STA);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed to set WiFi mode: %s", esp_err_to_name(err));
        return err;
    }

    // ... more operations ...

    return ESP_OK;
}
```

### Coordination with OTA Guard

The state machine uses **both guards** for complete protection:

```c
case NODE_STATE_WIFI_LOST:
    // OTA GUARD: Prevent race with OTA download
    if (g_state.ota_in_progress) {
        ESP_LOGW(TAG, "WiFi lost but OTA in progress - delaying reconnection");
        vTaskDelay(pdMS_TO_TICKS(5000));
        break;
    }

    // Attempt WiFi reconnection
    esp_err_t err = wifi_start_connect();
    if (err != ESP_OK) {
        if (g_state.restarting) {
            // Restart-safe guard triggered - expected
            ESP_LOGW(TAG, "[RESTART-SAFE-GUARD] WiFi reconnect skipped during restart");
        } else {
            // Real error - will retry
            ESP_LOGE(TAG, "WiFi reconnect failed: %s", esp_err_to_name(err));
        }
    }
    break;
```

## Testing and Validation

### Manual Testing

See `docs/tests/manual-ota-during-wifi-reconnect-test.md` for the complete test procedure.

### Automated Testing

The following test files validate the restart-safe guard pattern:

1. **`firmware/test/test_ota_during_wifi_reconnect.c`**
   - Tests OTA flag guard during WiFi reconnection
   - Validates that ESP_ERROR_CHECK abort is prevented

2. **`firmware/test/test_all_restart_trigger_points.c`**
   - Tests all 6 `esp_restart()` trigger points
   - Validates restart flag propagation across tasks

3. **`firmware/test/test_restart_flag_propagation.c`**
   - Tests restart flag visibility across FreeRTOS tasks
   - Validates memory ordering and synchronization

## Related Documentation

- **`docs/notes/ota-wifi-reconnection-race-summary.md`** - Complete analysis of the OTA vs WiFi reconnection race
- **`docs/notes/ota-wifi-race-investigation.md`** - Detailed investigation with call sequences
- **`docs/tests/manual-ota-during-wifi-reconnect-test.md`** - Manual testing procedures
- **`firmware/main/wifi.c:156-259`** - Implementation of restart-safe guard in `wifi_start_connect()`
- **`firmware/main/main.c`** - State machine usage of both guards

## Summary

The restart-safe guard pattern is a **race condition prevention mechanism** that ensures:

1. ✅ WiFi operations never race with imminent system restart
2. ✅ OTA downloads never race with WiFi reconnection
3. ✅ System reboots cleanly without abort loops
4. ✅ All operations retry gracefully after restart

**Key Principles:**
- Check flags before long-running operations
- Return errors, don't abort (except during one-time init)
- Let the state machine decide what to do
- Use explicit error checking, never `ESP_ERROR_CHECK` in application code

**For Contributors:**
When adding new operations that may take time or touch hardware:
1. Check if `g_state.restarting` should block the operation
2. Check if `g_state.ota_in_progress` should block the operation
3. Use explicit error checking, never `ESP_ERROR_CHECK`
4. Return `ESP_OK` for guard-triggered skips (not an error)
5. Add test coverage for the new operation
