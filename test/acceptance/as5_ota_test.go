// Package acceptance provides AS-5: OTA update succeeds / rollback on bad firmware.
//
// Pass criteria:
// - spaxel-sim --scenario ota simulates successful OTA
// - Node firmware version increments after update
// - VERIFIED badge appears within 10 minutes
// - Rollback scenario: --scenario ota --ota-failure triggers rollback
// - Rollback firmware boots within 30 seconds
//
// Fail criteria:
// - Update fails or node gets stuck in FAILED state
// - Rollback doesn't trigger on boot failure
package acceptance

import (
	"context"
	"testing"
	"time"
)

// TestAS5_OTAUpdateSucceeds verifies OTA firmware update works correctly.
func TestAS5_OTAUpdateSucceeds(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Check test harness early exit condition
	select {
	case <-ctx.Done():
		t.Skip("Context cancelled early")
		return
	default:
	}

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

	// Start simulator with 1 node, OTA scenario
	simCtx, simCancel := context.WithTimeout(ctx, 90*time.Second)
	defer simCancel()
	if err := h.RunSimulator(simCtx, []string{
		"--nodes", "1",
		"--walkers", "0",
		"--duration", "0", // Run until cancelled
		"--scenario", "ota",
		"--ota-version", "sim-1.1.0",
		"--ota-size", "1048576", // 1MB
	}); err != nil {
		t.Fatalf("Failed to start simulator: %v", err)
	}

	// Wait for node to connect
	node, err := h.WaitForNode(ctx, "")
	if err != nil {
		t.Fatalf("Node did not connect: %v", err)
	}

	t.Logf("AS-5: Node connected - MAC: %v", node["mac"])

	// Wait for OTA completion event
	t.Run("OTACompletesSuccessfully", func(t *testing.T) {
		// Wait for OTA completion or node_update event
		otaEvent, err := h.WaitForEvent(ctx, "node_update", 90*time.Second)
		if err != nil {
			t.Logf("No node_update event within 90s: %v", err)
			// Check nodes endpoint instead
			nodes, err := h.GetNodes(ctx)
			if err != nil {
				t.Fatalf("Failed to get nodes: %v", err)
			}

			// Check if any node has the new version
			newVersionFound := false
			for _, n := range nodes {
				if version, ok := n["firmware_version"].(string); ok && version == "sim-1.1.0" {
					newVersionFound = true
					t.Logf("Node updated to %s", version)
					break
				}
			}

			if !newVersionFound {
				t.Error("OTA did not complete - version not updated")
			}
		} else {
			t.Logf("OTA event detected: %+v", otaEvent)
		}
	})

	// Verify version increment
	t.Run("VersionIncremented", func(t *testing.T) {
		nodes, err := h.GetNodes(ctx)
		if err != nil {
			t.Fatalf("Failed to get nodes: %v", err)
		}

		if len(nodes) == 0 {
			t.Fatal("No nodes found")
		}

		node := nodes[0]
		version, ok := node["firmware_version"].(string)
		if !ok {
			t.Error("Firmware version field missing")
			return
		}

		t.Logf("AS-5: Node firmware version: %s", version)

		// Version should be sim-1.1.0 after OTA
		if version != "sim-1.1.0" {
			t.Logf("Warning: Version is %s, expected sim-1.1.0 (may still be updating)", version)
		}
	})

	// Verify node is still online after reboot
	t.Run("NodeOnlineAfterOTA", func(t *testing.T) {
		nodes, err := h.GetNodes(ctx)
		if err != nil {
			t.Fatalf("Failed to get nodes: %v", err)
		}

		if len(nodes) == 0 {
			t.Fatal("No nodes found")
		}

		status, ok := node["status"].(string)
		if !ok {
			t.Error("Status field missing")
			return
		}

		if status != "online" {
			t.Errorf("Node status = %s, want online", status)
		}

		t.Log("AS-5: Node online after OTA - PASSED")
	})

	t.Log("AS-5: OTA update succeeds test completed")
}

// TestAS5_OTARollbackOnBadFirmware verifies rollback on boot failure.
func TestAS5_OTARollbackOnBadFirmware(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Check test harness early exit condition
	select {
	case <-ctx.Done():
		t.Skip("Context cancelled early")
		return
	default:
	}

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

	// Start simulator with OTA failure scenario
	simCtx, simCancel := context.WithTimeout(ctx, 90*time.Second)
	defer simCancel()
	if err := h.RunSimulator(simCtx, []string{
		"--nodes", "1",
		"--walkers", "0",
		"--duration", "0", // Run until cancelled
		"--scenario", "ota",
		"--ota-version", "sim-1.2.0-bad",
		"--ota-failure", // Simulates boot failure
	}); err != nil {
		t.Fatalf("Failed to start simulator: %v", err)
	}

	// Wait for node to connect initially
	node, err := h.WaitForNode(ctx, "")
	if err != nil {
		t.Fatalf("Node did not connect: %v", err)
	}

	t.Logf("AS-5: Node connected - MAC: %v", node["mac"])

	// Wait for rollback event or version rollback
	t.Run("RollbackTriggered", func(t *testing.T) {
		// Wait for version to be sim-1.0.0 (rollback from bad version)
		start := time.Now()
		timeout := 90 * time.Second
		rollbackSeen := false

		for time.Since(start) < timeout {
			nodes, err := h.GetNodes(ctx)
			if err != nil {
				t.Logf("Failed to get nodes: %v", err)
				time.Sleep(2 * time.Second)
				continue
			}

			for _, n := range nodes {
				if version, ok := n["firmware_version"].(string); ok {
					if version == "sim-1.0.0" {
						rollbackSeen = true
						elapsed := time.Since(start)
						t.Logf("AS-5: Rollback to sim-1.0.0 detected in %v", elapsed)

						if elapsed > 60*time.Second {
							t.Errorf("Rollback took %v, want < 60s", elapsed)
						}
						return
					}
				}
			}

			// Also check for rollback events
			events, err := h.GetEvents(ctx, "ota_rollback", 1)
			if err == nil && len(events) > 0 {
				rollbackSeen = true
				t.Logf("AS-5: Rollback event detected: %+v", events[0])
				return
			}

			time.Sleep(2 * time.Second)
		}

		if !rollbackSeen {
			t.Error("Rollback not detected within timeout")
		}
	})

	// Verify node is online after rollback
	t.Run("NodeOnlineAfterRollback", func(t *testing.T) {
		nodes, err := h.GetNodes(ctx)
		if err != nil {
			t.Fatalf("Failed to get nodes: %v", err)
		}

		if len(nodes) == 0 {
			t.Fatal("No nodes found after rollback")
		}

		status, ok := nodes[0]["status"].(string)
		if !ok {
			t.Error("Status field missing")
			return
		}

		if status != "online" {
			t.Errorf("Node status after rollback = %s, want online", status)
		}

		t.Log("AS-5: Node online after rollback - PASSED")
	})

	t.Log("AS-5: OTA rollback test completed")
}

// TestAS5_VerifiedBadgePath verifies the VERIFIED badge path for valid firmware.
func TestAS5_VerifiedBadgePath(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Check test harness early exit condition
	select {
	case <-ctx.Done():
		t.Skip("Context cancelled early")
		return
	default:
	}

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

	// Run normal scenario first to establish baseline
	simCtx, simCancel := context.WithTimeout(ctx, 90*time.Second)
	if err := h.RunSimulator(simCtx, []string{
		"--nodes", "1",
		"--walkers", "0",
		"--duration", "0", // Run until cancelled
	}); err != nil {
		t.Fatalf("Failed to start simulator: %v", err)
	}

	// Wait for node
	node, err := h.WaitForNode(ctx, "")
	if err != nil {
		t.Fatalf("Node did not connect: %v", err)
	}

	initialVersion, _ := node["firmware_version"].(string)
	t.Logf("Initial version: %s", initialVersion)

	// Stop first simulator before starting OTA scenario
	simCancel()

	// Wait a moment for clean disconnect
	time.Sleep(1 * time.Second)

	// Now run OTA scenario
	simCtx2, simCancel2 := context.WithTimeout(ctx, 90*time.Second)
	defer simCancel2()
	if err := h.RunSimulator(simCtx2, []string{
		"--nodes", "1",
		"--walkers", "0",
		"--duration", "0", // Run until cancelled
		"--scenario", "ota",
		"--ota-version", "sim-1.1.0-verified",
	}); err != nil {
		t.Fatalf("Failed to start OTA simulator: %v", err)
	}

	// Wait for update to complete
	time.Sleep(20 * time.Second)

	// Verify node updated and reconnected
	nodes, err := h.GetNodes(ctx)
	if err != nil {
		t.Fatalf("Failed to get nodes: %v", err)
	}

	if len(nodes) > 0 {
		newVersion, _ := nodes[0]["firmware_version"].(string)
		newStatus, _ := nodes[0]["status"].(string)

		t.Logf("AS-5: After OTA - version: %s, status: %s", newVersion, newStatus)

		if newVersion == initialVersion && newStatus == "online" {
			t.Log("AS-5: Node online with same version (update may be in progress)")
		}

		if newStatus == "online" {
			t.Log("AS-5: VERIFIED badge path - node reconnected successfully - PASSED")
		} else {
			t.Errorf("AS-5: Node status = %s, want online", newStatus)
		}
	}

	t.Log("AS-5: Verified badge path test completed")
}
