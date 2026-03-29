// Package fleet implements link weather diagnostics and root-cause analysis
package fleet

import (
	"fmt"
	"math"
	"sync"
	"time"
)

// LinkWeatherCondition represents the health status of a link
type LinkWeatherCondition string

const (
	WeatherExcellent LinkWeatherCondition = "excellent" // All metrics good
	WeatherGood      LinkWeatherCondition = "good"      // Minor issues
	WeatherFair      LinkWeatherCondition = "fair"      // Moderate degradation
	WeatherPoor      LinkWeatherCondition = "poor"      // Significant issues
	WeatherCritical  LinkWeatherCondition = "critical"  // Link unusable
)

// LinkIssue represents a detected issue with a link
type LinkIssue struct {
	Type        string  // "snr", "phase_stability", "packet_loss", "drift", "motion_false_positive"
	Severity    float64 // 0-1, higher is worse
	Description string
	Suggestion  string
}

// LinkWeatherSnapshot represents a point-in-time health snapshot
type LinkWeatherSnapshot struct {
	Timestamp        time.Time
	SNR              float64
	PhaseStability   float64
	PacketRate       float64
	DriftRate        float64
	Condition        LinkWeatherCondition
	CompositeScore   float64 // Overall health score 0-1
	DeltaRMSVariance float64 // Motion variance for periodic interference detection
	IsQuietPeriod    bool    // True if no motion detected during snapshot
}

// LinkWeatherReport contains diagnostic information for a link
type LinkWeatherReport struct {
	LinkID           string
	CurrentCondition LinkWeatherCondition
	Confidence       float64 // 0-1 health score

	// Current issues
	Issues []LinkIssue

	// Trend over time
	TrendSNR          string // "improving", "stable", "degrading"
	TrendStability    string
	TrendPacketRate   string

	// Weekly statistics
	WeekUptimePct     float64
	WeekMeanSNR       float64
	WeekFalsePosRate  float64

	// Repositioning advice
	RepositionAdvice  string
	SuggestedPosition *PositionSuggestion
}

// PositionSuggestion contains node repositioning advice
type PositionSuggestion struct {
	NodeMAC string
	X       float64
	Z       float64
	Reason  string
	Gain    float64 // Expected GDOP improvement
}

// LinkWeatherDiagnostics manages link health analysis
type LinkWeatherDiagnostics struct {
	mu sync.RWMutex

	// Per-link weather history
	history    map[string][]LinkWeatherSnapshot
	maxHistory int

	// Issue detection thresholds
	snrWarnThreshold     float64
	snrCriticalThreshold float64
	phaseStabilityWarn   float64
	phaseStabilityCrit   float64
	packetRateMin        float64
	driftWarnThreshold   float64

	// False positive tracking
	falsePositiveEvents map[string][]time.Time

	// Node position access
	getNodePosition func(mac string) (x, z float64, ok bool)
	suggestPosition func() (x, z, improvement float64)
}

// NewLinkWeatherDiagnostics creates a new diagnostics manager
func NewLinkWeatherDiagnostics() *LinkWeatherDiagnostics {
	return &LinkWeatherDiagnostics{
		history:              make(map[string][]LinkWeatherSnapshot),
		maxHistory:           10080, // 1 week at 1-minute intervals
		falsePositiveEvents:  make(map[string][]time.Time),
		snrWarnThreshold:     0.5,
		snrCriticalThreshold: 0.25,
		phaseStabilityWarn:   0.5,
		phaseStabilityCrit:   0.8,
		packetRateMin:        5.0,
		driftWarnThreshold:   0.3,
	}
}

// SetNodePositionAccessor sets the function to get node positions
func (lwd *LinkWeatherDiagnostics) SetNodePositionAccessor(fn func(mac string) (x, z float64, ok bool)) {
	lwd.mu.Lock()
	lwd.getNodePosition = fn
	lwd.mu.Unlock()
}

// SetPositionSuggester sets the function to suggest optimal positions
func (lwd *LinkWeatherDiagnostics) SetPositionSuggester(fn func() (x, z, improvement float64)) {
	lwd.mu.Lock()
	lwd.suggestPosition = fn
	lwd.mu.Unlock()
}

// RecordSnapshot records a health snapshot for a link
func (lwd *LinkWeatherDiagnostics) RecordSnapshot(linkID string, snr, phaseStability, packetRate, driftRate float64) {
	lwd.mu.Lock()
	defer lwd.mu.Unlock()

	snapshot := LinkWeatherSnapshot{
		Timestamp:      time.Now(),
		SNR:            snr,
		PhaseStability: phaseStability,
		PacketRate:     packetRate,
		DriftRate:      driftRate,
		Condition:      lwd.classifyCondition(snr, phaseStability, packetRate, driftRate),
	}

	lwd.history[linkID] = append(lwd.history[linkID], snapshot)

	// Trim to max history
	if len(lwd.history[linkID]) > lwd.maxHistory {
		lwd.history[linkID] = lwd.history[linkID][len(lwd.history[linkID])-lwd.maxHistory:]
	}
}

// RecordFalsePositive records a potential false positive event
func (lwd *LinkWeatherDiagnostics) RecordFalsePositive(linkID string) {
	lwd.mu.Lock()
	defer lwd.mu.Unlock()

	lwd.falsePositiveEvents[linkID] = append(lwd.falsePositiveEvents[linkID], time.Now())

	// Keep only last 100 events per link
	if len(lwd.falsePositiveEvents[linkID]) > 100 {
		lwd.falsePositiveEvents[linkID] = lwd.falsePositiveEvents[linkID][100:]
	}
}

// classifyCondition determines the weather condition from metrics
func (lwd *LinkWeatherDiagnostics) classifyCondition(snr, phaseStability, packetRate, driftRate float64) LinkWeatherCondition {
	score := lwd.computeHealthScore(snr, phaseStability, packetRate, driftRate)

	switch {
	case score >= 0.9:
		return WeatherExcellent
	case score >= 0.75:
		return WeatherGood
	case score >= 0.5:
		return WeatherFair
	case score >= 0.25:
		return WeatherPoor
	default:
		return WeatherCritical
	}
}

// computeHealthScore computes a composite health score
func (lwd *LinkWeatherDiagnostics) computeHealthScore(snr, phaseStability, packetRate, driftRate float64) float64 {
	// Invert stability and drift (lower is better)
	stabilityScore := 1.0 - phaseStability
	driftScore := 1.0 - driftRate

	// Packet rate score
	rateScore := packetRate / 20.0 // Normalize to 20 Hz
	if rateScore > 1.0 {
		rateScore = 1.0
	}

	// Weighted average
	score := 0.3*snr + 0.25*stabilityScore + 0.25*rateScore + 0.2*driftScore

	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}

	return score
}

// GetReport generates a diagnostic report for a link
func (lwd *LinkWeatherDiagnostics) GetReport(linkID string) *LinkWeatherReport {
	lwd.mu.RLock()
	defer lwd.mu.RUnlock()

	report := &LinkWeatherReport{
		LinkID: linkID,
		Issues: make([]LinkIssue, 0),
	}

	history := lwd.history[linkID]
	if len(history) == 0 {
		report.CurrentCondition = WeatherFair
		report.Confidence = 0.5
		return report
	}

	// Get latest snapshot
	latest := history[len(history)-1]
	report.CurrentCondition = latest.Condition
	report.Confidence = lwd.computeHealthScore(latest.SNR, latest.PhaseStability, latest.PacketRate, latest.DriftRate)

	// Detect issues
	report.Issues = lwd.detectIssues(linkID, latest)

	// Compute trends
	report.TrendSNR = lwd.computeTrend(history, func(s LinkWeatherSnapshot) float64 { return s.SNR })
	report.TrendStability = lwd.computeTrend(history, func(s LinkWeatherSnapshot) float64 { return 1 - s.PhaseStability })
	report.TrendPacketRate = lwd.computeTrend(history, func(s LinkWeatherSnapshot) float64 { return s.PacketRate })

	// Weekly statistics
	report.WeekUptimePct = lwd.computeUptime(history)
	report.WeekMeanSNR = lwd.computeMean(history, func(s LinkWeatherSnapshot) float64 { return s.SNR })
	report.WeekFalsePosRate = lwd.computeFalsePositiveRate(linkID)

	// Generate repositioning advice
	report.RepositionAdvice, report.SuggestedPosition = lwd.generateRepositioningAdvice(linkID, latest)

	return report
}

// detectIssues identifies problems with a link
func (lwd *LinkWeatherDiagnostics) detectIssues(linkID string, current LinkWeatherSnapshot) []LinkIssue {
	issues := make([]LinkIssue, 0)

	// SNR issues
	if current.SNR < lwd.snrCriticalThreshold {
		issues = append(issues, LinkIssue{
			Type:        "snr",
			Severity:    1.0 - current.SNR,
			Description: "Signal-to-noise ratio is critically low",
			Suggestion:  "Check for obstructions or interference. Consider relocating nodes closer together or away from metal objects.",
		})
	} else if current.SNR < lwd.snrWarnThreshold {
		issues = append(issues, LinkIssue{
			Type:        "snr",
			Severity:    0.5 * (lwd.snrWarnThreshold - current.SNR) / lwd.snrWarnThreshold,
			Description: "Signal quality is degraded",
			Suggestion:  "Monitor for continued degradation. Check for new obstructions.",
		})
	}

	// Phase stability issues
	if current.PhaseStability > lwd.phaseStabilityCrit {
		issues = append(issues, LinkIssue{
			Type:        "phase_stability",
			Severity:    current.PhaseStability,
			Description: "Phase measurements are highly unstable",
			Suggestion:  "Check for multipath interference from reflective surfaces. Ensure nodes are securely mounted.",
		})
	} else if current.PhaseStability > lwd.phaseStabilityWarn {
		issues = append(issues, LinkIssue{
			Type:        "phase_stability",
			Severity:    0.5 * current.PhaseStability,
			Description: "Phase stability is reduced",
			Suggestion:  "Monitor for environmental changes (HVAC, moving objects) that could affect signal.",
		})
	}

	// Packet rate issues
	if current.PacketRate < lwd.packetRateMin {
		severity := 1.0 - current.PacketRate/lwd.packetRateMin
		issues = append(issues, LinkIssue{
			Type:        "packet_loss",
			Severity:    severity,
			Description: "Packet rate is below expected",
			Suggestion:  "Check WiFi congestion. Ensure nodes are on a clear channel. Reduce distance if possible.",
		})
	}

	// Drift issues
	if current.DriftRate > lwd.driftWarnThreshold {
		issues = append(issues, LinkIssue{
			Type:        "drift",
			Severity:    current.DriftRate,
			Description: "Baseline is drifting rapidly",
			Suggestion:  "Environmental conditions may be changing. Check for HVAC cycling or sunlight exposure.",
		})
	}

	return issues
}

// computeTrend determines if a metric is improving or degrading
func (lwd *LinkWeatherDiagnostics) computeTrend(history []LinkWeatherSnapshot, getMetric func(LinkWeatherSnapshot) float64) string {
	if len(history) < 10 {
		return "stable"
	}

	// Compare recent window to earlier window
	recentSize := len(history) / 5
	if recentSize < 2 {
		recentSize = 2
	}

	// Recent average
	var recentSum float64
	for i := len(history) - recentSize; i < len(history); i++ {
		recentSum += getMetric(history[i])
	}
	recentAvg := recentSum / float64(recentSize)

	// Earlier average
	var earlierSum float64
	earlierSize := len(history) / 5
	for i := 0; i < earlierSize && i < len(history)-recentSize; i++ {
		earlierSum += getMetric(history[i])
	}
	earlierAvg := earlierSum / float64(earlierSize)

	// Compute change
	delta := recentAvg - earlierAvg
	threshold := 0.1 // 10% change threshold

	if delta > threshold {
		return "improving"
	} else if delta < -threshold {
		return "degrading"
	}
	return "stable"
}

// computeUptime calculates the percentage of time the link was healthy
func (lwd *LinkWeatherDiagnostics) computeUptime(history []LinkWeatherSnapshot) float64 {
	if len(history) == 0 {
		return 0
	}

	healthy := 0
	for _, s := range history {
		if s.Condition == WeatherExcellent || s.Condition == WeatherGood || s.Condition == WeatherFair {
			healthy++
		}
	}

	return float64(healthy) / float64(len(history)) * 100
}

// computeMean calculates the mean of a metric over history
func (lwd *LinkWeatherDiagnostics) computeMean(history []LinkWeatherSnapshot, getMetric func(LinkWeatherSnapshot) float64) float64 {
	if len(history) == 0 {
		return 0
	}

	var sum float64
	for _, s := range history {
		sum += getMetric(s)
	}
	return sum / float64(len(history))
}

// computeFalsePositiveRate calculates false positive events per hour
func (lwd *LinkWeatherDiagnostics) computeFalsePositiveRate(linkID string) float64 {
	events := lwd.falsePositiveEvents[linkID]
	if len(events) == 0 {
		return 0
	}

	// Count events in last 24 hours
	now := time.Now()
	windowStart := now.Add(-24 * time.Hour)
	count := 0

	for _, t := range events {
		if t.After(windowStart) {
			count++
		}
	}

	// Return events per hour
	return float64(count) / 24.0
}

// generateRepositioningAdvice creates actionable advice for improving link quality
func (lwd *LinkWeatherDiagnostics) generateRepositioningAdvice(linkID string, current LinkWeatherSnapshot) (string, *PositionSuggestion) {
	if current.Condition == WeatherExcellent || current.Condition == WeatherGood {
		return "Link is performing well. No repositioning needed.", nil
	}

	// Extract node MACs from linkID
	if len(linkID) < 35 {
		return "Unable to identify nodes for this link.", nil
	}

	// Parse linkID format: "AA:BB:CC:DD:EE:FF:11:22:33:44:55:66"
	nodeAMAC := linkID[:17]
	nodeBMAC := linkID[18:]

	var advice string
	var suggestion *PositionSuggestion

	// Analyze the root cause
	if current.SNR < lwd.snrWarnThreshold {
		advice = "Low signal quality detected. "

		// Get current positions
		if lwd.getNodePosition != nil {
			ax, az, aOk := lwd.getNodePosition(nodeAMAC)
			bx, bz, bOk := lwd.getNodePosition(nodeBMAC)

			if aOk && bOk {
				distance := math.Sqrt((ax-bx)*(ax-bx) + (az-bz)*(az-bz))

				if distance > 8 {
					advice += "Nodes are far apart (" + formatDistance(distance) + "). Consider moving them closer together."
				} else if distance < 1 {
					advice += "Nodes are very close. This may cause saturation. Consider increasing separation."
				} else {
					advice += "Distance is reasonable. Check for obstructions between nodes."
				}
			}
		}

		// Suggest better position if available
		if lwd.suggestPosition != nil {
			x, z, improvement := lwd.suggestPosition()
			if improvement > 0.5 {
				suggestion = &PositionSuggestion{
					NodeMAC: nodeAMAC, // Suggest moving one of the nodes
					X:       x,
					Z:       z,
					Reason:  "Moving to this position would improve coverage",
					Gain:    improvement,
				}
			}
		}
	} else if current.PhaseStability > lwd.phaseStabilityWarn {
		advice = "Phase instability detected. This often indicates multipath interference. "
		advice += "Try moving nodes away from metal surfaces, mirrors, or large appliances."
	} else if current.PacketRate < lwd.packetRateMin {
		advice = "Packet loss detected. Check WiFi channel congestion and ensure nodes have clear line of sight."
	} else {
		advice = "Link quality is reduced. Review node placement and environmental factors."
	}

	return advice, suggestion
}

// formatDistance formats a distance in meters
func formatDistance(m float64) string {
	if m < 1 {
		return "< 1m"
	}
	return fmt.Sprintf("%.0fm", m)
}

// GetAllLinkReports returns reports for all tracked links
func (lwd *LinkWeatherDiagnostics) GetAllLinkReports() map[string]*LinkWeatherReport {
	lwd.mu.RLock()
	defer lwd.mu.RUnlock()

	reports := make(map[string]*LinkWeatherReport)
	for linkID := range lwd.history {
		reports[linkID] = lwd.GetReport(linkID)
	}
	return reports
}

// GetHistory returns historical snapshots for a link within the specified window
func (lwd *LinkWeatherDiagnostics) GetHistory(linkID string, window time.Duration) []LinkWeatherSnapshot {
	lwd.mu.RLock()
	defer lwd.mu.RUnlock()

	history := lwd.history[linkID]
	if len(history) == 0 {
		return nil
	}

	cutoff := time.Now().Add(-window)
	var result []LinkWeatherSnapshot
	for _, s := range history {
		if s.Timestamp.After(cutoff) {
			result = append(result, s)
		}
	}
	return result
}

// GetAllLinkIDs returns all tracked link IDs
func (lwd *LinkWeatherDiagnostics) GetAllLinkIDs() []string {
	lwd.mu.RLock()
	defer lwd.mu.RUnlock()

	ids := make([]string, 0, len(lwd.history))
	for linkID := range lwd.history {
		ids = append(ids, linkID)
	}
	return ids
}

// GetSystemWeatherSummary returns overall system health
func (lwd *LinkWeatherDiagnostics) GetSystemWeatherSummary() (condition LinkWeatherCondition, avgConfidence float64, issueCount int) {
	lwd.mu.RLock()
	defer lwd.mu.RUnlock()

	if len(lwd.history) == 0 {
		return WeatherFair, 0.5, 0
	}

	var totalConfidence float64
	linkCount := 0
	worstCondition := WeatherExcellent
	totalIssues := 0

	for linkID, history := range lwd.history {
		if len(history) == 0 {
			continue
		}

		latest := history[len(history)-1]
		conf := lwd.computeHealthScore(latest.SNR, latest.PhaseStability, latest.PacketRate, latest.DriftRate)
		totalConfidence += conf
		linkCount++

		// Track worst condition
		if compareConditions(latest.Condition, worstCondition) > 0 {
			worstCondition = latest.Condition
		}

		// Count issues
		totalIssues += len(lwd.detectIssues(linkID, latest))
	}

	if linkCount == 0 {
		return WeatherFair, 0.5, 0
	}

	return worstCondition, totalConfidence / float64(linkCount), totalIssues
}

// compareConditions compares two conditions, returning >0 if a is worse than b
func compareConditions(a, b LinkWeatherCondition) int {
	order := map[LinkWeatherCondition]int{
		WeatherExcellent: 0,
		WeatherGood:      1,
		WeatherFair:      2,
		WeatherPoor:      3,
		WeatherCritical:  4,
	}
	return order[a] - order[b]
}

// GetWeeklyTrend returns daily health averages for the past week
func (lwd *LinkWeatherDiagnostics) GetWeeklyTrend(linkID string) []DailyHealthSummary {
	lwd.mu.RLock()
	defer lwd.mu.RUnlock()

	history := lwd.history[linkID]
	if len(history) < 24 {
		return nil
	}

	// Group by day
	daySummaries := make(map[string]*dailyAccumulator)
	for _, s := range history {
		dayKey := s.Timestamp.Format("2006-01-02")
		if daySummaries[dayKey] == nil {
			daySummaries[dayKey] = &dailyAccumulator{}
		}
		daySummaries[dayKey].add(s)
	}

	// Convert to array
	result := make([]DailyHealthSummary, 0, 7)
	for day, acc := range daySummaries {
		t, _ := time.Parse("2006-01-02", day)
		result = append(result, DailyHealthSummary{
			Date:        t,
			MeanSNR:     acc.snr / float64(acc.count),
			MeanHealth:  acc.health / float64(acc.count),
			SampleCount: acc.count,
		})
	}

	// Sort by date
	sortDailySummaries(result)

	// Return last 7 days
	if len(result) > 7 {
		result = result[len(result)-7:]
	}

	return result
}

type dailyAccumulator struct {
	snr   float64
	health float64
	count int
}

func (d *dailyAccumulator) add(s LinkWeatherSnapshot) {
	d.snr += s.SNR
	d.health += 1 - s.PhaseStability // Stability as health
	d.count++
}

// DailyHealthSummary represents one day's health metrics
type DailyHealthSummary struct {
	Date        time.Time
	MeanSNR     float64
	MeanHealth  float64
	SampleCount int
}

// sortDailySummaries sorts by date ascending
func sortDailySummaries(summaries []DailyHealthSummary) {
	for i := 0; i < len(summaries); i++ {
		for j := i + 1; j < len(summaries); j++ {
			if summaries[j].Date.Before(summaries[i].Date) {
				summaries[i], summaries[j] = summaries[j], summaries[i]
			}
		}
	}
}
