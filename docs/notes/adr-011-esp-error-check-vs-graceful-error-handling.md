# ADR-011: 2026-08-23 — ESP_ERROR_CHECK vs Graceful Error Handling

## Context

ESP-IDF provides `ESP_ERROR_CHECK` as a convenience macro for error handling:

```c
ESP_ERROR_CHECK(expr);
```

This macro checks the return value, logs an error if it's not `ESP_OK`, and **aborts the program** by calling `esp_restart()`.

While `ESP_ERROR_CHECK` is useful during development and one-time initialization, it presents a critical problem for production firmware: **any failure results in an immediate system restart**, with no opportunity for:
- Graceful degradation
- Coordinating between concurrent tasks
- Preserving system state
- Implementing retry logic
- Logging diagnostic information

This becomes particularly dangerous in multi-task systems where operations can fail transiently due to:
- Network disconnects (WiFi race conditions)
- Resource contention
- Temporary hardware states
- OTA updates in progress

The WiFi race condition (documented in `ota-wifi-reconnection-race-summary.md`) exemplifies this: using `ESP_ERROR_CHECK` during WiFi reconnection while OTA was active caused system crashes mid-OTA, potentially corrupting the OTA partition.

## Decision

**We will NOT use `ESP_ERROR_CHECK` in application-level code.** Instead, we will use explicit error checking with graceful error handling.

### Usage Rules

#### ✅ ALLOWED: ESP_ERROR_CHECK in One-Time Initialization

`ESP_ERROR_CHECK` may ONLY be used in `app_main()` before FreeRTOS tasks start:

```c
void app_main(void) {
    // One-time system initialization - ESP_ERROR_CHECK is acceptable
    ESP_ERROR_CHECK(nvs_flash_init());
    ESP_ERROR_CHECK(esp_netif_init());
    ESP_ERROR_CHECK(esp_event_loop_create_default());

    // After tasks start, ESP_ERROR_CHECK is PROHIBITED
    xTaskCreate(state_machine_task, "state_machine", 8192, NULL, 5, NULL);
    // ...
}
```

**Rationale:** Failures during system bring-up indicate a fundamental problem that cannot be recovered (e.g., corrupted NVS, missing hardware). Restart is the only safe option.

#### ❌ PROHIBITED: ESP_ERROR_CHECK in Application Code

All files under `firmware/main/` MUST use explicit error checking:

```c
// WRONG - Will crash on transient failure
esp_err_t wifi_start_connect(void) {
    ESP_ERROR_CHECK(esp_wifi_set_mode(WIFI_MODE_STA));  // PROHIBITED
    ESP_ERROR_CHECK(esp_wifi_start());                 // PROHIBITED
    return ESP_OK;
}

// CORRECT - Returns error for caller to decide
esp_err_t wifi_start_connect(void) {
    // Restart-safe guard
    if (g_state.restarting) {
        ESP_LOGW(TAG, "[RESTART-SAFE-GUARD] Skipping WiFi connection - restart flag is set");
        return ESP_OK;  // Graceful skip, NOT an error
    }

    // Explicit error checking - NEVER ESP_ERROR_CHECK
    esp_err_t err = esp_wifi_set_mode(WIFI_MODE_STA);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed to set WiFi mode: %s", esp_err_to_name(err));
        return err;  // Return error, don't abort
    }

    err = esp_wifi_start();
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed to start WiFi: %s", esp_err_to_name(err));
        return err;
    }

    return ESP_OK;
}
```

### Error Handling Pattern

1. **Check restart flags before operations** (restart-safe guard pattern)
2. **Return error codes** to the caller
3. **Let the state machine decide** retry logic, fallback behavior, or state transitions
4. **Coordinate between tasks** using flags (`g_state.restarting`, `g_state.ota_in_progress`)

### State Machine Error Handling

```c
case NODE_STATE_WIFI_LOST:
    // Check if OTA is in progress - if so, delay reconnection
    if (g_state.ota_in_progress) {
        ESP_LOGW(TAG, "WiFi lost but OTA in progress - delaying reconnection");
        vTaskDelay(pdMS_TO_TICKS(5000));
        break;
    }

    esp_err_t err = wifi_start_connect();
    if (err != ESP_OK) {
        if (g_state.restarting) {
            ESP_LOGW(TAG, "[RESTART-SAFE-GUARD] WiFi reconnect skipped during restart (expected)");
        } else {
            ESP_LOGE(TAG, "WiFi reconnect failed unexpectedly: %s", esp_err_to_name(err));
            // State machine can decide: retry, fallback, or transition to error state
        }
    }
    break;
```

## Alternatives Considered

### Alternative 1: Use ESP_ERROR_CHECK Everywhere

**Approach:** Use `ESP_ERROR_CHECK` throughout the codebase for consistency.

**Rejected because:**
- Any transient failure (WiFi drop, temporary resource contention) crashes the system
- No coordination between tasks (OTA vs WiFi reconnection race)
- No graceful degradation possible
- System becomes unstable under normal operating conditions
- Impossible to implement retry logic or fallback behaviors

### Alternative 2: Custom Error-Handling Macro with Conditional Restart

**Approach:** Create a macro that only restarts on "fatal" errors.

**Rejected because:**
- Adds complexity (what counts as "fatal"?)
- Still couples error handling to restart logic
- Explicit error checking is clearer and more flexible
- State machine is better positioned to decide recovery strategy

### Alternative 3: C++ Exceptions

**Approach:** Use C++ exceptions for error handling.

**Rejected because:**
- ESP-IDF is primarily C-based
- Exception handling adds significant code size and runtime overhead
- Not idiomatic in embedded systems
- Intertwining with FreeRTOS tasks is complex

## Consequences

### Positive Impacts

1. **System stability:** Transient failures no longer crash the system
2. **Coordination:** Tasks can coordinate via flags (e.g., OTA vs WiFi reconnection)
3. **Recovery:** State machine can implement retry logic, fallback behaviors, and graceful degradation
4. **Observability:** Errors are logged before recovery actions, making issues easier to diagnose
5. **Testability:** Error paths can be tested without triggering system restarts

### Costs and Complexity

1. **More verbose code:** Explicit error checking requires more lines than `ESP_ERROR_CHECK`
2. **Must remember rules:** Developers need to know when to use restart-safe guards
3. **State machine complexity:** Error handling logic lives in the state machine rather than being centralized

### Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Developers forget to check errors | Code review checklist; CI linter to catch `ESP_ERROR_CHECK` in application code |
| Unhandled errors cause silent failures | State machine must handle all error returns; logged at each failure point |
| Restart-safe guards forgotten | Documented in `error-handling-patterns.md`; enforced via code review |

## Implementation Status

✅ **Completed:**
- Restart-safe guard pattern implemented in WiFi driver (`firmware/main/wifi.c:162-225`)
- OTA vs WiFi reconnection coordination (`firmware/main/main.c:347-350`)
- State machine error handling throughout (`firmware/main/main.c`)
- Documentation in `error-handling-patterns.md`

🔄 **In Progress:**
- None currently

## References

- **Related ADRs:**
  - [ADR-010: Error Handling Patterns](adr-010-error-handling-patterns.md) — General error handling philosophy
  - [ADR-004: OTA Reliability](../../../../../.beads/checkpoint/objects/gen-681f7f0d15f6fabfc11edc46c71facdd.jsonl) — OTA design decisions

- **Documentation:**
  - [Error Handling Patterns](error-handling-patterns.md) — Comprehensive pattern guide
  - [OTA WiFi Reconnection Race Summary](ota-wifi-reconnection-race-summary.md) — WiFi race condition analysis

- **Implementation:**
  - Restart-safe guard: `firmware/main/wifi.c:162-225`
  - OTA coordination: `firmware/main/main.c:347-350`
  - State machine error handling: `firmware/main/main.c:355-365`

- **Tests:**
  - OTA during WiFi reconnect: `firmware/test/test_ota_during_wifi_reconnect.c`
  - All restart trigger points: `firmware/test/test_all_restart_trigger_points.c`
  - Test planning: `docs/notes/wifi-restart-race-test-plan.md`

- **Related Issues:**
  - This bead: `spaxel-b41a6dd3`
  - WiFi race investigation: (see `ota-wifi-race-investigation.md`)

## Follow-up

**Tracking bead ID:** `spaxel-b41a6dd3`

**Future Work:**
- None currently — the pattern is well-established and documented

---

**Decision Date:** 2026-08-23
**Status:** Accepted
**Applies To:** All firmware code under `firmware/main/`
