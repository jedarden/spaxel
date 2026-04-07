// Package automation provides spatial automation with 3D trigger volumes.
package automation

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// TriggerType defines the type of trigger for an automation.
type TriggerType string

const (
	TriggerZoneEnter         TriggerType = "zone_enter"
	TriggerZoneLeave         TriggerType = "zone_leave"
	TriggerZoneDwell         TriggerType = "zone_dwell"
	TriggerZoneVacant        TriggerType = "zone_vacant"
	TriggerPersonCountChange TriggerType = "person_count_change"
	TriggerFallDetected      TriggerType = "fall_detected"
	TriggerAnomaly           TriggerType = "anomaly"
	TriggerBLEDevicePresent  TriggerType = "ble_device_present"
	TriggerBLEDeviceAbsent   TriggerType = "ble_device_absent"
	TriggerVolumeEnter       TriggerType = "volume_enter"
	TriggerVolumeLeave       TriggerType = "volume_leave"
	TriggerPredictedZoneEnter TriggerType = "predicted_zone_enter" // N minutes before predicted zone entry
)

// SystemMode represents the system operating mode.
type SystemMode string

const (
	ModeHome  SystemMode = "home"
	ModeAway  SystemMode = "away"
	ModeSleep SystemMode = "sleep"
)

// TriggerVolume represents a 3D region that can trigger automations.
type TriggerVolume struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Type      string  `json:"type"` // box, sphere, cylinder
	Enabled   bool    `json:"enabled"`
	AutomationID string `json:"automation_id,omitempty"`
	// Box type
	MinX float64 `json:"min_x,omitempty"`
	MinY float64 `json:"min_y,omitempty"`
	MinZ float64 `json:"min_z,omitempty"`
	MaxX float64 `json:"max_x,omitempty"`
	MaxY float64 `json:"max_y,omitempty"`
	MaxZ float64 `json:"max_z,omitempty"`
	// Sphere type
	CenterX float64 `json:"center_x,omitempty"`
	CenterY float64 `json:"center_y,omitempty"`
	CenterZ float64 `json:"center_z,omitempty"`
	Radius   float64 `json:"radius,omitempty"`
	// Cylinder type
	BaseX float64 `json:"base_x,omitempty"`
	BaseZ float64 `json:"base_z,omitempty"`
	BaseRadius      float64 `json:"base_radius,omitempty"`
	MinHeight float64 `json:"min_height,omitempty"`
	MaxHeight float64 `json:"max_height,omitempty"`
}

// TriggerConfig holds configuration for the trigger.
type TriggerConfig struct {
	ZoneID         string `json:"zone_id,omitempty"`         // For zone triggers
	VolumeID       string `json:"volume_id,omitempty"`       // For volume triggers
	PersonID       string `json:"person_id,omitempty"`       // "anyone" or specific person ID
	DeviceMAC      string `json:"device_mac,omitempty"`      // For BLE device triggers
	DwellMinutes   int    `json:"dwell_minutes,omitempty"`   // For zone_dwell
	AbsentMinutes  int    `json:"absent_minutes,omitempty"`  // For ble_device_absent
	CountThreshold int    `json:"count_threshold,omitempty"` // For person_count_change
	CountDirection string `json:"count_direction,omitempty"` // "up" or "down" for person_count_change
	AnyZone        bool   `json:"any_zone,omitempty"`        // Match any zone
	MinutesAhead   int    `json:"minutes_ahead,omitempty"`   // For predicted_zone_enter: trigger N minutes before predicted entry
}

// ConditionType defines types of conditions.
type ConditionType string

const (
	ConditionPersonFilter  ConditionType = "person_filter"
	ConditionTimeWindow    ConditionType = "time_window"
	ConditionDayOfWeek     ConditionType = "day_of_week"
	ConditionSystemMode    ConditionType = "system_mode"
	ConditionZoneOccupancy ConditionType = "zone_occupancy"
)

// Condition represents a condition for automation triggering.
type Condition struct {
	Type     ConditionType `json:"type"`
	Value    string        `json:"value"`              // Parsed based on type
	Operator string        `json:"operator,omitempty"` // eq, neq, gt, lt, gte, lte
}

// ActionType defines types of actions.
type ActionType string

const (
	ActionWebhook     ActionType = "webhook"
	ActionMQTT        ActionType = "mqtt_publish"
	ActionNtfy        ActionType = "ntfy"
	ActionPushover    ActionType = "pushover"
)

// Action represents an action to execute when triggered.
type Action struct {
	Type        ActionType            `json:"type"`
	URL         string                `json:"url,omitempty"`      // for webhook
	Topic       string                `json:"topic,omitempty"`    // for mqtt
	Server      string                `json:"server,omitempty"`   // for ntfy
	Token       string                `json:"token,omitempty"`    // for pushover
	UserKey     string                `json:"user_key,omitempty"` // for pushover
	Template    string                `json:"template,omitempty"` // payload template
	Headers     map[string]string     `json:"headers,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// Automation represents an automation rule.
type Automation struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Enabled       bool           `json:"enabled"`
	TriggerType   TriggerType    `json:"trigger_type"`
	TriggerConfig TriggerConfig  `json:"trigger_config"`
	Conditions    []Condition    `json:"conditions,omitempty"`
	Actions       []Action       `json:"actions"`
	Cooldown      int            `json:"cooldown"` // seconds between triggers
	LastFired     time.Time      `json:"last_fired"`
	FireCount     int            `json:"fire_count"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

// ActionResult represents the result of an action execution.
type ActionResult struct {
	ActionType ActionType `json:"action_type"`
	Success    bool       `json:"success"`
	Error      string     `json:"error,omitempty"`
	Duration   int64      `json:"duration_ms"`
}

// TriggerEventData contains data about the trigger event.
type TriggerEventData struct {
	AutomationID   string                 `json:"automation_id"`
	AutomationName string                 `json:"automation_name"`
	TriggerType    TriggerType            `json:"trigger_type"`
	PersonID       string                 `json:"person_id,omitempty"`
	PersonName     string                 `json:"person_name,omitempty"`
	PersonColor    string                 `json:"person_color,omitempty"`
	ZoneID         string                 `json:"zone_id,omitempty"`
	ZoneName       string                 `json:"zone_name,omitempty"`
	FromZone       string                 `json:"from_zone,omitempty"`
	ToZone         string                 `json:"to_zone,omitempty"`
	DeviceMAC      string                 `json:"device_mac,omitempty"`
	DeviceName     string                 `json:"device_name,omitempty"`
	OccupantCount  int                    `json:"occupant_count,omitempty"`
	EventTimestamp time.Time              `json:"timestamp"`
	Confidence     float64                `json:"confidence,omitempty"`
	TestMode       bool                   `json:"test_mode,omitempty"`
	Extra          map[string]interface{} `json:"extra,omitempty"`
}

// Event represents an internal event that can trigger automations.
type Event struct {
	Type         TriggerType
	Timestamp    time.Time
	PersonID     string
	PersonName   string
	PersonColor  string
	ZoneID       string
	ZoneName     string
	FromZone     string
	ToZone       string
	DeviceMAC    string
	DeviceName   string
	OccupantCount int
	Confidence   float64
	Extra        map[string]interface{}
}

// ZoneInfoProvider provides zone information.
type ZoneInfoProvider interface {
	GetZone(id string) (name string, ok bool)
	GetZoneOccupancy(zoneID string) (count int, blobIDs []int)
}

// PersonInfoProvider provides person information.
type PersonInfoProvider interface {
	GetPerson(id string) (name, color string, ok bool)
}

// DeviceInfoProvider provides BLE device information.
type DeviceInfoProvider interface {
	GetDevice(mac string) (name string, ok bool)
}

// MQTTClient interface for MQTT publishing.
type MQTTClient interface {
	Publish(topic string, payload []byte) error
	IsConnected() bool
}

// NotificationSender interface for sending notifications.
type NotificationSender interface {
	SendViaChannel(channelType string, title, body string, data map[string]interface{}) error
}

// PredictionProvider provides prediction information for predicted_zone_enter triggers.
type PredictionProvider interface {
	GetPredictions() []PredictionInfo
}

// PredictionInfo represents a prediction for use by automations.
type PredictionInfo struct {
	PersonID                   string
	PersonName                 string
	CurrentZoneID              string
	CurrentZoneName            string
	PredictedNextZoneID        string
	PredictedNextZoneName      string
	PredictionConfidence       float64
	EstimatedTransitionMinutes float64
	DataConfidence             string
}

// Engine manages automation rules and triggers.
type Engine struct {
	mu       sync.RWMutex
	db       *sql.DB

	automations   map[string]*Automation
	volumes       map[string]*TriggerVolume
	cooldowns     map[string]time.Time // automationID -> last trigger time

	// Zone dwell tracking
	zoneEnterTime map[string]map[int]time.Time // zoneID -> blobID -> enter time

	// BLE device last seen tracking
	deviceLastSeen map[string]time.Time // deviceMAC -> last seen time

	// System mode
	systemMode SystemMode

	// Providers
	zoneProvider    ZoneInfoProvider
	personProvider  PersonInfoProvider
	deviceProvider  DeviceInfoProvider
	predictionProvider PredictionProvider

	// Clients
	httpClient       *http.Client
	mqttClient       MQTTClient
	notifySender     NotificationSender

	// Callbacks
	onTrigger func(TriggerEventData)
}

// NewEngine creates a new automation engine.
func NewEngine(dbPath string) (*Engine, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	e := &Engine{
		db:              db,
		automations:     make(map[string]*Automation),
		volumes:         make(map[string]*TriggerVolume),
		cooldowns:       make(map[string]time.Time),
		zoneEnterTime:   make(map[string]map[int]time.Time),
		deviceLastSeen:  make(map[string]time.Time),
		systemMode:      ModeHome,
		httpClient:      &http.Client{Timeout: 10 * time.Second},
	}

	if err := e.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	if err := e.loadAutomations(); err != nil {
		log.Printf("[WARN] Failed to load automations: %v", err)
	}

	if err := e.loadVolumes(); err != nil {
		log.Printf("[WARN] Failed to load volumes: %v", err)
	}

	return e, nil
}

func (e *Engine) migrate() error {
	_, err := e.db.Exec(`
		CREATE TABLE IF NOT EXISTS automations (
			id            TEXT PRIMARY KEY,
			name          TEXT    NOT NULL,
			enabled       INTEGER NOT NULL DEFAULT 1,
			trigger_type  TEXT    NOT NULL,
			trigger_config TEXT   NOT NULL DEFAULT '{}',
			conditions    TEXT    NOT NULL DEFAULT '[]',
			actions       TEXT    NOT NULL DEFAULT '[]',
			cooldown      INTEGER NOT NULL DEFAULT 60,
			last_fired    INTEGER NOT NULL DEFAULT 0,
			fire_count    INTEGER NOT NULL DEFAULT 0,
			created_at    INTEGER NOT NULL DEFAULT 0,
			updated_at    INTEGER NOT NULL DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS trigger_volumes (
			id            TEXT PRIMARY KEY,
			automation_id TEXT,
			name          TEXT    NOT NULL,
			type          TEXT    NOT NULL DEFAULT 'box',
			enabled       INTEGER NOT NULL DEFAULT 1,
			min_x         REAL    NOT NULL DEFAULT 0,
			min_y         REAL    NOT NULL DEFAULT 0,
			min_z         REAL    NOT NULL DEFAULT 0,
			max_x         REAL    NOT NULL DEFAULT 1,
			max_y         REAL    NOT NULL DEFAULT 1,
			max_z         REAL    NOT NULL DEFAULT 1,
			center_x      REAL    NOT NULL DEFAULT 0,
			center_y      REAL    NOT NULL DEFAULT 0,
			center_z      REAL    NOT NULL DEFAULT 0,
			radius        REAL    NOT NULL DEFAULT 1,
			base_x        REAL    NOT NULL DEFAULT 0,
			base_z        REAL    NOT NULL DEFAULT 0,
			base_radius   REAL    NOT NULL DEFAULT 1,
			min_height    REAL    NOT NULL DEFAULT 0,
			max_height    REAL    NOT NULL DEFAULT 2,
			FOREIGN KEY (automation_id) REFERENCES automations(id) ON DELETE CASCADE
		);

		CREATE TABLE IF NOT EXISTS action_log (
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			automation_id  TEXT    NOT NULL,
			fired_at       INTEGER NOT NULL,
			event_json     TEXT    NOT NULL,
			actions_results_json TEXT NOT NULL DEFAULT '[]'
		);

		CREATE TABLE IF NOT EXISTS system_state (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);

		INSERT OR IGNORE INTO system_state (key, value) VALUES ('mode', 'home');

		CREATE INDEX IF NOT EXISTS idx_action_log_time ON action_log(fired_at);
		CREATE INDEX IF NOT EXISTS idx_action_log_automation ON action_log(automation_id);
	`)
	return err
}

func (e *Engine) loadAutomations() error {
	rows, err := e.db.Query(`
		SELECT id, name, enabled, trigger_type, trigger_config, conditions, actions,
		       cooldown, last_fired, fire_count, created_at, updated_at
		FROM automations
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		a := &Automation{}
		var enabled int
		var triggerConfigJSON, conditionsJSON, actionsJSON string
		var lastFiredNS, createdAtNS, updatedAtNS int64

		if err := rows.Scan(&a.ID, &a.Name, &enabled, &a.TriggerType, &triggerConfigJSON,
			&conditionsJSON, &actionsJSON, &a.Cooldown, &lastFiredNS, &a.FireCount,
			&createdAtNS, &updatedAtNS); err != nil {
			log.Printf("[WARN] Failed to scan automation: %v", err)
			continue
		}

		a.Enabled = enabled != 0

		if err := json.Unmarshal([]byte(triggerConfigJSON), &a.TriggerConfig); err != nil {
			log.Printf("[WARN] Failed to parse trigger config for %s: %v", a.ID, err)
			continue
		}

		if err := json.Unmarshal([]byte(conditionsJSON), &a.Conditions); err != nil {
			log.Printf("[WARN] Failed to parse conditions for %s: %v", a.ID, err)
			continue
		}

		if err := json.Unmarshal([]byte(actionsJSON), &a.Actions); err != nil {
			log.Printf("[WARN] Failed to parse actions for %s: %v", a.ID, err)
			continue
		}

		if lastFiredNS > 0 {
			a.LastFired = time.Unix(0, lastFiredNS)
		}
		if createdAtNS > 0 {
			a.CreatedAt = time.Unix(0, createdAtNS)
		}
		if updatedAtNS > 0 {
			a.UpdatedAt = time.Unix(0, updatedAtNS)
		}

		e.automations[a.ID] = a
	}

	return nil
}

func (e *Engine) loadVolumes() error {
	rows, err := e.db.Query(`
		SELECT id, automation_id, name, type, enabled,
		       min_x, min_y, min_z, max_x, max_y, max_z,
		       center_x, center_y, center_z, radius,
		       base_x, base_z, base_radius, min_height, max_height
		FROM trigger_volumes
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		v := &TriggerVolume{}
		var enabled int

		if err := rows.Scan(&v.ID, &v.AutomationID, &v.Name, &v.Type, &enabled,
			&v.MinX, &v.MinY, &v.MinZ, &v.MaxX, &v.MaxY, &v.MaxZ,
			&v.CenterX, &v.CenterY, &v.CenterZ, &v.Radius,
			&v.BaseX, &v.BaseZ, &v.BaseRadius, &v.MinHeight, &v.MaxHeight); err != nil {
			log.Printf("[WARN] Failed to scan volume: %v", err)
			continue
		}

		v.Enabled = enabled != 0
		e.volumes[v.ID] = v
	}

	return nil
}

// Close closes the database.
func (e *Engine) Close() error {
	return e.db.Close()
}

// SetZoneProvider sets the zone info provider.
func (e *Engine) SetZoneProvider(p ZoneInfoProvider) {
	e.mu.Lock()
	e.zoneProvider = p
	e.mu.Unlock()
}

// SetPersonProvider sets the person info provider.
func (e *Engine) SetPersonProvider(p PersonInfoProvider) {
	e.mu.Lock()
	e.personProvider = p
	e.mu.Unlock()
}

// SetDeviceProvider sets the device info provider.
func (e *Engine) SetDeviceProvider(p DeviceInfoProvider) {
	e.mu.Lock()
	e.deviceProvider = p
	e.mu.Unlock()
}

// SetMQTTClient sets the MQTT client.
func (e *Engine) SetMQTTClient(client MQTTClient) {
	e.mu.Lock()
	e.mqttClient = client
	e.mu.Unlock()
}

// SetNotificationSender sets the notification sender.
func (e *Engine) SetNotificationSender(sender NotificationSender) {
	e.mu.Lock()
	e.notifySender = sender
	e.mu.Unlock()
}

// SetOnTrigger sets callback for trigger events.
func (e *Engine) SetOnTrigger(cb func(TriggerEventData)) {
	e.mu.Lock()
	e.onTrigger = cb
	e.mu.Unlock()
}

// GetSystemMode returns the current system mode.
func (e *Engine) GetSystemMode() SystemMode {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.systemMode
}

// SetSystemMode sets the system mode.
func (e *Engine) SetSystemMode(mode SystemMode) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	_, err := e.db.Exec(`UPDATE system_state SET value = ? WHERE key = 'mode'`, string(mode))
	if err != nil {
		return err
	}

	e.systemMode = mode
	log.Printf("[INFO] System mode changed to: %s", mode)
	return nil
}

// CreateAutomation creates a new automation.
func (e *Engine) CreateAutomation(a *Automation) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	triggerConfigJSON, _ := json.Marshal(a.TriggerConfig)
	conditionsJSON, _ := json.Marshal(a.Conditions)
	actionsJSON, _ := json.Marshal(a.Actions)

	now := time.Now().UnixNano()
	_, err := e.db.Exec(`
		INSERT INTO automations (id, name, enabled, trigger_type, trigger_config, conditions, actions,
		                         cooldown, last_fired, fire_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, 0, ?, ?)
	`, a.ID, a.Name, a.Enabled, a.TriggerType, string(triggerConfigJSON),
		string(conditionsJSON), string(actionsJSON), a.Cooldown, now, now)
	if err != nil {
		return err
	}

	a.CreatedAt = time.Unix(0, now)
	a.UpdatedAt = time.Unix(0, now)
	a.FireCount = 0
	e.automations[a.ID] = a
	return nil
}

// UpdateAutomation updates an existing automation.
func (e *Engine) UpdateAutomation(a *Automation) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	triggerConfigJSON, _ := json.Marshal(a.TriggerConfig)
	conditionsJSON, _ := json.Marshal(a.Conditions)
	actionsJSON, _ := json.Marshal(a.Actions)

	now := time.Now().UnixNano()
	_, err := e.db.Exec(`
		UPDATE automations SET name=?, enabled=?, trigger_type=?, trigger_config=?, conditions=?, actions=?,
		                       cooldown=?, updated_at=?
		WHERE id=?
	`, a.Name, a.Enabled, a.TriggerType, string(triggerConfigJSON),
		string(conditionsJSON), string(actionsJSON), a.Cooldown, now, a.ID)
	if err != nil {
		return err
	}

	a.UpdatedAt = time.Unix(0, now)
	e.automations[a.ID] = a
	return nil
}

// DeleteAutomation deletes an automation.
func (e *Engine) DeleteAutomation(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	_, err := e.db.Exec(`DELETE FROM automations WHERE id=?`, id)
	if err != nil {
		return err
	}

	delete(e.automations, id)
	delete(e.cooldowns, id)
	return nil
}

// GetAutomation returns an automation by ID.
func (e *Engine) GetAutomation(id string) *Automation {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.automations[id]
}

// GetAllAutomations returns all automations.
func (e *Engine) GetAllAutomations() []*Automation {
	e.mu.RLock()
	defer e.mu.RUnlock()

	automations := make([]*Automation, 0, len(e.automations))
	for _, a := range e.automations {
		automations = append(automations, a)
	}
	return automations
}

// CreateTriggerVolume creates a new trigger volume.
func (e *Engine) CreateTriggerVolume(v *TriggerVolume) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	_, err := e.db.Exec(`
		INSERT INTO trigger_volumes (id, automation_id, name, type, enabled,
		                             min_x, min_y, min_z, max_x, max_y, max_z,
		                             center_x, center_y, center_z, radius,
		                             base_x, base_z, base_radius, min_height, max_height)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, v.ID, v.AutomationID, v.Name, v.Type, v.Enabled,
		v.MinX, v.MinY, v.MinZ, v.MaxX, v.MaxY, v.MaxZ,
		v.CenterX, v.CenterY, v.CenterZ, v.Radius,
		v.BaseX, v.BaseZ, v.BaseRadius, v.MinHeight, v.MaxHeight)
	if err != nil {
		return err
	}

	e.volumes[v.ID] = v
	return nil
}

// DeleteTriggerVolume deletes a trigger volume.
func (e *Engine) DeleteTriggerVolume(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	_, err := e.db.Exec(`DELETE FROM trigger_volumes WHERE id=?`, id)
	if err != nil {
		return err
	}

	delete(e.volumes, id)
	return nil
}

// GetTriggerVolume returns a trigger volume by ID.
func (e *Engine) GetTriggerVolume(id string) *TriggerVolume {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.volumes[id]
}

// GetAllTriggerVolumes returns all trigger volumes.
func (e *Engine) GetAllTriggerVolumes() []*TriggerVolume {
	e.mu.RLock()
	defer e.mu.RUnlock()

	volumes := make([]*TriggerVolume, 0, len(e.volumes))
	for _, v := range e.volumes {
		volumes = append(volumes, v)
	}
	return volumes
}

// ProcessEvent processes an event and triggers matching automations.
func (e *Engine) ProcessEvent(event Event) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	now := time.Now()

	for _, a := range e.automations {
		if !a.Enabled {
			continue
		}

		// Check trigger type match
		if !e.triggerMatches(a, event) {
			continue
		}

		// Check cooldown
		if lastTrigger, exists := e.cooldowns[a.ID]; exists {
			if now.Sub(lastTrigger).Seconds() < float64(a.Cooldown) {
				continue
			}
		}

		// Check conditions
		if !e.evaluateConditions(a.Conditions, event, now) {
			continue
		}

		// All conditions met - trigger the automation
		e.triggerAutomation(a, event, now, false)
	}
}

// triggerMatches checks if an automation's trigger matches the event.
func (e *Engine) triggerMatches(a *Automation, event Event) bool {
	if a.TriggerType != event.Type {
		return false
	}

	cfg := a.TriggerConfig

	switch a.TriggerType {
	case TriggerZoneEnter, TriggerZoneLeave, TriggerZoneDwell, TriggerZoneVacant:
		if !cfg.AnyZone && cfg.ZoneID != "" && cfg.ZoneID != event.ZoneID {
			return false
		}
		if cfg.PersonID != "" && cfg.PersonID != "anyone" && cfg.PersonID != event.PersonID {
			return false
		}

	case TriggerPersonCountChange:
		if !cfg.AnyZone && cfg.ZoneID != "" && cfg.ZoneID != event.ZoneID {
			return false
		}

	case TriggerFallDetected:
		if cfg.PersonID != "" && cfg.PersonID != "anyone" && cfg.PersonID != event.PersonID {
			return false
		}
		if !cfg.AnyZone && cfg.ZoneID != "" && cfg.ZoneID != event.ZoneID {
			return false
		}

	case TriggerAnomaly:
		// Anomaly events match any anomaly trigger

	case TriggerBLEDevicePresent:
		if cfg.DeviceMAC != "" && cfg.DeviceMAC != event.DeviceMAC {
			return false
		}

	case TriggerBLEDeviceAbsent:
		if cfg.DeviceMAC != "" && cfg.DeviceMAC != event.DeviceMAC {
			return false
		}

	case TriggerPredictedZoneEnter:
		if !cfg.AnyZone && cfg.ZoneID != "" && cfg.ZoneID != event.ZoneID {
			return false
		}
		if cfg.PersonID != "" && cfg.PersonID != "anyone" && cfg.PersonID != event.PersonID {
			return false
		}
	}

	return true
}

// evaluateConditions checks if all conditions are met.
func (e *Engine) evaluateConditions(conditions []Condition, event Event, now time.Time) bool {
	for _, cond := range conditions {
		switch cond.Type {
		case ConditionPersonFilter:
			if cond.Value != "anyone" && cond.Value != event.PersonID {
				return false
			}

		case ConditionTimeWindow:
			if !e.isTimeInRange(cond.Value, now) {
				return false
			}

		case ConditionDayOfWeek:
			if !e.isDayOfWeek(cond.Value, now) {
				return false
			}

		case ConditionSystemMode:
			if cond.Value != string(e.systemMode) {
				return false
			}

		case ConditionZoneOccupancy:
			// Format: "zone_id:operator:count" e.g., "living_room:lt:1"
			if !e.checkZoneOccupancy(cond.Value) {
				return false
			}
		}
	}
	return true
}

// isTimeInRange checks if time is within a range (format: "HH:MM-HH:MM").
func (e *Engine) isTimeInRange(rangeStr string, now time.Time) bool {
	parts := strings.Split(rangeStr, "-")
	if len(parts) != 2 {
		return false
	}

	startMins := parseTimeOfDay(parts[0])
	endMins := parseTimeOfDay(parts[1])
	currentMins := now.Hour()*60 + now.Minute()

	if startMins <= endMins {
		// Normal range (e.g., 09:00-17:00)
		return currentMins >= startMins && currentMins <= endMins
	}
	// Range crosses midnight (e.g., 22:00-07:00)
	return currentMins >= startMins || currentMins <= endMins
}

func parseTimeOfDay(s string) int {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 2 {
		return 0
	}
	hours := parseInt(parts[0])
	mins := parseInt(parts[1])
	return hours*60 + mins
}

func parseInt(s string) int {
	var n int
	for _, c := range strings.TrimSpace(s) {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}

// isDayOfWeek checks if the current day matches (format: bitmask or comma-separated).
func (e *Engine) isDayOfWeek(value string, now time.Time) bool {
	weekday := int(now.Weekday()) // 0=Sunday, 1=Monday, ...

	// Try bitmask format first
	if len(value) <= 7 {
		bitmask := parseInt(value)
		if bitmask > 0 {
			return (bitmask&(1<<weekday)) != 0
		}
	}

	// Try comma-separated format (0=Sun, 1=Mon, ..., 6=Sat)
	days := strings.Split(value, ",")
	for _, d := range days {
		if parseInt(strings.TrimSpace(d)) == weekday {
			return true
		}
	}
	return false
}

// checkZoneOccupancy checks zone occupancy condition.
func (e *Engine) checkZoneOccupancy(value string) bool {
	// Format: "zone_id:operator:count"
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return false
	}

	zoneID := parts[0]
	operator := parts[1]
	threshold := parseInt(parts[2])

	if e.zoneProvider == nil {
		return false
	}

	count, _ := e.zoneProvider.GetZoneOccupancy(zoneID)

	switch operator {
	case "eq":
		return count == threshold
	case "neq":
		return count != threshold
	case "lt":
		return count < threshold
	case "lte":
		return count <= threshold
	case "gt":
		return count > threshold
	case "gte":
		return count >= threshold
	}
	return false
}

// triggerAutomation executes an automation's actions.
func (e *Engine) triggerAutomation(a *Automation, event Event, now time.Time, testMode bool) {
	// Update cooldown
	e.cooldowns[a.ID] = now

	// Update fire count
	a.FireCount++
	a.LastFired = now
	e.db.Exec(`UPDATE automations SET last_fired=?, fire_count=? WHERE id=?`,
		now.UnixNano(), a.FireCount, a.ID)

	// Build event data
	eventData := e.buildEventData(a, event, testMode)

	// Execute actions
	var results []ActionResult
	for _, action := range a.Actions {
		result := e.executeAction(action, eventData)
		results = append(results, result)
	}

	// Log action results
	e.logActionResults(a.ID, eventData, results)

	// Fire callback
	if e.onTrigger != nil {
		go e.onTrigger(eventData)
	}

	log.Printf("[INFO] Automation triggered: %s (type=%s, test=%v)", a.Name, a.TriggerType, testMode)
}

func (e *Engine) buildEventData(a *Automation, event Event, testMode bool) TriggerEventData {
	data := TriggerEventData{
		AutomationID:   a.ID,
		AutomationName: a.Name,
		TriggerType:    a.TriggerType,
		PersonID:       event.PersonID,
		PersonName:     event.PersonName,
		PersonColor:    event.PersonColor,
		ZoneID:         event.ZoneID,
		ZoneName:       event.ZoneName,
		FromZone:       event.FromZone,
		ToZone:         event.ToZone,
		DeviceMAC:      event.DeviceMAC,
		DeviceName:     event.DeviceName,
		OccupantCount:  event.OccupantCount,
		EventTimestamp: event.Timestamp,
		Confidence:     event.Confidence,
		TestMode:       testMode,
		Extra:          event.Extra,
	}

	// Fill in person info if not provided
	if data.PersonName == "" && data.PersonID != "" && e.personProvider != nil {
		data.PersonName, data.PersonColor, _ = e.personProvider.GetPerson(data.PersonID)
	}

	// Fill in zone name if not provided
	if data.ZoneName == "" && data.ZoneID != "" && e.zoneProvider != nil {
		data.ZoneName, _ = e.zoneProvider.GetZone(data.ZoneID)
	}

	// Fill in device name if not provided
	if data.DeviceName == "" && data.DeviceMAC != "" && e.deviceProvider != nil {
		data.DeviceName, _ = e.deviceProvider.GetDevice(data.DeviceMAC)
	}

	return data
}

func (e *Engine) logActionResults(automationID string, eventData TriggerEventData, results []ActionResult) {
	eventJSON, _ := json.Marshal(eventData)
	resultsJSON, _ := json.Marshal(results)

	_, err := e.db.Exec(`
		INSERT INTO action_log (automation_id, fired_at, event_json, actions_results_json)
		VALUES (?, ?, ?, ?)
	`, automationID, eventData.EventTimestamp.UnixNano(), string(eventJSON), string(resultsJSON))
	if err != nil {
		log.Printf("[WARN] Failed to log action results: %v", err)
	}
}

// executeAction executes a single action.
func (e *Engine) executeAction(action Action, eventData TriggerEventData) ActionResult {
	start := time.Now()
	result := ActionResult{ActionType: action.Type, Success: false}

	payload := e.renderTemplate(action.Template, eventData)
	payloadBytes := []byte(payload)

	switch action.Type {
	case ActionWebhook:
		err := e.executeWebhook(action, payloadBytes, &result)
		if err != nil {
			result.Error = err.Error()
			// Retry once after 30s for 5xx errors
			if strings.Contains(err.Error(), "5") {
				time.AfterFunc(30*time.Second, func() {
					retryResult := ActionResult{ActionType: action.Type}
					retryErr := e.executeWebhook(action, payloadBytes, &retryResult)
					if retryErr != nil {
						log.Printf("[WARN] Webhook retry failed: %v", retryErr)
					} else {
						log.Printf("[INFO] Webhook retry succeeded")
					}
				})
			}
		} else {
			result.Success = true
		}

	case ActionMQTT:
		err := e.executeMQTT(action, payloadBytes)
		if err != nil {
			result.Error = err.Error()
		} else {
			result.Success = true
		}

	case ActionNtfy, ActionPushover:
		err := e.executeNotification(action, eventData)
		if err != nil {
			result.Error = err.Error()
		} else {
			result.Success = true
		}

	default:
		result.Error = fmt.Sprintf("unknown action type: %s", action.Type)
	}

	result.Duration = time.Since(start).Milliseconds()
	return result
}

// renderTemplate renders a payload template with event data variables.
func (e *Engine) renderTemplate(template string, data TriggerEventData) string {
	if template == "" {
		// Default JSON payload
		payload, _ := json.Marshal(map[string]interface{}{
			"automation_id":   data.AutomationID,
			"automation_name": data.AutomationName,
			"trigger_type":    data.TriggerType,
			"person_name":     data.PersonName,
			"person_color":    data.PersonColor,
			"zone_name":       data.ZoneName,
			"from_zone":       data.FromZone,
			"to_zone":         data.ToZone,
			"timestamp":       data.EventTimestamp.Format(time.RFC3339),
			"occupant_count":  data.OccupantCount,
			"confidence":      data.Confidence,
			"test_mode":       data.TestMode,
		})
		return string(payload)
	}

	// Simple variable substitution
	result := template
	result = strings.ReplaceAll(result, "{{person_name}}", data.PersonName)
	result = strings.ReplaceAll(result, "{{zone_name}}", data.ZoneName)
	result = strings.ReplaceAll(result, "{{from_zone}}", data.FromZone)
	result = strings.ReplaceAll(result, "{{to_zone}}", data.ToZone)
	result = strings.ReplaceAll(result, "{{timestamp}}", data.EventTimestamp.Format(time.RFC3339))
	result = strings.ReplaceAll(result, "{{occupant_count}}", fmt.Sprintf("%d", data.OccupantCount))
	result = strings.ReplaceAll(result, "{{event_type}}", string(data.TriggerType))
	result = strings.ReplaceAll(result, "{{person_color}}", data.PersonColor)
	result = strings.ReplaceAll(result, "{{confidence}}", fmt.Sprintf("%.2f", data.Confidence))

	return result
}

func (e *Engine) executeWebhook(action Action, payload []byte, result *ActionResult) error {
	req, err := http.NewRequest("POST", action.URL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range action.Headers {
		req.Header.Set(k, v)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return fmt.Errorf("server error: status %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("client error: status %d", resp.StatusCode)
	}

	log.Printf("[INFO] Webhook delivered to %s (status %d)", action.URL, resp.StatusCode)
	return nil
}

func (e *Engine) executeMQTT(action Action, payload []byte) error {
	e.mu.RLock()
	client := e.mqttClient
	e.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("MQTT client not configured")
	}

	if !client.IsConnected() {
		return fmt.Errorf("MQTT client not connected")
	}

	if err := client.Publish(action.Topic, payload); err != nil {
		return fmt.Errorf("publish failed: %w", err)
	}

	log.Printf("[INFO] MQTT published to %s", action.Topic)
	return nil
}

func (e *Engine) executeNotification(action Action, data TriggerEventData) error {
	if e.notifySender == nil {
		return fmt.Errorf("notification sender not configured")
	}

	title := fmt.Sprintf("Spaxel: %s", data.AutomationName)
	body := e.renderTemplate(action.Template, data)
	if body == action.Template {
		// Template wasn't rendered, create default body
		body = fmt.Sprintf("%s triggered: %s", data.TriggerType, data.PersonName)
		if data.ZoneName != "" {
			body += fmt.Sprintf(" in %s", data.ZoneName)
		}
	}

	channelType := string(action.Type)
	return e.notifySender.SendViaChannel(channelType, title, body, map[string]interface{}{
		"automation_id": data.AutomationID,
		"trigger_type":  data.TriggerType,
		"person_name":   data.PersonName,
		"zone_name":     data.ZoneName,
		"test_mode":     data.TestMode,
	})
}

// TestFire simulates a trigger event for testing.
func (e *Engine) TestFire(automationID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	a, exists := e.automations[automationID]
	if !exists {
		return fmt.Errorf("automation not found: %s", automationID)
	}

	// Create a simulated event
	event := Event{
		Type:       a.TriggerType,
		Timestamp:  time.Now(),
		PersonID:   a.TriggerConfig.PersonID,
		ZoneID:     a.TriggerConfig.ZoneID,
		DeviceMAC:  a.TriggerConfig.DeviceMAC,
	}

	// Get provider info
	if e.personProvider != nil && event.PersonID != "" && event.PersonID != "anyone" {
		event.PersonName, event.PersonColor, _ = e.personProvider.GetPerson(event.PersonID)
	}
	if e.zoneProvider != nil && event.ZoneID != "" {
		event.ZoneName, _ = e.zoneProvider.GetZone(event.ZoneID)
	}
	if e.deviceProvider != nil && event.DeviceMAC != "" {
		event.DeviceName, _ = e.deviceProvider.GetDevice(event.DeviceMAC)
	}

	// Trigger the automation in test mode
	e.triggerAutomation(a, event, time.Now(), true)

	return nil
}

// UpdateZoneDwellTracking updates dwell time tracking for zones.
func (e *Engine) UpdateZoneDwellTracking(blobID int, zoneID string, now time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if zoneID != "" {
		// Blob entered a zone
		if e.zoneEnterTime[zoneID] == nil {
			e.zoneEnterTime[zoneID] = make(map[int]time.Time)
		}
		if _, exists := e.zoneEnterTime[zoneID][blobID]; !exists {
			e.zoneEnterTime[zoneID][blobID] = now
		}
	}
}

// CheckDwellTriggers checks for dwell time triggers.
func (e *Engine) CheckDwellTriggers(now time.Time) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, a := range e.automations {
		if !a.Enabled || a.TriggerType != TriggerZoneDwell {
			continue
		}

		cfg := a.TriggerConfig
		targetZone := cfg.ZoneID

		// Check cooldown
		if lastTrigger, exists := e.cooldowns[a.ID]; exists {
			if now.Sub(lastTrigger).Seconds() < float64(a.Cooldown) {
				continue
			}
		}

		// Check all zones or specific zone
		for zoneID, blobTimes := range e.zoneEnterTime {
			if targetZone != "" && zoneID != targetZone {
				continue
			}

			for blobID, enterTime := range blobTimes {
				dwellMinutes := int(now.Sub(enterTime).Minutes())
				if dwellMinutes >= cfg.DwellMinutes {
					// Check conditions
					event := Event{
						Type:      TriggerZoneDwell,
						Timestamp: now,
						ZoneID:    zoneID,
					}
					if !e.evaluateConditions(a.Conditions, event, now) {
						continue
					}

					// Trigger automation
					e.triggerAutomation(a, event, now, false)

					// Reset enter time to prevent repeated triggers
					delete(e.zoneEnterTime[zoneID], blobID)
				}
			}
		}
	}
}

// UpdateBLEDeviceSeen updates the last seen time for a BLE device.
func (e *Engine) UpdateBLEDeviceSeen(mac string, now time.Time) {
	e.mu.Lock()
	e.deviceLastSeen[mac] = now
	e.mu.Unlock()
}

// CheckBLEAbsentTriggers checks for BLE device absent triggers.
func (e *Engine) CheckBLEAbsentTriggers(now time.Time) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, a := range e.automations {
		if !a.Enabled || a.TriggerType != TriggerBLEDeviceAbsent {
			continue
		}

		cfg := a.TriggerConfig

		// Check cooldown
		if lastTrigger, exists := e.cooldowns[a.ID]; exists {
			if now.Sub(lastTrigger).Seconds() < float64(a.Cooldown) {
				continue
			}
		}

		targetMAC := cfg.DeviceMAC
		if targetMAC == "" {
			continue // Need a specific device
		}

		lastSeen, exists := e.deviceLastSeen[targetMAC]
		if !exists {
			// Device never seen - trigger immediately
			event := Event{
				Type:       TriggerBLEDeviceAbsent,
				Timestamp:  now,
				DeviceMAC:  targetMAC,
			}
			if e.deviceProvider != nil {
				event.DeviceName, _ = e.deviceProvider.GetDevice(targetMAC)
			}
			if e.evaluateConditions(a.Conditions, event, now) {
				e.triggerAutomation(a, event, now, false)
			}
			continue
		}

		absentMinutes := int(now.Sub(lastSeen).Minutes())
		if absentMinutes >= cfg.AbsentMinutes {
			event := Event{
				Type:       TriggerBLEDeviceAbsent,
				Timestamp:  now,
				DeviceMAC:  targetMAC,
			}
			if e.deviceProvider != nil {
				event.DeviceName, _ = e.deviceProvider.GetDevice(targetMAC)
			}
			if e.evaluateConditions(a.Conditions, event, now) {
				e.triggerAutomation(a, event, now, false)
			}
		}
	}
}

// IsInVolume checks if a point is inside a trigger volume.
func (e *Engine) IsInVolume(x, y, z float64, volume TriggerVolume) bool {
	switch volume.Type {
	case "box":
		return x >= volume.MinX && x <= volume.MaxX &&
			y >= volume.MinY && y <= volume.MaxY &&
			z >= volume.MinZ && z <= volume.MaxZ

	case "sphere":
		dist := math.Sqrt(math.Pow(x-volume.CenterX, 2) +
			math.Pow(y-volume.CenterY, 2) +
			math.Pow(z-volume.CenterZ, 2))
		return dist <= volume.Radius

	case "cylinder":
		// Check horizontal distance from base
		dist := math.Sqrt(math.Pow(x-volume.BaseX, 2) + math.Pow(z-volume.BaseZ, 2))
		return dist <= volume.BaseRadius && y >= volume.MinHeight && y <= volume.MaxHeight

	default:
		return false
	}
}

// TrackedBlob represents a tracked entity for automation evaluation.
type TrackedBlob struct {
	ID         int
	X, Y, Z    float64
	VX, VY, VZ float64
	Confidence float64
}

// Evaluate processes tracked blobs and triggers automations based on zone/volume crossings.
// The zoneForBlob function returns the current zone ID for a blob, or empty string if not in a zone.
func (e *Engine) Evaluate(blobs []TrackedBlob, zoneForBlob func(blobID int) string) {
	now := time.Now()

	for _, blob := range blobs {
		currentZone := zoneForBlob(blob.ID)

		// Update dwell tracking for current zone
		if currentZone != "" {
			e.UpdateZoneDwellTracking(blob.ID, currentZone, now)
		}

		// Check for trigger volume crossings
		e.checkVolumeTriggers(blob, currentZone, now)
	}

	// Check dwell triggers
	e.CheckDwellTriggers(now)

	// Check BLE absent triggers
	e.CheckBLEAbsentTriggers(now)
}

// checkVolumeTriggers checks if a blob has entered or left any trigger volumes.
func (e *Engine) checkVolumeTriggers(blob TrackedBlob, currentZone string, now time.Time) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, volume := range e.volumes {
		if !volume.Enabled {
			continue
		}

		inVolume := e.IsInVolume(blob.X, blob.Y, blob.Z, *volume)

		// Find automations using this volume
		for _, a := range e.automations {
			if !a.Enabled {
				continue
			}
			if a.TriggerConfig.VolumeID != volume.ID {
				continue
			}

			// Check cooldown
			if lastTrigger, exists := e.cooldowns[a.ID]; exists {
				if now.Sub(lastTrigger).Seconds() < float64(a.Cooldown) {
					continue
				}
			}

			// Check trigger type and create event
			var event Event
			var shouldTrigger bool

			switch a.TriggerType {
			case TriggerVolumeEnter:
				// Trigger when blob enters the volume
				if inVolume {
					event = Event{
						Type:       TriggerVolumeEnter,
						Timestamp:  now,
						PersonID:   a.TriggerConfig.PersonID,
						ZoneID:     currentZone,
						ZoneName:   currentZone,
						Confidence: blob.Confidence,
						Extra: map[string]interface{}{
							"volume_id":   volume.ID,
							"volume_name": volume.Name,
							"position":    []float64{blob.X, blob.Y, blob.Z},
						},
					}
					shouldTrigger = true
				}

			case TriggerVolumeLeave:
				// Trigger when blob leaves the volume
				if !inVolume {
					event = Event{
						Type:       TriggerVolumeLeave,
						Timestamp:  now,
						PersonID:   a.TriggerConfig.PersonID,
						ZoneID:     currentZone,
						ZoneName:   currentZone,
						Confidence: blob.Confidence,
						Extra: map[string]interface{}{
							"volume_id":   volume.ID,
							"volume_name": volume.Name,
							"position":    []float64{blob.X, blob.Y, blob.Z},
						},
					}
					shouldTrigger = true
				}
			}

			if shouldTrigger {
				// Evaluate conditions
				if !e.evaluateConditions(a.Conditions, event, now) {
					continue
				}

				// Trigger the automation
				e.triggerAutomation(a, event, now, false)
			}
		}
	}
}

// GetRecentActionLog returns recent action log entries.
func (e *Engine) GetRecentActionLog(limit int) []struct {
	AutomationID  string
	FiredAt       time.Time
	EventJSON     string
	ResultsJSON   string
} {
	rows, err := e.db.Query(`
		SELECT automation_id, fired_at, event_json, actions_results_json
		FROM action_log
		ORDER BY fired_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		log.Printf("[WARN] Failed to query action log: %v", err)
		return nil
	}
	defer rows.Close()

	var results []struct {
		AutomationID  string
		FiredAt       time.Time
		EventJSON     string
		ResultsJSON   string
	}

	for rows.Next() {
		var entry struct {
			AutomationID  string
			FiredAt       time.Time
			EventJSON     string
			ResultsJSON   string
		}
		var firedAtNS int64
		if err := rows.Scan(&entry.AutomationID, &firedAtNS, &entry.EventJSON, &entry.ResultsJSON); err != nil {
			continue
		}
		entry.FiredAt = time.Unix(0, firedAtNS)
		results = append(results, entry)
	}

	return results
}
