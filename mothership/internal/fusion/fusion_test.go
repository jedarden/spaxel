package fusion

import (
	"math"
	"testing"
	"time"
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
