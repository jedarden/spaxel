package localization

import (
	"sync"
	"time"
)

// LinkMotion describes one link's current motion state for fusion.
type LinkMotion struct {
	NodeMAC     string
	PeerMAC     string
	DeltaRMS    float64
	Motion      bool
	HealthScore float64 // Link health score from signal processing (0-1)
}

// NodePosition holds a node's (x, y, z) position in room coordinates.
type NodePosition struct {
	MAC string
	X   float64 // metres (width)
	Y   float64 // metres (height)
	Z   float64 // metres (depth)
}

// FusionResult is returned after each fusion cycle.
type FusionResult struct {
	Peaks    [][3]float64 // [x, z, weight] top peaks
	GridCols int
	GridRows int
	GridData []float64 // normalised [0..1] row-major
	Timestamp time.Time
}

// Engine runs the multi-link Fresnel zone fusion loop.
type Engine struct {
	mu          sync.RWMutex
	grid        *Grid
	nodePos     map[string]NodePosition // MAC -> position
	minWeight   float64                 // deltaRMS threshold to include a link
	maxPeaks    int
	peakThresh  float64
	lastResult  *FusionResult
	subscribers []chan FusionResult

	// Learned weights (can be set externally)
	learnedWeights *LearnedWeights

	// Spatial weight learner for per-zone weights
	spatialWeightLearner *SpatialWeightLearner
}

// NewEngine creates a fusion engine for the given room dimensions.
func NewEngine(width, depth float64, originX, originZ float64) *Engine {
	return &Engine{
		grid:       NewGrid(width, depth, 0.2, originX, originZ),
		nodePos:    make(map[string]NodePosition),
		minWeight:  0.01,
		maxPeaks:   6,
		peakThresh: 0.3,
	}
}

// SetNodePosition updates a node's floor-plane position.
func (e *Engine) SetNodePosition(mac string, x, z float64) {
	e.mu.Lock()
	e.nodePos[mac] = NodePosition{MAC: mac, X: x, Z: z}
	e.mu.Unlock()
}

// RemoveNode removes a node's position entry.
func (e *Engine) RemoveNode(mac string) {
	e.mu.Lock()
	delete(e.nodePos, mac)
	e.mu.Unlock()
}

// SetLearnedWeights sets the learned weights for self-improving localization
func (e *Engine) SetLearnedWeights(weights *LearnedWeights) {
	e.mu.Lock()
	e.learnedWeights = weights
	e.mu.Unlock()
}

// GetLearnedWeights returns the current learned weights
func (e *Engine) GetLearnedWeights() *LearnedWeights {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.learnedWeights
}

// SetSpatialWeightLearner sets the spatial weight learner for per-zone weights
func (e *Engine) SetSpatialWeightLearner(learner *SpatialWeightLearner) {
	e.mu.Lock()
	e.spatialWeightLearner = learner
	e.mu.Unlock()
}

// GetSpatialWeightLearner returns the current spatial weight learner
func (e *Engine) GetSpatialWeightLearner() *SpatialWeightLearner {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.spatialWeightLearner
}

// ResizeRoom rebuilds the grid for updated room dimensions.
func (e *Engine) ResizeRoom(width, depth, originX, originZ float64) {
	e.mu.Lock()
	e.grid.Resize(width, depth, 0.2, originX, originZ)
	e.mu.Unlock()
}

// Fuse performs a single fusion step with the provided motion states.
// It returns a FusionResult containing the normalised grid snapshot and peak positions.
// If learned weights are available, they are applied to improve accuracy.
func (e *Engine) Fuse(links []LinkMotion) *FusionResult {
	e.mu.RLock()
	nodePos := make(map[string]NodePosition, len(e.nodePos))
	for k, v := range e.nodePos {
		nodePos[k] = v
	}
	learnedWeights := e.learnedWeights
	e.mu.RUnlock()

	e.grid.Reset()

	activeLinks := 0
	for _, lm := range links {
		if !lm.Motion || lm.DeltaRMS < e.minWeight {
			continue
		}
		posA, okA := nodePos[lm.NodeMAC]
		posB, okB := nodePos[lm.PeerMAC]
		if !okA || !okB {
			continue
		}

		// Apply learned weight multiplier if available
		weight := lm.DeltaRMS
		sigmaMultiplier := 0.0

		if learnedWeights != nil {
			linkID := lm.NodeMAC + "-" + lm.PeerMAC
			weightMultiplier := learnedWeights.GetLinkWeight(linkID)
			weight *= weightMultiplier
			sigmaMultiplier = learnedWeights.GetLinkSigma(linkID)
		}

		// Use the sigma-aware version if we have learned sigma
		if sigmaMultiplier != 0 {
			e.grid.AddLinkInfluenceWithSigma(posA.X, posA.Z, posB.X, posB.Z, weight, sigmaMultiplier)
		} else {
			e.grid.AddLinkInfluence(posA.X, posA.Z, posB.X, posB.Z, weight)
		}
		activeLinks++
	}

	result := &FusionResult{Timestamp: time.Now()}

	if activeLinks == 0 {
		cells, cols, rows := e.grid.Snapshot()
		result.GridCols = cols
		result.GridRows = rows
		result.GridData = cells
		e.mu.Lock()
		e.lastResult = result
		e.mu.Unlock()
		return result
	}

	e.grid.Normalize()

	cells, cols, rows := e.grid.Snapshot()
	result.GridCols = cols
	result.GridRows = rows
	result.GridData = cells
	result.Peaks = e.grid.Peaks(e.maxPeaks, e.peakThresh)

	e.mu.Lock()
	e.lastResult = result
	e.mu.Unlock()

	return result
}

// LastResult returns the most recent fusion result, or nil.
func (e *Engine) LastResult() *FusionResult {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.lastResult
}

// GetGrid returns the underlying grid for GDOP calculations.
func (e *Engine) GetGrid() *Grid {
	return e.grid
}

// GDOPMap computes a Geometric Dilution of Precision map for the given node positions.
// Returns a flat float32 slice (row-major) of GDOP values, same dims as the localization grid.
// GDOP < 2 = good, GDOP > 5 = poor.
func (e *Engine) GDOPMap(positions []NodePosition) ([]float32, int, int) {
	e.mu.RLock()
	cols, rows, cellSize, originX, originZ := e.grid.Dims()
	e.mu.RUnlock()

	out := make([]float32, cols*rows)

	for row := 0; row < rows; row++ {
		pz := originZ + (float64(row)+0.5)*cellSize
		for col := 0; col < cols; col++ {
			px := originX + (float64(col)+0.5)*cellSize
			gdop := computeGDOP(px, pz, positions)
			out[row*cols+col] = float32(gdop)
		}
	}
	return out, cols, rows
}

// computeGDOP computes a 2D GDOP value for a point (px, pz) given node positions.
// Uses the standard formula: GDOP = sqrt(trace(HᵀH)⁻¹).
func computeGDOP(px, pz float64, nodes []NodePosition) float64 {
	if len(nodes) < 2 {
		return 10.0 // undefined
	}

	// Build H matrix (n×2): direction cosines from each node to point.
	// H[i] = [(px-nx)/d, (pz-nz)/d]
	hh := [4]float64{} // HᵀH stored as [a,b; b,c]
	for _, n := range nodes {
		dx := px - n.X
		dz := pz - n.Z
		d := dx*dx + dz*dz
		if d < 0.0001 {
			continue
		}
		// inv sqrt
		invD := 1.0 / d
		hh[0] += dx * dx * invD // HᵀH[0,0]
		hh[1] += dx * dz * invD // HᵀH[0,1]
		hh[2] += dx * dz * invD // HᵀH[1,0] = HᵀH[0,1]
		hh[3] += dz * dz * invD // HᵀH[1,1]
	}

	// Invert 2×2: [[a,b],[c,d]]^-1 = 1/(ad-bc)*[[d,-b],[-c,a]]
	det := hh[0]*hh[3] - hh[1]*hh[2]
	if det < 1e-10 {
		return 10.0
	}
	trace := (hh[3] + hh[0]) / det
	if trace < 0 {
		return 10.0
	}
	gdop := trace
	if gdop > 10 {
		gdop = 10
	}
	return gdop
}
