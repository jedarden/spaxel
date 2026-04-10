// Package simulator provides virtual node state management for the virtual space.
// This module handles creation, persistence, and state management of virtual nodes
// within the simulation space.
package simulator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// VirtualNodeState represents the persistent state of a virtual node
type VirtualNodeState struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Type        NodeType  `json:"type"`
	Role        NodeRole  `json:"role"`
	Position    Point     `json:"position"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// For AP nodes
	APBSSID   string `json:"ap_bssid,omitempty"`
	APChannel int    `json:"ap_channel,omitempty"`
	// State metadata
	Description string                 `json:"description,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// VirtualNodeStore manages the persistence of virtual node states
type VirtualNodeStore struct {
	mu     sync.RWMutex
	nodes  map[string]*VirtualNodeState
	path   string
	space  *Space
	closed bool
}

// StoreConfig holds configuration for the virtual node store
type StoreConfig struct {
	DataDir string // Directory for storing node state files
	Space   *Space // The virtual space these nodes belong to
}

// NewVirtualNodeStore creates a new virtual node store with persistence
func NewVirtualNodeStore(config StoreConfig) (*VirtualNodeStore, error) {
	if config.DataDir == "" {
		config.DataDir = "./data"
	}
	if config.Space == nil {
		config.Space = DefaultSpace()
	}

	// Ensure data directory exists
	if err := os.MkdirAll(config.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	storePath := filepath.Join(config.DataDir, "virtual_nodes.json")

	store := &VirtualNodeStore{
		nodes: make(map[string]*VirtualNodeState),
		path:  storePath,
		space: config.Space,
	}

	// Load existing state if available
	if err := store.load(); err != nil {
		// If file doesn't exist, that's okay for new store
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("load virtual nodes: %w", err)
		}
	}

	return store, nil
}

// CreateNode creates a new virtual node at the specified position
func (s *VirtualNodeStore) CreateNode(id, name string, nodeType NodeType, position Point) (*VirtualNodeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, fmt.Errorf("store is closed")
	}

	if _, exists := s.nodes[id]; exists {
		return nil, fmt.Errorf("node %s already exists", id)
	}

	// Validate position is within space bounds
	minX, minY, minZ, maxX, maxY, maxZ := s.space.Bounds()
	if position.X < minX || position.X > maxX ||
		position.Y < minY || position.Y > maxY ||
		position.Z < minZ || position.Z > maxZ {
		return nil, fmt.Errorf("position (%f, %f, %f) is outside space bounds [%f, %f, %f] to [%f, %f, %f]",
			position.X, position.Y, position.Z, minX, minY, minZ, maxX, maxY, maxZ)
	}

	now := time.Now()
	state := &VirtualNodeState{
		ID:        id,
		Name:      name,
		Type:      nodeType,
		Role:      RoleTXRX,
		Position:  position,
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  make(map[string]interface{}),
		Tags:      make([]string, 0),
	}

	s.nodes[id] = state

	// Persist to disk
	if err := s.save(); err != nil {
		delete(s.nodes, id)
		return nil, fmt.Errorf("save node: %w", err)
	}

	return state, nil
}

// CreateVirtualNode creates a new virtual planning node
func (s *VirtualNodeStore) CreateVirtualNode(id, name string, position Point) (*VirtualNodeState, error) {
	return s.CreateNode(id, name, NodeTypeVirtual, position)
}

// CreateAPNode creates a new access point node (for passive radar)
func (s *VirtualNodeStore) CreateAPNode(id, name, bssid string, channel int, position Point) (*VirtualNodeState, error) {
	state, err := s.CreateNode(id, name, NodeTypeAP, position)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	state.Role = RoleTX
	state.APBSSID = bssid
	state.APChannel = channel
	state.UpdatedAt = time.Now()
	s.mu.Unlock()

	if err := s.save(); err != nil {
		return nil, fmt.Errorf("save AP node: %w", err)
	}

	return state, nil
}

// GetNode retrieves a node by ID
func (s *VirtualNodeStore) GetNode(id string) (*VirtualNodeState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, fmt.Errorf("store is closed")
	}

	state, exists := s.nodes[id]
	if !exists {
		return nil, fmt.Errorf("node %s not found", id)
	}

	// Return a copy to prevent external mutations
	return s.copyState(state), nil
}

// UpdateNodePosition updates a node's position
func (s *VirtualNodeStore) UpdateNodePosition(id string, position Point) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("store is closed")
	}

	state, exists := s.nodes[id]
	if !exists {
		return fmt.Errorf("node %s not found", id)
	}

	// Validate position is within space bounds
	minX, minY, minZ, maxX, maxY, maxZ := s.space.Bounds()
	if position.X < minX || position.X > maxX ||
		position.Y < minY || position.Y > maxY ||
		position.Z < minZ || position.Z > maxZ {
		return fmt.Errorf("position (%f, %f, %f) is outside space bounds [%f, %f, %f] to [%f, %f, %f]",
			position.X, position.Y, position.Z, minX, minY, minZ, maxX, maxY, maxZ)
	}

	state.Position = position
	state.UpdatedAt = time.Now()

	return s.save()
}

// UpdateNodeRole updates a node's role
func (s *VirtualNodeStore) UpdateNodeRole(id string, role NodeRole) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("store is closed")
	}

	state, exists := s.nodes[id]
	if !exists {
		return fmt.Errorf("node %s not found", id)
	}

	state.Role = role
	state.UpdatedAt = time.Now()

	return s.save()
}

// SetNodeEnabled enables or disables a node
func (s *VirtualNodeStore) SetNodeEnabled(id string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("store is closed")
	}

	state, exists := s.nodes[id]
	if !exists {
		return fmt.Errorf("node %s not found", id)
	}

	state.Enabled = enabled
	state.UpdatedAt = time.Now()

	return s.save()
}

// UpdateNodeMetadata updates a node's metadata
func (s *VirtualNodeStore) UpdateNodeMetadata(id string, metadata map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("store is closed")
	}

	state, exists := s.nodes[id]
	if !exists {
		return fmt.Errorf("node %s not found", id)
	}

	state.Metadata = metadata
	state.UpdatedAt = time.Now()

	return s.save()
}

// AddTag adds a tag to a node
func (s *VirtualNodeStore) AddTag(id, tag string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("store is closed")
	}

	state, exists := s.nodes[id]
	if !exists {
		return fmt.Errorf("node %s not found", id)
	}

	// Check if tag already exists
	for _, t := range state.Tags {
		if t == tag {
			return nil // Already has this tag
		}
	}

	state.Tags = append(state.Tags, tag)
	state.UpdatedAt = time.Now()

	return s.save()
}

// RemoveTag removes a tag from a node
func (s *VirtualNodeStore) RemoveTag(id, tag string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("store is closed")
	}

	state, exists := s.nodes[id]
	if !exists {
		return fmt.Errorf("node %s not found", id)
	}

	// Filter out the tag
	newTags := make([]string, 0, len(state.Tags))
	for _, t := range state.Tags {
		if t != tag {
			newTags = append(newTags, t)
		}
	}

	if len(newTags) == len(state.Tags) {
		return nil // Tag wasn't present
	}

	state.Tags = newTags
	state.UpdatedAt = time.Now()

	return s.save()
}

// DeleteNode removes a node from the store
func (s *VirtualNodeStore) DeleteNode(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("store is closed")
	}

	if _, exists := s.nodes[id]; !exists {
		return fmt.Errorf("node %s not found", id)
	}

	delete(s.nodes, id)

	return s.save()
}

// ListNodes returns all nodes
func (s *VirtualNodeStore) ListNodes() []*VirtualNodeState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil
	}

	result := make([]*VirtualNodeState, 0, len(s.nodes))
	for _, state := range s.nodes {
		result = append(result, s.copyState(state))
	}

	return result
}

// ListEnabledNodes returns only enabled nodes
func (s *VirtualNodeStore) ListEnabledNodes() []*VirtualNodeState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil
	}

	result := make([]*VirtualNodeState, 0)
	for _, state := range s.nodes {
		if state.Enabled {
			result = append(result, s.copyState(state))
		}
	}

	return result
}

// ListNodesByType returns nodes of a specific type
func (s *VirtualNodeStore) ListNodesByType(nodeType NodeType) []*VirtualNodeState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil
	}

	result := make([]*VirtualNodeState, 0)
	for _, state := range s.nodes {
		if state.Type == nodeType {
			result = append(result, s.copyState(state))
		}
	}

	return result
}

// ListNodesByTag returns nodes with a specific tag
func (s *VirtualNodeStore) ListNodesByTag(tag string) []*VirtualNodeState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil
	}

	result := make([]*VirtualNodeState, 0)
	for _, state := range s.nodes {
		for _, t := range state.Tags {
			if t == tag {
				result = append(result, s.copyState(state))
				break
			}
		}
	}

	return result
}

// Count returns the total number of nodes
func (s *VirtualNodeStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.nodes)
}

// GetSpace returns the space associated with this store
func (s *VirtualNodeStore) GetSpace() *Space {
	return s.space
}

// UpdateSpace updates the space bounds for this store
func (s *VirtualNodeStore) UpdateSpace(space *Space) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := space.Validate(); err != nil {
		return fmt.Errorf("validate space: %w", err)
	}

	s.space = space

	// Re-validate all node positions are still within bounds
	for _, state := range s.nodes {
		minX, minY, minZ, maxX, maxY, maxZ := s.space.Bounds()
		if state.Position.X < minX || state.Position.X > maxX ||
			state.Position.Y < minY || state.Position.Y > maxY ||
			state.Position.Z < minZ || state.Position.Z > maxZ {
			// Disable nodes that are now outside bounds
			state.Enabled = false
		}
	}

	return s.save()
}

// Clear removes all nodes from the store
func (s *VirtualNodeStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("store is closed")
	}

	s.nodes = make(map[string]*VirtualNodeState)

	return s.save()
}

// ToNodeSet converts the stored nodes to a NodeSet for simulation
func (s *VirtualNodeStore) ToNodeSet() *NodeSet {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ns := NewNodeSet()

	for _, state := range s.nodes {
		if !state.Enabled {
			continue
		}

		if state.Type == NodeTypeAP {
			ns.AddAPNode(state.ID, state.Name, state.APBSSID, state.APChannel, state.Position)
			// Update role from AddAPNode default
			for _, n := range ns.nodes {
				if n.ID == state.ID {
					n.Role = state.Role
					break
				}
			}
		} else {
			ns.AddNode(state.ID, state.Name, state.Type, state.Position)
			// Update role
			for _, n := range s.nodes {
				if n.ID == state.ID {
					n.Role = state.Role
					break
				}
			}
		}
	}

	return ns
}

// ImportFromNodeSet imports nodes from a NodeSet
func (s *VirtualNodeStore) ImportFromNodeSet(nodeSet *NodeSet) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("store is closed")
	}

	now := time.Now()
	for _, node := range nodeSet.All() {
		state := &VirtualNodeState{
			ID:        node.ID,
			Name:      node.Name,
			Type:      node.Type,
			Role:      node.Role,
			Position:  node.Position,
			Enabled:   node.Enabled,
			CreatedAt: now,
			UpdatedAt: now,
			Metadata:  make(map[string]interface{}),
			Tags:      make([]string, 0),
		}

		if node.IsAP() {
			state.APBSSID = node.APBSSID
			state.APChannel = node.APChannel
		}

		// Merge with existing if present
		if existing, exists := s.nodes[node.ID]; exists {
			state.CreatedAt = existing.CreatedAt
			state.Metadata = existing.Metadata
			state.Tags = existing.Tags
		}

		s.nodes[node.ID] = state
	}

	return s.save()
}

// Close closes the store and releases resources
func (s *VirtualNodeStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	// Mark as closed to prevent new operations during final save
	s.closed = true

	// Final save before closing (saveLocked checks closed flag, so we need special handling)
	data, err := json.MarshalIndent(s.nodes, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal nodes: %w", err)
	}

	// Write to temporary file first
	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("rename file: %w", err)
	}

	return nil
}

// save persists the current state to disk
func (s *VirtualNodeStore) save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.saveLocked()
}

// saveLocked saves state without acquiring lock (caller must hold lock)
func (s *VirtualNodeStore) saveLocked() error {
	if s.closed {
		return fmt.Errorf("store is closed")
	}

	data, err := json.MarshalIndent(s.nodes, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal nodes: %w", err)
	}

	// Write to temporary file first
	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("rename file: %w", err)
	}

	return nil
}

// load restores state from disk
func (s *VirtualNodeStore) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, &s.nodes); err != nil {
		return fmt.Errorf("unmarshal nodes: %w", err)
	}

	return nil
}

// copyState creates a deep copy of a node state
func (s *VirtualNodeStore) copyState(state *VirtualNodeState) *VirtualNodeState {
	// Copy metadata
	metadata := make(map[string]interface{})
	for k, v := range state.Metadata {
		metadata[k] = v
	}

	// Copy tags
	tags := make([]string, len(state.Tags))
	copy(tags, state.Tags)

	return &VirtualNodeState{
		ID:          state.ID,
		Name:        state.Name,
		Type:        state.Type,
		Role:        state.Role,
		Position:    state.Position,
		Enabled:     state.Enabled,
		CreatedAt:   state.CreatedAt,
		UpdatedAt:   state.UpdatedAt,
		APBSSID:     state.APBSSID,
		APChannel:   state.APChannel,
		Description: state.Description,
		Tags:        tags,
		Metadata:    metadata,
	}
}

// VirtualNodeSummary provides a summary of virtual nodes in the space
type VirtualNodeSummary struct {
	TotalCount      int               `json:"total_count"`
	EnabledCount    int               `json:"enabled_count"`
	VirtualCount    int               `json:"virtual_count"`
	APCount         int               `json:"ap_count"`
	ByType          map[string]int    `json:"by_type"`
	ByTag           map[string]int    `json:"by_tag"`
	BoundingBox     BoundingBox       `json:"bounding_box"`
	FirstCreated    *time.Time        `json:"first_created,omitempty"`
	LastUpdated     *time.Time        `json:"last_updated,omitempty"`
}

// BoundingBox represents the axis-aligned bounding box of all nodes
type BoundingBox struct {
	MinX, MinY, MinZ float64
	MaxX, MaxY, MaxZ float64
}

// Summary returns a summary of all nodes in the store
func (s *VirtualNodeStore) Summary() *VirtualNodeSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	summary := &VirtualNodeSummary{
		ByType:      make(map[string]int),
		ByTag:       make(map[string]int),
		BoundingBox: BoundingBox{MinX: 1e9, MinY: 1e9, MinZ: 1e9, MaxX: -1e9, MaxY: -1e9, MaxZ: -1e9},
	}

	var firstCreated, lastUpdated time.Time

	for _, state := range s.nodes {
		summary.TotalCount++
		summary.ByType[string(state.Type)]++

		if state.Enabled {
			summary.EnabledCount++
		}

		if state.Type == NodeTypeVirtual {
			summary.VirtualCount++
		}

		if state.Type == NodeTypeAP {
			summary.APCount++
		}

		for _, tag := range state.Tags {
			summary.ByTag[tag]++
		}

		// Update bounding box
		if state.Position.X < summary.BoundingBox.MinX {
			summary.BoundingBox.MinX = state.Position.X
		}
		if state.Position.X > summary.BoundingBox.MaxX {
			summary.BoundingBox.MaxX = state.Position.X
		}
		if state.Position.Y < summary.BoundingBox.MinY {
			summary.BoundingBox.MinY = state.Position.Y
		}
		if state.Position.Y > summary.BoundingBox.MaxY {
			summary.BoundingBox.MaxY = state.Position.Y
		}
		if state.Position.Z < summary.BoundingBox.MinZ {
			summary.BoundingBox.MinZ = state.Position.Z
		}
		if state.Position.Z > summary.BoundingBox.MaxZ {
			summary.BoundingBox.MaxZ = state.Position.Z
		}

		// Track timestamps
		if firstCreated.IsZero() || state.CreatedAt.Before(firstCreated) {
			firstCreated = state.CreatedAt
		}
		if lastUpdated.IsZero() || state.UpdatedAt.After(lastUpdated) {
			lastUpdated = state.UpdatedAt
		}
	}

	if !firstCreated.IsZero() {
		summary.FirstCreated = &firstCreated
	}
	if !lastUpdated.IsZero() {
		summary.LastUpdated = &lastUpdated
	}

	// Handle empty case
	if summary.TotalCount == 0 {
		summary.BoundingBox = BoundingBox{}
	}

	return summary
}
