package ingestion

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/spaxel/mothership/internal/apdetector"
	"github.com/spaxel/mothership/internal/db"
)

// openMigratedTestDB returns a *sql.DB with the full migration chain applied,
// so the test exercises the real nodes schema (including the role='ap' CHECK
// from migration 020 and the manufacturer column from 019).
func openMigratedTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dataDir := t.TempDir()
	migrator, err := db.NewMigrator(filepath.Join(dataDir, "test.db"), db.Config{DataDir: dataDir})
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}
	migrator.Register(db.AllMigrations()...)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return migrator.DB()
}

// waitForCond polls cond until it holds or the timeout elapses. The node
// protocol is asynchronous from the client's point of view: hello handling,
// virtual-node creation and link registration all happen server-side while
// this test only knows its own writes succeeded.
func waitForCond(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func mustMACBytes(t *testing.T, mac string) [6]byte {
	t.Helper()
	hw, err := net.ParseMAC(mac)
	if err != nil {
		t.Fatalf("ParseMAC(%q): %v", mac, err)
	}
	var out [6]byte
	copy(out[:], hw)
	return out
}

// csiTestFrame builds a binary CSI frame exactly as the firmware emits it:
// 24-byte header (node, peer, timestamp, rssi, noise, channel, n_sub) followed
// by n_sub interleaved I/Q pairs.
func csiTestFrame(nodeMAC, peerMAC [6]byte, channel, nSub uint8) []byte {
	buf := make([]byte, HeaderSize+int(nSub)*2)
	copy(buf[0:6], nodeMAC[:])
	copy(buf[6:12], peerMAC[:])
	binary.LittleEndian.PutUint64(buf[12:20], 1_000_000) // timestamp_us
	buf[20] = 0xC8                                       // RSSI: int8 -56 dBm
	buf[21] = 0xA0                                       // noise floor: int8 -96 dBm
	buf[22] = channel
	buf[23] = nSub
	for i := HeaderSize; i < len(buf); i++ {
		buf[i] = byte(i)
	}
	return buf
}

// dialNode connects a websocket client to the node endpoint under test.
func dialNode(t *testing.T, nodeSrv *httptest.Server) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(nodeSrv.URL, "http") + "/ws/node"
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial %s: %v", wsURL, err)
	}
	t.Cleanup(func() { ws.Close() })
	return ws
}

func sendHello(t *testing.T, ws *websocket.Conn, mac string) {
	t.Helper()
	hello := fmt.Sprintf(
		`{"type":"hello","mac":%q,"firmware_version":"test","capabilities":["csi"],`+
			`"ap_bssid":"4c:34:88:f4:df:8a","ap_channel":6}`, mac)
	if err := ws.WriteMessage(websocket.TextMessage, []byte(hello)); err != nil {
		t.Fatalf("write hello: %v", err)
	}
}

// TestPassiveRadarHelloToLink is the mothership half of the ADR-003 done-when,
// in process: one node connects, its hello carries the overheard router BSSID,
// the apdetector creates the router virtual node (role 'ap', virtual=1), and
// the node's CSI frames — addressed to that BSSID as peer — form a link that
// /api/links serves.
func TestPassiveRadarHelloToLink(t *testing.T) {
	sqlDB := openMigratedTestDB(t)

	detector := apdetector.NewDetector(sqlDB)
	srv := NewServer()
	srv.SetAPDetector(detector)

	nodeSrv := httptest.NewServer(http.HandlerFunc(srv.HandleNodeWS))
	t.Cleanup(nodeSrv.Close)

	const (
		nodeMAC = "AA:BB:CC:DD:EE:01"
		apBSSID = "4C:34:88:F4:DF:8A" // normalized form stored by the detector
	)

	ws := dialNode(t, nodeSrv)
	sendHello(t, ws, nodeMAC)

	waitForCond(t, 2*time.Second, "router virtual node in DB", func() bool {
		var role string
		err := sqlDB.QueryRow(
			`SELECT role FROM nodes WHERE mac = ? AND virtual = 1`, apBSSID).Scan(&role)
		return err == nil && role == "ap"
	})

	var nodeType string
	var apChannel int
	var manufacturer string
	if err := sqlDB.QueryRow(
		`SELECT node_type, ap_channel, manufacturer FROM nodes WHERE mac = ?`, apBSSID,
	).Scan(&nodeType, &apChannel, &manufacturer); err != nil {
		t.Fatalf("virtual node row: %v", err)
	}
	if nodeType != "ap" {
		t.Errorf("node_type = %q, want %q", nodeType, "ap")
	}
	if apChannel != 6 {
		t.Errorf("ap_channel = %d, want 6", apChannel)
	}
	if manufacturer != "Intel Corporate" {
		t.Errorf("manufacturer = %q, want %q (OUI 4C:34:88)", manufacturer, "Intel Corporate")
	}

	// The node senses the router's beacons and reports CSI with the router's
	// BSSID as the transmitting peer — the passive-radar link.
	frame := csiTestFrame(mustMACBytes(t, nodeMAC), mustMACBytes(t, apBSSID), 6, 4)
	if err := ws.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		t.Fatalf("write CSI frame: %v", err)
	}

	wantLink := nodeMAC + ":" + apBSSID
	waitForCond(t, 2*time.Second, "passive link registration", func() bool {
		for _, id := range srv.GetAllLinks() {
			if id == wantLink {
				return true
			}
		}
		return false
	})

	// /api/links is served from GetAllLinksWithHealth; the link must be
	// visible there, not only in the internal map.
	seen := false
	for _, lh := range srv.GetAllLinksWithHealth() {
		if lh.LinkID == wantLink {
			seen = true
		}
	}
	if !seen {
		t.Errorf("GetAllLinksWithHealth() has no entry for %s", wantLink)
	}
}

// TestPassiveRadarWithoutDetectorStillFormsLinks pins the bf-41h7g failure
// mode in reverse: if the apdetector is never wired in, node connections and
// link formation must still work — the passive path degrades, ingestion does
// not.
func TestPassiveRadarWithoutDetectorStillFormsLinks(t *testing.T) {
	srv := NewServer()

	nodeSrv := httptest.NewServer(http.HandlerFunc(srv.HandleNodeWS))
	t.Cleanup(nodeSrv.Close)

	const (
		nodeMAC = "AA:BB:CC:DD:EE:02"
		apBSSID = "4C:34:88:F4:DF:8A"
	)

	ws := dialNode(t, nodeSrv)
	sendHello(t, ws, nodeMAC)

	frame := csiTestFrame(mustMACBytes(t, nodeMAC), mustMACBytes(t, apBSSID), 6, 4)
	if err := ws.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		t.Fatalf("write CSI frame: %v", err)
	}

	wantLink := nodeMAC + ":" + apBSSID
	waitForCond(t, 2*time.Second, "link without detector", func() bool {
		for _, id := range srv.GetAllLinks() {
			if id == wantLink {
				return true
			}
		}
		return false
	})
}
