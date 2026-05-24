// Package simulator provides accuracy estimation for the pre-deployment simulator.
package simulator

import (
	"fmt"
	"math"
	mrand "math/rand"
)

// AccuracyEstimator computes accuracy metrics from simulation results.
type AccuracyEstimator struct{}

// NewAccuracyEstimator creates a new accuracy estimator.
func NewAccuracyEstimator() *AccuracyEstimator {
	return &AccuracyEstimator{}
}

// AccuracyReport contains accuracy metrics from a simulation run.
type AccuracyReport struct {
	MedianError       float64 `json:"median_error_m"`      // Median position error in meters
	MeanError         float64 `json:"mean_error_m"`        // Mean position error in meters
	MaxError          float64 `json:"max_error_m"`         // Maximum position error in meters
	P95Error          float64 `json:"p95_error_m"`         // 95th percentile error
	DetectionRate     float64 `json:"detection_rate"`      // Fraction of walkers detected
	FalsePositiveRate float64 `json:"false_positive_rate"` // False positives per second
	RecallAt1m        float64 `json:"recall_at_1m"`        // Fraction within 1m of true position
	RecallAt2m        float64 `json:"recall_at_2m"`        // Fraction within 2m of true position
	SampleCount       int     `json:"sample_count"`        // Number of walker positions evaluated
}

// Recommendation is a deployment recommendation.
type Recommendation struct {
	Priority string  `json:"priority"`           // "high", "medium", "low"
	Message  string  `json:"message"`            // Human-readable recommendation
	Impact   float64 `json:"impact"`             // Estimated improvement (0-1)
	Position *Point  `json:"position,omitempty"` // Suggested position (if applicable)
}

// RecommendationEngine generates deployment recommendations.
type RecommendationEngine struct{}

// NewRecommendationEngine creates a new recommendation engine.
func NewRecommendationEngine() *RecommendationEngine {
	return &RecommendationEngine{}
}

// Compute evaluates accuracy metrics from walker positions and blob detections.
func (ae *AccuracyEstimator) Compute(walkers []*SimWalker, blobs []BlobResult) AccuracyReport {
	if len(walkers) == 0 {
		return AccuracyReport{}
	}

	// Collect all true positions and matched blob positions
	truePositions := make([]Point, 0)
	detectedPositions := make([]Point, 0)
	errors := make([]float64, 0)

	for _, walker := range walkers {
		for _, truePos := range walker.TrueHistory {
			truePositions = append(truePositions, truePos)

			// Find nearest blob
			nearestDist := math.Inf(1)
			for _, blob := range blobs {
				if blob.WalkerID == walker.ID {
					dist := blob.Position.Distance(truePos)
					if dist < nearestDist {
						nearestDist = dist
					}
				}
			}

			if !math.IsInf(nearestDist, 1) {
				detectedPositions = append(detectedPositions, truePos)
				errors = append(errors, nearestDist)
			}
		}
	}

	if len(errors) == 0 {
		return AccuracyReport{
			MedianError:   math.Inf(1),
			MeanError:     math.Inf(1),
			MaxError:      math.Inf(1),
			DetectionRate: 0,
			SampleCount:   len(truePositions),
		}
	}

	// Compute statistics
	meanError := 0.0
	for _, e := range errors {
		meanError += e
	}
	meanError /= float64(len(errors))

	// Median error
	sortedErrors := make([]float64, len(errors))
	copy(sortedErrors, errors)
	for i := 0; i < len(sortedErrors); i++ {
		for j := i + 1; j < len(sortedErrors); j++ {
			if sortedErrors[i] > sortedErrors[j] {
				sortedErrors[i], sortedErrors[j] = sortedErrors[j], sortedErrors[i]
			}
		}
	}
	medianError := sortedErrors[len(sortedErrors)/2]

	// Max error
	maxError := sortedErrors[len(sortedErrors)-1]

	// 95th percentile
	p95Index := int(float64(len(sortedErrors)) * 0.95)
	if p95Index >= len(sortedErrors) {
		p95Index = len(sortedErrors) - 1
	}
	p95Error := sortedErrors[p95Index]

	// Detection rate
	detectionRate := float64(len(detectedPositions)) / float64(len(truePositions))

	// Recall at 1m and 2m
	recall1m := 0.0
	recall2m := 0.0
	for _, e := range errors {
		if e <= 1.0 {
			recall1m++
		}
		if e <= 2.0 {
			recall2m++
		}
	}
	recall1m /= float64(len(errors))
	recall2m /= float64(len(errors))

	// False positive rate (blobs without matching walker)
	falsePositives := 0
	for _, blob := range blobs {
		hasMatch := false
		for _, walker := range walkers {
			if blob.WalkerID == walker.ID {
				hasMatch = true
				break
			}
		}
		if !hasMatch {
			falsePositives++
		}
	}
	falsePositiveRate := float64(falsePositives) / float64(len(errors))

	return AccuracyReport{
		MedianError:       medianError,
		MeanError:         meanError,
		MaxError:          maxError,
		P95Error:          p95Error,
		DetectionRate:     detectionRate,
		FalsePositiveRate: falsePositiveRate,
		RecallAt1m:        recall1m,
		RecallAt2m:        recall2m,
		SampleCount:       len(errors),
	}
}

// Generate generates recommendations based on space, nodes, GDOP, and coverage.
func (re *RecommendationEngine) Generate(space *Space, nodes *NodeSet, gdopMap []float64, coverageScore float64) []Recommendation {
	recs := make([]Recommendation, 0)

	// Check coverage score
	if coverageScore < 50 {
		recs = append(recs, Recommendation{
			Priority: "high",
			Message:  fmt.Sprintf("Coverage is below 50%% (%.0f%%). Consider adding more nodes.", coverageScore),
			Impact:   0.3,
		})
	}

	// Check node count
	nodeCount := nodes.Count()
	if nodeCount < 4 {
		recs = append(recs, Recommendation{
			Priority: "medium",
			Message:  fmt.Sprintf("Only %d nodes. For best accuracy, use at least 4 nodes.", nodeCount),
			Impact:   0.2,
		})
	}

	// Check height diversity
	hasLow, hasHigh := false, false
	for _, node := range nodes.All() {
		if node.Position.Z < 1.0 {
			hasLow = true
		}
		if node.Position.Z > 2.0 {
			hasHigh = true
		}
	}

	if !hasLow || !hasHigh {
		recs = append(recs, Recommendation{
			Priority: "medium",
			Message:  "For better Z-axis accuracy, place nodes at mixed heights (some low, some high).",
			Impact:   0.15,
		})
	}

	// Find worst coverage areas
	minX, minY, _, maxX, maxY, _ := space.Bounds()
	if len(gdopMap) > 0 {
		// Find cells with worst GDOP (highest values, excluding infinity)
		maxGDOP := 0.0
		worstIdx := -1

		for i, gdop := range gdopMap {
			if !math.IsInf(gdop, 0) && gdop > maxGDOP {
				maxGDOP = gdop
				worstIdx = i
			}
		}

		if maxGDOP > 8.0 && worstIdx >= 0 {
			// Compute position from index
			widthCells := int(math.Ceil((maxX - minX) / 0.2))
			depthCells := int(math.Ceil((maxY - minY) / 0.2))

			_ = worstIdx / (widthCells * depthCells) // z-layer index, not used in 2D recommendation
			remainder := worstIdx % (widthCells * depthCells)
			x := remainder / depthCells
			y := remainder % depthCells

			posX := minX + float64(x)*0.2 + 0.1
			posY := minY + float64(y)*0.2 + 0.1

			recs = append(recs, Recommendation{
				Priority: "high",
				Message:  fmt.Sprintf("Poor coverage detected near (%.1f, %.1f). Consider adding a node nearby.", posX, posY),
				Impact:   0.25,
				Position: &Point{X: posX, Y: posY, Z: 2.0},
			})
		}
	}

	// Check for collinear nodes
	if nodeCount >= 3 {
		angles := make([]float64, 0, nodeCount)
		for _, node := range nodes.All() {
			// Compute angle from center
			centerX := (minX + maxX) / 2
			centerY := (minY + maxY) / 2
			angle := math.Atan2(node.Position.Y-centerY, node.Position.X-centerX)
			angles = append(angles, angle)
		}

		// Check if all angles are similar (collinear)
		angleSpread := 0.0
		for i := 1; i < len(angles); i++ {
			diff := math.Abs(angles[i] - angles[0])
			for diff > math.Pi {
				diff -= 2 * math.Pi
			}
			for diff < -math.Pi {
				diff += 2 * math.Pi
			}
			angleSpread += diff
		}
		angleSpread /= float64(len(angles) - 1)

		if angleSpread < 0.3 { // Less than ~17 degrees spread
			recs = append(recs, Recommendation{
				Priority: "medium",
				Message:  "Nodes appear to be nearly collinear. Spread them out for better coverage.",
				Impact:   0.2,
			})
		}
	}

	// Estimate improvement with additional nodes
	if nodeCount >= 2 && nodeCount < 8 {
		// Estimate improvement from adding one node
		estimatedImprovement := 0.1 * float64(8-nodeCount) / 6.0
		recs = append(recs, Recommendation{
			Priority: "low",
			Message:  fmt.Sprintf("Adding a node could improve accuracy by ~%.0f%%.", estimatedImprovement*100),
			Impact:   estimatedImprovement,
		})
	}

	// If no issues found
	if len(recs) == 0 {
		recs = append(recs, Recommendation{
			Priority: "low",
			Message:  "Coverage looks good! No specific recommendations.",
			Impact:   0,
		})
	}

	return recs
}

// ShoppingList contains hardware recommendations.
type ShoppingList struct {
	MinimumNodes         int             `json:"minimum_nodes"`
	RecommendedNodes     int             `json:"recommended_nodes"`
	ExpectedAccuracy     float64         `json:"expected_accuracy_m"`
	CoveragePercent      float64         `json:"coverage_percent"`
	HardwareList         []string        `json:"hardware_list"`
	AmazonSearchURL      string          `json:"amazon_search_url"`
	OptimalPositions     []Point         `json:"optimal_positions,omitempty"`
	CoverageGaps         []Point         `json:"coverage_gaps,omitempty"`         // Positions with poor coverage
	RecommendedAdditions []NodeAddition  `json:"recommended_additions,omitempty"` // Specific nodes to add
	EstimatedCost        float64         `json:"estimated_cost_usd,omitempty"`    // Estimated hardware cost in USD
	SpaceDimensions      SpaceDimensions `json:"space_dimensions"`                // Space dimensions for reference
}

// SpaceDimensions describes the space dimensions
type SpaceDimensions struct {
	Width  float64 `json:"width_m"`
	Depth  float64 `json:"depth_m"`
	Height float64 `json:"height_m"`
	Area   float64 `json:"area_m2"`
	Volume float64 `json:"volume_m3"`
}

// NodeAddition represents a specific node to add with position and role
type NodeAddition struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Position    Point   `json:"position"`
	Role        string  `json:"role"`
	Height      string  `json:"height_description"`    // e.g., "ceiling", "wall", "desk"
	Improvement float64 `json:"estimated_improvement"` // 0-1, estimated coverage improvement
}

// GenerateShoppingListFromResults creates a shopping list from simulation results.
func GenerateShoppingListFromResults(space *Space, nodes *NodeSet, coverageScore float64, accuracy AccuracyReport) ShoppingList {
	nodeCount := nodes.Count()

	// Space dimensions
	minX, minY, _, maxX, maxY, maxZ := space.Bounds()
	width := maxX - minX
	depth := maxY - minY
	height := maxZ - minY
	area := width * depth
	volume := width * depth * height

	// Minimum nodes based on space dimensions
	minNodes := int(math.Ceil(area / 30.0)) // ~30 m² per node for fair coverage

	// Recommended nodes based on desired accuracy
	recNodes := minNodes
	if accuracy.MedianError > 1.0 && minNodes < 6 {
		recNodes = minNodes + 1
	}
	if accuracy.MedianError > 0.8 && minNodes < 8 {
		recNodes = minNodes + 2
	}

	// Expected accuracy
	expectedAccuracy := accuracy.MedianError
	if math.IsInf(expectedAccuracy, 0) {
		// Estimate from node count
		if nodeCount >= 6 {
			expectedAccuracy = 0.5
		} else if nodeCount >= 4 {
			expectedAccuracy = 1.0
		} else {
			expectedAccuracy = 1.5
		}
	}

	// Generate optimal positions (corner + mixed heights)
	optimalPositions := generateOptimalPositions(space, recNodes)

	// Find coverage gaps using GDOP analysis
	coverageGaps := findCoverageGaps(space, nodes)

	// Generate recommended additions
	recommendedAdditions := generateNodeAdditions(space, nodes, coverageGaps)

	// Hardware list with quantities
	hardware := make([]string, 0)
	hardware = append(hardware, fmt.Sprintf("%d × ESP32-S3 Development Board (with PSRAM 8MB)", recNodes))
	hardware = append(hardware, fmt.Sprintf("%d × USB-C Power Supply (5V 2A)", recNodes))
	hardware = append(hardware, fmt.Sprintf("%d × USB-C Cable (1-2m)", recNodes))
	hardware = append(hardware, fmt.Sprintf("%d × Adhesive Cable Clips (for mounting)", recNodes*4))
	hardware = append(hardware, fmt.Sprintf("%d × 3D Printed Case (optional)", recNodes))

	// Estimated cost (as of 2025)
	estimatedCost := float64(recNodes)*15.0 + // ESP32-S3 dev board
		float64(recNodes)*8.0 + // Power supply
		float64(recNodes)*3.0 + // USB cable
		float64(recNodes)*2.0 // Cable clips

	// Amazon search URL (non-affiliate)
	searchURL := fmt.Sprintf("https://www.amazon.com/s?k=esp32-s3+devkit+usb-c+psram")

	return ShoppingList{
		MinimumNodes:         minNodes,
		RecommendedNodes:     recNodes,
		ExpectedAccuracy:     expectedAccuracy,
		CoveragePercent:      coverageScore,
		HardwareList:         hardware,
		AmazonSearchURL:      searchURL,
		OptimalPositions:     optimalPositions,
		CoverageGaps:         coverageGaps,
		RecommendedAdditions: recommendedAdditions,
		EstimatedCost:        estimatedCost,
		SpaceDimensions: SpaceDimensions{
			Width:  width,
			Depth:  depth,
			Height: height,
			Area:   area,
			Volume: volume,
		},
	}
}

// generateOptimalPositions generates optimal node positions for a given count
func generateOptimalPositions(space *Space, count int) []Point {
	minX, minY, _, maxX, maxY, _ := space.Bounds()
	positions := make([]Point, 0, count)

	// Strategy: place nodes at corners and mid-points, with mixed heights
	corners := []Point{
		{X: minX + 0.5, Y: minY + 0.5, Z: 2.2},        // Low corner, high
		{X: maxX - 0.5, Y: minY + 0.5, Z: 2.2},        // Low corner, high
		{X: minX + 0.5, Y: maxY - 0.5, Z: 2.2},        // Low corner, high
		{X: maxX - 0.5, Y: maxY - 0.5, Z: 2.2},        // Low corner, high
		{X: (minX + maxX) / 2, Y: minY + 0.5, Z: 2.5}, // Mid wall, high
		{X: (minX + maxX) / 2, Y: maxY - 0.5, Z: 2.5}, // Mid wall, high
		{X: minX + 0.5, Y: (minY + maxY) / 2, Z: 0.3}, // Mid wall, low
		{X: maxX - 0.5, Y: (minY + maxY) / 2, Z: 0.3}, // Mid wall, low
	}

	for i := 0; i < count; i++ {
		if i < len(corners) {
			positions = append(positions, corners[i])
		} else {
			// Add random position for extra nodes
			positions = append(positions, Point{
				X: minX + mrand.Float64()*(maxX-minX),
				Y: minY + mrand.Float64()*(maxY-minY),
				Z: 0.3 + mrand.Float64()*2.0, // Mixed height
			})
		}
	}

	return positions
}

// findCoverageGaps finds positions with poor GDOP (coverage gaps)
func findCoverageGaps(space *Space, nodes *NodeSet) []Point {
	minX, minY, _, maxX, maxY, _ := space.Bounds()
	links := GenerateAllLinks(nodes)

	if len(links) < 2 {
		// No links means no coverage - return center of space as gap
		return []Point{{X: (minX + maxX) / 2, Y: (minY + maxY) / 2, Z: 1.0}}
	}

	gdopComp := NewGDOPComputer(links, GridConfig{
		MinX:     minX,
		MinY:     minY,
		Width:    maxX - minX,
		Depth:    maxY - minY,
		CellSize: 0.2,
	})

	results := gdopComp.ComputeAll()
	gaps := make([]Point, 0)

	// Find cells with poor or no coverage
	for _, row := range results {
		for _, cell := range row {
			if cell.Quality == "poor" || cell.Quality == "none" {
				gaps = append(gaps, Point{X: cell.X, Y: cell.Y, Z: cell.Z})
			}
		}
	}

	// Limit to top 10 worst coverage gaps
	if len(gaps) > 10 {
		gaps = gaps[:10]
	}

	return gaps
}

// generateNodeAdditions creates specific node addition recommendations
func generateNodeAdditions(space *Space, nodes *NodeSet, gaps []Point) []NodeAddition {
	additions := make([]NodeAddition, 0)
	minX, minY, _, maxX, maxY, _ := space.Bounds()

	// If we have coverage gaps, suggest adding nodes there
	for i, gap := range gaps {
		if i >= 3 {
			break // Limit to 3 gap-based additions
		}

		heightDesc := "ceiling"
		if gap.Z < 1.0 {
			heightDesc = "wall mount"
		} else if gap.Z > 2.0 {
			heightDesc = "ceiling"
		} else {
			heightDesc = "high wall"
		}

		additions = append(additions, NodeAddition{
			ID:          fmt.Sprintf("node-gap-%d", i+1),
			Name:        fmt.Sprintf("Gap Coverage Node %d", i+1),
			Position:    gap,
			Role:        "tx_rx",
			Height:      heightDesc,
			Improvement: 0.2 + float64(3-i)*0.05, // Later gaps have lower priority
		})
	}

	// Suggest corner nodes if we have few nodes
	if nodes.Count() < 4 {
		corners := CornerPositions(space)
		for i := nodes.Count(); i < 4 && i < len(corners); i++ {
			heightDesc := "ceiling"
			if corners[i].Z < 1.0 {
				heightDesc = "low"
			}

			additions = append(additions, NodeAddition{
				ID:          fmt.Sprintf("node-corner-%d", i+1),
				Name:        fmt.Sprintf("Corner Node %d", i+1),
				Position:    corners[i],
				Role:        "tx_rx",
				Height:      heightDesc,
				Improvement: 0.15,
			})
		}
	}

	// If no specific additions, suggest a center node
	if len(additions) == 0 {
		additions = append(additions, NodeAddition{
			ID:          "node-center-1",
			Name:        "Center Node",
			Position:    Point{X: (minX + maxX) / 2, Y: (minY + maxY) / 2, Z: 2.0},
			Role:        "tx_rx",
			Height:      "ceiling",
			Improvement: 0.1,
		})
	}

	return additions
}
