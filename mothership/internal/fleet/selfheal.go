// Package fleet implements self-healing fleet management with GDOP optimization
package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sync"
	"time"
)

// SelfHealConfig holds configuration for the self-healing manager
type SelfHealConfig struct {
	// ReconnectGracePeriod is how long to wait before re-optimising after a node disconnects
	ReconnectGracePeriod time.Duration
	// HealInterval is how often to check for needed re-optimisation
	HealInterval time.Duration
	// DegradationThreshold is the coverage score drop that triggers a warning (0-1)
	DegradationThreshold float64
	// MaxHistorySize is the maximum number of optimisation history entries
	MaxHistorySize int
}

// DefaultSelfHealConfig returns sensible defaults
func DefaultSelfHealConfig() SelfHealConfig {
	return SelfHealConfig{
		ReconnectGracePeriod: 5 * time.Minute,
		HealInterval:         60 * time.Second,
		DegradationThreshold: 0.1, // 10% coverage drop
		MaxHistorySize:       100,
	}
}

// FleetChangeEvent is broadcast when the fleet configuration changes
type FleetChangeEvent struct {
	Type             string    `json:"type"` // "node_lost", "node_recovered", "reoptimised"
	Timestamp        time.Time `json:"timestamp"`
	TriggerReason    string    `json:"trigger_reason"`
	OfflineMAC       string    `json:"offline_mac,omitempty"`
	RecoveredMAC     string    `json:"recovered_mac,omitempty"`
	MeanGDOPBefore   float64   `json:"mean_gdop_before"`
	MeanGDOPAfter    float64   `json:"mean_gdop_after"`
	CoverageBefore   float64   `json:"coverage_before"`
	CoverageAfter    float64   `json:"coverage_after"`
	CoverageDelta    float64   `json:"coverage_delta"`
	IsDegradation    bool      `json:"is_degradation"` // true if coverage dropped significantly
	WarningMessage   string    `json:"warning_message,omitempty"`
	RoleAssignments  map[string]string `json:"role_assignments"`
	GDOPBefore       []float32 `json:"gdop_before,omitempty"` // GDOP map before
	GDOPAfter        []float32 `json:"gdop_after,omitempty"`  // GDOP map after
	GDOPCols         int       `json:"gdop_cols"`
	GDOPRows         int       `json:"gdop_rows"`
}

// OfflineNode tracks a node that has gone offline
type OfflineNode struct {
	MAC           string
	WentOfflineAt time.Time
	PreviousRole  string
	GracePeriod   time.Duration
}

// SelfHealManager handles graceful degradation and re-optimisation
type SelfHealManager struct {
	mu sync.RWMutex

	registry   *Registry
	optimiser  *RoleOptimiser
	notifier   NodeStateNotifier
	bcaster    FleetChangeBroadcaster
	gdopCalc   GDOPCalculator

	config SelfHealConfig

	// Node tracking
	online       map[string]struct{}
	offlineNodes map[string]*OfflineNode // MAC -> offline tracking

	// Coverage state
	lastCoverageScore float64
	lastMeanGDOP      float64
	lastGDOPMap       []float32
	lastGDOPCols      int
	lastGDOPRows      int

	// Current role assignments
	currentRoles map[string]string
}

// FleetChangeBroadcaster is called when fleet state changes
type FleetChangeBroadcaster interface {
	BroadcastFleetChange(event FleetChangeEvent)
}

// NewSelfHealManager creates a new self-healing manager
func NewSelfHealManager(registry *Registry, optimiser *RoleOptimiser, config SelfHealConfig) *SelfHealManager {
	return &SelfHealManager{
		registry:     registry,
		optimiser:    optimiser,
		config:       config,
		online:       make(map[string]struct{}),
		offlineNodes: make(map[string]*OfflineNode),
		currentRoles: make(map[string]string),
	}
}

// SetNotifier sets the node state notifier
func (shm *SelfHealManager) SetNotifier(n NodeStateNotifier) {
	shm.mu.Lock()
	shm.notifier = n
	shm.mu.Unlock()
}

// SetBroadcaster sets the fleet change broadcaster
func (shm *SelfHealManager) SetBroadcaster(b FleetChangeBroadcaster) {
	shm.mu.Lock()
	shm.bcaster = b
	shm.mu.Unlock()
}

// SetGDOPCalculator sets the GDOP calculator
func (shm *SelfHealManager) SetGDOPCalculator(calc GDOPCalculator) {
	shm.mu.Lock()
	shm.gdopCalc = calc
	shm.optimiser.SetGDOPCalculator(calc)
	shm.mu.Unlock()
}

// OnNodeConnected handles a node connection event
func (shm *SelfHealManager) OnNodeConnected(mac, firmware, chip string) {
	now := time.Now()

	shm.mu.Lock()
	// Check if this is a reconnection within grace period
	offline, wasOffline := shm.offlineNodes[mac]
	previousRole := ""
	if wasOffline {
		previousRole = offline.PreviousRole
		delete(shm.offlineNodes, mac)
	}

	shm.online[mac] = struct{}{}
	notifier := shm.notifier
	shm.mu.Unlock()

	// Persist node to registry
	if err := shm.registry.UpsertNode(mac, firmware, chip); err != nil {
		log.Printf("[WARN] fleet: upsert node %s: %v", mac, err)
	}

	// Clear offline timestamp
	if err := shm.registry.ClearNodeOffline(mac); err != nil {
		log.Printf("[WARN] fleet: clear offline %s: %v", mac, err)
	}

	// Get node position from registry
	node, err := shm.registry.GetNode(mac)
	if err != nil {
		log.Printf("[WARN] fleet: get node %s: %v", mac, err)
	}

	if wasOffline && now.Sub(offline.WentOfflineAt) < shm.config.ReconnectGracePeriod {
		// Within grace period - restore previous role without re-optimising
		log.Printf("[INFO] fleet: node %s reconnected within grace period (%.0fs) — restoring previous role %s",
			mac, now.Sub(offline.WentOfflineAt).Seconds(), previousRole)

		shm.mu.Lock()
		shm.currentRoles[mac] = previousRole
		shm.mu.Unlock()

		if err := shm.registry.SetNodeRole(mac, previousRole); err != nil {
			log.Printf("[WARN] fleet: set role %s: %v", mac, err)
		}

		if notifier != nil {
			notifier.SendRoleToMAC(mac, previousRole, "")
		}

		// Broadcast recovery event
		shm.broadcastEvent(FleetChangeEvent{
			Type:           "node_recovered",
			Timestamp:      now,
			TriggerReason:  "grace_period_reconnect",
			RecoveredMAC:   mac,
			RoleAssignments: shm.getCurrentRoles(),
		})

		return
	}

	// New connection or grace period expired - run optimisation
	triggerReason := "node_connected"
	if wasOffline {
		triggerReason = "node_reconnected_after_grace"
	}

	shm.optimiseAndApply(triggerReason, node)
}

// OnNodeDisconnected handles a node disconnection event
func (shm *SelfHealManager) OnNodeDisconnected(mac string) {
	shm.mu.Lock()

	// Get current role before disconnect
	previousRole := shm.currentRoles[mac]

	// Record coverage before
	coverageBefore := shm.lastCoverageScore
	gdopBefore := shm.lastMeanGDOP
	gdopMapBefore := shm.lastGDOPMap
	cols, rows := shm.lastGDOPCols, shm.lastGDOPRows

	// Remove from online set
	delete(shm.online, mac)

	// Track for grace period
	shm.offlineNodes[mac] = &OfflineNode{
		MAC:           mac,
		WentOfflineAt: time.Now(),
		PreviousRole:  previousRole,
		GracePeriod:   shm.config.ReconnectGracePeriod,
	}

	// Get remaining online nodes
	onlineList := make([]string, 0, len(shm.online))
	for m := range shm.online {
		onlineList = append(onlineList, m)
	}

	shm.mu.Unlock()

	// Save previous role and offline time to registry
	if err := shm.registry.SetNodePreviousRole(mac, previousRole); err != nil {
		log.Printf("[WARN] fleet: save previous role %s: %v", mac, err)
	}
	if err := shm.registry.SetNodeOffline(mac); err != nil {
		log.Printf("[WARN] fleet: set offline %s: %v", mac, err)
	}

	// Get node info for remaining online nodes
	remainingNodes := shm.getOnlineNodeInfo(onlineList)

	// Run optimisation with remaining nodes
	result := shm.optimiser.Optimise(remainingNodes, "node_disconnected:"+mac)

	shm.mu.Lock()
	// Update role assignments
	for _, assignment := range result.Assignments {
		shm.currentRoles[assignment.MAC] = assignment.Role
	}
	coverageAfter := result.CoverageScore
	gdopAfter := result.MeanGDOP
	shm.lastCoverageScore = coverageAfter
	shm.lastMeanGDOP = gdopAfter
	if len(result.GDOPAfter) > 0 {
		shm.lastGDOPMap = result.GDOPAfter
		shm.lastGDOPCols = result.GDOPCols
		shm.lastGDOPRows = result.GDOPRows
	}
	shm.mu.Unlock()

	// Calculate coverage delta
	coverageDelta := coverageAfter - coverageBefore
	isDegradation := coverageDelta < -shm.config.DegradationThreshold

	// Apply new roles to surviving nodes
	shm.applyRolesToNodes(result.Assignments, onlineList)

	// Build warning message if degraded
	var warningMessage string
	if isDegradation {
		pctBefore := coverageBefore * 100
		pctAfter := coverageAfter * 100
		warningMessage = formatDegradationWarning(mac, pctBefore, pctAfter)
		log.Printf("[WARN] fleet: coverage degraded by %.1f%% (%.1f%% -> %.1f%%) due to node %s disconnect",
			-coverageDelta*100, pctBefore, pctAfter, mac)
	}

	// Record to history
	nodesBeforeJSON, _ := json.Marshal(onlineList)
	nodesAfterJSON, _ := json.Marshal(remainingNodes)
	shm.registry.AddOptimisationHistory(OptimisationHistoryRecord{
		Timestamp:       time.Now(),
		TriggerReason:   "node_disconnected:" + mac,
		MeanGDOPBefore:  gdopBefore,
		MeanGDOPAfter:   gdopAfter,
		CoverageDelta:   coverageDelta,
		NodesBeforeJSON: string(nodesBeforeJSON),
		NodesAfterJSON:  string(nodesAfterJSON),
	})

	// Broadcast fleet change event
	shm.broadcastEvent(FleetChangeEvent{
		Type:            "node_lost",
		Timestamp:       time.Now(),
		TriggerReason:   "node_disconnected",
		OfflineMAC:      mac,
		MeanGDOPBefore:  gdopBefore,
		MeanGDOPAfter:   gdopAfter,
		CoverageBefore:  coverageBefore,
		CoverageAfter:   coverageAfter,
		CoverageDelta:   coverageDelta,
		IsDegradation:   isDegradation,
		WarningMessage:  warningMessage,
		RoleAssignments: shm.getCurrentRoles(),
		GDOPBefore:      gdopMapBefore,
		GDOPAfter:       result.GDOPAfter,
		GDOPCols:        cols,
		GDOPRows:        rows,
	})

	log.Printf("[INFO] fleet: node %s left, reoptimised roles (coverage: %.1f%% -> %.1f%%)",
		mac, coverageBefore*100, coverageAfter*100)
}

// Run starts the periodic self-healing loop
func (shm *SelfHealManager) Run(ctx context.Context) {
	ticker := time.NewTicker(shm.config.HealInterval)
	defer ticker.Stop()

	cleanupTicker := time.NewTicker(30 * time.Second)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			shm.selfHeal()
		case <-cleanupTicker.C:
			shm.cleanupExpiredGracePeriods()
		}
	}
}

// selfHeal checks for needed re-optimisation and re-pushes roles
func (shm *SelfHealManager) selfHeal() {
	shm.mu.RLock()
	notifier := shm.notifier
	onlineList := make([]string, 0, len(shm.online))
	for mac := range shm.online {
		onlineList = append(onlineList, mac)
	}
	roles := shm.currentRoles
	shm.mu.RUnlock()

	if notifier == nil {
		return
	}

	// Re-push roles to all online nodes
	for _, mac := range onlineList {
		if role, ok := roles[mac]; ok {
			notifier.SendRoleToMAC(mac, role, "")
		}
	}
}

// cleanupExpiredGracePeriods removes expired offline node entries
func (shm *SelfHealManager) cleanupExpiredGracePeriods() {
	shm.mu.Lock()
	defer shm.mu.Unlock()

	now := time.Now()
	for mac, offline := range shm.offlineNodes {
		if now.Sub(offline.WentOfflineAt) > offline.GracePeriod {
			delete(shm.offlineNodes, mac)
			log.Printf("[INFO] fleet: grace period expired for node %s", mac)
		}
	}
}

// optimiseAndApply runs optimisation and applies new roles
func (shm *SelfHealManager) optimiseAndApply(triggerReason string, connectedNode *NodeRecord) {
	shm.mu.Lock()
	onlineList := make([]string, 0, len(shm.online))
	for mac := range shm.online {
		onlineList = append(onlineList, mac)
	}
	coverageBefore := shm.lastCoverageScore
	gdopBefore := shm.lastMeanGDOP
	gdopMapBefore := shm.lastGDOPMap
	cols, rows := shm.lastGDOPCols, shm.lastGDOPRows
	shm.mu.Unlock()

	// Get node info for all online nodes
	nodes := shm.getOnlineNodeInfo(onlineList)

	// Run optimisation
	result := shm.optimiser.Optimise(nodes, triggerReason)

	shm.mu.Lock()
	// Update role assignments
	for _, assignment := range result.Assignments {
		shm.currentRoles[assignment.MAC] = assignment.Role
	}
	coverageAfter := result.CoverageScore
	gdopAfter := result.MeanGDOP
	shm.lastCoverageScore = coverageAfter
	shm.lastMeanGDOP = gdopAfter
	if len(result.GDOPAfter) > 0 {
		shm.lastGDOPMap = result.GDOPAfter
		shm.lastGDOPCols = result.GDOPCols
		shm.lastGDOPRows = result.GDOPRows
	}
	shm.mu.Unlock()

	// Apply new roles
	shm.applyRolesToNodes(result.Assignments, onlineList)

	// Calculate coverage delta
	coverageDelta := coverageAfter - coverageBefore

	// Record to history
	nodesBeforeJSON, _ := json.Marshal(onlineList)
	nodesAfterJSON, _ := json.Marshal(nodes)
	shm.registry.AddOptimisationHistory(OptimisationHistoryRecord{
		Timestamp:       time.Now(),
		TriggerReason:   triggerReason,
		MeanGDOPBefore:  gdopBefore,
		MeanGDOPAfter:   gdopAfter,
		CoverageDelta:   coverageDelta,
		NodesBeforeJSON: string(nodesBeforeJSON),
		NodesAfterJSON:  string(nodesAfterJSON),
	})

	// Broadcast event if coverage changed
	if math.Abs(coverageDelta) > 0.01 {
		isDegradation := coverageDelta < -shm.config.DegradationThreshold
		var warningMessage string
		if isDegradation {
			warningMessage = formatDegradationWarning("", coverageBefore*100, coverageAfter*100)
		}

		shm.broadcastEvent(FleetChangeEvent{
			Type:            "reoptimised",
			Timestamp:       time.Now(),
			TriggerReason:   triggerReason,
			MeanGDOPBefore:  gdopBefore,
			MeanGDOPAfter:   gdopAfter,
			CoverageBefore:  coverageBefore,
			CoverageAfter:   coverageAfter,
			CoverageDelta:   coverageDelta,
			IsDegradation:   isDegradation,
			WarningMessage:  warningMessage,
			RoleAssignments: shm.getCurrentRoles(),
			GDOPBefore:      gdopMapBefore,
			GDOPAfter:       result.GDOPAfter,
			GDOPCols:        cols,
			GDOPRows:        rows,
		})
	}

	log.Printf("[INFO] fleet: optimised roles (trigger: %s, coverage: %.1f%% -> %.1f%%)",
		triggerReason, coverageBefore*100, coverageAfter*100)
}

// ManualOptimise triggers a manual re-optimisation
func (shm *SelfHealManager) ManualOptimise() *OptimiseResult {
	shm.mu.RLock()
	onlineList := make([]string, 0, len(shm.online))
	for mac := range shm.online {
		onlineList = append(onlineList, mac)
	}
	shm.mu.RUnlock()

	nodes := shm.getOnlineNodeInfo(onlineList)
	result := shm.optimiser.Optimise(nodes, "manual_trigger")

	shm.mu.Lock()
	for _, assignment := range result.Assignments {
		shm.currentRoles[assignment.MAC] = assignment.Role
	}
	shm.lastCoverageScore = result.CoverageScore
	shm.lastMeanGDOP = result.MeanGDOP
	if len(result.GDOPAfter) > 0 {
		shm.lastGDOPMap = result.GDOPAfter
	}
	shm.mu.Unlock()

	shm.applyRolesToNodes(result.Assignments, onlineList)

	return result
}

// SimulateNodeRemoval predicts coverage impact if a node is removed
func (shm *SelfHealManager) SimulateNodeRemoval(mac string) (*OptimiseResult, float64, error) {
	shm.mu.RLock()
	onlineList := make([]string, 0, len(shm.online))
	for m := range shm.online {
		onlineList = append(onlineList, m)
	}
	shm.mu.RUnlock()

	nodes := shm.getOnlineNodeInfo(onlineList)
	result, delta := shm.optimiser.SimulateRemoval(nodes, mac)

	return result, delta, nil
}

// GetOnlineNodes returns list of online node MACs
func (shm *SelfHealManager) GetOnlineNodes() []string {
	shm.mu.RLock()
	defer shm.mu.RUnlock()

	list := make([]string, 0, len(shm.online))
	for mac := range shm.online {
		list = append(list, mac)
	}
	return list
}

// GetCurrentRoles returns current role assignments
func (shm *SelfHealManager) GetCurrentRoles() map[string]string {
	shm.mu.RLock()
	defer shm.mu.RUnlock()

	roles := make(map[string]string, len(shm.currentRoles))
	for k, v := range shm.currentRoles {
		roles[k] = v
	}
	return roles
}

// GetCoverageScore returns the current coverage score
func (shm *SelfHealManager) GetCoverageScore() float64 {
	shm.mu.RLock()
	defer shm.mu.RUnlock()
	return shm.lastCoverageScore
}

// GetOptimisationHistory returns recent optimisation history
func (shm *SelfHealManager) GetOptimisationHistory(limit int) ([]OptimisationHistoryRecord, error) {
	return shm.registry.GetOptimisationHistory(limit)
}

// getOnlineNodeInfo returns NodeInfo for the given MACs
func (shm *SelfHealManager) getOnlineNodeInfo(macs []string) []NodeInfo {
	nodes := make([]NodeInfo, 0, len(macs))
	for _, mac := range macs {
		record, err := shm.registry.GetNode(mac)
		if err != nil {
			continue
		}
		nodes = append(nodes, NodeInfo{
			MAC:         record.MAC,
			PosX:        record.PosX,
			PosY:        record.PosY,
			PosZ:        record.PosZ,
			HealthScore: record.HealthScore,
			Capabilities: NodeCapabilities{
				CanTX: true,
				CanRX: true,
			},
		})
	}
	return nodes
}

// applyRolesToNodes sends role commands to nodes
func (shm *SelfHealManager) applyRolesToNodes(assignments []RoleAssignment, onlineList []string) {
	shm.mu.RLock()
	notifier := shm.notifier
	shm.mu.RUnlock()

	if notifier == nil {
		return
	}

	onlineSet := make(map[string]struct{})
	for _, mac := range onlineList {
		onlineSet[mac] = struct{}{}
	}

	for _, assignment := range assignments {
		if _, online := onlineSet[assignment.MAC]; !online {
			continue
		}

		// Persist role
		if err := shm.registry.SetNodeRole(assignment.MAC, assignment.Role); err != nil {
			log.Printf("[WARN] fleet: set role %s: %v", assignment.MAC, err)
		}

		// Send to node
		notifier.SendRoleToMAC(assignment.MAC, assignment.Role, "")
	}
}

// getCurrentRoles returns a copy of current roles
func (shm *SelfHealManager) getCurrentRoles() map[string]string {
	shm.mu.RLock()
	defer shm.mu.RUnlock()

	roles := make(map[string]string, len(shm.currentRoles))
	for k, v := range shm.currentRoles {
		roles[k] = v
	}
	return roles
}

// broadcastEvent sends an event to the broadcaster
func (shm *SelfHealManager) broadcastEvent(event FleetChangeEvent) {
	shm.mu.RLock()
	bcaster := shm.bcaster
	shm.mu.RUnlock()

	if bcaster == nil {
		return
	}

	bcaster.BroadcastFleetChange(event)
}

// formatDegradationWarning creates a human-readable warning message
func formatDegradationWarning(offlineMAC string, pctBefore, pctAfter float64) string {
	if offlineMAC != "" {
		return formatWarning("Node %s is offline. Coverage dropped from %.0f%% to %.0f%%.", offlineMAC, pctBefore, pctAfter)
	}
	return formatWarning("Coverage dropped from %.0f%% to %.0f%%.", pctBefore, pctAfter)
}

// formatWarning formats a warning message
func formatWarning(format string, args ...interface{}) string {
	return fmt.Sprintf(format, args...)
}
