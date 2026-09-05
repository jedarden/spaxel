package ota

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
)

// mockNodeVersionSource is a test implementation of NodeVersionSource.
type mockNodeVersionSource struct {
	mu       sync.RWMutex
	versions map[string]string
}

func newMockNodeVersionSource() *mockNodeVersionSource {
	return &mockNodeVersionSource{versions: make(map[string]string)}
}

func (m *mockNodeVersionSource) GetAllNodeVersions() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]string, len(m.versions))
	for mac, version := range m.versions {
		out[mac] = version
	}
	return out
}

func (m *mockNodeVersionSource) set(mac, version string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.versions[mac] = version
}

func (m *mockNodeVersionSource) remove(mac string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.versions, mac)
}

// mockDriftStore is a test implementation of SettingStore, exercising the same
// JSON round trip *api.SettingsHandler performs.
type mockDriftStore struct {
	mu     sync.RWMutex
	values map[string]interface{}
	setErr error
}

func newMockDriftStore() *mockDriftStore {
	return &mockDriftStore{values: make(map[string]interface{})}
}

func (m *mockDriftStore) GetSingle(key string) (interface{}, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.values[key]
	return v, ok
}

func (m *mockDriftStore) Set(key string, value interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.setErr != nil {
		return m.setErr
	}
	// Round-trip through JSON so the value has the same shape a real
	// settings read would return.
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var decoded interface{}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return err
	}
	m.values[key] = decoded
	return nil
}

func (m *mockDriftStore) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.values, key)
	return nil
}

func (m *mockDriftStore) has(key string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.values[key]
	return ok
}

// newTestDriftMonitor wires a monitor against mocks and injects the clock.
func newTestDriftMonitor(version string, enabled bool, now time.Time) (*DriftMonitor, *mockNodeVersionSource, *mockDriftStore, *mockEventNotifier) {
	config := DefaultAutoUpdateConfig()
	config.Enabled = enabled

	monitor := NewDriftMonitor(version, func() AutoUpdateConfig { return config })
	source := newMockNodeVersionSource()
	store := newMockDriftStore()
	notifier := newMockEventNotifier()

	monitor.SetNodeVersionSource(source)
	monitor.SetStateStore(store)
	monitor.SetEventNotifier(notifier)

	monitor.now = func() time.Time { return now }

	return monitor, source, store, notifier
}

// driftTestNow is a fixed reference instant, so tests are independent of the
// wall clock.
var driftTestNow = time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)

func TestQuietWindowDuration(t *testing.T) {
	tests := []struct {
		name     string
		start    string
		end      string
		expected time.Duration
	}{
		{name: "shipped default", start: "02:00", end: "05:00", expected: 3 * time.Hour},
		{name: "daytime window", start: "09:30", end: "11:00", expected: 90 * time.Minute},
		{name: "overnight window wraps midnight", start: "22:00", end: "06:00", expected: 8 * time.Hour},
		{name: "same start and end wraps to a full day", start: "02:00", end: "02:00", expected: 24 * time.Hour},
		{name: "no window configured", start: "", end: "", expected: defaultQuietWindow},
		{name: "only start configured", start: "02:00", end: "", expected: defaultQuietWindow},
		{name: "only end configured", start: "", end: "05:00", expected: defaultQuietWindow},
		{name: "unparseable start", start: "2pm", end: "05:00", expected: defaultQuietWindow},
		{name: "unparseable end", start: "02:00", end: "five", expected: defaultQuietWindow},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := quietWindowDuration(AutoUpdateConfig{QuietWindowStart: tt.start, QuietWindowEnd: tt.end})
			if got != tt.expected {
				t.Errorf("quietWindowDuration(%q, %q) = %v, want %v", tt.start, tt.end, got, tt.expected)
			}
		})
	}
}

func TestDriftMonitorFaultAfterQuietWindow(t *testing.T) {
	monitor, source, store, notifier := newTestDriftMonitor("0.2.176", true, driftTestNow)

	threshold := 3 * time.Hour
	source.set("AA:BB:CC:DD:EE:01", "0.2.170")

	// Drift begins at driftTestNow: the monitor has to see it once at that
	// instant for the clock to be stamped there. Each subtest then advances
	// the clock and asserts against the elapsed time since that stamp — the
	// stamp must never be re-taken.
	monitor.now = func() time.Time { return driftTestNow }
	monitor.Evaluate()

	tests := []struct {
		name       string
		elapsed    time.Duration
		wantFault  bool
		wantDrift  float64
		wantEvents int
	}{
		{name: "just drifted", elapsed: time.Minute, wantFault: false, wantDrift: time.Minute.Seconds(), wantEvents: 0},
		{name: "half a window", elapsed: 90 * time.Minute, wantFault: false, wantDrift: (90 * time.Minute).Seconds(), wantEvents: 0},
		{name: "just under one window", elapsed: threshold - time.Minute, wantFault: false, wantDrift: (threshold - time.Minute).Seconds(), wantEvents: 0},
		{name: "one window", elapsed: threshold, wantFault: true, wantDrift: threshold.Seconds(), wantEvents: 1},
		{name: "well past one window", elapsed: 3 * threshold, wantFault: true, wantDrift: (3 * threshold).Seconds(), wantEvents: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monitor.now = func() time.Time { return driftTestNow.Add(tt.elapsed) }
			monitor.Evaluate()

			report := monitor.DriftSnapshot()
			if report.FaultCount != boolCount(tt.wantFault) {
				t.Errorf("FaultCount = %d, want %d", report.FaultCount, boolCount(tt.wantFault))
			}

			if len(report.Nodes) != 1 {
				t.Fatalf("got %d nodes in report, want 1", len(report.Nodes))
			}
			node := report.Nodes[0]
			if node.Fault != tt.wantFault {
				t.Errorf("node fault = %v, want %v", node.Fault, tt.wantFault)
			}
			if node.NodeVersion != "0.2.170" || node.ExpectedVersion != "0.2.176" {
				t.Errorf("node versions = %s / %s, want 0.2.170 / 0.2.176", node.NodeVersion, node.ExpectedVersion)
			}
			if node.FirstSeenMS != driftTestNow.UnixMilli() {
				t.Errorf("first seen = %d, want %d (the moment drift was first observed, not re-stamped)",
					node.FirstSeenMS, driftTestNow.UnixMilli())
			}
			if diff := node.DriftSeconds - tt.wantDrift; diff < -0.001 || diff > 0.001 {
				t.Errorf("drift seconds = %f, want %f", node.DriftSeconds, tt.wantDrift)
			}

			events := notifier.getEvents()
			if len(events) != tt.wantEvents {
				t.Fatalf("got %d timeline events, want %d", len(events), tt.wantEvents)
			}
			if tt.wantEvents > 0 {
				if events[0].eventType != "firmware_drift" {
					t.Errorf("event type = %s, want firmware_drift", events[0].eventType)
				}
				if events[0].mac != "AA:BB:CC:DD:EE:01" {
					t.Errorf("event mac = %s, want the drifted node", events[0].mac)
				}
			}

			// The clock must survive a restart: the store carries the
			// first-seen timestamp under the drift state key.
			if !store.has(driftStateKey) {
				t.Error("expected drift state to be persisted in the settings store")
			}
		})
	}
}

func TestDriftMonitorQuietWhenAutoUpdateDisabled(t *testing.T) {
	// Manual mode: a node on an old version is a deliberate choice, so drift
	// is reported but never escalated to a fault.
	monitor, source, _, notifier := newTestDriftMonitor("0.2.176", false, driftTestNow)
	source.set("AA:BB:CC:DD:EE:01", "0.2.170")

	monitor.now = func() time.Time { return driftTestNow.Add(48 * time.Hour) }
	monitor.Evaluate()

	report := monitor.DriftSnapshot()
	if report.FaultCount != 0 {
		t.Errorf("FaultCount = %d, want 0 while auto-update is disabled", report.FaultCount)
	}
	if report.Enabled {
		t.Error("report should say auto-update is disabled")
	}
	if len(report.Nodes) != 1 || report.Nodes[0].Fault {
		t.Fatalf("expected one drifted, non-faulting node, got %+v", report.Nodes)
	}
	if events := notifier.getEvents(); len(events) != 0 {
		t.Errorf("got %d timeline events, want none in manual mode", len(events))
	}
}

func TestDriftMonitorSkipsFaultOnDevBuild(t *testing.T) {
	// A dev build's version matches no node firmware; faulting against it
	// would mark the whole fleet.
	for _, version := range []string{"dev", ""} {
		t.Run("version="+fmt.Sprintf("%q", version), func(t *testing.T) {
			monitor, source, _, notifier := newTestDriftMonitor(version, true, driftTestNow)
			source.set("AA:BB:CC:DD:EE:01", "0.2.170")

			monitor.now = func() time.Time { return driftTestNow.Add(48 * time.Hour) }
			monitor.Evaluate()

			report := monitor.DriftSnapshot()
			if report.Monitoring {
				t.Error("report should say the version is not monitorable")
			}
			if report.FaultCount != 0 {
				t.Errorf("FaultCount = %d, want 0 for a non-monitorable build", report.FaultCount)
			}
			if events := notifier.getEvents(); len(events) != 0 {
				t.Errorf("got %d timeline events, want none for a non-monitorable build", len(events))
			}
		})
	}
}

func TestDriftMonitorConvergence(t *testing.T) {
	monitor, source, store, notifier := newTestDriftMonitor("0.2.176", true, driftTestNow)
	source.set("AA:BB:CC:DD:EE:01", "0.2.170")

	// Drift is first observed now, then the node is still on the old version
	// four hours later — past the threshold, so a fault is raised.
	monitor.now = func() time.Time { return driftTestNow }
	monitor.Evaluate()
	monitor.now = func() time.Time { return driftTestNow.Add(4 * time.Hour) }
	monitor.Evaluate()

	if got := monitor.DriftSnapshot().FaultCount; got != 1 {
		t.Fatalf("FaultCount = %d, want 1 before convergence", got)
	}
	if events := notifier.getEvents(); len(events) != 1 {
		t.Fatalf("got %d events, want 1 fault event", len(events))
	}

	// The node converges (auto-update reboots it onto the mothership's
	// build): the fault clears, a resolution is recorded, and the persisted
	// state is dropped entirely.
	source.set("AA:BB:CC:DD:EE:01", "0.2.176")
	monitor.Evaluate()

	report := monitor.DriftSnapshot()
	if report.FaultCount != 0 {
		t.Errorf("FaultCount = %d, want 0 after convergence", report.FaultCount)
	}
	if len(report.Nodes) != 1 || report.Nodes[0].DriftSeconds != 0 {
		t.Errorf("expected the converged node reported with no drift, got %+v", report.Nodes)
	}
	if store.has(driftStateKey) {
		t.Error("expected persisted drift state to be cleared once every node converged")
	}

	events := notifier.getEvents()
	if len(events) != 2 {
		t.Fatalf("got %d events, want fault + resolution", len(events))
	}
	if events[1].eventType != "firmware_drift_resolved" {
		t.Errorf("second event type = %s, want firmware_drift_resolved", events[1].eventType)
	}
}

func TestDriftMonitorRestoresPersistedDrift(t *testing.T) {
	monitor, source, store, _ := newTestDriftMonitor("0.2.176", true, driftTestNow)
	source.set("AA:BB:CC:DD:EE:01", "0.2.170")

	// Record drift, then restart: a fresh monitor loads the persisted
	// first-seen timestamps rather than restarting the clock.
	monitor.Evaluate()

	restarted, source2, _, notifier2 := newTestDriftMonitor("0.2.176", true, driftTestNow.Add(2*time.Hour))
	restarted.now = func() time.Time { return driftTestNow.Add(5 * time.Hour) }
	restarted.SetStateStore(store)
	restarted.SetEventNotifier(notifier2)
	source2.set("AA:BB:CC:DD:EE:01", "0.2.170")
	restarted.restoreLocked()
	// Evaluate pulls the node versions in and reconciles them against the
	// restored clock — what Start's run loop does on its first tick.
	restarted.Evaluate()

	report := restarted.DriftSnapshot()
	if report.FaultCount != 1 {
		t.Fatalf("FaultCount = %d, want 1 after restart with persisted drift", report.FaultCount)
	}
	if got := report.Nodes[0].FirstSeenMS; got != driftTestNow.UnixMilli() {
		t.Errorf("first seen = %d, want the persisted %d (restart must not restart the clock)",
			got, driftTestNow.UnixMilli())
	}

	// The restore path is what Start uses; make sure it does not lose state
	// on a monitor whose store is empty.
	fresh, _, _, freshNotifier := newTestDriftMonitor("0.2.176", true, driftTestNow)
	fresh.restoreLocked()
	if events := freshNotifier.getEvents(); len(events) != 0 {
		t.Errorf("got %d events from an empty restore, want 0", len(events))
	}
}

func TestDriftMonitorIgnoresBlankVersionsAndVanishedNodes(t *testing.T) {
	monitor, source, _, _ := newTestDriftMonitor("0.2.176", true, driftTestNow)
	source.set("AA:BB:CC:DD:EE:01", "")
	source.set("AA:BB:CC:DD:EE:02", "0.2.170")

	monitor.Evaluate()
	if got := len(monitor.DriftSnapshot().Nodes); got != 1 {
		t.Fatalf("got %d nodes in report, want the one that reports a version", got)
	}

	// A node deleted from the registry leaves no drift state behind.
	source.remove("AA:BB:CC:DD:EE:02")
	monitor.Evaluate()
	report := monitor.DriftSnapshot()
	if len(report.Nodes) != 0 || report.FaultCount != 0 {
		t.Errorf("expected an empty report after the node vanished, got %+v", report)
	}
}

func TestDriftMonitorWithoutNodeSource(t *testing.T) {
	monitor := NewDriftMonitor("0.2.176", func() AutoUpdateConfig { return DefaultAutoUpdateConfig() })
	monitor.Evaluate() // must not panic
}

func TestDriftSnapshotIsSortedByMAC(t *testing.T) {
	monitor, source, _, _ := newTestDriftMonitor("0.2.176", true, driftTestNow)
	source.set("AA:BB:CC:DD:EE:03", "0.2.170")
	source.set("AA:BB:CC:DD:EE:01", "0.2.169")
	source.set("AA:BB:CC:DD:EE:02", "0.2.176")

	monitor.Evaluate()

	nodes := monitor.DriftSnapshot().Nodes
	if len(nodes) != 3 {
		t.Fatalf("got %d nodes, want 3", len(nodes))
	}
	for i := 1; i < len(nodes); i++ {
		if nodes[i-1].MAC >= nodes[i].MAC {
			t.Errorf("nodes not sorted by MAC: %s before %s", nodes[i-1].MAC, nodes[i].MAC)
		}
	}
	if nodes[1].DriftSeconds != 0 {
		t.Errorf("converged node should report no drift, got %f", nodes[1].DriftSeconds)
	}
}

func TestEnsureAutoUpdateEnabled(t *testing.T) {
	tests := []struct {
		name         string
		prepMarker   bool
		initialValue interface{}
		wantEnabled  bool
		wantSetCall  bool
	}{
		{
			name:         "first boot after the flip enables convergence",
			prepMarker:   false,
			initialValue: false,
			wantEnabled:  true,
			wantSetCall:  true,
		},
		{
			name:         "operator's later choice is not overridden",
			prepMarker:   true,
			initialValue: false,
			wantEnabled:  false,
			wantSetCall:  false,
		},
		{
			name:         "already enabled stays enabled",
			prepMarker:   true,
			initialValue: true,
			wantEnabled:  true,
			wantSetCall:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockDriftStore()
			store.values["auto_update_enabled"] = tt.initialValue
			if tt.prepMarker {
				store.values["auto_update_default_applied"] = "2026-01-01T00:00:00Z"
			}

			if err := EnsureAutoUpdateEnabled(store); err != nil {
				t.Fatalf("EnsureAutoUpdateEnabled returned error: %v", err)
			}

			got, ok := store.GetSingle("auto_update_enabled")
			if !ok || got != tt.wantEnabled {
				t.Errorf("auto_update_enabled = %v (present %v), want %v", got, ok, tt.wantEnabled)
			}
			if store.has("auto_update_default_applied") != tt.wantSetCall && tt.wantSetCall {
				t.Error("expected the apply marker to be recorded")
			}
		})
	}

	t.Run("nil store is a no-op", func(t *testing.T) {
		if err := EnsureAutoUpdateEnabled(nil); err != nil {
			t.Errorf("EnsureAutoUpdateEnabled(nil) = %v, want nil", err)
		}
	})
}

func TestDriftMonitorStartAndStop(t *testing.T) {
	monitor, source, _, notifier := newTestDriftMonitor("0.2.176", true, driftTestNow)
	source.set("AA:BB:CC:DD:EE:01", "0.2.170")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	monitor.Start(ctx)
	monitor.Start(ctx) // second start must be a no-op, not a second loop

	if got := monitor.DriftSnapshot().FaultCount; got != 0 {
		t.Errorf("FaultCount = %d immediately after start, want 0 (drift just observed)", got)
	}

	monitor.Stop()
	monitor.Stop() // second stop must be a no-op

	// Stop waits for the initial Evaluate to finish, so the notifier is
	// settled: drift was observed a moment ago, well inside the window, so
	// no fault event may have fired.
	if events := notifier.getEvents(); len(events) != 0 {
		t.Errorf("got %d timeline events from a just-observed drift, want 0", len(events))
	}
}

func boolCount(b bool) int {
	if b {
		return 1
	}
	return 0
}
