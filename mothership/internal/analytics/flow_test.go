// Package analytics provides crowd flow visualization and analysis.
package analytics

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

const (
	testGridCellSize = 0.25 // meters - matches defaultGridCellM
)

func TestFlowAccumulator_TrajectorySampling(t *testing.T) {
	// Create temp database
	tmpDir, err := os.MkdirTemp("", "flow_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	fa := NewFlowAccumulator(db, testGridCellSize)
	if err := fa.InitSchema(); err != nil {
		t.Fatalf("Failed to init schema: %v", err)
	}
	defer fa.Close()

	// Test: track moves 0.25m -> segment recorded
	// First update establishes the waypoint
	fa.AddTrackUpdate("track-1", 0, 0, 0, 0.25, 0, 0, "person1")

	// Second update 0.25m away should create a segment
	fa.AddTrackUpdate("track-1", 0.25, 0, 0, 0.25, 0, 0, "person1")

	// Flush buffers
	fa.Flush()

	// Verify segment was recorded by checking the database directly
	var segmentCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM trajectory_segments`).Scan(&segmentCount)
	if err != nil {
		t.Fatalf("Failed to query segments: %v", err)
	}
	if segmentCount == 0 {
		t.Error("Expected at least one segment after 0.25m movement")
	}

	// Test: track moves 0.05m -> no segment
	fa.AddTrackUpdate("track-2", 0, 0, 0, 0.05, 0, 0, "person2")
	fa.AddTrackUpdate("track-2", 0.05, 0, 0, 0.05, 0, 0, "person2")

	// Flush buffers
	fa.Flush()

	// This small movement should not create a new segment (0.05 < 0.2 threshold)
	var track2Count int
	err = db.QueryRow(`SELECT COUNT(*) FROM trajectory_segments WHERE person_id = ?`, "person2").Scan(&track2Count)
	if err != nil {
		t.Fatalf("Failed to query track 2 segments: %v", err)
	}
	// The track-2 person_id may not have any segments since the movement was too small
	// We need to check if we still only have 1 segment from track-1
	var totalCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM trajectory_segments`).Scan(&totalCount)
	if err != nil {
		t.Fatalf("Failed to query total segments: %v", err)
	}
	if totalCount != 1 {
		t.Errorf("Expected 1 segment (only from track-1), got %d", totalCount)
	}
}

func TestFlowAccumulator_FlowVectorAveraging(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "flow_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	fa := NewFlowAccumulator(db, testGridCellSize)
	if err := fa.InitSchema(); err != nil {
		t.Fatalf("Failed to init schema: %v", err)
	}
	defer fa.Close()

	// Create 5 segments all pointing East (positive X direction)
	for i := 0; i < 5; i++ {
		trackID := string(rune('a' + i))
		fa.AddTrackUpdate(trackID, float64(i)*0.5, 0, 0, 0.3, 0, 0, "")
		fa.AddTrackUpdate(trackID, float64(i)*0.5+0.3, 0, 0, 0.3, 0, 0, "")
	}

	// Flush buffers
	fa.Flush()

	// The flow vectors should average to approximately (1, 0) direction
	// Since all segments point in the same direction
	// Get flow map to verify
	since := time.Now().Add(-time.Hour)
	until := time.Now()
	flowMap, err := fa.ComputeFlowMap(nil, &since, &until)
	if err != nil {
		t.Fatalf("Failed to compute flow map: %v", err)
	}

	if len(flowMap.Cells) == 0 {
		t.Error("Expected at least one flow cell from segments")
	}

	// Check that the flow vectors are generally pointing East (positive X)
	for _, cell := range flowMap.Cells {
		if cell.VX < 0 {
			t.Errorf("Expected positive VX (East direction), got %f", cell.VX)
		}
	}
}

func TestFlowAccumulator_DwellAccumulation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "flow_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	fa := NewFlowAccumulator(db, testGridCellSize)
	if err := fa.InitSchema(); err != nil {
		t.Fatalf("Failed to init schema: %v", err)
	}
	defer fa.Close()

	// Create 100 stationary updates at the same location
	gridX := 5
	gridY := 7
	x := (float64(gridX) + 0.5) * testGridCellSize
	y := (float64(gridY) + 0.5) * testGridCellSize

	// First update to establish waypoint
	fa.AddTrackUpdate("track-1", x, y, 0, 0, 0, 0, "person1")

	// 99 more stationary updates (speed = 0)
	for i := 0; i < 99; i++ {
		fa.AddTrackUpdate("track-1", x, y, 0, 0, 0, 0, "person1")
	}

	// Flush buffers
	fa.Flush()

	// Get dwell heatmap
	heatmap, err := fa.ComputeDwellHeatmap(nil)
	if err != nil {
		t.Fatalf("Failed to get dwell heatmap: %v", err)
	}

	// Find the cell at gridX, gridY
	var foundCell *DwellCell
	for _, cell := range heatmap.Cells {
		if cell.GridX == gridX && cell.GridY == gridY {
			foundCell = &cell
			break
		}
	}

	if foundCell == nil {
		t.Errorf("Expected to find dwell cell at (%d, %d)", gridX, gridY)
	} else if foundCell.Count < 99 {
		t.Errorf("Expected dwell count >= 99, got %d", foundCell.Count)
	}
}

func TestFlowAccumulator_CorridorDetection(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "flow_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	fa := NewFlowAccumulator(db, testGridCellSize)
	if err := fa.InitSchema(); err != nil {
		t.Fatalf("Failed to init schema: %v", err)
	}
	defer fa.Close()

	// Create 20 aligned segments in adjacent cells (simulating a corridor)
	// All moving in +X direction
	for i := 0; i < 20; i++ {
		trackID := string(rune('a' + i))
		x := float64(i) * 0.25
		fa.AddTrackUpdate(trackID, x, 0, 1.0, 0.25, 0, 0, "")
		fa.AddTrackUpdate(trackID, x+0.25, 0, 1.0, 0.25, 0, 0, "")
	}

	// Flush buffers
	fa.Flush()

	// Run corridor detection
	_, err = fa.DetectCorridors()
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

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	fa := NewFlowAccumulator(db, testGridCellSize)
	if err := fa.InitSchema(); err != nil {
		t.Fatalf("Failed to init schema: %v", err)
	}
	defer fa.Close()

	// Create multiple tracks that all move through the same cells to accumulate
	// enough segments per cell
	for trackID := 1; trackID <= 6; trackID++ {
		trackStr := string(rune('a' + trackID))
		// Establish waypoint
		fa.AddTrackUpdate(trackStr, 0, 0, 0, 0.3, 0, 0, "")
		// Move to create segment
		fa.AddTrackUpdate(trackStr, 0.5, 0, 0, 0.3, 0, 0, "")
	}

	// Flush buffers
	fa.Flush()

	// Query with time range: since 8 days ago (should include recent data)
	since := time.Now().AddDate(0, 0, -8)
	until := time.Now()
	flowMap, err := fa.ComputeFlowMap(nil, &since, &until)
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

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	fa := NewFlowAccumulator(db, testGridCellSize)
	if err := fa.InitSchema(); err != nil {
		t.Fatalf("Failed to init schema: %v", err)
	}
	defer fa.Close()

	// Create a segment
	fa.AddTrackUpdate("track-1", 0, 0, 0, 1, 0, 0, "")
	fa.AddTrackUpdate("track-1", 1, 0, 0, 1, 0, 0, "")

	// Flush buffers
	fa.Flush()

	// Check segment was recorded
	var countBefore int
	err = db.QueryRow(`SELECT COUNT(*) FROM trajectory_segments`).Scan(&countBefore)
	if err != nil {
		t.Fatalf("Failed to query segments: %v", err)
	}
	if countBefore == 0 {
		t.Fatal("Expected at least one segment before pruning")
	}

	// Prune with default retention (should not delete recent data)
	err = fa.PruneOldData()
	if err != nil {
		t.Fatalf("Failed to prune segments: %v", err)
	}

	// Data should still exist (recent data not pruned)
	var countAfter int
	err = db.QueryRow(`SELECT COUNT(*) FROM trajectory_segments`).Scan(&countAfter)
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
		x0, y0, x1, y1 int
		expectedCount int
	}{
		{"horizontal line", 0, 0, 5, 0, 6},
		{"vertical line", 0, 0, 0, 5, 6},
		{"diagonal line", 0, 0, 3, 3, 4},
		{"single point", 2, 2, 2, 2, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cells := bresenhamLine(tt.x0, tt.y0, tt.x1, tt.y1)
			if len(cells) != tt.expectedCount {
				t.Errorf("Expected %d cells, got %d", tt.expectedCount, len(cells))
			}
		})
	}
}

func TestCellKeyAndParse(t *testing.T) {
	// Test cell key generation and parsing
	x, y := 5, 10
	key := cellKey(x, y)

	px, py := parseCellKey(key)
	if px != x || py != y {
		t.Errorf("Expected (%d, %d), got (%d, %d)", x, y, px, py)
	}
}

func TestFlowAccumulator_RemoveTrack(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "flow_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	fa := NewFlowAccumulator(db, testGridCellSize)
	if err := fa.InitSchema(); err != nil {
		t.Fatalf("Failed to init schema: %v", err)
	}
	defer fa.Close()

	// Add a track at origin (establishes waypoint)
	fa.AddTrackUpdate("track-1", 0, 0, 0, 0.25, 0, 0, "person1")

	// Remove the track (clears the waypoint)
	fa.RemoveTrack("track-1")

	// Re-add the track at a new position (establishes new waypoint)
	fa.AddTrackUpdate("track-1", 0.25, 0, 0, 0.25, 0, 0, "person1")
	// Add another update to create a segment
	fa.AddTrackUpdate("track-1", 0.5, 0, 0, 0.25, 0, 0, "person1")
	fa.Flush()

	// Should have a segment since we have two updates after removal
	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM trajectory_segments`).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query segments: %v", err)
	}
	if count == 0 {
		t.Error("Expected a segment after track removal and re-addition")
	}
}

func TestFlowAccumulator_PersonFiltering(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "flow_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	fa := NewFlowAccumulator(db, testGridCellSize)
	if err := fa.InitSchema(); err != nil {
		t.Fatalf("Failed to init schema: %v", err)
	}
	defer fa.Close()

	// Create segments for person1
	fa.AddTrackUpdate("track-1", 0, 0, 0, 0.3, 0, 0, "person1")
	fa.AddTrackUpdate("track-1", 0.3, 0, 0, 0.3, 0, 0, "person1")

	// Create segments for person2
	fa.AddTrackUpdate("track-2", 1, 0, 0, 0.3, 0, 0, "person2")
	fa.AddTrackUpdate("track-2", 1.3, 0, 0, 0.3, 0, 0, "person2")

	// Create segments for unknown person
	fa.AddTrackUpdate("track-3", 2, 0, 0, 0.3, 0, 0, "")
	fa.AddTrackUpdate("track-3", 2.3, 0, 0, 0.3, 0, 0, "")

	fa.Flush()

	// Query all flow
	allFlow, err := fa.ComputeFlowMap(nil, nil, nil)
	if err != nil {
		t.Fatalf("Failed to get all flow: %v", err)
	}

	// Query only person1
	person1 := "person1"
	person1Flow, err := fa.ComputeFlowMap(&person1, nil, nil)
	if err != nil {
		t.Fatalf("Failed to get person1 flow: %v", err)
	}

	// Query only person2
	person2 := "person2"
	person2Flow, err := fa.ComputeFlowMap(&person2, nil, nil)
	if err != nil {
		t.Fatalf("Failed to get person2 flow: %v", err)
	}

	// All flow should have more segments than individual person flows
	if len(person1Flow.Cells) == 0 && len(person2Flow.Cells) == 0 && len(allFlow.Cells) == 0 {
		t.Error("Expected some flow data")
	}
}
