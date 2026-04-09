// Package replay provides time-travel debugging capabilities for CSI data.
//
// This file implements replay sessions that allow:
// - Seeking to specific timestamps
// - Playing at variable speeds (1x, 2x, 5x)
// - Pausing/resuming
// - Parameter tuning with instant preview
package replay

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sync"
	"time"
)

// Session represents a time-travel replay session.
type Session struct {
	mu           sync.RWMutex
	id           string
	store        *RecordingStore
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
	DeltaRMSThreshold  *float64 `json:"delta_rms_threshold,omitempty"`
	TauS               *float64 `json:"tau_s,omitempty"`
	FresnelDecay       *float64 `json:"fresnel_decay,omitempty"`
	NSubcarriers       *int     `json:"n_subcarriers,omitempty"`
	BreathingSensitivity *float64 `json:"breathing_sensitivity,omitempty"`
}

// NewSession creates a new replay session.
func NewSession(id string, store *RecordingStore, fromMS, toMS int64) *Session {
	ctx, cancel := context.WithCancel(context.Background())
	return &Session{
		id:         id,
		store:      store,
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

// Seek moves the playback position to the specified timestamp.
func (s *Session) Seek(targetMS int64) error {
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

	s.speed = speed
	s.state = StatePlaying
	s.updated_at = time.Now().UnixMilli()
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

// Stop stops playback and resets to the beginning.
func (s *Session) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state = StateStopped
	s.currentMS = s.fromMS
	s.cancel()
	s.updated_at = time.Now().UnixMilli()
	return nil
}

// Context returns the session's context for cancellation.
func (s *Session) Context() context.Context {
	return s.ctx
}

// GetFramesInRange returns all frames in the specified time range.
func (s *Session) GetFramesInRange(startMS, endMS int64) []Frame {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var frames []Frame
	s.store.Scan(func(recvTimeNS int64, rawFrame []byte) bool {
		recvMS := recvTimeNS / 1e6
		if recvMS < startMS {
			return true
		}
		if recvMS > endMS {
			return false
		}
		frames = append(frames, Frame{
			RecvTimeNS: recvTimeNS,
			Data:       rawFrame,
		})
		return true
	})
	return frames
}

// Frame represents a single CSI frame with its timestamp.
type Frame struct {
	RecvTimeNS int64
	Data       []byte
}

// SessionManager manages multiple replay sessions.
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	store    *RecordingStore
}

// NewSessionManager creates a new session manager.
func NewSessionManager(store *RecordingStore) *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
		store:    store,
	}
}

// CreateSession creates a new replay session.
func (m *SessionManager) CreateSession(id string, fromMS, toMS int64) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.sessions[id]; exists {
		return nil, fmt.Errorf("session %s already exists", id)
	}

	session := NewSession(id, m.store, fromMS, toMS)
	m.sessions[id] = session
	log.Printf("[INFO] Replay session %s created: %d ms to %d ms", id, fromMS, toMS)
	return session, nil
}

// GetSession returns a session by ID.
func (m *SessionManager) GetSession(id string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

// DeleteSession deletes a session.
func (m *SessionManager) DeleteSession(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[id]
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}

	session.Stop()
	delete(m.sessions, id)
	log.Printf("[INFO] Replay session %s deleted", id)
	return nil
}

// ListSessions returns all active sessions.
func (m *SessionManager) ListSessions() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessions := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	return sessions
}

// CleanExpiredSessions removes sessions that have been inactive for more than the specified duration.
func (m *SessionManager) CleanExpiredSessions(inactiveDuration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UnixMilli()
	for id, s := range m.sessions {
		if now-s.updated_at > inactiveDuration.Milliseconds() {
			s.Stop()
			delete(m.sessions, id)
			log.Printf("[INFO] Replay session %s expired and deleted", id)
		}
	}
}

// ToJSON serializes the session to JSON.
func (s *Session) ToJSON() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{}{
		"id":         s.id,
		"from_ms":    s.fromMS,
		"to_ms":      s.toMS,
		"current_ms": s.currentMS,
		"speed":      s.speed,
		"state":      string(s.state),
		"params":     s.params,
		"created_at": s.created_at,
		"updated_at": s.updated_at,
	}
}

// Stats returns statistics about the replay store.
func (s *Session) Stats() StoreStats {
	stats := s.store.Stats()
	return StoreStats{
		HasData:   stats.HasData,
		WritePos:  stats.WritePos,
		OldestPos: stats.OldestPos,
		FileSize:  stats.FileSize,
	}
}

// StoreStats contains statistics about the replay store.
type StoreStats struct {
	HasData   bool  `json:"has_data"`
	WritePos  int64 `json:"write_pos"`
	OldestPos int64 `json:"oldest_pos"`
	FileSize  int64 `json:"file_size"`
}

// SessionStats represents statistics for a session.
type SessionStats struct {
	ID            string      `json:"id"`
	State         SessionState `json:"state"`
	CurrentMS     int64       `json:"current_ms"`
	FromMS        int64       `json:"from_ms"`
	ToMS          int64       `json:"to_ms"`
	DurationMS    int64       `json:"duration_ms"`
	Progress      float64     `json:"progress"`
	Speed         int         `json:"speed"`
	StoreStats    StoreStats  `json:"store_stats"`
}

// GetStats returns statistics for the session.
func (s *Session) GetStats() SessionStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	duration := s.toMS - s.fromMS
	progress := 0.0
	if duration > 0 {
		progress = float64(s.currentMS-s.fromMS) / float64(duration)
	}

	return SessionStats{
		ID:         s.id,
		State:      s.state,
		CurrentMS:  s.currentMS,
		FromMS:     s.fromMS,
		ToMS:       s.toMS,
		DurationMS: duration,
		Progress:   math.Round(progress*10000) / 10000,
		Speed:      s.speed,
		StoreStats: s.Stats(),
	}
}

// MarshalJSON implements json.Marshaler for SessionStats.
func (s SessionStats) MarshalJSON() ([]byte, error) {
	type Alias SessionStats
	return json.Marshal(struct {
		Progress float64 `json:"progress"`
		Alias
	}{
		Progress: math.Round(s.Progress*10000) / 100,
		Alias:    (Alias)(s),
	})
}
