package simulator

import (
	"fmt"
	"math"
	mrand "math/rand"
	"time"
)

// GDOPResult contains GDOP computation results for a single cell
type GDOPResult struct {
	X, Y, Z      float64  // Cell center position
	GDOP         float64  // Computed GDOP value (Infinity = no coverage)
	Quality      string   // "excellent", "good", "fair", "poor", "none"
	ContributingLinks []string // Link IDs that contributed to this cell
}

// GridConfig defines the GDOP computation grid
type GridConfig struct {
	CellSize    float64 // Grid cell size in meters
	MinX, MinY  float64 // Grid origin
	Width       float64 // Grid width
	Depth       float64 // Grid depth
}

// GDOPComputer computes Geometric Dilution of Precision for coverage analysis
type GDOPComputer struct {
	links     []Link
	config    GridConfig
	maxZone   int // Maximum Fresnel zone to consider (default 3)
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
			// Add random position
			mrand.Seed(time.Now().UnixNano())
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

// GenerateShoppingList creates a shopping list for hardware based on simulation results
type ShoppingList struct {
	MinimumNodes      int     `json:"minimum_nodes"`
	RecommendedNodes  int     `json:"recommended_nodes"`
	ExpectedAccuracy  float64 `json:"expected_accuracy_m"`
	CoveragePercent   float64 `json:"coverage_percent"`
	OptimalPositions  []Point `json:"optimal_positions"`
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
