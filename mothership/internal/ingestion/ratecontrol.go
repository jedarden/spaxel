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
	RateActive = 50
	// idleTimeout is how long after the last motion event before dropping back to idle.
	idleTimeout = 30 * time.Second

	// DefaultVarianceThreshold is the on-device amplitude variance threshold sent to
	// nodes in idle mode. When the device's local variance exceeds this value it sends
	// a motion_hint, allowing the mothership to ramp the rate before the next server-side
	// detection frame arrives.
	DefaultVarianceThreshold = 1.0
)

// nodeRateState tracks the adaptive rate state for a single node.
type nodeRateState struct {
	active       bool
	lastMotionAt time.Time
}

// RateController manages per-node adaptive sensing rates. When motion is detected
// on a node's link, it ramps that node to RateActive (50 Hz). When no motion has
// been seen for idleTimeout, it drops back to RateIdle (2 Hz). The caller provides
// a configSender callback that sends the rate and variance threshold to the node.
type RateController struct {
	mu            sync.Mutex
	nodes         map[string]*nodeRateState // keyed by node MAC
	configSender  func(nodeMAC string, rateHz int, varianceThreshold float64)
	adjacentNodes func(nodeMAC string) []string // returns MACs of adjacent nodes; may be nil
}

// NewRateController creates a RateController. configSender is called whenever a
// node's rate should change; it must be goroutine-safe. varianceThreshold is sent
// in idle-mode configs so the device can fast-path motion detection locally.
func NewRateController(configSender func(nodeMAC string, rateHz int, varianceThreshold float64)) *RateController {
	return &RateController{
		nodes:        make(map[string]*nodeRateState),
		configSender: configSender,
	}
}

// SetAdjacentNodesFn configures a callback that returns the MACs of nodes
// adjacent to a given node. When set, OnMotionHint preemptively ramps adjacent
// nodes to RateActive so they are already at full rate when the motion front
// arrives.
func (rc *RateController) SetAdjacentNodesFn(fn func(nodeMAC string) []string) {
	rc.mu.Lock()
	rc.adjacentNodes = fn
	rc.mu.Unlock()
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
		rc.configSender(nodeMAC, RateActive, 0) // active: disable on-device hint, server handles it
	}
}

// OnMotionHint is called when the ESP32 sends a motion_hint message (on-device
// variance exceeded threshold). Ramps the hinting node and any adjacent nodes
// so they are already at full rate when the motion front arrives.
func (rc *RateController) OnMotionHint(nodeMAC string) {
	rc.OnMotionState(nodeMAC, true)

	rc.mu.Lock()
	adjFn := rc.adjacentNodes
	rc.mu.Unlock()

	if adjFn != nil {
		for _, adjMAC := range adjFn(nodeMAC) {
			rc.OnMotionState(adjMAC, true)
		}
	}
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
			rc.configSender(mac, RateIdle, DefaultVarianceThreshold) // idle: enable on-device hint
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
