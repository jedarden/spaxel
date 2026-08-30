#!/usr/bin/env bash
# Bead state consistency checker
# Runs every 15 minutes to detect and repair inconsistent bead states

set -euo pipefail

# Log file
LOG_FILE="/home/coding/spaxel/.beads/logs/state-checker.log"
mkdir -p "$(dirname "$LOG_FILE")"

log() {
    echo "[$(date -Iseconds)] $*" | tee -a "$LOG_FILE"
}

# Check for assigned-but-open beads: status=open AND assignee IS NOT NULL
check_assigned_but_open() {
    log "Checking for assigned-but-open beads (status=open with assignee)..."

    local output
    output=$(bead list --status open --json 2>/dev/null) || return 0

    # Check if output contains "No issues found"
    if [[ "$output" == *"No issues found"* ]]; then
        log "No open beads found"
        return 0
    fi

    local count=0
    while IFS= read -r line; do
        if [[ -n "$line" ]]; then
            local assignee
            assignee=$(echo "$line" | jq -r '.assignee // ""')

            if [[ -n "$assignee" && "$assignee" != "null" ]]; then
                local bead_id
                bead_id=$(echo "$line" | jq -r '.id')
                log "  - $bead_id (assignee: $assignee)"
                count=$((count + 1))

                # Attempt repair
                log "    Running: bead doctor --repair $bead_id"
                if bead doctor --repair "$bead_id" >> "$LOG_FILE" 2>&1; then
                    log "    ✓ Repair succeeded for $bead_id"
                else
                    log "    ✗ Repair failed for $bead_id, creating bug bead"
                    create_bug_bead "$bead_id" "assigned-but-open" "Bead has status=open but assignee=$assignee is set"
                fi
            fi
        fi
    done <<< "$output"

    if [[ "$count" -eq 0 ]]; then
        log "No assigned-but-open beads found"
    else
        log "Found $count assigned-but-open beads"
    fi
}

# Check for orphaned in-progress beads: status=in_progress AND assignee IS NULL
check_orphaned_in_progress() {
    log "Checking for orphaned in-progress beads (status=in_progress without assignee)..."

    local output
    output=$(bead list --status in_progress --json 2>/dev/null) || return 0

    # Check if output contains "No issues found"
    if [[ "$output" == *"No issues found"* ]]; then
        log "No in-progress beads found"
        return 0
    fi

    local count=0
    while IFS= read -r line; do
        if [[ -n "$line" ]]; then
            local assignee
            assignee=$(echo "$line" | jq -r '.assignee // ""')

            if [[ -z "$assignee" || "$assignee" == "null" ]]; then
                local bead_id
                bead_id=$(echo "$line" | jq -r '.id')
                log "  - $bead_id"
                count=$((count + 1))

                # Attempt repair
                log "    Running: bead doctor --repair $bead_id"
                if bead doctor --repair "$bead_id" >> "$LOG_FILE" 2>&1; then
                    log "    ✓ Repair succeeded for $bead_id"
                else
                    log "    ✗ Repair failed for $bead_id, creating bug bead"
                    create_bug_bead "$bead_id" "orphaned-in-progress" "Bead has status=in_progress but assignee is null"
                fi
            fi
        fi
    done <<< "$output"

    if [[ "$count" -eq 0 ]]; then
        log "No orphaned in-progress beads found"
    else
        log "Found $count orphaned in-progress beads"
    fi
}

# Check for stale dependency blocks: beads where all dependencies are closed but bead remains blocked
check_stale_dependencies() {
    log "Checking for stale dependency blocks..."

    local output
    output=$(bead list --status open --json 2>/dev/null) || return 0

    # Check if output contains "No issues found"
    if [[ "$output" == *"No issues found"* ]]; then
        log "No open beads found"
        return 0
    fi

    local count=0
    while IFS= read -r line; do
        if [[ -n "$line" ]]; then
            local bead_id
            bead_id=$(echo "$line" | jq -r '.id')

            local has_deps
            has_deps=$(echo "$line" | jq -r '.dependencies != null and (.dependencies | length > 0)')

            if [[ "$has_deps" == "true" ]]; then
                local deps
                deps=$(echo "$line" | jq -r '.dependencies[].blocker')

                if [[ -n "$deps" ]]; then
                    local all_closed=true
                    local dep_list=""

                    while IFS= read -r dep_id; do
                        if [[ -n "$dep_id" ]]; then
                            local dep_status
                            dep_status=$(bead show "$dep_id" 2>/dev/null | grep -oP 'Status:\s*\K[^[:space:]]+' || echo "unknown")

                            if [[ "$dep_status" != "closed" ]]; then
                                all_closed=false
                                break
                            fi

                            dep_list="${dep_list}${dep_id}, "
                        fi
                    done <<< "$deps"

                    if [[ "$all_closed" == "true" ]]; then
                        log "  - $bead_id: All dependencies closed but bead remains open"
                        count=$((count + 1))

                        # Attempt repair
                        log "    Running: bead doctor --repair $bead_id"
                        if bead doctor --repair "$bead_id" >> "$LOG_FILE" 2>&1; then
                            log "    ✓ Repair succeeded for $bead_id"
                        else
                            log "    ✗ Repair failed for $bead_id, creating bug bead"
                            create_bug_bead "$bead_id" "stale-dependency" "All dependencies are closed (${dep_list%, }) but bead remains blocked"
                        fi
                    fi
                fi
            fi
        fi
    done <<< "$output"

    if [[ "$count" -eq 0 ]]; then
        log "No stale dependency blocks found"
    else
        log "Found $count stale dependency blocks"
    fi
}

# Create a bug bead for repair failures
create_bug_bead() {
    local bead_id="$1"
    local issue_type="$2"
    local description="$3"

    local title="Bead state inconsistency: $issue_type in $bead_id"
    local body="Bead state checker detected an inconsistency that could not be auto-repaired.

Affected bead: $bead_id
Issue type: $issue_type

Description:
$description

Original bead details:
$(bead show "$bead_id" 2>/dev/null || echo "Could not retrieve bead details")

Repair attempt failed with 'bead doctor --repair'. Manual intervention required."

    log "Creating bug bead: $title"

    if bead create \
        --title "$title" \
        --priority 2 \
        --issue-type bug \
        --label "state-checker" \
        --label "auto-generated" \
        --body "$body" >> "$LOG_FILE" 2>&1; then
        log "  ✓ Bug bead created successfully"
    else
        log "  ✗ Failed to create bug bead"
    fi
}

main() {
    log "=== Bead state consistency check started ==="

    check_assigned_but_open
    check_orphaned_in_progress
    check_stale_dependencies

    log "=== Bead state consistency check completed ==="
}

main "$@"
