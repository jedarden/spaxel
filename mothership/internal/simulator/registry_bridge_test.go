package simulator

import (
	"fmt"
	"testing"
)

// fakeRegistry is an in-memory RegistryNodeAdapter that records every mutation
// so bridge tests can assert what was written to the registry (and in what order).
type fakeRegistry struct {
	nodes        map[string]NodeRecord
	addCalls     []NodeRecord
	setPosCalls  []NodeRecord
	setRoleCalls []macRole
}

type macRole struct {
	MAC, Role string
}

func newFakeRegistry() *fakeRegistry {
	return &fakeRegistry{nodes: make(map[string]NodeRecord)}
}

func (f *fakeRegistry) AddVirtualNode(mac, name string, x, y, z float64) error {
	rec := NodeRecord{MAC: mac, Name: name, PosX: x, PosY: y, PosZ: z, Virtual: true, Enabled: true}
	f.nodes[mac] = rec
	f.addCalls = append(f.addCalls, rec)
	return nil
}

func (f *fakeRegistry) SetNodePosition(mac string, x, y, z float64) error {
	rec := f.nodes[mac]
	rec.PosX, rec.PosY, rec.PosZ = x, y, z
	f.nodes[mac] = rec
	f.setPosCalls = append(f.setPosCalls, NodeRecord{MAC: mac, PosX: x, PosY: y, PosZ: z})
	return nil
}

func (f *fakeRegistry) SetNodeRole(mac, role string) error {
	rec := f.nodes[mac]
	rec.Role = role
	f.nodes[mac] = rec
	f.setRoleCalls = append(f.setRoleCalls, macRole{MAC: mac, Role: role})
	return nil
}

func (f *fakeRegistry) DeleteNode(mac string) error {
	delete(f.nodes, mac)
	return nil
}

func (f *fakeRegistry) GetNode(mac string) (*NodeRecord, error) {
	rec, ok := f.nodes[mac]
	if !ok {
		return nil, fmt.Errorf("not found: %s", mac)
	}
	return &rec, nil
}

func (f *fakeRegistry) GetAllNodes() ([]NodeRecord, error) {
	out := make([]NodeRecord, 0, len(f.nodes))
	for _, r := range f.nodes {
		out = append(out, r)
	}
	return out, nil
}

// storeWithNodes builds a fresh store (DefaultSpace) holding one virtual node
// per entry. Each entry gives (id, position). Positions at the default origin
// are created as-is (the origin is in-bounds for DefaultSpace).
func storeWithNodes(t *testing.T, nodes ...struct {
	ID  string
	Pos Point
}) *VirtualNodeStore {
	t.Helper()
	store, _ := tempStore(t)
	for _, n := range nodes {
		if _, err := store.CreateVirtualNode(n.ID, n.ID, n.Pos); err != nil {
			t.Fatalf("create %s: %v", n.ID, err)
		}
	}
	return store
}

// originStore builds a store with count nodes all at the default DB origin.
func originStore(t *testing.T, count int) *VirtualNodeStore {
	t.Helper()
	entries := make([]struct {
		ID  string
		Pos Point
	}, 0, count)
	for i := 0; i < count; i++ {
		entries = append(entries, struct {
			ID  string
			Pos Point
		}{
			ID:  fmt.Sprintf("node-%d", i+1),
			Pos: DefaultNodeOrigin,
		})
	}
	return storeWithNodes(t, entries...)
}

// assertDistinctNonOrigin fails the test if any record is still at the default
// origin or if any two records share the same position.
func assertDistinctNonOrigin(t *testing.T, recs []NodeRecord) {
	t.Helper()
	seen := make(map[Point]bool, len(recs))
	for _, r := range recs {
		p := Point{X: r.PosX, Y: r.PosY, Z: r.PosZ}
		if isDefaultOrigin(p) {
			t.Errorf("node %s still at default origin %v", r.MAC, p)
		}
		if seen[p] {
			t.Errorf("co-located registry nodes at %v (mac %s)", p, r.MAC)
		}
		seen[p] = true
	}
}

func TestIsDefaultOrigin(t *testing.T) {
	cases := []struct {
		name string
		p    Point
		want bool
	}{
		{"db default origin", DefaultNodeOrigin, true},
		{"explicit zero", Point{X: 0, Y: 0, Z: 0}, false},
		{"origin wrong Z", Point{X: 0, Y: 0, Z: 1.5}, false},
		{"origin shifted X", Point{X: 1, Y: 0, Z: 1}, false},
		{"placed node", Point{X: 3, Y: 2, Z: 1.5}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isDefaultOrigin(c.p); got != c.want {
				t.Errorf("isDefaultOrigin(%v) = %v, want %v", c.p, got, c.want)
			}
		})
	}
}

// TestSyncToRegistry_OriginNodesGetSpreadGeometry is the core acceptance check:
// a fleet of nodes created at the default origin must be written to the
// registry at distinct, non-degenerate positions (spread across the room).
func TestSyncToRegistry_OriginNodesGetSpreadGeometry(t *testing.T) {
	for _, count := range []int{1, 2, 3, 4, 6, 9} {
		t.Run(fmt.Sprintf("count=%d", count), func(t *testing.T) {
			store := originStore(t, count)
			bridge := NewFleetRegistryBridge(store)
			reg := newFakeRegistry()

			if err := bridge.SyncToRegistry(reg); err != nil {
				t.Fatalf("SyncToRegistry: %v", err)
			}

			if len(reg.nodes) != count {
				t.Fatalf("expected %d registry nodes, got %d", count, len(reg.nodes))
			}
			if len(reg.addCalls) != count {
				t.Fatalf("expected %d AddVirtualNode calls, got %d", count, len(reg.addCalls))
			}

			recs := make([]NodeRecord, 0, len(reg.nodes))
			for _, r := range reg.nodes {
				recs = append(recs, r)
			}
			assertDistinctNonOrigin(t, recs)
		})
	}
}

// TestSyncToRegistry_ExplicitPositionsPreserved verifies nodes placed at a real
// (non-origin) position are written through unchanged — the bridge only
// substitutes geometry for unset nodes.
func TestSyncToRegistry_ExplicitPositionsPreserved(t *testing.T) {
	a := Point{X: 1, Y: 1, Z: 1.5}
	b := Point{X: 4, Y: 4, Z: 2}
	store := storeWithNodes(t,
		struct {
			ID  string
			Pos Point
		}{"a", a},
		struct {
			ID  string
			Pos Point
		}{"b", b},
	)
	bridge := NewFleetRegistryBridge(store)
	reg := newFakeRegistry()

	if err := bridge.SyncToRegistry(reg); err != nil {
		t.Fatalf("SyncToRegistry: %v", err)
	}

	for id, want := range map[string]Point{"a": a, "b": b} {
		rec, ok := reg.nodes[bridge.virtualMAC(id)]
		if !ok {
			t.Fatalf("node %s missing from registry", id)
		}
		got := Point{X: rec.PosX, Y: rec.PosY, Z: rec.PosZ}
		if got != want {
			t.Errorf("node %s: expected explicit %v, got %v", id, want, got)
		}
	}
}

// TestSyncToRegistry_MixedNodes keeps explicit placements while spreading the
// unset ones, and asserts the final set is distinct and non-origin.
func TestSyncToRegistry_MixedNodes(t *testing.T) {
	store := storeWithNodes(t,
		struct {
			ID  string
			Pos Point
		}{"explicit", Point{X: 5, Y: 4, Z: 2}},
		struct {
			ID  string
			Pos Point
		}{"o1", DefaultNodeOrigin},
		struct {
			ID  string
			Pos Point
		}{"o2", DefaultNodeOrigin},
		struct {
			ID  string
			Pos Point
		}{"o3", DefaultNodeOrigin},
	)
	bridge := NewFleetRegistryBridge(store)
	reg := newFakeRegistry()

	if err := bridge.SyncToRegistry(reg); err != nil {
		t.Fatalf("SyncToRegistry: %v", err)
	}

	// Explicit node keeps its position.
	rec := reg.nodes[bridge.virtualMAC("explicit")]
	if (Point{X: rec.PosX, Y: rec.PosY, Z: rec.PosZ}) != (Point{X: 5, Y: 4, Z: 2}) {
		t.Errorf("explicit node moved: got %v", rec)
	}

	recs := make([]NodeRecord, 0, len(reg.nodes))
	for _, r := range reg.nodes {
		recs = append(recs, r)
	}
	assertDistinctNonOrigin(t, recs)
}

// TestSyncToRegistry_Idempotent asserts a second sync is a no-op: because the
// spread slot assignment is deterministic, the registry already holds the
// effective positions and no further writes occur.
func TestSyncToRegistry_Idempotent(t *testing.T) {
	store := originStore(t, 4)
	bridge := NewFleetRegistryBridge(store)
	reg := newFakeRegistry()

	if err := bridge.SyncToRegistry(reg); err != nil {
		t.Fatalf("first SyncToRegistry: %v", err)
	}
	firstAdds, firstSets := len(reg.addCalls), len(reg.setPosCalls)

	if err := bridge.SyncToRegistry(reg); err != nil {
		t.Fatalf("second SyncToRegistry: %v", err)
	}

	if len(reg.addCalls) != firstAdds {
		t.Errorf("second sync added nodes again: %d != %d", len(reg.addCalls), firstAdds)
	}
	if len(reg.setPosCalls) != firstSets {
		t.Errorf("second sync rewrote positions: %d != %d", len(reg.setPosCalls), firstSets)
	}
}

// TestSyncOneNode_MatchesFullSync asserts a single-node sync writes the same
// effective position that a full sync would — including spread geometry for a
// default-origin node.
func TestSyncOneNode_MatchesFullSync(t *testing.T) {
	store := originStore(t, 3)

	fullReg := newFakeRegistry()
	bridge := NewFleetRegistryBridge(store)
	if err := bridge.SyncToRegistry(fullReg); err != nil {
		t.Fatalf("SyncToRegistry: %v", err)
	}

	target := "node-2"
	mac := bridge.virtualMAC(target)
	wantRec, ok := fullReg.nodes[mac]
	if !ok {
		t.Fatalf("node %s missing from full sync", target)
	}

	oneReg := newFakeRegistry()
	if err := bridge.SyncOneNode(oneReg, target); err != nil {
		t.Fatalf("SyncOneNode: %v", err)
	}
	gotRec, ok := oneReg.nodes[mac]
	if !ok {
		t.Fatalf("node %s not written by SyncOneNode", target)
	}

	want := Point{X: wantRec.PosX, Y: wantRec.PosY, Z: wantRec.PosZ}
	got := Point{X: gotRec.PosX, Y: gotRec.PosY, Z: gotRec.PosZ}
	if got != want {
		t.Errorf("SyncOneNode position %v != SyncToRegistry position %v", got, want)
	}
	if isDefaultOrigin(got) {
		t.Errorf("SyncOneNode left default-origin node at the origin: %v", got)
	}
}

// TestSyncOneNode_ExplicitPreserved asserts SyncOneNode passes an explicit
// (non-origin) position through unchanged.
func TestSyncOneNode_ExplicitPreserved(t *testing.T) {
	store := storeWithNodes(t, struct {
		ID  string
		Pos Point
	}{"a", Point{X: 2, Y: 3, Z: 1.2}})
	bridge := NewFleetRegistryBridge(store)
	reg := newFakeRegistry()

	if err := bridge.SyncOneNode(reg, "a"); err != nil {
		t.Fatalf("SyncOneNode: %v", err)
	}
	rec := reg.nodes[bridge.virtualMAC("a")]
	got := Point{X: rec.PosX, Y: rec.PosY, Z: rec.PosZ}
	if got != (Point{X: 2, Y: 3, Z: 1.2}) {
		t.Errorf("expected explicit position preserved, got %v", got)
	}
}

// TestSyncToRegistry_NilRegistry guards the nil-adapter guard.
func TestSyncToRegistry_NilRegistry(t *testing.T) {
	store := originStore(t, 2)
	bridge := NewFleetRegistryBridge(store)
	if err := bridge.SyncToRegistry(nil); err == nil {
		t.Fatal("expected error syncing to nil registry")
	}
	if err := bridge.SyncOneNode(nil, "node-1"); err == nil {
		t.Fatal("expected error syncing one node to nil registry")
	}
}

// TestToRegistryRecords_UsesEffectivePositions ensures the record view matches
// what SyncToRegistry writes (spread geometry for origin nodes).
func TestToRegistryRecords_UsesEffectivePositions(t *testing.T) {
	store := originStore(t, 3)
	bridge := NewFleetRegistryBridge(store)

	records := bridge.ToRegistryRecords()
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}
	assertDistinctNonOrigin(t, records)

	// Cross-check against an actual sync.
	reg := newFakeRegistry()
	if err := bridge.SyncToRegistry(reg); err != nil {
		t.Fatalf("SyncToRegistry: %v", err)
	}
	for _, r := range records {
		synced := reg.nodes[r.MAC]
		if synced.PosX != r.PosX || synced.PosY != r.PosY || synced.PosZ != r.PosZ {
			t.Errorf("record %s %v != synced %v", r.MAC, r, synced)
		}
	}
}
