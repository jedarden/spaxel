// Package simulator provides common types used across simulation packages.
package simulator

// Link represents a directional TX->RX connection between two nodes.
// In simulation, links are used for GDOP computation and CSI generation.
type Link struct {
	TX *Node // Transmitting node
	RX *Node // Receiving node
}

// ID returns a unique identifier for this link
func (l Link) ID() string {
	return l.TX.ID + ":" + l.RX.ID
}

// Reverse returns the link with TX and RX swapped
func (l Link) Reverse() Link {
	return Link{TX: l.RX, RX: l.TX}
}

// CanonicalID returns the canonical form of the link ID for storage.
// For bidirectional links (TX/RX or TX_RX mode), this provides a consistent ID
// regardless of direction. For passive links (AP as TX), the AP is always first.
func (l Link) CanonicalID() string {
	if l.TX.IsAP() {
		// AP is always first component
		return l.TX.ID + ":" + l.RX.ID
	}
	// Sort lexicographically for bidirectional links
	if l.TX.ID < l.RX.ID {
		return l.TX.ID + ":" + l.RX.ID
	}
	return l.RX.ID + ":" + l.TX.ID
}

// IsBidirectional returns true if this link represents bidirectional communication
// (both nodes can TX and RX)
func (l Link) IsBidirectional() bool {
	return (l.TX.Role == RoleTXRX || l.TX.Role == RoleTX) &&
		(l.RX.Role == RoleTXRX || l.RX.Role == RoleRX)
}

// IsPassive returns true if this is a passive radar link (AP as TX)
func (l Link) IsPassive() bool {
	return l.TX.IsAP() && (l.RX.Role == RoleRX || l.RX.Role == RoleTXRX || l.RX.Role == RolePassive)
}
