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
func computeCSIForWalkers(tx, rx *VirtualNode, walkers []*Walker, walls []Wall) (float64, float64) {
	if len(walkers) == 0 {
		// No walkers, return baseline noise
		return 0.001, 0.0
	}

	var totalAmplitude float64
	var totalPhase float64
	var weight float64

	for _, walker := range walkers {
		// Distance from TX to walker
		d1 := distance(tx.Position, walker.Position)
		// Distance from walker to RX
		d2 := distance(walker.Position, rx.Position)
		// Direct TX-RX distance
		dDirect := distance(tx.Position, rx.Position)

		// Path length excess for Fresnel zone calculation
		excess := d1 + d2 - dDirect
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

		// Path loss
		pathLoss := 40.0 + 20.0*math.Log10(d1+d2)

		// Wall attenuation
		wallLoss := computeWallLoss(tx.Position, walker.Position, walls)
		wallLoss += computeWallLoss(walker.Position, rx.Position, walls)

		// Total loss in dB
		totalLossDB := pathLoss + wallLoss

		// Convert to linear amplitude
		amplitude := math.Pow(10.0, -totalLossDB/20.0)

		// Scale to reasonable values
		amplitude *= 1000.0 * decay

		// Phase at this position
		phase := 2 * math.Pi * (d1+d2) / wavelength

		// Accumulate (incoherent sum for amplitude, weighted average for phase)
		totalAmplitude += amplitude
		totalPhase += phase * decay
		weight += decay
	}

	// Normalize
	if weight > 0 {
		totalPhase /= weight
	}

	return totalAmplitude, totalPhase
}

// distance computes Euclidean distance between two points
func distance(a, b Point) float64 {
	dx := a.X - b.X
	dy := a.Y - b.Y
	dz := a.Z - b.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// computeWallLoss computes wall attenuation for a path
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
