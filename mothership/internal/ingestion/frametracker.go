package ingestion

import (
	"sync"
	"time"
)

// FrameTracker tracks frame rates per link for ambient traffic measurement
// Part of ADR-003: measuring whether beacon-only traffic clears detection thresholds
type FrameTracker struct {
	mu            sync.RWMutex
	linkStats     map[string]*linkFrameStats
	cleanupTicker  *time.Ticker
	done          chan struct{}
}

// linkFrameStats tracks frame statistics for a single link
type linkFrameStats struct {
	frameTimes  []time.Time // Ring buffer of recent frame timestamps (last 60 seconds)
	frameCount  int          // Total frames received
	windowStart  time.Time    // Start of current measurement window
	windowCount  int          // Frames in current window
}

const (
	statsWindowDuration = 60 * time.Second // Statistics window
	maxFrameTimes       = 600                // Keep up to 600 timestamps (10 Hz for 60s)
)

// NewFrameTracker creates a new frame tracker
func NewFrameTracker() *FrameTracker {
	ft := &FrameTracker{
		linkStats: make(map[string]*linkFrameStats),
		done:      make(chan struct{}),
	}

	// Start cleanup goroutine
	ft.cleanupTicker = time.NewTicker(60 * time.Second)
	go ft.cleanup()

	return ft
}

// cleanup removes stale link statistics
func (ft *FrameTracker) cleanup() {
	for {
		select {
		case <-ft.cleanupTicker.C:
			ft.mu.Lock()
			now := time.Now()
			for linkID, stats := range ft.linkStats {
				// Remove stats if no frames in last 5 minutes
				if stats.frameCount > 0 && now.Sub(stats.frameTimes[len(stats.frameTimes)-1]) > 5*time.Minute {
					delete(ft.linkStats, linkID)
				}
			}
			ft.mu.Unlock()
		case <-ft.done:
			return
		}
	}
}

// Stop stops the frame tracker
func (ft *FrameTracker) Stop() {
	close(ft.done)
	ft.cleanupTicker.Stop()
}

// RecordFrame records a frame arrival for a link
func (ft *FrameTracker) RecordFrame(linkID string) {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	stats, exists := ft.linkStats[linkID]
	if !exists {
		stats = &linkFrameStats{
			frameTimes: make([]time.Time, 0, maxFrameTimes),
			windowStart: time.Now(),
		}
		ft.linkStats[linkID] = stats
	}

	now := time.Now()
	stats.frameCount++
	stats.windowCount++

	// Add to frame times ring buffer
	if len(stats.frameTimes) < cap(stats.frameTimes) {
		stats.frameTimes = append(stats.frameTimes, now)
	} else {
		// Shift left and append
		copy(stats.frameTimes, stats.frameTimes[1:])
		stats.frameTimes[len(stats.frameTimes)-1] = now
	}

	// Reset window if duration exceeded
	if now.Sub(stats.windowStart) >= statsWindowDuration {
		stats.windowStart = now
		stats.windowCount = 0
	}
}

// GetFrameRate returns the frame rate (frames per second) for a link over the measurement window
func (ft *FrameTracker) GetFrameRate(linkID string) float64 {
	ft.mu.RLock()
	defer ft.mu.RUnlock()

	stats, exists := ft.linkStats[linkID]
	if !exists || len(stats.frameTimes) == 0 {
		return 0
	}

	// Calculate rate from window
	duration := time.Since(stats.windowStart)
	if duration <= 0 {
		return 0
	}

	// If we have enough history, use the frame times
	if len(stats.frameTimes) >= 2 {
		first := stats.frameTimes[0]
		last := stats.frameTimes[len(stats.frameTimes)-1]
		measuredDuration := last.Sub(first)
		if measuredDuration > 0 {
			return float64(len(stats.frameTimes)) / measuredDuration.Seconds()
		}
	}

	// Fallback to window count
	return float64(stats.windowCount) / duration.Seconds()
}

// GetFrameCount returns the total frame count for a link
func (ft *FrameTracker) GetFrameCount(linkID string) int {
	ft.mu.RLock()
	defer ft.mu.RUnlock()

	stats, exists := ft.linkStats[linkID]
	if !exists {
		return 0
	}
	return stats.frameCount
}

// GetAllStats returns statistics for all links
func (ft *FrameTracker) GetAllStats() map[string]LinkFrameStats {
	ft.mu.RLock()
	defer ft.mu.RUnlock()

	result := make(map[string]LinkFrameStats)
	for linkID, stats := range ft.linkStats {
		lastFrame := time.Now()
		if len(stats.frameTimes) > 0 {
			lastFrame = stats.frameTimes[len(stats.frameTimes)-1]
		}
		result[linkID] = LinkFrameStats{
			FrameRate:  ft.GetFrameRate(linkID),
			FrameCount: stats.frameCount,
			LastFrame:  lastFrame,
		}
	}
	return result
}

// LinkFrameStats represents frame statistics for a single link
type LinkFrameStats struct {
	FrameRate  float64   `json:"frame_rate"`
	FrameCount int       `json:"frame_count"`
	LastFrame  time.Time `json:"last_frame"`
}
