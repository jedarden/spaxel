// Package simulator provides signal propagation modeling for CSI simulation.
package simulator

import (
	"math"
)

// PropagationModel computes expected CSI amplitude using a two-ray model
// (direct path + first-order reflections) with wall attenuation.
type PropagationModel struct {
	space           *Space
	txPower         float64 // Transmit power in dBm (default -30)
	reflectionCoeff float64 // Reflection coefficient (power, dimensionless)
}

// NewPropagationModel creates a new propagation model for the given space.
func NewPropagationModel(space *Space) *PropagationModel {
	return &PropagationModel{
		space:           space,
		txPower:         -30.0, // Default TX power
		reflectionCoeff: 0.3,   // Default reflection coefficient
	}
}

// SetTXPower sets the transmit power in dBm
func (pm *PropagationModel) SetTXPower(power float64) {
	pm.txPower = power
}

// SetReflectionCoefficient sets the reflection coefficient (power ratio)
func (pm *PropagationModel) SetReflectionCoefficient(coeff float64) {
	pm.reflectionCoeff = coeff
}

// RayResult represents the result of a ray propagation computation
type RayResult struct {
	PathLength    float64 // Total path length in meters
	Power         float64 // Linear power (mW-equivalent)
	Phase         float64 // Phase in radians
	ReflectionIdx int     // 0 = direct, 1+ = reflection index
}

// AmplitudeAt computes the expected CSI amplitude at a walker position
// for a link between TX and RX nodes. This is the primary method used
// by the simulator to generate synthetic CSI data.
//
// The model uses a two-ray propagation model:
// 1. Direct path: TX -> walker -> RX (bistatic scattering)
// 2. First-order reflections: TX -> wall/floor/ceiling -> RX
//
// Returns a normalized amplitude value suitable for deltaRMS computation.
func (pm *PropagationModel) AmplitudeAt(tx, rx, walker Point) float64 {
	// Check if walker is in a valid Fresnel zone
	zone := FresnelZoneNumber(tx, rx, walker)
	if zone > 5 {
		// Outside zone 5, no meaningful contribution
		return 0.0
	}

	// Compute signal with walker present (scattering model)
	signalWithWalker := pm.computeSignalStrength(tx, rx, walker)

	// Compute signal without walker (empty room baseline)
	signalEmptyRoom := pm.computeEmptyRoomSignal(tx, rx)

	// deltaRMS is the relative change
	// deltaRMS = |amplitude(W) - amplitude(empty_room)| / amplitude(empty_room)
	if signalEmptyRoom < 1e-9 {
		// Avoid division by zero
		return signalWithWalker * 1000.0
	}

	deltaRMS := math.Abs(signalWithWalker-signalEmptyRoom) / signalEmptyRoom

	// Apply Fresnel zone modulation (zone 1 has highest effect)
	zoneModulation := fresnelZoneModulation(zone)
	deltaRMS *= zoneModulation

	// Clamp to reasonable range [0, 0.3]
	if deltaRMS < 0 {
		deltaRMS = 0
	}
	if deltaRMS > 0.3 {
		deltaRMS = 0.3
	}

	return deltaRMS
}

// computeSignalStrength computes the received signal strength when a walker
// is present at the given position. Uses bistatic scattering model:
// - Direct path: TX -> walker -> RX
// - Reflected paths: TX -> wall -> RX
func (pm *PropagationModel) computeSignalStrength(tx, rx, walker Point) float64 {
	// Direct bistatic path: TX -> walker -> RX
	d1 := tx.Distance(walker)
	d2 := walker.Distance(rx)
	directPathLen := d1 + d2

	// Path loss for direct path
	directPathLoss := pm.pathLoss(directPathLen)

	// Wall attenuation for TX->walker and walker->RX segments
	wallLoss := pm.wallAttenuation(tx, walker, pm.space)
	wallLoss += pm.wallAttenuation(walker, rx, pm.space)

	// Direct received power (dBm)
	directPowerDBm := pm.txPower - directPathLoss - wallLoss

	// Convert to linear power
	directPowerLinear := math.Pow(10.0, (directPowerDBm+30.0)/10.0)

	// Add first-order reflections
	reflectionPower := pm.computeReflectionPower(tx, rx)

	// Combine direct and reflected power (coherent sum approximation)
	// In reality, these would interfere, but for simulation we use power addition
	totalPower := directPowerLinear + reflectionPower

	return totalPower
}

// computeEmptyRoomSignal computes the received signal strength in an empty room
// (no walker present). Uses TX -> RX direct path plus reflections.
func (pm *PropagationModel) computeEmptyRoomSignal(tx, rx Point) float64 {
	// Direct path length
	directPathLen := tx.Distance(rx)

	// Path loss for direct path
	directPathLoss := pm.pathLoss(directPathLen)

	// Wall attenuation for direct TX->RX path
	wallLoss := pm.wallAttenuation(tx, rx, pm.space)

	// Direct received power (dBm)
	directPowerDBm := pm.txPower - directPathLoss - wallLoss

	// Convert to linear power
	directPowerLinear := math.Pow(10.0, (directPowerDBm+30.0)/10.0)

	// Add first-order reflections
	reflectionPower := pm.computeReflectionPower(tx, rx)

	// Combine direct and reflected power
	totalPower := directPowerLinear + reflectionPower

	return totalPower
}

// computeReflectionPower computes the total power from all first-order reflections.
// This includes wall reflections, floor reflections, and ceiling reflections.
// Only the strongest reflection per surface type is retained.
func (pm *PropagationModel) computeReflectionPower(tx, rx Point) float64 {
	maxPower := 0.0

	// Try each wall as a potential reflector
	for _, wall := range pm.space.GetWalls() {
		reflectionPoint, valid := pm.computeWallReflectionPoint(tx, rx, wall)
		if !valid {
			continue
		}

		// Compute reflected path length
		dReflect := tx.Distance(reflectionPoint) + reflectionPoint.Distance(rx)

		// Compute path loss for reflected path
		reflectPathLoss := pm.pathLoss(dReflect)

		// Wall penetration loss (signal passes through wall to reflect, or reflects off surface)
		// For reflection, we use the material's reflection coefficient
		// Harder surfaces (metal, concrete) reflect more
		materialReflectionCoeff := pm.materialReflectionCoefficient(wall.Material)

		// Reflected power
		// P_refl = P_tx × R × PL(d_refl)
		reflectionPowerDBm := pm.txPower - reflectPathLoss + 10*math.Log10(materialReflectionCoeff)

		// Convert to linear
		reflectionPowerLinear := math.Pow(10.0, (reflectionPowerDBm+30.0)/10.0)

		if reflectionPowerLinear > maxPower {
			maxPower = reflectionPowerLinear
		}
	}

	// Floor reflection (Z = 0 plane)
	if floorPower := pm.computeFloorReflectionPower(tx, rx); floorPower > maxPower {
		maxPower = floorPower
	}

	// Ceiling reflection (Z = room height)
	if ceilingPower := pm.computeCeilingReflectionPower(tx, rx); ceilingPower > maxPower {
		maxPower = ceilingPower
	}

	return maxPower
}

// materialReflectionCoefficient returns the reflection coefficient for a material.
// Higher values = more reflective (less absorption).
func (pm *PropagationModel) materialReflectionCoefficient(material WallMaterial) float64 {
	// Base reflection coefficient
	baseCoeff := pm.reflectionCoeff

	// Modify based on material properties
	switch material {
	case MaterialMetal:
		return 0.9 // Metal reflects most signal
	case MaterialGlass:
		return 0.7 // Glass is reflective
	case MaterialBrick, MaterialConcrete:
		return 0.5 // Brick/concrete are somewhat reflective
	case MaterialDrywall:
		return baseCoeff // Use default
	default:
		return baseCoeff
	}
}

// computeWallReflectionPoint computes the specular reflection point on a wall segment
// for a ray from tx to rx. Returns the reflection point and a validity flag.
func (pm *PropagationModel) computeWallReflectionPoint(tx, rx Point, wall WallSegment) (Point, bool) {
	// For a vertical wall, we compute the 2D reflection point on the XY plane
	// and then use the average Z height.

	// Project to 2D (ignore Z for wall reflection calculation)
	tx2D := Point{X: tx.X, Y: tx.Y}
	rx2D := Point{X: rx.X, Y: rx.Y}
	wallP1 := Point{X: wall.P1.X, Y: wall.P1.Y}
	wallP2 := Point{X: wall.P2.X, Y: wall.P2.Y}

	// Compute specular reflection point using image source method
	// For a line segment, find the point that minimizes total path length

	// Wall direction vector
	wallDirX := wallP2.X - wallP1.X
	wallDirY := wallP2.Y - wallP1.Y
	wallLen := math.Sqrt(wallDirX*wallDirX + wallDirY*wallDirY)
	if wallLen < 0.01 {
		return Point{}, false // Wall too short
	}

	// Normalize wall direction
	wallDirX /= wallLen
	wallDirY /= wallLen

	// Wall normal (perpendicular to wall direction)
	normalX := -wallDirY
	normalY := wallDirX

	// Check if TX and RX are on opposite sides of the wall
	// Vector from wall point to TX
	txToWallX := tx2D.X - wallP1.X
	txToWallY := tx2D.Y - wallP1.Y

	// Vector from wall point to RX
	rxToWallX := rx2D.X - wallP1.X
	rxToWallY := rx2D.Y - wallP1.Y

	// Check if TX and RX are on same side of infinite line containing wall
	txSide := txToWallX*normalX + txToWallY*normalY
	rxSide := rxToWallX*normalX + rxToWallY*normalY

	// For valid reflection, TX and RX should be on same side
	// (otherwise the signal would pass through the wall, not reflect)
	if txSide*rxSide < 0 {
		// Opposite sides - signal would pass through wall
		// This is handled by wall attenuation, not reflection
		return Point{}, false
	}

	// Compute reflection point using projection
	// The reflection point is where the angle of incidence equals angle of reflection
	// For a flat wall, this is the point on the wall that the ray would hit

	// Project TX onto wall line
	txProjX := wallP1.X
	txProjY := wallP1.Y
	txToWallDirX := tx2D.X - txProjX
	txToWallDirY := tx2D.Y - txProjY
	txProj := txToWallDirX*wallDirX + txToWallDirY*wallDirY

	// Compute reflection point parameter t along wall segment
	// Using image source method: reflect RX across wall to get RX', then find intersection
	// For simplicity, use midpoint projection
	t := txProj

	// Clamp to segment
	if t < 0 {
		t = 0
	} else if t > wallLen {
		t = wallLen
	}

	reflectionX := wallP1.X + t*wallDirX
	reflectionY := wallP1.Y + t*wallDirY

	// Use average Z height for reflection point
	reflectionZ := (tx.Z + rx.Z) / 2

	// Check if reflection point is within wall height bounds
	wallMinZ := math.Min(wall.P1.Z, wall.P2.Z)
	wallMaxZ := math.Max(wall.P1.Z, wall.P2.Z) + wall.Height

	if reflectionZ < wallMinZ || reflectionZ > wallMaxZ {
		return Point{}, false // Reflection point outside wall height
	}

	return Point{X: reflectionX, Y: reflectionY, Z: reflectionZ}, true
}

// computeFloorReflectionPower computes reflection power from the floor (Z=0 plane).
func (pm *PropagationModel) computeFloorReflectionPower(tx, rx Point) float64 {
	// Floor reflection point (mirror of average height across Z=0)
	floorZ := 0.0

	// Reflection point in XY is the midpoint
	reflectionX := (tx.X + rx.X) / 2
	reflectionY := (tx.Y + rx.Y) / 2

	reflectionPoint := Point{X: reflectionX, Y: reflectionY, Z: floorZ}

	// Compute reflected path length
	dReflect := tx.Distance(reflectionPoint) + reflectionPoint.Distance(rx)

	// Path loss for reflected path
	reflectPathLoss := pm.pathLoss(dReflect)

	// Floor reflection coefficient (concrete/floor)
	floorCoeff := 0.4 // Typical floor reflection

	// Reflected power
	reflectionPowerDBm := pm.txPower - reflectPathLoss + 10*math.Log10(floorCoeff)
	reflectionPowerLinear := math.Pow(10.0, (reflectionPowerDBm+30.0)/10.0)

	return reflectionPowerLinear
}

// computeCeilingReflectionPower computes reflection power from the ceiling.
func (pm *PropagationModel) computeCeilingReflectionPower(tx, rx Point) float64 {
	// Get ceiling height from space bounds
	_, _, _, _, _, maxZ := pm.space.Bounds()
	ceilingZ := maxZ

	// Reflection point in XY is the midpoint
	reflectionX := (tx.X + rx.X) / 2
	reflectionY := (tx.Y + rx.Y) / 2

	reflectionPoint := Point{X: reflectionX, Y: reflectionY, Z: ceilingZ}

	// Compute reflected path length
	dReflect := tx.Distance(reflectionPoint) + reflectionPoint.Distance(rx)

	// Path loss for reflected path
	reflectPathLoss := pm.pathLoss(dReflect)

	// Ceiling reflection coefficient (drywall/ceiling)
	ceilingCoeff := 0.3 // Typical ceiling reflection

	// Reflected power
	reflectionPowerDBm := pm.txPower - reflectPathLoss + 10*math.Log10(ceilingCoeff)
	reflectionPowerLinear := math.Pow(10.0, (reflectionPowerDBm+30.0)/10.0)

	return reflectionPowerLinear
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

// wallAttenuation computes total wall attenuation for a path.
// Returns dB loss from walls intersecting the path.
func (pm *PropagationModel) wallAttenuation(from, to Point, space *Space) float64 {
	totalLoss := 0.0

	// Check all walls in the space
	for _, wall := range space.GetWalls() {
		if wall.IntersectsLine(from, to) {
			totalLoss += WallPenetrationLoss(wall.Material)
		}
	}

	return totalLoss
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
	temporalPhase := 0.1 * math.Sin(2 * math.Pi * float64(frameNum) / 100.0)
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

// ComputeRays computes all ray paths between TX and RX for detailed analysis.
// Returns direct ray plus first-order reflections (walls, floor, ceiling).
func (pm *PropagationModel) ComputeRays(tx, rx Point) []RayResult {
	rays := make([]RayResult, 0, 1+len(pm.space.GetWalls())+2)

	// Direct ray
	directDist := tx.Distance(rx)
	directPathLoss := pm.pathLoss(directDist)
	directWallLoss := pm.wallAttenuation(tx, rx, pm.space)
	directPowerDBm := pm.txPower - directPathLoss - directWallLoss
	directPowerLinear := math.Pow(10.0, (directPowerDBm+30.0)/10.0)
	directPhase := 2 * math.Pi * directDist / Wavelength

	rays = append(rays, RayResult{
		PathLength:    directDist,
		Power:         directPowerLinear,
		Phase:         directPhase,
		ReflectionIdx: 0, // Direct path
	})

	// Wall reflections
	for _, wall := range pm.space.GetWalls() {
		reflectionPoint, valid := pm.computeWallReflectionPoint(tx, rx, wall)
		if !valid {
			continue
		}

		dReflect := tx.Distance(reflectionPoint) + reflectionPoint.Distance(rx)
		reflectPathLoss := pm.pathLoss(dReflect)
		materialCoeff := pm.materialReflectionCoefficient(wall.Material)
		reflectionPowerDBm := pm.txPower - reflectPathLoss + 10*math.Log10(materialCoeff)
		reflectionPowerLinear := math.Pow(10.0, (reflectionPowerDBm+30.0)/10.0)
		reflectionPhase := 2 * math.Pi * dReflect / Wavelength

		rays = append(rays, RayResult{
			PathLength:    dReflect,
			Power:         reflectionPowerLinear,
			Phase:         reflectionPhase,
			ReflectionIdx: len(rays),
		})
	}

	// Floor reflection
	_, _, _, _, _, maxZ := pm.space.Bounds()
	floorZ := 0.0
	reflectionZ := -((tx.Z + rx.Z) / 2)
	reflectionPoint := Point{X: (tx.X + rx.X) / 2, Y: (tx.Y + rx.Y) / 2, Z: floorZ}
	dReflect := tx.Distance(reflectionPoint) + reflectionPoint.Distance(Point{X: reflectionPoint.X, Y: reflectionPoint.Y, Z: reflectionZ})
	floorCoeff := 0.4
	floorPathLoss := pm.pathLoss(dReflect)
	floorPowerDBm := pm.txPower - floorPathLoss + 10*math.Log10(floorCoeff)
	floorPowerLinear := math.Pow(10.0, (floorPowerDBm+30.0)/10.0)
	floorPhase := 2 * math.Pi * dReflect / Wavelength

	rays = append(rays, RayResult{
		PathLength:    dReflect,
		Power:         floorPowerLinear,
		Phase:         floorPhase,
		ReflectionIdx: len(rays),
	})

	// Ceiling reflection
	ceilingZ := maxZ
	reflectionZ = ceilingZ + (ceilingZ - (tx.Z+rx.Z)/2)
	reflectionPoint = Point{X: (tx.X + rx.X) / 2, Y: (tx.Y + rx.Y) / 2, Z: ceilingZ}
	dReflect = tx.Distance(reflectionPoint) + reflectionPoint.Distance(Point{X: reflectionPoint.X, Y: reflectionPoint.Y, Z: reflectionZ})
	ceilingCoeff := 0.3
	ceilingPathLoss := pm.pathLoss(dReflect)
	ceilingPowerDBm := pm.txPower - ceilingPathLoss + 10*math.Log10(ceilingCoeff)
	ceilingPowerLinear := math.Pow(10.0, (ceilingPowerDBm+30.0)/10.0)
	ceilingPhase := 2 * math.Pi * dReflect / Wavelength

	rays = append(rays, RayResult{
		PathLength:    dReflect,
		Power:         ceilingPowerLinear,
		Phase:         ceilingPhase,
		ReflectionIdx: len(rays),
	})

	return rays
}
