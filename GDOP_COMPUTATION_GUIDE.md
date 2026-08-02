# GDOP Computation Guide for SetGDOPImprovementAccessor

## Overview

This document explains how to compute GDOP (Geometric Dilution of Precision) for node layout evaluation in the spaxel codebase. It provides the exact function signatures and call patterns needed to implement `SetGDOPImprovementAccessor`.

---

## 1. Primary GDOP Computation Functions

### Location
**File:** `mothership/internal/simulator/gdop.go`

### Core Function: `computeGDOPAngular`

**Signature:**
```go
func (gc *GDOPComputer) computeGDOPAngular(point Point, links []Link) float64
```

**Purpose:** Computes GDOP at a specific point using the Fisher information matrix formula from plan.md Component 11 (lines 1093-1109).

**Algorithm (Fisher Information Matrix):**

```go
// Step 1: Collect link angles (TX→RX direction projected to floor plane)
theta_i = atan2(RX.Y - TX.Y, RX.X - TX.X)

// Step 2: Build 2×2 Fisher information matrix F
F = Σ_i [ [cos²(θ_i),       cos(θ_i)·sin(θ_i)],
           [cos(θ_i)·sin(θ_i), sin²(θ_i)       ] ]

// Step 3: Compute determinant
det_F = F[0][0]·F[1][1] - F[0][1]·F[1][0]

// Step 4: Check for degenerate geometry (collinear links)
if det_F ≤ 1e-6:
    return Infinity  // No coverage

// Step 5: Compute trace of F^-1 (using 2×2 inverse formula)
trace_Finv = (F[0][0] + F[1][1]) / det_F

// Step 6: GDOP = sqrt(trace(F^-1))
GDOP = sqrt(trace_Finv)
```

**Parameters:**
- `point Point`: The 3D position (X, Y, Z) to evaluate GDOP at
- `links []Link`: List of TX→RX links that cover this point

**Returns:**
- `float64`: GDOP value (lower is better). Returns `math.Inf(1)` if no coverage or degenerate geometry.

**GDOP Quality Thresholds:**
- GDOP < 2.0: "excellent" (green)
- GDOP 2-4: "good" (yellow)
- GDOP 4-8: "fair" (orange)
- GDOP > 8: "poor" (red)
- GDOP = Infinity: "none" (gray, no coverage)

---

## 2. Grid-Level GDOP Computation

### Function: `ComputeAll`

**Signature:**
```go
func (gc *GDOPComputer) ComputeAll() [][]GDOPResult
```

**Purpose:** Computes GDOP for all cells in the configured grid.

**Returns:**
- `[][]GDOPResult`: 2D grid of GDOP results indexed by `[row][col]`

**GDOPResult Structure:**
```go
type GDOPResult struct {
    X, Y, Z           float64  // Cell center position
    GDOP              float64  // Computed GDOP value (Infinity = no coverage)
    Quality           string   // "excellent", "good", "fair", "poor", "none"
    ContributingLinks []string // Link IDs that contributed to this cell
}
```

### Function: `ComputeAt`

**Signature:**
```go
func (gc *GDOPComputer) ComputeAt(x, y, z float64) GDOPResult
```

**Purpose:** Computes GDOP at a single specific point.

**Parameters:**
- `x, y, z float64`: Position in meters

---

## 3. Worst-Case GDOP Computation

### Function: `GetWorstCoverageCells`

**Signature:**
```go
func (gc *GDOPComputer) GetWorstCoverageCells(results [][]GDOPResult, n int) []GDOPResult
```

**Purpose:** Returns the N cells with the worst (highest) GDOP values.

**Usage for Worst-Case GDOP:**
```go
// Compute full grid GDOP
results := gdopComputer.ComputeAll()

// Get worst cell (single worst-case)
worstCells := gdopComputer.GetWorstCoverageCells(results, 1)
if len(worstCells) > 0 {
    worstGDOP := worstCells[0].GDOP
    // Use worstGDOP for layout evaluation
}
```

---

## 4. Creating a GDOPComputer

### Constructor: `NewGDOPComputer`

**Signature:**
```go
func NewGDOPComputer(links []Link, config GridConfig) *GDOPComputer
```

**Parameters:**
- `links []Link`: List of TX→RX links (all active sensing links in the layout)
- `config GridConfig`: Grid configuration defining the computation area

**GridConfig Structure:**
```go
type GridConfig struct {
    CellSize   float64 // Grid cell size in meters (default 0.2)
    MinX, MinY float64 // Grid origin (bottom-left corner)
    Width      float64 // Grid width in meters
    Depth      float64 // Grid depth in meters
}
```

**Example:**
```go
// Create grid configuration covering room
gridConfig := simulator.GridConfig{
    CellSize: 0.2,            // 20cm cells
    MinX:     room.OriginX,
    MinY:     room.OriginY,
    Width:    room.Width,
    Depth:    room.Depth,
}

// Create GDOP computer with all TX→RX links
gdopComputer := simulator.NewGDOPComputer(allLinks, gridConfig)
```

---

## 5. Link Structure

**Location:** `mothership/internal/simulator/types.go`

```go
type Link struct {
    TX Node // Transmitting node
    RX Node // Receiving node
}

type Node struct {
    ID       string    // Node MAC or identifier
    Name     string    // Human-readable name
    Position Point     // Node position (X, Y, Z)
}

type Point struct {
    X, Y, Z float64
}
```

---

## 6. Fresnel Zone Filtering

**Function:** `IsInFresnelZones` (referenced in `ComputeAt`)

**Purpose:** Determines if a point is within the first N Fresnel zones of a link.

**Usage in GDOP computation:**
```go
// In ComputeAt, lines 83-88:
for _, link := range gc.links {
    if IsInFresnelZones(link.TX.Position, link.RX.Position, point, gc.maxZone) {
        coveringLinks = append(coveringLinks, link)
        linkIDs = append(linkIDs, link.TX.ID+":"+link.RX.ID)
    }
}
```

**Parameters:**
- `txPos, rxPos Point`: TX and RX node positions
- `point Point`: Point to test
- `maxZone int`: Maximum Fresnel zone number (default: 3)

**Returns:** `true` if the point is within the specified Fresnel zones of the link.

---

## 7. Call Pattern for "Before" and "After" GDOP

### Scenario: Evaluating a Hypothetical Node Move

```go
// Step 1: Get current node positions from registry
currentNodes := fh.registry.GetAllNodes()  // or similar

// Step 2: Build current link set (all active TX→RX pairs)
currentLinks := buildLinksFromNodes(currentNodes)

// Step 3: Create GDOP computer for current layout
gridConfig := simulator.GridConfig{
    CellSize: 0.2,
    MinX:     room.OriginX,
    MinY:     room.OriginY,
    Width:    room.Width,
    Depth:    room.Depth,
}
gdopComputerBefore := simulator.NewGDOPComputer(currentLinks, gridConfig)

// Step 4: Compute current (before) GDOP
resultsBefore := gdopComputerBefore.ComputeAll()
worstCellsBefore := gdopComputerBefore.GetWorstCoverageCells(resultsBefore, 1)
beforeGDOP := worstCellsBefore[0].GDOP  // Worst-case GDOP before move

// Step 5: Create hypothetical layout with node at new position
hypotheticalNodes := makeCopyWithNodeMoved(currentNodes, nodeMAC, newX, newY, newZ)
hypotheticalLinks := buildLinksFromNodes(hypotheticalNodes)

// Step 6: Create GDOP computer for hypothetical layout
gdopComputerAfter := simulator.NewGDOPComputer(hypotheticalLinks, gridConfig)

// Step 7: Compute hypothetical (after) GDOP
resultsAfter := gdopComputerAfter.ComputeAll()
worstCellsAfter := gdopComputerAfter.GetWorstCoverageCells(resultsAfter, 1)
afterGDOP := worstCellsAfter[0].GDOP  // Worst-case GDOP after move

// Step 8: Compare and compute improvement
improvement := beforeGDOP - afterGDOP
if improvement > 0 {
    // Move improves coverage
    fmt.Printf("Moving node %s to (%.2f, %.2f, %.2f) improves worst GDOP by %.2f\n",
        nodeMAC, newX, newY, newZ, improvement)
}
```

---

## 8. Helper Functions for Layout Evaluation

### Coverage Score

**Signature:**
```go
func (gc *GDOPComputer) CoverageScore(results [][]GDOPResult) float64
```

**Returns:** Percentage (0-100) of cells with "good" or better coverage (GDOP < 4).

### Average GDOP

**Signature:**
```go
func (gc *GDOPComputer) AverageGDOP(results [][]GDOPResult) float64
```

**Returns:** Average GDOP over all cells (excluding infinity values).

### Accuracy Estimation

**Signature:**
```go
func ExpectedAccuracy(gdop float64) float64
```

**Returns:** Expected localization accuracy in meters based on GDOP value.
- Formula: `baseAccuracy * gdop` where `baseAccuracy = 0.5m` for GDOP = 1
- Returns `Infinity` if GDOP is infinite (no coverage)

---

## 9. Integration with Fleet Management

### Using FleetHealer.GetWorstCoverageZone

**Location:** `mothership/internal/fleet/healer.go` (lines 620-663)

**Signature:**
```go
func (fh *FleetHealer) GetWorstCoverageZone() (x, z, gdop float64)
```

**Purpose:** Returns the position and GDOP value of the worst-covered zone in the current layout.

**Returns:**
- `x, z`: Room coordinates of worst cell center
- `gdop`: GDOP value at that position

**Usage:**
```go
worstX, worstZ, worstGDOP := fleetHealer.GetWorstCoverageZone()
fmt.Printf("Worst coverage at (%.2f, %.2f) with GDOP %.2f\n", worstX, worstZ, worstGDOP)
```

### Using FleetHealer.SuggestNodePosition

**Location:** `mothership/internal/fleet/healer.go` (lines 666-735)

**Signature:**
```go
func (fh *FleetHealer) SuggestNodePosition() (x, z, improvement float64)
```

**Purpose:** Suggests optimal position for a new node to maximize coverage improvement.

**Returns:**
- `x, z`: Suggested coordinates for new node
- `improvement`: Expected worst-GDOP improvement from adding node at this position

---

## 10. Complete Example: SetGDOPImprovementAccessor Implementation

```go
package fleet

import (
    "math"
    "spaxel/mothership/internal/simulator"
)

// SetGDOPImprovementAccessor computes the GDOP improvement that would result
// from moving a node to a hypothetical position.
func (fh *FleetHealer) SetGDOPImprovementAccessor(
    nodeMAC string,
    currentX, currentY, currentZ float64,
    hypotheticalX, hypotheticalY, hypotheticalZ float64,
) (currentGDOP, hypotheticalGDOP, improvement float64, err error) {
    fh.mu.Lock()
    defer fh.mu.Unlock()

    // Step 1: Get current positions and links
    if fh.gdopCalc == nil {
        return 0, 0, 0, fmt.Errorf("GDOP calculator not available")
    }

    // Get current node positions
    positions := make([]NodePosition, 0, len(fh.nodePositions))
    for mac, pos := range fh.nodePositions {
        positions = append(positions, pos)
    }

    // Compute current (before) GDOP map
    gdopMapBefore, cols, rows := fh.gdopCalc.GDOPMap(positions)
    if len(gdopMapBefore) == 0 {
        return 0, 0, 0, fmt.Errorf("unable to compute current GDOP")
    }

    // Find worst-case GDOP in current layout
    currentGDOP = computeWorstGDOP(gdopMapBefore)

    // Step 2: Create hypothetical layout with node moved
    hypotheticalPositions := make([]NodePosition, len(positions))
    copy(hypotheticalPositions, positions)

    // Update the target node's position
    for i, pos := range hypotheticalPositions {
        if pos.MAC == nodeMAC {
            hypotheticalPositions[i] = NodePosition{
                MAC: nodeMAC,
                X:   hypotheticalX,
                Z:   hypotheticalZ,
            }
            break
        }
    }

    // Step 3: Compute hypothetical (after) GDOP map
    gdopMapAfter, _, _ := fh.gdopCalc.GDOPMap(hypotheticalPositions)
    if len(gdopMapAfter) == 0 {
        return 0, 0, 0, fmt.Errorf("unable to compute hypothetical GDOP")
    }

    // Find worst-case GDOP in hypothetical layout
    hypotheticalGDOP = computeWorstGDOP(gdopMapAfter)

    // Step 4: Compute improvement
    improvement = currentGDOP - hypotheticalGDOP

    return currentGDOP, hypotheticalGDOP, improvement, nil
}

// computeWorstGDOP extracts the worst (maximum) GDOP value from a GDOP map
func computeWorstGDOP(gdopMap []float32) float64 {
    worstGDOP := 0.0
    for _, gdop := range gdopMap {
        g := float64(gdop)
        if math.IsInf(g, 0) {
            return math.Inf(1)  // Infinity is worst possible
        }
        if g > worstGDOP {
            worstGDOP = g
        }
    }
    return worstGDOP
}
```

---

## 11. Key Types and Interfaces

### GDOPCalculator Interface

**Location:** `mothership/internal/fleet/optimiser.go` (lines 13-16)

```go
type GDOPCalculator interface {
    GDOPMap(positions []NodePosition) ([]float32, int, int)
}
```

**Purpose:** Interface for computing a flattened GDOP map from node positions.

**Methods:**
- `GDOPMap(positions []NodePosition) ([]float32, int, int)`
  - **Returns:** Flattened GDOP array, grid columns, grid rows
  - **GDOP encoding:** 9999.0 = infinity (no coverage)

### NodePosition Structure

```go
type NodePosition struct {
    MAC string
    X   float64
    Z   float64
}
```

**Note:** Only X and Z are used (2D projection). Y (vertical) is not considered in current GDOP computation.

---

## 12. Summary

### For Computing GDOP Before/After a Node Move:

1. **Create a `GDOPComputer`** with `NewGDOPComputer(links, gridConfig)`
2. **Call `ComputeAll()`** to get full-grid GDOP results
3. **Call `GetWorstCoverageCells(results, 1)`** to get worst-case GDOP
4. **Compare** before/after worst-case GDOP values to compute improvement

### For Computing Coverage Score:

1. **Call `CoverageScore(results)`** on the grid results
2. **Returns:** Percentage (0-100) of cells with GDOP < 4

### For Finding Dead Zones:

1. **Call `FindDeadZones(results)`**
2. **Returns:** List of positions with "none" or "poor" coverage

---

## Appendix: Fisher Information Formula Reference

From plan.md Component 11 (lines 1093-1109):

```
For a cell at position P:
1. Collect all links (TX_i → RX_i) where P is within the first 3 Fresnel zones
2. If fewer than 2 qualifying links: GDOP = Infinity
3. For each qualifying link i: θ_i = atan2(RX_i.y - TX_i.y, RX_i.x - TX_i.x)
4. Build the 2×2 Fisher information matrix:
   F = Σ_i [ [cos²(θ_i),       cos(θ_i)·sin(θ_i)],
              [cos(θ_i)·sin(θ_i), sin²(θ_i)      ] ]
5. det_F = F[0][0]·F[1][1] - F[0][1]·F[1][0]
6. If det_F ≤ 1e-6: GDOP = Infinity (collinear links)
7. trace_Finv = (F[0][0] + F[1][1]) / det_F
8. GDOP = sqrt(trace_Finv)
```

This formula is implemented in `computeGDOPAngular` (lines 114-165 of `internal/simulator/gdop.go`).
