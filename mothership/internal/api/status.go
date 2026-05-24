// Package api provides REST API handlers for Spaxel system status and occupancy.
package api

import (
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/spaxel/mothership/internal/signal"
	"github.com/spaxel/mothership/internal/zones"
)

// StatusHandler handles GET /api/status and GET /api/occupancy.
type StatusHandler struct {
	mu           sync.RWMutex
	pm           ProcessorManagerProvider
	zonesMgr     ZonesManagerProvider
	startTime    time.Time
	getNodeCount func() int
}

// ProcessorManagerProvider provides access to signal processor data.
type ProcessorManagerProvider interface {
	GetSystemHealth() float64
	GetTrackedBlobs() []signal.TrackedBlob
}

// ZonesManagerProvider provides access to zone data.
type ZonesManagerProvider interface {
	GetAllZones() []*zones.Zone
	GetZoneOccupancy(zoneID string) *zones.ZoneOccupancy
}

// NewStatusHandler creates a new status handler.
func NewStatusHandler(startTime time.Time, getNodeCount func() int) *StatusHandler {
	return &StatusHandler{
		startTime:    startTime,
		getNodeCount: getNodeCount,
	}
}

// SetProcessorManager sets the signal processor manager.
func (h *StatusHandler) SetProcessorManager(pm ProcessorManagerProvider) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pm = pm
}

// SetZonesManager sets the zones manager.
func (h *StatusHandler) SetZonesManager(zm ZonesManagerProvider) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.zonesMgr = zm
}

// RegisterRoutes registers status and occupancy endpoints.
func (h *StatusHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/status", h.getStatus)
	r.Get("/api/occupancy", h.getOccupancy)
}

// getStatus handles GET /api/status.
//
// Returns:
//   - version: Application version string
//   - nodes: Number of online nodes
//   - blobs: Number of currently tracked blobs
//   - uptime_s: Uptime in seconds
//   - detection_quality: System-wide detection quality (0-100)
func (h *StatusHandler) getStatus(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Get node count
	nodes := 0
	if h.getNodeCount != nil {
		nodes = h.getNodeCount()
	}

	// Get blob count
	blobs := 0
	if h.pm != nil {
		blobs = len(h.pm.GetTrackedBlobs())
	}

	// Get uptime
	uptime := int64(time.Since(h.startTime).Seconds())

	// Get detection quality (0-100 scale)
	quality := 0.0
	if h.pm != nil {
		// GetSystemHealth returns 0-1, convert to 0-100
		quality = h.pm.GetSystemHealth() * 100
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"version":           "1.0.0", // TODO: get from build info
		"nodes":             nodes,
		"blobs":             blobs,
		"uptime_s":          uptime,
		"detection_quality": int(quality), // Convert to int for cleaner JSON
	})
}

// occupancyResponse represents the occupancy data for a single zone.
type occupancyResponse struct {
	Count  int      `json:"count"`
	People []string `json:"people"`
}

// getOccupancy handles GET /api/occupancy.
//
// Returns a map of zone names to their current occupancy:
//   - count: Number of people in the zone
//   - people: List of person names (BLE-identified) in the zone
func (h *StatusHandler) getOccupancy(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make(map[string]occupancyResponse)

	if h.zonesMgr == nil {
		writeJSON(w, http.StatusOK, result)
		return
	}

	// Get all zones
	allZones := h.zonesMgr.GetAllZones()

	for _, z := range allZones {
		occ := h.zonesMgr.GetZoneOccupancy(z.ID)
		if occ == nil {
			result[z.Name] = occupancyResponse{
				Count:  0,
				People: []string{},
			}
			continue
		}

		// Convert blob IDs to person labels if we have a processor manager
		// For now, return empty people list - identity resolution requires
		// additional provider interface
		result[z.Name] = occupancyResponse{
			Count:  occ.Count,
			People: []string{}, // TODO: resolve blob IDs to person labels
		}
	}

	writeJSON(w, http.StatusOK, result)
}
