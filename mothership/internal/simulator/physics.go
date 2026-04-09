// Package simulator provides shared physics functions for CSI simulation.
// This package is used by both the pre-deployment simulator and the CSI CLI simulator.
package simulator

import (
	"math"
	"math/rand"
)

// PhysicsModel provides physics calculations for CSI simulation
type PhysicsModel struct {
	space        *Space
	noiseSigma   float64 // Gaussian noise standard deviation for I/Q
	walls        []WallDefinition
}

// WallDefinition defines a wall segment for attenuation calculations
type WallDefinition struct {
	X1, Y1, X2, Y2 float64 // Wall endpoints (floor coordinates)
	Attenuation     float64 // dB attenuation
}

// NewPhysicsModel creates a new physics model for the given space
func NewPhysicsModel(space *Space) *PhysicsModel {
	return &PhysicsModel{
		space:      space,
		noiseSigma: 0.005, // Default noise level
		walls:      make([]WallDefinition, 0),
	}
}

// SetNoiseSigma sets the Gaussian noise standard deviation
func (pm *PhysicsModel) SetNoiseSigma(sigma float64) {
	pm.noiseSigma = sigma
}

// AddWall adds a wall definition to the physics model
func (pm *PhysicsModel) AddWall(x1, y1, x2, y2, attenuation float64) {
	pm.walls = append(pm.walls, WallDefinition{
		X1:          x1,
		Y1:          y1,
		X2:          x2,
		Y2:          y2,
		Attenuation: attenuation,
	})
}

// PathLossdB computes path loss in dB using log-distance model
// PL(d) = PL_0 + 10*n*log10(d/d_0)
// where PL_0 = 40 dB at d_0 = 1m, n = 2.0 (free space)
func (pm *PhysicsModel) PathLossdB(distance float64) float64 {
	const PL0 = 40.0 // dB at 1m reference
	const d0 = 1.0   // reference distance in meters
	const n = 2.0    // path loss exponent (free space)

	if distance < 0.01 {
		distance = 0.01 // Avoid log(0)
	}

	return PL0 + 10*n*math.Log10(distance/d0)
}

// WallAttenuation computes total wall attenuation for a path
func (pm *PhysicsModel) WallAttenuation(from, to Point) float64 {
	totalLoss := 0.0

	for _, wall := range pm.walls {
		if pm.pathIntersectsWall(from.X, from.Y, to.X, to.Y,
			wall.X1, wall.Y1, wall.X2, wall.Y2) {
			totalLoss += wall.Attenuation
		}
	}

	return totalLoss
}

// pathIntersectsWall checks if a path intersects a wall segment (2D)
func (pm *PhysicsModel) pathIntersectsWall(x1, y1, x2, y2, wx1, wy1, wx2, wy2 float64) bool {
	// Compute orientations
	ccw := func(ax, ay, bx, by, cx, cy float64) float64 {
		return (bx-ax)*(cy-ay) - (by-ay)*(cx-ax)
	}

	o1 := ccw(x1, y1, x2, y2, wx1, wy1)
	o2 := ccw(x1, y1, x2, y2, wx2, wy2)
	o3 := ccw(wx1, wy1, wx2, wy2, x1, y1)
	o4 := ccw(wx1, wy1, wx2, wy2, x2, y2)

	// Check for intersection
	return o1*o2 < 0 && o3*o4 < 0
}

// ComputeRSSI computes the RSSI in dBm for a given distance
// Returns RSSI in range [-90, -30] dBm
func (pm *PhysicsModel) ComputeRSSI(distance float64) int8 {
	pathLoss := pm.PathLossdB(distance)
	txPower := -30.0 // Reference transmit power in dBm

	rssi := txPower - pathLoss

	// Clamp to realistic range
	if rssi < -90 {
		rssi = -90
	}
	if rssi > -30 {
		rssi = -30
	}

	return int8(rssi)
}

// DeltaRMS computes the expected deltaRMS motion score
// when a walker is at the given position (vs empty room)
func (pm *PhysicsModel) DeltaRMS(tx, rx, walker Point) float64 {
	// Calculate Fresnel zone number
	zone := FresnelZoneNumber(tx, rx, walker)

	// DeltaRMS is highest in zone 1, decreases with zone number
	// Zone 1: 0.15, Zone 2: 0.08, Zone 3: 0.04, Zone 4: 0.02, Zone 5+: 0.01
	switch zone {
	case 1:
		return 0.15
	case 2:
		return 0.08
	case 3:
		return 0.04
	case 4:
		return 0.02
	default:
		return 0.01
	}
}

// GenerateIQPair generates a synthetic I/Q pair for a subcarrier
// with amplitude and phase, plus Gaussian noise
func (pm *PhysicsModel) GenerateIQPair(amplitude, phase float64) (int8, int8) {
	// Generate Gaussian noise using Box-Muller transform
	u1 := rand.Float64()
	u2 := rand.Float64()
	z0 := math.Sqrt(-2.0*math.Log(u1)) * math.Cos(2.0*math.Pi*u2)
	z1 := math.Sqrt(-2.0*math.Log(u1)) * math.Sin(2.0*math.Pi*u2)

	noiseI := z0 * pm.noiseSigma
	noiseQ := z1 * pm.noiseSigma

	// Convert to I/Q
	i := amplitude*math.Cos(phase) + noiseI
	q := amplitude*math.Sin(phase) + noiseQ

	// Clamp to int8 range [-127, 127]
	// Note: We avoid -128 to prevent overflow issues
	if i > 127 {
		i = 127
	}
	if i < -127 {
		i = -127
	}
	if q > 127 {
		q = 127
	}
	if q < -127 {
		q = -127
	}

	return int8(i), int8(q)
}

// GenerateSubcarrierCSI generates CSI data for all subcarriers
func (pm *PhysicsModel) GenerateSubcarrierCSI(tx, rx, walker Point, nSub int, frameNum int) []struct{ I, Q int8 } {
	result := make([]struct{ I, Q int8 }, nSub)

	// Base amplitude from deltaRMS
	deltaRMS := pm.DeltaRMS(tx, rx, walker)
	amplitude := deltaRMS * 500.0 // Scale to reasonable I/Q range

	for k := 0; k < nSub; k++ {
		// Compute phase at this subcarrier
		phase := pm.phaseAtSubcarrier(tx, rx, walker, k, frameNum)

		// Add subcarrier-dependent amplitude variation
		// Simulates frequency-selective fading
		freqFading := 0.8 + 0.4*math.Sin(2*math.Pi*float64(k)/16.0)
		subAmplitude := amplitude * freqFading

		result[k].I, result[k].Q = pm.GenerateIQPair(subAmplitude, phase)
	}

	return result
}

// PhaseAtSubcarrier computes phase for a given subcarrier index
func (pm *PhysicsModel) PhaseAtSubcarrier(tx, rx, walker Point, subcarrierIndex, frameNum int) float64 {
	// Total path length (TX -> walker -> RX)
	d1 := tx.Distance(walker)
	d2 := walker.Distance(rx)
	totalDist := d1 + d2

	// Phase = 2π × k × Δf × (d / c) + temporal_variation
	phase := 2*math.Pi*float64(subcarrierIndex)*SubcarrierSpacing*(totalDist/C)

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

// ValidateRSSI validates that RSSI is within expected range for distance
func ValidateRSSI(rssi int8, distance float64) bool {
	// Expected RSSI range for given distance
	expectedPathLoss := 40.0 + 20.0*math.Log10(distance/1.0)
	expectedRSSI := -30.0 - expectedPathLoss

	// Allow ±20 dB tolerance
	minRSSI := expectedRSSI - 20.0
	maxRSSI := expectedRSSI + 20.0

	// Clamp to realistic bounds
	if minRSSI < -90 {
		minRSSI = -90
	}
	if maxRSSI > -30 {
		maxRSSI = -30
	}

	return float64(rssi) >= minRSSI && float64(rssi) <= maxRSSI
}

// ValidateIQValues checks that I/Q values are in valid int8 range
func ValidateIQValues(i, q int8) bool {
	return i >= -127 && i <= 127 && q >= -127 && q <= 127
}

// IsInFresnelZones checks if a point is within the first N Fresnel zones
func IsInFresnelZones(tx, rx, point Point, maxZone int) bool {
	zone := FresnelZoneNumber(tx, rx, point)
	return zone <= maxZone && zone > 0
}

// ComputeFresnelModulation computes the Fresnel zone modulation factor
// Returns a value between 0 and 1, where 1 is maximum modulation (zone 1)
func ComputeFresnelModulation(tx, rx, point Point) float64 {
	zone := FresnelZoneNumber(tx, rx, point)

	// Zone 1: maximum modulation, Zone 5+: minimum
	if zone <= 1 {
		return 1.0
	}
	if zone >= 5 {
		return 0.0
	}

	return 1.0 / math.Pow(float64(zone), 2.0)
}

// ComputeLinkQuality estimates link quality (0-1) based on geometry
// Higher quality when links have good angular diversity
func ComputeLinkQuality(nodes []Point) float64 {
	if len(nodes) < 2 {
		return 0.0
	}

	// Simple metric: spread of node positions
	// Compute centroid
	var cx, cy, cz float64
	for _, n := range nodes {
		cx += n.X
		cy += n.Y
		cz += n.Z
	}
	cx /= float64(len(nodes))
	cy /= float64(len(nodes))
	cz /= float64(len(nodes))

	// Compute average distance from centroid
	avgDist := 0.0
	for _, n := range nodes {
		dx := n.X - cx
		dy := n.Y - cy
		dz := n.Z - cz
		avgDist += math.Sqrt(dx*dx + dy*dy + dz*dz)
	}
	avgDist /= float64(len(nodes))

	// Normalize: 5m spread = excellent quality (1.0)
	quality := avgDist / 5.0
	if quality > 1.0 {
		quality = 1.0
	}

	return quality
}
