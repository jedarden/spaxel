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
	identMgr     IdentityProvider
	startTime    time.Time
	getNodeCount func() int
	version      string
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

// IdentityProvider provides identity resolution for blobs.
type IdentityProvider interface {
	// GetMatch returns identity information for a blob, or nil if no match.
	GetMatch(blobID int) *IdentityMatch
}

// IdentityMatch represents identity information for a blob.
// This mirrors the relevant fields from ble.IdentityMatch.
type IdentityMatch struct {
	PersonName string `json:"person_name,omitempty"`
	PersonID   string `json:"person_id,omitempty"`
	DeviceName string `json:"device_name,omitempty"`
	IsBLEOnly  bool   `json:"is_ble_only,omitempty"`
}

// NewStatusHandler creates a new status handler.
//
// version is the build-time application version (injected via ldflags
// `-X main.version` in cmd/mothership/main.go) surfaced via GET /api/status.
func NewStatusHandler(startTime time.Time, getNodeCount func() int, version string) *StatusHandler {
	return &StatusHandler{
		startTime:    startTime,
		getNodeCount: getNodeCount,
		version:      version,
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

// SetIdentityProvider sets the identity provider.
func (h *StatusHandler) SetIdentityProvider(im IdentityProvider) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.identMgr = im
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
		"version":           h.version,
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

		// Resolve blob IDs to person names using the identity provider
		people := h.resolvePeopleIDs(occ.BlobIDs)

		result[z.Name] = occupancyResponse{
			Count:  occ.Count,
			People: people,
		}
	}

	writeJSON(w, http.StatusOK, result)
}

// resolvePeopleIDs converts a list of blob IDs to a list of person names.
// Only includes blobs with BLE-identified persons. Unidentified blobs are omitted.
func (h *StatusHandler) resolvePeopleIDs(blobIDs []int) []string {
	if len(blobIDs) == 0 || h.identMgr == nil {
		return []string{}
	}

	// Use a map to deduplicate person names
	peopleMap := make(map[string]bool)
	for _, blobID := range blobIDs {
		if match := h.identMgr.GetMatch(blobID); match != nil {
			// Use PersonName if available (BLE-identified person), otherwise DeviceName
			// Skip unidentified blobs (match.PersonName == "" && match.DeviceName == "")
			personName := match.PersonName
			if personName == "" {
				personName = match.DeviceName
			}
			if personName != "" {
				peopleMap[personName] = true
			}
		}
	}

	// Convert map to slice
	result := make([]string, 0, len(peopleMap))
	for person := range peopleMap {
		result = append(result, person)
	}
	return result
}
