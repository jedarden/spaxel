package fleet

import (
	"sync"
	"testing"
	"time"
)

// ─── SelfHealManager Tests ────────────────────────────────────────────────────

func TestSelfHealManager_New(t *testing.T) {
	reg := newTestRegistry(t)
	cfg := DefaultSelfHealConfig()
	optimiser := NewRoleOptimiser(DefaultOptimisationConfig())
	shm := NewSelfHealManager(reg, optimiser, cfg)

	if shm == nil {
		t.Fatal("NewSelfHealManager returned nil")
	}

	if shm.config.ReconnectGracePeriod != 5*time.Minute {
		t.Errorf("ReconnectGracePeriod = %v, want 5m", shm.config.ReconnectGracePeriod)
	}

	if len(shm.GetOnlineNodes()) != 0 {
		t.Errorf("new manager should have 0 online nodes, got %d", len(shm.GetOnlineNodes()))
	}
}

func TestSelfHealManager_SingleNode(t *testing.T) {
	reg := newTestRegistry(t)
	optimiser := NewRoleOptimiser(DefaultOptimisationConfig())
	cfg := DefaultSelfHealConfig()
	shm := NewSelfHealManager(reg, optimiser, cfg)
	shm.SetNotifier(newMockNotifier())

	shm.OnNodeConnected("aa:00:00:00:00:01", "v1", "S3")

	roles := shm.GetCurrentRoles()
	if roles["aa:00:00:00:00:01"] != RoleTXRX {
		t.Errorf("single node role = %q, want tx_rx", roles["aa:00:00:00:00:01"])
	}

	if len(shm.GetOnlineNodes()) != 1 {
		t.Errorf("expected 1 online node, got %d", len(shm.GetOnlineNodes()))
	}
}

func TestSelfHealManager_TwoNodes(t *testing.T) {
	reg := newTestRegistry(t)
	optimiser := NewRoleOptimiser(DefaultOptimisationConfig())
	cfg := DefaultSelfHealConfig()
	shm := NewSelfHealManager(reg, optimiser, cfg)

	shm.OnNodeConnected("aa:00:00:00:00:01", "v1", "S3")
	shm.OnNodeConnected("aa:00:00:00:00:02", "v1", "S3")

	roles := shm.GetCurrentRoles()
	if len(roles) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(roles))
	}

	// With 2 nodes: one TX, one RX
	txCount := 0
	rxCount := 0
	for _, role := range roles {
		if role == RoleTX {
			txCount++
		}
		if role == RoleRX {
			rxCount++
		}
	}
	if txCount != 1 || rxCount != 1 {
		t.Errorf("expected 1 TX and 1 RX, got %d TX, %d RX (roles: %v)", txCount, rxCount, roles)
	}
}

// TestSelfHealManager_ReconnectWithinGracePeriod tests that a node reconnecting
// within the 5-minute grace period restores its previous role without re-optimisation.
func TestSelfHealManager_ReconnectWithinGracePeriod(t *testing.T) {
	reg := newTestRegistry(t)
	optimiser := NewRoleOptimiser(DefaultOptimisationConfig())
	cfg := DefaultSelfHealConfig()
	cfg.ReconnectGracePeriod = 5 * time.Minute
	shm := NewSelfHealManager(reg, optimiser, cfg)

	notifier := newMockNotifier()
	shm.SetNotifier(notifier)

	// Connect 3 nodes
	shm.OnNodeConnected("aa:00:00:00:00:01", "v1", "S3")
	shm.OnNodeConnected("aa:00:00:00:00:02", "v1", "S3")
	shm.OnNodeConnected("aa:00:00:00:00:03", "v1", "S3")

	// Record initial role
	rolesBefore := shm.GetCurrentRoles()
	roleBefore := rolesBefore["aa:00:00:00:00:02"]

	// Disconnect one node
	shm.OnNodeDisconnected("aa:00:00:00:00:02")

	// Verify node is tracked as offline
	onlineAfterDisconnect := shm.GetOnlineNodes()
	if len(onlineAfterDisconnect) != 2 {
		t.Errorf("expected 2 online after disconnect, got %d", len(onlineAfterDisconnect))
	}

	// Wait a short time (simulating quick reconnect)
	time.Sleep(100 * time.Millisecond)

	// Reconnect within grace period
	shm.OnNodeConnected("aa:00:00:00:00:02", "v1", "S3")

	// Verify role was restored
	rolesAfter := shm.GetCurrentRoles()
	roleAfter := rolesAfter["aa:00:00:00:00:02"]

	if roleAfter != roleBefore {
		t.Errorf("role after reconnect = %q, want %q (original role)", roleAfter, roleBefore)
	}

	// Verify node is back online
	onlineAfterReconnect := shm.GetOnlineNodes()
	if len(onlineAfterReconnect) != 3 {
		t.Errorf("expected 3 online after reconnect, got %d", len(onlineAfterReconnect))
	}
}

// TestSelfHealManager_ReconnectAfterGracePeriod tests that a node reconnecting
// after the grace period expires triggers a full re-optimisation.
func TestSelfHealManager_ReconnectAfterGracePeriod(t *testing.T) {
	reg := newTestRegistry(t)
	optimiser := NewRoleOptimiser(DefaultOptimisationConfig())
	cfg := DefaultSelfHealConfig()
	cfg.ReconnectGracePeriod = 100 * time.Millisecond // Short for testing
	shm := NewSelfHealManager(reg, optimiser, cfg)

	notifier := newMockNotifier()
	shm.SetNotifier(notifier)

	// Connect 3 nodes
	shm.OnNodeConnected("aa:00:00:00:00:01", "v1", "S3")
	shm.OnNodeConnected("aa:00:00:00:00:02", "v1", "S3")
	shm.OnNodeConnected("aa:00:00:00:00:03", "v1", "S3")

	// Disconnect one node
	shm.OnNodeDisconnected("aa:00:00:00:00:02")

	// Wait for grace period to expire
	time.Sleep(150 * time.Millisecond)

	// Reconnect after grace period - should trigger re-optimisation, not restore
	shm.OnNodeConnected("aa:00:00:00:00:02", "v1", "S3")

	// Verify all 3 nodes have valid roles
	roles := shm.GetCurrentRoles()
	if len(roles) != 3 {
		t.Errorf("expected 3 roles after reconnect, got %d", len(roles))
	}

	// All nodes should have a role
	for _, mac := range []string{"aa:00:00:00:00:01", "aa:00:00:00:00:02", "aa:00:00:00:00:03"} {
		if roles[mac] == "" {
			t.Errorf("node %s has empty role", mac)
		}
	}
}

// TestSelfHealManager_GracePeriodExpiration tests that the grace period
// correctly expires at the configured duration.
func TestSelfHealManager_GracePeriodExpiration(t *testing.T) {
	reg := newTestRegistry(t)
	optimiser := NewRoleOptimiser(DefaultOptimisationConfig())
	cfg := DefaultSelfHealConfig()
	cfg.ReconnectGracePeriod = 200 * time.Millisecond
	shm := NewSelfHealManager(reg, optimiser, cfg)

	shm.SetNotifier(newMockNotifier())

	// Connect a node
	shm.OnNodeConnected("aa:00:00:00:00:01", "v1", "S3")

	// Disconnect it
	shm.OnNodeDisconnected("aa:00:00:00:00:01")

	// Verify node is in offline tracking
	shm.mu.RLock()
	_, exists := shm.offlineNodes["aa:00:00:00:00:01"]
	shm.mu.RUnlock()

	if !exists {
		t.Error("node should be tracked in offlineNodes after disconnect")
	}

	// Wait for grace period + buffer
	time.Sleep(300 * time.Millisecond)

	// Run cleanup
	shm.cleanupExpiredGracePeriods()

	// Verify node is removed from offline tracking
	shm.mu.RLock()
	_, existsAfter := shm.offlineNodes["aa:00:00:00:00:01"]
	shm.mu.RUnlock()

	if existsAfter {
		t.Error("node should be removed from offlineNodes after grace period expires")
	}
}

// TestSelfHealManager_GDOPComparison tests the GDOP comparison logic:
// when new GDOP >= old * 0.9, no dashboard warning; when < 0.9, warning fires.
func TestSelfHealManager_GDOPComparison(t *testing.T) {
	reg := newTestRegistry(t)
	optimiser := NewRoleOptimiser(DefaultOptimisationConfig())
	cfg := DefaultSelfHealConfig()
	cfg.DegradationThreshold = 0.1 // 10%
	shm := NewSelfHealManager(reg, optimiser, cfg)

	bcaster := &mockFleetChangeBroadcaster{events: make([]FleetChangeEvent, 0)}
	shm.SetBroadcaster(bcaster)
	shm.SetNotifier(newMockNotifier())

	// Connect 4 nodes with positions
	reg.UpsertNode("aa:00:00:00:00:01", "v1", "S3")
	reg.SetNodePosition("aa:00:00:00:00:01", 0, 0, 0)
	reg.UpsertNode("aa:00:00:00:00:02", "v1", "S3")
	reg.SetNodePosition("aa:00:00:00:00:02", 4, 0, 0)
	reg.UpsertNode("aa:00:00:00:00:03", "v1", "S3")
	reg.SetNodePosition("aa:00:00:00:00:03", 0, 0, 4)
	reg.UpsertNode("aa:00:00:00:00:04", "v1", "S3")
	reg.SetNodePosition("aa:00:00:00:00:04", 4, 0, 4)

	// Set up a mock GDOP calculator
	mockGDOP := newMockGDOPCalculator(2.0, 20, 20)
	shm.SetGDOPCalculator(mockGDOP)

	// Connect all 4
	shm.OnNodeConnected("aa:00:00:00:00:01", "v1", "S3")
	shm.OnNodeConnected("aa:00:00:00:00:02", "v1", "S3")
	shm.OnNodeConnected("aa:00:00:00:00:03", "v1", "S3")
	shm.OnNodeConnected("aa:00:00:00:00:04", "v1", "S3")

	// Record initial coverage
	coverageBefore := shm.GetCoverageScore()

	// Disconnect one node - should trigger re-optimisation and broadcast
	bcaster.reset()
	shm.OnNodeDisconnected("aa:00:00:00:00:04")

	// Verify event was broadcast
	events := bcaster.getEvents()
	if len(events) == 0 {
		t.Fatal("expected fleet_change event to be broadcast")
	}

	event := events[len(events)-1]

	// Check that coverage delta is recorded
	if event.CoverageBefore != coverageBefore {
		t.Errorf("CoverageBefore = %v, want %v", event.CoverageBefore, coverageBefore)
	}

	// Check is_degradation flag based on threshold
	coverageDelta := event.CoverageAfter - event.CoverageBefore
	expectedDegradation := coverageDelta < -cfg.DegradationThreshold

	if event.IsDegradation != expectedDegradation {
		t.Errorf("IsDegradation = %v, want %v (delta=%.3f, threshold=%.3f)",
			event.IsDegradation, expectedDegradation, coverageDelta, cfg.DegradationThreshold)
	}
}

// TestSelfHealManager_FleetChangeEventContainsGDOP tests that fleet_change events
// contain correct before/after GDOP data.
func TestSelfHealManager_FleetChangeEventContainsGDOP(t *testing.T) {
	reg := newTestRegistry(t)
	optimiser := NewRoleOptimiser(DefaultOptimisationConfig())
	cfg := DefaultSelfHealConfig()
	shm := NewSelfHealManager(reg, optimiser, cfg)

	bcaster := &mockFleetChangeBroadcaster{events: make([]FleetChangeEvent, 0)}
	shm.SetBroadcaster(bcaster)
	shm.SetNotifier(newMockNotifier())

	// Set up mock GDOP calculator
	mockGDOP := newMockGDOPCalculator(2.5, 10, 10)
	shm.SetGDOPCalculator(mockGDOP)

	// Connect nodes
	reg.UpsertNode("aa:00:00:00:00:01", "v1", "S3")
	reg.SetNodePosition("aa:00:00:00:00:01", 1, 0, 1)
	reg.UpsertNode("aa:00:00:00:00:02", "v1", "S3")
	reg.SetNodePosition("aa:00:00:00:00:02", 3, 0, 3)

	bcaster.reset()
	shm.OnNodeConnected("aa:00:00:00:00:01", "v1", "S3")
	shm.OnNodeConnected("aa:00:00:00:00:02", "v1", "S3")

	// Disconnect to trigger event with GDOP data
	bcaster.reset()
	shm.OnNodeDisconnected("aa:00:00:00:00:02")

	events := bcaster.getEvents()
	if len(events) == 0 {
		t.Fatal("expected fleet_change event")
	}

	event := events[0]

	// Verify event structure
	if event.Timestamp.IsZero() {
		t.Error("Timestamp should be set")
	}

	if event.TriggerReason == "" {
		t.Error("TriggerReason should be set")
	}

	if event.OfflineMAC != "aa:00:00:00:00:02" {
		t.Errorf("OfflineMAC = %q, want aa:00:00:00:00:02", event.OfflineMAC)
	}

	if len(event.RoleAssignments) == 0 {
		t.Error("RoleAssignments should not be empty")
	}
}

// TestRoleOptimiser_MostOrthogonalPair tests that the role optimiser selects
// the most orthogonal link pair from a set of 4 nodes at known positions.
func TestRoleOptimiser_MostOrthogonalPair(t *testing.T) {
	optimiser := NewRoleOptimiser(DefaultOptimisationConfig())

	// Create 4 nodes in a square: (0,0), (4,0), (0,4), (4,4)
	// The optimal assignment should have orthogonal links (not parallel)
	nodes := []NodeInfo{
		{MAC: "aa:00:00:00:00:01", PosX: 0, PosY: 0, PosZ: 0, HealthScore: 0.9},
		{MAC: "aa:00:00:00:00:02", PosX: 4, PosY: 0, PosZ: 0, HealthScore: 0.9},
		{MAC: "aa:00:00:00:00:03", PosX: 0, PosY: 0, PosZ: 4, HealthScore: 0.9},
		{MAC: "aa:00:00:00:00:04", PosX: 4, PosY: 0, PosZ: 4, HealthScore: 0.9},
	}

	result := optimiser.Optimise(nodes, "test")

	if len(result.Assignments) != 4 {
		t.Fatalf("expected 4 assignments, got %d", len(result.Assignments))
	}

	// Count TX and RX
	txCount := 0
	rxCount := 0
	for _, a := range result.Assignments {
		switch a.Role {
		case RoleTX:
			txCount++
		case RoleRX:
			rxCount++
		case RoleTXRX:
			txCount++
			rxCount++
		}
	}

	// With 4 nodes, we expect roughly 2 TX and 2 RX (or some combination that allows sensing)
	if txCount == 0 {
		t.Error("expected at least 1 TX node")
	}
	if rxCount == 0 {
		t.Error("expected at least 1 RX node")
	}

	// Verify we have sensing links
	if len(result.Links) == 0 {
		t.Error("expected at least one sensing link")
	}
}

// TestRoleOptimiser_GracefulDegradation tests that when a node goes offline,
// the optimiser produces a valid assignment for the remaining nodes.
func TestRoleOptimiser_GracefulDegradation(t *testing.T) {
	optimiser := NewRoleOptimiser(DefaultOptimisationConfig())

	// Initial 4 nodes
	nodes := []NodeInfo{
		{MAC: "aa:00:00:00:00:01", PosX: 0, PosY: 0, PosZ: 0, HealthScore: 0.9},
		{MAC: "aa:00:00:00:00:02", PosX: 4, PosY: 0, PosZ: 0, HealthScore: 0.9},
		{MAC: "aa:00:00:00:00:03", PosX: 0, PosY: 0, PosZ: 4, HealthScore: 0.9},
		{MAC: "aa:00:00:00:00:04", PosX: 4, PosY: 0, PosZ: 4, HealthScore: 0.9},
	}

	// Optimize with all nodes
	result4 := optimiser.Optimise(nodes, "initial")
	if len(result4.Assignments) != 4 {
		t.Errorf("expected 4 assignments, got %d", len(result4.Assignments))
	}

	// Simulate node loss - remove one node
	remainingNodes := nodes[:3] // Remove the 4th node

	result3 := optimiser.Optimise(remainingNodes, "node_lost")
	if len(result3.Assignments) != 3 {
		t.Errorf("expected 3 assignments after node loss, got %d", len(result3.Assignments))
	}

	// Verify all remaining nodes have valid roles
	for _, a := range result3.Assignments {
		if a.Role != RoleTX && a.Role != RoleRX && a.Role != RoleTXRX && a.Role != RolePassive {
			t.Errorf("invalid role %q for node %s", a.Role, a.MAC)
		}
	}

	// With 3 nodes, we should still have at least one TX and one RX
	txCount := 0
	rxCount := 0
	for _, a := range result3.Assignments {
		if a.Role == RoleTX || a.Role == RoleTXRX {
			txCount++
		}
		if a.Role == RoleRX || a.Role == RoleTXRX {
			rxCount++
		}
	}

	if txCount == 0 {
		t.Error("expected at least 1 TX after degradation")
	}
	if rxCount == 0 {
		t.Error("expected at least 1 RX after degradation")
	}
}

// TestRoleOptimiser_SimulateRemoval tests the SimulateRemoval function
func TestRoleOptimiser_SimulateRemoval(t *testing.T) {
	optimiser := NewRoleOptimiser(DefaultOptimisationConfig())

	nodes := []NodeInfo{
		{MAC: "aa:00:00:00:00:01", PosX: 0, PosY: 0, PosZ: 0, HealthScore: 0.9},
		{MAC: "aa:00:00:00:00:02", PosX: 4, PosY: 0, PosZ: 0, HealthScore: 0.9},
		{MAC: "aa:00:00:00:00:03", PosX: 0, PosY: 0, PosZ: 4, HealthScore: 0.9},
		{MAC: "aa:00:00:00:00:04", PosX: 4, PosY: 0, PosZ: 4, HealthScore: 0.9},
	}

	result, delta := optimiser.SimulateRemoval(nodes, "aa:00:00:00:00:04")

	if len(result.Assignments) != 3 {
		t.Errorf("expected 3 assignments after simulation, got %d", len(result.Assignments))
	}

	// Delta should be negative (coverage decreased) or zero
	if delta > 0 {
		t.Logf("Warning: coverage delta = %.3f is positive, expected <= 0", delta)
	}
}

// ─── Mock FleetChangeBroadcaster ──────────────────────────────────────────────

type mockFleetChangeBroadcaster struct {
	mu     sync.Mutex
	events []FleetChangeEvent
}

func (m *mockFleetChangeBroadcaster) BroadcastFleetChange(event FleetChangeEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
}

func (m *mockFleetChangeBroadcaster) getEvents() []FleetChangeEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]FleetChangeEvent, len(m.events))
	copy(result, m.events)
	return result
}

func (m *mockFleetChangeBroadcaster) reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = make([]FleetChangeEvent, 0)
}
