package dashboard

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/spaxel/mothership/internal/ingestion"
)

func TestHub_RegisterUnregister(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer func() {
		// Hub has no shutdown method in Phase 1, just let it run
	}()

	client := &Client{
		hub:  hub,
		send: make(chan []byte, 10),
	}

	// Register
	hub.Register(client)
	time.Sleep(10 * time.Millisecond)

	if hub.ClientCount() != 1 {
		t.Errorf("expected 1 client, got %d", hub.ClientCount())
	}

	// Unregister
	hub.Unregister(client)
	time.Sleep(10 * time.Millisecond)

	if hub.ClientCount() != 0 {
		t.Errorf("expected 0 clients, got %d", hub.ClientCount())
	}
}

// drainSnapshot reads and discards the initial snapshot message sent on connect.
func drainSnapshot(t *testing.T, ch chan []byte) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected snapshot message")
	}
}

func TestHub_Broadcast(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	client := &Client{
		hub:  hub,
		send: make(chan []byte, 10),
	}

	hub.Register(client)
	time.Sleep(10 * time.Millisecond)
	drainSnapshot(t, client.send)

	// Broadcast a message
	testMsg := []byte(`{"type":"test"}`)
	hub.Broadcast(testMsg)

	// Client should receive it
	select {
	case msg := <-client.send:
		if string(msg) != string(testMsg) {
			t.Errorf("expected %s, got %s", testMsg, msg)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected to receive broadcast message")
	}
}

func TestHub_BroadcastCSI(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	client := &Client{
		hub:  hub,
		send: make(chan []byte, 10),
	}

	hub.Register(client)
	time.Sleep(10 * time.Millisecond)
	drainSnapshot(t, client.send)

	// Broadcast CSI (raw binary data)
	testData := []byte{0x01, 0x02, 0x03, 0x04}
	hub.BroadcastCSI("AA:BB:CC:DD:EE:FF", "11:22:33:44:55:66", testData)

	select {
	case msg := <-client.send:
		if string(msg) != string(testData) {
			t.Errorf("expected %v, got %v", testData, msg)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected to receive CSI broadcast")
	}
}

func TestHub_NodeEvents(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	client := &Client{
		hub:  hub,
		send: make(chan []byte, 10),
	}

	hub.Register(client)
	time.Sleep(10 * time.Millisecond)
	drainSnapshot(t, client.send)

	// Test node connected event
	hub.BroadcastNodeConnected("AA:BB:CC:DD:EE:FF", "1.0.0", "ESP32-S3", false)

	msg := <-client.send
	expected := `{"chip":"ESP32-S3","firmware_version":"1.0.0","mac":"AA:BB:CC:DD:EE:FF","type":"node_connected","unpaired":false}`
	if string(msg) != expected {
		t.Errorf("expected %s, got %s", expected, msg)
	}

	// Test node disconnected event
	hub.BroadcastNodeDisconnected("AA:BB:CC:DD:EE:FF")

	msg = <-client.send
	expected = `{"mac":"AA:BB:CC:DD:EE:FF","type":"node_disconnected"}`
	if string(msg) != expected {
		t.Errorf("expected %s, got %s", expected, msg)
	}
}

func TestHub_LinkEvents(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	client := &Client{
		hub:  hub,
		send: make(chan []byte, 10),
	}

	hub.Register(client)
	time.Sleep(10 * time.Millisecond)
	drainSnapshot(t, client.send)

	// Test link active event
	hub.BroadcastLinkActive("AA:BB:CC:DD:EE:FF:11:22:33:44:55:66", "AA:BB:CC:DD:EE:FF", "11:22:33:44:55:66")

	msg := <-client.send
	expected := `{"id":"AA:BB:CC:DD:EE:FF:11:22:33:44:55:66","node_mac":"AA:BB:CC:DD:EE:FF","peer_mac":"11:22:33:44:55:66","type":"link_active"}`
	if string(msg) != expected {
		t.Errorf("expected %s, got %s", expected, msg)
	}
}

// MockIngestionState implements IngestionState for testing
type MockIngestionState struct {
	nodes []ingestion.NodeInfo
	links []ingestion.LinkInfo
}

func (m *MockIngestionState) GetConnectedNodesInfo() []ingestion.NodeInfo {
	return m.nodes
}

func (m *MockIngestionState) GetAllLinksInfo() []ingestion.LinkInfo {
	return m.links
}

func (m *MockIngestionState) GetAllMotionStates() []ingestion.MotionStateItem {
	return nil
}

func TestHub_SnapshotOnConnect(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	// Set mock ingestion state so the snapshot has content
	mock := &MockIngestionState{
		nodes: []ingestion.NodeInfo{
			{MAC: "AA:BB:CC:DD:EE:FF", FirmwareVersion: "1.0.0", Chip: "ESP32-S3"},
		},
		links: []ingestion.LinkInfo{
			{ID: "AA:BB:CC:DD:EE:FF:11:22:33:44:55:66", NodeMAC: "AA:BB:CC:DD:EE:FF", PeerMAC: "11:22:33:44:55:66"},
		},
	}
	hub.SetIngestionState(mock)

	client := &Client{
		hub:  hub,
		send: make(chan []byte, 10),
	}

	hub.Register(client)
	time.Sleep(10 * time.Millisecond)

	// First message must be the snapshot
	select {
	case msg := <-client.send:
		if len(msg) == 0 || msg[0] != '{' {
			t.Fatalf("expected JSON snapshot message, got %v", msg)
		}

		var parsed map[string]json.RawMessage
		if err := json.Unmarshal(msg, &parsed); err != nil {
			t.Fatalf("failed to parse snapshot JSON: %v", err)
		}

		// Must have type="snapshot"
		if typ, ok := parsed["type"]; !ok {
			t.Fatal("snapshot missing 'type' field")
		} else if strings.Trim(string(typ), `"`) != "snapshot" {
			t.Fatalf("expected type=snapshot, got %s", string(typ))
		}

		// Must have timestamp_ms
		if _, ok := parsed["timestamp_ms"]; !ok {
			t.Fatal("snapshot missing 'timestamp_ms' field")
		}

		// Must contain nodes from the mock
		if _, ok := parsed["nodes"]; !ok {
			t.Fatal("snapshot missing 'nodes' field")
		}

	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected snapshot message within 100 ms")
	}
}

func TestHub_SnapshotBeforeDelta(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	mock := &MockIngestionState{
		nodes: []ingestion.NodeInfo{
			{MAC: "AA:BB:CC:DD:EE:FF", FirmwareVersion: "1.0.0"},
		},
	}
	hub.SetIngestionState(mock)

	client := &Client{
		hub:  hub,
		send: make(chan []byte, 256),
	}

	hub.Register(client)
	time.Sleep(10 * time.Millisecond)

	// Drain all messages; first must be snapshot, rest must be deltas or events.
	timeout := time.After(200 * time.Millisecond)
	first := true
	for {
		select {
		case msg := <-client.send:
			var parsed map[string]json.RawMessage
			if err := json.Unmarshal(msg, &parsed); err != nil {
				continue // skip non-JSON (binary CSI)
			}
			if first {
				first = false
				typ := strings.Trim(string(parsed["type"]), `"`)
				if typ != "snapshot" {
					t.Fatalf("first message should be snapshot, got type=%s", typ)
				}
			} else {
				// Subsequent messages from tickDelta must NOT have a type field.
				// Event-driven messages (BroadcastNodeConnected etc.) do have type.
				if _, hasType := parsed["type"]; hasType {
					typ := strings.Trim(string(parsed["type"]), `"`)
					// These are acceptable event types
					switch typ {
					case "node_connected", "node_disconnected", "link_active",
						"link_inactive", "motion_state", "loc_update",
						"registry_state", "fleet_change", "system_health",
						"ble_scan", "trigger_state", "event", "alert",
						"anomaly_detected", "system_mode_change",
						"fleet_health", "fleet_history":
						// OK — event-driven broadcast
					default:
						t.Errorf("unexpected type in non-snapshot message: %s", typ)
					}
				}
				// No type field → delta update (expected from tickDelta)
			}
		case <-timeout:
			return // test passed
		}
	}
}

func TestHub_BroadcastAlert(t *testing.T) {
	tests := []struct {
		name         string
		alertID      string
		severity     string
		description  string
		acknowledged bool
	}{
		{
			name:         "critical anomaly alert",
			alertID:      "anomaly-001",
			severity:     "critical",
			description:  "Unusual activity detected in Kitchen at 3am",
			acknowledged: false,
		},
		{
			name:         "warning security mode armed",
			alertID:      "security-armed-20260407-030000",
			severity:     "warning",
			description:  "Security mode armed (auto-away)",
			acknowledged: false,
		},
		{
			name:         "acknowledged alert",
			alertID:      "anomaly-002",
			severity:     "warning",
			description:  "Environmental change detected",
			acknowledged: true,
		},
		{
			name:         "security mode disarmed",
			alertID:      "security-disarmed-20260407-080000",
			severity:     "warning",
			description:  "Security mode disarmed (BLE device detected)",
			acknowledged: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hub := NewHub()
			go hub.Run()

			client := &Client{
				hub:  hub,
				send: make(chan []byte, 10),
			}

			hub.Register(client)
			time.Sleep(10 * time.Millisecond)
			drainSnapshot(t, client.send)

			ts := time.Date(2026, 4, 7, 3, 0, 0, 0, time.UTC)
			hub.BroadcastAlert(tc.alertID, ts, tc.severity, tc.description, tc.acknowledged)

			select {
			case msg := <-client.send:
				var parsed map[string]interface{}
				if err := json.Unmarshal(msg, &parsed); err != nil {
					t.Fatalf("failed to parse alert JSON: %v", err)
				}

				if parsed["type"] != "alert" {
					t.Errorf("expected type=alert, got %v", parsed["type"])
				}

				alert, ok := parsed["alert"].(map[string]interface{})
				if !ok {
					t.Fatal("missing alert object")
				}

				if alert["id"] != tc.alertID {
					t.Errorf("expected id=%s, got %v", tc.alertID, alert["id"])
				}
				if alert["severity"] != tc.severity {
					t.Errorf("expected severity=%s, got %v", tc.severity, alert["severity"])
				}
				if alert["description"] != tc.description {
					t.Errorf("expected description=%s, got %v", tc.description, alert["description"])
				}
				if alert["acknowledged"] != tc.acknowledged {
					t.Errorf("expected acknowledged=%v, got %v", tc.acknowledged, alert["acknowledged"])
				}

				// ts should be Unix milliseconds
				tsVal, ok := alert["ts"].(float64)
				if !ok {
					t.Fatalf("expected ts to be numeric, got %T", alert["ts"])
				}
				expectedTs := float64(ts.UnixMilli())
				if tsVal != expectedTs {
					t.Errorf("expected ts=%v, got %v", expectedTs, tsVal)
				}

			case <-time.After(100 * time.Millisecond):
				t.Error("expected to receive alert broadcast")
			}
		})
	}
}

func TestHub_BroadcastEvent(t *testing.T) {
	tests := []struct {
		name        string
		eventID     string
		kind        string
		zone        string
		blobID      int
		personName  string
	}{
		{
			name:       "zone entry with person",
			eventID:    "zone_entry:1:1711234567890",
			kind:       "zone_entry",
			zone:       "Kitchen",
			blobID:     2,
			personName: "Alice",
		},
		{
			name:       "zone exit without person",
			eventID:    "zone_exit:1:1711234567891",
			kind:       "zone_exit",
			zone:       "Kitchen",
			blobID:     3,
			personName: "",
		},
		{
			name:       "portal crossing with person",
			eventID:    "portal:5:1711234567892",
			kind:       "portal_crossing",
			zone:       "Hallway",
			blobID:     1,
			personName: "Bob",
		},
		{
			name:       "presence transition",
			eventID:    "presence:AA:BB:CC:DD:EE:FF:11:22:33:44:55:66:1711234567893",
			kind:       "presence_transition",
			zone:       "AA:BB:CC:DD:EE:FF:11:22:33:44:55:66",
			blobID:     0,
			personName: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hub := NewHub()
			go hub.Run()

			client := &Client{
				hub:  hub,
				send: make(chan []byte, 10),
			}

			hub.Register(client)
			time.Sleep(10 * time.Millisecond)
			drainSnapshot(t, client.send)

			ts := time.Date(2026, 4, 7, 14, 30, 5, 0, time.UTC)
			hub.BroadcastEvent(tc.eventID, ts, tc.kind, tc.zone, tc.blobID, tc.personName)

			select {
			case msg := <-client.send:
				var parsed map[string]interface{}
				if err := json.Unmarshal(msg, &parsed); err != nil {
					t.Fatalf("failed to parse event JSON: %v", err)
				}

				if parsed["type"] != "event" {
					t.Errorf("expected type=event, got %v", parsed["type"])
				}

				evt, ok := parsed["event"].(map[string]interface{})
				if !ok {
					t.Fatal("missing event object")
				}

				if evt["id"] != tc.eventID {
					t.Errorf("expected id=%s, got %v", tc.eventID, evt["id"])
				}
				if evt["kind"] != tc.kind {
					t.Errorf("expected kind=%s, got %v", tc.kind, evt["kind"])
				}
				if evt["zone"] != tc.zone {
					t.Errorf("expected zone=%s, got %v", tc.zone, evt["zone"])
				}
				if evt["blob_id"] != float64(tc.blobID) {
					t.Errorf("expected blob_id=%d, got %v", tc.blobID, evt["blob_id"])
				}
				if tc.personName != "" && evt["person_name"] != tc.personName {
					t.Errorf("expected person_name=%s, got %v", tc.personName, evt["person_name"])
				}

				tsVal, ok := evt["ts"].(float64)
				if !ok {
					t.Fatalf("expected ts to be numeric, got %T", evt["ts"])
				}
				expectedTs := float64(ts.UnixMilli())
				if tsVal != expectedTs {
					t.Errorf("expected ts=%v, got %v", expectedTs, tsVal)
				}

			case <-time.After(100 * time.Millisecond):
				t.Error("expected to receive event broadcast")
			}
		})
	}
}

func TestHub_BroadcastBLEScan(t *testing.T) {
	tests := []struct {
		name    string
		devices []map[string]interface{}
	}{
		{
			name: "single device",
			devices: []map[string]interface{}{
				{"mac": "AA:BB:CC:DD:EE:FF", "name": "iPhone", "rssi": -62,
					"last_seen": int64(1711234567890), "label": "Alice", "blob_id": 1},
			},
		},
		{
			name: "multiple devices",
			devices: []map[string]interface{}{
				{"mac": "AA:BB:CC:DD:EE:FF", "name": "iPhone", "rssi": -62,
					"last_seen": int64(1711234567890), "label": "Alice", "blob_id": 1},
				{"mac": "11:22:33:44:55:66", "name": "Apple Watch", "rssi": -70,
					"last_seen": int64(1711234567891), "label": "", "blob_id": nil},
			},
		},
		{
			name:    "empty device list",
			devices: []map[string]interface{}{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hub := NewHub()
			go hub.Run()

			client := &Client{
				hub:  hub,
				send: make(chan []byte, 10),
			}

			hub.Register(client)
			time.Sleep(10 * time.Millisecond)
			drainSnapshot(t, client.send)

			hub.BroadcastBLEScan(tc.devices)

			select {
			case msg := <-client.send:
				var parsed map[string]interface{}
				if err := json.Unmarshal(msg, &parsed); err != nil {
					t.Fatalf("failed to parse ble_scan JSON: %v", err)
				}

				if parsed["type"] != "ble_scan" {
					t.Errorf("expected type=ble_scan, got %v", parsed["type"])
				}

				devs, ok := parsed["devices"].([]interface{})
				if !ok {
					t.Fatal("missing devices array")
				}
				if len(devs) != len(tc.devices) {
					t.Errorf("expected %d devices, got %d", len(tc.devices), len(devs))
				}

				for i, dev := range tc.devices {
					d := devs[i].(map[string]interface{})
					if d["mac"] != dev["mac"] {
						t.Errorf("device %d: expected mac=%v, got %v", i, dev["mac"], d["mac"])
					}
					if d["name"] != dev["name"] {
						t.Errorf("device %d: expected name=%v, got %v", i, dev["name"], d["name"])
					}
				}

			case <-time.After(100 * time.Millisecond):
				t.Error("expected to receive ble_scan broadcast")
			}
		})
	}
}

func TestHub_BroadcastEventFromDB(t *testing.T) {
	tests := []struct {
		name        string
		id          int64
		timestamp   int64
		eventType   string
		zone        string
		person      string
		blobID      int
		detailJSON  string
		severity    string
	}{
		{
			name:      "zone entry with person and detail",
			id:        42,
			timestamp: 1711234567890,
			eventType: "zone_entry",
			zone:      "Kitchen",
			person:    "Alice",
			blobID:    2,
			detailJSON: `{"direction":"north"}`,
			severity:  "info",
		},
		{
			name:       "zone exit without person",
			id:         43,
			timestamp:  1711234567891,
			eventType:  "zone_exit",
			zone:       "Kitchen",
			person:     "",
			blobID:     3,
			severity:   "info",
		},
		{
			name:       "portal crossing",
			id:         44,
			timestamp:  1711234567892,
			eventType:  "portal_crossing",
			zone:       "Hallway",
			person:     "Bob",
			blobID:     1,
			severity:   "info",
		},
		{
			name:       "anomaly alert",
			id:         45,
			timestamp:  1711234567893,
			eventType:  "anomaly",
			zone:       "Kitchen",
			person:     "",
			blobID:     0,
			detailJSON: `{"score":0.92}`,
			severity:   "warning",
		},
		{
			name:       "minimal event",
			id:         46,
			timestamp:  1711234567894,
			eventType:  "system",
			zone:       "",
			person:     "",
			blobID:     0,
			severity:   "info",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hub := NewHub()
			go hub.Run()

			client := &Client{
				hub:  hub,
				send: make(chan []byte, 10),
			}

			hub.Register(client)
			time.Sleep(10 * time.Millisecond)
			drainSnapshot(t, client.send)

			hub.BroadcastEventFromDB(tc.id, tc.timestamp, tc.eventType, tc.zone, tc.person, tc.blobID, tc.detailJSON, tc.severity)

			select {
			case msg := <-client.send:
				var parsed map[string]interface{}
				if err := json.Unmarshal(msg, &parsed); err != nil {
					t.Fatalf("failed to parse event JSON: %v", err)
				}

				if parsed["type"] != "event" {
					t.Errorf("expected type=event, got %v", parsed["type"])
				}

				evt, ok := parsed["event"].(map[string]interface{})
				if !ok {
					t.Fatal("missing event object")
				}

				// Verify canonical field names (matching BroadcastEvent format)
				if evt["ts"] != float64(tc.timestamp) {
					t.Errorf("expected ts=%d, got %v", tc.timestamp, evt["ts"])
				}
				if evt["kind"] != tc.eventType {
					t.Errorf("expected kind=%s, got %v", tc.eventType, evt["kind"])
				}
				if evt["zone"] != tc.zone {
					t.Errorf("expected zone=%s, got %v", tc.zone, evt["zone"])
				}
				if evt["blob_id"] != float64(tc.blobID) {
					t.Errorf("expected blob_id=%d, got %v", tc.blobID, evt["blob_id"])
				}
				if evt["person_name"] != tc.person {
					t.Errorf("expected person_name=%s, got %v", tc.person, evt["person_name"])
				}

				// Verify extra DB fields are present
				if evt["severity"] != tc.severity {
					t.Errorf("expected severity=%s, got %v", tc.severity, evt["severity"])
				}

				// detail_json should be present when non-empty
				if tc.detailJSON != "" {
					if evt["detail_json"] != tc.detailJSON {
						t.Errorf("expected detail_json=%s, got %v", tc.detailJSON, evt["detail_json"])
					}
				}

				// Verify legacy field names are NOT used
				if _, hasLegacy := evt["timestamp_ms"]; hasLegacy {
					t.Error("legacy field timestamp_ms should not be present (use ts)")
				}
				if _, hasLegacy := evt["type"]; hasLegacy {
					t.Error("legacy field type should not be present inside event (use kind)")
				}
				if _, hasLegacy := evt["person"]; hasLegacy {
					t.Error("legacy field person should not be present (use person_name)")
				}

			case <-time.After(100 * time.Millisecond):
				t.Error("expected to receive event broadcast")
			}
		})
	}
}

func TestHub_BroadcastSystemHealth(t *testing.T) {
	tests := []struct {
		name        string
		uptimeS     int64
		nodeCount   int
		beadCount   int
		goRoutines  int
		memMB       float64
	}{
		{
			name:       "fresh start",
			uptimeS:    60,
			nodeCount:  0,
			beadCount:  0,
			goRoutines: 12,
			memMB:      45.2,
		},
		{
			name:       "running system",
			uptimeS:    86400,
			nodeCount:  4,
			beadCount:  28,
			goRoutines: 87,
			memMB:      128.5,
		},
		{
			name:       "long uptime large fleet",
			uptimeS:    2592000,
			nodeCount:  16,
			beadCount:  120,
			goRoutines: 200,
			memMB:      512.0,
		},
		{
			name:       "zero values",
			uptimeS:    0,
			nodeCount:  0,
			beadCount:  0,
			goRoutines: 0,
			memMB:      0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hub := NewHub()
			go hub.Run()

			client := &Client{
				hub:  hub,
				send: make(chan []byte, 10),
			}

			hub.Register(client)
			time.Sleep(10 * time.Millisecond)
			drainSnapshot(t, client.send)

			hub.BroadcastSystemHealth(tc.uptimeS, tc.nodeCount, tc.beadCount, tc.goRoutines, tc.memMB)

			select {
			case msg := <-client.send:
				var parsed map[string]interface{}
				if err := json.Unmarshal(msg, &parsed); err != nil {
					t.Fatalf("failed to parse system_health JSON: %v", err)
				}

				if parsed["type"] != "system_health" {
					t.Errorf("expected type=system_health, got %v", parsed["type"])
				}

				health, ok := parsed["health"].(map[string]interface{})
				if !ok {
					t.Fatal("missing health object")
				}

				if health["uptime_s"] != float64(tc.uptimeS) {
					t.Errorf("expected uptime_s=%d, got %v", tc.uptimeS, health["uptime_s"])
				}
				if health["node_count"] != float64(tc.nodeCount) {
					t.Errorf("expected node_count=%d, got %v", tc.nodeCount, health["node_count"])
				}
				if health["bead_count"] != float64(tc.beadCount) {
					t.Errorf("expected bead_count=%d, got %v", tc.beadCount, health["bead_count"])
				}
				if health["go_routines"] != float64(tc.goRoutines) {
					t.Errorf("expected go_routines=%d, got %v", tc.goRoutines, health["go_routines"])
				}
				if health["mem_mb"] != tc.memMB {
					t.Errorf("expected mem_mb=%f, got %v", tc.memMB, health["mem_mb"])
				}

			case <-time.After(100 * time.Millisecond):
				t.Error("expected to receive system_health broadcast")
			}
		})
	}
}

func TestHub_DeltaOmitsTypeField(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	mock := &MockIngestionState{
		nodes: []ingestion.NodeInfo{
			{MAC: "AA:BB:CC:DD:EE:FF", FirmwareVersion: "1.0.0"},
		},
	}
	hub.SetIngestionState(mock)

	client := &Client{
		hub:  hub,
		send: make(chan []byte, 256),
	}

	hub.Register(client)
	time.Sleep(10 * time.Millisecond)

	// Read the first message (snapshot) and discard it
	select {
	case <-client.send:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected snapshot")
	}

	// Wait for a delta tick (100ms) and check it has no type field
	found := false
	for i := 0; i < 5; i++ {
		select {
		case msg := <-client.send:
			var parsed map[string]json.RawMessage
			if err := json.Unmarshal(msg, &parsed); err != nil {
				continue
			}
			if _, hasType := parsed["type"]; !hasType {
				// This is a delta message — must have timestamp_ms
				if _, ok := parsed["timestamp_ms"]; !ok {
					t.Error("delta message missing timestamp_ms")
				}
				found = true
			}
		case <-time.After(150 * time.Millisecond):
			// Try next tick
		}
	}
	if !found {
		t.Error("expected at least one delta message (no type field)")
	}
}

func TestHub_BroadcastTriggerState(t *testing.T) {
	tests := []struct {
		name       string
		triggerID  string
		triggerName string
		lastFired  time.Time
		enabled    bool
	}{
		{
			name:       "enabled trigger with last fired",
			triggerID:  "trigger-1",
			triggerName: "Couch Dwell",
			lastFired:  time.Date(2026, 4, 7, 14, 32, 5, 0, time.UTC),
			enabled:    true,
		},
		{
			name:       "disabled trigger never fired",
			triggerID:  "trigger-2",
			triggerName: "Hallway Motion",
			lastFired:  time.Time{},
			enabled:    false,
		},
		{
			name:       "enabled trigger never fired",
			triggerID:  "trigger-3",
			triggerName: "Kitchen Entry",
			lastFired:  time.Time{},
			enabled:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hub := NewHub()
			go hub.Run()

			client := &Client{
				hub:  hub,
				send: make(chan []byte, 10),
			}

			hub.Register(client)
			time.Sleep(10 * time.Millisecond)
			drainSnapshot(t, client.send)

			hub.BroadcastTriggerState(tc.triggerID, tc.triggerName, tc.lastFired, tc.enabled)

			select {
			case msg := <-client.send:
				var parsed map[string]interface{}
				if err := json.Unmarshal(msg, &parsed); err != nil {
					t.Fatalf("failed to parse trigger_state JSON: %v", err)
				}

				if parsed["type"] != "trigger_state" {
					t.Errorf("expected type=trigger_state, got %v", parsed["type"])
				}

				trigger, ok := parsed["trigger"].(map[string]interface{})
				if !ok {
					t.Fatal("missing trigger object")
				}

				if trigger["id"] != tc.triggerID {
					t.Errorf("expected id=%s, got %v", tc.triggerID, trigger["id"])
				}
				if trigger["name"] != tc.triggerName {
					t.Errorf("expected name=%s, got %v", tc.triggerName, trigger["name"])
				}
				if trigger["enabled"] != tc.enabled {
					t.Errorf("expected enabled=%v, got %v", tc.enabled, trigger["enabled"])
				}

				// Verify last_fired
				if !tc.lastFired.IsZero() {
					tsVal, ok := trigger["last_fired"].(float64)
					if !ok {
						t.Fatalf("expected last_fired to be numeric, got %T", trigger["last_fired"])
					}
					expectedTs := float64(tc.lastFired.UnixMilli())
					if tsVal != expectedTs {
						t.Errorf("expected last_fired=%v, got %v", expectedTs, tsVal)
					}
				}

			case <-time.After(100 * time.Millisecond):
				t.Error("expected to receive trigger_state broadcast")
			}
		})
	}
}

// ---- ExplainabilitySnapshot WebSocket flow tests ----

// TestHub_RequestExplain_ConsumeExplainRequests verifies that RequestExplain
// enqueues a blob ID and ConsumeExplainRequests drains the queue.
func TestHub_RequestExplain_ConsumeExplainRequests(t *testing.T) {
	hub := NewHub()

	// Initially nothing pending.
	if ids := hub.ConsumeExplainRequests(); len(ids) != 0 {
		t.Fatalf("expected empty queue, got %v", ids)
	}

	// Enqueue a request.
	hub.RequestExplain(42)
	ids := hub.ConsumeExplainRequests()
	if len(ids) != 1 || ids[0] != 42 {
		t.Fatalf("expected [42], got %v", ids)
	}

	// Queue should be empty after consuming.
	if ids2 := hub.ConsumeExplainRequests(); len(ids2) != 0 {
		t.Fatalf("expected empty queue after consume, got %v", ids2)
	}
}

// TestHub_RequestExplain_Deduplicate verifies that duplicate requests for the same
// blob ID are deduplicated (the ID appears only once per consume cycle).
func TestHub_RequestExplain_Deduplicate(t *testing.T) {
	hub := NewHub()

	hub.RequestExplain(7)
	hub.RequestExplain(7)
	hub.RequestExplain(7)

	ids := hub.ConsumeExplainRequests()
	if len(ids) != 1 {
		t.Fatalf("expected deduplication to 1 entry, got %d: %v", len(ids), ids)
	}
	if ids[0] != 7 {
		t.Fatalf("expected id=7, got %d", ids[0])
	}
}

// TestHub_BroadcastExplainSnapshot verifies that BroadcastExplainSnapshot sends a
// correctly structured "blob_explain" message to all connected clients.
func TestHub_BroadcastExplainSnapshot(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	client := &Client{
		hub:  hub,
		send: make(chan []byte, 20),
	}
	hub.Register(client)
	time.Sleep(10 * time.Millisecond)
	drainSnapshot(t, client.send) // discard the initial snapshot

	snapshot := map[string]interface{}{
		"blob_id":       1,
		"blob_position": [3]float64{3.2, 1.8, 1.0},
		"fusion_score":  0.87,
	}
	hub.BroadcastExplainSnapshot(1, snapshot)

	select {
	case msg := <-client.send:
		var parsed map[string]interface{}
		if err := json.Unmarshal(msg, &parsed); err != nil {
			t.Fatalf("failed to unmarshal blob_explain message: %v", err)
		}
		if parsed["type"] != "blob_explain" {
			t.Errorf("expected type='blob_explain', got %v", parsed["type"])
		}
		blobIDRaw, ok := parsed["blob_id"]
		if !ok {
			t.Fatal("missing blob_id field")
		}
		switch v := blobIDRaw.(type) {
		case float64:
			if int(v) != 1 {
				t.Errorf("expected blob_id=1, got %v", v)
			}
		default:
			t.Errorf("unexpected blob_id type %T: %v", blobIDRaw, blobIDRaw)
		}
		if _, ok := parsed["snapshot"]; !ok {
			t.Error("missing snapshot field in blob_explain message")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected to receive blob_explain broadcast")
	}
}

// TestServer_HandleRequestExplain verifies that a "request_explain" WebSocket
// command enqueues the blob ID in the hub for the next fusion tick.
func TestServer_HandleRequestExplain(t *testing.T) {
	hub := NewHub()
	server := NewServer(hub)

	cmd := []byte(`{"type":"request_explain","blob_id":99}`)
	server.handleCommand(cmd, nil)

	ids := hub.ConsumeExplainRequests()
	if len(ids) != 1 || ids[0] != 99 {
		t.Fatalf("expected [99] after handleRequestExplain, got %v", ids)
	}
}

// TestServer_HandleRequestExplain_MissingBlobID verifies graceful handling of a
// "request_explain" command with a missing or invalid blob_id field.
func TestServer_HandleRequestExplain_MissingBlobID(t *testing.T) {
	hub := NewHub()
	server := NewServer(hub)

	// Missing blob_id — should be silently ignored.
	server.handleCommand([]byte(`{"type":"request_explain"}`), nil)
	if ids := hub.ConsumeExplainRequests(); len(ids) != 0 {
		t.Fatalf("expected no enqueued IDs for missing blob_id, got %v", ids)
	}

	// blob_id is a string — should also be silently ignored.
	server.handleCommand([]byte(`{"type":"request_explain","blob_id":"not_a_number"}`), nil)
	if ids := hub.ConsumeExplainRequests(); len(ids) != 0 {
		t.Fatalf("expected no enqueued IDs for string blob_id, got %v", ids)
	}
}
