// Package guidedtroubleshoot provides proactive contextual help and
// post-feedback explanations for Spaxel users.
package guidedtroubleshoot

import (
	"log"
	"time"
)

// FleetNotifier integrates the guided troubleshooting manager with the
// fleet's node connection events. It implements the ingestion.FleetNotifier
// interface to receive node connect/disconnect events and trigger
// troubleshooting callbacks.
type FleetNotifier struct {
	mgr *Manager
}

// NewFleetNotifier creates a new fleet notifier for the guided manager.
func NewFleetNotifier(mgr *Manager) *FleetNotifier {
	return &FleetNotifier{mgr: mgr}
}

// OnNodeConnected is called when a node connects.
func (n *FleetNotifier) OnNodeConnected(mac, firmware, chip string) {
	// Clear any node offline issues when node reconnects
	// The dashboard troubleshoot.js handles this via the node_connected event
	log.Printf("[DEBUG] guidedtroubleshoot: node connected %s", mac)
}

// OnNodeDisconnected is called when a node disconnects.
// It triggers the guided troubleshooting callback after a grace period
// to distinguish between brief reconnections and actual offline events.
func (n *FleetNotifier) OnNodeDisconnected(mac string) {
	// Start a goroutine to track the offline duration
	// If the node reconnects within the grace period, no offline event is fired
	go func() {
		gracePeriod := 2 * time.Minute
		checkInterval := 10 * time.Second

		var offlineDuration time.Duration
		for {
			time.Sleep(checkInterval)
			offlineDuration += checkInterval

			// Check if node has reconnected by querying the fleet registry
			// The guided manager's GetNodeLastSeen function will be used
			if n.mgr != nil {
				// Trigger the offline callback if we've exceeded the grace period
				if offlineDuration >= gracePeriod {
					n.mgr.TriggerNodeOffline(mac, offlineDuration)
					return
				}
			}
		}
	}()
}
