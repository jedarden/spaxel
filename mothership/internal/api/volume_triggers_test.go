package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/spaxel/mothership/internal/volume"
)

// TestTestTriggerEndpoint tests POST /api/triggers/{id}/test.
func TestTestTriggerEndpoint(t *testing.T) {
	handler, err := NewVolumeTriggersHandler(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer handler.Close()

	// Create a trigger with a webhook action
	trigger := &volume.Trigger{
		Name: "test trigger",
		Shape: volume.ShapeJSON{
			Type: volume.ShapeBox,
			X:    float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W:    float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "dwell",
		Enabled:  true,
		Actions: []volume.Action{
			{Type: "webhook", Params: map[string]interface{}{"url": "http://example.com/hook"}},
		},
	}

	id, err := handler.store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	// Test with a mock server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer mockServer.Close()

	// Replace the action URL with the mock server URL
	tg, _ := handler.store.Get(id)
	tg.Actions[0].Params["url"] = mockServer.URL
	handler.store.Update(tg)

	// Call test endpoint
	req := httptest.NewRequest("POST", "/api/triggers/"+id+"/test", nil)
	w := httptest.NewRecorder()
	handler.testTrigger(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var result WebhookTestResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if result.Status != "ok" {
		t.Errorf("Expected status 'ok', got %s", result.Status)
	}

	if len(result.Actions) != 1 {
		t.Fatalf("Expected 1 action result, got %d", len(result.Actions))
	}

	if result.Actions[0].Status != 200 {
		t.Errorf("Expected action status 200, got %d", result.Actions[0].Status)
	}

	if result.Actions[0].ResponseMs <= 0 {
		t.Error("Expected positive response_ms")
	}
}

// TestTestTrigger_ReturnsErrorOnMissingURL tests that missing URL produces an error result.
func TestTestTrigger_ReturnsErrorOnMissingURL(t *testing.T) {
	handler, err := NewVolumeTriggersHandler(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer handler.Close()

	trigger := &volume.Trigger{
		Name: "no url trigger",
		Shape: volume.ShapeJSON{
			Type: volume.ShapeBox,
			X:    float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W:    float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "enter",
		Enabled:  true,
		Actions: []volume.Action{
			{Type: "webhook", Params: map[string]interface{}{}},
		},
	}

	id, err := handler.store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("POST", "/api/triggers/"+id+"/test", nil)
	w := httptest.NewRecorder()
	handler.testTrigger(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var result WebhookTestResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if len(result.Actions) != 1 {
		t.Fatalf("Expected 1 action result, got %d", len(result.Actions))
	}

	if result.Actions[0].Error != "missing url" {
		t.Errorf("Expected error 'missing url', got %q", result.Actions[0].Error)
	}
}

// TestTestTrigger_4xxInTestDoesNotDisable tests that test endpoint doesn't disable trigger on 4xx.
func TestTestTrigger_4xxInTestDoesNotDisable(t *testing.T) {
	handler, err := NewVolumeTriggersHandler(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer handler.Close()

	// Mock server that always returns 404
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	trigger := &volume.Trigger{
		Name: "4xx test trigger",
		Shape: volume.ShapeJSON{
			Type: volume.ShapeBox,
			X:    float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W:    float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "enter",
		Enabled:  true,
		Actions: []volume.Action{
			{Type: "webhook", Params: map[string]interface{}{"url": mockServer.URL}},
		},
	}

	id, err := handler.store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	// Call test endpoint — 4xx from mock
	req := httptest.NewRequest("POST", "/api/triggers/"+id+"/test", nil)
	w := httptest.NewRecorder()
	handler.testTrigger(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Trigger should still be enabled (test mode doesn't disable)
	tg, _ := handler.store.Get(id)
	if !tg.Enabled {
		t.Error("Expected trigger to remain enabled after test endpoint 4xx")
	}
}

// TestEnableEndpoint tests POST /api/triggers/{id}/enable.
func TestEnableEndpoint(t *testing.T) {
	handler, err := NewVolumeTriggersHandler(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer handler.Close()

	trigger := &volume.Trigger{
		Name: "test enable",
		Shape: volume.ShapeJSON{
			Type: volume.ShapeBox,
			X:    float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W:    float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "enter",
		Enabled:  false,
	}

	id, err := handler.store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	// Disable with error
	handler.store.DisableTriggerWithError(id, "HTTP 403")
	handler.store.IncrementErrorCount(id)

	req := httptest.NewRequest("POST", "/api/triggers/"+id+"/enable", nil)
	w := httptest.NewRecorder()
	handler.enableTrigger(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	tg, _ := handler.store.Get(id)
	if !tg.Enabled {
		t.Error("Expected trigger to be enabled after enable endpoint call")
	}
	if tg.ErrorMessage != "" {
		t.Errorf("Expected empty error_message, got %q", tg.ErrorMessage)
	}
	if tg.ErrorCount != 0 {
		t.Errorf("Expected error_count 0, got %d", tg.ErrorCount)
	}
}

// TestGetWebhookLogEndpoint tests GET /api/triggers/{id}/webhook-log.
func TestGetWebhookLogEndpoint(t *testing.T) {
	handler, err := NewVolumeTriggersHandler(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer handler.Close()

	trigger := &volume.Trigger{
		Name: "log test",
		Shape: volume.ShapeJSON{
			Type: volume.ShapeBox,
			X:    float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W:    float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "enter",
		Enabled: true,
	}

	id, err := handler.store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UnixMilli()
	handler.store.WriteWebhookLog(id, "http://a.com", now, 200, 50, "")
	handler.store.WriteWebhookLog(id, "http://b.com", now-1000, 500, "timeout")

	req := httptest.NewRequest("GET", "/api/triggers/"+id+"/webhook-log?limit=10", nil)
	w := httptest.NewRecorder()
	handler.getWebhookLog(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var entries []volume.WebhookLogEntry
	if err := json.NewDecoder(w.Body).Decode(&entries); err != nil {
		t.Fatal(err)
	}

	if len(entries) != 2 {
		t.Fatalf("Expected 2 log entries, got %d", len(entries))
	}

	// Most recent first
	if entries[0].URL != "http://a.com" {
		t.Errorf("Expected first entry URL 'http://a.com', got %s", entries[0].URL)
	}
	if entries[0].Status != 200 {
		t.Errorf("Expected first entry status 200, got %d", entries[0].Status)
	}
}

// TestWebhookPayloadSchema tests that the webhook payload contains all required fields.
func TestWebhookPayloadSchema(t *testing.T) {
	// Create a mock server to capture the payload
	var receivedPayload map[string]interface{}
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	handler, err := NewVolumeTriggersHandler(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer handler.Close()

	trigger := &volume.Trigger{
		Name: "payload test",
		Shape: volume.ShapeJSON{
			Type: volume.ShapeBox,
			X:    float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W:    float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "dwell",
		Enabled:  true,
		Actions: []volume.Action{
			{Type: "webhook", Params: map[string]interface{}{"url": mockServer.URL}},
		},
	}

	id, err := handler.store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	// Fire the trigger
	now := time.Now()
	handler.onTriggerFired(volume.FiredEvent{
		TriggerID:   id,
		TriggerName: "payload test",
		Condition:   "dwell",
		BlobIDs:     []int{1, 2},
		Timestamp:   now,
	})

	// Give the async callback time to complete
	time.Sleep(100 * time.Millisecond)

	requiredFields := []string{"trigger_id", "trigger_name", "condition", "blob_id", "person", "position", "zone", "dwell_s", "timestamp_ms"}
	for _, field := range requiredFields {
		if _, ok := receivedPayload[field]; !ok {
			t.Errorf("Missing required field %q in webhook payload", field)
		}
	}

	if receivedPayload["trigger_name"] != "payload test" {
		t.Errorf("Expected trigger_name 'payload test', got %v", receivedPayload["trigger_name"])
	}
	if receivedPayload["condition"] != "dwell" {
		t.Errorf("Expected condition 'dwell', got %v", receivedPayload["condition"])
	}
}

// Test5xxDoesNotDisableTrigger tests that 5xx responses only increment error count.
func Test5xxDoesNotDisableTrigger(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer mockServer.Close()

	handler, err := NewVolumeTriggersHandler(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer handler.Close()

	trigger := &volume.Trigger{
		Name: "5xx test",
		Shape: volume.ShapeJSON{
			Type: volume.ShapeBox,
			X:    float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W:    float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "enter",
		Enabled:  true,
		Actions: []volume.Action{
			{Type: "webhook", Params: map[string]interface{}{"url": mockServer.URL}},
		},
	}

	id, err := handler.store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	// Fire 5 times — all should be 5xx
	for i := 0; i < 5; i++ {
		handler.onTriggerFired(volume.FiredEvent{
			TriggerID:   id,
			TriggerName: "5xx test",
			Condition:   "enter",
			BlobIDs:     []int{1},
			Timestamp:   time.Now(),
		})
		time.Sleep(100 * time.Millisecond)
	}

	tg, _ := handler.store.Get(id)
	if !tg.Enabled {
		t.Error("Expected trigger to remain enabled after 5xx errors")
	}
	if tg.ErrorCount != 5 {
		t.Errorf("Expected error_count 5, got %d", tg.ErrorCount)
	}
}

// Test4xxDisablesTrigger tests that 4xx responses disable the trigger.
func Test4xxDisablesTrigger(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer mockServer.Close()

	handler, err := NewVolumeTriggersHandler(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer handler.Close()

	trigger := &volume.Trigger{
		Name: "4xx test",
		Shape: volume.ShapeJSON{
			Type: volume.ShapeBox,
			X:    float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W:    float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "enter",
		Enabled:  true,
		Actions: []volume.Action{
			{Type: "webhook", Params: map[string]interface{}{"url": mockServer.URL}},
		},
	}

	id, err := handler.store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	// Fire — should get 403 and disable
	handler.onTriggerFired(volume.FiredEvent{
		TriggerID:   id,
		TriggerName: "4xx test",
		Condition:   "enter",
		BlobIDs:     []int{1},
		Timestamp:   time.Now(),
	})

	time.Sleep(100 * time.Millisecond)

	tg, _ := handler.store.Get(id)
	if tg.Enabled {
		t.Error("Expected trigger to be disabled after 4xx response")
	}
	if tg.ErrorMessage == "" {
		t.Error("Expected non-empty error_message after 4xx response")
	}
}

// Test2xxResetsErrorCount tests that a 2xx response resets error_count.
func Test2xxResetsErrorCount(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return 500 first, then 200
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	handler, err := NewVolumeTriggersHandler(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer handler.Close()

	trigger := &volume.Trigger{
		Name: "2xx reset test",
		Shape: volume.ShapeJSON{
			Type: volume.ShapeBox,
			X:    float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W:    float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "enter",
		Enabled:  true,
		Actions: []volume.Action{
			{Type: "webhook", Params: map[string]interface{}{"url": mockServer.URL}},
		},
	}

	id, err := handler.store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	// Accumulate some errors first
	handler.store.IncrementErrorCount(id)
	handler.store.IncrementErrorCount(id)
	handler.store.IncrementErrorCount(id)

	// Fire — should get 200 and reset
	handler.onTriggerFired(volume.FiredEvent{
		TriggerID:   id,
		TriggerName: "2xx reset test",
		Condition:   "enter",
		BlobIDs:     []int{1},
		Timestamp:   time.Now(),
	})

	time.Sleep(100 * time.Millisecond)

	tg, _ := handler.store.Get(id)
	if tg.ErrorCount != 0 {
		t.Errorf("Expected error_count 0 after 2xx, got %d", tg.ErrorCount)
	}
	if !tg.Enabled {
		t.Error("Expected trigger to remain enabled after 2xx")
	}
}

// TestTimeoutDoesNotDisable tests that request timeouts don't disable the trigger.
func TestTimeoutDoesNotDisable(t *testing.T) {
	// Mock server that never responds (will cause timeout)
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second)
	}))
	defer mockServer.Close()

	handler, err := NewVolumeTriggersHandler(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer handler.Close()

	// Use a very short timeout for testing
	handler.httpClient.Timeout = 100 * time.Millisecond

	trigger := &volume.Trigger{
		Name: "timeout test",
		Shape: volume.ShapeJSON{
			Type: volume.ShapeBox,
			X:    float64Ptr(0), Y: float64Ptr(0), Z: float64Ptr(0),
			W:    float64Ptr(1), D: float64Ptr(1), H: float64Ptr(1),
		},
		Condition: "enter",
		Enabled:  true,
		Actions: []volume.Action{
			{Type: "webhook", Params: map[string]interface{}{"url": mockServer.URL}},
		},
	}

	id, err := handler.store.Create(trigger)
	if err != nil {
		t.Fatal(err)
	}

	handler.onTriggerFired(volume.FiredEvent{
		TriggerID:   id,
		TriggerName: "timeout test",
		Condition:   "enter",
		BlobIDs:     []int{1},
		Timestamp:   time.Now(),
	})

	// Wait for the timeout to complete
	time.Sleep(500 * time.Millisecond)

	tg, _ := handler.store.Get(id)
	if !tg.Enabled {
		t.Error("Expected trigger to remain enabled after timeout")
	}
	if tg.ErrorCount != 1 {
		t.Errorf("Expected error_count 1 after timeout, got %d", tg.ErrorCount)
	}
}

// Helper
func float64Ptr(f float64) *float64 {
	return &f
}
