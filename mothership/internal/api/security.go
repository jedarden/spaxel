// Package api provides REST API handlers for Spaxel security mode.
package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi"
	"github.com/spaxel/mothership/internal/analytics"
	"github.com/spaxel/mothership/internal/events"
)

// SecurityHandler manages security mode state and API endpoints.
type SecurityHandler struct {
	detector DetectorProvider
}

// DetectorProvider is an interface to access the anomaly detector.
type DetectorProvider interface {
	GetSecurityMode() analytics.SecurityMode
	SetSecurityMode(mode analytics.SecurityMode, reason string)
	IsSecurityModeActive() bool
	GetLearningProgress() float64
	IsModelReady() bool
	GetActiveAnomalies() []*events.AnomalyEvent
	GetAnomalyHistory(limit int) []*events.AnomalyEvent
}

// NewSecurityHandler creates a new security handler.
func NewSecurityHandler(detector DetectorProvider) *SecurityHandler {
	return &SecurityHandler{
		detector: detector,
	}
}

// RegisterRoutes registers security API routes on the given router.
func (h *SecurityHandler) RegisterRoutes(r chi.Router) {
	r.Post("/api/security/arm", h.handleArm)
	r.Post("/api/security/disarm", h.handleDisarm)
	r.Get("/api/security/status", h.handleStatus)
}

// SecurityStatus represents the current security mode state.
type SecurityStatus struct {
	Armed           bool   `json:"armed"`
	Mode            string `json:"mode,omitempty"`           // "armed", "armed_stay", or "disarmed"
	LearningUntil   string `json:"learning_until,omitempty"` // ISO8601 when model will be ready, empty if ready
	AnomalyCount24h int    `json:"anomaly_count_24h"`
	ModelReady      bool   `json:"model_ready"`
}

// handleStatus returns the current security mode status.
// Response JSON:
// {
//   "armed": true,
//   "mode": "armed",
//   "learning_until": "2024-04-15T10:30:00Z",  // omitted if model_ready
//   "anomaly_count_24h": 5,
//   "model_ready": false
// }
func (h *SecurityHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if h.detector == nil {
		http.Error(w, "detector not available", http.StatusServiceUnavailable)
		return
	}

	mode := h.detector.GetSecurityMode()
	armed := h.detector.IsSecurityModeActive()
	modelReady := h.detector.IsModelReady()
	progress := h.detector.GetLearningProgress()

	status := SecurityStatus{
		Armed:           armed,
		Mode:            string(mode),
		ModelReady:      modelReady,
		AnomalyCount24h: h.countAnomalies24h(),
	}

	// Calculate learning_until if model is not ready
	if !modelReady {
		// Get the learning start time by calculating from progress
		// progress = elapsed / (7 days)
		// elapsed = progress * 7 days
		// learning_until = start + 7 days = now + (7 days - elapsed)
		elapsed := time.Duration(float64(7*24*time.Hour) * progress)
		remaining := 7*24*time.Hour - elapsed
		learningUntil := time.Now().Add(remaining)
		status.LearningUntil = learningUntil.Format(time.RFC3339)
	}

	writeJSON(w, http.StatusOK, status)
}

// handleArm enables security mode.
// Request body (optional): {"mode": "armed"} or {"mode": "armed_stay"}
// Default mode is "armed" if not specified.
// Response: {"armed": true, "mode": "armed"}
func (h *SecurityHandler) handleArm(w http.ResponseWriter, r *http.Request) {
	if h.detector == nil {
		http.Error(w, "detector not available", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Mode string `json:"mode"` // "armed" or "armed_stay"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var mode analytics.SecurityMode
	switch req.Mode {
	case "armed_stay":
		mode = analytics.SecurityModeArmedStay
	case "armed", "":
		mode = analytics.SecurityModeArmed
	default:
		http.Error(w, "invalid mode: must be 'armed' or 'armed_stay'", http.StatusBadRequest)
		return
	}

	h.detector.SetSecurityMode(mode, "api")

	status := map[string]interface{}{
		"armed": true,
		"mode":  string(mode),
	}
	writeJSON(w, http.StatusOK, status)
}

// handleDisarm disables security mode.
// Response: {"armed": false, "mode": "disarmed"}
func (h *SecurityHandler) handleDisarm(w http.ResponseWriter, r *http.Request) {
	if h.detector == nil {
		http.Error(w, "detector not available", http.StatusServiceUnavailable)
		return
	}

	h.detector.SetSecurityMode(analytics.SecurityModeDisarmed, "api")

	status := map[string]interface{}{
		"armed": false,
		"mode":  "disarmed",
	}
	writeJSON(w, http.StatusOK, status)
}

// countAnomalies24h counts anomalies detected in the last 24 hours.
func (h *SecurityHandler) countAnomalies24h() int {
	if h.detector == nil {
		return 0
	}

	history := h.detector.GetAnomalyHistory(1000) // Get enough history
	cutoff := time.Now().Add(-24 * time.Hour)

	count := 0
	for _, event := range history {
		if event.Timestamp.After(cutoff) {
			count++
		}
	}

	return count
}
