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
	SetNodePosition(mac string, x, y, z float64)
}

// Worker reads CSI frames from a replay store and processes them.
type Worker struct {
	mu       sync.Mutex
	sessions map[string]*ReplaySession
	nextID   int

	store         RecordingStore
	processor     *signal.ProcessorManager
	fusionEngine  FusionEngine
	nodePositions map[string]localization.NodePosition // MAC -> position
	broadcaster   BlobBroadcaster
	done          chan struct{}
	wg            sync.WaitGroup
}

// RecordingStore is the interface to read recorded CSI frames.
type RecordingStore interface {
	Stats() Stats
	Scan(fn func(recvTimeNS int64, frame []byte) bool) error
	ScanRange(fromNS, toNS int64, fn func(recvTimeNS int64, frame []byte) bool) error
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


// NewWorker creates a new replay worker.
func NewWorker(store RecordingStore, processor *signal.ProcessorManager, broadcaster BlobBroadcaster) *Worker {
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
func (w *Worker) processSession(s *ReplaySession) {
	// Read next frame(s) from replay store
	// Use ScanRange to only read frames after current position
	var frameData []byte
	var frameTimeNS int64
	frameFound := false

	// Scan from current position to end of session, looking for the next frame
	// We add a small lookahead window (1 second worth at 20 Hz = 20 frames) to find the next frame
	fromNS := s.CurrentMS * 1e6
	toNS := s.ToMS * 1e6
	if toNS <= fromNS {
		// At end of session
		s.State = "paused"
		return
	}

	// Look ahead for the next frame after current position
	err := w.store.ScanRange(fromNS, toNS, func(recvTimeNS int64, frame []byte) bool {
		recvMS := recvTimeNS / 1e6
		if recvMS <= s.CurrentMS {
			return true // skip frames at or before current position
		}
		// Found next frame
		frameTimeNS = recvTimeNS
		frameData = frame
		frameFound = true
		s.CurrentMS = recvMS
		return false // stop at first frame after current position
	})

	if err != nil || !frameFound || len(frameData) == 0 {
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

	// Run fusion to generate blobs if we have a fusion engine
	if w.fusionEngine != nil {
		blobs := w.runFusion()
		s.LastBlobs = blobs
		s.LastBlobTime = frameTimeNS / 1e6
		w.broadcaster.BroadcastReplayBlobs(blobs, frameTimeNS/1e6)
	} else {
		s.LastBlobs = []BlobUpdate{}
		s.LastBlobTime = frameTimeNS / 1e6
		w.broadcaster.BroadcastReplayBlobs([]BlobUpdate{}, frameTimeNS/1e6)
	}
}

// runFusion runs the fusion algorithm on current motion states and generates blob updates.
func (w *Worker) runFusion() []BlobUpdate {
	if w.processor == nil || w.fusionEngine == nil {
		return []BlobUpdate{}
	}

	// Get motion states from all links
	motionStates := w.processor.GetAllMotionStates()

	// Convert to fusion LinkMotion format
	links := make([]localization.LinkMotion, 0, len(motionStates))
	for _, state := range motionStates {
		// Parse linkID format "nodeMAC:peerMAC"
		parts := splitLinkID(state.LinkID)
		if len(parts) != 2 {
			continue
		}

		link := localization.LinkMotion{
			NodeMAC:     parts[0],
			PeerMAC:     parts[1],
			DeltaRMS:    state.SmoothDeltaRMS,
			Motion:      state.MotionDetected,
			HealthScore: state.AmbientConfidence,
		}

		// Use BaselineConf if AmbientConfidence is not available
		if link.HealthScore == 0 && state.BaselineConf > 0 {
			link.HealthScore = state.BaselineConf
		}

		links = append(links, link)
	}

	// Run fusion
	result := w.fusionEngine.Fuse(links)
	if result == nil || len(result.Peaks) == 0 {
		return []BlobUpdate{}
	}

	// Convert fusion peaks to BlobUpdate format
	blobs := make([]BlobUpdate, 0, len(result.Peaks))
	for i, peak := range result.Peaks {
		blobs = append(blobs, BlobUpdate{
			ID:     i + 1,
			X:      peak[0],
			Y:      1.2, // Default height (meters above floor)
			Z:      peak[1],
			VX:     0,
			VY:     0,
			VZ:     0,
			Weight: peak[2],
		})
	}

	return blobs
}

// splitLinkID splits a link ID in "nodeMAC:peerMAC" format.
func splitLinkID(linkID string) []string {
	for i := 0; i < len(linkID); i++ {
		if linkID[i] == ':' {
			return []string{linkID[:i], linkID[i+1:]}
		}
	}
	return []string{linkID}
}

// StartSession creates a new replay session.
func (w *Worker) StartSession(fromMS, toMS int64, speed int) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	id := w.generateID()
	s := &ReplaySession{
		ID:        id,
		FromMS:    fromMS,
		ToMS:      toMS,
		CurrentMS: fromMS,
		Speed:     speed,
		State:     "paused",
		Params:    make(map[string]interface{}),
		CreatedAt: time.Now(),
		baselineState: make(map[string]*signal.BaselineState),
		LastBlobs:    []BlobUpdate{},
		LastBlobTime: fromMS,
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
func (w *Worker) GetSession(sessionID string) (*ReplaySession, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	s, exists := w.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}

	return s, nil
}

// GetAllSessions returns all active sessions.
func (w *Worker) GetAllSessions() []*ReplaySession {
	w.mu.Lock()
	defer w.mu.Unlock()

	sessions := make([]*ReplaySession, 0, len(w.sessions))
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

// GetStoreStats returns statistics about the replay store.
func (w *Worker) GetStoreStats() Stats {
	return w.store.Stats()
}

// GetStore returns the replay store.
func (w *Worker) GetStore() RecordingStore {
	return w.store
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
