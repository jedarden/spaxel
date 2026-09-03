package beads

import (
	"database/sql"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
)

// MonitoredPluck wraps the bead-rs Pluck command with automatic diagnostics
type MonitoredPluck struct {
	db                *sql.DB
	workspacePath     string
	enableDiagnostics bool
}

// NewMonitoredPluck creates a new monitored Pluck instance
func NewMonitoredPluck(db *sql.DB, workspacePath string) *MonitoredPluck {
	return &MonitoredPluck{
		db:                db,
		workspacePath:     workspacePath,
		enableDiagnostics: true,
	}
}

// PluckReady calls `bead pluck --ready` and runs diagnostics if zero candidates are returned
func (mp *MonitoredPluck) PluckReady(limit int) ([]string, error) {
	// Run the bead pluck command
	cmd := exec.Command("bead", "pluck", "--ready", "--limit", fmt.Sprintf("%d", limit))
	cmd.Dir = mp.workspacePath
	output, err := cmd.Output()

	if err != nil {
		return nil, fmt.Errorf("bead pluck failed: %w", err)
	}

	// Parse the output - bead IDs are newline-separated
	beadIDs := parseBeadIDs(output)

	// If zero candidates and diagnostics enabled, run diagnostics
	if len(beadIDs) == 0 && mp.enableDiagnostics {
		log.Printf("Pluck returned zero candidates - running diagnostics...")

		report, err := RunDiagnostics(mp.db, &PluckQueryParameters{
			Limit:            limit,
			CurrentTimestamp: getCurrentTimestamp(),
		})

		if err != nil {
			log.Printf("Failed to run diagnostics: %v", err)
		} else {
			// Write diagnostic log
			if err := WriteDiagnosticLog(report, mp.workspacePath); err != nil {
				log.Printf("Failed to write diagnostic log: %v", err)
			}

			// Log summary to stdout
			log.Printf("=== Pluck Starvation Diagnostic ===")
			log.Printf("Total beads: %d", report.TotalBeads)
			log.Printf("Ready candidates: %d", report.ReadyCandidates)
			log.Printf("Bottleneck: %s", report.IdentifiedBottleneck)

			// Check if we should create an auto-remediation bead
			if report.ShouldCreateRemediationBead() {
				if err := mp.createRemediationBead(report); err != nil {
					log.Printf("Failed to create remediation bead: %v", err)
				}
			}
		}
	}

	return beadIDs, nil
}

// parseBeadIDs parses bead IDs from command output
func parseBeadIDs(output []byte) []string {
	lines := strings.Split(string(output), "\n")
	var beadIDs []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "INFO") && !strings.HasPrefix(line, "WARN") && !strings.HasPrefix(line, "ERROR") {
			beadIDs = append(beadIDs, line)
		}
	}

	return beadIDs
}

// createRemediationBead creates an auto-remediation bead based on the diagnostic report
func (mp *MonitoredPluck) createRemediationBead(report *DiagnosticReport) error {
	title := report.GenerateRemediationBeadTitle()
	body := report.GenerateRemediationBeadBody()

	// Use the bead CLI to create the bead
	// Priority: 2 (medium) - not urgent but important for fleet health
	cmd := exec.Command("bead", "create",
		"--title", title,
		"--priority", "2",
		"--issue-type", "task",
		"--notes", body,
		"--label", "auto-remediation",
		"--label", "pluck-starvation",
	)
	cmd.Dir = mp.workspacePath

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create remediation bead: %w: %s", err, string(output))
	}

	log.Printf("Created auto-remediation bead: %s", title)
	return nil
}

// getCurrentTimestamp returns the current time in RFC3339 format
func getCurrentTimestamp() time.Time {
	return time.Now()
}

// DetectStarvation runs diagnostics without calling Pluck - useful for periodic health checks
func (mp *MonitoredPluck) DetectStarvation() (*DiagnosticReport, error) {
	report, err := RunDiagnostics(mp.db, &PluckQueryParameters{
		Limit:            10, // Default limit for health checks
		CurrentTimestamp: getCurrentTimestamp(),
	})

	if err != nil {
		return nil, err
	}

	// If ready candidates is zero, write diagnostic log
	if report.ReadyCandidates == 0 {
		if err := WriteDiagnosticLog(report, mp.workspacePath); err != nil {
			log.Printf("Failed to write diagnostic log: %v", err)
		}
	}

	return report, nil
}

// GetWorkspacePath returns the detected workspace path
func (mp *MonitoredPluck) GetWorkspacePath() string {
	return mp.workspacePath
}
