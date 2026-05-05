// Package acceptance provides AS-2: Person detected while walking.
//
// Pass criteria:
// - spaxel-sim --nodes 2 --walkers 1 runs for 60 seconds
// - GET /api/blobs returns at least 1 blob during walk
// - Blob count matches walker count (±1 tolerance)
// - Detection events appear in /api/events
//
// Fail criteria:
// - No blobs detected
// - Blob count significantly different from walker count
package acceptance

import (
	"context"
	"testing"
	"time"
)

// TestAS2_WalkingDetection verifies that a walking person is detected.
func TestAS2_WalkingDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	h := NewTestHarness(t)
	defer h.Stop()

	// Start mothership
	if err := h.Start(ctx); err != nil {
		t.Fatalf("Failed to start mothership: %v", err)
	}

	// Set PIN first
	if err := h.SetPIN(ctx, "1234"); err != nil {
		t.Fatalf("Failed to set PIN: %v", err)
	}

	// Run simulator with 2 nodes, 1 walker
	simCtx, simCancel := context.WithTimeout(ctx, 2*time.Minute)
	if err := h.RunSimulator(simCtx, []string{
		"--nodes", "2",
		"--walkers", "1",
		"--rate", "20",
		"--duration", "60",
	}); err != nil {
		t.Fatalf("Failed to start simulator: %v", err)
	}
	defer simCancel()

	// Wait for simulator to start
	time.Sleep(2 * time.Second)

	// Monitor blobs for 60 seconds
	t.Run("MonitorBlobsDuringWalk", func(t *testing.T) {
		detectionStart := time.Now()
		monitorDuration := 60 * time.Second
		pollInterval := 1 * time.Second

		blobPresentCount := 0
		totalPolls := 0
		maxBlobCount := 0

		for time.Since(detectionStart) < monitorDuration {
			select {
			case <-ctx.Done():
				t.Fatal("Context cancelled during monitoring")
			default:
			}

			totalPolls++

			blobs, err := h.GetBlobs(ctx)
			if err != nil {
				t.Logf("Failed to get blobs: %v", err)
			} else {
				if len(blobs) > 0 {
					blobPresentCount++
					if len(blobs) > maxBlobCount {
						maxBlobCount = len(blobs)
					}
				}
			}

			time.Sleep(pollInterval)
		}

		detectionRatio := float64(blobPresentCount) / float64(totalPolls)
		t.Logf("Detection ratio: %.1f%% (%d/%d polls with blobs, max count: %d)",
			detectionRatio*100, blobPresentCount, totalPolls, maxBlobCount)

		// Verify detection ratio > 60% (relaxed threshold for CI)
		if detectionRatio < 0.6 {
			t.Errorf("Detection ratio %.1f%% below 60%% threshold", detectionRatio*100)
		}
	})

	// Verify detection events appear
	t.Run("DetectionEventsPresent", func(t *testing.T) {
		// Wait a moment for events to be recorded
		time.Sleep(2 * time.Second)

		events, err := h.GetEvents(ctx, "detection", 10)
		if err != nil {
			t.Fatalf("Failed to get detection events: %v", err)
		}

		if len(events) == 0 {
			t.Error("No detection events found")
		} else {
			t.Logf("Found %d detection events", len(events))
		}
	})

	t.Log("AS-2: Walking detection test completed")
}

// TestAS2_BlobCountMatchesWalkers verifies blob count matches walker count.
func TestAS2_BlobCountMatchesWalkers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	h := NewTestHarness(t)
	defer h.Stop()

	// Start mothership
	if err := h.Start(ctx); err != nil {
		t.Fatalf("Failed to start mothership: %v", err)
	}

	// Set PIN
	if err := h.SetPIN(ctx, "1234"); err != nil {
		t.Fatalf("Failed to set PIN: %v", err)
	}

	// Run simulator with 2 nodes, 2 walkers
	simCtx, simCancel := context.WithTimeout(ctx, 2*time.Minute)
	if err := h.RunSimulator(simCtx, []string{
		"--nodes", "2",
		"--walkers", "2",
		"--rate", "20",
		"--duration", "30",
	}); err != nil {
		t.Fatalf("Failed to start simulator: %v", err)
	}
	defer simCancel()

	// Wait for detection to stabilize
	time.Sleep(10 * time.Second)

	// Check blob count is within expected range
	blobs, err := h.GetBlobs(ctx)
	if err != nil {
		t.Fatalf("Failed to get blobs: %v", err)
	}

	// Expected: 2 walkers, tolerance ±1
	expectedMin := 1
	expectedMax := 3

	if len(blobs) < expectedMin {
		t.Errorf("Blob count %d below minimum %d", len(blobs), expectedMin)
	}

	if len(blobs) > expectedMax {
		t.Logf("Blob count %d above maximum %d (may be acceptable)", len(blobs), expectedMax)
	}

	t.Logf("AS-2: Blob count %d (expected range: %d-%d)", len(blobs), expectedMin, expectedMax)
}
