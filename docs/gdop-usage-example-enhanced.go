// ============================================
// GDOP Function Usage Example
// ============================================
//
// This file demonstrates how to use GDOP (Geometric Dilution of Precision)
// computation functions in Spaxel to evaluate coverage quality and node placement.
//
// GDOP quantifies how well the geometric arrangement of nodes can localize a point.
// Lower GDOP values indicate better coverage quality.
//
// To run these examples, copy this file to the simulator package directory:
//   mothership/internal/simulator/
// Then run:
//   go run gdop_usage_example_enhanced.go
//
// Or run directly from docs/ (if simulator is imported):
//   go run gdop_usage_example_enhanced.go

package main

import (
	"fmt"
	"math"

	"spaxel/mothership/internal/simulator"
)

// ============================================
// Example 1: Basic GDOP Computation at a Single Point
// ============================================
//
// This example shows how to compute GDOP at a specific point
// to evaluate coverage quality at that location.
//
// Learning objectives:
// - How to create nodes with realistic positions
// - How to generate links between nodes
// - How to compute GDOP at a point
// - How to interpret the GDOP result
//
// Expected output:
// - GDOP value around 1.58 for 4 well-positioned corner nodes
// - Quality rating: "excellent"
// - Multiple contributing links (angular diversity)
func Example1_SinglePointGDOP() {
	fmt.Println("=== Example 1: Single Point GDOP ===\n")

	// Step 1: Create nodes at corners of a 10x10m room at 2m height
	//
	// Parameters explained:
	// - ID (1st arg): Unique identifier for the node
	//   * Use descriptive names for debugging ("kitchen-north", "living-south")
	//   * Must be unique within the node set
	// - Name (2nd arg): Human-readable label shown in dashboard
	//   * More descriptive than ID (e.g., "Kitchen North Sensor")
	// - NodeTypeVirtual (3rd arg): Indicates this is a simulated node
	//   * Use NodeTypeReal for physical ESP32-S3 hardware
	//   * Virtual nodes are for pre-deployment planning
	// - Position (4th arg): 3D coordinates in meters
	//   * X, Y: Floor plan position (0,0 = one corner)
	//   * Z: Height above floor in meters
	//   * Typical heights: 2.0m (ceiling), 0.3m (desk), 0.0m (floor)
	//
	// Realistic parameter values:
	// - Room size: 3-6m per dimension for typical residential rooms
	// - Node spacing: 3-10m between nodes for good angular diversity
	// - Heights: Mix of ceiling (2m) and desk level (0.3m) for Z-axis accuracy
	nodes := []*simulator.Node{
		simulator.NewNode("node1", "Kitchen North", simulator.NodeTypeVirtual,
			simulator.Point{X: 0, Y: 0, Z: 2.0}),
		simulator.NewNode("node2", "Kitchen South", simulator.NodeTypeVirtual,
			simulator.Point{X: 10, Y: 0, Z: 2.0}),
		simulator.NewNode("node3", "Living Room West", simulator.NodeTypeVirtual,
			simulator.Point{X: 0, Y: 10, Z: 2.0}),
		simulator.NewNode("node4", "Living Room East", simulator.NodeTypeVirtual,
			simulator.Point{X: 10, Y: 10, Z: 2.0}),
	}

	// Step 2: Set all nodes as TX_RX for bidirectional links
	//
	// Role options:
	// - "tx": Transmit only (sends CSI probes)
	// - "rx": Receive only (captures CSI from TX nodes)
	// - "tx_rx": Both transmit and receive (maximizes link count)
	// - "passive": RX-only using AP as TX (passive radar mode)
	// - "idle": Disabled (no participation)
	//
	// For best coverage with few nodes, use TX_RX mode.
	// This creates the maximum number of links for GDOP calculation.
	for _, node := range nodes {
		node.Role = simulator.RoleTXRX
	}

	// Step 3: Generate links from nodes
	//
	// Links represent TX→RX communication pairs.
	// With 4 nodes in TX_RX mode:
	// - Each node can transmit to 3 other nodes
	// - Total unidirectional links: 4 × 3 = 12
	// - Each bidirectional link counts as 2 unidirectional links
	//
	// Links are the fundamental unit for GDOP calculation.
	// More links with diverse angles = better GDOP.
	nodeSet := simulator.NewNodeSet()
	for _, node := range nodes {
		nodeSet.Add(node)
	}
	links := simulator.GenerateAllLinks(nodeSet)

	fmt.Printf("Generated %d links from %d nodes\n", len(links), len(nodes))

	// Step 4: Create GDOP computer with grid configuration
	//
	// GridConfig parameters:
	// - MinX, MinY: Grid origin coordinates in meters
	//   * (0, 0) = lower-left corner of evaluation space
	//   * Can be negative for positions outside room bounds
	// - Width: X dimension of space to evaluate (meters)
	//   * Extends from MinX to MinX + Width
	// - Depth: Y dimension of space to evaluate (meters)
	//   * Extends from MinY to MinY + Depth
	// - CellSize: Grid resolution in meters
	//   * 0.2m (20cm) = high resolution, good for detailed analysis
	//   * 0.5m = faster computation, good for real-time updates
	//   * Smaller values = more cells = more computation time
	//
	// For a 10×10m space with 0.2m cells:
	// - Columns: 10.0 / 0.2 = 50 cells
	// - Rows: 10.0 / 0.2 = 50 cells
	// - Total cells: 50 × 50 = 2,500 cells
	gc := simulator.NewGDOPComputer(links, simulator.GridConfig{
		MinX:     0.0,
		MinY:     0.0,
		Width:    10.0,
		Depth:    10.0,
		CellSize: 0.2,
	})

	// Step 5: Compute GDOP at center of room (5m, 5m, 1m height)
	//
	// ComputeAt parameters:
	// - x: X coordinate in meters (5.0 = center of 10m wide room)
	// - y: Y coordinate in meters (5.0 = center of 10m deep room)
	// - z: Z coordinate in meters (1.0 = typical person height)
	//
	// Return value (GDOPResult struct):
	// - X, Y, Z: Input coordinates echoed back
	// - GDOP: Computed GDOP value (lower = better coverage)
	// - Quality: Human-readable quality string
	// - ContributingLinks: List of link IDs covering this point
	//
	// GDOP interpretation:
	// - < 2.0: Excellent coverage (±0.5-1.0m accuracy expected)
	// - 2.0-4.0: Good coverage (±1.0-2.0m accuracy expected)
	// - 4.0-8.0: Fair coverage (±2-4m accuracy, may miss nearby people)
	// - ≥ 8.0: Poor coverage (>±4m accuracy or unreliable)
	// - Infinity: No coverage (insufficient links or collinear geometry)
	result := gc.ComputeAt(5.0, 5.0, 1.0)

	// Step 6: Interpret and display the result
	fmt.Println("\nResult for center of room (5m, 5m, 1m):")
	fmt.Printf("  GDOP Value: %.2f\n", result.GDOP)
	fmt.Printf("  Quality Rating: %s\n", result.Quality)
	fmt.Printf("  Contributing Links: %d\n", len(result.ContributingLinks))

	// Show specific contributing links
	fmt.Println("\nContributing link IDs:")
	for i, linkID := range result.ContributingLinks {
		if i < 6 {
			fmt.Printf("    - %s\n", linkID)
		}
	}
	if len(result.ContributingLinks) > 6 {
		fmt.Printf("    ... and %d more\n", len(result.ContributingLinks)-6)
	}

	// Calculate expected accuracy
	accuracy := simulator.ExpectedAccuracy(result.GDOP)
	if !math.IsInf(accuracy, 1) {
		fmt.Printf("\nExpected localization accuracy: ±%.2f meters\n", accuracy)
	}

	// Expected output for 4 corner nodes:
	// GDOP Value: ~1.58 (excellent - good angular diversity)
	// Quality Rating: excellent
	// Contributing Links: 12 (all node pairs)
	// Expected accuracy: ±0.79 meters
	fmt.Println("\n---")
}

// ============================================
// Example 2: Grid-Based Coverage Map
// ============================================
//
// This example shows how to compute GDOP for an entire grid
// to visualize coverage quality across a space.
//
// Learning objectives:
// - How to compute GDOP for all cells in a grid
// - How to calculate coverage statistics
// - How to interpret quality distribution
//
// Expected output:
// - High coverage score (>85%) for well-positioned nodes
// - Most cells rated "excellent" or "good"
func Example2_GridCoverageMap() {
	fmt.Println("\n=== Example 2: Grid Coverage Map ===\n")

	// Step 1: Create same 4-node setup (reuse from Example 1)
	nodes := []*simulator.Node{
		simulator.NewNode("node1", "Node 1", simulator.NodeTypeVirtual,
			simulator.Point{X: 0, Y: 0, Z: 2.0}),
		simulator.NewNode("node2", "Node 2", simulator.NodeTypeVirtual,
			simulator.Point{X: 10, Y: 0, Z: 2.0}),
		simulator.NewNode("node3", "Node 3", simulator.NodeTypeVirtual,
			simulator.Point{X: 0, Y: 10, Z: 2.0}),
		simulator.NewNode("node4", "Node 4", simulator.NodeTypeVirtual,
			simulator.Point{X: 10, Y: 10, Z: 2.0}),
	}

	for _, node := range nodes {
		node.Role = simulator.RoleTXRX
	}

	nodeSet := simulator.NewNodeSet()
	for _, node := range nodes {
		nodeSet.Add(node)
	}
	links := simulator.GenerateAllLinks(nodeSet)

	// Step 2: Create GDOP computer
	gc := simulator.NewGDOPComputer(links, simulator.GridConfig{
		MinX:     0.0,
		MinY:     0.0,
		Width:    10.0,
		Depth:    10.0,
		CellSize: 0.2,
	})

	// Step 3: Compute GDOP for entire grid
	//
	// ComputeAll returns: 2D slice indexed by [row][column]
	// - results[iy][ix] = cell at row iy, column ix
	// - iy = 0 to ny-1 (rows, Y dimension)
	// - ix = 0 to nx-1 (columns, X dimension)
	//
	// Cell position calculation:
	// - X = MinX + (ix + 0.5) * CellSize
	// - Y = MinY + (iy + 0.5) * CellSize
	// - The +0.5 ensures we get the cell center
	//
	// For 10×10m with 0.2m cells:
	// - nx = 50 columns, ny = 50 rows
	// - Cell [0][0]: (0.1, 0.1) = 10cm from origin
	// - Cell [25][25]: (5.0, 5.0) = center of room
	results := gc.ComputeAll()

	// Step 4: Compute coverage statistics
	//
	// CoverageScore: Percentage of cells with "good" or better quality
	// - "excellent" (GDOP < 2): ±0.5m accuracy
	// - "good" (GDOP 2-4): ±1.0m accuracy
	// - Higher score = better overall coverage
	//
	// AverageGDOP: Mean GDOP across all valid cells
	// - Excludes cells with GDOP = Infinity (no coverage)
	// - Lower average = better coverage
	//
	// QualityCounts: Breakdown of cells by quality level
	// - Use this to identify distribution of coverage quality
	coverageScore := gc.CoverageScore(results)
	averageGDOP := gc.AverageGDOP(results)
	qualityCounts := gc.QualityCounts(results)

	nRows := len(results)
	nCols := len(results[0])
	totalCells := nRows * nCols

	fmt.Printf("Grid Analysis Results:\n")
	fmt.Printf("  Grid size: %d × %d (%d cells)\n", nCols, nRows, totalCells)
	fmt.Printf("  Coverage Score: %.1f%% (good or better)\n", coverageScore)
	fmt.Printf("  Average GDOP: %.2f\n", averageGDOP)
	fmt.Printf("\nQuality Distribution:\n")

	// Calculate and display percentages
	for _, quality := range []string{"excellent", "good", "fair", "poor", "none"} {
		count := qualityCounts[quality]
		percent := 100.0 * float64(count) / float64(totalCells)
		fmt.Printf("  %-9s: %4d cells (%5.1f%%)\n", quality, count, percent)
	}

	// Expected output for 4 corner nodes:
	// Coverage Score: ~95% (most cells have excellent/good coverage)
	// Average GDOP: ~1.8 (excellent overall)
	// Quality Distribution: Mostly excellent/good, minimal poor/none

	// Step 5: Access specific cells
	fmt.Println("\nCell Access Examples:")

	// Center cell (row 25, col 25)
	centerRow, centerCol := 25, 25
	centerCell := results[centerRow][centerCol]
	fmt.Printf("  Center cell (%.1fm, %.1fm):\n", centerCell.X, centerCell.Y)
	fmt.Printf("    GDOP: %.2f (%s)\n", centerCell.GDOP, centerCell.Quality)
	fmt.Printf("    Links: %d\n", len(centerCell.ContributingLinks))

	// Corner cell (row 0, col 0)
	cornerCell := results[0][0]
	fmt.Printf("\n  Corner cell (%.1fm, %.1fm):\n", cornerCell.X, cornerCell.Y)
	fmt.Printf("    GDOP: %.2f (%s)\n", cornerCell.GDOP, cornerCell.Quality)
	fmt.Printf("    Links: %d\n", len(cornerCell.ContributingLinks))

	fmt.Println("\n---")
}

// ============================================
// Example 3: Evaluating Node Repositioning
// ============================================
//
// This example shows how to evaluate whether moving a node
// would improve overall coverage quality.
//
// Learning objectives:
// - How to identify suboptimal node placement
// - How to compute improvement from repositioning
// - How to interpret improvement percentage
//
// Expected output:
// - Significant improvement (30-60%) from fixing poor geometry
func Example3_NodeRepositioning() {
	fmt.Println("\n=== Example 3: Node Repositioning Evaluation ===\n")

	// Step 1: Create layout with suboptimal positioning
	//
	// Problem: node2 is only 1m from node1
	// This causes poor angular diversity and high GDOP
	nodes := []*simulator.Node{
		simulator.NewNode("node1", "Node 1", simulator.NodeTypeVirtual,
			simulator.Point{X: 0, Y: 0, Z: 2.0}),
		simulator.NewNode("node2", "Node 2", simulator.NodeTypeVirtual,
			simulator.Point{X: 1, Y: 0, Z: 2.0}), // Too close to node1!
		simulator.NewNode("node3", "Node 3", simulator.NodeTypeVirtual,
			simulator.Point{X: 0, Y: 10, Z: 2.0}),
	}

	for _, node := range nodes {
		node.Role = simulator.RoleTXRX
	}

	// Step 2: Compute current worst-case GDOP
	//
	// computeWorstGDOP finds the cell with highest GDOP across the entire space
	// A good layout should have LOW worst-case GDOP
	// (even the worst-covered area should be acceptable)
	//
	// This is important because:
	// - We want to ensure no area has terrible coverage
	// - High worst-case GDOP means there are coverage gaps
	// - Low worst-case GDOP means coverage is consistent across space
	currentWorstGDOP := simulator.ComputeWorstGDOPHelper(nodes)

	fmt.Printf("Current layout analysis:\n")
	fmt.Printf("  Worst-case GDOP: %.2f (%s)\n",
		currentWorstGDOP, gdopToQuality(currentWorstGDOP))
	fmt.Printf("  Current positions:\n")
	for _, node := range nodes {
		fmt.Printf("    %s: (%.1f, %.1f, %.1f)\n",
			node.Name, node.Position.X, node.Position.Y, node.Position.Z)
	}

	// Step 3: Evaluate moving node2 to a better position
	//
	// Target position: (10, 0, 2) - opposite corner from node1
	// This would create better angular diversity
	targetPos := simulator.Point{X: 10, Y: 0, Z: 2.0}

	fmt.Printf("\nEvaluating move: node2 → (%.1f, %.1f, %.1f)\n",
		targetPos.X, targetPos.Y, targetPos.Z)

	// Step 4: Compute improvement
	//
	// computeGDOPImprovement calculates:
	// improvement = (currentWorstGDOP - newWorstGDOP) / currentWorstGDOP
	//
	// Return value interpretation:
	// - Positive (0 to 1): Improvement
	//   * 0.3 = 30% improvement (worst GDOP reduced by 30%)
	//   * 1.0 = Maximum improvement (worst GDOP near zero)
	// - Negative (-1 to 0): Degradation
	//   * -0.5 = 50% degradation (worst GDOP increased by 50%)
	//   * -1.0 = Complete coverage loss (new GDOP = Infinity)
	// - 0.0: No change (same GDOP or node not found)
	//
	// Formula breakdown:
	// 1. Compute worst GDOP for current layout
	// 2. Create hypothetical layout with node moved
	// 3. Compute worst GDOP for hypothetical layout
	// 4. Calculate relative improvement
	improvement := simulator.ComputeGDOPImprovementHelper(nodes, "node2", targetPos)

	fmt.Printf("\nResults:\n")
	if improvement > 0 {
		fmt.Printf("  ✓ Expected improvement: %.1f%%\n", improvement*100)
		fmt.Printf("    (worst-case GDOP would decrease by %.1f%%)\n", improvement*100)
	} else if improvement < 0 {
		fmt.Printf("  ✗ Expected degradation: %.1f%%\n", improvement*100)
		fmt.Printf("    (worst-case GDOP would increase by %.1f%%)\n", -improvement*100)
	} else {
		fmt.Printf("  = No change in coverage quality\n")
	}

	// Expected output:
	// Current worst-case GDOP: > 5.0 (poor, nodes too close)
	// Expected improvement: ~40-60% (significant improvement from better geometry)

	fmt.Println("\nInterpretation:")
	fmt.Println("  Moving node2 from (1,0) to (10,0) creates better angular diversity")
	fmt.Println("  This significantly improves worst-case coverage across the space")

	fmt.Println("\n---")
}

// ============================================
// Example 4: Finding Dead Zones
// ============================================
//
// This example shows how to identify areas with poor coverage
// to guide node placement decisions.
//
// Learning objectives:
// - How to find dead zones (areas with poor/no coverage)
// - How to get recommendations for new node placement
// - How to interpret dead zone analysis
//
// Expected output:
// - Identification of specific dead zone positions
// - Recommendation for optimal new node position
func Example4_FindingDeadZones() {
	fmt.Println("\n=== Example 4: Finding Dead Zones ===\n")

	// Step 1: Create 3-node layout (triangle formation)
	//
	// With only 3 nodes, some areas may have poor coverage
	// This helps demonstrate dead zone detection
	nodes := []*simulator.Node{
		simulator.NewNode("node1", "Node 1", simulator.NodeTypeVirtual,
			simulator.Point{X: 0, Y: 0, Z: 2.0}),
		simulator.NewNode("node2", "Node 2", simulator.NodeTypeVirtual,
			simulator.Point{X: 10, Y: 0, Z: 2.0}),
		simulator.NewNode("node3", "Node 3", simulator.NodeTypeVirtual,
			simulator.Point{X: 5, Y: 8, Z: 2.0}),
	}

	for _, node := range nodes {
		node.Role = simulator.RoleTXRX
	}

	nodeSet := simulator.NewNodeSet()
	for _, node := range nodes {
		nodeSet.Add(node)
	}
	links := simulator.GenerateAllLinks(nodeSet)

	// Step 2: Compute GDOP for entire space
	gc := simulator.NewGDOPComputer(links, simulator.GridConfig{
		MinX:     0.0,
		MinY:     0.0,
		Width:    10.0,
		Depth:    10.0,
		CellSize: 0.2,
	})
	results := gc.ComputeAll()

	// Step 3: Find dead zones
	//
	// FindDeadZones returns positions where coverage is "none" or "poor"
	// - "none": GDOP = Infinity (< 2 covering links)
	// - "poor": GDOP ≥ 8.0 (significant coverage gaps)
	//
	// Use this to:
	// - Identify where additional nodes are needed
	// - Understand coverage limitations of current layout
	// - Guide placement of new nodes
	deadZones := gc.FindDeadZones(results)

	fmt.Printf("Dead Zone Analysis:\n")
	fmt.Printf("  Total dead zones: %d\n", len(deadZones))

	if len(deadZones) > 0 {
		fmt.Printf("\n  Sample dead zone positions (first 5):\n")
		for i, dz := range deadZones {
			if i < 5 {
				// Find quality for this dead zone
				quality := "?"
				for _, row := range results {
					for _, cell := range row {
						if cell.X == dz.X && cell.Y == dz.Y {
							quality = cell.Quality
							break
						}
					}
				}
				fmt.Printf("    (%.1f, %.1f, %.1f) - %s\n", dz.X, dz.Y, dz.Z, quality)
			}
		}
		if len(deadZones) > 5 {
			fmt.Printf("    ... and %d more\n", len(deadZones)-5)
		}
	}

	// Step 4: Get recommendation for new node placement
	//
	// RecommendNodePosition suggests where to place an additional node
	// based on the centroid of dead zones
	//
	// Algorithm:
	// 1. If no dead zones: suggest center of space
	// 2. If dead zones exist: calculate centroid
	// 3. Return position at 2m height (good for coverage)
	//
	// Use this to:
	// - Decide where to place the next node
	// - Prioritize node purchases for maximum coverage improvement
	space := simulator.NewSpace(10.0, 10.0, 3.0)
	recommendation := gc.RecommendNodePosition(results, space)

	fmt.Printf("\nRecommendation:\n")
	fmt.Printf("  Optimal position for new node: (%.1f, %.1f, %.1f)\n",
		recommendation.X, recommendation.Y, recommendation.Z)
	fmt.Printf("  (at centroid of dead zones for maximum coverage improvement)\n")

	fmt.Println("\nUsage:")
	fmt.Println("  Use this analysis to:")
	fmt.Println("  - Decide where to place additional nodes")
	fmt.Println("  - Prioritize which area needs coverage most")
	fmt.Println("  - Understand coverage limitations of current layout")

	fmt.Println("\n---")
}

// ============================================
// Example 5: GDOP Color Mapping for Visualization
// ============================================
//
// This example shows how to convert GDOP values to colors
// for heatmap visualization in the dashboard.
//
// Learning objectives:
// - How to map GDOP values to RGB colors
// - How to understand the color scheme
// - How to use colors for visualization
//
// Expected output:
// - Color mapping table showing all quality levels
func Example5_ColorVisualization() {
	fmt.Println("\n=== Example 5: Color Visualization ===\n")

	// Step 1: Define test GDOP values covering all quality levels
	testCases := []struct {
		gdop    float64
		quality string
	}{
		{1.5, "excellent"},
		{3.0, "good"},
		{6.0, "fair"},
		{10.0, "poor"},
		{math.Inf(1), "none"},
	}

	// Step 2: Convert GDOP to color
	//
	// GDOPColorMap returns RGB values for heatmap visualization:
	// - Green (#22c65e): Excellent (GDOP < 2.0)
	// - Yellow (#ffc107): Good (GDOP 2.0-4.0)
	// - Orange (#ff9200): Fair (GDOP 4.0-8.0)
	// - Red (#dc3545): Poor (GDOP ≥ 8.0)
	// - Gray (#808080): None (Infinity)
	//
	// Use this for:
	// - Dashboard coverage heatmaps
	// - Mobile app visualization
	// - Coverage quality reports
	fmt.Println("GDOP Color Mapping:")
	fmt.Println("─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─")
	fmt.Printf("%-12s %-10s %-12s %s\n", "GDOP", "Quality", "RGB Color", "Hex Color")
	fmt.Println("─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─")

	for _, tc := range testCases {
		color := simulator.GDOPColorMap(tc.gdop)
		hex := fmt.Sprintf("#%02x%02x%02x", color.R, color.G, color.B)

		var gdopStr string
		if math.IsInf(tc.gdop, 1) {
			gdopStr = "Infinity"
		} else {
			gdopStr = fmt.Sprintf("%.1f", tc.gdop)
		}

		fmt.Printf("%-12s %-10s RGB(%3d,%3d,%3d) %s\n",
			gdopStr, tc.quality, color.R, color.G, color.B, hex)
	}

	fmt.Println("\nVisualization usage:")
	fmt.Println("  Green areas: Excellent coverage (reliable detection)")
	fmt.Println("  Yellow areas: Good coverage (acceptable detection)")
	fmt.Println("  Orange areas: Fair coverage (may miss nearby people)")
	fmt.Println("  Red areas: Poor coverage (unreliable detection)")
	fmt.Println("  Gray areas: No coverage (cannot localize)")

	fmt.Println("\n---")
}

// ============================================
// Example 6: Expected Accuracy Estimation
// ============================================
//
// This example shows how to estimate localization accuracy
// from GDOP values for system planning.
//
// Learning objectives:
// - How to estimate expected accuracy from GDOP
// - How to plan deployment based on accuracy requirements
// - How to interpret accuracy for different use cases
//
// Expected output:
// - Accuracy estimates for all quality levels
// - Practical guidelines for system planning
func Example6_ExpectedAccuracy() {
	fmt.Println("\n=== Example 6: Expected Accuracy ===\n")

	// Step 1: Test accuracy estimation across GDOP range
	testCases := []struct {
		gdop    float64
		quality string
	}{
		{1.0, "excellent"},
		{1.5, "excellent"},
		{2.0, "good"},
		{3.0, "good"},
		{4.0, "fair"},
		{6.0, "fair"},
		{8.0, "poor"},
	}

	fmt.Println("Localization Accuracy Estimation:")
	fmt.Println("(Based on typical CSI performance with 4+ nodes)")
	fmt.Println()
	fmt.Println("─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─")
	fmt.Printf("%-10s %-12s %-20s %s\n", "GDOP", "Quality", "Expected Accuracy", "Use Case")
	fmt.Println("─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─")

	for _, tc := range testCases {
		accuracy := simulator.ExpectedAccuracy(tc.gdop)
		accuracyStr := fmt.Sprintf("±%.2fm", accuracy)

		var useCase string
		switch {
		case tc.gdop < 2:
			useCase = "Fall detection, high-accuracy tracking"
		case tc.gdop < 4:
			useCase = "Room-level presence, general tracking"
		case tc.gdop < 8:
			useCase = "Zone-level presence (which room)"
		default:
			useCase = "Coarse detection only"
		}

		fmt.Printf("%-10.1f %-12s %-20s %s\n", tc.gdop, tc.quality, accuracyStr, useCase)
	}

	fmt.Println("\nPractical Guidelines:")
	fmt.Println("─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─")
	fmt.Println("GDOP < 2 (Excellent):")
	fmt.Println("  ✓ Can reliably detect person within ±0.5-1.0m")
	fmt.Println("  ✓ Suitable for fall detection")
	fmt.Println("  ✓ Can distinguish standing vs. lying down")
	fmt.Println("")
	fmt.Println("GDOP 2-4 (Good):")
	fmt.Println("  ✓ Can reliably detect person within ±1.0-2.0m")
	fmt.Println("  ✓ Suitable for room-level presence detection")
	fmt.Println("  ✓ Can track movement between zones")
	fmt.Println("")
	fmt.Println("GDOP 4-8 (Fair):")
	fmt.Println("  ⚠ Accuracy degrades to ±2-4m")
	fmt.Println("  ⚠ May miss people standing nearby")
	fmt.Println("  ⚠ Suitable for coarse zone detection only")
	fmt.Println("")
	fmt.Println("GDOP > 8 (Poor):")
	fmt.Println("  ✗ Unreliable localization")
	fmt.Println("  ✗ May fail to detect people in same room")
	fmt.Println("  ✗ Needs additional nodes or better placement")
	fmt.Println("")
	fmt.Println("GDOP = Infinity (None):")
	fmt.Println("  ✗ Cannot localize at all")
	fmt.Println("  ✗ Insufficient nodes or bad geometry")
	fmt.Println("  ✗ Must add nodes or reposition existing ones")

	fmt.Println("\n---")
}

// ============================================
// Example 7: Minimum Node Count Estimation
// ============================================
//
// This example shows how to estimate the minimum number of nodes
// needed for a given space and target quality.
//
// Learning objectives:
// - How to calculate minimum nodes for different space sizes
// - How quality targets affect node count requirements
// - How to plan deployment budget
//
// Expected output:
// - Node count recommendations for different scenarios
func Example7_MinimumNodeCount() {
	fmt.Println("\n=== Example 7: Minimum Node Count ===\n")

	// Step 1: Define space parameters
	spaceSizes := []struct {
		width, depth float64
		description string
	}{
		{6.0, 5.0, "Small bedroom (30 m²)"},
		{10.0, 10.0, "Large living room (100 m²)"},
		{15.0, 20.0, "Open plan area (300 m²)"},
	}

	// Step 2: Define quality targets
	qualityTargets := []struct {
		gdop    float64
		quality string
	}{
		{2.0, "Excellent (GDOP < 2)"},
		{4.0, "Good (GDOP < 4)"},
		{6.0, "Fair (GDOP < 8)"},
	}

	fmt.Println("Minimum Node Count Recommendations:")
	fmt.Println()

	for _, space := range spaceSizes {
		fmt.Printf("%s (%.1fm × %.1fm):\n", space.description, space.width, space.depth)
		fmt.Println("─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─")

		space := simulator.NewSpace(space.width, space.depth, 2.5)

		for _, target := range qualityTargets {
			// Step 3: Estimate minimum node count
			//
			// MinimumNodeCount estimates based on area and desired GDOP:
			// - GDOP < 2 (excellent): ~1 node per 15 m²
			// - GDOP < 4 (good): ~1 node per 20 m²
			// - GDOP < 8 (fair): ~1 node per 30 m²
			//
			// This is a heuristic based on typical deployments
			// Actual requirements depend on:
			// - Room shape (long narrow rooms need more nodes)
			// - Obstacles (walls block signals, need more nodes)
			// - Ceiling height (affects Fresnel zone geometry)
			minNodes := simulator.MinimumNodeCount(space, target.gdop)

			fmt.Printf("  %s: %2d nodes\n", target.quality, minNodes)
		}
		fmt.Println()
	}

	fmt.Println("Planning Guidelines:")
	fmt.Println("─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─")
	fmt.Println("For excellent coverage (GDOP < 2):")
	fmt.Println("  • High-accuracy applications (fall detection)")
	fmt.Println("  • Requires more nodes (1 per 15 m²)")
	fmt.Println("  • Best for critical safety applications")
	fmt.Println("")
	fmt.Println("For good coverage (GDOP < 4):")
	fmt.Println("  • General presence detection")
	fmt.Println("  • Room-level tracking")
	fmt.Println("  • Good balance of cost vs. performance")
	fmt.Println("")
	fmt.Println("For fair coverage (GDOP < 8):")
	fmt.Println("  • Zone-level presence only")
	fmt.Println("  • Coarse localization")
	fmt.Println("  • Minimum viable deployment")

	fmt.Println("\nImportant Notes:")
	fmt.Println("─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─")
	fmt.Println("• These are minimum estimates - actual deployment may vary")
	fmt.Println("• Mixed-height nodes improve Z-axis accuracy")
	fmt.Println("• Corner placement maximizes angular diversity")
	fmt.Println("• Avoid placing all nodes in a straight line")
	fmt.Println("• Consider obstacles (walls, furniture) in placement")

	fmt.Println("\n---")
}

// ============================================
// Helper Functions
// ============================================

// gdopToQuality converts a GDOP value to a quality string
func gdopToQuality(gdop float64) string {
	if math.IsInf(gdop, 1) {
		return "none"
	}
	if gdop < 2.0 {
		return "excellent"
	}
	if gdop < 4.0 {
		return "good"
	}
	if gdop < 8.0 {
		return "fair"
	}
	return "poor"
}

// ============================================
// Main: Run All Examples
// ============================================

func main() {
	fmt.Println("==========================================")
	fmt.Println("  GDOP Function Usage Examples")
	fmt.Println("  Geometric Dilution of Precision in Spaxel")
	fmt.Println("==========================================")
	fmt.Println()
	fmt.Println("This file demonstrates how to use GDOP computation")
	fmt.Println("functions to evaluate coverage quality and node placement.")
	fmt.Println()
	fmt.Println("GDOP quantifies how well the geometric arrangement of nodes")
	fmt.Println("can localize a point. Lower GDOP values indicate better coverage.")
	fmt.Println()

	// Run all examples
	Example1_SinglePointGDOP()
	Example2_GridCoverageMap()
	Example3_NodeRepositioning()
	Example4_FindingDeadZones()
	Example5_ColorVisualization()
	Example6_ExpectedAccuracy()
	Example7_MinimumNodeCount()

	fmt.Println("==========================================")
	fmt.Println("All examples completed!")
	fmt.Println("==========================================")
}

// ============================================
// Quick Reference Guide
// ============================================
//
// GDOP Value Interpretation:
// ─────────────────────────────────
// GDOP Range    | Quality   | Accuracy      | Use Cases
// --------------+----------+--------------+------------------------------
// < 2.0        | Excellent | ±0.5-1.0m    | Fall detection, precise tracking
// 2.0 - 4.0    | Good      | ±1.0-2.0m    | Room presence, general tracking
// 4.0 - 8.0    | Fair     | ±2-4m       | Zone-level presence
// > 8.0        | Poor     | >±4m        | Coarse detection only
// Infinity     | None     | Cannot localize | Add nodes or reposition
//
// Function Reference:
// ─────────────────────────────────
// NewGDOPComputer(links, config)
//   - Creates GDOP computer for coverage analysis
//   - links: Slice of Link objects (TX→RX pairs)
//   - config: GridConfig with MinX, MinY, Width, Depth, CellSize
//
// ComputeAt(x, y, z)
//   - Computes GDOP at a single 3D point
//   - Returns GDOPResult with GDOP, Quality, ContributingLinks
//
// ComputeAll()
//   - Computes GDOP for entire grid
//   - Returns 2D slice of GDOPResult indexed by [row][col]
//
// CoverageScore(results)
//   - Calculates percentage of cells with "good" or better
//   - Returns float64 (0-100)
//
// AverageGDOP(results)
//   - Calculates mean GDOP across all valid cells
//   - Returns float64 (Infinity if no valid cells)
//
// QualityCounts(results)
//   - Counts cells by quality level
//   - Returns map[string]int with keys: excellent, good, fair, poor, none
//
// FindDeadZones(results)
//   - Returns positions with "poor" or "none" quality
//   - Returns []Point with X, Y, Z coordinates
//
// RecommendNodePosition(results, space)
//   - Suggests optimal position for additional node
//   - Returns Point with X, Y, Z coordinates
//
// MinimumNodeCount(space, targetGDOP)
//   - Estimates minimum nodes for space and quality target
//   - Returns int (minimum node count)
//
// ExpectedAccuracy(gdop)
//   - Estimates localization accuracy from GDOP
//   - Returns float64 (accuracy in meters)
//
// GDOPColorMap(gdop)
//   - Converts GDOP to RGB color for visualization
//   - Returns GDOPColor with R, G, B fields (0-255)
//
// Common Pitfalls:
// ─────────────────────────────────
// 1. Nodes too close together
//    • Symptom: High GDOP even with many nodes
//    • Fix: Space nodes 3-5m apart for angular diversity
//
// 2. All nodes at same height
//    • Symptom: Poor Z-axis accuracy
//    • Fix: Mix heights (ceiling 2m + desk 0.3m)
//
// 3. Nodes clustered in one area
//    • Symptom: Good coverage near cluster, poor elsewhere
//    • Fix: Distribute nodes around perimeter
//
// 4. Nodes in straight line
//    • Symptom: High GDOP perpendicular to line
//    • Fix: Arrange in triangle or square formation
//
// 5. Insufficient nodes for space size
//    • Symptom: Many cells with GDOP = Infinity
//    • Fix: Add more nodes (use MinimumNodeCount)
//
// Integration Points:
// ─────────────────────────────────
// These functions are used in:
// 1. Dashboard - Live Coverage Painting (real-time GDOP overlay)
// 2. Pre-Deployment Simulator (evaluate layouts before purchase)
// 3. Self-Healing Fleet (re-optimize after node failure)
// 4. Command Palette ("coverage quality", "evaluate position")
// 5. REST API (/api/coverage, /api/gdop/evaluate)
//
// Performance Considerations:
// ─────────────────────────────────
// • ComputeAt (single point): O(n) where n = number of links
// • ComputeAll (full grid): O(cols × rows × n)
//   - 50×50 grid with 12 links: ~30,000 operations (<2ms)
// • For real-time updates (dragging nodes): use 0.5m cells (coarser)
// • Maximum grid size: 100×100 cells (memory limit)
