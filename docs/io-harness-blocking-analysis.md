# IO Harness Blocking Analysis

**Date:** 2026-08-27  
**Scope:** Review websocket.c, ntp.c, ntpserver.go, and OTA changes for IO harness blocking  
**Related Bead:** spaxel-4a5bfadb

## Executive Summary

This analysis reviews code changes from August 1, 2026 and August 27, 2026 to identify potential blocking paths in the IO harness. The review covers:

1. **websocket.c recursive-mutex rewrite** (commit dbd27a9) - **LOW RISK** (designed to prevent blocking)
2. **ntp.c esp_sntp_stop() fix** (commit 0c5afc0) - **MEDIUM RISK** (potential blocking in esp_sntp_stop())
3. **mothership/internal/ntpserver/** (commit 02a5719) - **LOW RISK** (Go goroutine-based)
4. **OTA validation changes** - **LOW RISK** (timer-based, asynchronous)
5. **Diagnostic instrumentation** (commit c5053f4) - **NO RISK** (acceptance test harness only)

## Detailed Analysis

### 1. websocket.c Recursive-Mutex Rewrite (commit dbd27a9)

**Purpose:** Fix WS reconnect deadlock/crash/race after disconnect

**Key Changes:**
- Changed from `s_tx_mutex` (simple mutex) to `s_ws_mutex` (recursive mutex)
- `websocket_connect()` now calls `websocket_disconnect()` **before** taking the lock
- `websocket_disconnect()` releases lock before calling `esp_websocket_client_stop()`/_destroy()

**Code Location:** `firmware/main/websocket.c:193-328`

```c
bool websocket_connect(const char *host, uint16_t port) {
    // Tear down any existing client BEFORE taking the lock below.
    // esp_websocket_client_stop()/_destroy() can block for a while...
    websocket_disconnect();
    vTaskDelay(pdMS_TO_TICKS(100));
    
    xSemaphoreTakeRecursive(s_ws_mutex, portMAX_DELAY);
    // ... connection setup ...
    xSemaphoreGiveRecursive(s_ws_mutex);
}
```

**IO Harness Blocking Risk: LOW**

**Analysis:**
- **POSITIVE:** The code explicitly acknowledges that `esp_websocket_client_stop()`/_destroy() can block
- **POSITIVE:** The design releases the lock before calling blocking functions
- **POSITIVE:** `portMAX_DELAY` on `xSemaphoreTakeRecursive()` only blocks until lock acquisition, not during operations
- **CAUTION:** `websocket_send_csi()` holds the lock during send with `portMAX_DELAY` timeout

**Specific Blocking Path:**
```c
// firmware/main/websocket.c:440-444
xSemaphoreTakeRecursive(s_ws_mutex, portMAX_DELAY);
int sent = (s_connected && s_ws)
    ? esp_websocket_client_send_bin(s_ws, (char *)frame, frame_len, portMAX_DELAY)
    : -1;
xSemaphoreGiveRecursive(s_ws_mutex);
```

If the WebSocket send blocks for an extended period, it will hold `s_ws_mutex`, preventing other tasks from checking connection status. However:
- This is expected behavior for a send operation
- The send timeout is controlled by the WebSocket client's network timeout (30s in config)
- CSI TX tasks are expected to queue behind legitimate sends

**Recommendation:** NO ROLLBACK NEEDED. The change was specifically designed to prevent blocking.

---

### 2. ntp.c esp_sntp_stop() Fix (commit 0c5afc0)

**Purpose:** Stop SNTP client before reconfiguring to prevent assertion crash

**Key Change:**
- Added `esp_sntp_stop()` call at the beginning of `ntp_start_sync()`

**Code Location:** `firmware/main/ntp.c:76-89`

```c
esp_err_t ntp_start_sync(const char *ntp_server) {
    // ... existing code ...
    
    // Stop any already-running SNTP client before reconfiguring.
    // Calling esp_sntp_setoperatingmode() while the client is running is a hard
    // ESP-IDF assertion failure...
    esp_sntp_stop();
    
    // Configure SNTP
    esp_sntp_setoperatingmode(SNTP_OPMODE_POLL);
    esp_sntp_setservername(0, s_ntp_server);
    esp_sntp_init();
}
```

**IO Harness Blocking Risk: MEDIUM**

**Analysis:**
- **CONCERN:** `esp_sntp_stop()` is a blocking call that waits for the SNTP task to terminate
- **CONTEXT:** This function is called from:
  - Initial WiFi connect (one-time per boot)
  - WiFi reconnect after connection loss (NODE_STATE_WIFI_LOST recovery)
  - Runtime NTP server change pushed from mothership

**Potential Blocking Scenarios:**
1. **WiFi Reconnect Path:** If WiFi hiccups frequently, each reconnect calls `ntp_start_sync()`, which blocks on `esp_sntp_stop()`
2. **Concurrent State Machine:** The main state machine in `main.c` calls this during WiFi recovery, potentially blocking other state transitions

**Code Path from main.c:**
- The NTP sync is triggered after WiFi connects
- No separate task is spawned for NTP operations
- The state machine blocks until NTP sync completes

**Recommendation:** MONITOR but NO IMMEDIATE ROLLBACK. The fix prevents a hard crash (assertion failure). If blocking becomes problematic, consider:
- Moving NTP operations to a dedicated task
- Adding a timeout to `esp_sntp_stop()` (if ESP-IDF supports it)
- Checking if SNTP is already running before stopping

---

### 3. mothership/internal/ntpserver/ (commit 02a5719)

**Purpose:** Embedded SNTP server for internet-isolated deployments

**Key Implementation:**
- Go-based UDP server running in a background goroutine
- Simple packet-per-request model with no blocking operations

**Code Location:** `mothership/internal/ntpserver/server.go:52-98`

```go
func (s *Server) serve() {
    buf := make([]byte, packetSize)
    for {
        n, clientAddr, err := s.conn.ReadFromUDP(buf)
        if err != nil {
            return // socket closed (shutdown) or fatal read error
        }
        if n < packetSize {
            continue // too short to be a real NTP/SNTP request; ignore
        }
        
        resp, err := buildResponse(buf[:packetSize], time.Now())
        if err != nil {
            continue
        }
        if _, err := s.conn.WriteToUDP(resp, clientAddr); err != nil {
            log.Printf("[WARN] ntpserver: write to %s failed: %v", clientAddr, err)
        }
    }
}
```

**IO Harness Blocking Risk: LOW**

**Analysis:**
- **POSITIVE:** Runs in a dedicated goroutine (`go s.serve()` in `Start()`)
- **POSITIVE:** No blocking operations outside of UDP read/write (which are expected)
- **POSITIVE:** Errors return immediately, no retry loops
- **POSITIVE:** The code is stateless and handles each request independently

**Recommendation:** NO ROLLBACK NEEDED. This is mothership-side code (Go), not firmware, and is properly isolated in a goroutine.

---

### 4. OTA Validation Changes

**Purpose:** Validate OTA partition before marking it as permanent

**Key Implementation:**
- Timer-based validation with 60-second timeout
- Two-phase confirmation: role message + CSI flow detection
- Callback-based architecture

**Code Location:** `firmware/main/websocket.c:93-172`

```c
static void confirm_ota_valid(void) {
    if (s_ota_confirmed) {
        return;
    }
    s_ota_confirmed = true;
    if (s_ota_check_timer) {
        esp_timer_stop(s_ota_check_timer);
    }
    if (s_ota_valid_timer) {
        esp_timer_stop(s_ota_valid_timer);
    }
    if (is_running_ota_partition()) {
        esp_err_t err = esp_ota_mark_app_valid_cancel_rollback();
        // ... error handling ...
    }
}
```

**IO Harness Blocking Risk: LOW**

**Analysis:**
- **POSITIVE:** Timer-based, no polling or blocking waits
- **POSITIVE:** Callbacks execute in timer context, not main state machine
- **CAUTION:** `esp_ota_mark_app_valid_cancel_rollback()` in `confirm_ota_valid()` could block
- **CONTEXT:** This is called from:
  - `ota_check_cb()` - timer callback (every 2 seconds)
  - Not from the main state machine or IO harness

**Potential Blocking Path:**
- `esp_ota_mark_app_valid_cancel_rollback()` writes to flash, which can block
- However, this is called from a timer callback, not from the IO harness
- The flash write operation is asynchronous in ESP-IDF

**Recommendation:** NO ROLLBACK NEEDED. The OTA validation is properly isolated in timer callbacks and does not interfere with IO harness operations.

---

### 5. Diagnostic Instrumentation (commit c5053f4)

**Purpose:** Add comprehensive diagnostic instrumentation to acceptance tests

**Key Implementation:**
- Go-based diagnostic helper for test harness
- Goroutine dumps every 30 seconds
- Phase tracking and timeout detection

**Code Location:** `test/acceptance/diagnostics.go`

```go
func (d *DiagnosticHelper) periodicDump() {
    defer d.wg.Done()
    
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    // Initial dump
    d.dumpGoroutines("initial")
    
    for {
        select {
        case <-d.stopChan:
            d.dumpGoroutines("final")
            return
        case <-ticker.C:
            d.dumpGoroutines("periodic")
        }
    }
}
```

**IO Harness Blocking Risk: NONE**

**Analysis:**
- **POSITIVE:** This is acceptance test infrastructure only, not production firmware
- **POSITIVE:** Runs in Go, not on the ESP32
- **POSITIVE:** Designed to detect blocking, not cause it

**Recommendation:** NO ROLLBACK NEEDED. This is test infrastructure that helps identify the very problems this analysis is investigating.

---

## Summary of Blocking Paths

### High Confidence - Non-Blocking:
1. **websocket.c mutex redesign** - Explicitly designed to prevent blocking
2. **ntpserver.go** - Properly isolated in goroutine
3. **OTA validation** - Timer-based, asynchronous
4. **Diagnostic instrumentation** - Test infrastructure only

### Requires Monitoring:
1. **ntp.c `esp_sntp_stop()` call** - MEDIUM RISK
   - Blocks during WiFi reconnect scenarios
   - Prevents hard crash (assertion failure)
   - Should be monitored for excessive blocking

### Potential Blocking Locations (by code location):

1. **`firmware/main/ntp.c:87` - `esp_sntp_stop()`**
   - **Risk:** MEDIUM
   - **Context:** Called during WiFi reconnect
   - **Impact:** Can block state machine during network recovery

2. **`firmware/main/websocket.c:442` - `esp_websocket_client_send_bin()`**
   - **Risk:** LOW (expected behavior)
   - **Context:** Normal CSI data transmission
   - **Impact:** Holds lock for duration of send (up to 30s timeout)

3. **`firmware/main/websocket.c:320-321` - `esp_websocket_client_stop()`/_destroy()`**
   - **Risk:** LOW (properly isolated)
   - **Context:** WebSocket disconnect
   - **Impact:** Executes OUTSIDE lock, only affects disconnect path

---

## Recommendations

### Immediate Actions:
1. **NO IMMEDIATE ROLLBACKS REQUIRED** - All changes are either:
   - Designed to prevent blocking (websocket.c)
   - Preventing hard crashes (ntp.c)
   - Properly isolated (ntpserver, OTA, diagnostics)

### Monitoring:
1. **Add logging around `esp_sntp_stop()`** to measure blocking duration:
   ```c
   ESP_LOGI(TAG, "ntp_start_sync: stopping SNTP client...");
   int64_t start = esp_timer_get_time();
   esp_sntp_stop();
   int64_t elapsed = (esp_timer_get_time() - start) / 1000;
   ESP_LOGI(TAG, "ntp_start_sync: SNTP stop took %lld ms", elapsed);
   ```

2. **Review WiFi reconnect frequency** in production logs to determine if `ntp_start_sync()` is being called excessively

### Future Improvements:
1. **Consider dedicated NTP task** if WiFi reconnects are frequent and `esp_sntp_stop()` blocking becomes problematic
2. **Add timeout protection** to `esp_sntp_stop()` if ESP-IDF version supports it
3. **Cache SNTP operational state** to avoid unnecessary stop/start cycles

### Diagnostic Usage:
The new diagnostic instrumentation (commit c5053f4) can be used to detect IO harness blocking in acceptance tests:
```bash
./test/acceptance/run_with_diagnostics.sh TestIO1_FreshInstallFirstBoot 5m
```

---

## Conclusion

The code changes reviewed in this analysis show a **generally positive trend toward preventing IO harness blocking**:

- The websocket.c rewrite was explicitly designed to eliminate blocking paths
- The ntp.c fix trades a potential blocking call for preventing a hard crash (acceptable trade-off)
- Mothership-side changes (ntpserver) are properly isolated in goroutines
- OTA validation is timer-based and asynchronous

**Overall Risk Level: LOW** - No immediate rollbacks required. The ntp.c change should be monitored for blocking duration, but it solves a critical stability issue (crash prevention).

---

**Generated:** 2026-08-27  
**Analyst:** NEEDLE Worker  
**Related Issues:** bf-3c282 (websocket), bf-1y316 (ntp), spaxel-4a5bfadb (analysis)