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
	// RateSentinel is the CSI sampling rate (Hz) for the designated sentinel link in an idle zone.
	// Runs at 5 Hz to maintain minimal coverage while reducing bandwidth.
	RateSentinel = 5
	// RateFleetIdle is the CSI sampling rate (Hz) for non-sentinel links when the entire fleet is idle.
	// Only one sentinel per zone runs at RateSentinel; all others run at 1 Hz.
	RateFleetIdle = 1
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

// zoneRateState tracks the adaptive rate state for a single zone.
type zoneRateState struct {
	zoneID          string
	nodes           map[string]bool // nodes in this zone (keyed by MAC)
	sentinelNodeMAC string          // designated sentinel node (may be empty)
	allNodesIdle    bool            // true when all nodes in zone are idle
	lastMotionAt    time.Time       // last motion in this zone
}

// RateController manages per-node adaptive sensing rates. When motion is detected
// on a node's link, it ramps that node to RateActive (50 Hz). When no motion has
// been seen for idleTimeout, it drops back to RateIdle (2 Hz). The caller provides
// a configSender callback that sends the rate and variance threshold to the node.
//
// Zone-aware mode: When SetZoneMembershipFn is called, the controller also tracks
// zone-level idle states and designates sentinel links per zone at RateSentinel (5 Hz)
// when the fleet is idle, with all other links at RateFleetIdle (1 Hz).
type RateController struct {
	mu            sync.Mutex
	nodes         map[string]*nodeRateState // keyed by node MAC
	configSender  func(nodeMAC string, rateHz int, varianceThreshold float64)
	adjacentNodes func(nodeMAC string) []string // returns MACs of adjacent nodes; may be nil

	// Zone-aware fields
	zones                  map[string]*zoneRateState // keyed by zone ID
	zoneMembership         func(nodeMAC string) []string // returns zone IDs for a node
	adjacentZones          func(zoneID string) []string // returns zone IDs adjacent to a zone
	fleetIdle              bool // true when all zones are idle
}

// NewRateController creates a RateController. configSender is called whenever a
// node's rate should change; it must be goroutine-safe. varianceThreshold is sent
// in idle-mode configs so the device can fast-path motion detection locally.
func NewRateController(configSender func(nodeMAC string, rateHz int, varianceThreshold float64)) *RateController {
	return &RateController{
		nodes:        make(map[string]*nodeRateState),
		zones:        make(map[string]*zoneRateState),
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

// SetZoneMembershipFn configures a callback that returns the zone IDs for a given node.
// When set, the controller tracks zone-level idle states and designates sentinel links.
// The function should return zone IDs based on the node's position (X, Y, Z) within zone bounds.
func (rc *RateController) SetZoneMembershipFn(fn func(nodeMAC string) []string) {
	rc.mu.Lock()
	rc.zoneMembership = fn
	rc.mu.Unlock()
}

// SetAdjacentZonesFn configures a callback that returns zone IDs adjacent to a given zone.
// When set, OnMotionState preemptively ramps adjacent zones to RateSentinel when motion
// is detected in a zone. The function should return zone IDs based on portal connections.
func (rc *RateController) SetAdjacentZonesFn(fn func(zoneID string) []string) {
	rc.mu.Lock()
	rc.adjacentZones = fn
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

	// Zone-aware mode: update zones this node belongs to
	if rc.zoneMembership != nil {
		now := time.Now()
		zoneIDs := rc.zoneMembership(nodeMAC)
		for _, zoneID := range zoneIDs {
			zone := rc.getOrCreateZone(zoneID)
			zoneWasIdle := zone.allNodesIdle
			zone.allNodesIdle = false
			zone.lastMotionAt = now
			zone.nodes[nodeMAC] = true

			// If zone was idle and is now active, ramp all nodes in zone to active
			// and ramp adjacent zones to sentinel (preemptive coverage)
			if zoneWasIdle {
				rc.activateZone(zoneID)
			}
		}
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
	defer rc.mu.Unlock()

	// Remove from nodes
	delete(rc.nodes, nodeMAC)

	// If zone-aware mode is active, remove from all zones and redesignate sentinels if needed
	if rc.zoneMembership != nil {
		zoneIDs := rc.zoneMembership(nodeMAC)
		for _, zoneID := range zoneIDs {
			if zone, ok := rc.zones[zoneID]; ok {
				delete(zone.nodes, nodeMAC)
				// If this was the sentinel, redesignate
				if zone.sentinelNodeMAC == nodeMAC {
					zone.sentinelNodeMAC = ""
					if zone.allNodesIdle && len(zone.nodes) > 0 {
						rc.designateSentinel(zoneID)
					}
				}
			}
		}
	}
}

// RampZone preemptively ramps all nodes in a zone to active rate.
// Called by the prediction engine when a zone arrival is predicted (P(arrival) > threshold).
// Optionally ramps adjacent zones to sentinel rate for preemptive coverage.
func (rc *RateController) RampZone(zoneID string, rampAdjacent bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	zone := rc.getOrCreateZone(zoneID)
	now := time.Now()

	// Ramp all nodes in zone to active
	for nodeMAC := range zone.nodes {
		ns := rc.getOrCreate(nodeMAC)
		ns.active = true
		ns.lastMotionAt = now
		rc.configSender(nodeMAC, RateActive, 0)
	}

	// Update zone state
	zone.allNodesIdle = false
	zone.lastMotionAt = now

	// Optionally ramp adjacent zones to sentinel
	if rampAdjacent && rc.adjacentZones != nil {
		for _, adjZoneID := range rc.adjacentZones(zoneID) {
			rc.rampZoneToSentinel(adjZoneID)
		}
	}
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

	// First, handle per-node idle detection
	for mac, ns := range rc.nodes {
		if ns.active && now.Sub(ns.lastMotionAt) >= idleTimeout {
			ns.active = false
			// If zone-aware mode is active, don't send config here; let zone-level logic handle it
			if rc.zoneMembership == nil {
				rc.configSender(mac, RateIdle, DefaultVarianceThreshold)
			}
		}
	}

	// Then, handle zone-level idle detection and sentinel designation
	if rc.zoneMembership != nil {
		allZonesIdle := true
		now := time.Now()

		for zoneID, zone := range rc.zones {
			// Determine if all nodes in this zone are idle
			allNodesIdle := true
			oldestMotion := time.Unix(0, 0)

			for nodeMAC := range zone.nodes {
				ns := rc.nodes[nodeMAC]
				if ns.active {
					allNodesIdle = false
					break
				}
				if ns.lastMotionAt.After(oldestMotion) {
					oldestMotion = ns.lastMotionAt
				}
			}

			// Check if zone has been idle for timeout duration
			zoneIdle := allNodesIdle && (!oldestMotion.IsZero() && now.Sub(oldestMotion) >= idleTimeout)

			// Zone transition: active -> idle
			if zoneIdle && !zone.allNodesIdle {
				zone.allNodesIdle = true
				zone.lastMotionAt = oldestMotion
				rc.designateSentinel(zoneID)
			} else if !zoneIdle {
				// Zone is active (or has no nodes with valid timestamps)
				allZonesIdle = false
			}
		}

		// Update fleet idle state
		rc.fleetIdle = allZonesIdle && len(rc.zones) > 0
	}
}

// designateSentinel chooses a sentinel node for an idle zone and adjusts rates.
// The sentinel gets RateSentinel (5 Hz); all other nodes get RateFleetIdle (1 Hz).
func (rc *RateController) designateSentinel(zoneID string) {
	zone := rc.zones[zoneID]
	if len(zone.nodes) == 0 {
		return
	}

	// Find lexicographically smallest MAC as sentinel
	var sentinelMAC string
	for nodeMAC := range zone.nodes {
		if sentinelMAC == "" || nodeMAC < sentinelMAC {
			sentinelMAC = nodeMAC
		}
	}
	zone.sentinelNodeMAC = sentinelMAC

	// Set rates: sentinel at 5 Hz, others at 1 Hz
	for nodeMAC := range zone.nodes {
		rate := RateFleetIdle
		if nodeMAC == sentinelMAC {
			rate = RateSentinel
		}
		ns := rc.nodes[nodeMAC]
		if ns.active {
			// Shouldn't happen if zone is idle, but handle it
			ns.active = false
		}
		rc.configSender(nodeMAC, rate, DefaultVarianceThreshold)
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

// getOrCreateZone retrieves a zone's state, creating it if necessary.
func (rc *RateController) getOrCreateZone(zoneID string) *zoneRateState {
	if zone, ok := rc.zones[zoneID]; ok {
		return zone
	}
	zone := &zoneRateState{
		zoneID:       zoneID,
		nodes:        make(map[string]bool),
		allNodesIdle: true, // start idle; will be marked active on first motion
	}
	rc.zones[zoneID] = zone
	return zone
}

// activateZone ramps all nodes in a zone to active and ramps adjacent zones to sentinel.
// Called when a zone transitions from idle to active (motion detected in an idle zone).
func (rc *RateController) activateZone(zoneID string) {
	// Ramp all nodes in this zone to active
	zone := rc.zones[zoneID]
	for nodeMAC := range zone.nodes {
		ns := rc.getOrCreate(nodeMAC)
		if !ns.active {
			ns.active = true
			ns.lastMotionAt = time.Now()
			rc.configSender(nodeMAC, RateActive, 0)
		}
	}

	// Ramp adjacent zones to sentinel (preemptive coverage)
	if rc.adjacentZones != nil {
		for _, adjZoneID := range rc.adjacentZones(zoneID) {
			rc.rampZoneToSentinel(adjZoneID)
		}
	}
}

// rampZoneToSentinel ramps all nodes in a zone to sentinel rate (5 Hz).
// Used for preemptive coverage of zones adjacent to active zones.
func (rc *RateController) rampZoneToSentinel(zoneID string) {
	zone := rc.getOrCreateZone(zoneID)
	for nodeMAC := range zone.nodes {
		ns := rc.getOrCreate(nodeMAC)
		// Only ramp if node is idle (no need to affect already-active nodes)
		if !ns.active {
			ns.lastMotionAt = time.Now()
			rc.configSender(nodeMAC, RateSentinel, DefaultVarianceThreshold)
		}
	}
}
