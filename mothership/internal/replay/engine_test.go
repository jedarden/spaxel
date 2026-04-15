// Package replay tests for the replay engine.
package replay

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/spaxel/mothership/internal/recording"
)

// mockBroadcaster implements BlobBroadcaster for testing.
type mockBroadcaster struct {
	blobs     []BlobUpdate
	timestamp int64
	mu        sync.Mutex
	calls     int
}

func (m *mockBroadcaster) BroadcastReplayBlobs(blobs []BlobUpdate, timestampMS int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blobs = blobs
	m.timestamp = timestampMS
	m.calls++
}

func (m *mockBroadcaster) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// TestNewEngine verifies engine creation.
func TestNewEngine(t *testing.T) {
	tempDir := t.TempDir()
	bufferPath := filepath.Join(tempDir, "test.bin")
	buffer, err := recording.NewBuffer(bufferPath, 1, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewBuffer: %v", err)
	}
	defer buffer.Close()

	broadcaster := &mockBroadcaster{}
	engine := NewEngine(buffer, broadcaster)

	if engine == nil {
		t.Fatal("NewEngine returned nil")
	}
	if engine.sessions == nil {
		t.Error("sessions map not initialized")
	}
	if engine.defaultParams == nil {
		t.Error("defaultParams not initialized")
	}
}

// TestStartSession verifies session creation.
func TestStartSession(t *testing.T) {
	tempDir := t.TempDir()
	bufferPath := filepath.Join(tempDir, "test.bin")
	buffer, err := recording.NewBuffer(bufferPath, 1, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewBuffer: %v", err)
	}
	defer buffer.Close()

	// Write some test data
	now := time.Now().UnixNano()
	frame := make([]byte, 152)
	for i := 0; i < 10; i++ {
		ts := now + int64(i)*int64(50*time.Millisecond)
		if err := buffer.Append(ts, frame); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	broadcaster := &mockBroadcaster{}
	engine := NewEngine(buffer, broadcaster)

	// Start session
	fromMS := time.Unix(0, now).UnixMilli()
	toMS := fromMS + 500 // 500ms range

	session, err := engine.StartSession(fromMS, toMS)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	if session == nil {
		t.Fatal("session is nil")
	}
	if session.State() != StatePaused {
		t.Errorf("State = %v, want StatePaused", session.State())
	}
	if session.CurrentMS() != fromMS {
		t.Errorf("CurrentMS = %d, want %d", session.CurrentMS(), fromMS)
	}
	if session.Speed() != 1 {
		t.Errorf("Speed = %d, want 1", session.Speed())
	}
	if session.Params() == nil {
		t.Error("Params is nil")
	}
}

// TestStartSessionClampsRange verifies that requested range is clamped to available data.
func TestStartSessionClampsRange(t *testing.T) {
	tempDir := t.TempDir()
	bufferPath := filepath.Join(tempDir, "test.bin")
	buffer, err := recording.NewBuffer(bufferPath, 1, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewBuffer: %v", err)
	}
	defer buffer.Close()

	// Write test data with known timestamps
	baseTime := time.Unix(1_000_000, 0).UnixNano()
	frame := make([]byte, 152)
	for i := 0; i < 5; i++ {
		ts := baseTime + int64(i)*int64(time.Second)
		if err := buffer.Append(ts, frame); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	broadcaster := &mockBroadcaster{}
	engine := NewEngine(buffer, broadcaster)

	// Request range that extends beyond available data
	requestedFrom := time.Unix(1_000_000, 0).UnixMilli() - 1000 // 1 second before data
	requestedTo := time.Unix(1_000_000, 0).UnixMilli() + 6000   // 1 second after data

	session, err := engine.StartSession(requestedFrom, requestedTo)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	// Should be clamped to actual data range
	expectedFrom := time.Unix(1_000_000, 0).UnixMilli()
	expectedTo := time.Unix(1_000_004, 0).UnixMilli() // 5th frame is at +4 seconds

	if session.FromMS() != expectedFrom {
		t.Errorf("FromMS = %d, want %d (should be clamped to oldest)", session.FromMS(), expectedFrom)
	}
	if session.ToMS() != expectedTo {
		t.Errorf("ToMS = %d, want %d (should be clamped to newest)", session.ToMS(), expectedTo)
	}
}

// TestStartSessionRejectsInvalidRange verifies that from > to is rejected.
func TestStartSessionRejectsInvalidRange(t *testing.T) {
	tempDir := t.TempDir()
	bufferPath := filepath.Join(tempDir, "test.bin")
	buffer, err := recording.NewBuffer(bufferPath, 1, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewBuffer: %v", err)
	}
	defer buffer.Close()

	broadcaster := &mockBroadcaster{}
	engine := NewEngine(buffer, broadcaster)

	// Request invalid range (from > to)
	fromMS := int64(1000)
	toMS := int64(500)

	_, err = engine.StartSession(fromMS, toMS)
	if err == nil {
		t.Error("Expected error for from > to, got nil")
	}
}

// TestStopSession verifies session stopping.
func TestStopSession(t *testing.T) {
	tempDir := t.TempDir()
	bufferPath := filepath.Join(tempDir, "test.bin")
	buffer, err := recording.NewBuffer(bufferPath, 1, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewBuffer: %v", err)
	}
	defer buffer.Close()

	broadcaster := &mockBroadcaster{}
	engine := NewEngine(buffer, broadcaster)

	session, err := engine.StartSession(0, 1000)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	err = engine.StopSession(session.ID())
	if err != nil {
		t.Fatalf("StopSession: %v", err)
	}

	// Verify session was removed
	_, ok := engine.GetSession(session.ID())
	if ok {
		t.Error("Session still exists after StopSession")
	}
}

// TestStopSessionNotFound verifies error on unknown session ID.
func TestStopSessionNotFound(t *testing.T) {
	tempDir := t.TempDir()
	bufferPath := filepath.Join(tempDir, "test.bin")
	buffer, err := recording.NewBuffer(bufferPath, 1, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewBuffer: %v", err)
	}
	defer buffer.Close()

	broadcaster := &mockBroadcaster{}
	engine := NewEngine(buffer, broadcaster)

	err = engine.StopSession("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent session")
	}
}

// TestSeek verifies seeking to a timestamp.
func TestSeek(t *testing.T) {
	tempDir := t.TempDir()
	bufferPath := filepath.Join(tempDir, "test.bin")
	buffer, err := recording.NewBuffer(bufferPath, 1, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewBuffer: %v", err)
	}
	defer buffer.Close()

	// Write test data
	baseTime := time.Unix(1_000_000, 0).UnixNano()
	frame := make([]byte, 152)
	timestamps := make([]int64, 5)
	for i := 0; i < 5; i++ {
		timestamps[i] = baseTime + int64(i)*int64(time.Second)
		if err := buffer.Append(timestamps[i], frame); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	broadcaster := &mockBroadcaster{}
	engine := NewEngine(buffer, broadcaster)

	session, err := engine.StartSession(
		time.Unix(1_000_000, 0).UnixMilli(),
		time.Unix(1_000_010, 0).UnixMilli(), // 10 seconds of range
	)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	// Seek to the third frame
	targetMS := timestamps[2] / 1_000_000 // Convert ns to ms
	err = engine.Seek(session.ID(), targetMS)
	if err != nil {
		t.Fatalf("Seek: %v", err)
	}

	// Verify position was updated
	if session.State() != StatePaused {
		t.Errorf("State = %v, want StatePaused after seek", session.State())
	}
	// CurrentMS should be close to target (may not match exactly due to SeekToTimestamp finding nearest)
	if session.CurrentMS() < targetMS-100 || session.CurrentMS() > targetMS+100 {
		t.Errorf("CurrentMS = %d, want close to %d", session.CurrentMS(), targetMS)
	}
}

// TestSeekClampsToSessionRange verifies seeking is clamped to session bounds.
func TestSeekClampsToSessionRange(t *testing.T) {
	tempDir := t.TempDir()
	bufferPath := filepath.Join(tempDir, "test.bin")
	buffer, err := recording.NewBuffer(bufferPath, 1, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewBuffer: %v", err)
	}
	defer buffer.Close()

	broadcaster := &mockBroadcaster{}
	engine := NewEngine(buffer, broadcaster)

	session, err := engine.StartSession(1000, 5000)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	// Seek before session start
	err = engine.Seek(session.ID(), 500)
	if err != nil {
		t.Fatalf("Seek before start: %v", err)
	}
	if session.CurrentMS() != 1000 {
		t.Errorf("CurrentMS = %d, want 1000 (clamped to FromMS)", session.CurrentMS())
	}

	// Seek after session end
	err = engine.Seek(session.ID(), 10000)
	if err != nil {
		t.Fatalf("Seek after end: %v", err)
	}
	if session.CurrentMS() != 5000 {
		t.Errorf("CurrentMS = %d, want 5000 (clamped to ToMS)", session.CurrentMS())
	}
}

// TestPlay verifies playback starts.
func TestPlay(t *testing.T) {
	tempDir := t.TempDir()
	bufferPath := filepath.Join(tempDir, "test.bin")
	buffer, err := recording.NewBuffer(bufferPath, 1, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewBuffer: %v", err)
	}
	defer buffer.Close()

	// Write test data
	baseTime := time.Unix(1_000_000, 0).UnixNano()
	frame := make([]byte, 152)
	for i := 0; i < 10; i++ {
		ts := baseTime + int64(i)*int64(50*time.Millisecond)
		if err := buffer.Append(ts, frame); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	broadcaster := &mockBroadcaster{}
	engine := NewEngine(buffer, broadcaster)

	session, err := engine.StartSession(
		time.Unix(1_000_000, 0).UnixMilli(),
		time.Unix(1_000_060, 0).UnixMilli(), // 60-second range to sustain playback
	)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	// Start playback
	err = engine.Play(session.ID(), 2.0)
	if err != nil {
		t.Fatalf("Play: %v", err)
	}

	// Give the playback worker time to start
	time.Sleep(100 * time.Millisecond)

	if session.State() != StatePlaying {
		t.Errorf("State = %v, want StatePlaying", session.State())
	}
	if session.Speed() != 2 {
		t.Errorf("Speed = %d, want 2", session.Speed())
	}

	// Pause to stop the worker
	err = engine.Pause(session.ID())
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
}

// TestPause verifies pausing playback.
func TestPause(t *testing.T) {
	tempDir := t.TempDir()
	bufferPath := filepath.Join(tempDir, "test.bin")
	buffer, err := recording.NewBuffer(bufferPath, 1, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewBuffer: %v", err)
	}
	defer buffer.Close()

	broadcaster := &mockBroadcaster{}
	engine := NewEngine(buffer, broadcaster)

	session, err := engine.StartSession(0, 1000)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	// Pause when already paused should be a no-op
	err = engine.Pause(session.ID())
	if err != nil {
		t.Fatalf("Pause (already paused): %v", err)
	}
	if session.State() != StatePaused {
		t.Errorf("State = %v, want StatePaused", session.State())
	}
}

// TestSetSpeed verifies speed change.
func TestSetSpeed(t *testing.T) {
	tempDir := t.TempDir()
	bufferPath := filepath.Join(tempDir, "test.bin")
	buffer, err := recording.NewBuffer(bufferPath, 1, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewBuffer: %v", err)
	}
	defer buffer.Close()

	broadcaster := &mockBroadcaster{}
	engine := NewEngine(buffer, broadcaster)

	session, err := engine.StartSession(0, 1000)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	// Set speed while paused
	err = engine.SetSpeed(session.ID(), 5.0)
	if err != nil {
		t.Fatalf("SetSpeed: %v", err)
	}
	if session.Speed() != 5 {
		t.Errorf("Speed = %d, want 5", session.Speed())
	}
}

// TestSetParams verifies parameter updates.
func TestSetParams(t *testing.T) {
	tempDir := t.TempDir()
	bufferPath := filepath.Join(tempDir, "test.bin")
	buffer, err := recording.NewBuffer(bufferPath, 1, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewBuffer: %v", err)
	}
	defer buffer.Close()

	broadcaster := &mockBroadcaster{}
	engine := NewEngine(buffer, broadcaster)

	session, err := engine.StartSession(0, 1000)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	// Update deltaRMS threshold
	newThreshold := 0.05
	params := &TunableParams{
		DeltaRMSThreshold: &newThreshold,
	}

	err = engine.SetParams(session.ID(), params)
	if err != nil {
		t.Fatalf("SetParams: %v", err)
	}

	if session.Params().DeltaRMSThreshold == nil {
		t.Error("DeltaRMSThreshold not set")
	} else if *session.Params().DeltaRMSThreshold != newThreshold {
		t.Errorf("DeltaRMSThreshold = %f, want %f", *session.Params().DeltaRMSThreshold, newThreshold)
	}

	// Verify other defaults are preserved
	if session.Params().TauS == nil {
		t.Error("TauS not preserved")
	} else if *session.Params().TauS != 30.0 {
		t.Errorf("TauS = %f, want 30.0", *session.Params().TauS)
	}
}

// TestGetTimestampRange verifies getting the available timestamp range.
func TestGetTimestampRange(t *testing.T) {
	tempDir := t.TempDir()
	bufferPath := filepath.Join(tempDir, "test.bin")
	buffer, err := recording.NewBuffer(bufferPath, 1, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewBuffer: %v", err)
	}
	defer buffer.Close()

	broadcaster := &mockBroadcaster{}
	engine := NewEngine(buffer, broadcaster)

	// Empty buffer should return error
	_, _, err = engine.GetTimestampRange()
	if err == nil {
		t.Error("Expected error for empty buffer")
	}

	// Add some data
	now := time.Now().UnixNano()
	frame := make([]byte, 152)
	if err := buffer.Append(now, frame); err != nil {
		t.Fatalf("Append: %v", err)
	}

	oldest, newest, err := engine.GetTimestampRange()
	if err != nil {
		t.Fatalf("GetTimestampRange: %v", err)
	}

	if oldest.IsZero() {
		t.Error("oldest is zero")
	}
	if newest.IsZero() {
		t.Error("newest is zero")
	}
}
