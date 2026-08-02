# GDOP Function Usage Guide

**Geometric Dilution of Precision (GDOP)** quantifies how well the geometric arrangement of WiFi nodes can localize a point in space. Lower GDOP values indicate better coverage quality for indoor positioning.

## Quick Start

```go
import "github.com/spaxel/spaxel/mothership/internal/simulator"

// Create nodes and links
nodes := []*simulator.Node{
    simulator.NewNode("node1", "Kitchen NW", simulator.NodeTypeReal,
        simulator.Point{X: 0.0, Y: 0.0, Z: 2.0}),
    simulator.NewNode("node2", "Kitchen NE", simulator.NodeTypeReal,
        simulator.Point{X: 10.0, Y: 0.0, Z: 2.0}),
}

links := []simulator.Link{
    {TX: nodes[0], RX: nodes[1]},
}

// Create GDOP computer
computer := simulator.NewGDOPComputer(links, simulator.GridConfig{
    MinX: 0.0, MinY: 0.0, Width: 10.0, Depth: 10.0, CellSize: 0.2,
})

// Compute GDOP at a point
result := computer.ComputeAt(5.0, 5.0, 1.0)
fmt.Printf("GDOP: %.2f, Quality: %s\n", result.GDOP, result.Quality)
```

## Quality Thresholds

| GDOP Value | Quality | Expected Accuracy | Use Case |
|------------|---------|-------------------|----------|
| < 2.0 | excellent | ±0.5m | Precise tracking |
| 2.0 - 4.0 | good | ±1.0m | General positioning |
| 4.0 - 8.0 | fair | ±2-4m | Room-level detection |
| ≥ 8.0 | poor | ≥±4m | Marginal coverage |
| Infinity | none | No coverage | Insufficient nodes |

## Core API

### `NewGDOPComputer(links, config) *GDOPComputer`

Creates a new GDOP computer for coverage analysis.

**Parameters:**
- `links`: Slice of Link objects (TX→RX pairs)
- `config`: Grid configuration (cell size, dimensions, origin)

**Example:**
```go
computer := simulator.NewGDOPComputer(links, simulator.GridConfig{
    MinX: 0.0, MinY: 0.0, Width: 10.0, Depth: 10.0, CellSize: 0.2,
})
```

### `ComputeAt(x, y, z) GDOPResult`

Computes GDOP at a specific 3D point.

**Returns:**
- `GDOP`: Computed value (Infinity if no coverage)
- `Quality`: String rating ("excellent", "good", "fair", "poor", "none")
- `ContributingLinks`: IDs of links covering this point

**Example:**
```go
result := computer.ComputeAt(5.0, 5.0, 1.0)
if result.Quality == "excellent" {
    fmt.Println("Great coverage for precise tracking")
}
```

### `ComputeAll() [][]GDOPResult`

Computes GDOP for entire grid (used for heatmaps).

**Returns:** 2D array indexed by [row][column]

**Example:**
```go
results := computer.ComputeAll()
centerCell := results[25][25] // Center of 50x50 grid
fmt.Printf("Center GDOP: %.2f\n", centerCell.GDOP)
```

### `SetMaxZone(zone)`

Configures maximum Fresnel zone to consider (default: 3).

**Parameters:**
- `zone`: Maximum Fresnel zone (1-5)

**Example:**
```go
computer.SetMaxZone(1) // Only consider closest links
```

## Common Use Cases

### 1. Real-Time Coverage Query

Check coverage quality at a specific location:

```go
result := computer.ComputeAt(x, y, z)
if result.GDOP < 4.0 {
    // Good coverage - proceed with localization
} else {
    // Poor coverage - may need additional nodes
}
```

### 2. Coverage Heatmap Generation

Generate full-space coverage map:

```go
results := computer.ComputeAll()
for _, row := range results {
    for _, cell := range row {
        // Use cell.GDOP for heatmap color
        // Use cell.Quality for classification
        heatmap[cellIndex] = cell.GDOP
    }
}
```

### 3. Node Placement Optimization

Evaluate positioning before deployment:

```go
// Test candidate position
testPos := simulator.Point{X: 5.0, Y: 5.0, Z: 2.0}
tempNodes := append(existingNodes, newNodeAt(testPos))

tempLinks := buildLinks(tempNodes)
tempComputer := simulator.NewGDOPComputer(tempLinks, config)
result := tempComputer.ComputeAt(5.0, 5.0, 1.0)

if result.GDOP < currentGDOP {
    fmt.Println("New position improves coverage")
}
```

## Performance Considerations

### Grid Cell Size Impact

| Cell Size | 10×10m Grid | Computation Time | Use Case |
|-----------|--------------|------------------|----------|
| 0.1m | 100×100 = 10K cells | Slow | High-resolution analysis |
| 0.2m | 50×50 = 2.5K cells | Medium | Default, balanced |
| 0.5m | 20×20 = 400 cells | Fast | Quick preview |
| 1.0m | 10×10 = 100 cells | Very Fast | Coarse overview |

**Recommendation:** Use 0.5m cells for real-time updates (e.g., node drag), 0.2m for final analysis.

### Fresnel Zone Limiting

```go
// Conservative (faster): Only zone 1
computer.SetMaxZone(1)

// Balanced (default): Zones 1-3
computer.SetMaxZone(3)

// Aggressive (slower): Zones 1-5
computer.SetMaxZone(5)
```

## Common Pitfalls

### ❌ Wrong: Nodes on floor

```go
// Z=0 means nodes are on the floor
badNode := simulator.NewNode("node", "Floor", simulator.NodeTypeReal,
    simulator.Point{X: 0.0, Y: 0.0, Z: 0.0})
```

### ✅ Right: Nodes at proper height

```go
// Z=2.5m means ceiling-mounted (typical deployment)
goodNode := simulator.NewNode("node", "Ceiling", simulator.NodeTypeReal,
    simulator.Point{X: 0.0, Y: 0.0, Z: 2.5})
```

### ❌ Wrong: Forgetting unit system

```go
// Spaxel ALWAYS uses meters
badConfig := simulator.GridConfig{
    MinX: 0.0, MinY: 0.0, Width: 30.0, Depth: 30.0, // Feet!
}
```

### ✅ Right: Convert to meters

```go
// 30 feet = 9.144 meters
goodConfig := simulator.GridConfig{
    MinX: 0.0, MinY: 0.0, Width: 9.144, Depth: 9.144, // Meters!
}
```

## Integration with Spaxel

GDOP computation is used throughout Spaxel:

1. **Dashboard 3D Editor**: Real-time coverage painting during node drag
2. **Fleet Manager**: Self-healing role reassignment after node failure
3. **Pre-deployment Simulator**: Virtual node placement optimization
4. **Diagnostics**: Coverage quality reports and repositioning suggestions

## Mathematical Background

GDOP uses the Fisher Information Matrix:

```
F = Σ [ [cos²(θ),       cos(θ)·sin(θ)],
       [cos(θ)·sin(θ), sin²(θ)       ] ]

GDOP = sqrt(trace(F⁻¹))
```

Where θ is the angle of each link direction projected on the floor plane.

**Interpretation:**
- **Low GDOP**: Well-distributed link angles (good geometric diversity)
- **High GDOP**: Collinear or clustered links (poor geometric diversity)
- **Infinity**: Insufficient links (<2) or degenerate geometry

## Best Practices

1. **Minimum 3 nodes** for meaningful 2D localization
2. **Spread nodes apart** to maximize angular diversity
3. **Mixed heights** (1.5m and 2.5m) for Z-axis resolution
4. **Evaluate at each floor level** in multi-story buildings
5. **Use 0.2m cells** for detailed analysis, 0.5m for real-time updates
6. **Validate placement virtually** before physical deployment

## Error Handling

```go
result := computer.ComputeAt(x, y, z)

if math.IsInf(result.GDOP, 1) {
    // Handle no coverage case
    if result.Quality == "none" {
        if len(result.ContributingLinks) < 2 {
            log.Println("Insufficient nodes (need ≥2)")
        } else {
            log.Println("Degenerate geometry (collinear links)")
        }
    }
}
```

## Further Reading

- **Comprehensive Examples**: See `gdop_usage_examples.go`
- **Algorithm Details**: `docs/plan/plan.md` Section "Live Coverage Painting & GDOP Overlay"
- **Implementation**: `mothership/internal/simulator/gdop.go`
- **Tests**: `mothership/internal/simulator/gdop_test.go`
