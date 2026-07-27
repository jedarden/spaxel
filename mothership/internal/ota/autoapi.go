// Package ota provides REST API handlers for OTA auto-update functionality.
package ota

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/spaxel/mothership/internal/events"
)

// AutoAPIHandler provides REST API endpoints for auto-update management.
type AutoAPIHandler struct {
	mgr      *AutoUpdateManager
	timezone *time.Location
	db       *sql.DB
}

// NewAutoAPIHandler creates a new auto-update API handler.
func NewAutoAPIHandler(mgr *AutoUpdateManager, timezone *time.Location, db *sql.DB) *AutoAPIHandler {
	return &AutoAPIHandler{
		mgr:      mgr,
		timezone: timezone,
		db:       db,
	}
}

// RegisterRoutes registers the auto-update API endpoints.
//
// Auto-Update Endpoints:
//
//	GET  /api/ota/auto/status — Returns current auto-update status and configuration
//
//	@Summary		Get auto-update status
//	@Description	Returns the current auto-update state, configuration, and canary progress.
//	@Tags			ota
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}	"Auto-update status"
//	@Router			/api/ota/auto/status [get]
//
//	POST /api/ota/auto/trigger — Manually trigger an auto-update cycle
//
//	@Summary		Trigger auto-update
//	@Description	Manually triggers an auto-update cycle. Only works if auto-update is enabled.
//	@Tags			ota
//	@Accept			json
//	@Produce		json
//	@Success		202	{object}	map[string]string	"Update cycle started"
//	@Failure		400	{object}	map[string]string	"Auto-update disabled or no firmware available"
//	@Router			/api/ota/auto/trigger [post]
//
//	POST /api/ota/auto/cancel — Cancels the current auto-update cycle
//
//	@Summary		Cancel auto-update
//	@Description	Cancels the current in-progress auto-update cycle.
//	@Tags			ota
//	@Produce		json
//	@Success		200	{object}	map[string]string	"Update cycle cancelled"
//	@Router			/api/ota/auto/cancel [post]
//
//	GET /api/ota/auto/config — Returns current auto-update configuration
//
//	@Summary		Get auto-update config
//	@Description	Returns the current auto-update configuration.
//	@Tags			ota
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}	"Auto-update configuration"
//	@Router			/api/ota/auto/config [get]
//
//	GET /api/ota/auto/history — Returns auto-update history (future)
//
//	@Summary		Get auto-update history
//	@Description	Returns historical auto-update events.
//	@Tags			ota
//	@Produce		json
//	@Success		200	{array}		map[string]interface{}	"Auto-update history"
//	@Router			/api/ota/auto/history [get]
func (h *AutoAPIHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/ota/auto/status", h.handleStatus)
	r.Post("/api/ota/auto/trigger", h.handleTrigger)
	r.Post("/api/ota/auto/cancel", h.handleCancel)
	r.Get("/api/ota/auto/config", h.handleConfig)
	r.Get("/api/ota/auto/history", h.handleHistory)
}

// handleStatus handles GET /api/ota/auto/status
func (h *AutoAPIHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if h.mgr == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "auto-update manager not available")
		return
	}

	config := h.mgr.GetConfig()
	state := h.mgr.GetState()
	canaryNode := h.mgr.GetCanaryNode()
	baselineQuality := h.mgr.GetBaselineQuality()

	response := map[string]interface{}{
		"enabled":             config.Enabled,
		"state":               string(state),
		"canary_node":         canaryNode,
		"baseline_quality":    baselineQuality,
		"quiet_window_start":  config.QuietWindowStart,
		"quiet_window_end":    config.QuietWindowEnd,
		"canary_duration_min": config.CanaryDurationMin,
		"quality_threshold":   config.QualityThreshold,
		"is_in_quiet_window":  h.isInQuietWindow(config),
	}

	// Add canary progress info if in canary state
	if state == StateCanaryMonitor || state == StateCanaryDeploy {
		response["canary_progress"] = map[string]interface{}{
			"started_at": time.Now().Add(-time.Minute * 5), // Approximate
		}
	}

	writeJSON(w, http.StatusOK, response)
}

// handleTrigger handles POST /api/ota/auto/trigger
func (h *AutoAPIHandler) handleTrigger(w http.ResponseWriter, r *http.Request) {
	if h.mgr == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "auto-update manager not available")
		return
	}

	if err := h.mgr.TriggerUpdate(r.Context()); err != nil {
		log.Printf("[INFO] ota: auto-update trigger rejected: %v", err)
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	log.Printf("[INFO] ota: auto-update triggered manually via API")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "started", "message": "Auto-update cycle started"})
}

// handleCancel handles POST /api/ota/auto/cancel
func (h *AutoAPIHandler) handleCancel(w http.ResponseWriter, r *http.Request) {
	if h.mgr == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "auto-update manager not available")
		return
	}

	h.mgr.CancelUpdate()

	log.Printf("[INFO] ota: auto-update cancelled manually via API")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "cancelled", "message": "Auto-update cycle cancelled"})
}

// handleConfig handles GET /api/ota/auto/config
func (h *AutoAPIHandler) handleConfig(w http.ResponseWriter, r *http.Request) {
	if h.mgr == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "auto-update manager not available")
		return
	}

	config := h.mgr.GetConfig()

	response := map[string]interface{}{
		"enabled":                 config.Enabled,
		"quiet_window_start":      config.QuietWindowStart,
		"quiet_window_end":        config.QuietWindowEnd,
		"canary_duration_min":     config.CanaryDurationMin,
		"quality_threshold":       config.QualityThreshold,
		"is_in_quiet_window":      h.isInQuietWindow(config),
		"next_quiet_window_start": h.nextQuietWindowStart(config),
	}

	writeJSON(w, http.StatusOK, response)
}

// handleHistory handles GET /api/ota/auto/history
func (h *AutoAPIHandler) handleHistory(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database not available")
		return
	}

	// Parse limit parameter (default 50, max 500)
	limit := 50
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := json.Number(s).Int64(); err == nil && n > 0 {
			limit = int(n)
		}
	}
	if limit > 500 {
		limit = 500
	}
	if limit < 1 {
		limit = 50
	}

	// Parse before cursor for pagination (timestamp_ms as string)
	var beforeTS int64
	if s := r.URL.Query().Get("before"); s != "" {
		if n, err := json.Number(s).Int64(); err == nil {
			beforeTS = n
		}
	}

	// Query OTA update events from the timeline
	queryEvents, _, hasMore, err := events.QueryEvents(h.db, events.QueryParams{
		Type:     events.EventTypeOTAUpdate,
		Limit:    limit,
		BeforeTS: beforeTS,
	})
	if err != nil {
		log.Printf("[ERROR] Failed to query OTA history: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to query history")
		return
	}

	// Convert events to response format
	history := make([]map[string]interface{}, 0, len(queryEvents))
	for _, event := range queryEvents {
		// Parse detail JSON to extract OTA-specific fields
		var detail map[string]interface{}
		if err := json.Unmarshal([]byte(event.DetailJSON), &detail); err != nil {
			// If parsing fails, create a simple detail map
			detail = map[string]interface{}{
				"message": event.DetailJSON,
			}
		}

		// Build history entry
		entry := map[string]interface{}{
			"id":          event.ID,
			"timestamp":   event.TimestampMs,
			"type":        event.Type,
			"severity":    event.Severity,
			"detail":      detail,
		}

		// Extract OTA-specific fields if present
		if otaEvent, ok := detail["ota_event"].(string); ok {
			entry["ota_event"] = otaEvent
		}
		if mac, ok := detail["mac"].(string); ok {
			entry["mac"] = mac
		}
		if message, ok := detail["message"].(string); ok {
			entry["message"] = message
		}
		if metadata, ok := detail["metadata"].(map[string]interface{}); ok {
			entry["metadata"] = metadata
		}

		history = append(history, entry)
	}

	// Build response with pagination metadata
	response := map[string]interface{}{
		"history":  history,
		"has_more": hasMore,
	}

	// Use timestamp for cursor (not ID) to match query parameter type
	if hasMore && len(queryEvents) > 0 {
		// The cursor is the timestamp of the last event in the page
		response["cursor"] = fmt.Sprintf("%d", queryEvents[len(queryEvents)-1].TimestampMs)
	}

	writeJSON(w, http.StatusOK, response)
}

// isInQuietWindow checks if current time is within the quiet window
func (h *AutoAPIHandler) isInQuietWindow(config AutoUpdateConfig) bool {
	if config.QuietWindowStart == "" || config.QuietWindowEnd == "" {
		return true // No quiet window configured
	}

	now := time.Now().In(h.timezone)

	startTime, err := time.Parse("15:04", config.QuietWindowStart)
	if err != nil {
		return true
	}

	endTime, err := time.Parse("15:04", config.QuietWindowEnd)
	if err != nil {
		return true
	}

	currentTime := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), 0, 0, h.timezone)
	start := time.Date(now.Year(), now.Month(), now.Day(), startTime.Hour(), startTime.Minute(), 0, 0, h.timezone)
	end := time.Date(now.Year(), now.Month(), now.Day(), endTime.Hour(), endTime.Minute(), 0, 0, h.timezone)

	// Handle overnight windows (e.g., 22:00 to 06:00)
	if end.Before(start) {
		if currentTime.Before(start) {
			end = end.Add(24 * time.Hour)
		} else {
			end = end.Add(24 * time.Hour)
		}
	}

	return currentTime.After(start) && currentTime.Before(end)
}

// nextQuietWindowStart calculates when the next quiet window starts
func (h *AutoAPIHandler) nextQuietWindowStart(config AutoUpdateConfig) string {
	if config.QuietWindowStart == "" {
		return ""
	}

	now := time.Now().In(h.timezone)
	startTime, _ := time.Parse("15:04", config.QuietWindowStart)

	start := time.Date(now.Year(), now.Month(), now.Day(), startTime.Hour(), startTime.Minute(), 0, 0, h.timezone)

	// If we're past today's window start, return tomorrow's
	if now.After(start) {
		start = start.Add(24 * time.Hour)
	}

	return start.Format(time.RFC3339)
}

// writeJSON writes a JSON response with the given status code
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeJSONError writes a JSON error response
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
