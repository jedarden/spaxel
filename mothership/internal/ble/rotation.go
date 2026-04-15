// Package ble provides BLE address rotation detection and alias management.
// Modern smartphones rotate their BLE random MAC address every 15-30 minutes.
// This module implements heuristics to track these rotations and maintain identity continuity.
package ble

import (
	"log"
	"math"
	"sync"
	"time"
)

// Rotation constants per specification
const (
	// Rotation detection windows
	RotationTimeWindow      = 90 * time.Second  // Time window to look for rotation patterns
	RotationRSSIThreshold   = 10                // RSSI similarity threshold (dBm)
	RotationMinScore        = 0.7               // Minimum score to consider a rotation match
	RotationConfirmCount    = 3                 // Consecutive confirmations needed to merge
	RotationGracePeriod     = 5 * time.Minute   // Grace period before clearing identity
	RotationStaleThreshold  = 5 * time.Minute   // Time after which an alias is considered stale

	// Scoring weights for rotation detection
	WeightManufacturerMatch = 0.50  // Manufacturer data fingerprint
	WeightRSSIProximity     = 0.35  // Time + RSSI proximity
	WeightTimeGap           = 0.15  // Time gap factor
)

// RotationCandidate represents a possible address rotation match.
type RotationCandidate struct {
	OldAddr      string    // Original MAC address
	NewAddr      string    // Newly appeared MAC address
	Score        float64   // Combined rotation score (0-1)
	Reason       string    // Human-readable reason
	FirstSeen    time.Time // When new address was first observed
	ConfirmCount int       // Number of consecutive confirmations
	Confirmed    bool      // True if rotation has been confirmed
}

// RotationDetector implements BLE address rotation detection heuristics.
type RotationDetector struct {
	registry *Registry
	rssiCache *RSSICache

	mu              sync.RWMutex
	candidates      map[string]*RotationCandidate // canonical_addr -> candidate
	rotationHistory  map[string][]string          // canonical_addr -> list of rotated addresses
	lastCheck        time.Time
	gracePeriodExpiries map[string]time.Time     // canonical_addr -> identity expiry
}

// NewRotationDetector creates a new rotation detector.
func NewRotationDetector(registry *Registry, rssiCache *RSSICache) *RotationDetector {
	return &RotationDetector{
		registry:              registry,
		rssiCache:             rssiCache,
		candidates:            make(map[string]*RotationCandidate),
		rotationHistory:       make(map[string][]string),
		gracePeriodExpiries:   make(map[string]time.Time),
		lastCheck:             time.Now(),
	}
}

// ProcessObservations processes new BLE observations and detects potential rotations.
// Should be called whenever new BLE scan results arrive from nodes.
func (r *RotationDetector) ProcessObservations(observations map[string][]*RSSIObservation) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()

	// Get all registered devices to check for disappeared addresses
	devices, err := r.registry.GetDevices(false)
	if err != nil {
		log.Printf("[WARN] ble: failed to get devices for rotation detection: %v", err)
		return
	}

	// Build map of currently visible addresses
	visibleAddrs := make(map[string]bool)
	for addr := range observations {
		visibleAddrs[addr] = true
	}

	// Check for disappeared devices that might have rotated
	for _, dev := range devices {
		if dev.PersonID == "" {
			continue // Only track devices assigned to people
		}

		// Skip if device is currently visible (no rotation)
		if visibleAddrs[dev.Addr] {
			continue
		}

		// Check if there's a grace period expiry for this device's identity
		if expiry, exists := r.gracePeriodExpiries[dev.Addr]; exists && now.Before(expiry) {
			continue // Identity still within grace period
		}

		// Look for new addresses that might be the rotated version
		r.detectRotationForDevice(dev, observations, now)
	}

	// Confirm or age out candidates
	r.updateCandidates(now)

	r.lastCheck = now
}

// detectRotationForDevice looks for rotated addresses for a specific device.
func (r *RotationDetector) detectRotationForDevice(dev DeviceRecord, observations map[string][]*RSSIObservation, now time.Time) {
	// Get recent RSSI history for the disappeared device
	oldReadings := r.rssiCache.GetRecent(dev.Addr, RotationTimeWindow)
	if len(oldReadings) == 0 {
		return // No recent data to compare
	}

	// For each newly visible address, check if it's a rotation match
	for newAddr, newReadings := range observations {
		// Skip if this is the same address or already assigned to someone
		if newAddr == dev.Addr {
			continue
		}

		// Check if new address is already assigned to a different person
		newDev, err := r.registry.GetDevice(newAddr)
		if err == nil && newDev.PersonID != "" && newDev.PersonID != dev.PersonID {
			continue // Already assigned to someone else
		}

		// Calculate rotation score
		score, reason := r.calculateRotationScore(dev.Addr, oldReadings, newAddr, newReadings, now)

		if score >= RotationMinScore {
			// Found a potential rotation
			r.recordRotationCandidate(dev.Addr, newAddr, score, reason, now)
		}
	}
}

// calculateRotationScore computes the likelihood that newAddr is a rotation of oldAddr.
// Returns score (0-1) and reason string.
func (r *RotationDetector) calculateRotationScore(oldAddr string, oldReadings []*RSSIObservation, newAddr string, newReadings []*RSSIObservation, now time.Time) (float64, string) {
	var score float64
	var reasons []string

	// Get device records for manufacturer data comparison
	var oldDev, newDev *DeviceRecord
	if r.registry != nil {
		var err error
		oldDev, err = r.registry.GetDevice(oldAddr)
		if err != nil {
			oldDev = &DeviceRecord{Addr: oldAddr}
		}
		newDev, err = r.registry.GetDevice(newAddr)
		if err != nil {
			// New device not in registry yet - that's expected for rotations
			newDev = &DeviceRecord{Addr: newAddr}
		}
	} else {
		oldDev = &DeviceRecord{Addr: oldAddr}
		newDev = &DeviceRecord{Addr: newAddr}
	}

	// 1. Manufacturer data fingerprint (50% weight)
	mfrScore, mfrReason := r.compareManufacturerData(oldDev, newDev)
	score += mfrScore * WeightManufacturerMatch
	if mfrReason != "" {
		reasons = append(reasons, mfrReason)
	}

	// 2. Time + RSSI proximity (35% weight)
	rssiScore, rssiReason := r.compareRSSIProximity(oldReadings, newReadings, now)
	score += rssiScore * WeightRSSIProximity
	if rssiReason != "" {
		reasons = append(reasons, rssiReason)
	}

	// 3. Time gap factor (15% weight)
	timeScore := r.calculateTimeGapScore(oldReadings, newReadings, now)
	score += timeScore * WeightTimeGap

	// Build reason string
	reason := "rotation detected: "
	if len(reasons) > 0 {
		for i, r := range reasons {
			if i > 0 {
				reason += ", "
			}
			reason += r
		}
	} else {
		reason += "multiple matching factors"
	}

	return math.Min(1.0, score), reason
}

// compareManufacturerData compares manufacturer data between two devices.
func (r *RotationDetector) compareManufacturerData(oldDev, newDev *DeviceRecord) (float64, string) {
	// Check manufacturer ID match
	if oldDev.MfrID != 0 && newDev.MfrID != 0 {
		if oldDev.MfrID == newDev.MfrID {
			// Same manufacturer - good sign
			return 0.8, "same manufacturer"
		}
		return 0.1, "different manufacturer"
	}

	// For Apple devices (0x004C), use proximity UUID fingerprint
	if oldDev.MfrID == 0x004C && len(oldDev.MfrDataHex) >= 12 && len(newDev.MfrDataHex) >= 12 {
		// Compare first 6 bytes (12 hex chars) after company ID
		oldFingerprint := oldDev.MfrDataHex[4:16]
		newFingerprint := newDev.MfrDataHex[4:16]

		if oldFingerprint == newFingerprint {
			return 1.0, "Apple proximity UUID match"
		}
	}

	// Generic manufacturer data comparison
	if len(oldDev.MfrDataHex) >= 12 && len(newDev.MfrDataHex) >= 12 {
		if oldDev.MfrDataHex[:12] == newDev.MfrDataHex[:12] {
			return 0.7, "similar manufacturer data"
		}
	}

	// Device name comparison
	if oldDev.DeviceName != "" && newDev.DeviceName != "" {
		if oldDev.DeviceName == newDev.DeviceName {
			return 0.6, "same device name"
		}
	}

	return 0, ""
}

// compareRSSIProximity compares RSSI readings for temporal and signal proximity.
func (r *RotationDetector) compareRSSIProximity(oldReadings, newReadings []*RSSIObservation, now time.Time) (float64, string) {
	if len(oldReadings) == 0 || len(newReadings) == 0 {
		return 0, ""
	}

	// Find most recent old reading and oldest new reading
	var mostRecentOld *RSSIObservation
	for _, r := range oldReadings {
		if mostRecentOld == nil || r.Timestamp.After(mostRecentOld.Timestamp) {
			mostRecentOld = r
		}
	}

	var oldestNew *RSSIObservation
	for _, r := range newReadings {
		if oldestNew == nil || r.Timestamp.Before(oldestNew.Timestamp) {
			oldestNew = r
		}
	}

	if mostRecentOld == nil || oldestNew == nil {
		return 0, ""
	}

	// Calculate time gap
	timeGap := oldestNew.Timestamp.Sub(mostRecentOld.Timestamp)

	// Time factor: smaller gap = higher score
	timeScore := 1.0
	if timeGap < 0 {
		timeGap = -timeGap
	}
	if timeGap > RotationTimeWindow {
		// Too much time gap - unlikely to be rotation
		timeScore = 0.1
	} else {
		// Linear decay from 1.0 to 0.5 over the window
		timeScore = 1.0 - (0.5 * float64(timeGap) / float64(RotationTimeWindow))
	}

	// Check for same-node observations (strongest signal)
	rssiScore := 0.0
	nodeMatches := 0

	// Look for RSSI similarity at the same node
	for _, oldR := range oldReadings {
		for _, newR := range newReadings {
			if oldR.NodeMAC == newR.NodeMAC {
				rssiDiff := math.Abs(float64(oldR.RSSIdBm - newR.RSSIdBm))
				if rssiDiff <= RotationRSSIThreshold {
					nodeMatches++
					// RSSI similarity score
					rssiSimilarity := 1.0 - (rssiDiff / float64(RotationRSSIThreshold))
					rssiScore = math.Max(rssiScore, rssiSimilarity)
				}
			}
		}
	}

	// Combined score
	finalScore := timeScore * 0.4
	if nodeMatches > 0 {
		finalScore += rssiScore * 0.6
	}

	reason := ""
	if nodeMatches > 0 {
		reason = "similar RSSI at same node"
	} else if timeGap < 30*time.Second {
		reason = "appeared quickly after disappearance"
	}

	return finalScore, reason
}

// calculateTimeGapScore computes a score based on the time between disappearance and appearance.
func (r *RotationDetector) calculateTimeGapScore(oldReadings, newReadings []*RSSIObservation, now time.Time) float64 {
	if len(oldReadings) == 0 || len(newReadings) == 0 {
		return 0.5 // Neutral score
	}

	// Find time gaps
	var lastOldTime, firstNewTime time.Time
	for _, r := range oldReadings {
		if r.Timestamp.After(lastOldTime) {
			lastOldTime = r.Timestamp
		}
	}
	for _, r := range newReadings {
		if firstNewTime.IsZero() || r.Timestamp.Before(firstNewTime) {
			firstNewTime = r.Timestamp
		}
	}

	if lastOldTime.IsZero() || firstNewTime.IsZero() {
		return 0.5
	}

	gap := firstNewTime.Sub(lastOldTime)
	if gap < 0 {
		gap = -gap
	}

	// Optimal gap is 0-90 seconds (typical rotation window)
	// Score 1.0 for gaps < 30s, decaying to 0.2 for gaps > 180s
	if gap < 30*time.Second {
		return 1.0
	}
	if gap > 180*time.Second {
		return 0.2
	}

	// Linear decay from 1.0 to 0.2
	return 1.0 - (0.8 * float64(gap-30*time.Second) / float64(150*time.Second))
}

// recordRotationCandidate records a potential rotation match.
func (r *RotationDetector) recordRotationCandidate(oldAddr, newAddr string, score float64, reason string, now time.Time) {
	// Use oldAddr as the canonical key
	candidate, exists := r.candidates[oldAddr]

	if !exists {
		candidate = &RotationCandidate{
			OldAddr:   oldAddr,
			NewAddr:   newAddr,
			Score:     score,
			Reason:    reason,
			FirstSeen: now,
		}
		r.candidates[oldAddr] = candidate
		log.Printf("[INFO] ble: rotation detected: %s -> %s (score: %.2f, reason: %s)", oldAddr, newAddr, score, reason)
		return
	}

	// Update existing candidate
	if candidate.NewAddr == newAddr {
		candidate.ConfirmCount++
		candidate.Score = math.Max(candidate.Score, score) // Keep best score

		// Check if we have enough confirmations
		if candidate.ConfirmCount >= RotationConfirmCount && !candidate.Confirmed {
			candidate.Confirmed = true
			r.confirmRotation(candidate)
		}
	} else {
		// Different new address - replace candidate
		candidate.NewAddr = newAddr
		candidate.Score = score
		candidate.Reason = reason
		candidate.FirstSeen = now
		candidate.ConfirmCount = 1
		candidate.Confirmed = false
	}
}

// confirmRotation confirms a rotation and updates the registry.
func (r *RotationDetector) confirmRotation(candidate *RotationCandidate) {
	log.Printf("[INFO] ble: rotation confirmed: %s -> %s (after %d confirmations)",
		candidate.OldAddr, candidate.NewAddr, candidate.ConfirmCount)

	// Add alias to registry
	if err := r.registry.AddAlias(candidate.OldAddr, candidate.NewAddr); err != nil {
		log.Printf("[WARN] ble: failed to add alias: %v", err)
		return
	}

	// Update rotation history
	r.rotationHistory[candidate.OldAddr] = append(r.rotationHistory[candidate.OldAddr], candidate.NewAddr)

	// Set grace period for identity continuity
	r.gracePeriodExpiries[candidate.OldAddr] = time.Now().Add(RotationGracePeriod)

	// Remove from candidates
	delete(r.candidates, candidate.OldAddr)
}

// updateCandidates ages out or removes stale candidates.
func (r *RotationDetector) updateCandidates(now time.Time) {
	for key, candidate := range r.candidates {
		// Age out candidates that are too old or haven't been confirmed
		if now.Sub(candidate.FirstSeen) > RotationTimeWindow*2 {
			delete(r.candidates, key)
			continue
		}

		// Reset confirmation count if we haven't seen the new address recently
		lastSeen, err := r.registry.GetDeviceLastSeen(candidate.NewAddr)
		if err != nil || now.Sub(lastSeen) > RotationTimeWindow {
			candidate.ConfirmCount = 0
		}
	}

	// Clean up expired grace periods
	for key, expiry := range r.gracePeriodExpiries {
		if now.After(expiry) {
			delete(r.gracePeriodExpiries, key)
		}
	}
}

// GetCandidates returns all active rotation candidates.
func (r *RotationDetector) GetCandidates() []*RotationCandidate {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*RotationCandidate
	for _, c := range r.candidates {
		result = append(result, c)
	}
	return result
}

// GetRotationHistory returns the rotation history for a canonical address.
func (r *RotationDetector) GetRotationHistory(canonicalAddr string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if history, ok := r.rotationHistory[canonicalAddr]; ok {
		result := make([]string, len(history))
		copy(result, history)
		return result
	}
	return nil
}

// GetGracePeriodExpiry returns the grace period expiry time for a device.
func (r *RotationDetector) GetGracePeriodExpiry(addr string) (time.Time, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	expiry, exists := r.gracePeriodExpiries[addr]
	return expiry, exists
}

// ExtendGracePeriod extends the grace period for a device's identity.
func (r *RotationDetector) ExtendGracePeriod(canonicalAddr string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.gracePeriodExpiries[canonicalAddr] = time.Now().Add(RotationGracePeriod)
}

// IsWithinGracePeriod returns true if the device's identity is within the grace period.
func (r *RotationDetector) IsWithinGracePeriod(canonicalAddr string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if expiry, exists := r.gracePeriodExpiries[canonicalAddr]; exists {
		return time.Now().Before(expiry)
	}
	return false
}
