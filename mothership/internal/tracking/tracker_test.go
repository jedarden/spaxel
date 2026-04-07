package tracking

import (
	"sync"
	"testing"
	"time"
)

func TestTracker_BlobAppearCallback(t *testing.T) {
	tracker := NewTracker()

	var appeared []BlobEvent
	var mu sync.Mutex
	tracker.SetOnBlobAppear(func(ev BlobEvent) {
		mu.Lock()
		appeared = append(appeared, ev)
		mu.Unlock()
	})

	// First update with one measurement should trigger appear
	blobs := tracker.Update([][3]float64{{1.0, 2.0, 0.8}})
	if len(blobs) != 1 {
		t.Fatalf("expected 1 blob, got %d", len(blobs))
	}
	mu.Lock()
	if len(appeared) != 1 {
		t.Fatalf("expected 1 appear event, got %d", len(appeared))
	}
	ev := appeared[0]
	mu.Unlock()

	if ev.BlobID != 0 {
		t.Errorf("expected blob ID 0, got %d", ev.BlobID)
	}
	if ev.X != 1.0 || ev.Z != 2.0 {
		t.Errorf("expected position (1.0, 2.0), got (%.1f, %.1f)", ev.X, ev.Z)
	}
	if ev.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestTracker_BlobDisappearCallback(t *testing.T) {
	tracker := NewTracker()

	var appeared []BlobEvent
	var disappeared []BlobEvent
	var mu sync.Mutex
	tracker.SetOnBlobAppear(func(ev BlobEvent) {
		mu.Lock()
		appeared = append(appeared, ev)
		mu.Unlock()
	})
	tracker.SetOnBlobDisappear(func(ev BlobEvent) {
		mu.Lock()
		disappeared = append(disappeared, ev)
		mu.Unlock()
	})

	// Create a blob
	tracker.Update([][3]float64{{1.0, 2.0, 0.8}})
	mu.Lock()
	if len(appeared) != 1 {
		t.Fatalf("expected 1 appear, got %d", len(appeared))
	}
	mu.Unlock()

	// Update normally — no disappear should fire
	tracker.Update([][3]float64{{1.1, 2.1, 0.8}})
	mu.Lock()
	if len(disappeared) != 0 {
		t.Fatalf("expected 0 disappear, got %d", len(disappeared))
	}
	mu.Unlock()

	// Simulate staleness by advancing lastRun and not providing measurements
	tracker.mu.Lock()
	for _, b := range tracker.blobs {
		b.LastSeen = time.Now().Add(-6 * time.Second) // past staleTimeout
	}
	tracker.mu.Unlock()

	tracker.Update(nil)
	mu.Lock()
	if len(disappeared) != 1 {
		t.Fatalf("expected 1 disappear, got %d", len(disappeared))
	}
	ev := disappeared[0]
	mu.Unlock()

	if ev.BlobID != 0 {
		t.Errorf("expected blob ID 0, got %d", ev.BlobID)
	}
}

func TestTracker_NoCallbackSet(t *testing.T) {
	// Verify tracker works without callbacks (nil callbacks should not panic)
	tracker := NewTracker()

	// Should not panic
	blobs := tracker.Update([][3]float64{{1.0, 2.0, 0.8}})
	if len(blobs) != 1 {
		t.Fatalf("expected 1 blob, got %d", len(blobs))
	}

	// Simulate staleness
	tracker.mu.Lock()
	for _, b := range tracker.blobs {
		b.LastSeen = time.Now().Add(-6 * time.Second)
	}
	tracker.mu.Unlock()

	// Should not panic
	blobs = tracker.Update(nil)
	if len(blobs) != 0 {
		t.Fatalf("expected 0 blobs after staleness, got %d", len(blobs))
	}
}

func TestTracker_MultipleBlobs(t *testing.T) {
	tracker := NewTracker()

	var appeared, disappeared []BlobEvent
	var mu sync.Mutex
	tracker.SetOnBlobAppear(func(ev BlobEvent) {
		mu.Lock()
		appeared = append(appeared, ev)
		mu.Unlock()
	})
	tracker.SetOnBlobDisappear(func(ev BlobEvent) {
		mu.Lock()
		disappeared = append(disappeared, ev)
		mu.Unlock()
	})

	// Create two blobs
	tracker.Update([][3]float64{
		{1.0, 2.0, 0.8},
		{4.0, 5.0, 0.6},
	})
	mu.Lock()
	if len(appeared) != 2 {
		t.Fatalf("expected 2 appear events, got %d", len(appeared))
	}
	mu.Unlock()

	// Make one stale
	tracker.mu.Lock()
	tracker.blobs[0].LastSeen = time.Now().Add(-6 * time.Second)
	tracker.mu.Unlock()

	tracker.Update([][3]float64{{4.1, 5.1, 0.6}})

	mu.Lock()
	if len(disappeared) != 1 {
		t.Fatalf("expected 1 disappear, got %d", len(disappeared))
	}
	if disappeared[0].BlobID != 0 {
		t.Errorf("expected disappeared blob ID 0, got %d", disappeared[0].BlobID)
	}
	// Only one appear should fire for the remaining measurement (already associated)
	// No new appear events since the second blob was associated
	totalAppears := len(appeared)
	mu.Unlock()

	if totalAppears != 2 {
		t.Errorf("expected total 2 appear events, got %d", totalAppears)
	}
}

func TestTracker_CallbacksAfterReset(t *testing.T) {
	tracker := NewTracker()

	var appeared []BlobEvent
	var mu sync.Mutex
	tracker.SetOnBlobAppear(func(ev BlobEvent) {
		mu.Lock()
		appeared = append(appeared, ev)
		mu.Unlock()
	})

	// Create blob
	tracker.Update([][3]float64{{1.0, 2.0, 0.8}})
	mu.Lock()
	beforeCount := len(appeared)
	mu.Unlock()

	// Reset and create again
	tracker.Reset()
	tracker.Update([][3]float64{{3.0, 4.0, 0.7}})

	mu.Lock()
	if len(appeared) != beforeCount+1 {
		t.Fatalf("expected %d appear events total, got %d", beforeCount+1, len(appeared))
	}
	mu.Unlock()
}
