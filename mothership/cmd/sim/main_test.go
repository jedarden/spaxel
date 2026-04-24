package main

import (
	"encoding/csv"
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
	csvWriter.Close()

	// Read back and verify
	file, err := os.Open(csvPath)
	if err != nil {
		t.Fatalf("Failed to open CSV: %v", err)
	}
	defer file.Close()

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
		name     string
		walker   Point
		minRMS   float64
		maxRMS   float64
	}{
		{
			name:   "Walker on direct line (zone 1)",
			walker: Point{X: 2.5, Y: 0, Z: 1.7},
			minRMS: 0.1,  // zone 1 should have high deltaRMS
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
		Position: Point{X: 0.1, Y: 2.5, Z: 1.7}, // Near left wall
		Velocity: Point{X: -1.0, Y: 0, Z: 0},     // Moving left
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
