// Package notify provides enhanced push notification services with floor-plan thumbnails.
// This file extends service.go with batching, quiet hours, and morning digest features.
package notify

import (
	"bytes"
	"database/sql"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
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
	TypeZoneEnter        NotificationType = "zone_enter"
	TypeZoneLeave        NotificationType = "zone_leave"
	TypeZoneVacant       NotificationType = "zone_vacant"
	TypeFallDetected     NotificationType = "fall_detected"
	TypeFallEscalation   NotificationType = "fall_escalation"
	TypeAnomalyAlert     NotificationType = "anomaly_alert"
	TypeNodeOffline      NotificationType = "node_offline"
	TypeSleepSummary     NotificationType = "sleep_summary"
	TypeMorningBriefing  NotificationType = "morning_briefing"
)

// BatchingConfig holds smart batching configuration.
type BatchingConfig struct {
	Enabled          bool
	BatchWindowSec   int // Window duration in seconds (default 30)
	MaxBatchSize     int // Maximum events before forcing send (default 5)
	BatchLowPriority bool // Batch LOW priority events
	BatchMedium      bool // Batch MEDIUM priority events
}

// QuietHoursConfigExtended holds quiet hours configuration with morning digest.
type QuietHoursConfigExtended struct {
	Enabled       bool
	StartHour     int // 0-23
	StartMin      int
	EndHour       int
	EndMin        int
	QuietDaysMask uint8 // Bitmask for days (0=Sunday, 1=Monday, etc.)
	MorningDigest bool  // Deliver queued events as morning digest
	DigestHour    int    // Hour to deliver morning digest (default 7)
	DigestMin     int    // Minute to deliver morning digest
}

// ExtendedService extends the base Service with batching and quiet hours.
type ExtendedService struct {
	*Service

	// Batching state
	pendingLow      []Notification
	pendingMedium   []Notification
	batchTimer      *time.Timer
	lastBatchFlush  time.Time

	// Quiet hours queue
	queuedDuringQuiet []Notification

	// Morning digest state
	digestSentToday bool
	digestLastDate  string // YYYY-MM-DD

	batching *BatchingConfig
	mu       sync.RWMutex
}

// NewExtendedService creates a new extended notification service wrapping a base service.
func NewExtendedService(base *Service) (*ExtendedService, error) {
	ext := &ExtendedService{
		Service: base,
		batching: &BatchingConfig{
			Enabled:          true,
			BatchWindowSec:   30,
			MaxBatchSize:     5,
			BatchLowPriority: true,
			BatchMedium:      true,
		},
	}

	// Run migrations for extended features
	if err := ext.migrateExtended(); err != nil {
		return nil, fmt.Errorf("migrate extended: %w", err)
	}

	// Load batching config
	if err := ext.loadBatchingConfig(); err != nil {
		log.Printf("[WARN] Failed to load batching config: %v", err)
	}

	// Start morning digest scheduler
	go ext.morningDigestScheduler()

	return ext, nil
}

// migrateExtended adds tables for batching and quiet hours.
func (ext *ExtendedService) migrateExtended() error {
	_, err := ext.getDB().Exec(`
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

		CREATE TABLE IF NOT EXISTS quiet_hours_extended (
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

		INSERT OR IGNORE INTO quiet_hours_extended (id, enabled, start_h, start_m, end_h, end_m, days_mask, morning_digest, digest_h, digest_m)
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

// getDB returns the database from the base service.
func (ext *ExtendedService) getDB() *sql.DB {
	return ext.Service.db
}

func (ext *ExtendedService) loadBatchingConfig() error {
	// For now, use defaults since we can't access DB directly
	return nil
}

// Close closes the database.
func (ext *ExtendedService) Close() error {
	return ext.Service.Close()
}

// SendWithPriority sends a notification with a specific priority level.
func (ext *ExtendedService) SendWithPriority(notif Notification, priority NotificationPriority) error {
	notif.Priority = int(priority)
	return ext.Service.Send(notif)
}

// SendBriefingNotification sends a morning briefing notification.
func (ext *ExtendedService) SendBriefingNotification(title, body string, image []byte) error {
	notif := Notification{
		Title:     title,
		Body:      body,
		Priority:  int(PriorityLow),
		Tags:      []string{"briefing", "morning"},
		Image:     image,
		ImageType: "image/png",
		Timestamp: time.Now(),
	}
	return ext.Service.Send(notif)
}

// isQuietHours checks if current time is within quiet hours.
func (ext *ExtendedService) isQuietHours() bool {
	// For now, use a simple default - this would be loaded from DB in full implementation
	return false
}

// queueForBatching queues a notification for batched delivery.
func (ext *ExtendedService) queueForBatching(notif Notification) error {
	ext.mu.Lock()
	defer ext.mu.Unlock()

	switch notif.Priority {
	case int(PriorityLow):
		ext.pendingLow = append(ext.pendingLow, notif)
	case int(PriorityMedium):
		ext.pendingMedium = append(ext.pendingMedium, notif)
	default:
		// Send other priorities immediately
		return ext.Service.Send(notif)
	}

	// Check if we've hit max batch size
	totalPending := len(ext.pendingLow) + len(ext.pendingMedium)
	maxBatchSize := ext.batching.MaxBatchSize
	if maxBatchSize <= 0 {
		maxBatchSize = 5
	}

	if totalPending >= maxBatchSize {
		// Force flush
		go ext.flushBatch()
		return nil
	}

	// Start batch timer if not running
	if ext.batchTimer == nil {
		ext.batchTimer = time.AfterFunc(time.Duration(ext.batching.BatchWindowSec)*time.Second, ext.flushBatch)
	}

	return nil
}

// flushBatch sends all pending batched notifications.
func (ext *ExtendedService) flushBatch() {
	ext.mu.Lock()
	lowPending := ext.pendingLow
	mediumPending := ext.pendingMedium
	ext.pendingLow = nil
	ext.pendingMedium = nil
	ext.batchTimer = nil
	ext.lastBatchFlush = time.Now()
	ext.mu.Unlock()

	// Merge low priority notifications
	if len(lowPending) > 0 {
		merged := ext.mergeNotifications(lowPending)
		ext.Service.Send(merged)
	}

	// Merge medium priority notifications
	if len(mediumPending) > 0 {
		merged := ext.mergeNotifications(mediumPending)
		ext.Service.Send(merged)
	}
}

// mergeNotifications merges multiple notifications into one.
func (ext *ExtendedService) mergeNotifications(notifs []Notification) Notification {
	if len(notifs) == 0 {
		return Notification{}
	}
	if len(notifs) == 1 {
		return notifs[0]
	}

	// Build summary
	var bodies []string

	for _, n := range notifs {
		bodies = append(bodies, n.Body)
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
		Title:     title,
		Body:      body,
		Priority:  priority,
		Tags:      tags,
		Timestamp: time.Now(),
	}
}

// morningDigestScheduler runs periodically to check if it's time to send the morning digest.
func (ext *ExtendedService) morningDigestScheduler() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		ext.checkMorningDigest()
	}
}

// checkMorningDigest checks if it's time to send the morning digest.
func (ext *ExtendedService) checkMorningDigest() {
	ext.mu.Lock()
	queued := ext.queuedDuringQuiet
	ext.mu.Unlock()

	if len(queued) == 0 {
		return
	}

	// Simple check: if we have queued notifications and it's after 7am, send digest
	now := time.Now()
	if now.Hour() < 7 {
		return
	}

	// Check if we already sent today
	todayDate := now.Format("2006-01-02")
	ext.mu.Lock()
	if ext.digestSentToday && ext.digestLastDate == todayDate {
		ext.mu.Unlock()
		return
	}
	ext.mu.Unlock()

	// Send morning digest
	go ext.sendMorningDigest()
}

// sendMorningDigest sends the queued notifications as a morning digest.
func (ext *ExtendedService) sendMorningDigest() {
	ext.mu.Lock()
	queued := ext.queuedDuringQuiet
	ext.queuedDuringQuiet = nil
	ext.digestSentToday = true
	ext.digestLastDate = time.Now().Format("2006-01-02")
	ext.mu.Unlock()

	if len(queued) == 0 {
		return
	}

	// Create digest notification
	var bodies []string

	for _, n := range queued {
		bodies = append(bodies, n.Body)
	}

	title := fmt.Sprintf("Morning digest - %d events while you slept", len(queued))
	body := fmt.Sprintf("While you were asleep: %s", strings.Join(bodies, ". "))

	digestNotif := Notification{
		Title:     title,
		Body:      body,
		Priority:  int(PriorityLow),
		Tags:      []string{"digest", "morning"},
		Data: map[string]interface{}{
			"event_count": len(queued),
		},
		Timestamp: time.Now(),
	}

	// Send digest immediately
	ext.Service.Send(digestNotif)

	log.Printf("[INFO] Morning digest sent with %d events", len(queued))
}

// GenerateFloorPlanThumbnailExtended generates a mini floor plan PNG with zone positions.
func (ext *ExtendedService) GenerateFloorPlanThumbnailExtended(width, height int, zones []struct {
	ID, Name, Color string
	X, Y, W, D      float64
	Highlight       bool
}, people []struct {
	Name, Color string
	X, Y, Z     float64
	Confidence  float64
	IsFall      bool
}) ([]byte, error) {
	// Create image
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill background
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{26, 26, 46, 255}}, image.Point{}, draw.Src)

	// Get room dimensions from room config provider
	var roomW, roomD float64 = 6.0, 5.0

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

		// Use NRGBA (non-premultiplied) for semi-transparent colors.
		// color.RGBA stores premultiplied alpha, which causes overflow artifacts
		// when the stored R/G/B values exceed the alpha value.
		zoneColor := color.NRGBA{79, 195, 247, 51} // Default blue with 20% opacity
		if zone.Highlight {
			zoneColor = color.NRGBA{79, 195, 247, 150} // Brighter for highlight
		}

		// Draw zone fill
		draw.Draw(img, image.Rect(x, y, x+w, y+h), &image.Uniform{zoneColor}, image.Point{}, draw.Over)

		// Draw outline
		var outlineColor color.Color = color.NRGBA{255, 255, 255, 100}
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

// GetHistoryExtended returns recent notification history with type information.
func (ext *ExtendedService) GetHistoryExtended(limit int) []struct {
	Channel   string
	Type      string
	Title     string
	Body      string
	Success   bool
	Error     string
	Timestamp time.Time
} {
	// Use the base Service's GetHistory method
	baseHistory := ext.Service.GetHistory(limit)

	// Convert to extended format
	var history []struct {
		Channel   string
		Type      string
		Title     string
		Body      string
		Success   bool
		Error     string
		Timestamp time.Time
	}

	for _, h := range baseHistory {
		history = append(history, struct {
			Channel   string
			Type      string
			Title     string
			Body      string
			Success   bool
			Error     string
			Timestamp time.Time
		}{
			Channel:   h.Channel,
			Type:      "", // Type would need to be stored separately
			Title:     h.Title,
			Body:      h.Body,
			Success:   h.Success,
			Error:     h.Error,
			Timestamp: h.Timestamp,
		})
	}

	return history
}

// SetBatchingConfig updates the batching configuration.
func (ext *ExtendedService) SetBatchingConfig(bc BatchingConfig) error {
	ext.mu.Lock()
	defer ext.mu.Unlock()
	ext.batching = &bc
	return nil
}

// GetBatchingConfig returns the current batching configuration.
func (ext *ExtendedService) GetBatchingConfig() *BatchingConfig {
	ext.mu.RLock()
	defer ext.mu.RUnlock()

	if ext.batching == nil {
		return &BatchingConfig{}
	}
	// Return a copy
	bc := *ext.batching
	return &bc
}
