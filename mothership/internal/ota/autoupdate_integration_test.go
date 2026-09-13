// Package ota — end-to-end auto-update integration test (spaxel-0b49ab99).
//
// These tests drive the real auto-update state machine — the same
// checkForNewFirmware → selectCanary → canary deploy → monitor → evaluate →
// fleet rollout path production traffic takes — against a simulated node
// fleet. The emulator (simFleet) is the node side: it fetches the image the
// manager advertises over HTTP, verifies its SHA-256, reports download
// status, reboots, and says hello running the flashed version through the
// same OnOTAStatus / OnNodeReconnected hooks the ingestion server calls.
//
// Verified here, on top of the downgrade-prevention work from
// spaxel-005c84ce:
//   - mothership holds several releases (old / current / new) and a cycle
//     always installs the NEWEST one, never an older release;
//   - when the only available release is older than what the fleet already
//     runs, no cycle starts at all (and the explicit allow_downgrade setting
//     is the only way through);
//   - the deployment-time and per-node fleet gates hold even when version
//     reports change mid-cycle;
//   - every decision is logged (selection, trigger, canary deployment and
//     verification, per-node rollout, completion, and each downgrade skip);
//   - ten consecutive auto-update cycles complete without a single silent
//     rollback: no node's running version ever decreases.
package ota

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// simNode is one simulated node: the firmware it currently runs plus every
// version it has ever reported in a hello, in order.
type simNode struct {
	mac     string
	version string
	history []string
}

// otaSend records one OTA command the manager issued, the way the node
// received it: the advertised URL, the expected checksum and version.
type otaSend struct {
	mac     string
	url     string
	sha256  string
	version string
}

// simFleet is the node side of the integration test. It implements the OTA
// manager's NodeSender (plus its versionProvider extension), applies each
// update command the way an ESP32 does — download over HTTP, verify the
// image hash, report status, reboot — and says hello back through the same
// manager hooks the ingestion server drives. The fleet version view that
// canary selection and downgrade prevention read (mockNodeProvider) is
// updated when each hello lands, exactly as the ingestion server's would be.
type simFleet struct {
	mu       sync.Mutex
	t        *testing.T
	mgr      *Manager
	fleet    *mockNodeProvider
	images   map[string][]byte // filename → image bytes, served over HTTP
	nodes    map[string]*simNode
	sends    []otaSend
	failNext map[string]string // mac → inject the given OTA error on next send
}

func newSimFleet(t *testing.T, mgr *Manager, fleet *mockNodeProvider) *simFleet {
	return &simFleet{
		t:        t,
		mgr:      mgr,
		fleet:    fleet,
		images:   make(map[string][]byte),
		nodes:    make(map[string]*simNode),
		failNext: make(map[string]string),
	}
}

// addNode boots a node into the fleet running the given version.
func (f *simFleet) addNode(mac, role string, health float64, version string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nodes[mac] = &simNode{mac: mac, version: version, history: []string{version}}
	f.fleet.addNodeWithFirmware(mac, role, health, version)
}

// GetConnectedMACs implements NodeSender.
func (f *simFleet) GetConnectedMACs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	macs := make([]string, 0, len(f.nodes))
	for mac := range f.nodes {
		macs = append(macs, mac)
	}
	return macs
}

// GetNodeFirmwareVersion implements the versionProvider extension: the
// sender knows what each node runs, so OTA logs show a real version_before.
func (f *simFleet) GetNodeFirmwareVersion(mac string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if n, ok := f.nodes[mac]; ok {
		return n.version
	}
	return ""
}

// SendOTAToMAC implements NodeSender: run the ESP32 OTA flow for this node,
// synchronously, so each cycle's outcome is deterministic when it returns.
func (f *simFleet) SendOTAToMAC(mac, url, sha, version string) {
	f.mu.Lock()
	f.sends = append(f.sends, otaSend{mac: mac, url: url, sha256: sha, version: version})
	if err, ok := f.failNext[mac]; ok {
		delete(f.failNext, mac)
		f.mu.Unlock()
		f.mgr.OnOTAStatus(mac, "failed", 0, err)
		return
	}
	n := f.nodes[mac]
	f.mu.Unlock()

	if n == nil {
		f.mgr.OnOTAStatus(mac, "failed", 0, "unknown node")
		return
	}

	// Download: the node fetches exactly the URL the manager advertised and
	// refuses to flash an image that does not match the advertised checksum.
	resp, err := http.Get(url)
	if err != nil {
		f.mgr.OnOTAStatus(mac, "failed", 0, fmt.Sprintf("download: %v", err))
		return
	}
	image, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		f.mgr.OnOTAStatus(mac, "failed", 0, fmt.Sprintf("read: %v", err))
		return
	}
	if resp.StatusCode != http.StatusOK {
		f.mgr.OnOTAStatus(mac, "failed", 0, fmt.Sprintf("download: status %d", resp.StatusCode))
		return
	}
	sum := sha256.Sum256(image)
	if hex.EncodeToString(sum[:]) != sha {
		f.mgr.OnOTAStatus(mac, "failed", 0, "image checksum mismatch")
		return
	}

	// Flash, reboot, hello: the node comes back running the flashed version.
	f.mgr.OnOTAStatus(mac, "downloading", 100, "")
	f.mgr.OnOTAStatus(mac, "rebooting", 100, "")

	f.mu.Lock()
	n.version = version
	n.history = append(n.history, version)
	f.mu.Unlock()
	f.fleet.reportHelloFirmware(mac, version)

	f.mgr.OnNodeReconnected(mac, version)
}

// sendsTo returns every OTA command issued since the mark, plus the new mark.
func (f *simFleet) sendsSince(mark int) (sends []otaSend, next int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]otaSend(nil), f.sends[mark:]...), len(f.sends)
}

// sentCount reports how many OTA commands have been issued in total.
func (f *simFleet) sentCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sends)
}

// versionOf returns the version the node is running right now.
func (f *simFleet) versionOf(mac string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.nodes[mac].version
}

// historyOf returns every version the node has reported, oldest first.
func (f *simFleet) historyOf(mac string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.nodes[mac].history...)
}

// serveFirmware serves the seeded images over HTTP at /firmware/<filename>,
// the path the manager's advertised base URL produces.
func (f *simFleet) serveFirmware() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/firmware/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/firmware/")
		f.mu.Lock()
		image, ok := f.images[name]
		f.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(image)
	})
	return httptest.NewServer(mux)
}

// reportHelloFirmware records the firmware version a node announced in its
// hello message after rebooting — the ingestion server's fleet view update.
func (m *mockNodeProvider) reportHelloFirmware(mac, version string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.nodes[mac]; ok {
		m.firmwareVersions[mac] = version
	}
}

// logCapture collects the package's log output so the tests can assert the
// auto-update decision lines appeared with the right values. log output is
// global, so the tests using it must not run in parallel — they don't.
type logCapture struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (c *logCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.Write(p)
}

func (c *logCapture) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}

// captureLogs diverts package log output into a buffer for the duration of
// the test.
func captureLogs(t *testing.T) *logCapture {
	t.Helper()
	c := &logCapture{}
	log.SetOutput(c)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return c
}

// integrationFleet is one wired-up stack: mothership firmware store, real
// OTA manager, real auto-update manager, simulated fleet, mock providers.
type integrationFleet struct {
	srv          *Server
	autoMgr      *AutoUpdateManager
	settings     *mockSettingsProvider
	quality      *mockQualityProvider
	nodeProvider *mockNodeProvider
	notifier     *mockEventNotifier
	sim          *simFleet
	fwSrv        *httptest.Server
	logs         *logCapture
}

// newIntegrationFleet wires the full auto-update stack the way main.go does,
// with the timers shortened through the manager's injectable intervals so a
// whole canary cycle runs in milliseconds: the canary monitor ticks every
// 5ms instead of 30s, a parked rollout re-reads the quiet window every 10ms
// instead of 30s, and the canary duration is zero (the monitoring window has
// elapsed by the first tick). Quality holds steady at 0.85 so a canary never
// trips the degradation threshold.
func newIntegrationFleet(t *testing.T) *integrationFleet {
	t.Helper()

	srv := &Server{}
	otaMgr := NewManager(srv, "") // base URL set once the firmware server is up

	settings := newMockSettingsProvider()
	settings.set("auto_update_enabled", true)
	settings.set("canary_duration_min", float64(0))
	settings.set("auto_update_quality_threshold", 0.05)
	settings.set("quiet_window_start", "")
	settings.set("quiet_window_end", "")

	autoMgr := NewAutoUpdateManager(srv, otaMgr, time.UTC)
	autoMgr.SetSettingsProvider(settings)

	quality := newMockQualityProvider()
	quality.setQuality(0.85)
	autoMgr.SetQualityProvider(quality)

	nodeProvider := newMockNodeProvider()
	autoMgr.SetNodeProvider(nodeProvider)

	notifier := newMockEventNotifier()
	autoMgr.SetEventNotifier(notifier)

	sim := newSimFleet(t, otaMgr, nodeProvider)
	fwSrv := sim.serveFirmware()
	t.Cleanup(fwSrv.Close)
	otaMgr.baseURL = fwSrv.URL
	otaMgr.SetSender(sim)

	autoMgr.mu.Lock()
	autoMgr.windowPoll = 10 * time.Millisecond
	autoMgr.monitorPoll = 5 * time.Millisecond
	autoMgr.mu.Unlock()

	f := &integrationFleet{
		srv:          srv,
		autoMgr:      autoMgr,
		settings:     settings,
		quality:      quality,
		nodeProvider: nodeProvider,
		notifier:     notifier,
		sim:          sim,
		fwSrv:        fwSrv,
		logs:         captureLogs(t),
	}

	// Sanity: the wiring must not wedge the manager. GetState takes m.mu;
	// if anything self-deadlocks on the first cycle this catches it with a
	// readable message instead of a go-test timeout dump.
	done := make(chan UpdateState, 1)
	go func() { done <- autoMgr.GetState() }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("auto-update manager wedged before any cycle: GetState blocked on m.mu")
	}

	return f
}

// seedRelease adds a firmware release to the store (the firmware directory
// scan result) and the simulated download server. When latest is set, the
// store's latest pointer moves to it, as a directory rescan would.
func (f *integrationFleet) seedRelease(version string, latest bool) string {
	filename := fmt.Sprintf("spaxel-%s.bin", version)
	image := []byte("spaxel firmware image " + version)
	sum := sha256.Sum256(image)

	if f.srv.firmware == nil {
		f.srv.firmware = make(map[string]*FirmwareMeta)
	}
	f.srv.firmware[filename] = &FirmwareMeta{
		Filename:  filename,
		Version:   version,
		SHA256:    hex.EncodeToString(sum[:]),
		SizeBytes: int64(len(image)),
	}
	f.sim.mu.Lock()
	f.sim.images[filename] = image
	f.sim.mu.Unlock()

	if latest {
		f.srv.latestFile = filename
	}
	return filename
}

// waitForCycleState waits for the state machine to reach want. GetState is
// read on its own goroutine with a timeout so a regression that wedges m.mu
// fails with a targeted message rather than hanging the whole run.
func (f *integrationFleet) waitForCycleState(t *testing.T, want UpdateState, timeout time.Duration) UpdateState {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ch := make(chan UpdateState, 1)
		go func() { ch <- f.autoMgr.GetState() }()
		select {
		case got := <-ch:
			if got == want {
				return got
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("auto-update manager wedged: GetState blocked for 2s while waiting for %q", want)
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("auto-update state = %q after %s, want %q", f.getState(), timeout, want)
	return ""
}

func (f *integrationFleet) getState() UpdateState {
	ch := make(chan UpdateState, 1)
	go func() { ch <- f.autoMgr.GetState() }()
	select {
	case s := <-ch:
		return s
	case <-time.After(2 * time.Second):
		return "wedged"
	}
}

// eventsOfType returns the notifier's recorded events of one type.
func eventsOfType(notifier *mockEventNotifier, eventType string) []mockEvent {
	var out []mockEvent
	for _, e := range notifier.getEvents() {
		if e.eventType == eventType {
			out = append(out, e)
		}
	}
	return out
}

// requireLogSubstring fails the test unless the captured log contains want.
func requireLogSubstring(t *testing.T, logs *logCapture, want string) {
	t.Helper()
	if !strings.Contains(logs.String(), want) {
		t.Errorf("log line missing:\n  want: %s", want)
	}
}

// requireNoLogSubstring fails the test if the captured log contains banned.
func requireNoLogSubstring(t *testing.T, logs *logCapture, banned string) {
	t.Helper()
	if strings.Contains(logs.String(), banned) {
		t.Errorf("log line present but must not be:\n  banned: %s", banned)
	}
}

// TestAutoUpdateIntegrationInstallsNewestFirmware covers the core scenario:
// mothership holds an old release, the release the fleet currently runs, and
// a newer release; a node fleet running the current version auto-updates and
// every node ends up on the NEWEST release — the old release is never
// installed — with every decision logged.
func TestAutoUpdateIntegrationInstallsNewestFirmware(t *testing.T) {
	f := newIntegrationFleet(t)
	ctx := context.Background()

	// Mothership firmware store: an old release, the current one, and a
	// newer release marked latest (the directory scan's newest pick).
	f.seedRelease("0.1.9", false)
	f.seedRelease("0.2.1", false)
	f.seedRelease("0.2.2", true)

	// Two nodes online running the current release; the healthiest is the
	// canary candidate.
	f.sim.addNode("AA:BB:CC:00:00:01", "tx_rx", 0.9, "0.2.1")
	f.sim.addNode("AA:BB:CC:00:00:02", "tx_rx", 0.7, "0.2.1")

	// The automatic check (the run loop's per-minute tick, called directly)
	// must select the newest release and run a full canary cycle.
	f.autoMgr.checkForNewFirmware(ctx)
	f.waitForCycleState(t, StateComplete, 10*time.Second)
	f.autoMgr.wg.Wait() // rollout goroutine is done once the state settles

	if got := f.autoMgr.GetCanaryNode(); got != "AA:BB:CC:00:00:01" {
		t.Errorf("canary = %q, want the healthiest node AA:BB:CC:00:00:01", got)
	}

	// Every OTA command the manager issued carried the newest version —
	// never the old release, never the already-running one.
	sends := f.sim.sends
	if len(sends) != 2 {
		t.Fatalf("OTA sends = %d (%+v), want 2 (canary + fleet)", len(sends), sends)
	}
	for _, s := range sends {
		if s.version != "0.2.2" {
			t.Errorf("OTA to %s carried version %q, want the newest 0.2.2", s.mac, s.version)
		}
		wantURL := f.fwSrv.URL + "/firmware/spaxel-0.2.2.bin"
		if s.url != wantURL {
			t.Errorf("OTA to %s advertised url %q, want %q", s.mac, s.url, wantURL)
		}
	}
	if first := sends[0]; first.mac != "AA:BB:CC:00:00:01" {
		t.Errorf("first OTA went to %q, want the canary", first.mac)
	}

	// Both nodes run the newest release; neither ever ran anything else.
	for _, mac := range []string{"AA:BB:CC:00:00:01", "AA:BB:CC:00:00:02"} {
		if got := f.sim.versionOf(mac); got != "0.2.2" {
			t.Errorf("node %s runs %q, want 0.2.2", mac, got)
		}
		for _, v := range f.sim.historyOf(mac) {
			if v == "0.1.9" {
				t.Errorf("node %s reported running the old release 0.1.9 at some point", mac)
			}
		}
	}

	// The canary's OTA outcome is verified with the expected version. (The
	// monitor's own "canary %s verified" log line is unreachable in a
	// zero-length canary window — the first tick takes the deadline branch —
	// so the verification signal is asserted here instead.)
	progress := f.autoMgr.otaManager.GetProgress()
	if p, ok := progress["AA:BB:CC:00:00:01"]; !ok {
		t.Fatal("no OTA progress recorded for the canary")
	} else if p.State != OTAVerified || p.ExpectedVersion != "0.2.2" || p.PreviousVersion != "0.2.1" {
		t.Errorf("canary progress = %+v, want verified 0.2.1 → 0.2.2", p)
	}

	// The timeline recorded the full cycle.
	for _, eventType := range []string{"update_started", "canary_deploy", "canary_evaluated", "canary_passed", "node_update", "update_complete"} {
		if n := len(eventsOfType(f.notifier, eventType)); n != 1 {
			t.Errorf("%s events = %d, want 1", eventType, n)
		}
	}
	for _, e := range eventsOfType(f.notifier, "canary_deploy") {
		want := map[string]interface{}{
			"version_before": "0.2.1",
			"version_after":  "0.2.2",
			"trigger_type":   "automatic_canary",
		}
		for k, v := range want {
			if e.metadata[k] != v {
				t.Errorf("canary_deploy metadata[%q] = %v, want %v", k, e.metadata[k], v)
			}
		}
	}
	for _, e := range eventsOfType(f.notifier, "update_complete") {
		if e.metadata["firmware_version"] != "0.2.2" {
			t.Errorf("update_complete firmware_version = %v, want 0.2.2", e.metadata["firmware_version"])
		}
	}

	// Every decision was logged, with the right values attached. (The
	// selection line's sha256 field sits between filename and
	// selection_reason, so the two halves are asserted separately.)
	logTable := []string{
		"AUTO-UPDATE firmware selection: version=0.2.2 filename=spaxel-0.2.2.bin",
		"selection_reason=latest_stable_fresh_scan source=fresh_getLatest",
		"Auto-update triggered: trigger_type=auto version=0.2.2 filename=spaxel-0.2.2.bin",
		"AUTO-UPDATE cycle started: firmware_version=0.2.2 filename=spaxel-0.2.2.bin",
		"AUTO-UPDATE canary deployment: node=AA:BB:CC:00:00:01 update_type=auto version_before=0.2.1 version_after=0.2.2 baseline_quality=0.85 trigger_type=automatic_canary",
		"OTA initiated: node=AA:BB:CC:00:00:01 update_type=auto version_before=0.2.1 version_after=0.2.2",
		"update verified: node=AA:BB:CC:00:00:01 update_type=auto version_before=0.2.1 version_after=0.2.2",
		"AUTO-UPDATE canary passed: node=AA:BB:CC:00:00:01 update_type=auto version_before=0.2.1 version_after=0.2.2 quality_delta=0.00% threshold=5.00%",
		"AUTO-UPDATE node update: node=AA:BB:CC:00:00:02 update_type=auto version_before=0.2.1 version_after=0.2.2 trigger_type=automatic_fleet",
		"AUTO-UPDATE fleet rollout complete: firmware_version=0.2.2 nodes_updated=1",
	}
	for _, want := range logTable {
		requireLogSubstring(t, f.logs, want)
	}
	requireNoLogSubstring(t, f.logs, "downgrade prevention")
	requireNoLogSubstring(t, f.logs, "rollback")

	// The outcome counter recorded exactly one success and no failure.
	if got := triggerCounterValue(t, "failure"); got != 0 {
		t.Errorf("auto/failure counter = %v, want 0", got)
	}
}

// TestAutoUpdateIntegrationRejectsOlderRelease covers downgrade prevention
// at selection time: the only available releases are older than what the
// fleet already runs, so no cycle may start — and the per-minute check stays
// quiet afterwards (the release is latched as handled). The explicit
// allow_downgrade setting is the only way an older release is installed.
func TestAutoUpdateIntegrationRejectsOlderRelease(t *testing.T) {
	f := newIntegrationFleet(t)
	ctx := context.Background()

	// Store: an old release and one just below the fleet's current — both
	// older than what every node runs.
	f.seedRelease("0.1.9", false)
	f.seedRelease("0.2.0", true)
	f.sim.addNode("AA:BB:CC:00:00:01", "tx_rx", 0.9, "0.2.1")
	f.sim.addNode("AA:BB:CC:00:00:02", "tx_rx", 0.7, "0.2.1")

	// First automatic check: skip, latch, no cycle.
	f.autoMgr.checkForNewFirmware(ctx)

	if got := f.getState(); got != StateIdle {
		t.Errorf("state = %q after a downgrade candidate, want idle", got)
	}
	if got := f.sim.sentCount(); got != 0 {
		t.Errorf("OTA sends = %d, want 0 — nothing may be pushed for a downgrade candidate", got)
	}
	if n := len(eventsOfType(f.notifier, "update_started")); n != 0 {
		t.Errorf("update_started events = %d, want 0", n)
	}
	f.autoMgr.mu.RLock()
	latched := f.autoMgr.pendingFirmware != nil && f.autoMgr.pendingFirmware.Filename == "spaxel-0.2.0.bin"
	f.autoMgr.mu.RUnlock()
	if !latched {
		t.Error("downgrade candidate not latched; the per-minute check would re-log it every tick")
	}
	requireLogSubstring(t, f.logs,
		"downgrade prevention: candidate 0.2.0 (spaxel-0.2.0.bin) is not newer than 0.2.1 already running on AA:BB:CC:00:00:")
	requireNoLogSubstring(t, f.logs, "Auto-update triggered")

	// Second check (the next per-minute tick): the latch keeps it quiet.
	f.autoMgr.checkForNewFirmware(ctx)
	if got := strings.Count(f.logs.String(), "downgrade prevention: candidate 0.2.0"); got != 1 {
		t.Errorf("downgrade-prevention skip logged %d times across two checks, want exactly 1 (latched)", got)
	}

	// The operator sets allow_downgrade: the same candidate now runs, the
	// only path by which an older release is ever installed.
	f.settings.set("auto_update_allow_downgrade", true)
	f.autoMgr.CancelUpdate() // clear the handled-latch, as the API would
	f.autoMgr.checkForNewFirmware(ctx)
	f.waitForCycleState(t, StateComplete, 10*time.Second)
	f.autoMgr.wg.Wait()

	if got := f.sim.sentCount(); got == 0 {
		t.Error("no OTA sent despite allow_downgrade")
	}
	for _, mac := range []string{"AA:BB:CC:00:00:01", "AA:BB:CC:00:00:02"} {
		if got := f.sim.versionOf(mac); got != "0.2.0" {
			t.Errorf("node %s runs %q, want 0.2.0 (explicit allowance honored)", mac, got)
		}
	}
}

// flipOnCallNodeProvider reports a stale running version on one numbered
// call, simulating a node whose version report changes between canary
// selection and deployment.
type flipOnCallNodeProvider struct {
	*mockNodeProvider
	stale   string
	flipAt  int
	calls   int
	flipped bool
}

func (v *flipOnCallNodeProvider) GetNodeFirmwareVersion(mac string) string {
	v.calls++
	if v.calls == v.flipAt {
		v.flipped = true
		return v.stale
	}
	return v.mockNodeProvider.GetNodeFirmwareVersion(mac)
}

// TestAutoUpdateIntegrationDowngradeGatesHold covers the two defense-in-depth
// gates on top of selection-time prevention: a canary that turns out to
// already run a newer build is not rolled back at deployment time, and a
// fleet node that runs ahead of the candidate is skipped during rollout.
func TestAutoUpdateIntegrationDowngradeGatesHold(t *testing.T) {
	t.Run("CanaryDeploymentGate", func(t *testing.T) {
		f := newIntegrationFleet(t)
		ctx := context.Background()

		// One node whose reported version turns newer between selection
		// (calls 1–2: oldest-node check, canary selection) and deployment
		// (call 3: canaryPreviousVersion).
		f.seedRelease("0.1.357", true)
		f.sim.addNode("AA:BB:CC:00:00:01", "tx_rx", 0.9, "0.1.356")
		flip := &flipOnCallNodeProvider{mockNodeProvider: f.nodeProvider, stale: "0.1.358", flipAt: 3}
		f.autoMgr.mu.Lock()
		f.autoMgr.nodeProvider = flip
		f.autoMgr.mu.Unlock()

		failureBefore := triggerCounterValue(t, "failure")
		f.autoMgr.checkForNewFirmware(ctx)

		if got := f.getState(); got != StateIdle {
			t.Errorf("state = %q, want idle (the gate resets the cycle, it does not fail it)", got)
		}
		if !flip.flipped {
			t.Error("version flip never observed; the deployment gate was not exercised")
		}
		if got := f.sim.sentCount(); got != 0 {
			t.Errorf("OTA sends = %d, want 0 — the canary must not be rolled back", got)
		}
		skips := eventsOfType(f.notifier, "update_skipped")
		if len(skips) != 1 {
			t.Fatalf("update_skipped events = %d, want 1", len(skips))
		}
		if skips[0].metadata["reason"] != "downgrade_prevented" || skips[0].metadata["running_version"] != "0.1.358" {
			t.Errorf("update_skipped metadata = %v, want reason=downgrade_prevented running_version=0.1.358", skips[0].metadata)
		}
		if got := f.sim.versionOf("AA:BB:CC:00:00:01"); got != "0.1.356" {
			t.Errorf("node runs %q, want 0.1.356 (untouched — no OTA may reach it)", got)
		}
		requireLogSubstring(t, f.logs,
			"downgrade prevention: canary AA:BB:CC:00:00:01 already runs 0.1.358, candidate 0.1.357 is not newer")
		if got := triggerCounterValue(t, "failure") - failureBefore; got != 0 {
			t.Errorf("auto/failure counter delta = %v, want 0 (a skip is not a failure)", got)
		}
	})

	t.Run("FleetRolloutSkipsNodeRunningNewer", func(t *testing.T) {
		f := newIntegrationFleet(t)
		ctx := context.Background()

		// The canary lags the candidate; another node runs ahead of it. The
		// candidate upgrades the fleet overall (the oldest node), but the
		// rollout must skip the node that is already newer.
		f.seedRelease("0.2.0", false)
		f.seedRelease("0.2.1", true)
		f.sim.addNode("AA:BB:CC:00:00:01", "tx_rx", 0.9, "0.2.0")
		f.sim.addNode("AA:BB:CC:00:00:02", "tx_rx", 0.7, "0.3.0")

		f.autoMgr.checkForNewFirmware(ctx)
		f.waitForCycleState(t, StateComplete, 10*time.Second)
		f.autoMgr.wg.Wait()

		// The canary was upgraded; the ahead node was skipped, not rolled
		// back.
		if got := f.sim.versionOf("AA:BB:CC:00:00:01"); got != "0.2.1" {
			t.Errorf("canary runs %q, want 0.2.1", got)
		}
		if got := f.sim.versionOf("AA:BB:CC:00:00:02"); got != "0.3.0" {
			t.Errorf("fleet node runs %q, want 0.3.0 (must never be silently downgraded)", got)
		}
		sends, _ := f.sim.sendsSince(0)
		for _, s := range sends {
			if s.mac == "AA:BB:CC:00:00:02" {
				t.Error("OTA issued to a node already running a newer build")
			}
		}
		nodeUpdates := eventsOfType(f.notifier, "node_update")
		if len(nodeUpdates) != 0 {
			t.Errorf("node_update events = %d, want 0 (the only remaining node was skipped)", len(nodeUpdates))
		}
		requireLogSubstring(t, f.logs,
			"downgrade prevention: skipping node AA:BB:CC:00:00:02 already running 0.3.0, candidate 0.2.1 is not newer")
		requireLogSubstring(t, f.logs,
			"AUTO-UPDATE fleet rollout complete: firmware_version=0.2.1 nodes_updated=0")
	})
}

// TestAutoUpdateIntegrationFleetRolloutWaitsForQuietWindow covers the quiet
// window half of ADR-009 decision 4: the canary deploys immediately, the
// rest of the fleet holds while the window is closed, and a window change is
// picked up by the parked rollout without a restart.
func TestAutoUpdateIntegrationFleetRolloutWaitsForQuietWindow(t *testing.T) {
	f := newIntegrationFleet(t)
	ctx := context.Background()

	f.seedRelease("0.2.1", false)
	f.seedRelease("0.2.2", true)
	f.sim.addNode("AA:BB:CC:00:00:01", "tx_rx", 0.9, "0.2.1")
	f.sim.addNode("AA:BB:CC:00:00:02", "tx_rx", 0.7, "0.2.1")

	// A closed window: start == end is an empty window, so the hold is
	// deterministic no matter what wall-clock time the test runs at.
	f.settings.set("quiet_window_start", "05:00")
	f.settings.set("quiet_window_end", "05:00")

	f.autoMgr.checkForNewFirmware(ctx)

	// The canary went out immediately; the fleet node is untouched while the
	// rollout is parked.
	f.waitForCycleState(t, StateWaitingWindow, 10*time.Second)
	if got := f.sim.versionOf("AA:BB:CC:00:00:01"); got != "0.2.2" {
		t.Errorf("canary runs %q, want 0.2.2 (canary deploys before the window)", got)
	}
	if got := f.sim.versionOf("AA:BB:CC:00:00:02"); got != "0.2.1" {
		t.Errorf("fleet node runs %q, want 0.2.1 (rollout must hold for the window)", got)
	}
	if n := len(eventsOfType(f.notifier, "waiting_window")); n != 1 {
		t.Errorf("waiting_window events = %d, want 1", n)
	}
	requireLogSubstring(t, f.logs,
		"AUTO-UPDATE fleet rollout of 0.2.2 holding for quiet window (05:00–05:00), canary already deployed")

	// The operator clears the window; the parked rollout notices on its next
	// re-read and finishes the fleet.
	f.settings.set("quiet_window_start", "")
	f.settings.set("quiet_window_end", "")

	f.waitForCycleState(t, StateComplete, 10*time.Second)
	f.autoMgr.wg.Wait()

	if got := f.sim.versionOf("AA:BB:CC:00:00:02"); got != "0.2.2" {
		t.Errorf("fleet node runs %q, want 0.2.2 after the window opened", got)
	}
	if n := len(eventsOfType(f.notifier, "quiet_window_open")); n != 1 {
		t.Errorf("quiet_window_open events = %d, want 1", n)
	}
	requireLogSubstring(t, f.logs,
		"AUTO-UPDATE quiet window open, resuming fleet rollout of 0.2.2")
}

// TestAutoUpdateIntegrationTenCyclesNoSilentRollback is the soak: ten
// consecutive auto-update cycles, each publishing a newer release with every
// prior release still in the store. Between cycles it asserts that every OTA
// command carried that cycle's newest version, no node's running version ever
// decreased, no rollback state was ever entered, and each cycle completed
// cleanly with its decisions logged.
func TestAutoUpdateIntegrationTenCyclesNoSilentRollback(t *testing.T) {
	f := newIntegrationFleet(t)

	const cycles = 10

	// Store lineage: the fleet's starting release plus one older orphan —
	// old versions stay in the store forever, so every cycle's selection has
	// older releases to wrongly pick if downgrade prevention regresses.
	f.seedRelease("0.1.0", false)
	f.seedRelease("0.1.9", false)
	f.sim.addNode("AA:BB:CC:00:00:01", "tx_rx", 0.9, "0.1.0")
	f.sim.addNode("AA:BB:CC:00:00:02", "tx_rx", 0.7, "0.1.0")

	successBefore := triggerCounterValue(t, "success")
	failureBefore := triggerCounterValue(t, "failure")

	for i := 1; i <= cycles; i++ {
		version := fmt.Sprintf("0.2.%02d", i)
		filename := f.seedRelease(version, true) // newest release lands
		mark := f.sim.sentCount()
		failureMark := triggerCounterValue(t, "failure")

		// The upload hook fires the automatic check, exactly as a fresh
		// directory scan does in production.
		f.autoMgr.OnFirmwareUploaded(filename)
		f.waitForCycleState(t, StateComplete, 10*time.Second)
		f.autoMgr.wg.Wait()

		wantBefore := fmt.Sprintf("0.2.%02d", i-1)
		if i == 1 {
			wantBefore = "0.1.0"
		}

		// Every OTA command this cycle carried the new newest version.
		sends, _ := f.sim.sendsSince(mark)
		if len(sends) != 2 {
			t.Errorf("cycle %d: OTA sends = %d, want 2 (canary + fleet)", i, len(sends))
		}
		for _, s := range sends {
			if s.version != version {
				t.Errorf("cycle %d: OTA to %s carried %q, want the newest release %s", i, s.mac, s.version, version)
			}
		}

		// Both nodes run the new version; neither ever reported anything
		// older than what it ran before this cycle (no silent rollback).
		for _, mac := range []string{"AA:BB:CC:00:00:01", "AA:BB:CC:00:00:02"} {
			if got := f.sim.versionOf(mac); got != version {
				t.Errorf("cycle %d: node %s runs %q, want %s", i, mac, got, version)
			}
			history := f.sim.historyOf(mac)
			if len(history) < 2 {
				t.Fatalf("cycle %d: node %s reported no new version", i, mac)
			}
			last, prev := history[len(history)-1], history[len(history)-2]
			if compareFirmwareVersions(last, prev) < 0 {
				t.Errorf("cycle %d: node %s went from %s back to %s — silent rollback", i, mac, prev, last)
			}
		}

		// No rollback or failure anywhere: not in the OTA progress, not in
		// the timeline, not in the counters.
		for mac, p := range f.autoMgr.otaManager.GetProgress() {
			if p.State == OTARollback {
				t.Errorf("cycle %d: node %s OTA progress is in rollback state", i, mac)
			}
		}
		for _, eventType := range []string{"update_failed", "canary_failed", "update_skipped", "canary_rollback"} {
			if n := len(eventsOfType(f.notifier, eventType)); n != 0 {
				t.Errorf("cycle %d: %s events = %d, want 0", i, eventType, n)
			}
		}
		if got := triggerCounterValue(t, "failure") - failureMark; got != 0 {
			t.Errorf("cycle %d: auto/failure counter delta = %v, want 0", i, got)
		}

		// The cycle's decisions are logged with this cycle's versions: the
		// canary deployment shows the version continuity across cycles
		// (version_before is exactly what the node gained last cycle).
		for _, want := range []string{
			fmt.Sprintf("AUTO-UPDATE cycle started: firmware_version=%s", version),
			fmt.Sprintf("AUTO-UPDATE canary deployment: node=AA:BB:CC:00:00:01 update_type=auto version_before=%s version_after=%s", wantBefore, version),
			fmt.Sprintf("AUTO-UPDATE canary passed: node=AA:BB:CC:00:00:01 update_type=auto version_before=%s version_after=%s", wantBefore, version),
			fmt.Sprintf("AUTO-UPDATE fleet rollout complete: firmware_version=%s nodes_updated=1", version),
		} {
			requireLogSubstring(t, f.logs, want)
		}
		requireNoLogSubstring(t, f.logs, "downgrade prevention")
		requireNoLogSubstring(t, f.logs, "rollback")
	}

	// Ten cycles, ten clean completions, zero failures.
	if got := triggerCounterValue(t, "success") - successBefore; got != cycles {
		t.Errorf("auto/success counter delta = %v, want %d", got, cycles)
	}
	if got := triggerCounterValue(t, "failure") - failureBefore; got != 0 {
		t.Errorf("auto/failure counter delta = %v, want 0", got)
	}
	if got := strings.Count(f.logs.String(), "AUTO-UPDATE fleet rollout complete"); got != cycles {
		t.Errorf("fleet rollout complete logged %d times, want %d", got, cycles)
	}

	// End state: both nodes followed the full 0.2.01 → 0.2.10 lineage — ten
	// strictly increasing steps each, never a step backwards.
	for _, mac := range []string{"AA:BB:CC:00:00:01", "AA:BB:CC:00:00:02"} {
		history := f.sim.historyOf(mac)
		if len(history) != cycles+1 {
			t.Errorf("node %s reported %d versions, want %d (start + %d upgrades)", mac, len(history), cycles+1, cycles)
		}
		for j := 1; j < len(history); j++ {
			if compareFirmwareVersions(history[j], history[j-1]) <= 0 {
				t.Errorf("node %s version history not increasing at step %d: %v", mac, j, history)
				break
			}
		}
		if got := history[len(history)-1]; got != "0.2.10" {
			t.Errorf("node %s ended on %q, want 0.2.10", mac, got)
		}
	}
}
