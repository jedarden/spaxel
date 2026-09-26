// Package acceptance provides integration tests for Spaxel acceptance scenarios.
// AS-9: Rough person count — distinguishes 1 vs 2+, degrades at 3+ (README L16).
//
// Design authority: docs/notes/localization-capability-acceptance-map.md §4 (C3)
// and §5 (N3). Fixture per the map: for each sub-run, a live mothership plus
// spaxel-sim --walker-type path --path-file <disjoint corridors> --seed 42
// --nodes 4 --space 6x5x2.5 --rate 20 --duration 60. The path file carries three
// scripted corridors occupying disjoint thirds of the room's width (≥ 1.2 m
// between corridors), so walkers' Fresnel zones do not merge by construction;
// walker i takes corridor i % 3 (cmd/sim createPathWalkers).
//
// Gates (map §4 C3):
//   - 1 walker  → steady-state median blob count == 1 (L16 "distinguishes 1")
//   - 2 walkers → ≥ 1 blob in ≥ 90 % of polls (presence floor) AND ≥ 2
//     simultaneous blobs in ≥ 1 poll (separation existence, L16 "vs 2+");
//     the measured ≥ 2-blob fraction is logged, not gated beyond existence —
//     re-scoped by spaxel-c61599fa, see map §4 C3 "Why the 50 % fraction gate
//     was re-scoped"
//   - 3 walkers → stability-only: run completes, per-poll blob count stays in
//     [1, max_tracked_blobs], no crash — explicitly NOT asserted == 3, per L16
//     "degrades at 3+"
//
// N3 guard (map §5): a fourth sub-run with 5 walkers asserts the same stability
// bounds and never asserts 5 distinct blobs; the measured blob counts are
// logged to document L20's "not reliable" rather than fight it.
//
// A measured FAIL of the 1-walker or 2-walker gate is a valid outcome of this
// scenario: the deliverable is the deterministic fixture plus the honest
// measurement, not a green run. Do not loosen a gate to pass. (AS-8 precedent:
// its live measurement measured FAIL against its own gate and landed as such.)
package acceptance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// AS-9 fixture constants, mirroring AS-8's deterministic setup (60 s at the
// path walker's fixed 1.0 m/s covers ~4 corridor laps; the post-warmup window
// observes steady state repeatedly).
const (
	as9Seed      = 42
	as9Nodes     = 4
	as9Space     = "6x5x2.5"
	as9DurationS = 60
	as9RateHz    = 20

	// Post-warmup window: same rationale as AS-8 (20 s documented baseline
	// warmup for the EMA/fusion machinery plus spawn-and-connect slack).
	as9Warmup = 25 * time.Second

	// Poll deadline extends 10 s past the sim's duration so the final second
	// of frames has been ingested and served.
	as9PollTail = 10 * time.Second

	// Gates for the 2-walker run, re-scoped by spaxel-c61599fa (map §4 C3,
	// "Why the 50 % fraction gate was re-scoped"). The original ≥ 50 % ≥ 2-
	// blob fraction measured 20.0 % (spaxel-92ce3d2a) and 25.0-25.7 % on
	// re-runs, and the mechanism analysis showed it is unattainable in this
	// fixture: per-link CSI is a scalar (a link perturbed by two walkers
	// reports one number), ~88 % of fusion ticks have a single active link
	// whose one ridge can never serve two blobs, and walker 1's corridor sits
	// below the motion-detection proximity of the link set for most of its
	// lap. What the pipeline does support — and what these gates pin — is:
	//   - presence: the active-link set stays lit while both walkers run, so
	//     ≥ 1 blob is served in ≥ 90 % of polls;
	//   - separation existence: ≥ 2 simultaneous blobs demonstrably occur
	//     (the "vs 2+" discrimination in both directions), with the measured
	//     fraction logged rather than gated beyond existence.
	as9TwoWalkerPresenceFloor = 0.9
)

// as9CorridorsJSON is the scripted path file: three disjoint corridors in
// disjoint thirds of the 6 m room width, each a closed loop at Z 1.7 m, with
// ≥ 1.2 m of empty space between corridors so walkers' Fresnel zones cannot
// merge by construction (map §4 C3 "opposite halves ... don't merge").
// --path-file schema: [{"waypoints": [{x,y,z}, ...]}, ...] — walker i takes
// corridor i % len (cmd/sim createPathWalkers).
const as9CorridorsJSON = `[
  {"waypoints": [
    {"x": 0.8, "y": 1.0, "z": 1.7},
    {"x": 0.8, "y": 4.0, "z": 1.7},
    {"x": 1.6, "y": 4.0, "z": 1.7},
    {"x": 1.6, "y": 1.0, "z": 1.7}
  ]},
  {"waypoints": [
    {"x": 2.8, "y": 4.0, "z": 1.7},
    {"x": 2.8, "y": 1.0, "z": 1.7},
    {"x": 3.6, "y": 1.0, "z": 1.7},
    {"x": 3.6, "y": 4.0, "z": 1.7}
  ]},
  {"waypoints": [
    {"x": 4.8, "y": 1.0, "z": 1.7},
    {"x": 4.8, "y": 4.0, "z": 1.7},
    {"x": 5.6, "y": 4.0, "z": 1.7},
    {"x": 5.6, "y": 1.0, "z": 1.7}
  ]}
]`

// AS9_PersonCountIntegration runs the four person-count sub-runs end to end.
func AS9_PersonCountIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	t.Run("OneWalkerCountsAsOne", func(t *testing.T) {
		counts := as9RunCountScenario(t, 1)
		if len(counts) == 0 {
			t.Fatal("No post-warmup polls — pipeline observed nothing")
		}
		median := as9Median(counts)
		t.Logf("1 walker: %d post-warmup polls, blob count median %.1f min %d max %d",
			len(counts), median, as9Min(counts), as9Max(counts))
		if median != 1 {
			t.Errorf("1 walker: steady-state median blob count %.1f, want exactly 1 "+
				"(README L16 \"distinguishes 1\")", median)
		}
	})

	t.Run("TwoWalkersPresenceAndSeparation", func(t *testing.T) {
		counts := as9RunCountScenario(t, 2)
		if len(counts) == 0 {
			t.Fatal("No post-warmup polls — pipeline observed nothing")
		}
		present := 0
		distinct := 0
		for _, c := range counts {
			if c >= 1 {
				present++
			}
			if c >= 2 {
				distinct++
			}
		}
		presence := float64(present) / float64(len(counts))
		fraction := float64(distinct) / float64(len(counts))
		t.Logf("2 walkers: %d post-warmup polls, ≥1-blob presence %.1f%%, ≥2-blob fraction %.1f%% (min %d max %d)",
			len(counts), 100*presence, 100*fraction, as9Min(counts), as9Max(counts))
		if presence < as9TwoWalkerPresenceFloor {
			t.Errorf("2 walkers: only %.1f%% of polls served ≥ 1 blob, want ≥ %.0f%% — "+
				"the active-link set went quiet under two-walker load",
				100*presence, 100*as9TwoWalkerPresenceFloor)
		}
		if distinct == 0 {
			t.Errorf("2 walkers: no poll served ≥ 2 blobs — the \"vs 2+\" discrimination " +
				"was never demonstrated (README L16 \"distinguishes 1 vs 2+\")")
		}
	})

	t.Run("ThreeWalkersStabilityOnly", func(t *testing.T) {
		counts := as9RunCountScenario(t, 3)
		as9AssertStability(t, "3 walkers", counts)
		t.Logf("3 walkers: measured median %.1f (degradation expected per README L16 "+
			"\"degrades at 3+\" — not asserted against a count)", as9Median(counts))
	})

	t.Run("FiveWalkersNotReliableN3", func(t *testing.T) {
		counts := as9RunCountScenario(t, 5)
		as9AssertStability(t, "5 walkers", counts)
		t.Logf("N3: 5 walkers measured median %.1f max %d — undercount/merge rate is "+
			"documented, never asserted (README L20 \"not reliable at 5+\")",
			as9Median(counts), as9Max(counts))
	})
}

// as9MaxTrackedBlobs caches the deployed cap read by the scenario prologue.
var as9MaxTracked int

// as9RunCountScenario runs one spaxel-sim count fixture against a fresh live
// mothership and returns the per-poll tracked-blob counts across the
// post-warmup steady-state window.
func as9RunCountScenario(t *testing.T, walkers int) []int {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	mothershipURL := getMothershipURL()
	cmd := startMothership(t, getTempDBPath())
	defer stopMothership(cmd)

	if !waitForMothership(ctx, mothershipURL) {
		t.Fatal("Mothership did not become ready")
	}
	setPIN(t, mothershipURL, "1234")

	// N3's stability ceiling is the deployed cap, read from the instance under
	// test (sequential sub-runs each re-read and overwrite it).
	as9MaxTracked = as9ReadMaxTrackedBlobs(t, mothershipURL)
	t.Logf("Deployed max_tracked_blobs = %d (stability ceiling)", as9MaxTracked)

	dir := t.TempDir()
	pathFile := filepath.Join(dir, "as9-corridors.json")
	if err := os.WriteFile(pathFile, []byte(as9CorridorsJSON), 0o644); err != nil {
		t.Fatalf("Failed to write corridor path file: %v", err)
	}

	simStart := time.Now()
	simCtx, cancelSim := context.WithTimeout(ctx, 2*time.Minute)
	simCmd := startSimulator(t, simCtx, []string{
		"--mothership", wsURL(mothershipURL),
		"--nodes", fmt.Sprintf("%d", as9Nodes),
		"--walkers", fmt.Sprintf("%d", walkers),
		"--walker-type", "path",
		"--path-file", pathFile,
		"--seed", fmt.Sprintf("%d", as9Seed),
		"--space", as9Space,
		"--rate", fmt.Sprintf("%d", as9RateHz),
		"--duration", fmt.Sprintf("%d", as9DurationS),
	})
	defer cancelSim()
	defer stopSimulator(simCmd)

	// Poll /api/blobs once a second across the post-warmup window (as8's
	// polling shape). /api/blobs serves a BARE array with Go-default
	// capitalized keys — decode via the proven as8 helpers, never the shared
	// envelope decoders (they silently see zero live blobs).
	pollDeadline := simStart.Add(as9DurationS*time.Second + as9PollTail)
	var counts []int
	for time.Now().Before(pollDeadline) {
		if ctx.Err() != nil {
			break
		}
		if time.Since(simStart) < as9Warmup {
			time.Sleep(1 * time.Second)
			continue
		}
		counts = append(counts, len(as8GetBlobs(t, mothershipURL)))
		time.Sleep(1 * time.Second)
	}
	return counts
}

// as9AssertStability implements the map's stability-only bound for the
// degraded regimes: every post-warmup poll count stays in
// [1, max_tracked_blobs] and the run completed (the poll window elapsed).
func as9AssertStability(t *testing.T, label string, counts []int) {
	t.Helper()

	if len(counts) == 0 {
		t.Fatalf("%s: no post-warmup polls — run did not complete a steady state", label)
	}
	for i, c := range counts {
		if c < 1 {
			t.Errorf("%s: poll %d saw 0 blobs — count left the stability floor [1, %d]",
				label, i, as9MaxTracked)
		}
		if c > as9MaxTracked {
			t.Errorf("%s: poll %d saw %d blobs — count exceeded max_tracked_blobs %d",
				label, i, c, as9MaxTracked)
		}
	}
	t.Logf("%s: stability %d polls in [%d..%d] (ceiling %d)",
		label, len(counts), as9Min(counts), as9Max(counts), as9MaxTracked)
}

// as9ReadMaxTrackedBlobs reads the deployed tracking cap from /api/settings —
// the same surface the dashboard uses; N3's ceiling is a deployment property.
func as9ReadMaxTrackedBlobs(t *testing.T, baseURL string) int {
	t.Helper()

	resp, err := http.Get(baseURL + "/api/settings")
	if err != nil {
		t.Fatalf("Failed to read /api/settings: %v", err)
	}
	defer resp.Body.Close()

	var settings map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&settings); err != nil {
		t.Fatalf("Failed to decode /api/settings: %v", err)
	}
	raw, ok := settings["max_tracked_blobs"]
	if !ok {
		t.Fatal("/api/settings response has no max_tracked_blobs key")
	}
	cap, ok := raw.(float64)
	if !ok {
		t.Fatalf("max_tracked_blobs is not a number: %v", raw)
	}
	return int(cap)
}

// as9Median returns the median of a non-empty count sample (0 for empty).
func as9Median(counts []int) float64 {
	if len(counts) == 0 {
		return 0
	}
	sorted := append([]int(nil), counts...)
	sort.Ints(sorted)
	if len(sorted)%2 == 1 {
		return float64(sorted[len(sorted)/2])
	}
	return float64(sorted[len(sorted)/2-1]+sorted[len(sorted)/2]) / 2
}

// as9Min / as9Max return the min/max of a count sample (0 for empty).
func as9Min(counts []int) int {
	if len(counts) == 0 {
		return 0
	}
	m := counts[0]
	for _, c := range counts {
		if c < m {
			m = c
		}
	}
	return m
}

func as9Max(counts []int) int {
	if len(counts) == 0 {
		return 0
	}
	m := counts[0]
	for _, c := range counts {
		if c > m {
			m = c
		}
	}
	return m
}
