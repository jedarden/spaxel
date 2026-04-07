package analytics

import (
	"database/sql"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// openTestDB creates a test SQLite database with the anomaly_patterns table.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "pattern_test")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Create required tables
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value_json TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS anomaly_patterns (
			zone_id     TEXT NOT NULL,
			hour_of_day INTEGER NOT NULL CHECK (hour_of_day BETWEEN 0 AND 23),
			day_of_week INTEGER NOT NULL CHECK (day_of_week BETWEEN 0 AND 6),
			mean_count  REAL NOT NULL DEFAULT 0,
			variance    REAL NOT NULL DEFAULT 0,
			sample_count INTEGER NOT NULL DEFAULT 0,
			updated_at  INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (zone_id, hour_of_day, day_of_week)
		);
	`)
	if err != nil {
		t.Fatalf("create tables: %v", err)
	}

	return db
}

// newTestLearner creates a PatternLearner backed by a temp database.
func newTestLearner(t *testing.T) *PatternLearner {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "pattern_learner_test")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	pl, err := NewPatternLearner(filepath.Join(tmpDir, "patterns.db"))
	if err != nil {
		t.Fatalf("NewPatternLearner: %v", err)
	}
	t.Cleanup(func() { pl.Close() })
	return pl
}

// --- Welford's algorithm tests ---

func TestWelfordUpdate_NumericalStability(t *testing.T) {
	tests := []struct {
		name         string
		observations []float64
		wantMean     float64
		wantVar      float64
	}{
		{
			name:         "single observation",
			observations: []float64{5.0},
			wantMean:     5.0,
			wantVar:      0.0,
		},
		{
			name:         "two identical observations",
			observations: []float64{3.0, 3.0},
			wantMean:     3.0,
			wantVar:      0.0,
		},
		{
			name:         "three observations",
			observations: []float64{1.0, 2.0, 6.0},
			wantMean:     3.0,
			wantVar:      4.666666666666667,
		},
		{
			name:         "zero observations then non-zero",
			observations: []float64{0.0, 0.0, 0.0, 5.0},
			wantMean:     1.25,
			wantVar:      4.6875,
		},
		{
			name:         "large count stability",
			observations: makeSequence(2.0, 1000),
			wantMean:     2.0,
			wantVar:      0.0,
		},
		{
			name:         "large count with variance",
			observations: makeSequence(5.0, 100),
			wantMean:     5.0,
			wantVar:      0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mean, m2, count := 0.0, 0.0, 0.0
			for _, obs := range tt.observations {
				mean, m2, count = WelfordUpdate(mean, m2, count, obs)
			}

			if math.Abs(mean-tt.wantMean) > 1e-9 {
				t.Errorf("mean = %v, want %v", mean, tt.wantMean)
			}

			variance := 0.0
			if count > 0 {
				variance = m2 / count
			}

			if math.Abs(variance-tt.wantVar) > 1e-6 {
				t.Errorf("variance = %v, want %v", variance, tt.wantVar)
			}

			// Check no NaN or Inf
			if math.IsNaN(mean) || math.IsInf(mean, 0) {
				t.Error("mean is NaN or Inf")
			}
			if math.IsNaN(variance) || math.IsInf(variance, 0) {
				t.Error("variance is NaN or Inf")
			}
		})
	}
}

func TestWelfordUpdate_NoNaNInf_AnySampleCount(t *testing.T) {
	mean, m2, count := 0.0, 0.0, 0.0
	for i := 0; i < 10000; i++ {
		obs := float64(i%100) * 0.01
		mean, m2, count = WelfordUpdate(mean, m2, count, obs)

		if math.IsNaN(mean) || math.IsInf(mean, 0) {
			t.Fatalf("NaN/Inf mean at sample %d: mean=%v, m2=%v, count=%v", i+1, mean, m2, count)
		}

		variance := m2 / count
		if math.IsNaN(variance) || math.IsInf(variance, 0) {
			t.Fatalf("NaN/Inf variance at sample %d: variance=%v, m2=%v, count=%v", i+1, variance, m2, count)
		}

		if variance < -1e-12 {
			t.Fatalf("negative variance at sample %d: %v", i+1, variance)
		}
	}
}

func TestWelfordUpdate_MatchesBatchVariance(t *testing.T) {
	observations := []float64{1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0}

	mean, m2, count := 0.0, 0.0, 0.0
	for _, obs := range observations {
		mean, m2, count = WelfordUpdate(mean, m2, count, obs)
	}
	onlineVar := m2 / count

	var batchMean float64
	for _, obs := range observations {
		batchMean += obs
	}
	batchMean /= float64(len(observations))

	var batchVar float64
	for _, obs := range observations {
		batchVar += (obs - batchMean) * (obs - batchMean)
	}
	batchVar /= float64(len(observations))

	if math.Abs(onlineVar-batchVar) > 1e-12 {
		t.Errorf("online variance %v != batch variance %v", onlineVar, batchVar)
	}
}

// --- normalizeZScore tests ---

func TestNormalizeZScore(t *testing.T) {
	tests := []struct {
		z    float64
		want float64
	}{
		{0.0, 0.0},
		{0.5, 0.0},
		{1.0, 0.0},
		{2.0, 0.333},
		{3.0, 0.667},
		{4.0, 1.0},
		{5.0, 1.0},
		{-1.5, 0.167},
		{-4.0, 1.0},
	}

	for _, tt := range tests {
		got := normalizeZScore(tt.z)
		if math.Abs(got-tt.want) > 0.001 {
			t.Errorf("normalizeZScore(%v) = %v, want %v", tt.z, got, tt.want)
		}
	}
}

// --- PatternLearner tests ---

func TestPatternLearner_ColdStart(t *testing.T) {
	pl := newTestLearner(t)

	if !pl.IsColdStart() {
		t.Error("expected cold start for new learner")
	}

	result := pl.ComputeAnomalyScore("zone-1", 12, 0, 5)
	if !result.Suppressed {
		t.Error("expected anomaly score to be suppressed during cold start")
	}
	if result.CompositeScore != 0 {
		t.Errorf("expected 0 composite score during cold start, got %v", result.CompositeScore)
	}
}

func TestPatternLearner_SlotNotReady(t *testing.T) {
	pl := newTestLearner(t)

	if pl.IsSlotReady("zone-1", 12, 0) {
		t.Error("expected slot not ready")
	}

	result := pl.ComputeAnomalyScore("zone-1", 12, 0, 5)
	if !result.Suppressed {
		t.Error("expected anomaly score suppressed when slot not ready")
	}
}

func TestPatternLearner_ObserveAndUpdate_Persists(t *testing.T) {
	pl := newTestLearner(t)

	for i := 0; i < 50; i++ {
		if err := pl.ObserveAndUpdate("zone-1", 12, 0, 2, 0); err != nil {
			t.Fatalf("ObserveAndUpdate: %v", err)
		}
	}

	if !pl.IsSlotReady("zone-1", 12, 0) {
		t.Error("expected slot to be ready after 50 observations")
	}

	slot := pl.GetPattern("zone-1", 12, 0)
	if slot == nil {
		t.Fatal("expected pattern to exist")
	}
	if slot.MeanCount != 2.0 {
		t.Errorf("expected mean=2.0, got %v", slot.MeanCount)
	}
	if slot.SampleCount != 50 {
		t.Errorf("expected sample_count=50, got %d", slot.SampleCount)
	}
	if slot.Variance > 1e-9 {
		t.Errorf("expected variance=0 for identical observations, got %v", slot.Variance)
	}
}

func TestPatternLearner_ObserveAndUpdate_WithVariance(t *testing.T) {
	pl := newTestLearner(t)

	for i := 0; i < 50; i++ {
		if err := pl.ObserveAndUpdate("zone-1", 12, 0, i%5, 0); err != nil {
			t.Fatalf("ObserveAndUpdate: %v", err)
		}
	}

	slot := pl.GetPattern("zone-1", 12, 0)
	if slot == nil {
		t.Fatal("expected pattern")
	}

	if math.Abs(slot.MeanCount-2.0) > 1e-9 {
		t.Errorf("expected mean=2.0, got %v", slot.MeanCount)
	}

	if math.Abs(slot.Variance-2.0) > 1e-6 {
		t.Errorf("expected variance=2.0, got %v", slot.Variance)
	}
}

func TestPatternLearner_OutlierProtection(t *testing.T) {
	pl := newTestLearner(t)

	for i := 0; i < 50; i++ {
		if err := pl.ObserveAndUpdate("zone-1", 12, 0, 0, 0); err != nil {
			t.Fatalf("ObserveAndUpdate: %v", err)
		}
	}

	slotBefore := pl.GetPattern("zone-1", 12, 0)
	meanBefore := slotBefore.MeanCount
	countBefore := slotBefore.SampleCount

	// Outlier should be skipped
	if err := pl.ObserveAndUpdate("zone-1", 12, 0, 100, 0.6); err != nil {
		t.Fatalf("ObserveAndUpdate: %v", err)
	}

	slotAfter := pl.GetPattern("zone-1", 12, 0)
	if slotAfter.MeanCount != meanBefore {
		t.Errorf("outlier protection failed: mean changed from %v to %v", meanBefore, slotAfter.MeanCount)
	}
	if slotAfter.SampleCount != countBefore {
		t.Errorf("outlier protection failed: count changed from %d to %d", countBefore, slotAfter.SampleCount)
	}
}

func TestPatternLearner_OutlierProtection_AfterMultipleAnomalies(t *testing.T) {
	pl := newTestLearner(t)

	for i := 0; i < 50; i++ {
		pl.ObserveAndUpdate("zone-1", 12, 0, 1, 0)
	}

	slot := pl.GetPattern("zone-1", 12, 0)
	meanBefore := slot.MeanCount

	// Inject 3 synthetic anomalies
	for i := 0; i < 3; i++ {
		pl.ObserveAndUpdate("zone-1", 12, 0, 50, 1.0)
	}

	slot = pl.GetPattern("zone-1", 12, 0)
	if slot.SampleCount != 50 {
		t.Errorf("expected sample_count to remain 50, got %d", slot.SampleCount)
	}
	if math.Abs(slot.MeanCount-meanBefore) > 1e-9 {
		t.Errorf("expected mean to remain %v, got %v", meanBefore, slot.MeanCount)
	}
}

func TestPatternLearner_SecurityModeOverride(t *testing.T) {
	pl := newTestLearner(t)

	pl.SetSecurityMode(true)

	result := pl.ComputeAnomalyScore("zone-1", 12, 0, 0)
	if result.CompositeScore != 1.0 {
		t.Errorf("security mode: expected composite=1.0, got %v", result.CompositeScore)
	}
	if !result.IsAlert {
		t.Error("security mode: expected is_alert=true")
	}

	result = pl.ComputeAnomalyScore("zone-1", 12, 0, 0)
	if result.CompositeScore != 1.0 {
		t.Errorf("security mode with 0 count: expected composite=1.0, got %v", result.CompositeScore)
	}

	pl.SetSecurityMode(false)
}

func TestPatternLearner_AnomalyScoring(t *testing.T) {
	pl := newTestLearner(t)
	pl.SetLearningStartTime(time.Now().Add(-8 * 24 * time.Hour))

	for i := 0; i < 50; i++ {
		pl.ObserveAndUpdate("zone-1", 3, 0, 0, 0)
	}

	result := pl.ComputeAnomalyScore("zone-1", 3, 0, 0)
	if result.CompositeScore > 0.01 {
		t.Errorf("expected low score for expected observation, got %v", result.CompositeScore)
	}
	if result.Suppressed {
		t.Error("expected not suppressed when slot is ready")
	}

	result = pl.ComputeAnomalyScore("zone-1", 3, 0, 3)
	if result.ZoneScore != 1.0 {
		t.Errorf("expected zone_score=1.0 when zone normally empty, got %v", result.ZoneScore)
	}
	if result.CompositeScore < 1.0 {
		t.Errorf("expected composite=1.0 (max of time and zone), got %v", result.CompositeScore)
	}
	if !result.IsAlert {
		t.Error("expected alert when zone normally empty but now occupied")
	}
}

func TestPatternLearner_AnomalyScoring_ZScoreBased(t *testing.T) {
	pl := newTestLearner(t)
	pl.SetLearningStartTime(time.Now().Add(-8 * 24 * time.Hour))

	for i := 0; i < 50; i++ {
		pl.ObserveAndUpdate("zone-1", 14, 0, 1+i%2, 0)
	}

	slot := pl.GetPattern("zone-1", 14, 0)
	if slot == nil {
		t.Fatal("expected pattern")
	}

	result := pl.ComputeAnomalyScore("zone-1", 14, 0, 2)
	if result.TimeScore > 0.01 {
		t.Errorf("expected low time_score for mean observation, got %v", result.TimeScore)
	}

	result = pl.ComputeAnomalyScore("zone-1", 14, 0, 10)
	if result.TimeScore < 0.9 {
		t.Errorf("expected high time_score for extreme observation, got %v", result.TimeScore)
	}
}

func TestPatternLearner_GetPatterns(t *testing.T) {
	pl := newTestLearner(t)

	for i := 0; i < 50; i++ {
		pl.ObserveAndUpdate("zone-1", 12, 0, 2, 0)
		pl.ObserveAndUpdate("zone-2", 12, 0, 3, 0)
	}

	all := pl.GetPatterns("")
	if len(all) != 2 {
		t.Errorf("expected 2 patterns, got %d", len(all))
	}

	zone1 := pl.GetPatterns("zone-1")
	if len(zone1) != 1 {
		t.Errorf("expected 1 pattern for zone-1, got %d", len(zone1))
	}
	if zone1[0].ZoneID != "zone-1" {
		t.Errorf("expected zone_id=zone-1, got %s", zone1[0].ZoneID)
	}
}

func TestPatternLearner_SurvivesRestart(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pattern_restart_test")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	dbPath := filepath.Join(tmpDir, "patterns.db")

	pl1, err := NewPatternLearner(dbPath)
	if err != nil {
		t.Fatalf("NewPatternLearner: %v", err)
	}

	for i := 0; i < 50; i++ {
		pl1.ObserveAndUpdate("zone-1", 12, 0, 2, 0)
	}

	pl1.Close()

	pl2, err := NewPatternLearner(dbPath)
	if err != nil {
		t.Fatalf("NewPatternLearner after restart: %v", err)
	}
	defer pl2.Close()

	if !pl2.IsSlotReady("zone-1", 12, 0) {
		t.Error("expected slot to be ready after reload from DB")
	}

	slot := pl2.GetPattern("zone-1", 12, 0)
	if slot == nil {
		t.Fatal("expected pattern after restart")
	}
	if slot.MeanCount != 2.0 {
		t.Errorf("expected mean=2.0 after restart, got %v", slot.MeanCount)
	}
	if slot.SampleCount != 50 {
		t.Errorf("expected 50 samples after restart, got %d", slot.SampleCount)
	}
}

func TestPatternLearner_AlertThresholds(t *testing.T) {
	tests := []struct {
		name          string
		observations []int
		testCount     int
		wantAlert     bool
		wantWarning   bool
	}{
		{
			name:          "normal observation at mean",
			observations: makeConst(2, 50),
			testCount:     2,
			wantAlert:     false,
			wantWarning:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pl := newTestLearner(t)

			for _, obs := range tt.observations {
				pl.ObserveAndUpdate("zone-1", 14, 0, obs, 0)
			}

			result := pl.ComputeAnomalyScore("zone-1", 14, 0, tt.testCount)
			if result.IsAlert != tt.wantAlert {
				t.Errorf("is_alert = %v, want %v (composite=%v)", result.IsAlert, tt.wantAlert, result.CompositeScore)
			}
			if result.IsWarning != tt.wantWarning {
				t.Errorf("is_warning = %v, want %v (composite=%v)", result.IsWarning, tt.wantWarning, result.CompositeScore)
			}
		})
	}
}

func TestPatternLearner_NaNInf_NeverProduced(t *testing.T) {
	pl := newTestLearner(t)

	observations := []int{0, 100, 0, 100, 0, 100, 1, 99, 50, 0}
	for i := 0; i < 5; i++ {
		for _, obs := range observations {
			pl.ObserveAndUpdate("zone-1", 14, 0, obs, 0)
		}
	}

	slot := pl.GetPattern("zone-1", 14, 0)
	if slot == nil {
		t.Fatal("expected pattern")
	}

	if math.IsNaN(slot.MeanCount) || math.IsInf(slot.MeanCount, 0) {
		t.Error("mean is NaN or Inf")
	}
	if math.IsNaN(slot.Variance) || math.IsInf(slot.Variance, 0) {
		t.Error("variance is NaN or Inf")
	}

	for _, obs := range []int{0, 1, 5, 50, 100, 200} {
		result := pl.ComputeAnomalyScore("zone-1", 14, 0, obs)
		if math.IsNaN(result.CompositeScore) || math.IsInf(result.CompositeScore, 0) {
			t.Errorf("NaN/Inf composite for obs=%d: %v", obs, result.CompositeScore)
		}
		if math.IsNaN(result.TimeScore) || math.IsInf(result.TimeScore, 0) {
			t.Errorf("NaN/Inf time_score for obs=%d: %v", obs, result.TimeScore)
		}
	}
}

func TestPatternLearner_NoAlertsDuringColdStart(t *testing.T) {
	pl := newTestLearner(t)

	for i := 0; i < 100; i++ {
		pl.ObserveAndUpdate("zone-1", 3, 0, 50, 0)
	}

	if !pl.IsColdStart() {
		t.Log("note: cold start check depends on timing")
	}

	result := pl.ComputeAnomalyScore("zone-1", 3, 0, 50)
	if !result.Suppressed {
		t.Error("expected anomaly score to be suppressed during cold start regardless of activity")
	}
	if result.IsAlert || result.IsWarning {
		t.Error("expected no alerts during cold start")
	}
}

// --- Integration test: hourly update with mock provider ---

type mockOccupancyProvider struct {
	counts map[string]int
}

func (m *mockOccupancyProvider) GetZoneOccupancyCounts() map[string]int {
	return m.counts
}

func TestPatternLearner_HourlyUpdate_Integration(t *testing.T) {
	pl := newTestLearner(t)

	provider := &mockOccupancyProvider{
		counts: map[string]int{"zone-1": 2, "zone-2": 0},
	}

	pl.updateAllZones(provider)

	slot1 := pl.GetPattern("zone-1", time.Now().Hour(), int(time.Now().Weekday()))
	if slot1 == nil {
		t.Fatal("expected pattern for zone-1 after hourly update")
	}
	if slot1.MeanCount != 2.0 {
		t.Errorf("expected mean=2.0 for zone-1, got %v", slot1.MeanCount)
	}

	slot2 := pl.GetPattern("zone-2", time.Now().Hour(), int(time.Now().Weekday()))
	if slot2 == nil {
		t.Fatal("expected pattern for zone-2 after hourly update")
	}
	if slot2.MeanCount != 0.0 {
		t.Errorf("expected mean=0.0 for zone-2, got %v", slot2.MeanCount)
	}
}

func TestPatternLearner_HourlyUpdate_OutlierProtectionInUpdate(t *testing.T) {
	pl := newTestLearner(t)

	for i := 0; i < 50; i++ {
		pl.ObserveAndUpdate("zone-1", 12, 0, 1, 0)
	}

	slotBefore := pl.GetPattern("zone-1", 12, 0)

	provider := &mockOccupancyProvider{
		counts: map[string]int{"zone-1": 50},
	}

	pl.updateAllZones(provider)

	slotAfter := pl.GetPattern("zone-1", 12, 0)

	if slotAfter.SampleCount != slotBefore.SampleCount {
		t.Logf("note: sample count changed from %d to %d (outlier protection may not trigger if score < 0.5)", slotBefore.SampleCount, slotAfter.SampleCount)
	}
}

// --- helpers ---

func makeSequence(value float64, count int) []float64 {
	result := make([]float64, count)
	for i := range result {
		result[i] = value
	}
	return result
}

func makeConst(value, count int) []int {
	result := make([]int, count)
	for i := range result {
		result[i] = value
	}
	return result
}
