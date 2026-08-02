package simulator

import (
	"math"
	"testing"
)

// TestComputeGDOPImprovement tests the GDOP improvement computation function
func TestComputeGDOPImprovement(t *testing.T) {
	// Create a simple 4-node layout in corners of a 10x10m room
	nodes := []*Node{
		NewNode("node1", "Node 1", NodeTypeVirtual, Point{X: 0, Y: 0, Z: 2.0}),
		NewNode("node2", "Node 2", NodeTypeVirtual, Point{X: 10, Y: 0, Z: 2.0}),
		NewNode("node3", "Node 3", NodeTypeVirtual, Point{X: 0, Y: 10, Z: 2.0}),
		NewNode("node4", "Node 4", NodeTypeVirtual, Point{X: 10, Y: 10, Z: 2.0}),
	}

	// Set all nodes as TX_RX for bidirectional links
	for _, node := range nodes {
		node.Role = RoleTXRX
	}

	// Test 1: Moving a node should result in non-constant improvement values
	t.Run("DifferentPositionsYieldDifferentImprovements", func(t *testing.T) {
		// Move node1 to center (should improve coverage)
		centerPos := Point{X: 5, Y: 5, Z: 2.0}
		improvement1 := computeGDOPImprovement(nodes, "node1", centerPos)

		// Move node1 to corner far from others (should not improve as much)
		cornerPos := Point{X: 0, Y: 0, Z: 0.5} // Lower height
		improvement2 := computeGDOPImprovement(nodes, "node1", cornerPos)

		// The improvements should be different (not a constant placeholder)
		if improvement1 == improvement2 {
			t.Errorf("Expected different improvements for different positions, got %v and %v",
				improvement1, improvement2)
		}

		// At least one should show some change (not exactly 0.0 for both)
		if improvement1 == 0.0 && improvement2 == 0.0 {
			t.Errorf("Expected at least one position to show non-zero improvement, got %v and %v",
				improvement1, improvement2)
		}
	})

	// Test 2: Moving node to very poor position should yield negative improvement
	t.Run("PoorPositionYieldsNegativeImprovement", func(t *testing.T) {
		// Create a good initial layout
		nodes := []*Node{
			NewNode("node1", "Node 1", NodeTypeVirtual, Point{X: 0, Y: 0, Z: 2.0}),
			NewNode("node2", "Node 2", NodeTypeVirtual, Point{X: 5, Y: 0, Z: 2.0}),
			NewNode("node3", "Node 3", NodeTypeVirtual, Point{X: 2.5, Y: 5, Z: 2.0}),
		}
		for _, node := range nodes {
			node.Role = RoleTXRX
		}

		// Move node3 to same position as node1 (should degrade geometry)
		poorPos := Point{X: 0, Y: 0, Z: 2.0}
		improvement := computeGDOPImprovement(nodes, "node3", poorPos)

		// Should be negative or zero (degradation or no improvement)
		if improvement > 0 {
			t.Errorf("Expected negative or zero improvement for poor position, got %v", improvement)
		}
	})

	// Test 3: Node not in layout should return 0.0
	t.Run("NodeNotFoundReturnsZero", func(t *testing.T) {
		nodes := []*Node{
			NewNode("node1", "Node 1", NodeTypeVirtual, Point{X: 0, Y: 0, Z: 2.0}),
			NewNode("node2", "Node 2", NodeTypeVirtual, Point{X: 10, Y: 0, Z: 2.0}),
		}
		for _, node := range nodes {
			node.Role = RoleTXRX
		}

		improvement := computeGDOPImprovement(nodes, "nonexistent", Point{X: 5, Y: 5, Z: 2.0})
		if improvement != 0.0 {
			t.Errorf("Expected 0.0 for nonexistent node, got %v", improvement)
		}
	})

	// Test 4: Empty or single-node layout should handle gracefully
	t.Run("EmptyLayoutHandlesGracefully", func(t *testing.T) {
		// Empty layout
		nodes := []*Node{}
		improvement := computeGDOPImprovement(nodes, "node1", Point{X: 5, Y: 5, Z: 2.0})
		if improvement != 0.0 { // Should return 0 for no coverage baseline
			t.Errorf("Expected 0.0 for empty layout, got %v", improvement)
		}

		// Single node
		singleNode := []*Node{
			NewNode("node1", "Node 1", NodeTypeVirtual, Point{X: 0, Y: 0, Z: 2.0}),
		}
		improvement = computeGDOPImprovement(singleNode, "node1", Point{X: 5, Y: 5, Z: 2.0})
		if improvement != 0.0 { // Should return 0 for no coverage baseline
			t.Errorf("Expected 0.0 for single-node layout, got %v", improvement)
		}
	})

	// Test 5: Results should be clamped to [-1.0, 1.0]
	t.Run("ResultsClampedToValidRange", func(t *testing.T) {
		nodes := []*Node{
			NewNode("node1", "Node 1", NodeTypeVirtual, Point{X: 0, Y: 0, Z: 2.0}),
			NewNode("node2", "Node 2", NodeTypeVirtual, Point{X: 10, Y: 0, Z: 2.0}),
			NewNode("node3", "Node 3", NodeTypeVirtual, Point{X: 0, Y: 10, Z: 2.0}),
		}
		for _, node := range nodes {
			node.Role = RoleTXRX
		}

		// Test various positions
		positions := []Point{
			{X: 5, Y: 5, Z: 2.0},
			{X: 0, Y: 0, Z: 0.1},
			{X: 100, Y: 100, Z: 10.0}, // Far outside
		}

		for _, pos := range positions {
			improvement := computeGDOPImprovement(nodes, "node1", pos)
			if improvement < -1.0 || improvement > 1.0 {
				t.Errorf("Improvement not clamped to [-1.0, 1.0], got %v", improvement)
			}
		}
	})

	// Test 6: Using MAC address instead of ID
	t.Run("LookupByMACAddress", func(t *testing.T) {
		nodes := []*Node{
			NewNode("node1", "Node 1", NodeTypeVirtual, Point{X: 0, Y: 0, Z: 2.0}),
			NewNode("node2", "Node 2", NodeTypeVirtual, Point{X: 10, Y: 0, Z: 2.0}),
		}
		for _, node := range nodes {
			node.Role = RoleTXRX
		}

		// Get the actual MAC that will be generated
		targetMAC := nodes[0].GenerateMAC()

		// Should find the node by MAC and compute improvement
		improvement := computeGDOPImprovement(nodes, targetMAC, Point{X: 5, Y: 5, Z: 2.0})
		if improvement < -1.0 || improvement > 1.0 {
			t.Errorf("Expected valid improvement range, got %v", improvement)
		}
	})
}

// TestComputeWorstGDOP tests the helper function for worst-case GDOP computation
func TestComputeWorstGDOP(t *testing.T) {
	t.Run("GoodGeometryYieldsLowGDOP", func(t *testing.T) {
		// Well-positioned nodes should yield reasonable GDOP
		nodes := []*Node{
			NewNode("node1", "Node 1", NodeTypeVirtual, Point{X: 0, Y: 0, Z: 2.0}),
			NewNode("node2", "Node 2", NodeTypeVirtual, Point{X: 10, Y: 0, Z: 2.0}),
			NewNode("node3", "Node 3", NodeTypeVirtual, Point{X: 0, Y: 10, Z: 2.0}),
			NewNode("node4", "Node 4", NodeTypeVirtual, Point{X: 10, Y: 10, Z: 2.0}),
		}
		for _, node := range nodes {
			node.Role = RoleTXRX
		}

		worstGDOP := computeWorstGDOP(nodes)

		// Should have finite GDOP (good coverage)
		if math.IsInf(worstGDOP, 1) {
			t.Errorf("Expected finite GDOP for good geometry, got infinity")
		}

		// GDOP should be reasonable (< 10 for good geometry)
		if worstGDOP > 10.0 {
			t.Errorf("Expected reasonable GDOP < 10, got %v", worstGDOP)
		}
	})

	t.Run("InsufficientNodesReturnsInfinity", func(t *testing.T) {
		// Single node cannot compute GDOP
		nodes := []*Node{
			NewNode("node1", "Node 1", NodeTypeVirtual, Point{X: 0, Y: 0, Z: 2.0}),
		}

		worstGDOP := computeWorstGDOP(nodes)
		if !math.IsInf(worstGDOP, 1) {
			t.Errorf("Expected infinity for single node, got %v", worstGDOP)
		}
	})

	t.Run("CollinearNodesYieldHigherGDOP", func(t *testing.T) {
		// Use a smaller room (6x6m) to ensure cells are within zone 3.
		// Node Z must match ComputeAt's hardcoded 1m evaluation height:
		// coverage requires two links' Fresnel zones (<=3, ~18cm path-length
		// excess) to overlap, which for a non-crossing polygon like a triangle
		// only happens right at shared vertices -- a Z mismatch there adds a
		// path-length penalty comparable to the vertex-to-cell distance itself
		// and pushes every cell's zone past the threshold, making the grid
		// come back entirely uncovered (worst GDOP = infinity for any layout).
		// Collinear nodes should have higher GDOP than well-positioned nodes
		nodes := []*Node{
			NewNode("node1", "Node 1", NodeTypeVirtual, Point{X: 0, Y: 0, Z: 1.0}),
			NewNode("node2", "Node 2", NodeTypeVirtual, Point{X: 3, Y: 0, Z: 1.0}),
			NewNode("node3", "Node 3", NodeTypeVirtual, Point{X: 6, Y: 0, Z: 1.0}),
		}
		for _, node := range nodes {
			node.Role = RoleTXRX
		}

		worstGDOP := computeWorstGDOP(nodes)

		// Well-positioned nodes (triangular layout) should have finite, reasonable GDOP
		wellPositionedNodes := []*Node{
			NewNode("node1", "Node 1", NodeTypeVirtual, Point{X: 0, Y: 0, Z: 1.0}),
			NewNode("node2", "Node 2", NodeTypeVirtual, Point{X: 6, Y: 0, Z: 1.0}),
			NewNode("node3", "Node 3", NodeTypeVirtual, Point{X: 3, Y: 5, Z: 1.0}),
		}
		for _, node := range wellPositionedNodes {
			node.Role = RoleTXRX
		}

		wellPositionedGDOP := computeWorstGDOP(wellPositionedNodes)

		// Well-positioned nodes should have finite GDOP for this room size
		if math.IsInf(wellPositionedGDOP, 1) {
			t.Errorf("Expected finite GDOP for well-positioned triangular nodes in 6x6m room, got infinity")
		}

		// Well-positioned should have better (lower) GDOP than collinear
		if math.IsInf(worstGDOP, 1) {
			// Collinear resulted in infinity, well-positioned should be finite - test passes
			return
		}

		// If collinear is finite, it should still be worse than well-positioned
		if worstGDOP <= wellPositionedGDOP {
			t.Errorf("Expected collinear nodes (%v) to have higher GDOP than well-positioned triangular nodes (%v)",
				worstGDOP, wellPositionedGDOP)
		}
	})
}
