// Package notifications provides notification delivery clients.
package notifications

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// NtfyClient delivers notifications to ntfy.sh or self-hosted ntfy servers.
type NtfyClient struct {
	// URL is the ntfy server URL. If empty, defaults to "https://ntfy.sh"
	URL string

	// Topic is the ntfy topic to publish to
	Topic string

	// Token is the optional authentication token for private topics
	Token string

	// Priority is the default message priority (can be "min", "low", "default", "high", "urgent")
	Priority string

	// Tags are optional message tags
	Tags []string

	// Click is an optional URL to open when the notification is clicked
	Click string

	// Icon is an optional URL to an icon image
	Icon string

	// Delay is an optional delay for message delivery
	Delay string

	// Email is optional for email forwarding
	Email string

	// HTTPClient is the HTTP client to use (defaults to a 30s timeout client)
	HTTPClient *http.Client
}

// NtfyMessage represents a notification message for ntfy.sh
type NtfyMessage struct {
	Topic    string
	Title    string
	Message  string
	Priority string
	Tags     []string
	Click    string
	Icon     string
	Delay    string
	Email    string

	// Image is optional base64-encoded PNG data (data:image/png;base64,...)
	Image string
}

// NewNtfyClient creates a new ntfy client with default settings.
func NewNtfyClient(topic string) *NtfyClient {
	return &NtfyClient{
		URL:       "https://ntfy.sh",
		Topic:     topic,
		Priority:  "default",
		Tags:      nil,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Send delivers a notification to ntfy.
func (c *NtfyClient) Send(msg NtfyMessage) error {
	if c == nil {
		return fmt.Errorf("ntfy client is nil")
	}

	// Use client defaults if not specified
	if msg.Topic == "" && c.Topic != "" {
		msg.Topic = c.Topic
	}
	if msg.Priority == "" && c.Priority != "" {
		msg.Priority = c.Priority
	}
	if len(msg.Tags) == 0 && len(c.Tags) > 0 {
		msg.Tags = c.Tags
	}
	if msg.Click == "" && c.Click != "" {
		msg.Click = c.Click
	}
	if msg.Icon == "" && c.Icon != "" {
		msg.Icon = c.Icon
	}
	if msg.Delay == "" && c.Delay != "" {
		msg.Delay = c.Delay
	}
	if msg.Email == "" && c.Email != "" {
		msg.Email = c.Email
	}

	if msg.Topic == "" {
		return fmt.Errorf("topic is required")
	}

	// Build URL
	url := fmt.Sprintf("%s/%s", c.URL, msg.Topic)

	// Create request body
	body := msg.Message

	// Create request
	req, err := http.NewRequest("POST", url, bytes.NewBufferString(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "text/plain")

	if msg.Title != "" {
		req.Header.Set("Title", msg.Title)
	}

	if msg.Priority != "" {
		// Validate priority
		validPriorities := map[string]bool{
			"min":     true,
			"low":     true,
			"default": true,
			"high":    true,
			"urgent":  true,
		}
		if validPriorities[msg.Priority] {
			req.Header.Set("Priority", msg.Priority)
		}
	}

	if len(msg.Tags) > 0 {
		tags := ""
		for i, tag := range msg.Tags {
			if i > 0 {
				tags += ","
			}
			tags += tag
		}
		req.Header.Set("Tags", tags)
	}

	if msg.Click != "" {
		req.Header.Set("Click", msg.Click)
	}

	if msg.Icon != "" {
		req.Header.Set("Icon", msg.Icon)
	}

	if msg.Delay != "" {
		req.Header.Set("Delay", msg.Delay)
	}

	if msg.Email != "" {
		req.Header.Set("Email", msg.Email)
	}

	if msg.Image != "" {
		// Attach image as base64 data URL
		req.Header.Set("Attach", msg.Image)
	}

	// Add authentication token if provided
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	// Use client's HTTP client or default
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	// Send request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ntfy returned status %d: %s", resp.StatusCode, string(body))
	}

	log.Printf("[INFO] ntfy notification sent: topic=%s title=%s", msg.Topic, msg.Title)
	return nil
}

// AttachPNGImage encodes PNG data as a base64 data URL for attachment.
func AttachPNGImage(pngData []byte) string {
	encoded := base64.StdEncoding.EncodeToString(pngData)
	return "data:image/png;base64," + encoded
}

// SetPriority sets the message priority.
func (c *NtfyClient) SetPriority(priority string) {
	c.Priority = priority
}

// SetToken sets the authentication token.
func (c *NtfyClient) SetToken(token string) {
	c.Token = token
}

// SetURL sets the ntfy server URL.
func (c *NtfyClient) SetURL(url string) {
	c.URL = url
}
