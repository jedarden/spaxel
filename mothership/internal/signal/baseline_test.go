package signal

import (
	"math"
	"testing"
	"time"
)

func TestBaselineState_Initialize(t *testing.T) {
	bs := NewBaselineState(64)

	if bs.IsInitialized() {
		t.Error("New baseline should not be initialized")
	}

	amplitude := make([]float64, 64)
	for i := range amplitude {
		amplitude[i] = float64(i) + 1
	}

	bs.Initialize(amplitude)

	if !bs.IsInitialized() {
		t.Error("Baseline should be initialized after Initialize()")
	}

	baseline := bs.Get()
	for i := range baseline {
		if math.Abs(baseline[i]-amplitude[i]) > 0.001 {
			t.Errorf("Baseline[%d] = %.2f, expected %.2f", i, baseline[i], amplitude[i])
		}
	}

	if bs.GetConfidence() != 1.0 {
		t.Errorf("Expected confidence 1.0, got %.2f", bs.GetConfidence())
	}
}

func TestBaselineState_Update_WithMotion(t *testing.T) {
	bs := NewBaselineState(64)
	bs.Initialize(make([]float64, 64))

	// Try to update with motion present (should be blocked)
	amplitude := make([]float64, 64)
	for i := range amplitude {
		amplitude[i] = 100.0
	}

	updated := bs.Update(amplitude, 0.1, 0.0033) // smoothDeltaRMS > threshold

	if updated {
		t.Error("Update should be blocked when motion is detected")
	}

	// Baseline should remain at 0
	baseline := bs.Get()
	for i := range baseline {
		if math.Abs(baseline[i]) > 0.001 {
			t.Errorf("Baseline should be 0, got %.2f", baseline[i])
		}
	}
}

func TestBaselineState_Update_NoMotion(t *testing.T) {
	bs := NewBaselineState(64)

	// Initialize with zeros
	initial := make([]float64, 64)
	bs.Initialize(initial)

	// Update with no motion (should succeed)
	amplitude := make([]float64, 64)
	for i := range amplitude {
		amplitude[i] = 100.0
	}

	updated := bs.Update(amplitude, 0.01, 0.0033) // smoothDeltaRMS < threshold

	if !updated {
		t.Error("Update should succeed when no motion")
	}

	// Baseline should move toward new values
	baseline := bs.Get()
	for i := range baseline {
		// EMA: baseline = 0.0033 * 100 + (1-0.0033) * 0 = 0.33
		expected := 0.0033 * 100.0
		if math.Abs(baseline[i]-expected) > 0.1 {
			t.Errorf("Baseline[%d] = %.4f, expected ~%.4f", i, baseline[i], expected)
		}
	}
}

func TestBaselineState_RestoreFromSnapshot(t *testing.T) {
	bs := NewBaselineState(64)

	values := make([]float64, 64)
	for i := range values {
		values[i] = float64(i) * 2
	}

	// Restore from a recent snapshot
	snapshotTime := time.Now().Add(-1 * time.Hour)
	bs.RestoreFromSnapshot(values, snapshotTime)

	if !bs.IsInitialized() {
		t.Error("Baseline should be initialized after restore")
	}

	baseline := bs.Get()
	for i := range baseline {
		if math.Abs(baseline[i]-values[i]) > 0.001 {
			t.Errorf("Baseline[%d] = %.2f, expected %.2f", i, baseline[i], values[i])
		}
	}

	// Confidence should be high for recent snapshot
	if bs.GetConfidence() < 0.5 {
		t.Errorf("Expected high confidence for recent snapshot, got %.2f", bs.GetConfidence())
	}
}

func TestBaselineState_RestoreFromStaleSnapshot(t *testing.T) {
	bs := NewBaselineState(64)

	values := make([]float64, 64)
	for i := range values {
		values[i] = float64(i)
	}

	// Restore from a stale snapshot (> 7 days old)
	snapshotTime := time.Now().Add(-8 * 24 * time.Hour)
	bs.RestoreFromSnapshot(values, snapshotTime)

	// Confidence should be minimum for stale snapshot
	if bs.GetConfidence() != BaselineConfidenceMin {
		t.Errorf("Expected confidence %.2f for stale snapshot, got %.2f",
			BaselineConfidenceMin, bs.GetConfidence())
	}
}

func TestBaselineState_Reset(t *testing.T) {
	bs := NewBaselineState(64)
	bs.Initialize(make([]float64, 64))

	bs.Reset()

	if bs.IsInitialized() {
		t.Error("Baseline should not be initialized after reset")
	}

	if bs.GetConfidence() != 0.0 {
		t.Errorf("Expected confidence 0 after reset, got %.2f", bs.GetConfidence())
	}
}

func TestBaselineManager_GetOrCreate(t *testing.T) {
	bm := NewBaselineManager(64)

	bs1 := bm.GetOrCreate("AA:BB:CC:DD:EE:FF:11:22:33:44:55:66")
	bs2 := bm.GetOrCreate("AA:BB:CC:DD:EE:FF:11:22:33:44:55:66")

	if bs1 != bs2 {
		t.Error("GetOrCreate should return same baseline for same linkID")
	}

	if bm.LinkCount() != 1 {
		t.Errorf("Expected 1 link, got %d", bm.LinkCount())
	}
}

func TestBaselineManager_Remove(t *testing.T) {
	bm := NewBaselineManager(64)
	linkID := "AA:BB:CC:DD:EE:FF:11:22:33:44:55:66"

	bm.GetOrCreate(linkID)
	bm.Remove(linkID)

	if bm.LinkCount() != 0 {
		t.Errorf("Expected 0 links after remove, got %d", bm.LinkCount())
	}

	bs := bm.Get(linkID)
	if bs != nil {
		t.Error("Get should return nil for removed link")
	}
}

func TestBaselineManager_GetAllSnapshots(t *testing.T) {
	bm := NewBaselineManager(64)

	// Create two links
	bs1 := bm.GetOrCreate("link1")
	bs1.Initialize(make([]float64, 64))

	bs2 := bm.GetOrCreate("link2")
	bs2.Initialize(make([]float64, 64))

	snapshots := bm.GetAllSnapshots()

	if len(snapshots) != 2 {
		t.Errorf("Expected 2 snapshots, got %d", len(snapshots))
	}
}
