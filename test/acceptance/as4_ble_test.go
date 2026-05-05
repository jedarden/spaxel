// Package acceptance provides AS-4: BLE identity resolves to person name.
//
// Pass criteria:
// - BLE device registered via /api/ble/devices
// - spaxel-sim --ble runs with BLE advertisements
// - Blob appears with person="Alice" within 15 seconds
// - Identity persists across multiple detections
//
// Fail criteria:
// - BLE identity not resolved
// - Identity takes > 15 seconds to resolve
// - Identity not consistent across detections
package acceptance

import (
	"context"
	"testing"
	"time"
)

// TestAS4_BLEIdentityResolution verifies BLE identity matching.
func TestAS4_BLEIdentityResolution(t *testing.T) {
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

	// Set PIN
	if err := h.SetPIN(ctx, "1234"); err != nil {
		t.Fatalf("Failed to set PIN: %v", err)
	}

	// Register BLE device for "Alice"
	t.Run("RegisterBLEDevice", func(t *testing.T) {
		aliceDevice := map[string]interface{}{
			"addr":  "AA:BB:CC:DD:EE:FF",
			"label": "Alice",
			"type":  "person",
			"color": "#4488ff",
		}

		if err := h.RegisterBLEDevice(ctx, aliceDevice); err != nil {
			t.Fatalf("Failed to register BLE device: %v", err)
		}

		t.Log("AS-4: BLE device registered for Alice")
	})

	// Run simulator with BLE enabled
	simCtx, simCancel := context.WithTimeout(ctx, 2*time.Minute)
	if err := h.RunSimulator(simCtx, []string{
		"--nodes", "2",
		"--walkers", "1",
		"--duration", "30",
		"--ble",
	}); err != nil {
		t.Fatalf("Failed to start simulator: %v", err)
	}
	defer simCancel()

	// Wait for identity matching
	t.Run("IdentityResolved", func(t *testing.T) {
		start := time.Now()
		identityFound := false
		timeout := 20 * time.Second

		for time.Since(start) < timeout {
			blobs, err := h.GetBlobs(ctx)
			if err != nil {
				t.Logf("Failed to get blobs: %v", err)
				time.Sleep(500 * time.Millisecond)
				continue
			}

			for _, blob := range blobs {
				if person, ok := blob["person"].(string); ok && person == "Alice" {
					identityFound = true
					elapsed := time.Since(start)

					t.Logf("AS-4: Alice identity resolved within %v", elapsed)

					if elapsed > 15*time.Second {
						t.Errorf("Identity took %v, want < 15s", elapsed)
					}
					break
				}
			}

			if identityFound {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}

		if !identityFound {
			t.Error("Alice identity not resolved within timeout")
		}
	})

	// Verify identity persists
	t.Run("IdentityPersists", func(t *testing.T) {
		// Wait a bit more and check again
		time.Sleep(5 * time.Second)

		blobs, err := h.GetBlobs(ctx)
		if err != nil {
			t.Fatalf("Failed to get blobs: %v", err)
		}

		aliceFound := false
		for _, blob := range blobs {
			if person, ok := blob["person"].(string); ok && person == "Alice" {
				aliceFound = true
				break
			}
		}

		if !aliceFound {
			t.Error("Alice identity not found in subsequent check")
		} else {
			t.Log("AS-4: Identity persists across detections")
		}
	})

	t.Log("AS-4: BLE identity resolution test completed")
}

// TestAS4_MultipleBLEIdentities verifies multiple BLE identities can be resolved.
func TestAS4_MultipleBLEIdentities(t *testing.T) {
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

	// Set PIN
	if err := h.SetPIN(ctx, "1234"); err != nil {
		t.Fatalf("Failed to set PIN: %v", err)
	}

	// Register multiple BLE devices
	devices := []map[string]interface{}{
		{
			"addr":  "AA:BB:CC:DD:EE:01",
			"label": "Alice",
			"type":  "person",
			"color": "#4488ff",
		},
		{
			"addr":  "AA:BB:CC:DD:EE:02",
			"label": "Bob",
			"type":  "person",
			"color": "#44ff88",
		},
	}

	for _, device := range devices {
		if err := h.RegisterBLEDevice(ctx, device); err != nil {
			t.Fatalf("Failed to register BLE device: %v", err)
		}
	}

	t.Log("AS-4: Multiple BLE devices registered")

	// Run simulator with BLE and 2 walkers
	simCtx, simCancel := context.WithTimeout(ctx, 2*time.Minute)
	if err := h.RunSimulator(simCtx, []string{
		"--nodes", "2",
		"--walkers", "2",
		"--duration", "30",
		"--ble",
	}); err != nil {
		t.Fatalf("Failed to start simulator: %v", err)
	}
	defer simCancel()

	// Wait for identities to be resolved
	time.Sleep(15 * time.Second)

	// Check for both identities
	blobs, err := h.GetBlobs(ctx)
	if err != nil {
		t.Fatalf("Failed to get blobs: %v", err)
	}

	foundPersons := make(map[string]bool)
	for _, blob := range blobs {
		if person, ok := blob["person"].(string); ok {
			foundPersons[person] = true
		}
	}

	if !foundPersons["Alice"] {
		t.Error("Alice identity not found")
	}
	if !foundPersons["Bob"] {
		t.Error("Bob identity not found")
	}

	if foundPersons["Alice"] && foundPersons["Bob"] {
		t.Log("AS-4: Both BLE identities resolved successfully")
	}
}
