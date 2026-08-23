# GDOP Search Results - Spaxel Codebase

**Search Date:** 2026-08-23  
**Searched Terms:** 'gdop', 'geometric.*dilution', 'dilution.*precision', 'HDOP', 'VDOP', 'PDOP', 'TDOP'  
**File Types:** .rs, .c, .cpp, .h, .go, .js

---

## Summary

The codebase contains extensive GDOP (Geometric Dilution of Precision) implementation across multiple files. GDOP is a core metric for coverage quality in the Spaxel indoor positioning system.

## Key Files with GDOP Implementation

### 1. **GDOP Computation Core**
- **`mothership/internal/simulator/gdop.go`**
  - Primary GDOP computation implementation
  - `GDOPComputer` struct and methods
  - Computes GDOP metrics for coverage analysis
  - Lines with "GDOP": 80+ occurrences

- **`mothership/internal/localization/gdop_example.go`**
  - GDOP computation examples and documentation
  - Reference implementation for GDOP usage

- **`mothership/internal/localization/fusion.go`**
  - `GDOPMap()` - computes GDOP map for node positions
  - `computeGDOP()` - 2D GDOP calculation for a point
  - Integrated into the fusion/localization pipeline

### 2. **Dashboard Visualization**
- **`dashboard/js/placement.js`**
  - GDOP coverage overlay rendering
  - `computeHDOP()` - 2D horizontal DOP computation
  - `gdopToColor()` - color mapping for GDOP values
  - Real-time GDOP visualization during node placement

### 3. **Documentation & Examples**
- **`docs/gdop-usage-example.go`**
  - Example usage of GDOP computation functions
  - Three main examples:
    - `ExampleComputeGDOPAtPoint()` - single point computation
    - `ExampleComputeGDOPGrid()` - grid-wide computation
    - `ExampleComputeGDOPImprovement()` - placement optimization

- **`docs/gdop-usage-example-enhanced.go`**
  - Enhanced examples with detailed comments
  - Full name expansion: "Geometric Dilution of Precision in Spaxel"

- **`docs/examples/gdop_usage_examples.go`**
  - Comprehensive usage examples package
  - GDOP computation system documentation

### 4. **Diagnostics & Fleet Management**
- **`mothership/internal/diagnostics/reposition.go`**
  - GDOP calculator for node repositioning recommendations
  - Helps optimize node placement

- **`mothership/internal/fleet/healer.go`**
  - GDOP calculator for self-healing fleet management
  - Used when nodes go offline and roles need re-optimization

## GDOP Variants Found

### HDOP (Horizontal Dilution of Precision)
- **File:** `dashboard/js/placement.js`
- **Usage:** 2D horizontal DOP computation for ground plane positions
- **Function:** `computeHDOP(px, pz, nodes)`
- **Formula:** HDOP = √trace(G⁻¹) where G is the Fisher information matrix

### Other DOP Variants
- **VDOP, PDOP, TDOP:** NOT explicitly found in the codebase
- The system focuses on 2D horizontal DOP (HDOP) for floor-plan coverage
- Z-axis precision is handled through the 3D Fresnel zone system instead of VDOP

## GDOP Implementation Details

### Core Algorithm
The Fisher information matrix approach:
```
For each cell at position P:
1. Collect all links where P is within first 3 Fresnel zones
2. For each link, compute θ = atan2(RX.y - TX.y, RX.x - TX.x)
3. Build Fisher information matrix F = Σ[[cos²θ, cosθ·sinθ], [cosθ·sinθ, sin²θ]]
4. det_F = F[0][0]·F[1][1] - F[0][1]·F[1][0]
5. If det_F ≤ 1e-6: GDOP = Infinity (collinear links)
6. trace_Finv = (F[0][0] + F[1][1]) / det_F
7. GDOP = √(trace_Finv)
```

### GDOP Thresholds
- **Excellent:** GDOP < 2 (green)
- **Good:** GDOP 2–4 (yellow)
- **Poor:** GDOP > 4 (red)
- **No coverage:** GDOP = Infinity (gray)

### Key Functions

**`GDOPComputer` (mothership/internal/simulator/gdop.go)**
```go
type GDOPComputer struct {
    links         []Link
    gridConfig    GridConfig
    zoneCache     map[string][]CellWithZone
}

func (gc *GDOPComputer) ComputeGDOP(px, py, pz float64) (gdop, n_links int, active_links []int)
func (gc *GDOPComputer) ComputeGDOPGrid() []CellResult
func (gc *GDOPComputer) AverageGDOP(results []CellResult) float64
```

**`computeHDOP()` (dashboard/js/placement.js)**
```javascript
function computeHDOP(px, pz, nodes) {
    // Returns 2D horizontal DOP for a ground-plane point
    // Returns Infinity if insufficient link geometry
}
```

## Statistics

- **Total files with GDOP references:** 10+ files
- **Approximate GDOP occurrences:** 200+ across all files
- **Languages:** Go (backend), JavaScript (frontend)
- **No DOP variants found:** VDOP, PDOP, TDOP are not explicitly used

## Integration Points

1. **Placement UI:** Real-time GDOP overlay during node drag operations
2. **Coverage Painting:** GDOP heatmap visualization on floor plan
3. **Fleet Healer:** Automatic re-optimization using GDOP metrics
4. **Diagnostics:** Node repositioning recommendations based on GDOP

## Conclusion

GDOP is thoroughly implemented as a first-class metric in the Spaxel codebase:
- Core computation in Go backend
- Visualization in JavaScript frontend  
- Integrated into placement, healing, and diagnostics workflows
- Well-documented with multiple example files
- Focused on 2D horizontal DOP (HDOP) for floor-plan coverage

**No matches found for:** VDOP, PDOP, TDOP variants (the system uses 3D Fresnel zones instead for vertical precision).
