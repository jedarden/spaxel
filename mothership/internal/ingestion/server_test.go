package ingestion

import (
	"encoding/json"
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

// readRejectMsg reads the next WebSocket message and tries to unmarshal it as
// a RejectMessage. Returns the reject message or nil if the read fails or the
// message is not a reject.
func readRejectMsg(conn *websocket.Conn) *RejectMessage {
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		return nil
	}
	var rej RejectMessage
	if err := json.Unmarshal(data, &rej); err != nil {
		return nil
	}
	return &rej
}

func TestTokenValidation_ValidToken(t *testing.T) {
	ingestServer := NewServer()
	// Wire a simple validator: accepts mac="AA:BB:CC:DD:EE:FF" with token="good-token"
	ingestServer.SetTokenValidator(func(mac, token string) bool {
		return mac == "AA:BB:CC:DD:EE:FF" && token == "good-token"
	})

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ingestServer.HandleNodeWS(w, r)
	}))
	defer httpServer.Close()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/node"
	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	hello := `{"type":"hello","mac":"AA:BB:CC:DD:EE:FF","firmware_version":"1.0.0","chip":"ESP32-S3","token":"good-token"}`
	if err := conn.WriteMessage(websocket.TextMessage, []byte(hello)); err != nil {
		t.Fatalf("Failed to send hello: %v", err)
	}

	// Should receive role or config (not reject)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Expected role/config message, got error: %v", err)
	}
	var msg struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if msg.Type == "reject" {
		t.Fatal("Node with valid token was rejected")
	}

	// Verify node is registered
	if !ingestServer.IsNodeConnected("AA:BB:CC:DD:EE:FF") {
		t.Fatal("Node should be connected after valid token")
	}
}

func TestTokenValidation_MissingToken(t *testing.T) {
	ingestServer := NewServer()
	ingestServer.SetTokenValidator(func(mac, token string) bool {
		return token == "good-token"
	})

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ingestServer.HandleNodeWS(w, r)
	}))
	defer httpServer.Close()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/node"
	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Hello without token field
	hello := `{"type":"hello","mac":"AA:BB:CC:DD:EE:FF","firmware_version":"1.0.0","chip":"ESP32-S3"}`
	if err := conn.WriteMessage(websocket.TextMessage, []byte(hello)); err != nil {
		t.Fatalf("Failed to send hello: %v", err)
	}

	rej := readRejectMsg(conn)
	if rej == nil {
		t.Fatal("Expected reject message for missing token, got none")
	}
	if rej.Reason != "invalid_token" {
		t.Fatalf("Expected reason 'invalid_token', got %q", rej.Reason)
	}
}

func TestTokenValidation_WrongToken(t *testing.T) {
	ingestServer := NewServer()
	ingestServer.SetTokenValidator(func(mac, token string) bool {
		return token == "good-token"
	})

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ingestServer.HandleNodeWS(w, r)
	}))
	defer httpServer.Close()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/node"
	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	hello := `{"type":"hello","mac":"AA:BB:CC:DD:EE:FF","firmware_version":"1.0.0","chip":"ESP32-S3","token":"wrong-token"}`
	if err := conn.WriteMessage(websocket.TextMessage, []byte(hello)); err != nil {
		t.Fatalf("Failed to send hello: %v", err)
	}

	rej := readRejectMsg(conn)
	if rej == nil {
		t.Fatal("Expected reject message for wrong token, got none")
	}
	if rej.Reason != "invalid_token" {
		t.Fatalf("Expected reason 'invalid_token', got %q", rej.Reason)
	}
}

func TestTokenValidation_NoValidator(t *testing.T) {
	ingestServer := NewServer()
	// No validator set — all nodes should be accepted (backward compat)

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ingestServer.HandleNodeWS(w, r)
	}))
	defer httpServer.Close()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/node"
	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Hello without any token
	hello := `{"type":"hello","mac":"AA:BB:CC:DD:EE:FF","firmware_version":"1.0.0","chip":"ESP32-S3"}`
	if err := conn.WriteMessage(websocket.TextMessage, []byte(hello)); err != nil {
		t.Fatalf("Failed to send hello: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Expected role/config message, got error: %v", err)
	}
	var msg struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if msg.Type == "reject" {
		t.Fatal("Node without token should be accepted when no validator is configured")
	}
}

func TestMigrationWindow(t *testing.T) {
	tests := []struct {
		name             string
		validator        func(mac, token string) bool
		migrationDeadline time.Time
		helloJSON        string
		wantAccepted     bool
		wantUnpaired     bool
	}{
		{
			name:             "no validator accepts normally",
			validator:        nil,
			migrationDeadline: time.Time{},
			helloJSON:        `{"type":"hello","mac":"AA:BB:CC:DD:EE:FF","firmware_version":"1.0.0","chip":"ESP32-S3"}`,
			wantAccepted:     true,
			wantUnpaired:     false,
		},
		{
			name:             "valid token always accepted",
			validator:        func(mac, token string) bool { return token == "good" },
			migrationDeadline: time.Now().Add(24 * time.Hour),
			helloJSON:        `{"type":"hello","mac":"AA:BB:CC:DD:EE:FF","firmware_version":"1.0.0","chip":"ESP32-S3","token":"good"}`,
			wantAccepted:     true,
			wantUnpaired:     false,
		},
		{
			name:              "missing token accepted during migration window",
			validator:         func(mac, token string) bool { return token == "good" },
			migrationDeadline: time.Now().Add(24 * time.Hour),
			helloJSON:         `{"type":"hello","mac":"AA:BB:CC:DD:EE:FF","firmware_version":"1.0.0","chip":"ESP32-S3"}`,
			wantAccepted:      true,
			wantUnpaired:      true,
		},
		{
			name:              "wrong token accepted during migration window",
			validator:         func(mac, token string) bool { return token == "good" },
			migrationDeadline: time.Now().Add(24 * time.Hour),
			helloJSON:         `{"type":"hello","mac":"AA:BB:CC:DD:EE:FF","firmware_version":"1.0.0","chip":"ESP32-S3","token":"bad"}`,
			wantAccepted:      true,
			wantUnpaired:      true,
		},
		{
			name:              "missing token rejected after migration window",
			validator:         func(mac, token string) bool { return token == "good" },
			migrationDeadline: time.Now().Add(-1 * time.Hour), // expired
			helloJSON:         `{"type":"hello","mac":"AA:BB:CC:DD:EE:FF","firmware_version":"1.0.0","chip":"ESP32-S3"}`,
			wantAccepted:      false,
			wantUnpaired:      false,
		},
		{
			name:              "wrong token rejected after migration window",
			validator:         func(mac, token string) bool { return token == "good" },
			migrationDeadline: time.Now().Add(-1 * time.Hour),
			helloJSON:         `{"type":"hello","mac":"AA:BB:CC:DD:EE:FF","firmware_version":"1.0.0","chip":"ESP32-S3","token":"bad"}`,
			wantAccepted:      false,
			wantUnpaired:      false,
		},
		{
			name:              "zero deadline is strict mode rejects missing token",
			validator:         func(mac, token string) bool { return token == "good" },
			migrationDeadline: time.Time{}, // zero = strict
			helloJSON:         `{"type":"hello","mac":"AA:BB:CC:DD:EE:FF","firmware_version":"1.0.0","chip":"ESP32-S3"}`,
			wantAccepted:      false,
			wantUnpaired:      false,
		},
		{
			name:              "valid token accepted even after migration window",
			validator:         func(mac, token string) bool { return token == "good" },
			migrationDeadline: time.Now().Add(-1 * time.Hour), // expired
			helloJSON:         `{"type":"hello","mac":"AA:BB:CC:DD:EE:FF","firmware_version":"1.0.0","chip":"ESP32-S3","token":"good"}`,
			wantAccepted:      true,
			wantUnpaired:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ingestServer := NewServer()
			if tt.validator != nil {
				ingestServer.SetTokenValidator(tt.validator)
			}
			if !tt.migrationDeadline.IsZero() {
				ingestServer.SetMigrationDeadline(tt.migrationDeadline)
			}

			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ingestServer.HandleNodeWS(w, r)
			}))
			defer httpServer.Close()

			wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/node"
			dialer := websocket.Dialer{}
			conn, _, err := dialer.Dial(wsURL, nil)
			if err != nil {
				t.Fatalf("Failed to connect: %v", err)
			}
			defer conn.Close()

			if err := conn.WriteMessage(websocket.TextMessage, []byte(tt.helloJSON)); err != nil {
				t.Fatalf("Failed to send hello: %v", err)
			}

			conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, data, err := conn.ReadMessage()

			if tt.wantAccepted {
				if err != nil {
					t.Fatalf("Expected acceptance, got read error: %v", err)
				}
				var msg struct {
					Type string `json:"type"`
				}
				if err := json.Unmarshal(data, &msg); err != nil {
					t.Fatalf("Failed to parse response: %v", err)
				}
				if msg.Type == "reject" {
					t.Fatal("Node was rejected but expected acceptance")
				}

				if !ingestServer.IsNodeConnected("AA:BB:CC:DD:EE:FF") {
					t.Fatal("Node should be in connected list")
				}

				// Check Unpaired flag via NodeInfo
				infos := ingestServer.GetConnectedNodesInfo()
				var found *NodeInfo
				for i := range infos {
					if infos[i].MAC == "AA:BB:CC:DD:EE:FF" {
						found = &infos[i]
						break
					}
				}
				if found == nil {
					t.Fatal("Node not found in GetConnectedNodesInfo")
				}
				if found.Unpaired != tt.wantUnpaired {
					t.Errorf("Unpaired flag: got %v, want %v", found.Unpaired, tt.wantUnpaired)
				}

				// Check GetUnpairedMACs
				unpaired := ingestServer.GetUnpairedMACs()
				hasUnpaired := false
				for _, m := range unpaired {
					if m == "AA:BB:CC:DD:EE:FF" {
						hasUnpaired = true
						break
					}
				}
				if hasUnpaired != tt.wantUnpaired {
					t.Errorf("GetUnpairedMACs contains node: got %v, want %v", hasUnpaired, tt.wantUnpaired)
				}
			} else {
				if err == nil {
					var msg struct {
						Type   string `json:"type"`
						Reason string `json:"reason"`
					}
					_ = json.Unmarshal(data, &msg)
					if msg.Type != "reject" {
						t.Fatalf("Expected reject, got type=%q", msg.Type)
					}
					if msg.Reason != "invalid_token" {
						t.Fatalf("Expected reason 'invalid_token', got %q", msg.Reason)
					}
				}
				// Connection should close after reject
				if ingestServer.IsNodeConnected("AA:BB:CC:DD:EE:FF") {
					t.Fatal("Rejected node should not be in connected list")
				}
			}
		})
	}
}

func TestGetUnpairedMACs_Empty(t *testing.T) {
	server := NewServer()
	macs := server.GetUnpairedMACs()
	if len(macs) != 0 {
		t.Errorf("Expected empty slice, got %v", macs)
	}
}

func TestGetUnpairedMACs_WithUnpairedNodes(t *testing.T) {
	server := NewServer()
	server.mu.Lock()
	server.connections["AA:BB:CC:DD:EE:FF"] = &NodeConnection{
		MAC:      "AA:BB:CC:DD:EE:FF",
		Unpaired: true,
	}
	server.connections["11:22:33:44:55:66"] = &NodeConnection{
		MAC:      "11:22:33:44:55:66",
		Unpaired: false,
	}
	server.mu.Unlock()

	macs := server.GetUnpairedMACs()
	if len(macs) != 1 || macs[0] != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("Expected [AA:BB:CC:DD:EE:FF], got %v", macs)
	}
}

func TestTokenValidation_UnprovisionedNodeCannotPostCSI(t *testing.T) {
	ingestServer := NewServer()
	ingestServer.SetTokenValidator(func(mac, token string) bool {
		return token == "good-token"
	})

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ingestServer.HandleNodeWS(w, r)
	}))
	defer httpServer.Close()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws/node"
	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Send hello without token
	hello := `{"type":"hello","mac":"AA:BB:CC:DD:EE:FF","firmware_version":"1.0.0","chip":"ESP32-S3"}`
	if err := conn.WriteMessage(websocket.TextMessage, []byte(hello)); err != nil {
		t.Fatalf("Failed to send hello: %v", err)
	}

	// Expect reject
	rej := readRejectMsg(conn)
	if rej == nil || rej.Reason != "invalid_token" {
		t.Fatal("Expected reject with invalid_token")
	}

	// Try to send a CSI frame anyway — connection should be closed by server
	// Build a minimal valid CSI frame (24-byte header, no payload)
	frame := make([]byte, HeaderSize)
	// Set MAC fields
	copy(frame[0:6], []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF})
	copy(frame[6:12], []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66})
	// timestamp_us (bytes 12-19) = 0 is fine
	// rssi (byte 20)
	frame[20] = 0xE0 // -32 dBm
	// noise_floor (byte 21)
	frame[21] = 0xA1 // -95 dBm
	// channel (byte 22)
	frame[22] = 6
	// n_sub (byte 23) = 0, no payload

	if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		// Connection already closed — this is the expected case
		return
	}

	// Node should NOT be in connected list
	if ingestServer.IsNodeConnected("AA:BB:CC:DD:EE:FF") {
		t.Fatal("Unprovisioned node should not be in connected list")
	}

	// No links should exist for this node
	for _, link := range ingestServer.GetAllLinks() {
		if strings.HasPrefix(link, "AA:BB:CC:DD:EE:FF:") {
			t.Fatalf("Unprovisioned node should not have links, found: %s", link)
		}
	}
}
