// Package notify provides enhanced push notification services with floor-plan thumbnails.
package notify

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// NotificationPriority represents the urgency level of a notification.
type NotificationPriority int

const (
	PriorityLow      NotificationPriority = 1
	PriorityMedium   NotificationPriority = 2
	PriorityHigh     NotificationPriority = 3
	PriorityUrgent   NotificationPriority = 4
	PriorityCritical NotificationPriority = 5
)

// NotificationType represents the type of notification event.
type NotificationType string

const (
	TypeZoneEnter      NotificationType = "zone_enter"
	TypeZoneLeave      NotificationType = "zone_leave"
	TypeZoneVacant     NotificationType = "zone_vacant"
	TypeFallDetected   NotificationType = "fall_detected"
	TypeFallEscalation NotificationType = "fall_escalation"
	TypeAnomalyAlert   NotificationType = "anomaly_alert"
	TypeNodeOffline    NotificationType = "node_offline"
	TypeSleepSummary   NotificationType = "sleep_summary"
)

// Notification represents a notification to send.
type Notification struct {
	Type     NotificationType  `json:"type"`
	Title    string            `json:"title"`
	Body     string            `json:"body"`
	Priority NotificationPriority `json:"priority,omitempty"`
	Tags     []string          `json:"tags,omitempty"`
	Image    []byte            `json:"-"` // PNG image data
	ImageType string           `json:"image_type,omitempty"` // "image/png"
	Data     map[string]interface{} `json:"data,omitempty"`
	Timestamp time.Time        `json:"timestamp"`

	// Event-specific fields
	PersonID   string `json:"person_id,omitempty"`
	PersonName string `json:"person_name,omitempty"`
	ZoneID     string `json:"zone_id,omitempty"`
	ZoneName   string `json:"zone_name,omitempty"`
	BlobID     int    `json:"blob_id,omitempty"`
	NodeID     string `json:"node_id,omitempty"`
	NodeLabel  string `json:"node_label,omitempty"`
}

// NotificationChannel represents a notification destination.
type NotificationChannel string

const (
	ChannelNtfy     NotificationChannel = "ntfy"
	ChannelPushover NotificationChannel = "pushover"
	ChannelGotify    NotificationChannel = "gotify"
	ChannelWebhook   NotificationChannel = "webhook"
)

// ChannelConfig holds configuration for a notification channel.
type ChannelConfig struct {
	Type     NotificationChannel
	Enabled  bool
	URL      string            // ntfy server, gotify server, webhook URL
	Token    string            // pushover token, gotify token
	User     string            // pushover user key
	Username string            // basic auth username
	Password string            // basic auth password
	Headers  map[string]string // custom headers for webhook

	// Event type enable/disable
	EnabledTypes map[NotificationType]bool // Types that are enabled for this channel
}

// QuietHoursConfig holds quiet hours configuration.
type QuietHoursConfig struct {
	Enabled        bool
	StartHour      int // 0-23
	StartMin       int
	EndHour        int
	EndMin         int
	QuietDaysMask  uint8 // Bitmask for days (0=Sunday, 1=Monday, etc.)
	MorningDigest  bool  // Deliver queued events as morning digest
	DigestHour     int    // Hour to deliver morning digest (default 7)
	DigestMin      int    // Minute to deliver morning digest
}

// BatchingConfig holds smart batching configuration.
type BatchingConfig struct {
	Enabled          bool
	BatchWindowSec   int  // Window duration in seconds (default 30)
	MaxBatchSize     int  // Maximum events before forcing send (default 5)
	BatchLowPriority bool // Batch LOW priority events
	BatchMedium      bool // Batch MEDIUM priority events
}

// Service manages notification delivery with batching and quiet hours.
type Service struct {
	mu           sync.RWMutex
	db           *sql.DB
	channels     map[string]*ChannelConfig
	quietHours   *QuietHoursConfig
	batching     *BatchingConfig
	httpClient   *http.Client
	roomConfig   RoomConfigProvider

	// Batching state
	pendingLow      []Notification
	pendingMedium   []Notification
	batchTimer      *time.Timer
	lastBatchFlush  time.Time

	// Quiet hours queue
	queuedDuringQuiet []Notification

	// Morning digest state
	digestSentToday bool
	digestLastDate   string // YYYY-MM-DD

	// Callbacks
	onSend func(channel string, notif Notification, success bool)
}

// RoomConfigProvider provides room dimensions for floor plan rendering.
type RoomConfigProvider interface {
	GetRoom() (width, height, depth float64)
}

// NewService creates a new notification service.
func NewService(dbPath string) (*Service, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	s := &Service{
		db:          db,
		channels:    make(map[string]*ChannelConfig),
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		batching: &BatchingConfig{
			Enabled:          true,
			BatchWindowSec:   30,
			MaxBatchSize:     5,
			BatchLowPriority: true,
			BatchMedium:      true,
		},
	}

	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	if err := s.loadChannels(); err != nil {
		log.Printf("[WARN] Failed to load notification channels: %v", err)
	}

	if err := s.loadQuietHours(); err != nil {
		log.Printf("[WARN] Failed to load quiet hours: %v", err)
	}

	if err := s.loadBatchingConfig(); err != nil {
		log.Printf("[WARN] Failed to load batching config: %v", err)
	}

	// Start morning digest scheduler
	go s.morningDigestScheduler()

	return s, nil
}

func (s *Service) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS notification_channels (
			id       TEXT PRIMARY KEY,
			type     TEXT    NOT NULL,
			enabled  INTEGER NOT NULL DEFAULT 1,
			url      TEXT    NOT NULL DEFAULT '',
			token    TEXT    NOT NULL DEFAULT '',
			user_key TEXT    NOT NULL DEFAULT '',
			username TEXT    NOT NULL DEFAULT '',
			password TEXT    NOT NULL DEFAULT '',
			headers  TEXT    NOT NULL DEFAULT '{}',
			enabled_types TEXT NOT NULL DEFAULT '{}'
		);

		CREATE TABLE IF NOT EXISTS notification_history (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			channel     TEXT    NOT NULL,
			type        TEXT    NOT NULL,
			title       TEXT    NOT NULL,
			body        TEXT    NOT NULL,
			success     INTEGER NOT NULL DEFAULT 0,
			error_msg   TEXT    NOT NULL DEFAULT '',
			timestamp   INTEGER NOT NULL DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS quiet_hours (
			id       TEXT PRIMARY KEY DEFAULT 'default',
			enabled  INTEGER NOT NULL DEFAULT 0,
			start_h  INTEGER NOT NULL DEFAULT 22,
			start_m  INTEGER NOT NULL DEFAULT 0,
			end_h    INTEGER NOT NULL DEFAULT 7,
			end_m    INTEGER NOT NULL DEFAULT 0,
			days_mask INTEGER NOT NULL DEFAULT 0x7F,
			morning_digest INTEGER NOT NULL DEFAULT 1,
			digest_h  INTEGER NOT NULL DEFAULT 7,
			digest_m  INTEGER NOT NULL DEFAULT 0
		);

		INSERT OR IGNORE INTO quiet_hours (id, enabled, start_h, start_m, end_h, end_m, days_mask, morning_digest, digest_h, digest_m)
		VALUES ('default', 0, 22, 0, 7, 0, 0x7F, 1, 7, 0);

		CREATE TABLE IF NOT EXISTS batching_config (
			id              TEXT PRIMARY KEY DEFAULT 'default',
			enabled         INTEGER NOT NULL DEFAULT 1,
			batch_window_sec INTEGER NOT NULL DEFAULT 30,
			max_batch_size  INTEGER NOT NULL DEFAULT 5,
			batch_low       INTEGER NOT NULL DEFAULT 1,
			batch_medium    INTEGER NOT NULL DEFAULT 1
		);

		INSERT OR IGNORE INTO batching_config (id, enabled, batch_window_sec, max_batch_size, batch_low, batch_medium)
		VALUES ('default', 1, 30, 5, 1, 1);
	`)
	return err
}

func (s *Service) loadChannels() error {
	rows, err := s.db.Query(`
		SELECT id, type, enabled, url, token, user_key, username, password, headers, enabled_types
		FROM notification_channels WHERE enabled = 1
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var cc ChannelConfig
		var id string
		var enabled int
		var headersJSON, typesJSON string

		if err := rows.Scan(&id, &cc.Type, &enabled, &cc.URL, &cc.Token, &cc.User, &cc.Username, &cc.Password, &headersJSON, &typesJSON); err != nil {
			continue
		}

		cc.Enabled = enabled != 0
		cc.EnabledTypes = make(map[NotificationType]bool)
		if headersJSON != "" && headersJSON != "{}" {
			json.Unmarshal([]byte(headersJSON), &cc.Headers)
		}
		if typesJSON != "" && typesJSON != "{}" {
			json.Unmarshal([]byte(typesJSON), &cc.EnabledTypes)
		}
		s.channels[id] = &cc
	}
	return nil
}

func (s *Service) loadQuietHours() error {
	row := s.db.QueryRow(`SELECT enabled, start_h, start_m, end_h, end_m, days_mask, morning_digest, digest_h, digest_m FROM quiet_hours WHERE id = 'default'`)
	var enabled, daysMask, morningDigest int
	qh := &QuietHoursConfig{}
	if err := row.Scan(&enabled, &qh.StartHour, &qh.StartMin, &qh.EndHour, &qh.EndMin, &daysMask, &morningDigest, &qh.DigestHour, &qh.DigestMin); err != nil {
		return err
	}
	qh.Enabled = enabled != 0
	qh.QuietDaysMask = uint8(daysMask)
	qh.MorningDigest = morningDigest != 0
	s.quietHours = qh
	return nil
}

func (s *Service) loadBatchingConfig() error {
	row := s.db.QueryRow(`SELECT enabled, batch_window_sec, max_batch_size, batch_low, batch_medium FROM batching_config WHERE id = 'default'`)
	var enabled, batchLow, batchMedium int
	bc := &BatchingConfig{}
	if err := row.Scan(&enabled, &bc.BatchWindowSec, &bc.MaxBatchSize, &batchLow, &batchMedium); err != nil {
		return err
	}
	bc.Enabled = enabled != 0
	bc.BatchLowPriority = batchLow != 0
	bc.BatchMedium = batchMedium != 0
	s.batching = bc
	return nil
}

// Close closes the database.
func (s *Service) Close() error {
	return s.db.Close()
}

// SetRoomConfig sets the room config provider for floor plan generation.
func (s *Service) SetRoomConfig(provider RoomConfigProvider) {
	s.mu.Lock()
	s.roomConfig = provider
	s.mu.Unlock()
}

// SetOnSend sets callback for notification send events.
func (s *Service) SetOnSend(cb func(channel string, notif Notification, success bool)) {
	s.mu.Lock()
	s.onSend = cb
	s.mu.Unlock()
}

// AddChannel adds or updates a notification channel.
func (s *Service) AddChannel(id string, cc ChannelConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cc.EnabledTypes == nil {
		// Enable all types by default
		cc.EnabledTypes = map[NotificationType]bool{
			TypeZoneEnter:      true,
			TypeZoneLeave:      true,
			TypeZoneVacant:     true,
			TypeFallDetected:   true,
			TypeFallEscalation: true,
			TypeAnomalyAlert:   true,
			TypeNodeOffline:    true,
			TypeSleepSummary:   true,
		}
	}

	headersJSON, _ := json.Marshal(cc.Headers)
	typesJSON, _ := json.Marshal(cc.EnabledTypes)

	_, err := s.db.Exec(`
		INSERT INTO notification_channels (id, type, enabled, url, token, user_key, username, password, headers, enabled_types)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			type = excluded.type,
			enabled = excluded.enabled,
			url = excluded.url,
			token = excluded.token,
			user_key = excluded.user_key,
			username = excluded.username,
			password = excluded.password,
			headers = excluded.headers,
			enabled_types = excluded.enabled_types
	`, id, cc.Type, cc.Enabled, cc.URL, cc.Token, cc.User, cc.Username, cc.Password, string(headersJSON), string(typesJSON))
	if err != nil {
		return err
	}

	if cc.Enabled {
		s.channels[id] = &cc
	} else {
		delete(s.channels, id)
	}
	return nil
}

// RemoveChannel removes a notification channel.
func (s *Service) RemoveChannel(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`DELETE FROM notification_channels WHERE id = ?`, id)
	if err != nil {
		return err
	}

	delete(s.channels, id)
	return nil
}

// SetQuietHours updates the quiet hours configuration.
func (s *Service) SetQuietHours(qh QuietHoursConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	daysMask := int(qh.QuietDaysMask)
	morningDigest := 0
	if qh.MorningDigest {
		morningDigest = 1
	}

	_, err := s.db.Exec(`
		UPDATE quiet_hours SET enabled = ?, start_h = ?, start_m = ?, end_h = ?, end_m = ?, days_mask = ?, morning_digest = ?, digest_h = ?, digest_m = ?
		WHERE id = 'default'
	`, qh.Enabled, qh.StartHour, qh.StartMin, qh.EndHour, qh.EndMin, daysMask, morningDigest, qh.DigestHour, qh.DigestMin)
	if err != nil {
		return err
	}

	s.quietHours = &qh
	return nil
}

// SetBatchingConfig updates the batching configuration.
func (s *Service) SetBatchingConfig(bc BatchingConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	batchLow := 0
	if bc.BatchLowPriority {
		batchLow = 1
	}
	batchMedium := 0
	if bc.BatchMedium {
		batchMedium = 1
	}

	_, err := s.db.Exec(`
		UPDATE batching_config SET enabled = ?, batch_window_sec = ?, max_batch_size = ?, batch_low = ?, batch_medium = ?
		WHERE id = 'default'
	`, bc.Enabled, bc.BatchWindowSec, bc.MaxBatchSize, batchLow, batchMedium)
	if err != nil {
		return err
	}

	s.batching = &bc
	return nil
}

// isQuietHours checks if current time is within quiet hours.
func (s *Service) isQuietHours() bool {
	s.mu.RLock()
	qh := s.quietHours
	s.mu.RUnlock()

	if qh == nil || !qh.Enabled {
		return false
	}

	now := time.Now()
	currentMins := now.Hour()*60 + now.Minute()
	startMins := qh.StartHour*60 + qh.StartMin
	endMins := qh.EndHour*60 + qh.EndMin

	// Check day mask (0=Sunday, 1=Monday, etc.)
	dayMask := uint8(1 << now.Weekday())
	if dayMask&qh.QuietDaysMask == 0 {
		return false
	}

	if startMins < endMins {
		// Quiet hours don't cross midnight
		return currentMins >= startMins && currentMins < endMins
	}
	// Quiet hours cross midnight
	return currentMins >= startMins || currentMins < endMins
}

// isTypeEnabled checks if a notification type is enabled for all channels.
func (s *Service) isTypeEnabled(nType NotificationType) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, cc := range s.channels {
		if cc.Enabled {
			if enabled, ok := cc.EnabledTypes[nType]; !ok || !enabled {
				return false
			}
		}
	}
	return true
}

// Send sends a notification immediately (urgents) or queues for batching (low/medium).
func (s *Service) Send(notif Notification) error {
	notif.Timestamp = time.Now()

	// Check if notification type is enabled
	if !s.isTypeEnabled(notif.Type) {
		log.Printf("[DEBUG] Notification type %s disabled, skipping", notif.Type)
		return nil
	}

	// Check priority and handle accordingly
	switch notif.Priority {
	case PriorityUrgent, PriorityCritical:
		// Send immediately, bypassing batching and quiet hours
		return s.sendImmediate(notif)
	case PriorityHigh:
		// Send immediately unless in quiet hours
		if s.isQuietHours() {
			return s.queueForQuietHours(notif)
		}
		return s.sendImmediate(notif)
	case PriorityMedium:
		if s.isQuietHours() {
			return s.queueForQuietHours(notif)
		}
		if s.batching != nil && s.batching.Enabled && s.batching.BatchMedium {
			return s.queueForBatching(notif)
		}
		return s.sendImmediate(notif)
	case PriorityLow:
		if s.isQuietHours() {
			return s.queueForQuietHours(notif)
		}
		if s.batching != nil && s.batching.Enabled && s.batching.BatchLowPriority {
			return s.queueForBatching(notif)
		}
		return s.sendImmediate(notif)
	default:
		return s.sendImmediate(notif)
	}
}

// sendImmediate sends a notification immediately to all enabled channels.
func (s *Service) sendImmediate(notif Notification) error {
	s.mu.RLock()
	channels := make([]*ChannelConfig, 0, len(s.channels))
	for _, cc := range s.channels {
		channels = append(channels, cc)
	}
	s.mu.RUnlock()

	if len(channels) == 0 {
		return nil
	}

	var lastErr error
	for _, cc := range channels {
		if !cc.Enabled {
			continue
		}

		var err error
		switch cc.Type {
		case ChannelNtfy:
			err = s.sendNtfy(cc, notif)
		case ChannelPushover:
			err = s.sendPushover(cc, notif)
		case ChannelGotify:
			err = s.sendGotify(cc, notif)
		case ChannelWebhook:
			err = s.sendWebhook(cc, notif)
		default:
			log.Printf("[WARN] Unknown notification channel type: %s", cc.Type)
			continue
		}

		success := err == nil
		if s.onSend != nil {
			s.onSend(string(cc.Type), notif, success)
		}

		s.recordHistory(string(cc.Type), notif, success, err)

		if err != nil {
			log.Printf("[ERROR] Failed to send notification via %s: %v", cc.Type, err)
			lastErr = err
		} else {
			log.Printf("[INFO] Notification sent via %s: %s", cc.Type, notif.Title)
		}
	}

	return lastErr
}

// queueForBatching queues a notification for batched delivery.
func (s *Service) queueForBatching(notif Notification) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch notif.Priority {
	case PriorityLow:
		s.pendingLow = append(s.pendingLow, notif)
	case PriorityMedium:
		s.pendingMedium = append(s.pendingMedium, notif)
	}

	// Check if we've hit max batch size
	totalPending := len(s.pendingLow) + len(s.pendingMedium)
	maxBatchSize := s.batching.MaxBatchSize
	if maxBatchSize <= 0 {
		maxBatchSize = 5
	}

	if totalPending >= maxBatchSize {
		// Force flush
		go s.flushBatch()
		return nil
	}

	// Start batch timer if not running
	if s.batchTimer == nil {
		s.batchTimer = time.AfterFunc(time.Duration(s.batching.BatchWindowSec)*time.Second, s.flushBatch)
	}

	return nil
}

// flushBatch sends all pending batched notifications.
func (s *Service) flushBatch() {
	s.mu.Lock()
	lowPending := s.pendingLow
	mediumPending := s.pendingMedium
	s.pendingLow = nil
	s.pendingMedium = nil
	s.batchTimer = nil
	s.lastBatchFlush = time.Now()
	s.mu.Unlock()

	// Merge low priority notifications
	if len(lowPending) > 0 {
		merged := s.mergeNotifications(lowPending)
		s.sendImmediate(merged)
	}

	// Merge medium priority notifications
	if len(mediumPending) > 0 {
		merged := s.mergeNotifications(mediumPending)
		s.sendImmediate(merged)
	}
}

// mergeNotifications merges multiple notifications into one.
func (s *Service) mergeNotifications(notifs []Notification) Notification {
	if len(notifs) == 0 {
		return Notification{}
	}
	if len(notifs) == 1 {
		return notifs[0]
	}

	// Build summary
	var bodies []string
	personNames := make(map[string]bool)
	zoneNames := make(map[string]bool)

	for _, n := range notifs {
		bodies = append(bodies, n.Body)
		if n.PersonName != "" {
			personNames[n.PersonName] = true
		}
		if n.ZoneName != "" {
			zoneNames[n.ZoneName] = true
		}
	}

	// Generate summary title
	title := "Activity Update"
	if len(notifs) > 1 {
		title = fmt.Sprintf("%d presence events", len(notifs))
	}

	// Generate body
	body := strings.Join(bodies, ". ")
	if len(body) > 200 {
		body = body[:197] + "..."
	}

	// Determine priority (use highest)
	priority := notifs[0].Priority
	for _, n := range notifs {
		if n.Priority > priority {
			priority = n.Priority
		}
	}

	// Collect tags
	var tags []string
	tagSet := make(map[string]bool)
	for _, n := range notifs {
		for _, t := range n.Tags {
			tagSet[t] = true
		}
	}
	for t := range tagSet {
		tags = append(tags, t)
	}

	return Notification{
		Type:     notifs[0].Type,
		Title:    title,
		Body:     body,
		Priority: priority,
		Tags:     tags,
		Timestamp: time.Now(),
	}
}

// queueForQuietHours queues a notification for morning digest delivery.
func (s *Service) queueForQuietHours(notif Notification) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.queuedDuringQuiet = append(s.queuedDuringQuiet, notif)
	log.Printf("[DEBUG] Queued during quiet hours: %s", notif.Title)

	return nil
}

// morningDigestScheduler runs periodically to check if it's time to send the morning digest.
func (s *Service) morningDigestScheduler() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.checkMorningDigest()
	}
}

// checkMorningDigest checks if it's time to send the morning digest.
func (s *Service) checkMorningDigest() {
	s.mu.Lock()
	qh := s.quietHours
	queued := s.queuedDuringQuiet
	s.mu.Unlock()

	if qh == nil || !qh.MorningDigest || len(queued) == 0 {
		return
	}

	now := time.Now()
	digestTime := time.Date(now.Year(), now.Month(), now.Day(), qh.DigestHour, qh.DigestMin, 0, 0, now.Location())

	// Check if we're at the digest time (within 1 minute window)
	if now.Before(digestTime) || now.After(digestTime.Add(1*time.Minute)) {
		return
	}

	// Check if we already sent today
	todayDate := now.Format("2006-01-02")
	s.mu.Lock()
	if s.digestSentToday && s.digestLastDate == todayDate {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	// Send morning digest
	go s.sendMorningDigest()
}

// sendMorningDigest sends the queued notifications as a morning digest.
func (s *Service) sendMorningDigest() {
	s.mu.Lock()
	queued := s.queuedDuringQuiet
	s.queuedDuringQuiet = nil
	s.digestSentToday = true
	s.digestLastDate = time.Now().Format("2006-01-02")
	s.mu.Unlock()

	if len(queued) == 0 {
		return
	}

	// Create digest notification
	var bodies []string
	personCount := make(map[string]int)
	zoneCount := make(map[string]int)

	for _, n := range queued {
		bodies = append(bodies, n.Body)
		if n.PersonName != "" {
			personCount[n.PersonName]++
		}
		if n.ZoneName != "" {
			zoneCount[n.ZoneName]++
		}
	}

	title := fmt.Sprintf("Morning digest - %d events while you slept", len(queued))
	body := fmt.Sprintf("While you were asleep: %s", strings.Join(bodies, ". "))

	// Add summary
	if len(personCount) > 0 {
		var people []string
		for p := range personCount {
			people = append(people, p)
		}
		body += fmt.Sprintf("\n\nPeople detected: %s", strings.Join(people, ", "))
	}

	digestNotif := Notification{
		Type:     TypeZoneEnter, // Use generic type
		Title:    title,
		Body:     body,
		Priority: PriorityLow,
		Tags:     []string{"digest", "morning"},
		Data: map[string]interface{}{
			"event_count": len(queued),
			"people":      personCount,
			"zones":       zoneCount,
		},
		Timestamp: time.Now(),
	}

	// Send digest immediately (bypasses quiet hours)
	s.sendImmediate(digestNotif)

	log.Printf("[INFO] Morning digest sent with %d events", len(queued))
}

// recordHistory records notification send attempt in history.
func (s *Service) recordHistory(channel string, notif Notification, success bool, err error) {
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}

	_, dbErr := s.db.Exec(`
		INSERT INTO notification_history (channel, type, title, body, success, error_msg, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, channel, string(notif.Type), notif.Title, notif.Body, success, errMsg, time.Now().UnixNano())
	if dbErr != nil {
		log.Printf("[WARN] Failed to record notification history: %v", dbErr)
	}
}

// GetHistory returns recent notification history.
func (s *Service) GetHistory(limit int) []struct {
	Channel   string
	Type      string
	Title     string
	Body      string
	Success   bool
	Error     string
	Timestamp time.Time
} {
	rows, err := s.db.Query(`
		SELECT channel, type, title, body, success, error_msg, timestamp
		FROM notification_history
		ORDER BY timestamp DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var history []struct {
		Channel   string
		Type      string
		Title     string
		Body      string
		Success   bool
		Error     string
		Timestamp time.Time
	}

	for rows.Next() {
		var h struct {
			Channel   string
			Type      string
			Title     string
			Body      string
			Success   bool
			Error     string
			Timestamp time.Time
		}
		var success int
		var ts int64
		if err := rows.Scan(&h.Channel, &h.Type, &h.Title, &h.Body, &success, &h.Error, &ts); err != nil {
			continue
		}
		h.Success = success != 0
		h.Timestamp = time.Unix(0, ts)
		history = append(history, h)
	}

	return history
}

// GetChannels returns all configured channels.
func (s *Service) GetChannels() map[string]*ChannelConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*ChannelConfig, len(s.channels))
	for k, v := range s.channels {
		result[k] = v
	}
	return result
}

// GetQuietHours returns the current quiet hours configuration.
func (s *Service) GetQuietHours() *QuietHoursConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.quietHours == nil {
		return &QuietHoursConfig{}
	}
	// Return a copy
	qh := *s.quietHours
	return &qh
}

// GetBatchingConfig returns the current batching configuration.
func (s *Service) GetBatchingConfig() *BatchingConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.batching == nil {
		return &BatchingConfig{}
	}
	// Return a copy
	bc := *s.batching
	return &bc
}

// sendNtfy sends notification via ntfy.sh.
func (s *Service) sendNtfy(cc *ChannelConfig, notif Notification) error {
	if cc.URL == "" {
		return fmt.Errorf("ntfy URL not configured")
	}

	req, err := http.NewRequest("POST", cc.URL, bytes.NewBufferString(notif.Body))
	if err != nil {
		return err
	}

	req.Header.Set("Title", notif.Title)
	if len(notif.Tags) > 0 {
		tags := ""
		for _, t := range notif.Tags {
			if tags != "" {
				tags += ","
			}
			tags += t
		}
		req.Header.Set("Tags", tags)
	}

	// Set priority based on notification priority
	priority := "default"
	switch notif.Priority {
	case PriorityUrgent, PriorityCritical:
		priority = "urgent"
	case PriorityHigh:
		priority = "high"
	case PriorityLow:
		priority = "low"
	}
	req.Header.Set("Priority", priority)

	// Attach image if available
	if len(notif.Image) > 0 {
		// For ntfy, attach as base64 data URL
		dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(notif.Image)
		req.Header.Set("X-Attach", dataURL)
	}

	if cc.Username != "" || cc.Password != "" {
		req.SetBasicAuth(cc.Username, cc.Password)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("ntfy returned status %d", resp.StatusCode)
	}
	return nil
}

// sendPushover sends notification via Pushover API.
func (s *Service) sendPushover(cc *ChannelConfig, notif Notification) error {
	if cc.Token == "" || cc.User == "" {
		return fmt.Errorf("pushover token/user not configured")
	}

	data := url.Values{}
	data.Set("token", cc.Token)
	data.Set("user", cc.User)
	data.Set("title", notif.Title)
	data.Set("message", notif.Body)

	// Map priority
	priority := "0"
	switch notif.Priority {
	case PriorityUrgent:
		priority = "2" // Emergency
	case PriorityHigh:
		priority = "1" // High
	case PriorityLow:
		priority = "-1" // Low
	}
	data.Set("priority", priority)

	// Attach image if available
	if len(notif.Image) > 0 {
		// Pushover requires multipart form with attachment
		// For simplicity, we'll send without attachment in this implementation
		// A full implementation would need multipart/form-data encoding
	}

	resp, err := s.httpClient.PostForm("https://api.pushover.net/1/messages.json", data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("pushover returned status %d", resp.StatusCode)
	}
	return nil
}

// sendGotify sends notification via Gotify server.
func (s *Service) sendGotify(cc *ChannelConfig, notif Notification) error {
	if cc.URL == "" || cc.Token == "" {
		return fmt.Errorf("gotify URL/token not configured")
	}

	payload := map[string]interface{}{
		"title":    notif.Title,
		"message":  notif.Body,
		"priority": notif.Priority,
	}

	body, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/message?token=%s", strings.TrimSuffix(cc.URL, "/"), cc.Token)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("gotify returned status %d", resp.StatusCode)
	}
	return nil
}

// sendWebhook sends notification via custom webhook.
func (s *Service) sendWebhook(cc *ChannelConfig, notif Notification) error {
	if cc.URL == "" {
		return fmt.Errorf("webhook URL not configured")
	}

	payload := map[string]interface{}{
		"type":     string(notif.Type),
		"title":    notif.Title,
		"body":     notif.Body,
		"priority": int(notif.Priority),
		"tags":     notif.Tags,
		"timestamp": notif.Timestamp.Format(time.RFC3339),
		"data":     notif.Data,
	}

	// Add image as base64 if available
	if len(notif.Image) > 0 {
		payload["image_png_base64"] = base64.StdEncoding.EncodeToString(notif.Image)
	}

	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", cc.URL, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	for k, v := range cc.Headers {
		req.Header.Set(k, v)
	}

	if cc.Username != "" || cc.Password != "" {
		req.SetBasicAuth(cc.Username, cc.Password)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}

// GenerateFloorPlanThumbnail generates a mini floor plan PNG with blob positions.
// This is a simple implementation that draws zones and people.
func (s *Service) GenerateFloorPlanThumbnail(width, height int, zones []struct {
	ID, Name, Color string
	X, Y, W, D      float64
	Highlight      bool
}, people []struct {
	Name, Color string
	X, Y, Z     float64
	Confidence float64
	IsFall     bool
}) ([]byte, error) {
	// Create image
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill background
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{26, 26, 46, 255}}, image.Point{}, draw.Src)

	// Get room dimensions
	var roomW, roomD float64 = 6.0, 5.0
	if s.roomConfig != nil {
		roomW, _, roomD = s.roomConfig.GetRoom()
	}

	// Scale factor
	margin := 10
	drawW := float64(width - 2*margin)
	drawH := float64(height - 2*margin)
	scaleX := drawW / roomW
	scaleZ := drawH / roomD
	scale := scaleX
	if scaleZ < scale {
		scale = scaleZ
	}

	offsetX := float64(margin) + (drawW-roomW*scale)/2
	offsetY := float64(margin) + (drawH-roomD*scale)/2

	// Draw zones
	for _, zone := range zones {
		x := int(offsetX + zone.X*scale)
		y := int(offsetY + zone.Y*scale)
		w := int(zone.W * scale)
		h := int(zone.D * scale)

		zoneColor := color.RGBA{79, 195, 247, 51} // Default blue with 20% opacity
		if zone.Highlight {
			zoneColor = color.RGBA{79, 195, 247, 150} // Brighter for highlight
		}

		// Draw zone fill
		draw.Draw(img, image.Rect(x, y, x+w, y+h), &image.Uniform{zoneColor}, image.Point{}, draw.Over)

		// Draw outline
		outlineColor := color.RGBA{255, 255, 255, 100}
		if zone.Highlight {
			outlineColor = color.RGBA{255, 255, 255, 255}
		}
		// Draw outline (simplified - just corners)
		img.Set(x, y, outlineColor)
		img.Set(x+w-1, y, outlineColor)
		img.Set(x, y+h-1, outlineColor)
		img.Set(x+w-1, y+h-1, outlineColor)
	}

	// Draw people
	for _, person := range people {
		px := int(offsetX + person.X*scale)
		py := int(offsetY + person.Y*scale)

		// Clamp to image bounds
		if px < 5 || px >= width-5 || py < 5 || py >= height-5 {
			continue
		}

		// Color based on state
		clr := color.RGBA{100, 181, 246, 255} // Blue - normal
		if person.IsFall {
			clr = color.RGBA{239, 83, 80, 255} // Red - fall detected
		} else if person.Color != "" {
			// Parse hex color
			fmt.Sscanf(person.Color, "#%02x%02x%02x", &clr.R, &clr.G, &clr.B)
		}

		// Draw circle
		diameter := 10
		for dy := -diameter / 2; dy <= diameter/2; dy++ {
			for dx := -diameter / 2; dx <= diameter/2; dx++ {
				if dx*dx+dy*dy <= (diameter/2)*(diameter/2) {
					px2 := px + dx
					py2 := py + dy
					if px2 >= 0 && px2 < width && py2 >= 0 && py2 < height {
						img.Set(px2, py2, clr)
					}
				}
			}
		}
	}

	// Encode to PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
