package e2e

// Web Serial provisioning happy path (bead spaxel-53aeb0e6).
//
// The dashboard quickstart is: fresh install → set a PIN → configure the
// fleet WiFi in Settings → plug a node in and provision it over Web Serial →
// the node discovers the mothership over mDNS and streams CSI. No documented
// test walks that whole chain:
//
//   - mdns_discovery_test.go (spaxel-503359c5) proves the discovery surface
//     only — no PIN, no serial exchange, no CSI.
//   - provisioning_workflow_test.go (spaxel-f36ef25f) proves provision →
//     discover → dial → CSI, but its "provision" step is a bare HTTP POST:
//     the serial protocol leg never exists there.
//   - dashboard/js/onboard.test.js unit-covers the browser's serial adapter
//     against a scripted port — no mothership, no mDNS, no CSI.
//
// This file closes the gap at the protocol level. A mock serial device
// mirroring firmware/main/provision.c (SPAXEL READY banner, the
// {"provision":...} wrap, the documented error taxonomy in its documented
// precedence order) sits on one end of an in-memory pipe; the dashboard's
// serial flow runs on the other; and the whole thing is wired into a live
// mothership so the token that crosses the serial link ends in a real
// mDNS-discovered, CSI-streaming fleet connection — with no manual IP
// anywhere in the flow. The MAC enters test-side provisioning state only as
// read off the device banner, which is the browser's actual only source.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/mdns"
)

// serialWorkflowMAC is distinct from the provisioning_workflow MACs so a
// leaked node from one scenario can never satisfy another's assertions.
const serialWorkflowMAC = "AA:BB:CC:00:00:73"

// mockSerialDevice stands in for an ESP32-S3 running the provisioning
// service in firmware/main/provision.c. Its line protocol is that file's
// contract, in its precedence order:
//
//  1. a "SPAXEL READY <mac>" banner when the window opens,
//  2. any complete line once provisioned → {"ok":false,"error":"already_provisioned"},
//  3. unparseable JSON → invalid_json,
//  4. a "provision" key that is missing or not an object → missing_provision_key,
//  5. a payload without a non-empty string wifi_ssid → nvs_write_failed
//     (provision_write_nvs's ESP_ERR_INVALID_ARG, mapped by the caller),
//  6. success → {"ok":true,"mac":...} and the window closes.
type mockSerialDevice struct {
	mac         string
	conn        net.Conn
	provisioned bool
	stored      map[string]interface{} // last accepted payload, device-side
}

// startMockSerialDevice powers the mock on one end of a net.Pipe and returns
// the browser end. Closing the browser end (via cleanup) powers the device
// down: its pending read or write fails and the goroutine exits.
func startMockSerialDevice(t *testing.T, mac string) (*mockSerialDevice, net.Conn) {
	t.Helper()
	devEnd, browserEnd := net.Pipe()
	dev := &mockSerialDevice{mac: mac, conn: devEnd}
	go func() {
		defer devEnd.Close()
		dev.serve()
	}()
	t.Cleanup(func() { _ = browserEnd.Close() })
	return dev, browserEnd
}

func (d *mockSerialDevice) serve() {
	// Window-open banner, mirroring the immediate SPAXEL READY broadcast.
	// net.Pipe is synchronous, so a reader that never arrives blocks here —
	// the same backpressure a real UART line would sit in.
	if _, err := d.conn.Write([]byte("SPAXEL READY " + d.mac + "\n")); err != nil {
		return
	}

	reader := bufio.NewReader(d.conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return // browser end closed or errored — device powers down
		}
		d.handleLine(strings.TrimSuffix(line, "\n"))
	}
}

func (d *mockSerialDevice) handleLine(line string) {
	line = strings.TrimSuffix(line, "\r") // CR ignored, per the transport framing
	if strings.TrimSpace(line) == "" {
		return
	}
	if d.provisioned {
		d.respond(`{"ok":false,"error":"already_provisioned"}`)
		return
	}
	var wrapped map[string]interface{}
	if err := json.Unmarshal([]byte(line), &wrapped); err != nil {
		d.respond(`{"ok":false,"error":"invalid_json"}`)
		return
	}
	prov, ok := wrapped["provision"].(map[string]interface{})
	if !ok {
		d.respond(`{"ok":false,"error":"missing_provision_key"}`)
		return
	}
	ssid, _ := prov["wifi_ssid"].(string)
	if ssid == "" {
		d.respond(`{"ok":false,"error":"nvs_write_failed"}`)
		return
	}
	d.stored = prov
	d.provisioned = true
	d.respond(fmt.Sprintf(`{"ok":true,"mac":%q}`, d.mac))
}

func (d *mockSerialDevice) respond(s string) {
	_, _ = d.conn.Write([]byte(s + "\n"))
}

// serialBrowser is the dashboard's side of the serial link: one buffered
// reader over the port, mirroring onboard.js's accumulate-and-split reader
// (a fresh bufio per read would silently drop bytes the device sent early).
type serialBrowser struct {
	conn net.Conn
	r    *bufio.Reader
}

func newSerialBrowser(conn net.Conn) *serialBrowser {
	return &serialBrowser{conn: conn, r: bufio.NewReader(conn)}
}

// readLine returns the next non-empty line, bounded by a deadline sized for
// a machine-loaded CI host (onboard.js allows 4 s for the ack; a browser
// event loop is not the slowest reader this protocol must survive).
func (b *serialBrowser) readLine(t *testing.T, what string) string {
	t.Helper()
	if err := b.conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("set serial read deadline: %v", err)
	}
	for {
		line, err := b.r.ReadString('\n')
		if err != nil {
			t.Fatalf("read %s from the serial link: %v", what, err)
		}
		if line = strings.TrimRight(line, "\r\n"); line != "" {
			return line
		}
	}
}

// readBannerMAC mirrors onboard.js Phase 1: wait for the SPAXEL READY line
// and take the MAC as its last space-separated token. Non-banner lines are
// skipped — firmware logs to the same transport and repeats the banner.
func (b *serialBrowser) readBannerMAC(t *testing.T) string {
	t.Helper()
	for {
		line := b.readLine(t, "SPAXEL READY banner")
		if !strings.HasPrefix(line, "SPAXEL READY") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			t.Fatalf("malformed banner %q — a real browser could not extract a MAC from it", line)
		}
		return fields[len(fields)-1]
	}
}

// serialAck is the device's provisioning response line.
type serialAck struct {
	OK    bool   `json:"ok"`
	MAC   string `json:"mac"`
	Error string `json:"error"`
}

// readAck mirrors onboard.js Phase 3: collect lines until one parses as the
// response; any explicit ok:false is a hard failure.
func (b *serialBrowser) readAck(t *testing.T) serialAck {
	t.Helper()
	for {
		var ack serialAck
		line := b.readLine(t, "provisioning ack")
		if err := json.Unmarshal([]byte(line), &ack); err != nil {
			continue // non-JSON line — onboard.js ignores these too
		}
		if !ack.OK {
			t.Fatalf("device rejected the provisioning payload: %q", line)
		}
		return ack
	}
}

// putFleetNetworkSettings performs the Settings → Network step of the
// quickstart: the fleet WiFi is stored once, server-side (ADR-005), and the
// provisioning payload's wifi_ssid defaults from it.
func putFleetNetworkSettings(t *testing.T, client *http.Client, baseURL, ssid, pass string) {
	t.Helper()
	body := fmt.Sprintf(`{"wifi_ssid":%q,"wifi_password":%q}`, ssid, pass)
	req, err := http.NewRequest(http.MethodPut, baseURL+"/api/settings/network", strings.NewReader(body))
	if err != nil {
		t.Fatalf("build network settings request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("PUT /api/settings/network: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/settings/network: status %d (body: %s)", resp.StatusCode, respBody)
	}
	var got struct {
		WifiSSID   string `json:"wifi_ssid"`
		Configured bool   `json:"configured"`
	}
	if err := json.Unmarshal(respBody, &got); err != nil {
		t.Fatalf("network settings response is not JSON: %v (body: %s)", err, respBody)
	}
	if got.WifiSSID != ssid || !got.Configured {
		t.Fatalf("network settings after PUT: ssid=%q configured=%v, want %q/true", got.WifiSSID, got.Configured, ssid)
	}
}

func TestWebSerialProvisioningMDNSDiscoveryStreamsCSI(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping web-serial provisioning workflow test in short mode")
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

	// Quickstart leg 1: fresh install → set a PIN → dashboard session.
	// loginAsAdmin runs the first-run PIN setup on this fresh data dir; the
	// serial flow happens from the logged-in dashboard.
	client := loginAsAdmin(t, srv.baseURL)

	// Quickstart leg 2: Settings → Network. This leg is load-bearing for the
	// serial exchange: a real device refuses to write NVS without a
	// non-empty wifi_ssid, so the mock would nvs_write_failed without it.
	putFleetNetworkSettings(t, client, srv.baseURL, "e2e-fleet-webserial", "e2e-fleet-passphrase")

	// Quickstart leg 3: Add Node → Web Serial. Mock device on one end of the
	// pipe, the dashboard's documented serial flow on the other.
	dev, browserConn := startMockSerialDevice(t, serialWorkflowMAC)
	browser := newSerialBrowser(browserConn)

	bannerMAC := browser.readBannerMAC(t)
	if bannerMAC != serialWorkflowMAC {
		t.Fatalf("banner advertised %q, want the mock device's MAC %q", bannerMAC, serialWorkflowMAC)
	}

	// The browser's only MAC source is the banner: the provisioning request
	// is built from what came off the wire, never from test state.
	resp, err := http.Post(srv.baseURL+"/api/provision", "application/json",
		strings.NewReader(fmt.Sprintf(`{"mac":%q}`, bannerMAC)))
	if err != nil {
		t.Fatalf("POST /api/provision: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/provision: status %d (body: %s)", resp.StatusCode, raw)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("provision payload is not JSON: %v (body: %s)", err, raw)
	}
	var provisioned provisionedNode
	if err := json.Unmarshal(raw, &provisioned); err != nil {
		t.Fatalf("provision payload does not match the documented shape: %v", err)
	}

	// mDNS mode: no manual IP anywhere in the payload — discovery is the
	// node's only address source, which the second half of this test proves.
	if provisioned.MsIP != "" {
		t.Fatalf("payload carries ms_ip=%q in mDNS mode — the workflow would not be manual-IP-free", provisioned.MsIP)
	}
	if provisioned.MsMDNS != name {
		t.Errorf("payload ms_mdns=%q, want the configured instance %q", provisioned.MsMDNS, name)
	}
	if provisioned.MsPort != bindPort {
		t.Errorf("payload ms_port=%d, want the HTTP listener port %d", provisioned.MsPort, bindPort)
	}

	// Phase 2: wrap exactly as onboard.js does and write one line.
	wrapped, err := json.Marshal(map[string]interface{}{"provision": payload})
	if err != nil {
		t.Fatalf("wrap payload for serial: %v", err)
	}
	if _, err := browserConn.Write(append(wrapped, '\n')); err != nil {
		t.Fatalf("write provisioning payload over serial: %v", err)
	}

	// Phase 3: the ack must confirm THE MAC the banner advertised — a
	// device that provisions a different identity would strand the token.
	ack := browser.readAck(t)
	if ack.MAC != bannerMAC {
		t.Fatalf("ack mac=%q, want the banner-advertised %q", ack.MAC, bannerMAC)
	}

	// The device-side record of what it accepted: the fleet WiFi configured
	// in Settings must have reached the payload, and the node token must be
	// present for the post-discovery hello to authenticate.
	if got := dev.stored["wifi_ssid"]; got != "e2e-fleet-webserial" {
		t.Errorf("payload wifi_ssid=%v, want the fleet network configured in Settings", got)
	}
	if token, _ := dev.stored["node_token"].(string); token == "" {
		t.Errorf("payload carries no node_token — a provisioned node could not authenticate")
	}

	// Quickstart leg 4: the node side. Discovery presence needs a working
	// multicast path; skip honestly without one (the control service proves
	// the browse can see anything).
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
	if entry.Port != bindPort {
		t.Fatalf("discovered SRV port %d, want the HTTP listener port %d", entry.Port, bindPort)
	}
	assertTXT(t, entry, "ws=/ws/node")

	// Same A-record substitution contract as the provisioning workflow test:
	// this fixture binds loopback only, so keep the discovered port and path
	// verbatim and dial the bind host on them.
	nodeWSURL := fmt.Sprintf("ws://%s:%d/ws/node", bindHost, entry.Port)

	// Quickstart leg 5: hello → online → CSI burst → link stats, using only
	// what crossed the serial link (the token) and the wire (the port).
	runProvisionedNodeCSIWorkflow(t, client, srv.baseURL, nodeWSURL, bannerMAC, provisioned.NodeToken)
}

// TestMockSerialDeviceContract pins the wire contract this file's mock
// implements — the taxonomy firmware/main/provision.c answers with, in its
// documented precedence order. It needs no mothership and no multicast, so
// it runs wherever the package runs.
func TestMockSerialDeviceContract(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping serial device contract test in short mode")
	}

	devEnd, browserEnd := net.Pipe()
	dev := &mockSerialDevice{mac: serialWorkflowMAC, conn: devEnd}
	go func() {
		defer devEnd.Close()
		dev.serve()
	}()
	t.Cleanup(func() { _ = browserEnd.Close() })
	browser := newSerialBrowser(browserEnd)

	if got := browser.readBannerMAC(t); got != serialWorkflowMAC {
		t.Fatalf("banner mac %q, want %q", got, serialWorkflowMAC)
	}

	// Ordered: every case rides the same device, so the final
	// already_provisioned case proves the latched state a real second
	// provision attempt hits after a success.
	cases := []struct {
		name string
		line string
		want string // exact response line, minus the trailing newline
	}{
		{"not json", "{{{", `{"ok":false,"error":"invalid_json"}`},
		{"json without provision key", `{"foo":1}`, `{"ok":false,"error":"missing_provision_key"}`},
		{"provision not an object", `{"provision":"wifi"}`, `{"ok":false,"error":"missing_provision_key"}`},
		{"empty wifi_ssid", `{"provision":{"wifi_ssid":""}}`, `{"ok":false,"error":"nvs_write_failed"}`},
		{"missing wifi_ssid", `{"provision":{"node_id":"x"}}`, `{"ok":false,"error":"nvs_write_failed"}`},
		{"valid payload", `{"provision":{"wifi_ssid":"net","node_token":"tok"}}`, `{"ok":true,"mac":"` + serialWorkflowMAC + `"}`},
		{"second provision attempt", `{"provision":{"wifi_ssid":"net"}}`, `{"ok":false,"error":"already_provisioned"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := browser.conn.Write([]byte(tc.line + "\n")); err != nil {
				t.Fatalf("write: %v", err)
			}
			if line := browser.readLine(t, tc.name+" response"); line != tc.want {
				t.Fatalf("device replied %q, want %q", line, tc.want)
			}
		})
	}

	// Reads of dev state are ordered after the device's response crossed the
	// pipe, so the latched values are visible here.
	if !dev.provisioned {
		t.Errorf("device never latched provisioned state")
	}
	if got := dev.stored["wifi_ssid"]; got != "net" {
		t.Errorf("stored wifi_ssid=%v, want the accepted payload's", got)
	}
}

// retryWorkflowMAC is distinct from the other MACs in this file so a leaked
// node from one scenario can never satisfy another's assertions.
const retryWorkflowMAC = "AA:BB:CC:00:00:74"

// TestWebSerialProvisionRetryAfterFailure covers the failure half of the
// quickstart promise (bead spaxel-667b2bb2): the Add Node wizard cannot
// assume the steps were done in order. A user can plug the node in before
// Settings → Network, and the documented consequence is a refusal — with no
// fleet WiFi stored, /api/provision defaults wifi_ssid to empty (ADR-005)
// and provision.c maps an empty SSID to nvs_write_failed. The happy path
// above always configures the fleet network first, and
// TestMockSerialDeviceContract pins the refusal line itself, but no test let
// a browser recover from one against a live mothership. This is that
// recovery: surface the refusal, close the Settings gap, resend on the same
// serial session, and land the node in the identical discovered-and-
// streaming state the happy path ends in — no manual IP anywhere.
func TestWebSerialProvisionRetryAfterFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping web-serial provisioning retry test in short mode")
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

	// First-run PIN session as in the happy path — but deliberately no
	// Settings → Network yet. The out-of-order start is the failure path's
	// premise, and the mock would nvs_write_failed on any payload until it
	// is closed (ADR-005 note on the happy path's leg 2).
	client := loginAsAdmin(t, srv.baseURL)

	dev, browserConn := startMockSerialDevice(t, retryWorkflowMAC)
	browser := newSerialBrowser(browserConn)

	bannerMAC := browser.readBannerMAC(t)
	if bannerMAC != retryWorkflowMAC {
		t.Fatalf("banner advertised %q, want the mock device's MAC %q", bannerMAC, retryWorkflowMAC)
	}

	// The wizard derives a fresh payload per attempt — the retry resends a
	// newly built payload against current settings, never the refused one.
	provisionPayload := func() (map[string]interface{}, provisionedNode) {
		t.Helper()
		resp, err := http.Post(srv.baseURL+"/api/provision", "application/json",
			strings.NewReader(fmt.Sprintf(`{"mac":%q}`, bannerMAC)))
		if err != nil {
			t.Fatalf("POST /api/provision: %v", err)
		}
		defer resp.Body.Close() //nolint:errcheck
		raw, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("POST /api/provision: status %d (body: %s)", resp.StatusCode, raw)
		}
		var payloadMap map[string]interface{}
		if err := json.Unmarshal(raw, &payloadMap); err != nil {
			t.Fatalf("provision payload is not JSON: %v (body: %s)", err, raw)
		}
		var provisioned provisionedNode
		if err := json.Unmarshal(raw, &provisioned); err != nil {
			t.Fatalf("provision payload does not match the documented shape: %v", err)
		}
		return payloadMap, provisioned
	}
	sendProvisionLine := func(payloadMap map[string]interface{}) {
		t.Helper()
		wrapped, err := json.Marshal(map[string]interface{}{"provision": payloadMap})
		if err != nil {
			t.Fatalf("wrap payload for serial: %v", err)
		}
		if _, err := browserConn.Write(append(wrapped, '\n')); err != nil {
			t.Fatalf("write provisioning payload over serial: %v", err)
		}
	}

	// Attempt 1: no fleet WiFi configured, so the payload's wifi_ssid
	// defaults to empty and the device refuses it.
	payloadMap, _ := provisionPayload()
	if ssid, _ := payloadMap["wifi_ssid"].(string); ssid != "" {
		t.Fatalf("precondition: payload wifi_ssid=%q with no fleet network configured, want empty", ssid)
	}
	sendProvisionLine(payloadMap)

	// readAck hard-fails on ok:false (that is the happy path's contract), so
	// the refusal is read raw and asserted as the wizard's failure surface.
	line := browser.readLine(t, "refused provisioning ack")
	var refused serialAck
	if err := json.Unmarshal([]byte(line), &refused); err != nil {
		t.Fatalf("device answer is not a JSON ack: %v (line: %s)", err, line)
	}
	if refused.OK || refused.Error != "nvs_write_failed" {
		t.Fatalf("device answered ok=%v error=%q, want the documented refusal ok=false error=nvs_write_failed", refused.OK, refused.Error)
	}

	// A refusal must not latch the device — the whole retry path depends on
	// the same serial session still being provisionable.
	if dev.provisioned || len(dev.stored) != 0 {
		t.Fatalf("refused payload latched the device (provisioned=%v stored=%v) — a retry could never succeed", dev.provisioned, dev.stored)
	}

	// The user closes the gap the wizard surfaced: Settings → Network.
	putFleetNetworkSettings(t, client, srv.baseURL, "e2e-fleet-webserial-retry", "e2e-fleet-passphrase")

	// Attempt 2: a fresh payload on the SAME serial session — a real
	// wizard retry does not ask the user to replug the node.
	payloadMap, provisioned := provisionPayload()
	if ssid, _ := payloadMap["wifi_ssid"].(string); ssid != "e2e-fleet-webserial-retry" {
		t.Fatalf("retry payload wifi_ssid=%v, want the fleet network just configured", payloadMap["wifi_ssid"])
	}
	sendProvisionLine(payloadMap)

	ack := browser.readAck(t)
	if ack.MAC != bannerMAC {
		t.Fatalf("retry ack mac=%q, want the banner-advertised %q", ack.MAC, bannerMAC)
	}

	// Device-side record of what the retry accepted.
	if got := dev.stored["wifi_ssid"]; got != "e2e-fleet-webserial-retry" {
		t.Errorf("device stored wifi_ssid=%v, want the retried payload's", got)
	}
	if token, _ := dev.stored["node_token"].(string); token == "" {
		t.Errorf("retry payload carries no node_token — the provisioned node could not authenticate")
	}

	// The retried node proves itself exactly as the happy path's does:
	// discovered over mDNS, dialed with nothing but what crossed the wire,
	// streaming CSI on the link. Skips honestly without multicast.
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
		t.Fatalf("retried instance %q never discovered — a real node could not have proceeded past discovery", name)
	}
	if entry.Port != bindPort {
		t.Fatalf("discovered SRV port %d, want the HTTP listener port %d", entry.Port, bindPort)
	}
	assertTXT(t, entry, "ws=/ws/node")

	// Same A-record substitution contract as the happy path: this fixture
	// binds loopback only, so dial the bind host on the discovered port.
	nodeWSURL := fmt.Sprintf("ws://%s:%d/ws/node", bindHost, entry.Port)
	runProvisionedNodeCSIWorkflow(t, client, srv.baseURL, nodeWSURL, bannerMAC, provisioned.NodeToken)
}
