#!/usr/bin/env bash
# Walk-through presence test monitoring script for bf-3v39 walk session
#
# This script polls deltaRMS and blob APIs during a physical walk-through test
# to detect motion detection spikes. It records timestamps, deltaRMS values,
# and blob evidence for analysis.
#
# Usage: ./scripts/walkthrough_monitor.sh [duration_seconds]
#   duration_seconds: How long to monitor (default: 60)
#   Output: Creates timestamped log file with results
#
# OPERATOR INSTRUCTIONS:
# 1. Start this script before beginning the walk
# 2. Walk through the detection area between the node and home WiFi AP
# 3. The script will continuously poll APIs and record data
# 4. Review the output log for deltaRMS spikes > 0.05

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Configuration
DURATION="${1:-60}"                          # Default 60 seconds
POLL_INTERVAL=1                              # Poll every 1 second
PORT="${MOTHERSHIP_PORT:-8088}"              # Default mothership port
OUTPUT_DIR="$ROOT/data/walkthrough"
OUTPUT_FILE="$OUTPUT_DIR/walkthrough_$(date -u +"%Y%m%d_%H%M%S").log"
SPIKE_THRESHOLD=0.05                        # Expected deltaRMS spike threshold

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Create output directory
mkdir -p "$OUTPUT_DIR"

log() {
    echo "[$(date -u +"%Y-%m-%dT%H:%M:%SZ")] $*" | tee -a "$OUTPUT_FILE"
}

print_header() {
    cat << 'EOF'
╔══════════════════════════════════════════════════════════════╗
║         Walk-Through Presence Test Monitoring (bf-3v39)      ║
║                                                              ║
║  This script monitors deltaRMS and blob APIs during a       ║
║  physical walk-through test to detect motion detection.    ║
╚══════════════════════════════════════════════════════════════╝
EOF
}

print_instructions() {
    cat << EOF

${YELLOW}OPERATOR INSTRUCTIONS:${NC}
1. Ensure mothership is running on port $PORT
2. Position yourself at the start of the detection area
3. Press ENTER to begin monitoring and start walking
4. Walk through the detection area between node and WiFi AP
5. Continue walking for the full ${DURATION}s monitoring period
6. After completion, review the log file: $OUTPUT_FILE

${GREEN}Expected Results:${NC}
- deltaRMS should spike from baseline (~0.02) to > 0.05 during walk
- /api/blobs should show at least 1 tracked blob
- Timestamps should correlate walk timing with deltaRMS spikes

${RED}If no spike detected:${NC}
- Verify passive_bss NVS key holds correct AP BSSID
- Consider switching node to TX_RX mode for node-to-node CSI
- See notes/bf-3v39-troubleshooting-runbook.md

EOF
}

check_mothership() {
    if ! command -v curl &> /dev/null; then
        echo "ERROR: curl required but not installed"
        exit 1
    fi

    # Check if mothership is running
    if ! resp=$(curl -s --max-time 2 "http://localhost:$PORT/healthz" 2>/dev/null); then
        echo "ERROR: Cannot connect to mothership on port $PORT"
        echo "Ensure mothership is running: SPAXEL_BIND_ADDR=127.0.0.1:$PORT mothership"
        exit 1
    fi

    health_status=$(echo "$resp" | jq -r '.status // empty' 2>/dev/null)
    if [ "$health_status" != "ok" ]; then
        echo "ERROR: Mothership health check failed"
        echo "Response: $resp"
        exit 1
    fi

    log "Mothership confirmed healthy on port $PORT"
}

# Poll deltaRMS from explainability API
poll_deltarms() {
    local blob_id=$1
    local timestamp=$2

    # Get blob explanation to extract deltaRMS from contributing links
    local response
    response=$(curl -s --max-time 2 "http://localhost:$PORT/api/explain/$blob_id" 2>/dev/null)

    if [ -z "$response" ]; then
        echo "null"
        return
    fi

    # Extract maximum deltaRMS from contributing links
    local max_deltarms
    max_deltarms=$(echo "$response" | jq -r '
        if .contributing_links then
            ([.contributing_links[].delta_rms | select(. != null)] | max // 0.0)
        else
            0.0
        end
    ' 2>/dev/null)

    echo "${max_deltarms:-0}"
}

# Poll blob count and extract blob IDs
poll_blobs() {
    local response
    response=$(curl -s --max-time 2 "http://localhost:$PORT/api/blobs" 2>/dev/null)

    if [ -z "$response" ]; then
        echo "0"
        return
    fi

    # Get blob count
    local count
    count=$(echo "$response" | jq 'length' 2>/dev/null || echo "0")
    echo "$count"
}

# Get blob IDs for detailed deltaRMS polling
get_blob_ids() {
    local response
    response=$(curl -s --max-time 2 "http://localhost:$PORT/api/blobs" 2>/dev/null)

    if [ -z "$response" ]; then
        echo ""
        return
    fi

    echo "$response" | jq -r '.[].id' 2>/dev/null || echo ""
}

# Get system status
get_status() {
    local response
    response=$(curl -s --max-time 2 "http://localhost:$PORT/api/status" 2>/dev/null)

    if [ -z "$response" ]; then
        echo "{}"
        return
    fi

    echo "$response"
}

# Main monitoring loop
run_monitoring() {
    local start_time
    start_time=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

    log "=== WALK-THROUGH MONITORING STARTED ==="
    log "Duration: ${DURATION}s, Poll Interval: ${POLL_INTERVAL}s, Threshold: $SPIKE_THRESHOLD"
    log "Start time: $start_time"

    # Track peak values
    local peak_deltarms=0.0
    local peak_blob_count=0
    local spike_count=0
    local sample_count=0

    log ""
    log "TIME ELAPSED | BLOBS | PEAK BLOBS | DELTARMS | PEAK DELTARMS | SPIKE?"
    log "-----------|-------|------------|----------|----------------|-------"

    for elapsed in $(seq 1 "$DURATION"); do
        sleep "$POLL_INTERVAL"
        sample_count=$((sample_count + 1))

        # Poll blob count
        local blob_count
        blob_count=$(poll_blobs)

        # Track peak blob count
        if [ "$blob_count" -gt "$peak_blob_count" ]; then
            peak_blob_count=$blob_count
        fi

        # Get deltaRMS from first blob (if any)
        local deltarms=0.0
        local blob_ids
        blob_ids=$(get_blob_ids)

        if [ -n "$blob_ids" ]; then
            # Get deltaRMS from first available blob
            local first_blob_id
            first_blob_id=$(echo "$blob_ids" | head -1)
            if [ -n "$first_blob_id" ]; then
                deltarms=$(poll_deltarms "$first_blob_id")
            fi
        fi

        # Track peak deltaRMS
        deltarms=$(echo "$deltarms" | awk '{printf "%.4f", $1}')
        if (( $(echo "$deltarms > $peak_deltarms" | bc -l) )); then
            peak_deltarms=$deltarms
        fi

        # Check for spike
        local spike_indicator="❌"
        if (( $(echo "$deltarms > $SPIKE_THRESHOLD" | bc -l) )); then
            spike_indicator="✅"
            spike_count=$((spike_count + 1))
        fi

        # Log sample
        log "$(printf '%2ds         | %2d     | %2d          | %.4f   | %.4f         | %s' \
            "$elapsed" "$blob_count" "$peak_blob_count" "$deltarms" "$peak_deltarms" "$spike_indicator")"
    done

    local end_time
    end_time=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

    log ""
    log "=== MONITORING COMPLETE ==="
    log "End time: $end_time"
    log "Total samples: $sample_count"
    log "Spike count (deltaRMS > $SPIKE_THRESHOLD): $spike_count"
    log "Peak blob count: $peak_blob_count"
    log "Peak deltaRMS: $peak_deltarms"

    # Evaluation
    log ""
    log "=== EVALUATION ==="

    if (( $(echo "$peak_deltarms > $SPIKE_THRESHOLD" | bc -l) )); then
        log "✅ PASS: deltaRMS spike detected ($peak_deltarms > $SPIKE_THRESHOLD)"
        log "Motion detection confirmed during walk-through"
    else
        log "❌ FAIL: No deltaRMS spike detected (peak: $peak_deltarms, threshold: $SPIKE_THRESHOLD)"
        log "Troubleshooting required:"
        log "  1. Verify passive_bss NVS key holds correct AP BSSID"
        log "  2. Check node positioning and WiFi signal strength"
        log "  3. Consider switching node to TX_RX mode for node-to-node CSI"
        log "  4. Review notes/bf-3v39-troubleshooting-runbook.md"
    fi

    if [ "$peak_blob_count" -gt 0 ]; then
        log "✅ PASS: Tracked blobs observed (peak: $peak_blob_count)"
    else
        log "⚠️  WARNING: No tracked blobs observed during walk"
        log "This may indicate fusion or localization issues"
    fi

    log ""
    log "Full results saved to: $OUTPUT_FILE"
}

# Main execution
main() {
    print_header
    print_instructions

    echo -n "Ready to begin? Press ENTER to start monitoring: "
    read -r

    check_mothership
    run_monitoring

    echo ""
    echo "${GREEN}Monitoring complete!${NC}"
    echo "Review results: $OUTPUT_FILE"
}

main "$@"
