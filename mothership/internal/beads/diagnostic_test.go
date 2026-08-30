package beads

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDiagnosticReportEmptyDatabase tests the diagnostic report when no beads exist
func TestDiagnosticReportEmptyDatabase(t *testing.T) {
	// This test demonstrates the expected output structure
	report := &DiagnosticReport{
		Timestamp:          time.Now().Format(time.RFC3339),
		TotalBeads:         0,
		OpenBeads:          0,
		UnblockedBeads:     0,
		UnassignedBeads:    0,
		WithoutDependencies: 0,
		WithoutLockConflicts: 0,
		ReadyCandidates:    0,
		Recommendations:    []string{},
	}

	report.identifyBottleneck()

	if report.IdentifiedBottleneck != "empty_database" {
		t.Errorf("Expected bottleneck 'empty_database', got '%s'", report.IdentifiedBottleneck)
	}

	if len(report.Recommendations) == 0 {
		t.Error("Expected recommendations for empty database")
	}
}

// TestDiagnosticReportAllClosed tests when all beads are closed
func TestDiagnosticReportAllClosed(t *testing.T) {
	report := &DiagnosticReport{
		Timestamp:          time.Now().Format(time.RFC3339),
		TotalBeads:         100,
		OpenBeads:          0,
		UnblockedBeads:     0,
		UnassignedBeads:    0,
		WithoutDependencies: 0,
		WithoutLockConflicts: 0,
		ReadyCandidates:    0,
		Recommendations:    []string{},
	}

	report.identifyBottleneck()

	if report.IdentifiedBottleneck != "all_beads_closed" {
		t.Errorf("Expected bottleneck 'all_beads_closed', got '%s'", report.IdentifiedBottleneck)
	}
}

// TestDiagnosticReportDependencyBlock tests when all beads are blocked by dependencies
func TestDiagnosticReportDependencyBlock(t *testing.T) {
	report := &DiagnosticReport{
		Timestamp:          time.Now().Format(time.RFC3339),
		TotalBeads:         100,
		OpenBeads:          100,
		UnblockedBeads:     100,
		UnassignedBeads:    100,
		WithoutDependencies: 0,
		WithoutLockConflicts: 0,
		ReadyCandidates:    0,
		Recommendations:    []string{},
	}

	report.identifyBottleneck()

	if report.IdentifiedBottleneck != "all_beads_blocked_by_dependencies" {
		t.Errorf("Expected bottleneck 'all_beads_blocked_by_dependencies', got '%s'", report.IdentifiedBottleneck)
	}

	// Should recommend auto-remediation
	if !report.ShouldCreateRemediationBead() {
		t.Error("Expected auto-remediation bead to be created for dependency bottleneck")
	}

	title := report.GenerateRemediationBeadTitle()
	if title == "" {
		t.Error("Expected non-empty remediation bead title")
	}
}

// TestDiagnosticReportAssigneeBlock tests when all beads have assignees
func TestDiagnosticReportAssigneeBlock(t *testing.T) {
	report := &DiagnosticReport{
		Timestamp:          time.Now().Format(time.RFC3339),
		TotalBeads:         100,
		OpenBeads:          100,
		UnblockedBeads:     100,
		UnassignedBeads:    0,
		WithoutDependencies: 0,
		WithoutLockConflicts: 0,
		ReadyCandidates:    0,
		Recommendations:    []string{},
	}

	report.identifyBottleneck()

	if report.IdentifiedBottleneck != "all_beads_assigned" {
		t.Errorf("Expected bottleneck 'all_beads_assigned', got '%s'", report.IdentifiedBottleneck)
	}

	// Should recommend auto-remediation
	if !report.ShouldCreateRemediationBead() {
		t.Error("Expected auto-remediation bead to be created for assignee bottleneck")
	}
}

// TestDiagnosticReportManualBlock tests when all beads are manually blocked
func TestDiagnosticReportManualBlock(t *testing.T) {
	report := &DiagnosticReport{
		Timestamp:          time.Now().Format(time.RFC3339),
		TotalBeads:         100,
		OpenBeads:          100,
		UnblockedBeads:     0,
		UnassignedBeads:    0,
		WithoutDependencies: 0,
		WithoutLockConflicts: 0,
		ReadyCandidates:    0,
		Recommendations:    []string{},
	}

	report.identifyBottleneck()

	if report.IdentifiedBottleneck != "all_beads_manually_blocked" {
		t.Errorf("Expected bottleneck 'all_beads_manually_blocked', got '%s'", report.IdentifiedBottleneck)
	}

	// Should NOT recommend auto-remediation (may need human review)
	if report.ShouldCreateRemediationBead() {
		t.Error("Expected no auto-remediation bead for manual block bottleneck")
	}
}

// TestDiagnosticReportSerialization tests JSON serialization
func TestDiagnosticReportSerialization(t *testing.T) {
	report := &DiagnosticReport{
		Timestamp:          "2026-08-30T12:34:56Z",
		TotalBeads:         100,
		OpenBeads:          80,
		UnblockedBeads:     70,
		UnassignedBeads:    60,
		WithoutDependencies: 40,
		WithoutLockConflicts: 10,
		ReadyCandidates:    10,
		BlockedByStatus:    20,
		BlockedByManualBlock: 10,
		BlockedByAssignee:  10,
		BlockedByDependency: 20,
		BlockedByLockConflict: 30,
		IdentifiedBottleneck: "all_beads_blocked_by_lock_conflicts",
		Recommendations:    []string{"Check resource allocation", "Review lease expiration"},
	}

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("Failed to marshal report: %v", err)
	}

	var unmarshaled DiagnosticReport
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal report: %v", err)
	}

	if unmarshaled.TotalBeads != report.TotalBeads {
		t.Errorf("TotalBeads mismatch: got %d, want %d", unmarshaled.TotalBeads, report.TotalBeads)
	}

	if len(unmarshaled.Recommendations) != 2 {
		t.Errorf("Recommendations count mismatch: got %d, want 2", len(unmarshaled.Recommendations))
	}
}

// TestWriteDiagnosticLog creates a temporary log file
func TestWriteDiagnosticLog(t *testing.T) {
	// Create temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "bead-diagnostic-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	report := &DiagnosticReport{
		Timestamp:          "2026-08-30T12:34:56Z",
		TotalBeads:         10,
		OpenBeads:          5,
		UnblockedBeads:     5,
		UnassignedBeads:    5,
		WithoutDependencies: 0,
		WithoutLockConflicts: 0,
		ReadyCandidates:    0,
		IdentifiedBottleneck: "all_beads_blocked_by_dependencies",
		Recommendations:    []string{"Review dependency graph"},
	}

	if err := WriteDiagnosticLog(report, tmpDir); err != nil {
		t.Fatalf("Failed to write diagnostic log: %v", err)
	}

	// Check that the JSON log file was created
	diagnosticsDir := filepath.Join(tmpDir, ".beads", "diagnostics")
	files, err := os.ReadDir(diagnosticsDir)
	if err != nil {
		t.Fatalf("Failed to read diagnostics directory: %v", err)
	}

	jsonFiles := 0
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".json") {
			jsonFiles++
		}
	}

	if jsonFiles != 1 {
		t.Errorf("Expected 1 JSON log file, found %d", jsonFiles)
	}

	// Check that the summary log was created
	summaryPath := filepath.Join(diagnosticsDir, "pluck-starvation-summary.log")
	if _, err := os.Stat(summaryPath); os.IsNotExist(err) {
		t.Error("Summary log file was not created")
	}
}

// BenchmarkDiagnostics performance test
func BenchmarkRunDiagnostics(b *testing.B) {
	// This would require a test database setup
	// For now, just benchmark the report processing
	report := &DiagnosticReport{
		Timestamp:          time.Now().Format(time.RFC3339),
		TotalBeads:         1000,
		OpenBeads:          500,
		UnblockedBeads:     400,
		UnassignedBeads:    300,
		WithoutDependencies: 100,
		WithoutLockConflicts: 50,
		ReadyCandidates:    50,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		report.identifyBottleneck()
		_ = report.GenerateRemediationBeadTitle()
		_ = report.GenerateRemediationBeadBody()
	}
}
