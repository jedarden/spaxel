package fleet

import (
	"context"
	"log"
	"sort"
	"sync"
	"time"
)

// NodeStateNotifier is called when the manager sends a role or config to a node.
type NodeStateNotifier interface {
	// SendRoleToMAC sends a role assignment message to a connected node.
	SendRoleToMAC(mac, role, passiveBSSID string)
	// SendConfigToMAC sends a rate config to a connected node.
	SendConfigToMAC(mac string, rateHz int, varianceThreshold float64)
	// GetConnectedMACs returns the MACs of currently-connected nodes.
	GetConnectedMACs() []string
}

// RegistryBroadcaster is called when fleet state changes that the dashboard should see.
type RegistryBroadcaster interface {
	BroadcastRegistryState(nodes []NodeRecord, room RoomConfig)
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
}

// NewManager creates a fleet manager backed by registry.
func NewManager(reg *Registry) *Manager {
	return &Manager{
		registry: reg,
		online:   make(map[string]struct{}),
		healTick: 60 * time.Second,
	}
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

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.selfHeal()
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
		for _, txMAC := range newTX {
			if mac == txMAC {
				role = "tx"
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
		if role == "tx" && nTX > 1 {
			slotUS := 1000000 / (rateHz * nTX)
			txSlotUS = i * slotUS
		}
		_ = txSlotUS // will send via config when we have the param available
		notifier.SendRoleToMAC(mac, role, "")
	}
}

// applyRoleAndConfig sends role and rate config to a single node.
func (m *Manager) applyRoleAndConfig(mac, role string) {
	m.mu.RLock()
	notifier := m.notifier
	m.mu.RUnlock()

	if notifier == nil {
		return
	}

	notifier.SendRoleToMAC(mac, role, "")

	rateHz := 20
	if role == "rx" || role == "tx_rx" {
		notifier.SendConfigToMAC(mac, rateHz, 0.02)
	}
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
