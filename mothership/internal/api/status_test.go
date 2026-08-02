// Package api provides tests for the system status API handler.
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/spaxel/mothership/internal/zones"
)

// statusResponse is the JSON shape returned by GET /api/status.
// Mirrors the map written by StatusHandler.getStatus.
type statusResponse struct {
	Version          string `json:"version"`
	Nodes            int    `json:"nodes"`
	Blobs            int    `json:"blobs"`
	UptimeS          int64  `json:"uptime_s"`
	DetectionQuality int    `json:"detection_quality"`
}

// newStatusRouter wires a StatusHandler into a chi router for handler-level tests.
func newStatusRouter(h *StatusHandler) http.Handler {
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return r
}

// TestStatusHandlerVersionWired is the regression test for the hardcoded
// "1.0.0" version string (bf-5hz): the version passed to NewStatusHandler —
// which mirrors the build-time `-X main.version` ldflag injected in
// cmd/mothership/main.go — must be the value surfaced at GET /api/status, not
// a constant. Table-driven across representative version strings.
func TestStatusHandlerVersionWired(t *testing.T) {
	tests := []struct {
		name    string
		version string
	}{
		{"release_semver", "0.1.357"},
		{"dev_build", "dev"},
		{"dirty_tree", "0.1.358-dirty"},
		{"empty_string", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewStatusHandler(time.Now(), func() int { return 0 }, tt.version)
			server := newStatusRouter(h)

			req := httptest.NewRequest("GET", "/api/status", nil)
			w := httptest.NewRecorder()
			server.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d", w.Code)
			}

			var resp statusResponse
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil { //nolint:errcheck
				t.Fatalf("failed to decode response: %v", err)
			}

			if resp.Version != tt.version {
				t.Errorf("version = %q, want %q (must not be hardcoded \"1.0.0\")", resp.Version, tt.version)
			}
		})
	}
}

// TestStatusHandlerFields verifies the non-version fields of GET /api/status:
// node count is sourced from the callback, uptime is non-negative, and a nil
// processor manager yields zero blobs and zero detection quality.
func TestStatusHandlerFields(t *testing.T) {
	start := time.Now().Add(-30 * time.Second)
	h := NewStatusHandler(start, func() int { return 4 }, "0.1.357")
	server := newStatusRouter(h)

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp statusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil { //nolint:errcheck
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Nodes != 4 {
		t.Errorf("nodes = %d, want 4", resp.Nodes)
	}
	if resp.Blobs != 0 {
		t.Errorf("blobs = %d, want 0 (no processor manager)", resp.Blobs)
	}
	if resp.UptimeS < 0 {
		t.Errorf("uptime_s = %d, want >= 0", resp.UptimeS)
	}
	if resp.DetectionQuality != 0 {
		t.Errorf("detection_quality = %d, want 0 (no processor manager)", resp.DetectionQuality)
	}
}

// TestStatusHandlerNilNodeCountCallback confirms a nil getNodeCount callback
// is tolerated and reports zero nodes rather than panicking.
func TestStatusHandlerNilNodeCountCallback(t *testing.T) {
	h := NewStatusHandler(time.Now(), nil, "0.1.357")
	server := newStatusRouter(h)

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp statusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil { //nolint:errcheck
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Nodes != 0 {
		t.Errorf("nodes = %d, want 0 for nil callback", resp.Nodes)
	}
}

// mockIdentityProvider is a mock implementation of IdentityProvider for testing.
type mockIdentityProvider struct {
	matches map[int]*IdentityMatch
}

func (m *mockIdentityProvider) GetMatch(blobID int) *IdentityMatch {
	return m.matches[blobID]
}

// mockZonesManager is a mock implementation of ZonesManagerProvider for testing.
type mockZonesManager struct {
	zones     []*zones.Zone
	occupancy map[string]*zones.ZoneOccupancy
}

func (m *mockZonesManager) GetAllZones() []*zones.Zone {
	return m.zones
}

func (m *mockZonesManager) GetZoneOccupancy(zoneID string) *zones.ZoneOccupancy {
	return m.occupancy[zoneID]
}

// mockProcessorManager is a mock implementation of ProcessorManagerProvider for testing.
type mockProcessorManager struct {
	blobs   int
	quality float64
}

func (m *mockProcessorManager) GetSystemHealth() float64 {
	return m.quality
}

func (m *mockProcessorManager) GetTrackedBlobs() []struct {
	ID      int
	X, Y, Z float64
	Weight  float64
} {
	return []struct {
		ID      int
		X, Y, Z float64
		Weight  float64
	}{}
}

// TestOccupancyWithBLEIdentity verifies that a zone with a BLE-identified occupant
// returns that person's name in the People array.
func TestOccupancyWithBLEIdentity(t *testing.T) {
	h := NewStatusHandler(time.Now(), func() int { return 3 }, "0.1.357")

	// Set up mock zones manager with one zone and one occupant
	zm := &mockZonesManager{
		zones: []*zones.Zone{
			{ID: "zone1", Name: "Kitchen"},
		},
		occupancy: map[string]*zones.ZoneOccupancy{
			"zone1": {
				ZoneID:  "zone1",
				Count:   1,
				BlobIDs: []int{42},
			},
		},
	}
	h.SetZonesManager(zm)

	// Set up mock identity provider with a match for blob 42
	idp := &mockIdentityProvider{
		matches: map[int]*IdentityMatch{
			42: {
				PersonName: "Alice",
				PersonID:   "person-1",
				DeviceName: "iPhone",
				IsBLEOnly:  false,
			},
		},
	}
	h.SetIdentityProvider(idp)

	server := newStatusRouter(h)

	req := httptest.NewRequest("GET", "/api/occupancy", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var result map[string]occupancyResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil { //nolint:errcheck
		t.Fatalf("failed to decode response: %v", err)
	}

	kitchen, ok := result["Kitchen"]
	if !ok {
		t.Fatal("Kitchen zone missing from response")
	}

	if kitchen.Count != 1 {
		t.Errorf("Count = %d, want 1", kitchen.Count)
	}

	if len(kitchen.People) != 1 {
		t.Fatalf("People = %v (len=%d), want 1 person", kitchen.People, len(kitchen.People))
	}

	if kitchen.People[0] != "Alice" {
		t.Errorf("People[0] = %q, want \"Alice\"", kitchen.People[0])
	}
}

// TestOccupancyWithUnidentifiedBlob verifies that a zone with an unidentified
// blob (no BLE match) returns Count>0 but an empty People array.
func TestOccupancyWithUnidentifiedBlob(t *testing.T) {
	h := NewStatusHandler(time.Now(), func() int { return 3 }, "0.1.357")

	// Set up mock zones manager with one zone and one occupant
	zm := &mockZonesManager{
		zones: []*zones.Zone{
			{ID: "zone1", Name: "Living Room"},
		},
		occupancy: map[string]*zones.ZoneOccupancy{
			"zone1": {
				ZoneID:  "zone1",
				Count:   1,
				BlobIDs: []int{99},
			},
		},
	}
	h.SetZonesManager(zm)

	// Set up mock identity provider with NO match for blob 99
	idp := &mockIdentityProvider{
		matches: map[int]*IdentityMatch{},
	}
	h.SetIdentityProvider(idp)

	server := newStatusRouter(h)

	req := httptest.NewRequest("GET", "/api/occupancy", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var result map[string]occupancyResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil { //nolint:errcheck
		t.Fatalf("failed to decode response: %v", err)
	}

	livingRoom, ok := result["Living Room"]
	if !ok {
		t.Fatal("Living Room zone missing from response")
	}

	if livingRoom.Count != 1 {
		t.Errorf("Count = %d, want 1", livingRoom.Count)
	}

	if len(livingRoom.People) != 0 {
		t.Errorf("People = %v (len=%d), want empty array for unidentified blob", livingRoom.People, len(livingRoom.People))
	}
}

// TestOccupancyNilIdentityProvider verifies that a nil identity provider
// returns empty People arrays even when blobs are present.
func TestOccupancyNilIdentityProvider(t *testing.T) {
	h := NewStatusHandler(time.Now(), func() int { return 3 }, "0.1.357")

	// Set up mock zones manager with one zone and one occupant
	zm := &mockZonesManager{
		zones: []*zones.Zone{
			{ID: "zone1", Name: "Bedroom"},
		},
		occupancy: map[string]*zones.ZoneOccupancy{
			"zone1": {
				ZoneID:  "zone1",
				Count:   1,
				BlobIDs: []int{42},
			},
		},
	}
	h.SetZonesManager(zm)

	// Do NOT set identity provider - leave it as nil
	server := newStatusRouter(h)

	req := httptest.NewRequest("GET", "/api/occupancy", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var result map[string]occupancyResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil { //nolint:errcheck
		t.Fatalf("failed to decode response: %v", err)
	}

	bedroom, ok := result["Bedroom"]
	if !ok {
		t.Fatal("Bedroom zone missing from response")
	}

	if bedroom.Count != 1 {
		t.Errorf("Count = %d, want 1", bedroom.Count)
	}

	if len(bedroom.People) != 0 {
		t.Errorf("People = %v (len=%d), want empty array when no identity provider", bedroom.People, len(bedroom.People))
	}
}

// TestOccupancyMultiplePeople verifies that multiple identified people in
// a zone are all returned in the People array, with deduplication.
func TestOccupancyMultiplePeople(t *testing.T) {
	h := NewStatusHandler(time.Now(), func() int { return 3 }, "0.1.357")

	// Set up mock zones manager with one zone and three occupants
	zm := &mockZonesManager{
		zones: []*zones.Zone{
			{ID: "zone1", Name: "Kitchen"},
		},
		occupancy: map[string]*zones.ZoneOccupancy{
			"zone1": {
				ZoneID:  "zone1",
				Count:   3,
				BlobIDs: []int{42, 43, 44},
			},
		},
	}
	h.SetZonesManager(zm)

	// Set up mock identity provider with matches for all blobs
	// Blob 42 and 43 are the same person (Alice with two devices), blob 44 is Bob
	idp := &mockIdentityProvider{
		matches: map[int]*IdentityMatch{
			42: {
				PersonName: "Alice",
				PersonID:   "person-1",
				DeviceName: "iPhone",
			},
			43: {
				PersonName: "Alice",
				PersonID:   "person-1",
				DeviceName: "Apple Watch",
			},
			44: {
				PersonName: "Bob",
				PersonID:   "person-2",
				DeviceName: "Samsung Phone",
			},
		},
	}
	h.SetIdentityProvider(idp)

	server := newStatusRouter(h)

	req := httptest.NewRequest("GET", "/api/occupancy", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var result map[string]occupancyResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil { //nolint:errcheck
		t.Fatalf("failed to decode response: %v", err)
	}

	kitchen, ok := result["Kitchen"]
	if !ok {
		t.Fatal("Kitchen zone missing from response")
	}

	if kitchen.Count != 3 {
		t.Errorf("Count = %d, want 3", kitchen.Count)
	}

	// Should have exactly 2 people (Alice and Bob), deduplicated
	if len(kitchen.People) != 2 {
		t.Fatalf("People = %v (len=%d), want 2 people (Alice and Bob, deduplicated)", kitchen.People, len(kitchen.People))
	}

	// Check that both Alice and Bob are present
	hasAlice := false
	hasBob := false
	for _, person := range kitchen.People {
		if person == "Alice" {
			hasAlice = true
		}
		if person == "Bob" {
			hasBob = true
		}
	}

	if !hasAlice {
		t.Error("People array does not contain \"Alice\"")
	}
	if !hasBob {
		t.Error("People array does not contain \"Bob\"")
	}
}

// TestOccupancyMixedIdentity verifies that a zone with both identified and
// unidentified blobs returns only the identified people.
func TestOccupancyMixedIdentity(t *testing.T) {
	h := NewStatusHandler(time.Now(), func() int { return 3 }, "0.1.357")

	// Set up mock zones manager with one zone and three occupants
	zm := &mockZonesManager{
		zones: []*zones.Zone{
			{ID: "zone1", Name: "Hallway"},
		},
		occupancy: map[string]*zones.ZoneOccupancy{
			"zone1": {
				ZoneID:  "zone1",
				Count:   3,
				BlobIDs: []int{42, 43, 44}, // 42 and 43 are identified, 44 is not
			},
		},
	}
	h.SetZonesManager(zm)

	// Set up mock identity provider with matches for only blobs 42 and 43
	idp := &mockIdentityProvider{
		matches: map[int]*IdentityMatch{
			42: {
				PersonName: "Charlie",
				PersonID:   "person-3",
			},
			43: {
				PersonName: "Dana",
				PersonID:   "person-4",
			},
			// No match for blob 44
		},
	}
	h.SetIdentityProvider(idp)

	server := newStatusRouter(h)

	req := httptest.NewRequest("GET", "/api/occupancy", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var result map[string]occupancyResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil { //nolint:errcheck
		t.Fatalf("failed to decode response: %v", err)
	}

	hallway, ok := result["Hallway"]
	if !ok {
		t.Fatal("Hallway zone missing from response")
	}

	if hallway.Count != 3 {
		t.Errorf("Count = %d, want 3", hallway.Count)
	}

	// Should have exactly 2 people (Charlie and Dana), blob 44 is unidentified and omitted
	if len(hallway.People) != 2 {
		t.Fatalf("People = %v (len=%d), want 2 people (identified blobs only)", hallway.People, len(hallway.People))
	}
}
