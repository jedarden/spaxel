package ingestion

import (
	"encoding/binary"
	"testing"
)

// FuzzParseBinaryFrame is a property-based fuzz test for binary CSI frame parsing.
// The key property is: the parser must never panic on any input.
// It may return an error for malformed frames, but it must not crash.
func FuzzParseBinaryFrame(f *testing.F) {
	// Seed corpus: various valid and malformed frame patterns

	// 1. Valid minimal frame (n_sub=0, header only)
	validMinimal := makeValidFrame(0, 6, -50, -95, 1)
	f.Add(validMinimal)

	// 2. Valid typical frame (n_sub=64)
	validTypical := makeValidFrame(64, 6, -50, -95, 1)
	f.Add(validTypical)

	// 3. Truncated header (less than 24 bytes)
	f.Add([]byte{1, 2, 3, 4, 5})

	// 4. n_sub mismatch (header says 64 but payload has different size)
	mismatchPayload := makeValidFrame(64, 6, -50, -95, 1)
	mismatchPayload = mismatchPayload[:20] // truncate after header
	f.Add(mismatchPayload)

	// 5. channel=0 (invalid channel)
	channelZero := makeValidFrame(64, 6, -50, -95, 0)
	f.Add(channelZero)

	// 6. n_sub > 128 (implausible subcarrier count)
	f.Add(makeValidFrame(200, 6, -50, -95, 1))

	// 7. Maximum valid n_sub (128)
	maxNSub := makeValidFrame(128, 6, -50, -95, 6)
	f.Add(maxNSub)

	// 8. RSSI=0 (invalid but allowed)
	rssiZero := makeValidFrame(64, 6, 0, -95, 1)
	f.Add(rssiZero)

	// 9. All zeros frame
	f.Add(make([]byte, 24))

	// 10. Random-ish valid frame
	f.Add(makeValidFrame(47, 6, -45, -90, 11))

	// 11. Frame with maximum channel (14)
	channelMax := makeValidFrame(64, 6, -50, -95, 14)
	f.Add(channelMax)

	// 12. Frame with n_sub=1 (minimum with payload)
	nSubOne := makeValidFrame(1, 6, -50, -95, 1)
	f.Add(nSubOne)

	f.Fuzz(func(t *testing.T, data []byte) {
		// The key property: ParseFrame must never panic
		// It may return an error for invalid frames, but must not crash
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("ParseFrame panicked with input: %v\nPanic: %v", len(data), r)
			}
		}()

		// Call ParseFrame - it should handle any input gracefully
		_, _ = ParseFrame(data)

		// If we get here without panic, the property holds
		// The frame may be:
		// - Successfully parsed (returned *CSIFrame, nil)
		// - Rejected with error (returned nil, error)
		// Both are acceptable outcomes
	})
}

// makeValidFrame creates a valid CSI binary frame for testing
func makeValidFrame(nSub int, channel int, rssi int8, noiseFloor int8, channelNum uint8) []byte {
	frame := make([]byte, HeaderSize+nSub*2)

	// node_mac (6 bytes) - use test MAC
	copy(frame[0:6], []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF})

	// peer_mac (6 bytes) - use test peer MAC
	copy(frame[6:12], []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66})

	// timestamp_us (8 bytes) - little endian uint64
	binary.LittleEndian.PutUint64(frame[12:20], 1234567890)

	// rssi (1 byte)
	frame[20] = byte(rssi)

	// noise_floor (1 byte)
	frame[21] = byte(noiseFloor)

	// channel (1 byte)
	frame[22] = channelNum

	// n_sub (1 byte)
	frame[23] = byte(nSub)

	// Payload: fill with plausible I/Q values
	for i := 0; i < nSub*2; i += 2 {
		// Use small int8 values for I and Q components
		frame[HeaderSize+i] = 10  // I
		frame[HeaderSize+i+1] = 5 // Q
	}

	return frame
}

// TestParseFrameProperty tests specific properties of ParseFrame
func TestParseFrameProperty(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantErr bool
	}{
		{
			name:    "valid minimal frame (n_sub=0)",
			input:   makeValidFrame(0, 6, -50, -95, 1),
			wantErr: false,
		},
		{
			name:    "valid typical frame (n_sub=64)",
			input:   makeValidFrame(64, 6, -50, -95, 1),
			wantErr: false,
		},
		{
			name:    "truncated header",
			input:   []byte{1, 2, 3, 4, 5},
			wantErr: true,
		},
		{
			name:    "channel=0 is invalid",
			input:   makeValidFrame(64, 6, -50, -95, 0),
			wantErr: true,
		},
		{
			name:    "channel > 14 is invalid",
			input:   makeValidFrame(64, 6, -50, -95, 15),
			wantErr: true,
		},
		{
			name:    "n_sub > 128 is invalid",
			input:   makeValidFrame(200, 6, -50, -95, 1),
			wantErr: true,
		},
		{
			name:    "payload length mismatch",
			input:   payloadMismatchFrame(),
			wantErr: true,
		},
		{
			name:    "rssi=0 is allowed but logged",
			input:   makeValidFrame(64, 6, 0, -95, 1),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame, err := ParseFrame(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseFrame() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && frame != nil && len(tt.input) >= 24 {
				// Verify parsed fields match input (only for valid-length frames)
				if frame.NSub != uint8(tt.input[23]) {
					t.Errorf("NSub mismatch: got %d, want %d", frame.NSub, tt.input[23])
				}
				if frame.Channel != uint8(tt.input[22]) {
					t.Errorf("Channel mismatch: got %d, want %d", frame.Channel, tt.input[22])
				}
			}
		})
	}
}

// payloadMismatchFrame creates a frame with n_sub=64 but truncated payload
func payloadMismatchFrame() []byte {
	frame := makeValidFrame(64, 6, -50, -95, 1)
	// Return only header + partial payload (triggers length mismatch)
	return frame[:HeaderSize+32]
}
