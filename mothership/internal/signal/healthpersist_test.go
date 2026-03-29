package signal

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ─── Health Store Tests ────────────────────────────────────────────────────────

func TestHealthStore_New(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "health.db")

	store, err := NewHealthStore(dbPath)
	if err != nil {
		t.Fatalf("NewHealthStore: %v", err)
	}
	defer store.Close()

	if store == nil {
		t.Fatal("store is nil")
	}

	// Verify database file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database file was not created")
	}
}

func TestHealthStore_LogHealth(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "health.db")

	store, err := NewHealthStore(dbPath)
	if err != nil {
		t.Fatalf("NewHealthStore: %v", err)
	}
	defer store.Close()

	entry := HealthLogEntry{
		LinkID:           "link-001",
		Timestamp:        time.Now().Truncate(time.Second),
		SNR:              0.85,
		PhaseStability:   0.90,
		PacketRate:       0.95,
		DriftRate:        0.02,
		CompositeScore:   0.88,
		DeltaRMSVariance: 0.001,
		IsQuietPeriod:    true,
	}

	err = store.LogHealth(entry)
	if err != nil {
		t.Fatalf("LogHealth: %v", err)
	}
}

func TestHealthStore_LogHealthBatch(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "health.db")

	store, err := NewHealthStore(dbPath)
	if err != nil {
		t.Fatalf("NewHealthStore: %v", err)
	}
	defer store.Close()

	entries := []HealthLogEntry{
		{
			LinkID:         "link-001",
			Timestamp:      time.Now().Add(-2 * time.Minute).Truncate(time.Second),
			SNR:            0.80,
			CompositeScore: 0.80,
		},
		{
			LinkID:         "link-001",
			Timestamp:      time.Now().Add(-1 * time.Minute).Truncate(time.Second),
			SNR:            0.85,
			CompositeScore: 0.85,
		},
		{
			LinkID:         "link-001",
			Timestamp:      time.Now().Truncate(time.Second),
			SNR:            0.90,
			CompositeScore: 0.90,
		},
	}

	err = store.LogHealthBatch(entries)
	if err != nil {
		t.Fatalf("LogHealthBatch: %v", err)
	}

	// Verify all entries were logged
	history, err := store.GetHealthHistory("link-001", 10*time.Minute)
	if err != nil {
		t.Fatalf("GetHealthHistory: %v", err)
	}

	if len(history) != 3 {
		t.Errorf("expected 3 history entries, got %d", len(history))
	}
}

func TestHealthStore_GetHealthHistory(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "health.db")

	store, err := NewHealthStore(dbPath)
	if err != nil {
		t.Fatalf("NewHealthStore: %v", err)
	}
	defer store.Close()

	// Log entries at different times
	now := time.Now()
	for i := 0; i < 5; i++ {
		entry := HealthLogEntry{
			LinkID:         "link-001",
			Timestamp:      now.Add(-time.Duration(i) * time.Minute),
			SNR:            0.8 + float64(i)*0.02,
			CompositeScore: 0.8 + float64(i)*0.02,
		}
		store.LogHealth(entry)
	}

	// Get history for last 3 minutes
	history, err := store.GetHealthHistory("link-001", 3*time.Minute)
	if err != nil {
		t.Fatalf("GetHealthHistory: %v", err)
	}

	// Should get entries from 0, 1, 2 minutes ago (3 entries)
	if len(history) < 3 {
		t.Errorf("expected at least 3 history entries, got %d", len(history))
	}

	// Verify chronological order
	for i := 1; i < len(history); i++ {
		if history[i].Timestamp.Before(history[i-1].Timestamp) {
			t.Error("history entries not in chronological order")
		}
	}
}

func TestHealthStore_GetHealthHistory_NoData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "health.db")

	store, err := NewHealthStore(dbPath)
	if err != nil {
		t.Fatalf("NewHealthStore: %v", err)
	}
	defer store.Close()

	history, err := store.GetHealthHistory("nonexistent", time.Hour)
	if err != nil {
		t.Fatalf("GetHealthHistory: %v", err)
	}

	if len(history) != 0 {
		t.Errorf("expected empty history for nonexistent link, got %d entries", len(history))
	}
}

func TestHealthStore_GetRecentHealth(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "health.db")

	store, err := NewHealthStore(dbPath)
	if err != nil {
		t.Fatalf("NewHealthStore: %v", err)
	}
	defer store.Close()

	// Log entries for multiple links
	for _, linkID := range []string{"link-001", "link-002"} {
		for i := 0; i < 3; i++ {
			entry := HealthLogEntry{
				LinkID:         linkID,
				Timestamp:      time.Now().Add(-time.Duration(i) * time.Minute),
				SNR:            0.8,
				CompositeScore: 0.8,
			}
			store.LogHealth(entry)
		}
	}

	recent, err := store.GetRecentHealth(2)
	if err != nil {
		t.Fatalf("GetRecentHealth: %v", err)
	}

	if len(recent) != 2 {
		t.Errorf("expected 2 links, got %d", len(recent))
	}

	for linkID, entries := range recent {
		if len(entries) > 2 {
			t.Errorf("link %s: expected at most 2 entries, got %d", linkID, len(entries))
		}
	}
}

func TestHealthStore_RecordAndGetFeedback(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "health.db")

	store, err := NewHealthStore(dbPath)
	if err != nil {
		t.Fatalf("NewHealthStore: %v", err)
	}
	defer store.Close()

	event := FeedbackEventRecord{
		LinkID:    "link-001",
		EventType: "false_positive",
		PosX:      1.5,
		PosY:      0.0,
		PosZ:      2.0,
		Timestamp: time.Now().Truncate(time.Second),
	}

	err = store.RecordFeedback(event)
	if err != nil {
		t.Fatalf("RecordFeedback: %v", err)
	}

	events, err := store.GetFeedbackEvents("link-001", time.Hour)
	if err != nil {
		t.Fatalf("GetFeedbackEvents: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 feedback event, got %d", len(events))
	}

	if events[0].EventType != "false_positive" {
		t.Errorf("EventType = %s, want false_positive", events[0].EventType)
	}

	if events[0].PosX != 1.5 || events[0].PosZ != 2.0 {
		t.Errorf("position = (%v, %v, %v), want (1.5, 0, 2.0)", events[0].PosX, events[0].PosY, events[0].PosZ)
	}
}

func TestHealthStore_GetFeedbackEvents_Window(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "health.db")

	store, err := NewHealthStore(dbPath)
	if err != nil {
		t.Fatalf("NewHealthStore: %v", err)
	}
	defer store.Close()

	// Record events at different times
	now := time.Now()
	for i := 0; i < 5; i++ {
		event := FeedbackEventRecord{
			LinkID:    "link-001",
			EventType: "test",
			Timestamp: now.Add(-time.Duration(i) * time.Hour),
		}
		store.RecordFeedback(event)
	}

	// Get events from last 2 hours
	events, err := store.GetFeedbackEvents("link-001", 2*time.Hour)
	if err != nil {
		t.Fatalf("GetFeedbackEvents: %v", err)
	}

	// Should get events from 0 and 1 hour ago
	if len(events) < 2 {
		t.Errorf("expected at least 2 events in 2-hour window, got %d", len(events))
	}
}

func TestHealthStore_PruneOldHealthLogs(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "health.db")

	store, err := NewHealthStore(dbPath)
	if err != nil {
		t.Fatalf("NewHealthStore: %v", err)
	}
	defer store.Close()

	// Log an entry
	entry := HealthLogEntry{
		LinkID:         "link-001",
		Timestamp:      time.Now(),
		SNR:            0.8,
		CompositeScore: 0.8,
	}
	store.LogHealth(entry)

	// Prune entries older than 1 nanosecond
	deleted, err := store.PruneOldHealthLogs(time.Nanosecond)
	if err != nil {
		t.Fatalf("PruneOldHealthLogs: %v", err)
	}

	if deleted == 0 {
		t.Error("expected at least 1 deleted entry")
	}

	// Verify entry was deleted
	history, _ := store.GetHealthHistory("link-001", time.Hour)
	if len(history) != 0 {
		t.Error("entry should have been pruned")
	}
}

func TestHealthStore_GetAllLinkIDs(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "health.db")

	store, err := NewHealthStore(dbPath)
	if err != nil {
		t.Fatalf("NewHealthStore: %v", err)
	}
	defer store.Close()

	// Log entries for multiple links
	for _, linkID := range []string{"link-003", "link-001", "link-002"} {
		entry := HealthLogEntry{
			LinkID:         linkID,
			Timestamp:      time.Now(),
			SNR:            0.8,
			CompositeScore: 0.8,
		}
		store.LogHealth(entry)
	}

	linkIDs, err := store.GetAllLinkIDs()
	if err != nil {
		t.Fatalf("GetAllLinkIDs: %v", err)
	}

	if len(linkIDs) != 3 {
		t.Errorf("expected 3 link IDs, got %d", len(linkIDs))
	}

	// Should be sorted
	for i := 1; i < len(linkIDs); i++ {
		if linkIDs[i] < linkIDs[i-1] {
			t.Error("link IDs not sorted")
		}
	}
}

func TestHealthStore_DailyAggregation(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "health.db")

	store, err := NewHealthStore(dbPath)
	if err != nil {
		t.Fatalf("NewHealthStore: %v", err)
	}
	defer store.Close()

	// Log entries for yesterday (use Unix timestamp directly)
	yesterday := time.Now().Add(-24 * time.Hour)
	for i := 0; i < 10; i++ {
		entry := HealthLogEntry{
			LinkID:         "link-001",
			Timestamp:      yesterday,
			SNR:            0.8 + float64(i)*0.01,
			PhaseStability: 0.9,
			PacketRate:     0.95,
			CompositeScore: 0.85 + float64(i)*0.01,
		}
		store.LogHealth(entry)
	}

	// Run aggregation
	err = store.AggregateDaily()
	if err != nil {
		t.Fatalf("AggregateDaily: %v", err)
	}

	// Get weekly trend (should include yesterday)
	trends, err := store.GetWeeklyTrend("link-001")
	if err != nil {
		t.Fatalf("GetWeeklyTrend: %v", err)
	}

	if len(trends) == 0 {
		t.Error("expected at least 1 daily summary after aggregation")
	}
}

func TestHealthStore_GetAllWeeklyTrends(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "health.db")

	store, err := NewHealthStore(dbPath)
	if err != nil {
		t.Fatalf("NewHealthStore: %v", err)
	}
	defer store.Close()

	// Log and aggregate for multiple links
	yesterday := time.Now().Add(-24 * time.Hour)
	for _, linkID := range []string{"link-001", "link-002"} {
		for i := 0; i < 5; i++ {
			entry := HealthLogEntry{
				LinkID:         linkID,
				Timestamp:      yesterday,
				SNR:            0.8,
				CompositeScore: 0.85,
			}
			store.LogHealth(entry)
		}
	}
	store.AggregateDaily()

	trends, err := store.GetAllWeeklyTrends()
	if err != nil {
		t.Fatalf("GetAllWeeklyTrends: %v", err)
	}

	if len(trends) < 2 {
		t.Errorf("expected at least 2 links with trends, got %d", len(trends))
	}
}

func TestHealthStore_EmptyBatch(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "health.db")

	store, err := NewHealthStore(dbPath)
	if err != nil {
		t.Fatalf("NewHealthStore: %v", err)
	}
	defer store.Close()

	// Empty batch should succeed
	err = store.LogHealthBatch([]HealthLogEntry{})
	if err != nil {
		t.Fatalf("LogHealthBatch with empty slice: %v", err)
	}
}

func TestHealthStore_LogHealthEntryRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "health.db")

	store, err := NewHealthStore(dbPath)
	if err != nil {
		t.Fatalf("NewHealthStore: %v", err)
	}
	defer store.Close()

	original := HealthLogEntry{
		LinkID:           "link-001",
		Timestamp:        time.Now().Truncate(time.Second),
		SNR:              0.85,
		PhaseStability:   0.90,
		PacketRate:       0.95,
		DriftRate:        0.02,
		CompositeScore:   0.88,
		DeltaRMSVariance: 0.0015,
		IsQuietPeriod:    true,
	}

	store.LogHealth(original)

	history, _ := store.GetHealthHistory("link-001", time.Minute)
	if len(history) == 0 {
		t.Fatal("no history entries found")
	}

	loaded := history[0]

	if loaded.LinkID != original.LinkID {
		t.Errorf("LinkID = %s, want %s", loaded.LinkID, original.LinkID)
	}
	if loaded.SNR != original.SNR {
		t.Errorf("SNR = %v, want %v", loaded.SNR, original.SNR)
	}
	if loaded.PhaseStability != original.PhaseStability {
		t.Errorf("PhaseStability = %v, want %v", loaded.PhaseStability, original.PhaseStability)
	}
	if loaded.PacketRate != original.PacketRate {
		t.Errorf("PacketRate = %v, want %v", loaded.PacketRate, original.PacketRate)
	}
	if loaded.DriftRate != original.DriftRate {
		t.Errorf("DriftRate = %v, want %v", loaded.DriftRate, original.DriftRate)
	}
	if loaded.CompositeScore != original.CompositeScore {
		t.Errorf("CompositeScore = %v, want %v", loaded.CompositeScore, original.CompositeScore)
	}
	if loaded.DeltaRMSVariance != original.DeltaRMSVariance {
		t.Errorf("DeltaRMSVariance = %v, want %v", loaded.DeltaRMSVariance, original.DeltaRMSVariance)
	}
	if loaded.IsQuietPeriod != original.IsQuietPeriod {
		t.Errorf("IsQuietPeriod = %v, want %v", loaded.IsQuietPeriod, original.IsQuietPeriod)
	}
}

func TestBoolToInt(t *testing.T) {
	if boolToInt(true) != 1 {
		t.Error("boolToInt(true) should be 1")
	}
	if boolToInt(false) != 0 {
		t.Error("boolToInt(false) should be 0")
	}
}
