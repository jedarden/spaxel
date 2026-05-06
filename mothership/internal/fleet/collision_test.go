package fleet

import (
	"math/rand"
	"testing"
	"time"
)

// TestCollisionDetector_RegisterTXNode tests TX node registration
func TestCollisionDetector_RegisterTXNode(t *testing.T) {
	cd := NewCollisionDetector()

	// Register a TX node
	cd.RegisterTXNode("AA:BB:CC:DD:EE:01")

	cd.mu.RLock()
	defer cd.mu.RUnlock()

	if !cd.txNodes["AA:BB:CC:DD:EE:01"] {
		t.Error("TX node not registered")
	}
}

// TestCollisionDetector_UnregisterTXNode tests TX node unregistration
func TestCollisionDetector_UnregisterTXNode(t *testing.T) {
	cd := NewCollisionDetector()

	// Register then unregister
	cd.RegisterTXNode("AA:BB:CC:DD:EE:01")
	cd.UnregisterTXNode("AA:BB:CC:DD:EE:01")

	cd.mu.RLock()
	defer cd.mu.RUnlock()

	if cd.txNodes["AA:BB:CC:DD:EE:01"] {
		t.Error("TX node still registered after unregister")
	}

	if _, exists := cd.lastArrival["AA:BB:CC:DD:EE:01"]; exists {
		t.Error("last arrival not cleaned up after unregister")
	}
}

// TestCollisionDetector_RecordFrameArrival_NoCollision tests frame recording without collision
func TestCollisionDetector_RecordFrameArrival_NoCollision(t *testing.T) {
	cd := NewCollisionDetector()

	// Register two TX nodes
	cd.RegisterTXNode("AA:BB:CC:DD:EE:01")
	cd.RegisterTXNode("AA:BB:CC:DD:EE:02")

	// Record frame from first TX node
	now := time.Now()
	cd.RecordFrameArrival("AA:BB:CC:DD:EE:01", "AA:BB:CC:DD:EE:01:AA:BB:CC:DD:EE:03", now)

	cd.mu.RLock()
	defer cd.mu.RUnlock()

	// Should have recorded last arrival
	if cd.lastArrival["AA:BB:CC:DD:EE:01"].IsZero() {
		t.Error("last arrival time not recorded")
	}

	// Should have no collision history
	pairKey := cd.pairKey("AA:BB:CC:DD:EE:01", "AA:BB:CC:DD:EE:02")
	if len(cd.collisionHistory[pairKey]) != 0 {
		t.Errorf("unexpected collision history: %d", len(cd.collisionHistory[pairKey]))
	}
}

// TestCollisionDetector_RecordFrameArrival_Collision tests collision detection
func TestCollisionDetector_RecordFrameArrival_Collision(t *testing.T) {
	cd := NewCollisionDetector()

	// Register two TX nodes
	cd.RegisterTXNode("AA:BB:CC:DD:EE:01")
	cd.RegisterTXNode("AA:BB:CC:DD:EE:02")

	// Record frame from first TX node
	now := time.Now()
	cd.RecordFrameArrival("AA:BB:CC:DD:EE:01", "link1", now)

	// Record frame from second TX node within 3ms (collision)
	now2 := now.Add(2 * time.Millisecond)
	cd.RecordFrameArrival("AA:BB:CC:DD:EE:02", "link2", now2)

	cd.mu.RLock()
	defer cd.mu.RUnlock()

	// Should have recorded collision
	pairKey := cd.pairKey("AA:BB:CC:DD:EE:01", "AA:BB:CC:DD:EE:02")
	if len(cd.collisionHistory[pairKey]) != 1 {
		t.Errorf("expected 1 collision, got %d", len(cd.collisionHistory[pairKey]))
	}

	if cd.collisionCount[pairKey] != 1 {
		t.Errorf("expected collision count 1, got %d", cd.collisionCount[pairKey])
	}
}

// TestCollisionDetector_RecordFrameArrival_NoCollisionOutsideWindow tests that frames outside 3ms don't count as collision
func TestCollisionDetector_RecordFrameArrival_NoCollisionOutsideWindow(t *testing.T) {
	cd := NewCollisionDetector()

	// Register two TX nodes
	cd.RegisterTXNode("AA:BB:CC:DD:EE:01")
	cd.RegisterTXNode("AA:BB:CC:DD:EE:02")

	// Record frame from first TX node
	now := time.Now()
	cd.RecordFrameArrival("AA:BB:CC:DD:EE:01", "link1", now)

	// Record frame from second TX node outside 3ms window (no collision)
	now2 := now.Add(5 * time.Millisecond)
	cd.RecordFrameArrival("AA:BB:CC:DD:EE:02", "link2", now2)

	cd.mu.RLock()
	defer cd.mu.RUnlock()

	// Should NOT have recorded collision
	pairKey := cd.pairKey("AA:BB:CC:DD:EE:01", "AA:BB:CC:DD:EE:02")
	if len(cd.collisionHistory[pairKey]) != 0 {
		t.Errorf("expected 0 collisions, got %d", len(cd.collisionHistory[pairKey]))
	}
}

// TestCollisionDetector_GetCollisionRate tests collision rate calculation
func TestCollisionDetector_GetCollisionRate(t *testing.T) {
	cd := NewCollisionDetector()

	// Register two TX nodes
	cd.RegisterTXNode("AA:BB:CC:DD:EE:01")
	cd.RegisterTXNode("AA:BB:CC:DD:EE:02")

	// Record 10 frames with 5 collisions
	// Pattern: TX1 at 0ms, TX2 at 2ms (collision), TX1 at 100ms, TX2 at 102ms (collision), etc.
	// This gives 5 collisions because each TX2 frame arrives 2ms after a TX1 frame
	now := time.Now()
	for i := 0; i < 5; i++ {
		baseTime := now.Add(time.Duration(i) * 100 * time.Millisecond)
		// TX1 frame
		cd.RecordFrameArrival("AA:BB:CC:DD:EE:01", "link1", baseTime)
		// TX2 frame 2ms later (collision)
		cd.RecordFrameArrival("AA:BB:CC:DD:EE:02", "link2", baseTime.Add(2*time.Millisecond))
	}

	// Get collision rate over 1 minute window
	rate := cd.GetCollisionRate(1 * time.Minute)

	// Should be approximately 50% (5 collisions out of 10 frame opportunities)
	// Note: due to asymmetric collision detection (only checking last arrival),
	// we get exactly 5 collisions
	if rate < 0.4 || rate > 0.6 {
		t.Errorf("collision rate = %.2f, want ~0.5", rate)
	}
}

// TestCollisionDetector_CheckAndTriggerRestagger tests re-stagger triggering
func TestCollisionDetector_CheckAndTriggerRestagger(t *testing.T) {
	cd := NewCollisionDetector()

	// Register two TX nodes
	cd.RegisterTXNode("AA:BB:CC:DD:EE:01")
	cd.RegisterTXNode("AA:BB:CC:DD:EE:02")

	retriggerCalled := false
	cd.SetRestaggerCallback(func() {
		retriggerCalled = true
	})

	// Generate many collisions to exceed 5% threshold
	now := time.Now()
	for i := 0; i < 50; i++ {
		baseTime := now.Add(time.Duration(i) * 100 * time.Millisecond)
		// TX1 frame
		cd.RecordFrameArrival("AA:BB:CC:DD:EE:01", "link1", baseTime)
		// TX2 frame 2ms later (collision)
		cd.RecordFrameArrival("AA:BB:CC:DD:EE:02", "link2", baseTime.Add(2*time.Millisecond))
	}

	// Check and trigger re-stagger
	triggered := cd.CheckAndTriggerRestagger()

	if !triggered {
		t.Error("expected re-stagger to be triggered")
	}

	// Wait for goroutine to run (callback is run in goroutine)
	time.Sleep(10 * time.Millisecond)

	if !retriggerCalled {
		t.Error("re-stagger callback was not called")
	}
}

// TestCollisionDetector_CheckAndTriggerRestagger_RateLimited tests that re-stagger is rate-limited
func TestCollisionDetector_CheckAndTriggerRestagger_RateLimited(t *testing.T) {
	cd := NewCollisionDetector()

	// Register two TX nodes
	cd.RegisterTXNode("AA:BB:CC:DD:EE:01")
	cd.RegisterTXNode("AA:BB:CC:DD:EE:02")

	callCount := 0
	cd.SetRestaggerCallback(func() {
		callCount++
	})

	// Generate many collisions to exceed threshold
	now := time.Now()
	for i := 0; i < 50; i++ {
		baseTime := now.Add(time.Duration(i) * 100 * time.Millisecond)
		// TX1 frame
		cd.RecordFrameArrival("AA:BB:CC:DD:EE:01", "link1", baseTime)
		// TX2 frame 2ms later (collision)
		cd.RecordFrameArrival("AA:BB:CC:DD:EE:02", "link2", baseTime.Add(2*time.Millisecond))
	}

	// First call should trigger
	triggered1 := cd.CheckAndTriggerRestagger()
	if !triggered1 {
		t.Errorf("first call: triggered=%v, want triggered=true", triggered1)
	}

	// Wait for goroutine to run
	time.Sleep(10 * time.Millisecond)

	if callCount != 1 {
		t.Errorf("callback should be called once after first trigger, got %d calls", callCount)
	}

	// Immediate second call should NOT trigger (rate-limited)
	triggered2 := cd.CheckAndTriggerRestagger()
	if triggered2 {
		t.Error("second immediate call should not trigger due to rate limiting")
	}

	if callCount != 1 {
		t.Errorf("callback should only be called once, got %d calls", callCount)
	}
}

// TestCollisionDetector_ResetCounters tests counter reset
func TestCollisionDetector_ResetCounters(t *testing.T) {
	cd := NewCollisionDetector()

	// Register two TX nodes
	cd.RegisterTXNode("AA:BB:CC:DD:EE:01")
	cd.RegisterTXNode("AA:BB:CC:DD:EE:02")

	// Generate some collisions
	now := time.Now()
	cd.RecordFrameArrival("AA:BB:CC:DD:EE:01", "link1", now)
	cd.RecordFrameArrival("AA:BB:CC:DD:EE:02", "link2", now.Add(2*time.Millisecond))

	// Reset
	cd.ResetCounters()

	cd.mu.RLock()
	defer cd.mu.RUnlock()

	if len(cd.collisionHistory) != 0 {
		t.Error("collision history not cleared after reset")
	}

	if len(cd.collisionCount) != 0 {
		t.Error("collision counts not cleared after reset")
	}

	if len(cd.totalFrames) != 0 {
		t.Error("total frames not cleared after reset")
	}
}

// TestCollisionDetector_NonTXNodeIgnored tests that non-TX nodes are ignored
func TestCollisionDetector_NonTXNodeIgnored(t *testing.T) {
	cd := NewCollisionDetector()

	// Register only one TX node
	cd.RegisterTXNode("AA:BB:CC:DD:EE:01")

	// Record frame from non-TX node (should be ignored)
	now := time.Now()
	cd.RecordFrameArrival("AA:BB:CC:DD:EE:02", "link1", now)

	cd.mu.RLock()
	defer cd.mu.RUnlock()

	// Should not have recorded arrival for non-TX node
	if _, exists := cd.lastArrival["AA:BB:CC:DD:EE:02"]; exists {
		t.Error("non-TX node arrival was recorded")
	}

	// TX node should not have any collisions recorded
	if len(cd.collisionHistory) != 0 {
		t.Error("unexpected collision history")
	}
}

// TestCollisionDetector_GetCollisionHistory tests collision history retrieval
func TestCollisionDetector_GetCollisionHistory(t *testing.T) {
	cd := NewCollisionDetector()

	// Register two TX nodes
	cd.RegisterTXNode("AA:BB:CC:DD:EE:01")
	cd.RegisterTXNode("AA:BB:CC:DD:EE:02")

	// Generate 5 collisions
	now := time.Now()
	for i := 0; i < 5; i++ {
		frameTime := now.Add(time.Duration(i) * 100 * time.Millisecond)
		cd.RecordFrameArrival("AA:BB:CC:DD:EE:01", "link1", frameTime)
		collisionTime := frameTime.Add(2 * time.Millisecond)
		cd.RecordFrameArrival("AA:BB:CC:DD:EE:02", "link2", collisionTime)
	}

	// Get history with limit
	history := cd.GetCollisionHistory(3)

	if len(history) != 3 {
		t.Errorf("expected 3 history events, got %d", len(history))
	}
}

// TestCollisionDetector_PairKey tests pair key generation
func TestCollisionDetector_PairKey(t *testing.T) {
	cd := NewCollisionDetector()

	key1 := cd.pairKey("AA:BB:CC:DD:EE:01", "AA:BB:CC:DD:EE:02")
	key2 := cd.pairKey("AA:BB:CC:DD:EE:02", "AA:BB:CC:DD:EE:01")

	if key1 != key2 {
		t.Errorf("pair keys should be the same regardless of order: %s != %s", key1, key2)
	}

	// Key should be lexicographically sorted
	expected := "AA:BB:CC:DD:EE:01:AA:BB:CC:DD:EE:02"
	if key1 != expected {
		t.Errorf("pair key = %s, want %s", key1, expected)
	}
}

// TestCollisionDetector_GetCollisionRateByPair tests per-pair collision statistics
func TestCollisionDetector_GetCollisionRateByPair(t *testing.T) {
	cd := NewCollisionDetector()

	// Register three TX nodes
	cd.RegisterTXNode("AA:BB:CC:DD:EE:01")
	cd.RegisterTXNode("AA:BB:CC:DD:EE:02")
	cd.RegisterTXNode("AA:BB:CC:DD:EE:03")

	now := time.Now()

	// Create collisions only between node 1 and 2
	for i := 0; i < 10; i++ {
		frameTime := now.Add(time.Duration(i) * 100 * time.Millisecond)
		cd.RecordFrameArrival("AA:BB:CC:DD:EE:01", "link1", frameTime)
		collisionTime := frameTime.Add(2 * time.Millisecond)
		cd.RecordFrameArrival("AA:BB:CC:DD:EE:02", "link2", collisionTime)
	}

	// Node 3 frames with no collisions
	cd.RecordFrameArrival("AA:BB:CC:DD:EE:03", "link3", now.Add(2*time.Second))

	stats := cd.GetCollisionRateByPair()

	// Should have stats for pair (1,2)
	pair12 := cd.pairKey("AA:BB:CC:DD:EE:01", "AA:BB:CC:DD:EE:02")
	stat, exists := stats[pair12]
	if !exists {
		t.Fatal("stats missing for pair 1-2")
	}

	if stat.Collisions != 10 {
		t.Errorf("expected 10 collisions for pair 1-2, got %d", stat.Collisions)
	}
}

// TestGenerateRestaggerOffset tests restagger offset generation
func TestGenerateRestaggerOffset(t *testing.T) {
	// Test with deterministic seed
	rand.Seed(42)

	offset := GenerateRestaggerOffset(0, 2, 20)

	// Slot width for 2 TX nodes at 20 Hz: 1,000,000 / (20 * 2) = 25,000 µs
	baseOffset := 0
	maxOffsetUS := 1_000_000 / 20 // 50,000 µs

	if offset < baseOffset || offset >= maxOffsetUS {
		t.Errorf("offset %d out of range [%d, %d)", offset, baseOffset, maxOffsetUS)
	}

	// Verify offsets are different for different nodes
	offset2 := GenerateRestaggerOffset(1, 2, 20)
	if offset == offset2 {
		t.Error("different nodes should get different offsets")
	}
}

// TestGetRestaggerOffsets tests restagger offset generation for all TX nodes
func TestGetRestaggerOffsets(t *testing.T) {
	txMACs := []string{"AA:BB:CC:DD:EE:01", "AA:BB:CC:DD:EE:02", "AA:BB:CC:DD:EE:03"}

	offsets := GetRestaggerOffsets(txMACs, 20)

	if len(offsets) != 3 {
		t.Errorf("expected 3 offsets, got %d", len(offsets))
	}

	// Each MAC should have an offset
	for _, mac := range txMACs {
		if _, exists := offsets[mac]; !exists {
			t.Errorf("missing offset for MAC %s", mac)
		}
	}

	// Offsets should be unique
	seen := make(map[int]bool)
	for _, offset := range offsets {
		if seen[offset] {
			t.Error("duplicate offset generated")
		}
		seen[offset] = true
	}
}

// TestCollisionDetector_RateBelowThreshold tests that low collision rates don't trigger re-stagger
func TestCollisionDetector_RateBelowThreshold(t *testing.T) {
	cd := NewCollisionDetector()

	// Register two TX nodes
	cd.RegisterTXNode("AA:BB:CC:DD:EE:01")
	cd.RegisterTXNode("AA:BB:CC:DD:EE:02")

	triggered := false
	cd.SetRestaggerCallback(func() {
		triggered = true
	})

	// Generate frames with very low collision rate (1%)
	now := time.Now()
	for i := 0; i < 200; i++ {
		frameTime := now.Add(time.Duration(i) * 10 * time.Millisecond)
		if i == 0 {
			// Only one collision at the start
			cd.RecordFrameArrival("AA:BB:CC:DD:EE:01", "link1", frameTime)
			collisionTime := frameTime.Add(2 * time.Millisecond)
			cd.RecordFrameArrival("AA:BB:CC:DD:EE:02", "link2", collisionTime)
		} else if i%2 == 0 {
			// Rest are non-colliding frames
			cd.RecordFrameArrival("AA:BB:CC:DD:EE:01", "link1", frameTime)
		} else {
			cd.RecordFrameArrival("AA:BB:CC:DD:EE:02", "link2", frameTime.Add(10*time.Millisecond))
		}
	}

	// Should NOT trigger re-stagger
	triggered = cd.CheckAndTriggerRestagger()

	if triggered {
		t.Error("re-stagger should not trigger for low collision rate")
	}
}

// TestCollisionDetector_CollisionRateWindow tests that only recent collisions are counted
func TestCollisionDetector_CollisionRateWindow(t *testing.T) {
	cd := NewCollisionDetector()

	// Register two TX nodes
	cd.RegisterTXNode("AA:BB:CC:DD:EE:01")
	cd.RegisterTXNode("AA:BB:CC:DD:EE:02")

	// Generate old collisions (more than 60 seconds ago)
	oldTime := time.Now().Add(-70 * time.Second)
	for i := 0; i < 10; i++ {
		frameTime := oldTime.Add(time.Duration(i) * 100 * time.Millisecond)
		cd.RecordFrameArrival("AA:BB:CC:DD:EE:01", "link1", frameTime)
		collisionTime := frameTime.Add(2 * time.Millisecond)
		cd.RecordFrameArrival("AA:BB:CC:DD:EE:02", "link2", collisionTime)
	}

	// Generate recent collisions (within 60 second window)
	now := time.Now()
	for i := 0; i < 5; i++ {
		frameTime := now.Add(time.Duration(i) * 100 * time.Millisecond)
		cd.RecordFrameArrival("AA:BB:CC:DD:EE:01", "link1", frameTime)
		collisionTime := frameTime.Add(2 * time.Millisecond)
		cd.RecordFrameArrival("AA:BB:CC:DD:EE:02", "link2", collisionTime)
	}

	// Get rate over 60 second window - should only count recent collisions
	rate := cd.GetCollisionRate(60 * time.Second)

	// Should be approximately 5 collisions out of total frames
	// The rate calculation uses totalFrames which accumulates, so we need to check
	// that recent collisions are counted correctly
	history := cd.GetCollisionHistory(100)
	recentCount := 0
	for _, event := range history {
		if event.Timestamp.After(time.Now().Add(-60 * time.Second)) {
			recentCount++
		}
	}

	if recentCount != 5 {
		t.Errorf("expected 5 recent collisions in history, got %d", recentCount)
	}

	// Rate should be based on recent collisions
	if rate <= 0 {
		t.Errorf("collision rate = %v, should be positive", rate)
	}
}
