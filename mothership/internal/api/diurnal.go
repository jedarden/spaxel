// Package api provides REST API handlers for diurnal baseline data.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/spaxel/mothership/internal/signal"
)

// DiurnalHandler manages the diurnal baseline API endpoints.
type DiurnalHandler struct {
	pm DiurnalProcessorManager
}

// DiurnalProcessorManager defines the interface for accessing diurnal data from the signal processor.
type DiurnalProcessorManager interface {
	GetDiurnalLearningStatus() []signal.DiurnalLearningStatus
	GetProcessor(linkID string) DiurnalLinkProcessor
}

// DiurnalLinkProcessor defines the interface for accessing a single link's diurnal data.
type DiurnalLinkProcessor interface {
	GetDiurnal() *signal.DiurnalBaseline
}

// NewDiurnalHandler creates a new diurnal API handler.
func NewDiurnalHandler(pm DiurnalProcessorManager) *DiurnalHandler {
	return &DiurnalHandler{
		pm: pm,
	}
}

// RegisterRoutes registers diurnal endpoints.
func (h *DiurnalHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/diurnal/status", h.getDiurnalStatus)
	r.Get("/api/diurnal/slots/{linkID}", h.getDiurnalSlots)
}

// getDiurnalStatus handles GET /api/diurnal/status
// Returns the diurnal learning status for all links.
func (h *DiurnalHandler) getDiurnalStatus(w http.ResponseWriter, r *http.Request) {
	statuses := h.pm.GetDiurnalLearningStatus()
	writeJSON(w, http.StatusOK, statuses)
}

// getDiurnalSlots handles GET /api/diurnal/slots/{linkID}
// Returns the diurnal baseline slot data for a specific link.
func (h *DiurnalHandler) getDiurnalSlots(w http.ResponseWriter, r *http.Request) {
	linkID := chi.URLParam(r, "linkID")
	if linkID == "" {
		writeJSONError(w, http.StatusBadRequest, "link_id is required")
		return
	}

	processor := h.pm.GetProcessor(linkID)
	if processor == nil {
		writeJSONError(w, http.StatusNotFound, "link not found")
		return
	}

	diurnal := processor.GetDiurnal()
	if diurnal == nil {
		writeJSONError(w, http.StatusNotFound, "diurnal baseline not found")
		return
	}

	snapshot := diurnal.GetSnapshot()
	if snapshot == nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to get diurnal snapshot")
		return
	}

	// Build response with slot data
	response := map[string]interface{}{
		"link_id":           snapshot.LinkID,
		"created_at":        snapshot.Created,
		"current_hour":      snapshot.Created.Hour(), // For consistency with signal package
		"slot_amplitudes":   snapshot.SlotValues,
		"slot_confidences":  diurnal.GetAllSlotConfidences(),
		"slot_counts":       snapshot.SlotCounts,
		"is_learning":       diurnal.IsLearning(),
		"learning_progress": diurnal.GetLearningProgress(),
		"is_ready":          diurnal.IsReady(),
	}

	writeJSON(w, http.StatusOK, response)
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeJSONError writes a JSON error response.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
