// Package webhook provides system webhook integration for publishing all spaxel events.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/spaxel/mothership/internal/eventbus"
)

// Config holds webhook configuration.
type Config struct {
	URL            string        // Target URL for webhook delivery
	Timeout        time.Duration // Request timeout (default 5s)
	RetryDelay     time.Duration // Delay before retry (default 30s)
	Enabled        bool          // Whether webhook is enabled
}

// EventPayload represents the JSON payload sent to the webhook.
type EventPayload struct {
	EventType string                 `json:"event_type"` // zone_entry, zone_exit, fall_alert, anomaly, etc.
	Timestamp string                 `json:"timestamp"`
	Zone      string                 `json:"zone,omitempty"`
	Person    string                 `json:"person,omitempty"`
	BlobID    int                    `json:"blob_id,omitempty"`
	Severity  string                 `json:"severity,omitempty"`
	Detail    map[string]interface{} `json:"detail,omitempty"`
}

// Publisher publishes all spaxel events to a configured webhook URL.
type Publisher struct {
	mu       sync.RWMutex
	config   Config
	client   *http.Client
	stopped  chan struct{}
}

// NewPublisher creates a new webhook publisher.
func NewPublisher(cfg Config) *Publisher {
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.RetryDelay == 0 {
		cfg.RetryDelay = 30 * time.Second
	}

	return &Publisher{
		config:  cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
		stopped: make(chan struct{}),
	}
}

// UpdateConfig updates the webhook configuration.
func (p *Publisher) UpdateConfig(cfg Config) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.config = cfg
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.RetryDelay == 0 {
		cfg.RetryDelay = 30 * time.Second
	}
	p.client.Timeout = cfg.Timeout
}

// GetConfig returns the current webhook configuration.
func (p *Publisher) GetConfig() Config {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.config
}

// Start begins subscribing to events and publishing them to the webhook.
func (p *Publisher) Start() {
	log.Printf("[INFO] System webhook publisher starting (url=%s, enabled=%v)",
		p.config.URL, p.config.Enabled)

	// Subscribe to all event types
	eventbus.SubscribeDefault(func(e eventbus.Event) {
		p.publishEvent(e)
	})
}

// Stop stops the webhook publisher.
func (p *Publisher) Stop() {
	close(p.stopped)
	log.Printf("[INFO] System webhook publisher stopped")
}

// publishEvent publishes a single event to the webhook.
func (p *Publisher) publishEvent(e eventbus.Event) {
	select {
	case <-p.stopped:
		return
	default:
	}

	p.mu.RLock()
	enabled := p.config.Enabled
	url := p.config.URL
	p.mu.RUnlock()

	if !enabled || url == "" {
		return
	}

	// Build event payload
	payload := EventPayload{
		EventType: e.Type,
		Timestamp: time.Unix(0, e.TimestampMs).Format(time.RFC3339),
		Zone:      e.Zone,
		Person:    e.Person,
		BlobID:    e.BlobID,
		Severity:  e.Severity,
	}

	// Add detail if present
	if e.Detail != nil {
		if detail, ok := e.Detail.(map[string]interface{}); ok {
			payload.Detail = detail
		}
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[WARN] Failed to marshal webhook payload: %v", err)
		return
	}

	// Send to webhook with retry
	if err := p.sendWithRetry(jsonData); err != nil {
		log.Printf("[WARN] Failed to send webhook event: %v", err)
	}
}

// sendWithRetry sends the payload with a single retry on 5xx errors.
func (p *Publisher) sendWithRetry(jsonData []byte) error {
	p.mu.RLock()
	url := p.config.URL
	retryDelay := p.config.RetryDelay
	p.mu.RUnlock()

	// First attempt
	if err := p.sendOnce(url, jsonData); err == nil {
		return nil
	}

	// Retry on 5xx after delay
	time.Sleep(retryDelay)
	return p.sendOnce(url, jsonData)
}

// sendOnce sends a single webhook request.
func (p *Publisher) sendOnce(url string, jsonData []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), p.config.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Spaxel-Event", "spaxel-event") // Event type header
	req.Header.Set("User-Agent", "Spaxel/1.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check for 5xx errors (retryable)
	if resp.StatusCode >= 500 {
		return &HTTPError{StatusCode: resp.StatusCode, Message: "server error"}
	}

	// Check for 4xx errors (not retryable, but log)
	if resp.StatusCode >= 400 {
		log.Printf("[WARN] Webhook returned error status %d for %s event",
			resp.StatusCode, req.Header.Get("X-Spaxel-Event"))
	}

	return nil
}

// HTTPError represents an HTTP error with status code.
type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	return e.Message
}

// TestWebhook sends a test event to verify webhook configuration.
func (p *Publisher) TestWebhook() error {
	p.mu.RLock()
	url := p.config.URL
	p.mu.RUnlock()

	if url == "" {
		return &ValidationError{Field: "url", Reason: "webhook URL is not configured"}
	}

	// Create a test event payload
	payload := EventPayload{
		EventType: "test",
		Timestamp: time.Now().Format(time.RFC3339),
		Detail: map[string]interface{}{
			"message": "This is a test webhook event from Spaxel",
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	return p.sendOnce(url, jsonData)
}

// ValidationError represents a webhook configuration validation error.
type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Reason
}
