// Package replay implements time-travel debugging for CSI data.
// It enables pausing the live 3D view, scrubbing through historical CSI data,
// and replaying the detection pipeline with adjustable parameters.
package replay

import (
	"fmt"
	"sync"
	"time"

	"github.com/spaxel/mothership/internal/recording"
)

// Engine manages replay sessions and coordinates with the recording buffer.
type Engine struct {
	mu               sync.RWMutex
	sessions         map[string]*Session
	buffer           *recording.Buffer
	blobBroadcaster  BlobBroadcaster
	defaultParams    *TunableParams
	sessionIDCounter uint64
}

// NewEngine creates a new replay engine.
func NewEngine(buffer *recording.Buffer, broadcaster BlobBroadcaster) *Engine {
	return &Engine{
		sessions:        make(map[string]*Session),
		buffer:          buffer,
		blobBroadcaster: broadcaster,
		defaultParams: &TunableParams{
			DeltaRMSThreshold:     float64Ptr(0.02),
			TauS:                 float64Ptr(30.0),
			FresnelDecay:         float64Ptr(2.0),
			NSubcarriers:         intPtr(16),
			BreathingSensitivity: float64Ptr(0.005),
			MinConfidence:        float64Ptr(0.3),
		},
	}
}

// StartSession begins a new replay session.
func (e *Engine) StartSession(fromMS, toMS int64) (*Session, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Clamp range to available data if data exists.
	oldest, newest, err := e.buffer.GetTimestampRange()
	if err == nil {
		oldestMS := oldest.UnixMilli()
		newestMS := newest.UnixMilli()
		if fromMS < oldestMS {
			fromMS = oldestMS
		}
		if toMS > newestMS {
			toMS = newestMS
		}
	}

	if fromMS > toMS {
		return nil, fmt.Errorf("invalid range: fromMS %d > toMS %d", fromMS, toMS)
	}

	e.sessionIDCounter++
	sessionID := fmt.Sprintf("replay-%d", e.sessionIDCounter)

	sess := NewSession(sessionID, fromMS, toMS)

	e.sessions[sessionID] = sess
	return sess, nil
}

// GetSession retrieves a session by ID.
func (e *Engine) GetSession(id string) (*Session, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	sess, ok := e.sessions[id]
	return sess, ok
}

// StopSession stops and removes a session.
func (e *Engine) StopSession(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	sess, ok := e.sessions[id]
	if !ok {
		return fmt.Errorf("session not found: %s", id)
	}

	_ = sess.Stop()
	delete(e.sessions, id)
	return nil
}

// Seek moves a session to the specified timestamp.
func (e *Engine) Seek(id string, targetMS int64) error {
	e.mu.RLock()
	sess, ok := e.sessions[id]
	e.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session not found: %s", id)
	}
	return sess.SeekTo(targetMS)
}

// Play starts playback at the specified speed (float, rounded to int).
func (e *Engine) Play(id string, speed float64) error {
	e.mu.RLock()
	sess, ok := e.sessions[id]
	e.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session not found: %s", id)
	}
	s := int(speed)
	if s < 1 {
		s = 1
	}
	return sess.Play(s)
}

// Pause pauses playback for the session.
func (e *Engine) Pause(id string) error {
	e.mu.RLock()
	sess, ok := e.sessions[id]
	e.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session not found: %s", id)
	}
	return sess.Pause()
}

// SetSpeed updates the playback speed (float, rounded to int).
func (e *Engine) SetSpeed(id string, speed float64) error {
	e.mu.RLock()
	sess, ok := e.sessions[id]
	e.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session not found: %s", id)
	}
	s := int(speed)
	if s < 1 {
		s = 1
	}
	return sess.SetSpeed(s)
}

// SetParams updates the tunable parameters for a session, merging with existing params.
func (e *Engine) SetParams(id string, params *TunableParams) error {
	e.mu.RLock()
	sess, ok := e.sessions[id]
	e.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session not found: %s", id)
	}
	// Merge: start from defaults, then apply session's existing values, then new values
	merged := e.defaultParams.clone()
	current := sess.Params()
	if current != nil {
		if current.DeltaRMSThreshold != nil {
			merged.DeltaRMSThreshold = float64PtrCopy(current.DeltaRMSThreshold)
		}
		if current.TauS != nil {
			merged.TauS = float64PtrCopy(current.TauS)
		}
		if current.FresnelDecay != nil {
			merged.FresnelDecay = float64PtrCopy(current.FresnelDecay)
		}
		if current.NSubcarriers != nil {
			merged.NSubcarriers = intPtrCopy(current.NSubcarriers)
		}
		if current.BreathingSensitivity != nil {
			merged.BreathingSensitivity = float64PtrCopy(current.BreathingSensitivity)
		}
		if current.MinConfidence != nil {
			merged.MinConfidence = float64PtrCopy(current.MinConfidence)
		}
	}
	if params.DeltaRMSThreshold != nil {
		merged.DeltaRMSThreshold = float64PtrCopy(params.DeltaRMSThreshold)
	}
	if params.TauS != nil {
		merged.TauS = float64PtrCopy(params.TauS)
	}
	if params.FresnelDecay != nil {
		merged.FresnelDecay = float64PtrCopy(params.FresnelDecay)
	}
	if params.NSubcarriers != nil {
		merged.NSubcarriers = intPtrCopy(params.NSubcarriers)
	}
	if params.BreathingSensitivity != nil {
		merged.BreathingSensitivity = float64PtrCopy(params.BreathingSensitivity)
	}
	if params.MinConfidence != nil {
		merged.MinConfidence = float64PtrCopy(params.MinConfidence)
	}
	sess.SetParams(merged)
	return nil
}

// GetTimestampRange returns the available timestamp range in the recording buffer.
func (e *Engine) GetTimestampRange() (oldest, newest time.Time, err error) {
	oldest, newest, err = e.buffer.GetTimestampRange()
	if err != nil {
		return
	}
	if oldest.IsZero() && newest.IsZero() {
		err = fmt.Errorf("no data available")
	}
	return
}

// float64Ptr returns a pointer to a float64.
func float64Ptr(v float64) *float64 {
	return &v
}

// intPtr returns a pointer to an int.
func intPtr(v int) *int {
	return &v
}

// clone creates a deep copy of TunableParams.
func (p *TunableParams) clone() *TunableParams {
	if p == nil {
		return nil
	}
	return &TunableParams{
		DeltaRMSThreshold:     float64PtrCopy(p.DeltaRMSThreshold),
		TauS:                  float64PtrCopy(p.TauS),
		FresnelDecay:          float64PtrCopy(p.FresnelDecay),
		NSubcarriers:          intPtrCopy(p.NSubcarriers),
		BreathingSensitivity:  float64PtrCopy(p.BreathingSensitivity),
		MinConfidence:         float64PtrCopy(p.MinConfidence),
	}
}

func float64PtrCopy(p *float64) *float64 {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func intPtrCopy(p *int) *int {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
