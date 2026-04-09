// Package replay implements CSI replay with time-travel debugging.
//
// Replay reads recorded CSI frames from the replay store and processes them
// through a separate signal processing pipeline, enabling:
// - Pause live mode and scrub through historical data
// - Adjust detection parameters and see results immediately
// - Replay at different speeds (1x, 2x, 5x)
package replay

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/spaxel/mothership/internal/ingestion"
	"github.com/spaxel/mothership/internal/signal"
)

// Worker reads CSI frames from a replay store and processes them.
type Worker struct {
	mu       sync.Mutex
	sessions map[string]*session
	nextID   int

	store      RecordingStore
	processor  *signal.ProcessorManager
	broadcaster BlobBroadcaster
	done       chan struct{}
	wg         sync.WaitGroup
}

// RecordingStore is the interface to read recorded CSI frames.
type RecordingStore interface {
	Stats() Stats
	Scan(fn func(recvTimeNS int64, frame []byte) bool) error
	Close() error
}

// Stats represents replay store statistics.
type Stats struct {
	HasData   bool
	WritePos  int64
	OldestPos int64
	FileSize  int64
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

// session represents an active replay session.
type session struct {
	ID        string
	FromMS    int64
	ToMS      int64
	CurrentMS int64
	Speed     int
	State     string // playing, paused, stopped
	Params    map[string]interface{}
	CreatedAt time.Time

	// Pipeline state for this session
	baselineState map[string]*signal.BaselineState // per-link baseline
}

// NewWorker creates a new replay worker.
func NewWorker(store RecordingStore, processor *signal.ProcessorManager, broadcaster BlobBroadcaster) *Worker {
	return &Worker{
		sessions:    make(map[string]*session),
		store:       store,
		processor:   processor,
		broadcaster: broadcaster,
		done:        make(chan struct{}),
	}
}

// SetBroadcaster sets the blob broadcaster for replay results.
func (w *Worker) SetBroadcaster(broadcaster BlobBroadcaster) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.broadcaster = broadcaster
}

// SetProcessorManager sets the signal processor for replay frames.
func (w *Worker) SetProcessorManager(processor *signal.ProcessorManager) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.processor = processor
}

// Start begins the replay worker.
func (w *Worker) Start() {
	w.wg.Add(1)
	go w.run()
}

// Stop gracefully shuts down the worker.
func (w *Worker) Stop() {
	close(w.done)
	w.wg.Wait()
}

// run is the main worker loop.
func (w *Worker) run() {
	defer w.wg.Done()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-w.done:
			return
		case <-ticker.C:
			w.tick()
		}
	}
}

// tick processes all active replay sessions.
func (w *Worker) tick() {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, s := range w.sessions {
		if s.State == "playing" {
			w.processSession(s)
		}
	}
}

// processSession reads and processes frames for a session.
func (w *Worker) processSession(s *session) {
	// Read next frame(s) from replay store
	var frameData []byte
	var frameTimeNS int64

	err := w.store.Scan(func(recvTimeNS int64, frame []byte) bool {
		recvMS := recvTimeNS / 1e6
		if recvMS < s.CurrentMS {
			return true // skip frames before current position
		}
		if recvMS > s.ToMS {
			return false // past session end
		}
		if recvMS > s.CurrentMS {
			frameTimeNS = recvTimeNS
			frameData = frame
			s.CurrentMS = recvMS
			return false // stop at first frame after current position
		}
		return true
	})

	if err != nil || len(frameData) == 0 {
		// No more frames in this session
		s.State = "paused"
		return
	}

	// Parse and process the CSI frame
	parsed, err := ingestion.ParseFrame(frameData)
	if err != nil {
		log.Printf("[DEBUG] Replay frame parse error: %v", err)
		return
	}

	recvTime := time.Unix(0, frameTimeNS)

	// Process through signal pipeline with session's baseline
	linkID := parsed.LinkID()
	if w.processor != nil && int(parsed.NSub) > 0 {
		result, err := w.processor.ProcessWithBaseline(linkID, parsed.Payload,
			parsed.RSSI, int(parsed.NSub), recvTime, s.baselineState[linkID])
		if err != nil {
			log.Printf("[DEBUG] Replay signal processing error for %s: %v", linkID, err)
			return
		}

		// Store updated baseline
		if s.baselineState == nil {
			s.baselineState = make(map[string]*signal.BaselineState)
		}
		s.baselineState[linkID] = result.Baseline
	}

	// Broadcast replay blob update (empty for now - fusion will populate)
	w.broadcaster.BroadcastReplayBlobs([]BlobUpdate{}, frameTimeNS/1e6)
}

// StartSession creates a new replay session.
func (w *Worker) StartSession(fromMS, toMS int64, speed int) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	id := w.generateID()
	s := &session{
		ID:        id,
		FromMS:    fromMS,
		ToMS:      toMS,
		CurrentMS: fromMS,
		Speed:     speed,
		State:     "paused",
		Params:    make(map[string]interface{}),
		CreatedAt: time.Now(),
		baselineState: make(map[string]*signal.BaselineState),
	}

	w.sessions[id] = s
	log.Printf("[INFO] Replay session started: %s (from %d to %d, speed %dx)",
		id, fromMS, toMS, speed)

	return id, nil
}

// StopSession stops and removes a replay session.
func (w *Worker) StopSession(sessionID string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	s, exists := w.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}

	s.State = "stopped"
	delete(w.sessions, sessionID)

	log.Printf("[INFO] Replay session stopped: %s", sessionID)
	return nil
}

// Seek moves a session's cursor to the target timestamp.
func (w *Worker) Seek(sessionID string, targetMS int64) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	s, exists := w.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}

	if targetMS < s.FromMS || targetMS > s.ToMS {
		return ErrTimestampOutOfRange
	}

	s.CurrentMS = targetMS
	s.State = "paused"

	// Reset baseline state for clean replay
	s.baselineState = make(map[string]*signal.BaselineState)

	log.Printf("[INFO] Replay session seeked: %s to %d", sessionID, targetMS)
	return nil
}

// SetPlaybackSpeed changes a session's playback speed.
func (w *Worker) SetPlaybackSpeed(sessionID string, speed int) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	s, exists := w.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}

	if speed != 1 && speed != 2 && speed != 5 {
		return ErrInvalidSpeed
	}

	s.Speed = speed
	return nil
}

// SetState changes a session's playback state.
func (w *Worker) SetState(sessionID, state string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	s, exists := w.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}

	switch state {
	case "playing", "paused":
		s.State = state
	default:
		return ErrInvalidState
	}

	return nil
}

// UpdateParams updates a session's pipeline parameters.
func (w *Worker) UpdateParams(sessionID string, params map[string]interface{}) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	s, exists := w.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}

	// Merge params
	for k, v := range params {
		s.Params[k] = v
	}

	return nil
}

// GetSession returns a session by ID.
func (w *Worker) GetSession(sessionID string) (*session, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	s, exists := w.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}

	return s, nil
}

// GetAllSessions returns all active sessions.
func (w *Worker) GetAllSessions() []*session {
	w.mu.Lock()
	defer w.mu.Unlock()

	sessions := make([]*session, 0, len(w.sessions))
	for _, s := range w.sessions {
		sessions = append(sessions, s)
	}
	return sessions
}

func (w *Worker) generateID() string {
	w.nextID++
	return w.formatID(w.nextID)
}

func (w *Worker) formatID(n int) string {
	return "replay-" + time.Now().Format("20060102-150405") + "-" + string(rune('A'+(n%26)))
}

// Errors
var (
	ErrSessionNotFound     = &replayError{"session not found"}
	ErrTimestampOutOfRange = &replayError{"timestamp outside session range"}
	ErrInvalidSpeed        = &replayError{"speed must be 1, 2, or 5"}
	ErrInvalidState        = &replayError{"state must be 'playing' or 'paused'"}
)

type replayError struct {
	msg string
}

func (e *replayError) Error() string {
	return e.msg
}
