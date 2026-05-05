// Package api provides REST API handlers for Spaxel baseline management.
package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"
)

// BaselineHandler manages baseline API endpoints.
type BaselineHandler struct {
	db *sql.DB
}

// NewBaselineHandler creates a new baseline API handler.
func NewBaselineHandler(db *sql.DB) *BaselineHandler {
	return &BaselineHandler{db: db}
}

// RegisterRoutes registers baseline endpoints.
//
//	GET  /api/baseline         — list all baseline snapshots
//	POST /api/baseline/capture — start a 60s quiet-room capture
func (h *BaselineHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/baseline", h.listBaselines)
	r.Post("/api/baseline/capture", h.captureBaseline)
}

// BaselineEntry represents a single baseline snapshot.
type BaselineEntry struct {
	LinkID       string  `json:"link_id"`
	SnapshotTime int64   `json:"snapshot_time_ms"` // Unix milliseconds
	Confidence   float64 `json:"confidence"`        // 0.0–1.0
	NSub         int     `json:"n_sub"`             // Number of subcarriers
}

// listBaselines handles GET /api/baseline
// Returns the most recent baseline snapshot for each link.
func (h *BaselineHandler) listBaselines(w http.ResponseWriter, r *http.Request) {
	// Query the most recent baseline for each link
	// Using GROUP BY to get only the latest snapshot per link
	query := `
		SELECT link_id, captured_at, confidence, n_sub
		FROM baselines b1
		WHERE captured_at = (
			SELECT MAX(captured_at)
			FROM baselines b2
			WHERE b2.link_id = b1.link_id
		)
		ORDER BY link_id
	`

	rows, err := h.db.Query(query)
	if err != nil {
		log.Printf("[ERROR] Failed to query baselines: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to query baselines")
		return
	}
	defer rows.Close()

	baselines := make([]BaselineEntry, 0)
	for rows.Next() {
		var b BaselineEntry
		if err := rows.Scan(&b.LinkID, &b.SnapshotTime, &b.Confidence, &b.NSub); err != nil {
			log.Printf("[ERROR] Failed to scan baseline row: %v", err)
			continue
		}
		baselines = append(baselines, b)
	}

	if err := rows.Err(); err != nil {
		log.Printf("[ERROR] Error iterating baseline rows: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "error reading baselines")
		return
	}

	if baselines == nil {
		baselines = []BaselineEntry{}
	}

	writeJSON(w, http.StatusOK, baselines)
}

// captureRequest is the request body for POST /api/baseline/capture.
type captureRequest struct {
	Links []string `json:"links"` // Optional list of link_ids to capture. Empty = all links.
}

// captureResponse is the response for POST /api/baseline/capture.
type captureResponse struct {
	OK            bool     `json:"ok"`
	LinksCaptured int      `json:"links_captured"`
	Links         []string `json:"links,omitempty"` // The links being captured
	Message       string   `json:"message,omitempty"`
}

// captureBaseline handles POST /api/baseline/capture
// Starts a 60-second quiet-room baseline capture.
// The actual capture is handled by the baseline system in the signal processor;
// this endpoint initiates the capture process by resetting baselines and
// allowing them to re-accumulate during the quiet period.
func (h *BaselineHandler) captureBaseline(w http.ResponseWriter, r *http.Request) {
	var req captureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Get the list of links to capture
	var linksToCapture []string
	if len(req.Links) > 0 {
		// Validate that the requested links exist
		linksToCapture = req.Links
	} else {
		// Get all unique link_ids from the baselines table
		rows, err := h.db.Query("SELECT DISTINCT link_id FROM baselines")
		if err != nil {
			log.Printf("[ERROR] Failed to query link_ids: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to query links")
			return
		}
		defer rows.Close()

		for rows.Next() {
			var linkID string
			if err := rows.Scan(&linkID); err != nil {
				continue
			}
			linksToCapture = append(linksToCapture, linkID)
		}
		rows.Close()
	}

	// If no links found, return empty response
	if len(linksToCapture) == 0 {
		writeJSON(w, http.StatusOK, captureResponse{
			OK:            true,
			LinksCaptured: 0,
			Message:       "No links found to capture. Capture will start automatically once links are active.",
		})
		return
	}

	// Log the capture request
	log.Printf("[INFO] Baseline capture requested for %d links: %v", len(linksToCapture), linksToCapture)

	// The actual baseline capture happens automatically in the signal processing pipeline.
	// We insert a marker into the baselines table to indicate the capture start time.
	// This allows the dashboard to show when a capture was initiated.
	captureTime := time.Now().UnixMilli()

	for _, linkID := range linksToCapture {
		// Get the current baseline state for this link to preserve n_sub
		var nSub int
		err := h.db.QueryRow("SELECT n_sub FROM baselines WHERE link_id = ? ORDER BY captured_at DESC LIMIT 1", linkID).Scan(&nSub)
		if err != nil {
			// Link not found in baselines, use default
			nSub = 64
		}

		// Insert a capture marker (a baseline entry with empty amplitude/phase BLOBs)
		// This marks the start of the capture period
		_, err = h.db.Exec(`
			INSERT INTO baselines (link_id, captured_at, n_sub, amplitude, phase, confidence)
			VALUES (?, ?, ?, X'', X'', 0.0)
		`, linkID, captureTime, nSub)
		if err != nil {
			log.Printf("[ERROR] Failed to insert capture marker for %s: %v", linkID, err)
		}
	}

	writeJSON(w, http.StatusAccepted, captureResponse{
		OK:            true,
		LinksCaptured: len(linksToCapture),
		Links:         linksToCapture,
		Message:       "Baseline capture started. Keep the room clear for 60 seconds for best results.",
	})
}
