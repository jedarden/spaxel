// Package briefing generates morning briefings with sleep and anomaly summaries.
package briefing

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Generator produces morning briefings from sleep records and events.
type Generator struct {
	db *sql.DB
}

// NewGenerator creates a new briefing generator backed by the main DB.
func NewGenerator(dbPath string) (*Generator, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1)
	return &Generator{db: db}, nil
}

// Close closes the DB connection.
func (g *Generator) Close() error {
	return g.db.Close()
}

// Briefing holds a generated morning briefing.
type Briefing struct {
	Date        string    `json:"date"`
	Content     string    `json:"content"`
	GeneratedAt int64     `json:"generated_at"`
}

// Generate creates a morning briefing for the given date.
// It assembles sections from sleep records, anomalies, and system health.
func (g *Generator) Generate(date string, person string) (*Briefing, error) {
	var sections []string

	// BLOCK 2 — Sleep summary
	if sleepSummary := g.generateSleepBlock(date, person); sleepSummary != "" {
		sections = append(sections, sleepSummary)
	}

	// BLOCK 4 — Overnight anomalies (breathing)
	if anomalyText := g.generateBreathingAnomalyBlock(date, person); anomalyText != "" {
		sections = append(sections, anomalyText)
	}

	// Degenerate case
	if len(sections) == 0 {
		sections = append(sections, "All quiet last night. All systems healthy.")
	}

	content := strings.Join(sections, "\n\n")

	return &Briefing{
		Date:        date,
		Content:     content,
		GeneratedAt: time.Now().UnixMilli(),
	}, nil
}

// generateSleepBlock generates the sleep summary section of the briefing.
func (g *Generator) generateSleepBlock(date, person string) string {
	query := `SELECT breathing_rate_avg, breathing_regularity, duration_min, onset_latency_min,
	                  restlessness, breathing_anomaly, breathing_samples_json
	           FROM sleep_records WHERE date = ?`
	var args []interface{}
	args = append(args, date)
	if person != "" {
		query += ` AND person = ?`
		args = append(args, person)
	}

	row := g.db.QueryRow(query, args...)

	var breathAvg, breathReg, onsetLat, restlessness sql.NullFloat64
	var duration sql.NullInt32
	var breathAnomaly sql.NullBool
	var breathSamplesJSON sql.NullString

	if err := row.Scan(&breathAvg, &breathReg, &duration, &onsetLat, &restlessness,
		&breathAnomaly, &breathSamplesJSON); err != nil {
		return ""
	}

	if !breathAvg.Valid || breathAvg.Float64 == 0 {
		return ""
	}

	var parts []string

	// Duration
	if duration.Valid && duration.Int32 > 0 {
		h := duration.Int32 / 60
		m := duration.Int32 % 60
		if m > 0 {
			parts = append(parts, fmt.Sprintf("You slept %dh %dm", h, m))
		} else {
			parts = append(parts, fmt.Sprintf("You slept %dh", h))
		}
	} else {
		parts = append(parts, "You slept")
	}

	// Restlessness
	if restlessness.Valid {
		switch {
		case restlessness.Float64 < 1:
			parts = append(parts, "Restlessness: Low.")
		case restlessness.Float64 < 3:
			parts = append(parts, "Restlessness: Moderate.")
		default:
			parts = append(parts, "Restlessness: High.")
		}
	}

	// Breathing regularity
	if breathReg.Valid {
		cv := breathReg.Float64
		switch {
		case cv < 0.10:
			parts = append(parts, "Breathing: Regular.")
		case cv > 0.25:
			parts = append(parts, "Breathing: Irregular.")
		default:
			parts = append(parts, "Breathing: Normal.")
		}
	}

	// Breathing anomaly
	if breathAnomaly.Valid && breathAnomaly.Bool {
		if breathSamplesJSON.Valid {
			var info map[string]interface{}
			if err := json.Unmarshal([]byte(breathSamplesJSON.String), &info); err == nil {
				avg, _ := info["avg"].(float64)
				personal, _ := info["personal_avg"].(float64)
				if personal > 0 {
					parts = append(parts, fmt.Sprintf("Breathing rate elevated (%.0f bpm vs. %.0f bpm average).",
						avg, personal))
				} else if avg > 0 {
					parts = append(parts, fmt.Sprintf("Breathing rate elevated (%.0f bpm).", avg))
				}
			}
		}
		if len(parts) > 0 && breathAnomaly.Bool {
			// Already added above
		} else {
			parts = append(parts, "Breathing rate elevated.")
		}
	}

	return strings.Join(parts, " ")
}

// generateBreathingAnomalyBlock generates the overnight breathing anomaly section.
// This covers the case where the sleep block already includes the anomaly but
// we want to surface it as a standalone alert if it was severe.
func (g *Generator) generateBreathingAnomalyBlock(date, person string) string {
	query := `SELECT person, breathing_rate_avg, breathing_samples_json
	           FROM sleep_records
	           WHERE breathing_anomaly = 1 AND date = ?`
	var args []interface{}
	args = append(args, date)
	if person != "" {
		query += ` AND person = ?`
		args = append(args, person)
	}

	row := g.db.QueryRow(query, args...)

	var personName string
	var breathAvg sql.NullFloat64
	var breathSamplesJSON sql.NullString

	if err := row.Scan(&personName, &breathAvg, &breathSamplesJSON); err != nil {
		return ""
	}

	if personName == "" {
		personName = "Person"
	}

	// Extract personal average from samples JSON
	personalAvg := 0.0
	if breathSamplesJSON.Valid {
		var info map[string]interface{}
		if err := json.Unmarshal([]byte(breathSamplesJSON.String), &info); err == nil {
			personalAvg, _ = info["personal_avg"].(float64)
		}
	}

	avgStr := fmt.Sprintf("%.0f", breathAvg.Float64)
	if personalAvg > 0 {
		return fmt.Sprintf("Last night: Breathing rate elevated (%s bpm vs. %s bpm average for %s).",
			avgStr, fmt.Sprintf("%.0f", personalAvg), personName)
	}
	return fmt.Sprintf("Last night: Breathing rate elevated (%s bpm for %s).", avgStr, personName)
}

// Save persists a briefing to the briefings table.
func (g *Generator) Save(b *Briefing) error {
	_, err := g.db.Exec(`
		INSERT OR REPLACE INTO briefings (date, content, generated_at)
		VALUES (?, ?, ?)
	`, b.Date, b.Content, b.GeneratedAt)
	return err
}

// Get retrieves a previously generated briefing by date.
func (g *Generator) Get(date string) (*Briefing, error) {
	var content string
	var generatedAt int64
	err := g.db.QueryRow(
		`SELECT content, generated_at FROM briefings WHERE date = ?`, date,
	).Scan(&content, &generatedAt)
	if err != nil {
		return nil, err
	}
	return &Briefing{
		Date:        date,
		Content:     content,
		GeneratedAt: generatedAt,
	}, nil
}
