# Pluck Starvation Diagnostics

Auto-diagnostic system for detecting and resolving Pluck starvation alerts when bead-rs returns zero candidates.

## Overview

When `bead pluck --ready` returns zero candidates, this system automatically:

1. **Analyzes the database state** to identify which WHERE condition is filtering out all beads
2. **Produces a structured diagnostic report** showing bead counts at each filter stage
3. **Identifies the bottleneck** (e.g., "all beads blocked by dependencies")
4. **Creates auto-remediation beads** for actionable bottlenecks (dependency cycles, stale assignees)
5. **Writes diagnostic logs** for historical analysis and trend detection

## Architecture

### Diagnostic Report Structure

```go
type DiagnosticReport struct {
    Timestamp              string    // When the diagnostic ran
    TotalBeads             int64     // Total beads in database
    OpenBeads              int64     // Beads with base_status='open'
    UnblockedBeads         int64     // Open AND not manually blocked
    UnassignedBeads        int64     // Open, unblocked AND no assignee
    WithoutDependencies    int64     // Unassigned AND no blocking dependencies
    WithoutLockConflicts  int64     // Unassigned, no deps AND no lock conflicts
    ReadyCandidates        int64     // Final count (what Pluck returns)
    BlockedByStatus       int64     // Beads filtered by status check
    BlockedByManualBlock  int64     // Beads filtered by manual block
    BlockedByAssignee     int64     // Beads filtered by assignee check
    BlockedByDependency   int64     // Beads filtered by dependency check
    BlockedByLockConflict int64     // Beads filtered by lock conflict check
    IdentifiedBottleneck  string    // Which filter eliminated all candidates
    Recommendations        []string  // Actionable remediation steps
}
```

### Filter Analysis Flow

The diagnostic runs 6 COUNT queries, progressively removing WHERE conditions:

1. **Total beads**: No filters
2. **Open beads**: `WHERE base_status = 'open'`
3. **Unblocked beads**: Add `AND (manual_blocked IS NULL OR manual_blocked = 0)`
4. **Unassigned beads**: Add `AND assignee IS NULL`
5. **Without dependencies**: Add `AND NOT EXISTS (blocking dependencies)`
6. **Ready candidates**: Add `AND NOT EXISTS (lock conflicts)`

Each step reveals how many beads were filtered by that condition.

## Usage

### Basic Integration

```go
import (
    "database/sql"
    "mothership/internal/beads"
)

func main() {
    db, _ := sql.Open("sqlite", "./.beads/beads.db")
    monitoredPluck := beads.NewMonitoredPluck(db, "/path/to/workspace")

    // Automatically runs diagnostics if zero candidates returned
    beadIDs, err := monitoredPluck.PluckReady(10)
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("Pluck returned %d candidates", len(beadIDs))
}
```

### Standalone Diagnostics

```go
// Run diagnostics without calling Pluck (useful for health checks)
report, err := monitoredPluck.DetectStarvation()
if err != nil {
    log.Fatal(err)
}

log.Printf("Ready candidates: %d, Bottleneck: %s",
    report.ReadyCandidates,
    report.IdentifiedBottleneck)
```

### Direct Diagnostic Execution

```go
params := &beads.PluckQueryParameters{
    Limit:            10,
    CurrentTimestamp: time.Now(),
}

report, err := beads.RunDiagnostics(db, params)
if err != nil {
    log.Fatal(err)
}

// Write diagnostic log to workspace
err = beads.WriteDiagnosticLog(report, workspacePath)
```

## Auto-Remediation Beads

The system automatically creates remediation beads for certain bottleneck types:

### Creates Auto-Remediation

- **`all_beads_blocked_by_dependencies`**: Creates bead to fix dependency cycles
  - Title: "Fix dependency cycle blocking N beads"
  - Body includes full diagnostic data + recommendations

- **`all_beads_assigned`**: Creates bead to release stale assignees
  - Title: "Release stale assignees blocking N beads"
  - High-value because this indicates workers may be stalled

### Does NOT Create Auto-Remediation (requires human judgment)

- **`all_beads_manually_blocked`**: Manual blocks may be intentional
- **`all_beads_blocked_by_lock_conflicts`**: Resource allocation needs review

## Diagnostic Logs

Two log files are written to `.beads/diagnostics/`:

### JSON Detailed Log

Format: `pluck-starvation-20060830-153045.json`

```json
{
  "timestamp": "2026-08-30T15:30:45Z",
  "total_beads": 150,
  "open_beads": 120,
  "unblocked_beads": 115,
  "unassigned_beads": 80,
  "without_dependencies": 45,
  "without_lock_conflicts": 0,
  "ready_candidates": 0,
  "blocked_by_status": 30,
  "blocked_by_manual_block": 5,
  "blocked_by_assignee": 35,
  "blocked_by_dependency": 35,
  "blocked_by_lock_conflict": 45,
  "identified_bottleneck": "all_beads_blocked_by_lock_conflicts",
  "recommendations": [
    "All beads are blocked by resource lock conflicts. Review resource allocation and lease expiration."
  ]
}
```

### Summary Log (for tailing)

Format: `pluck-starvation-summary.log`

```
[2026-08-30T15:30:45Z] Bottleneck: all_beads_blocked_by_lock_conflicts | Ready: 0/150 | All beads are blocked by resource lock conflicts. Review resource allocation and lease expiration.
```

## Bottleneck Types

| Bottleneck | Description | Auto-Remediate |
|------------|-------------|-----------------|
| `empty_database` | No beads in database | No (create beads first) |
| `all_beads_closed` | All beads closed | No (may be legitimate) |
| `all_beads_manually_blocked` | All open beads manually blocked | No (may be intentional) |
| `all_beads_assigned` | All unassigned beads have assignees | Yes (stale assignees) |
| `all_beads_blocked_by_dependencies` | All beads have blocking dependencies | Yes (cycle detection) |
| `all_beads_blocked_by_lock_conflicts` | All beads blocked by resource locks | No (review needed) |
| `unknown_limit_or_other_filter` | Candidates exist but LIMIT/custom filter | No (check parameters) |

## Integration with Starvation Alerts

This system provides an alternative to human-blocked starvation alerts:

### Original Alert (spaxel-ddd2e3cf)
- Detects: Pluck returns zero candidates
- Action: Alert human operator
- Resolution: Human investigates and fixes

### Auto-Diagnostic (spaxel-07798c9c)
- Detects: Pluck returns zero candidates
- Action: Run diagnostic analysis
- Resolution: 
  - Create auto-remediation bead if bottleneck is actionable
  - Provide structured diagnostic data for human review
  - Skip human intervention for resolvable issues

## Example Scenarios

### Scenario 1: Dependency Cycle

```
Total beads: 100 → Open: 100 → Unblocked: 100 → Unassigned: 100 → Without deps: 0
Bottleneck: all_beads_blocked_by_dependencies
Action: Create bead "Fix dependency cycle blocking 100 beads"
```

### Scenario 2: Stale Assignees

```
Total beads: 50 → Open: 50 → Unblocked: 50 → Unassigned: 0
Bottleneck: all_beads_assigned
Action: Create bead "Release stale assignees blocking 50 beads"
```

### Scenario 3: Resource Lock Conflicts

```
Total beads: 75 → ... → Without deps: 75 → Ready: 0
Bottleneck: all_beads_blocked_by_lock_conflicts
Action: Log diagnostic, NO auto-remediation (human review needed)
```

## Testing

Run the test suite:

```bash
cd mothership
go test ./internal/beads/ -v
```

Run benchmarks:

```bash
go test ./internal/beads/ -bench=. -benchmem
```

## Performance Considerations

- Each diagnostic run executes 6 COUNT queries
- All queries use indexed columns (base_status, manual_blocked, assignee)
- Typical runtime: <10ms on databases with <10,000 beads
- Only runs when Pluck returns zero candidates (not on every Pluck call)
- Diagnostic logs are append-only writes (minimal overhead)

## Dependencies

- Requires `bead-rs` CLI in PATH for MonitoredPluck integration
- Requires SQLite database at `.beads/beads.db`
- Creates `.beads/diagnostics/` directory for logs

## Future Enhancements

Potential improvements:

1. **Trend detection**: Track repeated bottleneck patterns over time
2. **Historical analysis**: Identify beads that frequently cause starvation
3. **Predictive alerts**: Warn before starvation occurs based on trends
4. **Auto-recovery**: Automatic assignee release for certain patterns
5. **Dependency graph visualization**: Visual representation of blocking chains
