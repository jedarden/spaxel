// Package acceptance provides AS-7: Auth rejection test.
//
// Pass criteria:
// - Mothership rejects nodes without valid tokens
// - Simulator exits non-zero on reject message
// - Mothership logs the rejection
//
// Fail criteria:
// - Node connects without valid token
// - Simulator does not exit on rejection
package acceptance

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestAS7_AuthRejectMissingToken verifies that nodes without a token are rejected.
func TestAS7_AuthRejectMissingToken(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
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

	// Start mothership with auth enabled
	// Set SPAXEL_INSTALL_SECRET to enable token validation
	h.MothershipCmd.Env = append(h.MothershipCmd.Env,
		"SPAXEL_INSTALL_SECRET=test-secret")

	if err := h.Start(ctx); err != nil {
		t.Fatalf("Failed to start mothership: %v", err)
	}

	// Set PIN (required for auth to be active)
	if err := h.SetPIN(ctx, "1234"); err != nil {
		t.Fatalf("Failed to set PIN: %v", err)
	}

	// Try to connect simulator without a token
	simCtx, simCancel := context.WithTimeout(ctx, 45*time.Second)
	defer simCancel()

	// Empty token should trigger rejection
	simArgs := []string{
		"--nodes", "1",
		"--walkers", "0",
		"--duration", "0",
		"--token", "", // Empty token = missing
	}

	simErr := make(chan error, 1)
	go func() {
		simErr <- h.RunSimulator(simCtx, simArgs)
	}()

	// Wait for simulator to exit (should exit quickly on rejection)
	select {
	case <-time.After(20 * time.Second):
		t.Fatal("Simulator did not exit within 20s (expected rejection)")
	case err := <-simErr:
		if err == nil {
			t.Error("Simulator exited with nil error, expected non-zero exit on rejection")
		} else {
			// Verify the error message indicates rejection
			if !strings.Contains(err.Error(), "rejected") &&
				!strings.Contains(err.Error(), "401") &&
				!strings.Contains(err.Error(), "invalid") {
				t.Errorf("Simulator error does not indicate rejection: %v", err)
			} else {
				t.Logf("AS-7: Simulator correctly rejected: %v", err)
			}
		}
	}

	// Verify mothership logged the rejection
	// The stderr buffer should contain rejection logs
	stderrStr := h.stderrBuf.String()
	if !strings.Contains(stderrStr, "rejected") &&
		!strings.Contains(stderrStr, "invalid token") &&
		!strings.Contains(stderrStr, "missing token") {
		t.Error("Mothership logs do not contain rejection message")
	} else {
		t.Log("AS-7: Mothership logged rejection")
	}

	// Verify no nodes are connected
	nodes, err := h.GetNodes(ctx)
	if err != nil {
		t.Fatalf("Failed to get nodes: %v", err)
	}
	if len(nodes) > 0 {
		t.Errorf("Expected no nodes connected, got %d", len(nodes))
	} else {
		t.Log("AS-7: No nodes connected after rejection - PASSED")
	}
}

// TestAS7_AuthRejectInvalidToken verifies that nodes with invalid tokens are rejected.
func TestAS7_AuthRejectInvalidToken(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
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

	// Start mothership with auth enabled
	h.MothershipCmd.Env = append(h.MothershipCmd.Env,
		"SPAXEL_INSTALL_SECRET=test-secret")

	if err := h.Start(ctx); err != nil {
		t.Fatalf("Failed to start mothership: %v", err)
	}

	// Set PIN
	if err := h.SetPIN(ctx, "1234"); err != nil {
		t.Fatalf("Failed to set PIN: %v", err)
	}

	// Try to connect simulator with an invalid token
	simCtx, simCancel := context.WithTimeout(ctx, 45*time.Second)
	defer simCancel()

	// Invalid token that won't match the HMAC-SHA256 derivation
	simArgs := []string{
		"--nodes", "1",
		"--walkers", "0",
		"--duration", "0",
		"--token", "invalid-token-12345",
	}

	simErr := make(chan error, 1)
	go func() {
		simErr <- h.RunSimulator(simCtx, simArgs)
	}()

	// Wait for simulator to exit (should exit quickly on rejection)
	select {
	case <-time.After(20 * time.Second):
		t.Fatal("Simulator did not exit within 20s (expected rejection)")
	case err := <-simErr:
		if err == nil {
			t.Error("Simulator exited with nil error, expected non-zero exit on rejection")
		} else {
			// Verify the error message indicates rejection
			if !strings.Contains(err.Error(), "rejected") &&
				!strings.Contains(err.Error(), "401") &&
				!strings.Contains(err.Error(), "invalid") {
				t.Errorf("Simulator error does not indicate rejection: %v", err)
			} else {
				t.Logf("AS-7: Simulator correctly rejected: %v", err)
			}
		}
	}

	// Verify mothership logged the rejection
	stderrStr := h.stderrBuf.String()
	if !strings.Contains(stderrStr, "rejected") &&
		!strings.Contains(stderrStr, "invalid token") {
		t.Error("Mothership logs do not contain rejection message")
	} else {
		t.Log("AS-7: Mothership logged rejection - PASSED")
	}

	// Verify no nodes are connected
	nodes, err := h.GetNodes(ctx)
	if err != nil {
		t.Fatalf("Failed to get nodes: %v", err)
	}
	if len(nodes) > 0 {
		t.Errorf("Expected no nodes connected, got %d", len(nodes))
	}
}

// TestAS7_AuthAcceptValidToken verifies that nodes with valid tokens are accepted.
func TestAS7_AuthAcceptValidToken(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
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

	// Start mothership with auth enabled
	// Use the same install secret the simulator uses: "sim-install-secret"
	// The simulator generates tokens as HMAC-SHA256("sim-install-secret", "sim-node")
	installSecret := "sim-install-secret"
	h.MothershipCmd.Env = append(h.MothershipCmd.Env,
		"SPAXEL_INSTALL_SECRET="+installSecret)

	if err := h.Start(ctx); err != nil {
		t.Fatalf("Failed to start mothership: %v", err)
	}

	// Set PIN
	if err := h.SetPIN(ctx, "1234"); err != nil {
		t.Fatalf("Failed to set PIN: %v", err)
	}

	// Connect simulator with a valid token
	// The simulator auto-generates a valid token when --token is empty
	simCtx, simCancel := context.WithTimeout(ctx, 45*time.Second)
	defer simCancel()

	simArgs := []string{
		"--nodes", "1",
		"--walkers", "0",
		"--duration", "30s",
		// Empty token lets simulator auto-generate using sim-install-secret
	}

	if err := h.RunSimulator(simCtx, simArgs); err != nil {
		t.Fatalf("Failed to start simulator with valid token: %v", err)
	}

	// Wait for node to connect (should succeed)
	node, err := h.WaitForNode(ctx, "")
	if err != nil {
		t.Fatalf("Node did not connect with valid token: %v", err)
	}

	t.Logf("AS-7: Node connected with valid token - MAC: %v", node["mac"])

	// Verify node is online
	status, ok := node["status"].(string)
	if !ok || status != "online" {
		t.Errorf("Node status = %v, want online", node["status"])
	} else {
		t.Log("AS-7: Node with valid token accepted and online - PASSED")
	}
}
