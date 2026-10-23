package fusion

import (
	"math"
	"testing"
)

// TestNodeFromFleet locks the fleet→engine axis contract: fleet
// (X=width, Y=depth, Z=height) becomes engine (X=width, Y=height, Z=depth).
// The (0,0,1) case is the nodes-table schema default (pos_x=0, pos_y=0,
// pos_z=1), whose engine-frame image (0,1,0) is what the startup
// collapsed-fleet assertion in cmd/mothership must test for.
func TestNodeFromFleet(t *testing.T) {
	cases := []struct {
		name       string
		x, y, z    float64 // fleet frame in
		ex, ey, ez float64 // engine frame out
	}{
		{"mid-room node at 2 m height", 3.0, 4.0, 2.0, 3.0, 2.0, 4.0},
		{"perimeter corner", 0, 0, 2.0, 0, 2.0, 0},
		{"nodes-table schema default", 0, 0, 1, 0, 1, 0},
		{"far corner", 6.0, 5.0, 2.5, 6.0, 2.5, 5.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ex, ey, ez := NodeFromFleet(tc.x, tc.y, tc.z)
			if ex != tc.ex || ey != tc.ey || ez != tc.ez {
				t.Errorf("NodeFromFleet(%.2f, %.2f, %.2f) = (%.2f, %.2f, %.2f), want (%.2f, %.2f, %.2f)",
					tc.x, tc.y, tc.z, ex, ey, ez, tc.ex, tc.ey, tc.ez)
			}
		})
	}
}

// TestNodeFromFleetInvolution verifies the conversion is its own inverse:
// feeding engine-frame values back through it must restore the input.
func TestNodeFromFleetInvolution(t *testing.T) {
	for _, p := range [][3]float64{
		{3.0, 4.0, 2.0},
		{0, 0, 1},
		{6.0, 5.0, 2.5},
		{-1.5, 0.25, 100},
	} {
		x, y, z := NodeFromFleet(NodeFromFleet(p[0], p[1], p[2]))
		if x != p[0] || y != p[1] || z != p[2] {
			t.Errorf("double NodeFromFleet(%v) = (%v, %v, %v), want identity", p, x, y, z)
		}
	}
}

// TestBlobToFleet locks the engine→fleet axis swap and the no-mutation
// guarantee: the input Blob must be left untouched (Result.Blobs aliases the
// engine's cached last result).
func TestBlobToFleet(t *testing.T) {
	in := Blob{X: 2.5, Y: 1.4, Z: 3.75, Confidence: 0.82}
	got := BlobToFleet(in)
	want := Blob{X: 2.5, Y: 3.75, Z: 1.4, Confidence: 0.82}
	if got != want {
		t.Errorf("BlobToFleet(%+v) = %+v, want %+v", in, got, want)
	}
	if in != (Blob{X: 2.5, Y: 1.4, Z: 3.75, Confidence: 0.82}) {
		t.Errorf("BlobToFleet mutated its input: %+v", in)
	}
}

// TestBlobsToFleet covers slice-level behaviour: nil in nil out, values and
// order preserved, and a fresh backing array — callers serve the result
// while the engine keeps its own cached slice.
func TestBlobsToFleet(t *testing.T) {
	if got := BlobsToFleet(nil); got != nil {
		t.Errorf("BlobsToFleet(nil) = %v, want nil", got)
	}

	empty := BlobsToFleet([]Blob{})
	if len(empty) != 0 {
		t.Errorf("BlobsToFleet(empty) len = %d, want 0", len(empty))
	}

	in := []Blob{
		{X: 1, Y: 2, Z: 3, Confidence: 0.5},
		{X: 4, Y: 5, Z: 6, Confidence: 0.6},
	}
	got := BlobsToFleet(in)
	if len(got) != len(in) {
		t.Fatalf("len = %d, want %d", len(got), len(in))
	}
	for i := range in {
		want := Blob{X: in[i].X, Y: in[i].Z, Z: in[i].Y, Confidence: in[i].Confidence}
		if got[i] != want {
			t.Errorf("blob %d = %+v, want %+v", i, got[i], want)
		}
		if math.Abs(got[i].Y-in[i].Z) > 0 { // explicit: axes swapped, not zeroed
			t.Errorf("blob %d Y = %v, want %v", i, got[i].Y, in[i].Z)
		}
	}

	// Mutating the output must not touch the input slice.
	got[0].X = 99
	if in[0].X != 1 {
		t.Errorf("output aliases input backing array: in[0].X = %v, want 1", in[0].X)
	}
}
