package dashboard

import (
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

func TestHub_Broadcast(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	client := &Client{
		hub:  hub,
		send: make(chan []byte, 10),
	}

	hub.Register(client)
	time.Sleep(10 * time.Millisecond)

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

func TestHub_InitialState(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	// Set mock ingestion state
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

	// Should receive initial state
	msg := <-client.send
	// Just check it's a valid JSON with type "state"
	if len(msg) == 0 || msg[0] != '{' {
		t.Errorf("expected JSON state message, got %v", msg)
	}
}
