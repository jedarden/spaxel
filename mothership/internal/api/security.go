// Package api provides REST API handlers for Spaxel security mode.
package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
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
	CountAnomaliesSince(since time.Time) (int, error)
	GetSystemMode() events.SystemMode
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
	r.Get("/api/mode", h.handleGetMode)
	r.Post("/api/mode", h.handleSetMode)
}

// SecurityStatus represents the current security mode state.
type SecurityStatus struct {
	Armed           bool   `json:"armed"`
	Mode            string `json:"mode,omitempty"`           // "armed", "armed_stay", or "disarmed"
	LearningUntil   string `json:"learning_until,omitempty"` // ISO8601 when model will be ready, empty if ready
	AnomalyCount24h int    `json:"anomaly_count_24h"`
	ModelReady      bool   `json:"model_ready"`
}

// SystemModeResponse represents the current system mode response.
type SystemModeResponse struct {
	Mode            string    `json:"mode"`            // "home", "away", "sleep"
	Armed           bool      `json:"armed"`
	LearningUntil   string    `json:"learning_until,omitempty"`
	AnomalyCount24h int       `json:"anomaly_count_24h"`
	ModelReady      bool      `json:"model_ready"`
	LastChange      string    `json:"last_change,omitempty"`
	LastChangeBy    string    `json:"last_change_by,omitempty"`
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

	count, err := h.detector.CountAnomaliesSince(time.Now().Add(-24 * time.Hour))
	if err != nil {
		return 0
	}
	return count
}

// handleGetMode returns the current system mode (home/away/sleep).
// Response JSON:
// {
//   "mode": "home",
//   "armed": false,
//   "learning_until": "2024-04-15T10:30:00Z",  // omitted if model_ready
//   "anomaly_count_24h": 5,
//   "model_ready": false,
//   "last_change": "2024-04-15T10:30:00Z",
//   "last_change_by": "auto_away"
// }
func (h *SecurityHandler) handleGetMode(w http.ResponseWriter, r *http.Request) {
	if h.detector == nil {
		http.Error(w, "detector not available", http.StatusServiceUnavailable)
		return
	}

	mode := h.detector.GetSystemMode()
	armed := h.detector.IsSecurityModeActive()
	modelReady := h.detector.IsModelReady()
	progress := h.detector.GetLearningProgress()

	response := SystemModeResponse{
		Mode:            string(mode),
		Armed:           armed,
		ModelReady:      modelReady,
		AnomalyCount24h: h.countAnomalies24h(),
	}

	// Calculate learning_until if model is not ready
	if !modelReady {
		elapsed := time.Duration(float64(7*24*time.Hour) * progress)
		remaining := 7*24*time.Hour - elapsed
		learningUntil := time.Now().Add(remaining)
		response.LearningUntil = learningUntil.Format(time.RFC3339)
	}

	writeJSON(w, http.StatusOK, response)
}

// handleSetMode sets the system mode (home/away/sleep).
// Request body:
// {
//   "mode": "away",  // "home", "away", or "sleep"
//   "reason": "manual"  // optional reason for logging
// }
// Response: SystemModeResponse
func (h *SecurityHandler) handleSetMode(w http.ResponseWriter, r *http.Request) {
	if h.detector == nil {
		http.Error(w, "detector not available", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Mode   string `json:"mode"`   // "home", "away", or "sleep"
		Reason string `json:"reason"` // optional reason
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Default reason
	if req.Reason == "" {
		req.Reason = "api"
	}

	var securityMode analytics.SecurityMode
	switch req.Mode {
	case "away":
		securityMode = analytics.SecurityModeArmed
	case "sleep":
		securityMode = analytics.SecurityModeArmedStay
	case "home", "":
		securityMode = analytics.SecurityModeDisarmed
	default:
		http.Error(w, "invalid mode: must be 'home', 'away', or 'sleep'", http.StatusBadRequest)
		return
	}

	h.detector.SetSecurityMode(securityMode, req.Reason)

	// Return updated status
	mode := h.detector.GetSystemMode()
	armed := h.detector.IsSecurityModeActive()
	modelReady := h.detector.IsModelReady()

	response := SystemModeResponse{
		Mode:            string(mode),
		Armed:           armed,
		ModelReady:      modelReady,
		AnomalyCount24h: h.countAnomalies24h(),
		LastChange:      time.Now().Format(time.RFC3339),
		LastChangeBy:    req.Reason,
	}

	writeJSON(w, http.StatusOK, response)
}
