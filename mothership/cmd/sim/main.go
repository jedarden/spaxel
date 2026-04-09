// Package main provides a CSI simulator for testing the mothership.
// It simulates ESP32 nodes that send synthetic CSI frames via WebSocket.
package main

import (
	"encoding/binary"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spaxel/mothership/internal/simulator"
)

// Version is set by the build process via -ldflags
var version = "dev"

// isTimeoutErr checks if the error is a timeout (compatible with gorilla/websocket v1.5+).
func isTimeoutErr(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

const (
	// CSI frame constants from the plan
	HeaderSize   = 24
	MaxSubcarriers = 64
	DefaultSubcarriers = 52 // Typical HT20

	// WiFi wavelength for Fresnel calculations
	Wavelength = 0.123 // meters (2.4 GHz)
)

var (
	showVersion   = flag.Bool("version", false, "Show version information and exit")
	mothershipURL = flag.String("mothership", "ws://localhost:8080/ws/node", "Mothership WebSocket URL")
	token         = flag.String("token", "", "Node authentication token (X-Spaxel-Token header)")
	nodes         = flag.Int("nodes", 4, "Number of virtual nodes to simulate")
	walkers       = flag.Int("walkers", 1, "Number of walking persons to simulate")
	rate          = flag.Int("rate", 20, "CSI packet rate in Hz")
	duration      = flag.Duration("duration", 30*time.Second, "Simulation duration (0 = run until Ctrl+C)")
	enableBLE     = flag.Bool("ble", false, "Also send simulated BLE advertisements")
	seed          = flag.Int64("seed", 42, "Random seed for reproducible runs")
	spaceWidth    = flag.Float64("width", 6.0, "Space width in meters")
	spaceDepth    = flag.Float64("depth", 5.0, "Space depth in meters")
	spaceHeight   = flag.Float64("height", 2.5, "Space height in meters")
	spaceDims     = flag.String("space", "", "Space dimensions as 'WxDxH' (meters), overrides --width/--depth/--height")
	showFrameRate = flag.Bool("show-frame-rate", true, "Show per-second frame counts to stdout")
	verbose       = flag.Bool("verbose", false, "Enable verbose logging")

	// New flags for verification and advanced simulation
	verifyMode    = flag.Bool("verify", false, "Verify blob count after simulation (exit code 0=pass, 1=fail)")
	noiseSigma    = flag.Float64("noise-sigma", 0.005, "Gaussian noise standard deviation for I/Q generation")
	wallDefs      = flag.String("wall", "", "Add wall as 'x1,y1,x2,y2' (can be repeated)")
	outputCSV    = flag.String("output-csv", "", "Write ground truth to CSV file")
	provision     = flag.Bool("provision", false, "Auto-provision via POST /api/provision")
)

// CSIFrame represents a CSI binary frame
type CSIFrame struct {
	NodeMAC     [6]byte
	PeerMAC     [6]byte
	TimestampUS uint64
	RSSI        int8
	NoiseFloor  int8
	Channel     uint8
	NSub        uint8
	Payload     []int8 // Interleaved I,Q pairs
}

// HelloMessage is sent on connection
type HelloMessage struct {
	Type            string   `json:"type"`
	MAC             string   `json:"mac"`
	NodeID          string   `json:"node_id,omitempty"`
	FirmwareVersion string   `json:"firmware_version"`
	Capabilities    []string `json:"capabilities"`
	Chip            string   `json:"chip,omitempty"`
	FlashMB         int      `json:"flash_mb,omitempty"`
	UptimeMS        int64    `json:"uptime_ms,omitempty"`
	APBSSID         string   `json:"ap_bssid,omitempty"`
	APChannel       int      `json:"ap_channel,omitempty"`
}

// HealthMessage is sent every 10 seconds
type HealthMessage struct {
	Type         string `json:"type"`
	MAC          string `json:"mac"`
	TimestampMS  int64  `json:"timestamp_ms"`
	FreeHeapBytes int64  `json:"free_heap_bytes"`
	WifiRSSIdBm  int    `json:"wifi_rssi_dbm"`
	UptimeMS     int64  `json:"uptime_ms"`
	TemperatureC float64 `json:"temperature_c,omitempty"`
	CSIRateHz    int    `json:"csi_rate_hz"`
	WifiChannel  int    `json:"wifi_channel"`
	IP           string `json:"ip,omitempty"`
}

// BLEMessage is sent every 5 seconds
type BLEMessage struct {
	Type        string `json:"type"`
	MAC         string `json:"mac"`
	TimestampMS int64  `json:"timestamp_ms"`
	Devices     []BLEDevice `json:"devices"`
}

// BLEDevice represents a simulated BLE device
type BLEDevice struct {
	Addr       string `json:"addr"`
	AddrType   string `json:"addr_type,omitempty"`
	RSSIdBm    int    `json:"rssi_dbm"`
	Name       string `json:"name,omitempty"`
	MfrID      int    `json:"mfr_id,omitempty"`
	MfrDataHex string `json:"mfr_data_hex,omitempty"`
}

// VirtualNode represents a simulated ESP32 node
type VirtualNode struct {
	mac         string
	position   [3]float64 // x, y, z in meters
	conn        *websocket.Conn
	mu          sync.Mutex
	connected   bool
	frameCount  int
	lastSecond  time.Time
	secondCount int
}

// Walker represents a simulated person moving through space
type Walker struct {
	position [3]float64 // x, y, z in meters
	velocity [3]float64 // vx, vy, vz in m/s
	mac      string     // BLE address for this walker
}

func main() {
	flag.Parse()

	if *showVersion {
		fmt.Printf("CSI Simulator version %s\n", version)
		os.Exit(0)
	}

	// Parse --space flag if provided (format: "WxDxH")
	if *spaceDims != "" {
		width, depth, height, err := parseSpaceDims(*spaceDims)
		if err != nil {
			log.Fatalf("[ERROR] Invalid --space format: %v (expected 'WxDxH' e.g., '6x5x2.5')", err)
		}
		*spaceWidth = width
		*spaceDepth = depth
		*spaceHeight = height
	}

	// Parse wall definitions
	walls := parseWalls(*wallDefs)

	// Create simulation configuration
	config := SimConfig{
		MothershipURL: *mothershipURL,
		Token:         *token,
		Nodes:         *nodes,
		Walkers:       *walkers,
		Rate:          *rate,
		Duration:      *duration,
		EnableBLE:     *enableBLE,
		Seed:          *seed,
		SpaceWidth:    *spaceWidth,
		SpaceDepth:    *spaceDepth,
		SpaceHeight:   *spaceHeight,
		NoiseSigma:    *noiseSigma,
		VerifyMode:    *verifyMode,
		OutputCSV:     *outputCSV,
		Walls:         walls,
		Verbose:       *verbose,
		ShowFrameRate: *showFrameRate,
	}

	if err := runSimulation(config); err != nil {
		log.Fatalf("[ERROR] Simulation failed: %v", err)
	}
}

// runSimulation runs the complete simulation workflow
func runSimulation(config SimConfig) error {
	if config.Seed != 0 {
		rand.Seed(config.Seed)
	}

	log.Printf("[INFO] CSI Simulator starting")
	log.Printf("[INFO] Configuration: nodes=%d, walkers=%d, rate=%d Hz, duration=%s",
		config.Nodes, config.Walkers, config.Rate, config.Duration)
	log.Printf("[INFO] Space: %.1fx%.1fx%.1f m", config.SpaceWidth, config.SpaceDepth, config.SpaceHeight)
	if len(config.Walls) > 0 {
		log.Printf("[INFO] Walls: %d defined", len(config.Walls))
	}
	if config.Token != "" {
		log.Printf("[INFO] Using authentication token")
	}
	log.Printf("[INFO] Connecting to: %s", config.MothershipURL)

	// Create walker simulator
	walkerSim := NewWalkerSimulator(config.Walkers, config.SpaceWidth, config.SpaceDepth, config.SpaceHeight, config.Seed)

	// Open CSV file if specified
	if config.OutputCSV != "" {
		if err := walkerSim.OpenCSV(config.OutputCSV); err != nil {
			return fmt.Errorf("failed to open CSV file: %w", err)
		}
		defer walkerSim.CloseCSV()
		log.Printf("[INFO] Writing ground truth to %s", config.OutputCSV)
	}

	// Create virtual nodes at corners and edges of the room
	virtualNodes := createVirtualNodes(config.Nodes, config.SpaceWidth, config.SpaceDepth, config.SpaceHeight)

	// Start all nodes
	var wg sync.WaitGroup
	for i := range virtualNodes {
		wg.Add(1)
		go func(n *VirtualNode) {
			defer wg.Done()
			if err := n.run(config, walkerSim); err != nil {
				log.Printf("[ERROR] Node %s failed: %v", n.mac, err)
				os.Exit(1)
			}
		}(&virtualNodes[i])
	}

	// Wait for all nodes to complete or error
	wg.Wait()

	log.Printf("[INFO] Simulation completed successfully")
	if *showFrameRate {
		for i := range virtualNodes {
			n := &virtualNodes[i]
			log.Printf("[STATS] Node %s: sent %d frames", n.mac, n.frameCount)
		}
	}

	// Verification mode: check blob count
	if config.VerifyMode {
		log.Printf("[INFO] Running verification mode...")

		// Get walker positions
		walkerPositions := make([][3]float64, walkerSim.Count())
		for i, w := range walkerSim.GetWalkers() {
			walkerPositions[i] = w.Position
		}

		// Run verification
		verifier := NewVerifier(config.MothershipURL)
		result, err := verifier.Verify(walkerSim.Count(), walkerPositions)
		if err != nil {
			log.Printf("[ERROR] Verification failed: %v", err)
			os.Exit(1)
		}

		verifier.ExitWithResult(result)
	}
}

// SimConfig holds simulation configuration
type SimConfig struct {
	MothershipURL string
	Token         string
	Nodes         int
	Walkers       int
	Rate          int
	Duration      time.Duration
	EnableBLE     bool
	Seed          int64
	SpaceWidth    float64
	SpaceDepth    float64
	SpaceHeight   float64
	NoiseSigma    float64
	VerifyMode    bool
	OutputCSV     string
	Walls         []WallDef
	Verbose       bool
	ShowFrameRate bool
}

// WallDef defines a wall segment
type WallDef struct {
	X1, Y1, X2, Y2 float64
}
}

// createVirtualNodes positions virtual nodes in the space
func createVirtualNodes(count int, width, depth, height float64) []VirtualNode {
	nodes := make([]VirtualNode, count)

	for i := 0; i < count; i++ {
		// Position nodes around the perimeter and corners
		switch i {
		case 0:
			nodes[i].position = [3]float64{0, 0, height * 0.8} // Top-left, high
		case 1:
			nodes[i].position = [3]float64{width, 0, height * 0.8} // Top-right, high
		case 2:
			nodes[i].position = [3]float64{0, depth, height * 0.8} // Bottom-left, high
		case 3:
			nodes[i].position = [3]float64{width, depth, height * 0.8} // Bottom-right, high
		case 4:
			nodes[i].position = [3]float64{width / 2, 0, height * 0.3} // Top-middle, low
		case 5:
			nodes[i].position = [3]float64{width / 2, depth, height * 0.3} // Bottom-middle, low
		default:
			// Distribute remaining nodes evenly
			nodes[i].position = [3]float64{
				(float64(i) * width) / float64(count),
				(float64(i) * depth) / float64(count),
				height * 0.5,
			}
		}

		// Generate MAC address
		nodes[i].mac = fmt.Sprintf("AA:BB:CC:DD:%02X:00", i)
	}

	return nodes
}

// createWalkers creates simulated walkers
func createWalkers(count int, width, depth, height float64) []Walker {
	walkers := make([]Walker, count)

	for i := range walkers {
		// Start in center of room
		walkers[i].position = [3]float64{width / 2, depth / 2, 1.7} // 1.7m = average person height

		// Random initial velocity
		walkers[i].velocity = [3]float64{
			(rand.Float64() - 0.5) * 0.5, // -0.25 to +0.25 m/s X
			(rand.Float64() - 0.5) * 0.5, // -0.25 to +0.25 m/s Y
			0,                             // Z stays constant
		}

		// Generate BLE address
		walkers[i].mac = fmt.Sprintf("11:22:33:44:55:%02X", i)
	}

	return walkers
}

// parseWalls parses wall definitions from command line flag
func parseWalls(wallDefs string) []WallDef {
	if wallDefs == "" {
		return nil
	}

	var walls []WallDef
	parts := strings.Split(wallDefs, ",")
	if len(parts) < 4 {
		log.Printf("[WARN] Invalid wall definition: %s (expected x1,y1,x2,y2)", wallDefs)
		return nil
	}

	var x1, y1, x2, y2 float64
	var err error
	if x1, err = strconv.ParseFloat(parts[0], 64); err != nil {
		log.Printf("[WARN] Invalid wall x1: %v", err)
		return nil
	}
	if y1, err = strconv.ParseFloat(parts[1], 64); err != nil {
		log.Printf("[WARN] Invalid wall y1: %v", err)
		return nil
	}
	if x2, err = strconv.ParseFloat(parts[2], 64); err != nil {
		log.Printf("[WARN] Invalid wall x2: %v", err)
		return nil
	}
	if y2, err = strconv.ParseFloat(parts[3], 64); err != nil {
		log.Printf("[WARN] Invalid wall y2: %v", err)
		return nil
	}

	walls = append(walls, WallDef{X1: x1, Y1: y1, X2: x2, Y2: y2})
	log.Printf("[INFO] Added wall: (%.1f,%.1f) to (%.1f,%.1f)", x1, y1, x2, y2)
	return walls
}

// run starts the virtual node simulation
func (n *VirtualNode) run(config SimConfig, walkerSim *WalkerSimulator) error {
	// Parse mothership URL
	u, err := url.Parse(config.MothershipURL)
	if err != nil {
		return fmt.Errorf("invalid mothership URL: %w", err)
	}

	// Prepare WebSocket request headers with authentication token
	headers := make(map[string][]string)
	if config.Token != "" {
		headers["X-Spaxel-Token"] = []string{config.Token}
	}

	// Connect to mothership
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	conn, resp, err := dialer.Dial(u.String(), headers)
	if err != nil {
		// Check for authentication failure
		if resp != nil && resp.StatusCode == 401 {
			return fmt.Errorf("authentication failed: invalid token (HTTP 401)")
		}
		return fmt.Errorf("WebSocket dial failed: %w", err)
	}
	defer conn.Close()

	n.mu.Lock()
	n.conn = conn
	n.connected = true
	n.lastSecond = time.Now()
	n.mu.Unlock()

	log.Printf("[INFO] Node %s connected to mothership", n.mac)

	// Send hello message
	uptime := int64(1000) // 1 second
	hello := HelloMessage{
		Type:            "hello",
		MAC:             n.mac,
		NodeID:          fmt.Sprintf("sim-node-%s", n.mac),
		FirmwareVersion: "0.1.0-sim",
		Capabilities:    []string{"csi", "tx", "rx"},
		Chip:            "ESP32-S3",
		FlashMB:         16,
		UptimeMS:        uptime,
	}

	helloJSON, err := json.Marshal(hello)
	if err != nil {
		return fmt.Errorf("failed to marshal hello: %w", err)
	}

	if err := conn.WriteMessage(websocket.TextMessage, helloJSON); err != nil {
		return fmt.Errorf("failed to send hello: %w", err)
	}

	if config.Verbose {
		log.Printf("[DEBUG] Node %s sent hello", n.mac)
	}

	// Wait for role assignment
	time.Sleep(100 * time.Millisecond)

	// Start ticker for CSI frames
	ticker := time.NewTicker(time.Second / time.Duration(config.Rate))
	defer ticker.Stop()

	// Health ticker (every 10 seconds)
	healthTicker := time.NewTicker(10 * time.Second)
	defer healthTicker.Stop()

	// BLE ticker (every 5 seconds)
	var bleTicker *time.Ticker
	if config.EnableBLE {
		bleTicker = time.NewTicker(5 * time.Second)
		defer bleTicker.Stop()
	}

	// Frame rate tracking ticker
	var frameRateTicker *time.Ticker
	if config.ShowFrameRate {
		frameRateTicker = time.NewTicker(time.Second)
		defer frameRateTicker.Stop()
	}

	startTime := time.Now()
	frameIndex := uint64(0)
	lastCSVWrite := startTime

	// Main loop (run forever if duration is 0)
	for config.Duration == 0 || time.Since(startTime) < config.Duration {
		select {
		case <-ticker.C:
			// Update walker positions
			walkerSim.Update(0.05) // 50ms step

			// Write CSV row if configured (every 100ms)
			if config.OutputCSV != "" && time.Since(lastCSVWrite) > 100*time.Millisecond {
				for _, w := range walkerSim.GetWalkers() {
					if err := walkerSim.WriteCSVRow(time.Now(), w); err != nil {
						log.Printf("[WARN] Failed to write CSV: %v", err)
					}
				}
				lastCSVWrite = time.Now()
			}

			// Generate and send CSI frames for each link
			for _, walker := range walkerSim.GetWalkers() {
				frame := n.generateCSIFrame(walker, frameIndex, config.NoiseSigma)
				if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
					return fmt.Errorf("failed to send CSI frame: %w", err)
				}
				n.frameCount++
				n.secondCount++
				frameIndex++
			}

		case <-healthTicker.C:
			// Send health message
			uptime = time.Since(startTime).Milliseconds()
			health := HealthMessage{
				Type:         "health",
				MAC:          n.mac,
				TimestampMS:  time.Now().UnixMilli(),
				FreeHeapBytes: 204800,
				WifiRSSIdBm:  -50 - rand.Intn(20), // -50 to -70
				UptimeMS:     uptime,
				TemperatureC: 40 + rand.Float64()*5,
				CSIRateHz:    config.Rate,
				WifiChannel:  6,
				IP:           "192.168.1.100",
			}

			healthJSON, err := json.Marshal(health)
			if err != nil {
				log.Printf("[WARN] Failed to marshal health: %v", err)
				continue
			}

			if err := conn.WriteMessage(websocket.TextMessage, healthJSON); err != nil {
				return fmt.Errorf("failed to send health: %w", err)
			}

			if config.Verbose {
				log.Printf("[DEBUG] Node %s sent health", n.mac)
			}

		case <-bleTicker.C:
			// Send BLE scan results
			walkers := walkerSim.GetWalkers()
			if len(walkers) > 0 {
				walker := walkers[0] // Use first walker's BLE
				ble := BLEMessage{
					Type:        "ble",
					MAC:         n.mac,
					TimestampMS: time.Now().UnixMilli(),
					Devices: []BLEDevice{
						{
							Addr:     walker.BLEAddress,
							AddrType: "public",
							RSSIdBm:  -60 - rand.Intn(20),
							Name:     "SimPhone",
							MfrID:    76, // Apple
						},
					},
				}

				bleJSON, err := json.Marshal(ble)
				if err != nil {
					log.Printf("[WARN] Failed to marshal BLE: %v", err)
					continue
				}

				if err := conn.WriteMessage(websocket.TextMessage, bleJSON); err != nil {
					return fmt.Errorf("failed to send BLE: %w", err)
				}

				if config.Verbose {
					log.Printf("[DEBUG] Node %s sent BLE scan", n.mac)
				}
			}

		case <-frameRateTicker.C:
			// Report frame rate
			log.Printf("[STATS] Node %s: %d frames/s", n.mac, n.secondCount)
			n.secondCount = 0
		}

		// Check for reject message
		conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if !isTimeoutErr(err) && err.Error() != "EOF" {
				return fmt.Errorf("read error: %w", err)
			}
		} else if len(msg) > 0 && msg[0] == '{' {
			// JSON message
			var base struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(msg, &base); err == nil && base.Type == "reject" {
				return fmt.Errorf("node rejected by mothership")
			}
		}
	}

	return nil
}

// generateCSIFrame creates a synthetic CSI frame based on walker position
func (n *VirtualNode) generateCSIFrame(walker *Walker, frameIndex uint64, noiseSigma float64) []byte {
	nSub := DefaultSubcarriers

	// Calculate distance to walker
	dx := walker.Position[0] - n.position[0]
	dy := walker.Position[1] - n.position[1]
	dz := walker.Position[2] - n.position[2]
	distance := math.Sqrt(dx*dx + dy*dy + dz*dz)

	// Calculate path loss for RSSI
	// Free space path loss: PL(d) = PL_0 + 10*n*log10(d/d_0)
	// PL_0 = 40 dB at d_0 = 1m, n = 2.0
	pathLoss := 40.0 + 20.0*math.Log10(distance/1.0)
	rssi := int8(-30 - pathLoss) // -30 dBm reference

	// Clamp RSSI to valid range
	if rssi < -90 {
		rssi = -90
	}
	if rssi > -30 {
		rssi = -30
	}

	// Add Fresnel zone modulation
	// When walker is in a Fresnel zone, amplitude increases
	fresnelMod := fresnelModulation(n.position, walker.Position)

	// Create frame
	buf := make([]byte, HeaderSize + nSub*2)

	// Node MAC (6 bytes)
	macBytes := macToBytes(n.mac)
	copy(buf[0:6], macBytes[:])

	// Peer MAC (6 bytes) - use walker's simulated MAC
	peerMAC := macToBytes(fmt.Sprintf("11:22:33:44:55:%02X", 0))
	copy(buf[6:12], peerMAC[:])

	// Timestamp (8 bytes, uint64, little-endian)
	timestampUS := uint64(frameIndex * 1_000_000 / 20) // Assume 20 Hz
	binary.LittleEndian.PutUint64(buf[12:20], timestampUS)

	// RSSI (1 byte, int8)
	buf[20] = byte(rssi)

	// Noise floor (1 byte, int8)
	buf[21] = 161 // -95 dBm as int8 bit pattern (0xA1)

	// Channel (1 byte, uint8)
	buf[22] = 6 // Channel 6

	// Number of subcarriers (1 byte, uint8)
	buf[23] = byte(nSub)

	// Generate CSI payload (I, Q pairs)
	for k := 0; k < nSub; k++ {
		// Base amplitude with Fresnel modulation
		amplitude := 30.0 + float64(k)*0.1 + fresnelMod*8.0

		// Add subcarrier-dependent phase
		phase := float64(k) * 0.2

		// Add noise with configurable sigma
		noise := rand.NormFloat64() * noiseSigma * 100.0 // Scale up for visibility

		// Convert to I, Q and clamp to int8 range
		iVal := amplitude*math.Cos(phase) + noise
		qVal := amplitude*math.Sin(phase) + noise

		// Clamp to int8 range
		if iVal > 127 {
			iVal = 127
		}
		if iVal < -127 {
			iVal = -127
		}
		if qVal > 127 {
			qVal = 127
		}
		if qVal < -127 {
			qVal = -127
		}

		offset := HeaderSize + k*2
		buf[offset] = byte(int8(iVal))
		buf[offset+1] = byte(int8(qVal))
	}

	return buf
}

// fresnelModulation calculates the Fresnel zone modulation factor
func fresnelModulation(nodePos, walkerPos [3]float64) float64 {
	// Calculate path length excess
	nodeToWalker := math.Sqrt(
		math.Pow(walkerPos[0]-nodePos[0], 2) +
		math.Pow(walkerPos[1]-nodePos[1], 2) +
		math.Pow(walkerPos[2]-nodePos[2], 2))

	walkerToPeer := nodeToWalker // Simplified: peer is at same distance
	directPath := 5.0           // Simplified direct path

	deltaL := nodeToWalker + walkerToPeer - directPath

	// Fresnel zone number (λ/2 = 0.0615m)
	zone := math.Ceil(deltaL / 0.0615)

	// Modulation factor based on zone
	// Zone 1: maximum modulation, Zone 5+: minimum
	if zone <= 1 {
		return 1.0
	}
	if zone >= 5 {
		return 0.0
	}

	return 1.0 / math.Pow(zone, 2.0)
}

// updateWalkerPosition updates walker position with random walk
func updateWalkerPosition(w *Walker, width, depth float64) {
	const dt = 0.05 // 50ms step

	// Update position
	w.position[0] += w.velocity[0] * dt
	w.position[1] += w.velocity[1] * dt

	// Bounce off walls
	if w.position[0] < 0 || w.position[0] > width {
		w.velocity[0] *= -1
		w.position[0] = math.Max(0, math.Min(width, w.position[0]))
	}
	if w.position[1] < 0 || w.position[1] > depth {
		w.velocity[1] *= -1
		w.position[1] = math.Max(0, math.Min(depth, w.position[1]))
	}

	// Random velocity perturbation (simulates human motion)
	w.velocity[0] += (rand.Float64() - 0.5) * 0.1
	w.velocity[1] += (rand.Float64() - 0.5) * 0.1

	// Clamp velocity
	maxSpeed := 0.5
	speed := math.Sqrt(w.velocity[0]*w.velocity[0] + w.velocity[1]*w.velocity[1])
	if speed > maxSpeed {
		scale := maxSpeed / speed
		w.velocity[0] *= scale
		w.velocity[1] *= scale
	}
}

// macToBytes converts MAC string to bytes
func macToBytes(mac string) [6]byte {
	var b [6]byte
	fmt.Sscanf(mac, "%02X:%02X:%02X:%02X:%02X:%02X",
		&b[0], &b[1], &b[2], &b[3], &b[4], &b[5])
	return b
}

// parseSpaceDims parses space dimensions from "WxDxH" format
func parseSpaceDims(s string) (width, depth, height float64, err error) {
	// Split by 'x' to avoid hex interpretation (e.g., "0x5" parsed as hex)
	parts := strings.Split(s, "x")
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("invalid format: expected 3 dimensions separated by 'x'")
	}

	var err2 error
	width, err2 = strconv.ParseFloat(parts[0], 64)
	if err2 != nil {
		return 0, 0, 0, fmt.Errorf("invalid width: %w", err2)
	}
	depth, err2 = strconv.ParseFloat(parts[1], 64)
	if err2 != nil {
		return 0, 0, 0, fmt.Errorf("invalid depth: %w", err2)
	}
	height, err2 = strconv.ParseFloat(parts[2], 64)
	if err2 != nil {
		return 0, 0, 0, fmt.Errorf("invalid height: %w", err2)
	}

	if width <= 0 || depth <= 0 || height <= 0 {
		return 0, 0, 0, fmt.Errorf("dimensions must be positive")
	}
	return width, depth, height, nil
}
