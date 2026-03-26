package dashboard

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

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
		_, _, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[WARN] Dashboard read error: %v", err)
			}
			break
		}
		// Dashboard clients don't send meaningful messages in Phase 1
		// Just keep the connection alive
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

// Hub returns the server's hub for external use
func (s *Server) Hub() *Hub {
	return s.hub
}

// Client represents a dashboard WebSocket client
// (redeclared here for documentation; defined in hub.go)
type dashboardClient = Client
