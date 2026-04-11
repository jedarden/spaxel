// Package notifications provides a notification manager with smart batching,
// quiet hours filtering, and SQLite persistence for configuration.
package notifications

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// EventType represents the type of notification event.
type EventType string

const (
	ZoneEnter      EventType = "zone_enter"
	ZoneLeave      EventType = "zone_leave"
	ZoneVacant     EventType = "zone_vacant"
	FallDetected   EventType = "fall_detected"
	FallEscalation EventType = "fall_escalation"
	AnomalyAlert   EventType = "anomaly_alert"
	NodeOffline    EventType = "node_offline"
	SleepSummary   EventType = "sleep_summary"
)

// Priority represents the urgency level of a notification.
type Priority int

const (
	Low    Priority = 1
	Medium Priority = 2
	High   Priority = 3
	Urgent Priority = 4
)

// String returns the string representation of the priority.
func (p Priority) String() string {
	switch p {
	case Low:
		return "LOW"
	case Medium:
		return "MEDIUM"
	case High:
		return "HIGH"
	case Urgent:
		return "URGENT"
	default:
		return "UNKNOWN"
	}
}

// Event represents a notification event.
type Event struct {
	ID        string                 `json:"id"`
	Type      EventType              `json:"type"`
	Priority  Priority               `json:"priority"`
	Title     string                 `json:"title"`
	Body      string                 `json:"body"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// NotificationConfig holds configuration for notification channels.
type NotificationConfig struct {
	Channel          string `json:"channel"`
	QuietFrom        string `json:"quiet_from"`        // HH:MM format
	QuietTo          string `json:"quiet_to"`          // HH:MM format
	QuietDaysBitmask uint8  `json:"quiet_days_bitmask"` // Bitmask (0=Sun, 1=Mon, ..., 6=Sat)
	MorningDigest    bool   `json:"morning_digest"`
}

// NotificationManager manages notification delivery with batching and quiet hours.
type NotificationManager struct {
	mu                sync.RWMutex
	db                *sql.DB
	config            *NotificationConfig
	batchWindow       time.Duration
	maxBatchSize      int
	pendingLow        []*Event
	pendingMedium     []*Event
	batchTimer        *time.Timer
	queuedForDigest   []*Event
	sendCallback      func(Event)
	digestSentDate    string // YYYY-MM-DD
	location          *time.Location
}

// Config holds initialization options for NotificationManager.
type Config struct {
	DBPath            string        // Path to SQLite database
	BatchWindowSec    int           // Batching window in seconds (default 30)
	MaxBatchSize      int           // Max events before forcing batch (default 5)
	Location          *time.Location // Timezone for quiet hours (default UTC)
	SendCallback      func(Event)   // Callback for sending notifications
}

// New creates a new NotificationManager.
func New(cfg Config) (*NotificationManager, error) {
	if cfg.DBPath == "" {
		return nil, fmt.Errorf("DBPath is required")
	}

	// Create data directory if needed
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	// Open database
	db, err := sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	// Set defaults
	batchWindow := 30 * time.Second
	if cfg.BatchWindowSec > 0 {
		batchWindow = time.Duration(cfg.BatchWindowSec) * time.Second
	}

	maxBatchSize := 5
	if cfg.MaxBatchSize > 0 {
		maxBatchSize = cfg.MaxBatchSize
	}

	location := cfg.Location
	if location == nil {
		location = time.UTC
	}

	m := &NotificationManager{
		db:             db,
		batchWindow:    batchWindow,
		maxBatchSize:   maxBatchSize,
		sendCallback:   cfg.SendCallback,
		location:       location,
		pendingLow:     make([]*Event, 0, maxBatchSize),
		pendingMedium:  make([]*Event, 0, maxBatchSize),
		queuedForDigest: make([]*Event, 0),
	}

	if err := m.initDB(); err != nil {
		db.Close()
		return nil, fmt.Errorf("init database: %w", err)
	}

	if err := m.loadConfig(); err != nil {
		log.Printf("[WARN] Failed to load config: %v", err)
	}

	// Start morning digest checker
	go m.digestChecker()

	return m, nil
}

// initDB creates the database schema.
func (m *NotificationManager) initDB() error {
	_, err := m.db.Exec(`
		CREATE TABLE IF NOT EXISTS notifications_config (
			channel TEXT PRIMARY KEY,
			quiet_from TEXT NOT NULL DEFAULT '22:00',
			quiet_to TEXT NOT NULL DEFAULT '07:00',
			quiet_days_bitmask INTEGER NOT NULL DEFAULT 127,
			morning_digest INTEGER NOT NULL DEFAULT 1
		);

		CREATE TABLE IF NOT EXISTS notifications_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_type TEXT NOT NULL,
			priority INTEGER NOT NULL,
			title TEXT NOT NULL,
			body TEXT NOT NULL,
			sent_at INTEGER NOT NULL,
			was_batched INTEGER NOT NULL DEFAULT 0,
			was_queued INTEGER NOT NULL DEFAULT 0
		);

		CREATE INDEX IF NOT EXISTS idx_notifications_history_sent_at
			ON notifications_history(sent_at DESC);
	`)
	return err
}

// loadConfig loads notification configuration from database.
func (m *NotificationManager) loadConfig() error {
	var channel, quietFrom, quietTo string
	var quietDaysBitmask uint8
	var morningDigest int

	row := m.db.QueryRow(`SELECT channel, quiet_from, quiet_to, quiet_days_bitmask, morning_digest
		FROM notifications_config LIMIT 1`)

	err := row.Scan(&channel, &quietFrom, &quietTo, &quietDaysBitmask, &morningDigest)
	if err == sql.ErrNoRows {
		// Insert default config
		m.config = &NotificationConfig{
			Channel:          "default",
			QuietFrom:        "22:00",
			QuietTo:          "07:00",
			QuietDaysBitmask: 0x7F, // All days
			MorningDigest:    true,
		}
		return m.saveConfig()
	}
	if err != nil {
		return err
	}

	m.config = &NotificationConfig{
		Channel:          channel,
		QuietFrom:        quietFrom,
		QuietTo:          quietTo,
		QuietDaysBitmask: quietDaysBitmask,
		MorningDigest:    morningDigest != 0,
	}
	return nil
}

// saveConfig saves notification configuration to database.
func (m *NotificationManager) saveConfig() error {
	if m.config == nil {
		return fmt.Errorf("no config to save")
	}

	morningDigest := 0
	if m.config.MorningDigest {
		morningDigest = 1
	}

	_, err := m.db.Exec(`
		INSERT INTO notifications_config (channel, quiet_from, quiet_to, quiet_days_bitmask, morning_digest)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(channel) DO UPDATE SET
			quiet_from = excluded.quiet_from,
			quiet_to = excluded.quiet_to,
			quiet_days_bitmask = excluded.quiet_days_bitmask,
			morning_digest = excluded.morning_digest
	`, m.config.Channel, m.config.QuietFrom, m.config.QuietTo,
		m.config.QuietDaysBitmask, morningDigest)
	return err
}

// SetConfig updates the notification configuration.
func (m *NotificationManager) SetConfig(cfg NotificationConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.config = &cfg
	return m.saveConfig()
}

// GetConfig returns the current notification configuration.
func (m *NotificationManager) GetConfig() *NotificationConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.config == nil {
		return &NotificationConfig{}
	}
	cfg := *m.config
	return &cfg
}

// isQuietHours checks if current time is within quiet hours.
func (m *NotificationManager) isQuietHours() bool {
	m.mu.RLock()
	cfg := m.config
	loc := m.location
	m.mu.RUnlock()

	if cfg == nil {
		return false
	}

	now := time.Now().In(loc)

	// Check day mask (0=Sunday, 1=Monday, ..., 6=Saturday)
	dayMask := uint8(1 << now.Weekday())
	if dayMask&cfg.QuietDaysBitmask == 0 {
		return false
	}

	// Parse quiet hours
	quietFrom, err := time.Parse("15:04", cfg.QuietFrom)
	if err != nil {
		return false
	}
	quietTo, err := time.Parse("15:04", cfg.QuietTo)
	if err != nil {
		return false
	}

	currentTime := time.Date(0, 1, 1, now.Hour(), now.Minute(), 0, 0, time.UTC)

	if quietFrom.Before(quietTo) {
		// Quiet hours don't cross midnight
		return (currentTime.Equal(quietFrom) || currentTime.After(quietFrom)) &&
		       currentTime.Before(quietTo)
	}
	// Quiet hours cross midnight
	return currentTime.Equal(quietFrom) || currentTime.After(quietFrom) ||
	       currentTime.Before(quietTo)
}

// isQuietHoursEnd checks if current time is at the quiet hours end time.
func (m *NotificationManager) isQuietHoursEnd() bool {
	m.mu.RLock()
	cfg := m.config
	loc := m.location
	m.mu.RUnlock()

	if cfg == nil || !cfg.MorningDigest {
		return false
	}

	now := time.Now().In(loc)
	quietTo, err := time.Parse("15:04", cfg.QuietTo)
	if err != nil {
		return false
	}

	// Check if we're within 1 minute of quiet hours end
	return now.Hour() == quietTo.Hour() && now.Minute() == quietTo.Minute()
}

// Notify sends a notification event.
func (m *NotificationManager) Notify(event Event) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Check priority and route accordingly
	switch event.Priority {
	case Urgent:
		// Urgent events bypass batching and quiet hours
		return m.sendImmediate(event, false, false)

	case High:
		// High priority bypasses batching but respects quiet hours
		if m.isQuietHours() {
			return m.queueForDigest(event)
		}
		return m.sendImmediate(event, false, false)

	case Medium:
		if m.isQuietHours() {
			return m.queueForDigest(event)
		}
		return m.queueForBatching(event)

	case Low:
		if m.isQuietHours() {
			return m.queueForDigest(event)
		}
		return m.queueForBatching(event)

	default:
		return m.sendImmediate(event, false, false)
	}
}

// sendImmediate sends a notification immediately.
func (m *NotificationManager) sendImmediate(event Event, wasBatched, wasQueued bool) error {
	// Record in history
	if err := m.recordHistory(event, wasBatched, wasQueued); err != nil {
		log.Printf("[WARN] Failed to record history: %v", err)
	}

	// Call send callback if registered
	if m.sendCallback != nil {
		m.sendCallback(event)
	}

	log.Printf("[INFO] Notification sent: type=%s priority=%s title=%s",
		event.Type, event.Priority, event.Title)
	return nil
}

// queueForBatching queues a notification for batched delivery.
func (m *NotificationManager) queueForBatching(event Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Add to appropriate batch queue
	switch event.Priority {
	case Low:
		m.pendingLow = append(m.pendingLow, &event)
	case Medium:
		m.pendingMedium = append(m.pendingMedium, &event)
	}

	// Check if we've hit max batch size
	totalPending := len(m.pendingLow) + len(m.pendingMedium)
	if totalPending >= m.maxBatchSize {
		// Force flush
		go m.flushBatch()
		return nil
	}

	// Start batch timer if not running
	if m.batchTimer == nil {
		m.batchTimer = time.AfterFunc(m.batchWindow, func() {
			m.flushBatch()
		})
	}

	log.Printf("[DEBUG] Queued for batching: type=%s priority=%s", event.Type, event.Priority)
	return nil
}

// flushBatch sends all pending batched notifications.
func (m *NotificationManager) flushBatch() {
	m.mu.Lock()
	lowPending := m.pendingLow
	mediumPending := m.pendingMedium
	m.pendingLow = make([]*Event, 0, m.maxBatchSize)
	m.pendingMedium = make([]*Event, 0, m.maxBatchSize)
	if m.batchTimer != nil {
		m.batchTimer.Stop()
		m.batchTimer = nil
	}
	m.mu.Unlock()

	// Send low priority batch
	if len(lowPending) > 0 {
		m.sendBatch(lowPending)
	}

	// Send medium priority batch
	if len(mediumPending) > 0 {
		m.sendBatch(mediumPending)
	}
}

// sendBatch sends a batch of notifications as a single summary.
func (m *NotificationManager) sendBatch(events []*Event) {
	if len(events) == 0 {
		return
	}

	// If only one event, send it directly
	if len(events) == 1 {
		m.sendImmediate(*events[0], true, false)
		return
	}

	// Create summary event
	summary := m.createSummary(events)
	m.sendImmediate(summary, true, false)
}

// createSummary creates a summary notification from multiple events.
func (m *NotificationManager) createSummary(events []*Event) Event {
	// Count by type
	typeCounts := make(map[EventType]int)
	for _, e := range events {
		typeCounts[e.Type]++
	}

	// Build title
	title := fmt.Sprintf("Activity Update: %d events", len(events))
	if len(events) > m.maxBatchSize {
		title = fmt.Sprintf("Activity Update: %d+ events", len(events))
	}

	// Build body
	body := fmt.Sprintf("%d notification(s): ", len(events))
	first := true
	for eventType, count := range typeCounts {
		if !first {
			body += ", "
		}
		body += fmt.Sprintf("%d %s", count, eventType)
		first = false
	}

	return Event{
		ID:        fmt.Sprintf("batch-%d", time.Now().UnixNano()),
		Type:      ZoneEnter, // Generic type for summaries
		Priority:  Medium,     // Summaries are medium priority
		Title:     title,
		Body:      body,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"event_count":  len(events),
			"type_counts":  typeCounts,
			"is_batch":     true,
		},
	}
}

// queueForDigest queues a notification for morning digest delivery.
func (m *NotificationManager) queueForDigest(event Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.queuedForDigest = append(m.queuedForDigest, &event)
	log.Printf("[DEBUG] Queued for digest: type=%s priority=%s", event.Type, event.Priority)
	return nil
}

// digestChecker periodically checks if it's time to send the morning digest.
func (m *NotificationManager) digestChecker() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		m.checkDigestTime()
	}
}

// checkDigestTime checks if it's time to send the morning digest.
func (m *NotificationManager) checkDigestTime() {
	m.mu.Lock()
	queued := m.queuedForDigest
	cfg := m.config
	loc := m.location
	todayDate := time.Now().In(loc).Format("2006-01-02")
	m.mu.Unlock()

	if cfg == nil || !cfg.MorningDigest || len(queued) == 0 {
		return
	}

	// Check if already sent today
	if m.digestSentDate == todayDate {
		return
	}

	// Check if we're at quiet hours end
	if !m.isQuietHoursEnd() {
		return
	}

	// Send digest
	go m.sendDigest()
}

// sendDigest sends the morning digest with all queued notifications.
func (m *NotificationManager) sendDigest() {
	m.mu.Lock()

	// Check if already sent today
	todayDate := time.Now().In(m.location).Format("2006-01-02")
	if m.digestSentDate == todayDate {
		m.mu.Unlock()
		return
	}

	// Check if there are queued events
	if len(m.queuedForDigest) == 0 {
		m.mu.Unlock()
		return
	}

	queued := m.queuedForDigest
	m.queuedForDigest = make([]*Event, 0)
	m.digestSentDate = todayDate
	m.mu.Unlock()

	if len(queued) == 0 {
		return
	}

	// Create digest notification
	title := fmt.Sprintf("Morning Digest: %d event(s) while you slept", len(queued))
	body := fmt.Sprintf("Overnight activity:\n")

	typeCounts := make(map[EventType]int)
	for _, e := range queued {
		typeCounts[e.Type]++
		if len(body) < 300 { // Limit body length
			body += fmt.Sprintf("• %s: %s\n", e.Type, e.Title)
		}
	}

	digest := Event{
		ID:        fmt.Sprintf("digest-%d", time.Now().UnixNano()),
		Type:      ZoneEnter,
		Priority:  Low,
		Title:     title,
		Body:      body,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"event_count":  len(queued),
			"type_counts":  typeCounts,
			"is_digest":    true,
		},
	}

	// Send immediately (bypasses quiet hours)
	m.sendImmediate(digest, false, true)

	log.Printf("[INFO] Morning digest sent with %d events", len(queued))
}

// recordHistory records a notification event in the database.
func (m *NotificationManager) recordHistory(event Event, wasBatched, wasQueued bool) error {
	wasBatchedInt := 0
	if wasBatched {
		wasBatchedInt = 1
	}
	wasQueuedInt := 0
	if wasQueued {
		wasQueuedInt = 1
	}

	_, err := m.db.Exec(`
		INSERT INTO notifications_history (event_type, priority, title, body, sent_at, was_batched, was_queued)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, string(event.Type), int(event.Priority), event.Title, event.Body,
		event.Timestamp.UnixMilli(), wasBatchedInt, wasQueuedInt)
	return err
}

// GetHistory returns recent notification history.
func (m *NotificationManager) GetHistory(limit int) ([]map[string]interface{}, error) {
	rows, err := m.db.Query(`
		SELECT event_type, priority, title, body, sent_at, was_batched, was_queued
		FROM notifications_history
		ORDER BY sent_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []map[string]interface{}
	for rows.Next() {
		var eventType, title, body string
		var priority int
		var sentAt int64
		var wasBatched, wasQueued int

		if err := rows.Scan(&eventType, &priority, &title, &body, &sentAt, &wasBatched, &wasQueued); err != nil {
			continue
		}

		history = append(history, map[string]interface{}{
			"event_type":  eventType,
			"priority":    Priority(priority).String(),
			"title":       title,
			"body":        body,
			"sent_at":     sentAt,
			"was_batched": wasBatched != 0,
			"was_queued":  wasQueued != 0,
		})
	}

	return history, nil
}

// SetSendCallback sets the callback function for sending notifications.
func (m *NotificationManager) SetSendCallback(fn func(Event)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendCallback = fn
}

// Close closes the notification manager and database connection.
func (m *NotificationManager) Close() error {
	return m.db.Close()
}

// GetPendingCount returns the number of pending notifications in each queue.
func (m *NotificationManager) GetPendingCount() (low, medium, digest int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.pendingLow), len(m.pendingMedium), len(m.queuedForDigest)
}

// Flush forces immediate flush of all pending notifications.
func (m *NotificationManager) Flush() {
	m.flushBatch()

	m.mu.Lock()
	queued := m.queuedForDigest
	m.queuedForDigest = make([]*Event, 0)
	m.mu.Unlock()

	if len(queued) > 0 {
		// Send as immediate digest
		for _, e := range queued {
			m.sendImmediate(*e, false, true)
		}
	}
}
