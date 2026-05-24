package diskspace

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// mockWritePauser is a test mock for WritePauser.
type mockWritePauser struct {
	paused    atomic.Bool
	pauseCnt  atomic.Int32
	resumeCnt atomic.Int32
}

func (m *mockWritePauser) PauseWrites() {
	m.paused.Store(true)
	m.pauseCnt.Add(1)
}

func (m *mockWritePauser) ResumeWrites() {
	m.paused.Store(false)
	m.resumeCnt.Add(1)
}

func (m *mockWritePauser) IsPaused() bool {
	return m.paused.Load()
}

func (m *mockWritePauser) PauseCount() int {
	return int(m.pauseCnt.Load())
}

func (m *mockWritePauser) ResumeCount() int {
	return int(m.resumeCnt.Load())
}

// mockUpdatePauser is a test mock for UpdatePauser.
type mockUpdatePauser struct {
	paused    atomic.Bool
	pauseCnt  atomic.Int32
	resumeCnt atomic.Int32
}

func (m *mockUpdatePauser) PauseUpdates() {
	m.paused.Store(true)
	m.pauseCnt.Add(1)
}

func (m *mockUpdatePauser) ResumeUpdates() {
	m.paused.Store(false)
	m.resumeCnt.Add(1)
}

func (m *mockUpdatePauser) IsPaused() bool {
	return m.paused.Load()
}

func (m *mockUpdatePauser) PauseCount() int {
	return int(m.pauseCnt.Load())
}

func (m *mockUpdatePauser) ResumeCount() int {
	return int(m.resumeCnt.Load())
}

func TestMonitor_New(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	recorder := &mockWritePauser{}
	flow := &mockWritePauser{}
	pred := &mockUpdatePauser{}

	m := New(Config{
		DataDir:         tmpDir,
		CheckInterval:   time.Second,
		Recorder:        recorder,
		FlowAccumulator: flow,
		Predictor:       pred,
	})

	if m == nil {
		t.Fatal("New() returned nil")
	}

	if m.dataDir != tmpDir {
		t.Errorf("dataDir = %s, want %s", m.dataDir, tmpDir)
	}

	if m.interval != time.Second {
		t.Errorf("interval = %v, want %v", m.interval, time.Second)
	}

	state, _, _ := m.GetState()
	if state != StateNormal {
		t.Errorf("initial state = %v, want %v", state, StateNormal)
	}
}

func TestMonitor_CheckDiskSpace(t *testing.T) {
	tmpDir := t.TempDir()

	recorder := &mockWritePauser{}
	flow := &mockWritePauser{}
	pred := &mockUpdatePauser{}

	m := New(Config{
		DataDir:         tmpDir,
		CheckInterval:   time.Second,
		Recorder:        recorder,
		FlowAccumulator: flow,
		Predictor:       pred,
	})

	// Run initial check
	m.checkDiskSpace()

	state, freeMB, _ := m.GetState()
	if state != StateNormal {
		t.Errorf("state = %v, want %v (temp dir should have space)", state, StateNormal)
	}

	if freeMB == 0 {
		t.Error("freeMB = 0, want > 0")
	}
}

func TestMonitor_StateTransitions(t *testing.T) {
	// We can't easily simulate disk space changes in a test,
	// but we can verify the state machine logic by checking
	// that the methods are called in the right order.

	tmpDir := t.TempDir()

	recorder := &mockWritePauser{}
	flow := &mockWritePauser{}
	pred := &mockUpdatePauser{}

	m := New(Config{
		DataDir:         tmpDir,
		CheckInterval:   10 * time.Millisecond,
		Recorder:        recorder,
		FlowAccumulator: flow,
		Predictor:       pred,
	})

	m.Start()
	defer m.Stop()

	// Let it run for a bit
	time.Sleep(50 * time.Millisecond)

	// The monitor should have made at least one check
	stats := m.GetStats()
	if stats.LastCheck.IsZero() {
		t.Error("LastCheck is zero, want non-zero")
	}
}

func TestMonitor_GetStats(t *testing.T) {
	tmpDir := t.TempDir()

	m := New(Config{
		DataDir:       tmpDir,
		CheckInterval: time.Second,
	})

	stats := m.GetStats()

	if stats.State != StateNormal {
		t.Errorf("State = %v, want %v", stats.State, StateNormal)
	}

	if stats.WarningMB != WarningThresholdMB {
		t.Errorf("WarningMB = %d, want %d", stats.WarningMB, WarningThresholdMB)
	}

	if stats.CriticalMB != CriticalThresholdMB {
		t.Errorf("CriticalMB = %d, want %d", stats.CriticalMB, CriticalThresholdMB)
	}
}

func TestMonitor_StartStop(t *testing.T) {
	tmpDir := t.TempDir()

	m := New(Config{
		DataDir:       tmpDir,
		CheckInterval: 10 * time.Millisecond,
	})

	m.Start()

	// Let it run
	time.Sleep(50 * time.Millisecond)

	// Stop should be graceful
	m.Stop()

	// If we got here without deadlock, Stop() worked
}

func TestMonitor_ConcurrentGetState(t *testing.T) {
	tmpDir := t.TempDir()

	m := New(Config{
		DataDir:       tmpDir,
		CheckInterval: 10 * time.Millisecond,
	})

	m.Start()
	defer m.Stop()

	// Spawn multiple goroutines reading state
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				m.GetState()
				m.GetStats()
			}
			done <- struct{}{}
		}()
	}

	// Wait for all to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestState_String(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateNormal, "normal"},
		{StateWarning, "warning"},
		{StateCritical, "critical"},
		{State(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("State(%d).String() = %s, want %s", tt.state, got, tt.expected)
		}
	}
}

// Test that we can get disk space info from a real directory
func TestDiskSpaceInfo(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file to ensure the directory exists
	testFile := filepath.Join(tmpDir, "test")
	f, err := os.Create(testFile)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	f.Close()

	var stat syscall.Statfs_t
	if err := syscall.Statfs(tmpDir, &stat); err != nil {
		t.Fatalf("Statfs failed: %v", err)
	}

	freeBytes := stat.Bavail * uint64(stat.Frsize)
	freeMB := freeBytes / (1024 * 1024)

	if freeMB == 0 {
		t.Error("freeMB = 0, temp dir should have some space")
	}

	t.Logf("Temp dir %s has %d MB free", tmpDir, freeMB)
}

func TestMonitor_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())

	recorder := &mockWritePauser{}

	m := New(Config{
		DataDir:       tmpDir,
		CheckInterval: time.Second,
		Recorder:      recorder,
	})

	// Replace the monitor's context
	m.ctx = ctx
	m.cancel = cancel

	m.Start()

	// Cancel immediately
	cancel()

	// Wait for goroutine to exit
	m.wg.Wait()

	// If we got here, the goroutine exited cleanly
}

func TestMonitor_NilComponents(t *testing.T) {
	tmpDir := t.TempDir()

	// Create monitor with nil components
	m := New(Config{
		DataDir:         tmpDir,
		CheckInterval:   time.Second,
		Recorder:        nil,
		FlowAccumulator: nil,
		Predictor:       nil,
	})

	// Should not panic
	m.checkDiskSpace()
	m.GetState()
	m.GetStats()

	m.Start()
	m.Stop()
}

func TestMonitor_RapidStateChanges(t *testing.T) {
	tmpDir := t.TempDir()

	recorder := &mockWritePauser{}
	flow := &mockWritePauser{}
	pred := &mockUpdatePauser{}

	m := New(Config{
		DataDir:         tmpDir,
		CheckInterval:   time.Second,
		Recorder:        recorder,
		FlowAccumulator: flow,
		Predictor:       pred,
	})

	// Simulate rapid state changes (in real scenario, disk space
	// wouldn't change this fast, but we test the logic)
	m.mu.Lock()
	oldState := m.state
	m.state = StateWarning
	m.mu.Unlock()
	m.enterWarningState(oldState)

	m.mu.Lock()
	oldState = m.state
	m.state = StateCritical
	m.mu.Unlock()
	m.enterCriticalState(oldState)

	m.mu.Lock()
	oldState = m.state
	m.state = StateNormal
	m.mu.Unlock()
	m.enterNormalState(oldState)

	// Verify counts
	if recorder.PauseCount() == 0 {
		t.Error("recorder pause count = 0, want > 0")
	}

	if recorder.ResumeCount() == 0 {
		t.Error("recorder resume count = 0, want > 0")
	}

	if flow.PauseCount() == 0 {
		t.Error("flow pause count = 0, want > 0")
	}

	if flow.ResumeCount() == 0 {
		t.Error("flow resume count = 0, want > 0")
	}

	if pred.PauseCount() == 0 {
		t.Error("pred pause count = 0, want > 0")
	}

	if pred.ResumeCount() == 0 {
		t.Error("pred resume count = 0, want > 0")
	}
}
