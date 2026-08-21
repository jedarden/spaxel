# Contributing to Spaxel Firmware

This guide covers how to contribute to the ESP32-S3 firmware while maintaining reliability and avoiding common pitfalls.

## Before You Code

### Read the Error Handling Patterns

**CRITICAL:** The Spaxel firmware uses specific error handling patterns to prevent race conditions and system aborts. **You must read these before modifying any code:**

1. **[Error Handling Patterns](README.md#error-handling-patterns)** in this directory
2. **[ADR-010: Error Handling Patterns](../docs/notes/adr-010-error-handling-patterns.md)** - Architectural decision record
3. **[Race Condition Analysis](../docs/notes/ota-wifi-reconnection-race-summary.md)** - Why these patterns matter

### The Two Critical Rules

#### 1. Never Use ESP_ERROR_CHECK in Application Code

❌ **WRONG:**
```c
ESP_ERROR_CHECK(some_function());  // Will abort on any error
```

✅ **CORRECT:**
```c
esp_err_t err = some_function();
if (err != ESP_OK) {
    ESP_LOGE(TAG, "Operation failed: %s", esp_err_to_name(err));
    return err;  // Let caller decide what to do
}
```

**Why:** `ESP_ERROR_CHECK` aborts the system immediately, creating abort loops for transient errors and races with system restart.

#### 2. Check Restart Flags Before Long-Running Operations

✅ **REQUIRED PATTERN:**
```c
if (g_state.restarting) {
    ESP_LOGW(TAG, "[RESTART-SAFE-GUARD] Skipping operation - restart imminent");
    return ESP_OK;  // Graceful skip, NOT an error
}

// Now perform the operation
esp_err_t err = some_long_operation();
```

**Why:** Prevents races between your operation and imminent system restart (OTA, reboot command, timeout).

**Required flags:**
- `g_state.restarting` - Set when system is about to reboot (6 trigger points)
- `g_state.ota_in_progress` - Set during OTA download

## Common Tasks

### Adding a New WiFi-Related Operation

1. **Add the restart-safe guard at the function entry:**
   ```c
   esp_err_t my_new_wifi_operation(void) {
       if (g_state.restarting) {
           ESP_LOGW(TAG, "[RESTART-SAFE-GUARD] Skipping - restart imminent");
           return ESP_OK;
       }
       // ... rest of implementation
   }
   ```

2. **Use explicit error checking for all ESP-IDF API calls:**
   ```c
   esp_err_t err = esp_wifi_some_api();
   if (err != ESP_OK) {
       ESP_LOGE(TAG, "API failed: %s", esp_err_to_name(err));
       return err;
   }
   ```

3. **Let the state machine handle errors, not your function:**
   ```c
   // In your function
   return err;  // Don't retry, don't abort, just return the error

   // In the state machine (main.c)
   case MY_STATE:
       esp_err_t err = my_new_wifi_operation();
       if (err != ESP_OK) {
           if (g_state.restarting) {
               ESP_LOGW(TAG, "Expected during restart");
           } else {
               ESP_LOGE(TAG, "Will retry with backoff");
               // State machine handles retry
           }
       }
       break;
   ```

### Adding a New Long-Running Operation

Any operation that:
- Takes >100ms to complete
- Calls `vTaskDelay()` or blocks
- Touches hardware (WiFi, BLE, CSI)

**Must** check the restart flag first (see pattern above).

### Adding a New Restart Trigger Point

If your code needs to trigger a system restart:

1. **Set the flag BEFORE preparing for restart:**
   ```c
   g_state.restarting = true;
   // ... prepare for restart (cleanup, etc.) ...
   esp_restart();
   ```

2. **Document the trigger point** in the list in `docs/notes/adr-010-error-handling-patterns.md`

3. **Add test coverage** in `firmware/test/test_all_restart_trigger_points.c`

## Testing Your Changes

### Required Test Coverage

All new operations must include test coverage for:

1. **Restart guard scenarios** - Does the operation skip gracefully when `g_state.restarting` is set?
2. **Error handling** - Does the operation return errors correctly instead of aborting?
3. **State machine integration** - Does the state machine handle the operation's errors correctly?

See existing tests:
- `test/test_ota_during_wifi_reconnect.c`
- `test/test_all_restart_trigger_points.c`
- `test/test_restart_flag_propagation.c`

### Running Tests

```bash
# From the firmware directory
cd /path/to/spaxel/firmware

# Run host-based tests (gcc harness)
make -C test test

# Build and flash to hardware
idf.py build
idf.py -p /dev/ttyUSB0 flash
```

## Code Review Checklist

Before submitting a PR, verify:

- [ ] No `ESP_ERROR_CHECK` in application code (only allowed in `app_main()` before tasks start)
- [ ] Restart guard checked before any long-running operation
- [ ] Explicit error checking used throughout
- [ ] Errors returned to caller, not handled in the function
- [ ] State machine handles retries, not individual functions
- [ ] Test coverage for restart scenarios
- [ ] Documentation updated if patterns changed

## Design Philosophy

### Functions Return Error Codes, Never Abort

Functions should return `esp_err_t` error codes and let the caller decide what to do. This enables:

- **Retry logic** in the state machine
- **Graceful degradation** when errors are transient
- **Clean restarts** when errors are fatal

### State Machine Handles All Recovery

The state machine in `main.c` is responsible for:
- Deciding when to retry operations
- Implementing exponential backoff
- Coordinating between tasks during restart
- Transitioning between states (BOOT → WIFI_LOST → CONNECTED, etc.)

### Flags Coordinate Between Tasks

The global state flags (`g_state.restarting`, `g_state.ota_in_progress`, `g_state.provisioned`) are the coordination mechanism between FreeRTOS tasks. They prevent races by:

1. **Broadcasting intent** - Setting a flag tells all tasks "I'm about to do something"
2. **Checking before acting** - Each task checks flags before starting operations
3. **Graceful skip** - Returning `ESP_OK` from a guard is not an error, it's a skip

## Further Reading

### Architecture & Design
- [Project Plan](../docs/plan/plan.md) - Complete system architecture
- [Component Design](../docs/plan/plan.md#component-design) - Detailed component specifications

### Race Condition Analysis
- [OTA WiFi Reconnection Race Summary](../docs/notes/ota-wifi-reconnection-race-summary.md) - Why guards are needed
- [OTA WiFi Race Investigation](../docs/notes/ota-wifi-race-investigation.md) - Detailed call sequence analysis

### Related ADRs
- [ADR-004](../docs/plan/plan.md#adr-004-2026-07-30) - OTA reliability
- [ADR-006](../docs/plan/plan.md#adr-006-2026-08-07) - Firmware download authentication
- [ADR-007](../docs/plan/plan.md#adr-007-2026-08-07) - Node identity (superseded by ADR-008)
- [ADR-008](../docs/plan/plan.md#adr-008-2026-08-07) - Asymmetric node identity
- [ADR-009](../docs/plan/plan.md#adr-009-2026-08-07) - Automatic firmware convergence

### Testing
- [Manual OTA Test Procedure](../docs/tests/manual-ota-during-wifi-reconnect-test.md) - Hardware testing guide

## Getting Help

If you're unsure about any of these patterns:

1. **Read the existing code** - `wifi_start_connect()` in `wifi.c` is the canonical example
2. **Ask in a PR** - Tag your question with `firmware:` and someone will help
3. **Check the beads** - Search for beads with `firmware` or `error-handling` labels for context

## Thank You

Following these patterns ensures the Spaxel firmware remains:
- **Reliable** - No abort loops or race conditions
- **Maintainable** - Clear error handling and state machine coordination
- **Testable** - Guard scenarios are covered by automated tests

Your contributions help keep Spaxel working reliably in homes!
