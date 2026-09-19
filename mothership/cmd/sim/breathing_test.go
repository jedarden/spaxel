// Tests for the stationary-breathing scenario (--scenario stationary).
//
// Three layers are pinned:
//
//  1. Kinematics — the scripted chest displacement: correct frequency and
//     amplitude on the Z axis only, per-walker phase separation, and a
//     no-op when the oscillation is disabled.
//  2. CSI rendering — the displacement survives the two-ray generator and
//     phase sanitization as a tone at the configured frequency in the mean
//     detrended phase (the common-mode component DetrendedPhase retains;
//     the OLS ResidualPhase cancels it). The ambient generator wobble
//     (0.1 rad at 0.2 Hz) is common to live and disabled streams, so the
//     breathing tone is isolated by differencing streams generated with
//     identical per-frame seeds.
//  3. Quiet amplitude channel — the oscillation must not move the
//     amplitude the way walking does; breathing only shifts phase.
//
// Detector-side note (measured, out of scope here): the pipeline's
// breathing gate (BreathingMotionThreshold = 0.03, processor.go) compares
// against SmoothDeltaRMS in absolute RSSI-normalized amplitude units, where
// int8 I/Q quantization alone keeps any stream at ~0.23-0.46 — measured by
// driving this fixture's frames through the full signal.LinkProcessor;
// disabled and breathing-4mm streams are statistically identical, and zero
// frames fall below the gate in either. Opening the gate is detector
// calibration work (spaxel-a27b6dba family); this fixture supplies the
// micro-motion and keeps the amplitude channel still relative to walking.
package main

import (
	"bytes"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/spaxel/mothership/internal/signal"
)

func newBreathTestSpace() *Space {
	return &Space{Width: 6, Depth: 5, Height: 2.5}
}

// breathTestLink returns a TX/RX pair matching the default node geometry
// (nodes at Z = 2.0, corners of the default space) with the walker standing
// at the space center: ~0.3 m below the link chord, inside the first
// Fresnel zone, where the scripted displacement renders cleanly.
func breathTestLink() (*VirtualNode, *VirtualNode) {
	tx := &VirtualNode{ID: 1, Position: Point{X: 0.5, Y: 0.5, Z: 2.0}}
	rx := &VirtualNode{ID: 2, Position: Point{X: 5.5, Y: 4.5, Z: 2.0}}
	return tx, rx
}

// breathingFrameStream generates frames frames of CSI for the given walkers,
// stepping st once per frame at the default 20 Hz rate (st may be nil for no
// scenario). Each frame uses its own seeded rng so two streams generated
// with the same frame indices and walkers are directly comparable.
func breathingFrameStream(t *testing.T, tx, rx *VirtualNode, walkers []*Walker, st *BreathingScenarioState, frames int) [][]byte {
	t.Helper()
	out := make([][]byte, 0, frames)
	for i := 0; i < frames; i++ {
		if st != nil {
			st.Update(1.0 / 20.0)
		}
		frame := generateCSIFrame(tx, rx, walkers, nil, i, rand.New(rand.NewSource(1000+int64(i))))
		out = append(out, frame)
	}
	return out
}

func newStationaryWalker() *Walker {
	return &Walker{ID: 0, Position: Point{X: 3.0, Y: 2.5, Z: 1.7}, Height: 1.7}
}

// TestBreathingScenario_DisplacementKinematics walks the scenario through
// two minutes of virtual time at the 20 Hz frame rate and pins, for rates
// across the documented band: the configured amplitude is the exact
// oscillation peak, the oscillation frequency matches the configuration,
// and nothing but Z ever moves.
func TestBreathingScenario_DisplacementKinematics(t *testing.T) {
	tests := []struct {
		name    string
		rateHz  float64
		ampMM   float64
		walkers int
	}{
		{"0.1 Hz lower band edge", 0.1, 4, 1},
		{"0.2 Hz default", 0.2, 4, 1},
		{"0.4 Hz high normal", 0.4, 2, 1},
		{"0.5 Hz upper band edge", 0.5, 1, 1},
		{"two walkers phase-separated", 0.2, 4, 2},
	}

	const (
		frames = 2400 // 120 s at the 20 Hz frame rate
		dt     = 1.0 / 20.0
	)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			walkers := make([]*Walker, tt.walkers)
			for i := range walkers {
				walkers[i] = &Walker{
					ID:       i,
					Position: Point{X: 3.0, Y: 2.5, Z: 1.7},
					Height:   1.7,
				}
			}
			st, err := NewBreathingScenarioState(walkers, tt.rateHz, tt.ampMM)
			if err != nil {
				t.Fatalf("NewBreathingScenarioState(%v Hz, %v mm) failed: %v", tt.rateHz, tt.ampMM, err)
			}
			if want := tt.ampMM / 1000; math.Abs(st.AmpM-want) > 1e-12 {
				t.Errorf("AmpM = %.6f, want %.6f", st.AmpM, want)
			}
			if st.RateHz != tt.rateHz {
				t.Errorf("RateHz = %.4f, want %.4f", st.RateHz, tt.rateHz)
			}

			peaks := make([]float64, tt.walkers)
			crossings := make([]int, tt.walkers)
			prev := make([]float64, tt.walkers)
			for i := 0; i < frames; i++ {
				st.Update(dt)
				for w, walker := range st.walkers {
					d := walker.Position.Z - st.base[w].Z
					if abs := math.Abs(d); abs > peaks[w] {
						peaks[w] = abs
					}
					if (prev[w] < 0 && d >= 0) || (prev[w] > 0 && d <= 0) {
						crossings[w]++
					}
					prev[w] = d
					// Chest rise is vertical only; X/Y drift would read as walking.
					if walker.Position.X != st.base[w].X || walker.Position.Y != st.base[w].Y {
						t.Fatalf("walker %d drifted off-axis: X/Y = %.4f/%.4f, want %.4f/%.4f",
							w, walker.Position.X, walker.Position.Y, st.base[w].X, st.base[w].Y)
					}
				}
			}

			// The chest must rise by the configured amplitude. A sampled
			// sine misses its true peak by up to A·(1−cos(π·f·dt)) — the
			// nearest sample to the peak is at most half a frame away — so
			// the tolerance is that sampling deficit, not floating-point
			// epsilon.
			wantPeak := tt.ampMM / 1000
			sampleDeficit := wantPeak * (1 - math.Cos(math.Pi*tt.rateHz*dt))
			for w, peak := range peaks {
				if wantPeak-peak > sampleDeficit*1.01 {
					t.Errorf("walker %d peak displacement = %.9f m, want %.9f m (sampling deficit %.2e)",
						w, peak, wantPeak, sampleDeficit)
				}
			}

			// Two zero crossings per cycle: frequency recovered from the
			// displacement series must match the configuration. Window
			// alignment costs at most ±2 crossings over the run.
			wantCrossings := 2 * tt.rateHz * frames * dt
			for w, ncross := range crossings {
				if math.Abs(float64(ncross)-wantCrossings) > 2 {
					t.Errorf("walker %d displacement crossings = %d, want %.0f±2 (%.4f Hz)",
						w, ncross, wantCrossings, tt.rateHz)
				}
			}

			// Multiple stationary people must not breathe in lockstep.
			if tt.walkers > 1 && st.chestDisplacement(0) == st.chestDisplacement(1) {
				t.Error("walkers 0 and 1 have identical breathing phase; expected per-walker separation")
			}
		})
	}
}

// TestBreathingScenario_ParameterValidation pins the configuration guards:
// rates outside the 0.1-0.5 Hz band and amplitudes outside [0, 20] mm are
// rejected, ampMM = 0 disables the oscillation (any in-band rate), and a
// disabled state leaves the walkers exactly where they stand.
func TestBreathingScenario_ParameterValidation(t *testing.T) {
	if ScenarioStationary != "stationary" {
		t.Errorf(`ScenarioStationary = %q, want "stationary"`, ScenarioStationary)
	}

	tests := []struct {
		name    string
		rateHz  float64
		ampMM   float64
		wantErr bool
	}{
		{"rate below band", 0.05, 4, true},
		{"rate above band", 0.6, 4, true},
		{"negative amplitude", 0.2, -1, true},
		{"amplitude over 20mm", 0.2, 21, true},
		{"rate at lower edge", 0.1, 4, false},
		{"rate at upper edge", 0.5, 20, false},
		{"disabled ignores rate", 99, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st, err := NewBreathingScenarioState([]*Walker{newStationaryWalker()}, tt.rateHz, tt.ampMM)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewBreathingScenarioState(%.2f Hz, %.1f mm) error = %v, wantErr %v",
					tt.rateHz, tt.ampMM, err, tt.wantErr)
			}
			if err == nil && tt.ampMM == 0 {
				before := st.walkers[0].Position
				for i := 0; i < 100; i++ {
					st.Update(1.0 / 20.0)
				}
				if st.walkers[0].Position != before {
					t.Errorf("disabled scenario moved the walker: %+v -> %+v", before, st.walkers[0].Position)
				}
			}
		})
	}
}

// meanDetrendedPhase runs a generated frame through the unmodified phase
// sanitization and returns the mean detrended phase over data subcarriers —
// the common-mode component the breathing displacement modulates.
func meanDetrendedPhase(t *testing.T, frame []byte) float64 {
	t.Helper()
	payload := make([]int8, nSub*2)
	for k := 0; k < nSub*2; k++ {
		payload[k] = int8(frame[headerSize+k])
	}
	pc, err := signal.PhaseSanitize(payload, int8(frame[20]), nSub)
	if err != nil {
		t.Fatalf("PhaseSanitize failed: %v", err)
	}
	return signal.MeanPhase(pc.DetrendedPhase, signal.DataSubcarrierIndices(nSub))
}

func removeMean(s []float64) {
	var m float64
	for _, v := range s {
		m += v
	}
	m /= float64(len(s))
	for i := range s {
		s[i] -= m
	}
}

func rmsOf(s []float64) float64 {
	var sum float64
	for _, v := range s {
		sum += v * v
	}
	return math.Sqrt(sum / float64(len(s)))
}

// goertzelAmp returns the amplitude of the tone at freq in s (samples at fs).
func goertzelAmp(s []float64, freq, fs float64) float64 {
	n := float64(len(s))
	k := math.Round(freq * n / fs)
	w := 2 * math.Pi * k / n
	coeff := 2 * math.Cos(w)
	var s1, s2 float64
	for _, v := range s {
		s0 := v + coeff*s1 - s2
		s2, s1 = s1, s0
	}
	power := s1*s1 + s2*s2 - coeff*s1*s2
	return math.Sqrt(math.Max(0, 4*power/(n*n)))
}

// peakToneHz scans fMin..fMax and returns the frequency and amplitude of the
// strongest tone.
func peakToneHz(s []float64, fMin, fMax, fs float64) (float64, float64) {
	bestF, bestA := 0.0, -1.0
	for f := fMin; f <= fMax; f += fs / float64(len(s)) {
		if a := goertzelAmp(s, f, fs); a > bestA {
			bestA, bestF = a, f
		}
	}
	return bestF, bestA
}

// TestBreathingScenario_CSIBreathingSpectralPeak is the core fixture
// assertion: for two configured frequencies (0.2 and 0.4 Hz) the
// synthesized stream's breathing component — isolated by differencing
// against a breathing-disabled stream generated with identical per-frame
// seeds, which cancels the ambient generator wobble exactly — has its
// spectral peak at the configured frequency inside the documented
// 0.1-0.5 Hz band, strong enough to exercise the detector (>= 3x its 0.005
// rad threshold), linear in the configured displacement amplitude, and
// absent from the disabled control.
func TestBreathingScenario_CSIBreathingSpectralPeak(t *testing.T) {
	const (
		frames = 800 // 40 s: 8 cycles at 0.2 Hz, 16 at 0.4
		fs     = 20.0 // Hz frame rate
	)

	tx, rx := breathTestLink()

	for _, rateHz := range []float64{0.2, 0.4} {
		runAt := func(ampMM float64) float64 {
			st, err := NewBreathingScenarioState([]*Walker{newStationaryWalker()}, rateHz, ampMM)
			if err != nil {
				t.Fatalf("NewBreathingScenarioState failed: %v", err)
			}
			live := breathingFrameStream(t, tx, rx, st.walkers, st, frames)
			off, err := NewBreathingScenarioState([]*Walker{newStationaryWalker()}, rateHz, 0)
			if err != nil {
				t.Fatalf("NewBreathingScenarioState disabled failed: %v", err)
			}
			still := breathingFrameStream(t, tx, rx, off.walkers, off, frames)

			diff := make([]float64, frames)
			for i := range live {
				diff[i] = meanDetrendedPhase(t, live[i]) - meanDetrendedPhase(t, still[i])
			}
			return goertzelAmp(diff, rateHz, fs)
		}

		amp4 := runAt(4)
		amp2 := runAt(2)

		// Peak must sit at the configured frequency and clear the
		// detector threshold with margin.
		if amp4 < 3*signal.BreathingThreshold {
			t.Errorf("breathing tone at %.1f Hz = %.6f rad, want >= %.6f rad (3x detector threshold)",
				rateHz, amp4, 3*signal.BreathingThreshold)
		}

		// Displacement amplitude must map linearly into the tone.
		if ratio := amp4 / amp2; ratio < 1.6 || ratio > 2.4 {
			t.Errorf("tone amplitude ratio 4mm/2mm = %.3f at %.1f Hz, want ~2 (linear displacement scaling)", ratio, rateHz)
		}
	}

	// Full-band peak localization on the difference stream: the strongest
	// tone between 0.05 and 1.0 Hz must be the configured breathing rate.
	for _, rateHz := range []float64{0.2, 0.4} {
		st, err := NewBreathingScenarioState([]*Walker{newStationaryWalker()}, rateHz, 4)
		if err != nil {
			t.Fatalf("NewBreathingScenarioState failed: %v", err)
		}
		live := breathingFrameStream(t, tx, rx, st.walkers, st, frames)
		off, _ := NewBreathingScenarioState([]*Walker{newStationaryWalker()}, rateHz, 0)
		still := breathingFrameStream(t, tx, rx, off.walkers, off, frames)

		diff := make([]float64, frames)
		for i := range live {
			diff[i] = meanDetrendedPhase(t, live[i]) - meanDetrendedPhase(t, still[i])
		}
		pf, pa := peakToneHz(diff, 0.05, 1.0, fs)
		if math.Abs(pf-rateHz) > 0.05 {
			t.Errorf("spectral peak at %.3f Hz (amp %.6f), want configured %.2f Hz", pf, pa, rateHz)
		}
		if pf < breathingMinHz || pf > breathingMaxHz {
			t.Errorf("spectral peak %.3f Hz outside the documented %.1f-%.1f Hz band", pf, breathingMinHz, breathingMaxHz)
		}

		// The raw stream's strongest tone must land inside the band too
		// (the ambient wobble and the breathing tone are both in-band).
		raw := make([]float64, frames)
		for i := range live {
			raw[i] = meanDetrendedPhase(t, live[i])
		}
		removeMean(raw)
		if rf, _ := peakToneHz(raw, 0.05, 1.0, fs); rf < breathingMinHz || rf > breathingMaxHz {
			t.Errorf("raw stream peak %.3f Hz outside the documented %.1f-%.1f Hz band", rf, breathingMinHz, breathingMaxHz)
		}

		// Determinism control: two disabled streams must be bit-identical
		// (so any live-still difference is the breathing, nothing else).
		still2 := breathingFrameStream(t, tx, rx, off.walkers, off, frames)
		for i := range still {
			if !bytes.Equal(still[i], still2[i]) {
				t.Fatalf("disabled control stream not deterministic at frame %d", i)
			}
		}
	}
}

// TestBreathingScenario_BreathingDisabledMatchesStillWalker pins the
// no-regression contract: with the oscillation disabled the scenario's CSI
// output is byte-identical to a plain walker standing still — the state
// machine adds nothing to the stream.
func TestBreathingScenario_BreathingDisabledMatchesStillWalker(t *testing.T) {
	tx, rx := breathTestLink()
	const frames = 200

	st, err := NewBreathingScenarioState([]*Walker{newStationaryWalker()}, 0.2, 0)
	if err != nil {
		t.Fatalf("NewBreathingScenarioState failed: %v", err)
	}
	disabled := breathingFrameStream(t, tx, rx, st.walkers, st, frames)
	// No scenario state: the same walker simply never moves.
	still := breathingFrameStream(t, tx, rx, []*Walker{newStationaryWalker()}, nil, frames)

	for i := range disabled {
		if !bytes.Equal(disabled[i], still[i]) {
			t.Fatalf("frame %d differs between disabled scenario and plain still walker", i)
		}
	}
}

// TestBreathingScenario_AmplitudeChannelQuiet pins the other half of the
// fixture contract: breathing shifts phase, not amplitude. The mean
// RSSI-normalized amplitude of the live stream must stay within a fraction
// of a percent of the disabled stream's — orders of magnitude below the
// 30-70% swings a walking person produces — so the fixture cannot be
// mistaken for walking by any amplitude-based stage.
func TestBreathingScenario_AmplitudeChannelQuiet(t *testing.T) {
	tx, rx := breathTestLink()
	const frames = 400

	st, err := NewBreathingScenarioState([]*Walker{newStationaryWalker()}, 0.2, 4)
	if err != nil {
		t.Fatalf("NewBreathingScenarioState failed: %v", err)
	}
	live := breathingFrameStream(t, tx, rx, st.walkers, st, frames)
	off, _ := NewBreathingScenarioState([]*Walker{newStationaryWalker()}, 0.2, 0)
	still := breathingFrameStream(t, tx, rx, off.walkers, off, frames)

	meanAmp := func(stream [][]byte) float64 {
		var total float64
		for _, frame := range stream {
			payload := make([]int8, nSub*2)
			for k := 0; k < nSub*2; k++ {
				payload[k] = int8(frame[headerSize+k])
			}
			pc, err := signal.PhaseSanitize(payload, int8(frame[20]), nSub)
			if err != nil {
				t.Fatalf("PhaseSanitize failed: %v", err)
			}
			var sum float64
			for _, v := range pc.Amplitude {
				sum += v
			}
			total += sum / float64(len(pc.Amplitude))
		}
		return total / float64(len(stream))
	}

	liveMean, stillMean := meanAmp(live), meanAmp(still)
	if rel := math.Abs(liveMean-stillMean) / stillMean; rel > 0.005 {
		t.Errorf("breathing shifted mean amplitude by %.4f%% (live %.4f vs still %.4f); want < 0.5%%", 100*rel, liveMean, stillMean)
	}
}

// TestBreathingScenario_FrameClockDeterminism pins the scripted-clock
// contract: the oscillation is a function of accumulated frame time only —
// stepping the scenario produces identical positions regardless of any
// wall-clock delay between steps.
func TestBreathingScenario_FrameClockDeterminism(t *testing.T) {
	build := func() *BreathingScenarioState {
		st, err := NewBreathingScenarioState([]*Walker{newStationaryWalker()}, 0.2, 4)
		if err != nil {
			t.Fatalf("NewBreathingScenarioState failed: %v", err)
		}
		return st
	}
	a, b := build(), build()
	for i := 0; i < 400; i++ {
		a.Update(1.0 / 20.0)
		b.Update(1.0 / 20.0)
		if i == 200 {
			time.Sleep(25 * time.Millisecond) // wall-clock pause must not matter
		}
		if a.walkers[0].Position != b.walkers[0].Position {
			t.Fatalf("position diverged at frame %d despite identical frame clock", i)
		}
	}
}
