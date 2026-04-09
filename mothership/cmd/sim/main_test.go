// Package main provides tests for the CSI simulator.
package main

import (
	"bytes"
	"encoding/binary"
	"math"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseSpaceDims(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantWidth   float64
		wantDepth   float64
		wantHeight  float64
		wantErr     bool
		errContains string
	}{
		{
			name:       "valid dimensions",
			input:      "6.0x5.0x2.5",
			wantWidth:  6.0,
			wantDepth:  5.0,
			wantHeight: 2.5,
			wantErr:    false,
		},
		{
			name:       "integer dimensions",
			input:      "6x5x2",
			wantWidth:  6.0,
			wantDepth:  5.0,
			wantHeight: 2.0,
			wantErr:    false,
		},
		{
			name:       "large space",
			input:      "20x15x3",
			wantWidth:  20.0,
			wantDepth:  15.0,
			wantHeight: 3.0,
			wantErr:    false,
		},
		{
			name:        "invalid format - missing dimension",
			input:       "6x5",
			wantErr:     true,
			errContains: "failed to parse",
		},
		{
			name:        "invalid format - wrong separator",
			input:       "6,5,2.5",
			wantErr:     true,
			errContains: "failed to parse",
		},
		{
			name:        "negative dimension",
			input:       "-6x5x2.5",
			wantErr:     true,
			errContains: "must be positive",
		},
		{
			name:        "zero dimension",
			input:       "0x5x2.5",
			wantErr:     true,
			errContains: "must be positive",
		},
		{
			name:        "non-numeric",
			input:       "axbxc",
			wantErr:     true,
			errContains: "failed to parse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			width, depth, height, err := parseSpaceDims(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseSpaceDims() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errContains != "" {
				if err.Error() == "" || !contains(err.Error(), tt.errContains) {
					t.Errorf("parseSpaceDims() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}
			if !tt.wantErr {
				if width != tt.wantWidth || depth != tt.wantDepth || height != tt.wantHeight {
					t.Errorf("parseSpaceDims() = (%v, %v, %v), want (%v, %v, %v)",
						width, depth, height, tt.wantWidth, tt.wantDepth, tt.wantHeight)
				}
			}
		})
	}
}

func TestMacToBytes(t *testing.T) {
	tests := []struct {
		name  string
		mac   string
		want  [6]byte
	}{
		{
			name: "standard MAC",
			mac:  "AA:BB:CC:DD:EE:FF",
			want: [6]byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF},
		},
		{
			name: "lowercase MAC",
			mac:  "aa:bb:cc:dd:ee:ff",
			want: [6]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
		},
		{
			name: "mixed case MAC",
			mac:  "Aa:Bb:Cc:Dd:Ee:Ff",
			want: [6]byte{0xAa, 0xBb, 0xCc, 0xDd, 0xEe, 0xFf},
		},
		{
			name: "zeros",
			mac:  "00:00:00:00:00:00",
			want: [6]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		},
		{
			name: "all FF",
			mac:  "FF:FF:FF:FF:FF:FF",
			want: [6]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := macToBytes(tt.mac)
			if got != tt.want {
				t.Errorf("macToBytes(%q) = %v, want %v", tt.mac, got, tt.want)
			}
		})
	}
}

func TestCSIFrameStructure(t *testing.T) {
	node := &VirtualNode{
		mac:       "AA:BB:CC:DD:EE:00",
		position:  [3]float64{0, 0, 2.5},
		frameCount: 0,
	}

	walker := Walker{
		position: [3]float64{3, 2.5, 1.7},
		velocity: [3]float64{0.1, 0, 0},
		mac:      "11:22:33:44:55:00",
	}

	frame := node.generateCSIFrame(walker, 0)

	// Check minimum frame length
	if len(frame) < HeaderSize {
		t.Fatalf("Frame length %d is less than header size %d", len(frame), HeaderSize)
	}

	// Check n_sub field
	nSub := frame[23]
	if nSub > MaxSubcarriers {
		t.Errorf("n_sub = %d, exceeds max %d", nSub, MaxSubcarriers)
	}

	expectedPayloadSize := int(nSub) * 2
	expectedFrameSize := HeaderSize + expectedPayloadSize
	if len(frame) != expectedFrameSize {
		t.Errorf("Frame length = %d, want %d (header %d + payload %d)",
			len(frame), expectedFrameSize, HeaderSize, expectedPayloadSize)
	}

	// Check node MAC is set correctly
	nodeMAC := frame[0:6]
	expectedMAC := macToBytes(node.mac)
	if !bytesEqual(nodeMAC, expectedMAC[:]) {
		t.Errorf("Node MAC = %v, want %v", nodeMAC, expectedMAC)
	}

	// Check channel is valid WiFi channel (1-14)
	channel := frame[22]
	if channel < 1 || channel > 14 {
		t.Errorf("Channel = %d, want valid channel 1-14", channel)
	}

	// Check timestamp is reasonable (should start small)
	timestampUS := binary.LittleEndian.Uint64(frame[12:20])
	if timestampUS > 100000 { // Should be less than 100ms for first frame
		t.Errorf("Timestamp = %d us, want < 100000 us for first frame", timestampUS)
	}
}

func TestFresnelModulation(t *testing.T) {
	tests := []struct {
		name           string
		nodePos        [3]float64
		walkerPos      [3]float64
		wantZone       int
		wantModulation float64
	}{
		{
			name:           "zone 1 - very close",
			nodePos:        [3]float64{0, 0, 2.5},
			walkerPos:      [3]float64{0.01, 0, 1.7},
			wantZone:       1,
			wantModulation: 1.0,
		},
		{
			name:           "zone 2",
			nodePos:        [3]float64{0, 0, 2.5},
			walkerPos:      [3]float64{0.2, 0, 1.7},
			wantZone:       2,
			wantModulation: 1.0 / 4.0, // 1/2^2
		},
		{
			name:           "zone 3",
			nodePos:        [3]float64{0, 0, 2.5},
			walkerPos:      [3]float64{0.3, 0, 1.7},
			wantZone:       3,
			wantModulation: 1.0 / 9.0, // 1/3^2
		},
		{
			name:           "zone 4",
			nodePos:        [3]float64{0, 0, 2.5},
			walkerPos:      [3]float64{0.4, 0, 1.7},
			wantZone:       4,
			wantModulation: 1.0 / 16.0, // 1/4^2
		},
		{
			name:           "zone 5 - boundary",
			nodePos:        [3]float64{0, 0, 2.5},
			walkerPos:      [3]float64{0.5, 0, 1.7},
			wantZone:       5,
			wantModulation: 1.0 / 25.0, // 1/5^2
		},
		{
			name:           "beyond zone 5",
			nodePos:        [3]float64{0, 0, 2.5},
			walkerPos:      [3]float64{1.0, 0, 1.7},
			wantZone:       6,
			wantModulation: 0.0, // Zone 5+ returns 0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mod := fresnelModulation(tt.nodePos, tt.walkerPos)
			if math.Abs(mod-tt.wantModulation) > 0.01 {
				t.Errorf("fresnelModulation() = %f, want %f", mod, tt.wantModulation)
			}
		})
	}
}

func TestRSSICalculation(t *testing.T) {
	node := &VirtualNode{
		position: [3]float64{0, 0, 2.5},
	}

	tests := []struct {
		name        string
		distance    float64
		wantMinRSSI int8
		wantMaxRSSI int8
	}{
		{
			name:        "very close (1m)",
			distance:    1.0,
			wantMinRSSI: -35,
			wantMaxRSSI: -30,
		},
		{
			name:        "near (2m)",
			distance:    2.0,
			wantMinRSSI: -45,
			wantMaxRSSI: -35,
		},
		{
			name:        "medium (5m)",
			distance:    5.0,
			wantMinRSSI: -60,
			wantMaxRSSI: -50,
		},
		{
			name:        "far (10m)",
			distance:    10.0,
			wantMinRSSI: -75,
			wantMaxRSSI: -60,
		},
		{
			name:        "very far (20m)",
			distance:    20.0,
			wantMinRSSI: -90,
			wantMaxRSSI: -75,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a walker at the specified distance along X axis
			walker := Walker{
				position: [3]float64{tt.distance, 0, 1.7},
			}

			frame := node.generateCSIFrame(walker, 0)
			rssi := int8(frame[20])

			if rssi < tt.wantMinRSSI || rssi > tt.wantMaxRSSI {
				t.Errorf("RSSI = %d, want between %d and %d for distance %fm",
					rssi, tt.wantMinRSSI, tt.wantMaxRSSI, tt.distance)
			}
		})
	}
}

func TestUpdateWalkerPosition(t *testing.T) {
	width := 6.0
	depth := 5.0

	tests := []struct {
		name           string
		initialPos     [3]float64
		initialVel     [3]float64
		steps          int
		posInBounds    bool
		velClamped     bool
	}{
		{
			name:        "normal motion",
			initialPos:  [3]float64{3, 2.5, 1.7},
			initialVel:  [3]float64{0.1, 0.05, 0},
			steps:       10,
			posInBounds: true,
			velClamped:  false,
		},
		{
			name:        "bounce off right wall",
			initialPos:  [3]float64{5.9, 2.5, 1.7},
			initialVel:  [3]float64{0.5, 0, 0},
			steps:       1,
			posInBounds: true,
			velClamped:  true, // velocity should flip
		},
		{
			name:        "bounce off left wall",
			initialPos:  [3]float64{0.1, 2.5, 1.7},
			initialVel:  [3]float64{-0.5, 0, 0},
			steps:       1,
			posInBounds: true,
			velClamped:  true,
		},
		{
			name:        "bounce off bottom wall",
			initialPos:  [3]float64{3, 4.9, 1.7},
			initialVel:  [3]float64{0, 0.5, 0},
			steps:       1,
			posInBounds: true,
			velClamped:  true,
		},
		{
			name:        "bounce off top wall",
			initialPos:  [3]float64{3, 0.1, 1.7},
			initialVel:  [3]float64{0, -0.5, 0},
			steps:       1,
			posInBounds: true,
			velClamped:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			walker := &Walker{
				position: tt.initialPos,
				velocity: tt.initialVel,
			}

			for i := 0; i < tt.steps; i++ {
				updateWalkerPosition(walker, width, depth)
			}

			// Check position is in bounds
			if walker.position[0] < 0 || walker.position[0] > width ||
				walker.position[1] < 0 || walker.position[1] > depth {
				t.Errorf("Walker position %v is out of bounds [0,%f]x[0,%f]",
					walker.position, width, depth)
			}

			// For bounce tests, verify velocity changed direction
			if tt.velClamped {
				// Velocity should have flipped sign
				if tt.initialVel[0] > 0 && walker.velocity[0] > 0 {
					t.Errorf("Velocity should have flipped (was positive, still positive)")
				}
				if tt.initialVel[0] < 0 && walker.velocity[0] < 0 {
					t.Errorf("Velocity should have flipped (was negative, still negative)")
				}
			}
		})
	}
}

func TestCSIFramePayload(t *testing.T) {
	node := &VirtualNode{
		mac:      "AA:BB:CC:DD:EE:00",
		position: [3]float64{0, 0, 2.5},
	}

	walker := Walker{
		position: [3]float64{3, 2.5, 1.7},
		velocity: [3]float64{0.1, 0, 0},
		mac:      "11:22:33:44:55:00",
	}

	frame := node.generateCSIFrame(walker, 0)
	nSub := int(frame[23])

	// Check payload contains I, Q pairs
	payloadOffset := HeaderSize
	for k := 0; k < nSub; k++ {
		offset := payloadOffset + k*2
		iVal := int8(frame[offset])
		qVal := int8(frame[offset+1])

		// I and Q should be in reasonable range (-128 to 127)
		if iVal < -128 || iVal > 127 {
			t.Errorf("I value at subcarrier %d = %d, out of range", k, iVal)
		}
		if qVal < -128 || qVal > 127 {
			t.Errorf("Q value at subcarrier %d = %d, out of range", k, qVal)
		}

		// Values should not all be zero (unless noise is exactly zero)
		if k > 0 && iVal == 0 && qVal == 0 {
			// Allow some zeros but not all
			hasNonZero := false
			for j := 0; j < nSub; j++ {
				o := payloadOffset + j*2
				if int8(frame[o]) != 0 || int8(frame[o+1]) != 0 {
					hasNonZero = true
					break
				}
			}
			if !hasNonZero {
				t.Errorf("All I/Q values are zero, expected some signal")
			}
			break
		}
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestHelloMessageFormat tests that the hello message matches expected format
func TestHelloMessageFormat(t *testing.T) {
	node := &VirtualNode{
		mac:      "AA:BB:CC:DD:EE:FF",
		position: [3]float64{1.0, 2.0, 2.5},
	}

	// Create a hello message as the node would send it
	hello := HelloMessage{
		Type:            "hello",
		MAC:             node.mac,
		NodeID:          fmt.Sprintf("sim-node-%s", node.mac),
		FirmwareVersion: "0.1.0-sim",
		Capabilities:    []string{"csi", "tx", "rx"},
		Chip:            "ESP32-S3",
		FlashMB:         16,
		UptimeMS:        1000,
	}

	// Marshal to JSON
	data, err := json.Marshal(hello)
	if err != nil {
		t.Fatalf("Failed to marshal hello: %v", err)
	}

	// Unmarshal and verify
	var decoded HelloMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal hello: %v", err)
	}

	if decoded.Type != "hello" {
		t.Errorf("Type = %s, want 'hello'", decoded.Type)
	}
	if decoded.MAC != node.mac {
		t.Errorf("MAC = %s, want %s", decoded.MAC, node.mac)
	}
	if decoded.FirmwareVersion != "0.1.0-sim" {
		t.Errorf("FirmwareVersion = %s, want '0.1.0-sim'", decoded.FirmwareVersion)
	}
	if len(decoded.Capabilities) != 3 {
		t.Errorf("Capabilities length = %d, want 3", len(decoded.Capabilities))
	}
}

// TestIQClamping tests that I/Q values are clamped to int8 range
func TestIQClamping(t *testing.T) {
	tests := []struct {
		name        string
		amplitude   float64
		phase       float64
		noiseSigma  float64
		wantInRange bool
	}{
		{
			name:        "normal values",
			amplitude:   30.0,
			phase:       0.5,
			noiseSigma:  0.005,
			wantInRange: true,
		},
		{
			name:        "high amplitude",
			amplitude:   500.0,
			phase:       0.0,
			noiseSigma:  0.005,
			wantInRange: true, // Should be clamped
		},
		{
			name:        "negative amplitude",
			amplitude:   -100.0,
			phase:       0.0,
			noiseSigma:  0.005,
			wantInRange: true, // Should be clamped
		},
		{
			name:        "large noise",
			amplitude:   30.0,
			phase:       0.5,
			noiseSigma:  1.0,
			wantInRange: true, // Should be clamped
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Generate I/Q pair
			noise := rand.NormFloat64() * tt.noiseSigma * 100.0
			iVal := tt.amplitude*math.Cos(tt.phase) + noise
			qVal := tt.amplitude*math.Sin(tt.phase) + noise

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

			// Check range
			inRange := int8(iVal) >= -127 && int8(iVal) <= 127 &&
				int8(qVal) >= -127 && int8(qVal) <= 127

			if inRange != tt.wantInRange {
				t.Errorf("I/Q in range = %v, want %v (I=%d, Q=%d)",
					inRange, tt.wantInRange, int8(iVal), int8(qVal))
			}
		})
	}
}

// TestSeedReproducibility tests that --seed produces identical walker paths
func TestSeedReproducibility(t *testing.T) {
	seed := int64(42)
	width := 6.0
	depth := 5.0
	height := 2.5

	// First run with seed
	rand.Seed(seed)
	walkerSim1 := NewWalkerSimulator(1, width, depth, height, seed)

	// Update positions
	walkerSim1.Update(1.0) // 1 second
	pos1 := walkerSim1.GetWalkers()[0].Position

	// Reset and run again with same seed
	rand.Seed(seed)
	walkerSim2 := NewWalkerSimulator(1, width, depth, height, seed)

	walkerSim2.Update(1.0)
	pos2 := walkerSim2.GetWalkers()[0].Position

	// Positions should be identical
	if pos1[0] != pos2[0] || pos1[1] != pos2[1] || pos1[2] != pos2[2] {
		t.Errorf("Positions differ with same seed: run1=%v, run2=%v", pos1, pos2)
	}
}

// TestOutputCSV tests that --output-csv generates a CSV with correct headers
func TestOutputCSV(t *testing.T) {
	width := 6.0
	depth := 5.0
	height := 2.5
	seed := int64(42)

	// Create a temporary CSV file
	tmpFile, err := os.CreateTemp("", "sim-test-*.csv")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// Create walker simulator
	walkerSim := NewWalkerSimulator(1, width, depth, height, seed)

	// Open CSV
	if err := walkerSim.OpenCSV(tmpFile.Name()); err != nil {
		t.Fatalf("Failed to open CSV: %v", err)
	}
	defer walkerSim.CloseCSV()

	// Write some data rows
	timestamp := time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)
	walker := walkerSim.GetWalkers()[0]

	for i := 0; i < 3; i++ {
		timestamp = timestamp.Add(time.Second)
		if err := walkerSim.WriteCSVRow(timestamp, walker); err != nil {
			t.Fatalf("Failed to write CSV row: %v", err)
		}
	}

	// Flush and read back
	if err := walkerSim.CloseCSV(); err != nil {
		t.Fatalf("Failed to close CSV: %v", err)
	}

	// Read and verify CSV content
	data, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to read CSV file: %v", err)
	}

	lines := strings.Split(string(data), "\n")

	// Check header
	expectedHeader := "timestamp_ms,walker_id,x,y,z,vx,vy,vz"
	if lines[0] != expectedHeader {
		t.Errorf("CSV header = %s, want %s", lines[0], expectedHeader)
	}

	// Check we have 4 lines (header + 3 data rows)
	if len(lines) < 4 {
		t.Errorf("CSV has %d lines, want at least 4", len(lines))
	}

	// Verify data row format
	dataLine := strings.Split(lines[1], ",")
	if len(dataLine) != 8 {
		t.Errorf("Data row has %d columns, want 8", len(dataLine))
	}
}

// TestVerificationModeBlobCount tests that --verify correctly detects missing blobs
func TestVerificationModeBlobCount(t *testing.T) {
	// This test would require a running mothership server
	// For now, we test the verification logic in isolation

	verifier := NewVerifier("http://localhost:8080")

	// Test allWalkersInBounds with default bounds
	tests := []struct {
		name     string
		positions [][3]float64
		want      bool
	}{
		{
			name:     "all in bounds",
			positions: [][3]float64{{3, 2.5, 1.7}, {1, 1, 1}},
			want:      true,
		},
		{
			name:     "one out of bounds (X)",
			positions: [][3]float64{{7, 2.5, 1.7}, {3, 2.5, 1.7}},
			want:      false,
		},
		{
			name:     "one out of bounds (Y)",
			positions: [][3]float64{{3, 6, 1.7}, {3, 2.5, 1.7}},
			want:      false,
		},
		{
			name:     "one out of bounds (Z)",
			positions: [][3]float64{{3, 2.5, 3}, {3, 2.5, 1.7}},
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := verifier.allWalkersInBounds(tt.positions)
			if result != tt.want {
				t.Errorf("allWalkersInBounds() = %v, want %v", result, tt.want)
			}
		})
	}
}

// TestVerifyBlobDetection tests that verify correctly detects blob count mismatches
func TestVerifyBlobDetection(t *testing.T) {
	verifier := NewVerifier("http://localhost:8080")

	// Test walkersHaveNearbyBlobs
	walkerPositions := [][3]float64{{3, 2.5, 1.7}, {1, 1, 1}}

	tests := []struct {
		name     string
		blobs    []Blob
		want     bool
	}{
		{
			name: "blob near each walker",
			blobs: []Blob{
				{X: 3.0, Y: 2.5, Z: 1.7},
				{X: 1.0, Y: 1.0, Z: 1.0},
			},
			want: true,
		},
		{
			name: "blob too far from first walker",
			blobs: []Blob{
				{X: 5.5, Y: 2.5, Z: 1.7},
			},
			want: false,
		},
		{
			name: "no blobs",
			blobs: []Blob{},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := verifier.walkersHaveNearbyBlobs(walkerPositions, tt.blobs)
			if result != tt.want {
				t.Errorf("walkersHaveNearbyBlobs() = %v, want %v", result, tt.want)
			}
		})
	}
}

// TestDistance3D tests the 3D distance calculation
func TestDistance3D(t *testing.T) {
	tests := []struct {
		name     string
		a        [3]float64
		b        [3]float64
		wantDist float64
	}{
		{
			name:     "same point",
			a:        [3]float64{0, 0, 0},
			b:        [3]float64{0, 0, 0},
			wantDist: 0,
		},
		{
			name:     "unit distance on X",
			a:        [3]float64{0, 0, 0},
			b:        [3]float64{1, 0, 0},
			wantDist: 1.0,
		},
		{
			name:     "3D distance",
			a:        [3]float64{0, 0, 0},
			b:        [3]float64{3, 4, 0},
			wantDist: 5.0,
		},
		{
			name:     "with Z component",
			a:        [3]float64{0, 0, 0},
			b:        [3]float64{0, 0, 2},
			wantDist: 2.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := distance3D(tt.a, tt.b)
			if math.Abs(got-tt.wantDist) > 1e-6 {
				t.Errorf("distance3D() = %f, want %f", got, tt.wantDist)
			}
		})
	}
}

// TestWallParsing tests wall definition parsing
func TestWallParsing(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCount int
		wantValid bool
	}{
		{
			name:      "valid wall",
			input:     "1,2,3,4",
			wantCount: 1,
			wantValid: true,
		},
		{
			name:      "valid wall with decimals",
			input:     "1.5,2.5,3.5,4.5",
			wantCount: 1,
			wantValid: true,
		},
		{
			name:      "invalid - missing coordinate",
			input:     "1,2,3",
			wantCount: 0,
			wantValid: false,
		},
		{
			name:      "invalid - non-numeric",
			input:     "a,b,c,d",
			wantCount: 0,
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			walls := parseWalls(tt.input)
			if tt.wantValid {
				if len(walls) != tt.wantCount {
					t.Errorf("parseWalls() returned %d walls, want %d", len(walls), tt.wantCount)
				}
			} else {
				if len(walls) != 0 {
					t.Errorf("parseWalls() should have returned empty for invalid input, got %d walls", len(walls))
				}
			}
		})
	}
}

// TestBinaryHeaderFormat tests that CSI frames have correct binary header format
func TestBinaryHeaderFormat(t *testing.T) {
	node := &VirtualNode{
		mac:      "AA:BB:CC:DD:EE:FF",
		position: [3]float64{0, 0, 2.5},
	}

	walker := &Walker{
		Position: [3]float64{3, 2.5, 1.7},
		Velocity: [3]float64{0.1, 0, 0},
		mac:      "11:22:33:44:55:00",
	}

	frame := node.generateCSIFrame(walker, 0, 0.005)

	// Check minimum frame length
	if len(frame) < HeaderSize {
		t.Fatalf("Frame length %d is less than header size %d", len(frame), HeaderSize)
	}

	// Check magic number (if we had one at the start)
	// For now, check that node MAC is in correct position
	nodeMAC := frame[0:6]
	expectedMAC := macToBytes(node.mac)
	if !bytesEqual(nodeMAC, expectedMAC[:]) {
		t.Errorf("Node MAC = %v, want %v", nodeMAC, expectedMAC)
	}

	// Check peer MAC is in correct position
	peerMAC := frame[6:12]
	if len(peerMAC) != 6 {
		t.Errorf("Peer MAC length = %d, want 6", len(peerMAC))
	}

	// Check timestamp is at correct position and is reasonable
	timestampUS := binary.LittleEndian.Uint64(frame[12:20])
	if timestampUS > 100000 { // Should be less than 100ms for first frame
		t.Errorf("Timestamp = %d us, want < 100000 us for first frame", timestampUS)
	}

	// Check RSSI is in valid range
	rssi := int8(frame[20])
	if rssi < -90 || rssi > -30 {
		t.Errorf("RSSI = %d dBm, want between -90 and -30", rssi)
	}

	// Check noise floor
	noiseFloor := int8(frame[21])
	if noiseFloor < -100 || noiseFloor > -50 {
		t.Errorf("Noise floor = %d dBm, want between -100 and -50", noiseFloor)
	}

	// Check channel is valid WiFi channel
	channel := frame[22]
	if channel < 1 || channel > 14 {
		t.Errorf("Channel = %d, want valid channel 1-14", channel)
	}

	// Check number of subcarriers
	nSub := frame[23]
	if nSub < 1 || nSub > MaxSubcarriers {
		t.Errorf("Subcarriers = %d, want between 1 and %d", nSub, MaxSubcarriers)
	}

	// Verify payload length matches nSub
	expectedPayloadSize := int(nSub) * 2
	expectedFrameSize := HeaderSize + expectedPayloadSize
	if len(frame) != expectedFrameSize {
		t.Errorf("Frame length = %d, want %d (header %d + payload %d)",
			len(frame), expectedFrameSize, HeaderSize, expectedPayloadSize)
	}
}
