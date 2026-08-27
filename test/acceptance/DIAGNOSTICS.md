# Acceptance Test Diagnostic Instrumentation

## Overview

This diagnostic instrumentation helps pinpoint exactly where acceptance tests hang by providing:

1. **Interval-based goroutine dumps** (every 30 seconds) showing all running goroutines and their states
2. **Phase tracking** with timeout markers around major test phases
3. **IO operation logging** for all HTTP requests and websocket operations
4. **Channel and select statement tracking** to detect blocking operations
5. **Memory statistics** to detect leaks or excessive allocations
6. **Comprehensive stack traces** written to a dedicated diagnostic file

## Components

### `diagnostics.go`

Core diagnostic helper providing:

- `DiagnosticHelper` struct with methods for tracking test execution
- Automatic goroutine dumps every 30 seconds
- Phase-based timeout tracking
- IO operation logging
- Memory profiling
- SIGQUIT signal handling (documented for manual use)

### `acceptance_test.go` (Integrated)

The test harness now includes diagnostic logging:

- `TestHarness.diagnostics` field initialized in `NewTestHarness`
- All major operations wrapped with diagnostic calls
- Automatic capture of goroutine state on hangs and timeouts

### `run_with_diagnostics.sh`

Shell script to run tests with:

- Automatic timeout enforcement
- Output capture to log file
- Clear pass/fail reporting
- Instructions for finding diagnostic files

## Usage

### Running a Test with Diagnostics

```bash
# Run a specific test with 5-minute timeout
./test/acceptance/run_with_diagnostics.sh TestIO1_FreshInstallFirstBoot 5m

# Run a different test with 10-minute timeout
./test/acceptance/run_with_diagnostics.sh TestIO3_SingleNodeOnboarding 10m
```

### Direct Go Test with Diagnostics

```bash
# Enable integration tests and run with diagnostics
ACCEPTANCE_TEST=1 go test -v -run TestIO1_FreshInstallFirstBoot ./test/acceptance/...

# With timeout
timeout 5m go test -v -run TestIO1_FreshInstallFirstBoot ./test/acceptance/...
```

## Diagnostic Output

### Console Output

During test execution, you'll see:

```
[15:04:05.123] [PHASE] Entering: mothership-start at 2026-08-27T15:04:05Z
[15:04:05.234] [EVENT] Building mothership binary
[15:04:06.345] [EVENT] Mothership built successfully
[15:04:07.456] [PHASE] Entering: launch-mothership at 2026-08-27T15:04:07Z
[15:04:07.567] [EVENT] Launching mothership with DataDir: /tmp/spaxel-acceptance-12345
[15:04:08.678] [EVENT] Mothership started (PID: 12345)
```

### Diagnostic File

Full diagnostic file (`/tmp/spaxel-test-diagnostics-*.txt`) contains:

```
=== GOROUTINE DUMP [initial] ===
Time: 2026-08-27T15:04:10Z
Current Phase: health-check (elapsed: 2.5s)
Total Goroutines: 12
  - Running: 2
  - Runnable: 3
  - Waiting: 5
  - IO wait: 1
  - Syscall: 1
  - Sleeping: 0
  - Other: 0

Top functions by goroutine count:
  - main.(*TestHarness).WaitForHealth: 3
  - net/http.(*Transport).RoundTrip: 2
  - runtime.gopark: 2

Potentially blocked select statements:
  - goroutine 7 [select, 2 minutes]: main.(*TestHarness).WaitForHealth

=== END GOROUTINE DUMP ===

=== FULL STACK TRACE [initial] at 2026-08-27T15:04:10Z ===
goroutine 1 [running]:
testing.(*common).Log(...)
...
=== END FULL STACK TRACE ===
```

## Interpreting Results

### Identifying Hangs

1. **Check the phase**: The last phase entry shows where the test was when it hung
2. **Look for long-running goroutines**: Goroutines stuck in the same state across multiple dumps indicate blocking
3. **Examine "IO wait" and "syscall" states**: These show network/disk operations that may be stuck
4. **Review select statements**: Select statements that never fire indicate channels not being written to

### Common Patterns

**Network Hang** (IO wait or syscall with no completion):
```
goroutine 7 [IO wait, 30 seconds]:
net/http.(*Transport).awaitPhase2
net/http.(*Transport).getConn
```
→ Indicates HTTP request never completed (server not responding or firewall issue)

**Channel Block** (select with no case selected):
```
goroutine 12 [select, 45 seconds]:
main.(*TestHarness).WaitForNode
```
→ Indicates channel not being written to (expected message never arrived)

**Mutex Contention** (multiple goroutines on same Mutex.Lock):
```
goroutine 8 [sync.Mutex.Lock, 10 seconds]:
main.(*TestHarness).GetNodes
```
→ Indicates deadlock or severe contention

**Memory Leak** (memory stats showing continuous growth):
```
=== MEMORY STATS ===
Alloc: 512 MB (growing)
HeapObjects: 1500000 (increasing)
```
→ Indicates unbounded memory allocation

## Phase Categories

Tests are instrumented with these phases:

| Phase | Description | Common Hang Points |
|-------|-------------|-------------------|
| `build-mothership` | Compiling mothership binary | Build system issues, disk full |
| `launch-mothership` | Starting mothership process | Port conflicts, insufficient permissions |
| `health-check` | Waiting for /healthz endpoint | Server not starting, database lock |
| `build-simulator` | Compiling simulator binary | Build system issues |
| `simulator-start` | Starting spaxel-sim process | WebSocket connection failures |
| `wait-for-node` | Waiting for node to appear | Simulator not connecting, token rejection |
| `wait-for-event` | Waiting for events | Event pipeline blocked, database issues |
| `teardown` | Stopping all processes | Processes not responding to signals |

## Manual Goroutine Dump

To manually trigger a goroutine dump during a running test, send SIGQUIT to the test process:

```bash
# Find the test process PID
pgrep -f "go test.*acceptance"

# Send SIGQUIT (kill -3)
kill -QUIT <PID>
```

Note: The automatic periodic dumps (every 30 seconds) are usually sufficient.

## Troubleshooting

### Test Exits Immediately

**Problem**: Test exits without running
**Solution**: Ensure `ACCEPTANCE_TEST=1` or `SPAXEL_INTEGRATION_TEST=1` is set

### No Diagnostic File Created

**Problem**: `/tmp/spaxel-test-diagnostics-*.txt` not found
**Solution**: Check test output for file creation errors; may be permission issue

### Goroutine Dumps Missing

**Problem**: No goroutine dumps in output
**Solution**: Verify `diagnostics.go` is in the same package and compiled

### Excessive Output

**Problem**: Too much diagnostic noise
**Solution**: Focus on the last goroutine dump before timeout; that's where the hang occurred

## Performance Impact

Diagnostic overhead is minimal:

- Goroutine dumps: ~50ms every 30 seconds (negligible)
- Phase tracking: <1µs per call
- IO logging: <10µs per operation
- Memory: ~1MB additional for stack trace buffer

The diagnostics are designed to be safe for production use if needed.

## Examples

### Example 1: Pinpointing a WebSocket Hang

**Symptom**: Test hangs in `simulator-start` phase
**Diagnostic shows**:
```
goroutine 15 [IO wait, 30 seconds]:
net/http.(*Transport).RoundTrip
github.com/gorilla/websocket.(*Conn).Write
```
**Conclusion**: WebSocket write is blocking (server not reading from connection)

### Example 2: Identifying a Database Lock

**Symptom**: Test hangs in `health-check` phase
**Diagnostic shows**:
```
goroutine 8 [chan receive, 45 seconds]:
database/sql.(*Conn).QueryContext
main.(*TestHarness).WaitForHealth
```
**Conclusion**: Database query is stuck (possibly locked by another process)

### Example 3: Finding a Signal Delivery Issue

**Symptom**: `teardown` phase never completes
**Diagnostic shows**:
```
goroutine 1 [running]:
os/signal.signal_recv
syscall.Syscall
```
**Conclusion**: Signal not being delivered to subprocess (use Process.Kill instead)

## Future Enhancements

Potential improvements:

1. **Flame graph generation**: Visual representation of goroutine states over time
2. **CPU profiling**: pprof integration for performance analysis
3. **Network packet capture**: Wireshark integration for debugging network issues
4. **Lock contention analysis**: Detailed mutex/rwmutex tracking
5. **Custom event markers**: Allow test code to add custom diagnostic markers

## Contributing

When adding new acceptance tests:

1. Wrap major operations with `h.diagnostics.EnterPhase()`
2. Log significant events with `h.diagnostics.LogEvent()`
3. Add IO logging for new HTTP endpoints with `h.diagnostics.LogIO()`
4. Document any new phases in the phase table above

## References

- Go runtime debugging: https://go.dev/doc/diagnostics
- `runtime.Stack` documentation: https://pkg.go.dev/runtime#Stack
- Context cancellation: https://go.dev/blog/context
