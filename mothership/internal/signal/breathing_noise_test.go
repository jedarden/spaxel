package signal

import (
	"math/rand"
	"testing"
)

// Test false positive rate with 1000 trials per spec requirement
func TestFFTBreathingDetector_NoDetectionWithNoise(t *testing.T) {
	bd := NewFFTBreathingDetector()

	// Generate uniform random noise (no periodic component)
	falsePositives := 0
	trials := 1000

	for trial := 0; trial < trials; trial++ {
		bd.Reset()

		// Fill buffer with random noise (sigma=0.001)
		for i := 0; i < FFTBreathingBufferSize; i++ {
			noise := (rand.Float64() - 0.5) * 0.001
			bd.AddSample(noise)
	 }

        result := bd.Detect()
        if result.IsBreathing {
            falsePositives++
        }
    }

	falsePositiveRate := float64(falsePositives) / float64(trials)
    t.Logf("False positive rate: %.1f%% (target < 5%%)", falsePositiveRate*100)

    // Allow up to 5% false positive rate
    if falsePositiveRate > 0.05 {
        t.Errorf("False positive rate = %.1f%%, want < 5%%", falsePositiveRate*100)
    }
}
