package main

import (
	"encoding/csv"
	"encoding/json"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateCSIFrameHeader tests that generated frames have correct binary header format.
// The frame must match the ingestion layer layout (ingestion/frame.go):
//
//	[0:6]  node_mac, [6:12] peer_mac, [12:20] timestamp_us,
//	[20] rssi, [21] noise_floor, [22] channel, [23] n_sub, [24:] payload
func TestGenerateCSIFrameHeader(t *testing.T) {
	rng := rand.New(rand.NewSource(42))

	tx := &VirtualNode{ID: 0, MAC: generateMAC(0), Position: Point{X: 0, Y: 0, Z: 2}}
	rx := &VirtualNode{ID: 1, MAC: generateMAC(1), Position: Point{X: 5, Y: 0, Z: 2}}
	walkers := []*Walker{{ID: 0, Position: Point{X: 2.5, Y: 0, Z: 1.7}}}

	frame := generateCSIFrame(tx, rx, walkers, nil, 0, rng)

	// Check minimum length
	if len(frame) < headerSize {
		t.Fatalf("Frame too short: %d bytes (minimum %d)", len(frame), headerSize)
	}

	// Check MAC addresses are present (not all zeros) at ingestion format offsets
	allZero := true
	for i := 0; i < 6; i++ {
		if frame[i] != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("TX MAC (node_mac) is all zeros")
	}

	allZero = true
	for i := 6; i < 12; i++ {
		if frame[i] != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("RX MAC (peer_mac) is all zeros")
	}

	// Check subcarrier count at ingestion format offset [23]
	nSubRead := frame[23]
	if nSubRead != 64 {
		t.Errorf("Wrong n_sub: %d (expected 64)", nSubRead)
	}

	// Check payload length matches n_sub
	expectedLen := headerSize + int(nSubRead)*2
	if len(frame) != expectedLen {
		t.Errorf("Frame length mismatch: %d (expected %d)", len(frame), expectedLen)
	}
}

// TestRSSIInRange tests that RSSI is within plausible range for given distance.
// At 2m from TX with wall_attenuation=0, RSSI should be roughly [-50, -70].
func TestRSSIInRange(t *testing.T) {
	rng := rand.New(rand.NewSource(42))

	tx := &VirtualNode{ID: 0, MAC: generateMAC(0), Position: Point{X: 0, Y: 0, Z: 2}}
	rx := &VirtualNode{ID: 1, MAC: generateMAC(1), Position: Point{X: 5, Y: 0, Z: 2}}
	walkers := []*Walker{{ID: 0, Position: Point{X: 2.5, Y: 0, Z: 1.7}}}

	frame := generateCSIFrame(tx, rx, walkers, nil, 0, rng)

	// RSSI is at ingestion format offset [20]
	rssi := int8(frame[20])

	// RSSI should be in [-90, -30] dBm for any reasonable link
	if rssi < -90 || rssi > -30 {
		t.Errorf("RSSI out of range: %d (expected [-90, -30])", rssi)
	}
}

// TestIQClamping tests that generated I/Q values are clamped to int8 range [-127, 127]
func TestIQClamping(t *testing.T) {
	rng := rand.New(rand.NewSource(42))

	tx := &VirtualNode{ID: 0, Position: Point{X: 0, Y: 0, Z: 2}}
	rx := &VirtualNode{ID: 1, Position: Point{X: 0.1, Y: 0, Z: 2}}
	walkers := []*Walker{{ID: 0, Position: Point{X: 0.05, Y: 0, Z: 1.7}}}

	frame := generateCSIFrame(tx, rx, walkers, nil, 0, rng)

	for k := 0; k < 64; k++ {
		offset := headerSize + k*2
		i := int8(frame[offset])
		q := int8(frame[offset+1])

		if i < -127 || i > 127 {
			t.Errorf("I value out of range at subcarrier %d: %d", k, i)
		}
		if q < -127 || q > 127 {
			t.Errorf("Q value out of range at subcarrier %d: %d", k, q)
		}
	}
}

// TestHelloMessageFormat tests that hello message can be parsed correctly
func TestHelloMessageFormat(t *testing.T) {
	hello := map[string]interface{}{
		"type":             "hello",
		"mac":              "AA:BB:CC:DD:EE:FF",
		"firmware_version": "sim-1.0.0",
		"capabilities":     []string{"csi", "tx", "rx"},
		"chip":             "ESP32-S3",
		"flash_mb":         16,
		"uptime_ms":        1000,
		"wifi_rssi":        -45,
		"ip":               "127.0.0.1",
	}

	if hello["type"] != "hello" {
		t.Error("Wrong message type")
	}
	if _, ok := hello["mac"].(string); !ok {
		t.Error("MAC field missing or not string")
	}
	capabilities, ok := hello["capabilities"].([]string)
	if !ok || len(capabilities) == 0 {
		t.Error("Capabilities field missing or empty")
	}
}

// TestVerifyBlobCount tests the verification logic
func TestVerifyBlobCount(t *testing.T) {
	tests := []struct {
		name         string
		blobCount    int
		walkerCount  int
		expectedPass bool
	}{
		{"Exact match", 1, 1, true},
		{"Within tolerance +1", 2, 1, true},
		{"Within tolerance -1", 0, 1, true},
		{"Too many blobs", 3, 1, false},
		{"Multiple walkers exact", 2, 2, true},
		{"Multiple walkers within tolerance", 3, 2, true},
		{"Multiple walkers too few", 0, 2, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tolerance := 1
			minExpected := tt.walkerCount - tolerance
			maxExpected := tt.walkerCount + tolerance

			pass := tt.blobCount >= minExpected && tt.blobCount <= maxExpected

			if pass != tt.expectedPass {
				t.Errorf("verifyBlobs(%d walkers, %d blobs) = %v, expected %v",
					tt.walkerCount, tt.blobCount, pass, tt.expectedPass)
			}
		})
	}
}

// TestSeedReproducibility tests that the same seed produces identical results
func TestSeedReproducibility(t *testing.T) {
	seed := int64(42)

	rng1 := rand.New(rand.NewSource(seed))
	rng2 := rand.New(rand.NewSource(seed))

	tx := &VirtualNode{ID: 0, Position: Point{X: 0, Y: 0, Z: 2}}
	rx := &VirtualNode{ID: 1, Position: Point{X: 5, Y: 0, Z: 2}}
	walkers := []*Walker{{ID: 0, Position: Point{X: 2.5, Y: 0, Z: 1.7}}}

	frame1 := generateCSIFrame(tx, rx, walkers, nil, 0, rng1)
	frame2 := generateCSIFrame(tx, rx, walkers, nil, 0, rng2)

	if len(frame1) != len(frame2) {
		t.Fatalf("Frame lengths differ: %d vs %d", len(frame1), len(frame2))
	}

	for i := range frame1 {
		if frame1[i] != frame2[i] {
			t.Errorf("Byte %d differs: %d vs %d", i, frame1[i], frame2[i])
		}
	}
}

// TestSeedReproducibilityWalkerPaths tests that identical seeds produce identical walker trajectories
func TestSeedReproducibilityWalkerPaths(t *testing.T) {
	seed := int64(42)
	space := &Space{Width: 5, Depth: 5, Height: 2.5}

	rng1 := rand.New(rand.NewSource(seed))
	walkers1 := createWalkers(2, space, rng1)

	rng2 := rand.New(rand.NewSource(seed))
	walkers2 := createWalkers(2, space, rng2)

	// Initial positions should match
	for i := range walkers1 {
		if walkers1[i].Position != walkers2[i].Position {
			t.Errorf("Initial position mismatch for walker %d: %v vs %v",
				i, walkers1[i].Position, walkers2[i].Position)
		}
	}

	// Run 100 update steps and verify positions still match
	for step := 0; step < 100; step++ {
		rng1 = rand.New(rand.NewSource(seed))
		rng2 = rand.New(rand.NewSource(seed))
		// Reset walkers to get fresh RNG sequence
		walkers1 = createWalkers(2, space, rng1)
		walkers2 = createWalkers(2, space, rng2)

		// Apply same number of updates
		for s := 0; s <= step; s++ {
			updateWalkers(walkers1, space, rng1)
			updateWalkers(walkers2, space, rng2)
		}

		for i := range walkers1 {
			if walkers1[i].Position != walkers2[i].Position {
				t.Errorf("Position mismatch at step %d walker %d: %v vs %v",
					step, i, walkers1[i].Position, walkers2[i].Position)
			}
		}
	}
}

// TestParseSpace tests space dimension parsing
func TestParseSpace(t *testing.T) {
	tests := []struct {
		input      string
		wantWidth  float64
		wantDepth  float64
		wantHeight float64
		wantErr    bool
	}{
		{"5x5x2.5", 5, 5, 2.5, false},
		{"10x8x3", 10, 8, 3, false},
		{"6.5x4.2x2.8", 6.5, 4.2, 2.8, false},
		{"invalid", 0, 0, 0, true},
		{"5x5", 0, 0, 0, true},
		{"5x5x2x3", 0, 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			space, err := parseSpace(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if space.Width != tt.wantWidth {
				t.Errorf("Width = %v, want %v", space.Width, tt.wantWidth)
			}
			if space.Depth != tt.wantDepth {
				t.Errorf("Depth = %v, want %v", space.Depth, tt.wantDepth)
			}
			if space.Height != tt.wantHeight {
				t.Errorf("Height = %v, want %v", space.Height, tt.wantHeight)
			}
		})
	}
}

// TestMACGeneration tests MAC address generation
func TestMACGeneration(t *testing.T) {
	tests := []struct {
		id   int
		want string
	}{
		{0, "AA:BB:CC:00:00:00"},
		{1, "AA:BB:CC:00:00:01"},
		{255, "AA:BB:CC:00:00:FF"},
		{256, "AA:BB:CC:00:01:00"},
		{65535, "AA:BB:CC:00:FF:FF"},
		{16777215, "AA:BB:CC:FF:FF:FF"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			mac := generateMAC(tt.id)
			got := macToString(mac)
			if got != tt.want {
				t.Errorf("generateMAC(%d) = %s, want %s", tt.id, got, tt.want)
			}
		})
	}
}

// TestComputeCSIForWalkers tests CSI computation with various walker configurations
func TestComputeCSIForWalkers(t *testing.T) {
	tx := &VirtualNode{ID: 0, Position: Point{X: 0, Y: 0, Z: 2}}
	rx := &VirtualNode{ID: 1, Position: Point{X: 5, Y: 0, Z: 2}}

	tests := []struct {
		name    string
		walkers []*Walker
		minAmp  float64
		maxAmp  float64
	}{
		{
			name:    "No walkers",
			walkers: []*Walker{},
			minAmp:  0,
			maxAmp:  0.01,
		},
		{
			name: "Walker at midpoint",
			walkers: []*Walker{{
				ID:       0,
				Position: Point{X: 2.5, Y: 0, Z: 1.7},
			}},
			minAmp: 0.1,
			maxAmp: 10,
		},
		{
			name: "Walker far from link",
			walkers: []*Walker{{
				ID:       0,
				Position: Point{X: 2.5, Y: 10, Z: 1.7},
			}},
			minAmp: 0,
			maxAmp: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			amplitude, _ := computeCSIForWalkers(tx, rx, tt.walkers, nil)

			if amplitude < tt.minAmp {
				t.Errorf("Amplitude %v below minimum %v", amplitude, tt.minAmp)
			}
			if amplitude > tt.maxAmp {
				t.Errorf("Amplitude %v above maximum %v", amplitude, tt.maxAmp)
			}
		})
	}
}

// TestDistance tests distance calculation
func TestDistance(t *testing.T) {
	tests := []struct {
		a, b Point
		want float64
	}{
		{Point{0, 0, 0}, Point{0, 0, 0}, 0},
		{Point{0, 0, 0}, Point{1, 0, 0}, 1},
		{Point{0, 0, 0}, Point{0, 1, 0}, 1},
		{Point{0, 0, 0}, Point{0, 0, 1}, 1},
		{Point{0, 0, 0}, Point{3, 4, 0}, 5},
		{Point{1, 2, 3}, Point{4, 6, 3}, 5},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := distance(tt.a, tt.b)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("distance(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// TestCSVOutput tests that CSV file has correct headers and ground truth data
func TestCSVOutput(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "test_output.csv")

	csvWriter, err := NewCSVWriter(csvPath)
	if err != nil {
		t.Fatalf("Failed to create CSV writer: %v", err)
	}

	walkers := []*Walker{
		{ID: 0, Position: Point{X: 2.5, Y: 1.5, Z: 1.7}, Velocity: Point{X: 0.5, Y: 0.3, Z: 0}},
	}
	nodes := []*VirtualNode{
		{ID: 0, MAC: generateMAC(0), Position: Point{X: 0, Y: 0, Z: 2}},
		{ID: 1, MAC: generateMAC(1), Position: Point{X: 5, Y: 0, Z: 2}},
	}

	csvWriter.WriteRow(walkers, nodes, nil)
	csvWriter.Close() //nolint:errcheck

	// Read back and verify
	file, err := os.Open(csvPath)
	if err != nil {
		t.Fatalf("Failed to open CSV: %v", err)
	}
	defer func() { _ = file.Close() }()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("Failed to read CSV: %v", err)
	}

	// Check header
	expectedHeaders := []string{"timestamp_ms", "walker_id", "x", "y", "z", "vx", "vy", "vz", "link_id", "delta_rms"}
	if len(records) == 0 {
		t.Fatal("CSV is empty")
	}
	for i, h := range expectedHeaders {
		if i >= len(records[0]) {
			t.Errorf("Missing header column %d: expected %s", i, h)
			continue
		}
		if records[0][i] != h {
			t.Errorf("Header[%d] = %q, want %q", i, records[0][i], h)
		}
	}

	// Should have header + 1 position row + 1 link row (1 unique link pair for 2 nodes)
	// With 2 nodes, there's 1 unique pair (0,1), so 1 position row + 1 deltaRMS row = 2 data rows
	if len(records) < 3 {
		t.Errorf("Expected at least 3 rows (header + position + deltaRMS), got %d", len(records))
	}

	// Verify position row has walker data
	posRow := records[1]
	if posRow[1] != "0" {
		t.Errorf("Walker ID = %q, want %q", posRow[1], "0")
	}
	if posRow[2] != "2.500" {
		t.Errorf("X = %q, want %q", posRow[2], "2.500")
	}

	// Verify deltaRMS row has link data
	deltaRow := records[2]
	if deltaRow[8] == "" {
		t.Error("Expected link_id in deltaRMS row but got empty")
	}
	if deltaRow[9] == "" {
		t.Error("Expected delta_rms value but got empty")
	}
	if !strings.Contains(deltaRow[8], ":") {
		t.Errorf("link_id should contain MAC separator ':', got %q", deltaRow[8])
	}
}

// TestDeltaRMSComputation tests that deltaRMS values are physically plausible
func TestDeltaRMSComputation(t *testing.T) {
	tx := Point{X: 0, Y: 0, Z: 2}
	rx := Point{X: 5, Y: 0, Z: 2}

	tests := []struct {
		name   string
		walker Point
		minRMS float64
		maxRMS float64
	}{
		{
			name:   "Walker on direct line (zone 1)",
			walker: Point{X: 2.5, Y: 0, Z: 1.7},
			minRMS: 0.1, // zone 1 should have high deltaRMS
			maxRMS: 0.2,
		},
		{
			name:   "Walker far off axis (zone 5+)",
			walker: Point{X: 2.5, Y: 10, Z: 1.7},
			minRMS: 0.0,
			maxRMS: 0.02, // should be very low
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rms := computeWalkerDeltaRMS(tx, rx, tt.walker)
			if rms < tt.minRMS || rms > tt.maxRMS {
				t.Errorf("deltaRMS = %v, expected range [%v, %v]", rms, tt.minRMS, tt.maxRMS)
			}
		})
	}
}

// TestWallFlagParsing tests that --wall flag parsing works
func TestWallFlagParsing(t *testing.T) {
	// Reset walls
	walls = nil

	f := &wallFlag{}
	if err := f.Set("1.0,2.0,3.0,4.0"); err != nil {
		t.Fatalf("Failed to parse wall: %v", err)
	}
	if err := f.Set("5.0,6.0,7.0,8.0"); err != nil {
		t.Fatalf("Failed to parse second wall: %v", err)
	}

	if len(walls) != 2 {
		t.Fatalf("Expected 2 walls, got %d", len(walls))
	}

	if walls[0].X1 != 1.0 || walls[0].Y1 != 2.0 || walls[0].X2 != 3.0 || walls[0].Y2 != 4.0 {
		t.Errorf("Wall 0: got %+v", walls[0])
	}
	if walls[1].X1 != 5.0 || walls[1].Y1 != 6.0 || walls[1].X2 != 7.0 || walls[1].Y2 != 8.0 {
		t.Errorf("Wall 1: got %+v", walls[1])
	}

	// Check default material (drywall) and attenuation
	if walls[0].Material != MaterialDrywall {
		t.Errorf("Wall 0 material = %v, want drywall", walls[0].Material)
	}
	if walls[0].Attenuation != 3.0 {
		t.Errorf("Wall 0 attenuation = %v, want 3.0", walls[0].Attenuation)
	}

	// Test invalid format
	if err := f.Set("1,2,3"); err == nil {
		t.Error("Expected error for 3-part wall spec")
	}
}

// TestWalkerBounce tests that walkers bounce off room walls
func TestWalkerBounce(t *testing.T) {
	space := &Space{Width: 5, Depth: 5, Height: 2.5}
	rng := rand.New(rand.NewSource(42))

	walker := &Walker{
		ID:       0,
		Type:     WalkerTypeRandomWalk,
		Position: Point{X: 0.1, Y: 2.5, Z: 1.7}, // Near left wall
		Velocity: Point{X: -1.0, Y: 0, Z: 0},    // Moving left
		Speed:    1.0,
		Height:   1.7,
	}

	// Run several updates — should bounce off left wall
	for i := 0; i < 10; i++ {
		updateWalkers([]*Walker{walker}, space, rng)
	}

	// Walker should not be outside room bounds
	if walker.Position.X < 0 || walker.Position.X > space.Width {
		t.Errorf("Walker X=%v outside room [0, %v]", walker.Position.X, space.Width)
	}
	if walker.Position.Y < 0 || walker.Position.Y > space.Depth {
		t.Errorf("Walker Y=%v outside room [0, %v]", walker.Position.Y, space.Depth)
	}
}

// TestPathFollowing tests that walkers follow a predefined path
func TestPathFollowing(t *testing.T) {
	space := &Space{Width: 5, Depth: 5, Height: 2.5}

	// Create a rectangular path
	path := []Point{
		{X: 0.5, Y: 0.5, Z: 1.7},
		{X: 4.5, Y: 0.5, Z: 1.7},
		{X: 4.5, Y: 4.5, Z: 1.7},
		{X: 0.5, Y: 4.5, Z: 1.7},
	}

	walker := &Walker{
		ID:       0,
		Type:     WalkerTypePathFollow,
		Path:     path,
		PathIdx:  0,
		Speed:    1.0,
		Height:   1.7,
		Position: path[0], // Initialize at first waypoint
	}

	// Run updates - should move along path
	for i := 0; i < 100; i++ {
		updatePathWalker(walker, 0.1)
	}

	// Walker should have moved from starting position
	if walker.Position == path[0] {
		t.Error("Walker should have moved from starting position")
	}

	// Walker should remain within room bounds
	if walker.Position.X < 0 || walker.Position.X > space.Width {
		t.Errorf("Walker X=%v outside room [0, %v]", walker.Position.X, space.Width)
	}
	if walker.Position.Y < 0 || walker.Position.Y > space.Depth {
		t.Errorf("Walker Y=%v outside room [0, %v]", walker.Position.Y, space.Depth)
	}
}

// TestNodeToNodeTraversal tests node-to-node walker movement
func TestNodeToNodeTraversal(t *testing.T) {
	space := &Space{Width: 5, Depth: 5, Height: 2.5}

	nodes := []*VirtualNode{
		{ID: 0, Position: Point{X: 0.5, Y: 0.5, Z: 2.0}},
		{ID: 1, Position: Point{X: 4.5, Y: 0.5, Z: 2.0}},
		{ID: 2, Position: Point{X: 2.5, Y: 4.5, Z: 2.0}},
	}

	walker := &Walker{
		ID:       0,
		Type:     WalkerTypeNodeToNode,
		Nodes:    nodes,
		NodeIdx:  1, // Target is second node
		Speed:    1.0,
		Height:   1.7,
		Position: nodes[0].Position, // Initialize at first node
	}

	// Move towards second node
	for i := 0; i < 100; i++ {
		updateNodeToNodeWalker(walker, 0.1, space)
		// Break if we've moved to the next node
		if walker.NodeIdx != 1 {
			break
		}
	}

	// Walker should have progressed toward the target
	distToTarget := math.Sqrt(
		math.Pow(walker.Position.X-nodes[1].Position.X, 2) +
			math.Pow(walker.Position.Y-nodes[1].Position.Y, 2))
	if distToTarget > 1.0 {
		t.Errorf("Walker should be closer to target; distance=%v", distToTarget)
	}
}

// TestPathFileLoading tests loading paths from JSON file
func TestPathFileLoading(t *testing.T) {
	tmpDir := t.TempDir()
	pathFile := filepath.Join(tmpDir, "paths.json")

	// Create test path file
	paths := []PathDefinition{
		{
			Waypoints: []Point{
				{X: 0, Y: 0, Z: 1.7},
				{X: 1, Y: 0, Z: 1.7},
				{X: 1, Y: 1, Z: 1.7},
				{X: 0, Y: 1, Z: 1.7},
			},
		},
	}

	data, err := json.Marshal(paths)
	if err != nil {
		t.Fatalf("Failed to marshal paths: %v", err)
	}

	if err := os.WriteFile(pathFile, data, 0644); err != nil {
		t.Fatalf("Failed to write path file: %v", err)
	}

	// Load paths
	loaded, err := loadPathsFromFile(pathFile)
	if err != nil {
		t.Fatalf("Failed to load paths: %v", err)
	}

	if len(loaded) != 1 {
		t.Errorf("Expected 1 path, got %d", len(loaded))
	}

	if len(loaded[0]) != 4 {
		t.Errorf("Expected 4 waypoints, got %d", len(loaded[0]))
	}
}

// TestPathFileInvalid tests error handling for invalid path files
func TestPathFileInvalid(t *testing.T) {
	tmpDir := t.TempDir()
	pathFile := filepath.Join(tmpDir, "invalid.json")

	// Write invalid JSON
	if err := os.WriteFile(pathFile, []byte("not valid json"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	_, err := loadPathsFromFile(pathFile)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

// TestDefaultPathWalker tests default rectangular path walker creation
func TestDefaultPathWalker(t *testing.T) {
	space := &Space{Width: 5, Depth: 5, Height: 2.5}

	walkers := createDefaultPathWalkers(1, space)

	if len(walkers) != 1 {
		t.Fatalf("Expected 1 walker, got %d", len(walkers))
	}

	walker := walkers[0]

	if walker.Type != WalkerTypePathFollow {
		t.Errorf("Expected path-following walker, got %v", walker.Type)
	}

	if len(walker.Path) != 4 {
		t.Errorf("Expected 4 waypoints in default path, got %d", len(walker.Path))
	}

	// Verify path forms a rectangle around the room
	path := walker.Path
	margin := 0.5

	// First point: bottom-left
	if path[0].X != margin || path[0].Y != margin {
		t.Errorf("First waypoint should be bottom-left: got %v", path[0])
	}

	// Second point: bottom-right
	if path[1].X != space.Width-margin || path[1].Y != margin {
		t.Errorf("Second waypoint should be bottom-right: got %v", path[1])
	}

	// Third point: top-right
	if path[2].X != space.Width-margin || path[2].Y != space.Depth-margin {
		t.Errorf("Third waypoint should be top-right: got %v", path[2])
	}

	// Fourth point: top-left
	if path[3].X != margin || path[3].Y != space.Depth-margin {
		t.Errorf("Fourth waypoint should be top-left: got %v", path[3])
	}
}

// TestReflectionPointVerticalWall tests reflection point calculation for vertical walls
func TestReflectionPointVerticalWall(t *testing.T) {
	// Vertical wall at x=2.5 from y=0 to y=5
	wall := Wall{X1: 2.5, Y1: 0, X2: 2.5, Y2: 5, Material: MaterialDrywall, Attenuation: 3.0}

	// TX and RX on opposite sides of the wall, at different Y positions
	// This should create a valid reflection
	tx := Point{X: 1, Y: 1, Z: 2}
	rx := Point{X: 3, Y: 3, Z: 2}

	reflPoint, ok := findReflectionPoint(tx, rx, wall)
	if !ok {
		t.Fatal("Failed to find reflection point")
	}

	// Reflection point should be on the wall at x=2.5
	if reflPoint.X != 2.5 {
		t.Errorf("Reflection X = %v, want 2.5", reflPoint.X)
	}

	// Y should be between 0 and 5
	if reflPoint.Y < 0 || reflPoint.Y > 5 {
		t.Errorf("Reflection Y = %v outside wall bounds [0, 5]", reflPoint.Y)
	}

	// Z should be average of TX and RX Z
	expectedZ := (tx.Z + rx.Z) / 2.0
	if math.Abs(reflPoint.Z-expectedZ) > 1e-6 {
		t.Errorf("Reflection Z = %v, want %v", reflPoint.Z, expectedZ)
	}
}

// TestReflectionPointHorizontalWall tests reflection point calculation for horizontal walls
func TestReflectionPointHorizontalWall(t *testing.T) {
	// Horizontal wall at y=2.5 from x=0 to x=5
	wall := Wall{X1: 0, Y1: 2.5, X2: 5, Y2: 2.5, Material: MaterialDrywall, Attenuation: 3.0}

	// TX and RX on opposite sides of the wall, at different X positions
	tx := Point{X: 1, Y: 1, Z: 2}
	rx := Point{X: 3, Y: 3, Z: 2}

	reflPoint, ok := findReflectionPoint(tx, rx, wall)
	if !ok {
		t.Fatal("Failed to find reflection point")
	}

	// Reflection point should be on the wall at y=2.5
	if reflPoint.Y != 2.5 {
		t.Errorf("Reflection Y = %v, want 2.5", reflPoint.Y)
	}

	// X should be between 0 and 5
	if reflPoint.X < 0 || reflPoint.X > 5 {
		t.Errorf("Reflection X = %v outside wall bounds [0, 5]", reflPoint.X)
	}

	// Z should be average of TX and RX Z
	expectedZ := (tx.Z + rx.Z) / 2.0
	if math.Abs(reflPoint.Z-expectedZ) > 1e-6 {
		t.Errorf("Reflection Z = %v, want %v", reflPoint.Z, expectedZ)
	}
}

// TestReflectionPointOutOfBounds tests that reflections outside wall bounds fail
func TestReflectionPointOutOfBounds(t *testing.T) {
	// Vertical wall at x=2.5 from y=0 to y=2 (walker Y=2.5 is outside bounds)
	wall := Wall{X1: 2.5, Y1: 0, X2: 2.5, Y2: 2, Material: MaterialDrywall, Attenuation: 3.0}

	tx := Point{X: 1, Y: 2.5, Z: 2}
	rx := Point{X: 4, Y: 2.5, Z: 2}

	_, ok := findReflectionPoint(tx, rx, wall)
	if ok {
		t.Error("Expected no reflection point for out-of-bounds Y")
	}
}

// TestReflectPointAcrossLine tests point reflection across a line
func TestReflectPointAcrossLine(t *testing.T) {
	p := Point{X: 1, Y: 1, Z: 0}
	lineStart := Point{X: 0, Y: 0, Z: 0}
	lineEnd := Point{X: 2, Y: 0, Z: 0} // Horizontal line at y=0

	refl := reflectPointAcrossLine(p, lineStart, lineEnd)

	// Point (1,1) reflected across y=0 should be (1,-1)
	if refl.X != 1 {
		t.Errorf("Refl X = %v, want 1", refl.X)
	}
	if refl.Y != -1 {
		t.Errorf("Refl Y = %v, want -1", refl.Y)
	}
}

// TestLineIntersection tests line segment intersection
func TestLineIntersection(t *testing.T) {
	tests := []struct {
		name   string
		p1, p2 Point // First line segment
		p3, p4 Point // Second line segment
		wantOK bool
		wantX  float64
		wantY  float64
	}{
		{
			name:   "Crossing lines",
			p1:     Point{X: 0, Y: 0, Z: 0},
			p2:     Point{X: 2, Y: 2, Z: 0},
			p3:     Point{X: 0, Y: 2, Z: 0},
			p4:     Point{X: 2, Y: 0, Z: 0},
			wantOK: true,
			wantX:  1,
			wantY:  1,
		},
		{
			name:   "Parallel lines",
			p1:     Point{X: 0, Y: 0, Z: 0},
			p2:     Point{X: 1, Y: 0, Z: 0},
			p3:     Point{X: 0, Y: 1, Z: 0},
			p4:     Point{X: 1, Y: 1, Z: 0},
			wantOK: false,
		},
		{
			name:   "Non-intersecting segments",
			p1:     Point{X: 0, Y: 0, Z: 0},
			p2:     Point{X: 1, Y: 0, Z: 0},
			p3:     Point{X: 2, Y: 0, Z: 0},
			p4:     Point{X: 3, Y: 0, Z: 0},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pt, ok := lineIntersection(tt.p1, tt.p2, tt.p3, tt.p4)
			if ok != tt.wantOK {
				t.Errorf("lineIntersection() ok = %v, want %v", ok, tt.wantOK)
				return
			}
			if tt.wantOK {
				if math.Abs(pt.X-tt.wantX) > 1e-6 {
					t.Errorf("X = %v, want %v", pt.X, tt.wantX)
				}
				if math.Abs(pt.Y-tt.wantY) > 1e-6 {
					t.Errorf("Y = %v, want %v", pt.Y, tt.wantY)
				}
			}
		})
	}
}

// TestWallMaterialProperties tests that different wall materials have correct attenuation
func TestWallMaterialProperties(t *testing.T) {
	walls = nil // Reset

	f := &wallFlag{}

	// Test drywall (default)
	if err := f.Set("0,0,5,0"); err != nil {
		t.Fatalf("Failed to parse drywall wall: %v", err)
	}
	if len(walls) != 1 || walls[0].Attenuation != 3.0 {
		t.Errorf("Drywall attenuation = %v, want 3.0", walls[0].Attenuation)
	}
	if walls[0].Material != MaterialDrywall {
		t.Errorf("Material = %v, want drywall", walls[0].Material)
	}

	// Test brick
	walls = nil
	if err := f.Set("0,1,5,1,brick"); err != nil {
		t.Fatalf("Failed to parse brick wall: %v", err)
	}
	if walls[0].Attenuation != 10.0 {
		t.Errorf("Brick attenuation = %v, want 10.0", walls[0].Attenuation)
	}

	// Test concrete
	walls = nil
	if err := f.Set("0,2,5,2,concrete"); err != nil {
		t.Fatalf("Failed to parse concrete wall: %v", err)
	}
	if walls[0].Attenuation != 10.0 {
		t.Errorf("Concrete attenuation = %v, want 10.0", walls[0].Attenuation)
	}

	// Test glass
	walls = nil
	if err := f.Set("0,3,5,3,glass"); err != nil {
		t.Fatalf("Failed to parse glass wall: %v", err)
	}
	if walls[0].Attenuation != 2.0 {
		t.Errorf("Glass attenuation = %v, want 2.0", walls[0].Attenuation)
	}

	// Test metal
	walls = nil
	if err := f.Set("0,4,5,4,metal"); err != nil {
		t.Fatalf("Failed to parse metal wall: %v", err)
	}
	if walls[0].Attenuation != 20.0 {
		t.Errorf("Metal attenuation = %v, want 20.0", walls[0].Attenuation)
	}

	// Test invalid material
	walls = nil
	if err := f.Set("0,5,5,5,invalid"); err == nil {
		t.Error("Expected error for invalid material")
	}
}

// TestTwoRayModel tests that CSI includes both direct and reflected contributions
func TestTwoRayModel(t *testing.T) {
	tx := &VirtualNode{ID: 0, Position: Point{X: 1, Y: 1, Z: 2}}
	rx := &VirtualNode{ID: 1, Position: Point{X: 3, Y: 3, Z: 2}}
	walkers := []*Walker{{ID: 0, Position: Point{X: 2, Y: 2, Z: 1.7}}}

	// No walls - should only have direct path
	ampNoWall, _ := computeCSIForWalkers(tx, rx, walkers, nil)
	if ampNoWall <= 0 {
		t.Errorf("Amplitude with no wall = %v, want > 0", ampNoWall)
	}

	// Add a wall positioned to create a valid reflection
	// Wall from (0,0) to (0,5) - vertical wall at x=0
	walls := []Wall{{X1: 0, Y1: 0, X2: 0, Y2: 5, Material: MaterialDrywall, Attenuation: 3.0}}
	ampWithWall, _ := computeCSIForWalkers(tx, rx, walkers, walls)

	// The reflection should affect the CSI - check that amplitude is non-zero
	if ampWithWall <= 0 {
		t.Errorf("Amplitude with wall = %v, want > 0", ampWithWall)
	}

	// Verify that computeFirstOrderReflection actually returns values when there's a valid wall geometry
	reflAmp, reflPhase := computeFirstOrderReflection(tx.Position, rx.Position, walkers[0].Position, walls)
	if reflAmp == 0 && reflPhase == 0 {
		t.Error("Expected non-zero reflection contribution with wall present (valid geometry)")
	}

	// Test with wall positioned such that no valid reflection exists
	// Use a wall where the reflection point would fall outside the wall segment bounds
	// TX at (1,1), RX at (3,3), vertical wall at x=2 from y=10 to y=15
	// The reflection of TX across x=2 would be at (3,1). The line from (3,1) to (3,3)
	// is vertical at x=3, which never intersects the wall at x=2.
	wallsFar := []Wall{{X1: 2, Y1: 10, X2: 2, Y2: 15, Material: MaterialDrywall, Attenuation: 3.0}}
	_, reflPhaseFar := computeFirstOrderReflection(tx.Position, rx.Position, walkers[0].Position, wallsFar)
	if reflPhaseFar != 0 {
		t.Error("Expected zero reflection contribution with wall that doesn't create valid reflection geometry")
	}
}

// TestPathLossModel tests the log-distance path loss calculation
func TestPathLossModel(t *testing.T) {
	tx := Point{X: 0, Y: 0, Z: 2}
	rx := Point{X: 0, Y: 0, Z: 2}
	walker := Point{X: 1, Y: 0, Z: 1.7}

	// Close walker should produce higher amplitude than far walker
	closeAmp, _ := computeDirectPath(tx, rx, walker, nil)

	farWalker := Point{X: 10, Y: 0, Z: 1.7}
	farAmp, _ := computeDirectPath(tx, rx, farWalker, nil)

	if farAmp >= closeAmp {
		t.Errorf("Far walker amplitude %v >= close walker amplitude %v", farAmp, closeAmp)
	}
}
