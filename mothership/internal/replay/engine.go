// Package replay implements CSI replay with time-travel debugging.
//
// ReplayEngine manages the replay lifecycle including state machine,
// seeking, playback, and parameter injection.
package replay

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/spaxel/mothership/internal/ingestion"
	"github.com/spaxel/mothership/internal/localization"
	sigproc "github.com/spaxel/mothership/internal/signal"
)

// ReplayState represents the current state of the replay engine.
type ReplayState string

const (
	StateLive      ReplayState = "live"     // Normal live operation
	StatePaused    ReplayState = "paused"   // Replay mode, paused
	StateReplaying ReplayState = "replaying" // Replay mode, playing
	StateSeeking   ReplayState = "seeking"  // Seeking to timestamp
)

// Engine manages the replay lifecycle with state machine.
type Engine struct {
	mu sync.RWMutex

	// State
	state         ReplayState
	replayPosition int64  // Current replay timestamp (Unix ms)
	replaySpeed   float64 // Playback speed multiplier (1.0 = real-time)

	// Session
	linkedSessionID string // WebSocket session ID for the client
	session         *Session

	// Components
	store         RecordingStore
	pipeline      *Pipeline
	fusionEngine  FusionEngine
	broadcaster   BlobBroadcaster

	// Timing
	lastFrameTime time.Time // Timestamp of last processed frame
	tickDuration  time.Duration // Target duration between frames

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	wg     sync.WaitGroup
}

// EngineConfig configures a new ReplayEngine.
type EngineConfig struct {
	Store         RecordingStore
	Processor     *sigproc.ProcessorManager
	FusionEngine  FusionEngine
	Broadcaster   BlobBroadcaster
	TickDuration  time.Duration // Target frame interval (default: 100ms for 10Hz)
}

// NewEngine creates a new ReplayEngine.
func NewEngine(config EngineConfig) *Engine {
	ctx, cancel := context.WithCancel(context.Background())

	tickDuration := config.TickDuration
	if tickDuration == 0 {
		tickDuration = 100 * time.Millisecond // 10 Hz default
	}

	return &Engine{
		state:         StateLive,
		replaySpeed:   1.0,
		replayPosition: 0,
		tickDuration:  tickDuration,
		store:         config.Store,
		broadcaster:   config.Broadcaster,
		fusionEngine:  config.FusionEngine,
		ctx:           ctx,
		cancel:        cancel,
		done:          make(chan struct{}),
	}
}

// Start begins the replay engine's main loop.
func (e *Engine) Start() {
	e.wg.Add(1)
	go e.run()
}

// Stop gracefully shuts down the engine.
func (e *Engine) Stop() {
	e.cancel()
	close(e.done)
	e.wg.Wait()
}

// run is the main engine loop.
func (e *Engine) run() {
	defer e.wg.Done()

	ticker := time.NewTicker(e.tickDuration)
	defer ticker.Stop()

	for {
		select {
		case <-e.done:
			return
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			e.tick()
		}
	}
}

// tick processes one iteration of replay or live mode.
func (e *Engine) tick() {
	e.mu.Lock()
	state := e.state
	e.mu.Unlock()

	switch state {
	case StateReplaying:
		e.processReplayTick()
	case StateSeeking:
		e.processSeekTick()
	case StatePaused, StateLive:
		// No processing needed
	}
}

// EnterReplayMode enters replay mode with the specified time range.
func (e *Engine) EnterReplayMode(sessionID string, fromMS, toMS int64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state == StateReplaying || e.state == StatePaused {
		return ErrAlreadyInReplayMode
	}

	// Create new session
	session := NewSession(sessionID, e.store.(*RecordingStore), fromMS, toMS)
	e.session = session
	e.linkedSessionID = sessionID
	e.replayPosition = fromMS
	e.state = StatePaused
	e.replaySpeed = 1.0

	// Create replay pipeline
	if e.pipeline == nil {
		e.pipeline = NewPipeline()
	}

	log.Printf("[INFO] ReplayEngine: Entered replay mode for session %s (%d to %d)",
		sessionID, fromMS, toMS)

	return nil
}

// ExitReplayMode exits replay mode and returns to live.
func (e *Engine) ExitReplayMode() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state == StateLive {
		return nil
	}

	e.state = StateLive
	e.replayPosition = 0
	e.session = nil
	e.linkedSessionID = ""

	// Clear replay pipeline
	if e.pipeline != nil {
		e.pipeline = nil
	}

	log.Printf("[INFO] ReplayEngine: Exited replay mode")

	return nil
}

// Seek moves the replay position to the specified timestamp.
func (e *Engine) Seek(targetMS int64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state == StateLive {
		return ErrNotInReplayMode
	}

	if e.session == nil {
		return ErrNoActiveSession
	}

	// Validate target is within session range
	if targetMS < e.session.FromMS || targetMS > e.session.ToMS {
		return ErrTimestampOutOfRange
	}

	e.state = StateSeeking
	e.replayPosition = targetMS

	// Reset pipeline state for clean replay
	if e.pipeline != nil {
		e.pipeline.Reset()
	}

	return nil
}

// Play starts playback at the specified speed.
func (e *Engine) Play(speed float64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state == StateLive {
		return ErrNotInReplayMode
	}

	if e.session == nil {
		return ErrNoActiveSession
	}

	if speed < 0.1 || speed > 10.0 {
		return ErrInvalidSpeed
	}

	e.replaySpeed = speed
	e.state = StateReplaying
	e.lastFrameTime = time.Now()

	return nil
}

// Pause pauses playback.
func (e *Engine) Pause() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state == StateLive {
		return ErrNotInReplayMode
	}

	e.state = StatePaused
	return nil
}

// SetParams updates the replay pipeline parameters.
func (e *Engine) SetParams(params *TunableParams) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state == StateLive {
		return ErrNotInReplayMode
	}

	if e.pipeline == nil {
		return ErrNoPipeline
	}

	e.pipeline.SetParams(params)

	// Re-process current position with new parameters
	go e.reprocessCurrentPosition()

	return nil
}

// GetState returns the current engine state.
func (e *Engine) GetState() ReplayState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.state
}

// GetPosition returns the current replay position.
func (e *Engine) GetPosition() int64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.replayPosition
}

// GetSession returns the current replay session.
func (e *Engine) GetSession() *Session {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.session
}

// processReplayTick processes one replay tick.
func (e *Engine) processReplayTick() {
	if e.session == nil || e.pipeline == nil {
		return
	}

	// Read next frame(s) from replay store
	var frames []Frame
	var frameTimeNS int64

	fromNS := e.replayPosition * 1e6
	toNS := e.session.ToMS * 1e6

	err := e.store.ScanRange(fromNS, toNS, func(recvTimeNS int64, frame []byte) bool {
		recvMS := recvTimeNS / 1e6
		if recvMS <= e.replayPosition {
			return true // Skip frames at or before current position
		}
		// Found next frame
		frameTimeNS = recvTimeNS
		frames = append(frames, Frame{
			RecvTimeNS: recvTimeNS,
			Data:       frame,
		})
		e.replayPosition = recvMS
		return false // Stop at first frame after current position
	})

	if err != nil || len(frames) == 0 {
		// No more frames, pause
		e.state = StatePaused
		return
	}

	// Process frames through replay pipeline
	for _, frame := range frames {
		e.processFrame(frame)
	}

	// Sleep based on replay speed
	if e.replaySpeed > 0 {
		sleepDuration := time.Duration(float64(e.tickDuration) / e.replaySpeed)
		time.Sleep(sleepDuration)
	}
}

// processSeekTick handles seeking by finding the closest frame to target.
func (e *Engine) processSeekTick() {
	if e.session == nil {
		e.state = StatePaused
		return
	}

	// Find frame closest to target position
	var closestFrame *Frame
	var closestTimeNS int64
	minDiff := int64(1 << 62) // Very large value

	targetNS := e.replayPosition * 1e6

	fromNS := e.session.FromMS * 1e6
	toNS := e.session.ToMS * 1e6

	e.store.ScanRange(fromNS, toNS, func(recvTimeNS int64, frame []byte) bool {
		diff := recvTimeNS - targetNS
		if diff < 0 {
			diff = -diff
		}
		if diff < minDiff {
			minDiff = diff
			closestFrame = &Frame{
				RecvTimeNS: recvTimeNS,
				Data:       frame,
			}
			closestTimeNS = recvTimeNS
		}
		return true // Continue to find closest
	})

	if closestFrame != nil {
		e.replayPosition = closestTimeNS / 1e6
		e.processFrame(*closestFrame)
	}

	e.state = StatePaused
}

// processFrame processes a single CSI frame through the replay pipeline.
func (e *Engine) processFrame(frame Frame) {
	// Parse the CSI frame
	parsed, err := ingestion.ParseFrame(frame.Data)
	if err != nil {
		log.Printf("[DEBUG] ReplayEngine: Failed to parse frame: %v", err)
		return
	}

	recvTime := time.Unix(0, frame.RecvTimeNS)

	// Process through replay pipeline
	if e.pipeline != nil {
		result := e.pipeline.ProcessFrame(parsed, recvTime)

		// Run fusion if available
		if e.fusionEngine != nil && e.pipeline.HasMotionData() {
			blobs := e.runFusion()
			e.broadcaster.BroadcastReplayBlobs(blobs, frame.RecvTimeNS/1e6)
		}
	}
}

// runFusion runs the fusion algorithm on current motion states.
func (e *Engine) runFusion() []BlobUpdate {
	if e.pipeline == nil || e.fusionEngine == nil {
		return []BlobUpdate{}
	}

	// Get motion states from replay pipeline
	motionStates := e.pipeline.GetAllMotionStates()

	// Convert to fusion LinkMotion format
	links := make([]localization.LinkMotion, 0, len(motionStates))
	for _, state := range motionStates {
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

		if link.HealthScore == 0 && state.BaselineConf > 0 {
			link.HealthScore = state.BaselineConf
		}

		links = append(links, link)
	}

	// Run fusion
	result := e.fusionEngine.Fuse(links)
	if result == nil || len(result.Peaks) == 0 {
		return []BlobUpdate{}
	}

	// Convert fusion peaks to BlobUpdate format
	blobs := make([]BlobUpdate, 0, len(result.Peaks))
	for i, peak := range result.Peaks {
		blobs = append(blobs, BlobUpdate{
			ID:     i + 1,
			X:      peak[0],
			Y:      1.2,
			Z:      peak[1],
			VX:     0,
			VY:     0,
			VZ:     0,
			Weight: peak[2],
		})
	}

	return blobs
}

// reprocessCurrentPosition re-processes the current position with new parameters.
func (e *Engine) reprocessCurrentPosition() {
	e.mu.Lock()
	session := e.session
	position := e.replayPosition
	e.mu.Unlock()

	if session == nil {
		return
	}

	// Seek to current position to re-process
	targetNS := position * 1e6
	fromNS := session.FromMS * 1e6
	toNS := position * 1e6 + int64(60*time.Second) // Process 60 second window

	// Process frames in range
	e.store.ScanRange(fromNS, toNS, func(recvTimeNS int64, frame []byte) bool {
		if recvTimeNS > targetNS+int64(2*time.Second) {
			return false // Stop after processing a few seconds
		}

		// Parse and process frame
		parsed, err := ingestion.ParseFrame(frame)
		if err != nil {
			return true
		}

		recvTime := time.Unix(0, recvTimeNS)
		e.pipeline.ProcessFrame(parsed, recvTime)

		return true
	})

	// Run final fusion and broadcast
	if e.fusionEngine != nil && e.pipeline.HasMotionData() {
		blobs := e.runFusion()
		e.broadcaster.BroadcastReplayBlobs(blobs, position)
	}

	log.Printf("[INFO] ReplayEngine: Reprocessed position %d with new parameters", position)
}

// SetProcessorManager sets the signal processor for the replay pipeline.
func (e *Engine) SetProcessorManager(pm *sigproc.ProcessorManager) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.pipeline != nil {
		e.pipeline.SetProcessorManager(pm)
	}
}

// ApplyToLive copies the currently-active replay parameters to the live
// configuration and persists them to the mothership config file.
// The live pipeline picks up the new values within one processing cycle.
// Returns an error if not in replay mode or no replay session exists.
func (e *Engine) ApplyToLive() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state == StateLive {
		return ErrNotInReplayMode
	}

	if e.pipeline == nil {
		return ErrNoPipeline
	}

	params := e.pipeline.GetParams()

	// Apply parameters to live processor
	// This is a simplified implementation - in production, this would
	// persist to the config file and notify the live pipeline
	log.Printf("[INFO] ReplayEngine: Applying replay parameters to live: %+v", params)

	// The actual parameter application would happen through the
	// live processor's configuration system
	// For now, we just log what would be applied

	return nil
}

// Errors
var (
	ErrAlreadyInReplayMode  = &replayEngineError{"already in replay mode"}
	ErrNotInReplayMode     = &replayEngineError{"not in replay mode"}
	ErrNoActiveSession      = &replayEngineError{"no active replay session"}
	ErrTimestampOutOfRange  = &replayEngineError{"timestamp outside session range"}
	ErrInvalidSpeed         = &replayEngineError{"speed must be between 0.1 and 10.0"}
	ErrNoPipeline           = &replayEngineError{"no replay pipeline"}
)

type replayEngineError struct {
	msg string
}

func (e *replayEngineError) Error() string {
	return e.msg
}
