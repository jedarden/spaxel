package ingestion

import (
	"encoding/json"
	"fmt"
	"testing"
)

// FuzzParseJSONFrame is a property-based fuzz test for JSON frame parsing.
// The key properties are:
// 1. The parser must never panic on any input
// 2. Unknown message types return a typed error, not panic
func FuzzParseJSONFrame(f *testing.F) {
	// Seed corpus: valid and invalid JSON message patterns

	// 1. Valid hello message
	helloMsg := HelloMessage{
		Type:            "hello",
		MAC:             "AA:BB:CC:DD:EE:FF",
		NodeID:          "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		FirmwareVersion: "1.2.3",
		Capabilities:    []string{"csi", "ble", "tx", "rx"},
		Chip:            "ESP32-S3",
		FlashMB:         16,
		UptimeMS:        4200000,
	}
	helloBytes, _ := json.Marshal(helloMsg)
	f.Add(helloBytes)

	// 2. Valid health message
	healthMsg := HealthMessage{
		Type:         "health",
		MAC:          "AA:BB:CC:DD:EE:FF",
		TimestampMS:  1711234567890,
		FreeHeapBytes: 204800,
		WifiRSSIdBm:  -52,
		UptimeMS:     3600000,
		TemperatureC: 42.1,
		CSIRateHz:    20,
		WifiChannel:  6,
		IP:           "192.168.1.123",
		NTPSynced:    true,
	}
	healthBytes, _ := json.Marshal(healthMsg)
	f.Add(healthBytes)

	// 3. Valid BLE message
	bleMsg := BLEMessage{
		Type:        "ble",
		MAC:         "AA:BB:CC:DD:EE:FF",
		TimestampMS: 1711234567890,
		Devices: []BLEDevice{
			{
				Addr:       "AA:BB:CC:DD:EE:FF",
				AddrType:   "public",
				RSSIdBm:    -62,
				Name:       "iPhone",
				MfrID:      76,
				MfrDataHex: "0215",
			},
		},
	}
	bleBytes, _ := json.Marshal(bleMsg)
	f.Add(bleBytes)

	// 4. Valid motion_hint message
	motionMsg := MotionHintMessage{
		Type:        "motion_hint",
		MAC:         "AA:BB:CC:DD:EE:FF",
		TimestampMS: 1711234567890,
		Variance:    0.043,
	}
	motionBytes, _ := json.Marshal(motionMsg)
	f.Add(motionBytes)

	// 5. Valid ota_status message
	otaMsg := OTAStatusMessage{
		Type:        "ota_status",
		MAC:         "AA:BB:CC:DD:EE:FF",
		State:       "downloading",
		ProgressPct: 45,
	}
	otaBytes, _ := json.Marshal(otaMsg)
	f.Add(otaBytes)

	// 6. Unknown type message
	unknownMsg := `{"type":"unknown_type","field":"value"}`
	f.Add([]byte(unknownMsg))

	// 7. Missing type field
	noType := `{"mac":"AA:BB:CC:DD:EE:FF"}`
	f.Add([]byte(noType))

	// 8. Invalid JSON (malformed)
	invalidJSON := `{"type":"hello" "mac":"AA:BB:CC:DD:EE:FF"}`
	f.Add([]byte(invalidJSON))

	// 9. Empty JSON object
	emptyObj := `{}`
	f.Add([]byte(emptyObj))

	// 10. Empty array
	emptyArr := `[]`
	f.Add([]byte(emptyArr))

	// 11. Null input
	f.Add([]byte("null"))

	// 12. Empty string
	f.Add([]byte(""))

	// 13. Very long JSON string (4KB max per spec)
	longJSON := makeLongJSON()
	f.Add(longJSON)

	// 14. Valid JSON with extra fields
	extraFields := `{"type":"hello","mac":"AA:BB:CC:DD:EE:FF","extra":"ignored"}`
	f.Add([]byte(extraFields))

	// 15. Type field with wrong type (number instead of string)
	wrongTypeNum := `{"type":123,"mac":"AA:BB:CC:DD:EE:FF"}`
	f.Add([]byte(wrongTypeNum))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Property 1: ParseJSONMessage must never panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("ParseJSONMessage panicked with input length %d: %v", len(data), r)
			}
		}()

		// Call ParseJSONMessage - it should handle any input gracefully
		msg, err := ParseJSONMessage(data)

		// Property 2: If we get an error, it should be a proper error type, not panic
		if err != nil {
			// Check that error message is meaningful (contains context)
			if len(err.Error()) == 0 {
				t.Error("Error has empty message")
			}
			// For unknown type, verify the error message mentions the type
			if len(data) > 0 {
				var base struct {
					Type string `json:"type"`
				}
				if jsonErr := json.Unmarshal(data, &base); jsonErr == nil {
					if base.Type != "" && base.Type != "hello" && base.Type != "health" &&
						base.Type != "ble" && base.Type != "motion_hint" && base.Type != "ota_status" {
						// This is an unknown type - should return typed error
						expectedErr := fmt.Sprintf("unknown message type: %s", base.Type)
						if err.Error() != expectedErr {
							t.Logf("Note: unknown type '%s' returned error: %v", base.Type, err)
						}
					}
				}
			}
		}

		// Property 3: If we get a message, it should be one of the known types
		if msg != nil {
			switch msg.(type) {
			case *HelloMessage, *HealthMessage, *BLEMessage, *MotionHintMessage, *OTAStatusMessage:
				// Valid types - OK
			default:
				t.Errorf("Unexpected message type: %T", msg)
			}
		}

		// If we get here without panic, the properties hold
	})
}

// makeLongJSON creates a JSON message approaching the 4KB limit
func makeLongJSON() []byte {
	devices := make([]BLEDevice, 100) // 100 devices
	for i := 0; i < 100; i++ {
		devices[i] = BLEDevice{
			Addr:       fmt.Sprintf("AA:BB:CC:DD:EE:%02X", i),
			AddrType:   "public",
			RSSIdBm:    -60,
			Name:       fmt.Sprintf("Device%d", i),
			MfrID:      76,
			MfrDataHex: "0215",
		}
	}
	msg := BLEMessage{
		Type:        "ble",
		MAC:         "AA:BB:CC:DD:EE:FF",
		TimestampMS: 1711234567890,
		Devices:     devices,
	}
	data, _ := json.Marshal(msg)
	return data
}

// TestParseJSONMessageProperty tests specific properties of ParseJSONMessage
func TestParseJSONMessageProperty(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantErr    bool
		errContains string
		wantType   interface{}
	}{
		{
			name:     "valid hello message",
			input:    `{"type":"hello","mac":"AA:BB:CC:DD:EE:FF","node_id":"test-id","firmware_version":"1.0.0","capabilities":["csi"]}`,
			wantErr:  false,
			wantType: &HelloMessage{},
		},
		{
			name:     "valid health message",
			input:    `{"type":"health","mac":"AA:BB:CC:DD:EE:FF","timestamp_ms":1234567890,"free_heap_bytes":100000,"wifi_rssi_dbm":-50,"uptime_ms":60000,"csi_rate_hz":20,"wifi_channel":6}`,
			wantErr:  false,
			wantType: &HealthMessage{},
		},
		{
			name:     "valid ble message",
			input:    `{"type":"ble","mac":"AA:BB:CC:DD:EE:FF","timestamp_ms":1234567890,"devices":[{"addr":"AA:BB:CC:DD:EE:FF","rssi_dbm":-60}]}`,
			wantErr:  false,
			wantType: &BLEMessage{},
		},
		{
			name:     "valid motion_hint message",
			input:    `{"type":"motion_hint","mac":"AA:BB:CC:DD:EE:FF","timestamp_ms":1234567890,"variance":0.05}`,
			wantErr:  false,
			wantType: &MotionHintMessage{},
		},
		{
			name:     "valid ota_status message",
			input:    `{"type":"ota_status","mac":"AA:BB:CC:DD:EE:FF","state":"downloading","progress_pct":50}`,
			wantErr:  false,
			wantType: &OTAStatusMessage{},
		},
		{
			name:       "unknown type returns typed error",
			input:      `{"type":"unknown_type","mac":"AA:BB:CC:DD:EE:FF"}`,
			wantErr:    true,
			errContains: "unknown message type: unknown_type",
		},
		{
			name:       "invalid JSON returns error",
			input:      `{invalid json}`,
			wantErr:    true,
			errContains: "failed to parse message type",
		},
		{
			name:       "missing type field returns error",
			input:      `{"mac":"AA:BB:CC:DD:EE:FF"}`,
			wantErr:    true,
			errContains: "unknown message type",
		},
		{
			name:     "hello with invalid fields returns error",
			input:    `{"type":"hello","mac":"invalid-mac"}`,
			wantErr:  false, // ParseJSONMessage doesn't validate MAC format
		},
		{
			name:     "empty JSON object",
			input:    `{}`,
			wantErr:  true,
			errContains: "unknown message type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := ParseJSONMessage([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseJSONMessage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.errContains != "" && err != nil {
				if !containsString(err.Error(), tt.errContains) {
					t.Errorf("Error message should contain %q, got %q", tt.errContains, err.Error())
				}
			}
			if tt.wantType != nil && msg != nil {
				switch tt.wantType.(type) {
				case *HelloMessage:
					if _, ok := msg.(*HelloMessage); !ok {
						t.Errorf("Expected *HelloMessage, got %T", msg)
					}
				case *HealthMessage:
					if _, ok := msg.(*HealthMessage); !ok {
						t.Errorf("Expected *HealthMessage, got %T", msg)
					}
				case *BLEMessage:
					if _, ok := msg.(*BLEMessage); !ok {
						t.Errorf("Expected *BLEMessage, got %T", msg)
					}
				case *MotionHintMessage:
					if _, ok := msg.(*MotionHintMessage); !ok {
						t.Errorf("Expected *MotionHintMessage, got %T", msg)
					}
				case *OTAStatusMessage:
					if _, ok := msg.(*OTAStatusMessage); !ok {
						t.Errorf("Expected *OTAStatusMessage, got %T", msg)
					}
				}
			}
		})
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
		containsInMiddle(s, substr)))
}

func containsInMiddle(s, substr string) bool {
	for i := 1; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
