package ingestion

import (
	"encoding/json"
	"fmt"
)

// Upstream messages (node -> mothership)

// HelloMessage is sent by the node on WebSocket connect
type HelloMessage struct {
	Type            string   `json:"type"`
	MAC             string   `json:"mac"`
	NodeID          string   `json:"node_id,omitempty"`
	FirmwareVersion string   `json:"firmware_version"`
	Capabilities    []string `json:"capabilities"`
	Chip            string   `json:"chip,omitempty"`
	FlashMB         int      `json:"flash_mb,omitempty"`
	UptimeMS        int64    `json:"uptime_ms,omitempty"`
	APBSSID         string   `json:"ap_bssid,omitempty"`
	APChannel       int      `json:"ap_channel,omitempty"`
}

// HealthMessage is sent every 10 seconds
type HealthMessage struct {
	Type           string `json:"type"`
	MAC            string `json:"mac"`
	TimestampMS    int64  `json:"timestamp_ms"`
	FreeHeapBytes  int64  `json:"free_heap_bytes"`
	WifiRSSIdBm    int    `json:"wifi_rssi_dbm"`
	UptimeMS       int64  `json:"uptime_ms"`
	TemperatureC   float64 `json:"temperature_c,omitempty"`
	CSIRateHz      int    `json:"csi_rate_hz"`
	WifiChannel    int    `json:"wifi_channel"`
	IP             string `json:"ip,omitempty"`
	NTPSynced      bool   `json:"ntp_synced"`
}

// BLEDevice represents a discovered BLE device
type BLEDevice struct {
	Addr        string `json:"addr"`
	AddrType    string `json:"addr_type,omitempty"`
	RSSIdBm     int    `json:"rssi_dbm"`
	Name        string `json:"name,omitempty"`
	MfrID       int    `json:"mfr_id,omitempty"`
	MfrDataHex  string `json:"mfr_data_hex,omitempty"`
}

// BLEMessage is sent every 5 seconds with discovered devices
type BLEMessage struct {
	Type        string      `json:"type"`
	MAC         string      `json:"mac"`
	TimestampMS int64       `json:"timestamp_ms"`
	Devices     []BLEDevice `json:"devices"`
}

// MotionHintMessage is sent when on-device variance exceeds threshold
type MotionHintMessage struct {
	Type        string  `json:"type"`
	MAC         string  `json:"mac"`
	TimestampMS int64   `json:"timestamp_ms"`
	Variance    float64 `json:"variance"`
}

// OTAStatusMessage reports OTA progress
type OTAStatusMessage struct {
	Type        string `json:"type"`
	MAC         string `json:"mac"`
	State       string `json:"state"` // downloading | verifying | writing | rebooting | failed
	ProgressPct int    `json:"progress_pct,omitempty"`
	Error       string `json:"error,omitempty"`
}

// Downstream messages (mothership -> node)

// RoleMessage assigns operational role to a node
type RoleMessage struct {
	Type         string `json:"type"`
	Role         string `json:"role"` // tx | rx | tx_rx | passive | idle
	PassiveBSSID string `json:"passive_bssid,omitempty"`
}

// ConfigMessage changes operational parameters
type ConfigMessage struct {
	Type              string  `json:"type"`
	RateHz            *int    `json:"rate_hz,omitempty"`
	TXSlotUS          *int    `json:"tx_slot_us,omitempty"`
	VarianceThreshold *float64 `json:"variance_threshold,omitempty"`
	NTPServer         *string `json:"ntp_server,omitempty"`
}

// OTAMessage triggers a firmware update
type OTAMessage struct {
	Type    string `json:"type"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
	Version string `json:"version"`
}

// RebootMessage triggers a node reboot
type RebootMessage struct {
	Type     string `json:"type"`
	DelayMS  int    `json:"delay_ms,omitempty"`
}

// IdentifyMessage triggers LED blink for identification
type IdentifyMessage struct {
	Type       string `json:"type"`
	DurationMS int    `json:"duration_ms,omitempty"`
}

// BaselineRequestMessage requests baseline data from node
type BaselineRequestMessage struct {
	Type string `json:"type"`
}

// ShutdownMessage notifies node of mothership shutdown
type ShutdownMessage struct {
	Type          string `json:"type"`
	ReconnectInMS int    `json:"reconnect_in_ms"`
}

// RejectMessage rejects a connection
type RejectMessage struct {
	Type   string `json:"type"`
	Reason string `json:"reason"` // invalid_token | unknown_node | rate_limited
}

// ParseJSONMessage parses a JSON message and returns the appropriate type
func ParseJSONMessage(data []byte) (interface{}, error) {
	// First, extract the type field
	var base struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &base); err != nil {
		return nil, fmt.Errorf("failed to parse message type: %w", err)
	}

	switch base.Type {
	case "hello":
		var msg HelloMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return &msg, nil
	case "health":
		var msg HealthMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return &msg, nil
	case "ble":
		var msg BLEMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return &msg, nil
	case "motion_hint":
		var msg MotionHintMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return &msg, nil
	case "ota_status":
		var msg OTAStatusMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return &msg, nil
	default:
		// Unknown type - per protocol, silently ignore but return raw
		return nil, fmt.Errorf("unknown message type: %s", base.Type)
	}
}

// ToJSON serializes a downstream message to JSON bytes
func ToJSON(msg interface{}) ([]byte, error) {
	return json.Marshal(msg)
}
