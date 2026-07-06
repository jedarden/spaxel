package fusion

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/spaxel/mothership/internal/simulator"
)

// ---- Grid3D unit tests ----

func TestGrid3D_Reset(t *testing.T) {
	g := NewGrid3D(4, 3, 4, 0.5, 0, 0, 0)
	g.AddLinkInfluence(0, 1, 0, 4, 1, 4, 0.5)
	g.Reset()
	for i, v := range g.Snapshot() {
		if v != 0 {
			t.Fatalf("cell %d not zero after Reset: %f", i, v)
		}
	}
}

func TestGrid3D_AddLinkInfluence_Degenerate(t *testing.T) {
	g := NewGrid3D(4, 3, 4, 0.5, 0, 0, 0)
	// Same TX/RX position — should be a no-op.
	g.AddLinkInfluence(2, 1, 2, 2, 1, 2, 1.0)
	for i, v := range g.Snapshot() {
		if v != 0 {
			t.Fatalf("expected zero for degenerate link, cell %d = %f", i, v)
		}
	}
	// Zero activation — no-op.
	g.AddLinkInfluence(0, 1, 0, 4, 1, 4, 0)
	for i, v := range g.Snapshot() {
		if v != 0 {
			t.Fatalf("expected zero for zero activation, cell %d = %f", i, v)
		}
	}
}

func TestGrid3D_Normalize(t *testing.T) {
	g := NewGrid3D(4, 3, 4, 1.0, 0, 0, 0)
	g.AddLinkInfluence(0, 1, 0, 4, 1, 4, 1.0)
	ok := g.Normalize()
	if !ok {
		t.Fatal("Normalize returned false on non-empty grid")
	}
	maxVal := 0.0
	for _, v := range g.Snapshot() {
		if v > maxVal {
			maxVal = v
		}
	}
	if math.Abs(maxVal-1.0) > 1e-9 {
		t.Fatalf("max after Normalize = %f, want 1.0", maxVal)
	}
}

func TestGrid3D_NormalizeEmpty(t *testing.T) {
	g := NewGrid3D(4, 3, 4, 1.0, 0, 0, 0)
	if g.Normalize() {
		t.Fatal("Normalize should return false on empty grid")
	}
}

// ---- Fresnel zone geometry ----

func TestFresnelZoneRadius(t *testing.T) {
	// For a 5 m link at 2.4 GHz: r1 = sqrt(0.125 * 5 / 4) ≈ 0.395 m
	r := FresnelZoneRadius(5.0)
	expected := math.Sqrt(0.125 * 5.0 / 4.0)
	if math.Abs(r-expected) > 1e-9 {
		t.Fatalf("FresnelZoneRadius(5) = %f, want %f", r, expected)
	}
}

// ---- Fusion engine tests ----

func makeEngine(w, h, d float64) *Engine {
	return NewEngine(&Config{
		Width: w, Height: h, Depth: d,
		CellSize:      0.2,
		MinDeltaRMS:   0.01,
		MaxBlobs:      6,
		BlobThreshold: 0.3,
	})
}

func TestEngine_NoLinks(t *testing.T) {
	e := makeEngine(10, 3, 10)
	r := e.Fuse(nil)
	if r == nil {
		t.Fatal("nil result")
	}
	if len(r.Blobs) != 0 {
		t.Fatalf("expected 0 blobs with no links, got %d", len(r.Blobs))
	}
	if r.ActiveLinks != 0 {
		t.Fatalf("expected 0 active links, got %d", r.ActiveLinks)
	}
}

func TestEngine_NoMotionLinks(t *testing.T) {
	e := makeEngine(10, 3, 10)
	e.SetNodePosition("A", 0, 1, 0)
	e.SetNodePosition("B", 5, 1, 5)
	links := []LinkMotion{
		{NodeMAC: "A", PeerMAC: "B", DeltaRMS: 0.5, Motion: false},
	}
	r := e.Fuse(links)
	if len(r.Blobs) != 0 {
		t.Fatalf("expected 0 blobs when Motion=false, got %d", len(r.Blobs))
	}
}

func TestEngine_BelowThresholdLink(t *testing.T) {
	e := makeEngine(10, 3, 10)
	e.SetNodePosition("A", 0, 1, 0)
	e.SetNodePosition("B", 5, 1, 5)
	links := []LinkMotion{
		{NodeMAC: "A", PeerMAC: "B", DeltaRMS: 0.005, Motion: true},
	}
	r := e.Fuse(links)
	if r.ActiveLinks != 0 {
		t.Fatalf("expected link filtered by minDelta, got %d active", r.ActiveLinks)
	}
}

func TestEngine_MissingNodePosition(t *testing.T) {
	e := makeEngine(10, 3, 10)
	// Only register one of the two nodes.
	e.SetNodePosition("A", 0, 1, 0)
	links := []LinkMotion{
		{NodeMAC: "A", PeerMAC: "B", DeltaRMS: 0.5, Motion: true},
	}
	r := e.Fuse(links)
	if r.ActiveLinks != 0 {
		t.Fatalf("expected link skipped for missing position, got %d active", r.ActiveLinks)
	}
}

// TestEngine_SingleLink_MidpointPeak checks that the peak lies near the
// midpoint of a single crossing link.
func TestEngine_SingleLink_MidpointPeak(t *testing.T) {
	e := NewEngine(&Config{
		Width: 10, Height: 3, Depth: 10,
		CellSize: 0.2, MinDeltaRMS: 0.01, MaxBlobs: 6, BlobThreshold: 0.1,
	})
	// Horizontal link at height 1 m.
	e.SetNodePosition("TX", 0, 1, 5)
	e.SetNodePosition("RX", 10, 1, 5)

	links := []LinkMotion{
		{NodeMAC: "TX", PeerMAC: "RX", DeltaRMS: 1.0, Motion: true},
	}
	r := e.Fuse(links)

	if len(r.Blobs) == 0 {
		t.Fatal("expected at least one blob from active link")
	}
	top := r.Blobs[0]
	if math.Abs(top.X-5.0) > 1.5 {
		t.Errorf("top blob X = %.2f, expected near 5.0", top.X)
	}
	if math.Abs(top.Z-5.0) > 1.5 {
		t.Errorf("top blob Z = %.2f, expected near 5.0", top.Z)
	}
}

// TestEngine_FourLinks_PositionAccuracy is the acceptance-criterion test:
// with 4 links whose intersection is the target, the top blob must be within
// ±1 m of the true target position.
func TestEngine_FourLinks_PositionAccuracy(t *testing.T) {
	const (
		targetX = 5.0
		targetZ = 5.0
		tol     = 1.0
	)

	e := NewEngine(&Config{
		Width: 10, Height: 3, Depth: 10,
		CellSize: 0.2, MinDeltaRMS: 0.01, MaxBlobs: 6, BlobThreshold: 0.2,
	})

	// Four nodes at corners, height 1 m.
	e.SetNodePosition("N1", 0, 1, 0)
	e.SetNodePosition("N2", 10, 1, 0)
	e.SetNodePosition("N3", 10, 1, 10)
	e.SetNodePosition("N4", 0, 1, 10)

	links := []LinkMotion{
		{NodeMAC: "N1", PeerMAC: "N3", DeltaRMS: 0.9, Motion: true}, // diagonal through centre
		{NodeMAC: "N2", PeerMAC: "N4", DeltaRMS: 0.9, Motion: true}, // diagonal through centre
		{NodeMAC: "N1", PeerMAC: "N2", DeltaRMS: 0.5, Motion: true}, // near edge
		{NodeMAC: "N3", PeerMAC: "N4", DeltaRMS: 0.5, Motion: true}, // near edge
	}

	r := e.Fuse(links)

	if r.ActiveLinks != 4 {
		t.Fatalf("expected 4 active links, got %d", r.ActiveLinks)
	}
	if len(r.Blobs) == 0 {
		t.Fatal("expected at least one blob from 4 crossing links")
	}

	top := r.Blobs[0]
	dx := top.X - targetX
	dz := top.Z - targetZ
	dist2D := math.Sqrt(dx*dx + dz*dz)

	if dist2D > tol {
		t.Errorf("top blob at (%.2f, %.2f), target (%.1f, %.1f): 2D dist=%.2f > %.1f m",
			top.X, top.Z, targetX, targetZ, dist2D, tol)
	}
}

// TestEngine_FourLinks_OffCentre verifies accuracy when the target is not at
// the geometric centre of the node layout.
func TestEngine_FourLinks_OffCentre(t *testing.T) {
	const (
		targetX = 3.0
		targetZ = 7.0
		tol     = 1.0
	)

	e := NewEngine(&Config{
		Width: 10, Height: 3, Depth: 10,
		CellSize: 0.2, MinDeltaRMS: 0.01, MaxBlobs: 6, BlobThreshold: 0.2,
	})

	nodes := []NodePosition{
		{"N1", 0, 1, 0}, {"N2", 10, 1, 0},
		{"N3", 10, 1, 10}, {"N4", 0, 1, 10},
		{"N5", 0, 1, 5}, {"N6", 5, 1, 10},
	}
	for _, n := range nodes {
		e.SetNodePosition(n.MAC, n.X, n.Y, n.Z)
	}

	links := buildSyntheticLinks(nodes, targetX, 1.0, targetZ)

	r := e.Fuse(links)

	if len(r.Blobs) == 0 {
		t.Fatal("expected at least one blob")
	}
	top := r.Blobs[0]
	dx := top.X - targetX
	dz := top.Z - targetZ
	dist2D := math.Sqrt(dx*dx + dz*dz)

	if dist2D > tol {
		t.Errorf("off-centre: blob at (%.2f, %.2f), target (%.1f, %.1f): dist=%.2f > %.1f m",
			top.X, top.Z, targetX, targetZ, dist2D, tol)
	}
}

// buildSyntheticLinks creates link activations for all node pairs, weighted by
// how close the target point is to each link's Fresnel zone.
func buildSyntheticLinks(nodes []NodePosition, tx, ty, tz float64) []LinkMotion {
	var links []LinkMotion
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			a, b := nodes[i], nodes[j]
			dAT := math.Sqrt((tx-a.X)*(tx-a.X) + (ty-a.Y)*(ty-a.Y) + (tz-a.Z)*(tz-a.Z))
			dTB := math.Sqrt((tx-b.X)*(tx-b.X) + (ty-b.Y)*(ty-b.Y) + (tz-b.Z)*(tz-b.Z))
			dAB := math.Sqrt((b.X-a.X)*(b.X-a.X) + (b.Y-a.Y)*(b.Y-a.Y) + (b.Z-a.Z)*(b.Z-a.Z))
			excess := dAT + dTB - dAB

			const lambda = 0.125
			r1 := math.Sqrt(lambda * dAB / 4.0)
			if r1 < 0.1 {
				r1 = 0.1
			}
			ne := excess / r1
			activation := 1.0 / (1.0 + ne)
			if activation < 0.05 {
				continue
			}
			links = append(links, LinkMotion{
				NodeMAC:  a.MAC,
				PeerMAC:  b.MAC,
				DeltaRMS: activation,
				Motion:   true,
			})
		}
	}
	return links
}

// TestEngine_LastResult checks that LastResult is updated after each Fuse call.
func TestEngine_LastResult(t *testing.T) {
	e := makeEngine(10, 3, 10)
	if e.LastResult() != nil {
		t.Fatal("expected nil before first Fuse")
	}
	e.SetNodePosition("A", 0, 1, 0)
	e.SetNodePosition("B", 10, 1, 10)
	r := e.Fuse([]LinkMotion{{NodeMAC: "A", PeerMAC: "B", DeltaRMS: 0.5, Motion: true}})
	if e.LastResult() != r {
		t.Fatal("LastResult should return the most recent result")
	}
}

// TestEngine_RemoveNode ensures removed nodes are excluded from fusion.
func TestEngine_RemoveNode(t *testing.T) {
	e := makeEngine(10, 3, 10)
	e.SetNodePosition("A", 0, 1, 0)
	e.SetNodePosition("B", 10, 1, 10)
	e.RemoveNode("B")
	r := e.Fuse([]LinkMotion{{NodeMAC: "A", PeerMAC: "B", DeltaRMS: 0.5, Motion: true}})
	if r.ActiveLinks != 0 {
		t.Fatalf("expected 0 active links after RemoveNode, got %d", r.ActiveLinks)
	}
}

// seedPosKey is the distinctness key for a 3D position (see TestEngine_SeedNodePositions).
type seedPosKey struct{ x, y, z float64 }

// TestEngine_SeedNodePositions locks in the bf-6s3d startup-seeding invariant.
// At startup main.go iterates fleetReg.GetAllNodes() and calls
// SetNodePosition(node.MAC, node.PosX, node.PosY, node.PosZ) per node, reading the
// DB pos_x/pos_y/pos_z columns (the columns the nodes schema defaults to 0/0/1 —
// internal/db/migrations.go and fleet/registry.go). After that seeding the engine
// must hold: NodeCount() == number of fleet nodes, and NodePositions() holding a
// distinct, non-(0,0,1) position for each node (nodes must NOT collapse to the
// co-located DB default). This test replays that seeding pattern against the engine
// so a future refactor cannot silently regress the acceptance criteria.
func TestEngine_SeedNodePositions(t *testing.T) {
	e := makeEngine(6, 2.5, 5)

	// Distinct, realistic fleet placements (meters) — mirrors what
	// fleetReg.GetAllNodes() returns for a positioned fleet; none sit at the
	// (0,0,1) column default.
	seeded := []NodePosition{
		{MAC: "AA:00:00:00:00:01", X: 0.2, Y: 2.1, Z: 0.2},
		{MAC: "AA:00:00:00:00:02", X: 5.8, Y: 2.0, Z: 0.2},
		{MAC: "AA:00:00:00:00:03", X: 0.2, Y: 0.3, Z: 4.8},
		{MAC: "AA:00:00:00:00:04", X: 5.8, Y: 0.3, Z: 4.8},
	}
	for _, n := range seeded {
		e.SetNodePosition(n.MAC, n.X, n.Y, n.Z) // mirrors main.go seeding loop
	}

	// Acceptance criterion: NodeCount() equals the number of fleet nodes.
	if got := e.NodeCount(); got != len(seeded) {
		t.Fatalf("NodeCount() = %d, want %d (must equal fleet node count from GetAllNodes)", got, len(seeded))
	}

	// Acceptance criterion: a distinct, non-(0,0,1) position for each registered node.
	pos := e.NodePositions()
	if len(pos) != len(seeded) {
		t.Fatalf("NodePositions() returned %d entries, want %d", len(pos), len(seeded))
	}
	byMAC := make(map[string]NodePosition, len(seeded))
	for _, n := range seeded {
		byMAC[n.MAC] = n
	}
	seen := make(map[seedPosKey]bool, len(pos))
	for _, p := range pos {
		want, ok := byMAC[p.MAC]
		if !ok {
			t.Errorf("NodePositions() returned unknown MAC %q", p.MAC)
			continue
		}
		// Coordinates must round-trip exactly from what the seeding loop set.
		if p.X != want.X || p.Y != want.Y || p.Z != want.Z {
			t.Errorf("node %q position = (%.2f,%.2f,%.2f), want (%.2f,%.2f,%.2f)",
				p.MAC, p.X, p.Y, p.Z, want.X, want.Y, want.Z)
		}
		// Must NOT be the co-located DB default.
		if p.X == 0 && p.Y == 0 && p.Z == 1 {
			t.Errorf("node %q seeded at DB default (0,0,1) — positions must come from DB columns", p.MAC)
		}
		// Must be distinct from every other node.
		k := seedPosKey{p.X, p.Y, p.Z}
		if seen[k] {
			t.Errorf("node %q duplicates another node's position (%.2f,%.2f,%.2f) — positions must be distinct", p.MAC, p.X, p.Y, p.Z)
		}
		seen[k] = true
	}
}

// TestEngine_DefaultPlacementProducesPeaks is the bf-18yn acceptance test and
// closes the bf-4q5w symptom (the 3D fusion engine emitting no / degenerate
// peaks). It seeds the engine using ONLY the default node placement —
// simulator.DefaultNodePositions, the spread geometry a freshly-onboarded
// virtual/sim fleet receives with no manual positioning (bf-3fr6, bf-xrej) —
// then drives a synthetic walker through the room centre and asserts the
// accumulation grid produces non-zero fusion peaks (len(blobs) > 0).
//
// This is the fleet->engine counterpart to TestEngine_SeedNodePositions
// (bf-6s3d): that test locks in the seeding invariant (distinct, non-(0,0,1)
// positions); this one locks in the downstream consequence the seeding exists
// to deliver — that spread nodes actually let Fuse form blobs.
func TestEngine_DefaultPlacementProducesPeaks(t *testing.T) {
	space := simulator.DefaultSpace()
	minX, minY, minZ, maxX, maxY, maxZ := space.Bounds()

	// 2+ nodes are required to form any link; count == 1 has no links and
	// cannot produce peaks (that is a valid empty state, not a regression).
	for _, count := range []int{2, 3, 4, 6} {
		t.Run(fmt.Sprintf("nodes=%d", count), func(t *testing.T) {
			// DEFAULT placement — no manual positioning. Distinct, room-spanning
			// points distributed across the space's bounding box.
			pts := simulator.DefaultNodePositions(space, count)

			// Regression guard (bf-4q5w): if default placement ever collapses
			// back to the co-located DB origin, every link goes degenerate and
			// Fuse can only emit zero peaks. Fail loudly with a clear reason
			// rather than letting the regression pass silently.
			assertPlacementNotCollapsed(t, pts)

			// Seed the engine exactly as main.go does at startup: one
			// SetNodePosition per node, reading the (default) registry
			// positions. The grid is sized to the same bounding box so every
			// default-placed node lands in-bounds.
			e := NewEngine(&Config{
				Width:   maxX - minX,
				Height:  maxY - minY,
				Depth:   maxZ - minZ,
				OriginX: minX,
				OriginY: minY,
				OriginZ: minZ,
				CellSize:      0.2,
				MinDeltaRMS:   0.01,
				MaxBlobs:      6,
				BlobThreshold: 0.1,
			})
			nodes := make([]NodePosition, len(pts))
			for i, p := range pts {
				mac := fmt.Sprintf("SN:%02d", i+1)
				nodes[i] = NodePosition{MAC: mac, X: p.X, Y: p.Y, Z: p.Z}
				e.SetNodePosition(mac, p.X, p.Y, p.Z) // mirrors main.go seeding loop
			}

			// A walker at the room centre perturbs every link whose first
			// Fresnel zone crosses it — the same synthetic-motion model the
			// position-accuracy tests use (buildSyntheticLinks).
			links := buildSyntheticLinks(nodes,
				(minX+maxX)/2, (minY+maxY)/2, (minZ+maxZ)/2)
			if len(links) == 0 {
				t.Fatalf("expected at least one motion link crossing the room centre, got 0")
			}

			r := e.Fuse(links)

			// Acceptance criterion (bf-18yn / bf-4q5w): the default placement
			// must yield non-zero fusion peaks — either extracted blobs
			// (len > 0) OR an accumulation grid whose max rises above the peak
			// threshold. The OR covers small fleets whose links paint a flat
			// ridge that the strict local-maximum extractor may not promote to
			// a blob, even though the grid is plainly non-zero. Co-located
			// (0,0,1) nodes paint nothing, so their grid max stays at 0 — the
			// condition is non-trivial (see TestEngine_CoLocatedOriginYieldsNoPeaks).
			gridMax := gridMaxValue(e.GetGridSnapshot().Data)
			if len(r.Blobs) == 0 && gridMax <= e.blobThresh {
				t.Fatalf("default placement of %d nodes produced no peaks: "+
					"0 blobs and gridMax=%.4f <= threshold %.4f (activeLinks=%d) — "+
					"bf-4q5w regression: spread nodes must let the grid accumulate",
					count, gridMax, e.blobThresh, r.ActiveLinks)
			}
		})
	}
}

// TestEngine_CoLocatedOriginYieldsNoPeaks is the counter-example that pins the
// bf-4q5w symptom: nodes left collapsed at the (0,0,1) DB schema default are
// co-located, so every link is degenerate (length < 0.1 m), the accumulation
// grid stays at zero, and Fuse emits no blobs. This is exactly the failure the
// default placement (tested above) exists to prevent — a fleet seeded this way
// could never localize. It documents why the non-zero-peak assertion is
// meaningful rather than trivially satisfiable.
func TestEngine_CoLocatedOriginYieldsNoPeaks(t *testing.T) {
	e := NewEngine(&Config{
		Width: 6, Height: 5, Depth: 2.5,
		CellSize: 0.2, MinDeltaRMS: 0.01, MaxBlobs: 6, BlobThreshold: 0.1,
	})
	// Four nodes all at the DB default — the pre-spread state.
	for i := 0; i < 4; i++ {
		e.SetNodePosition(fmt.Sprintf("CN:%02d", i+1), 0, 0, 1)
	}
	links := []LinkMotion{
		{NodeMAC: "CN:01", PeerMAC: "CN:02", DeltaRMS: 1.0, Motion: true},
		{NodeMAC: "CN:03", PeerMAC: "CN:04", DeltaRMS: 1.0, Motion: true},
	}
	r := e.Fuse(links)
	if len(r.Blobs) != 0 {
		t.Fatalf("co-located (0,0,1) nodes must produce 0 blobs, got %d — "+
			"degenerate links should leave the grid at zero", len(r.Blobs))
	}
}

// assertPlacementNotCollapsed fails the test if any node sits at the co-located
// (0,0,1) DB default or if any two nodes share a position — i.e. the placement
// has collapsed instead of spreading across the room (bf-4q5w root cause).
func assertPlacementNotCollapsed(t *testing.T, pts []simulator.Point) {
	t.Helper()
	seen := make(map[simulator.Point]bool, len(pts))
	for i, p := range pts {
		if p.X == 0 && p.Y == 0 && p.Z == 1 {
			t.Fatalf("default-placed node %d collapsed to DB origin (0,0,1) — "+
				"bf-4q5w regression: positions must be spread, not co-located", i)
		}
		if seen[p] {
			t.Fatalf("default-placed nodes co-located at %v — bf-4q5w regression: "+
				"positions must be distinct", p)
		}
		seen[p] = true
	}
}

// gridMaxValue returns the maximum voxel value in a flat grid snapshot.
func gridMaxValue(data []float64) float64 {
	max := 0.0
	for _, v := range data {
		if v > max {
			max = v
		}
	}
	return max
}

// TestEngine_GeometryPlacementDrivesFusionPeaks (bf-4b1c) is the differential
// lock-in for "geometry placement and fusion peaks." It runs the SAME
// blob-producing harness under two placements and asserts the non-zero-peak
// condition flips with geometry — passing with the default (spread) placement
// a freshly-onboarded sim fleet receives, and failing with the old co-located
// (0,0,1) DB-default collapse.
//
// Placement is held as the SOLE variable: the engine grid, the walker-free
// all-pairs motion link set (every node pair, DeltaRMS=1.0, Motion=true), and
// the room are identical across both legs. Only where the nodes sit differs.
//
//  - Spread placement: links are non-degenerate, AddLinkInfluence paints the
//    accumulation grid, it normalizes to a non-zero max, and Fuse emits peaks.
//  - Co-located placement: every link has length < 0.1 m, AddLinkInfluence
//    early-returns, the grid stays at zero, and Fuse emits no peaks.
//
// That the identical non-zero-peak assertion holds for one and fails for the
// other is the demonstrable proof the bead's scope asks for: "test would fail
// with old co-located placement but passes with new geometry" — i.e. the test
// is genuinely sensitive to the bf-4q5w regression, not trivially satisfiable.
//
// This complements TestEngine_DefaultPlacementProducesPeaks (bf-18yn: spread
// alone, geometry-dependent synthetic links) and TestEngine_CoLocatedOrigin
// YieldsNoPeaks (co-located alone): both halves in one run, geometry isolated.
func TestEngine_GeometryPlacementDrivesFusionPeaks(t *testing.T) {
	space := simulator.DefaultSpace()
	minX, minY, minZ, maxX, maxY, maxZ := space.Bounds()

	// spreadPlace is the default onboarding placement (distinct, room-spanning).
	// colocatedPlace is the pre-spread DB-default collapse (all at (0,0,1)).
	spreadPlace := func(count int) []simulator.Point {
		return simulator.DefaultNodePositions(space, count)
	}
	colocatedPlace := func(count int) []simulator.Point {
		pts := make([]simulator.Point, count)
		for i := range pts {
			pts[i] = simulator.Point{X: 0, Y: 0, Z: 1}
		}
		return pts
	}

	for _, count := range []int{2, 4} {
		t.Run(fmt.Sprintf("nodes=%d", count), func(t *testing.T) {
			// ---- spread (new geometry): must produce non-zero peaks ----
			spreadResult, spreadGridMax := fuseWithPlacement(minX, minY, minZ,
				maxX, maxY, maxZ, count, spreadPlace)

			// Guard the placement itself did not regress to the collapse.
			assertPlacementNotCollapsed(t, spreadPlace(count))

			// Bead criterion: the accumulation grid is not all zeros. This is
			// the geometry-pure signal and the one that holds for every fleet
			// size (a single link still paints a non-zero ridge).
			if spreadGridMax <= 0 {
				t.Fatalf("spread placement produced an all-zero accumulation grid "+
					"(gridMax=%.4f, activeLinks=%d) — default placement must let the "+
					"grid accumulate non-zero peaks", spreadGridMax, spreadResult.ActiveLinks)
			}
			// A single link (count=2) paints a symmetric ridge with no strict
			// local maximum, so the peak extractor legitimately yields no blob
			// even though the grid is non-zero (documented in bf-18yn). With
			// count>=4 the crossing links form a true maximum and must extract
			// at least one blob — proving the non-zero grid localizes.
			if count >= 4 && len(spreadResult.Blobs) == 0 {
				t.Fatalf("spread placement of %d nodes produced 0 blobs despite "+
					"gridMax=%.4f > 0 (activeLinks=%d) — crossing links must yield a peak",
					count, spreadGridMax, spreadResult.ActiveLinks)
			}
			if len(spreadResult.Blobs) > 0 {
				top := spreadResult.Blobs[0]
				t.Logf("[spread] nodes=%d gridMax=%.4f blobs=%d top=(%.2f,%.2f,%.2f) conf=%.3f activeLinks=%d",
					count, spreadGridMax, len(spreadResult.Blobs), top.X, top.Y, top.Z,
					top.Confidence, spreadResult.ActiveLinks)
			} else {
				t.Logf("[spread] nodes=%d gridMax=%.4f blobs=0 (single-link ridge, no strict max) activeLinks=%d",
					count, spreadGridMax, spreadResult.ActiveLinks)
			}

			// ---- co-located (old geometry): must produce ZERO peaks ----
			coloResult, coloGridMax := fuseWithPlacement(minX, minY, minZ,
				maxX, maxY, maxZ, count, colocatedPlace)

			// The SAME non-zero-peak condition asserted above must FAIL here:
			// an all-zero grid (no accumulation) and zero extracted blobs. If
			// this leg ever starts producing peaks, the differential no longer
			// isolates geometry and the regression-sensitivity claim is void.
			if coloGridMax != 0 || len(coloResult.Blobs) != 0 {
				t.Fatalf("co-located (0,0,1) placement must yield an all-zero grid and "+
					"no peaks (got gridMax=%.4f, blobs=%d) — the differential only holds "+
					"if co-located nodes cannot accumulate", coloGridMax, len(coloResult.Blobs))
			}
			t.Logf("[colocated] nodes=%d gridMax=%.4f blobs=%d activeLinks=%d — degenerate links, no accumulation",
				count, coloGridMax, len(coloResult.Blobs), coloResult.ActiveLinks)

			// Explicit differential: the geometry change flips gridMax from
			// zero (old) to non-zero (new). This is the bead's central claim.
			if !(spreadGridMax > 0 && coloGridMax == 0) {
				t.Fatalf("differential broken: spread gridMax=%.4f colocated gridMax=%.4f — "+
					"expected spread>0 and colocated==0", spreadGridMax, coloGridMax)
			}
		})
	}
}

// fuseWithPlacement builds an engine sized to the given room bounds, seeds
// `count` nodes at the placement the `place` callback returns, fires an
// explicit geometry-independent motion link for every node pair (DeltaRMS=1.0),
// runs one Fuse step, and returns the result plus the accumulation grid's max
// voxel value. Placement is the ONLY thing that varies between callers — the
// link set, grid, and room are identical — so the returned gridMax is a pure
// function of geometry.
func fuseWithPlacement(minX, minY, minZ, maxX, maxY, maxZ float64,
	count int, place func(int) []simulator.Point) (*Result, float64) {

	e := NewEngine(&Config{
		Width:   maxX - minX,
		Height:  maxY - minY,
		Depth:   maxZ - minZ,
		OriginX: minX,
		OriginY: minY,
		OriginZ: minZ,
		CellSize:      0.2,
		MinDeltaRMS:   0.01,
		MaxBlobs:      6,
		BlobThreshold: 0.1,
	})

	pts := place(count)
	nodes := make([]NodePosition, len(pts))
	for i, p := range pts {
		mac := fmt.Sprintf("GN:%02d", i+1)
		nodes[i] = NodePosition{MAC: mac, X: p.X, Y: p.Y, Z: p.Z}
		e.SetNodePosition(mac, p.X, p.Y, p.Z)
	}

	// Explicit all-pairs motion links — geometry-independent. Same set for
	// every placement, so only the node coordinates differ between legs.
	links := make([]LinkMotion, 0, count*(count-1)/2)
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			links = append(links, LinkMotion{
				NodeMAC:  nodes[i].MAC,
				PeerMAC:  nodes[j].MAC,
				DeltaRMS: 1.0,
				Motion:   true,
			})
		}
	}

	r := e.Fuse(links)

	var gridMax float64
	if snap := e.GetGridSnapshot(); snap != nil {
		gridMax = gridMaxValue(snap.Data)
	}
	return r, gridMax
}

// TestEngine_HealthWeight verifies that links with lower health scores contribute less to fusion.
// Per spec: "each link's contribution to the 3D occupancy grid is multiplied by its health_score"
func TestEngine_HealthWeight(t *testing.T) {
	e := NewEngine(&Config{
		Width: 10, Height: 3, Depth: 10,
		CellSize: 0.2, MinDeltaRMS: 0.01, MaxBlobs: 6, BlobThreshold: 0.1,
	})

	e.SetNodePosition("A", 0, 1, 5)
	e.SetNodePosition("B", 10, 1, 5)

	// First, fuse with full health link
	linksFull := []LinkMotion{
		{NodeMAC: "A", PeerMAC: "B", DeltaRMS: 1.0, Motion: true, HealthScore: 1.0},
	}
	r1 := e.Fuse(linksFull)

	// Then fuse with 30% health link
	linksLow := []LinkMotion{
		{NodeMAC: "A", PeerMAC: "B", DeltaRMS: 1.0, Motion: true, HealthScore: 0.3},
	}
	r2 := e.Fuse(linksLow)

	if len(r1.Blobs) == 0 || len(r2.Blobs) == 0 {
		t.Fatal("expected blobs from both fusions")
	}

	// The peak with 30% health should have ~30% the confidence of full health
	// (approximately, since normalization affects final values)
	// At minimum, verify that low health produces lower-weighted blobs
	// The exact ratio depends on normalization, We check that r2's top blob
	// has lower confidence than r1's.
	if r2.Blobs[0].Confidence > r1.Blobs[0].Confidence {
		t.Errorf("low health link (%.2f) should produce lower confidence blob than full health", r2.Blobs[0].Confidence)
	}

	// Also test that default HealthScore (0) is treated as 1.0
	linksDefault := []LinkMotion{
		{NodeMAC: "A", PeerMAC: "B", DeltaRMS: 1.0, Motion: true, HealthScore: 0}, // 0 means default to 1.0
	}
	r3 := e.Fuse(linksDefault)
	if len(r3.Blobs) == 0 {
		t.Fatal("expected blob from link with default health")
	}
	// r3 should have similar confidence to r1 (both have effective health of 1.0)
	if math.Abs(r3.Blobs[0].Confidence-r1.Blobs[0].Confidence) > 0.05 {
		t.Errorf("default health (0) should be treated as 1.0: r1=%.3f, r3=%.3f", r1.Blobs[0].Confidence, r3.Blobs[0].Confidence)
	}
}

// TestEngine_PerformanceTwentyLinks checks that fusion over 20 links completes
// within the 50 ms acceptance criterion.
func TestEngine_PerformanceTwentyLinks(t *testing.T) {
	e := NewEngine(&Config{
		Width: 10, Height: 3, Depth: 10,
		CellSize: 0.2, MinDeltaRMS: 0.01, MaxBlobs: 6, BlobThreshold: 0.3,
	})

	macs := []string{"N0", "N1", "N2", "N3", "N4", "N5", "N6", "N7", "N8", "N9"}
	xs := []float64{0, 5, 10, 0, 5, 10, 0, 5, 10, 5}
	zs := []float64{0, 0, 0, 5, 5, 5, 10, 10, 10, 5}
	for i, m := range macs {
		e.SetNodePosition(m, xs[i], 1.0, zs[i])
	}

	var links []LinkMotion
	for i := 0; i < len(macs) && len(links) < 20; i++ {
		for j := i + 1; j < len(macs) && len(links) < 20; j++ {
			links = append(links, LinkMotion{
				NodeMAC: macs[i], PeerMAC: macs[j],
				DeltaRMS: 0.5, Motion: true,
			})
		}
	}

	const iterations = 10
	start := time.Now()
	for k := 0; k < iterations; k++ {
		e.Fuse(links)
	}
	perFuse := time.Since(start) / iterations
	const limit = 50 * time.Millisecond
	if perFuse > limit {
		t.Errorf("fusion took %v per call (limit %v)", perFuse, limit)
	}
}

// ---- ExplainabilitySnapshot tests ----

// TestExplainabilitySnapshot_ThreeLinks verifies that GenerateExplainabilitySnapshot
// correctly computes per-link contributions for 3 known links with a blob at a
// known position.
func TestExplainabilitySnapshot_ThreeLinks(t *testing.T) {
	nodePos := map[string]NodePosition{
		"AA:BB:CC:DD:EE:01": {MAC: "AA:BB:CC:DD:EE:01", X: 0, Y: 1, Z: 0},
		"AA:BB:CC:DD:EE:02": {MAC: "AA:BB:CC:DD:EE:02", X: 4, Y: 1, Z: 0},
		"AA:BB:CC:DD:EE:03": {MAC: "AA:BB:CC:DD:EE:03", X: 2, Y: 1, Z: 4},
	}
	links := []LinkMotion{
		{NodeMAC: "AA:BB:CC:DD:EE:01", PeerMAC: "AA:BB:CC:DD:EE:02", DeltaRMS: 0.10, Motion: true, HealthScore: 1.0},
		{NodeMAC: "AA:BB:CC:DD:EE:02", PeerMAC: "AA:BB:CC:DD:EE:03", DeltaRMS: 0.05, Motion: true, HealthScore: 1.0},
		{NodeMAC: "AA:BB:CC:DD:EE:01", PeerMAC: "AA:BB:CC:DD:EE:03", DeltaRMS: 0.08, Motion: true, HealthScore: 1.0},
	}
	result := &Result{
		Blobs:     []Blob{{X: 2, Y: 1, Z: 2, Confidence: 0.85}},
		Timestamp: time.Now(),
	}

	snap := GenerateExplainabilitySnapshot(result, 0, 1, links, nodePos, nil, 0.125, 0.2)
	if snap == nil {
		t.Fatal("expected non-nil snapshot")
	}
	if snap.BlobID != 1 {
		t.Errorf("blob_id: got %d, want 1", snap.BlobID)
	}
	if got := [3]float64{snap.BlobPosition[0], snap.BlobPosition[1], snap.BlobPosition[2]}; got != [3]float64{2, 1, 2} {
		t.Errorf("blob_position: got %v, want [2 1 2]", got)
	}
	if len(snap.PerLinkContributions) != 3 {
		t.Fatalf("expected 3 per-link contributions, got %d", len(snap.PerLinkContributions))
	}
	// Verify each contribution has a positive deltaRMS and correct link IDs.
	for _, c := range snap.PerLinkContributions {
		if c.DeltaRMS <= 0 {
			t.Errorf("link %s: DeltaRMS should be > 0, got %f", c.LinkID, c.DeltaRMS)
		}
		if c.ZoneNumber < 1 {
			t.Errorf("link %s: ZoneNumber should be >= 1, got %d", c.LinkID, c.ZoneNumber)
		}
		if c.CombinedWeight <= 0 {
			t.Errorf("link %s: CombinedWeight should be > 0, got %f", c.LinkID, c.CombinedWeight)
		}
		// Contributing flag: links with Motion=true and DeltaRMS > 0.02
		if !c.Contributing {
			t.Errorf("link %s: Contributing should be true (DeltaRMS=%f, Motion=true)", c.LinkID, c.DeltaRMS)
		}
	}
}

// TestExplainabilitySnapshot_ContributionPctSums verifies that the sum of
// ContributionPct across all links equals approximately 100%.
func TestExplainabilitySnapshot_ContributionPctSums(t *testing.T) {
	nodePos := map[string]NodePosition{
		"AA:BB:CC:DD:EE:01": {MAC: "AA:BB:CC:DD:EE:01", X: 0, Y: 1, Z: 0},
		"AA:BB:CC:DD:EE:02": {MAC: "AA:BB:CC:DD:EE:02", X: 4, Y: 1, Z: 0},
		"AA:BB:CC:DD:EE:03": {MAC: "AA:BB:CC:DD:EE:03", X: 2, Y: 1, Z: 4},
	}
	links := []LinkMotion{
		{NodeMAC: "AA:BB:CC:DD:EE:01", PeerMAC: "AA:BB:CC:DD:EE:02", DeltaRMS: 0.15, Motion: true, HealthScore: 1.0},
		{NodeMAC: "AA:BB:CC:DD:EE:02", PeerMAC: "AA:BB:CC:DD:EE:03", DeltaRMS: 0.08, Motion: true, HealthScore: 1.0},
		{NodeMAC: "AA:BB:CC:DD:EE:01", PeerMAC: "AA:BB:CC:DD:EE:03", DeltaRMS: 0.12, Motion: true, HealthScore: 1.0},
	}
	result := &Result{
		Blobs:     []Blob{{X: 2, Y: 1, Z: 2, Confidence: 0.80}},
		Timestamp: time.Now(),
	}

	snap := GenerateExplainabilitySnapshot(result, 0, 2, links, nodePos, nil, 0.125, 0.2)
	if snap == nil {
		t.Fatal("expected non-nil snapshot")
	}
	total := 0.0
	for _, c := range snap.PerLinkContributions {
		total += c.ContributionPct
	}
	if math.Abs(total-100.0) > 0.01 {
		t.Errorf("contribution_pct sum = %.4f, want ~100.0", total)
	}
}

// TestExplainabilitySnapshot_NilOnInvalidBlob verifies that nil is returned when
// the blob index is out of bounds.
func TestExplainabilitySnapshot_NilOnInvalidBlob(t *testing.T) {
	result := &Result{Blobs: []Blob{{X: 1, Y: 1, Z: 1, Confidence: 0.5}}}
	if snap := GenerateExplainabilitySnapshot(result, 5, 1, nil, nil, nil, 0.125, 0.2); snap != nil {
		t.Error("expected nil for out-of-range blob index")
	}
	if snap := GenerateExplainabilitySnapshot(nil, 0, 1, nil, nil, nil, 0.125, 0.2); snap != nil {
		t.Error("expected nil for nil result")
	}
}

// TestComputeFresnelEllipsoidAxes verifies the Fresnel ellipsoid geometry for a
// 4-metre link with 5 GHz WiFi (lambda = 0.06 m).
//
// Expected values:
//
//	d = 4.0 m
//	a = (d + lambda/2) / 2 = (4 + 0.03) / 2 = 2.015 m
//	b = sqrt(a² − (d/2)²) = sqrt(2.015² − 4) = sqrt(0.060225) ≈ 0.245 m
func TestComputeFresnelEllipsoidAxes(t *testing.T) {
	tx := NodePosition{X: 0, Y: 0, Z: 0}
	rx := NodePosition{X: 4, Y: 0, Z: 0}
	lambda := 0.06 // 5 GHz

	a, b, d := ComputeFresnelEllipsoidAxes(tx, rx, lambda)

	const tol = 0.001
	if math.Abs(d-4.0) > tol {
		t.Errorf("d = %f, want 4.000 (±%f)", d, tol)
	}
	if math.Abs(a-2.015) > tol {
		t.Errorf("a = %f, want 2.015 (±%f)", a, tol)
	}
	// b = sqrt(2.015^2 - 2^2) = sqrt(0.060225) ≈ 0.2454
	wantB := math.Sqrt(2.015*2.015 - 2.0*2.0)
	if math.Abs(b-wantB) > tol {
		t.Errorf("b = %f, want %f (±%f)", b, wantB, tol)
	}
}

// TestComputeFresnelEllipsoidAxes_2_4GHz verifies the geometry for 2.4 GHz WiFi
// (lambda = 0.125 m) with the same 4-metre link.
func TestComputeFresnelEllipsoidAxes_2_4GHz(t *testing.T) {
	tx := NodePosition{X: 0, Y: 0, Z: 0}
	rx := NodePosition{X: 4, Y: 0, Z: 0}
	lambda := 0.125

	a, b, d := ComputeFresnelEllipsoidAxes(tx, rx, lambda)

	const tol = 0.001
	if math.Abs(d-4.0) > tol {
		t.Errorf("d = %f, want 4.000", d)
	}
	wantA := (4.0 + 0.125/2) / 2
	if math.Abs(a-wantA) > tol {
		t.Errorf("a = %f, want %f", a, wantA)
	}
	wantB := math.Sqrt(wantA*wantA - 2.0*2.0)
	if math.Abs(b-wantB) > tol {
		t.Errorf("b = %f, want %f", b, wantB)
	}
}
