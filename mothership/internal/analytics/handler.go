package analytics

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi"
)

// Handler provides REST API handlers for analytics.
type Handler struct {
	accumulator *FlowAccumulator
}

// NewHandler creates a new analytics handler.
func NewHandler(accumulator *FlowAccumulator) *Handler {
	return &Handler{accumulator: accumulator}
}

// RegisterRoutes registers analytics API routes on the given router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/analytics/flow", h.handleGetFlow)
	r.Get("/api/analytics/dwell", h.handleGetDwell)
	r.Get("/api/analytics/corridors", h.handleGetCorridors)
}

// handleGetFlow returns the flow map.
// Query params:
//   - person_id: filter by person (optional)
//   - since: Unix timestamp (optional, default 30 days ago)
//   - until: Unix timestamp (optional, default now)
func (h *Handler) handleGetFlow(w http.ResponseWriter, r *http.Request) {
	if h.accumulator == nil {
		http.Error(w, "analytics not available", http.StatusServiceUnavailable)
		return
	}

	personID := r.URL.Query().Get("person_id")

	// Parse time range
	var since, until time.Time
	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		if sinceUnix, err := strconv.ParseInt(sinceStr, 10, 64); err == nil {
			since = time.Unix(sinceUnix, 0)
		}
	}
	if untilStr := r.URL.Query().Get("until"); untilStr != "" {
		if untilUnix, err := strconv.ParseInt(untilStr, 10, 64); err == nil {
			until = time.Unix(untilUnix, 0)
		}
	}

	// Default time range: last 30 days
	if since.IsZero() {
		since = time.Now().AddDate(0, 0, -30)
	}
	if until.IsZero() {
		until = time.Now()
	}

	flowMap, err := h.accumulator.GetFlowMap(personID, since, until)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, flowMap)
}

// handleGetDwell returns the dwell heatmap.
// Query params:
//   - person_id: filter by person (optional)
func (h *Handler) handleGetDwell(w http.ResponseWriter, r *http.Request) {
	if h.accumulator == nil {
		http.Error(w, "analytics not available", http.StatusServiceUnavailable)
		return
	}

	personID := r.URL.Query().Get("person_id")

	heatmap, err := h.accumulator.GetDwellHeatmap(personID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, heatmap)
}

// handleGetCorridors returns detected corridors.
func (h *Handler) handleGetCorridors(w http.ResponseWriter, r *http.Request) {
	if h.accumulator == nil {
		http.Error(w, "analytics not available", http.StatusServiceUnavailable)
		return
	}

	corridors, err := h.accumulator.GetCorridors()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, corridors)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
