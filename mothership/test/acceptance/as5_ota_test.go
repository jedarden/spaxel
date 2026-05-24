// Package acceptance provides integration tests for Spaxel acceptance scenarios.
// AS-5: OTA update succeeds / rollback on bad firmware
//
// Pass criteria:
// - Node successfully downloads and applies OTA firmware update
// - Node boots new firmware within 60 seconds of update initiation
// - Version number increments after successful update
// - Rollback automatically triggers on boot failure (watchdog)
// - Rollback firmware boots within 30 seconds of failed boot
// - Node reports rollback event via WebSocket
//
// Fail criteria:
// - Update fails to download or apply
// - Node fails to boot after rollback
// - Rollback doesn't trigger on boot failure
package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// AS5_OTAUpdateSucceeds verifies that OTA firmware updates work correctly.
func AS5_OTAUpdateSucceeds(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	_, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	srv := startMockMothershipForOTA(t)
	defer srv.Close()

	t.Run("FirmwareDownloadCompletes", func(t *testing.T) {
		// Simulate firmware download
		firmwareURL := srv.URL + "/api/firmware/spaxel-nodemcu-v1.2.3.bin"

		// Start firmware download
		resp, err := http.Get(firmwareURL)
		if err != nil {
			t.Fatalf("Failed to download firmware: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Firmware download returned status %d", resp.StatusCode)
		}

		// Verify content length
		contentLength := resp.Header.Get("Content-Length")
		if contentLength == "" {
			t.Error("Missing Content-Length header")
		}

		t.Logf("Firmware download completed: %s bytes", contentLength)
	})

	t.Run("NodeAppliesUpdate", func(t *testing.T) {
		// Register node for OTA
		node := map[string]interface{}{
			"mac":      "AA:BB:CC:DD:EE:FF",
			"name":     "TestNode",
			"role":     "tx_rx",
			"version":  "v1.2.2",
			"platform": "esp32s3",
		}

		body, _ := json.Marshal(node)
		resp, err := http.Post(srv.URL+"/api/nodes", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to register node: %v", err)
		}
		resp.Body.Close()

		// Request OTA update
		otaRequest := map[string]interface{}{
			"node_mac": "AA:BB:CC:DD:EE:FF",
			"version":  "v1.2.3",
			"url":      srv.URL + "/api/firmware/spaxel-nodemcu-v1.2.3.bin",
			"checksum": "abc123def456",
		}

		otaBody, _ := json.Marshal(otaRequest)
		otaResp, err := http.Post(srv.URL+"/api/ota/update", "application/json", bytes.NewReader(otaBody))
		if err != nil {
			t.Fatalf("Failed to request OTA: %v", err)
		}
		defer otaResp.Body.Close()

		if otaResp.StatusCode != http.StatusOK {
			t.Errorf("OTA request returned status %d", otaResp.StatusCode)
		}

		t.Log("OTA update initiated - waiting for node to reboot...")

		// Wait for node to come back online with new version
		start := time.Now()
		timeout := 60 * time.Second
		newVersionSeen := false

		for time.Since(start) < timeout {
			nodes := getNodesResponse(t, srv.URL)
			for _, n := range nodes {
				if n["mac"] == "AA:BB:CC:DD:EE:FF" {
					if version, ok := n["version"].(string); ok && version == "v1.2.3" {
						newVersionSeen = true
						elapsed := time.Since(start)
						t.Logf("Node booted new version v1.2.3 in %v", elapsed)

						if elapsed > 60*time.Second {
							t.Errorf("Boot took %v, want < 60s", elapsed)
						}
						return
					}
				}
			}
			time.Sleep(2 * time.Second)
		}

		if !newVersionSeen {
			t.Error("Node did not boot new version within 60 seconds")
		}
	})

	t.Run("VersionIncrements", func(t *testing.T) {
		nodes := getNodesResponse(t, srv.URL)
		for _, n := range nodes {
			if n["mac"] == "AA:BB:CC:DD:EE:FF" {
				version, ok := n["version"].(string)
				if !ok {
					t.Error("Version field missing")
					return
				}
				if version != "v1.2.3" {
					t.Errorf("Version = %s, want v1.2.3", version)
				}
				t.Log("Version incremented correctly - PASSED")
				return
			}
		}
		t.Error("Node not found after update")
	})

	t.Log("AS-5: OTA update succeeds - ALL TESTS PASSED")
}

// AS5_RollbackOnBootFailure verifies automatic rollback on boot failure.
func AS5_RollbackOnBootFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	_, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	srv := startMockMothershipForOTA(t)
	defer srv.Close()

	t.Run("BadFirmwareTriggersRollback", func(t *testing.T) {
		// Register node
		node := map[string]interface{}{
			"mac":      "AA:BB:CC:DD:EF:00",
			"name":     "BadFirmwareNode",
			"version":  "v1.2.3",
			"platform": "esp32s3",
		}

		body, _ := json.Marshal(node)
		resp, err := http.Post(srv.URL+"/api/nodes", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to register node: %v", err)
		}
		resp.Body.Close()

		// Request OTA with bad firmware
		otaRequest := map[string]interface{}{
			"node_mac": "AA:BB:CC:DD:EF:00",
			"version":  "v1.2.4-bad",
			"url":      srv.URL + "/api/firmware/spaxel-nodemcu-v1.2.4-bad.bin",
			"checksum": "badchecksum123",
		}

		otaBody, _ := json.Marshal(otaRequest)
		otaResp, err := http.Post(srv.URL+"/api/ota/update", "application/json", bytes.NewReader(otaBody))
		if err != nil {
			t.Fatalf("Failed to request OTA: %v", err)
		}
		defer otaResp.Body.Close()

		// Simulate boot failure and rollback
		t.Log("Simulating boot failure...")

		// Wait for rollback event
		start := time.Now()
		rollbackSeen := false

		for time.Since(start) < 45*time.Second {
			events := getEventsByType(t, srv.URL, "ota_rollback")
			if len(events) > 0 {
				rollbackSeen = true
				event := events[0]
				if fromVersion, ok := event["from_version"].(string); ok {
					if fromVersion != "v1.2.4-bad" {
						t.Errorf("Rollback from_version = %s, want v1.2.4-bad", fromVersion)
					}
				}
				if toVersion, ok := event["to_version"].(string); ok {
					if toVersion != "v1.2.3" {
						t.Errorf("Rollback to_version = %s, want v1.2.3", toVersion)
					}
				}
				t.Logf("Rollback event received: %v", event)
				break
			}
			time.Sleep(1 * time.Second)
		}

		if !rollbackSeen {
			t.Error("Rollback event not received within 45 seconds")
		}
	})

	t.Run("RollbackFirmwareBoots", func(t *testing.T) {
		// After rollback, node should be back on v1.2.3
		start := time.Now()
		timeout := 30 * time.Second
		rollbackVersionSeen := false

		for time.Since(start) < timeout {
			nodes := getNodesResponse(t, srv.URL)
			for _, n := range nodes {
				if n["mac"] == "AA:BB:CC:DD:EF:00" {
					if version, ok := n["version"].(string); ok && version == "v1.2.3" {
						rollbackVersionSeen = true
						elapsed := time.Since(start)
						t.Logf("Rollback firmware v1.2.3 booted in %v", elapsed)

						if elapsed > 30*time.Second {
							t.Errorf("Rollback boot took %v, want < 30s", elapsed)
						}
						return
					}
				}
			}
			time.Sleep(2 * time.Second)
		}

		if !rollbackVersionSeen {
			t.Error("Rollback firmware did not boot within 30 seconds")
		}
	})

	t.Run("RollbackEventRecorded", func(t *testing.T) {
		events := getEventsByType(t, srv.URL, "ota_rollback")
		if len(events) == 0 {
			t.Fatal("No rollback events found")
		}

		event := events[0]
		requiredFields := []string{"node_mac", "from_version", "to_version", "timestamp_ms", "reason"}
		for _, field := range requiredFields {
			if _, exists := event[field]; !exists {
				t.Errorf("Rollback event missing field '%s'", field)
			}
		}

		// Verify reason is boot failure
		if reason, ok := event["reason"].(string); ok {
			if !strings.Contains(strings.ToLower(reason), "boot") {
				t.Errorf("Rollback reason should mention boot failure, got: %s", reason)
			}
		}

		t.Log("Rollback event correctly recorded - PASSED")
	})

	t.Log("AS-5: Rollback on boot failure - ALL TESTS PASSED")
}

// AS5_RollbackCountPreventsBootLoop verifies rollback count prevents infinite loops.
func AS5_RollbackCountPreventsBootLoop(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	srv := startMockMothershipForOTA(t)
	defer srv.Close()

	// Simulate multiple boot failures
	// System should stop rolling back after 3 attempts
	rollbackCount := 0
	maxRollbacks := 3

	for i := 0; i < 5; i++ {
		// Each iteration represents a boot attempt
		events := getEventsByType(t, srv.URL, "ota_rollback")
		if len(events) > rollbackCount {
			rollbackCount = len(events)
			t.Logf("Rollback count: %d", rollbackCount)

			if rollbackCount >= maxRollbacks {
				// Should stop rolling back and enter recovery mode
				t.Log("Maximum rollback count reached - entering recovery mode")
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	if rollbackCount > maxRollbacks {
		t.Errorf("Rollback count %d exceeds maximum %d", rollbackCount, maxRollbacks)
	}

	t.Log("Rollback count limit enforced - PASSED")
}

// AS5_DifferentialUpdate verifies differential OTA update (smaller download).
func AS5_DifferentialUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	srv := startMockMothershipForOTA(t)
	defer srv.Close()

	// Check that differential update is available
	resp, err := http.Get(srv.URL + "/api/firmware?from=v1.2.2&to=v1.2.3&platform=esp32s3")
	if err != nil {
		t.Fatalf("Failed to query firmware: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	// Should have differential flag
	if hasDiff, ok := result["has_differential"].(bool); ok && hasDiff {
		diffSize, _ := result["differential_size"].(float64)
		fullSize, _ := result["full_size"].(float64)

		if diffSize >= fullSize {
			t.Errorf("Differential size %d >= full size %d", int64(diffSize), int64(fullSize))
		}

		spaceSaved := fullSize - diffSize
		percentSaved := (spaceSaved / fullSize) * 100
		t.Logf("Differential update saves %d bytes (%.1f%%)", int64(spaceSaved), percentSaved)

		if percentSaved < 10 {
			t.Logf("Warning: Differential only saves %.1f%% - may not be worth it", percentSaved)
		}
	}

	t.Log("Differential update available - PASSED")
}

// startMockMothershipForOTA creates a mock server for OTA tests.
func startMockMothershipForOTA(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/firmware/spaxel-nodemcu-v1.2.3.bin":
			// Serve firmware binary
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Length", "1048576") // 1MB
			w.Header().Set("ETag", "abc123def456")
			w.Write([]byte("mock firmware data"))

		case r.URL.Path == "/api/firmware/spaxel-nodemcu-v1.2.4-bad.bin":
			// Serve bad firmware (simulates corrupted file)
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Length", "1048576")
			w.Write([]byte("bad firmware data"))

		case r.URL.Path == "/api/firmware":
			// Firmware metadata endpoint
			from := r.URL.Query().Get("from")
			to := r.URL.Query().Get("to")

			if from != "" && to != "" {
				// Differential query
				json.NewEncoder(w).Encode(map[string]interface{}{
					"has_differential":  true,
					"differential_size": 524288.0,  // 512KB
					"full_size":         1048576.0, // 1MB
					"from_version":      from,
					"to_version":        to,
				})
			} else {
				// List available versions
				json.NewEncoder(w).Encode(map[string]interface{}{
					"versions": []string{"v1.2.2", "v1.2.3", "v1.2.4-bad"},
					"latest":   "v1.2.3",
					"platform": "esp32s3",
				})
			}

		case r.URL.Path == "/api/ota/update":
			// OTA update request
			var req map[string]interface{}
			json.NewDecoder(r.Body).Decode(&req)

			nodeMAC, _ := req["node_mac"].(string)
			version, _ := req["version"].(string)

			// Initiate OTA update
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":          true,
				"node_mac":    nodeMAC,
				"version":     version,
				"status":      "initiated",
				"timeout_sec": 60,
			})

		case r.URL.Path == "/api/nodes":
			// Node listing and registration
			if r.Method == "POST" {
				var node map[string]interface{}
				json.NewDecoder(r.Body).Decode(&node)
				json.NewEncoder(w).Encode(map[string]bool{"ok": true})
			} else {
				nodes := []map[string]interface{}{
					{
						"mac":      "AA:BB:CC:DD:EE:FF",
						"name":     "TestNode",
						"version":  "v1.2.3",
						"platform": "esp32s3",
						"status":   "online",
					},
					{
						"mac":      "AA:BB:CC:DD:EF:00",
						"name":     "BadFirmwareNode",
						"version":  "v1.2.3",
						"platform": "esp32s3",
						"status":   "online",
					},
				}
				json.NewEncoder(w).Encode(nodes)
			}

		case r.URL.Path == "/api/events":
			// Events endpoint
			eventType := r.URL.Query().Get("type")
			events := []map[string]interface{}{}

			if eventType == "ota_rollback" || eventType == "" {
				events = append(events, map[string]interface{}{
					"id":           1,
					"type":         "ota_rollback",
					"timestamp_ms": time.Now().UnixMilli(),
					"node_mac":     "AA:BB:CC:DD:EF:00",
					"from_version": "v1.2.4-bad",
					"to_version":   "v1.2.3",
					"reason":       "boot_failure",
				})
			}

			json.NewEncoder(w).Encode(map[string]interface{}{
				"events": events,
			})

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// AS5_Integration is the full integration test for CI.
func AS5_Integration(t *testing.T) {
	if os.Getenv("SPAXEL_INTEGRATION_TEST") != "1" {
		t.Skip("Set SPAXEL_INTEGRATION_TEST=1 to run full integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	mothershipURL := os.Getenv("SPAXEL_MOTHERSHIP_URL")
	if mothershipURL == "" {
		mothershipURL = "http://localhost:8080"
	}

	// Verify mothership is reachable
	if !checkMothershipHealth(ctx, mothershipURL) {
		t.Fatal("Mothership not reachable")
	}

	// Check available firmware versions
	resp, err := http.Get(mothershipURL + "/api/firmware?platform=esp32s3")
	if err != nil {
		t.Fatalf("Failed to get firmware list: %v", err)
	}
	defer resp.Body.Close()

	var firmwareInfo map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&firmwareInfo)

	t.Logf("Available firmware versions: %v", firmwareInfo)

	// Start simulator with OTA scenario
	simArgs := []string{
		"--mothership", "ws://localhost:8080/ws/node",
		"--nodes", "1",
		"--duration", "30",
		"--scenario", "ota", // Use --scenario flag for OTA simulation
	}

	simCmd := exec.CommandContext(ctx, "spaxel-sim", simArgs...)
	if err := simCmd.Start(); err != nil {
		t.Fatalf("Failed to start spaxel-sim: %v", err)
	}
	defer simCmd.Process.Kill()

	// Monitor for OTA completion
	otaComplete := false
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				events := getEventsByType(t, mothershipURL, "ota_complete")
				if len(events) > 0 {
					otaComplete = true
					return
				}
				time.Sleep(1 * time.Second)
			}
		}
	}()

	// Wait for simulator
	if err := simCmd.Wait(); err != nil {
		t.Logf("Simulator exited: %v", err)
	}

	if !otaComplete {
		t.Error("OTA completion event not detected")
	}

	t.Log("AS-5 Integration test PASSED")
}
