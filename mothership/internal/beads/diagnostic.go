package beads

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// DiagnosticReport captures the analysis of why Pluck returned zero candidates
type DiagnosticReport struct {
	Timestamp             string   `json:"timestamp"`
	TotalBeads            int64    `json:"total_beads"`
	OpenBeads             int64    `json:"open_beads"`
	UnblockedBeads        int64    `json:"unblocked_beads"`
	UnassignedBeads       int64    `json:"unassigned_beads"`
	WithoutDependencies   int64    `json:"without_dependencies"`
	WithoutLockConflicts  int64    `json:"without_lock_conflicts"`
	ReadyCandidates       int64    `json:"ready_candidates"`
	BlockedByStatus       int64    `json:"blocked_by_status"`
	BlockedByManualBlock  int64    `json:"blocked_by_manual_block"`
	BlockedByAssignee     int64    `json:"blocked_by_assignee"`
	BlockedByDependency   int64    `json:"blocked_by_dependency"`
	BlockedByLockConflict int64    `json:"blocked_by_lock_conflict"`
	IdentifiedBottleneck  string   `json:"identified_bottleneck,omitempty"`
	Recommendations       []string `json:"recommendations,omitempty"`
}

// PluckQueryParameters captures the parameters used in a Pluck query
type PluckQueryParameters struct {
	Limit            int       `json:"limit"`
	CurrentTimestamp time.Time `json:"current_timestamp"`
}

// RunDiagnostics analyzes the bead database to identify why Pluck returned zero candidates
func RunDiagnostics(db *sql.DB, params *PluckQueryParameters) (*DiagnosticReport, error) {
	report := &DiagnosticReport{
		Timestamp:       time.Now().Format(time.RFC3339),
		Recommendations: []string{},
	}

	var err error

	// Step 1: Total beads in the system
	err = db.QueryRow("SELECT COUNT(*) FROM issues").Scan(&report.TotalBeads)
	if err != nil {
		return nil, fmt.Errorf("failed to count total beads: %w", err)
	}

	// Step 2: Beads with base_status = 'open'
	err = db.QueryRow("SELECT COUNT(*) FROM issues WHERE base_status = 'open'").Scan(&report.OpenBeads)
	if err != nil {
		return nil, fmt.Errorf("failed to count open beads: %w", err)
	}
	report.BlockedByStatus = report.TotalBeads - report.OpenBeads

	// Step 3: Open beads that are not manually blocked
	err = db.QueryRow(`
		SELECT COUNT(*) FROM issues
		WHERE base_status = 'open'
		  AND (manual_blocked IS NULL OR manual_blocked = 0)
	`).Scan(&report.UnblockedBeads)
	if err != nil {
		return nil, fmt.Errorf("failed to count unblocked beads: %w", err)
	}
	report.BlockedByManualBlock = report.OpenBeads - report.UnblockedBeads

	// Step 4: Open, unblocked beads without assignee
	err = db.QueryRow(`
		SELECT COUNT(*) FROM issues
		WHERE base_status = 'open'
		  AND (manual_blocked IS NULL OR manual_blocked = 0)
		  AND assignee IS NULL
	`).Scan(&report.UnassignedBeads)
	if err != nil {
		return nil, fmt.Errorf("failed to count unassigned beads: %w", err)
	}
	report.BlockedByAssignee = report.UnblockedBeads - report.UnassignedBeads

	// Step 5: Beads without unfinished blocking dependencies
	// This checks the NOT EXISTS clause for dependencies
	err = db.QueryRow(`
		SELECT COUNT(*) FROM issues i
		WHERE base_status = 'open'
		  AND (manual_blocked IS NULL OR manual_blocked = 0)
		  AND assignee IS NULL
		  AND NOT EXISTS (
		      SELECT 1 FROM dependencies d
		      WHERE d.blocked_issue_id = i.id
		        AND d.kind = 'blocks'
		        AND d.blocker_issue_id IN (
		            SELECT id FROM issues WHERE base_status != 'closed'
		        )
		  )
	`).Scan(&report.WithoutDependencies)
	if err != nil {
		return nil, fmt.Errorf("failed to count beads without blocking dependencies: %w", err)
	}
	report.BlockedByDependency = report.UnassignedBeads - report.WithoutDependencies

	// Step 6: Beads without resource lock conflicts
	// This checks the NOT EXISTS clause for resource locks
	currentTime := params.CurrentTimestamp.Format(time.RFC3339)
	err = db.QueryRow(`
		SELECT COUNT(*) FROM issues i
		WHERE base_status = 'open'
		  AND (manual_blocked IS NULL OR manual_blocked = 0)
		  AND assignee IS NULL
		  AND NOT EXISTS (
		      SELECT 1 FROM dependencies d
		      WHERE d.blocked_issue_id = i.id
		        AND d.kind = 'blocks'
		        AND d.blocker_issue_id IN (
		            SELECT id FROM issues WHERE base_status != 'closed'
		        )
		  )
		  AND NOT EXISTS (
		      SELECT 1 FROM issue_resource_keys candidate_key
		      JOIN resource_locks held_lock ON held_lock.resource_key = candidate_key.resource_key
		      WHERE candidate_key.issue_id = i.id
		        AND held_lock.issue_id != i.id
		        AND (held_lock.lease_fencing_token IS NULL OR EXISTS (
		            SELECT 1 FROM leases active_lease
		            WHERE active_lease.issue_id = held_lock.issue_id
		              AND active_lease.fencing_token = held_lock.lease_fencing_token
		              AND active_lease.expires_at > ?
		        ))
		  )
	`, currentTime).Scan(&report.ReadyCandidates)
	if err != nil {
		return nil, fmt.Errorf("failed to count ready candidates: %w", err)
	}
	report.WithoutLockConflicts = report.WithoutDependencies - report.ReadyCandidates
	report.BlockedByLockConflict = report.WithoutDependencies - report.WithoutLockConflicts

	// Identify the bottleneck
	report.identifyBottleneck()

	return report, nil
}

// identifyBottleneck analyzes the diagnostic results to determine which filter is eliminating candidates
func (r *DiagnosticReport) identifyBottleneck() {
	// Work backwards from the final count to find where the drop occurs
	if r.TotalBeads == 0 {
		r.IdentifiedBottleneck = "empty_database"
		r.Recommendations = append(r.Recommendations, "No beads exist in the database. Create initial beads to begin tracking work.")
		return
	}

	if r.OpenBeads == 0 {
		r.IdentifiedBottleneck = "all_beads_closed"
		r.Recommendations = append(r.Recommendations, "All beads are closed. Either all work is complete, or beads were closed prematurely. Review recent closures.")
		return
	}

	if r.UnblockedBeads == 0 {
		r.IdentifiedBottleneck = "all_beads_manually_blocked"
		r.Recommendations = append(r.Recommendations, "All open beads are manually blocked. Review which beads are blocked and whether blocks should be cleared.")
		return
	}

	if r.UnassignedBeads == 0 {
		r.IdentifiedBottleneck = "all_beads_assigned"
		r.Recommendations = append(r.Recommendations, "All open, unblocked beads have assignees. Workers may be stalled. Check assignee status and release stuck assignments.")
		return
	}

	if r.WithoutDependencies == 0 {
		r.IdentifiedBottleneck = "all_beads_blocked_by_dependencies"
		r.Recommendations = append(r.Recommendations, "All unassigned open beads have unfinished blocking dependencies. Review dependency graph for cycles or stuck blockers.")
		return
	}

	if r.ReadyCandidates == 0 {
		r.IdentifiedBottleneck = "all_beads_blocked_by_lock_conflicts"
		r.Recommendations = append(r.Recommendations, "All beads are blocked by resource lock conflicts. Review resource allocation and lease expiration.")
		return
	}

	// If we get here, there are ready candidates but they weren't returned
	r.IdentifiedBottleneck = "unknown_limit_or_other_filter"
	r.Recommendations = append(r.Recommendations, fmt.Sprintf("Found %d ready candidates, but Pluck returned zero. Check LIMIT parameter or other custom filters.", r.ReadyCandidates))
}

// WriteDiagnosticLog writes the diagnostic report to a log file
func WriteDiagnosticLog(report *DiagnosticReport, workspacePath string) error {
	// Create diagnostics directory if it doesn't exist
	diagnosticsDir := filepath.Join(workspacePath, ".beads", "diagnostics")
	if err := os.MkdirAll(diagnosticsDir, 0755); err != nil {
		return fmt.Errorf("failed to create diagnostics directory: %w", err)
	}

	// Write to timestamped log file
	logPath := filepath.Join(diagnosticsDir, fmt.Sprintf("pluck-starvation-%s.json", time.Now().Format("20060102-150405")))
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal diagnostic report: %w", err)
	}

	if err := os.WriteFile(logPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write diagnostic log: %w", err)
	}

	// Also append to a summary log for easy tailing
	summaryLog := filepath.Join(diagnosticsDir, "pluck-starvation-summary.log")
	f, err := os.OpenFile(summaryLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open summary log: %w", err)
	}
	defer f.Close()

	logEntry := fmt.Sprintf("[%s] Bottleneck: %s | Ready: %d/%d | %s\n",
		report.Timestamp,
		report.IdentifiedBottleneck,
		report.ReadyCandidates,
		report.TotalBeads,
		report.Recommendations[0])

	if _, err := f.WriteString(logEntry); err != nil {
		return fmt.Errorf("failed to write to summary log: %w", err)
	}

	log.Printf("Diagnostic report written to: %s", logPath)
	return nil
}

// ShouldCreateRemediationBead determines if an auto-remediation bead should be created
func (r *DiagnosticReport) ShouldCreateRemediationBead() bool {
	// Only create auto-remediation beads for specific, actionable bottlenecks
	switch r.IdentifiedBottleneck {
	case "all_beads_blocked_by_dependencies":
		// Dependency cycles or stuck blockers - high value for auto-remediation
		return true
	case "all_beads_manually_blocked":
		// May need review before auto-clearing blocks
		return false
	case "all_beads_assigned":
		// Stale assignees - high value for auto-remediation
		return true
	case "all_beads_blocked_by_lock_conflicts":
		// Resource allocation issues - may need human judgment
		return false
	default:
		return false
	}
}

// GenerateRemediationBeadTitle creates a title for an auto-remediation bead
func (r *DiagnosticReport) GenerateRemediationBeadTitle() string {
	switch r.IdentifiedBottleneck {
	case "all_beads_blocked_by_dependencies":
		return fmt.Sprintf("Fix dependency cycle blocking %d beads", r.BlockedByDependency)
	case "all_beads_assigned":
		return fmt.Sprintf("Release stale assignees blocking %d beads", r.BlockedByAssignee)
	default:
		return fmt.Sprintf("Resolve Pluck starvation: %s", r.IdentifiedBottleneck)
	}
}

// GenerateRemediationBeadBody creates a description for an auto-remediation bead
func (r *DiagnosticReport) GenerateRemediationBeadBody() string {
	body := fmt.Sprintf("# Auto-remediation for Pluck starvation\n\n")
	body += fmt.Sprintf("**Bottleneck identified:** %s\n\n", r.IdentifiedBottleneck)
	body += fmt.Sprintf("**Diagnostic timestamp:** %s\n\n", r.Timestamp)
	body += fmt.Sprintf("**Beads affected:** %d\n\n", r.TotalBeads)

	body += "## Filter Analysis\n\n"
	body += fmt.Sprintf("- Total beads: %d\n", r.TotalBeads)
	body += fmt.Sprintf("- Open beads: %d (blocked by status: %d)\n", r.OpenBeads, r.BlockedByStatus)
	body += fmt.Sprintf("- Unblocked beads: %d (blocked by manual block: %d)\n", r.UnblockedBeads, r.BlockedByManualBlock)
	body += fmt.Sprintf("- Unassigned beads: %d (blocked by assignee: %d)\n", r.UnassignedBeads, r.BlockedByAssignee)
	body += fmt.Sprintf("- Without dependencies: %d (blocked by dependencies: %d)\n", r.WithoutDependencies, r.BlockedByDependency)
	body += fmt.Sprintf("- Ready candidates: %d (blocked by lock conflicts: %d)\n\n", r.ReadyCandidates, r.BlockedByLockConflict)

	body += "## Recommendations\n\n"
	for i, rec := range r.Recommendations {
		body += fmt.Sprintf("%d. %s\n", i+1, rec)
	}

	body += "\n## Next Steps\n\n"
	body += "Review the diagnostic data and implement the recommended fixes above. "
	body += "Once resolved, the bead fleet should resume normal processing."

	return body
}
