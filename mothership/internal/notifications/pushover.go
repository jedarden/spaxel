// Package notifications provides notification delivery clients.
package notifications

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"time"
)

// PushoverClient delivers notifications to the Pushover API.
type PushoverClient struct {
	// AppToken is the Pushover application token
	AppToken string

	// UserKey is the Pushover user key
	UserKey string

	// APIURL is the Pushover API endpoint (defaults to https://api.pushover.net/1/messages.json)
	APIURL string

	// Device is the optional device name to send to
	Device string

	// Title is the default message title
	Title string

	// URL is an optional URL to include
	URL string

	// URLTitle is an optional title for the URL
	URLTitle string

	// Priority is the default message priority (-2 to 2)
	// -2: lowest, -1: low, 0: normal, 1: high, 2: emergency
	Priority int

	// Sound is the notification sound (defaults to pushover default)
	Sound string

	// HTTPClient is the HTTP client to use
	HTTPClient *http.Client
}

// PushoverMessage represents a notification message for Pushover.
type PushoverMessage struct {
	Message    string
	Title      string
	Priority   int
	Device     string
	URL        string
	URLTitle   string
	Sound      string
	Timestamp  int64

	// PNGImageData is optional PNG image data to attach
	PNGImageData []byte

	// Emergency settings for priority 2
	Retry   int // Retry in seconds (min 30)
	Expire  int // Expire in seconds (max 10800)
}

// NewPushoverClient creates a new Pushover client.
func NewPushoverClient(appToken, userKey string) *PushoverClient {
	return &PushoverClient{
		AppToken:  appToken,
		UserKey:   userKey,
		APIURL:    "https://api.pushover.net/1/messages.json",
		Priority:  0,
		Sound:     "pushover",
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Send delivers a notification to Pushover.
func (c *PushoverClient) Send(msg PushoverMessage) error {
	if c == nil {
		return fmt.Errorf("pushover client is nil")
	}

	if c.AppToken == "" {
		return fmt.Errorf("app token is required")
	}

	if c.UserKey == "" {
		return fmt.Errorf("user key is required")
	}

	// Use client defaults if not specified
	if msg.Title == "" && c.Title != "" {
		msg.Title = c.Title
	}
	if msg.Device == "" && c.Device != "" {
		msg.Device = c.Device
	}
	if msg.URL == "" && c.URL != "" {
		msg.URL = c.URL
	}
	if msg.URLTitle == "" && c.URLTitle != "" {
		msg.URLTitle = c.URLTitle
	}
	if msg.Sound == "" && c.Sound != "" {
		msg.Sound = c.Sound
	}

	if msg.Message == "" {
		return fmt.Errorf("message is required")
	}

	// Create multipart form body
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add required fields
	writeField(writer, "token", c.AppToken)
	writeField(writer, "user", c.UserKey)
	writeField(writer, "message", msg.Message)

	// Add optional fields
	if msg.Title != "" {
		writeField(writer, "title", msg.Title)
	}

	// Use message priority if set, otherwise use client default
	priority := c.Priority
	if msg.Priority != 0 {
		priority = msg.Priority
	}
	// Validate priority range
	if priority < -2 || priority > 2 {
		priority = 0
	}
	writeField(writer, "priority", fmt.Sprintf("%d", priority))

	if msg.Device != "" {
		writeField(writer, "device", msg.Device)
	}

	if msg.URL != "" {
		writeField(writer, "url", msg.URL)
	}

	if msg.URLTitle != "" {
		writeField(writer, "url_title", msg.URLTitle)
	}

	if msg.Sound != "" {
		writeField(writer, "sound", msg.Sound)
	}

	if msg.Timestamp > 0 {
		writeField(writer, "timestamp", fmt.Sprintf("%d", msg.Timestamp))
	}

	// Emergency settings for priority 2
	if priority == 2 {
		if msg.Retry > 0 {
			// Minimum retry is 30 seconds
			if msg.Retry < 30 {
				msg.Retry = 30
			}
			writeField(writer, "retry", fmt.Sprintf("%d", msg.Retry))
		}
		if msg.Expire > 0 {
			// Maximum expire is 10800 seconds (3 hours)
			if msg.Expire > 10800 {
				msg.Expire = 10800
			}
			writeField(writer, "expire", fmt.Sprintf("%d", msg.Expire))
		}
	}

	// Add PNG attachment if provided
	if len(msg.PNGImageData) > 0 {
		// Validate it's a PNG by checking the signature
		if len(msg.PNGImageData) >= 8 && string(msg.PNGImageData[1:4]) == "PNG" {
			h := make(textproto.MIMEHeader)
			h.Set("Content-Disposition", `form-data; name="attachment"; filename="notification.png"`)
			h.Set("Content-Type", "image/png")

			part, err := writer.CreatePart(h)
			if err != nil {
				return fmt.Errorf("create attachment part: %w", err)
			}

			if _, err := part.Write(msg.PNGImageData); err != nil {
				return fmt.Errorf("write attachment data: %w", err)
			}
		}
	}

	// Close multipart writer
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close multipart writer: %w", err)
	}

	// Create request
	url := c.APIURL
	if url == "" {
		url = "https://api.pushover.net/1/messages.json"
	}

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

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
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("pushover returned status %d: %s", resp.StatusCode, string(respBody))
	}

	log.Printf("[INFO] pushover notification sent: title=%s", msg.Title)
	return nil
}

// SetPriority sets the default message priority.
func (c *PushoverClient) SetPriority(priority int) {
	if priority >= -2 && priority <= 2 {
		c.Priority = priority
	}
}

// SetSound sets the notification sound.
func (c *PushoverClient) SetSound(sound string) {
	c.Sound = sound
}

// SetDevice sets the target device.
func (c *PushoverClient) SetDevice(device string) {
	c.Device = device
}

// SetURL sets the URL to include with notifications.
func (c *PushoverClient) SetURL(url, urlTitle string) {
	c.URL = url
	c.URLTitle = urlTitle
}

// writeField is a helper that writes a form field and logs errors.
func writeField(writer *multipart.Writer, key, value string) {
	if err := writer.WriteField(key, value); err != nil {
		log.Printf("[WARN] Failed to write form field %s: %v", key, err)
	}
}

// AttachPNGBase64 decodes a base64 string and returns PNG data.
func AttachPNGBase64(base64Data string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return nil, fmt.Errorf("decode base64: %w", err)
	}

	// Verify PNG signature
	if len(data) < 8 {
		return nil, fmt.Errorf("data too short to be PNG")
	}

	// PNG signature: 137 80 78 71 13 10 26 10
	pngSig := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	for i := 0; i < 8; i++ {
		if data[i] != pngSig[i] {
			return nil, fmt.Errorf("data does not appear to be PNG")
		}
	}

	return data, nil
}
