package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/spaxel/mothership/internal/ingestion"
)

// FrameStatsHandler manages frame rate statistics API endpoints (ADR-003)
type FrameStatsHandler struct {
	frameTracker func() *ingestion.FrameTracker
}

// NewFrameStatsHandler creates a new frame stats API handler
func NewFrameStatsHandler(getTracker func() *ingestion.FrameTracker) *FrameStatsHandler {
	return &FrameStatsHandler{
		frameTracker: getTracker,
	}
}

// RegisterRoutes registers frame statistics endpoints
func (h *FrameStatsHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/framestats/all", h.getAllStats)
	r.Get("/api/framestats/link/{linkID}", h.getLinkStats)
}

// getAllStats handles GET /api/framestats/all
func (h *FrameStatsHandler) getAllStats(w http.ResponseWriter, r *http.Request) {
	if h.frameTracker == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "frame tracker not available")
		return
	}

	ft := h.frameTracker()
	if ft == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "frame tracker not initialized")
		return
	}

	stats := ft.GetAllStats()
	writeJSON(w, http.StatusOK, stats)
}

// getLinkStats handles GET /api/framestats/link/{linkID}
func (h *FrameStatsHandler) getLinkStats(w http.ResponseWriter, r *http.Request) {
	linkID := chi.URLParam(r, "linkID")
	if linkID == "" {
		writeJSONError(w, http.StatusBadRequest, "missing link ID")
		return
	}

	if h.frameTracker == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "frame tracker not available")
		return
	}

	ft := h.frameTracker()
	if ft == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "frame tracker not initialized")
		return
	}

	rate := ft.GetFrameRate(linkID)
	count := ft.GetFrameCount(linkID)

	result := map[string]interface{}{
		"link_id":     linkID,
		"frame_rate":  rate,
		"frame_count": count,
	}

	writeJSON(w, http.StatusOK, result)
}
