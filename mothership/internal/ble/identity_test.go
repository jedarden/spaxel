package ble

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// mockNodePositionAccessor implements NodePositionAccessor for testing
type mockNodePositionAccessor struct {
	positions map[string][3]float64
}

func (m *mockNodePositionAccessor) GetNodePosition(mac string) (x, y, z float64, ok bool) {
	if pos, exists := m.positions[mac]; exists {
		return pos[0], pos[1], pos[2], true
	}
	return 0, 0, 0, false
}

// ─── RSSI to Distance Conversion Tests ─────────────────────────────────────────────

func TestRSSIToDistance(t *testing.T) {
	tests := []struct {
		name     string
		rssi     int
		expected float64
		tolerance float64
	}{
		{
			name:      "RSSI at reference distance (1m)",
			rssi:      -65, // RefRSSI
			expected:  1.0,
			tolerance: 0.01,
		},
		{
			name:      "RSSI -75 dBm should be ~2.5m",
			rssi:      -75,
			expected:  2.5119,
			tolerance: 0.1,
		},
		{
			name:      "RSSI -55 dBm should be ~0.4m",
			rssi:      -55,
			expected:  0.398,
			tolerance: 0.05,
		},
		{
			name:      "RSSI -85 dBm should be ~6.3m",
			rssi:      -85,
			expected:  6.309,
			tolerance: 0.2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rssiToDistance(tt.rssi)
			if math.Abs(result-tt.expected) > tt.tolerance {
				t.Errorf("rssiToDistance(%d) = %.4f, want %.4f (±%.4f)", tt.rssi, result, tt.expected, tt.tolerance)
			}
		})
	}
}

// ─── Triangulation Tests ───────────────────────────────────────────────────────────

func TestTriangulationWithThreeNodes(t *testing.T) {
	// Setup mock node positions (equilateral triangle at known positions)
	mockNodes := &mockNodePositionAccessor{
		positions: map[string][3]float64{
			"node:00:01": {0.0, 1.5, 0.0},   // Origin
			"node:00:02": {4.0, 1.5, 0.0},   // 4m along X
			"node:00:03": {2.0, 1.5, 3.46},  // ~3.46m along Z (equilateral)
		},
	}

	// Create registry and RSSI cache
	reg := setupTestRegistryForIdentity(t)
	defer reg.Close()

	cache := NewRSSICache(30 * time.Second)

	matcher := NewIdentityMatcher(reg, cache, mockNodes)

	// Create a person and device
	person, _ := reg.CreatePerson("Alice", "#3b82f6")
	reg.ProcessRelayMessage("node:00:01", []BLEObservation{
		{Addr: "aa:bb:cc:dd:ee:01", Name: "Alice's Phone", MfrID: 0x004C, RSSIdBm: -65},
	})
	reg.AssignToPerson("aa:bb:cc:dd:ee:01", person.ID)

	// Add RSSI readings that place device at center of triangle (2.0, 1.5, 1.15)
	// Distance from center to each vertex ≈ 2.31m → RSSI ≈ -73 dBm
	now := time.Now()
	cache.AddWithTime("aa:bb:cc:dd:ee:01", "node:00:01", -73, now)
	cache.AddWithTime("aa:bb:cc:dd:ee:01", "node:00:02", -73, now)
	cache.AddWithTime("aa:bb:cc:dd:ee:01", "node:00:03", -73, now)

	// Triangulate
	readings := cache.GetRecent("aa:bb:cc:dd:ee:01", 5*time.Second)
	pos, conf, residual := matcher.triangulate(readings)

	// Verify position error < 0.5m as per spec
	if conf < 0.5 {
		t.Errorf("Expected confidence >= 0.5 for 3 nodes, got %.2f", conf)
	}

	// Expected position is roughly (2.0, 1.5, 1.15)
	distErr := math.Sqrt(math.Pow(pos.X-2.0, 2) + math.Pow(pos.Y-1.5, 2) + math.Pow(pos.Z-1.15, 2))
	if distErr > 0.5 {
		t.Errorf("Position error %.2fm exceeds 0.5m tolerance (got pos: %.2f, %.2f, %.2f)", distErr, pos.X, pos.Y, pos.Z)
	}

	t.Logf("Triangulated position: (%.2f, %.2f, %.2f), confidence: %.2f, residual: %.2f", pos.X, pos.Y, pos.Z, conf, residual)
}

func TestTriangulationWithSingleNode(t *testing.T) {
	mockNodes := &mockNodePositionAccessor{
		positions: map[string][3]float64{
			"node:00:01": {5.0, 1.5, 5.0},
		},
	}

	reg := setupTestRegistryForIdentity(t)
	defer reg.Close()

	cache := NewRSSICache(30 * time.Second)
	matcher := NewIdentityMatcher(reg, cache, mockNodes)

	// Add single RSSI reading
	now := time.Now()
	cache.AddWithTime("aa:bb:cc:dd:ee:01", "node:00:01", -65, now)

	readings := cache.GetRecent("aa:bb:cc:dd:ee:01", 5*time.Second)
	pos, conf, _ := matcher.triangulate(readings)

	// Single node should give low confidence (0.2 per spec)
	if conf != 0.2 {
		t.Errorf("Expected confidence 0.2 for single node, got %.2f", conf)
	}

	// Position should be near the node
	if math.Abs(pos.X-5.0) > 0.1 || math.Abs(pos.Z-5.0) > 0.1 {
		t.Errorf("Single node position should be near node, got (%.2f, %.2f, %.2f)", pos.X, pos.Y, pos.Z)
	}
}

func TestTriangulationWithTwoNodes(t *testing.T) {
	mockNodes := &mockNodePositionAccessor{
		positions: map[string][3]float64{
			"node:00:01": {0.0, 1.5, 0.0},
			"node:00:02": {4.0, 1.5, 0.0},
		},
	}

	reg := setupTestRegistryForIdentity(t)
	defer reg.Close()

	cache := NewRSSICache(30 * time.Second)
	matcher := NewIdentityMatcher(reg, cache, mockNodes)

	// Device at (2.0, 1.5, 0.0) - midpoint between nodes
	// Distance = 2m → RSSI ≈ -72 dBm
	now := time.Now()
	cache.AddWithTime("aa:bb:cc:dd:ee:01", "node:00:01", -72, now)
	cache.AddWithTime("aa:bb:cc:dd:ee:01", "node:00:02", -72, now)

	readings := cache.GetRecent("aa:bb:cc:dd:ee:01", 5*time.Second)
	pos, conf, _ := matcher.triangulate(readings)

	// Two nodes should give confidence 0.5 per spec
	if conf != 0.5 {
		t.Errorf("Expected confidence 0.5 for two nodes, got %.2f", conf)
	}

	// Position should be near midpoint
	if math.Abs(pos.X-2.0) > 1.0 {
		t.Errorf("Two node position should be near midpoint, got X=%.2f", pos.X)
	}
}

// ─── Nearest Blob Assignment Tests ─────────────────────────────────────────────────

func TestNearestBlobAssignment(t *testing.T) {
	mockNodes := &mockNodePositionAccessor{
		positions: map[string][3]float64{
			"node:00:01": {0.0, 1.5, 0.0},
			"node:00:02": {4.0, 1.5, 4.0},
			"node:00:03": {4.0, 1.5, 0.0},
		},
	}

	reg := setupTestRegistryForIdentity(t)
	defer reg.Close()

	cache := NewRSSICache(30 * time.Second)
	matcher := NewIdentityMatcher(reg, cache, mockNodes)

	// Create person and device
	person, _ := reg.CreatePerson("Alice", "#3b82f6")
	reg.ProcessRelayMessage("node:00:01", []BLEObservation{
		{Addr: "aa:bb:cc:dd:ee:01", Name: "Alice's Phone", MfrID: 0x004C, RSSIdBm: -65},
	})
	reg.AssignToPerson("aa:bb:cc:dd:ee:01", person.ID)

	// RSSI readings that triangulate to ~ (1.5, 1.5, 1.0) on the X-Z floor plane.
	// Distances: node1≈1.80m(→-71), node2≈3.91m(→-80), node3≈2.69m(→-76).
	now := time.Now()
	cache.AddWithTime("aa:bb:cc:dd:ee:01", "node:00:01", -71, now)
	cache.AddWithTime("aa:bb:cc:dd:ee:01", "node:00:02", -80, now)
	cache.AddWithTime("aa:bb:cc:dd:ee:01", "node:00:03", -76, now)

	// Two blobs: one near the triangulated position, one far away
	blobs := []struct {
		ID      int
		X, Y, Z float64
		Weight  float64
	}{
		{ID: 1, X: 1.5, Y: 1.5, Z: 1.0, Weight: 0.9}, // Closer to triangulated pos
		{ID: 2, X: 5.0, Y: 1.5, Z: 5.0, Weight: 0.9}, // Far away
	}

	matcher.UpdateBlobs(blobs)

	matches := matcher.GetMatches()
	if len(matches) == 0 {
		t.Fatal("Expected at least one match")
	}

	// Should match blob 1 (at 1.5, 1.5, 1.0) since triangulated position is ~ (1.5, 1.5, 1.0)
	var matchedBlobID int
	for blobID := range matches {
		matchedBlobID = blobID
		break
	}

	if matchedBlobID != 1 {
		t.Errorf("Expected match to blob 1, got blob %d", matchedBlobID)
	}
}

// ─── Confidence Gate Tests ─────────────────────────────────────────────────────────

func TestConfidenceGate(t *testing.T) {
	mockNodes := &mockNodePositionAccessor{
		positions: map[string][3]float64{
			"node:00:01": {0.0, 1.5, 0.0},
		},
	}

	reg := setupTestRegistryForIdentity(t)
	defer reg.Close()

	cache := NewRSSICache(30 * time.Second)
	matcher := NewIdentityMatcher(reg, cache, mockNodes)

	// Create person and device
	person, _ := reg.CreatePerson("Alice", "#3b82f6")
	reg.ProcessRelayMessage("node:00:01", []BLEObservation{
		{Addr: "aa:bb:cc:dd:ee:01", Name: "Alice's Phone", MfrID: 0x004C, RSSIdBm: -65},
	})
	reg.AssignToPerson("aa:bb:cc:dd:ee:01", person.ID)

	// Single node = 0.2 confidence, blob at 1.5m distance
	// f_obs=1.0, f_nodes=0.2, f_residual=1.0, f_dist≈0.33
	// Total confidence ≈ 0.066 < 0.6 threshold
	now := time.Now()
	cache.AddWithTime("aa:bb:cc:dd:ee:01", "node:00:01", -65, now)

	blobs := []struct {
		ID     int
		X, Y, Z float64
		Weight float64
	}{
		{ID: 1, X: 1.5, Y: 1.5, Z: 0.0, Weight: 0.9}, // 1.5m from node
	}

	matcher.UpdateBlobs(blobs)

	matches := matcher.GetMatches()
	if len(matches) != 0 {
		t.Errorf("Expected no match due to low confidence (< 0.6), got %d matches", len(matches))
	}
}

func TestHighConfidenceAssignment(t *testing.T) {
	mockNodes := &mockNodePositionAccessor{
		positions: map[string][3]float64{
			"node:00:01": {0.0, 1.5, 0.0},
			"node:00:02": {4.0, 1.5, 0.0},
			"node:00:03": {2.0, 1.5, 3.5},
		},
	}

	reg := setupTestRegistryForIdentity(t)
	defer reg.Close()

	cache := NewRSSICache(30 * time.Second)
	matcher := NewIdentityMatcher(reg, cache, mockNodes)

	// Create person and device
	person, _ := reg.CreatePerson("Alice", "#3b82f6")
	reg.ProcessRelayMessage("node:00:01", []BLEObservation{
		{Addr: "aa:bb:cc:dd:ee:01", Name: "Alice's Phone", MfrID: 0x004C, RSSIdBm: -65},
	})
	reg.AssignToPerson("aa:bb:cc:dd:ee:01", person.ID)

	// Three nodes, device near blob at (2.0, 1.5, 1.0) — RSSI chosen so distances match.
	// node1 d=2.24m→-74, node2 d=2.24m→-74, node3 d=2.5m→-75
	now := time.Now()
	cache.AddWithTime("aa:bb:cc:dd:ee:01", "node:00:01", -74, now)
	cache.AddWithTime("aa:bb:cc:dd:ee:01", "node:00:02", -74, now)
	cache.AddWithTime("aa:bb:cc:dd:ee:01", "node:00:03", -75, now)

	blobs := []struct {
		ID      int
		X, Y, Z float64
		Weight  float64
	}{
		{ID: 1, X: 2.0, Y: 1.5, Z: 1.0, Weight: 0.9}, // Close to triangulated position
	}

	matcher.UpdateBlobs(blobs)

	matches := matcher.GetMatches()
	if len(matches) == 0 {
		t.Fatal("Expected match with high confidence")
	}

	var match *IdentityMatch
	for _, m := range matches {
		match = m
		break
	}

	if match.Confidence < 0.6 {
		t.Errorf("Expected confidence >= 0.6, got %.2f", match.Confidence)
	}
}

// ─── BLE-Only Placeholder Track Tests ─────────────────────────────────────────────

func TestBLEOnlyPlaceholderTrack(t *testing.T) {
	mockNodes := &mockNodePositionAccessor{
		positions: map[string][3]float64{
			"node:00:01": {0.0, 1.5, 0.0},
			"node:00:02": {4.0, 1.5, 0.0},
			"node:00:03": {2.0, 1.5, 3.5},
		},
	}

	reg := setupTestRegistryForIdentity(t)
	defer reg.Close()

	cache := NewRSSICache(30 * time.Second)
	matcher := NewIdentityMatcher(reg, cache, mockNodes)

	// Create person and device
	person, _ := reg.CreatePerson("Alice", "#3b82f6")
	reg.ProcessRelayMessage("node:00:01", []BLEObservation{
		{Addr: "aa:bb:cc:dd:ee:01", Name: "Alice's Phone", MfrID: 0x004C, RSSIdBm: -65},
	})
	reg.AssignToPerson("aa:bb:cc:dd:ee:01", person.ID)

	// RSSI readings place device at triangulated position
	now := time.Now()
	cache.AddWithTime("aa:bb:cc:dd:ee:01", "node:00:01", -65, now)
	cache.AddWithTime("aa:bb:cc:dd:ee:01", "node:00:02", -65, now)
	cache.AddWithTime("aa:bb:cc:dd:ee:01", "node:00:03", -65, now)

	// Blob is far away (> 2m from triangulated position)
	blobs := []struct {
		ID     int
		X, Y, Z float64
		Weight float64
	}{
		{ID: 1, X: 10.0, Y: 1.5, Z: 10.0, Weight: 0.9}, // Far from BLE position
	}

	matcher.UpdateBlobs(blobs)

	// Should have a BLE-only track
	bleOnlyTracks := matcher.GetBLEOnlyTracks()
	if len(bleOnlyTracks) == 0 {
		t.Error("Expected BLE-only placeholder track when no blob within 2m")
	}

	for personID, track := range bleOnlyTracks {
		if personID != person.ID {
			t.Errorf("Expected person ID %s, got %s", person.ID, personID)
		}
		if !track.IsBLEOnly {
			t.Error("Expected IsBLEOnly to be true")
		}
		if track.PersonName != "Alice" {
			t.Errorf("Expected person name 'Alice', got '%s'", track.PersonName)
		}
		break
	}
}

// ─── Identity Persistence Tests ───────────────────────────────────────────────────

func TestIdentityPersistence(t *testing.T) {
	mockNodes := &mockNodePositionAccessor{
		positions: map[string][3]float64{
			"node:00:01": {0.0, 1.5, 0.0},
			"node:00:02": {4.0, 1.5, 0.0},
			"node:00:03": {2.0, 1.5, 3.5},
		},
	}

	reg := setupTestRegistryForIdentity(t)
	defer reg.Close()

	cache := NewRSSICache(30 * time.Second)
	matcher := NewIdentityMatcher(reg, cache, mockNodes)
	matcher.persistenceTime = 1 * time.Second // Shorten for test

	// Create person and device
	person, _ := reg.CreatePerson("Alice", "#3b82f6")
	reg.ProcessRelayMessage("node:00:01", []BLEObservation{
		{Addr: "aa:bb:cc:dd:ee:01", Name: "Alice's Phone", MfrID: 0x004C, RSSIdBm: -65},
	})
	reg.AssignToPerson("aa:bb:cc:dd:ee:01", person.ID)

	// Establish initial match — RSSI chosen to match distances to blob at (2.0, 1.5, 1.0).
	// node1 d=2.24m→-74, node2 d=2.24m→-74, node3 d=2.5m→-75
	now := time.Now()
	cache.AddWithTime("aa:bb:cc:dd:ee:01", "node:00:01", -74, now)
	cache.AddWithTime("aa:bb:cc:dd:ee:01", "node:00:02", -74, now)
	cache.AddWithTime("aa:bb:cc:dd:ee:01", "node:00:03", -75, now)

	blobs := []struct {
		ID      int
		X, Y, Z float64
		Weight  float64
	}{
		{ID: 1, X: 2.0, Y: 1.5, Z: 1.0, Weight: 0.9},
	}

	matcher.UpdateBlobs(blobs)

	// Verify initial match
	matches := matcher.GetMatches()
	if len(matches) == 0 {
		t.Fatal("Expected initial match")
	}

	// Clear RSSI cache and BLE position cache (simulate BLE device disappearing)
	cache = NewRSSICache(30 * time.Second)
	matcher.rssiCache = cache
	matcher.cachedDevices = nil // force re-triangulation with empty cache

	// Update blobs - identity should persist (from persistentIdent)
	matcher.UpdateBlobs(blobs)

	// Get persistent identity
	persistMatch := matcher.GetPersistentIdentity(1)
	if persistMatch == nil {
		t.Error("Expected persistent identity to exist")
	}

	// Wait for persistence to expire
	time.Sleep(1100 * time.Millisecond)

	// Update again - identity should be cleared (cachedDevices still nil, RSSI cache still empty)
	matcher.cachedDevices = nil
	matcher.UpdateBlobs(blobs)

	persistMatch = matcher.GetPersistentIdentity(1)
	if persistMatch != nil {
		t.Error("Expected persistent identity to expire after timeout")
	}
}

// ─── MAC Rotation / Identity Handoff Tests ─────────────────────────────────────────

func TestIdentityHandoffOnMACRotation(t *testing.T) {
	mockNodes := &mockNodePositionAccessor{
		positions: map[string][3]float64{
			"node:00:01": {0.0, 1.5, 0.0},
			"node:00:02": {4.0, 1.5, 0.0},
			"node:00:03": {2.0, 1.5, 3.5},
		},
	}

	reg := setupTestRegistryForIdentity(t)
	defer reg.Close()

	cache := NewRSSICache(30 * time.Second)
	matcher := NewIdentityMatcher(reg, cache, mockNodes)

	// Create person
	person, _ := reg.CreatePerson("Alice", "#3b82f6")

	// Create two devices (simulating MAC rotation) assigned to same person
	reg.ProcessRelayMessage("node:00:01", []BLEObservation{
		{Addr: "aa:bb:cc:dd:ee:01", Name: "iPhone", MfrID: 0x004C, RSSIdBm: -65},
	})
	reg.ProcessRelayMessage("node:00:01", []BLEObservation{
		{Addr: "aa:bb:cc:dd:ee:02", Name: "iPhone", MfrID: 0x004C, RSSIdBm: -65},
	})
	reg.AssignToPerson("aa:bb:cc:dd:ee:01", person.ID)
	reg.AssignToPerson("aa:bb:cc:dd:ee:02", person.ID)

	// RSSI from new MAC only — chosen to match distances to blob at (2.0, 1.5, 1.0).
	// node1 d=2.24m→-74, node2 d=2.24m→-74, node3 d=2.5m→-75
	now := time.Now()
	cache.AddWithTime("aa:bb:cc:dd:ee:02", "node:00:01", -74, now)
	cache.AddWithTime("aa:bb:cc:dd:ee:02", "node:00:02", -74, now)
	cache.AddWithTime("aa:bb:cc:dd:ee:02", "node:00:03", -75, now)

	blobs := []struct {
		ID      int
		X, Y, Z float64
		Weight  float64
	}{
		{ID: 1, X: 2.0, Y: 1.5, Z: 1.0, Weight: 0.9},
	}

	matcher.UpdateBlobs(blobs)

	// Should still match to Alice via the new MAC
	matches := matcher.GetMatches()
	if len(matches) == 0 {
		t.Fatal("Expected match via rotated MAC")
	}

	for _, match := range matches {
		if match.PersonID != person.ID {
			t.Errorf("Expected person ID %s, got %s", person.ID, match.PersonID)
		}
		if match.PersonName != "Alice" {
			t.Errorf("Expected person name 'Alice', got '%s'", match.PersonName)
		}
		break
	}
}

// ─── Match Confidence Computation Tests ───────────────────────────────────────────

func TestComputeMatchConfidence(t *testing.T) {
	tests := []struct {
		name       string
		td         *TriangulatedDevice
		blobDist   float64
		minConf    float64
		maxConf    float64
	}{
		{
			name: "High confidence: recent observation, 3+ nodes, low residual, close blob",
			td: &TriangulatedDevice{
				LastSeenAge: 1 * time.Second,
				NodeCount:   3,
				Residual:    0.2,
			},
			blobDist: 0.3,
			minConf:  0.6,
			maxConf:  1.0,
		},
		{
			name: "Medium confidence: older observation, 2 nodes",
			td: &TriangulatedDevice{
				LastSeenAge: 10 * time.Second,
				NodeCount:   2,
				Residual:    0.5,
			},
			blobDist: 1.0,
			minConf:  0.1,
			maxConf:  0.5,
		},
		{
			name: "Low confidence: single node, far blob",
			td: &TriangulatedDevice{
				LastSeenAge: 1 * time.Second,
				NodeCount:   1,
				Residual:    0.1,
			},
			blobDist: 1.8,
			minConf:  0.0,
			maxConf:  0.15,
		},
		{
			name: "Zero confidence: very old observation",
			td: &TriangulatedDevice{
				LastSeenAge: 20 * time.Second,
				NodeCount:   3,
				Residual:    0.1,
			},
			blobDist: 0.3,
			minConf:  0.0,
			maxConf:  0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := computeMatchConfidence(tt.td, tt.blobDist)
			if conf < tt.minConf || conf > tt.maxConf {
				t.Errorf("computeMatchConfidence() = %.3f, want in range [%.3f, %.3f]", conf, tt.minConf, tt.maxConf)
			}
		})
	}
}

// ─── Multiple Devices Same Person Tests ───────────────────────────────────────────

func TestMultipleDevicesSamePerson(t *testing.T) {
	mockNodes := &mockNodePositionAccessor{
		positions: map[string][3]float64{
			"node:00:01": {0.0, 1.5, 0.0},
			"node:00:02": {4.0, 1.5, 0.0},
			"node:00:03": {2.0, 1.5, 3.5},
		},
	}

	reg := setupTestRegistryForIdentity(t)
	defer reg.Close()

	cache := NewRSSICache(30 * time.Second)
	matcher := NewIdentityMatcher(reg, cache, mockNodes)

	// Create person
	person, _ := reg.CreatePerson("Alice", "#3b82f6")

	// Create phone and Fitbit for same person
	reg.ProcessRelayMessage("node:00:01", []BLEObservation{
		{Addr: "aa:bb:cc:dd:ee:01", Name: "Alice's Phone", MfrID: 0x004C, RSSIdBm: -65},
		{Addr: "aa:bb:cc:dd:ee:02", Name: "Alice's Fitbit", MfrID: 0x009E, RSSIdBm: -65},
	})
	reg.AssignToPerson("aa:bb:cc:dd:ee:01", person.ID)
	reg.AssignToPerson("aa:bb:cc:dd:ee:02", person.ID)

	// Both devices at blob position (2.0, 1.5, 0.0).
	// Distances: node1=2.0m→-73, node2=2.0m→-73, node3=3.5m→-79
	now := time.Now()
	cache.AddWithTime("aa:bb:cc:dd:ee:01", "node:00:01", -73, now)
	cache.AddWithTime("aa:bb:cc:dd:ee:01", "node:00:02", -73, now)
	cache.AddWithTime("aa:bb:cc:dd:ee:01", "node:00:03", -79, now)
	cache.AddWithTime("aa:bb:cc:dd:ee:02", "node:00:01", -73, now)
	cache.AddWithTime("aa:bb:cc:dd:ee:02", "node:00:02", -73, now)
	cache.AddWithTime("aa:bb:cc:dd:ee:02", "node:00:03", -79, now)

	blobs := []struct {
		ID      int
		X, Y, Z float64
		Weight  float64
	}{
		{ID: 1, X: 2.0, Y: 1.5, Z: 0.0, Weight: 0.9},
	}

	matcher.UpdateBlobs(blobs)

	// Should have exactly one match (highest confidence device wins)
	matches := matcher.GetMatches()
	if len(matches) != 1 {
		t.Errorf("Expected exactly 1 match for person with multiple devices, got %d", len(matches))
	}

	// Match should be for blob 1
	for blobID, match := range matches {
		if blobID != 1 {
			t.Errorf("Expected match for blob 1, got blob %d", blobID)
		}
		if match.PersonID != person.ID {
			t.Errorf("Expected person ID %s, got %s", person.ID, match.PersonID)
		}
	}
}

// ─── Helper Functions ─────────────────────────────────────────────────────────────

func setupTestRegistryForIdentity(t *testing.T) *Registry {
	tmpDir, err := os.MkdirTemp("", "ble-identity-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "ble.db")
	reg, err := NewRegistry(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create registry: %v", err)
	}

	t.Cleanup(func() {
		os.RemoveAll(tmpDir)
	})

	return reg
}
