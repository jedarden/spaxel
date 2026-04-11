package simulator

import (
	"os"
	"testing"
)

// Test helper to create a temporary store
func tempStore(t *testing.T) (*VirtualNodeStore, string) {
	t.Helper()

	tmpDir := t.TempDir()
	space := DefaultSpace()

	store, err := NewVirtualNodeStore(StoreConfig{
		DataDir: tmpDir,
		Space:   space,
	})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	return store, tmpDir
}

// TestNewVirtualNodeStore tests store creation
func TestNewVirtualNodeStore(t *testing.T) {
	store, tmpDir := tempStore(t)
	defer store.Close()

	// Check that data directory was created
	if _, err := os.Stat(tmpDir); err != nil {
		t.Errorf("Data directory not created: %v", err)
	}

	// Store should start empty
	if store.Count() != 0 {
		t.Errorf("New store should be empty, got %d nodes", store.Count())
	}

	// Check space
	space := store.GetSpace()
	if space == nil {
		t.Error("Space should not be nil")
	}
	if space.ID != "default" {
		t.Errorf("Expected space ID 'default', got '%s'", space.ID)
	}
}

// TestVirtualNodeStore_CreateNode tests basic node creation
func TestVirtualNodeStore_CreateNode(t *testing.T) {
	store, _ := tempStore(t)
	defer store.Close()

	// Create a virtual node
	position := NewPoint(1.0, 2.0, 1.5)
	state, err := store.CreateVirtualNode("node-1", "Test Node", position)

	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	// Verify state
	if state.ID != "node-1" {
		t.Errorf("Expected ID 'node-1', got '%s'", state.ID)
	}
	if state.Name != "Test Node" {
		t.Errorf("Expected name 'Test Node', got '%s'", state.Name)
	}
	if state.Type != NodeTypeVirtual {
		t.Errorf("Expected type '%s', got '%s'", NodeTypeVirtual, state.Type)
	}
	if state.Role != RoleTXRX {
		t.Errorf("Expected default role '%s', got '%s'", RoleTXRX, state.Role)
	}
	if !state.Enabled {
		t.Error("New node should be enabled")
	}

	// Verify position
	if state.Position.X != 1.0 || state.Position.Y != 2.0 || state.Position.Z != 1.5 {
		t.Errorf("Position mismatch: got (%f, %f, %f)",
			state.Position.X, state.Position.Y, state.Position.Z)
	}

	// Verify timestamps
	if state.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if state.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should not be zero")
	}

	// Verify node count
	if store.Count() != 1 {
		t.Errorf("Expected 1 node, got %d", store.Count())
	}
}

// TestVirtualNodeStore_CreateAPNode tests AP node creation
func TestVirtualNodeStore_CreateAPNode(t *testing.T) {
	store, _ := tempStore(t)
	defer store.Close()

	position := NewPoint(0, 0, 2.5)
	state, err := store.CreateAPNode("ap-1", "Router", "AA:BB:CC:DD:EE:FF", 6, position)

	if err != nil {
		t.Fatalf("Failed to create AP node: %v", err)
	}

	// Verify AP-specific fields
	if state.Type != NodeTypeAP {
		t.Errorf("Expected type '%s', got '%s'", NodeTypeAP, state.Type)
	}
	if state.Role != RoleTX {
		t.Errorf("AP should have TX role, got '%s'", state.Role)
	}
	if state.APBSSID != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("Expected BSSID 'AA:BB:CC:DD:EE:FF', got '%s'", state.APBSSID)
	}
	if state.APChannel != 6 {
		t.Errorf("Expected channel 6, got %d", state.APChannel)
	}
}

// TestVirtualNodeStore_DuplicateID tests duplicate node ID rejection
func TestVirtualNodeStore_DuplicateID(t *testing.T) {
	store, _ := tempStore(t)
	defer store.Close()

	position := NewPoint(1.0, 2.0, 1.5)
	_, err := store.CreateVirtualNode("node-1", "First", position)
	if err != nil {
		t.Fatalf("Failed to create first node: %v", err)
	}

	// Try to create with same ID
	_, err = store.CreateVirtualNode("node-1", "Second", NewPoint(2.0, 3.0, 1.0))
	if err == nil {
		t.Error("Expected error when creating duplicate node ID")
	}
}

// TestVirtualNodeStore_InvalidPosition tests position validation
func TestVirtualNodeStore_InvalidPosition(t *testing.T) {
	store, _ := tempStore(t)
	defer store.Close()

	// Position outside space bounds (default space is 6x5x2.5)
	invalidPos := NewPoint(10.0, 10.0, 10.0)
	_, err := store.CreateVirtualNode("node-1", "Invalid", invalidPos)

	if err == nil {
		t.Error("Expected error for position outside space bounds")
	}
}

// TestVirtualNodeStore_GetNode tests node retrieval
func TestVirtualNodeStore_GetNode(t *testing.T) {
	store, _ := tempStore(t)
	defer store.Close()

	position := NewPoint(1.0, 2.0, 1.5)
	_, err := store.CreateVirtualNode("node-1", "Test Node", position)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	// Get existing node
	state, err := store.GetNode("node-1")
	if err != nil {
		t.Fatalf("Failed to get node: %v", err)
	}

	if state.Name != "Test Node" {
		t.Errorf("Expected name 'Test Node', got '%s'", state.Name)
	}

	// Get non-existent node
	_, err = store.GetNode("non-existent")
	if err == nil {
		t.Error("Expected error for non-existent node")
	}
}

// TestVirtualNodeStore_UpdateNodePosition tests position updates
func TestVirtualNodeStore_UpdateNodePosition(t *testing.T) {
	store, _ := tempStore(t)
	defer store.Close()

	position := NewPoint(1.0, 2.0, 1.5)
	_, err := store.CreateVirtualNode("node-1", "Test Node", position)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	// Update position
	newPos := NewPoint(3.0, 4.0, 2.0)
	err = store.UpdateNodePosition("node-1", newPos)
	if err != nil {
		t.Fatalf("Failed to update position: %v", err)
	}

	// Verify update
	state, _ := store.GetNode("node-1")
	if state.Position.X != 3.0 || state.Position.Y != 4.0 || state.Position.Z != 2.0 {
		t.Errorf("Position not updated: got (%f, %f, %f)",
			state.Position.X, state.Position.Y, state.Position.Z)
	}

	// Try invalid position
	invalidPos := NewPoint(100.0, 100.0, 100.0)
	err = store.UpdateNodePosition("node-1", invalidPos)
	if err == nil {
		t.Error("Expected error for invalid position update")
	}
}

// TestVirtualNodeStore_UpdateNodeRole tests role updates
func TestVirtualNodeStore_UpdateNodeRole(t *testing.T) {
	store, _ := tempStore(t)
	defer store.Close()

	position := NewPoint(1.0, 2.0, 1.5)
	_, err := store.CreateVirtualNode("node-1", "Test Node", position)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	// Update role
	err = store.UpdateNodeRole("node-1", RoleRX)
	if err != nil {
		t.Fatalf("Failed to update role: %v", err)
	}

	// Verify update
	state, _ := store.GetNode("node-1")
	if state.Role != RoleRX {
		t.Errorf("Expected role '%s', got '%s'", RoleRX, state.Role)
	}
}

// TestVirtualNodeStore_SetNodeEnabled tests enable/disable
func TestVirtualNodeStore_SetNodeEnabled(t *testing.T) {
	store, _ := tempStore(t)
	defer store.Close()

	position := NewPoint(1.0, 2.0, 1.5)
	_, err := store.CreateVirtualNode("node-1", "Test Node", position)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	// Disable node
	err = store.SetNodeEnabled("node-1", false)
	if err != nil {
		t.Fatalf("Failed to disable node: %v", err)
	}

	state, _ := store.GetNode("node-1")
	if state.Enabled {
		t.Error("Node should be disabled")
	}

	// Re-enable
	err = store.SetNodeEnabled("node-1", true)
	if err != nil {
		t.Fatalf("Failed to enable node: %v", err)
	}

	state, _ = store.GetNode("node-1")
	if !state.Enabled {
		t.Error("Node should be enabled")
	}
}

// TestVirtualNodeStore_UpdateNodeMetadata tests metadata updates
func TestVirtualNodeStore_UpdateNodeMetadata(t *testing.T) {
	store, _ := tempStore(t)
	defer store.Close()

	position := NewPoint(1.0, 2.0, 1.5)
	_, err := store.CreateVirtualNode("node-1", "Test Node", position)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	// Update metadata
	metadata := map[string]interface{}{
		"location":  "kitchen",
		"priority":  1,
		"notes":     "Near window",
		"installed": "2024-01-15",
	}
	err = store.UpdateNodeMetadata("node-1", metadata)
	if err != nil {
		t.Fatalf("Failed to update metadata: %v", err)
	}

	// Verify metadata
	state, _ := store.GetNode("node-1")
	if state.Metadata["location"] != "kitchen" {
		t.Errorf("Metadata not updated: expected 'kitchen', got '%v'", state.Metadata["location"])
	}
	if state.Metadata["priority"] != 1 {
		t.Errorf("Priority metadata incorrect: expected 1, got %v", state.Metadata["priority"])
	}
}

// TestVirtualNodeStore_Tags tests tag management
func TestVirtualNodeStore_Tags(t *testing.T) {
	store, _ := tempStore(t)
	defer store.Close()

	position := NewPoint(1.0, 2.0, 1.5)
	_, err := store.CreateVirtualNode("node-1", "Test Node", position)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	// Add tags
	tags := []string{"kitchen", "window", "testing"}
	for _, tag := range tags {
		if err := store.AddTag("node-1", tag); err != nil {
			t.Fatalf("Failed to add tag '%s': %v", tag, err)
		}
	}

	// Verify tags
	state, _ := store.GetNode("node-1")
	if len(state.Tags) != 3 {
		t.Errorf("Expected 3 tags, got %d", len(state.Tags))
	}

	// Add duplicate tag (should not duplicate)
	if err := store.AddTag("node-1", "kitchen"); err != nil {
		t.Fatalf("Failed to add duplicate tag: %v", err)
	}

	state, _ = store.GetNode("node-1")
	if len(state.Tags) != 3 {
		t.Errorf("Duplicate tag should not increase count: got %d", len(state.Tags))
	}

	// Remove tag
	if err := store.RemoveTag("node-1", "window"); err != nil {
		t.Fatalf("Failed to remove tag: %v", err)
	}

	state, _ = store.GetNode("node-1")
	if len(state.Tags) != 2 {
		t.Errorf("Expected 2 tags after removal, got %d", len(state.Tags))
	}

	// Remove non-existent tag (should be no-op)
	if err := store.RemoveTag("node-1", "nonexistent"); err != nil {
		t.Error("Removing non-existent tag should not error")
	}
}

// TestVirtualNodeStore_DeleteNode tests node deletion
func TestVirtualNodeStore_DeleteNode(t *testing.T) {
	store, _ := tempStore(t)
	defer store.Close()

	position := NewPoint(1.0, 2.0, 1.5)
	_, err := store.CreateVirtualNode("node-1", "Test Node", position)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	// Delete node
	err = store.DeleteNode("node-1")
	if err != nil {
		t.Fatalf("Failed to delete node: %v", err)
	}

	// Verify deletion
	if store.Count() != 0 {
		t.Errorf("Expected 0 nodes after deletion, got %d", store.Count())
	}

	_, err = store.GetNode("node-1")
	if err == nil {
		t.Error("Expected error when getting deleted node")
	}

	// Delete non-existent node
	err = store.DeleteNode("non-existent")
	if err == nil {
		t.Error("Expected error when deleting non-existent node")
	}
}

// TestVirtualNodeStore_ListNodes tests listing nodes
func TestVirtualNodeStore_ListNodes(t *testing.T) {
	store, _ := tempStore(t)
	defer store.Close()

	// Create multiple nodes
	for i := 1; i <= 5; i++ {
		position := NewPoint(float64(i), float64(i), 1.5)
		_, err := store.CreateVirtualNode(
			string(rune('0'+i)),
			string(rune('A'+i)),
			position,
		)
		if err != nil {
			t.Fatalf("Failed to create node %d: %v", i, err)
		}
	}

	// List all nodes
	allNodes := store.ListNodes()
	if len(allNodes) != 5 {
		t.Errorf("Expected 5 nodes, got %d", len(allNodes))
	}

	// Disable one node
	if err := store.SetNodeEnabled("3", false); err != nil {
		t.Fatalf("Failed to disable node: %v", err)
	}

	// List enabled nodes
	enabledNodes := store.ListEnabledNodes()
	if len(enabledNodes) != 4 {
		t.Errorf("Expected 4 enabled nodes, got %d", len(enabledNodes))
	}
}

// TestVirtualNodeStore_ListNodesByType tests filtering by type
func TestVirtualNodeStore_ListNodesByType(t *testing.T) {
	store, _ := tempStore(t)
	defer store.Close()

	// Create virtual nodes
	for i := 1; i <= 3; i++ {
		position := NewPoint(float64(i), 0, 1.5)
		_, err := store.CreateVirtualNode(
			string(rune('0'+i)),
			string(rune('A'+i)),
			position,
		)
		if err != nil {
			t.Fatalf("Failed to create virtual node: %v", err)
		}
	}

	// Create AP node
	position := NewPoint(0, 0, 2.5)
	_, err := store.CreateAPNode("ap-1", "Router", "AA:BB:CC:DD:EE:FF", 6, position)
	if err != nil {
		t.Fatalf("Failed to create AP node: %v", err)
	}

	// List virtual nodes
	virtualNodes := store.ListNodesByType(NodeTypeVirtual)
	if len(virtualNodes) != 3 {
		t.Errorf("Expected 3 virtual nodes, got %d", len(virtualNodes))
	}

	// List AP nodes
	apNodes := store.ListNodesByType(NodeTypeAP)
	if len(apNodes) != 1 {
		t.Errorf("Expected 1 AP node, got %d", len(apNodes))
	}
}

// TestVirtualNodeStore_ListNodesByTag tests filtering by tag
func TestVirtualNodeStore_ListNodesByTag(t *testing.T) {
	store, _ := tempStore(t)
	defer store.Close()

	// Create nodes with different tags
	for i := 1; i <= 3; i++ {
		position := NewPoint(float64(i), 0, 1.5)
		_, err := store.CreateVirtualNode(
			string(rune('0'+i)),
			string(rune('A'+i)),
			position,
		)
		if err != nil {
			t.Fatalf("Failed to create node: %v", err)
		}
	}

	// Tag first two nodes as "kitchen"
	store.AddTag("1", "kitchen")
	store.AddTag("2", "kitchen")
	store.AddTag("3", "bedroom")

	// List by tag
	kitchenNodes := store.ListNodesByTag("kitchen")
	if len(kitchenNodes) != 2 {
		t.Errorf("Expected 2 kitchen nodes, got %d", len(kitchenNodes))
	}

	bedroomNodes := store.ListNodesByTag("bedroom")
	if len(bedroomNodes) != 1 {
		t.Errorf("Expected 1 bedroom node, got %d", len(bedroomNodes))
	}
}

// TestVirtualNodeStore_Clear tests clearing all nodes
func TestVirtualNodeStore_Clear(t *testing.T) {
	store, _ := tempStore(t)
	defer store.Close()

	// Create some nodes
	for i := 1; i <= 3; i++ {
		position := NewPoint(float64(i), 0, 1.5)
		_, err := store.CreateVirtualNode(
			string(rune('0'+i)),
			string(rune('A'+i)),
			position,
		)
		if err != nil {
			t.Fatalf("Failed to create node: %v", err)
		}
	}

	// Clear all
	if err := store.Clear(); err != nil {
		t.Fatalf("Failed to clear store: %v", err)
	}

	if store.Count() != 0 {
		t.Errorf("Expected 0 nodes after clear, got %d", store.Count())
	}
}

// TestVirtualNodeStore_Persistence tests saving and loading
func TestVirtualNodeStore_Persistence(t *testing.T) {
	tmpDir := t.TempDir()

	// Create first store and add nodes
	space := DefaultSpace()
	store1, err := NewVirtualNodeStore(StoreConfig{
		DataDir: tmpDir,
		Space:   space,
	})
	if err != nil {
		t.Fatalf("Failed to create first store: %v", err)
	}

	position := NewPoint(1.0, 2.0, 1.5)
	_, err = store1.CreateVirtualNode("node-1", "Persisted Node", position)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	store1.AddTag("node-1", "persistent")
	store1.UpdateNodeMetadata("node-1", map[string]interface{}{
		"test": "value",
	})

	// Close first store
	if err := store1.Close(); err != nil {
		t.Fatalf("Failed to close store: %v", err)
	}

	// Create new store (should load from disk)
	store2, err := NewVirtualNodeStore(StoreConfig{
		DataDir: tmpDir,
		Space:   space,
	})
	if err != nil {
		t.Fatalf("Failed to create second store: %v", err)
	}
	defer store2.Close()

	// Verify loaded state
	if store2.Count() != 1 {
		t.Errorf("Expected 1 loaded node, got %d", store2.Count())
	}

	state, err := store2.GetNode("node-1")
	if err != nil {
		t.Fatalf("Failed to get loaded node: %v", err)
	}

	if state.Name != "Persisted Node" {
		t.Errorf("Expected name 'Persisted Node', got '%s'", state.Name)
	}

	if len(state.Tags) != 1 || state.Tags[0] != "persistent" {
		t.Errorf("Tags not persisted: got %v", state.Tags)
	}

	if state.Metadata["test"] != "value" {
		t.Errorf("Metadata not persisted: got %v", state.Metadata)
	}
}

// TestVirtualNodeStore_UpdateSpace tests space updates
func TestVirtualNodeStore_UpdateSpace(t *testing.T) {
	store, _ := tempStore(t)
	defer store.Close()

	// Create a node
	position := NewPoint(1.0, 2.0, 1.5)
	_, err := store.CreateVirtualNode("node-1", "Test Node", position)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	// Update space to smaller bounds
	newSpace := &Space{
		ID:   "smaller",
		Name: "Smaller Space",
		Rooms: []Room{{
			ID:    "room-1",
			Name:  "Small Room",
			MinX:  0, MinY: 0, MinZ: 0,
			MaxX:  1.5, MaxY: 1.5, MaxZ: 1.5,
		}},
	}

	err = store.UpdateSpace(newSpace)
	if err != nil {
		t.Fatalf("Failed to update space: %v", err)
	}

	// Node should still be within bounds
	state, _ := store.GetNode("node-1")
	if !state.Enabled {
		t.Error("Node within new bounds should remain enabled")
	}

	// Shrink space further (node now outside)
	tinySpace := &Space{
		ID:   "tiny",
		Name: "Tiny Space",
		Rooms: []Room{{
			ID:    "room-1",
			Name:  "Tiny Room",
			MinX:  0, MinY: 0, MinZ: 0,
			MaxX:  0.5, MaxY: 0.5, MaxZ: 0.5,
		}},
	}

	err = store.UpdateSpace(tinySpace)
	if err != nil {
		t.Fatalf("Failed to shrink space: %v", err)
	}

	// Node should now be disabled
	state, _ = store.GetNode("node-1")
	if state.Enabled {
		t.Error("Node outside new bounds should be disabled")
	}
}

// TestVirtualNodeStore_ToNodeSet tests conversion to NodeSet
func TestVirtualNodeStore_ToNodeSet(t *testing.T) {
	store, _ := tempStore(t)
	defer store.Close()

	// Create various nodes
	position := NewPoint(1.0, 2.0, 1.5)
	_, err := store.CreateVirtualNode("virtual-1", "Virtual Node", position)
	if err != nil {
		t.Fatalf("Failed to create virtual node: %v", err)
	}

	position = NewPoint(0, 0, 2.5)
	_, err = store.CreateAPNode("ap-1", "Router", "AA:BB:CC:DD:EE:FF", 6, position)
	if err != nil {
		t.Fatalf("Failed to create AP node: %v", err)
	}

	// Convert to NodeSet
	nodeSet := store.ToNodeSet()

	if nodeSet.Count() != 2 {
		t.Errorf("Expected 2 nodes in NodeSet, got %d", nodeSet.Count())
	}

	// Verify virtual node
	virtualNode := nodeSet.GetByID("virtual-1")
	if virtualNode == nil {
		t.Error("Virtual node not in NodeSet")
	} else if !virtualNode.IsVirtual() {
		t.Error("Node should be marked as virtual")
	}

	// Verify AP node
	apNode := nodeSet.GetByID("ap-1")
	if apNode == nil {
		t.Error("AP node not in NodeSet")
	} else if !apNode.IsAP() {
		t.Error("Node should be marked as AP")
	}
}

// TestVirtualNodeStore_ImportFromNodeSet tests importing from NodeSet
func TestVirtualNodeStore_ImportFromNodeSet(t *testing.T) {
	store, _ := tempStore(t)
	defer store.Close()

	// Create a NodeSet
	nodeSet := NewNodeSet()
	nodeSet.AddVirtualNode("import-1", "Imported Virtual", NewPoint(1.0, 2.0, 1.5))
	nodeSet.AddAPNode("import-2", "Imported AP", "BB:CC:DD:EE:FF:00", 11, NewPoint(0, 0, 2))

	// Import
	if err := store.ImportFromNodeSet(nodeSet); err != nil {
		t.Fatalf("Failed to import NodeSet: %v", err)
	}

	if store.Count() != 2 {
		t.Errorf("Expected 2 nodes after import, got %d", store.Count())
	}

	// Verify imported nodes
	state1, _ := store.GetNode("import-1")
	if state1.Name != "Imported Virtual" {
		t.Errorf("Expected name 'Imported Virtual', got '%s'", state1.Name)
	}

	state2, _ := store.GetNode("import-2")
	if state2.APBSSID != "BB:CC:DD:EE:FF:00" {
		t.Errorf("Expected BSSID 'BB:CC:DD:EE:FF:00', got '%s'", state2.APBSSID)
	}
}

// TestVirtualNodeStore_Summary tests summary generation
func TestVirtualNodeStore_Summary(t *testing.T) {
	store, _ := tempStore(t)
	defer store.Close()

	// Create various nodes
	store.CreateVirtualNode("node-1", "Node 1", NewPoint(1.0, 1.0, 1.5))
	store.CreateVirtualNode("node-2", "Node 2", NewPoint(3.0, 4.0, 2.0))
	store.CreateAPNode("ap-1", "Router", "AA:BB:CC:DD:EE:FF", 6, NewPoint(0, 0, 2.5))
	store.AddTag("node-1", "kitchen")
	store.AddTag("node-2", "kitchen")

	// Disable one node
	store.SetNodeEnabled("node-2", false)

	// Get summary
	summary := store.Summary()

	if summary.TotalCount != 3 {
		t.Errorf("Expected total count 3, got %d", summary.TotalCount)
	}

	if summary.EnabledCount != 2 {
		t.Errorf("Expected enabled count 2, got %d", summary.EnabledCount)
	}

	if summary.VirtualCount != 2 {
		t.Errorf("Expected virtual count 2, got %d", summary.VirtualCount)
	}

	if summary.APCount != 1 {
		t.Errorf("Expected AP count 1, got %d", summary.APCount)
	}

	if summary.ByType["virtual"] != 2 {
		t.Errorf("Expected 2 virtual nodes by type, got %d", summary.ByType["virtual"])
	}

	if summary.ByTag["kitchen"] != 2 {
		t.Errorf("Expected 2 nodes with kitchen tag, got %d", summary.ByTag["kitchen"])
	}

	// Check bounding box
	if summary.BoundingBox.MinX != 0 {
		t.Errorf("Expected min X 0, got %f", summary.BoundingBox.MinX)
	}
	if summary.BoundingBox.MaxX != 3.0 {
		t.Errorf("Expected max X 3.0, got %f", summary.BoundingBox.MaxX)
	}

	// Check timestamps
	if summary.FirstCreated == nil {
		t.Error("FirstCreated should not be nil")
	}
	if summary.LastUpdated == nil {
		t.Error("LastUpdated should not be nil")
	}
}

// TestVirtualNodeStore_Close tests store closing
func TestVirtualNodeStore_Close(t *testing.T) {
	store, _ := tempStore(t)

	// Create a node
	position := NewPoint(1.0, 2.0, 1.5)
	_, err := store.CreateVirtualNode("node-1", "Test Node", position)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	// Close store
	if err := store.Close(); err != nil {
		t.Fatalf("Failed to close store: %v", err)
	}

	// Operations should fail after close
	_, err = store.CreateVirtualNode("node-2", "Should Fail", NewPoint(2.0, 3.0, 1.5))
	if err == nil {
		t.Error("Expected error when creating node after close")
	}

	_, err = store.GetNode("node-1")
	if err == nil {
		t.Error("Expected error when getting node after close")
	}

	// Double close should be safe
	if err := store.Close(); err != nil {
		t.Errorf("Double close should be safe: %v", err)
	}
}

// TestVirtualNodeStore_Immutability tests that returned states are copies
func TestVirtualNodeStore_Immutability(t *testing.T) {
	store, _ := tempStore(t)
	defer store.Close()

	position := NewPoint(1.0, 2.0, 1.5)
	_, err := store.CreateVirtualNode("node-1", "Test Node", position)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	// Get node
	state1, err := store.GetNode("node-1")
	if err != nil {
		t.Fatalf("Failed to get node: %v", err)
	}

	// Modify returned state
	state1.Name = "Modified"
	state1.Position.X = 999.0
	state1.Metadata["test"] = "injected"

	// Get node again
	state2, err := store.GetNode("node-1")
	if err != nil {
		t.Fatalf("Failed to get node: %v", err)
	}

	// Should have original values
	if state2.Name == "Modified" {
		t.Error("Returned state should be a copy, modifications should not affect stored state")
	}

	if state2.Position.X == 999.0 {
		t.Error("Position modification should not affect stored state")
	}

	if _, exists := state2.Metadata["test"]; exists {
		t.Error("Metadata injection should not affect stored state")
	}

	// ListNodes should also return copies
	nodes := store.ListNodes()
	nodes[0].Name = "ListModified"

	state3, _ := store.GetNode("node-1")
	if state3.Name == "ListModified" {
		t.Error("ListNodes should return copies")
	}
}

// TestVirtualNodeStore_StateIsolation tests that each node's state is independent
func TestVirtualNodeStore_StateIsolation(t *testing.T) {
	store, _ := tempStore(t)
	defer store.Close()

	// Create two nodes
	_, err := store.CreateVirtualNode("node-1", "Node 1", NewPoint(1.0, 1.0, 1.5))
	if err != nil {
		t.Fatalf("Failed to create node 1: %v", err)
	}

	_, err = store.CreateVirtualNode("node-2", "Node 2", NewPoint(3.0, 3.0, 1.5))
	if err != nil {
		t.Fatalf("Failed to create node 2: %v", err)
	}

	// Add tag to node 1
	store.AddTag("node-1", "tag1")

	// Add tag to node 2
	store.AddTag("node-2", "tag2")

	// Verify isolation
	state1, _ := store.GetNode("node-1")
	state2, _ := store.GetNode("node-2")

	if len(state1.Tags) != 1 || state1.Tags[0] != "tag1" {
		t.Errorf("Node 1 tags incorrect: %v", state1.Tags)
	}

	if len(state2.Tags) != 1 || state2.Tags[0] != "tag2" {
		t.Errorf("Node 2 tags incorrect: %v", state2.Tags)
	}
}
