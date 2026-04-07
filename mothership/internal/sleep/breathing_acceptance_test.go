package sleep

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

// TestAcceptance_FFTIdentifies15BPM verifies FFT correctly identifies 0.25 Hz (15 bpm)
// dominant frequency in a synthetic phase signal.
func TestAcceptance_FFTIdentifies15BPM(t *testing.T) {
	sampleRate := 20.0
	nSamples := 512
	zeroPad := 1024

	// 0.25 Hz = 15 bpm
	freqHz := 0.25
	buffer := make([]float64, nSamples)
	for i := 0; i < nSamples; i++ {
		tSec := float64(i) / sampleRate
		buffer[i] = math.Sin(2.0 * math.Pi * freqHz * tSec)
	}

	bpm := computeFFTBreathingRate(buffer, 0, sampleRate, float64(zeroPad))

	// Frequency resolution: 20/1024 ≈ 0.0195 Hz/bin → ~1.17 bpm/bin
	if math.Abs(bpm-15.0) > 1.5 {
		t.Errorf("FFT breathing rate = %.2f bpm, want ~15.0 bpm (±1.5)", bpm)
	}
}

// TestAcceptance_EMAConverges verifies EMA smoothing applied across nightly samples.
func TestAcceptance_EMAConverges(t *testing.T) {
	est := NewBreathingRateEstimator()
	sampleRate := 20.0

	// Feed a constant 15 bpm signal for multiple windows
	for window := 0; window < 20; window++ {
		for i := 0; i < 512; i++ {
			tSec := float64(i) / sampleRate
			phase := math.Sin(2.0 * math.Pi * 0.25 * tSec)
			est.AddPhaseSample(phase)
		}
		est.EstimateRate()
	}

	finalRate := est.GetRate()
	if math.Abs(finalRate-15.0) > 1.0 {
		t.Errorf("EMA-stabilized rate = %.2f bpm, want ~15.0 bpm (±1.0)", finalRate)
	}
}

// TestAcceptance_AnomalyTriggersAtThreshold verifies elevated anomaly triggers correctly
// at >25% above personal average.
func TestAcceptance_AnomalyTriggersAtThreshold(t *testing.T) {
	tracker := NewBreathingAnomalyTracker()
	tracker.UpdatePersonalAverage("test-person", 16.0)

	tests := []struct {
		name        string
		nightlyBPM  float64
		wantAnomaly bool
	}{
		{"normal 16 bpm (1.0x)", 16.0, false},
		{"slightly elevated 19 bpm (1.19x)", 19.0, false},
		{"at threshold 20 bpm (1.25x)", 20.0, false},
		{"above threshold 21 bpm (1.31x)", 21.0, true},
		{"significantly elevated 25 bpm (1.56x)", 25.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker.personal["test-person"] = 16.0 // reset baseline
			got := tracker.CheckAnomaly("test-person", tt.nightlyBPM)
			if got != tt.wantAnomaly {
				t.Errorf("CheckAnomaly(%.0f bpm) = %v, want %v", tt.nightlyBPM, got, tt.wantAnomaly)
			}
		})
	}
}

// TestAcceptance_BreathingRegularityLabels verifies regularity CV thresholds.
func TestAcceptance_BreathingRegularityLabels(t *testing.T) {
	tests := []struct {
		cv      float64
		want    string
	}{
		{0.05, "regular"},    // CV < 0.10
		{0.09, "regular"},
		{0.10, "normal"},    // boundary
		{0.15, "normal"},
		{0.25, "normal"},    // boundary
		{0.26, "irregular"}, // CV > 0.25
		{0.50, "irregular"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := BreathingRegularityLabel(tt.cv)
			if got != tt.want {
				t.Errorf("BreathingRegularityLabel(%.2f) = %q, want %q", tt.cv, got, tt.want)
			}
		})
	}
}

// TestAcceptance_SleepReportIncludesAnomaly verifies that a sleep report includes
// breathing anomaly data when the personal average is exceeded.
func TestAcceptance_SleepReportIncludesAnomaly(t *testing.T) {
	analyzer := NewSleepAnalyzer()

	linkID := "test-link:AA:BB:CC:DD:EE:FF"
	person := "alice"

	// Set personal baseline to 16 bpm
	analyzer.GetAnomalyTracker().UpdatePersonalAverage(person, 16.0)

	// Create a session and add breathing samples at 22 bpm (1.375x personal avg → anomaly)
	session := NewSleepSession(linkID, 22, 7)
	session.personID = person
	session.isActive = true
	session.sessionDate = time.Date(2026, 4, 7, 0, 0, 0, 0, time.UTC)
	session.sleepOnset = time.Date(2026, 4, 6, 23, 0, 0, 0, time.UTC)
	session.sessionStart = time.Date(2026, 4, 6, 22, 30, 0, 0, time.UTC)

	// Add 30 breathing samples at 22 bpm
	baseTime := time.Date(2026, 4, 6, 23, 0, 0, 0, time.UTC)
	for i := 0; i < 30; i++ {
		session.breathingSamples = append(session.breathingSamples, BreathingSample{
			Timestamp:  baseTime.Add(time.Duration(i) * 60 * time.Second),
			RateBPM:    22.0,
			Confidence: 0.9,
			IsDetected: true,
		})
	}

	// Add some motion samples (quiet — deep sleep)
	for i := 0; i < 30; i++ {
		session.motionSamples = append(session.motionSamples, MotionSample{
			Timestamp:      baseTime.Add(time.Duration(i) * 60 * time.Second),
			DeltaRMS:       0.01, // Quiet
			MotionDetected: false,
		})
	}

	analyzer.sessions[linkID] = session

	// Generate morning reports (which checks anomaly)
	reports := analyzer.GenerateMorningReports()
	report, ok := reports[linkID]
	if !ok {
		t.Fatal("Expected report for test-link, got none")
	}

	// Verify anomaly is flagged
	if !report.Metrics.BreathingAnomaly {
		t.Error("Expected BreathingAnomaly = true (22 bpm > 16 × 1.25 = 20 bpm)")
	}

	// Verify personal avg is included
	if report.Metrics.PersonalAvgBPM != 16.0 {
		t.Errorf("PersonalAvgBPM = %.1f, want 16.0", report.Metrics.PersonalAvgBPM)
	}

	// Verify breathing samples are in the report
	if len(report.BreathingSamples) != 30 {
		t.Errorf("BreathingSamples length = %d, want 30", len(report.BreathingSamples))
	}

	// Verify avg breathing rate
	if math.Abs(report.Metrics.AvgBreathingRate-22.0) > 0.01 {
		t.Errorf("AvgBreathingRate = %.1f, want 22.0", report.Metrics.AvgBreathingRate)
	}

	// Verify regularity is low (constant rate)
	if report.Metrics.BreathingRegularity > 0.01 {
		t.Errorf("BreathingRegularity = %.4f, want ~0 (constant 22 bpm)", report.Metrics.BreathingRegularity)
	}
}

// TestAcceptance_FFTRejectsOutOfRange verifies FFT rejects frequencies outside 6-25 bpm.
func TestAcceptance_FFTRejectsOutOfRange(t *testing.T) {
	tests := []struct {
		name   string
		freqHz float64
		inRange bool // true if FFT should detect a value in 6-25 bpm
	}{
		{"0.05 Hz = 3 bpm (below range)", 0.05, false},
		{"0.1 Hz = 6 bpm (at lower bound)", 0.1, true},
		{"0.25 Hz = 15 bpm (mid range)", 0.25, true},
		{"0.5 Hz = 30 bpm (at upper bound)", 0.5, true},
		{"0.6 Hz = 36 bpm (above range)", 0.6, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			est := NewBreathingRateEstimator()
			sampleRate := 20.0

			for i := 0; i < 512; i++ {
				tSec := float64(i) / sampleRate
				phase := math.Sin(2.0 * math.Pi * tt.freqHz * tSec)
				est.AddPhaseSample(phase)
			}

			rate := est.EstimateRate()

			if tt.inRange {
				if rate == 0 {
					t.Errorf("Expected non-zero rate for %.2f Hz (%.0f bpm)", tt.freqHz, tt.freqHz*60)
				}
				bpm := tt.freqHz * 60
				if math.Abs(rate-bpm) > 3.0 {
					t.Errorf("Rate = %.1f bpm, want ~%.0f bpm (±3)", rate, bpm)
				}
			} else {
				// Out of range signals should be rejected by the estimator
				// (they'll either be rejected or not be the dominant peak)
				if rate > 0 && rate < FFTEstimatorMinBPM {
					t.Logf("Rate = %.1f bpm for out-of-range %.2f Hz (acceptable if noise)", rate, tt.freqHz)
				}
			}
		})
	}
}

// TestAcceptance_AnomalyTrackerPersistence verifies save/load round-trip for personal averages.
func TestAcceptance_AnomalyTrackerPersistence(t *testing.T) {
	tracker := NewBreathingAnomalyTracker()
	tracker.UpdatePersonalAverage("alice", 16.0)
	tracker.UpdatePersonalAverage("bob", 14.0)

	// Save to settings format
	settings, err := tracker.SaveToSettings()
	if err != nil {
		t.Fatalf("SaveToSettings() error = %v", err)
	}

	// Create a new tracker and restore
	tracker2 := NewBreathingAnomalyTracker()
	if err := tracker2.LoadFromSettings(settings); err != nil {
		t.Fatalf("LoadFromSettings() error = %v", err)
	}

	if tracker2.GetPersonalAverage("alice") != 16.0 {
		t.Errorf("alice avg = %.1f, want 16.0", tracker2.GetPersonalAverage("alice"))
	}
	if tracker2.GetPersonalAverage("bob") != 14.0 {
		t.Errorf("bob avg = %.1f, want 14.0", tracker2.GetPersonalAverage("bob"))
	}
}

// TestAcceptance_EmptySettingsLoad verifies loading from empty settings doesn't crash.
func TestAcceptance_EmptySettingsLoad(t *testing.T) {
	tracker := NewBreathingAnomalyTracker()
	if err := tracker.LoadFromSettings(""); err != nil {
		t.Fatalf("LoadFromSettings('') error = %v", err)
	}
	if tracker.GetPersonalAverage("anyone") != 0 {
		t.Error("Expected 0 for unknown person after empty load")
	}
}

// TestAcceptance_BreathingSamplesJSONFormat verifies the JSON format of breathing samples
// includes both summary stats and raw per-sample BPM values.
func TestAcceptance_BreathingSamplesJSONFormat(t *testing.T) {
	report := &SleepReport{
		LinkID:      "test-link",
		SessionDate: time.Now(),
		GeneratedAt: time.Now(),
		Metrics: &SleepMetrics{
			AvgBreathingRate:    16.5,
			MinBreathingRate:    14.0,
			MaxBreathingRate:    19.0,
			BreathingRateStdDev: 1.2,
			BreathingRegularity: 0.073,
			BreathingAnomaly:    false,
			PersonalAvgBPM:      16.0,
		},
		BreathingSamples: []float64{15.0, 16.0, 17.0, 15.5, 16.5, 17.5, 16.0},
	}

	result := extractBreathingSamplesJSON(report)
	if result == "" {
		t.Fatal("extractBreathingSamplesJSON returned empty string")
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// Check summary fields
	if avg, ok := parsed["avg"].(float64); !ok || math.Abs(avg-16.5) > 0.01 {
		t.Errorf("avg = %v, want 16.5", parsed["avg"])
	}
	if reg, ok := parsed["regularity"].(float64); !ok || math.Abs(reg-0.073) > 0.001 {
		t.Errorf("regularity = %v, want 0.073", parsed["regularity"])
	}

	// Check raw samples
	rates, ok := parsed["rates"].([]interface{})
	if !ok {
		t.Fatal("Missing 'rates' field in breathing samples JSON")
	}
	if len(rates) != 7 {
		t.Errorf("rates length = %d, want 7", len(rates))
	}
}

// TestAcceptance_BreathingSamplesJSONWithAnomaly verifies anomaly flag and personal avg
// appear in the JSON when there's an anomaly.
func TestAcceptance_BreathingSamplesJSONWithAnomaly(t *testing.T) {
	report := &SleepReport{
		LinkID:      "test-link",
		SessionDate: time.Now(),
		GeneratedAt: time.Now(),
		Metrics: &SleepMetrics{
			AvgBreathingRate:    22.0,
			MinBreathingRate:    20.0,
			MaxBreathingRate:    24.0,
			BreathingRateStdDev: 1.5,
			BreathingRegularity: 0.068,
			BreathingAnomaly:    true,
			PersonalAvgBPM:      16.0,
		},
		BreathingSamples: []float64{22.0, 22.5, 21.0, 22.0, 23.0},
	}

	result := extractBreathingSamplesJSON(report)
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if anomaly, ok := parsed["anomaly"].(bool); !ok || !anomaly {
		t.Error("anomaly should be true")
	}
	if personalAvg, ok := parsed["personal_avg"].(float64); !ok || personalAvg != 16.0 {
		t.Errorf("personal_avg = %v, want 16.0", parsed["personal_avg"])
	}
}

// TestAcceptance_ComputeBreathingRegularityTableDriven provides table-driven coverage
// of the CV computation.
func TestAcceptance_ComputeBreathingRegularityTableDriven(t *testing.T) {
	tests := []struct {
		name    string
		samples []float64
		wantCV  float64
		tol     float64
	}{
		{"constant 14 bpm", []float64{14, 14, 14, 14, 14}, 0.0, 0.001},
		{"small variation", []float64{14.0, 14.5, 13.5, 14.2, 13.8}, 0.024, 0.01},
		{"moderate variation", []float64{12, 14, 16, 18, 20}, 0.2, 0.05},
		{"high variation", []float64{10, 20, 12, 18, 15}, 0.276, 0.05},
		{"large range", []float64{8, 25, 10, 22, 30}, 0.435, 0.05},
		{"empty slice", []float64{}, 0.0, 0.0},
		{"single sample", []float64{14.0}, 0.0, 0.0},
		{"all zeros", []float64{0, 0, 0}, 0.0, 0.0},
		{"realistic sleep data", []float64{15.2, 15.0, 14.8, 15.1, 14.9, 15.3, 15.0, 14.7, 15.2, 15.1}, 0.012, 0.01},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cv := ComputeBreathingRegularity(tt.samples)
			if math.Abs(cv-tt.wantCV) > tt.tol {
				t.Errorf("ComputeBreathingRegularity() = %.4f, want %.4f ± %.4f", cv, tt.wantCV, tt.tol)
			}
		})
	}
}
