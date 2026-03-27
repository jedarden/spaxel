package ingestion

import (
	"context"
	"sync"
	"time"
)

const (
	// RateIdle is the CSI sampling rate (Hz) when no motion is detected.
	RateIdle = 2
	// RateActive is the CSI sampling rate (Hz) when motion is detected.
	RateActive = 20
	// idleTimeout is how long after the last motion event before dropping back to idle.
	idleTimeout = 10 * time.Second
)

// nodeRateState tracks the adaptive rate state for a single node.
type nodeRateState struct {
	active       bool
	lastMotionAt time.Time
}

// RateController manages per-node adaptive sensing rates. When motion is detected
// on a node's link, it ramps that node to RateActive (20 Hz). When no motion has
// been seen for idleTimeout, it drops back to RateIdle (2 Hz). The caller provides
// a configSender callback that sends the rate command to the node over WebSocket.
type RateController struct {
	mu           sync.Mutex
	nodes        map[string]*nodeRateState // keyed by node MAC
	configSender func(nodeMAC string, rateHz int)
}

// NewRateController creates a RateController. configSender is called whenever a
// node's rate should change; it must be goroutine-safe.
func NewRateController(configSender func(nodeMAC string, rateHz int)) *RateController {
	return &RateController{
		nodes:        make(map[string]*nodeRateState),
		configSender: configSender,
	}
}

// OnMotionState is called after each CSI frame is processed. If the node was idle
// and motion is now detected, it ramps up immediately.
func (rc *RateController) OnMotionState(nodeMAC string, motionDetected bool) {
	if !motionDetected {
		return
	}

	rc.mu.Lock()
	defer rc.mu.Unlock()

	ns := rc.getOrCreate(nodeMAC)
	ns.lastMotionAt = time.Now()

	if !ns.active {
		ns.active = true
		rc.configSender(nodeMAC, RateActive)
	}
}

// OnMotionHint is called when the ESP32 sends a motion_hint message (on-device
// variance exceeded threshold). Treated identically to a detected motion event.
func (rc *RateController) OnMotionHint(nodeMAC string) {
	rc.OnMotionState(nodeMAC, true)
}

// OnNodeDisconnected removes rate state for a disconnected node.
func (rc *RateController) OnNodeDisconnected(nodeMAC string) {
	rc.mu.Lock()
	delete(rc.nodes, nodeMAC)
	rc.mu.Unlock()
}

// Run starts the background goroutine that enforces idle timeouts.
// It returns when ctx is cancelled.
func (rc *RateController) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rc.checkIdleTimeouts()
		}
	}
}

func (rc *RateController) checkIdleTimeouts() {
	now := time.Now()

	rc.mu.Lock()
	defer rc.mu.Unlock()

	for mac, ns := range rc.nodes {
		if ns.active && now.Sub(ns.lastMotionAt) >= idleTimeout {
			ns.active = false
			rc.configSender(mac, RateIdle)
		}
	}
}

func (rc *RateController) getOrCreate(nodeMAC string) *nodeRateState {
	if ns, ok := rc.nodes[nodeMAC]; ok {
		return ns
	}
	ns := &nodeRateState{}
	rc.nodes[nodeMAC] = ns
	return ns
}
