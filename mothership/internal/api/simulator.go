// Package api provides REST API handlers for Spaxel simulator.
package api

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"strconv"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/spaxel/mothership/internal/simulator"
)

// SimulatorHandler manages pre-deployment simulation API endpoints.
// It allows users to define virtual spaces, place virtual nodes, simulate walkers,
// and compute GDOP coverage quality before purchasing hardware.
type SimulatorHandler struct {
	mu       sync.RWMutex
	space    *simulator.Space
	nodes    *simulator.NodeSet
	walkers  *simulator.WalkerSet
}

// NewSimulatorHandler creates a new simulator handler.
func NewSimulatorHandler() *SimulatorHandler {
	// Start with a default space
	return &SimulatorHandler{
		space:   simulator.DefaultSpace(),
		nodes:   simulator.NewNodeSet(),
		walkers: simulator.NewWalkerSet(),
	}
}

// RegisterRoutes registers simulator routes on the router.
func (h *SimulatorHandler) RegisterRoutes(r chi.Router) {
	r.Route("/simulator", func(r chi.Router) {
		r.Get("/", h.GetState)
		r.Post("/reset", h.Reset)

		// Space management
		r.Route("/space", func(r chi.Router) {
			r.Get("/", h.GetSpace)
			r.Put("/", h.SetSpace)
			r.Post("/validate", h.ValidateSpace)
		})

		// Node management
		r.Route("/nodes", func(r chi.Router) {
			r.Get("/", h.GetNodes)
			r.Post("/", h.AddNode)
			r.Route("/{nodeID}", func(r chi.Router) {
				r.Delete("/", h.RemoveNode)
				r.Put("/", h.UpdateNode)
			})
			r.Post("/suggest", h.SuggestNodes)
			r.Post("/optimize", h.OptimizeNodes)
		})

		// Walker management
		r.Route("/walkers", func(r chi.Router) {
			r.Get("/", h.GetWalkers)
			r.Post("/", h.AddWalker)
			r.Post("/random", h.AddRandomWalkers)
			r.Post("/path", h.AddPathWalkers)
			r.Delete("/{walkerID}", h.RemoveWalker)
		})

		// GDOP computation
		r.Route("/gdop", func(r chi.Router) {
			r.Post("/compute", h.ComputeGDOP)
			r.Get("/coverage", h.GetCoverageScore)
			r.Get("/heatmap", h.GetGDOPHeatmap)
		})

		// Shopping list
		r.Get("/shopping-list", h.GetShoppingList)

		// Simulation
		r.Post("/simulate", h.RunSimulation)
	})
}

// GetState returns the complete simulator state
func (h *SimulatorHandler) GetState(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	state := map[string]interface{}{
		"space":   h.space,
		"nodes":   h.nodes.All(),
		"walkers": h.walkers.All(),
	}

	respondJSON(w, http.StatusOK, state)
}

// Reset resets the simulator to default state
func (h *SimulatorHandler) Reset(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.space = simulator.DefaultSpace()
	h.nodes = simulator.NewNodeSet()
	h.walkers = simulator.NewWalkerSet()

	respondJSON(w, http.StatusOK, map[string]string{"status": "reset"})
}

// GetSpace returns the current space definition
func (h *SimulatorHandler) GetSpace(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	respondJSON(w, http.StatusOK, h.space)
}

// SetSpace updates the space definition
func (h *SimulatorHandler) SetSpace(w http.ResponseWriter, r *http.Request) {
	var space simulator.Space
	if err := json.NewDecoder(r.Body).Decode(&space); err != nil {
		respondError(w, http.StatusBadRequest, "invalid space JSON")
		return
	}

	if err := space.Validate(); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.mu.Lock()
	h.space = &space
	h.mu.Unlock()

	respondJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// ValidateSpace validates the current space without modifying it
func (h *SimulatorHandler) ValidateSpace(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if err := h.space.Validate(); err != nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"valid": false,
			"error": err.Error(),
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"valid":       true,
		"volume_m3":   h.space.TotalVolume(),
		"bounds":      getBoundsJSON(h.space),
		"room_count":  len(h.space.Rooms),
		"wall_count":  len(h.space.GetWalls()),
	})
}

// GetNodes returns all virtual nodes
func (h *SimulatorHandler) GetNodes(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	respondJSON(w, http.StatusOK, h.nodes.All())
}

// AddNode adds a new virtual node
func (h *SimulatorHandler) AddNode(w http.ResponseWriter, r *http.Request) {
	var node simulator.Node
	if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
		respondError(w, http.StatusBadRequest, "invalid node JSON")
		return
	}

	h.mu.Lock()
	h.nodes.Add(&node)
	h.mu.Unlock()

	respondJSON(w, http.StatusCreated, node)
}

// UpdateNode updates an existing node
func (h *SimulatorHandler) UpdateNode(w http.ResponseWriter, r *http.Request) {
	nodeID := chi.URLParam(r, "nodeID")

	var node simulator.Node
	if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
		respondError(w, http.StatusBadRequest, "invalid node JSON")
		return
	}

	node.ID = nodeID // Ensure ID matches URL parameter

	h.mu.Lock()
	// Remove old node and add updated one
	h.nodes.Remove(nodeID)
	h.nodes.Add(&node)
	h.mu.Unlock()

	respondJSON(w, http.StatusOK, node)
}

// RemoveNode removes a virtual node
func (h *SimulatorHandler) RemoveNode(w http.ResponseWriter, r *http.Request) {
	nodeID := chi.URLParam(r, "nodeID")

	h.mu.Lock()
	removed := h.nodes.Remove(nodeID)
	h.mu.Unlock()

	if !removed {
		respondError(w, http.StatusNotFound, "node not found")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// SuggestNodes suggests optimal node positions for the current space
func (h *SimulatorHandler) SuggestNodes(w http.ResponseWriter, r *http.Request) {
	// Parse count from query string
	countStr := r.URL.Query().Get("count")
	count, err := strconv.Atoi(countStr)
	if err != nil || count < 1 {
		count = 4 // Default to 4 nodes
	}

	h.mu.RLock()
	space := h.space
	h.mu.RUnlock()

	suggested := simulator.SuggestedNodes(space, count)
	respondJSON(w, http.StatusOK, suggested.All())
}

// OptimizeNodes optimizes node positions for best coverage
func (h *SimulatorHandler) OptimizeNodes(w http.ResponseWriter, r *http.Request) {
	// Parse parameters
	countStr := r.URL.Query().Get("count")
	count, err := strconv.Atoi(countStr)
	if err != nil || count < 1 {
		count = 4
	}

	iterationsStr := r.URL.Query().Get("iterations")
	iterations, err := strconv.Atoi(iterationsStr)
	if err != nil || iterations < 1 {
		iterations = 50 // Default iterations
	}

	h.mu.RLock()
	space := h.space
	h.mu.RUnlock()

	optimized := simulator.OptimizeNodePositions(space, count, iterations)
	respondJSON(w, http.StatusOK, optimized.All())
}

// GetWalkers returns all walkers
func (h *SimulatorHandler) GetWalkers(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	respondJSON(w, http.StatusOK, h.walkers.All())
}

// AddWalker adds a new walker
func (h *SimulatorHandler) AddWalker(w http.ResponseWriter, r *http.Request) {
	var walker simulator.Walker
	if err := json.NewDecoder(r.Body).Decode(&walker); err != nil {
		respondError(w, http.StatusBadRequest, "invalid walker JSON")
		return
	}

	h.mu.Lock()
	h.walkers.Add(&walker)
	h.mu.Unlock()

	respondJSON(w, http.StatusCreated, walker)
}

// AddRandomWalkers adds random walkers to the simulation
func (h *SimulatorHandler) AddRandomWalkers(w http.ResponseWriter, r *http.Request) {
	// Parse count from query string
	countStr := r.URL.Query().Get("count")
	count, err := strconv.Atoi(countStr)
	if err != nil || count < 1 {
		count = 1
	}

	h.mu.RLock()
	space := h.space
	h.mu.RUnlock()

	walkers := simulator.CreateRandomWalkers(count, space)

	h.mu.Lock()
	for _, w := range walkers.All() {
		h.walkers.Add(w)
	}
	h.mu.Unlock()

	respondJSON(w, http.StatusCreated, walkers.All())
}

// AddPathWalkers adds path-following walkers
func (h *SimulatorHandler) AddPathWalkers(w http.ResponseWriter, r *http.Request) {
	// Parse count from query string
	countStr := r.URL.Query().Get("count")
	count, err := strconv.Atoi(countStr)
	if err != nil || count < 1 {
		count = 1
	}

	h.mu.RLock()
	space := h.space
	h.mu.RUnlock()

	walkers := simulator.CreatePathWalkers(count, space)

	h.mu.Lock()
	for _, w := range walkers.All() {
		h.walkers.Add(w)
	}
	h.mu.Unlock()

	respondJSON(w, http.StatusCreated, walkers.All())
}

// RemoveWalker removes a walker
func (h *SimulatorHandler) RemoveWalker(w http.ResponseWriter, r *http.Request) {
	walkerID := chi.URLParam(r, "walkerID")

	h.mu.Lock()
	removed := h.walkers.Remove(walkerID)
	h.mu.Unlock()

	if !removed {
		respondError(w, http.StatusNotFound, "walker not found")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// GDOPRequest contains parameters for GDOP computation
type GDOPRequest struct {
	CellSize  float64 `json:"cell_size"`  // Grid cell size in meters
	MaxZone   int     `json:"max_zone"`   // Maximum Fresnel zone to consider
	Threshold float64 `json:"threshold"`  // DeltaRMS threshold for active links
}

// GDOPResponse contains GDOP computation results
type GDOPResponse struct {
	Results        [][]simulator.GDOPResult `json:"results"`
	CoverageScore  float64                   `json:"coverage_score"`
	AverageGDOP    float64                   `json:"average_gdop"`
	QualityCounts  map[string]int            `json:"quality_counts"`
	DeadZones      []simulator.Point         `json:"dead_zones"`
	RecommendedPos simulator.Point          `json:"recommended_position"`
	Links          []simulator.Link          `json:"links"`
}

// ComputeGDOP computes GDOP for the current configuration
func (h *SimulatorHandler) ComputeGDOP(w http.ResponseWriter, r *http.Request) {
	var req GDOPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Use defaults if request body is empty
		req = GDOPRequest{
			CellSize:  0.2,
			MaxZone:   3,
			Threshold: 0.02,
		}
	}

	h.mu.RLock()
	space := h.space
	nodes := h.nodes
	walkers := h.walkers
	h.mu.RUnlock()

	if nodes.Count() < 2 {
		respondError(w, http.StatusBadRequest, "need at least 2 nodes for GDOP computation")
		return
	}

	minX, minY, _, maxX, maxY, _ := space.Bounds()

	// Generate links
	links := simulator.GenerateAllLinks(nodes)

	// Create GDOP computer
	config := simulator.GridConfig{
		MinX:     minX,
		MinY:     minY,
		Width:    maxX - minX,
		Depth:    maxY - minY,
		CellSize: req.CellSize,
	}
	gdopComp := simulator.NewGDOPComputer(links, config)
	if req.MaxZone > 0 {
		gdopComp.SetMaxZone(req.MaxZone)
	}

	// Compute GDOP
	results := gdopComp.ComputeAll()

	// Compute statistics
	coverageScore := gdopComp.CoverageScore(results)
	avgGDOP := gdopComp.AverageGDOP(results)
	qualityCounts := gdopComp.QualityCounts(results)
	deadZones := gdopComp.FindDeadZones(results)
	recommendedPos := gdopComp.RecommendNodePosition(results, space)

	response := GDOPResponse{
		Results:        results,
		CoverageScore:  coverageScore,
		AverageGDOP:    avgGDOP,
		QualityCounts:  qualityCounts,
		DeadZones:      deadZones,
		RecommendedPos: recommendedPos,
		Links:          links,
	}

	respondJSON(w, http.StatusOK, response)
}

// GetCoverageScore returns just the coverage score for quick assessment
func (h *SimulatorHandler) GetCoverageScore(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	space := h.space
	nodes := h.nodes
	h.mu.RUnlock()

	if nodes.Count() < 2 {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"coverage_percent": 0,
			"minimum_nodes":    simulator.MinimumNodeCount(space, 4.0),
			"current_nodes":    nodes.Count(),
		})
		return
	}

	minX, minY, _, maxX, maxY, _ := space.Bounds()
	links := simulator.GenerateAllLinks(nodes)

	config := simulator.GridConfig{
		MinX:     minX,
		MinY:     minY,
		Width:    maxX - minX,
		Depth:    maxY - minY,
		CellSize: 0.2,
	}
	gdopComp := simulator.NewGDOPComputer(links, config)
	results := gdopComp.ComputeAll()

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"coverage_percent": gdopComp.CoverageScore(results),
		"minimum_nodes":    simulator.MinimumNodeCount(space, 4.0),
		"current_nodes":    nodes.Count(),
		"average_gdop":     gdopComp.AverageGDOP(results),
	})
}

// GetGDOPHeatmap returns GDOP data in a format suitable for heatmap visualization
func (h *SimulatorHandler) GetGDOPHeatmap(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	space := h.space
	nodes := h.nodes
	h.mu.RUnlock()

	if nodes.Count() < 2 {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"gdop_map":         []float64{},
			"grid_dimensions": []int{0, 0, 0},
			"coverage_percent": 0,
			"error":            "need at least 2 nodes",
		})
		return
	}

	minX, minY, _, maxX, maxY, _ := space.Bounds()
	links := simulator.GenerateAllLinks(nodes)

	config := simulator.GridConfig{
		MinX:     minX,
		MinY:     minY,
		Width:    maxX - minX,
		Depth:    maxY - minY,
		CellSize: 0.2,
	}
	gdopComp := simulator.NewGDOPComputer(links, config)
	results := gdopComp.ComputeAll()

	// Convert results to heatmap format
	depth := len(results)
	width := 0
	if depth > 0 {
		width = len(results[0])
	}

	// Flatten GDOP values into 1D array (row-major order)
	gdopMap := make([]float64, width*depth)
	for y := 0; y < depth; y++ {
		for x := 0; x < width; x++ {
			idx := y*width + x
			if math.IsInf(results[y][x].GDOP, 0) {
				gdopMap[idx] = 9999.0 // Use 9999 to represent infinity
			} else {
				gdopMap[idx] = results[y][x].GDOP
			}
		}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"gdop_map":         gdopMap,
		"grid_dimensions": []int{width, depth, 1}, // 2D heatmap, so height = 1
		"coverage_percent": gdopComp.CoverageScore(results),
		"average_gdop":     gdopComp.AverageGDOP(results),
		"quality_counts":   gdopComp.QualityCounts(results),
	})
}

// GetShoppingList returns hardware recommendations
func (h *SimulatorHandler) GetShoppingList(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	space := h.space
	nodes := h.nodes
	h.mu.RUnlock()

	shoppingList := simulator.GenerateShoppingList(space, nodes)
	respondJSON(w, http.StatusOK, shoppingList)
}

// SimulationRequest contains parameters for running a simulation
type SimulationRequest struct {
	DurationSec int     `json:"duration_sec"` // Simulation duration in seconds
	RateHz      int     `json:"rate_hz"`      // Update rate in Hz
	Threshold   float64 `json:"threshold"`    // DeltaRMS threshold
}

// SimulationResponse contains simulation results
type SimulationResponse struct {
	WalkerPositions []simulator.Point `json:"walker_positions"`
	LinkActivity    map[string]float64 `json:"link_activity"`
	Duration        int                `json:"duration"`
	Ticks           int                `json:"ticks"`
}

// RunSimulation runs a time-step simulation with the current configuration
func (h *SimulatorHandler) RunSimulation(w http.ResponseWriter, r *http.Request) {
	var req SimulationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req = SimulationRequest{
			DurationSec: 10,
			RateHz:      10,
			Threshold:   0.02,
		}
	}

	h.mu.RLock()
	space := h.space
	nodes := h.nodes
	walkers := h.walkers
	h.mu.RUnlock()

	if walkers.Count() == 0 {
		respondError(w, http.StatusBadRequest, "no walkers in simulation")
		return
	}

	if nodes.Count() < 2 {
		respondError(w, http.StatusBadRequest, "need at least 2 nodes for simulation")
		return
	}

	// Create propagation model
	propModel := simulator.NewPropagationModel(space)

	// Generate all links
	links := simulator.GenerateAllLinks(nodes)

	// Run simulation ticks
	dt := 1.0 / float64(req.RateHz)
	numTicks := req.DurationSec * req.RateHz

	// Collect final positions and link activity
	finalPositions := make([]simulator.Point, 0)
	linkActivity := make(map[string]float64)

	for tick := 0; tick < numTicks; tick++ {
		// Update walker positions
		walkers.Update(dt, space)

		// Compute link activity
		for _, link := range links {
			linkID := link.TX.ID + ":" + link.RX.ID
			maxDelta := 0.0

			for _, walker := range walkers.All() {
				delta := propModel.ComputeLinkActivity(link, walker.Position, req.Threshold)
				if delta > maxDelta {
					maxDelta = delta
				}
			}

			if maxDelta > linkActivity[linkID] {
				linkActivity[linkID] = maxDelta
			}
		}
	}

	// Collect final walker positions
	for _, w := range walkers.All() {
		finalPositions = append(finalPositions, w.Position)
	}

	response := SimulationResponse{
		WalkerPositions: finalPositions,
		LinkActivity:    linkActivity,
		Duration:        req.DurationSec,
		Ticks:           numTicks,
	}

	respondJSON(w, http.StatusOK, response)
}

// Helper functions

func getBoundsJSON(space *simulator.Space) map[string]float64 {
	minX, minY, minZ, maxX, maxY, maxZ := space.Bounds()
	return map[string]float64{
		"min_x": minX,
		"min_y": minY,
		"min_z": minZ,
		"max_x": maxX,
		"max_y": maxY,
		"max_z": maxZ,
	}
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("[ERROR] Failed to encode JSON response: %v", err)
	}
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}
