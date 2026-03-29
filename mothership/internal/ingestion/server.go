package ingestion

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spaxel/mothership/internal/signal"
)

// CSIBroadcaster is a callback for broadcasting CSI frames to dashboard
type CSIBroadcaster interface {
	BroadcastCSI(nodeMAC, peerMAC string, data []byte)
	BroadcastNodeConnected(mac, firmware, chip string)
	BroadcastNodeDisconnected(mac string)
	BroadcastLinkActive(linkID, nodeMAC, peerMAC string)
}

// FleetNotifier receives node lifecycle events for fleet management.
type FleetNotifier interface {
	OnNodeConnected(mac, firmware, chip string)
	OnNodeDisconnected(mac string)
}

// OTAStatusHandler receives OTA status updates from nodes.
type OTAStatusHandler interface {
	OnOTAStatus(mac, state string, progressPct uint8, errMsg string)
	OnNodeReconnected(mac, firmwareVersion string)
}

// MotionBroadcaster broadcasts motion state changes to dashboard clients.
type MotionBroadcaster interface {
	BroadcastMotionState(states []MotionStateItem)
}

// MotionStateItem represents a single link's current motion state.
type MotionStateItem struct {
	LinkID            string  `json:"link_id"`
	MotionDetected    bool    `json:"motion_detected"`
	DeltaRMS          float64 `json:"delta_rms"`
	Confidence        float64 `json:"confidence"`
	DiurnalConfidence float64 `json:"diurnal_confidence"`
	DiurnalReady      bool    `json:"diurnal_ready"`
}

// ReplayAppender appends raw CSI frames to a persistent store.
type ReplayAppender interface {
	Append(recvTimeNS int64, rawFrame []byte) error
}

// Recorder records raw CSI frames to per-link segment files.
type Recorder interface {
	Write(linkID string, frame []byte)
}

// Server manages WebSocket connections from ESP32 nodes
type Server struct {
	mu          sync.RWMutex
	connections map[string]*NodeConnection // keyed by MAC
	links       map[string]*RingBuffer     // keyed by "nodeMAC:peerMAC"

	// Motion state per link (for change detection and state queries)
	linkMotionState map[string]bool    // linkID -> motionDetected
	linkDeltaRMS    map[string]float64 // linkID -> smoothDeltaRMS

	// Malformed frame tracking per connection
	malformedCounts map[string]*malformedCounter

	// WebSocket upgrader
	upgrader websocket.Upgrader

	// Shutdown state
	shutdown bool

	// Optional pipeline components (set via setters)
	dashboardBroadcaster CSIBroadcaster
	motionBroadcaster    MotionBroadcaster
	processorMgr         *signal.ProcessorManager
	replayStore          ReplayAppender
	recorder             Recorder
	rateCtrl             *RateController
	fleetNotifier        FleetNotifier
	otaHandler           OTAStatusHandler
}

// NodeConnection tracks state for a connected node
type NodeConnection struct {
	MAC            string
	Conn           *websocket.Conn
	Hello          *HelloMessage
	LastHealth     *HealthMessage
	LastHealthTime time.Time
	ConnectedAt    time.Time
	LastFrameTime  time.Time

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
		linkMotionState: make(map[string]bool),
		linkDeltaRMS:    make(map[string]float64),
		malformedCounts: make(map[string]*malformedCounter),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
			ReadBufferSize:  512,
			WriteBufferSize: 512,
		},
	}
}

// SetDashboardBroadcaster sets the callback for broadcasting CSI frames
func (s *Server) SetDashboardBroadcaster(broadcaster CSIBroadcaster) {
	s.mu.Lock()
	s.dashboardBroadcaster = broadcaster
	s.mu.Unlock()
}

// SetMotionBroadcaster sets the callback for broadcasting motion state changes.
func (s *Server) SetMotionBroadcaster(mb MotionBroadcaster) {
	s.mu.Lock()
	s.motionBroadcaster = mb
	s.mu.Unlock()
}

// SetProcessorManager sets the signal processing pipeline.
func (s *Server) SetProcessorManager(pm *signal.ProcessorManager) {
	s.mu.Lock()
	s.processorMgr = pm
	s.mu.Unlock()
}

// SetReplayStore sets the disk-backed recording store.
func (s *Server) SetReplayStore(store ReplayAppender) {
	s.mu.Lock()
	s.replayStore = store
	s.mu.Unlock()
}

// SetRecorder sets the per-link CSI frame recorder.
func (s *Server) SetRecorder(r Recorder) {
	s.mu.Lock()
	s.recorder = r
	s.mu.Unlock()
}

// SetRateController sets the adaptive rate controller.
func (s *Server) SetRateController(rc *RateController) {
	s.mu.Lock()
	s.rateCtrl = rc
	s.mu.Unlock()
}

// SetFleetNotifier sets the fleet manager for node lifecycle callbacks.
func (s *Server) SetFleetNotifier(fn FleetNotifier) {
	s.mu.Lock()
	s.fleetNotifier = fn
	s.mu.Unlock()
}

// SetOTAManager sets the OTA manager for status callbacks.
func (s *Server) SetOTAManager(h OTAStatusHandler) {
	s.mu.Lock()
	s.otaHandler = h
	s.mu.Unlock()
}

// GetConnectedMACs returns the MACs of currently-connected nodes.
func (s *Server) GetConnectedMACs() []string {
	return s.GetConnectedNodes()
}

// SendConfigToMAC sends a rate config command to a connected node by MAC.
// varianceThreshold > 0 enables on-device amplitude variance monitoring.
func (s *Server) SendConfigToMAC(mac string, rateHz int, varianceThreshold float64) {
	s.mu.RLock()
	nc, ok := s.connections[mac]
	s.mu.RUnlock()
	if !ok {
		return
	}
	s.sendConfig(nc, rateHz, 0, varianceThreshold)
}

// SendRoleToMAC sends a role assignment to a connected node by MAC.
// passiveBSSID is required only when role is "passive".
func (s *Server) SendRoleToMAC(mac, role, passiveBSSID string) {
	s.mu.RLock()
	nc, ok := s.connections[mac]
	s.mu.RUnlock()
	if !ok {
		return
	}
	s.sendRole(nc, role, passiveBSSID)
	log.Printf("[INFO] Sent role=%s to node %s", role, mac)
}

// SendOTAToMAC triggers a firmware update on a connected node.
func (s *Server) SendOTAToMAC(mac, url, sha256, version string) {
	s.mu.RLock()
	nc, ok := s.connections[mac]
	s.mu.RUnlock()
	if !ok {
		return
	}
	msg := OTAMessage{Type: "ota", URL: url, SHA256: sha256, Version: version}
	data, _ := json.Marshal(msg)
	nc.writeMu.Lock()
	nc.Conn.WriteMessage(websocket.TextMessage, data)
	nc.writeMu.Unlock()
	log.Printf("[INFO] Sent OTA trigger to node %s: version=%s url=%s", mac, version, url)
}

// HandleNodeWS handles WebSocket connections at /ws/node
func (s *Server) HandleNodeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WARN] WebSocket upgrade failed: %v", err)
		return
	}

	conn.SetReadDeadline(time.Now().Add(readDeadline))

	nc := &NodeConnection{
		Conn:        conn,
		ConnectedAt: time.Now(),
	}

	_, msg, err := conn.ReadMessage()
	if err != nil {
		log.Printf("[WARN] Failed to read hello: %v", err)
		conn.Close()
		return
	}

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

	s.mu.Lock()
	if existing, exists := s.connections[hello.MAC]; exists {
		existing.Conn.Close()
	}
	s.connections[hello.MAC] = nc
	s.malformedCounts[hello.MAC] = &malformedCounter{}
	broadcaster := s.dashboardBroadcaster
	fleetFn := s.fleetNotifier
	s.mu.Unlock()

	log.Printf("[INFO] Node connected: MAC=%s firmware=%s chip=%s",
		hello.MAC, hello.FirmwareVersion, hello.Chip)

	if broadcaster != nil {
		broadcaster.BroadcastNodeConnected(hello.MAC, hello.FirmwareVersion, hello.Chip)
	}

	if fleetFn != nil {
		fleetFn.OnNodeConnected(hello.MAC, hello.FirmwareVersion, hello.Chip)
	} else {
		s.sendRole(nc, "rx", "")
		s.sendConfig(nc, RateIdle, 0, DefaultVarianceThreshold)
	}

	// Notify OTA manager of reconnection (for rollback detection)
	s.mu.RLock()
	otaH := s.otaHandler
	s.mu.RUnlock()
	if otaH != nil {
		otaH.OnNodeReconnected(hello.MAC, hello.FirmwareVersion)
	}

	go s.pingLoop(nc)
	s.handleMessages(nc)
}

// handleMessages processes incoming WebSocket messages
func (s *Server) handleMessages(nc *NodeConnection) {
	defer func() {
		nc.Conn.Close()
		s.mu.Lock()
		delete(s.connections, nc.MAC)
		delete(s.malformedCounts, nc.MAC)
		broadcaster := s.dashboardBroadcaster
		rateCtrl := s.rateCtrl
		fleetFn := s.fleetNotifier
		s.mu.Unlock()

		log.Printf("[INFO] Node disconnected: MAC=%s", nc.MAC)

		if broadcaster != nil {
			broadcaster.BroadcastNodeDisconnected(nc.MAC)
		}
		if rateCtrl != nil {
			rateCtrl.OnNodeDisconnected(nc.MAC)
		}
		if fleetFn != nil {
			fleetFn.OnNodeDisconnected(nc.MAC)
		}
	}()

	for {
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

	nc.LastFrameTime = time.Now()
	recvTime := nc.LastFrameTime

	s.mu.RLock()
	replay := s.replayStore
	rec := s.recorder
	pm := s.processorMgr
	s.mu.RUnlock()

	// 1. Record raw frame to disk before any processing.
	if replay != nil {
		if err := replay.Append(recvTime.UnixNano(), data); err != nil {
			log.Printf("[WARN] Replay append error: %v", err)
		}
	}
	if rec != nil {
		rec.Write(frame.LinkID(), data)
	}

	// 2. Get or create ring buffer.
	linkID := frame.LinkID()
	s.mu.Lock()
	ring, exists := s.links[linkID]
	isNewLink := !exists
	if !exists {
		ring = NewRingBuffer()
		s.links[linkID] = ring
	}
	broadcaster := s.dashboardBroadcaster
	s.mu.Unlock()

	ring.Push(frame, recvTime)

	if broadcaster != nil {
		broadcaster.BroadcastCSI(frame.MACString(), frame.PeerMACString(), data)
		if isNewLink {
			broadcaster.BroadcastLinkActive(linkID, frame.MACString(), frame.PeerMACString())
		}
	}

	// 3. Signal processing pipeline.
	if pm != nil && int(frame.NSub) > 0 {
		result, err := pm.Process(linkID, frame.Payload, frame.RSSI, int(frame.NSub), recvTime)
		if err != nil {
			log.Printf("[DEBUG] Signal processing error for %s: %v", linkID, err)
			return
		}

		motionDetected := result.Features.MotionDetected
		deltaRMS := result.Features.SmoothDeltaRMS

		// Check if motion state changed.
		s.mu.Lock()
		prev := s.linkMotionState[linkID]
		stateChanged := prev != motionDetected
		s.linkMotionState[linkID] = motionDetected
		s.linkDeltaRMS[linkID] = deltaRMS
		mb := s.motionBroadcaster
		rateCtrl := s.rateCtrl
		s.mu.Unlock()

		if stateChanged && mb != nil {
			mb.BroadcastMotionState([]MotionStateItem{{
				LinkID:         linkID,
				MotionDetected: motionDetected,
				DeltaRMS:       deltaRMS,
			}})
		}

		if rateCtrl != nil {
			rateCtrl.OnMotionState(nc.MAC, motionDetected)
		}
	}
}

// handleJSONMessage processes a JSON control message
func (s *Server) handleJSONMessage(nc *NodeConnection, data []byte) {
	parsed, err := ParseJSONMessage(data)
	if err != nil {
		return
	}

	switch msg := parsed.(type) {
	case *HealthMessage:
		nc.LastHealth = msg
		nc.LastHealthTime = time.Now()

	case *BLEMessage:
		// TODO: forward BLE data to identity matcher

	case *MotionHintMessage:
		s.mu.RLock()
		rateCtrl := s.rateCtrl
		s.mu.RUnlock()
		if rateCtrl != nil {
			rateCtrl.OnMotionHint(nc.MAC)
		}

	case *OTAStatusMessage:
		log.Printf("[INFO] OTA %s from %s: state=%s progress=%d%%",
			msg.Type, nc.MAC, msg.State, msg.ProgressPct)
		if msg.Error != "" {
			log.Printf("[WARN] OTA error from %s: %s", nc.MAC, msg.Error)
		}
		s.mu.RLock()
		otaH := s.otaHandler
		s.mu.RUnlock()
		if otaH != nil {
			otaH.OnOTAStatus(nc.MAC, msg.State, uint8(msg.ProgressPct), msg.Error)
		}
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

	if time.Since(counter.firstSeen) > malformedWindow {
		counter.count = 0
		counter.firstSeen = time.Now()
	}

	counter.count++

	if counter.count == malformedWarnThreshold {
		log.Printf("[WARN] Node %s sending malformed CSI frames (count=%d)", mac, counter.count)
	}

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
			return
		}

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

// NodeInfo represents a connected node's state for dashboard
type NodeInfo struct {
	MAC             string `json:"mac"`
	FirmwareVersion string `json:"firmware_version,omitempty"`
	Chip            string `json:"chip,omitempty"`
}

// GetConnectedNodesInfo returns detailed info about connected nodes
func (s *Server) GetConnectedNodesInfo() []NodeInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	nodes := make([]NodeInfo, 0, len(s.connections))
	for mac, nc := range s.connections {
		info := NodeInfo{MAC: mac}
		if nc.Hello != nil {
			info.FirmwareVersion = nc.Hello.FirmwareVersion
			info.Chip = nc.Hello.Chip
		}
		nodes = append(nodes, info)
	}
	return nodes
}

// LinkInfo represents a link with its endpoints
type LinkInfo struct {
	ID      string `json:"id"`
	NodeMAC string `json:"node_mac"`
	PeerMAC string `json:"peer_mac"`
}

// LinkHealthInfo represents a link with health metrics for the API response
type LinkHealthInfo struct {
	LinkID        string               `json:"link_id"`
	TXMAC         string               `json:"tx_mac"`
	RXMAC         string               `json:"rx_mac"`
	HealthScore   float64              `json:"health_score"`
	HealthDetails signal.HealthDetails `json:"health_details"`
	LastUpdated   time.Time            `json:"last_updated"`
}

// GetAllLinksInfo returns detailed info about all active links
func (s *Server) GetAllLinksInfo() []LinkInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	links := make([]LinkInfo, 0, len(s.links))
	for linkID := range s.links {
		if len(linkID) >= 35 {
			links = append(links, LinkInfo{
				ID:      linkID,
				NodeMAC: linkID[:17],
				PeerMAC: linkID[18:],
			})
		}
	}
	return links
}

// GetAllMotionStates returns current motion state for all known links.
func (s *Server) GetAllMotionStates() []MotionStateItem {
	s.mu.RLock()
	pm := s.processorMgr
	s.mu.RUnlock()

	s.mu.RLock()
	defer s.mu.RUnlock()

	states := make([]MotionStateItem, 0, len(s.linkMotionState))
	for linkID, detected := range s.linkMotionState {
		item := MotionStateItem{
			LinkID:         linkID,
			MotionDetected: detected,
			DeltaRMS:       s.linkDeltaRMS[linkID],
		}
		if pm != nil {
			if proc := pm.GetProcessor(linkID); proc != nil {
				item.Confidence = proc.GetBaseline().GetConfidence()
				item.DiurnalConfidence = proc.GetDiurnal().GetOverallConfidence()
				item.DiurnalReady = proc.GetDiurnal().IsReady()
			}
		}
		states = append(states, item)
	}
	return states
}

// GetLinkBuffer returns the ring buffer for a specific link
func (s *Server) GetLinkBuffer(nodeMAC, peerMAC string) *RingBuffer {
	linkID := nodeMAC + ":" + peerMAC
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.links[linkID]
}

// GetAllLinksWithHealth returns all links with their health metrics
func (s *Server) GetAllLinksWithHealth() []LinkHealthInfo {
	s.mu.RLock()
	pm := s.processorMgr
	links := make([]string, 0, len(s.links))
	for linkID := range s.links {
		links = append(links, linkID)
	}
	s.mu.RUnlock()

	result := make([]LinkHealthInfo, 0, len(links))
	for _, linkID := range links {
		if len(linkID) < 35 {
			continue
		}

		info := LinkHealthInfo{
			LinkID: linkID,
			TXMAC:  linkID[:17],
			RXMAC:  linkID[18:],
		}

		if pm != nil {
			if proc := pm.GetProcessor(linkID); proc != nil {
				health := proc.GetHealth()
				if health != nil {
					info.HealthScore = health.GetAmbientConfidence()
					info.HealthDetails = health.GetHealthDetails()
					info.LastUpdated = time.Now()
				}
			}
		}

		// Default health if not available
		if info.HealthScore == 0 && info.HealthDetails == (signal.HealthDetails{}) {
			info.HealthScore = 0.5
			info.HealthDetails = signal.HealthDetails{
				SNR:           0.5,
				PhaseStability: 0.5,
				PacketRate:    0.5,
				BaselineDrift: 0.5,
			}
		}

		result = append(result, info)
	}
	return result
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
