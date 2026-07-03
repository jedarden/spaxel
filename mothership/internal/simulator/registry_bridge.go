// Package simulator provides integration between simulator virtual nodes and fleet registry.
package simulator

import (
	"fmt"
	"sort"
)

// DefaultNodeOrigin is the degenerate position virtual nodes carry when they
// are created without an explicit placement — it matches the fleet registry's
// nodes-table schema defaults (pos_x=0, pos_y=0, pos_z=1). Nodes still sitting
// at this origin are treated as "unset": the bridge reassigns them spread-out
// geometry at sync time so the registry and downstream fusion engine never see
// co-located / all-at-origin virtual nodes (core symptom in bf-18yn / bf-4q5w).
var DefaultNodeOrigin = Point{X: 0, Y: 0, Z: 1}

// isDefaultOrigin reports whether p is the unset DB-default origin position.
func isDefaultOrigin(p Point) bool {
	return p.X == DefaultNodeOrigin.X && p.Y == DefaultNodeOrigin.Y && p.Z == DefaultNodeOrigin.Z
}

// FleetRegistryBridge integrates virtual nodes with the fleet registry.
// This allows virtual nodes to participate in coverage planning and role assignment.
type FleetRegistryBridge struct {
	store       *VirtualNodeStore
	registryKey string // Prefix for MAC addresses in registry
}

// space returns the geometry the bridge spreads default nodes across. It
// prefers the store's configured space and falls back to DefaultSpace() so the
// bridge is safe even when the store has no space attached.
func (b *FleetRegistryBridge) space() *Space {
	if b.store != nil {
		if s := b.store.GetSpace(); s != nil {
			return s
		}
	}
	return DefaultSpace()
}

// effectivePositions returns the registry position to sync for each node ID.
// Nodes still at the default DB origin (DefaultNodeOrigin) are reassigned
// distinct, spread-out geometry from DefaultNodePositions — sized to the full
// node count so even when every node is unset the geometry spans the room —
// while explicitly-placed (non-origin) nodes keep their position. Default-origin
// nodes are assigned successive spread slots in sorted-ID order, so the mapping
// is deterministic regardless of map iteration order and a single-node sync
// produces the same position as a full sync. The result therefore has no two
// co-located points and is never entirely at the origin.
func (b *FleetRegistryBridge) effectivePositions(nodes []*VirtualNodeState) map[string]Point {
	effective := make(map[string]Point, len(nodes))
	var defaults []*VirtualNodeState
	for _, n := range nodes {
		if isDefaultOrigin(n.Position) {
			defaults = append(defaults, n)
		} else {
			effective[n.ID] = n.Position
		}
	}
	if len(defaults) == 0 {
		return effective
	}

	// Deterministic slot assignment independent of map iteration order.
	sort.Slice(defaults, func(i, j int) bool { return defaults[i].ID < defaults[j].ID })

	space := b.space()
	spread := DefaultNodePositions(space, len(nodes))

	// Track positions already in use so default-origin nodes never collide
	// with each other or with an explicitly-placed node.
	taken := make(map[Point]bool, len(effective)+len(defaults))
	for _, p := range effective {
		taken[p] = true
	}

	si := 0
	for _, n := range defaults {
		for si < len(spread) && taken[spread[si]] {
			si++
		}
		var pos Point
		if si < len(spread) {
			pos = spread[si]
			si++
		} else {
			// Spread set exhausted: more default nodes than slots. The set is
			// sized to len(nodes) so this is effectively unreachable; fall back
			// to the room center, which is still distinct from the origin for
			// any non-degenerate room.
			minX, minY, minZ, maxX, maxY, maxZ := space.Bounds()
			pos = Point{X: (minX + maxX) / 2, Y: (minY + maxY) / 2, Z: (minZ + maxZ) / 2}
		}
		effective[n.ID] = pos
		taken[pos] = true
	}

	return effective
}

// NewFleetRegistryBridge creates a new bridge between virtual nodes and fleet registry
func NewFleetRegistryBridge(store *VirtualNodeStore) *FleetRegistryBridge {
	return &FleetRegistryBridge{
		store:       store,
		registryKey: "virtual",
	}
}

// RegistryNodeAdapter is an interface for fleet registry operations
type RegistryNodeAdapter interface {
	AddVirtualNode(mac, name string, x, y, z float64) error
	SetNodePosition(mac string, x, y, z float64) error
	SetNodeRole(mac, role string) error
	DeleteNode(mac string) error
	GetNode(mac string) (*NodeRecord, error)
	GetAllNodes() ([]NodeRecord, error)
}

// NodeRecord represents a node record from the fleet registry
type NodeRecord struct {
	MAC     string
	Name    string
	Role    string
	PosX    float64
	PosY    float64
	PosZ    float64
	Virtual bool
	Enabled bool
}

// SyncToRegistry synchronizes all virtual nodes to the fleet registry.
//
// Positions are resolved through effectivePositions: any node still at the
// default DB origin (DefaultNodeOrigin) is reassigned distinct, spread-out
// geometry across the store's space so the registry — and the fusion engine
// fed from it via the existing wiring — never observes co-located or
// all-at-origin virtual nodes.
func (b *FleetRegistryBridge) SyncToRegistry(registry RegistryNodeAdapter) error {
	if registry == nil {
		return fmt.Errorf("registry is nil")
	}

	nodes := b.store.ListNodes()
	positions := b.effectivePositions(nodes)

	for _, node := range nodes {
		mac := b.virtualMAC(node.ID)
		pos := positions[node.ID]

		// Check if node exists in registry
		existing, err := registry.GetNode(mac)
		if err != nil {
			// Node doesn't exist, create it
			if err := registry.AddVirtualNode(
				mac,
				node.Name,
				pos.X,
				pos.Y,
				pos.Z,
			); err != nil {
				return fmt.Errorf("add virtual node %s: %w", node.ID, err)
			}
		} else {
			// Node exists, update position/role if changed
			if existing.PosX != pos.X ||
				existing.PosY != pos.Y ||
				existing.PosZ != pos.Z {
				if err := registry.SetNodePosition(mac,
					pos.X,
					pos.Y,
					pos.Z,
				); err != nil {
					return fmt.Errorf("update position for %s: %w", node.ID, err)
				}
			}

			if existing.Role != string(node.Role) {
				if err := registry.SetNodeRole(mac, string(node.Role)); err != nil {
					return fmt.Errorf("update role for %s: %w", node.ID, err)
				}
			}
		}
	}

	// TODO: Remove registry nodes that no longer exist in virtual store?
	// For now, we keep them to avoid accidentally deleting user data

	return nil
}

// SyncOneNode syncs a single virtual node to the registry.
//
// The effective position is resolved over the full node set (not just this
// node) so a single-node sync produces the same spread geometry a full sync
// would write: default-origin nodes receive deterministic spread slots keyed
// by their rank among the unset nodes.
func (b *FleetRegistryBridge) SyncOneNode(registry RegistryNodeAdapter, nodeID string) error {
	if registry == nil {
		return fmt.Errorf("registry is nil")
	}

	node, err := b.store.GetNode(nodeID)
	if err != nil {
		return fmt.Errorf("get node %s: %w", nodeID, err)
	}

	// Resolve over the full set so the slot assignment matches SyncToRegistry.
	positions := b.effectivePositions(b.store.ListNodes())
	pos := positions[nodeID]

	mac := b.virtualMAC(nodeID)

	existing, err := registry.GetNode(mac)
	if err != nil {
		// Node doesn't exist, create it
		return registry.AddVirtualNode(
			mac,
			node.Name,
			pos.X,
			pos.Y,
			pos.Z,
		)
	}

	// Update existing node
	if existing.PosX != pos.X ||
		existing.PosY != pos.Y ||
		existing.PosZ != pos.Z {
		if err := registry.SetNodePosition(mac,
			pos.X,
			pos.Y,
			pos.Z,
		); err != nil {
			return fmt.Errorf("update position: %w", err)
		}
	}

	if existing.Role != string(node.Role) {
		if err := registry.SetNodeRole(mac, string(node.Role)); err != nil {
			return fmt.Errorf("update role: %w", err)
		}
	}

	return nil
}

// RemoveFromRegistry removes a virtual node from the fleet registry
func (b *FleetRegistryBridge) RemoveFromRegistry(registry RegistryNodeAdapter, nodeID string) error {
	if registry == nil {
		return fmt.Errorf("registry is nil")
	}

	mac := b.virtualMAC(nodeID)
	return registry.DeleteNode(mac)
}

// virtualMAC generates a MAC address for a virtual node
func (b *FleetRegistryBridge) virtualMAC(nodeID string) string {
	// Hash the string nodeID into a uint32 for MAC generation
	var h uint32
	for _, c := range []byte(nodeID) {
		h = h*31 + uint32(c)
	}
	// Use a predictable MAC pattern for virtual nodes
	// Format: VE:II:II:II:II where II is node ID hash bytes
	return fmt.Sprintf("VE:%02X:%02X:%02X:%02X",
		(h>>24)&0xFF,
		(h>>16)&0xFF,
		(h>>8)&0xFF,
		h&0xFF)
}

// VirtualNodeID extracts the virtual node ID from a virtual MAC address
func (b *FleetRegistryBridge) VirtualNodeID(mac string) (string, bool) {
	// Check if this is a virtual MAC (starts with "VE:")
	if len(mac) < 3 || mac[0:2] != "VE" {
		return "", false
	}

	// Parse the MAC to get the node ID hash
	// This is a simplified version - in practice, you'd want
	// a more robust bidirectional mapping
	return "", true // TODO: implement reverse mapping
}

// ToRegistryRecords converts virtual nodes to fleet registry records. Positions
// are resolved through effectivePositions so the records reflect exactly what
// SyncToRegistry would write (spread geometry for default-origin nodes).
func (b *FleetRegistryBridge) ToRegistryRecords() []NodeRecord {
	nodes := b.store.ListNodes()
	positions := b.effectivePositions(nodes)
	records := make([]NodeRecord, 0, len(nodes))

	for _, node := range nodes {
		pos := positions[node.ID]
		records = append(records, NodeRecord{
			MAC:     b.virtualMAC(node.ID),
			Name:    node.Name,
			Role:    string(node.Role),
			PosX:    pos.X,
			PosY:    pos.Y,
			PosZ:    pos.Z,
			Virtual: true,
			Enabled: node.Enabled,
		})
	}

	return records
}

// GetStore returns the underlying virtual node store
func (b *FleetRegistryBridge) GetStore() *VirtualNodeStore {
	return b.store
}

// CoverageOptimization represents optimization suggestions for virtual node placement
type CoverageOptimization struct {
	CurrentScore       float64 `json:"current_score"`       // Current coverage score (0-100)
	RecommendedNodes   int     `json:"recommended_nodes"`   // Recommended number of nodes
	SuggestedPositions []Point `json:"suggested_positions"` // Suggested positions for new nodes
	WeakAreas          []Point `json:"weak_areas"`          // Areas with poor coverage
	ImprovementDelta   float64 `json:"improvement_delta"`   // Expected improvement with suggestions
}

// OptimizeCoverage analyzes current coverage and suggests improvements
func (b *FleetRegistryBridge) OptimizeCoverage(space *Space) (*CoverageOptimization, error) {
	nodeSet := b.store.ToNodeSet()
	links := GenerateAllLinks(nodeSet)

	minX, minY, _, maxX, maxY, _ := space.Bounds()

	config := GridConfig{
		MinX:     minX,
		MinY:     minY,
		Width:    maxX - minX,
		Depth:    maxY - minY,
		CellSize: 0.2,
	}

	gc := NewGDOPComputer(links, config)
	results := gc.ComputeAll()

	currentScore := gc.CoverageScore(results)

	// Find weak areas (cells with GDOP > 4)
	weakAreas := make([]Point, 0)
	for row := range results {
		for col := range results[row] {
			if results[row][col].GDOP > 4 {
				// Calculate position from grid indices
				x := minX + float64(col)*config.CellSize
				y := minY + float64(row)*config.CellSize
				weakAreas = append(weakAreas, Point{X: x, Y: y, Z: 1.5})
			}
		}
	}

	// Suggest positions based on corner placement strategy
	suggestedPositions := CornerPositions(space)

	// Estimate improvement with suggested nodes
	// This is a heuristic - in practice, you'd simulate with the suggested nodes
	improvementDelta := 100.0 - currentScore
	if improvementDelta > 50 {
		improvementDelta = 50 // Cap at 50% expected improvement
	}

	return &CoverageOptimization{
		CurrentScore:       currentScore,
		RecommendedNodes:   MinimumNodeCount(space, 4.0),
		SuggestedPositions: suggestedPositions,
		WeakAreas:          weakAreas,
		ImprovementDelta:   improvementDelta,
	}, nil
}
