package localization

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestShouldCollectSample_Gates(t *testing.T) {
	tests := []struct {
		name           string
		confidence     float64
		bleBlobDist    float64
		expectCollect  bool
	}{
		{
			name:          "high confidence, close position - should collect",
			confidence:    0.8,
			bleBlobDist:   0.3,
			expectCollect: true,
		},
		{
			name:          "exact threshold confidence - should collect",
			confidence:    0.7,
			bleBlobDist:   0.4,
			expectCollect: true,
		},
		{
			name:          "exact threshold distance - should collect",
			confidence:    0.8,
			bleBlobDist:   0.5,
			expectCollect: true,
		},
		{
			name:          "low confidence - should NOT collect",
			confidence:    0.6,
			bleBlobDist:   0.3,
			expectCollect: false,
		},
		{
			name:          "too far - should NOT collect",
			confidence:    0.8,
			bleBlobDist:   0.6,
			expectCollect: false,
		},
		{
			name:          "both fail - should NOT collect",
			confidence:    0.5,
			bleBlobDist:   1.0,
			expectCollect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldCollectSample(tt.confidence, tt.bleBlobDist)
			if result != tt.expectCollect {
				t.Errorf("ShouldCollectSample(%v, %v) = %v, want %v",
					tt.confidence, tt.bleBlobDist, result, tt.expectCollect)
			}
		})
	}
}

func TestComputeZoneGrid(t *testing.T) {
	tests := []struct {
		x, z     float64
		expectX  int
		expectY  int
	}{
		{0.0, 0.0, 0, 0},
		{0.25, 0.25, 0, 0},
		{0.5, 0.5, 1, 1},
		{1.0, 1.0, 2, 2},
		{1.49, 1.49, 2, 2},
		{1.5, 1.5, 3, 3},
		{5.0, 3.0, 10, 6},
		{-0.5, -0.5, -1, -1}, // Negative coordinates
	}

	for _, tt := range tests {
		gridX, gridY := ComputeZoneGrid(tt.x, tt.z)
		if gridX != tt.expectX || gridY != tt.expectY {
			t.Errorf("ComputeZoneGrid(%v, %v) = (%d, %d), want (%d, %d)",
				tt.x, tt.z, gridX, gridY, tt.expectX, tt.expectY)
		}
	}
}

func TestComputePositionError(t *testing.T) {
	tests := []struct {
		ble   Vec3
		blob  Vec3
		error float64
	}{
		{Vec3{0, 0, 0}, Vec3{0, 0, 0}, 0.0},
		{Vec3{1, 0, 0}, Vec3{0, 0, 0}, 1.0},
		{Vec3{0, 0, 1}, Vec3{0, 0, 0}, 1.0},
		{Vec3{3, 4, 0}, Vec3{0, 0, 0}, 5.0}, // 3-4-5 triangle
		{Vec3{1, 2, 2}, Vec3{0, 0, 0}, 3.0}, // sqrt(1+4+4) = 3
		{Vec3{5, 5, 5}, Vec3{5, 5, 5}, 0.0},
	}

	for _, tt := range tests {
		result := ComputePositionError(tt.ble, tt.blob)
		if math.Abs(result-tt.error) > 0.001 {
			t.Errorf("ComputePositionError(%v, %v) = %v, want %v",
				tt.ble, tt.blob, result, tt.error)
		}
	}
}

func TestSpatialWeightLearner_GetSpatialWeight_BilinearInterpolation(t *testing.T) {
	// Create a temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "spatial_weights.db")

	config := DefaultSpatialWeightLearnerConfig()
	learner, err := NewSpatialWeightLearner(dbPath, config)
	if err != nil {
		t.Fatalf("Failed to create learner: %v", err)
	}
	defer learner.Close()

	// Set weights at grid corners for a specific link
	linkID := "test-link-1"
	learner.mu.Lock()
	// Set weights at (0,0)=1.0, (1,0)=2.0, (0,1)=2.0, (1,1)=3.0
	// This creates a bilinear surface
	learner.setWeightLocked(linkID, 0, 0, 1.0)
	learner.setWeightLocked(linkID, 1, 0, 2.0)
	learner.setWeightLocked(linkID, 0, 1, 2.0)
	learner.setWeightLocked(linkID, 1, 1, 3.0)
	learner.mu.Unlock()

	tests := []struct {
		name     string
		x, z     float64
		expected float64
	}{
		// At grid points
		{"at origin", 0.0, 0.0, 1.0},
		{"at (0.5, 0)", 0.5, 0.0, 1.5}, // (1+2)/2
		{"at (0, 0.5)", 0.0, 0.5, 1.5}, // (1+2)/2
		{"at center", 0.25, 0.25, 1.5}, // Bilinear center of 1,2,2,3
		{"at (0.5, 0.5)", 0.5, 0.5, 2.0}, // Center of 1,2,2,3
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := learner.GetSpatialWeight(linkID, tt.x, tt.z)
			if math.Abs(result-tt.expected) > 0.01 {
				t.Errorf("GetSpatialWeight(%s, %v, %v) = %v, want %v",
					linkID, tt.x, tt.z, result, tt.expected)
			}
		})
	}
}

func TestSpatialWeightLearner_GetSpatialWeight_Fallback(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "spatial_weights.db")

	config := DefaultSpatialWeightLearnerConfig()
	learner, err := NewSpatialWeightLearner(dbPath, config)
	if err != nil {
		t.Fatalf("Failed to create learner: %v", err)
	}
	defer learner.Close()

	// Test unknown link returns default weight of 1.0
	result := learner.GetSpatialWeight("unknown-link", 5.0, 5.0)
	if result != 1.0 {
		t.Errorf("GetSpatialWeight for unknown link = %v, want 1.0", result)
	}

	// Test position with no learned weight returns 1.0
	learner.mu.Lock()
	learner.setWeightLocked("known-link", 0, 0, 2.0)
	learner.mu.Unlock()

	// At a far-away position where no weight is learned
	result = learner.GetSpatialWeight("known-link", 100.0, 100.0)
	if result != 1.0 {
		t.Errorf("GetSpatialWeight at unlearned position = %v, want 1.0", result)
	}
}

func TestSpatialWeightLearner_ProcessSample_SGD(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "spatial_weights.db")

	config := DefaultSpatialWeightLearnerConfig()
	config.LearningRate = 0.01 // Higher rate for visible effect
	learner, err := NewSpatialWeightLearner(dbPath, config)
	if err != nil {
		t.Fatalf("Failed to create learner: %v", err)
	}
	defer learner.Close()

	linkID := "link-test-1"
	zoneX, zoneY := 2, 2

	// Create a sample where blob position is far from BLE position
	// This should cause weight adjustment
	sample := GroundTruthSample{
		ID:       1,
		PersonID: "person1",
		BLEPosition: Vec3{
			X: 1.0,
			Y: 0.0,
			Z: 1.0,
		},
		BlobPosition: Vec3{
			X: 0.5, // 0.5m away from BLE
			Y: 0.0,
			Z: 0.5,
		},
		PositionError: 0.707, // sqrt(0.5^2 + 0.5^2)
		PerLinkDeltas: map[string]float64{
			linkID: 0.5,
		},
		PerLinkHealth: map[string]float64{
			linkID: 0.9,
		},
		BLEConfidence: 0.8,
		ZoneGridX:     zoneX,
		ZoneGridY:     zoneY,
		Timestamp:     time.Now(),
	}

	// Process multiple samples to see weight change
	for i := 0; i < 10; i++ {
		sample.ID = int64(i + 1)
		if err := learner.ProcessSample(sample); err != nil {
			t.Fatalf("ProcessSample failed: %v", err)
		}
	}

	// Check that weight has changed from default
	weight := learner.GetSpatialWeight(linkID, 1.0, 1.0)
	if weight == 1.0 {
		t.Errorf("Expected weight to change from 1.0 after SGD updates, got %v", weight)
	}
}

func TestSpatialWeightLearner_WeightClipping(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "spatial_weights.db")

	config := DefaultSpatialWeightLearnerConfig()
	config.MinWeight = 0.0
	config.MaxWeight = 5.0
	learner, err := NewSpatialWeightLearner(dbPath, config)
	if err != nil {
		t.Fatalf("Failed to create learner: %v", err)
	}
	defer learner.Close()

	linkID := "clip-test-link"

	// Set weight above max
	learner.mu.Lock()
	learner.setWeightLocked(linkID, 0, 0, 10.0)
	learner.mu.Unlock()

	// After normalization/clipping, should be at max
	// Note: bilinear interpolation will blend, so check the exact grid point
	learner.mu.RLock()
	weight := learner.getWeightLocked(linkID, 0, 0)
	learner.mu.RUnlock()

	if weight > config.MaxWeight {
		t.Errorf("Weight %v exceeds max %v", weight, config.MaxWeight)
	}
}

func TestGroundTruthStore_SampleCap(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "groundtruth.db")

	config := GroundTruthStoreConfig{
		MaxSamplesPerPerson: 10, // Small cap for testing
	}
	store, err := NewGroundTruthStore(dbPath, config)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	personID := "test-person"

	// Insert more samples than the cap
	for i := 0; i < 15; i++ {
		sample := GroundTruthSample{
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
			PersonID:  personID,
			BLEPosition: Vec3{
				X: float64(i) * 0.1,
				Y: 0,
				Z: float64(i) * 0.1,
			},
			BlobPosition: Vec3{
				X: float64(i) * 0.1,
				Y: 0,
				Z: float64(i) * 0.1,
			},
			PositionError: 0.1,
			PerLinkDeltas: map[string]float64{"link1": 0.5},
			PerLinkHealth: map[string]float64{"link1": 0.9},
			BLEConfidence: 0.8,
			ZoneGridX:     i % 5,
			ZoneGridY:     i % 5,
		}

		if err := store.AddSample(sample); err != nil {
			t.Fatalf("AddSample failed: %v", err)
		}

		// Small delay to allow async cap enforcement
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for async cap enforcement
	time.Sleep(100 * time.Millisecond)

	// Check count
	counts, err := store.GetSampleCountByPerson()
	if err != nil {
		t.Fatalf("GetSampleCountByPerson failed: %v", err)
	}

	if counts[personID] > config.MaxSamplesPerPerson {
		t.Errorf("Sample count %d exceeds cap %d", counts[personID], config.MaxSamplesPerPerson)
	}

	// Verify oldest samples were removed by checking we have recent samples
	total, err := store.GetTotalSampleCount()
	if err != nil {
		t.Fatalf("GetTotalSampleCount failed: %v", err)
	}

	if total > config.MaxSamplesPerPerson {
		t.Errorf("Total samples %d exceeds cap %d", total, config.MaxSamplesPerPerson)
	}
}

func TestGroundTruthCollector_CollectionGates(t *testing.T) {
	tmpDir := t.TempDir()
	gtPath := filepath.Join(tmpDir, "groundtruth.db")
	swPath := filepath.Join(tmpDir, "spatial_weights.db")

	gtStore, err := NewGroundTruthStore(gtPath, DefaultGroundTruthStoreConfig())
	if err != nil {
		t.Fatalf("Failed to create ground truth store: %v", err)
	}
	defer gtStore.Close()

	swLearner, err := NewSpatialWeightLearner(swPath, DefaultSpatialWeightLearnerConfig())
	if err != nil {
		t.Fatalf("Failed to create spatial weight learner: %v", err)
	}
	defer swLearner.Close()

	collector := NewGroundTruthCollector(gtStore, swLearner)

	tests := []struct {
		name          string
		confidence    float64
		bleBlobDist   float64
		expectCollect bool
	}{
		{"high confidence, close", 0.8, 0.3, true},
		{"low confidence", 0.6, 0.3, false},
		{"too far", 0.8, 0.6, false},
		{"at threshold", 0.7, 0.5, true},
		{"just below threshold", 0.69, 0.5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blePos := Vec3{X: 1.0, Y: 0.0, Z: 1.0}
			// Calculate blob position based on desired distance
			blobPos := Vec3{
				X: blePos.X + tt.bleBlobDist,
				Y: blePos.Y,
				Z: blePos.Z,
			}

			collected := collector.CollectSample(
				"person1",
				blePos,
				tt.confidence,
				blobPos,
				map[string]float64{"link1": 0.5},
				map[string]float64{"link1": 0.9},
			)

			if collected != tt.expectCollect {
				t.Errorf("CollectSample() = %v, want %v", collected, tt.expectCollect)
			}
		})
	}
}

func TestValidationChecker_ShouldAcceptUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	gtPath := filepath.Join(tmpDir, "groundtruth.db")

	gtStore, err := NewGroundTruthStore(gtPath, DefaultGroundTruthStoreConfig())
	if err != nil {
		t.Fatalf("Failed to create ground truth store: %v", err)
	}
	defer gtStore.Close()

	// Add some samples for validation
	for i := 0; i < 10; i++ {
		sample := GroundTruthSample{
			Timestamp: time.Now().Add(-time.Duration(i) * time.Hour),
			PersonID:  "person1",
			BLEPosition: Vec3{X: 1.0, Y: 0.0, Z: 1.0},
			BlobPosition: Vec3{X: 1.0 + float64(i)*0.01, Y: 0.0, Z: 1.0}, // Small errors
			PositionError: float64(i) * 0.01,
			PerLinkDeltas: map[string]float64{"link1": 0.5},
			PerLinkHealth: map[string]float64{"link1": 0.9},
			BLEConfidence: 0.8,
			ZoneGridX:     2,
			ZoneGridY:     2,
		}
		if err := gtStore.AddSample(sample); err != nil {
			t.Fatalf("AddSample failed: %v", err)
		}
	}

	config := DefaultSpatialWeightLearnerConfig()
	config.ImprovementThreshold = 0.05 // 5% improvement required

	checker := NewValidationChecker(gtStore, config)

	// Compute baseline error
	baseline, err := checker.ComputeBaselineError()
	if err != nil {
		t.Fatalf("ComputeBaselineError failed: %v", err)
	}

	// Baseline should be positive (we have samples)
	if baseline <= 0 {
		t.Errorf("Baseline error should be positive, got %v", baseline)
	}

	// Create a mock learner with no weights (all default to 1.0)
	swPath := filepath.Join(tmpDir, "spatial_weights.db")
	learner, err := NewSpatialWeightLearner(swPath, config)
	if err != nil {
		t.Fatalf("Failed to create learner: %v", err)
	}
	defer learner.Close()

	// Without learned weights, weighted error should be similar to baseline
	weighted, err := checker.ComputeWeightedError(learner)
	if err != nil {
		t.Fatalf("ComputeWeightedError failed: %v", err)
	}

	// Check that we can call ShouldAcceptUpdate
	shouldAccept, improvement, err := checker.ShouldAcceptUpdate(learner)
	if err != nil {
		t.Fatalf("ShouldAcceptUpdate failed: %v", err)
	}

	t.Logf("Baseline error: %.4f, Weighted error: %.4f, Improvement: %.2f%%, Should accept: %v",
		baseline, weighted, improvement*100, shouldAccept)
}

func TestSpatialWeightLearner_PersistAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "spatial_weights.db")

	config := DefaultSpatialWeightLearnerConfig()
	learner1, err := NewSpatialWeightLearner(dbPath, config)
	if err != nil {
		t.Fatalf("Failed to create learner1: %v", err)
	}

	// Set some weights
	learner1.mu.Lock()
	learner1.setWeightLocked("link1", 0, 0, 1.5)
	learner1.setWeightLocked("link1", 1, 0, 2.0)
	learner1.setWeightLocked("link2", 0, 0, 0.8)
	learner1.mu.Unlock()

	// Persist
	if err := learner1.PersistWeights(); err != nil {
		t.Fatalf("PersistWeights failed: %v", err)
	}

	learner1.Close()

	// Create new learner and verify weights are loaded
	learner2, err := NewSpatialWeightLearner(dbPath, config)
	if err != nil {
		t.Fatalf("Failed to create learner2: %v", err)
	}
	defer learner2.Close()

	// Check weights were loaded
	weight1 := learner2.GetSpatialWeight("link1", 0.0, 0.0)
	if math.Abs(weight1-1.5) > 0.01 {
		t.Errorf("Expected weight 1.5, got %v", weight1)
	}

	weight2 := learner2.GetSpatialWeight("link2", 0.0, 0.0)
	if math.Abs(weight2-0.8) > 0.01 {
		t.Errorf("Expected weight 0.8, got %v", weight2)
	}
}

func TestSpatialWeightIntegrator_AdjustLinkMotion(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "spatial_weights.db")

	config := DefaultSpatialWeightLearnerConfig()
	learner, err := NewSpatialWeightLearner(dbPath, config)
	if err != nil {
		t.Fatalf("Failed to create learner: %v", err)
	}
	defer learner.Close()

	// Set a weight
	learner.mu.Lock()
	learner.setWeightLocked("aa:bb-cc:dd", 5, 5, 2.0) // At zone (5,5) = position (2.5m, 2.5m)
	learner.mu.Unlock()

	integrator := NewSpatialWeightIntegrator(learner)

	lm := LinkMotion{
		NodeMAC:     "aa:bb",
		PeerMAC:     "cc:dd",
		DeltaRMS:    0.5,
		Motion:      true,
		HealthScore: 0.9,
	}

	// Adjust at position where weight is 2.0
	adjusted := integrator.AdjustLinkMotion(lm, 2.5, 2.5)

	// DeltaRMS should be multiplied by weight
	if adjusted.DeltaRMS < 0.9 || adjusted.DeltaRMS > 1.1 {
		t.Errorf("Expected DeltaRMS ~1.0, got %v", adjusted.DeltaRMS)
	}

	// Adjust at position where no weight is learned (should use 1.0)
	adjusted2 := integrator.AdjustLinkMotion(lm, 100.0, 100.0)
	if adjusted2.DeltaRMS != 0.5 {
		t.Errorf("Expected DeltaRMS 0.5 (no adjustment), got %v", adjusted2.DeltaRMS)
	}
}

func TestGetWeightStats(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "spatial_weights.db")

	config := DefaultSpatialWeightLearnerConfig()
	learner, err := NewSpatialWeightLearner(dbPath, config)
	if err != nil {
		t.Fatalf("Failed to create learner: %v", err)
	}
	defer learner.Close()

	// Initially no weights
	stats := learner.GetWeightStats()
	if stats["total_weights"].(int) != 0 {
		t.Errorf("Expected 0 weights initially, got %v", stats["total_weights"])
	}

	// Add some weights
	learner.mu.Lock()
	learner.setWeightLocked("link1", 0, 0, 1.5)
	learner.setWeightLocked("link1", 1, 0, 2.0)
	learner.setWeightLocked("link2", 0, 0, 0.5)
	learner.mu.Unlock()

	stats = learner.GetWeightStats()
	if stats["total_weights"].(int) != 3 {
		t.Errorf("Expected 3 weights, got %v", stats["total_weights"])
	}
	if stats["links_with_weights"].(int) != 2 {
		t.Errorf("Expected 2 links with weights, got %v", stats["links_with_weights"])
	}
}
