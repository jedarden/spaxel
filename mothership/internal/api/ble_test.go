// Package api provides REST API tests for Spaxel BLE device endpoints.
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi"
	"github.com/spaxel/mothership/internal/ble"
)

// newTestBLEHandler creates a BLE handler backed by a temporary database.
func newTestBLEHandler(t *testing.T) (*ble.Handler, *ble.Registry, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "ble.db")

	registry, err := ble.NewRegistry(dbPath)
	if err != nil {
		t.Fatalf("Failed to create BLE registry: %v", err)
	}

	handler := ble.NewHandler(registry)
	cleanup := func() { registry.Close() }

	return handler, registry, cleanup
}

// setupBLERouter creates a chi.Router with all BLE routes registered.
func setupBLERouter(h *ble.Handler) *chi.Mux {
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return r
}

// TestListBLEDevices tests GET /api/ble/devices.
func TestListBLEDevices(t *testing.T) {
	h, registry, cleanup := newTestBLEHandler(t)
	defer cleanup()

	// Seed some test devices via ProcessRelayMessage
	registry.ProcessRelayMessage("AA:BB:CC:DD:EE:01", []ble.BLEObservation{
		{Addr: "AA:BB:CC:DD:EE:01", Name: "iPhone", MfrID: 0x004C, MfrDataHex: "0215", RSSIdBm: -45},
	})
	registry.ProcessRelayMessage("AA:BB:CC:DD:EE:02", []ble.BLEObservation{
		{Addr: "AA:BB:CC:DD:EE:02", Name: "Samsung", MfrID: 0x0075, MfrDataHex: "", RSSIdBm: -60},
	})

	// Create a person and assign one device
	person, err := registry.CreatePerson("Alice", "#ff0000")
	if err != nil {
		t.Fatalf("CreatePerson: %v", err)
	}
	registry.UpdateDevice("AA:BB:CC:DD:EE:01", map[string]interface{}{
		"person_id": person.ID,
		"name":      "Alice's Phone",
	})

	r := setupBLERouter(h)
	req := httptest.NewRequest("GET", "/api/ble/devices", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var result map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	devices, ok := result["devices"].([]interface{})
	if !ok {
		t.Fatal("Expected 'devices' key in response")
	}
	if len(devices) != 2 {
		t.Fatalf("Expected 2 devices, got %d", len(devices))
	}

	// Check privacy notice header
	if privacyNotice := rr.Header().Get("X-Privacy-Notice"); privacyNotice == "" {
		t.Error("Expected X-Privacy-Notice header")
	}
}

// TestListBLEDevicesRegistered tests GET /api/ble/devices?registered=true.
func TestListBLEDevicesRegistered(t *testing.T) {
	h, registry, cleanup := newTestBLEHandler(t)
	defer cleanup()

	// Seed devices
	registry.ProcessRelayMessage("AA:BB:CC:DD:EE:01", []ble.BLEObservation{
		{Addr: "AA:BB:CC:DD:EE:01", Name: "iPhone", MfrID: 0x004C, RSSIdBm: -45},
	})
	registry.ProcessRelayMessage("AA:BB:CC:DD:EE:02", []ble.BLEObservation{
		{Addr: "AA:BB:CC:DD:EE:02", Name: "Unknown", MfrID: 0, RSSIdBm: -60},
	})

	// Create a person and assign one device
	person, _ := registry.CreatePerson("Alice", "#ff0000")
	registry.UpdateDevice("AA:BB:CC:DD:EE:01", map[string]interface{}{
		"person_id": person.ID,
		"name":      "Alice's Phone",
	})

	r := setupBLERouter(h)
	req := httptest.NewRequest("GET", "/api/ble/devices?registered=true", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var result map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&result)
	devices := result["devices"].([]interface{})

	// Should only return the registered device
	if len(devices) != 1 {
		t.Fatalf("Expected 1 registered device, got %d", len(devices))
	}
}

// TestListBLEDevicesDiscovered tests GET /api/ble/devices?discovered=true.
func TestListBLEDevicesDiscovered(t *testing.T) {
	h, registry, cleanup := newTestBLEHandler(t)
	defer cleanup()

	// Seed devices
	registry.ProcessRelayMessage("AA:BB:CC:DD:EE:01", []ble.BLEObservation{
		{Addr: "AA:BB:CC:DD:EE:01", Name: "iPhone", MfrID: 0x004C, RSSIdBm: -45},
	})
	registry.ProcessRelayMessage("AA:BB:CC:DD:EE:02", []ble.BLEObservation{
		{Addr: "AA:BB:CC:DD:EE:02", Name: "Unknown", MfrID: 0, RSSIdBm: -60},
	})

	// Create a person and assign one device
	person, _ := registry.CreatePerson("Alice", "#ff0000")
	registry.UpdateDevice("AA:BB:CC:DD:EE:01", map[string]interface{}{
		"person_id": person.ID,
	})

	r := setupBLERouter(h)
	req := httptest.NewRequest("GET", "/api/ble/devices?discovered=true", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var result map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&result)
	devices := result["devices"].([]interface{})

	// Should only return unregistered devices
	if len(devices) != 1 {
		t.Fatalf("Expected 1 unregistered device, got %d", len(devices))
	}
}

// TestListBLEDevicesEmpty tests GET /api/ble/devices with no devices.
func TestListBLEDevicesEmpty(t *testing.T) {
	h, _, cleanup := newTestBLEHandler(t)
	defer cleanup()

	r := setupBLERouter(h)
	req := httptest.NewRequest("GET", "/api/ble/devices", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", rr.Code)
	}

	var result map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&result)
	devices := result["devices"].([]interface{})
	if len(devices) != 0 {
		t.Errorf("Expected 0 devices, got %d", len(devices))
	}
}

// TestGetBLEDevice tests GET /api/ble/devices/{mac}.
func TestGetBLEDevice(t *testing.T) {
	h, registry, cleanup := newTestBLEHandler(t)
	defer cleanup()

	// Seed a device
	registry.ProcessRelayMessage("AA:BB:CC:DD:EE:01", []ble.BLEObservation{
		{Addr: "AA:BB:CC:DD:EE:01", Name: "iPhone", MfrID: 0x004C, MfrDataHex: "0215", RSSIdBm: -45},
	})

	r := setupBLERouter(h)
	req := httptest.NewRequest("GET", "/api/ble/devices/AA:BB:CC:DD:EE:01", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var device ble.DeviceRecord
	if err := json.NewDecoder(rr.Body).Decode(&device); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if device.Addr != "AA:BB:CC:DD:EE:01" {
		t.Errorf("Expected MAC AA:BB:CC:DD:EE:01, got %s", device.Addr)
	}
	if device.DeviceName != "iPhone" {
		t.Errorf("Expected name 'iPhone', got %s", device.DeviceName)
	}
}

// TestGetBLEDeviceNotFound tests GET /api/ble/devices/{mac} for nonexistent device.
func TestGetBLEDeviceNotFound(t *testing.T) {
	h, _, cleanup := newTestBLEHandler(t)
	defer cleanup()

	r := setupBLERouter(h)
	req := httptest.NewRequest("GET", "/api/ble/devices/AA:BB:CC:DD:EE:FF", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", rr.Code)
	}
}

// TestUpdateBLEDevice tests PUT /api/ble/devices/{mac}.
func TestUpdateBLEDevice(t *testing.T) {
	tests := []struct {
		name       string
		mac        string
		body       string
		wantStatus int
		wantLabel  string
		wantPerson string
	}{
		{
			name: "update label only",
			mac:  "AA:BB:CC:DD:EE:01",
			body: `{"label": "Alice's iPhone"}`,
			wantStatus: http.StatusOK,
			wantLabel: "Alice's iPhone",
		},
		{
			name: "update device type",
			mac:  "AA:BB:CC:DD:EE:02",
			body: `{"device_type": "apple_phone"}`,
			wantStatus: http.StatusOK,
		},
		{
			name: "update all fields",
			mac:  "AA:BB:CC:DD:EE:03",
			body: `{"label": "Bob's Phone", "device_type": "samsung"}`,
			wantStatus: http.StatusOK,
			wantLabel: "Bob's Phone",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, registry, cleanup := newTestBLEHandler(t)
			defer cleanup()

			// Seed a device
			registry.ProcessRelayMessage(tt.mac, []ble.BLEObservation{
				{Addr: tt.mac, Name: "Device", MfrID: 0x004C, RSSIdBm: -50},
			})

			r := setupBLERouter(h)
			req := httptest.NewRequest("PUT", "/api/ble/devices/"+tt.mac, bytes.NewReader([]byte(tt.body)))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("Expected %d, got %d: %s", tt.wantStatus, rr.Code, rr.Body.String())
			}

			var device ble.DeviceRecord
			if err := json.NewDecoder(rr.Body).Decode(&device); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if tt.wantLabel != "" && device.Label != tt.wantLabel {
				t.Errorf("Expected label %q, got %q", tt.wantLabel, device.Label)
			}
		})
	}
}

// TestUpdateBLEDeviceAssignToPerson tests PUT /api/ble/devices/{mac} with person assignment.
func TestUpdateBLEDeviceAssignToPerson(t *testing.T) {
	h, registry, cleanup := newTestBLEHandler(t)
	defer cleanup()

	// Seed a device and create a person
	registry.ProcessRelayMessage("AA:BB:CC:DD:EE:01", []ble.BLEObservation{
		{Addr: "AA:BB:CC:DD:EE:01", Name: "iPhone", MfrID: 0x004C, RSSIdBm: -45},
	})

	person, err := registry.CreatePerson("Alice", "#ff0000")
	if err != nil {
		t.Fatalf("CreatePerson: %v", err)
	}

	r := setupBLERouter(h)
	body := `{"label": "Alice's Phone", "person_id": "` + person.ID + `"}`
	req := httptest.NewRequest("PUT", "/api/ble/devices/AA:BB:CC:DD:EE:01", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var device ble.DeviceRecord
	json.NewDecoder(rr.Body).Decode(&device)

	if device.Label != "Alice's Phone" {
		t.Errorf("Expected label 'Alice's Phone', got %s", device.Label)
	}
	if device.PersonID != person.ID {
		t.Errorf("Expected person_id %s, got %s", person.ID, device.PersonID)
	}

	// Verify persistence via GET
	req2 := httptest.NewRequest("GET", "/api/ble/devices/AA:BB:CC:DD:EE:01", nil)
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, req2)

	var device2 ble.DeviceRecord
	json.NewDecoder(rr2.Body).Decode(&device2)

	if device2.Label != "Alice's Phone" {
		t.Errorf("After GET: expected label 'Alice's Phone', got %s", device2.Label)
	}
	if device2.PersonID != person.ID {
		t.Errorf("After GET: expected person_id %s, got %s", person.ID, device2.PersonID)
	}
}

// TestUpdateBLEDeviceInvalidPerson tests PUT /api/ble/devices/{mac} with invalid person_id.
func TestUpdateBLEDeviceInvalidPerson(t *testing.T) {
	h, registry, cleanup := newTestBLEHandler(t)
	defer cleanup()

	// Seed a device
	registry.ProcessRelayMessage("AA:BB:CC:DD:EE:01", []ble.BLEObservation{
		{Addr: "AA:BB:CC:DD:EE:01", Name: "iPhone", MfrID: 0x004C, RSSIdBm: -45},
	})

	r := setupBLERouter(h)
	body := `{"person_id": "nonexistent-person-id"}`
	req := httptest.NewRequest("PUT", "/api/ble/devices/AA:BB:CC:DD:EE:01", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for invalid person_id, got %d", rr.Code)
	}
}

// TestUpdateBLEDeviceNotFound tests PUT /api/ble/devices/{mac} for nonexistent device.
func TestUpdateBLEDeviceNotFound(t *testing.T) {
	h, _, cleanup := newTestBLEHandler(t)
	defer cleanup()

	r := setupBLERouter(h)
	body := `{"label": "Test"}`
	req := httptest.NewRequest("PUT", "/api/ble/devices/AA:BB:CC:DD:EE:FF", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", rr.Code)
	}
}

// TestUpdateBLEDeviceInvalid tests PUT /api/ble/devices/{mac} with invalid input.
func TestUpdateBLEDeviceInvalid(t *testing.T) {
	h, registry, cleanup := newTestBLEHandler(t)
	defer cleanup()

	// Seed a device
	registry.ProcessRelayMessage("AA:BB:CC:DD:EE:01", []ble.BLEObservation{
		{Addr: "AA:BB:CC:DD:EE:01", Name: "iPhone", MfrID: 0x004C, RSSIdBm: -45},
	})

	tests := []struct {
		name string
		body string
		want int
	}{
		{"malformed JSON", `{bad`, http.StatusBadRequest},
		{"empty body", ``, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := setupBLERouter(h)
			req := httptest.NewRequest("PUT", "/api/ble/devices/AA:BB:CC:DD:EE:01", bytes.NewReader([]byte(tt.body)))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != tt.want {
				t.Errorf("Expected %d, got %d", tt.want, rr.Code)
			}
		})
	}
}

// TestDeleteBLEDevice tests DELETE /api/ble/devices/{mac}.
func TestDeleteBLEDevice(t *testing.T) {
	h, registry, cleanup := newTestBLEHandler(t)
	defer cleanup()

	// Seed devices
	registry.ProcessRelayMessage("AA:BB:CC:DD:EE:01", []ble.BLEObservation{
		{Addr: "AA:BB:CC:DD:EE:01", Name: "Device1", MfrID: 0x004C, RSSIdBm: -45},
	})
	registry.ProcessRelayMessage("AA:BB:CC:DD:EE:02", []ble.BLEObservation{
		{Addr: "AA:BB:CC:DD:EE:02", Name: "Device2", MfrID: 0x004C, RSSIdBm: -50},
	})

	r := setupBLERouter(h)
	req := httptest.NewRequest("DELETE", "/api/ble/devices/AA:BB:CC:DD:EE:01", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("Expected 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify device is archived
	device, err := registry.GetDevice("AA:BB:CC:DD:EE:01")
	if err != nil {
		t.Fatal("Device should still exist (archived)")
	}
	if !device.IsArchived {
		t.Error("Device should be archived")
	}

	// Verify other device still exists
	device2, _ := registry.GetDevice("AA:BB:CC:DD:EE:02")
	if device2 == nil {
		t.Error("Other device should still exist")
	}
	if device2.IsArchived {
		t.Error("Other device should not be archived")
	}
}

// TestDeleteBLEDeviceNotFound tests DELETE /api/ble/devices/{mac} for nonexistent device.
func TestDeleteBLEDeviceNotFound(t *testing.T) {
	h, _, cleanup := newTestBLEHandler(t)
	defer cleanup()

	r := setupBLERouter(h)
	req := httptest.NewRequest("DELETE", "/api/ble/devices/AA:BB:CC:DD:EE:FF", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", rr.Code)
	}
}

// TestPreregisterBLEDevice tests POST /api/ble/devices/preregister.
func TestPreregisterBLEDevice(t *testing.T) {
	h, _, cleanup := newTestBLEHandler(t)
	defer cleanup()

	r := setupBLERouter(h)
	body := `{"mac": "AA:BB:CC:DD:EE:FF", "label": "My Tile Tracker"}`
	req := httptest.NewRequest("POST", "/api/ble/devices/preregister", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("Expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var device ble.DeviceRecord
	if err := json.NewDecoder(rr.Body).Decode(&device); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if device.Addr != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("Expected MAC AA:BB:CC:DD:EE:FF, got %s", device.Addr)
	}
	if device.Label != "My Tile Tracker" {
		t.Errorf("Expected label 'My Tile Tracker', got %s", device.Label)
	}
}

// TestPreregisterBLEDeviceInvalid tests POST /api/ble/devices/preregister with invalid input.
func TestPreregisterBLEDeviceInvalid(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantMsg string
	}{
		{
			name:    "missing MAC",
			body:    `{"label": "Test"}`,
			wantMsg: "mac is required",
		},
		{
			name:    "malformed JSON",
			body:    `{invalid}`,
			wantMsg: "invalid request body",
		},
		{
			name:    "empty body",
			body:    ``,
			wantMsg: "invalid request body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _, cleanup := newTestBLEHandler(t)
			defer cleanup()

			r := setupBLERouter(h)
			req := httptest.NewRequest("POST", "/api/ble/devices/preregister", bytes.NewReader([]byte(tt.body)))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("Expected 400, got %d", rr.Code)
			}
		})
	}
}

// TestGetDeviceHistory tests GET /api/ble/devices/{mac}/history.
func TestGetDeviceHistory(t *testing.T) {
	h, registry, cleanup := newTestBLEHandler(t)
	defer cleanup()

	// Seed a device with multiple observations
	mac := "AA:BB:CC:DD:EE:01"
	registry.ProcessRelayMessage("node1", []ble.BLEObservation{
		{Addr: mac, Name: "iPhone", MfrID: 0x004C, RSSIdBm: -45},
	})
	registry.ProcessRelayMessage("node2", []ble.BLEObservation{
		{Addr: mac, Name: "iPhone", MfrID: 0x004C, RSSIdBm: -55},
	})

	r := setupBLERouter(h)
	req := httptest.NewRequest("GET", "/api/ble/devices/"+mac+"/history", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var result map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&result)

	if result["mac"] != mac {
		t.Errorf("Expected mac %s, got %v", mac, result["mac"])
	}

	history, ok := result["history"].([]interface{})
	if !ok {
		t.Fatal("Expected 'history' key in response")
	}
	// Should have at least 2 sightings
	if len(history) < 2 {
		t.Errorf("Expected at least 2 history entries, got %d", len(history))
	}
}

// TestGetDeviceHistoryNotFound tests GET /api/ble/devices/{mac}/history for nonexistent device.
func TestGetDeviceHistoryNotFound(t *testing.T) {
	h, _, cleanup := newTestBLEHandler(t)
	defer cleanup()

	r := setupBLERouter(h)
	req := httptest.NewRequest("GET", "/api/ble/devices/AA:BB:CC:DD:EE:FF/history", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", rr.Code)
	}
}

// TestListPeople tests GET /api/people.
func TestListPeople(t *testing.T) {
	h, registry, cleanup := newTestBLEHandler(t)
	defer cleanup()

	// Create people
	registry.CreatePerson("Alice", "#ff0000")
	registry.CreatePerson("Bob", "#0000ff")

	r := setupBLERouter(h)
	req := httptest.NewRequest("GET", "/api/people", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var people []map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&people)

	if len(people) != 2 {
		t.Fatalf("Expected 2 people, got %d", len(people))
	}
}

// TestCreatePerson tests POST /api/people.
func TestCreatePerson(t *testing.T) {
	h, _, cleanup := newTestBLEHandler(t)
	defer cleanup()

	r := setupBLERouter(h)
	body := `{"name": "Charlie", "color": "#00ff00"}`
	req := httptest.NewRequest("POST", "/api/people", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("Expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var person map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&person)

	if person["name"] != "Charlie" {
		t.Errorf("Expected name 'Charlie', got %v", person["name"])
	}
	if person["color"] != "#00ff00" {
		t.Errorf("Expected color '#00ff00', got %v", person["color"])
	}
	if person["id"] == nil || person["id"] == "" {
		t.Error("Expected non-empty id")
	}
}

// TestCreatePersonDefaultColor tests POST /api/people with default color.
func TestCreatePersonDefaultColor(t *testing.T) {
	h, _, cleanup := newTestBLEHandler(t)
	defer cleanup()

	r := setupBLERouter(h)
	body := `{"name": "Dana"}`
	req := httptest.NewRequest("POST", "/api/people", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("Expected 201, got %d", rr.Code)
	}

	var person map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&person)

	// Default color should be #3b82f6
	if person["color"] != "#3b82f6" {
		t.Errorf("Expected default color '#3b82f6', got %v", person["color"])
	}
}

// TestGetPerson tests GET /api/people/{id}.
func TestGetPerson(t *testing.T) {
	h, registry, cleanup := newTestBLEHandler(t)
	defer cleanup()

	person, _ := registry.CreatePerson("Alice", "#ff0000")

	r := setupBLERouter(h)
	req := httptest.NewRequest("GET", "/api/people/"+person.ID, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var result map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&result)

	if result["name"] != "Alice" {
		t.Errorf("Expected name 'Alice', got %v", result["name"])
	}
	if result["id"] != person.ID {
		t.Errorf("Expected id %s, got %v", person.ID, result["id"])
	}
}

// TestUpdatePerson tests PUT /api/people/{id}.
func TestUpdatePerson(t *testing.T) {
	h, registry, cleanup := newTestBLEHandler(t)
	defer cleanup()

	person, _ := registry.CreatePerson("Alice", "#ff0000")

	r := setupBLERouter(h)
	body := `{"name": "Alice Smith", "color": "#ff5500"}`
	req := httptest.NewRequest("PUT", "/api/people/"+person.ID, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var result map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&result)

	if result["name"] != "Alice Smith" {
		t.Errorf("Expected name 'Alice Smith', got %v", result["name"])
	}
	if result["color"] != "#ff5500" {
		t.Errorf("Expected color '#ff5500', got %v", result["color"])
	}
}

// TestDeletePerson tests DELETE /api/people/{id}.
func TestDeletePerson(t *testing.T) {
	h, registry, cleanup := newTestBLEHandler(t)
	defer cleanup()

	person, _ := registry.CreatePerson("Alice", "#ff0000")

	r := setupBLERouter(h)
	req := httptest.NewRequest("DELETE", "/api/people/"+person.ID, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("Expected 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify person is deleted
	_, err := registry.GetPerson(person.ID)
	if err == nil {
		t.Error("Person should be deleted")
	}
}
