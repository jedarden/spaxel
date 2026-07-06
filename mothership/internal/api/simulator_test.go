// Package api tests the simulator REST API
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/spaxel/mothership/internal/fleet"
	"github.com/spaxel/mothership/internal/fusion"
	"github.com/spaxel/mothership/internal/simulator"
)

// TestAddNode_SpreadsDefaultOrigin tests that nodes created without explicit
// positions (at origin {0,0,0}) are assigned spread-out positions instead of
// remaining co-located, which would collapse Fresnel excess path length toward
// 0 and prevent blob formation (bf-18yn, bf-4q5w).
func TestAddNode_SpreadsDefaultOrigin(t *testing.T) {
	handler := NewSimulatorHandler(nil, nil, nil)
	space := simulator.DefaultSpace()
	handler.mu.Lock()
	handler.space = space
	handler.mu.Unlock()

	// Create multiple nodes without positions (all at origin)
	var createdNodes []simulator.Node
	for i := 0; i < 4; i++ {
		nodeJSON := map[string]interface{}{
			"id":   string(rune('a' + i)),
			"name": "Node Name",
		}
		body, _ := json.Marshal(nodeJSON)

		req := httptest.NewRequest("POST", "/api/simulator/nodes", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.AddNode(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected status %d, got %d", http.StatusCreated, w.Code)
		}

		var node simulator.Node
		if err := json.Unmarshal(w.Body.Bytes(), &node); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		createdNodes = append(createdNodes, node)
	}

	// Verify all nodes have distinct positions (not co-located at origin)
	positions := make(map[[3]float64]int)
	for _, node := range createdNodes {
		pos := [3]float64{node.Position.X, node.Position.Y, node.Position.Z}

		// Check not at origin
		if node.Position.X == 0 && node.Position.Y == 0 && node.Position.Z == 0 {
			t.Errorf("node %s still at origin (0,0,0)", node.ID)
		}

		// Check distinct
		positions[pos]++
		if positions[pos] > 1 {
			t.Errorf("duplicate position %v for node %s", pos, node.ID)
		}
	}

	// Verify positions span the room (not all at same point)
	_, _, _, _, _, _ = space.Bounds()

	var minXSeen, maxXSeen, minYSeen, maxYSeen float64
	for i, node := range createdNodes {
		if i == 0 {
			minXSeen, maxXSeen = node.Position.X, node.Position.X
			minYSeen, maxYSeen = node.Position.Y, node.Position.Y
		}
		if node.Position.X < minXSeen {
			minXSeen = node.Position.X
		}
		if node.Position.X > maxXSeen {
			maxXSeen = node.Position.X
		}
		if node.Position.Y < minYSeen {
			minYSeen = node.Position.Y
		}
		if node.Position.Y > maxYSeen {
			maxYSeen = node.Position.Y
		}
	}

	// For 4 nodes, we should see some spread across the room
	// (not strictly required for correctness, but verifies the fix works)
	if minXSeen == maxXSeen && minYSeen == maxYSeen {
		t.Errorf("nodes don't span room: all at (%.2f, %.2f)", minXSeen, minYSeen)
	}

	t.Logf("Successfully spread %d nodes across room: X=[%.2f,%.2f] Y=[%.2f,%.2f]",
		len(createdNodes), minXSeen, maxXSeen, minYSeen, maxYSeen)
}

// TestAddNode_KeepsExplicitPosition tests that nodes with explicit positions
// are not modified.
func TestAddNode_KeepsExplicitPosition(t *testing.T) {
	handler := NewSimulatorHandler(nil, nil, nil)

	nodeJSON := map[string]interface{}{
		"id": "test-node",
		"position": map[string]interface{}{
			"x": 1.5,
			"y": 2.3,
			"z": 1.0,
		},
	}
	body, _ := json.Marshal(nodeJSON)

	req := httptest.NewRequest("POST", "/api/simulator/nodes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.AddNode(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, w.Code)
	}

	var node simulator.Node
	if err := json.Unmarshal(w.Body.Bytes(), &node); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify explicit position was preserved
	if node.Position.X != 1.5 || node.Position.Y != 2.3 || node.Position.Z != 1.0 {
		t.Errorf("explicit position not preserved: got (%.2f, %.2f, %.2f)",
			node.Position.X, node.Position.Y, node.Position.Z)
	}
}

// recordingRegistry is a simulator.RegistryNodeAdapter that records the side
// effects of SyncToRegistry so tests can assert the simulator API drove an
// immediate write into the fleet registry (rather than only logging).
type recordingRegistry struct {
	mu         sync.Mutex
	added      []simulator.NodeRecord
	posUpdates []simulator.NodeRecord
	nodesByMAC map[string]simulator.NodeRecord
}

func newRecordingRegistry() *recordingRegistry {
	return &recordingRegistry{nodesByMAC: make(map[string]simulator.NodeRecord)}
}

func (r *recordingRegistry) AddVirtualNode(mac, name string, x, y, z float64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec := simulator.NodeRecord{MAC: mac, Name: name, PosX: x, PosY: y, PosZ: z, Virtual: true, Enabled: true}
	r.nodesByMAC[mac] = rec
	r.added = append(r.added, rec)
	return nil
}

func (r *recordingRegistry) SetNodePosition(mac string, x, y, z float64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec := r.nodesByMAC[mac]
	rec.PosX, rec.PosY, rec.PosZ = x, y, z
	r.nodesByMAC[mac] = rec
	r.posUpdates = append(r.posUpdates, simulator.NodeRecord{MAC: mac, PosX: x, PosY: y, PosZ: z})
	return nil
}

func (r *recordingRegistry) SetNodeRole(mac, role string) error { return nil }

func (r *recordingRegistry) DeleteNode(mac string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.nodesByMAC, mac)
	return nil
}

func (r *recordingRegistry) GetNode(mac string) (*simulator.NodeRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.nodesByMAC[mac]
	if !ok {
		return nil, fmt.Errorf("not found: %s", mac)
	}
	return &rec, nil
}

func (r *recordingRegistry) GetAllNodes() ([]simulator.NodeRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]simulator.NodeRecord, 0, len(r.nodesByMAC))
	for _, rec := range r.nodesByMAC {
		out = append(out, rec)
	}
	return out, nil
}

// newWiredHandler builds a SimulatorHandler backed by a real VirtualNodeStore
// (in a temp dir), a FleetRegistryBridge over that store, and the given adapter.
// This mirrors the wiring in cmd/mothership/main.go that drives immediate syncs.
func newWiredHandler(t *testing.T, adapter simulator.RegistryNodeAdapter) *SimulatorHandler {
	t.Helper()
	store, err := simulator.NewVirtualNodeStore(simulator.StoreConfig{
		DataDir: filepath.Join(t.TempDir(), "simulator"),
		Space:   simulator.DefaultSpace(),
	})
	if err != nil {
		t.Fatalf("create virtual store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	bridge := simulator.NewFleetRegistryBridge(store)
	return NewSimulatorHandler(store, bridge, adapter)
}

// postNode issues a POST /api/simulator/nodes for the given node body.
func postNode(t *testing.T, h *SimulatorHandler, body map[string]interface{}) simulator.Node {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/simulator/nodes", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.AddNode(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("AddNode status = %d, want %d (body: %s)", w.Code, http.StatusCreated, w.Body.String())
	}
	var node simulator.Node
	if err := json.Unmarshal(w.Body.Bytes(), &node); err != nil {
		t.Fatalf("decode node: %v", err)
	}
	return node
}

// putNode issues a PUT /api/simulator/nodes/{nodeID} with chi's URL param set.
func putNode(t *testing.T, h *SimulatorHandler, nodeID string, body map[string]interface{}) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest("PUT", "/api/simulator/nodes/"+nodeID, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("nodeID", nodeID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.UpdateNode(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("UpdateNode status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
}

// TestImmediateSyncOnAddAndUpdate verifies the bf-5lii wiring: AddNode and
// UpdateNode must drive an immediate SyncToRegistry into the fleet registry
// instead of only logging that the node will sync on the next periodic tick.
func TestImmediateSyncOnAddAndUpdate(t *testing.T) {
	cases := []struct {
		name    string
		adapter simulator.RegistryNodeAdapter // nil exercises the no-op path
	}{
		{"wired adapter triggers immediate sync", newRecordingRegistry()},
		{"nil adapter is a safe no-op", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newWiredHandler(t, c.adapter)
			rec, _ := c.adapter.(*recordingRegistry)

			// AddNode: node must reach the registry synchronously, before any ticker.
			node := postNode(t, h, map[string]interface{}{
				"id":       "node-A",
				"name":     "Node A",
				"position": map[string]interface{}{"x": 1.0, "y": 1.0, "z": 1.0},
			})

			if rec == nil {
				return // no-op path: nothing further to assert, just no panic
			}
			if len(rec.added) == 0 {
				t.Fatalf("AddNode did not trigger an immediate SyncToRegistry: no AddVirtualNode calls recorded")
			}
			added := rec.added[len(rec.added)-1]
			if added.Name != "Node A" {
				t.Errorf("registry saw name %q, want %q", added.Name, "Node A")
			}
			if added.PosX != node.Position.X || added.PosY != node.Position.Y || added.PosZ != node.Position.Z {
				t.Errorf("registry saw position (%.2f,%.2f,%.2f), want node position (%.2f,%.2f,%.2f)",
					added.PosX, added.PosY, added.PosZ, node.Position.X, node.Position.Y, node.Position.Z)
			}
			addedBeforeUpdate := len(rec.posUpdates)

			// UpdateNode: position change must push through immediately.
			putNode(t, h, "node-A", map[string]interface{}{
				"id":       "node-A",
				"name":     "Node A",
				"position": map[string]interface{}{"x": 2.5, "y": 3.0, "z": 1.5},
			})

			if len(rec.posUpdates) <= addedBeforeUpdate {
				t.Fatalf("UpdateNode did not trigger an immediate SyncToRegistry: no new SetNodePosition call")
			}
			upd := rec.posUpdates[len(rec.posUpdates)-1]
			if upd.MAC != added.MAC {
				t.Errorf("position update MAC %q != add MAC %q", upd.MAC, added.MAC)
			}
			if upd.PosX != 2.5 || upd.PosY != 3.0 || upd.PosZ != 1.5 {
				t.Errorf("position update saw (%.2f,%.2f,%.2f), want (2.50,3.00,1.50)", upd.PosX, upd.PosY, upd.PosZ)
			}
		})
	}
}

// fleetRegistryTestAdapter mirrors cmd/mothership/main.go's fleetRegistryAdapter:
// it exposes a real, SQLite-backed *fleet.Registry through the
// simulator.RegistryNodeAdapter interface. The production adapter lives in
// package main (not importable from tests), so this re-creates the identical
// thin wrapper — write-side methods delegate directly, and GetNode/GetAllNodes
// convert fleet.NodeRecord -> simulator.NodeRecord (Enabled follows the fleet's
// role-based model: role "idle" means disabled). Routing through the real
// registry is what makes the tests below genuinely end-to-end: a position
// written by the bridge is round-tripped through SQLite, not merely recorded by
// a fake. (bf-69ym)
//
// bf-u7ds: AddVirtualNode/SetNodePosition also forward the new position through
// forwardPos (mirroring the production adapter) so a simulator-driven registry
// write reaches the blob-producing fusion engine at runtime, not only at
// startup seeding. Tests set forwardPos to fleet.Manager.ForwardNodePosition,
// exactly as cmd/mothership/main.go does.
type fleetRegistryTestAdapter struct {
	reg        *fleet.Registry
	forwardPos func(mac string, x, y, z float64)
}

func (a *fleetRegistryTestAdapter) AddVirtualNode(mac, name string, x, y, z float64) error {
	if err := a.reg.AddVirtualNode(mac, name, x, y, z); err != nil {
		return err
	}
	if a.forwardPos != nil {
		a.forwardPos(mac, x, y, z)
	}
	return nil
}

func (a *fleetRegistryTestAdapter) SetNodePosition(mac string, x, y, z float64) error {
	if err := a.reg.SetNodePosition(mac, x, y, z); err != nil {
		return err
	}
	if a.forwardPos != nil {
		a.forwardPos(mac, x, y, z)
	}
	return nil
}

func (a *fleetRegistryTestAdapter) SetNodeRole(mac, role string) error {
	return a.reg.SetNodeRole(mac, role)
}

func (a *fleetRegistryTestAdapter) DeleteNode(mac string) error {
	return a.reg.DeleteNode(mac)
}

func (a *fleetRegistryTestAdapter) GetNode(mac string) (*simulator.NodeRecord, error) {
	rec, err := a.reg.GetNode(mac)
	if err != nil {
		return nil, err
	}
	return fleetNodeRecordToSimulatorView(rec), nil
}

func (a *fleetRegistryTestAdapter) GetAllNodes() ([]simulator.NodeRecord, error) {
	nodes, err := a.reg.GetAllNodes()
	if err != nil {
		return nil, err
	}
	out := make([]simulator.NodeRecord, 0, len(nodes))
	for i := range nodes {
		out = append(out, *fleetNodeRecordToSimulatorView(&nodes[i]))
	}
	return out, nil
}

// fleetNodeRecordToSimulatorView mirrors fleetNodeRecordToSimulator in
// cmd/mothership/main.go.
func fleetNodeRecordToSimulatorView(n *fleet.NodeRecord) *simulator.NodeRecord {
	return &simulator.NodeRecord{
		MAC:     n.MAC,
		Name:    n.Name,
		Role:    n.Role,
		PosX:    n.PosX,
		PosY:    n.PosY,
		PosZ:    n.PosZ,
		Virtual: n.Virtual,
		Enabled: n.Role != "idle",
	}
}

// newFleetRegistry opens a real fleet.Registry backed by a fresh temp SQLite
// DB, closing it automatically when the test ends.
func newFleetRegistry(t *testing.T) *fleet.Registry {
	t.Helper()
	reg, err := fleet.NewRegistry(filepath.Join(t.TempDir(), "fleet.db"))
	if err != nil {
		t.Fatalf("create fleet registry: %v", err)
	}
	t.Cleanup(func() { _ = reg.Close() })
	return reg
}

// registryNodesByName indexes the live fleet registry's nodes by Name for
// assertion lookups. The bridge's virtualMAC is unexported, so tests key on the
// human-readable name they supplied when creating the node.
func registryNodesByName(t *testing.T, reg *fleet.Registry) map[string]*fleet.NodeRecord {
	t.Helper()
	nodes, err := reg.GetAllNodes()
	if err != nil {
		t.Fatalf("GetAllNodes: %v", err)
	}
	out := make(map[string]*fleet.NodeRecord, len(nodes))
	for i := range nodes {
		n := nodes[i]
		out[n.Name] = &n
	}
	return out
}

// TestSimulatorHandlerToRegistry_CreateAndUpdate is the end-to-end lock-in for
// the simulator→fleet-registry position flow (bf-69ym, split from bf-4oiz). It
// wires a real SQLite-backed fleet.Registry behind the SimulatorHandler — the
// same wiring cmd/mothership/main.go uses — and drives the complete path:
//
//	AddNode (simulator handler) -> VirtualNodeStore.CreateVirtualNode ->
//	FleetRegistryBridge.SyncToRegistry -> adapter.AddVirtualNode -> fleet.Registry
//
// and then a position change via UpdateNode -> SetNodePosition. The create leg
// asserts the node lands in the live registry at the spread geometry
// DefaultNodePositions assigns (never the degenerate (0,0,0) / DB origin); the
// update leg asserts the new position flows through to the same registry row.
func TestSimulatorHandlerToRegistry_CreateAndUpdate(t *testing.T) {
	reg := newFleetRegistry(t)
	adapter := &fleetRegistryTestAdapter{reg: reg}
	h := newWiredHandler(t, adapter) // real VirtualNodeStore + bridge + adapter

	space := simulator.DefaultSpace()

	// Create: post a node with no position. AddNode assigns it a spread slot
	// from DefaultNodePositions and the immediate sync writes that into the live
	// fleet registry (not just the in-memory NodeSet).
	node := postNode(t, h, map[string]interface{}{
		"id":   "node-A",
		"name": "Node A",
	})

	wantSpread := simulator.DefaultNodePositions(space, 1)
	if len(wantSpread) != 1 {
		t.Fatalf("DefaultNodePositions(1) returned %d slots, want 1", len(wantSpread))
	}
	wantPos := wantSpread[0]

	byName := registryNodesByName(t, reg)
	rec, ok := byName["Node A"]
	if !ok {
		t.Fatalf("node \"Node A\" never reached the fleet registry; have %d nodes", len(byName))
	}
	if !rec.Virtual {
		t.Errorf("registry node not marked virtual: %+v", rec)
	}
	gotPos := simulator.Point{X: rec.PosX, Y: rec.PosY, Z: rec.PosZ}
	if gotPos != wantPos {
		t.Errorf("create: registry position %v != DefaultNodePositions spread %v", gotPos, wantPos)
	}
	if gotPos.X == 0 && gotPos.Y == 0 && gotPos.Z == 0 {
		t.Errorf("create: registry node landed at (0,0,0)")
	}
	// The handler-returned node and the registry must agree.
	if node.Position != wantPos {
		t.Errorf("create: handler position %v != spread position %v", node.Position, wantPos)
	}

	// Update: change the position; the new value must reach the live registry
	// via SetNodePosition, not just the in-memory NodeSet.
	newPos := simulator.Point{X: 2.5, Y: 3.0, Z: 1.5}
	putNode(t, h, "node-A", map[string]interface{}{
		"id":   "node-A",
		"name": "Node A",
		"position": map[string]interface{}{
			"x": newPos.X, "y": newPos.Y, "z": newPos.Z,
		},
	})

	byName = registryNodesByName(t, reg)
	rec, ok = byName["Node A"]
	if !ok {
		t.Fatalf("node \"Node A\" disappeared from fleet registry after update")
	}
	upd := simulator.Point{X: rec.PosX, Y: rec.PosY, Z: rec.PosZ}
	if upd != newPos {
		t.Errorf("update: registry position %v != new position %v", upd, newPos)
	}
	// Still exactly one registry row — an update must not duplicate.
	if len(byName) != 1 {
		t.Errorf("update: expected 1 registry node, got %d (%+v)", len(byName), byName)
	}
}

// singleEnginePosition returns the fusion engine's single registered node
// position, failing the test unless exactly one node is positioned. Used to
// assert what the accumulation grid will localize against.
func singleEnginePosition(t *testing.T, engine *fusion.Engine) simulator.Point {
	t.Helper()
	positions := engine.NodePositions()
	if len(positions) != 1 {
		t.Fatalf("expected exactly 1 fusion engine node position, got %d: %+v", len(positions), positions)
	}
	p := positions[0]
	return simulator.Point{X: p.X, Y: p.Y, Z: p.Z}
}

// TestSimulatorHandlerToRegistry_PositionsReachFusionEngine (bf-u7ds) is the
// end-to-end lock-in for the third registry-write path: a simulator-driven node
// create/update must reach the blob-producing 3D fusion engine, not just the
// fleet DB. The production wiring (cmd/mothership/main.go) builds the adapter
// with forwardPos = fleetMgr.ForwardNodePosition, and the manager's
// nodePositionSink is wired to fusion.Engine.SetNodePosition; this test
// recreates that exact chain behind the SimulatorHandler and asserts the
// engine's nodePos mirror updates on both AddNode and UpdateNode.
//
// Without the forwardPos hook, SyncToRegistry lands the position in SQLite but
// the engine mirror stays stale and Fuse localizes against wrong/missing
// geometry — the regression this bead guards against. Mirrors the two bf-3p6g
// lock-ins (PATCH /position and OnNodeConnected) for the simulator path.
func TestSimulatorHandlerToRegistry_PositionsReachFusionEngine(t *testing.T) {
	reg := newFleetRegistry(t)
	mgr := fleet.NewManager(reg)
	engine := fusion.NewEngine(nil)
	mgr.SetNodePositionSink(func(mac string, x, y, z float64) {
		engine.SetNodePosition(mac, x, y, z)
	})

	adapter := &fleetRegistryTestAdapter{
		reg:        reg,
		forwardPos: mgr.ForwardNodePosition,
	}
	h := newWiredHandler(t, adapter)

	space := simulator.DefaultSpace()

	// Sanity: before any node is added the engine holds nothing to localize on.
	if engine.NodeCount() != 0 {
		t.Fatalf("engine should hold no positions before AddNode, got %d", engine.NodeCount())
	}

	// AddNode with no position: the bridge assigns spread geometry and the
	// immediate sync must push it through to the engine's nodePos mirror.
	postNode(t, h, map[string]interface{}{
		"id":   "node-A",
		"name": "Node A",
	})
	wantSpread := simulator.DefaultNodePositions(space, 1)
	if len(wantSpread) != 1 {
		t.Fatalf("DefaultNodePositions(1) returned %d slots, want 1", len(wantSpread))
	}
	got := singleEnginePosition(t, engine)
	if got != wantSpread[0] {
		t.Fatalf("AddNode: fusion engine position %v != spread %v", got, wantSpread[0])
	}
	if got.X == 0 && got.Y == 0 && got.Z == 0 {
		t.Fatalf("AddNode: fusion engine landed at (0,0,0) — engine mirror not seeded")
	}

	// UpdateNode with a new explicit position: the engine mirror must move with it.
	newPos := simulator.Point{X: 2.5, Y: 3.0, Z: 1.5}
	putNode(t, h, "node-A", map[string]interface{}{
		"id":   "node-A",
		"name": "Node A",
		"position": map[string]interface{}{
			"x": newPos.X, "y": newPos.Y, "z": newPos.Z,
		},
	})
	got = singleEnginePosition(t, engine)
	if got != newPos {
		t.Fatalf("UpdateNode: fusion engine position %v != new position %v", got, newPos)
	}
	// Still a single positioned node — an update must not orphan the old entry.
	if engine.NodeCount() != 1 {
		t.Errorf("UpdateNode: expected 1 fusion engine node, got %d", engine.NodeCount())
	}
}

// TestSimulatorHandlerToRegistry_OriginNodesGetSpreadGeometry is the
// end-to-end analogue of the bridge-level
// TestSyncToRegistry_OriginNodesGetSpreadGeometry, but driven through the REST
// handler and persisted in SQLite. Nodes created at the DB default origin
// ({0,0,1}) must land in the live fleet registry at exactly the distinct,
// spread-out geometry DefaultNodePositions assigns — never co-located at the
// origin (which would collapse Fresnel excess path toward zero and prevent blob
// formation; bf-18yn / bf-4q5w).
func TestSimulatorHandlerToRegistry_OriginNodesGetSpreadGeometry(t *testing.T) {
	const count = 4
	reg := newFleetRegistry(t)
	adapter := &fleetRegistryTestAdapter{reg: reg}
	h := newWiredHandler(t, adapter)

	space := simulator.DefaultSpace()
	origin := simulator.Point{X: 0, Y: 0, Z: 1} // DefaultNodeOrigin

	for _, id := range []string{"n-a", "n-b", "n-c", "n-d"} {
		postNode(t, h, map[string]interface{}{
			"id":   id,
			"name": id,
			"position": map[string]interface{}{
				"x": origin.X, "y": origin.Y, "z": origin.Z,
			},
		})
	}

	nodes, err := reg.GetAllNodes()
	if err != nil {
		t.Fatalf("GetAllNodes: %v", err)
	}
	if len(nodes) != count {
		t.Fatalf("expected %d registry nodes, got %d", count, len(nodes))
	}

	want := simulator.DefaultNodePositions(space, count)
	wantSet := make(map[simulator.Point]bool, len(want))
	for _, p := range want {
		wantSet[p] = true
	}

	gotSet := make(map[simulator.Point]bool, len(nodes))
	for _, n := range nodes {
		if !n.Virtual {
			t.Errorf("registry node %s not marked virtual", n.MAC)
		}
		p := simulator.Point{X: n.PosX, Y: n.PosY, Z: n.PosZ}
		if p == origin || (p.X == 0 && p.Y == 0 && p.Z == 0) {
			t.Errorf("registry node %s still at origin/zero: %v", n.MAC, p)
		}
		if gotSet[p] {
			t.Errorf("duplicate registry position %v", p)
		}
		gotSet[p] = true
	}

	for p := range gotSet {
		if !wantSet[p] {
			t.Errorf("registry position %v is not a DefaultNodePositions slot", p)
		}
	}
	for _, p := range want {
		if !gotSet[p] {
			t.Errorf("expected DefaultNodePositions slot %v missing from registry", p)
		}
	}
}

// TestPositionPropagation_FullPipeline (bf-95tx) is the end-to-end lock-in for
// the COMPLETE simulator → fleet registry → fusion engine position path in a
// single test. One node position, posted once through the simulator REST API,
// must arrive — unchanged and in order — at every stage the runtime cares about:
//
//  1. simulator VirtualNodeStore       — the source of truth for virtual nodes
//  2. fleet.Registry (SQLite)          — persisted; read by the dashboard/REST
//  3. fusion.Engine nodePos mirror     — what Fuse localizes against at runtime
//
// bf-69ym locked in stages 1→2; bf-u7ds locked in stages 1→3. This test ties
// them together: it traces ONE position value through all three stages in a
// single run and asserts they agree at both AddNode and UpdateNode, with
// per-stage logging so the full data path is observable under `go test -v`.
// That is the demonstrable full-pipeline verification the position-propagation
// bead (bf-95tx) requires.
func TestPositionPropagation_FullPipeline(t *testing.T) {
	// Wire the full chain exactly as cmd/mothership/main.go does:
	// handler → bridge.SyncToRegistry → adapter → fleet.Registry (SQLite),
	// with the adapter's forwardPos → fleet.Manager.ForwardNodePosition →
	// nodePositionSink → fusion.Engine.SetNodePosition.
	reg := newFleetRegistry(t)
	mgr := fleet.NewManager(reg)
	engine := fusion.NewEngine(nil)
	mgr.SetNodePositionSink(func(mac string, x, y, z float64) {
		engine.SetNodePosition(mac, x, y, z)
	})
	adapter := &fleetRegistryTestAdapter{
		reg:        reg,
		forwardPos: mgr.ForwardNodePosition,
	}
	h := newWiredHandler(t, adapter) // real VirtualNodeStore + bridge + adapter

	// pipelineStages reads the posted node's position back from all three stages
	// and logs each, so a `go test -v` run visibly traces the value's path:
	// simulator → registry → fusion. nodeID keys the simulator store; name keys
	// the fleet registry row (the bridge's virtualMAC is unexported); the single
	// positioned node keys the fusion engine mirror.
	pipelineStages := func(label, nodeID, name string) (simPos, regPos, fusPos simulator.Point) {
		t.Helper()

		// Stage 1 — simulator VirtualNodeStore (the source).
		vs, err := h.virtualStore.GetNode(nodeID)
		if err != nil {
			t.Fatalf("%s: simulator store lookup %q: %v", label, nodeID, err)
		}
		simPos = simulator.Point{X: vs.Position.X, Y: vs.Position.Y, Z: vs.Position.Z}

		// Stage 2 — fleet.Registry (SQLite), matched by Name.
		byName := registryNodesByName(t, reg)
		rec, ok := byName[name]
		if !ok {
			t.Fatalf("%s: node %q missing from fleet registry (have %d nodes)", label, name, len(byName))
		}
		regPos = simulator.Point{X: rec.PosX, Y: rec.PosY, Z: rec.PosZ}

		// Stage 3 — fusion.Engine nodePos mirror (what Fuse localizes against).
		fusPos = singleEnginePosition(t, engine)

		t.Logf("[%s] position propagation  simulator=%v  registry=%v  fusion=%v",
			label, simPos, regPos, fusPos)
		return simPos, regPos, fusPos
	}

	// --- Create leg: post one node with an explicit position. ---
	const nodeID, nodeName = "node-A", "Node A"
	createPos := simulator.Point{X: 1.5, Y: 2.5, Z: 1.0}
	postNode(t, h, map[string]interface{}{
		"id":   nodeID,
		"name": nodeName,
		"position": map[string]interface{}{
			"x": createPos.X, "y": createPos.Y, "z": createPos.Z,
		},
	})

	simPos, regPos, fusPos := pipelineStages("create", nodeID, nodeName)
	if simPos != createPos {
		t.Errorf("create: simulator stage %v != posted %v", simPos, createPos)
	}
	if regPos != createPos {
		t.Errorf("create: registry stage %v != posted %v", regPos, createPos)
	}
	if fusPos != createPos {
		t.Errorf("create: fusion stage %v != posted %v", fusPos, createPos)
	}

	// --- Update leg: move the node and re-trace the full pipeline. ---
	updatePos := simulator.Point{X: 3.5, Y: 4.5, Z: 2.0}
	putNode(t, h, nodeID, map[string]interface{}{
		"id":   nodeID,
		"name": nodeName,
		"position": map[string]interface{}{
			"x": updatePos.X, "y": updatePos.Y, "z": updatePos.Z,
		},
	})

	simPos, regPos, fusPos = pipelineStages("update", nodeID, nodeName)
	if simPos != updatePos {
		t.Errorf("update: simulator stage %v != posted %v", simPos, updatePos)
	}
	if regPos != updatePos {
		t.Errorf("update: registry stage %v != posted %v", regPos, updatePos)
	}
	if fusPos != updatePos {
		t.Errorf("update: fusion stage %v != posted %v", fusPos, updatePos)
	}

	// No stage may still hold the pre-update value after the position moved.
	if simPos == createPos || regPos == createPos || fusPos == createPos {
		t.Errorf("update: a stage still holds the stale pre-update position %v "+
			"(sim=%v reg=%v fus=%v)", createPos, simPos, regPos, fusPos)
	}

	// Cross-stage consistency: a single node must stay a single node at every
	// stage — an update must not orphan or duplicate it anywhere in the path.
	if got := len(registryNodesByName(t, reg)); got != 1 {
		t.Errorf("update: expected 1 fleet registry node, got %d", got)
	}
	if got := engine.NodeCount(); got != 1 {
		t.Errorf("update: expected 1 fusion engine node, got %d", got)
	}
}

// TestPositionPropagation_ReachesAccumulationGrid (bf-3y9r) closes the last inch
// of the position-propagation chain the prior children wired. bf-95tx proved a
// posted position reaches the fusion engine's nodePos mirror, and the fusion
// package's unit tests prove a populated mirror drives peak formation — but no
// single test tied the two together through the live simulator chain. This one
// does: two nodes posted through the simulator REST API flow through
// registry → engine.nodePos, then one Fuse step with a link between those two
// MACs must localize a blob near the link midpoint. A blob there is only
// possible if BOTH positions reached the accumulation grid (Fuse's
// AddLinkInfluence reads nodePos[mac] for both endpoints; a missing or
// co-located position produces no peak — see TestEngine_CoLocatedOriginYieldsNoPeaks).
// That is the demonstrable "positions reach the fusion engine accumulation grid"
// verification this bead requires.
func TestPositionPropagation_ReachesAccumulationGrid(t *testing.T) {
	// Full production chain, exactly as cmd/mothership/main.go wires it.
	reg := newFleetRegistry(t)
	mgr := fleet.NewManager(reg)
	engine := fusion.NewEngine(nil)
	mgr.SetNodePositionSink(func(mac string, x, y, z float64) {
		engine.SetNodePosition(mac, x, y, z)
	})
	adapter := &fleetRegistryTestAdapter{
		reg:        reg,
		forwardPos: mgr.ForwardNodePosition,
	}
	h := newWiredHandler(t, adapter)

	// Two explicit, in-room positions forming a horizontal link. Their midpoint
	// is the unambiguous blob site a single link localizes to. Neither is at the
	// default origin, so they pass through the bridge unchanged. Coordinates are
	// kept inside both the simulator's DefaultSpace (6×5×2.5) and the engine's
	// default grid (10×3×10): X is the long floor axis, Y/Z shared.
	posA := simulator.Point{X: 0.5, Y: 1.0, Z: 1.0}
	posB := simulator.Point{X: 5.5, Y: 1.0, Z: 1.0}
	midX, midZ := (posA.X+posB.X)/2, (posA.Z+posB.Z)/2

	postNode(t, h, map[string]interface{}{
		"id":       "node-A",
		"name":     "Node A",
		"position": map[string]interface{}{"x": posA.X, "y": posA.Y, "z": posA.Z},
	})
	postNode(t, h, map[string]interface{}{
		"id":       "node-B",
		"name":     "Node B",
		"position": map[string]interface{}{"x": posB.X, "y": posB.Y, "z": posB.Z},
	})

	// Both simulator-created nodes must be positioned in the engine mirror.
	positions := engine.NodePositions()
	if len(positions) != 2 {
		t.Fatalf("expected 2 positioned nodes in engine, got %d: %+v", len(positions), positions)
	}
	// The bridge's virtualMAC is unexported; read the actual MACs the engine holds.
	macA, macB := positions[0].MAC, positions[1].MAC

	// One active link between the two simulator-created nodes. A peak near the
	// midpoint forms only if both endpoints' positions are in the grid lookup.
	links := []fusion.LinkMotion{
		{NodeMAC: macA, PeerMAC: macB, DeltaRMS: 1.0, Motion: true},
	}
	result := engine.Fuse(links)

	if result.ActiveLinks == 0 {
		t.Fatalf("Fuse reported 0 active links — an endpoint position never reached the grid lookup")
	}
	if len(result.Blobs) == 0 {
		t.Fatalf("no blob localized from the link — positions did not reach the accumulation grid")
	}
	top := result.Blobs[0]
	// A single horizontal link localizes its peak near the midpoint (mirrors the
	// tolerance used by TestEngine_SingleLink_MidpointPeak).
	if math.Abs(top.X-midX) > 1.5 {
		t.Errorf("peak X %.2f not near link midpoint %.2f", top.X, midX)
	}
	if math.Abs(top.Z-midZ) > 1.5 {
		t.Errorf("peak Z %.2f not near link midpoint %.2f", top.Z, midZ)
	}
	t.Logf("[grid] position propagation  midpoint=(%.2f,%.2f)  peak=(%.2f,%.2f,%.2f) conf=%.3f activeLinks=%d",
		midX, midZ, top.X, top.Y, top.Z, top.Confidence, result.ActiveLinks)
}
