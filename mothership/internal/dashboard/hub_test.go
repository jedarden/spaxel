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
	hub.BroadcastNodeConnected("AA:BB:CC:DD:EE:FF", "1.0.0", "ESP32-S3")

	msg := <-client.send
	expected := `{"chip":"ESP32-S3","firmware_version":"1.0.0","mac":"AA:BB:CC:DD:EE:FF","type":"node_connected"}`
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
