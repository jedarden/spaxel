// Package simulator provides signal propagation modeling for CSI simulation.
package simulator

import (
	"math"
)

// PropagationModel computes expected CSI amplitude using a two-ray model
// (direct path + first-order reflections) with wall attenuation.
type PropagationModel struct {
	space   *Space
	txPower float64 // Transmit power in dBm (default -30)
}

// NewPropagationModel creates a new propagation model for the given space.
func NewPropagationModel(space *Space) *PropagationModel {
	return &PropagationModel{
		space:   space,
		txPower: -30.0, // Default TX power
	}
}

// AmplitudeAt computes the expected CSI amplitude at a walker position
// for a link between TX and RX nodes. This is the primary method used
// by the simulator to generate synthetic CSI data.
//
// The model uses:
// 1. Log-distance path loss: PL(d) = 40 + 20*log10(d) dB
// 2. Wall attenuation: sum of losses for walls intersecting the direct path
// 3. First-order reflection: strongest single-bounce reflection off walls
//
// Returns a normalized amplitude value suitable for deltaRMS computation.
func (pm *PropagationModel) AmplitudeAt(tx, rx, walker Point) float64 {
	// Check if walker is in a valid Fresnel zone
	zone := FresnelZoneNumber(tx, rx, walker)
	if zone > 5 {
		// Outside zone 5, no meaningful contribution
		return 0.0
	}

	// Calculate direct path distance
	directPath := tx.Distance(rx) // TX -> RX direct
	if zone > 5 {
		// Outside zone 5, no meaningful contribution
		return 0.0
	}

	// Compute path loss in dB
	pathLossDB := pm.pathLoss(directPath)

	// Compute wall attenuation for direct path
	wallLoss := pm.wallAttenuation(tx, rx, pm.space)

	// Total received power (dBm)
	rxPowerDBm := pm.txPower - pathLossDB - wallLoss

	// Convert dBm to linear power
	// P(mW) = 10^((dBm + 30)/10)
	rxPowerLinear := math.Pow(10.0, (rxPowerDBm+30.0)/10.0)

	// Compute received power with first-order reflection
	reflectionPower := pm.reflectionPower(tx, rx, walker, pm.space)

	// Combine direct and reflected power (coherent sum approximation)
	// In reality, these would interfere, but for simulation we use power addition
	totalPower := rxPowerLinear + reflectionPower

	// Add Fresnel zone modulation
	// Zone 1 has highest sensitivity, zone 5 has lowest
	zoneModulation := fresnelZoneModulation(zone)

	// Normalize to deltaRMS-like value (0-0.2 range typical)
	// This scaling matches the deltaRMS thresholds used in live detection
	amplitude := totalPower * zoneModulation * 1000.0

	// Clamp to reasonable range
	if amplitude < 0 {
		amplitude = 0
	}
	if amplitude > 0.3 {
		amplitude = 0.3
	}

	return amplitude
}

// pathLoss computes path loss in dB using log-distance model.
// PL(d) = PL_0 + 10*n*log10(d/d_0)
// where PL_0 = 40 dB at d_0 = 1m, n = 2.0 (free space)
func (pm *PropagationModel) pathLoss(distance float64) float64 {
	const PL0 = 40.0 // dB at 1m reference
	const d0 = 1.0   // reference distance in meters
	const n = 2.0    // path loss exponent (free space)

	if distance < 0.01 {
		distance = 0.01 // Avoid log(0)
	}

	return PL0 + 10*n*math.Log10(distance/d0)
}

// wallAttenuation computes total wall attenuation for the TX->RX path.
// Returns dB loss from walls intersecting the direct path.
func (pm *PropagationModel) wallAttenuation(tx, rx Point, space *Space) float64 {
	totalLoss := 0.0

	// Check all walls in the space
	for _, wall := range space.GetWalls() {
		if wall.IntersectsLine(tx, rx) {
			totalLoss += WallPenetrationLoss(wall.Material)
		}
	}

	return totalLoss
}

// reflectionPower computes power from the strongest first-order reflection.
// This simulates signals bouncing off walls before reaching the receiver.
func (pm *PropagationModel) reflectionPower(tx, rx, walker Point, space *Space) float64 {
	maxReflectionPower := 0.0

	// Try each wall as a potential reflector
	for _, wall := range pm.space.GetWalls() {
		// Compute reflection point on wall segment
		reflectionPoint, valid := pm.computeReflectionPoint(tx, rx, wall, pm.space)
		if !valid {
			continue
		}

		// Check if reflection path is plausible
		// TX -> reflection point -> RX
		dReflect := tx.Distance(reflectionPoint) + reflectionPoint.Distance(rx)

		// Check if walker is near the reflection path
		// Use a simple proximity check: walker should be within 2m of reflection point
		if walker.Distance(reflectionPoint) > 2.0 {
			continue
		}

		// Compute path loss for reflected path
	reflectPathLoss := pm.pathLoss(dReflect)

		// Wall reflects some of the signal (not all)
		// Reflection coefficient R (power): 0.3 for typical indoor surfaces
		const R = 0.3

		// Reflected power
		reflectionPowerDBm := pm.txPower - reflectPathLoss - WallPenetrationLoss(wall.Material) + 10*math.Log10(R)
		reflectionPowerLinear := math.Pow(10.0, (reflectionPowerDBm+30.0)/10.0)

		if reflectionPowerLinear > maxReflectionPower {
			maxReflectionPower = reflectionPowerLinear
		}
	}

	return maxReflectionPower
}

// computeReflectionPoint computes the specular reflection point on a wall segment
// for a ray from tx to rx. Returns the reflection point and a validity flag.
func (pm *PropagationModel) computeReflectionPoint(tx, rx Point, wall WallSegment, space *Space) (Point, bool) {
	// For a vertical wall (Z variation), we compute the 2D reflection point on the XY plane
	// and then use the average Z height.

	// Project to 2D (ignore Z for wall reflection calculation)
	tx2D := Point{X: tx.X, Y: tx.Y}
	wallP1 := Point{X: wall.P1.X, Y: wall.P1.Y}
	wallP2 := Point{X: wall.P2.X, Y: wall.P2.Y}

	// Compute reflection using vector math
	// The reflection point is where the angle of incidence equals angle of reflection
	// For a line segment, this can be computed geometrically.

	// Wall direction vector
	wallDir := Point{
		X: wallP2.X - wallP1.X,
		Y: wallP2.Y - wallP1.Y,
	}
	wallLen := math.Sqrt(wallDir.X*wallDir.X + wallDir.Y*wallDir.Y)
	if wallLen < 0.01 {
		return Point{}, false // Wall too short
	}

	// Normalize wall direction
	wallDir.X /= wallLen
	wallDir.Y /= wallLen

	// Compute reflection point using formula for point on line segment
	// that minimizes total path length
	// This is a standard specular reflection calculation

	// Vector from P1 to TX
	v1 := Point{X: tx2D.X - wallP1.X, Y: tx2D.Y - wallP1.Y}

	// Project v1 onto wall direction
	t := v1.X*wallDir.X + v1.Y*wallDir.Y

	// Reflection point (clamped to segment)
	if t < 0 {
		t = 0
	} else if t > wallLen {
		t = wallLen
	}

	reflectionX := wallP1.X + t*wallDir.X
	reflectionY := wallP1.Y + t*wallDir.Y

	// Use average Z height
	reflectionZ := (tx.Z + rx.Z) / 2

	// Check if reflection point is within wall height bounds
	wallMinZ := math.Min(wall.P1.Z, wall.P2.Z)
	wallMaxZ := math.Max(wall.P1.Z, wall.P2.Z) + wall.Height

	if reflectionZ < wallMinZ || reflectionZ > wallMaxZ {
		return Point{}, false // Reflection point outside wall height
	}

	return Point{X: reflectionX, Y: reflectionY, Z: reflectionZ}, true
}

// fresnelZoneModulation returns the sensitivity modulation factor for a Fresnel zone.
// Zone 1 has maximum sensitivity (1.0), zone 5 has minimum (0.04).
func fresnelZoneModulation(zone int) float64 {
	if zone < 1 {
		zone = 1
	}
	// Zone decay: 1/zone^2 gives 1.0, 0.25, 0.11, 0.0625, 0.04 for zones 1-5
	return 1.0 / math.Pow(float64(zone), 2.0)
}

// ComputeLinkActivity computes the expected deltaRMS for a link when a walker
// is at the given position. This is used by the simulation engine to determine
// which links are "active" (above threshold) during each tick.
func (pm *PropagationModel) ComputeLinkActivity(link Link, walkerPos Point, threshold float64) float64 {
	amplitude := pm.AmplitudeAt(link.TX.Position, link.RX.Position, walkerPos)
	return amplitude
}

// ExpectedRSSI computes the expected RSSI in dBm for a receiver at the given distance
// from the transmitter, accounting for path loss and wall attenuation.
func (pm *PropagationModel) ExpectedRSSI(tx, rx Point) int8 {
	distance := tx.Distance(rx)
	pathLoss := pm.pathLoss(distance)
	wallLoss := pm.wallAttenuation(tx, rx, pm.space)

	rssi := pm.txPower - pathLoss - wallLoss

	// Clamp to realistic range
	if rssi < -90 {
		rssi = -90
	}
	if rssi > -30 {
		rssi = -30
	}

	return int8(rssi)
}

// PhaseAtSubcarrier computes the expected phase at a given subcarrier index
// for a signal traveling from tx to walker to rx.
func (pm *PropagationModel) PhaseAtSubcarrier(tx, rx, walker Point, subcarrierIndex int, frameNum int) float64 {
	// Total path length (TX -> walker -> RX)
	d1 := tx.Distance(walker)
	d2 := walker.Distance(rx)
	totalDist := d1 + d2

	// Phase = 2π × k × Δf × (d / c)
	phase := 2 * math.Pi * float64(subcarrierIndex) * SubcarrierSpacing * (totalDist / C)

	// Add small temporal variation for realism
	temporalPhase := 0.1 * math.Sin(2*math.Pi*float64(frameNum)/100.0)
	phase += temporalPhase

	// Normalize to [-π, π]
	for phase > math.Pi {
		phase -= 2 * math.Pi
	}
	for phase < -math.Pi {
		phase += 2 * math.Pi
	}

	return phase
}
