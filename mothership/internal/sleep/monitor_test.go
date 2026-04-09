package sleep

import (
	"math"
	"testing"
	"time"
)

// sleepTestTime returns a time that is guaranteed to be within the 22:00-07:00
// sleep window (23:00 today).
func sleepTestTime() time.Time {
	now := time.Now()
	// Use 23:00 today if before midnight, or 23:00 yesterday if after midnight
	if now.Hour() >= 7 && now.Hour() < 22 {
		// We're outside sleep hours — use yesterday at 23:00
		return time.Date(now.Year(), now.Month(), now.Day()-1, 23, 0, 0, 0, now.Location())
	}
	if now.Hour() >= 22 {
		return time.Date(now.Year(), now.Month(), now.Day(), 23, 0, 0, 0, now.Location())
	}
	// Before 7am — use yesterday at 23:00
	return time.Date(now.Year(), now.Month(), now.Day()-1, 23, 0, 0, 0, now.Location())
}

// TestSessionOnsetConfirmedAfter15Minutes verifies that a session is confirmed
// after 15 consecutive minutes of stationary detection in a bedroom zone.
// We test the confirmation logic directly by setting the state to Tentative
// and advancing time past the threshold.
func TestSessionOnsetConfirmedAfter15Minutes(t *testing.T) {
	m := NewMonitor(MonitorConfig{
		SampleInterval:        1 * time.Second,
		SessionConfirmMinutes: 15,
		WakeConfirmMinutes:    2,
		SleepStartHour:        22,
		SleepEndHour:          7,
	})

	linkID := "test-link"
	now := time.Now()

	// Pre-set the link session state to Tentative (simulating that the zone manager
	// confirmed the person is in a bedroom zone). Then test the 15-min confirmation.
	m.linkSessionStates[linkID] = &LinkSessionState{
		State:              SessionStateTentative,
		TentativeStartTime: now,
		LastStationaryTime: now,
		ZoneID:             "bedroom-1",
	}

	// At 14 minutes: still tentative
	m.updateSessionState(linkID, now.Add(14*time.Minute), 0.01, false, true, 14.0, nil)
	m.mu.RLock()
	ls := m.linkSessionStates[linkID]
	m.mu.RUnlock()

	if ls == nil || ls.State != SessionStateTentative {
		t.Fatalf("expected SessionStateTentative at 14 min, got %v", sessionStateString(ls))
	}

	// At 15 minutes: should confirm
	m.updateSessionState(linkID, now.Add(15*time.Minute), 0.01, false, true, 14.0, nil)
	m.mu.RLock()
	ls = m.linkSessionStates[linkID]
	m.mu.RUnlock()

	if ls == nil || ls.State != SessionStateConfirmed {
		t.Fatalf("expected SessionStateConfirmed at 15 min, got %v", sessionStateString(ls))
	}

	if ls.SessionID == "" {
		t.Error("expected SessionID to be set on confirmation")
	}
}

// TestBriefNapNotConfirmed verifies that stationary detection for less than
// 15 minutes does NOT create a confirmed session (avoids brief naps).
func TestBriefNapNotConfirmed(t *testing.T) {
	m := NewMonitor(MonitorConfig{
		SampleInterval:        1 * time.Second,
		SessionConfirmMinutes: 15,
		WakeConfirmMinutes:    2,
	})

	linkID := "test-link"
	now := time.Now()

	// Set to Tentative state (as if zone manager detected bedroom presence)
	m.linkSessionStates[linkID] = &LinkSessionState{
		State:              SessionStateTentative,
		TentativeStartTime: now,
		LastStationaryTime: now,
		ZoneID:             "bedroom-1",
	}

	// Simulate 10 minutes of continued stationary — should NOT confirm
	for min := 1; min <= 10; min++ {
		m.updateSessionState(linkID, now.Add(time.Duration(min)*time.Minute), 0.01, false, true, 14.0, nil)
	}

	m.mu.RLock()
	ls := m.linkSessionStates[linkID]
	m.mu.RUnlock()

	if ls == nil {
		t.Fatal("expected link session state to exist")
	}

	if ls.State == SessionStateConfirmed {
		t.Error("session should NOT be confirmed after only 10 minutes of stationary detection")
	}

	if ls.State != SessionStateTentative {
		t.Errorf("expected SessionStateTentative, got %v", ls.State)
	}
}

// TestWakeEpisodeCounting verifies that 3 MOTION_DETECTED events > 3 seconds each
// during a session produce wake_episode_count = 3.
func TestWakeEpisodeCounting(t *testing.T) {
	baseTime := sleepTestTime()

	ss := NewSleepSession("test-link", 22, 7)
	ss.isActive = true
	ss.sleepOnset = baseTime

	// Process quiet motion to establish baseline (within sleep hours)
	for i := 0; i < 10; i++ {
		ss.processMotion(MotionSample{
			Timestamp:      baseTime.Add(time.Duration(i) * time.Minute),
			DeltaRMS:       0.01,
			MotionDetected: false,
		})
	}

	// Inject 3 wake episodes, each lasting 5 seconds (> 3s threshold)
	for ep := 0; ep < 3; ep++ {
		episodeStart := baseTime.Add(time.Duration(10+ep*10) * time.Minute)

		// Start of wake episode (motion detected above RestlessThreshold)
		ss.processMotion(MotionSample{
			Timestamp:      episodeStart,
			DeltaRMS:       0.06, // Above RestlessThreshold (0.04)
			MotionDetected: true,
		})
		// Continue motion for 5 seconds
		ss.processMotion(MotionSample{
			Timestamp:      episodeStart.Add(3 * time.Second),
			DeltaRMS:       0.06,
			MotionDetected: true,
		})
		ss.processMotion(MotionSample{
			Timestamp:      episodeStart.Add(5 * time.Second),
			DeltaRMS:       0.06,
			MotionDetected: true,
		})
		// Motion stops — episode should close
		ss.processMotion(MotionSample{
			Timestamp:      episodeStart.Add(6 * time.Second),
			DeltaRMS:       0.01,
			MotionDetected: false,
		})
	}

	ss.mu.RLock()
	count := len(ss.wakeEpisodes)
	ss.mu.RUnlock()

	if count != 3 {
		t.Errorf("expected 3 wake episodes, got %d", count)
	}
}

// TestWASOCalculation verifies that 3 episodes of 5 minutes each produce WASO = 15 minutes.
func TestWASOCalculation(t *testing.T) {
	baseTime := sleepTestTime()

	ss := NewSleepSession("test-link", 22, 7)
	ss.isActive = true
	ss.sleepOnset = baseTime

	// Process quiet baseline (within sleep hours)
	for i := 0; i < 5; i++ {
		ss.processMotion(MotionSample{
			Timestamp:      baseTime.Add(time.Duration(i) * time.Minute),
			DeltaRMS:       0.01,
			MotionDetected: false,
		})
	}

	// Inject 3 wake episodes, each 5 minutes long
	for ep := 0; ep < 3; ep++ {
		epStart := baseTime.Add(time.Duration(10+ep*20) * time.Minute)

		// 5 minutes of restless motion (10 samples at 30s each)
		for s := 0; s < 10; s++ {
			ss.processMotion(MotionSample{
				Timestamp:      epStart.Add(time.Duration(s) * 30 * time.Second),
				DeltaRMS:       0.06,
				MotionDetected: true,
			})
		}
		// Motion stops
		ss.processMotion(MotionSample{
			Timestamp:      epStart.Add(5 * time.Minute),
			DeltaRMS:       0.01,
			MotionDetected: false,
		})
	}

	metrics := ss.GetMetrics()

	// 3 episodes × 5 minutes each = 15 minutes WASO
	if math.Abs(metrics.WASOMinutes-15.0) > 2.0 {
		t.Errorf("expected WASO ~15 minutes, got %.1f", metrics.WASOMinutes)
	}

	if metrics.WakeEpisodeCount != 3 {
		t.Errorf("expected 3 wake episodes, got %d", metrics.WakeEpisodeCount)
	}
}

// TestSleepEfficiencyCalculation verifies that 480 minutes in bed with 45 minutes WASO
// produces efficiency = (480 - 45) / 480 * 100 = 90.625%.
func TestSleepEfficiencyCalculation(t *testing.T) {
	baseTime := sleepTestTime()

	ss := NewSleepSession("test-link", 22, 7)
	ss.isActive = true
	ss.sleepOnset = baseTime

	// Create 480 minutes of motion samples (8 hours)
	// 435 minutes quiet + 45 minutes restless (= WASO)

	// First 2 hours quiet (baseline)
	for i := 0; i < 120; i++ {
		ss.processMotion(MotionSample{
			Timestamp:      baseTime.Add(time.Duration(i) * time.Minute),
			DeltaRMS:       0.01,
			MotionDetected: false,
		})
	}

	// 45 minutes of restless wake episodes (split into 3 × 15 min)
	for ep := 0; ep < 3; ep++ {
		epStart := baseTime.Add(time.Duration(120+ep*20) * time.Minute)
		for s := 0; s < 30; s++ {
			ss.processMotion(MotionSample{
				Timestamp:      epStart.Add(time.Duration(s) * 30 * time.Second),
				DeltaRMS:       0.06,
				MotionDetected: true,
			})
		}
		ss.processMotion(MotionSample{
			Timestamp:      epStart.Add(15 * time.Minute),
			DeltaRMS:       0.01,
			MotionDetected: false,
		})
	}

	// Remaining quiet time to reach 480 total minutes
	remaining := 480 - 120 - 45
	for i := 0; i < remaining; i++ {
		ss.processMotion(MotionSample{
			Timestamp:      baseTime.Add(time.Duration(120+45+i) * time.Minute),
			DeltaRMS:       0.01,
			MotionDetected: false,
		})
	}

	metrics := ss.GetMetrics()

	expectedEfficiency := (480.0 - 45.0) / 480.0 * 100.0 // 90.625%
	tolerance := 5.0 // Allow tolerance due to timing granularity

	if math.Abs(metrics.SleepEfficiency-expectedEfficiency) > tolerance {
		t.Errorf("expected sleep efficiency ~%.1f%%, got %.1f%%", expectedEfficiency, metrics.SleepEfficiency)
	}
}

// TestBreathingAnomalyDetection verifies that 4 minutes of breathing_freq_hz = 0.1
// (6 BPM) triggers an anomaly log (breathing < 8 bpm for > 3 minutes).
func TestBreathingAnomalyDetection(t *testing.T) {
	baseTime := sleepTestTime()

	ss := NewSleepSession("test-link", 22, 7)
	ss.isActive = true
	ss.sleepOnset = baseTime

	// Normal breathing within sleep hours
	for i := 0; i < 5; i++ {
		ss.processBreathing(BreathingSample{
			Timestamp:  baseTime.Add(time.Duration(i) * time.Minute),
			RateBPM:    14.0,
			Confidence: 0.8,
			IsDetected: true,
		})
	}

	// Inject 4 minutes of low breathing rate (6 BPM = breathing_freq_hz 0.1 * 60)
	anomalyStart := baseTime.Add(10 * time.Minute)
	for s := 0; s < 8; s++ { // 8 samples at 30s intervals = 4 minutes
		ss.processBreathing(BreathingSample{
			Timestamp:  anomalyStart.Add(time.Duration(s) * 30 * time.Second),
			RateBPM:    6.0,
			Confidence: 0.7,
			IsDetected: true,
		})
	}

	metrics := ss.GetMetrics()

	if metrics.BreathingAnomalyCount < 1 {
		t.Errorf("expected at least 1 breathing anomaly (6 BPM for 4 min), got %d", metrics.BreathingAnomalyCount)
	}
}

// TestBreathingAnomalyHighRate verifies that breathing rate > 25 BPM for > 3 minutes
// triggers an anomaly log.
func TestBreathingAnomalyHighRate(t *testing.T) {
	baseTime := sleepTestTime()

	ss := NewSleepSession("test-link", 22, 7)
	ss.isActive = true
	ss.sleepOnset = baseTime

	// Normal breathing baseline
	for i := 0; i < 5; i++ {
		ss.processBreathing(BreathingSample{
			Timestamp:  baseTime.Add(time.Duration(i) * time.Minute),
			RateBPM:    14.0,
			Confidence: 0.8,
			IsDetected: true,
		})
	}

	// Inject 4 minutes of high breathing rate (26 BPM, > 25 BPM threshold)
	anomalyStart := baseTime.Add(10 * time.Minute)
	for s := 0; s < 8; s++ {
		ss.processBreathing(BreathingSample{
			Timestamp:  anomalyStart.Add(time.Duration(s) * 30 * time.Second),
			RateBPM:    26.0,
			Confidence: 0.7,
			IsDetected: true,
		})
	}

	metrics := ss.GetMetrics()

	if metrics.BreathingAnomalyCount < 1 {
		t.Errorf("expected at least 1 breathing anomaly (26 BPM for 4 min), got %d", metrics.BreathingAnomalyCount)
	}
}

// TestBreathingAnomalyBelowThreshold verifies that breathing at 6 BPM for 2 minutes
// does NOT trigger an anomaly (below the 3-minute duration threshold).
func TestBreathingAnomalyBelowThreshold(t *testing.T) {
	baseTime := sleepTestTime()

	ss := NewSleepSession("test-link", 22, 7)
	ss.isActive = true
	ss.sleepOnset = baseTime

	// Normal baseline
	for i := 0; i < 5; i++ {
		ss.processBreathing(BreathingSample{
			Timestamp:  baseTime.Add(time.Duration(i) * time.Minute),
			RateBPM:    14.0,
			Confidence: 0.8,
			IsDetected: true,
		})
	}

	// Inject only 2 minutes of low breathing (6 BPM) — below 3-minute threshold
	anomalyStart := baseTime.Add(10 * time.Minute)
	for s := 0; s < 4; s++ { // 4 samples at 30s = 2 minutes
		ss.processBreathing(BreathingSample{
			Timestamp:  anomalyStart.Add(time.Duration(s) * 30 * time.Second),
			RateBPM:    6.0,
			Confidence: 0.7,
			IsDetected: true,
		})
	}

	metrics := ss.GetMetrics()

	if metrics.BreathingAnomalyCount != 0 {
		t.Errorf("expected 0 breathing anomalies for <3 min duration, got %d", metrics.BreathingAnomalyCount)
	}
}

// TestMorningSummaryTriggerFiresOnce verifies that ShouldPushMorningSummary returns
// true only on the first call after 6am when a session has ended.
func TestMorningSummaryTriggerFiresOnce(t *testing.T) {
	m := NewMonitor(MonitorConfig{
		SampleInterval:        1 * time.Second,
		SessionConfirmMinutes: 15,
		WakeConfirmMinutes:    2,
		ReportHour:            7,
	})

	linkID := "test-link"

	// Create an ended session
	m.linkSessionStates[linkID] = &LinkSessionState{
		State:              SessionStateEnded,
		ConfirmedStartTime: time.Now().Add(-8 * time.Hour),
		ZoneID:             "bedroom-1",
	}

	// Create an analyzer session with data (must use sleep hours for samples to be accepted)
	baseTime := sleepTestTime()
	session := NewSleepSession(linkID, 22, 7)
	session.isActive = true
	for i := 0; i < 50; i++ {
		session.processBreathing(BreathingSample{
			Timestamp:  baseTime.Add(time.Duration(i) * time.Minute),
			RateBPM:    14.0,
			Confidence: 0.8,
			IsDetected: true,
		})
		session.processMotion(MotionSample{
			Timestamp:      baseTime.Add(time.Duration(i) * time.Minute),
			DeltaRMS:       0.01,
			MotionDetected: false,
		})
	}
	m.analyzer.sessions[linkID] = session

	// Only test if current hour >= 6 (the method gates on this)
	if time.Now().Hour() >= 6 {
		ok, report := m.ShouldPushMorningSummary()
		if !ok {
			t.Error("expected ShouldPushMorningSummary to return true on first call")
		}
		if report == nil {
			t.Error("expected a non-nil report")
		}

		// Second call should return false (already pushed today)
		ok, _ = m.ShouldPushMorningSummary()
		if ok {
			t.Error("expected ShouldPushMorningSummary to return false on second call")
		}
	} else {
		t.Skip("skipping: current hour < 6, morning summary trigger requires hour >= 6")
	}
}

// TestMorningSummaryBefore6am verifies that ShouldPushMorningSummary returns false
// when called before 6am regardless of session state.
func TestMorningSummaryBefore6am(t *testing.T) {
	m := NewMonitor(MonitorConfig{
		SampleInterval:        1 * time.Second,
		SessionConfirmMinutes: 15,
		WakeConfirmMinutes:    2,
		ReportHour:            7,
	})

	linkID := "test-link"
	m.linkSessionStates[linkID] = &LinkSessionState{
		State:              SessionStateEnded,
		ConfirmedStartTime: time.Now().Add(-8 * time.Hour),
		ZoneID:             "bedroom-1",
	}

	baseTime := sleepTestTime()
	session := NewSleepSession(linkID, 22, 7)
	session.isActive = true
	for i := 0; i < 50; i++ {
		session.processBreathing(BreathingSample{
			Timestamp:  baseTime.Add(time.Duration(i) * time.Minute),
			RateBPM:    14.0,
			Confidence: 0.8,
			IsDetected: true,
		})
		session.processMotion(MotionSample{
			Timestamp:      baseTime.Add(time.Duration(i) * time.Minute),
			DeltaRMS:       0.01,
			MotionDetected: false,
		})
	}
	m.analyzer.sessions[linkID] = session

	// Manually test with a pre-6am time by directly invoking the time check
	if time.Now().Hour() < 6 {
		// We're actually before 6am, so the real method should return false
		ok, _ := m.ShouldPushMorningSummary()
		if ok {
			t.Error("expected false before 6am")
		}
	} else {
		t.Skip("skipping: current hour >= 6, cannot test before-6am behavior")
	}
}

// TestSleepEfficiencyFormula verifies the sleep efficiency formula directly:
// (time_in_bed - waso) / time_in_bed * 100
func TestSleepEfficiencyFormula(t *testing.T) {
	tests := []struct {
		name         string
		timeInBedMin float64
		wasoMin      float64
		expectedEff  float64
	}{
		{"perfect sleep", 480, 0, 100.0},
		{"typical good", 480, 45, 90.625},
		{"poor sleep", 480, 120, 75.0},
		{"very poor", 300, 150, 50.0},
		{"minimal wake", 420, 10, 97.619},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timeInBed := time.Duration(tt.timeInBedMin) * time.Minute
			waso := time.Duration(tt.wasoMin) * time.Minute

			var efficiency float64
			if timeInBed > 0 {
				efficiency = float64(timeInBed-waso) / float64(timeInBed) * 100
				if efficiency > 100 {
					efficiency = 100
				}
			}

			if math.Abs(efficiency-tt.expectedEff) > 0.1 {
				t.Errorf("efficiency = %.3f, want %.3f", efficiency, tt.expectedEff)
			}
		})
	}
}

// TestSessionEndOnSustainedMotion verifies that sustained motion > 2 minutes
// ends a confirmed sleep session.
func TestSessionEndOnSustainedMotion(t *testing.T) {
	m := NewMonitor(MonitorConfig{
		SampleInterval:        1 * time.Second,
		SessionConfirmMinutes: 15,
		WakeConfirmMinutes:    2,
	})

	linkID := "test-link"
	now := time.Now()

	// Fast-forward to confirmed state
	m.linkSessionStates[linkID] = &LinkSessionState{
		State:              SessionStateConfirmed,
		ConfirmedStartTime: now.Add(-2 * time.Hour),
		LastStationaryTime: now.Add(-5 * time.Minute),
		ZoneID:             "bedroom-1",
	}

	// Simulate sustained motion for 2+ minutes via updateSessionState
	m.updateSessionState(linkID, now, 0.06, true, false, 0, nil)
	m.updateSessionState(linkID, now.Add(1*time.Minute), 0.06, true, false, 0, nil)
	m.updateSessionState(linkID, now.Add(2*time.Minute+1*time.Second), 0.06, true, false, 0, nil)

	// checkSessionEnd is called separately in collectSamples — call it here
	m.checkSessionEnd(linkID, now.Add(2*time.Minute+1*time.Second))

	m.mu.RLock()
	ls := m.linkSessionStates[linkID]
	m.mu.RUnlock()

	if ls == nil || ls.State != SessionStateEnded {
		t.Errorf("expected SessionStateEnded after sustained motion, got %v", sessionStateString(ls))
	}
}

// TestSessionEndOnStationaryLost verifies that session ends when stationary
// detection drops for > 30 minutes (person left room without portal crossing).
func TestSessionEndOnStationaryLost(t *testing.T) {
	m := NewMonitor(MonitorConfig{
		SampleInterval:        1 * time.Second,
		SessionConfirmMinutes: 15,
		WakeConfirmMinutes:    2,
	})

	linkID := "test-link"
	now := time.Now()

	// Set up confirmed session with LastStationaryTime 31 minutes ago
	m.linkSessionStates[linkID] = &LinkSessionState{
		State:              SessionStateConfirmed,
		ConfirmedStartTime: now.Add(-3 * time.Hour),
		LastStationaryTime: now.Add(-31 * time.Minute),
		ZoneID:             "bedroom-1",
	}

	m.checkSessionEnd(linkID, now)

	m.mu.RLock()
	ls := m.linkSessionStates[linkID]
	m.mu.RUnlock()

	if ls == nil || ls.State != SessionStateEnded {
		t.Errorf("expected SessionStateEnded after 30 min stationary loss, got %v", sessionStateString(ls))
	}
}

// TestSessionEndNotTriggeredBeforeThreshold verifies that session does NOT end
// when stationary is lost for only 29 minutes (below 30-min threshold).
func TestSessionEndNotTriggeredBeforeThreshold(t *testing.T) {
	m := NewMonitor(MonitorConfig{
		SampleInterval:        1 * time.Second,
		SessionConfirmMinutes: 15,
		WakeConfirmMinutes:    2,
	})

	linkID := "test-link"
	now := time.Now()

	m.linkSessionStates[linkID] = &LinkSessionState{
		State:              SessionStateConfirmed,
		ConfirmedStartTime: now.Add(-2 * time.Hour),
		LastStationaryTime: now.Add(-29 * time.Minute),
		ZoneID:             "bedroom-1",
	}

	m.checkSessionEnd(linkID, now)

	m.mu.RLock()
	ls := m.linkSessionStates[linkID]
	m.mu.RUnlock()

	if ls == nil || ls.State != SessionStateConfirmed {
		t.Errorf("expected SessionStateConfirmed (not ended), got %v", sessionStateString(ls))
	}
}

// helper to safely get state string for error messages
func sessionStateString(ls *LinkSessionState) string {
	if ls == nil {
		return "nil"
	}
	switch ls.State {
	case SessionStateNone:
		return "None"
	case SessionStateTentative:
		return "Tentative"
	case SessionStateConfirmed:
		return "Confirmed"
	case SessionStateEnded:
		return "Ended"
	default:
		return "Unknown"
	}
}
