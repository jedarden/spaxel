package sleep

import (
	"encoding/json"
	"testing"
)

func TestBreathingAnomalyTrackerCheckAnomaly(t *testing.T) {
	tracker := NewBreathingAnomalyTracker()

	// Establish a personal average of 16 bpm
	tracker.UpdatePersonalAverage("alice", 16.0)

	tests := []struct {
		name       string
		avgBPM     float64
		personal   float64
		wantAnomaly bool
	}{
		{"normal rate", 16.0, 16.0, false},
		{"slightly elevated", 19.0, 16.0, false},       // 19/16 = 1.1875 < 1.25
		{"at threshold", 20.0, 16.0, false},             // 20/16 = 1.25 = threshold (not >)
		{"above threshold", 21.0, 16.0, true},            // 21/16 = 1.3125 > 1.25
		{"significantly elevated", 25.0, 16.0, true},     // 25/16 = 1.5625
		{"below average", 12.0, 16.0, false},             // 12/16 = 0.75
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset and set personal average
			tracker.personal["alice"] = tt.personal
			got := tracker.CheckAnomaly("alice", tt.avgBPM)
			if got != tt.wantAnomaly {
				t.Errorf("CheckAnomaly(%s, %.1f) = %v, want %v (personal=%.1f)",
					"alice", tt.avgBPM, got, tt.wantAnomaly, tt.personal)
			}
		})
	}
}

func TestBreathingAnomalyTrackerNoBaseline(t *testing.T) {
	tracker := NewBreathingAnomalyTracker()

	// No personal average set — should not flag anomaly
	if tracker.CheckAnomaly("bob", 25.0) {
		t.Error("CheckAnomaly should return false when no baseline exists")
	}

	if tracker.GetPersonalAverage("bob") != 0 {
		t.Error("GetPersonalAverage should return 0 for unknown person")
	}
}

func TestBreathingAnomalyTrackerEmaUpdate(t *testing.T) {
	tracker := NewBreathingAnomalyTracker()

	// Night 1: 16 bpm
	tracker.UpdatePersonalAverage("alice", 16.0)
	avg1 := tracker.GetPersonalAverage("alice")

	// Night 2: 18 bpm — EMA should pull average up slightly
	tracker.UpdatePersonalAverage("alice", 18.0)
	avg2 := tracker.GetPersonalAverage("alice")

	if avg2 <= avg1 {
		t.Errorf("Personal average should increase: before=%.2f, after=%.2f", avg1, avg2)
	}

	// EMA formula: avg = 0.05 * 18 + 0.95 * 16 = 0.9 + 15.2 = 16.1
	expected := 0.05*18.0 + 0.95*16.0
	if mathAbs(avg2-expected) > 0.001 {
		t.Errorf("EMA = %.4f, want %.4f", avg2, expected)
	}
}

func TestBreathingAnomalyTrackerZeroBPMIgnored(t *testing.T) {
	tracker := NewBreathingAnomalyTracker()
	tracker.UpdatePersonalAverage("alice", 16.0)

	// Zero BPM should not update the personal average
	tracker.UpdatePersonalAverage("alice", 0)
	if tracker.GetPersonalAverage("alice") != 16.0 {
		t.Errorf("Personal average changed after zero BPM update, got %.2f", tracker.GetPersonalAverage("alice"))
	}
}

func TestBreathingAnomalyTrackerNegativeBPMIgnored(t *testing.T) {
	tracker := NewBreathingAnomalyTracker()
	tracker.UpdatePersonalAverage("alice", 16.0)

	tracker.UpdatePersonalAverage("alice", -5.0)
	if tracker.GetPersonalAverage("alice") != 16.0 {
		t.Errorf("Personal average changed after negative BPM update")
	}
}

func TestBreathingAnomalyTrackerJSONRoundTrip(t *testing.T) {
	tracker := NewBreathingAnomalyTracker()
	tracker.UpdatePersonalAverage("alice", 16.0)
	tracker.UpdatePersonalAverage("bob", 14.5)

	data, err := tracker.SaveToJSON()
	if err != nil {
		t.Fatalf("SaveToJSON() error = %v", err)
	}

	tracker2 := NewBreathingAnomalyTracker()
	if err := tracker2.LoadFromJSON(data); err != nil {
		t.Fatalf("LoadFromJSON() error = %v", err)
	}

	if tracker2.GetPersonalAverage("alice") != tracker.GetPersonalAverage("alice") {
		t.Errorf("alice avg mismatch: %.2f vs %.2f",
			tracker2.GetPersonalAverage("alice"), tracker.GetPersonalAverage("alice"))
	}
	if tracker2.GetPersonalAverage("bob") != tracker.GetPersonalAverage("bob") {
		t.Errorf("bob avg mismatch: %.2f vs %.2f",
			tracker2.GetPersonalAverage("bob"), tracker.GetPersonalAverage("bob"))
	}
}

func TestBreathingAnomalyTrackerJSONEmpty(t *testing.T) {
	tracker := NewBreathingAnomalyTracker()

	data, err := tracker.SaveToJSON()
	if err != nil {
		t.Fatalf("SaveToJSON() error = %v", err)
	}

	// Should be valid empty JSON object
	var m map[string]float64
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}
	if len(m) != 0 {
		t.Errorf("Expected empty map, got %d entries", len(m))
	}
}

func TestBreathingAnomalyTrackerMultiplePeople(t *testing.T) {
	tracker := NewBreathingAnomalyTracker()

	tracker.UpdatePersonalAverage("alice", 16.0)
	tracker.UpdatePersonalAverage("bob", 13.0)

	// Alice: 21 bpm is 1.3125x her average → anomaly
	if !tracker.CheckAnomaly("alice", 21.0) {
		t.Error("Alice 21 bpm should be anomaly (personal avg 16)")
	}

	// Bob: 21 bpm is 1.615x his average → anomaly
	if !tracker.CheckAnomaly("bob", 21.0) {
		t.Error("Bob 21 bpm should be anomaly (personal avg 13)")
	}

	// Alice: 18 bpm is 1.125x → not anomaly
	if tracker.CheckAnomaly("alice", 18.0) {
		t.Error("Alice 18 bpm should NOT be anomaly")
	}

	// Bob: 14 bpm is 1.077x → not anomaly
	if tracker.CheckAnomaly("bob", 14.0) {
		t.Error("Bob 14 bpm should NOT be anomaly")
	}
}

func mathAbs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
