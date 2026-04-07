// Package volume provides 3D trigger volume geometry and point-in-volume testing
// for spatial automation triggers.
package volume

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// ShapeType represents the type of volume geometry.
type ShapeType string

const (
	ShapeBox      ShapeType = "box"
	ShapeCylinder ShapeType = "cylinder"
)

// ShapeJSON represents the geometry of a trigger volume as JSON.
// For box:   {"type":"box","x":0,"y":0,"z":0,"w":1,"d":1,"h":2}
// For cylinder: {"type":"cylinder","cx":0,"cy":0,"z":0,"r":0.5,"h":2}
type ShapeJSON struct {
	Type ShapeType `json:"type"`
	// Box fields (axis-aligned bounding box)
	X *float64 `json:"x,omitempty"` // Box origin X
	Y *float64 `json:"y,omitempty"` // Box origin Y
	Z *float64 `json:"z,omitempty"` // Box origin Z
	W *float64 `json:"w,omitempty"` // Box width
	D *float64 `json:"d,omitempty"` // Box depth
	H *float64 `json:"h,omitempty"` // Box height (or cylinder height)
	// Cylinder fields
	CX *float64 `json:"cx,omitempty"` // Cylinder center X
	CY *float64 `json:"cy,omitempty"` // Cylinder center Y
	R  *float64 `json:"r,omitempty"`  // Cylinder radius
}

// Point3D represents a 3D point for volume testing.
type Point3D struct {
	X float64
	Y float64
	Z float64
}

// IsInside returns true if the point is inside the volume.
func (s *ShapeJSON) IsInside(p Point3D) bool {
	switch s.Type {
	case ShapeBox:
		return s.isInsideBox(p)
	case ShapeCylinder:
		return s.isInsideCylinder(p)
	default:
		return false
	}
}

// isInsideBox tests if a point is inside an axis-aligned box.
// Box definition: {type:"box", x, y, z, w, d, h}
// The box spans from (x, y, z) to (x+w, y+h, z+d).
func (s *ShapeJSON) isInsideBox(p Point3D) bool {
	if s.X == nil || s.Y == nil || s.Z == nil || s.W == nil || s.D == nil || s.H == nil {
		return false
	}

	x, y, z := *s.X, *s.Y, *s.Z
	w, d, h := *s.W, *s.D, *s.H

	// Box spans from (x, y, z) to (x+w, y+h, z+d)
	return p.X >= x && p.X < x+w &&
		p.Y >= y && p.Y < y+h &&
		p.Z >= z && p.Z < z+d
}

// isInsideCylinder tests if a point is inside a cylinder.
// Cylinder definition: {type:"cylinder", cx, cy, z, r, h}
// The cylinder is vertical (aligned with Y axis), centered at (cx, cy, z),
// with radius r and extending from z to z+h in height.
func (s *ShapeJSON) isInsideCylinder(p Point3D) bool {
	if s.CX == nil || s.CY == nil || s.Z == nil || s.R == nil || s.H == nil {
		return false
	}

	cx, cy := *s.CX, *s.CY
	z, r, h := *s.Z, *s.R, *s.H

	// Check horizontal distance from center (in X-Z plane for vertical cylinder)
	dx := p.X - cx
	dy := p.Y - cy
	distSq := dx*dx + dy*dy

	// Check if within radius and within height bounds
	return distSq <= r*r && p.Z >= z && p.Z < z+h
}

// Trigger represents a spatial automation trigger from the triggers table.
type Trigger struct {
	ID                 string     `json:"id"`
	Name               string     `json:"name"`
	Shape              ShapeJSON  `json:"shape"`
	Condition          string     `json:"condition"`          // enter, leave, dwell, vacant, count
	ConditionParams    ConditionParams `json:"condition_params"`
	TimeConstraint     *TimeConstraint `json:"time_constraint,omitempty"`
	Actions            []Action    `json:"actions"`
	Enabled            bool       `json:"enabled"`
	ErrorMessage       string     `json:"error_message,omitempty"` // Set when disabled by 4xx
	ErrorCount         int        `json:"error_count"`            // Incremented on 5xx/timeout, reset on 2xx
	LastFired          *time.Time `json:"last_fired,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// ConditionParams holds trigger condition parameters.
type ConditionParams struct {
	DurationS      *int    `json:"duration_s,omitempty"`      // For dwell: seconds inside volume
	CountThreshold *int    `json:"count_threshold,omitempty"` // For count: minimum blob count
	PersonID       string  `json:"person_id,omitempty"`      // Filter by person ID
}

// TimeConstraint represents a time window constraint.
type TimeConstraint struct {
	From string `json:"from"` // HH:MM format
	To   string `json:"to"`   // HH:MM format
}

// Action represents an action to execute when a trigger fires.
type Action struct {
	Type    string                 `json:"type"` // webhook, mqtt, internal
	Params  map[string]interface{} `json:"params"`
}

// BlobState represents the state of a tracked blob relative to a trigger.
type BlobState struct {
	BlobID        int
	Inside        bool      // Current inside/outside state
	EnterTime     time.Time // When blob entered the volume
	LastCheckTime time.Time // Last evaluation time
}

// TriggerState holds the state machine for a trigger across all blobs.
type TriggerState struct {
	TriggerID        string
	Blobs            map[int]*BlobState // blob_id -> state
	LastFired        time.Time
	VacantTimerStart time.Time // Separate field so fireTrigger doesn't clobber the vacant timer
}

// FiredEvent represents a trigger firing event.
type FiredEvent struct {
	TriggerID   string
	TriggerName string
	Condition   string
	BlobIDs     []int
	Timestamp   time.Time
}

// FiringCallback is called when a trigger fires.
type FiringCallback func(event FiredEvent)

// Store provides trigger storage and state management.
type Store struct {
	mu           sync.RWMutex
	db           *sql.DB
	triggers     map[string]*Trigger
	triggerState map[string]*TriggerState // trigger_id -> state
	blobVolumes  map[int]string          // blob_id -> current volume_id (for tracking)
	onFired      FiringCallback          // Called when a trigger fires
}

// NewStore creates a new trigger volume store.
func NewStore(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	s := &Store{
		db:           db,
		triggers:     make(map[string]*Trigger),
		triggerState: make(map[string]*TriggerState),
		blobVolumes:  make(map[int]string),
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	if err := s.init(); err != nil {
		db.Close()
		return nil, fmt.Errorf("init store: %w", err)
	}

	return s, nil
}

func (s *Store) init() error {
	// Create triggers table if not exists (matches schema in migrations.go)
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS triggers (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			name        TEXT NOT NULL,
			shape_json  TEXT NOT NULL,
			condition   TEXT NOT NULL CHECK (condition IN ('enter','leave','dwell','vacant','count')),
			condition_params_json TEXT,
			time_constraint_json TEXT,
			actions_json TEXT NOT NULL,
			enabled     INTEGER NOT NULL DEFAULT 1,
			last_fired  INTEGER,
			created_at  INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000),
			updated_at  INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
		);
	`)
	if err != nil {
		return fmt.Errorf("create triggers table: %w", err)
	}

	// Add error_message and error_count columns (idempotent via try/catch pattern)
	s.db.Exec(`ALTER TABLE triggers ADD COLUMN error_message TEXT DEFAULT ''`)
	s.db.Exec(`ALTER TABLE triggers ADD COLUMN error_count INTEGER NOT NULL DEFAULT 0`)

	// Create trigger_state table for persisting blob states across restarts
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS trigger_state (
			trigger_id  INTEGER NOT NULL,
			blob_id     INTEGER NOT NULL,
			inside      INTEGER NOT NULL DEFAULT 0,
			enter_time  INTEGER NOT NULL DEFAULT 0,
			last_check  INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (trigger_id, blob_id),
			FOREIGN KEY (trigger_id) REFERENCES triggers(id) ON DELETE CASCADE
		);
	`)
	if err != nil {
		return fmt.Errorf("create trigger_state table: %w", err)
	}

	// Create webhook_log audit table
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS webhook_log (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			trigger_id  INTEGER NOT NULL,
			fired_at_ms INTEGER NOT NULL,
			url         TEXT NOT NULL,
			status_code INTEGER,
			latency_ms  INTEGER NOT NULL DEFAULT 0,
			error       TEXT DEFAULT '',
			FOREIGN KEY (trigger_id) REFERENCES triggers(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_webhook_log_trigger ON webhook_log(trigger_id, fired_at_ms DESC);
	`)
	if err != nil {
		return fmt.Errorf("create webhook_log table: %w", err)
	}

	return s.load()
}

func (s *Store) load() error {
	// Load triggers
	rows, err := s.db.Query(`
		SELECT id, name, shape_json, condition, condition_params_json,
		       time_constraint_json, actions_json, enabled, last_fired, created_at, updated_at,
		       COALESCE(error_message, ''), COALESCE(error_count, 0)
		FROM triggers
	`)
	if err != nil {
		return fmt.Errorf("query triggers: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var t Trigger
		var id int
		var shapeJSON, conditionParamsJSON, timeConstraintJSON, actionsJSON string
		var enabled int
		var lastFiredMs sql.NullInt64
		var createdAtMs, updatedAtMs int64

		if err := rows.Scan(&id, &t.Name, &shapeJSON, &t.Condition, &conditionParamsJSON,
			&timeConstraintJSON, &actionsJSON, &enabled, &lastFiredMs, &createdAtMs, &updatedAtMs,
			&t.ErrorMessage, &t.ErrorCount); err != nil {
			log.Printf("[WARN] Failed to scan trigger: %v", err)
			continue
		}

		t.ID = fmt.Sprintf("%d", id)
		t.Enabled = enabled != 0

		if err := json.Unmarshal([]byte(shapeJSON), &t.Shape); err != nil {
			log.Printf("[WARN] Failed to parse shape_json for trigger %s: %v", t.ID, err)
			continue
		}

		if conditionParamsJSON != "" && conditionParamsJSON != "{}" {
			json.Unmarshal([]byte(conditionParamsJSON), &t.ConditionParams)
		}

		if timeConstraintJSON != "" && timeConstraintJSON != "{}" {
			json.Unmarshal([]byte(timeConstraintJSON), &t.TimeConstraint)
		}

		if actionsJSON != "" && actionsJSON != "[]" {
			json.Unmarshal([]byte(actionsJSON), &t.Actions)
		}

		if lastFiredMs.Valid && lastFiredMs.Int64 > 0 {
			ts := time.Unix(0, lastFiredMs.Int64)
			t.LastFired = &ts
		}

		t.CreatedAt = time.Unix(0, createdAtMs)
		t.UpdatedAt = time.Unix(0, updatedAtMs)

		s.triggers[t.ID] = &t
		s.triggerState[t.ID] = &TriggerState{
			TriggerID: t.ID,
			Blobs:     make(map[int]*BlobState),
		}
	}

	// Load blob states
	stateRows, err := s.db.Query(`SELECT trigger_id, blob_id, inside, enter_time, last_check FROM trigger_state`)
	if err != nil {
		return fmt.Errorf("query trigger_state: %w", err)
	}
	defer stateRows.Close()

	for stateRows.Next() {
		var triggerID string
		var blobID int
		var inside int
		var enterTimeMs, lastCheckMs int64

		if err := stateRows.Scan(&triggerID, &blobID, &inside, &enterTimeMs, &lastCheckMs); err != nil {
			continue
		}

		state := s.triggerState[triggerID]
		if state == nil {
			continue
		}

		state.Blobs[blobID] = &BlobState{
			BlobID:        blobID,
			Inside:        inside != 0,
			EnterTime:     time.Unix(0, enterTimeMs),
			LastCheckTime: time.Unix(0, lastCheckMs),
		}
	}

	return nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// Create creates a new trigger.
func (s *Store) Create(t *Trigger) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	shapeJSON, err := json.Marshal(t.Shape)
	if err != nil {
		return "", fmt.Errorf("marshal shape: %w", err)
	}

	conditionParamsJSON, _ := json.Marshal(t.ConditionParams)
	var timeConstraintJSON []byte
	if t.TimeConstraint != nil {
		timeConstraintJSON, _ = json.Marshal(t.TimeConstraint)
	}
	actionsJSON, _ := json.Marshal(t.Actions)

	now := time.Now().UnixNano()
	enabled := 0
	if t.Enabled {
		enabled = 1
	}

	result, err := s.db.Exec(`
		INSERT INTO triggers (name, shape_json, condition, condition_params_json,
		                      time_constraint_json, actions_json, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, t.Name, string(shapeJSON), t.Condition, string(conditionParamsJSON),
		string(timeConstraintJSON), string(actionsJSON), enabled, now, now)
	if err != nil {
		return "", fmt.Errorf("insert trigger: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return "", fmt.Errorf("get last insert id: %w", err)
	}

	t.ID = fmt.Sprintf("%d", id)
	t.CreatedAt = time.Unix(0, now)
	t.UpdatedAt = time.Unix(0, now)

	s.triggers[t.ID] = t
	s.triggerState[t.ID] = &TriggerState{
		TriggerID: t.ID,
		Blobs:     make(map[int]*BlobState),
	}

	return t.ID, nil
}

// Update updates an existing trigger.
func (s *Store) Update(t *Trigger) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.triggers[t.ID]; !exists {
		return fmt.Errorf("trigger not found: %s", t.ID)
	}

	shapeJSON, err := json.Marshal(t.Shape)
	if err != nil {
		return fmt.Errorf("marshal shape: %w", err)
	}

	conditionParamsJSON, _ := json.Marshal(t.ConditionParams)
	var timeConstraintJSON []byte
	if t.TimeConstraint != nil {
		timeConstraintJSON, _ = json.Marshal(t.TimeConstraint)
	}
	actionsJSON, _ := json.Marshal(t.Actions)

	now := time.Now().UnixNano()
	enabled := 0
	if t.Enabled {
		enabled = 1
	}

	_, err = s.db.Exec(`
		UPDATE triggers SET name=?, shape_json=?, condition=?, condition_params_json=?,
		                    time_constraint_json=?, actions_json=?, enabled=?, updated_at=?
		WHERE id=?
	`, t.Name, string(shapeJSON), t.Condition, string(conditionParamsJSON),
		string(timeConstraintJSON), string(actionsJSON), enabled, now, t.ID)
	if err != nil {
		return fmt.Errorf("update trigger: %w", err)
	}

	t.UpdatedAt = time.Unix(0, now)
	s.triggers[t.ID] = t

	return nil
}

// Delete deletes a trigger.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`DELETE FROM triggers WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("delete trigger: %w", err)
	}

	delete(s.triggers, id)
	delete(s.triggerState, id)

	// Clear blob volumes for deleted trigger
	for blobID, volID := range s.blobVolumes {
		if volID == id {
			delete(s.blobVolumes, blobID)
		}
	}

	return nil
}

// Get retrieves a trigger by ID.
func (s *Store) Get(id string) (*Trigger, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	t, exists := s.triggers[id]
	if !exists {
		return nil, fmt.Errorf("trigger not found: %s", id)
	}

	return t, nil
}

// GetAll returns all triggers.
func (s *Store) GetAll() []*Trigger {
	s.mu.RLock()
	defer s.mu.RUnlock()

	triggers := make([]*Trigger, 0, len(s.triggers))
	for _, t := range s.triggers {
		triggers = append(triggers, t)
	}

	return triggers
}

// SetOnFired sets the callback that is invoked when a trigger fires.
func (s *Store) SetOnFired(cb FiringCallback) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onFired = cb
}

// Evaluate evaluates all enabled triggers against the current blob positions.
// Returns a list of trigger IDs that should fire.
func (s *Store) Evaluate(blobs []BlobPos, now time.Time) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	var fired []string

	for _, t := range s.triggers {
		if !t.Enabled {
			continue
		}

		// Check time constraint
		if t.TimeConstraint != nil && !s.isTimeInRange(t.TimeConstraint, now) {
			continue
		}

		state := s.triggerState[t.ID]

		// Initialize blob states for all current blobs.
		// New blobs start with Inside=false so the condition evaluator can
		// detect the first transition (enter/leave/etc). Without this, a blob
		// that starts inside the volume would never trigger "enter" because
		// no outside→inside transition would be observed.
		for _, blob := range blobs {
			if _, exists := state.Blobs[blob.ID]; !exists {
				state.Blobs[blob.ID] = &BlobState{
					BlobID:        blob.ID,
					Inside:        false,
					LastCheckTime: now,
				}
			}
		}

		// Evaluate condition
		shouldFire := false
		switch t.Condition {
		case "enter":
			shouldFire = s.evaluateEnter(t, state, blobs, now)
		case "leave":
			shouldFire = s.evaluateLeave(t, state, blobs, now)
		case "dwell":
			shouldFire = s.evaluateDwell(t, state, blobs, now)
		case "vacant":
			shouldFire = s.evaluateVacant(t, state, blobs, now)
		case "count":
			shouldFire = s.evaluateCount(t, state, blobs, now)
		}

		if shouldFire {
			fired = append(fired, t.ID)
			s.fireTrigger(t.ID, now)
			// Reset vacant timer after firing so it doesn't re-fire immediately
			if t.Condition == "vacant" {
				state.VacantTimerStart = time.Time{}
			}
		}

		// Persist blob states
		s.persistBlobStates(t.ID, state)
	}

	return fired
}

// evaluateEnter triggers when a blob transitions from outside to inside the volume.
func (s *Store) evaluateEnter(t *Trigger, state *TriggerState, blobs []BlobPos, now time.Time) bool {
	var fired bool

	for _, blob := range blobs {
		// Check person filter
		if t.ConditionParams.PersonID != "" && t.ConditionParams.PersonID != "anyone" {
			// TODO: Filter by person ID when blob has person info
		}

		blobState := state.Blobs[blob.ID]
		wasInside := blobState != nil && blobState.Inside

		isInside := t.Shape.IsInside(Point3D{X: blob.X, Y: blob.Y, Z: blob.Z})

		if !wasInside && isInside {
			// Transition from outside to inside
			blobState.Inside = true
			blobState.EnterTime = now
			blobState.LastCheckTime = now

			s.blobVolumes[blob.ID] = t.ID
			fired = true
		} else if blobState != nil {
			blobState.Inside = isInside
			blobState.LastCheckTime = now
			if !isInside {
				delete(s.blobVolumes, blob.ID)
			}
		}
	}

	return fired
}

// evaluateLeave triggers when a blob transitions from inside to outside the volume.
func (s *Store) evaluateLeave(t *Trigger, state *TriggerState, blobs []BlobPos, now time.Time) bool {
	var fired bool

	for _, blob := range blobs {
		blobState := state.Blobs[blob.ID]
		wasInside := blobState != nil && blobState.Inside

		isInside := t.Shape.IsInside(Point3D{X: blob.X, Y: blob.Y, Z: blob.Z})

		if wasInside && !isInside {
			// Transition from inside to outside
			blobState.Inside = false
			blobState.LastCheckTime = now
			delete(s.blobVolumes, blob.ID)
			fired = true
		} else if blobState != nil {
			blobState.Inside = isInside
			blobState.LastCheckTime = now
			if isInside && blobState.EnterTime.IsZero() {
				blobState.EnterTime = now
				s.blobVolumes[blob.ID] = t.ID
			}
		}
	}

	// Clean up blobs that no longer exist
	for blobID, blobState := range state.Blobs {
		found := false
		for _, blob := range blobs {
			if blob.ID == blobID {
				found = true
				break
			}
		}
		if !found && blobState.Inside {
			// Blob disappeared - treat as leave
			blobState.Inside = false
			delete(s.blobVolumes, blobID)
			fired = true
		}
	}

	return fired
}

// evaluateDwell triggers after a blob has been inside for N continuous seconds.
// Per spec: fires exactly once per entry; re-fires after blob leaves and re-enters.
// Fire rate limiting: minimum 60s between dwell firings (after firing, must exit and re-enter).
func (s *Store) evaluateDwell(t *Trigger, state *TriggerState, blobs []BlobPos, now time.Time) bool {
	if t.ConditionParams.DurationS == nil {
		return false
	}

	durationThreshold := time.Duration(*t.ConditionParams.DurationS) * time.Second

	var fired bool

	for _, blob := range blobs {
		blobState := state.Blobs[blob.ID]

		isInside := t.Shape.IsInside(Point3D{X: blob.X, Y: blob.Y, Z: blob.Z})

		if isInside {
			if !blobState.Inside {
				// Just entered — start dwell timer
				blobState.Inside = true
				blobState.EnterTime = now
				s.blobVolumes[blob.ID] = t.ID
			} else if blobState.EnterTime.IsZero() {
				// Inside but no enter time — was set to fired state, must leave and re-enter
				// Don't restart timer until blob exits and re-enters
			} else {
				// Was inside with active timer, check dwell time
				elapsed := now.Sub(blobState.EnterTime)
				if elapsed >= durationThreshold {
					fired = true
					// Mark as fired — enter time zeroed means "waiting for exit/re-entry"
					blobState.EnterTime = time.Time{}
				}
			}
		} else {
			if blobState.Inside {
				// Just left — reset for potential re-entry
				blobState.Inside = false
				blobState.EnterTime = time.Time{}
				delete(s.blobVolumes, blob.ID)
			}
		}

		blobState.LastCheckTime = now
	}

	return fired
}

// evaluateVacant triggers when the volume has been empty for N continuous seconds.
func (s *Store) evaluateVacant(t *Trigger, state *TriggerState, blobs []BlobPos, now time.Time) bool {
	if t.ConditionParams.DurationS == nil {
		// If no duration specified, trigger immediately when vacant
		for _, blob := range blobs {
			if t.Shape.IsInside(Point3D{X: blob.X, Y: blob.Y, Z: blob.Z}) {
				return false // Someone is inside
			}
		}
		return true
	}

	durationThreshold := time.Duration(*t.ConditionParams.DurationS) * time.Second

	// Check if any blob is inside
	anyInside := false
	for _, blob := range blobs {
		if t.Shape.IsInside(Point3D{X: blob.X, Y: blob.Y, Z: blob.Z}) {
			anyInside = true
			break
		}
	}

	if anyInside {
		// Reset vacant timer when someone enters
		state.VacantTimerStart = time.Time{}
		return false
	}

	// Check if vacant long enough
	if !state.VacantTimerStart.IsZero() && now.Sub(state.VacantTimerStart) >= durationThreshold {
		return true
	}

	// Start vacant timer
	if state.VacantTimerStart.IsZero() {
		state.VacantTimerStart = now
	}

	return false
}

// evaluateCount triggers when the blob count inside crosses a threshold.
// Fires on rising edge only: count was below threshold, now >= threshold.
func (s *Store) evaluateCount(t *Trigger, state *TriggerState, blobs []BlobPos, now time.Time) bool {
	if t.ConditionParams.CountThreshold == nil {
		return false
	}

	// Count blobs inside
	insideCount := 0
	for _, blob := range blobs {
		if t.Shape.IsInside(Point3D{X: blob.X, Y: blob.Y, Z: blob.Z}) {
			insideCount++
		}
	}

	threshold := *t.ConditionParams.CountThreshold

	// Get previous count from the special -999 slot (never persisted — in-memory only)
	prevCount := 0
	if prevSlot := state.Blobs[-999]; prevSlot != nil {
		prevCount = int(prevSlot.EnterTime.UnixNano())
	}

	// Check if we crossed the threshold from below to at/above (rising edge only)
	crossedThreshold := prevCount < threshold && insideCount >= threshold

	// Store current count for next evaluation (in-memory only, never persisted)
	if state.Blobs == nil {
		state.Blobs = make(map[int]*BlobState)
	}
	state.Blobs[-999] = &BlobState{
		BlobID:    -999, // Special in-memory-only slot for count storage
		EnterTime: time.Unix(0, int64(insideCount)),
		Inside:    false,
	}

	return crossedThreshold
}

// isTimeInRange checks if the current time is within the constraint window.
func (s *Store) isTimeInRange(tc *TimeConstraint, now time.Time) bool {
	if tc == nil {
		return true
	}

	fromMins := s.parseTimeOfDay(tc.From)
	toMins := s.parseTimeOfDay(tc.To)
	currentMins := now.Hour()*60 + now.Minute()

	if fromMins <= toMins {
		// Normal range (e.g., 09:00-17:00)
		return currentMins >= fromMins && currentMins <= toMins
	}
	// Range crosses midnight (e.g., 22:00-07:00)
	return currentMins >= fromMins || currentMins <= toMins
}

func (s *Store) parseTimeOfDay(timeStr string) int {
	// Parse HH:MM format
	var hours, minutes int
	for i, c := range timeStr {
		if c >= '0' && c <= '9' {
			if i < 2 {
				hours = hours*10 + int(c-'0')
			} else {
				minutes = minutes*10 + int(c-'0')
			}
		}
	}
	return hours*60 + minutes
}

// fireTrigger marks a trigger as fired, persists to database, and invokes the firing callback.
func (s *Store) fireTrigger(triggerID string, now time.Time) {
	t := s.triggers[triggerID]
	if t == nil {
		return
	}

	t.LastFired = &now
	s.triggerState[triggerID].LastFired = now

	// Persist to database
	lastFiredNs := now.UnixNano()
	s.db.Exec(`UPDATE triggers SET last_fired=? WHERE id=?`, lastFiredNs, triggerID)

	// Collect blob IDs that are inside this volume
	var blobIDs []int
	for blobID, volID := range s.blobVolumes {
		if volID == triggerID {
			blobIDs = append(blobIDs, blobID)
		}
	}

	// Invoke firing callback if set
	if s.onFired != nil {
		// Unlock before calling callback to avoid deadlock
		s.mu.Unlock()
		s.onFired(FiredEvent{
			TriggerID:   triggerID,
			TriggerName: t.Name,
			Condition:   t.Condition,
			BlobIDs:     blobIDs,
			Timestamp:   now,
		})
		s.mu.Lock()
	}
}

// persistBlobStates persists blob states to the trigger_state table.
// Skips in-memory-only slots (negative blob IDs used for count tracking).
func (s *Store) persistBlobStates(triggerID string, state *TriggerState) {
	// Delete existing states for this trigger
	s.db.Exec(`DELETE FROM trigger_state WHERE trigger_id=?`, triggerID)

	// Insert current states (skip in-memory-only slots with negative blob IDs)
	for blobID, blobState := range state.Blobs {
		if blobID < 0 {
			continue // In-memory-only, don't persist
		}
		inside := 0
		if blobState.Inside {
			inside = 1
		}
		enterTimeNs := blobState.EnterTime.UnixNano()
		lastCheckNs := blobState.LastCheckTime.UnixNano()

		s.db.Exec(`
			INSERT INTO trigger_state (trigger_id, blob_id, inside, enter_time, last_check)
			VALUES (?, ?, ?, ?, ?)
		`, triggerID, blobID, inside, enterTimeNs, lastCheckNs)
	}
}

// GetRecentFirings returns the last N trigger firings with details.
func (s *Store) GetRecentFirings(limit int) []FiringRecord {
	rows, err := s.db.Query(`
		SELECT t.id, t.name, t.condition, t.last_fired
		FROM triggers t
		WHERE t.last_fired > 0
		ORDER BY t.last_fired DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var records []FiringRecord
	for rows.Next() {
		var r FiringRecord
		var id int
		var lastFiredNs int64

		if err := rows.Scan(&id, &r.TriggerName, &r.Condition, &lastFiredNs); err != nil {
			continue
		}

		r.TriggerID = fmt.Sprintf("%d", id)
		r.FiredAt = time.Unix(0, lastFiredNs)
		records = append(records, r)
	}

	return records
}

// FiringRecord represents a trigger firing event.
type FiringRecord struct {
	TriggerID   string    `json:"trigger_id"`
	TriggerName string    `json:"trigger_name"`
	Condition   string    `json:"condition"`
	FiredAt     time.Time `json:"fired_at"`
	BlobID      int       `json:"blob_id,omitempty"` // Future: track which blob caused it
}

// WebhookLogEntry represents an entry in the webhook audit log.
type WebhookLogEntry struct {
	ID        int64     `json:"id"`
	TriggerID string    `json:"trigger_id"`
	URL       string    `json:"url"`
	FiredAtMs int64     `json:"fired_at_ms"`
	Status    int       `json:"status_code,omitempty"`
	LatencyMs int64     `json:"latency_ms"`
	Error     string    `json:"error,omitempty"`
}

// BlobPos represents a blob's position for trigger evaluation.
type BlobPos struct {
	ID int
	X, Y, Z float64
}

// IsInVolume is a convenience function to test if a point is in a trigger's volume.
func (s *Store) IsInVolume(triggerID string, x, y, z float64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	t := s.triggers[triggerID]
	if t == nil {
		return false
	}

	return t.Shape.IsInside(Point3D{X: x, Y: y, Z: z})
}

// GetTrigger retrieves a trigger by ID (without locking - for use in callbacks).
func (s *Store) GetTrigger(id string) *Trigger {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.triggers[id]
}

// DisableTriggerWithError disables a trigger and sets its error message.
// Called when a webhook returns a 4xx response.
func (s *Store) DisableTriggerWithError(triggerID, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, exists := s.triggers[triggerID]
	if !exists {
		return
	}

	t.Enabled = false
	t.ErrorMessage = errMsg

	s.db.Exec(`UPDATE triggers SET enabled=0, error_message=? WHERE id=?`, errMsg, triggerID)
	log.Printf("[WARN] Trigger %q disabled due to webhook error: %s", t.Name, errMsg)
}

// IncrementErrorCount increments the error count for a trigger.
// Called when a webhook returns a 5xx or times out.
func (s *Store) IncrementErrorCount(triggerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, exists := s.triggers[triggerID]
	if !exists {
		return
	}

	t.ErrorCount++
	s.db.Exec(`UPDATE triggers SET error_count=? WHERE id=?`, t.ErrorCount, triggerID)
	log.Printf("[WARN] Trigger %q webhook error count: %d", t.Name, t.ErrorCount)
}

// ResetErrorCount resets the error count for a trigger (on first 2xx).
func (s *Store) ResetErrorCount(triggerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, exists := s.triggers[triggerID]
	if !exists {
		return
	}

	t.ErrorCount = 0
	s.db.Exec(`UPDATE triggers SET error_count=0 WHERE id=?`, triggerID)
}

// EnableTrigger clears error state and re-enables a trigger.
func (s *Store) EnableTrigger(triggerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, exists := s.triggers[triggerID]
	if !exists {
		return fmt.Errorf("trigger not found: %s", triggerID)
	}

	t.Enabled = true
	t.ErrorMessage = ""
	t.ErrorCount = 0

	_, err := s.db.Exec(`UPDATE triggers SET enabled=1, error_message='', error_count=0 WHERE id=?`, triggerID)
	if err != nil {
		return fmt.Errorf("enable trigger: %w", err)
	}

	log.Printf("[INFO] Trigger %q re-enabled", t.Name)
	return nil
}

// WriteWebhookLog writes an entry to the webhook_log audit table.
func (s *Store) WriteWebhookLog(triggerID string, url string, firedAtMs int64, statusCode int, latencyMs int64, errMsg string) {
	s.db.Exec(`
		INSERT INTO webhook_log (trigger_id, fired_at_ms, url, status_code, latency_ms, error)
		VALUES (?, ?, ?, ?, ?, ?)
	`, triggerID, firedAtMs, url, statusCode, latencyMs, errMsg)
}

// GetWebhookLog returns the last N webhook log entries for a specific trigger.
func (s *Store) GetWebhookLog(triggerID string, limit int) []WebhookLogEntry {
	rows, err := s.db.Query(`
		SELECT id, trigger_id, fired_at_ms, url, status_code, latency_ms, COALESCE(error, '')
		FROM webhook_log
		WHERE trigger_id = ?
		ORDER BY fired_at_ms DESC
		LIMIT ?
	`, triggerID, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var entries []WebhookLogEntry
	for rows.Next() {
		var e WebhookLogEntry
		if err := rows.Scan(&e.ID, &e.TriggerID, &e.FiredAtMs, &e.URL, &e.Status, &e.LatencyMs, &e.Error); err != nil {
			continue
		}
		entries = append(entries, e)
	}

	return entries
}
