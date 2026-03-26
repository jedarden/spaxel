// Package dashboard handles WebSocket connections from dashboard clients
// and broadcasts CSI data from the ingestion server.
package dashboard

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/spaxel/mothership/internal/ingestion"
)

// Hub manages all dashboard client connections and broadcasts
type Hub struct {
	mu          sync.RWMutex
	clients     map[*Client]struct{}
	broadcast   chan []byte
	register    chan *Client
	unregister  chan *Client

	// Reference to ingestion server for state queries
	ingestionState IngestionState
}

// IngestionState is an interface to query node/link state from ingestion
type IngestionState interface {
	GetConnectedNodesInfo() []ingestion.NodeInfo
	GetAllLinksInfo() []ingestion.LinkInfo
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
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = struct{}{}
			h.mu.Unlock()
			log.Printf("[INFO] Dashboard client connected (total: %d)", len(h.clients))

			// Send initial state
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

		case <-ticker.C:
			// Periodic state broadcast
			h.broadcastState()
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
	// For now, just forward the raw binary frame
	// Dashboard clients will parse it
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

func (h *Hub) sendInitialState(client *Client) {
	h.mu.RLock()
	state := h.ingestionState
	h.mu.RUnlock()

	if state == nil {
		return
	}

	// Build state message
	msg := map[string]interface{}{
		"type": "state",
	}

	nodes := state.GetConnectedNodesInfo()
	if nodes != nil {
		msg["nodes"] = nodes
	}

	links := state.GetAllLinksInfo()
	if links != nil {
		msg["links"] = links
	}

	data, _ := json.Marshal(msg)

	select {
	case client.send <- data:
	default:
		// Buffer full, skip
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

	// Build state message
	msg := map[string]interface{}{
		"type": "state",
	}

	nodes := state.GetConnectedNodesInfo()
	if nodes != nil {
		msg["nodes"] = nodes
	}

	links := state.GetAllLinksInfo()
	if links != nil {
		msg["links"] = links
	}

	data, _ := json.Marshal(msg)
	h.Broadcast(data)
}

// ClientCount returns the number of connected dashboard clients
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
