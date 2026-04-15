package ingestion

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestMalformedCounter_WarnThreshold verifies that WARN is logged when count exceeds 100
func TestMalformedCounter_WarnThreshold(t *testing.T) {
	server := NewServer()

	// Create a fake connection state
	mac := "AA:BB:CC:DD:EE:FF"
	server.mu.Lock()
	server.malformedCounts[mac] = &malformedCounter{
		count:     100,
		firstSeen: time.Now(),
	}
	server.mu.Unlock()

	// This should trigger WARN (count becomes 101)
	server.recordMalformed(mac)

	server.mu.RLock()
	counter := server.malformedCounts[mac]
	server.mu.RUnlock()

	if counter.count != 101 {
		t.Errorf("Expected count 101, got %d", counter.count)
	}
}

// TestMalformedCounter_CloseThreshold verifies that connection closes when count exceeds 1000
func TestMalformedCounter_CloseThreshold(t *testing.T) {
	server := NewServer()

	// Create a mock connection
	mac := "AA:BB:CC:DD:EE:FF"

	// We need to set up a minimal NodeConnection to test the close behavior
	server.mu.Lock()
	// Create a fake connection state with counter at 1000
	server.malformedCounts[mac] = &malformedCounter{
		count:     1000,
		firstSeen: time.Now(),
	}
	server.mu.Unlock()

	// This should trigger close (count becomes 1001)
	server.recordMalformed(mac)

	server.mu.RLock()
	counter := server.malformedCounts[mac]
	server.mu.RUnlock()

	if counter.count != 1001 {
		t.Errorf("Expected count 1001, got %d", counter.count)
	}
}

// TestMalformedCounter_WindowReset verifies that counter resets after window expires
func TestMalformedCounter_WindowReset(t *testing.T) {
	server := NewServer()

	mac := "AA:BB:CC:DD:EE:FF"

	// Set up counter with old timestamp
	server.mu.Lock()
	server.malformedCounts[mac] = &malformedCounter{
		count:     500,
		firstSeen: time.Now().Add(-61 * time.Second), // Outside the window
	}
	server.mu.Unlock()

	// This should reset the counter
	server.recordMalformed(mac)

	server.mu.RLock()
	counter := server.malformedCounts[mac]
	server.mu.RUnlock()

	// Counter should have been reset to 0 then incremented to 1
	if counter.count != 1 {
		t.Errorf("Expected count 1 after reset, got %d", counter.count)
	}
}

// TestMalformedCounter_MissingMAC verifies that missing MACs are handled gracefully
func TestMalformedCounter_MissingMAC(t *testing.T) {
	server := NewServer()

	// Record malformed for a MAC that doesn't exist
	// Should not panic
	server.recordMalformed("FF:FF:FF:FF:FF:FF")
}

// TestHandleBinaryFrame_ValidationErrors verifies that ParseFrame errors are recorded
func TestHandleBinaryFrame_ValidationErrors(t *testing.T) {
	server := NewServer()

	// Create a mock connection
	mac := "AA:BB:CC:DD:EE:FF"
	nc := &NodeConnection{
		MAC: mac,
	}

	server.mu.Lock()
	server.connections[mac] = nc
	server.malformedCounts[mac] = &malformedCounter{
		count:     0,
		firstSeen: time.Now(),
	}
	server.mu.Unlock()

	// Test various invalid frames
	invalidFrames := []struct {
		name string
		data []byte
	}{
		{"too short", make([]byte, 10)},
		{"payload mismatch", make([]byte, HeaderSize+10)}, // n_sub=0 but has payload
		{"invalid channel 0", []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}}, // channel at byte 22
	}

	for _, tt := range invalidFrames {
		t.Run(tt.name, func(t *testing.T) {
			server.mu.RLock()
			initialCount := server.malformedCounts[mac].count
			server.mu.RUnlock()

			server.handleBinaryFrame(nc, tt.data)

			server.mu.RLock()
			finalCount := server.malformedCounts[mac].count
			server.mu.RUnlock()

			if finalCount != initialCount+1 {
				t.Errorf("Malformed count not incremented for %s: got %d, want %d", tt.name, finalCount, initialCount+1)
			}
		})
	}
}

// TestMalformedCounter_SlidingWindow verifies the sliding window behavior
func TestMalformedCounter_SlidingWindow(t *testing.T) {
	server := NewServer()

	mac := "AA:BB:CC:DD:EE:FF"

	server.mu.Lock()
	server.malformedCounts[mac] = &malformedCounter{
		count:     99,
		firstSeen: time.Now(),
	}
	server.mu.Unlock()

	// Add one more - should NOT trigger WARN yet (only > 100)
	server.recordMalformed(mac)

	server.mu.RLock()
	count := server.malformedCounts[mac].count
	server.mu.RUnlock()

	if count != 100 {
		t.Errorf("Expected count 100, got %d", count)
	}

	// Wait a bit to avoid spam detection in the same second
	time.Sleep(10 * time.Millisecond)

	// Add one more - should trigger WARN (101 > 100)
	server.recordMalformed(mac)

	server.mu.RLock()
	count = server.malformedCounts[mac].count
	server.mu.RUnlock()

	if count != 101 {
		t.Errorf("Expected count 101, got %d", count)
	}
}

// TestMalformedCounter_ConnectionCloseIntegration verifies integration with WebSocket
func TestMalformedCounter_ConnectionCloseIntegration(t *testing.T) {
	// Create a test server with WebSocket upgrader
	ingestServer := NewServer()

	// Create a test HTTP server
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ingestServer.HandleNodeWS(w, r)
	}))
	defer httpServer.Close()

	// Convert http:// to ws://
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/node"

	// Create a WebSocket connection
	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Send a hello message first
	hello := `{"type":"hello","mac":"AA:BB:CC:DD:EE:FF","firmware_version":"1.0.0","chip":"ESP32-S3"}`
	if err := conn.WriteMessage(websocket.TextMessage, []byte(hello)); err != nil {
		t.Fatalf("Failed to send hello: %v", err)
	}

	// Drain all initial messages (role + config) sent by the server on connect
	// The server sends two messages: role assignment and config — drain them both.
	for i := 0; i < 2; i++ {
		conn.SetReadDeadline(time.Now().Add(time.Second))
		_, _, err = conn.ReadMessage()
		if err != nil {
			break // Fewer messages than expected is ok
		}
	}

	// Now send many malformed frames to trigger the close threshold
	mac := "AA:BB:CC:DD:EE:FF"
	ingestServer.mu.Lock()
	// Set counter close to threshold to speed up test
	ingestServer.malformedCounts[mac] = &malformedCounter{
		count:     1000,
		firstSeen: time.Now(),
	}
	ingestServer.mu.Unlock()

	// Send one more malformed frame
	invalidFrame := make([]byte, 10)
	if err := conn.WriteMessage(websocket.BinaryMessage, invalidFrame); err != nil {
		// Connection might already be closing
	}

	// Wait for the close to be processed
	time.Sleep(100 * time.Millisecond)

	// Try to read - should get close error
	conn.SetReadDeadline(time.Now().Add(time.Second))
	_, _, err = conn.ReadMessage()
	if err == nil {
		t.Error("Expected connection to be closed, but it's still open")
	}
}
