package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi"
)

// newTriggerTestHandler creates a TriggersHandler backed by an in-memory database.
func newTriggerTestHandler(t *testing.T) (*TriggersHandler, func()) {
	t.Helper()
	h, err := NewTriggersHandler(":memory:")
	if err != nil {
		t.Fatalf("NewTriggersHandler: %v", err)
	}
	return h, func() { h.Close() }
}

// newTriggerTestRouter creates a chi.Router with trigger routes registered.
func newTriggerTestRouter(h *TriggersHandler) *chi.Mux {
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return r
}

// seedTrigger creates a trigger directly in the handler for test setup.
func seedTrigger(t *testing.T, h *TriggersHandler, tr Trigger) {
	t.Helper()
	now := time.Now().UnixNano()
	enabled := 0
	if tr.Enabled {
		enabled = 1
	}
	actions := tr.Actions
	if len(actions) == 0 {
		actions = json.RawMessage("[]")
	}
	conditionParams := tr.ConditionParams
	if len(conditionParams) == 0 {
		conditionParams = json.RawMessage("{}")
	}
	timeConstraint := tr.TimeConstraint
	if len(timeConstraint) == 0 {
		timeConstraint = json.RawMessage("{}")
	}
	_, err := h.db.Exec(`
		INSERT INTO triggers (id, name, enabled, condition, condition_params, time_constraint, actions, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, tr.ID, tr.Name, enabled, tr.Condition, string(conditionParams), string(timeConstraint), string(actions), now)
	if err != nil {
		t.Fatalf("seedTrigger: %v", err)
	}
	h.mu.Lock()
	h.triggers[tr.ID] = &Trigger{
		ID:              tr.ID,
		Name:            tr.Name,
		Enabled:         tr.Enabled,
		Condition:       tr.Condition,
		ConditionParams: conditionParams,
		TimeConstraint:  timeConstraint,
		Actions:         actions,
		CreatedAt:       time.Unix(0, now),
	}
	h.mu.Unlock()
}

// ── GET /api/triggers ─────────────────────────────────────────────────────────────

// TestListTriggers tests GET /api/triggers.
func TestListTriggers(t *testing.T) {
	tests := []struct {
		name     string
		setup    []Trigger
		wantLen  int
		wantCode int
	}{
		{
			name:     "empty store",
			setup:    nil,
			wantLen:  0,
			wantCode: http.StatusOK,
		},
		{
			name: "single trigger",
			setup: []Trigger{
				{ID: "t1", Name: "Couch Dwell", Condition: "dwell", Enabled: true},
			},
			wantLen:  1,
			wantCode: http.StatusOK,
		},
		{
			name: "multiple triggers",
			setup: []Trigger{
				{ID: "t1", Name: "Enter Hallway", Condition: "enter", Enabled: true},
				{ID: "t2", Name: "Leave Home", Condition: "vacant", Enabled: false},
				{ID: "t3", Name: "Count Kitchen", Condition: "count", Enabled: true},
			},
			wantLen:  3,
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, cleanup := newTriggerTestHandler(t)
			defer cleanup()

			for _, tr := range tt.setup {
				seedTrigger(t, h, tr)
			}

			r := newTriggerTestRouter(h)
			req := httptest.NewRequest("GET", "/api/triggers", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Fatalf("expected %d, got %d: %s", tt.wantCode, w.Code, w.Body.String())
			}

			var result []Trigger
			if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
				t.Fatalf("failed to decode: %v", err)
			}
			if len(result) != tt.wantLen {
				t.Errorf("expected %d triggers, got %d", tt.wantLen, len(result))
			}
		})
	}
}

// ── POST /api/triggers ────────────────────────────────────────────────────────────

// TestCreateTrigger tests POST /api/triggers.
func TestCreateTrigger(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCode int
		wantID   string
		wantErr  string
	}{
		{
			name: "valid trigger with all fields",
			body: `{
				"id": "t1",
				"name": "Couch Dwell",
				"condition": "dwell",
				"condition_params": {"duration_s": 30},
				"time_constraint": {"from": "22:00", "to": "06:00"},
				"actions": [{"type": "webhook", "url": "http://example.com/hook"}],
				"enabled": true
			}`,
			wantCode: http.StatusCreated,
			wantID:   "t1",
		},
		{
			name: "minimal valid trigger",
			body: `{"id": "t2", "name": "Enter", "condition": "enter"}`,
			wantCode: http.StatusCreated,
			wantID:   "t2",
		},
		{
			name:     "missing id",
			body:     `{"name": "No ID", "condition": "enter"}`,
			wantCode: http.StatusBadRequest,
			wantErr:  "id is required",
		},
		{
			name:     "missing name",
			body:     `{"id": "t3", "condition": "enter"}`,
			wantCode: http.StatusBadRequest,
			wantErr:  "name is required",
		},
		{
			name:     "invalid condition",
			body:     `{"id": "t4", "name": "Bad", "condition": "fly"}`,
			wantCode: http.StatusBadRequest,
			wantErr:  "condition must be one of",
		},
		{
			name:     "missing condition",
			body:     `{"id": "t5", "name": "NoCond"}`,
			wantCode: http.StatusBadRequest,
			wantErr:  "condition must be one of",
		},
		{
			name:     "malformed JSON",
			body:     `{invalid}`,
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
			name:     "disabled by default when not specified",
			body:     `{"id": "t6", "name": "Default", "condition": "leave"}`,
			wantCode: http.StatusCreated,
			wantID:   "t6",
		},
		{
			name: "explicitly disabled",
			body: `{"id": "t7", "name": "Off", "condition": "vacant", "enabled": false}`,
			wantCode: http.StatusCreated,
			wantID:   "t7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, cleanup := newTriggerTestHandler(t)
			defer cleanup()

			r := newTriggerTestRouter(h)
			req := httptest.NewRequest("POST", "/api/triggers", bytes.NewReader([]byte(tt.body)))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Fatalf("expected %d, got %d: %s", tt.wantCode, w.Code, w.Body.String())
			}

			if tt.wantErr != "" {
				if !bytes.Contains(w.Body.Bytes(), []byte(tt.wantErr)) {
					t.Errorf("expected error to contain %q, got %s", tt.wantErr, w.Body.String())
				}
				return
			}

			var created Trigger
			if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
				t.Fatalf("failed to decode: %v", err)
			}
			if created.ID != tt.wantID {
				t.Errorf("expected ID %q, got %q", tt.wantID, created.ID)
			}
			if created.CreatedAt.IsZero() {
				t.Error("expected non-zero CreatedAt")
			}
		})
	}
}

// TestCreateTriggerDuplicate tests that creating a trigger with a duplicate ID fails.
func TestCreateTriggerDuplicate(t *testing.T) {
	h, cleanup := newTriggerTestHandler(t)
	defer cleanup()

	seedTrigger(t, h, Trigger{ID: "t1", Name: "First", Condition: "enter", Enabled: true})

	r := newTriggerTestRouter(h)
	body := `{"id": "t1", "name": "Duplicate", "condition": "dwell"}`
	req := httptest.NewRequest("POST", "/api/triggers", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// SQLite PRIMARY KEY constraint should reject the duplicate
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for duplicate ID, got %d", w.Code)
	}
}

// TestCreateTriggerPersists tests that a created trigger survives a handler reload.
func TestCreateTriggerPersists(t *testing.T) {
	h, cleanup := newTriggerTestHandler(t)
	defer cleanup()

	r := newTriggerTestRouter(h)
	body := `{"id": "persist", "name": "Persistent", "condition": "leave"}`
	req := httptest.NewRequest("POST", "/api/triggers", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", w.Code)
	}

	// Reload from DB
	if err := h.load(); err != nil {
		t.Fatalf("reload: %v", err)
	}

	// Verify it's still there
	req2 := httptest.NewRequest("GET", "/api/triggers", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	var result []Trigger
	json.NewDecoder(w2.Body).Decode(&result)
	if len(result) != 1 {
		t.Fatalf("after reload: expected 1 trigger, got %d", len(result))
	}
	if result[0].Name != "Persistent" {
		t.Errorf("after reload: expected name 'Persistent', got %s", result[0].Name)
	}
}

// ── PUT /api/triggers/{id} ────────────────────────────────────────────────────────

// TestUpdateTrigger tests PUT /api/triggers/{id}.
func TestUpdateTrigger(t *testing.T) {
	tests := []struct {
		name       string
		setup      Trigger
		body       string
		wantCode   int
		wantName   string
		wantEnable bool
	}{
		{
			name:       "update name",
			setup:      Trigger{ID: "t1", Name: "Old", Condition: "enter", Enabled: true},
			body:       `{"name": "New Name"}`,
			wantCode:   http.StatusOK,
			wantName:   "New Name",
			wantEnable: true,
		},
		{
			name:       "disable trigger",
			setup:      Trigger{ID: "t1", Name: "On", Condition: "dwell", Enabled: true},
			body:       `{"enabled": false}`,
			wantCode:   http.StatusOK,
			wantName:   "On",
			wantEnable: false,
		},
		{
			name:       "enable trigger",
			setup:      Trigger{ID: "t1", Name: "Off", Condition: "vacant", Enabled: false},
			body:       `{"enabled": true}`,
			wantCode:   http.StatusOK,
			wantName:   "Off",
			wantEnable: true,
		},
		{
			name:       "change condition",
			setup:      Trigger{ID: "t1", Name: "Flex", Condition: "enter", Enabled: true},
			body:       `{"condition": "dwell"}`,
			wantCode:   http.StatusOK,
			wantName:   "Flex",
			wantEnable: true,
		},
		{
			name:       "update multiple fields",
			setup:      Trigger{ID: "t1", Name: "Old", Condition: "enter", Enabled: true},
			body:       `{"name": "Multi", "condition": "count", "enabled": false}`,
			wantCode:   http.StatusOK,
			wantName:   "Multi",
			wantEnable: false,
		},
		{
			name:     "no-op update returns current",
			setup:    Trigger{ID: "t1", Name: "Same", Condition: "leave", Enabled: true},
			body:     `{}`,
			wantCode: http.StatusOK,
			wantName: "Same",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, cleanup := newTriggerTestHandler(t)
			defer cleanup()

			seedTrigger(t, h, tt.setup)

			r := newTriggerTestRouter(h)
			req := httptest.NewRequest("PUT", "/api/triggers/"+tt.setup.ID, bytes.NewReader([]byte(tt.body)))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Fatalf("expected %d, got %d: %s", tt.wantCode, w.Code, w.Body.String())
			}

			var updated Trigger
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

// TestUpdateTriggerInvalid tests PUT /api/triggers/{id} with invalid input.
func TestUpdateTriggerInvalid(t *testing.T) {
	tests := []struct {
		name    string
		setup   Trigger
		body    string
		want    int
		wantErr string
	}{
		{
			name:    "nonexistent trigger",
			setup:   Trigger{ID: "t1", Name: "Exists", Condition: "enter", Enabled: true},
			body:    `{"name": "Nope"}`,
			want:    http.StatusNotFound,
			wantErr: "trigger not found",
		},
		{
			name:    "malformed JSON",
			setup:   Trigger{ID: "t1", Name: "Exists", Condition: "enter", Enabled: true},
			body:    `{bad}`,
			want:    http.StatusBadRequest,
			wantErr: "invalid request body",
		},
		{
			name:    "invalid condition",
			setup:   Trigger{ID: "t1", Name: "Exists", Condition: "enter", Enabled: true},
			body:    `{"condition": "invalid"}`,
			want:    http.StatusBadRequest,
			wantErr: "condition must be one of",
		},
		{
			name:    "empty body",
			setup:   Trigger{ID: "t1", Name: "Exists", Condition: "enter", Enabled: true},
			body:    ``,
			want:    http.StatusBadRequest,
			wantErr: "invalid request body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, cleanup := newTriggerTestHandler(t)
			defer cleanup()

			seedTrigger(t, h, tt.setup)

			id := tt.setup.ID
			if tt.name == "nonexistent trigger" {
				id = "nonexistent"
			}

			r := newTriggerTestRouter(h)
			req := httptest.NewRequest("PUT", "/api/triggers/"+id, bytes.NewReader([]byte(tt.body)))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.want {
				t.Fatalf("expected %d, got %d: %s", tt.want, w.Code, w.Body.String())
			}
			if !bytes.Contains(w.Body.Bytes(), []byte(tt.wantErr)) {
				t.Errorf("expected error to contain %q, got %s", tt.wantErr, w.Body.String())
			}
		})
	}
}

// TestUpdateTriggerPersists tests that an update is persisted across reload.
func TestUpdateTriggerPersists(t *testing.T) {
	h, cleanup := newTriggerTestHandler(t)
	defer cleanup()

	seedTrigger(t, h, Trigger{ID: "t1", Name: "Original", Condition: "enter", Enabled: true})

	r := newTriggerTestRouter(h)
	body := `{"name": "Updated Name"}`
	req := httptest.NewRequest("PUT", "/api/triggers/t1", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d", w.Code)
	}

	// Reload from DB
	if err := h.load(); err != nil {
		t.Fatalf("reload: %v", err)
	}

	req2 := httptest.NewRequest("GET", "/api/triggers", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	var result []Trigger
	json.NewDecoder(w2.Body).Decode(&result)
	if len(result) != 1 {
		t.Fatalf("after reload: expected 1 trigger, got %d", len(result))
	}
	if result[0].Name != "Updated Name" {
		t.Errorf("after reload: expected name 'Updated Name', got %s", result[0].Name)
	}
}

// ── DELETE /api/triggers/{id} ─────────────────────────────────────────────────────

// TestDeleteTrigger tests DELETE /api/triggers/{id}.
func TestDeleteTrigger(t *testing.T) {
	tests := []struct {
		name     string
		setup    []Trigger
		deleteID string
		wantCode int
		wantLen  int
	}{
		{
			name: "delete existing trigger",
			setup: []Trigger{
				{ID: "t1", Name: "Keep", Condition: "enter", Enabled: true},
				{ID: "t2", Name: "Delete Me", Condition: "dwell", Enabled: true},
			},
			deleteID: "t2",
			wantCode: http.StatusNoContent,
			wantLen:  1,
		},
		{
			name:     "delete nonexistent trigger",
			setup:    []Trigger{{ID: "t1", Name: "Only", Condition: "enter", Enabled: true}},
			deleteID: "nonexistent",
			wantCode: http.StatusNotFound,
			wantLen:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, cleanup := newTriggerTestHandler(t)
			defer cleanup()

			for _, tr := range tt.setup {
				seedTrigger(t, h, tr)
			}

			r := newTriggerTestRouter(h)
			req := httptest.NewRequest("DELETE", "/api/triggers/"+tt.deleteID, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Fatalf("expected %d, got %d: %s", tt.wantCode, w.Code, w.Body.String())
			}

			// Verify via list
			req2 := httptest.NewRequest("GET", "/api/triggers", nil)
			w2 := httptest.NewRecorder()
			r.ServeHTTP(w2, req2)
			var result []Trigger
			json.NewDecoder(w2.Body).Decode(&result)
			if len(result) != tt.wantLen {
				t.Errorf("expected %d triggers after delete, got %d", tt.wantLen, len(result))
			}

			// Verify deleted trigger is not in memory
			if tt.wantCode == http.StatusNoContent {
				h.mu.RLock()
				_, exists := h.triggers[tt.deleteID]
				h.mu.RUnlock()
				if exists {
					t.Error("trigger should be removed from memory")
				}
			}
		})
	}
}

// ── POST /api/triggers/{id}/test ─────────────────────────────────────────────────

// TestTestTrigger tests POST /api/triggers/{id}/test.
func TestTestTrigger(t *testing.T) {
	tests := []struct {
		name     string
		setup    Trigger
		testID   string
		engine   TriggerEngine
		wantCode int
		wantKey  string
	}{
		{
			name:   "test with no engine returns simulated",
			setup:  Trigger{ID: "t1", Name: "Sim", Condition: "dwell", Enabled: true},
			testID: "t1",
			engine: nil,
			wantCode: http.StatusOK,
			wantKey:  "simulated",
		},
		{
			name:   "test with engine that succeeds",
			setup:  Trigger{ID: "t1", Name: "Fire", Condition: "enter", Enabled: true},
			testID: "t1",
			engine: &mockEngine{err: nil},
			wantCode: http.StatusOK,
			wantKey:  "fired",
		},
		{
			name:   "test with engine that fails",
			setup:  Trigger{ID: "t1", Name: "Fail", Condition: "leave", Enabled: true},
			testID: "t1",
			engine: &mockEngine{err: fmt.Errorf("boom")},
			wantCode: http.StatusInternalServerError,
			wantKey:  "test fire failed",
		},
		{
			name:   "test nonexistent trigger",
			setup:  Trigger{ID: "t1", Name: "Exists", Condition: "enter", Enabled: true},
			testID: "nonexistent",
			engine: nil,
			wantCode: http.StatusNotFound,
			wantKey:  "trigger not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, cleanup := newTriggerTestHandler(t)
			defer cleanup()

			seedTrigger(t, h, tt.setup)
			if tt.engine != nil {
				h.SetEngine(tt.engine)
			}

			r := newTriggerTestRouter(h)
			req := httptest.NewRequest("POST", "/api/triggers/"+tt.testID+"/test", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Fatalf("expected %d, got %d: %s", tt.wantCode, w.Code, w.Body.String())
			}

			if !bytes.Contains(w.Body.Bytes(), []byte(tt.wantKey)) {
				t.Errorf("expected response to contain %q, got %s", tt.wantKey, w.Body.String())
			}
		})
	}
}

// TestTestTriggerDoesNotUpdateLastFired verifies that the test endpoint
// does not modify last_fired on the trigger.
func TestTestTriggerDoesNotUpdateLastFired(t *testing.T) {
	h, cleanup := newTriggerTestHandler(t)
	defer cleanup()

	tr := Trigger{ID: "t1", Name: "Test", Condition: "enter", Enabled: true}
	seedTrigger(t, h, tr)

	r := newTriggerTestRouter(h)
	req := httptest.NewRequest("POST", "/api/triggers/t1/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	h.mu.RLock()
	trigger := h.triggers["t1"]
	h.mu.RUnlock()

	if trigger.LastFired != nil {
		t.Error("expected last_fired to remain nil after test endpoint")
	}
}

// ── CRUD round-trip ───────────────────────────────────────────────────────────────

// TestTriggerCRUDRoundTrip verifies the full lifecycle: create -> list -> update -> list -> delete -> verify gone.
func TestTriggerCRUDRoundTrip(t *testing.T) {
	h, cleanup := newTriggerTestHandler(t)
	defer cleanup()

	r := newTriggerTestRouter(h)

	// 1. Create
	body := `{"id": "rt", "name": "Round Trip", "condition": "dwell", "condition_params": {"duration_s": 60}}`
	req := httptest.NewRequest("POST", "/api/triggers", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", w.Code)
	}

	// 2. List and verify
	req2 := httptest.NewRequest("GET", "/api/triggers", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	var triggers []Trigger
	json.NewDecoder(w2.Body).Decode(&triggers)
	if len(triggers) != 1 {
		t.Fatalf("after create: expected 1 trigger, got %d", len(triggers))
	}
	if triggers[0].Name != "Round Trip" {
		t.Errorf("after create: expected name 'Round Trip', got %s", triggers[0].Name)
	}

	// 3. Update
	body3 := `{"name": "Updated Trip", "enabled": false}`
	req3 := httptest.NewRequest("PUT", "/api/triggers/rt", bytes.NewReader([]byte(body3)))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d", w3.Code)
	}

	// 4. Verify update
	req4 := httptest.NewRequest("GET", "/api/triggers", nil)
	w4 := httptest.NewRecorder()
	r.ServeHTTP(w4, req4)
	json.NewDecoder(w4.Body).Decode(&triggers)
	if triggers[0].Name != "Updated Trip" {
		t.Errorf("after update: expected name 'Updated Trip', got %s", triggers[0].Name)
	}
	if triggers[0].Enabled {
		t.Error("after update: expected enabled=false")
	}

	// 5. Delete
	req5 := httptest.NewRequest("DELETE", "/api/triggers/rt", nil)
	w5 := httptest.NewRecorder()
	r.ServeHTTP(w5, req5)
	if w5.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", w5.Code)
	}

	// 6. Verify gone
	req6 := httptest.NewRequest("GET", "/api/triggers", nil)
	w6 := httptest.NewRecorder()
	r.ServeHTTP(w6, req6)
	json.NewDecoder(w6.Body).Decode(&triggers)
	if len(triggers) != 0 {
		t.Errorf("after delete: expected 0 triggers, got %d", len(triggers))
	}
}

// ── EvaluateTriggers ─────────────────────────────────────────────────────────────

// TestEvaluateTriggers tests trigger evaluation logic.
func TestEvaluateTriggers(t *testing.T) {
	h, cleanup := newTriggerTestHandler(t)
	defer cleanup()

	seedTrigger(t, h, Trigger{
		ID:        "vacant",
		Name:      "House Empty",
		Condition: "vacant",
		Enabled:   true,
	})
	seedTrigger(t, h, Trigger{
		ID:        "disabled",
		Name:      "Disabled",
		Condition: "vacant",
		Enabled:   false,
	})

	// No blobs = vacant should fire
	fired := h.EvaluateTriggers(nil)
	if len(fired) != 1 || fired[0] != "vacant" {
		t.Errorf("expected [vacant], got %v", fired)
	}

	// With blobs = vacant should not fire
	fired = h.EvaluateTriggers([]BlobPos{{ID: 1, X: 1, Y: 1, Z: 1}})
	if len(fired) != 0 {
		t.Errorf("expected no fires with blob present, got %v", fired)
	}

	// Disabled trigger never fires
	fired = h.EvaluateTriggers(nil)
	for _, id := range fired {
		if id == "disabled" {
			t.Error("disabled trigger should not fire")
		}
	}
}

// ── mock engine ──────────────────────────────────────────────────────────────────

type mockEngine struct {
	err error
}

func (m *mockEngine) TestFire(triggerID string) error { return m.err }
func (m *mockEngine) IsInVolume(x, y, z float64, volumeID string) bool {
	return true
}
