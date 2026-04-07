// Package sleep provides sleep record persistence against the main spaxel.db.
package sleep

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"time"
)

// SleepRecord represents a row in the sleep_records table (main spaxel.db).
type SleepRecord struct {
	ID                   int64     `json:"id"`
	Person               string    `json:"person,omitempty"`
	ZoneID               *int      `json:"zone_id,omitempty"`
	Date                 string    `json:"date"`
	BedTimeMs            *int64    `json:"bed_time_ms,omitempty"`
	WakeTimeMs           *int64    `json:"wake_time_ms,omitempty"`
	DurationMin          *int      `json:"duration_min,omitempty"`
	OnsetLatencyMin      *float64  `json:"onset_latency_min,omitempty"`
	Restlessness         *float64  `json:"restlessness,omitempty"`
	BreathingRateAvg     *float64  `json:"breathing_rate_avg,omitempty"`
	BreathingRegularity  *float64  `json:"breathing_regularity,omitempty"`
	BreathingAnomaly     *bool     `json:"breathing_anomaly,omitempty"`
	BreathingSamplesJSON *string   `json:"breathing_samples_json,omitempty"`
	SummaryJSON          *string   `json:"summary_json,omitempty"`
}

// SleepRecordStore handles persistence of sleep records against the main DB.
type SleepRecordStore struct {
	db *sql.DB
}

// NewSleepRecordStore creates a store backed by an open main DB connection.
func NewSleepRecordStore(db *sql.DB) *SleepRecordStore {
	return &SleepRecordStore{db: db}
}

// Save persists a sleep report as a sleep_records row.
// If a record for the same (person, date) already exists it is replaced.
func (s *SleepRecordStore) Save(person string, report *SleepReport) error {
	m := report.Metrics
	dateStr := report.SessionDate.Format("2006-01-02")

	var durationMin *int
	if report.Metrics.TimeInBed > 0 {
		d := int(report.Metrics.TimeInBed.Minutes())
		durationMin = &d
	}

	var onsetLat *float64
	if m.SleepLatencyMinutes > 0 {
		onsetLat = &m.SleepLatencyMinutes
	}

	var restlessness *float64
	if m.RestlessPeriods > 0 {
		timeInBedH := m.TimeInBed.Hours()
		if timeInBedH > 0 {
			r := math.Min(5.0, float64(m.RestlessPeriods)/timeInBedH)
			restlessness = &r
		}
	}

	var breathingAvg *float64
	if m.AvgBreathingRate > 0 {
		breathingAvg = &m.AvgBreathingRate
	}

	var breathingReg *float64
	if m.BreathingRegularity > 0 {
		breathingReg = &m.BreathingRegularity
	}

	// Build 30-min summary JSON
	summaryBytes, _ := json.Marshal(report.ToJSONMap())
	summaryStr := string(summaryBytes)

	// Breathing samples as JSON array of BPM values
	var samplesJSON *string
	if samples := extractBreathingSamplesJSON(report); samples != "" {
		samplesJSON = &samples
	}

	bedMs := toMsPtr(report.Metrics.SleepStartTime)
	wakeMs := toMsPtr(report.Metrics.SleepEndTime)

	// Upsert: replace existing record for same person+date
	_, err := s.db.Exec(`
		INSERT INTO sleep_records (person, date, bed_time_ms, wake_time_ms, duration_min,
			onset_latency_min, restlessness, breathing_rate_avg, breathing_regularity,
			breathing_anomaly, breathing_samples_json, summary_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(person, date) DO UPDATE SET
			bed_time_ms = excluded.bed_time_ms,
			wake_time_ms = excluded.wake_time_ms,
			duration_min = excluded.duration_min,
			onset_latency_min = excluded.onset_latency_min,
			restlessness = excluded.restlessness,
			breathing_rate_avg = excluded.breathing_rate_avg,
			breathing_regularity = excluded.breathing_regularity,
			breathing_anomaly = excluded.breathing_anomaly,
			breathing_samples_json = excluded.breathing_samples_json,
			summary_json = excluded.summary_json
	`, person, dateStr, bedMs, wakeMs, durationMin, onsetLat, restlessness,
		breathingAvg, breathingReg, m.BreathingAnomaly, samplesJSON, summaryStr)

	return err
}

// extractBreathingSamplesJSON builds a JSON object with per-night breathing statistics
// and the raw per-sample BPM values collected throughout the ASLEEP state.
func extractBreathingSamplesJSON(report *SleepReport) string {
	m := report.Metrics
	if m.AvgBreathingRate == 0 && m.MinBreathingRate == 0 && m.MaxBreathingRate == 0 {
		return ""
	}

	samples := map[string]interface{}{
		"avg":        m.AvgBreathingRate,
		"min":        m.MinBreathingRate,
		"max":        m.MaxBreathingRate,
		"std_dev":    m.BreathingRateStdDev,
		"regularity": m.BreathingRegularity,
		"anomaly":    m.BreathingAnomaly,
	}
	if m.PersonalAvgBPM > 0 {
		samples["personal_avg"] = m.PersonalAvgBPM
	}
	// Include raw per-sample BPM values
	if len(report.BreathingSamples) > 0 {
		samples["rates"] = report.BreathingSamples
	}

	b, err := json.Marshal(samples)
	if err != nil {
		return ""
	}
	return string(b)
}

// Query retrieves sleep records, optionally filtered by person, with a limit.
func (s *SleepRecordStore) Query(person string, limit int) ([]SleepRecord, error) {
	query := `SELECT id, person, zone_id, date, bed_time_ms, wake_time_ms, duration_min,
	                  onset_latency_min, restlessness, breathing_rate_avg, breathing_regularity,
	                  breathing_anomaly, breathing_samples_json, summary_json
	           FROM sleep_records`
	var args []interface{}
	if person != "" {
		query += ` WHERE person = ?`
		args = append(args, person)
	}
	query += ` ORDER BY date DESC`
	if limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanSleepRecords(rows)
}

// GetSummary returns the most recent sleep record for a person.
func (s *SleepRecordStore) GetSummary(person string) (*SleepRecord, error) {
	if person == "" {
		return nil, fmt.Errorf("person parameter required")
	}

	query := `SELECT id, person, zone_id, date, bed_time_ms, wake_time_ms, duration_min,
	                  onset_latency_min, restlessness, breathing_rate_avg, breathing_regularity,
	                  breathing_anomaly, breathing_samples_json, summary_json
	           FROM sleep_records WHERE person = ? ORDER BY date DESC LIMIT 1`

	row := s.db.QueryRow(query, person)
	rec := SleepRecord{}
	var zoneID sql.NullInt64
	var bedMs, wakeMs sql.NullInt64
	var durMin sql.NullInt32
	var onsetLat, restless, breathAvg, breathReg sql.NullFloat64
	var breathAnomaly sql.NullBool
	var breathSamplesJSON, summaryJSON sql.NullString

	err := row.Scan(&rec.ID, &rec.Person, &zoneID, &rec.Date, &bedMs, &wakeMs,
		&durMin, &onsetLat, &restless, &breathAvg, &breathReg,
		&breathAnomaly, &breathSamplesJSON, &summaryJSON)
	if err != nil {
		return nil, err
	}

	assignNullableFields(&rec, zoneID, bedMs, wakeMs, durMin, onsetLat, restless,
		breathAvg, breathReg, breathAnomaly, breathSamplesJSON, summaryJSON)
	return &rec, nil
}

// GetLatestAnomalyRecords returns records where breathing_anomaly is true, ordered by date desc.
func (s *SleepRecordStore) GetLatestAnomalyRecords(limit int) ([]SleepRecord, error) {
	query := `SELECT id, person, zone_id, date, bed_time_ms, wake_time_ms, duration_min,
	                  onset_latency_min, restlessness, breathing_rate_avg, breathing_regularity,
	                  breathing_anomaly, breathing_samples_json, summary_json
	           FROM sleep_records WHERE breathing_anomaly = 1
	           ORDER BY date DESC`
	if limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, limit)
	}

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanSleepRecords(rows)
}

func scanSleepRecords(rows *sql.Rows) ([]SleepRecord, error) {
	var records []SleepRecord
	for rows.Next() {
		rec := SleepRecord{}
		var zoneID sql.NullInt64
		var bedMs, wakeMs sql.NullInt64
		var durMin sql.NullInt32
		var onsetLat, restless, breathAvg, breathReg sql.NullFloat64
		var breathAnomaly sql.NullBool
		var breathSamplesJSON, summaryJSON sql.NullString

		err := rows.Scan(&rec.ID, &rec.Person, &zoneID, &rec.Date, &bedMs, &wakeMs,
			&durMin, &onsetLat, &restless, &breathAvg, &breathReg,
			&breathAnomaly, &breathSamplesJSON, &summaryJSON)
		if err != nil {
			return records, err
		}

		assignNullableFields(&rec, zoneID, bedMs, wakeMs, durMin, onsetLat, restless,
			breathAvg, breathReg, breathAnomaly, breathSamplesJSON, summaryJSON)
		records = append(records, rec)
	}
	return records, rows.Err()
}

func assignNullableFields(rec *SleepRecord, zoneID sql.NullInt64,
	bedMs, wakeMs sql.NullInt64, durMin sql.NullInt32,
	onsetLat, restless, breathAvg, breathReg sql.NullFloat64,
	breathAnomaly sql.NullBool, breathSamplesJSON, summaryJSON sql.NullString) {

	if zoneID.Valid {
		z := int(zoneID.Int64)
		rec.ZoneID = &z
	}
	if bedMs.Valid {
		rec.BedTimeMs = &bedMs.Int64
	}
	if wakeMs.Valid {
		rec.WakeTimeMs = &wakeMs.Int64
	}
	if durMin.Valid {
		d := int(durMin.Int32)
		rec.DurationMin = &d
	}
	if onsetLat.Valid {
		rec.OnsetLatencyMin = &onsetLat.Float64
	}
	if restless.Valid {
		rec.Restlessness = &restless.Float64
	}
	if breathAvg.Valid {
		rec.BreathingRateAvg = &breathAvg.Float64
	}
	if breathReg.Valid {
		rec.BreathingRegularity = &breathReg.Float64
	}
	if breathAnomaly.Valid {
		rec.BreathingAnomaly = &breathAnomaly.Bool
	}
	if breathSamplesJSON.Valid {
		rec.BreathingSamplesJSON = &breathSamplesJSON.String
	}
	if summaryJSON.Valid {
		rec.SummaryJSON = &summaryJSON.String
	}
}

func toMsPtr(t time.Time) *int64 {
	if t.IsZero() {
		return nil
	}
	ms := t.UnixMilli()
	return &ms
}
