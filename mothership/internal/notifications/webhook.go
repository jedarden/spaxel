// Package notifications provides notification delivery clients.
package notifications

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// WebhookClient delivers notifications to user-configured webhook URLs.
type WebhookClient struct {
	// URL is the webhook endpoint URL
	URL string

	// Method is the HTTP method to use (defaults to POST)
	Method string

	// Headers are additional HTTP headers to include
	Headers map[string]string

	// Timeout is the request timeout (defaults to 10s)
	Timeout time.Duration

	// HTTPClient is the HTTP client to use
	HTTPClient *http.Client
}

// WebhookPayload represents the JSON payload sent to webhook URLs.
type WebhookPayload struct {
	EventType         string  `json:"event_type"`
	Message           string  `json:"message"`
	Title             string  `json:"title,omitempty"`
	Priority          string  `json:"priority,omitempty"`
	PersonID          string  `json:"person_id,omitempty"`
	PersonName        string  `json:"person_name,omitempty"`
	ZoneID            string  `json:"zone_id,omitempty"`
	ZoneName          string  `json:"zone_name,omitempty"`
	Timestamp         int64   `json:"timestamp"`
	TimestampISO      string  `json:"timestamp_iso"`
	FloorplanPNGBase64 string `json:"floorplan_png_base64,omitempty"`

	// Additional fields for specific event types
	BlobID      *int     `json:"blob_id,omitempty"`
	BlobX       *float64 `json:"blob_x,omitempty"`
	BlobY       *float64 `json:"blob_y,omitempty"`
	BlobZ       *float64 `json:"blob_z,omitempty"`
	Confidence  *float64 `json:"confidence,omitempty"`

	// Node information for node events
	NodeMAC     string `json:"node_mac,omitempty"`
	NodeName    string `json:"node_name,omitempty"`
	NodeRole    string `json:"node_role,omitempty"`

	// Additional metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// NewWebhookClient creates a new webhook client.
func NewWebhookClient(url string) *WebhookClient {
	return &WebhookClient{
		URL:     url,
		Method:  "POST",
		Headers: make(map[string]string),
		Timeout: 10 * time.Second,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Send delivers a notification to the webhook URL.
func (c *WebhookClient) Send(payload WebhookPayload) error {
	if c == nil {
		return fmt.Errorf("webhook client is nil")
	}

	if c.URL == "" {
		return fmt.Errorf("webhook URL is required")
	}

	// Marshal payload to JSON
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	// Create request
	method := c.Method
	if method == "" {
		method = "POST"
	}

	req, err := http.NewRequest(method, c.URL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Spaxel/1.0")

	// Add custom headers
	for key, value := range c.Headers {
		req.Header.Set(key, value)
	}

	// Use client's HTTP client or default
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: c.Timeout,
		}
	}

	// Send request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	// Check response - accept 2xx status codes
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(body))
	}

	log.Printf("[INFO] webhook notification sent: url=%s event=%s", c.URL, payload.EventType)
	return nil
}

// SetHeader sets a custom header for webhook requests.
func (c *WebhookClient) SetHeader(key, value string) {
	if c.Headers == nil {
		c.Headers = make(map[string]string)
	}
	c.Headers[key] = value
}

// SetBasicAuth sets basic authentication headers.
func (c *WebhookClient) SetBasicAuth(username, password string) {
	// Note: Using a custom header instead of Authorization header
	// to avoid automatic handling by http.Client
	// Users can also use SetHeader("Authorization", "Basic "+encoded) directly
	c.SetHeader("X-Webhook-Username", username)
	c.SetHeader("X-Webhook-Password", password)
}

// SetTimeout sets the request timeout.
func (c *WebhookClient) SetTimeout(timeout time.Duration) {
	c.Timeout = timeout
	if c.HTTPClient != nil {
		c.HTTPClient.Timeout = timeout
	}
}

// AttachPNGImage sets the floorplan PNG base64 data in the payload.
func (p *WebhookPayload) AttachPNGImage(pngData []byte) {
	if len(pngData) == 0 {
		return
	}

	// Verify PNG signature
	if len(pngData) >= 8 && string(pngData[1:4]) == "PNG" {
		encoded := base64.StdEncoding.EncodeToString(pngData)
		p.FloorplanPNGBase64 = encoded
	}
}

// SetBlobPosition sets blob position data in the payload.
func (p *WebhookPayload) SetBlobPosition(blobID int, x, y, z, confidence float64) {
	p.BlobID = &blobID
	p.BlobX = &x
	p.BlobY = &y
	p.BlobZ = &z
	p.Confidence = &confidence
}

// SetNodeInfo sets node information in the payload.
func (p *WebhookPayload) SetNodeInfo(mac, name, role string) {
	p.NodeMAC = mac
	p.NodeName = name
	p.NodeRole = role
}

// AddMetadata adds a metadata field to the payload.
func (p *WebhookPayload) AddMetadata(key string, value interface{}) {
	if p.Metadata == nil {
		p.Metadata = make(map[string]interface{})
	}
	p.Metadata[key] = value
}

// NewWebhookPayload creates a new webhook payload with required fields.
func NewWebhookPayload(eventType, message string) WebhookPayload {
	now := time.Now()
	return WebhookPayload{
		EventType:    eventType,
		Message:      message,
		Timestamp:    now.Unix(),
		TimestampISO: now.Format(time.RFC3339),
	}
}

// NewFallDetectedPayload creates a webhook payload for fall detection events.
func NewFallDetectedPayload(personName, zoneName string, blobID int, x, y, z, confidence float64) WebhookPayload {
	payload := NewWebhookPayload("fall_detected", fmt.Sprintf("Fall detected: %s in %s", personName, zoneName))
	payload.Title = "Fall Detected"
	payload.Priority = "urgent"
	payload.PersonName = personName
	payload.ZoneName = zoneName
	payload.SetBlobPosition(blobID, x, y, z, confidence)
	payload.AddMetadata("requires_action", true)
	return payload
}

// NewZoneEnterPayload creates a webhook payload for zone entry events.
func NewZoneEnterPayload(personName, zoneName string) WebhookPayload {
	payload := NewWebhookPayload("zone_enter", fmt.Sprintf("%s entered %s", personName, zoneName))
	payload.PersonName = personName
	payload.ZoneName = zoneName
	payload.Priority = "low"
	return payload
}

// NewZoneLeavePayload creates a webhook payload for zone exit events.
func NewZoneLeavePayload(personName, zoneName string) WebhookPayload {
	payload := NewWebhookPayload("zone_leave", fmt.Sprintf("%s left %s", personName, zoneName))
	payload.PersonName = personName
	payload.ZoneName = zoneName
	payload.Priority = "low"
	return payload
}

// NewAnomalyAlertPayload creates a webhook payload for anomaly alerts.
func NewAnomalyAlertPayload(zoneName string, score float64, details string) WebhookPayload {
	payload := NewWebhookPayload("anomaly_alert", fmt.Sprintf("Unusual activity in %s", zoneName))
	payload.Title = "Anomaly Alert"
	payload.ZoneName = zoneName
	payload.Priority = "high"
	payload.AddMetadata("anomaly_score", score)
	payload.AddMetadata("details", details)
	return payload
}

// NewNodeOfflinePayload creates a webhook payload for node offline events.
func NewNodeOfflinePayload(nodeMAC, nodeName, nodeRole string) WebhookPayload {
	payload := NewWebhookPayload("node_offline", fmt.Sprintf("Node %s (%s) went offline", nodeName, nodeMAC))
	payload.Title = "Node Offline"
	payload.Priority = "medium"
	payload.SetNodeInfo(nodeMAC, nodeName, nodeRole)
	return payload
}
