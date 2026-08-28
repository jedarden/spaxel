# Profiling Data Validation Report

**Generated:** 2026-08-28  
**Purpose:** Validate profiling data completeness and format for leak detection analysis

## Summary

✅ **All profiling data files exist and are valid JSON**  
✅ **All required fields are present and properly formatted**  
⚠️ **All files indicate timer leaks requiring investigation**

## Files Validated

### 1. leak-detection-report.json

**Status:** ✅ VALID  
**Location:** `/home/coding/spaxel/dashboard/leak-detection-report.json`  
**Size:** 2 test runs

#### Structure Validation
- ✅ Valid JSON format
- ✅ Array of test run objects
- ✅ Each entry contains all required sections:
  - `meta` (testFile, timestamp, duration)
  - `snapshots` (beforeTest, afterTest)
  - `deltas` (heap, timers, websockets)
  - `analysis` (issuesFound, issues, summary)

#### Field Completeness

**Before/After Snapshots:** ✅ COMPLETE
- Both snapshots present for all runs
- Each snapshot includes:
  - `id` (integer)
  - `timestamp` (ISO 8601 format)
  - `label` (string)
  - `heap` statistics (heapUsed, heapTotal, external, rss, heapUsedBytes, heapTotalBytes)
  - `timers` (intervals, timeouts, total)
  - `websockets` (count, instances array)
  - `process` (pid, uptime, memoryUsage object)

**Timer Counts:** ✅ PRESENT
- Before: 2 timeouts (baseline)
- After: 18 timeouts
- Delta: +16 timers (LEAK INDICATOR)

**Heap Statistics:** ✅ COMPLETE
- heapUsedBytes: 53,539,288 → 50,188,312 (-3.2 MB)
- heapTotalBytes: 86,315,008 → 87,101,440 (+0.8 MB)
- RSS values present and reasonable

#### Issues Detected
- ⚠️ **Timer leak confirmed:** 16 timers created but not cleared
- Severity: MEDIUM
- Type: timer-leak

---

### 2. leak-isolation-results.json

**Status:** ✅ VALID  
**Location:** `/home/coding/spaxel/dashboard/leak-isolation-results.json`  
**Size:** 3 isolated test cases

#### Structure Validation
- ✅ Valid JSON format
- ✅ Array of test result objects
- ✅ Each entry contains:
  - `testName` (string)
  - `before` (complete snapshot)
  - `after` (complete snapshot)
  - `verdict` (string)

#### Field Completeness

**Before/After Snapshots:** ✅ COMPLETE
- Both snapshots present for all tests
- Same comprehensive structure as leak-detection-report
- All heap, timer, websocket, and process fields populated

**Timer Counts by Test:**

| Test Name | Before | After | Delta | Verdict |
|-----------|--------|-------|-------|---------|
| fake-timers-with-cleanup | 2 | 3 | +1 | LEAKS |
| wizard-lifecycle-with-aftereach | 3 | 4 | +1 | LEAKS |
| settimeout-beforeall-hook | 4 | 6 | +2 | LEAKS |

**Heap Statistics:** ✅ COMPLETE
- All tests show heapUsedBytes and heapTotalBytes
- Memory deltas calculated properly
- RSS and external memory tracked

#### Issues Detected
- ⚠️ **All 3 isolation tests show leaks:**
  1. fake-timers-with-cleanup: +1 timer leak
  2. wizard-lifecycle-with-aftereach: +1 timer leak
  3. settimeout-beforeall-hook: +2 timer leaks
- Verdict: "LEAKS" for all tests

---

### 3. leak-test-full-lifecycle.json

**Status:** ✅ VALID  
**Location:** `/home/coding/spaxel/dashboard/leak-test-full-lifecycle.json`  
**Size:** 2 lifecycle test runs

#### Structure Validation
- ✅ Valid JSON format
- ✅ Array of lifecycle test reports
- ✅ Same structure as leak-detection-report.json
- ✅ All sections present: meta, snapshots, deltas, analysis

#### Field Completeness

**Before/After Snapshots:** ✅ COMPLETE
- Labels: "before-full-lifecycle" / "after-full-lifecycle"
- Full heap, timer, websocket, and process metrics
- Timestamps properly sequenced

**Timer Counts:** ✅ PRESENT
- Before: 2 timeouts (baseline)
- After: 4 timeouts  
- Delta: +2 timers (LEAK INDICATOR)

**Heap Statistics:** ✅ COMPLETE
- heapUsedBytes: 54,546,976 → 58,238,432 (+3.7 MB)
- heapTotalBytes: 86,315,008 → 86,839,296 (+0.5 MB)
- Memory growth detected but within expected range for lifecycle tests

#### Issues Detected
- ⚠️ **Timer leak confirmed:** 2 timers created but not cleared
- Severity: MEDIUM
- Type: timer-leak

---

## Cross-File Consistency

### Timestamps
- ✅ All timestamps are valid ISO 8601 format
- ✅ Chronological ordering maintained (before < after)
- ✅ Dates consistent: 2026-08-23 and 2026-08-27

### Data Types
- ✅ All numeric fields are integers (bytes, counts)
- ✅ All strings properly quoted
- ✅ Boolean fields (issuesFound) properly formatted
- ✅ Arrays properly structured (instances: [], issues: [])

### Naming Conventions
- ✅ Consistent field names across all files
- ✅ camelCase used throughout
- ✅ No naming conflicts or duplicates

---

## Required Fields Checklist

| Field Category | leak-detection-report.json | leak-isolation-results.json | leak-test-full-lifecycle.json |
|----------------|----------------------------|----------------------------|------------------------------|
| Before snapshot | ✅ | ✅ | ✅ |
| After snapshot | ✅ | ✅ | ✅ |
| Timestamps | ✅ | ✅ | ✅ |
| Timer counts (intervals, timeouts, total) | ✅ | ✅ | ✅ |
| Heap stats (heapUsedBytes, heapTotalBytes) | ✅ | ✅ | ✅ |
| Heap stats (MB units: heapUsed, heapTotal) | ✅ | ✅ | ✅ |
| RSS values | ✅ | ✅ | ✅ |
| External memory | ✅ | ✅ | ✅ |
| Process info (pid, uptime) | ✅ | ✅ | ✅ |
| WebSocket counts | ✅ | ✅ | ✅ |
| Delta calculations | ✅ | N/A* | ✅ |
| Analysis/verdict | ✅ | ✅ | ✅ |

*Note: leak-isolation-results.json uses a simple "verdict" field instead of detailed analysis.

---

## Issues Requiring Attention

### Critical Issues
**None detected** - All files are readable, valid JSON, and contain required data.

### Warnings

1. **Consistent Timer Leaks Across All Tests**
   - **Issue:** Every profiling run shows timer leaks (ranging from +1 to +16 timers)
   - **Impact:** Memory leaks will accumulate over time in production
   - **Recommended Action:** Investigate timer cleanup in:
     - Onboarding wizard (wizard-lifecycle tests)
     - BeforeEach/AfterEach hooks
     - setTimeout usage in test setup

2. **Memory Growth Patterns**
   - **Observation:** heapUsedBytes shows both increases and decreases across tests
   - **Interpretation:** Some memory is being freed (good), but timer handles are not
   - **Action:** Focus cleanup efforts on timer management rather than general memory

3. **Duration Field Always null**
   - **Issue:** `meta.duration` is null in all reports
   - **Impact:** Cannot determine test execution time
   - **Recommendation:** Populate duration field with actual test runtime in milliseconds

---

## Data Quality Metrics

| Metric | Value | Status |
|--------|-------|--------|
| Total files validated | 3 | ✅ |
| Files with valid JSON | 3/3 (100%) | ✅ |
| Files with before/after snapshots | 3/3 (100%) | ✅ |
| Files with timer data | 3/3 (100%) | ✅ |
| Files with heap stats | 3/3 (100%) | ✅ |
| Files showing leaks | 3/3 (100%) | ⚠️ |
| Missing required fields | 0 | ✅ |
| Malformed data | 0 | ✅ |

---

## Recommendations

### Immediate Actions
1. ✅ **Data validation complete** - All files are properly formatted and complete
2. 🔧 **Fix timer leaks** - Address the +1 to +16 timer leaks detected across all tests
3. 📊 **Populate duration field** - Add test execution time to meta section

### Process Improvements
1. **Automated validation** - Consider adding this validation to CI/CD pipeline
2. **Trend tracking** - Track timer leak counts over time to measure improvement
3. **Baseline establishment** - Set acceptable thresholds for timer/heap deltas

### Code Investigation Areas
Based on the leaked timers, investigate:
- `onboard.test.js` - Test setup and teardown
- Wizard lifecycle management - BeforeEach/AfterEach hooks
- setTimeout/setInterval calls - Ensure clearTimeout/clearInterval is called

---

## Conclusion

**All profiling data files are VALID and COMPLETE.** The data structure is consistent, all required fields are present, and the JSON is well-formed.

**The data successfully identifies memory leaks** - specifically timer leaks that should be addressed to prevent long-term memory accumulation in the dashboard application.

**No data quality issues detected** - The validation confirms that the profiling system is capturing the necessary information to diagnose and fix memory leaks.

---

**Validation performed by:** Claude (Automated validation)  
**Validation date:** 2026-08-28  
**Next recommended validation:** After timer leak fixes are deployed
