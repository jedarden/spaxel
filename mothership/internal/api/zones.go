// Package api provides REST API handlers for Spaxel zones and portals.
package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi"
	"github.com/spaxel/mothership/internal/dashboard"
	"github.com/spaxel/mothership/internal/zones"
)

// ZonesHandler manages zones and portals via the zones.Manager.
// Changes to zones and portals are immediately broadcast to dashboard clients
// via the ZoneChangeBroadcaster, and also reflected in the next delta tick.
type ZonesHandler struct {
	mu  sync.RWMutex
	mgr *zones.Manager
	bc  dashboard.ZoneChangeBroadcaster
}

// zoneWithOcc extends a zone with current occupancy and people list for API responses.
type zoneWithOcc struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Color     string    `json:"color,omitempty"`
	MinX      float64   `json:"x"`
	MinY      float64   `json:"y"`
	MinZ      float64   `json:"z"`
	MaxX      float64   `json:"max_x"`
	MaxY      float64   `json:"max_y"`
	MaxZ      float64   `json:"max_z"`
	Width     float64   `json:"w"`
	Depth     float64   `json:"d"`
	Height    float64   `json:"h"`
	Enabled   bool      `json:"enabled"`
	ZoneType  string    `json:"zone_type"`
	Occupancy int       `json:"occupancy"`
	People    []string  `json:"people"`
	CreatedAt time.Time `json:"created_at"`
}

// portalWithZones extends a portal with resolved zone names for API responses.
type portalWithZones struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	ZoneA     string    `json:"zone_a"`
	ZoneB     string    `json:"zone_b"`
	P1X       float64   `json:"p1_x"`
	P1Y       float64   `json:"p1_y"`
	P1Z       float64   `json:"p1_z"`
	P2X       float64   `json:"p2_x"`
	P2Y       float64   `json:"p2_y"`
	P2Z       float64   `json:"p2_z"`
	P3X       float64   `json:"p3_x"`
	P3Y       float64   `json:"p3_y"`
	P3Z       float64   `json:"p3_z"`
	NX        float64   `json:"n_x"`
	NY        float64   `json:"n_y"`
	NZ        float64   `json:"n_z"`
	Width     float64   `json:"width"`
	Height    float64   `json:"height"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

// crossingResponse is a single crossing event as returned by the API.
type crossingResponse struct {
	ID        int64     `json:"id"`
	PortalID  string    `json:"portal_id"`
	BlobID    int       `json:"blob_id"`
	Direction string    `json:"direction"`
	FromZone  string    `json:"from_zone"`
	ToZone    string    `json:"to_zone"`
	Timestamp time.Time `json:"timestamp"`
	Person    string    `json:"person,omitempty"`
}

// NewZonesHandler creates a new zones handler backed by a zones.Manager.
func NewZonesHandler(mgr *zones.Manager) *ZonesHandler {
	return &ZonesHandler{mgr: mgr}
}

// SetZoneChangeBroadcaster sets the broadcaster for immediate WebSocket
// notifications when zones or portals are modified.
func (h *ZonesHandler) SetZoneChangeBroadcaster(bc dashboard.ZoneChangeBroadcaster) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.bc = bc
}

// notifyZoneChange broadcasts a zone change event if a broadcaster is set.
func (h *ZonesHandler) notifyZoneChange(action string, z *zones.Zone) {
	h.mu.RLock()
	bc := h.bc
	h.mu.RUnlock()
	if bc == nil {
		return
	}
	occ := h.mgr.GetZoneOccupancy(z.ID)
	count := 0
	if occ != nil {
		count = occ.Count
	}
	bc.BroadcastZoneChange(action, dashboard.ZoneSnapshot{
		ID:    z.ID,
		Name:  z.Name,
		Count: count,
		MinX:  z.MinX,
		MinY:  z.MinY,
		MinZ:  z.MinZ,
		SizeX: z.MaxX - z.MinX,
		SizeY: z.MaxY - z.MinY,
		SizeZ: z.MaxZ - z.MinZ,
	})
}

// notifyPortalChange broadcasts a portal change event if a broadcaster is set.
func (h *ZonesHandler) notifyPortalChange(action string, p *zones.Portal) {
	h.mu.RLock()
	bc := h.bc
	h.mu.RUnlock()
	if bc == nil {
		return
	}
	bc.BroadcastPortalChange(action, dashboard.PortalSnapshot{
		ID:      p.ID,
		Name:    p.Name,
		ZoneA:   p.ZoneAID,
		ZoneB:   p.ZoneBID,
		P1X:     p.P1X,
		P1Y:     p.P1Y,
		P1Z:     p.P1Z,
		P2X:     p.P2X,
		P2Y:     p.P2Y,
		P2Z:     p.P2Z,
		P3X:     p.P3X,
		P3Y:     p.P3Y,
		P3Z:     p.P3Z,
		NX:      p.NX,
		NY:      p.NY,
		NZ:      p.NZ,
		Width:   p.Width,
		Height:  p.Height,
		Enabled: p.Enabled,
	})
}

// Close closes the underlying manager.
func (h *ZonesHandler) Close() error {
	return h.mgr.Close()
}

// RegisterRoutes registers zones and portals endpoints.
//
// Zones:
//
//	GET /api/zones
//
//	@Summary		List all zones
//		@Description	Returns all defined spatial zones with current occupancy counts.
//	@Tags			zones
//	@Produce		json
//	@Success		200	{array}		zoneWithOcc	"List of zones"
//	@Router			/api/zones [get]
//
//	POST /api/zones
//
//	@Summary		Create a zone
//	@Description	Creates a new spatial zone. If no ID is provided, one is auto-generated.
//	@Tags			zones
//	@Accept			json
//	@Produce		json
//	@Param			zone	body		zones.Zone	true	"Zone definition"
//	@Success		201	{object}	zones.Zone	"Created zone"
//	@Failure		400	{object}	map[string]string	"Invalid request body"
//	@Router			/api/zones [post]
//
//	PUT /api/zones/{id}
//
//	@Summary		Update a zone
//	@Description	Updates an existing zone's properties. All fields are replaced.
//	@Tags			zones
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string		true	"Zone ID"
//	@Param			zone	body		zones.Zone	true	"Updated zone definition"
//	@Success		200	{object}	zones.Zone	"Updated zone"
//	@Failure		404	{object}	map[string]string	"Zone not found"
//	@Router			/api/zones/{id} [put]
//
//	DELETE /api/zones/{id}
//
//	@Summary		Delete a zone
//	@Description	Deletes a zone and removes it from the floor plan.
//	@Tags			zones
//	@Param			id		path		string	true	"Zone ID"
//	@Success		204		"Zone deleted"
//	@Router			/api/zones/{id} [delete]
//
//	GET /api/zones/{id}/history
//
//	@Summary		Zone occupancy history
//	@Description	Returns hourly occupancy history for a zone.
//	@Tags			zones
//	@Produce		json
//	@Param			id		path		string	true	"Zone ID"
//	@Param			period	query		string	false	"Time period: 24h (default), 7d, 30d"
//	@Success		200	{array}		historyEntry	"Hourly occupancy buckets"
//	@Failure		404	{object}	map[string]string	"Zone not found"
//	@Router			/api/zones/{id}/history [get]
//
// Portals:
//
//	GET /api/portals
//
//	@Summary		List all portals
//	@Description	Returns all doorway portals with computed normal vectors.
//	@Tags			portals
//	@Produce		json
//	@Success		200	{array}		portalWithZones	"List of portals"
//	@Router			/api/portals [get]
//
//	POST /api/portals
//
//	@Summary		Create a portal
//	@Description	Creates a new doorway portal between two zones. Normal vector is auto-computed from the three defining points.
//	@Tags			portals
//	@Accept			json
//	@Produce		json
//	@Param			portal	body		zones.Portal	true	"Portal definition"
//	@Success		201	{object}	zones.Portal	"Created portal"
//	@Failure		400	{object}	map[string]string	"Invalid request body"
//	@Router			/api/portals [post]
//
//	PUT /api/portals/{id}
//
//	@Summary		Update a portal
//	@Description	Updates an existing portal's properties. Normal vector is recomputed if points change.
//	@Tags			portals
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string		true	"Portal ID"
//	@Param			portal	body		zones.Portal	true	"Updated portal definition"
//	@Success		200	{object}	zones.Portal	"Updated portal"
//	@Failure		404	{object}	map[string]string	"Portal not found"
//	@Router			/api/portals/{id} [put]
//
//	DELETE /api/portals/{id}
//
//	@Summary		Delete a portal
//	@Description	Deletes a portal and its crossing history.
//	@Tags			portals
//	@Param			id		path		string	true	"Portal ID"
//	@Success		204		"Portal deleted"
//	@Router			/api/portals/{id} [delete]
//
//	GET /api/portals/{id}/crossings
//
//	@Summary		Portal crossing log
//	@Description	Returns recent directional crossings for a portal.
//	@Tags			portals
//	@Produce		json
//	@Param			id		path		string	true	"Portal ID"
//	@Param			limit	query		int		false	"Max crossings to return (default: 50)"
//	@Success		200	{array}		crossingResponse	"Crossing events"
//	@Failure		404	{object}	map[string]string	"Portal not found"
//	@Router			/api/portals/{id}/crossings [get]
func (h *ZonesHandler) RegisterRoutes(r chi.Router) {
	// Zones
	r.Get("/api/zones", h.listZones)
	r.Post("/api/zones", h.createZone)
	r.Put("/api/zones/{id}", h.updateZone)
	r.Delete("/api/zones/{id}", h.deleteZone)
	r.Get("/api/zones/{id}/history", h.getZoneHistory)

	// Portals
	r.Get("/api/portals", h.listPortals)
	r.Post("/api/portals", h.createPortal)
	r.Put("/api/portals/{id}", h.updatePortal)
	r.Delete("/api/portals/{id}", h.deletePortal)
	r.Get("/api/portals/{id}/crossings", h.getPortalCrossings)
}

// toZoneResponse converts a zones.Zone to the API response format with occupancy.
func (h *ZonesHandler) toZoneResponse(z *zones.Zone) zoneWithOcc {
	occ := h.mgr.GetZoneOccupancy(z.ID)
	count := 0
	if occ != nil {
		count = occ.Count
	}
	return zoneWithOcc{
		ID:        z.ID,
		Name:      z.Name,
		Color:     z.Color,
		MinX:      z.MinX,
		MinY:      z.MinY,
		MinZ:      z.MinZ,
		MaxX:      z.MaxX,
		MaxY:      z.MaxY,
		MaxZ:      z.MaxZ,
		Width:     z.MaxX - z.MinX,
		Depth:     z.MaxY - z.MinY,
		Height:    z.MaxZ - z.MinZ,
		Enabled:   z.Enabled,
		ZoneType:  string(z.ZoneType),
		Occupancy: count,
		People:    []string{},
		CreatedAt: z.CreatedAt,
	}
}

// toPortalResponse converts a zones.Portal to the API response format.
func toPortalResponse(p *zones.Portal) portalWithZones {
	return portalWithZones{
		ID:        p.ID,
		Name:      p.Name,
		ZoneA:     p.ZoneAID,
		ZoneB:     p.ZoneBID,
		P1X:       p.P1X,
		P1Y:       p.P1Y,
		P1Z:       p.P1Z,
		P2X:       p.P2X,
		P2Y:       p.P2Y,
		P2Z:       p.P2Z,
		P3X:       p.P3X,
		P3Y:       p.P3Y,
		P3Z:       p.P3Z,
		NX:        p.NX,
		NY:        p.NY,
		NZ:        p.NZ,
		Width:     p.Width,
		Height:    p.Height,
		Enabled:   p.Enabled,
		CreatedAt: p.CreatedAt,
	}
}

// ── Zones ───────────────────────────────────────────────────────────────────────

// listZones returns all zones with current occupancy.
func (h *ZonesHandler) listZones(w http.ResponseWriter, r *http.Request) {
	allZones := h.mgr.GetAllZones()
	h.mu.RLock()
	defer h.mu.RUnlock()

	response := make([]zoneWithOcc, 0, len(allZones))
	for _, z := range allZones {
		response = append(response, h.toZoneResponse(z))
	}

	writeJSON(w, http.StatusOK, response)
}

// createZone creates a new zone. Auto-generates an ID if none is provided.
func (h *ZonesHandler) createZone(w http.ResponseWriter, r *http.Request) {
	var zone zones.Zone
	if err := json.NewDecoder(r.Body).Decode(&zone); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if zone.ID == "" {
		zone.ID = "zone_" + time.Now().Format("20060102-150405")
	}

	if zone.Name == "" {
		writeJSONError(w, http.StatusBadRequest, "name is required")
		return
	}

	// Set defaults for color
	if zone.Color == "" {
		zone.Color = "#4fc3f7"
	}

	if err := h.mgr.CreateZone(&zone); err != nil {
		http.Error(w, "failed to create zone: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.mu.RLock()
	resp := h.toZoneResponse(h.mgr.GetZone(zone.ID))
	h.mu.RUnlock()

	log.Printf("[INFO] Zone created: %s (%s)", zone.ID, zone.Name)
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, http.StatusCreated, resp)
}

// updateZone updates an existing zone.
func (h *ZonesHandler) updateZone(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if h.mgr.GetZone(id) == nil {
		writeJSONError(w, http.StatusNotFound, "zone not found")
		return
	}

	var zone zones.Zone
	if err := json.NewDecoder(r.Body).Decode(&zone); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	// Preserve the ID from the URL param
	zone.ID = id

	if err := h.mgr.UpdateZone(&zone); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to update zone: "+err.Error())
		return
	}

	h.mu.RLock()
	resp := h.toZoneResponse(h.mgr.GetZone(zone.ID))
	h.mu.RUnlock()

	log.Printf("[INFO] Zone updated: %s (%s)", zone.ID, zone.Name)
	writeJSON(w, http.StatusOK, resp)
}

// deleteZone removes a zone by ID.
func (h *ZonesHandler) deleteZone(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.mgr.DeleteZone(id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to delete zone: "+err.Error())
		return
	}

	log.Printf("[INFO] Zone deleted: %s", id)
	w.WriteHeader(http.StatusNoContent)
}

// historyEntry represents an hourly occupancy bucket for the zone history API.
type historyEntry struct {
	Timestamp int64    `json:"timestamp"`
	Count     int      `json:"count"`
	People   []string `json:"people"`
}

// getZoneHistory returns hourly occupancy history for a zone.
func (h *ZonesHandler) getZoneHistory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if h.mgr.GetZone(id) == nil {
		writeJSONError(w, http.StatusNotFound, "zone not found")
		return
	}

	period := r.URL.Query().Get("period")
	limit := 24
	if period == "7d" {
		limit = 24 * 7
	} else if period == "30d" {
		limit = 24 * 30
	}

	// Generate synthetic history data (in real implementation, query from events)
	history := make([]historyEntry, limit)
	now := time.Now()
	for i := range history {
		history[i] = historyEntry{
			Timestamp: now.Add(-time.Duration(i) * time.Hour).UnixNano() / 1e6,
			Count:     0,
			People:    []string{},
		}
	}

	writeJSON(w, http.StatusOK, history)
}

// ── Portals ─────────────────────────────────────────────────────────────────────

// listPortals returns all portals.
func (h *ZonesHandler) listPortals(w http.ResponseWriter, r *http.Request) {
	allPortals := h.mgr.GetAllPortals()

	response := make([]portalWithZones, 0, len(allPortals))
	for _, p := range allPortals {
		response = append(response, toPortalResponse(p))
	}

	writeJSON(w, http.StatusOK, response)
}

// createPortal creates a new portal. Auto-generates an ID if none is provided.
func (h *ZonesHandler) createPortal(w http.ResponseWriter, r *http.Request) {
	var portal zones.Portal
	if err := json.NewDecoder(r.Body).Decode(&portal); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if portal.ID == "" {
		portal.ID = "portal_" + time.Now().Format("20060102-150405")
	}

	// Validate zone references
	if portal.ZoneAID != "" && h.mgr.GetZone(portal.ZoneAID) == nil {
		writeJSONError(w, http.StatusBadRequest, "zone_a not found")
		return
	}
	if portal.ZoneBID != "" && h.mgr.GetZone(portal.ZoneBID) == nil {
		writeJSONError(w, http.StatusBadRequest, "zone_b not found")
		return
	}

	if err := h.mgr.CreatePortal(&portal); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to create portal: "+err.Error())
		return
	}

	resp := toPortalResponse(h.mgr.GetPortal(portal.ID))
	log.Printf("[INFO] Portal created: %s (%s)", portal.ID, portal.Name)
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, http.StatusCreated, resp)
}

// updatePortal updates an existing portal.
func (h *ZonesHandler) updatePortal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if h.mgr.GetPortal(id) == nil {
		writeJSONError(w, http.StatusNotFound, "portal not found")
		return
	}

	var portal zones.Portal
	if err := json.NewDecoder(r.Body).Decode(&portal); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	// Preserve the ID from the URL param
	portal.ID = id

	// Validate zone references if changed
	if portal.ZoneAID != "" && h.mgr.GetZone(portal.ZoneAID) == nil {
		writeJSONError(w, http.StatusBadRequest, "zone_a not found")
		return
	}
	if portal.ZoneBID != "" && h.mgr.GetZone(portal.ZoneBID) == nil {
		writeJSONError(w, http.StatusBadRequest, "zone_b not found")
		return
	}

	if err := h.mgr.UpdatePortal(&portal); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to update portal: "+err.Error())
		return
	}

	resp := toPortalResponse(h.mgr.GetPortal(portal.ID))
	log.Printf("[INFO] Portal updated: %s (%s)", portal.ID, portal.Name)
	writeJSON(w, http.StatusOK, resp)
}

// deletePortal removes a portal by ID.
func (h *ZonesHandler) deletePortal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.mgr.DeletePortal(id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to delete portal: "+err.Error())
		return
	}

	log.Printf("[INFO] Portal deleted: %s", id)
	w.WriteHeader(http.StatusNoContent)
}

// getPortalCrossings returns recent crossing events for a portal.
func (h *ZonesHandler) getPortalCrossings(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if h.mgr.GetPortal(id) == nil {
		writeJSONError(w, http.StatusNotFound, "portal not found")
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	events := h.mgr.GetRecentCrossings(limit)
	writeJSON(w, http.StatusOK, events)
}
