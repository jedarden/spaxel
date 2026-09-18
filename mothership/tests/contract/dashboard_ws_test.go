package contract

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/spaxel/mothership/internal/tracking"
)

// wsFrame is the generic {"type":...} envelope every dashboard frame shares.
type wsFrame struct {
	Type        string `json:"type"`
	TimestampMs int64  `json:"timestamp_ms"`
}

// dialDashboard connects to the rig's /ws/dashboard endpoint. The hub's Run
// loop must be running (registration, snapshot delivery and broadcast fan-out
// all flow through its channels).
func dialDashboard(t *testing.T, rg *rig) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(rg.srv.URL, "http") + "/ws/dashboard"
	dialer := &websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	ws, resp, err := dialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial %s: %v (http %v)", url, err, resp)
	}
	t.Cleanup(func() { ws.Close() })
	_ = ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	return ws
}

// readFrame reads one text frame and decodes the envelope.
func readFrame(t *testing.T, ws *websocket.Conn) (wsFrame, []byte) {
	t.Helper()
	mt, raw, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	if mt != websocket.TextMessage {
		t.Fatalf("frame type = %d, want text (dashboard frames are JSON)", mt)
	}
	var f wsFrame
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("decode frame %q: %v", raw, err)
	}
	return f, raw
}

// TestDashboardWSSnapshotIsFirstFrame pins the lifecycle order: the very first
// frame after upgrade is the snapshot — deltas may follow it but can never
// precede it. (Note the hub caches the latest loc_update blobs, so a broadcast
// fired *before* a client connects is folded into that client's snapshot
// rather than replayed as a delta.)
func TestDashboardWSSnapshotIsFirstFrame(t *testing.T) {
	rg := newRig(t, false)
	go rg.hub.Run()

	ws := dialDashboard(t, rg)
	f, raw := readFrame(t, ws)
	if f.Type != "snapshot" {
		t.Fatalf("first frame = %q (%s), want snapshot", f.Type, raw)
	}
	if f.TimestampMs == 0 {
		t.Error("snapshot timestamp_ms = 0, want server time in epoch ms")
	}
	// Fresh hub, no broadcasts yet: no subsystems are wired, so all optional
	// state keys are absent rather than null.
	for _, absent := range []string{"nodes", "links", "blobs", "zones", "portals"} {
		if strings.Contains(string(raw), `"`+absent+`"`) {
			t.Errorf("snapshot contains %q key with no subsystem wired — optional state must be absent", absent)
		}
	}

	// A broadcast after connect arrives as the next frame, after the snapshot.
	rg.hub.BroadcastLocUpdate([]tracking.Blob{{ID: 1, X: 1, Z: 1, Weight: 1}})
	if f, _ = readFrame(t, ws); f.Type != "loc_update" {
		t.Fatalf("second frame = %q, want the loc_update delta", f.Type)
	}
}

// TestDashboardWSLocUpdateShape pins the lowercase blobJSON contract of
// loc_update (contrast with the capitalized REST /api/blobs keys).
func TestDashboardWSLocUpdateShape(t *testing.T) {
	rg := newRig(t, false)
	go rg.hub.Run()
	ws := dialDashboard(t, rg)

	if f, _ := readFrame(t, ws); f.Type != "snapshot" {
		t.Fatalf("first frame = %q, want snapshot", f.Type)
	}

	rg.hub.BroadcastLocUpdate([]tracking.Blob{{
		ID: 7, X: 1.5, Z: -2.25, VX: 0.25, VZ: -0.5, Weight: 0.9,
		Trail: [][2]float64{{1.5, -2.25}},
	}})

	f, raw := readFrame(t, ws)
	if f.Type != "loc_update" {
		t.Fatalf("frame = %q, want loc_update", f.Type)
	}
	var msg struct {
		Blobs []struct {
			ID     int          `json:"id"`
			X      float64      `json:"x"`
			Z      float64      `json:"z"`
			VX     float64      `json:"vx"`
			VZ     float64      `json:"vz"`
			Weight float64      `json:"weight"`
			Trail  [][2]float64 `json:"trail"`
		} `json:"blobs"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("decode loc_update %q: %v", raw, err)
	}
	if len(msg.Blobs) != 1 {
		t.Fatalf("blobs = %d, want 1", len(msg.Blobs))
	}
	b := msg.Blobs[0]
	if b.ID != 7 || b.X != 1.5 || b.Z != -2.25 || b.Weight != 0.9 {
		t.Errorf("blob = %+v, want id 7 x 1.5 z -2.25 weight 0.9", b)
	}
	if len(b.Trail) != 1 || b.Trail[0][0] != 1.5 {
		t.Errorf("trail = %v, want one point at x=1.5", b.Trail)
	}
}

// TestDashboardWSCommands pins the client→server command surface.
func TestDashboardWSCommands(t *testing.T) {
	rg := newRig(t, false)
	go rg.hub.Run()
	ws := dialDashboard(t, rg)

	if f, _ := readFrame(t, ws); f.Type != "snapshot" {
		t.Fatalf("first frame = %q, want snapshot", f.Type)
	}

	// request_explain must reach the hub's pending-queue for the fusion loop.
	if err := ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"request_explain","blob_id":42}`)); err != nil {
		t.Fatalf("send request_explain: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		ids := rg.hub.ConsumeExplainRequests()
		if len(ids) == 1 && ids[0] == 42 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("request_explain never reached the hub (pending=%v)", ids)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// An unknown command is ignored silently — the connection stays usable
	// and no error frame is sent back.
	if err := ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"not_a_real_command"}`)); err != nil {
		t.Fatalf("send unknown command: %v", err)
	}
	// Prove liveness with a second recognized command.
	if err := ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"request_explain","blob_id":43}`)); err != nil {
		t.Fatalf("connection broke after unknown command: %v", err)
	}
	deadline = time.Now().Add(2 * time.Second)
	found := false
	for !found {
		ids := rg.hub.ConsumeExplainRequests()
		for _, id := range ids {
			if id == 43 {
				found = true
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("second request_explain never reached the hub — connection did not survive the unknown frame")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestDashboardWSOverMaxClients pins maxClients=0 semantics: the hub accepts
// unlimited concurrent dashboard clients, each with its own snapshot.
func TestDashboardWSOverMaxClients(t *testing.T) {
	// maxClients=0 means unlimited; a dial must always succeed.
	rg := newRig(t, false)
	go rg.hub.Run()
	ws1 := dialDashboard(t, rg)
	ws2 := dialDashboard(t, rg)
	if f, _ := readFrame(t, ws1); f.Type != "snapshot" {
		t.Errorf("client 1 first frame = %q, want snapshot", f.Type)
	}
	if f, _ := readFrame(t, ws2); f.Type != "snapshot" {
		t.Errorf("client 2 first frame = %q, want snapshot", f.Type)
	}
}
