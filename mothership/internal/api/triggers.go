// Package api provides REST API handlers for Spaxel automation triggers.
package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi"
	_ "modernc.org/sqlite"
)

// TriggersHandler manages automation triggers.
type TriggersHandler struct {
	mu       sync.RWMutex
	db       *sql.DB
	triggers map[string]*Trigger
	engine   TriggerEngine
}

// Trigger represents an automation trigger.
type Trigger struct {
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	Enabled         bool             `json:"enabled"`
	Condition       string           `json:"condition"` // enter, leave, dwell, vacant, count
	ConditionParams json.RawMessage `json:"condition_params"`
	TimeConstraint  json.RawMessage `json:"time_constraint,omitempty"`
	Actions         json.RawMessage `json:"actions"`
	LastFired       *time.Time       `json:"last_fired,omitempty"`
	Elapsed         int              `json:"elapsed,omitempty"` // seconds since last fire
	CreatedAt       time.Time        `json:"created_at"`
}

// TriggerEngine is the interface to the automation engine.
type TriggerEngine interface {
	TestFire(triggerID string) error
	IsInVolume(x, y, z float64, volumeID string) bool
}

// NewTriggersHandler creates a new triggers handler.
func NewTriggersHandler(dbPath string) (*TriggersHandler, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	t := &TriggersHandler{
		db:       db,
		triggers: make(map[string]*Trigger),
	}

	if err := t.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	if err := t.load(); err != nil {
		log.Printf("[WARN] Failed to load triggers: %v", err)
	}

	return t, nil
}

func (t *TriggersHandler) migrate() error {
	_, err := t.db.Exec(`
		CREATE TABLE IF NOT EXISTS triggers (
			id                TEXT PRIMARY KEY,
			name              TEXT    NOT NULL DEFAULT '',
			enabled           INTEGER NOT NULL DEFAULT 1,
			condition         TEXT    NOT NULL,
			condition_params  TEXT    NOT NULL DEFAULT '{}',
			time_constraint   TEXT    NOT NULL DEFAULT '{}',
			actions           TEXT    NOT NULL DEFAULT '[]',
			last_fired        INTEGER NOT NULL DEFAULT 0,
			created_at        INTEGER NOT NULL DEFAULT 0
		);
	`)
	return err
}

func (t *TriggersHandler) load() error {
	rows, err := t.db.Query(`
		SELECT id, name, enabled, condition, condition_params, time_constraint, actions, last_fired, created_at
		FROM triggers
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var trigger Trigger
		var enabled int
		var lastFiredNS int64
		var createdAtNS int64

		if err := rows.Scan(&trigger.ID, &trigger.Name, &enabled, &trigger.Condition,
			&trigger.ConditionParams, &trigger.TimeConstraint, &trigger.Actions,
			&lastFiredNS, &createdAtNS); err != nil {
			continue
		}

		trigger.Enabled = enabled != 0
		if lastFiredNS > 0 {
			ts := time.Unix(0, lastFiredNS)
			trigger.LastFired = &ts
			trigger.Elapsed = int(time.Since(ts).Seconds())
		}
		trigger.CreatedAt = time.Unix(0, createdAtNS)

		t.triggers[trigger.ID] = &trigger
	}

	return nil
}

// Close closes the database.
func (t *TriggersHandler) Close() error {
	return t.db.Close()
}

// SetEngine sets the automation engine for testing.
func (t *TriggersHandler) SetEngine(engine TriggerEngine) {
	t.mu.Lock()
	t.engine = engine
	t.mu.Unlock()
}

// RegisterRoutes registers triggers endpoints on the given router.
//
// Routes:
//
//	GET    /api/triggers          — list all triggers
//	POST   /api/triggers          — create a new trigger
//	PUT    /api/triggers/{id}     — update an existing trigger
//	DELETE /api/triggers/{id}     — delete a trigger
//	POST   /api/triggers/{id}/test — fire trigger actions once for testing
func (t *TriggersHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/triggers", t.listTriggers)
	r.Post("/api/triggers", t.createTrigger)
	r.Put("/api/triggers/{id}", t.updateTrigger)
	r.Delete("/api/triggers/{id}", t.deleteTrigger)
	r.Post("/api/triggers/{id}/test", t.testTrigger)
}

// listTriggers handles GET /api/triggers.
//
// Returns all registered triggers as a JSON array.
//
// Response 200 (application/json):
//
//	[{
//	  "id": "t1",
//	  "name": "Couch Dwell",
//	  "enabled": true,
//	  "condition": "dwell",
//	  "condition_params": {"duration_s": 30},
//	  "time_constraint": {"from": "22:00", "to": "06:00"},
//	  "actions": [{"type": "webhook", "url": "http://example.com/hook"}],
//	  "last_fired": "2024-03-15T14:32:05Z",
//	  "elapsed": 142,
//	  "created_at": "2024-03-10T08:00:00Z"
//	}]
func (t *TriggersHandler) listTriggers(w http.ResponseWriter, r *http.Request) {
	t.mu.RLock()
	triggers := make([]*Trigger, 0, len(t.triggers))
	for _, trigger := range t.triggers {
		if trigger.LastFired != nil {
			trigger.Elapsed = int(time.Since(*trigger.LastFired).Seconds())
		}
		triggers = append(triggers, trigger)
	}
	t.mu.RUnlock()

	writeJSON(w, http.StatusOK, triggers)
}

type createTriggerRequest struct {
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	Enabled         *bool            `json:"enabled,omitempty"`
	Condition       string           `json:"condition"`
	ConditionParams json.RawMessage `json:"condition_params"`
	TimeConstraint  json.RawMessage `json:"time_constraint,omitempty"`
	Actions         json.RawMessage `json:"actions"`
}

// createTrigger handles POST /api/triggers.
//
// Creates a new automation trigger. The request body must include id, name,
// and condition. Actions default to an empty array if omitted.
//
// Request body (application/json):
//
//	{
//	  "id": "t1",
//	  "name": "Couch Dwell",
//	  "condition": "dwell",
//	  "condition_params": {"duration_s": 30},
//	  "time_constraint": {"from": "22:00", "to": "06:00"},
//	  "actions": [{"type": "webhook", "url": "http://example.com/hook"}],
//	  "enabled": true
//	}
//
// Response 201 (application/json): the created trigger object.
// Response 400: missing required fields or invalid condition value.
// Response 500: database error.
func (t *TriggersHandler) createTrigger(w http.ResponseWriter, r *http.Request) {
	var req createTriggerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.ID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	validConditions := map[string]bool{
		"enter": true, "leave": true, "dwell": true,
		"vacant": true, "count": true,
	}
	if !validConditions[req.Condition] {
		http.Error(w, "condition must be one of: enter, leave, dwell, vacant, count", http.StatusBadRequest)
		return
	}

	now := time.Now().UnixNano()
	enabled := 1
	if req.Enabled != nil && !*req.Enabled {
		enabled = 0
	}

	conditionParams := req.ConditionParams
	if len(conditionParams) == 0 {
		conditionParams = []byte("{}")
	}
	timeConstraint := req.TimeConstraint
	if len(timeConstraint) == 0 {
		timeConstraint = []byte("{}")
	}
	actions := req.Actions
	if len(actions) == 0 {
		actions = []byte("[]")
	}

	_, err := t.db.Exec(`
		INSERT INTO triggers (id, name, enabled, condition, condition_params, time_constraint, actions, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, req.ID, req.Name, enabled, req.Condition, string(conditionParams),
		string(timeConstraint), string(actions), now)
	if err != nil {
		http.Error(w, "failed to create trigger", http.StatusInternalServerError)
		return
	}

	t.mu.Lock()
	t.triggers[req.ID] = &Trigger{
		ID:              req.ID,
		Name:            req.Name,
		Enabled:         enabled != 0,
		Condition:       req.Condition,
		ConditionParams: conditionParams,
		TimeConstraint:  timeConstraint,
		Actions:         actions,
		CreatedAt:       time.Unix(0, now),
	}
	t.mu.Unlock()

	writeJSON(w, http.StatusCreated, t.triggers[req.ID])
}

type updateTriggerRequest struct {
	Name            *string          `json:"name,omitempty"`
	Enabled         *bool            `json:"enabled,omitempty"`
	Condition       *string          `json:"condition,omitempty"`
	ConditionParams *json.RawMessage `json:"condition_params,omitempty"`
	TimeConstraint  *json.RawMessage `json:"time_constraint,omitempty"`
	Actions         *json.RawMessage `json:"actions,omitempty"`
}

// updateTrigger handles PUT /api/triggers/{id}.
//
// Updates an existing trigger. Only fields present in the request body are
// modified; omitted fields retain their current values. If the body contains
// no recognized fields, the current trigger is returned unchanged.
//
// Request body (application/json): partial trigger object with fields to update.
//
// Response 200 (application/json): the updated trigger object.
// Response 400: invalid request body or invalid condition value.
// Response 404: trigger not found.
// Response 500: database error.
func (t *TriggersHandler) updateTrigger(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	t.mu.RLock()
	trigger, exists := t.triggers[id]
	t.mu.RUnlock()

	if !exists {
		http.Error(w, "trigger not found", http.StatusNotFound)
		return
	}

	var req updateTriggerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	updates := []string{}
	args := []interface{}{}

	if req.Name != nil {
		updates = append(updates, "name = ?")
		args = append(args, *req.Name)
	}
	if req.Enabled != nil {
		updates = append(updates, "enabled = ?")
		if *req.Enabled {
			args = append(args, 1)
		} else {
			args = append(args, 0)
		}
	}
	if req.Condition != nil {
		validConditions := map[string]bool{
			"enter": true, "leave": true, "dwell": true,
			"vacant": true, "count": true,
		}
		if !validConditions[*req.Condition] {
			http.Error(w, "condition must be one of: enter, leave, dwell, vacant, count", http.StatusBadRequest)
			return
		}
		updates = append(updates, "condition = ?")
		args = append(args, *req.Condition)
	}
	if req.ConditionParams != nil {
		updates = append(updates, "condition_params = ?")
		args = append(args, string(*req.ConditionParams))
	}
	if req.TimeConstraint != nil {
		updates = append(updates, "time_constraint = ?")
		args = append(args, string(*req.TimeConstraint))
	}
	if req.Actions != nil {
		updates = append(updates, "actions = ?")
		args = append(args, string(*req.Actions))
	}

	if len(updates) == 0 {
		writeJSON(w, http.StatusOK, trigger)
		return
	}

	args = append(args, id)
	query := "UPDATE triggers SET " + strings.Join(updates, ", ") + " WHERE id = ?"

	_, err := t.db.Exec(query, args...)
	if err != nil {
		http.Error(w, "failed to update trigger", http.StatusInternalServerError)
		return
	}

	// Update in-memory copy
	t.mu.Lock()
	if req.Name != nil {
		trigger.Name = *req.Name
	}
	if req.Enabled != nil {
		trigger.Enabled = *req.Enabled
	}
	if req.Condition != nil {
		trigger.Condition = *req.Condition
	}
	if req.ConditionParams != nil {
		trigger.ConditionParams = *req.ConditionParams
	}
	if req.TimeConstraint != nil {
		trigger.TimeConstraint = *req.TimeConstraint
	}
	if req.Actions != nil {
		trigger.Actions = *req.Actions
	}
	t.mu.Unlock()

	writeJSON(w, http.StatusOK, trigger)
}

// deleteTrigger handles DELETE /api/triggers/{id}.
//
// Removes a trigger by ID. Deleting a nonexistent ID is a no-op.
//
// Response 204: trigger deleted (or did not exist).
// Response 500: database error.
func (t *TriggersHandler) deleteTrigger(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	t.mu.RLock()
	_, exists := t.triggers[id]
	t.mu.RUnlock()

	if !exists {
		http.Error(w, "trigger not found", http.StatusNotFound)
		return
	}

	_, err := t.db.Exec(`DELETE FROM triggers WHERE id = ?`, id)
	if err != nil {
		http.Error(w, "failed to delete trigger", http.StatusInternalServerError)
		return
	}

	t.mu.Lock()
	delete(t.triggers, id)
	t.mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

// testTrigger handles POST /api/triggers/{id}/test.
//
// Fires the trigger's actions once with a synthetic event payload for testing.
// If no automation engine is attached, returns a simulated success response.
// Does not update last_fired or trigger any real automation logic.
//
// Response 200 (application/json):
//
//	{
//	  "status": "fired",
//	  "message": "Trigger fired successfully",
//	  "trigger": { ... }
//	}
//
// Response 404: trigger not found.
// Response 500: engine test-fire failed.
func (t *TriggersHandler) testTrigger(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	t.mu.RLock()
	trigger, exists := t.triggers[id]
	t.mu.RUnlock()

	if !exists {
		http.Error(w, "trigger not found", http.StatusNotFound)
		return
	}

	t.mu.RLock()
	engine := t.engine
	t.mu.RUnlock()

	if engine == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":  "ok",
			"message": "trigger test simulated (no engine attached)",
			"trigger": trigger,
		})
		return
	}

	if err := engine.TestFire(id); err != nil {
		http.Error(w, fmt.Sprintf("test fire failed: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "fired",
		"message": "Trigger fired successfully",
		"trigger": trigger,
	})
}

// ── Trigger evaluation (called by fusion engine) ───────────────────────────────────

// EvaluateTriggers evaluates all enabled triggers against current state.
// Returns a list of trigger IDs that should fire.
func (t *TriggersHandler) EvaluateTriggers(blobs []BlobPos) []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var fired []string

	for _, trigger := range t.triggers {
		if !trigger.Enabled {
			continue
		}

		// Check cooldown (5 second minimum refire interval)
		if trigger.LastFired != nil && time.Since(*trigger.LastFired) < 5*time.Second {
			continue
		}

		// Parse condition params
		var params struct {
			DurationS      *int `json:"duration_s"`
			CountThreshold *int `json:"count_threshold"`
			PersonID       string `json:"person_id,omitempty"`
			VolumeID       string `json:"volume_id,omitempty"`
		}
		if len(trigger.ConditionParams) > 0 && string(trigger.ConditionParams) != "{}" {
			json.Unmarshal(trigger.ConditionParams, &params)
		}

		shouldFire := false
		switch trigger.Condition {
		case "enter", "leave":
			if params.VolumeID != "" {
				for _, blob := range blobs {
					if t.engine != nil && t.engine.IsInVolume(blob.X, blob.Y, blob.Z, params.VolumeID) {
						if trigger.Condition == "enter" {
							shouldFire = true
						}
					} else {
						if trigger.Condition == "leave" {
							shouldFire = true
						}
					}
				}
			}
		case "dwell":
			if params.DurationS != nil && trigger.LastFired != nil {
				elapsed := int(time.Since(*trigger.LastFired).Seconds())
				if elapsed >= *params.DurationS {
					shouldFire = true
				}
			}
		case "vacant":
			if len(blobs) == 0 {
				shouldFire = true
			}
		case "count":
			if params.CountThreshold != nil {
				if len(blobs) >= *params.CountThreshold {
					shouldFire = true
				}
			}
		}

		if shouldFire {
			fired = append(fired, trigger.ID)
			now := time.Now()
			trigger.LastFired = &now
			trigger.Elapsed = 0
			t.db.Exec(`UPDATE triggers SET last_fired = ? WHERE id = ?`, now.UnixNano(), trigger.ID)
		}
	}

	return fired
}

// BlobPos represents a blob position for trigger evaluation.
type BlobPos struct {
	ID      int
	X, Y, Z float64
}
