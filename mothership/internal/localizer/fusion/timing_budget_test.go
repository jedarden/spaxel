// Package fusion provides timing benchmarks for the fusion pipeline.
// This benchmark enforces the fusion loop timing budget as a CI quality gate,
// per plan §Quality Gates / Definition of Done (item 9).
//
// The benchmark runs the full fusion pipeline:
//   1. Phase sanitization → Feature extraction → Fresnel accumulation → Peak extraction → UKF update
//   against synthetic CSI data from spaxel-sim output.
//
// Asserts:
//   - Median fusion iteration < 15 ms over 600 iterations (60 seconds at 10 Hz)
//   - P99 < 40 ms (hard limit)
//
// CI integration:
//   Add to Argo Workflows CI step after go test ./...:
//     go test -bench=BenchmarkFusionLoop -benchtime=60s -count=1 ./internal/localizer/fusion/
//
//   Acceptance: Workflow fails if median latency exceeds 30 ms on CI runner
//               (2x allowance for slower hardware; 15 ms production target)
package fusion

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"testing"
	"time"

	"github.com/spaxel/mothership/internal/fusion"
	"github.com/spaxel/mothership/internal/signal"
	"github.com/spaxel/mothership/internal/tracking"
)

const (
	// WiFi physical constants (matching spaxel-sim)
	wavelength        = 0.123     // meters (2.4 GHz)
	halfWavelength    = wavelength / 2.0
	nSub              = 64        // number of subcarriers for HT20
	headerSize        = 24        // CSI frame header size
	fusionRate        = 10        // Hz (fusion loop rate)
	fusionIterations  = 600       // Number of iterations for timing (60s at 10Hz)
	productionTarget  = 15 * time.Millisecond // Production target (per iteration)
	ciThreshold       = 30 * time.Millisecond // CI threshold (2x allowance)
	hardLimit         = 40 * time.Millisecond // P99 hard limit
)

// CSIFrame represents a synthetic CSI frame matching spaxel-sim output format
type CSIFrame struct {
	NodeMAC    [6]byte
	PeerMAC    [6]byte
	Timestamp  uint64
	RSSI       int8
	NoiseFloor int8
	Channel    uint8
	NSub       uint8
	IQ         []int8 // Interleaved I,Q pairs
}

// Walker represents a synthetic person for CSI generation
type Walker struct {
	ID       int
	Position Point
	Velocity Point
}

// Point represents a 3D position
type Point struct {
	X, Y, Z float64
}

// VirtualNode represents a simulated ESP32 node
type VirtualNode struct {
	ID       int
	MAC      [6]byte
	Position Point
}

// durationSlice implements sort.Interface for []time.Duration
type durationSlice []time.Duration

func (d durationSlice) Len() int           { return len(d) }
func (d durationSlice) Less(i, j int) bool { return d[i] < d[j] }
func (d durationSlice) Swap(i, j int) { d[i], d[j] = d[j], d[i] }

// BenchmarkFusionLoop benchmarks the full fusion pipeline timing.
// It simulates 4 nodes with 2 walkers, running 600 fusion iterations (60s at 10Hz).
//
// The benchmark measures per-iteration latency and asserts:
//   - median < 15ms (production target)
//   - P99 < 40ms (hard limit)
//
// CI Threshold: 30ms median (2x allowance for slower CI hardware)
func BenchmarkFusionLoop(b *testing.B) {
	// Setup: create virtual nodes and walkers
	nodes := createVirtualNodes(4) // 4 nodes at corners of a 10x10m space
	walkers := createWalkers(2)    // 2 walkers

	// Create fusion engine
	engine := fusion.NewEngine(&fusion.Config{
		Width:        10,
		Height:       3,
		Depth:        10,
		CellSize:     0.2,
		MinDeltaRMS:  0.01,
		MaxBlobs:     6,
		BlobThreshold: 0.3,
	})

	// Set node positions
	for _, node := range nodes {
		engine.SetNodePosition(macToString(node.MAC), node.Position.X, node.Position.Y, node.Position.Z)
	}

	// Create signal processor for each link
	processorManager := signal.NewProcessorManager(signal.ProcessorManagerConfig{
		NSub:       nSub,
		FusionRate: float64(fusionRate),
		Tau:        30, // 30 second baseline time constant
	})

	// Create UKF tracker for each blob
	ukfTrackers := make(map[int]*tracking.UKF)
	nextBlobID := 1

	// Timing measurements
	timings := make([]time.Duration, 0, fusionIterations)

	// Reset benchmark timer - we want to measure just the fusion loop
	b.ResetTimer()

	// Run fusion iterations
	for i := 0; i < fusionIterations; i++ {
		start := time.Now()

		// Update walker positions (random walk)
		updateWalkers(walkers, 10.0, 10.0)

		// Generate and process CSI frames for all links
		linkMotions := make([]fusion.LinkMotion, 0)

		for _, tx := range nodes {
			for _, rx := range nodes {
				if tx.ID == rx.ID {
					continue
				}

				// Generate synthetic CSI frame
				frame := generateCSIFrame(tx, rx, walkers, i)

				// Convert to payload (int8 slice) - matching CSI frame format
				payload := make([]int8, headerSize+nSub*2)

				// Copy MAC addresses manually (convert []byte to []int8)
				for j := 0; j < 6; j++ {
					payload[j] = int8(frame.NodeMAC[j])
					payload[6+j] = int8(frame.PeerMAC[j])
				}

				// Timestamp (little-endian uint64)
				for j := 0; j < 8; j++ {
					payload[12+j] = int8((frame.Timestamp >> (j * 8)) & 0xFF)
				}

				payload[20] = frame.RSSI
				payload[21] = frame.NoiseFloor
				payload[22] = int8(frame.Channel)
				payload[23] = int8(frame.NSub)

				for k, iq := range frame.IQ {
					payload[headerSize+k] = iq
				}

				// Process CSI frame through signal pipeline
				linkID := fmt.Sprintf("%s:%s", macToString(frame.NodeMAC), macToString(frame.PeerMAC))
				result, err := processorManager.Process(linkID, payload, frame.RSSI, int(frame.NSub), time.Now())
				if err != nil {
					b.Fatalf("Process failed: %v", err)
				}

				// Add link motion if motion detected
				if result.Features != nil && result.Features.MotionDetected {
					linkMotions = append(linkMotions, fusion.LinkMotion{
						NodeMAC:    macToString(frame.NodeMAC),
						PeerMAC:    macToString(frame.PeerMAC),
						DeltaRMS:   result.Features.SmoothDeltaRMS,
						Motion:     true,
						HealthScore: 1.0, // Perfect health for synthetic data
					})
				}
			}
		}

		// Run fusion engine
		fusionResult := engine.Fuse(linkMotions)

		// Update UKF trackers for each detected blob
		for _, blob := range fusionResult.Blobs {
			ukf, exists := ukfTrackers[nextBlobID]
			if !exists {
				ukf = tracking.NewUKF(blob.X, blob.Z)
				ukfTrackers[nextBlobID] = ukf
			}

			// Predict
			ukf.Predict(1.0 / float64(fusionRate))

			// Update with measurement
			ukf.Update([2]float64{blob.X, blob.Z})
		}

		elapsed := time.Since(start)
		timings = append(timings, elapsed)
	}

	b.StopTimer()

	// Analyze timing statistics
	sort.Sort(durationSlice(timings))

	median := timings[len(timings)/2]
	p99Index := len(timings) * 99 / 100
	if p99Index >= len(timings) {
		p99Index = len(timings) - 1
	}
	p99 := timings[p99Index]

	// Report metrics
	b.ReportMetric(float64(median.Microseconds()), "ms/iter")
	b.ReportMetric(float64(p99.Microseconds()), "ms/p99")

	// Assert timing constraints
	// Note: benchmarks don't fail on assertion, so we log failures
	// The CI gate will check these values

	if median > ciThreshold {
		b.Logf("FAIL: Median fusion iteration %v exceeds CI threshold %v", median, ciThreshold)
		b.Fail()
	}

	if median > productionTarget {
		b.Logf("WARNING: Median fusion iteration %v exceeds production target %v", median, productionTarget)
	}

	if p99 > hardLimit {
		b.Logf("FAIL: P99 fusion iteration %v exceeds hard limit %v", p99, hardLimit)
		b.Fail()
	}

	b.Logf("Timing Results (n=%d):", len(timings))
	b.Logf("  Median: %v (target: %v, CI threshold: %v)", median, productionTarget, ciThreshold)
	b.Logf("  P99: %v (hard limit: %v)", p99, hardLimit)
	b.Logf("  Min: %v", timings[0])
	b.Logf("  Max: %v", timings[len(timings)-1])
}

// createVirtualNodes creates virtual nodes at corners of a space
func createVirtualNodes(count int) []*VirtualNode {
	nodes := make([]*VirtualNode, count)
	width, depth := 10.0, 10.0

	positions := []Point{
		{X: 0, Y: 1, Z: 0},
		{X: width, Y: 1, Z: 0},
		{X: width, Y: 1, Z: depth},
		{X: 0, Y: 1, Z: depth},
	}

	for i := 0; i < count; i++ {
		node := &VirtualNode{
			ID:  i,
			MAC: generateMAC(i),
		}
		if i < len(positions) {
			node.Position = positions[i]
		} else {
			node.Position = Point{
				X: float64(i) * width / float64(count),
				Y: 1,
				Z: depth / 2,
			}
		}
		nodes[i] = node
	}

	return nodes
}

// createWalkers creates synthetic walkers with random positions
func createWalkers(count int) []*Walker {
	walkers := make([]*Walker, count)
	rng := rand.New(rand.NewSource(42)) // Fixed seed for reproducibility

	for i := 0; i < count; i++ {
		walkers[i] = &Walker{
			ID: i,
			Position: Point{
				X: 2 + rng.Float64()*6, // Keep away from edges
				Y: 1.7,                // Person height
				Z: 2 + rng.Float64()*6,
			},
			Velocity: Point{
				X: (rng.Float64() - 0.5) * 0.5,
				Y: 0,
				Z: (rng.Float64() - 0.5) * 0.5,
			},
		}
	}

	return walkers
}

// updateWalkers updates walker positions with random walk
func updateWalkers(walkers []*Walker, width, depth float64) {
	dt := 1.0 / float64(fusionRate)

	for _, walker := range walkers {
		walker.Position.X += walker.Velocity.X * dt
		walker.Position.Z += walker.Velocity.Z * dt

		// Bounce off walls
		margin := 0.5
		if walker.Position.X < margin {
			walker.Position.X = margin
			walker.Velocity.X *= -1
		}
		if walker.Position.X > width-margin {
			walker.Position.X = width - margin
			walker.Velocity.X *= -1
		}
		if walker.Position.Z < margin {
			walker.Position.Z = margin
			walker.Velocity.Z *= -1
		}
		if walker.Position.Z > depth-margin {
			walker.Position.Z = depth - margin
			walker.Velocity.Z *= -1
		}

		// Random velocity perturbation (small)
		perturbation := 0.05
		walker.Velocity.X += (rand.Float64() - 0.5) * perturbation
		walker.Velocity.Z += (rand.Float64() - 0.5) * perturbation

		// Clamp velocity
		speed := math.Sqrt(walker.Velocity.X*walker.Velocity.X + walker.Velocity.Z*walker.Velocity.Z)
		maxSpeed := 1.0 // m/s
		if speed > maxSpeed {
			scale := maxSpeed / speed
			walker.Velocity.X *= scale
			walker.Velocity.Z *= scale
		}
	}
}

// generateCSIFrame generates a synthetic CSI frame similar to spaxel-sim
func generateCSIFrame(tx, rx *VirtualNode, walkers []*Walker, frameNum int) *CSIFrame {
	// Calculate combined CSI from all walkers
	amplitude, phaseBase := computeCSIForWalkers(tx, rx, walkers)

	// Compute RSSI from amplitude
	rssi := amplitudeToRSSI(amplitude)

	// Create frame
	frame := &CSIFrame{
		NodeMAC:    tx.MAC,
		PeerMAC:    rx.MAC,
		Timestamp:  uint64(frameNum * 50000), // 50us intervals
		RSSI:       rssi,
		NoiseFloor: -95,
		Channel:    6,
		NSub:       nSub,
		IQ:         make([]int8, nSub*2),
	}

	// Generate I/Q pairs for each subcarrier
	for k := 0; k < nSub; k++ {
		phase := phaseBase + float64(k)*0.1
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
		i, q := generateIQPair(subAmplitude, phase)

		frame.IQ[k*2] = i
		frame.IQ[k*2+1] = q
	}

	return frame
}

// computeCSIForWalkers computes combined CSI amplitude and phase from all walkers
func computeCSIForWalkers(tx, rx *VirtualNode, walkers []*Walker) (float64, float64) {
	if len(walkers) == 0 {
		return 0.001, 0.0
	}

	var totalAmplitude float64
	var totalPhase float64
	var weight float64

	for _, walker := range walkers {
		d1 := distance(tx.Position, walker.Position)
		d2 := distance(walker.Position, rx.Position)
		dDirect := distance(tx.Position, rx.Position)

		excess := d1 + d2 - dDirect
		if excess < 0 {
			excess = 0
		}

		zoneNumber := int(math.Ceil(excess / halfWavelength))
		if zoneNumber < 1 {
			zoneNumber = 1
		}

		decay := 1.0 / math.Pow(float64(zoneNumber), 2.0)
		pathLoss := 40.0 + 20.0*math.Log10(d1+d2)

		totalLossDB := pathLoss
		amplitude := math.Pow(10.0, -totalLossDB/20.0)
		amplitude *= 1000.0 * decay

		phase := 2 * math.Pi * (d1+d2) / wavelength

		totalAmplitude += amplitude
		totalPhase += phase * decay
		weight += decay
	}

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

// amplitudeToRSSI converts amplitude to RSSI in dBm
func amplitudeToRSSI(amplitude float64) int8 {
	amplitudeDBm := -30.0 + 20.0*math.Log10(amplitude)

	if amplitudeDBm < -90 {
		amplitudeDBm = -90
	}
	if amplitudeDBm > -30 {
		amplitudeDBm = -30
	}

	return int8(amplitudeDBm)
}

// generateIQPair generates a synthetic I/Q pair with minimal noise
func generateIQPair(amplitude, phase float64) (int8, int8) {
	// Minimal noise for deterministic benchmark
	noiseLevel := 0.01
	i := amplitude*math.Cos(phase) + (rand.Float64()-0.5)*noiseLevel
	q := amplitude*math.Sin(phase) + (rand.Float64()-0.5)*noiseLevel

	scale := 127.0 / 10.0
	i *= scale
	q *= scale

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

// generateMAC generates a synthetic MAC address
func generateMAC(id int) [6]byte {
	var mac [6]byte
	mac[0] = 0xAA
	mac[1] = 0xBB
	mac[2] = 0xCC
	mac[3] = byte((id >> 16) & 0xFF)
	mac[4] = byte((id >> 8) & 0xFF)
	mac[5] = byte(id & 0xFF)
	return mac
}

// macToString converts a 6-byte MAC to colon-separated hex
func macToString(mac [6]byte) string {
	return fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
		mac[0], mac[1], mac[2], mac[3], mac[4], mac[5])
}

// TestTimingBudgetProduction verifies the timing budget meets production targets
func TestTimingBudgetProduction(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping timing test in short mode")
	}

	nodes := createVirtualNodes(4)
	walkers := createWalkers(2)

	engine := fusion.NewEngine(&fusion.Config{
		Width:        10,
		Height:       3,
		Depth:        10,
		CellSize:     0.2,
		MinDeltaRMS:  0.01,
		MaxBlobs:     6,
		BlobThreshold: 0.3,
	})

	for _, node := range nodes {
		engine.SetNodePosition(macToString(node.MAC), node.Position.X, node.Position.Y, node.Position.Z)
	}

	processorManager := signal.NewProcessorManager(signal.ProcessorManagerConfig{
		NSub:       nSub,
		FusionRate: float64(fusionRate),
		Tau:        30,
	})

	ukfTrackers := make(map[int]*tracking.UKF)
	nextBlobID := 1

	timings := make([]time.Duration, 0, fusionIterations)

	for i := 0; i < fusionIterations; i++ {
		start := time.Now()

		updateWalkers(walkers, 10.0, 10.0)

		linkMotions := make([]fusion.LinkMotion, 0)

		for _, tx := range nodes {
			for _, rx := range nodes {
				if tx.ID == rx.ID {
					continue
				}

				frame := generateCSIFrame(tx, rx, walkers, i)
				payload := make([]int8, headerSize+nSub*2)

				// Copy MAC addresses manually
				for j := 0; j < 6; j++ {
					payload[j] = int8(frame.NodeMAC[j])
					payload[6+j] = int8(frame.PeerMAC[j])
				}

				// Timestamp (little-endian uint64)
				for j := 0; j < 8; j++ {
					payload[12+j] = int8((frame.Timestamp >> (j * 8)) & 0xFF)
				}

				payload[20] = frame.RSSI
				payload[21] = frame.NoiseFloor
				payload[22] = int8(frame.Channel)
				payload[23] = int8(frame.NSub)

				for k, iq := range frame.IQ {
					payload[headerSize+k] = iq
				}

				linkID := fmt.Sprintf("%s:%s", macToString(frame.NodeMAC), macToString(frame.PeerMAC))
				result, err := processorManager.Process(linkID, payload, frame.RSSI, int(frame.NSub), time.Now())
				if err != nil {
					t.Fatalf("Process failed: %v", err)
				}

				if result.Features != nil && result.Features.MotionDetected {
					linkMotions = append(linkMotions, fusion.LinkMotion{
						NodeMAC:     macToString(frame.NodeMAC),
						PeerMAC:     macToString(frame.PeerMAC),
						DeltaRMS:    result.Features.SmoothDeltaRMS,
						Motion:      true,
						HealthScore: 1.0,
					})
				}
			}
		}

		fusionResult := engine.Fuse(linkMotions)

		for _, blob := range fusionResult.Blobs {
			ukf, exists := ukfTrackers[nextBlobID]
			if !exists {
				ukf = tracking.NewUKF(blob.X, blob.Z)
				ukfTrackers[nextBlobID] = ukf
			}

			ukf.Predict(1.0 / float64(fusionRate))
			ukf.Update([2]float64{blob.X, blob.Z})
		}

		elapsed := time.Since(start)
		timings = append(timings, elapsed)
	}

	// Sort and compute statistics
	sort.Sort(durationSlice(timings))

	median := timings[len(timings)/2]
	p99Index := len(timings) * 99 / 100
	if p99Index >= len(timings) {
		p99Index = len(timings) - 1
	}
	p99 := timings[p99Index]

	// Log statistics
	t.Logf("Timing Results (n=%d):", len(timings))
	t.Logf("  Min: %v", timings[0])
	t.Logf("  Median: %v (target: %v, CI threshold: %v)", median, productionTarget, ciThreshold)
	t.Logf("  P99: %v (hard limit: %v)", p99, hardLimit)
	t.Logf("  Max: %v", timings[len(timings)-1])

	// Assert timing constraints
	if median > productionTarget {
		t.Errorf("Median fusion iteration %v exceeds production target %v", median, productionTarget)
	}

	if median > ciThreshold {
		t.Errorf("Median fusion iteration %v exceeds CI threshold %v", median, ciThreshold)
	}

	if p99 > hardLimit {
		t.Errorf("P99 fusion iteration %v exceeds hard limit %v", p99, hardLimit)
	}
}
