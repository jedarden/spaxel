// Package explainability provides the detection explainability API.
// This allows users to understand why a blob was detected at a specific location
// by showing per-link contributions, Fresnel zone intersections, and confidence breakdown.
package explainability

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

// Handler provides the explainability HTTP API.
type Handler struct {
	mu                   sync.RWMutex
	blobHistory          map[int]*BlobExplanation  // blobID -> explanation data
	blobHistoryByTime    map[int64]*BlobExplanation // timestamp -> explanation for feedback lookups
	linkStates           map[string]*LinkState      // linkID -> link state
	fusionResult         *FusionResultSnapshot       // latest fusion result
}

// BlobExplanation contains all data needed to explain a blob detection.
type BlobExplanation struct {
	BlobID       int                    `json:"blob_id"`
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
	Confidence   float64               `json:"confidence"`
	Timestamp    int64                 `json:"timestamp_ms"`
	ContributingLinks []LinkContribution `json:"contributing_links"`
	AllLinks      []LinkContribution    `json:"all_links"`
	BLEMatch     *BLEMatch             `json:"ble_match,omitempty"`
	FresnelZones []FresnelZone         `json:"fresnel_zones"`
}

// LinkContribution describes how much a link contributed to a blob detection.
type LinkContribution struct {
	LinkID        string  `json:"link_id"`         // e.g., "AA:BB:CC:DD:EE:FF"
	NodeMAC       string  `json:"node_mac"`
	PeerMAC       string  `json:"peer_mac"`
	DeltaRMS      float64 `json:"delta_rms"`
	ZoneNumber    int     `json:"zone_number"`     // Fresnel zone number at blob position
	Weight        float64 `json:"weight"`          // Learned weight multiplier
	Contributing   bool    `json:"contributing"`    // true if deltaRMS exceeded threshold
	Contribution   float64 `json:"contribution"`    // amount added to fusion grid at blob position
}

// BLEMatch describes a BLE device match for the blob.
type BLEMatch struct {
	PersonID          string  `json:"person_id"`
	PersonLabel       string  `json:"person_label"`
	PersonColor       string  `json:"person_color"`
	DeviceAddr        string  `json:"device_addr"`
	Confidence        float64 `json:"confidence"`
	MatchMethod       string  `json:"match_method"` // "ble_triangulation" or "ble_only"
	ReportedByNodes   []string `json:"reported_by_nodes"`
	TriangulationPos  *[3]float64 `json:"triangulation_pos,omitempty"` // [x, y, z]
}

// FresnelZone describes a Fresnel zone ellipsoid for a link.
type FresnelZone struct {
	LinkID      string   `json:"link_id"`
	CenterPos   [3]float64 `json:"center_pos"`   // [x, y, z] zone center
	SemiAxes    [3]float64 `json:"semi_axes"`    // [a, b, c] for ellipsoid
	ZoneNumber  int      `json:"zone_number"`   // zone number for this blob position
}

// LinkState captures the current state of a link.
type LinkState struct {
	NodeMAC    string
	PeerMAC    string
	NodePos    [3]float64 // [x, y, z]
	PeerPos    [3]float64 // [x, y, z]
	DeltaRMS   float64
	Motion     bool
	Weight     float64 // Learned weight
	HealthScore float64
}

// FusionResultSnapshot captures the latest fusion result for explainability.
type FusionResultSnapshot struct {
	Timestamp   int64
	Blobs       []BlobSnapshot
	GridData    *GridSnapshot
}

// BlobSnapshot is a lightweight blob representation.
type BlobSnapshot struct {
	ID         int
	X, Y, Z    float64
	Confidence float64
	Weight     float64 // Peak height in the grid
}

// GridSnapshot captures the fusion grid for computing contributions.
type GridSnapshot struct {
	Width, Depth, CellSize float64
	OriginX, OriginZ       float64
	Data                    []float64 // Normalised [0-1] row-major grid data
	Rows, Cols             int
}

// NewHandler creates a new explainability handler.
func NewHandler() *Handler {
	return &Handler{
		blobHistory:     make(map[int]*BlobExplanation),
		blobHistoryByTime: make(map[int64]*BlobExplanation),
		linkStates:      make(map[string]*LinkState),
		fusionResult:    &FusionResultSnapshot{},
	}
}

// UpdateBlobs updates the handler with the latest blob and link data.
// This should be called from the signal processing pipeline whenever blobs are detected.
func (h *Handler) UpdateBlobs(blobs []BlobSnapshot, links []LinkState, grid *GridSnapshot, identity map[int]*BLEMatch) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Update fusion result snapshot
	h.fusionResult = &FusionResultSnapshot{
		Timestamp: time.Now().Unix(),
		Blobs:    blobs,
		GridData: grid,
	}

	// Update link states
	for _, link := range links {
		linkID := link.NodeMAC + ":" + link.PeerMAC
		h.linkStates[linkID] = &link
	}

	// Generate explanations for each blob
	for _, blob := range blobs {
		explanation := h.computeExplanation(blob, links, grid)
		if bleMatch := identity[blob.ID]; bleMatch != nil {
			explanation.BLEMatch = bleMatch
		}
		h.blobHistory[blob.ID] = explanation
		// Store by timestamp for feedback lookups
		timestamp := time.Now().UnixMilli()
		explanation.Timestamp = timestamp
		h.blobHistoryByTime[timestamp] = explanation
	}

	// Clean up old blob history (keep last 100)
	if len(h.blobHistory) > 100 {
		// Remove oldest entries (simple FIFO by recreating map with last 100)
		// In practice, blob IDs are incrementing, so we can remove IDs < current - 100
		var maxID int
		for id := range h.blobHistory {
			if id > maxID {
				maxID = id
			}
		}
		cutoff := maxID - 100
		for id := range h.blobHistory {
			if id < cutoff {
				delete(h.blobHistory, id)
			}
		}
	}
}

// RegisterRoutes registers the explainability API routes.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/explain/{blobID}", h.explainBlob)
	r.Post("/api/explain/refresh", h.refreshData)
	r.Get("/api/explain/blob/{blobID}/at/{timestamp}", h.explainBlobAtTime)
}

// explainBlob handles GET /api/explain/{blobID}
func (h *Handler) explainBlob(w http.ResponseWriter, r *http.Request) {
	blobIDStr := chi.URLParam(r, "blobID")
	blobID, err := strconv.Atoi(blobIDStr)
	if err != nil {
		http.Error(w, "Invalid blob ID", http.StatusBadRequest)
		return
	}

	h.mu.RLock()
	explanation, ok := h.blobHistory[blobID]
	h.mu.RUnlock()

	if !ok {
		// Return empty explanation for unknown blob
		explanation = &BlobExplanation{
			BlobID: blobID,
			X:      0,
			Y:      0,
			Z:      0,
			Confidence: 0,
		}
	}

	writeJSON(w, explanation)
}

// explainBlobAtTime handles GET /api/explain/blob/{blobID}/at/{timestamp}
// Returns the explainability snapshot for a blob at or near a specific timestamp.
// This is used by the feedback system to explain why a detection occurred.
func (h *Handler) explainBlobAtTime(w http.ResponseWriter, r *http.Request) {
	blobIDStr := chi.URLParam(r, "blobID")
	blobID, err := strconv.Atoi(blobIDStr)
	if err != nil {
		http.Error(w, "Invalid blob ID", http.StatusBadRequest)
		return
	}

	timestampStr := chi.URLParam(r, "timestamp")
	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid timestamp", http.StatusBadRequest)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	// First try to get by blob ID directly
	if explanation, ok := h.blobHistory[blobID]; ok {
		// Check if the timestamp is close (within 1 minute)
		if abs64(explanation.Timestamp-timestamp) < 60000 {
			writeJSON(w, explanation)
			return
		}
	}

	// If not found by blob ID or timestamp mismatch, search by timestamp
	// Find the closest explanation within 1 minute
	var closest *BlobExplanation
	minDiff := int64(60000) // 1 minute

	for _, exp := range h.blobHistoryByTime {
		diff := abs64(exp.Timestamp - timestamp)
		if diff < minDiff {
			minDiff = diff
			closest = exp
		}
	}

	if closest != nil {
		writeJSON(w, closest)
		return
	}

	// Return empty explanation if nothing found
	explanation := &BlobExplanation{
		BlobID:     blobID,
		X:          0,
		Y:          0,
		Z:          0,
		Confidence: 0,
		Timestamp:  timestamp,
	}
	writeJSON(w, explanation)
}

// abs64 returns the absolute value of an int64.
func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// GetExplanationForBlob retrieves the explainability snapshot for a blob at or near a specific timestamp.
// This is a public method used by other handlers (like feedback) to access explainability data.
func (h *Handler) GetExplanationForBlob(blobID int, timestamp int64) *BlobExplanation {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// First try to get by blob ID directly
	if explanation, ok := h.blobHistory[blobID]; ok {
		// Check if the timestamp is close (within 1 minute)
		if abs64(explanation.Timestamp-timestamp) < 60000 {
			return explanation
		}
	}

	// If not found by blob ID or timestamp mismatch, search by timestamp
	// Find the closest explanation within 1 minute
	var closest *BlobExplanation
	minDiff := int64(60000) // 1 minute

	for _, exp := range h.blobHistoryByTime {
		diff := abs64(exp.Timestamp - timestamp)
		if diff < minDiff {
			minDiff = diff
			closest = exp
		}
	}

	if closest != nil {
		return closest
	}

	// Return nil if nothing found
	return nil
}

// refreshData handles POST /api/explain/refresh
// This is called by the dashboard to refresh the explainability data.
func (h *Handler) refreshData(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Blobs       []BlobSnapshot    `json:"blobs"`
		Links       []LinkState      `json:"links"`
		GridData    *GridSnapshot    `json:"grid_data,omitempty"`
		Identity    map[int]*BLEMatch `json:"identity,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Update fusion result snapshot
	h.fusionResult = &FusionResultSnapshot{
		Timestamp: int64(req.GridData.Rows), // placeholder
		Blobs:    req.Blobs,
		GridData: req.GridData,
	}

	// Update link states
	for _, link := range req.Links {
		linkID := link.NodeMAC + ":" + link.PeerMAC
		h.linkStates[linkID] = &link
	}

	// Generate explanations for each blob
	for _, blob := range req.Blobs {
		explanation := h.computeExplanation(blob, req.Links, req.GridData)
		if bleMatch := req.Identity[blob.ID]; bleMatch != nil {
			explanation.BLEMatch = bleMatch
		}
		h.blobHistory[blob.ID] = explanation
	}

	writeJSON(w, map[string]interface{}{
		"updated": len(h.blobHistory),
	})
}

// computeExplanation calculates the explanation for a single blob.
func (h *Handler) computeExplanation(blob BlobSnapshot, links []LinkState, grid *GridSnapshot) *BlobExplanation {
	if grid == nil {
		return &BlobExplanation{
			BlobID:     blob.ID,
			X:          blob.X,
			Y:          blob.Y,
			Z:          blob.Z,
			Confidence: blob.Confidence,
			AllLinks:    []LinkContribution{},
		}
	}

	explanation := &BlobExplanation{
		BlobID:            blob.ID,
		X:                 blob.X,
		Y:                 blob.Y,
		Z:                 blob.Z,
		Confidence:        blob.Confidence,
		ContributingLinks: []LinkContribution{},
		AllLinks:           make([]LinkContribution, 0, len(links)),
		FresnelZones:      []FresnelZone{},
	}

	// WiFi wavelength constant (2.4 GHz -> ~0.123 m)
	const lambda = 0.123
	const halfLambda = lambda / 2

	// Compute contribution for each link
	for _, link := range links {
		linkID := link.NodeMAC + ":" + link.PeerMAC

		// Get node positions
		nodePos := [3]float64{link.NodePos[0], link.NodePos[1], link.NodePos[2]}
		peerPos := [3]float64{link.PeerPos[0], link.PeerPos[1], link.PeerPos[2]}

		// Calculate path length excess at blob position
		pathDirect := math.Sqrt(
			math.Pow(peerPos[0]-nodePos[0], 2) +
				math.Pow(peerPos[1]-nodePos[1], 2) +
				math.Pow(peerPos[2]-nodePos[2], 2))
		pathViaBlob := math.Sqrt(
			math.Pow(blob.X-nodePos[0], 2) +
				math.Pow(blob.Y-nodePos[1], 2) +
				math.Pow(blob.Z-nodePos[2], 2)) +
			math.Sqrt(
				math.Pow(peerPos[0]-blob.X, 2) +
					math.Pow(peerPos[1]-blob.Y, 2) +
					math.Pow(peerPos[2]-blob.Z, 2))
		deltaL := pathViaBlob - pathDirect

		// Fresnel zone number
		zoneNumber := int(math.Ceil(deltaL / halfLambda))
		if zoneNumber < 1 {
			zoneNumber = 1
		}

		// Zone decay function (default decay_rate = 2.0)
		zoneDecay := 1.0 / math.Pow(float64(zoneNumber), 2.0)

		// Contribution = deltaRMS * weight * zoneDecay
		contribution := link.DeltaRMS * link.Weight * zoneDecay

		linkContrib := LinkContribution{
			LinkID:      linkID,
			NodeMAC:     link.NodeMAC,
			PeerMAC:     link.PeerMAC,
			DeltaRMS:    link.DeltaRMS,
			ZoneNumber:  zoneNumber,
			Weight:      link.Weight,
			Contributing: link.Motion && link.DeltaRMS > 0.02,
			Contribution: contribution,
		}

		explanation.AllLinks = append(explanation.AllLinks, linkContrib)

		if link.Motion && link.DeltaRMS > 0.02 {
			explanation.ContributingLinks = append(explanation.ContributingLinks, linkContrib)
		}

		// Add Fresnel zone ellipsoid data for contributing links
		if link.Motion && link.DeltaRMS > 0.02 {
			// Compute ellipsoid parameters
			// Center is midpoint between nodes
			centerX := (nodePos[0] + peerPos[0]) / 2
			centerY := (nodePos[1] + peerPos[1]) / 2
			centerZ := (nodePos[2] + peerPos[2]) / 2

			// Semi-axes approximation for first Fresnel zone
			// The ellipsoid is roughly centered at the midpoint, with the
			// long axis along the link direction
			linkLength := pathDirect
			longAxis := linkLength / 2
			// Width of Fresnel zone at this distance
			zoneWidth := 2 * math.Sqrt(math.Pow(lambda*float64(zoneNumber)/2, 2) -
				math.Pow(float64(zoneNumber-1)*lambda/2, 2))

			explanation.FresnelZones = append(explanation.FresnelZones, FresnelZone{
				LinkID:     linkID,
				CenterPos:  [3]float64{centerX, centerY, centerZ},
				SemiAxes:   [3]float64{longAxis, zoneWidth, zoneWidth},
				ZoneNumber: zoneNumber,
			})
		}
	}

	return explanation
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	data, err := json.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(data)
}
