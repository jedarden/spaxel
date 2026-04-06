// Package api provides REST API handlers for Spaxel notification channels.
package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-chi/chi"
	_ "modernc.org/sqlite"
)

// NotificationsHandler manages notification delivery channels.
type NotificationsHandler struct {
	mu            sync.RWMutex
	db            *sql.DB
	channels      map[string]*NotificationChannel
	notifyService NotifySender
}

// NotificationChannel represents a notification delivery channel.
type NotificationChannel struct {
	Type    string      `json:"type"` // ntfy, pushover, gotify, webhook
	Enabled bool        `json:"enabled"`
	Config  interface{} `json:"config"`
}

// NotifySender is the interface for sending test notifications.
type NotifySender interface {
	Send(title, body string, data map[string]interface{}) error
}

// NewNotificationsHandler creates a new notifications handler.
func NewNotificationsHandler(dbPath string) (*NotificationsHandler, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	n := &NotificationsHandler{
		db:       db,
		channels: make(map[string]*NotificationChannel),
	}

	if err := n.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	if err := n.load(); err != nil {
		log.Printf("[WARN] Failed to load notification channels: %v", err)
	}

	return n, nil
}

func (n *NotificationsHandler) migrate() error {
	_, err := n.db.Exec(`
		CREATE TABLE IF NOT EXISTS notification_channels (
			id       TEXT PRIMARY KEY,
			type     TEXT    NOT NULL,
			enabled  INTEGER NOT NULL DEFAULT 0,
			config   TEXT    NOT NULL DEFAULT '{}',
			updated_at INTEGER NOT NULL DEFAULT 0
		);
	`)
	return err
}

func (n *NotificationsHandler) load() error {
	rows, err := n.db.Query(`SELECT id, type, enabled, config FROM notification_channels`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var nc NotificationChannel
		var id string
		var enabled int
		var configJSON string

		if err := rows.Scan(&id, &nc.Type, &enabled, &configJSON); err != nil {
			continue
		}

		nc.Enabled = enabled != 0
		if err := json.Unmarshal([]byte(configJSON), &nc.Config); err != nil {
			// Keep as string if not valid JSON
			nc.Config = configJSON
		}

		n.channels[id] = &nc
	}

	return nil
}

// Close closes the database.
func (n *NotificationsHandler) Close() error {
	return n.db.Close()
}

// SetNotifyService sets the notification sender for test notifications.
func (n *NotificationsHandler) SetNotifyService(ns NotifySender) {
	n.mu.Lock()
	n.notifyService = ns
	n.mu.Unlock()
}

// RegisterRoutes registers notification endpoints.
//
// GET    /api/notifications/config — get delivery channel config
// POST   /api/notifications/config — set channel config
// POST   /api/notifications/test  — send a test notification
func (n *NotificationsHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/notifications/config", n.getConfig)
	r.Post("/api/notifications/config", n.setConfig)
	r.Post("/api/notifications/test", n.sendTest)
}

func (n *NotificationsHandler) getConfig(w http.ResponseWriter, r *http.Request) {
	n.mu.RLock()
	channels := make(map[string]*NotificationChannel)
	for k, v := range n.channels {
		channels[k] = v
	}
	n.mu.RUnlock()

	writeJSON(w, map[string]interface{}{
		"channels": channels,
	})
}

type setConfigRequest struct {
	Channels map[string]struct {
		Type    string      `json:"type"`
		Enabled bool        `json:"enabled"`
		Config  interface{} `json:"config"`
	} `json:"channels"`
}

func (n *NotificationsHandler) setConfig(w http.ResponseWriter, r *http.Request) {
	var req setConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	now := time.Now().UnixNano()

	for id, ch := range req.Channels {
		configJSON, err := json.Marshal(ch.Config)
		if err != nil {
			http.Error(w, "failed to marshal config", http.StatusBadRequest)
			return
		}
		enabled := 0
		if ch.Enabled {
			enabled = 1
		}

		_, err = n.db.Exec(`
			INSERT INTO notification_channels (id, type, enabled, config, updated_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET type = ?, enabled = ?, config = ?, updated_at = ?
		`, id, ch.Type, enabled, string(configJSON), now,
			ch.Type, enabled, string(configJSON), now)
		if err != nil {
			http.Error(w, "failed to save config", http.StatusInternalServerError)
			return
		}

		n.mu.Lock()
		n.channels[id] = &NotificationChannel{
			Type:    ch.Type,
			Enabled: ch.Enabled,
			Config:  ch.Config,
		}
		n.mu.Unlock()
	}

	n.getConfig(w, r)
}

type testNotificationRequest struct {
	ChannelType string                 `json:"channel_type"`
	Title       string                 `json:"title"`
	Body        string                 `json:"body"`
	Data        map[string]interface{} `json:"data,omitempty"`
}

func (n *NotificationsHandler) sendTest(w http.ResponseWriter, r *http.Request) {
	var req testNotificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Title == "" {
		req.Title = "Spaxel Test Notification"
	}
	if req.Body == "" {
		req.Body = "This is a test notification from Spaxel."
	}
	if req.Data == nil {
		req.Data = make(map[string]interface{})
	}
	req.Data["test"] = true

	// Check if channel type exists
	var found bool
	n.mu.RLock()
	for _, ch := range n.channels {
		if ch.Type == req.ChannelType && ch.Enabled {
			found = true
			break
		}
	}
	n.mu.RUnlock()

	if !found {
		http.Error(w, "no enabled channel found for type: "+req.ChannelType, http.StatusBadRequest)
		return
	}

	// Send test notification
	n.mu.RLock()
	sender := n.notifyService
	n.mu.RUnlock()

	if sender == nil {
		writeJSON(w, map[string]interface{}{
			"status":  "simulated",
			"message": "Test notification simulated (no sender attached)",
		})
		return
	}

	if err := sender.Send(req.Title, req.Body, req.Data); err != nil {
		http.Error(w, "failed to send notification: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{
		"status":  "sent",
		"message": "Test notification sent successfully",
	})
}

// ── Notification sending (called by automation engine) ────────────────────────────

// SendNotification sends a notification via all enabled channels.
func (n *NotificationsHandler) SendNotification(title, body string, data map[string]interface{}) error {
	n.mu.RLock()
	sender := n.notifyService
	channels := make([]NotificationChannel, 0, len(n.channels))
	for _, ch := range n.channels {
		if ch.Enabled {
			channels = append(channels, *ch)
		}
	}
	n.mu.RUnlock()

	if len(channels) == 0 {
		return nil
	}

	if sender == nil {
		log.Printf("[INFO] No notification sender attached, skipping: %s", title)
		return nil
	}

	return sender.Send(title, body, data)
}
