// Package fusion provides 3D Fresnel zone weighted multi-link spatial localization.
package fusion

import (
	"math"
	"sync"
)

const defaultCellSize = 0.2 // metres

// Grid3D is a 3D voxel grid for accumulating link activation weights.
// Axes: X (width), Y (height), Z (depth).
type Grid3D struct {
	mu       sync.RWMutex
	cells    []float64
	nx, ny, nz int
	cellSize   float64
	ox, oy, oz float64 // origin (min corner)
}

// NewGrid3D creates a voxel grid covering the given room dimensions.
// width/height/depth in metres; origin is the minimum-corner in world space.
func NewGrid3D(width, height, depth, cellSize, ox, oy, oz float64) *Grid3D {
	if cellSize <= 0 {
		cellSize = defaultCellSize
	}
	nx := max1(int(math.Ceil(width / cellSize)))
	ny := max1(int(math.Ceil(height / cellSize)))
	nz := max1(int(math.Ceil(depth / cellSize)))
	return &Grid3D{
		cells:    make([]float64, nx*ny*nz),
		nx:       nx,
		ny:       ny,
		nz:       nz,
		cellSize: cellSize,
		ox:       ox,
		oy:       oy,
		oz:       oz,
	}
}

func max1(v int) int {
	if v < 1 {
		return 1
	}
	return v
}

// Reset zeroes all voxels.
func (g *Grid3D) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	for i := range g.cells {
		g.cells[i] = 0
	}
}

// idx returns the flat index for voxel (ix, iy, iz).
func (g *Grid3D) idx(ix, iy, iz int) int {
	return iz*g.ny*g.nx + iy*g.nx + ix
}

// AddLinkInfluence paints the Fresnel-zone influence of a single TX-RX link.
//
// TX is at (ax, ay, az), RX at (bx, by, bz), all in metres.
// activation is the deltaRMS value for this link (must be > 0).
//
// For each voxel centre P the contribution is:
//
//	excess = dist(A,P) + dist(P,B) - dist(A,B)   (excess path length)
//
// The first Fresnel zone radius at the midpoint of the link is:
//
//	r1 = sqrt(λ·d/4)   where d = dist(A,B), λ = 0.125 m (2.4 GHz)
//
// We define a "normalised excess" ne = excess / r1.
// When ne <= 1 the voxel is inside the first Fresnel zone.
//
// Weight = activation / (1 + ne)  — inverse distance to the ellipsoid surface.
// This peaks sharply at ne=0 (voxel on the direct path) and falls off as
// the voxel moves away from the ellipsoid.
func (g *Grid3D) AddLinkInfluence(ax, ay, az, bx, by, bz, activation float64) {
	if activation <= 0 {
		return
	}

	dx := bx - ax
	dy := by - ay
	dz := bz - az
	ab := math.Sqrt(dx*dx + dy*dy + dz*dz)
	if ab < 0.1 {
		return // degenerate link
	}

	// First Fresnel zone semi-axis (λ = 0.125 m at 2.4 GHz).
	const lambda = 0.125
	r1 := math.Sqrt(lambda * ab / 4.0)
	if r1 < 0.1 {
		r1 = 0.1
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	for iz := 0; iz < g.nz; iz++ {
		pz := g.oz + (float64(iz)+0.5)*g.cellSize
		for iy := 0; iy < g.ny; iy++ {
			py := g.oy + (float64(iy)+0.5)*g.cellSize
			for ix := 0; ix < g.nx; ix++ {
				px := g.ox + (float64(ix)+0.5)*g.cellSize

				dAP := math.Sqrt((px-ax)*(px-ax) + (py-ay)*(py-ay) + (pz-az)*(pz-az))
				dPB := math.Sqrt((px-bx)*(px-bx) + (py-by)*(py-by) + (pz-bz)*(pz-bz))
				excess := dAP + dPB - ab
				if excess < 0 {
					excess = 0
				}
				ne := excess / r1
				weight := activation / (1.0 + ne)
				g.cells[g.idx(ix, iy, iz)] += weight
			}
		}
	}
}

// Normalize scales the grid so the maximum voxel value is 1.0.
// Returns false if all voxels are zero.
func (g *Grid3D) Normalize() bool {
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
	inv := 1.0 / maxVal
	for i := range g.cells {
		g.cells[i] *= inv
	}
	return true
}

// Peaks returns the top-N local maxima as (x, y, z, weight) tuples.
// A voxel is a local maximum if it exceeds threshold and is strictly greater
// than all 26-connected neighbours.
func (g *Grid3D) Peaks(n int, threshold float64) [][4]float64 {
	g.mu.RLock()
	defer g.mu.RUnlock()

	type peak struct{ x, y, z, w float64 }
	var candidates []peak

	for iz := 1; iz < g.nz-1; iz++ {
		for iy := 1; iy < g.ny-1; iy++ {
			for ix := 1; ix < g.nx-1; ix++ {
				v := g.cells[g.idx(ix, iy, iz)]
				if v < threshold {
					continue
				}
				isMax := true
			outer:
				for dz := -1; dz <= 1; dz++ {
					for dy := -1; dy <= 1; dy++ {
						for dx := -1; dx <= 1; dx++ {
							if dx == 0 && dy == 0 && dz == 0 {
								continue
							}
							if g.cells[g.idx(ix+dx, iy+dy, iz+dz)] > v {
								isMax = false
								break outer
							}
						}
					}
				}
				if isMax {
					px := g.ox + (float64(ix)+0.5)*g.cellSize
					py := g.oy + (float64(iy)+0.5)*g.cellSize
					pz := g.oz + (float64(iz)+0.5)*g.cellSize
					candidates = append(candidates, peak{px, py, pz, v})
				}
			}
		}
	}

	// Sort descending by weight (insertion sort — candidate count is small).
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0 && candidates[j].w > candidates[j-1].w; j-- {
			candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
		}
	}

	if n > len(candidates) {
		n = len(candidates)
	}
	out := make([][4]float64, n)
	for i := 0; i < n; i++ {
		c := candidates[i]
		out[i] = [4]float64{c.x, c.y, c.z, c.w}
	}
	return out
}

// Dims returns (nx, ny, nz, cellSize, ox, oy, oz).
func (g *Grid3D) Dims() (int, int, int, float64, float64, float64, float64) {
	return g.nx, g.ny, g.nz, g.cellSize, g.ox, g.oy, g.oz
}

// Snapshot returns a copy of all voxel values (z-major, then y, then x).
func (g *Grid3D) Snapshot() []float64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]float64, len(g.cells))
	copy(out, g.cells)
	return out
}
