package tracker

import (
	"github.com/spaxel/mothership/internal/ble"
)

// IdentityMatcherGetter is the interface expected from the BLE package's IdentityMatcher.
// This allows the TrackManager to get identity information from the BLE matcher.
type IdentityMatcherGetter interface {
	GetMatch(blobID int) *ble.IdentityMatch
	GetPersistentIdentity(blobID int) *ble.IdentityMatch
}

// BLEIdentityProvider adapts ble.IdentityMatcher to the tracker.IdentityProvider interface.
// This allows the TrackManager to get identity information from the BLE matcher.
type BLEIdentityProvider struct {
	matcher IdentityMatcherGetter
}

// NewBLEIdentityProvider creates a new BLE identity provider.
func NewBLEIdentityProvider(matcher IdentityMatcherGetter) *BLEIdentityProvider {
	return &BLEIdentityProvider{
		matcher: matcher,
	}
}

// GetIdentity returns identity info for a blob, or nil if no match.
// It first checks for a current match, then falls back to persistent identity.
func (p *BLEIdentityProvider) GetIdentity(blobID int) *IdentityInfo {
	if p.matcher == nil {
		return nil
	}

	// Try current match first
	if match := p.matcher.GetMatch(blobID); match != nil {
		source := "ble_triangulation"
		if match.IsBLEOnly {
			source = "ble_only"
		}
		return &IdentityInfo{
			PersonID:          match.PersonID,
			PersonLabel:       match.PersonName,
			PersonColor:       match.PersonColor,
			IdentityConfidence: match.Confidence,
			IdentitySource:    source,
		}
	}

	// Fall back to persistent identity (for 5-min persistence)
	if persist := p.matcher.GetPersistentIdentity(blobID); persist != nil {
		source := "ble_triangulation"
		if persist.IsBLEOnly {
			source = "ble_only"
		}
		return &IdentityInfo{
			PersonID:          persist.PersonID,
			PersonLabel:       persist.PersonName,
			PersonColor:       persist.PersonColor,
			IdentityConfidence: persist.Confidence,
			IdentitySource:    source,
		}
	}

	return nil
}
