package simulator

import (
	"encoding/json"
	"fmt"
	"math/rand"
)

// NodeType represents the type of node
type NodeType string

const (
	NodeTypeReal    NodeType = "esp32"  // Real ESP32-S3 node
	NodeTypeVirtual NodeType = "virtual" // Simulated virtual node
	NodeTypeAP      NodeType = "ap"     // Access point (passive radar TX)
)

// NodeRole represents the operational role of a node
type NodeRole string

const (
	RoleTX    NodeRole = "tx"     // Transmit only
	RoleRX    NodeRole = "rx"     // Receive only
	RoleTXRX  NodeRole = "tx_rx"  // Both transmit and receive
	RolePassive NodeRole = "passive" // Passive radar (RX only, AP as TX)
	RoleIdle  NodeRole = "idle"   // Disabled
)

// Node represents a virtual or real node in the simulation
type Node struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Type     NodeType `json:"type"`
	Role     NodeRole `json:"role"`
	Position Point    `json:"position"` // X, Y, Z in meters
	Enabled  bool     `json:"enabled"`
	// For AP nodes
	APBSSID     string `json:"ap_bssid,omitempty"`
	APChannel   int    `json:"ap_channel,omitempty"`
}

// Position returns the node's position as a Point
func (n *Node) PositionVec() Point {
	return n.Position
}

// IsVirtual returns true if this is a virtual node
func (n *Node) IsVirtual() bool {
	return n.Type == NodeTypeVirtual
}

// IsAP returns true if this is an access point node
func (n *Node) IsAP() bool {
	return n.Type == NodeTypeAP
}

// GenerateMAC returns a simulated MAC address for this node
func (n *Node) GenerateMAC() string {
	// Generate a deterministic MAC based on node ID
	if n.IsAP() && n.APBSSID != "" {
		return n.APBSSID
	}
	// Use a simple MAC pattern based on ID hash
	hash := 0
	for _, c := range n.ID {
		hash += int(c)
	}
	return fmt.Sprintf("AA:BB:CC:DD:%02X:%02X", (hash&0xFF), ((hash>>8)&0xFF))
}

// NewNode creates a new node at the given position
func NewNode(id, name string, nodeType NodeType, position Point) *Node {
	return &Node{
		ID:       id,
		Name:     name,
		Type:     nodeType,
		Role:     RoleTXRX,
		Position: position,
		Enabled:  true,
	}
}

// NewVirtualNode creates a new virtual node for planning
func NewVirtualNode(id, name string, position Point) *Node {
	return NewNode(id, name, NodeTypeVirtual, position)
}

// NewAPNode creates a new access point node (for passive radar)
func NewAPNode(id, name, bssid string, channel int, position Point) *Node {
	n := NewNode(id, name, NodeTypeAP, position)
	n.Role = RoleTX // AP acts as TX in passive radar
	n.APBSSID = bssid
	n.APChannel = channel
	return n
}

// NodeSet is a collection of nodes with helper methods
type NodeSet struct {
	nodes []*Node
}

// NewNodeSet creates an empty node set
func NewNodeSet() *NodeSet {
	return &NodeSet{nodes: make([]*Node, 0)}
}

// Add adds a node to the set
func (ns *NodeSet) Add(n *Node) {
	ns.nodes = append(ns.nodes, n)
}

// AddNode adds a node at the given position
func (ns *NodeSet) AddNode(id, name string, nodeType NodeType, position Point) {
	ns.Add(NewNode(id, name, nodeType, position))
}

// AddVirtualNode adds a virtual node at the given position
func (ns *NodeSet) AddVirtualNode(id, name string, position Point) {
	ns.Add(NewVirtualNode(id, name, position))
}

// AddAPNode adds an AP node at the given position
func (ns *NodeSet) AddAPNode(id, name, bssid string, channel int, position Point) {
	ns.Add(NewAPNode(id, name, bssid, channel, position))
}

// Count returns the number of nodes
func (ns *NodeSet) Count() int {
	return len(ns.nodes)
}

// All returns all nodes
func (ns *NodeSet) All() []*Node {
	return ns.nodes
}

// Enabled returns only enabled nodes
func (ns *NodeSet) Enabled() []*Node {
	result := make([]*Node, 0)
	for _, n := range ns.nodes {
		if n.Enabled {
			result = append(result, n)
		}
	}
	return result
}

// TXNodes returns nodes that can transmit (TX or TX_RX or AP)
func (ns *NodeSet) TXNodes() []*Node {
	result := make([]*Node, 0)
	for _, n := range ns.Enabled() {
		if n.Role == RoleTX || n.Role == RoleTXRX || n.IsAP() {
			result = append(result, n)
		}
	}
	return result
}

// RXNodes returns nodes that can receive (RX or TX_RX or Passive)
func (ns *NodeSet) RXNodes() []*Node {
	result := make([]*Node, 0)
	for _, n := range ns.Enabled() {
		if n.Role == RoleRX || n.Role == RoleTXRX || n.Role == RolePassive {
			result = append(result, n)
		}
	}
	return result
}

// GetByID returns a node by ID
func (ns *NodeSet) GetByID(id string) *Node {
	for _, n := range ns.nodes {
		if n.ID == id {
			return n
		}
	}
	return nil
}

// Remove removes a node by ID
func (ns *NodeSet) Remove(id string) bool {
	for i, n := range ns.nodes {
		if n.ID == id {
			ns.nodes = append(ns.nodes[:i], ns.nodes[i+1:]...)
			return true
		}
	}
	return false
}

// Clear removes all nodes
func (ns *NodeSet) Clear() {
	ns.nodes = make([]*Node, 0)
}

// MarshalJSON implements custom JSON marshaling
func (ns *NodeSet) MarshalJSON() ([]byte, error) {
	return json.Marshal(ns.nodes)
}

// UnmarshalJSON implements custom JSON unmarshaling
func (ns *NodeSet) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &ns.nodes)
}

// CornerPositions returns suggested node positions at room corners
// for a given space. Useful for quick initial placement.
func CornerPositions(s *Space) []Point {
	minX, minY, minZ, maxX, maxY, maxZ := s.Bounds()
	height := (maxZ - minZ) / 2 // Average height

	return []Point{
		{X: minX, Y: minY, Z: height},          // Bottom-left, high
		{X: maxX, Y: minY, Z: height},          // Bottom-right, high
		{X: minX, Y: maxY, Z: height},          // Top-left, high
		{X: maxX, Y: maxY, Z: height},          // Top-right, high
		{X: (minX + maxX) / 2, Y: minY, Z: 0.3}, // Bottom-middle, low
		{X: (minX + maxX) / 2, Y: maxY, Z: 0.3}, // Top-middle, low
	}
}

// SuggestedNodes creates a suggested node set for a space
// with nodes positioned at corners and mid-points
func SuggestedNodes(s *Space, count int) *NodeSet {
	ns := NewNodeSet()
	positions := CornerPositions(s)

	// Use corner positions, then add random positions if needed
	for i := 0; i < count; i++ {
		var pos Point
		if i < len(positions) {
			pos = positions[i]
		} else {
			// Random position within bounds
			minX, minY, _, maxX, maxY, maxZ := s.Bounds()
			pos = Point{
				X: minX + rand.Float64()*(maxX-minX),
				Y: minY + rand.Float64()*(maxY-minY),
				Z: rand.Float64() * maxZ,
			}
		}

		role := RoleTXRX

		// Last node can be AP for passive radar
		if i == count-1 {
			role = RoleTX
			ns.AddAPNode(
				fmt.Sprintf("node-%d", i),
				fmt.Sprintf("Node %d", i+1),
				"AA:BB:CC:DD:EE:FF",
				6,
				pos,
			)
		} else {
			ns.AddVirtualNode(
				fmt.Sprintf("node-%d", i),
				fmt.Sprintf("Node %d", i+1),
				pos,
			)
			ns.nodes[i].Role = role
		}
	}

	return ns
}
