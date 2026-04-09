// Package simulator provides pre-deployment simulation capabilities for Spaxel.
//
// This package allows users to:
// - Define their space in a 3D editor (or via API)
// - Place virtual nodes at candidate positions
// - Generate synthetic "walkers" that move through the space
// - Compute expected CSI using propagation models
// - Apply the same Fresnel zone localization algorithm as live mode
// - View GDOP overlay, accuracy estimates, and minimum node recommendations
package simulator

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"sync"
	"time"
)

// Engine is the pre-deployment simulator engine.
type Engine struct {
	mu               sync.RWMutex
	space            *Space
	nodes            *NodeSet
	walkers          []*SimWalker
	grid             *Grid
	links            []Link
	publishedResults *SimulationResult
	subscribers      []chan *SimulationResult
	propagation      *PropagationModel
	accuracy         *AccuracyEstimator
	recommendations  *RecommendationEngine
}

// SimWalker represents a simulated person moving through the space.
type SimWalker struct {
	ID          string     `json:"id"`
	Type        WalkerType `json:"type"`
	Position    Point      `json:"position"`
	Velocity    Point      `json:"velocity"`
	Path        []Point    `json:"path,omitempty"`        // for path walks
	PathIndex   int        `json:"path_index,omitempty"`   // current position in path
	TargetZones []string   `json:"target_zones,omitempty"` // for zone walks
	TrueHistory []Point    `json:"true_history,omitempty"` // ground truth positions
}

// Grid is the 3D spatial grid for Fresnel accumulation.
type Grid struct {
	CellSize    float64  `json:"cell_size"`    // meters
	OriginX     float64  `json:"origin_x"`     // meters
	OriginY     float64  `json:"origin_y"`     // meters
	OriginZ     float64  `json:"origin_z"`     // meters
	WidthCells  int      `json:"width_cells"`  // number of cells in X
	DepthCells  int      `json:"depth_cells"`  // number of cells in Y
	HeightCells int      `json:"height_cells"` // number of cells in Z
	Data        []float64 `json:"data"`         // flattened 3D array [z][x][y]
}

// ZoneInfo contains Fresnel zone information for a grid cell.
type ZoneInfo struct {
	CellIndex int     `json:"cell_index"` // flattened index
	Zone      int     `json:"zone"`       // Fresnel zone number
	Decay     float64 `json:"decay"`      // zone decay factor
}

// SimulationResult contains the results of a simulation run.
type SimulationResult struct {
	Timestamp         int64         `json:"timestamp_ms"`
	Blobs             []BlobResult  `json:"blobs"`
	CoverageScore     float64       `json:"coverage_score"`      // 0-100
	GDOPMap           []float64     `json:"gdop_map"`           // flattened grid
	GridDimensions    []int         `json:"grid_dimensions"`    // [width_cells, depth_cells, height_cells]
	Recommendations   []Recommendation `json:"recommendations"`
	Accuracy          AccuracyReport `json:"accuracy"`
	ShoppingList      ShoppingList  `json:"shopping_list"`
}

// BlobResult is a simulated detection result.
type BlobResult struct {
	ID         int     `json:"id"`
	Position   Point   `json:"position"`
	Confidence float64 `json:"confidence"`
	Velocity   Point   `json:"velocity"`
	WalkerID   string  `json:"walker_id"`
	TrueError  float64 `json:"true_error_m,omitempty"` // distance from true position
}

// NewEngine creates a new simulator engine.
func NewEngine(space *Space) *Engine {
	return &Engine{
		space:       space,
		nodes:       NewNodeSet(),
		walkers:     make([]*SimWalker, 0),
		subscribers: make([]chan *SimulationResult, 0),
		propagation: NewPropagationModel(space),
		accuracy:    NewAccuracyEstimator(),
		recommendations: NewRecommendationEngine(),
	}
}

// SetSpace updates the space definition.
func (e *Engine) SetSpace(space *Space) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.space = space
	e.propagation = NewPropagationModel(space)
	e.grid = nil // Invalidate grid
}

// AddVirtualNode adds a virtual node at the specified position.
func (e *Engine) AddVirtualNode(node *Node) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Validate position is within space
	minX, minY, minZ, maxX, maxY, maxZ := e.space.Bounds()
	if node.Position.X < minX || node.Position.X > maxX {
		return ErrNodeOutsideSpace
	}
	if node.Position.Y < minY || node.Position.Y > maxY {
		return ErrNodeOutsideSpace
	}
	if node.Position.Z < minZ || node.Position.Z > maxZ {
		return ErrNodeOutsideSpace
	}

	e.nodes.Add(node)
	e.links = nil // Invalidate links
	e.grid = nil  // Invalidate grid

	log.Printf("[SIM] Added virtual node %s at (%.2f, %.2f, %.2f)", node.ID, node.Position.X, node.Position.Y, node.Position.Z)

	return nil
}

// RemoveVirtualNode removes a virtual node by ID.
func (e *Engine) RemoveVirtualNode(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.nodes.Remove(id) {
		e.links = nil
		e.grid = nil
		log.Printf("[SIM] Removed virtual node %s", id)
	}
}

// AddWalker adds a simulated walker.
func (e *Engine) AddWalker(walker *SimWalker) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.walkers = append(e.walkers, walker)
	walker.TrueHistory = make([]Point, 0)
	log.Printf("[SIM] Added walker %s", walker.ID)
}

// RemoveWalker removes a walker by ID.
func (e *Engine) RemoveWalker(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for i, w := range e.walkers {
		if w.ID == id {
			e.walkers = append(e.walkers[:i], e.walkers[i+1:]...)
			log.Printf("[SIM] Removed walker %s", id)
			return
		}
	}
}

// GetVirtualNodes returns all virtual nodes.
func (e *Engine) GetVirtualNodes() []*Node {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.nodes.All()
}

// GetWalkers returns all walkers.
func (e *Engine) GetWalkers() []*SimWalker {
	e.mu.RLock()
	defer e.mu.RUnlock()

	walkers := make([]*SimWalker, len(e.walkers))
	copy(walkers, e.walkers)
	return walkers
}

// RunSimulation runs one simulation tick and publishes results.
func (e *Engine) RunSimulation() *SimulationResult {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Update grid if needed
	if e.grid == nil {
		e.initializeGrid()
	}

	// Update walker positions
	e.updateWalkers()

	// Generate virtual links between all node pairs
	if e.links == nil {
		e.generateLinks()
	}

	// Compute CSI at each walker position
	blobResults := e.detectBlobs()

	// Compute GDOP map
	gdopMap := e.computeGDOPMap()

	// Compute coverage score
	coverageScore := e.computeCoverageScore(gdopMap)

	// Generate recommendations
	recommendations := e.recommendations.Generate(e.space, e.nodes, gdopMap, coverageScore)

	// Compute accuracy
	accuracy := e.accuracy.Compute(e.walkers, blobResults)

	// Generate shopping list
	shoppingList := GenerateShoppingListFromResults(e.space, e.nodes, coverageScore, accuracy)

	result := &SimulationResult{
		Timestamp:       time.Now().UnixMilli(),
		Blobs:           blobResults,
		CoverageScore:   coverageScore,
		GDOPMap:         gdopMap,
		GridDimensions:  []int{e.grid.WidthCells, e.grid.DepthCells, e.grid.HeightCells},
		Recommendations: recommendations,
		Accuracy:        accuracy,
		ShoppingList:    shoppingList,
	}

	e.publishedResults = result

	// Notify subscribers
	for _, ch := range e.subscribers {
		select {
		case ch <- result:
		default:
			// Channel full, skip
		}
	}

	return result
}

// Subscribe creates a channel for simulation result updates.
func (e *Engine) Subscribe() <-chan *SimulationResult {
	e.mu.Lock()
	defer e.mu.Unlock()

	ch := make(chan *SimulationResult, 1)
	e.subscribers = append(e.subscribers, ch)
	return ch
}

// Unsubscribe removes a subscription channel.
func (e *Engine) Unsubscribe(ch <-chan *SimulationResult) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for i, sub := range e.subscribers {
		if sub == ch {
			e.subscribers = append(e.subscribers[:i], e.subscribers[i+1:]...)
			close(sub)
			return
		}
	}
}

// initializeGrid creates the spatial grid.
func (e *Engine) initializeGrid() {
	const cellSize = 0.2 // 20cm cells

	minX, minY, minZ, maxX, maxY, maxZ := e.space.Bounds()

	widthCells := int(math.Ceil((maxX - minX) / cellSize))
	depthCells := int(math.Ceil((maxY - minY) / cellSize))
	heightCells := int(math.Ceil((maxZ - minZ) / cellSize))

	e.grid = &Grid{
		CellSize:    cellSize,
		OriginX:     minX,
		OriginY:     minY,
		OriginZ:     minZ,
		WidthCells:  widthCells,
		DepthCells:  depthCells,
		HeightCells: heightCells,
		Data:        make([]float64, widthCells*depthCells*heightCells),
	}

	log.Printf("[SIM] Grid initialized: %dx%dx%d cells", widthCells, depthCells, heightCells)
}

// generateLinks creates virtual links between all node pairs.
func (e *Engine) generateLinks() {
	e.links = GenerateAllLinks(e.nodes)
	log.Printf("[SIM] Generated %d links", len(e.links))
}

// updateWalkers updates all walker positions.
func (e *Engine) updateWalkers() {
	const dt = 0.1 // 100ms time step

	minX, minY, minZ, maxX, maxY, maxZ := e.space.Bounds()

	for _, walker := range e.walkers {
		// Record true position
		walker.TrueHistory = append(walker.TrueHistory, walker.Position)

		if walker.Type == WalkerTypePath && len(walker.Path) > 0 {
			// Follow path
			target := walker.Path[walker.PathIndex]
			dx := target.X - walker.Position.X
			dy := target.Y - walker.Position.Y
			dz := target.Z - walker.Position.Z
			dist := math.Sqrt(dx*dx + dy*dy + dz*dz)

			stepSize := 0.5 / float64(10) // 0.5 m/s at 10 Hz

			if dist <= stepSize {
				// Reached waypoint, move to next
				walker.Position = target
				walker.PathIndex = (walker.PathIndex + 1) % len(walker.Path)
			} else {
				// Move toward target
				walker.Position.X += (dx / dist) * stepSize
				walker.Position.Y += (dy / dist) * stepSize
				walker.Position.Z += (dz / dist) * stepSize
			}
		} else {
			// Random walk
			walker.Position.X += walker.Velocity.X * dt
			walker.Position.Y += walker.Velocity.Y * dt
			walker.Position.Z += walker.Velocity.Z * dt

			// Bounce off walls
			if walker.Position.X < minX {
				walker.Position.X = minX
				walker.Velocity.X *= -1
			}
			if walker.Position.X > maxX {
				walker.Position.X = maxX
				walker.Velocity.X *= -1
			}
			if walker.Position.Y < minY {
				walker.Position.Y = minY
				walker.Velocity.Y *= -1
			}
			if walker.Position.Y > maxY {
				walker.Position.Y = maxY
				walker.Velocity.Y *= -1
			}
			if walker.Position.Z < minZ {
				walker.Position.Z = minZ
				walker.Velocity.Z *= -1
			}
			if walker.Position.Z > maxZ {
				walker.Position.Z = maxZ
				walker.Velocity.Z *= -1
			}

			// Random velocity perturbation
			walker.Velocity.X += (rand.Float64() - 0.5) * 0.1
			walker.Velocity.Y += (rand.Float64() - 0.5) * 0.1
			walker.Velocity.Z += (rand.Float64() - 0.5) * 0.05

			// Clamp velocity
			maxSpeed := 0.5
			speed := math.Sqrt(walker.Velocity.X*walker.Velocity.X + walker.Velocity.Y*walker.Velocity.Y)
			if speed > maxSpeed {
				scale := maxSpeed / speed
				walker.Velocity.X *= scale
				walker.Velocity.Y *= scale
			}
		}
	}
}

// detectBlobs runs the Fresnel zone accumulation to detect walker positions.
func (e *Engine) detectBlobs() []BlobResult {
	// Clear grid
	for i := range e.grid.Data {
		e.grid.Data[i] = 0
	}

	// Accumulate for each link and walker
	for _, link := range e.links {
		for _, walker := range e.walkers {
			// Compute CSI amplitude at walker position
			amplitude := e.propagation.AmplitudeAt(link.TX.Position, link.RX.Position, walker.Position)

			// Add to grid cells covered by this link
			for x := 0; x < e.grid.WidthCells; x++ {
				for y := 0; y < e.grid.DepthCells; y++ {
					for z := 0; z < e.grid.HeightCells; z++ {
						// Cell center position
						cx := e.grid.OriginX + float64(x)*e.grid.CellSize + e.grid.CellSize/2
						cy := e.grid.OriginY + float64(y)*e.grid.CellSize + e.grid.CellSize/2
						cz := e.grid.OriginZ + float64(z)*e.grid.CellSize + e.grid.CellSize/2
						cellPos := Point{X: cx, Y: cy, Z: cz}

						// Check if in Fresnel zone
						zone := FresnelZoneNumber(link.TX.Position, link.RX.Position, cellPos)
						if zone > 5 {
							continue
						}

						// Zone decay (default decay_rate = 2.0)
						decay := 1.0 / math.Pow(float64(zone), 2.0)

						cellIndex := z*e.grid.WidthCells*e.grid.DepthCells + x*e.grid.DepthCells + y
						contribution := amplitude * decay
						e.grid.Data[cellIndex] += contribution
					}
				}
			}
		}
	}

	// Extract peaks (blobs)
	blobs := make([]BlobResult, 0)
	blobID := 1

	for z := 0; z < e.grid.HeightCells; z++ {
		for x := 0; x < e.grid.WidthCells; x++ {
			for y := 0; y < e.grid.DepthCells; y++ {
				cellIndex := z*e.grid.WidthCells*e.grid.DepthCells + x*e.grid.DepthCells + y
				value := e.grid.Data[cellIndex]

				if value < 0.1 {
					continue // Below threshold
				}

				// Check if this is a local maximum
				if e.isLocalMaximum(x, y, z, value) {
					// Compute position from cell index
					posX := e.grid.OriginX + float64(x)*e.grid.CellSize + e.grid.CellSize/2
					posY := e.grid.OriginY + float64(y)*e.grid.CellSize + e.grid.CellSize/2
					posZ := e.grid.OriginZ + float64(z)*e.grid.CellSize + e.grid.CellSize/2
					blobPos := Point{X: posX, Y: posY, Z: posZ}

					// Find nearest walker and compute true error
					nearestWalker := ""
					minDist := 9999.0
					for _, walker := range e.walkers {
						dist := blobPos.Distance(walker.Position)
						if dist < minDist {
							minDist = dist
							nearestWalker = walker.ID
						}
					}

					blobs = append(blobs, BlobResult{
						ID:         blobID,
						Position:   blobPos,
						Confidence: math.Min(1.0, value/5.0), // Normalize confidence
						WalkerID:   nearestWalker,
						TrueError:  minDist,
					})
					blobID++
				}
			}
		}
	}

	return blobs
}

// isLocalMaximum checks if a cell is a local maximum in its 6-neighborhood.
func (e *Engine) isLocalMaximum(x, y, z int, value float64) bool {
	// Check 6-connected neighbors
	neighbors := [][3]int{
		{x - 1, y, z}, {x + 1, y, z},
		{x, y - 1, z}, {x, y + 1, z},
		{x, y, z - 1}, {x, y, z + 1},
	}

	for _, n := range neighbors {
		if n[0] < 0 || n[0] >= e.grid.WidthCells ||
			n[1] < 0 || n[1] >= e.grid.DepthCells ||
			n[2] < 0 || n[2] >= e.grid.HeightCells {
			continue
		}

		idx := n[2]*e.grid.WidthCells*e.grid.DepthCells + n[0]*e.grid.DepthCells + n[1]
		if e.grid.Data[idx] > value {
			return false
		}
	}

	return true
}

// computeGDOPMap computes GDOP values for each grid cell.
func (e *Engine) computeGDOPMap() []float64 {
	gdopMap := make([]float64, len(e.grid.Data))

	if len(e.links) < 2 {
		// Not enough links, set all to infinity
		for i := range gdopMap {
			gdopMap[i] = 9999.0
		}
		return gdopMap
	}

	for z := 0; z < e.grid.HeightCells; z++ {
		for x := 0; x < e.grid.WidthCells; x++ {
			for y := 0; y < e.grid.DepthCells; y++ {
				cellIndex := z*e.grid.WidthCells*e.grid.DepthCells + x*e.grid.DepthCells + y

				// Cell position
				cx := e.grid.OriginX + float64(x)*e.grid.CellSize + e.grid.CellSize/2
				cy := e.grid.OriginY + float64(y)*e.grid.CellSize + e.grid.CellSize/2
				cz := e.grid.OriginZ + float64(z)*e.grid.CellSize + e.grid.CellSize/2

				gdopMap[cellIndex] = e.computeGDOPAt(cx, cy, cz)
			}
		}
	}

	return gdopMap
}

// computeGDOPAt computes GDOP at a specific position.
func (e *Engine) computeGDOPAt(x, y, z float64) float64 {
	point := Point{X: x, Y: y, Z: z}

	// Collect links that cover this point (within zone 5)
	var angles []float64
	linkCount := 0

	for _, link := range e.links {
		// Check if this point is within zone 5
		d1 := point.Distance(link.TX.Position)
		d2 := point.Distance(link.RX.Position)
		totalDist := d1 + d2
		deltaL := totalDist - link.TX.Position.Distance(link.RX.Position)

		zoneNumber := int(math.Ceil(deltaL / HalfWavelength))

		if zoneNumber <= 5 {
			linkCount++
			// Compute angle to link direction
			angle := math.Atan2(link.RX.Position.Y-link.TX.Position.Y, link.RX.Position.X-link.TX.Position.X)
			angles = append(angles, angle)
		}
	}

	if linkCount < 2 {
		return 9999.0 // Infinity
	}

	// Build Fisher information matrix
	var sumCos2, sumSin2, sumSinCos float64
	for _, angle := range angles {
		sumCos2 += math.Cos(angle) * math.Cos(angle)
		sumSin2 += math.Sin(angle) * math.Sin(angle)
		sumSinCos += math.Sin(angle) * math.Cos(angle)
	}

	detF := sumCos2*sumSin2 - sumSinCos*sumSinCos
	if detF < 1e-6 {
		return 9999.0 // Collinear links
	}

	// GDOP = sqrt(trace(F^-1)) = sqrt((sumCos2 + sumSin2) / detF)
	traceInv := (sumCos2 + sumSin2) / detF
	gdop := math.Sqrt(traceInv)

	return gdop
}

// computeCoverageScore calculates the percentage of cells with "good" GDOP.
func (e *Engine) computeCoverageScore(gdopMap []float64) float64 {
	if len(gdopMap) == 0 {
		return 0
	}

	goodCells := 0
	for _, gdop := range gdopMap {
		if gdop < 4.0 { // Good or excellent GDOP
			goodCells++
		}
	}

	return 100.0 * float64(goodCells) / float64(len(gdopMap))
}

// GetResults returns the most recent simulation results from the engine.
func (e *Engine) GetResults() *SimulationResult {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.publishedResults
}

// Errors
var (
	ErrNodeOutsideSpace = fmt.Errorf("node position is outside the defined space")
)
