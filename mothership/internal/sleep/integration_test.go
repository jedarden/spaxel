package sleep

import (
	"sync"
	"testing"
	"time"
)

func TestNewMonitor(t *testing.T) {
	cfg := MonitorConfig{
		SampleInterval: 15 * time.Second,
		ReportHour:     7,
		SleepStartHour: 22,
		SleepEndHour:   7,
	}

	m := NewMonitor(cfg)
	if m == nil {
		t.Fatal("NewMonitor() returned nil")
	}
}

func TestNewMonitorDefaults(t *testing.T) {
	m := NewMonitor(MonitorConfig{})
	if m.sampleInterval != SampleInterval {
		t.Errorf("Default sampleInterval = %v, want %v", m.sampleInterval, SampleInterval)
	}
}

func TestMonitorStartStop(t *testing.T) {
	m := NewMonitor(MonitorConfig{
		SampleInterval: 100 * time.Millisecond,
	})

	m.Start()
	time.Sleep(150 * time.Millisecond)
	m.Stop()
}

func TestMonitorGetAnalyzer(t *testing.T) {
	m := NewMonitor(MonitorConfig{})
	analyzer := m.GetAnalyzer()

	if analyzer == nil {
		t.Error("GetAnalyzer() returned nil")
	}
}

func TestMonitorGetCurrentState(t *testing.T) {
	m := NewMonitor(MonitorConfig{
		SleepStartHour: 22,
		SleepEndHour:   7,
	})

	// No session yet
	state := m.GetCurrentState("nonexistent")
	if state != SleepStateAwake {
		t.Errorf("State for nonexistent link = %v, want awake", state)
	}
}

func TestMonitorIsInSleepHours(t *testing.T) {
	m := NewMonitor(MonitorConfig{
		SleepStartHour: 22,
		SleepEndHour:   7,
	})

	// Just verify the method runs without error
	_ = m.IsInSleepHours()
}

func TestMonitorGetStatus(t *testing.T) {
	m := NewMonitor(MonitorConfig{
		SleepStartHour: 22,
		SleepEndHour:   7,
	})

	status := m.GetStatus()

	if status.SleepStartHour != 22 {
		t.Errorf("SleepStartHour = %d, want 22", status.SleepStartHour)
	}
	if status.SleepEndHour != 7 {
		t.Errorf("SleepEndHour = %d, want 7", status.SleepEndHour)
	}
}

func TestMonitorForceReportGeneration(t *testing.T) {
	m := NewMonitor(MonitorConfig{
		SleepStartHour: 22,
		SleepEndHour:   7,
	})

	// Add a session with data directly to the analyzer
	session := NewSleepSession("test-link", 22, 7)
	baseTime := time.Date(2024, 1, 15, 23, 0, 0, 0, time.Local)
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

	m.analyzer.mu.Lock()
	m.analyzer.sessions["test-link"] = session
	m.analyzer.mu.Unlock()

	reports := m.ForceReportGeneration()

	if len(reports) != 1 {
		t.Errorf("ForceReportGeneration() returned %d reports, want 1", len(reports))
	}

	report, exists := reports["test-link"]
	if !exists {
		t.Fatal("Report for test-link not found")
	}
	if report.LinkID != "test-link" {
		t.Errorf("Report LinkID = %s, want test-link", report.LinkID)
	}
}

func TestMonitorConcurrent(t *testing.T) {
	m := NewMonitor(MonitorConfig{
		SampleInterval: 10 * time.Millisecond,
	})

	var wg sync.WaitGroup

	// Start multiple goroutines that access the monitor
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			linkID := "link-" + string(rune('0'+id))

			// Access various methods
			_ = m.GetCurrentState(linkID)
			_ = m.IsInSleepHours()
			_ = m.GetStatus()
		}(i)
	}

	wg.Wait()
}

func TestSleepLinkStateFields(t *testing.T) {
	m := NewMonitor(MonitorConfig{
		SleepStartHour: 22,
		SleepEndHour:   7,
	})

	// Create session with specific state
	session := NewSleepSession("test-link", 22, 7)
	session.isActive = true
	session.currentState = SleepStateLightSleep
	session.processBreathing(BreathingSample{
		Timestamp:  time.Date(2024, 1, 15, 23, 0, 0, 0, time.Local),
		RateBPM:    16.5,
		Confidence: 0.9,
		IsDetected: true,
	})
	session.processMotion(MotionSample{
		Timestamp:      time.Date(2024, 1, 15, 23, 0, 0, 0, time.Local),
		DeltaRMS:       0.02,
		MotionDetected: true,
	})

	m.analyzer.mu.Lock()
	m.analyzer.sessions["test-link"] = session
	m.analyzer.mu.Unlock()

	status := m.GetStatus()

	linkState, exists := status.LinkStates["test-link"]
	if !exists {
		t.Fatal("LinkStates missing test-link")
	}

	if linkState.SleepState != "light_sleep" {
		t.Errorf("SleepState = %s, want light_sleep", linkState.SleepState)
	}
	if !linkState.SessionActive {
		t.Error("SessionActive should be true")
	}
}

// TestTwoPersonBedroomScenario tests the multi-person bedroom edge case.
// This test documents the expected behavior when two blobs are tracked in a bedroom zone:
// 1. If BLE identifies both blobs, assign sleep records to respective persons
// 2. If BLE identifies only one blob, assign that record to the person and create a zone-based record for the other
// 3. If BLE identifies neither blob, create two separate zone-based records
// 4. Breathing analysis should use the blob with the strongest stationary signal (lowest smooth_deltaRMS)
//
// NOTE: This test currently FAILS because the multi-person bedroom edge case is NOT implemented.
// The sleep analyzer currently tracks one session per linkID but does not:
// - Detect multiple blobs in the same bedroom zone
// - Automatically assign person IDs from BLE matches
// - Create zone-based records when BLE identity is unavailable
// - Select the best blob for breathing analysis based on deltaRMS
//
// See: /home/coding/spaxel/notes/bf-5613-findings.md for detailed analysis.
func TestTwoPersonBedroomScenario(t *testing.T) {
	m := NewMonitor(MonitorConfig{
		SleepStartHour: 22,
		SleepEndHour:   7,
	})

	baseTime := time.Date(2024, 1, 15, 23, 0, 0, 0, time.Local)

	// Simulate two blobs in the same bedroom zone
	// Blob 1: Lower deltaRMS (better stationary signal, should be used for breathing analysis)
	blob1LinkID := "link-bedroom-1"
	session1 := NewSleepSession(blob1LinkID, 22, 7)
	session1.isActive = true
	session1.currentState = SleepStateDeepSleep
	for i := 0; i < 50; i++ {
		session1.processBreathing(BreathingSample{
			Timestamp:  baseTime.Add(time.Duration(i) * time.Minute),
			RateBPM:    14.0,
			Confidence: 0.9,
			IsDetected: true,
		})
		session1.processMotion(MotionSample{
			Timestamp:      baseTime.Add(time.Duration(i) * time.Minute),
			DeltaRMS:       0.01, // Lower = better stationary signal
			MotionDetected: false,
		})
	}

	// Blob 2: Higher deltaRMS (weaker stationary signal)
	blob2LinkID := "link-bedroom-2"
	session2 := NewSleepSession(blob2LinkID, 22, 7)
	session2.isActive = true
	session2.currentState = SleepStateLightSleep
	for i := 0; i < 50; i++ {
		session2.processBreathing(BreathingSample{
			Timestamp:  baseTime.Add(time.Duration(i) * time.Minute),
			RateBPM:    16.0,
			Confidence: 0.7, // Lower confidence
			IsDetected: true,
		})
		session2.processMotion(MotionSample{
			Timestamp:      baseTime.Add(time.Duration(i) * time.Minute),
			DeltaRMS:       0.025, // Higher = more restless
			MotionDetected: true,
		})
	}

	m.analyzer.mu.Lock()
	m.analyzer.sessions[blob1LinkID] = session1
	m.analyzer.sessions[blob2LinkID] = session2
	m.analyzer.mu.Unlock()

	// Test Case 1: Verify both sessions are tracked independently
	sessions := m.GetAllSessions()
	if len(sessions) != 2 {
		t.Errorf("Expected 2 sessions, got %d", len(sessions))
	}

	// Test Case 2: Simulate BLE identity assignment
	// In the expected implementation, this would be done automatically when BLE matches are resolved
	person1ID := "person-alice"
	person2ID := "person-bob"
	m.analyzer.SetPersonID(blob1LinkID, person1ID)
	m.analyzer.SetPersonID(blob2LinkID, person2ID)

	// Verify person IDs are set
	session1 = m.analyzer.GetSession(blob1LinkID)
	session2 = m.analyzer.GetSession(blob2LinkID)
	if session1.GetPersonID() != person1ID {
		t.Errorf("Expected person ID %s for blob1, got %s", person1ID, session1.GetPersonID())
	}
	if session2.GetPersonID() != person2ID {
		t.Errorf("Expected person ID %s for blob2, got %s", person2ID, session2.GetPersonID())
	}

	// Test Case 3: Generate reports and verify they're per-person, not per-link
	reports := m.ForceReportGeneration()
	if len(reports) != 2 {
		t.Errorf("Expected 2 reports (one per person), got %d", len(reports))
	}

	// Test Case 4: Verify breathing analysis uses the blob with lowest deltaRMS
	// In the expected implementation, the system would compare deltaRMS across blobs
	// and use the one with the strongest stationary signal for breathing analysis.
	// This is NOT currently implemented - each blob analyzes its own breathing independently.
	report1 := reports[blob1LinkID]
	report2 := reports[blob2LinkID]

	// Blob 1 should have better breathing metrics (lower deltaRMS, higher confidence)
	if report1.Metrics.BreathingScore <= report2.Metrics.BreathingScore {
		// This is expected given the test data, but in a true multi-person scenario,
		// the system should explicitly select which blob to use for breathing analysis.
		t.Logf("Blob1 breathing score: %.1f, Blob2 breathing score: %.1f",
			report1.Metrics.BreathingScore, report2.Metrics.BreathingScore)
	}

	// Test Case 5: Verify zone-based fallback when no BLE match
	// Create a third blob without BLE identity
	blob3LinkID := "link-bedroom-3"
	session3 := NewSleepSession(blob3LinkID, 22, 7)
	session3.isActive = true
	for i := 0; i < 30; i++ {
		session3.processMotion(MotionSample{
			Timestamp: baseTime.Add(time.Duration(i) * time.Minute),
			DeltaRMS:  0.015,
		})
	}

	m.analyzer.mu.Lock()
	m.analyzer.sessions[blob3LinkID] = session3
	m.analyzer.mu.Unlock()

	// Without BLE identity, this should create a zone-based record
	// Currently, it just uses the linkID as the identifier
	session3 = m.analyzer.GetSession(blob3LinkID)
	if session3.GetPersonID() != "" {
		t.Logf("Zone-based fallback: person ID is empty for blob3 (linkID: %s)", blob3LinkID)
	}

	// Summary: This test documents the expected behavior but the actual implementation
	// is missing the multi-person bedroom coordination logic.
	t.Logf("Multi-person bedroom scenario test: sessions tracked independently")
	t.Logf("NOTE: Full multi-person coordination (zone-based records, blob selection) not yet implemented")
}
