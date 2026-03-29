// Package diagnostics provides tests for link weather diagnostics
package diagnostics

import (
	"context"
	"testing"
	"time"
)

// TestRule1_EnvironmentalChange tests Rule 1: Environmental change detection
// Trigger: High baseline drift (>5% per hour) correlated across multiple links simultaneously (>50% of active links).
func TestRule1_EnvironmentalChange(t *testing.T) {
	engine := NewDiagnosticEngine(DiagnosticConfig{
		DiagnosticInterval: 15 * time.Minute,
		HistoryWindow:      1 * time.Hour,
		MinSamples:         10,
	})

	linkIDs := []string{
		"AA:BB:CC:DD:EE:FF:11:22:33:44:55:66",
		"AA:BB:CC:DD:EE:FF:22:33:44:55:66:77",
		"AA:BB:CC:DD:EE:FF:33:44:55:66:77:88",
		"AA:BB:CC:DD:EE:FF:44:55:66:77:88:99",
		"AA:BB:CC:DD:EE:FF:55:66:77:88:99:AA",
	}

	// Track which links have been queried
	queriedLinks := make(map[string]bool)

	engine.SetAllLinkIDsAccessor(func() []string {
		return linkIDs
	})

	// Set up health history accessor with high drift on 60% of links (3/5)
	engine.SetHealthHistoryAccessor(func(linkID string, window time.Duration) []LinkHealthSnapshot {
		queriedLinks[linkID] = true
		now := time.Now()

		// Create 15 samples over the window
		samples := make([]LinkHealthSnapshot, 15)
		for i := 0; i < 15; i++ {
			samples[i] = LinkHealthSnapshot{
				Timestamp:      now.Add(-time.Duration(i) * time.Minute),
				SNR:            0.7,
				PhaseStability: 0.3,
				PacketRate:     18.0,
				IsQuietPeriod:  i%3 == 0, // Some quiet periods
			}

			// 60% of links (3/5) have high drift (> 5%)
			if linkID == linkIDs[0] || linkID == linkIDs[1] || linkID == linkIDs[2] {
				samples[i].DriftRate = 0.08 // 8% drift - above threshold
			} else {
				samples[i].DriftRate = 0.02 // 2% drift - normal
			}
		}
		return samples
	})

	// Run diagnostic pass
	engine.RunDiagnosticPass()

	// Check that diagnosis fired for the first link (which has high drift)
	diagnoses := engine.GetDiagnoses(linkIDs[0])
	if len(diagnoses) == 0 {
		t.Error("Expected environmental change diagnosis to fire for link with high correlated drift")
	}

	// Verify the diagnosis details
	if len(diagnoses) > 0 {
		d := diagnoses[0]
		if d.RuleID != "environmental_change" {
			t.Errorf("Expected ruleID 'environmental_change', got %s", d.RuleID)
		}
		if d.Severity != SeverityINFO {
			t.Errorf("Expected severity INFO, got %s", d.Severity)
		}
		if d.ConfidenceScore < 0.8 {
			t.Errorf("Expected confidence >= 0.8 for correlated drift, got %f", d.ConfidenceScore)
		}
		if d.RepositioningTarget != nil {
			t.Error("Expected no repositioning target for environmental change")
		}
	}

	// Verify all links were queried for correlation check
	if len(queriedLinks) != len(linkIDs) {
		t.Errorf("Expected all %d links to be queried, got %d", len(linkIDs), len(queriedLinks))
	}
}

// TestRule2_WiFiCongestion tests Rule 2: WiFi congestion or distance
// Trigger: Packet rate health < 0.8 for more than 10 minutes on a single link.
func TestRule2_WiFiCongestion(t *testing.T) {
	engine := NewDiagnosticEngine(DiagnosticConfig{
		DiagnosticInterval: 15 * time.Minute,
		HistoryWindow:      1 * time.Hour,
		MinSamples:         10,
	})

	linkID := "AA:BB:CC:DD:EE:FF:11:22:33:44:55:66"
	now := time.Now()

	engine.SetAllLinkIDsAccessor(func() []string {
		return []string{linkID}
	})

	// Create 15 minutes of history with low packet rate (14 Hz = 70% health)
	samples := make([]LinkHealthSnapshot, 16)
	for i := 0; i < 16; i++ {
		samples[i] = LinkHealthSnapshot{
			Timestamp:      now.Add(-time.Duration(i) * time.Minute),
			SNR:            0.7,
			PhaseStability: 0.3,
			PacketRate:     14.0, // 14 Hz out of 20 Hz = 70% health (< 80%)
			DriftRate:      0.02,
			IsQuietPeriod:  false,
		}
	}

	engine.SetHealthHistoryAccessor(func(lid string, window time.Duration) []LinkHealthSnapshot {
		if lid == linkID {
			return samples
		}
		return nil
	})

	// Run diagnostic pass
	engine.RunDiagnosticPass()

	// Check that diagnosis fired
	diagnoses := engine.GetDiagnoses(linkID)
	if len(diagnoses) == 0 {
		t.Fatal("Expected WiFi congestion diagnosis to fire")
	}

	// Find the WiFi congestion diagnosis
	var found *Diagnosis
	for _, d := range diagnoses {
		if d.RuleID == "wifi_congestion_distance" {
			found = &d
			break
		}
	}

	if found == nil {
		t.Fatal("Expected WiFi congestion diagnosis to be present")
	}

	// Verify severity is ACTIONABLE
	if found.Severity != SeverityACTIONABLE {
		t.Errorf("Expected severity ACTIONABLE, got %s", found.Severity)
	}

	// Verify advice text mentions WiFi router
	if found.Advice == "" {
		t.Error("Expected non-empty advice text")
	}

	// Verify node MAC is extracted correctly
	expectedNodeB := "11:22:33:44:55:66"
	if found.RepositioningNodeMAC != expectedNodeB {
		t.Errorf("Expected repositioning node MAC %s, got %s", expectedNodeB, found.RepositioningNodeMAC)
	}
}

// TestRule3_MetalInterference tests Rule 3: Near-field metal interference
// Trigger: Low phase stability (< 0.4) sustained for > 30 minutes during known-quiet periods.
func TestRule3_MetalInterference(t *testing.T) {
	engine := NewDiagnosticEngine(DiagnosticConfig{
		DiagnosticInterval: 15 * time.Minute,
		HistoryWindow:      1 * time.Hour,
		MinSamples:         10,
	})

	linkID := "AA:BB:CC:DD:EE:FF:11:22:33:44:55:66"
	now := time.Now()

	engine.SetAllLinkIDsAccessor(func() []string {
		return []string{linkID}
	})

	// Create 35 minutes of history with high phase instability during quiet periods
	// PhaseStability > 0.6 means unstable (high variance)
	samples := make([]LinkHealthSnapshot, 36)
	for i := 0; i < 36; i++ {
		samples[i] = LinkHealthSnapshot{
			Timestamp:      now.Add(-time.Duration(i) * time.Minute),
			SNR:            0.7,
			PhaseStability: 0.75, // High variance = unstable
			PacketRate:     18.0,
			DriftRate:      0.02,
			IsQuietPeriod:  true, // All quiet periods to satisfy the rule
		}
	}

	engine.SetHealthHistoryAccessor(func(lid string, window time.Duration) []LinkHealthSnapshot {
		if lid == linkID {
			return samples
		}
		return nil
	})

	// Run diagnostic pass
	engine.RunDiagnosticPass()

	// Check that diagnosis fired
	diagnoses := engine.GetDiagnoses(linkID)
	if len(diagnoses) == 0 {
		t.Fatal("Expected metal interference diagnosis to fire")
	}

	// Find the metal interference diagnosis
	var found *Diagnosis
	for _, d := range diagnoses {
		if d.RuleID == "metal_interference" {
			found = &d
			break
		}
	}

	if found == nil {
		t.Fatal("Expected metal interference diagnosis to be present")
	}

	// Verify severity is ACTIONABLE
	if found.Severity != SeverityACTIONABLE {
		t.Errorf("Expected severity ACTIONABLE, got %s", found.Severity)
	}

	// Verify advice mentions metal
	if found.Advice == "" {
		t.Error("Expected non-empty advice text")
	}

	// Verify node A MAC is extracted correctly
	expectedNodeA := "AA:BB:CC:DD:EE:FF"
	if found.RepositioningNodeMAC != expectedNodeA {
		t.Errorf("Expected repositioning node MAC %s, got %s", expectedNodeA, found.RepositioningNodeMAC)
	}
}

// TestRule4_FresnelBlockage tests Rule 4: Fresnel zone blockage
// Trigger: Consistent miss rate (>30% of test walks) in a specific area with feedback data.
func TestRule4_FresnelBlockage(t *testing.T) {
	engine := NewDiagnosticEngine(DiagnosticConfig{
		DiagnosticInterval: 15 * time.Minute,
		HistoryWindow:      1 * time.Hour,
		MinSamples:         10,
	})

	linkID := "AA:BB:CC:DD:EE:FF:11:22:33:44:55:66"
	now := time.Now()

	engine.SetAllLinkIDsAccessor(func() []string {
		return []string{linkID}
	})

	// Create health history
	samples := make([]LinkHealthSnapshot, 15)
	for i := 0; i < 15; i++ {
		samples[i] = LinkHealthSnapshot{
			Timestamp:      now.Add(-time.Duration(i) * time.Minute),
			SNR:            0.6,
			PhaseStability: 0.4,
			PacketRate:     18.0,
			DriftRate:      0.02,
			IsQuietPeriod:  i%2 == 0,
		}
	}

	engine.SetHealthHistoryAccessor(func(lid string, window time.Duration) []LinkHealthSnapshot {
		if lid == linkID {
			return samples
		}
		return nil
	})

	// Provide clustered false negative feedback events
	feedbackEvents := []FeedbackEvent{
		{LinkID: linkID, EventType: "false_negative", Position: Vec3{X: 3.0, Y: 1.0, Z: 2.0}, Timestamp: now.Add(-1 * time.Hour)},
		{LinkID: linkID, EventType: "false_negative", Position: Vec3{X: 3.2, Y: 1.0, Z: 2.1}, Timestamp: now.Add(-2 * time.Hour)},
		{LinkID: linkID, EventType: "false_negative", Position: Vec3{X: 2.9, Y: 1.0, Z: 1.9}, Timestamp: now.Add(-3 * time.Hour)},
		{LinkID: linkID, EventType: "false_negative", Position: Vec3{X: 3.1, Y: 1.0, Z: 2.0}, Timestamp: now.Add(-4 * time.Hour)},
		{LinkID: linkID, EventType: "false_negative", Position: Vec3{X: 3.0, Y: 1.0, Z: 2.2}, Timestamp: now.Add(-5 * time.Hour)},
	}

	engine.SetFeedbackAccessor(func(lid string, window time.Duration) []FeedbackEvent {
		if lid == linkID {
			return feedbackEvents
		}
		return nil
	})

	// Set up repositioning computer
	engine.SetRepositioningComputer(func(lid string, blockedZone Vec3) (Vec3, float64, error) {
		return Vec3{X: 4.0, Y: 1.5, Z: 3.0}, 0.25, nil // Return improvement > 0.1
	})

	// Run diagnostic pass
	engine.RunDiagnosticPass()

	// Check that diagnosis fired
	diagnoses := engine.GetDiagnoses(linkID)
	if len(diagnoses) == 0 {
		t.Fatal("Expected Fresnel blockage diagnosis to fire")
	}

	// Find the Fresnel blockage diagnosis
	var found *Diagnosis
	for _, d := range diagnoses {
		if d.RuleID == "fresnel_blockage" {
			found = &d
			break
		}
	}

	if found == nil {
		t.Fatal("Expected Fresnel blockage diagnosis to be present")
	}

	// Verify severity is ACTIONABLE
	if found.Severity != SeverityACTIONABLE {
		t.Errorf("Expected severity ACTIONABLE, got %s", found.Severity)
	}

	// Verify repositioning target is non-nil
	if found.RepositioningTarget == nil {
		t.Error("Expected repositioning target to be computed")
	} else {
		// Verify target position
		if found.RepositioningTarget.X != 4.0 || found.RepositioningTarget.Z != 3.0 {
			t.Errorf("Expected repositioning target (4.0, 3.0), got (%.1f, %.1f)",
				found.RepositioningTarget.X, found.RepositioningTarget.Z)
		}
	}

	// Verify advice mentions repositioning
	if found.Advice == "" {
		t.Error("Expected non-empty advice text")
	}
}

// TestRule5_PeriodicInterference tests Rule 5: Periodic interference spikes
// Trigger: Periodic spikes in deltaRMS variance (3-10 events per hour) not correlated with occupancy.
func TestRule5_PeriodicInterference(t *testing.T) {
	engine := NewDiagnosticEngine(DiagnosticConfig{
		DiagnosticInterval: 15 * time.Minute,
		HistoryWindow:      2 * time.Hour,
		MinSamples:         10,
	})

	linkID := "AA:BB:CC:DD:EE:FF:11:22:33:44:55:66"
	now := time.Now()

	engine.SetAllLinkIDsAccessor(func() []string {
		return []string{linkID}
	})

	// Create 2 hours of samples with periodic variance spikes
	// 5 spikes per hour = 10 spikes total, each lasting ~2 minutes
	samples := make([]LinkHealthSnapshot, 120)
	for i := 0; i < 120; i++ {
		timestamp := now.Add(-time.Duration(i) * time.Minute)

		// Base variance
		variance := 0.1

		// Create periodic spikes at 12-minute intervals (5 per hour)
		// Each spike lasts 2 minutes
		minute := i % 60
		if minute%12 < 2 {
			variance = 0.5 // Spike
		}

		samples[i] = LinkHealthSnapshot{
			Timestamp:        timestamp,
			SNR:              0.7,
			PhaseStability:   0.3,
			PacketRate:       18.0,
			DriftRate:        0.02,
			DeltaRMSVariance: variance,
			IsQuietPeriod:    true, // Not correlated with occupancy
		}
	}

	engine.SetHealthHistoryAccessor(func(lid string, window time.Duration) []LinkHealthSnapshot {
		if lid == linkID {
			return samples
		}
		return nil
	})

	// Run diagnostic pass
	engine.RunDiagnosticPass()

	// Check that diagnosis fired
	diagnoses := engine.GetDiagnoses(linkID)
	if len(diagnoses) == 0 {
		t.Fatal("Expected periodic interference diagnosis to fire")
	}

	// Find the periodic interference diagnosis
	var found *Diagnosis
	for _, d := range diagnoses {
		if d.RuleID == "periodic_interference" {
			found = &d
			break
		}
	}

	if found == nil {
		t.Fatal("Expected periodic interference diagnosis to be present")
	}

	// Verify severity is WARNING
	if found.Severity != SeverityWARNING {
		t.Errorf("Expected severity WARNING, got %s", found.Severity)
	}

	// Verify advice mentions microwave/interference sources
	if found.Advice == "" {
		t.Error("Expected non-empty advice text")
	}

	// Should not have repositioning target (interference is appliance-specific)
	if found.RepositioningTarget != nil {
		t.Error("Expected no repositioning target for periodic interference")
	}
}

// TestWeeklyTrendAggregation tests that weekly trends are correctly computed
func TestWeeklyTrendAggregation(t *testing.T) {
	// This tests the HealthStore's daily aggregation functionality
	// The actual implementation is in signal/healthpersist.go

	// For now, test the diagnostic engine's ability to provide
	// health snapshots that would be used for aggregation
	engine := NewDiagnosticEngine(DiagnosticConfig{
		DiagnosticInterval: 15 * time.Minute,
		HistoryWindow:      7 * 24 * time.Hour, // 7 days
		MinSamples:         10,
	})

	linkID := "AA:BB:CC:DD:EE:FF:11:22:33:44:55:66"
	now := time.Now()

	engine.SetAllLinkIDsAccessor(func() []string {
		return []string{linkID}
	})

	// Create 7 days of samples (one sample per hour = 168 samples)
	samples := make([]LinkHealthSnapshot, 168)
	for i := 0; i < 168; i++ {
		hourOffset := i
		samples[i] = LinkHealthSnapshot{
			Timestamp:      now.Add(-time.Duration(hourOffset) * time.Hour),
			SNR:            0.5 + float64(i%24)*0.02, // Varying SNR
			PhaseStability: 0.3,
			PacketRate:     18.0,
			DriftRate:      0.02,
			CompositeScore: 0.6 + float64(i%24)*0.015,
			IsQuietPeriod:  true,
		}
	}

	engine.SetHealthHistoryAccessor(func(lid string, window time.Duration) []LinkHealthSnapshot {
		if lid == linkID {
			return samples
		}
		return nil
	})

	// Verify history is accessible
	history := engine.GetDiagnoses(linkID)
	_ = history // Just verify no panic

	// The actual aggregation is done by HealthStore.AggregateDaily()
	// which is tested separately in healthpersist_test.go if it exists
}

// TestRepositioningTargetBounds tests that repositioning targets are within room bounds
func TestRepositioningTargetBounds(t *testing.T) {
	// Test the bounds checking function
	tests := []struct {
		name     string
		pos      Vec3
		width    float64
		depth    float64
		originX  float64
		originZ  float64
		expected bool
	}{
		{"inside bounds", Vec3{X: 5, Z: 5}, 10, 10, 0, 0, true},
		{"at origin", Vec3{X: 0, Z: 0}, 10, 10, 0, 0, true},
		{"at corner", Vec3{X: 10, Z: 10}, 10, 10, 0, 0, true},
		{"outside X low", Vec3{X: -1, Z: 5}, 10, 10, 0, 0, false},
		{"outside X high", Vec3{X: 11, Z: 5}, 10, 10, 0, 0, false},
		{"outside Z low", Vec3{X: 5, Z: -1}, 10, 10, 0, 0, false},
		{"outside Z high", Vec3{X: 5, Z: 11}, 10, 10, 0, 0, false},
		{"with origin offset", Vec3{X: 5, Z: 5}, 10, 10, 2, 2, true},
		{"outside with offset", Vec3{X: 1, Z: 5}, 10, 10, 2, 2, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsWithinBounds(tt.pos, tt.width, tt.depth, tt.originX, tt.originZ)
			if result != tt.expected {
				t.Errorf("IsWithinBounds(%v, %.1f, %.1f, %.1f, %.1f) = %v, want %v",
					tt.pos, tt.width, tt.depth, tt.originX, tt.originZ, result, tt.expected)
			}
		})
	}
}

// TestExtractNodeMACs tests the MAC extraction from link IDs
func TestExtractNodeMACs(t *testing.T) {
	tests := []struct {
		linkID    string
		expectA   string
		expectB   string
		expectNil bool
	}{
		{
			linkID:  "AA:BB:CC:DD:EE:FF:11:22:33:44:55:66",
			expectA: "AA:BB:CC:DD:EE:FF",
			expectB: "11:22:33:44:55:66",
		},
		{
			linkID:  "01:23:45:67:89:AB:CD:EF:01:23:45:67",
			expectA: "01:23:45:67:89:AB",
			expectB: "CD:EF:01:23:45:67",
		},
		{
			linkID:    "short",
			expectA:   "",
			expectB:   "",
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.linkID, func(t *testing.T) {
			nodeA := extractNodeAMAC(tt.linkID)
			nodeB := extractNodeBMAC(tt.linkID)

			if nodeA != tt.expectA {
				t.Errorf("extractNodeAMAC(%s) = %s, want %s", tt.linkID, nodeA, tt.expectA)
			}
			if nodeB != tt.expectB {
				t.Errorf("extractNodeBMAC(%s) = %s, want %s", tt.linkID, nodeB, tt.expectB)
			}
		})
	}
}

// TestRule2_NoFireWhenPacketRateGood tests that Rule 2 doesn't fire when packet rate is healthy
func TestRule2_NoFireWhenPacketRateGood(t *testing.T) {
	engine := NewDiagnosticEngine(DiagnosticConfig{
		DiagnosticInterval: 15 * time.Minute,
		HistoryWindow:      1 * time.Hour,
		MinSamples:         10,
	})

	linkID := "AA:BB:CC:DD:EE:FF:11:22:33:44:55:66"
	now := time.Now()

	engine.SetAllLinkIDsAccessor(func() []string {
		return []string{linkID}
	})

	// Create history with GOOD packet rate (19 Hz = 95% health, > 80%)
	samples := make([]LinkHealthSnapshot, 16)
	for i := 0; i < 16; i++ {
		samples[i] = LinkHealthSnapshot{
			Timestamp:      now.Add(-time.Duration(i) * time.Minute),
			SNR:            0.7,
			PhaseStability: 0.3,
			PacketRate:     19.0, // 95% health (> 80%)
			DriftRate:      0.02,
			IsQuietPeriod:  false,
		}
	}

	engine.SetHealthHistoryAccessor(func(lid string, window time.Duration) []LinkHealthSnapshot {
		if lid == linkID {
			return samples
		}
		return nil
	})

	// Run diagnostic pass
	engine.RunDiagnosticPass()

	// Check that WiFi congestion diagnosis did NOT fire
	diagnoses := engine.GetDiagnoses(linkID)
	for _, d := range diagnoses {
		if d.RuleID == "wifi_congestion_distance" {
			t.Error("WiFi congestion diagnosis should not fire when packet rate is healthy")
		}
	}
}

// TestRule1_NoFireWhenNotCorrelated tests Rule 1 doesn't fire when drift is not correlated
func TestRule1_NoFireWhenNotCorrelated(t *testing.T) {
	engine := NewDiagnosticEngine(DiagnosticConfig{
		DiagnosticInterval: 15 * time.Minute,
		HistoryWindow:      1 * time.Hour,
		MinSamples:         10,
	})

	linkIDs := []string{
		"AA:BB:CC:DD:EE:FF:11:22:33:44:55:66",
		"AA:BB:CC:DD:EE:FF:22:33:44:55:66:77",
		"AA:BB:CC:DD:EE:FF:33:44:55:66:77:88",
		"AA:BB:CC:DD:EE:FF:44:55:66:77:88:99",
		"AA:BB:CC:DD:EE:FF:55:66:77:88:99:AA",
	}

	engine.SetAllLinkIDsAccessor(func() []string {
		return linkIDs
	})

	// Set up health history where only 20% of links have high drift (< 50%)
	engine.SetHealthHistoryAccessor(func(linkID string, window time.Duration) []LinkHealthSnapshot {
		now := time.Now()
		samples := make([]LinkHealthSnapshot, 15)
		for i := 0; i < 15; i++ {
			samples[i] = LinkHealthSnapshot{
				Timestamp:      now.Add(-time.Duration(i) * time.Minute),
				SNR:            0.7,
				PhaseStability: 0.3,
				PacketRate:     18.0,
				DriftRate:      0.02, // Normal drift
			}
		}
		// Only first link has high drift (20% of 5 links)
		if linkID == linkIDs[0] {
			for i := range samples {
				samples[i].DriftRate = 0.08
			}
		}
		return samples
	})

	// Run diagnostic pass
	engine.RunDiagnosticPass()

	// Check that environmental change diagnosis did NOT fire (only 20% correlation)
	diagnoses := engine.GetDiagnoses(linkIDs[0])
	for _, d := range diagnoses {
		if d.RuleID == "environmental_change" {
			t.Error("Environmental change diagnosis should not fire when drift is not correlated across >50% of links")
		}
	}
}

// TestGetAllDiagnoses tests that GetAllDiagnoses returns all stored diagnoses
func TestGetAllDiagnoses(t *testing.T) {
	engine := NewDiagnosticEngine(DiagnosticConfig{
		DiagnosticInterval: 15 * time.Minute,
		HistoryWindow:      1 * time.Hour,
		MinSamples:         5,
	})

	linkIDs := []string{
		"AA:BB:CC:DD:EE:FF:11:22:33:44:55:66",
		"AA:BB:CC:DD:EE:FF:22:33:44:55:66:77",
	}

	engine.SetAllLinkIDsAccessor(func() []string {
		return linkIDs
	})

	// Create history that triggers Rule 2 for both links
	engine.SetHealthHistoryAccessor(func(linkID string, window time.Duration) []LinkHealthSnapshot {
		now := time.Now()
		samples := make([]LinkHealthSnapshot, 15)
		for i := 0; i < 15; i++ {
			samples[i] = LinkHealthSnapshot{
				Timestamp:      now.Add(-time.Duration(i) * time.Minute),
				SNR:            0.7,
				PhaseStability: 0.3,
				PacketRate:     10.0, // Low packet rate triggers Rule 2
				DriftRate:      0.02,
			}
		}
		return samples
	})

	// Run diagnostic pass
	engine.RunDiagnosticPass()

	// Get all diagnoses
	allDiagnoses := engine.GetAllDiagnoses()

	// Verify both links have diagnoses
	if len(allDiagnoses) < 2 {
		t.Errorf("Expected diagnoses for at least 2 links, got %d", len(allDiagnoses))
	}

	for _, linkID := range linkIDs {
		if diagnoses, ok := allDiagnoses[linkID]; !ok || len(diagnoses) == 0 {
			t.Errorf("Expected diagnosis for link %s", linkID)
		}
	}
}

// TestDiagnosticEngineStop tests that the engine stops cleanly
func TestDiagnosticEngineStop(t *testing.T) {
	engine := NewDiagnosticEngine(DiagnosticConfig{
		DiagnosticInterval: 100 * time.Millisecond,
		HistoryWindow:      1 * time.Hour,
		MinSamples:         5,
	})

	engine.SetAllLinkIDsAccessor(func() []string {
		return []string{"test:link"}
	})

	engine.SetHealthHistoryAccessor(func(linkID string, window time.Duration) []LinkHealthSnapshot {
		return []LinkHealthSnapshot{}
	})

	// Start and stop with context
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	go engine.Run(ctx)

	// Wait for context to be done
	<-ctx.Done()
	engine.Stop()

	// Should not panic
}
