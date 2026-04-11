// Package replay implements CSI replay with time-travel debugging.
//
// Replay reads recorded CSI frames from the replay store and processes them
// through a separate signal processing pipeline, enabling:
// - Pause live mode and scrub through historical data
// - Adjust detection parameters and see results immediately
// - Replay at different speeds (1x, 2x, 5x)
package replay

import (
	"fmt"
	"sync"
	"time"

	"github.com/spaxel/mothership/internal/localization"
	"github.com/spaxel/mothership/internal/signal"
)

// ReplaySession represents an active replay session in the worker.
type ReplaySession struct {
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

	// Most recent blobs from replay fusion
	LastBlobs    []BlobUpdate
	LastBlobTime int64 // timestamp_ms of the last blob update
}

// FusionEngine is the interface required for replay blob generation.
type FusionEngine interface {
	Fuse(links []localization.LinkMotion) *localization.FusionResult
	SetNodePosition(mac string, x, z float64)
}

// Worker reads CSI frames from a replay store and processes them.
type Worker struct {
	mu       sync.Mutex
	sessions map[string]*ReplaySession
	nextID   int

	store         FrameReader
	processor     *signal.ProcessorManager
	fusionEngine  FusionEngine
	nodePositions map[string]localization.NodePosition // MAC -> position
	broadcaster   BlobBroadcaster
	done          chan struct{}
	wg            sync.WaitGroup
}

// FrameReader is the interface to read recorded CSI frames.
type FrameReader interface {
	Stats() Stats
	Scan(fn func(recvTimeNS int64, frame []byte) bool) error
	ScanRange(fromNS, toNS int64, fn func(recvTimeNS int64, frame []byte) bool) error
	Close() error
}

// StoreStats is an alias for Stats for backward compatibility.
type StoreStats = Stats


// NewWorker creates a new replay worker.
func NewWorker(store FrameReader, processor *signal.ProcessorManager, broadcaster BlobBroadcaster) *Worker {
	return &Worker{
		sessions:    make(map[string]*ReplaySession),
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

// SetFusionEngine sets the fusion engine for replay blob generation.
func (w *Worker) SetFusionEngine(fusionEngine FusionEngine) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.fusionEngine = fusionEngine
	if w.nodePositions == nil {
		w.nodePositions = make(map[string]localization.NodePosition)
	}
}

// SetNodePosition updates a node's position for replay fusion.
func (w *Worker) SetNodePosition(mac string, x, y, z float64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.nodePositions == nil {
		w.nodePositions = make(map[string]localization.NodePosition)
	}
	w.nodePositions[mac] = localization.NodePosition{MAC: mac, X: x, Y: y, Z: z}
	// Also update fusion engine if available
	if w.fusionEngine != nil {
		w.fusionEngine.SetNodePosition(mac, x, z)
	}
}

// Start starts the worker background goroutines.
func (w *Worker) Start() {
	// No-op for now: sessions run inline when started
}

// Stop shuts down the worker and all active sessions.
func (w *Worker) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, sess := range w.sessions {
		sess.State = "stopped"
	}
}

// GetStoreStats returns statistics about the replay store.
func (w *Worker) GetStoreStats() StoreStats {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.store == nil {
		return StoreStats{}
	}
	return w.store.Stats()
}

// GetStore returns the underlying frame reader for direct access.
func (w *Worker) GetStore() FrameReader {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.store
}

// StartSession creates a new replay session with the given time range and speed.
func (w *Worker) StartSession(fromMS, toMS int64, speed int) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.nextID++
	sessionID := fmt.Sprintf("replay-%d", w.nextID)
	w.sessions[sessionID] = &ReplaySession{
		ID:        sessionID,
		FromMS:    fromMS,
		ToMS:      toMS,
		CurrentMS: fromMS,
		Speed:     speed,
		State:     "paused",
		Params:    make(map[string]interface{}),
		CreatedAt: time.Now(),
	}
	return sessionID, nil
}

// StopSession stops and removes a replay session.
func (w *Worker) StopSession(sessionID string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.sessions[sessionID]; !ok {
		return fmt.Errorf("session not found")
	}
	delete(w.sessions, sessionID)
	return nil
}

// GetSession retrieves a session by ID.
func (w *Worker) GetSession(sessionID string) (*ReplaySession, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	sess, ok := w.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session not found")
	}
	return sess, nil
}

// Seek moves a session's current position to the target timestamp.
func (w *Worker) Seek(sessionID string, targetMS int64) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	sess, ok := w.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found")
	}
	if targetMS < sess.FromMS || targetMS > sess.ToMS {
		return fmt.Errorf("timestamp outside session range")
	}
	sess.CurrentMS = targetMS
	sess.State = "paused"
	return nil
}

// UpdateParams updates the tunable parameters for a session.
func (w *Worker) UpdateParams(sessionID string, params map[string]interface{}) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	sess, ok := w.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found")
	}
	for k, v := range params {
		sess.Params[k] = v
	}
	return nil
}

// SetPlaybackSpeed updates the playback speed for a session.
func (w *Worker) SetPlaybackSpeed(sessionID string, speed int) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	sess, ok := w.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found")
	}
	sess.Speed = speed
	return nil
}

// SetState updates the playback state for a session.
func (w *Worker) SetState(sessionID string, state string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	sess, ok := w.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found")
	}
	sess.State = state
	return nil
}
