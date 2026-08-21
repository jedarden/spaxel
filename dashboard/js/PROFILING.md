# Test Suite Profiling

This directory contains tools to profile the `onboard.test.js` test suite and capture evidence of resource leaks between tests.

## Tools

### 1. `onboard.test.profiling.js`

Core profiling infrastructure providing:

- **ResourceTracker** - Captures snapshots of:
  - Active timer handles (setTimeout)
  - Active interval handles (setInterval)
  - WebSocket instances
  - Event listeners
  - DOM nodes
  - Heap usage

- **Instrumentation** - Monkey-patches global APIs to track resource creation:
  - Wraps `setTimeout`/`clearTimeout`
  - Wraps `setInterval`/`clearInterval`
  - Wraps `WebSocket` constructor

- **ProfilingTestRunner** - Orchestrates profiling across test runs

### 2. `run-profiled-tests.js`

Runs the existing Jest test suite and captures system-level metrics:

```bash
node dashboard/js/run-profiled-tests.js
```

**Captures:**
- Active handles before/after test run
- Active requests before/after test run
- Heap usage growth
- Handle type breakdown
- Writes report to `onboard.test.profiling-output.json`

### 3. `profile-suite.js`

Standalone profiling suite that instruments the test environment:

```bash
node dashboard/js/profile-suite.js
```

**Features:**
- Installs instrumentation before tests
- Tracks timers/intervals/WebSockets during execution
- Captures snapshots at key points
- Generates detailed JSON report

### 4. `profile-demo.js`

Demonstration script showing profiling in action:

```bash
node dashboard/js/profile-demo.js
```

**Demonstrates:**
- Resource tracking setup
- Snapshot capture
- Report generation
- Leak detection (intentionally leaks an interval to show it works)

## Output Format

All tools generate `onboard.test.profiling-output.json` with structure:

```json
{
  "timestamp": "2024-08-21T...",
  "summary": {
    "totalSnapshots": 5,
    "totalLeakedTimers": 2,
    "totalLeakedIntervals": 1,
    "totalLeakedWebSockets": 0,
    "totalHeapGrowth": 1048576
  },
  "snapshots": [
    {
      "label": "BEFORE_ALL",
      "timestamp": 1692619200000,
      "timers": 0,
      "intervals": 0,
      "websockets": 0,
      "memory": { "heapUsed": 12345678, ... }
    },
    ...
  ],
  "deltas": [
    {
      "fromLabel": "BEFORE_ALL",
      "toLabel": "AFTER_TEST_1",
      "timers": 1,
      "intervals": 0,
      "websockets": 0,
      "heapGrowth": 4096
    },
    ...
  ]
}
```

## Interpreting Results

### Concerning Signs

- **Leaked timers/intervals**: Resources created but not cleaned up
- **Heap growth > 1 MB per test**: Possible memory leak
- **Growing WebSocket count**: Connections not closed

### Acceptable Patterns

- **Small heap growth**: Test data accumulation (acceptable)
- **Transient handles**: Handles created and cleaned within a test
- **Zero or negative deltas**: Proper cleanup

## Integration with CI

Add to CI pipeline:

```yaml
# .github/workflows/test.yml (or Argo Workflow)
- name: Run profiled tests
  run: node dashboard/js/run-profiled-tests.js

- name: Check for leaks
  run: |
    node -e "
      const report = require('./dashboard/js/onboard.test.profiling-output.json');
      if (report.summary.totalLeakedTimers > 5 ||
          report.summary.totalLeakedIntervals > 2 ||
          report.summary.totalHeapGrowth > 5242880) {
        process.exit(1);
      }
    "
```

## Next Steps

1. **Run the full suite**: `npm test -- dashboard/js/onboard.test.js --verbose`
2. **Profile the run**: `node dashboard/js/run-profiled-tests.js`
3. **Analyze output**: Check `onboard.test.profiling-output.json`
4. **Fix leaks**: Update `afterEach` hooks in test to clean up resources
5. **Verify**: Re-run profiling to confirm cleanup

## Current Known Issues

The test suite has `afterEach` hooks that attempt cleanup:
```javascript
afterEach(() => {
    if (_state.pollTimer) { clearInterval(_state.pollTimer); _state.pollTimer = null; }
    if (_state.calibrateTimer) { clearTimeout(_state.calibrateTimer); _state.calibrateTimer = null; }
    if (_state.ws) { _state.ws.close(); _state.ws = null; }
});
```

Profiling will reveal if any tests are creating resources that escape this cleanup.
