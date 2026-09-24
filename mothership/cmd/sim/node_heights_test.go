package main

import (
	"math"
	"math/rand"
	"testing"
)

// TestParseNodeHeights covers --node-heights parsing: empty (flag absent)
// keeps the uniform placement, "mixed" selects the engine parity scheme
// (case/whitespace tolerant), and anything else is an error rather than a
// silent fallback.
func TestParseNodeHeights(t *testing.T) {
	tests := []struct {
		spec    string
		want    nodeHeightMode
		wantErr bool
	}{
		{spec: "", want: uniformHeightMode},
		{spec: "mixed", want: mixedHeightMode},
		{spec: "  Mixed ", want: mixedHeightMode},
		{spec: "MIXED", want: mixedHeightMode},
		{spec: "2.4,0.9", wantErr: true},
		{spec: "uniform", wantErr: true},
		{spec: "bogus", wantErr: true},
	}

	for _, tt := range tests {
		got, err := parseNodeHeights(tt.spec)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseNodeHeights(%q): expected error, got mode %v", tt.spec, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseNodeHeights(%q): unexpected error: %v", tt.spec, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseNodeHeights(%q): expected mode %v, got %v", tt.spec, tt.want, got)
		}
	}
}

// TestCreateVirtualNodes_UniformDefault pins the default (flag absent)
// placement: every node stays at the historical Z = 2.0 m perimeter layout,
// unchanged from before --node-heights existed.
func TestCreateVirtualNodes_UniformDefault(t *testing.T) {
	space := &Space{Width: 6, Depth: 5, Height: 3.0}

	for _, count := range []int{1, 2, 3, 4, 6, 8} {
		nodes := createVirtualNodes(count, space, rand.New(rand.NewSource(42)), uniformHeightMode)
		if len(nodes) != count {
			t.Fatalf("count=%d: expected %d nodes, got %d", count, count, len(nodes))
		}
		for _, node := range nodes {
			if node.Position.Z != 2.0 {
				t.Errorf("count=%d node %d: default placement must stay at Z=2.0, got %v",
					count, node.ID, node.Position.Z)
			}
		}
	}
}

// TestCreateVirtualNodes_MixedHeights asserts --node-heights mixed places
// perimeter nodes on the engine parity bands — even slots at 0.25 of room
// height, odd slots at 0.75 — giving >= 2 distinct Z values for count >= 2,
// while leaving the perimeter X/Y layout identical to the default mode.
func TestCreateVirtualNodes_MixedHeights(t *testing.T) {
	space := &Space{Width: 6, Depth: 5, Height: 3.0}
	lowZ := 0.25 * space.Height
	highZ := 0.75 * space.Height

	for _, count := range []int{2, 3, 4, 6, 8} {
		nodes := createVirtualNodes(count, space, rand.New(rand.NewSource(42)), mixedHeightMode)
		if len(nodes) != count {
			t.Fatalf("count=%d: expected %d nodes, got %d", count, count, len(nodes))
		}

		distinct := map[float64]bool{}
		for _, node := range nodes {
			want := lowZ
			if node.ID%2 != 0 {
				want = highZ
			}
			if node.Position.Z != want {
				t.Errorf("count=%d node %d: expected parity Z %v, got %v",
					count, node.ID, want, node.Position.Z)
			}
			distinct[node.Position.Z] = true

			if node.Position.Z < 0 || node.Position.Z > space.Height {
				t.Errorf("count=%d node %d: Z %v outside room height %v",
					count, node.ID, node.Position.Z, space.Height)
			}
		}
		if len(distinct) < 2 {
			t.Errorf("count=%d: mixed placement must produce >= 2 distinct Z values, got %v",
				count, distinct)
		}
	}

	// Single node: only the low band is reachable, still within the room.
	one := createVirtualNodes(1, space, rand.New(rand.NewSource(42)), mixedHeightMode)
	if one[0].Position.Z != lowZ {
		t.Errorf("count=1: expected low-band Z %v, got %v", lowZ, one[0].Position.Z)
	}

	// X/Y placement is unchanged by the height mode: perimeter geometry is
	// identical between uniform and mixed for the same count.
	uniform := createVirtualNodes(6, space, rand.New(rand.NewSource(42)), uniformHeightMode)
	mixed := createVirtualNodes(6, space, rand.New(rand.NewSource(42)), mixedHeightMode)
	for i := range uniform {
		if uniform[i].Position.X != mixed[i].Position.X ||
			uniform[i].Position.Y != mixed[i].Position.Y {
			t.Errorf("node %d: mixed mode changed X/Y placement: uniform (%v,%v) vs mixed (%v,%v)",
				i, uniform[i].Position.X, uniform[i].Position.Y, mixed[i].Position.X, mixed[i].Position.Y)
		}
	}
}

// TestCreateVirtualNodes_DeterministicUnderSeed asserts the AC determinism
// guarantee: the same --seed implies the same node Z assignment (and full
// position assignment), and the Z scheme itself is index-derived so it holds
// across seeds too.
func TestCreateVirtualNodes_DeterministicUnderSeed(t *testing.T) {
	space := &Space{Width: 6, Depth: 5, Height: 3.0}

	run := func() [][3]float64 {
		nodes := createVirtualNodes(6, space, rand.New(rand.NewSource(42)), mixedHeightMode)
		positions := make([][3]float64, len(nodes))
		for i, node := range nodes {
			positions[i] = [3]float64{node.Position.X, node.Position.Y, node.Position.Z}
		}
		return positions
	}

	first, second := run(), run()
	for i := range first {
		if first[i] != second[i] {
			t.Errorf("seed 42 not deterministic at node %d: %v vs %v", i, first[i], second[i])
		}
	}

	// Z assignment is parity-of-index, independent of the RNG stream.
	for _, seed := range []int64{1, 42, 999} {
		nodes := createVirtualNodes(6, space, rand.New(rand.NewSource(seed)), mixedHeightMode)
		for _, node := range nodes {
			want := 0.25 * space.Height
			if node.ID%2 != 0 {
				want = 0.75 * space.Height
			}
			if math.Abs(node.Position.Z-want) > 1e-9 {
				t.Errorf("seed %d node %d: Z %v != parity band %v", seed, node.ID, node.Position.Z, want)
			}
		}
	}
}
