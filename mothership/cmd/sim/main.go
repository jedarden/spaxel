// Package main provides a CSI simulator for testing the mothership.
// It simulates ESP32 nodes that send synthetic CSI frames via WebSocket.
package main

import (
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// CSI frame constants from the plan
	HeaderSize   = 24
	MaxSubcarriers = 64
	DefaultSubcarriers = 52 // Typical HT20

	// WiFi wavelength for Fresnel calculations
	Wavelength = 0.123 // meters (2.4 GHz)
)

var (
	mothershipURL = flag.String("mothership", "ws://localhost:8080/ws/node", "Mothership WebSocket URL")
	nodes         = flag.Int("nodes", 4, "Number of virtual nodes to simulate")
	walkers       = flag.Int("walkers", 1, "Number of walking persons to simulate")
	rate          = flag.Int("rate", 20, "CSI packet rate in Hz")
	duration      = flag.Duration("duration", 30*time.Second, "Simulation duration")
	enableBLE     = flag.Bool("ble", false, "Also send simulated BLE advertisements")
	seed          = flag.Int64("seed", 42, "Random seed for reproducible runs")
	spaceWidth    = flag.Float64("width", 6.0, "Space width in meters")
	spaceDepth    = flag.Float64("depth", 5.0, "Space depth in meters")
	spaceHeight   = flag.Float64("height", 2.5, "Space height in meters")
	showFrameRate = flag.Bool("show-frame-rate", true, "Show per-second frame counts to stdout")
	verbose       = flag.Bool("verbose", false, "Enable verbose logging")
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

	if *seed != 0 {
		rand.Seed(*seed)
	}

	log.Printf("[INFO] CSI Simulator starting")
	log.Printf("[INFO] Configuration: nodes=%d, walkers=%d, rate=%d Hz, duration=%s", *nodes, *walkers, *rate, *duration)
	log.Printf("[INFO] Space: %.1fx%.1fx%.1f m", *spaceWidth, *spaceDepth, *spaceHeight)
	log.Printf("[INFO] Connecting to: %s", *mothershipURL)

	// Create virtual nodes at corners and edges of the room
	virtualNodes := createVirtualNodes(*nodes, *spaceWidth, *spaceDepth, *spaceHeight)

	// Create walkers
	walkers := createWalkers(*walkers, *spaceWidth, *spaceDepth, *spaceHeight)

	// Start all nodes
	var wg sync.WaitGroup
	for i := range virtualNodes {
		wg.Add(1)
		go func(n *VirtualNode) {
			defer wg.Done()
			if err := n.run(walkers, *rate, *duration, *enableBLE, *verbose); err != nil {
				log.Printf("[ERROR] Node %s failed: %v", n.mac, err)
				os.Exit(1)
			}
		}(&virtualNodes[i])
	}

	// Wait for all nodes to complete or error
	wg.Wait()

	log.Printf("[INFO] Simulation completed successfully")
	if *showFrameRate {
		for _, n := range virtualNodes {
			log.Printf("[STATS] Node %s: sent %d frames", n.mac, n.frameCount)
		}
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

// run starts the virtual node simulation
func (n *VirtualNode) run(walkers []Walker, rateHz int, duration time.Duration, enableBLE, verbose bool) error {
	// Parse mothership URL
	u, err := url.Parse(*mothershipURL)
	if err != nil {
		return fmt.Errorf("invalid mothership URL: %w", err)
	}

	// Connect to mothership
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
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

	if verbose {
		log.Printf("[DEBUG] Node %s sent hello", n.mac)
	}

	// Wait for role assignment
	time.Sleep(100 * time.Millisecond)

	// Start ticker for CSI frames
	ticker := time.NewTicker(time.Second / time.Duration(rateHz))
	defer ticker.Stop()

	// Health ticker (every 10 seconds)
	healthTicker := time.NewTicker(10 * time.Second)
	defer healthTicker.Stop()

	// BLE ticker (every 5 seconds)
	var bleTicker *time.Ticker
	if enableBLE {
		bleTicker = time.NewTicker(5 * time.Second)
		defer bleTicker.Stop()
	}

	// Frame rate tracking ticker
	var frameRateTicker *time.Ticker
	if *showFrameRate {
		frameRateTicker = time.NewTicker(time.Second)
		defer frameRateTicker.Stop()
	}

	startTime := time.Now()
	frameIndex := uint64(0)

	// Main loop
	for time.Since(startTime) < duration {
		select {
		case <-ticker.C:
			// Update walker positions
			for i := range walkers {
				updateWalkerPosition(&walkers[i], *spaceWidth, *spaceDepth)
			}

			// Generate and send CSI frames for each link
			for _, walker := range walkers {
				frame := n.generateCSIFrame(walker, frameIndex)
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
				CSIRateHz:    rateHz,
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

			if verbose {
				log.Printf("[DEBUG] Node %s sent health", n.mac)
			}

		case <-bleTicker.C:
			// Send BLE scan results
			if len(walkers) > 0 {
				walker := walkers[0] // Use first walker's BLE
				ble := BLEMessage{
					Type:        "ble",
					MAC:         n.mac,
					TimestampMS: time.Now().UnixMilli(),
					Devices: []BLEDevice{
						{
							Addr:     walker.mac,
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

				if verbose {
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
			if !websocket.IsTimeout(err) && err.Error() != "EOF" {
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
func (n *VirtualNode) generateCSIFrame(walker Walker, frameIndex uint64) []byte {
	nSub := DefaultSubcarriers

	// Calculate distance to walker
	dx := walker.position[0] - n.position[0]
	dy := walker.position[1] - n.position[1]
	dz := walker.position[2] - n.position[2]
	distance := math.Sqrt(dx*dx + dy*dy + dz*dz)

	// Calculate path loss for RSSI
	// Free space path loss: PL(d) = PL_0 + 10*n*log10(d/d_0)
	// PL_0 = 40 dB at d_0 = 1m, n = 2.0
	pathLoss := 40 + 20*math.Log10(distance/1.0)
	rssi := int8(-30 - pathLoss) // -30 dBm reference

	// Add Fresnel zone modulation
	// When walker is in a Fresnel zone, amplitude increases
	fresnelMod := fresnelModulation(n.position, walker.position)

	// Create frame
	buf := make([]byte, HeaderSize + nSub*2)

	// Node MAC (6 bytes)
	macBytes := macToBytes(n.mac)
	copy(buf[0:6], macBytes[:])

	// Peer MAC (6 bytes) - use walker's simulated MAC
	peerMAC := macToBytes(fmt.Sprintf("11:22:33:44:55:%02X", 0))
	copy(buf[6:12], peerMAC[:])

	// Timestamp (8 bytes, uint64, little-endian)
	timestampUS := uint64(frameIndex * 1_000_000 / uint64(*rate))
	binary.LittleEndian.PutUint64(buf[12:20], timestampUS)

	// RSSI (1 byte, int8)
	buf[20] = byte(rssi)

	// Noise floor (1 byte, int8)
	buf[21] = byte(-95) // Typical noise floor

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

		// Add noise
		noise := rand.NormFloat64() * 2.0

		// Convert to I, Q
		iVal := int8(amplitude*math.Cos(phase) + noise)
		qVal := int8(amplitude*math.Sin(phase) + noise)

		offset := HeaderSize + k*2
		buf[offset] = byte(iVal)
		buf[offset+1] = byte(qVal)
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
