package analytics

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
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

// AnomalyHandler provides REST API handlers for anomaly detection.
type AnomalyHandler struct {
	detector *Detector
}

// NewAnomalyHandler creates a new anomaly handler.
func NewAnomalyHandler(detector *Detector) *AnomalyHandler {
	return &AnomalyHandler{detector: detector}
}

// RegisterRoutes registers anomaly API routes on the given router.
func (h *AnomalyHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/anomalies", h.handleGetAnomalies)
	r.Get("/api/anomalies/active", h.handleGetActiveAnomalies)
	r.Get("/api/anomalies/history", h.handleGetHistory)
	r.Post("/api/anomalies/{id}/acknowledge", h.handleAcknowledge)
	r.Get("/api/anomalies/summary", h.handleGetSummary)
	r.Get("/api/anomalies/learning", h.handleGetLearningProgress)
	r.Post("/api/anomalies/model/update", h.handleUpdateModel)
}

// handleGetAnomalies returns anomalies filtered by the `since` query parameter.
// Query params:
//   - since: duration string (e.g. "24h", "7d", "1h"). Default "24h".
// Uses DB-backed QueryAnomalyEvents so results survive server restarts.
func (h *AnomalyHandler) handleGetAnomalies(w http.ResponseWriter, r *http.Request) {
	if h.detector == nil {
		http.Error(w, "anomaly detector not available", http.StatusServiceUnavailable)
		return
	}

	active := h.detector.GetActiveAnomalies()

	// Parse since duration
	sinceStr := r.URL.Query().Get("since")
	if sinceStr == "" {
		sinceStr = "24h"
	}
	sinceDur, err := time.ParseDuration(sinceStr)
	if err != nil {
		http.Error(w, "invalid since duration: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Use DB-backed query so results persist across restarts
	cutoff := time.Now().Add(-sinceDur)
	history, err := h.detector.QueryAnomalyEvents(cutoff, 1000)
	if err != nil {
		http.Error(w, "failed to query anomalies: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"active":  active,
		"history": history,
		"since":   sinceStr,
	}
	writeJSON(w, response)
}

// handleGetActiveAnomalies returns only active (unacknowledged) anomalies.
func (h *AnomalyHandler) handleGetActiveAnomalies(w http.ResponseWriter, r *http.Request) {
	if h.detector == nil {
		http.Error(w, "anomaly detector not available", http.StatusServiceUnavailable)
		return
	}

	active := h.detector.GetActiveAnomalies()
	writeJSON(w, active)
}

// handleGetHistory returns anomaly history.
func (h *AnomalyHandler) handleGetHistory(w http.ResponseWriter, r *http.Request) {
	if h.detector == nil {
		http.Error(w, "anomaly detector not available", http.StatusServiceUnavailable)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			limit = n
		}
	}

	history := h.detector.GetAnomalyHistory(limit)
	writeJSON(w, history)
}

// handleAcknowledge acknowledges an anomaly.
func (h *AnomalyHandler) handleAcknowledge(w http.ResponseWriter, r *http.Request) {
	if h.detector == nil {
		http.Error(w, "anomaly detector not available", http.StatusServiceUnavailable)
		return
	}

	anomalyID := chi.URLParam(r, "id")

	var req struct {
		Feedback string `json:"feedback"`
		By       string `json:"acknowledged_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.detector.AcknowledgeAnomaly(anomalyID, req.Feedback, req.By); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	writeJSON(w, map[string]string{"status": "acknowledged"})
}

// handleGetSummary returns the weekly anomaly summary.
func (h *AnomalyHandler) handleGetSummary(w http.ResponseWriter, r *http.Request) {
	if h.detector == nil {
		http.Error(w, "anomaly detector not available", http.StatusServiceUnavailable)
		return
	}

	summary := h.detector.GetWeeklySummary()
	writeJSON(w, summary)
}

// handleGetLearningProgress returns the learning progress.
func (h *AnomalyHandler) handleGetLearningProgress(w http.ResponseWriter, r *http.Request) {
	if h.detector == nil {
		http.Error(w, "anomaly detector not available", http.StatusServiceUnavailable)
		return
	}

	progress := h.detector.GetLearningProgress()
	ready := h.detector.IsModelReady()

	response := map[string]interface{}{
		"progress":    progress,
		"model_ready": ready,
		"days_learned": int(progress * 7),
		"days_remaining": int((1 - progress) * 7),
	}
	writeJSON(w, response)
}

// handleUpdateModel triggers a behaviour model update.
func (h *AnomalyHandler) handleUpdateModel(w http.ResponseWriter, r *http.Request) {
	if h.detector == nil {
		http.Error(w, "anomaly detector not available", http.StatusServiceUnavailable)
		return
	}

	if err := h.detector.UpdateBehaviourModel(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"status": "updated"})
}

// PatternHandler provides REST API handlers for the Welford pattern learner.
type PatternHandler struct {
	learner *PatternLearner
}

// NewPatternHandler creates a new pattern handler.
func NewPatternHandler(learner *PatternLearner) *PatternHandler {
	return &PatternHandler{learner: learner}
}

// RegisterRoutes registers pattern API routes on the given router.
func (h *PatternHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/anomaly_patterns", h.handleGetPatterns)
}

// handleGetPatterns returns pattern model data for debugging.
// Query params:
//   - zone: filter by zone_id (string). If omitted, returns all patterns.
func (h *PatternHandler) handleGetPatterns(w http.ResponseWriter, r *http.Request) {
	if h.learner == nil {
		http.Error(w, "pattern learner not available", http.StatusServiceUnavailable)
		return
	}

	zoneID := r.URL.Query().Get("zone")

	patterns := h.learner.GetPatterns(zoneID)

	response := map[string]interface{}{
		"cold_start": h.learner.IsColdStart(),
		"patterns":   patterns,
		"count":      len(patterns),
	}
	writeJSON(w, response)
}
