// Package falldetect provides fall detection using blob tracking data.
package falldetect

import (
	"testing"
	"time"
)

// fallBlobInput mirrors the anonymous struct accepted by Detector.Update so the
// test can build tick inputs concisely.
type fallBlobInput struct {
	ID         int
	X, Y, Z    float64
	VX, VY, VZ float64
	Posture    string
}

// updateBlob aliases the anonymous struct Detector.Update expects, so the helper
// slice converter can return a concrete type without repeating the field list.
type updateBlob = struct {
	ID         int
	X, Y, Z    float64
	VX, VY, VZ float64
	Posture    string
}

// toUpdate converts the helper to the anonymous struct Detector.Update expects.
func (b fallBlobInput) toUpdate() updateBlob {
	return updateBlob{
		ID: b.ID, X: b.X, Y: b.Y, Z: b.Z,
		VX: b.VX, VY: b.VY, VZ: b.VZ, Posture: b.Posture,
	}
}

// fallBlobsToUpdate converts a slice of test helpers into the []updateBlob
// Detector.Update expects. It is a free function rather than a method because Go
// does not allow methods on unnamed slice types ([]fallBlobInput).
func fallBlobsToUpdate(s []fallBlobInput) []updateBlob {
	out := make([]updateBlob, len(s))
	for i, b := range s {
		out[i] = b.toUpdate()
	}
	return out
}

// identityFieldsConfig is a fall-detection Config tuned for a fast, deterministic
// in-test fall: a tiny stillness-confirmation window so the state machine reaches
// StateFallConfirmed within a handful of synthetic ticks (no wall-clock waiting).
func identityFieldsConfig() Config {
	return Config{
		DescentVelocityThreshold: -1.5,
		DescentDropThreshold:     0.8,
		DescentWindow:            0.2,
		FloorLevelThreshold:      0.4,
		StillnessMotionThreshold: 0.01,
		StillnessTimeRequired:    0.5,
		StandardDropThreshold:    0.8,
		ElevatedDropThreshold:    1.2,
		PostureHistoryWindow:     600,
		EscalationTime1:          2 * time.Minute,
		EscalationTime2:          5 * time.Minute,
		AlertCooldown:            5 * time.Minute,
	}
}

// driveFall feeds a synthetic blob through a complete fall: a standing phase to
// build history, a rapid descent that clears the trigger, then a still floor-level
// phase that confirms the fall and fires triggerFallAlert (which calls identityFunc).
// `now` is advanced synthetically so the test is deterministic and clock-independent.
func driveFall(t *testing.T, d *Detector, base time.Time) {
	t.Helper()
	blob := fallBlobInput{ID: 7, X: 3, Y: 2, Posture: "standing"}

	// Standing phase: build ≥3 samples of history at Z=1.7m so the descent tick
	// can compute a >1s lookback z-drop.
	for _, off := range []time.Duration{0, 500 * time.Millisecond, 1 * time.Second} {
		blob.Z = 1.7
		d.Update(fallBlobsToUpdate([]fallBlobInput{blob}), base.Add(off))
	}

	// Descent: Z drops 1.7→0.3 in one tick with VZ=-2.0 (clears velocity + drop
	// thresholds) → StateDescentDetected.
	blob.Z, blob.VZ = 0.3, -2.0
	d.Update(fallBlobsToUpdate([]fallBlobInput{blob}), base.Add(1100*time.Millisecond))

	// Floor-level stillness: Z stays 0.3 (< floor), deltaRMS stays below the
	// stillness threshold for ≥ StillnessTimeRequired → StateFallConfirmed →
	// triggerFallAlert → identityFunc(7) called.
	blob.VZ = 0
	for off := 1200; off <= 1700; off += 100 {
		d.Update(fallBlobsToUpdate([]fallBlobInput{blob}), base.Add(time.Duration(off)*time.Millisecond))
	}
}

// TestDetector_FallEventCarriesResolvedIdentity verifies the falldetect
// projection observes a NON-EMPTY identity at runtime: when a fall is confirmed,
// triggerFallAlert calls identityFunc(blobID) and writes its result onto
// FallEvent.Identity. At runtime (cmd/mothership) identityFunc returns
// matcher.GetMatch(blobID).Label() — the canonical PersonName — so this wires
// the same value and asserts the projection surfaces it. (bf-2v9g)
func TestDetector_FallEventCarriesResolvedIdentity(t *testing.T) {
	t.Parallel()
	d := NewDetectorWithConfig(identityFieldsConfig())
	d.SetDeltaRMSFunc(func(int) float64 { return 0.005 }) // below stillness threshold

	// Mirror the runtime identityFunc: return the canonical person name (Label()).
	d.SetIdentityFunc(func(blobID int) string {
		if blobID != 7 {
			return ""
		}
		return "Alice" // stand-in for matcher.GetMatch(7).Label()
	})

	got := make(chan FallEvent, 1)
	d.SetOnFall(func(e FallEvent) { got <- e })

	driveFall(t, d, time.Now())

	select {
	case e := <-got:
		if e.Identity != "Alice" {
			t.Fatalf("falldetect projection observed empty identity: FallEvent.Identity=%q, want %q", e.Identity, "Alice")
		}
		if e.BlobID != 7 {
			t.Fatalf("FallEvent.BlobID=%d, want 7", e.BlobID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("onFall never fired: fall not confirmed within timeout")
	}
}

// TestDetector_FallEventNilSafeWithoutIdentityFunc verifies the falldetect
// projection never dereferences an UNSET identity field at runtime: with
// identityFunc left nil (no matcher / no resolved person), a confirmed fall still
// fires without panic and FallEvent.Identity stays empty — the
// "(where applicable)" / unidentified case. (bf-2v9g)
func TestDetector_FallEventNilSafeWithoutIdentityFunc(t *testing.T) {
	t.Parallel()
	d := NewDetectorWithConfig(identityFieldsConfig()) // identityFunc intentionally nil
	d.SetDeltaRMSFunc(func(int) float64 { return 0.005 })

	got := make(chan FallEvent, 1)
	d.SetOnFall(func(e FallEvent) { got <- e })

	driveFall(t, d, time.Now())

	select {
	case e := <-got:
		// No panic, identity gracefully empty — the unidentified-intruder path.
		if e.Identity != "" {
			t.Fatalf("FallEvent.Identity=%q, want empty when identityFunc is unset", e.Identity)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("onFall never fired: fall not confirmed within timeout")
	}
}

// TestDetector_DefaultConfigStillnessOutlivesFallWindow pins the fall-window /
// stillness interaction at DEFAULT config, which the runtime (cmd/mothership
// falldetect.NewDetector) uses: StillnessTimeRequired (30s) is longer than the
// 10s fall-completion window, so the window must only expire a confirmation
// whose stillness never began. Before the stillnessStarted guard, the window
// reset always fired first and a default-config fall alert was unreachable on
// every path (unit and composed); the only passing confirmation tests overrode
// StillnessTimeRequired to sub-second values.
func TestDetector_DefaultConfigStillnessOutlivesFallWindow(t *testing.T) {
	t.Parallel()

	t.Run("StillnessBegunContinuesPastWindow", func(t *testing.T) {
		d := NewDetector() // runtime defaults: stillness 30s vs window 10s
		got := make(chan FallEvent, 1)
		d.SetOnFall(func(e FallEvent) { got <- e })

		// Same synthetic shape as driveFall but at default config: descent at
		// base+1.1s, floor stillness from base+1.2s, confirmation expected at
		// base+31.2s (descent + 30s stillness). Ticks at 10Hz like the runtime.
		base := time.Now()
		blob := fallBlobInput{ID: 9, X: 3, Y: 2, Posture: "standing"}
		for _, off := range []time.Duration{0, 500 * time.Millisecond, 1 * time.Second} {
			blob.Z = 1.7
			d.Update(fallBlobsToUpdate([]fallBlobInput{blob}), base.Add(off))
		}
		blob.Z, blob.VZ = 0.3, -2.0
		d.Update(fallBlobsToUpdate([]fallBlobInput{blob}), base.Add(1100*time.Millisecond))
		blob.VZ = 0
		for off := 1200; off <= 32000; off += 100 {
			d.Update(fallBlobsToUpdate([]fallBlobInput{blob}), base.Add(time.Duration(off)*time.Millisecond))
			select {
			case e := <-got:
				if e.BlobID != 9 {
					t.Fatalf("FallEvent.BlobID=%d, want 9", e.BlobID)
				}
				if e.StillnessTime < 30.0 {
					t.Fatalf("FallEvent.StillnessTime=%.1fs, want >= 30s (confirmed too early)", e.StillnessTime)
				}
				return
			default:
			}
		}
		t.Fatal("onFall never fired: default-config stillness (30s) did not outlive the 10s fall window")
	})

	t.Run("NeverStillStillExpiresWindow", func(t *testing.T) {
		d := NewDetector()
		got := make(chan FallEvent, 1)
		d.SetOnFall(func(e FallEvent) { got <- e })
		d.SetDeltaRMSFunc(func(int) float64 { return 0.5 }) // never still

		// Descent into floor level followed by continued motion: the window
		// must still reset the state (the guard's false-positive purpose) —
		// no alert at any point afterwards.
		base := time.Now()
		blob := fallBlobInput{ID: 9, X: 3, Y: 2, Posture: "standing"}
		for _, off := range []time.Duration{0, 500 * time.Millisecond, 1 * time.Second} {
			blob.Z = 1.7
			d.Update(fallBlobsToUpdate([]fallBlobInput{blob}), base.Add(off))
		}
		blob.Z, blob.VZ = 0.3, -2.0
		d.Update(fallBlobsToUpdate([]fallBlobInput{blob}), base.Add(1100*time.Millisecond))
		blob.VZ = 0
		for off := 1200; off <= 40000; off += 100 {
			d.Update(fallBlobsToUpdate([]fallBlobInput{blob}), base.Add(time.Duration(off)*time.Millisecond))
			select {
			case <-got:
				t.Fatal("onFall fired for a never-still descent: window expiry lost")
			default:
			}
		}
	})
}
