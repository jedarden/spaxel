// Package api provides REST API handlers for presence prediction.
package api

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/spaxel/mothership/internal/prediction"
)

// PredictionHandler manages prediction API endpoints.
type PredictionHandler struct {
	predictor       *prediction.Predictor
	history         *prediction.HistoryUpdater
	accuracyTracker *prediction.AccuracyTracker
	horizonPredictor *prediction.HorizonPredictor
	zoneProvider    ZoneProvider
	personProvider  PersonProvider
}

// ZoneProvider provides zone information.
type ZoneProvider interface {
	GetZone(id string) (name string, ok bool)
}

// PersonProvider provides person information.
type PersonProvider interface {
	GetPeople() ([]struct {
		ID   string
		Name string
	}, error)
}

// NewPredictionHandler creates a new prediction API handler.
func NewPredictionHandler(pred *prediction.Predictor, history *prediction.HistoryUpdater, accuracy *prediction.AccuracyTracker, horizon *prediction.HorizonPredictor) *PredictionHandler {
	return &PredictionHandler{
		predictor:        pred,
		history:          history,
		accuracyTracker:  accuracy,
		horizonPredictor: horizon,
	}
}

// SetZoneProvider sets the zone provider.
func (h *PredictionHandler) SetZoneProvider(zp ZoneProvider) {
	h.zoneProvider = zp
}

// SetPersonProvider sets the person provider.
func (h *PredictionHandler) SetPersonProvider(pp PersonProvider) {
	h.personProvider = pp
}

// RegisterRoutes registers prediction endpoints.
func (h *PredictionHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/predictions", h.getPredictions)
	r.Get("/api/predictions/stats", h.getStats)
	r.Post("/api/predictions/recompute", h.recompute)

	// Accuracy endpoints
	if h.accuracyTracker != nil {
		r.Get("/api/predictions/accuracy", h.getAccuracyAll)
		r.Get("/api/predictions/accuracy/overall", h.getAccuracyOverall)
		r.Get("/api/predictions/accuracy/{personID}", h.getAccuracyPerson)
		r.Get("/api/predictions/pending", h.getPending)

		// Zone occupancy patterns
		r.Get("/api/predictions/patterns/zones", h.getZonePatterns)
		r.Get("/api/predictions/patterns/zones/{zoneID}", h.getZonePattern)
		r.Post("/api/predictions/patterns/compute", h.computePatterns)
	}

	// Horizon prediction endpoints (Monte Carlo)
	if h.horizonPredictor != nil {
		r.Get("/api/predictions/horizon", h.getHorizonPredictions)
		r.Get("/api/predictions/horizon/{personID}", h.getHorizonPrediction)
	}
}

// getPredictions handles GET /api/predictions
func (h *PredictionHandler) getPredictions(w http.ResponseWriter, r *http.Request) {
	if h.predictor == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "prediction service not available")
		return
	}

	// Parse query parameters
	personID := r.URL.Query().Get("person")
	horizonStr := r.URL.Query().Get("horizon")

	predictions := h.predictor.GetPredictions()

	// Filter by person if requested
	if personID != "" {
		filtered := make([]prediction.PersonPrediction, 0)
		for _, p := range predictions {
			if p.PersonID == personID {
				filtered = append(filtered, p)
			}
		}
		predictions = filtered
	}

	// Filter by horizon if specified
	if horizonStr != "" && h.horizonPredictor != nil {
		horizonMin, err := strconv.Atoi(horizonStr)
		if err == nil {
			// Get horizon predictions at the specified horizon
			horizon := time.Duration(horizonMin) * time.Minute

			// Get current positions
			positions := h.predictor.GetPredictions()
			horizonPredictions := make([]prediction.HorizonPrediction, 0)

			for _, pos := range positions {
				hp := h.horizonPredictor.PredictAtHorizon(pos.PersonID, pos.CurrentZoneID, horizon)
				horizonPredictions = append(horizonPredictions, *hp)
			}

			writeJSON(w, http.StatusOK, map[string]interface{}{
				"horizon_minutes": horizonMin,
				"predictions":     horizonPredictions,
			})
			return
		}
	}

	writeJSON(w, http.StatusOK, predictions)
}

// getStats handles GET /api/predictions/stats
func (h *PredictionHandler) getStats(w http.ResponseWriter, r *http.Request) {
	if h.history == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "prediction history not available")
		return
	}

	count, dataAge, err := h.history.GetTransitionStats()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"transition_count":  count,
		"data_age_days":     dataAge.Hours() / 24,
		"minimum_data_age":  prediction.MinimumDataAge.Hours() / 24,
		"has_minimum_data": dataAge >= prediction.MinimumDataAge,
	})
}

// recompute handles POST /api/predictions/recompute
func (h *PredictionHandler) recompute(w http.ResponseWriter, r *http.Request) {
	if h.history == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "prediction history not available")
		return
	}

	if err := h.history.ForceRecompute(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "recompute_started"})
}

// getAccuracyAll handles GET /api/predictions/accuracy
func (h *PredictionHandler) getAccuracyAll(w http.ResponseWriter, r *http.Request) {
	if h.accuracyTracker == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "accuracy tracker not available")
		return
	}

	stats, err := h.accuracyTracker.GetAllAccuracyStats()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// getAccuracyOverall handles GET /api/predictions/accuracy/overall
func (h *PredictionHandler) getAccuracyOverall(w http.ResponseWriter, r *http.Request) {
	if h.accuracyTracker == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "accuracy tracker not available")
		return
	}

	accuracy, total, err := h.accuracyTracker.GetOverallAccuracy()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	pending := h.accuracyTracker.GetPendingCount()
	horizon := int(prediction.PredictionHorizon.Minutes())

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"accuracy_percent":    accuracy * 100,
		"total_predictions":   total,
		"pending_predictions": pending,
		"target_accuracy":     75.0,
		"meets_target":        accuracy >= 0.75 && total >= prediction.MinPredictionsForAccuracy,
		"horizon_minutes":     horizon,
	})
}

// getAccuracyPerson handles GET /api/predictions/accuracy/{personID}
func (h *PredictionHandler) getAccuracyPerson(w http.ResponseWriter, r *http.Request) {
	if h.accuracyTracker == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "accuracy tracker not available")
		return
	}

	personID := chi.URLParam(r, "personID")
	stats, err := h.accuracyTracker.GetAccuracyStats(personID, int(prediction.PredictionHorizon.Minutes()))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if stats == nil {
		writeJSONError(w, http.StatusNotFound, "no accuracy data for person")
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// getPending handles GET /api/predictions/pending
func (h *PredictionHandler) getPending(w http.ResponseWriter, r *http.Request) {
	if h.accuracyTracker == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "accuracy tracker not available")
		return
	}

	pending := h.accuracyTracker.GetPendingCount()
	writeJSON(w, http.StatusOK, map[string]int{"pending_predictions": pending})
}

// getZonePatterns handles GET /api/predictions/patterns/zones
func (h *PredictionHandler) getZonePatterns(w http.ResponseWriter, r *http.Request) {
	if h.accuracyTracker == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "accuracy tracker not available")
		return
	}

	if h.zoneProvider == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "zone provider not available")
		return
	}

	// Get all zones - this will come from the zones manager
	// For now, return empty patterns
	patterns := make([]map[string]interface{}, 0)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"patterns": patterns,
	})
}

// getZonePattern handles GET /api/predictions/patterns/zones/{zoneID}
func (h *PredictionHandler) getZonePattern(w http.ResponseWriter, r *http.Request) {
	if h.accuracyTracker == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "accuracy tracker not available")
		return
	}

	zoneID := chi.URLParam(r, "zoneID")
	hourOfWeek := prediction.HourOfWeek(time.Now())

	pattern, err := h.accuracyTracker.GetZoneOccupancyPattern(zoneID, hourOfWeek)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if pattern == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"zone_id":     zoneID,
			"hour_of_week": hourOfWeek,
			"message":     "no pattern data for this zone/time",
		})
		return
	}

	writeJSON(w, http.StatusOK, pattern)
}

// computePatterns handles POST /api/predictions/patterns/compute
func (h *PredictionHandler) computePatterns(w http.ResponseWriter, r *http.Request) {
	if h.accuracyTracker == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "accuracy tracker not available")
		return
	}

	if err := h.accuracyTracker.ComputeZoneOccupancyPatterns(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "patterns_computed"})
}

// getHorizonPredictions handles GET /api/predictions/horizon
func (h *PredictionHandler) getHorizonPredictions(w http.ResponseWriter, r *http.Request) {
	if h.horizonPredictor == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "horizon predictor not available")
		return
	}

	// Parse horizon parameter (default 15 minutes)
	horizonStr := r.URL.Query().Get("horizon")
	horizonMin := 15
	if horizonStr != "" {
		if n, err := strconv.Atoi(horizonStr); err == nil {
			horizonMin = n
		}
	}

	_ = time.Duration(horizonMin) * time.Minute // horizon variable (unused but kept for context)
	predictions := h.horizonPredictor.UpdateAllPredictions()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"horizon_minutes": horizonMin,
		"predictions":     predictions,
	})
}

// getHorizonPrediction handles GET /api/predictions/horizon/{personID}
func (h *PredictionHandler) getHorizonPrediction(w http.ResponseWriter, r *http.Request) {
	if h.horizonPredictor == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "horizon predictor not available")
		return
	}

	personID := chi.URLParam(r, "personID")

	// Parse horizon parameter (default 15 minutes)
	horizonStr := r.URL.Query().Get("horizon")
	horizonMin := 15
	if horizonStr != "" {
		if n, err := strconv.Atoi(horizonStr); err == nil {
			horizonMin = n
		}
	}

	// Get current position for this person
	predictions := h.predictor.GetPredictions()
	var currentZone string
	for _, p := range predictions {
		if p.PersonID == personID {
			currentZone = p.CurrentZoneID
			break
		}
	}

	if currentZone == "" {
		writeJSONError(w, http.StatusNotFound, "person not found or no current zone")
		return
	}

	horizon := time.Duration(horizonMin) * time.Minute
	prediction := h.horizonPredictor.PredictAtHorizon(personID, currentZone, horizon)

	writeJSON(w, http.StatusOK, prediction)
}

// LogPredictionAccuracy logs the current prediction accuracy for monitoring.
func LogPredictionAccuracy(tracker *prediction.AccuracyTracker) {
	if tracker == nil {
		return
	}

	accuracy, total, err := tracker.GetOverallAccuracy()
	if err != nil {
		log.Printf("[WARN] prediction: failed to get overall accuracy: %v", err)
		return
	}

	if total > 0 {
		log.Printf("[INFO] prediction: overall accuracy %.1f%% (%d predictions, target: 75%%)",
			accuracy*100, total)
	}

	// Log per-person accuracy
	stats, err := tracker.GetAllAccuracyStats()
	if err != nil {
		return
	}

	for _, stat := range stats {
		if stat.TotalPredictions > 0 {
			meetsTarget := "✓"
			if !stat.MeetsTarget {
				meetsTarget = "✗"
			}
			log.Printf("[INFO] prediction: %s accuracy %.1f%% (%d predictions) %s",
				stat.PersonID, stat.Accuracy*100, stat.TotalPredictions, meetsTarget)
		}
	}
}
