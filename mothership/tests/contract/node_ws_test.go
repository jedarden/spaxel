package contract

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/spaxel/mothership/internal/ingestion"
)

// The /ws/node tests pin the node-protocol half of the API contract
// (docs/notes/api-contract.md §8): hello auth, the post-hello role/config
// push, the binary CSI frame layout and its validation rules, the JSON
// control-message surface, and shutdown behaviour. spaxel-sim
// (cmd/sim/connectNode) is the reference client for this exact surface.

var (
	testMACBytes = [6]byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}
	// Second node identity used as a frame peer / unknown hello MAC.
	testPeerMAC     = "11:22:33:44:55:66"
	testPeerMACByte = [6]byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66}
)

// dialNode connects to the rig's /ws/node endpoint, optionally carrying
// handshake headers (X-Spaxel-Token is the documented node auth channel).
func dialNode(t *testing.T, rg *rig, hdr http.Header) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(rg.srv.URL, "http") + "/ws/node"
	dialer := &websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	ws, resp, err := dialer.Dial(url, hdr)
	if err != nil {
		t.Fatalf("dial %s: %v (http %v)", url, err, resp)
	}
	t.Cleanup(func() { ws.Close() })
	_ = ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	return ws
}

// sendHello sends a hello with the documented fields. mac is the announced
// node MAC; token is the hello-body token ("" omits it — header-token
// clients like spaxel-sim never put the token in the body).
func sendHello(t *testing.T, ws *websocket.Conn, mac, token string) {
	t.Helper()
	hello := map[string]any{
		"type":             "hello",
		"mac":              mac,
		"firmware_version": "1.2.3",
		"capabilities":     []string{"csi", "tx", "rx"},
		"chip":             "ESP32-S3",
	}
	if token != "" {
		hello["token"] = token
	}
	raw, err := json.Marshal(hello)
	if err != nil {
		t.Fatalf("marshal hello: %v", err)
	}
	if err := ws.WriteMessage(websocket.TextMessage, raw); err != nil {
		t.Fatalf("send hello: %v", err)
	}
}

// readNodeFrame reads one text frame from the node connection and returns
// the decoded {"type":...} envelope plus the raw bytes.
func readNodeFrame(t *testing.T, ws *websocket.Conn) (wsFrame, []byte) {
	t.Helper()
	mt, raw, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("read node frame: %v", err)
	}
	if mt != websocket.TextMessage {
		t.Fatalf("node frame type = %d, want text (node control frames are JSON)", mt)
	}
	var f wsFrame
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("decode node frame %q: %v", raw, err)
	}
	return f, raw
}

// expectAccepted reads the post-hello downstream push a bare server sends
// when no fleet manager is wired — role assignment then config — and pins
// both shapes (role=rx, rate_hz=RateIdle, variance_threshold=Default).
func expectAccepted(t *testing.T, ws *websocket.Conn) {
	t.Helper()
	f, raw := readNodeFrame(t, ws)
	if f.Type != "role" {
		t.Fatalf("first post-hello frame = %s, want type role", raw)
	}
	var role ingestion.RoleMessage
	if err := json.Unmarshal(raw, &role); err != nil {
		t.Fatalf("decode role frame %s: %v", raw, err)
	}
	if role.Role != "rx" {
		t.Fatalf("bare-server role = %q, want rx", role.Role)
	}

	f, raw = readNodeFrame(t, ws)
	if f.Type != "config" {
		t.Fatalf("second post-hello frame = %s, want type config", raw)
	}
	var cfg ingestion.ConfigMessage
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("decode config frame %s: %v", raw, err)
	}
	if cfg.RateHz == nil || *cfg.RateHz != ingestion.RateIdle {
		t.Fatalf("config rate_hz = %v, want %d", cfg.RateHz, ingestion.RateIdle)
	}
	if cfg.VarianceThreshold == nil || *cfg.VarianceThreshold != ingestion.DefaultVarianceThreshold {
		t.Fatalf("config variance_threshold = %v, want %v", cfg.VarianceThreshold, ingestion.DefaultVarianceThreshold)
	}
	if cfg.NTPServer != nil {
		t.Fatalf("config ntp_server = %q, want absent (rig sets no NTP server)", *cfg.NTPServer)
	}
}

// expectReject reads one frame and pins the reject envelope.
func expectReject(t *testing.T, ws *websocket.Conn, wantReason string) {
	t.Helper()
	f, raw := readNodeFrame(t, ws)
	if f.Type != "reject" {
		t.Fatalf("frame = %s, want type reject", raw)
	}
	var rej ingestion.RejectMessage
	if err := json.Unmarshal(raw, &rej); err != nil {
		t.Fatalf("decode reject frame %s: %v", raw, err)
	}
	if rej.Reason != wantReason {
		t.Fatalf("reject reason = %q, want %q", rej.Reason, wantReason)
	}
}

// waitFor polls cond until it holds or the timeout elapses (node message
// handling happens on the server's connection goroutine).
func waitFor(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(msg)
}

// csiFrame builds a binary CSI frame with the documented §8 header layout
// (24-byte header + nSub interleaved int8 I/Q pairs) — the same encoder
// shape as the firmware and cmd/sim/generator.go.
func csiFrame(nodeMAC, peerMAC [6]byte, nSub uint8, channel byte, tsUS uint64) []byte {
	rssi := int8(-50)       // dBm
	noiseFloor := int8(-95) // dBm
	frame := make([]byte, ingestion.HeaderSize+int(nSub)*2)
	copy(frame[0:6], nodeMAC[:])                      // node_mac
	copy(frame[6:12], peerMAC[:])                     // peer_mac
	binary.LittleEndian.PutUint64(frame[12:20], tsUS) // timestamp_us
	frame[20] = byte(rssi)
	frame[21] = byte(noiseFloor)
	frame[22] = channel // channel
	frame[23] = nSub    // n_sub
	// Payload stays zero-valued I/Q: parsing validates framing, not samples.
	return frame
}

// TestNodeWSHelloAuth pins the hello authentication matrix: token via body
// or X-Spaxel-Token header (body wins when both are present), unknown MACs
// sharing the invalid_token reason, and the migration-window grace that
// accepts a tokenless node but flags it unpaired.
func TestNodeWSHelloAuth(t *testing.T) {
	cases := []struct {
		name    string
		mac     string
		bodyTok string
		hdrTok  string
		grace   bool   // migration window open?
		want    string // "" = accepted, else the expected reject reason
	}{
		{"body_token_valid", testMAC, testNodeToken, "", false, ""},
		{"header_token_valid", testMAC, "", testNodeToken, false, ""},
		{"body_token_wins_over_header", testMAC, "wrong-token", testNodeToken, false, "invalid_token"},
		{"invalid_body_token", testMAC, "wrong-token", "", false, "invalid_token"},
		{"unknown_mac_same_reason", testPeerMAC, testNodeToken, "", false, "invalid_token"},
		{"missing_token_strict", testMAC, "", "", false, "invalid_token"},
		{"missing_token_grace_window", testMAC, "", "", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rg := newRig(t, false)
			if tc.grace {
				rg.ingestion.SetMigrationDeadline(time.Now().Add(time.Minute))
			}
			hdr := http.Header{}
			if tc.hdrTok != "" {
				hdr.Set("X-Spaxel-Token", tc.hdrTok)
			}
			ws := dialNode(t, rg, hdr)
			sendHello(t, ws, tc.mac, tc.bodyTok)

			if tc.want != "" {
				expectReject(t, ws, tc.want)
				return
			}

			expectAccepted(t, ws)
			waitFor(t, 2*time.Second, func() bool {
				return len(rg.ingestion.GetConnectedNodesInfo()) == 1
			}, "accepted node never registered")

			info := rg.ingestion.GetConnectedNodesInfo()[0]
			if info.Unpaired != tc.grace {
				t.Fatalf("unpaired = %v, want %v", info.Unpaired, tc.grace)
			}
			if info.FirmwareVersion != "1.2.3" || info.Chip != "ESP32-S3" {
				t.Fatalf("hello fields not surfaced: %+v", info)
			}
			if tc.grace && len(rg.ingestion.GetUnpairedMACs()) != 1 {
				t.Fatalf("grace-window node not listed unpaired: %v", rg.ingestion.GetUnpairedMACs())
			}
		})
	}
}

// TestNodeWSHelloMustBeFirst pins the first-message contract: the first
// frame after upgrade must be a JSON hello — anything else earns a reject
// frame and a closed connection.
func TestNodeWSHelloMustBeFirst(t *testing.T) {
	cases := []struct {
		name string
		send func(t *testing.T, ws *websocket.Conn)
		want string
	}{
		{"garbage_text", func(t *testing.T, ws *websocket.Conn) {
			if err := ws.WriteMessage(websocket.TextMessage, []byte("not json")); err != nil {
				t.Fatalf("send garbage: %v", err)
			}
		}, "invalid hello format"},
		{"non_hello_json", func(t *testing.T, ws *websocket.Conn) {
			raw, _ := json.Marshal(map[string]any{"type": "health", "mac": testMAC})
			if err := ws.WriteMessage(websocket.TextMessage, raw); err != nil {
				t.Fatalf("send health-first: %v", err)
			}
		}, "expected hello first"},
		{"binary_first", func(t *testing.T, ws *websocket.Conn) {
			if err := ws.WriteMessage(websocket.BinaryMessage, csiFrame(testMACBytes, testPeerMACByte, 64, 6, 1)); err != nil {
				t.Fatalf("send binary-first: %v", err)
			}
		}, "invalid hello format"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rg := newRig(t, false)
			ws := dialNode(t, rg, nil)
			tc.send(t, ws)
			expectReject(t, ws, tc.want)
		})
	}
}

// TestNodeWSBinaryCSIFrames pins the binary CSI ingress contract: a
// well-formed frame creates its node:peer link; malformed frames are
// dropped silently (no error frame, connection survives, no link).
func TestNodeWSBinaryCSIFrames(t *testing.T) {
	rg := newRig(t, false)
	ws := dialNode(t, rg, http.Header{"X-Spaxel-Token": []string{testNodeToken}})
	sendHello(t, ws, testMAC, "")
	expectAccepted(t, ws)

	link1 := testMAC + ":" + testPeerMAC
	if err := ws.WriteMessage(websocket.BinaryMessage, csiFrame(testMACBytes, testPeerMACByte, 64, 6, 1000)); err != nil {
		t.Fatalf("send csi frame: %v", err)
	}
	waitFor(t, 2*time.Second, func() bool {
		for _, l := range rg.ingestion.GetAllLinks() {
			if l == link1 {
				return true
			}
		}
		return false
	}, "accepted frame never created link "+link1)

	// Malformed variants, one per documented validation rule (rule order in
	// ingestion.ParseFrame: length → payload match → n_sub cap → channel).
	badLen := csiFrame(testMACBytes, testPeerMACByte, 64, 6, 1001)
	malformed := []struct {
		name string
		data []byte
	}{
		{"too_short", make([]byte, 10)},
		{"payload_length_mismatch", badLen[:len(badLen)-1]},                       // n_sub says 64, one I/Q pair missing
		{"n_sub_over_128", csiFrame(testMACBytes, testPeerMACByte, 200, 6, 1002)}, // length matches declared 200
		{"channel_zero", csiFrame(testMACBytes, testPeerMACByte, 64, 0, 1003)},
		{"channel_over_14", csiFrame(testMACBytes, testPeerMACByte, 64, 15, 1004)},
	}
	for _, tc := range malformed {
		if err := ws.WriteMessage(websocket.BinaryMessage, tc.data); err != nil {
			t.Fatalf("send %s frame: %v", tc.name, err)
		}
	}

	// The connection survives the malformed burst: a follow-up valid frame on
	// a second peer is processed normally.
	link2 := testMAC + ":66:55:44:33:22:11"
	frame2 := csiFrame(testMACBytes, [6]byte{0x66, 0x55, 0x44, 0x33, 0x22, 0x11}, 64, 6, 1005)
	if err := ws.WriteMessage(websocket.BinaryMessage, frame2); err != nil {
		t.Fatalf("send follow-up frame: %v", err)
	}
	waitFor(t, 2*time.Second, func() bool {
		for _, l := range rg.ingestion.GetAllLinks() {
			if l == link2 {
				return true
			}
		}
		return false
	}, "connection did not survive malformed frames")

	// And no link materialised for any malformed frame.
	if got := rg.ingestion.GetAllLinks(); len(got) != 2 {
		t.Fatalf("links after burst = %v, want exactly [%s %s]", got, link1, link2)
	}

	// Nothing is ever written back for a malformed frame (no error frame,
	// no close) — a short-deadline read must time out.
	_ = ws.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, _, err := ws.ReadMessage(); err == nil {
		t.Fatal("server wrote an unexpected frame in response to malformed input")
	}
	_ = ws.SetReadDeadline(time.Now().Add(5 * time.Second))
}

// TestNodeWSJSONControlMessages pins the post-hello JSON surface: unknown
// upstream types are ignored silently (the protocol's forward-compat
// mechanism) and a documented health message lands in the connection state.
func TestNodeWSJSONControlMessages(t *testing.T) {
	rg := newRig(t, false)
	ws := dialNode(t, rg, http.Header{"X-Spaxel-Token": []string{testNodeToken}})
	sendHello(t, ws, testMAC, "")
	expectAccepted(t, ws)

	// Unknown type: silently ignored, connection stays up.
	if err := ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"hologram","future_field":42}`)); err != nil {
		t.Fatalf("send unknown type: %v", err)
	}

	health := map[string]any{
		"type":            "health",
		"mac":             testMAC,
		"timestamp_ms":    1234,
		"free_heap_bytes": 123456,
		"wifi_rssi_dbm":   -45,
		"uptime_ms":       5678,
		"csi_rate_hz":     2,
		"wifi_channel":    6,
		"ntp_synced":      true,
	}
	raw, err := json.Marshal(health)
	if err != nil {
		t.Fatalf("marshal health: %v", err)
	}
	if err := ws.WriteMessage(websocket.TextMessage, raw); err != nil {
		t.Fatalf("send health: %v", err)
	}

	waitFor(t, 2*time.Second, func() bool {
		for _, n := range rg.ingestion.GetConnectedNodesInfo() {
			if n.MAC == testMAC && n.FreeHeapBytes == 123456 {
				return true
			}
		}
		return false
	}, "health message never landed in connection state")
}

// TestNodeWSShutdownRejectsNewUpgrades pins the shutdown behaviour: once
// SetShuttingDown has run, new upgrade requests get HTTP 503 with a JSON
// body instead of a WebSocket upgrade.
func TestNodeWSShutdownRejectsNewUpgrades(t *testing.T) {
	rg := newRig(t, false)
	rg.ingestion.SetShuttingDown()

	url := "ws" + strings.TrimPrefix(rg.srv.URL, "http") + "/ws/node"
	dialer := &websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	ws, resp, err := dialer.Dial(url, nil)
	if err == nil {
		ws.Close()
		t.Fatal("upgrade succeeded during shutdown, want 503")
	}
	if resp == nil || resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("shutdown status = %v, want 503", resp)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read 503 body: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode 503 body %q: %v", body, err)
	}
	if got["error"] != "mothership shutting down" || got["code"] != "shutting_down" {
		t.Fatalf(`503 body = %s, want {"error":"mothership shutting down","code":"shutting_down"}`, body)
	}
}
