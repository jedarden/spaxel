// Package main provides tests for the CSI simulator.
package main

import (
	"encoding/binary"
	"math"
	"testing"
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
