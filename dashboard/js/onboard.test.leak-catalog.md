# Timer/Interval/WebSocket Leak Catalog for onboard.test.js

## Generated
2026-08-27

## Summary
This file catalogs all timer, interval, and WebSocket mock instances in the test file that could potentially leak, along with their cleanup status and locations.

---

## 1. setInterval/clearInterval Pairs

| Line | Type | Location | Cleanup Status | Notes |
|------|------|----------|----------------|-------|
| 163 | `clearInterval(_state.pollTimer)` | afterEach in 'Browser check step' describe block | ✅ Cleaned up | Paired with line 29 reset |
| 198 | `clearInterval(_state.pollTimer)` | afterEach in 'State persistence' describe block | ✅ Cleaned up | Paired with line 29 reset |
| 243 | `clearInterval(_state.pollTimer)` | afterEach in 'Serial port handling' describe block | ✅ Cleaned up | Paired with line 29 reset |
| 276 | `clearInterval(_state.pollTimer)` | afterEach in 'Provisioning payload' describe block | ✅ Cleaned up | Paired with line 29 reset |
| 321 | `clearInterval(_state.pollTimer)` | afterEach in 'Node detection' describe block | ✅ Cleaned up | Paired with line 29 reset |
| 424 | `clearInterval(_state.pollTimer)` | afterEach in 'CSI frame parser' describe block | ✅ Cleaned up | Paired with line 29 reset |

**Note:** No `setInterval` creation calls are found in the test file. The `clearInterval` calls clean up `_state.pollTimer` which is set by the wizard code under test, not by the test itself.

---

## 2. setTimeout/clearTimeout Pairs

| Line | Type | Location | Outside 'Wizard state transitions'? | Cleanup Status | Notes |
|------|------|----------|-------------------------------------|-----------------|-------|
| 80 | `setTimeout(resolve, 0)` | beforeAll hook | ✅ YES | ⚠️ No cleanup needed | Short-lived, resolves immediately |
| 90 | `setTimeout(resolve, 0)` | afterAll hook | ✅ YES | ⚠️ No cleanup needed | Short-lived, resolves immediately |
| 164 | `clearTimeout(_state.calibrateTimer)` | afterEach in 'Browser check step' | ❌ No | ✅ Cleaned up | Paired with line 30 reset |
| 199 | `clearTimeout(_state.calibrateTimer)` | afterEach in 'State persistence' | ❌ No | ✅ Cleaned up | Paired with line 30 reset |
| 244 | `clearTimeout(_state.calibrateTimer)` | afterEach in 'Serial port handling' | ❌ No | ✅ Cleaned up | Paired with line 30 reset |
| 277 | `clearTimeout(_state.calibrateTimer)` | afterEach in 'Provisioning payload' | ❌ No | ✅ Cleaned up | Paired with line 30 reset |
| 322 | `clearTimeout(_state.calibrateTimer)` | afterEach in 'Node detection' | ❌ No | ✅ Cleaned up | Paired with line 30 reset |
| 425 | `clearTimeout(_state.calibrateTimer)` | afterEach in 'CSI frame parser' | ❌ No | ✅ Cleaned up | Paired with line 30 reset |

**Note:** No `setTimeout` creation calls are found in the test file (except the short-lived ones in beforeAll/afterAll). The `clearTimeout` calls clean up `_state.calibrateTimer` which is set by the wizard code under test, not by the test itself.

---

## 3. WebSocket Mock Instances

| Line | Type | Location | Outside 'Wizard state transitions'? | Cleanup Status | Notes |
|------|------|----------|-------------------------------------|-----------------|-------|
| 53-64 | `WebSocket.mockImplementation(...)` | resetWizardState function (lines 20-65) | ✅ YES | ⚠️ Indirect cleanup | Creates mock object, cleanup via afterEach hooks |
| 165 | `_state.ws.close()` | afterEach in 'Browser check step' | ❌ No | ✅ Cleaned up | Sets to null after close |
| 200 | `_state.ws.close()` | afterEach in 'State persistence' | ❌ No | ✅ Cleaned up | Sets to null after close |
| 245 | `_state.ws.close()` | afterEach in 'Serial port handling' | ❌ No | ✅ Cleaned up | Sets to null after close |
| 278 | `_state.ws.close()` | afterEach in 'Provisioning payload' | ❌ No | ✅ Cleaned up | Sets to null after close |
| 323 | `_state.ws.close()` | afterEach in 'Node detection' | ❌ No | ✅ Cleaned up | Sets to null after close |
| 426 | `_state.ws.close()` | afterEach in 'CSI frame parser' | ❌ No | ✅ Cleaned up | Sets to null after close |

---

## 4. Describe Blocks with MISSING afterEach Cleanup

The following describe blocks have **NO afterEach hook** for timer/WebSocket cleanup:

| Describe Block | Lines | Issue | Risk Level |
|----------------|-------|-------|------------|
| 'Onboard configuration' | 125-138 | No afterEach cleanup | 🔴 HIGH |
| 'Step definitions' | 140-154 | No afterEach cleanup | 🔴 HIGH |
| 'Step indicator rendering' | 605-627 | Has afterEach but only calls `SpaxelOnboard.close()` | 🟡 MEDIUM |
| 'Error message mapping' | 632-648 | No afterEach cleanup | 🔴 HIGH |
| 'Browser check without serial API' | 653-701 | Has afterEach but doesn't clean timers/WebSocket | 🟡 MEDIUM |
| 'Wizard lifecycle' | 529-600 | No afterEach cleanup | 🔴 HIGH |
| 'Mothership-Level WiFi Configuration (ADR-005)' | 943-1041 | Has afterEach but only calls `SpaxelOnboard.close()` | 🟡 MEDIUM |
| 'Provisioning Payload Assembly and Serial Send' | 1046-1138 | Has afterEach but only calls `__clearLastEncodedData()` | 🟡 MEDIUM |
| 'Node detection wizard transition' | 1143-1231 | Has afterEach but only calls `SpaxelOnboard.close()` and `jest.useRealTimers()` | 🟡 MEDIUM |
| 'Session storage restore at each step' | 1236-1320 | Has afterEach but only calls `SpaxelOnboard.close()` and `jest.useRealTimers()` | 🟡 MEDIUM |
| 'Re-provision mode' | 1325-1457 | Has afterEach but only calls `SpaxelOnboard.close()` and `jest.useRealTimers()` | 🟡 MEDIUM |

---

## 5. Potential Leak Sources OUTSIDE 'Wizard state transitions' Describe Block

### 'Wizard state transitions' describe block spans lines 706-938.

### Outside sources (potential leaks):

| Line | Item | Describe Block | Leak Risk | Notes |
|------|------|----------------|-----------|-------|
| 29 | `_state.pollTimer = null` | resetWizardState (function) | 🟢 LOW | Reset in beforeEach, safe |
| 30 | `_state.calibrateTimer = null` | resetWizardState (function) | 🟢 LOW | Reset in beforeEach, safe |
| 32 | `_state.ws = null` | resetWizardState (function) | 🟢 LOW | Reset in beforeEach, safe |
| 53-64 | `WebSocket.mockImplementation` | resetWizardState (function) | 🟢 LOW | Re-applied on each reset, safe |
| 80 | `setTimeout(resolve, 0)` | beforeAll hook | 🟢 LOW | Short-lived, safe |
| 90 | `setTimeout(resolve, 0)` | afterAll hook | 🟢 LOW | Short-lived, safe |

---

## 6. Critical Issues Found

### 🔴 HIGH PRIORITY

1. **'Onboard configuration' (lines 125-138)** - No afterEach cleanup
   - Tests verify configuration values only
   - Risk: LOW (tests don't create timers/WebSockets)
   - Recommendation: Add afterEach for consistency

2. **'Step definitions' (lines 140-154)** - No afterEach cleanup
   - Tests verify step arrays only
   - Risk: LOW (tests don't create timers/WebSockets)
   - Recommendation: Add afterEach for consistency

3. **'Error message mapping' (lines 632-648)** - No afterEach cleanup
   - Tests create UserError instances
   - Risk: LOW (tests don't create timers/WebSockets)
   - Recommendation: Add afterEach for consistency

4. **'Wizard lifecycle' (lines 529-600)** - No afterEach cleanup
   - Tests call `SpaxelOnboard.start()` and `SpaxelOnboard.close()`
   - Risk: HIGH - Tests use `jest.useFakeTimers()` but may not clean up properly
   - Recommendation: Add afterEach with timer/WebSocket cleanup

### 🟡 MEDIUM PRIORITY

1. **'Step indicator rendering' (lines 605-627)** - Incomplete afterEach cleanup
   - Current: Only calls `SpaxelOnboard.close()`
   - Missing: Timer/WebSocket cleanup
   - Recommendation: Add standard cleanup pattern

2. **'Browser check without serial API' (lines 653-701)** - Incomplete afterEach cleanup
   - Current: Restores serial API mock
   - Missing: Timer/WebSocket cleanup
   - Recommendation: Add standard cleanup pattern

3. **'Mothership-Level WiFi Configuration (ADR-005)' (lines 943-1041)** - Incomplete afterEach cleanup
   - Current: Only calls `SpaxelOnboard.close()`
   - Missing: Timer/WebSocket cleanup
   - Recommendation: Add standard cleanup pattern

4. **'Provisioning Payload Assembly and Serial Send' (lines 1046-1138)** - Incomplete afterEach cleanup
   - Current: Only calls `__clearLastEncodedData()`
   - Missing: Timer/WebSocket cleanup
   - Recommendation: Add standard cleanup pattern

5. **'Node detection wizard transition' (lines 1143-1231)** - Incomplete afterEach cleanup
   - Current: Only calls `SpaxelOnboard.close()` and `jest.useRealTimers()`
   - Missing: Explicit timer/WebSocket cleanup
   - Recommendation: Add standard cleanup pattern

6. **'Session storage restore at each step' (lines 1236-1320)** - Incomplete afterEach cleanup
   - Current: Only calls `SpaxelOnboard.close()` and `jest.useRealTimers()`
   - Missing: Explicit timer/WebSocket cleanup
   - Recommendation: Add standard cleanup pattern

7. **'Re-provision mode' (lines 1325-1457)** - Incomplete afterEach cleanup
   - Current: Only calls `SpaxelOnboard.close()` and `jest.useRealTimers()`
   - Missing: Explicit timer/WebSocket cleanup
   - Recommendation: Add standard cleanup pattern

---

## 7. Recommended Standard afterEach Cleanup Pattern

Based on the patterns used in lines 161-166, 197-201, 242-246, 275-279, 320-324, 423-427:

```javascript
afterEach(() => {
    // Clean up any timers or WebSocket connections
    if (_state.pollTimer) { clearInterval(_state.pollTimer); _state.pollTimer = null; }
    if (_state.calibrateTimer) { clearTimeout(_state.calibrateTimer); _state.calibrateTimer = null; }
    if (_state.ws) { _state.ws.close(); _state.ws = null; }
    // Additional cleanup as needed
    SpaxelOnboard.close();
    jest.useRealTimers();
});
```

---

## 8. Conclusion

The test file has a **mixed cleanup strategy**:

✅ **Good:**
- Most describe blocks that interact with the wizard state have proper cleanup
- The `resetWizardState` function properly resets state variables
- WebSocket mock is properly re-applied on each reset

⚠️ **Needs Improvement:**
- Several describe blocks are missing afterEach cleanup hooks entirely
- Some describe blocks have incomplete cleanup (missing timer/WebSocket cleanup)
- 'Wizard lifecycle' block uses fake timers but lacks proper cleanup

🔴 **Potential Leaks:**
- Tests in 'Wizard lifecycle', 'Step indicator rendering', 'Browser check without serial API', and other blocks with incomplete cleanup could leak timers or WebSocket connections if tests fail before reaching their manual cleanup calls (e.g., `SpaxelOnboard.close()`)
