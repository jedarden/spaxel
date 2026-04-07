package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi"
	"github.com/spaxel/mothership/internal/dashboard"
	"github.com/spaxel/mothership/internal/zones"
)

// newTestHandler creates a ZonesHandler backed by a temporary zones.Manager.
func newTestHandler(t *testing.T) (*ZonesHandler, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "zones.db")
	mgr, err := zones.NewManager(dbPath, nil)
	if err != nil {
		t.Fatalf("Failed to create zones manager: %v", err)
	}
	handler := NewZonesHandler(mgr)
	return handler, func() { mgr.Close() }
}

// setupRouter creates a chi.Router with all zones/portals routes registered.
func setupRouter(h *ZonesHandler) *chi.Mux {
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return r
}

// TestListZones tests GET /api/zones.
func TestListZones(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	// Seed two zones
	if err := h.mgr.CreateZone(&zones.Zone{
		ID: "z1", Name: "Kitchen", MinX: 0, MinY: 0, MinZ: 0,
		MaxX: 4, MaxY: 3, MaxZ: 2.5,
	}); err != nil {
		t.Fatalf("CreateZone: %v", err)
	}
	if err := h.mgr.CreateZone(&zones.Zone{
		ID: "z2", Name: "Bedroom", MinX: 4, MinY: 0, MinZ: 0,
		MaxX: 8, MaxY: 4, MaxZ: 2.5, ZoneType: zones.ZoneTypeBedroom,
	}); err != nil {
		t.Fatalf("CreateZone: %v", err)
	}

	r := setupRouter(h)
	req := httptest.NewRequest("GET", "/api/zones", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var result []zoneWithOcc
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("Expected 2 zones, got %d", len(result))
	}

	// Verify fields
	if result[0].ID != "z1" || result[0].Name != "Kitchen" {
		t.Errorf("Zone z1 mismatch: %+v", result[0])
	}
	if result[1].ID != "z2" || result[1].Name != "Bedroom" {
		t.Errorf("Zone z2 mismatch: %+v", result[1])
	}
	if result[1].ZoneType != "bedroom" {
		t.Errorf("Expected zone_type=bedroom, got %s", result[1].ZoneType)
	}

	// Occupancy defaults
	for _, z := range result {
		if z.Occupancy != 0 {
			t.Errorf("Zone %s: expected occupancy=0, got %d", z.ID, z.Occupancy)
		}
		if z.People == nil {
			t.Errorf("Zone %s: expected non-nil people", z.ID)
		}
	}

	// Verify computed width/depth/height
	if result[0].Width != 4 || result[0].Depth != 3 || result[0].Height != 2.5 {
		t.Errorf("Zone z1 dimensions wrong: w=%f d=%f h=%f", result[0].Width, result[0].Depth, result[0].Height)
	}
}

// TestListZonesEmpty tests GET /api/zones with no zones.
func TestListZonesEmpty(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	r := setupRouter(h)
	req := httptest.NewRequest("GET", "/api/zones", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", rr.Code)
	}

	var result []zoneWithOcc
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("Expected 0 zones, got %d", len(result))
	}
}

// TestCreateZone tests POST /api/zones.
func TestCreateZone(t *testing.T) {
	tests := []struct {
		name       string
		body       zones.Zone
		wantStatus int
		wantID     string
	}{
		{
			name: "create with explicit ID",
			body: zones.Zone{
				ID: "kitchen", Name: "Kitchen",
				MinX: 0, MinY: 0, MinZ: 0, MaxX: 4, MaxY: 3, MaxZ: 2.5,
			},
			wantStatus: http.StatusCreated,
			wantID:     "kitchen",
		},
		{
			name: "create with auto-generated ID",
			body: zones.Zone{
				Name: "Living Room",
				MinX: 4, MinY: 0, MinZ: 0, MaxX: 8, MaxY: 5, MaxZ: 2.5,
			},
			wantStatus: http.StatusCreated,
			wantID:     "", // auto-generated, check prefix in test
		},
		{
			name: "create bedroom zone",
			body: zones.Zone{
				ID: "bed1", Name: "Master Bedroom", ZoneType: zones.ZoneTypeBedroom,
				MinX: 0, MinY: 5, MinZ: 0, MaxX: 4, MaxY: 9, MaxZ: 2.5,
			},
			wantStatus: http.StatusCreated,
			wantID:     "bed1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, cleanup := newTestHandler(t)
			defer cleanup()

			r := setupRouter(h)
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest("POST", "/api/zones", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("Expected %d, got %d: %s", tt.wantStatus, rr.Code, rr.Body.String())
			}

			var created zoneWithOcc
			if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}
			// For auto-generated IDs, check prefix; otherwise check exact match
			if tt.wantID == "" {
				if !strings.HasPrefix(created.ID, "zone_") {
					t.Errorf("Expected ID starting with \"zone_\", got %q", created.ID)
				}
			} else {
				if created.ID != tt.wantID {
					t.Errorf("Expected ID %q, got %q", tt.wantID, created.ID)
				}
			}
			if created.CreatedAt.IsZero() {
				t.Error("Expected non-zero CreatedAt")
			}
		})
	}
}

// TestCreateZoneInvalid tests POST /api/zones with invalid input.
func TestCreateZoneInvalid(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantMsg string
	}{
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
			h, cleanup := newTestHandler(t)
			defer cleanup()

			r := setupRouter(h)
			req := httptest.NewRequest("POST", "/api/zones", bytes.NewReader([]byte(tt.body)))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("Expected 400, got %d", rr.Code)
			}

			var errResp map[string]string
			if err := json.NewDecoder(rr.Body).Decode(&errResp); err != nil {
				t.Fatalf("Failed to decode error: %v", err)
			}
			if errResp["error"] == "" {
				t.Error("Expected error message")
			}
		})
	}
}

// TestUpdateZone tests PUT /api/zones/{id}.
func TestUpdateZone(t *testing.T) {
	tests := []struct {
		name     string
		setup    zones.Zone
		update   zones.Zone
		wantName string
	}{
		{
			name:  "update zone name",
			setup: zones.Zone{ID: "z1", Name: "Kitchen", MinX: 0, MinY: 0, MinZ: 0, MaxX: 4, MaxY: 3, MaxZ: 2.5},
			update: zones.Zone{ID: "z1", Name: "Big Kitchen", MinX: 0, MinY: 0, MinZ: 0, MaxX: 6, MaxY: 5, MaxZ: 3},
			wantName: "Big Kitchen",
		},
		{
			name:  "update zone type to bedroom",
			setup: zones.Zone{ID: "z1", Name: "Room", MinX: 0, MinY: 0, MinZ: 0, MaxX: 4, MaxY: 3, MaxZ: 2.5},
			update: zones.Zone{ID: "z1", Name: "Room", MinX: 0, MinY: 0, MinZ: 0, MaxX: 4, MaxY: 3, MaxZ: 2.5, ZoneType: zones.ZoneTypeBedroom},
			wantName: "Room",
		},
		{
			name:  "update zone bounds",
			setup: zones.Zone{ID: "z1", Name: "Box", MinX: 0, MinY: 0, MinZ: 0, MaxX: 1, MaxY: 1, MaxZ: 1},
			update: zones.Zone{ID: "z1", Name: "Box", MinX: 2, MinY: 3, MinZ: 1, MaxX: 10, MaxY: 8, MaxZ: 4},
			wantName: "Box",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, cleanup := newTestHandler(t)
			defer cleanup()

			// Setup
			if err := h.mgr.CreateZone(&tt.setup); err != nil {
				t.Fatalf("CreateZone: %v", err)
			}

			// Update
			r := setupRouter(h)
			body, _ := json.Marshal(tt.update)
			req := httptest.NewRequest("PUT", "/api/zones/"+tt.setup.ID, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("Expected 200, got %d: %s", rr.Code, rr.Body.String())
			}

			var updated zoneWithOcc
			if err := json.NewDecoder(rr.Body).Decode(&updated); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}
			if updated.Name != tt.wantName {
				t.Errorf("Expected name %q, got %q", tt.wantName, updated.Name)
			}
			if updated.ID != tt.setup.ID {
				t.Errorf("Expected ID %q, got %q", tt.setup.ID, updated.ID)
			}

			// Verify the update persisted via GET
			req2 := httptest.NewRequest("GET", "/api/zones", nil)
			rr2 := httptest.NewRecorder()
			r.ServeHTTP(rr2, req2)
			var allZones []zoneWithOcc
			json.NewDecoder(rr2.Body).Decode(&allZones)
			found := false
			for _, z := range allZones {
				if z.ID == tt.setup.ID {
					found = true
					if z.Name != tt.wantName {
						t.Errorf("GET after PUT: expected name %q, got %q", tt.wantName, z.Name)
					}
				}
			}
			if !found {
				t.Error("Zone not found after update")
			}
		})
	}
}

// TestUpdateZoneInvalid tests PUT /api/zones/{id} with invalid input.
func TestUpdateZoneInvalid(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	// Setup a zone
	h.mgr.CreateZone(&zones.Zone{ID: "z1", Name: "Room", MinX: 0, MinY: 0, MinZ: 0, MaxX: 1, MaxY: 1, MaxZ: 1})

	tests := []struct {
		name string
		body string
		want int
	}{
		{"malformed JSON", `{bad}`, http.StatusBadRequest},
		{"empty body", ``, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := setupRouter(h)
			req := httptest.NewRequest("PUT", "/api/zones/z1", bytes.NewReader([]byte(tt.body)))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != tt.want {
				t.Errorf("Expected %d, got %d", tt.want, rr.Code)
			}
		})
	}
}

// TestUpdateZoneNotFound tests PUT /api/zones/{id} for nonexistent zone.
func TestUpdateZoneNotFound(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	r := setupRouter(h)
	body := `{"name": "Nope"}`
	req := httptest.NewRequest("PUT", "/api/zones/nonexistent", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", rr.Code)
	}
}

// TestDeleteZone tests DELETE /api/zones/{id}.
func TestDeleteZone(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	// Setup
	h.mgr.CreateZone(&zones.Zone{ID: "z1", Name: "Room", MinX: 0, MinY: 0, MinZ: 0, MaxX: 1, MaxY: 1, MaxZ: 1})
	h.mgr.CreateZone(&zones.Zone{ID: "z2", Name: "Room2", MinX: 2, MinY: 0, MinZ: 0, MaxX: 3, MaxY: 1, MaxZ: 1})

	r := setupRouter(h)

	// Delete z1
	req := httptest.NewRequest("DELETE", "/api/zones/z1", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("Expected 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify z1 is gone
	if h.mgr.GetZone("z1") != nil {
		t.Error("Zone z1 should be deleted")
	}

	// Verify z2 still exists
	if h.mgr.GetZone("z2") == nil {
		t.Error("Zone z2 should still exist")
	}

	// Verify via GET
	req2 := httptest.NewRequest("GET", "/api/zones", nil)
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, req2)
	var allZones []zoneWithOcc
	json.NewDecoder(rr2.Body).Decode(&allZones)
	if len(allZones) != 1 {
		t.Errorf("Expected 1 zone after delete, got %d", len(allZones))
	}
}

// TestDeleteZoneNotFound tests DELETE /api/zones/{id} for nonexistent zone.
func TestDeleteZoneNotFound(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	r := setupRouter(h)
	req := httptest.NewRequest("DELETE", "/api/zones/nonexistent", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Manager.DeleteZone returns nil error even if zone doesn't exist
	if rr.Code != http.StatusNoContent {
		t.Fatalf("Expected 204, got %d", rr.Code)
	}
}

// TestGetZoneHistory tests GET /api/zones/{id}/history.
func TestGetZoneHistory(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	h.mgr.CreateZone(&zones.Zone{ID: "z1", Name: "Room", MinX: 0, MinY: 0, MinZ: 0, MaxX: 1, MaxY: 1, MaxZ: 1})

	tests := []struct {
		name     string
		zoneID   string
		wantCode int
	}{
		{"existing zone", "z1", http.StatusOK},
		{"nonexistent zone", "nope", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := setupRouter(h)
			req := httptest.NewRequest("GET", "/api/zones/"+tt.zoneID+"/history", nil)
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != tt.wantCode {
				t.Errorf("Expected %d, got %d", tt.wantCode, rr.Code)
			}
		})
	}
}

// ── Portals ─────────────────────────────────────────────────────────────────────

// TestListPortals tests GET /api/portals.
func TestListPortals(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	// Seed zones for the portals
	h.mgr.CreateZone(&zones.Zone{ID: "z1", Name: "Kitchen", MinX: 0, MinY: 0, MinZ: 0, MaxX: 4, MaxY: 3, MaxZ: 2.5})
	h.mgr.CreateZone(&zones.Zone{ID: "z2", Name: "Hallway", MinX: 4, MinY: 0, MinZ: 0, MaxX: 8, MaxY: 3, MaxZ: 2.5})

	// Create a portal
	p := zones.Portal{
		ID: "p1", Name: "Kitchen Door", ZoneAID: "z1", ZoneBID: "z2",
		P1X: 4, P1Y: 0, P1Z: 0,
		P2X: 4, P2Y: 2, P2Z: 0,
		P3X: 4, P3Y: 2, P3Z: 2.5,
		Width: 2.5, Height: 2.5,
	}
	if err := h.mgr.CreatePortal(&p); err != nil {
		t.Fatalf("CreatePortal: %v", err)
	}

	r := setupRouter(h)
	req := httptest.NewRequest("GET", "/api/portals", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var result []portalWithZones
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("Expected 1 portal, got %d", len(result))
	}
	if result[0].ID != "p1" {
		t.Errorf("Expected portal ID p1, got %s", result[0].ID)
	}
	if result[0].Name != "Kitchen Door" {
		t.Errorf("Expected name 'Kitchen Door', got %s", result[0].Name)
	}
	// Normal vector should be computed
	if result[0].NX == 0 && result[0].NY == 0 && result[0].NZ == 0 {
		t.Error("Expected computed normal vector, got zero")
	}
}

// TestCreatePortal tests POST /api/portals.
func TestCreatePortal(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	// Seed zones
	h.mgr.CreateZone(&zones.Zone{ID: "z1", Name: "A", MinX: 0, MinY: 0, MinZ: 0, MaxX: 1, MaxY: 1, MaxZ: 1})
	h.mgr.CreateZone(&zones.Zone{ID: "z2", Name: "B", MinX: 1, MinY: 0, MinZ: 0, MaxX: 2, MaxY: 1, MaxZ: 1})

	p := zones.Portal{
		ID: "door1", Name: "A-B Door", ZoneAID: "z1", ZoneBID: "z2",
		P1X: 1, P1Y: 0, P1Z: 0,
		P2X: 1, P2Y: 0.5, P2Z: 0,
		P3X: 1, P3Y: 0.5, P3Z: 1,
		Width: 1, Height: 1,
	}

	r := setupRouter(h)
	body, _ := json.Marshal(p)
	req := httptest.NewRequest("POST", "/api/portals", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("Expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var created portalWithZones
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if created.ID != "door1" {
		t.Errorf("Expected ID 'door1', got %s", created.ID)
	}
	if created.CreatedAt.IsZero() {
		t.Error("Expected non-zero CreatedAt")
	}

	// Verify it persists
	portal := h.mgr.GetPortal("door1")
	if portal == nil {
		t.Fatal("Portal not found after creation")
	}
	if portal.Name != "A-B Door" {
		t.Errorf("Expected name 'A-B Door', got %s", portal.Name)
	}
}

// TestCreatePortalAutoID tests POST /api/portals with no ID.
func TestCreatePortalAutoID(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	h.mgr.CreateZone(&zones.Zone{ID: "z1", Name: "A", MinX: 0, MinY: 0, MinZ: 0, MaxX: 1, MaxY: 1, MaxZ: 1})
	h.mgr.CreateZone(&zones.Zone{ID: "z2", Name: "B", MinX: 1, MinY: 0, MinZ: 0, MaxX: 2, MaxY: 1, MaxZ: 1})

	p := zones.Portal{
		Name: "Auto Door", ZoneAID: "z1", ZoneBID: "z2",
		P1X: 1, P1Y: 0, P1Z: 0,
		P2X: 1, P2Y: 0.5, P2Z: 0,
		P3X: 1, P3Y: 0.5, P3Z: 1,
		Width: 1, Height: 1,
	}

	r := setupRouter(h)
	body, _ := json.Marshal(p)
	req := httptest.NewRequest("POST", "/api/portals", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("Expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var created portalWithZones
	json.NewDecoder(rr.Body).Decode(&created)
	if created.ID == "" {
		t.Error("Expected auto-generated ID, got empty")
	}
}

// TestCreatePortalInvalid tests POST /api/portals with invalid input.
func TestCreatePortalInvalid(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	tests := []struct {
		name string
		body string
		want int
	}{
		{"malformed JSON", `{bad}`, http.StatusBadRequest},
		{"empty body", ``, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := setupRouter(h)
			req := httptest.NewRequest("POST", "/api/portals", bytes.NewReader([]byte(tt.body)))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != tt.want {
				t.Errorf("Expected %d, got %d", tt.want, rr.Code)
			}
		})
	}
}

// TestCreatePortalInvalidZone tests POST /api/portals with nonexistent zone reference.
func TestCreatePortalInvalidZone(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	h.mgr.CreateZone(&zones.Zone{ID: "z1", Name: "A", MinX: 0, MinY: 0, MinZ: 0, MaxX: 1, MaxY: 1, MaxZ: 1})

	p := zones.Portal{
		ID: "p1", Name: "Bad Zone", ZoneAID: "z1", ZoneBID: "nonexistent",
		P1X: 0, P1Y: 0, P1Z: 0, P2X: 1, P2Y: 0, P2Z: 0, P3X: 0, P3Y: 0, P3Z: 1,
		Width: 1, Height: 1,
	}

	r := setupRouter(h)
	body, _ := json.Marshal(p)
	req := httptest.NewRequest("POST", "/api/portals", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for invalid zone_b, got %d", rr.Code)
	}
}

// TestUpdatePortal tests PUT /api/portals/{id}.
func TestUpdatePortal(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	h.mgr.CreateZone(&zones.Zone{ID: "z1", Name: "A", MinX: 0, MinY: 0, MinZ: 0, MaxX: 1, MaxY: 1, MaxZ: 1})
	h.mgr.CreateZone(&zones.Zone{ID: "z2", Name: "B", MinX: 1, MinY: 0, MinZ: 0, MaxX: 2, MaxY: 1, MaxZ: 1})

	// Create initial portal
	h.mgr.CreatePortal(&zones.Portal{
		ID: "p1", Name: "Old Door", ZoneAID: "z1", ZoneBID: "z2",
		P1X: 1, P1Y: 0, P1Z: 0,
		P2X: 1, P2Y: 0.5, P2Z: 0,
		P3X: 1, P3Y: 0.5, P3Z: 1,
		Width: 1, Height: 1,
	})

	// Update portal
	updated := zones.Portal{
		ID: "p1", Name: "New Door", ZoneAID: "z1", ZoneBID: "z2",
		P1X: 1, P1Y: 0, P1Z: 0,
		P2X: 1, P2Y: 1, P2Z: 0,
		P3X: 1, P3Y: 1, P3Z: 2,
		Width: 2, Height: 2,
	}

	r := setupRouter(h)
	body, _ := json.Marshal(updated)
	req := httptest.NewRequest("PUT", "/api/portals/p1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var result portalWithZones
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if result.Name != "New Door" {
		t.Errorf("Expected name 'New Door', got %s", result.Name)
	}

	// Verify persist
	p := h.mgr.GetPortal("p1")
	if p.Name != "New Door" {
		t.Errorf("Persisted name mismatch: %s", p.Name)
	}
}

// TestUpdatePortalInvalid tests PUT /api/portals/{id} with invalid input.
func TestUpdatePortalInvalid(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	h.mgr.CreateZone(&zones.Zone{ID: "z1", Name: "A", MinX: 0, MinY: 0, MinZ: 0, MaxX: 1, MaxY: 1, MaxZ: 1})
	h.mgr.CreatePortal(&zones.Portal{
		ID: "p1", Name: "Door", ZoneAID: "z1", ZoneBID: "z1",
		P1X: 0, P1Y: 0, P1Z: 0, P2X: 1, P2Y: 0, P2Z: 0, P3X: 0, P3Y: 0, P3Z: 1,
		Width: 1, Height: 1,
	})

	tests := []struct {
		name string
		body string
		want int
	}{
		{"malformed JSON", `{bad}`, http.StatusBadRequest},
		{"empty body", ``, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := setupRouter(h)
			req := httptest.NewRequest("PUT", "/api/portals/p1", bytes.NewReader([]byte(tt.body)))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != tt.want {
				t.Errorf("Expected %d, got %d", tt.want, rr.Code)
			}
		})
	}
}

// TestUpdatePortalNotFound tests PUT /api/portals/{id} for nonexistent portal.
func TestUpdatePortalNotFound(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	r := setupRouter(h)
	body := `{"name": "Nope"}`
	req := httptest.NewRequest("PUT", "/api/portals/nonexistent", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", rr.Code)
	}
}

// TestDeletePortal tests DELETE /api/portals/{id}.
func TestDeletePortal(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	h.mgr.CreateZone(&zones.Zone{ID: "z1", Name: "A", MinX: 0, MinY: 0, MinZ: 0, MaxX: 1, MaxY: 1, MaxZ: 1})
	h.mgr.CreatePortal(&zones.Portal{
		ID: "p1", Name: "Door", ZoneAID: "z1", ZoneBID: "z1",
		P1X: 0, P1Y: 0, P1Z: 0, P2X: 1, P2Y: 0, P2Z: 0, P3X: 0, P3Y: 0, P3Z: 1,
		Width: 1, Height: 1,
	})

	r := setupRouter(h)
	req := httptest.NewRequest("DELETE", "/api/portals/p1", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("Expected 204, got %d: %s", rr.Code, rr.Body.String())
	}

	if h.mgr.GetPortal("p1") != nil {
		t.Error("Portal should be deleted")
	}

	// Verify via GET
	req2 := httptest.NewRequest("GET", "/api/portals", nil)
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, req2)
	var result []portalWithZones
	json.NewDecoder(rr2.Body).Decode(&result)
	if len(result) != 0 {
		t.Errorf("Expected 0 portals after delete, got %d", len(result))
	}
}

// TestDeletePortalNotFound tests DELETE /api/portals/{id} for nonexistent portal.
func TestDeletePortalNotFound(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	r := setupRouter(h)
	req := httptest.NewRequest("DELETE", "/api/portals/nonexistent", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("Expected 204, got %d", rr.Code)
	}
}

// TestGetPortalCrossings tests GET /api/portals/{id}/crossings.
func TestGetPortalCrossings(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	h.mgr.CreateZone(&zones.Zone{ID: "z1", Name: "A", MinX: 0, MinY: 0, MinZ: 0, MaxX: 1, MaxY: 1, MaxZ: 1})
	h.mgr.CreateZone(&zones.Zone{ID: "z2", Name: "B", MinX: 1, MinY: 0, MinZ: 0, MaxX: 2, MaxY: 1, MaxZ: 1})
	h.mgr.CreatePortal(&zones.Portal{
		ID: "p1", Name: "Door", ZoneAID: "z1", ZoneBID: "z2",
		P1X: 1, P1Y: 0, P1Z: 0, P2X: 1, P2Y: 0.5, P2Z: 0, P3X: 1, P3Y: 0.5, P3Z: 1,
		Width: 1, Height: 1,
	})

	tests := []struct {
		name     string
		portalID string
		wantCode int
	}{
		{"existing portal", "p1", http.StatusOK},
		{"nonexistent portal", "nope", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := setupRouter(h)
			req := httptest.NewRequest("GET", "/api/portals/"+tt.portalID+"/crossings", nil)
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != tt.wantCode {
				t.Errorf("Expected %d, got %d", tt.wantCode, rr.Code)
			}
		})
	}
}

// TestPortalNormalComputed verifies that portal normal vector is auto-computed on creation.
func TestPortalNormalComputed(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	h.mgr.CreateZone(&zones.Zone{ID: "z1", Name: "A", MinX: 0, MinY: 0, MinZ: 0, MaxX: 1, MaxY: 1, MaxZ: 1})
	h.mgr.CreateZone(&zones.Zone{ID: "z2", Name: "B", MinX: 1, MinY: 0, MinZ: 0, MaxX: 2, MaxY: 1, MaxZ: 1})

	// Portal on the X=1 plane, pointing in +X direction
	p := zones.Portal{
		ID: "p1", Name: "Door", ZoneAID: "z1", ZoneBID: "z2",
		P1X: 1, P1Y: 0, P1Z: 0,
		P2X: 1, P2Y: 1, P2Z: 0,
		P3X: 1, P3Y: 1, P3Z: 1,
		Width: 1, Height: 1,
	}

	r := setupRouter(h)
	body, _ := json.Marshal(p)
	req := httptest.NewRequest("POST", "/api/portals", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("Expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var created portalWithZones
	json.NewDecoder(rr.Body).Decode(&created)

	// Normal should point in roughly +X direction
	if created.NX <= 0 {
		t.Errorf("Expected NX > 0 (portal normal in +X), got %f", created.NX)
	}
	// For this geometry, NY and NZ should be ~0 since the portal is on the X=1 plane
	if created.NY > 0.01 || created.NZ > 0.01 {
		t.Errorf("Expected NY≈0, NZ≈0 for X=1 plane portal, got NY=%f, NZ=%f", created.NY, created.NZ)
	}
}

// TestZoneCRUDRoundTrip verifies the full lifecycle: create -> read -> update -> read -> delete -> verify gone.
func TestZoneCRUDRoundTrip(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	r := setupRouter(h)

	// 1. Create
	zone := zones.Zone{
		ID: "roundtrip", Name: "Initial", ZoneType: zones.ZoneTypeKitchen,
		MinX: 0, MinY: 0, MinZ: 0, MaxX: 3, MaxY: 3, MaxZ: 2.5,
	}
	body, _ := json.Marshal(zone)
	req := httptest.NewRequest("POST", "/api/zones", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("Create: expected 201, got %d", rr.Code)
	}

	// 2. Read (via list)
	req2 := httptest.NewRequest("GET", "/api/zones", nil)
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, req2)
	var zonesList []zoneWithOcc
	json.NewDecoder(rr2.Body).Decode(&zonesList)
	if len(zonesList) != 1 {
		t.Fatalf("After create: expected 1 zone, got %d", len(zonesList))
	}
	if zonesList[0].Name != "Initial" {
		t.Errorf("After create: expected name 'Initial', got %s", zonesList[0].Name)
	}

	// 3. Update
	zone.Name = "Updated"
	zone.MaxX = 5
	zone.MaxY = 4
	body, _ = json.Marshal(zone)
	req3 := httptest.NewRequest("PUT", "/api/zones/roundtrip", bytes.NewReader(body))
	req3.Header.Set("Content-Type", "application/json")
	rr3 := httptest.NewRecorder()
	r.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Fatalf("Update: expected 200, got %d", rr3.Code)
	}

	// 4. Read after update
	req4 := httptest.NewRequest("GET", "/api/zones", nil)
	rr4 := httptest.NewRecorder()
	r.ServeHTTP(rr4, req4)
	json.NewDecoder(rr4.Body).Decode(&zonesList)
	if zonesList[0].Name != "Updated" {
		t.Errorf("After update: expected name 'Updated', got %s", zonesList[0].Name)
	}

	// 5. Delete
	req5 := httptest.NewRequest("DELETE", "/api/zones/roundtrip", nil)
	rr5 := httptest.NewRecorder()
	r.ServeHTTP(rr5, req5)
	if rr5.Code != http.StatusNoContent {
		t.Fatalf("Delete: expected 204, got %d", rr5.Code)
	}

	// 6. Verify gone
	req6 := httptest.NewRequest("GET", "/api/zones", nil)
	rr6 := httptest.NewRecorder()
	r.ServeHTTP(rr6, req6)
	json.NewDecoder(rr6.Body).Decode(&zonesList)
	if len(zonesList) != 0 {
		t.Errorf("After delete: expected 0 zones, got %d", len(zonesList))
	}
}

// TestPortalCRUDRoundTrip verifies the full portal lifecycle.
func TestPortalCRUDRoundTrip(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	// Seed zones
	h.mgr.CreateZone(&zones.Zone{ID: "z1", Name: "A", MinX: 0, MinY: 0, MinZ: 0, MaxX: 1, MaxY: 1, MaxZ: 1})
	h.mgr.CreateZone(&zones.Zone{ID: "z2", Name: "B", MinX: 1, MinY: 0, MinZ: 0, MaxX: 2, MaxY: 1, MaxZ: 1})

	r := setupRouter(h)

	// Create
	p := zones.Portal{
		ID: "ptrt", Name: "Door", ZoneAID: "z1", ZoneBID: "z2",
		P1X: 1, P1Y: 0, P1Z: 0, P2X: 1, P2Y: 0.5, P2Z: 0, P3X: 1, P3Y: 0.5, P3Z: 1,
		Width: 1, Height: 1,
	}
	body, _ := json.Marshal(p)
	req := httptest.NewRequest("POST", "/api/portals", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("Create: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify via list
	req2 := httptest.NewRequest("GET", "/api/portals", nil)
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, req2)
	var portals []portalWithZones
	json.NewDecoder(rr2.Body).Decode(&portals)
	if len(portals) != 1 {
		t.Fatalf("Expected 1 portal after create, got %d", len(portals))
	}

	// Update
	p.Name = "Big Door"
	p.Width = 2
	body, _ = json.Marshal(p)
	req3 := httptest.NewRequest("PUT", "/api/portals/ptrt", bytes.NewReader(body))
	req3.Header.Set("Content-Type", "application/json")
	rr3 := httptest.NewRecorder()
	r.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Fatalf("Update: expected 200, got %d: %s", rr3.Code, rr3.Body.String())
	}

	// Verify updated
	var updated portalWithZones
	json.NewDecoder(rr3.Body).Decode(&updated)
	if updated.Name != "Big Door" {
		t.Errorf("Expected name 'Big Door', got %s", updated.Name)
	}

	// Delete
	req4 := httptest.NewRequest("DELETE", "/api/portals/ptrt", nil)
	rr4 := httptest.NewRecorder()
	r.ServeHTTP(rr4, req4)
	if rr4.Code != http.StatusNoContent {
		t.Fatalf("Delete: expected 204, got %d", rr4.Code)
	}

	// Verify gone
	req5 := httptest.NewRequest("GET", "/api/portals", nil)
	rr5 := httptest.NewRecorder()
	r.ServeHTTP(rr5, req5)
	json.NewDecoder(rr5.Body).Decode(&portals)
	if len(portals) != 0 {
		t.Errorf("Expected 0 portals after delete, got %d", len(portals))
	}
}

// ── Zone/Portal WebSocket Broadcast Tests ─────────────────────────────────────

// mockZoneBroadcaster captures zone and portal change broadcasts for testing.
type mockZoneBroadcaster struct {
	mu            sync.Mutex
	zoneChanges   []mockZoneChange
	portalChanges []mockPortalChange
}

type mockZoneChange struct {
	action string
	zone   dashboard.ZoneSnapshot
}

type mockPortalChange struct {
	action string
	portal dashboard.PortalSnapshot
}

func (m *mockZoneBroadcaster) BroadcastZoneChange(action string, zone dashboard.ZoneSnapshot) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.zoneChanges = append(m.zoneChanges, mockZoneChange{action: action, zone: zone})
}

func (m *mockZoneBroadcaster) BroadcastPortalChange(action string, portal dashboard.PortalSnapshot) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.portalChanges = append(m.portalChanges, mockPortalChange{action: action, portal: portal})
}

func (m *mockZoneBroadcaster) getZoneChanges() []mockZoneChange {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]mockZoneChange{}, m.zoneChanges...)
}

func (m *mockZoneBroadcaster) getPortalChanges() []mockPortalChange {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]mockPortalChange{}, m.portalChanges...)
}

// newTestHandlerWithBroadcaster creates a ZonesHandler with a mock broadcaster.
func newTestHandlerWithBroadcaster(t *testing.T) (*ZonesHandler, *mockZoneBroadcaster, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "zones.db")
	mgr, err := zones.NewManager(dbPath, nil)
	if err != nil {
		t.Fatalf("Failed to create zones manager: %v", err)
	}
	handler := NewZonesHandler(mgr)
	mock := &mockZoneBroadcaster{}
	handler.SetZoneChangeBroadcaster(mock)
	return handler, mock, func() { mgr.Close() }
}

// TestZoneCreateBroadcasts verifies that creating a zone triggers a WebSocket broadcast.
func TestZoneCreateBroadcasts(t *testing.T) {
	h, mock, cleanup := newTestHandlerWithBroadcaster(t)
	defer cleanup()

	r := setupRouter(h)
	body, _ := json.Marshal(zones.Zone{
		ID: "z1", Name: "Kitchen",
		MinX: 0, MinY: 0, MinZ: 0, MaxX: 4, MaxY: 3, MaxZ: 2.5,
	})
	req := httptest.NewRequest("POST", "/api/zones", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("Expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	changes := mock.getZoneChanges()
	if len(changes) != 1 {
		t.Fatalf("Expected 1 zone broadcast, got %d", len(changes))
	}
	if changes[0].action != "created" {
		t.Errorf("Expected action 'created', got %q", changes[0].action)
	}
	if changes[0].zone.ID != "z1" || changes[0].zone.Name != "Kitchen" {
		t.Errorf("Broadcast zone mismatch: %+v", changes[0].zone)
	}
	if changes[0].zone.SizeX != 4 || changes[0].zone.SizeY != 3 || changes[0].zone.SizeZ != 2.5 {
		t.Errorf("Broadcast zone dimensions wrong: %+v", changes[0].zone)
	}
}

// TestZoneUpdateBroadcasts verifies that updating a zone triggers a WebSocket broadcast.
func TestZoneUpdateBroadcasts(t *testing.T) {
	h, mock, cleanup := newTestHandlerWithBroadcaster(t)
	defer cleanup()

	h.mgr.CreateZone(&zones.Zone{
		ID: "z1", Name: "Kitchen",
		MinX: 0, MinY: 0, MinZ: 0, MaxX: 4, MaxY: 3, MaxZ: 2.5,
	})

	r := setupRouter(h)
	body, _ := json.Marshal(zones.Zone{
		ID: "z1", Name: "Big Kitchen",
		MinX: 0, MinY: 0, MinZ: 0, MaxX: 8, MaxY: 6, MaxZ: 3,
	})
	req := httptest.NewRequest("PUT", "/api/zones/z1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	changes := mock.getZoneChanges()
	if len(changes) != 1 {
		t.Fatalf("Expected 1 zone broadcast, got %d", len(changes))
	}
	if changes[0].action != "updated" {
		t.Errorf("Expected action 'updated', got %q", changes[0].action)
	}
	if changes[0].zone.Name != "Big Kitchen" {
		t.Errorf("Expected name 'Big Kitchen', got %q", changes[0].zone.Name)
	}
	if changes[0].zone.SizeX != 8 {
		t.Errorf("Expected SizeX=8, got %f", changes[0].zone.SizeX)
	}
}

// TestZoneDeleteBroadcasts verifies that deleting a zone triggers a WebSocket broadcast.
func TestZoneDeleteBroadcasts(t *testing.T) {
	h, mock, cleanup := newTestHandlerWithBroadcaster(t)
	defer cleanup()

	h.mgr.CreateZone(&zones.Zone{
		ID: "z1", Name: "Kitchen",
		MinX: 0, MinY: 0, MinZ: 0, MaxX: 4, MaxY: 3, MaxZ: 2.5,
	})

	r := setupRouter(h)
	req := httptest.NewRequest("DELETE", "/api/zones/z1", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("Expected 204, got %d: %s", rr.Code, rr.Body.String())
	}

	changes := mock.getZoneChanges()
	if len(changes) != 1 {
		t.Fatalf("Expected 1 zone broadcast, got %d", len(changes))
	}
	if changes[0].action != "deleted" {
		t.Errorf("Expected action 'deleted', got %q", changes[0].action)
	}
	if changes[0].zone.ID != "z1" {
		t.Errorf("Expected zone ID 'z1', got %q", changes[0].zone.ID)
	}
}

// TestPortalCreateBroadcasts verifies that creating a portal triggers a WebSocket broadcast.
func TestPortalCreateBroadcasts(t *testing.T) {
	h, mock, cleanup := newTestHandlerWithBroadcaster(t)
	defer cleanup()

	h.mgr.CreateZone(&zones.Zone{ID: "z1", Name: "A", MinX: 0, MinY: 0, MinZ: 0, MaxX: 1, MaxY: 1, MaxZ: 1})
	h.mgr.CreateZone(&zones.Zone{ID: "z2", Name: "B", MinX: 1, MinY: 0, MinZ: 0, MaxX: 2, MaxY: 1, MaxZ: 1})

	r := setupRouter(h)
	body, _ := json.Marshal(zones.Portal{
		ID: "p1", Name: "Door", ZoneAID: "z1", ZoneBID: "z2",
		P1X: 1, P1Y: 0, P1Z: 0, P2X: 1, P2Y: 0.5, P2Z: 0, P3X: 1, P3Y: 0.5, P3Z: 1,
		Width: 1, Height: 1,
	})
	req := httptest.NewRequest("POST", "/api/portals", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("Expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	changes := mock.getPortalChanges()
	if len(changes) != 1 {
		t.Fatalf("Expected 1 portal broadcast, got %d", len(changes))
	}
	if changes[0].action != "created" {
		t.Errorf("Expected action 'created', got %q", changes[0].action)
	}
	if changes[0].portal.ID != "p1" || changes[0].portal.Name != "Door" {
		t.Errorf("Broadcast portal mismatch: %+v", changes[0].portal)
	}
}

// TestPortalUpdateBroadcasts verifies that updating a portal triggers a WebSocket broadcast.
func TestPortalUpdateBroadcasts(t *testing.T) {
	h, mock, cleanup := newTestHandlerWithBroadcaster(t)
	defer cleanup()

	h.mgr.CreateZone(&zones.Zone{ID: "z1", Name: "A", MinX: 0, MinY: 0, MinZ: 0, MaxX: 1, MaxY: 1, MaxZ: 1})
	h.mgr.CreateZone(&zones.Zone{ID: "z2", Name: "B", MinX: 1, MinY: 0, MinZ: 0, MaxX: 2, MaxY: 1, MaxZ: 1})
	h.mgr.CreatePortal(&zones.Portal{
		ID: "p1", Name: "Door", ZoneAID: "z1", ZoneBID: "z2",
		P1X: 1, P1Y: 0, P1Z: 0, P2X: 1, P2Y: 0.5, P2Z: 0, P3X: 1, P3Y: 0.5, P3Z: 1,
		Width: 1, Height: 1,
	})

	r := setupRouter(h)
	body, _ := json.Marshal(zones.Portal{
		ID: "p1", Name: "Big Door", ZoneAID: "z1", ZoneBID: "z2",
		P1X: 1, P1Y: 0, P1Z: 0, P2X: 1, P2Y: 1, P2Z: 0, P3X: 1, P3Y: 1, P3Z: 2,
		Width: 2, Height: 2,
	})
	req := httptest.NewRequest("PUT", "/api/portals/p1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	changes := mock.getPortalChanges()
	if len(changes) != 1 {
		t.Fatalf("Expected 1 portal broadcast, got %d", len(changes))
	}
	if changes[0].action != "updated" {
		t.Errorf("Expected action 'updated', got %q", changes[0].action)
	}
	if changes[0].portal.Name != "Big Door" {
		t.Errorf("Expected name 'Big Door', got %q", changes[0].portal.Name)
	}
}

// TestPortalDeleteBroadcasts verifies that deleting a portal triggers a WebSocket broadcast.
func TestPortalDeleteBroadcasts(t *testing.T) {
	h, mock, cleanup := newTestHandlerWithBroadcaster(t)
	defer cleanup()

	h.mgr.CreateZone(&zones.Zone{ID: "z1", Name: "A", MinX: 0, MinY: 0, MinZ: 0, MaxX: 1, MaxY: 1, MaxZ: 1})
	h.mgr.CreatePortal(&zones.Portal{
		ID: "p1", Name: "Door", ZoneAID: "z1", ZoneBID: "z1",
		P1X: 0, P1Y: 0, P1Z: 0, P2X: 1, P2Y: 0, P2Z: 0, P3X: 0, P3Y: 0, P3Z: 1,
		Width: 1, Height: 1,
	})

	r := setupRouter(h)
	req := httptest.NewRequest("DELETE", "/api/portals/p1", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("Expected 204, got %d: %s", rr.Code, rr.Body.String())
	}

	changes := mock.getPortalChanges()
	if len(changes) != 1 {
		t.Fatalf("Expected 1 portal broadcast, got %d", len(changes))
	}
	if changes[0].action != "deleted" {
		t.Errorf("Expected action 'deleted', got %q", changes[0].action)
	}
	if changes[0].portal.ID != "p1" {
		t.Errorf("Expected portal ID 'p1', got %q", changes[0].portal.ID)
	}
}

// TestNoBroadcastWithoutBroadcaster verifies that zone CRUD works even when
// no broadcaster is set (nil broadcaster is a no-op).
func TestNoBroadcastWithoutBroadcaster(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	r := setupRouter(h)
	body, _ := json.Marshal(zones.Zone{
		ID: "z1", Name: "Kitchen",
		MinX: 0, MinY: 0, MinZ: 0, MaxX: 4, MaxY: 3, MaxZ: 2.5,
	})
	req := httptest.NewRequest("POST", "/api/zones", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("Expected 201 without broadcaster, got %d: %s", rr.Code, rr.Body.String())
	}
}
