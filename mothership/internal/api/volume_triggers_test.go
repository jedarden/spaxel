package api

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/spaxel/mothership/internal/volume"
)

// newTestRouter creates a chi.Router with the trigger routes registered.
func newTestRouter(h *VolumeTriggersHandler) *chi.Mux {
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return r
}

// newVolumeTestHandler creates a VolumeTriggersHandler backed by an in-memory database.
func newVolumeTestHandler(t *testing.T) (*VolumeTriggersHandler, func()) {
	t.Helper()
	h, err := NewVolumeTriggersHandler(":memory:")
	if err != nil {
		t.Fatalf("NewVolumeTriggersHandler: %v", err)
	}
	return h, func() { h.Close() }
}

// seedVolumeTrigger creates a trigger directly in the store for test setup.
func seedVolumeTrigger(t *testing.T, h *VolumeTriggersHandler, tr *volume.Trigger) string {
	t.Helper()
	id, err := h.store.Create(tr)
	if err != nil {
		t.Fatalf("seedVolumeTrigger: %v", err)
	}
	return id
}

// validBoxShape returns a valid box shape for test triggers.
func validBoxShape() volume.ShapeJSON {
	return volume.ShapeJSON{
		Type: volume.ShapeBox,
		X:    float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
		W:    float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
	}
}

// ── GET /api/triggers ─────────────────────────────────────────────────────────────

// TestVolumeListTriggers tests GET /api/triggers.
func TestVolumeListTriggers(t *testing.T) {
	tests := []struct {
		name    string
		seed    int // number of triggers to create before listing
		wantLen int
		wantErr bool
	}{
		{name: "empty store", seed: 0, wantLen: 0},
		{name: "single trigger", seed: 1, wantLen: 1},
		{name: "three triggers", seed: 3, wantLen: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, cleanup := newVolumeTestHandler(t)
			defer cleanup()

			for i := 0; i < tt.seed; i++ {
				seedVolumeTrigger(t, h, &volume.Trigger{
					Name:      "Trigger",
					Shape:     validBoxShape(),
					Condition: "enter",
					Enabled:   true,
				})
			}

			router := newTestRouter(h)
			req := httptest.NewRequest("GET", "/api/triggers", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}

			var result []TriggerResponse
			if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if len(result) != tt.wantLen {
				t.Errorf("expected %d triggers, got %d", tt.wantLen, len(result))
			}
		})
	}
}

// ── POST /api/triggers ────────────────────────────────────────────────────────────

// TestVolumeCreateTrigger tests POST /api/triggers.
func TestVolumeCreateTrigger(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCode int
		wantErr  string
	}{
		{
			name:     "valid trigger with all fields",
			body:     `{"name":"Couch Dwell","shape":{"type":"box","x":0,"y":0,"z":0,"w":1,"d":1,"h":1.5},"condition":"dwell","condition_params":{"duration_s":30},"time_constraint":{"from":"22:00","to":"06:00"},"actions":[{"type":"webhook","params":{"url":"http://example.com/hook"}}],"enabled":true}`,
			wantCode: http.StatusCreated,
		},
		{
			name:     "minimal valid trigger",
			body:     `{"name":"Enter Hall","shape":{"type":"box","x":1,"y":2,"z":0,"w":3,"d":4,"h":2},"condition":"enter"}`,
			wantCode: http.StatusCreated,
		},
		{
			name:     "cylinder shape",
			body:     `{"name":"Cyl","shape":{"type":"cylinder","cx":0,"cy":0,"z":0,"r":1,"h":2},"condition":"enter"}`,
			wantCode: http.StatusCreated,
		},
		{
			name:     "missing name",
			body:     `{"shape":{"type":"box","x":0,"y":0,"z":0,"w":1,"d":1,"h":1},"condition":"enter"}`,
			wantCode: http.StatusBadRequest,
			wantErr:  "name is required",
		},
		{
			name:     "invalid shape type",
			body:     `{"name":"Bad","shape":{"type":"sphere","x":0,"y":0,"z":0},"condition":"enter"}`,
			wantCode: http.StatusBadRequest,
			wantErr:  "invalid shape",
		},
		{
			name:     "invalid condition",
			body:     `{"name":"Bad","shape":{"type":"box","x":0,"y":0,"z":0,"w":1,"d":1,"h":1},"condition":"fly"}`,
			wantCode: http.StatusBadRequest,
			wantErr:  "condition must be one of",
		},
		{
			name:     "malformed JSON",
			body:     `{bad json}`,
			wantCode: http.StatusBadRequest,
			wantErr:  "invalid request body",
		},
		{
			name:     "empty body",
			body:     ``,
			wantCode: http.StatusBadRequest,
			wantErr:  "invalid request body",
		},
		{
			name:     "box with zero width",
			body:     `{"name":"ZeroW","shape":{"type":"box","x":0,"y":0,"z":0,"w":0,"d":1,"h":1},"condition":"enter"}`,
			wantCode: http.StatusBadRequest,
			wantErr:  "invalid shape",
		},
		{
			name:     "explicitly disabled",
			body:     `{"name":"Off","shape":{"type":"box","x":0,"y":0,"z":0,"w":1,"d":1,"h":1},"condition":"vacant","enabled":false}`,
			wantCode: http.StatusCreated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, cleanup := newVolumeTestHandler(t)
			defer cleanup()

			router := newTestRouter(h)
			req := httptest.NewRequest("POST", "/api/triggers", bytes.NewReader([]byte(tt.body)))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Fatalf("expected %d, got %d: %s", tt.wantCode, w.Code, w.Body.String())
			}

			if tt.wantErr != "" {
				if !bytes.Contains(w.Body.Bytes(), []byte(tt.wantErr)) {
					t.Errorf("expected error to contain %q, got %s", tt.wantErr, w.Body.String())
				}
				return
			}

			var created TriggerResponse
			if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if created.ID == "" {
				t.Error("expected non-empty ID")
			}
			if created.CreatedAt.IsZero() {
				t.Error("expected non-zero CreatedAt")
			}
		})
	}
}

// TestVolumeCreateTriggerAssignsID tests that created triggers get a unique auto-incremented ID.
func TestVolumeCreateTriggerAssignsID(t *testing.T) {
	h, cleanup := newVolumeTestHandler(t)
	defer cleanup()

	router := newTestRouter(h)
	body := `{"name":"First","shape":{"type":"box","x":0,"y":0,"z":0,"w":1,"d":1,"h":1},"condition":"enter"}`
	req := httptest.NewRequest("POST", "/api/triggers", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var first TriggerResponse
	json.NewDecoder(w.Body).Decode(&first)

	body2 := `{"name":"Second","shape":{"type":"box","x":0,"y":0,"z":0,"w":1,"d":1,"h":1},"condition":"dwell"}`
	req2 := httptest.NewRequest("POST", "/api/triggers", bytes.NewReader([]byte(body2)))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	var second TriggerResponse
	json.NewDecoder(w2.Body).Decode(&second)

	if first.ID == second.ID {
		t.Errorf("expected different IDs, both got %q", first.ID)
	}
}

// ── GET /api/triggers/{id} ────────────────────────────────────────────────────────

// TestVolumeGetTrigger tests GET /api/triggers/{id}.
func TestVolumeGetTrigger(t *testing.T) {
	tests := []struct {
		name     string
		setup    bool // whether to seed a trigger
		getID    string // empty = use the seeded ID
		wantCode int
		wantErr  string
	}{
		{name: "existing trigger", setup: true, wantCode: http.StatusOK},
		{name: "nonexistent trigger", setup: true, getID: "99999", wantCode: http.StatusNotFound, wantErr: "trigger not found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, cleanup := newVolumeTestHandler(t)
			defer cleanup()

			seededID := ""
			if tt.setup {
				seededID = seedVolumeTrigger(t, h, &volume.Trigger{
					Name:      "Get Me",
					Shape:     validBoxShape(),
					Condition: "enter",
					Enabled:   true,
				})
			}

			getID := seededID
			if tt.getID != "" {
				getID = tt.getID
			}

			router := newTestRouter(h)
			req := httptest.NewRequest("GET", "/api/triggers/"+getID, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Fatalf("expected %d, got %d: %s", tt.wantCode, w.Code, w.Body.String())
			}

			if tt.wantErr != "" {
				if !bytes.Contains(w.Body.Bytes(), []byte(tt.wantErr)) {
					t.Errorf("expected error to contain %q, got %s", tt.wantErr, w.Body.String())
				}
				return
			}

			var result TriggerResponse
			if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
				t.Fatalf("failed to decode: %v", err)
			}
			if result.Name != "Get Me" {
				t.Errorf("expected name 'Get Me', got %s", result.Name)
			}
			if result.Condition != "enter" {
				t.Errorf("expected condition 'enter', got %s", result.Condition)
			}
		})
	}
}

// ── PUT /api/triggers/{id} ────────────────────────────────────────────────────────

// TestVolumeUpdateTrigger tests PUT /api/triggers/{id}.
func TestVolumeUpdateTrigger(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantCode   int
		wantName   string
		wantEnable bool
		wantErr    string
	}{
		{
			name:       "update name",
			body:       `{"name":"New Name"}`,
			wantCode:   http.StatusOK,
			wantName:   "New Name",
			wantEnable: true,
		},
		{
			name:       "disable trigger",
			body:       `{"enabled":false}`,
			wantCode:   http.StatusOK,
			wantName:   "Original",
			wantEnable: false,
		},
		{
			name:       "change condition",
			body:       `{"condition":"dwell"}`,
			wantCode:   http.StatusOK,
			wantName:   "Original",
			wantEnable: true,
		},
		{
			name:       "update shape",
			body:       `{"shape":{"type":"cylinder","cx":1,"cy":1,"z":0,"r":2,"h":3}}`,
			wantCode:   http.StatusOK,
			wantName:   "Original",
			wantEnable: true,
		},
		{
			name:       "update multiple fields",
			body:       `{"name":"Multi","condition":"count","enabled":false}`,
			wantCode:   http.StatusOK,
			wantName:   "Multi",
			wantEnable: false,
		},
		{
			name:       "no-op update returns current",
			body:       `{}`,
			wantCode:   http.StatusOK,
			wantName:   "Original",
			wantEnable: true,
		},
		{
			name:     "nonexistent trigger",
			body:     `{"name":"Nope"}`,
			wantCode: http.StatusNotFound,
			wantErr:  "trigger not found",
		},
		{
			name:     "malformed JSON",
			body:     `{bad}`,
			wantCode: http.StatusBadRequest,
			wantErr:  "invalid request body",
		},
		{
			name:     "invalid shape",
			body:     `{"shape":{"type":"box","x":0}}`,
			wantCode: http.StatusBadRequest,
			wantErr:  "invalid shape",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, cleanup := newVolumeTestHandler(t)
			defer cleanup()

			seededID := seedVolumeTrigger(t, h, &volume.Trigger{
				Name:      "Original",
				Shape:     validBoxShape(),
				Condition: "enter",
				Enabled:   true,
			})

			getID := seededID
			if tt.name == "nonexistent trigger" {
				getID = "99999"
			}

			router := newTestRouter(h)
			req := httptest.NewRequest("PUT", "/api/triggers/"+getID, bytes.NewReader([]byte(tt.body)))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Fatalf("expected %d, got %d: %s", tt.wantCode, w.Code, w.Body.String())
			}

			if tt.wantErr != "" {
				if !bytes.Contains(w.Body.Bytes(), []byte(tt.wantErr)) {
					t.Errorf("expected error to contain %q, got %s", tt.wantErr, w.Body.String())
				}
				return
			}

			var updated TriggerResponse
			if err := json.NewDecoder(w.Body).Decode(&updated); err != nil {
				t.Fatalf("failed to decode: %v", err)
			}
			if updated.Name != tt.wantName {
				t.Errorf("expected name %q, got %q", tt.wantName, updated.Name)
			}
			if updated.Enabled != tt.wantEnable {
				t.Errorf("expected enabled=%v, got %v", tt.wantEnable, updated.Enabled)
			}
		})
	}
}

// ── DELETE /api/triggers/{id} ─────────────────────────────────────────────────────

// TestVolumeDeleteTrigger tests DELETE /api/triggers/{id}.
func TestVolumeDeleteTrigger(t *testing.T) {
	tests := []struct {
		name     string
		setup    int  // number of triggers to seed
		deleteN  int  // 0 = delete nonexistent, 1 = delete first trigger
		wantCode int
		wantLen  int
	}{
		{
			name:     "delete existing trigger",
			setup:    2,
			deleteN:  1,
			wantCode: http.StatusNoContent,
			wantLen:  1,
		},
		{
			name:     "delete only trigger",
			setup:    1,
			deleteN:  1,
			wantCode: http.StatusNoContent,
			wantLen:  0,
		},
		{
			name:     "delete nonexistent trigger",
			setup:    1,
			deleteN:  0,
			wantCode: http.StatusNoContent,
			wantLen:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, cleanup := newVolumeTestHandler(t)
			defer cleanup()

			var ids []string
			for i := 0; i < tt.setup; i++ {
				id := seedVolumeTrigger(t, h, &volume.Trigger{
					Name:      "Trigger",
					Shape:     validBoxShape(),
					Condition: "enter",
					Enabled:   true,
				})
				ids = append(ids, id)
			}

			deleteID := "99999"
			if tt.deleteN > 0 && tt.deleteN <= len(ids) {
				deleteID = ids[tt.deleteN-1]
			}

			router := newTestRouter(h)
			req := httptest.NewRequest("DELETE", "/api/triggers/"+deleteID, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Fatalf("expected %d, got %d: %s", tt.wantCode, w.Code, w.Body.String())
			}

			// Verify remaining count via list
			req2 := httptest.NewRequest("GET", "/api/triggers", nil)
			w2 := httptest.NewRecorder()
			router.ServeHTTP(w2, req2)
			var result []TriggerResponse
			json.NewDecoder(w2.Body).Decode(&result)
			if len(result) != tt.wantLen {
				t.Errorf("expected %d triggers remaining, got %d", tt.wantLen, len(result))
			}
		})
	}
}

// ── CRUD round-trip ───────────────────────────────────────────────────────────────

// TestVolumeTriggerCRUDRoundTrip verifies the full lifecycle:
// create -> list -> get -> update -> get -> delete -> verify gone.
func TestVolumeTriggerCRUDRoundTrip(t *testing.T) {
	h, cleanup := newVolumeTestHandler(t)
	defer cleanup()

	router := newTestRouter(h)

	// 1. Create
	createBody := `{"name":"Round Trip","shape":{"type":"box","x":1,"y":2,"z":0,"w":3,"d":4,"h":2},"condition":"dwell","condition_params":{"duration_s":60}}`
	req := httptest.NewRequest("POST", "/api/triggers", bytes.NewReader([]byte(createBody)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var created TriggerResponse
	json.NewDecoder(w.Body).Decode(&created)
	createdID := created.ID

	// 2. List and verify
	req2 := httptest.NewRequest("GET", "/api/triggers", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	var triggers []TriggerResponse
	json.NewDecoder(w2.Body).Decode(&triggers)
	if len(triggers) != 1 {
		t.Fatalf("after create: expected 1 trigger, got %d", len(triggers))
	}
	if triggers[0].Name != "Round Trip" {
		t.Errorf("after create: expected name 'Round Trip', got %s", triggers[0].Name)
	}

	// 3. Get single
	req3 := httptest.NewRequest("GET", "/api/triggers/"+createdID, nil)
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", w3.Code)
	}
	var fetched TriggerResponse
	json.NewDecoder(w3.Body).Decode(&fetched)
	if fetched.Condition != "dwell" {
		t.Errorf("get: expected condition 'dwell', got %s", fetched.Condition)
	}

	// 4. Update
	updateBody := `{"name":"Updated Trip","enabled":false}`
	req4 := httptest.NewRequest("PUT", "/api/triggers/"+createdID, bytes.NewReader([]byte(updateBody)))
	req4.Header.Set("Content-Type", "application/json")
	w4 := httptest.NewRecorder()
	router.ServeHTTP(w4, req4)
	if w4.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d", w4.Code)
	}

	// 5. Verify update via get
	req5 := httptest.NewRequest("GET", "/api/triggers/"+createdID, nil)
	w5 := httptest.NewRecorder()
	router.ServeHTTP(w5, req5)
	var afterUpdate TriggerResponse
	json.NewDecoder(w5.Body).Decode(&afterUpdate)
	if afterUpdate.Name != "Updated Trip" {
		t.Errorf("after update: expected name 'Updated Trip', got %s", afterUpdate.Name)
	}
	if afterUpdate.Enabled {
		t.Error("after update: expected enabled=false")
	}

	// 6. Delete
	req6 := httptest.NewRequest("DELETE", "/api/triggers/"+createdID, nil)
	w6 := httptest.NewRecorder()
	router.ServeHTTP(w6, req6)
	if w6.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", w6.Code)
	}

	// 7. Verify gone
	req7 := httptest.NewRequest("GET", "/api/triggers/"+createdID, nil)
	w7 := httptest.NewRecorder()
	router.ServeHTTP(w7, req7)
	if w7.Code != http.StatusNotFound {
		t.Errorf("after delete: expected 404, got %d", w7.Code)
	}

	// 8. Verify list is empty
	req8 := httptest.NewRequest("GET", "/api/triggers", nil)
	w8 := httptest.NewRecorder()
	router.ServeHTTP(w8, req8)
	json.NewDecoder(w8.Body).Decode(&triggers)
	if len(triggers) != 0 {
		t.Errorf("after delete: expected 0 triggers, got %d", len(triggers))
	}
}

// ── POST /api/triggers/{id}/test (existing tests below) ───────────────────────────

// TestTestTriggerEndpoint tests POST /api/triggers/{id}/test.
func TestTestTriggerEndpoint(t *testing.T) {
	handler, err := NewVolumeTriggersHandler(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer handler.Close()

	// Create a trigger with a webhook action
	trigger := &volume.Trigger{
		Name: "test trigger",
		Shape: volume.ShapeJSON{
			Type: volume.ShapeBox,
			X:    float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W:    float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "dwell",
		Enabled:  true,
		Actions: []volume.Action{
			{Type: "webhook", Params: map[string]interface{}{"url": "http://example.com/hook"}},
		},
	}

	id, err := handler.store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	// Test with a mock server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer mockServer.Close()

	// Replace the action URL with the mock server URL
	tg, _ := handler.store.Get(id)
	tg.Actions[0].Params["url"] = mockServer.URL
	handler.store.Update(tg)

	// Call test endpoint via chi router
	router := newTestRouter(handler)
	req := httptest.NewRequest("POST", "/api/triggers/"+id+"/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var result WebhookTestResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if result.Status != "ok" {
		t.Errorf("Expected status 'ok', got %s", result.Status)
	}

	if len(result.Actions) != 1 {
		t.Fatalf("Expected 1 action result, got %d", len(result.Actions))
	}

	if result.Actions[0].Status != 200 {
		t.Errorf("Expected action status 200, got %d", result.Actions[0].Status)
	}

	if result.Actions[0].ResponseMs < 0 {
		t.Errorf("Expected non-negative response_ms, got %d", result.Actions[0].ResponseMs)
	}
}

// TestTestTrigger_ReturnsErrorOnMissingURL tests that missing URL produces an error result.
func TestTestTrigger_ReturnsErrorOnMissingURL(t *testing.T) {
	handler, err := NewVolumeTriggersHandler(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer handler.Close()

	trigger := &volume.Trigger{
		Name: "no url trigger",
		Shape: volume.ShapeJSON{
			Type: volume.ShapeBox,
			X:    float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W:    float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "enter",
		Enabled:  true,
		Actions: []volume.Action{
			{Type: "webhook", Params: map[string]interface{}{}},
		},
	}

	id, err := handler.store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	router := newTestRouter(handler)
	req := httptest.NewRequest("POST", "/api/triggers/"+id+"/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var result WebhookTestResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if len(result.Actions) != 1 {
		t.Fatalf("Expected 1 action result, got %d", len(result.Actions))
	}

	if result.Actions[0].Error != "missing url" {
		t.Errorf("Expected error 'missing url', got %q", result.Actions[0].Error)
	}
}

// TestTestTrigger_4xxInTestDoesNotDisable tests that test endpoint doesn't disable trigger on 4xx.
func TestTestTrigger_4xxInTestDoesNotDisable(t *testing.T) {
	handler, err := NewVolumeTriggersHandler(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer handler.Close()

	// Mock server that always returns 404
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	trigger := &volume.Trigger{
		Name: "4xx test trigger",
		Shape: volume.ShapeJSON{
			Type: volume.ShapeBox,
			X:    float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W:    float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "enter",
		Enabled:  true,
		Actions: []volume.Action{
			{Type: "webhook", Params: map[string]interface{}{"url": mockServer.URL}},
		},
	}

	id, err := handler.store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	// Call test endpoint — 4xx from mock
	router := newTestRouter(handler)
	req := httptest.NewRequest("POST", "/api/triggers/"+id+"/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Trigger should still be enabled (test mode doesn't disable)
	tg, _ := handler.store.Get(id)
	if !tg.Enabled {
		t.Error("Expected trigger to remain enabled after test endpoint 4xx")
	}
}

// TestEnableEndpoint tests POST /api/triggers/{id}/enable.
func TestEnableEndpoint(t *testing.T) {
	handler, err := NewVolumeTriggersHandler(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer handler.Close()

	trigger := &volume.Trigger{
		Name: "test enable",
		Shape: volume.ShapeJSON{
			Type: volume.ShapeBox,
			X:    float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W:    float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "enter",
		Enabled:  false,
	}

	id, err := handler.store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	// Disable with error
	handler.store.DisableTriggerWithError(id, "HTTP 403")
	handler.store.IncrementErrorCount(id)

	router := newTestRouter(handler)
	req := httptest.NewRequest("POST", "/api/triggers/"+id+"/enable", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	tg, _ := handler.store.Get(id)
	if !tg.Enabled {
		t.Error("Expected trigger to be enabled after enable endpoint call")
	}
	if tg.ErrorMessage != "" {
		t.Errorf("Expected empty error_message, got %q", tg.ErrorMessage)
	}
	if tg.ErrorCount != 0 {
		t.Errorf("Expected error_count 0, got %d", tg.ErrorCount)
	}
}

// TestGetWebhookLogEndpoint tests GET /api/triggers/{id}/webhook-log.
func TestGetWebhookLogEndpoint(t *testing.T) {
	handler, err := NewVolumeTriggersHandler(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer handler.Close()

	trigger := &volume.Trigger{
		Name: "log test",
		Shape: volume.ShapeJSON{
			Type: volume.ShapeBox,
			X:    float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W:    float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "enter",
		Enabled: true,
	}

	id, err := handler.store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UnixMilli()
	handler.store.WriteWebhookLog(id, "http://a.com", now, 200, 50, "")
	handler.store.WriteWebhookLog(id, "http://b.com", now-1000, 500, 0, "timeout")

	router := newTestRouter(handler)
	req := httptest.NewRequest("GET", "/api/triggers/"+id+"/webhook-log?limit=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var entries []volume.WebhookLogEntry
	if err := json.NewDecoder(w.Body).Decode(&entries); err != nil {
		t.Fatal(err)
	}

	if len(entries) != 2 {
		t.Fatalf("Expected 2 log entries, got %d", len(entries))
	}

	// Most recent first
	if entries[0].URL != "http://a.com" {
		t.Errorf("Expected first entry URL 'http://a.com', got %s", entries[0].URL)
	}
	if entries[0].Status != 200 {
		t.Errorf("Expected first entry status 200, got %d", entries[0].Status)
	}
}

// TestWebhookPayloadSchema tests that the webhook payload contains all required fields.
func TestWebhookPayloadSchema(t *testing.T) {
	// Create a mock server to capture the payload
	var receivedPayload map[string]interface{}
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	handler, err := NewVolumeTriggersHandler(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer handler.Close()

	trigger := &volume.Trigger{
		Name: "payload test",
		Shape: volume.ShapeJSON{
			Type: volume.ShapeBox,
			X:    float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W:    float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "dwell",
		Enabled:  true,
		Actions: []volume.Action{
			{Type: "webhook", Params: map[string]interface{}{"url": mockServer.URL}},
		},
	}

	id, err := handler.store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	// Fire the trigger
	now := time.Now()
	handler.onTriggerFired(volume.FiredEvent{
		TriggerID:   id,
		TriggerName: "payload test",
		Condition:   "dwell",
		BlobIDs:     []int{1, 2},
		Timestamp:   now,
	})

	// Give the async callback time to complete
	time.Sleep(100 * time.Millisecond)

	requiredFields := []string{"trigger_id", "trigger_name", "condition", "blob_id", "person", "position", "zone", "dwell_s", "timestamp_ms"}
	for _, field := range requiredFields {
		if _, ok := receivedPayload[field]; !ok {
			t.Errorf("Missing required field %q in webhook payload", field)
		}
	}

	if receivedPayload["trigger_name"] != "payload test" {
		t.Errorf("Expected trigger_name 'payload test', got %v", receivedPayload["trigger_name"])
	}
	if receivedPayload["condition"] != "dwell" {
		t.Errorf("Expected condition 'dwell', got %v", receivedPayload["condition"])
	}
}

// Test5xxDoesNotDisableTrigger tests that 5xx responses only increment error count.
func Test5xxDoesNotDisableTrigger(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer mockServer.Close()

	handler, err := NewVolumeTriggersHandler(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer handler.Close()

	trigger := &volume.Trigger{
		Name: "5xx test",
		Shape: volume.ShapeJSON{
			Type: volume.ShapeBox,
			X:    float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W:    float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "enter",
		Enabled:  true,
		Actions: []volume.Action{
			{Type: "webhook", Params: map[string]interface{}{"url": mockServer.URL}},
		},
	}

	id, err := handler.store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	// Fire 5 times — all should be 5xx
	for i := 0; i < 5; i++ {
		handler.onTriggerFired(volume.FiredEvent{
			TriggerID:   id,
			TriggerName: "5xx test",
			Condition:   "enter",
			BlobIDs:     []int{1},
			Timestamp:   time.Now(),
		})
		time.Sleep(100 * time.Millisecond)
	}

	tg, _ := handler.store.Get(id)
	if !tg.Enabled {
		t.Error("Expected trigger to remain enabled after 5xx errors")
	}
	if tg.ErrorCount != 5 {
		t.Errorf("Expected error_count 5, got %d", tg.ErrorCount)
	}
}

// Test4xxDisablesTrigger tests that 4xx responses disable the trigger.
func Test4xxDisablesTrigger(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer mockServer.Close()

	handler, err := NewVolumeTriggersHandler(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer handler.Close()

	trigger := &volume.Trigger{
		Name: "4xx test",
		Shape: volume.ShapeJSON{
			Type: volume.ShapeBox,
			X:    float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W:    float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "enter",
		Enabled:  true,
		Actions: []volume.Action{
			{Type: "webhook", Params: map[string]interface{}{"url": mockServer.URL}},
		},
	}

	id, err := handler.store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	// Fire — should get 403 and disable
	handler.onTriggerFired(volume.FiredEvent{
		TriggerID:   id,
		TriggerName: "4xx test",
		Condition:   "enter",
		BlobIDs:     []int{1},
		Timestamp:   time.Now(),
	})

	time.Sleep(100 * time.Millisecond)

	tg, _ := handler.store.Get(id)
	if tg.Enabled {
		t.Error("Expected trigger to be disabled after 4xx response")
	}
	if tg.ErrorMessage == "" {
		t.Error("Expected non-empty error_message after 4xx response")
	}
}

// Test2xxResetsErrorCount tests that a 2xx response resets error_count.
func Test2xxResetsErrorCount(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return 500 first, then 200
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	handler, err := NewVolumeTriggersHandler(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer handler.Close()

	trigger := &volume.Trigger{
		Name: "2xx reset test",
		Shape: volume.ShapeJSON{
			Type: volume.ShapeBox,
			X:    float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W:    float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "enter",
		Enabled:  true,
		Actions: []volume.Action{
			{Type: "webhook", Params: map[string]interface{}{"url": mockServer.URL}},
		},
	}

	id, err := handler.store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	// Accumulate some errors first
	handler.store.IncrementErrorCount(id)
	handler.store.IncrementErrorCount(id)
	handler.store.IncrementErrorCount(id)

	// Fire — should get 200 and reset
	handler.onTriggerFired(volume.FiredEvent{
		TriggerID:   id,
		TriggerName: "2xx reset test",
		Condition:   "enter",
		BlobIDs:     []int{1},
		Timestamp:   time.Now(),
	})

	time.Sleep(100 * time.Millisecond)

	tg, _ := handler.store.Get(id)
	if tg.ErrorCount != 0 {
		t.Errorf("Expected error_count 0 after 2xx, got %d", tg.ErrorCount)
	}
	if !tg.Enabled {
		t.Error("Expected trigger to remain enabled after 2xx")
	}
}

// TestTimeoutDoesNotDisable tests that request timeouts don't disable the trigger.
func TestTimeoutDoesNotDisable(t *testing.T) {
	// Use a raw listener that accepts but never responds, then closes cleanly.
	// httptest.Server.Close() blocks waiting for active connections, which
	// causes the test to hang. A raw listener avoids this.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	handler, err := NewVolumeTriggersHandler(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer handler.Close()

	// Use a very short timeout for testing
	handler.httpClient.Timeout = 100 * time.Millisecond

	trigger := &volume.Trigger{
		Name: "timeout test",
		Shape: volume.ShapeJSON{
			Type: volume.ShapeBox,
			X:    float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W:    float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "enter",
		Enabled:  true,
		Actions: []volume.Action{
			{Type: "webhook", Params: map[string]interface{}{"url": "http://" + listener.Addr().String()}},
		},
	}

	id, err := handler.store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	handler.onTriggerFired(volume.FiredEvent{
		TriggerID:   id,
		TriggerName: "timeout test",
		Condition:   "enter",
		BlobIDs:     []int{1},
		Timestamp:   time.Now(),
	})

	// Wait for the timeout to complete
	time.Sleep(500 * time.Millisecond)

	tg, _ := handler.store.Get(id)
	if !tg.Enabled {
		t.Error("Expected trigger to remain enabled after timeout")
	}
	if tg.ErrorCount != 1 {
		t.Errorf("Expected error_count 1 after timeout, got %d", tg.ErrorCount)
	}
}

// Helper
func float64Ptr(f float64) *float64 {
	return &f
}
