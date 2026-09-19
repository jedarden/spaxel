// Stationary-breathing scenario: the room's walkers stand perfectly still
// while their chest position oscillates vertically at the breathing rate
// with millimetre amplitude. The displacement is a scripted function of the
// CSI frame clock (no RNG, no wall clock), and it flows through the same
// two-ray propagation model as walking — generateCSIFrame turns the
// sub-wavelength position change into a CSI phase shift — so
// internal/signal's BreathingDetector is exercised unmodified. This is the
// simulator fixture for the C5/AS-10 stationary-person claim
// (docs/notes/localization-capability-acceptance-map.md).
//
// Displacement axis: the oscillation is vertical (chest rise). A standing
// walker (Z = 1.7) sits ~0.3 m below the default node height (Z = 2.0), so
// the extra path length stays inside the first Fresnel zone, where the
// walker-coupled direct ray dominates and the phase shift renders cleanly.
// A horizontal axis would not couple at all for walkers standing on a
// TX-RX chord: path length is stationary there to first order.
package main

import (
	"flag"
	"fmt"
	"math"
)

// Scenario flags for --scenario stationary. These live in this file rather
// than main.go's shared flag block so the scenario carries its own diff.
var (
	flagBreathingHz    = flag.Float64("breathing-hz", 0.2, "Breathing frequency in Hz for --scenario stationary (0.1-0.5)")
	flagBreathingAmpMM = flag.Float64("breathing-amp-mm", 4.0, "Chest displacement amplitude in mm for --scenario stationary (0-20; 0 disables the oscillation)")
)

// Breathing band (plan.md "Breathing band": 0.1-0.5 Hz, i.e. 6-30 BPM) and
// the supported chest displacement range.
const (
	breathingMinHz    = 0.1
	breathingMaxHz    = 0.5
	breathingMaxAmpMM = 20.0

	// goldenAngle separates the breathing phase of multiple stationary
	// walkers deterministically (no RNG) so N people do not breathe in
	// lockstep.
	goldenAngle = 2.399963229728653
)

// BreathingScenarioState drives the stationary-breathing scenario: it owns
// the walkers' vertical micro-motion while the scenario is active.
type BreathingScenarioState struct {
	walkers []*Walker
	base    []Point // position each walker was frozen at

	// RateHz is the breathing frequency in Hz (documented band 0.1-0.5).
	RateHz float64

	// AmpM is the chest displacement amplitude in meters (default 4 mm;
	// 0 disables the oscillation entirely).
	AmpM float64

	// elapsedSec accumulates simulated time (dt per frame) so the
	// oscillation phase follows the CSI frame clock, not wall-clock jitter.
	elapsedSec float64
}

// NewBreathingScenarioState freezes the given walkers at their current
// positions and prepares to oscillate each chest vertically at rateHz with
// ampMM millimetres of displacement. ampMM = 0 disables the oscillation,
// leaving the walkers exactly where they stand.
func NewBreathingScenarioState(walkers []*Walker, rateHz, ampMM float64) (*BreathingScenarioState, error) {
	if ampMM < 0 || ampMM > breathingMaxAmpMM {
		return nil, fmt.Errorf("breathing amplitude %.1f mm outside supported range [0, %.0f]", ampMM, breathingMaxAmpMM)
	}
	if ampMM > 0 && (rateHz < breathingMinHz || rateHz > breathingMaxHz) {
		return nil, fmt.Errorf("breathing rate %.2f Hz outside the documented %.1f-%.1f Hz band", rateHz, breathingMinHz, breathingMaxHz)
	}

	base := make([]Point, len(walkers))
	for i, w := range walkers {
		base[i] = w.Position
	}

	return &BreathingScenarioState{
		walkers: walkers,
		base:    base,
		RateHz:  rateHz,
		AmpM:    ampMM / 1000.0,
	}, nil
}

// chestDisplacement returns the vertical chest offset (meters) for walker
// index i at the scenario's current simulated time.
func (b *BreathingScenarioState) chestDisplacement(i int) float64 {
	return b.AmpM * math.Sin(2*math.Pi*b.RateHz*b.elapsedSec + float64(i)*goldenAngle)
}

// Update advances the scenario clock by dt seconds and repositions each
// walker's chest. Called once per CSI frame (dt = 1/flagRate) from
// runSimulation; with the oscillation disabled it is a no-op, so the
// walkers stay byte-for-byte where a still walker stands today.
func (b *BreathingScenarioState) Update(dt float64) {
	if b.AmpM == 0 {
		return
	}
	b.elapsedSec += dt
	for i, w := range b.walkers {
		w.Position.Z = b.base[i].Z + b.chestDisplacement(i)
	}
}
