// Package fleet implements self-healing fleet management with GDOP optimization
package fleet

import (
	"context"
	"log"
	"math"
	"sort"
	"sync"
	"time"
)

// GDOPCalculator computes geometric dilution of precision
type GDOPCalculator interface {
	GDOPMap(positions []NodePosition) ([]float32, int, int)
}

// NodePosition represents a node's position for GDOP calculation
type NodePosition struct {
	MAC string
	X   float64
	Z   float64
}

// CoverageResult holds the result of a coverage analysis
type CoverageResult struct {
	Timestamp       time.Time
	ActiveNodes     int
	TotalNodes      int
	MeanGDOP        float64
	WorstGDOP       float64
	GDOPBelow2Pct   float64 // Percentage of room with GDOP < 2
	GDOPBelow5Pct   float64 // Percentage of room with GDOP < 5
	CoverageScore   float64 // 0-1 composite score
	RoleAssignments map[string]string
}

// FleetHealer manages self-healing operations with GDOP optimization
type FleetHealer struct {
	mu sync.RWMutex

	registry      *Registry
	gdopCalc      GDOPCalculator
	notifier      NodeStateNotifier
	bcaster       RegistryBroadcaster

	// State tracking
	online        map[string]struct{}
	nodePositions map[string]NodePosition

	// Coverage history for before/after comparison
	lastCoverage   *CoverageResult
	coverageHistory []*CoverageResult
	maxHistorySize int

	// Healing configuration
	healInterval    time.Duration
	minOnlineNodes  int // Minimum nodes before degraded mode
	degradedMode    bool

	// Role optimization
	optimalRoles    map[string]string
	txNodes         []string
}

// FleetHealerConfig holds configuration for FleetHealer
type FleetHealerConfig struct {
	HealInterval   time.Duration
	MinOnlineNodes int
	MaxHistorySize int
}

// NewFleetHealer creates a new self-healing fleet manager
func NewFleetHealer(registry *Registry, cfg FleetHealerConfig) *FleetHealer {
	if cfg.HealInterval == 0 {
		cfg.HealInterval = 60 * time.Second
	}
	if cfg.MinOnlineNodes == 0 {
		cfg.MinOnlineNodes = 2
	}
	if cfg.MaxHistorySize == 0 {
		cfg.MaxHistorySize = 100
	}

	return &FleetHealer{
		registry:        registry,
		online:          make(map[string]struct{}),
		nodePositions:   make(map[string]NodePosition),
		optimalRoles:    make(map[string]string),
		healInterval:    cfg.HealInterval,
		minOnlineNodes:  cfg.MinOnlineNodes,
		maxHistorySize:  cfg.MaxHistorySize,
		coverageHistory: make([]*CoverageResult, 0, cfg.MaxHistorySize),
	}
}

// SetGDOPCalculator sets the GDOP calculator (typically the localization engine)
func (fh *FleetHealer) SetGDOPCalculator(calc GDOPCalculator) {
	fh.mu.Lock()
	fh.gdopCalc = calc
	fh.mu.Unlock()
}

// SetNotifier sets the node state notifier
func (fh *FleetHealer) SetNotifier(n NodeStateNotifier) {
	fh.mu.Lock()
	fh.notifier = n
	fh.mu.Unlock()
}

// SetBroadcaster sets the registry broadcaster
func (fh *FleetHealer) SetBroadcaster(b RegistryBroadcaster) {
	fh.mu.Lock()
	fh.bcaster = b
	fh.mu.Unlock()
}

// OnNodeConnected handles a node connection event
func (fh *FleetHealer) OnNodeConnected(mac, firmware, chip string) {
	fh.mu.Lock()
	fh.online[mac] = struct{}{}
	wasDegraded := fh.degradedMode
	fh.mu.Unlock()

	// Get node position from registry
	if node, err := fh.registry.GetNode(mac); err == nil {
		fh.nodePositions[mac] = NodePosition{
			MAC: mac,
			X:   node.PosX,
			Z:   node.PosZ,
		}
	}

	// Re-optimize roles
	fh.optimizeRoles()

	// Check if we recovered from degraded mode
	fh.mu.RLock()
	nowDegraded := fh.degradedMode
	fh.mu.RUnlock()

	if wasDegraded && !nowDegraded {
		log.Printf("[INFO] fleet: recovered from degraded mode, now have %d nodes online", len(fh.online))
		fh.broadcastCoverage("Node %s reconnected, system recovered from degraded mode", mac)
	}
}

// OnNodeDisconnected handles a node disconnection event
func (fh *FleetHealer) OnNodeDisconnected(mac string) {
	// Get previous coverage for comparison
	beforeCoverage := fh.computeCoverage()

	fh.mu.Lock()
	delete(fh.online, mac)
	delete(fh.nodePositions, mac)
	onlineCount := len(fh.online)
	totalNodes := onlineCount + 1 // Include the one that just left
	fh.mu.Unlock()

	// Check for degraded mode
	if onlineCount < fh.minOnlineNodes {
		fh.mu.Lock()
		if !fh.degradedMode {
			fh.degradedMode = true
			log.Printf("[WARN] fleet: entering degraded mode - only %d/%d nodes online", onlineCount, totalNodes)
		}
		fh.mu.Unlock()
	}

	// Re-optimize roles with remaining nodes
	fh.optimizeRoles()

	// Compute new coverage and compare
	afterCoverage := fh.computeCoverage()

	// Broadcast degradation warning if significant
	if beforeCoverage != nil && afterCoverage != nil {
		coverageDelta := afterCoverage.CoverageScore - beforeCoverage.CoverageScore
		if coverageDelta < -0.1 {
			fh.broadcastCoverage("Node %s disconnected, coverage degraded by %.0f%%", mac, -coverageDelta*100)
		}
	}
}

// Run starts the periodic self-healing loop
func (fh *FleetHealer) Run(ctx context.Context) {
	ticker := time.NewTicker(fh.healInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fh.selfHeal()
		}
	}
}

// selfHeal performs periodic health checks and role optimization
func (fh *FleetHealer) selfHeal() {
	fh.mu.RLock()
	onlineMacs := make([]string, 0, len(fh.online))
	for mac := range fh.online {
		onlineMacs = append(onlineMacs, mac)
	}
	notifier := fh.notifier
	fh.mu.RUnlock()

	if notifier == nil {
		return
	}

	// Re-push optimal roles to all online nodes
	for _, mac := range onlineMacs {
		fh.mu.RLock()
		role, exists := fh.optimalRoles[mac]
		fh.mu.RUnlock()

		if exists {
			notifier.SendRoleToMAC(mac, role, "")
		}
	}
}

// optimizeRoles finds the optimal TX/RX assignment to minimize worst-case GDOP
func (fh *FleetHealer) optimizeRoles() {
	fh.mu.Lock()
	defer fh.mu.Unlock()

	if len(fh.online) == 0 {
		fh.optimalRoles = make(map[string]string)
		fh.txNodes = nil
		return
	}

	onlineList := make([]string, 0, len(fh.online))
	for mac := range fh.online {
		onlineList = append(onlineList, mac)
	}
	sort.Strings(onlineList)

	n := len(onlineList)

	// Special cases
	switch {
	case n == 1:
		fh.optimalRoles = map[string]string{onlineList[0]: "tx_rx"}
		fh.txNodes = []string{onlineList[0]}
		fh.degradedMode = false
		return

	case n == 2:
		// One TX, one RX
		fh.optimalRoles = map[string]string{
			onlineList[0]: "tx",
			onlineList[1]: "rx",
		}
		fh.txNodes = []string{onlineList[0]}
		fh.degradedMode = false
		return
	}

	// General case: optimize TX assignment using GDOP
	targetTX := n / 2
	if targetTX < 1 {
		targetTX = 1
	}

	// Get positions for GDOP calculation
	positions := make([]NodePosition, 0, n)
	for _, mac := range onlineList {
		if pos, ok := fh.nodePositions[mac]; ok {
			positions = append(positions, pos)
		}
	}

	// If we have a GDOP calculator and enough positions, optimize
	if fh.gdopCalc != nil && len(positions) >= n-1 {
		fh.optimalRoles, fh.txNodes = fh.optimizeRolesByGDOP(onlineList, positions, targetTX)
	} else {
		// Fallback: simple alternating assignment
		fh.optimalRoles, fh.txNodes = fh.simpleRoleAssignment(onlineList, targetTX)
	}

	fh.degradedMode = n < fh.minOnlineNodes

	// Apply roles to registry
	for mac, role := range fh.optimalRoles {
		_ = fh.registry.SetNodeRole(mac, role)
	}
}

// optimizeRolesByGDOP finds TX/RX assignment that minimizes worst-case GDOP
func (fh *FleetHealer) optimizeRolesByGDOP(nodes []string, positions []NodePosition, targetTX int) (map[string]string, []string) {
	n := len(nodes)
	bestRoles := make(map[string]string)
	bestTXNodes := make([]string, 0)
	bestWorstGDOP := math.MaxFloat64

	// Generate all combinations of targetTX nodes from n total
	// For small n, we can enumerate all combinations
	// For larger n, use greedy approach

	if n <= 10 {
		// Enumerate all combinations
		combinations := generateCombinations(n, targetTX)
		for _, txIndices := range combinations {
			// Evaluate this TX assignment
			txPositions := make([]NodePosition, 0, targetTX)
			for _, idx := range txIndices {
				txPositions = append(txPositions, positions[idx])
			}

			if fh.gdopCalc == nil {
				continue
			}

			gdopMap, _, _ := fh.gdopCalc.GDOPMap(txPositions)
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
				bestTXNodes = make([]string, 0, targetTX)
				for _, idx := range txIndices {
					bestTXNodes = append(bestTXNodes, nodes[idx])
				}
			}
		}
	} else {
		// Greedy approach: add TX nodes one by one to minimize GDOP
		selectedIndices := make([]int, 0, targetTX)
		remainingIndices := make([]int, n)
		for i := 0; i < n; i++ {
			remainingIndices[i] = i
		}

		for len(selectedIndices) < targetTX {
			bestIdx := -1
			bestGDOP := math.MaxFloat64

			for _, idx := range remainingIndices {
				// Try adding this index
				testIndices := append(selectedIndices, idx)
				txPositions := make([]NodePosition, 0, len(testIndices))
				for _, i := range testIndices {
					txPositions = append(txPositions, positions[i])
				}

				if fh.gdopCalc == nil {
					continue
				}

				gdopMap, _, _ := fh.gdopCalc.GDOPMap(txPositions)
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

		bestTXNodes = make([]string, 0, len(selectedIndices))
		for _, idx := range selectedIndices {
			bestTXNodes = append(bestTXNodes, nodes[idx])
		}
	}

	// Build role map
	txSet := make(map[string]struct{})
	for _, mac := range bestTXNodes {
		txSet[mac] = struct{}{}
	}

	for _, mac := range nodes {
		if _, isTX := txSet[mac]; isTX {
			bestRoles[mac] = "tx"
		} else {
			bestRoles[mac] = "rx"
		}
	}

	return bestRoles, bestTXNodes
}

// simpleRoleAssignment assigns TX roles to first N nodes
func (fh *FleetHealer) simpleRoleAssignment(nodes []string, targetTX int) (map[string]string, []string) {
	roles := make(map[string]string)
	txNodes := make([]string, 0, targetTX)

	for i, mac := range nodes {
		if i < targetTX {
			roles[mac] = "tx"
			txNodes = append(txNodes, mac)
		} else {
			roles[mac] = "rx"
		}
	}

	return roles, txNodes
}

// generateCombinations generates all C(n, k) combinations
func generateCombinations(n, k int) [][]int {
	result := make([][]int, 0)
	combination := make([]int, k)

	var generate func(int, int)
	generate = func(start, idx int) {
		if idx == k {
			comb := make([]int, k)
			copy(comb, combination)
			result = append(result, comb)
			return
		}

		for i := start; i < n; i++ {
			combination[idx] = i
			generate(i+1, idx+1)
		}
	}

	generate(0, 0)
	return result
}

// computeCoverage calculates current coverage metrics
func (fh *FleetHealer) computeCoverage() *CoverageResult {
	fh.mu.RLock()
	defer fh.mu.RUnlock()

	result := &CoverageResult{
		Timestamp:       time.Now(),
		ActiveNodes:     len(fh.online),
		RoleAssignments: make(map[string]string),
	}

	// Get total nodes from registry
	nodes, err := fh.registry.GetAllNodes()
	if err == nil {
		result.TotalNodes = len(nodes)
	}

	// Copy role assignments
	for mac, role := range fh.optimalRoles {
		result.RoleAssignments[mac] = role
	}

	// Compute GDOP metrics if calculator available
	if fh.gdopCalc != nil && len(fh.nodePositions) > 0 {
		positions := make([]NodePosition, 0, len(fh.nodePositions))
		for _, pos := range fh.nodePositions {
			positions = append(positions, pos)
		}

		gdopMap, cols, rows := fh.gdopCalc.GDOPMap(positions)
		if len(gdopMap) > 0 {
			var sumGDOP float64
			var worstGDOP float64
			var below2, below5 int

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

			nCells := float64(cols * rows)
			result.MeanGDOP = sumGDOP / nCells
			result.WorstGDOP = worstGDOP
			result.GDOPBelow2Pct = float64(below2) / nCells
			result.GDOPBelow5Pct = float64(below5) / nCells

			// Coverage score: weighted combination
			// - 50% weight to GDOP < 2 percentage
			// - 30% weight to GDOP < 5 percentage
			// - 20% penalty for worst GDOP (capped at 10)
			worstPenalty := math.Min(worstGDOP/10, 1.0)
			result.CoverageScore = 0.5*result.GDOPBelow2Pct + 0.3*result.GDOPBelow5Pct + 0.2*(1-worstPenalty)
		}
	}

	// Store in history
	fh.coverageHistory = append(fh.coverageHistory, result)
	if len(fh.coverageHistory) > fh.maxHistorySize {
		fh.coverageHistory = fh.coverageHistory[1:]
	}
	fh.lastCoverage = result

	return result
}

// GetCoverage returns current coverage metrics
func (fh *FleetHealer) GetCoverage() *CoverageResult {
	fh.mu.RLock()
	defer fh.mu.RUnlock()
	if fh.lastCoverage == nil {
		return fh.computeCoverage()
	}
	return fh.lastCoverage
}

// GetCoverageHistory returns recent coverage history
func (fh *FleetHealer) GetCoverageHistory(limit int) []*CoverageResult {
	fh.mu.RLock()
	defer fh.mu.RUnlock()

	if limit <= 0 || limit > len(fh.coverageHistory) {
		limit = len(fh.coverageHistory)
	}

	// Return last N entries
	start := len(fh.coverageHistory) - limit
	if start < 0 {
		start = 0
	}

	result := make([]*CoverageResult, len(fh.coverageHistory[start:]))
	copy(result, fh.coverageHistory[start:])
	return result
}

// IsDegraded returns whether the system is in degraded mode
func (fh *FleetHealer) IsDegraded() bool {
	fh.mu.RLock()
	defer fh.mu.RUnlock()
	return fh.degradedMode
}

// GetOnlineNodes returns the list of currently online nodes
func (fh *FleetHealer) GetOnlineNodes() []string {
	fh.mu.RLock()
	defer fh.mu.RUnlock()

	nodes := make([]string, 0, len(fh.online))
	for mac := range fh.online {
		nodes = append(nodes, mac)
	}
	return nodes
}

// GetOptimalRoles returns the current optimal role assignments
func (fh *FleetHealer) GetOptimalRoles() map[string]string {
	fh.mu.RLock()
	defer fh.mu.RUnlock()

	roles := make(map[string]string, len(fh.optimalRoles))
	for k, v := range fh.optimalRoles {
		roles[k] = v
	}
	return roles
}

// UpdateNodePosition updates a node's position for GDOP calculation
func (fh *FleetHealer) UpdateNodePosition(mac string, x, z float64) {
	fh.mu.Lock()
	defer fh.mu.Unlock()

	fh.nodePositions[mac] = NodePosition{MAC: mac, X: x, Z: z}
}

// broadcastCoverage broadcasts a coverage update message
func (fh *FleetHealer) broadcastCoverage(format string, args ...interface{}) {
	if fh.bcaster != nil {
		nodes, _ := fh.registry.GetAllNodes()
		room, _ := fh.registry.GetRoom()
		if room != nil {
			fh.bcaster.BroadcastRegistryState(nodes, *room)
		}
	}
}

// GetWorstCoverageZone returns the position with the worst GDOP
func (fh *FleetHealer) GetWorstCoverageZone() (x, z, gdop float64) {
	fh.mu.RLock()
	defer fh.mu.RUnlock()

	if fh.gdopCalc == nil || len(fh.nodePositions) < 2 {
		return 0, 0, 10
	}

	positions := make([]NodePosition, 0, len(fh.nodePositions))
	for _, pos := range fh.nodePositions {
		positions = append(positions, pos)
	}

	gdopMap, cols, rows := fh.gdopCalc.GDOPMap(positions)
	if len(gdopMap) == 0 {
		return 0, 0, 10
	}

	// Find worst cell
	var worstGDOP float64
	var worstRow, worstCol int

	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			g := float64(gdopMap[row*cols+col])
			if g > worstGDOP {
				worstGDOP = g
				worstRow = row
				worstCol = col
			}
		}
	}

	// Convert to room coordinates (assuming 0.2m cell size)
	room, _ := fh.registry.GetRoom()
	cellSize := 0.2
	if room != nil {
		x = room.OriginX + (float64(worstCol) + 0.5) * cellSize
		z = room.OriginZ + (float64(worstRow) + 0.5) * cellSize
	}

	return x, z, worstGDOP
}

// SuggestNodePosition returns an optimal position for a new node
func (fh *FleetHealer) SuggestNodePosition() (x, z float64, improvement float64) {
	fh.mu.RLock()
	defer fh.mu.RUnlock()

	if fh.gdopCalc == nil || len(fh.nodePositions) < 1 {
		return 0, 0, 0
	}

	room, _ := fh.registry.GetRoom()
	if room == nil {
		return 0, 0, 0
	}

	// Grid search over room
	bestX, bestZ := 0.0, 0.0
	bestImprovement := 0.0

	step := 0.5 // 50cm steps
	currentPositions := make([]NodePosition, 0, len(fh.nodePositions))
	for _, pos := range fh.nodePositions {
		currentPositions = append(currentPositions, pos)
	}

	// Get current worst GDOP
	_, _, currentWorst := fh.GetWorstCoverageZone()

	// Try adding a virtual node at each position
	for x = room.OriginX + step; x < room.OriginX+room.Width; x += step {
		for z = room.OriginZ + step; z < room.OriginZ+room.Depth; z += step {
			// Check if position is too close to existing node
			tooClose := false
			for _, pos := range currentPositions {
				dist := math.Sqrt((pos.X-x)*(pos.X-x) + (pos.Z-z)*(pos.Z-z))
				if dist < 0.5 {
					tooClose = true
					break
				}
			}
			if tooClose {
				continue
			}

			// Evaluate with virtual node
			testPositions := append(currentPositions, NodePosition{MAC: "virtual", X: x, Z: z})
			gdopMap, _, _ := fh.gdopCalc.GDOPMap(testPositions)

			if len(gdopMap) == 0 {
				continue
			}

			// Find worst GDOP with virtual node
			var newWorstGDOP float64
			for _, gdop := range gdopMap {
				if float64(gdop) > newWorstGDOP {
					newWorstGDOP = float64(gdop)
				}
			}

			// Compute improvement
			improvement := currentWorst - newWorstGDOP
			if improvement > bestImprovement {
				bestImprovement = improvement
				bestX = x
				bestZ = z
			}
		}
	}

	return bestX, bestZ, bestImprovement
}
