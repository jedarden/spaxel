// Package sleep implements overnight sleep analysis and reporting.
package sleep

import (
	"fmt"
	"math"
	"sync"
	"time"
)

// Sleep state constants
const (
	// Sleep window defaults (can be overridden)
	DefaultSleepStartHour = 22 // 10 PM
	DefaultSleepEndHour   = 7  // 7 AM

	// Session confirmation thresholds
	SessionConfirmDuration = 15 * time.Minute // Must be stationary for 15 min to confirm sleep onset
	WakeConfirmDuration    = 2 * time.Minute  // Must be moving for 2 min to confirm wake

	// Scoring weights
	BreathingWeight = 0.4
	MotionWeight    = 0.3
	ContinuityWeight = 0.3

	// Breathing quality thresholds
	BreathingRateLow  = 10.0 // BPM - below this is concerning
	BreathingRateHigh = 25.0 // BPM - above this is concerning
	BreathingRateOptimal = 14.0 // BPM - optimal breathing rate

	// Breathing anomaly thresholds (per task spec: <8 or >25 bpm)
	BreathingAnomalyLow  = 8.0  // BPM - apnea indicator
	BreathingAnomalyHigh = 25.0 // BPM - hyperventilation indicator
	BreathingAnomalyDurationThreshold = 3 * time.Minute

	// Motion thresholds (deltaRMS)
	QuietMotionThreshold = 0.015 // Below this is considered quiet
	RestlessThreshold    = 0.04  // Above this is restless
	WakeMotionThreshold  = 0.03  // Above this indicates potential wake episode

	// Wake episode thresholds
	WakeEpisodeMinDuration = 3 * time.Second // Minimum duration to count as wake episode

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
	SleepOnsetTime  time.Time `json:"sleep_onset_time,omitempty"` // When sleep was confirmed (15 min stationary)
	TotalDuration   time.Duration `json:"total_duration"`
	TimeInBed       time.Duration `json:"time_in_bed"`

	// Sleep efficiency (per task spec: (time_in_bed - waso) / time_in_bed * 100)
	SleepEfficiency     float64 `json:"sleep_efficiency"`      // 0-100%
	SleepLatencyMinutes float64 `json:"sleep_latency_minutes"` // Time from entering bedroom to sleep onset
	WASOMinutes         float64 `json:"waso_minutes"`          // Wake After Sleep Onset
	WakeEpisodeCount    int     `json:"wake_episode_count"`

	// Breathing metrics
	AvgBreathingRate    float64 `json:"avg_breathing_rate"`
	MinBreathingRate    float64 `json:"min_breathing_rate"`
	MaxBreathingRate    float64 `json:"max_breathing_rate"`
	BreathingRateStdDev float64 `json:"breathing_rate_std_dev"`
	BreathingRegularity float64 `json:"breathing_regularity"` // CV (std/mean)
	BreathingScore      float64 `json:"breathing_score"` // 0-100
	BreathingAnomalyCount int   `json:"breathing_anomaly_count"` // Anomalies < 8 or > 25 bpm
	BreathingAnomaly    bool    `json:"breathing_anomaly"`    // Elevated vs personal average
	PersonalAvgBPM      float64 `json:"personal_avg_bpm,omitempty"` // Person's rolling average for comparison
	BreathingSamplesJSON string `json:"breathing_samples_json,omitempty"` // Raw samples for storage

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

// Breathing anomaly thresholds are defined above (lines 32-34)

// WakeEpisode represents a period of wakefulness during sleep
type WakeEpisode struct {
	ID              string        `json:"id"`
	SessionID       string        `json:"session_id,omitempty"`
	EpisodeStart    time.Time     `json:"episode_start"`
	EpisodeEnd      time.Time     `json:"episode_end,omitempty"`
	Duration        time.Duration `json:"duration"`
	DurationSeconds float64       `json:"duration_seconds"`
}

// BreathingAnomaly represents a detected breathing anomaly
type BreathingAnomaly struct {
	ID          string    `json:"id"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time,omitempty"`
	RateBPM     float64   `json:"rate_bpm"`
	AnomalyType string    `json:"anomaly_type"` // "low" or "high"
	Duration    time.Duration `json:"duration"`
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

	// Session timing
	sessionStart   time.Time // When person entered bedroom/started tracking
	sleepOnset     time.Time // When sleep was confirmed (15 min after stationary detection)
	wakeTime       time.Time // When session ended

	// Sample buffers
	breathingSamples []BreathingSample
	motionSamples    []MotionSample

	// Period tracking
	sleepPeriods []SleepPeriod
	currentPeriod *SleepPeriod

	// Wake episode tracking
	wakeEpisodes     []WakeEpisode
	currentWakeEpisode *WakeEpisode
	wakeEpisodeStart time.Time // Track when current wake period started

	// Breathing anomaly tracking
	breathingAnomalies []BreathingAnomaly
	currentAnomaly     *BreathingAnomaly
	anomalyStartTime   time.Time
	anomalyType        string

	// Zone and identity
	zoneID   string
	personID string

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

	// Breathing anomaly tracking (per-person rolling baseline)
	anomalyTracker *BreathingAnomalyTracker

	// Report callback
	onReportGenerated func(linkID string, report *SleepReport)
}

// NewSleepAnalyzer creates a new sleep analyzer
func NewSleepAnalyzer() *SleepAnalyzer {
	return &SleepAnalyzer{
		sessions:        make(map[string]*SleepSession),
		sleepStartHour:  DefaultSleepStartHour,
		sleepEndHour:    DefaultSleepEndHour,
		anomalyTracker:  NewBreathingAnomalyTracker(),
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

// GenerateMorningReports generates reports for all completed sleep sessions.
// It also checks breathing anomalies against personal baselines and updates them.
func (sa *SleepAnalyzer) GenerateMorningReports() map[string]*SleepReport {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	reports := make(map[string]*SleepReport)
	for linkID, session := range sa.sessions {
		if report := session.GenerateReport(); report != nil {
			// Check breathing anomaly against personal baseline
			person := session.personID
			if person == "" {
				person = linkID
			}
			if report.Metrics.AvgBreathingRate > 0 {
				personalAvg := sa.anomalyTracker.GetPersonalAverage(person)
				report.Metrics.PersonalAvgBPM = personalAvg
				isAnomaly := sa.anomalyTracker.CheckAnomaly(person, report.Metrics.AvgBreathingRate)
				report.Metrics.BreathingAnomaly = isAnomaly
				// Update personal rolling average after checking
				sa.anomalyTracker.UpdatePersonalAverage(person, report.Metrics.AvgBreathingRate)
			}

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

// GetAnomalyTracker returns the breathing anomaly tracker for external access
// (e.g., loading/saving personal baselines from SQLite).
func (sa *SleepAnalyzer) GetAnomalyTracker() *BreathingAnomalyTracker {
	return sa.anomalyTracker
}

// SetPersonID sets the person identity for a sleep session link.
func (sa *SleepAnalyzer) SetPersonID(linkID, personID string) {
	sa.mu.Lock()
	defer sa.mu.Unlock()
	if session, exists := sa.sessions[linkID]; exists {
		session.mu.Lock()
		session.personID = personID
		session.mu.Unlock()
	}
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
		wakeEpisodes:     make([]WakeEpisode, 0, 50),
		breathingAnomalies: make([]BreathingAnomaly, 0, 20),
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
		ss.sessionStart = sample.Timestamp
		ss.metrics = nil // Reset metrics for new session
	}

	ss.breathingSamples = append(ss.breathingSamples, sample)

	// Detect breathing anomalies (apnea/hyperventilation indicators)
	if sample.IsDetected && sample.RateBPM > 0 {
		ss.detectBreathingAnomaly(sample)
	}
}

// detectBreathingAnomaly checks for breathing rates outside normal range
func (ss *SleepSession) detectBreathingAnomaly(sample BreathingSample) {
	isAnomalous := false
	anomalyType := ""

	if sample.RateBPM < BreathingAnomalyLow && sample.RateBPM > 0 {
		isAnomalous = true
		anomalyType = "low" // Potential apnea
	} else if sample.RateBPM > BreathingAnomalyHigh {
		isAnomalous = true
		anomalyType = "high" // Potential hyperventilation
	}

	if isAnomalous {
		if ss.anomalyStartTime.IsZero() {
			// Start tracking potential anomaly
			ss.anomalyStartTime = sample.Timestamp
			ss.anomalyType = anomalyType
		} else if ss.anomalyType == anomalyType {
			// Continue tracking same type of anomaly
			duration := sample.Timestamp.Sub(ss.anomalyStartTime)
			if duration >= BreathingAnomalyDurationThreshold && ss.currentAnomaly == nil {
				// Anomaly persisted for 3+ minutes - record it
				ss.currentAnomaly = &BreathingAnomaly{
					ID:          fmt.Sprintf("%s-%d", ss.linkID, ss.anomalyStartTime.Unix()),
					StartTime:   ss.anomalyStartTime,
					RateBPM:     sample.RateBPM,
					AnomalyType: anomalyType,
				}
				ss.breathingAnomalies = append(ss.breathingAnomalies, *ss.currentAnomaly)
			}
		} else {
			// Different anomaly type - reset tracking
			ss.anomalyStartTime = sample.Timestamp
			ss.anomalyType = anomalyType
		}
	} else {
		// Breathing returned to normal - close any ongoing anomaly
		if ss.currentAnomaly != nil {
			ss.currentAnomaly.EndTime = sample.Timestamp
			ss.currentAnomaly.Duration = sample.Timestamp.Sub(ss.currentAnomaly.StartTime)
			// Update the last anomaly in the slice
			if len(ss.breathingAnomalies) > 0 {
				ss.breathingAnomalies[len(ss.breathingAnomalies)-1] = *ss.currentAnomaly
			}
			ss.currentAnomaly = nil
		}
		ss.anomalyStartTime = time.Time{}
		ss.anomalyType = ""
	}
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
		ss.sessionStart = sample.Timestamp
		ss.metrics = nil
	}

	// Track motion state changes
	ss.updateSleepState(sample)

	// Track wake episodes during confirmed sleep
	if ss.sleepOnsetConfirmed() {
		ss.trackWakeEpisode(sample)
	}

	ss.motionSamples = append(ss.motionSamples, sample)
}

// sleepOnsetConfirmed returns true if sleep onset has been confirmed (15 min of stationary)
func (ss *SleepSession) sleepOnsetConfirmed() bool {
	return !ss.sleepOnset.IsZero()
}

// trackWakeEpisode tracks wake episodes during sleep
func (ss *SleepSession) trackWakeEpisode(sample MotionSample) {
	// Wake episode starts when motion > threshold for sustained period
	if sample.DeltaRMS > RestlessThreshold || sample.MotionDetected {
		if ss.wakeEpisodeStart.IsZero() {
			// Start tracking potential wake episode
			ss.wakeEpisodeStart = sample.Timestamp
		} else {
			// Check if this has been sustained long enough
			duration := sample.Timestamp.Sub(ss.wakeEpisodeStart)
			if duration >= WakeEpisodeMinDuration && ss.currentWakeEpisode == nil {
				// Create new wake episode
				ss.currentWakeEpisode = &WakeEpisode{
					ID:           fmt.Sprintf("%s-wake-%d", ss.linkID, ss.wakeEpisodeStart.Unix()),
					EpisodeStart: ss.wakeEpisodeStart,
				}
			}
		}
	} else {
		// Motion returned to quiet - close any ongoing wake episode
		if ss.currentWakeEpisode != nil {
			ss.currentWakeEpisode.EpisodeEnd = sample.Timestamp
			ss.currentWakeEpisode.Duration = sample.Timestamp.Sub(ss.currentWakeEpisode.EpisodeStart)
			ss.wakeEpisodes = append(ss.wakeEpisodes, *ss.currentWakeEpisode)
			ss.currentWakeEpisode = nil
		}
		ss.wakeEpisodeStart = time.Time{}
	}
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

	// Set sleep onset if confirmed
	if !ss.sleepOnset.IsZero() {
		m.SleepOnsetTime = ss.sleepOnset
	}

	// Calculate time in bed
	if !m.SleepEndTime.IsZero() {
		m.TimeInBed = m.SleepEndTime.Sub(m.SleepStartTime)
	}

	// Calculate sleep latency (time from entering bed to sleep onset)
	if !ss.sleepOnset.IsZero() && !ss.sessionStart.IsZero() {
		m.SleepLatencyMinutes = ss.sleepOnset.Sub(ss.sessionStart).Minutes()
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

	// Calculate WASO (Wake After Sleep Onset) from wake episodes
	m.WakeEpisodeCount = len(ss.wakeEpisodes)
	var wasoDuration time.Duration
	for _, episode := range ss.wakeEpisodes {
		// Only count episodes after sleep onset
		if episode.EpisodeStart.After(ss.sleepOnset) {
			wasoDuration += episode.Duration
		}
	}
	m.WASOMinutes = wasoDuration.Minutes()

	// Calculate sleep efficiency: (time_in_bed - waso) / time_in_bed * 100
	// Per task spec: a value above 85% is considered good sleep efficiency
	if m.TimeInBed > 0 {
		effectiveSleep := m.TimeInBed - wasoDuration
		m.SleepEfficiency = (float64(effectiveSleep) / float64(m.TimeInBed)) * 100
		// Cap at 100%
		if m.SleepEfficiency > 100 {
			m.SleepEfficiency = 100
		}
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

	// Count breathing anomalies (per task spec: < 8 or > 25 bpm for > 3 minutes)
	m.BreathingAnomalyCount = len(ss.breathingAnomalies)

	// Compute breathing regularity (coefficient of variation)
	m.BreathingRegularity = ss.computeBreathingRegularity()

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
	if stdDev > 2 {
		score -= math.Min(35, (stdDev-2)*10)
	}

	return math.Max(0, math.Min(100, score))
}

// computeBreathingRegularity computes CV (std/mean) of detected breathing rates.
func (ss *SleepSession) computeBreathingRegularity() float64 {
	var rates []float64
	for _, sample := range ss.breathingSamples {
		if sample.IsDetected && sample.RateBPM > 0 {
			rates = append(rates, sample.RateBPM)
		}
	}
	return ComputeBreathingRegularity(rates)
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

// GetPersonID returns the person identity for this session.
func (ss *SleepSession) GetPersonID() string {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.personID
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
