package ingestion

import (
	"testing"
	"time"
)

type rateSend struct {
	mac  string
	rate int
}

func newTestRC() (*RateController, *[]rateSend) {
	sent := &[]rateSend{}
	rc := NewRateController(func(mac string, rateHz int) {
		*sent = append(*sent, rateSend{mac, rateHz})
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
