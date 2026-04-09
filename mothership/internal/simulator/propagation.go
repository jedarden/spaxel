package simulator

import (
	"math"
)

// PropagationModel computes RF signal propagation characteristics
// Uses simplified two-ray model (direct + first-order reflection)
type PropagationModel struct {
	space *Space
}

// NewPropagationModel creates a propagation model for a space
func NewPropagationModel(space *Space) *PropagationModel {
	return &PropagationModel{space: space}
}

// PathLoss computes the path loss in dB for a given distance
// Using log-distance model: PL(d) = PL_0 + 10*n*log10(d/d_0)
// where PL_0 = 40 dB at d_0 = 1m, n = 2.0 (free space)
func (pm *PropagationModel) PathLoss(distance float64) float64 {
	const PL0 = 40.0 // dB at 1m
	const d0 = 1.0   // reference distance in meters
	const n = 2.0    // path loss exponent (free space)

	if distance < 0.01 {
		distance = 0.01 // Avoid log(0)
	}

	return PL0 + 10*n*math.Log10(distance/d0)
}

// WallLoss computes the total wall penetration loss for a path
// Returns the sum of losses for all walls intersected by the path
func (pm *PropagationModel) WallLoss(from, to Point) float64 {
	totalLoss := 0.0
	walls := pm.space.GetWalls()

	for _, wall := range walls {
		if wall.IntersectsLine(from, to) {
			totalLoss += WallPenetrationLoss(wall.Material)
		}
	}

	return totalLoss
}

// ReceivedPower computes the expected received signal power in dBm
// at position 'to' from a transmitter at position 'from' with transmit power txPowerdBm
func (pm *PropagationModel) ReceivedPower(from, to Point, txPowerdBm float64) float64 {
	distance := from.Distance(to)
	pathLoss := pm.PathLoss(distance)
	wallLoss := pm.WallLoss(from, to)

	// Add reflected signal contribution (simplified)
	reflectionPower := pm.reflectionContribution(from, to, txPowerdBm)

	// Total power = direct + reflected (incoherent sum in power domain)
	directPower := txPowerdBm - pathLoss - wallLoss

	// Convert to linear, add, convert back to dB
	directLin := math.Pow(10, directPower/10.0)
	reflectionLin := math.Pow(10, reflectionPower/10.0)
	totalLin := directLin + reflectionLin

	return 10 * math.Log10(totalLin)
}

// reflectionContribution computes the power contribution from the strongest reflection
// Returns power in dBm
func (pm *PropagationModel) reflectionContribution(from, to Point, txPowerdBm float64) float64 {
	const reflectionCoeff = 0.3 // Power reflection coefficient

	// Find the wall with the weakest material (lowest loss) for reflection
	walls := pm.space.GetWalls()
	if len(walls) == 0 {
		return -100 // No walls, no reflection
	}

	bestReflectionPower := -100.0

	for _, wall := range walls {
		// Compute reflection point (simplified: use wall midpoint)
		wallMidX := (wall.P1.X + wall.P2.X) / 2
		wallMidY := (wall.P1.Y + wall.P2.Y) / 2
		wallMidZ := (wall.P1.Z + wall.P2.Z + wall.Height) / 2

		reflectionPoint := Point{X: wallMidX, Y: wallMidY, Z: wallMidZ}

		// Total path length: from -> reflectionPoint -> to
		d1 := from.Distance(reflectionPoint)
		d2 := reflectionPoint.Distance(to)
		totalDist := d1 + d2

		// Path loss for reflected path
		pathLoss := pm.PathLoss(totalDist)

		// Reflection loss (material-dependent)
		reflectionLoss := WallPenetrationLoss(wall.Material)

		// Total reflected power
		reflectedPower := txPowerdBm - pathLoss - reflectionLoss - 10*math.Log10(1.0/reflectionCoeff)

		if reflectedPower > bestReflectionPower {
			bestReflectionPower = reflectedPower
		}
	}

	return bestReflectionPower
}

// AmplitudeAt computes the expected CSI amplitude at a receiver position
// from a transmitter, normalized to [0, 1] range
func (pm *PropagationModel) AmplitudeAt(tx, rx, walker Point) float64 {
	// Distance from TX to walker
	d1 := tx.Distance(walker)
	// Distance from walker to RX
	d2 := walker.Distance(rx)
	// Direct TX-RX distance
	dDirect := tx.Distance(rx)

	// Path length excess (Fresnel zone calculation)
	excess := d1 + d2 - dDirect
	if excess < 0 {
		excess = 0
	}

	// Fresnel zone number
	zoneNumber := math.Ceil(excess / HalfWavelength)
	if zoneNumber < 1 {
		zoneNumber = 1
	}

	// Base amplitude from received power
	txPower := 20.0 // dBm (typical WiFi TX power)
	rxPower := pm.ReceivedPower(tx, rx, txPower)

	// Convert to linear amplitude (normalized)
	// Reference: -30 dBm = 1.0 amplitude
	amplitude := math.Pow(10, (rxPower+30)/20.0)

	// Modulate by Fresnel zone
	// Zone 1: maximum, Zone 5+: minimum
	if zoneNumber >= 5 {
		amplitude *= 0.01
	} else {
		decay := math.Pow(zoneNumber, 2.0)
		amplitude /= decay
	}

	return amplitude
}

// PhaseAt computes the expected CSI phase at a subcarrier index
// for a given link and walker position
func (pm *PropagationModel) PhaseAt(tx, rx, walker Point, subcarrierIndex int) float64 {
	// Total path length (TX -> walker -> RX)
	d1 := tx.Distance(walker)
	d2 := walker.Distance(rx)
	totalDist := d1 + d2

	// Phase = 2π × k × Δf × (d / c)
	// where k is subcarrier index, Δf is subcarrier spacing
	phase := 2 * math.Pi * float64(subcarrierIndex) * SubcarrierSpacing * (totalDist / C)

	// Normalize to [-π, π]
	for phase > math.Pi {
		phase -= 2 * math.Pi
	}
	for phase < -math.Pi {
		phase += 2 * math.Pi
	}

	return phase
}

// DeltaRMS computes the expected deltaRMS motion score
// when a walker is at the given position (vs empty room)
func (pm *PropagationModel) DeltaRMS(tx, rx, walker Point, baselineAmplitude float64) float64 {
	// Amplitude with walker present
	amplitudeWithWalker := pm.AmplitudeAt(tx, rx, walker)

	// DeltaRMS = |amplitude - baseline| / baseline
	if baselineAmplitude < 1e-6 {
		baselineAmplitude = 1e-6
	}

	delta := math.Abs(amplitudeWithWalker - baselineAmplitude)

	return delta / baselineAmplitude
}

// Link represents a TX-RX link for simulation
type Link struct {
	TX *Node
	RX *Node
}

// ComputeLinkActivity computes whether a link would be "active"
// (deltaRMS above threshold) given a walker position
func (pm *PropagationModel) ComputeLinkActivity(link Link, walker Point, threshold float64) float64 {
	// Baseline amplitude (empty room)
	baseline := pm.AmplitudeAt(link.TX.Position, link.RX.Position, Point{X: -1000, Y: -1000, Z: 0})

	// DeltaRMS with walker
	deltaRMS := pm.DeltaRMS(link.TX.Position, link.RX.Position, walker, baseline)

	return deltaRMS
}

// GenerateAllLinks generates all possible TX-RX links from a node set
func GenerateAllLinks(nodes *NodeSet) []Link {
	links := make([]Link, 0)
	txs := nodes.TXNodes()
	rxs := nodes.RXNodes()

	for _, tx := range txs {
		for _, rx := range rxs {
			if tx.ID == rx.ID {
				continue // Skip self-links
			}
			links = append(links, Link{TX: tx, RX: rx})
		}
	}

	return links
}

// SimulateCSIData generates simulated CSI data for all links and walkers
// Returns a map of link ID to deltaRMS value
func (pm *PropagationModel) SimulateCSIData(links []Link, walkers []*Walker, threshold float64) map[string]float64 {
	results := make(map[string]float64)

	for _, link := range links {
		maxDeltaRMS := 0.0
		linkID := link.TX.ID + ":" + link.RX.ID

		for _, walker := range walkers {
			deltaRMS := pm.ComputeLinkActivity(link, walker.Position, threshold)
			if deltaRMS > maxDeltaRMS {
				maxDeltaRMS = deltaRMS
			}
		}

		// Only include links above threshold
		if maxDeltaRMS >= threshold {
			results[linkID] = maxDeltaRMS
		}
	}

	return results
}

// FresnelZoneNumber computes the Fresnel zone number for a point
// relative to a TX-RX link
func FresnelZoneNumber(tx, rx, point Point) int {
	dAP := tx.Distance(point)
	dPB := point.Distance(rx)
	dAB := tx.Distance(rx)

	excess := dAP + dPB - dAB
	if excess < 0 {
		excess = 0
	}

	zone := int(math.Ceil(excess / HalfWavelength))
	if zone < 1 {
		zone = 1
	}
	return zone
}

// IsInFirstFresnelZone returns true if the point is inside the first Fresnel zone
func IsInFirstFresnelZone(tx, rx, point Point) bool {
	return FresnelZoneNumber(tx, rx, point) == 1
}

// IsInFresnelZones returns true if the point is within the first N Fresnel zones
func IsInFresnelZones(tx, rx, point Point, maxZone int) bool {
	return FresnelZoneNumber(tx, rx, point) <= maxZone
}
