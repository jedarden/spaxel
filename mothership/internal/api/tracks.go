// Package api provides REST API handlers for Spaxel tracks (tracked people).
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Track represents a tracked person with identity and position.
type Track struct {
	ID                 int     `json:"id"`
	X                  float64 `json:"x"`
	Y                  float64 `json:"y"`
	Z                  float64 `json:"z"`
	VX                 float64 `json:"vx"`
	VY                 float64 `json:"vy"`
	VZ                 float64 `json:"vz"`
	Weight             float64 `json:"weight"`
	PersonID           string  `json:"person_id,omitempty"`
	PersonLabel        string  `json:"person_label,omitempty"`
	PersonColor        string  `json:"person_color,omitempty"`
	IdentityConfidence float64 `json:"identity_confidence,omitempty"`
	IdentitySource     string  `json:"identity_source,omitempty"`
	Posture            string  `json:"posture,omitempty"`
}

// TracksProvider is the interface for getting current tracked blobs.
type TracksProvider interface {
	GetTrackedBlobs() []TrackedBlob
}

// TrackedBlob represents a tracked spatial blob from the fusion engine.
type TrackedBlob struct {
	ID                 int
	X, Y, Z            float64
	VX, VY, VZ         float64
	Weight             float64
	PersonID           string
	PersonLabel        string
	PersonColor        string
	IdentityConfidence float64
	IdentitySource     string
	Posture            string
}

// TracksHandler manages the tracks REST API.
type TracksHandler struct {
	provider TracksProvider
}

// NewTracksHandler creates a new tracks handler.
func NewTracksHandler(provider TracksProvider) *TracksHandler {
	return &TracksHandler{provider: provider}
}

// RegisterRoutes mounts tracks endpoints on r.
//
// GET /api/tracks
//
//	@Summary		List tracked people
//	@Description	Returns all currently tracked people with identity information and position. Identity is populated by BLE-to-blob matching when BLE devices are associated with people.
//	@Tags			tracks
//	@Produce		json
//	@Success		200	{array}	Track	"List of tracks with identity fields"
//	@Router			/api/tracks [get]
func (h *TracksHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/tracks", h.listTracks)
}

// listTracks handles GET /api/tracks.
//
// Returns all currently tracked people with identity information and position.
// The response includes:
//   - id: Blob ID
//   - x, y, z: Position coordinates (meters)
//   - vx, vy, vz: Velocity vectors (m/s)
//   - weight: Blob weight (confidence)
//   - person_id: UUID of associated person (if matched)
//   - person_label: Human-readable name (if matched)
//   - person_color: Display color for person (if matched)
//   - identity_confidence: BLE-to-blob match confidence (0-1)
//   - identity_source: Source of identity ("ble", "vision", etc.)
//   - posture: Detected posture (standing, sitting, etc.)
//
// Status codes:
//   - 200: Success
func (h *TracksHandler) listTracks(w http.ResponseWriter, r *http.Request) {
	blobs := h.provider.GetTrackedBlobs()
	tracks := make([]Track, len(blobs))
	for i, b := range blobs {
		tracks[i] = Track{
			ID:                 b.ID,
			X:                  b.X,
			Y:                  b.Y,
			Z:                  b.Z,
			VX:                 b.VX,
			VY:                 b.VY,
			VZ:                 b.VZ,
			Weight:             b.Weight,
			PersonID:           b.PersonID,
			PersonLabel:        b.PersonLabel,
			PersonColor:        b.PersonColor,
			IdentityConfidence: b.IdentityConfidence,
			IdentitySource:     b.IdentitySource,
			Posture:            b.Posture,
		}
	}
	writeJSON(w, http.StatusOK, tracks)
}
