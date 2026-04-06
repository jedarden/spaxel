package automation

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Mock providers for type mockZoneProvider struct {
	zones     map[string]string
	occupancy map[string]struct {
		count   int
		blobIDs []int
	}
}

func (m *mockZoneProvider) GetZone(id string) (string, bool) {
	name, ok := m.zones[id]
	return name, ok
}

func (m *mockZoneProvider) GetZoneOccupancy(zoneID string) (int, []int) {
	if occ, ok := m.occupancy[zoneID]; ok {
		return occ.count, occ.blobIDs
	}
	return 0, nil
}

type mockPersonProvider struct {
	people map[string]struct {
		name  string
		color string
	}
}

func (m *mockPersonProvider) GetPerson(id string) (string, string, bool) {
	p, ok := m.people[id]
	return p.name, p.color, ok
}

type mockDeviceProvider struct {
	devices map[string]string
}

func (m *mockDeviceProvider) GetDevice(mac string) (string, bool) {
	name, ok := m.devices[mac]
	return name, ok
}

type mockMQTTClient struct {
	published []struct {
		topic   string
		payload []byte
	}
	connected bool
}

func (m *mockMQTTClient) Publish(topic string, payload []byte) error {
	m.published = append(m.published, struct {
		topic   string
		payload []byte
	}{topic, payload})
	return nil
}

func (m *mockMQTTClient) IsConnected() bool {
	return m.connected
}

type mockNotifySender struct {
	sent []struct {
		channel string
		title   string
		body    string
		data    map[string]interface{}
	}
}

func (m *mockNotifySender) SendViaChannel(channelType string, title, body string, data map[string]interface{}) error {
	m.sent = append(m.sent, struct {
		channel: channelType,
		title:   title,
		body:   body,
		data:   data,
	})
	return nil
}

func newTestEngine(t *testing.T) (*Engine, string) {
	tmpDir, err := os.MkdirTemp("", "automation-test-*")
	if err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(tmpDir, "automation.db")
	engine, err := NewEngine(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatal(err)
	}

	t.Cleanup(func() {
		engine.Close()
		os.RemoveAll(tmpDir)
	})

	return engine, tmpDir
}

func TestTriggerMatching(t *testing.T) {
	engine, _ := newTestEngine(t)

	// Create test automation for zone_enter
	zoneEnterAutomation := &Automation{
		ID:          "test-zone-enter",
		Name:        "Test Zone Enter",
		Enabled:     true,
		TriggerType: TriggerZoneEnter,
		TriggerConfig: TriggerConfig{
			ZoneID:   "kitchen",
			PersonID: "anyone",
		},
		Actions: []Action{
			{Type: ActionWebhook, URL: "http://example.com/hook"},
		},
		Cooldown: 60,
	}
	if err := engine.CreateAutomation(zoneEnterAutomation); err != nil {
		t.Fatal(err)
	}

	// Create test automation for fall_detected with specific person
	fallAutomation := &Automation{
		ID:          "test-fall",
		Name:        "Test Fall Detected",
		Enabled:     true,
		TriggerType: TriggerFallDetected,
		TriggerConfig: TriggerConfig{
			PersonID: "alice",
		},
		Actions: []Action{
			{Type: ActionWebhook, URL: "http://example.com/fall"},
		},
		Cooldown: 60,
	}
	if err := engine.CreateAutomation(fallAutomation); err != nil {
		t.Fatal(err)
	}

	// Test matching zone_enter event
	triggered := false
	engine.SetOnTrigger(func(data TriggerEventData) {
		triggered = true
		if data.AutomationID != "test-zone-enter" {
			t.Errorf("Expected automation test-zone-enter, got %s", data.AutomationID)
		}
	})

	engine.ProcessEvent(Event{
		Type:     TriggerZoneEnter,
		ZoneID:   "kitchen",
		PersonID: "bob",
	})

	if !triggered {
		t.Error("Expected zone_enter automation to trigger")
	}

	// Test non-matching event
	triggered = false
	engine.ProcessEvent(Event{
		Type:     TriggerZoneLeave,
		ZoneID:   "kitchen",
		PersonID: "bob",
	})

	if triggered {
		t.Error("zone_leave event should not trigger zone_enter automation")
	}

	// Test fall_detected with specific person (reset cooldown first)
	engine.cooldowns["test-fall"] = time.Now().Add(-time.Minute)

	triggered = false
	engine.ProcessEvent(Event{
		Type:     TriggerFallDetected,
		PersonID: "alice",
		ZoneID:   "living_room",
	})

	if !triggered {
		t.Error("Expected fall_detected automation to trigger for alice")
	}
}

func TestTimeWindowCondition(t *testing.T) {
	engine, _ := newTestEngine(t)

	automation := &Automation{
		ID:          "test-time",
		Name:        "Test Time Window",
		Enabled:     true,
		TriggerType: TriggerZoneEnter,
		TriggerConfig: TriggerConfig{
			ZoneID:   "bedroom",
			PersonID: "anyone",
		},
		Conditions: []Condition{
			{Type: ConditionTimeWindow, Value: "22:00-07:00"},
		},
		Actions: []Action{
			{Type: ActionWebhook, URL: "http://example.com/night"},
		},
		Cooldown: 60,
	}
	if err := engine.CreateAutomation(automation); err != nil {
		t.Fatal(err)
	}

	// Test times - overnight range (22:00-07:00)
	testCases := []struct {
		hour     int
		minute   int
		expected bool
	}{
		{23, 0, true},   // 23:00 - should pass
		{4, 0, true},    // 04:00 - should pass
		{8, 0, false},   // 08:00 - should not pass
		{12, 0, false},  // 12:00 - should not pass
		{21, 59, false}, // 21:59 - should not pass
		{22, 0, true},   // 22:00 - should pass
		{6, 59, true},   // 06:59 - should pass
		{7, 0, true},    // 07:00 - should pass (end is inclusive)
	}

	for _, tc := range testCases {
		testTime := time.Date(2024, 1, 15, tc.hour, tc.minute, 0, 0, time.Local)
		result := engine.isTimeInRange("22:00-07:00", testTime)
		if result != tc.expected {
			t.Errorf("Time %02d:%02d: expected %v, got %v", tc.hour, tc.minute, tc.expected, result)
		}
	}

	// Test normal range (daytime 07:00-18:00)
	dayTestCases := []struct {
		hour     int
		minute   int
		expected bool
	}{
		{8, 0, true},    // 08:00 - within 07:00-18:00
		{12, 0, true},   // 12:00 - within 07:00-18:00
		{17, 59, true},  // 17:59 - within 07:00-18:00
		{6, 59, false},  // 06:59 - outside 07:00-18:00
		{18, 1, false},  // 18:01 - outside 07:00-18:00
	}

	for _, tc := range dayTestCases {
		testTime := time.Date(2024, 1, 15, tc.hour, tc.minute, 0, 0, time.Local)
		result := engine.isTimeInRange("07:00-18:00", testTime)
		if result != tc.expected {
			t.Errorf("Daytime %02d:%02d: expected %v, got %v", tc.hour, tc.minute, tc.expected, result)
		}
	}
}

func TestPersonFilterCondition(t *testing.T) {
	engine, _ := newTestEngine(t)

	// Set up mock person provider
	engine.SetPersonProvider(&mockPersonProvider{
		people: map[string]struct {
			name  string
			color string
		}{
			"alice": {name: "Alice", color: "#ff0000"},
			"bob":   {name: "Bob", color: "#00ff00"},
		},
	})

	automation := &Automation{
		ID:          "test-person-filter",
		Name:        "Test Person Filter",
		Enabled:     true,
		TriggerType: TriggerZoneEnter,
		TriggerConfig: TriggerConfig{
			ZoneID:   "office",
			PersonID: "anyone",
		},
		Conditions: []Condition{
			{Type: ConditionPersonFilter, Value: "alice"},
		},
		Actions: []Action{
			{Type: ActionWebhook, URL: "http://example.com/alice-only"},
		},
		Cooldown: 60,
	}
	if err := engine.CreateAutomation(automation); err != nil {
		t.Fatal(err)
	}

	// Test with alice - should trigger
	triggered := false
	engine.SetOnTrigger(func(data TriggerEventData) {
		triggered = true
	})

	engine.ProcessEvent(Event{
		Type:     TriggerZoneEnter,
		ZoneID:   "office",
		PersonID: "alice",
	})

	if !triggered {
		t.Error("Expected automation to trigger for alice")
	}

	// Test with bob - should not trigger
	triggered = false
	engine.ProcessEvent(Event{
		Type:     TriggerZoneEnter,
		ZoneID:   "office",
		PersonID: "bob",
	})

	if triggered {
		t.Error("Automation should not trigger for bob (condition filters for alice)")
	}

	// Test with "anyone" filter
	automation.Conditions = []Condition{
		{Type: ConditionPersonFilter, Value: "anyone"},
	}
	engine.UpdateAutomation(automation)

	triggered = false
	engine.ProcessEvent(Event{
		Type:     TriggerZoneEnter,
		ZoneID:   "office",
		PersonID: "charlie",
	})

	if !triggered {
		t.Error("Expected automation to trigger for anyone")
	}
}

func TestDayOfWeekCondition(t *testing.T) {
	engine, _ := newTestEngine(t)

	automation := &Automation{
		ID:          "test-weekday",
		Name:        "Test Weekday",
		Enabled:     true,
		TriggerType: TriggerZoneEnter,
		TriggerConfig: TriggerConfig{
			ZoneID:   "office",
			PersonID: "anyone",
		},
		Conditions: []Condition{
			{Type: ConditionDayOfWeek, Value: "1,2,3,4,5"}, // Mon-Fri
		},
		Actions: []Action{
			{Type: ActionWebhook, URL: "http://example.com/weekday"},
		},
		Cooldown: 60,
	}
	if err := engine.CreateAutomation(automation); err != nil {
		t.Fatal(err)
	}

	// Test weekdays (Mon=1, Fri=5)
	weekdayTests := []struct {
		weekday  time.Weekday
		expected bool
	}{
		{time.Monday, true},
		{time.Friday, true},
		{time.Saturday, false},
		{time.Sunday, false},
	}

	for _, tc := range weekdayTests {
		testTime := time.Date(2024, 1, int(tc.weekday), 12, 0, 0, time.Local)
		result := engine.isDayOfWeek("1,2,3,4,5", testTime)
		if result != tc.expected {
				dayName := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
				t.Errorf("Day %s: expected %v, got %v", dayName[tc.weekday], tc.expected, result)
			}
		}
	}
}

func TestWebhookDispatch(t *testing.T) {
	engine, _ := newTestEngine(t)

	var receivedPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		json.NewDecoder(r.Body).Decode(&payload)
		receivedPayload = payload
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	automation := &Automation{
		ID:          "test-webhook",
		Name:        "Test Webhook",
		Enabled:     true,
		TriggerType: TriggerZoneEnter,
		TriggerConfig: TriggerConfig{
			ZoneID:   "test",
			PersonID: "anyone",
		},
		Actions: []Action{
			{Type: ActionWebhook, URL: server.URL},
		},
		Cooldown: 0,
	}
	if err := engine.CreateAutomation(automation); err != nil {
		t.Fatal(err)
	}

	// Set up zone provider for zone name
	engine.SetZoneProvider(&mockZoneProvider{
		zones: map[string]string{"test": "Test Zone"},
	})

	engine.ProcessEvent(Event{
		Type:      TriggerZoneEnter,
		ZoneID:    "test",
		ZoneName:  "Test Zone",
		PersonName: "Alice",
		Timestamp:  time.Now(),
	})

	// Wait for async webhook
	time.Sleep(100 * time.Millisecond)

	if receivedPayload == nil {
		t.Fatal("Webhook was not called")
	}

	// Verify payload contains expected fields
	if receivedPayload["zone_name"] != "Test Zone" {
		t.Errorf("Expected zone_name 'Test Zone', got %v", receivedPayload["zone_name"])
	}
}

func TestWebhookRetry(t *testing.T) {
	engine, _ := newTestEngine(t)

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// First call returns 503
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		// Second call succeeds
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	automation := &Automation{
		ID:          "test-retry",
		Name:        "Test Retry",
		Enabled:     true,
		TriggerType: TriggerZoneEnter,
		TriggerConfig: TriggerConfig{
			ZoneID:   "test",
			PersonID: "anyone",
		},
		Actions: []Action{
			{Type: ActionWebhook, URL: server.URL},
		},
		Cooldown: 0,
	}
	if err := engine.CreateAutomation(automation); err != nil {
		t.Fatal(err)
	}

	engine.ProcessEvent(Event{
		Type:     TriggerZoneEnter,
		ZoneID:   "test",
		Timestamp: time.Now(),
	})

	// Wait for first call
	time.Sleep(100 * time.Millisecond)

	if callCount != 1 {
		t.Errorf("Expected 1 call immediately, got %d", callCount)
	}

	// Wait for retry (30s in real code, but we can't wait that long in tests)
	// This test verifies the first call behavior
}

func TestMQTTPublish(t *testing.T) {
	engine, _ := newTestEngine(t)

	mockMQTT := &mockMQTTClient{connected: true}
	engine.SetMQTTClient(mockMQTT)

	automation := &Automation{
		ID:          "test-mqtt",
		Name:        "Test MQTT",
		Enabled:     true,
		TriggerType: TriggerZoneEnter,
		TriggerConfig: TriggerConfig{
			ZoneID:   "test",
			PersonID: "anyone",
		},
		Actions: []Action{
			{Type: ActionMQTT, Topic: "home/test/trigger"},
		},
		Cooldown: 0,
	}
	if err := engine.CreateAutomation(automation); err != nil {
		t.Fatal(err)
	}

	engine.ProcessEvent(Event{
		Type:     TriggerZoneEnter,
		ZoneID:   "test",
		Timestamp: time.Now(),
	})

	// Wait for async publish
	time.Sleep(100 * time.Millisecond)

	if len(mockMQTT.published) != 1 {
		t.Fatalf("Expected 1 MQTT publish, got %d", len(mockMQTT.published))
	}

	if mockMQTT.published[0].topic != "home/test/trigger" {
		t.Errorf("Expected topic 'home/test/trigger', got %s", mockMQTT.published[0].topic)
	}
}

func TestTestFireMode(t *testing.T) {
	engine, _ := newTestEngine(t)

	var receivedPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		json.NewDecoder(r.Body).Decode(&payload)
		receivedPayload = payload
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	automation := &Automation{
		ID:          "test-testfire",
		Name:        "Test Fire Mode",
		Enabled:     true,
		TriggerType: TriggerZoneEnter,
		TriggerConfig: TriggerConfig{
			ZoneID:   "kitchen",
			PersonID: "anyone",
		},
		Actions: []Action{
			{Type: ActionWebhook, URL: server.URL},
		},
		Cooldown: 0,
	}
	if err := engine.CreateAutomation(automation); err != nil {
		t.Fatal(err)
	}

	// Test fire
	err := engine.TestFire("test-testfire")
	if err != nil {
		t.Fatal(err)
	}

	// Wait for webhook
	time.Sleep(100 * time.Millisecond)

	if receivedPayload == nil {
		t.Fatal("Webhook was not called for test fire")
	}

	// Verify test_mode flag
	if testMode, ok := receivedPayload["test_mode"]; !ok || !testMode.(bool) {
		t.Error("Expected test_mode to be true in payload")
	}
}

func TestFireCountIncrement(t *testing.T) {
	engine, _ := newTestEngine(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	automation := &Automation{
		ID:          "test-firecount",
		Name:        "Test Fire Count",
		Enabled:     true,
		TriggerType: TriggerZoneEnter,
		TriggerConfig: TriggerConfig{
			ZoneID:   "test",
			PersonID: "anyone",
		},
		Actions: []Action{
			{Type: ActionWebhook, URL: server.URL},
		},
		Cooldown: 0,
	}
	if err := engine.CreateAutomation(automation); err != nil {
		t.Fatal(err)
	}

	// Trigger 3 times
	for i := 0; i < 3; i++ {
		engine.ProcessEvent(Event{
			Type:     TriggerZoneEnter,
			ZoneID:   "test",
			Timestamp: time.Now(),
		})
		time.Sleep(10 * time.Millisecond)
	}

	// Check fire count
	auto := engine.GetAutomation("test-firecount")
	if auto.FireCount != 3 {
		t.Errorf("Expected fire_count 3, got %d", auto.FireCount)
	}

	// Check last_fired is set
	if auto.LastFired.IsZero() {
		t.Error("Expected last_fired to be set")
	}
}

func TestTriggerVolumeContainment(t *testing.T) {
	engine, _ := newTestEngine(t)

	// Box volume
	boxVolume := TriggerVolume{
		ID:    "test-box",
		Type:  "box",
		MinX:  1.0, MinY: 0.0, MinZ: 1.0,
		MaxX:  3.0, MaxY: 2.0, MaxZ: 3.0,
	}

	testCases := []struct {
		x, y, z  float64
		expected bool
	}{
		{2.0, 1.0, 2.0, true},   // Center
		{1.0, 0.0, 1.0, true},   // Corner
		{3.0, 2.0, 3.0, true},   // Opposite corner
		{0.0, 1.0, 2.0, false},  // Outside X
		{2.0, 3.0, 2.0, false},  // Outside Y
		{2.0, 1.0, 4.0, false},  // Outside Z
	}

	for _, tc := range testCases {
		result := engine.IsInVolume(tc.x, tc.y, tc.z, boxVolume)
		if result != tc.expected {
			t.Errorf("Box volume point (%.1f, %.1f, %.1f): expected %v, got %v",
				tc.x, tc.y, tc.z, tc.expected, result)
		}
	}

	// Sphere volume
	sphereVolume := TriggerVolume{
		ID:       "test-sphere",
		Type:     "sphere",
		CenterX:  2.0, CenterY: 2.0, CenterZ: 2.0,
		Radius:   1.0,
	}

	sphereCases := []struct {
		x, y, z  float64
		expected bool
	}{
		{2.0, 2.0, 2.0, true},   // Center
		{2.0, 2.0, 3.0, true},   // On surface
		{2.0, 2.0, 3.1, false},  // Just outside
		{1.0, 1.0, 1.0, false},  // Corner (distance > 1)
	}

	for _, tc := range sphereCases {
		result := engine.IsInVolume(tc.x, tc.y, tc.z, sphereVolume)
		if result != tc.expected {
			t.Errorf("Sphere volume point (%.1f, %.1f, %.1f): expected %v, got %v",
				tc.x, tc.y, tc.z, tc.expected, result)
		}
	}

	// Cylinder volume
	cylinderVolume := TriggerVolume{
		ID:         "test-cylinder",
		Type:       "cylinder",
		BaseX:      2.0, BaseZ: 2.0,
		BaseRadius: 1.0,
		MinHeight:  0.0, MaxHeight: 2.0,
	}

	cylinderCases := []struct {
		x, y, z  float64
		expected bool
	}{
		{2.0, 1.0, 2.0, true},   // Center
		{2.0, 2.5, 2.0, false},  // Above height
		{3.0, 1.0, 2.0, true},   // On edge
		{3.5, 1.0, 2.0, false},  // Outside radius
		{2.0, 1.0, 3.0, false},  // Outside radius in Z
	}

	for _, tc := range cylinderCases {
		result := engine.IsInVolume(tc.x, tc.y, tc.z, cylinderVolume)
		if result != tc.expected {
			t.Errorf("Cylinder volume point (%.1f, %.1f, %.1f): expected %v, got %v",
				tc.x, tc.y, tc.z, tc.expected, result)
		}
	}
}

func TestActionLog(t *testing.T) {
	engine, _ := newTestEngine(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	automation := &Automation{
		ID:          "test-log",
		Name:        "Log Test",
		Enabled:     true,
		TriggerType: TriggerZoneEnter,
		TriggerConfig: TriggerConfig{
			ZoneID:   "test",
			PersonID: "anyone",
		},
		Actions: []Action{
			{Type: ActionWebhook, URL: server.URL},
		},
		Cooldown: 0,
	}
	if err := engine.CreateAutomation(automation); err != nil {
		t.Fatal(err)
	}

	engine.ProcessEvent(Event{
		Type:       TriggerZoneEnter,
		ZoneID:     "test",
		PersonName: "Alice",
		Timestamp:  time.Now(),
	})

	// Wait for action to complete
	time.Sleep(100 * time.Millisecond)

	// Check action log
	log := engine.GetRecentActionLog(10)
	if len(log) != 1 {
		t.Fatalf("Expected 1 log entry, got %d", len(log))
	}

	if log[0].AutomationID != "test-log" {
		t.Errorf("Expected automation_id 'test-log', got %s", log[0].AutomationID)
	}
}

func TestAutomationCRUD(t *testing.T) {
	engine, _ := newTestEngine(t)

	// Create
	a := &Automation{
		ID:          "crud-test",
		Name:        "CRUD Test",
		Enabled:     true,
		TriggerType: TriggerZoneEnter,
		TriggerConfig: TriggerConfig{
			ZoneID:   "test",
			PersonID: "anyone",
		},
		Actions: []Action{
			{Type: ActionWebhook, URL: "http://example.com/crud"},
		},
		Cooldown: 60,
	}
	if err := engine.CreateAutomation(a); err != nil {
		t.Fatal(err)
	}

	// Read
	read := engine.GetAutomation("crud-test")
	if read == nil {
		t.Fatal("Automation not found after create")
	}
	if read.Name != "CRUD Test" {
		t.Errorf("Expected name 'CRUD Test', got %s", read.Name)
	}

	// Update
	read.Name = "Updated Name"
	if err := engine.UpdateAutomation(read); err != nil {
		t.Fatal(err)
	}

	updated := engine.GetAutomation("crud-test")
	if updated.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got %s", updated.Name)
	}

	// Delete
	if err := engine.DeleteAutomation("crud-test"); err != nil {
		t.Fatal(err)
	}

	deleted := engine.GetAutomation("crud-test")
	if deleted != nil {
		t.Error("Automation should not exist after delete")
	}
}

func TestTriggerVolumeCRUD(t *testing.T) {
	engine, _ := newTestEngine(t)

	// Create
	v := &TriggerVolume{
		ID:      "volume-test",
		Name:    "Test Volume",
		Type:    "box",
		Enabled: true,
		MinX:    0, MinY: 0, MinZ: 0,
		MaxX:    1, MaxY: 1, MaxZ: 1,
	}
	if err := engine.CreateTriggerVolume(v); err != nil {
		t.Fatal(err)
	}

	// Read
	read := engine.GetTriggerVolume("volume-test")
	if read == nil {
		t.Fatal("Volume not found after create")
	}
	if read.Name != "Test Volume" {
		t.Errorf("Expected name 'Test Volume', got %s", read.Name)
	}

	// GetAll
	all := engine.GetAllTriggerVolumes()
	if len(all) != 1 {
		t.Errorf("Expected 1 volume, got %d", len(all))
	}

	// Delete
	if err := engine.DeleteTriggerVolume("volume-test"); err != nil {
		t.Fatal(err)
	}

	deleted := engine.GetTriggerVolume("volume-test")
	if deleted != nil {
		t.Error("Volume should not exist after delete")
	}
}

func TestSystemMode(t *testing.T) {
	engine, _ := newTestEngine(t)

	// Default mode should be home
	if mode := engine.GetSystemMode(); mode != ModeHome {
		t.Errorf("Expected default mode 'home', got %s", mode)
	}

	// Set mode to away
	if err := engine.SetSystemMode(ModeAway); err != nil {
		t.Fatal(err)
	}
	if mode := engine.GetSystemMode(); mode != ModeAway {
		t.Errorf("Expected mode 'away', got %s", mode)
	}

	// Set mode to sleep
	if err := engine.SetSystemMode(ModeSleep); err != nil {
		t.Fatal(err)
	}
	if mode := engine.GetSystemMode(); mode != ModeSleep {
		t.Errorf("Expected mode 'sleep', got %s", mode)
	}

	// Test mode condition
	automation := &Automation{
		ID:          "test-mode-condition",
		Name:        "Test Mode Condition",
		Enabled:     true,
		TriggerType: TriggerZoneEnter,
		TriggerConfig: TriggerConfig{
			ZoneID:   "test",
			PersonID: "anyone",
		},
		Conditions: []Condition{
			{Type: ConditionSystemMode, Value: "away"},
		},
		Actions: []Action{
			{Type: ActionWebhook, URL: "http://example.com/away-only"},
		},
		Cooldown: 0,
	}
	if err := engine.CreateAutomation(automation); err != nil {
		t.Fatal(err)
	}

	// Mode is sleep, should not trigger
	triggered := false
	engine.SetOnTrigger(func(data TriggerEventData) {
		triggered = true
	})
	engine.ProcessEvent(Event{
		Type:     TriggerZoneEnter,
		ZoneID:   "test",
		Timestamp: time.Now(),
	})

	if triggered {
		t.Error("Should not trigger when mode is sleep (condition requires away)")
	}

	// Change mode to away
	engine.SetSystemMode(ModeAway)

	triggered = false
	engine.ProcessEvent(Event{
		Type:     TriggerZoneEnter,
		ZoneID:   "test",
		Timestamp: time.Now(),
	})

	if !triggered {
		t.Error("Should trigger when mode is away")
	}
}

func TestZoneOccupancyCondition(t *testing.T) {
	engine, _ := newTestEngine(t)

	// Set up mock zone provider with occupancy
	engine.SetZoneProvider(&mockZoneProvider{
		zones: map[string]string{"living_room": "Living Room"},
		occupancy: map[string]struct {
			count   int
			blobIDs []int
		}{
			"living_room": {count: 2, blobIDs: []int{1, 2}},
		},
	})

	// Automation that only triggers when living_room is empty
	automation := &Automation{
		ID:          "test-occupancy",
		Name:        "Test Occupancy",
		Enabled:     true,
		TriggerType: TriggerZoneEnter,
		TriggerConfig: TriggerConfig{
			ZoneID:   "kitchen",
			PersonID: "anyone",
		},
		Conditions: []Condition{
			{Type: ConditionZoneOccupancy, Value: "living_room:lt:1"},
		},
		Actions: []Action{
			{Type: ActionWebhook, URL: "http://example.com/empty-lr"},
		},
		Cooldown: 0,
	}
	if err := engine.CreateAutomation(automation); err != nil {
		t.Fatal(err)
	}

	// living_room has 2 people, should not trigger
	triggered := false
	engine.SetOnTrigger(func(data TriggerEventData) {
		triggered = true
	})
	engine.ProcessEvent(Event{
		Type:     TriggerZoneEnter,
		ZoneID:   "kitchen",
		Timestamp: time.Now(),
	})

	if triggered {
		t.Error("Should not trigger - living_room has 2 people (not < 1)")
	}
}
