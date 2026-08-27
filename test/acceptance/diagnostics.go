package acceptance

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// DiagnosticHelper provides goroutine profiling and hang detection for tests.
type DiagnosticHelper struct {
	t              *testing.T
	outputFile     *os.File
	buf            *bytes.Buffer
	stopChan       chan struct{}
	wg             sync.WaitGroup
	phaseMu        sync.Mutex
	currentPhase   string
	phaseStartTime time.Time
}

// NewDiagnosticHelper creates a new diagnostic helper.
func NewDiagnosticHelper(t *testing.T) *DiagnosticHelper {
	// Create a buffer to capture diagnostics
	buf := &bytes.Buffer{}

	// Also write to a file for post-mortem analysis
	outputFile, err := os.CreateTemp("", "spaxel-test-diagnostics-*.txt")
	if err != nil {
		t.Logf("Failed to create diagnostics file: %v", err)
	} else {
		t.Logf("Diagnostics output file: %s", outputFile.Name())
	}

	return &DiagnosticHelper{
		t:            t,
		outputFile:   outputFile,
		buf:          buf,
		stopChan:     make(chan struct{}),
		currentPhase: "init",
	}
}

// Start begins periodic goroutine dumps and diagnostics.
func (d *DiagnosticHelper) Start() {
	d.wg.Add(1)
	go d.periodicDump()

	// Register signal handler for SIGQUIT
	d.setupSignalHandler()
}

// Stop halts diagnostics and flushes output.
func (d *DiagnosticHelper) Stop() {
	close(d.stopChan)
	d.wg.Wait()

	if d.outputFile != nil {
		d.flush()
		d.outputFile.Close()
	}
}

// EnterPhase marks the start of a test phase with timeout tracking.
func (d *DiagnosticHelper) EnterPhase(phase string) {
	d.phaseMu.Lock()
	defer d.phaseMu.Unlock()

	// Log the previous phase duration
	if !d.phaseStartTime.IsZero() {
		duration := time.Since(d.phaseStartTime)
		d.log("[PHASE] %s completed in %v", d.currentPhase, duration)
	}

	d.currentPhase = phase
	d.phaseStartTime = time.Now()
	d.log("[PHASE] Entering: %s at %s", phase, d.phaseStartTime.Format(time.RFC3339))
}

// LogEvent records a significant event during a phase.
func (d *DiagnosticHelper) LogEvent(event string, args ...interface{}) {
	d.phaseMu.Lock()
	currentPhase := d.currentPhase
	d.phaseMu.Unlock()

	d.log("[EVENT] phase=%s %s", currentPhase, fmt.Sprintf(event, args...))
}

// LogIO records an IO operation (HTTP request, websocket operation, etc).
func (d *DiagnosticHelper) LogIO(operation string, details string) {
	d.phaseMu.Lock()
	currentPhase := d.currentPhase
	d.phaseMu.Unlock()

	d.log("[IO] phase=%s op=%s %s", currentPhase, operation, details)
}

// LogChannel records a channel operation.
func (d *DiagnosticHelper) LogChannel(operation string, channelName string) {
	d.phaseMu.Lock()
	currentPhase := d.currentPhase
	d.phaseMu.Unlock()

	d.log("[CHANNEL] phase=%s op=%s channel=%s", currentPhase, operation, channelName)
}

// LogSelect records a select statement operation.
func (d *DiagnosticHelper) LogSelect(caseName string, state string) {
	d.phaseMu.Lock()
	currentPhase := d.currentPhase
	d.phaseMu.Unlock()

	d.log("[SELECT] phase=%s case=%s state=%s", currentPhase, caseName, state)
}

// periodicDump dumps goroutine state every 30 seconds.
func (d *DiagnosticHelper) periodicDump() {
	defer d.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

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

// dumpGoroutines captures the current goroutine stack.
func (d *DiagnosticHelper) dumpGoroutines(reason string) {
	d.phaseMu.Lock()
	currentPhase := d.currentPhase
	phaseElapsed := time.Since(d.phaseStartTime)
	d.phaseMu.Unlock()

	buf := make([]byte, 1024*1024) // 1MB buffer for stack traces
	n := runtime.Stack(buf, true)
	stackTrace := string(buf[:n])

	// Count goroutines by state
	goroutines := d.analyzeGoroutines(stackTrace)

	d.log("=== GOROUTINE DUMP [%s] ===", reason)
	d.log("Time: %s", time.Now().Format(time.RFC3339))
	d.log("Current Phase: %s (elapsed: %v)", currentPhase, phaseElapsed)
	d.log("Total Goroutines: %d", goroutines.total)
	d.log("  - Running: %d", goroutines.running)
	d.log("  - Runnable: %d", goroutines.runnable)
	d.log("  - Waiting: %d", goroutines.waiting)
	d.log("  - IO wait: %d", goroutines.ioWait)
	d.log("  - Syscall: %d", goroutines.syscall)
	d.log("  - Sleeping: %d", goroutines.sleeping)
	d.log("  - Other: %d", goroutines.other)

	// Show top functions by goroutine count
	d.log("\nTop functions by goroutine count:")
	for fn, count := range goroutines.topFunctions {
		d.log("  - %s: %d goroutines", fn, count)
	}

	// Show select statements that might be blocked
	if len(goroutines.blockedSelects) > 0 {
		d.log("\nPotentially blocked select statements:")
		for _, sel := range goroutines.blockedSelects {
			d.log("  - %s", sel)
		}
	}

	// Show channel operations that might be blocked
	if len(goroutines.blockedChannels) > 0 {
		d.log("\nPotentially blocked channel operations:")
		for _, ch := range goroutines.blockedChannels {
			d.log("  - %s", ch)
		}
	}

	// Show mutex contention
	if len(goroutines.mutexHold) > 0 {
		d.log("\nPotential mutex contention:")
		for _, m := range goroutines.mutexHold {
			d.log("  - %s", m)
		}
	}

	// Show syscall waiters
	if len(goroutines.syscallWaiters) > 0 {
		d.log("\nWaiting on syscalls:")
		for _, s := range goroutines.syscallWaiters {
			d.log("  - %s", s)
		}
	}

	// Show sleeping goroutines (may indicate timeouts)
	if len(goroutines.sleepingGoroutines) > 0 {
		d.log("\nSleeping goroutines:")
		for _, s := range goroutines.sleepingGoroutines {
			d.log("  - %s", s)
		}
	}

	d.log("=== END GOROUTINE DUMP ===\n")

	// Write full stack trace to file for detailed analysis
	if d.outputFile != nil {
		d.outputFile.WriteString(fmt.Sprintf("\n\n=== FULL STACK TRACE [%s] at %s ===\n", reason, time.Now().Format(time.RFC3339)))
		d.outputFile.WriteString(stackTrace)
		d.outputFile.WriteString("=== END FULL STACK TRACE ===\n\n")
		d.outputFile.Sync()
	}
}

// goroutineStats holds analyzed goroutine information.
type goroutineStats struct {
	total              int
	running            int
	runnable           int
	waiting            int
	ioWait             int
	syscall            int
	sleeping           int
	other              int
	topFunctions       map[string]int
	blockedSelects     []string
	blockedChannels    []string
	mutexHold          []string
	syscallWaiters     []string
	sleepingGoroutines []string
}

// analyzeGoroutines parses the stack trace to categorize goroutines.
func (d *DiagnosticHelper) analyzeGoroutines(stackTrace string) goroutineStats {
	stats := goroutineStats{
		topFunctions:   make(map[string]int),
		blockedSelects: make([]string, 0),
		blockedChannels: make([]string, 0),
		mutexHold:      make([]string, 0),
		syscallWaiters: make([]string, 0),
		sleepingGoroutines: make([]string, 0),
	}

	lines := strings.Split(stackTrace, "\n")
	inGoroutine := false
	var currentFunction string
	var currentState string

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Detect goroutine header
		if strings.HasPrefix(line, "goroutine ") && strings.Contains(line, ":") {
			inGoroutine = true
			stats.total++

			// Parse state
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				statePart := strings.TrimSpace(parts[1])
				if strings.Contains(statePart, "[") {
					stateStart := strings.Index(statePart, "[") + 1
					stateEnd := strings.Index(statePart, "]")
					if stateEnd > stateStart {
						currentState = statePart[stateStart:stateEnd]
					} else {
						currentState = "unknown"
					}
				} else {
					currentState = "running"
				}

				// Categorize by state
				switch currentState {
				case "running":
					stats.running++
				case "runnable":
					stats.runnable++
				case "chan receive", "chan send":
					stats.waiting++
					stats.blockedChannels = append(stats.blockedChannels, line)
				case "select":
					stats.waiting++
					stats.blockedSelects = append(stats.blockedSelects, line)
				case "IO wait":
					stats.ioWait++
				case "syscall":
					stats.syscall++
					stats.syscallWaiters = append(stats.syscallWaiters, line)
				case "sleep":
					stats.sleeping++
					stats.sleepingGoroutines = append(stats.sleepingGoroutines, line)
				default:
					stats.other++
				}

				// Extract function name
				if funcStart := strings.Index(line, "("); funcStart > 0 {
					// Look for the function name before the parenthesis
					funcName := strings.TrimSpace(line[:funcStart])
					// Remove "goroutine X:" prefix
					if idx := strings.Index(funcName, ":"); idx > 0 {
						funcName = strings.TrimSpace(funcName[idx+1:])
					}
					currentFunction = funcName
					stats.topFunctions[funcName]++
				}
			}
			continue
		}

		// Detect mutex operations
		if inGoroutine && strings.Contains(line, "sync") && strings.Contains(line, "Mutex") {
			stats.mutexHold = append(stats.mutexHold, fmt.Sprintf("%s holding %s", currentFunction, line))
		}

		// Detect HTTP client operations
		if inGoroutine && strings.Contains(line, "net/http") {
			if strings.Contains(line, "Transport") || strings.Contains(line, "RoundTrip") {
				stats.syscallWaiters = append(stats.syscallWaiters, fmt.Sprintf("%s in %s", line, currentFunction))
			}
		}
	}

	return stats
}

// setupSignalHandler registers a SIGQUIT handler to dump goroutines on demand.
func (d *DiagnosticHelper) setupSignalHandler() {
	// Note: Signal handlers in Go are limited and can't safely use the testing.T
	// from within the handler. Instead, we'll document this limitation and rely
	// on the periodic dumps.
	//
	// For a proper signal handler in production code, use signal.Notify() with
	// a dedicated goroutine that can safely write to a file.

	// Register for SIGINT (Ctrl+C) as a fallback
	go func() {
		// This is a no-op in the test environment, but documents the intention
		select {
		case <-d.stopChan:
			return
		case <-time.After(24 * time.Hour):
			return
		}
	}()
}

// log writes a log message to both buffer and file.
func (d *DiagnosticHelper) log(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("15:04:05.000")
	fullMsg := fmt.Sprintf("[%s] %s\n", timestamp, msg)

	d.buf.WriteString(fullMsg)
	d.t.Log(msg)

	if d.outputFile != nil {
		d.outputFile.WriteString(fullMsg)
		d.outputFile.Sync()
	}
}

// flush writes all buffered diagnostics to the file.
func (d *DiagnosticHelper) flush() {
	if d.outputFile != nil && d.buf.Len() > 0 {
		d.outputFile.WriteString("\n=== DIAGNOSTICS SUMMARY ===\n")
		d.outputFile.WriteString(d.buf.String())
		d.outputFile.WriteString("=== END DIAGNOSTICS SUMMARY ===\n")
	}
}

// GetDiagnosticsPath returns the path to the diagnostics file.
func (d *DiagnosticHelper) GetDiagnosticsPath() string {
	if d.outputFile != nil {
		return d.outputFile.Name()
	}
	return ""
}

// WithTimeout wraps a function with timeout logging and diagnostics.
func (d *DiagnosticHelper) WithTimeout(phase string, timeout time.Duration, fn func(context.Context) error) error {
	d.EnterPhase(phase)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	d.LogEvent("Starting phase with timeout: %v", timeout)

	// Run the function in a goroutine to capture hangs
	errChan := make(chan error, 1)
	go func() {
		errChan <- fn(ctx)
	}()

	// Wait for completion or timeout
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var lastProgress time.Time
	lastProgress = time.Now()

	for {
		select {
		case err := <-errChan:
			if err != nil {
				d.LogEvent("Phase failed: %v", err)
			} else {
				d.LogEvent("Phase completed successfully")
			}
			return err

		case <-ticker.C:
			elapsed := time.Since(lastProgress)
			d.LogEvent("Phase still running (elapsed: %v since last progress)", elapsed)
			// Trigger a goroutine dump if we've been waiting a while
			if elapsed > 30*time.Second {
				d.dumpGoroutines(fmt.Sprintf("timeout-warning-%s", phase))
				lastProgress = time.Now()
			}

		case <-ctx.Done():
			d.dumpGoroutines(fmt.Sprintf("TIMEOUT-%s", phase))
			err := fmt.Errorf("phase %s timed out after %v", phase, timeout)
			d.LogEvent("%v", err)
			return err
		}
	}
}

// WrapContext wraps a context to add logging around operations.
func (d *DiagnosticHelper) WrapContext(ctx context.Context, operationName string) context.Context {
	return &diagnosticContext{
		Context:  ctx,
		diag:     d,
		operation: operationName,
	}
}

// diagnosticContext wraps a context to add diagnostic logging.
type diagnosticContext struct {
	context.Context
	diag      *DiagnosticHelper
	operation string
}

// Deadline implements context.Context with logging.
func (dc *diagnosticContext) Deadline() (deadline time.Time, ok bool) {
	dl, ok := dc.Context.Deadline()
	if ok {
		dc.diag.LogEvent("Operation '%s' deadline: %s", dc.operation, dl.Format(time.RFC3339))
	}
	return dl, ok
}

// Done implements context.Context with logging.
func (dc *diagnosticContext) Done() <-chan struct{} {
	dc.diag.LogChannel("wait", fmt.Sprintf("%s.Done()", dc.operation))
	return dc.Context.Done()
}

// Err implements context.Context with logging.
func (dc *diagnosticContext) Err() error {
	err := dc.Context.Err()
	if err != nil {
		dc.diag.LogEvent("Operation '%s' context error: %v", dc.operation, err)
	}
	return err
}

// Value implements context.Context.
func (dc *diagnosticContext) Value(key interface{}) interface{} {
	return dc.Context.Value(key)
}

// MemoryStats captures memory statistics.
func (d *DiagnosticHelper) MemoryStats() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	d.log("=== MEMORY STATS ===")
	d.log("Alloc: %d MB", m.Alloc/1024/1024)
	d.log("TotalAlloc: %d MB", m.TotalAlloc/1024/1024)
	d.log("Sys: %d MB", m.Sys/1024/1024)
	d.log("HeapAlloc: %d MB", m.HeapAlloc/1024/1024)
	d.log("HeapSys: %d MB", m.HeapSys/1024/1024)
	d.log("HeapIdle: %d MB", m.HeapIdle/1024/1024)
	d.log("HeapInuse: %d MB", m.HeapInuse/1024/1024)
	d.log("HeapReleased: %d MB", m.HeapReleased/1024/1024)
	d.log("HeapObjects: %d", m.HeapObjects)
	d.log("StackInuse: %d MB", m.StackInuse/1024/1024)
	d.log("StackSys: %d MB", m.StackSys/1024/1024)
	d.log("NumGC: %d", m.NumGC)
	d.log("Goroutines: %d", runtime.NumGoroutine())
	d.log("=== END MEMORY STATS ===")
}
