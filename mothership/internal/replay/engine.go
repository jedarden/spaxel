// Package replay implements time-travel debugging for CSI data.
// It enables pausing the live 3D view, scrubbing through historical CSI data,
// and replaying the detection pipeline with adjustable parameters.
package replay

import (
	"fmt"
	"sync"

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

	// Validate time range
	oldest, newest, err := e.buffer.GetTimestampRange()
	if err != nil {
		return nil, fmt.Errorf("failed to get timestamp range: %w", err)
	}

	oldestMS := oldest.UnixMilli()
	newestMS := newest.UnixMilli()

	if oldestMS == 0 && newestMS == 0 {
		return nil, fmt.Errorf("no data available for replay")
	}

	if fromMS < oldestMS {
		fromMS = oldestMS
	}
	if toMS > newestMS {
		toMS = newestMS
	}
	if fromMS > toMS {
		fromMS, toMS = toMS, fromMS
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
