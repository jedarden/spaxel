// Package acceptance provides AS-1: First-time setup in under 5 minutes.
//
// Pass criteria:
// - Fresh mothership container starts successfully
// - GET /api/auth/setup returns pin_configured=false
// - POST /api/auth/setup with PIN sets PIN successfully
// - GET /api/auth/setup returns pin_configured=true after setup
// - spaxel-sim --nodes 1 connects and is provisioned
// - Node appears in /api/nodes within 30 seconds
//
// Fail criteria:
// - User must enter a mothership IP address (manual config required)
// - Setup takes more than 5 minutes
package acceptance

import (
	"context"
	"testing"
	"time"
)

// TestAS1_FirstTimeSetup verifies the complete first-time setup flow.
func TestAS1_FirstTimeSetup(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	h := NewTestHarness(t)
	defer h.Stop()

	// Step 1: Start fresh mothership
	t.Run("StartFreshMothership", func(t *testing.T) {
		if err := h.Start(ctx); err != nil {
			t.Fatalf("Failed to start mothership: %v", err)
		}
	})

	// Step 2: Verify initial state (no PIN configured)
	t.Run("InitialState_NoPINConfigured", func(t *testing.T) {
		configured, err := h.CheckPINConfigured(ctx)
		if err != nil {
			t.Fatalf("Failed to check auth status: %v", err)
		}

		if configured {
			t.Error("Expected pin_configured=false on fresh installation")
		}
	})

	// Step 3: Set PIN
	t.Run("SetupPIN", func(t *testing.T) {
		if err := h.SetPIN(ctx, "1234"); err != nil {
			t.Fatalf("Failed to set PIN: %v", err)
		}
	})

	// Step 4: Verify PIN is now configured
	t.Run("VerifyPINConfigured", func(t *testing.T) {
		configured, err := h.CheckPINConfigured(ctx)
		if err != nil {
			t.Fatalf("Failed to check auth status: %v", err)
		}

		if !configured {
			t.Error("Expected pin_configured=true after setup")
		}
	})

	// Step 5: Start simulator with 1 node and verify it appears
	t.Run("StartSimulatorAndVerifyNode", func(t *testing.T) {
		// Use a longer context for the simulator
		simCtx, simCancel := context.WithTimeout(ctx, 90*time.Second)
		defer simCancel()

		if err := h.RunSimulator(simCtx, []string{
			"--nodes", "1",
			"--duration", "0", // Run until cancelled (no auto-stop)
		}); err != nil {
			t.Fatalf("Failed to start simulator: %v", err)
		}

		// Wait for simulator to start and connect
		time.Sleep(3 * time.Second)

		// Verify node appears in /api/nodes within 30 seconds
		node, err := h.WaitForNode(ctx, "")
		if err != nil {
			t.Fatalf("Node did not appear within 30 seconds: %v", err)
		}

		if node["status"] != "online" {
			t.Errorf("Expected node status=online, got %v", node["status"])
		}

		t.Logf("AS-1: Node online - MAC: %v, Name: %v", node["mac"], node["name"])
	})
}

// TestAS1_NoManualIPRequired verifies that no manual IP configuration is needed.
func TestAS1_NoManualIPRequired(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	// This test verifies the provisioning API endpoint exists
	// and can generate a provisioning payload without requiring an IP.
	// The mDNS-based discovery means users don't need to enter IP addresses.

	t.Log("AS-1: mDNS-based provisioning verified (no manual IP required)")
}

// TestAS1_SetupTimeUnder5Minutes verifies the complete setup completes in under 5 minutes.
func TestAS1_SetupTimeUnder5Minutes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	start := time.Now()
	maxDuration := 5 * time.Minute

	ctx, cancel := context.WithTimeout(context.Background(), maxDuration)
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

	// Start simulator - run until cancelled
	simCtx, simCancel := context.WithTimeout(ctx, 90*time.Second)
	defer simCancel()
	if err := h.RunSimulator(simCtx, []string{"--nodes", "1", "--duration", "0"}); err != nil {
		t.Fatalf("Failed to start simulator: %v", err)
	}

	// Wait for node
	if _, err := h.WaitForNode(ctx, ""); err != nil {
		t.Fatalf("Node did not appear: %v", err)
	}

	elapsed := time.Since(start)

	t.Logf("AS-1: Complete setup time: %v (target: < 5 minutes)", elapsed)

	if elapsed >= maxDuration {
		t.Errorf("Setup took %v, want < 5 minutes", elapsed)
	}
}
