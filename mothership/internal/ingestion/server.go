package ingestion

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Server manages WebSocket connections from ESP32 nodes
type Server struct {
	mu         sync.RWMutex
	connections map[string]*NodeConnection // keyed by MAC
	links      map[string]*RingBuffer     // keyed by "nodeMAC:peerMAC"

	// Malformed frame tracking per connection
	malformedCounts map[string]*malformedCounter

	// WebSocket upgrader
	upgrader websocket.Upgrader

	// Shutdown state
	shutdown bool
}

// NodeConnection tracks state for a connected node
type NodeConnection struct {
	MAC             string
	Conn            *websocket.Conn
	Hello           *HelloMessage
	LastHealth      *HealthMessage
	LastHealthTime  time.Time
	ConnectedAt     time.Time
	LastFrameTime   time.Time

	// Write mutex for thread-safe sends
	writeMu sync.Mutex
}

// malformedCounter tracks malformed frame counts for rate limiting
type malformedCounter struct {
	count     int
	firstSeen time.Time
}

const (
	// Ping/pong timing
	pingInterval = 30 * time.Second
	readDeadline = 60 * time.Second

	// Malformed frame thresholds
	malformedWarnThreshold  = 100
	malformedCloseThreshold = 1000
	malformedWindow         = time.Minute
)

// NewServer creates a new ingestion server
func NewServer() *Server {
	return &Server{
		connections:     make(map[string]*NodeConnection),
		links:           make(map[string]*RingBuffer),
		malformedCounts: make(map[string]*malformedCounter),
		upgrader: websocket.Upgrader{
			// Allow all origins for development (TODO: restrict in production)
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
			ReadBufferSize:  512,
			WriteBufferSize: 512,
		},
	}
}

// HandleNodeWS handles WebSocket connections at /ws/node
func (s *Server) HandleNodeWS(w http.ResponseWriter, r *http.Request) {
	// Upgrade HTTP connection to WebSocket
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WARN] WebSocket upgrade failed: %v", err)
		return
	}

	// Set initial read deadline
	conn.SetReadDeadline(time.Now().Add(readDeadline))

	// Create connection state
	nc := &NodeConnection{
		Conn:        conn,
		ConnectedAt: time.Now(),
	}

	// Wait for hello message (must be first)
	_, msg, err := conn.ReadMessage()
	if err != nil {
		log.Printf("[WARN] Failed to read hello: %v", err)
		conn.Close()
		return
	}

	// Parse as JSON (hello must be JSON)
	parsed, err := ParseJSONMessage(msg)
	if err != nil {
		s.sendReject(conn, "invalid hello format")
		conn.Close()
		return
	}

	hello, ok := parsed.(*HelloMessage)
	if !ok {
		s.sendReject(conn, "expected hello first")
		conn.Close()
		return
	}

	nc.MAC = hello.MAC
	nc.Hello = hello

	// Register connection
	s.mu.Lock()
	// Close existing connection from same MAC if present
	if existing, exists := s.connections[hello.MAC]; exists {
		existing.Conn.Close()
	}
	s.connections[hello.MAC] = nc
	s.malformedCounts[hello.MAC] = &malformedCounter{}
	s.mu.Unlock()

	log.Printf("[INFO] Node connected: MAC=%s firmware=%s chip=%s",
		hello.MAC, hello.FirmwareVersion, hello.Chip)

	// Send initial role and config
	s.sendRole(nc, "rx", "")
	s.sendConfig(nc, 20, 0, 0) // 20 Hz default

	// Start ping goroutine
	go s.pingLoop(nc)

	// Message handling loop
	s.handleMessages(nc)
}

// handleMessages processes incoming WebSocket messages
func (s *Server) handleMessages(nc *NodeConnection) {
	defer func() {
		nc.Conn.Close()
		s.mu.Lock()
		delete(s.connections, nc.MAC)
		delete(s.malformedCounts, nc.MAC)
		s.mu.Unlock()
		log.Printf("[INFO] Node disconnected: MAC=%s", nc.MAC)
	}()

	for {
		// Reset read deadline on each message
		nc.Conn.SetReadDeadline(time.Now().Add(readDeadline))

		messageType, data, err := nc.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[WARN] Unexpected close from %s: %v", nc.MAC, err)
			}
			return
		}

		if messageType == websocket.BinaryMessage {
			s.handleBinaryFrame(nc, data)
		} else if messageType == websocket.TextMessage {
			s.handleJSONMessage(nc, data)
		}
	}
}

// handleBinaryFrame processes a CSI binary frame
func (s *Server) handleBinaryFrame(nc *NodeConnection, data []byte) {
	frame, err := ParseFrame(data)
	if err != nil {
		s.recordMalformed(nc.MAC)
		return
	}

	// Update last frame time
	nc.LastFrameTime = time.Now()

	// Get or create ring buffer for this link
	linkID := frame.LinkID()
	s.mu.Lock()
	ring, exists := s.links[linkID]
	if !exists {
		ring = NewRingBuffer()
		s.links[linkID] = ring
	}
	s.mu.Unlock()

	// Push frame to ring buffer
	ring.Push(frame, time.Now())
}

// handleJSONMessage processes a JSON control message
func (s *Server) handleJSONMessage(nc *NodeConnection, data []byte) {
	parsed, err := ParseJSONMessage(data)
	if err != nil {
		// Unknown types are silently ignored per protocol
		return
	}

	switch msg := parsed.(type) {
	case *HealthMessage:
		nc.LastHealth = msg
		nc.LastHealthTime = time.Now()
		// TODO: expose health metrics

	case *BLEMessage:
		// TODO: forward BLE data to identity matcher

	case *MotionHintMessage:
		// TODO: trigger adaptive rate changes

	case *OTAStatusMessage:
		// TODO: track OTA progress
	}
}

// recordMalformed tracks malformed frames and closes connection if threshold exceeded
func (s *Server) recordMalformed(mac string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	counter, exists := s.malformedCounts[mac]
	if !exists {
		return
	}

	// Reset counter if window has passed
	if time.Since(counter.firstSeen) > malformedWindow {
		counter.count = 0
		counter.firstSeen = time.Now()
	}

	counter.count++

	// Warn at 100
	if counter.count == malformedWarnThreshold {
		log.Printf("[WARN] Node %s sending malformed CSI frames (count=%d)", mac, counter.count)
	}

	// Close at 1000
	if counter.count >= malformedCloseThreshold {
		log.Printf("[ERROR] Node %s exceeded malformed frame threshold, closing connection", mac)
		if nc, exists := s.connections[mac]; exists {
			nc.Conn.Close()
		}
	}
}

// pingLoop sends periodic ping frames to keep the connection alive
func (s *Server) pingLoop(nc *NodeConnection) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for range ticker.C {
		nc.writeMu.Lock()
		err := nc.Conn.WriteMessage(websocket.PingMessage, nil)
		nc.writeMu.Unlock()

		if err != nil {
			return // Connection closed
		}

		// Check for shutdown
		s.mu.RLock()
		shutdown := s.shutdown
		s.mu.RUnlock()
		if shutdown {
			return
		}
	}
}

// sendReject sends a reject message and closes the connection
func (s *Server) sendReject(conn *websocket.Conn, reason string) {
	msg := RejectMessage{Type: "reject", Reason: reason}
	data, _ := json.Marshal(msg)
	conn.WriteMessage(websocket.TextMessage, data)
}

// sendRole sends a role assignment to a node
func (s *Server) sendRole(nc *NodeConnection, role string, passiveBSSID string) {
	msg := RoleMessage{Type: "role", Role: role, PassiveBSSID: passiveBSSID}
	data, _ := json.Marshal(msg)

	nc.writeMu.Lock()
	nc.Conn.WriteMessage(websocket.TextMessage, data)
	nc.writeMu.Unlock()
}

// sendConfig sends configuration to a node
func (s *Server) sendConfig(nc *NodeConnection, rateHz int, txSlotUS int, varianceThreshold float64) {
	msg := ConfigMessage{Type: "config"}
	if rateHz > 0 {
		msg.RateHz = &rateHz
	}
	if txSlotUS > 0 {
		msg.TXSlotUS = &txSlotUS
	}
	if varianceThreshold > 0 {
		msg.VarianceThreshold = &varianceThreshold
	}
	data, _ := json.Marshal(msg)

	nc.writeMu.Lock()
	nc.Conn.WriteMessage(websocket.TextMessage, data)
	nc.writeMu.Unlock()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) {
	s.mu.Lock()
	s.shutdown = true

	// Send shutdown message to all connected nodes
	shutdownMsg := ShutdownMessage{Type: "shutdown", ReconnectInMS: 30000}
	data, _ := json.Marshal(shutdownMsg)

	for mac, nc := range s.connections {
		nc.writeMu.Lock()
		nc.Conn.WriteMessage(websocket.TextMessage, data)
		nc.Conn.Close()
		nc.writeMu.Unlock()
		delete(s.connections, mac)
	}
	s.mu.Unlock()

	log.Printf("[INFO] Ingestion server shutdown complete")
}

// GetConnectedNodes returns a list of connected node MACs
func (s *Server) GetConnectedNodes() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	macs := make([]string, 0, len(s.connections))
	for mac := range s.connections {
		macs = append(macs, mac)
	}
	return macs
}

// GetLinkBuffer returns the ring buffer for a specific link
func (s *Server) GetLinkBuffer(nodeMAC, peerMAC string) *RingBuffer {
	linkID := nodeMAC + ":" + peerMAC
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.links[linkID]
}

// GetAllLinks returns all link IDs that have data
func (s *Server) GetAllLinks() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	links := make([]string, 0, len(s.links))
	for linkID := range s.links {
		links = append(links, linkID)
	}
	return links
}
