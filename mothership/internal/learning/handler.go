// Package learning provides REST API handlers for feedback and accuracy
package learning

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// SpatialWeightProvider provides access to spatial weight learner stats
type SpatialWeightProvider interface {
	GetAllWeights() []interface{}
	GetWeightStats() map[string]interface{}
}

// PositionAccuracyProvider provides access to position accuracy data
type PositionAccuracyProvider interface {
	GetPositionAccuracyHistory(weeks int) ([]interface{}, error)
	GetPositionImprovementStats() (map[string]interface{}, error)
	GetTotalSampleCount() (int, error)
	GetSampleCountByPerson() (map[string]int, error)
	GetSamplesTodayCount() (int, error)
}

// Handler provides REST API handlers for the learning package
type Handler struct {
	store               *FeedbackStore
	processor           *Processor
	accuracyComp        *AccuracyComputer
	spatialWeightProv   SpatialWeightProvider
	positionAccuracyProv PositionAccuracyProvider
}

// NewHandler creates a new learning handler
func NewHandler(store *FeedbackStore, processor *Processor, accuracyComp *AccuracyComputer) *Handler {
	return &Handler{
		store:        store,
		processor:    processor,
		accuracyComp: accuracyComp,
	}
}

// SetSpatialWeightProvider sets the spatial weight provider for weight debug endpoints
func (h *Handler) SetSpatialWeightProvider(p SpatialWeightProvider) {
	h.spatialWeightProv = p
}

// SetPositionAccuracyProvider sets the position accuracy provider
func (h *Handler) SetPositionAccuracyProvider(p PositionAccuracyProvider) {
	h.positionAccuracyProv = p
}

// RegisterRoutes registers learning API routes on the given router
func (h *Handler) RegisterRoutes(r chi.Router) {
	// Feedback submission and retrieval
	r.Post("/api/learning/feedback", h.handleSubmitFeedback)
	r.Get("/api/learning/feedback", h.handleGetFeedback)
	r.Get("/api/learning/feedback/{eventID}", h.handleGetFeedbackByEvent)
	r.Get("/api/learning/stats", h.handleGetStats)

	// Accuracy metrics
	r.Get("/api/learning/accuracy", h.handleGetAccuracy)
	r.Get("/api/learning/accuracy/history", h.handleGetAccuracyHistory)
	r.Get("/api/learning/accuracy/improvement", h.handleGetImprovement)

	// Position accuracy (from ground truth samples)
	r.Get("/api/accuracy/position", h.handleGetPositionAccuracy)
	r.Get("/api/accuracy/position/history", h.handleGetPositionAccuracyHistory)

	// Weight debug endpoint
	r.Get("/api/accuracy/weights", h.handleGetWeights)

	// Manual processing trigger (for testing/admin)
	r.Post("/api/learning/process", h.handleTriggerProcess)
}

// Feedback submission request
type submitFeedbackRequest struct {
	EventID      string                 `json:"event_id"`
	EventType    string                 `json:"event_type"`
	FeedbackType string                 `json:"feedback_type"`
	Details      map[string]interface{} `json:"details"`
}

// handleSubmitFeedback handles POST /api/learning/feedback
func (h *Handler) handleSubmitFeedback(w http.ResponseWriter, r *http.Request) {
	var req submitFeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Validate feedback type
	validTypes := map[string]bool{
		string(TruePositive):   true,
		string(FalsePositive):  true,
		string(FalseNegative):  true,
		string(WrongIdentity):  true,
		string(WrongZone):      true,
	}
	if !validTypes[req.FeedbackType] {
		http.Error(w, "invalid feedback_type", http.StatusBadRequest)
		return
	}

	// Validate event type
	validEventTypes := map[string]bool{
		string(BlobDetection):   true,
		string(ZoneTransition):  true,
		string(FallAlert):       true,
		string(Anomaly):         true,
	}
	if !validEventTypes[req.EventType] {
		http.Error(w, "invalid event_type", http.StatusBadRequest)
		return
	}

	// Create feedback record
	feedback := FeedbackRecord{
		ID:           uuid.New().String(),
		EventID:      req.EventID,
		EventType:    EventType(req.EventType),
		FeedbackType: FeedbackType(req.FeedbackType),
		Details:      req.Details,
		Timestamp:    time.Now(),
		Applied:      false,
	}

	if feedback.Details == nil {
		feedback.Details = make(map[string]interface{})
	}

	// Store feedback
	if err := h.store.RecordFeedback(feedback); err != nil {
		http.Error(w, "failed to record feedback", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{
		"id":      feedback.ID,
		"success": true,
	})
}

// handleGetFeedback handles GET /api/learning/feedback
func (h *Handler) handleGetFeedback(w http.ResponseWriter, r *http.Request) {
	// Get pagination params
	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}

	// Get unprocessed param
	unprocessedOnly := r.URL.Query().Get("unprocessed") == "true"

	var feedbacks []FeedbackRecord
	var err error

	if unprocessedOnly {
		feedbacks, err = h.store.GetUnprocessedFeedback()
		if limit > 0 && len(feedbacks) > limit {
			feedbacks = feedbacks[:limit]
		}
	} else {
		// Would need to implement a general query method
		// For now, return stats instead
		stats, err := h.store.GetFeedbackStats()
		if err != nil {
			http.Error(w, "failed to get feedback", http.StatusInternalServerError)
			return
		}
		writeJSON(w, stats)
		return
	}

	if err != nil {
		http.Error(w, "failed to get feedback", http.StatusInternalServerError)
		return
	}

	writeJSON(w, feedbacks)
}

// handleGetFeedbackByEvent handles GET /api/learning/feedback/{eventID}
func (h *Handler) handleGetFeedbackByEvent(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "eventID")
	if eventID == "" {
		http.Error(w, "event_id required", http.StatusBadRequest)
		return
	}

	feedbacks, err := h.store.GetFeedbackByEvent(eventID)
	if err != nil {
		http.Error(w, "failed to get feedback", http.StatusInternalServerError)
		return
	}

	writeJSON(w, feedbacks)
}

// handleGetStats handles GET /api/learning/stats
func (h *Handler) handleGetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.store.GetFeedbackStats()
	if err != nil {
		http.Error(w, "failed to get stats", http.StatusInternalServerError)
		return
	}

	// Get total feedback count
	count, err := h.store.GetFeedbackCount()
	if err != nil {
		http.Error(w, "failed to get feedback count", http.StatusInternalServerError)
		return
	}
	stats["total_count"] = count

	writeJSON(w, stats)
}

// handleGetAccuracy handles GET /api/learning/accuracy
func (h *Handler) handleGetAccuracy(w http.ResponseWriter, r *http.Request) {
	scopeType := r.URL.Query().Get("scope_type")
	scopeID := r.URL.Query().Get("scope_id")

	// Default to system-wide accuracy
	if scopeType == "" {
		scopeType = ScopeTypeSystem
	}
	if scopeID == "" {
		scopeID = ScopeIDSystem
	}

	// Get current week's accuracy
	if h.accuracyComp != nil {
		record, err := h.accuracyComp.GetCurrentAccuracy(scopeType, scopeID)
		if err != nil {
			http.Error(w, "failed to get accuracy", http.StatusInternalServerError)
			return
		}
		if record != nil {
			writeJSON(w, record)
			return
		}
	}

	// Fallback: return from store
	records, err := h.store.GetAccuracyHistory(scopeType, scopeID, 1)
	if err != nil {
		http.Error(w, "failed to get accuracy", http.StatusInternalServerError)
		return
	}

	if len(records) == 0 {
		writeJSON(w, map[string]interface{}{
			"scope_type": scopeType,
			"scope_id":   scopeID,
			"f1":         nil,
			"precision":  nil,
			"recall":     nil,
			"message":    "no accuracy data available yet",
		})
		return
	}

	writeJSON(w, records[0])
}

// handleGetAccuracyHistory handles GET /api/learning/accuracy/history
func (h *Handler) handleGetAccuracyHistory(w http.ResponseWriter, r *http.Request) {
	scopeType := r.URL.Query().Get("scope_type")
	scopeID := r.URL.Query().Get("scope_id")
	weeksStr := r.URL.Query().Get("weeks")

	weeks := 8
	if weeksStr != "" {
		if n, err := strconv.Atoi(weeksStr); err == nil && n > 0 && n <= 52 {
			weeks = n
		}
	}

	// Default to system-wide
	if scopeType == "" {
		scopeType = ScopeTypeSystem
	}
	if scopeID == "" {
		scopeID = ScopeIDSystem
	}

	records, err := h.store.GetAccuracyHistory(scopeType, scopeID, weeks)
	if err != nil {
		http.Error(w, "failed to get accuracy history", http.StatusInternalServerError)
		return
	}

	writeJSON(w, records)
}

// handleGetImprovement handles GET /api/learning/accuracy/improvement
func (h *Handler) handleGetImprovement(w http.ResponseWriter, r *http.Request) {
	if h.accuracyComp == nil {
		http.Error(w, "accuracy computer not available", http.StatusServiceUnavailable)
		return
	}

	stats, err := h.accuracyComp.GetImprovementStats()
	if err != nil {
		http.Error(w, "failed to get improvement stats", http.StatusInternalServerError)
		return
	}

	writeJSON(w, stats)
}

// handleTriggerProcess handles POST /api/learning/process
func (h *Handler) handleTriggerProcess(w http.ResponseWriter, r *http.Request) {
	if h.processor == nil {
		http.Error(w, "processor not available", http.StatusServiceUnavailable)
		return
	}

	// Trigger immediate processing
	if err := h.processor.ProcessNow(); err != nil {
		http.Error(w, "processing failed", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{
		"status": "processed",
	})
}

// handleGetWeights handles GET /api/accuracy/weights
// Returns the current learned weight map for debugging
func (h *Handler) handleGetWeights(w http.ResponseWriter, r *http.Request) {
	if h.spatialWeightProv == nil {
		writeJSON(w, map[string]interface{}{
			"weights":   []interface{}{},
			"stats":     nil,
			"available": false,
			"message":   "spatial weight learner not available",
		})
		return
	}

	weights := h.spatialWeightProv.GetAllWeights()
	stats := h.spatialWeightProv.GetWeightStats()

	writeJSON(w, map[string]interface{}{
		"weights":   weights,
		"stats":     stats,
		"available": true,
	})
}

// handleGetPositionAccuracy handles GET /api/accuracy/position
// Returns current position accuracy from ground truth samples
func (h *Handler) handleGetPositionAccuracy(w http.ResponseWriter, r *http.Request) {
	if h.positionAccuracyProv == nil {
		writeJSON(w, map[string]interface{}{
			"available": false,
			"message":   "position accuracy tracking not available",
		})
		return
	}

	stats, err := h.positionAccuracyProv.GetPositionImprovementStats()
	if err != nil {
		http.Error(w, "failed to get position accuracy", http.StatusInternalServerError)
		return
	}

	// Get sample counts
	totalSamples, _ := h.positionAccuracyProv.GetTotalSampleCount()
	todaySamples, _ := h.positionAccuracyProv.GetSamplesTodayCount()
	byPerson, _ := h.positionAccuracyProv.GetSampleCountByPerson()

	stats["total_samples"] = totalSamples
	stats["today_samples"] = todaySamples
	stats["samples_by_person"] = byPerson
	stats["available"] = true

	writeJSON(w, stats)
}

// handleGetPositionAccuracyHistory handles GET /api/accuracy/position/history
// Returns weekly position accuracy history for sparkline chart
func (h *Handler) handleGetPositionAccuracyHistory(w http.ResponseWriter, r *http.Request) {
	weeksStr := r.URL.Query().Get("weeks")
	weeks := 8
	if weeksStr != "" {
		if n, err := strconv.Atoi(weeksStr); err == nil && n > 0 && n <= 52 {
			weeks = n
		}
	}

	if h.positionAccuracyProv == nil {
		writeJSON(w, []interface{}{})
		return
	}

	records, err := h.positionAccuracyProv.GetPositionAccuracyHistory(weeks)
	if err != nil {
		http.Error(w, "failed to get position accuracy history", http.StatusInternalServerError)
		return
	}

	writeJSON(w, records)
}

// writeJSON writes a JSON response
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
