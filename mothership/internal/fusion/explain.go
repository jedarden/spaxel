package fusion

import (
	"math"
	"time"
)

// ExplainabilitySnapshot contains all data needed to explain why a specific
// blob appeared at a specific position. It is emitted alongside each BlobUpdate.
type ExplainabilitySnapshot struct {
	// BlobID is the ID of the blob being explained.
	BlobID int `json:"blob_id"`
	// BlobPosition is the final estimated position [x, y, z] in metres.
	BlobPosition [3]float64 `json:"blob_position"`
	// PerLinkContributions describes how each link contributed to this detection.
	PerLinkContributions []ExplainLinkContribution `json:"per_link_contributions"`
	// BLEMatch is optional identity information if a BLE device matched.
	BLEMatch *ExplainBLEMatch `json:"ble_match,omitempty"`
	// FusionScore is the total occupancy grid score at blob position.
	FusionScore float64 `json:"fusion_score"`
	// Timestamp is when this snapshot was generated.
	Timestamp time.Time `json:"timestamp"`
}

// ExplainLinkContribution describes a single link's contribution to a blob.
type ExplainLinkContribution struct {
	// LinkID is the canonical link identifier ("tx_mac:rx_mac").
	LinkID string `json:"link_id"`
	// TXMAC is the transmitting node's MAC address.
	TXMAC string `json:"tx_mac"`
	// RXMAC is the receiving node's MAC address.
	RXMAC string `json:"rx_mac"`
	// Weight is the geometric Fresnel weight (health score) for this link.
	Weight float64 `json:"weight"`
	// LearnedWeight is the per-link learned spatial weight from the weight learner.
	LearnedWeight float64 `json:"learned_weight"`
	// CombinedWeight is Weight * LearnedWeight.
	CombinedWeight float64 `json:"combined_weight"`
	// DeltaRMS is the current deltaRMS for this link.
	DeltaRMS float64 `json:"delta_rms"`
	// ContributionPct is the percentage of total fusion score contributed by this link.
	ContributionPct float64 `json:"contribution_pct"`
	// FresnelIntersectionVolume is a proxy for how much this link "sees" the blob
	// position — estimated from ellipsoid volume and zone decay.
	FresnelIntersectionVolume float64 `json:"fresnel_intersection_volume"`
	// ZoneNumber is the Fresnel zone number at the blob position (1 = highest sensitivity).
	ZoneNumber int `json:"zone_number"`
	// Contributing is true if this link actively contributed (motion above threshold).
	Contributing bool `json:"contributing"`
}

// ExplainBLEMatch holds optional BLE identity information for a blob.
type ExplainBLEMatch struct {
	DeviceMAC               string  `json:"device_mac"`
	PersonID                string  `json:"person_id"`
	PersonLabel             string  `json:"person_label"`
	BLEDistanceM            float64 `json:"ble_distance_m"`
	TriangulationConfidence float64 `json:"triangulation_confidence"`
}

// GenerateExplainabilitySnapshot creates an ExplainabilitySnapshot for a single
// blob using the fusion result and per-link motion data.
//
//   - result is the most recent fusion result.
//   - blobIdx selects which blob in result.Blobs to explain.
//   - blobID is the tracking ID assigned by the blob tracker.
//   - links is the full list of links processed in the most recent fusion cycle.
//   - nodePos maps node MAC addresses to their 3D positions.
//   - learnedWeights maps canonical link IDs to their learned weight (1.0 default).
//   - lambda is the WiFi wavelength in metres (0.125 for 2.4 GHz, 0.06 for 5 GHz).
//   - cellSize is the fusion grid cell size in metres.
func GenerateExplainabilitySnapshot(
	result *Result,
	blobIdx int,
	blobID int,
	links []LinkMotion,
	nodePos map[string]NodePosition,
	learnedWeights map[string]float64,
	lambda, cellSize float64,
) *ExplainabilitySnapshot {
	if result == nil || blobIdx < 0 || blobIdx >= len(result.Blobs) {
		return nil
	}
	if lambda <= 0 {
		lambda = 0.125
	}
	if cellSize <= 0 {
		cellSize = 0.2
	}

	blob := result.Blobs[blobIdx]
	snap := &ExplainabilitySnapshot{
		BlobID:       blobID,
		BlobPosition: [3]float64{blob.X, blob.Y, blob.Z},
		FusionScore:  blob.Confidence,
		Timestamp:    result.Timestamp,
	}

	type linkScore struct {
		lm       LinkMotion
		posA     NodePosition
		posB     NodePosition
		weight   float64
		learned  float64
		combined float64
		zoneNum  int
		rawScore float64
		fiv      float64
	}

	// Compute raw score for each link at the blob position.
	scores := make([]linkScore, 0, len(links))
	totalScore := 0.0

	for _, lm := range links {
		posA, okA := nodePos[lm.NodeMAC]
		posB, okB := nodePos[lm.PeerMAC]
		if !okA || !okB {
			continue
		}

		linkID := lm.NodeMAC + ":" + lm.PeerMAC
		learned := 1.0
		if lw, ok := learnedWeights[linkID]; ok && lw > 0 {
			learned = lw
		}

		// Geometric Fresnel weight comes from the HealthScore field; default to 1.0.
		geoWeight := lm.HealthScore
		if geoWeight <= 0 {
			geoWeight = 1.0
		}
		combined := geoWeight * learned

		// Fresnel zone number at blob position.
		zoneNum := fresnelZoneAtPosition(posA, posB, blob.X, blob.Y, blob.Z)

		// Zone decay (decay_rate = 2.0 per plan.md).
		zoneDecay := 1.0 / math.Pow(float64(zoneNum), 2.0)

		rawScore := lm.DeltaRMS * combined * zoneDecay
		fiv := fresnelIntersectionVolume(posA, posB, blob.X, blob.Y, blob.Z, cellSize, lambda)

		totalScore += rawScore
		scores = append(scores, linkScore{
			lm:       lm,
			posA:     posA,
			posB:     posB,
			weight:   geoWeight,
			learned:  learned,
			combined: combined,
			zoneNum:  zoneNum,
			rawScore: rawScore,
			fiv:      fiv,
		})
	}

	// Build ExplainLinkContribution for each link with normalised contribution_pct.
	contribs := make([]ExplainLinkContribution, 0, len(scores))
	for _, s := range scores {
		pct := 0.0
		if totalScore > 0 {
			pct = (s.rawScore / totalScore) * 100.0
		}
		linkID := s.lm.NodeMAC + ":" + s.lm.PeerMAC
		contribs = append(contribs, ExplainLinkContribution{
			LinkID:                    linkID,
			TXMAC:                     s.lm.NodeMAC,
			RXMAC:                     s.lm.PeerMAC,
			Weight:                    s.weight,
			LearnedWeight:             s.learned,
			CombinedWeight:            s.combined,
			DeltaRMS:                  s.lm.DeltaRMS,
			ContributionPct:           pct,
			FresnelIntersectionVolume: s.fiv,
			ZoneNumber:                s.zoneNum,
			Contributing:              s.lm.Motion && s.lm.DeltaRMS > 0.02,
		})
	}

	snap.PerLinkContributions = contribs
	return snap
}

// fresnelIntersectionVolume estimates the volume of the first Fresnel zone ellipsoid
// that overlaps a voxel of the given cellSize centred on the blob position.
//
// This is a simplified proxy calculation: the actual ellipsoid/voxel intersection
// requires expensive numerical integration. Instead we estimate by scaling the
// full ellipsoid volume by the zone decay factor and capping at one voxel volume.
func fresnelIntersectionVolume(tx, rx NodePosition, px, py, pz, cellSize, lambda float64) float64 {
	if lambda <= 0 {
		lambda = 0.125
	}

	// Direct path distance for the link.
	dxl := rx.X - tx.X
	dyl := rx.Y - tx.Y
	dzl := rx.Z - tx.Z
	directDist := math.Sqrt(dxl*dxl + dyl*dyl + dzl*dzl)
	if directDist < 1e-9 {
		return 0
	}

	// First Fresnel zone ellipsoid semi-axes.
	a := (directDist + lambda/2) / 2
	bAxis := math.Sqrt(math.Max(0, a*a-(directDist/2)*(directDist/2)))
	ellipsoidVolume := (4.0 / 3.0) * math.Pi * a * bAxis * bAxis

	// Path length excess at the blob position.
	dtx := math.Sqrt((px-tx.X)*(px-tx.X) + (py-tx.Y)*(py-tx.Y) + (pz-tx.Z)*(pz-tx.Z))
	dtr := math.Sqrt((rx.X-px)*(rx.X-px) + (rx.Y-py)*(rx.Y-py) + (rx.Z-pz)*(rx.Z-pz))
	excess := dtx + dtr - directDist
	if excess < 0 {
		excess = 0
	}

	// Zone number and decay.
	zone := math.Ceil(excess / (lambda / 2))
	if zone < 1 {
		zone = 1
	}
	decay := 1.0 / (zone * zone)

	voxelVol := cellSize * cellSize * cellSize
	return math.Min(ellipsoidVolume, voxelVol) * decay
}

// ComputeFresnelEllipsoidAxes returns the semi-major axis (a), semi-minor axis (b),
// and link distance (d) for the first Fresnel zone ellipsoid of a link.
//
// TX and RX give the positions of the two endpoints; lambda is the wavelength in metres.
// Returns a, b, d.
func ComputeFresnelEllipsoidAxes(tx, rx NodePosition, lambda float64) (a, b, d float64) {
	if lambda <= 0 {
		lambda = 0.125
	}
	dx := rx.X - tx.X
	dy := rx.Y - tx.Y
	dz := rx.Z - tx.Z
	d = math.Sqrt(dx*dx + dy*dy + dz*dz)
	a = (d + lambda/2) / 2
	b = math.Sqrt(math.Max(0, a*a-(d/2)*(d/2)))
	return
}
