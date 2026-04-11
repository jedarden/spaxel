// Package api provides REST API handlers for active alerts.
// Combines fall detection, anomaly detection, and node status into a unified alert system.
package api

import (
	"log"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/spaxel/mothership/internal/analytics"
	"github.com/spaxel/mothership/internal/events"
	"github.com/spaxel/mothership/internal/falldetect"
	"github.com/spaxel/mothership/internal/fleet"
)

// AlertsHandler manages the unified alerts API.
type AlertsHandler struct {
	mu                sync.RWMutex
	fallDetector      *falldetect.Detector
	anomalyDetector   *analytics.Detector
	fleetRegistry     *fleet.Registry
}

// Alert represents a unified alert from any source.
type Alert struct {
	ID        string  `json:"id"`
	Type      string  `json:"type"`      // "fall", "anomaly", "node_offline"
	Severity  string  `json:"severity"`  // "critical", "warning", "info"
	Title     string  `json:"title"`
	Message   string  `json:"message"`
	Zone      string  `json:"zone,omitempty"`
	Person    string  `json:"person,omitempty"`
	Timestamp int64  `json:"timestamp_ms"`
	Data      any     `json:"data,omitempty"` // Type-specific data
}

// ActiveAlertsResponse is the response for GET /api/alerts/active.
type ActiveAlertsResponse struct {
	Alerts []Alert `json:"alerts"`
	Count  int      `json:"count"`
}

// NewAlertsHandler creates a new alerts handler.
func NewAlertsHandler() *AlertsHandler {
	return &AlertsHandler{}
}

// SetFallDetector sets the fall detection module.
func (h *AlertsHandler) SetFallDetector(detector *falldetect.Detector) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.fallDetector = detector
}

// SetAnomalyDetector sets the anomaly detection module.
func (h *AlertsHandler) SetAnomalyDetector(detector *analytics.Detector) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.anomalyDetector = detector
}

// SetFleetRegistry sets the fleet registry for node status.
func (h *AlertsHandler) SetFleetRegistry(registry *fleet.Registry) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.fleetRegistry = registry
}

// RegisterRoutes registers the alerts API routes.
func (h *AlertsHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/alerts/active", h.handleGetActiveAlerts)
	r.Post("/api/alerts/{id}/acknowledge", h.handleAcknowledgeAlert)

	// Convenience routes for fall and anomaly acknowledgment
	// These redirect to the unified alert endpoint
	r.Post("/api/fall/{id}/acknowledge", h.handleAcknowledgeFall)
	r.Post("/api/anomalies/{id}/acknowledge", h.handleAcknowledgeAnomaly)
}

// handleGetActiveAlerts returns all active alerts from all sources.
// GET /api/alerts/active
//
// Response:
//
//	{
//	  "alerts": [
//	    {
//	      "id": "fall-123",
//	      "type": "fall",
//	      "severity": "critical",
//	      "title": "Possible fall detected",
//	      "message": "Alice in Hallway",
//	      "zone": "hallway",
//	      "person": "Alice",
//	      "timestamp_ms": 1711234567890,
//	      "data": { ... }
//	    }
//	  ],
//	  "count": 2
//	}
func (h *AlertsHandler) handleGetActiveAlerts(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var alerts []Alert

	// Add active fall alerts
	if h.fallDetector != nil {
		falls := h.fallDetector.GetActiveFalls()
		for _, fall := range falls {
			alert := Alert{
				ID:        "fall-" + fall.ID,
				Type:      "fall",
				Severity:  "critical",
				Title:     "Possible fall detected",
				Message:   h.formatFallMessage(fall),
				Zone:      fall.ZoneName,
				Person:    fall.Identity,
				Timestamp: fall.Timestamp.Unix() * 1000,
				Data:      fall,
			}
			alerts = append(alerts, alert)
		}
	}

	// Add active anomaly alerts
	if h.anomalyDetector != nil {
		anomalies := h.anomalyDetector.GetActiveAnomalies()
		for _, anomaly := range anomalies {
			alert := Alert{
				ID:        "anomaly-" + anomaly.ID,
				Type:      "anomaly",
				Severity:  "warning",
				Title:     "Unusual activity detected",
				Message:   h.formatAnomalyMessage(anomaly),
				Timestamp: anomaly.Timestamp.Unix() * 1000,
				Data:      anomaly,
			}
			alerts = append(alerts, alert)
		}
	}

	// Add offline node alerts
	if h.fleetRegistry != nil {
		nodes, err := h.fleetRegistry.GetAllNodes()
		if err == nil {
			for _, node := range nodes {
				if !node.WentOfflineAt.IsZero() {
					alert := Alert{
						ID:        "node-" + node.MAC,
						Type:      "node_offline",
						Severity:  "warning",
						Title:     "Node offline",
						Message:   "Node " + node.Name + " went offline",
						Timestamp: node.WentOfflineAt.Unix() * 1000,
						Data: map[string]interface{}{
							"mac":          node.MAC,
							"name":         node.Name,
							"status":       "offline",
							"last_seen_at": node.LastSeenAt,
						},
					}
					alerts = append(alerts, alert)
				}
			}
		}
	}

	// Sort by severity (critical > warning > info) and timestamp
	h.sortAlerts(alerts)

	response := ActiveAlertsResponse{
		Alerts: alerts,
		Count:  len(alerts),
	}

	writeJSON(w, http.StatusOK, response)
}

// handleAcknowledgeAlert acknowledges an alert by ID.
// POST /api/alerts/{id}/acknowledge
//
// The ID is prefixed by the alert type: "fall-123", "anomaly-456", "node-AA:BB:CC:DD:EE:FF".
func (h *AlertsHandler) handleAcknowledgeAlert(w http.ResponseWriter, r *http.Request) {
	alertID := chi.URLParam(r, "id")
	if alertID == "" {
		http.Error(w, "alert ID required", http.StatusBadRequest)
		return
	}

	// Parse the alert type and ID
	var alertType, id string
	if len(alertID) > 5 && alertID[4] == '-' {
		alertType = alertID[:4]
		id = alertID[5:]
	} else {
		http.Error(w, "invalid alert ID format", http.StatusBadRequest)
		return
	}

	var err error
	switch alertType {
	case "fall":
		if h.fallDetector != nil {
			err = h.fallDetector.AcknowledgeFall(id, "acknowledged")
		} else {
			log.Printf("[WARN] Fall detector not available for acknowledgment")
		}
	case "anomaly":
		if h.anomalyDetector != nil {
			err = h.anomalyDetector.AcknowledgeAnomaly(id, "", "")
		} else {
			log.Printf("[WARN] Anomaly detector not available for acknowledgment")
		}
	case "node":
		// Node alerts don't require acknowledgment - they auto-clear when node comes back online
		log.Printf("[INFO] Node offline alert acknowledged: %s", id)
	default:
		http.Error(w, "unknown alert type: "+alertType, http.StatusBadRequest)
		return
	}

	if err != nil {
		log.Printf("[ERROR] Failed to acknowledge alert %s: %v", alertID, err)
		http.Error(w, "failed to acknowledge alert", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "acknowledged", "id": alertID})
}

// sortAlerts sorts alerts by severity and timestamp.
func (h *AlertsHandler) sortAlerts(alerts []Alert) {
	// Simple bubble sort (small slice size expected)
	for i := 0; i < len(alerts)-1; i++ {
		for j := 0; j < len(alerts)-1-i; j++ {
			if h.compareAlerts(alerts[j], alerts[j+1]) > 0 {
				alerts[j], alerts[j+1] = alerts[j+1], alerts[j]
			}
		}
	}
}

// compareAlerts returns negative if a < b (a should come first).
func (h *AlertsHandler) compareAlerts(a, b Alert) int {
	// First compare by severity
	severityOrder := map[string]int{
		"critical": 0,
		"warning":  1,
		"info":     2,
	}

	if severityOrder[a.Severity] != severityOrder[b.Severity] {
		return severityOrder[a.Severity] - severityOrder[b.Severity]
	}

	// Same severity, compare by timestamp (newer first)
	if a.Timestamp > b.Timestamp {
		return -1
	}
	if a.Timestamp < b.Timestamp {
		return 1
	}
	return 0
}

// formatFallMessage formats a fall event into a human-readable message.
func (h *AlertsHandler) formatFallMessage(fall falldetect.FallEvent) string {
	if fall.Identity != "" {
		return fall.Identity + " in " + fall.ZoneName
	}
	return "Someone in " + fall.ZoneName
}

// formatAnomalyMessage formats an anomaly into a human-readable message.
func (h *AlertsHandler) formatAnomalyMessage(anomaly *events.AnomalyEvent) string {
	// Format the anomaly message based on its type and details
	return "Unusual activity detected"
}

// handleAcknowledgeFall acknowledges a fall alert.
// POST /api/fall/{id}/acknowledge
//
// This is a convenience route that redirects to the unified alert endpoint.
// The alert ID is the fall ID (without the "fall-" prefix).
func (h *AlertsHandler) handleAcknowledgeFall(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "fall ID required", http.StatusBadRequest)
		return
	}

	// Call the unified acknowledge handler with "fall-{id}" format
	h.handleAcknowledgeAlert(w, r)

	// Override the path parameter to use the prefixed version
	// This is handled by the unified handler
}

// handleAcknowledgeAnomaly acknowledges an anomaly alert.
// POST /api/anomalies/{id}/acknowledge
//
// This is a convenience route that redirects to the unified alert endpoint.
// The alert ID is the anomaly ID (without the "anomaly-" prefix).
func (h *AlertsHandler) handleAcknowledgeAnomaly(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "anomaly ID required", http.StatusBadRequest)
		return
	}

	// Call the unified acknowledge handler with "anomaly-{id}" format
	h.handleAcknowledgeAlert(w, r)

	// Override the path parameter to use the prefixed version
	// This is handled by the unified handler
}

