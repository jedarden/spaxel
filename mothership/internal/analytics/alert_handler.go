// Package analytics provides alert handling for anomaly detection using the notification service.
package analytics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/spaxel/mothership/internal/events"
)

// NotificationAlertHandler implements AlertHandler using a notification service.
type NotificationAlertHandler struct {
	notifyService NotificationService
	httpClient   *http.Client
	webhookURL   string
	escalationURL string
}

// NotificationService is the interface needed from the notify package.
type NotificationService interface {
	Send(notif Notification) error
	GenerateFloorPlanThumbnail(width, height int, blobs []struct {
		X, Y, Z   float64
		Identity  string
		IsFall    bool
	}) ([]byte, error)
}

// Notification represents a notification to send.
type Notification struct {
	Title     string
	Body      string
	Priority  int
	Tags      []string
	Image     []byte
	ImageType string
	Data      map[string]interface{}
	Timestamp time.Time
}

// NewNotificationAlertHandler creates a new alert handler backed by the notification service.
func NewNotificationAlertHandler(notifyService NotificationService) *NotificationAlertHandler {
	return &NotificationAlertHandler{
		notifyService: notifyService,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

// SetWebhookURL sets the webhook URL for anomaly alerts.
func (h *NotificationAlertHandler) SetWebhookURL(url string) {
	h.webhookURL = url
}

// SetEscalationURL sets the escalation webhook URL.
func (h *NotificationAlertHandler) SetEscalationURL(url string) {
	h.escalationURL = url
}

// SendAlert sends an alert notification.
func (h *NotificationAlertHandler) SendAlert(event events.AnomalyEvent, immediate bool) error {
	// Generate floor plan thumbnail
	thumbnail, err := h.notifyService.GenerateFloorPlanThumbnail(400, 300, []struct {
		X, Y, Z   float64
		Identity  string
		IsFall    bool
	}{
		{
			X:        event.Position.X,
			Y:        event.Position.Y,
			Z:        event.Position.Z,
			Identity: event.PersonName,
			IsFall:   false,
		},
	})
	if err != nil {
		log.Printf("[WARN] Failed to generate thumbnail for alert: %v", err)
	}

	notif := Notification{
		Title:     "Anomaly Detected",
		Body:      event.Description,
		Priority:  2, // High priority for anomalies
		Tags:      []string{"spaxel", "anomaly"},
		Image:     thumbnail,
		ImageType: "image/png",
		Data: map[string]interface{}{
			"anomaly_id":      event.ID,
			"anomaly_type":    string(event.Type),
			"zone_id":         event.ZoneID,
			"zone_name":       event.ZoneName,
			"person_id":       event.PersonID,
			"person_name":     event.PersonName,
			"timestamp":       event.Timestamp.Format(time.RFC3339),
			"immediate":       immediate,
		},
		Timestamp: time.Now(),
	}

	// Set higher priority for security mode
	if immediate {
		notif.Priority = 4 // Emergency priority
		notif.Tags = append(notif.Tags, "security")
	}

	return h.notifyService.Send(notif)
}

// SendWebhook sends a webhook notification.
func (h *NotificationAlertHandler) SendWebhook(event events.AnomalyEvent, immediate bool) error {
	if h.webhookURL == "" {
		return nil // No webhook configured
	}

	payload := map[string]interface{}{
		"anomaly_id":      event.ID,
		"type":             string(event.Type),
		"score":            event.Score,
		"description":      event.Description,
		"timestamp":        event.Timestamp.Format(time.RFC3339),
		"zone_id":          event.ZoneID,
		"zone_name":        event.ZoneName,
		"person_id":        event.PersonID,
		"person_name":      event.PersonName,
		"device_mac":       event.DeviceMAC,
		"position":         event.Position,
		"hour_of_week":     event.HourOfWeek,
		"expected_occupancy": event.ExpectedOccupancy,
		"immediate":        immediate,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	req, err := http.NewRequest("POST", h.webhookURL, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("create webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Spaxel/1.0")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	log.Printf("[INFO] Anomaly webhook sent: %s (status: %d)", event.ID, resp.StatusCode)
	return nil
}

// SendEscalation sends an escalation webhook notification.
func (h *NotificationAlertHandler) SendEscalation(event events.AnomalyEvent) error {
	if h.escalationURL == "" {
		return nil // No escalation webhook configured
	}

	payload := map[string]interface{}{
		"anomaly_id":   event.ID,
		"type":          string(event.Type),
		"score":         event.Score,
		"description":   event.Description,
		"timestamp":     event.Timestamp.Format(time.RFC3339),
		"zone_id":       event.ZoneID,
		"zone_name":     event.ZoneName,
		"person_id":     event.PersonID,
		"person_name":   event.PersonName,
		"escalation":    true,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal escalation payload: %w", err)
	}

	req, err := http.NewRequest("POST", h.escalationURL, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("create escalation request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Spaxel/1.0")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send escalation: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("escalation returned status %d", resp.StatusCode)
	}

	log.Printf("[INFO] Anomaly escalation sent: %s (status: %d)", event.ID, resp.StatusCode)
	return nil
}
