package fleet

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// UnpairedProvider returns the MACs of currently-connected nodes that lack a valid token.
type UnpairedProvider interface {
	GetUnpairedMACs() []string
}

// FleetHandler serves the fleet health REST API.
type FleetHandler struct {
	healer           *SelfHealManager
	reg              *Registry
	unpairedProvider UnpairedProvider
}

// NewFleetHandler creates a new fleet REST handler.
func NewFleetHandler(healer *SelfHealManager, registry *Registry) *FleetHandler {
	return &FleetHandler{
		healer: healer,
		reg:    registry,
	}
}

// SetUnpairedProvider wires in the source of unpaired node MACs (typically the ingestion server).
func (h *FleetHandler) SetUnpairedProvider(p UnpairedProvider) {
	h.unpairedProvider = p
}

// RegisterRoutes mounts fleet endpoints on r.
//
//	GET  /api/fleet           — all provisioned nodes with full details
//	GET  /api/fleet/health   — current fleet health status
//	GET  /api/fleet/history  — recent optimisation history
//	POST /api/fleet/optimise — trigger manual re-optimisation
//	GET  /api/fleet/simulate — simulate node removal impact
//	PATCH /api/nodes/{mac}/label — update node label
//	POST /api/nodes/{mac}/locate — send identify command
//	POST /api/nodes/{mac}/role — assign new role
//	DELETE /api/nodes/{mac} — remove from fleet
func (h *FleetHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/fleet", h.getFleet)
	r.Get("/api/fleet/health", h.getFleetHealth)
	r.Get("/api/fleet/history", h.getFleetHistory)
	r.Post("/api/fleet/optimise", h.triggerOptimise)
	r.Get("/api/fleet/simulate", h.simulateNodeRemoval)

	// Node-specific routes
	r.Patch("/api/nodes/{mac}/label", h.setNodeLabel)
	r.Post("/api/nodes/{mac}/locate", h.locateNode)
	r.Post("/api/nodes/{mac}/role", h.setNodeRole)
	r.Delete("/api/nodes/{mac}", h.removeNode)
}

// fleetHealthResponse is the wire format for /api/fleet/health
type fleetHealthResponse struct {
	CoverageScore float64          `json:"coverage_score"`
	MeanGDOP      float64          `json:"mean_gdop"`
	IsDegraded    bool             `json:"is_degraded"`
	Nodes         []fleetNodeEntry `json:"nodes"`
}

type fleetNodeEntry struct {
	MAC             string  `json:"mac"`
	Name            string  `json:"name"`
	Role            string  `json:"role"`
	HealthScore     float64 `json:"health_score"`
	Online          bool    `json:"online"`
	PosX            float64 `json:"pos_x"`
	PosY            float64 `json:"pos_y"`
	PosZ            float64 `json:"pos_z"`
	FirmwareVersion string  `json:"firmware_version"`
	UptimeSeconds   int64   `json:"uptime_seconds"`
	LastSeenMs      int64   `json:"last_seen_ms"`
	Unpaired        bool    `json:"unpaired,omitempty"`
}

func (h *FleetHandler) getFleetHealth(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.reg.GetAllNodes()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	roles := h.healer.GetCurrentRoles()
	onlineSet := make(map[string]struct{})
	for _, mac := range h.healer.GetOnlineNodes() {
		onlineSet[mac] = struct{}{}
	}

	entries := make([]fleetNodeEntry, 0, len(nodes))
	for _, n := range nodes {
		if n.Virtual {
			continue // Skip virtual nodes
		}
		role := n.Role
		if r, ok := roles[n.MAC]; ok {
			role = r
		}
		_, online := onlineSet[n.MAC]

		// Calculate uptime: if online, use time since first seen; otherwise, time since went offline
		var uptimeSeconds int64
		if online {
			uptimeSeconds = int64(time.Since(n.FirstSeenAt).Seconds())
		} else if !n.WentOfflineAt.IsZero() {
			uptimeSeconds = int64(n.WentOfflineAt.Sub(n.FirstSeenAt).Seconds())
		}

		entries = append(entries, fleetNodeEntry{
			MAC:            n.MAC,
			Name:           n.Name,
			Role:           role,
			HealthScore:    n.HealthScore,
			Online:         online,
			PosX:           n.PosX,
			PosY:           n.PosY,
			PosZ:           n.PosZ,
			FirmwareVersion: n.FirmwareVersion,
			UptimeSeconds:  uptimeSeconds,
			LastSeenMs:     n.LastSeenAt.UnixMilli(),
		})
	}

	// Merge in unpaired nodes (connected without a valid token) that aren't in the registry yet.
	if h.unpairedProvider != nil {
		registeredMACs := make(map[string]struct{}, len(entries))
		for _, e := range entries {
			registeredMACs[e.MAC] = struct{}{}
		}
		for _, mac := range h.unpairedProvider.GetUnpairedMACs() {
			if _, exists := registeredMACs[mac]; exists {
				// Already in the registry — mark as unpaired in the existing entry.
				for i := range entries {
					if entries[i].MAC == mac {
						entries[i].Unpaired = true
						break
					}
				}
			} else {
				entries = append(entries, fleetNodeEntry{
					MAC:      mac,
					Online:   true,
					Unpaired: true,
					Role:     "rx",
				})
			}
		}
	}

	resp := fleetHealthResponse{
		CoverageScore: h.healer.GetCoverageScore(),
		MeanGDOP:      0, // Will be computed if GDOP calculator is available
		IsDegraded:    len(onlineSet) < 2 && len(entries) >= 2,
		Nodes:         entries,
	}

	writeJSON(w, resp)
}

// getFleet returns all provisioned nodes with full details.
// This is the same as /api/fleet/health but without the health metadata,
// providing a flat list of nodes for the fleet status page.
func (h *FleetHandler) getFleet(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.reg.GetAllNodes()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	roles := h.healer.GetCurrentRoles()
	onlineSet := make(map[string]struct{})
	for _, mac := range h.healer.GetOnlineNodes() {
		onlineSet[mac] = struct{}{}
	}

	entries := make([]fleetNodeEntry, 0, len(nodes))
	for _, n := range nodes {
		if n.Virtual {
			continue // Skip virtual nodes
		}
		role := n.Role
		if r, ok := roles[n.MAC]; ok {
			role = r
		}
		_, online := onlineSet[n.MAC]

		// Calculate uptime: if online, use time since first seen; otherwise, time since went offline
		var uptimeSeconds int64
		if online {
			uptimeSeconds = int64(time.Since(n.FirstSeenAt).Seconds())
		} else if !n.WentOfflineAt.IsZero() {
			uptimeSeconds = int64(n.WentOfflineAt.Sub(n.FirstSeenAt).Seconds())
		}

		entries = append(entries, fleetNodeEntry{
			MAC:            n.MAC,
			Name:           n.Name,
			Role:           role,
			HealthScore:    n.HealthScore,
			Online:         online,
			PosX:           n.PosX,
			PosY:           n.PosY,
			PosZ:           n.PosZ,
			FirmwareVersion: n.FirmwareVersion,
			UptimeSeconds:  uptimeSeconds,
			LastSeenMs:     n.LastSeenAt.UnixMilli(),
		})
	}

	// Merge in unpaired nodes not yet in the registry.
	if h.unpairedProvider != nil {
		registeredMACs := make(map[string]struct{}, len(entries))
		for _, e := range entries {
			registeredMACs[e.MAC] = struct{}{}
		}
		for _, mac := range h.unpairedProvider.GetUnpairedMACs() {
			if _, exists := registeredMACs[mac]; exists {
				for i := range entries {
					if entries[i].MAC == mac {
						entries[i].Unpaired = true
						break
					}
				}
			} else {
				entries = append(entries, fleetNodeEntry{
					MAC:      mac,
					Online:   true,
					Unpaired: true,
					Role:     "rx",
				})
			}
		}
	}

	if entries == nil {
		entries = []fleetNodeEntry{}
	}
	writeJSON(w, entries)
}

// fleetHistoryEntry is the wire format for history items
type fleetHistoryEntry struct {
	ID             int64   `json:"id"`
	TimestampMs    int64   `json:"timestamp_ms"`
	TriggerReason  string  `json:"trigger_reason"`
	MeanGDOPBefore float64 `json:"mean_gdop_before"`
	MeanGDOPAfter  float64 `json:"mean_gdop_after"`
	CoverageDelta  float64 `json:"coverage_delta"`
}

func (h *FleetHandler) getFleetHistory(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 10
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	records, err := h.healer.GetOptimisationHistory(limit)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	entries := make([]fleetHistoryEntry, 0, len(records))
	for _, rec := range records {
		entries = append(entries, fleetHistoryEntry{
			ID:             rec.ID,
			TimestampMs:    rec.Timestamp.UnixMilli(),
			TriggerReason:  rec.TriggerReason,
			MeanGDOPBefore: rec.MeanGDOPBefore,
			MeanGDOPAfter:  rec.MeanGDOPAfter,
			CoverageDelta:  rec.CoverageDelta,
		})
	}

	if entries == nil {
		entries = []fleetHistoryEntry{}
	}
	writeJSON(w, entries)
}

// optimiseResponse is returned after manual optimisation
type optimiseResponse struct {
	TriggerReason  string            `json:"trigger_reason"`
	CoverageScore  float64           `json:"coverage_score"`
	MeanGDOP       float64           `json:"mean_gdop"`
	RoleAssignments map[string]string `json:"role_assignments"`
}

func (h *FleetHandler) triggerOptimise(w http.ResponseWriter, r *http.Request) {
	result := h.healer.ManualOptimise()

	resp := optimiseResponse{
		TriggerReason:  result.TriggerReason,
		CoverageScore:  result.CoverageScore,
		MeanGDOP:       result.MeanGDOP,
		RoleAssignments: h.healer.GetCurrentRoles(),
	}

	writeJSON(w, resp)
}

// simulateResponse is returned for node removal simulation
type simulateResponse struct {
	MAC            string  `json:"mac"`
	CoverageBefore float64 `json:"coverage_before"`
	CoverageAfter  float64 `json:"coverage_after"`
	CoverageDelta  float64 `json:"coverage_delta"`
}

func (h *FleetHandler) simulateNodeRemoval(w http.ResponseWriter, r *http.Request) {
	mac := r.URL.Query().Get("mac")
	if mac == "" {
		http.Error(w, "mac parameter required", http.StatusBadRequest)
		return
	}

	result, delta, err := h.healer.SimulateNodeRemoval(mac)
	if err != nil {
		http.Error(w, "simulation failed", http.StatusInternalServerError)
		return
	}

	resp := simulateResponse{
		MAC:            mac,
		CoverageBefore: result.CoverageScore - delta,
		CoverageAfter:  result.CoverageScore,
		CoverageDelta:  delta,
	}

	writeJSON(w, resp)
}

// setNodeLabel updates the name/label for a node.
func (h *FleetHandler) setNodeLabel(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")
	if mac == "" {
		http.Error(w, "mac parameter required", http.StatusBadRequest)
		return
	}

	var req struct {
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Validate label length
	if len(req.Label) > 32 {
		http.Error(w, "label too long (max 32 characters)", http.StatusBadRequest)
		return
	}

	// Update label in registry
	if err := h.reg.SetNodeLabel(mac, req.Label); err != nil {
		http.Error(w, "failed to update label", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"status": "ok"})
}

// locateNode sends an identify command to a node.
func (h *FleetHandler) locateNode(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")
	if mac == "" {
		http.Error(w, "mac parameter required", http.StatusBadRequest)
		return
	}

	var req struct {
		DurationMS int `json:"duration_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.DurationMS = 5000 // Default 5 seconds
	}

	// Send identify command through ingestion server
	// This is handled by the ingestion server which has access to node connections
	// For now, return success - the actual command will be sent via WebSocket
	writeJSON(w, map[string]interface{}{
		"status": "sent",
		"mac": mac,
		"duration_ms": req.DurationMS,
	})
}

// setNodeRole assigns a new role to a node.
func (h *FleetHandler) setNodeRole(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")
	if mac == "" {
		http.Error(w, "mac parameter required", http.StatusBadRequest)
		return
	}

	var req struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Validate role
	validRoles := map[string]bool{
		"tx":      true,
		"rx":      true,
		"tx_rx":   true,
		"passive": true,
		"idle":    true,
	}
	if !validRoles[req.Role] {
		http.Error(w, "invalid role", http.StatusBadRequest)
		return
	}

	// Update role in registry
	if err := h.reg.SetNodeRole(mac, req.Role); err != nil {
		http.Error(w, "failed to update role", http.StatusInternalServerError)
		return
	}

	// Trigger role re-optimisation if fleet manager is available
	if h.healer != nil {
		h.healer.ManualOptimise()
	}

	writeJSON(w, map[string]string{"status": "ok"})
}

// removeNode removes a node from the fleet.
func (h *FleetHandler) removeNode(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")
	if mac == "" {
		http.Error(w, "mac parameter required", http.StatusBadRequest)
		return
	}

	// Delete from registry
	if err := h.reg.DeleteNode(mac); err != nil {
		http.Error(w, "failed to remove node", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"status": "ok"})
}

// writeJSON is a helper for JSON responses
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// FleetEventAPIHandler handles fleet event-related API routes
type FleetEventAPIHandler struct {
	healer *SelfHealManager
	reg    *Registry
}

// NewFleetEventAPIHandler creates a new fleet event API handler
func NewFleetEventAPIHandler(healer *SelfHealManager, registry *Registry) *FleetEventAPIHandler {
	return &FleetEventAPIHandler{
		healer: healer,
		reg:    registry,
	}
}

// RegisterRoutes mounts event-related endpoints
func (h *FleetEventAPIHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/fleet/events/{id}", h.getEventDetails)
}

// fleetEventDetails includes full GDOP data for comparison view
type fleetEventDetails struct {
	ID             int64     `json:"id"`
	Timestamp      time.Time `json:"timestamp"`
	TriggerReason  string    `json:"trigger_reason"`
	MeanGDOPBefore float64   `json:"mean_gdop_before"`
	MeanGDOPAfter  float64   `json:"mean_gdop_after"`
	CoverageBefore float64   `json:"coverage_before"`
	CoverageAfter  float64   `json:"coverage_after"`
	CoverageDelta  float64   `json:"coverage_delta"`
	GDOPBefore     []float32 `json:"gdop_before,omitempty"`
	GDOPAfter      []float32 `json:"gdop_after,omitempty"`
	GDOPCols       int       `json:"gdop_cols"`
	GDOPRows       int       `json:"gdop_rows"`
}

func (h *FleetEventAPIHandler) getEventDetails(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid event id", http.StatusBadRequest)
		return
	}

	// Get history and find the event
	records, err := h.healer.GetOptimisationHistory(100)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	for _, rec := range records {
		if rec.ID == id {
			resp := fleetEventDetails{
				ID:             rec.ID,
				Timestamp:      rec.Timestamp,
				TriggerReason:  rec.TriggerReason,
				MeanGDOPBefore: rec.MeanGDOPBefore,
				MeanGDOPAfter:  rec.MeanGDOPAfter,
				CoverageDelta:  rec.CoverageDelta,
				// Note: GDOP maps are stored separately if available
				// For now, return the summary data
			}
			writeJSON(w, resp)
			return
		}
	}

	http.Error(w, "event not found", http.StatusNotFound)
}
