package signal

// Regression tests for the nSub mismatch between the configured HT20 map
// (64 subcarriers) and what real HT20 L-LTF captures report (52 I/Q pairs;
// firmware csi.c: n_sub = info->len/2). The NBVI selection built for nSub=64
// handed indices up to 62 to PhaseVariance, which indexed the shorter
// per-frame ResidualPhase and panicked — killing the node's WebSocket
// connection on the first real frame.

import (
	"math"
	"math/rand"
	"testing"
	"time"
)

// pseudoRandomPayload builds nSub interleaved I/Q int8 pairs with
// deterministic pseudo-random content (varied enough to exercise Welford
// statistics and phase regression, stable enough for reproducible tests).
func pseudoRandomPayload(nSub int, seed int64) []int8 {
	rng := rand.New(rand.NewSource(seed))
	payload := make([]int8, nSub*2)
	for i := range payload {
		payload[i] = int8(rng.Intn(61)) - 30
	}
	return payload
}

// processedHT20 runs PhaseSanitize over a pseudo-random payload, standing in
// for one real frame of nSub usable subcarriers.
func processedHT20(t *testing.T, nSub int, seed int64) *ProcessedCSI {
	t.Helper()
	processed, err := PhaseSanitize(pseudoRandomPayload(nSub, seed), -40, nSub)
	if err != nil {
		t.Fatalf("PhaseSanitize(%d subcarriers): %v", nSub, err)
	}
	return processed
}

func TestMeanPhase_IgnoresOutOfBoundsIndices(t *testing.T) {
	cases := []struct {
		name    string
		phase   []float64
		indices []int
		want    float64
	}{
		{"empty indices", []float64{1, 2, 3}, nil, 0},
		{"all in range", []float64{1, 2, 3}, []int{0, 2}, 2},
		{"out of range skipped", []float64{1, 2, 3}, []int{0, 2, 5, 7}, 2},
		{"negative index skipped", []float64{1, 2, 3}, []int{-1, 1}, 2},
		{"all out of range", []float64{1, 2, 3}, []int{9, 10}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MeanPhase(tc.phase, tc.indices)
			if math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("MeanPhase = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPhaseVariance_IgnoresOutOfBoundsIndices(t *testing.T) {
	cases := []struct {
		name    string
		phase   []float64
		indices []int
		want    float64
	}{
		{"fewer than two valid", []float64{1, 2, 3}, []int{0, 9}, 0},
		{"all in range", []float64{1, 2, 3}, []int{0, 1, 2}, 2.0 / 3.0},
		{"out of range excluded from mean and count", []float64{1, 2, 3}, []int{0, 1, 9}, 0.25},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := PhaseVariance(tc.phase, tc.indices)
			if math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("PhaseVariance = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestMotionDetector_HandlesShortHT20Frames is the core regression: a
// detector configured for the 64-subcarrier map must keep processing
// 52-subcarrier frames — the shape every real HT20 node sends — and keep its
// subcarrier selection bounded by what the frames actually carry.
func TestMotionDetector_HandlesShortHT20Frames(t *testing.T) {
	md := NewMotionDetector(64)

	baseline := make([]float64, 64)
	for k := range baseline {
		baseline[k] = 100.0
	}

	const frames = 200 // crosses NBVIMinSamples (50) and a recalculation (80)
	for i := 0; i < frames; i++ {
		processed := processedHT20(t, 52, int64(i))
		// Push amplitudes away from the flat 100 baseline so deltaRMS is
		// meaningfully nonzero.
		for k := range processed.Amplitude {
			processed.Amplitude[k] += 10
		}

		features := md.Process(processed, baseline)

		if features.DeltaRMS <= 0 {
			t.Fatalf("frame %d: DeltaRMS = %v, want > 0 for frames that differ from baseline", i, features.DeltaRMS)
		}
		if math.IsNaN(features.PhaseVariance) || math.IsNaN(features.SmoothDeltaRMS) {
			t.Fatalf("frame %d: NaN in features (%+v)", i, features)
		}
		if i+1 < 2*NBVIUpdateInterval {
			// Before the first NBVI recalculation (which fires inside the
			// update of frame 79, when count reaches 80), the selection
			// falls back to every data carrier the frame carries:
			// 52 - 2 null - 11 guard - 3 pilots below index 52
			if features.SelectedCount != 36 {
				t.Fatalf("frame %d (pre-selection): SelectedCount = %d, want 36", i, features.SelectedCount)
			}
		} else if features.SelectedCount > NBVITopCount {
			// Post-training: NBVI top-N selection is in force
			t.Fatalf("frame %d: SelectedCount = %d exceeds NBVITopCount", i, features.SelectedCount)
		}
	}

	indices := md.GetNBVITracker().GetSelectedIndices()
	for _, k := range indices {
		if k >= 52 {
			t.Errorf("selected index %d exceeds the 52-subcarrier frames the tracker was fed", k)
		}
	}
	if len(indices) == 0 {
		t.Error("selection empty after training on short frames")
	}
}

// TestMotionDetector_MixedFrameLengths covers fleets that report both shapes
// (e.g. mixed firmware during an OTA rollout): neither length may panic, and
// features must stay finite across the whole stream.
func TestMotionDetector_MixedFrameLengths(t *testing.T) {
	md := NewMotionDetector(64)

	baseline := make([]float64, 64)
	for k := range baseline {
		baseline[k] = 100.0
	}

	lengths := []int{64, 52, 52, 64, 52, 64, 64, 52}
	for i, nSub := range lengths {
		processed := processedHT20(t, nSub, int64(i))
		features := md.Process(processed, baseline)
		if math.IsNaN(features.SmoothDeltaRMS) || math.IsNaN(features.PhaseVariance) {
			t.Fatalf("frame %d (nSub=%d): NaN in features (%+v)", i, nSub, features)
		}
	}
}

// TestNBVITracker_TrainsOnShortFrames checks the tracker actually learns from
// frames shorter than the configured map instead of discarding them.
func TestNBVITracker_TrainsOnShortFrames(t *testing.T) {
	tracker := NewNBVITracker(64)

	// Every data subcarrier below 52 oscillates with a distinct amplitude;
	// subcarrier parity splits high- from low-variance carriers.
	for i := 0; i < 100; i++ {
		amplitude := make([]float64, 52)
		for k := 0; k < 52; k++ {
			if IsDataSubcarrier(k) {
				if k%2 == 0 {
					amplitude[k] = 100.0 + float64(i%20)
				} else {
					amplitude[k] = 100.0
				}
			}
		}
		tracker.Update(amplitude)
	}

	if got := tracker.SampleCount(); got != 100 {
		t.Errorf("SampleCount = %d, want 100 (short frames must count as samples)", got)
	}

	indices := tracker.GetSelectedIndices()
	if len(indices) == 0 {
		t.Fatal("no subcarriers selected after training on short frames")
	}
	for _, k := range indices {
		if k >= 52 {
			t.Errorf("selected index %d was never observed in a 52-carrier frame", k)
		}
	}
}

// TestDiurnalBaseline_LearnsFromShortFrames: the exact-length guard used to
// silently discard every short frame, so the diurnal slot never learned on a
// real HT20 deployment.
func TestDiurnalBaseline_LearnsFromShortFrames(t *testing.T) {
	db := NewDiurnalBaseline("diurnal-test", 64)

	amplitude := make([]float64, 52)
	for k := range amplitude {
		amplitude[k] = 100.0
	}
	for i := 0; i < 3; i++ {
		db.Update(amplitude)
	}

	slot := db.GetSlot(time.Now().Hour())
	if slot == nil {
		t.Fatal("no slot for current hour")
	}
	if slot.SampleCount == 0 {
		t.Error("diurnal slot never learned from 52-carrier quiet frames")
	}
}

// TestLinkProcessor_HT20ShortFramesEndToEnd drives the full per-link pipeline
// the ingestion server calls, configured for 64 but fed the 52-carrier frames
// real HT20 firmware produces.
func TestLinkProcessor_HT20ShortFramesEndToEnd(t *testing.T) {
	lp := NewLinkProcessor("ht20-link", 64, 0.0033)

	const frames = 100
	for i := 0; i < frames; i++ {
		result, err := lp.Process(pseudoRandomPayload(52, int64(i)), -40, 52, testTime())
		if err != nil {
			t.Fatalf("frame %d: Process returned error: %v", i, err)
		}
		if result == nil || result.Features == nil {
			t.Fatalf("frame %d: missing features", i)
		}
	}

	indices := lp.GetMotionDetector().GetNBVITracker().GetSelectedIndices()
	for _, k := range indices {
		if k >= 52 {
			t.Errorf("selection leaked index %d beyond the 52-carrier stream", k)
		}
	}
}

// TestProcessorManager_ShortNSubFrame exercises the exact ingestion call —
// ProcessorManager configured with NSub:64 receiving a frame that declares
// nSub=52, the shape that used to panic inside MeanPhase.
func TestProcessorManager_ShortNSubFrame(t *testing.T) {
	pm := NewProcessorManager(ProcessorManagerConfig{
		NSub:       64,
		FusionRate: 10.0,
		Tau:        30.0,
	})

	for i := 0; i < 60; i++ {
		result, err := pm.Process("node-a", pseudoRandomPayload(52, int64(i)), -40, 52, testTime())
		if err != nil {
			t.Fatalf("frame %d: Process returned error: %v", i, err)
		}
		if result == nil || result.Processed == nil {
			t.Fatalf("frame %d: missing processed CSI", i)
		}
		if got := len(result.Processed.ResidualPhase); got != 52 {
			t.Fatalf("frame %d: ResidualPhase length %d, want 52", i, got)
		}
	}
}
