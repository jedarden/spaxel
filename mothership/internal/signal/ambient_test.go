package signal

import (
	"math"
	"testing"
	"time"
)

func TestLinkHealth_New(t *testing.T) {
	lh := NewLinkHealth("test:link", 64)
	if lh == nil {
		t.Fatal("NewLinkHealth returned nil")
	}
	if lh.nSub != 64 {
		t.Errorf("nSub = %d, want 64", lh.nSub)
	}
	if lh.configuredRate != float64(HealthSampleRate) {
		t.Errorf("configuredRate = %f, want %f", lh.configuredRate, float64(HealthSampleRate))
	}
}

func TestLinkHealth_ComputeHealth_AllOnes(t *testing.T) {
	lh := NewLinkHealth("test:link", 64)

	// Manually set sub-scores to 1.0
	lh.mu.Lock()
	lh.SNRScore = 1.0
	lh.PhaseStabilityScore = 1.0
	lh.PacketRateScore = 1.0
	lh.DriftScore = 1.0
	lh.mu.Unlock()

	lh.ComputeHealth()

	confidence := lh.GetAmbientConfidence()
	if math.Abs(confidence-1.0) > 0.001 {
		t.Errorf("Composite score with all 1.0 = %f, want 1.0", confidence)
	}
}

func TestLinkHealth_ComputeHealth_Weighted(t *testing.T) {
	lh := NewLinkHealth("test:link", 64)

	// Set packet_rate = 0.5, others = 1.0
	// Expected: 0.4*1.0 + 0.3*1.0 + 0.2*0.5 + 0.1*1.0 = 0.4 + 0.3 + 0.1 + 0.1 = 0.9
	lh.mu.Lock()
	lh.SNRScore = 1.0
	lh.PhaseStabilityScore = 1.0
	lh.PacketRateScore = 0.5
	lh.DriftScore = 1.0
	lh.mu.Unlock()

	lh.ComputeHealth()

	confidence := lh.GetAmbientConfidence()
	expected := SNRWeight*1.0 + PhaseStabilityWeight*1.0 + PacketRateWeight*0.5 + BaselineDriftWeight*1.0
	if math.Abs(confidence-expected) > 0.001 {
		t.Errorf("Composite score = %f, want %f", confidence, expected)
	}
}

func TestLinkHealth_SNRScoreMapping(t *testing.T) {
	tests := []struct {
		name     string
		snrRatio float64
		wantMin  float64
		wantMax  float64
	}{
		{"SNR=1 (ratio=1)", 1.0, 0.0, 0.001},
		{"SNR=10 (ratio=10)", 10.0, 0.49, 0.51},
		{"SNR=100 (ratio=100)", 100.0, 0.99, 1.01},
		{"SNR=1000 (ratio=1000)", 1000.0, 0.99, 1.01}, // capped at 1.0
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lh := NewLinkHealth("test:link", 64)

			// Add motion samples with signal level matching snrRatio
			for i := 0; i < 20; i++ {
				lh.deltaRMSHistory[i] = tt.snrRatio
			}
			lh.deltaRMSCount = 20

			// Add quiet samples with variance = 1 (noise std = 1)
			for i := 0; i < 20; i++ {
				lh.quietDeltaRMSHistory[i] = 1.0 // mean=1, variance=0
			}
			lh.quietDeltaRMSCount = 20
			// Actually add some variance
			for i := 0; i < 20; i++ {
				if i%2 == 0 {
					lh.quietDeltaRMSHistory[i] = 1.5
				} else {
					lh.quietDeltaRMSHistory[i] = 0.5
				}
			}
			// Now quietMean = 1.0, quietVariance = 0.25, quietStd = 0.5
			// SNR = signalLevel / quietStd = snrRatio / 0.5 = 2 * snrRatio

			lh.ComputeHealth()
			details := lh.GetHealthDetails()

			// The actual score will be based on adjusted SNR due to variance
			if details.SNR < tt.wantMin || details.SNR > tt.wantMax {
				t.Errorf("SNR score for ratio %f = %f, want [%f, %f]",
					tt.snrRatio, details.SNR, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestLinkHealth_PhaseStabilityScore(t *testing.T) {
	tests := []struct {
		name          string
		variance      float64
		expectedScore float64
	}{
		{"variance=0", 0.0, 1.0},
		{"variance=0.25", 0.25, 0.5},
		{"variance=0.5", 0.5, 0.0},
		{"variance=1.0 (high)", 1.0, 0.0}, // capped at 0
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lh := NewLinkHealth("test:link", 64)

			// Fill phase variance history
			for i := 0; i < PhaseStabilityWindow; i++ {
				lh.phaseVarHistory[i] = tt.variance
			}
			lh.phaseVarCount = PhaseStabilityWindow

			lh.ComputeHealth()
			details := lh.GetHealthDetails()

			if math.Abs(details.PhaseStability-tt.expectedScore) > 0.01 {
				t.Errorf("Phase stability score for variance %f = %f, want %f",
					tt.variance, details.PhaseStability, tt.expectedScore)
			}
		})
	}
}

func TestLinkHealth_PacketRateScore(t *testing.T) {
	lh := NewLinkHealth("test:link", 64)
	lh.SetConfiguredRate(20.0)

	// Add timestamps at 10 Hz for 30 samples (3 seconds)
	now := time.Now()
	for i := 0; i < 30; i++ {
		lh.UpdateTimestamp(now.Add(-time.Duration(i*100) * time.Millisecond))
	}

	lh.ComputeHealth()
	details := lh.GetHealthDetails()

	// At 10 Hz with 20 Hz configured, score should be ~0.5
	if details.PacketRate < 0.4 || details.PacketRate > 0.6 {
		t.Errorf("Packet rate score = %f, want ~0.5 (10 Hz actual / 20 Hz configured)", details.PacketRate)
	}
}

func TestLinkHealth_BaselineDriftScore(t *testing.T) {
	lh := NewLinkHealth("test:link", 64)

	// Create two baseline snapshots with 5% difference (normalized L2)
	nSub := 64
	oldBaseline := make([]float64, nSub)
	newBaseline := make([]float64, nSub)

	// Old baseline: all 1.0
	for i := 0; i < nSub; i++ {
		oldBaseline[i] = 1.0
	}
	// New baseline: 5% higher (drift_rate = 0.05)
	for i := 0; i < nSub; i++ {
		newBaseline[i] = 1.05
	}

	lh.baselineHistory[0] = oldBaseline
	lh.baselineHistory[1] = newBaseline
	lh.baselineWriteIdx = 2
	lh.baselineCount = 2

	lh.ComputeHealth()
	details := lh.GetHealthDetails()

	// With 5% drift: score = 1 - 0.05/0.1 = 0.5
	expected := 0.5
	if math.Abs(details.BaselineDrift-expected) > 0.1 {
		t.Errorf("Baseline drift score = %f, want ~%f", details.BaselineDrift, expected)
	}
}

func TestLinkHealth_CompositeScoreWeights(t *testing.T) {
	// Verify the weights add up to 1.0
	totalWeight := SNRWeight + PhaseStabilityWeight + PacketRateWeight + BaselineDriftWeight
	if math.Abs(totalWeight-1.0) > 0.001 {
		t.Errorf("Weight sum = %f, want 1.0", totalWeight)
	}

	// Verify expected values
	if SNRWeight != 0.40 {
		t.Errorf("SNRWeight = %f, want 0.40", SNRWeight)
	}
	if PhaseStabilityWeight != 0.30 {
		t.Errorf("PhaseStabilityWeight = %f, want 0.30", PhaseStabilityWeight)
	}
	if PacketRateWeight != 0.20 {
		t.Errorf("PacketRateWeight = %f, want 0.20", PacketRateWeight)
	}
	if BaselineDriftWeight != 0.10 {
		t.Errorf("BaselineDriftWeight = %f, want 0.10", BaselineDriftWeight)
	}
}

func TestLinkHealth_BreathingHealthThreshold(t *testing.T) {
	// Verify the threshold constant
	if BreathingHealthThreshold != 0.7 {
		t.Errorf("BreathingHealthThreshold = %f, want 0.7", BreathingHealthThreshold)
	}
}

func TestHealthManager_GetOrCreate(t *testing.T) {
	hm := NewHealthManager(64)

	// First access creates
	h1 := hm.GetOrCreate("link1")
	if h1 == nil {
		t.Fatal("GetOrCreate returned nil")
	}

	// Second access returns same instance
	h2 := hm.GetOrCreate("link1")
	if h1 != h2 {
		t.Error("GetOrCreate should return same instance for same linkID")
	}

	// Different linkID creates new instance
	h3 := hm.GetOrCreate("link2")
	if h1 == h3 {
		t.Error("GetOrCreate should return different instance for different linkID")
	}
}

func TestHealthManager_GetSystemHealth(t *testing.T) {
	hm := NewHealthManager(64)

	// Empty manager returns 0
	if score := hm.GetSystemHealth(); score != 0 {
		t.Errorf("Empty system health = %f, want 0", score)
	}

	// Add two links with known scores
	h1 := hm.GetOrCreate("link1")
	h1.mu.Lock()
	h1.ambientConfidence = 0.8
	h1.mu.Unlock()

	h2 := hm.GetOrCreate("link2")
	h2.mu.Lock()
	h2.ambientConfidence = 0.6
	h2.mu.Unlock()

	// Average should be 0.7
	expected := 0.7
	if score := hm.GetSystemHealth(); math.Abs(score-expected) > 0.001 {
		t.Errorf("System health = %f, want %f", score, expected)
	}
}

func TestHealthManager_GetWorstLink(t *testing.T) {
	hm := NewHealthManager(64)

	// Add three links
	h1 := hm.GetOrCreate("link1")
	h2 := hm.GetOrCreate("link2")
	h3 := hm.GetOrCreate("link3")

	h1.mu.Lock()
	h1.ambientConfidence = 0.9
	h1.mu.Unlock()

	h2.mu.Lock()
	h2.ambientConfidence = 0.5
	h2.mu.Unlock()

	h3.mu.Lock()
	h3.ambientConfidence = 0.7
	h3.mu.Unlock()

	linkID, score := hm.GetWorstLink()
	if linkID != "link2" {
		t.Errorf("Worst link ID = %s, want link2", linkID)
	}
	if math.Abs(score-0.5) > 0.001 {
		t.Errorf("Worst link score = %f, want 0.5", score)
	}
}

func TestLinkHealth_GetHealthDetails(t *testing.T) {
	lh := NewLinkHealth("test:link", 64)

	lh.mu.Lock()
	lh.SNRScore = 0.91
	lh.PhaseStabilityScore = 0.78
	lh.PacketRateScore = 0.97
	lh.DriftScore = 0.62
	lh.mu.Unlock()

	details := lh.GetHealthDetails()

	if math.Abs(details.SNR-0.91) > 0.001 {
		t.Errorf("SNR detail = %f, want 0.91", details.SNR)
	}
	if math.Abs(details.PhaseStability-0.78) > 0.001 {
		t.Errorf("PhaseStability detail = %f, want 0.78", details.PhaseStability)
	}
	if math.Abs(details.PacketRate-0.97) > 0.001 {
		t.Errorf("PacketRate detail = %f, want 0.97", details.PacketRate)
	}
	if math.Abs(details.BaselineDrift-0.62) > 0.001 {
		t.Errorf("BaselineDrift detail = %f, want 0.62", details.BaselineDrift)
	}
}

func TestLinkHealth_UpdateDeltaRMS(t *testing.T) {
	lh := NewLinkHealth("test:link", 64)

	// Update motion deltaRMS
	lh.UpdateDeltaRMS(0.5, true)
	lh.UpdateDeltaRMS(0.6, true)
	lh.UpdateDeltaRMS(0.4, true)

	lh.mu.RLock()
	if lh.deltaRMSCount != 3 {
		t.Errorf("deltaRMSCount = %d, want 3", lh.deltaRMSCount)
	}
	lh.mu.RUnlock()

	// Update quiet deltaRMS
	lh.UpdateDeltaRMS(0.05, false)
	lh.UpdateDeltaRMS(0.03, false)

	lh.mu.RLock()
	if lh.quietDeltaRMSCount != 2 {
		t.Errorf("quietDeltaRMSCount = %d, want 2", lh.quietDeltaRMSCount)
	}
	lh.mu.RUnlock()
}

func TestLinkHealth_ClampToRange(t *testing.T) {
	lh := NewLinkHealth("test:link", 64)

	// Test clamping with extreme values
	lh.mu.Lock()
	lh.SNRScore = 2.0   // Above 1.0
	lh.PhaseStabilityScore = -0.5 // Below 0
	lh.PacketRateScore = 0.5
	lh.DriftScore = 0.5
	lh.mu.Unlock()

	lh.ComputeHealth()

	confidence := lh.GetAmbientConfidence()
	// Should be clamped to [0, 1]
	if confidence < 0 || confidence > 1 {
		t.Errorf("Composite score = %f, should be in [0, 1]", confidence)
	}
}

func TestLinkHealth_Reset(t *testing.T) {
	lh := NewLinkHealth("test:link", 64)

	// Add some data
	lh.UpdateRSSI(-50)
	lh.UpdateTimestamp(time.Now())
	lh.UpdatePhaseVariance(0.1)
	lh.UpdateDeltaRMS(0.5, true)

	// Reset
	lh.Reset()

	// Verify cleared
	lh.mu.RLock()
	if lh.rssiCount != 0 {
		t.Errorf("rssiCount after reset = %d, want 0", lh.rssiCount)
	}
	if lh.timestampCount != 0 {
		t.Errorf("timestampCount after reset = %d, want 0", lh.timestampCount)
	}
	if lh.phaseVarCount != 0 {
		t.Errorf("phaseVarCount after reset = %d, want 0", lh.phaseVarCount)
	}
	if lh.deltaRMSCount != 0 {
		t.Errorf("deltaRMSCount after reset = %d, want 0", lh.deltaRMSCount)
	}
	lh.mu.RUnlock()
}
