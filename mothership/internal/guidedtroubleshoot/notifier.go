// Package guidedtroubleshoot provides proactive contextual help and
// post-feedback explanations for Spaxel users.
package guidedtroubleshoot

import (
	"log"
	"sync"
	"time"
)

// FleetNotifier integrates the guided troubleshooting manager with the
// fleet's node connection events. It implements the ingestion.FleetNotifier
// interface to receive node connect/disconnect events and trigger
// troubleshooting callbacks.
type FleetNotifier struct {
	mu              sync.RWMutex
	mgr             *Manager
	offlineNodes    map[string]time.Time // mac -> offline start time
	getNodeLastSeen func(mac string) time.Time
}

// NewFleetNotifier creates a new fleet notifier for the guided manager.
func NewFleetNotifier(mgr *Manager, getNodeLastSeen func(mac string) time.Time) *FleetNotifier {
	return &FleetNotifier{
		mgr:             mgr,
		offlineNodes:    make(map[string]time.Time),
		getNodeLastSeen: getNodeLastSeen,
	}
}

// OnNodeConnected is called when a node connects.
func (n *FleetNotifier) OnNodeConnected(mac, firmware, chip string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Clear offline tracking when node reconnects
	delete(n.offlineNodes, mac)
	log.Printf("[DEBUG] guidedtroubleshoot: node connected %s", mac)
}

// OnNodeDisconnected is called when a node disconnects.
func (n *FleetNotifier) OnNodeDisconnected(mac string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Record when the node went offline
	n.offlineNodes[mac] = time.Now()
	log.Printf("[DEBUG] guidedtroubleshoot: node disconnected %s", mac)
}

// CheckOfflineNodes checks all tracked offline nodes and triggers callbacks
// for nodes that have been offline for more than 2 hours.
// This should be called periodically from the manager's Run loop.
func (n *FleetNotifier) CheckOfflineNodes() {
	n.mu.Lock()
	defer n.mu.Unlock()

	offlineThreshold := 2 * time.Hour
	now := time.Now()

	for mac, offlineStart := range n.offlineNodes {
		offlineDuration := now.Sub(offlineStart)

		// Trigger callback if offline for >2 hours
		if offlineDuration >= offlineThreshold {
			if n.mgr != nil {
				n.mgr.TriggerNodeOffline(mac, offlineDuration)
			}
			// Remove from tracking so we don't trigger again for the same offline event
			delete(n.offlineNodes, mac)
		}
	}
}

// GetOfflineDuration returns how long a node has been offline, or 0 if online.
func (n *FleetNotifier) GetOfflineDuration(mac string) time.Duration {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if offlineStart, exists := n.offlineNodes[mac]; exists {
		return time.Since(offlineStart)
	}
	return 0
}
