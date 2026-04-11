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
	sendIdentifyFunc  func(mac string, durationMS int) bool
	sendRebootFunc    func(mac string, delayMS int) bool
	getConnectedMACs  func() []string
}

func (m *mockNodeIdentifier) SendIdentifyToMAC(mac string, durationMS int) bool {
	if m.sendIdentifyFunc != nil {
		return m.sendIdentifyFunc(mac, durationMS)
	}
	return true
}

func (m *mockNodeIdentifier) SendRebootToMAC(mac string, delayMS int) bool {
	if m.sendRebootFunc != nil {
		return m.sendRebootFunc(mac, delayMS)
	}
	return true
}

func (m *mockNodeIdentifier) GetConnectedMACs() []string {
	if m.getConnectedMACs != nil {
		return m.getConnectedMACs()
	}
	return []string{}
}

// mockRegistry is a mock implementation of Registry for testing.
type mockRegistry struct {
	nodes map[string]NodeRecord
}

func (m *mockRegistry) GetNode(mac string) (*NodeRecord, error) {
	if node, ok := m.nodes[mac]; ok {
		return &node, nil
	}
	return nil, sql.ErrNoRows
}

func (m *mockRegistry) GetAllNodes() ([]NodeRecord, error) {
	var nodes []NodeRecord
	for _, node := range m.nodes {
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func (m *mockRegistry) SetNodeLabel(mac, label string) error {
	if node, ok := m.nodes[mac]; ok {
		node.Name = label
		m.nodes[mac] = node
		return nil
	}
	return sql.ErrNoRows
}

func (m *mockRegistry) SetNodePosition(mac string, x, y, z float64) error {
	if node, ok := m.nodes[mac]; ok {
		node.PosX = x
		node.PosY = y
		node.PosZ = z
		m.nodes[mac] = node
		return nil
	}
	return sql.ErrNoRows
}

func (m *mockRegistry) AddVirtualNode(mac, name string, x, y, z float64) error {
	m.nodes[mac] = NodeRecord{
		MAC:     mac,
		Name:    name,
		Role:    "virtual",
		PosX:    x,
		PosY:    y,
		PosZ:    z,
		Virtual: true,
	}
	return nil
}

func (m *mockRegistry) DeleteNode(mac string) error {
	delete(m.nodes, mac)
	return nil
}

func (m *mockRegistry) UpsertNode(mac, firmware, chip string) error {
	if _, ok := m.nodes[mac]; !ok {
		m.nodes[mac] = NodeRecord{
			MAC:             mac,
			Name:            "",
			Role:            "rx",
			FirmwareVersion: firmware,
			ChipModel:       chip,
		}
	} else {
		node := m.nodes[mac]
		node.FirmwareVersion = firmware
		node.ChipModel = chip
		m.nodes[mac] = node
	}
	return nil
}

func (m *mockRegistry) SetNodeRole(mac, role string) error {
	if node, ok := m.nodes[mac]; ok {
		node.Role = role
		m.nodes[mac] = node
		return nil
	}
	return sql.ErrNoRows
}

func (m *mockRegistry) Close() error {
	return nil
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
			// Create a real registry with the test node
			reg := newTestRegistry(t)
			if tt.nodeExists {
				err := reg.UpsertNode(tt.mac, "1.0.0", "ESP32-S3")
				if err != nil {
					t.Fatalf("UpsertNode: %v", err)
				}
			}

			// Create a manager with the registry
			mgr := NewManager(reg)

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

			reg := newTestRegistry(t)
			err := reg.UpsertNode("AA:BB:CC:DD:EE:FF", "1.0.0", "ESP32-S3")
			if err != nil {
				t.Fatalf("UpsertNode: %v", err)
			}

			mgr := NewManager(reg)

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
	reg := newTestRegistry(t)
	mgr := NewManager(reg)
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

// ─── Fleet list endpoint tests ────────────────────────────────────────────────

func TestHandlerListFleet(t *testing.T) {
	reg := newTestRegistry(t)
	reg.UpsertNode("AA:BB:CC:DD:EE:FF", "1.0.0", "ESP32-S3")
	reg.SetNodeLabel("AA:BB:CC:DD:EE:FF", "Node 1")
	reg.SetNodePosition("AA:BB:CC:DD:EE:FF", 1.0, 2.0, 3.0)
	reg.UpsertNode("11:22:33:44:55:66", "1.1.0", "ESP32-S3")
	reg.SetNodeLabel("11:22:33:44:55:66", "Node 2")
	reg.SetNodePosition("11:22:33:44:55:66", 4.0, 5.0, 6.0)

	mgr := NewManager(reg)

	h := &Handler{
		mgr: mgr,
		nodeID: &mockNodeIdentifier{
			sendIdentifyFunc: func(mac string, durationMS int) bool {
				return true
			},
			getConnectedMACs: func() []string {
				return []string{"AA:BB:CC:DD:EE:FF"}
			},
		},
	}

	req := httptest.NewRequest("GET", "/api/fleet", nil)
	w := httptest.NewRecorder()

	h.listFleet(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("listFleet() status = %v, want %v", w.Code, http.StatusOK)
	}

	var nodes []FleetNode
	if err := json.NewDecoder(w.Body).Decode(&nodes); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(nodes) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(nodes))
	}

	// Check first node (should be online)
	if nodes[0].MAC != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("Expected first node MAC to be AA:BB:CC:DD:EE:FF, got %s", nodes[0].MAC)
	}
	if nodes[0].Status != "online" {
		t.Errorf("Expected first node status to be online, got %s", nodes[0].Status)
	}

	// Check second node (should be offline - not in connected list)
	if nodes[1].MAC != "11:22:33:44:55:66" {
		t.Errorf("Expected second node MAC to be 11:22:33:44:55:66, got %s", nodes[1].MAC)
	}
	if nodes[1].Status != "offline" {
		t.Errorf("Expected second node status to be offline, got %s", nodes[1].Status)
	}
}

func TestHandlerListFleetEmpty(t *testing.T) {
	reg := newTestRegistry(t)

	mgr := NewManager(reg)

	h := &Handler{mgr: mgr}

	req := httptest.NewRequest("GET", "/api/fleet", nil)
	w := httptest.NewRecorder()

	h.listFleet(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("listFleet() status = %v, want %v", w.Code, http.StatusOK)
	}

	var nodes []FleetNode
	if err := json.NewDecoder(w.Body).Decode(&nodes); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(nodes) != 0 {
		t.Errorf("Expected 0 nodes, got %d", len(nodes))
	}
}

// ─── Get node endpoint tests ───────────────────────────────────────────────────

func TestHandlerGetNode(t *testing.T) {
	tests := []struct {
		name       string
		mac        string
		nodeExists bool
		wantStatus int
	}{
		{
			name:       "node found",
			mac:        "AA:BB:CC:DD:EE:FF",
			nodeExists: true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "node not found",
			mac:        "AA:BB:CC:DD:EE:FF",
			nodeExists: false,
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := newTestRegistry(t)
			if tt.nodeExists {
				reg.UpsertNode(tt.mac, "1.0.0", "ESP32-S3")
				reg.SetNodeLabel(tt.mac, "Test Node")
			}

			mgr := NewManager(reg)

			h := &Handler{mgr: mgr}

			req := httptest.NewRequest("GET", "/api/nodes/"+tt.mac, nil)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("mac", tt.mac)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			w := httptest.NewRecorder()
			h.getNode(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("getNode() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.wantStatus == http.StatusOK {
				var node NodeRecord
				if err := json.NewDecoder(w.Body).Decode(&node); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				if node.MAC != tt.mac {
					t.Errorf("Expected MAC to be %s, got %s", tt.mac, node.MAC)
				}
			}
		})
	}
}

// ─── Update node label endpoint tests ───────────────────────────────────────────

func TestHandlerUpdateNodeLabel(t *testing.T) {
	tests := []struct {
		name          string
		mac           string
		requestBody   string
		nodeExists    bool
		wantStatus    int
		expectedLabel string
	}{
		{
			name:          "successful label update",
			mac:           "AA:BB:CC:DD:EE:FF",
			requestBody:   `{"label": "New Label"}`,
			nodeExists:    true,
			wantStatus:    http.StatusNoContent,
			expectedLabel: "New Label",
		},
		{
			name:        "node not found",
			mac:         "AA:BB:CC:DD:EE:FF",
			requestBody: `{"label": "New Label"}`,
			nodeExists:  false,
			wantStatus:  http.StatusNotFound,
		},
		{
			name:        "invalid request body",
			mac:         "AA:BB:CC:DD:EE:FF",
			requestBody: `invalid json`,
			nodeExists:  true,
			wantStatus:  http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &mockRegistry{
				nodes: make(map[string]NodeRecord),
			}
			if tt.nodeExists {
				reg.nodes[tt.mac] = NodeRecord{
					MAC:  tt.mac,
					Name: "Old Label",
					Role: "rx",
				}
			}

			mgr := &Manager{
				registry: reg,
			}

			h := &Handler{mgr: mgr}

			req := httptest.NewRequest("PATCH", "/api/nodes/"+tt.mac+"/label", bytes.NewBufferString(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("mac", tt.mac)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			w := httptest.NewRecorder()
			h.updateNodeLabel(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("updateNodeLabel() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.wantStatus == http.StatusNoContent {
				// Verify the label was updated
				if reg.nodes[tt.mac].Name != tt.expectedLabel {
					t.Errorf("Expected label to be %s, got %s", tt.expectedLabel, reg.nodes[tt.mac].Name)
				}
			}
		})
	}
}

// ─── Set node role endpoint tests ───────────────────────────────────────────────

func TestHandlerSetNodeRole(t *testing.T) {
	tests := []struct {
		name          string
		mac           string
		requestBody   string
		nodeExists    bool
		wantStatus    int
		expectedRole  string
	}{
		{
			name:         "successful role change to tx",
			mac:          "AA:BB:CC:DD:EE:FF",
			requestBody:  `{"role": "tx"}`,
			nodeExists:   true,
			wantStatus:   http.StatusOK,
			expectedRole: "tx",
		},
		{
			name:         "successful role change to rx",
			mac:          "AA:BB:CC:DD:EE:FF",
			requestBody:  `{"role": "rx"}`,
			nodeExists:   true,
			wantStatus:   http.StatusOK,
			expectedRole: "rx",
		},
		{
			name:         "successful role change to tx_rx",
			mac:          "AA:BB:CC:DD:EE:FF",
			requestBody:  `{"role": "tx_rx"}`,
			nodeExists:   true,
			wantStatus:   http.StatusOK,
			expectedRole: "tx_rx",
		},
		{
			name:         "successful role change to passive",
			mac:          "AA:BB:CC:DD:EE:FF",
			requestBody:  `{"role": "passive"}`,
			nodeExists:   true,
			wantStatus:   http.StatusOK,
			expectedRole: "passive",
		},
		{
			name:        "node not found",
			mac:         "AA:BB:CC:DD:EE:FF",
			requestBody: `{"role": "tx"}`,
			nodeExists:  false,
			wantStatus:  http.StatusNotFound,
		},
		{
			name:        "invalid role",
			mac:         "AA:BB:CC:DD:EE:FF",
			requestBody: `{"role": "invalid"}`,
			nodeExists:  true,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "invalid request body",
			mac:         "AA:BB:CC:DD:EE:FF",
			requestBody: `invalid json`,
			nodeExists:  true,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "empty role",
			mac:         "AA:BB:CC:DD:EE:FF",
			requestBody: `{"role": ""}`,
			nodeExists:  true,
			wantStatus:  http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			mgr := &Manager{
				registry: reg,
			}

			h := &Handler{mgr: mgr}

			req := httptest.NewRequest("POST", "/api/nodes/"+tt.mac+"/role", bytes.NewBufferString(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("mac", tt.mac)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			w := httptest.NewRecorder()
			h.setNodeRole(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("setNodeRole() status = %v, want %v", w.Code, tt.wantStatus)
			}
		})
	}
}

// ─── Delete node endpoint tests ─────────────────────────────────────────────────

func TestHandlerDeleteNode(t *testing.T) {
	tests := []struct {
		name       string
		mac        string
		nodeExists bool
		wantStatus int
	}{
		{
			name:       "successful deletion",
			mac:        "AA:BB:CC:DD:EE:FF",
			nodeExists: true,
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "delete non-existent node succeeds",
			mac:        "AA:BB:CC:DD:EE:FF",
			nodeExists: false,
			wantStatus: http.StatusNoContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			mgr := &Manager{
				registry: reg,
			}

			h := &Handler{mgr: mgr}

			req := httptest.NewRequest("DELETE", "/api/nodes/"+tt.mac, nil)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("mac", tt.mac)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			w := httptest.NewRecorder()
			h.deleteNode(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("deleteNode() status = %v, want %v", w.Code, tt.wantStatus)
			}
		})
	}
}

// ─── OTA endpoint tests ───────────────────────────────────────────────────────────

func TestHandlerTriggerNodeOTA(t *testing.T) {
	tests := []struct {
		name          string
		mac           string
		requestBody   string
		nodeExists    bool
		otaAvailable  bool
		wantStatus    int
	}{
		{
			name:         "successful OTA trigger",
			mac:          "AA:BB:CC:DD:EE:FF",
			requestBody:  `{}`,
			nodeExists:   true,
			otaAvailable: true,
			wantStatus:   http.StatusOK,
		},
		{
			name:         "OTA with specific version",
			mac:          "AA:BB:CC:DD:EE:FF",
			requestBody:  `{"version": "1.2.0"}`,
			nodeExists:   true,
			otaAvailable: true,
			wantStatus:   http.StatusOK,
		},
		{
			name:         "node not found",
			mac:          "AA:BB:CC:DD:EE:FF",
			requestBody:  `{}`,
			nodeExists:   false,
			otaAvailable: true,
			wantStatus:   http.StatusNotFound,
		},
		{
			name:         "invalid request body",
			mac:          "AA:BB:CC:DD:EE:FF",
			requestBody:  `invalid json`,
			nodeExists:   true,
			otaAvailable: true,
			wantStatus:   http.StatusBadRequest,
		},
		{
			name:         "OTA manager not available",
			mac:          "AA:BB:CC:DD:EE:FF",
			requestBody:  `{}`,
			nodeExists:   true,
			otaAvailable: false,
			wantStatus:   http.StatusOK, // Still succeeds without OTA manager
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &mockRegistry{
				nodes: make(map[string]NodeRecord),
			}
			if tt.nodeExists {
				reg.nodes[tt.mac] = NodeRecord{
					MAC:             tt.mac,
					Name:            "Test Node",
					Role:            "rx",
					FirmwareVersion: "1.0.0",
				}
			}

			mgr := &Manager{
				registry: reg,
			}

			h := &Handler{mgr: mgr}
			if tt.otaAvailable {
				// OTA manager is optional in the handler
				h.otaMgr = nil // Mock would go here
			}

			req := httptest.NewRequest("POST", "/api/nodes/"+tt.mac+"/ota", bytes.NewBufferString(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("mac", tt.mac)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			w := httptest.NewRecorder()
			h.triggerNodeOTA(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("triggerNodeOTA() status = %v, want %v", w.Code, tt.wantStatus)
			}
		})
	}
}

// ─── Reboot endpoint tests ─────────────────────────────────────────────────────────

func TestHandlerRebootNode(t *testing.T) {
	tests := []struct {
		name          string
		mac           string
		requestBody   string
		nodeExists    bool
		nodeConnected bool
		wantStatus    int
	}{
		{
			name:          "successful reboot with default delay",
			mac:           "AA:BB:CC:DD:EE:FF",
			requestBody:   `{}`,
			nodeExists:    true,
			nodeConnected: true,
			wantStatus:    http.StatusOK,
		},
		{
			name:          "successful reboot with custom delay",
			mac:           "AA:BB:CC:DD:EE:FF",
			requestBody:   `{"delay_ms": 5000}`,
			nodeExists:    true,
			nodeConnected: true,
			wantStatus:    http.StatusOK,
		},
		{
			name:          "node not found",
			mac:           "AA:BB:CC:DD:EE:FF",
			requestBody:   `{}`,
			nodeExists:    false,
			nodeConnected: true,
			wantStatus:    http.StatusNotFound,
		},
		{
			name:          "node not connected",
			mac:           "AA:BB:CC:DD:EE:FF",
			requestBody:   `{}`,
			nodeExists:    true,
			nodeConnected: false,
			wantStatus:    http.StatusNotFound,
		},
		{
			name:          "invalid request body",
			mac:           "AA:BB:CC:DD:EE:FF",
			requestBody:   `invalid json`,
			nodeExists:    true,
			nodeConnected: true,
			wantStatus:    http.StatusBadRequest,
		},
		{
			name:          "zero delay uses default",
			mac:           "AA:BB:CC:DD:EE:FF",
			requestBody:   `{"delay_ms": 0}`,
			nodeExists:    true,
			nodeConnected: true,
			wantStatus:    http.StatusOK,
		},
		{
			name:          "negative delay uses default",
			mac:           "AA:BB:CC:DD:EE:FF",
			requestBody:   `{"delay_ms": -1000}`,
			nodeExists:    true,
			nodeConnected: true,
			wantStatus:    http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			mgr := &Manager{
				registry: reg,
			}

			h := &Handler{
				mgr: mgr,
				nodeID: &mockNodeIdentifier{
					sendIdentifyFunc: func(mac string, delayMS int) bool {
						return tt.nodeConnected
					},
				},
			}

			req := httptest.NewRequest("POST", "/api/nodes/"+tt.mac+"/reboot", bytes.NewBufferString(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("mac", tt.mac)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			w := httptest.NewRecorder()
			h.rebootNode(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("rebootNode() status = %v, want %v", w.Code, tt.wantStatus)
			}
		})
	}
}

// ─── Update all endpoint tests ────────────────────────────────────────────────────

func TestHandlerUpdateAllNodes(t *testing.T) {
	tests := []struct {
		name           string
		connectedMACs  []string
		wantStatus     int
		expectedCount  int
	}{
		{
			name:          "update all connected nodes",
			connectedMACs: []string{"AA:BB:CC:DD:EE:FF", "11:22:33:44:55:66"},
			wantStatus:    http.StatusOK,
			expectedCount: 2,
		},
		{
			name:          "no connected nodes",
			connectedMACs: []string{},
			wantStatus:    http.StatusOK,
			expectedCount: 0,
		},
		{
			name:          "single connected node",
			connectedMACs: []string{"AA:BB:CC:DD:EE:FF"},
			wantStatus:    http.StatusOK,
			expectedCount: 1,
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

			h := &Handler{
				mgr: mgr,
				nodeID: &mockNodeIdentifier{
					GetConnectedMACs: func() []string {
						return tt.connectedMACs
					},
				},
			}

			req := httptest.NewRequest("POST", "/api/nodes/update-all", nil)
			w := httptest.NewRecorder()

			h.updateAllNodes(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("updateAllNodes() status = %v, want %v", w.Code, tt.wantStatus)
			}

			var resp map[string]interface{}
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			count, ok := resp["count"].(float64)
			if !ok {
				t.Fatalf("Expected count to be a number, got %T", resp["count"])
			}

			if int(count) != tt.expectedCount {
				t.Errorf("Expected count to be %d, got %d", tt.expectedCount, int(count))
			}
		})
	}
}

// ─── Export/Import endpoint tests ─────────────────────────────────────────────────

func TestHandlerExportConfig(t *testing.T) {
	reg := &mockRegistry{
		nodes: map[string]NodeRecord{
			"AA:BB:CC:DD:EE:FF": {
				MAC:             "AA:BB:CC:DD:EE:FF",
				Name:            "Node 1",
				Role:            "rx",
				FirmwareVersion: "1.0.0",
				ChipModel:       "ESP32-S3",
			},
			"11:22:33:44:55:66": {
				MAC:             "11:22:33:44:55:66",
				Name:            "Node 2",
				Role:            "tx",
				FirmwareVersion: "1.1.0",
				ChipModel:       "ESP32-S3",
			},
		},
	}

	mgr := &Manager{
		registry: reg,
	}

	h := &Handler{mgr: mgr}

	req := httptest.NewRequest("GET", "/api/export", nil)
	w := httptest.NewRecorder()

	h.exportConfig(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("exportConfig() status = %v, want %v", w.Code, http.StatusOK)
	}

	var config map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&config); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Check version
	if config["version"] != float64(1) {
		t.Errorf("Expected version to be 1, got %v", config["version"])
	}

	// Check exported_at exists
	if config["exported_at"] == nil {
		t.Error("Expected exported_at to be present")
	}

	// Check nodes
	nodes, ok := config["nodes"].([]interface{})
	if !ok {
		t.Fatalf("Expected nodes to be an array, got %T", config["nodes"])
	}

	if len(nodes) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(nodes))
	}
}

func TestHandlerImportConfig(t *testing.T) {
	tests := []struct {
		name        string
		requestBody string
		wantStatus  int
	}{
		{
			name:        "valid import config",
			requestBody: `{"version": 1, "nodes": []}`,
			wantStatus:  http.StatusOK,
		},
		{
			name:        "valid import with nodes",
			requestBody: `{"version": 1, "nodes": [{"mac": "AA:BB:CC:DD:EE:FF", "name": "Test"}]}`,
			wantStatus:  http.StatusOK,
		},
		{
			name:        "invalid json",
			requestBody: `invalid json`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "empty object",
			requestBody: `{}`,
			wantStatus:  http.StatusOK,
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

			req := httptest.NewRequest("POST", "/api/import", bytes.NewBufferString(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			h.importConfig(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("importConfig() status = %v, want %v", w.Code, tt.wantStatus)
			}
		})
	}
}

// ─── Rebaseline endpoint tests ────────────────────────────────────────────────────

func TestHandlerRebaselineAllNodes(t *testing.T) {
	reg := &mockRegistry{
		nodes: make(map[string]NodeRecord),
	}

	mgr := &Manager{
		registry: reg,
	}

	h := &Handler{mgr: mgr}

	req := httptest.NewRequest("POST", "/api/nodes/rebaseline-all", nil)
	w := httptest.NewRecorder()

	h.rebaselineAllNodes(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("rebaselineAllNodes() status = %v, want %v", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp["ok"] != true {
		t.Errorf("Expected ok to be true, got %v", resp["ok"])
	}
}

// ─── Add virtual node endpoint tests ───────────────────────────────────────────────

func TestHandlerAddVirtualNode(t *testing.T) {
	tests := []struct {
		name        string
		requestBody string
		wantStatus  int
	}{
		{
			name:        "successful virtual node creation",
			requestBody: `{"mac": "AA:BB:CC:DD:EE:FF", "name": "Virtual Node", "x": 1.0, "y": 2.0, "z": 3.0}`,
			wantStatus:  http.StatusCreated,
		},
		{
			name:        "missing MAC address",
			requestBody: `{"name": "Virtual Node", "x": 1.0, "y": 2.0, "z": 3.0}`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "invalid json",
			requestBody: `invalid json`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "empty body",
			requestBody: `{}`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "negative coordinates",
			requestBody: `{"mac": "AA:BB:CC:DD:EE:FF", "name": "Virtual Node", "x": -1.0, "y": -2.0, "z": -3.0}`,
			wantStatus:  http.StatusCreated,
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

			req := httptest.NewRequest("POST", "/api/nodes/virtual", bytes.NewBufferString(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			h.addVirtualNode(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("addVirtualNode() status = %v, want %v", w.Code, tt.wantStatus)
			}
		})
	}
}

// ─── Update node position endpoint tests ────────────────────────────────────────

func TestHandlerUpdateNodePosition(t *testing.T) {
	tests := []struct {
		name        string
		mac         string
		requestBody string
		wantStatus  int
		expectedX   float64
		expectedY   float64
		expectedZ   float64
	}{
		{
			name:        "successful position update",
			mac:         "AA:BB:CC:DD:EE:FF",
			requestBody: `{"x": 1.5, "y": 2.5, "z": 3.5}`,
			wantStatus:  http.StatusNoContent,
			expectedX:   1.5,
			expectedY:   2.5,
			expectedZ:   3.5,
		},
		{
			name:        "negative coordinates",
			mac:         "AA:BB:CC:DD:EE:FF",
			requestBody: `{"x": -1.0, "y": -2.0, "z": -3.0}`,
			wantStatus:  http.StatusNoContent,
			expectedX:   -1.0,
			expectedY:   -2.0,
			expectedZ:   -3.0,
		},
		{
			name:        "zero coordinates",
			mac:         "AA:BB:CC:DD:EE:FF",
			requestBody: `{"x": 0, "y": 0, "z": 0}`,
			wantStatus:  http.StatusNoContent,
			expectedX:   0,
			expectedY:   0,
			expectedZ:   0,
		},
		{
			name:        "invalid request body",
			mac:         "AA:BB:CC:DD:EE:FF",
			requestBody: `invalid json`,
			wantStatus:  http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &mockRegistry{
				nodes: map[string]NodeRecord{
					"AA:BB:CC:DD:EE:FF": {
						MAC:  "AA:BB:CC:DD:EE:FF",
						Name: "Test Node",
						Role: "rx",
						PosX: 0,
						PosY: 0,
						PosZ: 0,
					},
				},
			}

			mgr := &Manager{
				registry: reg,
			}

			h := &Handler{mgr: mgr}

			req := httptest.NewRequest("PUT", "/api/nodes/"+tt.mac+"/position", bytes.NewBufferString(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("mac", tt.mac)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			w := httptest.NewRecorder()
			h.updateNodePosition(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("updateNodePosition() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.wantStatus == http.StatusNoContent {
				// Verify the position was updated
				node := reg.nodes[tt.mac]
				if node.PosX != tt.expectedX || node.PosY != tt.expectedY || node.PosZ != tt.expectedZ {
					t.Errorf("Expected position to be (%v, %v, %v), got (%v, %v, %v)",
						tt.expectedX, tt.expectedY, tt.expectedZ,
						node.PosX, node.PosY, node.PosZ)
				}
			}
		})
	}
}
