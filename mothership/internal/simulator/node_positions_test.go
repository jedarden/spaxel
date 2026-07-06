package simulator

import (
	"math"
	"strconv"
	"testing"
)

// TestDefaultNodePositions_CountZeroAndNegative asserts that non-positive
// counts return an empty (non-nil) slice rather than panicking.
func TestDefaultNodePositions_CountZeroAndNegative(t *testing.T) {
	for _, count := range []int{0, -1, -5} {
		got := DefaultNodePositions(DefaultSpace(), count)
		if len(got) != 0 {
			t.Errorf("count=%d: expected empty slice, got %d positions", count, len(got))
		}
	}
}

// TestDefaultNodePositions_CountAndBounds asserts the helper returns exactly
// count positions, all within the space's bounding box, across a range of fleet
// sizes — including a space with a non-zero origin.
func TestDefaultNodePositions_CountAndBounds(t *testing.T) {
	spaces := map[string]*Space{
		"default": DefaultSpace(), // 6x5x2.5 at origin
		"offset": {
			ID: "offset",
			Rooms: []Room{{
				ID: "r", Name: "R",
				MinX: 10, MinY: -3, MinZ: 1,
				MaxX: 16, MaxY: 2, MaxZ: 3.5,
			}},
		},
	}

	for name, space := range spaces {
		minX, minY, minZ, maxX, maxY, maxZ := space.Bounds()
		for _, count := range []int{1, 2, 3, 4, 5, 6, 8, 9, 16, 25} {
			t.Run(name+"/"+strconv.Itoa(count), func(t *testing.T) {
				got := DefaultNodePositions(space, count)
				if len(got) != count {
					t.Fatalf("expected %d positions, got %d", count, len(got))
				}
				for i, p := range got {
					if p.X < minX-tol || p.X > maxX+tol ||
						p.Y < minY-tol || p.Y > maxY+tol ||
						p.Z < minZ-tol || p.Z > maxZ+tol {
						t.Errorf("node %d %v out of bounds min=(%v,%v,%v) max=(%v,%v,%v)",
							i, p, minX, minY, minZ, maxX, maxY, maxZ)
					}
				}
			})
		}
	}
}

// TestDefaultNodePositions_Distinct asserts that no two returned nodes are
// co-located: every pairwise distance is strictly greater than zero. This is
// the core non-degeneracy guarantee the helper exists to provide.
func TestDefaultNodePositions_Distinct(t *testing.T) {
	space := DefaultSpace()
	for _, count := range []int{1, 2, 3, 4, 5, 8, 16, 25} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			got := DefaultNodePositions(space, count)
			for i := 0; i < len(got); i++ {
				for j := i + 1; j < len(got); j++ {
					d := got[i].Distance(got[j])
					if d <= 0 {
						t.Errorf("count=%d: nodes %d and %d co-located at %v (dist=%v)",
							count, i, j, got[i], d)
					}
				}
			}
		})
	}
}

// TestDefaultNodePositions_SpansRoom asserts that for count >= 2 the positions
// span the room on both X and Y: the min and max coordinates differ on each
// axis. A degenerate line or single-point cluster would fail this.
func TestDefaultNodePositions_SpansRoom(t *testing.T) {
	space := DefaultSpace()
	minX, minY, _, maxX, maxY, _ := space.Bounds()
	width := maxX - minX
	depth := maxY - minY

	for _, count := range []int{2, 3, 4, 5, 6, 8, 9, 16, 25} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			got := DefaultNodePositions(space, count)
			if len(got) < 2 {
				t.Fatalf("expected >=2 positions, got %d", len(got))
			}

			minPX, maxPX := got[0].X, got[0].X
			minPY, maxPY := got[0].Y, got[0].Y
			for _, p := range got[1:] {
				minPX = math.Min(minPX, p.X)
				maxPX = math.Max(maxPX, p.X)
				minPY = math.Min(minPY, p.Y)
				maxPY = math.Max(maxPY, p.Y)
			}

			// Span must be a meaningful fraction of the room, not just an
			// epsilon. Require it to cover at least half of each axis.
			if (maxPX - minPX) < width*0.5 {
				t.Errorf("count=%d: X span %v < half width %v", count, maxPX-minPX, width*0.5)
			}
			if (maxPY - minPY) < depth*0.5 {
				t.Errorf("count=%d: Y span %v < half depth %v", count, maxPY-minPY, depth*0.5)
			}
		})
	}
}

// TestDefaultNodePositions_SingleNode is the degenerate case: one node sits at
// the room center (no spanning is possible with a single point).
func TestDefaultNodePositions_SingleNode(t *testing.T) {
	space := DefaultSpace()
	got := DefaultNodePositions(space, 1)
	if len(got) != 1 {
		t.Fatalf("expected 1 position, got %d", len(got))
	}
	minX, minY, minZ, maxX, maxY, maxZ := space.Bounds()
	want := Point{X: (minX + maxX) / 2, Y: (minY + maxY) / 2, Z: (minZ + maxZ) / 2}
	if got[0] != want {
		t.Errorf("expected center %v, got %v", want, got[0])
	}
}

const tol = 1e-9
