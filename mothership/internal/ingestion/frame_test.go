package ingestion

import (
	"testing"
)

func TestParseFrame_Valid(t *testing.T) {
	// Create a valid frame with 64 subcarriers
	nSub := uint8(64)
	payloadSize := int(nSub) * 2
	frameSize := HeaderSize + payloadSize

	data := make([]byte, frameSize)

	// Node MAC: AA:BB:CC:DD:EE:FF
	copy(data[0:6], []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF})

	// Peer MAC: 11:22:33:44:55:66
	copy(data[6:12], []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66})

	// Timestamp: 12345678 microseconds (little-endian uint64)
	data[12] = 0x4E
	data[13] = 0x61
	data[14] = 0xBC
	data[15] = 0x00
	// bytes 16-19 are 0

	// RSSI: -52 dBm (int8 = 204 = 0xCC)
	data[20] = 0xCC

	// Noise floor: -95 dBm (int8 = 161 = 0xA1)
	data[21] = 0xA1

	// Channel: 6
	data[22] = 0x06

	// n_sub: 64
	data[23] = nSub

	// Payload: alternating I, Q values
	for i := 0; i < payloadSize; i++ {
		data[HeaderSize+i] = byte(i % 256)
	}

	frame, err := ParseFrame(data)
	if err != nil {
		t.Fatalf("ParseFrame failed: %v", err)
	}

	// Verify header fields
	expectedNodeMAC := [6]byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}
	if frame.NodeMAC != expectedNodeMAC {
		t.Errorf("NodeMAC mismatch: got %v, want %v", frame.NodeMAC, expectedNodeMAC)
	}

	expectedPeerMAC := [6]byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66}
	if frame.PeerMAC != expectedPeerMAC {
		t.Errorf("PeerMAC mismatch: got %v, want %v", frame.PeerMAC, expectedPeerMAC)
	}

	if frame.TimestampUS != 0xBC614E {
		t.Errorf("TimestampUS mismatch: got %d, want %d", frame.TimestampUS, 0xBC614E)
	}

	if frame.RSSI != -52 {
		t.Errorf("RSSI mismatch: got %d, want -52", frame.RSSI)
	}

	if frame.NoiseFloor != -95 {
		t.Errorf("NoiseFloor mismatch: got %d, want -95", frame.NoiseFloor)
	}

	if frame.Channel != 6 {
		t.Errorf("Channel mismatch: got %d, want 6", frame.Channel)
	}

	if frame.NSub != 64 {
		t.Errorf("NSub mismatch: got %d, want 64", frame.NSub)
	}

	// Verify payload length
	if len(frame.Payload) != int(nSub)*2 {
		t.Errorf("Payload length mismatch: got %d, want %d", len(frame.Payload), int(nSub)*2)
	}

	// Verify MAC string format
	if frame.MACString() != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("MACString mismatch: got %s", frame.MACString())
	}

	if frame.PeerMACString() != "11:22:33:44:55:66" {
		t.Errorf("PeerMACString mismatch: got %s", frame.PeerMACString())
	}

	if frame.LinkID() != "AA:BB:CC:DD:EE:FF:11:22:33:44:55:66" {
		t.Errorf("LinkID mismatch: got %s", frame.LinkID())
	}
}

func TestParseFrame_TooShort(t *testing.T) {
	data := make([]byte, 10) // Less than header size
	_, err := ParseFrame(data)
	if err == nil {
		t.Error("Expected error for short frame")
	}
}

func TestParseFrame_PayloadMismatch(t *testing.T) {
	data := make([]byte, HeaderSize+10)
	data[23] = 64 // n_sub says 64, but only 10 bytes of payload
	_, err := ParseFrame(data)
	if err == nil {
		t.Error("Expected error for payload length mismatch")
	}
}

func TestParseFrame_InvalidChannel(t *testing.T) {
	tests := []struct {
		name    string
		channel uint8
	}{
		{"zero channel", 0},
		{"channel 15", 15},
		{"channel 255", 255},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, HeaderSize)
			data[22] = tt.channel
			data[23] = 0 // n_sub = 0 is valid (header-only frame)
			_, err := ParseFrame(data)
			if err == nil {
				t.Error("Expected error for invalid channel")
			}
		})
	}
}

func TestParseFrame_ExcessSubcarriers(t *testing.T) {
	data := make([]byte, HeaderSize+256)
	data[23] = 129 // n_sub > 128
	_, err := ParseFrame(data)
	if err == nil {
		t.Error("Expected error for excess subcarriers")
	}
}

func TestParseFrame_HeaderOnly(t *testing.T) {
	// Header-only frame (n_sub = 0) is valid per the spec
	data := make([]byte, HeaderSize)
	data[22] = 6 // valid channel
	data[23] = 0 // n_sub = 0

	frame, err := ParseFrame(data)
	if err != nil {
		t.Fatalf("ParseFrame failed: %v", err)
	}

	if frame.NSub != 0 {
		t.Errorf("NSub mismatch: got %d, want 0", frame.NSub)
	}

	if len(frame.Payload) != 0 {
		t.Errorf("Payload should be empty, got %d bytes", len(frame.Payload))
	}
}
