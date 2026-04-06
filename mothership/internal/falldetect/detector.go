// Package falldetect provides fall detection using blob tracking data.
// Fall detection is a critical safety feature that analyses Z-position history
// to detect rapid descents followed by sustained stillness at floor level.
package falldetect

import (
	"fmt"
	"log"
	"math"
	"sync"
	"time"
)

// FallState represents the state of fall detection for a track.
type FallState int

const (
	// StateNormal: no fall detected
	StateNormal FallState = iota
	// StateDescentDetected: Z velocity trigger fired, monitoring for confirmation
	StateDescentDetected
	// StateFallConfirmed: post-fall stillness confirmed, alert active
	StateFallConfirmed
	// StateAlertAcknowledged: user acknowledged, monitoring continues
	StateAlertAcknowledged
)

func (s FallState) String() string {
	switch s {
	case StateDescentDetected:
		return "descent_detected"
	case StateFallConfirmed:
		return "fall_confirmed"
	case StateAlertAcknowledged:
		return "alert_acknowledged"
	default:
		return "normal"
	}
}

// FallEvent represents a detected fall.
type FallEvent struct {
	ID             string    `json:"id"`
	BlobID         int       `json:"blob_id"`
	Identity       string    `json:"identity,omitempty"`
	ZoneID         string    `json:"zone_id,omitempty"`
	ZoneName       string    `json:"zone_name,omitempty"`
	StartZ         float64   `json:"start_z"`         // Height before fall
	EndZ           float64   `json:"end_z"`           // Height after fall
	ZDrop          float64   `json:"z_drop"`          // Total Z drop in meters
	PeakVelocity   float64   `json:"peak_velocity"`   // Peak downward velocity (m/s)
	FallDuration   float64   `json:"fall_duration"`   // Time from descent to stillness (seconds)
	StillnessTime  float64   `json:"stillness_time"`  // Seconds of stillness after fall
	Confidence     float64   `json:"confidence"`
	Timestamp      time.Time `json:"timestamp"`
	Position       Position  `json:"position"`
	State          FallState `json:"state"`
	AcknowledgedAt time.Time `json:"acknowledged_at,omitempty"`
	Feedback       string    `json:"feedback,omitempty"` // "fine", "needed_help", "false_alarm"
}

// Position represents a 3D position.
type Position struct {
	X, Y, Z float64 `json:"x,y,z"`
}

// BlobSnapshot captures blob state at a point in time.
type BlobSnapshot struct {
	ID        int
	X, Y, Z   float64
	VX, VY, VZ float64
	Posture   string
	Timestamp time.Time
	DeltaRMS  float64 // Motion level from contributing links
}

// AlertLevel represents the escalation level of a fall alert.
type AlertLevel int

const (
	AlertLevelNone AlertLevel = iota
	AlertLevelInitial    // T+0s
	AlertLevelEscalated  // T+2min
	AlertLevelUrgent     // T+5min
)

// fallTrackState holds per-blob fall detection state.
type fallTrackState struct {
	state FallState

	// Descent detection
	descentStartTime time.Time
	descentStartZ    float64
	peakVelocity     float64

	// Fall confirmation
	fallConfirmTime  time.Time
	fallEndZ         float64
	stillnessStart   time.Time
	consecutiveStill int // Count of consecutive still samples

	// Alert tracking
	alertLevel      AlertLevel
	alertSentAt     time.Time
	escalationTimer *time.Timer

	// Acknowledgement
	acknowledged bool

	// History for posture-based suppression
	recentPostures []postureEntry
}

type postureEntry struct {
	posture  string
	time     time.Time
}

// Config holds fall detector configuration.
type Config struct {
	// Descent trigger thresholds
	DescentVelocityThreshold float64 // Z velocity threshold (m/s), default: -1.5
	DescentDropThreshold     float64 // Total Z drop in 1s (m), default: 0.8
	DescentWindow            float64 // Rolling window for velocity (seconds), default: 0.2

	// Post-fall confirmation
	FloorLevelThreshold   float64 // Z below this is "floor level" (m), default: 0.4
	StillnessMotionThreshold float64 // DeltaRMS below this is "still", default: 0.01
	StillnessTimeRequired    float64 // Seconds of stillness to confirm (s), default: 30

	// False positive suppression
	StandardDropThreshold  float64 // Normal drop threshold (m), default: 0.8
	ElevatedDropThreshold  float64 // Higher threshold if recent sitting/lying (m), default: 1.2
	PostureHistoryWindow   float64 // How far back to check posture (s), default: 600 (10 min)

	// Alert timing
	EscalationTime1 time.Duration // Time to first escalation, default: 2min
	EscalationTime2 time.Duration // Time to urgent escalation, default: 5min
	AlertCooldown   time.Duration // Don't re-alert for same blob, default: 5min
}

// DefaultConfig returns the default fall detection configuration.
func DefaultConfig() Config {
	return Config{
		DescentVelocityThreshold:  -1.5,
		DescentDropThreshold:      0.8,
		DescentWindow:             0.2,
		FloorLevelThreshold:       0.4,
		StillnessMotionThreshold:  0.01,
		StillnessTimeRequired:     30.0,
		StandardDropThreshold:     0.8,
		ElevatedDropThreshold:     1.2,
		PostureHistoryWindow:      600.0,
		EscalationTime1:           2 * time.Minute,
		EscalationTime2:           5 * time.Minute,
		AlertCooldown:             5 * time.Minute,
	}
}

// Detector analyzes blob tracking data for falls.
type Detector struct {
	mu     sync.RWMutex
	config Config

	// Per-blob tracking
	trackStates  map[int]*fallTrackState
	blobHistory  map[int][]BlobSnapshot

	// Active fall events (for acknowledgement)
	activeFalls   map[string]*FallEvent
	fallIDCounter int

	// Callbacks
	onFall        func(FallEvent)
	onEscalation  func(FallEvent, AlertLevel)
	identityFunc  func(blobID int) string
	zoneFunc      func(blobID int) (zoneID, zoneName string)
	childrenZoneFunc func(blobID int) bool // Returns true if blob is in children's zone
	deltaRMSFunc  func(blobID int) float64 // Gets motion level for blob

	// Alert tracking
	recentAlerts map[int]time.Time // blobID -> last alert time

	// Cleanup
	lastCleanup time.Time
}

// NewDetector creates a new fall detector with default configuration.
func NewDetector() *Detector {
	return NewDetectorWithConfig(DefaultConfig())
}

// NewDetectorWithConfig creates a new fall detector with custom configuration.
func NewDetectorWithConfig(config Config) *Detector {
	return &Detector{
		config:       config,
		trackStates:  make(map[int]*fallTrackState),
		blobHistory:  make(map[int][]BlobSnapshot),
		activeFalls:  make(map[string]*FallEvent),
		recentAlerts: make(map[int]time.Time),
		lastCleanup:  time.Now(),
	}
}

// SetOnFall sets callback for fall confirmation events.
func (d *Detector) SetOnFall(cb func(FallEvent)) {
	d.mu.Lock()
	d.onFall = cb
	d.mu.Unlock()
}

// SetOnEscalation sets callback for alert escalation events.
func (d *Detector) SetOnEscalation(cb func(FallEvent, AlertLevel)) {
	d.mu.Lock()
	d.onEscalation = cb
	d.mu.Unlock()
}

// SetIdentityFunc sets function to get identity for a blob.
func (d *Detector) SetIdentityFunc(fn func(blobID int) string) {
	d.mu.Lock()
	d.identityFunc = fn
	d.mu.Unlock()
}

// SetZoneFunc sets function to get zone info for a blob.
func (d *Detector) SetZoneFunc(fn func(blobID int) (zoneID, zoneName string)) {
	d.mu.Lock()
	d.zoneFunc = fn
	d.mu.Unlock()
}

// SetChildrenZoneFunc sets function to check if blob is in a children's zone.
func (d *Detector) SetChildrenZoneFunc(fn func(blobID int) bool) {
	d.mu.Lock()
	d.childrenZoneFunc = fn
	d.mu.Unlock()
}

// SetDeltaRMSFunc sets function to get motion level (deltaRMS) for a blob.
func (d *Detector) SetDeltaRMSFunc(fn func(blobID int) float64) {
	d.mu.Lock()
	d.deltaRMSFunc = fn
	d.mu.Unlock()
}

// Update processes new blob positions. This should be called at 10Hz.
func (d *Detector) Update(blobs []struct {
	ID       int
	X, Y, Z  float64
	VX, VY, VZ float64
	Posture  string
}, now time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Cleanup old data periodically
	if now.Sub(d.lastCleanup) > time.Minute {
		d.cleanup(now)
		d.lastCleanup = now
	}

	for _, blob := range blobs {
		d.processBlob(blob, now)
	}
}

// processBlob analyzes a single blob for fall patterns.
func (d *Detector) processBlob(blob struct {
	ID       int
	X, Y, Z  float64
	VX, VY, VZ float64
	Posture  string
}, now time.Time) {
	// Record snapshot
	snapshot := BlobSnapshot{
		ID:        blob.ID,
		X:         blob.X,
		Y:         blob.Y,
		Z:         blob.Z,
		VX:        blob.VX,
		VY:        blob.VY,
		VZ:        blob.VZ,
		Posture:   blob.Posture,
		Timestamp: now,
	}

	// Get deltaRMS if available
	if d.deltaRMSFunc != nil {
		snapshot.DeltaRMS = d.deltaRMSFunc(blob.ID)
	}

	history := d.blobHistory[blob.ID]
	history = append(history, snapshot)

	// Keep last 300 samples (at 10Hz = 30 seconds for full stillness confirmation)
	if len(history) > 300 {
		history = history[len(history)-300:]
	}
	d.blobHistory[blob.ID] = history

	// Get or create track state
	state, exists := d.trackStates[blob.ID]
	if !exists {
		state = &fallTrackState{
			state:          StateNormal,
			recentPostures: make([]postureEntry, 0),
		}
		d.trackStates[blob.ID] = state
	}

	// Update posture history
	state.recentPostures = append(state.recentPostures, postureEntry{
		posture: blob.Posture,
		time:    now,
	})
	// Keep posture history for the configured window
	cutoff := now.Add(-time.Duration(d.config.PostureHistoryWindow * float64(time.Second)))
	newPostures := make([]postureEntry, 0)
	for _, pe := range state.recentPostures {
		if pe.time.After(cutoff) {
			newPostures = append(newPostures, pe)
		}
	}
	state.recentPostures = newPostures

	// Check for children's zone suppression
	if d.childrenZoneFunc != nil && d.childrenZoneFunc(blob.ID) {
		// Suppress fall detection in children's zones
		return
	}

	// State machine
	switch state.state {
	case StateNormal:
		d.checkForDescent(blob, history, state, now)

	case StateDescentDetected:
		d.checkForFallConfirmation(blob, history, state, now)

	case StateFallConfirmed, StateAlertAcknowledged:
		// Continue monitoring for recovery
		d.checkForRecovery(blob, state, now)
	}
}

// checkForDescent detects the initial rapid descent that could indicate a fall.
func (d *Detector) checkForDescent(blob struct {
	ID       int
	X, Y, Z  float64
	VX, VY, VZ float64
	Posture  string
}, history []BlobSnapshot, state *fallTrackState, now time.Time) {
	if len(history) < 3 {
		return
	}

	// Calculate Z velocity over rolling window
	windowStart := now.Add(-time.Duration(d.config.DescentWindow * float64(time.Second)))
	var startZ, endZ float64
	var startFound, endFound bool

	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Timestamp.After(windowStart) || history[i].Timestamp.Equal(windowStart) {
			if !endFound {
				endZ = history[i].Z
				endFound = true
			}
			startZ = history[i].Z
			startFound = true
		} else {
			break
		}
	}

	if !startFound || !endFound {
		return
	}

	// Calculate velocity (positive VZ means downward in our coordinate system)
	zVelocity := blob.VZ // VZ is negative when falling (going down)

	// Calculate total Z drop in last 1 second
	oneSecondAgo := now.Add(-time.Second)
	var zOneSecondAgo float64
	var foundHistory bool
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Timestamp.Before(oneSecondAgo) {
			zOneSecondAgo = history[i].Z
			foundHistory = true
			break
		}
	}
	if !foundHistory && len(history) > 0 {
		zOneSecondAgo = history[0].Z
	}
	zDrop := zOneSecondAgo - blob.Z

	// Apply false positive suppression based on posture history
	dropThreshold := d.config.StandardDropThreshold
	if d.hasRecentSittingOrLying(state) {
		dropThreshold = d.config.ElevatedDropThreshold
	}

	// Check descent trigger conditions
	// Z velocity < -1.5 m/s (falling down) AND Z drop > threshold in 1s
	if zVelocity < d.config.DescentVelocityThreshold && zDrop > dropThreshold {
		state.state = StateDescentDetected
		state.descentStartTime = now
		state.descentStartZ = blob.Z + zDrop // Approximate starting height
		state.peakVelocity = zVelocity

		log.Printf("[DEBUG] Fall detection: descent triggered for blob %d (velocity=%.2f m/s, drop=%.2f m)",
			blob.ID, zVelocity, zDrop)
	}
}

// checkForFallConfirmation checks if the descent is followed by stillness at floor level.
func (d *Detector) checkForFallConfirmation(blob struct {
	ID       int
	X, Y, Z  float64
	VX, VY, VZ float64
	Posture  string
}, history []BlobSnapshot, state *fallTrackState, now time.Time) {
	// Check if person got up (Z rose above floor level)
	if blob.Z > d.config.FloorLevelThreshold+0.1 { // Small hysteresis
		// Person got up, reset to normal
		log.Printf("[DEBUG] Fall detection: blob %d got up, resetting", blob.ID)
		d.resetTrackState(state)
		return
	}

	// Check if fall window expired (took too long to become still)
	fallDuration := now.Sub(state.descentStartTime).Seconds()
	if fallDuration > 10.0 { // 10 second max for fall to complete
		log.Printf("[DEBUG] Fall detection: blob %d fall window expired", blob.ID)
		d.resetTrackState(state)
		return
	}

	// Check if at floor level
	if blob.Z >= d.config.FloorLevelThreshold {
		// Not at floor level yet
		return
	}

	// Get motion level
	deltaRMS := 0.0
	if d.deltaRMSFunc != nil {
		deltaRMS = d.deltaRMSFunc(blob.ID)
	}

	// Check for stillness
	if deltaRMS < d.config.StillnessMotionThreshold {
		if state.stillnessStart.IsZero() {
			state.stillnessStart = now
			state.consecutiveStill = 1
		} else {
			state.consecutiveStill++
		}

		stillnessTime := now.Sub(state.stillnessStart).Seconds()

		// Check if stillness duration requirement met
		if stillnessTime >= d.config.StillnessTimeRequired {
			// Fall confirmed!
			state.state = StateFallConfirmed
			state.fallConfirmTime = now
			state.fallEndZ = blob.Z

			// Trigger fall alert
			d.triggerFallAlert(blob, state, fallDuration, stillnessTime, now)
		}
	} else {
		// Movement detected, reset stillness counter
		state.stillnessStart = time.Time{}
		state.consecutiveStill = 0
	}
}

// checkForRecovery monitors a confirmed fall for recovery (person gets up).
func (d *Detector) checkForRecovery(blob struct {
	ID       int
	X, Y, Z  float64
	VX, VY, VZ float64
	Posture  string
}, state *fallTrackState, now time.Time) {
	// If person rises significantly, they've recovered
	if blob.Z > d.config.FloorLevelThreshold+0.3 {
		log.Printf("[INFO] Fall detection: blob %d appears to have recovered (Z=%.2f)", blob.ID, blob.Z)
		// Don't reset immediately - keep monitoring in case they fall again
	}
}

// hasRecentSittingOrLying checks if the person has been sitting or lying recently.
func (d *Detector) hasRecentSittingOrLying(state *fallTrackState) bool {
	for _, pe := range state.recentPostures {
		if pe.posture == "seated" || pe.posture == "lying" {
			return true
		}
	}
	return false
}

// triggerFallAlert fires the fall alert and sets up escalation timers.
func (d *Detector) triggerFallAlert(blob struct {
	ID       int
	X, Y, Z  float64
	VX, VY, VZ float64
	Posture  string
}, state *fallTrackState, fallDuration, stillnessTime float64, now time.Time) {
	// Check cooldown
	if lastAlert, exists := d.recentAlerts[blob.ID]; exists {
		if now.Sub(lastAlert) < d.config.AlertCooldown {
			log.Printf("[DEBUG] Fall alert for blob %d suppressed by cooldown", blob.ID)
			return
		}
	}

	// Generate event ID
	d.fallIDCounter++
	eventID := fmt.Sprintf("fall_%d_%d", now.Unix(), d.fallIDCounter)

	// Calculate confidence
	confidence := d.calculateConfidence(state, stillnessTime)

	// Build event
	event := FallEvent{
		ID:            eventID,
		BlobID:        blob.ID,
		StartZ:        state.descentStartZ,
		EndZ:          blob.Z,
		ZDrop:         state.descentStartZ - blob.Z,
		PeakVelocity:  state.peakVelocity,
		FallDuration:  fallDuration,
		StillnessTime: stillnessTime,
		Confidence:    confidence,
		Timestamp:     now,
		State:         StateFallConfirmed,
		Position:      Position{X: blob.X, Y: blob.Y, Z: blob.Z},
	}

	// Get identity
	if d.identityFunc != nil {
		event.Identity = d.identityFunc(blob.ID)
	}

	// Get zone
	if d.zoneFunc != nil {
		event.ZoneID, event.ZoneName = d.zoneFunc(blob.ID)
	}

	// Store as active fall
	d.activeFalls[eventID] = &event

	// Record alert time
	d.recentAlerts[blob.ID] = now
	state.alertSentAt = now
	state.alertLevel = AlertLevelInitial

	// Set up escalation timers
	d.setupEscalationTimers(state, &event, now)

	// Fire callback
	if d.onFall != nil {
		go d.onFall(event)
	}

	log.Printf("[WARN] FALL CONFIRMED: blob=%d identity=%s confidence=%.2f at (%.2f, %.2f, %.2f) zone=%s",
		blob.ID, event.Identity, confidence, blob.X, blob.Y, blob.Z, event.ZoneName)
}

// calculateConfidence computes confidence score for a fall detection.
func (d *Detector) calculateConfidence(state *fallTrackState, stillnessTime float64) float64 {
	confidence := 0.5

	// Stronger descent = higher confidence
	zDrop := state.descentStartZ - state.fallEndZ
	if zDrop > 1.2 {
		confidence += 0.2
	} else if zDrop > 1.0 {
		confidence += 0.15
	} else if zDrop > 0.8 {
		confidence += 0.1
	}

	// Higher velocity = higher confidence
	if state.peakVelocity < -2.0 {
		confidence += 0.15
	} else if state.peakVelocity < -1.5 {
		confidence += 0.1
	}

	// Longer stillness = higher confidence
	if stillnessTime > 60 {
		confidence += 0.15
	} else if stillnessTime > 45 {
		confidence += 0.1
	} else if stillnessTime > 30 {
		confidence += 0.05
	}

	// Cap at 1.0
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

// setupEscalationTimers sets up the T+2min and T+5min escalation timers.
func (d *Detector) setupEscalationTimers(state *fallTrackState, event *FallEvent, now time.Time) {
	// T+2min escalation
	time.AfterFunc(d.config.EscalationTime1, func() {
		d.mu.Lock()
		defer d.mu.Unlock()

		if state.state == StateFallConfirmed && !state.acknowledged {
			state.alertLevel = AlertLevelEscalated
			log.Printf("[WARN] Fall alert escalated (T+2min) for blob %d", event.BlobID)

			if d.onEscalation != nil {
				d.onEscalation(*event, AlertLevelEscalated)
			}
		}
	})

	// T+5min urgent escalation
	time.AfterFunc(d.config.EscalationTime2, func() {
		d.mu.Lock()
		defer d.mu.Unlock()

		if state.state == StateFallConfirmed && !state.acknowledged {
			state.alertLevel = AlertLevelUrgent
			log.Printf("[WARN] Fall alert URGENT (T+5min) for blob %d", event.BlobID)

			if d.onEscalation != nil {
				d.onEscalation(*event, AlertLevelUrgent)
			}
		}
	})
}

// AcknowledgeFall acknowledges a fall alert, canceling escalation timers.
func (d *Detector) AcknowledgeFall(eventID string, feedback string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	event, exists := d.activeFalls[eventID]
	if !exists {
		return fmt.Errorf("fall event not found: %s", eventID)
	}

	state, exists := d.trackStates[event.BlobID]
	if !exists {
		return fmt.Errorf("track state not found for blob %d", event.BlobID)
	}

	state.state = StateAlertAcknowledged
	state.acknowledged = true
	event.State = StateAlertAcknowledged
	event.AcknowledgedAt = time.Now()
	event.Feedback = feedback

	log.Printf("[INFO] Fall alert acknowledged: event=%s feedback=%s", eventID, feedback)

	return nil
}

// GetActiveFalls returns all active (unacknowledged) fall events.
func (d *Detector) GetActiveFalls() []FallEvent {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var falls []FallEvent
	for _, event := range d.activeFalls {
		if event.State == StateFallConfirmed {
			falls = append(falls, *event)
		}
	}
	return falls
}

// GetFallEvent returns a specific fall event by ID.
func (d *Detector) GetFallEvent(eventID string) (*FallEvent, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	event, exists := d.activeFalls[eventID]
	if !exists {
		return nil, fmt.Errorf("fall event not found: %s", eventID)
	}
	return event, nil
}

// GetTrackState returns the current fall detection state for a blob.
func (d *Detector) GetTrackState(blobID int) FallState {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if state, exists := d.trackStates[blobID]; exists {
		return state.state
	}
	return StateNormal
}

// ResetBlob resets detection state for a blob.
func (d *Detector) ResetBlob(blobID int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if state, exists := d.trackStates[blobID]; exists {
		d.resetTrackState(state)
	}
	delete(d.blobHistory, blobID)
}

// resetTrackState resets a track state to normal.
func (d *Detector) resetTrackState(state *fallTrackState) {
	state.state = StateNormal
	state.descentStartTime = time.Time{}
	state.descentStartZ = 0
	state.peakVelocity = 0
	state.fallConfirmTime = time.Time{}
	state.fallEndZ = 0
	state.stillnessStart = time.Time{}
	state.consecutiveStill = 0
	state.alertLevel = AlertLevelNone
	state.alertSentAt = time.Time{}
	state.acknowledged = false
}

// cleanup removes stale blob data.
func (d *Detector) cleanup(now time.Time) {
	// Remove history for blobs without recent state
	for blobID := range d.blobHistory {
		if _, exists := d.trackStates[blobID]; !exists {
			delete(d.blobHistory, blobID)
		}
	}

	// Remove old alerts from cooldown tracking
	for blobID, t := range d.recentAlerts {
		if now.Sub(t) > d.config.AlertCooldown*2 {
			delete(d.recentAlerts, blobID)
		}
	}

	// Clean up old fall events (keep for 24 hours for logging)
	cutoff := now.Add(-24 * time.Hour)
	for id, event := range d.activeFalls {
		if event.Timestamp.Before(cutoff) {
			delete(d.activeFalls, id)
		}
	}
}
