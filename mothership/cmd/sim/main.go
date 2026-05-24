// Command sim is a CSI simulator CLI for testing Spaxel without hardware.
// It connects to a running mothership via WebSocket and streams synthetic CSI data.
package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spaxel/mothership/internal/simulator"
)

const (
	// CSI frame header size (24 bytes) — matches ingestion/frame.go
	headerSize = 24

	// Default values
	defaultMothership = "ws://localhost:8080/ws/node"
	defaultNodes      = 2
	defaultWalkers    = 1
	defaultRate       = 20        // Hz
	defaultDuration   = 60        // seconds
	defaultChannel    = 6         // 2.4 GHz channel 6
	defaultSeed       = 0         // random seed (0 = use current time)
	defaultSpace      = "5x5x2.5" // room dimensions
	defaultNoiseSigma = 0.005
)

var (
	// CLI flags
	flagMothership = flag.String("mothership", defaultMothership, "URL of the mothership WebSocket endpoint")
	flagToken      = flag.String("token", "", "Provisioning token (auto-generated if empty)")
	flagNodes      = flag.Int("nodes", defaultNodes, "Number of virtual nodes")
	flagWalkers    = flag.Int("walkers", defaultWalkers, "Number of synthetic walkers")
	flagRate       = flag.Int("rate", defaultRate, "CSI transmission rate in Hz per node pair")
	flagDuration   = flag.Int("duration", defaultDuration, "Total run time in seconds (0 = run until Ctrl+C)")
	flagSeed       = flag.Int64("seed", defaultSeed, "Random seed for reproducible walker paths")
	flagSpace      = flag.String("space", defaultSpace, "Room dimensions in WxDxH format (meters)")
	flagBLE        = flag.Bool("ble", false, "Include synthetic BLE advertisements")
	flagVerify     = flag.Bool("verify", false, "Verify blob detection after duration")
	flagNoiseSigma = flag.Float64("noise-sigma", defaultNoiseSigma, "Gaussian noise standard deviation for I/Q")
	flagOutputCSV  = flag.String("output-csv", "", "Write ground truth to CSV file")
	flagChannel    = flag.Int("channel", defaultChannel, "WiFi channel (1-14 for 2.4 GHz)")
	flagWalkerType = flag.String("walker-type", "random", "Walker type: random, path, node-to-node")
	flagPathFile   = flag.String("path-file", "", "JSON file containing walker paths")

	// GDOP and shopping list flags
	flagGDOPOverlay  = flag.Bool("gdop-overlay", false, "Output GDOP overlay data as JSON to stdout")
	flagShoppingList = flag.Bool("shopping-list", false, "Output shopping list as JSON to stdout")
	flagCellSize     = flag.Float64("cell-size", 0.2, "GDOP grid cell size in meters")

	// Scenario flags
	flagScenario     = flag.String("scenario", "normal", "Scenario type: normal, fall, ota, bag-on-couch")
	flagFallDelay    = flag.Duration("fall-delay", 5*time.Second, "Delay before fall triggers (fall scenario)")
	flagFallDuration = flag.Duration("fall-duration", 800*time.Millisecond, "Fall duration (fall scenario)")
	flagStillness    = flag.Duration("stillness", 15*time.Second, "Stillness duration after fall (fall scenario)")
	flagOTAVersion   = flag.String("ota-version", "sim-1.1.0", "Target firmware version (OTA scenario)")
	flagOTASize      = flag.Int64("ota-size", 1024*1024, "Firmware size in bytes (OTA scenario)")
	flagOTAFailure   = flag.Bool("ota-failure", false, "Simulate OTA boot failure for rollback (OTA scenario)")
)

// walls is populated from repeated --wall flags
var walls []Wall

// addWall is a custom flag value for repeated --wall flags
type wallFlag struct{}

func (w *wallFlag) String() string { return "" }
func (w *wallFlag) Set(value string) error {
	// Format: x1,y1,x2,y2[:material]
	parts := strings.Split(value, ",")
	if len(parts) < 4 {
		return fmt.Errorf("expected x1,y1,x2,y2[:material] format, got: %s", value)
	}
	x1, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return fmt.Errorf("invalid x1: %w", err)
	}
	y1, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return fmt.Errorf("invalid y1: %w", err)
	}
	x2, err := strconv.ParseFloat(parts[2], 64)
	if err != nil {
		return fmt.Errorf("invalid x2: %w", err)
	}
	y2, err := strconv.ParseFloat(parts[3], 64)
	if err != nil {
		return fmt.Errorf("invalid y2: %w", err)
	}

	// Parse material (default: drywall)
	material := MaterialDrywall
	attenuation := 3.0
	if len(parts) >= 5 {
		material = WallMaterial(strings.ToLower(parts[4]))
		switch material {
		case MaterialDrywall:
			attenuation = 3.0
		case MaterialBrick, MaterialConcrete:
			attenuation = 10.0
		case MaterialGlass:
			attenuation = 2.0
		case MaterialMetal:
			attenuation = 20.0
		default:
			return fmt.Errorf("unknown material: %s (use: drywall, brick, concrete, glass, metal)", parts[4])
		}
	}

	walls = append(walls, Wall{X1: x1, Y1: y1, X2: x2, Y2: y2, Material: material, Attenuation: attenuation})
	return nil
}

// VirtualNode represents a simulated ESP32 node
type VirtualNode struct {
	ID       int
	MAC      [6]byte
	Position Point
	Role     string // "tx", "rx", or "tx_rx"
	Conn     *websocket.Conn
	mu       sync.Mutex
}

// WalkerType defines how a walker moves
type WalkerType string

const (
	WalkerTypeRandomWalk WalkerType = "random"
	WalkerTypePathFollow WalkerType = "path"
	WalkerTypeNodeToNode WalkerType = "node-to-node"
)

// Walker represents a simulated person
type Walker struct {
	ID       int
	Position Point
	Velocity Point
	Speed    float64
	Height   float64
	Type     WalkerType
	Path     []Point        // For path-following mode
	PathIdx  int            // Current position along path
	Nodes    []*VirtualNode // For node-to-node mode
	NodeIdx  int            // Current target node index
}

// Point represents a 3D position
type Point struct {
	X, Y, Z float64
}

// Space represents the room dimensions
type Space struct {
	Width, Depth, Height float64
}

// Bounds returns the bounding box of the space
func (s *Space) Bounds() (minX, minY, minZ, maxX, maxY, maxZ float64) {
	return 0, 0, 0, s.Width, s.Depth, s.Height
}

// WallMaterial defines the type of wall material
type WallMaterial string

const (
	MaterialDrywall  WallMaterial = "drywall"
	MaterialBrick    WallMaterial = "brick"
	MaterialConcrete WallMaterial = "concrete"
	MaterialGlass    WallMaterial = "glass"
	MaterialMetal    WallMaterial = "metal"
)

// Wall represents a wall segment with material properties
type Wall struct {
	X1, Y1, X2, Y2 float64
	Material       WallMaterial
	Attenuation    float64 // dB loss
}

// Stats tracks simulation statistics
type Stats struct {
	FramesSent     atomic.Int64
	FramesPerSec   float64
	StartTime      time.Time
	LastStatsTime  time.Time
	LastFramesSent int64
}

func main() {
	flag.Var(&wallFlag{}, "wall", "Add a wall as x1,y1,x2,y2 (can be repeated)")
	flag.Parse()

	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Printf("[SIM] CSI Simulator CLI starting")

	// Parse space dimensions
	space, err := parseSpace(*flagSpace)
	if err != nil {
		log.Fatalf("[SIM] Invalid space dimensions: %v", err)
	}

	// Initialize random seed
	if *flagSeed == 0 {
		*flagSeed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(*flagSeed))
	log.Printf("[SIM] Random seed: %d", *flagSeed)

	// Validate channel
	if *flagChannel < 1 || *flagChannel > 14 {
		log.Fatalf("[SIM] Invalid channel: %d (must be 1-14)", *flagChannel)
	}

	// Create virtual nodes
	nodes := createVirtualNodes(*flagNodes, space, rng)

	// Create walkers (may be nil for node-to-node mode)
	walkers := createWalkers(*flagWalkers, space, rng)

	// For node-to-node mode, create walkers after nodes exist
	if WalkerType(*flagWalkerType) == WalkerTypeNodeToNode {
		walkers = createNodeToNodeWalkers(*flagWalkers, nodes)
	}

	log.Printf("[SIM] Configuration:")
	log.Printf("[SIM]   Mothership: %s", *flagMothership)
	log.Printf("[SIM]   Scenario: %s", *flagScenario)
	log.Printf("[SIM]   Nodes: %d", *flagNodes)
	log.Printf("[SIM]   Walkers: %d", *flagWalkers)
	log.Printf("[SIM]   Rate: %d Hz", *flagRate)
	log.Printf("[SIM]   Duration: %d s", *flagDuration)
	log.Printf("[SIM]   Space: %.1fx%.1fx%.1f m", space.Width, space.Depth, space.Height)
	log.Printf("[SIM]   Walls: %d", len(walls))
	log.Printf("[SIM]   BLE: %v", *flagBLE)

	// Output GDOP overlay if requested
	if *flagGDOPOverlay {
		if err := outputGDOPOverlay(space, nodes, *flagCellSize); err != nil {
			log.Fatalf("[SIM] Failed to output GDOP overlay: %v", err)
		}
	}

	// Output shopping list if requested
	if *flagShoppingList {
		if err := outputShoppingList(space, nodes); err != nil {
			log.Fatalf("[SIM] Failed to output shopping list: %v", err)
		}
	}

	// If only output was requested, exit here
	if *flagGDOPOverlay || *flagShoppingList {
		if !*flagVerify && *flagDuration == 0 {
			log.Printf("[SIM] Output complete (no simulation requested)")
			os.Exit(0)
		}
	}

	// Create scenario configuration
	scenarioConfig := &ScenarioConfig{
		Type:      ScenarioType(*flagScenario),
		StartedAt: time.Now(),
		FallParams: FallScenarioParams{
			TriggerAfter:      *flagFallDelay,
			DescentDuration:   *flagFallDuration,
			StillnessDuration: *flagStillness,
			MinVelocity:       -1.5, // Below -1.5 m/s threshold
			MinZDrop:          0.8,  // At least 0.8m drop
			EndZ:              0.3,  // Floor level
		},
		OTAParams: OTAScenarioParams{
			UpdateAfter:      10 * time.Second,
			FirmwareSize:     *flagOTASize,
			NewVersion:       *flagOTAVersion,
			RebootDelay:      3 * time.Second,
			BootFailDuration: 30 * time.Second,
			SimulateFailure:  *flagOTAFailure,
		},
	}

	// Create context for shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)

	// Open CSV writer if specified
	var csvWriter *CSVWriter
	if *flagOutputCSV != "" {
		csvWriter, err = NewCSVWriter(*flagOutputCSV)
		if err != nil {
			log.Fatalf("[SIM] Failed to open CSV file: %v", err)
		}
		defer func() { _ = csvWriter.Close() }() //nolint:errcheck
		log.Printf("[SIM] Writing ground truth to %s", *flagOutputCSV)
	}

	// Start statistics reporter
	stats := &Stats{StartTime: time.Now()}
	httpBaseURL := getHTTPBaseURL(*flagMothership)
	go reportStats(ctx, stats, httpBaseURL)

	// Connect all nodes to mothership
	if err := connectNodes(ctx, nodes); err != nil {
		log.Fatalf("[SIM] Failed to connect nodes: %v", err)
	}
	defer closeAllNodes(nodes)

	// Main simulation loop
	simulationComplete := make(chan struct{})
	go runSimulation(ctx, nodes, walkers, space, rng, csvWriter, stats, simulationComplete, scenarioConfig)

	// Wait for completion or interrupt
	var durationTimer <-chan time.Time
	if *flagDuration > 0 {
		durationTimer = time.After(time.Duration(*flagDuration) * time.Second)
	} else {
		// Never timeout if duration is 0
		durationTimer = make(chan time.Time)
	}

	select {
	case <-simulationComplete:
		log.Printf("[SIM] Simulation completed")
	case <-sigChan:
		log.Printf("[SIM] Interrupted by user")
		cancel()
	case <-durationTimer:
		log.Printf("[SIM] Duration elapsed")
		cancel()
	}

	// Verify blob count if requested
	if *flagVerify {
		if err := verifyBlobs(*flagWalkers, walkers, space); err != nil {
			log.Printf("[SIM] Verification FAILED: %v", err)
			os.Exit(1)
		}
		log.Printf("[SIM] Verification PASSED")
	}

	// Print final statistics
	printFinalStats(stats, len(walkers), httpBaseURL)
}

// parseSpace parses space dimensions from WxDxH format
func parseSpace(s string) (*Space, error) {
	parts := strings.Split(s, "x")
	if len(parts) != 3 {
		return nil, fmt.Errorf("expected WxDxH format, got: %s", s)
	}
	width, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return nil, fmt.Errorf("invalid width: %w", err)
	}
	depth, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return nil, fmt.Errorf("invalid depth: %w", err)
	}
	height, err := strconv.ParseFloat(parts[2], 64)
	if err != nil {
		return nil, fmt.Errorf("invalid height: %w", err)
	}
	return &Space{Width: width, Depth: depth, Height: height}, nil
}

// createVirtualNodes creates virtual nodes positioned in the space
func createVirtualNodes(count int, space *Space, rng *rand.Rand) []*VirtualNode {
	nodes := make([]*VirtualNode, count)

	for i := 0; i < count; i++ {
		node := &VirtualNode{
			ID:   i,
			MAC:  generateMAC(i),
			Role: "tx_rx",
		}

		// Distribute nodes around perimeter
		perimeter := 2 * (space.Width + space.Depth)
		pos := float64(i) / float64(count) * perimeter

		if pos < space.Width {
			// Bottom edge
			node.Position = Point{X: pos, Y: 0, Z: 2.0}
		} else if pos < space.Width+space.Depth {
			// Right edge
			node.Position = Point{X: space.Width, Y: pos - space.Width, Z: 2.0}
		} else if pos < 2*space.Width+space.Depth {
			// Top edge
			node.Position = Point{X: space.Width - (pos - space.Width - space.Depth), Y: space.Depth, Z: 2.0}
		} else {
			// Left edge
			node.Position = Point{X: 0, Y: space.Depth - (pos - 2*space.Width - space.Depth), Z: 2.0}
		}

		nodes[i] = node
	}

	return nodes
}

// generateMAC generates a synthetic MAC address for a virtual node
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

// createWalkers creates synthetic walkers
func createWalkers(count int, space *Space, rng *rand.Rand) []*Walker {
	walkerType := WalkerType(*flagWalkerType)

	if walkerType == WalkerTypePathFollow {
		return createPathWalkers(count, space)
	}

	if walkerType == WalkerTypeNodeToNode {
		// Will be created after nodes are initialized
		return nil
	}

	// Default: random walk
	walkers := make([]*Walker, count)

	for i := 0; i < count; i++ {
		walkers[i] = &Walker{
			ID:   i,
			Type: WalkerTypeRandomWalk,
			Position: Point{
				X: rng.Float64() * space.Width,
				Y: rng.Float64() * space.Depth,
				Z: 1.7,
			},
			Velocity: Point{
				X: (rng.Float64() - 0.5) * 0.5,
				Y: (rng.Float64() - 0.5) * 0.5,
				Z: 0,
			},
			Speed:  0.8 + rng.Float64()*0.4,
			Height: 1.7,
		}
	}

	return walkers
}

// createPathWalkers creates walkers that follow predefined paths
func createPathWalkers(count int, space *Space) []*Walker {
	if *flagPathFile == "" {
		log.Printf("[SIM] Path-following mode requires --path-file, using default rectangular path")
		return createDefaultPathWalkers(count, space)
	}

	// Load paths from JSON file
	paths, err := loadPathsFromFile(*flagPathFile)
	if err != nil {
		log.Fatalf("[SIM] Failed to load paths from %s: %v", *flagPathFile, err)
	}

	walkers := make([]*Walker, count)
	for i := 0; i < count; i++ {
		// Use path modulo count to allow fewer paths than walkers
		pathIdx := i % len(paths)
		walkers[i] = &Walker{
			ID:      i,
			Type:    WalkerTypePathFollow,
			Path:    paths[pathIdx],
			PathIdx: 0,
			Speed:   1.0,
			Height:  1.7,
		}
		if len(paths[pathIdx]) > 0 {
			walkers[i].Position = paths[pathIdx][0]
		}
	}

	return walkers
}

// createDefaultPathWalkers creates walkers following a default rectangular perimeter path
func createDefaultPathWalkers(count int, space *Space) []*Walker {
	margin := 0.5
	path := []Point{
		{X: margin, Y: margin, Z: 1.7},
		{X: space.Width - margin, Y: margin, Z: 1.7},
		{X: space.Width - margin, Y: space.Depth - margin, Z: 1.7},
		{X: margin, Y: space.Depth - margin, Z: 1.7},
	}

	walkers := make([]*Walker, count)
	for i := 0; i < count; i++ {
		startIdx := (i * len(path) / count) % len(path)
		walkers[i] = &Walker{
			ID:      i,
			Type:    WalkerTypePathFollow,
			Path:    path,
			PathIdx: startIdx,
			Speed:   0.8 + 0.4*float64(i)/float64(count),
			Height:  1.7,
		}
		if len(path) > 0 {
			walkers[i].Position = path[startIdx]
		}
	}

	return walkers
}

// createNodeToNodeWalkers creates walkers that traverse between virtual nodes
func createNodeToNodeWalkers(count int, nodes []*VirtualNode) []*Walker {
	if len(nodes) < 2 {
		log.Fatalf("[SIM] Node-to-node mode requires at least 2 nodes")
	}

	walkers := make([]*Walker, count)
	for i := 0; i < count; i++ {
		walkers[i] = &Walker{
			ID:      i,
			Type:    WalkerTypeNodeToNode,
			Nodes:   nodes,
			NodeIdx: 1, // Target is second node
			Speed:   1.0,
			Height:  1.7,
		}
		// Start at first node
		if len(nodes) > 0 {
			walkers[i].Position = nodes[0].Position
		}
	}

	return walkers
}

// PathDefinition defines a walker path in JSON format
type PathDefinition struct {
	Waypoints []Point `json:"waypoints"` // Ordered list of points to visit
}

// loadPathsFromFile loads walker paths from a JSON file
func loadPathsFromFile(filename string) ([][]Point, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var paths []PathDefinition
	if err := json.Unmarshal(data, &paths); err != nil {
		return nil, err
	}

	if len(paths) == 0 {
		return nil, fmt.Errorf("no paths defined in file")
	}

	result := make([][]Point, len(paths))
	for i, pathDef := range paths {
		if len(pathDef.Waypoints) == 0 {
			return nil, fmt.Errorf("path %d has no waypoints", i)
		}
		result[i] = pathDef.Waypoints
	}

	log.Printf("[SIM] Loaded %d paths from %s", len(result), filename)
	return result, nil
}

// connectNodes connects all virtual nodes to the mothership.
// Each node gets its own persistent connection with background goroutines
// for ping, health, and message reading.
func connectNodes(ctx context.Context, nodes []*VirtualNode) error {
	// Get or generate token
	token := *flagToken
	if token == "" {
		var err error
		token, err = provisionToken()
		if err != nil {
			return fmt.Errorf("failed to provision token: %w", err)
		}
		log.Printf("[SIM] Auto-provisioned token: %s...", token[:min(16, len(token))])
	}

	// Parse mothership URL
	wsURL, err := url.Parse(*flagMothership)
	if err != nil {
		return fmt.Errorf("invalid mothership URL: %w", err)
	}

	// Convert http(s) to ws(s)
	if wsURL.Scheme == "http" {
		wsURL.Scheme = "ws"
	} else if wsURL.Scheme == "https" {
		wsURL.Scheme = "wss"
	}

	errChan := make(chan error, len(nodes))

	for _, node := range nodes {
		// Add node WS path if needed
		nodeURL := wsURL.String()
		if !strings.Contains(nodeURL, "/ws/") && !strings.HasSuffix(nodeURL, "/ws") {
			if strings.HasSuffix(nodeURL, "/") {
				nodeURL = nodeURL + "ws"
			} else {
				nodeURL = nodeURL + "/ws"
			}
		}

		headers := http.Header{}
		headers.Set("X-Spaxel-Token", token)

		log.Printf("[SIM] Node %d connecting to %s", node.ID, nodeURL)

		conn, resp, err := websocket.DefaultDialer.DialContext(ctx, nodeURL, headers)
		if err != nil {
			if resp != nil {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("node %d dial failed: %w (status %d: %s)", node.ID, err, resp.StatusCode, string(body))
			}
			return fmt.Errorf("node %d dial failed: %w", node.ID, err)
		}

		node.Conn = conn
		log.Printf("[SIM] Node %d connected", node.ID)

		// Send hello message
		hello := map[string]interface{}{
			"type":             "hello",
			"mac":              macToString(node.MAC),
			"firmware_version": "sim-1.0.0",
			"capabilities":     []string{"csi", "tx", "rx"},
			"chip":             "ESP32-S3",
			"flash_mb":         16,
			"uptime_ms":        1000,
			"wifi_rssi":        -45,
			"ip":               fmt.Sprintf("127.0.0.%d", node.ID+2),
		}

		helloBytes, err := json.Marshal(hello)
		if err != nil {
			conn.Close() //nolint:errcheck
			return fmt.Errorf("node %d marshal hello: %w", node.ID, err)
		}

		node.mu.Lock()
		err = conn.WriteMessage(websocket.TextMessage, helloBytes)
		node.mu.Unlock()

		if err != nil {
			conn.Close() //nolint:errcheck
			return fmt.Errorf("node %d send hello: %w", node.ID, err)
		}

		// Wait for role assignment
		conn.SetReadDeadline(time.Now().Add(10 * time.Second)) //nolint:errcheck
		_, message, err := conn.ReadMessage()
		if err != nil {
			conn.Close() //nolint:errcheck
			return fmt.Errorf("node %d read role: %w", node.ID, err)
		}

		var roleMsg map[string]interface{}
		if err := json.Unmarshal(message, &roleMsg); err != nil {
			conn.Close() //nolint:errcheck
			return fmt.Errorf("node %d parse role: %w", node.ID, err)
		}

		if roleMsg["type"] == "reject" {
			conn.Close() //nolint:errcheck
			return fmt.Errorf("node %d rejected: %v", node.ID, roleMsg["reason"])
		}

		log.Printf("[SIM] Node %d received role: %v", node.ID, roleMsg["role"])

		// Start background goroutines for this connection
		startTime := time.Now()
		go node.pingLoop(ctx)
		go node.healthLoop(ctx, startTime)
		go node.readLoop(ctx, errChan)
	}

	return nil
}

// provisionToken provisions a token. Tries the mothership API first,
// falls back to a synthetic HMAC token.
func provisionToken() (string, error) {
	// Parse mothership URL to get HTTP endpoint
	wsURL, err := url.Parse(*flagMothership)
	if err != nil {
		return "", fmt.Errorf("invalid mothership URL: %w", err)
	}

	httpURL := *wsURL
	if httpURL.Scheme == "ws" {
		httpURL.Scheme = "http"
	} else if httpURL.Scheme == "wss" {
		httpURL.Scheme = "https"
	}

	// Trim /ws suffix to get base URL
	baseURL := strings.TrimSuffix(httpURL.String(), "/ws")
	baseURL = strings.TrimSuffix(baseURL, "/")
	provisionURL := baseURL + "/api/provision"

	// Try POST /api/provision with synthetic credentials
	body := strings.NewReader(`{"mac":"AA:BB:CC:00:00:00"}`)
	resp, err := http.Post(provisionURL, "application/json", body)
	if err == nil && resp.StatusCode == http.StatusOK {
		var result map[string]interface{}
		if json.NewDecoder(resp.Body).Decode(&result) == nil {
			_ = resp.Body.Close()
			if token, ok := result["node_token"].(string); ok && token != "" {
				return token, nil
			}
		}
	}
	if resp != nil {
		_ = resp.Body.Close()
	}

	// Fallback: generate synthetic token
	h := hmac.New(sha256.New, []byte("sim-install-secret"))
	h.Write([]byte("sim-node"))
	return fmt.Sprintf("%064x", h.Sum(nil)), nil
}

// closeAllNodes closes all node WebSocket connections
func closeAllNodes(nodes []*VirtualNode) {
	for _, node := range nodes {
		if node.Conn != nil {
			node.mu.Lock()
			node.Conn.WriteMessage(websocket.CloseMessage, //nolint:errcheck
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, "sim shutdown"))
			node.Conn.Close() //nolint:errcheck
			node.mu.Unlock()
		}
	}
}

// macToString converts a 6-byte MAC to colon-separated hex
func macToString(mac [6]byte) string {
	return fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
		mac[0], mac[1], mac[2], mac[3], mac[4], mac[5])
}

// pingLoop sends WebSocket pings
func (n *VirtualNode) pingLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n.mu.Lock()
			err := n.Conn.WriteMessage(websocket.PingMessage, nil)
			n.mu.Unlock()

			if err != nil {
				log.Printf("[SIM] Node %d ping failed: %v", n.ID, err)
				return
			}
		}
	}
}

// healthLoop sends periodic health messages
func (n *VirtualNode) healthLoop(ctx context.Context, startTime time.Time) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			health := map[string]interface{}{
				"type":            "health",
				"mac":             macToString(n.MAC),
				"timestamp_ms":    time.Now().UnixMilli(),
				"free_heap_bytes": 200000,
				"wifi_rssi_dbm":   -45,
				"uptime_ms":       time.Since(startTime).Milliseconds(),
				"csi_rate_hz":     *flagRate,
				"wifi_channel":    *flagChannel,
			}

			healthBytes, err := json.Marshal(health)
			if err != nil {
				log.Printf("[SIM] Node %d marshal health: %v", n.ID, err)
				continue
			}

			n.mu.Lock()
			err = n.Conn.WriteMessage(websocket.TextMessage, healthBytes)
			n.mu.Unlock()

			if err != nil {
				log.Printf("[SIM] Node %d send health failed: %v", n.ID, err)
				return
			}
		}
	}
}

// readLoop reads downstream messages from the WebSocket
func (n *VirtualNode) readLoop(ctx context.Context, errChan chan<- error) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		n.mu.Lock()
		conn := n.Conn
		n.mu.Unlock()

		if conn == nil {
			return
		}

		conn.SetReadDeadline(time.Now().Add(60 * time.Second)) //nolint:errcheck
		_, message, err := conn.ReadMessage()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if websocket.IsCloseError(err) {
				log.Printf("[SIM] Node %d connection closed", n.ID)
				return
			}
			log.Printf("[SIM] Node %d read error: %v", n.ID, err)
			return
		}

		var msg map[string]interface{}
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		msgType, ok := msg["type"].(string)
		if !ok {
			continue
		}

		switch msgType {
		case "role":
			log.Printf("[SIM] Node %d role update: %v", n.ID, msg["role"])
		case "config":
			log.Printf("[SIM] Node %d config update: %v", n.ID, msg)
		case "reject":
			errChan <- fmt.Errorf("node %d rejected: %v", n.ID, msg["reason"])
			return
		case "shutdown":
			log.Printf("[SIM] Node %d received shutdown", n.ID)
			return
		}
	}
}

// runSimulation runs the main CSI generation loop
func runSimulation(ctx context.Context, nodes []*VirtualNode, walkers []*Walker, space *Space, rng *rand.Rand, csvWriter *CSVWriter, stats *Stats, done chan<- struct{}, scenario *ScenarioConfig) {
	defer close(done)

	ticker := time.NewTicker(time.Duration(1000/(*flagRate)) * time.Millisecond)
	defer ticker.Stop()

	frameNum := 0
	lastBLETime := time.Now()

	// Initialize fall scenario state
	var fallState *FallScenarioState
	if scenario.Type == ScenarioFall || scenario.Type == ScenarioBagOnCouch {
		if len(walkers) > 0 {
			fallState = &FallScenarioState{
				Walker: walkers[0],
				State:  "walking",
			}
			if scenario.Type == ScenarioBagOnCouch {
				// For bag-on-couch, start with a lower position
				walkers[0].Position.Z = 1.0
				walkers[0].Velocity.Z = -0.2 // Slow descent
			}
		}
	}

	// Track scenario timing
	scenarioStarted := false

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Handle fall scenario timing
			if fallState != nil && !scenarioStarted && time.Since(scenario.StartedAt) >= scenario.FallParams.TriggerAfter {
				if scenario.Type == ScenarioFall {
					fallState.StartFall(scenario.FallParams)
					scenarioStarted = true
				}
			}

			// Update walker positions
			if fallState != nil {
				dt := 1.0 / float64(*flagRate)
				fallState.UpdateForFallScenario(dt, scenario.FallParams, space, rng)
			} else {
				updateWalkers(walkers, space, rng)
			}

			// Write to CSV
			if csvWriter != nil {
				csvWriter.WriteRow(walkers, nodes, walls)
			}

			// Send CSI frames for each node pair
			for _, txNode := range nodes {
				for _, rxNode := range nodes {
					if txNode.ID == rxNode.ID {
						continue
					}

					frame := generateCSIFrame(txNode, rxNode, walkers, walls, frameNum, rng)

					txNode.mu.Lock()
					err := txNode.Conn.WriteMessage(websocket.BinaryMessage, frame)
					txNode.mu.Unlock()

					if err != nil {
						log.Printf("[SIM] Node %d send CSI failed: %v", txNode.ID, err)
						continue
					}

					stats.FramesSent.Add(1)
				}
			}

			// Send BLE messages if enabled
			if *flagBLE && time.Since(lastBLETime) > 5*time.Second {
				sendBLEMessages(nodes, walkers)
				lastBLETime = time.Now()
			}

			frameNum++
		}
	}
}

// updateWalkers updates walker positions based on their movement type
func updateWalkers(walkers []*Walker, space *Space, rng *rand.Rand) {
	dt := 1.0 / float64(*flagRate)

	for _, walker := range walkers {
		switch walker.Type {
		case WalkerTypePathFollow:
			updatePathWalker(walker, dt)
		case WalkerTypeNodeToNode:
			updateNodeToNodeWalker(walker, dt, space)
		case WalkerTypeRandomWalk:
			updateRandomWalker(walker, dt, space, rng)
		}
	}
}

// updateRandomWalker implements random walk motion
func updateRandomWalker(walker *Walker, dt float64, space *Space, rng *rand.Rand) {
	walker.Position.X += walker.Velocity.X * dt
	walker.Position.Y += walker.Velocity.Y * dt

	// Bounce off walls
	margin := 0.2
	if walker.Position.X < margin {
		walker.Position.X = margin
		walker.Velocity.X *= -1
	}
	if walker.Position.X > space.Width-margin {
		walker.Position.X = space.Width - margin
		walker.Velocity.X *= -1
	}
	if walker.Position.Y < margin {
		walker.Position.Y = margin
		walker.Velocity.Y *= -1
	}
	if walker.Position.Y > space.Depth-margin {
		walker.Position.Y = space.Depth - margin
		walker.Velocity.Y *= -1
	}

	// Random velocity perturbation
	perturbation := 0.1
	walker.Velocity.X += (rng.Float64() - 0.5) * perturbation
	walker.Velocity.Y += (rng.Float64() - 0.5) * perturbation

	// Clamp velocity
	speed := walker.Speed * (0.5 + rng.Float64()*0.5)
	currentSpeed := math.Sqrt(walker.Velocity.X*walker.Velocity.X + walker.Velocity.Y*walker.Velocity.Y)
	if currentSpeed > 0 {
		walker.Velocity.X = (walker.Velocity.X / currentSpeed) * speed
		walker.Velocity.Y = (walker.Velocity.Y / currentSpeed) * speed
	}

	walker.Position.Z = walker.Height
}

// updatePathWalker implements path-following motion
func updatePathWalker(walker *Walker, dt float64) {
	if len(walker.Path) == 0 {
		return
	}

	target := walker.Path[walker.PathIdx]
	dx := target.X - walker.Position.X
	dy := target.Y - walker.Position.Y
	dz := target.Z - walker.Position.Z
	dist := math.Sqrt(dx*dx + dy*dy + dz*dz)

	// If very close to target, move to next waypoint
	if dist < 0.1 {
		walker.PathIdx = (walker.PathIdx + 1) % len(walker.Path)
		return
	}

	// Move towards target at constant speed
	moveDist := walker.Speed * dt
	if moveDist > dist {
		moveDist = dist
	}

	t := moveDist / dist
	walker.Position.X += dx * t
	walker.Position.Y += dy * t

	// Update velocity vector for consistency
	if dist > 0 {
		walker.Velocity.X = (dx / dist) * walker.Speed
		walker.Velocity.Y = (dy / dist) * walker.Speed
		walker.Velocity.Z = (dz / dist) * walker.Speed
	}
}

// updateNodeToNodeWalker implements traversal between virtual nodes
func updateNodeToNodeWalker(walker *Walker, dt float64, space *Space) {
	if len(walker.Nodes) < 2 {
		updateRandomWalker(walker, dt, space, rand.New(rand.NewSource(time.Now().UnixNano())))
		return
	}

	targetNode := walker.Nodes[walker.NodeIdx]
	targetPos := targetNode.Position

	dx := targetPos.X - walker.Position.X
	dy := targetPos.Y - walker.Position.Y
	dz := targetPos.Z - walker.Position.Z
	dist := math.Sqrt(dx*dx + dy*dy + dz*dz)

	// Check if we've arrived at the target node (horizontal distance)
	horizontalDist := math.Sqrt(dx*dx + dy*dy)
	if horizontalDist < 0.3 {
		// Move to next node
		walker.NodeIdx = (walker.NodeIdx + 1) % len(walker.Nodes)
		return
	}

	// Move towards target node
	moveDist := walker.Speed * dt
	if moveDist > horizontalDist {
		moveDist = horizontalDist
	}

	t := moveDist / horizontalDist
	walker.Position.X += dx * t
	walker.Position.Y += dy * t

	// Update velocity vector
	if horizontalDist > 0 {
		walker.Velocity.X = (dx / horizontalDist) * walker.Speed
		walker.Velocity.Y = (dy / horizontalDist) * walker.Speed
		walker.Velocity.Z = (dz / dist) * walker.Speed
	}

	walker.Position.Z = walker.Height
}

// sendBLEMessages sends synthetic BLE scan results
func sendBLEMessages(nodes []*VirtualNode, walkers []*Walker) {
	for _, node := range nodes {
		devices := make([]map[string]interface{}, 0)

		for _, walker := range walkers {
			dx := walker.Position.X - node.Position.X
			dy := walker.Position.Y - node.Position.Y
			dz := walker.Position.Z - node.Position.Z
			dist := math.Sqrt(dx*dx + dy*dy + dz*dz)

			rssi := -50.0 - 20.0*math.Log10(dist/1.0)
			if rssi < -90 {
				rssi = -90
			}

			devices = append(devices, map[string]interface{}{
				"addr": fmt.Sprintf("AA:BB:CC:DD:EE:%02X", walker.ID),
				"rssi": int(rssi),
				"name": fmt.Sprintf("sim-person-%d", walker.ID),
			})
		}

		if len(devices) == 0 {
			continue
		}

		bleMsg := map[string]interface{}{
			"type":         "ble",
			"mac":          macToString(node.MAC),
			"timestamp_ms": time.Now().UnixMilli(),
			"devices":      devices,
		}

		bleBytes, err := json.Marshal(bleMsg)
		if err != nil {
			log.Printf("[SIM] Node %d marshal BLE: %v", node.ID, err)
			continue
		}

		node.mu.Lock()
		err = node.Conn.WriteMessage(websocket.TextMessage, bleBytes)
		node.mu.Unlock()

		if err != nil {
			log.Printf("[SIM] Node %d send BLE failed: %v", node.ID, err)
		}
	}
}

// reportStats periodically prints statistics including blob counts
func reportStats(ctx context.Context, stats *Stats, httpBaseURL string) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			elapsed := now.Sub(stats.StartTime).Seconds()
			framesSent := stats.FramesSent.Load()

			// Fetch blob count from mothership
			blobCount := fetchBlobCount(httpBaseURL)

			if elapsed > 0 {
				fps := float64(framesSent) / elapsed
				log.Printf("[SIM] Stats: frames=%d fps=%.1f blobs=%d elapsed=%.1fs", framesSent, fps, blobCount, elapsed)
			}
		}
	}
}

// fetchBlobCount queries the mothership for current blob count
func fetchBlobCount(baseURL string) int {
	if baseURL == "" {
		return 0
	}

	// Ensure we have the correct API endpoint
	blobsURL := strings.TrimSuffix(baseURL, "/ws")
	blobsURL = strings.TrimSuffix(blobsURL, "/")
	blobsURL += "/api/blobs"

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(blobsURL)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0
	}

	var blobs []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&blobs); err != nil {
		return 0
	}

	return len(blobs)
}

// printFinalStats prints final simulation statistics
func printFinalStats(stats *Stats, walkerCount int, httpBaseURL string) {
	elapsed := time.Since(stats.StartTime).Seconds()
	framesSent := stats.FramesSent.Load()

	// Fetch final blob count
	blobCount := fetchBlobCount(httpBaseURL)

	log.Printf("[SIM] Final Statistics:")
	log.Printf("[SIM]   Frames sent: %d", framesSent)
	log.Printf("[SIM]   Duration: %.1f seconds", elapsed)
	if elapsed > 0 {
		log.Printf("[SIM]   Average FPS: %.1f", float64(framesSent)/elapsed)
	}
	log.Printf("[SIM]   Walkers: %d", walkerCount)
	log.Printf("[SIM]   Blobs detected: %d", blobCount)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// toSimulatorPoint converts a main.Point to simulator.Point
func toSimulatorPoint(p Point) simulator.Point {
	return simulator.Point{X: p.X, Y: p.Y, Z: p.Z}
}

// toSimulatorSpace converts a main.Space to simulator.Space
func toSimulatorSpace(s *Space) *simulator.Space {
	return &simulator.Space{
		ID:   "sim-space",
		Name: "Simulation Space",
		Rooms: []simulator.Room{{
			ID:   "room-1",
			Name: "Main Room",
			MinX: 0, MinY: 0, MinZ: 0,
			MaxX: s.Width, MaxY: s.Depth, MaxZ: s.Height,
		}},
	}
}

// getHTTPBaseURL converts a WebSocket URL to an HTTP base URL for API calls
func getHTTPBaseURL(wsURL string) string {
	u, err := url.Parse(wsURL)
	if err != nil {
		return ""
	}

	if u.Scheme == "ws" {
		u.Scheme = "http"
	} else if u.Scheme == "wss" {
		u.Scheme = "https"
	}

	// Remove /ws path if present
	path := u.Path
	if strings.Contains(path, "/ws") {
		if idx := strings.Index(path, "/ws"); idx != -1 {
			u.Path = path[:idx]
		}
	} else if strings.HasSuffix(path, "/ws") {
		u.Path = strings.TrimSuffix(path, "/ws")
	}

	// Ensure trailing slash is removed for consistent URL construction
	u.Path = strings.TrimSuffix(u.Path, "/")

	return u.String()
}

// outputGDOPOverlay computes and outputs GDOP overlay data as JSON
func outputGDOPOverlay(space *Space, nodes []*VirtualNode, cellSize float64) error {
	// Convert to simulator types
	simSpace := toSimulatorSpace(space)
	nodeSet := simulator.NewNodeSet()
	for _, vn := range nodes {
		simPos := toSimulatorPoint(vn.Position)
		nodeID := fmt.Sprintf("node-%d", vn.ID)
		nodeName := fmt.Sprintf("Node %d", vn.ID)
		nodeSet.AddVirtualNode(nodeID, nodeName, simPos)
	}

	// Generate links
	links := simulator.GenerateAllLinks(nodeSet)

	if len(links) < 2 {
		return fmt.Errorf("need at least 2 nodes for GDOP computation")
	}

	// Get space bounds from simulator space
	minX, minY, _, maxX, maxY, _ := simSpace.Bounds()

	// Create GDOP computer
	config := simulator.GridConfig{
		MinX:     minX,
		MinY:     minY,
		Width:    maxX - minX,
		Depth:    maxY - minY,
		CellSize: cellSize,
	}
	gdopComp := simulator.NewGDOPComputer(links, config)

	// Compute GDOP for all cells
	results := gdopComp.ComputeAll()

	// Convert to heatmap data
	heatmapData := gdopComp.ToHeatmapData(results)

	// Get average GDOP, handling infinity
	avgGDOP := gdopComp.AverageGDOP(results)
	avgGDOPOutput := "Infinity"
	if !math.IsInf(avgGDOP, 0) {
		avgGDOPOutput = fmt.Sprintf("%.2f", avgGDOP)
	}

	// Output JSON
	output := map[string]interface{}{
		"type": "gdop_overlay",
		"space_dimensions": map[string]float64{
			"width_m":  space.Width,
			"depth_m":  space.Depth,
			"height_m": space.Height,
		},
		"grid_dimensions": []int{heatmapData.Width, heatmapData.Depth, 1},
		"cell_size_m":     cellSize,
		"origin":          map[string]float64{"x": heatmapData.OriginX, "y": heatmapData.OriginY},
		"gdop_values":     heatmapData.GDOPValues,
		"qualities":       heatmapData.Qualities,
		"colors":          heatmapData.Colors,
		"accuracy_map":    heatmapData.AccuracyMap,
		"coverage_score":  gdopComp.CoverageScore(results),
		"average_gdop":    avgGDOPOutput,
		"quality_counts":  gdopComp.QualityCounts(results),
		"dead_zones":      gdopComp.FindDeadZones(results),
		"links":           links,
		"timestamp":       time.Now().Format(time.RFC3339),
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

// outputShoppingList generates and outputs the shopping list as JSON
func outputShoppingList(space *Space, nodes []*VirtualNode) error {
	// Convert to simulator types
	simSpace := toSimulatorSpace(space)
	nodeSet := simulator.NewNodeSet()
	for _, vn := range nodes {
		simPos := toSimulatorPoint(vn.Position)
		nodeID := fmt.Sprintf("node-%d", vn.ID)
		nodeName := fmt.Sprintf("Node %d", vn.ID)
		nodeSet.AddVirtualNode(nodeID, nodeName, simPos)
	}

	// Generate basic coverage info (no walkers for static shopping list)
	links := simulator.GenerateAllLinks(nodeSet)
	minX, minY, _, maxX, maxY, _ := simSpace.Bounds()

	gdopComp := simulator.NewGDOPComputer(links, simulator.GridConfig{
		MinX:     minX,
		MinY:     minY,
		Width:    maxX - minX,
		Depth:    maxY - minY,
		CellSize: 0.2,
	})
	results := gdopComp.ComputeAll()
	coverageScore := gdopComp.CoverageScore(results)

	// Create accuracy report (default values since no simulation run)
	accuracy := simulator.AccuracyReport{
		MedianError: 1.0,
		MeanError:   1.2,
		MaxError:    2.5,
		P95Error:    2.0,
	}

	// Generate shopping list
	shoppingList := simulator.GenerateShoppingListFromResults(simSpace, nodeSet, coverageScore, accuracy)

	// Output JSON
	output := map[string]interface{}{
		"type":      "shopping_list",
		"list":      shoppingList,
		"timestamp": time.Now().Format(time.RFC3339),
		"note":      "This is a pre-deployment estimate. Actual accuracy may vary based on environment.",
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}
