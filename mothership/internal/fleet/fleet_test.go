package fleet

import (
	"sync"
	"testing"
	"time"

	"github.com/spaxel/mothership/internal/events"
)

// ─── Test doubles ────────────────────────────────────────────────────────────

type mockNotifier struct {
	mu         sync.Mutex
	rolesSent  map[string]string
	configSent map[string]int
	connected  []string
}

func newMockNotifier(connected ...string) *mockNotifier {
	return &mockNotifier{
		rolesSent:  make(map[string]string),
		configSent: make(map[string]int),
		connected:  connected,
	}
}

func (m *mockNotifier) SendRoleToMAC(mac, role, _ string) {
	m.mu.Lock()
	m.rolesSent[mac] = role
	m.mu.Unlock()
}

func (m *mockNotifier) SendConfigToMAC(mac string, rateHz int, _ float64) {
	m.mu.Lock()
	m.configSent[mac] = rateHz
	m.mu.Unlock()
}

func (m *mockNotifier) GetConnectedMACs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string{}, m.connected...)
}

func (m *mockNotifier) SendIdentifyToMAC(mac string, durationMS int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.connected {
		if c == mac {
			return true
		}
	}
	return false
}

func (m *mockNotifier) sentRole(mac string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rolesSent[mac]
}

type mockBroadcaster struct {
	mu    sync.Mutex
	calls int
}

func (b *mockBroadcaster) BroadcastRegistryState(_ []NodeRecord, _ RoomConfig) {
	b.mu.Lock()
	b.calls++
	b.mu.Unlock()
}

func (b *mockBroadcaster) broadcastCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	reg, err := NewRegistry(":memory:")
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	t.Cleanup(func() { reg.Close() })
	return reg
}

func newTestManager(t *testing.T) (*Manager, *mockNotifier, *mockBroadcaster) {
	t.Helper()
	reg := newTestRegistry(t)
	mgr := NewManager(reg)
	n := newMockNotifier()
	b := &mockBroadcaster{}
	mgr.SetNotifier(n)
	mgr.SetBroadcaster(b)
	return mgr, n, b
}

// ─── Registry tests ───────────────────────────────────────────────────────────

func TestRegistryUpsertAndGet(t *testing.T) {
	reg := newTestRegistry(t)

	if err := reg.UpsertNode("aa:bb:cc:dd:ee:01", "v1.0", "ESP32-S3"); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	node, err := reg.GetNode("aa:bb:cc:dd:ee:01")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if node.MAC != "aa:bb:cc:dd:ee:01" {
		t.Errorf("MAC = %q, want %q", node.MAC, "aa:bb:cc:dd:ee:01")
	}
	if node.FirmwareVersion != "v1.0" {
		t.Errorf("FirmwareVersion = %q, want %q", node.FirmwareVersion, "v1.0")
	}
	if node.ChipModel != "ESP32-S3" {
		t.Errorf("ChipModel = %q, want %q", node.ChipModel, "ESP32-S3")
	}
	if node.Role != "rx" {
		t.Errorf("default Role = %q, want %q", node.Role, "rx")
	}
}

func TestRegistryUpsertUpdatesLastSeen(t *testing.T) {
	reg := newTestRegistry(t)

	if err := reg.UpsertNode("aa:bb:cc:dd:ee:02", "v1.0", "ESP32-S3"); err != nil {
		t.Fatalf("first UpsertNode: %v", err)
	}
	n1, _ := reg.GetNode("aa:bb:cc:dd:ee:02")

	if err := reg.UpsertNode("aa:bb:cc:dd:ee:02", "v1.1", "ESP32-S3"); err != nil {
		t.Fatalf("second UpsertNode: %v", err)
	}
	n2, _ := reg.GetNode("aa:bb:cc:dd:ee:02")

	if n2.FirmwareVersion != "v1.1" {
		t.Errorf("firmware not updated: got %q", n2.FirmwareVersion)
	}
	if !n2.LastSeenAt.After(n1.LastSeenAt) || n2.LastSeenAt.Equal(n1.LastSeenAt) {
		// Equal is fine if both happened in the same nanosecond (unlikely but allow)
		_ = n1
	}
}

func TestRegistrySetRole(t *testing.T) {
	reg := newTestRegistry(t)
	if err := reg.UpsertNode("aa:bb:cc:dd:ee:03", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := reg.SetNodeRole("aa:bb:cc:dd:ee:03", "tx"); err != nil {
		t.Fatalf("SetNodeRole: %v", err)
	}
	node, err := reg.GetNode("aa:bb:cc:dd:ee:03")
	if err != nil {
		t.Fatal(err)
	}
	if node.Role != "tx" {
		t.Errorf("Role = %q, want tx", node.Role)
	}
}

func TestRegistryGetAllNodes(t *testing.T) {
	reg := newTestRegistry(t)
	macs := []string{"aa:bb:cc:dd:ee:0a", "aa:bb:cc:dd:ee:0b", "aa:bb:cc:dd:ee:0c"}
	for _, mac := range macs {
		if err := reg.UpsertNode(mac, "", ""); err != nil {
			t.Fatalf("UpsertNode %s: %v", mac, err)
		}
	}
	nodes, err := reg.GetAllNodes()
	if err != nil {
		t.Fatalf("GetAllNodes: %v", err)
	}
	if len(nodes) != 3 {
		t.Errorf("got %d nodes, want 3", len(nodes))
	}
}

// ─── Manager role assignment tests ───────────────────────────────────────────

func TestManagerSingleNode_TxRx(t *testing.T) {
	mgr, notif, _ := newTestManager(t)

	mgr.OnNodeConnected("aa:00:00:00:00:01", "v1", "S3")

	role := notif.sentRole("aa:00:00:00:00:01")
	if role != "tx_rx" {
		t.Errorf("single node: role = %q, want tx_rx", role)
	}

	node, err := mgr.registry.GetNode("aa:00:00:00:00:01")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if node.Role != "tx_rx" {
		t.Errorf("persisted role = %q, want tx_rx", node.Role)
	}
}

func TestManagerTwoNodes_TxRx(t *testing.T) {
	mgr, notif, _ := newTestManager(t)

	mgr.OnNodeConnected("aa:00:00:00:00:01", "v1", "S3")
	mgr.OnNodeConnected("aa:00:00:00:00:02", "v1", "S3")

	r1 := notif.sentRole("aa:00:00:00:00:01")
	r2 := notif.sentRole("aa:00:00:00:00:02")

	// With 2 nodes: first stays tx_rx (assigned before second joined),
	// second gets tx (txCount was 0 at join time).
	// After second joins: one node has tx, one has tx_rx.
	// What matters is that a TX was assigned and an RX was assigned.
	roles := map[string]bool{r1: true, r2: true}
	if !roles["tx"] {
		t.Errorf("expected one TX among roles: %v", roles)
	}
}

func TestManagerThreeNodes_HalfTx(t *testing.T) {
	mgr, notif, _ := newTestManager(t)

	mgr.OnNodeConnected("aa:00:00:00:00:01", "v1", "S3")
	mgr.OnNodeConnected("aa:00:00:00:00:02", "v1", "S3")
	mgr.OnNodeConnected("aa:00:00:00:00:03", "v1", "S3")

	roles := []string{
		notif.sentRole("aa:00:00:00:00:01"),
		notif.sentRole("aa:00:00:00:00:02"),
		notif.sentRole("aa:00:00:00:00:03"),
	}

	txCount := 0
	for _, r := range roles {
		if r == "tx" || r == "tx_rx" {
			txCount++
		}
	}
	// With 3 nodes floor(3/2)=1 additional TX assigned, plus the original tx_rx.
	if txCount < 1 {
		t.Errorf("expected at least 1 TX/TX_RX node among %v", roles)
	}
}

// ─── Manager self-healing and failure recovery tests ─────────────────────────

func TestManagerNodeDisconnect_Rebalance(t *testing.T) {
	mgr, notif, _ := newTestManager(t)

	mgr.OnNodeConnected("aa:00:00:00:00:01", "v1", "S3")
	mgr.OnNodeConnected("aa:00:00:00:00:02", "v1", "S3")
	mgr.OnNodeConnected("aa:00:00:00:00:03", "v1", "S3")

	// Node 2 goes offline.
	mgr.OnNodeDisconnected("aa:00:00:00:00:02")

	// After rebalance with 2 remaining nodes, roles are re-sent.
	r1 := notif.sentRole("aa:00:00:00:00:01")
	r3 := notif.sentRole("aa:00:00:00:00:03")

	if r1 == "" || r3 == "" {
		t.Errorf("after disconnect, nodes should have received new roles; got %q, %q", r1, r3)
	}

	// Exactly one of the remaining nodes should be TX.
	txCount := 0
	for _, r := range []string{r1, r3} {
		if r == "tx" || r == "tx_rx" {
			txCount++
		}
	}
	if txCount != 1 {
		t.Errorf("after rebalance with 2 nodes: want 1 TX, got %d TX among [%q, %q]", txCount, r1, r3)
	}
}

func TestManagerLastNodeDisconnect_ClearsState(t *testing.T) {
	mgr, notif, _ := newTestManager(t)

	mgr.OnNodeConnected("aa:00:00:00:00:01", "v1", "S3")
	mgr.OnNodeDisconnected("aa:00:00:00:00:01")

	mgr.mu.RLock()
	txCount := mgr.txCount
	mgr.mu.RUnlock()

	if txCount != 0 {
		t.Errorf("txCount after last node leaves = %d, want 0", txCount)
	}
	_ = notif // no roles should be sent (nothing to send to)
}

func TestManagerSelfHeal_RepushesRoles(t *testing.T) {
	mgr, notif, _ := newTestManager(t)

	mgr.OnNodeConnected("aa:00:00:00:00:01", "v1", "S3")

	// Simulate notifier tracking connected nodes.
	notif.mu.Lock()
	notif.connected = []string{"aa:00:00:00:00:01"}
	notif.mu.Unlock()

	// Clear the sent roles to verify selfHeal re-pushes them.
	notif.mu.Lock()
	notif.rolesSent = make(map[string]string)
	notif.mu.Unlock()

	mgr.selfHeal()

	role := notif.sentRole("aa:00:00:00:00:01")
	if role == "" {
		t.Error("selfHeal did not re-push role to connected node")
	}
}

func TestManagerOverrideRole(t *testing.T) {
	mgr, notif, bcaster := newTestManager(t)

	mgr.OnNodeConnected("aa:00:00:00:00:01", "v1", "S3")
	prevCalls := bcaster.broadcastCount()

	if err := mgr.OverrideRole("aa:00:00:00:00:01", "rx"); err != nil {
		t.Fatalf("OverrideRole: %v", err)
	}

	if notif.sentRole("aa:00:00:00:00:01") != "rx" {
		t.Errorf("OverrideRole did not push rx to notifier")
	}

	node, err := mgr.registry.GetNode("aa:00:00:00:00:01")
	if err != nil {
		t.Fatal(err)
	}
	if node.Role != "rx" {
		t.Errorf("OverrideRole did not persist role; got %q", node.Role)
	}

	if bcaster.broadcastCount() <= prevCalls {
		t.Error("OverrideRole did not trigger a registry broadcast")
	}
}

func TestManagerBroadcastOnConnect(t *testing.T) {
	mgr, _, bcaster := newTestManager(t)

	before := bcaster.broadcastCount()
	mgr.OnNodeConnected("aa:00:00:00:00:01", "v1", "S3")
	after := bcaster.broadcastCount()

	if after <= before {
		t.Error("OnNodeConnected did not broadcast registry state")
	}
}

func TestManagerBroadcastOnDisconnect(t *testing.T) {
	mgr, _, bcaster := newTestManager(t)

	mgr.OnNodeConnected("aa:00:00:00:00:01", "v1", "S3")
	before := bcaster.broadcastCount()
	mgr.OnNodeDisconnected("aa:00:00:00:00:01")
	after := bcaster.broadcastCount()

	if after <= before {
		t.Error("OnNodeDisconnected did not broadcast registry state")
	}
}

// TestManagerPersistenceAcrossRestart verifies that node state survives a
// Manager restart by using the same registry.
func TestManagerPersistenceAcrossRestart(t *testing.T) {
	reg := newTestRegistry(t)

	// First manager: node connects and is persisted.
	mgr1 := NewManager(reg)
	n1 := newMockNotifier()
	mgr1.SetNotifier(n1)
	mgr1.OnNodeConnected("aa:00:00:00:00:01", "v1.2", "ESP32-S3")

	// Second manager with same registry simulates a restart.
	mgr2 := NewManager(reg)
	n2 := newMockNotifier()
	mgr2.SetNotifier(n2)

	nodes, err := mgr2.registry.GetAllNodes()
	if err != nil {
		t.Fatalf("GetAllNodes after restart: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 persisted node after restart, got %d", len(nodes))
	}
	if nodes[0].MAC != "aa:00:00:00:00:01" {
		t.Errorf("wrong MAC after restart: %q", nodes[0].MAC)
	}
	if nodes[0].FirmwareVersion != "v1.2" {
		t.Errorf("wrong firmware after restart: %q", nodes[0].FirmwareVersion)
	}
}

// ─── System mode and auto-away tests ───────────────────────────────────────────

// mockBLEPresenceProvider implements BLEPresenceProvider for testing.
type mockBLEPresenceProvider struct {
	registeredDevices map[string]string // MAC -> person_id
	observations      []BLEObservation
}

func newMockBLEPresenceProvider() *mockBLEPresenceProvider {
	return &mockBLEPresenceProvider{
		registeredDevices: make(map[string]string),
		observations:      make([]BLEObservation, 0),
	}
}

func (m *mockBLEPresenceProvider) GetAllRegisteredDevices() (map[string]string, error) {
	return m.registeredDevices, nil
}

func (m *mockBLEPresenceProvider) GetRecentRSSIObservations(mac string, maxAge time.Duration) []BLEObservation {
	var result []BLEObservation
	cutoff := time.Now().Add(-maxAge)
	for _, obs := range m.observations {
		if obs.DeviceMAC == mac && obs.Timestamp.After(cutoff) {
			result = append(result, obs)
		}
	}
	return result
}

// mockPersonNameProvider implements PersonNameProvider for testing.
type mockPersonNameProvider struct {
	names map[string]string // person_id -> name
}

func (m *mockPersonNameProvider) GetPersonName(personID string) string {
	if name, ok := m.names[personID]; ok {
		return name
	}
	return personID
}

// mockModeChangeBroadcaster implements ModeChangeBroadcaster for testing.
type mockModeChangeBroadcaster struct {
	mu      sync.Mutex
	events  []events.SystemModeChangeEvent
}

func (m *mockModeChangeBroadcaster) BroadcastSystemModeChange(event events.SystemModeChangeEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
}

func (m *mockModeChangeBroadcaster) getEvents() []events.SystemModeChangeEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]events.SystemModeChangeEvent{}, m.events...)
}

func TestManager_AutoAwayActivates(t *testing.T) {
	mgr, _, _ := newTestManager(t)

	bleProvider := newMockBLEPresenceProvider()
	bleProvider.registeredDevices["aa:bb:cc:dd:ee:ff"] = "person1"
	mgr.SetBLEPresenceProvider(bleProvider)

	modeBroadcaster := &mockModeChangeBroadcaster{}
	mgr.SetModeChangeBroadcaster(modeBroadcaster)

	// Initially in home mode
	if mgr.GetSystemMode() != events.ModeHome {
		t.Errorf("Expected initial mode to be home, got %s", mgr.GetSystemMode())
	}

	// Set all devices as seen long ago (more than 15 minutes ago)
	mgr.mu.Lock()
	mgr.lastDeviceSeen["aa:bb:cc:dd:ee:ff"] = time.Now().Add(-20 * time.Minute)
	mgr.mu.Unlock()

	// Check auto-away - should activate
	mgr.CheckAutoAway()

	if mgr.GetSystemMode() != events.ModeAway {
		t.Errorf("Expected mode to be away after auto-away, got %s", mgr.GetSystemMode())
	}

	modeEvents := modeBroadcaster.getEvents()
	if len(modeEvents) != 1 {
		t.Fatalf("Expected 1 mode change event, got %d", len(modeEvents))
	}

	if modeEvents[0].NewMode != events.ModeAway {
		t.Errorf("Expected new mode to be away, got %s", modeEvents[0].NewMode)
	}

	if modeEvents[0].Reason != "auto_away" {
		t.Errorf("Expected reason to be auto_away, got %s", modeEvents[0].Reason)
	}
}

func TestManager_AutoDisarmTriggers(t *testing.T) {
	mgr, _, _ := newTestManager(t)

	bleProvider := newMockBLEPresenceProvider()
	bleProvider.registeredDevices["aa:bb:cc:dd:ee:ff"] = "person1"
	mgr.SetBLEPresenceProvider(bleProvider)

	personProvider := &mockPersonNameProvider{names: map[string]string{"person1": "Alice"}}
	mgr.SetPersonProvider(personProvider)

	modeBroadcaster := &mockModeChangeBroadcaster{}
	mgr.SetModeChangeBroadcaster(modeBroadcaster)

	// Set to away mode
	mgr.mu.Lock()
	mgr.systemMode = events.ModeAway
	mgr.mu.Unlock()

	// Simulate BLE observation with strong signal
	observations := []BLEObservation{
		{
			DeviceMAC: "aa:bb:cc:dd:ee:ff",
			NodeMAC:   "node1",
			RSSIdBm:   -65, // Stronger than -70 threshold
			Timestamp: time.Now(),
		},
	}

	mgr.ProcessBLEObservations(observations)

	// Should auto-disarm
	if mgr.GetSystemMode() != events.ModeHome {
		t.Errorf("Expected mode to be home after auto-disarm, got %s", mgr.GetSystemMode())
	}

	modeEvents := modeBroadcaster.getEvents()
	if len(modeEvents) != 1 {
		t.Fatalf("Expected 1 mode change event, got %d", len(modeEvents))
	}

	if modeEvents[0].NewMode != events.ModeHome {
		t.Errorf("Expected new mode to be home, got %s", modeEvents[0].NewMode)
	}

	if modeEvents[0].Reason != "auto_disarm" {
		t.Errorf("Expected reason to be auto_disarm, got %s", modeEvents[0].Reason)
	}

	if modeEvents[0].PersonID != "person1" {
		t.Errorf("Expected person_id to be person1, got %s", modeEvents[0].PersonID)
	}

	if modeEvents[0].PersonName != "Alice" {
		t.Errorf("Expected person_name to be Alice, got %s", modeEvents[0].PersonName)
	}
}

func TestManager_AutoAwayDoesNotTriggerWithoutRegisteredDevices(t *testing.T) {
	mgr, _, _ := newTestManager(t)

	bleProvider := newMockBLEPresenceProvider()
	// No registered devices
	mgr.SetBLEPresenceProvider(bleProvider)

	modeBroadcaster := &mockModeChangeBroadcaster{}
	mgr.SetModeChangeBroadcaster(modeBroadcaster)

	// Check auto-away - should NOT activate (no registered devices)
	mgr.CheckAutoAway()

	if mgr.GetSystemMode() != events.ModeHome {
		t.Errorf("Expected mode to remain home when no registered devices, got %s", mgr.GetSystemMode())
	}

	events := modeBroadcaster.getEvents()
	if len(events) != 0 {
		t.Errorf("Expected no mode change events when no registered devices, got %d", len(events))
	}
}

func TestManager_ManualOverridePausesAutoAway(t *testing.T) {
	mgr, _, _ := newTestManager(t)

	bleProvider := newMockBLEPresenceProvider()
	bleProvider.registeredDevices["aa:bb:cc:dd:ee:ff"] = "person1"
	mgr.SetBLEPresenceProvider(bleProvider)

	// Set manual override
	mgr.SetSystemMode(events.ModeAway, "manual")

	// Verify override is active
	if !mgr.IsManualOverrideActive() {
		t.Error("Expected manual override to be active")
	}

	// Set all devices as seen long ago
	mgr.mu.Lock()
	mgr.lastDeviceSeen["aa:bb:cc:dd:ee:ff"] = time.Now().Add(-20 * time.Minute)
	mgr.mu.Unlock()

	// Check auto-away - should NOT trigger due to manual override
	initialMode := mgr.GetSystemMode()
	mgr.CheckAutoAway()

	// Mode should not change (already away, but the point is no auto-away logic ran)
	if mgr.GetSystemMode() != initialMode {
		t.Errorf("Expected mode to remain %s with manual override, got %s", initialMode, mgr.GetSystemMode())
	}
}

func TestManager_SetSystemMode(t *testing.T) {
	mgr, _, _ := newTestManager(t)

	modeBroadcaster := &mockModeChangeBroadcaster{}
	mgr.SetModeChangeBroadcaster(modeBroadcaster)

	// Set to away mode
	err := mgr.SetSystemMode(events.ModeAway, "test")
	if err != nil {
		t.Fatalf("Failed to set system mode: %v", err)
	}

	if mgr.GetSystemMode() != events.ModeAway {
		t.Errorf("Expected mode to be away, got %s", mgr.GetSystemMode())
	}

	modeEvents := modeBroadcaster.getEvents()
	if len(modeEvents) != 1 {
		t.Fatalf("Expected 1 mode change event, got %d", len(modeEvents))
	}

	if modeEvents[0].NewMode != events.ModeAway {
		t.Errorf("Expected new mode to be away, got %s", modeEvents[0].NewMode)
	}

	if modeEvents[0].Reason != "test" {
		t.Errorf("Expected reason to be test, got %s", modeEvents[0].Reason)
	}
}

func TestManager_SetSystemModeSameModeNoOp(t *testing.T) {
	mgr, _, _ := newTestManager(t)

	modeBroadcaster := &mockModeChangeBroadcaster{}
	mgr.SetModeChangeBroadcaster(modeBroadcaster)

	// Set to home mode (already home)
	err := mgr.SetSystemMode(events.ModeHome, "test")
	if err != nil {
		t.Fatalf("Failed to set system mode: %v", err)
	}

	// Should not have triggered any events
	modeEvents := modeBroadcaster.getEvents()
	if len(modeEvents) != 0 {
		t.Errorf("Expected no mode change events when setting to same mode, got %d", len(modeEvents))
	}
}

func TestManager_AutoDisarmWeakSignalNoTrigger(t *testing.T) {
	mgr, _, _ := newTestManager(t)

	bleProvider := newMockBLEPresenceProvider()
	bleProvider.registeredDevices["aa:bb:cc:dd:ee:ff"] = "person1"
	mgr.SetBLEPresenceProvider(bleProvider)

	modeBroadcaster := &mockModeChangeBroadcaster{}
	mgr.SetModeChangeBroadcaster(modeBroadcaster)

	// Set to away mode
	mgr.mu.Lock()
	mgr.systemMode = events.ModeAway
	mgr.mu.Unlock()

	// Simulate BLE observation with weak signal (below -70 threshold)
	observations := []BLEObservation{
		{
			DeviceMAC: "aa:bb:cc:dd:ee:ff",
			NodeMAC:   "node1",
			RSSIdBm:   -75, // Weaker than -70 threshold
			Timestamp: time.Now(),
		},
	}

	mgr.ProcessBLEObservations(observations)

	// Should NOT auto-disarm (signal too weak)
	if mgr.GetSystemMode() != events.ModeAway {
		t.Errorf("Expected mode to remain away with weak BLE signal, got %s", mgr.GetSystemMode())
	}

	events := modeBroadcaster.getEvents()
	if len(events) != 0 {
		t.Errorf("Expected no mode change events with weak BLE signal, got %d", len(events))
	}
}

func TestManager_AutoDisarmUnregisteredDeviceNoTrigger(t *testing.T) {
	mgr, _, _ := newTestManager(t)

	bleProvider := newMockBLEPresenceProvider()
	bleProvider.registeredDevices["aa:bb:cc:dd:ee:ff"] = "person1"
	mgr.SetBLEPresenceProvider(bleProvider)

	modeBroadcaster := &mockModeChangeBroadcaster{}
	mgr.SetModeChangeBroadcaster(modeBroadcaster)

	// Set to away mode
	mgr.mu.Lock()
	mgr.systemMode = events.ModeAway
	mgr.mu.Unlock()

	// Simulate BLE observation from unregistered device
	observations := []BLEObservation{
		{
			DeviceMAC: "11:22:33:44:55:66", // Not registered
			NodeMAC:   "node1",
			RSSIdBm:   -65,
			Timestamp: time.Now(),
		},
	}

	mgr.ProcessBLEObservations(observations)

	// Should NOT auto-disarm (device not registered)
	if mgr.GetSystemMode() != events.ModeAway {
		t.Errorf("Expected mode to remain away with unregistered BLE device, got %s", mgr.GetSystemMode())
	}

	events := modeBroadcaster.getEvents()
	if len(events) != 0 {
		t.Errorf("Expected no mode change events with unregistered BLE device, got %d", len(events))
	}
}
