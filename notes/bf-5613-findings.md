# Sleep Monitoring Multi-Person Bedroom Analysis (bf-5613)

## Task Verification Results

### Specification (from plan.md)
> **Multi-person bedroom edge case:** If two blobs are tracked in a bedroom zone simultaneously, the system assigns the sleep record to the BLE-matched person if available, otherwise creates two separate `zone-based` records (one per occupant slot). Breathing analysis uses the blob with the strongest stationary signal (lowest smooth_deltaRMS).

### Findings

#### 1. Two-Blob Handling: NOT IMPLEMENTED
**Current behavior:**
- The `SleepAnalyzer` tracks sessions per `linkID` (one blob = one session)
- Each blob gets its own `SleepSession` through `sessions map[string]*SleepSession`
- No logic exists to detect when multiple blobs are in a bedroom zone simultaneously
- No logic to create separate "zone-based" records when BLE identity is unavailable

**Code location:** `mothership/internal/sleep/analyzer.go:228`
```go
sessions map[string]*SleepSession  // Per-link sleep sessions
```

**Missing:**
- Detection of multi-blob scenarios in bedroom zones
- Logic to create occupant-slot-based records (blob_1, blob_2) when no BLE match
- Coordination between multiple blobs in the same zone

#### 2. BLE-First Assignment: PARTIALLY IMPLEMENTED
**Current behavior:**
- `SetPersonID(linkID, personID string)` method exists in `SleepAnalyzer`
- The method is never called from the main application loop
- No integration between `identityMatcher.GetMatch()` and sleep session person assignment

**Code location:** `mothership/cmd/mothership/main.go`
- The main loop gets BLE matches via `identityMatcher.GetMatch(blob.ID)`
- However, these matches are used for automation/ground truth, NOT for sleep person assignment
- The sleep system samples all links regardless of BLE identity

**Missing:**
- Automatic call to `SetPersonID()` when BLE identity is resolved
- Logic to prioritize BLE-matched blobs over zone-based records
- Person-based session tracking vs link-based tracking

#### 3. Lowest-DeltaRMS Blob Selection: NOT IMPLEMENTED
**Current behavior:**
- Each blob/link processes its own breathing samples independently
- Breathing analysis happens per-blob, not across blobs
- No logic to compare `SmoothDeltaRMS` across blobs to select the "best" signal

**Code location:** `mothership/internal/sleep/integration.go:206-258`
- `collectSamples()` processes all links independently
- No comparison or selection logic between multiple links in the same zone

**Missing:**
- Logic to compare `SmoothDeltaRMS` values across blobs in the same zone
- Selection of the blob with the lowest (strongest stationary signal) for breathing analysis
- Fallback to zone-based records when all blobs have poor signals

#### 4. Integration Test: MISSING
**Current state:** `integration_test.go` has no two-person scenario test

**Test coverage gap:**
- No test for two blobs in a bedroom zone
- No test for BLE-based person assignment
- No test for zone-based fallback when no BLE match
- No test for deltaRMS blob selection

### Summary

The multi-person bedroom edge case is **NOT implemented** as specified. The current implementation:
- ✅ Handles single-person sleep tracking per blob
- ❌ Does NOT detect/handle multiple blobs in bedroom zones
- ❌ Does NOT assign sleep records to BLE-matched persons automatically
- ❌ Does NOT create zone-based records as fallback
- ❌ Does NOT select best blob for breathing analysis based on deltaRMS

### Implementation Requirements

To fulfill the specification, the following would need to be added:

1. **Multi-blob detection**: Monitor zone occupancy and detect when multiple blobs are in bedroom zones
2. **BLE integration**: Call `SetPersonID()` automatically when BLE identity is resolved
3. **Zone-based records**: Create occupant-slot records (e.g., "bedroom_slot_1", "bedroom_slot_2") when no BLE match
4. **Blob selection**: Compare `SmoothDeltaRMS` across blobs and select the one with the lowest value for breathing analysis
5. **Integration test**: Add test case covering the two-person bedroom scenario
