// Package api provides REST API handlers for presence prediction.
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/spaxel/mothership/internal/prediction"
)

// mockZoneProvider implements ZoneProvider for testing.
type mockZoneProvider struct {
	zones map[string]string
}

func (m *mockZoneProvider) GetZone(id string) (string, bool) {
	if m.zones == nil {
		return "", false
	}
	name, ok := m.zones[id]
	return name, ok
}

// mockPersonProvider implements PersonProvider for testing.
type mockPersonProvider struct {
	people []struct {
		ID   string
		Name string
	}
}

func (m *mockPersonProvider) GetPeople() ([]struct {
	ID   string
	Name string
}, error) {
	return m.people, nil
}

func TestPredictionHandler_getPredictions(t *testing.T) {
	// Create temporary database
	tmpDir, err := os.MkdirTemp("", "prediction_api_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create prediction components
	store, err := prediction.NewModelStore(filepath.Join(tmpDir, "predictions.db"))
	if err != nil {
		t.Fatalf("Failed to create model store: %v", err)
	}
	defer store.Close()

	accuracy, err := prediction.NewAccuracyTracker(filepath.Join(tmpDir, "accuracy.db"))
	if err != nil {
		t.Fatalf("Failed to create accuracy tracker: %v", err)
	}
	defer accuracy.Close()

	predictor := prediction.NewPredictor(store)
	horizon := prediction.NewHorizonPredictor(store, accuracy)

	history := prediction.NewHistoryUpdater(store)

	handler := NewPredictionHandler(predictor, history, accuracy, horizon)

	// Set mock providers
	zp := &mockZoneProvider{
		zones: map[string]string{
			"zone_1": "Kitchen",
			"zone_2": "Living Room",
		},
	}
	pp := &mockPersonProvider{
		people: []struct {
			ID   string
			Name string
		}{
			{ID: "person_1", Name: "Alice"},
		},
	}
	handler.SetZoneProvider(zp)
	handler.SetPersonProvider(pp)

	// Create test router
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	// Test GET /api/predictions
	req := httptest.NewRequest("GET", "/api/predictions", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var predictions []prediction.PersonPrediction
	if err := json.NewDecoder(w.Body).Decode(&predictions); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Initially should be empty
	if len(predictions) != 0 {
		t.Errorf("Expected 0 predictions, got %d", len(predictions))
	}
}

func TestPredictionHandler_getStats(t *testing.T) {
	// Create temporary database
	tmpDir, err := os.MkdirTemp("", "prediction_api_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := prediction.NewModelStore(filepath.Join(tmpDir, "predictions.db"))
	if err != nil {
		t.Fatalf("Failed to create model store: %v", err)
	}
	defer store.Close()

	accuracy, err := prediction.NewAccuracyTracker(filepath.Join(tmpDir, "accuracy.db"))
	if err != nil {
		t.Fatalf("Failed to create accuracy tracker: %v", err)
	}
	defer accuracy.Close()

	predictor := prediction.NewPredictor(store)
	history := prediction.NewHistoryUpdater(store)

	handler := NewPredictionHandler(predictor, history, accuracy, nil)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	// Test GET /api/predictions/stats
	req := httptest.NewRequest("GET", "/api/predictions/stats", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var stats map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Check fields
	if _, ok := stats["transition_count"]; !ok {
		t.Error("Missing transition_count field")
	}
	if _, ok := stats["data_age_days"]; !ok {
		t.Error("Missing data_age_days field")
	}
	if _, ok := stats["has_minimum_data"]; !ok {
		t.Error("Missing has_minimum_data field")
	}
}

func TestPredictionHandler_getAccuracyOverall(t *testing.T) {
	// Create temporary database
	tmpDir, err := os.MkdirTemp("", "prediction_api_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := prediction.NewModelStore(filepath.Join(tmpDir, "predictions.db"))
	if err != nil {
		t.Fatalf("Failed to create model store: %v", err)
	}
	defer store.Close()

	accuracy, err := prediction.NewAccuracyTracker(filepath.Join(tmpDir, "accuracy.db"))
	if err != nil {
		t.Fatalf("Failed to create accuracy tracker: %v", err)
	}
	defer accuracy.Close()

	predictor := prediction.NewPredictor(store)
	history := prediction.NewHistoryUpdater(store)

	handler := NewPredictionHandler(predictor, history, accuracy, nil)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	// Test GET /api/predictions/accuracy/overall
	req := httptest.NewRequest("GET", "/api/predictions/accuracy/overall", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Check required fields
	requiredFields := []string{"accuracy_percent", "total_predictions", "pending_predictions", "target_accuracy", "meets_target", "horizon_minutes"}
	for _, field := range requiredFields {
		if _, ok := result[field]; !ok {
			t.Errorf("Missing required field: %s", field)
		}
	}

	// Verify target accuracy is 75%
	if target, ok := result["target_accuracy"].(float64); !ok || target != 75.0 {
		t.Errorf("Expected target_accuracy 75.0, got %v", result["target_accuracy"])
	}

	// Verify horizon is 15 minutes
	if horizon, ok := result["horizon_minutes"].(float64); !ok || horizon != 15 {
		t.Errorf("Expected horizon_minutes 15, got %v", result["horizon_minutes"])
	}
}

func TestPredictionHandler_getHorizonPredictions(t *testing.T) {
	// Create temporary database
	tmpDir, err := os.MkdirTemp("", "prediction_api_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := prediction.NewModelStore(filepath.Join(tmpDir, "predictions.db"))
	if err != nil {
		t.Fatalf("Failed to create model store: %v", err)
	}
	defer store.Close()

	accuracy, err := prediction.NewAccuracyTracker(filepath.Join(tmpDir, "accuracy.db"))
	if err != nil {
		t.Fatalf("Failed to create accuracy tracker: %v", err)
	}
	defer accuracy.Close()

	predictor := prediction.NewPredictor(store)
	horizon := prediction.NewHorizonPredictor(store, accuracy)
	history := prediction.NewHistoryUpdater(store)

	handler := NewPredictionHandler(predictor, history, accuracy, horizon)

	// Set mock providers
	zp := &mockZoneProvider{
		zones: map[string]string{
			"zone_1": "Kitchen",
		},
	}
	pp := &mockPersonProvider{
		people: []struct {
			ID   string
			Name string
		}{
			{ID: "person_1", Name: "Alice"},
		},
	}
	handler.SetZoneProvider(zp)
	handler.SetPersonProvider(pp)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	// Test GET /api/predictions/horizon
	req := httptest.NewRequest("GET", "/api/predictions/horizon?horizon=30", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Check horizon parameter was respected
	if horizon, ok := result["horizon_minutes"].(float64); !ok || horizon != 30 {
		t.Errorf("Expected horizon_minutes 30, got %v", result["horizon_minutes"])
	}

	// Check predictions array exists
	if _, ok := result["predictions"]; !ok {
		t.Error("Missing predictions field")
	}
}

func TestLogPredictionAccuracy(t *testing.T) {
	// Create temporary database
	tmpDir, err := os.MkdirTemp("", "prediction_api_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	accuracy, err := prediction.NewAccuracyTracker(filepath.Join(tmpDir, "accuracy.db"))
	if err != nil {
		t.Fatalf("Failed to create accuracy tracker: %v", err)
	}
	defer accuracy.Close()

	// Record some predictions
	_ = accuracy.RecordPrediction("person1", "zone_a", "zone_b", 0.8, 15*time.Minute)
	_ = accuracy.RecordPrediction("person1", "zone_a", "zone_b", 0.9, 15*time.Minute)

	// Evaluate them as if they were correct
	actualPositions := map[string]string{"person1": "zone_b"}
	_, _, _ = accuracy.EvaluatePending(actualPositions)

	// Log accuracy (should not crash)
	LogPredictionAccuracy(accuracy)
}
