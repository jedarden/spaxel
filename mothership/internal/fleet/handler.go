package fleet

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/spaxel/mothership/internal/events"
	"github.com/spaxel/mothership/internal/ota"
)

// NodeIdentifier sends identify commands to connected nodes.
type NodeIdentifier interface {
	SendIdentifyToMAC(mac string, durationMS int) bool
	SendRebootToMAC(mac string, delayMS int) bool
	GetConnectedMACs() []string
}

// Handler serves the fleet REST API.
type Handler struct {
	mgr    *Manager
	nodeID NodeIdentifier
	otaMgr *ota.Manager
}

// NewHandler creates a new fleet REST handler backed by mgr.
func NewHandler(mgr *Manager) *Handler {
	return &Handler{mgr: mgr}
}

// SetOTAManager sets the OTA manager for handling firmware updates.
func (h *Handler) SetOTAManager(mgr *ota.Manager) {
	h.otaMgr = mgr
}

// SetNodeIdentifier sets the node identifier for sending identify commands.
func (h *Handler) SetNodeIdentifier(ni NodeIdentifier) {
	h.nodeID = ni
}

// RegisterRoutes mounts fleet endpoints on r.
//
//	GET    /api/nodes                — list all nodes
//	GET    /api/nodes/{mac}          — get single node
//	POST   /api/nodes/{mac}/role     — override node role
//	PUT    /api/nodes/{mac}/position — update node 3D position
//	PATCH  /api/nodes/{mac}/label    — update node label
//	DELETE /api/nodes/{mac}          — delete a node
//	POST   /api/nodes/{mac}/identify — blink LED for identification
//	POST   /api/nodes/{mac}/reboot   — reboot node
//	POST   /api/nodes/{mac}/ota      — trigger OTA update
//	POST   /api/nodes/update-all     — OTA update all nodes
//	POST   /api/nodes/rebaseline-all — re-baseline all links
//	POST   /api/nodes/virtual        — add a virtual planning node
//	PUT    /api/room                 — update room dimensions
//	GET    /api/export               — export configuration
//	POST   /api/import               — import configuration
//	GET    /api/mode                 — get system mode
//	POST   /api/mode                 — set system mode
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/fleet", h.listFleet) // Extended fleet data with computed fields
	r.Get("/api/nodes", h.listNodes)
	r.Get("/api/nodes/{mac}", h.getNode)
	r.Post("/api/nodes/{mac}/role", h.setNodeRole)
	r.Put("/api/nodes/{mac}/position", h.updateNodePosition)
	r.Patch("/api/nodes/{mac}/label", h.updateNodeLabel)
	r.Delete("/api/nodes/{mac}", h.deleteNode)
	r.Post("/api/nodes/{mac}/identify", h.identifyNode)
	r.Post("/api/nodes/{mac}/locate", h.identifyNode) // alias for identify
	r.Post("/api/nodes/{mac}/reboot", h.rebootNode)
	r.Post("/api/nodes/{mac}/ota", h.triggerNodeOTA)
	r.Post("/api/nodes/update-all", h.updateAllNodes)
	r.Post("/api/nodes/rebaseline-all", h.rebaselineAllNodes)
	r.Post("/api/nodes/virtual", h.addVirtualNode)
	r.Put("/api/room", h.updateRoom)
	// System mode endpoints
	r.Get("/api/mode", h.getSystemMode)
	r.Post("/api/mode", h.setSystemMode)
	// Export/Import endpoints
	r.Get("/api/export", h.exportConfig)
	r.Post("/api/import", h.importConfig)
}

func (h *Handler) listNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.mgr.registry.GetAllNodes()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if nodes == nil {
		nodes = []NodeRecord{}
	}
	writeJSON(w, nodes)
}

// FleetNode represents extended node data for the fleet page.
type FleetNode struct {
	MAC             string  `json:"mac"`
	Name            string  `json:"name"`
	Label           string  `json:"label"`
	Role            string  `json:"role"`
	Status          string  `json:"status"` // "online", "offline", "updating"
	FirmwareVersion string  `json:"firmware_version"`
	ChipModel       string  `json:"chip_model"`
	PosX            float64 `json:"pos_x"`
	PosY            float64 `json:"pos_y"`
	PosZ            float64 `json:"pos_z"`
	Virtual         bool    `json:"virtual"`
	HealthScore     float64 `json:"health_score"`
	// Computed fields
	LastSeenMS    int64   `json:"last_seen_ms"`
	UptimeSeconds int64   `json:"uptime_seconds"`
	PacketRate    float64 `json:"packet_rate"`
	ConfiguredRate int   `json:"configured_rate"`
	Temperature   float64 `json:"temperature"`
	OTAInProgress bool    `json:"ota_in_progress"`
}

// listFleet returns extended node data with computed fields for the fleet page.
func (h *Handler) listFleet(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.mgr.registry.GetAllNodes()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if nodes == nil {
		nodes = []NodeRecord{}
	}

	// Get connected MACs for status determination
	var connectedMACs []string
	if h.nodeID != nil {
		connectedMACs = h.nodeID.GetConnectedMACs()
	}
	connectedSet := make(map[string]bool)
	for _, mac := range connectedMACs {
		connectedSet[mac] = true
	}

	// Get OTA progress if OTA manager is available
	var otaProgress map[string]ota.NodeOTAProgress
	if h.otaMgr != nil {
		otaProgress = h.otaMgr.GetProgress()
	}

	// Convert to FleetNode with computed fields
	fleetNodes := make([]FleetNode, 0, len(nodes))
	now := time.Now()
	for _, node := range nodes {
		fleetNode := FleetNode{
			MAC:             node.MAC,
			Name:            node.Name,
			Label:           node.Name, // Label is same as name
			Role:            node.Role,
			FirmwareVersion: node.FirmwareVersion,
			ChipModel:       node.ChipModel,
			PosX:            node.PosX,
			PosY:            node.PosY,
			PosZ:            node.PosZ,
			Virtual:         node.Virtual,
			HealthScore:     node.HealthScore,
			LastSeenMS:      node.LastSeenAt.UnixMilli(),
			ConfiguredRate:  20, // Default configured rate
			Temperature:     0,  // Not currently tracked
		}

		// Determine status - check OTA progress first
		if otaProgress != nil {
			if progress, ok := otaProgress[node.MAC]; ok {
				// Node has OTA progress - determine status from OTA state
				switch progress.State {
				case ota.OTAPending, ota.OTADownloading, ota.OTARebooting:
					fleetNode.Status = "updating"
					fleetNode.OTAInProgress = true
				case ota.OTAFailed, ota.OTARollback:
					// Failed or rollback - show as offline
					fleetNode.Status = "offline"
					fleetNode.OTAInProgress = false
				case ota.OTAVerified:
					// Verified - check if currently connected
					if connectedSet[node.MAC] {
						fleetNode.Status = "online"
					} else {
						fleetNode.Status = "offline"
					}
					fleetNode.OTAInProgress = false
				default:
					// No active OTA - check connection status
					if connectedSet[node.MAC] {
						fleetNode.Status = "online"
					} else {
						fleetNode.Status = "offline"
					}
					fleetNode.OTAInProgress = false
				}
			} else {
				// No OTA progress for this node - check connection status
				if connectedSet[node.MAC] {
					fleetNode.Status = "online"
				} else if node.WentOfflineAt.IsZero() {
					// Never seen online or still in initial state
					fleetNode.Status = "offline"
				} else {
					fleetNode.Status = "offline"
				}
				fleetNode.OTAInProgress = false
			}
		} else {
			// No OTA manager - check connection status
			if connectedSet[node.MAC] {
				fleetNode.Status = "online"
			} else if node.WentOfflineAt.IsZero() {
				// Never seen online or still in initial state
				fleetNode.Status = "offline"
			} else {
				fleetNode.Status = "offline"
			}
			fleetNode.OTAInProgress = false
		}

		// Calculate uptime (time since first seen, approximated as last seen - first seen + current session)
		if !node.FirstSeenAt.IsZero() && !node.LastSeenAt.IsZero() {
			// Approximate uptime as time since first seen
			fleetNode.UptimeSeconds = int64(now.Sub(node.FirstSeenAt).Seconds())
		}

		// Packet rate - would need to be calculated from recent CSI data
		// For now, use a reasonable default or calculate from health score
		if fleetNode.Status == "online" && fleetNode.HealthScore > 0 {
			fleetNode.PacketRate = fleetNode.HealthScore * 20 // Approximate based on health
		}

		fleetNodes = append(fleetNodes, fleetNode)
	}

	writeJSON(w, fleetNodes)
}

func (h *Handler) getNode(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")
	node, err := h.mgr.registry.GetNode(mac)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "node not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, node)
}

var validRoles = map[string]bool{
	"tx": true, "rx": true, "tx_rx": true, "passive": true, "virtual": true,
}

type setRoleRequest struct {
	Role string `json:"role"`
}

func (h *Handler) setNodeRole(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")

	// Verify node exists.
	if _, err := h.mgr.registry.GetNode(mac); errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "node not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var req setRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Role == "" {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if !validRoles[req.Role] {
		http.Error(w, "invalid role", http.StatusBadRequest)
		return
	}

	if err := h.mgr.OverrideRole(mac, req.Role); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	node, err := h.mgr.registry.GetNode(mac)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, node)
}

// ── position / virtual / room endpoints ──────────────────────────────────────

type updatePositionRequest struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

func (h *Handler) updateNodePosition(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")
	var req updatePositionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := h.mgr.GetRegistry().SetNodePosition(mac, req.X, req.Y, req.Z); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.mgr.BroadcastRegistry()
	w.WriteHeader(http.StatusNoContent)
}

type addVirtualNodeRequest struct {
	MAC  string  `json:"mac"`
	Name string  `json:"name"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Z    float64 `json:"z"`
}

func (h *Handler) addVirtualNode(w http.ResponseWriter, r *http.Request) {
	var req addVirtualNodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.MAC == "" {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := h.mgr.GetRegistry().AddVirtualNode(req.MAC, req.Name, req.X, req.Y, req.Z); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.mgr.BroadcastRegistry()
	w.WriteHeader(http.StatusCreated)
}

func (h *Handler) deleteNode(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")
	if err := h.mgr.GetRegistry().DeleteNode(mac); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.mgr.BroadcastRegistry()
	w.WriteHeader(http.StatusNoContent)
}

type identifyNodeRequest struct {
	DurationMS int `json:"duration_ms"`
}

func (h *Handler) identifyNode(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")

	// Verify node exists.
	if _, err := h.mgr.registry.GetNode(mac); errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "node not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Parse request body.
	var req identifyNodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Default to 5000ms if not specified.
	durationMS := req.DurationMS
	if durationMS <= 0 {
		durationMS = 5000
	}

	// Send identify command if node identifier is available.
	if h.nodeID != nil {
		if !h.nodeID.SendIdentifyToMAC(mac, durationMS) {
			http.Error(w, "node not connected", http.StatusNotFound)
			return
		}
	}

	writeJSON(w, map[string]bool{"ok": true})
}

func (h *Handler) rebootNode(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")

	// Verify node exists.
	if _, err := h.mgr.registry.GetNode(mac); errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "node not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Parse request body for optional delay.
	var req struct {
		DelayMS int `json:"delay_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	delayMS := req.DelayMS
	if delayMS <= 0 {
		delayMS = 1000 // Default 1 second delay
	}

	// Send reboot command if node identifier is available.
	if h.nodeID != nil {
		if !h.nodeID.SendRebootToMAC(mac, delayMS) {
			http.Error(w, "node not connected", http.StatusNotFound)
			return
		}
	}

	writeJSON(w, map[string]bool{"ok": true})
}

func (h *Handler) updateAllNodes(w http.ResponseWriter, r *http.Request) {
	// Trigger rolling update with 30-second stagger (if OTA manager is configured)
	if h.otaMgr != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancel()

			if err := h.otaMgr.SendOTAAll(ctx, 30*time.Second); err != nil {
				log.Printf("[ERROR] fleet: updateAllNodes failed: %v", err)
			}
		}()
	}

	// Return immediately with the count of nodes that will be updated
	var count int
	if h.nodeID != nil {
		macs := h.nodeID.GetConnectedMACs()
		count = len(macs)
	}

	writeJSON(w, map[string]interface{}{
		"ok":    true,
		"count": count,
	})
}

func (h *Handler) rebaselineAllNodes(w http.ResponseWriter, r *http.Request) {
	// This is a placeholder - the actual baseline manager would handle this
	// For now, return a success response
	writeJSON(w, map[string]interface{}{
		"ok":    true,
		"count": 0,
	})
}

func (h *Handler) exportConfig(w http.ResponseWriter, r *http.Request) {
	// Collect all configuration data
	nodes, err := h.mgr.registry.GetAllNodes()
	if err != nil {
		http.Error(w, "failed to get nodes", http.StatusInternalServerError)
		return
	}

	config := map[string]interface{}{
		"version":    1,
		"exported_at": time.Now().Format(time.RFC3339),
		"nodes":      nodes,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(config); err != nil {
		http.Error(w, "failed to encode config", http.StatusInternalServerError)
	}
}

func (h *Handler) importConfig(w http.ResponseWriter, r *http.Request) {
	var config map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// For now, just return success - a full implementation would validate and apply the config
	writeJSON(w, map[string]interface{}{
		"ok": true,
		"imported": map[string]interface{}{
			"nodes": 0,
		},
	})
}

type updateRoomRequest struct {
	Width   float64 `json:"width"`
	Depth   float64 `json:"depth"`
	Height  float64 `json:"height"`
	OriginX float64 `json:"origin_x"`
	OriginZ float64 `json:"origin_z"`
}

func (h *Handler) updateRoom(w http.ResponseWriter, r *http.Request) {
	var req updateRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Width <= 0 || req.Depth <= 0 || req.Height <= 0 {
		http.Error(w, "dimensions must be positive", http.StatusBadRequest)
		return
	}
	room := RoomConfig{
		ID:      "main",
		Name:    "Main",
		Width:   req.Width,
		Depth:   req.Depth,
		Height:  req.Height,
		OriginX: req.OriginX,
		OriginZ: req.OriginZ,
	}
	if err := h.mgr.GetRegistry().SetRoom(room); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.mgr.BroadcastRegistry()
	w.WriteHeader(http.StatusNoContent)
}

// ── System Mode endpoints ───────────────────────────────────────────────────────

type systemModeResponse struct {
	Mode           string `json:"mode"`
	Reason         string `json:"reason,omitempty"`
	AutoAwayConfig autoAwayConfigResponse `json:"auto_away_config"`
}

type autoAwayConfigResponse struct {
	Enabled         bool `json:"enabled"`
	AbsenceDurationSec int `json:"absence_duration_sec"`
}

// getSystemMode returns the current system mode.
func (h *Handler) getSystemMode(w http.ResponseWriter, r *http.Request) {
	mode := h.mgr.GetSystemMode()
	cfg := h.mgr.GetAutoAwayConfig()

	resp := systemModeResponse{
		Mode:   string(mode),
		AutoAwayConfig: autoAwayConfigResponse{
			Enabled:           cfg.Enabled,
			AbsenceDurationSec: int(cfg.AbsenceDuration.Seconds()),
		},
	}
	writeJSON(w, resp)
}

type setSystemModeRequest struct {
	Mode   string `json:"mode"`
	Reason string `json:"reason,omitempty"`
}

// setSystemMode sets the system mode manually.
func (h *Handler) setSystemMode(w http.ResponseWriter, r *http.Request) {
	var req setSystemModeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var mode events.SystemMode
	switch req.Mode {
	case "home":
		mode = events.ModeHome
	case "away":
		mode = events.ModeAway
	case "sleep":
		mode = events.ModeSleep
	default:
		http.Error(w, "invalid mode: must be home, away, or sleep", http.StatusBadRequest)
		return
	}

	reason := req.Reason
	if reason == "" {
		reason = "manual"
	}

	if err := h.mgr.SetSystemMode(mode, reason); err != nil {
		http.Error(w, "failed to set mode", http.StatusInternalServerError)
		return
	}

	resp := systemModeResponse{
		Mode:   string(mode),
		Reason: reason,
	}
	writeJSON(w, resp)
}

// ── Label and OTA endpoints ─────────────────────────────────────────────────────

type updateLabelRequest struct {
	Label string `json:"label"`
}

func (h *Handler) updateNodeLabel(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")

	// Verify node exists.
	if _, err := h.mgr.registry.GetNode(mac); errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "node not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var req updateLabelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.mgr.registry.SetNodeLabel(mac, req.Label); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.mgr.BroadcastRegistry()
	w.WriteHeader(http.StatusNoContent)
}

type triggerOTARequest struct {
	Version string `json:"version,omitempty"`
}

func (h *Handler) triggerNodeOTA(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")

	// Verify node exists.
	node, err := h.mgr.registry.GetNode(mac)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "node not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var req triggerOTARequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Trigger OTA if manager is available.
	if h.otaMgr != nil {
		var err error
		if req.Version != "" {
			// Send specific version
			err = h.otaMgr.SendOTAVersion(mac, req.Version)
		} else {
			// Send latest/default OTA
			err = h.otaMgr.SendOTA(mac)
		}
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to trigger OTA: %v", err), http.StatusInternalServerError)
			return
		}
	}

	writeJSON(w, map[string]interface{}{
		"ok":           true,
		"target_mac":   mac,
		"target_label": node.Name,
		"version":      req.Version,
	})
}
