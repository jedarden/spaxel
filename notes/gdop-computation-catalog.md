# Simulator GDOP Computation Catalog

**Last updated:** 2026-08-27
**Scope:** Simulator-side GDOP entry points in `mothership/internal/simulator/`

## Overview

GDOP (Geometric Dilution of Precision) computation in the simulator is split across two main files:
- `gdop.go` - Core GDOP computation engine with analysis utilities
- `engine.go` - Simulation orchestration that calls GDOP methods

## Public GDOP Entry Points (gdop.go)

### Constructor

#### `NewGDOPComputer(links []Link, config GridConfig) *GDOPComputer`
- **Purpose:** Creates a new GDOP computer instance for coverage analysis
- **Parameters:** 
  - `links`: Slice of Link objects (TX→RX pairs)
  - `config`: Grid configuration (cell size, bounds, dimensions)
- **Returns:** Initialized GDOPComputer pointer
- **Default behavior:** Sets maxZone to 3 (considers first 3 Fresnel zones), defaults CellSize to 0.2m if ≤ 0
- **Usage:** Primary entry point for creating a GDOP computation engine

### Configuration

#### `SetMaxZone(zone int)`
- **Purpose:** Sets maximum Fresnel zone to consider (default: 3)
- **Parameter:** `zone` - Maximum zone number (minimum: 1 if < 1)
- **Usage:** Configures how many Fresnel zones are included in GDOP calculation

### Primary Computation Methods

#### `ComputeAll() [][]GDOPResult`
- **Purpose:** Computes GDOP for entire grid defined by GridConfig
- **Returns:** 2D slice of GDOPResult indexed by [row][column]
  - Outer slice: rows (Y dimension, depth)
  - Inner slice: columns (X dimension, width)
- **Grid dimensions:**
  - `nx = ceil(Width / CellSize)` columns
  - `ny = ceil(Depth / CellSize)` rows
- **Algorithm:** Iterates all grid cells, calls `ComputeAt()` for each
- **Output per cell:** GDOP value, quality rating, contributing link IDs
- **Usage:** Full-space coverage analysis, heatmap generation, optimization

#### `ComputeAt(x, y, z float64) GDOPResult`
- **Purpose:** Computes GDOP at a specific 3D point
- **Parameters:**
  - `x, y, z`: Coordinates in meters (must be finite)
- **Returns:** Single GDOPResult struct with:
  - Position (X, Y, Z echoed back)
  - GDOP value (Infinity if no coverage)
  - Quality string ("excellent"/"good"/"fair"/"poor"/"none")
  - ContributingLinks (IDs of covering links)
- **Quality thresholds:**
  - "excellent": GDOP < 2.0
  - "good": 2.0 ≤ GDOP < 4.0
  - "fair": 4.0 ≤ GDOP < 8.0
  - "poor": 8.0 ≤ GDOP < Infinity
  - "none": GDOP = Infinity
- **Algorithm:**
  1. Collect links where point is within maxZone Fresnel zones
  2. If < 2 links: return Infinity GDOP
  3. Call `computeGDOPAngular()` for angular diversity calculation
  4. Map GDOP to quality string
- **Usage:** Real-time coverage queries, point-by-point analysis

### Internal Computation Methods

#### `computeGDOPAngular(point Point, links []Link) float64`
- **Purpose:** Computes GDOP based on angular diversity of link directions (2D Fisher Information Matrix method)
- **Parameters:**
  - `point`: 3D position (X, Y used for 2D analysis, Z projected)
  - `links`: Pre-filtered links covering the point
- **Returns:** GDOP value (< 2 excellent, 2-4 good, 4-8 fair, >8 poor, Infinity degenerate)
- **Algorithm:**
  1. Compute link angle θ = atan2(RY-TY, RX-TX) in floor plane
  2. Build Fisher matrix F = Σ [[cos²θ, cosθ·sinθ], [cosθ·sinθ, sin²θ]]
  3. Compute det(F) = f00*f11 - f01²
  4. If det(F) ≤ 1e-6: return Infinity (collinear links)
  5. Compute trace(F^-1) = (f00 + f11) / det(F)
  6. GDOP = sqrt(trace(F^-1))
- **Usage:** Called by `ComputeAt()` for actual GDOP calculation

### Analysis Methods

#### `CoverageScore(results [][]GDOPResult) float64`
- **Purpose:** Computes percentage of cells with "good" or better coverage
- **Parameter:** `results` - 2D array from `ComputeAll()`
- **Returns:** Percentage 0-100
- **Algorithm:** Counts cells with quality "excellent" or "good", divides by total cells
- **Usage:** Coverage quality metric for optimization

#### `AverageGDOP(results [][]GDOPResult) float64`
- **Purpose:** Computes average GDOP over all cells (excluding infinity)
- **Parameter:** `results` - 2D array from `ComputeAll()`
- **Returns:** Average GDOP value (Infinity if no valid cells)
- **Algorithm:** Sums finite GDOP values, divides by count of non-infinity cells
- **Usage:** Overall coverage quality metric

#### `QualityCounts(results [][]GDOPResult) map[string]int`
- **Purpose:** Returns count of cells by quality level
- **Parameter:** `results` - 2D array from `ComputeAll()`
- **Returns:** Map with keys "excellent", "good", "fair", "poor", "none"
- **Usage:** Coverage statistics, debugging

#### `FindDeadZones(results [][]GDOPResult) []Point`
- **Purpose:** Returns positions where coverage is "none" or "poor"
- **Parameter:** `results` - 2D array from `ComputeAll()`
- **Returns:** Slice of Point positions
- **Usage:** Identify areas needing additional nodes

#### `RecommendNodePosition(results [][]GDOPResult, space *Space) Point`
- **Purpose:** Suggests optimal positions for additional nodes based on dead zones
- **Parameters:**
  - `results` - 2D array from `ComputeAll()`
  - `space` - Space definition
- **Returns:** Recommended Point position (Z=2.0 for high placement)
- **Algorithm:** Computes centroid of dead zones, or space center if none
- **Usage:** Node placement recommendations

#### `GetWorstCoverageCells(results [][]GDOPResult, n int) []GDOPResult`
- **Purpose:** Returns N cells with worst GDOP values
- **Parameters:**
  - `results` - 2D array from `ComputeAll()`
  - `n` - Number of cells to return
- **Returns:** Slice of worst GDOPResult cells (sorted descending by GDOP)
- **Algorithm:** Flattens grid, sorts by GDOP descending (infinity first)
- **Usage:** Identify problem areas for debugging

#### `GetBestCoverageCells(results [][]GDOPResult, n int) []GDOPResult`
- **Purpose:** Returns N cells with best GDOP values
- **Parameters:**
  - `results` - 2D array from `ComputeAll()`
  - `n` - Number of cells to return
- **Returns:** Slice of best GDOPResult cells (sorted ascending by GDOP)
- **Algorithm:** Flattens grid, sorts by GDOP ascending (finite first)
- **Usage:** Identify best coverage areas

### Visualization Methods

#### `GDOPColorMap(gdop float64) GDOPColor`
- **Purpose:** Returns RGB color for a given GDOP value for visualization
- **Parameter:** `gdop` - GDOP value
- **Returns:** GDOPColor struct with R, G, B (0-255)
- **Color mapping:**
  - Infinity → Gray (80, 80, 80)
  - < 2.0 → Green (34, 197, 94)
  - 2.0-4.0 → Yellow (255, 193, 7)
  - 4.0-8.0 → Orange (255, 146, 0)
  - > 8.0 → Red (220, 53, 69)
- **Usage:** Frontend heatmap rendering

#### `ToHeatmapData(results [][]GDOPResult) *GDOPHeatmapData`
- **Purpose:** Converts GDOP results to heatmap-friendly format for frontend
- **Parameter:** `results` - 2D array from `ComputeAll()`
- **Returns:** GDOPHeatmapData struct with flattened arrays:
  - GDOP values (9999 = infinity)
  - Quality strings
  - RGB colors
  - Expected accuracy map
- **Usage:** Prepare data for Three.js texture or canvas rendering

#### `ComputeAccuracyMap(results [][]GDOPResult) [][]float64`
- **Purpose:** Computes expected accuracy for each cell in meters
- **Parameter:** `results` - 2D array from `ComputeAll()`
- **Returns:** 2D array of accuracy values (infinity = no coverage)
- **Algorithm:** Calls `ExpectedAccuracy()` for each cell
- **Usage:** Accuracy visualization

#### `ComputeColorMap(results [][]GDOPResult) [][]uint8`
- **Purpose:** Computes RGB colors for each cell for visualization
- **Parameter:** `results` - 2D array from `ComputeAll()`
- **Returns:** Flattened RGB array [width*depth*3]
- **Algorithm:** Calls `GDOPColorMap()` for each cell
- **Usage:** Direct color data for texture upload

### Package-Level Helper Functions

#### `MinimumNodeCount(space *Space, targetGDOP float64) int`
- **Purpose:** Estimates minimum number of nodes needed for good coverage
- **Parameters:**
  - `space` - Space definition
  - `targetGDOP` - Desired GDOP threshold
- **Returns:** Estimated node count
- **Algorithm:** Area-based heuristic
  - GDOP < 2: 1 node per 15 m²
  - GDOP < 4: 1 node per 20 m²
  - GDOP ≥ 4: 1 node per 30 m²
- **Usage:** Shopping list generation, deployment planning

#### `ExpectedAccuracy(gdop float64) float64`
- **Purpose:** Estimates expected localization accuracy based on GDOP
- **Parameter:** `gdop` - GDOP value
- **Returns:** Expected accuracy in meters (Infinity if GDOP infinite)
- **Algorithm:** `baseAccuracy * gdop` where baseAccuracy = 0.5m for GDOP=1
- **Usage:** Accuracy estimation, visualization

### Optimization Methods

#### `OptimizeNodePositions(space *Space, numNodes int, iterations int) *NodeSet`
- **Purpose:** Uses greedy algorithm to find better node positions
- **Parameters:**
  - `space` - Space definition
  - `numNodes` - Number of nodes to optimize
  - `iterations` - Number of optimization iterations
- **Returns:** Optimized NodeSet
- **Algorithm:**
  1. Start with corner positions
  2. Iteratively try moving each node slightly
  3. Keep moves that improve coverage score
  4. Return best configuration found
- **Usage:** Automatic node placement optimization

### Package-Level Functions (Not Methods)

#### `GenerateShoppingList(space *Space, currentNodes *NodeSet) *ShoppingList`
- **Purpose:** Creates shopping list from simulation results
- **Parameters:**
  - `space` - Space definition
  - `currentNodes` - Current node set (nil → suggests 4 nodes)
- **Returns:** ShoppingList struct with recommendations
- **Algorithm:** Computes GDOP coverage, estimates minimum nodes, expected accuracy
- **Usage:** Deployment planning, user recommendations

#### `computeGDOPImprovement(currentLayout []*Node, nodeMAC string, targetPos Point) float64`
- **Purpose:** Computes GDOP improvement for hypothetical node repositioning
- **Parameters:**
  - `currentLayout` - All nodes in current positions
  - `nodeMAC` - MAC/ID of node to move
  - `targetPos` - Target position to move node to
- **Returns:** Relative improvement in range [-1.0, 1.0]
  - Positive: improvement (lower GDOP)
  - Negative: degradation (higher GDOP)
  - 0.0: no change or node not found
- **Algorithm:**
  1. Compute worst-case GDOP for current layout
  2. Create hypothetical layout with node moved
  3. Compute worst-case GDOP for hypothetical layout
  4. Calculate relative improvement
- **Usage:** Coverage painting during node drag, real-time feedback

#### `computeWorstGDOP(nodes []*Node) float64`
- **Purpose:** Calculates worst-case GDOP across all grid cells for a node layout
- **Parameter:** `nodes` - Slice of nodes to evaluate
- **Returns:** Worst GDOP value (maximum across all cells)
- **Algorithm:**
  1. Generate all links from node set
  2. Create grid covering node positions ± 1m margin
  3. Compute GDOP for each cell
  4. Return maximum GDOP found
- **Usage:** Layout quality assessment, optimization objective function

---

## Engine-Level Entry Points (engine.go)

### Integration Methods

#### `computeGDOPMap() []float64`
- **Purpose:** Computes GDOP map for entire grid (called by RunSimulation)
- **Returns:** Flattened array of GDOP values (9999.0 = infinity)
- **Algorithm:**
  1. Check if < 2 links → return all infinity
  2. For each grid cell, compute cell center position
  3. Call `computeGDOPAt()` for each cell
  4. Store in flattened array
- **Usage:** Full simulation run, coverage visualization
- **Called by:** `RunSimulation()`

#### `computeGDOPAt(x, y, z float64) float64`
- **Purpose:** Computes GDOP at specific position (called by computeGDOPMap)
- **Parameters:** `x, y, z` - Position coordinates in meters
- **Returns:** GDOP value (9999.0 for infinity)
- **Algorithm:**
  1. Collect links where point is within zone 5 of TX→RX path
  2. If < 2 links → return infinity
  3. Build Fisher information matrix from link angles
  4. Compute determinant and trace
  5. Return sqrt(trace(F^-1))
- **Differences from gdop.go version:**
  - Uses fixed zone 5 (vs. configurable maxZone)
  - Returns 9999.0 instead of math.Inf(1)
  - Inline Fisher matrix calculation (no helper method)
- **Usage:** Per-cell GDOP computation during simulation
- **Called by:** `computeGDOPMap()`

#### `computeCoverageScore(gdopMap []float64) float64`
- **Purpose:** Calculates percentage of cells with "good" GDOP (< 4.0)
- **Parameter:** `gdopMap` - Flattened GDOP array from computeGDOPMap()
- **Returns:** Coverage score 0-100
- **Algorithm:** Counts cells with GDOP < 4.0, divides by total cells
- **Usage:** Simulation result aggregation
- **Called by:** `RunSimulation()`

---

## Call Relationships

### Primary Computation Paths

**Path 1: Full Grid Computation (Coverage Analysis)**
```
NewGDOPComputer(links, config)
  └─ ComputeAll()
       ├─ For each cell: ComputeAt(x, y, z)
       │    └─ computeGDOPAngular(point, coveringLinks)
       └─ Returns: [][]GDOPResult
            └─ Used by: CoverageScore(), AverageGDOP(), ToHeatmapData(), etc.
```

**Path 2: Single-Point Query (Real-time Analysis)**
```
NewGDOPComputer(links, config)
  └─ ComputeAt(x, y, z)
       ├─ Collect covering links (within maxZone)
       └─ computeGDOPAngular(point, links)
            └─ Returns: GDOPResult
```

**Path 3: Simulation Orchestration (Engine)**
```
Engine.RunSimulation()
  ├─ computeGDOPMap()
  │    └─ For each cell: computeGDOPAt(x, y, z)
  │             └─ Inline Fisher matrix computation
  ├─ computeCoverageScore(gdopMap)
  └─ Returns: SimulationResult (includes GDOP map, coverage score)
```

**Path 4: Optimization Loop (Node Placement)**
```
OptimizeNodePositions()
  ├─ Generate initial links
  ├─ Create GDOPComputer
  ├─ ComputeAll() → evaluate coverage
  └─ Iteratively improve positions using GDOP as objective
```

**Path 5: Coverage Painting (Real-time Drag Feedback)**
```
computeGDOPImprovement(currentLayout, nodeMAC, targetPos)
  ├─ computeWorstGDOP(currentLayout)
  ├─ Create hypothetical layout with moved node
  ├─ computeWorstGDOP(hypotheticalLayout)
  └─ Calculate relative improvement
```

### Caller Summary

| Function | Called By | Context |
|----------|-----------|---------|
| `NewGDOPComputer` | `OptimizeNodePositions`, `GenerateShoppingList`, `computeWorstGDOP` | Create GDOP engine |
| `ComputeAll` | `OptimizeNodePositions`, `GenerateShoppingList`, `computeWorstGDOP` | Full grid evaluation |
| `ComputeAt` | `ComputeAll` (per cell), direct API calls | Point queries |
| `computeGDOPAngular` | `ComputeAt` | Core GDOP calculation |
| `computeGDOPMap` | `RunSimulation` | Engine-level full grid |
| `computeGDOPAt` | `computeGDOPMap` (per cell) | Engine-level point query |
| `computeCoverageScore` | `RunSimulation` | Engine-level coverage metric |
| `computeWorstGDOP` | `computeGDOPImprovement`, `OptimizeNodePositions` | Layout assessment |
| `computeGDOPImprovement` | External (coverage painting) | Real-time drag feedback |

---

## Parameter Differences

### Zone Handling
- **gdop.go:** Configurable via `SetMaxZone()` (default: 3 zones)
- **engine.go:** Hardcoded to zone 5 in `computeGDOPAt()`

### Infinity Encoding
- **gdop.go:** Uses `math.Inf(1)` for no coverage
- **engine.go:** Uses `9999.0` for infinity (in flattened arrays)

### Fixed Z Height
- **gdop.go `ComputeAll()`:** Fixed at Z = 1.0m for 2D analysis
- **gdop.go `ComputeAt()`:** Takes Z as parameter, used for Fresnel zone calculation
- **engine.go:** Uses Z from grid (3D computation)

### Grid Size
- **gdop.go:** Configured via GridConfig (CellSize, Width, Depth)
- **engine.go:** Fixed at 0.2m cell size in `initializeGrid()`

---

## Primary vs Alternate Computation Paths

### Primary Paths
1. **Coverage Analysis:** `NewGDOPComputer → ComputeAll → helper analysis methods`
2. **Simulation:** `RunSimulation → computeGDOPMap → computeGDOPAt → computeCoverageScore`
3. **Real-time Point Query:** `NewGDOPComputer → ComputeAt → computeGDOPAngular`

### Alternate/Utility Paths
1. **Node Placement Optimization:** `OptimizeNodePositions → ComputeAll (in loop)`
2. **Coverage Painting:** `computeWorstGDOP → computeGDOPImprovement` (evaluates hypothetical layouts)
3. **Visualization:** `ToHeatmapData`, `ComputeAccuracyMap`, `ComputeColorMap` (post-processing)
4. **Shopping List Generation:** `GenerateShoppingList → NewGDOPComputer → ComputeAll`

### Key Difference
- **gdop.go** methods are the primary API for external coverage analysis
- **engine.go** methods are internal to simulation orchestration and have slightly different parameters (zone 5 vs. 3, 9999 vs. infinity)
- The two implementations have overlapping but not identical GDOP calculation logic
