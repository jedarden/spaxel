// Package diagnostics implements link weather diagnostics with root-cause analysis
// and actionable repositioning advice.
package diagnostics

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"sync"
	"time"
)

// DiagnosisSeverity represents the severity level of a diagnosis
type DiagnosisSeverity string

const (
	SeverityINFO      DiagnosisSeverity = "INFO"      // Informational, no action needed
	SeverityWARNING   DiagnosisSeverity = "WARNING"   // Attention recommended
	SeverityACTIONABLE DiagnosisSeverity = "ACTIONABLE" // Specific action required
)

// Diagnosis represents a diagnostic finding for a link
type Diagnosis struct {
	LinkID              string
	RuleID              string            // Identifies which rule fired
	Severity            DiagnosisSeverity // INFO, WARNING, ACTIONABLE
	Title               string            // Human-readable headline
	Detail              string            // Explanation in plain language
	Advice              string            // Specific actionable steps
	RepositioningTarget *Vec3             // 3D position to move node, or nil
	RepositioningNodeMAC string           // Which node to move
	ConfidenceScore     float64           // How confident the engine is (0-1)
	Timestamp           time.Time
}

// Vec3 represents a 3D position
type Vec3 struct {
	X float64
	Y float64
	Z float64
}

// LinkHealthSnapshot represents a health snapshot for diagnostic analysis
type LinkHealthSnapshot struct {
	Timestamp       time.Time
	SNR             float64
	PhaseStability  float64
	PacketRate      float64
	DriftRate       float64
	CompositeScore  float64
	DeltaRMSVariance float64 // For periodic interference detection
	IsQuietPeriod   bool     // True if no motion detected
}

// FeedbackEvent represents user-reported false negative/positive for Rule 4
type FeedbackEvent struct {
	LinkID    string
	EventType string    // "false_negative" or "false_positive"
	Position  Vec3      // Where the event occurred
	Timestamp time.Time
}

// DiagnosticConfig holds configuration for the DiagnosticEngine
type DiagnosticConfig struct {
	DiagnosticInterval time.Duration // How often to run diagnostics (default 15m)
	HistoryWindow      time.Duration // How much history to analyze (default 1h)
	MinSamples         int           // Minimum samples needed for diagnosis
}

// DiagnosticEngine runs background diagnostics on link health
type DiagnosticEngine struct {
	mu sync.RWMutex

	config DiagnosticConfig

	// Health data access
	getHealthHistory func(linkID string, window time.Duration) []LinkHealthSnapshot
	getAllLinkIDs    func() []string

	// Feedback data access (for Rule 4)
	getFeedbackEvents func(linkID string, window time.Duration) []FeedbackEvent

	// Repositioning computation (for Rule 4)
	computeRepositioning func(linkID string, blockedZone Vec3) (Vec3, float64, error)

	// GDOP calculator access
	getGDOPImprovement func(nodeMAC string, targetPos Vec3) float64

	// Node position access
	getNodePosition func(mac string) (Vec3, bool)

	// Occupancy state for quiet period detection
	getOccupancyState func() int // Returns number of detected persons

	// Recent diagnoses per link (last N diagnoses)
	recentDiagnoses map[string][]Diagnosis
	maxDiagnoses    int

	// Running state
	running bool
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewDiagnosticEngine creates a new diagnostic engine
func NewDiagnosticEngine(cfg DiagnosticConfig) *DiagnosticEngine {
	if cfg.DiagnosticInterval == 0 {
		cfg.DiagnosticInterval = 15 * time.Minute
	}
	if cfg.HistoryWindow == 0 {
		cfg.HistoryWindow = 1 * time.Hour
	}
	if cfg.MinSamples == 0 {
		cfg.MinSamples = 10
	}

	return &DiagnosticEngine{
		config:          cfg,
		recentDiagnoses: make(map[string][]Diagnosis),
		maxDiagnoses:    10,
	}
}

// SetHealthHistoryAccessor sets the function to get health history
func (de *DiagnosticEngine) SetHealthHistoryAccessor(fn func(linkID string, window time.Duration) []LinkHealthSnapshot) {
	de.mu.Lock()
	de.getHealthHistory = fn
	de.mu.Unlock()
}

// SetAllLinkIDsAccessor sets the function to get all link IDs
func (de *DiagnosticEngine) SetAllLinkIDsAccessor(fn func() []string) {
	de.mu.Lock()
	de.getAllLinkIDs = fn
	de.mu.Unlock()
}

// SetFeedbackAccessor sets the function to get feedback events
func (de *DiagnosticEngine) SetFeedbackAccessor(fn func(linkID string, window time.Duration) []FeedbackEvent) {
	de.mu.Lock()
	de.getFeedbackEvents = fn
	de.mu.Unlock()
}

// SetRepositioningComputer sets the function to compute repositioning targets
func (de *DiagnosticEngine) SetRepositioningComputer(fn func(linkID string, blockedZone Vec3) (Vec3, float64, error)) {
	de.mu.Lock()
	de.computeRepositioning = fn
	de.mu.Unlock()
}

// SetGDOPImprovementAccessor sets the function to get GDOP improvement estimates
func (de *DiagnosticEngine) SetGDOPImprovementAccessor(fn func(nodeMAC string, targetPos Vec3) float64) {
	de.mu.Lock()
	de.getGDOPImprovement = fn
	de.mu.Unlock()
}

// SetNodePositionAccessor sets the function to get node positions
func (de *DiagnosticEngine) SetNodePositionAccessor(fn func(mac string) (Vec3, bool)) {
	de.mu.Lock()
	de.getNodePosition = fn
	de.mu.Unlock()
}

// SetOccupancyAccessor sets the function to get occupancy state
func (de *DiagnosticEngine) SetOccupancyAccessor(fn func() int) {
	de.mu.Lock()
	de.getOccupancyState = fn
	de.mu.Unlock()
}

// Run starts the diagnostic engine background loop
func (de *DiagnosticEngine) Run(ctx context.Context) {
	de.mu.Lock()
	if de.running {
		de.mu.Unlock()
		return
	}
	de.running = true
	de.ctx, de.cancel = context.WithCancel(ctx)
	de.mu.Unlock()

	ticker := time.NewTicker(de.config.DiagnosticInterval)
	defer ticker.Stop()

	log.Printf("[INFO] diagnostics: engine started (interval: %v)", de.config.DiagnosticInterval)

	for {
		select {
		case <-de.ctx.Done():
			log.Printf("[INFO] diagnostics: engine stopped")
			return
		case <-ticker.C:
			de.runDiagnosticPass()
		}
	}
}

// Stop stops the diagnostic engine
func (de *DiagnosticEngine) Stop() {
	de.mu.Lock()
	defer de.mu.Unlock()
	if de.cancel != nil {
		de.cancel()
	}
	de.running = false
}

// runDiagnosticPass runs all diagnostic rules on all links
func (de *DiagnosticEngine) runDiagnosticPass() {
	de.mu.RLock()
	getAllLinkIDs := de.getAllLinkIDs
	de.mu.RUnlock()

	if getAllLinkIDs == nil {
		return
	}

	linkIDs := getAllLinkIDs()
	for _, linkID := range linkIDs {
		diagnoses := de.diagnoseLink(linkID)
		if len(diagnoses) > 0 {
			de.storeDiagnoses(linkID, diagnoses)
		}
	}
}

// diagnoseLink runs all diagnostic rules on a single link
func (de *DiagnosticEngine) diagnoseLink(linkID string) []Diagnosis {
	de.mu.RLock()
	getHealthHistory := de.getHealthHistory
	window := de.config.HistoryWindow
	de.mu.RUnlock()
	_ = de.getFeedbackEvents
	_ = de.getOccupancyState

	if getHealthHistory == nil {
		return nil
	}

	history := getHealthHistory(linkID, window)
	if len(history) < de.config.MinSamples {
		return nil
	}

	var diagnoses []Diagnosis

	// Rule 1: Environmental Change
	if d := de.checkEnvironmentalChange(linkID, history); d != nil {
		diagnoses = append(diagnoses, *d)
	}

	// Rule 2: WiFi Congestion or Distance
	if d := de.checkWiFiCongestion(linkID, history); d != nil {
		diagnoses = append(diagnoses, *d)
	}

	// Rule 3: Near-Field Metal Interference
	if d := de.checkMetalInterference(linkID, history); d != nil {
		diagnoses = append(diagnoses, *d)
	}

	// Rule 4: Fresnel Zone Blockage
	if d := de.checkFresnelBlockage(linkID, history); d != nil {
		diagnoses = append(diagnoses, *d)
	}

	// Rule 5: Periodic Interference Spikes
	if d := de.checkPeriodicInterference(linkID, history); d != nil {
		diagnoses = append(diagnoses, *d)
	}

	return diagnoses
}

// Rule 1: Environmental Change
// Trigger: High baseline drift (>5% per hour) correlated across multiple links simultaneously (>50% of active links).
func (de *DiagnosticEngine) checkEnvironmentalChange(linkID string, history []LinkHealthSnapshot) *Diagnosis {
	de.mu.RLock()
	getAllLinkIDs := de.getAllLinkIDs
	getHealthHistory := de.getHealthHistory
	de.mu.RUnlock()

	if getAllLinkIDs == nil || getHealthHistory == nil {
		return nil
	}

	// Calculate drift rate for this link
	avgDrift := calculateAverageDrift(history)
	if avgDrift < 0.05 { // < 5% per hour
		return nil
	}

	// Check if drift is correlated across multiple links
	allLinks := getAllLinkIDs()
	correlatedCount := 0
	totalActive := 0

	for _, lid := range allLinks {
		lhistory := getHealthHistory(lid, de.config.HistoryWindow)
		if len(lhistory) < de.config.MinSamples {
			continue
		}
		totalActive++
		lavgDrift := calculateAverageDrift(lhistory)
		if lavgDrift >= 0.05 {
			correlatedCount++
		}
	}

	if totalActive == 0 {
		return nil
	}

	correlationRatio := float64(correlatedCount) / float64(totalActive)
	if correlationRatio < 0.5 {
		return nil // Not correlated enough
	}

	// Confidence: 0.85 if drift is correlated across >50% of links
	confidence := 0.85

	return &Diagnosis{
		LinkID:           linkID,
		RuleID:           "environmental_change",
		Severity:         SeverityINFO,
		Title:            "Environmental change detected",
		Detail:           "Multiple sensing links are showing simultaneous baseline shifts. This typically indicates a temperature change, or a large object was moved in the space. The system is adapting automatically.",
		Advice:           "No action needed. The baseline will re-stabilise within 30 minutes.",
		RepositioningTarget: nil,
		ConfidenceScore:  confidence,
		Timestamp:        time.Now(),
	}
}

// Rule 2: WiFi Congestion or Distance
// Trigger: Packet rate health < 0.8 for more than 10 minutes on a single link.
func (de *DiagnosticEngine) checkWiFiCongestion(linkID string, history []LinkHealthSnapshot) *Diagnosis {
	// Check if packet rate has been low for > 10 minutes
	// Expected rate is 20 Hz, so health < 0.8 means < 16 Hz
	packetHealthThreshold := 0.8
	minDuration := 10 * time.Minute

	if len(history) < 2 {
		return nil
	}

	// Find the time span of history (handles both ascending and descending order)
	startTime := history[0].Timestamp
	endTime := history[len(history)-1].Timestamp
	duration := endTime.Sub(startTime)
	if duration < 0 {
		duration = -duration
	}

	if duration < minDuration {
		return nil
	}

	// Check if packet rate health has been consistently low
	lowCount := 0
	totalCount := 0
	var avgPacketRate float64

	for _, h := range history {
		// Packet rate health: actual/expected (20 Hz)
		rateHealth := h.PacketRate / 20.0
		avgPacketRate += h.PacketRate
		totalCount++
		if rateHealth < packetHealthThreshold {
			lowCount++
		}
	}

	if totalCount == 0 {
		return nil
	}

	avgPacketRate /= float64(totalCount)
	lowRatio := float64(lowCount) / float64(totalCount)

	// Require > 80% of samples to have low packet rate
	if lowRatio < 0.8 {
		return nil
	}

	// Extract node B MAC from linkID
	nodeBMAC := extractNodeBMAC(linkID)

	return &Diagnosis{
		LinkID:           linkID,
		RuleID:           "wifi_congestion_distance",
		Severity:         SeverityACTIONABLE,
		Title:            "Node has low signal rate",
		Detail:           formatWiFiDetail(nodeBMAC, avgPacketRate),
		Advice: formatWiFiAdvice(nodeBMAC),
		RepositioningTarget: nil,
		RepositioningNodeMAC: nodeBMAC,
		ConfidenceScore:  0.75,
		Timestamp:        time.Now(),
	}
}

// Rule 3: Near-Field Metal Interference
// Trigger: Low phase stability (< 0.4) sustained for > 30 minutes during known-quiet periods.
func (de *DiagnosticEngine) checkMetalInterference(linkID string, history []LinkHealthSnapshot) *Diagnosis {
	// Phase stability: lower is more stable. We need to check if stability score is low.
	// In the codebase, PhaseStability is normalized variance where < 0.4 is stable
	// So "low phase stability" means the value is HIGH (> 0.6 or so)
	// Actually looking at ambient.go: stability score of 0.1 is very stable, 1.0 is unstable
	// The task says "Low phase stability (< 0.4)" but this is confusing.
	// Looking at weather.go: phaseStabilityWarn = 0.5, phaseStabilityCrit = 0.8
	// I'll interpret "low phase stability" as "high phase variance" i.e. PhaseStability > 0.6

	phaseThreshold := 0.6 // High variance = unstable
	minDuration := 30 * time.Minute

	if len(history) < 2 {
		return nil
	}

	// Check if we've had enough time (handles both ascending and descending order)
	startTime := history[0].Timestamp
	endTime := history[len(history)-1].Timestamp
	duration := endTime.Sub(startTime)
	if duration < 0 {
		duration = -duration
	}

	if duration < minDuration {
		return nil
	}

	// Count samples during quiet periods with high phase instability
	unstableQuietCount := 0
	totalQuietCount := 0

	for _, h := range history {
		if h.IsQuietPeriod {
			totalQuietCount++
			if h.PhaseStability > phaseThreshold {
				unstableQuietCount++
			}
		}
	}

	// Need enough quiet period samples
	if totalQuietCount < 10 {
		return nil
	}

	// Require > 70% of quiet period samples to have instability
	unstableRatio := float64(unstableQuietCount) / float64(totalQuietCount)
	if unstableRatio < 0.7 {
		return nil
	}

	nodeAMAC := extractNodeAMAC(linkID)

	return &Diagnosis{
		LinkID:           linkID,
		RuleID:           "metal_interference",
		Severity:         SeverityACTIONABLE,
		Title:            formatMetalTitle(nodeAMAC),
		Detail:           formatMetalDetail(linkID),
		Advice:           formatMetalAdvice(nodeAMAC),
		RepositioningTarget: nil,
		RepositioningNodeMAC: nodeAMAC,
		ConfidenceScore:  0.80,
		Timestamp:        time.Now(),
	}
}

// Rule 4: Fresnel Zone Blockage (Half-Room Dead Zone)
// Trigger: Consistent miss rate (>30% of test walks that should be detected are missed)
// in a specific area of the room.
func (de *DiagnosticEngine) checkFresnelBlockage(linkID string, history []LinkHealthSnapshot) *Diagnosis {
	de.mu.RLock()
	getFeedbackEvents := de.getFeedbackEvents
	computeRepositioning := de.computeRepositioning
	getNodePosition := de.getNodePosition
	de.mu.RUnlock()

	if getFeedbackEvents == nil {
		// No feedback data - try heuristic approach
		return de.checkFresnelBlockageHeuristic(linkID, history)
	}

	// Get recent false negative feedback events
	events := getFeedbackEvents(linkID, 7*24*time.Hour) // Last 7 days
	if len(events) < 5 {
		// Not enough feedback data
		return de.checkFresnelBlockageHeuristic(linkID, history)
	}

	// Cluster false negatives by position
	falseNegatives := make([]FeedbackEvent, 0)
	for _, e := range events {
		if e.EventType == "false_negative" {
			falseNegatives = append(falseNegatives, e)
		}
	}

	if len(falseNegatives) < 3 {
		return nil
	}

	// Find the center of the blocked zone
	blockedZone := findClusterCenter(falseNegatives)

	// Compute optimal repositioning target
	var target *Vec3
	var improvement float64
	var targetNodeMAC string

	if computeRepositioning != nil {
		pos, imp, err := computeRepositioning(linkID, blockedZone)
		if err == nil && imp > 0.1 {
			target = &pos
			improvement = imp
			targetNodeMAC = extractNodeBMAC(linkID)
		}
	}

	// Verify node position access for description
	var zoneDesc string
	if getNodePosition != nil {
		zoneDesc = formatZoneDescription(blockedZone)
	} else {
		zoneDesc = "one side of the room"
	}

	nodeBMAC := extractNodeBMAC(linkID)
	detail := formatFresnelDetail(linkID, zoneDesc)
	advice := formatFresnelAdvice(nodeBMAC, &blockedZone, target, improvement)

	return &Diagnosis{
		LinkID:              linkID,
		RuleID:              "fresnel_blockage",
		Severity:            SeverityACTIONABLE,
		Title:               "Coverage gap detected - possible obstruction",
		Detail:              detail,
		Advice:              advice,
		RepositioningTarget: target,
		RepositioningNodeMAC: targetNodeMAC,
		ConfidenceScore:     0.75,
		Timestamp:           time.Now(),
	}
}

// checkFresnelBlockageHeuristic uses blob confidence data when no feedback is available
func (de *DiagnosticEngine) checkFresnelBlockageHeuristic(linkID string, history []LinkHealthSnapshot) *Diagnosis {
	// Heuristic: look for consistent low composite scores during activity periods
	// This would require additional data from the fusion/localization engine
	// For now, we'll check if there's consistently degraded health during non-quiet periods

	var activityScores []float64
	var quietScores []float64

	for _, h := range history {
		if h.IsQuietPeriod {
			quietScores = append(quietScores, h.CompositeScore)
		} else {
			activityScores = append(activityScores, h.CompositeScore)
		}
	}

	// Need enough activity period data
	if len(activityScores) < 10 {
		return nil
	}

	// Calculate average scores
	avgActivity := average(activityScores)
	avgQuiet := average(quietScores)

	// If activity scores are significantly worse than quiet scores,
	// there may be detection issues during movement
	if avgActivity < 0.5 && avgActivity < avgQuiet-0.2 {
		nodeBMAC := extractNodeBMAC(linkID)
		return &Diagnosis{
			LinkID:           linkID,
			RuleID:           "fresnel_blockage_heuristic",
			Severity:         SeverityWARNING,
			Title:            "Possible coverage gap detected",
			Detail:           "Detection quality degrades during movement periods. This may indicate an obstruction in the sensing zone.",
			Advice:           "Submit feedback using the app when detection misses occur. This will help identify the exact location of the coverage gap.",
			RepositioningTarget: nil,
			RepositioningNodeMAC: nodeBMAC,
			ConfidenceScore:  0.60, // Lower confidence for heuristic
			Timestamp:        time.Now(),
		}
	}

	return nil
}

// Rule 5: Periodic Interference Spikes
// Trigger: Periodic spikes in deltaRMS variance (3-10 events per hour, each lasting 1-3 minutes)
// not correlated with occupancy data.
func (de *DiagnosticEngine) checkPeriodicInterference(linkID string, history []LinkHealthSnapshot) *Diagnosis {
	// Look for periodic variance spikes
	minEvents := 3
	maxEvents := 10

	// Find variance spikes
	spikes := findVarianceSpikes(history, 2.0) // 2x normal variance

	if len(spikes) < minEvents || len(spikes) > maxEvents*3 { // Allow some tolerance
		return nil
	}

	// Check for periodicity
	if !isPeriodic(spikes, 1*time.Minute, 3*time.Minute) {
		return nil
	}

	// Calculate events per hour (handles both ascending and descending order)
	startTime := history[0].Timestamp
	endTime := history[len(history)-1].Timestamp
	historyDuration := endTime.Sub(startTime)
	if historyDuration < 0 {
		historyDuration = -historyDuration
	}

	if historyDuration < time.Hour {
		return nil // Need at least an hour of data
	}

	eventsPerHour := float64(len(spikes)) / historyDuration.Hours()
	if eventsPerHour < float64(minEvents) || eventsPerHour > float64(maxEvents) {
		return nil
	}

	nodeAMAC := extractNodeAMAC(linkID)
	nodeBMAC := extractNodeBMAC(linkID)

	return &Diagnosis{
		LinkID:           linkID,
		RuleID:           "periodic_interference",
		Severity:         SeverityWARNING,
		Title:            "Periodic interference detected",
		Detail:           formatInterferenceDetail(nodeAMAC, nodeBMAC, int(eventsPerHour)),
		Advice:           formatInterferenceAdvice(nodeAMAC, nodeBMAC),
		RepositioningTarget: nil,
		ConfidenceScore:  0.70,
		Timestamp:        time.Now(),
	}
}

// storeDiagnoses stores recent diagnoses for a link
func (de *DiagnosticEngine) storeDiagnoses(linkID string, diagnoses []Diagnosis) {
	de.mu.Lock()
	defer de.mu.Unlock()

	de.recentDiagnoses[linkID] = append(diagnoses, de.recentDiagnoses[linkID]...)
	if len(de.recentDiagnoses[linkID]) > de.maxDiagnoses {
		de.recentDiagnoses[linkID] = de.recentDiagnoses[linkID][:de.maxDiagnoses]
	}
}

// GetDiagnoses returns recent diagnoses for a link
func (de *DiagnosticEngine) GetDiagnoses(linkID string) []Diagnosis {
	de.mu.RLock()
	defer de.mu.RUnlock()

	diagnoses := de.recentDiagnoses[linkID]
	result := make([]Diagnosis, len(diagnoses))
	copy(result, diagnoses)
	return result
}

// GetAllDiagnoses returns all recent diagnoses
func (de *DiagnosticEngine) GetAllDiagnoses() map[string][]Diagnosis {
	de.mu.RLock()
	defer de.mu.RUnlock()

	result := make(map[string][]Diagnosis)
	for linkID, diagnoses := range de.recentDiagnoses {
		result[linkID] = make([]Diagnosis, len(diagnoses))
		copy(result[linkID], diagnoses)
	}
	return result
}

// RunDiagnosticPass triggers an immediate diagnostic pass (for testing)
func (de *DiagnosticEngine) RunDiagnosticPass() {
	de.runDiagnosticPass()
}

// Helper functions

func calculateAverageDrift(history []LinkHealthSnapshot) float64 {
	if len(history) == 0 {
		return 0
	}
	var sum float64
	for _, h := range history {
		sum += h.DriftRate
	}
	return sum / float64(len(history))
}

func average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func extractNodeAMAC(linkID string) string {
	if len(linkID) < 17 {
		return ""
	}
	return linkID[:17]
}

func extractNodeBMAC(linkID string) string {
	if len(linkID) < 35 {
		return ""
	}
	return linkID[18:]
}

func findClusterCenter(events []FeedbackEvent) Vec3 {
	if len(events) == 0 {
		return Vec3{}
	}

	var sumX, sumY, sumZ float64
	for _, e := range events {
		sumX += e.Position.X
		sumY += e.Position.Y
		sumZ += e.Position.Z
	}
	n := float64(len(events))
	return Vec3{X: sumX / n, Y: sumY / n, Z: sumZ / n}
}

func findVarianceSpikes(history []LinkHealthSnapshot, threshold float64) []time.Time {
	if len(history) == 0 {
		return nil
	}

	// Calculate baseline variance
	var sum float64
	for _, h := range history {
		sum += h.DeltaRMSVariance
	}
	baseline := sum / float64(len(history))

	if baseline == 0 {
		baseline = 1 // Avoid division by zero
	}

	// First, identify all high-variance samples with their timestamps
	type spikeSample struct {
		ts       time.Time
		variance float64
	}
	var allSpikes []spikeSample
	for _, h := range history {
		if h.DeltaRMSVariance > baseline*threshold {
			allSpikes = append(allSpikes, spikeSample{ts: h.Timestamp, variance: h.DeltaRMSVariance})
		}
	}

	if len(allSpikes) == 0 {
		return nil
	}

	// Sort spikes by timestamp
	sort.Slice(allSpikes, func(i, j int) bool {
		return allSpikes[i].ts.Before(allSpikes[j].ts)
	})

	// Cluster consecutive spikes (within 2 minutes) into events
	// Return the start time of each event
	var events []time.Time
	eventStart := allSpikes[0].ts
	lastSpikeTime := allSpikes[0].ts

	for i := 1; i < len(allSpikes); i++ {
		// If this spike is more than 2 minutes after the last, it's a new event
		if allSpikes[i].ts.Sub(lastSpikeTime) > 2*time.Minute {
			events = append(events, eventStart)
			eventStart = allSpikes[i].ts
		}
		lastSpikeTime = allSpikes[i].ts
	}
	// Add the last event
	events = append(events, eventStart)

	return events
}

func isPeriodic(spikes []time.Time, minInterval, maxInterval time.Duration) bool {
	if len(spikes) < 3 {
		return false
	}

	// Sort spikes by timestamp to handle any order
	sortedSpikes := make([]time.Time, len(spikes))
	copy(sortedSpikes, spikes)
	sort.Slice(sortedSpikes, func(i, j int) bool {
		return sortedSpikes[i].Before(sortedSpikes[j])
	})

	// Calculate intervals between spikes
	intervals := make([]time.Duration, len(sortedSpikes)-1)
	for i := 0; i < len(sortedSpikes)-1; i++ {
		intervals[i] = sortedSpikes[i+1].Sub(sortedSpikes[i])
	}

	// Check if intervals are relatively consistent (within 50% of each other)
	if len(intervals) == 0 {
		return false
	}

	avgInterval := averageDurations(intervals)
	if avgInterval < minInterval || avgInterval > maxInterval*3 {
		return false
	}

	// Count intervals within tolerance of average
	tolerance := 0.5
	withinTolerance := 0
	for _, interval := range intervals {
		ratio := float64(interval) / float64(avgInterval)
		if ratio > (1-tolerance) && ratio < (1+tolerance) {
			withinTolerance++
		}
	}

	// At least 60% of intervals should be within tolerance
	return float64(withinTolerance)/float64(len(intervals)) >= 0.6
}

func averageDurations(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	var sum time.Duration
	for _, d := range durations {
		sum += d
	}
	return sum / time.Duration(len(durations))
}

// Formatting helpers

func formatWiFiDetail(nodeMAC string, avgPacketRate float64) string {
	pct := int(avgPacketRate / 20.0 * 100)
	if nodeMAC != "" {
		return formatNodeLabel(nodeMAC) + " is only delivering " + formatInt(pct) + "% of the expected 20 packets per second. The most common causes are distance from the WiFi router or congestion from nearby networks."
	}
	return "This link is only delivering " + formatInt(pct) + "% of the expected packets. The most common causes are distance from the WiFi router or congestion from nearby networks."
}

func formatWiFiAdvice(nodeMAC string) string {
	label := "the node"
	if nodeMAC != "" {
		label = formatNodeLabel(nodeMAC)
	}
	return "1. Move " + label + " within 10 metres of your WiFi router. " +
		"2. If already close, check if the 2.4GHz channel is congested (3+ networks on overlapping channels). " +
		"3. ESP32-S3 supports both 2.4GHz and 5GHz - if your router supports 5GHz, update the node's WiFi config to use the 5GHz SSID."
}

func formatMetalTitle(nodeMAC string) string {
	if nodeMAC != "" {
		return "Metal interference near " + formatNodeLabel(nodeMAC)
	}
	return "Metal interference detected"
}

func formatMetalDetail(linkID string) string {
	nodeA := extractNodeAMAC(linkID)
	nodeB := extractNodeBMAC(linkID)
	if nodeA != "" && nodeB != "" {
		return "The sensing link " + formatNodeLabel(nodeA) + " to " + formatNodeLabel(nodeB) + " has unstable phase measurements even when no one is moving. This is typically caused by metal objects in the near field of the node's antenna (within 10cm): metal shelves, radiators, TV backs, or large appliances."
	}
	return "This sensing link has unstable phase measurements even when no one is moving. This is typically caused by metal objects in the near field of the node's antenna (within 10cm)."
}

func formatMetalAdvice(nodeMAC string) string {
	label := "the node"
	if nodeMAC != "" {
		label = formatNodeLabel(nodeMAC)
	}
	return "Check for metal objects within 10cm of " + label + ". " +
		"If " + label + " is on a metal surface or shelf, mount it on a non-metal bracket or wall. " +
		"Try repositioning it 20-30cm away from metal surfaces."
}

func formatFresnelDetail(linkID, zoneDesc string) string {
	nodeA := extractNodeAMAC(linkID)
	nodeB := extractNodeBMAC(linkID)
	if nodeA != "" && nodeB != "" {
		return "The area near " + zoneDesc + " shows lower detection coverage. An obstacle may be blocking the path between " + formatNodeLabel(nodeA) + " and " + formatNodeLabel(nodeB) + ", interrupting their sensing zone."
	}
	return "The area near " + zoneDesc + " shows lower detection coverage. An obstacle may be blocking the sensing zone."
}

func formatFresnelAdvice(nodeMAC string, blockedZone, target *Vec3, improvement float64) string {
	if target == nil || nodeMAC == "" {
		return "Check for obstructions between the nodes. Large furniture, appliances, or metal objects can block the sensing zone."
	}

	// Calculate direction from current position
	direction := calculateDirection(blockedZone, target)
	distance := calculateDistance(blockedZone, target)

	advice := "Move " + formatNodeLabel(nodeMAC) + " " + direction + " by approximately " + formatDistance(distance) + " to restore coverage."

	if improvement > 0 {
		advice += " The target position is marked in green in the 3D view."
	}

	return advice
}

func formatInterferenceDetail(nodeA, nodeB string, eventsPerHour int) string {
	if nodeA != "" && nodeB != "" {
		return formatNodeLabel(nodeA) + " to " + formatNodeLabel(nodeB) + " is experiencing regular interference bursts " + formatInt(eventsPerHour) + " times per hour. This pattern is consistent with a microwave oven, a cordless phone, or a pulsed 2.4GHz source."
	}
	return "This link is experiencing regular interference bursts " + formatInt(eventsPerHour) + " times per hour. This pattern is consistent with a microwave oven, a cordless phone, or a pulsed 2.4GHz source."
}

func formatInterferenceAdvice(nodeA, nodeB string) string {
	nodes := "the nodes"
	if nodeA != "" && nodeB != "" {
		nodes = formatNodeLabel(nodeA) + " or " + formatNodeLabel(nodeB)
	}
	return "Consider the following: " +
		"1. Is " + nodes + " near a kitchen? Microwave ovens cause strong 2.4GHz interference. " +
		"2. A cordless DECT phone or baby monitor near one of the nodes may be the source. " +
		"3. Try moving the affected node at least 2 metres from any 2.4GHz appliances."
}

func formatZoneDescription(pos Vec3) string {
	// Simple zone description based on position
	// This could be enhanced with room-relative positioning
	return formatFloat(pos.X, 1) + "m, " + formatFloat(pos.Z, 1) + "m from origin"
}

func formatNodeLabel(mac string) string {
	if len(mac) >= 8 {
		return "Node " + mac[:8]
	}
	return "Node"
}

func calculateDirection(from, to *Vec3) string {
	if from == nil || to == nil {
		return ""
	}

	dx := to.X - from.X
	dz := to.Z - from.Z

	var direction string
	if math.Abs(dz) > math.Abs(dx) {
		if dz > 0 {
			direction = "forward"
		} else {
			direction = "backward"
		}
	} else {
		if dx > 0 {
			direction = "right"
		} else {
			direction = "left"
		}
	}

	return direction
}

func calculateDistance(from, to *Vec3) float64 {
	if from == nil || to == nil {
		return 0
	}
	dx := to.X - from.X
	dz := to.Z - from.Z
	return math.Sqrt(dx*dx + dz*dz)
}

func formatDistance(m float64) string {
	if m < 0.5 {
		return formatFloat(m*100, 0) + "cm"
	}
	return formatFloat(m, 1) + " metres"
}

func formatInt(n int) string {
	if n < 0 {
		return "0"
	}
	return fmt.Sprintf("%d", n)
}

func formatFloat(f float64, decimals int) string {
	return fmt.Sprintf("%.[2]*[1]f", f, decimals)
}
