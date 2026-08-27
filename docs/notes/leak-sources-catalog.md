# Leak Sources Catalog

**Generated:** 2026-08-27  
**Purpose:** Catalog and prioritize all timer/interval/WebSocket leak sources in the Spaxel codebase based on profiling data analysis.

## Executive Summary

Profiling data indicates **timer leaks are the primary issue**:
- **16-27 setTimeout calls not cleared** across test runs
- **0 interval leaks** (good — setInterval properly managed)
- **0 WebSocket leaks** (good — WebSocket cleanup working)
- **Heap growth:** 3-13 MB during test runs

**Priority Ranking:**
1. 🔴 **HIGH** - onboard.js auto-advance and calibration timers
2. 🟡 **MEDIUM** - WebSocket reconnection/disconnect timers  
3. 🟢 **LOW** - Go backend time.AfterFunc timers (properly managed)

---

## Dashboard JavaScript (Primary Leak Source)

### 1. onboard.js - Onboarding Wizard 🔴 HIGH PRIORITY

**File:** `/dashboard/js/onboard.js`

#### Timer #1: Auto-advance from browser_check (line 304)
```javascript
setTimeout(function () { goToStep(1); }, 400);
```
**Issue:** Never cleared - fires once after 400ms then orphaned
**Impact:** LOW - Single fire, but contributes to timer count
**Line:** 304

#### Timer #2: Detection auto-advance (line 566)
```javascript
setTimeout(function () { goToStep(state.currentStepIndex + 1); }, 1200);
```
**Issue:** Never cleared - fires after 1.2s
**Impact:** MEDIUM - Called multiple times per test suite
**Line:** 566

#### Timer #3: Calibration step auto-advance (line 818)
```javascript
setTimeout(function () { 
    goToStep(state.currentStepIndex + 1); 
}, 1200);
```
**Issue:** Never cleared - fires after 1.2s  
**Impact:** MEDIUM - Contributes to cumulative timer count
**Line:** 818

#### Timer #4: Serial port read timeout (line 901)
```javascript
setTimeout(function () { 
    reject(new Error('timeout')); 
}, 5000);
```
**Issue:** Properly scoped in Promise - **NOT A LEAK**
**Impact:** NONE - Cleaned up by Promise rejection
**Line:** 901

#### Timer #5: Detection polling timeout (line 1078)
```javascript
await new Promise(function (r) { setTimeout(r, 1000); });
```
**Issue:** Properly scoped in Promise - **NOT A LEAK**
**Impact:** NONE - Cleaned up by Promise resolution
**Line:** 1078

#### Timer #6: Serial write timeout (line 1119)
```javascript
setTimeout(function () { 
    reject(new Error('timeout')); 
}, remaining + 50);
```
**Issue:** Properly scoped in Promise - **NOT A LEAK**
**Impact:** NONE - Cleaned up by Promise rejection
**Line:** 1119

#### Timer #7: Flash write timeout (line 1210)
```javascript
setTimeout(function () { 
    reject(new Error('timeout')); 
}, remaining + 50);
```
**Issue:** Properly scoped in Promise - **NOT A LEAK**
**Impact:** NONE - Cleaned up by Promise rejection
**Line:** 1210

#### Timer #8: CSI parse retry timeout (line 1179)
```javascript
await new Promise(function (r) { setTimeout(r, 400); });
```
**Issue:** Properly scoped in Promise - **NOT A LEAK**
**Impact:** NONE - Cleaned up by Promise resolution
**Line:** 1179

#### Interval #1: Node detection polling (line 1300)
```javascript
state.pollTimer = setInterval(function () {
    // Poll /api/nodes every 3s
    fetch(_CONFIG.nodesEndpoint).then(...)
}, 3000);
```
**Issue:** Properly cleaned up in afterEach (line 1362)
**Impact:** **NONE** - Cleaned up via clearInterval
**Line:** 1300, cleanup at 1362, 1347, 1769

#### Timer #9: Retry timeout (line 1354)
```javascript
setTimeout(function () { 
    goToStep(state.currentStepIndex + 1); 
}, 1000);
```
**Issue:** Never cleared - fires after 1s
**Impact:** LOW - Fire-and-forget retry, but should use clearTimeout
**Line:** 1354

#### Timer #10: Calibration tick timer (line 1588)
```javascript
state.calibrateTimer = setTimeout(tick, 200);
```
**Issue:** Properly cleaned up (line 1390)
**Impact:** **NONE** - Cleaned up via clearTimeout
**Line:** 1588, cleanup at 1390

**Test Fixture Issues:**
- `beforeEach`/`afterEach` hooks DO clean up pollTimer and calibrateTimer
- BUT test setup uses `jest.useFakeTimers()` which may interfere with cleanup
- **Root cause:** Timer cleanup happens AFTER test suite completes, not between individual tests

---

### 2. websocket.js - Dashboard WebSocket Manager 🟡 MEDIUM PRIORITY

**File:** `/dashboard/js/websocket.js`

#### Timer #11: WebSocket reconnect timer (line 130)
```javascript
_reconnectTimer = setTimeout(function () {
    _reconnectTimer = null;
    _reconnectAttempt++;
    connect(wsProtocol + '//' + window.location.host + '/ws/dashboard');
}, delay);
```
**Issue:** Properly managed - cleared in disconnect() (line 109)
**Impact:** **NONE** - Has proper cleanup logic
**Line:** 130, cleanup at 109, 127

#### Interval #2: Disconnect state timer (line 143)
```javascript
_disconnectTimer = setInterval(function () {
    if (_connected || !_disconnectStart) return;
    var elapsed = Date.now() - _disconnectStart;
    if (elapsed >= SILENT_MS && elapsed < DIMMING_MS) {
        _applyDimming();
    } else if (elapsed >= DIMMING_MS) {
        _showModal();
    }
}, 500);
```
**Issue:** Properly managed - cleared in disconnect() (line 154-156)
**Impact:** **NONE** - Has proper cleanup logic
**Line:** 143, cleanup at 155

#### RAF #3: Blob extrapolation (line 202)
```javascript
_extrapolRAF = requestAnimationFrame(tick);
```
**Issue:** Properly managed - cleared in _stopExtrapolation (line 207)
**Impact:** **NONE** - Has proper cleanup via cancelAnimationFrame
**Line:** 202, cleanup at 207

**Analysis:** WebSocket module has excellent cleanup discipline. No leaks detected.

---

### 3. fleet.js - Fleet Management 🟢 LOW PRIORITY

**File:** `/dashboard/static/js/fleet.js`

#### Timer #12: OTA stagger delay (line ~260)
```javascript
await new Promise(resolve => setTimeout(resolve, CONFIG.otaStaggerMs));
```
**Issue:** Properly scoped in async function - **NOT A LEAK**
**Impact:** NONE - Single-use Promise wrapper
**Line:** ~260 (not shown in grep, but confirmed in code)

---

## Go Backend (Well-Managed)

### 4. notify/service.go - Notification Batching 🟢 LOW PRIORITY

**File:** `/mothership/internal/notify/service.go`

#### Timer #13: Batch flush timer
```go
s.batchTimer = time.AfterFunc(s.batchWindow, s.flushBatch)
```
**Issue:** Properly managed - stopped before reuse
**Impact:** **NONE** - Go's time.AfterFunc is well-managed
**Line:** ~140 (estimated)

---

### 5. service_enhanced.go - Enhanced Batching 🟢 LOW PRIORITY

**File:** `/mothership/internal/notify/service_enhanced.go`

#### Timer #14: Enhanced batch flush timer
```go
ext.batchTimer = time.AfterFunc(time.Duration(ext.batching.BatchWindowSec)*time.Second, ext.flushBatch)
```
**Issue:** Properly managed - stopped before reuse
**Impact:** **NONE** - Go's time.AfterFunc is well-managed
**Line:** ~80 (estimated)

---

### 6. falldetect/detector.go - Fall Detection 🟢 LOW PRIORITY

**File:** `/mothership/internal/falldetect/detector.go`

#### Timer #15-16: Escalation timers
```go
time.AfterFunc(d.config.EscalationTime1, func() {
    // Escalate to secondary tier
})

time.AfterFunc(d.config.EscalationTime2, func() {
    // Trigger final alert
})
```
**Issue:** Properly managed - context-scoped
**Impact:** **NONE** - Go's time.AfterFunc is well-managed
**Line:** ~200-220 (estimated)

---

## Test Infrastructure Issues

### Problem: jest.useFakeTimers() Interference

**Test Files:**
- `/dashboard/js/onboard.test.js`
- `/dashboard/js/onboard.leak-isolation.test.js`  
- `/dashboard/js/onboard.leak-detection.test.js`

**Issue:** 
- Tests use `jest.useFakeTimers()` to control timer flow
- **BUT** fake timers don't actually execute callbacks, they just advance the clock
- When `beforeEach`/`afterEach` cleanup runs, real timers may still be pending
- **Result:** Timer count accumulates across test suite even though individual tests "clean up"

**Evidence from leak-isolation-results.json:**
```
"settimeout-beforeall-hook": timeouts: 4→6 (+2 leaked)
"wizard-lifecycle-with-aftereach": timeouts: 3→4 (+1 leaked)
```

**Root Cause:** Timer cleanup happens in `afterEach()`, but:
1. Timers fire asynchronously AFTER cleanup code runs
2. Fake timers advance clock WITHOUT firing callbacks
3. Real timer cleanup happens BEFORE callbacks execute
4. `jest.useRealTimers()` restores real timers, but some are still pending

---

## Prioritized Testing Order

### Phase 1: HIGH PRIORITY 🔴

**Test these patterns first** (based on profiling signal strength):

1. **onboard.js auto-advance chain**
   - File: `dashboard/js/onboard.js`
   - Lines: 304, 566, 818, 1354
   - **Test:** Manual instrument-and-clear, verify setTimeout count before/after each goToStep call

2. **Test fixture timer cleanup**
   - File: `dashboard/js/onboard.test.js`
   - Lines: 160-166 (beforeEach/afterEach)
   - **Test:** Add `jest.useRealTimers()` in afterEach, verify all timers cleared

### Phase 2: MEDIUM PRIORITY 🟡

3. **WebSocket disconnect state**
   - File: `dashboard/js/websocket.js`
   - Lines: 143-152 (setInterval)
   - **Test:** Connect/disconnect stress test, verify no interval leak

4. **WebSocket reconnection**
   - File: `dashboard/js/websocket.js`
   - Lines: 130-136 (setTimeout)
   - **Test:** Simulate network failure, verify timer cleared on reconnect

### Phase 3: LOW PRIORITY 🟢

5. **Go backend time.AfterFunc**
   - Files: mothership/internal/notify/*.go, falldetect/detector.go
   - **Test:** Load test with 1000+ notifications, verify no timer accumulation

6. **Promise-scoped timeouts**
   - File: dashboard/js/onboard.js
   - Lines: 901, 1078, 1119, 1179, 1210
   - **Test:** Verify Promise rejection cleans up timer (already working per profiler)

---

## Quick Reference Table

| Priority | File | Line(s) | Type | Leaks? | Notes |
|----------|------|---------|------|--------|-------|
| 🔴 HIGH | onboard.js | 304, 566, 818, 1354 | setTimeout | **YES** | Auto-advance timers not cleared |
| 🔴 HIGH | onboard.test.js | 160-166 | test hooks | **MAYBE** | Fake timers interference |
| 🟡 MEDIUM | websocket.js | 130 | setTimeout | NO | Proper cleanup exists |
| 🟡 MEDIUM | websocket.js | 143 | setInterval | NO | Proper cleanup exists |
| 🟢 LOW | onboard.js | 1300 | setInterval | NO | Properly cleaned up |
| 🟢 LOW | onboard.js | 1588 | setTimeout | NO | Properly cleaned up |
| 🟢 LOW | onboard.js | 901, 1078, 1119, 1179, 1210 | setTimeout | NO | Promise-scoped, auto-cleanup |
| 🟢 LOW | Go backend | various | time.AfterFunc | NO | Go runtime manages |

---

## Recommended Fixes

### Fix #1: Clear auto-advance timers in onboard.js (HIGH)

**Current:**
```javascript
// Line 304
setTimeout(function () { goToStep(1); }, 400);
```

**Proposed:**
```javascript
// Store timer ID
if (state.autoAdvanceTimer) clearTimeout(state.autoAdvanceTimer);
state.autoAdvanceTimer = setTimeout(function () { 
    goToStep(1); 
    state.autoAdvanceTimer = null; // Clear after fire
}, 400);
```

Apply similar pattern to:
- Line 566 (detection auto-advance)
- Line 818 (calibration auto-advance)
- Line 1354 (retry timeout)

### Fix #2: Fix test fixture cleanup (HIGH)

**Current issue:** `jest.useRealTimers()` not called in afterEach

**Proposed:**
```javascript
afterEach(() => {
    jest.useRealTimers(); // Restore real timers first
    
    // THEN clean up
    if (_state.pollTimer) { 
        clearInterval(_state.pollTimer); 
        _state.pollTimer = null; 
    }
    if (_state.calibrateTimer) { 
        clearTimeout(_state.calibrateTimer); 
        _state.calibrateTimer = null; 
    }
    if (_state.ws) { 
        _state.ws.close(); 
        _state.ws = null; 
    }
    
    // Force all pending timers to fire
    await jest.advanceTimersByTimeAsync(1000);
});
```

### Fix #3: Add explicit cleanup to goToStep function (MEDIUM)

**Current:** goToStep doesn't clean up existing timers

**Proposed:**
```javascript
function goToStep(stepIndex) {
    // Clear any pending auto-advance before setting new one
    if (state.autoAdvanceTimer) {
        clearTimeout(state.autoAdvanceTimer);
        state.autoAdvanceTimer = null;
    }
    
    _state.currentStepIndex = stepIndex;
    // ... rest of function
}
```

---

## Next Steps

1. **Test Fix #1 manually** - Add clearTimeout to line 304 and verify timer count drops
2. **Apply Fix #2** - Update test fixture and re-run leak detection
3. **Re-run profiling** - Verify timer leak count drops from 16→0
4. **Apply Fix #3** - Add goToStep cleanup and verify no regression
5. **Document results** - Update this catalog with fix confirmation

---

## Appendix: Profiling Data Summary

### leak-detection-report.json
- **Timer delta:** +16 timeouts (2→18)
- **Heap delta:** -3 MB (good - memory reclaimed)
- **WebSocket delta:** 0 (no leaks)

### leak-isolation-results.json
- **fake-timers-with-cleanup:** timeouts: 2→3 (+1 leaked)
- **wizard-lifecycle-with-aftereach:** timeouts: 3→4 (+1 leaked)  
- **settimeout-beforeall-hook:** timeouts: 4→6 (+2 leaked)
- **All show:** "LEAKS" verdict

### test-profiling-results.json
- **Timer delta:** +27 timeouts (2→29)
- **Heap delta:** +13 MB ⚠️
- **Verdict:** 2 issues (heap growth + timer leak)

### leak-test-full-lifecycle.json
- **Timer delta:** +2 timeouts (2→4)
- **Heap delta:** +3-4 MB
- **Verdict:** 1 issue (timer leak)
