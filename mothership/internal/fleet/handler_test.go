package fleet

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// mockNodeIdentifier is a mock implementation of NodeIdentifier for testing.
type mockNodeIdentifier struct {
	sendIdentifyFunc func(mac string, durationMS int) bool
}

func (m *mockNodeIdentifier) SendIdentifyToMAC(mac string, durationMS int) bool {
	if m.sendIdentifyFunc != nil {
		return m.sendIdentifyFunc(mac, durationMS)
	}
	return true
}

// mockRegistry is a minimal mock of Registry for testing.
type mockRegistry struct {
	nodes map[string]NodeRecord
	err   error
}

func (m *mockRegistry) GetNode(mac string) (NodeRecord, error) {
	if m.err != nil {
		return NodeRecord{}, m.err
	}
	if node, ok := m.nodes[mac]; ok {
		return node, nil
	}
	return NodeRecord{}, sql.ErrNoRows
}

func (m *mockRegistry) GetAllNodes() ([]NodeRecord, error) {
	var nodes []NodeRecord
	for _, node := range m.nodes {
		nodes = append(nodes, node)
	}
	return nodes, m.err
}

func (m *mockRegistry) SetNodePosition(mac string, x, y, z float64) error {
	return nil
}

func (m *mockRegistry) AddVirtualNode(mac, name string, x, y, z float64) error {
	return nil
}

func (m *mockRegistry) DeleteNode(mac string) error {
	return nil
}

func (m *mockRegistry) SetRoom(room RoomConfig) error {
	return nil
}

func (m *mockRegistry) GetRoom() (RoomConfig, error) {
	return RoomConfig{}, nil
}

func (m *mockRegistry) GetNodesByRole(role string) ([]NodeRecord, error) {
	return nil, nil
}

func TestHandlerIdentifyNode(t *testing.T) {
	tests := []struct {
		name           string
		mac            string
		reqBody        string
		nodeExists     bool
		nodeConnected  bool
		wantStatus     int
		wantResponse   string
	}{
		{
			name:          "successful identify with default duration",
			mac:           "AA:BB:CC:DD:EE:FF",
			reqBody:       `{}`,
			nodeExists:    true,
			nodeConnected: true,
			wantStatus:    http.StatusOK,
			wantResponse:  `{"ok":true}`,
		},
		{
			name:          "successful identify with custom duration",
			mac:           "AA:BB:CC:DD:EE:FF",
			reqBody:       `{"duration_ms": 10000}`,
			nodeExists:    true,
			nodeConnected: true,
			wantStatus:    http.StatusOK,
			wantResponse:  `{"ok":true}`,
		},
		{
			name:          "node not found",
			mac:           "AA:BB:CC:DD:EE:FF",
			reqBody:       `{}`,
			nodeExists:    false,
			nodeConnected: true,
			wantStatus:    http.StatusNotFound,
			wantResponse:  "node not found\n",
		},
		{
			name:          "node not connected",
			mac:           "AA:BB:CC:DD:EE:FF",
			reqBody:       `{}`,
			nodeExists:    true,
			nodeConnected: false,
			wantStatus:    http.StatusNotFound,
			wantResponse:  "node not connected\n",
		},
		{
			name:          "invalid request body",
			mac:           "AA:BB:CC:DD:EE:FF",
			reqBody:       `invalid json`,
			nodeExists:    true,
			nodeConnected: true,
			wantStatus:    http.StatusBadRequest,
			wantResponse:  "invalid request body\n",
		},
		{
			name:          "zero duration uses default",
			mac:           "AA:BB:CC:DD:EE:FF",
			reqBody:       `{"duration_ms": 0}`,
			nodeExists:    true,
			nodeConnected: true,
			wantStatus:    http.StatusOK,
			wantResponse:  `{"ok":true}`,
		},
		{
			name:          "negative duration uses default",
			mac:           "AA:BB:CC:DD:EE:FF",
			reqBody:       `{"duration_ms": -1000}`,
			nodeExists:    true,
			nodeConnected: true,
			wantStatus:    http.StatusOK,
			wantResponse:  `{"ok":true}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock registry with the test node
			reg := &mockRegistry{
				nodes: make(map[string]NodeRecord),
			}
			if tt.nodeExists {
				reg.nodes[tt.mac] = NodeRecord{
					MAC:  tt.mac,
					Name: "Test Node",
					Role: "rx",
				}
			}

			// Create a manager with the mock registry
			mgr := &Manager{
				registry: reg,
			}

			// Create handler with mock node identifier
			h := &Handler{
				mgr: mgr,
				nodeID: &mockNodeIdentifier{
					sendIdentifyFunc: func(mac string, durationMS int) bool {
						return tt.nodeConnected
					},
				},
			}

			// Create a test request
			req := httptest.NewRequest("POST", "/api/nodes/"+tt.mac+"/identify", bytes.NewBufferString(tt.reqBody))
			req.Header.Set("Content-Type", "application/json")

			// Use chi URLParam to set the MAC parameter
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("mac", tt.mac)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			// Create response recorder
			w := httptest.NewRecorder()

			// Call the handler
			h.identifyNode(w, req)

			// Check status code
			if w.Code != tt.wantStatus {
				t.Errorf("identifyNode() status = %v, want %v", w.Code, tt.wantStatus)
			}

			// Check response body
			if tt.wantResponse != "" {
				resp := w.Body.String()
				if resp != tt.wantResponse {
					t.Errorf("identifyNode() response = %q, want %q", resp, tt.wantResponse)
				}
			}
		})
	}
}

func TestHandlerIdentifyNodeDurationParsing(t *testing.T) {
	tests := []struct {
		name            string
		reqBody         string
		expectedDuration int
	}{
		{
			name:            "default duration when not specified",
			reqBody:         `{}`,
			expectedDuration: 5000,
		},
		{
			name:            "custom duration",
			reqBody:         `{"duration_ms": 10000}`,
			expectedDuration: 10000,
		},
		{
			name:            "zero uses default",
			reqBody:         `{"duration_ms": 0}`,
			expectedDuration: 5000,
		},
		{
			name:            "negative uses default",
			reqBody:         `{"duration_ms": -1000}`,
			expectedDuration: 5000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var actualDuration int

			reg := &mockRegistry{
				nodes: map[string]NodeRecord{
					"AA:BB:CC:DD:EE:FF": {
						MAC:  "AA:BB:CC:DD:EE:FF",
						Name: "Test Node",
						Role: "rx",
					},
				},
			}

			mgr := &Manager{
				registry: reg,
			}

			h := &Handler{
				mgr: mgr,
				nodeID: &mockNodeIdentifier{
					sendIdentifyFunc: func(mac string, durationMS int) bool {
						actualDuration = durationMS
						return true
					},
				},
			}

			req := httptest.NewRequest("POST", "/api/nodes/AA:BB:CC:DD:EE:FF/identify", bytes.NewBufferString(tt.reqBody))
			req.Header.Set("Content-Type", "application/json")

			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("mac", "AA:BB:CC:DD:EE:FF")
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			w := httptest.NewRecorder()
			h.identifyNode(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status OK, got %v", w.Code)
			}

			if actualDuration != tt.expectedDuration {
				t.Errorf("Duration = %v, want %v", actualDuration, tt.expectedDuration)
			}
		})
	}
}

func TestIdentifyNodeRequest(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
	}{
		{
			name:    "valid empty object",
			json:    `{}`,
			wantErr: false,
		},
		{
			name:    "valid with duration",
			json:    `{"duration_ms": 10000}`,
			wantErr: false,
		},
		{
			name:    "valid with zero duration",
			json:    `{"duration_ms": 0}`,
			wantErr: false,
		},
		{
			name:    "valid with negative duration",
			json:    `{"duration_ms": -1000}`,
			wantErr: false,
		},
		{
			name:    "invalid json",
			json:    `invalid`,
			wantErr: true,
		},
		{
			name:    "extra fields ignored",
			json:    `{"duration_ms": 5000, "extra": "ignored"}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req identifyNodeRequest
			err := json.NewDecoder(bytes.NewBufferString(tt.json)).Decode(&req)

			if (err != nil) != tt.wantErr {
				t.Errorf("json.NewDecoder().Decode() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ─── System mode endpoint tests ─────────────────────────────────────────────

func TestHandlerGetSystemMode(t *testing.T) {
	reg := &mockRegistry{
		nodes: make(map[string]NodeRecord),
	}

	mgr := &Manager{
		registry: reg,
	}

	h := &Handler{mgr: mgr}

	req := httptest.NewRequest("GET", "/api/mode", nil)
	w := httptest.NewRecorder()

	h.getSystemMode(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("getSystemMode() status = %v, want %v", w.Code, http.StatusOK)
	}

	var resp systemModeResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Mode != "home" {
		t.Errorf("Expected mode to be home, got %s", resp.Mode)
	}

	if !resp.AutoAwayConfig.Enabled {
		t.Errorf("Expected auto-away to be enabled by default")
	}

	if resp.AutoAwayConfig.AbsenceDurationSec != 900 { // 15 minutes
		t.Errorf("Expected absence duration to be 900s, got %d", resp.AutoAwayConfig.AbsenceDurationSec)
	}
}

func TestHandlerSetSystemMode(t *testing.T) {
	tests := []struct {
		name         string
		requestBody  string
		wantStatus   int
		expectedMode string
	}{
		{
			name:         "set to away mode",
			requestBody:  `{"mode": "away", "reason": "manual"}`,
			wantStatus:   http.StatusOK,
			expectedMode: "away",
		},
		{
			name:         "set to home mode",
			requestBody:  `{"mode": "home", "reason": "manual"}`,
			wantStatus:   http.StatusOK,
			expectedMode: "home",
		},
		{
			name:         "set to sleep mode",
			requestBody:  `{"mode": "sleep", "reason": "night"}`,
			wantStatus:   http.StatusOK,
			expectedMode: "sleep",
		},
		{
			name:         "mode defaults reason to manual",
			requestBody:  `{"mode": "away"}`,
			wantStatus:   http.StatusOK,
			expectedMode: "away",
		},
		{
			name:         "invalid mode",
			requestBody:  `{"mode": "invalid"}`,
			wantStatus:   http.StatusBadRequest,
		},
		{
			name:         "invalid json",
			requestBody:  `invalid json`,
			wantStatus:   http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &mockRegistry{
				nodes: make(map[string]NodeRecord),
			}

			mgr := &Manager{
				registry: reg,
			}

			h := &Handler{mgr: mgr}

			req := httptest.NewRequest("POST", "/api/mode", bytes.NewBufferString(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()

			h.setSystemMode(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("setSystemMode() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.wantStatus == http.StatusOK {
				var resp systemModeResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}

				if resp.Mode != tt.expectedMode {
					t.Errorf("Expected mode to be %s, got %s", tt.expectedMode, resp.Mode)
				}
			}
		})
	}
}
