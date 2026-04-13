package dashboard

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spaxel/mothership/internal/replay"
)

// parseISO8601 parses an ISO8601 timestamp string and returns Unix milliseconds
func parseISO8601(s string) (int64, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		// Try alternative formats
		t, err = time.Parse("2006-01-02T15:04:05", s)
		if err != nil {
			t, err = time.Parse("2006-01-02T15:04:05Z", s)
			if err != nil {
				t, err = time.Parse("2006-01-02T15:04:05.999Z", s)
				if err != nil {
					return 0, err
				}
			}
		}
	}
	return t.UnixMilli(), nil
}

const (
	// Dashboard ping/pong timing
	dashboardPingInterval = 30 * time.Second
	dashboardReadDeadline = 60 * time.Second

	// Send buffer size per client
	sendBufferSize = 1024
)

// Server handles WebSocket connections from dashboard clients
type Server struct {
	hub      *Hub
	upgrader websocket.Upgrader
}

// NewServer creates a new dashboard server
func NewServer(hub *Hub) *Server {
	return &Server{
		hub: hub,
		upgrader: websocket.Upgrader{
			// Allow all origins for development
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
			ReadBufferSize:  256,
			WriteBufferSize: 4096,
		},
	}
}

// HandleDashboardWS handles WebSocket connections at /ws/dashboard
func (s *Server) HandleDashboardWS(w http.ResponseWriter, r *http.Request) {
	// Upgrade HTTP connection to WebSocket
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WARN] Dashboard WebSocket upgrade failed: %v", err)
		return
	}

	// Create client
	client := &Client{
		hub:  s.hub,
		send: make(chan []byte, sendBufferSize),
	}

	// Register with hub
	s.hub.Register(client)

	// Start write goroutine
	go s.writePump(conn, client)

	// Run read pump in this goroutine
	s.readPump(conn, client)
}

// readPump handles incoming messages from the dashboard client
func (s *Server) readPump(conn *websocket.Conn, client *Client) {
	defer func() {
		conn.Close()
		s.hub.Unregister(client)
	}()

	// Set read deadline
	conn.SetReadDeadline(time.Now().Add(dashboardReadDeadline))

	// Set pong handler to reset deadline
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(dashboardReadDeadline))
		return nil
	})

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[WARN] Dashboard read error: %v", err)
			}
			break
		}

		// Handle WebSocket commands from dashboard
		s.handleCommand(message, client)
	}
}

// handleCommand processes WebSocket commands from the dashboard client
func (s *Server) handleCommand(data []byte, client *Client) {
	var cmd map[string]interface{}
	if err := json.Unmarshal(data, &cmd); err != nil {
		log.Printf("[DEBUG] Failed to parse WebSocket command: %v", err)
		return
	}

	cmdType, ok := cmd["type"].(string)
	if !ok {
		return
	}

	switch cmdType {
	case "replay_seek":
		s.handleReplaySeek(cmd)
	case "replay_play":
		s.handleReplayPlay(cmd)
	case "replay_pause":
		s.handleReplayPause(cmd)
	case "replay_set_params":
		s.handleReplaySetParams(cmd)
	case "replay_apply_to_live":
		s.handleReplayApplyToLive(cmd)
	case "replay_set_speed":
		s.handleReplaySetSpeed(cmd)
	case "request_explain":
		s.handleRequestExplain(cmd)
	default:
		// Unknown command type - ignore
		log.Printf("[DEBUG] Unknown WebSocket command type: %s", cmdType)
	}
}

// handleReplaySeek handles replay_seek commands
func (s *Server) handleReplaySeek(cmd map[string]interface{}) {
	targetISO, ok := cmd["timestamp_iso8601"].(string)
	if !ok {
		log.Printf("[WARN] replay_seek missing timestamp_iso8601")
		return
	}

	targetMS, err := parseISO8601(targetISO)
	if err != nil {
		log.Printf("[WARN] replay_seek invalid timestamp: %v", err)
		return
	}

	// Forward to replay handler if available
	if s.hub.replayHandler != nil {
		s.hub.replayHandler.SeekTo(targetMS)
	}
}

// handleReplayPlay handles replay_play commands
func (s *Server) handleReplayPlay(cmd map[string]interface{}) {
	speedVal, ok := cmd["speed"]
	var speed float64 = 1.0
	if ok {
		switch v := speedVal.(type) {
		case float64:
			speed = v
		case int:
			speed = float64(v)
		}
	}

	// Forward to replay handler if available
	if s.hub.replayHandler != nil {
		s.hub.replayHandler.Play(speed)
	}
}

// handleReplayPause handles replay_pause commands
func (s *Server) handleReplayPause(cmd map[string]interface{}) {
	// Forward to replay handler if available
	if s.hub.replayHandler != nil {
		s.hub.replayHandler.Pause()
	}
}

// handleReplaySetParams handles replay_set_params commands
func (s *Server) handleReplaySetParams(cmd map[string]interface{}) {
	params := &replay.TunableParams{}

	if val, ok := cmd["delta_rms_threshold"]; ok {
		if f, ok := val.(float64); ok {
			params.DeltaRMSThreshold = &f
		}
	}
	if val, ok := cmd["tau_s"]; ok {
		if f, ok := val.(float64); ok {
			params.TauS = &f
		}
	}
	if val, ok := cmd["fresnel_decay"]; ok {
		if f, ok := val.(float64); ok {
			params.FresnelDecay = &f
		}
	}
	if val, ok := cmd["n_subcarriers"]; ok {
		if i, ok := val.(float64); ok {
			ival := int(i)
			params.NSubcarriers = &ival
		}
	}
	if val, ok := cmd["breathing_sensitivity"]; ok {
		if f, ok := val.(float64); ok {
			params.BreathingSensitivity = &f
		}
	}

	// Forward to replay handler if available
	if s.hub.replayHandler != nil {
		s.hub.replayHandler.SetParams(params)
	}
}

// handleReplayApplyToLive handles replay_apply_to_live commands
func (s *Server) handleReplayApplyToLive(cmd map[string]interface{}) {
	// This would copy replay parameters to live configuration
	// Requires confirmation from user (handled on frontend)
	log.Printf("[INFO] Apply replay parameters to live requested")

	// Forward to replay handler if available
	if s.hub.replayHandler != nil {
		s.hub.replayHandler.ApplyToLive()
	}
}

// handleReplaySetSpeed handles replay_set_speed commands
func (s *Server) handleReplaySetSpeed(cmd map[string]interface{}) {
	speedVal, ok := cmd["speed"]
	var speed float64 = 1.0
	if ok {
		switch v := speedVal.(type) {
		case float64:
			speed = v
		case int:
			speed = float64(v)
		}
	}

	// Forward to replay handler if available
	if s.hub.replayHandler != nil {
		s.hub.replayHandler.SetSpeed(speed)
	}
}

// writePump handles outgoing messages to the dashboard client
func (s *Server) writePump(conn *websocket.Conn, client *Client) {
	ticker := time.NewTicker(dashboardPingInterval)
	defer func() {
		ticker.Stop()
		conn.Close()
	}()

	for {
		select {
		case message, ok := <-client.send:
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				// Hub closed the channel
				conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// Determine message type (binary vs text)
			if len(message) > 0 && message[0] == '{' {
				// Looks like JSON, send as text
				err := conn.WriteMessage(websocket.TextMessage, message)
				if err != nil {
					log.Printf("[WARN] Dashboard write error: %v", err)
					return
				}
			} else {
				// Binary CSI frame
				err := conn.WriteMessage(websocket.BinaryMessage, message)
				if err != nil {
					log.Printf("[WARN] Dashboard write error: %v", err)
					return
				}
			}

		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleRequestExplain handles "request_explain" commands from the dashboard.
// The client sends {"type":"request_explain","blob_id":N} to request that the
// server emit a "blob_explain" message on the next fusion tick.
func (s *Server) handleRequestExplain(cmd map[string]interface{}) {
	var blobID int
	switch v := cmd["blob_id"].(type) {
	case float64:
		blobID = int(v)
	case int:
		blobID = v
	default:
		log.Printf("[WARN] request_explain: missing or invalid blob_id field")
		return
	}
	s.hub.RequestExplain(blobID)
	log.Printf("[DEBUG] request_explain queued for blob %d", blobID)
}

// Hub returns the server's hub for external use
func (s *Server) Hub() *Hub {
	return s.hub
}

// Client represents a dashboard WebSocket client
// (redeclared here for documentation; defined in hub.go)
type dashboardClient = Client
