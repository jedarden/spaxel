// Package api provides REST API handlers for self-improving localization.
package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/spaxel/mothership/internal/localization"
)

// LocalizationHandler manages self-improving localization API endpoints.
type LocalizationHandler struct {
	groundTruthStore   *localization.GroundTruthStore
	spatialWeightLearner *localization.SpatialWeightLearner
	weightLearner      *localization.WeightLearner
	weightStore        *localization.WeightStore
	selfImprovingLocalizer *localization.SelfImprovingLocalizer
}

// NewLocalizationHandler creates a new localization API handler.
func NewLocalizationHandler(
	gtStore *localization.GroundTruthStore,
	swLearner *localization.SpatialWeightLearner,
	wLearner *localization.WeightLearner,
	wStore *localization.WeightStore,
	sil *localization.SelfImprovingLocalizer,
) *LocalizationHandler {
	return &LocalizationHandler{
		groundTruthStore:   gtStore,
		spatialWeightLearner: swLearner,
		weightLearner:      wLearner,
		weightStore:        wStore,
		selfImprovingLocalizer: sil,
	}
}

// RegisterRoutes registers localization endpoints.
func (h *LocalizationHandler) RegisterRoutes(r chi.Router) {
	// Learned weights endpoints
	r.Get("/api/localization/weights", h.getWeights)
	r.Get("/api/localization/weights/{linkID}", h.getLinkWeight)
	r.Post("/api/localization/weights/reset", h.resetWeights)
	r.Get("/api/localization/weights/stats", h.getWeightStats)

	// Spatial weights endpoints
	r.Get("/api/localization/spatial-weights", h.getSpatialWeights)
	r.Get("/api/localization/spatial-weights/stats", h.getSpatialWeightStats)
	r.Get("/api/localization/spatial-weights/zone/{zoneX}/{zoneY}", h.getSpatialWeightsForZone)

	// Ground truth endpoints
	r.Get("/api/localization/groundtruth/samples", h.getGroundTruthSamples)
	r.Get("/api/localization/groundtruth/stats", h.getGroundTruthStats)
	r.Post("/api/localization/groundtruth/compute-accuracy", h.computeWeeklyAccuracy)

	// Accuracy and improvement endpoints
	r.Get("/api/localization/accuracy/history", h.getAccuracyHistory)
	r.Get("/api/localization/accuracy/current", h.getCurrentAccuracy)
	r.Get("/api/localization/accuracy/improvement", h.getImprovementStats)

	// Learning progress endpoints
	r.Get("/api/localization/learning/progress", h.getLearningProgress)
	r.Get("/api/localization/learning/history", h.getImprovementHistory)

	// Self-improving localizer endpoints
	r.Get("/api/localization/self-improving/status", h.getSelfImprovingStatus)
	r.Post("/api/localization/self-improving/process", h.processLearning)
}

// getWeights handles GET /api/localization/weights
func (h *LocalizationHandler) getWeights(w http.ResponseWriter, r *http.Request) {
	if h.weightLearner == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "weight learner not available")
		return
	}

	weights := h.weightLearner.GetLearnedWeights().GetAllWeights()
	stats := h.weightLearner.GetAllStats()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"weights": weights,
		"stats":   stats,
	})
}

// getLinkWeight handles GET /api/localization/weights/{linkID}
func (h *LocalizationHandler) getLinkWeight(w http.ResponseWriter, r *http.Request) {
	if h.weightLearner == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "weight learner not available")
		return
	}

	linkID := chi.URLParam(r, "linkID")
	weights := h.weightLearner.GetLearnedWeights()
	stats := h.weightLearner.GetLinkStats(linkID)

	result := map[string]interface{}{
		"link_id": linkID,
		"weight":  weights.GetLinkWeight(linkID),
		"sigma":   weights.GetLinkSigma(linkID),
	}

	if stats != nil {
		result["stats"] = stats
	}

	writeJSON(w, http.StatusOK, result)
}

// resetWeights handles POST /api/localization/weights/reset
func (h *LocalizationHandler) resetWeights(w http.ResponseWriter, r *http.Request) {
	if h.weightLearner == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "weight learner not available")
		return
	}

	// Reset all weights to default
	weights := h.weightLearner.GetLearnedWeights()
	weights.mu.Lock()
	weights.linkWeights = make(map[string]float64)
	weights.linkSigmas = make(map[string]float64)
	weights.linkStats = make(map[string]*localization.LinkLearningStats)
	weights.lastUpdate = time.Now()
	weights.mu.Unlock()

	// Persist reset
	if h.weightStore != nil {
		if err := h.weightStore.SaveWeights(weights); err != nil {
			log.Printf("[WARN] Failed to save reset weights: %v", err)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "weights_reset"})
}

// getWeightStats handles GET /api/localization/weights/stats
func (h *LocalizationHandler) getWeightStats(w http.ResponseWriter, r *http.Request) {
	if h.weightLearner == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "weight learner not available")
		return
	}

	stats := h.weightLearner.GetAllStats()
	progress := h.weightLearner.GetLearningProgress()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"stats":    stats,
		"progress": progress,
	})
}

// getSpatialWeights handles GET /api/localization/spatial-weights
func (h *LocalizationHandler) getSpatialWeights(w http.ResponseWriter, r *http.Request) {
	if h.spatialWeightLearner == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "spatial weight learner not available")
		return
	}

	weights := h.spatialWeightLearner.GetAllWeights()
	stats := h.spatialWeightLearner.GetWeightStats()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"spatial_weights": weights,
		"stats":           stats,
	})
}

// getSpatialWeightStats handles GET /api/localization/spatial-weights/stats
func (h *LocalizationHandler) getSpatialWeightStats(w http.ResponseWriter, r *http.Request) {
	if h.spatialWeightLearner == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "spatial weight learner not available")
		return
	}

	stats := h.spatialWeightLearner.GetWeightStats()

	writeJSON(w, http.StatusOK, stats)
}

// getSpatialWeightsForZone handles GET /api/localization/spatial-weights/zone/{zoneX}/{zoneY}
func (h *LocalizationHandler) getSpatialWeightsForZone(w http.ResponseWriter, r *http.Request) {
	if h.spatialWeightLearner == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "spatial weight learner not available")
		return
	}

	zoneX, err := strconv.Atoi(chi.URLParam(r, "zoneX"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid zoneX")
		return
	}

	zoneY, err := strconv.Atoi(chi.URLParam(r, "zoneY"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid zoneY")
		return
	}

	weights := h.spatialWeightLearner.GetWeightsForZone(zoneX, zoneY)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"zone_x":  zoneX,
		"zone_y":  zoneY,
		"weights": weights,
	})
}

// getGroundTruthSamples handles GET /api/localization/groundtruth/samples
func (h *LocalizationHandler) getGroundTruthSamples(w http.ResponseWriter, r *http.Request) {
	if h.groundTruthStore == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "ground truth store not available")
		return
	}

	// Parse query parameters
	personID := r.URL.Query().Get("person")
	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			limit = n
		}
	}

	var samples []localization.GroundTruthSample
	var err error

	if personID != "" {
		// Get samples for specific person
		samples, err = h.groundTruthStore.GetSamplesInTimeRange(
			time.Now().Add(-24*time.Hour), time.Now())
		// Filter by person
		filtered := make([]localization.GroundTruthSample, 0)
		for _, s := range samples {
			if s.PersonID == personID {
				filtered = append(filtered, s)
			}
		}
		samples = filtered
	} else {
		samples, err = h.groundTruthStore.GetRecentSamples(limit)
	}

	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"samples": samples,
		"count":   len(samples),
	})
}

// getGroundTruthStats handles GET /api/localization/groundtruth/stats
func (h *LocalizationHandler) getGroundTruthStats(w http.ResponseWriter, r *http.Request) {
	if h.groundTruthStore == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "ground truth store not available")
		return
	}

	total, err := h.groundTruthStore.GetTotalSampleCount()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	byPerson, err := h.groundTruthStore.GetSampleCountByPerson()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	today, err := h.groundTruthStore.GetSamplesTodayCount()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	zoneCounts, err := h.groundTruthStore.GetZoneSampleCounts()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_samples":  total,
		"today_samples":  today,
		"by_person":      byPerson,
		"zone_counts":    zoneCounts,
	})
}

// computeWeeklyAccuracy handles POST /api/localization/groundtruth/compute-accuracy
func (h *LocalizationHandler) computeWeeklyAccuracy(w http.ResponseWriter, r *http.Request) {
	if h.groundTruthStore == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "ground truth store not available")
		return
	}

	// Get current week
	week := localization.GetWeekString(time.Now())

	if err := h.groundTruthStore.ComputeWeeklyAccuracy(week); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "accuracy_computed",
		"week":   week,
	})
}

// getAccuracyHistory handles GET /api/localization/accuracy/history
func (h *LocalizationHandler) getAccuracyHistory(w http.ResponseWriter, r *http.Request) {
	if h.groundTruthStore == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "ground truth store not available")
		return
	}

	weeksStr := r.URL.Query().Get("weeks")
	weeks := 12 // Default 12 weeks
	if weeksStr != "" {
		if n, err := strconv.Atoi(weeksStr); err == nil && n > 0 {
			weeks = n
		}
	}

	records, err := h.groundTruthStore.GetPositionAccuracyHistory(weeks)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"records": records,
		"weeks":   weeks,
	})
}

// getCurrentAccuracy handles GET /api/localization/accuracy/current
func (h *LocalizationHandler) getCurrentAccuracy(w http.ResponseWriter, r *http.Request) {
	if h.groundTruthStore == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "ground truth store not available")
		return
	}

	current, err := h.groundTruthStore.GetCurrentPositionAccuracy()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if current == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"message": "no accuracy data for current week",
		})
		return
	}

	writeJSON(w, http.StatusOK, current)
}

// getImprovementStats handles GET /api/localization/accuracy/improvement
func (h *LocalizationHandler) getImprovementStats(w http.ResponseWriter, r *http.Request) {
	if h.groundTruthStore == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "ground truth store not available")
		return
	}

	stats, err := h.groundTruthStore.GetPositionImprovementStats()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// getLearningProgress handles GET /api/localization/learning/progress
func (h *LocalizationHandler) getLearningProgress(w http.ResponseWriter, r *http.Request) {
	if h.weightLearner == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "weight learner not available")
		return
	}

	progress := h.weightLearner.GetLearningProgress()
	stats := h.weightLearner.GetAllStats()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"progress": progress,
		"stats":    stats,
	})
}

// getImprovementHistory handles GET /api/localization/learning/history
func (h *LocalizationHandler) getImprovementHistory(w http.ResponseWriter, r *http.Request) {
	if h.weightLearner == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "weight learner not available")
		return
	}

	history := h.weightLearner.GetImprovementHistory()
	stats := h.weightLearner.GetImprovementStats()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"history": history,
		"stats":   stats,
	})
}

// getSelfImprovingStatus handles GET /api/localization/self-improving/status
func (h *LocalizationHandler) getSelfImprovingStatus(w http.ResponseWriter, r *http.Request) {
	if h.selfImprovingLocalizer == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "self-improving localizer not available")
		return
	}

	progress := h.selfImprovingLocalizer.GetLearningProgress()
	weights := h.selfImprovingLocalizer.GetLearnedWeights()
	improvementStats := h.selfImprovingLocalizer.GetImprovementStats()
	improvementHistory := h.selfImprovingLocalizer.GetImprovementHistory()
	gtStats, _ := h.selfImprovingLocalizer.GetGroundTruthProvider().GetObservationCount()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"learning_progress":       progress,
		"learned_weights":        weights,
		"improvement_stats":      improvementStats,
		"improvement_history":    improvementHistory,
		"ble_observations_count": gtStats,
	})
}

// processLearning handles POST /api/localization/self-improving/process
func (h *LocalizationHandler) processLearning(w http.ResponseWriter, r *http.Request) {
	if h.weightLearner == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "weight learner not available")
		return
	}

	if err := h.weightLearner.ProcessLearning(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Record error snapshot for improvement tracking
	h.weightLearner.RecordErrorSnapshot()

	writeJSON(w, http.StatusOK, map[string]string{
		"status":      "learning_processed",
		"timestamp":   time.Now().Format(time.RFC3339),
	})
}
