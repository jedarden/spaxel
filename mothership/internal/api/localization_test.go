// Package api provides REST API tests for self-improving localization.
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
	"github.com/spaxel/mothership/internal/localization"
)

func TestLocalizationHandler_getWeights(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "localization_api_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create components
	gtStore, err := localization.NewGroundTruthStore(
		filepath.Join(tmpDir, "groundtruth.db"),
		localization.DefaultGroundTruthStoreConfig(),
	)
	if err != nil {
		t.Fatalf("Failed to create ground truth store: %v", err)
	}
	defer gtStore.Close()

	swLearner, err := localization.NewSpatialWeightLearner(
		filepath.Join(tmpDir, "spatial_weights.db"),
		localization.DefaultSpatialWeightLearnerConfig(),
	)
	if err != nil {
		t.Fatalf("Failed to create spatial weight learner: %v", err)
	}
	defer swLearner.Close()

	wStore, err := localization.NewWeightStore(filepath.Join(tmpDir, "weights.db"))
	if err != nil {
		t.Fatalf("Failed to create weight store: %v", err)
	}
	defer wStore.Close()

	config := localization.DefaultSelfImprovingLocalizerConfig()
	sil := localization.NewSelfImprovingLocalizer(config)

	// Create a separate weight learner for the handler
	// (SelfImprovingLocalizer doesn't expose its internal weightLearner)
	groundTruthProvider := localization.NewBLEGroundTruthProvider(localization.DefaultBLETrilaterationConfig())
	engine := localization.NewEngine(10.0, 10.0, 0.0, 0.0)
	wLearner := localization.NewWeightLearner(groundTruthProvider, engine, localization.DefaultWeightLearnerConfig())

	handler := NewLocalizationHandler(gtStore, swLearner, wLearner, wStore, sil)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	// Test GET /api/localization/weights
	req := httptest.NewRequest("GET", "/api/localization/weights", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Check fields
	if _, ok := result["weights"]; !ok {
		t.Error("Missing weights field")
	}
	if _, ok := result["stats"]; !ok {
		t.Error("Missing stats field")
	}
}

func TestLocalizationHandler_getLinkWeight(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "localization_api_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	gtStore, err := localization.NewGroundTruthStore(
		filepath.Join(tmpDir, "groundtruth.db"),
		localization.DefaultGroundTruthStoreConfig(),
	)
	if err != nil {
		t.Fatalf("Failed to create ground truth store: %v", err)
	}
	defer gtStore.Close()

	swLearner, err := localization.NewSpatialWeightLearner(
		filepath.Join(tmpDir, "spatial_weights.db"),
		localization.DefaultSpatialWeightLearnerConfig(),
	)
	if err != nil {
		t.Fatalf("Failed to create spatial weight learner: %v", err)
	}
	defer swLearner.Close()

	wStore, err := localization.NewWeightStore(filepath.Join(tmpDir, "weights.db"))
	if err != nil {
		t.Fatalf("Failed to create weight store: %v", err)
	}
	defer wStore.Close()

	config := localization.DefaultSelfImprovingLocalizerConfig()
	sil := localization.NewSelfImprovingLocalizer(config)

	// Create a separate weight learner for the handler
	// (SelfImprovingLocalizer doesn't expose its internal weightLearner)
	groundTruthProvider := localization.NewBLEGroundTruthProvider(localization.DefaultBLETrilaterationConfig())
	engine := localization.NewEngine(10.0, 10.0, 0.0, 0.0)
	wLearner := localization.NewWeightLearner(groundTruthProvider, engine, localization.DefaultWeightLearnerConfig())

	handler := NewLocalizationHandler(gtStore, swLearner, wLearner, wStore, sil)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	// Test GET /api/localization/weights/test-link-1
	req := httptest.NewRequest("GET", "/api/localization/weights/test-link-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Check fields
	if result["link_id"] != "test-link-1" {
		t.Errorf("Expected link_id test-link-1, got %v", result["link_id"])
	}
	if _, ok := result["weight"]; !ok {
		t.Error("Missing weight field")
	}
	if _, ok := result["sigma"]; !ok {
		t.Error("Missing sigma field")
	}
}

func TestLocalizationHandler_resetWeights(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "localization_api_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	gtStore, err := localization.NewGroundTruthStore(
		filepath.Join(tmpDir, "groundtruth.db"),
		localization.DefaultGroundTruthStoreConfig(),
	)
	if err != nil {
		t.Fatalf("Failed to create ground truth store: %v", err)
	}
	defer gtStore.Close()

	swLearner, err := localization.NewSpatialWeightLearner(
		filepath.Join(tmpDir, "spatial_weights.db"),
		localization.DefaultSpatialWeightLearnerConfig(),
	)
	if err != nil {
		t.Fatalf("Failed to create spatial weight learner: %v", err)
	}
	defer swLearner.Close()

	wStore, err := localization.NewWeightStore(filepath.Join(tmpDir, "weights.db"))
	if err != nil {
		t.Fatalf("Failed to create weight store: %v", err)
	}
	defer wStore.Close()

	config := localization.DefaultSelfImprovingLocalizerConfig()
	sil := localization.NewSelfImprovingLocalizer(config)

	// Create a separate weight learner for the handler
	groundTruthProvider := localization.NewBLEGroundTruthProvider(localization.DefaultBLETrilaterationConfig())
	engine := localization.NewEngine(10.0, 10.0, 0.0, 0.0)
	wLearner := localization.NewWeightLearner(groundTruthProvider, engine, localization.DefaultWeightLearnerConfig())

	// Set some weights first
	weights := wLearner.GetLearnedWeights()
	weights.SetWeights("test-link", 1.5, 0.5)

	handler := NewLocalizationHandler(gtStore, swLearner, wLearner, wStore, sil)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	// Test POST /api/localization/weights/reset
	req := httptest.NewRequest("POST", "/api/localization/weights/reset", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result["status"] != "weights_reset" {
		t.Errorf("Expected status weights_reset, got %v", result["status"])
	}

	// Verify weights were reset
	weight := wLearner.GetLearnedWeights().GetLinkWeight("test-link")
	if weight != 1.0 {
		t.Errorf("Expected weight to be reset to 1.0, got %v", weight)
	}
}

func TestLocalizationHandler_getSpatialWeights(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "localization_api_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	gtStore, err := localization.NewGroundTruthStore(
		filepath.Join(tmpDir, "groundtruth.db"),
		localization.DefaultGroundTruthStoreConfig(),
	)
	if err != nil {
		t.Fatalf("Failed to create ground truth store: %v", err)
	}
	defer gtStore.Close()

	swLearner, err := localization.NewSpatialWeightLearner(
		filepath.Join(tmpDir, "spatial_weights.db"),
		localization.DefaultSpatialWeightLearnerConfig(),
	)
	if err != nil {
		t.Fatalf("Failed to create spatial weight learner: %v", err)
	}
	defer swLearner.Close()

	wStore, err := localization.NewWeightStore(filepath.Join(tmpDir, "weights.db"))
	if err != nil {
		t.Fatalf("Failed to create weight store: %v", err)
	}
	defer wStore.Close()

	config := localization.DefaultSelfImprovingLocalizerConfig()
	sil := localization.NewSelfImprovingLocalizer(config)

	// Create a separate weight learner for the handler
	// (SelfImprovingLocalizer doesn't expose its internal weightLearner)
	groundTruthProvider := localization.NewBLEGroundTruthProvider(localization.DefaultBLETrilaterationConfig())
	engine := localization.NewEngine(10.0, 10.0, 0.0, 0.0)
	wLearner := localization.NewWeightLearner(groundTruthProvider, engine, localization.DefaultWeightLearnerConfig())

	handler := NewLocalizationHandler(gtStore, swLearner, wLearner, wStore, sil)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	// Test GET /api/localization/spatial-weights
	req := httptest.NewRequest("GET", "/api/localization/spatial-weights", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Check fields
	if _, ok := result["spatial_weights"]; !ok {
		t.Error("Missing spatial_weights field")
	}
	if _, ok := result["stats"]; !ok {
		t.Error("Missing stats field")
	}
}

func TestLocalizationHandler_getSpatialWeightsForZone(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "localization_api_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	gtStore, err := localization.NewGroundTruthStore(
		filepath.Join(tmpDir, "groundtruth.db"),
		localization.DefaultGroundTruthStoreConfig(),
	)
	if err != nil {
		t.Fatalf("Failed to create ground truth store: %v", err)
	}
	defer gtStore.Close()

	swLearner, err := localization.NewSpatialWeightLearner(
		filepath.Join(tmpDir, "spatial_weights.db"),
		localization.DefaultSpatialWeightLearnerConfig(),
	)
	if err != nil {
		t.Fatalf("Failed to create spatial weight learner: %v", err)
	}
	defer swLearner.Close()

	// Set some weights for testing using the public API
	// Note: We can't directly set weights without unexported methods,
	// so we'll create a GroundTruthSample to establish weights instead.
	sample := localization.GroundTruthSample{
		Timestamp:     time.Now(),
		PersonID:      "test-person",
		BLEPosition:   localization.Vec3{X: 1.0, Y: 0.0, Z: 1.0},
		BlobPosition:  localization.Vec3{X: 1.0, Y: 0.0, Z: 1.0},
		PositionError: 0.1,
		PerLinkDeltas: map[string]float64{"link1": 0.5, "link2": 0.3},
		PerLinkHealth: map[string]float64{"link1": 0.9, "link2": 0.8},
		BLEConfidence: 0.8,
		ZoneGridX:     0,
		ZoneGridY:     0,
	}
	_ = sample // We'll use this to establish weights implicitly through the system

	wStore, err := localization.NewWeightStore(filepath.Join(tmpDir, "weights.db"))
	if err != nil {
		t.Fatalf("Failed to create weight store: %v", err)
	}
	defer wStore.Close()

	config := localization.DefaultSelfImprovingLocalizerConfig()
	sil := localization.NewSelfImprovingLocalizer(config)

	// Create a separate weight learner for the handler
	// (SelfImprovingLocalizer doesn't expose its internal weightLearner)
	groundTruthProvider := localization.NewBLEGroundTruthProvider(localization.DefaultBLETrilaterationConfig())
	engine := localization.NewEngine(10.0, 10.0, 0.0, 0.0)
	wLearner := localization.NewWeightLearner(groundTruthProvider, engine, localization.DefaultWeightLearnerConfig())

	handler := NewLocalizationHandler(gtStore, swLearner, wLearner, wStore, sil)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	// Test GET /api/localization/spatial-weights/zone/0/0
	req := httptest.NewRequest("GET", "/api/localization/spatial-weights/zone/0/0", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Check fields (JSON unmarshals integers as float64)
	if result["zone_x"] != float64(0) {
		t.Errorf("Expected zone_x 0, got %v", result["zone_x"])
	}
	if result["zone_y"] != float64(0) {
		t.Errorf("Expected zone_y 0, got %v", result["zone_y"])
	}
	if _, ok := result["weights"]; !ok {
		t.Error("Missing weights field")
	}

	// Verify weights is a map (may be empty if no samples have been processed)
	if _, ok := result["weights"].(map[string]interface{}); !ok {
		t.Fatal("weights is not a map")
	}
}

func TestLocalizationHandler_getGroundTruthSamples(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "localization_api_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	gtStore, err := localization.NewGroundTruthStore(
		filepath.Join(tmpDir, "groundtruth.db"),
		localization.DefaultGroundTruthStoreConfig(),
	)
	if err != nil {
		t.Fatalf("Failed to create ground truth store: %v", err)
	}
	defer gtStore.Close()

	// Add some test samples
	for i := 0; i < 5; i++ {
		sample := localization.GroundTruthSample{
			Timestamp: time.Now().Add(-time.Duration(i) * time.Minute),
			PersonID:  "test-person",
			BLEPosition: localization.Vec3{X: 1.0, Y: 0.0, Z: 1.0},
			BlobPosition: localization.Vec3{X: 1.0 + float64(i)*0.1, Y: 0.0, Z: 1.0},
			PositionError: float64(i) * 0.1,
			PerLinkDeltas: map[string]float64{"link1": 0.5},
			PerLinkHealth: map[string]float64{"link1": 0.9},
			BLEConfidence: 0.8,
			ZoneGridX:     0,
			ZoneGridY:     0,
		}
		if err := gtStore.AddSample(sample); err != nil {
			t.Fatalf("Failed to add sample: %v", err)
		}
	}

	swLearner, err := localization.NewSpatialWeightLearner(
		filepath.Join(tmpDir, "spatial_weights.db"),
		localization.DefaultSpatialWeightLearnerConfig(),
	)
	if err != nil {
		t.Fatalf("Failed to create spatial weight learner: %v", err)
	}
	defer swLearner.Close()

	wStore, err := localization.NewWeightStore(filepath.Join(tmpDir, "weights.db"))
	if err != nil {
		t.Fatalf("Failed to create weight store: %v", err)
	}
	defer wStore.Close()

	config := localization.DefaultSelfImprovingLocalizerConfig()
	sil := localization.NewSelfImprovingLocalizer(config)

	// Create a separate weight learner for the handler
	// (SelfImprovingLocalizer doesn't expose its internal weightLearner)
	groundTruthProvider := localization.NewBLEGroundTruthProvider(localization.DefaultBLETrilaterationConfig())
	engine := localization.NewEngine(10.0, 10.0, 0.0, 0.0)
	wLearner := localization.NewWeightLearner(groundTruthProvider, engine, localization.DefaultWeightLearnerConfig())

	handler := NewLocalizationHandler(gtStore, swLearner, wLearner, wStore, sil)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	// Test GET /api/localization/groundtruth/samples
	req := httptest.NewRequest("GET", "/api/localization/groundtruth/samples?limit=10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Check fields
	if _, ok := result["samples"]; !ok {
		t.Error("Missing samples field")
	}
	if _, ok := result["count"]; !ok {
		t.Error("Missing count field")
	}

	// Verify we got samples (JSON unmarshals numbers as float64)
	count, ok := result["count"].(float64)
	if !ok || count != 5 {
		t.Errorf("Expected 5 samples, got %v", result["count"])
	}
}

func TestLocalizationHandler_getGroundTruthStats(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "localization_api_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	gtStore, err := localization.NewGroundTruthStore(
		filepath.Join(tmpDir, "groundtruth.db"),
		localization.DefaultGroundTruthStoreConfig(),
	)
	if err != nil {
		t.Fatalf("Failed to create ground truth store: %v", err)
	}
	defer gtStore.Close()

	// Add test samples
	sample := localization.GroundTruthSample{
		Timestamp:     time.Now(),
		PersonID:      "test-person",
		BLEPosition:   localization.Vec3{X: 1.0, Y: 0.0, Z: 1.0},
		BlobPosition:  localization.Vec3{X: 1.0, Y: 0.0, Z: 1.0},
		PositionError: 0.1,
		PerLinkDeltas: map[string]float64{"link1": 0.5},
		PerLinkHealth: map[string]float64{"link1": 0.9},
		BLEConfidence: 0.8,
		ZoneGridX:     0,
		ZoneGridY:     0,
	}
	if err := gtStore.AddSample(sample); err != nil {
		t.Fatalf("Failed to add sample: %v", err)
	}

	swLearner, err := localization.NewSpatialWeightLearner(
		filepath.Join(tmpDir, "spatial_weights.db"),
		localization.DefaultSpatialWeightLearnerConfig(),
	)
	if err != nil {
		t.Fatalf("Failed to create spatial weight learner: %v", err)
	}
	defer swLearner.Close()

	wStore, err := localization.NewWeightStore(filepath.Join(tmpDir, "weights.db"))
	if err != nil {
		t.Fatalf("Failed to create weight store: %v", err)
	}
	defer wStore.Close()

	config := localization.DefaultSelfImprovingLocalizerConfig()
	sil := localization.NewSelfImprovingLocalizer(config)

	// Create a separate weight learner for the handler
	// (SelfImprovingLocalizer doesn't expose its internal weightLearner)
	groundTruthProvider := localization.NewBLEGroundTruthProvider(localization.DefaultBLETrilaterationConfig())
	engine := localization.NewEngine(10.0, 10.0, 0.0, 0.0)
	wLearner := localization.NewWeightLearner(groundTruthProvider, engine, localization.DefaultWeightLearnerConfig())

	handler := NewLocalizationHandler(gtStore, swLearner, wLearner, wStore, sil)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	// Test GET /api/localization/groundtruth/stats
	req := httptest.NewRequest("GET", "/api/localization/groundtruth/stats", nil)
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
	requiredFields := []string{"total_samples", "today_samples", "by_person", "zone_counts"}
	for _, field := range requiredFields {
		if _, ok := result[field]; !ok {
			t.Errorf("Missing required field: %s", field)
		}
	}

	// Verify total samples (JSON unmarshals numbers as float64)
	total, ok := result["total_samples"].(float64)
	if !ok || total != 1 {
		t.Errorf("Expected 1 total sample, got %v", result["total_samples"])
	}
}

func TestLocalizationHandler_getAccuracyHistory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "localization_api_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	gtStore, err := localization.NewGroundTruthStore(
		filepath.Join(tmpDir, "groundtruth.db"),
		localization.DefaultGroundTruthStoreConfig(),
	)
	if err != nil {
		t.Fatalf("Failed to create ground truth store: %v", err)
	}
	defer gtStore.Close()

	swLearner, err := localization.NewSpatialWeightLearner(
		filepath.Join(tmpDir, "spatial_weights.db"),
		localization.DefaultSpatialWeightLearnerConfig(),
	)
	if err != nil {
		t.Fatalf("Failed to create spatial weight learner: %v", err)
	}
	defer swLearner.Close()

	wStore, err := localization.NewWeightStore(filepath.Join(tmpDir, "weights.db"))
	if err != nil {
		t.Fatalf("Failed to create weight store: %v", err)
	}
	defer wStore.Close()

	config := localization.DefaultSelfImprovingLocalizerConfig()
	sil := localization.NewSelfImprovingLocalizer(config)

	// Create a separate weight learner for the handler
	// (SelfImprovingLocalizer doesn't expose its internal weightLearner)
	groundTruthProvider := localization.NewBLEGroundTruthProvider(localization.DefaultBLETrilaterationConfig())
	engine := localization.NewEngine(10.0, 10.0, 0.0, 0.0)
	wLearner := localization.NewWeightLearner(groundTruthProvider, engine, localization.DefaultWeightLearnerConfig())

	handler := NewLocalizationHandler(gtStore, swLearner, wLearner, wStore, sil)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	// Test GET /api/localization/accuracy/history
	req := httptest.NewRequest("GET", "/api/localization/accuracy/history?weeks=4", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Check fields
	if _, ok := result["records"]; !ok {
		t.Error("Missing records field")
	}
	if _, ok := result["weeks"]; !ok {
		t.Error("Missing weeks field")
	}
}

func TestLocalizationHandler_getLearningProgress(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "localization_api_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	gtStore, err := localization.NewGroundTruthStore(
		filepath.Join(tmpDir, "groundtruth.db"),
		localization.DefaultGroundTruthStoreConfig(),
	)
	if err != nil {
		t.Fatalf("Failed to create ground truth store: %v", err)
	}
	defer gtStore.Close()

	swLearner, err := localization.NewSpatialWeightLearner(
		filepath.Join(tmpDir, "spatial_weights.db"),
		localization.DefaultSpatialWeightLearnerConfig(),
	)
	if err != nil {
		t.Fatalf("Failed to create spatial weight learner: %v", err)
	}
	defer swLearner.Close()

	wStore, err := localization.NewWeightStore(filepath.Join(tmpDir, "weights.db"))
	if err != nil {
		t.Fatalf("Failed to create weight store: %v", err)
	}
	defer wStore.Close()

	config := localization.DefaultSelfImprovingLocalizerConfig()
	sil := localization.NewSelfImprovingLocalizer(config)

	// Create a separate weight learner for the handler
	// (SelfImprovingLocalizer doesn't expose its internal weightLearner)
	groundTruthProvider := localization.NewBLEGroundTruthProvider(localization.DefaultBLETrilaterationConfig())
	engine := localization.NewEngine(10.0, 10.0, 0.0, 0.0)
	wLearner := localization.NewWeightLearner(groundTruthProvider, engine, localization.DefaultWeightLearnerConfig())

	handler := NewLocalizationHandler(gtStore, swLearner, wLearner, wStore, sil)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	// Test GET /api/localization/learning/progress
	req := httptest.NewRequest("GET", "/api/localization/learning/progress", nil)
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
	requiredFields := []string{"progress", "stats"}
	for _, field := range requiredFields {
		if _, ok := result[field]; !ok {
			t.Errorf("Missing required field: %s", field)
		}
	}
}

func TestLocalizationHandler_getSelfImprovingStatus(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "localization_api_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	gtStore, err := localization.NewGroundTruthStore(
		filepath.Join(tmpDir, "groundtruth.db"),
		localization.DefaultGroundTruthStoreConfig(),
	)
	if err != nil {
		t.Fatalf("Failed to create ground truth store: %v", err)
	}
	defer gtStore.Close()

	swLearner, err := localization.NewSpatialWeightLearner(
		filepath.Join(tmpDir, "spatial_weights.db"),
		localization.DefaultSpatialWeightLearnerConfig(),
	)
	if err != nil {
		t.Fatalf("Failed to create spatial weight learner: %v", err)
	}
	defer swLearner.Close()

	wStore, err := localization.NewWeightStore(filepath.Join(tmpDir, "weights.db"))
	if err != nil {
		t.Fatalf("Failed to create weight store: %v", err)
	}
	defer wStore.Close()

	config := localization.DefaultSelfImprovingLocalizerConfig()
	sil := localization.NewSelfImprovingLocalizer(config)

	// Create a separate weight learner for the handler
	// (SelfImprovingLocalizer doesn't expose its internal weightLearner)
	groundTruthProvider := localization.NewBLEGroundTruthProvider(localization.DefaultBLETrilaterationConfig())
	engine := localization.NewEngine(10.0, 10.0, 0.0, 0.0)
	wLearner := localization.NewWeightLearner(groundTruthProvider, engine, localization.DefaultWeightLearnerConfig())

	handler := NewLocalizationHandler(gtStore, swLearner, wLearner, wStore, sil)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	// Test GET /api/localization/self-improving/status
	req := httptest.NewRequest("GET", "/api/localization/self-improving/status", nil)
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
	requiredFields := []string{
		"learning_progress", "learned_weights", "improvement_stats",
		"improvement_history", "ble_observations_count",
	}
	for _, field := range requiredFields {
		if _, ok := result[field]; !ok {
			t.Errorf("Missing required field: %s", field)
		}
	}
}

func TestLocalizationHandler_processLearning(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "localization_api_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	gtStore, err := localization.NewGroundTruthStore(
		filepath.Join(tmpDir, "groundtruth.db"),
		localization.DefaultGroundTruthStoreConfig(),
	)
	if err != nil {
		t.Fatalf("Failed to create ground truth store: %v", err)
	}
	defer gtStore.Close()

	swLearner, err := localization.NewSpatialWeightLearner(
		filepath.Join(tmpDir, "spatial_weights.db"),
		localization.DefaultSpatialWeightLearnerConfig(),
	)
	if err != nil {
		t.Fatalf("Failed to create spatial weight learner: %v", err)
	}
	defer swLearner.Close()

	wStore, err := localization.NewWeightStore(filepath.Join(tmpDir, "weights.db"))
	if err != nil {
		t.Fatalf("Failed to create weight store: %v", err)
	}
	defer wStore.Close()

	config := localization.DefaultSelfImprovingLocalizerConfig()
	sil := localization.NewSelfImprovingLocalizer(config)

	// Create a separate weight learner for the handler
	// (SelfImprovingLocalizer doesn't expose its internal weightLearner)
	groundTruthProvider := localization.NewBLEGroundTruthProvider(localization.DefaultBLETrilaterationConfig())
	engine := localization.NewEngine(10.0, 10.0, 0.0, 0.0)
	wLearner := localization.NewWeightLearner(groundTruthProvider, engine, localization.DefaultWeightLearnerConfig())

	handler := NewLocalizationHandler(gtStore, swLearner, wLearner, wStore, sil)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	// Test POST /api/localization/self-improving/process
	req := httptest.NewRequest("POST", "/api/localization/self-improving/process", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Check fields
	if result["status"] != "learning_processed" {
		t.Errorf("Expected status learning_processed, got %v", result["status"])
	}
	if _, ok := result["timestamp"]; !ok {
		t.Error("Missing timestamp field")
	}
}

func TestLocalizationHandler_getImprovementHistory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "localization_api_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	gtStore, err := localization.NewGroundTruthStore(
		filepath.Join(tmpDir, "groundtruth.db"),
		localization.DefaultGroundTruthStoreConfig(),
	)
	if err != nil {
		t.Fatalf("Failed to create ground truth store: %v", err)
	}
	defer gtStore.Close()

	swLearner, err := localization.NewSpatialWeightLearner(
		filepath.Join(tmpDir, "spatial_weights.db"),
		localization.DefaultSpatialWeightLearnerConfig(),
	)
	if err != nil {
		t.Fatalf("Failed to create spatial weight learner: %v", err)
	}
	defer swLearner.Close()

	wStore, err := localization.NewWeightStore(filepath.Join(tmpDir, "weights.db"))
	if err != nil {
		t.Fatalf("Failed to create weight store: %v", err)
	}
	defer wStore.Close()

	config := localization.DefaultSelfImprovingLocalizerConfig()
	sil := localization.NewSelfImprovingLocalizer(config)

	// Create a separate weight learner for the handler
	// (SelfImprovingLocalizer doesn't expose its internal weightLearner)
	groundTruthProvider := localization.NewBLEGroundTruthProvider(localization.DefaultBLETrilaterationConfig())
	engine := localization.NewEngine(10.0, 10.0, 0.0, 0.0)
	wLearner := localization.NewWeightLearner(groundTruthProvider, engine, localization.DefaultWeightLearnerConfig())

	handler := NewLocalizationHandler(gtStore, swLearner, wLearner, wStore, sil)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	// Test GET /api/localization/learning/history
	req := httptest.NewRequest("GET", "/api/localization/learning/history", nil)
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
	if _, ok := result["history"]; !ok {
		t.Error("Missing history field")
	}
	if _, ok := result["stats"]; !ok {
		t.Error("Missing stats field")
	}
}
