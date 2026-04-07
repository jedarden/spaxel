// Package ingestion handles WebSocket connections from ESP32 nodes
package ingestion

import (
	"encoding/binary"
	"fmt"
	"log"
)

// Frame constants from the plan
const (
	HeaderSize     = 24 // Fixed header size
	MaxPayloadSize = 128 * 2
	MaxFrameSize   = HeaderSize + MaxPayloadSize
	MinFrameSize   = HeaderSize
)

// CSIFrame represents a parsed CSI binary frame
// Header (fixed 24 bytes):
//   node_mac:     6 bytes  — source node MAC
//   peer_mac:     6 bytes  — transmitting peer MAC
//   timestamp_us: 8 bytes  — uint64, microseconds since node boot
//   rssi:         1 byte   — int8, dBm
//   noise_floor:  1 byte   — int8, dBm
//   channel:      1 byte   — uint8, WiFi channel
//   n_sub:        1 byte   — uint8, subcarrier count
// Payload (n_sub × 2 bytes):
//   Per subcarrier: int8 I, int8 Q
type CSIFrame struct {
	NodeMAC     [6]byte
	PeerMAC     [6]byte
	TimestampUS uint64
	RSSI        int8
	NoiseFloor  int8
	Channel     uint8
	NSub        uint8
	Payload     []int8 // Interleaved I,Q pairs (length = NSub * 2)
}

// ParseFrame parses a binary WebSocket frame into a CSIFrame
// Returns nil and an error if the frame is malformed.
// Logs at DEBUG level for each validation failure to aid debugging
// without flooding logs at high frame rates.
func ParseFrame(data []byte) (*CSIFrame, error) {
	// Validation rule 1: minimum length
	if len(data) < MinFrameSize {
		log.Printf("[DEBUG] CSI frame validation failed: too short (%d bytes < %d minimum)", len(data), MinFrameSize)
		return nil, fmt.Errorf("frame too short: %d bytes (minimum %d)", len(data), MinFrameSize)
	}

	// Read header fields
	var frame CSIFrame
	copy(frame.NodeMAC[:], data[0:6])
	copy(frame.PeerMAC[:], data[6:12])
	frame.TimestampUS = binary.LittleEndian.Uint64(data[12:20])
	frame.RSSI = int8(data[20])
	frame.NoiseFloor = int8(data[21])
	frame.Channel = uint8(data[22])
	frame.NSub = uint8(data[23])

	// Validation rule 2: n_sub read from byte 23
	nSub := frame.NSub

	// Validation rule 3: payload length must match
	expectedLen := HeaderSize + int(nSub)*2
	if len(data) != expectedLen {
		log.Printf("[DEBUG] CSI frame validation failed: payload length mismatch (n_sub=%d, expected %d bytes, got %d)", nSub, expectedLen, len(data))
		return nil, fmt.Errorf("payload length mismatch: expected %d bytes, got %d", expectedLen, len(data))
	}

	// Validation rule 4: n_sub must not exceed 128
	if nSub > 128 {
		log.Printf("[DEBUG] CSI frame validation failed: implausible subcarrier count (n_sub=%d > 128 max)", nSub)
		return nil, fmt.Errorf("implausible subcarrier count: %d (max 128)", nSub)
	}

	// Validation rule 5: rssi == 0 is allowed but logged at DEBUG (invalid RSSI per firmware spec)
	// The frame is still processed, but the signal pipeline should skip AGC normalization
	if frame.RSSI == 0 {
		log.Printf("[DEBUG] CSI frame has RSSI=0 (invalid/missing); AGC normalization will be skipped")
	}

	// Validation rule 6: channel must be valid (1-14 for 2.4 GHz)
	if frame.Channel == 0 {
		log.Printf("[DEBUG] CSI frame validation failed: channel=0 is invalid")
		return nil, fmt.Errorf("invalid channel: %d", frame.Channel)
	}
	if frame.Channel > 14 {
		log.Printf("[DEBUG] CSI frame validation failed: channel=%d > 14 (invalid 2.4 GHz channel)", frame.Channel)
		return nil, fmt.Errorf("invalid channel: %d", frame.Channel)
	}

	// Parse payload (I,Q pairs as int8)
	if nSub > 0 {
		frame.Payload = make([]int8, int(nSub)*2)
		payloadData := data[HeaderSize:]
		for i := range frame.Payload {
			frame.Payload[i] = int8(payloadData[i])
		}
	}

	return &frame, nil
}

// MACString returns the node MAC as a colon-separated hex string
func (f *CSIFrame) MACString() string {
	return macToString(f.NodeMAC)
}

// PeerMACString returns the peer MAC as a colon-separated hex string
func (f *CSIFrame) PeerMACString() string {
	return macToString(f.PeerMAC)
}

// LinkID returns a unique identifier for this link (node_mac:peer_mac)
func (f *CSIFrame) LinkID() string {
	return fmt.Sprintf("%s:%s", f.MACString(), f.PeerMACString())
}

// macToString converts a 6-byte MAC to uppercase colon-separated hex
func macToString(mac [6]byte) string {
	return fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
		mac[0], mac[1], mac[2], mac[3], mac[4], mac[5])
}
