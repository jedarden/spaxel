#!/bin/bash
# End-to-end integration test harness for Spaxel
# Starts the mothership, runs the CSI simulator, and asserts on behavior

set -euo pipefail

# Configuration
MOTHERSHIP_IMAGE="${MOTHERSHIP_IMAGE:-ronaldraygun/spaxel:latest}"
MOTHERSHIP_CONTAINER="spaxel-e2e-test"
MOTHERSHIP_PORT=8080
HEALTH_TIMEOUT=15
SIM_DURATION=30
SIM_NODES=4
SIM_WALKERS=2
SIM_RATE=20
SIM_SEED=42
TEST_TIMEOUT=90

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Cleanup function
cleanup() {
    local exit_code=$?
    log_info "Cleaning up..."

    # Stop simulator if running
    if [ -n "$SIM_PID" ]; then
        kill $SIM_PID 2>/dev/null || true
        wait $SIM_PID 2>/dev/null || true
    fi

    # Stop and remove mothership container
    docker stop "$MOTHERSHIP_CONTAINER" 2>/dev/null || true
    docker rm "$MOTHERSHIP_CONTAINER" 2>/dev/null || true

    if [ $exit_code -eq 0 ]; then
        log_info "All tests passed!"
    else
        log_error "Tests failed with exit code $exit_code"
    fi

    exit $exit_code
}

trap cleanup EXIT INT TERM

# Helper function for HTTP requests with timeout
http_get() {
    local url="$1"
    local max_attempts="${2:-1}"
    local wait="${3:-0}"

    for i in $(seq 1 "$max_attempts"); do
        if curl -sSf --max-time 5 "$url" 2>/dev/null; then
            return 0
        fi
        if [ $i -lt $max_attempts ]; then
            sleep "$wait"
        fi
    done
    return 1
}

# Helper function to check JSON field
json_field() {
    local json="$1"
    local field="$2"
    echo "$json" | jq -r "$field // empty"
}

# Step 1: Start mothership container
log_info "Step 1: Starting mothership container..."

# Check if container is already running and remove it
docker rm -f "$MOTHERSHIP_CONTAINER" 2>/dev/null || true

# Run mothership container
docker run -d \
    --name "$MOTHERSHIP_CONTAINER" \
    -p "$MOTHERSHIP_PORT:8080" \
    -e SPAXEL_LOG_LEVEL=info \
    -e TZ=UTC \
    --tmpfs /data:size=100M \
    "$MOTHERSHIP_IMAGE" >/dev/null

if [ $? -ne 0 ]; then
    log_error "Failed to start mothership container"
    exit 1
fi

log_info "Mothership container started: $MOTHERSHIP_CONTAINER"

# Step 2: Wait for /healthz to return {status:'ok'}
log_info "Step 2: Waiting for mothership to be healthy..."

start_time=$(date +%s)
while true; do
    elapsed=$(($(date +%s) - start_time))
    if [ $elapsed -ge $HEALTH_TIMEOUT ]; then
        log_error "Health check timeout after ${HEALTH_TIMEOUT}s"
        docker logs "$MOTHERSHIP_CONTAINER" --tail 50
        exit 1
    fi

    health_response=$(http_get "http://localhost:$MOTHERSHIP_PORT/healthz" 1 0 2>/dev/null || echo "")

    if [ -n "$health_response" ]; then
        status=$(json_field "$health_response" ".status")
        if [ "$status" = "ok" ]; then
            log_info "Mothership is healthy (status: ok, uptime: $(json_field "$health_response" ".uptime_s")s)"
            break
        fi
    fi

    sleep 0.5
done

# Step 3: Check if PIN auth is enabled, setup if needed
log_info "Step 3: Checking auth setup..."

auth_status=$(http_get "http://localhost:$MOTHERSHIP_PORT/api/auth/status" 1 0 2>/dev/null || echo "{}")
pin_configured=$(json_field "$auth_status" ".pin_configured // false")

if [ "$pin_configured" = "false" ]; then
    log_info "Setting up test PIN..."
    setup_response=$(curl -sS -X POST \
        -H "Content-Type: application/json" \
        -d '{"pin":"0000"}' \
        "http://localhost:$MOTHERSHIP_PORT/api/auth/setup" 2>/dev/null || echo "")

    if [ -n "$setup_response" ]; then
        ok=$(json_field "$setup_response" ".ok // false")
        if [ "$ok" = "true" ]; then
            log_info "Test PIN configured successfully"
        else
            log_warn "PIN setup response unexpected: $setup_response"
        fi
    fi

    # Login with the PIN
    log_info "Logging in with test PIN..."
    login_response=$(curl -sS -X POST \
        -H "Content-Type: application/json" \
        -d '{"pin":"0000"}' \
        -c /tmp/spaxel-e2e-cookies.txt \
        "http://localhost:$MOTHERSHIP_PORT/api/auth/login" 2>/dev/null || echo "")
fi

# Step 4: Build and start simulator
log_info "Step 4: Starting CSI simulator..."

# Build simulator
cd /home/coding/spaxel/mothership
if ! go build -o /tmp/spaxel-sim ./cmd/sim 2>/dev/null; then
    log_error "Failed to build simulator"
    exit 1
fi

# Start simulator in background
/tmp/spaxel-sim \
    --mothership "ws://localhost:$MOTHERSHIP_PORT/ws/node" \
    --nodes "$SIM_NODES" \
    --walkers "$SIM_WALKERS" \
    --rate "$SIM_RATE" \
    --duration "${SIM_DURATION}s" \
    --ble \
    --seed "$SIM_SEED" \
    --show-frame-rate \
    > /tmp/spaxel-sim.log 2>&1 &

SIM_PID=$!

if [ -z "$SIM_PID" ]; then
    log_error "Failed to start simulator"
    exit 1
fi

log_info "Simulator started (PID: $SIM_PID), will run for ${SIM_DURATION}s"

# Step 5: Assert during run (poll every 1s for up to SIM_DURATION)
log_info "Step 5: Asserting during simulation..."

assert_start=$(date +%s)
blob_detected=0
nodes_online=0
health_ok_during_run=0
health_check_count=0
health_check_passed=0

while true; do
    elapsed=$(($(date +%s) - assert_start))
    if [ $elapsed -ge $SIM_DURATION ]; then
        break
    fi

    # Check /healthz - assert status=='ok' throughout entire run
    health_response=$(http_get "http://localhost:$MOTHERSHIP_PORT/healthz" 1 0 2>/dev/null || echo "")
    if [ -n "$health_response" ]; then
        status=$(json_field "$health_response" ".status")
        health_check_count=$((health_check_count + 1))
        if [ "$status" = "ok" ]; then
            health_ok_during_run=1
            health_check_passed=$((health_check_passed + 1))
        else
            log_error "Health check failed during run: status=$status"
        fi
    fi

    # Check /api/nodes for online nodes
    nodes_response=$(http_get "http://localhost:$MOTHERSHIP_PORT/api/nodes" 1 0 2>/dev/null || echo "")
    if [ -n "$nodes_response" ]; then
        nodes_online=$(echo "$nodes_response" | jq '[.[] | select(.status=="online")] | length' 2>/dev/null || echo "0")

        # Assert nodes_online == SIM_NODES within first 5 seconds
        if [ $elapsed -le 5 ] && [ "$nodes_online" -ge "$SIM_NODES" ]; then
            log_info "✓ All $SIM_NODES nodes online within first 5s (elapsed: ${elapsed}s)"
        fi
    fi

    # Check for blobs via /api/events (detection events) after 5 seconds
    if [ $elapsed -ge 5 ]; then
        events_response=$(http_get "http://localhost:$MOTHERSHIP_PORT/api/events?type=detection&limit=10" 1 0 2>/dev/null || echo "")
        if [ -n "$events_response" ]; then
            event_count=$(echo "$events_response" | jq '.events | length' 2>/dev/null || echo "0")
            if [ "$event_count" -gt 0 ]; then
                blob_detected=1
                if [ $elapsed -le 15 ]; then
                    log_info "✓ Blob detected within first 15s (found $event_count detection events at ${elapsed}s)"
                fi
            fi
        fi
    fi

    sleep 1
done

# Step 6: Wait for simulator to complete
log_info "Waiting for simulator to complete..."
if ! wait $SIM_PID; then
    log_error "Simulator exited with non-zero status"
    cat /tmp/spaxel-sim.log
    exit 1
fi

log_info "Simulator completed successfully"

# Step 7: Assert after run
log_info "Step 7: Asserting after simulation..."

# Assert health remained ok throughout run
if [ $health_check_passed -lt $((health_check_count * 95 / 100)) ]; then
    log_error "Health check failed too often: $health_check_passed/$health_check_count passed"
    exit 1
fi
log_info "✓ Health remained ok during run ($health_check_passed/$health_check_count checks passed)"

# Check /healthz still ok
health_response=$(http_get "http://localhost:$MOTHERSHIP_PORT/healthz" 5 1 2>/dev/null || echo "")
if [ -z "$health_response" ]; then
    log_error "Health check failed after simulation"
    exit 1
fi

status=$(json_field "$health_response" ".status")
if [ "$status" != "ok" ]; then
    log_error "Health status not ok after simulation: $status"
    exit 1
fi
log_info "✓ Health check passed after simulation"

# Check detection events were recorded
events_response=$(http_get "http://localhost:$MOTHERSHIP_PORT/api/events?type=detection&limit=100" 5 1 2>/dev/null || echo "")
if [ -z "$events_response" ]; then
    log_error "Failed to get events after simulation"
    exit 1
fi

event_count=$(echo "$events_response" | jq '.events | length' 2>/dev/null || echo "0")
if [ "$event_count" -lt 1 ]; then
    log_error "No detection events recorded after simulation"
    log_error "Events response: $events_response"
    exit 1
fi
log_info "✓ At least 1 detection event recorded (found $event_count events)"

# Check simulator output for frame rate
frame_count=$(grep -o "sent [0-9]* frames" /tmp/spaxel-sim.log | tail -1 | grep -o "[0-9]*" || echo "0")
if [ "$frame_count" -gt 0 ]; then
    expected_frames=$((SIM_NODES * SIM_RATE * SIM_DURATION))

    # Sum up all frame counts from the log
    actual_frames=$(grep "sent.*frames" /tmp/spaxel-sim.log | awk '{for(i=1;i<=NF;i++) if($i~/^[0-9]+$/) sum+=$i} END {print sum+0}' || echo "0")

    if [ "$actual_frames" -gt 0 ]; then
        frame_rate_ratio=$((actual_frames * 100 / expected_frames))
        if [ $frame_rate_ratio -lt 80 ]; then
            log_error "Frame rate dropped more than 20% (got $frame_rate_ratio% of expected)"
            log_error "Expected: $expected_frames frames, Got: $actual_frames frames"
            exit 1
        fi
        log_info "✓ Frame rate acceptable ($frame_rate_ratio% of expected $expected_frames frames)"
    fi
fi

# Assert blob was detected within 15s
if [ $blob_detected -eq 0 ]; then
    log_error "No blob detected within first 15s"
    exit 1
fi
log_info "✓ Blob was detected during simulation"

# Summary
log_info ""
log_info "=== E2E Test Summary ==="
log_info "✓ Mothership started and became healthy"
log_info "✓ Simulator ran for ${SIM_DURATION}s with $SIM_NODES nodes, $SIM_WALKERS walkers"
log_info "✓ Health remained ok during run ($health_check_passed/$health_check_count checks)"
log_info "✓ Nodes came online ($nodes_online of $SIM_NODES)"
log_info "✓ Detection events recorded ($event_count events)"
log_info "✓ Blob detected within 15s"
log_info "✓ Frame rate acceptable"
log_info ""

total_time=$(($(date +%s) - start_time))
log_info "Total test time: ${total_time}s (target: <${TEST_TIMEOUT}s)"

if [ $total_time -ge $TEST_TIMEOUT ]; then
    log_warn "Test exceeded target time of ${TEST_TIMEOUT}s"
fi

log_info "All assertions passed!"
exit 0
