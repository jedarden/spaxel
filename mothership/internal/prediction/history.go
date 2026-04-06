// Package prediction provides presence prediction using transition probability models.
package prediction

import (
	"log"
	"sync"
	"time"
)

// HistoryUpdater handles recording zone transitions and triggering model recomputation.
type HistoryUpdater struct {
	mu sync.RWMutex

	store *ModelStore

	// Current zone tracking for each person
	personZones map[string]struct {
		ZoneID    string
		EntryTime time.Time
		BlobID    int
	}

	// Recomputation schedule
	lastRecompute    time.Time
	recomputeDays    int // Days between recomputation
	onRecomputeStart func()
	onRecomputeEnd   func()
}

// NewHistoryUpdater creates a new history updater.
func NewHistoryUpdater(store *ModelStore) *HistoryUpdater {
	return &HistoryUpdater{
		store:       store,
		personZones: make(map[string]struct {
			ZoneID    string
			EntryTime time.Time
			BlobID    int
		}),
		recomputeDays: 7, // Recompute weekly by default
	}
}

// SetRecomputeInterval sets the number of days between recomputations.
func (h *HistoryUpdater) SetRecomputeInterval(days int) {
	h.mu.Lock()
	h.recomputeDays = days
	h.mu.Unlock()
}

// SetOnRecomputeStart sets callback for recomputation start.
func (h *HistoryUpdater) SetOnRecomputeStart(cb func()) {
	h.mu.Lock()
	h.onRecomputeStart = cb
	h.mu.Unlock()
}

// SetOnRecomputeEnd sets callback for recomputation end.
func (h *HistoryUpdater) SetOnRecomputeEnd(cb func()) {
	h.mu.Lock()
	h.onRecomputeEnd = cb
	h.mu.Unlock()
}

// PersonZoneChange records a zone transition for a person.
func (h *HistoryUpdater) PersonZoneChange(personID, fromZoneID, toZoneID string, blobID int, timestamp time.Time) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Get previous zone info
	prev, existed := h.personZones[personID]

	// Calculate dwell duration if we have a previous zone
	var dwellMinutes float64
	if existed && prev.ZoneID != "" {
		dwellMinutes = timestamp.Sub(prev.EntryTime).Minutes()
	}

	// Record the transition if there was a previous zone
	if fromZoneID != "" && toZoneID != "" && fromZoneID != toZoneID {
		hourOfWeek := HourOfWeek(timestamp)

		transition := ZoneTransition{
			PersonID:             personID,
			FromZoneID:           fromZoneID,
			ToZoneID:             toZoneID,
			HourOfWeek:           hourOfWeek,
			DwellDurationMinutes: dwellMinutes,
			Timestamp:            timestamp,
		}

		// Generate ID
		transition.ID = generateTransitionID(personID, timestamp)

		if err := h.store.RecordTransition(transition); err != nil {
			log.Printf("[WARN] prediction: failed to record transition: %v", err)
			return err
		}

		log.Printf("[INFO] prediction: recorded transition %s -> %s for %s (dwell: %.1f min, hour: %d)",
			fromZoneID, toZoneID, personID, dwellMinutes, hourOfWeek)
	}

	// Update current zone
	h.personZones[personID] = struct {
		ZoneID    string
		EntryTime time.Time
		BlobID    int
	}{
		ZoneID:    toZoneID,
		EntryTime: timestamp,
		BlobID:    blobID,
	}

	// Also update in store for persistence
	if toZoneID != "" {
		h.store.UpdatePersonZoneEntry(personID, toZoneID, timestamp, blobID)
	} else if fromZoneID != "" {
		h.store.ClearPersonZoneEntry(personID, fromZoneID)
	}

	// Check if we need to recompute probabilities
	h.checkRecompute(timestamp)

	return nil
}

// UpdatePersonZone updates the current zone for a person without recording a transition.
// Use this when initializing from stored state.
func (h *HistoryUpdater) UpdatePersonZone(personID, zoneID string, blobID int, entryTime time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.personZones[personID] = struct {
		ZoneID    string
		EntryTime time.Time
		BlobID    int
	}{
		ZoneID:    zoneID,
		EntryTime: entryTime,
		BlobID:    blobID,
	}
}

// GetPersonZone returns the current zone for a person.
func (h *HistoryUpdater) GetPersonZone(personID string) (zoneID string, entryTime time.Time, blobID int, ok bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	info, exists := h.personZones[personID]
	if !exists {
		return "", time.Time{}, 0, false
	}
	return info.ZoneID, info.EntryTime, info.BlobID, true
}

// GetAllPersonZones returns all current person zones.
func (h *HistoryUpdater) GetAllPersonZones() map[string]struct {
	ZoneID    string
	EntryTime time.Time
	BlobID    int
} {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make(map[string]struct {
		ZoneID    string
		EntryTime time.Time
		BlobID    int
	})
	for k, v := range h.personZones {
		result[k] = v
	}
	return result
}

// PersonLeft records that a person has left all zones.
func (h *HistoryUpdater) PersonLeft(personID string, timestamp time.Time) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	prev, existed := h.personZones[personID]
	if !existed || prev.ZoneID == "" {
		return nil
	}

	// Record transition to "away"
	hourOfWeek := HourOfWeek(timestamp)
	dwellMinutes := timestamp.Sub(prev.EntryTime).Minutes()

	transition := ZoneTransition{
		PersonID:             personID,
		FromZoneID:           prev.ZoneID,
		ToZoneID:             "away",
		HourOfWeek:           hourOfWeek,
		DwellDurationMinutes: dwellMinutes,
		Timestamp:            timestamp,
	}
	transition.ID = generateTransitionID(personID, timestamp)

	if err := h.store.RecordTransition(transition); err != nil {
		log.Printf("[WARN] prediction: failed to record departure transition: %v", err)
		return err
	}

	// Clear zone
	h.store.ClearPersonZoneEntry(personID, prev.ZoneID)
	delete(h.personZones, personID)

	log.Printf("[INFO] prediction: recorded departure from %s for %s (dwell: %.1f min)",
		prev.ZoneID, personID, dwellMinutes)

	return nil
}

// checkRecompute checks if it's time to recompute probabilities.
func (h *HistoryUpdater) checkRecompute(now time.Time) {
	if h.recomputeDays <= 0 {
		return
	}

	if h.lastRecompute.IsZero() {
		// First run - schedule recomputation
		go h.doRecompute()
		h.lastRecompute = now
		return
	}

	// Check if enough time has passed
	nextRecompute := h.lastRecompute.Add(time.Duration(h.recomputeDays) * 24 * time.Hour)
	if now.After(nextRecompute) {
		go h.doRecompute()
		h.lastRecompute = now
	}
}

// doRecompute performs probability recomputation.
func (h *HistoryUpdater) doRecompute() {
	if h.onRecomputeStart != nil {
		h.onRecomputeStart()
	}

	log.Printf("[INFO] prediction: starting probability recomputation")

	if err := h.store.RecomputeProbabilities(); err != nil {
		log.Printf("[ERROR] prediction: failed to recompute probabilities: %v", err)
	}

	if err := h.store.RecomputeDwellTimes(); err != nil {
		log.Printf("[ERROR] prediction: failed to recompute dwell times: %v", err)
	}

	log.Printf("[INFO] prediction: completed probability recomputation")

	if h.onRecomputeEnd != nil {
		h.onRecomputeEnd()
	}
}

// ForceRecompute forces an immediate probability recomputation.
func (h *HistoryUpdater) ForceRecompute() error {
	h.doRecompute()
	h.mu.Lock()
	h.lastRecompute = time.Now()
	h.mu.Unlock()
	return nil
}

// LoadStoredPositions loads person zone positions from the store.
func (h *HistoryUpdater) LoadStoredPositions() error {
	entries, err := h.store.GetAllPersonZoneEntries()
	if err != nil {
		return err
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	for personID, entry := range entries {
		h.personZones[personID] = struct {
			ZoneID    string
			EntryTime time.Time
			BlobID    int
		}{
			ZoneID:    entry.ZoneID,
			EntryTime: entry.EntryTime,
			BlobID:    entry.BlobID,
		}
	}

	log.Printf("[INFO] prediction: loaded %d stored person zone entries", len(entries))
	return nil
}

// generateTransitionID generates a unique ID for a transition.
func generateTransitionID(personID string, timestamp time.Time) string {
	return personID + "-" + timestamp.Format("20060102-150405.000")
}

// GetTransitionStats returns statistics about recorded transitions.
func (h *HistoryUpdater) GetTransitionStats() (totalCount int, dataAge time.Duration, err error) {
	totalCount, err = h.store.GetTransitionCount()
	if err != nil {
		return 0, 0, err
	}
	dataAge = h.store.GetDataAge()
	return totalCount, dataAge, nil
}
