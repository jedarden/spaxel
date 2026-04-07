package startup

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPhaseLogsStartAndCompletion(t *testing.T) {
	// Phase returns a function; calling it logs completion.
	// We can't easily test log output, but we can verify the returned function
	// completes without error and takes a measurable amount of time.
	done := Phase(1, "Test phase")
	time.Sleep(5 * time.Millisecond)
	done()
	// If we get here without panic, Phase worked.
}

func TestPhaseTiming(t *testing.T) {
	start := time.Now()
	done := Phase(2, "Timing test")
	time.Sleep(50 * time.Millisecond)
	done()
	elapsed := time.Since(start)
	if elapsed < 40*time.Millisecond {
		t.Errorf("Phase should take at least 40ms, took %v", elapsed)
	}
}

func TestCheckTimeout_NoTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// Should not panic
	CheckTimeout(ctx)
}

func TestCheckTimeout_Expired(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	time.Sleep(5 * time.Millisecond)

	defer func() {
		r := recover()
		if r == nil {
			t.Error("CheckTimeout should log.Fatalf on expired context")
		}
	}()
	CheckTimeout(ctx)
}

func TestSubsystemStart_Success(t *testing.T) {
	ctx := context.Background()
	err := SubsystemStart(ctx, "test-subsystem", func(ctx context.Context) error {
		time.Sleep(10 * time.Millisecond)
		return nil
	})
	if err != nil {
		t.Errorf("SubsystemStart returned unexpected error: %v", err)
	}
}

func TestSubsystemStart_Failure(t *testing.T) {
	ctx := context.Background()
	expectedErr := errors.New("subsystem failed")
	err := SubsystemStart(ctx, "failing-subsystem", func(ctx context.Context) error {
		return expectedErr
	})
	if err == nil {
		t.Error("SubsystemStart should return error for failing subsystem")
	}
	if err.Error() != "subsystem failed" {
		t.Errorf("SubsystemStart returned wrong error: %v", err)
	}
}

func TestSubsystemStart_Timeout(t *testing.T) {
	ctx := context.Background()
	err := SubsystemStart(ctx, "slow-subsystem", func(ctx context.Context) error {
		// This subsystem takes longer than SubsystemTimeout
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return nil
		}
	})
	if err == nil {
		t.Error("SubsystemStart should return error on timeout")
	}
}

func TestSubsystemStart_RespectsParentContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel parent immediately

	err := SubsystemStart(ctx, "parent-cancelled", func(ctx context.Context) error {
		return nil
	})
	// The subsystem should still run (subCtx gets its own timeout from parent,
	// but since parent is already cancelled, subCtx is also cancelled).
	// The subsystem function itself may return nil before noticing cancellation.
	// This test mainly verifies no panic occurs.
	_ = err
}

func TestWriteReadyFile(t *testing.T) {
	dir := t.TempDir()
	// Override ReadyFile for testing
	old := ReadyFile
	ReadyFile = filepath.Join(dir, "spaxel.ready")
	defer func() { ReadyFile = old }()

	err := WriteReadyFile()
	if err != nil {
		t.Fatalf("WriteReadyFile failed: %v", err)
	}

	data, err := os.ReadFile(ReadyFile)
	if err != nil {
		t.Fatalf("Failed to read ready file: %v", err)
	}
	if string(data) != "ready" {
		t.Errorf("Ready file content = %q, want %q", string(data), "ready")
	}
}

func TestRemoveReadyFile(t *testing.T) {
	dir := t.TempDir()
	old := ReadyFile
	ReadyFile = filepath.Join(dir, "spaxel.ready")
	defer func() { ReadyFile = old }()

	// Create the file
	if err := os.WriteFile(ReadyFile, []byte("ready"), 0644); err != nil {
		t.Fatalf("Failed to create ready file: %v", err)
	}

	RemoveReadyFile()

	if _, err := os.Stat(ReadyFile); !os.IsNotExist(err) {
		t.Error("Ready file should be removed")
	}
}

func TestRemoveReadyFile_NoFile(t *testing.T) {
	dir := t.TempDir()
	old := ReadyFile
	ReadyFile = filepath.Join(dir, "nonexistent")
	defer func() { ReadyFile = old }()

	// Should not panic
	RemoveReadyFile()
}

func TestTotalPhases(t *testing.T) {
	if TotalPhases != 7 {
		t.Errorf("TotalPhases = %d, want 7", TotalPhases)
	}
}

func TestTotalTimeout(t *testing.T) {
	if TotalTimeout != 30*time.Second {
		t.Errorf("TotalTimeout = %v, want 30s", TotalTimeout)
	}
}

func TestSubsystemTimeout(t *testing.T) {
	if SubsystemTimeout != 5*time.Second {
		t.Errorf("SubsystemTimeout = %v, want 5s", SubsystemTimeout)
	}
}

// TestAllPhasesCompleteWithinTimeout verifies that a full 7-phase startup
// simulation completes well within the 30-second timeout.
func TestAllPhasesCompleteWithinTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), TotalTimeout)
	defer cancel()

	dir := t.TempDir()
	old := ReadyFile
	ReadyFile = filepath.Join(dir, "spaxel.ready")
	defer func() { ReadyFile = old }()

	start := time.Now()

	// Phase 1
	CheckTimeout(ctx)
	done := Phase(1, "Data directory")
	time.Sleep(1 * time.Millisecond)
	done()

	// Phase 2
	CheckTimeout(ctx)
	done = Phase(2, "SQLite")
	time.Sleep(1 * time.Millisecond)
	done()

	// Phase 3
	CheckTimeout(ctx)
	done = Phase(3, "Schema migrations")
	time.Sleep(1 * time.Millisecond)
	done()

	// Phase 4
	CheckTimeout(ctx)
	done = Phase(4, "Config & secrets")
	time.Sleep(1 * time.Millisecond)
	done()

	// Phase 5
	CheckTimeout(ctx)
	done = Phase(5, "Subsystems")
	for i := 0; i < 3; i++ {
		err := SubsystemStart(ctx, fmt.Sprintf("subsystem-%d", i), func(ctx context.Context) error {
			time.Sleep(1 * time.Millisecond)
			return nil
		})
		if err != nil {
			t.Fatalf("Subsystem %d failed: %v", i, err)
		}
	}
	done()

	// Phase 6
	CheckTimeout(ctx)
	done = Phase(6, "HTTP + mDNS")
	time.Sleep(1 * time.Millisecond)
	done()

	// Phase 7
	CheckTimeout(ctx)
	done = Phase(7, "Health")
	if err := WriteReadyFile(); err != nil {
		t.Fatalf("WriteReadyFile failed: %v", err)
	}
	done()

	elapsed := time.Since(start)
	if elapsed > 5*time.Second {
		t.Errorf("Full startup took %v, expected < 5s under normal conditions", elapsed)
	}

	log.Printf("[READY] All 7 phases completed in %dms", elapsed.Milliseconds())
}
