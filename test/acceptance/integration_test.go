// Package acceptance provides the integration test that runs all acceptance scenarios.
//
// This test serves as the entry point for the Argo CI workflow that verifies
// all acceptance scenarios pass using spaxel-sim as the test harness.
//
// To run this test:
//   ACCEPTANCE_TEST=1 go test -v ./test/acceptance/... -run TestIntegration
//
// Tests require:
// - The mothership binary to be built and available
// - The spaxel-sim binary to be built and in PATH
package acceptance

import (
	"context"
	"testing"
	"time"
)

// TestIntegration runs all acceptance scenarios in sequence to verify the system
// meets all its acceptance criteria.
//
// This test serves as the main integration test entry point for CI/CD pipelines.
// Each scenario is tested in its own sub-test for clear failure reporting.
func TestIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Overall test timeout - 15 minutes should be enough for all scenarios
	_, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	// Run all acceptance scenarios
	t.Run("AS1_FirstTimeSetup", func(t *testing.T) {
		TestAS1_FirstTimeSetup(t)
	})

	t.Run("AS2_WalkingDetection", func(t *testing.T) {
		TestAS2_WalkingDetection(t)
	})

	t.Run("AS2_BlobCountMatchesWalkers", func(t *testing.T) {
		TestAS2_BlobCountMatchesWalkers(t)
	})

	t.Run("AS3_FallDetection", func(t *testing.T) {
		TestAS3_FallDetection(t)
	})

	t.Run("AS3_FallAlertSeverity", func(t *testing.T) {
		TestAS3_FallAlertSeverity(t)
	})

	t.Run("AS4_BLEIdentityResolution", func(t *testing.T) {
		TestAS4_BLEIdentityResolution(t)
	})

	t.Run("AS4_MultipleBLEIdentities", func(t *testing.T) {
		TestAS4_MultipleBLEIdentities(t)
	})

	t.Run("AS5_OTAUpdateSucceeds", func(t *testing.T) {
		TestAS5_OTAUpdateSucceeds(t)
	})

	t.Run("AS5_OTARollbackOnBadFirmware", func(t *testing.T) {
		TestAS5_OTARollbackOnBadFirmware(t)
	})

	t.Run("AS5_VerifiedBadgePath", func(t *testing.T) {
		TestAS5_VerifiedBadgePath(t)
	})

	t.Run("AS6_ReplayShowsRecordedHistory", func(t *testing.T) {
		TestAS6_ReplayShowsRecordedHistory(t)
	})

	t.Run("AS6_ReplayBlobsWithFlag", func(t *testing.T) {
		TestAS6_ReplayBlobsWithFlag(t)
	})

	t.Run("AS6_SeekReplay", func(t *testing.T) {
		TestAS6_SeekReplay(t)
	})

	t.Run("AS6_BackToLive", func(t *testing.T) {
		TestAS6_BackToLive(t)
	})

	t.Run("AS6_Replay30SecondWindow", func(t *testing.T) {
		TestAS6_Replay30SecondWindow(t)
	})

	t.Log("All acceptance scenarios completed")
}

// TestIntegrationQuick runs a quick subset of acceptance scenarios for faster CI feedback.
//
// This test is useful for getting faster feedback during development while still
// verifying the core functionality works end-to-end.
func TestIntegrationQuick(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	_, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	// Run a quick subset of critical scenarios
	t.Run("AS1_FirstTimeSetup", func(t *testing.T) {
		TestAS1_FirstTimeSetup(t)
	})

	t.Run("AS2_WalkingDetection", func(t *testing.T) {
		TestAS2_WalkingDetection(t)
	})

	t.Run("AS4_BLEIdentityResolution", func(t *testing.T) {
		TestAS4_BLEIdentityResolution(t)
	})

	t.Log("Quick integration test completed")
}
