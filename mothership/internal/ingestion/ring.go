package ingestion

import (
	"sync"
	"time"
)

// RingBufferCapacity is the maximum number of CSI samples per link
const RingBufferCapacity = 256

// TimestampedFrame wraps a CSIFrame with the mothership receive time
type TimestampedFrame struct {
	Frame      *CSIFrame
	RecvTime   time.Time
	RecvTimeNS int64 // Nanoseconds since Unix epoch for precise timing
}

// RingBuffer is a thread-safe circular buffer for CSI samples
type RingBuffer struct {
	mu       sync.RWMutex
	frames   [RingBufferCapacity]*TimestampedFrame
	head     int // Next write position
	count    int // Number of valid entries
	totalSeq int // Total frames ever written (for computing sequence numbers)
}

// NewRingBuffer creates a new ring buffer
func NewRingBuffer() *RingBuffer {
	return &RingBuffer{}
}

// Push adds a frame to the buffer
func (rb *RingBuffer) Push(frame *CSIFrame, recvTime time.Time) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.frames[rb.head] = &TimestampedFrame{
		Frame:      frame,
		RecvTime:   recvTime,
		RecvTimeNS: recvTime.UnixNano(),
	}
	rb.head = (rb.head + 1) % RingBufferCapacity
	if rb.count < RingBufferCapacity {
		rb.count++
	}
	rb.totalSeq++
}

// GetAll returns all frames in chronological order (oldest first)
func (rb *RingBuffer) GetAll() []*TimestampedFrame {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if rb.count == 0 {
		return nil
	}

	result := make([]*TimestampedFrame, rb.count)

	// If buffer is not full, start from beginning
	// If buffer is full, head points to oldest entry
	start := 0
	if rb.count == RingBufferCapacity {
		start = rb.head
	}

	for i := 0; i < rb.count; i++ {
		idx := (start + i) % RingBufferCapacity
		// Return copies to prevent race conditions
		f := rb.frames[idx]
		result[i] = &TimestampedFrame{
			Frame:      f.Frame,
			RecvTime:   f.RecvTime,
			RecvTimeNS: f.RecvTimeNS,
		}
	}

	return result
}

// GetLast returns the N most recent frames (or all if N > count)
func (rb *RingBuffer) GetLast(n int) []*TimestampedFrame {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if rb.count == 0 || n <= 0 {
		return nil
	}

	count := n
	if count > rb.count {
		count = rb.count
	}

	result := make([]*TimestampedFrame, count)

	// Most recent entry is at (head - 1 + Capacity) % Capacity
	for i := 0; i < count; i++ {
		idx := (rb.head - 1 - i + RingBufferCapacity) % RingBufferCapacity
		f := rb.frames[idx]
		result[count-1-i] = &TimestampedFrame{
			Frame:      f.Frame,
			RecvTime:   f.RecvTime,
			RecvTimeNS: f.RecvTimeNS,
		}
	}

	return result
}

// Count returns the number of frames in the buffer
func (rb *RingBuffer) Count() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.count
}

// Clear removes all frames from the buffer
func (rb *RingBuffer) Clear() {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.head = 0
	rb.count = 0
	rb.totalSeq = 0
	for i := range rb.frames {
		rb.frames[i] = nil
	}
}

// TotalSeq returns the total number of frames ever pushed
func (rb *RingBuffer) TotalSeq() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.totalSeq
}
