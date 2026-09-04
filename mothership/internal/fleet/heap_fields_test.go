package fleet

// The fleet package's free_heap_bytes surfaces, pinned by bead
// spaxel-404da1fc ("Verify other response messages free_heap_bytes").
//
// Spaxel has no protobuf layer (no .proto file has ever existed in this
// repo), so the dispatch's protobuf vocabulary resolves onto JSON struct
// tags: the "field type" is the Go field's kind, and "field numbering"
// maps onto the duplicate-key / case-collision hazards that encoding/json
// silently resolves by dropping one of the two keys.
//
// These tests cover the three DB-backed response shapes (NodeRecord,
// FleetNode, fleetNodeEntry) and the read path that feeds them from
// fleet.db. ingestion.NodeInfo — the fourth surface, served on the
// dashboard WebSocket from in-memory state — is covered by
// internal/ingestion/nodeinfo_test.go.
//
// Note on seeding: the tests below write free_heap_bytes straight into the
// nodes table rather than through Registry.UpdateNodeHealth, because that
// method cannot currently execute — its UPDATE names uptime_ms,
// wifi_rssi_dbm, temperature_c, ip and updated_at, columns that exist in
// the OTHER database's nodes table (spaxel.db, internal/db/migrations.go)
// but not in the fleet.db schema this Registry owns. See
// docs/research/free-heap-verification.md for the full finding.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

const heapJSONKey = "free_heap_bytes"

// seedFreeHeapBytes writes a free-heap reading into the nodes table the way
// a working health write would, bypassing the broken Registry.UpdateNodeHealth.
func seedFreeHeapBytes(t *testing.T, reg *Registry, mac string, freeHeapBytes int64) {
	t.Helper()
	if _, err := reg.db.Exec(`UPDATE nodes SET free_heap_bytes=? WHERE mac=?`, freeHeapBytes, mac); err != nil {
		t.Fatalf("seed free_heap_bytes for %s: %v", mac, err)
	}
}

// TestHeapBearingResponseTypesWireShape asserts, for every DB-backed
// response type that carries a free-heap reading, that exactly one exported
// field marshals to the exact key free_heap_bytes, that the field is int64
// (the width HealthMessage.FreeHeapBytes uses on the wire, from which these
// values are ultimately copied), and that no sibling field collides with it
// case-insensitively.
func TestHeapBearingResponseTypesWireShape(t *testing.T) {
	types := []struct {
		name string
		typ  reflect.Type
	}{
		{"NodeRecord (GET /api/nodes/{mac})", reflect.TypeOf(NodeRecord{})},
		{"FleetNode (GET /api/fleet)", reflect.TypeOf(FleetNode{})},
		{"fleetNodeEntry (GET /api/fleet/health)", reflect.TypeOf(fleetNodeEntry{})},
	}

	for _, tt := range types {
		t.Run(tt.name, func(t *testing.T) {
			var heapField reflect.StructField
			matches := 0
			for i := 0; i < tt.typ.NumField(); i++ {
				f := tt.typ.Field(i)
				if f.PkgPath != "" {
					continue // unexported
				}
				name := jsonFieldName(f)
				if name == heapJSONKey {
					heapField = f
					matches++
					continue
				}
				// encoding/json matches keys case-insensitively on
				// unmarshal, so a case-only variant would silently shadow
				// the real field — the JSON analogue of a duplicate field
				// number.
				if strings.EqualFold(name, heapJSONKey) {
					t.Errorf("%s has field %q whose json name %q collides with %q case-insensitively", tt.typ, f.Name, name, heapJSONKey)
				}
			}
			if matches != 1 {
				t.Fatalf("%s has %d fields named %q, want exactly 1", tt.typ, matches, heapJSONKey)
			}

			if got := heapField.Type.Kind(); got != reflect.Int64 {
				t.Errorf("%s.%s kind = %v, want int64 (matches HealthMessage.FreeHeapBytes)", tt.typ, heapField.Name, got)
			}

			// The marshalled key must be exactly free_heap_bytes, matching
			// the firmware sender in firmware/main/websocket.c and the
			// ingestion parser in internal/ingestion/message.go.
			if got := heapField.Tag.Get("json"); got != heapJSONKey {
				t.Errorf("%s.%s json tag = %q, want exactly %q", tt.typ, heapField.Name, got, heapJSONKey)
			}

			// Marshalling the zero value must emit the key exactly once —
			// a duplicate json name in the struct would collapse here.
			wire, err := json.Marshal(reflect.New(tt.typ).Elem().Interface())
			if err != nil {
				t.Fatalf("marshal %s: %v", tt.typ, err)
			}
			if got := strings.Count(string(wire), `"`+heapJSONKey+`"`); got != 1 {
				t.Errorf("marshalled %s contains %q %d times, want exactly 1: %s", tt.typ, heapJSONKey, got, wire)
			}
		})
	}
}

// jsonFieldName returns the name a struct field marshals to, handling the
// no-tag case (encoding/json falls back to the Go field name).
func jsonFieldName(f reflect.StructField) string {
	tag := f.Tag.Get("json")
	if tag == "" {
		return f.Name
	}
	if idx := strings.Index(tag, ","); idx >= 0 {
		tag = tag[:idx]
	}
	if tag == "-" {
		return ""
	}
	return tag
}

// TestFreeHeapBytesColumnPersistsIntoNodeRecord pins the read half of the
// health population chain: the free_heap_bytes column exists in the fleet
// schema, and whatever is stored there is what GetNode and GetAllNodes
// return. Every DB-backed response surface copies its value from here.
func TestFreeHeapBytesColumnPersistsIntoNodeRecord(t *testing.T) {
	tests := []struct {
		name          string
		freeHeapBytes int64
	}{
		{"typical ESP32-S3 reading", 187456},
		{"explicit zero reading", 0},
		{"large reading", 320000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := newTestRegistry(t)
			if err := reg.UpsertNode("AA:BB:CC:DD:EE:01", "1.0.0", "ESP32-S3"); err != nil {
				t.Fatalf("UpsertNode: %v", err)
			}
			seedFreeHeapBytes(t, reg, "AA:BB:CC:DD:EE:01", tt.freeHeapBytes)

			node, err := reg.GetNode("AA:BB:CC:DD:EE:01")
			if err != nil {
				t.Fatalf("GetNode: %v", err)
			}
			if node.FreeHeapBytes != tt.freeHeapBytes {
				t.Errorf("GetNode FreeHeapBytes = %d, want %d", node.FreeHeapBytes, tt.freeHeapBytes)
			}

			nodes, err := reg.GetAllNodes()
			if err != nil {
				t.Fatalf("GetAllNodes: %v", err)
			}
			if len(nodes) != 1 || nodes[0].FreeHeapBytes != tt.freeHeapBytes {
				t.Errorf("GetAllNodes free heap = %v, want [%d]", nodes, tt.freeHeapBytes)
			}
		})
	}

	t.Run("node with no health report reads as schema default 0", func(t *testing.T) {
		reg := newTestRegistry(t)
		if err := reg.UpsertNode("AA:BB:CC:DD:EE:02", "1.0.0", "ESP32-S3"); err != nil {
			t.Fatalf("UpsertNode: %v", err)
		}
		node, err := reg.GetNode("AA:BB:CC:DD:EE:02")
		if err != nil {
			t.Fatalf("GetNode: %v", err)
		}
		if node.FreeHeapBytes != 0 {
			t.Errorf("FreeHeapBytes = %d, want 0 (no health report has arrived)", node.FreeHeapBytes)
		}
	})
}

// TestListFleetServesHeapFromRegistry exercises the GET /api/fleet path end
// to end: a stored free-heap reading must reach the free_heap_bytes key of
// the response, and a node that has never reported health must serve 0
// rather than being omitted.
func TestListFleetServesHeapFromRegistry(t *testing.T) {
	const (
		reportedMAC  = "AA:BB:CC:DD:EE:03"
		quietMAC     = "AA:BB:CC:DD:EE:04"
		reportedHeap = int64(214500)
	)

	reg := newTestRegistry(t)
	for _, mac := range []string{reportedMAC, quietMAC} {
		if err := reg.UpsertNode(mac, "1.0.0", "ESP32-S3"); err != nil {
			t.Fatalf("UpsertNode(%s): %v", mac, err)
		}
	}
	seedFreeHeapBytes(t, reg, reportedMAC, reportedHeap)

	h := &Handler{
		mgr: NewManager(reg),
		nodeID: &mockNodeIdentifier{
			getConnectedMACs: func() []string { return []string{reportedMAC} },
			getUnpairedMACs:  func() []string { return nil },
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/fleet", nil)
	w := httptest.NewRecorder()
	h.listFleet(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("listFleet() status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Nodes []FleetNode `json:"nodes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode listFleet response: %v", err)
	}

	heaps := make(map[string]int64, len(resp.Nodes))
	for _, n := range resp.Nodes {
		heaps[n.MAC] = n.FreeHeapBytes
	}
	if _, ok := heaps[reportedMAC]; !ok {
		t.Fatalf("node %s missing from /api/fleet response: %s", reportedMAC, w.Body.String())
	}
	if got := heaps[reportedMAC]; got != reportedHeap {
		t.Errorf("free_heap_bytes for %s = %d, want %d", reportedMAC, got, reportedHeap)
	}
	if got := heaps[quietMAC]; got != 0 {
		t.Errorf("free_heap_bytes for %s = %d, want 0 (never reported health)", quietMAC, got)
	}
}

// TestGetNodeServesHeapFromRegistry does the same for GET /api/nodes/{mac},
// whose body is the NodeRecord itself.
func TestGetNodeServesHeapFromRegistry(t *testing.T) {
	const (
		mac  = "AA:BB:CC:DD:EE:05"
		heap = int64(96500)
	)

	reg := newTestRegistry(t)
	if err := reg.UpsertNode(mac, "1.0.0", "ESP32-S3"); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}
	seedFreeHeapBytes(t, reg, mac, heap)

	h := &Handler{mgr: NewManager(reg)}

	req := httptest.NewRequest(http.MethodGet, "/api/nodes/"+mac, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("mac", mac)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.getNode(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("getNode() status = %d, want %d", w.Code, http.StatusOK)
	}

	var raw struct {
		FreeHeapBytes *int64 `json:"free_heap_bytes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode getNode response: %v", err)
	}
	if raw.FreeHeapBytes == nil {
		t.Fatalf("response has no free_heap_bytes key at all: %s", w.Body.String())
	}
	if *raw.FreeHeapBytes != heap {
		t.Errorf("free_heap_bytes = %d, want %d", *raw.FreeHeapBytes, heap)
	}
}

// TestGetFleetHealthServesHeapFromRegistry covers the third DB-backed
// surface, GET /api/fleet/health, which projects the registry value into
// fleetNodeEntry.
func TestGetFleetHealthServesHeapFromRegistry(t *testing.T) {
	const (
		mac  = "AA:BB:CC:DD:EE:06"
		heap = int64(178200)
	)

	reg := newTestRegistry(t)
	if err := reg.UpsertNode(mac, "1.0.0", "ESP32-S3"); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}
	seedFreeHeapBytes(t, reg, mac, heap)

	h := NewFleetHandler(NewSelfHealManager(reg, NewRoleOptimiser(DefaultOptimisationConfig()), DefaultSelfHealConfig()), reg)

	req := httptest.NewRequest(http.MethodGet, "/api/fleet/health", nil)
	w := httptest.NewRecorder()
	h.getFleetHealth(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("getFleetHealth() status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Nodes []fleetNodeEntry `json:"nodes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode getFleetHealth response: %v", err)
	}

	for _, e := range resp.Nodes {
		if e.MAC != mac {
			continue
		}
		if e.FreeHeapBytes != heap {
			t.Errorf("free_heap_bytes for %s = %d, want %d", mac, e.FreeHeapBytes, heap)
		}
		return
	}
	t.Fatalf("node %s missing from /api/fleet/health response: %s", mac, w.Body.String())
}
