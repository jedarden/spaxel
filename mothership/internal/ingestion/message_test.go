package ingestion

import (
	"encoding/json"
	"testing"
)

func TestParseJSONMessage_Hello(t *testing.T) {
	data := `{"type":"hello","mac":"AA:BB:CC:DD:EE:FF","firmware_version":"1.0.0","capabilities":["csi","ble","tx","rx"],"chip":"ESP32-S3","flash_mb":16,"uptime_ms":4200}`

	msg, err := ParseJSONMessage([]byte(data))
	if err != nil {
		t.Fatalf("ParseJSONMessage failed: %v", err)
	}

	hello, ok := msg.(*HelloMessage)
	if !ok {
		t.Fatal("Expected HelloMessage type")
	}

	if hello.MAC != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("MAC mismatch: got %s", hello.MAC)
	}

	if hello.FirmwareVersion != "1.0.0" {
		t.Errorf("FirmwareVersion mismatch: got %s", hello.FirmwareVersion)
	}

	if len(hello.Capabilities) != 4 {
		t.Errorf("Capabilities count: got %d, want 4", len(hello.Capabilities))
	}

	if hello.Chip != "ESP32-S3" {
		t.Errorf("Chip mismatch: got %s", hello.Chip)
	}

	if hello.FlashMB != 16 {
		t.Errorf("FlashMB mismatch: got %d", hello.FlashMB)
	}
}

func TestParseJSONMessage_Health(t *testing.T) {
	data := `{"type":"health","mac":"AA:BB:CC:DD:EE:FF","timestamp_ms":1711234567890,"free_heap_bytes":204800,"wifi_rssi_dbm":-52,"uptime_ms":3600000,"temperature_c":42.1,"csi_rate_hz":20,"wifi_channel":6,"ip":"192.168.1.123"}`

	msg, err := ParseJSONMessage([]byte(data))
	if err != nil {
		t.Fatalf("ParseJSONMessage failed: %v", err)
	}

	health, ok := msg.(*HealthMessage)
	if !ok {
		t.Fatal("Expected HealthMessage type")
	}

	if health.MAC != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("MAC mismatch: got %s", health.MAC)
	}

	if health.FreeHeapBytes != 204800 {
		t.Errorf("FreeHeapBytes mismatch: got %d", health.FreeHeapBytes)
	}

	if health.WifiRSSIdBm != -52 {
		t.Errorf("WifiRSSIdBm mismatch: got %d", health.WifiRSSIdBm)
	}

	if health.TemperatureC != 42.1 {
		t.Errorf("TemperatureC mismatch: got %f", health.TemperatureC)
	}
}

func TestParseJSONMessage_BLE(t *testing.T) {
	data := `{"type":"ble","mac":"AA:BB:CC:DD:EE:FF","timestamp_ms":1711234567890,"devices":[{"addr":"AA:BB:CC:DD:EE:00","addr_type":"public","rssi_dbm":-62,"name":"iPhone"}]}`

	msg, err := ParseJSONMessage([]byte(data))
	if err != nil {
		t.Fatalf("ParseJSONMessage failed: %v", err)
	}

	ble, ok := msg.(*BLEMessage)
	if !ok {
		t.Fatal("Expected BLEMessage type")
	}

	if len(ble.Devices) != 1 {
		t.Fatalf("Devices count: got %d, want 1", len(ble.Devices))
	}

	if ble.Devices[0].Name != "iPhone" {
		t.Errorf("Device name mismatch: got %s", ble.Devices[0].Name)
	}

	if ble.Devices[0].RSSIdBm != -62 {
		t.Errorf("Device RSSI mismatch: got %d", ble.Devices[0].RSSIdBm)
	}
}

func TestParseJSONMessage_MotionHint(t *testing.T) {
	data := `{"type":"motion_hint","mac":"AA:BB:CC:DD:EE:FF","timestamp_ms":1711234567890,"variance":0.043}`

	msg, err := ParseJSONMessage([]byte(data))
	if err != nil {
		t.Fatalf("ParseJSONMessage failed: %v", err)
	}

	motion, ok := msg.(*MotionHintMessage)
	if !ok {
		t.Fatal("Expected MotionHintMessage type")
	}

	if motion.Variance != 0.043 {
		t.Errorf("Variance mismatch: got %f", motion.Variance)
	}
}

func TestParseJSONMessage_OTAStatus(t *testing.T) {
	data := `{"type":"ota_status","mac":"AA:BB:CC:DD:EE:FF","state":"downloading","progress_pct":45}`

	msg, err := ParseJSONMessage([]byte(data))
	if err != nil {
		t.Fatalf("ParseJSONMessage failed: %v", err)
	}

	ota, ok := msg.(*OTAStatusMessage)
	if !ok {
		t.Fatal("Expected OTAStatusMessage type")
	}

	if ota.State != "downloading" {
		t.Errorf("State mismatch: got %s", ota.State)
	}

	if ota.ProgressPct != 45 {
		t.Errorf("ProgressPct mismatch: got %d", ota.ProgressPct)
	}
}

func TestParseJSONMessage_Unknown(t *testing.T) {
	data := `{"type":"unknown_type","mac":"AA:BB:CC:DD:EE:FF"}`

	_, err := ParseJSONMessage([]byte(data))
	if err == nil {
		t.Error("Expected error for unknown message type")
	}
}

func TestParseJSONMessage_Invalid(t *testing.T) {
	data := `{"type":`

	_, err := ParseJSONMessage([]byte(data))
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestToJSON_RoleMessage(t *testing.T) {
	msg := RoleMessage{Type: "role", Role: "rx"}
	data, err := ToJSON(msg)
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	// Verify it's valid JSON
	var parsed RoleMessage
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Result is not valid JSON: %v", err)
	}

	if parsed.Role != "rx" {
		t.Errorf("Role mismatch: got %s", parsed.Role)
	}
}

func TestToJSON_ConfigMessage(t *testing.T) {
	rate := 50
	msg := ConfigMessage{Type: "config", RateHz: &rate}
	data, err := ToJSON(msg)
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	var parsed ConfigMessage
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Result is not valid JSON: %v", err)
	}

	if parsed.RateHz == nil || *parsed.RateHz != 50 {
		t.Errorf("RateHz mismatch")
	}
}

func TestToJSON_RejectMessage(t *testing.T) {
	msg := RejectMessage{Type: "reject", Reason: "invalid_token"}
	data, err := ToJSON(msg)
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	var parsed RejectMessage
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Result is not valid JSON: %v", err)
	}

	if parsed.Reason != "invalid_token" {
		t.Errorf("Reason mismatch: got %s", parsed.Reason)
	}
}
