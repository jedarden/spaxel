// Package briefing generates morning briefings with sleep, anomaly, and system summaries.
package briefing

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// Generator produces morning briefings from sleep records, events, and system state.
type Generator struct {
	db                 *sql.DB
	zoneProvider       ZoneProvider
	personProvider     PersonProvider
	predictionProvider PredictionProvider
	healthProvider     HealthProvider
	nodeProvider       NodeInfoProvider
	weatherAPIURL      string // Optional weather API URL
	quietHoursStart    int    // Hour when quiet hours start (default 22 = 10pm)
	quietHoursEnd      int    // Hour when quiet hours end (default 6 = 6am)
}

// ZoneProvider provides zone information.
type ZoneProvider interface {
	GetZoneName(id int) string
	GetZoneOccupancy(zoneID int) int
	GetPeopleInZone(zoneID int) []string
}

// PersonProvider provides person information.
type PersonProvider interface {
	GetPeopleHome() []string
	GetPersonLastSeen(person string) time.Time
	GetPersonZone(person string) string
}

// PredictionProvider provides prediction information.
type PredictionProvider interface {
	GetPrediction(person string, horizonMinutes int) (zone string, probability float64, ok bool)
	GetDaysComplete(person string) int
	IsModelReady(person string) bool
}

// HealthProvider provides system health information.
type HealthProvider interface {
	GetDetectionQuality() float64
	GetNodeCount() (online, total int)
	GetAccuracyDelta() (percent float64, feedbackCount int)
	GetNodeOfflineDuration(mac string) time.Duration
}

// NodeInfoProvider provides node information for system health section.
type NodeInfoProvider interface {
	GetNodeName(mac string) string
	GetAllNodeMACs() []string
}

// NewGenerator creates a new briefing generator backed by the main DB.
func NewGenerator(dbPath string) (*Generator, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1)

	// Check for weather API URL in settings
	var weatherURL string
	row := db.QueryRow("SELECT value_json FROM settings WHERE key = 'weather_api_url'")
	var weatherURLJSON sql.NullString
	if err := row.Scan(&weatherURLJSON); err == nil && weatherURLJSON.Valid {
		weatherURL = weatherURLJSON.String
		// Unwrap if it's JSON
		if strings.HasPrefix(weatherURL, `"`) {
			var url string
			json.Unmarshal([]byte(weatherURL), &url)
			weatherURL = url
		}
	}

	return &Generator{
		db:              db,
		weatherAPIURL:   weatherURL,
		quietHoursStart: 22, // 10pm
		quietHoursEnd:   6,  // 6am
	}, nil
}

// SetQuietHours sets the quiet hours range for overnight events.
func (g *Generator) SetQuietHours(start, end int) {
	g.quietHoursStart = start
	g.quietHoursEnd = end
}

// SetWeatherAPIURL sets the weather API URL for weather section.
func (g *Generator) SetWeatherAPIURL(url string) {
	g.weatherAPIURL = url
}

// Close closes the DB connection.
func (g *Generator) Close() error {
	return g.db.Close()
}

// SetProviders sets the provider interfaces for briefing generation.
func (g *Generator) SetProviders(z ZoneProvider, p PersonProvider, pr PredictionProvider, h HealthProvider) {
	g.zoneProvider = z
	g.personProvider = p
	g.predictionProvider = pr
	g.healthProvider = h
}

// SetNodeInfoProvider sets the node info provider.
func (g *Generator) SetNodeInfoProvider(n NodeInfoProvider) {
	g.nodeProvider = n
}

// DailyBriefing is the primary struct for a morning briefing.
// Alias for Briefing with additional delivery tracking fields.
type DailyBriefing = Briefing

// BriefingSectionType defines the type of briefing section.
type BriefingSectionType string

const (
	SectionTypeSleep           BriefingSectionType = "sleep"
	SectionTypeOvernightEvents BriefingSectionType = "overnight_events"
	SectionTypeSystemHealth    BriefingSectionType = "system_health"
	SectionTypePredictions     BriefingSectionType = "predictions"
	SectionTypeWeather         BriefingSectionType = "weather"
	SectionTypeAlert           BriefingSectionType = "alert"
	SectionTypePeople          BriefingSectionType = "people"
	SectionTypeAnomaly         BriefingSectionType = "anomaly"
	SectionTypeLearning        BriefingSectionType = "learning"
)

// Briefing holds a generated morning briefing.
type Briefing struct {
	ID           string            `json:"id"` // UUID
	Date         string            `json:"date"`
	Person       string            `json:"person,omitempty"`
	Content      string            `json:"content"`
	GeneratedAt  int64             `json:"generated_at"`
	Sections     []Section         `json:"sections,omitempty"`
	Delivered    bool              `json:"delivered"`          // Set true after first push
	Acknowledged bool              `json:"acknowledged"`       // Set true when user dismisses
	Metadata     map[string]string `json:"metadata,omitempty"` // Additional metadata
}

// Section represents a single section of the briefing.
type Section struct {
	Type     BriefingSectionType `json:"type"`
	Content  string              `json:"content"`
	Priority int                 `json:"priority"`           // Higher = shown first
	Severity string              `json:"severity,omitempty"` // For alerts: "info", "warning", "error"
}

// Generate creates a morning briefing for the given date and person.
// If person is empty, generates a household-wide briefing.
func (g *Generator) Generate(date string, person string) (*Briefing, error) {
	var sections []Section

	// Parse date for calculations
	dateTime, err := time.Parse("2006-01-02", date)
	if err != nil {
		return nil, fmt.Errorf("parse date: %w", err)
	}

	// Calculate time range for "last night" (18:00 yesterday to now)
	nightStart := time.Date(dateTime.Year(), dateTime.Month(), dateTime.Day()-1, 18, 0, 0, 0, time.Local)
	nightEnd := dateTime

	// BLOCK 1 — Critical alerts (fall, security)
	if alertSection := g.generateAlertBlock(nightStart, nightEnd, person); alertSection != nil {
		sections = append(sections, *alertSection)
	}

	// BLOCK 2 — Sleep summary
	if sleepSection := g.generateSleepBlock(date, person); sleepSection != nil {
		sections = append(sections, *sleepSection)
	}

	// BLOCK 3 — Who is home (current state)
	if peopleSection := g.generatePeopleBlock(person); peopleSection != nil {
		sections = append(sections, *peopleSection)
	}

	// BLOCK 4 — Overnight events (replaces anomaly block with more detailed filtering)
	if overnightSection := g.generateOvernightEventsBlock(nightStart, nightEnd, person); overnightSection != nil {
		sections = append(sections, *overnightSection)
	}

	// BLOCK 5 — System health
	if healthSection := g.generateHealthBlock(); healthSection != nil {
		sections = append(sections, *healthSection)
	}

	// BLOCK 6 — Weather (optional)
	if weatherSection := g.generateWeatherBlock(); weatherSection != nil {
		sections = append(sections, *weatherSection)
	}

	// BLOCK 7 — Prediction hint
	if predictionSection := g.generatePredictionBlock(person); predictionSection != nil {
		sections = append(sections, *predictionSection)
	}

	// BLOCK 8 — Learning progress
	if learningSection := g.generateLearningBlock(); learningSection != nil {
		sections = append(sections, *learningSection)
	}

	// Degenerate case
	if len(sections) == 0 {
		sections = append(sections, Section{
			Type:     "info",
			Content:  "All quiet last night. All systems healthy.",
			Priority: 0,
		})
	}

	// Sort by priority descending
	for i := 0; i < len(sections)-1; i++ {
		for j := i + 1; j < len(sections); j++ {
			if sections[j].Priority > sections[i].Priority {
				sections[i], sections[j] = sections[j], sections[i]
			}
		}
	}

	// Build content from prioritized sections
	contentParts := make([]string, 0, len(sections))
	for _, s := range sections {
		contentParts = append(contentParts, s.Content)
	}
	content := strings.Join(contentParts, "\n\n")

	return &Briefing{
		ID:           uuid.New().String(),
		Date:         date,
		Person:       person,
		Content:      content,
		GeneratedAt:  time.Now().UnixMilli(),
		Sections:     sections,
		Delivered:    false,
		Acknowledged: false,
	}, nil
}

// generateAlertBlock generates BLOCK 1 — Critical alerts.
func (g *Generator) generateAlertBlock(nightStart, nightEnd time.Time, person string) *Section {
	query := `SELECT type, zone, person, detail_json, severity
	           FROM events
	           WHERE timestamp_ms >= ? AND timestamp_ms < ?
	             AND type IN ('fall_alert', 'security_alert')
	             AND severity IN ('alert', 'critical')`
	args := []interface{}{nightStart.UnixMilli(), nightEnd.UnixMilli()}

	if person != "" {
		query += ` AND person = ?`
		args = append(args, person)
	}
	query += ` ORDER BY timestamp_ms ASC LIMIT 5`

	log.Printf("[DEBUG] Alert query: nightStart=%s (%d), nightEnd=%s (%d), person=%s",
		nightStart, nightStart.UnixMilli(), nightEnd, nightEnd.UnixMilli(), person)

	rows, err := g.db.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var alerts []string
	for rows.Next() {
		var eventType, zone, personName, detailJSON, severity string
		if err := rows.Scan(&eventType, &zone, &personName, &detailJSON, &severity); err != nil {
			continue
		}

		var alert strings.Builder
		switch eventType {
		case "fall_alert":
			alert.WriteString("⚠ Fall detected")
			if personName != "" {
				alert.WriteString(": ")
				alert.WriteString(personName)
			}
			if zone != "" {
				alert.WriteString(" in ")
				alert.WriteString(zone)
			}
		case "security_alert":
			alert.WriteString("⚠ Security alert")
			if zone != "" {
				alert.WriteString(": Motion in ")
				alert.WriteString(zone)
			}
		}
		alerts = append(alerts, alert.String())
	}

	if len(alerts) == 0 {
		return nil
	}

	content := "⚠ " + strings.Join(alerts, "; ")
	if len(alerts) > 1 {
		content = fmt.Sprintf("⚠ %d critical events overnight. ", len(alerts)) + strings.Join(alerts, "; ")
	}

	return &Section{
		Type:     "alert",
		Content:  content,
		Priority: 100,
	}
}

// generateSleepBlock generates BLOCK 2 — Sleep summary.
func (g *Generator) generateSleepBlock(date, person string) *Section {
	query := `SELECT duration_min, onset_latency_min, restlessness,
	                 breathing_rate_avg, breathing_regularity, breathing_anomaly,
	                 breathing_samples_json, person
	           FROM sleep_records WHERE date = ?`
	args := []interface{}{date}
	if person != "" {
		query += ` AND person = ?`
		args = append(args, person)
	}

	rows, err := g.db.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var sleepRecords []struct {
		Duration      sql.NullInt32
		OnsetLatency  sql.NullFloat64
		Restlessness  sql.NullFloat64
		BreathAvg     sql.NullFloat64
		BreathReg     sql.NullFloat64
		BreathAnomaly sql.NullBool
		BreathSamples sql.NullString
		Person        sql.NullString
	}

	for rows.Next() {
		var r struct {
			Duration      sql.NullInt32
			OnsetLatency  sql.NullFloat64
			Restlessness  sql.NullFloat64
			BreathAvg     sql.NullFloat64
			BreathReg     sql.NullFloat64
			BreathAnomaly sql.NullBool
			BreathSamples sql.NullString
			Person        sql.NullString
		}
		if err := rows.Scan(&r.Duration, &r.OnsetLatency, &r.Restlessness,
			&r.BreathAvg, &r.BreathReg, &r.BreathAnomaly, &r.BreathSamples, &r.Person); err != nil {
			continue
		}
		sleepRecords = append(sleepRecords, r)
	}

	if len(sleepRecords) == 0 {
		return nil
	}

	// For multi-person, aggregate or pick the primary record
	// For now, use the first record
	r := sleepRecords[0]

	var parts []string
	personName := "You"
	if r.Person.Valid && r.Person.String != "" {
		personName = r.Person.String
	}

	// Duration and deviation from average
	if r.Duration.Valid && r.Duration.Int32 > 0 {
		h := r.Duration.Int32 / 60
		m := r.Duration.Int32 % 60
		if m > 0 {
			parts = append(parts, fmt.Sprintf("%s slept %dh %dm", personName, h, m))
		} else {
			parts = append(parts, fmt.Sprintf("%s slept %dh", personName, h))
		}

		// Compare with average (get from recent records)
		avgDuration := g.getAverageSleepDuration(r.Person.String)
		if avgDuration > 0 {
			delta := int(r.Duration.Int32) - avgDuration
			if math.Abs(float64(delta)) >= 10 {
				if delta > 0 {
					parts[len(parts)-1] += fmt.Sprintf(" — %d minutes more than your average", delta)
				} else {
					parts[len(parts)-1] += fmt.Sprintf(" — %d minutes less than your average", -delta)
				}
			}
		}
	} else {
		parts = append(parts, personName+" slept")
	}

	// Restlessness
	if r.Restlessness.Valid {
		switch {
		case r.Restlessness.Float64 < 1:
			parts = append(parts, "Restlessness: Low.")
		case r.Restlessness.Float64 < 3:
			parts = append(parts, "Restlessness: Moderate.")
		default:
			parts = append(parts, "Restlessness: High.")
		}
	}

	// Breathing regularity
	if r.BreathReg.Valid {
		cv := r.BreathReg.Float64
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
	if r.BreathAnomaly.Valid && r.BreathAnomaly.Bool {
		if r.BreathSamples.Valid {
			var info map[string]interface{}
			if err := json.Unmarshal([]byte(r.BreathSamples.String), &info); err == nil {
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
	}

	if len(parts) == 0 {
		return nil
	}

	return &Section{
		Type:     "sleep",
		Content:  strings.Join(parts, " "),
		Priority: 80,
	}
}

// generatePeopleBlock generates BLOCK 3 — Who is home.
func (g *Generator) generatePeopleBlock(person string) *Section {
	if g.personProvider == nil {
		return nil
	}

	peopleHome := g.personProvider.GetPeopleHome()
	if len(peopleHome) == 0 {
		return &Section{
			Type:     "people",
			Content:  "The house is currently empty.",
			Priority: 60,
		}
	}

	var content string
	if len(peopleHome) == 1 {
		content = fmt.Sprintf("%s is home.", peopleHome[0])
	} else {
		content = fmt.Sprintf("%s are home.", strings.Join(peopleHome, ", "))
	}

	// Add information about who left when
	// This would need event history, for now skip
	return &Section{
		Type:     "people",
		Content:  content,
		Priority: 60,
	}
}

// generateAnomalyBlock generates BLOCK 4 — Overnight anomalies.
func (g *Generator) generateAnomalyBlock(nightStart, nightEnd time.Time, person string) *Section {
	query := `SELECT type, zone, detail_json, timestamp_ms
	           FROM events
	           WHERE timestamp_ms >= ? AND timestamp_ms < ?
	             AND type IN ('anomaly', 'unusual_activity')
	             AND severity IN ('warning', 'alert')
	           ORDER BY timestamp_ms ASC`
	args := []interface{}{nightStart.UnixMilli(), nightEnd.UnixMilli()}

	if person != "" {
		query += ` AND person = ?`
		args = append(args, person)
	}
	query += ` LIMIT 3`

	rows, err := g.db.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var anomalies []string
	for rows.Next() {
		var eventType, zone, detailJSON string
		var timestamp int64
		if err := rows.Scan(&eventType, &zone, &detailJSON, &timestamp); err != nil {
			continue
		}

		// Parse anomaly score from detail_json
		var detail map[string]interface{}
		var score float64
		if err := json.Unmarshal([]byte(detailJSON), &detail); err == nil {
			if s, ok := detail["score"].(float64); ok {
				score = s
			}
		}

		timeStr := time.Unix(0, timestamp*1e6).Format("3:04pm")
		var anomaly strings.Builder
		if zone != "" {
			anomaly.WriteString(fmt.Sprintf("Motion in %s at %s", zone, timeStr))
		} else {
			anomaly.WriteString(fmt.Sprintf("Unusual activity at %s", timeStr))
		}

		if score >= 0.85 {
			anomaly.WriteString(". High-confidence.")
		} else if score < 0.7 {
			anomaly.WriteString(". Likely environmental.")
		}

		anomalies = append(anomalies, anomaly.String())
	}

	if len(anomalies) == 0 {
		return nil
	}

	var content string
	if len(anomalies) == 1 {
		content = "Last night: " + anomalies[0]
	} else {
		content = fmt.Sprintf("Last night: %d unusual events. Most notable: ", len(anomalies)) + anomalies[0]
	}

	return &Section{
		Type:     "anomaly",
		Content:  content,
		Priority: 70,
	}
}

// generateHealthBlock generates BLOCK 5 — System health.
func (g *Generator) generateHealthBlock() *Section {
	if g.healthProvider == nil {
		return nil
	}

	quality := g.healthProvider.GetDetectionQuality()
	online, total := g.healthProvider.GetNodeCount()

	// Build content with node health details
	var contentParts []string

	contentParts = append(contentParts, fmt.Sprintf("%d nodes healthy.", online))

	// Check for offline nodes with duration
	if g.nodeProvider != nil {
		allMACs := g.nodeProvider.GetAllNodeMACs()
		for _, mac := range allMACs {
			// Get offline duration from health provider
			if g.healthProvider != nil {
				duration := g.healthProvider.GetNodeOfflineDuration(mac)
				if duration > 0 {
					name := g.nodeProvider.GetNodeName(mac)
					if name == "" {
						name = mac
					}

					// Format duration
					durationH := int(duration.Hours())
					durationM := int(duration.Minutes()) % 60
					if durationH > 0 {
						if durationM > 0 {
							contentParts = append(contentParts, fmt.Sprintf("Node %s has been offline for %dh %dm.", name, durationH, durationM))
						} else {
							contentParts = append(contentParts, fmt.Sprintf("Node %s has been offline for %dh.", name, durationH))
						}
					} else if durationM > 0 {
						contentParts = append(contentParts, fmt.Sprintf("Node %s has been offline for %dmin.", name, durationM))
					}
				}
			}
		}
	}

	// Add detection quality if not excellent
	if quality < 90 {
		contentParts = append(contentParts, fmt.Sprintf("Detection quality: %.0f%%.", quality))
	}

	// Skip if everything is healthy
	if len(contentParts) == 1 && online == total && quality >= 90 {
		return nil
	}

	content := strings.Join(contentParts, " ")

	return &Section{
		Type:     SectionTypeSystemHealth,
		Content:  content,
		Priority: 30,
	}
}

// generatePredictionBlock generates BLOCK 6 — Prediction hint.
func (g *Generator) generatePredictionBlock(person string) *Section {
	if g.predictionProvider == nil {
		return nil
	}

	// Get prediction for next action (15 min horizon)
	zone, probability, ok := g.predictionProvider.GetPrediction(person, 15)
	if !ok || probability < 0.7 {
		return nil
	}

	// Format day of week
	now := time.Now()
	dayOfWeek := now.Weekday().String()

	// Find what action this prediction suggests
	content := fmt.Sprintf("Today's forecast: Based on your %s pattern, you'll likely be in %s in 15 minutes (%.0f%% confidence).",
		dayOfWeek, zone, probability*100)

	return &Section{
		Type:     "prediction",
		Content:  content,
		Priority: 40,
	}
}

// generateLearningBlock generates BLOCK 7 — Learning progress.
func (g *Generator) generateLearningBlock() *Section {
	if g.healthProvider == nil {
		return nil
	}

	delta, feedbackCount := g.healthProvider.GetAccuracyDelta()
	if feedbackCount == 0 {
		return nil
	}

	var content string
	if delta > 0 {
		content = fmt.Sprintf("Accuracy improved %.0f%% this week thanks to your %d corrections.",
			delta, feedbackCount)
	} else {
		content = fmt.Sprintf("You provided %d corrections this week.", feedbackCount)
	}

	return &Section{
		Type:     SectionTypeLearning,
		Content:  content,
		Priority: 20,
	}
}

// generateOvernightEventsBlock generates the overnight events section.
// Filters for FallDetected, AnomalyDetected, NodeDisconnected events during quiet hours.
func (g *Generator) generateOvernightEventsBlock(nightStart, nightEnd time.Time, person string) *Section {
	// Calculate quiet hours period
	quietStart := time.Date(nightEnd.Year(), nightEnd.Month(), nightEnd.Day(), g.quietHoursStart, 0, 0, 0, time.Local)
	quietEnd := time.Date(nightEnd.Year(), nightEnd.Month(), nightEnd.Day()+1, g.quietHoursEnd, 0, 0, 0, time.Local)

	query := `SELECT type, zone, person, detail_json, timestamp_ms, severity
	           FROM events
	           WHERE timestamp_ms >= ? AND timestamp_ms < ?
		             AND type IN ('fall_alert', 'anomaly', 'node_disconnected')
		             AND severity IN ('warning', 'alert', 'critical')
	           ORDER BY timestamp_ms ASC
	           LIMIT 6`

	args := []interface{}{quietStart.UnixMilli(), quietEnd.UnixMilli()}

	if person != "" {
		query += ` AND person = ?`
		args = append(args, person)
	}

	rows, err := g.db.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var events []struct {
		Type       string
		Zone       string
		Person     string
		DetailJSON string
		Timestamp  int64
		Severity   string
		Acked      bool
	}

	for rows.Next() {
		var e struct {
			Type       string
			Zone       string
			Person     string
			DetailJSON string
			Timestamp  int64
			Severity   string
			Acked      bool
		}
		if err := rows.Scan(&e.Type, &e.Zone, &e.Person, &e.DetailJSON, &e.Timestamp, &e.Severity); err != nil {
			continue
		}

		// Check if acknowledged from detail_json
		var detail map[string]interface{}
		if err := json.Unmarshal([]byte(e.DetailJSON), &detail); err == nil {
			if acked, ok := detail["acknowledged"].(bool); ok {
				e.Acked = acked
			}
		}

		events = append(events, e)
	}

	if len(events) == 0 {
		return &Section{
			Type:     SectionTypeOvernightEvents,
			Content:  "No incidents overnight.",
			Priority: 40,
		}
	}

	var contentParts []string
	for i, e := range events {
		var eventStr strings.Builder
		timeStr := time.Unix(0, e.Timestamp*1e6).Format("3:04pm")

		switch e.Type {
		case "fall_alert":
			eventStr.WriteString(fmt.Sprintf("Possible fall detected at %s", timeStr))
			if e.Person != "" {
				eventStr.WriteString(fmt.Sprintf(" for %s", e.Person))
			}
			if e.Zone != "" {
				eventStr.WriteString(fmt.Sprintf(" in %s", e.Zone))
			}
			if e.Acked {
				eventStr.WriteString(" (acknowledged)")
			}

		case "anomaly":
			eventStr.WriteString(fmt.Sprintf("Anomaly detected at %s", timeStr))
			if e.Zone != "" {
				eventStr.WriteString(fmt.Sprintf(" in %s", e.Zone))
			}
			if e.Acked {
				eventStr.WriteString(" (acknowledged)")
			}

		case "node_disconnected":
			eventStr.WriteString(fmt.Sprintf("Node %s went offline at %s", e.Zone, timeStr))
			// Try to get reconnection time
			var reconnectTime sql.NullInt64
			reconnectQuery := `SELECT timestamp_ms FROM events
			                   WHERE type = 'node_connected' AND zone = ?
			                   AND timestamp_ms > ? ORDER BY timestamp_ms ASC LIMIT 1`
			err := g.db.QueryRow(reconnectQuery, e.Zone, e.Timestamp).Scan(&reconnectTime)
			if err == nil && reconnectTime.Valid {
				reconnectStr := time.Unix(0, reconnectTime.Int64*1e6).Format("3:04pm")
				eventStr.WriteString(fmt.Sprintf(" and reconnected at %s", reconnectStr))
			}
		}

		contentParts = append(contentParts, eventStr.String())

		// Limit to 5 events
		if i >= 4 && len(events) > 5 {
			contentParts = append(contentParts, fmt.Sprintf("...and %d more events.", len(events)-5))
			break
		}
	}

	content := strings.Join(contentParts, ". ")
	if len(contentParts) > 1 {
		content += "."
	}

	return &Section{
		Type:     SectionTypeOvernightEvents,
		Content:  content,
		Priority: 75,
	}
}

// generateWeatherBlock generates the optional weather section.
func (g *Generator) generateWeatherBlock() *Section {
	if g.weatherAPIURL == "" {
		return nil
	}

	// Fetch weather from wttr.in
	// Format: GET https://wttr.in/{location}?format=%t+%C
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(g.weatherAPIURL)
	if err != nil {
		log.Printf("[WARN] Failed to fetch weather: %v", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[WARN] Weather API returned status %d", resp.StatusCode)
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[WARN] Failed to read weather response: %v", err)
		return nil
	}

	weather := strings.TrimSpace(string(body))
	if weather == "" || weather == "Unknown" {
		return nil
	}

	return &Section{
		Type:     SectionTypeWeather,
		Content:  fmt.Sprintf("Outside: %s", weather),
		Priority: 15,
	}
}

// getAverageSleepDuration calculates average sleep duration over the past 7 days.
func (g *Generator) getAverageSleepDuration(person string) int {
	query := `SELECT AVG(duration_min) FROM sleep_records
	           WHERE date >= date('now', '-7 days')`
	args := []interface{}{}
	if person != "" {
		query += ` AND person = ?`
		args = append(args, person)
	}

	var avg sql.NullFloat64
	err := g.db.QueryRow(query, args...).Scan(&avg)
	if err != nil || !avg.Valid {
		return 0
	}
	return int(avg.Float64)
}

// Save persists a briefing to the briefings table.
func (g *Generator) Save(b *Briefing) error {
	// Check which columns exist in the briefings table
	var personColExists, sectionsJSONColExists bool

	// Check for person column
	err := g.db.QueryRow(`
		SELECT COUNT(*) > 0 FROM pragma_table_info('briefings') WHERE name = 'person'
	`).Scan(&personColExists)
	if err != nil {
		return fmt.Errorf("check person column: %w", err)
	}

	// Check for sections_json column
	err = g.db.QueryRow(`
		SELECT COUNT(*) > 0 FROM pragma_table_info('briefings') WHERE name = 'sections_json'
	`).Scan(&sectionsJSONColExists)
	if err != nil {
		return fmt.Errorf("check sections_json column: %w", err)
	}

	// Build query dynamically based on available columns
	if personColExists && sectionsJSONColExists {
		// Marshal sections to JSON if present
		var sectionsJSON sql.NullString
		if len(b.Sections) > 0 {
			data, err := json.Marshal(b.Sections)
			if err == nil {
				sectionsJSON = sql.NullString{String: string(data), Valid: true}
			}
		}

		_, err = g.db.Exec(`
			INSERT OR REPLACE INTO briefings (date, person, content, generated_at, sections_json)
			VALUES (?, ?, ?, ?, ?)
		`, b.Date, b.Person, b.Content, b.GeneratedAt, sectionsJSON)
		return err
	}

	// Fallback for old schema without person and sections_json
	_, err = g.db.Exec(`
		INSERT OR REPLACE INTO briefings (date, content, generated_at)
		VALUES (?, ?, ?)
	`, b.Date, b.Content, b.GeneratedAt)
	return err
}

// Get retrieves a previously generated briefing by date and optional person.
func (g *Generator) Get(date string, person string) (*Briefing, error) {
	// First, try to query with sections_json (new schema)
	var content string
	var generatedAt int64
	var personVal sql.NullString
	var sectionsJSON sql.NullString

	query := `SELECT content, generated_at, person, sections_json FROM briefings WHERE date = ?`
	args := []interface{}{date}

	if person != "" {
		query += ` AND person = ?`
		args = append(args, person)
	} else {
		query += ` AND (person IS NULL OR person = '')`
	}

	err := g.db.QueryRow(query, args...).Scan(&content, &generatedAt, &personVal, &sectionsJSON)
	if err != nil {
		// If the query fails, it might be because sections_json column doesn't exist
		// Try fallback query without sections_json
		query = `SELECT content, generated_at FROM briefings WHERE date = ?`
		args = []interface{}{date}

		if person != "" {
			query += ` AND person = ?`
			args = append(args, person)
		} else {
			query += ` AND (person IS NULL OR person = '')`
		}

		var content2 string
		var generatedAt2 int64
		err2 := g.db.QueryRow(query, args...).Scan(&content2, &generatedAt2)
		if err2 != nil {
			return nil, err2
		}
		content = content2
		generatedAt = generatedAt2
		// personVal and sectionsJSON remain NULL/invalid
	}

	b := &Briefing{
		Date:        date,
		Person:      personVal.String,
		Content:     content,
		GeneratedAt: generatedAt,
	}

	// Unmarshal sections if present
	if sectionsJSON.Valid {
		if err := json.Unmarshal([]byte(sectionsJSON.String), &b.Sections); err != nil {
			log.Printf("[WARN] Failed to unmarshal sections for %s: %v", date, err)
		}
	}

	return b, nil
}

// GetLatest retrieves the most recent briefing (for any person).
func (g *Generator) GetLatest() (*Briefing, error) {
	var date, person, content string
	var generatedAt int64
	var sectionsJSON sql.NullString

	// Try new schema first
	err := g.db.QueryRow(`
		SELECT date, person, content, generated_at, sections_json
		FROM briefings
		ORDER BY generated_at DESC
		LIMIT 1
	`).Scan(&date, &person, &content, &generatedAt, &sectionsJSON)

	if err != nil {
		// Fallback to old schema without sections_json
		err = g.db.QueryRow(`
			SELECT date, content, generated_at
			FROM briefings
			ORDER BY generated_at DESC
			LIMIT 1
		`).Scan(&date, &content, &generatedAt)
		if err != nil {
			return nil, err
		}
		// person and sectionsJSON remain empty/invalid
	}

	b := &Briefing{
		Date:        date,
		Person:      person,
		Content:     content,
		GeneratedAt: generatedAt,
	}

	// Unmarshal sections if present
	if sectionsJSON.Valid {
		if err := json.Unmarshal([]byte(sectionsJSON.String), &b.Sections); err != nil {
			log.Printf("[WARN] Failed to unmarshal sections for latest briefing: %v", err)
		}
	}

	return b, nil
}

// ShouldGenerate checks if a briefing should be generated for the given date.
// Returns true if no briefing exists for this date yet.
func (g *Generator) ShouldGenerate(date string, person string) bool {
	var count int
	query := `SELECT COUNT(*) FROM briefings WHERE date = ?`
	args := []interface{}{date}

	if person != "" {
		query += ` AND (person = ? OR person IS NULL OR person = '')`
		args = append(args, person)
	}

	err := g.db.QueryRow(query, args...).Scan(&count)
	return err == nil && count == 0
}

// MarkDelivered marks a briefing as delivered.
func (g *Generator) MarkDelivered(id string) error {
	// Check if delivered column exists
	var deliveredColExists bool
	err := g.db.QueryRow(`
		SELECT COUNT(*) > 0 FROM pragma_table_info('briefings') WHERE name = 'delivered'
	`).Scan(&deliveredColExists)
	if err != nil {
		return fmt.Errorf("check delivered column: %w", err)
	}

	if !deliveredColExists {
		// Column doesn't exist yet, skip
		return nil
	}

	_, err = g.db.Exec(`UPDATE briefings SET delivered = 1 WHERE id = ?`, id)
	return err
}

// MarkAcknowledged marks a briefing as acknowledged by the user.
func (g *Generator) MarkAcknowledged(id string) error {
	// Check if acknowledged column exists
	var acknowledgedColExists bool
	err := g.db.QueryRow(`
		SELECT COUNT(*) > 0 FROM pragma_table_info('briefings') WHERE name = 'acknowledged'
	`).Scan(&acknowledgedColExists)
	if err != nil {
		return fmt.Errorf("check acknowledged column: %w", err)
	}

	if !acknowledgedColExists {
		// Column doesn't exist yet, skip
		return nil
	}

	_, err = g.db.Exec(`UPDATE briefings SET acknowledged = 1 WHERE id = ?`, id)
	return err
}

// GetTodayBriefing returns today's briefing as a map for the dashboard.
func (g *Generator) GetTodayBriefing() (map[string]interface{}, error) {
	today := time.Now().Format("2006-01-02")

	b, err := g.Get(today, "")
	if err != nil {
		// No briefing exists, try generating one
		b, err = g.Generate(today, "")
		if err != nil {
			return nil, err
		}
		// Save the new briefing
		if err := g.Save(b); err != nil {
			log.Printf("[WARN] Failed to save briefing: %v", err)
		}
	}

	// Convert to map for JSON marshaling
	result := map[string]interface{}{
		"id":           b.ID,
		"date":         b.Date,
		"content":      b.Content,
		"generated_at": b.GeneratedAt,
		"delivered":    b.Delivered,
		"acknowledged": b.Acknowledged,
	}

	if len(b.Sections) > 0 {
		result["sections"] = b.Sections
	}

	return result, nil
}

// ShouldPushBriefing checks if the briefing should be pushed to clients.
// Returns true if it's after 6am and the briefing hasn't been delivered yet.
func (g *Generator) ShouldPushBriefing() bool {
	now := time.Now()
	hour := now.Hour()

	// Only push after 6am
	if hour < 6 {
		return false
	}

	today := now.Format("2006-01-02")

	// Check if a briefing exists for today
	var count int
	err := g.db.QueryRow(`SELECT COUNT(*) FROM briefings WHERE date = ?`, today).Scan(&count)
	if err != nil || count == 0 {
		return false
	}

	// Check if delivered column exists
	var deliveredColExists bool
	err = g.db.QueryRow(`
		SELECT COUNT(*) > 0 FROM pragma_table_info('briefings') WHERE name = 'delivered'
	`).Scan(&deliveredColExists)
	if err != nil || !deliveredColExists {
		// If column doesn't exist, assume not delivered
		return true
	}

	// Check if already delivered
	var delivered int
	err = g.db.QueryRow(`SELECT delivered FROM briefings WHERE date = ?`, today).Scan(&delivered)
	if err != nil {
		return true
	}

	return delivered == 0
}
