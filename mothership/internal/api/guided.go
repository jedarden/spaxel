// Package api provides REST API handlers for Spaxel guided troubleshooting.
package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// GuidedHandler provides endpoints for proactive contextual help.
type GuidedHandler struct {
	guidedMgr interface {
		GetZonesWithPoorQuality() []int
		MarkQualityBannerShown(zoneID int)
		TriggerCalibrationComplete(zoneID int, qualityBefore, qualityAfter float64)
		TriggerNodeOffline(mac string, offlineDuration float64) // for testing
	}
	zonesHandler interface {
		GetZone(id int) (map[string]interface{}, error)
		GetAllZones() ([]map[string]interface{}, error)
	}
	nodesHandler interface {
		GetAllNodes() ([]map[string]interface{}, error)
	}
}

// NewGuidedHandler creates a new guided troubleshooting handler.
func NewGuidedHandler(guidedMgr interface {
	GetZonesWithPoorQuality() []int
	MarkQualityBannerShown(zoneID int)
	TriggerCalibrationComplete(zoneID int, qualityBefore, qualityAfter float64)
	TriggerNodeOffline(mac string, offlineDuration float64)
}) *GuidedHandler {
	return &GuidedHandler{
		guidedMgr: guidedMgr,
	}
}

// SetZonesHandler sets the zones handler for zone information access.
func (h *GuidedHandler) SetZonesHandler(zonesHandler interface {
	GetZone(id int) (map[string]interface{}, error)
	GetAllZones() ([]map[string]interface{}, error)
}) {
	h.zonesHandler = zonesHandler
}

// SetNodesHandler sets the nodes handler for node information access.
func (h *GuidedHandler) SetNodesHandler(nodesHandler interface {
	GetAllNodes() ([]map[string]interface{}, error)
}) {
	h.nodesHandler = nodesHandler
}

// RegisterRoutes registers guided troubleshooting endpoints.
func (h *GuidedHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/guided/issues", h.handleGetIssues)
	r.Post("/api/guided/issues/quality/{zoneId}/dismiss", h.handleDismissQualityIssue)
	r.Post("/api/guided/feedback/response", h.handleGetFeedbackResponse)
	r.Post("/api/guided/calibration/complete", h.handleCalibrationComplete)
	r.Get("/api/guided/node/{mac}/troubleshoot", h.handleGetNodeTroubleshoot)
	r.Get("/api/guided/tooltip/{featureId}", h.handleGetTooltip)
	r.Post("/api/guided/tooltip/{featureId}/dismiss", h.handleDismissTooltip)
}

// handleGetIssues returns all active guided troubleshooting issues.
func (h *GuidedHandler) handleGetIssues(w http.ResponseWriter, r *http.Request) {
	if h.guidedMgr == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"issues": []interface{}{}})
		return
	}

	var issues []map[string]interface{}

	// Quality issues
	poorZones := h.guidedMgr.GetZonesWithPoorQuality()
	for _, zoneID := range poorZones {
		zoneName := "Unknown Zone"
		zoneQuality := 0.0

		if h.zonesHandler != nil {
			if zone, err := h.getZoneByID(zoneID); err == nil {
				zoneName = zone["name"].(string)
				if q, ok := zone["quality"].(float64); ok {
					zoneQuality = q
				}
			}
		}

		issues = append(issues, map[string]interface{}{
			"type":        "quality_drop",
			"zone_id":     zoneID,
			"zone_name":   zoneName,
			"quality":     zoneQuality,
			"severity":    "warning",
			"title":       "Detection quality has degraded in " + zoneName,
			"description": "Detection quality in " + zoneName + " has been below 60% for over 24 hours. This may indicate node placement issues or environmental changes.",
			"actions": []map[string]string{
				{"label": "Check node connectivity", "action": "connectivity"},
				{"label": "View link health", "action": "link_health"},
				{"label": "Re-baseline links", "action": "rebaseline"},
				{"label": "Run guided diagnostics", "action": "diagnostics"},
			},
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"issues": issues})
}

// handleDismissQualityIssue dismisses a quality banner for a zone.
func (h *GuidedHandler) handleDismissQualityIssue(w http.ResponseWriter, r *http.Request) {
	zoneID := chi.URLParam(r, "zoneId")
	var zoneIDInt int
	if _, err := json.Unmarshal([]byte(zoneID), &zoneIDInt); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid zone ID")
		return
	}

	if h.guidedMgr != nil {
		h.guidedMgr.MarkQualityBannerShown(zoneIDInt)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// handleGetFeedbackResponse returns the inline response message for a feedback submission.
func (h *GuidedHandler) handleGetFeedbackResponse(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FeedbackType string `json:"feedback_type"` // "incorrect" or "correct"
		Links        []struct {
			LinkID string `json:"link_id"`
			DeltaRMS float64 `json:"delta_rms"`
		} `json:"links,omitempty"`
		ZoneID   *int    `json:"zone_id,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var response map[string]interface{}

	switch req.FeedbackType {
	case "incorrect":
		response = map[string]interface{}{
			"type":    "adjustment",
			"title":   "Adjusting detection threshold",
			"message": "I've slightly raised the detection threshold for the contributing links. If this keeps happening at this time of day, my hourly baseline will adapt within a few days. You can also adjust sensitivity manually in Settings.",
			"actions": []map[string]string{
				{"label": "Open Settings", "action": "open_settings"},
				{"label": "View Link Details", "action": "view_links"},
			},
		}

	case "correct":
		response = map[string]interface{}{
			"type":    "confirmation",
			"title":   "Detection confirmed",
			"message": "Thanks for confirming! This helps improve detection accuracy over time.",
		}

	default:
		response = map[string]interface{}{
			"type":    "info",
			"message": "Feedback recorded",
		}
	}

	writeJSON(w, http.StatusOK, response)
}

// handleCalibrationComplete reports calibration completion and triggers reinforcement.
func (h *GuidedHandler) handleCalibrationComplete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ZoneID         int     `json:"zone_id"`
		QualityBefore  float64 `json:"quality_before"`
		QualityAfter   float64 `json:"quality_after"`
		LinksCalibrated int     `json:"links_calibrated"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if h.guidedMgr != nil {
		h.guidedMgr.TriggerCalibrationComplete(req.ZoneID, req.QualityBefore, req.QualityAfter)
	}

	// Calculate improvement
	improvement := req.QualityAfter - req.QualityBefore
	improvementPct := int(improvement)

	response := map[string]interface{}{
		"type":         "calibration_complete",
		"title":        "Re-baseline complete",
		"message":      "Detection quality in this zone has improved.",
		"improvement":  improvementPct,
		"quality_after": req.QualityAfter,
		"links":        req.LinksCalibrated,
	}

	// Add encouraging message based on improvement
	if improvement > 20 {
		response["encouragement"] = "Excellent! That's a significant improvement."
	} else if improvement > 10 {
		response["encouragement"] = "Great progress! Detection is much more reliable now."
	} else if improvement > 0 {
		response["encouragement"] = "Getting better. The system will continue to refine baseline over time."
	} else {
		response["encouragement"] = "Baseline has been updated. The system needs more data to adapt to this environment."
	}

	writeJSON(w, http.StatusOK, response)
}

// handleGetNodeTroubleshoot returns troubleshooting steps for an offline node.
func (h *GuidedHandler) handleGetNodeTroubleshoot(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")

	// Get node info
	var nodeName, nodeRole, lastSeen string
	var offlineDuration float64

	if h.nodesHandler != nil {
		nodes, err := h.nodesHandler.(interface {
			GetAllNodes() ([]map[string]interface{}, error)
		}).GetAllNodes()
		if err == nil {
			for _, node := range nodes {
				if nodeMAC, ok := node["mac"].(string); ok && nodeMAC == mac {
					nodeName = node["name"].(string)
					nodeRole = node["role"].(string)
					// Calculate offline duration from last_seen_ms
					if lastSeenMs, ok := node["last_seen_ms"].(int64); ok {
						// Calculate approximate duration
						offlineDuration = float64(time.Now().UnixMilli()-lastSeenMs) / 1000 / 60 // in minutes
					}
					break
				}
			}
		}
	}

	// Create troubleshooting steps
	steps := []map[string]interface{}{
		{
			"step":        1,
			"title":       "Check power connection",
			"description": "Verify the node's USB cable is securely connected and the power LED is on (solid green = connected, blinking = attempting WiFi).",
			"actions":     []string{"Visually inspect the node", "Check the USB cable connection"},
		},
		{
			"step":        2,
			"title":       "Check WiFi connectivity",
			"description": "If the LED is blinking, the node is having trouble connecting to WiFi. Try moving it closer to your WiFi router.",
			"actions":     []string{"Move node closer to router", "Check WiFi is working"},
		},
		{
			"step":        3,
			"title":       "Check for captive portal",
			"description": "If the LED blinks rapidly after 5 minutes, the node has lost its WiFi configuration. Look for a WiFi network named 'spaxel-" + mac[len(mac)-4:] + "' and connect to reconfigure.",
			"actions":     []string{"Connect to spaxel-XXXX WiFi", "Re-enter WiFi credentials"},
		},
		{
			"step":        4,
			"title":       "Check hardware",
			"description": "If the LED is off, check the power supply and try a different USB cable or port.",
			"actions":     []string{"Try different USB cable", "Try different power source"},
		},
	}

	response := map[string]interface{}{
		"mac":              mac,
		"name":             nodeName,
		"role":             nodeRole,
		"offline_minutes":  int(offlineDuration),
		"troubleshooting":  steps,
		"escalation":       "If the issue persists after these steps, you may need to reflash the firmware or reset the node to factory defaults.",
	}

	writeJSON(w, http.StatusOK, response)
}

// getZoneByID is a helper to get zone information by ID.
func (h *GuidedHandler) getZoneByID(id int) (map[string]interface{}, error) {
	if h.zonesHandler == nil {
		return nil, ErrZoneNotFound
	}

	// Try to get specific zone first
	type zoneGetter interface {
		GetZone(id int) (map[string]interface{}, error)
	}

	if zg, ok := h.zonesHandler.(zoneGetter); ok {
		zone, err := zg.GetZone(id)
		if err == nil {
			return zone, nil
		}
	}

	// Fall back to getting all zones
	type allZonesGetter interface {
		GetAllZones() ([]map[string]interface{}, error)
	}

	if azg, ok := h.zonesHandler.(allZonesGetter); ok {
		zones, err := azg.GetAllZones()
		if err != nil {
			return nil, err
		}

		for _, zone := range zones {
			if zoneID, ok := zone["id"].(int); ok && zoneID == id {
				return zone, nil
			}
			if zoneID, ok := zone["id"].(float64); ok && int(zoneID) == id {
				return zone, nil
			}
		}
	}

	return nil, ErrZoneNotFound
}

// ErrZoneNotFound is returned when a zone cannot be found.
var ErrZoneNotFound = &HTTPError{StatusCode: 404, Message: "zone not found"}

// HTTPError represents an HTTP error with a status code and message.
type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	return e.Message
}

// handleGetTooltip returns the tooltip for a feature if it should be shown.
func (h *GuidedHandler) handleGetTooltip(w http.ResponseWriter, r *http.Request) {
	if h.guidedMgr == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"show": false})
		return
	}

	featureID := chi.URLParam(r, "featureId")
	if featureID == "" {
		writeJSONError(w, http.StatusBadRequest, "missing feature ID")
		return
	}

	shouldShow := h.guidedMgr.ShouldShowTooltip(featureID)
	if !shouldShow {
		writeJSON(w, http.StatusOK, map[string]interface{}{"show": false})
		return
	}

	tooltip, exists := h.guidedMgr.GetTooltip(featureID)
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "tooltip not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"show":       true,
		"title":      tooltip.Title,
		"description": tooltip.Description,
		"direction":  tooltip.Direction,
	})
}

// handleDismissTooltip marks a tooltip as shown (dismissed).
func (h *GuidedHandler) handleDismissTooltip(w http.ResponseWriter, r *http.Request) {
	if h.guidedMgr == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		return
	}

	featureID := chi.URLParam(r, "featureId")
	if featureID == "" {
		writeJSONError(w, http.StatusBadRequest, "missing feature ID")
		return
	}

	h.guidedMgr.MarkTooltipShown(featureID)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}
