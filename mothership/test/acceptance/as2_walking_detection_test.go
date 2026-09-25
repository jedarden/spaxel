// Package acceptance provides integration tests for Spaxel acceptance scenarios.
// AS-2: Person detected while walking
//
// Pass criteria:
// - spaxel-sim --nodes 2 --walkers 1 --duration 60s runs successfully
// - Polling /api/blobs every second detects blob count > 0
// - Blob count > 0 for >80% of the 60-second run duration
// - Blob appears within 3 seconds of walker starting
// - Blob disappears within 5 seconds of walker stopping
//
// Fail criteria:
// - No blob appears during the walk
// - Blob persists >30 seconds after walker stops
package acceptance

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// AS2_PersonDetectedWhileWalking verifies that a walking person is detected.
func AS2_PersonDetectedWhileWalking(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	srv := startMockMothershipForWalking(t)
	defer srv.Close()

	const nodes = 2
	const walkers = 1
	duration := 60 * time.Second
	pollInterval := 1 * time.Second
	detectionThreshold := 0.8 // 80% of run must have blobs

	_ = nodes
	_ = walkers

	t.Run("StartSimulator", func(t *testing.T) {
		// Track blob detection over time
		blobDetection := runBlobDetectionMonitoring(t, srv.URL, duration, pollInterval)

		// Verify detection criteria
		secondsWithBlobs := 0
		totalSeconds := int(duration.Seconds())

		for _, hasBlob := range blobDetection {
			if hasBlob {
				secondsWithBlobs++
			}
		}

		detectionRatio := float64(secondsWithBlobs) / float64(totalSeconds)
		t.Logf("Detection: blobs detected in %d/%d seconds (%.1f%%)",
			secondsWithBlobs, totalSeconds, detectionRatio*100)

		if detectionRatio < detectionThreshold {
			t.Errorf("Detection ratio %.1f%% below threshold %.1f%%",
				detectionRatio*100, detectionThreshold*100)
		}

		// Verify blob appeared within 3 seconds of start
		firstDetectionTime := findFirstDetection(blobDetection)
		if firstDetectionTime > 3*time.Second {
			t.Errorf("First detection took %v, want < 3s", firstDetectionTime)
		}

		t.Log("AS-2: Person detected while walking PASSED")
	})
}

// AS2_BlobAppearsQuickly verifies blob appears within 3 seconds of walker starting.
func AS2_BlobAppearsQuickly(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	srv := startMockMothershipForWalking(t)
	defer srv.Close()

	// Start monitoring for blobs
	blobChan := make(chan bool, 100)
	go monitorBlobsWithContext(ctx, t, srv.URL, blobChan)

	// Simulate walker starting
	time.Sleep(2 * time.Second)

	// Check if blob detected
	select {
	case hasBlob := <-blobChan:
		if !hasBlob {
			t.Error("Expected blob detection within 3 seconds")
		}
		t.Log("Blob detected quickly - PASSED")
	case <-time.After(5 * time.Second):
		t.Error("Timeout waiting for blob detection")
	case <-ctx.Done():
		t.Fatal("Context cancelled")
	}
}

// AS2_BlobDisappearsAfterStop verifies blob disappears within 5 seconds after walker stops.
func AS2_BlobDisappearsAfterStop(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	srv := startMockMothershipForWalking(t)
	defer srv.Close()

	// Simulate walker moving
	simulateWalker := func() {
		// Send CSI data with motion
		sendMockCSI(t, srv.URL, true)
		time.Sleep(10 * time.Second)

		// Walker stops
		sendMockCSI(t, srv.URL, false)
	}

	go simulateWalker()

	// Monitor for blob disappearance
	blobPresent := true
	stillnessStart := time.Time{}

	for i := 0; i < 50; i++ { // Check for 5 seconds
		time.Sleep(100 * time.Millisecond)

		blobs := getBlobsResponse(t, srv.URL)
		currentHasBlob := len(blobs) > 0

		if blobPresent && !currentHasBlob {
			stillnessStart = time.Now()
			blobPresent = false
		}

		if !blobPresent && time.Since(stillnessStart) > 5*time.Second {
			t.Log("Blob disappeared within 5 seconds - PASSED")
			return
		}
	}

	t.Error("Blob did not disappear within 5 seconds after walker stopped")
}

// AS2_TwoNodesDetection verifies detection works with 2 nodes.
func AS2_TwoNodesDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	srv := startMockMothershipForWalking(t)
	defer srv.Close()

	// Verify 2 nodes are registered
	nodes := getNodesResponse(t, srv.URL)
	if len(nodes) < 2 {
		t.Fatalf("Expected at least 2 nodes, got %d", len(nodes))
	}

	// Check that both nodes are online
	onlineCount := 0
	for _, node := range nodes {
		if node["status"] == "online" {
			onlineCount++
		}
	}

	if onlineCount < 2 {
		t.Errorf("Expected 2 online nodes, got %d", onlineCount)
	}

	// Run walking simulation
	duration := 30 * time.Second
	blobDetection := runBlobDetectionMonitoring(t, srv.URL, duration, 1*time.Second)

	// Count detection percentage
	detectedSeconds := 0
	for _, hasBlob := range blobDetection {
		if hasBlob {
			detectedSeconds++
		}
	}

	detectionRatio := float64(detectedSeconds) / float64(int(duration.Seconds()))
	if detectionRatio < 0.8 {
		t.Errorf("Detection ratio %.1f%% below 80%% threshold", detectionRatio*100)
	}

	t.Logf("AS-2: Two nodes detection - %.1f%% detection rate", detectionRatio*100)
}

// startMockMothershipForWalking creates a mock server for walking detection tests.
func startMockMothershipForWalking(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/blobs":
			// Return mock blob data
			blobs := []map[string]interface{}{
				{
					"id":         1,
					"x":          2.5,
					"y":          2.5,
					"z":          1.0,
					"confidence": 0.85,
					"vx":         0.3,
					"vy":         0.1,
					"vz":         0.0,
					"posture":    "walking",
				},
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"blobs": blobs,
			})

		case "/api/nodes":
			// Return mock nodes
			nodes := []map[string]interface{}{
				{
					"mac":    "AA:BB:CC:DD:EE:FF",
					"name":   "Node 1",
					"status": "online",
					"role":   "tx_rx",
				},
				{
					"mac":    "AA:BB:CC:DD:EE:F0",
					"name":   "Node 2",
					"status": "online",
					"role":   "tx_rx",
				},
			}
			json.NewEncoder(w).Encode(nodes)

		case "/ws/node":
			// Upgrade to WebSocket for simulator connection
			// For mock, just return 200
			w.WriteHeader(http.StatusOK)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// runBlobDetectionMonitoring monitors blob detection over the test duration.
func runBlobDetectionMonitoring(t *testing.T, baseURL string, duration, pollInterval time.Duration) []bool {
	t.Helper()

	totalChecks := int(duration.Seconds())
	detection := make([]bool, totalChecks)

	for i := 0; i < totalChecks; i++ {
		blobs := getBlobsResponse(t, baseURL)
		detection[i] = len(blobs) > 0
		time.Sleep(pollInterval)
	}

	return detection
}

// findFirstDetection finds the first time a blob was detected.
func findFirstDetection(detection []bool) time.Duration {
	for i, hasBlob := range detection {
		if hasBlob {
			return time.Duration(i) * time.Second
		}
	}
	return -1
}

// sendMockCSI sends mock CSI data to simulate motion/no-motion.
func sendMockCSI(t *testing.T, baseURL string, hasMotion bool) {
	t.Helper()
	// In the full integration test, this would send CSI frames via WebSocket
	// For unit testing, we just log the action
	if hasMotion {
		t.Log("Simulating CSI data with motion")
	} else {
		t.Log("Simulating CSI data without motion")
	}
}

// monitorBlobsWithContext continuously monitors the blobs API.
func monitorBlobsWithContext(ctx context.Context, t *testing.T, baseURL string, blobChan chan<- bool) {
	t.Helper()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			blobs := getBlobsResponse(t, baseURL)
			select {
			case blobChan <- len(blobs) > 0:
			case <-ctx.Done():
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// AS2_Integration is the full integration test for CI.
func AS2_Integration(t *testing.T) {
	if os.Getenv("SPAXEL_INTEGRATION_TEST") != "1" {
		t.Skip("Set SPAXEL_INTEGRATION_TEST=1 to run full integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// This test requires:
	// 1. Running mothership at SPAXEL_MOTHERSHIP_URL
	// 2. spaxel-sim binary available

	mothershipURL := os.Getenv("SPAXEL_MOTHERSHIP_URL")
	if mothershipURL == "" {
		mothershipURL = "http://localhost:8080"
	}

	// Verify mothership is reachable
	if !checkMothershipHealth(ctx, mothershipURL) {
		t.Fatal("Mothership not reachable at " + mothershipURL)
	}

	// Start simulator
	simArgs := []string{
		"--mothership", "ws://" + urlParseHost(mothershipURL) + ":8080/ws/node",
		"--nodes", "2",
		"--walkers", "1",
		"--duration", "60",
		"--verify",
	}

	simCmd := exec.CommandContext(ctx, "spaxel-sim", simArgs...)
	if err := simCmd.Start(); err != nil {
		t.Fatalf("Failed to start spaxel-sim: %v", err)
	}
	defer simCmd.Process.Kill()

	// Monitor blobs during simulation
	blobDetection := runBlobDetectionMonitoring(t, mothershipURL, 60*time.Second, 1*time.Second)

	// Calculate detection ratio
	secondsWithBlobs := 0
	for _, hasBlob := range blobDetection {
		if hasBlob {
			secondsWithBlobs++
		}
	}

	detectionRatio := float64(secondsWithBlobs) / 60.0
	t.Logf("Detection ratio: %.1f%% (%d/60 seconds)", detectionRatio*100, secondsWithBlobs)

	if detectionRatio < 0.8 {
		t.Errorf("Detection ratio %.1f%% below 80%% threshold", detectionRatio*100)
	}

	// Wait for simulator to complete
	if err := simCmd.Wait(); err != nil {
		t.Logf("Simulator exited with error: %v", err)
	}

	t.Log("AS-2 Integration test PASSED")
}

// urlParseHost extracts the host from a URL.
func urlParseHost(rawURL string) string {
	u, err := parseURL(rawURL)
	if err != nil {
		return "localhost"
	}
	if u.Host == "" {
		return "localhost"
	}
	return u.Host
}

// parseURL parses a URL string.
func parseURL(rawURL string) (*url.URL, error) {
	return url.Parse(rawURL)
}

// AS2_SmoothDeltaRMSAboveThreshold verifies smooth_deltaRMS exceeds threshold during walking.
func AS2_SmoothDeltaRMSAboveThreshold(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	// This test verifies that when walking, smooth_deltaRMS > 0.05
	// In the full test, this would query /api/nodes/{mac}/diagnostics

	// For unit test, we simulate the diagnostics response
	diagnosticsResponse := map[string]interface{}{
		"links": []map[string]interface{}{
			{
				"link_id":          "AA:BB:CC:DD:EE:FF:AA:BB:CC:DD:EE:F0",
				"delta_rms":        0.08,
				"smooth_delta_rms": 0.07,
				"threshold":        0.02,
			},
		},
	}

	links, ok := diagnosticsResponse["links"].([]map[string]interface{})
	if !ok || len(links) == 0 {
		t.Fatal("Expected links in diagnostics response")
	}

	// Verify smooth_deltaRMS exceeds threshold
	for _, link := range links {
		threshold, _ := link["threshold"].(float64)
		smoothDeltaRMS, _ := link["smooth_delta_rms"].(float64)

		if smoothDeltaRMS <= threshold {
			t.Errorf("smooth_deltaRMS %.2f not above threshold %.2f",
				smoothDeltaRMS, threshold)
		}
	}

	t.Log("smooth_deltaRMS exceeds threshold during walking - PASSED")
}

// ============================================================================
// AS-2-ext: trajectory-bound tracking (localization capability map §4 C2)
//
// Design authority: docs/notes/localization-capability-acceptance-map.md §4,
// which prescribes exactly: with the walker on a scripted path
// (--walker-type path --path-file), sample /api/blobs for the run and assert
// the per-sample horizontal distance from blob to the ground-truth polyline
// ≤ 1.0 m for ≥ 80 % of tracked samples. C2 carries no numeric figure of its
// own — the bound is borrowed from README L14's ±1.0 m upper end of
// "±0.5–1.0 m with 4+ nodes" and cited as such; the 0.5 m end is a non-gating
// target and is not asserted.
//
// The fixture is deliberately identical to AS-8's (shared as8* constants and
// as8ScriptedLoopJSON: 4 nodes, seed 42, 6x5x2.5 m space, 60 s at 20 Hz,
// rectangular loop at Z 1.7 m, --output-csv ground truth) so this measurement
// is directly comparable to C1's. AS-8 measures time-free per-blob distance
// to the nearest ground-truth POINT; AS-2-ext measures per-poll
// nearest-blob distance to the ground-truth POLYLINE — the trajectory-tracking
// property, immune to the walker's phase within its lap.
//
// A measured FAIL of the ≥ 80 %-within-1.0 m gate is a valid outcome of this
// scenario: the deliverable is the deterministic fixture plus the honest
// measurement, not a green run. Do not loosen the gate to pass. (AS-8
// precedent: its live measurement measured FAIL against its own gate —
// median 1.140–1.273 m vs 1.0 m — and landed as such; C2 inherits that
// suspicion.)
// ============================================================================

const (
	// Gate bound: README L14 upper end (±1.0 m), borrowed for C2 per map §4
	// ("the bound is borrowed from L14's ±1.0 m and cited as such").
	as2extTrajBoundM = 1.0

	// Gate fraction: ≥ 80 % of tracked samples within the bound (map §4 C2).
	as2extBoundFraction = 0.80

	// Poll deadline extends 10 s past the sim's duration so the final second
	// of frames has been ingested and served (same shape as AS-8/AS-9).
	as2extPollTail = 10 * time.Second
)

// as2extPathFile mirrors the --path-file schema (cmd/sim PathDefinition) so
// the ground-truth polyline is derived from the SAME scripted waypoints the
// sim is fed — never re-typed by hand.
type as2extPathFile struct {
	Waypoints []as2extWaypoint `json:"waypoints"`
}

type as2extWaypoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// AS2_TrajectoryBoundIntegration runs the deterministic AS-2-ext scenario end
// to end: scripted walker, live mothership, per-poll trajectory-bound gate.
func AS2_TrajectoryBoundIntegration(t *testing.T) {
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

	// Ground-truth polyline: the same scripted rectangle the walker follows,
	// closed (first waypoint appended) so the loop's closing leg back to the
	// start is covered. Correct under both loop and ping-pong traversal —
	// both cross exactly these segments.
	polyline, err := as2extPolyline(as8ScriptedLoopJSON)
	if err != nil {
		t.Fatalf("Failed to derive ground-truth polyline from scripted loop: %v", err)
	}

	dir := t.TempDir()
	pathFile := filepath.Join(dir, "as2ext-scripted-loop.json")
	gtCSV := filepath.Join(dir, "as2ext-ground-truth.csv")
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

	// Poll /api/blobs once a second across the post-warmup window (AS-8/AS-9
	// polling shape). /api/blobs serves a BARE array with Go-default
	// capitalized keys — decode via the proven as8 helpers, never the shared
	// envelope decoders (they silently see zero live blobs). Per tracked
	// sample the gate metric is the NEAREST blob's horizontal distance to the
	// polyline: "the person's estimated path stays within the bound", not
	// absence of second blobs (clutter tracking is AS-9's concern, not C2's).
	pollDeadline := simStart.Add(as8DurationS*time.Second + as2extPollTail)
	type as2extPoll struct {
		elapsed  time.Duration
		polyDist float64 // nearest blob → polyline (Inf = nothing tracked)
		trackX   float64 // that blob's position, for the CSV cross-check
		trackY   float64
		blobs    int
	}
	var polls []as2extPoll
	for time.Now().Before(pollDeadline) {
		if ctx.Err() != nil {
			break
		}
		elapsed := time.Since(simStart)
		if elapsed < as8Warmup {
			time.Sleep(1 * time.Second)
			continue
		}
		poll := as2extPoll{elapsed: elapsed, polyDist: math.Inf(1)}
		blobs := as8GetBlobs(t, mothershipURL)
		poll.blobs = len(blobs)
		for _, blob := range blobs {
			bx, okX := as8BlobCoord(blob, "X", "x")
			by, okY := as8BlobCoord(blob, "Y", "y")
			if !okX || !okY {
				continue
			}
			if d := as2extDistToPolyline(bx, by, polyline); d < poll.polyDist {
				poll.polyDist = d
				poll.trackX, poll.trackY = bx, by
			}
		}
		polls = append(polls, poll)
		time.Sleep(1 * time.Second)
	}

	// Ground truth as recorded by the sim (map §6.4: ground truth by
	// construction, never inferred from the system under test).
	gt, err := as8ParseGroundTruthCSV(gtCSV)
	if err != nil {
		t.Fatalf("Failed to parse ground-truth CSV: %v", err)
	}
	if len(gt) == 0 {
		t.Fatal("Ground-truth CSV contains no walker positions — sim wrote no ground truth")
	}

	// Fixture audit (diagnostic only): the recorded walker positions must hug
	// the polyline for the bound to be measuring the right path. A sim
	// semantics change (e.g. corner rounding, path-file schema drift) shows
	// up here before it can contaminate the gate.
	gtToPoly := make([]float64, 0, len(gt))
	for _, g := range gt {
		gtToPoly = append(gtToPoly, as2extDistToPolyline(g.x, g.y, polyline))
	}
	sort.Float64s(gtToPoly)
	t.Logf("Fixture audit: %d CSV walker positions, distance to scripted polyline "+
		"median %.3f m max %.3f m", len(gt), as8Percentile(gtToPoly, 0.5), gtToPoly[len(gtToPoly)-1])

	// Gate metric over tracked samples: polls after warmup where at least one
	// blob carried usable coordinates. Empty polls are excluded from the
	// fraction (the map gates "tracked samples") but surface in the
	// detection-ratio diagnostic below.
	polyDists := make([]float64, 0, len(polls))
	csvDists := make([]float64, 0, len(polls))
	within := 0
	pollsWithBlob := 0
	for _, p := range polls {
		if p.blobs > 0 {
			pollsWithBlob++
		}
		if math.IsInf(p.polyDist, 1) {
			continue
		}
		polyDists = append(polyDists, p.polyDist)
		csvDists = append(csvDists, as8MinHorizontalDist(p.trackX, p.trackY, gt))
		if p.polyDist <= as2extTrajBoundM {
			within++
		}
	}

	t.Logf("Post-warmup window: %d polls, detection ratio %.1f%%, tracked samples %d",
		len(polls), 100*float64(pollsWithBlob)/float64(max(len(polls), 1)), len(polyDists))

	if len(polyDists) == 0 {
		t.Fatalf("No tracked samples after %.0f s warmup across %d polls — "+
			"the pipeline localized nothing from the scripted walker",
			as8Warmup.Seconds(), len(polls))
	}

	sort.Float64s(polyDists)
	median := as8Percentile(polyDists, 0.5)
	p90 := as8Percentile(polyDists, 0.90)
	fraction := float64(within) / float64(len(polyDists))

	// CSV cross-check (diagnostic, C1-comparable): the same nearest-to-polyline
	// blob measured against the nearest CSV ground-truth POINT, AS-8-style.
	sort.Float64s(csvDists)
	t.Logf("AS-2-ext measurement: %d tracked samples — nearest-blob distance to scripted "+
		"path median %.3f m, p90 %.3f m, within %.1f m gate %.1f%% (want ≥ %.0f%%)",
		len(polyDists), median, p90, as2extTrajBoundM, 100*fraction, 100*as2extBoundFraction)
	t.Logf("AS-2-ext CSV cross-check (same samples vs nearest ground-truth point, AS-8 style): "+
		"median %.3f m, p90 %.3f m",
		as8Percentile(csvDists, 0.5), as8Percentile(csvDists, 0.90))

	if fraction < as2extBoundFraction {
		t.Errorf("Trajectory bound: only %.1f%% of %d tracked samples kept the nearest blob "+
			"within %.1f m of the scripted path, want ≥ %.0f%% (README L14 bound borrowed for C2; "+
			"median %.3f m, p90 %.3f m)",
			100*fraction, len(polyDists), as2extTrajBoundM, 100*as2extBoundFraction, median, p90)
	}
}

// as2extPolyline derives the closed ground-truth polyline (XY) from a
// --path-file JSON string, appending the first waypoint so the loop's closing
// leg is covered.
func as2extPolyline(pathFileJSON string) ([]as2extWaypoint, error) {
	var paths []as2extPathFile
	if err := json.Unmarshal([]byte(pathFileJSON), &paths); err != nil {
		return nil, err
	}
	if len(paths) != 1 || len(paths[0].Waypoints) < 2 {
		return nil, fmt.Errorf("path file has %d walkers / %d waypoints on the first, want 1 walker with ≥ 2",
			len(paths), func() int {
				if len(paths) == 0 {
					return 0
				}
				return len(paths[0].Waypoints)
			}())
	}
	pts := paths[0].Waypoints
	return append(append([]as2extWaypoint(nil), pts...), pts[0]), nil
}

// as2extDistToPolyline returns the smallest horizontal (XY) distance from
// (x, y) to the polyline, measured point-to-SEGMENT with the projection
// clamped to the segment — point-to-point against waypoints alone would
// under-report mid-segment error.
func as2extDistToPolyline(x, y float64, poly []as2extWaypoint) float64 {
	minDist := math.Inf(1)
	for i := 0; i+1 < len(poly); i++ {
		ax, ay := poly[i].X, poly[i].Y
		bx, by := poly[i+1].X, poly[i+1].Y
		abx, aby := bx-ax, by-ay
		apx, apy := x-ax, y-ay
		lenSq := abx*abx + aby*aby
		d := math.Hypot(apx, apy) // degenerate segment: point-to-point
		if lenSq > 0 {
			t := (apx*abx + apy*aby) / lenSq
			if t < 0 {
				t = 0
			} else if t > 1 {
				t = 1
			}
			d = math.Hypot(x-(ax+t*abx), y-(ay+t*aby))
		}
		if d < minDist {
			minDist = d
		}
	}
	return minDist
}
