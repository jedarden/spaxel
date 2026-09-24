package e2e

// E2E regression for bead spaxel-6253ee99: real HT20 firmware reports 52
// I/Q pairs per CSI frame (firmware csi.c: n_sub = info->len/2) while the
// mothership pipeline was configured for the 64-subcarrier HT20 map. The
// NBVI selection handed indices up to 62 to PhaseVariance, which indexed the
// 52-long per-frame arrays and panicked inside handleBinaryFrame — killing
// the node's WebSocket on the very first real frame. These tests stream
// genuine 52-carrier frames at a node and assert the connection survives.

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestShortHT20FrameKeepsConnectionOpen(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping short-frame ingestion e2e in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), TestTimeout)
	defer cancel()

	h := NewTestHarness(t)
	if err := h.Start(ctx); err != nil {
		t.Fatalf("start mothership: %v", err)
	}
	defer h.Stop()

	const mac = "AA:BB:CC:DD:00:52"
	token, err := provisionNodeToken(ctx, h.APIURL, mac)
	if err != nil {
		t.Fatalf("provision node token: %v", err)
	}

	header := http.Header{}
	header.Set("X-Spaxel-Token", token)
	conn, _, err := websocket.DefaultDialer.Dial(h.MothershipURL, header)
	if err != nil {
		t.Fatalf("dial node websocket: %v", err)
	}
	defer conn.Close() //nolint:errcheck

	hello := map[string]interface{}{
		"type":             "hello",
		"mac":              mac,
		"node_id":          "e2e-short-frame-" + mac,
		"firmware_version": "0.1.0-e2e",
		"capabilities":     []string{"csi", "tx", "rx"},
		"chip":             "ESP32-S3",
		"flash_mb":         16,
		"uptime_ms":        1000,
	}
	if err := conn.WriteJSON(hello); err != nil {
		t.Fatalf("send hello: %v", err)
	}

	waitUntil(t, 15*time.Second, "node "+mac+" to come online", func() bool {
		nodes, err := h.GetNodes(ctx)
		if err != nil {
			return false
		}
		for _, node := range nodes {
			if node.MAC == mac && node.Status == "online" {
				return true
			}
		}
		return false
	})

	// Stream 100 genuine HT20-shaped frames (generateCSIFrame emits 52
	// subcarriers) at 20Hz. That crosses NBVIMinSamples (50) and the first
	// NBVI recalculation (80) — the frames that used to panic the pipeline.
	// Any server-side kill surfaces here as a write error or a close frame
	// on the read side.
	const frames = 100
	for i := 0; i < frames; i++ {
		frame := generateCSIFrame(mac, uint64(i))
		if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
			t.Fatalf("frame %d: write failed (connection was closed by the server): %v", i, err)
		}

		conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if isTimeoutErr(err) {
				// No server traffic this tick — fine.
			} else {
				t.Fatalf("frame %d: read failed (connection closed by server): %v", i, err)
			}
		} else if len(msg) > 0 && msg[0] == '{' {
			// Server message (e.g. role assignment) — discard.
		}

		time.Sleep(time.Second / 20)
	}

	// The node must still be online after the whole burst: a panicked
	// handler tears down the connection and the hub marks the node offline.
	nodes, err := h.GetNodes(ctx)
	if err != nil {
		t.Fatalf("get nodes after burst: %v", err)
	}
	online := false
	for _, node := range nodes {
		if node.MAC == mac {
			online = node.Status == "online"
			break
		}
	}
	if !online {
		t.Fatalf("node %s offline after streaming %d short frames — connection did not survive", mac, frames)
	}

	// And the connection must still accept traffic.
	if err := conn.WriteMessage(websocket.BinaryMessage, generateCSIFrame(mac, frames)); err != nil {
		t.Fatalf("post-burst frame write failed: %v", err)
	}
}
