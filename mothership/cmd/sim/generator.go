// Package main provides CSI frame generation for the simulator.
package main

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/spaxel/mothership/internal/simulator"
)

const (
	// CSI frame header size (24 bytes per Phase 1 protocol)
	HeaderSize = 24

	// Magic number for CSI frame identification
	CSIMagic = 0xABCDEF01

	// Protocol version
	CSIVersion = 1

	// Default number of subcarriers (HT20)
	DefaultSubcarriers = 52

	// Maximum subcarriers (HT20 64 total)
	MaxSubcarriers = 64

	// WiFi channels
	DefaultChannel = 6 // 2.4 GHz channel 6
	Channel5GHzStart = 36 // 5 GHz channel 36
)

// CSIFrameGenerator generates synthetic CSI binary frames
type CSIFrameGenerator struct {
	nodeMAC      []byte
	nodePosition simulator.Point
	peerMAC      []byte
	peerPosition simulator.Point
	channel      uint8
	frameIndex   uint64
	physics      *simulator.PhysicsModel
}

// NewCSIFrameGenerator creates a new CSI frame generator
func NewCSIFrameGenerator(nodeMAC string, nodePos simulator.Point, physics *simulator.PhysicsModel) *CSIFrameGenerator {
	macBytes := macStringToBytes(nodeMAC)

	return &CSIFrameGenerator{
		nodeMAC:      macBytes[:],
		nodePosition: nodePos,
		peerMAC:      []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x00}, // Default peer
		peerPosition: simulator.Point{X: 0, Y: 0, Z: 1.7},
		channel:      DefaultChannel,
		frameIndex:   0,
		physics:      physics,
	}
}

// SetPeer sets the peer (transmitter) MAC and position
func (g *CSIFrameGenerator) SetPeer(peerMAC string, pos simulator.Point) {
	macBytes := macStringToBytes(peerMAC)
	copy(g.peerMAC, macBytes[:])
	g.peerPosition = pos
}

// SetChannel sets the WiFi channel
func (g *CSIFrameGenerator) SetChannel(channel uint8) {
	g.channel = channel
}

// SetFrameIndex sets the current frame index
func (g *CSIFrameGenerator) SetFrameIndex(index uint64) {
	g.frameIndex = index
}

// GenerateFrame generates a synthetic CSI frame for a walker at the given position
// Returns the complete binary frame ready to send over WebSocket
func (g *CSIFrameGenerator) GenerateFrame(walkerPos simulator.Point) []byte {
	nSub := DefaultSubcarriers

	// Calculate frame size
	frameSize := HeaderSize + nSub*2
	buf := make([]byte, frameSize)

	// Calculate distance to walker for RSSI
	distance := g.nodePosition.Distance(walkerPos)
	rssi := g.physics.ComputeRSSI(distance)

	// Calculate Fresnel modulation
	fresnelMod := simulator.ComputeFresnelModulation(g.nodePosition, g.peerPosition, walkerPos)

	// Write header
	g.writeHeader(buf, nSub, rssi, walkerPos)

	// Generate subcarrier CSI payload
	g.writePayload(buf[HeaderSize:], walkerPos, fresnelMod, nSub)

	// Increment frame index
	g.frameIndex++

	return buf
}

// writeHeader writes the 24-byte CSI frame header matching the Phase 1 protocol
func (g *CSIFrameGenerator) writeHeader(buf []byte, nSub int, rssi int8, walkerPos simulator.Point) {
	// According to the task, header is 24 bytes:
	// Bytes 0-5: Node MAC (6 bytes)
	// Bytes 6-11: Peer MAC (6 bytes)
	// Bytes 12-19: Timestamp_us (8 bytes, uint64)
	// Byte 20: RSSI (1 byte, signed)
	// Byte 21: Noise floor (1 byte, signed)
	// Byte 22: Channel (1 byte)
	// Byte 23: Num subcarriers (1 byte)

	// Bytes 0-5: Node MAC
	copy(buf[0:6], g.nodeMAC)

	// Bytes 6-11: Peer MAC
	copy(buf[6:12], g.peerMAC)

	// Bytes 12-19: Timestamp microseconds (little-endian uint64)
	// Default to 20 Hz for timestamp calculation
	timestampUS := g.frameIndex * (1_000_000 / 20)
	binary.LittleEndian.PutUint64(buf[12:20], timestampUS)

	// Byte 20: RSSI (signed int8)
	buf[20] = byte(rssi)

	// Byte 21: Noise floor (signed int8) - typical -95 dBm
	buf[21] = 0xA1 // -95 as signed int8 bit pattern

	// Byte 22: Channel
	buf[22] = g.channel

	// Byte 23: Number of subcarriers
	buf[23] = byte(nSub)
}

// writePayload writes the I/Q payload for each subcarrier
func (g *CSIFrameGenerator) writePayload(payload []byte, walkerPos simulator.Point, fresnelMod float64, nSub int) {
	// Get base amplitude from deltaRMS
	deltaRMS := g.physics.DeltaRMS(g.peerPosition, g.nodePosition, walkerPos)
	amplitude := deltaRMS * 500.0 // Scale to reasonable I/Q range

	// Apply Fresnel modulation
	modulation := 1.0 + fresnelMod*2.0 // Enhance signal when in zone 1
	scaledAmplitude := amplitude * modulation

	// Generate I/Q pairs for each subcarrier
	for k := 0; k < nSub; k++ {
		// Compute phase at this subcarrier using physics model
		phase := g.physics.PhaseAtSubcarrier(
			g.peerPosition,
			g.nodePosition,
			walkerPos,
			k,
			int(g.frameIndex),
		)

		// Add subcarrier-dependent amplitude variation (frequency-selective fading)
		freqFading := 0.8 + 0.4*math.Sin(2*math.Pi*float64(k)/16.0)
		subAmplitude := scaledAmplitude * freqFading

		// Generate I/Q pair with noise
		i, q := g.physics.GenerateIQPair(subAmplitude, phase)

		payload[k*2] = byte(i)
		payload[k*2+1] = byte(q)
	}
}

// macStringToBytes converts MAC address string to byte slice
func macStringToBytes(mac string) [6]byte {
	var b [6]byte
	fmt.Sscanf(mac, "%02X:%02X:%02X:%02X:%02X:%02X",
		&b[0], &b[1], &b[2], &b[3], &b[4], &b[5])
	return b
}

// ValidateFrame validates a generated CSI frame
func ValidateFrame(frame []byte) error {
	// Check minimum frame length
	if len(frame) < HeaderSize {
		return fmt.Errorf("frame too short: %d bytes (minimum %d)", len(frame), HeaderSize)
	}

	// Get n_sub from byte 23
	nSub := int(frame[23])

	// Validate payload length matches
	expectedLen := HeaderSize + nSub*2
	if len(frame) != expectedLen {
		return fmt.Errorf("payload length mismatch: got %d, expected %d (n_sub=%d)",
			len(frame), expectedLen, nSub)
	}

	// Validate n_sub range
	if nSub > MaxSubcarriers {
		return fmt.Errorf("n_sub too large: %d (max %d)", nSub, MaxSubcarriers)
	}

	// Validate channel is valid WiFi channel (1-14 for 2.4 GHz)
	channel := frame[22]
	if channel < 1 || channel > 14 {
		return fmt.Errorf("invalid channel: %d (valid range 1-14)", channel)
	}

	return nil
}

// GetRSSIFromFrame extracts RSSI from a CSI frame
func GetRSSIFromFrame(frame []byte) (int8, error) {
	if len(frame) < HeaderSize {
		return 0, fmt.Errorf("frame too short")
	}
	return int8(frame[20]), nil
}

// GetChannelFromFrame extracts channel from a CSI frame
func GetChannelFromFrame(frame []byte) (uint8, error) {
	if len(frame) < HeaderSize {
		return 0, fmt.Errorf("frame too short")
	}
	return frame[22], nil
}

// GetTimestampFromFrame extracts timestamp from a CSI frame
func GetTimestampFromFrame(frame []byte) (uint64, error) {
	if len(frame) < HeaderSize {
		return 0, fmt.Errorf("frame too short")
	}
	return binary.LittleEndian.Uint64(frame[12:20]), nil
}

// GetSubcarrierCount extracts number of subcarriers from a CSI frame
func GetSubcarrierCount(frame []byte) (int, error) {
	if len(frame) < HeaderSize {
		return 0, fmt.Errorf("frame too short")
	}
	return int(frame[23]), nil
}

// GetIQPair extracts I and Q values for a specific subcarrier
func GetIQPair(frame []byte, subcarrierIndex int) (int8, int8, error) {
	if len(frame) < HeaderSize {
		return 0, 0, fmt.Errorf("frame too short")
	}

	nSub := int(frame[23])
	if subcarrierIndex >= nSub {
		return 0, 0, fmt.Errorf("subcarrier index %d out of range (n_sub=%d)",
			subcarrierIndex, nSub)
	}

	offset := HeaderSize + subcarrierIndex*2
	if offset+2 > len(frame) {
		return 0, 0, fmt.Errorf("frame truncated reading subcarrier %d", subcarrierIndex)
	}

	i := int8(frame[offset])
	q := int8(frame[offset+1])

	return i, q, nil
}

// ComputeExpectedRSSI computes expected RSSI for a given distance
// using the same physics model as the frame generator
func ComputeExpectedRSSI(distance float64, physics *simulator.PhysicsModel) int8 {
	return physics.ComputeRSSI(distance)
}

// DistanceToWalker computes distance from node to walker
func DistanceToWalker(nodePos, walkerPos simulator.Point) float64 {
	dx := walkerPos.X - nodePos.X
	dy := walkerPos.Y - nodePos.Y
	dz := walkerPos.Z - nodePos.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}
