// Package simulator provides the API handlers for the pre-deployment simulator.
package simulator

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

// Handler provides the HTTP API for the pre-deployment simulator.
type Handler struct {
	mu     sync.RWMutex
	engine *Engine
}

// NewHandler creates a new simulator API handler.
func NewHandler(engine *Engine) *Handler {
	return &Handler{
		engine: engine,
	}
}

// RegisterRoutes registers simulator API routes.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/simulator/space", h.getSpace)
	r.Put("/api/simulator/space", h.setSpace)
	r.Get("/api/simulator/nodes", h.getNodes)
	r.Post("/api/simulator/nodes", h.addNode)
	r.Delete("/api/simulator/nodes/{id}", h.removeNode)
	r.Get("/api/simulator/walkers", h.getWalkers)
	r.Post("/api/simulator/walkers", h.addWalker)
	r.Delete("/api/simulator/walkers/{id}", h.removeWalker)
	r.Post("/api/simulator/simulate", h.simulate)
	r.Get("/api/simulator/results", h.getResults)
	r.Post("/api/simulator/gdop", h.computeGDOP)
	r.Get("/api/simulator/status", h.getStatus)
	r.Post("/api/simulator/subscribe", h.subscribe)
}

// getSpace handles GET /api/simulator/space
func (h *Handler) getSpace(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	space := h.engine.space
	writeJSON(w, space)
}

// setSpace handles PUT /api/simulator/space
func (h *Handler) setSpace(w http.ResponseWriter, r *http.Request) {
	var space Space
	if err := json.NewDecoder(r.Body).Decode(&space); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := space.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	h.engine.SetSpace(&space)
	h.mu.Unlock()

	log.Printf("[SIM] Space updated: %s", space.ID)

	writeJSON(w, map[string]interface{}{
		"ok": true,
	})
}

// getNodes handles GET /api/simulator/nodes
func (h *Handler) getNodes(w http.ResponseWriter, r *http.Request) {
	nodes := h.engine.GetVirtualNodes()
	writeJSON(w, nodes)
}

// addNode handles POST /api/simulator/nodes
func (h *Handler) addNode(w http.ResponseWriter, r *http.Request) {
	var node Node
	if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if node.ID == "" {
		node.ID = fmt.Sprintf("node_%d", time.Now().UnixNano())
	}

	if node.Type == "" {
		node.Type = NodeTypeVirtual
	}

	if err := h.engine.AddVirtualNode(&node); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, node)
}

// removeNode handles DELETE /api/simulator/nodes/{id}
func (h *Handler) removeNode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.engine.RemoveVirtualNode(id)

	writeJSON(w, map[string]interface{}{
		"ok": true,
	})
}

// getWalkers handles GET /api/simulator/walkers
func (h *Handler) getWalkers(w http.ResponseWriter, r *http.Request) {
	walkers := h.engine.GetWalkers()
	writeJSON(w, walkers)
}

// addWalker handles POST /api/simulator/walkers
func (h *Handler) addWalker(w http.ResponseWriter, r *http.Request) {
	var walker SimWalker
	if err := json.NewDecoder(r.Body).Decode(&walker); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if walker.ID == "" {
		walker.ID = fmt.Sprintf("walker_%d", time.Now().UnixNano())
	}

	if walker.Type == "" {
		walker.Type = WalkerTypeRandom
	}

	h.engine.AddWalker(&walker)

	writeJSON(w, walker)
}

// removeWalker handles DELETE /api/simulator/walkers/{id}
func (h *Handler) removeWalker(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.engine.RemoveWalker(id)

	writeJSON(w, map[string]interface{}{
		"ok": true,
	})
}

// simulate handles POST /api/simulator/simulate
// Runs one simulation tick and returns results.
func (h *Handler) simulate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DurationSec  int     `json:"duration_sec"`
		TickRateHz   int     `json:"tick_rate_hz"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Use defaults if request body is empty
		req.DurationSec = 30
		req.TickRateHz = 10
	}

	result := h.engine.RunSimulation()
	writeJSON(w, result)
}

// getResults handles GET /api/simulator/results
// Returns the most recent simulation results.
func (h *Handler) getResults(w http.ResponseWriter, r *http.Request) {
	result := h.engine.GetResults()
	if result == nil {
		// Run a simulation if no results yet
		result = h.engine.RunSimulation()
	}
	writeJSON(w, result)
}

// computeGDOP handles POST /api/simulator/gdop
func (h *Handler) computeGDOP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Nodes []Node `json:"nodes"`
		Space *Space  `json:"space"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Space == nil {
		req.Space = DefaultSpace()
	}

	// Create temporary engine for GDOP computation
	engine := NewEngine(req.Space)
	for _, node := range req.Nodes {
		engine.AddVirtualNode(&node)
	}

	// Run simulation to get GDOP map
	result := engine.RunSimulation()

	writeJSON(w, map[string]interface{}{
		"gdop_map":        result.GDOPMap,
		"grid_dimensions": result.GridDimensions,
		"coverage_score":  result.CoverageScore,
	})
}

// getStatus handles GET /api/simulator/status
func (h *Handler) getStatus(w http.ResponseWriter, r *http.Request) {
	result := h.engine.GetResults()

	walkerPositions := make([]map[string]interface{}, 0)
	if result != nil {
		for _, blob := range result.Blobs {
			walkerPositions = append(walkerPositions, map[string]interface{}{
				"id":       blob.WalkerID,
				"position": blob.Position,
			})
		}
	}

	writeJSON(w, map[string]interface{}{
		"state":            "running",
		"walker_positions": walkerPositions,
	})
}

// subscribe handles POST /api/simulator/subscribe
// Creates Server-Sent Events stream for simulation updates.
func (h *Handler) subscribe(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Subscribe to results
	ch := h.engine.Subscribe()
	defer h.engine.Unsubscribe(ch)

	// Send initial results
	result := h.engine.GetResults()
	if result != nil {
		sendSSEEvent(w, "simulation", result)
	}
	flusher.Flush()

	// Keep connection alive and send updates
	notify := r.Context().Done()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-notify:
			return
		case result := <-ch:
			sendSSEEvent(w, "simulation", result)
			flusher.Flush()
		case <-ticker.C:
			// Send keepalive
			sendSSEComment(w, "keepalive")
			flusher.Flush()
		}
	}
}

// sendSSEEvent sends an SSE event.
func sendSSEEvent(w http.ResponseWriter, event string, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, jsonData)
}

// sendSSEComment sends an SSE comment.
func sendSSEComment(w http.ResponseWriter, comment string) {
	fmt.Fprintf(w, ": %s\n\n", comment)
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	data, err := json.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(data)
}
