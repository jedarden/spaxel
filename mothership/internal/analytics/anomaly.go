// Package analytics provides anomaly detection based on learned normal behaviour patterns.
package analytics

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/spaxel/mothership/internal/events"
	"github.com/spaxel/mothership/internal/learning"

	_ "modernc.org/sqlite"
)

// NormalBehaviourSlot represents expected behaviour for a specific hour_of_week and zone.
type NormalBehaviourSlot struct {
	HourOfWeek         int   `json:"hour_of_week"`          // 0-167
	ZoneID             string `json:"zone_id"`
	ExpectedOccupancy  float64 `json:"expected_occupancy"` // 0.0-1.0, fraction of samples with occupancy
	TypicalPersonCount float64 `json:"typical_person_count"` // Mean person count
	SampleCount        int     `json:"sample_count"`
	TypicalBLEDevices  map[string]float64 `json:"typical_ble_devices,omitempty"` // MAC -> frequency (0.0-1.0)
}

// DwellBehaviourSlot represents expected dwell duration for a person in a zone at a specific hour.
type DwellBehaviourSlot struct {
	HourOfWeek        int           `json:"hour_of_week"`
	ZoneID            string        `json:"zone_id"`
	PersonID          string        `json:"person_id"`
	MeanDwellDuration time.Duration `json:"mean_dwell_duration"`
	StdDwellDuration  time.Duration `json:"std_dwell_duration"`
	SampleCount       int           `json:"sample_count"`
}

// AnomalyScoreConfig holds configurable thresholds for anomaly scoring.
type AnomalyScoreConfig struct {
	// Unusual hour presence
	UnusualHourScore          float64 `json:"unusual_hour_score"`           // Default: 0.7
	UnusualHourScoreSecurity  float64 `json:"unusual_hour_score_security"`  // Default: 0.9
	LateNightMultiplier       float64 `json:"late_night_multiplier"`        // Default: 1.5 (00:00-06:00)

	// Unknown BLE device
	UnknownBLEScore          float64 `json:"unknown_ble_score"`           // Default: 0.5
	UnknownBLEScoreSecurity  float64 `json:"unknown_ble_score_security"`  // Default: 0.8
	SeenOnceScore            float64 `json:"seen_once_score"`             // Default: 0.3
	CloseRangeRSSIThreshold  int     `json:"close_range_rssi_threshold"`  // Default: -60 dBm

	// Motion during away
	MotionDuringAwayScore    float64 `json:"motion_during_away_score"`    // Default: 0.95

	// Unusual dwell duration
	UnusualDwellScore        float64 `json:"unusual_dwell_score"`         // Default: 0.4
	DwellMultiplierThreshold float64 `json:"dwell_multiplier_threshold"`  // Default: 5.0

	// Alert thresholds
	AlertThresholdNormal     float64 `json:"alert_threshold_normal"`      // Default: 0.6
	AlertThresholdSecurity   float64 `json:"alert_threshold_security"`   // Default: 0.4

	// Auto-away/disarm
	AutoAwayDuration         time.Duration `json:"auto_away_duration"`     // Default: 15 minutes
	AutoDisarmRSSIThreshold  int           `json:"auto_disarm_rssi_threshold"` // Default: -70 dBm
	ManualOverrideDuration   time.Duration `json:"manual_override_duration"` // Default: 30 minutes
}

// DefaultAnomalyScoreConfig returns default configuration.
func DefaultAnomalyScoreConfig() AnomalyScoreConfig {
	return AnomalyScoreConfig{
		UnusualHourScore:          0.7,
		UnusualHourScoreSecurity:  0.9,
		LateNightMultiplier:       1.5,
		UnknownBLEScore:           0.5,
		UnknownBLEScoreSecurity:   0.8,
		SeenOnceScore:             0.3,
		CloseRangeRSSIThreshold:   -60,
		MotionDuringAwayScore:     0.95,
		UnusualDwellScore:         0.4,
		DwellMultiplierThreshold:  5.0,
		AlertThresholdNormal:      0.6,
		AlertThresholdSecurity:    0.4,
		AutoAwayDuration:          15 * time.Minute,
		AutoDisarmRSSIThreshold:   -70,
		ManualOverrideDuration:    30 * time.Minute,
	}
}

// SecurityMode represents the current security mode state.
type SecurityMode string

const (
	SecurityModeDisarmed SecurityMode = "disarmed"
	SecurityModeArmed    SecurityMode = "armed"
	SecurityModeArmedStay SecurityMode = "armed_stay" // Armed but people are home
)

// AutoAwayState tracks state for auto-away functionality.
type AutoAwayState struct {
	LastMotionTime   time.Time `json:"last_motion_time"`
	LastPersonCount  int       `json:"last_person_count"`
	AutoAwayTriggered bool     `json:"auto_away_triggered"`
}

// AutoDisarmState tracks state for auto-disarm functionality.
type AutoDisarmState struct {
	RegisteredDeviceSeen bool      `json:"registered_device_seen"`
	SeenDeviceMAC       string    `json:"seen_device_mac"`
	SeenDeviceRSSI      int       `json:"seen_device_rssi"`
	LastSeenTime        time.Time `json:"last_seen_time"`
}

// AnomalyCooldownConfig holds configuration for anomaly deduplication.
type AnomalyCooldownConfig struct {
	// Cooldown duration per anomaly type+zone combination
	UnusualHourCooldown    time.Duration `json:"unusual_hour_cooldown"`    // Default: 30 minutes
	UnknownBLECooldown     time.Duration `json:"unknown_ble_cooldown"`     // Default: 10 minutes
	MotionDuringAwayCooldown time.Duration `json:"motion_during_away_cooldown"` // Default: 5 minutes
	UnusualDwellCooldown   time.Duration `json:"unusual_dwell_cooldown"`   // Default: 1 hour
}

// DefaultAnomalyCooldownConfig returns default cooldown configuration.
func DefaultAnomalyCooldownConfig() AnomalyCooldownConfig {
	return AnomalyCooldownConfig{
		UnusualHourCooldown:      30 * time.Minute,
		UnknownBLECooldown:       10 * time.Minute,
		MotionDuringAwayCooldown: 5 * time.Minute,
		UnusualDwellCooldown:     1 * time.Hour,
	}
}

// cooldownKey tracks the last time an anomaly was raised for deduplication.
type cooldownKey struct {
	anomalyType events.AnomalyType
	zoneID      string
	personID    string
	deviceMAC   string
}

// Detector detects anomalies based on learned normal behaviour.
type Detector struct {
	mu      sync.RWMutex
	db      *sql.DB
	config  AnomalyScoreConfig
	cooldownConfig AnomalyCooldownConfig

	// Normal behaviour model (loaded from DB)
	behaviourSlots   map[string]*NormalBehaviourSlot   // key: "hour-zone"
	dwellSlots       map[string]*DwellBehaviourSlot    // key: "hour-zone-person"

	// Active anomaly tracking
	activeAnomalies  map[string]*events.AnomalyEvent    // id -> event
	anomalyHistory   []*events.AnomalyEvent

	// Pending alert timers
	pendingAlerts    map[string]*alertTimerState

	// Anomaly cooldown tracking for deduplication
	anomalyCooldowns map[cooldownKey]time.Time // key -> last triggered time

	// Model state
	learningStartTime time.Time
	modelReady        bool
	modelReadyAt      time.Time

	// Registered devices and people
	registeredDevices map[string]bool   // MAC -> registered
	registeredPeople  map[string]string // person_id -> name
	deviceFirstSeen   map[string]time.Time // MAC -> first seen time

	// Security mode state
	securityMode      SecurityMode
	autoAwayState     AutoAwayState
	autoDisarmState   AutoDisarmState
	manualOverrideUntil time.Time // Manual mode override expiry

	// Providers
	zoneProvider      ZoneProvider
	personProvider    PersonProvider
	deviceProvider    DeviceProvider
	positionProvider  PositionProvider
	alertHandler      AlertHandler

	// Feedback store for accuracy tracking
	feedbackStore     *learning.FeedbackStore

	// Callbacks
	onAnomaly         func(event events.AnomalyEvent)
	onModeChange      func(event events.SystemModeChangeEvent)
	onSecurityModeChange func(oldMode, newMode SecurityMode, reason string)
}

// ZoneProvider provides zone information.
type ZoneProvider interface {
	GetZoneName(zoneID string) string
	GetZoneOccupancy(zoneID string) (count int, blobIDs []int)
}

// PersonProvider provides person information.
type PersonProvider interface {
	GetPersonDevices(personID string) ([]string, error)
	GetAllRegisteredDevices() (map[string]string, error) // MAC -> person_id
	GetPersonName(personID string) string
}

// DeviceProvider provides device information.
type DeviceProvider interface {
	IsDeviceRegistered(mac string) bool
	IsDeviceSeenBefore(mac string) bool
	GetDeviceName(mac string) string
}

// PositionProvider provides position for blobs.
type PositionProvider interface {
	GetBlobPosition(blobID int) (x, y, z float64, ok bool)
}

// AlertHandler handles alert delivery.
type AlertHandler interface {
	SendAlert(event events.AnomalyEvent, immediate bool) error
	SendWebhook(event events.AnomalyEvent, immediate bool) error
	SendEscalation(event events.AnomalyEvent) error
}

type alertTimerState struct {
	alertTimer      *time.Timer
	webhookTimer    *time.Timer
	escalationTimer *time.Timer
	anomalyID       string
}

// NewDetector creates a new anomaly detector.
func NewDetector(dbPath string, config AnomalyScoreConfig) (*Detector, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	d := &Detector{
		db:               db,
		config:           config,
		cooldownConfig:   DefaultAnomalyCooldownConfig(),
		behaviourSlots:   make(map[string]*NormalBehaviourSlot),
		dwellSlots:       make(map[string]*DwellBehaviourSlot),
		activeAnomalies:  make(map[string]*events.AnomalyEvent),
		pendingAlerts:    make(map[string]*alertTimerState),
		anomalyCooldowns: make(map[cooldownKey]time.Time),
		registeredDevices: make(map[string]bool),
		registeredPeople:  make(map[string]string),
		deviceFirstSeen:   make(map[string]time.Time),
		securityMode:      SecurityModeDisarmed,
	}

	if err := d.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	if err := d.loadBehaviourModel(); err != nil {
		log.Printf("[WARN] Failed to load behaviour model: %v", err)
	}

	if err := d.loadLearningState(); err != nil {
		log.Printf("[WARN] Failed to load learning state: %v", err)
	}

	return d, nil
}

func (d *Detector) migrate() error {
	_, err := d.db.Exec(`
		CREATE TABLE IF NOT EXISTS behaviour_slots (
			hour_of_week    INTEGER NOT NULL,
			zone_id         TEXT    NOT NULL,
			expected_occupancy REAL NOT NULL DEFAULT 0,
			typical_person_count REAL NOT NULL DEFAULT 0,
			sample_count    INTEGER NOT NULL DEFAULT 0,
			typical_ble_devices TEXT NOT NULL DEFAULT '{}',
			PRIMARY KEY (hour_of_week, zone_id)
		);

		CREATE TABLE IF NOT EXISTS dwell_slots (
			hour_of_week    INTEGER NOT NULL,
			zone_id         TEXT    NOT NULL,
			person_id       TEXT    NOT NULL,
			mean_dwell_ns   INTEGER NOT NULL DEFAULT 0,
			std_dwell_ns    INTEGER NOT NULL DEFAULT 0,
			sample_count    INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (hour_of_week, zone_id, person_id)
		);

		CREATE TABLE IF NOT EXISTS occupancy_samples (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			hour_of_week    INTEGER NOT NULL,
			zone_id         TEXT    NOT NULL,
			person_count    INTEGER NOT NULL,
			ble_devices     TEXT    NOT NULL DEFAULT '[]',
			timestamp       INTEGER NOT NULL
		);

		CREATE TABLE IF NOT EXISTS dwell_samples (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			hour_of_week    INTEGER NOT NULL,
			zone_id         TEXT    NOT NULL,
			person_id       TEXT    NOT NULL,
			dwell_ns        INTEGER NOT NULL,
			timestamp       INTEGER NOT NULL
		);

		CREATE TABLE IF NOT EXISTS anomaly_events (
			id              TEXT PRIMARY KEY,
			type            TEXT    NOT NULL,
			score           REAL    NOT NULL,
			description     TEXT    NOT NULL,
			timestamp       INTEGER NOT NULL,
			zone_id         TEXT,
			zone_name       TEXT,
			blob_id         INTEGER,
			person_id       TEXT,
			person_name     TEXT,
			device_mac      TEXT,
			device_name     TEXT,
			position_x      REAL,
			position_y      REAL,
			position_z      REAL,
			hour_of_week    INTEGER,
			expected_occupancy REAL,
			dwell_duration_ns INTEGER,
			expected_dwell_ns INTEGER,
			rssi_dbm        INTEGER,
			seen_before     INTEGER,
			acknowledged    INTEGER NOT NULL DEFAULT 0,
			acknowledged_at INTEGER,
			feedback        TEXT,
			alert_sent      INTEGER NOT NULL DEFAULT 0,
			webhook_sent    INTEGER NOT NULL DEFAULT 0,
			escalation_sent INTEGER NOT NULL DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS learning_state (
			key             TEXT PRIMARY KEY,
			value           TEXT    NOT NULL
		);

		CREATE TABLE IF NOT EXISTS device_first_seen (
			mac             TEXT PRIMARY KEY,
			first_seen_ns   INTEGER NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_occupancy_samples_time ON occupancy_samples(timestamp);
		CREATE INDEX IF NOT EXISTS idx_dwell_samples_time ON dwell_samples(timestamp);
		CREATE INDEX IF NOT EXISTS idx_anomaly_events_time ON anomaly_events(timestamp);
	`)
	return err
}

func (d *Detector) loadBehaviourModel() error {
	// Load behaviour slots
	rows, err := d.db.Query(`
		SELECT hour_of_week, zone_id, expected_occupancy, typical_person_count, sample_count, typical_ble_devices
		FROM behaviour_slots
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		slot := &NormalBehaviourSlot{
			TypicalBLEDevices: make(map[string]float64),
		}
		var bleDevicesJSON string
		if err := rows.Scan(&slot.HourOfWeek, &slot.ZoneID, &slot.ExpectedOccupancy,
			&slot.TypicalPersonCount, &slot.SampleCount, &bleDevicesJSON); err != nil {
			continue
		}
		// Parse BLE devices JSON
		if bleDevicesJSON != "" && bleDevicesJSON != "{}" {
			var devices map[string]float64
			if err := jsonUnmarshal(bleDevicesJSON, &devices); err == nil {
				slot.TypicalBLEDevices = devices
			}
		}
		key := fmt.Sprintf("%d-%s", slot.HourOfWeek, slot.ZoneID)
		d.behaviourSlots[key] = slot
	}

	// Load dwell slots
	dwellRows, err := d.db.Query(`
		SELECT hour_of_week, zone_id, person_id, mean_dwell_ns, std_dwell_ns, sample_count
		FROM dwell_slots
	`)
	if err != nil {
		return err
	}
	defer dwellRows.Close()

	for dwellRows.Next() {
		slot := &DwellBehaviourSlot{}
		var meanNS, stdNS int64
		if err := dwellRows.Scan(&slot.HourOfWeek, &slot.ZoneID, &slot.PersonID,
			&meanNS, &stdNS, &slot.SampleCount); err != nil {
			continue
		}
		slot.MeanDwellDuration = time.Duration(meanNS)
		slot.StdDwellDuration = time.Duration(stdNS)
		key := fmt.Sprintf("%d-%s-%s", slot.HourOfWeek, slot.ZoneID, slot.PersonID)
		d.dwellSlots[key] = slot
	}

	return nil
}

func (d *Detector) loadLearningState() error {
	var startNS int64
	err := d.db.QueryRow(`SELECT value FROM learning_state WHERE key = 'learning_start'`).Scan(&startNS)
	if err == sql.ErrNoRows {
		// Initialize learning start time
		d.learningStartTime = time.Now()
		d.db.Exec(`INSERT INTO learning_state (key, value) VALUES ('learning_start', ?)`, time.Now().UnixNano())
		return nil
	}
	if err != nil {
		return err
	}

	d.learningStartTime = time.Unix(0, startNS)

	// Check if 7 days have passed
	if time.Since(d.learningStartTime) >= 7*24*time.Hour {
		d.modelReady = true
		d.modelReadyAt = d.learningStartTime.Add(7 * 24 * time.Hour)
	}

	// Load security_mode from database (persisted across restarts)
	var securityModeStr string
	err = d.db.QueryRow(`SELECT value FROM learning_state WHERE key = 'security_mode'`).Scan(&securityModeStr)
	if err == nil {
		d.securityMode = SecurityMode(securityModeStr)
		log.Printf("[INFO] Loaded security mode from database: %s", d.securityMode)
	} else if err != sql.ErrNoRows {
		log.Printf("[WARN] Failed to load security_mode from database: %v", err)
	}
	// If security_mode doesn't exist in DB, default to disarmed (already set in NewDetector)

	// Load device first seen times
	deviceRows, err := d.db.Query(`SELECT mac, first_seen_ns FROM device_first_seen`)
	if err != nil {
		return err
	}
	defer deviceRows.Close()

	for deviceRows.Next() {
		var mac string
		var firstSeenNS int64
		if err := deviceRows.Scan(&mac, &firstSeenNS); err != nil {
			continue
		}
		d.deviceFirstSeen[mac] = time.Unix(0, firstSeenNS)
	}

	return nil
}

// Close closes the database.
func (d *Detector) Close() error {
	return d.db.Close()
}

// SetZoneProvider sets the zone provider.
func (d *Detector) SetZoneProvider(p ZoneProvider) {
	d.mu.Lock()
	d.zoneProvider = p
	d.mu.Unlock()
}

// SetPersonProvider sets the person provider.
func (d *Detector) SetPersonProvider(p PersonProvider) {
	d.mu.Lock()
	d.personProvider = p
	d.mu.Unlock()
}

// SetDeviceProvider sets the device provider.
func (d *Detector) SetDeviceProvider(p DeviceProvider) {
	d.mu.Lock()
	d.deviceProvider = p
	d.mu.Unlock()
}

// SetPositionProvider sets the position provider.
func (d *Detector) SetPositionProvider(p PositionProvider) {
	d.mu.Lock()
	d.positionProvider = p
	d.mu.Unlock()
}

// SetAlertHandler sets the alert handler.
func (d *Detector) SetAlertHandler(h AlertHandler) {
	d.mu.Lock()
	d.alertHandler = h
	d.mu.Unlock()
}

// SetFeedbackStore sets the feedback store for accuracy tracking.
func (d *Detector) SetFeedbackStore(store *learning.FeedbackStore) {
	d.mu.Lock()
	d.feedbackStore = store
	d.mu.Unlock()
}

// SetOnAnomaly sets callback for anomaly events.
func (d *Detector) SetOnAnomaly(cb func(event events.AnomalyEvent)) {
	d.mu.Lock()
	d.onAnomaly = cb
	d.mu.Unlock()
}

// SetOnModeChange sets callback for mode change events.
func (d *Detector) SetOnModeChange(cb func(event events.SystemModeChangeEvent)) {
	d.mu.Lock()
	d.onModeChange = cb
	d.mu.Unlock()
}

// SetOnSecurityModeChange sets callback for security mode changes.
func (d *Detector) SetOnSecurityModeChange(cb func(oldMode, newMode SecurityMode, reason string)) {
	d.mu.Lock()
	d.onSecurityModeChange = cb
	d.mu.Unlock()
}

// GetSecurityMode returns the current security mode.
func (d *Detector) GetSecurityMode() SecurityMode {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.securityMode
}

// SetSecurityMode sets the security mode.
func (d *Detector) SetSecurityMode(mode SecurityMode, reason string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.securityMode == mode {
		return // No change
	}

	oldMode := d.securityMode
	d.securityMode = mode

	log.Printf("[INFO] Security mode changed: %s -> %s (reason: %s)", oldMode, mode, reason)

	// Fire callback
	if d.onSecurityModeChange != nil {
		go d.onSecurityModeChange(oldMode, mode, reason)
	}

	// Persist to database
	d.db.Exec(`INSERT OR REPLACE INTO learning_state (key, value) VALUES ('security_mode', ?)`, string(mode))

	if mode == SecurityModeArmed || mode == SecurityModeArmedStay {
		// Record armed timestamp for persistence across restarts
		d.db.Exec(`INSERT OR REPLACE INTO learning_state (key, value) VALUES ('security_mode_armed_at', ?)`, time.Now().UnixNano())
		d.manualOverrideUntil = time.Time{}
	} else {
		// Clear armed timestamp on disarm
		d.db.Exec(`DELETE FROM learning_state WHERE key = 'security_mode_armed_at'`)
	}
}

// IsSecurityModeActive returns true if security mode is armed or armed_stay.
func (d *Detector) IsSecurityModeActive() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.securityMode == SecurityModeArmed || d.securityMode == SecurityModeArmedStay
}

// GetArmedAt returns the timestamp when security mode was last armed, or nil if disarmed.
// The timestamp is loaded from the persisted learning_state table on startup.
func (d *Detector) GetArmedAt() *time.Time {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.securityMode != SecurityModeArmed && d.securityMode != SecurityModeArmedStay {
		return nil
	}

	var armedAtNS int64
	err := d.db.QueryRow(
		`SELECT value FROM learning_state WHERE key = 'security_mode_armed_at'`,
	).Scan(&armedAtNS)
	if err != nil {
		return nil
	}

	t := time.Unix(0, armedAtNS)
	return &t
}

// SetManualOverride sets a temporary override for security mode.
func (d *Detector) SetManualOverride(duration time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.manualOverrideUntil = time.Now().Add(duration)
	log.Printf("[INFO] Manual security override set for %v", duration)
}

// ClearManualOverride clears the manual override.
func (d *Detector) ClearManualOverride() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.manualOverrideUntil = time.Time{}
	log.Printf("[INFO] Manual security override cleared")
}

// IsManualOverrideActive returns true if manual override is active.
func (d *Detector) IsManualOverrideActive() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return !d.manualOverrideUntil.IsZero() && time.Now().Before(d.manualOverrideUntil)
}

// SetRegisteredDevices sets the list of registered BLE devices.
func (d *Detector) SetRegisteredDevices(devices []string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.registeredDevices = make(map[string]bool)
	for _, mac := range devices {
		d.registeredDevices[mac] = true
	}
}

// IsModelReady returns true if 7 days of learning have passed.
func (d *Detector) IsModelReady() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.modelReady
}

// GetLearningProgress returns the fraction of learning completed (0.0-1.0).
func (d *Detector) GetLearningProgress() float64 {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.modelReady {
		return 1.0
	}

	elapsed := time.Since(d.learningStartTime)
	total := 7 * 24 * time.Hour
	progress := float64(elapsed) / float64(total)
	if progress > 1.0 {
		progress = 1.0
	}
	return progress
}

// ProcessBLEDeviceSighting processes a BLE device sighting for auto-away/auto-disarm.
// Returns a SystemModeChangeEvent if the mode changed, nil otherwise.
func (d *Detector) ProcessBLEDeviceSighting(mac string, rssi int, nodeMAC string) *events.SystemModeChangeEvent {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()

	// Track when registered devices are seen
	if d.registeredDevices[mac] && rssi > d.config.AutoDisarmRSSIThreshold {
		// Registered device detected in close range
		d.autoDisarmState.RegisteredDeviceSeen = true
		d.autoDisarmState.SeenDeviceMAC = mac
		d.autoDisarmState.SeenDeviceRSSI = rssi
		d.autoDisarmState.LastSeenTime = now

		// Auto-disarm: if currently in away mode, switch to home
		if d.securityMode == SecurityModeArmed && !d.IsManualOverrideActive() {
			// Get person name from device provider
			personName := ""
			if d.personProvider != nil {
				if devices, err := d.personProvider.GetAllRegisteredDevices(); err == nil {
					if personID, ok := devices[mac]; ok {
						personName = d.personProvider.GetPersonName(personID)
					}
				}
			}

			reason := "Auto-disarm activated — registered device detected"
			if personName != "" {
				reason = fmt.Sprintf("Welcome home — %s arrived", personName)
			}

			return d.setSystemMode(events.ModeHome, reason, personName)
		}
	}

	// Update last motion time for auto-away (any BLE device indicates presence)
	d.autoAwayState.LastMotionTime = now

	return nil
}

// ProcessMotionForAutoAway should be called when any motion is detected.
// It updates the last motion time to prevent auto-away while activity is present.
func (d *Detector) ProcessMotionForAutoAway() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.autoAwayState.LastMotionTime = time.Now()
}

// CheckAutoAway checks if auto-away should be triggered (all registered devices absent for > 15 minutes).
// Should be called periodically (e.g., every minute).
// Returns a SystemModeChangeEvent if mode changed, nil otherwise.
func (d *Detector) CheckAutoAway() *events.SystemModeChangeEvent {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()

	// Check if all registered devices have been absent for > 15 minutes
	if len(d.registeredDevices) == 0 {
		return nil // No registered devices, can't do auto-away
	}

	// Check if any registered device has been seen recently (within last 15 minutes)
	// This is a simplified check - in practice, we'd track per-device last seen times
	timeSinceLastMotion := now.Sub(d.autoAwayState.LastMotionTime)

	if timeSinceLastMotion > d.config.AutoAwayDuration && !d.IsManualOverrideActive() {
		// Only trigger if not already in away mode
		if d.securityMode != SecurityModeArmed {
			return d.setSystemMode(events.ModeAway, "Auto-away activated — all BLE devices absent", "")
		}
	}

	return nil
}

// setSystemMode sets the system mode and fires the mode change callback.
// Must be called while holding the mutex.
func (d *Detector) setSystemMode(newMode events.SystemMode, reason, personName string) *events.SystemModeChangeEvent {
	oldSecurityMode := d.securityMode
	oldMode := d.securityModeToSystemMode(d.securityMode)
	event := &events.SystemModeChangeEvent{
		PreviousMode: oldMode,
		NewMode:      newMode,
		Reason:       reason,
		Timestamp:    time.Now(),
		PersonName:   personName,
	}

	// Update security mode state
	switch newMode {
	case events.ModeAway:
		d.securityMode = SecurityModeArmed
	case events.ModeHome:
		d.securityMode = SecurityModeDisarmed
	case events.ModeSleep:
		d.securityMode = SecurityModeArmedStay // Stay mode for sleep
	}

	// Fire callback
	if d.onModeChange != nil {
		go d.onModeChange(*event)
	}

	// Broadcast to dashboard
	if d.onSecurityModeChange != nil {
		go d.onSecurityModeChange(oldSecurityMode, d.securityMode, reason)
	}

	// Persist to database
	d.db.Exec(`INSERT OR REPLACE INTO learning_state (key, value) VALUES ('security_mode', ?)`, string(d.securityMode))

	log.Printf("[INFO] System mode changed: %s -> %s (reason: %s)", oldMode, newMode, reason)

	return event
}

// securityModeToSystemMode converts SecurityMode to SystemMode.
func (d *Detector) securityModeToSystemMode(mode SecurityMode) events.SystemMode {
	switch mode {
	case SecurityModeArmed:
		return events.ModeAway
	case SecurityModeArmedStay:
		return events.ModeSleep
	default:
		return events.ModeHome
	}
}

// GetSystemMode returns the current SystemMode (Home/Away/Sleep).
func (d *Detector) GetSystemMode() events.SystemMode {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.securityModeToSystemMode(d.securityMode)
}

// ProcessOccupancy records an occupancy observation and checks for unusual hour anomalies.
func (d *Detector) ProcessOccupancy(zoneID string, personCount int, bleDevices []string, isSecurityMode bool) *events.AnomalyEvent {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	hourOfWeek := getHourOfWeek(now)

	// Record the sample
	d.recordOccupancySample(hourOfWeek, zoneID, personCount, bleDevices, now)

	// Update person count for auto-away tracking
	d.autoAwayState.LastPersonCount = personCount
	if personCount > 0 {
		d.autoAwayState.LastMotionTime = now
	}

	// Check for anomaly (only if model is ready, or if in security mode)
	if !d.modelReady && !isSecurityMode {
		return nil
	}

	key := fmt.Sprintf("%d-%s", hourOfWeek, zoneID)
	slot, exists := d.behaviourSlots[key]

	if !exists || slot.SampleCount < 10 {
		// Not enough data for this slot
		return nil
	}

	// Check if this is an unusual hour (low expected occupancy but we see people)
	if personCount > 0 && slot.ExpectedOccupancy < 0.1 {
		// Check cooldown for this anomaly type+zone
		cooldownKey := cooldownKey{
			anomalyType: events.AnomalyUnusualHour,
			zoneID:      zoneID,
		}
		if d.isInCooldown(cooldownKey, d.cooldownConfig.UnusualHourCooldown) {
			return nil
		}

		score := d.config.UnusualHourScore
		if isSecurityMode {
			score = d.config.UnusualHourScoreSecurity
		}

		// Apply late night multiplier (00:00-06:00)
		hour := now.Hour()
		if hour >= 0 && hour < 6 {
			score *= d.config.LateNightMultiplier
			if score > 1.0 {
				score = 1.0
			}
		}

		// Get zone name
		zoneName := zoneID
		if d.zoneProvider != nil {
			zoneName = d.zoneProvider.GetZoneName(zoneID)
		}

		// Create anomaly event
		event := events.AnomalyEvent{
			ID:                uuid.New().String(),
			Type:              events.AnomalyUnusualHour,
			Score:             score,
			Description:       fmt.Sprintf("Motion detected in %s at %s (unusual hour)", zoneName, now.Format("3:04pm")),
			Timestamp:         now,
			ZoneID:            zoneID,
			ZoneName:          zoneName,
			HourOfWeek:        hourOfWeek,
			ExpectedOccupancy: slot.ExpectedOccupancy,
		}

		// Mark cooldown
		d.anomalyCooldowns[cooldownKey] = now

		return d.createAnomaly(&event, isSecurityMode)
	}

	return nil
}

// ProcessBLEDevice checks for unknown BLE device anomalies.
func (d *Detector) ProcessBLEDevice(mac string, rssi int, isSecurityMode bool) *events.AnomalyEvent {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()

	// Track first seen time for this device
	if _, exists := d.deviceFirstSeen[mac]; !exists {
		d.deviceFirstSeen[mac] = now
		d.db.Exec(`INSERT OR REPLACE INTO device_first_seen (mac, first_seen_ns) VALUES (?, ?)`,
			mac, now.UnixNano())
	}

	// Check if device is registered
	if d.registeredDevices[mac] {
		return nil
	}

	// Check if close range
	if rssi < d.config.CloseRangeRSSIThreshold {
		return nil // Not close enough to be concerning
	}

	// Check cooldown for this device
	cooldownKey := cooldownKey{
		anomalyType: events.AnomalyUnknownBLE,
		deviceMAC:   mac,
	}
	if d.isInCooldown(cooldownKey, d.cooldownConfig.UnknownBLECooldown) {
		return nil
	}

	// Check if device was seen before
	seenBefore := false
	if d.deviceProvider != nil {
		seenBefore = d.deviceProvider.IsDeviceSeenBefore(mac)
	}

	// Calculate score
	var score float64
	if !seenBefore {
		// Never seen before
		score = d.config.UnknownBLEScore
		if isSecurityMode {
			score = d.config.UnknownBLEScoreSecurity
		}
	} else {
		// Seen before but not registered
		score = d.config.SeenOnceScore
	}

	if score < d.getAlertThreshold(isSecurityMode) {
		return nil
	}

	// Get device name
	deviceName := mac
	if d.deviceProvider != nil {
		deviceName = d.deviceProvider.GetDeviceName(mac)
	}

	event := events.AnomalyEvent{
		ID:          uuid.New().String(),
		Type:        events.AnomalyUnknownBLE,
		Score:       score,
		Description: fmt.Sprintf("Unknown device detected nearby: %s (RSSI: %d dBm)", deviceName, rssi),
		Timestamp:   now,
		DeviceMAC:   mac,
		DeviceName:  deviceName,
		RSSIdBm:     rssi,
		SeenBefore:  seenBefore,
	}

	// Mark cooldown
	d.anomalyCooldowns[cooldownKey] = now

	return d.createAnomaly(&event, isSecurityMode)
}

// ProcessMotionDuringAway checks for motion when system is in away mode.
func (d *Detector) ProcessMotionDuringAway(zoneID string, blobID int, isSecurityMode bool) *events.AnomalyEvent {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()

	// Check cooldown for this anomaly type+zone
	cooldownKey := cooldownKey{
		anomalyType: events.AnomalyMotionDuringAway,
		zoneID:      zoneID,
	}
	if d.isInCooldown(cooldownKey, d.cooldownConfig.MotionDuringAwayCooldown) {
		return nil
	}

	// This anomaly always fires regardless of model training status
	score := d.config.MotionDuringAwayScore

	// Get zone name
	zoneName := zoneID
	if d.zoneProvider != nil {
		zoneName = d.zoneProvider.GetZoneName(zoneID)
	}

	// Get position
	var pos events.Position
	if d.positionProvider != nil {
		x, y, z, ok := d.positionProvider.GetBlobPosition(blobID)
		if ok {
			pos = events.Position{X: x, Y: y, Z: z}
		}
	}

	event := events.AnomalyEvent{
		ID:          uuid.New().String(),
		Type:        events.AnomalyMotionDuringAway,
		Score:       score,
		Description: fmt.Sprintf("Motion detected in %s while everyone is away", zoneName),
		Timestamp:   now,
		ZoneID:      zoneID,
		ZoneName:    zoneName,
		BlobID:      blobID,
		Position:    pos,
	}

	// Mark cooldown
	d.anomalyCooldowns[cooldownKey] = now

	return d.createAnomaly(&event, isSecurityMode)
}

// ProcessDwellDuration checks for unusual dwell duration.
func (d *Detector) ProcessDwellDuration(zoneID, personID string, dwellDuration time.Duration, isSecurityMode bool, isFallDetected bool) *events.AnomalyEvent {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Don't report if fall is already detected (fall detection takes priority)
	if isFallDetected {
		return nil
	}

	now := time.Now()
	hourOfWeek := getHourOfWeek(now)

	// Record the sample
	d.recordDwellSample(hourOfWeek, zoneID, personID, dwellDuration, now)

	// Only check if model is ready (this anomaly requires learned patterns)
	if !d.modelReady {
		return nil
	}

	key := fmt.Sprintf("%d-%s-%s", hourOfWeek, zoneID, personID)
	slot, exists := d.dwellSlots[key]

	if !exists || slot.SampleCount < 5 {
		return nil
	}

	// Check if dwelling for > 5x mean
	if dwellDuration > time.Duration(float64(slot.MeanDwellDuration)*d.config.DwellMultiplierThreshold) {
		// Check cooldown for this anomaly type+zone+person
		cooldownKey := cooldownKey{
			anomalyType: events.AnomalyUnusualDwell,
			zoneID:      zoneID,
			personID:    personID,
		}
		if d.isInCooldown(cooldownKey, d.cooldownConfig.UnusualDwellCooldown) {
			return nil
		}

		score := d.config.UnusualDwellScore

		// Get names
		zoneName := zoneID
		if d.zoneProvider != nil {
			zoneName = d.zoneProvider.GetZoneName(zoneID)
		}
		personName := personID
		if d.personProvider != nil {
			personName = d.personProvider.GetPersonName(personID)
		}

		event := events.AnomalyEvent{
			ID:             uuid.New().String(),
			Type:           events.AnomalyUnusualDwell,
			Score:          score,
			Description:    fmt.Sprintf("%s in %s for longer than usual (%.0f minutes)", personName, zoneName, dwellDuration.Minutes()),
			Timestamp:      now,
			ZoneID:         zoneID,
			ZoneName:       zoneName,
			PersonID:       personID,
			PersonName:     personName,
			DwellDuration:  dwellDuration,
			ExpectedDwell:  slot.MeanDwellDuration,
		}

		// Mark cooldown
		d.anomalyCooldowns[cooldownKey] = now

		return d.createAnomaly(&event, isSecurityMode)
	}

	return nil
}

func (d *Detector) createAnomaly(event *events.AnomalyEvent, isSecurityMode bool) *events.AnomalyEvent {
	threshold := d.getAlertThreshold(isSecurityMode)
	if event.Score < threshold {
		return nil
	}

	// Store in active anomalies
	d.activeAnomalies[event.ID] = event

	// Also append to history
	d.anomalyHistory = append(d.anomalyHistory, event)

	// Persist to database
	d.persistAnomaly(event)

	// Start alert chain
	d.startAlertChain(event, isSecurityMode)

	// Fire callback
	if d.onAnomaly != nil {
		go d.onAnomaly(*event)
	}

	log.Printf("[INFO] Anomaly detected: %s (score=%.2f, type=%s)", event.Description, event.Score, event.Type)

	return event
}

func (d *Detector) getAlertThreshold(isSecurityMode bool) float64 {
	if isSecurityMode {
		return d.config.AlertThresholdSecurity
	}
	return d.config.AlertThresholdNormal
}

// isInCooldown checks if an anomaly key is in cooldown period.
func (d *Detector) isInCooldown(key cooldownKey, duration time.Duration) bool {
	if lastTime, exists := d.anomalyCooldowns[key]; exists {
		return time.Since(lastTime) < duration
	}
	return false
}

// cleanupStaleCooldowns removes expired cooldown entries.
func (d *Detector) cleanupStaleCooldowns() {
	now := time.Now()
	maxCooldown := d.cooldownConfig.UnusualDwellCooldown // Use the longest cooldown
	for key, lastTime := range d.anomalyCooldowns {
		if now.Sub(lastTime) > maxCooldown {
			delete(d.anomalyCooldowns, key)
		}
	}
}

func (d *Detector) recordOccupancySample(hourOfWeek int, zoneID string, personCount int, bleDevices []string, timestamp time.Time) {
	devicesJSON, _ := jsonMarshal(bleDevices)
	_, err := d.db.Exec(`
		INSERT INTO occupancy_samples (hour_of_week, zone_id, person_count, ble_devices, timestamp)
		VALUES (?, ?, ?, ?, ?)
	`, hourOfWeek, zoneID, personCount, string(devicesJSON), timestamp.UnixNano())
	if err != nil {
		log.Printf("[WARN] Failed to record occupancy sample: %v", err)
	}
}

func (d *Detector) recordDwellSample(hourOfWeek int, zoneID, personID string, dwellDuration time.Duration, timestamp time.Time) {
	_, err := d.db.Exec(`
		INSERT INTO dwell_samples (hour_of_week, zone_id, person_id, dwell_ns, timestamp)
		VALUES (?, ?, ?, ?, ?)
	`, hourOfWeek, zoneID, personID, dwellDuration.Nanoseconds(), timestamp.UnixNano())
	if err != nil {
		log.Printf("[WARN] Failed to record dwell sample: %v", err)
	}
}

func (d *Detector) persistAnomaly(event *events.AnomalyEvent) {
	_, err := d.db.Exec(`
		INSERT INTO anomaly_events (
			id, type, score, description, timestamp,
			zone_id, zone_name, blob_id, person_id, person_name,
			device_mac, device_name, position_x, position_y, position_z,
			hour_of_week, expected_occupancy, dwell_duration_ns, expected_dwell_ns,
			rssi_dbm, seen_before
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, event.ID, event.Type, event.Score, event.Description, event.Timestamp.UnixNano(),
		nullString(event.ZoneID), nullString(event.ZoneName), event.BlobID,
		nullString(event.PersonID), nullString(event.PersonName),
		nullString(event.DeviceMAC), nullString(event.DeviceName),
		event.Position.X, event.Position.Y, event.Position.Z,
		event.HourOfWeek, event.ExpectedOccupancy,
		event.DwellDuration.Nanoseconds(), event.ExpectedDwell.Nanoseconds(),
		event.RSSIdBm, event.SeenBefore)
	if err != nil {
		log.Printf("[WARN] Failed to persist anomaly: %v", err)
	}
}

func (d *Detector) startAlertChain(event *events.AnomalyEvent, isSecurityMode bool) {
	state := &alertTimerState{
		anomalyID: event.ID,
	}

	// T+0: Dashboard alarm (immediate - handled by UI via callback)
	// Fire alert handler immediately for dashboard
	if d.alertHandler != nil {
		go d.alertHandler.SendAlert(*event, isSecurityMode)
	}

	if isSecurityMode {
		// Security mode: all alerts fire immediately
		if d.alertHandler != nil {
			d.alertHandler.SendWebhook(*event, true)
			d.alertHandler.SendEscalation(*event)
		}
		event.AlertSent = true
		event.WebhookSent = true
		event.EscalationSent = true
		now := time.Now()
		event.AlertSentAt = now
		event.WebhookSentAt = now
		event.EscalationSentAt = now
		d.updateAnomalyAlertState(event)
	} else {
		// Normal mode: staged alerts
		// T+30s: notification
		state.alertTimer = time.AfterFunc(30*time.Second, func() {
			d.mu.Lock()
			defer d.mu.Unlock()
			if anomaly, exists := d.activeAnomalies[event.ID]; exists && !anomaly.Acknowledged {
				if d.alertHandler != nil {
					d.alertHandler.SendAlert(*anomaly, false)
				}
				anomaly.AlertSent = true
				anomaly.AlertSentAt = time.Now()
				d.updateAnomalyAlertState(anomaly)
			}
		})

		// T+2min: webhook
		state.webhookTimer = time.AfterFunc(2*time.Minute, func() {
			d.mu.Lock()
			defer d.mu.Unlock()
			if anomaly, exists := d.activeAnomalies[event.ID]; exists && !anomaly.Acknowledged {
				if d.alertHandler != nil {
					d.alertHandler.SendWebhook(*anomaly, false)
				}
				anomaly.WebhookSent = true
				anomaly.WebhookSentAt = time.Now()
				d.updateAnomalyAlertState(anomaly)
			}
		})

		// T+5min: escalation
		state.escalationTimer = time.AfterFunc(5*time.Minute, func() {
			d.mu.Lock()
			defer d.mu.Unlock()
			if anomaly, exists := d.activeAnomalies[event.ID]; exists && !anomaly.Acknowledged {
				if d.alertHandler != nil {
					d.alertHandler.SendEscalation(*anomaly)
				}
				anomaly.EscalationSent = true
				anomaly.EscalationSentAt = time.Now()
				d.updateAnomalyAlertState(anomaly)
			}
		})
	}

	d.pendingAlerts[event.ID] = state
}

func (d *Detector) updateAnomalyAlertState(event *events.AnomalyEvent) {
	d.db.Exec(`
		UPDATE anomaly_events SET
			alert_sent = ?, alert_sent_at = ?,
			webhook_sent = ?, webhook_sent_at = ?,
			escalation_sent = ?, escalation_sent_at = ?
		WHERE id = ?
	`, event.AlertSent, nullTime(event.AlertSentAt),
		event.WebhookSent, nullTime(event.WebhookSentAt),
		event.EscalationSent, nullTime(event.EscalationSentAt),
		event.ID)
}

// AcknowledgeAnomaly acknowledges an anomaly and cancels pending timers.
func (d *Detector) AcknowledgeAnomaly(anomalyID, feedback, acknowledgedBy string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	event, exists := d.activeAnomalies[anomalyID]
	if !exists {
		return fmt.Errorf("anomaly not found: %s", anomalyID)
	}

	// Cancel pending timers
	if state, exists := d.pendingAlerts[anomalyID]; exists {
		if state.alertTimer != nil {
			state.alertTimer.Stop()
		}
		if state.webhookTimer != nil {
			state.webhookTimer.Stop()
		}
		if state.escalationTimer != nil {
			state.escalationTimer.Stop()
		}
		delete(d.pendingAlerts, anomalyID)
	}

	// Update event
	event.Acknowledged = true
	event.AcknowledgedAt = time.Now()
	event.Feedback = feedback
	event.AcknowledgedBy = acknowledgedBy

	// Update database
	_, err := d.db.Exec(`
		UPDATE anomaly_events SET
			acknowledged = 1,
			acknowledged_at = ?,
			feedback = ?
		WHERE id = ?
	`, event.AcknowledgedAt.UnixNano(), feedback, anomalyID)

	if err != nil {
		return err
	}

	// Record feedback to learning store for accuracy tracking
	if d.feedbackStore != nil && feedback != "" {
		feedbackType := mapFeedbackToLearningType(feedback)
		details := map[string]interface{}{
			"anomaly_type":    string(event.Type),
			"score":           event.Score,
			"zone_id":         event.ZoneID,
			"person_id":       event.PersonID,
			"device_mac":      event.DeviceMAC,
			"hour_of_week":    event.HourOfWeek,
			"acknowledged_by": acknowledgedBy,
		}
		if event.Position.X != 0 || event.Position.Y != 0 || event.Position.Z != 0 {
			details["position_x"] = event.Position.X
			details["position_y"] = event.Position.Y
			details["position_z"] = event.Position.Z
		}

		learningFeedback := learning.FeedbackRecord{
			ID:           uuid.New().String(),
			EventID:      anomalyID,
			EventType:    learning.Anomaly,
			FeedbackType: feedbackType,
			Details:      details,
			Timestamp:    time.Now(),
			Applied:      false,
		}

		if err := d.feedbackStore.RecordFeedback(learningFeedback); err != nil {
			log.Printf("[WARN] Failed to record anomaly feedback to learning store: %v", err)
		} else {
			log.Printf("[INFO] Recorded anomaly feedback: %s -> %s", anomalyID, feedbackType)
		}
	}

	log.Printf("[INFO] Anomaly acknowledged: %s (feedback: %s)", anomalyID, feedback)

	return nil
}

// mapFeedbackToLearningType maps anomaly feedback to learning feedback types.
func mapFeedbackToLearningType(feedback string) learning.FeedbackType {
	switch feedback {
	case "expected":
		return learning.FalsePositive // User says it was expected, so detection was wrong
	case "intrusion":
		return learning.TruePositive // User confirms it was real
	case "false_alarm":
		return learning.FalsePositive
	default:
		return learning.FalsePositive
	}
}

// UpdateBehaviourModel updates the behaviour model from collected samples.
// Should be called periodically (e.g., weekly).
func (d *Detector) UpdateBehaviourModel() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	log.Printf("[INFO] Updating behaviour model from collected samples...")

	// Update behaviour slots from occupancy samples
	// First, collect all slots into memory to avoid holding a query connection
	// while doing nested queries (deadlock with SetMaxOpenConns(1)).
	type aggSlot struct {
		HourOfWeek         int
		ZoneID             string
		ExpectedOccupancy  float64
		TypicalPersonCount float64
		SampleCount        int
	}
	rows, err := d.db.Query(`
		SELECT hour_of_week, zone_id,
		       AVG(CASE WHEN person_count > 0 THEN 1.0 ELSE 0.0 END) as expected_occupancy,
		       AVG(person_count) as typical_person_count,
		       COUNT(*) as sample_count
		FROM occupancy_samples
		GROUP BY hour_of_week, zone_id
	`)
	if err != nil {
		return err
	}

	var slots []aggSlot
	for rows.Next() {
		var s aggSlot
		if err := rows.Scan(&s.HourOfWeek, &s.ZoneID, &s.ExpectedOccupancy,
			&s.TypicalPersonCount, &s.SampleCount); err != nil {
			continue
		}
		slots = append(slots, s)
	}
	rows.Close()

	for _, s := range slots {
		slot := &NormalBehaviourSlot{
			TypicalBLEDevices: make(map[string]float64),
		}
		slot.HourOfWeek = s.HourOfWeek
		slot.ZoneID = s.ZoneID
		slot.ExpectedOccupancy = s.ExpectedOccupancy
		slot.TypicalPersonCount = s.TypicalPersonCount
		slot.SampleCount = s.SampleCount

		// Calculate typical BLE devices (seen in > 50% of this slot)
		bleRows, err := d.db.Query(`
			SELECT ble_devices FROM occupancy_samples
			WHERE hour_of_week = ? AND zone_id = ?
		`, slot.HourOfWeek, slot.ZoneID)
		if err == nil {
			deviceCounts := make(map[string]int)
			totalSamples := 0
			for bleRows.Next() {
				var devicesJSON string
				if err := bleRows.Scan(&devicesJSON); err != nil {
					continue
				}
				var devices []string
				if jsonUnmarshal(devicesJSON, &devices) == nil {
					totalSamples++
					for _, mac := range devices {
						deviceCounts[mac]++
					}
				}
			}
			bleRows.Close()

			// Only include devices seen > 50% of the time
			if totalSamples > 0 {
				for mac, count := range deviceCounts {
					frequency := float64(count) / float64(totalSamples)
					if frequency > 0.5 {
						slot.TypicalBLEDevices[mac] = frequency
					}
				}
			}
		}

		// Upsert to database
		devicesJSON, _ := jsonMarshal(slot.TypicalBLEDevices)
		d.db.Exec(`
			INSERT INTO behaviour_slots (hour_of_week, zone_id, expected_occupancy, typical_person_count, sample_count, typical_ble_devices)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(hour_of_week, zone_id) DO UPDATE SET
				expected_occupancy = excluded.expected_occupancy,
				typical_person_count = excluded.typical_person_count,
				sample_count = excluded.sample_count,
				typical_ble_devices = excluded.typical_ble_devices
		`, slot.HourOfWeek, slot.ZoneID, slot.ExpectedOccupancy,
			slot.TypicalPersonCount, slot.SampleCount, string(devicesJSON))

		key := fmt.Sprintf("%d-%s", slot.HourOfWeek, slot.ZoneID)
		d.behaviourSlots[key] = slot
	}

	// Update dwell slots
	dwellRows, err := d.db.Query(`
		SELECT hour_of_week, zone_id, person_id,
		       AVG(dwell_ns) as mean_dwell_ns,
		       SQRT(MAX(0, AVG(dwell_ns * dwell_ns) - AVG(dwell_ns) * AVG(dwell_ns))) as std_dwell_ns,
		       COUNT(*) as sample_count
		FROM dwell_samples
		GROUP BY hour_of_week, zone_id, person_id
	`)
	if err != nil {
		return err
	}
	defer dwellRows.Close()

	for dwellRows.Next() {
		slot := &DwellBehaviourSlot{}
		var meanNS, stdNS int64
		if err := dwellRows.Scan(&slot.HourOfWeek, &slot.ZoneID, &slot.PersonID,
			&meanNS, &stdNS, &slot.SampleCount); err != nil {
			continue
		}
		slot.MeanDwellDuration = time.Duration(meanNS)
		slot.StdDwellDuration = time.Duration(stdNS)

		d.db.Exec(`
			INSERT INTO dwell_slots (hour_of_week, zone_id, person_id, mean_dwell_ns, std_dwell_ns, sample_count)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(hour_of_week, zone_id, person_id) DO UPDATE SET
				mean_dwell_ns = excluded.mean_dwell_ns,
				std_dwell_ns = excluded.std_dwell_ns,
				sample_count = excluded.sample_count
		`, slot.HourOfWeek, slot.ZoneID, slot.PersonID,
			slot.MeanDwellDuration.Nanoseconds(), slot.StdDwellDuration.Nanoseconds(), slot.SampleCount)

		key := fmt.Sprintf("%d-%s-%s", slot.HourOfWeek, slot.ZoneID, slot.PersonID)
		d.dwellSlots[key] = slot
	}

	// Check if model should become ready
	if !d.modelReady && time.Since(d.learningStartTime) >= 7*24*time.Hour {
		d.modelReady = true
		d.modelReadyAt = time.Now()
		log.Printf("[INFO] Behaviour model is now ready after 7 days of learning")
	}

	log.Printf("[INFO] Behaviour model updated: %d occupancy slots, %d dwell slots",
		len(d.behaviourSlots), len(d.dwellSlots))

	return nil
}

// GetActiveAnomalies returns all unacknowledged anomalies.
func (d *Detector) GetActiveAnomalies() []*events.AnomalyEvent {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make([]*events.AnomalyEvent, 0, len(d.activeAnomalies))
	for _, event := range d.activeAnomalies {
		if !event.Acknowledged {
			result = append(result, event)
		}
	}
	return result
}

// GetAnomalyHistory returns recent anomaly events from memory.
func (d *Detector) GetAnomalyHistory(limit int) []*events.AnomalyEvent {
	d.mu.RLock()
	history := d.anomalyHistory
	d.mu.RUnlock()

	if len(history) <= limit {
		return history
	}
	return history[len(history)-limit:]
}

// QueryAnomalyEvents queries persisted anomaly events from the database.
// This works across server restarts unlike GetAnomalyHistory which is in-memory only.
func (d *Detector) QueryAnomalyEvents(since time.Time, limit int) ([]*events.AnomalyEvent, error) {
	rows, err := d.db.Query(`
		SELECT id, type, score, description, timestamp,
		       zone_id, zone_name, blob_id, person_id, person_name,
		       device_mac, device_name, position_x, position_y, position_z,
		       acknowledged
		FROM anomaly_events
		WHERE timestamp >= ?
		ORDER BY timestamp DESC
		LIMIT ?
	`, since.UnixNano(), limit)
	if err != nil {
		return nil, fmt.Errorf("query anomaly events: %w", err)
	}
	defer rows.Close()

	var result []*events.AnomalyEvent
	for rows.Next() {
		var e events.AnomalyEvent
		var tsNS int64
		var acknowledged int
		if err := rows.Scan(&e.ID, &e.Type, &e.Score, &e.Description, &tsNS,
			&e.ZoneID, &e.ZoneName, &e.BlobID,
			&e.PersonID, &e.PersonName,
			&e.DeviceMAC, &e.DeviceName,
			&e.Position.X, &e.Position.Y, &e.Position.Z,
			&acknowledged); err != nil {
			continue
		}
		e.Timestamp = time.Unix(0, tsNS)
		e.Acknowledged = acknowledged == 1
		result = append(result, &e)
	}
	return result, rows.Err()
}

// CountAnomaliesSince returns the count of anomaly events since the given time.
func (d *Detector) CountAnomaliesSince(since time.Time) (int, error) {
	var count int
	err := d.db.QueryRow(
		`SELECT COUNT(*) FROM anomaly_events WHERE timestamp >= ?`,
		since.UnixNano(),
	).Scan(&count)
	return count, err
}

// GetWeeklySummary returns a summary of anomalies for the past week.
func (d *Detector) GetWeeklySummary() events.WeeklyAnomalySummary {
	d.mu.RLock()
	defer d.mu.RUnlock()

	summary := events.WeeklyAnomalySummary{
		ByType: make(map[events.AnomalyType]int),
	}

	oneWeekAgo := time.Now().Add(-7 * 24 * time.Hour)

	for _, event := range d.anomalyHistory {
		if event.Timestamp.Before(oneWeekAgo) {
			continue
		}

		summary.TotalAnomalies++
		summary.ByType[event.Type]++

		if event.Acknowledged {
			switch event.Feedback {
			case "expected":
				summary.ExpectedEvents++
			case "intrusion":
				summary.GenuineIntrusions++
			case "false_alarm":
				summary.FalseAlarms++
			}
		} else {
			summary.Unacknowledged++
		}
	}

	return summary
}

// ClearAnomaly removes an anomaly from active state.
func (d *Detector) ClearAnomaly(anomalyID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Cancel timers
	if state, exists := d.pendingAlerts[anomalyID]; exists {
		if state.alertTimer != nil {
			state.alertTimer.Stop()
		}
		if state.webhookTimer != nil {
			state.webhookTimer.Stop()
		}
		if state.escalationTimer != nil {
			state.escalationTimer.Stop()
		}
		delete(d.pendingAlerts, anomalyID)
	}

	// Move to history
	if event, exists := d.activeAnomalies[anomalyID]; exists {
		d.anomalyHistory = append(d.anomalyHistory, event)
		delete(d.activeAnomalies, anomalyID)
	}
}

// RunPeriodicUpdate starts a goroutine that updates the behaviour model periodically.
func (d *Detector) RunPeriodicUpdate(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := d.UpdateBehaviourModel(); err != nil {
					log.Printf("[WARN] Failed to update behaviour model: %v", err)
				}
			}
		}
	}()
}

// getHourOfWeek returns the hour of the week (0-167) for a given time.
func getHourOfWeek(t time.Time) int {
	weekday := int(t.Weekday())
	hour := t.Hour()
	return weekday*24 + hour
}

func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullTime(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t.UnixNano()
}

// JSON helpers using standard library
func jsonMarshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func jsonUnmarshal(data string, v interface{}) error {
	return json.Unmarshal([]byte(data), v)
}

// Math helper
var _ = math.E // Use math package to avoid unused import error
