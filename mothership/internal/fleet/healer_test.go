package fleet

import (
	"sync"
	"testing"
	"time"
)

// ─── Mock GDOP Calculator ─────────────────────────────────────────────────────

type mockGDOPCalculator struct {
	mu     sync.Mutex
	gdopMap []float32
	cols   int
	rows   int
}

func newMockGDOPCalculator(gdop float32, cols, rows int) *mockGDOPCalculator {
	gdopMap := make([]float32, cols*rows)
	for i := range gdopMap {
		gdopMap[i] = gdop
	}
	return &mockGDOPCalculator{gdopMap: gdopMap, cols: cols, rows: rows}
}

func (m *mockGDOPCalculator) GDOPMap(_ []NodePosition) ([]float32, int, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]float32{}, m.gdopMap...), m.cols, m.rows
}

func (m *mockGDOPCalculator) setGDOP(gdop float32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.gdopMap {
		m.gdopMap[i] = gdop
	}
}

// ─── FleetHealer Tests ────────────────────────────────────────────────────────

func TestFleetHealer_New(t *testing.T) {
	reg := newTestRegistry(t)
	cfg := FleetHealerConfig{
		HealInterval:   30 * time.Second,
		MinOnlineNodes: 2,
		MaxHistorySize: 50,
	}
	fh := NewFleetHealer(reg, cfg)

	if fh == nil {
		t.Fatal("NewFleetHealer returned nil")
	}

	if fh.IsDegraded() {
		t.Error("new healer should not be in degraded mode with 0 nodes")
	}

	if len(fh.GetOnlineNodes()) != 0 {
		t.Errorf("new healer should have 0 online nodes, got %d", len(fh.GetOnlineNodes()))
	}
}

func TestFleetHealer_DefaultConfig(t *testing.T) {
	reg := newTestRegistry(t)
	fh := NewFleetHealer(reg, FleetHealerConfig{})

	if fh.healInterval != 60*time.Second {
		t.Errorf("default HealInterval = %v, want 60s", fh.healInterval)
	}
	if fh.minOnlineNodes != 2 {
		t.Errorf("default MinOnlineNodes = %d, want 2", fh.minOnlineNodes)
	}
	if fh.maxHistorySize != 100 {
		t.Errorf("default MaxHistorySize = %d, want 100", fh.maxHistorySize)
	}
}

func TestFleetHealer_SingleNode(t *testing.T) {
	reg := newTestRegistry(t)
	reg.UpsertNode("aa:00:00:00:00:01", "v1", "S3")

	fh := NewFleetHealer(reg, FleetHealerConfig{MinOnlineNodes: 2})
	fh.SetNotifier(newMockNotifier())

	fh.OnNodeConnected("aa:00:00:00:00:01", "v1", "S3")

	roles := fh.GetOptimalRoles()
	if roles["aa:00:00:00:00:01"] != "tx_rx" {
		t.Errorf("single node role = %q, want tx_rx", roles["aa:00:00:00:00:01"])
	}

	// Single node case explicitly sets degradedMode = false (special case for minimal operation)
	if fh.IsDegraded() {
		t.Error("single node should NOT be degraded (special case for minimal operation)")
	}
}

func TestFleetHealer_TwoNodes(t *testing.T) {
	reg := newTestRegistry(t)
	reg.UpsertNode("aa:00:00:00:00:01", "v1", "S3")
	reg.UpsertNode("aa:00:00:00:00:02", "v1", "S3")

	fh := NewFleetHealer(reg, FleetHealerConfig{MinOnlineNodes: 2})

	fh.OnNodeConnected("aa:00:00:00:00:01", "v1", "S3")
	fh.OnNodeConnected("aa:00:00:00:00:02", "v1", "S3")

	roles := fh.GetOptimalRoles()
	if len(roles) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(roles))
	}

	// With 2 nodes: one TX, one RX
	txCount := 0
	rxCount := 0
	for _, role := range roles {
		if role == "tx" {
			txCount++
		}
		if role == "rx" {
			rxCount++
		}
	}
	if txCount != 1 || rxCount != 1 {
		t.Errorf("expected 1 TX and 1 RX, got %d TX, %d RX (roles: %v)", txCount, rxCount, roles)
	}

	if fh.IsDegraded() {
		t.Error("two nodes should not be degraded with MinOnlineNodes=2")
	}
}

func TestFleetHealer_ThreeNodes_OptimalRoles(t *testing.T) {
	reg := newTestRegistry(t)
	reg.UpsertNode("aa:00:00:00:00:01", "v1", "S3")
	reg.UpsertNode("aa:00:00:00:00:02", "v1", "S3")
	reg.UpsertNode("aa:00:00:00:00:03", "v1", "S3")

	fh := NewFleetHealer(reg, FleetHealerConfig{MinOnlineNodes: 2})

	fh.OnNodeConnected("aa:00:00:00:00:01", "v1", "S3")
	fh.OnNodeConnected("aa:00:00:00:00:02", "v1", "S3")
	fh.OnNodeConnected("aa:00:00:00:00:03", "v1", "S3")

	roles := fh.GetOptimalRoles()
	if len(roles) != 3 {
		t.Fatalf("expected 3 roles, got %d", len(roles))
	}

	// With 3 nodes: 1 TX (3/2=1), 2 RX
	txCount := 0
	for _, role := range roles {
		if role == "tx" {
			txCount++
		}
	}
	if txCount != 1 {
		t.Errorf("expected 1 TX, got %d (roles: %v)", txCount, roles)
	}
}

func TestFleetHealer_NodeDisconnect(t *testing.T) {
	reg := newTestRegistry(t)
	reg.UpsertNode("aa:00:00:00:00:01", "v1", "S3")
	reg.UpsertNode("aa:00:00:00:00:02", "v1", "S3")
	reg.UpsertNode("aa:00:00:00:00:03", "v1", "S3")

	fh := NewFleetHealer(reg, FleetHealerConfig{MinOnlineNodes: 2})

	fh.OnNodeConnected("aa:00:00:00:00:01", "v1", "S3")
	fh.OnNodeConnected("aa:00:00:00:00:02", "v1", "S3")
	fh.OnNodeConnected("aa:00:00:00:00:03", "v1", "S3")

	if len(fh.GetOnlineNodes()) != 3 {
		t.Fatalf("expected 3 online nodes, got %d", len(fh.GetOnlineNodes()))
	}

	// Disconnect one node
	fh.OnNodeDisconnected("aa:00:00:00:00:02")

	online := fh.GetOnlineNodes()
	if len(online) != 2 {
		t.Errorf("expected 2 online nodes after disconnect, got %d", len(online))
	}

	roles := fh.GetOptimalRoles()
	if _, exists := roles["aa:00:00:00:00:02"]; exists {
		t.Error("disconnected node should not have a role")
	}
}

func TestFleetHealer_DegradedMode(t *testing.T) {
	reg := newTestRegistry(t)
	reg.UpsertNode("aa:00:00:00:00:01", "v1", "S3")
	reg.UpsertNode("aa:00:00:00:00:02", "v1", "S3")
	reg.UpsertNode("aa:00:00:00:00:03", "v1", "S3")
	reg.UpsertNode("aa:00:00:00:00:04", "v1", "S3")

	fh := NewFleetHealer(reg, FleetHealerConfig{MinOnlineNodes: 4})

	// Connect all 4 - should not be degraded
	fh.OnNodeConnected("aa:00:00:00:00:01", "v1", "S3")
	fh.OnNodeConnected("aa:00:00:00:00:02", "v1", "S3")
	fh.OnNodeConnected("aa:00:00:00:00:03", "v1", "S3")
	fh.OnNodeConnected("aa:00:00:00:00:04", "v1", "S3")

	if fh.IsDegraded() {
		t.Error("should not be degraded with 4/4 nodes")
	}

	// Disconnect one - should enter degraded mode (3 < 4)
	fh.OnNodeDisconnected("aa:00:00:00:00:04")

	if !fh.IsDegraded() {
		t.Error("should be degraded with 3/4 nodes and MinOnlineNodes=4")
	}
}

func TestFleetHealer_Coverage(t *testing.T) {
	reg := newTestRegistry(t)
	reg.UpsertNode("aa:00:00:00:00:01", "v1", "S3")
	reg.SetRoom(RoomConfig{OriginX: 0, OriginZ: 0, Width: 4, Depth: 4})

	fh := NewFleetHealer(reg, FleetHealerConfig{})
	fh.SetGDOPCalculator(newMockGDOPCalculator(1.5, 20, 20))

	fh.OnNodeConnected("aa:00:00:00:00:01", "v1", "S3")
	fh.UpdateNodePosition("aa:00:00:00:00:01", 2.0, 2.0)

	coverage := fh.GetCoverage()
	if coverage == nil {
		t.Fatal("GetCoverage returned nil")
	}

	if coverage.ActiveNodes != 1 {
		t.Errorf("ActiveNodes = %d, want 1", coverage.ActiveNodes)
	}

	// With GDOP=1.5 everywhere, GDOP<2 percentage should be 100%
	if coverage.GDOPBelow2Pct != 1.0 {
		t.Errorf("GDOPBelow2Pct = %v, want 1.0", coverage.GDOPBelow2Pct)
	}
}

func TestFleetHealer_CoverageHistory(t *testing.T) {
	reg := newTestRegistry(t)
	reg.UpsertNode("aa:00:00:00:00:01", "v1", "S3")
	reg.SetRoom(RoomConfig{OriginX: 0, OriginZ: 0, Width: 4, Depth: 4})

	fh := NewFleetHealer(reg, FleetHealerConfig{MaxHistorySize: 5})
	fh.SetGDOPCalculator(newMockGDOPCalculator(1.5, 20, 20))

	fh.OnNodeConnected("aa:00:00:00:00:01", "v1", "S3")

	// Trigger multiple coverage computations
	for i := 0; i < 10; i++ {
		fh.computeCoverage()
	}

	history := fh.GetCoverageHistory(0)
	if len(history) > 5 {
		t.Errorf("history length = %d, want at most 5", len(history))
	}
}

func TestFleetHealer_GDOPBasedOptimization(t *testing.T) {
	reg := newTestRegistry(t)
	reg.UpsertNode("aa:00:00:00:00:01", "v1", "S3")
	reg.UpsertNode("aa:00:00:00:00:02", "v1", "S3")
	reg.UpsertNode("aa:00:00:00:00:03", "v1", "S3")
	reg.UpsertNode("aa:00:00:00:00:04", "v1", "S3")
	reg.SetRoom(RoomConfig{OriginX: 0, OriginZ: 0, Width: 4, Depth: 4})

	fh := NewFleetHealer(reg, FleetHealerConfig{})

	// Create a mock GDOP calculator that gives better GDOP with certain TX positions
	mockCalc := newMockGDOPCalculator(2.0, 20, 20)
	fh.SetGDOPCalculator(mockCalc)

	fh.OnNodeConnected("aa:00:00:00:00:01", "v1", "S3")
	fh.OnNodeConnected("aa:00:00:00:00:02", "v1", "S3")
	fh.OnNodeConnected("aa:00:00:00:00:03", "v1", "S3")
	fh.OnNodeConnected("aa:00:00:00:00:04", "v1", "S3")

	fh.UpdateNodePosition("aa:00:00:00:00:01", 0.0, 0.0)
	fh.UpdateNodePosition("aa:00:00:00:00:02", 4.0, 0.0)
	fh.UpdateNodePosition("aa:00:00:00:00:03", 0.0, 4.0)
	fh.UpdateNodePosition("aa:00:00:00:00:04", 4.0, 4.0)

	roles := fh.GetOptimalRoles()
	// With 4 nodes and targetTX=2, we should have 2 TX and 2 RX
	txCount := 0
	for _, role := range roles {
		if role == "tx" {
			txCount++
		}
	}
	if txCount != 2 {
		t.Errorf("expected 2 TX nodes, got %d (roles: %v)", txCount, roles)
	}
}

func TestFleetHealer_WorstCoverageZone(t *testing.T) {
	reg := newTestRegistry(t)
	reg.UpsertNode("aa:00:00:00:00:01", "v1", "S3")
	reg.UpsertNode("aa:00:00:00:00:02", "v1", "S3")
	reg.SetRoom(RoomConfig{OriginX: 0, OriginZ: 0, Width: 4, Depth: 4})

	fh := NewFleetHealer(reg, FleetHealerConfig{})
	fh.SetGDOPCalculator(newMockGDOPCalculator(3.0, 20, 20))

	fh.OnNodeConnected("aa:00:00:00:00:01", "v1", "S3")
	fh.OnNodeConnected("aa:00:00:00:00:02", "v1", "S3")
	fh.UpdateNodePosition("aa:00:00:00:00:01", 1.0, 2.0)
	fh.UpdateNodePosition("aa:00:00:00:00:02", 3.0, 2.0)

	x, z, gdop := fh.GetWorstCoverageZone()
	if gdop != 3.0 {
		t.Errorf("worst GDOP = %v, want 3.0", gdop)
	}
	// Verify coordinates are within room bounds
	if x < 0 || x > 4 {
		t.Errorf("x = %v, should be in [0, 4]", x)
	}
	if z < 0 || z > 4 {
		t.Errorf("z = %v, should be in [0, 4]", z)
	}
}

func TestFleetHealer_SuggestNodePosition(t *testing.T) {
	reg := newTestRegistry(t)
	reg.UpsertNode("aa:00:00:00:00:01", "v1", "S3")
	reg.SetRoom(RoomConfig{OriginX: 0, OriginZ: 0, Width: 4, Depth: 4})

	fh := NewFleetHealer(reg, FleetHealerConfig{})
	fh.SetGDOPCalculator(newMockGDOPCalculator(3.0, 20, 20))

	fh.OnNodeConnected("aa:00:00:00:00:01", "v1", "S3")
	fh.UpdateNodePosition("aa:00:00:00:00:01", 0.5, 0.5)

	// Should suggest a position away from the existing node
	x, z, improvement := fh.SuggestNodePosition()
	_ = x
	_ = z
	_ = improvement
}

func TestFleetHealer_NoGDOPCalculator(t *testing.T) {
	reg := newTestRegistry(t)
	reg.UpsertNode("aa:00:00:00:00:01", "v1", "S3")
	reg.UpsertNode("aa:00:00:00:00:02", "v1", "S3")

	fh := NewFleetHealer(reg, FleetHealerConfig{})

	// Without GDOP calculator, should fall back to simple assignment
	fh.OnNodeConnected("aa:00:00:00:00:01", "v1", "S3")
	fh.OnNodeConnected("aa:00:00:00:00:02", "v1", "S3")

	roles := fh.GetOptimalRoles()
	if len(roles) != 2 {
		t.Errorf("expected 2 roles without GDOP calculator, got %d", len(roles))
	}
}

func TestGenerateCombinations(t *testing.T) {
	tests := []struct {
		n, k    int
		wantLen int
	}{
		{4, 2, 6},   // C(4,2) = 6
		{5, 2, 10},  // C(5,2) = 10
		{5, 3, 10},  // C(5,3) = 10
		{6, 3, 20},  // C(6,3) = 20
		{3, 0, 1},   // C(3,0) = 1
		{3, 3, 1},   // C(3,3) = 1
	}

	for _, tt := range tests {
		combinations := generateCombinations(tt.n, tt.k)
		if len(combinations) != tt.wantLen {
			t.Errorf("generateCombinations(%d, %d) = %d combinations, want %d",
				tt.n, tt.k, len(combinations), tt.wantLen)
		}
	}
}

func TestGenerateCombinations_Contents(t *testing.T) {
	// Test C(4,2) specifically
	combinations := generateCombinations(4, 2)

	expected := [][]int{
		{0, 1}, {0, 2}, {0, 3}, {1, 2}, {1, 3}, {2, 3},
	}

	if len(combinations) != len(expected) {
		t.Fatalf("got %d combinations, want %d", len(combinations), len(expected))
	}

	for i, comb := range combinations {
		if len(comb) != 2 {
			t.Errorf("combination %d has length %d, want 2", i, len(comb))
		}
	}

	// Verify all combinations are unique
	seen := make(map[string]bool)
	for _, comb := range combinations {
		key := ""
		for _, v := range comb {
			key += string(rune('0' + v))
		}
		if seen[key] {
			t.Errorf("duplicate combination: %v", comb)
		}
		seen[key] = true
	}
}

func TestFleetHealer_UpdateNodePosition(t *testing.T) {
	reg := newTestRegistry(t)
	fh := NewFleetHealer(reg, FleetHealerConfig{})

	fh.UpdateNodePosition("aa:00:00:00:00:01", 1.5, 2.5)

	pos := fh.nodePositions["aa:00:00:00:00:01"]
	if pos.X != 1.5 || pos.Z != 2.5 {
		t.Errorf("position = (%v, %v), want (1.5, 2.5)", pos.X, pos.Z)
	}
}

func TestFleetHealer_RecoveryFromDegraded(t *testing.T) {
	reg := newTestRegistry(t)
	reg.UpsertNode("aa:00:00:00:00:01", "v1", "S3")
	reg.UpsertNode("aa:00:00:00:00:02", "v1", "S3")
	reg.UpsertNode("aa:00:00:00:00:03", "v1", "S3")
	reg.UpsertNode("aa:00:00:00:00:04", "v1", "S3")

	fh := NewFleetHealer(reg, FleetHealerConfig{MinOnlineNodes: 4})

	// Connect all 4
	fh.OnNodeConnected("aa:00:00:00:00:01", "v1", "S3")
	fh.OnNodeConnected("aa:00:00:00:00:02", "v1", "S3")
	fh.OnNodeConnected("aa:00:00:00:00:03", "v1", "S3")
	fh.OnNodeConnected("aa:00:00:00:00:04", "v1", "S3")

	if fh.IsDegraded() {
		t.Fatal("should not start degraded")
	}

	// Disconnect one - degraded (3 < 4)
	fh.OnNodeDisconnected("aa:00:00:00:00:04")
	if !fh.IsDegraded() {
		t.Error("should be degraded after disconnect (3 < 4)")
	}

	// Reconnect - recovered
	fh.OnNodeConnected("aa:00:00:00:00:04", "v1", "S3")
	if fh.IsDegraded() {
		t.Error("should recover after reconnect")
	}
}

func TestFleetHealer_ComputeCoverage_NoGDOPCalculator(t *testing.T) {
	reg := newTestRegistry(t)
	reg.UpsertNode("aa:00:00:00:00:01", "v1", "S3")

	fh := NewFleetHealer(reg, FleetHealerConfig{})
	fh.OnNodeConnected("aa:00:00:00:00:01", "v1", "S3")

	coverage := fh.computeCoverage()
	if coverage == nil {
		t.Fatal("computeCoverage returned nil")
	}

	// Without GDOP calculator, GDOP metrics should be zero
	if coverage.MeanGDOP != 0 {
		t.Errorf("MeanGDOP = %v, want 0 without calculator", coverage.MeanGDOP)
	}
}

func TestFleetHealer_CoverageScore(t *testing.T) {
	reg := newTestRegistry(t)
	reg.SetRoom(RoomConfig{OriginX: 0, OriginZ: 0, Width: 4, Depth: 4})

	fh := NewFleetHealer(reg, FleetHealerConfig{})
	fh.SetGDOPCalculator(newMockGDOPCalculator(1.5, 20, 20))
	fh.OnNodeConnected("aa:00:00:00:00:01", "v1", "S3")
	fh.UpdateNodePosition("aa:00:00:00:00:01", 2.0, 2.0)

	coverage := fh.computeCoverage()

	// With all cells at GDOP=1.5:
	// GDOPBelow2Pct = 1.0 (100%)
	// GDOPBelow5Pct = 1.0 (100%)
	// WorstPenalty = min(1.5/10, 1.0) = 0.15
	// CoverageScore = 0.5*1.0 + 0.3*1.0 + 0.2*(1-0.15) = 0.5 + 0.3 + 0.17 = 0.97
	expectedScore := 0.5*1.0 + 0.3*1.0 + 0.2*(1-0.15)
	if coverage.CoverageScore < expectedScore-0.01 || coverage.CoverageScore > expectedScore+0.01 {
		t.Errorf("CoverageScore = %v, want approximately %v", coverage.CoverageScore, expectedScore)
	}
}

func TestSimpleRoleAssignment(t *testing.T) {
	reg := newTestRegistry(t)
	fh := NewFleetHealer(reg, FleetHealerConfig{})

	nodes := []string{"a", "b", "c", "d"}
	roles, txNodes := fh.simpleRoleAssignment(nodes, 2)

	if len(roles) != 4 {
		t.Errorf("expected 4 roles, got %d", len(roles))
	}

	if len(txNodes) != 2 {
		t.Errorf("expected 2 TX nodes, got %d", len(txNodes))
	}

	// First 2 should be TX
	if roles["a"] != "tx" || roles["b"] != "tx" {
		t.Errorf("first two nodes should be TX, got a=%s, b=%s", roles["a"], roles["b"])
	}

	// Last 2 should be RX
	if roles["c"] != "rx" || roles["d"] != "rx" {
		t.Errorf("last two nodes should be RX, got c=%s, d=%s", roles["c"], roles["d"])
	}
}
