// Package localization provides multi-link WiFi CSI-based spatial localization.
package localization

import (
	"math"
	"sync"
)

// Grid is a 2D occupancy probability grid on the floor plane (XZ in Three.js coords).
// Cells represent 0.2 m × 0.2 m tiles; values are accumulated probability weights.
type Grid struct {
	mu       sync.RWMutex
	cells    []float64
	cols     int // X dimension
	rows     int // Z dimension
	cellSize float64
	originX  float64
	originZ  float64
}

// NewGrid creates a grid covering the given room bounds at the given cell resolution.
// width is room X extent (metres), depth is room Z extent (metres).
func NewGrid(width, depth, cellSize float64, originX, originZ float64) *Grid {
	cols := int(math.Ceil(width / cellSize))
	rows := int(math.Ceil(depth / cellSize))
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	return &Grid{
		cells:    make([]float64, cols*rows),
		cols:     cols,
		rows:     rows,
		cellSize: cellSize,
		originX:  originX,
		originZ:  originZ,
	}
}

// Reset zeroes all cells.
func (g *Grid) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	for i := range g.cells {
		g.cells[i] = 0
	}
}

// AddLinkInfluence paints the Fresnel-zone ellipsoidal influence of a single
// WiFi link onto the grid.
//
// The link runs from (ax, az) to (bx, bz).
// weight is the deltaRMS value for this link (higher = stronger motion).
//
// Model: for each grid cell P, compute the excess path length
//
//	excess = dist(A,P) + dist(P,B) - dist(A,B)
//
// The influence falls off as exp(-excess² / (2σ²)), where σ ≈ λ/2 (Fresnel
// zone width parameter).  We scale by the link weight so strongly-active links
// dominate weakly-active ones.
func (g *Grid) AddLinkInfluence(ax, az, bx, bz, weight float64) {
	if weight <= 0 {
		return
	}

	ab := math.Sqrt((bx-ax)*(bx-ax) + (bz-az)*(bz-az))
	if ab < 0.1 {
		return // degenerate link
	}

	// σ is chosen so the first Fresnel zone (excess = λ/2 ≈ 0.062m at 2.4GHz)
	// maps to ~1σ, giving comfortable spatial spread.  In practice a wider
	// sigma (0.5m) gives better localisation for indoor multipath.
	sigma := math.Max(ab*0.25, 0.5)
	twoSigSq := 2 * sigma * sigma

	g.mu.Lock()
	defer g.mu.Unlock()

	for row := 0; row < g.rows; row++ {
		pz := g.originZ + (float64(row)+0.5)*g.cellSize
		for col := 0; col < g.cols; col++ {
			px := g.originX + (float64(col)+0.5)*g.cellSize

			dAP := math.Sqrt((px-ax)*(px-ax) + (pz-az)*(pz-az))
			dPB := math.Sqrt((px-bx)*(px-bx) + (pz-bz)*(pz-bz))
			excess := dAP + dPB - ab

			if excess < 0 {
				excess = 0
			}
			influence := weight * math.Exp(-(excess * excess) / twoSigSq)
			g.cells[row*g.cols+col] += influence
		}
	}
}

// Normalize scales the grid so the maximum cell value is 1.0.
// Returns false if the grid is all zero.
func (g *Grid) Normalize() bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	maxVal := 0.0
	for _, v := range g.cells {
		if v > maxVal {
			maxVal = v
		}
	}
	if maxVal == 0 {
		return false
	}
	for i := range g.cells {
		g.cells[i] /= maxVal
	}
	return true
}

// Peaks returns the top-N local maxima in the grid as (x, z, weight) triplets.
// Peaks are found by 3×3 neighbourhood suppression after the grid is normalized.
func (g *Grid) Peaks(n int, threshold float64) [][3]float64 {
	g.mu.RLock()
	defer g.mu.RUnlock()

	type peak struct {
		x, z, w float64
	}
	var candidates []peak

	for row := 1; row < g.rows-1; row++ {
		for col := 1; col < g.cols-1; col++ {
			v := g.cells[row*g.cols+col]
			if v < threshold {
				continue
			}
			// Check 8-neighbours.
			isMax := true
			for dr := -1; dr <= 1 && isMax; dr++ {
				for dc := -1; dc <= 1 && isMax; dc++ {
					if dr == 0 && dc == 0 {
						continue
					}
					if g.cells[(row+dr)*g.cols+(col+dc)] > v {
						isMax = false
					}
				}
			}
			if isMax {
				x := g.originX + (float64(col)+0.5)*g.cellSize
				z := g.originZ + (float64(row)+0.5)*g.cellSize
				candidates = append(candidates, peak{x, z, v})
			}
		}
	}

	// Sort descending by weight.
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0 && candidates[j].w > candidates[j-1].w; j-- {
			candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
		}
	}

	if n > len(candidates) {
		n = len(candidates)
	}
	out := make([][3]float64, n)
	for i := 0; i < n; i++ {
		out[i] = [3]float64{candidates[i].x, candidates[i].z, candidates[i].w}
	}
	return out
}

// Snapshot returns a copy of the grid cells as a flat slice (row-major, row=Z, col=X).
func (g *Grid) Snapshot() (cells []float64, cols, rows int) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]float64, len(g.cells))
	copy(out, g.cells)
	return out, g.cols, g.rows
}

// Dims returns (cols, rows, cellSize, originX, originZ).
func (g *Grid) Dims() (int, int, float64, float64, float64) {
	return g.cols, g.rows, g.cellSize, g.originX, g.originZ
}

// Resize rebuilds the grid for new room dimensions, discarding existing data.
func (g *Grid) Resize(width, depth, cellSize, originX, originZ float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	cols := int(math.Ceil(width / cellSize))
	rows := int(math.Ceil(depth / cellSize))
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	g.cols = cols
	g.rows = rows
	g.cellSize = cellSize
	g.originX = originX
	g.originZ = originZ
	g.cells = make([]float64, cols*rows)
}
