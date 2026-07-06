// Package api tests the simulator REST API
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spaxel/mothership/internal/simulator"
)

// TestAddNode_SpreadsDefaultOrigin tests that nodes created without explicit
// positions (at origin {0,0,0}) are assigned spread-out positions instead of
// remaining co-located, which would collapse Fresnel excess path length toward
// 0 and prevent blob formation (bf-18yn, bf-4q5w).
func TestAddNode_SpreadsDefaultOrigin(t *testing.T) {
	handler := NewSimulatorHandler()
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
	handler := NewSimulatorHandler()

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
