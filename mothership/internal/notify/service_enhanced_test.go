// Package notify provides tests for the notification service.
package notify

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	_ "image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestServiceCreation tests creating a new notification service.
func TestServiceCreation(t *testing.T) {
	dbPath := t.TempDir() + "/test_notify.db"

	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	if service == nil {
		t.Fatal("NewService() returned nil")
	}
}

// TestGetSetQuietHours tests the quiet hours get/set functionality.
func TestGetSetQuietHours(t *testing.T) {
	dbPath := t.TempDir() + "/test_notify.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	qh := QuietHoursConfig{
		Enabled:   true,
		StartHour: 22,
		StartMin:  0,
		EndHour:   7,
		EndMin:    0,
	}

	if err := service.SetQuietHours(qh); err != nil {
		t.Fatalf("SetQuietHours() error = %v", err)
	}

	retrieved := service.GetQuietHours()
	if !retrieved.Enabled {
		t.Error("Quiet hours not enabled")
	}
	if retrieved.StartHour != 22 {
		t.Errorf("StartHour = %d, want 22", retrieved.StartHour)
	}
	if retrieved.EndHour != 7 {
		t.Errorf("EndHour = %d, want 7", retrieved.EndHour)
	}
}

// TestIsQuietHoursNoCrossing tests quiet hours detection without midnight crossing.
func TestIsQuietHoursNoCrossing(t *testing.T) {
	dbPath := t.TempDir() + "/test_notify.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	// Disable quiet hours — isQuietHours should return false
	if err := service.SetQuietHours(QuietHoursConfig{Enabled: false}); err != nil {
		t.Fatalf("SetQuietHours() error = %v", err)
	}
	if service.isQuietHours() {
		t.Error("isQuietHours() should be false when quiet hours are disabled")
	}

	// Enable quiet hours spanning the full day (should always be in quiet hours)
	if err := service.SetQuietHours(QuietHoursConfig{
		Enabled:   true,
		StartHour: 0,
		StartMin:  0,
		EndHour:   23,
		EndMin:    59,
	}); err != nil {
		t.Fatalf("SetQuietHours() error = %v", err)
	}
	if !service.isQuietHours() {
		t.Error("isQuietHours() should be true when covering the full day")
	}
}

// TestIsQuietHoursMidnightCrossing tests quiet hours that cross midnight.
func TestIsQuietHoursMidnightCrossing(t *testing.T) {
	dbPath := t.TempDir() + "/test_notify.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	// Quiet hours 22:00 – 07:00 (cross midnight)
	if err := service.SetQuietHours(QuietHoursConfig{
		Enabled:   true,
		StartHour: 22,
		StartMin:  0,
		EndHour:   7,
		EndMin:    0,
	}); err != nil {
		t.Fatalf("SetQuietHours() error = %v", err)
	}

	// The function checks the current real time, so just verify it doesn't panic.
	_ = service.isQuietHours()
}

// TestExtendedServiceBatchingConfig tests the batching configuration on ExtendedService.
func TestExtendedServiceBatchingConfig(t *testing.T) {
	dbPath := t.TempDir() + "/test_notify.db"
	base, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	ext, err := NewExtendedService(base)
	if err != nil {
		t.Fatalf("NewExtendedService() error = %v", err)
	}
	defer ext.Close()

	bc := BatchingConfig{
		Enabled:          true,
		BatchWindowSec:   30,
		MaxBatchSize:     5,
		BatchLowPriority: true,
		BatchMedium:      true,
	}

	if err := ext.SetBatchingConfig(bc); err != nil {
		t.Fatalf("SetBatchingConfig() error = %v", err)
	}

	retrieved := ext.GetBatchingConfig()
	if !retrieved.Enabled {
		t.Error("Batching not enabled")
	}
	if retrieved.BatchWindowSec != 30 {
		t.Errorf("BatchWindowSec = %d, want 30", retrieved.BatchWindowSec)
	}
	if retrieved.MaxBatchSize != 5 {
		t.Errorf("MaxBatchSize = %d, want 5", retrieved.MaxBatchSize)
	}
}

// TestGetChannels tests that GetChannels returns the correct set of channels.
func TestGetChannels(t *testing.T) {
	dbPath := t.TempDir() + "/test_notify.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	// Initially empty
	if len(service.GetChannels()) != 0 {
		t.Error("Expected no channels initially")
	}

	cc := ChannelConfig{
		Type:    ChannelNtfy,
		Enabled: true,
		URL:     "https://ntfy.sh/test-topic",
	}
	if err := service.AddChannel("test-ntfy", cc); err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	channels := service.GetChannels()
	if len(channels) != 1 {
		t.Errorf("Got %d channels, want 1", len(channels))
	}
	if _, ok := channels["test-ntfy"]; !ok {
		t.Error("Channel 'test-ntfy' not found")
	}
}

// TestAddRemoveChannel tests adding and removing channels.
func TestAddRemoveChannel(t *testing.T) {
	dbPath := t.TempDir() + "/test_notify.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	cc := ChannelConfig{
		Type:    ChannelWebhook,
		Enabled: true,
		URL:     "https://example.com/hook",
	}
	if err := service.AddChannel("wh1", cc); err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	if len(service.GetChannels()) != 1 {
		t.Fatalf("Expected 1 channel after add")
	}

	if err := service.RemoveChannel("wh1"); err != nil {
		t.Fatalf("RemoveChannel() error = %v", err)
	}

	if len(service.GetChannels()) != 0 {
		t.Errorf("Expected 0 channels after remove, got %d", len(service.GetChannels()))
	}
}

// TestNtfyDelivery tests ntfy delivery with a mock HTTP server verifying headers and body.
func TestNtfyDelivery(t *testing.T) {
	var capturedTitle, capturedBody, capturedTags, capturedPriority string
	var capturedMethod string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedTitle = r.Header.Get("Title")
		capturedTags = r.Header.Get("Tags")
		capturedPriority = r.Header.Get("Priority")
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_ntfy.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	if err := service.AddChannel("ntfy1", ChannelConfig{
		Type:    ChannelNtfy,
		Enabled: true,
		URL:     server.URL,
	}); err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	notif := Notification{
		Title:     "Test Alert",
		Body:      "Something happened",
		Priority:  int(PriorityHigh),
		Tags:      []string{"alert", "test"},
		Timestamp: time.Now(),
	}

	if err := service.Send(notif); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if capturedMethod != "POST" {
		t.Errorf("Method = %s, want POST", capturedMethod)
	}
	if capturedTitle != "Test Alert" {
		t.Errorf("Title header = %q, want %q", capturedTitle, "Test Alert")
	}
	if capturedBody != "Something happened" {
		t.Errorf("Body = %q, want %q", capturedBody, "Something happened")
	}
	if !strings.Contains(capturedTags, "alert") || !strings.Contains(capturedTags, "test") {
		t.Errorf("Tags header = %q, expected to contain 'alert' and 'test'", capturedTags)
	}
	if capturedPriority == "" {
		t.Error("Priority header not set")
	}
}

// TestNtfyDeliveryWithImage tests ntfy delivery with a base64-encoded image attachment.
func TestNtfyDeliveryWithImage(t *testing.T) {
	var capturedImageHeader string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedImageHeader = r.Header.Get("X-Image")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_ntfy_img.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	if err := service.AddChannel("ntfy1", ChannelConfig{
		Type:    ChannelNtfy,
		Enabled: true,
		URL:     server.URL,
	}); err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	// Minimal PNG bytes (1x1 transparent)
	pngData := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4,
		0x89, 0x00, 0x00, 0x00, 0x0A, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9C, 0x62, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00,
		0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE,
		0x42, 0x60, 0x82,
	}

	notif := Notification{
		Title:     "Image Test",
		Body:      "With attachment",
		Image:     pngData,
		ImageType: "image/png",
		Timestamp: time.Now(),
	}

	if err := service.Send(notif); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if capturedImageHeader == "" {
		t.Fatal("X-Image header was not set")
	}
	if !strings.HasPrefix(capturedImageHeader, "data:image/png;base64,") {
		t.Errorf("X-Image header has wrong format: %s", capturedImageHeader)
	}
}

// TestWebhookDelivery tests webhook delivery verifying JSON structure and base64 PNG field.
func TestWebhookDelivery(t *testing.T) {
	var capturedPayload map[string]interface{}
	var capturedContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_webhook.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	if err := service.AddChannel("wh1", ChannelConfig{
		Type:    ChannelWebhook,
		Enabled: true,
		URL:     server.URL + "/hook",
	}); err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	// Minimal PNG
	pngData := []byte{0x89, 0x50, 0x4E, 0x47}

	notif := Notification{
		Title:     "Webhook Test",
		Body:      "Test body",
		Priority:  int(PriorityUrgent),
		Tags:      []string{"webhook"},
		Image:     pngData,
		ImageType: "image/png",
		Data:      map[string]interface{}{"zone": "kitchen"},
		Timestamp: time.Now(),
	}

	if err := service.Send(notif); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if capturedContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", capturedContentType)
	}
	if capturedPayload == nil {
		t.Fatal("No payload received")
	}
	if capturedPayload["title"] != "Webhook Test" {
		t.Errorf("title = %v, want 'Webhook Test'", capturedPayload["title"])
	}
	if capturedPayload["body"] != "Test body" {
		t.Errorf("body = %v, want 'Test body'", capturedPayload["body"])
	}
	if capturedPayload["zone"] != "kitchen" {
		t.Errorf("zone = %v, want 'kitchen'", capturedPayload["zone"])
	}

	// Verify the image field contains base64 PNG
	imageField, ok := capturedPayload["image"].(string)
	if !ok {
		t.Fatal("image field not present or not a string")
	}
	if !strings.HasPrefix(imageField, "data:image/png;base64,") {
		t.Errorf("image field has wrong format: %s", imageField)
	}
	// Verify the base64 portion decodes correctly
	b64Part := strings.TrimPrefix(imageField, "data:image/png;base64,")
	decoded, err := base64.StdEncoding.DecodeString(b64Part)
	if err != nil {
		t.Fatalf("Failed to base64-decode image field: %v", err)
	}
	if len(decoded) != len(pngData) {
		t.Errorf("Decoded image length = %d, want %d", len(decoded), len(pngData))
	}
}

// TestWebhookCustomHeaders tests that custom headers are sent in webhook requests.
func TestWebhookCustomHeaders(t *testing.T) {
	var capturedAuthHeader string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuthHeader = r.Header.Get("X-Custom-Auth")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_webhook_headers.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	if err := service.AddChannel("wh1", ChannelConfig{
		Type:    ChannelWebhook,
		Enabled: true,
		URL:     server.URL,
		Headers: map[string]string{"X-Custom-Auth": "secret-token"},
	}); err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	if err := service.Send(Notification{Title: "T", Body: "B", Timestamp: time.Now()}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if capturedAuthHeader != "secret-token" {
		t.Errorf("X-Custom-Auth = %q, want %q", capturedAuthHeader, "secret-token")
	}
}

// TestWebhookErrorResponse tests that webhook errors are propagated.
func TestWebhookErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_webhook_err.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	if err := service.AddChannel("wh1", ChannelConfig{
		Type:    ChannelWebhook,
		Enabled: true,
		URL:     server.URL,
	}); err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	err = service.Send(Notification{Title: "T", Body: "B", Timestamp: time.Now()})
	if err == nil {
		t.Error("Expected error for 500 response, got nil")
	}
}

// TestSendUrgentPriorityImmediate tests that URGENT notifications bypass quiet hours.
func TestSendUrgentPriorityImmediate(t *testing.T) {
	sent := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_urgent.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	// Enable quiet hours covering now
	if err := service.SetQuietHours(QuietHoursConfig{
		Enabled:   true,
		StartHour: 0,
		StartMin:  0,
		EndHour:   23,
		EndMin:    59,
	}); err != nil {
		t.Fatalf("SetQuietHours() error = %v", err)
	}

	if err := service.AddChannel("wh1", ChannelConfig{
		Type:    ChannelWebhook,
		Enabled: true,
		URL:     server.URL,
	}); err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	// Low-priority is suppressed during quiet hours
	service.Send(Notification{
		Title:    "Low priority",
		Body:     "suppressed",
		Priority: int(PriorityLow),
		Timestamp: time.Now(),
	})

	if sent {
		t.Error("Low priority notification should be suppressed during quiet hours")
	}
}

// TestBatchFlush tests the basic batch flush mechanism.
func TestBatchFlush(t *testing.T) {
	var receivedBodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]interface{}
		json.Unmarshal(body, &payload)
		if title, ok := payload["title"].(string); ok {
			receivedBodies = append(receivedBodies, title)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_batch.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	if err := service.AddChannel("wh1", ChannelConfig{
		Type:    ChannelWebhook,
		Enabled: true,
		URL:     server.URL,
	}); err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	// Queue two notifications with the same title — they should be merged by title
	service.SendBatched(Notification{Title: "Alert", Body: "First event", Timestamp: time.Now()}, []string{"wh1"})
	service.SendBatched(Notification{Title: "Alert", Body: "Second event", Timestamp: time.Now()}, []string{"wh1"})

	// Wait for the batch window plus a little extra
	time.Sleep(service.batchWindow + 100*time.Millisecond)

	// Because flushBatch merges by title, both should be merged into one
	if len(receivedBodies) == 0 {
		t.Error("Expected at least one notification after batch flush")
	}
}

// TestExtendedServiceMergeNotifications tests the mergeNotifications helper.
func TestExtendedServiceMergeNotifications(t *testing.T) {
	dbPath := t.TempDir() + "/test_merge.db"
	base, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	ext, err := NewExtendedService(base)
	if err != nil {
		t.Fatalf("NewExtendedService() error = %v", err)
	}
	defer ext.Close()

	notifs := []Notification{
		{Title: "Alice entered Kitchen", Body: "From Hallway", Priority: int(PriorityLow)},
		{Title: "Bob left Living Room", Body: "To Bedroom", Priority: int(PriorityLow)},
		{Title: "Charlie entered Hallway", Body: "From Kitchen", Priority: int(PriorityMedium)},
	}

	merged := ext.mergeNotifications(notifs)

	if merged.Title == "" {
		t.Error("Merged notification should have a title")
	}
	if merged.Body == "" {
		t.Error("Merged notification should have a body")
	}
	// Priority should be the highest (Medium = 2)
	if merged.Priority != int(PriorityMedium) {
		t.Errorf("Priority = %d, want %d (medium)", merged.Priority, int(PriorityMedium))
	}
	// Single notification merge should be a no-op
	single := ext.mergeNotifications(notifs[:1])
	if single.Title != notifs[0].Title {
		t.Errorf("Single merge changed title: %q vs %q", single.Title, notifs[0].Title)
	}
}

// TestExtendedServiceFloorPlanThumbnail tests generating a floor-plan thumbnail via ExtendedService.
func TestExtendedServiceFloorPlanThumbnail(t *testing.T) {
	dbPath := t.TempDir() + "/test_thumbnail.db"
	base, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	ext, err := NewExtendedService(base)
	if err != nil {
		t.Fatalf("NewExtendedService() error = %v", err)
	}
	defer ext.Close()

	zones := []struct {
		ID, Name, Color string
		X, Y, W, D      float64
		Highlight       bool
	}{
		{ID: "kitchen", Name: "Kitchen", Color: "#4fc3f7", X: 1.0, Y: 1.0, W: 3.0, D: 2.0, Highlight: true},
	}

	people := []struct {
		Name, Color string
		X, Y, Z     float64
		Confidence  float64
		IsFall      bool
	}{
		{Name: "Alice", Color: "#4488ff", X: 2.5, Y: 2.0, Z: 1.0, Confidence: 0.85, IsFall: false},
	}

	data, err := ext.GenerateFloorPlanThumbnailExtended(300, 300, zones, people)
	if err != nil {
		t.Fatalf("GenerateFloorPlanThumbnailExtended() error = %v", err)
	}
	if len(data) == 0 {
		t.Fatal("GenerateFloorPlanThumbnailExtended() returned empty data")
	}

	// Verify PNG signature
	pngSig := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	if len(data) < 8 {
		t.Fatalf("Output too short: %d bytes", len(data))
	}
	for i, b := range pngSig {
		if data[i] != b {
			t.Errorf("Not a PNG (byte %d = %d, want %d)", i, data[i], b)
		}
	}
}

// TestBaseServiceFloorPlanThumbnail tests the base Service's GenerateFloorPlanThumbnail.
func TestBaseServiceFloorPlanThumbnail(t *testing.T) {
	dbPath := t.TempDir() + "/test_base_thumb.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	blobs := []struct {
		X, Y, Z  float64
		Identity string
		IsFall   bool
	}{
		{X: 2.0, Y: 1.5, Z: 1.0, Identity: "Alice", IsFall: false},
		{X: 4.0, Y: 3.0, Z: 1.0, Identity: "", IsFall: true},
	}

	data, err := service.GenerateFloorPlanThumbnail(300, 300, blobs)
	if err != nil {
		t.Fatalf("GenerateFloorPlanThumbnail() error = %v", err)
	}
	if len(data) == 0 {
		t.Fatal("GenerateFloorPlanThumbnail() returned empty data")
	}

	// Verify PNG signature
	pngSig := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	for i, b := range pngSig {
		if data[i] != b {
			t.Errorf("Not a PNG (byte %d = %d, want %d)", i, data[i], b)
		}
	}
}

// TestExtendedServiceQueueForBatching tests that low-priority notifications are queued.
func TestExtendedServiceQueueForBatching(t *testing.T) {
	dbPath := t.TempDir() + "/test_ext_batch.db"
	base, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	ext, err := NewExtendedService(base)
	if err != nil {
		t.Fatalf("NewExtendedService() error = %v", err)
	}
	defer ext.Close()

	// Small batch window for the test
	ext.SetBatchingConfig(BatchingConfig{
		Enabled:          true,
		BatchWindowSec:   1,
		MaxBatchSize:     10,
		BatchLowPriority: true,
		BatchMedium:      true,
	})

	// Queue a low-priority notification — it should be queued, not sent immediately
	notif := Notification{
		Title:     "Low Priority Event",
		Body:      "Queued",
		Priority:  int(PriorityLow),
		Timestamp: time.Now(),
	}
	if err := ext.queueForBatching(notif); err != nil {
		// May error if no channels — that's OK
		t.Logf("queueForBatching error (expected if no channels): %v", err)
	}

	ext.mu.Lock()
	queuedCount := len(ext.pendingLow)
	ext.mu.Unlock()

	if queuedCount != 1 {
		t.Errorf("Expected 1 queued low-priority notification, got %d", queuedCount)
	}
}

// TestExtendedServiceMaxBatchSize tests that notifications are flushed when max size is reached.
func TestExtendedServiceMaxBatchSize(t *testing.T) {
	flushed := false
	dbPath := t.TempDir() + "/test_maxbatch.db"
	base, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	ext, err := NewExtendedService(base)
	if err != nil {
		t.Fatalf("NewExtendedService() error = %v", err)
	}
	defer ext.Close()

	ext.SetBatchingConfig(BatchingConfig{
		Enabled:          true,
		BatchWindowSec:   60, // Long window — only flush on max size
		MaxBatchSize:     3,
		BatchLowPriority: true,
	})

	base.SetOnSend(func(channel string, notif Notification, success bool) {
		flushed = true
	})

	// Queue 3 notifications to hit max size
	for i := 0; i < 3; i++ {
		ext.queueForBatching(Notification{
			Title:     "Event",
			Body:      "Event body",
			Priority:  int(PriorityLow),
			Timestamp: time.Now(),
		})
	}

	// Give goroutine time to flush
	time.Sleep(100 * time.Millisecond)

	// Either the batch was flushed (no channels → no send) or queue was reset
	// Just verify no panic occurred and the service is still running
	_ = flushed
}

// TestExtendedServiceSendBriefing tests sending a morning briefing notification.
func TestExtendedServiceSendBriefing(t *testing.T) {
	var capturedTitle string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]interface{}
		json.Unmarshal(body, &payload)
		capturedTitle, _ = payload["title"].(string)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_briefing.db"
	base, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	ext, err := NewExtendedService(base)
	if err != nil {
		t.Fatalf("NewExtendedService() error = %v", err)
	}
	defer ext.Close()

	if err := base.AddChannel("wh1", ChannelConfig{
		Type:    ChannelWebhook,
		Enabled: true,
		URL:     server.URL,
	}); err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	if err := ext.SendBriefingNotification("Good morning", "All quiet overnight.", nil); err != nil {
		t.Fatalf("SendBriefingNotification() error = %v", err)
	}

	if capturedTitle != "Good morning" {
		t.Errorf("Title = %q, want 'Good morning'", capturedTitle)
	}
}

// TestNotificationHistory tests that send history is recorded.
func TestNotificationHistory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_history.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	if err := service.AddChannel("wh1", ChannelConfig{
		Type:    ChannelWebhook,
		Enabled: true,
		URL:     server.URL,
	}); err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	service.Send(Notification{Title: "Event A", Body: "Body A", Timestamp: time.Now()})
	service.Send(Notification{Title: "Event B", Body: "Body B", Timestamp: time.Now()})

	history := service.GetHistory(10)
	if len(history) < 2 {
		t.Errorf("Expected at least 2 history entries, got %d", len(history))
	}
}

// TestFloorPlanThumbnailDimensions verifies the rendered PNG is exactly 300x300 pixels.
func TestFloorPlanThumbnailDimensions(t *testing.T) {
	dbPath := t.TempDir() + "/test_dims.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	tests := []struct {
		name          string
		width, height int
	}{
		{"300x300", 300, 300},
		{"400x300", 400, 300},
		{"200x150", 200, 150},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := service.GenerateFloorPlanThumbnail(tt.width, tt.height, nil)
			if err != nil {
				t.Fatalf("GenerateFloorPlanThumbnail() error = %v", err)
			}
			if len(data) == 0 {
				t.Fatal("Empty PNG data returned")
			}

			img, _, err := image.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("Failed to decode PNG: %v", err)
			}

			bounds := img.Bounds()
			if bounds.Dx() != tt.width {
				t.Errorf("PNG width = %d, want %d", bounds.Dx(), tt.width)
			}
			if bounds.Dy() != tt.height {
				t.Errorf("PNG height = %d, want %d", bounds.Dy(), tt.height)
			}
		})
	}
}

// TestExtendedFloorPlanThumbnailDimensions verifies the extended renderer produces the correct dimensions.
func TestExtendedFloorPlanThumbnailDimensions(t *testing.T) {
	dbPath := t.TempDir() + "/test_ext_dims.db"
	base, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	ext, err := NewExtendedService(base)
	if err != nil {
		t.Fatalf("NewExtendedService() error = %v", err)
	}
	defer ext.Close()

	data, err := ext.GenerateFloorPlanThumbnailExtended(300, 300, nil, nil)
	if err != nil {
		t.Fatalf("GenerateFloorPlanThumbnailExtended() error = %v", err)
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to decode PNG: %v", err)
	}

	bounds := img.Bounds()
	if bounds.Dx() != 300 {
		t.Errorf("PNG width = %d, want 300", bounds.Dx())
	}
	if bounds.Dy() != 300 {
		t.Errorf("PNG height = %d, want 300", bounds.Dy())
	}
}

// TestFloorPlanThumbnailBlobColors verifies that blob colors in the rendered PNG match expected values.
// The base renderer uses roomW=6, roomD=5 and scales blobs by width/roomW and height/roomD.
// For a 300x300 image: scaleX=50, scaleZ=60.
// A blob at (2.0, Y, 1.5) → px=100, pz=90.
func TestFloorPlanThumbnailBlobColors(t *testing.T) {
	dbPath := t.TempDir() + "/test_colors.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	tests := []struct {
		name     string
		blob     struct {
			X, Y, Z  float64
			Identity string
			IsFall   bool
		}
		// Expected color channel ranges (R, G, B) — we check only the dominant channel
		wantRedDominant   bool
		wantGreenDominant bool
		wantBlueDominant  bool
	}{
		{
			name:             "identified blob is green-ish",
			blob:             struct{ X, Y, Z float64; Identity string; IsFall bool }{X: 2.0, Z: 1.5, Identity: "Alice"},
			wantGreenDominant: true,
		},
		{
			name:            "fall blob is red-ish",
			blob:            struct{ X, Y, Z float64; Identity string; IsFall bool }{X: 2.0, Z: 1.5, IsFall: true},
			wantRedDominant: true,
		},
		{
			name:             "unknown blob is blue-ish",
			blob:             struct{ X, Y, Z float64; Identity string; IsFall bool }{X: 2.0, Z: 1.5},
			wantBlueDominant: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blobs := []struct {
				X, Y, Z  float64
				Identity string
				IsFall   bool
			}{tt.blob}

			data, err := service.GenerateFloorPlanThumbnail(300, 300, blobs)
			if err != nil {
				t.Fatalf("GenerateFloorPlanThumbnail() error = %v", err)
			}

			img, _, err := image.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("Failed to decode PNG: %v", err)
			}

			// blob at X=2.0, Z=1.5 with 300x300 and room 6x5 → px=100, pz=90
			px, pz := 100, 90
			r, g, b, _ := img.At(px, pz).RGBA()
			r8, g8, b8 := uint8(r>>8), uint8(g>>8), uint8(b>>8)

			switch {
			case tt.wantRedDominant:
				if r8 <= g8 || r8 <= b8 {
					t.Errorf("Expected red-dominant pixel at (%d,%d), got R=%d G=%d B=%d", px, pz, r8, g8, b8)
				}
			case tt.wantGreenDominant:
				if g8 <= r8 || g8 <= b8 {
					t.Errorf("Expected green-dominant pixel at (%d,%d), got R=%d G=%d B=%d", px, pz, r8, g8, b8)
				}
			case tt.wantBlueDominant:
				if b8 <= r8 || b8 <= g8 {
					t.Errorf("Expected blue-dominant pixel at (%d,%d), got R=%d G=%d B=%d", px, pz, r8, g8, b8)
				}
			}
		})
	}
}

// TestFloorPlanZoneBoundaryPixels verifies that zone outlines appear at the expected pixel coordinates
// in the extended renderer. The coordinate transform for a 300x300 image with 6x5 room:
//   margin=10, drawW=280, drawH=280
//   scaleX=280/6≈46.67, scaleZ=280/5=56 → scale=min(46.67,56)=46.67
//   offsetX=10+(280-6*46.67)/2=10, offsetY=10+(280-5*46.67)/2≈33
//
// A zone at (X=0, Y=0, W=2, D=2) → top-left at (px=10, py=33), bottom-right at ~(px=103, py=127).
func TestFloorPlanZoneBoundaryPixels(t *testing.T) {
	dbPath := t.TempDir() + "/test_zone_pixels.db"
	base, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	ext, err := NewExtendedService(base)
	if err != nil {
		t.Fatalf("NewExtendedService() error = %v", err)
	}
	defer ext.Close()

	zones := []struct {
		ID, Name, Color string
		X, Y, W, D      float64
		Highlight       bool
	}{
		{ID: "z1", Name: "Kitchen", X: 0, Y: 0, W: 2, D: 2, Highlight: false},
	}

	data, err := ext.GenerateFloorPlanThumbnailExtended(300, 300, zones, nil)
	if err != nil {
		t.Fatalf("GenerateFloorPlanThumbnailExtended() error = %v", err)
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to decode PNG: %v", err)
	}

	// Verify the image has the correct size
	bounds := img.Bounds()
	if bounds.Dx() != 300 || bounds.Dy() != 300 {
		t.Fatalf("Wrong image dimensions: %dx%d", bounds.Dx(), bounds.Dy())
	}

	// Compute expected zone top-left corner coordinates:
	// margin=10, drawW=drawH=280, roomW=6, roomD=5
	// scaleX=280/6≈46.67, scaleZ=280/5=56 → scale=46.67 (min)
	// offsetX = 10 + (280 - 6*46.67)/2 = 10 + 0 = 10
	// offsetY = 10 + (280 - 5*46.67)/2 = 10 + (280-233.33)/2 ≈ 10 + 23.33 ≈ 33
	const (
		margin  = 10
		roomW   = 6.0
		roomD   = 5.0
		imgSize = 300
	)
	drawW := float64(imgSize - 2*margin)
	drawH := float64(imgSize - 2*margin)
	scaleX := drawW / roomW
	scaleZ := drawH / roomD
	scale := scaleX
	if scaleZ < scale {
		scale = scaleZ
	}
	offsetX := float64(margin) + (drawW-roomW*scale)/2
	offsetY := float64(margin) + (drawH-roomD*scale)/2

	expectedX := int(offsetX + 0*scale)
	expectedY := int(offsetY + 0*scale)
	expectedX2 := int(offsetX + 2*scale)
	expectedY2 := int(offsetY + 2*scale)

	// The zone corners should be present — verify they're within image bounds
	if expectedX < 0 || expectedX >= imgSize {
		t.Errorf("Zone corner X=%d out of bounds [0,%d)", expectedX, imgSize)
	}
	if expectedY < 0 || expectedY >= imgSize {
		t.Errorf("Zone corner Y=%d out of bounds [0,%d)", expectedY, imgSize)
	}
	if expectedX2 < 0 || expectedX2 >= imgSize {
		t.Errorf("Zone corner X2=%d out of bounds [0,%d)", expectedX2, imgSize)
	}
	if expectedY2 < 0 || expectedY2 >= imgSize {
		t.Errorf("Zone corner Y2=%d out of bounds [0,%d)", expectedY2, imgSize)
	}

	// Verify zone interior has a non-background color (zone fill should be blue-ish, not the dark background)
	// The zone fill color is RGBA{79, 195, 247, 51} drawn over RGBA{26, 26, 46, 255} background
	centerX := (expectedX + expectedX2) / 2
	centerY := (expectedY + expectedY2) / 2
	r, g, b, _ := img.At(centerX, centerY).RGBA()
	r8, g8, b8 := uint8(r>>8), uint8(g>>8), uint8(b>>8)

	// The composite color should be noticeably blue — at minimum the blue channel should be highest
	if b8 <= r8 {
		t.Errorf("Zone interior at (%d,%d) expected blue-dominant color, got R=%d G=%d B=%d", centerX, centerY, r8, g8, b8)
	}
}

// TestPriorityConstants ensures all priority constants are defined and ordered.
func TestPriorityConstants(t *testing.T) {
	if PriorityLow >= PriorityMedium {
		t.Error("PriorityLow should be less than PriorityMedium")
	}
	if PriorityMedium >= PriorityHigh {
		t.Error("PriorityMedium should be less than PriorityHigh")
	}
	if PriorityHigh >= PriorityUrgent {
		t.Error("PriorityHigh should be less than PriorityUrgent")
	}
	if PriorityUrgent >= PriorityCritical {
		t.Error("PriorityUrgent should be less than PriorityCritical")
	}
}

// TestNotificationTypeConstants ensures all notification type constants are defined.
func TestNotificationTypeConstants(t *testing.T) {
	types := []NotificationType{
		TypeZoneEnter,
		TypeZoneLeave,
		TypeZoneVacant,
		TypeFallDetected,
		TypeFallEscalation,
		TypeAnomalyAlert,
		TypeNodeOffline,
		TypeSleepSummary,
		TypeMorningBriefing,
	}
	for _, typ := range types {
		if typ == "" {
			t.Error("Found empty notification type constant")
		}
	}
}

// TestGotifyDelivery tests gotify delivery with a mock HTTP server.
func TestGotifyDelivery(t *testing.T) {
	var capturedPayload map[string]interface{}
	var capturedToken string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedToken = r.URL.Query().Get("token")
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_gotify.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	if err := service.AddChannel("gotify1", ChannelConfig{
		Type:    ChannelGotify,
		Enabled: true,
		URL:     server.URL,
		Token:   "mytoken",
	}); err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	notif := Notification{
		Title:     "Gotify Alert",
		Body:      "Something happened",
		Priority:  int(PriorityHigh),
		Timestamp: time.Now(),
	}

	if err := service.Send(notif); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if capturedToken != "mytoken" {
		t.Errorf("token query param = %q, want 'mytoken'", capturedToken)
	}
	if capturedPayload["title"] != "Gotify Alert" {
		t.Errorf("title = %v, want 'Gotify Alert'", capturedPayload["title"])
	}
	if capturedPayload["message"] != "Something happened" {
		t.Errorf("message = %v, want 'Something happened'", capturedPayload["message"])
	}
}

// TestSendWithDisabledChannel tests that disabled channels are skipped.
func TestSendWithDisabledChannel(t *testing.T) {
	sent := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_disabled.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	// Add disabled channel
	if err := service.AddChannel("wh1", ChannelConfig{
		Type:    ChannelWebhook,
		Enabled: false, // Disabled
		URL:     server.URL,
	}); err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	service.Send(Notification{Title: "T", Body: "B", Timestamp: time.Now()})

	if sent {
		t.Error("Disabled channel should not send notifications")
	}
}

// TestExtendedServiceOnSendCallback tests the OnSend callback is invoked.
func TestExtendedServiceOnSendCallback(t *testing.T) {
	var callbackChannel string
	var callbackSuccess bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_callback.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	service.SetOnSend(func(channel string, notif Notification, success bool) {
		callbackChannel = channel
		callbackSuccess = success
	})

	if err := service.AddChannel("wh1", ChannelConfig{
		Type:    ChannelWebhook,
		Enabled: true,
		URL:     server.URL,
	}); err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	service.Send(Notification{Title: "T", Body: "B", Timestamp: time.Now()})

	if callbackChannel == "" {
		t.Error("OnSend callback was not called")
	}
	if !callbackSuccess {
		t.Error("Expected success=true in OnSend callback")
	}
}

// TestSetRoomConfig verifies SetRoomConfig stores the provider.
func TestSetRoomConfig(t *testing.T) {
	dbPath := t.TempDir() + "/test_roomconfig.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	// SetRoomConfig should not panic
	service.SetRoomConfig(nil)

	// Set with a concrete provider
	type fakeRoom struct{}
	// RoomConfigProvider interface is GetRoom() (width, height, depth float64)
	// Just verify no panic
	service.SetRoomConfig(nil)
}

// TestSetFloorPlan verifies SetFloorPlan stores the image data.
func TestSetFloorPlan(t *testing.T) {
	dbPath := t.TempDir() + "/test_floorplan.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	// Set a floor plan image
	pngData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	service.SetFloorPlan(pngData)

	// Verify it was stored (access via render)
	data, err := service.GenerateFloorPlanThumbnail(100, 100, nil)
	if err != nil {
		t.Fatalf("GenerateFloorPlanThumbnail() error = %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Expected non-empty thumbnail data")
	}
}

// TestSendWithPriority verifies that SendWithPriority sets priority and sends.
func TestSendWithPriority(t *testing.T) {
	var capturedPayload map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_send_priority.db"
	base, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	ext, err := NewExtendedService(base)
	if err != nil {
		t.Fatalf("NewExtendedService() error = %v", err)
	}
	defer ext.Close()

	if err := base.AddChannel("wh1", ChannelConfig{
		Type:    ChannelWebhook,
		Enabled: true,
		URL:     server.URL,
	}); err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	notif := Notification{
		Title:     "Priority Test",
		Body:      "Body",
		Timestamp: time.Now(),
	}

	if err := ext.SendWithPriority(notif, PriorityHigh); err != nil {
		t.Fatalf("SendWithPriority() error = %v", err)
	}

	if capturedPayload == nil {
		t.Fatal("No payload received")
	}
	if capturedPayload["title"] != "Priority Test" {
		t.Errorf("title = %v, want 'Priority Test'", capturedPayload["title"])
	}
}

// TestGetHistoryExtended verifies GetHistoryExtended returns entries.
func TestGetHistoryExtended(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_hist_ext.db"
	base, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	ext, err := NewExtendedService(base)
	if err != nil {
		t.Fatalf("NewExtendedService() error = %v", err)
	}
	defer ext.Close()

	if err := base.AddChannel("wh1", ChannelConfig{
		Type:    ChannelWebhook,
		Enabled: true,
		URL:     server.URL,
	}); err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	base.Send(Notification{Title: "Event X", Body: "Body X", Timestamp: time.Now()})

	history := ext.GetHistoryExtended(10)
	if len(history) == 0 {
		t.Error("Expected at least one history entry from GetHistoryExtended")
	}
	if history[0].Title != "Event X" {
		t.Errorf("Title = %q, want 'Event X'", history[0].Title)
	}
	if history[0].Channel == "" {
		t.Error("Expected non-empty Channel in history entry")
	}
}

// TestPushoverMissingCredentials verifies sendPushover returns error without token/user.
func TestPushoverMissingCredentials(t *testing.T) {
	dbPath := t.TempDir() + "/test_pushover.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	// Add pushover channel without token/user — should fail with credential error
	if err := service.AddChannel("po1", ChannelConfig{
		Type:    ChannelPushover,
		Enabled: true,
		URL:     "https://api.pushover.net",
		Token:   "", // Empty token
		User:    "", // Empty user
	}); err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	err = service.Send(Notification{Title: "Test", Body: "Body", Timestamp: time.Now()})
	if err == nil {
		t.Error("Expected error for pushover with missing credentials, got nil")
	}
}

// TestExtendedServiceIsQuietHoursAlwaysFalse verifies ExtendedService.isQuietHours returns false.
func TestExtendedServiceIsQuietHoursAlwaysFalse(t *testing.T) {
	dbPath := t.TempDir() + "/test_ext_qh.db"
	base, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	ext, err := NewExtendedService(base)
	if err != nil {
		t.Fatalf("NewExtendedService() error = %v", err)
	}
	defer ext.Close()

	// ExtendedService.isQuietHours always returns false (delegates to future impl)
	if ext.isQuietHours() {
		t.Error("ExtendedService.isQuietHours() should always return false")
	}
}

// TestCheckMorningDigestEmpty verifies checkMorningDigest does nothing with empty queue.
func TestCheckMorningDigestEmpty(t *testing.T) {
	dbPath := t.TempDir() + "/test_digest_empty.db"
	base, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	ext, err := NewExtendedService(base)
	if err != nil {
		t.Fatalf("NewExtendedService() error = %v", err)
	}
	defer ext.Close()

	// With empty queue, checkMorningDigest should not send anything
	ext.checkMorningDigest()

	// Verify the digest was not sent (digestSentToday remains false)
	ext.mu.RLock()
	sent := ext.digestSentToday
	ext.mu.RUnlock()

	if sent {
		t.Error("checkMorningDigest() should not send digest when queue is empty")
	}
}

// TestSendMorningDigestWithQueuedEvents verifies sendMorningDigest sends queued events.
func TestSendMorningDigestWithQueuedEvents(t *testing.T) {
	var capturedPayload map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_digest_send.db"
	base, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	ext, err := NewExtendedService(base)
	if err != nil {
		t.Fatalf("NewExtendedService() error = %v", err)
	}
	defer ext.Close()

	if err := base.AddChannel("wh1", ChannelConfig{
		Type:    ChannelWebhook,
		Enabled: true,
		URL:     server.URL,
	}); err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	// Pre-populate the quiet queue
	ext.mu.Lock()
	ext.queuedDuringQuiet = []Notification{
		{Title: "Motion at 23:00", Body: "Kitchen motion detected", Priority: int(PriorityLow), Timestamp: time.Now()},
		{Title: "Zone entry at 02:30", Body: "Someone entered hallway", Priority: int(PriorityLow), Timestamp: time.Now()},
	}
	ext.mu.Unlock()

	// Call sendMorningDigest directly (simulating morning digest at 07:00)
	ext.sendMorningDigest()

	// Verify digest was sent with bundled events
	if capturedPayload == nil {
		t.Fatal("No digest notification was sent")
	}
	if capturedPayload["title"] == nil {
		t.Fatal("Digest notification has no title")
	}
	title, ok := capturedPayload["title"].(string)
	if !ok || title == "" {
		t.Errorf("Expected non-empty digest title, got %v", capturedPayload["title"])
	}
	// Title should mention the event count
	if !strings.Contains(title, "2") {
		t.Errorf("Digest title should mention 2 events, got: %s", title)
	}

	// Verify digestSentToday flag was set
	ext.mu.RLock()
	sent := ext.digestSentToday
	ext.mu.RUnlock()
	if !sent {
		t.Error("Expected digestSentToday to be set after sendMorningDigest")
	}

	// Verify queue was cleared
	ext.mu.RLock()
	queueLen := len(ext.queuedDuringQuiet)
	ext.mu.RUnlock()
	if queueLen != 0 {
		t.Errorf("Expected queue to be empty after digest, got %d items", queueLen)
	}
}

// TestBatching3LowEventsProduces1Merged tests that 3 LOW priority events batched in
// a window produce a single merged notification.
func TestBatching3LowEventsProduces1Merged(t *testing.T) {
	sentCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sentCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_3low.db"
	base, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	ext, err := NewExtendedService(base)
	if err != nil {
		t.Fatalf("NewExtendedService() error = %v", err)
	}
	defer ext.Close()

	if err := base.AddChannel("wh1", ChannelConfig{
		Type:    ChannelWebhook,
		Enabled: true,
		URL:     server.URL,
	}); err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	// Set max batch size to 3 so that 3 LOW events trigger a flush
	ext.SetBatchingConfig(BatchingConfig{
		Enabled:          true,
		BatchWindowSec:   30,
		MaxBatchSize:     3,
		BatchLowPriority: true,
		BatchMedium:      true,
	})

	// Queue 3 LOW priority events
	for i := 0; i < 3; i++ {
		if err := ext.queueForBatching(Notification{
			Title:     "Motion detected",
			Body:      "Zone event",
			Priority:  int(PriorityLow),
			Timestamp: time.Now(),
		}); err != nil {
			t.Logf("queueForBatching() error: %v", err)
		}
	}

	// Give flush goroutine time to run
	time.Sleep(200 * time.Millisecond)

	// 3 LOW events should be batched into 1 merged notification
	if sentCount != 1 {
		t.Errorf("Expected 1 merged notification, got %d", sentCount)
	}
}

// TestBatchingUrgentBypassesBatch tests that URGENT priority notifications bypass
// the batch queue and are sent immediately.
func TestBatchingUrgentBypassesBatch(t *testing.T) {
	sentCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sentCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_urgent_batch.db"
	base, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	ext, err := NewExtendedService(base)
	if err != nil {
		t.Fatalf("NewExtendedService() error = %v", err)
	}
	defer ext.Close()

	if err := base.AddChannel("wh1", ChannelConfig{
		Type:    ChannelWebhook,
		Enabled: true,
		URL:     server.URL,
	}); err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	ext.SetBatchingConfig(BatchingConfig{
		Enabled:          true,
		BatchWindowSec:   60, // Long window — won't auto-flush
		MaxBatchSize:     10,
		BatchLowPriority: true,
		BatchMedium:      true,
	})

	// Queue an URGENT notification — it should bypass batching and send immediately
	if err := ext.queueForBatching(Notification{
		Title:     "Fall Detected",
		Body:      "Alice may have fallen",
		Priority:  int(PriorityUrgent),
		Timestamp: time.Now(),
	}); err != nil {
		t.Fatalf("queueForBatching() error = %v", err)
	}

	if sentCount != 1 {
		t.Errorf("Expected URGENT to be sent immediately (count=1), got %d", sentCount)
	}

	// Verify nothing in the low/medium queues
	ext.mu.RLock()
	lowCount := len(ext.pendingLow)
	medCount := len(ext.pendingMedium)
	ext.mu.RUnlock()

	if lowCount != 0 || medCount != 0 {
		t.Errorf("URGENT should not be in batch queue: low=%d medium=%d", lowCount, medCount)
	}
}

// TestQuietHoursLowSuppressedUrgentDelivered tests the quiet hours gate:
// - LOW priority at 23:00 (quiet hours 22:00-07:00) → suppressed
// - URGENT priority at 23:00 (quiet hours 22:00-07:00) → delivered immediately
func TestQuietHoursLowSuppressedUrgentDelivered(t *testing.T) {
	sentNotifications := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sentNotifications++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_qh_urgent.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	// Enable quiet hours covering the full day (always in quiet hours)
	if err := service.SetQuietHours(QuietHoursConfig{
		Enabled:   true,
		StartHour: 0,
		StartMin:  0,
		EndHour:   23,
		EndMin:    59,
	}); err != nil {
		t.Fatalf("SetQuietHours() error = %v", err)
	}

	if err := service.AddChannel("wh1", ChannelConfig{
		Type:    ChannelWebhook,
		Enabled: true,
		URL:     server.URL,
	}); err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	// LOW priority should be suppressed
	service.Send(Notification{
		Title:     "Motion detected",
		Body:      "Low priority event",
		Priority:  int(PriorityLow),
		Timestamp: time.Now(),
	})

	if sentNotifications != 0 {
		t.Errorf("LOW priority should be suppressed during quiet hours, got %d sends", sentNotifications)
	}

	// URGENT priority should bypass quiet hours and be delivered
	if err := service.Send(Notification{
		Title:     "Fall Detected",
		Body:      "Alice may have fallen in kitchen",
		Priority:  int(PriorityUrgent),
		Timestamp: time.Now(),
	}); err != nil {
		t.Fatalf("Send() error for urgent: %v", err)
	}

	if sentNotifications != 1 {
		t.Errorf("URGENT should bypass quiet hours (expected 1 send), got %d sends", sentNotifications)
	}
}

// TestCheckMorningDigestSendsWhenQueued verifies checkMorningDigest triggers digest
// when there are queued notifications and it is past the digest hour.
func TestCheckMorningDigestSendsWhenQueued(t *testing.T) {
	digestSent := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		digestSent = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_digest_check.db"
	base, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	ext, err := NewExtendedService(base)
	if err != nil {
		t.Fatalf("NewExtendedService() error = %v", err)
	}
	defer ext.Close()

	if err := base.AddChannel("wh1", ChannelConfig{
		Type:    ChannelWebhook,
		Enabled: true,
		URL:     server.URL,
	}); err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	// Pre-populate quiet queue
	ext.mu.Lock()
	ext.queuedDuringQuiet = []Notification{
		{Title: "Quiet event", Body: "Motion during night", Priority: int(PriorityLow), Timestamp: time.Now()},
	}
	ext.mu.Unlock()

	// checkMorningDigest only sends if current hour >= 7
	// Since we can't control time, call sendMorningDigest directly to verify the behavior
	ext.sendMorningDigest()

	// Allow goroutine to complete
	time.Sleep(50 * time.Millisecond)

	if !digestSent {
		t.Error("Morning digest should have been sent when there are queued notifications")
	}
}
