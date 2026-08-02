// Package localization provides GDOP (Geometric Dilution of Precision) computation
// examples for spatial coverage analysis.
//
// GDOP quantifies how well the geometric arrangement of nodes can localize a point.
// Lower GDOP values indicate better coverage quality.
//
// Key Functions:
//   - computeGDOP(px, pz, nodes): Core 2D GDOP computation for a single point
//   - GDOPMap(positions): Computes GDOP for entire grid (public Engine method)
//   - computeWorstGDOP(nodes): Finds worst coverage across space (simulator)
//   - computeGDOPImprovement(layout, nodeMAC, targetPos): Evaluates repositioning benefit
//
// Example: Basic GDOP Computation
//
//	// Create node positions (corners of a 10x10m room at 2m height)
//	nodes := []localization.NodePosition{
//		{MAC: "AA:BB:CC:DD:EE:F1", X: 0.0, Z: 0.0},
//		{MAC: "AA:BB:CC:DD:EE:F2", X: 10.0, Z: 0.0},
//		{MAC: "AA:BB:CC:DD:EE:F3", X: 0.0, Z: 10.0},
//		{MAC: "AA:BB:CC:DD:EE:F4", X: 10.0, Z: 10.0},
//	}
//
//	// Compute GDOP at center of room (5m, 5m)
//	gdop := localization.computeGDOP(5.0, 5.0, nodes)
//	// Returns ~1.58 (excellent coverage)
//
//	// Compute GDOP at edge (0.1m, 0.1m) - near corner node
//	gdop = localization.computeGDOP(0.1, 0.1, nodes)
//	// Returns ~5.0 (poor coverage - too close to nodes)
//
// Example: Grid-based Coverage Map
//
//	// Create fusion engine for 10x10m room
//	engine := localization.NewEngine(10.0, 10.0, 0.0, 0.0)
//
//	// Same node positions as above
//	positions := []localization.NodePosition{
//		{MAC: "AA:BB:CC:DD:EE:F1", X: 0.0, Z: 0.0},
//		{MAC: "AA:BB:CC:DD:EE:F2", X: 10.0, Z: 0.0},
//		{MAC: "AA:BB:CC:DD:EE:F3", X: 0.0, Z: 10.0},
//		{MAC: "AA:BB:CC:DD:EE:F4", X: 10.0, Z: 10.0},
//	}
//
//	// Generate GDOP map (50x50 grid with 0.2m cells)
//	gdopMap, cols, rows := engine.GDOPMap(positions)
//	// gdopMap: 2500 float32 values (50 * 50)
//	// cols: 50, rows: 50
//
//	// Access cell at row 25, col 25 (center of room)
//	centerGDOP := gdopMap[25*cols+25]
//	// Returns ~1.6 (excellent)
//
//	// Count cells by quality level
//	excellent, good, fair, poor, none := 0, 0, 0, 0, 0
//	for _, val := range gdopMap {
//		switch {
//		case val >= 10.0:
//			none++
//		case val >= 8.0:
//			poor++
//		case val >= 4.0:
//			fair++
//		case val >= 2.0:
//			good++
//		default:
//			excellent++
//		}
//	}
//	// For 4 corner nodes: most cells are "good" or "excellent"
//
// Example: Evaluating Node Repositioning
//
//	// Current layout with suboptimal positioning
//	layout := []*simulator.Node{
//		simulator.NewNode("node1", "Node 1", simulator.NodeTypeVirtual,
//			simulator.Point{X: 0, Y: 0, Z: 2.0}),
//		simulator.NewNode("node2", "Node 2", simulator.NodeTypeVirtual,
//			simulator.Point{X: 1, Y: 0, Z: 2.0}), // Too close to node1!
//		simulator.NewNode("node3", "Node 3", simulator.NodeTypeVirtual,
//			simulator.Point{X: 0, Y: 10, Z: 2.0}),
//	}
//
//	// Current worst-case GDOP (should be high due to poor geometry)
//	currentWorst := simulator.computeWorstGDOP(layout)
//	// Returns > 5.0 (poor coverage)
//
//	// Evaluate moving node2 to better position
//	targetPos := simulator.Point{X: 10, Y: 0, Z: 2.0}
//	improvement := simulator.computeGDOPImprovement(layout, "node2", targetPos)
//	// Returns positive value (e.g., 0.4 = 40% improvement)
//
// Example: GDOP Quality Interpretation
//
//	// Quality thresholds for decision-making
//	func interpretGDOP(gdop float64) string {
//		switch {
//		case gdop < 2.0:
//			return "excellent" // ±0.5m accuracy expected
//		case gdop < 4.0:
//			return "good"      // ±1.0m accuracy expected
//		case gdop < 8.0:
//			return "fair"      // ±2-4m accuracy expected
//		default:
//			return "poor"      // >±4m accuracy or no coverage
//		}
//	}
//
// Mathematical Background:
//
// GDOP is computed from the Fisher information matrix F = HᵀH, where H contains
// direction cosines from each node to the target point. For 2D localization:
//
//   GDOP = sqrt(trace(F⁻¹))
//
// Where:
//   - H[i] = [(px-nx)/d, (pz-nz)/d] for node i at (nx, nz)
//   - d = sqrt((px-nx)² + (pz-nz)²) is distance to node
//   - F is a 2×2 symmetric matrix accumulated over all nodes
//   - trace(F⁻¹) = (F[0,0] + F[1,1]) / det(F)
//
// Geometric interpretation:
//   - Low GDOP: Nodes well-distributed in angle around the point
//   - High GDOP: Nodes collinear or clustered in one direction
//   - Infinite GDOP: Insufficient nodes (<2) or degenerate geometry
//
// Performance considerations:
//   - computeGDOP: O(n) where n = number of nodes (single point)
//   - GDOPMap: O(cols * rows * n) for full grid (typically 2500 * 4 = 10k ops)
//   - computeWorstGDOP: O(cols * rows * n) but over larger simulation space
//
// For real-time usage (e.g., live coverage painting during node drag),
// consider using a coarser grid (0.5m cells) or computing only visible region.
package localization
