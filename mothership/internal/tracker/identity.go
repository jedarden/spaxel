package tracker

import (
	"sync"
	"time"
)

// IdentityMatcher interface for dependency injection from ble package.
type IdentityMatcher interface {
	GetMatch(blobID int) *struct {
		PersonID          string
		PersonName        string
		PersonColor       string
		Confidence        float64
		IsBLEOnly         bool
		Timestamp         time.Time
	}
	GetMatches() map[int]interface{}
}

// IdentityInfo represents identity information for a blob.
type IdentityInfo struct {
	PersonID          string
	PersonLabel       string
	PersonColor       string
	IdentityConfidence float64
	IdentitySource    string
}

// IdentityProvider provides identity matching for tracked blobs.
type IdentityProvider interface {
	// GetIdentity returns identity info for a blob, or nil if no match.
	GetIdentity(blobID int) *IdentityInfo
}

// TrackManager wraps Tracker with identity matching support.
type TrackManager struct {
	mu             sync.RWMutex
	tracker        *Tracker
	identity       IdentityProvider
	blobs          []Blob
	identityTTL    time.Duration // How long to persist identity after match lost
	lastIdentities map[int]*IdentityInfo
}

// NewTrackManager creates a new track manager.
func NewTrackManager(tracker *Tracker) *TrackManager {
	return &TrackManager{
		tracker:        tracker,
		identityTTL:    5 * time.Minute,
		lastIdentities: make(map[int]*IdentityInfo),
	}
}

// SetIdentityProvider sets the identity provider (usually ble.IdentityMatcher).
func (tm *TrackManager) SetIdentityProvider(provider IdentityProvider) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.identity = provider
}

// Update runs a tracking cycle and updates blob identities.
func (tm *TrackManager) Update(measurements [][4]float64) []Blob {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Run the underlying tracker
	tm.blobs = tm.tracker.Update(measurements)

	// Update identities
	tm.updateIdentities()

	return tm.blobs
}

// UpdateWithIdentity runs tracking with explicit identity updates from external matcher.
func (tm *TrackManager) UpdateWithIdentity(measurements [][4]float64, identities map[int]*IdentityInfo) []Blob {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Run the underlying tracker
	tm.blobs = tm.tracker.Update(measurements)

	// Apply external identities
	now := time.Now()
	applied := make(map[int]bool)

	for _, i := range tm.blobs {
		if info, ok := identities[i.ID]; ok {
			tm.applyIdentity(i, info, now)
			tm.lastIdentities[i.ID] = info
			applied[i.ID] = true
		} else if lastInfo, hadIdentity := tm.lastIdentities[i.ID]; hadIdentity {
			// Check persistence
			if now.Sub(i.IdentityLastSeen) < tm.identityTTL {
				// Keep the identity
				i.PersonID = lastInfo.PersonID
				i.PersonLabel = lastInfo.PersonLabel
				i.PersonColor = lastInfo.PersonColor
				i.IdentityConfidence = lastInfo.IdentityConfidence * 0.9 // Decay slightly
				i.IdentitySource = "persistent"
			} else {
				// Identity expired
				delete(tm.lastIdentities, i.ID)
				tm.clearIdentity(i)
			}
		}
	}

	// Clean up stale identities
	for id := range tm.lastIdentities {
		if !applied[id] {
			found := false
			for _, b := range tm.blobs {
				if b.ID == id {
					found = true
					break
				}
			}
			if !found {
				delete(tm.lastIdentities, id)
			}
		}
	}

	return tm.blobs
}

// updateIdentities applies identity information to tracked blobs.
func (tm *TrackManager) updateIdentities() {
	if tm.identity == nil {
		return
	}

	now := time.Now()

	for i := range tm.blobs {
		blob := &tm.blobs[i]
		info := tm.identity.GetIdentity(blob.ID)

		if info != nil {
			tm.applyIdentity(blob, info, now)
			tm.lastIdentities[blob.ID] = info
		} else if lastInfo, hadIdentity := tm.lastIdentities[blob.ID]; hadIdentity {
			// Check if identity should persist
			if now.Sub(blob.IdentityLastSeen) < tm.identityTTL {
				// Keep the identity
				blob.PersonID = lastInfo.PersonID
				blob.PersonLabel = lastInfo.PersonLabel
				blob.PersonColor = lastInfo.PersonColor
				blob.IdentityConfidence = lastInfo.IdentityConfidence * 0.9 // Decay slightly
				blob.IdentitySource = "persistent"
			} else {
				// Identity expired
				delete(tm.lastIdentities, blob.ID)
				tm.clearIdentity(blob)
			}
		}
	}
}

// applyIdentity applies identity info to a blob.
func (tm *TrackManager) applyIdentity(blob *Blob, info *IdentityInfo, now time.Time) {
	blob.PersonID = info.PersonID
	blob.PersonLabel = info.PersonLabel
	blob.PersonColor = info.PersonColor
	blob.IdentityConfidence = info.IdentityConfidence
	blob.IdentityLastSeen = now

	if info.IdentitySource != "" {
		blob.IdentitySource = info.IdentitySource
	} else {
		blob.IdentitySource = "ble_triangulation"
	}
}

// clearIdentity clears identity fields from a blob.
func (tm *TrackManager) clearIdentity(blob *Blob) {
	blob.PersonID = ""
	blob.PersonLabel = ""
	blob.PersonColor = ""
	blob.IdentityConfidence = 0
	blob.IdentitySource = ""
}

// GetBlob returns a blob by ID, or nil if not found.
func (tm *TrackManager) GetBlob(id int) *Blob {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	for i := range tm.blobs {
		if tm.blobs[i].ID == id {
			return &tm.blobs[i]
		}
	}
	return nil
}

// GetAllBlobs returns all current blobs.
func (tm *TrackManager) GetAllBlobs() []Blob {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	result := make([]Blob, len(tm.blobs))
	copy(result, tm.blobs)
	return result
}

// Reset clears all tracks.
func (tm *TrackManager) Reset() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.tracker.Reset()
	tm.blobs = nil
	tm.lastIdentities = make(map[int]*IdentityInfo)
}

// SetIdentityTTL sets the identity persistence duration.
func (tm *TrackManager) SetIdentityTTL(ttl time.Duration) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.identityTTL = ttl
}
