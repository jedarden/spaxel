package fleet

import (
	"context"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/spaxel/mothership/internal/events"
)

// NodeStateNotifier is called when the manager sends a role or config to a node.
type NodeStateNotifier interface {
	// SendRoleToMAC sends a role assignment message to a connected node.
	SendRoleToMAC(mac, role, passiveBSSID string)
	// SendConfigToMAC sends a rate config to a connected node.
	// txSlotUS is the TX slot offset in microseconds (0 means no stagger).
	SendConfigToMAC(mac string, rateHz int, txSlotUS int, varianceThreshold float64)
	// SendIdentifyToMAC sends an LED blink command to a connected node.
	// Returns false if the node is not connected.
	SendIdentifyToMAC(mac string, durationMS int) bool
	// GetConnectedMACs returns the MACs of currently-connected nodes.
	GetConnectedMACs() []string
}

// RegistryBroadcaster is called when fleet state changes that the dashboard should see.
type RegistryBroadcaster interface {
	BroadcastRegistryState(nodes []NodeRecord, room RoomConfig)
}

// ModeChangeBroadcaster is called when system mode changes.
type ModeChangeBroadcaster interface {
	BroadcastSystemModeChange(event events.SystemModeChangeEvent)
}

// BLEPresenceProvider provides BLE device presence information for auto-away detection.
type BLEPresenceProvider interface {
	// GetAllRegisteredDevices returns all registered BLE devices (MAC -> person_id)
	GetAllRegisteredDevices() (map[string]string, error)
	// GetRecentRSSIObservations returns recent RSSI observations for a device
	GetRecentRSSIObservations(mac string, maxAge time.Duration) []BLEObservation
}

// PersonNameProvider provides person name lookups for mode change events.
type PersonNameProvider interface {
	GetPersonName(personID string) string
}

// BLEObservation represents a BLE RSSI observation with device info.
type BLEObservation struct {
	DeviceMAC string // The BLE device MAC address
	NodeMAC   string // The node that observed this device
	RSSIdBm   int
	Timestamp time.Time
}

// AutoAwayConfig holds configuration for auto-away detection.
type AutoAwayConfig struct {
	Enabled             bool          `json:"enabled"`
	AbsenceDuration     time.Duration `json:"absence_duration"`      // Default: 15 minutes
	AutoDisarmRSSI      int           `json:"auto_disarm_rssi"`      // Default: -70 dBm
	ManualOverridePause time.Duration `json:"manual_override_pause"` // Default: 30 minutes
}

// DefaultAutoAwayConfig returns default auto-away configuration.
func DefaultAutoAwayConfig() AutoAwayConfig {
	return AutoAwayConfig{
		Enabled:             true,
		AbsenceDuration:     15 * time.Minute,
		AutoDisarmRSSI:      -70,
		ManualOverridePause: 30 * time.Minute,
	}
}

// Manager handles fleet-level operations: role assignment, stagger scheduling, and self-healing.
type Manager struct {
	mu       sync.RWMutex
	registry *Registry
	notifier NodeStateNotifier
	bcaster  RegistryBroadcaster

	// online tracks which MACs are currently connected.
	online map[string]struct{}

	// roleIndex tracks which nodes have been assigned TX.
	txNodes []string

	// stagger scheduling: how many TX nodes have been assigned.
	txCount int

	// healTick is how often we check for stale/missing assignments.
	healTick time.Duration

	// System mode management
	systemMode            events.SystemMode
	modeChangeBroadcaster ModeChangeBroadcaster
	autoAwayConfig        AutoAwayConfig
	blePresenceProvider   BLEPresenceProvider
	personProvider        PersonNameProvider
	manualOverrideUntil   time.Time
	lastDeviceSeen        map[string]time.Time // MAC -> last seen time
	modeCheckInterval     time.Duration

	// Callback for mode changes
	onModeChange func(events.SystemModeChangeEvent)

	// Collision detection and adaptive re-stagger
	collisionDetector  *CollisionDetector
	collisionCheckTick time.Duration
}

// NewManager creates a fleet manager backed by registry.
func NewManager(reg *Registry) *Manager {
	m := &Manager{
		registry:           reg,
		online:             make(map[string]struct{}),
		healTick:           60 * time.Second,
		systemMode:         events.ModeHome,
		autoAwayConfig:     DefaultAutoAwayConfig(),
		lastDeviceSeen:     make(map[string]time.Time),
		modeCheckInterval:  30 * time.Second,
		collisionDetector:  NewCollisionDetector(),
		collisionCheckTick: 10 * time.Second,
	}
	// Set up re-stagger callback
	m.collisionDetector.SetRestaggerCallback(func() {
		m.AdaptiveRestagger()
	})
	return m
}

// SetNotifier sets the ingestion server callback.
func (m *Manager) SetNotifier(n NodeStateNotifier) {
	m.mu.Lock()
	m.notifier = n
	m.mu.Unlock()
}

// SetBroadcaster sets the dashboard broadcaster.
func (m *Manager) SetBroadcaster(b RegistryBroadcaster) {
	m.mu.Lock()
	m.bcaster = b
	m.mu.Unlock()
}

// OnNodeConnected is called when a node completes its hello handshake.
// It persists the node, assigns a role, and broadcasts updated state.
func (m *Manager) OnNodeConnected(mac, firmware, chip string) {
	if err := m.registry.UpsertNode(mac, firmware, chip); err != nil {
		log.Printf("[WARN] fleet: upsert node %s: %v", mac, err)
	}

	m.mu.Lock()
	m.online[mac] = struct{}{}
	m.mu.Unlock()

	role := m.assignRole(mac)
	if err := m.registry.SetNodeRole(mac, role); err != nil {
		log.Printf("[WARN] fleet: set role %s: %v", mac, err)
	}

	m.applyRoleAndConfig(mac, role)
	m.broadcastRegistry()
	log.Printf("[INFO] fleet: node %s joined as %s", mac, role)
}

// OnNodeDisconnected is called when a node disconnects.
func (m *Manager) OnNodeDisconnected(mac string) {
	m.mu.Lock()
	delete(m.online, mac)
	m.mu.Unlock()

	// If the lost node was a TX node, reassign TX roles.
	m.rebalanceRoles()
	m.broadcastRegistry()
	log.Printf("[INFO] fleet: node %s left, rebalanced roles", mac)
}

// Run starts the periodic self-healing loop.
func (m *Manager) Run(ctx context.Context) {
	ticker := time.NewTicker(m.healTick)
	defer ticker.Stop()

	collisionTicker := time.NewTicker(m.collisionCheckTick)
	defer collisionTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.selfHeal()
		case <-collisionTicker.C:
			m.checkCollisions()
		}
	}
}

// GetRegistry returns the underlying registry for REST API use.
func (m *Manager) GetRegistry() *Registry {
	return m.registry
}

// BroadcastRegistry triggers a one-off registry state broadcast.
func (m *Manager) BroadcastRegistry() {
	m.broadcastRegistry()
}

// OverrideRole manually sets a node's role, pushing the update to the node if online,
// and broadcasting the updated registry state.
func (m *Manager) OverrideRole(mac, role string) error {
	if err := m.registry.SetNodeRole(mac, role); err != nil {
		return err
	}
	m.mu.RLock()
	notifier := m.notifier
	m.mu.RUnlock()
	if notifier != nil {
		notifier.SendRoleToMAC(mac, role, "")
	}
	m.broadcastRegistry()
	return nil
}

// IdentifyNode sends an LED blink command to a node for identification.
// Returns true if the command was sent successfully, false if the node is not connected.
func (m *Manager) IdentifyNode(mac string, durationMS int) bool {
	m.mu.RLock()
	notifier := m.notifier
	m.mu.RUnlock()
	if notifier == nil {
		return false
	}
	return notifier.SendIdentifyToMAC(mac, durationMS)
}

// ─── Role Assignment ────────────────────────────────────────────────────────

// assignRole determines the role for the given MAC based on total connected count.
//
// Strategy:
//   - 1 node: tx_rx (both TX and RX, single-node mode)
//   - 2 nodes: one TX, one RX (alternating by join order)
//   - 3+ nodes: floor(N/2) nodes assigned TX, rest RX, staggered TX slots
func (m *Manager) assignRole(mac string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	n := len(m.online)
	switch {
	case n <= 1:
		return "tx_rx"
	case n == 2:
		// First to arrive is TX, second is RX.
		if m.txCount == 0 {
			m.txCount++
			m.txNodes = append(m.txNodes, mac)
			return "tx"
		}
		return "rx"
	default:
		// Keep TX count at floor(N/2), promote this node to TX if needed.
		targetTX := n / 2
		if len(m.txNodes) < targetTX {
			m.txCount++
			m.txNodes = append(m.txNodes, mac)
			return "tx"
		}
		return "rx"
	}
}

// rebalanceRoles re-evaluates TX/RX assignments when a node leaves.
func (m *Manager) rebalanceRoles() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Rebuild tx node list from only online nodes.
	online := make([]string, 0, len(m.online))
	for mac := range m.online {
		online = append(online, mac)
	}
	sort.Strings(online)

	n := len(online)
	if n == 0 {
		m.txNodes = nil
		m.txCount = 0
		return
	}

	targetTX := 1
	if n >= 2 {
		targetTX = n / 2
	}

	newTX := make([]string, 0, targetTX)
	for i := 0; i < len(online) && len(newTX) < targetTX; i++ {
		newTX = append(newTX, online[i])
	}
	m.txNodes = newTX
	m.txCount = len(newTX)

	notifier := m.notifier
	if notifier == nil {
		return
	}

	// Send updated roles; stagger TX slot assignments.
	nTX := len(newTX)
	for i, mac := range online {
		role := "rx"
		txIndex := -1
		for j, txMAC := range newTX {
			if mac == txMAC {
				role = "tx"
				txIndex = j
				break
			}
		}
		if n == 1 {
			role = "tx_rx"
		}

		_ = m.registry.SetNodeRole(mac, role) //nolint:errcheck

		// Stagger TX slot: divide 1s into nTX slots.
		rateHz := 20
		txSlotUS := 0
		if (role == "tx" || role == "tx_rx") && nTX > 1 {
			slotUS := 1000000 / (rateHz * nTX)
			// Use TX index for proper staggering
			if txIndex >= 0 {
				txSlotUS = txIndex * slotUS
			} else {
				// For tx_rx role, use node index
				txSlotUS = i * slotUS
			}
		}

		// Update collision detector tracking
		m.updateTXNodeCollisionTrackingLocked(mac, role)

		notifier.SendRoleToMAC(mac, role, "")
		notifier.SendConfigToMAC(mac, rateHz, txSlotUS, 0.02)
	}
}

// applyRoleAndConfig sends role and rate config to a single node.
func (m *Manager) applyRoleAndConfig(mac, role string) {
	m.mu.Lock()
	notifier := m.notifier
	txNodes := m.txNodes
	m.mu.Unlock()

	if notifier == nil {
		return
	}

	notifier.SendRoleToMAC(mac, role, "")

	rateHz := 20

	// Compute txSlotUS for TX nodes
	txSlotUS := 0
	if role == "tx" || role == "tx_rx" {
		// Find the index of this MAC in the TX nodes list
		txIndex := -1
		for i, txMAC := range txNodes {
			if txMAC == mac {
				txIndex = i
				break
			}
		}

		// Calculate stagger offset if we have multiple TX nodes
		numTX := len(txNodes)
		if numTX > 1 && txIndex >= 0 {
			slotUS := 1_000_000 / (rateHz * numTX)
			txSlotUS = txIndex * slotUS
		}
	}

	// Register/unregister with collision detector based on role
	m.updateTXNodeCollisionTracking(mac, role)

	notifier.SendConfigToMAC(mac, rateHz, txSlotUS, 0.02)
}

// selfHeal checks for mismatched roles and re-pushes config if needed.
func (m *Manager) selfHeal() {
	nodes, err := m.registry.GetAllNodes()
	if err != nil {
		log.Printf("[WARN] fleet: self-heal query: %v", err)
		return
	}

	m.mu.RLock()
	notifier := m.notifier
	m.mu.RUnlock()

	if notifier == nil {
		return
	}

	connected := make(map[string]struct{})
	for _, mac := range notifier.GetConnectedMACs() {
		connected[mac] = struct{}{}
	}

	for _, n := range nodes {
		if _, ok := connected[n.MAC]; !ok {
			continue
		}
		// Re-push stored role for nodes that are online.
		notifier.SendRoleToMAC(n.MAC, n.Role, "")
	}
}

// broadcastRegistry sends current node and room state to dashboard clients.
func (m *Manager) broadcastRegistry() {
	m.mu.RLock()
	bcaster := m.bcaster
	m.mu.RUnlock()

	if bcaster == nil {
		return
	}

	nodes, err := m.registry.GetAllNodes()
	if err != nil {
		log.Printf("[WARN] fleet: get nodes for broadcast: %v", err)
		return
	}

	room, err := m.registry.GetRoom()
	if err != nil {
		log.Printf("[WARN] fleet: get room for broadcast: %v", err)
		return
	}

	bcaster.BroadcastRegistryState(nodes, *room)
}

// ─── System Mode Management ─────────────────────────────────────────────────────

// SetModeChangeBroadcaster sets the broadcaster for mode change events.
func (m *Manager) SetModeChangeBroadcaster(b ModeChangeBroadcaster) {
	m.mu.Lock()
	m.modeChangeBroadcaster = b
	m.mu.Unlock()
}

// SetBLEPresenceProvider sets the BLE presence provider for auto-away detection.
func (m *Manager) SetBLEPresenceProvider(p BLEPresenceProvider) {
	m.mu.Lock()
	m.blePresenceProvider = p
	m.mu.Unlock()
}

// ProcessBLEObservations processes BLE observations for auto-away/disarm detection.
// This should be called when BLE data is received from nodes.
func (m *Manager) ProcessBLEObservations(observations []BLEObservation) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Skip if no BLE provider or auto-away is disabled
	if m.blePresenceProvider == nil || !m.autoAwayConfig.Enabled {
		return
	}

	// Check if manual override is active
	if time.Now().Before(m.manualOverrideUntil) {
		return
	}

	now := time.Now()

	// Get all registered devices
	registeredDevices, err := m.blePresenceProvider.GetAllRegisteredDevices()
	if err != nil {
		log.Printf("[WARN] fleet: get registered devices: %v", err)
		return
	}

	// Check for auto-disarm: any registered device seen with RSSI > threshold
	if m.systemMode == events.ModeAway {
		for _, obs := range observations {
			if personID, isRegistered := registeredDevices[obs.DeviceMAC]; isRegistered {
				if obs.RSSIdBm >= m.autoAwayConfig.AutoDisarmRSSI {
					// Get person name if available
					personName := ""
					if m.personProvider != nil {
						personName = m.personProvider.GetPersonName(personID)
					}

					// Auto-disarm
					prevMode := m.systemMode
					m.systemMode = events.ModeHome

					event := events.SystemModeChangeEvent{
						PreviousMode: prevMode,
						NewMode:      events.ModeHome,
						Reason:       "auto_disarm",
						Timestamp:    now,
						PersonID:     personID,
						PersonName:   personName,
					}

					if m.modeChangeBroadcaster != nil {
						m.modeChangeBroadcaster.BroadcastSystemModeChange(event)
					}

					if m.onModeChange != nil {
						go m.onModeChange(event)
					}

					log.Printf("[INFO] fleet: auto-disarm triggered - registered device %s seen (RSSI: %d)", obs.DeviceMAC, obs.RSSIdBm)
					return
				}
			}
		}
	}

	// Update last seen times for registered devices
	for _, obs := range observations {
		if _, isRegistered := registeredDevices[obs.DeviceMAC]; isRegistered {
			m.lastDeviceSeen[obs.DeviceMAC] = now
		}
	}
}

// CheckAutoAway checks if all registered devices have been absent for the configured duration.
// This should be called periodically.
func (m *Manager) CheckAutoAway() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Skip if no BLE provider or auto-away is disabled
	if m.blePresenceProvider == nil || !m.autoAwayConfig.Enabled {
		return
	}

	// Check if manual override is active
	if time.Now().Before(m.manualOverrideUntil) {
		return
	}

	// Don't auto-away if already away
	if m.systemMode == events.ModeAway {
		return
	}

	// Get all registered devices
	registeredDevices, err := m.blePresenceProvider.GetAllRegisteredDevices()
	if err != nil {
		log.Printf("[WARN] fleet: get registered devices for auto-away: %v", err)
		return
	}

	if len(registeredDevices) == 0 {
		return // No registered devices, can't determine away status
	}

	// Check if all devices have been absent for the configured duration
	now := time.Now()
	allAbsent := true

	for mac := range registeredDevices {
		lastSeen, exists := m.lastDeviceSeen[mac]
		if !exists || now.Sub(lastSeen) >= m.autoAwayConfig.AbsenceDuration {
			// Device not seen recently
			continue
		}
		// At least one device is present
		allAbsent = false
		break
	}

	if allAbsent {
		// Auto-away
		prevMode := m.systemMode
		m.systemMode = events.ModeAway

		event := events.SystemModeChangeEvent{
			PreviousMode: prevMode,
			NewMode:      events.ModeAway,
			Reason:       "auto_away",
			Timestamp:    now,
		}

		if m.modeChangeBroadcaster != nil {
			m.modeChangeBroadcaster.BroadcastSystemModeChange(event)
		}

		if m.onModeChange != nil {
			go m.onModeChange(event)
		}

		log.Printf("[INFO] fleet: auto-away activated - all BLE devices absent for %v", m.autoAwayConfig.AbsenceDuration)
	}
}

// RunModeCheck starts the periodic auto-away check loop.
func (m *Manager) RunModeCheck(ctx context.Context) {
	ticker := time.NewTicker(m.modeCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.CheckAutoAway()
		}
	}
}

// GetAutoAwayConfig returns the current auto-away configuration.
func (m *Manager) GetAutoAwayConfig() AutoAwayConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.autoAwayConfig
}

// SetAutoAwayConfig updates the auto-away configuration.
func (m *Manager) SetAutoAwayConfig(cfg AutoAwayConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.autoAwayConfig = cfg
}

// SetPersonProvider sets the person name provider for mode change events.
func (m *Manager) SetPersonProvider(p PersonNameProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.personProvider = p
}

// GetSystemMode returns the current system mode.
func (m *Manager) GetSystemMode() events.SystemMode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.systemMode
}

// SetSystemMode manually sets the system mode with a reason.
func (m *Manager) SetSystemMode(mode events.SystemMode, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	prevMode := m.systemMode
	if prevMode == mode {
		return nil // No change needed
	}

	m.systemMode = mode

	// Set manual override pause
	m.manualOverrideUntil = time.Now().Add(m.autoAwayConfig.ManualOverridePause)

	event := events.SystemModeChangeEvent{
		PreviousMode: prevMode,
		NewMode:      mode,
		Reason:       reason,
		Timestamp:    time.Now(),
	}

	if m.modeChangeBroadcaster != nil {
		m.modeChangeBroadcaster.BroadcastSystemModeChange(event)
	}

	if m.onModeChange != nil {
		go m.onModeChange(event)
	}

	log.Printf("[INFO] fleet: system mode changed: %s -> %s (reason: %s)", prevMode, mode, reason)
	return nil
}

// SetOnModeChange sets the callback for mode change events.
func (m *Manager) SetOnModeChange(cb func(events.SystemModeChangeEvent)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onModeChange = cb
}

// IsSecurityMode returns true if the system is in away mode (security mode).
func (m *Manager) IsSecurityMode() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.systemMode == events.ModeAway
}

// IsManualOverrideActive returns true if a manual mode override is currently active.
func (m *Manager) IsManualOverrideActive() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return time.Now().Before(m.manualOverrideUntil)
}

// GetConnectedMACs returns the MACs of currently-connected nodes.
// Delegates to the NodeStateNotifier.
func (m *Manager) GetConnectedMACs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.notifier == nil {
		return nil
	}
	return m.notifier.GetConnectedMACs()
}

// ─── Collision Detection & Adaptive Re-Stagger ─────────────────────────────────

// GetCollisionDetector returns the collision detector for integration with ingestion.
func (m *Manager) GetCollisionDetector() *CollisionDetector {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.collisionDetector
}

// checkCollisions checks the collision rate and triggers re-stagger if needed.
func (m *Manager) checkCollisions() {
	m.mu.RLock()
	cd := m.collisionDetector
	m.mu.RUnlock()

	if cd != nil {
		cd.CheckAndTriggerRestagger()
	}
}

// AdaptiveRestagger performs adaptive re-stagger of TX slot assignments.
// This is called when the collision rate exceeds the threshold.
func (m *Manager) AdaptiveRestagger() {
	log.Printf("[INFO] fleet: Performing adaptive re-stagger due to high collision rate")

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.notifier == nil {
		log.Printf("[WARN] fleet: Cannot re-stagger - no notifier configured")
		return
	}

	// Get current TX nodes
	txMACs := make([]string, 0, len(m.txNodes))
	for _, mac := range m.txNodes {
		if _, online := m.online[mac]; online {
			txMACs = append(txMACs, mac)
		}
	}

	if len(txMACs) < 2 {
		log.Printf("[DEBUG] fleet: Skipping re-stagger - fewer than 2 TX nodes")
		return
	}

	// Generate new stagger offsets
	rateHz := 20 // Default rate Hz
	newOffsets := GetRestaggerOffsets(txMACs, rateHz)

	// Push updated config to all TX nodes
	for _, mac := range txMACs {
		txSlotUS := newOffsets[mac]
		m.notifier.SendConfigToMAC(mac, rateHz, txSlotUS, 0.02)
		log.Printf("[INFO] fleet: Re-staggered %s to tx_slot_us=%d", mac, txSlotUS)
	}

	log.Printf("[INFO] fleet: Adaptive re-stagger complete - %d TX nodes updated", len(txMACs))
}

// updateTXNodeCollisionTracking updates the collision detector's TX node registration
// when a node's role changes.
func (m *Manager) updateTXNodeCollisionTracking(mac string, role string) {
	m.mu.RLock()
	cd := m.collisionDetector
	m.mu.RUnlock()

	if cd == nil {
		return
	}

	// Unregister first (in case role is changing)
	cd.UnregisterTXNode(mac)

	// Register as TX if role is tx or tx_rx
	if role == "tx" || role == "tx_rx" {
		cd.RegisterTXNode(mac)
	}
}

// updateTXNodeCollisionTrackingLocked updates the collision detector's TX node registration
// when a node's role changes. Caller must hold m.mu.
func (m *Manager) updateTXNodeCollisionTrackingLocked(mac string, role string) {
	cd := m.collisionDetector
	if cd == nil {
		return
	}

	// Unregister first (in case role is changing)
	cd.UnregisterTXNode(mac)

	// Register as TX if role is tx or tx_rx
	if role == "tx" || role == "tx_rx" {
		cd.RegisterTXNode(mac)
	}
}
