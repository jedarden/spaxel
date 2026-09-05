# GDOP Computation Functions Documentation

## Overview

Geometric Dilution of Precision (GDOP) quantifies how well the geometric arrangement of nodes can localize a point in space. Lower GDOP values indicate better coverage quality. The Spaxel system uses GDOP to evaluate and optimize node placement for spatial coverage.

## Core GDOP Computation Function

### Function: `computeGDOPAngular`

**Location:** `/home/coding/spaxel/mothership/internal/simulator/gdop.go` (lines 114-165)

**Signature:**
```go
func (gc *GDOPComputer) computeGDOPAngular(point Point, links []Link) float64
```

**Parameters:**
- `point Point` - The 3D position (X, Y, Z) where GDOP is computed. For 2D analysis, only X and Y are used.
- `links []Link` - Array of TX→RX links that cover the point (within maxZone Fresnel zones). Each link contains TX and RX node positions.

**Return Type:**
- `float64` - GDOP value at the specified point.
  - **< 2.0**: Excellent coverage (±0.5m accuracy expected)
  - **2.0-4.0**: Good coverage (±1.0m accuracy expected)
  - **4.0-8.0**: Fair coverage (±2-4m accuracy expected)
  - **> 8.0**: Poor coverage (>±4m accuracy)
  - **Infinity**: No coverage (insufficient nodes or degenerate geometry)

**Mathematical Background:**

GDOP is computed from the Fisher information matrix F = HᵀH, where H contains direction cosines from each link to the target point. For 2D localization:

```
GDOP = sqrt(trace(F⁻¹))
```

Where:
- `H[i] = [(px-nx)/d, (pz-nz)/d]` for link i at (nx, nz)
- `d = sqrt((px-nx)² + (pz-nz)²)` is distance to node
- `F` is a 2×2 symmetric matrix accumulated over all links
- `trace(F⁻¹) = (F[0,0] + F[1,1]) / det(F)`

**Algorithm Steps:**

1. **Collect link angles:** For each link, compute the angle θ of the line from TX to RX as seen from the point (projected to floor plane):
   ```go
   theta = atan2(RY - TY, RX - TX)
   ```

2. **Build Fisher information matrix:**
   ```go
   F = Σ [ [cos²(θ),       cos(θ)·sin(θ)],
           [cos(θ)·sin(θ), sin²(θ)       ] ]
   ```

3. **Compute determinant:**
   ```go
   det = f00*f11 - f01*f01
   ```

4. **Check for degenerate geometry:** If `det <= 1e-6`, links are collinear → return Infinity

5. **Compute trace of F⁻¹:**
   ```go
   traceFInv = (f00 + f11) / det
   ```

6. **Return GDOP:**
   ```go
   GDOP = sqrt(traceFInv)
   ```

**Example Usage:**
```go
// Create links from nodes
links := []Link{
    {TX: Node{Position: Point{X: 0, Y: 0}}, RX: Node{Position: Point{X: 10, Y: 0}}},
    {TX: Node{Position: Point{X: 0, Y: 10}}, RX: Node{Position: Point{X: 10, Y: 10}}},
}

// Compute GDOP at center of room (5m, 5m)
gdop := computeGDOPAngular(Point{X: 5, Y: 5, Z: 1.0}, links)
// Returns ~1.58 (excellent coverage)
```

**Performance:** O(n) where n = number of links

---

## Supporting GDOP Functions

### Function: `computeGDOPImprovement`

**Location:** `/home/coding/spaxel/mothership/internal/simulator/gdop.go` (lines 712-774)

**Signature:**
```go
func computeGDOPImprovement(currentLayout []*Node, nodeMAC string, targetPos Point) float64
```

**Purpose:** Evaluates how much the overall coverage would improve if a specific node were moved to a new position.

**Parameters:**
- `currentLayout []*Node` - Array of all nodes in their current positions (minimum 2 required)
- `nodeMAC string` - MAC address or ID of the node to move
- `targetPos Point` - Target position to move the node to

**Return Type:**
- `float64` - Relative improvement in range [-1.0, 1.0]:
  - **Positive (0 to 1)**: Improvement (lower GDOP is better)
  - **Negative (-1 to 0)**: Degradation (higher GDOP is worse)
  - **0.0**: No change, node not found, or no coverage baseline
  - **1.0**: Maximum improvement (reduced to near-zero GDOP)
  - **-1.0**: Complete coverage loss at target position

**Example Usage:**
```go
layout := []*Node{
    NewNode("node1", "Node 1", NodeTypeVirtual, Point{X: 0, Y: 0, Z: 2.0}),
    NewNode("node2", "Node 2", NodeTypeVirtual, Point{X: 10, Y: 0, Z: 2.0}),
    NewNode("node3", "Node 3", NodeTypeVirtual, Point{X: 0, Y: 10, Z: 2.0}),
}

// Evaluate moving node1 to center
targetPos := Point{X: 5, Y: 5, Z: 2.0}
improvement := computeGDOPImprovement(layout, "node1", targetPos)
// Returns ~0.3 (30% improvement) for moving corner node to center
```

---

### Function: `computeWorstGDOP`

**Location:** `/home/coding/spaxel/mothership/internal/simulator/gdop.go` (lines 815-908)

**Signature:**
```go
func computeWorstGDOP(nodes []*Node) float64
```

**Purpose:** Finds the worst-case GDOP value across all grid cells for a given node layout. A good layout should have a low worst-case GDOP, indicating that even the worst-covered area has reasonable localization accuracy.

**Parameters:**
- `nodes []*Node` - Array of nodes to evaluate (minimum 2 required)

**Return Type:**
- `float64` - Worst-case GDOP value:
  - **< 2.0**: Excellent layout (worst area still has good coverage)
  - **2.0-4.0**: Good layout
  - **4.0-8.0**: Fair layout (some areas with poor coverage)
  - **> 8.0**: Poor layout (significant coverage gaps)
  - **Infinity**: No coverage (insufficient nodes or links)

**Algorithm Steps:**

1. Generate all links from the node set (TX→RX pairs)
2. Create a grid covering the space bounded by node positions ± 1m margin
3. Compute GDOP for each cell using angular diversity of covering links
4. Return the maximum GDOP found (worst coverage)

**Example Usage:**
```go
nodes := []*Node{
    NewNode("node1", "Node 1", NodeTypeVirtual, Point{X: 0, Y: 0, Z: 2.0}),
    NewNode("node2", "Node 2", NodeTypeVirtual, Point{X: 10, Y: 0, Z: 2.0}),
    NewNode("node3", "Node 3", NodeTypeVirtual, Point{X: 0, Y: 10, Z: 2.0}),
    NewNode("node4", "Node 4", NodeTypeVirtual, Point{X: 10, Y: 10, Z: 2.0}),
}

worstGDOP := computeWorstGDOP(nodes)
// Returns ~1.8 for well-positioned 4-node corner layout
```

---

## GDOPComputer Type

The `GDOPComputer` type provides methods for grid-based GDOP computation:

### Constructor: `NewGDOPComputer`

```go
func NewGDOPComputer(links []Link, config GridConfig) *GDOPComputer
```

**Parameters:**
- `links []Link` - Array of TX→RX links
- `config GridConfig` - Grid configuration:
  - `CellSize float64` - Grid cell size in meters (default 0.2)
  - `MinX, MinY float64` - Grid origin
  - `Width, Depth float64` - Grid dimensions

### Key Methods

**`ComputeAt(x, y, z float64) GDOPResult`**
- Computes GDOP at a specific point
- Returns `GDOPResult` struct with position, GDOP value, quality level, and contributing links

**`ComputeAll() [][]GDOPResult`**
- Computes GDOP for the entire grid
- Returns 2D array of GDOP results indexed by cell position

**`CoverageScore(results [][]GDOPResult) float64`**
- Computes percentage of cells with "good" or better coverage
- Returns value from 0-100

**`AverageGDOP(results [][]GDOPResult) float64`**
- Computes average GDOP over all cells (excluding infinity)

---

## Quality Thresholds

```go
func gdopToQuality(gdop float64) string {
    if math.IsInf(gdop, 0) {
        return "none"
    }
    if gdop < 2.0 {
        return "excellent"  // ±0.5m accuracy expected
    }
    if gdop < 4.0 {
        return "good"       // ±1.0m accuracy expected
    }
    if gdop < 8.0 {
        return "fair"       // ±2-4m accuracy expected
    }
    return "poor"           // >±4m accuracy or no coverage
}
```

---

## Implementation Files

- **Main Implementation:** `/home/coding/spaxel/mothership/internal/simulator/gdop.go`
- **Tests:** `/home/coding/spaxel/mothership/internal/simulator/gdop_test.go`
- **Documentation/Examples:** `/home/coding/spaxel/mothership/internal/localization/gdop_example.go`

---

## Test Coverage

The implementation includes comprehensive tests in `gdop_test.go`:

1. **TestComputeGDOPImprovement** - Tests GDOP improvement computation:
   - Different positions yield different improvements
   - Poor positions yield negative improvement
   - Node not found returns 0.0
   - Empty/single-node layouts handled gracefully
   - Results clamped to [-1.0, 1.0]
   - Lookup by MAC address works

2. **TestComputeWorstGDOP** - Tests worst-case GDOP computation:
   - Good geometry yields low GDOP
   - Insufficient nodes return infinity
   - Collinear nodes yield higher GDOP than well-positioned triangular nodes

Run tests with:
```bash
cd mothership && go test ./internal/simulator/...
```

---

## Use Cases in Spaxel

1. **Live Coverage Painting:** Real-time GDOP computation during node placement in 3D editor
2. **Pre-deployment Simulator:** Evaluate node placement before hardware purchase
3. **Self-healing Fleet:** Automatic role re-optimization based on GDOP analysis
4. **Node Position Recommendations:** Suggest optimal positions for additional nodes
5. **Coverage Quality Dashboard:** Display coverage score and dead zones

---

## Fleet-Side GDOP Integration

*Consolidated 2026-09-04 from the root-level `GDOP_COMPUTATION_GUIDE.md` (deleted;
its simulator-side sections duplicated this document). Line numbers verified
against HEAD at consolidation time.*

### GDOPCalculator interface

```go
// mothership/internal/fleet/healer.go
type GDOPCalculator interface {
    GDOPMap(positions []NodePosition) ([]float32, int, int)
}

type NodePosition struct {
    MAC string
    X   float64
    Z   float64
}
```

Only X and Z are used — the fleet-side GDOP model is a 2D floor-plane projection.
A concrete implementation lives at `mothership/internal/localization/fusion.go:248`
(`Engine.GDOPMap`), which evaluates `computeGDOP` per 0.2 m grid cell; main.go
bridges it into the fleet package through the `gdopAdapter` wrapper:

```go
// mothership/cmd/mothership/main.go (self-heal wiring)
gdopCalc := &gdopAdapter{eng: selfImprovingLocalizer.GetEngine()}
selfHealManager.SetGDOPCalculator(gdopCalc)
roleOptimiser.SetGDOPCalculator(gdopCalc)
```

`SelfHealManager.SetGDOPCalculator` (`mothership/internal/fleet/selfheal.go:124`)
also forwards to the internal optimiser.

### Map encoding

The flattened map returned by `GDOPMap` uses **9999.0 as the infinity / no-coverage
sentinel** (see `mothership/internal/simulator/engine.go:508,557,570` and
`mothership/internal/simulator/gdop.go:537`). Treat any value above the worst real
threshold (GDOP > 8, "poor") as effectively uncovered.

### FleetHealer entry points

```go
// mothership/internal/fleet/healer.go:620
func (fh *FleetHealer) GetWorstCoverageZone() (x, z, gdop float64)
```
Scans the current GDOP grid for the worst (maximum) cell and returns its room
coordinates as cell centres (`room.OriginX/OriginZ + (col|row + 0.5) * 0.2m`).
Sentinel: `(0, 0, 10)` when no calculator is wired or fewer than two node
positions are known.

```go
// mothership/internal/fleet/healer.go:665
func (fh *FleetHealer) SuggestNodePosition() (x, z float64, improvement float64)
```
Grid-searches the room for the placement of an additional node with the best
expected worst-GDOP improvement, and returns the chosen coordinates together
with that improvement.

### Before/after a node move

To evaluate a hypothetical move (the pattern `SuggestNodePosition` embodies):

1. Collect current `NodePosition` values and call `GDOPMap` → worst cell = current GDOP.
2. Copy the position slice and overwrite the moved node's `X`/`Z`.
3. Call `GDOPMap` on the hypothetical layout → worst cell = after GDOP.
4. `improvement = currentGDOP − afterGDOP` (positive means the move helps).

### Diagnostic accessor

```go
// mothership/internal/diagnostics/linkweather.go:155
func (de *DiagnosticEngine) SetGDOPImprovementAccessor(fn func(nodeMAC string, targetPos Vec3) float64)
```
Registers the callback the diagnostics engine uses to estimate the GDOP impact of
repositioning a node; wired in `mothership/cmd/mothership/main.go:2238`. Note the
receiver is `DiagnosticEngine`, not `FleetHealer`.
