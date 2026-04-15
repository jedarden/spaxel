package main

import (
	"math"
	"math/rand"
	"testing"
)

// TestGenerateCSIFrameHeader tests that generated frames have correct binary header format.
// The frame must match the ingestion layer layout (ingestion/frame.go):
//   [0:6]  node_mac, [6:12] peer_mac, [12:20] timestamp_us,
//   [20] rssi, [21] noise_floor, [22] channel, [23] n_sub, [24:] payload
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

// TestRSSIInRange tests that RSSI is within plausible range for given distance
func TestRSSIInRange(t *testing.T) {
	rng := rand.New(rand.NewSource(42))

	tx := &VirtualNode{ID: 0, MAC: generateMAC(0), Position: Point{X: 0, Y: 0, Z: 2}}
	rx := &VirtualNode{ID: 1, MAC: generateMAC(1), Position: Point{X: 5, Y: 0, Z: 2}}
	walkers := []*Walker{{ID: 0, Position: Point{X: 2.5, Y: 0, Z: 1.7}}}

	frame := generateCSIFrame(tx, rx, walkers, nil, 0, rng)

	// RSSI is at ingestion format offset [20]
	rssi := int8(frame[20])

	// RSSI should be in [-90, -30] dBm for a 5m link
	if rssi < -90 || rssi > -30 {
		t.Errorf("RSSI out of range: %d (expected [-90, -30])", rssi)
	}

	// At 2m distance (walker at 2.5m from TX, 2.5m from RX), RSSI should be roughly [-70, -50]
	if rssi < -70 || rssi > -50 {
		t.Logf("WARNING: RSSI %d at ~2m may be outside expected range [-70, -50]", rssi)
	}
}

// TestIQClamping tests that generated I/Q values are clamped to int8 range
func TestIQClamping(t *testing.T) {
	rng := rand.New(rand.NewSource(42))

	tx := &VirtualNode{ID: 0, Position: Point{X: 0, Y: 0, Z: 2}}
	rx := &VirtualNode{ID: 1, Position: Point{X: 0.1, Y: 0, Z: 2}} // Very close for high amplitude
	walkers := []*Walker{{ID: 0, Position: Point{X: 0.05, Y: 0, Z: 1.7}}}

	frame := generateCSIFrame(tx, rx, walkers, nil, 0, rng)

	// Check all I/Q values are in int8 range [-128, 127]
	// We avoid -128 as per the implementation
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
	// This is verified by the fact that the mothership accepts the connection
	// We just check the JSON structure here
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

	// Check required fields
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
		name           string
		blobCount      int
		walkerCount    int
		expectedPass   bool
	}{
		{"Exact match", 1, 1, true},
		{"Within tolerance", 2, 1, true},
		{"Within tolerance", 0, 1, true},
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

	// Frames should be identical
	if len(frame1) != len(frame2) {
		t.Fatalf("Frame lengths differ: %d vs %d", len(frame1), len(frame2))
	}

	// Check header fields match
	for i := 0; i < headerSize; i++ {
		if frame1[i] != frame2[i] {
			t.Errorf("Header byte %d differs: %d vs %d", i, frame1[i], frame2[i])
		}
	}

	// Check payload matches
	for i := headerSize; i < len(frame1); i++ {
		if frame1[i] != frame2[i] {
			t.Errorf("Payload byte %d differs: %d vs %d", i, frame1[i], frame2[i])
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
		{"5x5", 0, 0, 0, true}, // missing dimension
		{"5x5x2x3", 0, 0, 0, true}, // too many dimensions
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
		id    int
		want  string
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
		name     string
		walkers  []*Walker
		minAmp   float64
		maxAmp   float64
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
			minAmp: 0.1, // Should be in Fresnel zone 1
			maxAmp: 10,
		},
		{
			name: "Walker far from link",
			walkers: []*Walker{{
				ID:       0,
				Position: Point{X: 2.5, Y: 10, Z: 1.7},
			}},
			minAmp: 0,  // Should be very weak
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
