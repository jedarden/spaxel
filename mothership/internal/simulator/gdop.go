package simulator

import (
	"fmt"
	"math"
	mrand "math/rand"
)

// GDOPResult contains GDOP computation results for a single cell
type GDOPResult struct {
	X, Y, Z           float64  // Cell center position
	GDOP              float64  // Computed GDOP value (Infinity = no coverage)
	Quality           string   // "excellent", "good", "fair", "poor", "none"
	ContributingLinks []string // Link IDs that contributed to this cell
}

// GridConfig defines the GDOP computation grid
type GridConfig struct {
	CellSize   float64 // Grid cell size in meters
	MinX, MinY float64 // Grid origin
	Width      float64 // Grid width
	Depth      float64 // Grid depth
}

// GDOPComputer computes Geometric Dilution of Precision for coverage analysis
type GDOPComputer struct {
	links   []Link
	config  GridConfig
	maxZone int // Maximum Fresnel zone to consider (default 3)
}

// NewGDOPComputer creates a new GDOP computer
func NewGDOPComputer(links []Link, config GridConfig) *GDOPComputer {
	if config.CellSize <= 0 {
		config.CellSize = 0.2 // Default 20cm
	}
	return &GDOPComputer{
		links:   links,
		config:  config,
		maxZone: 3, // Default: consider first 3 Fresnel zones
	}
}

// SetMaxZone sets the maximum Fresnel zone to consider
func (gc *GDOPComputer) SetMaxZone(zone int) {
	if zone < 1 {
		zone = 1
	}
	gc.maxZone = zone
}

// ComputeAll computes GDOP for the entire grid
// Returns a slice of GDOP results indexed by cell position
func (gc *GDOPComputer) ComputeAll() [][]GDOPResult {
	nx := int(math.Ceil(gc.config.Width / gc.config.CellSize))
	ny := int(math.Ceil(gc.config.Depth / gc.config.CellSize))

	results := make([][]GDOPResult, ny)

	for iy := 0; iy < ny; iy++ {
		results[iy] = make([]GDOPResult, nx)
		for ix := 0; ix < nx; ix++ {
			x := gc.config.MinX + (float64(ix)+0.5)*gc.config.CellSize
			y := gc.config.MinY + (float64(iy)+0.5)*gc.config.CellSize
			z := 1.0 // Use 1m height for 2D GDOP analysis

			result := gc.ComputeAt(x, y, z)
			results[iy][ix] = result
		}
	}

	return results
}

// ComputeAt computes GDOP at a specific point
func (gc *GDOPComputer) ComputeAt(x, y, z float64) GDOPResult {
	point := Point{X: x, Y: y, Z: z}

	// Collect links that cover this point (within maxZone Fresnel zones)
	var coveringLinks []Link
	var linkIDs []string

	for _, link := range gc.links {
		if IsInFresnelZones(link.TX.Position, link.RX.Position, point, gc.maxZone) {
			coveringLinks = append(coveringLinks, link)
			linkIDs = append(linkIDs, link.TX.ID+":"+link.RX.ID)
		}
	}

	result := GDOPResult{
		X:                 x,
		Y:                 y,
		Z:                 z,
		ContributingLinks: linkIDs,
	}

	if len(coveringLinks) < 2 {
		// Need at least 2 links for 2D localization
		result.GDOP = math.Inf(1)
		result.Quality = "none"
		return result
	}

	// Compute GDOP using angular diversity
	gdop := gc.computeGDOPAngular(point, coveringLinks)
	result.GDOP = gdop
	result.Quality = gdopToQuality(gdop)

	return result
}

// computeGDOPAngular computes GDOP based on angular diversity of link directions
// This is the 2D GDOP formula from the plan
func (gc *GDOPComputer) computeGDOPAngular(point Point, links []Link) float64 {
	// Step 1: Collect link angles
	// For each link, compute the angle of the line from TX to RX as seen from point
	type linkAngle struct {
		theta float64 // angle in radians
		link  Link
	}

	angles := make([]linkAngle, 0, len(links))
	for _, link := range links {
		// Project to floor plane (ignore Z for 2D analysis)
		dx := link.RX.Position.X - link.TX.Position.X
		dy := link.RX.Position.Y - link.TX.Position.Y
		theta := math.Atan2(dy, dx)
		angles = append(angles, linkAngle{theta: theta, link: link})
	}

	if len(angles) < 2 {
		return math.Inf(1)
	}

	// Step 2: Build Fisher information matrix
	// F = Σ [ [cos²(θ),       cos(θ)·sin(θ)],
	//        [cos(θ)·sin(θ), sin²(θ)       ] ]
	var f00, f01, f11 float64

	for _, la := range angles {
		c := math.Cos(la.theta)
		s := math.Sin(la.theta)

		f00 += c * c
		f01 += c * s
		f11 += s * s
	}

	// Step 3: Compute determinant
	det := f00*f11 - f01*f01

	// Check for degenerate geometry (collinear links)
	if det <= 1e-6 {
		return math.Inf(1)
	}

	// Step 4: Compute trace of F^-1
	// For 2x2 matrix: trace(F^-1) = (f00 + f11) / det
	traceFInv := (f00 + f11) / det

	// Step 5: GDOP = sqrt(trace(F^-1))
	gdop := math.Sqrt(traceFInv)

	return gdop
}

// gdopToQuality converts GDOP value to quality string
func gdopToQuality(gdop float64) string {
	if math.IsInf(gdop, 0) {
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

// CoverageScore computes the percentage of cells with "good" or better coverage
func (gc *GDOPComputer) CoverageScore(results [][]GDOPResult) float64 {
	if len(results) == 0 || len(results[0]) == 0 {
		return 0
	}

	goodCells := 0
	totalCells := 0

	for _, row := range results {
		for _, cell := range row {
			totalCells++
			if cell.Quality == "excellent" || cell.Quality == "good" {
				goodCells++
			}
		}
	}

	return 100.0 * float64(goodCells) / float64(totalCells)
}

// AverageGDOP computes the average GDOP over all cells (excluding infinity)
func (gc *GDOPComputer) AverageGDOP(results [][]GDOPResult) float64 {
	sum := 0.0
	count := 0

	for _, row := range results {
		for _, cell := range row {
			if !math.IsInf(cell.GDOP, 0) {
				sum += cell.GDOP
				count++
			}
		}
	}

	if count == 0 {
		return math.Inf(1)
	}
	return sum / float64(count)
}

// QualityCounts returns the count of cells by quality level
func (gc *GDOPComputer) QualityCounts(results [][]GDOPResult) map[string]int {
	counts := map[string]int{
		"excellent": 0,
		"good":      0,
		"fair":      0,
		"poor":      0,
		"none":      0,
	}

	for _, row := range results {
		for _, cell := range row {
			counts[cell.Quality]++
		}
	}

	return counts
}

// FindDeadZones returns positions where coverage is "none" or "poor"
func (gc *GDOPComputer) FindDeadZones(results [][]GDOPResult) []Point {
	deadZones := make([]Point, 0)

	for _, row := range results {
		for _, cell := range row {
			if cell.Quality == "none" || cell.Quality == "poor" {
				deadZones = append(deadZones, Point{X: cell.X, Y: cell.Y, Z: cell.Z})
			}
		}
	}

	return deadZones
}

// RecommendNodePosition suggests optimal positions for additional nodes
// based on current dead zones
func (gc *GDOPComputer) RecommendNodePosition(results [][]GDOPResult, space *Space) Point {
	// Find the centroid of the largest dead zone
	deadZones := gc.FindDeadZones(results)
	if len(deadZones) == 0 {
		// No dead zones, suggest center of space
		minX, minY, _, maxX, maxY, _ := space.Bounds()
		return Point{
			X: (minX + maxX) / 2,
			Y: (minY + maxY) / 2,
			Z: 2.0, // Suggest high placement
		}
	}

	// Cluster dead zones and find the largest cluster
	// Simplified: just return the centroid of all dead zones
	var sumX, sumY float64
	for _, dz := range deadZones {
		sumX += dz.X
		sumY += dz.Y
	}

	centroid := Point{
		X: sumX / float64(len(deadZones)),
		Y: sumY / float64(len(deadZones)),
		Z: 2.0, // Suggest high placement for better coverage
	}

	return centroid
}

// MinimumNodeCount estimates the minimum number of nodes needed for good coverage
// Based on space dimensions and desired quality threshold
func MinimumNodeCount(space *Space, targetGDOP float64) int {
	width, depth, _ := space.Dimensions()
	area := width * depth

	// Heuristic: nodes needed based on area and desired GDOP
	// For GDOP < 4 (good coverage): approximately 1 node per 15-20 m²
	if targetGDOP < 2 {
		// Excellent coverage requires more nodes
		return int(math.Ceil(area / 15.0))
	}
	if targetGDOP < 4 {
		// Good coverage
		return int(math.Ceil(area / 20.0))
	}
	// Fair coverage
	return int(math.Ceil(area / 30.0))
}

// ExpectedAccuracy estimates the expected localization accuracy at a point
// based on its GDOP value
func ExpectedAccuracy(gdop float64) float64 {
	if math.IsInf(gdop, 0) {
		return math.Inf(1)
	}

	// Based on research: typical CSI accuracy with 4+ nodes is ±0.5-1.0m
	// GDOP < 2: ±0.5m, GDOP 2-4: ±1.0m, GDOP > 4: degrades further
	baseAccuracy := 0.5 // meters for GDOP = 1

	return baseAccuracy * gdop
}

// GDOPColor represents a color for GDOP visualization
type GDOPColor struct {
	R, G, B uint8 // RGB values 0-255
}

// GDOPColorMap returns the color for a given GDOP value for visualization
// Uses: green (excellent), yellow (good), orange (fair), red (poor), gray (none)
func GDOPColorMap(gdop float64) GDOPColor {
	if math.IsInf(gdop, 0) {
		return GDOPColor{R: 80, G: 80, B: 80} // Gray for no coverage
	}
	if gdop < 2.0 {
		return GDOPColor{R: 34, G: 197, B: 94} // Green (#22c65e) for excellent
	}
	if gdop < 4.0 {
		return GDOPColor{R: 255, G: 193, B: 7} // Yellow (#ffc107) for good
	}
	if gdop < 8.0 {
		return GDOPColor{R: 255, G: 146, B: 0} // Orange (#ff9200) for fair
	}
	return GDOPColor{R: 220, G: 53, B: 69} // Red (#dc3545) for poor
}

// GDOPHeatmapData represents flattened GDOP data for frontend rendering
type GDOPHeatmapData struct {
	Width       int       `json:"width"`        // Grid width (columns)
	Depth       int       `json:"depth"`        // Grid depth (rows)
	CellSize    float64   `json:"cell_size"`    // Cell size in meters
	OriginX     float64   `json:"origin_x"`     // Grid origin X
	OriginY     float64   `json:"origin_y"`     // Grid origin Y
	GDOPValues  []float64 `json:"gdop_values"`  // Flattened GDOP values (9999 = infinity)
	Qualities   []string  `json:"qualities"`    // Flattened quality strings
	Colors      [][]uint8 `json:"colors"`       // Flattened RGB colors [width*depth*3]
	AccuracyMap []float64 `json:"accuracy_map"` // Expected accuracy in meters per cell
}

// ToHeatmapData converts GDOP results to a heatmap-friendly format
func (gc *GDOPComputer) ToHeatmapData(results [][]GDOPResult) *GDOPHeatmapData {
	if len(results) == 0 || len(results[0]) == 0 {
		return &GDOPHeatmapData{}
	}

	depth := len(results)    // rows (Y)
	width := len(results[0]) // cols (X)
	totalCells := width * depth

	data := &GDOPHeatmapData{
		Width:       width,
		Depth:       depth,
		CellSize:    gc.config.CellSize,
		OriginX:     gc.config.MinX,
		OriginY:     gc.config.MinY,
		GDOPValues:  make([]float64, totalCells),
		Qualities:   make([]string, totalCells),
		Colors:      make([][]uint8, totalCells),
		AccuracyMap: make([]float64, totalCells),
	}

	for y := 0; y < depth; y++ {
		for x := 0; x < width; x++ {
			idx := y*width + x
			result := results[y][x]

			// GDOP value (9999 for infinity)
			if math.IsInf(result.GDOP, 0) {
				data.GDOPValues[idx] = 9999.0
			} else {
				data.GDOPValues[idx] = result.GDOP
			}

			// Quality string
			data.Qualities[idx] = result.Quality

			// RGB color
			color := GDOPColorMap(result.GDOP)
			data.Colors[idx] = []uint8{color.R, color.G, color.B}

			// Expected accuracy (use 9999.0 for infinity)
			accuracy := ExpectedAccuracy(result.GDOP)
			if math.IsInf(accuracy, 0) {
				data.AccuracyMap[idx] = 9999.0
			} else {
				data.AccuracyMap[idx] = accuracy
			}
		}
	}

	return data
}

// ComputeAccuracyMap computes expected accuracy for each cell
// Returns a 2D array of accuracy values in meters (infinity = no coverage)
func (gc *GDOPComputer) ComputeAccuracyMap(results [][]GDOPResult) [][]float64 {
	if len(results) == 0 {
		return nil
	}

	accuracyMap := make([][]float64, len(results))
	for i := range results {
		accuracyMap[i] = make([]float64, len(results[i]))
		for j := range results[i] {
			accuracyMap[i][j] = ExpectedAccuracy(results[i][j].GDOP)
		}
	}

	return accuracyMap
}

// ComputeColorMap computes RGB colors for each cell for visualization
// Returns a flattened array of RGB values [width*depth*3]
func (gc *GDOPComputer) ComputeColorMap(results [][]GDOPResult) [][]uint8 {
	if len(results) == 0 || len(results[0]) == 0 {
		return nil
	}

	depth := len(results)
	width := len(results[0])
	totalCells := width * depth

	colors := make([][]uint8, totalCells)

	for y := 0; y < depth; y++ {
		for x := 0; x < width; x++ {
			idx := y*width + x
			color := GDOPColorMap(results[y][x].GDOP)
			colors[idx] = []uint8{color.R, color.G, color.B}
		}
	}

	return colors
}

// GetWorstCoverageCells returns the N cells with the worst GDOP values
func (gc *GDOPComputer) GetWorstCoverageCells(results [][]GDOPResult, n int) []GDOPResult {
	if len(results) == 0 {
		return nil
	}

	// Flatten all cells
	cells := make([]GDOPResult, 0)
	for _, row := range results {
		cells = append(cells, row...)
	}

	// Sort by GDOP (descending, so worst first)
	for i := 0; i < len(cells); i++ {
		for j := i + 1; j < len(cells); j++ {
			// Handle infinity: infinity is worse than any finite value
			iInf := math.IsInf(cells[i].GDOP, 0)
			jInf := math.IsInf(cells[j].GDOP, 0)

			var swap bool
			if iInf && !jInf {
				swap = false // i stays (infinity at top)
			} else if !iInf && jInf {
				swap = true // j is infinity, should be before i
			} else if !iInf && !jInf {
				swap = cells[j].GDOP > cells[i].GDOP
			}

			if swap {
				cells[i], cells[j] = cells[j], cells[i]
			}
		}
	}

	// Return top N worst cells
	if n > len(cells) {
		n = len(cells)
	}
	return cells[:n]
}

// GetBestCoverageCells returns the N cells with the best GDOP values
func (gc *GDOPComputer) GetBestCoverageCells(results [][]GDOPResult, n int) []GDOPResult {
	if len(results) == 0 {
		return nil
	}

	// Flatten all cells
	cells := make([]GDOPResult, 0)
	for _, row := range results {
		cells = append(cells, row...)
	}

	// Sort by GDOP (ascending, so best first)
	for i := 0; i < len(cells); i++ {
		for j := i + 1; j < len(cells); j++ {
			// Handle infinity: finite values are better than infinity
			iInf := math.IsInf(cells[i].GDOP, 0)
			jInf := math.IsInf(cells[j].GDOP, 0)

			var swap bool
			if !iInf && jInf {
				swap = false // i is finite, j is infinity, i is better
			} else if iInf && !jInf {
				swap = true // i is infinity, j is finite, j should be before i
			} else if !iInf && !jInf {
				swap = cells[j].GDOP < cells[i].GDOP
			}

			if swap {
				cells[i], cells[j] = cells[j], cells[i]
			}
		}
	}

	// Return top N best cells
	if n > len(cells) {
		n = len(cells)
	}
	return cells[:n]
}

// OptimizeNodePositions uses a greedy algorithm to find better node positions
// for a given number of nodes within the space
func OptimizeNodePositions(space *Space, numNodes int, iterations int) *NodeSet {
	minX, minY, _, maxX, maxY, maxZ := space.Bounds()

	// Start with corner positions
	bestSet := NewNodeSet()
	corners := CornerPositions(space)

	for i := 0; i < numNodes; i++ {
		if i < len(corners) {
			bestSet.AddVirtualNode(
				fmt.Sprintf("node-%d", i),
				fmt.Sprintf("Node %d", i+1),
				corners[i],
			)
		} else {
			// Add random position (Go 1.20+ automatically seeds global generator)
			pos := Point{
				X: minX + mrand.Float64()*(maxX-minX),
				Y: minY + mrand.Float64()*(maxY-minY),
				Z: mrand.Float64() * maxZ,
			}
			bestSet.AddVirtualNode(
				fmt.Sprintf("node-%d", i),
				fmt.Sprintf("Node %d", i+1),
				pos,
			)
		}
	}

	// Generate initial links and compute coverage
	links := GenerateAllLinks(bestSet)
	gdopComp := NewGDOPComputer(links, GridConfig{
		MinX:     minX,
		MinY:     minY,
		Width:    maxX - minX,
		Depth:    maxY - minY,
		CellSize: 0.2,
	})
	results := gdopComp.ComputeAll()
	bestScore := gdopComp.CoverageScore(results)

	// Iterative improvement
	for iter := 0; iter < iterations; iter++ {
		// Try moving each node slightly
		for i, node := range bestSet.All() {
			// Generate candidate position
			candidatePos := Point{
				X: node.Position.X + (mrand.Float64()-0.5)*1.0, // ±0.5m
				Y: node.Position.Y + (mrand.Float64()-0.5)*1.0,
				Z: node.Position.Z + (mrand.Float64()-0.5)*0.5, // Less Z variation
			}

			// Keep within bounds
			candidatePos.X = math.Max(minX, math.Min(maxX, candidatePos.X))
			candidatePos.Y = math.Max(minY, math.Min(maxY, candidatePos.Y))
			candidatePos.Z = math.Max(0, math.Min(maxZ, candidatePos.Z))

			// Create test set with this node moved
			testSet := NewNodeSet()
			for j, n := range bestSet.All() {
				if j == i {
					testSet.AddVirtualNode(n.ID, n.Name, candidatePos)
				} else {
					testSet.Add(n)
				}
			}

			// Evaluate
			testLinks := GenerateAllLinks(testSet)
			testGDOP := NewGDOPComputer(testLinks, GridConfig{
				MinX:     minX,
				MinY:     minY,
				Width:    maxX - minX,
				Depth:    maxY - minY,
				CellSize: 0.2,
			})
			testResults := testGDOP.ComputeAll()
			testScore := testGDOP.CoverageScore(testResults)

			// Keep if better
			if testScore > bestScore {
				bestScore = testScore
				bestSet.All()[i].Position = candidatePos
			}
		}
	}

	return bestSet
}

// GenerateShoppingList creates a shopping list from simulation results
func GenerateShoppingList(space *Space, currentNodes *NodeSet) *ShoppingList {
	nodes := currentNodes
	if nodes == nil || nodes.Count() == 0 {
		nodes = SuggestedNodes(space, 4)
	}

	links := GenerateAllLinks(nodes)
	minX, minY, _, maxX, maxY, _ := space.Bounds()

	gdopComp := NewGDOPComputer(links, GridConfig{
		MinX:     minX,
		MinY:     minY,
		Width:    maxX - minX,
		Depth:    maxY - minY,
		CellSize: 0.2,
	})
	results := gdopComp.ComputeAll()

	coverage := gdopComp.CoverageScore(results)
	avgGDOP := gdopComp.AverageGDOP(results)

	return &ShoppingList{
		MinimumNodes:     MinimumNodeCount(space, 4.0),
		RecommendedNodes: nodes.Count(),
		ExpectedAccuracy: ExpectedAccuracy(avgGDOP),
		CoveragePercent:  coverage,
		OptimalPositions: extractNodePositions(nodes),
	}
}

func extractNodePositions(nodes *NodeSet) []Point {
	positions := make([]Point, 0, nodes.Count())
	for _, n := range nodes.All() {
		positions = append(positions, n.Position)
	}
	return positions
}

// computeGDOPImprovement computes the GDOP improvement for a hypothetical node repositioning.
//
// This function evaluates how much the overall coverage would improve or degrade if a
// specific node were moved to a new target position. It computes the worst-case GDOP
// for both the current layout and a hypothetical layout with the node moved, then
// returns the relative improvement.
//
// Algorithm:
// 1. Compute worst-case GDOP for current layout across entire space
// 2. Create hypothetical layout with target node moved to new position
// 3. Compute worst-case GDOP for hypothetical layout
// 4. Calculate improvement: (currentWorstGDOP - newWorstGDOP) / currentWorstGDOP
//
// Parameters:
//   currentLayout - Slice of all nodes in their current positions
//   nodeMAC       - MAC address (or ID) of the node to move
//   targetPos     - Target position to move the node to
//
// Returns:
//   Relative improvement in range [-1.0, 1.0]:
//   - Positive (0 to 1): Improvement (lower GDOP is better)
//   - Negative (-1 to 0): Degradation (higher GDOP is worse)
//   - 0.0: No change, node not found, or no coverage baseline
//   - 1.0: Maximum improvement (reduced to near-zero GDOP)
//   - -1.0: Complete coverage loss at target position
//
// Requirements:
//   - Minimum 2 nodes in layout required
//   - At least 2 links required for meaningful GDOP calculation
//   - Node MAC/ID must exist in currentLayout
//
// Example usage:
//
//   layout := []*Node{
//     NewNode("node1", "Node 1", NodeTypeVirtual, Point{X: 0, Y: 0, Z: 2.0}),
//     NewNode("node2", "Node 2", NodeTypeVirtual, Point{X: 10, Y: 0, Z: 2.0}),
//     NewNode("node3", "Node 3", NodeTypeVirtual, Point{X: 0, Y: 10, Z: 2.0}),
//   }
//   targetPos := Point{X: 5, Y: 5, Z: 2.0} // Move to center
//   improvement := computeGDOPImprovement(layout, "node1", targetPos)
//   // Returns ~0.3 (30% improvement) for moving corner node to center
//
func computeGDOPImprovement(currentLayout []*Node, nodeMAC string, targetPos Point) float64 {
	// Step 1: Compute worst-case GDOP for current layout
	currentWorstGDOP := computeWorstGDOP(currentLayout)

	// Handle edge case: current layout has no coverage
	if math.IsInf(currentWorstGDOP, 1) {
		// If current has no coverage, any improvement is meaningless
		return 0.0
	}

	// Step 2: Create hypothetical layout with node moved to target position
	hypotheticalLayout := make([]*Node, 0, len(currentLayout))
	found := false
	for _, node := range currentLayout {
		newNode := &Node{
			ID:       node.ID,
			Name:     node.Name,
			Type:     node.Type,
			Role:     node.Role,
			Position: node.Position,
			Enabled:  node.Enabled,
			APBSSID:  node.APBSSID,
			APChannel: node.APChannel,
		}

		// Move the target node to the new position
		if node.ID == nodeMAC || node.GenerateMAC() == nodeMAC {
			newNode.Position = targetPos
			found = true
		}

		hypotheticalLayout = append(hypotheticalLayout, newNode)
	}

	// If node not found, no change possible
	if !found {
		return 0.0
	}

	// Step 3: Compute worst-case GDOP for hypothetical layout
	newWorstGDOP := computeWorstGDOP(hypotheticalLayout)

	// Handle edge case: hypothetical layout has no coverage
	if math.IsInf(newWorstGDOP, 1) {
		// Moving to this position results in complete coverage loss
		return -1.0
	}

	// Step 4: Compute relative improvement
	// Improvement = (currentWorstGDOP - newWorstGDOP) / currentWorstGDOP
	// Positive value = improvement (GDOP decreased)
	// Negative value = degradation (GDOP increased)
	improvement := (currentWorstGDOP - newWorstGDOP) / currentWorstGDOP

	// Step 5: Clamp to sane range [-1.0, 1.0]
	if improvement > 1.0 {
		improvement = 1.0
	} else if improvement < -1.0 {
		improvement = -1.0
	}

	return improvement
}

// computeWorstGDOP calculates the worst-case GDOP value across all grid cells for a given node layout.
//
// This function evaluates the coverage quality of a node layout by finding the cell with
// the highest (worst) GDOP value. A good layout should have a low worst-case GDOP,
// indicating that even the worst-covered area has reasonable localization accuracy.
//
// Algorithm:
// 1. Generate all links from the node set (TX→RX pairs)
// 2. Create a grid covering the space bounded by node positions ± 1m margin
// 3. Compute GDOP for each cell using angular diversity of covering links
// 4. Return the maximum GDOP found (worst coverage)
//
// Parameters:
//   nodes - Slice of nodes to evaluate. Must have at least 2 nodes.
//
// Returns:
//   Worst-case GDOP value:
//   - < 2.0: Excellent layout (worst area still has good coverage)
//   - 2.0-4.0: Good layout
//   - 4.0-8.0: Fair layout (some areas with poor coverage)
//   - > 8.0: Poor layout (significant coverage gaps)
//   - Infinity: No coverage (insufficient nodes or links)
//
// Requirements:
//   - Minimum 2 nodes required
//   - At least 2 valid links required (depends on node roles)
//   - Nodes must have valid (non-infinite) positions
//
// Example usage:
//
//   nodes := []*Node{
//     NewNode("node1", "Node 1", NodeTypeVirtual, Point{X: 0, Y: 0, Z: 2.0}),
//     NewNode("node2", "Node 2", NodeTypeVirtual, Point{X: 10, Y: 0, Z: 2.0}),
//     NewNode("node3", "Node 3", NodeTypeVirtual, Point{X: 0, Y: 10, Z: 2.0}),
//     NewNode("node4", "Node 4", NodeTypeVirtual, Point{X: 10, Y: 10, Z: 2.0}),
//   }
//   worstGDOP := computeWorstGDOP(nodes)
//   // Returns ~1.8 for well-positioned 4-node corner layout
//
func computeWorstGDOP(nodes []*Node) float64 {
	if len(nodes) < 2 {
		return math.Inf(1) // Need at least 2 nodes for localization
	}

	// Create a NodeSet and generate links
	nodeSet := NewNodeSet()
	for _, node := range nodes {
		if node != nil {
			nodeSet.Add(node)
		}
	}

	links := GenerateAllLinks(nodeSet)
	if len(links) < 2 {
		return math.Inf(1) // Need at least 2 links for GDOP calculation
	}

	// Determine grid bounds from node positions
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)

	for _, node := range nodes {
		if node != nil && node.Enabled {
			p := node.Position
			if p.X < minX {
				minX = p.X
			}
			if p.X > maxX {
				maxX = p.X
			}
			if p.Y < minY {
				minY = p.Y
			}
			if p.Y > maxY {
				maxY = p.Y
			}
		}
	}

	// If no valid bounds found, use defaults
	if math.IsInf(minX, 1) || math.IsInf(maxX, -1) {
		minX, maxX = -5.0, 5.0
		minY, maxY = -5.0, 5.0
	}

	// Add margin to bounds
	margin := 1.0
	minX -= margin
	minY -= margin
	maxX += margin
	maxY += margin

	// Create GDOP computer
	gdopComp := NewGDOPComputer(links, GridConfig{
		MinX:     minX,
		MinY:     minY,
		Width:    maxX - minX,
		Depth:    maxY - minY,
		CellSize: 0.2, // 20cm grid cells
	})

	// Compute GDOP for all cells
	results := gdopComp.ComputeAll()

	// Find worst GDOP value (excluding infinity)
	// Track if we found any valid cells
	foundValid := false
	worstGDOP := 0.0
	for _, row := range results {
		for _, cell := range row {
			if !math.IsInf(cell.GDOP, 0) {
				foundValid = true
				if cell.GDOP > worstGDOP {
					worstGDOP = cell.GDOP
				}
			}
		}
	}

	// If all cells are infinity (no coverage), return infinity
	if !foundValid {
		return math.Inf(1)
	}

	return worstGDOP
}
