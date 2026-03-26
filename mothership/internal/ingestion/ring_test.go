package ingestion

import (
	"testing"
	"time"
)

func makeTestFrame(seq int) *CSIFrame {
	return &CSIFrame{
		NodeMAC:     [6]byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, byte(seq)},
		PeerMAC:     [6]byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66},
		TimestampUS: uint64(seq * 50000), // 50ms intervals
		RSSI:        -50,
		Channel:     6,
		NSub:        2,
		Payload:     []int8{int8(seq), int8(seq + 1), int8(seq + 2), int8(seq + 3)},
	}
}

func TestRingBuffer_PushAndGetAll(t *testing.T) {
	rb := NewRingBuffer()

	// Push 5 frames
	for i := 0; i < 5; i++ {
		rb.Push(makeTestFrame(i), time.Now())
	}

	if rb.Count() != 5 {
		t.Errorf("Count mismatch: got %d, want 5", rb.Count())
	}

	frames := rb.GetAll()
	if len(frames) != 5 {
		t.Fatalf("GetAll returned %d frames, want 5", len(frames))
	}

	// Verify chronological order (oldest first)
	for i, tf := range frames {
		if tf.Frame.TimestampUS != uint64(i*50000) {
			t.Errorf("Frame %d has wrong timestamp: %d", i, tf.Frame.TimestampUS)
		}
	}
}

func TestRingBuffer_Wraparound(t *testing.T) {
	rb := NewRingBuffer()

	// Push more frames than capacity
	for i := 0; i < RingBufferCapacity+50; i++ {
		rb.Push(makeTestFrame(i), time.Now())
	}

	if rb.Count() != RingBufferCapacity {
		t.Errorf("Count after wrap: got %d, want %d", rb.Count(), RingBufferCapacity)
	}

	frames := rb.GetAll()
	if len(frames) != RingBufferCapacity {
		t.Fatalf("GetAll returned %d frames, want %d", len(frames), RingBufferCapacity)
	}

	// First frame should be frame 50 (oldest after wrap)
	if frames[0].Frame.TimestampUS != 50*50000 {
		t.Errorf("First frame timestamp: got %d, want %d", frames[0].Frame.TimestampUS, 50*50000)
	}

	// Last frame should be frame 299 (newest)
	lastIdx := len(frames) - 1
	if frames[lastIdx].Frame.TimestampUS != (50+RingBufferCapacity-1)*50000 {
		t.Errorf("Last frame timestamp: got %d, want %d",
			frames[lastIdx].Frame.TimestampUS, (50+RingBufferCapacity-1)*50000)
	}
}

func TestRingBuffer_GetLast(t *testing.T) {
	rb := NewRingBuffer()

	// Push 10 frames
	for i := 0; i < 10; i++ {
		rb.Push(makeTestFrame(i), time.Now())
	}

	// Get last 3
	frames := rb.GetLast(3)
	if len(frames) != 3 {
		t.Fatalf("GetLast(3) returned %d frames", len(frames))
	}

	// Should be frames 7, 8, 9 in chronological order
	for i, tf := range frames {
		expectedSeq := 7 + i
		if tf.Frame.TimestampUS != uint64(expectedSeq*50000) {
			t.Errorf("Frame %d: expected timestamp %d, got %d",
				i, expectedSeq*50000, tf.Frame.TimestampUS)
		}
	}
}

func TestRingBuffer_GetLastMoreThanCount(t *testing.T) {
	rb := NewRingBuffer()

	// Push 5 frames
	for i := 0; i < 5; i++ {
		rb.Push(makeTestFrame(i), time.Now())
	}

	// Request 10, should get 5
	frames := rb.GetLast(10)
	if len(frames) != 5 {
		t.Errorf("GetLast(10) returned %d frames, want 5", len(frames))
	}
}

func TestRingBuffer_Empty(t *testing.T) {
	rb := NewRingBuffer()

	if rb.Count() != 0 {
		t.Errorf("Empty buffer count: got %d, want 0", rb.Count())
	}

	frames := rb.GetAll()
	if frames != nil {
		t.Errorf("GetAll on empty buffer: got %v, want nil", frames)
	}

	frames = rb.GetLast(5)
	if frames != nil {
		t.Errorf("GetLast on empty buffer: got %v, want nil", frames)
	}
}

func TestRingBuffer_Clear(t *testing.T) {
	rb := NewRingBuffer()

	for i := 0; i < 10; i++ {
		rb.Push(makeTestFrame(i), time.Now())
	}

	rb.Clear()

	if rb.Count() != 0 {
		t.Errorf("After clear, count: got %d, want 0", rb.Count())
	}

	frames := rb.GetAll()
	if frames != nil {
		t.Errorf("After clear, GetAll: got %v, want nil", frames)
	}
}

func TestRingBuffer_TotalSeq(t *testing.T) {
	rb := NewRingBuffer()

	for i := 0; i < 10; i++ {
		rb.Push(makeTestFrame(i), time.Now())
	}

	if rb.TotalSeq() != 10 {
		t.Errorf("TotalSeq: got %d, want 10", rb.TotalSeq())
	}

	rb.Clear()

	// TotalSeq should also reset on clear
	if rb.TotalSeq() != 0 {
		t.Errorf("After clear, TotalSeq: got %d, want 0", rb.TotalSeq())
	}
}

func TestRingBuffer_RecvTime(t *testing.T) {
	rb := NewRingBuffer()

	now := time.Now()
	rb.Push(makeTestFrame(0), now)

	frames := rb.GetAll()
	if len(frames) != 1 {
		t.Fatal("Expected 1 frame")
	}

	if !frames[0].RecvTime.Equal(now) {
		t.Errorf("RecvTime mismatch: got %v, want %v", frames[0].RecvTime, now)
	}

	if frames[0].RecvTimeNS != now.UnixNano() {
		t.Errorf("RecvTimeNS mismatch: got %d, want %d", frames[0].RecvTimeNS, now.UnixNano())
	}
}
