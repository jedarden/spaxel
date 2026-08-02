// Package gdop_examples provides comprehensive usage examples for the GDOP (Geometric Dilution of Precision) computation system.
//
// GDOP quantifies how well the geometric arrangement of WiFi nodes can localize a point in space.
// Lower GDOP values indicate better coverage quality for indoor positioning.
//
// These examples demonstrate:
//   - Basic GDOP computation at single points
//   - Grid-based coverage mapping for spaces
//   - Error handling and edge cases
//   - Performance optimization strategies
//   - Common pitfalls and best practices
//   - Integration with the Spaxel positioning system
//
// Quality Thresholds:
//   - excellent: GDOP < 2.0  (±0.5m accuracy expected)
//   - good:      2.0 ≤ GDOP < 4.0  (±1.0m accuracy expected)
//   - fair:      4.0 ≤ GDOP < 8.0  (±2-4m accuracy expected)
//   - poor:      GDOP ≥ 8.0         (≥±4m accuracy or no coverage)
//   - none:      GDOP = Infinity    (insufficient coverage)
//
// Mathematical Background:
//   GDOP is computed from the Fisher information matrix F = HᵀH, where H contains
//   direction cosines from each link to the target point. For 2D localization:
//
//     GDOP = sqrt(trace(F⁻¹))
//
//   Where F is accumulated over all covering links as:
//     F = Σ [ [cos²(θ),       cos(θ)·sin(θ)],
//            [cos(θ)·sin(θ), sin²(θ)       ] ]
//
//   Geometric interpretation:
//     - Low GDOP: Nodes well-distributed in angle around the point
//     - High GDOP: Nodes collinear or clustered in one direction
//     - Infinite GDOP: Insufficient nodes (<2) or degenerate geometry
package gdop_examples

import (
	"fmt"
	"math"
	"time"

	"github.com/spaxel/spaxel/mothership/internal/simulator"
)

// Example 1: Basic Point Computation
//
// Demonstrates how to compute GDOP at a single point in a space.
// This is the most common usage for real-time coverage queries.
func ExampleBasicPointComputation() {
	// Step 1: Define node positions (corners of a 10x10m room at 2m height)
	nodes := []*simulator.Node{
		simulator.NewNode("node1", "Kitchen NW", simulator.NodeTypeReal,
			simulator.Point{X: 0.0, Y: 0.0, Z: 2.0}),
		simulator.NewNode("node2", "Kitchen NE", simulator.NodeTypeReal,
			simulator.Point{X: 10.0, Y: 0.0, Z: 2.0}),
		simulator.NewNode("node3", "Kitchen SW", simulator.NodeTypeReal,
			simulator.Point{X: 0.0, Y: 10.0, Z: 2.0}),
		simulator.NewNode("node4", "Kitchen SE", simulator.NodeTypeReal,
			simulator.Point{X: 10.0, Y: 10.0, Z: 2.0}),
	}

	// Step 2: Create communication links between nodes
	// In a real deployment, these would be actual TX→RX pairs from your fleet
	links := []simulator.Link{
		{TX: nodes[0], RX: nodes[1]}, // node1 → node2
		{TX: nodes[0], RX: nodes[2]}, // node1 → node3
		{TX: nodes[1], RX: nodes[3]}, // node2 → node4
		{TX: nodes[2], RX: nodes[3]}, // node3 → node4
	}

	// Step 3: Create GDOP computer with grid configuration
	computer := simulator.NewGDOPComputer(links, simulator.GridConfig{
		MinX:     0.0,                        // Grid origin X
		MinY:     0.0,                        // Grid origin Y
		Width:    10.0,                       // Room width (meters)
		Depth:    10.0,                       // Room depth (meters)
		CellSize: 0.2,                        // Grid cell size (20cm)
	})

	// Step 4: Compute GDOP at center of room (5m, 5m)
	centerResult := computer.ComputeAt(5.0, 5.0, 1.0)
	fmt.Printf("Center of room (5.0, 5.0):\n")
	fmt.Printf("  GDOP: %.2f\n", centerResult.GDOP)
	fmt.Printf("  Quality: %s\n", centerResult.Quality)
	fmt.Printf("  Contributing links: %v\n", centerResult.ContributingLinks)
	// Expected output: GDOP ~1.6, Quality: "excellent"
	// Four well-distributed corner nodes provide excellent angular diversity

	// Step 5: Compute GDOP at edge (near corner node)
	edgeResult := computer.ComputeAt(0.5, 0.5, 1.0)
	fmt.Printf("\nNear corner (0.5, 0.5):\n")
	fmt.Printf("  GDOP: %.2f\n", edgeResult.GDOP)
	fmt.Printf("  Quality: %s\n", edgeResult.Quality)
	// Expected output: GDOP > 4.0, Quality: "fair" or "poor"
	// Being too close to nodes reduces angular diversity
}

// Example 2: Grid-Based Coverage Mapping
//
// Demonstrates how to compute GDOP across an entire space to generate
// a coverage heatmap. This is used for node placement optimization and
// coverage quality visualization in the dashboard.
func ExampleGridBasedCoverageMapping() {
	// Define nodes in a suboptimal configuration (3 nodes clustered)
	nodes := []*simulator.Node{
		simulator.NewNode("node1", "Living Room NW", simulator.NodeTypeReal,
			simulator.Point{X: 0.0, Y: 0.0, Z: 2.0}),
		simulator.NewNode("node2", "Living Room NE", simulator.NodeTypeReal,
			simulator.Point{X: 1.0, Y: 0.0, Z: 2.0}), // Too close to node1!
		simulator.NewNode("node3", "Living Room SE", simulator.NodeTypeReal,
			simulator.Point{X: 10.0, Y: 10.0, Z: 2.0}),
	}

	// Create links (bidirectional communication)
	links := []simulator.Link{
		{TX: nodes[0], RX: nodes[1]},
		{TX: nodes[1], RX: nodes[0]},
		{TX: nodes[0], RX: nodes[2]},
		{TX: nodes[2], RX: nodes[0]},
		{TX: nodes[1], RX: nodes[2]},
		{TX: nodes[2], RX: nodes[1]},
	}

	// Create computer for 10x10m room with 0.5m cells (coarser grid for speed)
	computer := simulator.NewGDOPComputer(links, simulator.GridConfig{
		MinX:     0.0,
		MinY:     0.0,
		Width:    10.0,
		Depth:    10.0,
		CellSize: 0.5, // 50cm cells (20x20 grid instead of 50x50)
	})

	// Compute GDOP for entire grid
	startTime := time.Now()
	results := computer.ComputeAll()
	elapsed := time.Since(startTime)

	// Analyze coverage statistics
	excellent, good, fair, poor, none := 0, 0, 0, 0, 0
	totalCells := 0

	for _, row := range results {
		for _, result := range row {
			totalCells++
			switch result.Quality {
			case "excellent":
				excellent++
			case "good":
				good++
			case "fair":
				fair++
			case "poor":
				poor++
			case "none":
				none++
			}
		}
	}

	fmt.Printf("Coverage analysis for 10x10m room:\n")
	fmt.Printf("  Computation time: %v\n", elapsed)
	fmt.Printf("  Total cells: %d\n", totalCells)
	fmt.Printf("  Excellent: %d (%.1f%%)\n", excellent, float64(excellent)/float64(totalCells)*100)
	fmt.Printf("  Good: %d (%.1f%%)\n", good, float64(good)/float64(totalCells)*100)
	fmt.Printf("  Fair: %d (%.1f%%)\n", fair, float64(fair)/float64(totalCells)*100)
	fmt.Printf("  Poor: %d (%.1f%%)\n", poor, float64(poor)/float64(totalCells)*100)
	fmt.Printf("  No coverage: %d (%.1f%%)\n", none, float64(none)/float64(totalCells)*100)

	// Calculate coverage score (fraction with GDOP < 4)
	coverageScore := float64(excellent+good) / float64(totalCells) * 100
	fmt.Printf("  Coverage score: %.1f%%\n", coverageScore)
	// Expected: Low score (<60%) due to clustered nodes
}

// Example 3: Error Handling and Edge Cases
//
// Demonstrates proper error handling for common edge cases:
// - Insufficient nodes (<2)
// - Collinear node arrangements
// - Points outside coverage area
// - Invalid input parameters
func ExampleErrorHandling() {
	// Case 1: Insufficient nodes (only 1 node)
	fmt.Println("=== Case 1: Insufficient Nodes ===")
	singleNode := simulator.NewNode("node1", "Lone Node", simulator.NodeTypeReal,
		simulator.Point{X: 5.0, Y: 5.0, Z: 2.0})

	// Create computer with no links (insufficient for any GDOP computation)
	computer := simulator.NewGDOPComputer([]simulator.Link{}, simulator.GridConfig{
		MinX: 0.0, MinY: 0.0, Width: 10.0, Depth: 10.0, CellSize: 0.2,
	})

	result := computer.ComputeAt(5.0, 5.0, 1.0)
	fmt.Printf("  Point (5.0, 5.0) with no links:\n")
	fmt.Printf("    GDOP: %v\n", result.GDOP)
	fmt.Printf("    Quality: %s\n", result.Quality)
	// Expected: GDOP = Inf, Quality = "none"

	// Case 2: Collinear node arrangement
	fmt.Println("\n=== Case 2: Collinear Nodes ===")
	collinearNodes := []*simulator.Node{
		simulator.NewNode("node1", "West", simulator.NodeTypeReal,
			simulator.Point{X: 0.0, Y: 5.0, Z: 2.0}),
		simulator.NewNode("node2", "Center", simulator.NodeTypeReal,
			simulator.Point{X: 5.0, Y: 5.0, Z: 2.0}),
		simulator.NewNode("node3", "East", simulator.NodeTypeReal,
			simulator.Point{X: 10.0, Y: 5.0, Z: 2.0}),
	}

	collinearLinks := []simulator.Link{
		{TX: collinearNodes[0], RX: collinearNodes[1]},
		{TX: collinearNodes[1], RX: collinearNodes[2]},
		{TX: collinearNodes[0], RX: collinearNodes[2]},
	}

	collinearComputer := simulator.NewGDOPComputer(collinearLinks, simulator.GridConfig{
		MinX: 0.0, MinY: 0.0, Width: 10.0, Depth: 10.0, CellSize: 0.2,
	})

	collinearResult := collinearComputer.ComputeAt(5.0, 5.0, 1.0)
	fmt.Printf("  Point (5.0, 5.0) with collinear nodes:\n")
	fmt.Printf("    GDOP: %v\n", collinearResult.GDOP)
	fmt.Printf("    Quality: %s\n", collinearResult.Quality)
	// Expected: GDOP = Inf (degenerate geometry from collinear links)

	// Case 3: Point far from any node (outside Fresnel zones)
	fmt.Println("\n=== Case 3: Point Outside Coverage ===")
	wellPlacedNodes := []*simulator.Node{
		simulator.NewNode("node1", "NW Corner", simulator.NodeTypeReal,
			simulator.Point{X: 0.0, Y: 0.0, Z: 2.0}),
		simulator.NewNode("node2", "NE Corner", simulator.NodeTypeReal,
			simulator.Point{X: 10.0, Y: 0.0, Z: 2.0}),
	}

	wellPlacedLinks := []simulator.Link{
		{TX: wellPlacedNodes[0], RX: wellPlacedNodes[1]},
	}

	wellPlacedComputer := simulator.NewGDOPComputer(wellPlacedLinks, simulator.GridConfig{
		MinX: 0.0, MinY: 0.0, Width: 20.0, Depth: 20.0, CellSize: 0.2,
	})

	// Point far outside the room defined by nodes
	farResult := wellPlacedComputer.ComputeAt(15.0, 15.0, 1.0)
	fmt.Printf("  Point (15.0, 15.0) far from nodes:\n")
	fmt.Printf("    GDOP: %v\n", farResult.GDOP)
	fmt.Printf("    Quality: %s\n", farResult.Quality)
	fmt.Printf("    Contributing links: %v\n", farResult.ContributingLinks)
	// Expected: GDOP = Inf, Quality = "none" (no Fresnel zone coverage)
}

// Example 4: Performance Optimization
//
// Demonstrates strategies for optimizing GDOP computation performance:
// - Grid cell size selection
// - Fresnel zone limiting
// - Incremental computation for real-time updates
func ExamplePerformanceOptimization() {
	nodes := []*simulator.Node{
		simulator.NewNode("node1", "NW", simulator.NodeTypeReal,
			simulator.Point{X: 0.0, Y: 0.0, Z: 2.0}),
		simulator.NewNode("node2", "NE", simulator.NodeTypeReal,
			simulator.Point{X: 10.0, Y: 0.0, Z: 2.0}),
		simulator.NewNode("node3", "SW", simulator.NodeTypeReal,
			simulator.Point{X: 0.0, Y: 10.0, Z: 2.0}),
		simulator.NewNode("node4", "SE", simulator.NodeTypeReal,
			simulator.Point{X: 10.0, Y: 10.0, Z: 2.0}),
	}

	links := []simulator.Link{
		{TX: nodes[0], RX: nodes[1]},
		{TX: nodes[0], RX: nodes[2]},
		{TX: nodes[1], RX: nodes[3]},
		{TX: nodes[2], RX: nodes[3]},
	}

	// Strategy 1: Coarse grid for fast preview (1m cells instead of 0.2m)
	fmt.Println("=== Strategy 1: Grid Cell Size Impact ===")
	cellSizes := []float64{0.1, 0.2, 0.5, 1.0}

	for _, cellSize := range cellSizes {
		computer := simulator.NewGDOPComputer(links, simulator.GridConfig{
			MinX: 0.0, MinY: 0.0, Width: 10.0, Depth: 10.0, CellSize: cellSize,
		})

		start := time.Now()
		results := computer.ComputeAll()
		elapsed := time.Since(start)

		rows, cols := len(results), len(results[0])
		fmt.Printf("  CellSize: %.1fm, Grid: %dx%d, Time: %v\n",
			cellSize, cols, rows, elapsed)
	}
	// Expected: Finer grids (0.1m) take significantly longer than coarse grids (1.0m)

	// Strategy 2: Limit Fresnel zones for faster computation
	fmt.Println("\n=== Strategy 2: Fresnel Zone Limiting ===")

	// Default: consider first 3 Fresnel zones
	defaultComputer := simulator.NewGDOPComputer(links, simulator.GridConfig{
		MinX: 0.0, MinY: 0.0, Width: 10.0, Depth: 10.0, CellSize: 0.2,
	})
	// maxZone = 3 by default

	start := time.Now()
	defaultResults := defaultComputer.ComputeAll()
	defaultTime := time.Since(start)

	// Conservative: consider only first Fresnel zone
	conservativeComputer := simulator.NewGDOPComputer(links, simulator.GridConfig{
		MinX: 0.0, MinY: 0.0, Width: 10.0, Depth: 10.0, CellSize: 0.2,
	})
	conservativeComputer.SetMaxZone(1) // Only zone 1

	start = time.Now()
	conservativeResults := conservativeComputer.ComputeAll()
	conservativeTime := time.Since(start)

	fmt.Printf("  Default (3 zones): %v\n", defaultTime)
	fmt.Printf("  Conservative (1 zone): %v\n", conservativeTime)
	// Expected: Conservative is slightly faster (fewer links qualify)

	// Strategy 3: Incremental computation (recompute only changed cells)
	fmt.Println("\n=== Strategy 3: Incremental Updates ===")

	// Initial computation
	originalComputer := simulator.NewGDOPComputer(links, simulator.GridConfig{
		MinX: 0.0, MinY: 0.0, Width: 10.0, Depth: 10.0, CellSize: 0.2,
	})

	// Simulate node repositioning: compute only affected region
	// Instead of recomputing entire grid, focus on area around moved node
	singlePointStart := time.Now()
	for i := 0; i < 100; i++ {
		x := float64(i) * 0.1
		y := float64(i) * 0.1
		originalComputer.ComputeAt(x, y, 1.0)
	}
	singlePointTime := time.Since(singlePointStart)

	fmt.Printf("  100 single-point computations: %v\n", singlePointTime)
	fmt.Printf("  Average per point: %v\n", singlePointTime/100)
	// Expected: Single-point queries are much faster than full grid recomputation
}

// Example 5: Common Pitfalls and Best Practices
//
// Demonstrates common mistakes and their corrections:
// - Pitfall 1: Forgetting to set node positions
// - Pitfall 2: Using wrong coordinate system
// - Pitfall 3: Ignoring Z-axis in multi-floor environments
// - Best Practice 1: Node placement optimization
// - Best Practice 2: Coverage validation before deployment
func ExampleCommonPitfalls() {
	fmt.Println("=== Pitfall 1: Forgetting Node Positions ===")

	// WRONG: Nodes with zero positions
	badNodes := []*simulator.Node{
		simulator.NewNode("node1", "Unpositioned", simulator.NodeTypeReal,
			simulator.Point{X: 0.0, Y: 0.0, Z: 0.0}), // Z=0 means on floor
		simulator.NewNode("node2", "Also Unpositioned", simulator.NodeTypeReal,
			simulator.Point{X: 0.0, Y: 0.0, Z: 0.0}),
	}

	// RIGHT: Nodes placed at proper height
	goodNodes := []*simulator.Node{
		simulator.NewNode("node1", "Ceiling NW", simulator.NodeTypeReal,
			simulator.Point{X: 0.0, Y: 0.0, Z: 2.5}), // 2.5m height
		simulator.NewNode("node2", "Ceiling NE", simulator.NodeTypeReal,
			simulator.Point{X: 10.0, Y: 0.0, Z: 2.5}),
	}

	fmt.Println("  ✅ Place nodes at appropriate heights (1.5m-3m)")

	fmt.Println("\n=== Pitfall 2: Wrong Coordinate System ===")

	// WRONG: Mixing coordinate systems or units
	// Assuming meters when using feet, or vice versa
	badLinks := []simulator.Link{
		{TX: goodNodes[0], RX: goodNodes[1]},
	}
	badComputer := simulator.NewGDOPComputer(badLinks, simulator.GridConfig{
		MinX: 0.0, MinY: 0.0, Width: 30.0, Depth: 30.0, CellSize: 0.2, // Actually feet!
	})

	// RIGHT: Always use meters in Spaxel
	goodComputer := simulator.NewGDOPComputer(badLinks, simulator.GridConfig{
		MinX: 0.0, MinY: 0.0, Width: 9.14, Depth: 9.14, CellSize: 0.2, // 30ft = 9.14m
	})

	fmt.Println("  ✅ Always use meters for all coordinates")

	fmt.Println("\n=== Pitfall 3: Ignoring Z-Axis ===")

	// WRONG: Assuming 2D analysis works for multi-story buildings
	twoStoryNodes := []*simulator.Node{
		simulator.NewNode("node1", "Floor1", simulator.NodeTypeReal,
			simulator.Point{X: 5.0, Y: 5.0, Z: 2.0}), // First floor
		simulator.NewNode("node2", "Floor2", simulator.NodeTypeReal,
			simulator.Point{X: 5.0, Y: 5.0, Z: 5.0}), // Second floor
	}

	twoStoryLinks := []simulator.Link{
		{TX: twoStoryNodes[0], RX: twoStoryNodes[1]},
	}

	twoStoryComputer := simulator.NewGDOPComputer(twoStoryLinks, simulator.GridConfig{
		MinX: 0.0, MinY: 0.0, Width: 10.0, Depth: 10.0, CellSize: 0.2,
	})

	// Check coverage at different heights
	floor1Result := twoStoryComputer.ComputeAt(5.0, 5.0, 1.0) // First floor
	floor2Result := twoStoryComputer.ComputeAt(5.0, 5.0, 4.0) // Second floor

	fmt.Printf("  First floor (Z=1.0m): GDOP=%.2f, Quality=%s\n",
		floor1Result.GDOP, floor1Result.Quality)
	fmt.Printf("  Second floor (Z=4.0m): GDOP=%.2f, Quality=%s\n",
		floor2Result.GDOP, floor2Result.Quality)
	fmt.Println("  ✅ For multi-story buildings, evaluate coverage at each floor level")

	fmt.Println("\n=== Best Practice 1: Node Placement Optimization ===")

	// Start with poor placement
	initialNodes := []*simulator.Node{
		simulator.NewNode("node1", "Clustered1", simulator.NodeTypeReal,
			simulator.Point{X: 0.0, Y: 0.0, Z: 2.0}),
		simulator.NewNode("node2", "Clustered2", simulator.NodeTypeReal,
			simulator.Point{X: 1.0, Y: 0.0, Z: 2.0}), // Only 1m apart!
		simulator.NewNode("node3", "Clustered3", simulator.NodeTypeReal,
			simulator.Point{X: 2.0, Y: 0.0, Z: 2.0}), // Only 1m apart!
	}

	initialLinks := []simulator.Link{
		{TX: initialNodes[0], RX: initialNodes[1]},
		{TX: initialNodes[0], RX: initialNodes[2]},
		{TX: initialNodes[1], RX: initialNodes[2]},
	}

	initialComputer := simulator.NewGDOPComputer(initialLinks, simulator.GridConfig{
		MinX: 0.0, MinY: 0.0, Width: 10.0, Depth: 10.0, CellSize: 0.5,
	})
	initialResults := initialComputer.ComputeAll()

	// Calculate initial coverage score
	var initialGoodCells int
	for _, row := range initialResults {
		for _, result := range row {
			if result.Quality == "excellent" || result.Quality == "good" {
				initialGoodCells++
			}
		}
	}
	totalCells := len(initialResults) * len(initialResults[0])
	initialScore := float64(initialGoodCells) / float64(totalCells) * 100

	// Optimize: spread nodes apart
	optimizedNodes := []*simulator.Node{
		simulator.NewNode("node1", "Spread1", simulator.NodeTypeReal,
			simulator.Point{X: 0.0, Y: 0.0, Z: 2.0}),
		simulator.NewNode("node2", "Spread2", simulator.NodeTypeReal,
			simulator.Point{X: 10.0, Y: 0.0, Z: 2.0}),
		simulator.NewNode("node3", "Spread3", simulator.NodeTypeReal,
			simulator.Point{X: 0.0, Y: 10.0, Z: 2.0}),
	}

	optimizedLinks := []simulator.Link{
		{TX: optimizedNodes[0], RX: optimizedNodes[1]},
		{TX: optimizedNodes[0], RX: optimizedNodes[2]},
		{TX: optimizedNodes[1], RX: optimizedNodes[2]},
	}

	optimizedComputer := simulator.NewGDOPComputer(optimizedLinks, simulator.GridConfig{
		MinX: 0.0, MinY: 0.0, Width: 10.0, Depth: 10.0, CellSize: 0.5,
	})
	optimizedResults := optimizedComputer.ComputeAll()

	var optimizedGoodCells int
	for _, row := range optimizedResults {
		for _, result := range row {
			if result.Quality == "excellent" || result.Quality == "good" {
				optimizedGoodCells++
			}
		}
	}
	optimizedScore := float64(optimizedGoodCells) / float64(totalCells) * 100

	fmt.Printf("  Initial placement: %.1f%% good coverage\n", initialScore)
	fmt.Printf("  Optimized placement: %.1f%% good coverage\n", optimizedScore)
	fmt.Printf("  Improvement: +%.1f%%\n", optimizedScore-initialScore)
	fmt.Println("  ✅ Spread nodes apart to maximize angular diversity")

	fmt.Println("\n=== Best Practice 2: Pre-Deployment Validation ===")

	// Before physical deployment, simulate coverage
	deploymentNodes := []*simulator.Node{
		simulator.NewNode("node1", "Proposed1", simulator.NodeTypeVirtual,
			simulator.Point{X: 0.0, Y: 0.0, Z: 2.0}),
		simulator.NewNode("node2", "Proposed2", simulator.NodeTypeVirtual,
			simulator.Point{X: 10.0, Y: 0.0, Z: 2.0}),
		simulator.NewNode("node3", "Proposed3", simulator.NodeTypeVirtual,
			simulator.Point{X: 5.0, Y: 10.0, Z: 2.0}),
		simulator.NewNode("node4", "Proposed4", simulator.NodeTypeVirtual,
			simulator.Point{X: 5.0, Y: 5.0, Z: 2.0}), // Central node
	}

	deploymentLinks := []simulator.Link{
		{TX: deploymentNodes[0], RX: deploymentNodes[1]},
		{TX: deploymentNodes[0], RX: deploymentNodes[2]},
		{TX: deploymentNodes[0], RX: deploymentNodes[3]},
		{TX: deploymentNodes[1], RX: deploymentNodes[2]},
		{TX: deploymentNodes[1], RX: deploymentNodes[3]},
		{TX: deploymentNodes[2], RX: deploymentNodes[3]},
	}

	deploymentComputer := simulator.NewGDOPComputer(deploymentLinks, simulator.GridConfig{
		MinX: 0.0, MinY: 0.0, Width: 10.0, Depth: 10.0, CellSize: 0.2,
	})
	deploymentResults := deploymentComputer.ComputeAll()

	// Find worst coverage point
	worstGDOP := 0.0
	var worstPoint simulator.Point
	for _, row := range deploymentResults {
		for _, result := range row {
			if result.GDOP > worstGDOP && !math.IsInf(result.GDOP, 1) {
				worstGDOP = result.GDOP
				worstPoint = simulator.Point{X: result.X, Y: result.Y, Z: result.Z}
			}
		}
	}

	fmt.Printf("  Pre-deployment validation:\n")
	fmt.Printf("    Worst GDOP: %.2f at (%.1f, %.1f)\n", worstGDOP, worstPoint.X, worstPoint.Y)
	fmt.Printf("    Recommendation: %s\n",
		map[bool]string{true: "✅ Proceed with deployment", false: "❌ Add more nodes or reposition"}[worstGDOP < 4.0])
	fmt.Println("  ✅ Validate coverage virtually before physical installation")
}

// Example 6: Integration with Spaxel Positioning System
//
// Demonstrates how GDOP computation integrates with the broader Spaxel
// system for real-time coverage painting and node optimization.
func ExampleSpaxelIntegration() {
	fmt.Println("=== Real-Time Coverage Painting ===")

	// In a real Spaxel deployment, this would be called from:
	// - Dashboard 3D editor during node drag operations
	// - Fleet manager when evaluating repositioning suggestions
	// - Self-healing fleet when a node goes offline

	// Simulate current fleet state
	currentNodes := []*simulator.Node{
		simulator.NewNode("AA:BB:CC:DD:EE:F1", "Kitchen NW", simulator.NodeTypeReal,
			simulator.Point{X: 0.0, Y: 0.0, Z: 2.0}),
		simulator.NewNode("AA:BB:CC:DD:EE:F2", "Kitchen NE", simulator.NodeTypeReal,
			simulator.Point{X: 10.0, Y: 0.0, Z: 2.0}),
		simulator.NewNode("AA:BB:CC:DD:EE:F3", "Kitchen SW", simulator.NodeTypeReal,
			simulator.Point{X: 0.0, Y: 10.0, Z: 2.0}),
	}

	currentLinks := []simulator.Link{
		{TX: currentNodes[0], RX: currentNodes[1]},
		{TX: currentNodes[0], RX: currentNodes[2]},
		{TX: currentNodes[1], RX: currentNodes[2]},
	}

	// Create GDOP computer for coverage painting
	// Use 0.2m cells for high-resolution coverage overlay
	coverageComputer := simulator.NewGDOPComputer(currentLinks, simulator.GridConfig{
		MinX:     0.0,
		MinY:     0.0,
		Width:    10.0,
		Depth:    10.0,
		CellSize: 0.2, // High resolution for live visualization
	})

	// Simulate user dragging node2 to new position
	// In real system, this would be called on every animation frame
	fmt.Println("  Simulating node drag...")

	// Initial position
	initialPos := simulator.Point{X: 10.0, Y: 0.0, Z: 2.0}
	initialResult := coverageComputer.ComputeAt(5.0, 5.0, 1.0)
	fmt.Printf("    Node at (%.1f, %.1f): Center GDOP = %.2f (%s)\n",
		initialPos.X, initialPos.Y, initialResult.GDOP, initialResult.Quality)

	// New position after drag
	newPos := simulator.Point{X: 10.0, Y: 10.0, Z: 2.0}

	// Update link with new position (in real system, this happens automatically)
	draggedNodes := []*simulator.Node{
		simulator.NewNode("AA:BB:CC:DD:EE:F1", "Kitchen NW", simulator.NodeTypeReal,
			simulator.Point{X: 0.0, Y: 0.0, Z: 2.0}),
		simulator.NewNode("AA:BB:CC:DD:EE:F2", "Kitchen SE (dragged)", simulator.NodeTypeReal,
			newPos), // New position
		simulator.NewNode("AA:BB:CC:DD:EE:F3", "Kitchen SW", simulator.NodeTypeReal,
			simulator.Point{X: 0.0, Y: 10.0, Z: 2.0}),
	}

	draggedLinks := []simulator.Link{
		{TX: draggedNodes[0], RX: draggedNodes[1]},
		{TX: draggedNodes[0], RX: draggedNodes[2]},
		{TX: draggedNodes[1], RX: draggedNodes[2]},
	}

	draggedComputer := simulator.NewGDOPComputer(draggedLinks, simulator.GridConfig{
		MinX: 0.0, MinY: 0.0, Width: 10.0, Depth: 10.0, CellSize: 0.2,
	})

	draggedResult := draggedComputer.ComputeAt(5.0, 5.0, 1.0)
	fmt.Printf("    Node at (%.1f, %.1f): Center GDOP = %.2f (%s)\n",
		newPos.X, newPos.Y, draggedResult.GDOP, draggedResult.Quality)

	// Calculate improvement
	improvement := initialResult.GDOP - draggedResult.GDOP
	if improvement > 0 {
		fmt.Printf("    ✅ Improvement: %.2f GDOP points\n", improvement)
	} else if improvement < 0 {
		fmt.Printf("    ⚠️  Degradation: %.2f GDOP points\n", -improvement)
	} else {
		fmt.Printf("    ➡️  No change\n")
	}

	fmt.Println("\n=== Fleet Self-Healing ===")

	// Simulate node going offline
	fmt.Println("  Simulating node failure...")

	// Remaining nodes after node3 goes offline
	reducedNodes := []*simulator.Node{
		simulator.NewNode("AA:BB:CC:DD:EE:F1", "Kitchen NW", simulator.NodeTypeReal,
			simulator.Point{X: 0.0, Y: 0.0, Z: 2.0}),
		simulator.NewNode("AA:BB:CC:DD:EE:F2", "Kitchen SE", simulator.NodeTypeReal,
			simulator.Point{X: 10.0, Y: 10.0, Z: 2.0}),
	}

	reducedLinks := []simulator.Link{
		{TX: reducedNodes[0], RX: reducedNodes[1]},
	}

	// Only 1 link remaining = insufficient for localization
	reducedComputer := simulator.NewGDOPComputer(reducedLinks, simulator.GridConfig{
		MinX: 0.0, MinY: 0.0, Width: 10.0, Depth: 10.0, CellSize: 0.2,
	})

	reducedResult := reducedComputer.ComputeAt(5.0, 5.0, 1.0)
	fmt.Printf("    After node loss: GDOP = %v, Quality = %s\n",
		reducedResult.GDOP, reducedResult.Quality)

	// In real system, self-healing would:
	// 1. Detect coverage degradation
	// 2. Re-optimize roles among remaining nodes
	// 3. Alert operator: "Significant coverage gaps in kitchen corridor"
	// 4. Suggest: "Add relay node at (5, 5) to restore coverage"

	fmt.Println("  ✅ Fleet self-healing uses GDOP to maintain coverage")
}

// Example 7: Advanced Usage - Custom Fresnel Zone Configuration
//
// Demonstrates how to configure Fresnel zone limits for different scenarios:
// - Conservative: Only zone 1 (highest sensitivity)
// - Balanced: Zones 1-3 (default)
// - Aggressive: Zones 1-5 (maximum coverage)
func ExampleAdvancedConfiguration() {
	fmt.Println("=== Fresnel Zone Configuration ===")

	nodes := []*simulator.Node{
		simulator.NewNode("node1", "Corner1", simulator.NodeTypeReal,
			simulator.Point{X: 0.0, Y: 0.0, Z: 2.0}),
		simulator.NewNode("node2", "Corner2", simulator.NodeTypeReal,
			simulator.Point{X: 10.0, Y: 0.0, Z: 2.0}),
	}

	links := []simulator.Link{
		{TX: nodes[0], RX: nodes[1]},
	}

	config := simulator.GridConfig{
		MinX: 0.0, MinY: 0.0, Width: 10.0, Depth: 10.0, CellSize: 0.5,
	}

	// Test different Fresnel zone limits
	zones := []int{1, 2, 3, 4, 5}
	testPoint := simulator.Point{X: 5.0, Y: 5.0, Z: 1.0}

	fmt.Println("  Coverage at center (5.0, 5.0) with different zone limits:")

	for _, maxZone := range zones {
		computer := simulator.NewGDOPComputer(links, config)
		computer.SetMaxZone(maxZone)

		result := computer.ComputeAt(testPoint.X, testPoint.Y, testPoint.Z)

		coverageStatus := "❌ No coverage"
		if len(result.ContributingLinks) > 0 {
			coverageStatus = "✅ Covered"
		}

		fmt.Printf("    MaxZone %d: GDOP=%.2f, Quality=%s, Links=%d %s\n",
			maxZone, result.GDOP, result.Quality,
			len(result.ContributingLinks), coverageStatus)
	}

	fmt.Println("\n  Fresnel zone guidance:")
	fmt.Println("    • Zone 1 only: Most sensitive, least coverage")
	fmt.Println("    • Zones 1-3: Balanced (default, recommended)")
	fmt.Println("    • Zones 1-5: Maximum coverage, may include noisy links")
	fmt.Println("  ✅ Use default (3 zones) unless you have specific requirements")
}

// Helper function to demonstrate usage in main
func ExampleMain() {
	fmt.Println("==========================================")
	fmt.Println("   GDOP Usage Examples")
	fmt.Println("   Spaxel Indoor Positioning System")
	fmt.Println("==========================================\n")

	ExampleBasicPointComputation()
	fmt.Println("\n" + strings.Repeat("-", 50) + "\n")

	ExampleGridBasedCoverageMapping()
	fmt.Println("\n" + strings.Repeat("-", 50) + "\n")

	ExampleErrorHandling()
	fmt.Println("\n" + strings.Repeat("-", 50) + "\n")

	ExamplePerformanceOptimization()
	fmt.Println("\n" + strings.Repeat("-", 50) + "\n")

	ExampleCommonPitfalls()
	fmt.Println("\n" + strings.Repeat("-", 50) + "\n")

	ExampleSpaxelIntegration()
	fmt.Println("\n" + strings.Repeat("-", 50) + "\n")

	ExampleAdvancedConfiguration()
}

// Add this to imports
import "strings"
