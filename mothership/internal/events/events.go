// Package events provides event types for the spaxel system.
package events

import (
	"time"
)

// AnomalyType represents the type of anomaly detected.
type AnomalyType string

const (
	AnomalyUnusualHour       AnomalyType = "unusual_hour"        // Motion at unusual time
	AnomalyUnknownBLE        AnomalyType = "unknown_ble"         // Unknown BLE device nearby
	AnomalyMotionDuringAway  AnomalyType = "motion_during_away"  // Motion when system is in away mode
	AnomalyUnusualDwell      AnomalyType = "unusual_dwell"       // Person in zone longer than usual
)

// AnomalyEvent represents a detected anomaly.
type AnomalyEvent struct {
	ID           string      `json:"id"`
	Type         AnomalyType `json:"type"`
	Score        float64     `json:"score"`         // 0.0 - 1.0, higher = more anomalous
	Description  string      `json:"description"`   // Human-readable description
	Timestamp    time.Time   `json:"timestamp"`

	// Context for the anomaly
	ZoneID       string      `json:"zone_id,omitempty"`
	ZoneName     string      `json:"zone_name,omitempty"`
	BlobID       int         `json:"blob_id,omitempty"`
	PersonID     string      `json:"person_id,omitempty"`
	PersonName   string      `json:"person_name,omitempty"`
	DeviceMAC    string      `json:"device_mac,omitempty"`
	DeviceName   string      `json:"device_name,omitempty"`
	Position     Position    `json:"position,omitempty"`

	// For unusual hour anomalies
	HourOfWeek    int     `json:"hour_of_week,omitempty"`   // 0-167
	ExpectedOccupancy float64 `json:"expected_occupancy,omitempty"` // 0.0-1.0

	// For unusual dwell anomalies
	DwellDuration time.Duration `json:"dwell_duration,omitempty"`
	ExpectedDwell time.Duration `json:"expected_dwell,omitempty"`

	// For BLE anomalies
	RSSIdBm       int    `json:"rssi_dbm,omitempty"`
	SeenBefore    bool   `json:"seen_before,omitempty"` // Was this device seen before (even if not regular)

	// Acknowledgement state
	Acknowledged  bool      `json:"acknowledged"`
	AcknowledgedAt time.Time `json:"acknowledged_at,omitempty"`
	Feedback      string    `json:"feedback,omitempty"` // "expected", "intrusion", "false_alarm"
	AcknowledgedBy string   `json:"acknowledged_by,omitempty"` // User who acknowledged

	// Alert chain state
	AlertSent       bool      `json:"alert_sent"`
	WebhookSent     bool      `json:"webhook_sent"`
	EscalationSent  bool      `json:"escalation_sent"`
	AlertSentAt     time.Time `json:"alert_sent_at,omitempty"`
	WebhookSentAt   time.Time `json:"webhook_sent_at,omitempty"`
	EscalationSentAt time.Time `json:"escalation_sent_at,omitempty"`
}

// Position represents a 3D position.
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// SystemMode represents the system operating mode.
type SystemMode string

const (
	ModeHome  SystemMode = "home"
	ModeAway  SystemMode = "away"
	ModeSleep SystemMode = "sleep"
)

// SystemModeChangeEvent represents a change in system mode.
type SystemModeChangeEvent struct {
	PreviousMode SystemMode `json:"previous_mode"`
	NewMode      SystemMode `json:"new_mode"`
	Reason       string     `json:"reason"` // "auto_away", "auto_disarm", "manual"
	Timestamp    time.Time  `json:"timestamp"`
	PersonID     string     `json:"person_id,omitempty"`     // For auto-disarm
	PersonName   string     `json:"person_name,omitempty"`  // For auto-disarm
}

// AnomalyFeedback is used for recording user feedback on anomalies.
type AnomalyFeedback struct {
	AnomalyID string    `json:"anomaly_id"`
	Feedback  string    `json:"feedback"` // "expected", "intrusion", "false_alarm"
	Timestamp time.Time `json:"timestamp"`
	Notes     string    `json:"notes,omitempty"`
}

// WeeklyAnomalySummary provides a summary of anomalies for the past week.
type WeeklyAnomalySummary struct {
	TotalAnomalies    int `json:"total_anomalies"`
	FalseAlarms       int `json:"false_alarms"`
	GenuineIntrusions int `json:"genuine_intrusions"`
	ExpectedEvents    int `json:"expected_events"`
	Unacknowledged    int `json:"unacknowledged"`
	ByType            map[AnomalyType]int `json:"by_type"`
}

// OccupancySample represents a single occupancy observation for behaviour modelling.
type OccupancySample struct {
	HourOfWeek     int       `json:"hour_of_week"`      // 0-167 (hour of week)
	ZoneID         string    `json:"zone_id"`
	PersonCount    int       `json:"person_count"`
	BLEDevices     []string  `json:"ble_devices,omitempty"`
	Timestamp      time.Time `json:"timestamp"`
}

// DwellSample represents a dwell duration observation for behaviour modelling.
type DwellSample struct {
	HourOfWeek     int           `json:"hour_of_week"`
	ZoneID         string        `json:"zone_id"`
	PersonID       string        `json:"person_id,omitempty"`
	DwellDuration  time.Duration `json:"dwell_duration"`
	Timestamp      time.Time    `json:"timestamp"`
}

// SleepSessionStartEvent represents the start of a sleep session.
type SleepSessionStartEvent struct {
	SessionID    string    `json:"session_id"`
	PersonID     string    `json:"person_id,omitempty"`
	ZoneID       string    `json:"zone_id"`
	LinkID       string    `json:"link_id"`
	TentativeStart time.Time `json:"tentative_start"` // When conditions first met
	ConfirmedStart time.Time `json:"confirmed_start"` // When 15-min confirmation elapsed
	SessionDate  string    `json:"session_date"`     // Date this sleep night belongs to
	Timestamp    time.Time `json:"timestamp"`
}

// SleepSessionEndEvent represents the end of a sleep session.
type SleepSessionEndEvent struct {
	SessionID           string        `json:"session_id"`
	PersonID            string        `json:"person_id,omitempty"`
	ZoneID              string        `json:"zone_id"`
	LinkID              string        `json:"link_id"`
	SessionDate         string        `json:"session_date"`
	SleepOnset          time.Time     `json:"sleep_onset"`
	WakeTime            time.Time     `json:"wake_time"`
	TimeInBed           time.Duration `json:"time_in_bed"`
	WakeEpisodeCount    int           `json:"wake_episode_count"`
	WASODuration        time.Duration `json:"waso_duration"`
	SleepEfficiency     float64       `json:"sleep_efficiency"`      // 0-100
	AvgBreathingRate    float64       `json:"avg_breathing_rate"`   // BPM
	BreathingAnomalyCount int         `json:"breathing_anomaly_count"`
	OverallScore        float64       `json:"overall_score"`        // 0-100
	QualityRating       string        `json:"quality_rating"`       // poor/fair/good/excellent
	EndReason           string        `json:"end_reason"`           // "zone_exit", "sustained_motion", "stationary_drop"
	Timestamp           time.Time     `json:"timestamp"`
}

// MorningSummaryEvent represents the morning sleep summary pushed to dashboards.
type MorningSummaryEvent struct {
	SessionID           string        `json:"session_id"`
	PersonID            string        `json:"person_id,omitempty"`
	PersonName          string        `json:"person_name,omitempty"`
	SessionDate         string        `json:"session_date"`
	SleepDuration       time.Duration `json:"sleep_duration"`
	SleepEfficiency     float64       `json:"sleep_efficiency"`
	EfficiencyColor     string        `json:"efficiency_color"`     // "green", "amber", "red"
	WakeEpisodeCount    int           `json:"wake_episode_count"`
	WASODuration        time.Duration `json:"waso_duration"`
	AvgBreathingRate    float64       `json:"avg_breathing_rate"`
	BreathingAnomalyCount int         `json:"breathing_anomaly_count"`
	AnomalyNote         string        `json:"anomaly_note,omitempty"`
	OverallScore        float64       `json:"overall_score"`
	QualityRating       string        `json:"quality_rating"`
	SleepOnset          time.Time     `json:"sleep_onset"`
	WakeTime            time.Time     `json:"wake_time"`
	GeneratedAt         time.Time     `json:"generated_at"`
}
