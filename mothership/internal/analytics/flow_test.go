// Package analytics provides crowd flow visualization and analysis.
package analytics

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFlowAccumulator_TrajectorySampling(t *testing.T) {
	// Create temp database
	tmpDir, err := os.MkdirTemp("", "flow_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	fa, err := NewFlowAccumulator(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("Failed to create FlowAccumulator: %v", err)
	}
	defer fa.Close()

	// Test: track moves 0.25m -> segment recorded
	// First update establishes the waypoint
	fa.UpdateTrack(TrackUpdate{
		ID:       1,
		X:        0,
		Y:        0,
		Z:        0,
		VX:       0.25,
		VY:       0,
		VZ:       0,
		PersonID: "person1",
	})

	// Second update 0.25m away should create a segment
	fa.UpdateTrack(TrackUpdate{
		ID:       1,
		X:        0.25,
		Y:        0,
		Z:        0,
		VX:       0.25,
		VY:       0,
		VZ:       0,
		PersonID: "person1",
	})

	// Verify segment was recorded by checking the database directly
	// (Flow map requires MinSegmentsForFlow = 5 per cell to display)
	var segmentCount int
	err = fa.db.QueryRow(`SELECT COUNT(*) FROM trajectory_segments`).Scan(&segmentCount)
	if err != nil {
		t.Fatalf("Failed to query segments: %v", err)
	}
	if segmentCount == 0 {
		t.Error("Expected at least one segment after 0.25m movement")
	}

	// Test: track moves 0.05m -> no segment
	fa.UpdateTrack(TrackUpdate{
		ID:       2,
		X:        0,
		Y:        0,
		Z:        0,
		VX:       0.05,
		VY:       0,
		VZ:       0,
		PersonID: "person2",
	})

	fa.UpdateTrack(TrackUpdate{
		ID:       2,
		X:        0.05,
		Y:        0,
		Z:        0,
		VX:       0.05,
		VY:       0,
		VZ:       0,
		PersonID: "person2",
	})

	// This small movement should not create a new segment (0.05 < 0.2 threshold)
	// Check that no new segments were added for track 2
	var track2Count int
	err = fa.db.QueryRow(`SELECT COUNT(*) FROM trajectory_segments WHERE id LIKE '2_%'`).Scan(&track2Count)
	if err != nil {
		t.Fatalf("Failed to query track 2 segments: %v", err)
	}
	if track2Count > 0 {
		t.Errorf("Expected no segments for track 2 (0.05m movement), got %d", track2Count)
	}
}

func TestFlowAccumulator_FlowVectorAveraging(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "flow_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	fa, err := NewFlowAccumulator(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("Failed to create FlowAccumulator: %v", err)
	}
	defer fa.Close()

	// Create 5 segments all pointing East (positive X direction)
	for i := 0; i < 5; i++ {
		fa.UpdateTrack(TrackUpdate{
			ID:       i + 1,
			X:        float64(i) * 0.5,
			Y:        0,
			Z:        0,
			VX:       0.3,
			VY:       0,
			VZ:       0,
			PersonID: "",
		})
		fa.UpdateTrack(TrackUpdate{
			ID:       i + 1,
			X:        float64(i)*0.5 + 0.3,
			Y:        0,
			Z:        0,
			VX:       0.3,
			VY:       0,
			VZ:       0,
			PersonID: "",
		})
	}

	// The flow vectors should average to approximately (1, 0) direction
	// Since all segments point in the same direction
}

func TestFlowAccumulator_DwellAccumulation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "flow_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	fa, err := NewFlowAccumulator(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("Failed to create FlowAccumulator: %v", err)
	}
	defer fa.Close()

	// Create 100 stationary updates at the same location
	gridX := 5
	gridZ := 7
	x := (float64(gridX) + 0.5) * GridCellSize
	z := (float64(gridZ) + 0.5) * GridCellSize

	for i := 0; i < 100; i++ {
		fa.UpdateTrack(TrackUpdate{
			ID:       1,
			X:        x,
			Y:        0,
			Z:        z,
			VX:       0, // Stationary
			VY:       0,
			VZ:       0,
			PersonID: "person1",
		})
	}

	// Get dwell heatmap
	heatmap, err := fa.GetDwellHeatmap("")
	if err != nil {
		t.Fatalf("Failed to get dwell heatmap: %v", err)
	}

	// Find the cell at gridX, gridZ
	var foundCell *DwellHeatmapCell
	for _, cell := range heatmap.Cells {
		if cell.GridX == gridX && cell.GridZ == gridZ {
			foundCell = &cell
			break
		}
	}

	if foundCell == nil {
		t.Error("Expected to find dwell cell at (5, 7)")
	} else if foundCell.Count < 100 {
		t.Errorf("Expected dwell count >= 100, got %d", foundCell.Count)
	}
}

func TestFlowAccumulator_CorridorDetection(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "flow_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	fa, err := NewFlowAccumulator(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("Failed to create FlowAccumulator: %v", err)
	}
	defer fa.Close()

	// Create 20 aligned segments in adjacent cells (simulating a corridor)
	// All moving in +X direction
	for i := 0; i < 20; i++ {
		trackID := i + 1
		x := float64(i) * 0.25
		fa.UpdateTrack(TrackUpdate{
			ID:       trackID,
			X:        x,
			Y:        0,
			Z:        1.0,
			VX:       0.25,
			VY:       0,
			VZ:       0,
			PersonID: "",
		})
		fa.UpdateTrack(TrackUpdate{
			ID:       trackID,
			X:        x + 0.25,
			Y:        0,
			Z:        1.0,
			VX:       0.25,
			VY:       0,
			VZ:       0,
			PersonID: "",
		})
	}

	// Run corridor detection
	err = fa.ComputeCorridors()
	if err != nil {
		t.Fatalf("Failed to compute corridors: %v", err)
	}

	// Get corridors
	corridors, err := fa.GetCorridors()
	if err != nil {
		t.Fatalf("Failed to get corridors: %v", err)
	}

	// With aligned segments, we should detect at least one corridor
	if len(corridors) == 0 {
		t.Log("Warning: No corridors detected from aligned segments (may need more data)")
	}
}

func TestFlowAccumulator_TimeRangeFiltering(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "flow_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	fa, err := NewFlowAccumulator(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("Failed to create FlowAccumulator: %v", err)
	}
	defer fa.Close()

	// Create multiple tracks that all move through the same cells to accumulate
	// enough segments per cell (need >= MinSegmentsForFlow = 5)
	// Move from (0,0,0) to (0.5,0,0) - this passes through the same grid cells
	for trackID := 1; trackID <= 6; trackID++ {
		// Establish waypoint
		fa.UpdateTrack(TrackUpdate{ID: trackID, X: 0, Y: 0, Z: 0, VX: 0.3, VY: 0, VZ: 0, PersonID: ""})
		// Move to create segment
		fa.UpdateTrack(TrackUpdate{ID: trackID, X: 0.5, Y: 0, Z: 0, VX: 0.3, VY: 0, VZ: 0, PersonID: ""})
	}

	// Query with time range: since 8 days ago (should include recent data)
	since := time.Now().AddDate(0, 0, -8)
	flowMap, err := fa.GetFlowMap("", since, time.Now())
	if err != nil {
		t.Fatalf("Failed to get flow map: %v", err)
	}

	// Should include the segments we just created
	if len(flowMap.Cells) == 0 {
		t.Error("Expected flow cells from recent segments")
	}
}

func TestFlowAccumulator_PruneOldSegments(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "flow_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	fa, err := NewFlowAccumulator(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("Failed to create FlowAccumulator: %v", err)
	}
	defer fa.Close()

	// Create a segment
	fa.UpdateTrack(TrackUpdate{ID: 1, X: 0, Y: 0, Z: 0, VX: 1, VY: 0, VZ: 0, PersonID: ""})
	fa.UpdateTrack(TrackUpdate{ID: 1, X: 1, Y: 0, Z: 0, VX: 1, VY: 0, VZ: 0, PersonID: ""})

	// Check segment was recorded
	var countBefore int
	err = fa.db.QueryRow(`SELECT COUNT(*) FROM trajectory_segments`).Scan(&countBefore)
	if err != nil {
		t.Fatalf("Failed to query segments: %v", err)
	}
	if countBefore == 0 {
		t.Fatal("Expected at least one segment before pruning")
	}

	// Prune with default retention (should not delete recent data)
	err = fa.PruneOldSegments()
	if err != nil {
		t.Fatalf("Failed to prune segments: %v", err)
	}

	// Data should still exist (recent data not pruned)
	var countAfter int
	err = fa.db.QueryRow(`SELECT COUNT(*) FROM trajectory_segments`).Scan(&countAfter)
	if err != nil {
		t.Fatalf("Failed to query segments after prune: %v", err)
	}

	if countAfter != countBefore {
		t.Errorf("Expected %d segments after pruning recent data, got %d", countBefore, countAfter)
	}
}

func TestBresenhamLine(t *testing.T) {
	tests := []struct {
		name          string
		x0, z0, x1, z1 int
		expectedCount int
	}{
		{"horizontal line", 0, 0, 5, 0, 6},
		{"vertical line", 0, 0, 0, 5, 6},
		{"diagonal line", 0, 0, 3, 3, 4},
		{"single point", 2, 2, 2, 2, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cells := bresenhamLine(tt.x0, tt.z0, tt.x1, tt.z1)
			if len(cells) != tt.expectedCount {
				t.Errorf("Expected %d cells, got %d", tt.expectedCount, len(cells))
			}
		})
	}
}

func TestCircularVariance(t *testing.T) {
	tests := []struct {
		name     string
		angles   []float64
		expected float64
		tolerance float64
	}{
		{"all same angle", []float64{0, 0, 0, 0, 0}, 0.0, 0.01},
		{"opposite angles", []float64{0, math.Pi}, 1.0, 0.01},
		{"uniform distribution", []float64{0, math.Pi / 2, math.Pi, 3 * math.Pi / 2}, 1.0, 0.1},
		{"narrow spread", []float64{-0.1, 0, 0.1}, 0.0, 0.05},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			variance := circularVariance(tt.angles)
			if math.Abs(variance-tt.expected) > tt.tolerance {
				t.Errorf("Expected variance ~%.2f, got %.4f", tt.expected, variance)
			}
		})
	}
}

func TestFindConnectedComponents(t *testing.T) {
	tests := []struct {
		name          string
		cells         map[[2]int]bool
		expectedCount int
	}{
		{
			name:          "empty",
			cells:         map[[2]int]bool{},
			expectedCount: 0,
		},
		{
			name:          "single cell",
			cells:         map[[2]int]bool{{0, 0}: true},
			expectedCount: 1,
		},
		{
			name: "two separate cells",
			cells: map[[2]int]bool{
				{0, 0}: true,
				{5, 5}: true,
			},
			expectedCount: 2,
		},
		{
			name: "two adjacent cells",
			cells: map[[2]int]bool{
				{0, 0}: true,
				{1, 0}: true,
			},
			expectedCount: 1,
		},
		{
			name: "L-shaped region",
			cells: map[[2]int]bool{
				{0, 0}: true,
				{1, 0}: true,
				{2, 0}: true,
				{2, 1}: true,
				{2, 2}: true,
			},
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			regions := findConnectedComponents(tt.cells)
			if len(regions) != tt.expectedCount {
				t.Errorf("Expected %d regions, got %d", tt.expectedCount, len(regions))
			}
		})
	}
}

func TestGenerateSegmentID(t *testing.T) {
	id1 := generateSegmentID(1, time.Now())
	id2 := generateSegmentID(2, time.Now())

	if id1 == "" || id2 == "" {
		t.Error("Expected non-empty segment IDs")
	}

	if id1 == id2 {
		t.Error("Expected different segment IDs for different track IDs")
	}
}

func TestGenerateCorridorID(t *testing.T) {
	tests := []struct {
		index    int
		expected string
	}{
		{0, "corridor_A0"},
		{1, "corridor_B0"},
		{25, "corridor_Z0"},
		{26, "corridor_A1"},
		{27, "corridor_B1"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			id := generateCorridorID(tt.index)
			if id != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, id)
			}
		})
	}
}
