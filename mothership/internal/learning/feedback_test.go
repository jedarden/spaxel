package learning

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewFeedbackStore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "learning-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "learning.db")
	store, err := NewFeedbackStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create feedback store: %v", err)
	}
	defer store.Close()

	// Verify database file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Database file was not created")
	}
}

func TestRecordFeedback(t *testing.T) {
	store := setupTestFeedbackStore(t)
	defer store.Close()

	// Record a true positive
	feedback := FeedbackRecord{
		ID:           "test-1",
		EventID:      "event-1",
		EventType:    BlobDetection,
		FeedbackType: TruePositive,
		Details: map[string]interface{}{
			"zone_id": "zone-kitchen",
			"notes":   "Correct detection",
		},
		Timestamp: time.Now(),
		Applied:   false,
	}

	err := store.RecordFeedback(feedback)
	if err != nil {
		t.Fatalf("RecordFeedback failed: %v", err)
	}

	// Verify feedback count
	count, err := store.GetFeedbackCount()
	if err != nil {
		t.Fatalf("GetFeedbackCount failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected count 1, got %d", count)
	}
}

func TestGetUnprocessedFeedback(t *testing.T) {
	store := setupTestFeedbackStore(t)
	defer store.Close()

	// Record multiple feedback entries
	for i := 0; i < 3; i++ {
		feedback := FeedbackRecord{
			ID:           "test-" + string(rune('a'+i)),
			EventID:      "event-" + string(rune('a'+i)),
			EventType:    BlobDetection,
			FeedbackType: TruePositive,
			Timestamp:    time.Now(),
			Applied:      false,
		}
		store.RecordFeedback(feedback)
	}

	// Get unprocessed feedback
	feedbacks, err := store.GetUnprocessedFeedback()
	if err != nil {
		t.Fatalf("GetUnprocessedFeedback failed: %v", err)
	}

	if len(feedbacks) != 3 {
		t.Errorf("Expected 3 unprocessed feedback entries, got %d", len(feedbacks))
	}
}

func TestMarkFeedbackProcessed(t *testing.T) {
	store := setupTestFeedbackStore(t)
	defer store.Close()

	// Record feedback
	feedback := FeedbackRecord{
		ID:           "test-1",
		EventID:      "event-1",
		EventType:    BlobDetection,
		FeedbackType: FalsePositive,
		Timestamp:    time.Now(),
		Applied:      false,
	}
	store.RecordFeedback(feedback)

	// Verify unprocessed
	feedbacks, _ := store.GetUnprocessedFeedback()
	if len(feedbacks) != 1 {
		t.Fatalf("Expected 1 unprocessed, got %d", len(feedbacks))
	}

	// Mark as processed
	err := store.MarkFeedbackProcessed([]string{"test-1"})
	if err != nil {
		t.Fatalf("MarkFeedbackProcessed failed: %v", err)
	}

	// Verify no unprocessed remain
	feedbacks, _ = store.GetUnprocessedFeedback()
	if len(feedbacks) != 0 {
		t.Errorf("Expected 0 unprocessed, got %d", len(feedbacks))
	}

	// Verify stats show processed count
	stats, err := store.GetFeedbackStats()
	if err != nil {
		t.Fatalf("GetFeedbackStats failed: %v", err)
	}
	if stats["processed_count"].(int) != 1 {
		t.Errorf("Expected processed_count 1, got %d", stats["processed_count"])
	}
}

func TestFalsePositiveFrameStorage(t *testing.T) {
	store := setupTestFeedbackStore(t)
	defer store.Close()

	// Add false positive frame
	frame := FalsePositiveFrame{
		LinkID:   "link-1-2",
		Timestamp: time.Now(),
		DeltaRMS: 0.15,
		Context: map[string]interface{}{
			"zone_id": "zone-living",
		},
	}

	err := store.AddFalsePositiveFrame(frame)
	if err != nil {
		t.Fatalf("AddFalsePositiveFrame failed: %v", err)
	}

	// Retrieve frames
	frames, err := store.GetFalsePositiveFrames("link-1-2", 24*time.Hour)
	if err != nil {
		t.Fatalf("GetFalsePositiveFrames failed: %v", err)
	}

	if len(frames) != 1 {
		t.Errorf("Expected 1 frame, got %d", len(frames))
	}

	if frames[0].DeltaRMS != 0.15 {
		t.Errorf("Expected DeltaRMS 0.15, got %f", frames[0].DeltaRMS)
	}
}

func TestFalseNegativeFrameStorage(t *testing.T) {
	store := setupTestFeedbackStore(t)
	defer store.Close()

	// Add false negative frame
	frame := FalseNegativeFrame{
		LinkID:            "link-2-3",
		Timestamp:         time.Now(),
		ExpectedPositionX: 1.5,
		ExpectedPositionY: 0.0,
		ExpectedPositionZ: 2.0,
		Context: map[string]interface{}{
			"user_reported": true,
		},
	}

	err := store.AddFalseNegativeFrame(frame)
	if err != nil {
		t.Fatalf("AddFalseNegativeFrame failed: %v", err)
	}

	// Retrieve frames
	frames, err := store.GetFalseNegativeFrames("link-2-3", 24*time.Hour)
	if err != nil {
		t.Fatalf("GetFalseNegativeFrames failed: %v", err)
	}

	if len(frames) != 1 {
		t.Errorf("Expected 1 frame, got %d", len(frames))
	}

	if frames[0].ExpectedPositionX != 1.5 {
		t.Errorf("Expected X position 1.5, got %f", frames[0].ExpectedPositionX)
	}
}

func TestSaveAccuracyRecord(t *testing.T) {
	store := setupTestFeedbackStore(t)
	defer store.Close()

	record := AccuracyRecord{
		Week:       GetWeekString(time.Now()),
		ScopeType:  ScopeTypeSystem,
		ScopeID:    ScopeIDSystem,
		Precision:  0.8,
		Recall:     0.888,
		F1:         0.841,
		TPCount:    8,
		FPCount:    2,
		FNCount:    1,
		ComputedAt: time.Now(),
	}

	err := store.SaveAccuracyRecord(record)
	if err != nil {
		t.Fatalf("SaveAccuracyRecord failed: %v", err)
	}

	// Retrieve history
	records, err := store.GetAccuracyHistory(ScopeTypeSystem, ScopeIDSystem, 1)
	if err != nil {
		t.Fatalf("GetAccuracyHistory failed: %v", err)
	}

	if len(records) != 1 {
		t.Errorf("Expected 1 record, got %d", len(records))
	}

	// Verify values
	if records[0].Precision != 0.8 {
		t.Errorf("Expected precision 0.8, got %f", records[0].Precision)
	}
	if records[0].TPCount != 8 {
		t.Errorf("Expected TP count 8, got %d", records[0].TPCount)
	}
}

func TestAccuracyMetrics(t *testing.T) {
	// Test precision/recall/F1 calculation
	// precision = TP / (TP + FP)
	// recall = TP / (TP + FN)
	// F1 = 2 * precision * recall / (precision + recall)

	tp := 8
	fp := 2
	fn := 1

	precision := float64(tp) / float64(tp+fp) // 0.8
	recall := float64(tp) / float64(tp+fn)    // 0.888...
	f1 := 2 * precision * recall / (precision + recall)

	if precision != 0.8 {
		t.Errorf("Expected precision 0.8, got %f", precision)
	}

	expectedRecall := 8.0 / 9.0
	if recall < expectedRecall-0.001 || recall > expectedRecall+0.001 {
		t.Errorf("Expected recall ~0.888, got %f", recall)
	}

	// F1 should be around 0.842
	if f1 < 0.84 || f1 > 0.85 {
		t.Errorf("Expected F1 ~0.842, got %f", f1)
	}
}

func TestGetFeedbackStats(t *testing.T) {
	store := setupTestFeedbackStore(t)
	defer store.Close()

	// Record various feedback types
	types := []FeedbackType{TruePositive, FalsePositive, FalseNegative, TruePositive}
	for i, ft := range types {
		store.RecordFeedback(FeedbackRecord{
			ID:           "test-" + string(rune('a'+i)),
			EventType:    BlobDetection,
			FeedbackType: ft,
			Timestamp:    time.Now(),
			Applied:      false,
		})
	}

	stats, err := store.GetFeedbackStats()
	if err != nil {
		t.Fatalf("GetFeedbackStats failed: %v", err)
	}

	if stats["total_count"].(int) != 4 {
		t.Errorf("Expected total_count 4, got %d", stats["total_count"])
	}

	byType := stats["by_type"].(map[string]int)
	if byType[string(TruePositive)] != 2 {
		t.Errorf("Expected 2 TRUE_POSITIVE, got %d", byType[string(TruePositive)])
	}
	if byType[string(FalsePositive)] != 1 {
		t.Errorf("Expected 1 FALSE_POSITIVE, got %d", byType[string(FalsePositive)])
	}
}

func TestGetFeedbackByEvent(t *testing.T) {
	store := setupTestFeedbackStore(t)
	defer store.Close()

	// Record feedback for specific event
	store.RecordFeedback(FeedbackRecord{
		ID:           "test-1",
		EventID:      "event-123",
		EventType:    BlobDetection,
		FeedbackType: TruePositive,
		Timestamp:    time.Now(),
	})
	store.RecordFeedback(FeedbackRecord{
		ID:           "test-2",
		EventID:      "event-456",
		EventType:    FallAlert,
		FeedbackType: FalsePositive,
		Timestamp:    time.Now(),
	})

	// Get feedback for event-123
	feedbacks, err := store.GetFeedbackByEvent("event-123")
	if err != nil {
		t.Fatalf("GetFeedbackByEvent failed: %v", err)
	}

	if len(feedbacks) != 1 {
		t.Errorf("Expected 1 feedback for event-123, got %d", len(feedbacks))
	}

	if feedbacks[0].FeedbackType != TruePositive {
		t.Errorf("Expected TRUE_POSITIVE, got %s", feedbacks[0].FeedbackType)
	}
}

func TestGetWeekString(t *testing.T) {
	// Test that GetWeekString produces ISO week format
	testTime := time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)
	weekStr := GetWeekString(testTime)

	// Should be in format "2026-W14" (March 29, 2026 is in week 14)
	if len(weekStr) != 8 {
		t.Errorf("Expected week string length 8, got %d", len(weekStr))
	}

	// Should start with year
	if weekStr[:5] != "2026-" {
		t.Errorf("Expected week string to start with '2026-', got %s", weekStr[:5])
	}
}

func TestFeedbackProcessor(t *testing.T) {
	store := setupTestFeedbackStore(t)
	defer store.Close()

	// Create processor
	config := DefaultProcessorConfig()
	processor := NewProcessor(store, config)

	// Record false positive with link details
	store.RecordFeedback(FeedbackRecord{
		ID:           "test-1",
		EventID:      "event-1",
		EventType:    BlobDetection,
		FeedbackType: FalsePositive,
		Details: map[string]interface{}{
			"link_id":   "link-1-2",
			"delta_rms": 0.15,
		},
		Timestamp: time.Now(),
		Applied:   false,
	})

	// Process feedback
	err := processor.ProcessNow()
	if err != nil {
		t.Fatalf("ProcessNow failed: %v", err)
	}

	// Verify feedback was marked as processed
	feedbacks, _ := store.GetUnprocessedFeedback()
	if len(feedbacks) != 0 {
		t.Errorf("Expected 0 unprocessed, got %d", len(feedbacks))
	}

	// Verify false positive frame was stored
	frames, err := store.GetFalsePositiveFrames("link-1-2", 24*time.Hour)
	if err != nil {
		t.Fatalf("GetFalsePositiveFrames failed: %v", err)
	}
	if len(frames) != 1 {
		t.Errorf("Expected 1 false positive frame, got %d", len(frames))
	}
}

// Helper function to set up a test feedback store
func setupTestFeedbackStore(t *testing.T) *FeedbackStore {
	tmpDir, err := os.MkdirTemp("", "learning-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "learning.db")
	store, err := NewFeedbackStore(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create feedback store: %v", err)
	}

	t.Cleanup(func() {
		os.RemoveAll(tmpDir)
	})

	return store
}
