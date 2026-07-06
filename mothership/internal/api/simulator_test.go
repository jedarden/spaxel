// Package api tests the simulator REST API
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
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
	mu          sync.Mutex
	added       []simulator.NodeRecord
	posUpdates  []simulator.NodeRecord
	nodesByMAC  map[string]simulator.NodeRecord
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
				"id": "node-A",
				"name": "Node A",
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
				"id":   "node-A",
				"name": "Node A",
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
