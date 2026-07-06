package ingestion

import (
	"testing"
	"time"
)

type rateSend struct {
	mac               string
	rate              int
	varianceThreshold float64
}

func newTestRC() (*RateController, *[]rateSend) {
	sent := &[]rateSend{}
	rc := NewRateController(func(mac string, rateHz int, vt float64) {
		*sent = append(*sent, rateSend{mac, rateHz, vt})
	})
	return rc, sent
}

func TestIdleToActive(t *testing.T) {
	rc, sent := newTestRC()

	rc.OnMotionState("AA:BB:CC:DD:EE:FF", true)

	if len(*sent) != 1 {
		t.Fatalf("expected 1 config send, got %d", len(*sent))
	}
	if (*sent)[0].rate != RateActive {
		t.Errorf("expected RateActive (%d), got %d", RateActive, (*sent)[0].rate)
	}
	if (*sent)[0].mac != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("unexpected mac %q", (*sent)[0].mac)
	}
	// Active mode: variance threshold 0 (disable on-device hint; server handles detection)
	if (*sent)[0].varianceThreshold != 0 {
		t.Errorf("expected varianceThreshold=0 in active mode, got %v", (*sent)[0].varianceThreshold)
	}
}

func TestActiveNoRedundantSend(t *testing.T) {
	rc, sent := newTestRC()

	// First motion event: ramp up
	rc.OnMotionState("AA:BB:CC:DD:EE:FF", true)
	// Subsequent motion events while active: no additional sends
	rc.OnMotionState("AA:BB:CC:DD:EE:FF", true)
	rc.OnMotionState("AA:BB:CC:DD:EE:FF", true)

	if len(*sent) != 1 {
		t.Errorf("expected 1 send, got %d (redundant sends should be suppressed)", len(*sent))
	}
}

func TestNoSendOnNoMotion(t *testing.T) {
	rc, sent := newTestRC()

	rc.OnMotionState("AA:BB:CC:DD:EE:FF", false)

	if len(*sent) != 0 {
		t.Errorf("expected 0 sends for no-motion event, got %d", len(*sent))
	}
}

func TestIdleTimeoutDropsRate(t *testing.T) {
	rc, sent := newTestRC()

	// Ramp up
	rc.OnMotionState("AA:BB:CC:DD:EE:FF", true)

	// Manually set lastMotionAt far in the past to simulate timeout
	rc.mu.Lock()
	rc.nodes["AA:BB:CC:DD:EE:FF"].lastMotionAt = time.Now().Add(-idleTimeout - time.Second)
	rc.mu.Unlock()

	rc.checkIdleTimeouts()

	if len(*sent) != 2 {
		t.Fatalf("expected 2 sends (active + idle), got %d", len(*sent))
	}
	if (*sent)[1].rate != RateIdle {
		t.Errorf("expected RateIdle (%d) after timeout, got %d", RateIdle, (*sent)[1].rate)
	}
	// Idle mode: variance threshold re-enabled so device can self-burst
	if (*sent)[1].varianceThreshold != DefaultVarianceThreshold {
		t.Errorf("expected varianceThreshold=%v in idle mode, got %v",
			DefaultVarianceThreshold, (*sent)[1].varianceThreshold)
	}
}

func TestIdleTimeoutIs30Seconds(t *testing.T) {
	if idleTimeout != 30*time.Second {
		t.Errorf("idleTimeout should be 30s, got %v", idleTimeout)
	}
}

func TestIdleTimeoutNotTriggeredEarly(t *testing.T) {
	rc, sent := newTestRC()

	rc.OnMotionState("AA:BB:CC:DD:EE:FF", true)
	initialSends := len(*sent)

	// Timeout has not elapsed
	rc.checkIdleTimeouts()

	if len(*sent) != initialSends {
		t.Errorf("expected no new sends before timeout, got %d new sends", len(*sent)-initialSends)
	}
}

func TestMotionHint(t *testing.T) {
	rc, sent := newTestRC()

	rc.OnMotionHint("AA:BB:CC:DD:EE:FF")

	if len(*sent) != 1 || (*sent)[0].rate != RateActive {
		t.Errorf("OnMotionHint should ramp to active; sends=%v", *sent)
	}
}

func TestMotionHintRampsAdjacentNodes(t *testing.T) {
	rc, sent := newTestRC()

	rc.SetAdjacentNodesFn(func(mac string) []string {
		if mac == "AA:BB:CC:DD:EE:01" {
			return []string{"AA:BB:CC:DD:EE:02", "AA:BB:CC:DD:EE:03"}
		}
		return nil
	})

	rc.OnMotionHint("AA:BB:CC:DD:EE:01")

	// Should have ramped: hinting node + 2 adjacent = 3 sends
	if len(*sent) != 3 {
		t.Fatalf("expected 3 sends (hinting + 2 adjacent), got %d: %v", len(*sent), *sent)
	}

	macs := map[string]bool{}
	for _, s := range *sent {
		macs[s.mac] = true
		if s.rate != RateActive {
			t.Errorf("expected RateActive for %s, got %d", s.mac, s.rate)
		}
	}
	for _, mac := range []string{"AA:BB:CC:DD:EE:01", "AA:BB:CC:DD:EE:02", "AA:BB:CC:DD:EE:03"} {
		if !macs[mac] {
			t.Errorf("expected config send for %s, not found", mac)
		}
	}
}

func TestMotionHintNoAdjacentFn(t *testing.T) {
	rc, sent := newTestRC()
	// No adjacentNodes set: only the hinting node is ramped
	rc.OnMotionHint("AA:BB:CC:DD:EE:FF")
	if len(*sent) != 1 {
		t.Errorf("expected 1 send without adjacentNodes fn, got %d", len(*sent))
	}
}

func TestActiveToIdleAndBackToActive(t *testing.T) {
	rc, sent := newTestRC()

	// Ramp up
	rc.OnMotionState("AA:BB:CC:DD:EE:FF", true)

	// Force timeout
	rc.mu.Lock()
	rc.nodes["AA:BB:CC:DD:EE:FF"].lastMotionAt = time.Now().Add(-idleTimeout - time.Second)
	rc.mu.Unlock()
	rc.checkIdleTimeouts()

	// Motion detected again
	rc.OnMotionState("AA:BB:CC:DD:EE:FF", true)

	// Should have: active, idle, active
	if len(*sent) != 3 {
		t.Fatalf("expected 3 sends, got %d: %v", len(*sent), *sent)
	}
	if (*sent)[2].rate != RateActive {
		t.Errorf("expected third send to be RateActive, got %d", (*sent)[2].rate)
	}
}

func TestNodeDisconnectClearsState(t *testing.T) {
	rc, _ := newTestRC()

	rc.OnMotionState("AA:BB:CC:DD:EE:FF", true)
	rc.OnNodeDisconnected("AA:BB:CC:DD:EE:FF")

	rc.mu.Lock()
	_, exists := rc.nodes["AA:BB:CC:DD:EE:FF"]
	rc.mu.Unlock()

	if exists {
		t.Error("node state should be removed after disconnect")
	}
}

// Zone-aware tests

func TestZoneMembershipTracking(t *testing.T) {
	rc, sent := newTestRC()

	// Set zone membership function
	rc.SetZoneMembershipFn(func(mac string) []string {
		if mac == "AA:BB:CC:DD:EE:01" {
			return []string{"zone-a", "zone-b"} // Node in overlapping zones
		}
		if mac == "AA:BB:CC:DD:EE:02" {
			return []string{"zone-a"}
		}
		return nil
	})

	// Motion in zone-a node
	rc.OnMotionState("AA:BB:CC:DD:EE:01", true)

	// Should ramp the node to active
	if len(*sent) != 1 || (*sent)[0].rate != RateActive {
		t.Errorf("expected node to ramp to active, got %d sends: %v", len(*sent), *sent)
	}

	// Check zone state was updated
	rc.mu.Lock()
	zoneA := rc.zones["zone-a"]
	zoneB := rc.zones["zone-b"]
	rc.mu.Unlock()

	if zoneA == nil || zoneB == nil {
		t.Fatal("zones should be created")
	}
	if !zoneA.nodes["AA:BB:CC:DD:EE:01"] || !zoneB.nodes["AA:BB:CC:DD:EE:01"] {
		t.Error("node should be registered in both zones")
	}
	if zoneA.allNodesIdle || zoneB.allNodesIdle {
		t.Error("zones should be marked active after motion")
	}
}

func TestZoneIdleDetectionAndSentinelDesignation(t *testing.T) {
	rc, sent := newTestRC()

	// Set zone membership
	rc.SetZoneMembershipFn(func(mac string) []string {
		return []string{"zone-a"}
	})

	// Add two nodes to zone-a (both active)
	rc.OnMotionState("AA:BB:CC:DD:EE:01", true)
	rc.OnMotionState("AA:BB:CC:DD:EE:02", true)

	// Both should be at active rate
	if len(*sent) != 2 {
		t.Fatalf("expected 2 active sends, got %d", len(*sent))
	}

	// Force both nodes to timeout
	rc.mu.Lock()
	rc.nodes["AA:BB:CC:DD:EE:01"].lastMotionAt = time.Now().Add(-idleTimeout - time.Second)
	rc.nodes["AA:BB:CC:DD:EE:02"].lastMotionAt = time.Now().Add(-idleTimeout - time.Second)
	rc.mu.Unlock()

	// Run idle timeout check
	rc.checkIdleTimeouts()

	// Should have sent 2 more configs: sentinel (5 Hz) and non-sentinel (1 Hz)
	// Sentinel is lexicographically smaller MAC
	if len(*sent) != 4 {
		t.Fatalf("expected 4 sends total (2 active + 2 idle), got %d: %v", len(*sent), *sent)
	}

	// Verify rates
	sentinelMAC := "AA:BB:CC:DD:EE:01" // Lexicographically smaller
	otherMAC := "AA:BB:CC:DD:EE:02"

	sentinelRate, otherRate := -1, -1
	for _, s := range *sent {
		if s.mac == sentinelMAC && s.rate != RateActive { // Skip initial active send
			sentinelRate = s.rate
		}
		if s.mac == otherMAC && s.rate != RateActive {
			otherRate = s.rate
		}
	}

	if sentinelRate != RateSentinel {
		t.Errorf("expected sentinel rate %d, got %d", RateSentinel, sentinelRate)
	}
	if otherRate != RateFleetIdle {
		t.Errorf("expected non-sentinel rate %d, got %d", RateFleetIdle, otherRate)
	}

	// Verify zone state
	rc.mu.Lock()
	zone := rc.zones["zone-a"]
	rc.mu.Unlock()

	if !zone.allNodesIdle {
		t.Error("zone should be marked idle after timeout")
	}
	if zone.sentinelNodeMAC != sentinelMAC {
		t.Errorf("expected sentinel %s, got %s", sentinelMAC, zone.sentinelNodeMAC)
	}
}

func TestFleetIdleDetection(t *testing.T) {
	rc, sent := newTestRC()

	// Set zone membership and adjacent zones
	rc.SetZoneMembershipFn(func(mac string) []string {
		if mac == "AA:BB:CC:DD:EE:01" || mac == "AA:BB:CC:DD:EE:02" {
			return []string{"zone-a"}
		}
		if mac == "AA:BB:CC:DD:EE:03" {
			return []string{"zone-b"}
		}
		return nil
	})

	rc.SetAdjacentZonesFn(func(zoneID string) []string {
		if zoneID == "zone-a" {
			return []string{"zone-b"}
		}
		if zoneID == "zone-b" {
			return []string{"zone-a"}
		}
		return nil
	})

	// Add nodes to both zones (all active)
	rc.OnMotionState("AA:BB:CC:DD:EE:01", true)
	rc.OnMotionState("AA:BB:CC:DD:EE:02", true)
	rc.OnMotionState("AA:BB:CC:DD:EE:03", true)

	// All should be at active rate
	if len(*sent) != 3 {
		t.Fatalf("expected 3 active sends, got %d", len(*sent))
	}

	// Force all nodes to timeout
	rc.mu.Lock()
	for mac := range rc.nodes {
		rc.nodes[mac].lastMotionAt = time.Now().Add(-idleTimeout - time.Second)
	}
	rc.mu.Unlock()

	// Run idle timeout check
	rc.checkIdleTimeouts()

	// Should have sent configs for sentinel + non-sentinel in each zone
	// zone-a: 2 nodes (sentinel + non-sentinel), zone-b: 1 node (sentinel only)
	expectedSends := 3 + 2 + 1 // initial active + idle configs
	if len(*sent) != expectedSends {
		t.Fatalf("expected %d sends, got %d: %v", expectedSends, len(*sent), *sent)
	}

	// Verify fleet is idle
	rc.mu.Lock()
	fleetIdle := rc.fleetIdle
	rc.mu.Unlock()

	if !fleetIdle {
		t.Error("fleet should be idle when all zones are idle")
	}
}

func TestAdjacentZoneRamping(t *testing.T) {
	rc, sent := newTestRC()

	// Set zone membership and adjacent zones
	rc.SetZoneMembershipFn(func(mac string) []string {
		if mac == "AA:BB:CC:DD:EE:01" {
			return []string{"zone-a"}
		}
		if mac == "AA:BB:CC:DD:EE:02" {
			return []string{"zone-b"}
		}
		return nil
	})

	rc.SetAdjacentZonesFn(func(zoneID string) []string {
		if zoneID == "zone-a" {
			return []string{"zone-b"}
		}
		return nil
	})

	// Zone-b is idle (timeout) - start with idle node
	rc.OnMotionState("AA:BB:CC:DD:EE:02", true)
	rc.mu.Lock()
	rc.nodes["AA:BB:CC:DD:EE:02"].lastMotionAt = time.Now().Add(-idleTimeout - time.Second)
	rc.mu.Unlock()

	// Run idle timeout to mark zone-b idle and designate sentinel
	rc.checkIdleTimeouts()

	// Reset sent tracker to count only zone-a activation sends
	initialSends := len(*sent)
	*sent = (*sent)[:initialSends]

	// Zone-a detects motion (should ramp adjacent zone-b to sentinel)
	rc.OnMotionState("AA:BB:CC:DD:EE:01", true)

	// Should have: zone-a node active + zone-b node ramped to sentinel
	// Note: zone-b node is already at sentinel (5 Hz), but we ramp it again
	// This is expected behavior for preemptive coverage
	if len(*sent) != initialSends+2 {
		t.Fatalf("expected 2 sends (zone-a active + zone-b sentinel), got %d: %v", len(*sent)-initialSends, (*sent)[initialSends:])
	}

	// Verify zone-a went to active and zone-b went to sentinel
	zoneAActive := false
	zoneBSentinel := false
	for i := initialSends; i < len(*sent); i++ {
		if (*sent)[i].mac == "AA:BB:CC:DD:EE:01" && (*sent)[i].rate == RateActive {
			zoneAActive = true
		}
		if (*sent)[i].mac == "AA:BB:CC:DD:EE:02" && (*sent)[i].rate == RateSentinel {
			zoneBSentinel = true
		}
	}
	if !zoneAActive {
		t.Error("zone-a node should be ramped to active rate")
	}
	if !zoneBSentinel {
		t.Error("adjacent zone-b node should be ramped to sentinel rate")
	}
}

func TestRampZonePredictionEngine(t *testing.T) {
	rc, sent := newTestRC()

	// Set zone membership
	rc.SetZoneMembershipFn(func(mac string) []string {
		if mac == "AA:BB:CC:DD:EE:01" || mac == "AA:BB:CC:DD:EE:02" {
			return []string{"zone-a"}
		}
		return nil
	})

	// Add nodes to zone-a (they'll be idle by default)
	rc.OnMotionState("AA:BB:CC:DD:EE:01", true)
	rc.OnMotionState("AA:BB:CC:DD:EE:02", true)

	// Force idle timeout
	rc.mu.Lock()
	for mac := range rc.nodes {
		rc.nodes[mac].lastMotionAt = time.Now().Add(-idleTimeout - time.Second)
	}
	rc.mu.Unlock()
	rc.checkIdleTimeouts()

	// Reset sent tracker to count only RampZone sends
	initialSends := len(*sent)
	*sent = (*sent)[:initialSends]

	// Prediction engine ramps zone-a (with adjacent zone ramping disabled)
	rc.RampZone("zone-a", false)

	// Should have ramped both nodes to active
	if len(*sent) != initialSends+2 {
		t.Fatalf("expected 2 ramp sends, got %d: %v", len(*sent)-initialSends, (*sent)[initialSends:])
	}

	for i := initialSends; i < len(*sent); i++ {
		if (*sent)[i].rate != RateActive {
			t.Errorf("RampZone should set active rate, got %d", (*sent)[i].rate)
		}
	}

	// Verify zone state updated
	rc.mu.Lock()
	zone := rc.zones["zone-a"]
	rc.mu.Unlock()

	if zone.allNodesIdle {
		t.Error("zone should be marked active after RampZone")
	}
}

func TestBackwardCompatibilityNoZones(t *testing.T) {
	rc, sent := newTestRC()

	// No zone membership function set - should fall back to per-node behavior

	rc.OnMotionState("AA:BB:CC:DD:EE:FF", true)

	if len(*sent) != 1 || (*sent)[0].rate != RateActive {
		t.Errorf("should work without zones (per-node fallback), got %d sends: %v", len(*sent), *sent)
	}

	// Force timeout
	rc.mu.Lock()
	rc.nodes["AA:BB:CC:DD:EE:FF"].lastMotionAt = time.Now().Add(-idleTimeout - time.Second)
	rc.mu.Unlock()

	rc.checkIdleTimeouts()

	// Should drop to idle rate (2 Hz)
	if len(*sent) != 2 || (*sent)[1].rate != RateIdle {
		t.Errorf("should drop to RateIdle (2 Hz) without zones, got %d sends: %v", len(*sent), *sent)
	}
}

func TestNodeDisconnectWithSentinelRedesignation(t *testing.T) {
	rc, sent := newTestRC()

	// Set zone membership
	rc.SetZoneMembershipFn(func(mac string) []string {
		return []string{"zone-a"}
	})

	// Add two nodes, let them go idle
	rc.OnMotionState("AA:BB:CC:DD:EE:01", true)
	rc.OnMotionState("AA:BB:CC:DD:EE:02", true)

	// Force timeout
	rc.mu.Lock()
	for mac := range rc.nodes {
		rc.nodes[mac].lastMotionAt = time.Now().Add(-idleTimeout - time.Second)
	}
	rc.mu.Unlock()
	rc.checkIdleTimeouts()

	// Sentinel should be 01 (lexicographically smaller)
	rc.mu.Lock()
	sentinelMAC := rc.zones["zone-a"].sentinelNodeMAC
	rc.mu.Unlock()

	if sentinelMAC != "AA:BB:CC:DD:EE:01" {
		t.Errorf("expected sentinel 01, got %s", sentinelMAC)
	}

	// Disconnect sentinel
	rc.OnNodeDisconnected("AA:BB:CC:DD:EE:01")

	// New sentinel should be redesignated (02)
	rc.mu.Lock()
	newSentinelMAC := rc.zones["zone-a"].sentinelNodeMAC
	rc.mu.Unlock()

	if newSentinelMAC != "AA:BB:CC:DD:EE:02" {
		t.Errorf("expected new sentinel 02, got %s", newSentinelMAC)
	}

	// Should have sent config for new sentinel
	lastSend := (*sent)[len(*sent)-1]
	if lastSend.mac != "AA:BB:CC:DD:EE:02" || lastSend.rate != RateSentinel {
		t.Errorf("expected new sentinel config, got %d Hz for %s", lastSend.rate, lastSend.mac)
	}
}
