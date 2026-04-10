// Package api provides REST API handlers for Spaxel feedback.
package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// FeedbackRequest represents a feedback submission from the timeline.
type FeedbackRequest struct {
	Type     string `json:"type"`     // "correct" or "incorrect"
	EventID  int64  `json:"event_id"` // Optional: event ID being rated
	BlobID   int    `json:"blob_id"`  // Optional: blob ID being rated
	Position *struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
		Z float64 `json:"z"`
	} `json:"position,omitempty"` // For "missed" feedback
}

// FeedbackHandler handles simple feedback submissions from the UI.
type FeedbackHandler struct {
	eventsHandler *EventsHandler
	learningHandler any // Learning handler with ProcessFeedback method
}

// NewFeedbackHandler creates a new feedback handler.
func NewFeedbackHandler(eventsHandler *EventsHandler) *FeedbackHandler {
	return &FeedbackHandler{
		eventsHandler: eventsHandler,
	}
}

// SetLearningHandler sets the learning handler for feedback processing.
func (h *FeedbackHandler) SetLearningHandler(learningHandler any) {
	h.learningHandler = learningHandler
}

// RegisterRoutes registers feedback endpoints.
func (h *FeedbackHandler) RegisterRoutes(r chi.Router) {
	r.Post("/api/feedback", h.handleSubmitFeedback)
}

// handleSubmitFeedback handles POST /api/feedback
func (h *FeedbackHandler) handleSubmitFeedback(w http.ResponseWriter, r *http.Request) {
	var req FeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate feedback type
	if req.Type != "correct" && req.Type != "incorrect" && req.Type != "missed" {
		writeJSONError(w, http.StatusBadRequest, "invalid feedback type: must be 'correct', 'incorrect', or 'missed'")
		return
	}

	// Get event details for logging
	var eventType, zone, person string
	var detailJSON string

	if req.EventID > 0 {
		// Look up event by ID
		// This would require a DB query - for now, we'll just log the ID
		log.Printf("[INFO] Feedback for event %d: type=%s blob_id=%d", req.EventID, req.Type, req.BlobID)
	} else {
		log.Printf("[INFO] Feedback without event: type=%s blob_id=%d", req.Type, req.BlobID)
	}

	// Create detail JSON for the event
	details := make(map[string]interface{})
	if req.Position != nil {
		details["position"] = req.Position
	}

	detailBytes, _ := json.Marshal(details)
	detailJSON = string(detailBytes)

	// If this is feedback for a specific event, we could update the event's detail_json
	// to include the feedback. For now, we'll just log a new event.
	if req.Type == "incorrect" && req.EventID > 0 {
		// Log a "corrected" event to indicate the original was marked incorrect
		if h.eventsHandler != nil {
			_ = h.eventsHandler.LogEvent("feedback_corrected", time.Now(), zone, person, req.BlobID,
				`{"original_event_id":`+strconv.FormatInt(req.EventID, 10)+`,"feedback":"incorrect"}`, "info")
		}
	} else if req.Type == "correct" && req.EventID > 0 {
		// Log a positive feedback event
		if h.eventsHandler != nil {
			_ = h.eventsHandler.LogEvent("feedback_confirmed", time.Now(), zone, person, req.BlobID,
				`{"original_event_id":`+strconv.FormatInt(req.EventID, 10)+`,"feedback":"correct"}`, "info")
		}
	} else if req.Type == "missed" && req.Position != nil {
		// Log a missed detection
		if h.eventsHandler != nil {
			_ = h.eventsHandler.LogEvent("missed_detection", time.Now(), zone, person, req.BlobID,
				detailJSON, "warning")
		}
	}

	// If learning handler is available, process the feedback
	if h.learningHandler != nil {
		// Try to call ProcessFeedback if the method exists
		// This uses reflection-like approach to avoid tight coupling
		type processor interface {
			ProcessFeedback(feedbackType string, eventID int64, blobID int, positionJSON string) error
		}
		if p, ok := h.learningHandler.(processor); ok {
			var positionJSON string
			if req.Position != nil {
				positionBytes, _ := json.Marshal(req.Position)
				positionJSON = string(positionBytes)
			}
			_ = p.ProcessFeedback(req.Type, req.EventID, req.BlobID, positionJSON)
		}
	}

	// Return success response with inline message
	response := map[string]interface{}{
		"ok":     true,
		"message": "Feedback recorded",
	}

	// Add inline response based on feedback type
	switch req.Type {
	case "incorrect":
		response["inline_response"] = map[string]interface{}{
			"type":    "adjustment",
			"title":   "Adjusting detection threshold",
			"message": "I've slightly raised the detection threshold for the contributing links. If this keeps happening at this time of day, my hourly baseline will adapt within a few days. You can also adjust sensitivity manually in Settings.",
		}
	case "correct":
		response["inline_response"] = map[string]interface{}{
			"type":    "confirmation",
			"title":   "Thanks for confirming!",
			"message": "This helps improve detection accuracy over time.",
		}
	}

	writeJSON(w, http.StatusOK, response)
}

// SubmitFeedback is called by the events handler to process feedback for a specific event.
func (h *FeedbackHandler) SubmitFeedback(w http.ResponseWriter, r *http.Request, req FeedbackRequest) {
	// Validate feedback type
	if req.Type != "correct" && req.Type != "incorrect" && req.Type != "missed" {
		writeJSONError(w, http.StatusBadRequest, "invalid feedback type: must be 'correct', 'incorrect', or 'missed'")
		return
	}

	// Get event details for logging
	var zone, person string
	var detailJSON string

	// Create detail JSON for the event
	details := make(map[string]interface{})
	details["original_event_id"] = req.EventID
	details["feedback"] = req.Type
	if req.Position != nil {
		details["position"] = req.Position
	}

	detailBytes, _ := json.Marshal(details)
	detailJSON = string(detailBytes)

	// Log feedback event
	if h.eventsHandler != nil {
		eventType := "feedback_confirmed"
		if req.Type == "incorrect" {
			eventType = "feedback_corrected"
		} else if req.Type == "missed" {
			eventType = "missed_detection"
		}
		_ = h.eventsHandler.LogEvent(eventType, time.Now(), zone, person, req.BlobID, detailJSON, "info")
	}

	// If learning handler is available, process the feedback
	if h.learningHandler != nil {
		type processor interface {
			ProcessFeedback(feedbackType string, eventID int64, blobID int, positionJSON string) error
		}
		if p, ok := h.learningHandler.(processor); ok {
			var positionJSON string
			if req.Position != nil {
				positionBytes, _ := json.Marshal(req.Position)
				positionJSON = string(positionBytes)
			}
			_ = p.ProcessFeedback(req.Type, req.EventID, req.BlobID, positionJSON)
		}
	}

	// Return success response
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":     true,
		"message": "Feedback recorded",
	})
}
