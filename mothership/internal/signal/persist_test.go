package signal

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ─── Baseline Store Tests ──────────────────────────────────────────────────────

func TestBaselineStore_New(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewBaselineStore(dbPath)
	if err != nil {
		t.Fatalf("NewBaselineStore: %v", err)
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

func TestBaselineStore_SaveAndLoadBaseline(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewBaselineStore(dbPath)
	if err != nil {
		t.Fatalf("NewBaselineStore: %v", err)
	}
	defer store.Close()

	snapshot := &BaselineSnapshot{
		Values:     []float64{1.0, 2.0, 3.0, 4.0, 5.0},
		SampleTime: time.Now().Truncate(time.Second),
		Confidence: 0.85,
	}

	err = store.SaveBaseline("link-001", snapshot)
	if err != nil {
		t.Fatalf("SaveBaseline: %v", err)
	}

	loaded, err := store.LoadBaseline("link-001")
	if err != nil {
		t.Fatalf("LoadBaseline: %v", err)
	}

	if loaded == nil {
		t.Fatal("loaded snapshot is nil")
	}

	if len(loaded.Values) != len(snapshot.Values) {
		t.Errorf("Values length = %d, want %d", len(loaded.Values), len(snapshot.Values))
	}

	for i := range snapshot.Values {
		if loaded.Values[i] != snapshot.Values[i] {
			t.Errorf("Values[%d] = %v, want %v", i, loaded.Values[i], snapshot.Values[i])
		}
	}

	if loaded.Confidence != snapshot.Confidence {
		t.Errorf("Confidence = %v, want %v", loaded.Confidence, snapshot.Confidence)
	}
}

func TestBaselineStore_LoadNonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewBaselineStore(dbPath)
	if err != nil {
		t.Fatalf("NewBaselineStore: %v", err)
	}
	defer store.Close()

	loaded, err := store.LoadBaseline("nonexistent")
	if err != nil {
		t.Fatalf("LoadBaseline error: %v", err)
	}

	if loaded != nil {
		t.Error("expected nil for nonexistent baseline")
	}
}

func TestBaselineStore_SaveAllAndLoadAll(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewBaselineStore(dbPath)
	if err != nil {
		t.Fatalf("NewBaselineStore: %v", err)
	}
	defer store.Close()

	baselines := map[string]*BaselineSnapshot{
		"link-001": {
			Values:     []float64{1.0, 1.1, 1.2},
			SampleTime: time.Now().Truncate(time.Second),
			Confidence: 0.9,
		},
		"link-002": {
			Values:     []float64{2.0, 2.1, 2.2},
			SampleTime: time.Now().Truncate(time.Second),
			Confidence: 0.8,
		},
		"link-003": {
			Values:     []float64{3.0, 3.1, 3.2},
			SampleTime: time.Now().Truncate(time.Second),
			Confidence: 0.7,
		},
	}

	err = store.SaveAllBaselines(baselines)
	if err != nil {
		t.Fatalf("SaveAllBaselines: %v", err)
	}

	loaded, err := store.LoadAllBaselines()
	if err != nil {
		t.Fatalf("LoadAllBaselines: %v", err)
	}

	if len(loaded) != 3 {
		t.Errorf("loaded %d baselines, want 3", len(loaded))
	}

	for linkID, snap := range baselines {
		loadedSnap, exists := loaded[linkID]
		if !exists {
			t.Errorf("missing baseline for %s", linkID)
			continue
		}
		if len(loadedSnap.Values) != len(snap.Values) {
			t.Errorf("%s: Values length mismatch", linkID)
		}
	}
}

func TestBaselineStore_DeleteBaseline(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewBaselineStore(dbPath)
	if err != nil {
		t.Fatalf("NewBaselineStore: %v", err)
	}
	defer store.Close()

	snapshot := &BaselineSnapshot{
		Values:     []float64{1.0, 2.0},
		SampleTime: time.Now(),
		Confidence: 0.9,
	}

	store.SaveBaseline("link-001", snapshot)

	err = store.DeleteBaseline("link-001")
	if err != nil {
		t.Fatalf("DeleteBaseline: %v", err)
	}

	loaded, _ := store.LoadBaseline("link-001")
	if loaded != nil {
		t.Error("baseline should be deleted")
	}
}

func TestBaselineStore_PruneStale(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewBaselineStore(dbPath)
	if err != nil {
		t.Fatalf("NewBaselineStore: %v", err)
	}
	defer store.Close()

	snapshot := &BaselineSnapshot{
		Values:     []float64{1.0, 2.0},
		SampleTime: time.Now(),
		Confidence: 0.9,
	}

	store.SaveBaseline("link-001", snapshot)

	// Prune entries older than 0 (entries from the past, which should be none since we just added)
	// This tests that the prune function works without error
	deleted, err := store.PruneStale(time.Hour * 24 * 365) // 1 year
	if err != nil {
		t.Fatalf("PruneStale: %v", err)
	}

	// The entry we just added should NOT be pruned (it's from now, not 1 year ago)
	loaded, _ := store.LoadBaseline("link-001")
	if loaded == nil {
		t.Error("recent baseline should not have been pruned")
	}

	// Prune with very short duration - might delete depending on timing
	_ = deleted // Use deleted to avoid unused variable warning
}

// ─── Diurnal Persistence Tests ────────────────────────────────────────────────

func TestBaselineStore_SaveAndLoadDiurnal(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewBaselineStore(dbPath)
	if err != nil {
		t.Fatalf("NewBaselineStore: %v", err)
	}
	defer store.Close()

	nSub := 3
	snapshot := &DiurnalSnapshot{
		LinkID:  "link-001",
		Created: time.Now().Truncate(time.Second),
	}

	// Initialize all 24 slots
	for i := 0; i < DiurnalSlots; i++ {
		snapshot.SlotValues[i] = make([]float64, nSub)
		snapshot.SlotCounts[i] = 0
		snapshot.SlotTimes[i] = time.Time{}
	}

	// Set some slots with data
	for slot := 0; slot < 12; slot++ {
		snapshot.SlotValues[slot] = []float64{float64(slot) * 0.1, float64(slot) * 0.2, float64(slot) * 0.3}
		snapshot.SlotCounts[slot] = 100 + slot
		snapshot.SlotTimes[slot] = time.Now().Add(-time.Duration(slot) * time.Hour)
	}

	err = store.SaveDiurnal("link-001", snapshot)
	if err != nil {
		t.Fatalf("SaveDiurnal: %v", err)
	}

	loaded, err := store.LoadDiurnal("link-001", nSub)
	if err != nil {
		t.Fatalf("LoadDiurnal: %v", err)
	}

	if loaded == nil {
		t.Fatal("loaded snapshot is nil")
	}

	if loaded.LinkID != snapshot.LinkID {
		t.Errorf("LinkID = %s, want %s", loaded.LinkID, snapshot.LinkID)
	}

	// Verify slot data
	for slot := 0; slot < 12; slot++ {
		if loaded.SlotCounts[slot] != snapshot.SlotCounts[slot] {
			t.Errorf("Slot %d count = %d, want %d", slot, loaded.SlotCounts[slot], snapshot.SlotCounts[slot])
		}
	}
}

func TestBaselineStore_LoadNonexistentDiurnal(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewBaselineStore(dbPath)
	if err != nil {
		t.Fatalf("NewBaselineStore: %v", err)
	}
	defer store.Close()

	loaded, err := store.LoadDiurnal("nonexistent", 3)
	if err != nil {
		t.Fatalf("LoadDiurnal error: %v", err)
	}

	if loaded != nil {
		t.Error("expected nil for nonexistent diurnal")
	}
}

func TestBaselineStore_SaveAllAndLoadAllDiurnal(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewBaselineStore(dbPath)
	if err != nil {
		t.Fatalf("NewBaselineStore: %v", err)
	}
	defer store.Close()

	nSub := 2

	diurnals := map[string]*DiurnalSnapshot{
		"link-001": createTestDiurnalSnapshot("link-001", nSub),
		"link-002": createTestDiurnalSnapshot("link-002", nSub),
	}

	err = store.SaveAllDiurnal(diurnals)
	if err != nil {
		t.Fatalf("SaveAllDiurnal: %v", err)
	}

	loaded, err := store.LoadAllDiurnal(nSub)
	if err != nil {
		t.Fatalf("LoadAllDiurnal: %v", err)
	}

	if len(loaded) != 2 {
		t.Errorf("loaded %d diurnals, want 2", len(loaded))
	}
}

func TestBaselineStore_DeleteDiurnal(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewBaselineStore(dbPath)
	if err != nil {
		t.Fatalf("NewBaselineStore: %v", err)
	}
	defer store.Close()

	nSub := 2
	snapshot := createTestDiurnalSnapshot("link-001", nSub)

	store.SaveDiurnal("link-001", snapshot)

	err = store.DeleteDiurnal("link-001")
	if err != nil {
		t.Fatalf("DeleteDiurnal: %v", err)
	}

	loaded, _ := store.LoadDiurnal("link-001", nSub)
	if loaded != nil {
		t.Error("diurnal should be deleted")
	}
}

func TestBaselineStore_OverwriteBaseline(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewBaselineStore(dbPath)
	if err != nil {
		t.Fatalf("NewBaselineStore: %v", err)
	}
	defer store.Close()

	// Save first version
	snapshot1 := &BaselineSnapshot{
		Values:     []float64{1.0, 2.0},
		SampleTime: time.Now(),
		Confidence: 0.8,
	}
	store.SaveBaseline("link-001", snapshot1)

	// Overwrite with second version
	snapshot2 := &BaselineSnapshot{
		Values:     []float64{3.0, 4.0, 5.0},
		SampleTime: time.Now(),
		Confidence: 0.9,
	}
	store.SaveBaseline("link-001", snapshot2)

	loaded, _ := store.LoadBaseline("link-001")

	if len(loaded.Values) != 3 {
		t.Errorf("Values length = %d, want 3", len(loaded.Values))
	}

	if loaded.Values[0] != 3.0 {
		t.Errorf("Values[0] = %v, want 3.0", loaded.Values[0])
	}

	if loaded.Confidence != 0.9 {
		t.Errorf("Confidence = %v, want 0.9", loaded.Confidence)
	}
}

func TestBaselineStore_EmptyBaseline(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewBaselineStore(dbPath)
	if err != nil {
		t.Fatalf("NewBaselineStore: %v", err)
	}
	defer store.Close()

	// Save and load an empty baseline
	snapshot := &BaselineSnapshot{
		Values:     []float64{},
		SampleTime: time.Now(),
		Confidence: 0.0,
	}

	err = store.SaveBaseline("link-empty", snapshot)
	if err != nil {
		t.Fatalf("SaveBaseline: %v", err)
	}

	loaded, err := store.LoadBaseline("link-empty")
	if err != nil {
		t.Fatalf("LoadBaseline: %v", err)
	}

	if loaded == nil {
		t.Fatal("loaded snapshot is nil")
	}

	if len(loaded.Values) != 0 {
		t.Errorf("expected empty values slice, got %d elements", len(loaded.Values))
	}
}

// ─── Helper Functions ──────────────────────────────────────────────────────────

func createTestDiurnalSnapshot(linkID string, nSub int) *DiurnalSnapshot {
	snapshot := &DiurnalSnapshot{
		LinkID:  linkID,
		Created: time.Now().Truncate(time.Second),
	}

	// Initialize all slots
	for i := 0; i < DiurnalSlots; i++ {
		snapshot.SlotValues[i] = make([]float64, nSub)
		snapshot.SlotCounts[i] = 0
		snapshot.SlotTimes[i] = time.Time{}
	}

	// Set first 6 slots with data
	for slot := 0; slot < 6; slot++ {
		for sub := 0; sub < nSub; sub++ {
			snapshot.SlotValues[slot][sub] = float64(slot*10 + sub)
		}
		snapshot.SlotCounts[slot] = 50
		snapshot.SlotTimes[slot] = time.Now().Add(-time.Duration(slot) * time.Hour)
	}

	return snapshot
}
