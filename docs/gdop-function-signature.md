# GDOP Computation Function Signature Documentation

## Overview

GDOP (Geometric Dilution of Precision) computation in Spaxel is implemented in the `mothership/internal/simulator/gdop.go` file. This document provides comprehensive documentation of function signatures, parameters, return types, and usage examples.

## Primary GDOP Computation Function

### Function: `computeGDOPAngular`

**Location:** `mothership/internal/simulator/gdop.go:296-347`

**Signature:**
```go
func (gc *GDOPComputer) computeGDOPAngular(point Point, links []Link) float64
```

**Purpose:** Computes GDOP based on angular diversity of link directions using the 2D Fisher Information Matrix approach.

**Parameters:**
- `point Point`: The 3D position (X, Y, Z) at which to compute GDOP
  - Only X and Y are used for 2D angular diversity analysis (Z is projected to floor plane)
  - Must have finite, non-NaN coordinates
  - `Point` struct: `type Point struct { X, Y, Z float64 }`

- `links []Link`: Slice of Link objects covering the point
  - Each link contains TX and RX node positions
  - Must have at least 2 links for meaningful GDOP calculation
  - Links should already be filtered to those that cover the point (within maxZone Fresnel zones)
  - `Link` struct contains `TX` and `RX` nodes with positions

**Return Type:** `float64`
- GDOP value indicating geometric dilution of precision
- `< 2.0`: Excellent coverage (high angular diversity)
- `2.0-4.0`: Good coverage
- `4.0-8.0`: Fair coverage
- `> 8.0`: Poor coverage
- `Infinity`: Degenerate geometry (collinear links) or insufficient links (< 2)

**Algorithm Implementation:**
```go
// Step 1: Collect link angles (TX→RX direction projected to floor plane)
// Step 2: Build Fisher information matrix F = Σ [[cos²θ, cosθ·sinθ], [cosθ·sinθ, sin²θ]]
// Step 3: Compute determinant det(F) = f00*f11 - f01²
// Step 4: Check for degenerate geometry (det ≤ 1e-6 → return Infinity)
// Step 5: Compute trace of F^-1 = (f00 + f11) / det
// Step 6: GDOP = sqrt(trace(F^-1))
```

**Example Usage:**
```go
computer := NewGDOPComputer(links, GridConfig{...})
point := Point{X: 5.0, Y: 5.0, Z: 1.0}
links := []Link{link1, link2, link3} // pre-filtered by Fresnel coverage
gdop := computer.computeGDOPAngular(point, links)
// Returns ~1.5 for 3 well-diverse links
```

## Supporting GDOP Functions

### Function: `NewGDOPComputer`

**Location:** `mothership/internal/simulator/gdop.go:84-93`

**Signature:**
```go
func NewGDOPComputer(links []Link, config GridConfig) *GDOPComputer
```

**Purpose:** Creates a new GDOP computer for coverage analysis.

**Parameters:**
- `links []Link`: Slice of Link objects representing TX→RX communication pairs
  - Each link contains TX and RX nodes with positions
  - Can be empty (but ComputeAll/ComputeAt will return Infinity)
  - Must contain non-nil nodes with valid positions
  
- `config GridConfig`: GridConfig defining the spatial grid for GDOP evaluation
  - `CellSize`: Grid cell size in meters (must be > 0, defaults to 0.2 if ≤ 0)
  - `MinX, MinY`: Grid origin coordinates in meters (can be negative)
  - `Width`: Grid width in meters (must be > 0)
  - `Depth`: Grid depth in meters (must be > 0)

**Return Type:** `*GDOPComputer`
- Initialized GDOPComputer pointer with links, config, and maxZone=3
- Config CellSize defaults to 0.2 if ≤ 0
- Heap-allocated (caller responsible for lifecycle)

### Function: `ComputeAt`

**Location:** `mothership/internal/simulator/gdop.go:217-251`

**Signature:**
```go
func (gc *GDOPComputer) ComputeAt(x, y, z float64) GDOPResult
```

**Purpose:** Computes GDOP at a specific 3D point in the space.

**Parameters:**
- `x float64`: X coordinate in meters (floor plan position, must be finite)
- `y float64`: Y coordinate in meters (floor plan position, must be finite)
- `z float64`: Z coordinate in meters (height, used for Fresnel zone calculation but projected to floor plane for 2D GDOP analysis, must be finite)

**Return Type:** `GDOPResult`
```go
type GDOPResult struct {
    X, Y, Z           float64  // Cell center position
    GDOP              float64  // Computed GDOP value (Infinity = no coverage)
    Quality           string   // "excellent", "good", "fair", "poor", "none"
    ContributingLinks []string // Link IDs that contributed to this cell
}
```

**Quality Thresholds:**
- `"excellent"`: GDOP < 2.0 (high angular diversity, precise localization)
- `"good"`: 2.0 ≤ GDOP < 4.0 (acceptable localization accuracy)
- `"fair"`: 4.0 ≤ GDOP < 8.0 (degraded accuracy, coverage gaps)
- `"poor"`: 8.0 ≤ GDOP < Infinity (marginal coverage)
- `"none"`: GDOP = Infinity (< 2 covering links or collinear)

### Function: `ComputeAll`

**Location:** `mothership/internal/simulator/gdop.go:150-169`

**Signature:**
```go
func (gc *GDOPComputer) ComputeAll() [][]GDOPResult
```

**Purpose:** Computes GDOP for the entire grid defined by the GridConfig.

**Return Type:** `[][]GDOPResult`
- 2D slice of GDOPResult indexed by cell position (row, column)
- Outer slice: rows (Y dimension, depth), indexed by iy = 0 to ny-1
- Inner slice: columns (X dimension, width), indexed by ix = 0 to nx-1
- Result at [iy][ix] corresponds to cell at:
  - X = MinX + (ix + 0.5) * CellSize (cell center X)
  - Y = MinY + (iy + 0.5) * CellSize (cell center Y)
  - Z = 1.0 (fixed height for 2D GDOP analysis)

**Grid Dimensions:**
- `nx = ceil(Width / CellSize)` = number of columns
- `ny = ceil(Depth / CellSize)` = number of rows
- Total cells = nx * ny (capped by memory limits in practice)

## Analysis Functions

### Function: `computeWorstGDOP`

**Location:** `mothership/internal/simulator/gdop.go:1024-1110`

**Signature:**
```go
func computeWorstGDOP(nodes []*Node) float64
```

**Purpose:** Calculates the worst-case GDOP value across all grid cells for a given node layout.

**Parameters:**
- `nodes []*Node`: Slice of nodes to evaluate
  - Must have at least 2 non-nil, enabled nodes with valid positions
  - Nodes with nil pointers or disabled nodes are silently skipped
  - Must not be empty or contain only nil nodes

**Return Type:** `float64`
- Worst-case GDOP value (maximum across all grid cells)
- `< 2.0`: Excellent layout (worst area still has good coverage)
- `2.0-4.0`: Good layout (acceptable coverage everywhere)
- `4.0-8.0`: Fair layout (some areas with poor coverage)
- `> 8.0`: Poor layout (significant coverage gaps)
- `Infinity`: No coverage (insufficient nodes, links, or all cells uncovered)

### Function: `computeGDOPImprovement`

**Location:** `mothership/internal/simulator/gdop.go:908-970`

**Signature:**
```go
func computeGDOPImprovement(currentLayout []*Node, nodeMAC string, targetPos Point) float64
```

**Purpose:** Evaluates how much overall coverage would improve or degrade if a specific node were moved to a new target position.

**Parameters:**
- `currentLayout []*Node`: Slice of all nodes in their current positions
  - Must contain at least 2 nodes with valid (finite, non-NaN) positions
  - Nodes with nil or disabled nodes are skipped
  - Must not be empty
  
- `nodeMAC string`: MAC address (or ID) of the node to move
  - Must match either `node.ID` field or `node.GenerateMAC()` result
  - If no match found, returns 0.0 (no change)
  
- `targetPos Point`: Target position to move the node to
  - Must have finite X, Y, Z coordinates (no NaN/Inf)
  - Used as-is without bounds checking

**Return Type:** `float64`
- Relative improvement in range [-1.0, 1.0]
- Positive (0 to 1): Improvement (lower GDOP is better)
  - E.g., 0.3 = 30% improvement in worst-case coverage
- Negative (-1 to 0): Degradation (higher GDOP is worse)
  - E.g., -0.5 = 50% degradation in worst-case coverage
- `0.0`: No change (same GDOP), node not found, or current layout has no coverage
- `1.0`: Maximum improvement (worst GDOP reduced to near-zero)
- `-1.0`: Complete coverage loss (new GDOP = Infinity)

## Utility Functions

### Function: `ExpectedAccuracy`

**Location:** `mothership/internal/simulator/gdop.go:495-505`

**Signature:**
```go
func ExpectedAccuracy(gdop float64) float64
```

**Purpose:** Estimates the expected localization accuracy at a point based on its GDOP value.

**Parameters:**
- `gdop float64`: GDOP value (typically from GDOPResult.GDOP)

**Return Type:** `float64`
- Expected accuracy in meters
- `Infinity`: No coverage (gdop is Infinity)
- Formula: `0.5 * gdop` (base accuracy of ±0.5m for GDOP=1, scales linearly)

**Accuracy Estimates:**
- GDOP < 2: ±0.5m accuracy
- GDOP 2-4: ±1.0m accuracy  
- GDOP > 4: degrades further

### Function: `gdopToQuality`

**Location:** `mothership/internal/simulator/gdop.go:350-364`

**Signature:**
```go
func gdopToQuality(gdop float64) string
```

**Purpose:** Converts GDOP value to quality string.

**Parameters:**
- `gdop float64`: GDOP value to convert

**Return Type:** `string`
- `"none"`: gdop is Infinity
- `"excellent"`: gdop < 2.0
- `"good"`: 2.0 ≤ gdop < 4.0
- `"fair"`: 4.0 ≤ gdop < 8.0
- `"poor"`: gdop ≥ 8.0

### Function: `GDOPColorMap`

**Location:** `mothership/internal/simulator/gdop.go:514-528`

**Signature:**
```go
func GDOPColorMap(gdop float64) GDOPColor
```

**Purpose:** Returns the color for a given GDOP value for visualization.

**Parameters:**
- `gdop float64`: GDOP value to map to color

**Return Type:** `GDOPColor`
```go
type GDOPColor struct {
    R, G, B uint8 // RGB values 0-255
}
```

**Color Mapping:**
- Gray (#808080): No coverage (Infinity)
- Green (#22c65e): Excellent (GDOP < 2.0)
- Yellow (#ffc107): Good (2.0 ≤ GDOP < 4.0)
- Orange (#ff9200): Fair (4.0 ≤ GDOP < 8.0)
- Red (#dc3545): Poor (GDOP ≥ 8.0)

## Data Structures

### `GDOPComputer` Struct
```go
type GDOPComputer struct {
    links   []Link         // TX→RX communication pairs
    config  GridConfig     // Grid configuration
    maxZone int            // Maximum Fresnel zone to consider (default 3)
}
```

### `GDOPResult` Struct
```go
type GDOPResult struct {
    X, Y, Z           float64  // Cell center position
    GDOP              float64  // Computed GDOP value
    Quality           string   // Quality rating
    ContributingLinks []string // Link IDs that contributed
}
```

### `GridConfig` Struct
```go
type GridConfig struct {
    CellSize   float64 // Grid cell size in meters (default 0.2)
    MinX, MinY float64 // Grid origin in meters
    Width      float64 // Grid width in meters
    Depth      float64 // Grid depth in meters
}
```

## Complete Example Usage

```go
package main

import (
    "fmt"
    "spaxel/mothership/internal/simulator"
)

func main() {
    // Step 1: Create nodes at corners of a 10x10m room
    nodes := []*simulator.Node{
        simulator.NewNode("node1", "Kitchen North", simulator.NodeTypeVirtual,
            simulator.Point{X: 0, Y: 0, Z: 2.0}),
        simulator.NewNode("node2", "Kitchen South", simulator.NodeTypeVirtual,
            simulator.Point{X: 10, Y: 0, Z: 2.0}),
        simulator.NewNode("node3", "Living Room West", simulator.NodeTypeVirtual,
            simulator.Point{X: 0, Y: 10, Z: 2.0}),
        simulator.NewNode("node4", "Living Room East", simulator.NodeTypeVirtual,
            simulator.Point{X: 10, Y: 10, Z: 2.0}),
    }

    // Step 2: Set all nodes as TX_RX for bidirectional links
    for _, node := range nodes {
        node.Role = simulator.RoleTXRX
    }

    // Step 3: Generate links from nodes
    nodeSet := simulator.NewNodeSet()
    for _, node := range nodes {
        nodeSet.Add(node)
    }
    links := simulator.GenerateAllLinks(nodeSet)

    // Step 4: Create GDOP computer
    gc := simulator.NewGDOPComputer(links, simulator.GridConfig{
        MinX:     0.0,
        MinY:     0.0,
        Width:    10.0,
        Depth:    10.0,
        CellSize: 0.2, // 20cm grid cells
    })

    // Step 5: Compute GDOP at center of room
    result := gc.ComputeAt(5.0, 5.0, 1.0)

    fmt.Printf("GDOP at center (5,5): %.2f\n", result.GDOP)
    fmt.Printf("Quality: %s\n", result.Quality)
    fmt.Printf("Contributing links: %v\n", result.ContributingLinks)

    // Step 6: Compute GDOP for entire grid
    results := gc.ComputeAll()
    
    // Step 7: Calculate coverage statistics
    coverageScore := gc.CoverageScore(results)
    avgGDOP := gc.AverageGDOP(results)
    qualityCounts := gc.QualityCounts(results)

    fmt.Printf("\nGrid Analysis:\n")
    fmt.Printf("Coverage Score: %.1f%%\n", coverageScore)
    fmt.Printf("Average GDOP: %.2f\n", avgGDOP)
    fmt.Printf("Quality Distribution: %v\n", qualityCounts)
}
```

## Key Constraints and Requirements

1. **Minimum Requirements:**
   - At least 2 nodes with valid positions required
   - At least 2 covering links required for GDOP calculation
   - All coordinates must be finite (no NaN/Inf)

2. **Fresnel Zone Filtering:**
   - Links must be within maxZone Fresnel zones (default: zone 3)
   - Zone 1: ΔL < λ/2, Zone 2: ΔL < λ, Zone 3: ΔL < 3λ/2
   - Links outside zones are excluded from GDOP calculation

3. **Geometric Constraints:**
   - Collinear links result in Infinity GDOP (det(F) ≤ 1e-6)
   - Poor angular diversity increases GDOP
   - Mixed heights improve Z-axis accuracy

4. **Performance Considerations:**
   - ComputeAt (single point): O(n) where n = number of links
   - ComputeAll (full grid): O(cols × rows × n)
   - 50×50 grid with 12 links: ~30,000 operations (<2ms)
   - Maximum grid size: 100×100 cells (memory limit)

This documentation provides complete reference information for all GDOP computation functions in the Spaxel system.