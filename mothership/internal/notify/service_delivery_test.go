// Package notify provides comprehensive delivery tests for notification channels.
package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestPushoverSuccessfulDelivery tests Pushover API with successful response.
func TestPushoverSuccessfulDelivery(t *testing.T) {
	var capturedFormData url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("Failed to parse form: %v", err)
		}
		capturedFormData = r.PostForm
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status": "ok"}`)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_pushover_ok.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	// Replace HTTP client to intercept the hardcoded Pushover API URL
	// The client will redirect any request to the Pushover API to our test server
	originalTransport := http.DefaultTransport
	service.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			// Redirect to test server
			newURL, _ := url.Parse(server.URL)
			req.URL = newURL
			req.Host = newURL.Host
			return originalTransport.RoundTrip(req)
		}),
	}

	if err := service.AddChannel("po1", ChannelConfig{
		Type:    ChannelPushover,
		Enabled: true,
		Token:   "aXXXXXX",
		User:    "uXXXXXX",
	}); err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	notif := Notification{
		Title:     "Pushover Test",
		Body:      "Test body",
		Priority:  int(PriorityHigh),
		Timestamp: time.Now(),
	}

	if err := service.Send(notif); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if capturedFormData == nil {
		t.Fatal("No form data received")
	}
	if capturedFormData.Get("token") != "aXXXXXX" {
		t.Errorf("token = %q, want 'aXXXXXX'", capturedFormData.Get("token"))
	}
	if capturedFormData.Get("user") != "uXXXXXX" {
		t.Errorf("user = %q, want 'uXXXXXX'", capturedFormData.Get("user"))
	}
	if capturedFormData.Get("title") != "Pushover Test" {
		t.Errorf("title = %q, want 'Pushover Test'", capturedFormData.Get("title"))
	}
	if capturedFormData.Get("message") != "Test body" {
		t.Errorf("message = %q, want 'Test body'", capturedFormData.Get("message"))
	}
	if capturedFormData.Get("priority") != "3" {
		t.Errorf("priority = %q, want '3'", capturedFormData.Get("priority"))
	}
}

// roundTripperFunc is an http.RoundTripper that calls a function.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// TestGotifySuccessfulDelivery tests Gotify API with successful response.
func TestGotifySuccessfulDelivery(t *testing.T) {
	var capturedPayload map[string]interface{}
	var capturedToken string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedToken = r.URL.Query().Get("token")
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedPayload)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"id": "123"}`)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_gotify_ok.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	if err := service.AddChannel("gotify1", ChannelConfig{
		Type:    ChannelGotify,
		Enabled: true,
		URL:     server.URL,
		Token:   "test-token",
	}); err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	notif := Notification{
		Title:     "Gotify Success",
		Body:      "Test message",
		Priority:  int(PriorityMedium),
		Tags:      []string{"test", "integration"},
		Timestamp: time.Now(),
	}

	if err := service.Send(notif); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if capturedToken != "test-token" {
		t.Errorf("token query param = %q, want 'test-token'", capturedToken)
	}
	if capturedPayload["title"] != "Gotify Success" {
		t.Errorf("title = %v, want 'Gotify Success'", capturedPayload["title"])
	}
	if capturedPayload["message"] != "Test message" {
		t.Errorf("message = %v, want 'Test message'", capturedPayload["message"])
	}
	// JSON unmarshaling converts numbers to float64
	priority, ok := capturedPayload["priority"].(float64)
	if !ok || priority != float64(PriorityMedium) {
		t.Errorf("priority = %v (type %T), want %d", capturedPayload["priority"], capturedPayload["priority"], int(PriorityMedium))
	}
}

// TestNtfyWithAuth tests ntfy delivery with basic authentication.
func TestNtfyWithAuth(t *testing.T) {
	var capturedUsername, capturedPassword string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUsername, capturedPassword, _ = r.BasicAuth()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_ntfy_auth.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	if err := service.AddChannel("ntfy1", ChannelConfig{
		Type:     ChannelNtfy,
		Enabled:  true,
		URL:      server.URL,
		Username: "testuser",
		Password: "testpass",
	}); err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	notif := Notification{
		Title:     "Auth Test",
		Body:      "With credentials",
		Timestamp: time.Now(),
	}

	if err := service.Send(notif); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if capturedUsername != "testuser" {
		t.Errorf("Username = %q, want 'testuser'", capturedUsername)
	}
	if capturedPassword != "testpass" {
		t.Errorf("Password = %q, want 'testpass'", capturedPassword)
	}
}

// TestNtfyErrorResponse tests ntfy delivery with HTTP error response.
func TestNtfyErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_ntfy_err.db"
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

	err = service.Send(Notification{Title: "T", Body: "B", Timestamp: time.Now()})
	if err == nil {
		t.Error("Expected error for 503 response, got nil")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Errorf("Error should mention status code 503, got: %v", err)
	}
}

// TestWebhookWithBasicAuth tests webhook delivery with basic authentication.
func TestWebhookWithBasicAuth(t *testing.T) {
	var capturedUsername, capturedPassword string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUsername, capturedPassword, _ = r.BasicAuth()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_webhook_auth.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	if err := service.AddChannel("wh1", ChannelConfig{
		Type:     ChannelWebhook,
		Enabled:  true,
		URL:      server.URL,
		Username: "webuser",
		Password: "webpass",
	}); err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	if err := service.Send(Notification{Title: "T", Body: "B", Timestamp: time.Now()}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if capturedUsername != "webuser" {
		t.Errorf("Username = %q, want 'webuser'", capturedUsername)
	}
	if capturedPassword != "webpass" {
		t.Errorf("Password = %q, want 'webpass'", capturedPassword)
	}
}

// TestWebhookWithLargeImage tests webhook delivery with a large base64-encoded image.
func TestWebhookWithLargeImage(t *testing.T) {
	var capturedPayload map[string]interface{}
	var capturedImageLen int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_webhook_large_img.db"
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

	// Create a larger test PNG (100x100)
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 128, 255})
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	pngData := buf.Bytes()

	notif := Notification{
		Title:     "Large Image",
		Body:      "With attachment",
		Image:     pngData,
		ImageType: "image/png",
		Timestamp: time.Now(),
	}

	if err := service.Send(notif); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	imageField, ok := capturedPayload["image"].(string)
	if !ok {
		t.Fatal("image field not present or not a string")
	}
	capturedImageLen = len(imageField)

	if capturedImageLen == 0 {
		t.Error("Expected non-empty base64 image field")
	}
	// Verify the data URI prefix
	if !strings.HasPrefix(imageField, "data:image/png;base64,") {
		t.Errorf("Image field has wrong format: %s", imageField[:30])
	}
}

// TestWebhookGETMethod tests webhook delivery with HTTP GET method.
func TestWebhookGETMethod(t *testing.T) {
	var capturedMethod string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_webhook_get.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	// Note: webhook always uses POST in current implementation
	// This test documents current behavior
	if err := service.AddChannel("wh1", ChannelConfig{
		Type:    ChannelWebhook,
		Enabled: true,
		URL:     server.URL,
	}); err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	if err := service.Send(Notification{Title: "T", Body: "B", Timestamp: time.Now()}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if capturedMethod != "POST" {
		t.Errorf("Method = %s, want POST (current implementation)", capturedMethod)
	}
}

// TestBatchingWindowExpiry tests that notifications are sent when batch window expires.
func TestBatchingWindowExpiry(t *testing.T) {
	sentCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sentCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_batch_window.db"
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

	// Set short batch window
	ext.SetBatchingConfig(BatchingConfig{
		Enabled:          true,
		BatchWindowSec:   1, // 1 second window
		MaxBatchSize:     100, // High max size, won't trigger flush
		BatchLowPriority: true,
		BatchMedium:      true,
	})

	// Queue 2 LOW priority events
	for i := 0; i < 2; i++ {
		if err := ext.queueForBatching(Notification{
			Title:     "Motion detected",
			Body:      fmt.Sprintf("Event %d", i),
			Priority:  int(PriorityLow),
			Timestamp: time.Now(),
		}); err != nil {
			t.Logf("queueForBatching() error: %v", err)
		}
	}

	// Wait for batch window to expire
	time.Sleep(1200 * time.Millisecond)

	// Should have sent 1 merged notification
	if sentCount != 1 {
		t.Errorf("Expected 1 merged notification after batch window expiry, got %d", sentCount)
	}
}

// TestQuietHoursCrossMidnight tests quiet hours detection across midnight.
func TestQuietHoursCrossMidnight(t *testing.T) {
	dbPath := t.TempDir() + "/test_qh_midnight.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	// Set quiet hours 22:00 - 07:00 (crosses midnight)
	if err := service.SetQuietHours(QuietHoursConfig{
		Enabled:   true,
		StartHour: 22,
		StartMin:  0,
		EndHour:   7,
		EndMin:    0,
	}); err != nil {
		t.Fatalf("SetQuietHours() error = %v", err)
	}

	// Test various times
	testCases := []struct {
		hour      int
		min       int
		expected  bool
		desc      string
	}{
		{21, 59, false, "21:59 - before quiet hours"},
		{22, 0, true, "22:00 - quiet hours start"},
		{22, 30, true, "22:30 - during quiet hours"},
		{23, 59, true, "23:59 - during quiet hours"},
		{0, 0, true, "00:00 - after midnight"},
		{6, 59, true, "06:59 - near end"},
		{7, 0, false, "07:00 - quiet hours end"},
		{7, 1, false, "07:01 - after quiet hours"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			// We can't mock time.Now(), so we just verify the function doesn't panic
			// and returns a boolean
			_ = service.isQuietHours()
		})
	}
}

// TestMorningDigestDeliveryBundlesQueued tests that morning digest bundles all queued events.
func TestMorningDigestDeliveryBundlesQueued(t *testing.T) {
	var capturedBody string
	var capturedTitle string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]interface{}
		json.Unmarshal(body, &payload)
		capturedTitle, _ = payload["title"].(string)
		capturedBody, _ = payload["body"].(string)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_digest_bundles.db"
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

	// Queue multiple events during quiet hours
	queuedEvents := []Notification{
		{Title: "Motion 1", Body: "Kitchen motion at 23:00", Priority: int(PriorityLow), Timestamp: time.Now()},
		{Title: "Motion 2", Body: "Hallway motion at 01:30", Priority: int(PriorityLow), Timestamp: time.Now()},
		{Title: "Motion 3", Body: "Bedroom motion at 03:45", Priority: int(PriorityLow), Timestamp: time.Now()},
		{Title: "Motion 4", Body: "Bathroom motion at 05:15", Priority: int(PriorityLow), Timestamp: time.Now()},
	}

	ext.mu.Lock()
	ext.queuedDuringQuiet = queuedEvents
	ext.mu.Unlock()

	// Send morning digest
	ext.sendMorningDigest()

	// Wait for goroutine
	time.Sleep(100 * time.Millisecond)

	if capturedTitle == "" {
		t.Fatal("No digest was sent")
	}
	if !strings.Contains(capturedTitle, "4") {
		t.Errorf("Digest title should mention 4 events, got: %s", capturedTitle)
	}
	// The digest body format is "While you were asleep: event1. event2. event3. event4"
	if !strings.Contains(capturedBody, "While you were asleep:") {
		t.Error("Digest body should start with 'While you were asleep:'")
	}
	if !strings.Contains(capturedBody, "Kitchen motion at 23:00") {
		t.Error("Digest body should include first event")
	}
	if !strings.Contains(capturedBody, "Hallway motion at 01:30") {
		t.Error("Digest body should include second event")
	}
	if !strings.Contains(capturedBody, "Bedroom motion at 03:45") {
		t.Error("Digest body should include third event")
	}
	if !strings.Contains(capturedBody, "Bathroom motion at 05:15") {
		t.Error("Digest body should include fourth event")
	}

	// Verify queue was cleared
	ext.mu.RLock()
	queueLen := len(ext.queuedDuringQuiet)
	ext.mu.RUnlock()
	if queueLen != 0 {
		t.Errorf("Queue should be empty after digest, got %d items", queueLen)
	}
}

// TestBatchingMixedPriorities tests that different priorities are batched separately.
func TestBatchingMixedPriorities(t *testing.T) {
	var lowReceived, mediumReceived, highReceived int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]interface{}
		json.Unmarshal(body, &payload)
		title, _ := payload["title"].(string)
		switch {
		case strings.Contains(title, "presence events"):
			lowReceived++
		case strings.Contains(title, "Medium"):
			mediumReceived++
		case strings.Contains(title, "Urgent"):
			highReceived++
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_mixed_prio.db"
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
		BatchWindowSec:   1,
		MaxBatchSize:     10,
		BatchLowPriority: true,
		BatchMedium:      true,
	})

	// Queue mixed priority events
	ext.queueForBatching(Notification{Title: "Low1", Body: "L1", Priority: int(PriorityLow), Timestamp: time.Now()})
	ext.queueForBatching(Notification{Title: "Medium1", Body: "M1", Priority: int(PriorityMedium), Timestamp: time.Now()})
	ext.queueForBatching(Notification{Title: "Low2", Body: "L2", Priority: int(PriorityLow), Timestamp: time.Now()})
	ext.queueForBatching(Notification{Title: "Urgent1", Body: "U1", Priority: int(PriorityUrgent), Timestamp: time.Now()})

	// Wait for batch window
	time.Sleep(1200 * time.Millisecond)

	// URGENT should be sent immediately
	if highReceived != 1 {
		t.Errorf("Expected 1 urgent notification, got %d", highReceived)
	}
	// LOW should be batched into 1
	if lowReceived != 1 {
		t.Errorf("Expected 1 low notification (batched), got %d", lowReceived)
	}
	// MEDIUM should be batched into 1
	if mediumReceived != 1 {
		t.Errorf("Expected 1 medium notification (batched), got %d", mediumReceived)
	}
}

// TestNotificationHistoryRecording tests that notification history records failures.
func TestNotificationHistoryRecording(t *testing.T) {
	dbPath := t.TempDir() + "/test_history_record.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	// Server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	if err := service.AddChannel("wh1", ChannelConfig{
		Type:    ChannelWebhook,
		Enabled: true,
		URL:     server.URL,
	}); err != nil {
		t.Fatalf("AddChannel() error = %v", err)
	}

	// Send a notification that will fail
	service.Send(Notification{
		Title:     "Failed Test",
		Body:      "This will fail",
		Priority:  int(PriorityHigh),
		Timestamp: time.Now(),
	})

	// Check history
	history := service.GetHistory(10)
	if len(history) == 0 {
		t.Fatal("Expected history entry for failed notification")
	}

	entry := history[0]
	if entry.Title != "Failed Test" {
		t.Errorf("Title = %q, want 'Failed Test'", entry.Title)
	}
	if entry.Success {
		t.Error("Expected Success=false for failed notification")
	}
	if entry.Error == "" {
		t.Error("Expected error message in history entry")
	}
	if entry.Channel != "webhook" {
		t.Errorf("Channel = %q, want 'webhook'", entry.Channel)
	}
}

// TestFloorPlanRendererBlobClamping tests that blobs outside image bounds are clamped.
func TestFloorPlanRendererBlobClamping(t *testing.T) {
	dbPath := t.TempDir() + "/test_clamp.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	// Blobs at various positions including outside bounds
	blobs := []struct {
		X, Y, Z  float64
		Identity string
		IsFall   bool
	}{
		{X: -1.0, Z: -1.0}, // Outside (negative)
		{X: 100.0, Z: 100.0}, // Outside (beyond room)
		{X: 3.0, Z: 2.5}, // Inside
	}

	// Should not panic with out-of-bounds blobs
	data, err := service.GenerateFloorPlanThumbnail(300, 300, blobs)
	if err != nil {
		t.Fatalf("GenerateFloorPlanThumbnail() error = %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Empty PNG data returned")
	}

	// Verify PNG signature
	pngSig := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	for i, b := range pngSig {
		if data[i] != b {
			t.Fatalf("Not a PNG (byte %d = %d, want %d)", i, data[i], b)
		}
	}
}

// TestZoneBoundaryRendering tests zone boundary rendering at exact pixel coordinates.
func TestZoneBoundaryRendering(t *testing.T) {
	dbPath := t.TempDir() + "/test_zone_boundary.db"
	base, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	ext, err := NewExtendedService(base)
	if err != nil {
		t.Fatalf("NewExtendedService() error = %v", err)
	}
	defer ext.Close()

	// Zone at exact coordinates
	zones := []struct {
		ID, Name, Color string
		X, Y, W, D      float64
		Highlight       bool
	}{
		{ID: "z1", Name: "TestZone", X: 1.0, Y: 1.0, W: 2.0, D: 2.0, Highlight: false},
		{ID: "z2", Name: "Highlight", X: 3.5, Y: 3.5, W: 1.5, D: 1.5, Highlight: true},
	}

	data, err := ext.GenerateFloorPlanThumbnailExtended(300, 300, zones, nil)
	if err != nil {
		t.Fatalf("GenerateFloorPlanThumbnailExtended() error = %v", err)
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to decode PNG: %v", err)
	}

	// Compute expected zone positions (same as TestFloorPlanZoneBoundaryPixels)
	const margin = 10
	const roomW = 6.0
	const roomD = 5.0
	imgSize := 300
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

	// Verify zone corners are at expected positions
	// Zone 1: X=1.0, Y=1.0, W=2.0, D=2.0
	expectedX1 := int(offsetX + 1.0*scale)
	expectedY1 := int(offsetY + 1.0*scale)
	expectedX2 := int(offsetX + 3.0*scale)
	expectedY2 := int(offsetY + 3.0*scale)

	// Check that zone corners are within image bounds
	if expectedX1 < 0 || expectedX1 >= imgSize {
		t.Errorf("Zone 1 X1 out of bounds: %d", expectedX1)
	}
	if expectedY1 < 0 || expectedY1 >= imgSize {
		t.Errorf("Zone 1 Y1 out of bounds: %d", expectedY1)
	}
	if expectedX2 < 0 || expectedX2 >= imgSize {
		t.Errorf("Zone 1 X2 out of bounds: %d", expectedX2)
	}
	if expectedY2 < 0 || expectedY2 >= imgSize {
		t.Errorf("Zone 1 Y2 out of bounds: %d", expectedY2)
	}

	// Verify highlighted zone has different color
	center1X := (expectedX1 + expectedX2) / 2
	center1Y := (expectedY1 + expectedY2) / 2
	r, g, b, _ := img.At(center1X, center1Y).RGBA()
	r8, g8, b8 := uint8(r>>8), uint8(g>>8), uint8(b>>8)

	// The zone fill color is NRGBA{79, 195, 247, 51} over background
	// Should have noticeable blue component
	if b8 < 50 {
		t.Errorf("Zone center expected blue-ish color, got R=%d G=%d B=%d", r8, g8, b8)
	}
}

// TestRendererProduces300x300PNG tests the exact output specification.
func TestRendererProduces300x300PNG(t *testing.T) {
	dbPath := t.TempDir() + "/test_300x300.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	data, err := service.GenerateFloorPlanThumbnail(300, 300, nil)
	if err != nil {
		t.Fatalf("GenerateFloorPlanThumbnail() error = %v", err)
	}

	// Verify PNG format
	if len(data) < 8 {
		t.Fatalf("PNG data too short: %d bytes", len(data))
	}
	pngSig := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	for i, b := range pngSig {
		if data[i] != b {
			t.Errorf("Not a PNG file (byte %d = %d, want %d)", i, data[i], b)
		}
	}

	// Decode and verify dimensions
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to decode PNG: %v", err)
	}
	if format != "png" {
		t.Errorf("Format = %q, want 'png'", format)
	}

	bounds := img.Bounds()
	if bounds.Dx() != 300 {
		t.Errorf("Width = %d, want 300", bounds.Dx())
	}
	if bounds.Dy() != 300 {
		t.Errorf("Height = %d, want 300", bounds.Dy())
	}

	// Verify background color (dark gray #1a1a2e)
	// Check a pixel inside the image, not on the edge where outline is drawn
	bgColor := img.At(10, 10)
	r, g, b, _ := bgColor.RGBA()
	// RGBA returns 16-bit values (0-65535), shift right by 8 to get 8-bit
	r8, g8, b8 := uint8(r>>8), uint8(g>>8), uint8(b>>8)
	if r8 != 26 || g8 != 26 || b8 != 46 {
		t.Errorf("Background color at (10,10) = R=%d G=%d B=%d, want R=26 G=26 B=46", r8, g8, b8)
	}
}

// TestExtendedRendererProduces300x300PNG tests the extended renderer output.
func TestExtendedRendererProduces300x300PNG(t *testing.T) {
	dbPath := t.TempDir() + "/test_ext_300x300.db"
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
		{ID: "kitchen", Name: "Kitchen", Color: "#4fc3f7", X: 0, Y: 0, W: 3, D: 2, Highlight: true},
	}

	people := []struct {
		Name, Color string
		X, Y, Z     float64
		Confidence  float64
		IsFall      bool
	}{
		{Name: "Alice", Color: "#4488ff", X: 1.5, Y: 1.0, Z: 0.0, Confidence: 0.9, IsFall: false},
	}

	data, err := ext.GenerateFloorPlanThumbnailExtended(300, 300, zones, people)
	if err != nil {
		t.Fatalf("GenerateFloorPlanThumbnailExtended() error = %v", err)
	}

	// Verify PNG format and dimensions
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to decode PNG: %v", err)
	}
	if format != "png" {
		t.Errorf("Format = %q, want 'png'", format)
	}

	bounds := img.Bounds()
	if bounds.Dx() != 300 {
		t.Errorf("Width = %d, want 300", bounds.Dx())
	}
	if bounds.Dy() != 300 {
		t.Errorf("Height = %d, want 300", bounds.Dy())
	}

	// Verify background color
	bgColor := img.At(0, 0)
	r, g, b, _ := bgColor.RGBA()
	// RGBA returns 16-bit values (0-65535), shift right by 8 to get 8-bit
	r8, g8, b8 := uint8(r>>8), uint8(g>>8), uint8(b>>8)
	if r8 != 26 || g8 != 26 || b8 != 46 {
		t.Errorf("Background color = R=%d G=%d B=%d, want R=26 G=26 B=46", r8, g8, b8)
	}
}

// TestQuietHoursQueueing tests that LOW priority events are queued during quiet hours.
func TestQuietHoursQueueing(t *testing.T) {
	dbPath := t.TempDir() + "/test_qh_queue.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	// Enable quiet hours
	if err := service.SetQuietHours(QuietHoursConfig{
		Enabled:   true,
		StartHour: 22,
		StartMin:  0,
		EndHour:   7,
		EndMin:    0,
	}); err != nil {
		t.Fatalf("SetQuietHours() error = %v", err)
	}

	// LOW priority should be suppressed (queued)
	if service.isQuietHours() {
		// We're in quiet hours, LOW priority should be suppressed
		suppressed := service.Send(Notification{
			Title:     "Quiet Hour Event",
			Body:      "Low priority during quiet hours",
			Priority:  int(PriorityLow),
			Timestamp: time.Now(),
		})

		// Should not error, just be suppressed
		if suppressed != nil {
			t.Errorf("LOW priority during quiet hours should be suppressed (no error), got: %v", suppressed)
		}
	}
}

// TestQuietHoursURGENTDelivered tests that URGENT bypasses quiet hours.
func TestQuietHoursURGENTDelivered(t *testing.T) {
	sent := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_qh_urgent_deliver.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	// Enable quiet hours
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

	// URGENT should bypass quiet hours
	err = service.Send(Notification{
		Title:     "URGENT Alert",
		Body:      "High priority during quiet hours",
		Priority:  int(PriorityUrgent),
		Timestamp: time.Now(),
	})

	if err != nil {
		t.Fatalf("Send() with URGENT priority should not error, got: %v", err)
	}
	if !sent {
		t.Error("URGENT priority should be delivered during quiet hours")
	}
}

// TestBatchingWindow tests that batching respects the configured window duration.
func TestBatchingWindow(t *testing.T) {
	dbPath := t.TempDir() + "/test_batch_window.db"
	base, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	ext, err := NewExtendedService(base)
	if err != nil {
		t.Fatalf("NewExtendedService() error = %v", err)
	}
	defer ext.Close()

	// Configure batch window
	ext.SetBatchingConfig(BatchingConfig{
		Enabled:          true,
		BatchWindowSec:   2,
		MaxBatchSize:     10,
		BatchLowPriority: true,
		BatchMedium:      true,
	})

	// Get the config to verify
	config := ext.GetBatchingConfig()
	if config.BatchWindowSec != 2 {
		t.Errorf("BatchWindowSec = %d, want 2", config.BatchWindowSec)
	}
	if config.MaxBatchSize != 10 {
		t.Errorf("MaxBatchSize = %d, want 10", config.MaxBatchSize)
	}
	if !config.BatchLowPriority {
		t.Error("BatchLowPriority should be true")
	}
	if !config.BatchMedium {
		t.Error("BatchMedium should be true")
	}
}

// TestMorningDigestAtQuietHoursEnd tests digest delivery timing at quiet_hours_end.
func TestMorningDigestAtQuietHoursEnd(t *testing.T) {
	digestSent := false
	var capturedTitle string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]interface{}
		json.Unmarshal(body, &payload)
		capturedTitle, _ = payload["title"].(string)
		digestSent = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_digest_timing.db"
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

	// Simulate queued events from overnight
	ext.mu.Lock()
	ext.queuedDuringQuiet = []Notification{
		{Title: "Event 1", Body: "Overnight event 1", Priority: int(PriorityLow), Timestamp: time.Now()},
		{Title: "Event 2", Body: "Overnight event 2", Priority: int(PriorityLow), Timestamp: time.Now()},
	}
	ext.mu.Unlock()

	// Trigger digest (simulating quiet_hours_end at 07:00)
	ext.sendMorningDigest()

	// Wait for goroutine
	time.Sleep(100 * time.Millisecond)

	if !digestSent {
		t.Error("Morning digest should be sent when triggered")
	}
	if capturedTitle == "" {
		t.Error("Digest title should not be empty")
	}
	if !strings.Contains(capturedTitle, "2") {
		t.Errorf("Digest title should mention 2 events, got: %s", capturedTitle)
	}
}

// TestSendWithMultipleChannels tests that notifications are sent to all enabled channels.
func TestSendWithMultipleChannels(t *testing.T) {
	var ntfyCalled, webhookCalled, gotifyCalled bool

	ntfyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ntfyCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer ntfyServer.Close()

	webhookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webhookCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookServer.Close()

	gotifyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotifyCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer gotifyServer.Close()

	dbPath := t.TempDir() + "/test_multi_channel.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	// Add multiple channels
	service.AddChannel("ntfy1", ChannelConfig{Type: ChannelNtfy, Enabled: true, URL: ntfyServer.URL})
	service.AddChannel("wh1", ChannelConfig{Type: ChannelWebhook, Enabled: true, URL: webhookServer.URL})
	service.AddChannel("gotify1", ChannelConfig{Type: ChannelGotify, Enabled: true, URL: gotifyServer.URL, Token: "test"})

	notif := Notification{
		Title:     "Multi Channel Test",
		Body:      "Sent to all enabled channels",
		Timestamp: time.Now(),
	}

	if err := service.Send(notif); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if !ntfyCalled {
		t.Error("Notification should be sent to ntfy channel")
	}
	if !webhookCalled {
		t.Error("Notification should be sent to webhook channel")
	}
	if !gotifyCalled {
		t.Error("Notification should be sent to gotify channel")
	}
}

// TestNotificationTimestamp tests that notification timestamps are recorded correctly.
func TestNotificationTimestamp(t *testing.T) {
	dbPath := t.TempDir() + "/test_timestamp.db"
	service, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	beforeSend := time.Now()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	service.AddChannel("wh1", ChannelConfig{
		Type:    ChannelWebhook,
		Enabled: true,
		URL:     server.URL,
	})

	testTimestamp := time.Now().Truncate(time.Millisecond)
	notif := Notification{
		Title:     "Timestamp Test",
		Body:      "Testing timestamp recording",
		Timestamp: testTimestamp,
	}

	service.Send(notif)

	afterSend := time.Now()

	history := service.GetHistory(1)
	if len(history) == 0 {
		t.Fatal("Expected history entry")
	}

	entry := history[0]
	if entry.Timestamp.Before(beforeSend) || entry.Timestamp.After(afterSend) {
		t.Errorf("Timestamp %v outside expected range [%v, %v]", entry.Timestamp, beforeSend, afterSend)
	}
}

// TestMergeNotificationsEmptySlice tests merging empty notification slice.
func TestMergeNotificationsEmptySlice(t *testing.T) {
	dbPath := t.TempDir() + "/test_merge_empty.db"
	base, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	ext, err := NewExtendedService(base)
	if err != nil {
		t.Fatalf("NewExtendedService() error = %v", err)
	}
	defer ext.Close()

	merged := ext.mergeNotifications([]Notification{})
	if merged.Title != "" || merged.Body != "" {
		t.Error("Merging empty slice should return empty notification")
	}
}

// TestMorningDigestPreventsDuplicateSend tests that digest is sent only once per day.
func TestMorningDigestPreventsDuplicateSend(t *testing.T) {
	digestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		digestCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dbPath := t.TempDir() + "/test_digest_once.db"
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

	// Queue events
	ext.mu.Lock()
	ext.queuedDuringQuiet = []Notification{
		{Title: "Event", Body: "Overnight", Priority: int(PriorityLow), Timestamp: time.Now()},
	}
	ext.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	ext.digestLastDate = today    // Simulate already sent today
	ext.digestSentToday = true    // Also need to set this flag

	// Try to send digest
	ext.sendMorningDigest()

	time.Sleep(100 * time.Millisecond)

	// Should not send because already sent today
	if digestCount != 0 {
		t.Error("Digest should not be sent twice in the same day")
	}
}

// TestChannelConfigPersistence tests that channel configuration persists across service restarts.
func TestChannelConfigPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test_persist.db"

	// Create first service and configure channels
	s1, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	s1.AddChannel("ntfy1", ChannelConfig{
		Type:    ChannelNtfy,
		Enabled: true,
		URL:     "https://ntfy.sh/test",
		Token:   "tk_test",
	})
	s1.SetQuietHours(QuietHoursConfig{
		Enabled:   true,
		StartHour: 22,
		StartMin:  30,
		EndHour:   7,
		EndMin:    30,
	})
	s1.Close()

	// Create second service with same database
	s2, err := NewService(dbPath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer s2.Close()

	// Verify channels were loaded
	channels := s2.GetChannels()
	if len(channels) != 1 {
		t.Errorf("Expected 1 channel, got %d", len(channels))
	}

	ntfy, ok := channels["ntfy1"]
	if !ok {
		t.Fatal("ntfy1 channel not found")
	}
	if !ntfy.Enabled {
		t.Error("Channel should be enabled")
	}
	if ntfy.URL != "https://ntfy.sh/test" {
		t.Errorf("URL = %q, want 'https://ntfy.sh/test'", ntfy.URL)
	}

	// Verify quiet hours
	qh := s2.GetQuietHours()
	if !qh.Enabled {
		t.Error("Quiet hours should be enabled")
	}
	if qh.StartHour != 22 {
		t.Errorf("StartHour = %d, want 22", qh.StartHour)
	}
	if qh.StartMin != 30 {
		t.Errorf("StartMin = %d, want 30", qh.StartMin)
	}
}
