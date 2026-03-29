// Package sleep implements overnight sleep analysis and reporting.
package sleep

import (
	"math"
	"sync"
	"time"
)

// Sleep state constants
const (
	// Sleep window defaults (can be overridden)
	DefaultSleepStartHour = 22 // 10 PM
	DefaultSleepEndHour   = 7  // 7 AM

	// Scoring weights
	BreathingWeight = 0.4
	MotionWeight    = 0.3
	ContinuityWeight = 0.3

	// Breathing quality thresholds
	BreathingRateLow  = 10.0 // BPM - below this is concerning
	BreathingRateHigh = 25.0 // BPM - above this is concerning
	BreathingRateOptimal = 14.0 // BPM - optimal breathing rate

	// Motion thresholds (deltaRMS)
	QuietMotionThreshold = 0.015 // Below this is considered quiet
	RestlessThreshold    = 0.04  // Above this is restless

	// Sample collection
	SampleInterval = 30 * time.Second
)

// SleepState represents the current sleep state
type SleepState int

const (
	SleepStateAwake SleepState = iota
	SleepStateFallingAsleep
	SleepStateLightSleep
	SleepStateDeepSleep
	SleepStateREM
	SleepStateRestless
)

// String returns the string representation of the sleep state
func (s SleepState) String() string {
	switch s {
	case SleepStateAwake:
		return "awake"
	case SleepStateFallingAsleep:
		return "falling_asleep"
	case SleepStateLightSleep:
		return "light_sleep"
	case SleepStateDeepSleep:
		return "deep_sleep"
	case SleepStateREM:
		return "rem"
	case SleepStateRestless:
		return "restless"
	default:
		return "unknown"
	}
}

// MarshalText implements encoding.TextMarshaler
func (s SleepState) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// BreathingSample represents a breathing measurement during sleep
type BreathingSample struct {
	Timestamp   time.Time `json:"timestamp"`
	RateBPM     float64   `json:"rate_bpm"`
	Confidence  float64   `json:"confidence"`
	IsDetected  bool      `json:"is_detected"`
	HealthGated bool      `json:"health_gated"`
}

// MotionSample represents a motion measurement during sleep
type MotionSample struct {
	Timestamp     time.Time `json:"timestamp"`
	DeltaRMS      float64   `json:"delta_rms"`
	MotionDetected bool     `json:"motion_detected"`
}

// SleepPeriod represents a continuous period of sleep
type SleepPeriod struct {
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time,omitempty"`
	Duration    time.Duration `json:"duration"`
	State       SleepState `json:"state"`
	Interruptions int      `json:"interruptions"`
}

// SleepMetrics aggregates metrics for a sleep session
type SleepMetrics struct {
	// Timing
	SleepStartTime  time.Time `json:"sleep_start_time"`
	SleepEndTime    time.Time `json:"sleep_end_time,omitempty"`
	TotalDuration   time.Duration `json:"total_duration"`
	TimeInBed       time.Duration `json:"time_in_bed"`

	// Breathing metrics
	AvgBreathingRate    float64 `json:"avg_breathing_rate"`
	MinBreathingRate    float64 `json:"min_breathing_rate"`
	MaxBreathingRate    float64 `json:"max_breathing_rate"`
	BreathingRateStdDev float64 `json:"breathing_rate_std_dev"`
	BreathingScore      float64 `json:"breathing_score"` // 0-100

	// Motion metrics
	MotionEvents      int     `json:"motion_events"`
	RestlessPeriods   int     `json:"restless_periods"`
	QuietTimePct      float64 `json:"quiet_time_pct"`
	MotionScore       float64 `json:"motion_score"` // 0-100

	// Sleep continuity
	Interruptions     int     `json:"interruptions"`
	LongestDeepPeriod time.Duration `json:"longest_deep_period"`
	ContinuityScore   float64 `json:"continuity_score"` // 0-100

	// Overall score
	OverallScore      float64 `json:"overall_score"` // 0-100
	QualityRating     string  `json:"quality_rating"` // poor/fair/good/excellent
}

// SleepSession represents a complete sleep session
type SleepSession struct {
	mu sync.RWMutex

	// Configuration
	sleepStartHour int
	sleepEndHour   int

	// State
	currentState   SleepState
	sessionDate    time.Time // Date of sleep session (midnight of the night)
	isActive       bool

	// Sample buffers
	breathingSamples []BreathingSample
	motionSamples    []MotionSample

	// Period tracking
	sleepPeriods []SleepPeriod
	currentPeriod *SleepPeriod

	// Aggregated metrics (computed on demand)
	metrics *SleepMetrics

	// Link ID this session is tracking
	linkID string
}

// SleepAnalyzer manages sleep analysis for multiple links
type SleepAnalyzer struct {
	mu sync.RWMutex

	// Per-link sleep sessions
	sessions map[string]*SleepSession

	// Configuration
	sleepStartHour int
	sleepEndHour   int

	// Report callback
	onReportGenerated func(linkID string, report *SleepReport)
}

// NewSleepAnalyzer creates a new sleep analyzer
func NewSleepAnalyzer() *SleepAnalyzer {
	return &SleepAnalyzer{
		sessions:       make(map[string]*SleepSession),
		sleepStartHour: DefaultSleepStartHour,
		sleepEndHour:   DefaultSleepEndHour,
	}
}

// SetSleepWindow configures the sleep detection window
func (sa *SleepAnalyzer) SetSleepWindow(startHour, endHour int) {
	sa.mu.Lock()
	defer sa.mu.Unlock()
	sa.sleepStartHour = startHour
	sa.sleepEndHour = endHour
}

// SetReportCallback sets the callback for when sleep reports are generated
func (sa *SleepAnalyzer) SetReportCallback(cb func(linkID string, report *SleepReport)) {
	sa.mu.Lock()
	defer sa.mu.Unlock()
	sa.onReportGenerated = cb
}

// ProcessBreathing processes a breathing sample for a link
func (sa *SleepAnalyzer) ProcessBreathing(linkID string, sample BreathingSample) {
	sa.mu.Lock()
	session := sa.getOrCreateSession(linkID)
	sa.mu.Unlock()

	session.processBreathing(sample)
}

// ProcessMotion processes a motion sample for a link
func (sa *SleepAnalyzer) ProcessMotion(linkID string, sample MotionSample) {
	sa.mu.Lock()
	session := sa.getOrCreateSession(linkID)
	sa.mu.Unlock()

	session.processMotion(sample)
}

// GetSession returns the current sleep session for a link
func (sa *SleepAnalyzer) GetSession(linkID string) *SleepSession {
	sa.mu.RLock()
	defer sa.mu.RUnlock()
	return sa.sessions[linkID]
}

// GetCurrentState returns the current sleep state for a link
func (sa *SleepAnalyzer) GetCurrentState(linkID string) SleepState {
	sa.mu.RLock()
	session, exists := sa.sessions[linkID]
	sa.mu.RUnlock()

	if !exists {
		return SleepStateAwake
	}
	return session.GetCurrentState()
}

// GetAllSessions returns all active sleep sessions
func (sa *SleepAnalyzer) GetAllSessions() map[string]*SleepSession {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	result := make(map[string]*SleepSession, len(sa.sessions))
	for k, v := range sa.sessions {
		result[k] = v
	}
	return result
}

// GenerateMorningReports generates reports for all completed sleep sessions
func (sa *SleepAnalyzer) GenerateMorningReports() map[string]*SleepReport {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	reports := make(map[string]*SleepReport)
	for linkID, session := range sa.sessions {
		if report := session.GenerateReport(); report != nil {
			reports[linkID] = report

			if sa.onReportGenerated != nil {
				sa.onReportGenerated(linkID, report)
			}
		}
	}

	return reports
}

// getOrCreateSession gets or creates a sleep session for a link (caller must hold lock)
func (sa *SleepAnalyzer) getOrCreateSession(linkID string) *SleepSession {
	if session, exists := sa.sessions[linkID]; exists {
		return session
	}

	session := NewSleepSession(linkID, sa.sleepStartHour, sa.sleepEndHour)
	sa.sessions[linkID] = session
	return session
}

// NewSleepSession creates a new sleep session
func NewSleepSession(linkID string, sleepStartHour, sleepEndHour int) *SleepSession {
	return &SleepSession{
		linkID:           linkID,
		sleepStartHour:   sleepStartHour,
		sleepEndHour:     sleepEndHour,
		currentState:     SleepStateAwake,
		breathingSamples: make([]BreathingSample, 0, 1440), // ~12 hours at 30s intervals
		motionSamples:    make([]MotionSample, 0, 1440),
		sleepPeriods:     make([]SleepPeriod, 0, 100),
	}
}

// processBreathing processes a breathing sample
func (ss *SleepSession) processBreathing(sample BreathingSample) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	// Check if we're in sleep hours
	if !ss.isSleepHours(sample.Timestamp) {
		return
	}

	// Start session if not active
	if !ss.isActive {
		ss.isActive = true
		ss.sessionDate = ss.getSleepDate(sample.Timestamp)
		ss.metrics = nil // Reset metrics for new session
	}

	ss.breathingSamples = append(ss.breathingSamples, sample)
}

// processMotion processes a motion sample
func (ss *SleepSession) processMotion(sample MotionSample) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	// Check if we're in sleep hours
	if !ss.isSleepHours(sample.Timestamp) {
		return
	}

	// Start session if not active
	if !ss.isActive {
		ss.isActive = true
		ss.sessionDate = ss.getSleepDate(sample.Timestamp)
		ss.metrics = nil
	}

	// Track motion state changes
	ss.updateSleepState(sample)

	ss.motionSamples = append(ss.motionSamples, sample)
}

// updateSleepState updates the sleep state based on motion
func (ss *SleepSession) updateSleepState(sample MotionSample) {
	prevState := ss.currentState

	// Determine new state based on motion
	if sample.MotionDetected {
		if sample.DeltaRMS > RestlessThreshold {
			ss.currentState = SleepStateRestless
		} else {
			ss.currentState = SleepStateLightSleep
		}
	} else {
		// No motion - could be deep sleep or REM
		// Use breathing to distinguish (REM has more irregular breathing)
		if len(ss.breathingSamples) > 0 {
			lastBreath := ss.breathingSamples[len(ss.breathingSamples)-1]
			if lastBreath.IsDetected && lastBreath.Confidence > 0.7 {
				ss.currentState = SleepStateDeepSleep
			} else {
				ss.currentState = SleepStateLightSleep
			}
		} else {
			ss.currentState = SleepStateLightSleep
		}
	}

	// Track sleep periods
	if ss.currentState != prevState {
		ss.handleStateChange(sample.Timestamp, prevState, ss.currentState)
	}
}

// handleStateChange handles sleep state transitions
func (ss *SleepSession) handleStateChange(timestamp time.Time, from, to SleepState) {
	// Close current period if exists
	if ss.currentPeriod != nil {
		ss.currentPeriod.EndTime = timestamp
		ss.currentPeriod.Duration = timestamp.Sub(ss.currentPeriod.StartTime)
		ss.sleepPeriods = append(ss.sleepPeriods, *ss.currentPeriod)

		// Count interruptions (transitions to restless or awake during sleep)
		if to == SleepStateRestless || to == SleepStateAwake {
			// This will be counted in metrics calculation
		}
	}

	// Start new period
	ss.currentPeriod = &SleepPeriod{
		StartTime: timestamp,
		State:     to,
	}
}

// isSleepHours checks if the current time is within configured sleep hours
func (ss *SleepSession) isSleepHours(t time.Time) bool {
	hour := t.Hour()

	// Handle overnight window (e.g., 22:00 - 07:00)
	if ss.sleepStartHour > ss.sleepEndHour {
		// Window spans midnight
		return hour >= ss.sleepStartHour || hour < ss.sleepEndHour
	}
	// Window within same day
	return hour >= ss.sleepStartHour && hour < ss.sleepEndHour
}

// getSleepDate returns the date (midnight) of the sleep session
// For overnight sessions, this is the date at midnight of the night
func (ss *SleepSession) getSleepDate(t time.Time) time.Time {
	if t.Hour() >= ss.sleepStartHour {
		// Evening - sleep date is today
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	}
	// Morning - sleep date is yesterday
	yesterday := t.AddDate(0, 0, -1)
	return time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, t.Location())
}

// GetCurrentState returns the current sleep state
func (ss *SleepSession) GetCurrentState() SleepState {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.currentState
}

// GetMetrics returns computed sleep metrics
func (ss *SleepSession) GetMetrics() *SleepMetrics {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if ss.metrics == nil {
		ss.metrics = ss.computeMetrics()
	}
	return ss.metrics
}

// computeMetrics computes all sleep metrics
func (ss *SleepSession) computeMetrics() *SleepMetrics {
	m := &SleepMetrics{}

	if len(ss.breathingSamples) == 0 && len(ss.motionSamples) == 0 {
		return m
	}

	// Calculate timing
	ss.calculateTiming(m)

	// Calculate breathing metrics
	ss.calculateBreathingMetrics(m)

	// Calculate motion metrics
	ss.calculateMotionMetrics(m)

	// Calculate continuity
	ss.calculateContinuityMetrics(m)

	// Calculate overall score
	ss.calculateOverallScore(m)

	return m
}

// calculateTiming computes timing metrics
func (ss *SleepSession) calculateTiming(m *SleepMetrics) {
	if len(ss.motionSamples) == 0 {
		return
	}

	// Find first and last sample times
	m.SleepStartTime = ss.motionSamples[0].Timestamp
	m.SleepEndTime = ss.motionSamples[len(ss.motionSamples)-1].Timestamp

	if !m.SleepEndTime.IsZero() {
		m.TimeInBed = m.SleepEndTime.Sub(m.SleepStartTime)
	}

	// Count actual sleep time (excluding awake periods)
	for _, period := range ss.sleepPeriods {
		if period.State != SleepStateAwake {
			m.TotalDuration += period.Duration
		}
	}

	// Include current period if sleeping
	if ss.currentPeriod != nil && ss.currentPeriod.State != SleepStateAwake {
		m.TotalDuration += time.Since(ss.currentPeriod.StartTime)
	}
}

// calculateBreathingMetrics computes breathing quality metrics
func (ss *SleepSession) calculateBreathingMetrics(m *SleepMetrics) {
	if len(ss.breathingSamples) == 0 {
		m.BreathingScore = 50 // Default neutral score
		return
	}

	var sum, sumSq float64
	count := 0

	m.MinBreathingRate = 1000
	m.MaxBreathingRate = 0

	for _, sample := range ss.breathingSamples {
		if sample.IsDetected && sample.RateBPM > 0 {
			rate := sample.RateBPM
			sum += rate
			sumSq += rate * rate
			count++

			if rate < m.MinBreathingRate {
				m.MinBreathingRate = rate
			}
			if rate > m.MaxBreathingRate {
				m.MaxBreathingRate = rate
			}
		}
	}

	if count > 0 {
		m.AvgBreathingRate = sum / float64(count)
		variance := sumSq/float64(count) - m.AvgBreathingRate*m.AvgBreathingRate
		m.BreathingRateStdDev = math.Sqrt(math.Max(0, variance))
	}

	// Calculate breathing score (0-100)
	m.BreathingScore = ss.calculateBreathingScore(m.AvgBreathingRate, m.BreathingRateStdDev, m.MinBreathingRate, m.MaxBreathingRate)
}

// calculateBreathingScore computes a score based on breathing quality
func (ss *SleepSession) calculateBreathingScore(avg, stdDev, min, max float64) float64 {
	if avg == 0 {
		return 50
	}

	score := 100.0

	// Penalize deviation from optimal rate
	optimalDiff := math.Abs(avg - BreathingRateOptimal)
	if optimalDiff > 2 {
		score -= math.Min(30, optimalDiff*3)
	}

	// Penalize rates outside normal range
	if min < BreathingRateLow {
		score -= 15
	}
	if max > BreathingRateHigh {
		score -= 15
	}

	// Penalize high variability
	if stdDev > 3 {
		score -= math.Min(20, (stdDev-3)*4)
	}

	return math.Max(0, math.Min(100, score))
}

// calculateMotionMetrics computes motion quality metrics
func (ss *SleepSession) calculateMotionMetrics(m *SleepMetrics) {
	if len(ss.motionSamples) == 0 {
		m.MotionScore = 50
		return
	}

	quietCount := 0
	motionEventCount := 0
	restlessCount := 0

	for _, sample := range ss.motionSamples {
		if sample.DeltaRMS < QuietMotionThreshold {
			quietCount++
		}
		if sample.MotionDetected {
			motionEventCount++
		}
		if sample.DeltaRMS > RestlessThreshold {
			restlessCount++
		}
	}

	total := len(ss.motionSamples)
	m.MotionEvents = motionEventCount
	m.RestlessPeriods = restlessCount
	m.QuietTimePct = float64(quietCount) / float64(total) * 100

	// Motion score based on quiet time percentage
	m.MotionScore = m.QuietTimePct

	// Penalize high motion events
	if motionEventCount > total/10 { // More than 10% motion events
		m.MotionScore -= 10
	}

	m.MotionScore = math.Max(0, math.Min(100, m.MotionScore))
}

// calculateContinuityMetrics computes sleep continuity metrics
func (ss *SleepSession) calculateContinuityMetrics(m *SleepMetrics) {
	// Count interruptions from sleep periods
	for _, period := range ss.sleepPeriods {
		if period.State == SleepStateRestless {
			m.Interruptions++
		}
	}

	// Find longest deep sleep period
	for _, period := range ss.sleepPeriods {
		if period.State == SleepStateDeepSleep && period.Duration > m.LongestDeepPeriod {
			m.LongestDeepPeriod = period.Duration
		}
	}

	// Continuity score based on interruptions and deep sleep
	m.ContinuityScore = 100.0

	// Penalize interruptions
	m.ContinuityScore -= float64(m.Interruptions) * 5

	// Reward long deep sleep periods
	if m.LongestDeepPeriod > 30*time.Minute {
		m.ContinuityScore += math.Min(20, float64(m.LongestDeepPeriod.Minutes())/3)
	}

	// Penalize very short sessions
	if m.TotalDuration < 4*time.Hour {
		m.ContinuityScore -= 30
	} else if m.TotalDuration < 6*time.Hour {
		m.ContinuityScore -= 15
	}

	m.ContinuityScore = math.Max(0, math.Min(100, m.ContinuityScore))
}

// calculateOverallScore computes the overall sleep quality score
func (ss *SleepSession) calculateOverallScore(m *SleepMetrics) {
	// Weighted average of component scores
	m.OverallScore = m.BreathingScore*BreathingWeight +
		m.MotionScore*MotionWeight +
		m.ContinuityScore*ContinuityWeight

	// Assign quality rating
	switch {
	case m.OverallScore >= 80:
		m.QualityRating = "excellent"
	case m.OverallScore >= 60:
		m.QualityRating = "good"
	case m.OverallScore >= 40:
		m.QualityRating = "fair"
	default:
		m.QualityRating = "poor"
	}
}

// GenerateReport generates a sleep report for the current session
func (ss *SleepSession) GenerateReport() *SleepReport {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if !ss.isActive || len(ss.motionSamples) == 0 {
		return nil
	}

	metrics := ss.computeMetrics()

	report := &SleepReport{
		LinkID:       ss.linkID,
		SessionDate:  ss.sessionDate,
		GeneratedAt:  time.Now(),
		Metrics:      metrics,
		BreathingSummary: generateBreathingSummary(metrics),
		MotionSummary:    generateMotionSummary(metrics),
		Recommendations:  generateRecommendations(metrics),
	}

	return report
}

// Reset clears the session state for a new night
func (ss *SleepSession) Reset() {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	ss.currentState = SleepStateAwake
	ss.isActive = false
	ss.sessionDate = time.Time{}
	ss.breathingSamples = make([]BreathingSample, 0, 1440)
	ss.motionSamples = make([]MotionSample, 0, 1440)
	ss.sleepPeriods = make([]SleepPeriod, 0, 100)
	ss.currentPeriod = nil
	ss.metrics = nil
}

// GetBreathingSamples returns all breathing samples for the session
func (ss *SleepSession) GetBreathingSamples() []BreathingSample {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	result := make([]BreathingSample, len(ss.breathingSamples))
	copy(result, ss.breathingSamples)
	return result
}

// GetMotionSamples returns all motion samples for the session
func (ss *SleepSession) GetMotionSamples() []MotionSample {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	result := make([]MotionSample, len(ss.motionSamples))
	copy(result, ss.motionSamples)
	return result
}

// IsInSleepHours checks if current time is within sleep hours
func (ss *SleepSession) IsInSleepHours() bool {
	return ss.isSleepHours(time.Now())
}
