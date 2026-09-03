// Package replay provides fuzz tests for timestamp seeking functionality.
package replay

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/spaxel/mothership/internal/recording"
)

// FuzzSessionSeekTo fuzzes Session.SeekTo() with arbitrary int64 timestamps.
// Key properties:
//  1. SeekTo must NEVER panic on any int64 target.
//  2. After a seek, if a frame is retrieved from the recording buffer, its timestamp
//     must be >= the (clamped) target timestamp.
//  3. Seeking before the session range should clamp to FromMS.
//  4. Seeking after the session range should clamp to ToMS.
//  5. Edge cases: math.MinInt64, math.MaxInt64, 0, -1, negative values.
func FuzzSessionSeekTo(f *testing.F) {
	// Seed corpus covering edge cases and typical scenarios
	f.Add(int64(0))                    // zero timestamp
	f.Add(int64(1))                    // one millisecond
	f.Add(int64(-1))                   // negative timestamp
	f.Add(int64(1000))                 // one second
	f.Add(int64(60000))                // one minute
	f.Add(int64(math.MaxInt64))        // maximum int64
	f.Add(int64(math.MinInt64))        // minimum int64
	f.Add(int64(9223372036854775807))  // MaxInt64 - 1
	f.Add(int64(-9223372036854775808)) // MinInt64 + 1
	f.Add(int64(1234567890123))        // arbitrary large value
	f.Add(int64(-1234567890123))       // arbitrary negative value

	f.Fuzz(func(t *testing.T, targetMS int64) {
		// Create a temporary recording buffer
		tempDir := t.TempDir()
		bufferPath := filepath.Join(tempDir, "test.bin")
		buffer, err := recording.NewBuffer(bufferPath, 1, 24*time.Hour, false, 0)
		if err != nil {
			t.Skipf("Failed to create buffer: %v", err)
		}
		defer buffer.Close()

		// Write test data with known timestamps
		now := time.Now().UnixNano()
		frame := make([]byte, 152)
		timestamps := make([]int64, 5)
		for i := 0; i < 5; i++ {
			timestamps[i] = now + int64(i)*int64(time.Second)
			if err := buffer.Append(timestamps[i], frame); err != nil {
				t.Skipf("Failed to append frame %d: %v", i, err)
			}
		}

		// Create a session with a known range
		fromMS := time.Unix(0, timestamps[0]).UnixMilli()
		toMS := time.Unix(0, timestamps[4]).UnixMilli()
		session := NewSession("test-session", fromMS, toMS)

		// Property 1: SeekTo must NEVER panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("SeekTo panicked with targetMS=%d: %v", targetMS, r)
			}
		}()

		// Perform the seek
		err = session.SeekTo(targetMS)

		// SeekTo should always succeed (it clamps to range)
		if err != nil {
			t.Errorf("SeekTo(%d) returned unexpected error: %v", targetMS, err)
		}

		// Verify clamping behavior
		currentMS := session.CurrentMS()

		// Verify bounds are respected
		if currentMS < fromMS {
			t.Errorf("CurrentMS=%d < FromMS=%d (should clamp to lower bound)", currentMS, fromMS)
		}
		if currentMS > toMS {
			t.Errorf("CurrentMS=%d > ToMS=%d (should clamp to upper bound)", currentMS, toMS)
		}

		// Property 2: If we can retrieve a frame near the seek position,
		// its timestamp should be >= the clamped target (when converted to ns)
		// Convert currentMS (milliseconds) to nanoseconds for comparison
		clampedTargetNS := currentMS * 1_000_000

		// Try to read a frame near the seeked position
		targetTime := time.Unix(0, clampedTargetNS)
		frameData, frameTimestampNS, err := buffer.SeekToTimestamp(targetTime)

		if err == nil && frameData != nil {
			// If we successfully retrieved a frame, verify the timestamp property
			// The returned frame should have a timestamp >= the clamped target
			if frameTimestampNS < clampedTargetNS {
				t.Errorf("Frame timestamp=%d < clamped target=%d (timestamp should be >= target)",
					frameTimestampNS, clampedTargetNS)
			}
		}
	})
}

// FuzzEngineSeek fuzzes Engine.Seek() with arbitrary int64 timestamps.
// Key properties:
// 1. Engine.Seek must NEVER panic on any int64 target.
// 2. Invalid session IDs should be handled gracefully (error, not panic).
// 3. Valid seeks should clamp to session range as with Session.SeekTo.
func FuzzEngineSeek(f *testing.F) {
	// Seed corpus with session ID and timestamp combinations
	f.Add("valid-session", int64(0))
	f.Add("valid-session", int64(1000))
	f.Add("valid-session", int64(-1000))
	f.Add("valid-session", int64(math.MaxInt64))
	f.Add("valid-session", int64(math.MinInt64))
	f.Add("", int64(12345))                            // empty session ID
	f.Add("nonexistent-session", int64(5000))          // unknown session ID
	f.Add("session-with-special-chars", int64(999999)) // session ID with special chars (if allowed)

	f.Fuzz(func(t *testing.T, sessionID string, targetMS int64) {
		// Create a temporary recording buffer
		tempDir := t.TempDir()
		bufferPath := filepath.Join(tempDir, "test.bin")
		buffer, err := recording.NewBuffer(bufferPath, 1, 24*time.Hour, false, 0)
		if err != nil {
			t.Skipf("Failed to create buffer: %v", err)
		}
		defer buffer.Close()

		// Write minimal test data
		now := time.Now().UnixNano()
		frame := make([]byte, 152)
		if err := buffer.Append(now, frame); err != nil {
			t.Skipf("Failed to append frame: %v", err)
		}

		// Create engine and a valid session
		broadcaster := &mockBroadcaster{}
		engine := NewEngine(buffer, broadcaster)

		fromMS := time.Unix(0, now).UnixMilli()
		toMS := fromMS + 60000 // 60 second range
		session, err := engine.StartSession(fromMS, toMS)
		if err != nil {
			t.Skipf("Failed to start session: %v", err)
		}
		validSessionID := session.ID()

		// Use the provided session ID, or a valid one if empty
		seekSessionID := sessionID
		if seekSessionID == "" {
			seekSessionID = validSessionID
		}

		// Property 1: Engine.Seek must NEVER panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Engine.Seek panicked with sessionID=%q, targetMS=%d: %v",
					seekSessionID, targetMS, r)
			}
		}()

		// Perform the seek
		err = engine.Seek(seekSessionID, targetMS)

		// Property 2: Invalid session IDs should return error, not panic
		if seekSessionID != validSessionID {
			if err == nil {
				t.Errorf("Engine.Seek(%q, %d) should error for invalid session ID",
					seekSessionID, targetMS)
			}
			return // No further checks needed for invalid session
		}

		// For valid session, seek should succeed (with clamping)
		if err != nil {
			t.Errorf("Engine.Seek(%q, %d) returned unexpected error: %v",
				seekSessionID, targetMS, err)
		}

		// Verify the session state
		sess, ok := engine.GetSession(seekSessionID)
		if !ok {
			t.Errorf("Session not found after successful seek")
			return
		}

		currentMS := sess.CurrentMS()

		// Verify clamping to session range
		if currentMS < fromMS {
			t.Errorf("CurrentMS=%d < FromMS=%d (should clamp)", currentMS, fromMS)
		}
		if currentMS > toMS {
			t.Errorf("CurrentMS=%d > ToMS=%d (should clamp)", currentMS, toMS)
		}
	})
}

// TestSessionSeekToEdgeCases tests specific edge cases for Session.SeekTo.
func TestSessionSeekToEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		fromMS      int64
		toMS        int64
		targetMS    int64
		wantClamped int64
		description string
	}{
		{
			name:        "seek to zero",
			fromMS:      1000,
			toMS:        10000,
			targetMS:    0,
			wantClamped: 1000, // Should clamp to FromMS
			description: "Target before session start should clamp to FromMS",
		},
		{
			name:        "seek within range",
			fromMS:      1000,
			toMS:        10000,
			targetMS:    5000,
			wantClamped: 5000, // Should not clamp
			description: "Target within range should be unchanged",
		},
		{
			name:        "seek after range",
			fromMS:      1000,
			toMS:        10000,
			targetMS:    20000,
			wantClamped: 10000, // Should clamp to ToMS
			description: "Target after session end should clamp to ToMS",
		},
		{
			name:        "seek to negative",
			fromMS:      1000,
			toMS:        10000,
			targetMS:    -5000,
			wantClamped: 1000, // Should clamp to FromMS
			description: "Negative target should clamp to FromMS",
		},
		{
			name:        "seek to max int64",
			fromMS:      1000,
			toMS:        10000,
			targetMS:    math.MaxInt64,
			wantClamped: 10000, // Should clamp to ToMS
			description: "MaxInt64 target should clamp to ToMS",
		},
		{
			name:        "seek to min int64",
			fromMS:      1000,
			toMS:        10000,
			targetMS:    math.MinInt64,
			wantClamped: 1000, // Should clamp to FromMS
			description: "MinInt64 target should clamp to FromMS",
		},
		{
			name:        "seek at fromMS boundary",
			fromMS:      1000,
			toMS:        10000,
			targetMS:    1000,
			wantClamped: 1000, // Exact match to FromMS
			description: "Seek to exact FromMS should succeed",
		},
		{
			name:        "seek at toMS boundary",
			fromMS:      1000,
			toMS:        10000,
			targetMS:    10000,
			wantClamped: 10000, // Exact match to ToMS
			description: "Seek to exact ToMS should succeed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := NewSession("test", tt.fromMS, tt.toMS)

			err := session.SeekTo(tt.targetMS)
			if err != nil {
				t.Fatalf("SeekTo(%d) failed: %v", tt.targetMS, err)
			}

			currentMS := session.CurrentMS()
			if currentMS != tt.wantClamped {
				t.Errorf("SeekTo(%d): CurrentMS=%d, want %d. %s",
					tt.targetMS, currentMS, tt.wantClamped, tt.description)
			}
		})
	}
}

// TestEngineSeekEdgeCases tests specific edge cases for Engine.Seek.
func TestEngineSeekEdgeCases(t *testing.T) {
	tempDir := t.TempDir()
	bufferPath := filepath.Join(tempDir, "test.bin")
	buffer, err := recording.NewBuffer(bufferPath, 1, 24*time.Hour, false, 0)
	if err != nil {
		t.Fatalf("Failed to create buffer: %v", err)
	}
	defer buffer.Close()

	broadcaster := &mockBroadcaster{}
	engine := NewEngine(buffer, broadcaster)

	session, err := engine.StartSession(1000, 10000)
	if err != nil {
		t.Fatalf("Failed to start session: %v", err)
	}

	tests := []struct {
		name        string
		targetMS    int64
		wantError   bool
		description string
	}{
		{
			name:        "seek before range",
			targetMS:    500,
			wantError:   false,
			description: "Should clamp to FromMS",
		},
		{
			name:        "seek within range",
			targetMS:    5000,
			wantError:   false,
			description: "Should succeed unchanged",
		},
		{
			name:        "seek after range",
			targetMS:    15000,
			wantError:   false,
			description: "Should clamp to ToMS",
		},
		{
			name:        "seek to max int64",
			targetMS:    math.MaxInt64,
			wantError:   false,
			description: "Should clamp to ToMS",
		},
		{
			name:        "seek to min int64",
			targetMS:    math.MinInt64,
			wantError:   false,
			description: "Should clamp to FromMS",
		},
		{
			name:        "seek to negative",
			targetMS:    -1000,
			wantError:   false,
			description: "Should clamp to FromMS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := engine.Seek(session.ID(), tt.targetMS)
			if (err != nil) != tt.wantError {
				t.Errorf("Seek(%d) error = %v, wantError %v. %s",
					tt.targetMS, err, tt.wantError, tt.description)
			}
			if !tt.wantError {
				// Verify clamping
				sess, _ := engine.GetSession(session.ID())
				currentMS := sess.CurrentMS()
				if currentMS < 1000 {
					t.Errorf("CurrentMS=%d < FromMS (should clamp)", currentMS)
				}
				if currentMS > 10000 {
					t.Errorf("CurrentMS=%d > ToMS (should clamp)", currentMS)
				}
			}
		})
	}
}

// TestEngineSeekInvalidSession tests seeking with invalid session IDs.
func TestEngineSeekInvalidSession(t *testing.T) {
	tempDir := t.TempDir()
	bufferPath := filepath.Join(tempDir, "test.bin")
	buffer, err := recording.NewBuffer(bufferPath, 1, 24*time.Hour, false, 0)
	if err != nil {
		t.Fatalf("Failed to create buffer: %v", err)
	}
	defer buffer.Close()

	broadcaster := &mockBroadcaster{}
	engine := NewEngine(buffer, broadcaster)

	invalidIDs := []string{
		"",
		"nonexistent",
		"---invalid---",
		"12345",
	}

	for _, id := range invalidIDs {
		t.Run("invalid ID: "+id, func(t *testing.T) {
			err := engine.Seek(id, 5000)
			if err == nil {
				t.Errorf("Seek(%q, 5000) should error for invalid session ID", id)
			}
		})
	}
}

// FuzzSeek is a comprehensive fuzz test for both Engine.Seek and Session.SeekTo.
// It covers all int64 timestamp edge cases including:
// - Before session range (should clamp to FromMS)
// - After session range (should clamp to ToMS)
// - Exactly at fromMS/toMS boundaries
// - Wrap points: math.MinInt64, math.MaxInt64, 0, -1
// - Negative values
//
// Properties verified:
// 1. Never panics on any int64 target (for both Seek and SeekTo)
// 2. If a frame is returned after seek, Frame.RecvTimeNS >= (clamped target * 1e6)
func FuzzSeek(f *testing.F) {
	// Seed corpus covering comprehensive edge cases
	// Basic range tests
	f.Add(int64(0))     // zero
	f.Add(int64(1))     // one millisecond
	f.Add(int64(-1))    // negative
	f.Add(int64(1000))  // one second in ms
	f.Add(int64(60000)) // one minute in ms

	// Boundary cases at extreme values
	f.Add(int64(math.MaxInt64))     // maximum int64
	f.Add(int64(math.MinInt64))     // minimum int64
	f.Add(int64(math.MaxInt64 - 1)) // MaxInt64 - 1
	f.Add(int64(math.MinInt64 + 1)) // MinInt64 + 1

	// Boundary values around typical session ranges
	f.Add(int64(499))   // just before a 500-10000 range
	f.Add(int64(500))   // at FromMS boundary
	f.Add(int64(10000)) // at ToMS boundary
	f.Add(int64(10001)) // just after range

	// Arbitrary large values
	f.Add(int64(1234567890123))  // large positive
	f.Add(int64(-1234567890123)) // large negative

	// Additional edge cases for comprehensive coverage
	f.Add(int64(2))                    // small positive
	f.Add(int64(-2))                   // small negative
	f.Add(int64(9223372036854775806))  // MaxInt64 - 1 (explicit duplicate for emphasis)
	f.Add(int64(-9223372036854775807)) // MinInt64 + 1 (explicit duplicate for emphasis)
	f.Add(int64(999))                  // another in-range value
	f.Add(int64(1001))                 // just past boundary

	f.Fuzz(func(t *testing.T, targetMS int64) {
		// Setup: Create a temporary recording buffer with test data
		tempDir := t.TempDir()
		bufferPath := filepath.Join(tempDir, "test.bin")
		buffer, err := recording.NewBuffer(bufferPath, 1, 24*time.Hour, false, 0)
		if err != nil {
			t.Skipf("Failed to create buffer: %v", err)
		}
		defer buffer.Close()

		// Write test frames with known timestamps
		now := time.Now().UnixNano()
		frame := make([]byte, 152)
		timestamps := make([]int64, 5)
		for i := 0; i < 5; i++ {
			timestamps[i] = now + int64(i)*int64(time.Second)
			if err := buffer.Append(timestamps[i], frame); err != nil {
				t.Skipf("Failed to append frame %d: %v", i, err)
			}
		}

		// Define session range (using the written timestamps)
		fromMS := time.Unix(0, timestamps[0]).UnixMilli()
		toMS := time.Unix(0, timestamps[4]).UnixMilli()

		// ===== Test Session.SeekTo =====
		t.Run("Session.SeekTo", func(t *testing.T) {
			session := NewSession("fuzz-session", fromMS, toMS)

			// Property 1: SeekTo must NEVER panic on any int64 target
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Session.SeekTo panicked with targetMS=%d: %v", targetMS, r)
				}
			}()

			// Perform the seek
			err := session.SeekTo(targetMS)

			// SeekTo should always succeed (it clamps to range)
			if err != nil {
				t.Errorf("SeekTo(%d) returned unexpected error: %v", targetMS, err)
			}

			// Get the clamped position after seek
			clampedMS := session.CurrentMS()

			// Verify clamping respects session bounds
			if clampedMS < fromMS {
				t.Errorf("After SeekTo(%d): CurrentMS=%d < FromMS=%d (should clamp to lower bound)",
					targetMS, clampedMS, fromMS)
			}
			if clampedMS > toMS {
				t.Errorf("After SeekTo(%d): CurrentMS=%d > ToMS=%d (should clamp to upper bound)",
					targetMS, clampedMS, toMS)
			}

			// Property 2: If we retrieve a frame after seek, its timestamp >= clamped target
			// Convert clampedMS (milliseconds) to nanoseconds for comparison
			clampedTargetNS := clampedMS * 1_000_000

			// Try to read a frame at the seeked position
			targetTime := time.Unix(0, clampedTargetNS)
			frameData, frameRecvTimeNS, err := buffer.SeekToTimestamp(targetTime)

			if err == nil && frameData != nil {
				// Successfully retrieved a frame - verify timestamp property
				if frameRecvTimeNS < clampedTargetNS {
					t.Errorf("Frame RecvTimeNS=%d < clamped target=%d (should be >= target)",
						frameRecvTimeNS, clampedTargetNS)
				}
			}
		})

		// ===== Test Engine.Seek =====
		t.Run("Engine.Seek", func(t *testing.T) {
			broadcaster := &mockBroadcaster{}
			engine := NewEngine(buffer, broadcaster)

			// Start a session with the same range
			session, err := engine.StartSession(fromMS, toMS)
			if err != nil {
				t.Skipf("Failed to start session: %v", err)
			}
			sessionID := session.ID()

			// Property 1: Engine.Seek must NEVER panic on any int64 target
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Engine.Seek panicked with targetMS=%d: %v", targetMS, r)
				}
			}()

			// Perform the seek through Engine
			err = engine.Seek(sessionID, targetMS)

			// Engine.Seek should always succeed for valid session (it clamps to range)
			if err != nil {
				t.Errorf("Engine.Seek(%d) returned unexpected error: %v", targetMS, err)
			}

			// Get the session to verify clamping
			sess, ok := engine.GetSession(sessionID)
			if !ok {
				t.Errorf("Session not found after Engine.Seek")
				return
			}

			clampedMS := sess.CurrentMS()

			// Verify clamping respects session bounds
			if clampedMS < fromMS {
				t.Errorf("After Engine.Seek(%d): CurrentMS=%d < FromMS=%d (should clamp)",
					targetMS, clampedMS, fromMS)
			}
			if clampedMS > toMS {
				t.Errorf("After Engine.Seek(%d): CurrentMS=%d > ToMS=%d (should clamp)",
					targetMS, clampedMS, toMS)
			}

			// Property 2: Verify frame timestamp property
			clampedTargetNS := clampedMS * 1_000_000
			targetTime := time.Unix(0, clampedTargetNS)
			frameData, frameRecvTimeNS, err := buffer.SeekToTimestamp(targetTime)

			if err == nil && frameData != nil {
				if frameRecvTimeNS < clampedTargetNS {
					t.Errorf("After Engine.Seek: Frame RecvTimeNS=%d < clamped target=%d (should be >=)",
						frameRecvTimeNS, clampedTargetNS)
				}
			}
		})
	})
}
