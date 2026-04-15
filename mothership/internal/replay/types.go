// Package replay provides time-travel debugging capabilities for CSI data.
//
// This file contains types shared across the replay package.
package replay

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"
)

// Session represents a time-travel replay session.
type Session struct {
	mu           sync.RWMutex
	id           string
	fromMS       int64
	toMS         int64
	currentMS    int64
	speed        int
	state        SessionState
	params       *TunableParams
	created_at   int64
	updated_at   int64
	ctx          context.Context
	cancel       context.CancelFunc
	stopCh       chan struct{}
}

// SessionState is the playback state of a session.
type SessionState string

const (
	StatePaused  SessionState = "paused"
	StatePlaying SessionState = "playing"
	StateStopped SessionState = "stopped"
)

// TunableParams holds pipeline parameters that can be tuned during replay.
type TunableParams struct {
	DeltaRMSThreshold     *float64 `json:"delta_rms_threshold,omitempty"`
	TauS                  *float64 `json:"tau_s,omitempty"`
	FresnelDecay          *float64 `json:"fresnel_decay,omitempty"`
	NSubcarriers          *int     `json:"n_subcarriers,omitempty"`
	BreathingSensitivity  *float64 `json:"breathing_sensitivity,omitempty"`
	FresnelWeightSigma    *float64 `json:"fresnel_weight_sigma,omitempty"`
	MinConfidence         *float64 `json:"min_confidence,omitempty"`
}

// NewSession creates a new replay session.
func NewSession(id string, fromMS, toMS int64) *Session {
	ctx, cancel := context.WithCancel(context.Background())
	return &Session{
		id:         id,
		fromMS:     fromMS,
		toMS:       toMS,
		currentMS:  fromMS,
		speed:      1,
		state:      StatePaused,
		params:     &TunableParams{},
		created_at: time.Now().UnixMilli(),
		updated_at: time.Now().UnixMilli(),
		ctx:        ctx,
		cancel:     cancel,
		stopCh:     make(chan struct{}),
	}
}

// ID returns the session ID.
func (s *Session) ID() string {
	return s.id
}

// CurrentMS returns the current playback position.
func (s *Session) CurrentMS() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentMS
}

// FromMS returns the session start timestamp in milliseconds.
func (s *Session) FromMS() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fromMS
}

// ToMS returns the session end timestamp in milliseconds.
func (s *Session) ToMS() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.toMS
}

// State returns the current session state.
func (s *Session) State() SessionState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// Speed returns the current playback speed.
func (s *Session) Speed() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.speed
}

// SetSpeed updates the playback speed without changing state.
func (s *Session) SetSpeed(speed int) error {
	if speed < 1 || speed > 5 {
		return fmt.Errorf("invalid speed: %d (must be 1-5)", speed)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.speed = speed
	s.updated_at = time.Now().UnixMilli()
	return nil
}

// Params returns the current tunable parameters.
func (s *Session) Params() *TunableParams {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.params
}

// SetParams updates the tunable parameters.
func (s *Session) SetParams(params *TunableParams) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.params = params
	s.updated_at = time.Now().UnixMilli()
}

// SeekTo moves the replay position to the target timestamp, clamping to session range.
func (s *Session) SeekTo(targetMS int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if targetMS < s.fromMS {
		targetMS = s.fromMS
	}
	if targetMS > s.toMS {
		targetMS = s.toMS
	}

	s.currentMS = targetMS
	s.updated_at = time.Now().UnixMilli()
	return nil
}

// Play starts playback at the specified speed.
func (s *Session) Play(speed int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if speed < 1 || speed > 5 {
		return fmt.Errorf("invalid speed: %d (must be 1-5)", speed)
	}

	s.state = StatePlaying
	s.speed = speed
	s.updated_at = time.Now().UnixMilli()

	// Start playback goroutine if not already running
	go s.playbackLoop()

	return nil
}

// Pause pauses playback.
func (s *Session) Pause() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state = StatePaused
	s.updated_at = time.Now().UnixMilli()
	return nil
}

// Stop stops playback and terminates the session.
func (s *Session) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state = StateStopped
	s.cancel()
	close(s.stopCh)
	s.updated_at = time.Now().UnixMilli()
	return nil
}

// playbackLoop is the main playback loop.
func (s *Session) playbackLoop() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.mu.Lock()
			if s.state != StatePlaying {
				s.mu.Unlock()
				continue
			}

			// Advance position based on speed
			dt := int64(100 * time.Millisecond.Milliseconds() * int64(s.speed))
			s.currentMS += dt

			// Check if we've reached the end
			if s.currentMS >= s.toMS {
				s.state = StatePaused
				s.currentMS = s.toMS
				s.mu.Unlock()
				return
			}

			s.updated_at = time.Now().UnixMilli()
			s.mu.Unlock()

			// Emit frames for the current window
			s.emitFrames()
		}
	}
}

// emitFrames reads and processes frames for the current position.
func (s *Session) emitFrames() {
	// This would read frames from the store and emit them
	// For now, it's a placeholder
}

// ToJSON converts the session to JSON for storage.
func (s *Session) ToJSON() (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data := map[string]interface{}{
		"id":         s.id,
		"state":      s.state,
		"from_ms":    s.fromMS,
		"to_ms":      s.toMS,
		"current_ms": s.currentMS,
		"speed":      s.speed,
		"created_at": s.created_at,
		"updated_at": s.updated_at,
	}

	if s.params != nil {
		data["params"] = s.params
	}

	bytes, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

// Errors
var (
	ErrSessionNotFound = &ReplayError{Code: "session_not_found", Message: "Session not found"}
	ErrInvalidTime     = &ReplayError{Code: "invalid_time", Message: "Invalid time range"}
)

// ReplayError represents a replay-specific error
type ReplayError struct {
	Code    string
	Message string
}

func (e *ReplayError) Error() string {
	return e.Message
}

// Helper functions for math operations
func clamp(v, min, max float64) float64 {
	return math.Max(min, math.Min(max, v))
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// BlobBroadcaster broadcasts replay blob results to dashboard clients.
type BlobBroadcaster interface {
	BroadcastReplayBlobs(blobs []BlobUpdate, timestampMS int64)
}

// BlobUpdate represents a blob position during replay.
type BlobUpdate struct {
	ID                 int       `json:"id"`
	X                  float64   `json:"x"`
	Y                  float64   `json:"y"`
	Z                  float64   `json:"z"`
	VX                 float64   `json:"vx"`
	VY                 float64   `json:"vy"`
	VZ                 float64   `json:"vz"`
	Weight             float64   `json:"weight"`
	Posture            string    `json:"posture,omitempty"`
	PersonID           string    `json:"person_id,omitempty"`
	PersonLabel        string    `json:"person_label,omitempty"`
	PersonColor        string    `json:"person_color,omitempty"`
	IdentityConfidence float64   `json:"identity_confidence,omitempty"`
	IdentitySource     string    `json:"identity_source,omitempty"`
	Trail              []float64 `json:"trail,omitempty"` // [x,z,x,z,...]
}
