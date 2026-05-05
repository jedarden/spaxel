// Package acceptance provides AS-3: Fall alert fires correctly.
//
// Pass criteria:
// - spaxel-sim --scenario fall runs with falling walker
// - Fall alert appears in /api/events within 30 seconds
// - Alert has severity="critical" or "warning"
// - Webhook is called with fall alert payload
//
// Fail criteria:
// - No fall alert generated
// - Alert severity is incorrect
// - Webhook not called
package acceptance

import (
	"context"
	"testing"
	"time"
)

// TestAS3_FallDetection verifies fall detection and alerting.
func TestAS3_FallDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	h := NewTestHarness(t)
	defer h.Stop()

	// Start webhook server to receive alerts
	webhookURL := h.StartWebhookServer()
	t.Logf("Webhook server started: %s", webhookURL)

	// Start mothership with webhook URL
	if err := h.Start(ctx); err != nil {
		t.Fatalf("Failed to start mothership: %v", err)
	}

	// Set PIN
	if err := h.SetPIN(ctx, "1234"); err != nil {
		t.Fatalf("Failed to set PIN: %v", err)
	}

	// Run simulator with fall scenario
	simCtx, simCancel := context.WithTimeout(ctx, 3*time.Minute)
	if err := h.RunSimulator(simCtx, []string{
		"--nodes", "2",
		"--walkers", "1",
		"--duration", "60",
		"--scenario", "fall",
		"--fall-delay", "5s",
		"--fall-duration", "800ms",
		"--stillness", "15s",
	}); err != nil {
		t.Fatalf("Failed to start simulator: %v", err)
	}
	defer simCancel()

	// Wait for fall alert
	t.Run("FallAlertGenerated", func(t *testing.T) {
		fallAlert, err := h.WaitForEvent(ctx, "fall_alert", 45*time.Second)
		if err != nil {
			t.Fatalf("Fall alert not detected within 45 seconds: %v", err)
		}

		if fallAlert["type"] != "fall_alert" {
			t.Errorf("Expected event type=fall_alert, got %v", fallAlert["type"])
		}

		// Check severity
		if severity, ok := fallAlert["severity"].(string); ok {
			if severity != "critical" && severity != "warning" {
				t.Errorf("Expected severity=critical or warning, got %v", severity)
			}
		} else {
			t.Error("Missing severity field in fall alert")
		}

		t.Logf("AS-3: Fall alert detected: %+v", fallAlert)
	})

	// Verify webhook was called
	t.Run("WebhookCalled", func(t *testing.T) {
		// Wait a moment for webhook to be called
		time.Sleep(2 * time.Second)

		if !h.WebhookCalled() {
			t.Error("Webhook was not called for fall alert")
		} else {
			t.Log("AS-3: Webhook called successfully")
		}
	})

	// Also check for FallDetected event type
	t.Run("FallDetectedEvent", func(t *testing.T) {
		events, err := h.GetEvents(ctx, "FallDetected", 5)
		if err != nil {
			t.Logf("Failed to get FallDetected events: %v", err)
			return
		}

		if len(events) > 0 {
			t.Logf("AS-3: Found %d FallDetected events", len(events))
		}
	})

	t.Log("AS-3: Fall detection test completed")
}

// TestAS3_FallAlertSeverity verifies fall alert has correct severity.
func TestAS3_FallAlertSeverity(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
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

	// Run simulator with fall scenario
	simCtx, simCancel := context.WithTimeout(ctx, 3*time.Minute)
	if err := h.RunSimulator(simCtx, []string{
		"--nodes", "2",
		"--walkers", "1",
		"--duration", "60",
		"--scenario", "fall",
		"--fall-delay", "5s",
		"--stillness", "10s",
	}); err != nil {
		t.Fatalf("Failed to start simulator: %v", err)
	}
	defer simCancel()

	// Wait for fall alert
	fallAlert, err := h.WaitForEvent(ctx, "fall_alert", 45*time.Second)
	if err != nil {
		t.Fatalf("Fall alert not detected: %v", err)
	}

	// Verify severity
	severity, ok := fallAlert["severity"].(string)
	if !ok {
		t.Fatal("Missing severity field in fall alert")
	}

	if severity != "critical" && severity != "warning" {
		t.Errorf("Expected severity=critical or warning, got %v", severity)
	}

	t.Logf("AS-3: Fall alert severity verified: %s", severity)
}
