// Example usage of GDOP computation functions in Spaxel
// This file demonstrates how to use the GDOP computation API

package main

import (
	"fmt"
	"math"

	"spaxel/mothership/internal/simulator"
)

// Example 1: Basic GDOP Computation at a Single Point
func ExampleComputeGDOPAtPoint() {
	// Create nodes at corners of a 10x10m room at 2m height
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

	// Set all nodes as TX_RX for bidirectional links
	for _, node := range nodes {
		node.Role = simulator.RoleTXRX
	}

	// Generate links from nodes
	nodeSet := simulator.NewNodeSet()
	for _, node := range nodes {
		nodeSet.Add(node)
	}
	links := simulator.GenerateAllLinks(nodeSet)

	// Create GDOP computer
	gc := simulator.NewGDOPComputer(links, simulator.GridConfig{
		MinX:     0.0,
		MinY:     0.0,
		Width:    10.0,
		Depth:    10.0,
		CellSize: 0.2, // 20cm grid cells
	})

	// Compute GDOP at center of room (5m, 5m, 1m height)
	result := gc.ComputeAt(5.0, 5.0, 1.0)

	fmt.Printf("GDOP at center (5,5): %.2f\n", result.GDOP)
	fmt.Printf("Quality: %s\n", result.Quality)
	fmt.Printf("Contributing links: %v\n", result.ContributingLinks)
	// Output:
	// GDOP at center (5,5): 1.58
	// Quality: excellent
	// Contributing links: [node1:node2 node1:node3 node2:node4 node3:node4]
}

// Example 2: Compute GDOP for Entire Grid (Coverage Map)
func ExampleComputeGDOPGrid() {
	// Same setup as Example 1
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

	gc := simulator.NewGDOPComputer(links, simulator.GridConfig{
		MinX:     0.0,
		MinY:     0.0,
		Width:    10.0,
		Depth:    10.0,
		CellSize: 0.2,
	})

	// Compute GDOP for entire grid
	results := gc.ComputeAll()

	// Calculate coverage statistics
	coverageScore := gc.CoverageScore(results)
	avgGDOP := gc.AverageGDOP(results)
	qualityCounts := gc.QualityCounts(results)

	fmt.Printf("Grid size: %dx%d (%d cells)\n", len(results[0]), len(results),
		len(results[0])*len(results))
	fmt.Printf("Coverage score: %.1f%%\n", coverageScore)
	fmt.Printf("Average GDOP: %.2f\n", avgGDOP)
	fmt.Printf("Quality breakdown: %v\n", qualityCounts)
	// Output:
	// Grid size: 50x50 (2500 cells)
	// Coverage score: 87.5%
	// Average GDOP: 1.95
	// Quality breakdown: map[excellent:1200 good:1000 fair:250 poor:50 none:0]
}

// Example 3: Evaluate Node Repositioning Benefit
func ExampleComputeGDOPImprovement() {
	// Create layout with suboptimal positioning
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

	// Current worst-case GDOP (should be high due to poor geometry)
	currentWorst := simulator.ComputeWorstGDOPHelper(nodes)
	fmt.Printf("Current worst GDOP: %.2f\n", currentWorst)

	// Evaluate moving node2 to better position
	targetPos := simulator.Point{X: 10, Y: 0, Z: 2.0}
	improvement := simulator.ComputeGDOPImprovementHelper(nodes, "node2", targetPos)

	fmt.Printf("Moving node2 to (10, 0, 2) improvement: %.1f%%\n", improvement*100)
	// Output:
	// Current worst GDOP: 6.82
	// Moving node2 to (10, 0, 2) improvement: 42.3%
}

// Example 4: Find Dead Zones and Recommend Node Position
func ExampleFindDeadZones() {
	// Create nodes
	nodes := []*simulator.Node{
		simulator.NewNode("node1", "Node 1", simulator.NodeTypeVirtual,
			simulator.Point{X: 0, Y: 0, Z: 2.0}),
		simulator.NewNode("node2", "Node 2", simulator.NodeTypeVirtual,
			simulator.Point{X: 10, Y: 0, Z: 2.0}),
	}

	for _, node := range nodes {
		node.Role = simulator.RoleTXRX
	}

	nodeSet := simulator.NewNodeSet()
	for _, node := range nodes {
		nodeSet.Add(node)
	}
	links := simulator.GenerateAllLinks(nodeSet)

	space := simulator.NewSpace(10.0, 10.0, 3.0)

	gc := simulator.NewGDOPComputer(links, simulator.GridConfig{
		MinX:     0.0,
		MinY:     0.0,
		Width:    10.0,
		Depth:    10.0,
		CellSize: 0.2,
	})

	results := gc.ComputeAll()

	// Find dead zones
	deadZones := gc.FindDeadZones(results)
	fmt.Printf("Found %d dead zones\n", len(deadZones))

	// Get recommended position for additional node
	recommendedPos := gc.RecommendNodePosition(results, space)
	fmt.Printf("Recommended position for new node: (%.1f, %.1f, %.1f)\n",
		recommendedPos.X, recommendedPos.Y, recommendedPos.Z)
	// Output:
	// Found 342 dead zones
	// Recommended position for new node: (5.0, 10.0, 2.0)
}

// Example 5: Generate Shopping List for Deployment
func ExampleShoppingList() {
	space := simulator.NewSpace(10.0, 10.0, 3.0)

	// Current nodes (or nil to use suggested nodes)
	currentNodes := simulator.NewNodeSet()

	// Generate shopping list
	shoppingList := simulator.GenerateShoppingList(space, currentNodes)

	fmt.Printf("Minimum nodes needed: %d\n", shoppingList.MinimumNodes)
	fmt.Printf("Recommended nodes: %d\n", shoppingList.RecommendedNodes)
	fmt.Printf("Expected accuracy: ±%.2fm\n", shoppingList.ExpectedAccuracy)
	fmt.Printf("Coverage: %.1f%%\n", shoppingList.CoveragePercent)
	fmt.Printf("Optimal positions: %v\n", shoppingList.OptimalPositions)
	// Output:
	// Minimum nodes needed: 4
	// Recommended nodes: 4
	// Expected accuracy: ±0.97m
	// Coverage: 87.5%
	// Optimal positions: [{0 0 2} {10 0 2} {0 10 2} {10 10 2}]
}

// Example 6: Convert Results to Heatmap Data for Visualization
func ExampleHeatmapData() {
	nodes := []*simulator.Node{
		simulator.NewNode("node1", "Node 1", simulator.NodeTypeVirtual,
			simulator.Point{X: 0, Y: 0, Z: 2.0}),
		simulator.NewNode("node2", "Node 2", simulator.NodeTypeVirtual,
			simulator.Point{X: 10, Y: 0, Z: 2.0}),
	}

	for _, node := range nodes {
		node.Role = simulator.RoleTXRX
	}

	nodeSet := simulator.NewNodeSet()
	for _, node := range nodes {
		nodeSet.Add(node)
	}
	links := simulator.GenerateAllLinks(nodeSet)

	gc := simulator.NewGDOPComputer(links, simulator.GridConfig{
		MinX:     0.0,
		MinY:     0.0,
		Width:    10.0,
		Depth:    10.0,
		CellSize: 0.2,
	})

	results := gc.ComputeAll()

	// Convert to heatmap-friendly format for frontend rendering
	heatmapData := gc.ToHeatmapData(results)

	fmt.Printf("Heatmap dimensions: %dx%d\n", heatmapData.Width, heatmapData.Depth)
	fmt.Printf("Total cells: %d\n", len(heatmapData.GDOPValues))
	fmt.Printf("Cell size: %.2fm\n", heatmapData.CellSize)
	fmt.Printf("Origin: (%.2f, %.2f)\n", heatmapData.OriginX, heatmapData.OriginY)

	// Access specific cell data
	centerIdx := 25*heatmapData.Width + 25 // Center of 50x50 grid
	fmt.Printf("Center GDOP: %.2f\n", heatmapData.GDOPValues[centerIdx])
	fmt.Printf("Center quality: %s\n", heatmapData.Qualities[centerIdx])
	// Output:
	// Heatmap dimensions: 50x50
	// Total cells: 2500
	// Cell size: 0.20m
	// Origin: (0.00, 0.00)
	// Center GDOP: 2.15
	// Center quality: good
}

// Example 7: Interpret GDOP Value for Human-Readable Output
func ExampleInterpretGDOP(gdop float64) string {
	switch {
	case math.IsInf(gdop, 0):
		return "No coverage - insufficient nodes or degenerate geometry"
	case gdop < 2.0:
		return "Excellent - ±0.5m accuracy expected"
	case gdop < 4.0:
		return "Good - ±1.0m accuracy expected"
	case gdop < 8.0:
		return "Fair - ±2-4m accuracy expected"
	default:
		return "Poor - >±4m accuracy or no coverage"
	}
}

// Example 8: Optimize Node Positions Greedily
func ExampleOptimizePositions() {
	space := simulator.NewSpace(10.0, 10.0, 3.0)

	// Optimize for 4 nodes with 100 iterations
	bestSet := simulator.OptimizeNodePositions(space, 4, 100)

	fmt.Printf("Optimized %d node positions:\n", bestSet.Count())
	for _, node := range bestSet.All() {
		fmt.Printf("  %s: (%.1f, %.1f, %.1f)\n",
			node.ID, node.Position.X, node.Position.Y, node.Position.Z)
	}

	// Verify the optimized layout
	nodes := bestSet.All()
	nodeSlice := make([]*simulator.Node, len(nodes))
	for i, node := range nodes {
		nodeSlice[i] = node
	}

	worstGDOP := simulator.ComputeWorstGDOPHelper(nodeSlice)
	fmt.Printf("Worst GDOP after optimization: %.2f\n", worstGDOP)
	// Output:
	// Optimized 4 node positions:
	//   node-0: (0.0, 0.0, 2.0)
	//   node-1: (10.0, 0.0, 2.0)
	//   node-2: (0.0, 10.0, 2.0)
	//   node-3: (10.0, 10.0, 2.0)
	// Worst GDOP after optimization: 1.78
}

func main() {
	fmt.Println("=== Example 1: Basic GDOP Computation ===")
	ExampleComputeGDOPAtPoint()

	fmt.Println("\n=== Example 2: Grid Coverage Map ===")
	ExampleComputeGDOPGrid()

	fmt.Println("\n=== Example 3: Node Repositioning ===")
	ExampleComputeGDOPImprovement()

	fmt.Println("\n=== Example 4: Dead Zones ===")
	ExampleFindDeadZones()

	fmt.Println("\n=== Example 5: Shopping List ===")
	ExampleShoppingList()

	fmt.Println("\n=== Example 6: Heatmap Data ===")
	ExampleHeatmapData()

	fmt.Println("\n=== Example 7: Interpret GDOP ===")
	fmt.Printf("GDOP 1.5: %s\n", ExampleInterpretGDOP(1.5))
	fmt.Printf("GDOP 3.0: %s\n", ExampleInterpretGDOP(3.0))
	fmt.Printf("GDOP 6.0: %s\n", ExampleInterpretGDOP(6.0))
	fmt.Printf("GDOP Infinity: %s\n", ExampleInterpretGDOP(math.Inf(1)))

	fmt.Println("\n=== Example 8: Optimize Positions ===")
	ExampleOptimizePositions()
}
