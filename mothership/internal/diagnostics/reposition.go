// Package diagnostics provides repositioning target computation for coverage optimization
package diagnostics

import (
	"math"
)

// RepositioningComputer computes optimal node repositioning targets
type RepositioningComputer struct {
	// GDOPCalculator computes geometric dilution of precision
	gdopCalc GDOPCalculator

	// Node position accessor
	getNodePosition func(mac string) (Vec3, bool)

	// All node positions
	getAllPositions func() map[string]Vec3

	// Room dimensions
	getRoomDimensions func() (width, depth, height float64, originX, originZ float64)
}

// GDOPCalculator interface for GDOP computation
type GDOPCalculator interface {
	GDOPMapWithVirtual(positions []NodePosition, virtualPos *NodePosition) ([]float32, int, int)
}

// NodePosition represents a node's position for GDOP calculation
type NodePosition struct {
	MAC string
	X   float64
	Z   float64
}

// NewRepositioningComputer creates a new repositioning computer
func NewRepositioningComputer() *RepositioningComputer {
	return &RepositioningComputer{}
}

// SetGDOPCalculator sets the GDOP calculator
func (rc *RepositioningComputer) SetGDOPCalculator(calc GDOPCalculator) {
	rc.gdopCalc = calc
}

// SetNodePositionAccessor sets the function to get node positions
func (rc *RepositioningComputer) SetNodePositionAccessor(fn func(mac string) (Vec3, bool)) {
	rc.getNodePosition = fn
}

// SetAllPositionsAccessor sets the function to get all node positions
func (rc *RepositioningComputer) SetAllPositionsAccessor(fn func() map[string]Vec3) {
	rc.getAllPositions = fn
}

// SetRoomDimensionsAccessor sets the function to get room dimensions
func (rc *RepositioningComputer) SetRoomDimensionsAccessor(fn func() (width, depth, height float64, originX, originZ float64)) {
	rc.getRoomDimensions = fn
}

// ComputeRepositioningTarget computes the optimal position to move a node
// to improve coverage in a blocked zone
func (rc *RepositioningComputer) ComputeRepositioningTarget(linkID string, blockedZone Vec3) (Vec3, float64, error) {
	if rc.gdopCalc == nil || rc.getAllPositions == nil || rc.getRoomDimensions == nil {
		return Vec3{}, 0, nil
	}

	// Get current positions
	allPositions := rc.getAllPositions()
	if len(allPositions) < 2 {
		return Vec3{}, 0, nil
	}

	// Identify which node to move (the RX node of the link)
	nodeMAC := extractNodeBMAC(linkID)
	if nodeMAC == "" {
		return Vec3{}, 0, nil
	}

	// Get current position of the node to move
	currentPos, ok := rc.getNodePosition(nodeMAC)
	if !ok {
		return Vec3{}, 0, nil
	}

	// Get room dimensions
	width, depth, _, originX, originZ := rc.getRoomDimensions()

	// Build list of fixed positions (all nodes except the one being moved)
	fixedPositions := make([]NodePosition, 0, len(allPositions)-1)
	for mac, pos := range allPositions {
		if mac != nodeMAC {
			fixedPositions = append(fixedPositions, NodePosition{
				MAC: mac,
				X:   pos.X,
				Z:   pos.Z,
			})
		}
	}

	// Get current worst GDOP
	currentWorst := rc.computeWorstGDOPNear(fixedPositions, nil, blockedZone, 2.0)

	// Grid search for optimal position
	bestPos := currentPos
	bestImprovement := 0.0

	step := 0.3 // 30cm steps
	minDist := 0.5 // Minimum distance from other nodes

	for x := originX + step; x < originX+width-step; x += step {
		for z := originZ + step; z < originZ+depth-step; z += step {
			candidatePos := Vec3{X: x, Z: z}

			// Check if too close to existing nodes
			tooClose := false
			for _, fixed := range fixedPositions {
				dist := math.Sqrt((fixed.X-x)*(fixed.X-x) + (fixed.Z-z)*(fixed.Z-z))
				if dist < minDist {
					tooClose = true
					break
				}
			}
			if tooClose {
				continue
			}

			// Check if position is within bounds
			if x < originX || x > originX+width || z < originZ || z > originZ+depth {
				continue
			}

			// Compute GDOP improvement near blocked zone
			virtualNode := &NodePosition{MAC: "virtual", X: x, Z: z}
			newWorst := rc.computeWorstGDOPNear(fixedPositions, virtualNode, blockedZone, 2.0)

			improvement := currentWorst - newWorst
			if improvement > bestImprovement {
				bestImprovement = improvement
				bestPos = candidatePos
			}
		}
	}

	return bestPos, bestImprovement, nil
}

// computeWorstGDOPNear computes the worst GDOP in a zone around a point
func (rc *RepositioningComputer) computeWorstGDOPNear(fixedPositions []NodePosition, virtualNode *NodePosition, center Vec3, radius float64) float64 {
	if rc.gdopCalc == nil {
		return 10.0 // Assume worst case
	}

	gdopMap, cols, rows := rc.gdopCalc.GDOPMapWithVirtual(fixedPositions, virtualNode)
	if len(gdopMap) == 0 || cols == 0 || rows == 0 {
		return 10.0
	}

	// Get room dimensions for coordinate conversion
	width, depth, _, originX, originZ := rc.getRoomDimensions()
	cellWidth := width / float64(cols)
	cellDepth := depth / float64(rows)

	// Find cells within radius of center
	var worstGDOP float64
	cellCount := 0

	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			// Convert cell to room coordinates
			cellX := originX + (float64(col) + 0.5) * cellWidth
			cellZ := originZ + (float64(row) + 0.5) * cellDepth

			// Check distance from center
			dist := math.Sqrt((cellX-center.X)*(cellX-center.X) + (cellZ-center.Z)*(cellZ-center.Z))
			if dist <= radius {
				gdop := float64(gdopMap[row*cols+col])
				if gdop > worstGDOP {
					worstGDOP = gdop
				}
				cellCount++
			}
		}
	}

	if cellCount == 0 {
		return 10.0
	}

	return worstGDOP
}

// SuggestRepositioningForLink suggests a repositioning target for a specific link
// This is a convenience method that wraps ComputeRepositioningTarget
func (rc *RepositioningComputer) SuggestRepositioningForLink(linkID string) (Vec3, float64, string, error) {
	// Get the midpoint of the link as the blocked zone heuristic
	if rc.getAllPositions == nil {
		return Vec3{}, 0, "", nil
	}

	nodeA := extractNodeAMAC(linkID)
	nodeB := extractNodeBMAC(linkID)

	allPositions := rc.getAllPositions()
	posA, okA := allPositions[nodeA]
	posB, okB := allPositions[nodeB]

	if !okA || !okB {
		return Vec3{}, 0, "", nil
	}

	// Use midpoint as the blocked zone (this is a heuristic)
	blockedZone := Vec3{
		X: (posA.X + posB.X) / 2,
		Z: (posA.Z + posB.Z) / 2,
	}

	target, improvement, err := rc.ComputeRepositioningTarget(linkID, blockedZone)
	if err != nil {
		return Vec3{}, 0, "", err
	}

	return target, improvement, nodeB, nil
}

// IsWithinBounds checks if a position is within room bounds
func IsWithinBounds(pos Vec3, width, depth, originX, originZ float64) bool {
	return pos.X >= originX && pos.X <= originX+width &&
		pos.Z >= originZ && pos.Z <= originZ+depth
}

// CalculateGDOPImprovement estimates the GDOP improvement from moving a node
func (rc *RepositioningComputer) CalculateGDOPImprovement(nodeMAC string, targetPos Vec3) float64 {
	if rc.gdopCalc == nil || rc.getAllPositions == nil {
		return 0
	}

	allPositions := rc.getAllPositions()
	if len(allPositions) < 2 {
		return 0
	}

	// Build list of positions with the node at its new position
	positions := make([]NodePosition, 0, len(allPositions))
	for mac, pos := range allPositions {
		if mac == nodeMAC {
			positions = append(positions, NodePosition{
				MAC: mac,
				X:   targetPos.X,
				Z:   targetPos.Z,
			})
		} else {
			positions = append(positions, NodePosition{
				MAC: mac,
				X:   pos.X,
				Z:   pos.Z,
			})
		}
	}

	// Compute new worst GDOP
	virtualNode := &NodePosition{MAC: nodeMAC, X: targetPos.X, Z: targetPos.Z}
	fixedPositions := make([]NodePosition, 0)
	for mac, pos := range allPositions {
		if mac != nodeMAC {
			fixedPositions = append(fixedPositions, NodePosition{MAC: mac, X: pos.X, Z: pos.Z})
		}
	}

	// Get current worst GDOP (without virtual node)
	gdopMap, _, _ := rc.gdopCalc.GDOPMapWithVirtual(fixedPositions, nil)
	var currentWorst float64
	for _, g := range gdopMap {
		if float64(g) > currentWorst {
			currentWorst = float64(g)
		}
	}

	// Get new worst GDOP (with virtual node at target)
	newGdopMap, _, _ := rc.gdopCalc.GDOPMapWithVirtual(fixedPositions, virtualNode)
	var newWorst float64
	for _, g := range newGdopMap {
		if float64(g) > newWorst {
			newWorst = float64(g)
		}
	}

	return currentWorst - newWorst
}
