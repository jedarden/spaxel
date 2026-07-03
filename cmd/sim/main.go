// Command spaxel-sim is a CSI simulator CLI for testing Spaxel without hardware.
// It connects to a running mothership via WebSocket and streams synthetic CSI data.
package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// CSI frame header size (24 bytes) — matches ingestion/frame.go
	headerSize = 24

	// Default values
	defaultMothership = "ws://localhost:8080/ws/node"
	defaultNodes      = 4
	defaultWalkers    = 1
	defaultRate       = 20  // Hz
	defaultDuration   = 60  // seconds
	defaultChannel    = 6   // 2.4 GHz channel 6
	defaultSeed       = 42  // random seed
	defaultSpace      = "6x5x2.5" // room dimensions

	// WiFi physical constants
	wavelength     = 0.123  // meters (2.4 GHz)
	halfWavelength = wavelength / 2.0
	nSub           = 64 // number of subcarriers for HT20

	// Path loss model constants
	pl0 = 40.0 // dBm reference power at d0=1m
	n   = 2.0  // path loss exponent (free space)
)

var (
	// CLI flags
	flagMothership = flag.String("mothership", defaultMothership, "URL of the mothership WebSocket endpoint")
	flagToken      = flag.String("token", "", "Provisioning token (auto-generated if empty)")
	flagNodes      = flag.Int("nodes", defaultNodes, "Number of virtual nodes")
	flagWalkers    = flag.Int("walkers", defaultWalkers, "Number of synthetic walkers")
	flagRate       = flag.Int("rate", defaultRate, "CSI transmission rate in Hz per node pair")
	flagDuration   = flag.Int("duration", defaultDuration, "Total run time in seconds (0 = run forever)")
	flagBLE        = flag.Bool("ble", false, "Include simulated BLE advertisements")
	flagSeed       = flag.Int64("seed", defaultSeed, "Random seed for reproducible runs")
	flagSpace      = flag.String("space", defaultSpace, "Room dimensions in WxDxH format (meters)")
)

// VirtualNode represents a simulated ESP32 node
type VirtualNode struct {
	ID       int
	MAC      [6]byte
	Position Point
	Conn     *websocket.Conn
	mu       sync.Mutex
}

// Walker represents a simulated person
type Walker struct {
	ID       int
	Position Point
	Velocity Point
}

// Point represents a 3D position
type Point struct {
	X, Y, Z float64
}

// Space represents the room dimensions
type Space struct {
	Width, Depth, Height float64
}

// Stats tracks simulation statistics
type Stats struct {
	FramesSent     atomic.Int64
	FramesPerSec   float64
	StartTime      time.Time
	LastStatsTime  time.Time
	LastFramesSent int64
	BlobCount      int
	Rejected       atomic.Bool // Set to true when any node is rejected
}

var stats Stats

func main() {
	flag.Parse()

	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Printf("[SIM] CSI Simulator starting")

	// Parse space dimensions
	space, err := parseSpace(*flagSpace)
	if err != nil {
		log.Fatalf("[SIM] Invalid space dimensions: %v", err)
	}

	// Initialize random seed
	rng := rand.New(rand.NewSource(*flagSeed))
	log.Printf("[SIM] Random seed: %d", *flagSeed)

	// Generate or validate token
	token := *flagToken
	if token == "" {
		// For testing, generate a dummy token
		// In production, this should be derived from the install secret
		token = fmt.Sprintf("%064x", rng.Uint64())
		log.Printf("[SIM] Auto-generated token (first 16 chars): %s...", token[:16])
	}

	// Create virtual nodes at fixed positions (corners, evenly distributed)
	nodes := createVirtualNodes(*flagNodes, space, rng)

	// Create walkers with random walk behavior
	walkers := createWalkers(*flagWalkers, space, rng)

	log.Printf("[SIM] Configuration:")
	log.Printf("[SIM]   Mothership: %s", *flagMothership)
	log.Printf("[SIM]   Nodes: %d", *flagNodes)
	log.Printf("[SIM]   Walkers: %d", *flagWalkers)
	log.Printf("[SIM]   Rate: %d Hz", *flagRate)
	log.Printf("[SIM]   Duration: %d s", *flagDuration)
	log.Printf("[SIM]   Space: %.1fx%.1fx%.1f m", space.Width, space.Depth, space.Height)
	log.Printf("[SIM]   BLE: %v", *flagBLE)

	// Create context for shutdown
	ctx, cancel := contextWithCancel()
	defer cancel()

	// Channel for reject notifications
	rejectChan := make(chan struct{}, len(nodes))

	// Connect all nodes to mothership
	if err := connectNodes(ctx, nodes, token, rng, rejectChan); err != nil {
		log.Fatalf("[SIM] Failed to connect nodes: %v", err)
	}

	// Start blob count polling
	go pollBlobCount()

	// Start stats reporting
	go reportStats()

	// Start simulation (monitor for reject)
	runSimulation(ctx, nodes, walkers, space, rng, rejectChan)

	// Shutdown
	log.Printf("[SIM] Shutting down...")
	for _, node := range nodes {
		node.mu.Lock()
		if node.Conn != nil {
			node.Conn.Close()
		}
		node.mu.Unlock()
	}

	// Print final statistics
	printFinalStats()

	// Exit non-zero if rejected
	if stats.Rejected.Load() {
		log.Printf("[SIM] Exiting due to rejection")
		os.Exit(1)
	}
}

// parseSpace parses space dimensions from "WxDxH" format
func parseSpace(s string) (*Space, error) {
	var w, d, h float64
	_, err := fmt.Sscanf(s, "%fx%fx%f", &w, &d, &h)
	if err != nil {
		return nil, fmt.Errorf("invalid format (expected WxDxH): %w", err)
	}
	if w <= 0 || d <= 0 || h <= 0 {
		return nil, fmt.Errorf("dimensions must be positive")
	}
	return &Space{Width: w, Depth: d, Height: h}, nil
}

// createVirtualNodes creates virtual nodes at corners, evenly distributed
func createVirtualNodes(count int, space *Space, rng *rand.Rand) []*VirtualNode {
	nodes := make([]*VirtualNode, count)

	// Position nodes at corners and midpoints
	positions := generateNodePositions(count, space)

	for i := 0; i < count; i++ {
		mac := generateMAC(i)
		nodes[i] = &VirtualNode{
			ID:       i,
			MAC:      mac,
			Position: positions[i],
		}
		log.Printf("[SIM] Node %d: MAC=%s pos=(%.2f,%.2f,%.2f)",
			i, macToString(mac), positions[i].X, positions[i].Y, positions[i].Z)
	}

	return nodes
}

// generateNodePositions generates positions for nodes evenly distributed in the space
func generateNodePositions(count int, space *Space) []Point {
	positions := make([]Point, count)

	// For small counts, use corners
	// For larger counts, distribute evenly
	if count == 1 {
		positions[0] = Point{X: space.Width / 2, Y: space.Depth / 2, Z: space.Height / 2}
	} else if count == 2 {
		positions[0] = Point{X: 0, Y: 0, Z: space.Height}
		positions[1] = Point{X: space.Width, Y: space.Depth, Z: space.Height}
	} else if count == 3 {
		positions[0] = Point{X: 0, Y: 0, Z: space.Height}
		positions[1] = Point{X: space.Width, Y: 0, Z: space.Height}
		positions[2] = Point{X: space.Width / 2, Y: space.Depth, Z: 0}
	} else if count == 4 {
		positions[0] = Point{X: 0, Y: 0, Z: space.Height}
		positions[1] = Point{X: space.Width, Y: 0, Z: space.Height}
		positions[2] = Point{X: 0, Y: space.Depth, Z: space.Height}
		positions[3] = Point{X: space.Width, Y: space.Depth, Z: space.Height}
	} else {
		// For more than 4 nodes, distribute in a grid pattern
		gridSize := int(math.Ceil(math.Sqrt(float64(count))))
		for i := 0; i < count; i++ {
			row := i / gridSize
			col := i % gridSize
			positions[i] = Point{
				X: float64(col) * space.Width / float64(gridSize-1),
				Y: float64(row) * space.Depth / float64(gridSize-1),
				Z: space.Height / 2,
			}
		}
	}

	return positions
}

// generateMAC generates a MAC address for a virtual node
func generateMAC(id int) [6]byte {
	var mac [6]byte
	// Use a predictable OUI + node ID
	mac[0] = 0x02 // Locally administered
	mac[1] = 0x53 // Spaxel OUI (fictional)
	mac[2] = 0xAC
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

// createWalkers creates walkers with random walk behavior
func createWalkers(count int, space *Space, rng *rand.Rand) []*Walker {
	walkers := make([]*Walker, count)
	for i := 0; i < count; i++ {
		walkers[i] = &Walker{
			ID:       i,
			Position: Point{X: space.Width / 2, Y: space.Depth / 2, Z: 1.0}, // Start in center
			Velocity: Point{X: 0, Y: 0, Z: 0},
		}
		log.Printf("[SIM] Walker %d: starting at (%.2f,%.2f,%.2f)",
			i, walkers[i].Position.X, walkers[i].Position.Y, walkers[i].Position.Z)
	}
	return walkers
}

// contextWithCancel creates a context that can be cancelled
func contextWithCancel() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

// connectNodes connects all virtual nodes to the mothership via WebSocket
func connectNodes(ctx context.Context, nodes []*VirtualNode, token string, rng *rand.Rand, rejectChan chan<- struct{}) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(nodes))

	for _, node := range nodes {
		wg.Add(1)
		go func(n *VirtualNode) {
			defer wg.Done()

			// Build WebSocket URL with token in header
			u, err := url.Parse(*flagMothership)
			if err != nil {
				errChan <- fmt.Errorf("node %d: invalid URL: %w", n.ID, err)
				return
			}

			// Create request with token header
			reqHeader := http.Header{}
			reqHeader.Set("X-Spaxel-Token", token)

			// Connect to WebSocket
			conn, resp, err := websocket.DefaultDialer.DialContext(ctx, u.String(), reqHeader)
			if err != nil {
				if resp != nil {
					// Check for reject response
					if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
						body, _ := io.ReadAll(resp.Body)
						resp.Body.Close()
						log.Printf("[SIM] Node %d: REJECT response from mothership (status %d): %s", n.ID, resp.StatusCode, string(body))
						stats.Rejected.Store(true)
						select {
						case rejectChan <- struct{}{}:
						case <-ctx.Done():
						}
						errChan <- fmt.Errorf("node %d: rejected by mothership", n.ID)
						return
					}
					resp.Body.Close()
				}
				errChan <- fmt.Errorf("node %d: connection failed: %w", n.ID, err)
				return
			}
			defer resp.Body.Close()

			n.mu.Lock()
			n.Conn = conn
			n.mu.Unlock()

			log.Printf("[SIM] Node %d: connected to mothership", n.ID)

			// Send hello message. Announce the node's computed position
			// (createVirtualNodes corner geometry) so the mothership persists it
			// in the fleet/DB row instead of leaving it at the schema default (bf-24xp).
			hello := map[string]interface{}{
				"type":            "hello",
				"mac":             macToString(n.MAC),
				"firmware_version": "sim-1.0.0",
				"capabilities":    []string{"csi", "ble", "tx", "rx"},
				"chip":            "ESP32-S3",
				"flash_mb":        16,
				"uptime_ms":       1000,
				"pos_x":           n.Position.X,
				"pos_y":           n.Position.Y,
				"pos_z":           n.Position.Z,
			}
			if err := conn.WriteJSON(hello); err != nil {
				log.Printf("[SIM] Node %d: failed to send hello: %v", n.ID, err)
				errChan <- err
				return
			}

			// Listen for downstream messages (role assignment, config, reject)
			go n.listenForDownstream(ctx, rejectChan)
		}(node)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("connection errors: %v", errs)
	}

	return nil
}

// listenForDownstream listens for downstream messages from the mothership
func (n *VirtualNode) listenForDownstream(ctx context.Context, rejectChan chan<- struct{}) {
	n.mu.Lock()
	conn := n.Conn
	n.mu.Unlock()

	defer func() {
		n.mu.Lock()
		if n.Conn == conn {
			n.Conn = nil
		}
		n.mu.Unlock()
		conn.Close()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var msg json.RawMessage
		if err := conn.ReadJSON(&msg); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("[SIM] Node %d: read error: %v", n.ID, err)
			return
		}

		// Parse message type
		var typeMsg struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(msg, &typeMsg); err != nil {
			log.Printf("[SIM] Node %d: invalid message: %s", n.ID, string(msg))
			continue
		}

		switch typeMsg.Type {
		case "reject":
			log.Printf("[SIM] Node %d: REJECT message received: %s", n.ID, string(msg))
			stats.Rejected.Store(true)
			select {
			case rejectChan <- struct{}{}:
			case <-ctx.Done():
			}
			return
		case "role", "config":
			log.Printf("[SIM] Node %d: received %s message", n.ID, typeMsg.Type)
		case "ota", "reboot", "identify", "baseline_request":
			log.Printf("[SIM] Node %d: received %s message (acknowledged)", n.ID, typeMsg.Type)
		default:
			log.Printf("[SIM] Node %d: received unknown message type: %s", n.ID, typeMsg.Type)
		}
	}
}

// runSimulation runs the main simulation loop
func runSimulation(ctx context.Context, nodes []*VirtualNode, walkers []*Walker, space *Space, rng *rand.Rand, rejectChan <-chan struct{}) {
	stats.StartTime = time.Now()
	stats.LastStatsTime = stats.StartTime

	ticker := time.NewTicker(time.Duration(1000/(*flagRate)) * time.Millisecond)
	defer ticker.Stop()

	bleTicker := time.NewTicker(5 * time.Second)
	defer bleTicker.Stop()

	durationTimer := time.NewTimer(time.Duration(*flagDuration) * time.Second)
	if *flagDuration == 0 {
		durationTimer.Stop()
	}

	frameNum := 0
	walkerUpdateTicker := time.NewTicker(50 * time.Millisecond) // Update walkers every 50ms
	defer walkerUpdateTicker.Stop()

	// Handle interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)

	for {
		select {
		case <-ctx.Done():
			return
		case <-sigChan:
			log.Printf("[SIM] Interrupted, shutting down...")
			return
		case <-durationTimer.C:
			log.Printf("[SIM] Duration elapsed, shutting down...")
			return
		case <-rejectChan:
			log.Printf("[SIM] Node rejected by mothership, exiting...")
			stats.Rejected.Store(true)
			return
		case <-ticker.C:
			// Send CSI frames for all TX->RX pairs
			for _, tx := range nodes {
				for _, rx := range nodes {
					if tx.ID == rx.ID {
						continue // Skip self-pairs
					}

					frame := generateCSIFrame(tx, rx, walkers, frameNum, rng)

					tx.mu.Lock()
					conn := tx.Conn
					tx.mu.Unlock()

					if conn != nil {
						if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
							log.Printf("[SIM] Node %d: send error: %v", tx.ID, err)
							continue
						}
						stats.FramesSent.Add(1)
					}
				}
			}
			frameNum++
		case <-walkerUpdateTicker.C:
			// Update walker positions (random walk)
			updateWalkers(walkers, space, rng)
		case <-bleTicker.C:
			if *flagBLE {
				sendBLEAdvertisements(nodes, rng)
			}
		}
	}
}

// generateCSIFrame generates a synthetic CSI binary frame
func generateCSIFrame(tx, rx *VirtualNode, walkers []*Walker, frameNum int, rng *rand.Rand) []byte {
	// Calculate combined CSI from all walkers
	amplitude, phaseBase := computeCSIForWalkers(tx, rx, walkers)

	// Compute RSSI from amplitude
	rssi := amplitudeToRSSI(amplitude)

	// Create frame buffer
	frame := make([]byte, headerSize+nSub*2)

	// Write header (matches ingestion/frame.go ParseFrame layout)
	copy(frame[0:6], tx.MAC[:])   // node_mac
	copy(frame[6:12], rx.MAC[:])  // peer_mac
	binary.LittleEndian.PutUint64(frame[12:20], uint64(frameNum*50000)) // timestamp_us
	frame[20] = byte(int8(rssi))  // rssi
	frame[21] = byte(-95 & 0xFF)   // noise_floor: -95 dBm
	frame[22] = byte(defaultChannel) // channel
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

		// Add Gaussian noise
		amplitudeNoisy := subAmplitude * (1 + randNorm(rng, 0, 0.05))

		// Generate I/Q
		i, q := generateIQPair(amplitudeNoisy, phase, rng)

		// Write to payload (interleaved I,Q)
		offset := headerSize + k*2
		frame[offset] = byte(int8(i))
		frame[offset+1] = byte(int8(q))
	}

	return frame
}

// computeCSIForWalkers computes the combined CSI amplitude and phase from all walkers
func computeCSIForWalkers(tx, rx *VirtualNode, walkers []*Walker) (float64, float64) {
	if len(walkers) == 0 {
		// No walkers, return baseline noise
		return 0.001, 0.0
	}

	var totalAmplitude float64
	var totalPhase float64
	var weight float64

	for _, walker := range walkers {
		// Direct path contribution
		directAmp, directPhase := computeDirectPath(tx.Position, rx.Position, walker.Position)

		// Scale to reasonable values
		combinedAmp := directAmp * 1000.0

		// Accumulate
		totalAmplitude += combinedAmp
		totalPhase += directPhase
		weight += 1.0
	}

	// Normalize phase
	if weight > 0 {
		totalPhase /= weight
	}

	return totalAmplitude, totalPhase
}

// computeDirectPath computes the CSI contribution from the direct path
func computeDirectPath(tx, rx, walker Point) (float64, float64) {
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
	if dTotal >= 1.0 {
		pathLossDB = pl0 + 10.0*n*math.Log10(dTotal/1.0)
	} else {
		pathLossDB = pl0
	}

	// Convert to linear amplitude
	amplitude := math.Pow(10.0, -pathLossDB/20.0)

	// Apply Fresnel zone decay
	amplitude *= decay

	// Phase at this position (based on total path length)
	phase := 2 * math.Pi * dTotal / wavelength

	return amplitude, phase
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

// generateIQPair generates a synthetic I/Q pair
func generateIQPair(amplitude, phase float64, rng *rand.Rand) (float64, float64) {
	i := amplitude * math.Cos(phase)
	q := amplitude * math.Sin(phase)
	return i, q
}

// randNorm generates a normally-distributed random value (Box-Muller)
func randNorm(rng *rand.Rand, mean, stddev float64) float64 {
	u1 := rng.Float64()
	u2 := rng.Float64()
	z0 := math.Sqrt(-2.0*math.Log(u1)) * math.Cos(2.0*math.Pi*u2)
	return mean + stddev*z0
}

// updateWalkers updates walker positions using random walk behavior
func updateWalkers(walkers []*Walker, space *Space, rng *rand.Rand) {
	const dt = 0.05 // 50ms in seconds
	const sigma = 0.3 // m/s per axis

	for _, walker := range walkers {
		// Gaussian velocity update
		dvx := randNorm(rng, 0, sigma)
		dvy := randNorm(rng, 0, sigma)
		dvz := randNorm(rng, 0, sigma)

		walker.Velocity.X += dvx * dt
		walker.Velocity.Y += dvy * dt
		walker.Velocity.Z += dvz * dt

		// Clamp velocity to reasonable range
		maxV := 2.0 // m/s
		vMag := math.Sqrt(walker.Velocity.X*walker.Velocity.X +
			walker.Velocity.Y*walker.Velocity.Y +
			walker.Velocity.Z*walker.Velocity.Z)
		if vMag > maxV {
			scale := maxV / vMag
			walker.Velocity.X *= scale
			walker.Velocity.Y *= scale
			walker.Velocity.Z *= scale
		}

		// Update position
		walker.Position.X += walker.Velocity.X * dt
		walker.Position.Y += walker.Velocity.Y * dt
		walker.Position.Z += walker.Velocity.Z * dt

		// Reflect at walls
		if walker.Position.X < 0 {
			walker.Position.X = -walker.Position.X
			walker.Velocity.X *= -1
		}
		if walker.Position.X > space.Width {
			walker.Position.X = 2*space.Width - walker.Position.X
			walker.Velocity.X *= -1
		}
		if walker.Position.Y < 0 {
			walker.Position.Y = -walker.Position.Y
			walker.Velocity.Y *= -1
		}
		if walker.Position.Y > space.Depth {
			walker.Position.Y = 2*space.Depth - walker.Position.Y
			walker.Velocity.Y *= -1
		}
		if walker.Position.Z < 0 {
			walker.Position.Z = -walker.Position.Z
			walker.Velocity.Z *= -1
		}
		if walker.Position.Z > space.Height {
			walker.Position.Z = 2*space.Height - walker.Position.Z
			walker.Velocity.Z *= -1
		}
	}
}

// sendBLEAdvertisements sends simulated BLE advertisements from one node
func sendBLEAdvertisements(nodes []*VirtualNode, rng *rand.Rand) {
	if len(nodes) == 0 {
		return
	}

	// Send from first node
	node := nodes[0]

	node.mu.Lock()
	conn := node.Conn
	node.mu.Unlock()

	if conn == nil {
		return
	}

	// Generate simulated BLE device address
	addr := fmt.Sprintf("AA:BB:CC:DD:%02X:%02X", rng.Intn(256), rng.Intn(256))
	rssi := -60 + rng.Intn(20) // -60 to -40 dBm

	ble := map[string]interface{}{
		"type": "ble",
		"mac":  macToString(node.MAC),
		"devices": []map[string]interface{}{
			{
				"addr":        addr,
				"addr_type":   "random",
				"rssi_dbm":    rssi,
				"name":        "SimPerson",
			},
		},
	}

	if err := conn.WriteJSON(ble); err != nil {
		log.Printf("[SIM] Failed to send BLE advertisement: %v", err)
	}
}

// pollBlobCount polls the mothership for blob count
func pollBlobCount() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Build HTTP URL from WebSocket URL
		wsURL, err := url.Parse(*flagMothership)
		if err != nil {
			continue
		}

		httpURL := *wsURL
		if httpURL.Scheme == "ws" {
			httpURL.Scheme = "http"
		} else if httpURL.Scheme == "wss" {
			httpURL.Scheme = "https"
		}

		blobsURL := httpURL.String()
		blobsURL = strings.TrimSuffix(blobsURL, "/ws")
		blobsURL = strings.TrimSuffix(blobsURL, "/")
		blobsURL += "/api/blobs"

		resp, err := http.Get(blobsURL)
		if err != nil {
			continue
		}

		if resp.StatusCode == http.StatusOK {
			var blobs []json.RawMessage
			if err := json.NewDecoder(resp.Body).Decode(&blobs); err == nil {
				stats.BlobCount = len(blobs)
			}
		}
		resp.Body.Close()
	}
}

// reportStats reports statistics every second
func reportStats() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		elapsed := now.Sub(stats.LastStatsTime).Seconds()
		if elapsed < 1 {
			continue
		}

		framesSent := stats.FramesSent.Load()
		framesInPeriod := framesSent - stats.LastFramesSent

		stats.FramesPerSec = float64(framesInPeriod) / elapsed

		log.Printf("[SIM] Stats: frames/s=%.1f total=%d blobs=%d",
			stats.FramesPerSec, framesSent, stats.BlobCount)

		stats.LastStatsTime = now
		stats.LastFramesSent = framesSent
	}
}

// printFinalStats prints final simulation statistics
func printFinalStats() {
	elapsed := time.Since(stats.StartTime).Seconds()
	framesSent := stats.FramesSent.Load()

	log.Printf("[SIM] Final Statistics:")
	log.Printf("[SIM]   Frames sent: %d", framesSent)
	log.Printf("[SIM]   Duration: %.1f seconds", elapsed)
	if elapsed > 0 {
		log.Printf("[SIM]   Average FPS: %.1f", float64(framesSent)/elapsed)
	}
	log.Printf("[SIM]   Blobs detected: %d", stats.BlobCount)
}
