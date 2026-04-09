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
	mu                sync.RWMutex
	space             *SpaceDefinition
	virtualNodes      []*VirtualNode
	walkers           []*EngineWalker
	grid              *Grid
	links             []*EngineLink
	publishedResults  *SimulationResult
	subscribers       []chan *SimulationResult
}

// SpaceDefinition defines the monitored space.
type SpaceDefinition struct {
	Width  float64 `json:"width"`  // meters
	Depth  float64 `json:"depth"`  // meters
	Height float64 `json:"height"` // meters
	OriginX float64 `json:"origin_x"`
	OriginZ float64 `json:"origin_z"`
}

// VirtualNode represents a virtual (planned) node position.
type VirtualNode struct {
	ID       string  `json:"id"`
	Position [3]float64 `json:"position"` // x, y, z in meters
	Height   float64 `json:"height"`       // height in meters (same as position[2])
	Virtual  bool    `json:"virtual"`      // true = not yet purchased
}

// Walker represents a simulated person moving through the space.
type EngineWalker struct {
	ID          string      `json:"id"`
	Position    [3]float64 `json:"position"`    // x, y, z in meters
	Velocity    [3]float64 `json:"velocity"`    // vx, vy, vz in m/s
	PathType    string      `json:"path_type"`   // "random" or "path"
	PathPoints  [][3]float64 `json:"path_points,omitempty"` // for path-following
	CurrentPath int         `json:"current_path"` // index in path_points
}

// Grid is the 3D spatial grid for Fresnel accumulation.
type Grid struct {
	CellSize   float64 `json:"cell_size"`   // meters
	OriginX    float64 `json:"origin_x"`    // meters
	OriginZ    float64 `json:"origin_z"`    // meters
	WidthCells int     `json:"width_cells"` // number of cells in X
	DepthCells int     `json:"depth_cells"` // number of cells in Z
	HeightCells int    `json:"height_cells"` // number of cells in Y (Z axis)
	Data       []float64 `json:"data"`       // flattened 3D array [z][x][y]
}

// Link represents a virtual WiFi link between two nodes.
type EngineLink struct {
	ID         string     `json:"id"`
	TXNodeID   string     `json:"tx_node_id"`
	RXNodeID   string     `json:"rx_node_id"`
	TXPosition [3]float64 `json:"tx_position"`
	RXPosition [3]float64 `json:"rx_position"`
	Length     float64    `json:"length"`     // meters
	ZoneCache  []*ZoneInfo `json:"zone_cache"` // per-cell zone numbers
}

// ZoneInfo contains Fresnel zone information for a grid cell.
type ZoneInfo struct {
	CellIndex int     `json:"cell_index"` // flattened index
	Zone      int     `json:"zone"`       // Fresnel zone number
	Decay     float64 `json:"decay"`      // zone decay factor
}

// SimulationResult contains the results of a simulation run.
type SimulationResult struct {
	Timestamp      int64          `json:"timestamp_ms"`
	Blobs          []BlobResult  `json:"blobs"`
	CoverageScore  float64        `json:"coverage_score"`     // 0-100
	GDOPMap        []float64      `json:"gdop_map"`          // flattened grid
	GridDimensions []int          `json:"grid_dimensions"`   // [width_cells, depth_cells, height_cells]
	Recommendations []string     `json:"recommendations"`
}

// BlobResult is a simulated detection result.
type BlobResult struct {
	ID         int       `json:"id"`
	Position   [3]float64 `json:"position"`
	Confidence float64   `json:"confidence"`
	Velocity   [3]float64 `json:"velocity"`
	WalkerID   string    `json:"walker_id"`
}

// NewEngine creates a new simulator engine.
func NewEngine(space *SpaceDefinition) *Engine {
	return &Engine{
		space:       space,
		virtualNodes: make([]*VirtualNode, 0),
		walkers:      make([]*EngineWalker, 0),
		subscribers:  make([]chan *SimulationResult, 0),
	}
}

// SetSpace updates the space definition.
func (e *Engine) SetSpace(space *SpaceDefinition) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.space = space
	e.grid = nil // Invalidate grid
}

// AddVirtualNode adds a virtual node at the specified position.
func (e *Engine) AddVirtualNode(node *VirtualNode) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Validate position is within space
	if node.Position[0] < e.space.OriginX || node.Position[0] > e.space.OriginX+e.space.Width {
		return ErrNodeOutsideSpace
	}
	if node.Position[1] < e.space.OriginZ || node.Position[1] > e.space.OriginZ+e.space.Depth {
		return ErrNodeOutsideSpace
	}
	if node.Position[2] < 0 || node.Position[2] > e.space.Height {
		return ErrNodeOutsideSpace
	}

	e.virtualNodes = append(e.virtualNodes, node)
	e.links = nil // Invalidate links
	e.grid = nil  // Invalidate grid

	log.Printf("[SIM] Added virtual node %s at (%.2f, %.2f, %.2f)", node.ID, node.Position[0], node.Position[1], node.Position[2])

	return nil
}

// RemoveVirtualNode removes a virtual node by ID.
func (e *Engine) RemoveVirtualNode(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for i, node := range e.virtualNodes {
		if node.ID == id {
			e.virtualNodes = append(e.virtualNodes[:i], e.virtualNodes[i+1:]...)
			e.links = nil
			e.grid = nil
			log.Printf("[SIM] Removed virtual node %s", id)
			return
		}
	}
}

// AddWalker adds a simulated walker.
func (e *Engine) AddWalker(walker *EngineWalker) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.walkers = append(e.walkers, walker)
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
func (e *Engine) GetVirtualNodes() []*VirtualNode {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Return copies to avoid mutation
	nodes := make([]*VirtualNode, len(e.virtualNodes))
	for i, n := range e.virtualNodes {
		nodeCopy := *n
		nodes[i] = &nodeCopy
	}
	return nodes
}

// GetWalkers returns all walkers.
func (e *Engine) GetWalkers() []*EngineWalker {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Return copies
	walkers := make([]*EngineWalker, len(e.walkers))
	for i, w := range e.walkers {
		walkerCopy := *w
		walkers[i] = &walkerCopy
	}
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
	recommendations := e.generateRecommendations(coverageScore, gdopMap)

	result := &SimulationResult{
		Timestamp:       time.Now().UnixMilli(),
		Blobs:           blobResults,
		CoverageScore:   coverageScore,
		GDOPMap:         gdopMap,
		GridDimensions:  []int{e.grid.WidthCells, e.grid.DepthCells, e.grid.HeightCells},
		Recommendations: recommendations,
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

	widthCells := int(math.Ceil(e.space.Width / cellSize))
	depthCells := int(math.Ceil(e.space.Depth / cellSize))
	heightCells := int(math.Ceil(e.space.Height / cellSize))

	e.grid = &Grid{
		CellSize:    cellSize,
		OriginX:     e.space.OriginX,
		OriginZ:     e.space.OriginZ,
		WidthCells:  widthCells,
		DepthCells:  depthCells,
		HeightCells: heightCells,
		Data:        make([]float64, widthCells*depthCells*heightCells),
	}

	log.Printf("[SIM] Grid initialized: %dx%dx%d cells", widthCells, depthCells, heightCells)
}

// generateLinks creates virtual links between all node pairs.
func (e *Engine) generateLinks() {
	e.links = make([]*EngineLink, 0)

	// Create links between all pairs of nodes
	for i, tx := range e.virtualNodes {
		for j, rx := range e.virtualNodes {
			if i >= j {
				continue // Skip duplicates and self
			}

			link := &Link{
				ID:         tx.ID + ":" + rx.ID,
				TXNodeID:   tx.ID,
				RXNodeID:   rx.ID,
				TXPosition: tx.Position,
				RXPosition: rx.Position,
			}

			// Compute link length
			dx := rx.Position[0] - tx.Position[0]
			dy := rx.Position[1] - tx.Position[1]
			dz := rx.Position[2] - tx.Position[2]
			link.Length = math.Sqrt(dx*dx + dy*dy + dz*dz)

			// Precompute zone cache for this link
			link.ZoneCache = e.computeZoneCache(link)

			e.links = append(e.links, link)
		}
	}

	log.Printf("[SIM] Generated %d links", len(e.links))
}

// computeZoneCache precomputes Fresnel zone numbers for all grid cells.
func (e *Engine) computeZoneCache(link *EngineLink) []*ZoneInfo {
	const lambda = 0.123    // WiFi wavelength
	halfLambda := lambda / 2

	cache := make([]*ZoneInfo, 0)

	for z := 0; z < e.grid.HeightCells; z++ {
		for x := 0; x < e.grid.WidthCells; x++ {
			for y := 0; y < e.grid.DepthCells; y++ {
				// Cell center position
				cx := e.grid.OriginX + float64(x)*e.grid.CellSize + e.grid.CellSize/2
				cy := e.grid.OriginZ + float64(y)*e.grid.CellSize + e.grid.CellSize/2
				cz := float64(z) * e.grid.CellSize + e.grid.CellSize/2

				// Path length excess at this cell position
				pathViaCell := math.Sqrt(
					math.Pow(cx-link.TXPosition[0], 2) +
						math.Pow(cy-link.TXPosition[1], 2) +
						math.Pow(cz-link.TXPosition[2], 2))
				pathViaCell += math.Sqrt(
					math.Pow(link.RXPosition[0]-cx, 2) +
						math.Pow(link.RXPosition[1]-cy, 2) +
						math.Pow(link.RXPosition[2]-cz, 2))
				directPath := link.Length

				deltaL := pathViaCell - directPath
				zoneNumber := int(math.Ceil(deltaL / halfLambda))

				if zoneNumber > 5 {
					continue // Outside zone 5, skip
				}

				// Zone decay (default decay_rate = 2.0)
				decay := 1.0 / math.Pow(float64(zoneNumber), 2.0)

				cellIndex := z*e.grid.WidthCells*e.grid.DepthCells + x*e.grid.DepthCells + y
				cache = append(cache, &ZoneInfo{
					CellIndex: cellIndex,
					Zone:      zoneNumber,
					Decay:     decay,
				})
			}
		}
	}

	return cache
}

// updateWalkers updates all walker positions.
func (e *Engine) updateWalkers() {
	const dt = 0.1 // 100ms time step

	for _, walker := range e.walkers {
		if walker.PathType == "path" && len(walker.PathPoints) > 0 {
			// Follow path
			target := walker.PathPoints[walker.CurrentPath]
			dx := target[0] - walker.Position[0]
			dy := target[1] - walker.Position[1]
			dz := target[2] - walker.Position[2]
			dist := math.Sqrt(dx*dx + dy*dy + dz*dz)

			if dist < 0.1 {
				// Reached waypoint, move to next
				walker.CurrentPath = (walker.CurrentPath + 1) % len(walker.PathPoints)
			} else {
				// Move toward target
				speed := 0.5 // m/s
				walker.Position[0] += (dx / dist) * speed * dt
				walker.Position[1] += (dy / dist) * speed * dt
				walker.Position[2] += (dz / dist) * speed * dt
			}
		} else {
			// Random walk
			walker.Position[0] += walker.Velocity[0] * dt
			walker.Position[1] += walker.Velocity[1] * dt
			walker.Position[2] += walker.Velocity[2] * dt

			// Bounce off walls
			if walker.Position[0] < e.space.OriginX || walker.Position[0] > e.space.OriginX+e.space.Width {
				walker.Velocity[0] *= -1
				walker.Position[0] = math.Max(e.space.OriginX, math.Min(e.space.OriginX+e.space.Width, walker.Position[0]))
			}
			if walker.Position[1] < e.space.OriginZ || walker.Position[1] > e.space.OriginZ+e.space.Depth {
				walker.Velocity[1] *= -1
				walker.Position[1] = math.Max(e.space.OriginZ, math.Min(e.space.OriginZ+e.space.Depth, walker.Position[1]))
			}

			// Random velocity perturbation
			walker.Velocity[0] += (rand.Float64() - 0.5) * 0.1
			walker.Velocity[1] += (rand.Float64() - 0.5) * 0.1

			// Clamp velocity
			maxSpeed := 0.5
			speed := math.Sqrt(walker.Velocity[0]*walker.Velocity[0] + walker.Velocity[1]*walker.Velocity[1])
			if speed > maxSpeed {
				scale := maxSpeed / speed
				walker.Velocity[0] *= scale
				walker.Velocity[1] *= scale
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
			amplitude := e.computeCSIAtPosition(link, walker.Position)

			// Add to grid cells covered by this link
			for _, zoneInfo := range link.ZoneCache {
				contribution := amplitude * zoneInfo.Decay
				e.grid.Data[zoneInfo.CellIndex] += contribution
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
					posY := e.grid.OriginZ + float64(y)*e.grid.CellSize + e.grid.CellSize/2
					posZ := float64(z) * e.grid.CellSize + e.grid.CellSize/2

					// Find nearest walker
					nearestWalker := ""
					minDist := 999.0
					for _, walker := range e.walkers {
						dx := walker.Position[0] - posX
						dy := walker.Position[1] - posY
						dz := walker.Position[2] - posZ
						dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
						if dist < minDist {
							minDist = dist
							nearestWalker = walker.ID
						}
					}

					blobs = append(blobs, BlobResult{
						ID:         blobID,
						Position:   [3]float64{posX, posY, posZ},
						Confidence: math.Min(1.0, value/5.0), // Normalize confidence
						WalkerID:   nearestWalker,
					})
					blobID++
				}
			}
		}
	}

	return blobs
}

// computeCSIAtPosition computes simulated CSI amplitude at a position.
func (e *Engine) computeCSIAtPosition(link *EngineLink, pos [3]float64) float64 {
	// Simplified path loss model
	// PL(d) = PL_0 + 10*n*log10(d/d_0)
	// PL_0 = 40 dB at d_0 = 1m, n = 2.0 (free space)

	dx := pos[0] - link.TXPosition[0]
	dy := pos[1] - link.TXPosition[1]
	dz := pos[2] - link.TXPosition[2]
	distFromTX := math.Sqrt(dx*dx + dy*dy + dz*dz)

	dx = pos[0] - link.RXPosition[0]
	dy = pos[1] - link.RXPosition[1]
	dz = pos[2] - link.RXPosition[2]
	distFromRX := math.Sqrt(dx*dx + dy*dy + dz*dz)

	totalDist := distFromTX + distFromRX
	pathLoss := 40.0 + 20.0*math.Log10(totalDist)

	// Convert path loss to linear amplitude
	// Amplitude ~ 10^(-pathLoss/20)
	amplitude := math.Pow(10.0, -pathLoss/20.0)

	// Scale to reasonable values
	return amplitude * 1000.0
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
				cy := e.grid.OriginZ + float64(y)*e.grid.CellSize + e.grid.CellSize/2
				cz := float64(z) * e.grid.CellSize + e.grid.CellSize/2

				gdopMap[cellIndex] = e.computeGDOPAt(cx, cy, cz)
			}
		}
	}

	return gdopMap
}

// computeGDOPAt computes GDOP at a specific position.
func (e *Engine) computeGDOPAt(x, y, z float64) float64 {
	// Collect links that cover this point (within zone 5)
	var angles []float64
	linkCount := 0

	for _, link := range e.links {
		// Check if this point is within zone 5
		dx := x - link.TXPosition[0]
		dy := y - link.TXPosition[1]
		dz := z - link.TXPosition[2]
		distFromTX := math.Sqrt(dx*dx + dy*dy + dz*dz)

		dx = x - link.RXPosition[0]
		dy = y - link.RXPosition[1]
		dz = z - link.RXPosition[2]
		distFromRX := math.Sqrt(dx*dx + dy*dy + dz*dz)

		totalDist := distFromTX + distFromRX
		deltaL := totalDist - link.Length

		const halfLambda = 0.0615
		zoneNumber := int(math.Ceil(deltaL / halfLambda))

		if zoneNumber <= 5 {
			linkCount++
			// Compute angle to link direction
			angle := math.Atan2(link.RXPosition[1]-link.TXPosition[1], link.RXPosition[0]-link.TXPosition[0])
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

// generateRecommendations generates deployment recommendations.
func (e *Engine) generateRecommendations(coverageScore float64, gdopMap []float64) []string {
	recs := make([]string, 0)

	if coverageScore < 50 {
		recs = append(recs, "Coverage is below 50%. Consider adding more nodes.")
	}

	// Find worst coverage areas
	worstX, worstY, worstZ := -1, -1, -1
	maxGDOP := 0.0

	for z := 0; z < e.grid.HeightCells; z++ {
		for x := 0; x < e.grid.WidthCells; x++ {
			for y := 0; y < e.grid.DepthCells; y++ {
				idx := z*e.grid.WidthCells*e.grid.DepthCells + x*e.grid.DepthCells + y
				if gdopMap[idx] > maxGDOP {
					maxGDOP = gdopMap[idx]
					worstX, worstY, worstZ = x, y, z
				}
			}
		}
	}

	if maxGDOP > 10.0 {
		posX := e.grid.OriginX + float64(worstX)*e.grid.CellSize
		posY := e.grid.OriginZ + float64(worstY)*e.grid.CellSize
		recs = append(recs, fmt.Sprintf("Worst coverage at (%.1f, %.1f). Consider adding a node nearby.", posX, posY))
	}

	// Check for node count recommendations
	nodeCount := len(e.virtualNodes)
	if nodeCount < 4 {
		recs = append(recs, fmt.Sprintf("Only %d nodes. For best accuracy, use at least 4 nodes.", nodeCount))
	}

	// Check height diversity
	hasLow, hasHigh := false, false
	for _, node := range e.virtualNodes {
		if node.Position[2] < 1.0 {
			hasLow = true
		}
		if node.Position[2] > 2.0 {
			hasHigh = true
		}
	}

	if !hasLow || !hasHigh {
		recs = append(recs, "For better Z-axis accuracy, place nodes at mixed heights (some low, some high).")
	}

	if len(recs) == 0 {
		recs = append(recs, "Coverage looks good! No specific recommendations.")
	}

	return recs
}

// Errors
var (
	ErrNodeOutsideSpace = fmt.Errorf("node position is outside the defined space")
)
