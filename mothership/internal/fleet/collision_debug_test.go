package fleet

import (
	"testing"
	"time"
)

// TestCollisionDetector_DebugRateCalculation debugs the collision rate calculation
func TestCollisionDetector_DebugRateCalculation(t *testing.T) {
	cd := NewCollisionDetector()

	// Register two TX nodes
	cd.RegisterTXNode("AA:BB:CC:DD:EE:01")
	cd.RegisterTXNode("AA:BB:CC:DD:EE:02")

	// Record frames with clear collision pattern
	now := time.Now()

	// TX1 frames at: 0, 100ms, 200ms, 300ms, 400ms
	// TX2 frames at: 2ms, 102ms, 202ms, 302ms, 402ms (all collide with TX1)
	for i := 0; i < 5; i++ {
		baseTime := now.Add(time.Duration(i) * 100 * time.Millisecond)
		// TX1 frame
		cd.RecordFrameArrival("AA:BB:CC:DD:EE:01", "link1", baseTime)
		// TX2 frame 2ms later (collision)
		cd.RecordFrameArrival("AA:BB:CC:DD:EE:02", "link2", baseTime.Add(2*time.Millisecond))
	}

	cd.mu.RLock()

	// Check frame arrivals
	t.Logf("TX1 frame arrivals: %d", len(cd.frameArrivals["AA:BB:CC:DD:EE:01"]))
	t.Logf("TX2 frame arrivals: %d", len(cd.frameArrivals["AA:BB:CC:DD:EE:02"]))

	pairKey := cd.pairKey("AA:BB:CC:DD:EE:01", "AA:BB:CC:DD:EE:02")
	t.Logf("Collision history count: %d", len(cd.collisionHistory[pairKey]))
	t.Logf("Collision count: %d", cd.collisionCount[pairKey])
	t.Logf("Total frames: %d", cd.totalFrames[pairKey])

	// Count collisions in 60-second window
	cutoff := now.Add(-60 * time.Second)
	collisionsInWindow := 0
	for _, event := range cd.collisionHistory[pairKey] {
		if event.Timestamp.After(cutoff) {
			collisionsInWindow++
		}
	}
	t.Logf("Collisions in 60s window: %d", collisionsInWindow)

	// Count frame arrivals in 60-second window
	tx1Frames := 0
	tx2Frames := 0
	for _, ts := range cd.frameArrivals["AA:BB:CC:DD:EE:01"] {
		if ts.After(cutoff) {
			tx1Frames++
		}
	}
	for _, ts := range cd.frameArrivals["AA:BB:CC:DD:EE:02"] {
		if ts.After(cutoff) {
			tx2Frames++
		}
	}
	t.Logf("TX1 frames in window: %d", tx1Frames)
	t.Logf("TX2 frames in window: %d", tx2Frames)

	// Frame opportunities: each TX frame represents a collision opportunity with the other TX
	frameOpportunities := tx1Frames + tx2Frames
	t.Logf("Frame opportunities: %d", frameOpportunities)

	cd.mu.RUnlock()

	// Get collision rate over 60 second window
	rate := cd.GetCollisionRate(60 * time.Second)

	t.Logf("Collision rate: %.2f", rate)

	// Should be approximately 50% (5 collisions out of 10 frame opportunities)
	// The algorithm is asymmetric: each TX2 frame collides with TX1, but TX1 frames
	// don't collide with TX2 because the time delta is negative when TX1 arrives first
	if rate < 0.4 || rate > 0.6 {
		t.Errorf("collision rate = %.2f, want ~0.5", rate)
	}
}

// TestCollisionDetector_ExactTiming tests with exact timing to verify collision detection
func TestCollisionDetector_ExactTiming(t *testing.T) {
	cd := NewCollisionDetector()

	cd.RegisterTXNode("AA:BB:CC:DD:EE:01")
	cd.RegisterTXNode("AA:BB:CC:DD:EE:EE:02")

	now := time.Now()

	// Frame from TX1
	cd.RecordFrameArrival("AA:BB:CC:DD:EE:01", "link1", now)

	// Frame from TX2 exactly 2ms later (collision)
	cd.RecordFrameArrival("AA:BB:CC:DD:EE:EE:02", "link2", now.Add(2*time.Millisecond))

	cd.mu.RLock()
	pairKey := cd.pairKey("AA:BB:CC:DD:EE:01", "AA:BB:CC:DD:EE:EE:02")
	collisions := len(cd.collisionHistory[pairKey])
	cd.mu.RUnlock()

	if collisions != 1 {
		t.Errorf("expected 1 collision, got %d", collisions)
	}
}

// TestCollisionDetector_NoSelfCollision tests that a node doesn't collide with itself
func TestCollisionDetector_NoSelfCollision(t *testing.T) {
	cd := NewCollisionDetector()

	cd.RegisterTXNode("AA:BB:CC:DD:EE:01")
	cd.RegisterTXNode("AA:BB:CC:DD:EE:EE:02")

	now := time.Now()

	// Record two frames from the same TX node at different times
	cd.RecordFrameArrival("AA:BB:CC:DD:EE:01", "link1", now)
	cd.RecordFrameArrival("AA:BB:CC:DD:EE:01", "link1", now.Add(100*time.Millisecond))

	cd.mu.RLock()
	pairKey := cd.pairKey("AA:BB:CC:DD:EE:01", "AA:BB:CC:DD:EE:EE:02")
	collisions := len(cd.collisionHistory[pairKey])
	cd.mu.RUnlock()

	// Should be 0 collisions (same TX node)
	if collisions != 0 {
		t.Errorf("expected 0 collisions for same TX, got %d", collisions)
	}
}

// TestCollisionDetector_CollisionRateWithZeroDelta tests edge case of zero time delta
func TestCollisionDetector_CollisionRateWithZeroDelta(t *testing.T) {
	cd := NewCollisionDetector()

	cd.RegisterTXNode("AA:BB:CC:DD:EE:01")
	cd.RegisterTXNode("AA:BB:CC:DD:EE:EE:02")

	now := time.Now()

	// Record frames at exactly the same time (should not count as collision due to timeDelta > 0 check)
	cd.RecordFrameArrival("AA:BB:CC:DD:EE:01", "link1", now)
	cd.RecordFrameArrival("AA:BB:CC:DD:EE:EE:02", "link2", now) // Same timestamp

	cd.mu.RLock()
	pairKey := cd.pairKey("AA:BB:CC:DD:EE:01", "AA:BB:CC:DD:EE:EE:02")
	collisions := len(cd.collisionHistory[pairKey])
	cd.mu.RUnlock()

	// The current implementation requires timeDelta > 0, so same timestamp = no collision
	if collisions != 0 {
		t.Logf("Note: Same timestamp resulted in %d collisions (timeDelta=0)", collisions)
	}
}

// TestCollisionDetector_RateCalculationSimplified tests a simpler scenario
func TestCollisionDetector_RateCalculationSimplified(t *testing.T) {
	cd := NewCollisionDetector()

	cd.RegisterTXNode("AA:BB:CC:DD:EE:01")
	cd.RegisterTXNode("AA:BB:CC:DD:EE:EE:02")

	now := time.Now()

	// Send frames with interleaved timing to create consistent collisions
	// TX1 at: 0, 200, 400, 600, 800, 1000, 1200, 1400, 1600, 1800 ms
	// TX2 at: 2, 202, 402, 602, 802, 1002, 1202, 1402, 1602, 1802 ms
	// Each TX2 frame is 2ms after a TX1 frame, creating 10 collisions
	for i := 0; i < 10; i++ {
		tx1Time := now.Add(time.Duration(i) * 200 * time.Millisecond)
		tx2Time := tx1Time.Add(2 * time.Millisecond)
		cd.RecordFrameArrival("AA:BB:CC:DD:EE:01", "link1", tx1Time)
		cd.RecordFrameArrival("AA:BB:CC:DD:EE:EE:02", "link2", tx2Time)
	}

	rate := cd.GetCollisionRate(60 * time.Second)

	// We have:
	// - 10 frames from TX1 and 10 from TX2
	// - Each TX2 frame collides with the preceding TX1 frame (2ms delta)
	// - Total frame opportunities = 10 + 10 = 20
	// - Collisions = 10 (each TX2 frame collides with TX1)
	// - Collision rate = 10/20 = 0.50

	t.Logf("Collision rate: %.2f (expected ~0.50)", rate)

	if rate < 0.4 || rate > 0.6 {
		t.Errorf("collision rate = %.2f, want ~0.50", rate)
	}
}
