package sleep

import (
	"log"
	"sync"
	"time"

	"github.com/spaxel/mothership/internal/signal"
)

// Monitor integrates the sleep analyzer with the signal processing pipeline.
// It periodically samples breathing and motion data during sleep hours.
type Monitor struct {
	mu sync.RWMutex

	// Dependencies
	analyzer     *SleepAnalyzer
	processorMgr *signal.ProcessorManager

	// Configuration
	sampleInterval   time.Duration
	reportHour       int // Hour of day to generate morning reports (0-23)
	sleepStartHour   int
	sleepEndHour     int

	// State
	running     bool
	stopCh      chan struct{}
	lastSample  map[string]time.Time
	lastReport  time.Time
}

// MonitorConfig holds configuration for the sleep monitor
type MonitorConfig struct {
	SampleInterval time.Duration // How often to sample data (default 30s)
	ReportHour     int           // Hour to generate morning reports (default 7)
	SleepStartHour int           // Start of sleep window (default 22)
	SleepEndHour   int           // End of sleep window (default 7)
}

// NewMonitor creates a new sleep monitor
func NewMonitor(cfg MonitorConfig) *Monitor {
	if cfg.SampleInterval == 0 {
		cfg.SampleInterval = SampleInterval
	}
	if cfg.ReportHour == 0 {
		cfg.ReportHour = 7
	}
	if cfg.SleepStartHour == 0 {
		cfg.SleepStartHour = DefaultSleepStartHour
	}
	if cfg.SleepEndHour == 0 {
		cfg.SleepEndHour = DefaultSleepEndHour
	}

	analyzer := NewSleepAnalyzer()
	analyzer.SetSleepWindow(cfg.SleepStartHour, cfg.SleepEndHour)

	return &Monitor{
		analyzer:       analyzer,
		sampleInterval: cfg.SampleInterval,
		reportHour:     cfg.ReportHour,
		sleepStartHour: cfg.SleepStartHour,
		sleepEndHour:   cfg.SleepEndHour,
		stopCh:         make(chan struct{}),
		lastSample:     make(map[string]time.Time),
	}
}

// SetProcessorManager sets the signal processor manager
func (m *Monitor) SetProcessorManager(pm *signal.ProcessorManager) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processorMgr = pm
}

// SetReportCallback sets the callback for when reports are generated
func (m *Monitor) SetReportCallback(cb func(linkID string, report *SleepReport)) {
	m.analyzer.SetReportCallback(cb)
}

// Start starts the sleep monitoring loop
func (m *Monitor) Start() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.mu.Unlock()

	go m.runLoop()
	log.Printf("[INFO] Sleep monitor started (window: %d:00-%d:00, report at %d:00)",
		m.sleepStartHour, m.sleepEndHour, m.reportHour)
}

// Stop stops the sleep monitoring loop
func (m *Monitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return
	}

	m.running = false
	close(m.stopCh)
	log.Printf("[INFO] Sleep monitor stopped")
}

// runLoop is the main monitoring loop
func (m *Monitor) runLoop() {
	sampleTicker := time.NewTicker(m.sampleInterval)
	defer sampleTicker.Stop()

	reportTicker := time.NewTicker(1 * time.Minute)
	defer reportTicker.Stop()

	for {
		select {
		case <-m.stopCh:
			return

		case <-sampleTicker.C:
			m.collectSamples()

		case <-reportTicker.C:
			m.checkReportGeneration()
		}
	}
}

// collectSamples collects breathing and motion samples from all links
func (m *Monitor) collectSamples() {
	m.mu.RLock()
	pm := m.processorMgr
	analyzer := m.analyzer
	sleepStartHour := m.sleepStartHour
	sleepEndHour := m.sleepEndHour
	m.mu.RUnlock()

	if pm == nil {
		return
	}

	now := time.Now()
	hour := now.Hour()

	// Check if we're in sleep hours
	inSleepHours := false
	if sleepStartHour > sleepEndHour {
		// Window spans midnight
		inSleepHours = hour >= sleepStartHour || hour < sleepEndHour
	} else {
		inSleepHours = hour >= sleepStartHour && hour < sleepEndHour
	}

	if !inSleepHours {
		return
	}

	// Get all link states
	states := pm.GetAllMotionStates()

	for _, state := range states {
		// Throttle sampling per link
		if last, exists := m.lastSample[state.LinkID]; exists {
			if now.Sub(last) < m.sampleInterval {
				continue
			}
		}
		m.lastSample[state.LinkID] = now

		// Create motion sample
		motionSample := MotionSample{
			Timestamp:      now,
			DeltaRMS:       state.SmoothDeltaRMS,
			MotionDetected: state.MotionDetected,
		}
		analyzer.ProcessMotion(state.LinkID, motionSample)

		// Create breathing sample
		breathingSample := BreathingSample{
			Timestamp:   now,
			RateBPM:     state.BreathingRate,
			Confidence:  state.AmbientConfidence,
			IsDetected:  state.BreathingDetected,
			HealthGated: false,
		}
		analyzer.ProcessBreathing(state.LinkID, breathingSample)
	}
}

// checkReportGeneration checks if it's time to generate morning reports
func (m *Monitor) checkReportGeneration() {
	now := time.Now()

	// Check if it's report hour and we haven't reported today
	if now.Hour() == m.reportHour {
		reportDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

		m.mu.Lock()
		lastReport := m.lastReport
		m.mu.Unlock()

		// Only generate if we haven't reported today
		if lastReport.IsZero() || lastReport.Before(reportDate) {
			m.generateMorningReports()
			m.mu.Lock()
			m.lastReport = now
			m.mu.Unlock()
		}
	}
}

// generateMorningReports generates reports for all sessions
func (m *Monitor) generateMorningReports() {
	reports := m.analyzer.GenerateMorningReports()

	for linkID, report := range reports {
		log.Printf("[INFO] Sleep report generated for %s: score=%.1f rating=%s",
			linkID, report.Metrics.OverallScore, report.Metrics.QualityRating)
	}
}

// GetAnalyzer returns the sleep analyzer for direct access
func (m *Monitor) GetAnalyzer() *SleepAnalyzer {
	return m.analyzer
}

// GetCurrentState returns the current sleep state for a link
func (m *Monitor) GetCurrentState(linkID string) SleepState {
	return m.analyzer.GetCurrentState(linkID)
}

// GetAllSessions returns all current sleep sessions
func (m *Monitor) GetAllSessions() map[string]*SleepSession {
	return m.analyzer.GetAllSessions()
}

// GetSleepReport generates a report for a specific link
func (m *Monitor) GetSleepReport(linkID string) *SleepReport {
	session := m.analyzer.GetSession(linkID)
	if session == nil {
		return nil
	}
	return session.GenerateReport()
}

// ForceReportGeneration forces generation of reports for all sessions
func (m *Monitor) ForceReportGeneration() map[string]*SleepReport {
	return m.analyzer.GenerateMorningReports()
}

// IsInSleepHours returns whether current time is within sleep hours
func (m *Monitor) IsInSleepHours() bool {
	now := time.Now()
	hour := now.Hour()

	if m.sleepStartHour > m.sleepEndHour {
		return hour >= m.sleepStartHour || hour < m.sleepEndHour
	}
	return hour >= m.sleepStartHour && hour < m.sleepEndHour
}

// SleepStatus represents the current sleep monitoring status
type SleepStatus struct {
	InSleepHours   bool                      `json:"in_sleep_hours"`
	SleepStartHour int                       `json:"sleep_start_hour"`
	SleepEndHour   int                       `json:"sleep_end_hour"`
	ActiveSessions int                       `json:"active_sessions"`
	LinkStates     map[string]SleepLinkState `json:"link_states"`
}

// SleepLinkState represents sleep state for a single link
type SleepLinkState struct {
	LinkID          string    `json:"link_id"`
	SleepState      string    `json:"sleep_state"`
	SamplesCollected int      `json:"samples_collected"`
	SessionActive   bool      `json:"session_active"`
	CurrentBreathingRate float64 `json:"current_breathing_rate"`
	CurrentMotion      bool     `json:"current_motion"`
}

// GetStatus returns the current sleep monitoring status
func (m *Monitor) GetStatus() SleepStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := SleepStatus{
		InSleepHours:   m.IsInSleepHours(),
		SleepStartHour: m.sleepStartHour,
		SleepEndHour:   m.sleepEndHour,
		LinkStates:     make(map[string]SleepLinkState),
	}

	sessions := m.analyzer.GetAllSessions()
	status.ActiveSessions = len(sessions)

	for linkID, session := range sessions {
		state := SleepLinkState{
			LinkID:     linkID,
			SleepState: session.GetCurrentState().String(),
			SessionActive: session.isActive,
		}

		session.mu.RLock()
		state.SamplesCollected = len(session.breathingSamples) + len(session.motionSamples)

		// Get latest breathing rate
		if len(session.breathingSamples) > 0 {
			state.CurrentBreathingRate = session.breathingSamples[len(session.breathingSamples)-1].RateBPM
		}

		// Get latest motion state
		if len(session.motionSamples) > 0 {
			state.CurrentMotion = session.motionSamples[len(session.motionSamples)-1].MotionDetected
		}
		session.mu.RUnlock()

		status.LinkStates[linkID] = state
	}

	return status
}
