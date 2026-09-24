package e2e

// Closed-loop provisioning workflow coverage (bead spaxel-f36ef25f).
//
// mdns_discovery_test.go proves the discovery SURFACE: the advertisement,
// the browse result, the payload fields, the doctor check. Neither documented
// workflow is taken end to end, though — no node there dials an address it
// acquired through mDNS discovery or through the payload's manual ms_ip, and
// no CSI flows over the provisioned connection. These tests close that loop
// for both documented modes:
//
//	mDNS enabled:      provision (payload carries NO ms_ip) → browse for the
//	                   advertised instance → dial the discovered SRV port on
//	                   the TXT ws path → hello → online → stream CSI →
//	                   frames counted on the node's link
//	mDNS disabled:     provision with ms_ip → dial ws://<ms_ip>:<ms_port>
//	                   from the payload alone → hello → online → stream CSI →
//	                   frames counted on the node's link
//
// "Without manual recovery" is the absence of any operator step between
// provisioning and a streaming fleet link: no re-provision, no restart, no
// hand-edited address. The node comes online on the first dial and stays
// online through the burst.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/mdns"
)

// workflowFrameCount is the CSI burst each workflow streams at 20 Hz — 1.5 s
// of firmware-shaped traffic, far below any load-shedding threshold, so every
// frame must be accounted for in the link stats.
const workflowFrameCount = 30

// workflowCSIMACs are distinct per scenario so a leaked node from one
// instance can never satisfy the other's link-stats assertion.
const (
	workflowMACMDNS = "AA:BB:CC:00:00:71"
	workflowMACIP   = "AA:BB:CC:00:00:72"
)

// linkFrameCount sums /api/framestats/all frame_count over every link keyed
// by the node's MAC — the server-side record that parsed CSI frames from this
// node's radio reached the ingestion pipeline. Request failures fail outright
// rather than reading as zero: auth or availability problems are not the
// "no frames yet" condition the caller is polling for.
func linkFrameCount(t *testing.T, client *http.Client, baseURL, mac string) int {
	t.Helper()

	resp, err := client.Get(baseURL + "/api/framestats/all")
	if err != nil {
		t.Fatalf("GET /api/framestats/all: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/framestats/all: got status %d, want 200 (body: %s)", resp.StatusCode, body)
	}
	var stats map[string]struct {
		FrameCount int `json:"frame_count"`
	}
	if err := json.Unmarshal(body, &stats); err != nil {
		t.Fatalf("GET /api/framestats/all: body is not JSON: %v (body: %s)", err, body)
	}

	total := 0
	for linkID, stat := range stats {
		if strings.Contains(strings.ToLower(linkID), strings.ToLower(mac)) {
			total += stat.FrameCount
		}
	}
	return total
}

// runProvisionedNodeCSIWorkflow takes a freshly provisioned node's token and
// the node-side WebSocket URL it would have derived from its provisioning
// facts, then drives the post-provisioning half of the workflow: hello →
// online → CSI burst → link stats → still online. The caller decides where
// nodeWSURL comes from; that choice is the workflow under test.
func runProvisionedNodeCSIWorkflow(t *testing.T, client *http.Client, baseURL, nodeWSURL, mac, token string) {
	t.Helper()

	header := http.Header{}
	header.Set("X-Spaxel-Token", token)
	conn, _, err := websocket.DefaultDialer.Dial(nodeWSURL, header)
	if err != nil {
		t.Fatalf("dial the provisioned node URL %s: %v", nodeWSURL, err)
	}
	defer func() { _ = conn.Close() }()

	hello := map[string]interface{}{
		"type":             "hello",
		"mac":              mac,
		"node_id":          "e2e-provision-workflow-" + mac,
		"firmware_version": "0.1.0-e2e",
		"capabilities":     []string{"csi", "tx", "rx"},
		"chip":             "ESP32-S3",
		"flash_mb":         16,
		"uptime_ms":        1000,
	}
	if err := conn.WriteJSON(hello); err != nil {
		t.Fatalf("send hello: %v", err)
	}

	waitUntil(t, 15*time.Second, "node "+mac+" to come online after provisioning",
		func() bool { return fleetNodeOnline(t, client, baseURL, mac) })

	// Stream firmware-shaped frames at 20 Hz. A server-side kill (auth
	// rejection, panic in the ingestion path) surfaces here as a write error.
	for i := 0; i < workflowFrameCount; i++ {
		if err := conn.WriteMessage(websocket.BinaryMessage, generateCSIFrame(mac, uint64(i))); err != nil {
			t.Fatalf("frame %d: write failed (connection closed by the server): %v", i, err)
		}

		conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if !isTimeoutErr(err) {
				t.Fatalf("frame %d: read failed (connection closed by server): %v", i, err)
			}
		} else if len(msg) > 0 && msg[0] == '{' {
			// Server message (e.g. role assignment) — discard.
		}

		time.Sleep(time.Second / 20)
	}

	// The pipeline must have ingested every frame on this node's link: the
	// provisioning workflow is only proven when CSI is actually flowing.
	waitUntil(t, 15*time.Second, fmt.Sprintf("%d ingested frames on %s's link", workflowFrameCount, mac),
		func() bool { return linkFrameCount(t, client, baseURL, mac) >= workflowFrameCount })

	// And the link must still be up with no operator action: online node,
	// connection still accepting traffic.
	if !fleetNodeOnline(t, client, baseURL, mac) {
		t.Errorf("node %s offline after streaming — the workflow needed manual recovery", mac)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, generateCSIFrame(mac, workflowFrameCount)); err != nil {
		t.Errorf("post-burst frame write failed: %v", err)
	}
}

func TestProvisioningWorkflowMDNSDiscoveryStreamsCSI(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping mDNS provisioning workflow test in short mode")
	}

	bin := buildDemoBinary(t)
	name := mdnsTestName()
	srv := startMDNSServer(t, bin, t.TempDir(),
		"SPAXEL_MDNS_ENABLED=true",
		"SPAXEL_MDNS_NAME="+name,
	)
	defer func() { _ = srv.stop() }()

	bindURL, err := url.Parse(srv.baseURL)
	if err != nil {
		t.Fatalf("parse baseURL: %v", err)
	}
	bindPort, err := strconv.Atoi(bindURL.Port())
	if err != nil {
		t.Fatalf("parse bind port: %v", err)
	}
	bindHost := bindURL.Hostname()

	// The mDNS-mode payload must leave ms_ip empty: the node's ONLY address
	// source is discovery, which is exactly what this test exercises.
	provisioned := postProvision(t, srv.baseURL, fmt.Sprintf(`{"mac":%q}`, workflowMACMDNS))
	if provisioned.MsIP != "" {
		t.Fatalf("payload carries ms_ip=%q in mDNS mode — the node would skip discovery and this scenario proves nothing", provisioned.MsIP)
	}
	if provisioned.MsMDNS != name {
		t.Errorf("payload ms_mdns=%q, want the configured instance %q", provisioned.MsMDNS, name)
	}

	// Discovery presence needs a working multicast path; skip honestly
	// without one (the control service proves the browse can see anything).
	_, stopControl := requireMulticastEnvironment(t)
	defer stopControl()

	var entry *mdns.ServiceEntry
	for attempt := 0; attempt < 3 && entry == nil; attempt++ {
		entry = mdnsBrowse(t, 3*time.Second)[mdnsAdvertisedFQDN(name)]
		if entry == nil {
			time.Sleep(500 * time.Millisecond)
		}
	}
	if entry == nil {
		t.Fatalf("provisioned instance %q never discovered — a real node could not have proceeded past discovery", name)
	}

	// Discovery's genuine contribution to the node's dial: the SRV port and
	// the TXT ws path. Both are asserted before use — the test dials what a
	// node would have derived from the wire, not from the test's knowledge.
	if entry.Port != bindPort {
		t.Fatalf("discovered SRV port %d, want the HTTP listener port %d", entry.Port, bindPort)
	}
	assertTXT(t, entry, "ws=/ws/node")

	// The advertised A record names the host the mothership resolved for its
	// non-loopback interface; this fixture binds loopback only, so that host
	// may not be dialable from here. A production mothership binds all
	// interfaces, where the advertised host IS a listener address. Keep the
	// discovered port and path verbatim and substitute the bind host, logging
	// the substitution.
	dialHost := bindHost
	if entry.AddrV4 != nil {
		if got := entry.AddrV4.String(); got == dialHost {
			t.Logf("discovered A record %s matches the bind host", got)
		} else {
			t.Logf("discovered A record %s is not reachable from this loopback-bound fixture; dialing bind host %s on the discovered port %d", got, dialHost, entry.Port)
		}
	}
	nodeWSURL := fmt.Sprintf("ws://%s:%d/ws/node", dialHost, entry.Port)

	client := loginAsAdmin(t, srv.baseURL)
	runProvisionedNodeCSIWorkflow(t, client, srv.baseURL, nodeWSURL, workflowMACMDNS, provisioned.NodeToken)
}

func TestProvisioningWorkflowManualIPStreamsCSI(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping manual-IP provisioning workflow test in short mode")
	}

	// SPAXEL_MDNS_ENABLED=false: the documented mDNS-less workflow. Entirely
	// hermetic — no multicast, no browse, just the payload override.
	srv := startMDNSServer(t, buildDemoBinary(t), t.TempDir(), "SPAXEL_MDNS_ENABLED=false")
	defer func() { _ = srv.stop() }()

	bindURL, err := url.Parse(srv.baseURL)
	if err != nil {
		t.Fatalf("parse baseURL: %v", err)
	}
	bindPort, err := strconv.Atoi(bindURL.Port())
	if err != nil {
		t.Fatalf("parse bind port: %v", err)
	}

	provisioned := postProvision(t, srv.baseURL, fmt.Sprintf(`{"mac":%q,"ms_ip":"127.0.0.1"}`, workflowMACIP))
	if provisioned.MsIP != "127.0.0.1" {
		t.Fatalf("payload ms_ip=%q, want the requested manual override 127.0.0.1", provisioned.MsIP)
	}
	if provisioned.MsPort != bindPort {
		t.Errorf("payload ms_port=%d, want the HTTP listener port %d", provisioned.MsPort, bindPort)
	}

	// The node-side URL built ONLY from payload fields — the firmware's
	// ms_ip/ms_port NVS path, with nothing else in the test's knowledge
	// leaking into the dial.
	nodeWSURL := fmt.Sprintf("ws://%s:%d/ws/node", provisioned.MsIP, provisioned.MsPort)

	client := loginAsAdmin(t, srv.baseURL)
	runProvisionedNodeCSIWorkflow(t, client, srv.baseURL, nodeWSURL, workflowMACIP, provisioned.NodeToken)
}
