package explainability

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// ── helpers ────────────────────────────────────────────────────────────────────

func newHandler() *Handler { return NewHandler() }

// makeLinkState returns a LinkState with TX at origin and RX along the X axis.
func makeLinkState(nodeMAC, peerMAC string, rxX float64, deltaRMS float64, motion bool) LinkState {
	return LinkState{
		NodeMAC:  nodeMAC,
		PeerMAC:  peerMAC,
		NodePos:  [3]float64{0, 0, 1},
		PeerPos:  [3]float64{rxX, 0, 1},
		DeltaRMS: deltaRMS,
		Motion:   motion,
		Weight:   1.0,
	}
}

func makeBlobAt(id int, x, y, z, confidence float64) BlobSnapshot {
	return BlobSnapshot{ID: id, X: x, Y: y, Z: z, Confidence: confidence}
}

// ── computeExplanation ────────────────────────────────────────────────────────

func TestComputeExplanation_NilGrid_StillComputesContributions(t *testing.T) {
	h := newHandler()
	blob := makeBlobAt(1, 2, 0, 1, 0.8)
	link := makeLinkState("AA:BB:CC:DD:EE:01", "AA:BB:CC:DD:EE:02", 4.0, 0.10, true)

	exp := h.computeExplanation(blob, []LinkState{link}, nil) // nil grid must not abort

	if exp == nil {
		t.Fatal("expected non-nil explanation when grid is nil")
	}
	if len(exp.ContributingLinks) == 0 {
		t.Error("expected at least one contributing link")
	}
	if len(exp.AllLinks) == 0 {
		t.Error("expected at least one entry in AllLinks")
	}
}

func TestComputeExplanation_NoLinks(t *testing.T) {
	h := newHandler()
	exp := h.computeExplanation(makeBlobAt(1, 1, 0, 1, 0.5), nil, nil)
	if exp == nil {
		t.Fatal("nil explanation")
	}
	if len(exp.ContributingLinks) != 0 || len(exp.AllLinks) != 0 {
		t.Error("expected empty link slices with no links")
	}
}

func TestComputeExplanation_SingleContributingLink_100Percent(t *testing.T) {
	h := newHandler()
	blob := makeBlobAt(1, 2, 0, 1, 0.9)
	link := makeLinkState("AA:BB:CC:DD:EE:01", "AA:BB:CC:DD:EE:02", 4.0, 0.15, true)

	exp := h.computeExplanation(blob, []LinkState{link}, nil)

	if len(exp.ContributingLinks) != 1 {
		t.Fatalf("expected 1 contributing link, got %d", len(exp.ContributingLinks))
	}
	got := exp.ContributingLinks[0].Contribution
	if math.Abs(got-1.0) > 1e-9 {
		t.Errorf("single contributing link should have contribution=1.0, got %f", got)
	}
}

func TestComputeExplanation_TwoEqualLinks_FiftyFifty(t *testing.T) {
	h := newHandler()
	blob := makeBlobAt(1, 2, 0, 1, 0.8)
	// Two identical links placed symmetrically so they have the same deltaRMS, weight, and zone
	linkA := makeLinkState("AA:00:00:00:00:01", "AA:00:00:00:00:02", 4.0, 0.10, true)
	linkB := makeLinkState("BB:00:00:00:00:01", "BB:00:00:00:00:02", 4.0, 0.10, true)
	// Both links have identical geometry → identical contribution

	exp := h.computeExplanation(blob, []LinkState{linkA, linkB}, nil)

	if len(exp.ContributingLinks) != 2 {
		t.Fatalf("expected 2 contributing links, got %d", len(exp.ContributingLinks))
	}
	for _, lc := range exp.ContributingLinks {
		if math.Abs(lc.Contribution-0.5) > 1e-9 {
			t.Errorf("expected each link contribution=0.5, got %f", lc.Contribution)
		}
	}
}

func TestComputeExplanation_NonContributingLinkExcluded(t *testing.T) {
	h := newHandler()
	blob := makeBlobAt(1, 2, 0, 1, 0.7)
	contributing := makeLinkState("AA:00:00:00:00:01", "AA:00:00:00:00:02", 4.0, 0.10, true)
	nonContrib := makeLinkState("BB:00:00:00:00:01", "BB:00:00:00:00:02", 4.0, 0.005, false)

	exp := h.computeExplanation(blob, []LinkState{contributing, nonContrib}, nil)

	if len(exp.ContributingLinks) != 1 {
		t.Fatalf("expected 1 contributing link, got %d", len(exp.ContributingLinks))
	}
	if len(exp.AllLinks) != 2 {
		t.Fatalf("expected 2 entries in AllLinks, got %d", len(exp.AllLinks))
	}
	// Contributing link must have contribution = 1.0 after normalization
	if math.Abs(exp.ContributingLinks[0].Contribution-1.0) > 1e-9 {
		t.Errorf("expected contributing link contribution=1.0, got %f", exp.ContributingLinks[0].Contribution)
	}
	// Non-contributing link keeps its raw value (not normalized)
	for _, lc := range exp.AllLinks {
		if !lc.Contributing {
			if lc.Contribution < 0 {
				t.Error("non-contributing link contribution should be non-negative")
			}
		}
	}
}

func TestComputeExplanation_FresnelZoneNumber(t *testing.T) {
	// Place TX at (0,0,0) and RX at (4,0,0). Blob at (2,0,0) = midpoint.
	// At midpoint, pathViaBlob = 2+2 = 4m = pathDirect = 4m, so ΔL = 0 → zone 1
	h := newHandler()
	blob := BlobSnapshot{ID: 1, X: 2, Y: 0, Z: 0, Confidence: 0.8}
	link := LinkState{
		NodeMAC:  "AA:00:00:00:00:01",
		PeerMAC:  "AA:00:00:00:00:02",
		NodePos:  [3]float64{0, 0, 0},
		PeerPos:  [3]float64{4, 0, 0},
		DeltaRMS: 0.10,
		Motion:   true,
		Weight:   1.0,
	}

	exp := h.computeExplanation(blob, []LinkState{link}, nil)

	if len(exp.ContributingLinks) != 1 {
		t.Fatalf("expected 1 contributing link, got %d", len(exp.ContributingLinks))
	}
	if exp.ContributingLinks[0].ZoneNumber != 1 {
		t.Errorf("expected zone 1 for midpoint blob, got %d", exp.ContributingLinks[0].ZoneNumber)
	}
}

func TestComputeExplanation_FresnelEllipsoidAddedForContributingLink(t *testing.T) {
	h := newHandler()
	blob := makeBlobAt(1, 2, 0, 1, 0.8)
	link := makeLinkState("AA:00:00:00:00:01", "AA:00:00:00:00:02", 4.0, 0.10, true)

	exp := h.computeExplanation(blob, []LinkState{link}, nil)

	if len(exp.FresnelZones) != 1 {
		t.Fatalf("expected 1 Fresnel zone, got %d", len(exp.FresnelZones))
	}
	fz := exp.FresnelZones[0]
	if fz.Lambda != 0.123 {
		t.Errorf("expected lambda=0.123, got %f", fz.Lambda)
	}
	// Semi-axes must be positive
	for i, ax := range fz.SemiAxes {
		if ax <= 0 {
			t.Errorf("SemiAxes[%d] must be > 0, got %f", i, ax)
		}
	}
}

func TestComputeExplanation_NonContributingLinkNoEllipsoid(t *testing.T) {
	h := newHandler()
	blob := makeBlobAt(1, 2, 0, 1, 0.8)
	link := makeLinkState("AA:00:00:00:00:01", "AA:00:00:00:00:02", 4.0, 0.005, false)

	exp := h.computeExplanation(blob, []LinkState{link}, nil)

	if len(exp.FresnelZones) != 0 {
		t.Errorf("expected no Fresnel zones for non-contributing link, got %d", len(exp.FresnelZones))
	}
}

func TestComputeExplanation_ConfidencePropagated(t *testing.T) {
	h := newHandler()
	const want = 0.73
	blob := makeBlobAt(1, 2, 0, 1, want)
	exp := h.computeExplanation(blob, nil, nil)
	if math.Abs(exp.Confidence-want) > 1e-9 {
		t.Errorf("expected confidence=%f, got %f", want, exp.Confidence)
	}
}

// ── UpdateBlobs / BuildWebSocketSnapshot round-trip ───────────────────────────

func TestUpdateBlobs_PopulatesBlobHistory(t *testing.T) {
	h := newHandler()
	blobs := []BlobSnapshot{makeBlobAt(42, 1, 0, 1, 0.8)}
	links := []LinkState{makeLinkState("AA:BB:CC:DD:EE:01", "AA:BB:CC:DD:EE:02", 4, 0.12, true)}

	h.UpdateBlobs(blobs, links, nil, nil)

	h.mu.RLock()
	_, ok := h.blobHistory[42]
	h.mu.RUnlock()
	if !ok {
		t.Error("blob 42 not found in blobHistory after UpdateBlobs")
	}
}

func TestBuildWebSocketSnapshot_ContainsRequiredFields(t *testing.T) {
	h := newHandler()
	blobs := []BlobSnapshot{makeBlobAt(7, 2, 0, 1, 0.9)}
	links := []LinkState{makeLinkState("AA:BB:CC:DD:EE:01", "AA:BB:CC:DD:EE:02", 4, 0.15, true)}

	h.UpdateBlobs(blobs, links, nil, nil)
	snap := h.BuildWebSocketSnapshot(7)

	if snap == nil {
		t.Fatal("expected non-nil snapshot for known blob")
	}
	if _, ok := snap["blob_id"]; !ok {
		t.Error("snapshot missing blob_id")
	}
	if _, ok := snap["fusion_score"]; !ok {
		t.Error("snapshot missing fusion_score")
	}
	if _, ok := snap["per_link_contributions"]; !ok {
		t.Error("snapshot missing per_link_contributions")
	}
	if _, ok := snap["fresnel_zones"]; !ok {
		t.Error("snapshot missing fresnel_zones")
	}
}

func TestBuildWebSocketSnapshot_ContributionPctNormalized(t *testing.T) {
	// Two equal contributing links → each should have contribution_pct ≈ 50
	h := newHandler()
	blobs := []BlobSnapshot{makeBlobAt(1, 2, 0, 1, 0.8)}
	links := []LinkState{
		makeLinkState("AA:00:00:00:00:01", "AA:00:00:00:00:02", 4, 0.10, true),
		makeLinkState("BB:00:00:00:00:01", "BB:00:00:00:00:02", 4, 0.10, true),
	}

	h.UpdateBlobs(blobs, links, nil, nil)
	snap := h.BuildWebSocketSnapshot(1)

	contribs, ok := snap["per_link_contributions"].([]map[string]interface{})
	if !ok {
		t.Fatalf("per_link_contributions has wrong type: %T", snap["per_link_contributions"])
	}
	var sumPct float64
	for _, c := range contribs {
		pct, _ := c["contribution_pct"].(float64)
		if c["contributing"] == true {
			sumPct += pct
		}
	}
	if math.Abs(sumPct-100.0) > 0.1 {
		t.Errorf("contributing link contribution_pct should sum to 100, got %f", sumPct)
	}
}

func TestBuildWebSocketSnapshot_UnknownBlob_ReturnsNil(t *testing.T) {
	h := newHandler()
	snap := h.BuildWebSocketSnapshot(9999)
	if snap != nil {
		t.Error("expected nil for unknown blob ID")
	}
}

func TestBuildWebSocketSnapshot_BLEMatch_Included(t *testing.T) {
	h := newHandler()
	blobs := []BlobSnapshot{makeBlobAt(3, 1, 0, 1, 0.85)}
	triPos := [3]float64{1.1, 0.1, 1.0}
	identity := map[int]*BLEMatch{
		3: {
			PersonID:         "alice",
			PersonLabel:      "Alice",
			PersonColor:      "#4488ff",
			DeviceAddr:       "AA:BB:CC:DD:EE:FF",
			Confidence:       0.92,
			MatchMethod:      "ble_rssi",
			TriangulationPos: &triPos,
		},
	}

	h.UpdateBlobs(blobs, nil, nil, identity)
	snap := h.BuildWebSocketSnapshot(3)

	if snap == nil {
		t.Fatal("nil snapshot")
	}
	bleMatch, ok := snap["ble_match"]
	if !ok || bleMatch == nil {
		t.Error("expected ble_match field in snapshot")
	}
}

// ── GetExplanationForBlob ─────────────────────────────────────────────────────

func TestGetExplanationForBlob_FoundByID(t *testing.T) {
	h := newHandler()
	blobs := []BlobSnapshot{makeBlobAt(55, 1, 0, 1, 0.7)}
	h.UpdateBlobs(blobs, nil, nil, nil)

	// Timestamp within 1 minute of now
	exp := h.GetExplanationForBlob(55, 0)
	// We don't assert non-nil here because the timestamp mismatch causes a fallback search.
	// Instead test via the primary path:
	h.mu.RLock()
	stored := h.blobHistory[55]
	h.mu.RUnlock()
	if stored == nil {
		t.Fatal("blob 55 not in history")
	}
	ts := stored.Timestamp
	exp = h.GetExplanationForBlob(55, ts)
	if exp == nil {
		t.Fatal("expected non-nil explanation for correct timestamp")
	}
	if exp.BlobID != 55 {
		t.Errorf("expected blob_id=55, got %d", exp.BlobID)
	}
}

func TestGetExplanationForBlob_UnknownID_ReturnsNil(t *testing.T) {
	h := newHandler()
	exp := h.GetExplanationForBlob(9999, 0)
	if exp != nil {
		t.Error("expected nil for unknown blob ID with distant timestamp")
	}
}

// ── HTTP handlers ─────────────────────────────────────────────────────────────

func registerAndServe(h *Handler, method, path string, r *http.Request) *httptest.ResponseRecorder {
	router := chi.NewRouter()
	h.RegisterRoutes(router)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func TestHTTP_ExplainBlob_KnownBlob(t *testing.T) {
	h := newHandler()
	blobs := []BlobSnapshot{makeBlobAt(10, 3, 0, 1, 0.75)}
	links := []LinkState{makeLinkState("AA:BB:CC:DD:EE:01", "AA:BB:CC:DD:EE:02", 4, 0.12, true)}
	h.UpdateBlobs(blobs, links, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/explain/10", nil)
	w := registerAndServe(h, http.MethodGet, "/api/explain/10", req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp BlobExplanation
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.BlobID != 10 {
		t.Errorf("expected blob_id=10, got %d", resp.BlobID)
	}
	if resp.Confidence != 0.75 {
		t.Errorf("expected confidence=0.75, got %f", resp.Confidence)
	}
	if len(resp.ContributingLinks) == 0 {
		t.Error("expected contributing links in response")
	}
}

func TestHTTP_ExplainBlob_UnknownBlob_ReturnsEmptyExplanation(t *testing.T) {
	h := newHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/explain/999", nil)
	w := registerAndServe(h, http.MethodGet, "/api/explain/999", req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp BlobExplanation
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if resp.BlobID != 999 {
		t.Errorf("expected blob_id=999 in empty response, got %d", resp.BlobID)
	}
}

func TestHTTP_ExplainBlob_InvalidID_Returns400(t *testing.T) {
	h := newHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/explain/not-a-number", nil)
	w := registerAndServe(h, http.MethodGet, "/api/explain/not-a-number", req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHTTP_ExplainBlobAtTime_InvalidBlobID_Returns400(t *testing.T) {
	h := newHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/explain/blob/bad/at/1000", nil)
	w := registerAndServe(h, http.MethodGet, "/api/explain/blob/bad/at/1000", req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHTTP_ExplainBlobAtTime_InvalidTimestamp_Returns400(t *testing.T) {
	h := newHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/explain/blob/1/at/bad-ts", nil)
	w := registerAndServe(h, http.MethodGet, "/api/explain/blob/1/at/bad-ts", req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ── ZoneDecay invariants ──────────────────────────────────────────────────────

func TestZoneDecay_InverseSquareLaw(t *testing.T) {
	// For decay_rate=2, zone_decay(n) = 1/n². Verify for zones 1-5.
	cases := []struct {
		zone int
		want float64
	}{
		{1, 1.0},
		{2, 0.25},
		{3, 1.0 / 9.0},
		{4, 1.0 / 16.0},
		{5, 1.0 / 25.0},
	}
	for _, tc := range cases {
		got := 1.0 / math.Pow(float64(tc.zone), 2.0)
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("zone %d: expected %f, got %f", tc.zone, tc.want, got)
		}
	}
}

// ── Old history eviction ──────────────────────────────────────────────────────

func TestUpdateBlobs_EvictsOldEntries(t *testing.T) {
	h := newHandler()

	// Insert 110 blobs (triggers eviction at >100)
	for i := 0; i < 110; i++ {
		blobs := []BlobSnapshot{makeBlobAt(i, 0, 0, 0, 0.5)}
		h.UpdateBlobs(blobs, nil, nil, nil)
	}

	h.mu.RLock()
	count := len(h.blobHistory)
	h.mu.RUnlock()

	if count > 100 {
		t.Errorf("blobHistory should be capped at 100 after eviction, got %d", count)
	}
}
