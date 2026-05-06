// Package fleet implements TX slot collision detection and adaptive re-stagger
package fleet

import (
	"log"
	"math/rand"
	"sync"
	"time"
)

const (
	// CollisionWindow is the time window within which arrivals are considered a collision (3ms)
	CollisionWindow = 3 * time.Millisecond

	// CollisionRateWindow is the time window over which collision rate is calculated (60 seconds)
	CollisionRateWindow = 60 * time.Second

	// CollisionRateThreshold is the collision rate that triggers re-stagger (5%)
	CollisionRateThreshold = 0.05

	// MaxCollisionHistory is the maximum number of collision events to keep per link pair
	MaxCollisionHistory = 1000
)

// CollisionEvent represents a detected collision between two TX nodes
type CollisionEvent struct {
	Timestamp   time.Time
	TXMAC1      string
	TXMAC2      string
	TimeDeltaMs float64
	LinkID1     string
	LinkID2     string
}

// CollisionStats tracks collision statistics for a pair of TX nodes
type CollisionStats struct {
	TotalFrames  int
	Collisions   int
	LastCollision time.Time
	LastReset     time.Time
}

// CollisionDetector tracks TX slot collisions and triggers re-stagger
type CollisionDetector struct {
	mu sync.RWMutex

	// Per-link-pair collision tracking
	// Key is "tx_mac1:tx_mac2" (sorted lexicographically)
	collisionCount map[string]int
	totalFrames    map[string]int

	// Collision event history with timestamps
	collisionHistory map[string][]CollisionEvent

	// TX node tracking (which nodes are currently in TX mode)
	txNodes map[string]bool

	// Last frame arrival time per TX node
	lastArrival map[string]time.Time

	// Re-stagger state
	lastRestagger time.Time
	restaggersEnabled bool

	// Callback for re-stagger trigger
	onRestagger func()

	// Frame arrival tracking with timestamps for accurate rate calculation
	frameArrivals map[string][]time.Time // txMAC -> list of arrival timestamps
}

// NewCollisionDetector creates a new collision detector
func NewCollisionDetector() *CollisionDetector {
	return &CollisionDetector{
		collisionCount:   make(map[string]int),
		totalFrames:      make(map[string]int),
		collisionHistory: make(map[string][]CollisionEvent),
		txNodes:          make(map[string]bool),
		lastArrival:      make(map[string]time.Time),
		restaggersEnabled: true,
		frameArrivals:    make(map[string][]time.Time),
	}
}

// SetRestaggerCallback sets the callback function to trigger re-stagger
func (cd *CollisionDetector) SetRestaggerCallback(cb func()) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	cd.onRestagger = cb
}

// SetRestaggersEnabled enables or disables adaptive re-stagger
func (cd *CollisionDetector) SetRestaggersEnabled(enabled bool) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	cd.restaggersEnabled = enabled
}

// RegisterTXNode registers a node as a TX transmitter
func (cd *CollisionDetector) RegisterTXNode(mac string) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	cd.txNodes[mac] = true
}

// UnregisterTXNode removes a node from TX tracking
func (cd *CollisionDetector) UnregisterTXNode(mac string) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	delete(cd.txNodes, mac)
	delete(cd.lastArrival, mac)
	delete(cd.frameArrivals, mac)
}

// RecordFrameArrival records the arrival of a CSI frame from a TX node
// linkID is in the format "tx_mac:rx_mac"
func (cd *CollisionDetector) RecordFrameArrival(txMAC, linkID string, recvTime time.Time) {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	// Only track frames from TX nodes
	if !cd.txNodes[txMAC] {
		return
	}

	cd.lastArrival[txMAC] = recvTime

	// Track frame arrival with timestamp for rate calculation
	cd.frameArrivals[txMAC] = append(cd.frameArrivals[txMAC], recvTime)

	// Clean up old frame arrivals (older than 2x the collision rate window to save memory)
	cutoff := recvTime.Add(-2 * CollisionRateWindow)
	for mac := range cd.frameArrivals {
		cleaned := cd.frameArrivals[mac][:0]
		for _, ts := range cd.frameArrivals[mac] {
			if ts.After(cutoff) {
				cleaned = append(cleaned, ts)
			}
		}
		cd.frameArrivals[mac] = cleaned
	}

	// Check for collisions with all other TX nodes
	for otherTXMAC, otherArrival := range cd.lastArrival {
		if otherTXMAC == txMAC {
			continue
		}

		// Calculate time difference
		timeDelta := recvTime.Sub(otherArrival)
		if timeDelta < 0 {
			timeDelta = -timeDelta
		}

		// Check if within collision window
		if timeDelta <= CollisionWindow && timeDelta > 0 {
			// Only record collision if time delta is positive (avoid self-collision)
			cd.recordCollision(txMAC, otherTXMAC, linkID, "", recvTime, float64(timeDelta.Milliseconds()))
		}
	}

	// Update total frame count for all pairs involving this TX
	for otherTXMAC := range cd.txNodes {
		if otherTXMAC == txMAC {
			continue
		}

		pairKey := cd.pairKey(txMAC, otherTXMAC)
		cd.totalFrames[pairKey]++
	}
}

// recordCollision records a collision event
func (cd *CollisionDetector) recordCollision(txMAC1, txMAC2, linkID1, linkID2 string, timestamp time.Time, timeDeltaMs float64) {
	pairKey := cd.pairKey(txMAC1, txMAC2)
	cd.collisionCount[pairKey]++

	event := CollisionEvent{
		Timestamp:   timestamp,
		TXMAC1:      txMAC1,
		TXMAC2:      txMAC2,
		TimeDeltaMs: timeDeltaMs,
		LinkID1:     linkID1,
		LinkID2:     linkID2,
	}

	cd.collisionHistory[pairKey] = append(cd.collisionHistory[pairKey], event)

	// Trim history
	if len(cd.collisionHistory[pairKey]) > MaxCollisionHistory {
		cd.collisionHistory[pairKey] = cd.collisionHistory[pairKey][1:]
	}

	// Log collision event
	log.Printf("[INFO] collision: TX slot collision detected between %s and %s (time delta: %.2f ms)", txMAC1, txMAC2, timeDeltaMs)
}

// pairKey generates a consistent key for a TX pair (lexicographically sorted)
func (cd *CollisionDetector) pairKey(mac1, mac2 string) string {
	if mac1 < mac2 {
		return mac1 + ":" + mac2
	}
	return mac2 + ":" + mac1
}

// GetCollisionRate returns the collision rate over the specified window
func (cd *CollisionDetector) GetCollisionRate(window time.Duration) float64 {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	if len(cd.frameArrivals) == 0 {
		return 0
	}

	now := time.Now()
	cutoff := now.Add(-window)

	totalCollisions := 0
	totalFrameOpportunities := 0

	// Count total frame opportunities from all TX nodes in the window
	for _, arrivals := range cd.frameArrivals {
		framesInWindow := 0
		for _, ts := range arrivals {
			if ts.After(cutoff) {
				framesInWindow++
			}
		}

		// Each frame from a TX node represents a collision opportunity with every other TX node
		// So total opportunities = frames from this TX * (number of other TX nodes)
		otherTXCount := len(cd.txNodes) - 1
		if otherTXCount > 0 {
			totalFrameOpportunities += framesInWindow * otherTXCount
		}
	}

	// Count collisions in the window
	for _, events := range cd.collisionHistory {
		for _, event := range events {
			if event.Timestamp.After(cutoff) {
				totalCollisions++
			}
		}
	}

	if totalFrameOpportunities == 0 {
		return 0
	}

	return float64(totalCollisions) / float64(totalFrameOpportunities)
}

// GetCollisionRateByPair returns collision statistics for each TX pair
func (cd *CollisionDetector) GetCollisionRateByPair() map[string]CollisionStats {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	stats := make(map[string]CollisionStats)
	now := time.Now()
	cutoff := now.Add(-CollisionRateWindow)

	for pairKey, events := range cd.collisionHistory {
		stat := CollisionStats{
			LastReset: now,
		}

		// Count collisions in the window
		for _, event := range events {
			if event.Timestamp.After(cutoff) {
				stat.Collisions++
				if stat.LastCollision.Before(event.Timestamp) {
					stat.LastCollision = event.Timestamp
				}
			}
		}

		stat.TotalFrames = cd.totalFrames[pairKey]
		stats[pairKey] = stat
	}

	return stats
}

// CheckAndTriggerRestagger checks if the collision rate exceeds the threshold
// and triggers re-stagger if needed. Returns true if re-stagger was triggered.
func (cd *CollisionDetector) CheckAndTriggerRestagger() bool {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	if !cd.restaggersEnabled {
		return false
	}

	// Rate-limit re-staggers to at most once per 30 seconds
	if time.Since(cd.lastRestagger) < 30*time.Second {
		return false
	}

	rate := cd.getCollisionRateLocked()

	if rate > CollisionRateThreshold {
		log.Printf("[WARN] collision: Collision rate %.2f%% exceeds threshold %.2f%%, triggering adaptive re-stagger",
			rate*100, CollisionRateThreshold*100)

		cd.lastRestagger = time.Now()

		// Trigger re-stagger callback if set
		if cd.onRestagger != nil {
			go cd.onRestagger()
		}

		return true
	}

	return false
}

// getCollisionRateLocked calculates collision rate (caller must hold lock)
func (cd *CollisionDetector) getCollisionRateLocked() float64 {
	now := time.Now()
	cutoff := now.Add(-CollisionRateWindow)

	totalCollisions := 0
	totalFrameOpportunities := 0

	// Count total frame opportunities from all TX nodes in the window
	for _, arrivals := range cd.frameArrivals {
		framesInWindow := 0
		for _, ts := range arrivals {
			if ts.After(cutoff) {
				framesInWindow++
			}
		}

		// Each frame from a TX node represents a collision opportunity with every other TX node
		// So total opportunities = frames from this TX * (number of other TX nodes)
		otherTXCount := len(cd.txNodes) - 1
		if otherTXCount > 0 {
			totalFrameOpportunities += framesInWindow * otherTXCount
		}
	}

	// Count collisions in the window
	for _, events := range cd.collisionHistory {
		for _, event := range events {
			if event.Timestamp.After(cutoff) {
				totalCollisions++
			}
		}
	}

	if totalFrameOpportunities == 0 {
		return 0
	}

	return float64(totalCollisions) / float64(totalFrameOpportunities)
}

// ResetCounters resets all collision counters
func (cd *CollisionDetector) ResetCounters() {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	cd.collisionCount = make(map[string]int)
	cd.totalFrames = make(map[string]int)
	cd.collisionHistory = make(map[string][]CollisionEvent)
	cd.frameArrivals = make(map[string][]time.Time)
	cd.lastRestagger = time.Now()

	log.Printf("[INFO] collision: Collision counters reset")
}

// GetCollisionHistory returns recent collision events for all pairs
func (cd *CollisionDetector) GetCollisionHistory(limit int) []CollisionEvent {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	var allEvents []CollisionEvent
	for _, events := range cd.collisionHistory {
		allEvents = append(allEvents, events...)
	}

	// Sort by timestamp descending
	for i := 0; i < len(allEvents); i++ {
		for j := i + 1; j < len(allEvents); j++ {
			if allEvents[j].Timestamp.After(allEvents[i].Timestamp) {
				allEvents[i], allEvents[j] = allEvents[j], allEvents[i]
			}
		}
	}

	if limit > 0 && len(allEvents) > limit {
		allEvents = allEvents[:limit]
	}

	return allEvents
}

// GenerateRestaggerOffset generates a new random stagger offset for a TX node
// This shifts one node's slot by a fraction of the slot width to break up collisions
func GenerateRestaggerOffset(txIndex, numTXNodes int, rateHz int) int {
	// Calculate slot width in microseconds
	slotWidthUS := 1_000_000 / (rateHz * numTXNodes)

	// Shift by a random fraction of the slot width (between 10% and 90%)
	// This ensures nodes don't all align at the same time
	fraction := 0.1 + rand.Float64()*0.8
	shiftUS := int(float64(slotWidthUS) * fraction)

	// Calculate the base offset for this TX node
	baseOffsetUS := txIndex * (1_000_000 / (rateHz * numTXNodes))

	// Apply the shift
	newOffsetUS := baseOffsetUS + shiftUS

	// Ensure we don't exceed the period
	maxOffsetUS := 1_000_000 / rateHz
	if newOffsetUS >= maxOffsetUS {
		newOffsetUS = newOffsetUS % maxOffsetUS
	}

	return newOffsetUS
}

// GetRestaggerOffsets generates new stagger offsets for all TX nodes
// Returns a map of MAC address to tx_slot_us value
func GetRestaggerOffsets(txMACs []string, rateHz int) map[string]int {
	numTX := len(txMACs)
	offsets := make(map[string]int)

	for i, mac := range txMACs {
		offsets[mac] = GenerateRestaggerOffset(i, numTX, rateHz)
	}

	return offsets
}
