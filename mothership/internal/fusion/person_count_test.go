package fusion

// Tests for the person-count fix (spaxel-508fa3ac): one walking person must
// serve as ONE blob, and two people in disjoint corridors as two.
//
// The fragmentation chain this pins: one active link paints a long activation
// ridge; the ridge's plateau and the two voxel rows symmetric about the link
// line promote several strict local maxima a few cells apart; and the tracker
// serves every peak — one walker fragmented into 2-6 blobs (the pre-fix live
// AS-9 median equalled MaxBlobs=6). The fix is two-level: non-maximum
// suppression in Grid3D.Peaks (fragments within MinPeakSeparation of a
// stronger peak are dropped) and a same-single-link merge in Fuse (a peak
// backed by exactly one physical link is a point on that link's ridge; a
// second such peak on the SAME ridge is the same hypothesis, not a second
// person).

import (
	"fmt"
	"math"
	"testing"

	"github.com/spaxel/mothership/internal/simulator"
)

// ---- Non-maximum suppression unit tests ----

// TestGrid3D_Peaks_NonMaximumSuppression pins the separation contract of the
// NMS pass: fragments of one activation ridge (a few cells apart, typically
// the voxel rows symmetric about a link line) collapse to their strongest
// member, while maxima far enough apart both survive. Suppression is
// strongest-first: the kept peak of a suppressed pair is the higher-weighted
// one, and equal weights resolve to the first-scanned voxel.
func TestGrid3D_Peaks_NonMaximumSuppression(t *testing.T) {
	tests := []struct {
		name          string
		voxels        map[[3]int]float64 // (ix, iy, iz) → value; cells are 0.2 m
		minSeparation float64
		wantPeaks     int
		wantFirst     [3]float64 // world coords of the top peak; ignored when 0
	}{
		{
			name: "symmetric ridge rows 0.2 m apart merge to one peak",
			voxels: map[[3]int]float64{
				{2, 4, 2}: 1.0, // world (0.5, 0.9, 0.5)
				{2, 5, 2}: 1.0, // world (0.5, 1.1, 0.5) — the mirrored row
			},
			minSeparation: 0.5,
			wantPeaks:     1,
			wantFirst:     [3]float64{0.5, 0.9, 0.5}, // first-scanned of the tie
		},
		{
			name: "distinct maxima 0.8 m apart both survive",
			voxels: map[[3]int]float64{
				{1, 1, 1}: 1.0,
				{5, 1, 1}: 0.9, // 4 cells away along X
			},
			minSeparation: 0.5,
			wantPeaks:     2,
			wantFirst:     [3]float64{0.3, 0.3, 0.3},
		},
		{
			name: "stronger peak suppresses weaker neighbour",
			voxels: map[[3]int]float64{
				{1, 1, 1}: 0.8,
				{2, 1, 1}: 1.0, // adjacent cell, stronger — kept over the first
			},
			minSeparation: 0.5,
			wantPeaks:     1,
			wantFirst:     [3]float64{0.5, 0.3, 0.3},
		},
		{
			name: "zero separation disables suppression",
			voxels: map[[3]int]float64{
				{2, 4, 2}: 1.0,
				{2, 5, 2}: 1.0,
			},
			minSeparation: 0,
			wantPeaks:     2,
			wantFirst:     [3]float64{0.5, 0.9, 0.5}, // first-scanned of the tie
		},
		{
			name: "diagonal 3D neighbours within separation still merge",
			voxels: map[[3]int]float64{
				{3, 3, 3}: 1.0,
				{4, 4, 4}: 0.95, // 0.2·√3 ≈ 0.35 m away
			},
			minSeparation: 0.5,
			wantPeaks:     1,
			wantFirst:     [3]float64{0.7, 0.7, 0.7},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGrid3D(2, 2.6, 2, 0.2, 0, 0, 0)
			for cell, v := range tt.voxels {
				g.cells[g.idx(cell[0], cell[1], cell[2])] = v
			}
			peaks := g.Peaks(6, 0.5, tt.minSeparation)
			if len(peaks) != tt.wantPeaks {
				t.Fatalf("Peaks(sep=%.1f) returned %d peaks, want %d: %v",
					tt.minSeparation, len(peaks), tt.wantPeaks, peaks)
			}
			if tt.wantPeaks > 0 {
				for axis, want := range tt.wantFirst {
					if math.Abs(peaks[0][axis]-want) > 1e-9 {
						t.Fatalf("top peak axis %d = %f, want %f", axis, peaks[0][axis], want)
					}
				}
			}
		})
	}
}

// ---- Person-count geometry (the AS-9 fixture at unit level) ----

// as9SimNodePositions replicates cmd/sim's createVirtualNodes placement for
// the AS-9 fixture: nodes distributed around the perimeter of a W×D room at
// uniform height 2.0 m (AS-9 passes no --node-heights flag).
func as9SimNodePositions(w, d float64, count int) []simulator.Point {
	perimeter := 2 * (w + d)
	pts := make([]simulator.Point, count)
	for i := 0; i < count; i++ {
		pos := float64(i) / float64(count) * perimeter
		nodeZ := 2.0
		switch {
		case pos < w:
			pts[i] = simulator.Point{X: pos, Y: 0, Z: nodeZ}
		case pos < w+d:
			pts[i] = simulator.Point{X: w, Y: pos - w, Z: nodeZ}
		case pos < 2*w+d:
			pts[i] = simulator.Point{X: w - (pos - w - d), Y: d, Z: nodeZ}
		default:
			pts[i] = simulator.Point{X: 0, Y: d - (pos - 2*w - d), Z: nodeZ}
		}
	}
	return pts
}

// as9NewFixture builds the AS-9 geometry as an in-process fusion engine: a
// 6×5×2.5 room, four perimeter nodes, and the engine configured exactly as
// cmd/mothership constructs it (defaults: MaxBlobs 6, BlobThreshold 0.3,
// MinPeakSeparation 0.5).
func as9NewFixture(t *testing.T) (*Engine, []simulator.Point) {
	t.Helper()
	const (
		w, d, h = 6.0, 5.0, 2.5
	)
	e := NewEngine(&Config{Width: w, Height: h, Depth: d})
	nodes := as9SimNodePositions(w, d, 4)
	for i, p := range nodes {
		// Sim space (X, Y=depth, Z=height) → engine space (X, Y=height, Z=depth).
		e.SetNodePosition(fmt.Sprintf("N%d", i), p.X, p.Z, p.Y)
	}
	return e, nodes
}

// as9LinkMotions converts walker positions into the LinkMotion set the live
// pipeline would feed Fuse: per ordered node pair, the propagation model's
// deltaRMS, gated by the live MotionDetected threshold (smooth > 0.02). With
// multiple walkers a link's activation is the strongest single-walker
// response — each walker activates its own link set independently.
func as9LinkMotions(nodes []simulator.Point, walkers ...simulator.Point) []LinkMotion {
	space := &simulator.Space{Rooms: []simulator.Room{{
		ID: "as9-unit", Name: "AS9 Unit", MinX: 0, MinY: 0, MinZ: 0,
		MaxX: 6, MaxY: 5, MaxZ: 2.5,
	}}}
	prop := simulator.NewPropagationModel(space)

	amps := make(map[int]float64) // ordered pair (i*len+j) → strongest amp
	for _, walker := range walkers {
		for i, a := range nodes {
			for j, b := range nodes {
				if i == j {
					continue
				}
				amp := prop.AmplitudeAt(a, b, walker)
				key := i*len(nodes) + j
				if amp > amps[key] {
					amps[key] = amp
				}
			}
		}
	}
	var links []LinkMotion
	for key, amp := range amps {
		if amp <= 0.02 {
			continue
		}
		i, j := key/len(nodes), key%len(nodes)
		links = append(links, LinkMotion{
			NodeMAC:  fmt.Sprintf("N%d", i),
			PeerMAC:  fmt.Sprintf("N%d", j),
			DeltaRMS: amp,
			Motion:   true,
		})
	}
	return links
}

// TestEngine_SingleWalkerServesOneBlob is the unit-level AS-9 gate (a): a
// single walker patrolling corridor 1 (as9CorridorsJSON in
// test/acceptance/as9_person_count_test.go) must serve exactly ONE blob at
// every position along the corridor. The pre-fix pipeline fragmented one
// walker into 2-6 blobs (live median 6 == MaxBlobs).
//
// Deterministic by construction: fixed geometry, no RNG — the propagation
// model's response to each position is fixed, so unlike the live gate (which
// tolerates a median) this pins every sample.
func TestEngine_SingleWalkerServesOneBlob(t *testing.T) {
	e, nodes := as9NewFixture(t)

	for _, wx := range []float64{0.8, 1.0, 1.2, 1.4, 1.6} {
		for _, wy := range []float64{1.0, 1.5, 2.0, 2.5, 3.0, 3.5, 4.0} {
			walker := simulator.Point{X: wx, Y: wy, Z: 1.7}
			r := e.Fuse(as9LinkMotions(nodes, walker))

			if len(r.Blobs) != 1 {
				t.Errorf("walker (%.1f, %.1f): served %d blobs, want 1: %v",
					wx, wy, len(r.Blobs), r.Blobs)
				continue
			}
			if r.Blobs[0].Confidence < e.blobThresh {
				t.Errorf("walker (%.1f, %.1f): blob confidence %.3f below threshold %.2f",
					wx, wy, r.Blobs[0].Confidence, e.blobThresh)
			}
		}
	}
}

// TestEngine_FragmentMergeKeepsStrongerCrossing pins the merge's positive
// side: two ridges backed by two DIFFERENT physical links (each fed in both
// directions, as the live pipeline does) each serve exactly one blob, and a
// third crossing link's multi-link-backed peak never collapses them.
func TestEngine_FragmentMergeKeepsStrongerCrossing(t *testing.T) {
	e := NewEngine(&Config{
		Width: 10, Height: 3, Depth: 10,
		CellSize: 0.2, MinDeltaRMS: 0.01, MaxBlobs: 6, BlobThreshold: 0.1,
	})
	e.SetNodePosition("A", 0, 1, 0)
	e.SetNodePosition("B", 10, 1, 0)
	e.SetNodePosition("C", 0, 1, 10)
	e.SetNodePosition("D", 10, 1, 10)

	// Two independent ridges (A–B along one wall, C–D along the other), each
	// fed in both directions. Two ridges must serve two blobs — four
	// directional LinkMotion entries, two blobs, not four.
	links := []LinkMotion{
		{NodeMAC: "A", PeerMAC: "B", DeltaRMS: 0.30, Motion: true},
		{NodeMAC: "B", PeerMAC: "A", DeltaRMS: 0.28, Motion: true},
		{NodeMAC: "C", PeerMAC: "D", DeltaRMS: 0.26, Motion: true},
		{NodeMAC: "D", PeerMAC: "C", DeltaRMS: 0.24, Motion: true},
	}
	r := e.Fuse(links)
	if len(r.Blobs) != 2 {
		t.Fatalf("two disjoint ridges served %d blobs, want 2: %v", len(r.Blobs), r.Blobs)
	}
	// The two survivors sit on opposite walls — never merged with each other.
	dz := math.Abs(r.Blobs[0].Z - r.Blobs[1].Z)
	if dz < 5 {
		t.Errorf("surviving blobs %.1f m apart in Z, want one per wall (≥5)", dz)
	}

	// Add a crossing link (A–D diagonal): its peak is backed by the diagonal
	// alone (where it doesn't cross the others) and stays a separate
	// hypothesis; the wall peaks keep their own singletons.
	links = append(links,
		LinkMotion{NodeMAC: "A", PeerMAC: "D", DeltaRMS: 0.22, Motion: true},
		LinkMotion{NodeMAC: "D", PeerMAC: "A", DeltaRMS: 0.22, Motion: true},
	)
	r = e.Fuse(links)
	if len(r.Blobs) < 2 {
		t.Fatalf("crossing link collapsed the count to %d blobs, want ≥ 2: %v",
			len(r.Blobs), r.Blobs)
	}
}
