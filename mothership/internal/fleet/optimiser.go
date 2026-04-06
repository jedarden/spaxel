// Package fleet implements self-healing fleet management with GDOP optimization
package fleet

import (
	"math"
	"sort"
	"time"
)

// Role constants
const (
	RoleTX     = "tx"
	RoleRX     = "rx"
	RoleTXRX   = "tx_rx"
	RolePassive = "passive"
)

// NodeCapabilities describes what a node can do
type NodeCapabilities struct {
	CanTX        bool
	CanRX        bool
	HardwareType string
}

// NodeInfo combines position and capabilities for optimisation
type NodeInfo struct {
	MAC          string
	PosX         float64
	PosY         float64
	PosZ         float64
	HealthScore  float64 // 0-1, from ambient confidence
	Capabilities NodeCapabilities
}

// RoleAssignment is the output of the optimiser
type RoleAssignment struct {
	MAC  string
	Role string
}

// SensingLink represents a TX-RX pair for sensing
type SensingLink struct {
	TXMAC string
	RXMAC string
	Angle float64 // Angle of the TX-RX axis in radians
}

// OptimisationConfig holds configuration for the role optimiser
type OptimisationConfig struct {
	// Minimum health score for a node to be considered for active role (0-1)
	MinHealthScore float64
	// Maximum sensing range in metres
	MaxSensingRange float64
	// UseGDOP when true, use GDOP calculator if available
	UseGDOP bool
}

// DefaultOptimisationConfig returns sensible defaults
func DefaultOptimisationConfig() OptimisationConfig {
	return OptimisationConfig{
		MinHealthScore:  0.3,
		MaxSensingRange: 15.0,
		UseGDOP:         true,
	}
}

// RoleOptimiser computes optimal role assignments to maximise coverage
type RoleOptimiser struct {
	config       OptimisationConfig
	gdopCalc     GDOPCalculator
	roomConfig   RoomConfig
}

// NewRoleOptimiser creates a new role optimiser
func NewRoleOptimiser(config OptimisationConfig) *RoleOptimiser {
	return &RoleOptimiser{
		config: config,
	}
}

// SetGDOPCalculator sets the GDOP calculator for coverage-based optimisation
func (ro *RoleOptimiser) SetGDOPCalculator(calc GDOPCalculator) {
	ro.gdopCalc = calc
}

// SetRoomConfig sets the room configuration for GDOP calculations
func (ro *RoleOptimiser) SetRoomConfig(room RoomConfig) {
	ro.roomConfig = room
}

// OptimiseResult contains the result of a role optimisation
type OptimiseResult struct {
	Assignments    []RoleAssignment
	Links          []SensingLink
	MeanGDOP       float64
	CoverageScore  float64
	OptimisedAt    time.Time
	TriggerReason  string
	GDOPBefore     []float32 // GDOP map before (if available)
	GDOPAfter      []float32 // GDOP map after
	GDOPCols       int
	GDOPRows       int
}

// Optimise computes the optimal role assignment for the given nodes
func (ro *RoleOptimiser) Optimise(nodes []NodeInfo, triggerReason string) *OptimiseResult {
	result := &OptimiseResult{
		OptimisedAt:   time.Now(),
		TriggerReason: triggerReason,
	}

	if len(nodes) == 0 {
		return result
	}

	// Filter nodes by health score
	healthyNodes := make([]NodeInfo, 0, len(nodes))
	for _, n := range nodes {
		if n.HealthScore >= ro.config.MinHealthScore {
			healthyNodes = append(healthyNodes, n)
		}
	}

	if len(healthyNodes) == 0 {
		// Fall back to using all nodes if none are healthy enough
		healthyNodes = nodes
	}

	// Sort by MAC for deterministic results
	sort.Slice(healthyNodes, func(i, j int) bool {
		return healthyNodes[i].MAC < healthyNodes[j].MAC
	})

	n := len(healthyNodes)

	// Special cases
	switch {
	case n == 1:
		// Single node operates as TX-RX
		result.Assignments = []RoleAssignment{
			{MAC: healthyNodes[0].MAC, Role: RoleTXRX},
		}
		return result

	case n == 2:
		// Two nodes: one TX, one RX
		// Choose the one with better health as TX
		if healthyNodes[0].HealthScore >= healthyNodes[1].HealthScore {
			result.Assignments = []RoleAssignment{
				{MAC: healthyNodes[0].MAC, Role: RoleTX},
				{MAC: healthyNodes[1].MAC, Role: RoleRX},
			}
		} else {
			result.Assignments = []RoleAssignment{
				{MAC: healthyNodes[1].MAC, Role: RoleTX},
				{MAC: healthyNodes[0].MAC, Role: RoleRX},
			}
		}
		result.Links = []SensingLink{{
			TXMAC: result.Assignments[0].MAC,
			RXMAC: result.Assignments[1].MAC,
			Angle: ro.linkAngle(healthyNodes[0], healthyNodes[1]),
		}}
		return result
	}

	// General case: use GDOP or angular separation optimisation
	if ro.config.UseGDOP && ro.gdopCalc != nil {
		result = ro.optimiseByGDOP(healthyNodes, triggerReason)
	} else {
		result = ro.optimiseByAngularSeparation(healthyNodes, triggerReason)
	}

	return result
}

// optimiseByGDOP finds the TX/RX assignment that minimises worst-case GDOP
func (ro *RoleOptimiser) optimiseByGDOP(nodes []NodeInfo, triggerReason string) *OptimiseResult {
	n := len(nodes)
	targetTX := n / 2
	if targetTX < 1 {
		targetTX = 1
	}

	// Get node positions for GDOP calculation
	positions := make([]NodePosition, n)
	for i, node := range nodes {
		positions[i] = NodePosition{
			MAC: node.MAC,
			X:   node.PosX,
			Z:   node.PosZ,
		}
	}

	bestAssignments := make([]RoleAssignment, n)
	bestTXNodes := make([]int, 0)
	bestWorstGDOP := math.MaxFloat64
	bestGDOPMap := []float32(nil)

	// For small n, enumerate all combinations
	if n <= 10 {
		combinations := generateCombinations(n, targetTX)
		for _, txIndices := range combinations {
			// Evaluate this TX assignment
			txPositions := make([]NodePosition, 0, targetTX)
			for _, idx := range txIndices {
				txPositions = append(txPositions, positions[idx])
			}

			gdopMap, cols, rows := ro.gdopCalc.GDOPMap(txPositions)
			if len(gdopMap) == 0 {
				continue
			}

			// Compute worst-case GDOP
			var worstGDOP float64
			for _, gdop := range gdopMap {
				if float64(gdop) > worstGDOP {
					worstGDOP = float64(gdop)
				}
			}

			if worstGDOP < bestWorstGDOP {
				bestWorstGDOP = worstGDOP
				bestTXNodes = make([]int, len(txIndices))
				copy(bestTXNodes, txIndices)
				bestGDOPMap = gdopMap
				// Store dimensions for result
				_ = cols
				_ = rows
			}
		}
	} else {
		// Greedy approach for larger fleets
		selectedIndices := make([]int, 0, targetTX)
		remainingIndices := make([]int, n)
		for i := 0; i < n; i++ {
			remainingIndices[i] = i
		}

		for len(selectedIndices) < targetTX {
			bestIdx := -1
			bestGDOP := math.MaxFloat64

			for _, idx := range remainingIndices {
				testIndices := append(selectedIndices, idx)
				txPositions := make([]NodePosition, 0, len(testIndices))
				for _, i := range testIndices {
					txPositions = append(txPositions, positions[i])
				}

				gdopMap, _, _ := ro.gdopCalc.GDOPMap(txPositions)
				if len(gdopMap) == 0 {
					continue
				}

				var worstGDOP float64
				for _, gdop := range gdopMap {
					if float64(gdop) > worstGDOP {
						worstGDOP = float64(gdop)
					}
				}

				if worstGDOP < bestGDOP {
					bestGDOP = worstGDOP
					bestIdx = idx
					bestGDOPMap = gdopMap
				}
			}

			if bestIdx >= 0 {
				selectedIndices = append(selectedIndices, bestIdx)
				// Remove from remaining
				for i, idx := range remainingIndices {
					if idx == bestIdx {
						remainingIndices = append(remainingIndices[:i], remainingIndices[i+1:]...)
						break
					}
				}
			} else {
				break
			}
		}
		bestTXNodes = selectedIndices
	}

	// Build role assignments
	txSet := make(map[string]struct{})
	for _, idx := range bestTXNodes {
		txSet[nodes[idx].MAC] = struct{}{}
	}

	for i, node := range nodes {
		role := RoleRX
		if _, isTX := txSet[node.MAC]; isTX {
			role = RoleTX
		}
		// Check if node should be passive (co-located with another node)
		if ro.shouldBePassive(node, nodes, txSet) {
			role = RolePassive
		}
		bestAssignments[i] = RoleAssignment{MAC: node.MAC, Role: role}
	}

	// Compute mean GDOP for result
	var sumGDOP float64
	var count int
	for _, gdop := range bestGDOPMap {
		sumGDOP += float64(gdop)
		count++
	}
	meanGDOP := 0.0
	if count > 0 {
		meanGDOP = sumGDOP / float64(count)
	}

	// Build sensing links
	links := ro.buildSensingLinks(nodes, txSet)

	return &OptimiseResult{
		Assignments:   bestAssignments,
		Links:         links,
		MeanGDOP:      meanGDOP,
		CoverageScore: ro.computeCoverageScore(bestGDOPMap),
		OptimisedAt:   time.Now(),
		TriggerReason: triggerReason,
		GDOPAfter:     bestGDOPMap,
	}
}

// optimiseByAngularSeparation finds TX/RX assignment maximising angular separation
func (ro *RoleOptimiser) optimiseByAngularSeparation(nodes []NodeInfo, triggerReason string) *OptimiseResult {
	n := len(nodes)
	targetTX := n / 2
	if targetTX < 1 {
		targetTX = 1
	}

	// Build list of candidate TX-RX pairs with their angular scores
	type linkScore struct {
		txIdx, rxIdx int
		score        float64
		angle        float64
	}

	var candidates []linkScore

	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if i == j {
				continue
			}
			// Check if within sensing range
			dist := ro.nodeDistance(nodes[i], nodes[j])
			if dist > ro.config.MaxSensingRange {
				continue
			}

			angle := ro.linkAngle(nodes[i], nodes[j])
			// Initial score based on health and distance
			score := nodes[i].HealthScore * nodes[j].HealthScore / (dist + 1)
			candidates = append(candidates, linkScore{
				txIdx: i,
				rxIdx: j,
				score: score,
				angle: angle,
			})
		}
	}

	// Sort candidates by score
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	// Greedy selection: pick links that maximise angular separation
	selectedLinks := make([]linkScore, 0, targetTX)
	usedAsTX := make(map[int]struct{})
	usedAsRX := make(map[int]struct{})

	for _, c := range candidates {
		if _, used := usedAsTX[c.txIdx]; used {
			continue
		}
		if _, used := usedAsRX[c.rxIdx]; used {
			continue
		}

		// Compute angular separation score
		angularScore := c.score
		for _, sl := range selectedLinks {
			sep := angularSeparation(c.angle, sl.angle)
			// Penalise near-parallel links (low separation)
			if sep < math.Pi/4 { // Less than 45 degrees
				angularScore *= 0.5
			}
		}

		// Accept this link if score is still good
		if angularScore > 0.1 {
			selectedLinks = append(selectedLinks, c)
			usedAsTX[c.txIdx] = struct{}{}
			usedAsRX[c.rxIdx] = struct{}{}
		}

		if len(selectedLinks) >= targetTX {
			break
		}
	}

	// Build role assignments
	txSet := make(map[string]struct{})
	assignments := make([]RoleAssignment, n)
	assigned := make(map[string]string)

	for _, link := range selectedLinks {
		txSet[nodes[link.txIdx].MAC] = struct{}{}
		assigned[nodes[link.txIdx].MAC] = RoleTX
		assigned[nodes[link.rxIdx].MAC] = RoleRX
	}

	for i, node := range nodes {
		if role, ok := assigned[node.MAC]; ok {
			assignments[i] = RoleAssignment{MAC: node.MAC, Role: role}
		} else {
			// Unassigned node becomes passive or RX based on capabilities
			if _, isTX := txSet[node.MAC]; isTX {
				assignments[i] = RoleAssignment{MAC: node.MAC, Role: RoleTX}
			} else if len(txSet) < targetTX && node.Capabilities.CanTX {
				txSet[node.MAC] = struct{}{}
				assignments[i] = RoleAssignment{MAC: node.MAC, Role: RoleTX}
			} else {
				assignments[i] = RoleAssignment{MAC: node.MAC, Role: RoleRX}
			}
		}
	}

	// Build sensing links
	links := make([]SensingLink, len(selectedLinks))
	for i, sl := range selectedLinks {
		links[i] = SensingLink{
			TXMAC: nodes[sl.txIdx].MAC,
			RXMAC: nodes[sl.rxIdx].MAC,
			Angle: sl.angle,
		}
	}

	return &OptimiseResult{
		Assignments:   assignments,
		Links:         links,
		OptimisedAt:   time.Now(),
		TriggerReason: triggerReason,
	}
}

// linkAngle computes the angle of the link from TX to RX
func (ro *RoleOptimiser) linkAngle(tx, rx NodeInfo) float64 {
	dx := rx.PosX - tx.PosX
	dz := rx.PosZ - tx.PosZ
	return math.Atan2(dz, dx)
}

// nodeDistance computes the 3D distance between two nodes
func (ro *RoleOptimiser) nodeDistance(a, b NodeInfo) float64 {
	dx := a.PosX - b.PosX
	dy := a.PosY - b.PosY
	dz := a.PosZ - b.PosZ
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// angularSeparation computes the smaller angle between two link axes
func angularSeparation(a, b float64) float64 {
	diff := math.Abs(a - b)
	if diff > math.Pi {
		diff = 2*math.Pi - diff
	}
	return diff
}

// shouldBePassive checks if a node should be assigned passive role
// (e.g., co-located with another node)
func (ro *RoleOptimiser) shouldBePassive(node NodeInfo, allNodes []NodeInfo, txSet map[string]struct{}) bool {
	const colocateThreshold = 0.5 // metres

	for _, other := range allNodes {
		if other.MAC == node.MAC {
			continue
		}
		dist := ro.nodeDistance(node, other)
		if dist < colocateThreshold {
			// Co-located with another node
			// If the other node is TX, this one should be passive
			if _, isTX := txSet[other.MAC]; isTX {
				return true
			}
		}
	}
	return false
}

// computeCoverageScore computes a 0-1 coverage score from a GDOP map
func (ro *RoleOptimiser) computeCoverageScore(gdopMap []float32) float64 {
	if len(gdopMap) == 0 {
		return 0
	}

	var below2, below5 int
	var sumGDOP float64
	var worstGDOP float64

	for _, gdop := range gdopMap {
		g := float64(gdop)
		sumGDOP += g
		if g > worstGDOP {
			worstGDOP = g
		}
		if g < 2 {
			below2++
		}
		if g < 5 {
			below5++
		}
	}

	n := float64(len(gdopMap))
	pctBelow2 := float64(below2) / n
	pctBelow5 := float64(below5) / n
	worstPenalty := math.Min(worstGDOP/10, 1.0)

	// Weighted combination:
	// - 50% weight to GDOP < 2 percentage
	// - 30% weight to GDOP < 5 percentage
	// - 20% penalty for worst GDOP (capped at 10)
	return 0.5*pctBelow2 + 0.3*pctBelow5 + 0.2*(1-worstPenalty)
}

// buildSensingLinks creates a list of sensing links from TX nodes to all RX nodes
func (ro *RoleOptimiser) buildSensingLinks(nodes []NodeInfo, txSet map[string]struct{}) []SensingLink {
	var links []SensingLink

	for _, tx := range nodes {
		if _, isTX := txSet[tx.MAC]; !isTX {
			continue
		}
		for _, rx := range nodes {
			if _, isTX := txSet[rx.MAC]; isTX {
				continue // Skip TX-TX links
			}
			if tx.MAC == rx.MAC {
				continue
			}
			dist := ro.nodeDistance(tx, rx)
			if dist <= ro.config.MaxSensingRange {
				links = append(links, SensingLink{
					TXMAC: tx.MAC,
					RXMAC: rx.MAC,
					Angle: ro.linkAngle(tx, rx),
				})
			}
		}
	}

	return links
}

// SimulateRemoval predicts coverage impact if a node is removed
func (ro *RoleOptimiser) SimulateRemoval(nodes []NodeInfo, removeMAC string) (*OptimiseResult, float64) {
	// Get current coverage
	currentResult := ro.Optimise(nodes, "current_state")
	currentScore := currentResult.CoverageScore

	// Remove the node
	remaining := make([]NodeInfo, 0, len(nodes)-1)
	for _, n := range nodes {
		if n.MAC != removeMAC {
			remaining = append(remaining, n)
		}
	}

	// Get coverage after removal
	newResult := ro.Optimise(remaining, "simulated_removal")
	newScore := newResult.CoverageScore

	// Coverage delta (negative means degradation)
	coverageDelta := newScore - currentScore

	return newResult, coverageDelta
}
