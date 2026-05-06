package main

import (
	"encoding/binary"
	"math"
	"math/rand"
)

const (
	// WiFi physical constants
	wavelength        = 0.123     // meters (2.4 GHz)
	halfWavelength    = wavelength / 2.0
	subcarrierSpacing = 312.5e3 // Hz
	c                 = 3e8     // speed of light m/s

	// CSI frame constants — must match ingestion/frame.go format
	nSub = 64 // number of subcarriers for HT20

	// Path loss model constants (log-distance model)
	// PL(d) = PL_0 + 10*n*log10(d/d_0)
	pl0    = 40.0 // dBm reference power at d0
	d0     = 1.0   // meters reference distance
	n      = 2.0   // path loss exponent (free space)

	// Reflection coefficient (power, dimensionless)
	reflectionCoeff = 0.3
)

// generateCSIFrame generates a synthetic CSI binary frame.
// The frame format matches the ingestion layer (ingestion/frame.go):
//
//	Header (24 bytes fixed):
//	  [0:6]   node_mac     — TX node MAC address
//	  [6:12]  peer_mac     — RX node MAC address
//	  [12:20] timestamp_us — uint64 LE, microseconds since node boot
//	  [20]    rssi         — int8, dBm
//	  [21]    noise_floor  — int8, dBm
//	  [22]    channel      — uint8, WiFi channel
//	  [23]    n_sub        — uint8, subcarrier count
//	Payload (n_sub × 2 bytes):
//	  Per subcarrier: int8 I, int8 Q
func generateCSIFrame(tx, rx *VirtualNode, walkers []*Walker, walls []Wall, frameNum int, rng *rand.Rand) []byte {
	// Calculate combined CSI from all walkers
	amplitude, phaseBase := computeCSIForWalkers(tx, rx, walkers, walls)

	// Compute RSSI from amplitude
	rssi := amplitudeToRSSI(amplitude)

	// Create frame buffer (headerSize=24 defined in main.go)
	frame := make([]byte, headerSize+nSub*2)

	// Write header (matches ingestion/frame.go ParseFrame layout)
	copy(frame[0:6], tx.MAC[:])   // node_mac
	copy(frame[6:12], rx.MAC[:])  // peer_mac
	binary.LittleEndian.PutUint64(frame[12:20], uint64(frameNum*50000)) // timestamp_us
	frame[20] = byte(rssi)        // rssi
	var noiseFloor int8 = -95
	frame[21] = byte(noiseFloor) // noise_floor: -95 dBm
	frame[22] = byte(*flagChannel) // channel (from --channel flag)
	frame[23] = nSub              // n_sub

	// Generate I/Q pairs for each subcarrier
	for k := 0; k < nSub; k++ {
		// Phase for this subcarrier
		phase := phaseBase + float64(k)*0.1

		// Add temporal variation
		phase += 0.1 * math.Sin(2*math.Pi*float64(frameNum)/100.0)

		// Normalize phase to [-π, π]
		for phase > math.Pi {
			phase -= 2 * math.Pi
		}
		for phase < -math.Pi {
			phase += 2 * math.Pi
		}

		// Add frequency-selective fading
		freqFading := 0.8 + 0.4*math.Sin(2*math.Pi*float64(k)/16.0)
		subAmplitude := amplitude * freqFading

		// Generate I/Q with noise
		i, q := generateIQPair(subAmplitude, phase, rng)

		// Write to payload (interleaved I,Q)
		offset := headerSize + k*2
		frame[offset] = byte(i)
		frame[offset+1] = byte(q)
	}

	return frame
}

// computeCSIForWalkers computes the combined CSI amplitude and phase from all walkers
// using a two-ray propagation model (direct path + first-order reflection).
func computeCSIForWalkers(tx, rx *VirtualNode, walkers []*Walker, walls []Wall) (float64, float64) {
	if len(walkers) == 0 {
		// No walkers, return baseline noise
		return 0.001, 0.0
	}

	var totalAmplitude float64
	var totalPhase float64
	var weight float64

	for _, walker := range walkers {
		// Direct path contribution
		directAmp, directPhase := computeDirectPath(tx.Position, rx.Position, walker.Position, walls)

		// First-order reflection contribution (strongest reflected ray)
		reflAmp, reflPhase := computeFirstOrderReflection(tx.Position, rx.Position, walker.Position, walls)

		// Combine direct and reflected paths (coherent sum for two-ray model)
		// The reflected ray interferes with the direct ray, creating constructive/destructive interference
		combinedAmp := directAmp + reflAmp*math.Cos(directPhase-reflPhase)
		combinedPhase := math.Atan2(
			directAmp*math.Sin(directPhase)+reflAmp*math.Sin(reflPhase),
			directAmp*math.Cos(directPhase)+reflAmp*math.Cos(reflPhase),
		)

		// Scale to reasonable values
		combinedAmp *= 1000.0

		// Accumulate
		totalAmplitude += combinedAmp
		totalPhase += combinedPhase
		weight += 1.0
	}

	// Normalize phase
	if weight > 0 {
		totalPhase /= weight
	}

	return totalAmplitude, totalPhase
}

// computeDirectPath computes the CSI contribution from the direct path
// through the walker to the receiver, using the log-distance path loss model.
func computeDirectPath(tx, rx, walker Point, walls []Wall) (float64, float64) {
	// Distance from TX to walker
	d1 := distance(tx, walker)
	// Distance from walker to RX
	d2 := distance(walker, rx)
	// Total path length
	dTotal := d1 + d2

	// Direct TX-RX distance (for Fresnel zone calculation)
	dDirect := distance(tx, rx)

	// Path length excess for Fresnel zone calculation
	excess := dTotal - dDirect
	if excess < 0 {
		excess = 0
	}

	// Fresnel zone number
	zoneNumber := int(math.Ceil(excess / halfWavelength))
	if zoneNumber < 1 {
		zoneNumber = 1
	}

	// Zone decay (inverse square)
	decay := 1.0 / math.Pow(float64(zoneNumber), 2.0)

	// Log-distance path loss model: PL(d) = PL_0 + 10*n*log10(d/d_0)
	var pathLossDB float64
	if dTotal >= d0 {
		pathLossDB = pl0 + 10.0*n*math.Log10(dTotal/d0)
	} else {
		// For distances < d0, use free space approximation
		pathLossDB = pl0
	}

	// Wall attenuation on the direct path
	wallLoss := computeWallLoss(tx, walker, walls)
	wallLoss += computeWallLoss(walker, rx, walls)

	// Total loss in dB
	totalLossDB := pathLossDB + wallLoss

	// Convert to linear amplitude
	amplitude := math.Pow(10.0, -totalLossDB/20.0)

	// Apply Fresnel zone decay
	amplitude *= decay

	// Phase at this position (based on total path length)
	phase := 2 * math.Pi * dTotal / wavelength

	return amplitude, phase
}

// computeFirstOrderReflection computes the CSI contribution from the strongest
// first-order reflection off a wall segment. Uses image method to find reflection point.
func computeFirstOrderReflection(tx, rx, walker Point, walls []Wall) (float64, float64) {
	var bestReflAmp float64
	var bestReflPhase float64
	var foundReflection bool

	for _, wall := range walls {
		// Compute reflection point using image method
		reflPoint, ok := findReflectionPoint(tx, rx, wall)
		if !ok {
			continue
		}

		// Reflected path: TX -> reflection point -> RX
		dTxRefl := distance(tx, reflPoint)
		dReflRx := distance(reflPoint, rx)
		dReflTotal := dTxRefl + dReflRx

		// Direct path length (for comparison)
		dDirect := distance(tx, rx)

		// Log-distance path loss for reflected path
		var pathLossDB float64
		if dReflTotal >= d0 {
			pathLossDB = pl0 + 10.0*n*math.Log10(dReflTotal/d0)
		} else {
			pathLossDB = pl0
		}

		// Wall attenuation (transmission loss through wall if any)
		// Plus reflection loss (weakest material first = lowest attenuation)
		wallLoss := wall.Attenuation

		// Total loss in dB
		totalLossDB := pathLossDB + wallLoss

		// Convert to linear amplitude
		amplitude := math.Pow(10.0, -totalLossDB/20.0)

		// Apply reflection coefficient (power coefficient)
		amplitude *= math.Sqrt(reflectionCoeff)

		// Normalize by direct path loss (so reflected ray is relative to direct)
		directPathLoss := pl0 + 10.0*n*math.Log10(dDirect/d0)
		amplitude *= math.Pow(10.0, (directPathLoss-pathLossDB)/20.0)

		// Phase based on reflected path length
		phase := 2 * math.Pi * dReflTotal / wavelength

		// Keep the strongest reflection (lowest attenuation)
		if !foundReflection || amplitude > bestReflAmp {
			bestReflAmp = amplitude
			bestReflPhase = phase
			foundReflection = true
		}
	}

	if !foundReflection {
		return 0, 0
	}

	return bestReflAmp, bestReflPhase
}

// findReflectionPoint finds the specular reflection point on a wall segment
// for a ray from TX to RX using the image method.
// Returns the reflection point and true if a valid reflection exists, false otherwise.
func findReflectionPoint(tx, rx Point, wall Wall) (Point, bool) {
	// For a vertical wall segment (in 2D floor plane), the reflection point
	// is found by reflecting the TX across the wall line and finding the
	// intersection with the wall segment.

	// Wall line equation: ax + by + c = 0
	// For vertical wall from (x1, y1) to (x2, y2):
	// If x1 == x2 (vertical wall), reflection is straightforward
	if math.Abs(wall.X1-wall.X2) < 1e-6 {
		// Vertical wall at x = wall.X1
		// Reflect TX across the wall
		reflTxX := 2*wall.X1 - tx.X
		reflTxY := tx.Y

		// Find intersection of line from reflTx to RX with the wall
		// Parametric line: reflTx + t*(rx - reflTx)
		dx := rx.X - reflTxX
		dy := rx.Y - reflTxY

		if math.Abs(dx) < 1e-6 {
			// Line is vertical, no intersection with vertical wall (or parallel)
			return Point{}, false
		}

		t := (wall.X1 - reflTxX) / dx

		// Compute intersection point
		intersectY := reflTxY + t*dy

		// Check if intersection is within wall segment bounds
		minY := math.Min(wall.Y1, wall.Y2)
		maxY := math.Max(wall.Y1, wall.Y2)

		if intersectY < minY || intersectY > maxY {
			return Point{}, false
		}

		// Z coordinate is average of TX and RX Z (reflection in vertical plane)
		intersectZ := (tx.Z + rx.Z) / 2.0

		return Point{X: wall.X1, Y: intersectY, Z: intersectZ}, true
	}

	// For horizontal wall segment (y1 == y2)
	if math.Abs(wall.Y1-wall.Y2) < 1e-6 {
		// Horizontal wall at y = wall.Y1
		// Reflect TX across the wall
		reflTxX := tx.X
		reflTxY := 2*wall.Y1 - tx.Y

		// Find intersection of line from reflTx to RX with the wall
		dx := rx.X - reflTxX
		dy := rx.Y - reflTxY

		if math.Abs(dy) < 1e-6 {
			// Line is horizontal, no intersection
			return Point{}, false
		}

		t := (wall.Y1 - reflTxY) / dy

		// Compute intersection point
		intersectX := reflTxX + t*dx

		// Check if intersection is within wall segment bounds
		minX := math.Min(wall.X1, wall.X2)
		maxX := math.Max(wall.X1, wall.X2)

		if intersectX < minX || intersectX > maxX {
			return Point{}, false
		}

		// Z coordinate is average of TX and RX Z
		intersectZ := (tx.Z + rx.Z) / 2.0

		return Point{X: intersectX, Y: wall.Y1, Z: intersectZ}, true
	}

	// General case: angled wall segment
	// Compute line intersection
	// Wall line: (x1, y1) to (x2, y2)
	// Ray from reflected TX to RX

	// Reflect TX across the wall line
	reflTx := reflectPointAcrossLine(tx, Point{X: wall.X1, Y: wall.Y1, Z: 0}, Point{X: wall.X2, Y: wall.Y2, Z: 0})

	// Find intersection of line from reflTx to RX with the wall line
	intersect, ok := lineIntersection(
		reflTx, rx,
		Point{X: wall.X1, Y: wall.Y1, Z: 0}, Point{X: wall.X2, Y: wall.Y2, Z: 0},
	)
	if !ok {
		return Point{}, false
	}

	// Check if intersection is within wall segment bounds
	minX := math.Min(wall.X1, wall.X2)
	maxX := math.Max(wall.X1, wall.X2)
	minY := math.Min(wall.Y1, wall.Y2)
	maxY := math.Max(wall.Y1, wall.Y2)

	if intersect.X < minX || intersect.X > maxX || intersect.Y < minY || intersect.Y > maxY {
		return Point{}, false
	}

	// Z coordinate is average of TX and RX Z
	intersect.Z = (tx.Z + rx.Z) / 2.0

	return intersect, true
}

// reflectPointAcrossLine reflects a point across a line defined by two points.
// Uses the formula for reflection of a point across a line in 2D.
func reflectPointAcrossLine(p, lineStart, lineEnd Point) Point {
	// Line direction vector
	lx := lineEnd.X - lineStart.X
	ly := lineEnd.Y - lineStart.Y

	// Vector from line start to point
	px := p.X - lineStart.X
	py := p.Y - lineStart.Y

	// Project p onto the line (dot product)
	dot := px*lx + py*ly
	lenSq := lx*lx + ly*ly

	if lenSq < 1e-10 {
		// Line segment is too short
		return p
	}

	// Projection parameter
	t := dot / lenSq

	// Closest point on line
	closestX := lineStart.X + t*lx
	closestY := lineStart.Y + t*ly

	// Reflected point: p' = closest + (closest - p) = 2*closest - p
	return Point{
		X: 2*closestX - p.X,
		Y: 2*closestY - p.Y,
		Z: p.Z,
	}
}

// lineIntersection finds the intersection point of two line segments in 2D.
// Returns the intersection point and true if the lines intersect, false otherwise.
func lineIntersection(p1, p2, p3, p4 Point) (Point, bool) {
	// Line 1: p1 to p2
	// Line 2: p3 to p4

	x1, y1 := p1.X, p1.Y
	x2, y2 := p2.X, p2.Y
	x3, y3 := p3.X, p3.Y
	x4, y4 := p4.X, p4.Y

	// Compute denominator
	denom := (x1-x2)*(y3-y4) - (y1-y2)*(x3-x4)
	if math.Abs(denom) < 1e-10 {
		// Lines are parallel
		return Point{}, false
	}

	// Compute intersection point using parametric form
	t := ((x1-x3)*(y3-y4) - (y1-y3)*(x3-x4)) / denom
	u := -((x1-x2)*(y1-y3) - (y1-y2)*(x1-x3)) / denom

	// Check if intersection is within both line segments
	if t < 0 || t > 1 || u < 0 || u > 1 {
		return Point{}, false
	}

	// Compute intersection point
	intersectX := x1 + t*(x2-x1)
	intersectY := y1 + t*(y2-y1)

	return Point{X: intersectX, Y: intersectY}, true
}

// distance computes Euclidean distance between two points
func distance(a, b Point) float64 {
	dx := a.X - b.X
	dy := a.Y - b.Y
	dz := a.Z - b.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// computeWallLoss computes wall attenuation for a path through all walls.
// For each wall the path intersects, add the wall's attenuation value.
func computeWallLoss(from, to Point, walls []Wall) float64 {
	totalLoss := 0.0

	for _, wall := range walls {
		if pathIntersectsWall(from.X, from.Y, to.X, to.Y, wall.X1, wall.Y1, wall.X2, wall.Y2) {
			totalLoss += wall.Attenuation
		}
	}

	return totalLoss
}

// pathIntersectsWall checks if a path intersects a wall segment (2D)
func pathIntersectsWall(x1, y1, x2, y2, wx1, wy1, wx2, wy2 float64) bool {
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

// amplitudeToRSSI converts amplitude to RSSI in dBm
func amplitudeToRSSI(amplitude float64) int8 {
	// Convert amplitude to dBm (reference: amplitude 1.0 = -30 dBm)
	amplitudeDBm := -30.0 + 20.0*math.Log10(amplitude)

	// Clamp to realistic range
	if amplitudeDBm < -90 {
		amplitudeDBm = -90
	}
	if amplitudeDBm > -30 {
		amplitudeDBm = -30
	}

	return int8(amplitudeDBm)
}

// generateIQPair generates a synthetic I/Q pair with Gaussian noise
func generateIQPair(amplitude, phase float64, rng *rand.Rand) (int8, int8) {
	// Box-Muller transform for Gaussian noise
	u1 := rng.Float64()
	u2 := rng.Float64()
	z0 := math.Sqrt(-2.0*math.Log(u1)) * math.Cos(2.0*math.Pi*u2)
	z1 := math.Sqrt(-2.0*math.Log(u1)) * math.Sin(2.0*math.Pi*u2)

	noiseI := z0 * *flagNoiseSigma
	noiseQ := z1 * *flagNoiseSigma

	// Convert to I/Q
	i := amplitude*math.Cos(phase) + noiseI
	q := amplitude*math.Sin(phase) + noiseQ

	// Scale to int8 range
	scale := 127.0 / 10.0 // Scale factor
	i *= scale
	q *= scale

	// Clamp to int8 range [-127, 127]
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
