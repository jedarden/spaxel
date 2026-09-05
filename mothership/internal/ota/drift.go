package ota

// Firmware version drift (ADR-009 decision 6). Once automatic convergence is
// on, a node sitting on a firmware version other than the mothership's own
// build is not a state a human is expected to notice by eye: the whole point
// of the auto-updater is that it should not persist. The DriftMonitor tracks
// per-node versions against the mothership's build, and after longer than one
// quiet window it raises a fault — a timeline event at warning severity, a
// field on the drift API the fleet page badges from, and a Prometheus gauge.

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	firmwareDriftFaultGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "node_firmware_drift_fault",
			Help: "1 when a node has run a firmware version other than the mothership's own build for longer than one quiet window (ADR-009 decision 6)",
		},
		[]string{"mac", "node_version", "expected_version"},
	)
	firmwareDriftSecondsGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "node_firmware_drift_seconds",
			Help: "Seconds a node has run a firmware version other than the mothership's own build",
		},
		[]string{"mac"},
	)
)

// devVersion is the placeholder baked into un-versioned builds. It matches no
// node firmware, so faulting against it would mark the whole fleet.
const devVersion = "dev"

// defaultQuietWindow is the length of the shipped default window
// (02:00–05:00). It is the drift-fault threshold when no usable window is
// configured.
const defaultQuietWindow = 3 * time.Hour

// driftStateKey holds the first-seen timestamps of every currently-drifted
// node, so a mothership restart does not restart the drift clock.
const driftStateKey = "firmware_drift_first_seen"

// SettingStore reads and writes individual system settings. Implemented by
// *api.SettingsHandler, which is what cmd/mothership wires in.
type SettingStore interface {
	GetSingle(key string) (interface{}, bool)
	Set(key string, value interface{}) error
	Delete(key string) error
}

// NodeVersionSource reports the firmware version running on every registered
// node. Registered, not just connected: a node that dropped offline on an old
// version is exactly the drift case worth alerting on.
type NodeVersionSource interface {
	GetAllNodeVersions() map[string]string
}

// DriftStatus is one node's firmware drift from the mothership's own build.
type DriftStatus struct {
	MAC             string  `json:"mac"`
	NodeVersion     string  `json:"node_version"`
	ExpectedVersion string  `json:"expected_version"`
	FirstSeenMS     int64   `json:"first_seen_ms,omitempty"`
	DriftSeconds    float64 `json:"drift_seconds"`
	Fault           bool    `json:"fault"`
}

// DriftSnapshotReport is the /api/ota/auto/drift response.
type DriftSnapshotReport struct {
	Enabled          bool          `json:"enabled"`
	Monitoring       bool          `json:"monitoring"`
	ExpectedVersion  string        `json:"expected_version"`
	ThresholdSeconds float64       `json:"threshold_seconds"`
	FaultCount       int           `json:"fault_count"`
	Nodes            []DriftStatus `json:"nodes"`
}

// DriftMonitor compares node firmware versions against the mothership's own
// build and raises a fault when a node stays behind for longer than one quiet
// window.
type DriftMonitor struct {
	mu       sync.RWMutex
	version  string
	config   func() AutoUpdateConfig
	nodes    NodeVersionSource
	store    SettingStore
	notifier EventNotifier
	now      func() time.Time

	// versions is the last set of node versions reported by the source, kept
	// so DriftFor can answer between evaluation ticks.
	versions map[string]string
	// firstSeen maps MAC -> when drift was first observed for it; faulting
	// holds the MACs whose fault has already been raised, so the event fires
	// once per drift episode rather than on every tick.
	firstSeen map[string]time.Time
	faulting  map[string]bool

	running bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewDriftMonitor creates a drift monitor for a mothership running the given
// firmware version. config supplies the live auto-update configuration; the
// quiet window it describes is the fault threshold.
func NewDriftMonitor(version string, config func() AutoUpdateConfig) *DriftMonitor {
	return &DriftMonitor{
		version:   version,
		config:    config,
		now:       time.Now,
		versions:  make(map[string]string),
		firstSeen: make(map[string]time.Time),
		faulting:  make(map[string]bool),
	}
}

// SetNodeVersionSource sets the source of per-node firmware versions.
func (m *DriftMonitor) SetNodeVersionSource(ns NodeVersionSource) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nodes = ns
}

// SetStateStore sets the store used to persist drift first-seen timestamps.
func (m *DriftMonitor) SetStateStore(store SettingStore) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.store = store
}

// SetEventNotifier sets the timeline notifier for drift faults.
func (m *DriftMonitor) SetEventNotifier(n EventNotifier) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifier = n
}

// Start loads persisted drift state and begins evaluating on a one-minute
// cadence, matching the auto-update manager's own loop.
func (m *DriftMonitor) Start(ctx context.Context) {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.restoreLocked()
	ctx, m.cancel = context.WithCancel(ctx)
	m.mu.Unlock()

	m.wg.Add(1)
	go m.run(ctx)

	log.Printf("[INFO] ota: firmware drift monitor started (expected version %s, fault after one quiet window)", m.version)
}

// Stop shuts the monitor down. Nothing is flushed here: first-seen timestamps
// are persisted as they change, not on exit.
func (m *DriftMonitor) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Unlock()

	m.wg.Wait()
	log.Printf("[INFO] ota: firmware drift monitor stopped")
}

func (m *DriftMonitor) run(ctx context.Context) {
	defer m.wg.Done()

	m.Evaluate()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.Evaluate()
		}
	}
}

// monitoredVersion reports whether this build's version is one a node can
// actually be compared against.
func (m *DriftMonitor) monitoredVersion() bool {
	return m.version != "" && m.version != devVersion
}

// Evaluate reconciles the drift state against the current node versions. It
// is what the run loop calls every minute, and is safe to call directly.
func (m *DriftMonitor) Evaluate() {
	m.mu.RLock()
	nodes := m.nodes
	expected := m.version
	monitorable := m.monitoredVersion()
	notifier := m.notifier
	now := m.now()
	m.mu.RUnlock()

	if nodes == nil {
		return
	}

	config := m.config()
	threshold := quietWindowDuration(config)

	versions := nodes.GetAllNodeVersions()

	var resolved []resolvedDrift
	var raised []DriftStatus

	m.mu.Lock()
	persisted := len(m.firstSeen) > 0
	m.versions = versions
	seen := make(map[string]bool, len(versions))

	for mac, version := range versions {
		if mac == "" || version == "" {
			continue
		}
		seen[mac] = true

		if version == expected {
			// Converged: clear the clock, and close out a fault raised
			// against this node.
			if m.faulting[mac] {
				delete(m.faulting, mac)
				resolved = append(resolved, resolvedDrift{mac: mac, version: version})
			}
			delete(m.firstSeen, mac)
			continue
		}

		if !monitorable {
			// Nothing to compare against (dev build): track the mismatch but
			// never fault on it.
			if _, ok := m.firstSeen[mac]; !ok {
				m.firstSeen[mac] = now
			}
			continue
		}

		first, ok := m.firstSeen[mac]
		if !ok {
			first = now
			m.firstSeen[mac] = first
		}

		drifted := now.Sub(first)
		// In manual mode (auto-update disabled) an old version is a
		// deliberate choice, so drift is reported but never faulted.
		fault := config.Enabled && drifted >= threshold

		switch {
		case fault && !m.faulting[mac]:
			m.faulting[mac] = true
			raised = append(raised, DriftStatus{
				MAC:             mac,
				NodeVersion:     version,
				ExpectedVersion: expected,
				DriftSeconds:    drifted.Seconds(),
			})
		case !fault && m.faulting[mac]:
			// Auto-update was switched off, or the threshold widened.
			delete(m.faulting, mac)
		}
	}

	// Nodes that vanished from the source (deleted from the registry) have
	// nothing to be drifted from any more.
	for mac := range m.firstSeen {
		if !seen[mac] {
			delete(m.firstSeen, mac)
			delete(m.faulting, mac)
		}
	}

	if len(m.firstSeen) > 0 || persisted {
		m.persistLocked()
	}
	snapshot := m.snapshotLocked(now, threshold, config.Enabled)
	m.mu.Unlock()

	for _, status := range raised {
		log.Printf("[WARN] ota: FIRMWARE DRIFT FAULT: node=%s version=%s expected=%s drifted_for=%s (threshold %s)",
			status.MAC, status.NodeVersion, status.ExpectedVersion,
			formatDuration(time.Duration(status.DriftSeconds*float64(time.Second))), formatDuration(threshold))
		m.publish(notifier, "firmware_drift", status.MAC,
			fmt.Sprintf("Node %s has run firmware %s instead of %s for %s — longer than one quiet window",
				status.MAC, status.NodeVersion, status.ExpectedVersion,
				formatDuration(time.Duration(status.DriftSeconds*float64(time.Second)))),
			map[string]interface{}{
				"node_version":      status.NodeVersion,
				"expected_version":  status.ExpectedVersion,
				"drifted_seconds":   status.DriftSeconds,
				"threshold_seconds": threshold.Seconds(),
			})
	}

	for _, r := range resolved {
		m.publish(notifier, "firmware_drift_resolved", r.mac,
			fmt.Sprintf("Node %s is back on firmware %s", r.mac, r.version),
			map[string]interface{}{
				"node_version":     r.version,
				"expected_version": expected,
			})
	}

	m.updateMetrics(snapshot)
}

type resolvedDrift struct {
	mac     string
	version string
}

// DriftSnapshot returns the current drift state of every node that reports a
// version, sorted by MAC.
func (m *DriftMonitor) DriftSnapshot() DriftSnapshotReport {
	m.mu.RLock()
	defer m.mu.RUnlock()

	config := m.config()
	threshold := quietWindowDuration(config)

	report := DriftSnapshotReport{
		Enabled:          config.Enabled,
		Monitoring:       m.monitoredVersion(),
		ExpectedVersion:  m.version,
		ThresholdSeconds: threshold.Seconds(),
		Nodes:            m.snapshotLocked(m.now(), threshold, config.Enabled),
	}
	for _, node := range report.Nodes {
		if node.Fault {
			report.FaultCount++
		}
	}
	return report
}

// ExpectedVersion returns the firmware version this mothership runs.
func (m *DriftMonitor) ExpectedVersion() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.version
}

func (m *DriftMonitor) snapshotLocked(now time.Time, threshold time.Duration, enabled bool) []DriftStatus {
	expected := m.version
	monitorable := m.monitoredVersion()

	out := make([]DriftStatus, 0, len(m.versions))
	for mac, version := range m.versions {
		if mac == "" || version == "" {
			continue
		}
		status := DriftStatus{
			MAC:             mac,
			NodeVersion:     version,
			ExpectedVersion: expected,
		}
		if version != expected {
			if first, ok := m.firstSeen[mac]; ok {
				status.FirstSeenMS = first.UnixMilli()
				status.DriftSeconds = now.Sub(first).Seconds()
			}
			status.Fault = monitorable && enabled && status.DriftSeconds >= threshold.Seconds()
		}
		out = append(out, status)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MAC < out[j].MAC })
	return out
}

func (m *DriftMonitor) updateMetrics(snapshot []DriftStatus) {
	faulting := make(map[string]bool, len(snapshot))
	for _, status := range snapshot {
		firmwareDriftSecondsGauge.WithLabelValues(status.MAC).Set(status.DriftSeconds)
		if status.Fault {
			faulting[status.MAC] = true
			firmwareDriftFaultGauge.WithLabelValues(status.MAC, status.NodeVersion, status.ExpectedVersion).Set(1)
		}
	}
	// Retired faults (node converged, node removed, auto-update switched
	// off) must not keep reading 1 forever, so drop this MAC's series. The
	// node_version label changes between episodes, so key the cleanup on the
	// MAC's currently-known series rather than on the label values.
	for _, status := range snapshot {
		if !faulting[status.MAC] {
			firmwareDriftFaultGauge.DeletePartialMatch(prometheus.Labels{"mac": status.MAC})
		}
	}
}

// publish emits a timeline event for a drift transition.
func (m *DriftMonitor) publish(notifier EventNotifier, eventType, mac, message string, metadata map[string]interface{}) {
	if notifier == nil {
		return
	}
	notifier.PublishOTAEvent(eventType, mac, message, metadata)
}

// restoreLocked reloads persisted first-seen timestamps. Called from Start
// with m.mu held.
func (m *DriftMonitor) restoreLocked() {
	if m.store == nil {
		return
	}

	raw, ok := m.store.GetSingle(driftStateKey)
	if !ok || raw == nil {
		return
	}

	// The value round-trips through JSON, so it comes back as
	// map[string]interface{} regardless of what was written.
	var stored map[string]string
	switch typed := raw.(type) {
	case map[string]string:
		stored = typed
	case map[string]interface{}:
		stored = make(map[string]string, len(typed))
		for mac, value := range typed {
			if s, ok := value.(string); ok {
				stored[mac] = s
			}
		}
	default:
		return
	}

	for mac, encoded := range stored {
		first, err := time.Parse(time.RFC3339, encoded)
		if err != nil {
			log.Printf("[WARN] ota: ignoring unreadable drift timestamp for %s: %v", mac, err)
			continue
		}
		m.firstSeen[mac] = first
	}
	if len(m.firstSeen) > 0 {
		log.Printf("[INFO] ota: restored firmware drift state for %d node(s)", len(m.firstSeen))
	}
}

// persistLocked writes the first-seen timestamps back to the settings store,
// or clears the key when nothing is drifted any more. Called with m.mu held.
func (m *DriftMonitor) persistLocked() {
	if m.store == nil {
		return
	}

	if len(m.firstSeen) == 0 {
		if err := m.store.Delete(driftStateKey); err != nil {
			log.Printf("[WARN] ota: failed to clear persisted drift state: %v", err)
		}
		return
	}

	stored := make(map[string]string, len(m.firstSeen))
	for mac, first := range m.firstSeen {
		stored[mac] = first.Format(time.RFC3339)
	}
	if err := m.store.Set(driftStateKey, stored); err != nil {
		log.Printf("[WARN] ota: failed to persist drift state: %v", err)
	}
}

// quietWindowDuration returns the length of the configured quiet window —
// the drift-fault threshold, per ADR-009 decision 6 ("longer than one quiet
// window"). Falls back to the shipped default window length when no window is
// configured or the values do not parse.
func quietWindowDuration(config AutoUpdateConfig) time.Duration {
	if config.QuietWindowStart == "" || config.QuietWindowEnd == "" {
		return defaultQuietWindow
	}

	start, err := time.Parse("15:04", config.QuietWindowStart)
	if err != nil {
		return defaultQuietWindow
	}
	end, err := time.Parse("15:04", config.QuietWindowEnd)
	if err != nil {
		return defaultQuietWindow
	}

	window := end.Sub(start)
	if window <= 0 {
		// An overnight window (e.g. 22:00–06:00) crosses midnight.
		window += 24 * time.Hour
	}
	return window
}

// EnsureAutoUpdateEnabled turns automatic firmware convergence on, once, on
// the first boot after the ADR-009 flip. Deployments that predate the flip
// carry auto_update_enabled=false in their settings row, which a new default
// cannot reach; the marker records that the flip was applied, so an operator
// who disables it afterwards keeps their choice.
func EnsureAutoUpdateEnabled(store SettingStore) error {
	if store == nil {
		return nil
	}

	const marker = "auto_update_default_applied"
	if _, ok := store.GetSingle(marker); ok {
		return nil
	}

	if err := store.Set("auto_update_enabled", true); err != nil {
		return fmt.Errorf("enable auto-update: %w", err)
	}
	if err := store.Set(marker, time.Now().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("record auto-update enable marker: %w", err)
	}

	log.Printf("[INFO] ota: ADR-009 automatic firmware convergence enabled (first boot after flip); disable via Settings to opt out")
	return nil
}
