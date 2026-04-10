package simulator

import (
	"math"
	mrand "math/rand"
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

// CSIData represents synthetic CSI data matching the WebSocket binary frame format
type CSIData struct {
	NodeMAC     []byte    // 6 bytes
	PeerMAC     []byte    // 6 bytes
	TimestampUs uint64    // microseconds since boot
	RSSI        int8      // dBm
	NoiseFloor  int8      // dBm
	Channel     uint8     // WiFi channel
	NSub        uint8     // Number of subcarriers
	Subcarriers []Complex // I/Q pairs for each subcarrier
}

// Complex represents I/Q complex numbers
type Complex struct {
	I int8 // In-phase
	Q int8 // Quadrature
}

// GenerateCSIFrame generates a synthetic CSI frame matching the binary WebSocket format
// with realistic characteristics including temporal variations and noise
func (pm *PropagationModel) GenerateCSIFrame(tx, rx, walker Point, frameNum int) CSIData {
	// Number of subcarriers for HT20 (64 total, but we simulate all)
	nSub := uint8(64)

	// Compute base amplitude at walker position
	baseAmplitude := pm.AmplitudeAt(tx, rx, walker)

	// Convert to dBm reference
	// At 1m with -30dBm reference: amplitude 1.0 = -30dBm
	amplitudeDBm := -30.0 + 20.0*math.Log10(baseAmplitude)

	// Add realistic temporal variations (small-scale fading)
	// Simulate Rayleigh fading with time correlation
	fading := pm.computeTemporalFading(frameNum)
	amplitudeDBm += fading

	// Clamp to realistic range
	if amplitudeDBm > -20 {
		amplitudeDBm = -20
	}
	if amplitudeDBm < -90 {
		amplitudeDBm = -90
	}

	// Generate per-subcarrier CSI with realistic characteristics
	subcarriers := make([]Complex, nSub)
	for k := 0; k < int(nSub); k++ {
		// Compute phase at this subcarrier
		phase := pm.PhaseAt(tx, rx, walker, k)

		// Add subcarrier-dependent amplitude variation (frequency selectivity)
		// Simulate frequency-selective fading with sinusoidal variation
	freqFading := 0.8 + 0.4*math.Sin(2*math.Pi*float64(k)/16.0)
	amplitude := math.Pow(10.0, (amplitudeDBm+30)/20.0) * freqFading

		// Convert to int8 I/Q (range -128 to 127)
		amplitude = amplitude / 1000.0 // Scale to reasonable int8 range
		if amplitude > 1.0 {
			amplitude = 1.0
		}

		subcarriers[k] = Complex{
			I: int8(amplitude*math.Cos(phase) * 127),
			Q: int8(amplitude*math.Sin(phase) * 127),
		}

		// Add noise
		subcarriers[k].I += int8((mrand.Float64() - 0.5) * 20)
		subcarriers[k].Q += int8((mrand.Float64() - 0.5) * 20)
	}

	// Generate MAC addresses (simplified)
	nodeMAC := []byte{0xAA, 0xBB, 0xCC, 0x00, 0x01, 0x00}
	peerMAC := []byte{0xAA, 0xBB, 0xCC, 0x00, 0x02, 0x00}

	// RSSI from amplitude (clipped to int8 range)
	rssi := int8(amplitudeDBm)
	if rssi < -90 {
		rssi = -90
	}
	if rssi > -30 {
		rssi = -30
	}

	return CSIData{
		NodeMAC:     nodeMAC,
		PeerMAC:     peerMAC,
		TimestampUs: uint64(frameNum * 50000), // 50ms intervals at 20Hz
		RSSI:        rssi,
		NoiseFloor:  -95, // Typical noise floor
		Channel:     6,   // Default channel 6
		NSub:        nSub,
		Subcarriers: subcarriers,
	}
}

// computeTemporalFading computes small-scale temporal fading variation
// Simulates Rayleigh fading with temporal correlation
func (pm *PropagationModel) computeTemporalFading(frameNum int) float64 {
	// Use a simple sinusoidal model to simulate fading variation
	// Real fading would be more complex with multiple paths
	// This provides temporal correlation between consecutive frames

	// Fading period: ~100 frames (5 seconds at 20Hz)
	fadingPeriod := 100.0
	// Fading depth: ±3 dB
	fadingDepth := 3.0

	return fadingDepth * math.Sin(2*math.Pi*float64(frameNum)/fadingPeriod)
}

// GenerateCSIFrames generates a sequence of CSI frames for a link
// Useful for time-series simulation and testing
func (pm *PropagationModel) GenerateCSIFrames(link Link, walker Point, numFrames int, rateHz int) []CSIData {
	frames := make([]CSIData, numFrames)
	intervalUs := uint64(1000000 / rateHz)

	for i := 0; i < numFrames; i++ {
		frame := pm.GenerateCSIFrame(
			link.TX.Position,
			link.RX.Position,
			walker,
			i,
		)
		frame.TimestampUs = uint64(i) * intervalUs
		frames[i] = frame
	}

	return frames
}

// SimulatedLinkMetrics represents metrics for a simulated link
type SimulatedLinkMetrics struct {
	AvgRSSI        float64 // Average RSSI in dBm
	RSSIStdDev     float64 // RSSI standard deviation
	AvgDeltaRMS    float64 // Average deltaRMS
	PacketDelivery float64 // Packet delivery rate (0-1)
	LinkQuality    float64 // Overall link quality (0-1)
}

// ComputeLinkMetrics computes realistic link metrics over a simulation run
func (pm *PropagationModel) ComputeLinkMetrics(link Link, walkerPositions []Point, numSamples int) SimulatedLinkMetrics {
	if len(walkerPositions) == 0 {
		walkerPositions = []Point{{X: 0, Y: 0, Z: 1.7}} // Default position
	}
	if numSamples == 0 {
		numSamples = len(walkerPositions)
	}

	// Sample RSSI values
	rssiValues := make([]float64, numSamples)
	deltaRMSValues := make([]float64, numSamples)
	receivedCount := 0

	for i := 0; i < numSamples; i++ {
		// Cycle through walker positions
		pos := walkerPositions[i%len(walkerPositions)]

		// Compute RSSI at this position
		amplitude := pm.AmplitudeAt(link.TX.Position, link.RX.Position, pos)
		rssiDBm := -30.0 + 20.0*math.Log10(amplitude)

		// Add fading variation
		rssiDBm += pm.computeTemporalFading(i)

		// Clamp to realistic range
		if rssiDBm < -90 {
			rssiDBm = -90
		}
		if rssiDBm > -20 {
			rssiDBm = -20
		}

		rssiValues[i] = rssiDBm

		// Compute deltaRMS (change from baseline)
		baselineAmplitude := pm.AmplitudeAt(link.TX.Position, link.RX.Position, Point{X: -1000, Y: -1000, Z: 0})
		deltaRMS := math.Abs(amplitude-baselineAmplitude) / baselineAmplitude
		deltaRMSValues[i] = deltaRMS

		// Simulate packet loss based on RSSI
		// Typical WiFi: packet loss increases below -80 dBm
		if rssiDBm > -80 {
			receivedCount++
		} else if rssiDBm > -90 && mrand.Float64() > 0.5 {
			receivedCount++
		}
	}

	// Compute statistics
	avgRSSI := 0.0
	for _, v := range rssiValues {
		avgRSSI += v
	}
	avgRSSI /= float64(numSamples)

	variance := 0.0
	for _, v := range rssiValues {
		diff := v - avgRSSI
		variance += diff * diff
	}
	rssiStdDev := math.Sqrt(variance / float64(numSamples))

	avgDeltaRMS := 0.0
	for _, v := range deltaRMSValues {
		avgDeltaRMS += v
	}
	avgDeltaRMS /= float64(numSamples)

	pdr := float64(receivedCount) / float64(numSamples)

	// Link quality: combines RSSI, PDR, and deltaRMS
	// Higher RSSI = better, higher PDR = better, higher deltaRMS = better
	rssiScore := (avgRSSI + 90) / 70.0 // Map -90..-20 to 0..1
	if rssiScore < 0 {
		rssiScore = 0
	}
	if rssiScore > 1 {
		rssiScore = 1
	}

	// DeltaRMS score: values > 0.05 are good
	deltaRMSScore := math.Min(avgDeltaRMS/0.1, 1.0)

	linkQuality := 0.5*rssiScore + 0.3*pdr + 0.2*deltaRMSScore

	return SimulatedLinkMetrics{
		AvgRSSI:        avgRSSI,
		RSSIStdDev:     rssiStdDev,
		AvgDeltaRMS:    avgDeltaRMS,
		PacketDelivery: pdr,
		LinkQuality:    linkQuality,
	}
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

