// Package simulator provides integration between simulator virtual nodes and fleet registry.
package simulator

import (
	"fmt"
)

// FleetRegistryBridge integrates virtual nodes with the fleet registry.
// This allows virtual nodes to participate in coverage planning and role assignment.
type FleetRegistryBridge struct {
	store       *VirtualNodeStore
	registryKey string // Prefix for MAC addresses in registry
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
	MAC      string
	Name     string
	Role     string
	PosX     float64
	PosY     float64
	PosZ     float64
	Virtual  bool
	Enabled  bool
}

// SyncToRegistry synchronizes all virtual nodes to the fleet registry
func (b *FleetRegistryBridge) SyncToRegistry(registry RegistryNodeAdapter) error {
	if registry == nil {
		return fmt.Errorf("registry is nil")
	}

	nodes := b.store.ListNodes()

	for _, node := range nodes {
		mac := b.virtualMAC(node.ID)

		// Check if node exists in registry
		existing, err := registry.GetNode(mac)
		if err != nil {
			// Node doesn't exist, create it
			if err := registry.AddVirtualNode(
				mac,
				node.Name,
				node.Position.X,
				node.Position.Y,
				node.Position.Z,
			); err != nil {
				return fmt.Errorf("add virtual node %s: %w", node.ID, err)
			}
		} else {
			// Node exists, update position/role if changed
			if existing.PosX != node.Position.X ||
				existing.PosY != node.Position.Y ||
				existing.PosZ != node.Position.Z {
				if err := registry.SetNodePosition(mac,
					node.Position.X,
					node.Position.Y,
					node.Position.Z,
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

// SyncOneNode syncs a single virtual node to the registry
func (b *FleetRegistryBridge) SyncOneNode(registry RegistryNodeAdapter, nodeID string) error {
	if registry == nil {
		return fmt.Errorf("registry is nil")
	}

	node, err := b.store.GetNode(nodeID)
	if err != nil {
		return fmt.Errorf("get node %s: %w", nodeID, err)
	}

	mac := b.virtualMAC(nodeID)

	existing, err := registry.GetNode(mac)
	if err != nil {
		// Node doesn't exist, create it
		return registry.AddVirtualNode(
			mac,
			node.Name,
			node.Position.X,
			node.Position.Y,
			node.Position.Z,
		)
	}

	// Update existing node
	if existing.PosX != node.Position.X ||
		existing.PosY != node.Position.Y ||
		existing.PosZ != node.Position.Z {
		if err := registry.SetNodePosition(mac,
			node.Position.X,
			node.Position.Y,
			node.Position.Z,
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
	// Use a predictable MAC pattern for virtual nodes
	// Format: VE:EE:II:II:II:II where VE identifies virtual, II is node ID hash
	return fmt.Sprintf("VE:%02X:%02X:%02X:%02X",
		(nodeID>>24)&0xFF,
		(nodeID>>16)&0xFF,
		(nodeID>>8)&0xFF,
		nodeID&0xFF)
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

// ToRegistryRecords converts virtual nodes to fleet registry records
func (b *FleetRegistryBridge) ToRegistryRecords() []NodeRecord {
	nodes := b.store.ListNodes()
	records := make([]NodeRecord, 0, len(nodes))

	for _, node := range nodes {
		records = append(records, NodeRecord{
			MAC:     b.virtualMAC(node.ID),
			Name:    node.Name,
			Role:    string(node.Role),
			PosX:    node.Position.X,
			PosY:    node.Position.Y,
			PosZ:    node.Position.Z,
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
	CurrentScore       float64  `json:"current_score"`       // Current coverage score (0-100)
	RecommendedNodes   int      `json:"recommended_nodes"`   // Recommended number of nodes
	SuggestedPositions []Point  `json:"suggested_positions"`  // Suggested positions for new nodes
	WeakAreas          []Point  `json:"weak_areas"`           // Areas with poor coverage
	ImprovementDelta   float64  `json:"improvement_delta"`    // Expected improvement with suggestions
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
			if results[row][col] > 4 {
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
