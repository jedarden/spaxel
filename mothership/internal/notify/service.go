// Package notify provides push notification services with floor-plan thumbnails.
package notify

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
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

// NotificationChannel represents a notification destination.
type NotificationChannel string

const (
	ChannelNtfy    NotificationChannel = "ntfy"
	ChannelPushover NotificationChannel = "pushover"
	ChannelGotify   NotificationChannel = "gotify"
	ChannelWebhook   NotificationChannel = "webhook"
)

// Notification represents a notification to send.
type Notification struct {
	Title       string                 `json:"title"`
	Body        string                 `json:"body"`
	Priority    int                    `json:"priority,omitempty"` // 1-5 for Pushover
	Tags        []string               `json:"tags,omitempty"`
	Image       []byte                 `json:"-"` // Base64 encoded image
	ImageType   string                 `json:"image_type,omitempty"`
	Data        map[string]interface{} `json:"data,omitempty"`
	Timestamp   time.Time             `json:"timestamp"`
}

// ChannelConfig holds configuration for a notification channel.
type ChannelConfig struct {
	Type     NotificationChannel
	Enabled  bool
	URL      string            // ntfy server, gotify server, webhook URL
	Token    string            // pushover token, gotify token
	User     string            // pushover user key
	Username string            // basic auth username
	Password string            // basic auth password
	Headers  map[string]string  // custom headers for webhook
}

// QuietHoursConfig holds quiet hours configuration.
type QuietHoursConfig struct {
	Enabled   bool
	StartHour int // 0-23
	StartMin  int
	EndHour   int
	EndMin    int
}

// Service manages notification delivery.
type Service struct {
	mu           sync.RWMutex
	db           *sql.DB
	channels     map[string]*ChannelConfig
	quietHours   *QuietHoursConfig
	httpClient   *http.Client
	roomConfig   RoomConfigProvider
	floorPlan    []byte // Cached floor plan image

	// Batching
	pending      []batchedNotification
	batchTimer   *time.Timer
	batchWindow  time.Duration

	// Callbacks
	onSend       func(channel string, notif Notification, success bool)
}

type batchedNotification struct {
	notif    Notification
	channels []string
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
		batchWindow: 5 * time.Second,
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
			headers  TEXT    NOT NULL DEFAULT '{}'
		);

		CREATE TABLE IF NOT EXISTS notification_history (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			channel     TEXT    NOT NULL,
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
			end_m    INTEGER NOT NULL DEFAULT 0
		);

		INSERT OR IGNORE INTO quiet_hours (id, enabled, start_h, start_m, end_h, end_m)
		VALUES ('default', 0, 22, 0, 7, 0);
	`)
	return err
}

func (s *Service) loadChannels() error {
	rows, err := s.db.Query(`
		SELECT id, type, enabled, url, token, user_key, username, password, headers
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
		var headersJSON string

		if err := rows.Scan(&id, &cc.Type, &enabled, &cc.URL, &cc.Token, &cc.User, &cc.Username, &cc.Password, &headersJSON); err != nil {
			continue
		}

		cc.Enabled = enabled != 0
		if headersJSON != "" && headersJSON != "{}" {
			json.Unmarshal([]byte(headersJSON), &cc.Headers)
		}
		s.channels[id] = &cc
	}
	return nil
}

func (s *Service) loadQuietHours() error {
	row := s.db.QueryRow(`SELECT enabled, start_h, start_m, end_h, end_m FROM quiet_hours WHERE id = 'default'`)
	var enabled int
	qh := &QuietHoursConfig{}
	if err := row.Scan(&enabled, &qh.StartHour, &qh.StartMin, &qh.EndHour, &qh.EndMin); err != nil {
		return err
	}
	qh.Enabled = enabled != 0
	s.quietHours = qh
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

// SetFloorPlan sets the floor plan image for thumbnails.
func (s *Service) SetFloorPlan(imageData []byte) {
	s.mu.Lock()
	s.floorPlan = imageData
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

	headersJSON, _ := json.Marshal(cc.Headers)
	_, err := s.db.Exec(`
		INSERT INTO notification_channels (id, type, enabled, url, token, user_key, username, password, headers)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			type = excluded.type,
			enabled = excluded.enabled,
			url = excluded.url,
			token = excluded.token,
			user_key = excluded.user_key,
			username = excluded.username,
			password = excluded.password,
			headers = excluded.headers
	`, id, cc.Type, cc.Enabled, cc.URL, cc.Token, cc.User, cc.Username, cc.Password, string(headersJSON))
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

	_, err := s.db.Exec(`
		UPDATE quiet_hours SET enabled = ?, start_h = ?, start_m = ?, end_h = ?, end_m = ?
		WHERE id = 'default'
	`, qh.Enabled, qh.StartHour, qh.StartMin, qh.EndHour, qh.EndMin)
	if err != nil {
		return err
	}

	s.quietHours = &qh
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

	if startMins < endMins {
		// Quiet hours don't cross midnight
		return currentMins >= startMins && currentMins < endMins
	}
	// Quiet hours cross midnight
	return currentMins >= startMins || currentMins < endMins
}

// Send sends a notification to all enabled channels.
func (s *Service) Send(notif Notification) error {
	if s.isQuietHours() {
		log.Printf("[DEBUG] Notification suppressed (quiet hours): %s", notif.Title)
		return nil
	}

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

// SendBatched queues a notification for batch sending.
func (s *Service) SendBatched(notif Notification, channels []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pending = append(s.pending, batchedNotification{
		notif:    notif,
		channels: channels,
	})

	// Start batch timer if not running
	if s.batchTimer == nil {
		s.batchTimer = time.AfterFunc(s.batchWindow, s.flushBatch)
	}

	return nil
}

func (s *Service) flushBatch() {
	s.mu.Lock()
	pending := s.pending
	s.pending = nil
	s.batchTimer = nil
	s.mu.Unlock()

	if len(pending) == 0 {
		return
	}

	// Merge notifications by title
	merged := make(map[string]*Notification)
	for _, p := range pending {
		if existing, exists := merged[p.notif.Title]; exists {
			// Append body
			existing.Body += "\n" + p.notif.Body
		} else {
			merged[p.notif.Title] = &p.notif
		}
	}

	// Send merged notifications
	for _, notif := range merged {
		s.Send(*notif)
	}
}

// recordHistory records notification send attempt in history.
func (s *Service) recordHistory(channel string, notif Notification, success bool, err error) {
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}

	_, dbErr := s.db.Exec(`
		INSERT INTO notification_history (channel, title, body, success, error_msg, timestamp)
		VALUES (?, ?, ?, ?, ?, ?)
	`, channel, notif.Title, notif.Body, success, errMsg, time.Now().UnixNano())
	if dbErr != nil {
		log.Printf("[WARN] Failed to record notification history: %v", dbErr)
	}
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
	if notif.Priority > 0 {
		req.Header.Set("Priority", fmt.Sprintf("%d", notif.Priority))
	}

	// Attach image if available
	if len(notif.Image) > 0 && notif.ImageType != "" {
		// For ntfy, we attach as base64 in header
		req.Header.Set("X-Image", "data:"+notif.ImageType+";base64,"+base64.StdEncoding.EncodeToString(notif.Image))
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
	if notif.Priority > 0 {
		data.Set("priority", fmt.Sprintf("%d", notif.Priority))
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
	if len(notif.Tags) > 0 {
		payload["extras"] = map[string]interface{}{
			"client::display": map[string]interface{}{
				"contentType": "text/markdown",
			},
		}
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
		"title":     notif.Title,
		"body":      notif.Body,
		"priority":  notif.Priority,
		"tags":      notif.Tags,
		"timestamp": notif.Timestamp.Format(time.RFC3339),
	}
	for k, v := range notif.Data {
		payload[k] = v
	}

	// Add image as base64 if available
	if len(notif.Image) > 0 && notif.ImageType != "" {
		payload["image"] = "data:" + notif.ImageType + ";base64," + base64.StdEncoding.EncodeToString(notif.Image)
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
func (s *Service) GenerateFloorPlanThumbnail(width, height int, blobs []struct {
	X, Y, Z float64
	Identity string
	IsFall   bool
}) ([]byte, error) {
	s.mu.RLock()
	roomConfig := s.roomConfig
	floorPlan := s.floorPlan
	s.mu.RUnlock()

	// Create image
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill background
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{26, 26, 46, 255}}, image.Point{}, draw.Src)

	// Get room dimensions
	var roomW, roomD float64 = 6.0, 5.0 // defaults
	if roomConfig != nil {
		roomW, _, roomD = roomConfig.GetRoom()
	}

	// Scale factor
	scaleX := float64(width) / roomW
	scaleZ := float64(height) / roomD

	// Draw floor plan if available
	if len(floorPlan) > 0 {
		// Would decode and draw, but skip for simplicity
	}

	// Draw blobs
	for _, blob := range blobs {
		px := int(blob.X * scaleX)
		pz := int(blob.Z * scaleZ)

		// Clamp to image bounds
		if px < 5 || px >= width-5 || pz < 5 || pz >= height-5 {
			continue
		}

		// Color based on state
		clr := color.RGBA{100, 181, 246, 255} // Blue - normal
		if blob.IsFall {
			clr = color.RGBA{239, 83, 80, 255} // Red - fall detected
		} else if blob.Identity != "" {
			clr = color.RGBA{129, 199, 132, 255} // Green - identified
		}

		// Draw circle
		for dy := -4; dy <= 4; dy++ {
			for dx := -4; dx <= 4; dx++ {
				if dx*dx+dy*dy <= 16 {
					img.Set(px+dx, pz+dy, clr)
				}
			}
		}
	}

	// Draw room outline
	outlineColor := color.RGBA{100, 100, 120, 255}
	for x := 0; x < width; x++ {
		img.Set(x, 0, outlineColor)
		img.Set(x, height-1, outlineColor)
	}
	for y := 0; y < height; y++ {
		img.Set(0, y, outlineColor)
		img.Set(width-1, y, outlineColor)
	}

	// Encode to PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// GetHistory returns recent notification history.
func (s *Service) GetHistory(limit int) []struct {
	Channel   string
	Title     string
	Body      string
	Success   bool
	Error     string
	Timestamp time.Time
} {
	rows, err := s.db.Query(`
		SELECT channel, title, body, success, error_msg, timestamp
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
		Title     string
		Body      string
		Success   bool
		Error     string
		Timestamp time.Time
	}

	for rows.Next() {
		var h struct {
			Channel   string
			Title     string
			Body      string
			Success   bool
			Error     string
			Timestamp time.Time
		}
		var success int
		var ts int64
		if err := rows.Scan(&h.Channel, &h.Title, &h.Body, &success, &h.Error, &ts); err != nil {
			continue
		}
		h.Success = success != 0
		h.Timestamp = time.Unix(0, ts)
		history = append(history, h)
	}

	return history
}
