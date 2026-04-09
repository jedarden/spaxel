// Package replay implements time-travel debugging for CSI data.
// It enables pausing the live 3D view, scrubbing through historical CSI data,
// and replaying the detection pipeline with adjustable parameters.
package replay

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/spaxel/mothership/internal/recording"
)

// State represents the replay state machine.
type State int

const (
	StateStopped State = iota
	StatePaused
	StatePlaying
	StateSeeking
)

func (s State) String() string {
	switch s {
	case StateStopped:
		return "stopped"
	case StatePaused:
		return "paused"
	case StatePlaying:
		return "playing"
	case StateSeeking:
		return "seeking"
	default:
		return "unknown"
	}
}

// TunableParams holds adjustable signal processing parameters for replay.
type TunableParams struct {
	DeltaRMSThreshold     *float64 // Motion detection threshold (default 0.02)
	TauS                  *float64 // Baseline EMA time constant in seconds (default 30)
	FresnelDecay          *float64 // Fresnel zone weight decay rate (default 2.0)
	NSubcarriers          *int     // Number of subcarriers to use (default 16)
	BreathingSensitivity  *float64 // Breathing band sensitivity (default 0.005)
	MinConfidence         *float64 // Minimum confidence for blob reporting (default 0.3)
}

// Session represents a single replay session (per dashboard client).
type Session struct {
	ID        string
	State     State
	FromMS    int64
	ToMS      int64
	CurrentMS int64
	Speed     float64
	Params    *TunableParams

	mu               sync.Mutex
	pipeline         *Pipeline
	blobBroadcaster  BlobBroadcaster
	buffer           *recording.Buffer
	stopCh           chan struct{}
}

// BlobBroadcaster is the interface for broadcasting replay blob updates.
type BlobBroadcaster interface {
	BroadcastReplayBlobs(blobs []BlobUpdate, timestampMS int64)
}

// BlobUpdate represents a blob position update during replay.
type BlobUpdate struct {
	ID                 int      `json:"id"`
	X                  float64  `json:"x"`
	Z                  float64  `json:"z"`
	VX                 float64  `json:"vx"`
	VZ                 float64  `json:"vz"`
	Weight             float64  `json:"weight"`
	Trail              []float64 `json:"trail"` // Flat [x,z,x,z,...]
	Posture            string    `json:"posture,omitempty"`
	PersonID           string    `json:"person_id,omitempty"`
	PersonLabel        string    `json:"person_label,omitempty"`
	PersonColor        string    `json:"person_color,omitempty"`
	IdentityConfidence float64  `json:"identity_confidence,omitempty"`
	IdentitySource     string    `json:"identity_source,omitempty"`
}

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
			DeltaRMSThreshold:    float64Ptr(0.02),
			TauS:                float64Ptr(30.0),
			FresnelDecay:        float64Ptr(2.0),
			NSubcarriers:        intPtr(16),
			BreathingSensitivity: float64Ptr(0.005),
			MinConfidence:       float64Ptr(0.3),
		},
	}
}

// StartSession begins a new replay session.
func (e *Engine) StartSession(fromMS, toMS int64) (*Session, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Verify the requested range is available
	oldest, newest, err := e.buffer.GetTimestampRange()
	if err != nil {
		return nil, fmt.Errorf("failed to get timestamp range: %w", err)
	}

	oldestMS := oldest.UnixMilli()
	newestMS := newest.UnixMilli()

	// Clamp requested range to available data
	if fromMS < oldestMS {
		fromMS = oldestMS
	}
	if toMS > newestMS {
		toMS = newestMS
	}
	if fromMS > toMS {
		return nil, errors.New("invalid time range: from > to")
	}

	// Generate session ID
	e.sessionIDCounter++
	sessionID := fmt.Sprintf("replay-%d", e.sessionIDCounter)

	// Start paused at the beginning of the range
	session := &Session{
		ID:       sessionID,
		State:    StatePaused,
		FromMS:   fromMS,
		ToMS:     toMS,
		CurrentMS: fromMS,
		Speed:    1.0,
		Params:   e.defaultParams,
		buffer:   e.buffer,
		blobBroadcaster: e.blobBroadcaster,
		stopCh:   make(chan struct{}),
	}

	// Create replay pipeline
	session.pipeline = NewPipeline(session.Params, e.blobBroadcaster)

	e.sessions[sessionID] = session

	log.Printf("[REPLAY] Started session %s: %d to %d (available: %d to %d)",
		sessionID, fromMS, toMS, oldestMS, newestMS)

	return session, nil
}

// StopSession stops and removes a replay session.
func (e *Engine) StopSession(sessionID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	session, ok := e.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	close(session.stopCh)

	if session.State == StatePlaying {
		session.pipeline.Stop()
	}

	session.State = StateStopped
	delete(e.sessions, sessionID)

	log.Printf("[REPLAY] Stopped session %s", sessionID)
	return nil
}

// Seek moves a session to a specific timestamp.
func (e *Engine) Seek(sessionID string, targetMS int64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	session, ok := e.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	// Clamp target to session range
	if targetMS < session.FromMS {
		targetMS = session.FromMS
	}
	if targetMS > session.ToMS {
		targetMS = session.ToMS
	}

	// Stop current playback if playing
	if session.State == StatePlaying {
		session.pipeline.Stop()
		// Signal stop to playback worker
		select {
		case session.stopCh <- struct{}{}:
		default:
		}
		session.State = StateSeeking
	}

	// Seek in the recording buffer
	targetTime := time.Unix(0, targetMS*1_000_000).UTC()
	frame, frameTS, err := session.buffer.SeekToTimestamp(targetTime)
	if err != nil {
		return fmt.Errorf("seek failed: %w", err)
	}

	// Update current position
	session.CurrentMS = frameTS
	session.State = StatePaused

	// Process the single frame to update the display
	if session.pipeline != nil {
		session.pipeline.ProcessFrame(frame, frameTS)
	}

	log.Printf("[REPLAY] Session %s seeked to %d (found frame at %d)", sessionID, targetMS, frameTS)
	return nil
}

// Play starts playback at the specified speed.
func (e *Engine) Play(sessionID string, speed float64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	session, ok := e.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	if session.State == StatePlaying {
		// Already playing, just update speed
		session.pipeline.SetSpeed(speed)
		session.Speed = speed
		return nil
	}

	// Start playback from current position
	session.State = StatePlaying
	session.Speed = speed

	// Start the pipeline worker
	go session.playbackWorker()

	log.Printf("[REPLAY] Session %s playing at %.1fx speed", sessionID, speed)
	return nil
}

// Pause pauses playback.
func (e *Engine) Pause(sessionID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	session, ok := e.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	if session.State != StatePlaying {
		return nil // Already paused
	}

	session.State = StatePaused

	// Signal stop to playback worker
	select {
	case session.stopCh <- struct{}{}:
	default:
	}

	if session.pipeline != nil {
		session.pipeline.Stop()
	}

	log.Printf("[REPLAY] Session %s paused", sessionID)
	return nil
}

// SetParams updates the tunable parameters for a session.
// The pipeline will re-process from the current position with new parameters.
func (e *Engine) SetParams(sessionID string, params *TunableParams) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	session, ok := e.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	// Merge with existing params
	if session.Params == nil {
		session.Params = &TunableParams{}
	}
	if params.DeltaRMSThreshold != nil {
		session.Params.DeltaRMSThreshold = params.DeltaRMSThreshold
	}
	if params.TauS != nil {
		session.Params.TauS = params.TauS
	}
	if params.FresnelDecay != nil {
		session.Params.FresnelDecay = params.FresnelDecay
	}
	if params.NSubcarriers != nil {
		session.Params.NSubcarriers = params.NSubcarriers
	}
	if params.BreathingSensitivity != nil {
		session.Params.BreathingSensitivity = params.BreathingSensitivity
	}
	if params.MinConfidence != nil {
		session.Params.MinConfidence = params.MinConfidence
	}

	// Recreate pipeline with new params
	wasPlaying := session.State == StatePlaying
	if wasPlaying {
		session.pipeline.Stop()
		// Signal stop to playback worker
		select {
		case session.stopCh <- struct{}{}:
		default:
		}
	}

	session.pipeline = NewPipeline(session.Params, e.blobBroadcaster)

	// Re-process a window around current position
	go session.reprocessWindow()

	log.Printf("[REPLAY] Session %s params updated, reprocessing from %d", sessionID, session.CurrentMS)
	return nil
}

// SetSpeed changes the playback speed without stopping/starting.
func (e *Engine) SetSpeed(sessionID string, speed float64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	session, ok := e.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	session.Speed = speed
	if session.State == StatePlaying && session.pipeline != nil {
		session.pipeline.SetSpeed(speed)
	}

	return nil
}

// ApplyToLive copies the current replay parameters to the live configuration.
// This is a placeholder - the actual implementation would update the live
// signal processing configuration.
func (e *Engine) ApplyToLive(sessionID string) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	session, ok := e.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// This would trigger a callback to update the live configuration
	// For now, just log the action
	session.mu.Lock()
	params := session.Params
	session.mu.Unlock()

	log.Printf("[REPLAY] Apply to live requested from session %s: %+v", sessionID, params)

	// TODO: Implement live parameter update via callback interface
	return nil
}

// GetSession returns a session by ID.
func (e *Engine) GetSession(sessionID string) (*Session, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	s, ok := e.sessions[sessionID]
	return s, ok
}

// GetTimestampRange returns the available timestamp range in the recording buffer.
func (e *Engine) GetTimestampRange() (oldest, newest time.Time, err error) {
	return e.buffer.GetTimestampRange()
}

// playbackWorker runs the playback loop for a session.
func (s *Session) playbackWorker() {
	defer func() {
		s.mu.Lock()
		if s.State == StatePlaying {
			s.State = StatePaused
		}
		s.mu.Unlock()
	}()

	const bufferSize = 100 // Number of frames to buffer ahead

	frames := make([][]byte, 0, bufferSize)
	timestamps := make([]int64, 0, bufferSize)

	// Scan from current position to buffer ahead
	fromTime := time.Unix(0, s.CurrentMS*1_000_000).UTC()
	toTime := time.Unix(0, s.ToMS*1_000_000).UTC()

	err := s.buffer.ScanRange(fromTime, toTime, func(recvTimeNS int64, frame []byte) bool {
		if len(frames) < bufferSize {
			frames = append(frames, frame)
			timestamps = append(timestamps, recvTimeNS)
			return true
		}
		return false // Stop when buffer is full
	})

	if err != nil {
		log.Printf("[REPLAY] Scan error in playback worker: %v", err)
		return
	}

	if len(frames) == 0 {
		log.Printf("[REPLAY] No frames to play in session %s", s.ID)
		return
	}

	// Play frames at the specified speed
	startTime := time.Now()
	for i, frame := range frames {
		s.mu.Lock()

		// Check if we should stop
		select {
		case <-s.stopCh:
			s.mu.Unlock()
			return
		default:
		}

		if s.State != StatePlaying {
			s.mu.Unlock()
			return
		}

		s.CurrentMS = timestamps[i]

		// Calculate delay based on speed
		delay := time.Duration(0)
		if i > 0 && s.Speed > 0 {
			realDelta := time.Duration(timestamps[i]-timestamps[i-1]) * time.Nanosecond
			delay = time.Duration(float64(realDelta) / s.Speed)
			if delay > 0 && delay < 10*time.Second {
				// Release lock while sleeping
				s.mu.Unlock()
				time.Sleep(delay)
				s.mu.Lock()

				// Re-check state after sleep
				if s.State != StatePlaying {
					s.mu.Unlock()
					return
				}
			}
		}
		s.mu.Unlock()

		// Process the frame
		s.pipeline.ProcessFrame(frame, timestamps[i])
	}

	elapsed := time.Since(startTime)
	log.Printf("[REPLAY] Session %s played %d frames in %v", s.ID, len(frames), elapsed)
}

// reprocessWindow re-processes a window of CSI frames around the current position
// with updated parameters. This provides instant feedback when sliders change.
func (s *Session) reprocessWindow() {
	const windowDuration = 60 * time.Second // 60 seconds of data

	windowStart := time.Unix(0, s.CurrentMS*1_000_000).Add(-windowDuration/2).UTC()
	windowEnd := time.Unix(0, s.CurrentMS*1_000_000).Add(windowDuration/2).UTC()

	// Clamp to session bounds
	if windowStart.Before(time.Unix(0, s.FromMS*1_000_000).UTC()) {
		windowStart = time.Unix(0, s.FromMS*1_000_000).UTC()
	}
	if windowEnd.After(time.Unix(0, s.ToMS*1_000_000).UTC()) {
		windowEnd = time.Unix(0, s.ToMS*1_000_000).UTC()
	}

	startTime := time.Now()
	frameCount := 0

	// Scan and process frames as fast as possible (no real-time delay)
	s.buffer.ScanRange(windowStart, windowEnd, func(recvTimeNS int64, frame []byte) bool {
		s.pipeline.ProcessFrame(frame, recvTimeNS)
		frameCount++
		return true
	})

	elapsed := time.Since(startTime)
	log.Printf("[REPLAY] Session %s reprocessed %d frames in %v", s.ID, frameCount, elapsed)
}

// Helper functions for pointer creation
func float64Ptr(v float64) *float64 {
	return &v
}

func intPtr(v int) *int {
	return &v
}

// MarshalJSON implements JSON marshaling for TunableParams.
func (p *TunableParams) MarshalJSON() ([]byte, error) {
	obj := make(map[string]interface{})
	if p.DeltaRMSThreshold != nil {
		obj["delta_rms_threshold"] = *p.DeltaRMSThreshold
	}
	if p.TauS != nil {
		obj["tau_s"] = *p.TauS
	}
	if p.FresnelDecay != nil {
		obj["fresnel_decay"] = *p.FresnelDecay
	}
	if p.NSubcarriers != nil {
		obj["n_subcarriers"] = *p.NSubcarriers
	}
	if p.BreathingSensitivity != nil {
		obj["breathing_sensitivity"] = *p.BreathingSensitivity
	}
	if p.MinConfidence != nil {
		obj["min_confidence"] = *p.MinConfidence
	}
	return json.Marshal(obj)
}
