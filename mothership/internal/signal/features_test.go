package signal

import (
	"math"
	"testing"
	"time"
)

func TestNBVITracker_InitialSelection(t *testing.T) {
	tracker := NewNBVITracker(64)

	// Before enough samples, should return all data subcarriers
	indices := tracker.GetSelectedIndices()
	expectedCount := 46 // data subcarriers (64 - 3 null - 11 guard - 4 pilot)

	if len(indices) != expectedCount {
		t.Errorf("Expected %d initial indices, got %d", expectedCount, len(indices))
	}
}

func TestNBVITracker_UpdateAndSelection(t *testing.T) {
	tracker := NewNBVITracker(64)

	// Add samples with varying variance
	for i := 0; i < 100; i++ {
		amplitude := make([]float64, 64)
		for k := 0; k < 64; k++ {
			if IsDataSubcarrier(k) {
				// Create varying variance - some subcarriers more variable than others
				if k%2 == 0 {
					amplitude[k] = 100.0 + float64(i%20) // Higher variance
				} else {
					amplitude[k] = 100.0 // Low variance
				}
			}
		}
		tracker.Update(amplitude)
	}

	// After enough samples, should have selected subcarriers
	indices := tracker.GetSelectedIndices()

	if len(indices) == 0 {
		t.Error("Should have selected subcarriers")
	}

	// Even-indexed subcarriers should be preferred (higher variance)
	evenCount := 0
	for _, idx := range indices {
		if idx%2 == 0 {
			evenCount++
		}
	}

	if evenCount < len(indices)/2 {
		t.Errorf("Expected more even-indexed subcarriers selected, got %d/%d", evenCount, len(indices))
	}
}

func TestNBVITracker_SampleCount(t *testing.T) {
	tracker := NewNBVITracker(64)

	for i := 0; i < 50; i++ {
		amplitude := make([]float64, 64)
		for k := 0; k < 64; k++ {
			amplitude[k] = 100.0
		}
		tracker.Update(amplitude)
	}

	if tracker.SampleCount() != 50 {
		t.Errorf("Expected 50 samples, got %d", tracker.SampleCount())
	}
}

func TestMotionDetector_InitialNoMotion(t *testing.T) {
	md := NewMotionDetector(64)

	if md.IsMotionDetected() {
		t.Error("Should not detect motion initially")
	}
}

func TestMotionDetector_Process_NoMotion(t *testing.T) {
	md := NewMotionDetector(64)

	// Create baseline at the same amplitude we'll use for the test frame
	nSub := 64
	payload := make([]int8, nSub*2)
	for k := 0; k < nSub; k++ {
		// amplitude ~100 -> I,Q ~70 each (sqrt(100^2/2))
		val := int8(70)
		payload[k*2] = val
		payload[k*2+1] = val
	}

	processed, err := PhaseSanitize(payload, 0, nSub)
	if err != nil {
		t.Fatalf("PhaseSanitize failed: %v", err)
	}

	// Use the processed amplitude as the baseline
	baseline := processed.Amplitude

	// Process same frame again (no motion)
	features := md.Process(processed, baseline)

	// DeltaRMS should be very small since baseline == current amplitude
	if features.DeltaRMS > 0.01 {
		t.Errorf("Expected small DeltaRMS, got %.4f", features.DeltaRMS)
	}

	// After smoothing, motion should not be detected
	// (smoothDeltaRMS = 0.3 * deltaRMS + 0.7 * 0 initially)
	if features.SmoothDeltaRMS > DefaultDeltaRMSThreshold {
		t.Errorf("Expected smoothDeltaRMS < %.2f, got %.4f", DefaultDeltaRMSThreshold, features.SmoothDeltaRMS)
	}
}

func TestMotionDetector_Process_WithMotion(t *testing.T) {
	md := NewMotionDetector(64)

	// Create baseline
	baseline := make([]float64, 64)
	for k := 0; k < 64; k++ {
		baseline[k] = 10.0
	}

	// Process multiple frames to train NBVI and smooth deltaRMS
	for i := 0; i < 10; i++ {
		// Create frame very different from baseline (motion)
		nSub := 64
		payload := make([]int8, nSub*2)
		for k := 0; k < nSub; k++ {
			// Large amplitude difference
			payload[k*2] = 100
			payload[k*2+1] = 100
		}

		processed, err := PhaseSanitize(payload, 0, nSub)
		if err != nil {
			t.Fatalf("PhaseSanitize failed: %v", err)
		}

		md.Process(processed, baseline)
	}

	if !md.IsMotionDetected() {
		t.Error("Should detect motion for different frame")
	}
}

func TestMotionDetector_Reset(t *testing.T) {
	md := NewMotionDetector(64)

	// Build up some state
	baseline := make([]float64, 64)
	for i := 0; i < 50; i++ {
		payload := make([]int8, 128)
		processed, _ := PhaseSanitize(payload, -40, 64)
		md.Process(processed, baseline)
	}

	md.Reset()

	if md.IsMotionDetected() {
		t.Error("Should not detect motion after reset")
	}

	if md.GetSmoothDeltaRMS() != 0 {
		t.Errorf("Expected 0 smoothDeltaRMS after reset, got %.4f", md.GetSmoothDeltaRMS())
	}
}

func TestMotionDetector_DeltaRMSSmoothing(t *testing.T) {
	md := NewMotionDetector(64)

	baseline := make([]float64, 64)
	for k := 0; k < 64; k++ {
		baseline[k] = 100.0 // Match the frame amplitude
	}

	// First frame - large delta (different from baseline)
	payload1 := make([]int8, 128)
	payload1[0] = 100
	payload1[1] = 100
	processed1, _ := PhaseSanitize(payload1, 0, 64)
	features1 := md.Process(processed1, baseline)

	// Second frame - similar to first
	payload2 := make([]int8, 128)
	payload2[0] = 100
	payload2[1] = 100
	processed2, _ := PhaseSanitize(payload2, 0, 64)
	features2 := md.Process(processed2, baseline)

	// Smooth deltaRMS should be affected by exponential smoothing
	// smooth = alpha * raw + (1-alpha) * prev_smooth
	// First: smooth1 = 0.3 * deltaRMS1 + 0.7 * 0 = 0.3 * deltaRMS1
	// Second: smooth2 = 0.3 * deltaRMS2 + 0.7 * smooth1
	if features1.SmoothDeltaRMS <= 0 {
		t.Error("First SmoothDeltaRMS should be positive")
	}

	if features2.SmoothDeltaRMS <= 0 {
		t.Error("Second SmoothDeltaRMS should be positive")
	}

	// Second smooth should be larger than first (accumulating)
	if features2.SmoothDeltaRMS < features1.SmoothDeltaRMS*0.9 {
		t.Errorf("SmoothDeltaRMS should accumulate: first=%.4f, second=%.4f",
			features1.SmoothDeltaRMS, features2.SmoothDeltaRMS)
	}
}

func TestMotionFeatures_SelectedCount(t *testing.T) {
	md := NewMotionDetector(64)

	baseline := make([]float64, 64)
	for k := 0; k < 64; k++ {
		baseline[k] = 100.0
	}

	// Before NBVI training, should use all data subcarriers
	payload := make([]int8, 128)
	processed, _ := PhaseSanitize(payload, -40, 64)
	features := md.Process(processed, baseline)

	// Initially should be 46 data subcarriers (64 - 3 null - 11 guard - 4 pilot)
	if features.SelectedCount != 46 {
		t.Errorf("Expected 46 selected subcarriers initially, got %d", features.SelectedCount)
	}
}

func TestLinkProcessor_Process(t *testing.T) {
	lp := NewLinkProcessor("test-link", 64, 0.0033)

	// First frame initializes baseline
	payload := make([]int8, 128)
	for k := 0; k < 64; k++ {
		payload[k*2] = 50
		payload[k*2+1] = 50
	}

	result, err := lp.Process(payload, -40, 64, testTime())
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if result.Processed == nil {
		t.Error("Expected processed CSI")
	}

	if result.Features == nil {
		t.Error("Expected motion features")
	}

	// Baseline should be initialized
	if !lp.GetBaseline().IsInitialized() {
		t.Error("Baseline should be initialized")
	}
}

func TestLinkProcessor_MotionGatedBaseline(t *testing.T) {
	lp := NewLinkProcessor("test-link", 64, 0.0033)

	// Initialize with low values
	payload1 := make([]int8, 128)
	result, _ := lp.Process(payload1, -40, 64, testTime())

	// Process with high motion (should not update baseline)
	payload2 := make([]int8, 128)
	for k := 0; k < 64; k++ {
		payload2[k*2] = 100
		payload2[k*2+1] = 100
	}

	// Process many frames to build up motion
	for i := 0; i < 20; i++ {
		result, _ = lp.Process(payload2, -40, 64, testTime())
	}

	if result.BaselineUpdated {
		t.Error("Baseline should not update during motion")
	}
}

func TestProcessorManager_GetOrCreate(t *testing.T) {
	pm := NewProcessorManager(ProcessorManagerConfig{
		NSub:       64,
		FusionRate: 10.0,
		Tau:        30.0,
	})

	p1 := pm.GetOrCreateProcessor("link1")
	p2 := pm.GetOrCreateProcessor("link1")

	if p1 != p2 {
		t.Error("Should return same processor for same linkID")
	}

	if pm.LinkCount() != 1 {
		t.Errorf("Expected 1 link, got %d", pm.LinkCount())
	}
}

func TestProcessorManager_Process(t *testing.T) {
	pm := NewProcessorManager(ProcessorManagerConfig{
		NSub:       64,
		FusionRate: 10.0,
		Tau:        30.0,
	})

	payload := make([]int8, 128)
	result, err := pm.Process("link1", payload, -40, 64, testTime())

	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if result.LinkID != "link1" {
		t.Errorf("Expected linkID 'link1', got '%s'", result.LinkID)
	}
}

func TestProcessorManager_GetAllMotionStates(t *testing.T) {
	pm := NewProcessorManager(ProcessorManagerConfig{
		NSub:       64,
		FusionRate: 10.0,
		Tau:        30.0,
	})

	// Process a few frames for two links
	payload := make([]int8, 128)
	for i := 0; i < 5; i++ {
		pm.Process("link1", payload, -40, 64, testTime())
		pm.Process("link2", payload, -40, 64, testTime())
	}

	states := pm.GetAllMotionStates()

	if len(states) != 2 {
		t.Errorf("Expected 2 states, got %d", len(states))
	}

	for _, state := range states {
		if state.LinkID != "link1" && state.LinkID != "link2" {
			t.Errorf("Unexpected linkID: %s", state.LinkID)
		}
	}
}

func TestProcessorManager_ActiveLinks(t *testing.T) {
	pm := NewProcessorManager(ProcessorManagerConfig{
		NSub:       64,
		FusionRate: 10.0,
		Tau:        30.0,
	})

	// Initially no active links
	if pm.ActiveLinks() != 0 {
		t.Errorf("Expected 0 active links, got %d", pm.ActiveLinks())
	}

	// Create a link with motion
	payload := make([]int8, 128)
	for k := 0; k < 64; k++ {
		payload[k*2] = 100
		payload[k*2+1] = 100
	}

	// Initialize with zeros first
	pm.Process("link1", make([]int8, 128), -40, 64, testTime())

	// Then process many high-amplitude frames to trigger motion
	for i := 0; i < 30; i++ {
		pm.Process("link1", payload, -40, 64, testTime())
	}

	// Should have at least 1 active link now
	activeCount := 0
	for _, state := range pm.GetAllMotionStates() {
		if state.MotionDetected {
			activeCount++
		}
	}

	if activeCount == 0 {
		t.Error("Expected at least one active link with motion")
	}
}

func TestProcessorManager_RestoreBaseline(t *testing.T) {
	pm := NewProcessorManager(ProcessorManagerConfig{
		NSub:       64,
		FusionRate: 10.0,
		Tau:        30.0,
	})

	// Create a snapshot
	values := make([]float64, 64)
	for i := range values {
		values[i] = float64(i) * 2
	}
	snapshot := &BaselineSnapshot{
		Values:     values,
		SampleTime: testTime(),
		Confidence: 0.8,
	}

	// Restore it
	pm.RestoreBaseline("link1", snapshot)

	processor := pm.GetProcessor("link1")
	if processor == nil {
		t.Fatal("Expected processor to be created")
	}

	baseline := processor.GetBaseline()
	if !baseline.IsInitialized() {
		t.Error("Baseline should be initialized after restore")
	}

	confidence := baseline.GetConfidence()
	if math.Abs(confidence-0.8) > 0.01 {
		t.Errorf("Expected confidence 0.8, got %.2f", confidence)
	}
}

func testTime() time.Time {
	return time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
}
