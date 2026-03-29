package fusion

import (
	"math"
	"sync"
	"time"
)

// LinkMotion describes one link's current motion state for 3D fusion.
type LinkMotion struct {
	// NodeMAC is the transmitting node's MAC address.
	NodeMAC string
	// PeerMAC is the receiving node's MAC address.
	PeerMAC string
	// DeltaRMS is the motion feature amplitude (from signal.MotionFeatures).
	DeltaRMS float64
	// Motion is true when the link reports motion above threshold.
	Motion bool
	// HealthScore is the link's ambient confidence score [0-1].
	// Links with lower health contribute less to the fusion grid.
	// If zero, defaults to 1.0 (full contribution).
	HealthScore float64
}

// NodePosition holds a node's 3D position in world coordinates (metres).
type NodePosition struct {
	MAC string
	X   float64
	Y   float64 // height above floor
	Z   float64
}

// Blob is a detected presence position with a confidence score.
type Blob struct {
	X, Y, Z    float64 // world-space position (metres)
	Confidence float64 // normalised [0..1]
}

// Result is returned after each fusion cycle.
type Result struct {
	Blobs     []Blob
	Timestamp time.Time
	// ActiveLinks is the number of links that contributed to this fusion.
	ActiveLinks int
}

// Engine runs the multi-link 3D Fresnel zone fusion.
type Engine struct {
	mu         sync.RWMutex
	grid       *Grid3D
	nodePos    map[string]NodePosition
	minDelta   float64 // minimum deltaRMS to use a link
	maxBlobs   int
	blobThresh float64 // normalised activation threshold for peak detection
	lastResult *Result
}

// Config holds optional configuration for NewEngine.
type Config struct {
	// Room dimensions in metres.
	Width, Height, Depth float64
	// Origin of the grid's minimum corner.
	OriginX, OriginY, OriginZ float64
	// CellSize in metres (default 0.2).
	CellSize float64
	// MinDeltaRMS is the minimum link deltaRMS to include (default 0.01).
	MinDeltaRMS float64
	// MaxBlobs is the maximum number of blobs to return (default 6).
	MaxBlobs int
	// BlobThreshold is the normalised activation floor for peak detection (default 0.3).
	BlobThreshold float64
}

// NewEngine creates a 3D fusion engine.
// If cfg is nil, sensible defaults for a 10×3×10 m room are used.
func NewEngine(cfg *Config) *Engine {
	if cfg == nil {
		cfg = &Config{
			Width: 10, Height: 3, Depth: 10,
		}
	}
	cellSize := cfg.CellSize
	if cellSize <= 0 {
		cellSize = defaultCellSize
	}
	minDelta := cfg.MinDeltaRMS
	if minDelta <= 0 {
		minDelta = 0.01
	}
	maxBlobs := cfg.MaxBlobs
	if maxBlobs <= 0 {
		maxBlobs = 6
	}
	blobThresh := cfg.BlobThreshold
	if blobThresh <= 0 {
		blobThresh = 0.3
	}
	g := NewGrid3D(cfg.Width, cfg.Height, cfg.Depth, cellSize,
		cfg.OriginX, cfg.OriginY, cfg.OriginZ)
	return &Engine{
		grid:       g,
		nodePos:    make(map[string]NodePosition),
		minDelta:   minDelta,
		maxBlobs:   maxBlobs,
		blobThresh: blobThresh,
	}
}

// SetNodePosition updates a node's 3D world-space position.
func (e *Engine) SetNodePosition(mac string, x, y, z float64) {
	e.mu.Lock()
	e.nodePos[mac] = NodePosition{MAC: mac, X: x, Y: y, Z: z}
	e.mu.Unlock()
}

// RemoveNode removes a node from the position registry.
func (e *Engine) RemoveNode(mac string) {
	e.mu.Lock()
	delete(e.nodePos, mac)
	e.mu.Unlock()
}

// Fuse performs a single fusion step over the provided link motion states.
// It returns a Result containing detected blob positions and confidence scores.
// Each link's contribution is weighted by its HealthScore (0-1). A link with
// HealthScore=0.3 contributes only 30% as much as a link with HealthScore=1.0.
func (e *Engine) Fuse(links []LinkMotion) *Result {
	// Snapshot node positions under read lock.
	e.mu.RLock()
	nodePos := make(map[string]NodePosition, len(e.nodePos))
	for k, v := range e.nodePos {
		nodePos[k] = v
	}
	minDelta := e.minDelta
	e.mu.RUnlock()

	e.grid.Reset()

	activeLinks := 0
	for _, lm := range links {
		if !lm.Motion || lm.DeltaRMS < minDelta {
			continue
		}
		posA, okA := nodePos[lm.NodeMAC]
		posB, okB := nodePos[lm.PeerMAC]
		if !okA || !okB {
			continue
		}
		// Apply health score weighting: default to 1.0 if not set
		healthWeight := lm.HealthScore
		if healthWeight <= 0 {
			healthWeight = 1.0
		}
		// Weight activation by health score
		weightedActivation := lm.DeltaRMS * healthWeight
		e.grid.AddLinkInfluence(
			posA.X, posA.Y, posA.Z,
			posB.X, posB.Y, posB.Z,
			weightedActivation,
		)
		activeLinks++
	}

	result := &Result{
		Timestamp:   time.Now(),
		ActiveLinks: activeLinks,
	}

	if activeLinks == 0 {
		e.mu.Lock()
		e.lastResult = result
		e.mu.Unlock()
		return result
	}

	e.grid.Normalize()

	rawPeaks := e.grid.Peaks(e.maxBlobs, e.blobThresh)
	blobs := make([]Blob, len(rawPeaks))
	for i, p := range rawPeaks {
		blobs[i] = Blob{X: p[0], Y: p[1], Z: p[2], Confidence: p[3]}
	}
	result.Blobs = blobs

	e.mu.Lock()
	e.lastResult = result
	e.mu.Unlock()

	return result
}

// LastResult returns the most recent fusion result, or nil.
func (e *Engine) LastResult() *Result {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.lastResult
}

// FresnelZoneRadius returns the maximum first-Fresnel-zone radius at the
// midpoint of a link of length d (metres) at 2.4 GHz.
// Useful for callers choosing grid resolution.
func FresnelZoneRadius(linkLength float64) float64 {
	const lambda = 0.125
	return math.Sqrt(lambda * linkLength / 4.0)
}
