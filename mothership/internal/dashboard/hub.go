// Package dashboard handles WebSocket connections from dashboard clients
// and broadcasts CSI data from the ingestion server.
package dashboard

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/spaxel/mothership/internal/fleet"
	"github.com/spaxel/mothership/internal/ingestion"
	"github.com/spaxel/mothership/internal/tracking"
)

// Hub manages all dashboard client connections and broadcasts
type Hub struct {
	mu         sync.RWMutex
	clients    map[*Client]struct{}
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client

	// Reference to ingestion server for state queries
	ingestionState IngestionState
}

// IngestionState is an interface to query node/link/motion state from ingestion
type IngestionState interface {
	GetConnectedNodesInfo() []ingestion.NodeInfo
	GetAllLinksInfo() []ingestion.LinkInfo
	GetAllMotionStates() []ingestion.MotionStateItem
}

// Client represents a dashboard WebSocket client
type Client struct {
	hub  *Hub
	send chan []byte
}

// NewHub creates a new dashboard hub
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]struct{}),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// SetIngestionState sets the ingestion state provider
func (h *Hub) SetIngestionState(state IngestionState) {
	h.mu.Lock()
	h.ingestionState = state
	h.mu.Unlock()
}

// Run starts the hub's main loop
func (h *Hub) Run() {
	stateTicker := time.NewTicker(5 * time.Second)
	defer stateTicker.Stop()

	presenceTicker := time.NewTicker(500 * time.Millisecond)
	defer presenceTicker.Stop()

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = struct{}{}
			h.mu.Unlock()
			log.Printf("[INFO] Dashboard client connected (total: %d)", len(h.clients))
			h.sendInitialState(client)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			log.Printf("[INFO] Dashboard client disconnected (total: %d)", len(h.clients))

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// Client buffer full, skip
				}
			}
			h.mu.RUnlock()

		case <-stateTicker.C:
			h.broadcastState()

		case <-presenceTicker.C:
			h.broadcastPresence()
		}
	}
}

// Register registers a new dashboard client
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// Unregister unregisters a dashboard client
func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

// Broadcast sends a message to all connected dashboard clients
func (h *Hub) Broadcast(message []byte) {
	select {
	case h.broadcast <- message:
	default:
		// Channel full, drop message
	}
}

// BroadcastCSI broadcasts a CSI frame to all dashboard clients
func (h *Hub) BroadcastCSI(nodeMAC, peerMAC string, data []byte) {
	h.Broadcast(data)
}

// BroadcastNodeConnected notifies dashboards of a new node
func (h *Hub) BroadcastNodeConnected(mac, firmware, chip string) {
	msg := map[string]interface{}{
		"type":             "node_connected",
		"mac":              mac,
		"firmware_version": firmware,
		"chip":             chip,
	}
	data, _ := json.Marshal(msg)
	h.Broadcast(data)
}

// BroadcastNodeDisconnected notifies dashboards of a node leaving
func (h *Hub) BroadcastNodeDisconnected(mac string) {
	msg := map[string]interface{}{
		"type": "node_disconnected",
		"mac":  mac,
	}
	data, _ := json.Marshal(msg)
	h.Broadcast(data)
}

// BroadcastLinkActive notifies dashboards of an active link
func (h *Hub) BroadcastLinkActive(linkID, nodeMAC, peerMAC string) {
	msg := map[string]interface{}{
		"type":     "link_active",
		"id":       linkID,
		"node_mac": nodeMAC,
		"peer_mac": peerMAC,
	}
	data, _ := json.Marshal(msg)
	h.Broadcast(data)
}

// BroadcastLinkInactive notifies dashboards of an inactive link
func (h *Hub) BroadcastLinkInactive(linkID string) {
	msg := map[string]interface{}{
		"type": "link_inactive",
		"id":   linkID,
	}
	data, _ := json.Marshal(msg)
	h.Broadcast(data)
}

// BroadcastMotionState sends motion state for one or more links to all dashboard clients.
// Called on state changes (idle↔motion) so the dashboard updates immediately.
func (h *Hub) BroadcastMotionState(states []ingestion.MotionStateItem) {
	msg := map[string]interface{}{
		"type":  "motion_state",
		"links": states,
	}
	data, _ := json.Marshal(msg)
	h.Broadcast(data)
}

// BroadcastPresenceUpdate sends periodic presence state for all links.
// Broadcasts every 500ms with {type: "presence_update", links: {linkID: {...}}}.
func (h *Hub) broadcastPresence() {
	h.mu.RLock()
	state := h.ingestionState
	clientCount := len(h.clients)
	h.mu.RUnlock()

	if state == nil || clientCount == 0 {
		return
	}

	items := state.GetAllMotionStates()
	if len(items) == 0 {
		return
	}

	links := make(map[string]ingestion.MotionStateItem, len(items))
	for _, item := range items {
		links[item.LinkID] = item
	}

	msg := map[string]interface{}{
		"type":  "presence_update",
		"links": links,
	}
	data, _ := json.Marshal(msg)
	h.Broadcast(data)
}

// ─── Phase 3 Broadcasts ─────────────────────────────────────────────────────

// nodeJSON is the wire format for a fleet node sent to the dashboard.
type nodeJSON struct {
	MAC      string  `json:"mac"`
	Name     string  `json:"name"`
	Role     string  `json:"role"`
	PosX     float64 `json:"pos_x"`
	PosY     float64 `json:"pos_y"`
	PosZ     float64 `json:"pos_z"`
	Virtual  bool    `json:"virtual"`
	LastSeen int64   `json:"last_seen_ms"`
}

// roomJSON is the wire format for room configuration.
type roomJSON struct {
	Width   float64 `json:"width"`
	Depth   float64 `json:"depth"`
	Height  float64 `json:"height"`
	OriginX float64 `json:"origin_x"`
	OriginZ float64 `json:"origin_z"`
}

// BroadcastRegistryState sends updated node registry and room config to all dashboard clients.
func (h *Hub) BroadcastRegistryState(nodes []fleet.NodeRecord, room fleet.RoomConfig) {
	wireNodes := make([]nodeJSON, len(nodes))
	for i, n := range nodes {
		wireNodes[i] = nodeJSON{
			MAC:      n.MAC,
			Name:     n.Name,
			Role:     n.Role,
			PosX:     n.PosX,
			PosY:     n.PosY,
			PosZ:     n.PosZ,
			Virtual:  n.Virtual,
			LastSeen: n.LastSeenAt.UnixMilli(),
		}
	}
	msg := map[string]interface{}{
		"type":  "registry_state",
		"nodes": wireNodes,
		"room": roomJSON{
			Width:   room.Width,
			Depth:   room.Depth,
			Height:  room.Height,
			OriginX: room.OriginX,
			OriginZ: room.OriginZ,
		},
	}
	data, _ := json.Marshal(msg)
	h.Broadcast(data)
}

// trailPoint is a compact [x, z] pair for JSON serialisation.
type trailPoint [2]float64

// blobJSON is the wire format for a tracked person blob.
type blobJSON struct {
	ID     int          `json:"id"`
	X      float64      `json:"x"`
	Z      float64      `json:"z"`
	VX     float64      `json:"vx"`
	VZ     float64      `json:"vz"`
	Weight float64      `json:"weight"`
	Trail  []trailPoint `json:"trail"`
}

// BroadcastLocUpdate sends localisation results to all dashboard clients.
func (h *Hub) BroadcastLocUpdate(blobs []tracking.Blob) {
	wireBlobs := make([]blobJSON, len(blobs))
	for i, b := range blobs {
		trail := make([]trailPoint, len(b.Trail))
		for j, pt := range b.Trail {
			trail[j] = trailPoint{pt[0], pt[1]}
		}
		wireBlobs[i] = blobJSON{
			ID:     b.ID,
			X:      b.X,
			Z:      b.Z,
			VX:     b.VX,
			VZ:     b.VZ,
			Weight: b.Weight,
			Trail:  trail,
		}
	}
	msg := map[string]interface{}{
		"type":  "loc_update",
		"blobs": wireBlobs,
	}
	data, _ := json.Marshal(msg)
	h.Broadcast(data)
}

// BroadcastCoverageMap sends the GDOP coverage map to all dashboard clients.
// data is a row-major float32 array of GDOP values, cols × rows cells.
func (h *Hub) BroadcastCoverageMap(data []float32, cols, rows int, cellSize float64, originX, originZ float64) {
	// Encode as a compact flat array of float32 values (JSON).
	vals := make([]float64, len(data))
	for i, v := range data {
		vals[i] = float64(v)
	}
	msg := map[string]interface{}{
		"type":      "coverage_map",
		"cols":      cols,
		"rows":      rows,
		"cell_size": cellSize,
		"origin_x":  originX,
		"origin_z":  originZ,
		"data":      vals,
	}
	encoded, _ := json.Marshal(msg)
	h.Broadcast(encoded)
}

func (h *Hub) sendInitialState(client *Client) {
	h.mu.RLock()
	state := h.ingestionState
	h.mu.RUnlock()

	if state == nil {
		return
	}

	msg := h.buildStateMsg(state)
	data, _ := json.Marshal(msg)

	select {
	case client.send <- data:
	default:
	}
}

func (h *Hub) broadcastState() {
	h.mu.RLock()
	state := h.ingestionState
	clientCount := len(h.clients)
	h.mu.RUnlock()

	if state == nil || clientCount == 0 {
		return
	}

	msg := h.buildStateMsg(state)
	data, _ := json.Marshal(msg)
	h.Broadcast(data)
}

func (h *Hub) buildStateMsg(state IngestionState) map[string]interface{} {
	msg := map[string]interface{}{
		"type": "state",
	}

	if nodes := state.GetConnectedNodesInfo(); nodes != nil {
		msg["nodes"] = nodes
	}
	if links := state.GetAllLinksInfo(); links != nil {
		msg["links"] = links
	}
	if motionStates := state.GetAllMotionStates(); len(motionStates) > 0 {
		msg["motion_states"] = motionStates
	}

	return msg
}

// ClientCount returns the number of connected dashboard clients
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
