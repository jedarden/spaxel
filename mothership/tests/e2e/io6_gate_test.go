//go:build io6_gate

// io6_gate_test.go holds the IO-6 release hard-gate scenario
// (TestIO6HardGate_WalkerProducesTrackedBlob). It is gated behind the
// "io6_gate" build tag because it runs a 30-second release scenario and is
// intentionally opt-in for the default `go test ./...` suite. The tag only
// controls when the strict assertion runs; it does not weaken the assertion.
//
// RUNBOOK — run the gated scenario explicitly:
//
//	cd mothership && go test -tags io6_gate -run TestIO6HardGate ./tests/e2e/...
//
// A failure is a release-gate failure: the assertion still t.Fatalf's on zero
// tracked blobs, skips nothing on zero blobs, and downgrades nothing to a log.
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// GetNodeRecords retrieves the raw node records (including persisted positions)
// from /api/nodes. Unlike GetNodes (which drops PosX/PosY/PosZ), this preserves
// the announced positions so a RED gate can prove whether real geometry reached
// the mothership before fusion triage begins.
func (h *TestHarness) GetNodeRecords(ctx context.Context) ([]NodeRecord, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.APIURL+"/api/nodes", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck

	var records []NodeRecord
	if err := json.NewDecoder(resp.Body).Decode(&records); err != nil { //nolint:errcheck
		return nil, err
	}
	return records, nil
}

// StatusResponse is the /api/status snapshot — node count, tracked-blob count,
// and the system-wide detection-quality score. Used by the IO-6 hard-gate
// diagnostics to record the live state at the moment the gate is evaluated.
type StatusResponse struct {
	Nodes            int `json:"nodes"`
	Blobs            int `json:"blobs"`
	UptimeS          int `json:"uptime_s"`
	DetectionQuality int `json:"detection_quality"`
}

// GetStatus retrieves the /api/status snapshot.
func (h *TestHarness) GetStatus(ctx context.Context) (*StatusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.APIURL+"/api/status", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck

	var status StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil { //nolint:errcheck
		return nil, err
	}
	return &status, nil
}

// CaptureIO6Diagnostics assembles the diagnostic evidence the IO-6 hard-gate
// triage flow uses when the gate is RED (zero blobs).
//
// The evidence it emits is DATA-DRIVEN (bf-48juo), so the RED state stays
// actionable without guessing at the root cause:
//
//   - distinct geometry admitted (>=1 node with positions away from the
//     (0,0,1) schema default) AND zero blobs -> investigate the fusion
//     accumulation grid and Engine.SetNodePosition path, preserving the
//     strict assertion and feeding evidence back to bf-4q5w.
//   - no node admitted (empty /api/nodes, or all nodes collapsed to the
//     (0,0,1) schema default) -> auth/provision failure, NOT bf-4q5w. The
//     fusion engine never saw node geometry because no node was admitted in
//     the first place. bf-5k1z found the old unconditional bf-4q5w conclusion
//     printing alongside nodes=0, misattributing an auth failure to the wiring
//     gap.
//
// The raw data lines (status, per-node positions, atOrigin tally) are emitted
// unchanged either way. peakBlobs/detectionCount are the run-window maxima
// observed by the caller. The returned string is logged on failure rather than
// weakening the assertion; detailed fusion traces belong under .beads/traces/.
func (h *TestHarness) CaptureIO6Diagnostics(ctx context.Context, peakBlobs, detectionCount int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "IO-6 hard-gate diagnostics (do NOT weaken the assertion):\n")
	fmt.Fprintf(&sb, "  run-window maxima: peak concurrent blobs=%d, detection events=%d\n", peakBlobs, detectionCount)

	if status, err := h.GetStatus(ctx); err == nil {
		fmt.Fprintf(&sb, "  /api/status: nodes=%d blobs=%d detection_quality=%d uptime_s=%d\n",
			status.Nodes, status.Blobs, status.DetectionQuality, status.UptimeS)
	} else {
		fmt.Fprintf(&sb, "  /api/status: fetch failed: %v\n", err)
	}

	// Node-admission state is hoisted out of the fetch block so the conclusion
	// below can branch on it. The raw data lines (per-node positions, atOrigin
	// tally) are emitted unchanged either way.
	recordCount := 0
	atOrigin := 0
	recordsFetched := false
	if records, err := h.GetNodeRecords(ctx); err == nil {
		recordsFetched = true
		recordCount = len(records)
		fmt.Fprintf(&sb, "  node positions announced by spaxel-sim (prove real geometry reached the DB):\n")
		for _, n := range records {
			fmt.Fprintf(&sb, "    mac=%s role=%s pos=(%.3f, %.3f, %.3f)\n",
				n.MAC, n.Role, n.PosX, n.PosY, n.PosZ)
		}
		// Diagnostic summary: are the announced positions actually distinct, or did
		// they collapse to the (0,0,1) schema default? Distinct positions plus zero
		// blobs identifies the fusion path as the next triage boundary.
		for _, n := range records {
			if n.PosX == 0 && n.PosY == 0 {
				atOrigin++
			}
		}
		fmt.Fprintf(&sb, "    -> %d/%d nodes at xy=(0,0); distinct geometry was announced = %v\n",
			atOrigin, recordCount, atOrigin < recordCount)
	} else {
		fmt.Fprintf(&sb, "  /api/nodes: fetch failed: %v\n", err)
	}

	// DATA-DRIVEN TRIAGE (bf-48juo). Only send a zero-blob result toward fusion
	// triage when node geometry genuinely reached the DB distinctly (>=1 node
	// admitted, not all collapsed to the schema origin). An empty /api/nodes or
	// all nodes at origin means no node was admitted in the first place and must
	// NOT be misattributed to bf-4q5w.
	switch {
	case peakBlobs >= 1:
		sb.WriteString("  conclusion: /api/blobs observed tracked output, but the dashboard WebSocket\n" +
			"  feed was empty at assertion time; fusion and node-position wiring produced output.\n" +
			"  Triage the dashboard broadcast path and inspect .beads/traces/; keep the assertion strict.\n")
	case recordCount >= 1 && atOrigin < recordCount:
		sb.WriteString("  conclusion: CSI + distinct node geometry reach the mothership, but no tracked\n" +
			"  blob was observed. Inspect the fusion accumulation grid, Engine.SetNodePosition\n" +
			"  wiring, and traces under .beads/traces/, then feed the finding back to bf-4q5w;\n" +
			"  this is not a tolerated quiet-room condition — keep the IO-6 assertion strict.")
	case !recordsFetched:
		sb.WriteString("  conclusion: could not fetch /api/nodes, so node-admission state is unknown —\n" +
			"  do NOT attribute to bf-4q5w from this evidence alone; re-run the gate and, if it\n" +
			"  stays empty, see the auth-fix child.\n")
	default:
		fmt.Fprintf(&sb, "  conclusion: no nodes admitted (auth/provision failure) — %d/%d nodes at\n"+
			"  xy=(0,0); do NOT attribute to bf-4q5w, the fusion engine never saw node geometry\n"+
			"  because no node was admitted in the first place — see the auth-fix child.\n",
			atOrigin, recordCount)
	}
	return sb.String()
}

// TestIO6HardGate_WalkerProducesTrackedBlob is the dedicated IO-6 release
// hard-gate scenario.
//
// It runs RunSimulator(ctx, 4 nodes, 2 walkers, 20 Hz, duration) — the config
// described in bf-4q5w — and asserts that a walker produces a tracked blob
// end-to-end: BOTH the dashboard WS feed (AssertBlobObserved, bf-2330) AND the
// /api/blobs concurrent count (GetBlobCount, bf-2330) must show >=1 tracked
// blob during the run. TestFullE2EIntegration (bf-2aqf) and TestDetectionEvents
// (bf-16c1) cover the broader capstone; this is the single-purpose gate with
// diagnostic-evidence capture (CaptureIO6Diagnostics) for the triage flow.
//
// STRICT TRIAGE GATE: this assertion must remain hard even if a future fusion
// change makes the run RED. Never re-accept an empty feed, skip on zero blobs,
// or downgrade the failure to a log. If it regresses, capture the diagnostics
// from CaptureIO6Diagnostics, add fusion/grid traces under .beads/traces/, and
// feed that finding back to bf-4q5w rather than weakening the gate.
func TestIO6HardGate_WalkerProducesTrackedBlob(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), TestTimeout)
	defer cancel()

	h := NewTestHarness(t)
	defer h.Stop()

	if err := h.Start(ctx); err != nil {
		t.Fatalf("Failed to start mothership: %v", err)
	}

	// IO-6 hard-gate config: 4 nodes + 2 walkers at 20 Hz (bf-4q5w).
	simDuration := 30 * time.Second
	if err := h.RunSimulator(ctx, 4, 2, 20, simDuration); err != nil {
		t.Fatalf("Failed to run simulator: %v", err)
	}

	// Capture the dashboard WS blob feed concurrently while AssertDuringRun
	// polls the live state. WatchDashboardWS blocks for simDuration, so it must
	// run alongside AssertDuringRun (the simulator has finished by the time it
	// returns).
	var blobCounts []int
	var wsErr error
	var wsWG sync.WaitGroup
	wsWG.Add(1)
	go func() {
		defer wsWG.Done()
		blobCounts, wsErr = h.WatchDashboardWS(ctx, simDuration)
	}()

	// /api/blobs is a live concurrent view: a tracked blob can disappear on a
	// later fusion tick after the simulator stops. Keep the run-window peak
	// separately instead of sampling the endpoint only after the grace period.
	var apiPeak int
	var apiErr error
	var apiWG sync.WaitGroup
	apiWG.Add(1)
	go func() {
		defer apiWG.Done()
		apiPeak, apiErr = h.WatchBlobCount(ctx, simDuration)
	}()

	// AssertDuringRun (hardened by bf-2330) hard-fails if the whole run window
	// produces neither a blob nor a detection event. It also returns the
	// observed maxima implicitly via the live polls below.
	if err := h.AssertDuringRun(ctx, simDuration, 4); err != nil {
		// Capture diagnostics before failing so the RED state is actionable and
		// can be filed straight to bf-4q5w. Do NOT weaken the gate.
		peakBlobs, detections := io6RunMaxima(ctx, h)
		t.Fatalf("IO-6 hard-gate failed during run: %v\n%s", err,
			h.CaptureIO6Diagnostics(ctx, peakBlobs, detections))
	}

	wsWG.Wait()
	if wsErr != nil {
		t.Fatalf("Failed to watch dashboard WS: %v", wsErr)
	}
	apiWG.Wait()
	if apiErr != nil {
		t.Fatalf("Failed to watch /api/blobs: %v", apiErr)
	}

	// Grace period for any in-flight blobs/detection events to land.
	time.Sleep(2 * time.Second)

	// HARD GATE: a walker must produce a tracked blob end-to-end. Assert on BOTH
	// the dashboard WS feed (what the UI sees) and the concurrent /api/blobs
	// count (what the live 10 Hz loop exposes). An empty feed AND a zero count
	// means the fusion+tracking loop localized no walker from the 4-node /
	// 2-walker CSI stream — a detection regression at the fusion boundary, not a
	// tolerated quiet-room condition. The bf-2330 helper gates nil/empty/zero.
	_, detections := io6RunMaxima(ctx, h)
	if assertErr := AssertBlobObserved(blobCounts); assertErr != nil {
		t.Fatalf("IO-6 hard-gate (dashboard WS feed) failed: expected >=1 tracked "+
			"blob from a 4-node/2-walker run, but none was observed [%v]\n%s",
			assertErr, h.CaptureIO6Diagnostics(ctx, apiPeak, detections))
	}
	if apiPeak < 1 {
		t.Fatalf("IO-6 hard-gate (/api/blobs concurrent count) failed: expected >=1 "+
			"tracked blob from a 4-node/2-walker run, but peak concurrent count was %d\n%s",
			apiPeak, h.CaptureIO6Diagnostics(ctx, apiPeak, detections))
	}

	t.Logf("✓ IO-6 hard-gate PASSED: walker produced a tracked blob (dashboard WS peak >=1, /api/blobs peak=%d, %d detection events)",
		apiPeak, detections)
}

// WatchBlobCount records the peak concurrent tracked-blob count exposed by
// /api/blobs during a run. The endpoint is intentionally sampled throughout
// the run because tracked blobs are transient and may be gone by the time the
// simulator process exits.
func (h *TestHarness) WatchBlobCount(ctx context.Context, duration time.Duration) (int, error) {
	deadline := time.Now().Add(duration)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	peak := 0
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return peak, ctx.Err()
		case <-ticker.C:
			count, err := h.GetBlobCount(ctx)
			if err != nil {
				continue
			}
			if count > peak {
				peak = count
			}
		}
	}
	return peak, nil
}

// io6RunMaxima returns the current concurrent blob count (/api/blobs) and the
// detection-event count (/api/events?type=detection) for diagnostic capture.
// Used both inside the run loop (on AssertDuringRun failure) and at the gate.
func io6RunMaxima(ctx context.Context, h *TestHarness) (peakBlobs, detections int) {
	if n, err := h.GetBlobCount(ctx); err == nil && n > peakBlobs {
		peakBlobs = n
	}
	if events, err := h.GetEvents(ctx, "detection", 100); err == nil {
		detections = len(events.Events)
	}
	return peakBlobs, detections
}
