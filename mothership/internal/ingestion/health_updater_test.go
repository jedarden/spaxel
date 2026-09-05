package ingestion

import (
	"errors"
	"sync"
	"testing"
)

// The write half of the free_heap_bytes chain, pinned by bead
// spaxel-3b6699a4. A health message must reach the registered
// NodeHealthUpdater, not just the connection's in-memory LastHealth — the
// DB-backed REST surfaces (GET /api/nodes, /api/nodes/{mac}, /api/fleet,
// /api/fleet/health) read fleet.db and have no access to connection state.
// The read half of the chain is pinned in internal/fleet/heap_fields_test.go.

// recordingHealthUpdater captures every UpdateNodeHealth call.
type recordingHealthUpdater struct {
	mu    sync.Mutex
	calls []healthCall
}

type healthCall struct {
	mac           string
	freeHeapBytes int64
}

func (r *recordingHealthUpdater) UpdateNodeHealth(mac string, freeHeapBytes int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, healthCall{mac: mac, freeHeapBytes: freeHeapBytes})
	return nil
}

func (r *recordingHealthUpdater) recorded() []healthCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]healthCall(nil), r.calls...)
}

// failingHealthUpdater stands in for a registry write that errors.
type failingHealthUpdater struct{}

func (failingHealthUpdater) UpdateNodeHealth(mac string, freeHeapBytes int64) error {
	return errors.New("no such column: uptime_ms")
}

// TestHealthMessagePersistsFreeHeapBytes verifies that a node health message
// is forwarded to the NodeHealthUpdater with the connection's MAC and the
// message's reading, and that a later reading replaces an earlier one.
func TestHealthMessagePersistsFreeHeapBytes(t *testing.T) {
	const mac = "AA:BB:CC:DD:EE:07"

	s := NewServer()
	updater := &recordingHealthUpdater{}
	s.SetNodeHealthUpdater(updater)

	nc := &NodeConnection{MAC: mac}

	s.handleJSONMessage(nc, []byte(`{"type":"health","mac":"`+mac+`","timestamp_ms":1700000000000,`+
		`"free_heap_bytes":187456,"wifi_rssi_dbm":-52,"uptime_ms":3600000,`+
		`"csi_rate_hz":100,"wifi_channel":6,"ntp_synced":true}`))

	// The in-memory path is unchanged: the dashboard WebSocket still reads it.
	if nc.LastHealth == nil || nc.LastHealth.FreeHeapBytes != 187456 {
		t.Fatalf("LastHealth = %+v, want FreeHeapBytes 187456", nc.LastHealth)
	}

	calls := updater.recorded()
	if len(calls) != 1 {
		t.Fatalf("UpdateNodeHealth called %d times, want 1: %+v", len(calls), calls)
	}
	if calls[0].mac != mac || calls[0].freeHeapBytes != 187456 {
		t.Errorf("UpdateNodeHealth called with (%q, %d), want (%q, %d)",
			calls[0].mac, calls[0].freeHeapBytes, mac, 187456)
	}

	// A later tick replaces the earlier reading.
	s.handleJSONMessage(nc, []byte(`{"type":"health","mac":"`+mac+`","free_heap_bytes":165000}`))
	calls = updater.recorded()
	if len(calls) != 2 {
		t.Fatalf("UpdateNodeHealth called %d times after second message, want 2", len(calls))
	}
	if calls[1].freeHeapBytes != 165000 {
		t.Errorf("second call free_heap_bytes = %d, want 165000 (latest reading wins)", calls[1].freeHeapBytes)
	}
}

// TestHealthMessageWithoutUpdaterIsDropped verifies the hook stays optional:
// a Server that never had SetNodeHealthUpdater called (the state every
// existing deployment ran in) must accept health messages without panicking.
func TestHealthMessageWithoutUpdaterIsDropped(t *testing.T) {
	s := NewServer()
	nc := &NodeConnection{MAC: "AA:BB:CC:DD:EE:08"}

	s.handleJSONMessage(nc, []byte(`{"type":"health","mac":"AA:BB:CC:DD:EE:08","free_heap_bytes":1000}`))

	if nc.LastHealth == nil || nc.LastHealth.FreeHeapBytes != 1000 {
		t.Fatalf("LastHealth = %+v, want FreeHeapBytes 1000", nc.LastHealth)
	}
}

// TestHealthMessageUpdaterErrorIsContained verifies a failing persistence
// write does not panic the message loop or evict the connection's reading —
// a registry hiccup must cost one log line, not the node's health state.
func TestHealthMessageUpdaterErrorIsContained(t *testing.T) {
	const mac = "AA:BB:CC:DD:EE:09"

	s := NewServer()
	s.SetNodeHealthUpdater(failingHealthUpdater{})
	nc := &NodeConnection{MAC: mac}

	s.handleJSONMessage(nc, []byte(`{"type":"health","mac":"`+mac+`","free_heap_bytes":187456}`))

	if nc.LastHealth == nil || nc.LastHealth.FreeHeapBytes != 187456 {
		t.Fatalf("LastHealth = %+v, want FreeHeapBytes 187456 despite the write error", nc.LastHealth)
	}
}

// TestNonHealthMessageSkipsHealthUpdater verifies only health messages reach
// the updater — a ble or motion_hint payload must not write a heap reading.
func TestNonHealthMessageSkipsHealthUpdater(t *testing.T) {
	s := NewServer()
	updater := &recordingHealthUpdater{}
	s.SetNodeHealthUpdater(updater)

	nc := &NodeConnection{MAC: "AA:BB:CC:DD:EE:12"}
	s.handleJSONMessage(nc, []byte(`{"type":"ble","mac":"AA:BB:CC:DD:EE:12","devices":[]}`))
	s.handleJSONMessage(nc, []byte(`{"type":"motion_hint","mac":"AA:BB:CC:DD:EE:12"}`))
	s.handleJSONMessage(nc, []byte(`{"type":"nonsense`))

	if calls := updater.recorded(); len(calls) != 0 {
		t.Errorf("UpdateNodeHealth called for non-health messages: %+v", calls)
	}
}
