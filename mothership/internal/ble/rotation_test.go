// Package ble provides tests for BLE address rotation detection.
package ble

import (
	"testing"
	"time"
)

// TestCompareManufacturerData tests manufacturer data comparison.
func TestCompareManufacturerData(t *testing.T) {
	detector := &RotationDetector{}

	tests := []struct {
		name     string
		oldDev   *DeviceRecord
		newDev   *DeviceRecord
		minScore float64
	}{
		{
			name: "same Apple device with proximity UUID",
			oldDev: &DeviceRecord{
				Addr:       "AA:BB:CC:DD:EE:FF",
				MfrID:      0x004C,
				MfrDataHex: "02015C00000000000000ABCD1234",
			},
			newDev: &DeviceRecord{
				Addr:       "11:22:33:44:55:66",
				MfrID:      0x004C,
				MfrDataHex: "02015C00000000000000ABCD5678",
			},
			minScore: 0.7, // Same fingerprint portion
		},
		{
			name: "same manufacturer ID",
			oldDev: &DeviceRecord{
				Addr:  "AA:BB:CC:DD:EE:FF",
				MfrID: 0x004C,
			},
			newDev: &DeviceRecord{
				Addr:  "11:22:33:44:55:66",
				MfrID: 0x004C,
			},
			minScore: 0.7,
		},
		{
			name: "different manufacturer",
			oldDev: &DeviceRecord{
				Addr:  "AA:BB:CC:DD:EE:FF",
				MfrID: 0x004C,
			},
			newDev: &DeviceRecord{
				Addr:  "11:22:33:44:55:66",
				MfrID: 0x0075,
			},
			minScore: 0.1, // Different manufacturer
		},
		{
			name: "same device name",
			oldDev: &DeviceRecord{
				Addr:       "AA:BB:CC:DD:EE:FF",
				DeviceName: "iPhone",
			},
			newDev: &DeviceRecord{
				Addr:       "11:22:33:44:55:66",
				DeviceName: "iPhone",
			},
			minScore: 0.5, // Same name
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, _ := detector.compareManufacturerData(tt.oldDev, tt.newDev)
			if score < tt.minScore {
				t.Errorf("compareManufacturerData() score = %.2f, want >= %.2f", score, tt.minScore)
			}
		})
	}
}

// TestCalculateRotationScore tests the rotation score calculation.
func TestCalculateRotationScore(t *testing.T) {
	detector := &RotationDetector{}
	now := time.Now()

	oldReadings := []*RSSIObservation{
		{NodeMAC: "node1", RSSIdBm: -60, Timestamp: now.Add(-60 * time.Second)},
		{NodeMAC: "node2", RSSIdBm: -55, Timestamp: now.Add(-30 * time.Second)},
	}

	newReadings := []*RSSIObservation{
		{NodeMAC: "node1", RSSIdBm: -58, Timestamp: now.Add(-10 * time.Second)}, // Similar RSSI at same node
		{NodeMAC: "node2", RSSIdBm: -53, Timestamp: now.Add(-5 * time.Second)},
	}

	score, reason := detector.calculateRotationScore(
		"AA:BB:CC:DD:EE:FF", oldReadings,
		"11:22:33:44:55:66", newReadings,
		now,
	)

	if score < 0.5 {
		t.Errorf("calculateRotationScore() score = %.2f, want >= 0.5", score)
	}

	t.Logf("Rotation score: %.2f, reason: %s", score, reason)
}

// TestCalculateTimeGapScore tests time gap score calculation.
func TestCalculateTimeGapScore(t *testing.T) {
	detector := &RotationDetector{}
	now := time.Now()

	tests := []struct {
		name       string
		oldReadings []*RSSIObservation
		newReadings []*RSSIObservation
		minScore   float64
		maxScore   float64
	}{
		{
			name: "immediate appearance (ideal)",
			oldReadings: []*RSSIObservation{
				{Timestamp: now.Add(-10 * time.Second)},
			},
			newReadings: []*RSSIObservation{
				{Timestamp: now},
			},
			minScore: 0.9,
			maxScore: 1.0,
		},
		{
			name: "30 second gap",
			oldReadings: []*RSSIObservation{
				{Timestamp: now.Add(-40 * time.Second)},
			},
			newReadings: []*RSSIObservation{
				{Timestamp: now.Add(-10 * time.Second)},
			},
			minScore: 0.9,
			maxScore: 1.0,
		},
		{
			name: "2 minute gap (within rotation window)",
			oldReadings: []*RSSIObservation{
				{Timestamp: now.Add(-150 * time.Second)},
			},
			newReadings: []*RSSIObservation{
				{Timestamp: now},
			},
			minScore: 0.5,
			maxScore: 1.0,
		},
		{
			name: "5 minute gap (late rotation)",
			oldReadings: []*RSSIObservation{
				{Timestamp: now.Add(-360 * time.Second)},
			},
			newReadings: []*RSSIObservation{
				{Timestamp: now},
			},
			minScore: 0.1,
			maxScore: 0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := detector.calculateTimeGapScore(tt.oldReadings, tt.newReadings, now)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("calculateTimeGapScore() score = %.2f, want [%.2f, %.2f]", score, tt.minScore, tt.maxScore)
			}
			t.Logf("Time gap score: %.2f", score)
		})
	}
}

// TestRotationDetectionFlow tests the full rotation detection flow.
func TestRotationDetectionFlow(t *testing.T) {
	// Create a test registry
	registry, err := NewRegistry(":memory:")
	if err != nil {
		t.Fatalf("NewRegistry() failed: %v", err)
	}
	defer registry.Close()

	cache := NewRSSICache(30 * time.Second)
	detector := NewRotationDetector(registry, cache)

	now := time.Now()

	// Create a person and device
	person, err := registry.CreatePerson("Alice", "#ff0000")
	if err != nil {
		t.Fatalf("CreatePerson() failed: %v", err)
	}

	// Simulate old device being seen
	oldAddr := "AA:BB:CC:DD:EE:FF"
	registry.ProcessRelayMessage("node1", []BLEObservation{
		{
			Addr:     oldAddr,
			Name:     "iPhone",
			MfrID:    0x004C,
			MfrDataHex: "02015C00000000000000ABCD1234",
			RSSIdBm:  -60,
		},
	})

	// Assign to person
	registry.AssignToPerson(oldAddr, person.ID)

	// Simulate device disappearing (no new observations for oldAddr)
	// And new address appearing
	newAddr := "11:22:33:44:55:66"
	observations := map[string][]*RSSIObservation{
		newAddr: {
			{NodeMAC: "node1", RSSIdBm: -58, Timestamp: now.Add(-10 * time.Second)},
			{NodeMAC: "node2", RSSIdBm: -55, Timestamp: now.Add(-5 * time.Second)},
		},
	}

	detector.ProcessObservations(observations)

	// Check that a candidate was created
	candidates := detector.GetCandidates()
	if len(candidates) == 0 {
		t.Fatal("No rotation candidates detected")
	}

	// Verify the candidate
	candidate := candidates[0]
	if candidate.OldAddr != oldAddr {
		t.Errorf("Expected OldAddr = %s, got %s", oldAddr, candidate.OldAddr)
	}
	if candidate.NewAddr != newAddr {
		t.Errorf("Expected NewAddr = %s, got %s", newAddr, candidate.NewAddr)
	}

	t.Logf("Rotation detected: %s -> %s (score: %.2f)", candidate.OldAddr, candidate.NewAddr, candidate.Score)
}
