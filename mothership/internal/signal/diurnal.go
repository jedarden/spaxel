// Package signal implements diurnal adaptive baseline for hourly environmental patterns
package signal

import (
	"math"
	"sync"
	"time"
)

// Diurnal configuration constants
const (
	DiurnalSlots        = 24 // One slot per hour
	DiurnalMinSamples   = 300 // Minimum samples per slot (spec requirement: >= 300 samples/slot to mark ready)
	DiurnalLearningDays = 7   // Days before diurnal baseline activates

	// DiurnalUpdateAlpha is the slow EMA coefficient for slot updates
	// tau = 7 days at 2Hz = 7 * 24 * 3600 * 2 = 1209600 samples
	// alpha ≈ 0.00017 per sample (spec value)
	DiurnalUpdateAlpha = 0.00017

	// Confidence staleness threshold (days)
	DiurnalStaleDays = 3

	// DiurnalCrossfadeMinutes is the duration of the EMA-to-diurnal crossfade at hour boundaries
	DiurnalCrossfadeMinutes = 15 // Crossfade over first 15 minutes of each hour
)

// Confidence weights for composite score
const (
	ConfidenceWeightBaselineAge = 0.3
	ConfidenceWeightDiurnalProg = 0.3
	ConfidenceWeightPacketRate  = 0.4
)

// DiurnalSlot holds the baseline for a single hour of the day
type DiurnalSlot struct {
	Values      []float64 // Per-subcarrier baseline amplitude
	SampleCount int       // Number of quiet samples accumulated
	LastUpdate  time.Time // Time of last update
}

// DiurnalBaseline manages 24 hourly baseline slots for a single link
type DiurnalBaseline struct {
	mu      sync.RWMutex
	slots   [DiurnalSlots]*DiurnalSlot
	nSub    int
	linkID  string
	created time.Time // When this diurnal baseline was created
}

// NewDiurnalBaseline creates a new diurnal baseline manager
func NewDiurnalBaseline(linkID string, nSub int) *DiurnalBaseline {
	db := &DiurnalBaseline{
		nSub:    nSub,
		linkID:  linkID,
		created: time.Now(),
	}
	// Initialize all slots
	for i := 0; i < DiurnalSlots; i++ {
		db.slots[i] = &DiurnalSlot{
			Values: make([]float64, nSub),
		}
	}
	return db
}

// GetCurrentSlot returns the slot for the current hour
func (db *DiurnalBaseline) GetCurrentSlot() *DiurnalSlot {
	hour := time.Now().Hour()
	return db.slots[hour]
}

// GetSlot returns the slot for a specific hour (0-23)
func (db *DiurnalBaseline) GetSlot(hour int) *DiurnalSlot {
	if hour < 0 || hour >= DiurnalSlots {
		return nil
	}
	return db.slots[hour]
}

// Update updates the current hour's slot with quiet-room data
// This should only be called when smoothDeltaRMS < motion threshold
func (db *DiurnalBaseline) Update(amplitude []float64) {
	db.mu.Lock()
	defer db.mu.Unlock()

	hour := time.Now().Hour()
	slot := db.slots[hour]

	if len(amplitude) != db.nSub {
		return
	}

	// If slot is empty, initialize with current amplitude
	if slot.SampleCount == 0 {
		copy(slot.Values, amplitude)
		slot.SampleCount = 1
		slot.LastUpdate = time.Now()
		return
	}

	// Slow EMA update for the slot
	for k := 0; k < db.nSub && k < len(amplitude); k++ {
		slot.Values[k] = DiurnalUpdateAlpha*amplitude[k] + (1-DiurnalUpdateAlpha)*slot.Values[k]
	}
	slot.SampleCount++
	slot.LastUpdate = time.Now()
}

// GetActiveBaseline returns the blended baseline for the current time
// Uses crossfade between adjacent hourly slots based on minute of hour
// Returns: blendedBaseline, crossfadeWeight (0-1), slotsReady
func (db *DiurnalBaseline) GetActiveBaseline(emaBaseline []float64) ([]float64, float64, bool) {
	return db.GetActiveBaselineAt(time.Now(), emaBaseline)
}

// GetActiveBaselineAt returns the blended baseline for a specific timestamp.
// Uses a 15-minute EMA-to-diurnal crossfade at each hour boundary:
//   - For the first 15 minutes of the hour: blend from EMA baseline (frac=0) to diurnal slot (frac=1)
//   - After 15 minutes: use diurnal slot exclusively (frac=1.0)
//
// frac = secondsIntoHour / (DiurnalCrossfadeMinutes * 60), clamped to [0, 1].
// Returns: blendedBaseline, frac (0-1), diurnalReady
func (db *DiurnalBaseline) GetActiveBaselineAt(t time.Time, emaBaseline []float64) ([]float64, float64, bool) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	hour := t.Hour()
	minute := t.Minute()
	second := t.Second()

	currentSlot := db.slots[hour]

	// If slot not ready, fall back to EMA baseline
	if currentSlot.SampleCount < DiurnalMinSamples || len(emaBaseline) != db.nSub {
		result := make([]float64, db.nSub)
		copy(result, emaBaseline)
		return result, 0.0, false
	}

	// Seconds elapsed since the start of this hour
	secondsIntoHour := minute*60 + second
	crossfadeDuration := DiurnalCrossfadeMinutes * 60 // 15 * 60 = 900 seconds

	// Calculate crossfade weight: 0 at start, 1 after 15 minutes
	var frac float64
	if secondsIntoHour >= crossfadeDuration {
		frac = 1.0
	} else {
		frac = float64(secondsIntoHour) / float64(crossfadeDuration)
	}

	result := make([]float64, db.nSub)
	for k := 0; k < db.nSub && k < len(currentSlot.Values) && k < len(emaBaseline); k++ {
		result[k] = (1-frac)*emaBaseline[k] + frac*currentSlot.Values[k]
	}
	return result, frac, true
}

// GetActiveBaselineCosine returns the blended baseline using cosine crossfade
// frac_smooth = (1 - cos(pi * frac)) / 2 for perceptually smoother transition
func (db *DiurnalBaseline) GetActiveBaselineCosine(emaBaseline []float64) ([]float64, float64, bool) {
	return db.GetActiveBaselineCosineAt(time.Now(), emaBaseline)
}

// GetActiveBaselineCosineAt returns cosine-crossfaded baseline for a specific timestamp.
// Uses cosine interpolation for smoother transition between adjacent hour slots.
// frac = (minute + second/60) / 60  — linear position within hour.
// frac_smooth = (1 - cos(π * frac)) / 2  — cosine smoothing.
// Result = (1 - frac_smooth) * currentSlot + frac_smooth * nextSlot.
// Returns: blendedBaseline, fracSmooth (0-1), diurnalReady
func (db *DiurnalBaseline) GetActiveBaselineCosineAt(t time.Time, emaBaseline []float64) ([]float64, float64, bool) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	hour := t.Hour()
	minute := t.Minute()
	second := t.Second()

	// Get the current and next hour's slots
	currentSlot := db.slots[hour]
	nextHour := (hour + 1) % 24
	nextSlot := db.slots[nextHour]

	// Check if both slots are ready
	currentReady := currentSlot.SampleCount >= DiurnalMinSamples
	nextReady := nextSlot.SampleCount >= DiurnalMinSamples

	// If current slot is not ready, fall back to EMA baseline
	if !currentReady || len(emaBaseline) != db.nSub {
		result := make([]float64, db.nSub)
		copy(result, emaBaseline)
		return result, 0.0, false
	}

	// Calculate fractional position within the hour
	frac := (float64(minute) + float64(second)/60.0) / 60.0

	// Apply cosine smoothing
	fracSmooth := (1 - math.Cos(math.Pi*frac)) / 2

	// If next slot not ready or at hour start, use current slot exclusively
	if !nextReady || frac == 0.0 {
		result := make([]float64, db.nSub)
		copy(result, currentSlot.Values)
		return result, fracSmooth, true
	}

	// Blend: (1-fracSmooth) * currentSlot + fracSmooth * nextSlot
	result := make([]float64, db.nSub)
	for k := 0; k < db.nSub && k < len(currentSlot.Values) && k < len(nextSlot.Values); k++ {
		result[k] = (1-fracSmooth)*currentSlot.Values[k] + fracSmooth*nextSlot.Values[k]
	}
	return result, fracSmooth, true
}

// GetSlotConfidence returns the confidence level for a specific hour's slot
// Returns 0.0 if slot has no samples, approaches 1.0 as samples accumulate
func (db *DiurnalBaseline) GetSlotConfidence(hour int) float64 {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if hour < 0 || hour >= DiurnalSlots {
		return 0.0
	}

	slot := db.slots[hour]
	if slot.SampleCount == 0 {
		return 0.0
	}

	// Confidence based on sample count (caps at 1.0)
	conf := float64(slot.SampleCount) / float64(DiurnalMinSamples)
	if conf > 1.0 {
		conf = 1.0
	}

	// Reduce confidence for old slots (not updated in 7 days)
	age := time.Since(slot.LastUpdate)
	if age > DiurnalLearningDays*24*time.Hour {
		conf *= 0.5
	}

	return conf
}

// GetAllSlotConfidences returns confidence for all 24 slots
func (db *DiurnalBaseline) GetAllSlotConfidences() []float64 {
	db.mu.RLock()
	defer db.mu.RUnlock()

	confidences := make([]float64, DiurnalSlots)
	for i := 0; i < DiurnalSlots; i++ {
		slot := db.slots[i]
		if slot.SampleCount == 0 {
			confidences[i] = 0.0
			continue
		}

		conf := float64(slot.SampleCount) / float64(DiurnalMinSamples)
		if conf > 1.0 {
			conf = 1.0
		}

		// Age penalty
		age := time.Since(slot.LastUpdate)
		if age > DiurnalLearningDays*24*time.Hour {
			conf *= 0.5
		}

		confidences[i] = conf
	}
	return confidences
}

// GetOverallConfidence returns the overall diurnal baseline confidence
// This is the percentage of slots that have sufficient samples
func (db *DiurnalBaseline) GetOverallConfidence() float64 {
	db.mu.RLock()
	defer db.mu.RUnlock()

	ready := 0
	for i := 0; i < DiurnalSlots; i++ {
		if db.slots[i].SampleCount >= DiurnalMinSamples {
			ready++
		}
	}

	return float64(ready) / float64(DiurnalSlots)
}

// IsReady returns true if the diurnal baseline is ready for use
// Requires: 7+ days since first update AND all 24 slots have >= 100 samples
func (db *DiurnalBaseline) IsReady() bool {
	return db.IsReadyAt(time.Now())
}

// IsReadyAt checks readiness at a specific timestamp
func (db *DiurnalBaseline) IsReadyAt(t time.Time) bool {
	db.mu.RLock()
	defer db.mu.RUnlock()

	// Check time requirement: 7+ days since creation
	if t.Sub(db.created) < DiurnalLearningDays*24*time.Hour {
		return false
	}

	// Check sample requirement: all 24 slots must have >= DiurnalMinSamples
	for i := 0; i < DiurnalSlots; i++ {
		if db.slots[i].SampleCount < DiurnalMinSamples {
			return false
		}
	}

	return true
}

// CompositeConfidence returns the composite confidence score
// Components:
//   - baseline_age (0.3): staleness of current hour slot (0 if > 3 days stale)
//   - diurnal_progress (0.3): 0.0 before 7 days, interpolates to 1.0 at 14 days
//   - packet_rate (0.4): actual vs configured sample rate
func (db *DiurnalBaseline) CompositeConfidence(packetRateRatio float64) float64 {
	return db.CompositeConfidenceAt(time.Now(), packetRateRatio)
}

// CompositeConfidenceAt calculates composite confidence at a specific timestamp
func (db *DiurnalBaseline) CompositeConfidenceAt(t time.Time, packetRateRatio float64) float64 {
	db.mu.RLock()
	defer db.mu.RUnlock()

	// 1. Baseline age confidence (0.3 weight)
	// Staleness reduces confidence. If slot not updated for > 3 days, contribution = 0
	hour := t.Hour()
	slot := db.slots[hour]

	var baselineAgeConf float64
	if slot.SampleCount == 0 {
		baselineAgeConf = 0
	} else {
		age := t.Sub(slot.LastUpdate)
		if age > DiurnalStaleDays*24*time.Hour {
			baselineAgeConf = 0
		} else {
			// Linear degradation from 1.0 (fresh) to 0.0 (3 days stale)
			baselineAgeConf = 1.0 - float64(age)/float64(DiurnalStaleDays*24*time.Hour)
			if baselineAgeConf < 0 {
				baselineAgeConf = 0
			}
		}
	}

	// 2. Diurnal learning progress confidence (0.3 weight)
	// 0.0 before 7 days, interpolates to 1.0 at 14 days
	var diurnalProgConf float64
	elapsed := t.Sub(db.created)
	learningPeriod := DiurnalLearningDays * 24 * time.Hour
	rampPeriod := 7 * 24 * time.Hour // 7 more days to full confidence

	if elapsed < learningPeriod {
		diurnalProgConf = 0
	} else if elapsed < learningPeriod+rampPeriod {
		// Ramp from 0 to 1 over the 7-day period after learning
		diurnalProgConf = float64(elapsed-learningPeriod) / float64(rampPeriod)
	} else {
		diurnalProgConf = 1.0
	}

	// 3. Packet rate confidence (0.4 weight)
	// If packet rate is 80% of configured, confidence = 0.8
	// At 50%, confidence = 0.5
	packetRateConf := packetRateRatio
	if packetRateConf > 1.0 {
		packetRateConf = 1.0
	}
	if packetRateConf < 0 {
		packetRateConf = 0
	}

	// Composite: weighted average
	confidence := ConfidenceWeightBaselineAge*baselineAgeConf +
		ConfidenceWeightDiurnalProg*diurnalProgConf +
		ConfidenceWeightPacketRate*packetRateConf

	// Clamp to [0, 1]
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}

	return confidence
}

// DiurnalSnapshot represents a serializable snapshot of diurnal baseline state
type DiurnalSnapshot struct {
	LinkID     string
	Created    time.Time
	SlotValues [DiurnalSlots][]float64
	SlotCounts [DiurnalSlots]int
	SlotTimes  [DiurnalSlots]time.Time
}

// GetSnapshot returns a snapshot for persistence
func (db *DiurnalBaseline) GetSnapshot() *DiurnalSnapshot {
	db.mu.RLock()
	defer db.mu.RUnlock()

	snap := &DiurnalSnapshot{
		LinkID:  db.linkID,
		Created: db.created,
	}

	for i := 0; i < DiurnalSlots; i++ {
		snap.SlotValues[i] = make([]float64, db.nSub)
		copy(snap.SlotValues[i], db.slots[i].Values)
		snap.SlotCounts[i] = db.slots[i].SampleCount
		snap.SlotTimes[i] = db.slots[i].LastUpdate
	}

	return snap
}

// RestoreFromSnapshot restores diurnal baseline from a persisted snapshot
func (db *DiurnalBaseline) RestoreFromSnapshot(snap *DiurnalSnapshot) {
	db.mu.Lock()
	defer db.mu.Unlock()

	db.created = snap.Created

	for i := 0; i < DiurnalSlots; i++ {
		if len(snap.SlotValues[i]) == db.nSub {
			copy(db.slots[i].Values, snap.SlotValues[i])
		}
		db.slots[i].SampleCount = snap.SlotCounts[i]
		db.slots[i].LastUpdate = snap.SlotTimes[i]
	}
}

// Reset clears all slots
func (db *DiurnalBaseline) Reset() {
	db.mu.Lock()
	defer db.mu.Unlock()

	for i := 0; i < DiurnalSlots; i++ {
		for k := range db.slots[i].Values {
			db.slots[i].Values[k] = 0
		}
		db.slots[i].SampleCount = 0
	}
	db.created = time.Now()
}

// IsLearning returns true if the system is still in the 7-day learning phase
func (db *DiurnalBaseline) IsLearning() bool {
	return time.Since(db.created) < DiurnalLearningDays*24*time.Hour
}

// GetLearningProgress returns the learning progress as a percentage (0-100)
func (db *DiurnalBaseline) GetLearningProgress() float64 {
	elapsed := time.Since(db.created)
	total := DiurnalLearningDays * 24 * time.Hour
	progress := float64(elapsed) / float64(total) * 100
	if progress > 100 {
		progress = 100
	}
	return progress
}

// GetCreatedAt returns the creation time of the diurnal baseline
func (db *DiurnalBaseline) GetCreatedAt() time.Time {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.created
}

// DiurnalManager manages diurnal baselines for all links
type DiurnalManager struct {
	mu       sync.RWMutex
	diurnals map[string]*DiurnalBaseline // keyed by linkID
	nSub     int
}

// NewDiurnalManager creates a new diurnal baseline manager
func NewDiurnalManager(nSub int) *DiurnalManager {
	return &DiurnalManager{
		diurnals: make(map[string]*DiurnalBaseline),
		nSub:     nSub,
	}
}

// GetOrCreate returns the diurnal baseline for a link, creating if needed
func (dm *DiurnalManager) GetOrCreate(linkID string) *DiurnalBaseline {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if db, exists := dm.diurnals[linkID]; exists {
		return db
	}

	db := NewDiurnalBaseline(linkID, dm.nSub)
	dm.diurnals[linkID] = db
	return db
}

// Get returns the diurnal baseline for a link, or nil if not exists
func (dm *DiurnalManager) Get(linkID string) *DiurnalBaseline {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.diurnals[linkID]
}

// GetAllSnapshots returns snapshots of all diurnal baselines for persistence
func (dm *DiurnalManager) GetAllSnapshots() map[string]*DiurnalSnapshot {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	result := make(map[string]*DiurnalSnapshot)
	for linkID, db := range dm.diurnals {
		result[linkID] = db.GetSnapshot()
	}
	return result
}

// RestoreFromSnapshot restores a diurnal baseline from a snapshot
func (dm *DiurnalManager) RestoreFromSnapshot(linkID string, snap *DiurnalSnapshot) {
	db := dm.GetOrCreate(linkID)
	db.RestoreFromSnapshot(snap)
}

// Remove removes a diurnal baseline for a link
func (dm *DiurnalManager) Remove(linkID string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	delete(dm.diurnals, linkID)
}

// LinkCount returns the number of tracked links
func (dm *DiurnalManager) LinkCount() int {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return len(dm.diurnals)
}
