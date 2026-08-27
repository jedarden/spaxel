# Leak Profiling Analysis - onboard.test.js

**Generated**: 2026-08-27  
**Test Suite**: dashboard/js/onboard.test.js  
**Total Tests**: 71 (70 passed, 1 unrelated failure)  
**Profiling Tool**: testProfiler.js

## Executive Summary

Heap profiling reveals **2 significant leaks** in the Spaxel onboarding wizard test suite:

1. **HIGH Severity**: Heap grew by **13.18 MB** during test run
2. **MEDIUM Severity**: **27 timers** created but not cleared

## Profiling Data

### Before Test Suite
```json
{
  "heap": {
    "heapUsed": "63 MB",
    "heapUsedBytes": 65792264
  },
  "timers": {
    "intervals": 0,
    "timeouts": 2,
    "total": 2
  },
  "websockets": {
    "count": 0
  }
}
```

### After Test Suite
```json
{
  "heap": {
    "heapUsed": "76 MB",
    "heapUsedBytes": 79613584
  },
  "timers": {
    "intervals": 0,
    "timeouts": 29,
    "total": 29
  },
  "websockets": {
    "count": 0
  }
}
```

### Deltas
```json
{
  "heap": {
    "usedDelta": "+13.82 MB",
    "usedDeltaBytes": "+13821320"
  },
  "timers": {
    "timeoutDelta": "+27",
    "intervalDelta": 0,
    "totalDelta": "+27"
  },
  "websockets": {
    "countDelta": 0
  }
}
```

## Issues Detected

### 1. HIGH: Heap Growth (13.18 MB)

**Severity**: `high`  
**Type**: `heap-growth`  
**Message**: `Heap grew by 13.18 MB during test run`

#### Impact
- **13.82 MB heap increase** across 71 tests
- Average leak per test: **~195 KB**
- Cumulative effect in production wizard usage
- Potential browser tab crashes after multiple wizard sessions

#### Likely Sources
1. **Event listeners not removed**: DOM event listeners attached in wizard steps but never cleaned up
2. **Closure captures**: Timer callbacks capturing large wizard state objects
3. **DOM node retention**: Wizard DOM elements removed from tree but still referenced
4. **CSI history accumulation**: `_state.csiHistory` array growing unbounded during calibration tests

#### Investigation Priority
1. Check `SpaxelOnboard.close()` for incomplete cleanup
2. Audit all `addEventListener` calls for missing `removeEventListener`
3. Verify `clearInterval` / `clearTimeout` calls in afterEach hooks
4. Inspect `_state.csiHistory` growth pattern

### 2. MEDIUM: Timer Leak (27 timers)

**Severity**: `medium`  
**Type**: `timer-leak`  
**Message**: `27 timers created but not cleared`

#### Impact
- **27 active setTimeout handles** after test suite completion
- Timers may fire after tests finish, causing side effects
- Memory held by timer callback closures
- Potential race conditions in subsequent test runs

#### Breakdown
- **Before tests**: 2 timeouts (likely Jest internals)
- **After tests**: 29 timeouts
- **Leaked**: 27 timeouts

#### Likely Sources
1. **`_state.pollTimer`**: Node detection polling interval not always cleared
2. **`_state.calibrateTimer`**: Calibration phase timer not cleared on wizard close
3. **Auto-advance timeouts**: 400ms browser_check timeout may not be cleared
4. **Jest fake timers**: `jest.useFakeTimers()` / `jest.useRealTimers()` mismatch

#### Investigation Priority
1. Add logging to all `setTimeout` / `setInterval` calls
2. Verify `beforeEach` / `afterEach` cleanup in test suites
3. Check for tests missing timer cleanup in afterEach
4. Audit wizard lifecycle for uncancelled timers on `close()`

## WebSocket State

**Status**: ✅ **No leaks detected**

- Before: 0 instances
- After: 0 instances
- Delta: 0

The WebSocket mocking and cleanup is working correctly.

## Reproducibility

### Run Profiling
```bash
cd dashboard
node run-leak-profiling.js
```

### With Garbage Collection
```bash
node --expose-gc run-leak-profiling.js --gc
```

### View Results
```bash
cat test-profiling-results.json | jq .
```

## Next Steps

### Immediate (Fix Timer Leak)
1. ✅ Add timer tracking to testProfiler.js (already done)
2. ✅ Run profiling to capture evidence (already done)
3. ⏳ **TODO**: Audit wizard lifecycle for uncleared timers
4. ⏳ **TODO**: Fix timer cleanup in `SpaxelOnboard.close()`
5. ⏳ **TODO**: Re-run profiling to verify fix

### Short-term (Fix Heap Leak)
1. ✅ Add heap snapshots to testProfiler.js (already done)
2. ✅ Capture baseline heap data (already done)
3. ⏳ **TODO**: Use Chrome DevTools heap snapshot comparison
4. ⏳ **TODO**: Identify retained objects in wizard state
5. ⏳ **TODO**: Fix closure captures and event listener leaks

### Long-term (Prevention)
1. ⏳ **TODO**: Add leak detection to CI/CD pipeline
2. ⏳ **TODO**: Set up automated heap profiling on every test run
3. ⏳ **TODO**: Establish leak threshold thresholds (fail if > 5MB growth)
4. ⏳ **TODO**: Add memory leak regression tests

## Technical Details

### Profiling Infrastructure
- **Location**: `dashboard/js/testProfiler.js`
- **Hooks**: `dashboard/js/onboard.test.js` (lines 70-120)
- **Runner**: `dashboard/run-leak-profiling.js`

### What's Tracked
1. **Heap Usage**: `process.memoryUsage().heapUsed`
2. **Timers**: Monkey-patched `setTimeout`/`clearTimeout` and `setInterval`/`clearInterval`
3. **WebSockets**: Mock WebSocket instance tracking
4. **Process Info**: PID, uptime, full memory breakdown

### Snapshot Points
- **beforeTest**: Before any test runs (after Jest setup)
- **afterTest**: After all tests complete (before teardown)

### Analysis Algorithm
```javascript
// High severity: Heap growth > 5MB
if (heapDeltaMB > 5) {
    issues.push({ severity: 'high', type: 'heap-growth' });
}

// Medium severity: Timer leak
if (after.timers.total > before.timers.total) {
    issues.push({ severity: 'medium', type: 'timer-leak' });
}

// Medium severity: WebSocket leak
if (after.websockets.count > before.websockets.count) {
    issues.push({ severity: 'medium', type: 'websocket-leak' });
}
```

## Related Files

- `dashboard/js/onboard.test.js` - Test suite with profiling hooks
- `dashboard/js/testProfiler.js` - Profiling instrumentation
- `dashboard/run-leak-profiling.js` - Standalone runner
- `dashboard/test-profiling-results.json` - Latest profiling data
- `dashboard/LEAK_PROFILING_ANALYSIS.md` - This document

## References

- **Task Bead**: `spaxel-b34d9e43`
- **Node.js Memory Docs**: https://nodejs.org/api/process.html#process_process_memoryusage
- **Jest Timer Mocks**: https://jestjs.io/docs/timer-mocks
- **Chrome DevTools Heap Snapshots**: https://developer.chrome.com/docs/devtools/memory-problems/
