// Package replay provides time-travel debugging capabilities for CSI data.
//
// This file implements replay sessions that allow:
// - Seeking to specific timestamps
// - Playing at variable speeds (1x, 2x, 5x)
// - Pausing/resuming
// - Parameter tuning with instant preview
package replay

import (
	"fmt"
	"time"
)

// Helper functions for replay operations

// FormatTimestamp formats a timestamp for display.
func FormatTimestamp(ms int64) string {
	t := time.Unix(0, ms*int64(time.Millisecond))
	return t.Format("2006-01-02 15:04:05.000")
}

// DurationMS returns the duration between two timestamps in milliseconds.
func DurationMS(from, to int64) int64 {
	if to > from {
		return to - from
	}
	return from - to
}

// Progress calculates the playback progress (0.0 to 1.0).
func (s *Session) Progress() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.toMS <= s.fromMS {
		return 0.0
	}

	progress := float64(s.currentMS-s.fromMS) / float64(s.toMS-s.fromMS)
	if progress < 0.0 {
		return 0.0
	}
	if progress > 1.0 {
		return 1.0
	}
	return progress
}

// IsPlaying returns true if the session is currently playing.
func (s *Session) IsPlaying() bool {
	return s.State() == StatePlaying
}

// IsPaused returns true if the session is currently paused.
func (s *Session) IsPaused() bool {
	return s.State() == StatePaused
}

// IsStopped returns true if the session is stopped.
func (s *Session) IsStopped() bool {
	return s.State() == StateStopped
}

// ValidateRange checks if a time range is valid for replay.
func ValidateRange(fromMS, toMS int64) error {
	if fromMS < 0 || toMS < 0 {
		return fmt.Errorf("timestamps cannot be negative: from=%d, to=%d", fromMS, toMS)
	}
	if fromMS > toMS {
		return fmt.Errorf("from_ms (%d) cannot be greater than to_ms (%d)", fromMS, toMS)
	}
	return nil
}

// ClampTimestamp clamps a timestamp to the valid range.
func ClampTimestamp(ts, min, max int64) int64 {
	if ts < min {
		return min
	}
	if ts > max {
		return max
	}
	return ts
}

// LogReplayEvent logs a replay event.
func LogReplayEvent(event string, sessionID string, args ...interface{}) {
	// Use the standard logger - log package is already imported elsewhere
	// This is a simple wrapper for consistency
	fmt.Printf("[replay] session=%s %s\n", sessionID, fmt.Sprintf(event, args...))
}
