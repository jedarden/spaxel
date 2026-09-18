// Package acceptance provides integration tests for Spaxel acceptance scenarios.
// AS-8: Deterministic 2D-position accuracy (localization map C1, guard N1).
//
// Design authority: docs/notes/localization-capability-acceptance-map.md §4 (C1)
// and §5 (N1). Fixture per the map: live mothership (startMothership) +
// spaxel-sim --nodes 4 --walker-type path --path-file <scripted loop> --seed 42
// --space 6x5x2.5 --duration 60, with --output-csv capturing the walker's
// ground-truth trajectory (map §6.4: ground truth by construction, never
// inferred from the system under test).
//
// Gate (README L14 upper bound): median horizontal (XY) error of tracked blobs
// vs. the CSV ground truth ≤ 1.0 m. The 0.5 m end of the L14 band is a
// non-gating target and is not asserted. RecallAt1m / RecallAt2m are logged as
// diagnostics, mirroring simulator.AccuracyReport's definitions.
//
// N1 guard (README L20 negative claim): the Fresnel grid cell from
// /api/settings ("grid_cell_m", default 0.2 m) must be ≥ 0.10 m, and no
// assertion in this file pins position error below 0.10 m — the suite must
// never demand the sub-10 cm accuracy the README explicitly disclaims.
//
// A measured FAIL of the 1.0 m gate (median/p90 in the test log) is a valid
// outcome of this scenario: the deliverable is the deterministic fixture plus
// the honest measurement, not a green run. Do not loosen the gate to pass.
package acceptance

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// AS-8 fixture constants. The walker follows a scripted rectangular loop
// (14 m perimeter) at the path walker's fixed 1.0 m/s, so the 60 s run covers
// just over four laps and the post-warmup window observes the full loop
// repeatedly. Determinism: fixed seed, scripted polyline, and the simulator's
// fixed default noise sigma (--noise-sigma deliberately not overridden).
const (
	as8Seed      = 42
	as8Nodes     = 4
	as8Space     = "6x5x2.5"
	as8DurationS = 60
	as8RateHz    = 20

	// Post-warmup window: polls in the first 25 s after spawn are excluded
	// (20 s documented baseline warmup for the EMA/fusion machinery plus
	// spawn-and-connect slack). Only polls after this mark feed the gate.
	as8Warmup = 25 * time.Second

	// Gate: README L14 upper bound on approximate 2D position accuracy.
	as8MedianErrorGateM = 1.0

	// N1 resolution floor: nothing in the suite may assert accuracy better
	// than this, and the deployed grid cell must not be finer.
	as8GridCellFloorM = 0.10
)

// AS8_2DPositionAccuracyIntegration runs the deterministic AS-8 scenario end to end.
func AS8_2DPositionAccuracyIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	mothershipURL := getMothershipURL()
	cmd := startMothership(t, getTempDBPath())
	defer stopMothership(cmd)

	if !waitForMothership(ctx, mothershipURL) {
		t.Fatal("Mothership did not become ready")
	}
	setPIN(t, mothershipURL, "1234")

	// N1 guard before any accuracy sampling: a deployment configured with a
	// sub-10 cm grid cell invalidates the run's premise (README L20).
	as8AssertGridCellFloor(t, mothershipURL)

	dir := t.TempDir()
	pathFile := filepath.Join(dir, "as8-scripted-loop.json")
	gtCSV := filepath.Join(dir, "as8-ground-truth.csv")
	if err := os.WriteFile(pathFile, []byte(as8ScriptedLoopJSON), 0o644); err != nil {
		t.Fatalf("Failed to write scripted path file: %v", err)
	}

	simStart := time.Now()
	simCtx, cancelSim := context.WithTimeout(ctx, 2*time.Minute)
	simCmd := startSimulator(t, simCtx, []string{
		"--mothership", wsURL(mothershipURL),
		"--nodes", fmt.Sprintf("%d", as8Nodes),
		"--walkers", "1",
		"--walker-type", "path",
		"--path-file", pathFile,
		"--seed", fmt.Sprintf("%d", as8Seed),
		"--space", as8Space,
		"--rate", fmt.Sprintf("%d", as8RateHz),
		"--duration", fmt.Sprintf("%d", as8DurationS),
		"--output-csv", gtCSV,
	})
	defer cancelSim()
	defer stopSimulator(simCmd)

	// Poll /api/blobs once a second across the whole run window. Blobs decay
	// once CSI stops, so polls past the sim's 60 s duration may legitimately
	// be empty; they still count toward the post-warmup detection-ratio
	// diagnostic but contribute no blob error samples.
	pollDeadline := simStart.Add(as8DurationS*time.Second + 10*time.Second)
	var samples []as8BlobSample
	var blobsPerPoll []int
	polls := 0
	for time.Now().Before(pollDeadline) {
		if ctx.Err() != nil {
			break
		}
		elapsed := time.Since(simStart)
		if elapsed < as8Warmup {
			time.Sleep(1 * time.Second)
			continue
		}
		polls++
		var pollBlobs []as8BlobSample
		for _, blob := range as8GetBlobs(t, mothershipURL) {
			bx, ok := as8BlobCoord(blob, "X", "x")
			if !ok {
				continue
			}
			by, ok := as8BlobCoord(blob, "Y", "y")
			if !ok {
				continue
			}
			pollBlobs = append(pollBlobs, as8BlobSample{elapsed: elapsed, x: bx, y: by})
		}
		blobsPerPoll = append(blobsPerPoll, len(pollBlobs))
		samples = append(samples, pollBlobs...)
		time.Sleep(1 * time.Second)
	}

	gt, err := as8ParseGroundTruthCSV(gtCSV)
	if err != nil {
		t.Fatalf("Failed to parse ground-truth CSV: %v", err)
	}
	if len(gt) == 0 {
		t.Fatal("Ground-truth CSV contains no walker positions — sim wrote no ground truth")
	}
	t.Logf("Ground truth: %d walker positions from %s", len(gt), filepath.Base(gtCSV))

	pollsWithBlob := 0
	for _, c := range blobsPerPoll {
		if c > 0 {
			pollsWithBlob++
		}
	}
	t.Logf("Post-warmup window: %d polls, detection ratio %.1f%%, blobs/poll min %d max %d",
		polls, 100*float64(pollsWithBlob)/float64(max(polls, 1)),
		as8MinInt(blobsPerPoll), as8MaxInt(blobsPerPoll))

	if len(samples) == 0 {
		t.Fatalf("No tracked-blob samples after %.0f s warmup across %d polls — "+
			"the pipeline localized nothing from the scripted walker", as8Warmup.Seconds(), polls)
	}

	// Gate metric: per blob sample, the horizontal (XY) distance to the nearest
	// ground-truth position — the map's "median horizontal (XY) error of
	// tracked blobs vs. ground-truth CSV", computed time-free against the
	// CSV-sampled trajectory (the loop repeats every 14 s, so the post-warmup
	// window covers the full loop ~3×; trajectory distance is the spatial
	// accuracy claim of README L14, while pipeline latency is a separate
	// property asserted by AS-2-ext, not here).
	errors := make([]float64, 0, len(samples))
	for _, s := range samples {
		errors = append(errors, as8MinHorizontalDist(s.x, s.y, gt))
	}

	// Engine-style diagnostic (simulator.AccuracyEstimator.Compute direction):
	// per ground-truth position, distance to the nearest observed blob.
	gtErrors := make([]float64, 0, len(gt))
	for _, g := range gt {
		gtErrors = append(gtErrors, as8MinHorizontalDistToSamples(g.x, g.y, samples))
	}

	sort.Float64s(errors)
	median := as8Percentile(errors, 0.5)
	p90 := as8Percentile(errors, 0.90)
	recall1m, recall2m := 0.0, 0.0
	for _, e := range errors {
		if e <= 1.0 {
			recall1m++
		}
		if e <= 2.0 {
			recall2m++
		}
	}
	recall1m /= float64(len(errors))
	recall2m /= float64(len(errors))

	t.Logf("AS-8 measurement: %d blob samples — median XY error %.3f m (gate ≤ %.1f m), "+
		"p90 %.3f m, RecallAt1m %.1f%%, RecallAt2m %.1f%%",
		len(errors), median, as8MedianErrorGateM, p90, 100*recall1m, 100*recall2m)
	t.Logf("AS-8 diagnostic (engine-style, per ground-truth position): median distance "+
		"to nearest blob %.3f m, p90 %.3f m",
		as8Percentile(gtErrors, 0.5), as8Percentile(gtErrors, 0.90))

	if median > as8MedianErrorGateM {
		t.Errorf("Median XY error %.3f m exceeds the README L14 gate of %.1f m "+
			"(p90 %.3f m, RecallAt1m %.1f%%, RecallAt2m %.1f%% over %d blob samples)",
			median, as8MedianErrorGateM, p90, 100*recall1m, 100*recall2m, len(errors))
	}
}

// as8ScriptedLoopJSON is the scripted rectangular loop the path walker follows:
// (1,1) → (5,1) → (5,4) → (1,4) inside the 6x5 m room, all at Z 1.7 m.
// --path-file schema: [{"waypoints": [{x,y,z}, ...]}] (cmd/sim PathDefinition).
const as8ScriptedLoopJSON = `[
  {"waypoints": [
    {"x": 1.0, "y": 1.0, "z": 1.7},
    {"x": 5.0, "y": 1.0, "z": 1.7},
    {"x": 5.0, "y": 4.0, "z": 1.7},
    {"x": 1.0, "y": 4.0, "z": 1.7}
  ]}
]`

// as8BlobSample is one tracked-blob observation from /api/blobs.
type as8BlobSample struct {
	elapsed time.Duration
	x, y    float64
}

// as8GTPosition is one ground-truth walker position from the sim's CSV output.
type as8GTPosition struct {
	x, y float64
}

// as8GetBlobs fetches the current tracked blobs. The live /api/blobs endpoint
// serializes pm.GetTrackedBlobs() (internal/signal TrackedBlob) as a BARE JSON
// array with Go-default capitalized keys — TrackedBlob carries no json tags on
// ID/X/Y/Z — unlike both the {"blobs": [...]} envelope the shared acceptance
// helpers decode and the lowercase keys the as2 mock serves. This helper
// decodes the array the server actually sends.
func as8GetBlobs(t testingT, baseURL string) []map[string]interface{} {
	t.Helper()

	resp, err := http.Get(baseURL + "/api/blobs")
	if err != nil {
		t.Logf("Failed to get blobs: %v", err)
		return nil
	}
	defer resp.Body.Close()

	var blobs []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&blobs); err != nil {
		t.Logf("Failed to decode blobs: %v", err)
		return nil
	}
	return blobs
}

// as8BlobCoord reads a blob coordinate by its Go-default capitalized key,
// falling back to the lowercase alias. The live REST contract (TrackedBlob
// without json tags) is capitalized; the fallback keeps the measurement honest
// if a tagged lowercase alias is added later.
func as8BlobCoord(blob map[string]interface{}, keys ...string) (float64, bool) {
	for _, k := range keys {
		if v, ok := blob[k].(float64); ok {
			return v, true
		}
	}
	return 0, false
}

// as8AssertGridCellFloor implements the N1 guard: the deployed Fresnel grid
// cell ("grid_cell_m" in /api/settings, SPAXEL_GRID_CELL_M, default 0.2 m)
// must be at least the 0.10 m resolution floor the README disclaims below.
func as8AssertGridCellFloor(t *testing.T, baseURL string) {
	t.Helper()

	resp, err := http.Get(baseURL + "/api/settings")
	if err != nil {
		t.Fatalf("N1 guard: failed to read /api/settings: %v", err)
	}
	defer resp.Body.Close()

	var settings map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&settings); err != nil {
		t.Fatalf("N1 guard: failed to decode /api/settings: %v", err)
	}
	raw, ok := settings["grid_cell_m"]
	if !ok {
		t.Fatal("N1 guard: /api/settings response has no grid_cell_m key")
	}
	cell, ok := raw.(float64)
	if !ok {
		t.Fatalf("N1 guard: grid_cell_m is not a number: %v", raw)
	}
	t.Logf("N1 guard: grid_cell_m = %.3f m (floor %.2f m)", cell, as8GridCellFloorM)
	if cell < as8GridCellFloorM {
		t.Fatalf("N1 guard violated: grid cell %.3f m is below the %.2f m resolution "+
			"floor the README disclaims (README L20)", cell, as8GridCellFloorM)
	}
}

// as8ParseGroundTruthCSV reads walker positions from a spaxel-sim --output-csv
// file. Position rows carry the walker x/y in columns 2/3; link rows repeat the
// position but carry a link_id and delta_rms and are skipped.
func as8ParseGroundTruthCSV(path string) ([]as8GTPosition, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 1 {
		return nil, fmt.Errorf("CSV has no header row")
	}
	const wantCols = 10 // timestamp_ms, walker_id, x, y, z, vx, vy, vz, link_id, delta_rms

	gt := make([]as8GTPosition, 0, len(rows))
	for _, row := range rows[1:] {
		if len(row) != wantCols {
			return nil, fmt.Errorf("CSV row has %d columns, want %d: %v", len(row), wantCols, row)
		}
		if row[8] != "" { // link_id set → per-link deltaRMS row, not a position row
			continue
		}
		var p as8GTPosition
		if _, err := fmt.Sscanf(row[2], "%f", &p.x); err != nil {
			return nil, fmt.Errorf("CSV x column %q: %w", row[2], err)
		}
		if _, err := fmt.Sscanf(row[3], "%f", &p.y); err != nil {
			return nil, fmt.Errorf("CSV y column %q: %w", row[3], err)
		}
		gt = append(gt, p)
	}
	return gt, nil
}

// as8MinHorizontalDist returns the smallest XY-plane distance from (x, y) to
// any ground-truth position.
func as8MinHorizontalDist(x, y float64, gt []as8GTPosition) float64 {
	minDist := math.Inf(1)
	for _, g := range gt {
		if d := math.Hypot(x-g.x, y-g.y); d < minDist {
			minDist = d
		}
	}
	return minDist
}

// as8MinHorizontalDistToSamples returns the smallest XY-plane distance from
// (x, y) to any observed blob sample.
func as8MinHorizontalDistToSamples(x, y float64, samples []as8BlobSample) float64 {
	minDist := math.Inf(1)
	for _, s := range samples {
		if d := math.Hypot(x-s.x, y-s.y); d < minDist {
			minDist = d
		}
	}
	return minDist
}

// as8Percentile returns the p-quantile (0 ≤ p ≤ 1) of a sorted slice by
// nearest-rank indexing, mirroring simulator.AccuracyReport's percentile
// convention of indexing into the sorted error list.
func as8Percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return math.Inf(1)
	}
	return sorted[int(p*float64(len(sorted)-1))]
}

// as8MinInt / as8MaxInt return the min/max of a non-empty-ish int slice
// (0 for an empty one).
func as8MinInt(vals []int) int {
	if len(vals) == 0 {
		return 0
	}
	m := vals[0]
	for _, v := range vals {
		if v < m {
			m = v
		}
	}
	return m
}

func as8MaxInt(vals []int) int {
	if len(vals) == 0 {
		return 0
	}
	m := vals[0]
	for _, v := range vals {
		if v > m {
			m = v
		}
	}
	return m
}
