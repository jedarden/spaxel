package simulator

import (
	"fmt"
	"math"
	"testing"
)

func TestPathLoss(t *testing.T) {
	pm := NewPhysicsModel(DefaultSpace())

	tests := []struct {
		distance float64
		expected float64 // Approximate expected path loss
	}{
		{1.0, 40.0},   // At reference distance
		{2.0, 46.0},   // 2x distance = +6 dB
		{10.0, 60.0},  // 10x distance = +20 dB
		{100.0, 80.0}, // 100x distance = +40 dB
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("distance=%.1f", tt.distance), func(t *testing.T) {
			loss := pm.PathLossdB(tt.distance)
			// Allow small floating point error
			if math.Abs(loss-tt.expected) > 1.0 {
				t.Errorf("Distance %f: expected loss ~%f dB, got %f dB", tt.distance, tt.expected, loss)
			}
		})
	}
}

func TestWallAttenuation(t *testing.T) {
	pm := NewPhysicsModel(DefaultSpace())
	// Add a wall at x=2
	pm.AddWall(2, 0, 2, 10, 3.0) // drywall

	tests := []struct {
		name     string
		from, to Point
		expected float64
	}{
		{
			name:     "no wall intersection",
			from:     NewPoint(0, 5, 1),
			to:       NewPoint(1, 5, 1),
			expected: 0,
		},
		{
			name:     "crosses wall",
			from:     NewPoint(0, 5, 1),
			to:       NewPoint(5, 5, 1),
			expected: 3.0, // Drywall loss
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loss := pm.WallAttenuation(tt.from, tt.to)
			if loss != tt.expected {
				t.Errorf("Expected loss %f, got %f", tt.expected, loss)
			}
		})
	}
}

func TestExpectedRSSI(t *testing.T) {
	pm := NewPropagationModel(DefaultSpace())

	tx := NewPoint(0, 0, 2)
	rx := NewPoint(5, 0, 2)

	rssi := pm.ExpectedRSSI(tx, rx)

	// RSSI should be in realistic range [-90, -30]
	if rssi < -90 || rssi > -30 {
		t.Errorf("RSSI %d is outside realistic range [-90, -30]", rssi)
	}

	// RSSI should be less than TX power (-30 dBm)
	if rssi > -30 {
		t.Errorf("RSSI %d should be less than TX power -30 dBm", rssi)
	}
}

func TestAmplitudeAt(t *testing.T) {
	pm := NewPropagationModel(DefaultSpace())

	tx := NewPoint(0, 0, 2)
	rx := NewPoint(5, 0, 2)
	walker := NewPoint(2.5, 0, 1.7) // Midpoint

	amp := pm.AmplitudeAt(tx, rx, walker)

	// Amplitude should be positive and reasonable
	if amp < 0 || amp > 10 {
		t.Errorf("Amplitude %f is out of reasonable range", amp)
	}
}

func TestPhaseAtSubcarrier(t *testing.T) {
	pm := NewPropagationModel(DefaultSpace())

	tx := NewPoint(0, 0, 2)
	rx := NewPoint(5, 0, 2)
	walker := NewPoint(2.5, 0, 1.7)

	// Test multiple subcarriers
	for k := 0; k < 10; k++ {
		phase := pm.PhaseAtSubcarrier(tx, rx, walker, k, 0)

		// Phase should be in [-π, π]
		if phase < -math.Pi || phase > math.Pi {
			t.Errorf("Subcarrier %d: phase %f is outside [-π, π]", k, phase)
		}
	}
}

func TestDeltaRMS(t *testing.T) {
	pm := NewPhysicsModel(DefaultSpace())

	tx := NewPoint(0, 0, 2)
	rx := NewPoint(5, 0, 2)
	walker := NewPoint(2.5, 0, 1.7)

	deltaRMS := pm.DeltaRMS(tx, rx, walker)

	// DeltaRMS should be positive
	if deltaRMS < 0 {
		t.Errorf("DeltaRMS %f should be non-negative", deltaRMS)
	}

	// Walker at midpoint should produce significant delta (in zone 1)
	if deltaRMS < 0.1 {
		t.Errorf("DeltaRMS %f seems too low for walker at midpoint", deltaRMS)
	}
}

func TestFresnelZoneNumber(t *testing.T) {
	tx := NewPoint(0, 0, 2)
	rx := NewPoint(6, 0, 2)

	tests := []struct {
		name     string
		point    Point
		expected int
	}{
		{
			name:     "on direct path (midpoint)",
			point:    NewPoint(3, 0, 2),
			expected: 1, // Zone 1
		},
		{
			name:     "at TX",
			point:    NewPoint(0, 0, 2),
			expected: 1, // Zone 1
		},
		{
			name:     "at RX",
			point:    NewPoint(6, 0, 2),
			expected: 1, // Zone 1
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zone := FresnelZoneNumber(tx, rx, tt.point)
			if zone != tt.expected {
				t.Errorf("Expected zone %d, got %d", tt.expected, zone)
			}
		})
	}
}

func TestIsInFresnelZones(t *testing.T) {
	tx := NewPoint(0, 0, 2)
	rx := NewPoint(6, 0, 2)

	// Points on direct path should be in first Fresnel zone
	midpoint := NewPoint(3, 0, 2)
	if !IsInFresnelZones(tx, rx, midpoint, 1) {
		t.Error("Midpoint should be in first Fresnel zone")
	}

	// Points far from direct path should not be in first zone
	farPoint := NewPoint(3, 10, 2)
	if IsInFresnelZones(tx, rx, farPoint, 1) {
		t.Error("Far point from direct path should not be in first Fresnel zone")
	}

	// Midpoint should be in first 3 zones
	if !IsInFresnelZones(tx, rx, midpoint, 3) {
		t.Error("Midpoint should be in first 3 Fresnel zones")
	}
}

func TestGenerateAllLinks(t *testing.T) {
	nodes := NewNodeSet()
	nodes.AddVirtualNode("node-1", "Node 1", NewPoint(0, 0, 2))
	nodes.AddVirtualNode("node-2", "Node 2", NewPoint(5, 0, 2))
	nodes.AddVirtualNode("node-3", "Node 3", NewPoint(2.5, 5, 2))

	links := GenerateAllLinks(nodes)

	// With 3 TXRX nodes, should have 6 links (each direction)
	expectedMinLinks := 6 // At minimum

	if len(links) < expectedMinLinks {
		t.Errorf("Expected at least %d links, got %d", expectedMinLinks, len(links))
	}

	// No self-links
	for _, link := range links {
		if link.TX.ID == link.RX.ID {
			t.Errorf("Found self-link: %s -> %s", link.TX.ID, link.RX.ID)
		}
	}
}

func TestGDOPComputer(t *testing.T) {
	space := DefaultSpace()
	nodes := SuggestedNodes(space, 4)
	links := GenerateAllLinks(nodes)

	minX, minY, _, maxX, maxY, _ := space.Bounds()

	config := GridConfig{
		MinX:     minX,
		MinY:     minY,
		Width:    maxX - minX,
		Depth:    maxY - minY,
		CellSize: 0.5, // Larger cells for faster test
	}

	gc := NewGDOPComputer(links, config)

	// Should compute without error
	results := gc.ComputeAll()

	if len(results) == 0 {
		t.Error("Expected non-empty GDOP results")
	}

	// Check that we have reasonable grid dimensions
	expectedRows := int(math.Ceil(config.Depth / config.CellSize))
	expectedCols := int(math.Ceil(config.Width / config.CellSize))

	if len(results) != expectedRows {
		t.Errorf("Expected %d rows, got %d", expectedRows, len(results))
	}

	if len(results[0]) != expectedCols {
		t.Errorf("Expected %d cols, got %d", expectedCols, len(results[0]))
	}
}

func TestGDOPQuality(t *testing.T) {
	tests := []struct {
		gdop    float64
		quality string
	}{
		{1.0, "excellent"},
		{2.5, "good"},
		{5.0, "fair"},
		{10.0, "poor"},
		{math.Inf(1), "none"},
	}

	for _, tt := range tests {
		t.Run(tt.quality, func(t *testing.T) {
			quality := gdopToQuality(tt.gdop)
			if quality != tt.quality {
				t.Errorf("GDOP %f: expected quality '%s', got '%s'", tt.gdop, tt.quality, quality)
			}
		})
	}
}

func TestCoverageScore(t *testing.T) {
	space := DefaultSpace()
	nodes := SuggestedNodes(space, 4)
	links := GenerateAllLinks(nodes)

	minX, minY, _, maxX, maxY, _ := space.Bounds()

	config := GridConfig{
		MinX:     minX,
		MinY:     minY,
		Width:    maxX - minX,
		Depth:    maxY - minY,
		CellSize: 0.5,
	}

	gc := NewGDOPComputer(links, config)
	results := gc.ComputeAll()

	score := gc.CoverageScore(results)

	// Score should be between 0 and 100
	if score < 0 || score > 100 {
		t.Errorf("Coverage score %f is outside [0, 100] range", score)
	}

	// With 4 nodes at corners, should have reasonable coverage
	if score < 10 {
		t.Errorf("Coverage score %f seems too low for 4 corner nodes", score)
	}
}

func TestAverageGDOP(t *testing.T) {
	space := DefaultSpace()
	nodes := SuggestedNodes(space, 4)
	links := GenerateAllLinks(nodes)

	minX, minY, _, maxX, maxY, _ := space.Bounds()

	config := GridConfig{
		MinX:     minX,
		MinY:     minY,
		Width:    maxX - minX,
		Depth:    maxY - minY,
		CellSize: 0.5,
	}

	gc := NewGDOPComputer(links, config)
	results := gc.ComputeAll()

	avgGDOP := gc.AverageGDOP(results)

	// Average GDOP should be finite and reasonable
	if math.IsInf(avgGDOP, 0) {
		t.Error("Average GDOP is infinite - no coverage?")
	}

	if avgGDOP < 0 {
		t.Errorf("Average GDOP %f is negative", avgGDOP)
	}
}

func TestQualityCounts(t *testing.T) {
	space := DefaultSpace()
	nodes := SuggestedNodes(space, 4)
	links := GenerateAllLinks(nodes)

	minX, minY, _, maxX, maxY, _ := space.Bounds()

	config := GridConfig{
		MinX:     minX,
		MinY:     minY,
		Width:    maxX - minX,
		Depth:    maxY - minY,
		CellSize: 0.5,
	}

	gc := NewGDOPComputer(links, config)
	results := gc.ComputeAll()

	counts := gc.QualityCounts(results)

	// Should have all quality categories
	qualities := []string{"excellent", "good", "fair", "poor", "none"}
	totalCells := 0

	for _, quality := range qualities {
		count, exists := counts[quality]
		if !exists {
			t.Errorf("Missing quality category: %s", quality)
		}
		totalCells += count
	}

	// Total cells should match grid size
	expectedRows := int(math.Ceil(config.Depth / config.CellSize))
	expectedCols := int(math.Ceil(config.Width / config.CellSize))
	expectedTotal := expectedRows * expectedCols

	if totalCells != expectedTotal {
		t.Errorf("Total cells %d doesn't match grid size %d", totalCells, expectedTotal)
	}
}

func TestMinimumNodeCount(t *testing.T) {
	space := DefaultSpace()

	// Test different GDOP targets
	tests := []struct {
		targetGDOP float64
		minNodes   int
	}{
		{2.0, 1}, // Excellent coverage
		{4.0, 1}, // Good coverage
		{8.0, 1}, // Fair coverage
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("targetGDOP=%.1f", tt.targetGDOP), func(t *testing.T) {
			count := MinimumNodeCount(space, tt.targetGDOP)
			if count < tt.minNodes {
				t.Errorf("Expected at least %d nodes, got %d", tt.minNodes, count)
			}
		})
	}
}

func TestExpectedAccuracy(t *testing.T) {
	tests := []struct {
		gdop        float64
		minAccuracy float64
		maxAccuracy float64
	}{
		{1.0, 0.4, 0.6},  // GDOP 1: ~0.5m
		{2.0, 0.8, 1.2},  // GDOP 2: ~1.0m
		{4.0, 1.6, 2.4},  // GDOP 4: ~2.0m
		{math.Inf(1), -1, -1}, // Infinity: no accuracy
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("gdop=%.1f", tt.gdop), func(t *testing.T) {
			accuracy := ExpectedAccuracy(tt.gdop)

			if math.IsInf(tt.gdop, 0) {
				if !math.IsInf(accuracy, 0) {
					t.Errorf("Infinite GDOP should give infinite accuracy, got %f", accuracy)
				}
				return
			}

			if accuracy < tt.minAccuracy || accuracy > tt.maxAccuracy {
				t.Errorf("GDOP %f: accuracy %f outside expected range [%f, %f]",
					tt.gdop, accuracy, tt.minAccuracy, tt.maxAccuracy)
			}
		})
	}
}

func TestCornerPositions(t *testing.T) {
	space := DefaultSpace()
	positions := CornerPositions(space)

	if len(positions) != 6 {
		t.Errorf("Expected 6 corner positions, got %d", len(positions))
	}

	// All positions should be within space bounds
	minX, minY, minZ, maxX, maxY, maxZ := space.Bounds()

	for i, pos := range positions {
		if pos.X < minX || pos.X > maxX {
			t.Errorf("Position %d: X %f outside bounds [%f, %f]", i, pos.X, minX, maxX)
		}
		if pos.Y < minY || pos.Y > maxY {
			t.Errorf("Position %d: Y %f outside bounds [%f, %f]", i, pos.Y, minY, maxY)
		}
		if pos.Z < minZ || pos.Z > maxZ {
			t.Errorf("Position %d: Z %f outside bounds [%f, %f]", i, pos.Z, minZ, maxZ)
		}
	}
}

func TestSuggestedNodes(t *testing.T) {
	space := DefaultSpace()
	nodes := SuggestedNodes(space, 4)

	if nodes.Count() != 4 {
		t.Errorf("Expected 4 nodes, got %d", nodes.Count())
	}

	// All nodes should be enabled
	allNodes := nodes.All()
	for _, node := range allNodes {
		if !node.Enabled {
			t.Errorf("Node %s should be enabled", node.ID)
		}
	}
}

func TestGenerateShoppingList(t *testing.T) {
	space := DefaultSpace()
	nodes := SuggestedNodes(space, 4)

	list := GenerateShoppingList(space, nodes)

	// Should have positive values
	if list.MinimumNodes < 1 {
		t.Errorf("Minimum nodes %d should be at least 1", list.MinimumNodes)
	}

	if list.RecommendedNodes < 1 {
		t.Errorf("Recommended nodes %d should be at least 1", list.RecommendedNodes)
	}

	if list.CoveragePercent < 0 || list.CoveragePercent > 100 {
		t.Errorf("Coverage percent %f outside [0, 100] range", list.CoveragePercent)
	}

	if list.ExpectedAccuracy < 0 {
		t.Errorf("Expected accuracy %f should be non-negative", list.ExpectedAccuracy)
	}

	if len(list.OptimalPositions) != nodes.Count() {
		t.Errorf("Expected %d optimal positions, got %d", nodes.Count(), len(list.OptimalPositions))
	}
}

func TestGDOPColorMap(t *testing.T) {
	tests := []struct {
		gdop        float64
		expectedR   uint8
		expectedG   uint8
		expectedB   uint8
		description string
	}{
		{1.0, 34, 197, 94, "excellent - green"},
		{2.5, 255, 193, 7, "good - yellow"},
		{5.0, 255, 146, 0, "fair - orange"},
		{10.0, 220, 53, 69, "poor - red"},
		{math.Inf(1), 80, 80, 80, "none - gray"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			color := GDOPColorMap(tt.gdop)
			if color.R != tt.expectedR {
				t.Errorf("GDOP %f: expected R=%d, got R=%d", tt.gdop, tt.expectedR, color.R)
			}
			if color.G != tt.expectedG {
				t.Errorf("GDOP %f: expected G=%d, got G=%d", tt.gdop, tt.expectedG, color.G)
			}
			if color.B != tt.expectedB {
				t.Errorf("GDOP %f: expected B=%d, got B=%d", tt.gdop, tt.expectedB, color.B)
			}
		})
	}
}

func TestGDOPHeatmapData(t *testing.T) {
	space := DefaultSpace()
	nodes := SuggestedNodes(space, 4)
	links := GenerateAllLinks(nodes)

	minX, minY, _, maxX, maxY, _ := space.Bounds()

	config := GridConfig{
		MinX:     minX,
		MinY:     minY,
		Width:    maxX - minX,
		Depth:    maxY - minY,
		CellSize: 0.5,
	}

	gc := NewGDOPComputer(links, config)
	results := gc.ComputeAll()
	heatmap := gc.ToHeatmapData(results)

	// Verify dimensions match
	if heatmap.Width != len(results[0]) {
		t.Errorf("Expected width %d, got %d", len(results[0]), heatmap.Width)
	}
	if heatmap.Depth != len(results) {
		t.Errorf("Expected depth %d, got %d", len(results), heatmap.Depth)
	}

	// Verify array sizes
	expectedCells := heatmap.Width * heatmap.Depth
	if len(heatmap.GDOPValues) != expectedCells {
		t.Errorf("Expected %d GDOP values, got %d", expectedCells, len(heatmap.GDOPValues))
	}
	if len(heatmap.Qualities) != expectedCells {
		t.Errorf("Expected %d qualities, got %d", expectedCells, len(heatmap.Qualities))
	}
	if len(heatmap.Colors) != expectedCells {
		t.Errorf("Expected %d colors, got %d", expectedCells, len(heatmap.Colors))
	}
	if len(heatmap.AccuracyMap) != expectedCells {
		t.Errorf("Expected %d accuracy values, got %d", expectedCells, len(heatmap.AccuracyMap))
	}

	// Verify cell size and origin
	if heatmap.CellSize != config.CellSize {
		t.Errorf("Expected cell size %f, got %f", config.CellSize, heatmap.CellSize)
	}
	if heatmap.OriginX != config.MinX {
		t.Errorf("Expected origin X %f, got %f", config.MinX, heatmap.OriginX)
	}
	if heatmap.OriginY != config.MinY {
		t.Errorf("Expected origin Y %f, got %f", config.MinY, heatmap.OriginY)
	}

	// Verify all colors have 3 components (RGB)
	for i, color := range heatmap.Colors {
		if len(color) != 3 {
			t.Errorf("Color at index %d should have 3 components, got %d", i, len(color))
		}
	}
}

func TestComputeAccuracyMap(t *testing.T) {
	space := DefaultSpace()
	nodes := SuggestedNodes(space, 4)
	links := GenerateAllLinks(nodes)

	minX, minY, _, maxX, maxY, _ := space.Bounds()

	config := GridConfig{
		MinX:     minX,
		MinY:     minY,
		Width:    maxX - minX,
		Depth:    maxY - minY,
		CellSize: 0.5,
	}

	gc := NewGDOPComputer(links, config)
	results := gc.ComputeAll()
	accuracyMap := gc.ComputeAccuracyMap(results)

	// Verify dimensions
	if len(accuracyMap) != len(results) {
		t.Errorf("Expected %d rows, got %d", len(results), len(accuracyMap))
	}

	for i, row := range accuracyMap {
		if len(row) != len(results[i]) {
			t.Errorf("Row %d: expected %d cols, got %d", i, len(results[i]), len(row))
		}
	}

	// All accuracy values should be non-negative
	for y, row := range accuracyMap {
		for x, accuracy := range row {
			if !math.IsInf(accuracy, 1) && accuracy < 0 {
				t.Errorf("Accuracy at [%d][%d] is negative: %f", y, x, accuracy)
			}
		}
	}
}

func TestComputeColorMap(t *testing.T) {
	space := DefaultSpace()
	nodes := SuggestedNodes(space, 4)
	links := GenerateAllLinks(nodes)

	minX, minY, _, maxX, maxY, _ := space.Bounds()

	config := GridConfig{
		MinX:     minX,
		MinY:     minY,
		Width:    maxX - minX,
		Depth:    maxY - minY,
		CellSize: 0.5,
	}

	gc := NewGDOPComputer(links, config)
	results := gc.ComputeAll()
	colors := gc.ComputeColorMap(results)

	// Verify flattened size
	expectedCells := len(results) * len(results[0])
	if len(colors) != expectedCells {
		t.Errorf("Expected %d color entries, got %d", expectedCells, len(colors))
	}

	// All colors should have 3 components (RGB)
	for i, color := range colors {
		if len(color) != 3 {
			t.Errorf("Color at index %d should have 3 components, got %d", i, len(color))
		}
		// RGB values should be in [0, 255]
		for j, v := range color {
			if v < 0 || v > 255 {
				t.Errorf("Color[%d][%d] = %d is outside [0, 255] range", i, j, v)
			}
		}
	}
}

func TestGetWorstCoverageCells(t *testing.T) {
	space := DefaultSpace()
	nodes := SuggestedNodes(space, 4)
	links := GenerateAllLinks(nodes)

	minX, minY, _, maxX, maxY, _ := space.Bounds()

	config := GridConfig{
		MinX:     minX,
		MinY:     minY,
		Width:    maxX - minX,
		Depth:    maxY - minY,
		CellSize: 0.5,
	}

	gc := NewGDOPComputer(links, config)
	results := gc.ComputeAll()

	worst := gc.GetWorstCoverageCells(results, 5)

	// Should return at most 5 cells
	if len(worst) > 5 {
		t.Errorf("Expected at most 5 cells, got %d", len(worst))
	}

	// Should be sorted by GDOP (worst first)
	for i := 1; i < len(worst); i++ {
		prevGDOP := worst[i-1].GDOP
		currGDOP := worst[i].GDOP

		// Handle infinity comparison
		prevInf := math.IsInf(prevGDOP, 0)
		currInf := math.IsInf(currGDOP, 0)

		if prevInf && !currInf {
			t.Errorf("Cell %d should have infinity (worst), but doesn't", i-1)
		}
		if !prevInf && !currInf && currGDOP > prevGDOP {
			t.Errorf("Cells not sorted by GDOP: [%d]=%f, [%d]=%f", i-1, prevGDOP, i, currGDOP)
		}
	}
}

func TestGetBestCoverageCells(t *testing.T) {
	space := DefaultSpace()
	nodes := SuggestedNodes(space, 4)
	links := GenerateAllLinks(nodes)

	minX, minY, _, maxX, maxY, _ := space.Bounds()

	config := GridConfig{
		MinX:     minX,
		MinY:     minY,
		Width:    maxX - minX,
		Depth:    maxY - minY,
		CellSize: 0.5,
	}

	gc := NewGDOPComputer(links, config)
	results := gc.ComputeAll()

	best := gc.GetBestCoverageCells(results, 5)

	// Should return at most 5 cells
	if len(best) > 5 {
		t.Errorf("Expected at most 5 cells, got %d", len(best))
	}

	// Should be sorted by GDOP (best first)
	for i := 1; i < len(best); i++ {
		prevGDOP := best[i-1].GDOP
		currGDOP := best[i].GDOP

		// Handle infinity comparison
		prevInf := math.IsInf(prevGDOP, 0)
		currInf := math.IsInf(currGDOP, 0)

		if !prevInf && currInf {
			t.Errorf("Cell %d should have finite (best), but has infinity", i)
		}
		if !prevInf && !currInf && currGDOP < prevGDOP {
			t.Errorf("Cells not sorted by GDOP: [%d]=%f, [%d]=%f", i-1, prevGDOP, i, currGDOP)
		}
	}

	// All best cells should have good or excellent GDOP (< 4)
	for i, cell := range best {
		if !math.IsInf(cell.GDOP, 0) && cell.GDOP >= 4.0 {
			t.Errorf("Best cell %d has GDOP %f, which is not 'good'", i, cell.GDOP)
		}
	}
}

func TestEngineRunSimulation(t *testing.T) {
	space := DefaultSpace()
	engine := NewEngine(space)

	// Add some virtual nodes
	nodes := SuggestedNodes(space, 4)
	for _, node := range nodes.All() {
		err := engine.AddVirtualNode(node)
		if err != nil {
			t.Fatalf("Failed to add virtual node: %v", err)
		}
	}

	// Add a walker
	walker := &SimWalker{
		ID:       "walker-1",
		Type:     WalkerTypeRandomWalk,
		Position: NewPoint(3, 2.5, 1.7),
		Velocity: NewPoint(0.1, 0.1, 0),
	}
	engine.AddWalker(walker)

	// Run simulation
	result := engine.RunSimulation()

	// Verify results
	if result == nil {
		t.Fatal("Expected non-nil simulation result")
	}

	// Should have some data
	if len(result.GridDimensions) != 3 {
		t.Errorf("Expected 3 grid dimensions, got %d", len(result.GridDimensions))
	}

	if len(result.GDOPMap) == 0 {
		t.Error("Expected non-empty GDOP map")
	}

	if result.CoverageScore < 0 || result.CoverageScore > 100 {
		t.Errorf("Coverage score %f outside [0, 100] range", result.CoverageScore)
	}
}

func TestPhysicsModelDeltaRMS(t *testing.T) {
	pm := NewPhysicsModel(DefaultSpace())

	tx := NewPoint(0, 0, 2)
	rx := NewPoint(5, 0, 2)

	// Test at midpoint (zone 1)
	walker := NewPoint(2.5, 0, 1.7)
	deltaRMS := pm.DeltaRMS(tx, rx, walker)

	// Zone 1 should have high deltaRMS
	if deltaRMS < 0.1 {
		t.Errorf("Zone 1 deltaRMS %f too low", deltaRMS)
	}
}

func TestPhysicsModelPhaseAtSubcarrier(t *testing.T) {
	pm := NewPhysicsModel(DefaultSpace())

	tx := NewPoint(0, 0, 2)
	rx := NewPoint(5, 0, 2)
	walker := NewPoint(2.5, 0, 1.7)

	// Test multiple subcarriers
	for k := 0; k < 10; k++ {
		phase := pm.PhaseAtSubcarrier(tx, rx, walker, k, 0)

		// Phase should be in [-π, π]
		if phase < -math.Pi || phase > math.Pi {
			t.Errorf("Subcarrier %d: phase %f is outside [-π, π]", k, phase)
		}
	}
}

func TestComputeFresnelModulation(t *testing.T) {
	tx := NewPoint(0, 0, 2)
	rx := NewPoint(6, 0, 2)

	// Zone 1 (on direct path) - maximum modulation
	midpoint := NewPoint(3, 0, 2)
	modulation := ComputeFresnelModulation(tx, rx, midpoint)
	if modulation != 1.0 {
		t.Errorf("Zone 1 should have modulation 1.0, got %f", modulation)
	}

	// Far from direct path - low modulation
	farPoint := NewPoint(3, 10, 2)
	farModulation := ComputeFresnelModulation(tx, rx, farPoint)
	if farModulation >= modulation {
		t.Errorf("Far point should have lower modulation than midpoint")
	}
}

func TestComputeLinkQuality(t *testing.T) {
	// Well-spread nodes should have good quality
	nodes := []Point{
		NewPoint(0, 0, 2),
		NewPoint(10, 0, 2),
		NewPoint(0, 10, 2),
		NewPoint(10, 10, 2),
	}
	quality := ComputeLinkQuality(nodes)
	if quality < 0.5 {
		t.Errorf("Well-spread nodes should have quality >= 0.5, got %f", quality)
	}

	// Clustered nodes should have poor quality
	clustered := []Point{
		NewPoint(5, 5, 2),
		NewPoint(5.1, 5, 2),
		NewPoint(5, 5.1, 2),
		NewPoint(5.1, 5.1, 2),
	}
	clusterQuality := ComputeLinkQuality(clustered)
	if clusterQuality >= quality {
		t.Errorf("Clustered nodes should have lower quality than spread nodes")
	}
}

func TestValidateRSSI(t *testing.T) {
	pm := NewPhysicsModel(DefaultSpace())

	// Test various distances
	distances := []float64{1.0, 5.0, 10.0, 20.0}
	for _, dist := range distances {
		rssi := pm.ComputeRSSI(dist)
		if !ValidateRSSI(rssi, dist) {
			t.Errorf("RSSI %d at distance %f should be valid", rssi, dist)
		}
	}

	// Invalid RSSI for distance
	if ValidateRSSI(-30, 100.0) {
		t.Error("RSSI -30 at 100m distance should be invalid")
	}
}
