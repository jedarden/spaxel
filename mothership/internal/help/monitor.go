// Package help provides feature discovery monitoring and notification.
package help

import (
	"database/sql"
	"log"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// FeatureMonitor checks for feature availability and fires notifications.
// It runs periodically to check if features have become available.
type FeatureMonitor struct {
	mu               sync.Mutex
	db               *sql.DB
	notifier         *Notifier
	checkInterval    time.Duration
	stopCh           chan struct{}
	wg               sync.WaitGroup

	// Callbacks for checking feature availability
	checkDiurnalReady      func() bool
	checkFirstSleepSession func() bool
	checkWeightUpdate      func() bool
	checkFirstAutomation   func() bool
	checkPredictionReady   func(personID string) bool

	// Track what we've already notified
	notifiedDiurnalReady      bool
	notifiedFirstSleepSession bool
	notifiedWeightUpdate      bool
	notifiedFirstAutomation   bool
	notifiedPredictionReady   map[string]bool // personID -> notified
}

// FeatureMonitorConfig holds configuration for the feature monitor.
type FeatureMonitorConfig struct {
	DB            *sql.DB
	Notifier      *Notifier
	CheckInterval time.Duration // How often to check for new features
}

// NewFeatureMonitor creates a new feature discovery monitor.
func NewFeatureMonitor(cfg FeatureMonitorConfig) *FeatureMonitor {
	if cfg.CheckInterval == 0 {
		cfg.CheckInterval = 5 * time.Minute // Check every 5 minutes
	}

	return &FeatureMonitor{
		db:                    cfg.DB,
		notifier:              cfg.Notifier,
		checkInterval:         cfg.CheckInterval,
		stopCh:                make(chan struct{}),
		notifiedPredictionReady: make(map[string]bool),
	}
}

// SetDiurnalReadyChecker sets the callback to check if diurnal baseline is ready.
func (m *FeatureMonitor) SetDiurnalReadyChecker(fn func() bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkDiurnalReady = fn
}

// SetFirstSleepSessionChecker sets the callback to check if first sleep session is complete.
func (m *FeatureMonitor) SetFirstSleepSessionChecker(fn func() bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkFirstSleepSession = fn
}

// SetWeightUpdateChecker sets the callback to check if weight update is approved.
func (m *FeatureMonitor) SetWeightUpdateChecker(fn func() bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkWeightUpdate = fn
}

// SetFirstAutomationChecker sets the callback to check if first automation has fired.
func (m *FeatureMonitor) SetFirstAutomationChecker(fn func() bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkFirstAutomation = fn
}

// SetPredictionReadyChecker sets the callback to check if prediction model is ready for a person.
func (m *FeatureMonitor) SetPredictionReadyChecker(fn func(personID string) bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkPredictionReady = fn
}

// Start begins the monitoring loop.
func (m *FeatureMonitor) Start() {
	m.wg.Add(1)
	go m.monitorLoop()
	log.Printf("[INFO] Feature discovery monitor started (check interval: %v)", m.checkInterval)
}

// Stop gracefully stops the monitor.
func (m *FeatureMonitor) Stop() {
	close(m.stopCh)
	m.wg.Wait()
	log.Printf("[INFO] Feature discovery monitor stopped")
}

// monitorLoop runs the periodic check for feature availability.
func (m *FeatureMonitor) monitorLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()

	// Run initial check
	m.checkAllFeatures()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkAllFeatures()
		}
	}
}

// checkAllFeatures checks all feature availability conditions.
func (m *FeatureMonitor) checkAllFeatures() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check diurnal baseline activation
	if m.checkDiurnalReady != nil && !m.notifiedDiurnalReady {
		if m.checkDiurnalReady() {
			m.notifier.FireNotification(
				EventDiurnalBaselineActivated,
				getNotificationTitle(EventDiurnalBaselineActivated),
				getNotificationMessage(EventDiurnalBaselineActivated),
			)
			m.notifiedDiurnalReady = true
			log.Printf("[INFO] Feature notification fired: %s", EventDiurnalBaselineActivated)
		}
	}

	// Check first sleep session
	if m.checkFirstSleepSession != nil && !m.notifiedFirstSleepSession {
		if m.checkFirstSleepSession() {
			m.notifier.FireNotification(
				EventFirstSleepSessionComplete,
				getNotificationTitle(EventFirstSleepSessionComplete),
				getNotificationMessage(EventFirstSleepSessionComplete),
			)
			m.notifiedFirstSleepSession = true
			log.Printf("[INFO] Feature notification fired: %s", EventFirstSleepSessionComplete)
		}
	}

	// Check weight update approval
	if m.checkWeightUpdate != nil && !m.notifiedWeightUpdate {
		if m.checkWeightUpdate() {
			m.notifier.FireNotification(
				EventWeightUpdateApproved,
				getNotificationTitle(EventWeightUpdateApproved),
				getNotificationMessage(EventWeightUpdateApproved),
			)
			m.notifiedWeightUpdate = true
			log.Printf("[INFO] Feature notification fired: %s", EventWeightUpdateApproved)
		}
	}

	// Check first automation
	if m.checkFirstAutomation != nil && !m.notifiedFirstAutomation {
		if m.checkFirstAutomation() {
			m.notifier.FireNotification(
				EventAutomationFirstFired,
				getNotificationTitle(EventAutomationFirstFired),
				getNotificationMessage(EventAutomationFirstFired),
			)
			m.notifiedFirstAutomation = true
			log.Printf("[INFO] Feature notification fired: %s", EventAutomationFirstFired)
		}
	}

	// Check prediction model readiness for each person
	if m.checkPredictionReady != nil {
		// Get list of persons from database
		persons := m.getPersonsWithPredictionModels()
		for _, personID := range persons {
			if !m.notifiedPredictionReady[personID] {
				if m.checkPredictionReady(personID) {
					// Use person-specific event ID
					eventID := PredictionModelReadyEventID(personID)
					m.notifier.FireNotification(
						eventID,
						getPersonNotificationTitle(personID, EventPredictionModelReady),
						getPersonNotificationMessage(personID, EventPredictionModelReady),
					)
					m.notifiedPredictionReady[personID] = true
					log.Printf("[INFO] Feature notification fired: prediction model ready for person %s", personID)
				}
			}
		}
	}
}

// getPersonsWithPredictionModels returns a list of person IDs with prediction models.
func (m *FeatureMonitor) getPersonsWithPredictionModels() []string {
	// Query the prediction_models table for persons
	rows, err := m.db.Query(`
		SELECT DISTINCT person FROM prediction_models
		WHERE sample_count >= 3
		ORDER BY person
	`)
	if err != nil {
		log.Printf("[WARN] Failed to query prediction models: %v", err)
		return nil
	}
	defer rows.Close()

	var persons []string
	for rows.Next() {
		var person string
		if err := rows.Scan(&person); err != nil {
			continue
		}
		persons = append(persons, person)
	}

	return persons
}

// PredictionModelReadyEventID returns the event ID for a person's prediction model readiness.
func PredictionModelReadyEventID(personID string) string {
	return EventPredictionModelReady + "_" + personID
}

// getPersonNotificationTitle returns a person-specific notification title.
func getPersonNotificationTitle(personID, baseEvent string) string {
	switch baseEvent {
	case EventPredictionModelReady:
		return "Presence predictions are now available for " + personID
	default:
		return getNotificationTitle(baseEvent)
	}
}

// getPersonNotificationMessage returns a person-specific notification message.
func getPersonNotificationMessage(personID, baseEvent string) string {
	switch baseEvent {
	case EventPredictionModelReady:
		return "The system has learned when " + personID + " is typically in each room. " +
			"Predictions appear in the Predictions panel. Accuracy will continue to improve over the coming days."
	default:
		return getNotificationMessage(baseEvent)
	}
}
