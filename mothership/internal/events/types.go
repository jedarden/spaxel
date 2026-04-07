// Package events provides domain event types used across subsystems.
package events

import "time"

// AnomalyType classifies different kinds of anomalies.
type AnomalyType string

const (
	AnomalyUnusualHour      AnomalyType = "unusual_hour"
	AnomalyUnknownBLE       AnomalyType = "unknown_ble"
	AnomalyMotionDuringAway AnomalyType = "motion_during_away"
	AnomalyUnusualDwell     AnomalyType = "unusual_dwell"
)

// Position represents a 3D spatial position in meters.
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// AnomalyEvent represents a detected anomaly with full metadata.
type AnomalyEvent struct {
	ID                string    `json:"id"`
	Type              AnomalyType `json:"type"`
	Score             float64   `json:"score"`
	Description       string    `json:"description"`
	Timestamp         time.Time `json:"timestamp"`
	ZoneID            string    `json:"zone_id,omitempty"`
	ZoneName          string    `json:"zone_name,omitempty"`
	BlobID            int       `json:"blob_id,omitempty"`
	PersonID          string    `json:"person_id,omitempty"`
	PersonName        string    `json:"person_name,omitempty"`
	DeviceMAC         string    `json:"device_mac,omitempty"`
	DeviceName        string    `json:"device_name,omitempty"`
	Position          Position  `json:"position,omitempty"`
	HourOfWeek        int       `json:"hour_of_week,omitempty"`
	ExpectedOccupancy float64   `json:"expected_occupancy,omitempty"`
	DwellDuration     time.Duration `json:"dwell_duration,omitempty"`
	ExpectedDwell     time.Duration `json:"expected_dwell,omitempty"`
	RSSIdBm           int       `json:"rssi_dbm,omitempty"`
	SeenBefore        bool      `json:"seen_before,omitempty"`
	Acknowledged      bool      `json:"acknowledged"`
	AcknowledgedAt    time.Time `json:"acknowledged_at,omitempty"`
	Feedback          string    `json:"feedback,omitempty"`
	AcknowledgedBy    string    `json:"acknowledged_by,omitempty"`
	AlertSent         bool      `json:"alert_sent"`
	AlertSentAt       time.Time `json:"alert_sent_at,omitempty"`
	WebhookSent       bool      `json:"webhook_sent"`
	WebhookSentAt     time.Time `json:"webhook_sent_at,omitempty"`
	EscalationSent    bool      `json:"escalation_sent"`
	EscalationSentAt  time.Time `json:"escalation_sent_at,omitempty"`
}

// WeeklyAnomalySummary aggregates anomaly counts for the past week.
type WeeklyAnomalySummary struct {
	TotalAnomalies     int                `json:"total_anomalies"`
	ByType             map[AnomalyType]int `json:"by_type"`
	ExpectedEvents     int                `json:"expected_events"`
	GenuineIntrusions  int                `json:"genuine_intrusions"`
	FalseAlarms        int                `json:"false_alarms"`
	Unacknowledged     int                `json:"unacknowledged"`
}

// SystemMode represents the current home occupancy mode.
type SystemMode string

const (
	ModeHome  SystemMode = "home"
	ModeAway  SystemMode = "away"
	ModeSleep SystemMode = "sleep"
)

// SystemModeChangeEvent is emitted when the system mode changes.
type SystemModeChangeEvent struct {
	PreviousMode SystemMode `json:"previous_mode"`
	NewMode      SystemMode `json:"new_mode"`
	Reason       string     `json:"reason"`
	Timestamp    time.Time  `json:"timestamp"`
	PersonID     string     `json:"person_id,omitempty"`
	PersonName   string     `json:"person_name,omitempty"`
}

// SleepSessionStartEvent is emitted when a sleep session is detected.
type SleepSessionStartEvent struct {
	ZoneID      string    `json:"zone_id"`
	PersonID    string    `json:"person_id,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
	BlobID      int       `json:"blob_id,omitempty"`
}

// SleepSessionEndEvent is emitted when a sleep session ends.
type SleepSessionEndEvent struct {
	ZoneID          string        `json:"zone_id"`
	PersonID        string        `json:"person_id,omitempty"`
	StartTimestamp   time.Time     `json:"start_timestamp"`
	EndTimestamp     time.Time     `json:"end_timestamp"`
	DurationMin      float64       `json:"duration_min"`
	BlobID          int           `json:"blob_id,omitempty"`
}
