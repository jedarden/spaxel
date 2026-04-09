// Package replay implements time-travel debugging for CSI data.
// It provides a replay engine that can seek to any point in the recording
// buffer and replay CSI frames through a separate signal processing pipeline.
package replay

import (
	"sync"
	"time"
)

// State represents the current replay state
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

// Session represents a single replay session
type Session struct {
	ID            string
	State         State
	ReplayPos     time.Time
	ReplaySpeed   float64
	From          time.Time
	To            time.Time
	Params        *TunableParams
	mu            sync.Mutex
	blobChan      chan []BlobUpdate
	done          chan struct{}
}

// TunableParams holds algorithm parameters that can be tuned during replay
type TunableParams struct {
	DeltaRMSThreshold    *float64  // deltaRMS threshold for motion detection
	TauS                 *float64  // EMA time constant in seconds
	FresnelDecay         *float64  // Zone decay function exponent
	FresnelWeightSigma   *float64  // Gaussian sigma for Fresnel zone contribution
	MinConfidence        *float64  // Minimum confidence for detection
	BreathingSensitivity *float64  // Breathing band sensitivity multiplier
	NSubcarriers        *int      // Number of subcarriers to use
}

// DefaultTunableParams returns the default parameters
func DefaultTunableParams() *TunableParams {
	motionThreshold := 0.02
	tauS := 30.0
	fresnelDecay := 2.0
	fresnelWeightSigma := 0.1
	minConfidence := 0.3
	breathingSensitivity := 1.0
	nSubcarriers := 16

	return &TunableParams{
		DeltaRMSThreshold:    &motionThreshold,
		TauS:                 &tauS,
		FresnelDecay:         &fresnelDecay,
		FresnelWeightSigma:   &fresnelWeightSigma,
		MinConfidence:        &minConfidence,
		BreathingSensitivity: &breathingSensitivity,
		NSubcarriers:        &nSubcarriers,
	}
}

// BlobUpdate represents a single blob position update from replay
type BlobUpdate struct {
	ID                 int       `json:"id"`
	X                  float64   `json:"x"`
	Y                  float64   `json:"y"`
	Z                  float64   `json:"z"`
	VX                 float64   `json:"vx"`
	VY                 float64   `json:"vy"`
	VZ                 float64   `json:"vz"`
	Weight             float64   `json:"weight"`
	Trail              []float64 `json:"trail"` // Flat [x,z,x,z,...]
	Posture            string    `json:"posture,omitempty"`
	PersonID           string    `json:"person_id,omitempty"`
	PersonLabel        string    `json:"person_label,omitempty"`
	PersonColor        string    `json:"person_color,omitempty"`
	IdentityConfidence float64   `json:"identity_confidence,omitempty"`
	IdentitySource     string    `json:"identity_source,omitempty"`
}

// BlobBroadcaster sends replay blob updates to dashboard clients
type BlobBroadcaster interface {
	BroadcastReplayBlobs(blobs []BlobUpdate, timestampMS int64)
}

// FrameReader reads CSI frames from storage
type FrameReader interface {
	SeekToTimestamp(target time.Time) ([]byte, int64, error)
	GetTimestampRange() (oldest, newest time.Time, err error)
	ReadFrames(from time.Time, to time.Time, callback func(recvTimeNS int64, frame []byte) bool) error
}

// Engine manages replay sessions and coordinates replay operations
type Engine struct {
	mu            sync.RWMutex
	sessions      map[string]*Session
	frameReader   FrameReader
	broadcaster   BlobBroadcaster
	nextSessionID int64
}

// NewEngine creates a new replay engine
func NewEngine(reader FrameReader, broadcaster BlobBroadcaster) *Engine {
	return &Engine{
		sessions:    make(map[string]*Session),
		frameReader: reader,
		broadcaster: broadcaster,
	}
}

// StartSession starts a new replay session
func (e *Engine) StartSession(from, to time.Time) (*Session, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Validate time range
	oldest, newest, err := e.frameReader.GetTimestampRange()
	if err != nil {
		return nil, err
	}

	if from.Before(oldest) {
		from = oldest
	}
	if to.After(newest) {
		to = newest
	}
	if from.After(to) {
		from, to = to, from
	}

	e.nextSessionID++
	sessionID := generateSessionID(e.nextSessionID)

	sess := &Session{
		ID:          sessionID,
		State:       StatePaused,
		ReplayPos:   from,
		ReplaySpeed: 1.0,
		From:        from,
		To:          to,
		Params:      DefaultTunableParams(),
		blobChan:    make(chan []BlobUpdate, 10),
		done:        make(chan struct{}),
	}

	e.sessions[sessionID] = sess

	// Start the replay goroutine
	go sess.run()

	return sess, nil
}

// GetSession retrieves a session by ID
func (e *Engine) GetSession(id string) (*Session, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	sess, ok := e.sessions[id]
	return sess, ok
}

// StopSession stops and removes a session
func (e *Engine) StopSession(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	sess, ok := e.sessions[id]
	if !ok {
		return ErrSessionNotFound
	}

	close(sess.done)
	delete(e.sessions, id)
	return nil
}

// run is the main replay loop for a session
func (s *Session) run() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.mu.Lock()
			if s.State == StatePlaying {
				// Advance replay position
				dt := time.Duration(float64(100*time.Millisecond) * s.ReplaySpeed)
				s.ReplayPos = s.ReplayPos.Add(dt)

				// Check if we've reached the end
				if s.ReplayPos.After(s.To) {
					s.State = StatePaused
					s.ReplayPos = s.To
				}
			}
			s.mu.Unlock()
		}
	}
}

// Seek moves the replay position to the target time
func (s *Session) Seek(target time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.State = StateSeeking
	s.ReplayPos = target
	s.State = StatePaused

	return nil
}

// Play starts playback at the specified speed
func (s *Session) Play(speed float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.State = StatePlaying
	s.ReplaySpeed = speed

	return nil
}

// Pause pauses playback
func (s *Session) Pause() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.State = StatePaused
	return nil
}

// SetParams updates the tunable parameters
func (s *Session) SetParams(params *TunableParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Params = params
	return nil
}

// SetSpeed updates the replay speed
func (s *Session) SetSpeed(speed float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ReplaySpeed = speed
	return nil
}

// GetPosition returns the current replay position
func (s *Session) GetPosition() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ReplayPos
}

// GetState returns the current replay state
func (s *Session) GetState() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.State
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

// generateSessionID generates a unique session ID
func generateSessionID(n int64) string {
	// Simple session ID generation
	return time.Now().Format("20060102-150405") + "-" + string(rune('A'+(n%26)))
}
