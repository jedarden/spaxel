package sleep

import (
	"fmt"
	"time"
)

// SleepReport represents a complete sleep quality report
type SleepReport struct {
	LinkID      string       `json:"link_id"`
	SessionDate time.Time    `json:"session_date"`
	GeneratedAt time.Time    `json:"generated_at"`
	Metrics     *SleepMetrics `json:"metrics"`

	// Summary text
	BreathingSummary string   `json:"breathing_summary"`
	MotionSummary    string   `json:"motion_summary"`
	Recommendations  []string `json:"recommendations"`
}

// generateBreathingSummary creates a human-readable breathing analysis summary
func generateBreathingSummary(m *SleepMetrics) string {
	if m.AvgBreathingRate == 0 {
		return "No breathing data available for this session."
	}

	var summary string

	// Average rate assessment
	switch {
	case m.AvgBreathingRate < BreathingRateLow:
		summary = fmt.Sprintf("Your average breathing rate was %.1f breaths per minute, which is below the typical range. ", m.AvgBreathingRate)
	case m.AvgBreathingRate > BreathingRateHigh:
		summary = fmt.Sprintf("Your average breathing rate was %.1f breaths per minute, which is above the typical range. ", m.AvgBreathingRate)
	case m.AvgBreathingRate >= BreathingRateOptimal-2 && m.AvgBreathingRate <= BreathingRateOptimal+2:
		summary = fmt.Sprintf("Your average breathing rate was %.1f breaths per minute, which is optimal for restful sleep. ", m.AvgBreathingRate)
	default:
		summary = fmt.Sprintf("Your average breathing rate was %.1f breaths per minute, which is within normal range. ", m.AvgBreathingRate)
	}

	// Variability assessment
	if m.BreathingRateStdDev > 3 {
		summary += fmt.Sprintf("Your breathing showed high variability (std dev: %.1f), which may indicate restless sleep.", m.BreathingRateStdDev)
	} else if m.BreathingRateStdDev > 1.5 {
		summary += fmt.Sprintf("Your breathing showed moderate variability (std dev: %.1f).", m.BreathingRateStdDev)
	} else {
		summary += "Your breathing was steady throughout the night."
	}

	// Range info
	if m.MaxBreathingRate > 0 {
		summary += fmt.Sprintf(" Range: %.1f-%.1f BPM.", m.MinBreathingRate, m.MaxBreathingRate)
	}

	return summary
}

// generateMotionSummary creates a human-readable motion analysis summary
func generateMotionSummary(m *SleepMetrics) string {
	if m.TimeInBed == 0 {
		return "No motion data available for this session."
	}

	var summary string

	// Quiet time assessment
	switch {
	case m.QuietTimePct >= 80:
		summary = fmt.Sprintf("You had a very restful night with %.0f%% quiet time. ", m.QuietTimePct)
	case m.QuietTimePct >= 60:
		summary = fmt.Sprintf("You had a moderately restful night with %.0f%% quiet time. ", m.QuietTimePct)
	default:
		summary = fmt.Sprintf("Your night was somewhat restless with only %.0f%% quiet time. ", m.QuietTimePct)
	}

	// Motion events
	if m.MotionEvents > 0 {
		if m.MotionEvents > 20 {
			summary += fmt.Sprintf("There were %d motion events detected, indicating significant movement. ", m.MotionEvents)
		} else if m.MotionEvents > 5 {
			summary += fmt.Sprintf("There were %d motion events detected during the night. ", m.MotionEvents)
		} else {
			summary += fmt.Sprintf("Only %d motion events were detected. ", m.MotionEvents)
		}
	}

	// Restless periods
	if m.RestlessPeriods > 0 {
		summary += fmt.Sprintf("%d restless period(s) were identified.", m.RestlessPeriods)
	} else {
		summary += "No significant restless periods were detected."
	}

	return summary
}

// generateRecommendations creates sleep improvement recommendations
func generateRecommendations(m *SleepMetrics) []string {
	var recs []string

	// Breathing-based recommendations
	if m.AvgBreathingRate > BreathingRateHigh {
		recs = append(recs, "Consider relaxation techniques before bed to help lower your breathing rate")
	}
	if m.BreathingRateStdDev > 3 {
		recs = append(recs, "Irregular breathing patterns were observed - try maintaining a consistent sleep environment")
	}

	// Motion-based recommendations
	if m.QuietTimePct < 60 {
		recs = append(recs, "High restlessness detected - evaluate your mattress, pillow, and room temperature")
	}
	if m.MotionEvents > 15 {
		recs = append(recs, "Frequent movement may indicate discomfort or stress - consider a pre-sleep routine")
	}

	// Duration-based recommendations
	if m.TotalDuration < 6*time.Hour {
		recs = append(recs, "Your sleep duration was less than 6 hours - aim for 7-9 hours of sleep")
	} else if m.TotalDuration < 7*time.Hour {
		recs = append(recs, "Consider going to bed earlier to get 7-9 hours of sleep")
	}

	// Continuity-based recommendations
	if m.Interruptions > 3 {
		recs = append(recs, "Multiple sleep interruptions detected - check for noise, light, or temperature issues")
	}
	if m.LongestDeepPeriod < 20*time.Minute {
		recs = append(recs, "Limited deep sleep detected - avoid caffeine and screens before bed")
	}

	// Positive reinforcement
	if m.OverallScore >= 80 {
		recs = append(recs, "Excellent sleep quality! Maintain your current sleep habits")
	} else if len(recs) == 0 {
		recs = append(recs, "Your sleep quality was good - continue your current routine")
	}

	return recs
}

// FormatDuration formats a duration in human-readable form
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", d/time.Second)
	}
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", d/time.Minute)
	}

	hours := d / time.Hour
	mins := (d % time.Hour) / time.Minute

	if mins == 0 {
		return fmt.Sprintf("%d hour%s", hours, pluralS(int(hours)))
	}
	return fmt.Sprintf("%d hour%s %d minute%s", hours, pluralS(int(hours)), mins, pluralS(int(mins)))
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// ToJSONMap converts the report to a map for JSON serialization
func (r *SleepReport) ToJSONMap() map[string]interface{} {
	m := map[string]interface{}{
		"link_id":        r.LinkID,
		"session_date":   r.SessionDate.Format("2006-01-02"),
		"generated_at":   r.GeneratedAt.UnixMilli(),
		"overall_score":  r.Metrics.OverallScore,
		"quality_rating": r.Metrics.QualityRating,
		"breathing_summary": r.BreathingSummary,
		"motion_summary":    r.MotionSummary,
		"recommendations":   r.Recommendations,
	}

	// Add detailed metrics
	m["metrics"] = map[string]interface{}{
		"total_duration_hours":   r.Metrics.TotalDuration.Hours(),
		"time_in_bed_hours":      r.Metrics.TimeInBed.Hours(),
		"avg_breathing_rate":     r.Metrics.AvgBreathingRate,
		"breathing_rate_std_dev": r.Metrics.BreathingRateStdDev,
		"breathing_score":        r.Metrics.BreathingScore,
		"quiet_time_pct":         r.Metrics.QuietTimePct,
		"motion_events":          r.Metrics.MotionEvents,
		"restless_periods":       r.Metrics.RestlessPeriods,
		"motion_score":           r.Metrics.MotionScore,
		"interruptions":          r.Metrics.Interruptions,
		"longest_deep_period_mins": r.Metrics.LongestDeepPeriod.Minutes(),
		"continuity_score":       r.Metrics.ContinuityScore,
		"breathing_score":        r.Metrics.BreathingScore,
		"motion_score":           r.Metrics.MotionScore,
		"continuity_score":       r.Metrics.ContinuityScore,
	}

	// Add timing
	if !r.Metrics.SleepStartTime.IsZero() {
		m["sleep_start_time"] = r.Metrics.SleepStartTime.Format("15:04")
	}
	if !r.Metrics.SleepEndTime.IsZero() {
		m["sleep_end_time"] = r.Metrics.SleepEndTime.Format("15:04")
	}

	return m
}
