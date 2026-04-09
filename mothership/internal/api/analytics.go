// Package api provides REST API handlers for crowd flow analytics.
package api

import (
	"database/sql"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/spaxel/mothership/internal/analytics"
)

// AnalyticsHandler manages the crowd flow analytics API endpoints.
type AnalyticsHandler struct {
	flowAccumulator *analytics.FlowAccumulator
	db             *sql.DB
}

// NewAnalyticsHandler creates a new analytics handler.
func NewAnalyticsHandler(db *sql.DB, cellSizeM float64) *AnalyticsHandler {
	flowAcc := analytics.NewFlowAccumulator(db, cellSizeM)
	if err := flowAcc.InitSchema(); err != nil {
		log.Printf("[WARN] Failed to initialize analytics schema: %v", err)
	}

	// Start background prune job
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if err := flowAcc.PruneOldData(); err != nil {
				log.Printf("[WARN] Failed to prune old analytics data: %v", err)
			}
		}
	}()

	return &AnalyticsHandler{
		flowAccumulator: flowAcc,
		db:             db,
	}
}

// GetFlowAccumulator returns the flow accumulator for use by other packages.
func (h *AnalyticsHandler) GetFlowAccumulator() *analytics.FlowAccumulator {
	return h.flowAccumulator
}

// RegisterRoutes registers analytics endpoints.
func (h *AnalyticsHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/analytics/flow", h.getFlowMap)
	r.Get("/api/analytics/dwell", h.getDwellHeatmap)
	r.Get("/api/analytics/corridors", h.getCorridors)
}

// getFlowMap handles GET /api/analytics/flow
// Query params: person_id (optional), since (ISO8601), until (ISO8601)
func (h *AnalyticsHandler) getFlowMap(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	var personID *string
	if pid := r.URL.Query().Get("person_id"); pid != "" {
		personID = &pid
	}

	var since, until *time.Time
	if s := r.URL.Query().Get("since"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid since timestamp")
			return
		}
		since = &t
	}

	if u := r.URL.Query().Get("until"); u != "" {
		t, err := time.Parse(time.RFC3339, u)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid until timestamp")
			return
		}
		until = &t
	}

	flowMap, err := h.flowAccumulator.ComputeFlowMap(personID, since, until)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to compute flow map")
		return
	}

	writeJSON(w, http.StatusOK, flowMap)
}

// getDwellHeatmap handles GET /api/analytics/dwell
// Query params: person_id (optional)
func (h *AnalyticsHandler) getDwellHeatmap(w http.ResponseWriter, r *http.Request) {
	var personID *string
	if pid := r.URL.Query().Get("person_id"); pid != "" {
		personID = &pid
	}

	heatmap, err := h.flowAccumulator.ComputeDwellHeatmap(personID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to compute dwell heatmap")
		return
	}

	writeJSON(w, http.StatusOK, heatmap)
}

// getCorridors handles GET /api/analytics/corridors
func (h *AnalyticsHandler) getCorridors(w http.ResponseWriter, r *http.Request) {
	corridors, err := h.flowAccumulator.GetCorridors()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to get corridors")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"corridors": corridors,
	})
}
