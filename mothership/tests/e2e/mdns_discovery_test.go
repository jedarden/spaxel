package e2e

// Integration coverage for mDNS discovery and the manual-IP provisioning
// fallback.
//
// The mothership is driven the way production drives it — a real subprocess of
// the compiled binary (same pattern as demo_mode_test.go), with the
// SPAXEL_MDNS_* / SPAXEL_ADVERTISED_BASE_URL environment switches as the only
// difference between scenarios. Discovery is observed with a genuine
// hashicorp/mdns query against the running process, so the advertised
// instance name, SRV port and TXT records are asserted the way a node on the
// LAN would see them, not by introspecting the library.
//
// Multicast is a property of the host, not of the code. Every positive or
// negative discovery claim is therefore anchored by a control service that
// the test process advertises itself: if the control instance is not visible
// to the same browse that (misses) the mothership, the host has no working
// multicast path and the discovery assertion skips rather than issuing a
// verdict it cannot back. The HTTP-, log- and payload-level assertions still
// run first in every scenario, so a multicast-less CI box verifies everything
// that does not need the wire.
//
// Scenarios:
//
//	TestMDNSDiscoveryEnabled          — instance discoverable, SRV port equals
//	                                    the HTTP listener, TXT carries the WS
//	                                    paths, provisioning payload agrees,
//	                                    /api/doctor reports the binding ok.
//	TestMDNSDiscoveryDisabled         — SPAXEL_MDNS_ENABLED=false puts nothing
//	                                    on the wire; doctor stays ok; the
//	                                    provisioning payload carries ms_mdns
//	                                    and honors the manual ms_ip override.
//	TestAdvertisedBaseURLDiagnostics  — a non-routable advertised URL degrades
//	                                    to a WARN with fallback advertisement;
//	                                    a wildcard URL refuses to start.
//	TestNodeReconnectAfterDisconnect  — a provisioned node's WS drop marks it
//	                                    offline and the same credentials
//	                                    reconnect it without re-provisioning.
//	TestComposeHostNetworkingContract — docker-compose.yml keeps the plan's
//	                                    mDNS-vs-docker-bridge invariant.
//
// (bead spaxel-503359c5)

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/mdns"
)

// mdnsTestPIN satisfies the 4-8 digit PIN validation. Test fixture, not a
// credential: the mothership under test is a subprocess bound to loopback on
// a throwaway data directory.
const mdnsTestPIN = "864209"

// mdnsTestName returns an mDNS instance name unique to this test process so
// parallel runs (or a leftover mothership from another checkout) can never
// answer for each other on the shared multicast segment.
func mdnsTestName() string {
	return fmt.Sprintf("spaxel-e2e-mdns-%d", os.Getpid())
}

// mdnsAdvertisedFQDN is the service name a browse returns for an instance:
// "<instance>._spaxel._tcp.local.".
func mdnsAdvertisedFQDN(instance string) string {
	return instance + "._spaxel._tcp.local."
}

// mdnsServerEnv builds the subprocess environment: the same strict base the
// rest of the e2e harness uses, plus caller-supplied mDNS settings. Nothing
// sets SPAXEL_MDNS_ENABLED implicitly — every test states the mode it is
// proving.
func mdnsServerEnv(bindAddr, dataDir string, extra ...string) []string {
	env := append(os.Environ(),
		"SPAXEL_BIND_ADDR="+bindAddr,
		"SPAXEL_DATA_DIR="+dataDir,
		"SPAXEL_LOG_LEVEL=info",
		"TZ=UTC",
		// Window-independent auth policy (same rationale as the harness Start()).
		"SPAXEL_MIGRATION_WINDOW_HOURS=0",
		// The docker build embeds the dashboard behind the `embed` tag; a plain
		// `go build` does not, so point the static dir at the repo dashboard/
		// (same rationale as startDemoServer).
		"SPAXEL_STATIC_DIR="+filepath.Join(moduleRoot(), "..", "dashboard"),
	)
	return append(env, extra...)
}

// startMDNSServer launches a mothership subprocess on an ephemeral loopback
// port and waits for /healthz. It fails the test if the server never becomes
// healthy; the fatal-startup scenario uses assertFatalStartup instead.
func startMDNSServer(t *testing.T, bin, dataDir string, extraEnv ...string) *demoServer {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve an ephemeral port: %v", err)
	}
	bindAddr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatalf("failed to release reserved port %s: %v", bindAddr, err)
	}

	stderr := &bytes.Buffer{}
	cmd := &exec.Cmd{
		Path:   bin,
		Env:    mdnsServerEnv(bindAddr, dataDir, extraEnv...),
		Dir:    filepath.Join(moduleRoot(), ".."),
		Stdout: io.Discard,
		Stderr: stderr,
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start mothership: %v", err)
	}

	srv := &demoServer{cmd: cmd, stderr: stderr, baseURL: "http://" + bindAddr}
	if err := srv.waitHealthy(t); err != nil {
		t.Logf("mothership stderr:\n%s", stderr.String())
		srv.stop()
		t.Fatalf("mothership never became healthy: %v", err)
	}
	return srv
}

// assertFatalStartup starts mothership with env and asserts it exits non-zero
// after logging wantMessage — the contract for a config error that must stop
// startup (ADR-004: an explicitly-set-but-invalid advertised URL is fatal).
func assertFatalStartup(t *testing.T, bin string, env []string, wantMessage string) {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve an ephemeral port: %v", err)
	}
	bindAddr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatalf("failed to release reserved port %s: %v", bindAddr, err)
	}
	fullEnv := mdnsServerEnv(bindAddr, t.TempDir())
	fullEnv = append(fullEnv, env...)

	stderr := &bytes.Buffer{}
	cmd := &exec.Cmd{
		Path:   bin,
		Env:    fullEnv,
		Dir:    filepath.Join(moduleRoot(), ".."),
		Stdout: io.Discard,
		Stderr: stderr,
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start mothership: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case waitErr := <-done:
		if waitErr == nil {
			t.Fatalf("mothership exited 0; wanted a fatal startup error containing %q\nstderr:\n%s", wantMessage, stderr.String())
		}
		if !strings.Contains(stderr.String(), wantMessage) {
			t.Fatalf("mothership exited with %v but stderr lacks %q\nstderr:\n%s", waitErr, wantMessage, stderr.String())
		}
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		t.Fatalf("mothership kept running on an invalid configuration; wanted a fatal error containing %q\nstderr:\n%s", wantMessage, stderr.String())
	}
}

// mdnsBrowse collects every _spaxel._tcp instance visible on the local
// segment within timeout, keyed by service FQDN. A browse error yields an
// empty result — the caller's control-service probe is what decides whether
// that means "absent" or "unknowable".
func mdnsBrowse(t *testing.T, timeout time.Duration) map[string]*mdns.ServiceEntry {
	t.Helper()

	entries := make(chan *mdns.ServiceEntry, 32) // buffered per QueryParam docs
	browseCtx, cancel := context.WithTimeout(context.Background(), timeout+2*time.Second)
	defer cancel()

	go func() {
		defer close(entries)
		err := mdns.Query(&mdns.QueryParam{
			Service: "_spaxel._tcp",
			Domain:  "local.",
			Timeout: timeout,
			Entries: entries,
		})
		if err != nil {
			t.Logf("mdns browse: %v", err)
		}
	}()

	found := make(map[string]*mdns.ServiceEntry)
	for {
		select {
		case entry, ok := <-entries:
			if !ok {
				return found
			}
			found[entry.Name] = entry
		case <-browseCtx.Done():
			return found
		}
	}
}

// requireMulticastEnvironment advertises a control _spaxel._tcp instance from
// the test process and re-browses until it sees it. It returns the control's
// stop function. If the control never becomes visible, the host has no
// working multicast path — the caller's discovery assertion could not be
// judged honestly, so the test skips (with the control shut down first).
func requireMulticastEnvironment(t *testing.T) (controlName string, stopControl func()) {
	t.Helper()

	controlName = fmt.Sprintf("mdns-e2e-control-%d", os.Getpid())
	fqdn := mdnsAdvertisedFQDN(controlName)

	service, err := mdns.NewMDNSService(controlName, "_spaxel._tcp", "local.", "", 1, nil, nil)
	if err != nil {
		t.Fatalf("create control mDNS service: %v", err)
	}
	server, err := mdns.NewServer(&mdns.Config{Zone: service})
	if err != nil {
		t.Fatalf("start control mDNS server: %v", err)
	}
	stopControl = func() { server.Shutdown() } //nolint:errcheck

	for attempt := 0; attempt < 3; attempt++ {
		if _, ok := mdnsBrowse(t, 2*time.Second)[fqdn]; ok {
			return controlName, stopControl
		}
		time.Sleep(500 * time.Millisecond)
	}

	stopControl()
	t.Skipf("host multicast unavailable — control service %s never visible to browse; discovery presence cannot be judged here", fqdn)
	return "", nil // unreachable
}

// provisionedNode is the slice of the /api/provision payload that decides how
// a node reaches the mothership.
type provisionedNode struct {
	NodeToken string `json:"node_token"`
	MsMDNS    string `json:"ms_mdns"`
	MsIP      string `json:"ms_ip"`
	MsPort    int    `json:"ms_port"`
}

// postProvision POSTs body to /api/provision and decodes the reachability fields.
func postProvision(t *testing.T, baseURL, body string) provisionedNode {
	t.Helper()

	status, _, payload := demoDo(t, http.MethodPost, baseURL+"/api/provision", body)
	if status != http.StatusOK {
		t.Fatalf("POST /api/provision: got status %d, want 200 (body: %s)", status, payload)
	}
	var provisioned provisionedNode
	if err := json.Unmarshal([]byte(payload), &provisioned); err != nil {
		t.Fatalf("POST /api/provision: body is not JSON: %v (body: %s)", err, payload)
	}
	return provisioned
}

// loginAsAdmin runs the fresh-install PIN setup and login against baseURL and
// returns a client carrying the session cookie.
func loginAsAdmin(t *testing.T, baseURL string) *http.Client {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("create cookie jar: %v", err)
	}
	client := &http.Client{Jar: jar, Timeout: 10 * time.Second}

	for _, step := range []struct {
		path string
		body string
	}{
		{"/api/auth/setup", `{"pin":"` + mdnsTestPIN + `"}`},
		{"/api/auth/login", `{"pin":"` + mdnsTestPIN + `"}`},
	} {
		req, err := http.NewRequest(http.MethodPost, baseURL+step.path, strings.NewReader(step.body))
		if err != nil {
			t.Fatalf("build POST %s: %v", step.path, err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("POST %s: %v", step.path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close() //nolint:errcheck
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("POST %s: got status %d, want 200 (body: %s)", step.path, resp.StatusCode, body)
		}
	}
	return client
}

// doctorCheck is one entry of the /api/doctor checks array.
type doctorCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// fetchDoctorMDNSCheck authenticates and returns the mdns_binding check from
// GET /api/doctor.
func fetchDoctorMDNSCheck(t *testing.T, client *http.Client, baseURL string) doctorCheck {
	t.Helper()

	resp, err := client.Get(baseURL + "/api/doctor")
	if err != nil {
		t.Fatalf("GET /api/doctor: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/doctor: got status %d, want 200 (body: %s)", resp.StatusCode, body)
	}

	var doctor struct {
		Checks []doctorCheck `json:"checks"`
	}
	if err := json.Unmarshal(body, &doctor); err != nil {
		t.Fatalf("GET /api/doctor: body is not JSON: %v (body: %s)", err, body)
	}
	for _, check := range doctor.Checks {
		if check.Name == "mdns_binding" {
			return check
		}
	}
	t.Fatalf("GET /api/doctor: no mdns_binding check in %+v", doctor.Checks)
	return doctorCheck{}
}

// waitUntil polls done until it returns true or the timeout elapses.
func waitUntil(t *testing.T, timeout time.Duration, what string, done func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if done() {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for %s", timeout, what)
}

// fleetNodeOnline reports whether /api/fleet/health lists mac as online — the
// hub's connection-authoritative view, not a last-seen approximation.
func fleetNodeOnline(t *testing.T, client *http.Client, baseURL, mac string) bool {
	t.Helper()

	resp, err := client.Get(baseURL + "/api/fleet/health")
	if err != nil {
		t.Fatalf("GET /api/fleet/health: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/fleet/health: got status %d, want 200 (body: %s)", resp.StatusCode, body)
	}
	var health struct {
		Nodes []struct {
			MAC    string `json:"mac"`
			Online bool   `json:"online"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(body, &health); err != nil {
		t.Fatalf("GET /api/fleet/health: body is not JSON: %v (body: %s)", err, body)
	}
	for _, node := range health.Nodes {
		if strings.EqualFold(node.MAC, mac) {
			return node.Online
		}
	}
	return false
}

// assertTXT asserts the discovery TXT record carries every field a node uses
// to locate the WS endpoints.
func assertTXT(t *testing.T, entry *mdns.ServiceEntry, wantFields ...string) {
	t.Helper()

	for _, want := range wantFields {
		found := false
		for _, field := range entry.InfoFields {
			if field == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("discovered TXT record lacks %q (InfoFields: %v, Info: %q)", want, entry.InfoFields, entry.Info)
		}
	}
}

func TestMDNSDiscoveryEnabled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping mDNS discovery integration test in short mode")
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

	// The startup log names the instance and the port it advertises. This is
	// the first actionable diagnostic an operator sees; it must carry the real
	// listener port, not a hardcoded default.
	t.Run("startup log names the advertised instance and listener port", func(t *testing.T) {
		wantLog := fmt.Sprintf("mDNS advertising %s._spaxel._tcp.local:%d", name, bindPort)
		if !strings.Contains(srv.stderr.String(), wantLog) {
			t.Errorf("stderr lacks %q; stderr:\n%s", wantLog, srv.stderr.String())
		}
	})

	// The provisioning payload must agree with what mDNS advertises: a node
	// provisions from the same instance/port it can discover.
	t.Run("provisioning payload matches the advertisement", func(t *testing.T) {
		provisioned := postProvision(t, srv.baseURL, fmt.Sprintf(`{"mac":%q}`, "AA:BB:CC:00:00:01"))
		if provisioned.MsMDNS != name {
			t.Errorf("payload ms_mdns=%q, want the configured instance %q", provisioned.MsMDNS, name)
		}
		if provisioned.MsPort != bindPort {
			t.Errorf("payload ms_port=%d, want the HTTP listener port %d", provisioned.MsPort, bindPort)
		}
		if provisioned.MsIP != "" {
			t.Errorf("payload carries ms_ip=%q in mDNS mode; the manual override belongs to mDNS-less networks only", provisioned.MsIP)
		}
	})

	// Doctor is the operator's "why can't my nodes find me" surface.
	t.Run("doctor reports the mDNS binding ok", func(t *testing.T) {
		client := loginAsAdmin(t, srv.baseURL)
		check := fetchDoctorMDNSCheck(t, client, srv.baseURL)
		if check.Status != "ok" {
			t.Errorf("doctor mdns_binding status=%q message=%q, want ok", check.Status, check.Message)
		}
	})

	// Live discovery last: it is the only assertion that needs multicast, so a
	// host without it skips here instead of failing the whole scenario.
	t.Run("instance is discoverable with the listener port and WS paths", func(t *testing.T) {
		_, stopControl := requireMulticastEnvironment(t)
		defer stopControl()

		entries := mdnsBrowse(t, 3*time.Second)
		entry, ok := entries[mdnsAdvertisedFQDN(name)]
		if !ok {
			names := make([]string, 0, len(entries))
			for discovered := range entries {
				names = append(names, discovered)
			}
			t.Fatalf("mothership instance %q not discovered; browse saw %v", name, names)
		}
		if entry.Port != bindPort {
			t.Errorf("advertised SRV port %d, want the HTTP listener port %d", entry.Port, bindPort)
		}
		assertTXT(t, entry, "version=1", "ws=/ws/node", "dashboard=/ws/dashboard")
	})
}

func TestMDNSDiscoveryDisabled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping mDNS discovery integration test in short mode")
	}

	bin := buildDemoBinary(t)
	name := mdnsTestName()
	srv := startMDNSServer(t, bin, t.TempDir(),
		"SPAXEL_MDNS_ENABLED=false",
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

	t.Run("nothing is advertised on the wire", func(t *testing.T) {
		if strings.Contains(srv.stderr.String(), "mDNS advertising") {
			t.Errorf("stderr contains an mDNS advertising line with SPAXEL_MDNS_ENABLED=false; stderr:\n%s", srv.stderr.String())
		}
	})

	t.Run("doctor still reports ok while disabled", func(t *testing.T) {
		client := loginAsAdmin(t, srv.baseURL)
		check := fetchDoctorMDNSCheck(t, client, srv.baseURL)
		if check.Status != "ok" {
			t.Errorf("doctor mdns_binding status=%q message=%q with mDNS disabled, want ok", check.Status, check.Message)
		}
	})

	t.Run("provisioning payload carries ms_mdns and honors the manual ms_ip", func(t *testing.T) {
		// ms_mdns stays in the payload even with discovery off — it costs
		// nothing and a node that later lands on an mDNS-capable segment can
		// still resolve it. The mDNS-less path is the explicit ms_ip override.
		provisioned := postProvision(t, srv.baseURL, fmt.Sprintf(`{"mac":%q,"ms_ip":"192.168.1.50"}`, "AA:BB:CC:00:00:02"))
		if provisioned.MsMDNS != name {
			t.Errorf("payload ms_mdns=%q, want %q even with mDNS disabled", provisioned.MsMDNS, name)
		}
		if provisioned.MsPort != bindPort {
			t.Errorf("payload ms_port=%d, want the HTTP listener port %d", provisioned.MsPort, bindPort)
		}
		if provisioned.MsIP != "192.168.1.50" {
			t.Errorf("payload ms_ip=%q, want the requested manual override 192.168.1.50", provisioned.MsIP)
		}
	})

	t.Run("instance is absent from a working browse", func(t *testing.T) {
		_, stopControl := requireMulticastEnvironment(t)
		defer stopControl()

		entries := mdnsBrowse(t, 3*time.Second)
		if entry, ok := entries[mdnsAdvertisedFQDN(name)]; ok {
			t.Errorf("instance %q discovered despite SPAXEL_MDNS_ENABLED=false (advertised port %d)", entry.Name, entry.Port)
		}
	})
}

func TestAdvertisedBaseURLDiagnostics(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping advertised-URL integration test in short mode")
	}

	bin := buildDemoBinary(t)

	t.Run("non-routable advertised URL warns and falls back to the system interface", func(t *testing.T) {
		// 203.0.113.0/24 is TEST-NET-3: never routable, never a local
		// interface, but a syntactically valid http URL. Startup must degrade
		// loudly, not die — the mDNS advertisement falls back to the system
		// multicast interface so nodes on the same segment still resolve.
		name := mdnsTestName()
		srv := startMDNSServer(t, bin, t.TempDir(),
			"SPAXEL_MDNS_ENABLED=true",
			"SPAXEL_MDNS_NAME="+name,
			"SPAXEL_ADVERTISED_BASE_URL=http://203.0.113.7:8080",
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

		wantWarn := fmt.Sprintf("Could not bind mDNS to the node-reachable address %q", "http://203.0.113.7:8080")
		if !strings.Contains(srv.stderr.String(), wantWarn) {
			t.Errorf("stderr lacks the actionable WARN %q; stderr:\n%s", wantWarn, srv.stderr.String())
		}

		// The fallback advertisement must still name the real listener port —
		// regression guard for the hardcoded :8080 the advertisement used to
		// carry regardless of SPAXEL_BIND_ADDR.
		wantLog := fmt.Sprintf("mDNS advertising %s._spaxel._tcp.local:%d", name, bindPort)
		if !strings.Contains(srv.stderr.String(), wantLog) {
			t.Errorf("stderr lacks %q; stderr:\n%s", wantLog, srv.stderr.String())
		}

		_, stopControl := requireMulticastEnvironment(t)
		defer stopControl()
		entries := mdnsBrowse(t, 3*time.Second)
		entry, ok := entries[mdnsAdvertisedFQDN(name)]
		if !ok {
			t.Fatalf("fallback advertisement for %q not discovered", name)
		}
		if entry.Port != bindPort {
			t.Errorf("fallback advertisement port %d, want the HTTP listener port %d", entry.Port, bindPort)
		}
	})

	t.Run("wildcard advertised URL is a fatal startup error", func(t *testing.T) {
		// 0.0.0.0 advertised to nodes is worse than no advertisement — every
		// node would dial a wildcard. ADR-004: explicit-but-invalid is fatal.
		assertFatalStartup(t, bin,
			[]string{"SPAXEL_MDNS_ENABLED=true", "SPAXEL_ADVERTISED_BASE_URL=http://0.0.0.0:8080"},
			"wildcard",
		)
	})
}

func TestNodeReconnectAfterDisconnect(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping node reconnect integration test in short mode")
	}

	// mDNS off: the reconnect contract is independent of discovery, and this
	// keeps the scenario hermetic on multicast-less hosts.
	srv := startMDNSServer(t, buildDemoBinary(t), t.TempDir(), "SPAXEL_MDNS_ENABLED=false")
	defer func() { _ = srv.stop() }()

	client := loginAsAdmin(t, srv.baseURL)

	const mac = "AA:BB:CC:DD:00:42"
	token := postProvision(t, srv.baseURL, fmt.Sprintf(`{"mac":%q}`, mac)).NodeToken

	nodeURL := "ws://" + strings.TrimPrefix(srv.baseURL, "http://") + "/ws/node"
	connect := func() *websocket.Conn {
		t.Helper()
		header := http.Header{}
		header.Set("X-Spaxel-Token", token)
		conn, _, err := websocket.DefaultDialer.Dial(nodeURL, header)
		if err != nil {
			t.Fatalf("dial %s: %v", nodeURL, err)
		}
		hello := map[string]interface{}{
			"type":             "hello",
			"mac":              mac,
			"node_id":          "e2e-reconnect-" + mac,
			"firmware_version": "0.1.0-e2e",
			"capabilities":     []string{"csi", "tx", "rx"},
			"chip":             "ESP32-S3",
			"flash_mb":         16,
			"uptime_ms":        1000,
		}
		if err := conn.WriteJSON(hello); err != nil {
			t.Fatalf("send hello: %v", err)
		}
		return conn
	}

	conn := connect()
	waitUntil(t, 15*time.Second, "node "+mac+" to come online",
		func() bool { return fleetNodeOnline(t, client, srv.baseURL, mac) })

	// Abrupt drop — no close frame, exactly like a power-cut node. The hub's
	// connection state is authoritative, so offline must not wait out any
	// last-seen grace window.
	if err := conn.Close(); err != nil {
		t.Logf("close returned %v (expected for an abrupt client close)", err)
	}
	waitUntil(t, 15*time.Second, "node "+mac+" to drop offline after disconnect",
		func() bool { return !fleetNodeOnline(t, client, srv.baseURL, mac) })

	// Redial with the SAME MAC and the SAME token: the provisioning payload
	// written to the node's NVS stays valid across reconnects.
	reconnected := connect()
	defer func() { _ = reconnected.Close() }()
	waitUntil(t, 15*time.Second, "node "+mac+" to come back online without re-provisioning",
		func() bool { return fleetNodeOnline(t, client, srv.baseURL, mac) })
}

func TestComposeHostNetworkingContract(t *testing.T) {
	// The plan's anti-pattern: a Docker bridge network drops multicast
	// (224.0.0.251), so a bridged mothership is undiscoverable by mDNS no
	// matter how correctly it advertises. The compose file must keep
	// network_mode: host — or, if it ever moves back to a bridge, must
	// explicitly set SPAXEL_MDNS_ENABLED=false so operators get the documented
	// manual-IP fallback instead of silent discovery failure.
	composePath := filepath.Join(moduleRoot(), "..", "docker-compose.yml")
	raw, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read %s: %v", composePath, err)
	}

	hasHostNetworking := false
	disablesMDNS := false
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "network_mode:") && strings.Contains(trimmed, "host") {
			hasHostNetworking = true
		}
		if strings.HasPrefix(trimmed, "SPAXEL_MDNS_ENABLED:") && !strings.Contains(trimmed, "${") && strings.Contains(trimmed, "false") {
			disablesMDNS = true
		}
	}

	t.Run("compose keeps host networking for multicast", func(t *testing.T) {
		if !hasHostNetworking {
			t.Errorf("%s lost network_mode: host", composePath)
		}
	})

	t.Run("bridge networking would require SPAXEL_MDNS_ENABLED=false", func(t *testing.T) {
		if hasHostNetworking || disablesMDNS {
			return
		}
		t.Errorf("%s neither uses network_mode: host nor sets SPAXEL_MDNS_ENABLED=false — on a Docker bridge multicast is dropped and nodes can never discover the mothership; pick one of the two documented modes", composePath)
	})
}
