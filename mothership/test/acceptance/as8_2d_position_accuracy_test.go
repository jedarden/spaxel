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
// vs. the CSV ground truth ≤ 1.5 m. The L14 band was re-based from the original
// ±0.5–1.0 m estimate to ±1.0–1.5 m by spaxel-1a2c7859: the recorded seed-42
// runs measured medians of 1.140/1.273 m with p90 1.474 m, so the 1.0 m figure
// encoded an aspiration, not delivered capability (map §4 C1 records the
// rationale; README L14 and this gate moved together). The 1.0 m end of the
// revised band is a non-gating target (~25 % of samples) and is not asserted.
// RecallAt1m / RecallAt2m are logged as diagnostics, mirroring
// simulator.AccuracyReport's definitions.
//
// N1 guard (README L20 negative claim): the Fresnel grid cell from
// /api/settings ("grid_cell_m", default 0.2 m) must be ≥ 0.10 m, and no
// assertion in this file pins position error below 0.10 m — the suite must
// never demand the sub-10 cm accuracy the README explicitly disclaims.
//
// A measured FAIL of the 1.5 m gate (median/p90 in the test log) is a valid
// outcome of this scenario: the deliverable is the deterministic fixture plus
// the honest measurement, not a green run. Gate changes go through the map
// (docs/notes/localization-capability-acceptance-map.md §4 C1) together with
// the README claim — never per-run to turn a run green.
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

	// Gate: README L14 upper bound on approximate 2D position accuracy — the
	// upper end of the re-based ±1.0–1.5 m band (spaxel-1a2c7859; the original
	// 1.0 m figure over-stated delivered capability, see map §4 C1).
	as8MedianErrorGateM = 1.5

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

// AS-8-ext constants — joint 3D (x,y,z) position accuracy. The gate is
// derived from the already-measured per-axis bands, not an aspiration (map
// §4): composing the worst recorded medians — AS-8's XY 1.273 m with
// AS-3-ext's Z 1.00 m — gives sqrt(1.273² + 1.00²) ≈ 1.62 m, and the best
// recorded pair (1.069 m, 0.40 m) composes to ≈ 1.14 m. The gate sits at the
// 2.0 m ceiling of the README L17 Z band — the same measured-capability
// headroom pattern as AS-8's 1.5 m gate over its 1.27 m measured median.
const (
	as8dMedianErrorGateM = 2.0

	// Non-gating target: the best recorded band pair composed. Logged for the
	// map's §8 record; never asserted (and comfortably above the 0.10 m N1
	// floor).
	as8dMedianErrorTargetM = 1.14
)

// AS8_3DPositionAccuracyIntegration runs the deterministic AS-8-ext scenario:
// joint 3D (x,y,z) position accuracy of tracked blobs against the CSV ground
// truth, pinning the current capability behind README L5's "estimate 2D/3D
// position" claim the way AS-8 pins the 2D claim.
//
// Numbering: AS-8-ext, extend-in-place — map §2 keeps scenario numbers
// contiguous and AS-10 is assigned-but-unimplemented, so no fresh AS-11 can be
// reserved; the extension follows the AS-2-ext/AS-3-ext precedent.
//
// Fixture: the AS-8 fixture plus --node-heights mixed (required: mixed-height
// perimeter nodes are what gives the fusion stack its vertical geometry — the
// same condition README L17 places on the Z claim), one path walker on the
// same scripted loop, seed 42. One walker deliberately: joint-3D accuracy must
// not be confounded with AS-9's documented Fresnel-merging separation weakness
// (two simultaneous walkers present as one ridge in the large majority of
// polls) — that is a counting/separation defect (map C3), not an accuracy one.
//
// Z ground truth: the sim engine's path walker moves in XY only —
// updatePathFollow keeps Position.Z at the first waypoint's Z, and its 3D
// arrival check (< 0.1 m) would wedge a walker on z-varying waypoints — so the
// walker stands at the loop's 1.7 m for the whole run and vertical error is
// exercised as the localized blob Z against that standing height. Horizontal
// and vertical error are exercised by the same walker in the same run.
//
// A measured FAIL of the 2.0 m gate is a valid outcome (the documented AS-9
// separation weakness and the AS-2-ext trajectory bound both predict pressure
// on the joint figure): the deliverable is the deterministic fixture plus the
// honest measurement, recorded in map §8. Gate changes go through the map —
// never per-run to turn a run green.
func AS8_3DPositionAccuracyIntegration(t *testing.T) {
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

	// N1 guard, same premise as AS-8: no sub-10 cm resolution may be assumed.
	as8AssertGridCellFloor(t, mothershipURL)

	dir := t.TempDir()
	pathFile := filepath.Join(dir, "as8d-scripted-loop.json")
	gtCSV := filepath.Join(dir, "as8d-ground-truth.csv")
	if err := os.WriteFile(pathFile, []byte(as8ScriptedLoopJSON), 0o644); err != nil {
		t.Fatalf("Failed to write scripted path file: %v", err)
	}

	simStart := time.Now()
	simCtx, cancelSim := context.WithTimeout(ctx, 2*time.Minute)
	simCmd := startSimulator(t, simCtx, []string{
		"--mothership", wsURL(mothershipURL),
		"--nodes", fmt.Sprintf("%d", as8Nodes),
		"--node-heights", "mixed",
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

	// Same polling discipline as AS-8: 1 Hz across the run window, post-warmup
	// samples only, and every sample must carry all three axes — a blob
	// without Z cannot form a joint-3D error and is counted, not silently
	// averaged in as z=0.
	pollDeadline := simStart.Add(as8DurationS*time.Second + 10*time.Second)
	var samples []as8BlobSample
	noZ := 0
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
		for _, blob := range as8GetBlobs(t, mothershipURL) {
			bx, ok := as8BlobCoord(blob, "X", "x")
			if !ok {
				continue
			}
			by, ok := as8BlobCoord(blob, "Y", "y")
			if !ok {
				continue
			}
			bz, ok := as8BlobCoord(blob, "Z", "z")
			if !ok {
				noZ++
				continue
			}
			samples = append(samples, as8BlobSample{elapsed: elapsed, x: bx, y: by, z: bz})
		}
		time.Sleep(1 * time.Second)
	}

	gt, err := as8ParseGroundTruthCSV(gtCSV)
	if err != nil {
		t.Fatalf("Failed to parse ground-truth CSV: %v", err)
	}
	if len(gt) == 0 {
		t.Fatal("Ground-truth CSV contains no walker positions — sim wrote no ground truth")
	}

	zs := make([]float64, 0, len(gt))
	for _, g := range gt {
		zs = append(zs, g.z)
	}
	sort.Float64s(zs)
	t.Logf("Ground truth: %d walker positions, GT z range %.2f–%.2f m (path walker holds z constant), %d mixed-height nodes",
		len(gt), zs[0], zs[len(zs)-1], as8Nodes)
	if noZ > 0 {
		t.Logf("Skipped %d blob samples lacking a Z coordinate", noZ)
	}

	if len(samples) == 0 {
		t.Fatalf("No tracked-blob samples with all three axes after %.0f s warmup across %d polls — "+
			"the pipeline localized nothing measurable in 3D from the scripted walker",
			as8Warmup.Seconds(), polls)
	}

	// Gate metric: per blob sample, the full 3D Euclidean distance to the
	// nearest ground-truth position (time-free, as AS-8 — the loop repeats
	// every 14 s so the post-warmup window covers it ~3×). Computed generically
	// in all three axes: today the GT z is constant, but the metric must not
	// silently degrade to 2D if a z-varying fixture ever lands.
	errors3d := make([]float64, 0, len(samples))
	xyAtNearest := make([]float64, 0, len(samples))
	dzAtNearest := make([]float64, 0, len(samples))
	for _, s := range samples {
		d, g := as8Nearest3D(s.x, s.y, s.z, gt)
		errors3d = append(errors3d, d)
		xyAtNearest = append(xyAtNearest, math.Hypot(s.x-g.x, s.y-g.y))
		dzAtNearest = append(dzAtNearest, math.Abs(s.z-g.z))
	}

	sort.Float64s(errors3d)
	sort.Float64s(xyAtNearest)
	sort.Float64s(dzAtNearest)
	median3d := as8Percentile(errors3d, 0.5)
	p90_3d := as8Percentile(errors3d, 0.90)
	recall15m, recall20m := 0.0, 0.0
	for _, e := range errors3d {
		if e <= 1.5 {
			recall15m++
		}
		if e <= 2.0 {
			recall20m++
		}
	}
	recall15m /= float64(len(errors3d))
	recall20m /= float64(len(errors3d))

	t.Logf("AS-8-ext measurement: %d blob samples — median 3D error %.3f m (gate ≤ %.1f m, "+
		"measured-band target %.2f m), p90 %.3f m, joint recall ≤1.5 m %.1f%%, ≤2.0 m %.1f%%",
		len(errors3d), median3d, as8dMedianErrorGateM, as8dMedianErrorTargetM, p90_3d,
		100*recall15m, 100*recall20m)
	t.Logf("AS-8-ext decomposition at each sample's 3D-nearest ground truth: median XY %.3f m, median |Δz| %.3f m",
		as8Percentile(xyAtNearest, 0.5), as8Percentile(dzAtNearest, 0.5))

	if median3d > as8dMedianErrorGateM {
		t.Errorf("Median 3D error %.3f m exceeds the %.1f m gate derived from the measured bands "+
			"(p90 %.3f m, joint recall ≤1.5 m %.1f%%, ≤2.0 m %.1f%% over %d blob samples) — "+
			"a FAIL is a recorded outcome (map §8), not a gate to be loosened",
			median3d, as8dMedianErrorGateM, p90_3d, 100*recall15m, 100*recall20m, len(errors3d))
	}
}

// as8Nearest3D returns the 3D Euclidean distance from (x, y, z) to the nearest
// ground-truth position, together with that position (for per-axis
// decomposition).
func as8Nearest3D(x, y, z float64, gt []as8GTPosition) (float64, as8GTPosition) {
	best := math.Inf(1)
	var nearest as8GTPosition
	for _, g := range gt {
		dx, dy, dz := x-g.x, y-g.y, z-g.z
		if d := math.Sqrt(dx*dx + dy*dy + dz*dz); d < best {
			best, nearest = d, g
		}
	}
	return best, nearest
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

// as8BlobSample is one tracked-blob observation from /api/blobs. z is
// populated only by the AS-8-ext joint-3D scenario (AS-8's horizontal metric
// ignores it and its samples carry the zero value).
type as8BlobSample struct {
	elapsed time.Duration
	x, y, z float64
}

// as8GTPosition is one ground-truth walker position from the sim's CSV output.
// z is the CSV height column; the path walker holds it constant at the first
// waypoint's height (engine quirk, see AS8_3DPositionAccuracyIntegration).
type as8GTPosition struct {
	x, y, z float64
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
		if _, err := fmt.Sscanf(row[4], "%f", &p.z); err != nil {
			return nil, fmt.Errorf("CSV z column %q: %w", row[4], err)
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
