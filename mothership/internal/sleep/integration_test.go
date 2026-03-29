package sleep

import (
	"sync"
	"testing"
	"time"
)

func TestNewMonitor(t *testing.T) {
	cfg := MonitorConfig{
		SampleInterval:   15 * time.Second,
		ReportHour:       7,
		SleepStartHour:   22,
		SleepEndHour:     7,
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
