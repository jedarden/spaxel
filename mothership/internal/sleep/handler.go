// Package sleep provides REST API handlers for sleep quality monitoring.
package sleep

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi"
)

// Handler provides REST API handlers for the sleep module.
type Handler struct {
	monitor *Monitor
}

// NewHandler creates a new sleep handler.
func NewHandler(monitor *Monitor) *Handler {
	return &Handler{
		monitor: monitor,
	}
}

// RegisterRoutes registers the sleep API routes on the provided router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/sleep/status", h.handleGetStatus)
	r.Get("/api/sleep/reports", h.handleGetReports)
	r.Get("/api/sleep/reports/{linkID}", h.handleGetReport)
	r.Post("/api/sleep/reports/generate", h.handleGenerateReports)
	r.Get("/api/sleep/sessions", h.handleGetSessions)
	r.Get("/api/sleep/sessions/{linkID}", h.handleGetSession)
	r.Get("/api/sleep/sessions/{linkID}/samples", h.handleGetSamples)
}

// handleGetStatus returns the current sleep monitoring status.
func (h *Handler) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	if h.monitor == nil {
		http.Error(w, "sleep monitor not available", http.StatusServiceUnavailable)
		return
	}

	status := h.monitor.GetStatus()
	writeJSON(w, status)
}

// handleGetReports returns all available sleep reports.
func (h *Handler) handleGetReports(w http.ResponseWriter, r *http.Request) {
	if h.monitor == nil {
		http.Error(w, "sleep monitor not available", http.StatusServiceUnavailable)
		return
	}

	reports := h.monitor.ForceReportGeneration()

	// Convert to JSON-serializable format
	result := make(map[string]interface{})
	for linkID, report := range reports {
		result[linkID] = report.ToJSONMap()
	}

	writeJSON(w, result)
}

// handleGetReport returns the sleep report for a specific link.
func (h *Handler) handleGetReport(w http.ResponseWriter, r *http.Request) {
	if h.monitor == nil {
		http.Error(w, "sleep monitor not available", http.StatusServiceUnavailable)
		return
	}

	linkID := chi.URLParam(r, "linkID")
	if linkID == "" {
		http.Error(w, "link_id required", http.StatusBadRequest)
		return
	}

	report := h.monitor.GetSleepReport(linkID)
	if report == nil {
		http.Error(w, "no sleep report available for this link", http.StatusNotFound)
		return
	}

	writeJSON(w, report.ToJSONMap())
}

// handleGenerateReports forces generation of morning reports.
func (h *Handler) handleGenerateReports(w http.ResponseWriter, r *http.Request) {
	if h.monitor == nil {
		http.Error(w, "sleep monitor not available", http.StatusServiceUnavailable)
		return
	}

	reports := h.monitor.ForceReportGeneration()

	// Return summary of generated reports
	summaries := make([]map[string]interface{}, 0, len(reports))
	for linkID, report := range reports {
		summaries = append(summaries, map[string]interface{}{
			"link_id":        linkID,
			"overall_score":  report.Metrics.OverallScore,
			"quality_rating": report.Metrics.QualityRating,
			"generated_at":   report.GeneratedAt.Unix(),
		})
	}

	writeJSON(w, map[string]interface{}{
		"generated": len(reports),
		"reports":   summaries,
		"timestamp": time.Now().Unix(),
	})
}

// handleGetSessions returns all active sleep sessions.
func (h *Handler) handleGetSessions(w http.ResponseWriter, r *http.Request) {
	if h.monitor == nil {
		http.Error(w, "sleep monitor not available", http.StatusServiceUnavailable)
		return
	}

	sessions := h.monitor.GetAllSessions()

	// Convert sessions to summary format
	result := make([]map[string]interface{}, 0, len(sessions))
	for linkID, session := range sessions {
		session.mu.RLock()
		summary := map[string]interface{}{
			"link_id":          linkID,
			"current_state":    session.GetCurrentState().String(),
			"is_active":        session.isActive,
			"breathing_samples": len(session.breathingSamples),
			"motion_samples":   len(session.motionSamples),
			"sleep_periods":    len(session.sleepPeriods),
		}

		if !session.sessionDate.IsZero() {
			summary["session_date"] = session.sessionDate.Format("2006-01-02")
		}

		// Add latest sample info
		if len(session.breathingSamples) > 0 {
			lastBreath := session.breathingSamples[len(session.breathingSamples)-1]
			summary["last_breathing_rate"] = lastBreath.RateBPM
			summary["last_breathing_time"] = lastBreath.Timestamp.Unix()
		}
		if len(session.motionSamples) > 0 {
			lastMotion := session.motionSamples[len(session.motionSamples)-1]
			summary["last_motion_detected"] = lastMotion.MotionDetected
			summary["last_motion_time"] = lastMotion.Timestamp.Unix()
		}

		session.mu.RUnlock()
		result = append(result, summary)
	}

	writeJSON(w, result)
}

// handleGetSession returns details for a specific sleep session.
func (h *Handler) handleGetSession(w http.ResponseWriter, r *http.Request) {
	if h.monitor == nil {
		http.Error(w, "sleep monitor not available", http.StatusServiceUnavailable)
		return
	}

	linkID := chi.URLParam(r, "linkID")
	if linkID == "" {
		http.Error(w, "link_id required", http.StatusBadRequest)
		return
	}

	session := h.monitor.GetAnalyzer().GetSession(linkID)
	if session == nil {
		http.Error(w, "no sleep session found for this link", http.StatusNotFound)
		return
	}

	// Get metrics first (acquires its own lock) to avoid deadlock
	var metrics *SleepMetrics
	if len(session.GetBreathingSamples()) > 0 || len(session.GetMotionSamples()) > 0 {
		metrics = session.GetMetrics()
	}

	session.mu.RLock()
	defer session.mu.RUnlock()

	result := map[string]interface{}{
		"link_id":           linkID,
		"current_state":     session.currentState.String(),
		"is_active":         session.isActive,
		"sleep_start_hour":  session.sleepStartHour,
		"sleep_end_hour":    session.sleepEndHour,
		"breathing_samples": len(session.breathingSamples),
		"motion_samples":    len(session.motionSamples),
		"sleep_periods":     len(session.sleepPeriods),
	}

	if !session.sessionDate.IsZero() {
		result["session_date"] = session.sessionDate.Format("2006-01-02")
	}

	// Include current period if active
	if session.currentPeriod != nil {
		result["current_period"] = map[string]interface{}{
			"state":      session.currentPeriod.State.String(),
			"start_time": session.currentPeriod.StartTime.Unix(),
		}
	}

	// Include live metrics if available
	if metrics != nil {
		metricsMap := map[string]interface{}{
			"total_duration_hours":     metrics.TotalDuration.Hours(),
			"time_in_bed_hours":        metrics.TimeInBed.Hours(),
			"avg_breathing_rate":       metrics.AvgBreathingRate,
			"breathing_rate_std_dev":   metrics.BreathingRateStdDev,
			"breathing_regularity":     metrics.BreathingRegularity,
			"breathing_score":          metrics.BreathingScore,
			"breathing_anomaly":        metrics.BreathingAnomaly,
			"breathing_anomaly_count":  metrics.BreathingAnomalyCount,
			"quiet_time_pct":           metrics.QuietTimePct,
			"motion_events":            metrics.MotionEvents,
			"restless_periods":         metrics.RestlessPeriods,
			"motion_score":             metrics.MotionScore,
			"interruptions":            metrics.Interruptions,
			"longest_deep_period_mins": metrics.LongestDeepPeriod.Minutes(),
			"continuity_score":         metrics.ContinuityScore,
			"overall_score":            metrics.OverallScore,
			"quality_rating":           metrics.QualityRating,
		}

		if metrics.PersonalAvgBPM > 0 {
			metricsMap["personal_avg_bpm"] = metrics.PersonalAvgBPM
		}

		if !metrics.SleepStartTime.IsZero() {
			metricsMap["sleep_start_time"] = metrics.SleepStartTime.Format("15:04")
		}
		if !metrics.SleepEndTime.IsZero() {
			metricsMap["sleep_end_time"] = metrics.SleepEndTime.Format("15:04")
		}

		result["metrics"] = metricsMap
	}

	writeJSON(w, result)
}

// handleGetSamples returns the raw samples for a specific session.
func (h *Handler) handleGetSamples(w http.ResponseWriter, r *http.Request) {
	if h.monitor == nil {
		http.Error(w, "sleep monitor not available", http.StatusServiceUnavailable)
		return
	}

	linkID := chi.URLParam(r, "linkID")
	if linkID == "" {
		http.Error(w, "link_id required", http.StatusBadRequest)
		return
	}

	session := h.monitor.GetAnalyzer().GetSession(linkID)
	if session == nil {
		http.Error(w, "no sleep session found for this link", http.StatusNotFound)
		return
	}

	// Get sample type from query params (breathing, motion, or all)
	sampleType := r.URL.Query().Get("type")
	if sampleType == "" {
		sampleType = "all"
	}

	result := map[string]interface{}{
		"link_id": linkID,
	}

	switch sampleType {
	case "breathing":
		result["breathing_samples"] = session.GetBreathingSamples()
	case "motion":
		result["motion_samples"] = session.GetMotionSamples()
	default:
		result["breathing_samples"] = session.GetBreathingSamples()
		result["motion_samples"] = session.GetMotionSamples()
	}

	// Add sleep periods
	session.mu.RLock()
	periods := make([]map[string]interface{}, len(session.sleepPeriods))
	for i, p := range session.sleepPeriods {
		period := map[string]interface{}{
			"state":     p.State.String(),
			"start_time": p.StartTime.Unix(),
			"duration_seconds": p.Duration.Seconds(),
		}
		if !p.EndTime.IsZero() {
			period["end_time"] = p.EndTime.Unix()
		}
		periods[i] = period
	}
	session.mu.RUnlock()

	result["sleep_periods"] = periods

	writeJSON(w, result)
}

// writeJSON is a helper to write JSON responses.
func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
