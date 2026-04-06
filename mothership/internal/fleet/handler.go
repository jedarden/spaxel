package fleet

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi"
	"github.com/spaxel/mothership/internal/events"
)

// Handler serves the fleet REST API.
type Handler struct {
	mgr *Manager
}

// NewHandler creates a new fleet REST handler backed by mgr.
func NewHandler(mgr *Manager) *Handler {
	return &Handler{mgr: mgr}
}

// RegisterRoutes mounts fleet endpoints on r.
//
//	GET    /api/nodes                — list all nodes
//	GET    /api/nodes/{mac}          — get single node
//	POST   /api/nodes/{mac}/role     — override node role
//	PUT    /api/nodes/{mac}/position — update node 3D position
//	DELETE /api/nodes/{mac}          — delete a node
//	POST   /api/nodes/virtual        — add a virtual planning node
//	PUT    /api/room                 — update room dimensions
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/nodes", h.listNodes)
	r.Get("/api/nodes/{mac}", h.getNode)
	r.Post("/api/nodes/{mac}/role", h.setNodeRole)
	r.Put("/api/nodes/{mac}/position", h.updateNodePosition)
	r.Delete("/api/nodes/{mac}", h.deleteNode)
	r.Post("/api/nodes/virtual", h.addVirtualNode)
	r.Put("/api/room", h.updateRoom)
	// System mode endpoints
	r.Get("/api/mode", h.getSystemMode)
	r.Post("/api/mode", h.setSystemMode)
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
