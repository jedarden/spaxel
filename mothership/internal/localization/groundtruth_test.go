package localization

import (
	"math"
	"testing"
	"time"
)

func TestBLETrilateration_Basic(t *testing.T) {
	config := DefaultBLETrilaterationConfig()
	provider := NewBLEGroundTruthProvider(config)

	// Set up 4 nodes in a 10x10 room
	provider.SetNodePosition("node1", 0, 1.0, 0)
	provider.SetNodePosition("node2", 10, 1.0, 0)
	provider.SetNodePosition("node3", 10, 1.0, 10)
	provider.SetNodePosition("node4", 0, 1.0, 10)

	// Target is at (5, 5)
	// Distance to each node: sqrt(5^2 + 5^2) = 7.07m
	// Using path loss model: RSSI = TX - 10*n*log10(d)
	// For d=7.07m: RSSI = -59 - 10*2.5*log10(7.07) ≈ -80 dBm

	now := time.Now()
	provider.AddObservation("phone1", "node1", -80, now)
	provider.AddObservation("phone1", "node2", -80, now)
	provider.AddObservation("phone1", "node3", -80, now)
	provider.AddObservation("phone1", "node4", -80, now)

	pos := provider.GetGroundTruth("phone1")
	if pos == nil {
		t.Fatal("Expected ground truth position, got nil")
	}

	// Check position is approximately (5, 5)
	tolerance := 2.0 // 2 metre tolerance
	if math.Abs(pos.X-5) > tolerance {
		t.Errorf("X position: expected ~5, got %.2f", pos.X)
	}
	if math.Abs(pos.Z-5) > tolerance {
		t.Errorf("Z position: expected ~5, got %.2f", pos.Z)
	}
}

func TestBLETrilateration_OffCenter(t *testing.T) {
	config := DefaultBLETrilaterationConfig()
	provider := NewBLEGroundTruthProvider(config)

	// Set up 4 nodes in a 10x10 room
	provider.SetNodePosition("node1", 0, 1.0, 0)
	provider.SetNodePosition("node2", 10, 1.0, 0)
	provider.SetNodePosition("node3", 10, 1.0, 10)
	provider.SetNodePosition("node4", 0, 1.0, 10)

	// Target is at (2, 8)
	// Distance to node1 (0,0): sqrt(2^2 + 8^2) = 8.25m → RSSI ≈ -81.8
	// Distance to node2 (10,0): sqrt(8^2 + 8^2) = 11.31m → RSSI ≈ -84.6
	// Distance to node3 (10,10): sqrt(8^2 + 2^2) = 8.25m → RSSI ≈ -81.8
	// Distance to node4 (0,10): sqrt(2^2 + 2^2) = 2.83m → RSSI ≈ -70.3

	now := time.Now()
	provider.AddObservation("phone1", "node1", -81.8, now)
	provider.AddObservation("phone1", "node2", -84.6, now)
	provider.AddObservation("phone1", "node3", -81.8, now)
	provider.AddObservation("phone1", "node4", -70.3, now)

	pos := provider.GetGroundTruth("phone1")
	if pos == nil {
		t.Fatal("Expected ground truth position, got nil")
	}

	// Check position is approximately (2, 8)
	tolerance := 2.5 // More tolerant for off-center positions
	if math.Abs(pos.X-2) > tolerance {
		t.Errorf("X position: expected ~2, got %.2f", pos.X)
	}
	if math.Abs(pos.Z-8) > tolerance {
		t.Errorf("Z position: expected ~8, got %.2f", pos.Z)
	}
}

func TestBLETrilateration_InsufficientNodes(t *testing.T) {
	config := DefaultBLETrilaterationConfig()
	provider := NewBLEGroundTruthProvider(config)

	// Only set up 2 nodes
	provider.SetNodePosition("node1", 0, 1.0, 0)
	provider.SetNodePosition("node2", 10, 1.0, 0)

	now := time.Now()
	provider.AddObservation("phone1", "node1", -70, now)
	provider.AddObservation("phone1", "node2", -70, now)

	pos := provider.GetGroundTruth("phone1")
	if pos != nil {
		t.Error("Expected nil for insufficient nodes, got position")
	}
}

func TestBLETrilateration_StaleObservations(t *testing.T) {
	config := DefaultBLETrilaterationConfig()
	config.MaxAge = 1 * time.Second
	provider := NewBLEGroundTruthProvider(config)

	provider.SetNodePosition("node1", 0, 1.0, 0)
	provider.SetNodePosition("node2", 10, 1.0, 0)
	provider.SetNodePosition("node3", 10, 1.0, 10)

	// Add old observations
	oldTime := time.Now().Add(-10 * time.Second)
	provider.AddObservation("phone1", "node1", -70, oldTime)
	provider.AddObservation("phone1", "node2", -70, oldTime)
	provider.AddObservation("phone1", "node3", -70, oldTime)

	pos := provider.GetGroundTruth("phone1")
	if pos != nil {
		t.Error("Expected nil for stale observations, got position")
	}
}

func TestLearnedWeights_Defaults(t *testing.T) {
	lw := NewLearnedWeights()

	// Unknown link should return default weight of 1.0
	weight := lw.GetLinkWeight("unknown-link")
	if weight != 1.0 {
		t.Errorf("Expected default weight 1.0, got %.2f", weight)
	}

	// Unknown link should return default sigma of 0.0
	sigma := lw.GetLinkSigma("unknown-link")
	if sigma != 0.0 {
		t.Errorf("Expected default sigma 0.0, got %.2f", sigma)
	}
}

func TestLearnedWeights_Updates(t *testing.T) {
	lw := NewLearnedWeights()
	linkID := "aa:bb:cc:dd:ee:ff-11:22:33:44:55:66"

	// Simulate learning updates via internal maps
	lw.mu.Lock()
	lw.linkWeights[linkID] = 1.5
	lw.linkSigmas[linkID] = 0.2
	lw.mu.Unlock()

	weight := lw.GetLinkWeight(linkID)
	if weight != 1.5 {
		t.Errorf("Expected weight 1.5, got %.2f", weight)
	}

	sigma := lw.GetLinkSigma(linkID)
	if sigma != 0.2 {
		t.Errorf("Expected sigma 0.2, got %.2f", sigma)
	}
}

func TestWeightLearner_LearningFromFeedback(t *testing.T) {
	// Create a mock ground truth source
	mockGT := &mockGroundTruthSource{
		positions: map[string]*GroundTruthPosition{
			"phone1": {
				EntityID:   "phone1",
				X:          5.0,
				Y:          1.0,
				Z:          5.0,
				Confidence: 0.8,
				Timestamp:  time.Now(),
				Source:     "test",
			},
		},
		confidence: 0.8,
	}

	config := DefaultWeightLearnerConfig()
	config.LearningRate = 0.1
	config.RewardThreshold = 0.5

	engine := NewEngine(10, 10, 0, 0)
	learner := NewWeightLearner(mockGT, engine, config)

	// Record a good prediction (close to ground truth)
	goodPeaks := [][3]float64{{4.8, 5.2, 0.9}} // Close to (5, 5)
	linkStates := []LinkMotion{
		{NodeMAC: "node1", PeerMAC: "node2", DeltaRMS: 0.5, Motion: true},
	}
	learner.RecordPrediction(goodPeaks, linkStates, "phone1")

	// Process learning
	err := learner.ProcessLearning()
	if err != nil {
		t.Fatalf("ProcessLearning failed: %v", err)
	}

	// Check that learning occurred
	stats := learner.GetLinkStats("node1-node2")
	if stats == nil {
		t.Fatal("Expected stats for link, got nil")
	}
	if stats.ObservationCount == 0 {
		t.Error("Expected observation count > 0")
	}
}

func TestWeightLearner_PoorPrediction(t *testing.T) {
	mockGT := &mockGroundTruthSource{
		positions: map[string]*GroundTruthPosition{
			"phone1": {
				EntityID:   "phone1",
				X:          5.0,
				Y:          1.0,
				Z:          5.0,
				Confidence: 0.8,
				Timestamp:  time.Now(),
				Source:     "test",
			},
		},
		confidence: 0.8,
	}

	config := DefaultWeightLearnerConfig()
	config.LearningRate = 0.1
	config.PenaltyThreshold = 1.5

	engine := NewEngine(10, 10, 0, 0)
	learner := NewWeightLearner(mockGT, engine, config)

	// Record a poor prediction (far from ground truth)
	poorPeaks := [][3]float64{{0.5, 0.5, 0.9}} // Far from (5, 5)
	linkStates := []LinkMotion{
		{NodeMAC: "node1", PeerMAC: "node2", DeltaRMS: 0.5, Motion: true},
	}
	learner.RecordPrediction(poorPeaks, linkStates, "phone1")

	// Process learning
	err := learner.ProcessLearning()
	if err != nil {
		t.Fatalf("ProcessLearning failed: %v", err)
	}

	// Check weight was penalized
	weights := learner.GetLearnedWeights()
	weight := weights.GetLinkWeight("node1-node2")
	if weight >= 1.0 {
		t.Errorf("Expected penalized weight < 1.0, got %.2f", weight)
	}
}

func TestSelfImprovingLocalizer_Integration(t *testing.T) {
	config := DefaultSelfImprovingConfig()
	config.RoomWidth = 10
	config.RoomDepth = 10
	config.AdjustmentInterval = 1 * time.Second

	sil := NewSelfImprovingLocalizer(config)

	// Set up nodes
	sil.SetNodePosition("node1", 0, 0)
	sil.SetNodePosition("node2", 10, 0)
	sil.SetNodePosition("node3", 10, 10)
	sil.SetNodePosition("node4", 0, 10)

	// Add BLE observations for an entity at (5, 5)
	now := time.Now()
	sil.AddBLEObservation("phone1", "node1", -80, now)
	sil.AddBLEObservation("phone1", "node2", -80, now)
	sil.AddBLEObservation("phone1", "node3", -80, now)
	sil.AddBLEObservation("phone1", "node4", -80, now)

	// Check ground truth
	gt := sil.GetGroundTruth("phone1")
	if gt == nil {
		t.Fatal("Expected ground truth, got nil")
	}

	t.Logf("Ground truth: X=%.2f, Z=%.2f, Accuracy=%.2fm", gt.X, gt.Z, gt.Accuracy)

	// Check we can get learning progress
	progress := sil.GetLearningProgress()
	if progress == nil {
		t.Fatal("Expected learning progress, got nil")
	}

	t.Logf("Learning progress: %+v", progress)
}

// Mock ground truth source for testing
type mockGroundTruthSource struct {
	positions  map[string]*GroundTruthPosition
	confidence float64
}

func (m *mockGroundTruthSource) GetGroundTruth(entityID string) *GroundTruthPosition {
	return m.positions[entityID]
}

func (m *mockGroundTruthSource) GetAllGroundTruth() map[string]*GroundTruthPosition {
	result := make(map[string]*GroundTruthPosition)
	for k, v := range m.positions {
		result[k] = v
	}
	return result
}

func (m *mockGroundTruthSource) Confidence() float64 {
	return m.confidence
}

func TestGrid_WithLearnedSigma(t *testing.T) {
	grid := NewGrid(10, 10, 0.2, 0, 0)

	// Add influence with default sigma
	grid.AddLinkInfluence(0, 5, 10, 5, 1.0)

	cells1, cols, rows := grid.Snapshot()
	maxDefault := 0.0
	for _, v := range cells1 {
		if v > maxDefault {
			maxDefault = v
		}
	}

	grid.Reset()

	// Add influence with narrower sigma (0.5 multiplier)
	grid.AddLinkInfluenceWithSigma(0, 5, 10, 5, 1.0, 0.5)

	cells2, _, _ := grid.Snapshot()
	maxNarrow := 0.0
	for _, v := range cells2 {
		if v > maxNarrow {
			maxNarrow = v
		}
	}

	// Narrower sigma should concentrate more weight at the center
	if maxNarrow <= maxDefault {
		t.Errorf("Expected narrower sigma to have higher peak, got default=%.2f, narrow=%.2f",
			maxDefault, maxNarrow)
	}

	t.Logf("Grid size: %d x %d = %d cells", cols, rows, cols*rows)
	t.Logf("Max activation: default=%.3f, narrow=%.3f", maxDefault, maxNarrow)
}

func TestFusion_WithLearnedWeights(t *testing.T) {
	engine := NewEngine(10, 10, 0, 0)

	// Set up nodes
	engine.SetNodePosition("node1", 0, 0)
	engine.SetNodePosition("node2", 10, 0)
	engine.SetNodePosition("node3", 10, 10)
	engine.SetNodePosition("node4", 0, 10)

	// Create learned weights
	lw := NewLearnedWeights()
	lw.mu.Lock()
	lw.linkWeights["node1-node2"] = 1.5 // Boost this link
	lw.linkWeights["node3-node4"] = 0.5 // Suppress this link
	lw.mu.Unlock()

	engine.SetLearnedWeights(lw)

	// Run fusion with all links active
	links := []LinkMotion{
		{NodeMAC: "node1", PeerMAC: "node2", DeltaRMS: 0.3, Motion: true},
		{NodeMAC: "node2", PeerMAC: "node3", DeltaRMS: 0.3, Motion: true},
		{NodeMAC: "node3", PeerMAC: "node4", DeltaRMS: 0.3, Motion: true},
		{NodeMAC: "node4", PeerMAC: "node1", DeltaRMS: 0.3, Motion: true},
	}

	result := engine.Fuse(links)

	if result == nil {
		t.Fatal("Expected fusion result, got nil")
	}

	t.Logf("Fusion result: %d peaks", len(result.Peaks))
	for i, peak := range result.Peaks {
		t.Logf("  Peak %d: X=%.2f, Z=%.2f, weight=%.3f", i+1, peak[0], peak[1], peak[2])
	}
}
