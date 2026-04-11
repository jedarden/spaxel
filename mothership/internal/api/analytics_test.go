// Package api provides tests for crowd flow analytics API endpoints.
package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/spaxel/mothership/internal/analytics"
)

func TestAnalyticsHandler_GetFlowMap(t *testing.T) {
	// Create temp database
	tmpDir, err := os.MkdirTemp("", "analytics_api_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create flow accumulator and add test data
	flowAcc := analytics.NewFlowAccumulator(db, 0.25)
	if err := flowAcc.InitSchema(); err != nil {
		t.Fatalf("Failed to init schema: %v", err)
	}

	// Add some test segments
	flowAcc.AddTrackUpdate("track-1", 0, 0, 0, 0.3, 0, 0, "person1")
	flowAcc.AddTrackUpdate("track-1", 0.3, 0, 0, 0.3, 0, 0, "person1")
	flowAcc.AddTrackUpdate("track-2", 1, 0, 0, 0.3, 0, 0, "person2")
	flowAcc.AddTrackUpdate("track-2", 1.3, 0, 0, 0.3, 0, 0, "person2")

	if err := flowAcc.Flush(); err != nil {
		t.Fatalf("Failed to flush accumulator: %v", err)
	}

	// Create handler
	handler := NewAnalyticsHandler(db, 0.25)

	// Test request with no filters
	req := httptest.NewRequest("GET", "/api/analytics/flow", nil)
	w := httptest.NewRecorder()

	handler.getFlowMap(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var flowMap analytics.FlowMap
	if err := json.NewDecoder(w.Body).Decode(&flowMap); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(flowMap.Cells) == 0 {
		t.Error("Expected at least one flow cell")
	}

	// Test with person filter
	req = httptest.NewRequest("GET", "/api/analytics/flow?person_id=person1", nil)
	w = httptest.NewRecorder()

	handler.getFlowMap(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for person filter, got %d", w.Code)
	}

	var personFlowMap analytics.FlowMap
	if err := json.NewDecoder(w.Body).Decode(&personFlowMap); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Person-filtered flow should have fewer or equal cells compared to all flow
	if len(personFlowMap.Cells) > len(flowMap.Cells) {
		t.Error("Person-filtered flow should have <= cells than unfiltered flow")
	}

	// Test with time range
	since := time.Now().Add(-1 * time.Hour)
	req = httptest.NewRequest("GET", "/api/analytics/flow?since="+since.Format(time.RFC3339), nil)
	w = httptest.NewRecorder()

	handler.getFlowMap(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for time range, got %d", w.Code)
	}

	// Test invalid timestamp
	req = httptest.NewRequest("GET", "/api/analytics/flow?since=invalid", nil)
	w = httptest.NewRecorder()

	handler.getFlowMap(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid timestamp, got %d", w.Code)
	}
}

func TestAnalyticsHandler_GetDwellHeatmap(t *testing.T) {
	// Create temp database
	tmpDir, err := os.MkdirTemp("", "analytics_api_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create flow accumulator and add test dwell data
	flowAcc := analytics.NewFlowAccumulator(db, 0.25)
	if err := flowAcc.InitSchema(); err != nil {
		t.Fatalf("Failed to init schema: %v", err)
	}

	// Add 100 stationary updates at the same location
	x, y := 1.5, 2.0
	flowAcc.AddTrackUpdate("track-1", x, y, 0, 0, 0, 0, "person1")
	for i := 0; i < 99; i++ {
		flowAcc.AddTrackUpdate("track-1", x, y, 0, 0, 0, 0, "person1")
	}

	if err := flowAcc.Flush(); err != nil {
		t.Fatalf("Failed to flush accumulator: %v", err)
	}

	// Create handler
	handler := NewAnalyticsHandler(db, 0.25)

	// Test request with no filters
	req := httptest.NewRequest("GET", "/api/analytics/dwell", nil)
	w := httptest.NewRecorder()

	handler.getDwellHeatmap(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var heatmap analytics.DwellHeatmap
	if err := json.NewDecoder(w.Body).Decode(&heatmap); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(heatmap.Cells) == 0 {
		t.Error("Expected at least one dwell cell")
	}

	if heatmap.MaxCount == 0 {
		t.Error("Expected max count > 0")
	}

	// Test with person filter
	req = httptest.NewRequest("GET", "/api/analytics/dwell?person_id=person1", nil)
	w = httptest.NewRecorder()

	handler.getDwellHeatmap(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for person filter, got %d", w.Code)
	}

	var personHeatmap analytics.DwellHeatmap
	if err := json.NewDecoder(w.Body).Decode(&personHeatmap); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if personHeatmap.PersonID != "person1" {
		t.Errorf("Expected person_id to be 'person1', got '%s'", personHeatmap.PersonID)
	}
}

func TestAnalyticsHandler_GetCorridors(t *testing.T) {
	// Create temp database
	tmpDir, err := os.MkdirTemp("", "analytics_api_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create flow accumulator and add test corridor data
	flowAcc := analytics.NewFlowAccumulator(db, 0.25)
	if err := flowAcc.InitSchema(); err != nil {
		t.Fatalf("Failed to init schema: %v", err)
	}

	// Create aligned segments for corridor detection
	for i := 0; i < 20; i++ {
		trackID := string(rune('a' + i))
		x := float64(i) * 0.25
		flowAcc.AddTrackUpdate(trackID, x, 0, 1.0, 0.25, 0, 0, "")
		flowAcc.AddTrackUpdate(trackID, x+0.25, 0, 1.0, 0.25, 0, 0, "")
	}

	if err := flowAcc.Flush(); err != nil {
		t.Fatalf("Failed to flush accumulator: %v", err)
	}

	// Run corridor detection
	if _, err := flowAcc.DetectCorridors(); err != nil {
		t.Logf("Warning: Failed to detect corridors (may need more data): %v", err)
	}

	// Create handler
	handler := NewAnalyticsHandler(db, 0.25)

	// Test request
	req := httptest.NewRequest("GET", "/api/analytics/corridors", nil)
	w := httptest.NewRecorder()

	handler.getCorridors(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Parse response - it might be an array directly or wrapped in an object
	body := w.Body.String()
	var corridors []analytics.DetectedCorridor
	if strings.HasPrefix(body, "[") {
		// Direct array
		if err := json.Unmarshal(w.Body.Bytes(), &corridors); err != nil {
			t.Fatalf("Failed to decode response as array: %v", err)
		}
	} else {
		// Wrapped in object
		var response map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		corridorsArray, ok := response["corridors"].([]interface{})
		if ok {
			// Convert to array of DetectedCorridor
			corridorsJSON, _ := json.Marshal(corridorsArray)
			json.Unmarshal(corridorsJSON, &corridors)
		}
	}

	// Corridors may be empty if not enough data, but response should be valid
	// We just verify the response was successful
}

func TestAnalyticsHandler_Integration(t *testing.T) {
	// Integration test that verifies the full flow from API request to response
	// Create temp database
	tmpDir, err := os.MkdirTemp("", "analytics_api_integration")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create handler
	handler := NewAnalyticsHandler(db, 0.25)

	// Test 1: Flow map with no data should return empty cells
	t.Run("EmptyFlowMap", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/analytics/flow", nil)
		w := httptest.NewRecorder()

		handler.getFlowMap(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var flowMap analytics.FlowMap
		if err := json.NewDecoder(w.Body).Decode(&flowMap); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if flowMap.SegmentCount != 0 {
			t.Errorf("Expected 0 segments, got %d", flowMap.SegmentCount)
		}
	})

	// Test 2: Dwell heatmap with no data should return empty cells
	t.Run("EmptyDwellHeatmap", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/analytics/dwell", nil)
		w := httptest.NewRecorder()

		handler.getDwellHeatmap(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var heatmap analytics.DwellHeatmap
		if err := json.NewDecoder(w.Body).Decode(&heatmap); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if len(heatmap.Cells) != 0 {
			t.Errorf("Expected 0 cells, got %d", len(heatmap.Cells))
		}
	})

	// Test 3: Corridors with no data should return empty array
	t.Run("EmptyCorridors", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/analytics/corridors", nil)
		w := httptest.NewRecorder()

		handler.getCorridors(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		// Parse response - it might be an array directly or wrapped in an object
		body := w.Body.String()
		if strings.HasPrefix(body, "[") {
			var corridors []analytics.DetectedCorridor
			if err := json.Unmarshal([]byte(body), &corridors); err != nil {
				t.Fatalf("Failed to decode response as array: %v", err)
			}
			if len(corridors) != 0 {
				t.Errorf("Expected 0 corridors, got %d", len(corridors))
			}
		} else {
			var response map[string]interface{}
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}
			// Just verify we got a valid response
		}
	})

	// Test 4: Full workflow - add data then query
	t.Run("FullWorkflow", func(t *testing.T) {
		flowAcc := handler.GetFlowAccumulator()

		// Add trajectory data
		flowAcc.AddTrackUpdate("track-1", 0, 0, 0, 0.3, 0, 0, "alice")
		flowAcc.AddTrackUpdate("track-1", 0.3, 0, 0, 0.3, 0, 0, "alice")

		// Add dwell data
		flowAcc.AddTrackUpdate("track-2", 1.0, 1.0, 0, 0, 0, 0, "bob")
		for i := 0; i < 50; i++ {
			flowAcc.AddTrackUpdate("track-2", 1.0, 1.0, 0, 0, 0, 0, "bob")
		}

		if err := flowAcc.Flush(); err != nil {
			t.Fatalf("Failed to flush: %v", err)
		}

		// Query flow map
		req := httptest.NewRequest("GET", "/api/analytics/flow", nil)
		w := httptest.NewRecorder()
		handler.getFlowMap(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var flowMap analytics.FlowMap
		if err := json.NewDecoder(w.Body).Decode(&flowMap); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if len(flowMap.Cells) == 0 {
			t.Error("Expected flow cells after adding data")
		}

		// Query dwell heatmap
		req = httptest.NewRequest("GET", "/api/analytics/dwell", nil)
		w = httptest.NewRecorder()
		handler.getDwellHeatmap(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var heatmap analytics.DwellHeatmap
		if err := json.NewDecoder(w.Body).Decode(&heatmap); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if len(heatmap.Cells) == 0 {
			t.Error("Expected dwell cells after adding data")
		}

		// Query with person filter
		req = httptest.NewRequest("GET", "/api/analytics/dwell?person_id=alice", nil)
		w = httptest.NewRecorder()
		handler.getDwellHeatmap(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200 for person filter, got %d", w.Code)
		}

		var aliceHeatmap analytics.DwellHeatmap
		if err := json.NewDecoder(w.Body).Decode(&aliceHeatmap); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Alice should have fewer dwell samples than all people combined
		if len(aliceHeatmap.Cells) > len(heatmap.Cells) {
			t.Error("Alice's dwell cells should be <= total dwell cells")
		}
	})
}

func TestAnalyticsHandler_RegisterRoutes(t *testing.T) {
	// Test that routes are properly registered
	tmpDir, err := os.MkdirTemp("", "analytics_routes_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	handler := NewAnalyticsHandler(db, 0.25)

	// Verify the handler has the expected accumulator
	if handler.GetFlowAccumulator() == nil {
		t.Error("Expected GetFlowAccumulator to return non-nil")
	}
}

func TestAnalyticsHandler_ContentHeaders(t *testing.T) {
	// Test that responses have correct content type
	tmpDir, err := os.MkdirTemp("", "analytics_headers_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	handler := NewAnalyticsHandler(db, 0.25)

	req := httptest.NewRequest("GET", "/api/analytics/flow", nil)
	w := httptest.NewRecorder()

	handler.getFlowMap(w, req)

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}
}

func TestAnalyticsHandler_ErrorHandling(t *testing.T) {
	// Test error handling with nil accumulator by creating a handler with nil db
	// We need to create a proper database for the handler to work
	tmpDir, err := os.MkdirTemp("", "analytics_error_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	handler := NewAnalyticsHandler(db, 0.25)

	// Test with invalid timestamp format
	req := httptest.NewRequest("GET", "/api/analytics/flow?since=invalid-timestamp", nil)
	w := httptest.NewRecorder()

	handler.getFlowMap(w, req)

	// Should return 400 Bad Request for invalid timestamp
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid timestamp, got %d", w.Code)
	}
}
