package sleep

import (
	"testing"
	"time"
)

func TestSleepStateString(t *testing.T) {
	tests := []struct {
		state    SleepState
		expected string
	}{
		{SleepStateAwake, "awake"},
		{SleepStateFallingAsleep, "falling_asleep"},
		{SleepStateLightSleep, "light_sleep"},
		{SleepStateDeepSleep, "deep_sleep"},
		{SleepStateREM, "rem"},
		{SleepStateRestless, "restless"},
		{SleepState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("SleepState.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSleepStateMarshalText(t *testing.T) {
	state := SleepStateDeepSleep
	data, err := state.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText() error = %v", err)
	}
	if string(data) != "deep_sleep" {
		t.Errorf("MarshalText() = %s, want deep_sleep", string(data))
	}
}

func TestNewSleepAnalyzer(t *testing.T) {
	sa := NewSleepAnalyzer()
	if sa == nil {
		t.Fatal("NewSleepAnalyzer() returned nil")
	}
}

func TestSleepAnalyzerSetSleepWindow(t *testing.T) {
	sa := NewSleepAnalyzer()
	sa.SetSleepWindow(23, 8)
	// Verify it doesn't panic
}

func TestSleepSessionIsSleepHours(t *testing.T) {
	tests := []struct {
		name      string
		startHour int
		endHour   int
		testHour  int
		expected  bool
	}{
		{"during night window", 22, 7, 23, true},
		{"after midnight", 22, 7, 2, true},
		{"during morning outside window", 22, 7, 9, false},
		{"at start boundary", 22, 7, 22, true},
		{"at end boundary", 22, 7, 7, false},
		{"past midnight in window", 22, 7, 3, true},
		{"window within day - inside", 12, 18, 14, true},
		{"window within day - outside", 12, 18, 10, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ss := NewSleepSession("test", tt.startHour, tt.endHour)
			testTime := time.Date(2024, 1, 15, tt.testHour, 0, 0, 0, time.Local)
			got := ss.isSleepHours(testTime)
			if got != tt.expected {
				t.Errorf("isSleepHours(%d:00) = %v, want %v", tt.testHour, got, tt.expected)
			}
		})
	}
}

func TestCalculateBreathingScore(t *testing.T) {
	tests := []struct {
		name    string
		avg     float64
		stdDev  float64
		min     float64
		max     float64
		wantMin float64
		wantMax float64
	}{
		{"optimal breathing", 14.0, 1.0, 12.0, 16.0, 80, 100},
		{"slightly high rate", 18.0, 2.0, 14.0, 22.0, 60, 90},
		{"high variability", 14.0, 5.0, 10.0, 20.0, 40, 70},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ss := NewSleepSession("test", 22, 7)
			score := ss.calculateBreathingScore(tt.avg, tt.stdDev, tt.min, tt.max)

			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("calculateBreathingScore() = %.1f, want between %.1f and %.1f",
					score, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestSleepReportGeneration(t *testing.T) {
	ss := NewSleepSession("test-link", 22, 7)

	// Add samples during sleep hours
	baseTime := time.Date(2024, 1, 15, 23, 0, 0, 0, time.Local)

	for i := 0; i < 100; i++ {
		timestamp := baseTime.Add(time.Duration(i) * time.Minute)

		// Add breathing sample
		ss.processBreathing(BreathingSample{
			Timestamp:  timestamp,
			RateBPM:    14.0 + float64(i%5-2), // Vary between 12-16
			Confidence: 0.8,
			IsDetected: true,
		})

		// Add motion sample (mostly quiet)
		deltaRMS := 0.01
		motionDetected := false
		if i%20 == 0 {
			deltaRMS = 0.05
			motionDetected = true
		}

		ss.processMotion(MotionSample{
			Timestamp:      timestamp,
			DeltaRMS:       deltaRMS,
			MotionDetected: motionDetected,
		})
	}

	report := ss.GenerateReport()
	if report == nil {
		t.Fatal("GenerateReport() returned nil")
	}

	if report.LinkID != "test-link" {
		t.Errorf("LinkID = %s, want test-link", report.LinkID)
	}

	if report.Metrics == nil {
		t.Fatal("Metrics is nil")
	}

	if report.Metrics.OverallScore < 0 || report.Metrics.OverallScore > 100 {
		t.Errorf("OverallScore = %.1f, expected 0-100", report.Metrics.OverallScore)
	}

	if report.BreathingSummary == "" {
		t.Error("BreathingSummary is empty")
	}

	if report.MotionSummary == "" {
		t.Error("MotionSummary is empty")
	}

	if len(report.Recommendations) == 0 {
		t.Error("Expected at least one recommendation")
	}
}

func TestSleepReportToJSONMap(t *testing.T) {
	ss := NewSleepSession("test-link", 22, 7)

	baseTime := time.Date(2024, 1, 15, 23, 0, 0, 0, time.Local)
	for i := 0; i < 50; i++ {
		timestamp := baseTime.Add(time.Duration(i) * time.Minute)
		ss.processBreathing(BreathingSample{
			Timestamp:  timestamp,
			RateBPM:    14.0,
			Confidence: 0.8,
			IsDetected: true,
		})
		ss.processMotion(MotionSample{
			Timestamp:      timestamp,
			DeltaRMS:       0.01,
			MotionDetected: false,
		})
	}

	report := ss.GenerateReport()
	if report == nil {
		t.Fatal("GenerateReport returned nil")
	}
	jsonMap := report.ToJSONMap()

	requiredFields := []string{
		"link_id",
		"session_date",
		"generated_at",
		"overall_score",
		"quality_rating",
		"breathing_summary",
		"motion_summary",
		"recommendations",
		"metrics",
	}

	for _, field := range requiredFields {
		if _, exists := jsonMap[field]; !exists {
			t.Errorf("Missing field: %s", field)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{30 * time.Second, "30 seconds"},
		{90 * time.Second, "1 minute"},
		{150 * time.Second, "2 minutes"},
		{2 * time.Hour, "2 hours"},
		{2*time.Hour + 30*time.Minute, "2 hours 30 minutes"},
		{1 * time.Hour, "1 hour"},
		{1 * time.Minute, "1 minute"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := FormatDuration(tt.duration)
			if got != tt.expected {
				t.Errorf("FormatDuration(%v) = %s, want %s", tt.duration, got, tt.expected)
			}
		})
	}
}

func TestSleepSessionReset(t *testing.T) {
	ss := NewSleepSession("test", 22, 7)

	// Add data
	now := time.Date(2024, 1, 15, 23, 30, 0, 0, time.Local)
	ss.processBreathing(BreathingSample{
		Timestamp:  now,
		RateBPM:    14.0,
		Confidence: 0.8,
		IsDetected: true,
	})
	ss.processMotion(MotionSample{
		Timestamp:      now,
		DeltaRMS:       0.01,
		MotionDetected: false,
	})

	// Reset
	ss.Reset()

	if ss.currentState != SleepStateAwake {
		t.Errorf("State after reset = %v, want awake", ss.currentState)
	}
	if ss.isActive {
		t.Error("isActive should be false after reset")
	}
	if len(ss.breathingSamples) != 0 {
		t.Errorf("breathingSamples not cleared, len=%d", len(ss.breathingSamples))
	}
	if len(ss.motionSamples) != 0 {
		t.Errorf("motionSamples not cleared, len=%d", len(ss.motionSamples))
	}
}

func TestReportCallback(t *testing.T) {
	sa := NewSleepAnalyzer()
	sa.SetSleepWindow(22, 7)

	var callbackCalled bool
	var callbackLinkID string
	var callbackReport *SleepReport

	sa.SetReportCallback(func(linkID string, report *SleepReport) {
		callbackCalled = true
		callbackLinkID = linkID
		callbackReport = report
	})

	// Add samples during sleep hours
	now := time.Date(2024, 1, 15, 23, 30, 0, 0, time.Local)
	for i := 0; i < 50; i++ {
		sa.ProcessBreathing("link1", BreathingSample{
			Timestamp:  now.Add(time.Duration(i) * time.Minute),
			RateBPM:    14.0,
			Confidence: 0.8,
			IsDetected: true,
		})
		sa.ProcessMotion("link1", MotionSample{
			Timestamp:      now.Add(time.Duration(i) * time.Minute),
			DeltaRMS:       0.01,
			MotionDetected: false,
		})
	}

	// Generate reports
	reports := sa.GenerateMorningReports()

	if len(reports) == 0 {
		t.Fatal("No reports generated")
	}

	if !callbackCalled {
		t.Error("Callback was not called")
	}
	if callbackLinkID != "link1" {
		t.Errorf("Callback linkID = %s, want link1", callbackLinkID)
	}
	if callbackReport == nil {
		t.Error("Callback report is nil")
	}
}

func TestGenerateBreathingSummary(t *testing.T) {
	tests := []struct {
		name    string
		metrics *SleepMetrics
	}{
		{
			name: "optimal breathing",
			metrics: &SleepMetrics{
				AvgBreathingRate:    14.0,
				BreathingRateStdDev: 1.0,
				MinBreathingRate:    12.0,
				MaxBreathingRate:    16.0,
			},
		},
		{
			name: "high breathing rate",
			metrics: &SleepMetrics{
				AvgBreathingRate:    26.0,
				BreathingRateStdDev: 2.0,
				MinBreathingRate:    22.0,
				MaxBreathingRate:    30.0,
			},
		},
		{
			name: "high variability",
			metrics: &SleepMetrics{
				AvgBreathingRate:    14.0,
				BreathingRateStdDev: 4.0,
				MinBreathingRate:    10.0,
				MaxBreathingRate:    20.0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := generateBreathingSummary(tt.metrics)
			if summary == "" {
				t.Error("Summary is empty")
			}
		})
	}
}

func TestGenerateMotionSummary(t *testing.T) {
	tests := []struct {
		name    string
		metrics *SleepMetrics
	}{
		{
			name: "very restful",
			metrics: &SleepMetrics{
				TimeInBed:       8 * time.Hour,
				QuietTimePct:    90,
				MotionEvents:    3,
				RestlessPeriods: 1,
			},
		},
		{
			name: "restless night",
			metrics: &SleepMetrics{
				TimeInBed:       7 * time.Hour,
				QuietTimePct:    50,
				MotionEvents:    25,
				RestlessPeriods: 10,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := generateMotionSummary(tt.metrics)
			if summary == "" {
				t.Error("Summary is empty")
			}
		})
	}
}
