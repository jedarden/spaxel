package fleet

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi"
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
//	GET  /api/nodes            — list all nodes
//	GET  /api/nodes/{mac}      — get single node
//	POST /api/nodes/{mac}/role — override node role
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/nodes", h.listNodes)
	r.Get("/api/nodes/{mac}", h.getNode)
	r.Post("/api/nodes/{mac}/role", h.setNodeRole)
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

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}
